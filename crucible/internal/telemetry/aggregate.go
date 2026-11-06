package telemetry

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/jczastkiewicz/crucible/internal/store"
)

// MatchupKey names the two decks of a matchup in seat order.
type MatchupKey struct{ A, B string }

// Matchup is the games of one deck pair, counted by outcome. Wins are the
// first deck's and the second deck's, whichever seat or turn order they had.
type Matchup struct {
	Games int
	WinsA int
	WinsB int
	// Undecided are draws, turn caps, errors, panics and hangs.
	Undecided int
	// OnPlayWinsA counts games deck A won while going first.
	OnPlayA, OnPlayWinsA int
	OnDrawA, OnDrawWinsA int
}

// WinRate is deck A's wins over its decided games, with a 95% Wilson
// interval; ok is false with no decided game.
func (m Matchup) WinRate() (rate, lo, hi float64, ok bool) {
	return Wilson(m.WinsA, m.WinsA+m.WinsB)
}

// Wilson is the Wilson score interval at 95% for k successes in n trials.
func Wilson(k, n int) (rate, lo, hi float64, ok bool) {
	if n == 0 {
		return 0, 0, 0, false
	}
	const z = 1.959963984540054
	p := float64(k) / float64(n)
	nf := float64(n)
	denom := 1 + z*z/nf
	centre := (p + z*z/(2*nf)) / denom
	half := z * math.Sqrt(p*(1-p)/nf+z*z/(4*nf*nf)) / denom
	return p, math.Max(0, centre-half), math.Min(1, centre+half), true
}

// CardStat is MET-7's tenure rows of one card in one deck, folded.
type CardStat struct {
	Tenures   int
	HeldTotal int
	Cast      int
	Played    int
	Discarded int
	// Stuck are tenures still open when the game ended.
	Stuck int
	// BlockedTurns and PlayableUnusedTurns total the probe columns; they are
	// reported side by side and never added (MET-8).
	BlockedTurns, PlayableUnusedTurns int
	// DeadTenures are tenures blocked at least DeadThresholdTurns turns (MET-9).
	DeadTenures int
	// Blocks counts tenures by the reason that dominated them.
	Blocks map[string]int
}

// DeadThresholdTurns is MET-9's default: a card is dead in a game when the
// probe blocked it for at least this many turns.
const DeadThresholdTurns = 2

// DeadRate is the share of the card's tenures that were dead (MET-9).
func (c CardStat) DeadRate() float64 {
	if c.Tenures == 0 {
		return 0
	}
	return float64(c.DeadTenures) / float64(c.Tenures)
}

// MeanTurnsHeld is the mean of TurnsHeld over the card's tenures.
func (c CardStat) MeanTurnsHeld() float64 {
	if c.Tenures == 0 {
		return 0
	}
	return float64(c.HeldTotal) / float64(c.Tenures)
}

// Summary is a run folded: the matchups and, per deck, the cards and the mana.
type Summary struct {
	Matchups map[MatchupKey]*Matchup
	Cards    map[string]map[string]*CardStat // deck -> card -> stat
	Mana     map[string]*ManaStat            // deck -> mana health
}

// MET-10 and MET-11 defaults (MET-2: thresholds are configuration; these are
// the values in force until a run overrides them).
const (
	ScrewCurveCap         = 4
	ScrewConsecutiveTurns = 2
	ScrewWindow           = 6
	FloodStartTurn        = 5
	FloodExcess           = 2
	FloodSlack            = 2
	DeckCurveTop          = 4
)

// ManaStat is one deck's mana health over the games it played (MET-10 to
// MET-14). Turns count the deck's own turns, the first being 1.
type ManaStat struct {
	Games      int
	ScrewGames int
	FloodGames int
	// Turns sampled, and of those: no land played with none in hand, and with
	// one held (MET-14 keeps the two apart).
	Turns         int
	MissedNoLand  int
	MissedHeld    int
	UnusedMana    int
	CurveTurnsMet int
}

// ScrewRate and FloodRate are the shares of the deck's games flagged.
func (m ManaStat) ScrewRate() float64 { return share(m.ScrewGames, m.Games) }

// FloodRate is the share of the deck's games MET-11 flags.
func (m ManaStat) FloodRate() float64 { return share(m.FloodGames, m.Games) }

func share(k, n int) float64 {
	if n == 0 {
		return 0
	}
	return float64(k) / float64(n)
}

// foldMana folds one seat's samples of one game, in turn order.
func (m *ManaStat) foldMana(rows []ManaRow) {
	m.Games++
	streak, screwed, flooded := 0, false, false
	for i, r := range rows {
		turn := i + 1
		m.Turns++
		switch r.MissedDrop {
		case "no_land":
			m.MissedNoLand++
		case "held":
			m.MissedHeld++
		}
		m.UnusedMana += r.ManaAvailable - r.ManaSpent
		if r.ManaSpent >= min(turn, DeckCurveTop) {
			m.CurveTurnsMet++
		}
		if turn <= ScrewWindow {
			if r.LandsInPlay < min(turn, ScrewCurveCap) {
				streak++
				if streak >= ScrewConsecutiveTurns {
					screwed = true
				}
			} else {
				streak = 0
			}
		}
		if turn > FloodStartTurn && r.LandsInPlay+r.LandsInHand > min(turn, DeckCurveTop)+FloodExcess &&
			r.ManaAvailable-r.ManaSpent >= FloodSlack {
			flooded = true
		}
	}
	if screwed {
		m.ScrewGames++
	}
	if flooded {
		m.FloodGames++
	}
}

// Summarize streams a run's rows into a Summary. Tenure rows name the seat,
// the game row (read first in a game's rows) names that seat's deck.
func Summarize(r *store.Reader) (Summary, error) {
	s := Summary{Matchups: map[MatchupKey]*Matchup{}, Cards: map[string]map[string]*CardStat{}, Mana: map[string]*ManaStat{}}
	decks := map[int][2]string{} // game -> decks
	var tenures []TenureRow
	type seatKey struct{ game, seat int }
	manaRows := map[seatKey][]ManaRow{}
	var manaOrder []seatKey
	err := r.Each(func(row store.Row) error {
		switch row.Kind {
		case KindGame:
			var g GameRow
			if err := json.Unmarshal(row.Data, &g); err != nil {
				return err
			}
			decks[g.Game] = g.Decks
			s.addGame(g)
		case KindTenure:
			var t TenureRow
			if err := json.Unmarshal(row.Data, &t); err != nil {
				return err
			}
			tenures = append(tenures, t)
		case KindMana:
			var m ManaRow
			if err := json.Unmarshal(row.Data, &m); err != nil {
				return err
			}
			k := seatKey{m.Game, m.Seat}
			if _, seen := manaRows[k]; !seen {
				manaOrder = append(manaOrder, k)
			}
			manaRows[k] = append(manaRows[k], m)
		default:
			return fmt.Errorf("telemetry: unknown row kind %q", row.Kind)
		}
		return nil
	})
	if err != nil {
		return Summary{}, err
	}
	for _, t := range tenures {
		d, ok := decks[t.Game]
		if !ok || t.Seat < 1 || t.Seat > 2 {
			return Summary{}, fmt.Errorf("telemetry: tenure of game %d seat %d has no game row", t.Game, t.Seat)
		}
		s.addTenure(d[t.Seat-1], t)
	}
	for _, k := range manaOrder {
		d, ok := decks[k.game]
		if !ok || k.seat < 1 || k.seat > 2 {
			return Summary{}, fmt.Errorf("telemetry: mana sample of game %d seat %d has no game row", k.game, k.seat)
		}
		st := s.Mana[d[k.seat-1]]
		if st == nil {
			st = &ManaStat{}
			s.Mana[d[k.seat-1]] = st
		}
		st.foldMana(manaRows[k])
	}
	return s, nil
}

func (s *Summary) addGame(g GameRow) {
	a, b := g.Decks[0], g.Decks[1]
	swapped := b < a
	key := MatchupKey{a, b}
	if swapped {
		key = MatchupKey{b, a}
	}
	m := s.Matchups[key]
	if m == nil {
		m = &Matchup{}
		s.Matchups[key] = m
	}
	m.Games++
	// aSeat is the seat the key's first deck sat in.
	aSeat := 1
	if swapped {
		aSeat = 2
	}
	aFirst := g.OnPlay == aSeat
	if aFirst {
		m.OnPlayA++
	} else {
		m.OnDrawA++
	}
	switch {
	case g.Outcome != "won":
		m.Undecided++
	case g.Winner == aSeat:
		m.WinsA++
		if aFirst {
			m.OnPlayWinsA++
		} else {
			m.OnDrawWinsA++
		}
	default:
		m.WinsB++
	}
}

func (s *Summary) addTenure(deck string, t TenureRow) {
	cards := s.Cards[deck]
	if cards == nil {
		cards = map[string]*CardStat{}
		s.Cards[deck] = cards
	}
	c := cards[t.Card]
	if c == nil {
		c = &CardStat{}
		cards[t.Card] = c
	}
	c.Tenures++
	c.HeldTotal += t.TurnsHeld
	c.BlockedTurns += t.TurnsBlocked
	c.PlayableUnusedTurns += t.TurnsPlayableUnused
	if t.TurnsBlocked >= DeadThresholdTurns {
		c.DeadTenures++
	}
	if t.DominantBlock != "" {
		if c.Blocks == nil {
			c.Blocks = map[string]int{}
		}
		c.Blocks[t.DominantBlock]++
	}
	switch t.Exit {
	case "cast":
		c.Cast++
	case "played":
		c.Played++
	case "discarded":
		c.Discarded++
	case "end":
		c.Stuck++
	}
}

// MatchupKeys lists the matchups sorted, for stable printing.
func (s Summary) MatchupKeys() []MatchupKey {
	keys := make([]MatchupKey, 0, len(s.Matchups))
	for k := range s.Matchups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].A != keys[j].A {
			return keys[i].A < keys[j].A
		}
		return keys[i].B < keys[j].B
	})
	return keys
}
