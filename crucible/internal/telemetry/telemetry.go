// Package telemetry folds a game's engine events into the rows a run stores
// (ADR-0013, MET-7) and aggregates stored rows into the figures a report
// prints (R23d). The recorder runs synchronously in the game's goroutine: it
// has no queue, so a run's output cannot depend on machine load.
//
// Not yet recorded, because the engine does not yet emit what they need: the
// castability probe (MET-5, MET-6) and the per-turn mana sample (MET-10 to
// MET-19). Hand tenure therefore carries no TurnsBlocked columns.
package telemetry

import (
	"sort"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// SchemaVersion is stamped on every shard (ADR-0013); it moves when a row
// gains a field or a kind.
const SchemaVersion = 1

// MetricsVersion is MET-1's version of the metric definitions the rows were
// folded under.
const MetricsVersion = 1

// Row kinds.
const (
	KindGame   = "game"
	KindTenure = "tenure"
)

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
}

type openTenure struct {
	seat   int
	card   string
	turn   int
	source string
}

// Recorder is an engine.Sink that keeps hand tenures.
type Recorder struct {
	g     *engine.Game
	game  int
	open  map[engine.CardID]openTenure
	order []engine.CardID // open cards in the order they entered, for a stable close
	rows  []TenureRow
}

// NewRecorder records game number game of g.
func NewRecorder(g *engine.Game, game int) *Recorder {
	return &Recorder{g: g, game: game, open: map[engine.CardID]openTenure{}}
}

// Emit implements engine.Sink.
func (r *Recorder) Emit(e engine.Event) {
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
	r.rows = append(r.rows, TenureRow{
		Game: r.game, Seat: t.seat, Card: t.card, EnteredTurn: t.turn, ExitTurn: turn,
		TurnsHeld: turn - t.turn, Source: t.source, Exit: exit,
	})
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
