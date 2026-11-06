// Package telemetry folds a game's engine events into the rows a run stores
// (ADR-0013, MET-7) and aggregates stored rows into the figures a report
// prints (R23d). The recorder runs synchronously in the game's goroutine: it
// has no queue, so a run's output cannot depend on machine load.
//
// The castability probe (MET-5, MET-6) runs at the start of each active
// player's combat phase, which is the end of their precombat main phase with an
// empty stack, and fills the TurnsBlocked columns of hand tenure. The mana
// sample (MET-10 to MET-15) is taken per turn of the active player: the sources
// they could tap after their untap step, and at the end step what is left.
package telemetry

import (
	"sort"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// SchemaVersion is stamped on every shard (ADR-0013); it moves when a row
// gains a field or a kind.
const SchemaVersion = 3

// MetricsVersion is MET-1's version of the metric definitions the rows were
// folded under.
const MetricsVersion = 1

// Row kinds.
const (
	KindGame   = "game"
	KindTenure = "tenure"
	KindMana   = "mana"
)

// ManaRow is one turn's mana sample for the active player (MET-10 to MET-15).
type ManaRow struct {
	Game int `json:"game"`
	Seat int `json:"seat"`
	Turn int `json:"turn"`
	// LandsInPlay and LandsInHand are counted at the end step.
	LandsInPlay int `json:"lands_in_play"`
	LandsInHand int `json:"lands_in_hand"`
	// NonLandSources are permanents other than lands that could make mana
	// after the untap step.
	NonLandSources int `json:"nonland_sources"`
	// ManaAvailable is how many sources could be tapped for mana after the
	// untap step; ManaSpent how many of them were tapped by the end step. A
	// source tapped for something other than mana (an attacking mana creature)
	// counts as spent: the sample reads the board, not the mana pool.
	ManaAvailable int      `json:"mana_available"`
	ManaSpent     int      `json:"mana_spent"`
	Colors        []string `json:"colors,omitempty"`
	// LandPlayed is whether a land was played this turn; MissedDrop says why
	// not when none was: "no_land" (none in hand) or "held" (one was kept,
	// MET-14's AI or sequencing case).
	LandPlayed bool   `json:"land_played"`
	MissedDrop string `json:"missed_drop,omitempty"`
}

// GameRow is one game's outcome.
type GameRow struct {
	Game    int       `json:"game"`
	Matchup int       `json:"matchup"`
	Seed    int64     `json:"seed"`
	Decks   [2]string `json:"decks"`
	// OnPlay is the seat that went first, 1 or 2.
	OnPlay int `json:"on_play"`
	// Outcome is the runner's word: won, draw, turn cap, errored, panicked, hung.
	Outcome string `json:"outcome"`
	// Winner is the winning seat, 0 for none.
	Winner int `json:"winner"`
	Turns  int `json:"turns"`
}

// TenureRow is MET-7's hand tenure for one card: opened when it entered a
// hand, closed when it left or the game ended.
type TenureRow struct {
	Game        int    `json:"game"`
	Seat        int    `json:"seat"`
	Card        string `json:"card"`
	EnteredTurn int    `json:"entered_turn"`
	ExitTurn    int    `json:"exit_turn"`
	TurnsHeld   int    `json:"turns_held"`
	// Source is how it entered: opening, draw, or other.
	Source string `json:"source"`
	// Exit is how it left: cast, played, discarded, exiled, tucked, other, or
	// end (still in hand when the game ended).
	Exit string `json:"exit"`
	// TurnsBlocked and TurnsPlayableUnused are MET-7's probe columns: turns the
	// castability probe said the card could not be cast, and turns it said it
	// could and it stayed in hand. They are never summed (MET-8).
	TurnsBlocked        int `json:"turns_blocked"`
	TurnsPlayableUnused int `json:"turns_playable_unused"`
	// BlockedBy is the union of the block reasons, DominantBlock the one that
	// blocked it most turns ("" when it was never blocked).
	BlockedBy     []string `json:"blocked_by,omitempty"`
	DominantBlock string   `json:"dominant_block,omitempty"`
}

type openTenure struct {
	seat   int
	card   string
	turn   int
	source string
	// The probe's tally over the turns the card sat in hand.
	blocked, unused int
	reasons         [8]int
}

// Recorder is an engine.Sink that keeps hand tenures and mana samples.
type Recorder struct {
	g     *engine.Game
	game  int
	open  map[engine.CardID]openTenure
	order []engine.CardID // open cards in the order they entered, for a stable close
	rows  []TenureRow
	// The active player's sources after their untap step, waiting for the end
	// step to close the sample.
	upkeepSources []engine.ManaSource
	upkeepSeat    int
	mana          []ManaRow
}

// ManaRows are the mana samples taken so far, in turn order.
func (r *Recorder) ManaRows() []ManaRow { return r.mana }

// NewRecorder records game number game of g.
func NewRecorder(g *engine.Game, game int) *Recorder {
	return &Recorder{g: g, game: game, open: map[engine.CardID]openTenure{}}
}

// Emit implements engine.Sink.
func (r *Recorder) Emit(e engine.Event) {
	if e.Kind == engine.PhaseBegan {
		switch e.Phase {
		case engine.CombatBegin:
			r.probe(e.Active)
		case engine.Upkeep:
			r.upkeepSources, r.upkeepSeat = r.g.ManaSources(e.Active), int(e.Active)
		case engine.EndOfTurn:
			r.sampleMana(e.Active, int(e.Turn))
		}
		return
	}
	if e.Kind != engine.ZoneChanged || e.From == e.To {
		return
	}
	if e.To == engine.Hand {
		src := "other"
		if e.From == engine.Library {
			src = "draw"
			if e.Turn == 0 {
				src = "opening"
			}
		}
		r.open[e.Source] = openTenure{seat: int(e.Actor), card: r.g.Card(e.Source).Name(), turn: int(e.Turn), source: src}
		r.order = append(r.order, e.Source)
		return
	}
	if e.From != engine.Hand {
		return
	}
	t, ok := r.open[e.Source]
	if !ok {
		return
	}
	r.close(e.Source, t, int(e.Turn), exitName(e.To))
}

// probe runs the castability probe for the active player and tallies each card
// still open in their hand.
func (r *Recorder) probe(pid engine.PlayerID) {
	if len(r.open) == 0 {
		return
	}
	for _, p := range r.g.CastabilityProbe(pid) {
		t, ok := r.open[p.Card]
		if !ok {
			continue
		}
		if p.Castable {
			t.unused++
		} else {
			t.blocked++
			for i := range t.reasons {
				if p.Blocked&(1<<uint(i)) != 0 {
					t.reasons[i]++
				}
			}
		}
		r.open[p.Card] = t
	}
}

// sampleMana closes the turn's mana sample at the active player's end step.
func (r *Recorder) sampleMana(pid engine.PlayerID, turn int) {
	if r.upkeepSeat != int(pid) {
		return
	}
	row := ManaRow{Game: r.game, Seat: int(pid), Turn: turn, ManaAvailable: len(r.upkeepSources)}
	still := map[engine.CardID]bool{}
	for _, s := range r.g.ManaSources(pid) {
		still[s.Card] = true
	}
	var colors mana.Colors
	seen := map[engine.CardID]bool{}
	for _, s := range r.upkeepSources {
		colors |= s.Colors
		if !still[s.Card] {
			row.ManaSpent++
		}
		if !seen[s.Card] && !r.g.Card(s.Card).Type().Has(cardtype.Land) {
			row.NonLandSources++
		}
		seen[s.Card] = true
	}
	for _, c := range [...]mana.Colors{mana.White, mana.Blue, mana.Black, mana.Red, mana.Green} {
		if colors&c != 0 {
			row.Colors = append(row.Colors, c.String())
		}
	}
	for _, id := range r.g.Zone(engine.Battlefield, pid).Cards() {
		if r.g.Card(id).Type().Has(cardtype.Land) {
			row.LandsInPlay++
		}
	}
	for _, id := range r.g.Zone(engine.Hand, pid).Cards() {
		if r.g.Card(id).Type().Has(cardtype.Land) {
			row.LandsInHand++
		}
	}
	row.LandPlayed = r.g.Player(pid).LandsPlayed > 0
	if !row.LandPlayed {
		row.MissedDrop = "no_land"
		if row.LandsInHand > 0 {
			row.MissedDrop = "held"
		}
	}
	r.mana = append(r.mana, row)
	r.upkeepSources, r.upkeepSeat = nil, 0
}

func exitName(to engine.ZoneType) string {
	switch to {
	case engine.Stack:
		return "cast"
	case engine.Battlefield:
		return "played"
	case engine.Graveyard:
		return "discarded"
	case engine.Exile:
		return "exiled"
	case engine.Library:
		return "tucked"
	}
	return "other"
}

func (r *Recorder) close(id engine.CardID, t openTenure, turn int, exit string) {
	delete(r.open, id)
	row := TenureRow{
		Game: r.game, Seat: t.seat, Card: t.card, EnteredTurn: t.turn, ExitTurn: turn,
		TurnsHeld: turn - t.turn, Source: t.source, Exit: exit,
		TurnsBlocked: t.blocked, TurnsPlayableUnused: t.unused,
	}
	most := 0
	for i, n := range t.reasons {
		if n == 0 {
			continue
		}
		name := engine.CastBlock(1 << uint(i)).Names()[0]
		row.BlockedBy = append(row.BlockedBy, name)
		if n > most {
			most, row.DominantBlock = n, name
		}
	}
	r.rows = append(r.rows, row)
}

// Finish closes the tenures still open at the end of the game on turn and
// returns every row in the order the tenures closed.
func (r *Recorder) Finish(turn int) []TenureRow {
	for _, id := range r.order {
		if t, ok := r.open[id]; ok {
			r.close(id, t, turn, "end")
		}
	}
	r.order = nil
	return r.rows
}

// SortedCards lists the distinct cards of rows, sorted; a helper for tests and
// tools.
func SortedCards(rows []TenureRow) []string {
	seen := map[string]bool{}
	for _, row := range rows {
		seen[row.Card] = true
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}
