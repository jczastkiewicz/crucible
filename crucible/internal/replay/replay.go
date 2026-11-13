package replay

import (
	"fmt"
	"runtime/debug"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// Result is what replaying a log found.
type Result struct {
	// Turns is the turn the Go game stopped in.
	Turns int
	// Question is the first question the log could not answer, nil if none.
	Question *Question
	// Events is the first divergence between the two games' events, nil if
	// none.
	Events *Divergence
	// Err is the Go game's own error or panic, nil if none.
	Err error
	// Go is the Go game's normalised events, for a report that shows them.
	Go []Event
}

// Same reports whether the Go game played the recorded game: every question
// answered from the log and no event divergence.
func (r *Result) Same() bool { return r.Question == nil && r.Events == nil && r.Err == nil }

// Run replays log in a Go game over db up to maxTurns: the recorded decks
// seated, every shuffle in its recorded order, every question answered by
// [Controller], every event compared (ADR-0010 L3).
func Run(db *compile.DB, log *Log, maxTurns int) (*Result, error) {
	s, err := newScript(log)
	if err != nil {
		return nil, err
	}
	g, ids, err := setup(db, log)
	if err != nil {
		return nil, err
	}
	g.SetShuffler(&shuffler{s: s, ids: ids})
	norm := NewNormalizer(ids)
	g.SetSink(norm)
	ctl := newController(g, s, ids)
	res := &Result{}
	func() {
		defer func() {
			if r := recover(); r != nil {
				res.Err = fmt.Errorf("replay: Go game panicked: %v\n%s", r, debug.Stack())
			}
		}()
		// Match.prepareAllZones (Match.java:352-356): each seat's companion
		// before the libraries are shuffled.
		for _, pid := range g.Players() {
			if res.Err = g.AssignCompanion(pid, ctl); res.Err != nil {
				return
			}
		}
		first := engine.DealOpeningHands(g, ctl)
		engine.PerformMulligans(g, ctl, first)
		reg := engine.NewRegistry()
		if first, res.Err = engine.RunOpeningHandActions(g, reg, ctl, first); res.Err != nil {
			return
		}
		g.StartTurn(first, ctl)
		res.Err = g.Run(reg, ctl, maxTurns)
	}()
	if res.Err != nil && ctl.LastPlay() != "" {
		res.Err = fmt.Errorf("%w (last Java play: %s)", res.Err, ctl.LastPlay())
	}
	res.Turns = g.Turn()
	res.Question = ctl.Unanswered()
	res.Go = norm.Events()
	res.Events = Compare(log.Events(), res.Go)
	return res, nil
}

// setup seats the log's decks in a new Go game: each seat's cards in Java id
// order -- the order Java created them in, deck order -- the starting
// library's in the library and the rest in the sideboard, each paired with
// its Java id.
func setup(db *compile.DB, log *Log) (*engine.Game, *IDMap, error) {
	h := log.Header
	if len(h.StartingLibraries) != len(h.Seats) {
		return nil, nil, fmt.Errorf("replay: %d seats, %d starting libraries", len(h.Seats), len(h.StartingLibraries))
	}
	names := make([]string, len(h.Seats))
	for i, s := range h.Seats {
		names[i] = s.Deck
	}
	g := engine.NewGame(db, javarand.New(h.Seed), names)
	ids := newIDMap(g, log)
	for i, s := range h.Seats {
		pid := g.Players()[i]
		inLibrary := map[int]bool{}
		for _, id := range h.StartingLibraries[i] {
			inLibrary[id] = true
		}
		g.Player(pid).StartingLibrarySize = len(h.StartingLibraries[i])
		for _, c := range s.Cards {
			name, ok := db.Resolve(c.Name)
			def, _ := db.Card(name)
			if !ok || def == nil {
				return nil, nil, fmt.Errorf("replay: seat %d: no card %q", i, c.Name)
			}
			zone := engine.Sideboard
			if inLibrary[c.Card] {
				zone = engine.Library
			}
			ids.pair(c.Card, g.NewCard(def, pid, zone))
		}
	}
	return g, ids, nil
}

// shuffler gives each Go library shuffle the next recorded order of that
// seat's.
type shuffler struct {
	s    *script
	ids  *IDMap
	used map[int]int
}

func (sh *shuffler) Order(kind engine.ZoneType, owner engine.PlayerID, _ []engine.CardID) ([]engine.CardID, bool) {
	if kind != engine.Library {
		return nil, false
	}
	if sh.used == nil {
		sh.used = map[int]int{}
	}
	st := seat(owner)
	q := sh.s.shuffles[st]
	if sh.used[st] >= len(q) {
		return nil, false
	}
	rec := q[sh.used[st]]
	sh.used[st]++
	out := make([]engine.CardID, 0, len(rec.Order))
	for _, j := range rec.Order {
		id, ok := sh.ids.GoCard(j)
		if !ok {
			return nil, false
		}
		out = append(out, id)
	}
	return out, true
}
