// Package sim runs batches of games: a worker pool over independent
// goroutine-per-game engines, each with its own seed, a turn cap, and crash
// isolation so one bad game fails only itself (ADR-0005, GO-7).
//
// Games share only the immutable card database. A game's result is a pure
// function of its deck pair, its seed and its controller, so a batch re-run
// from the same base seed gives the same results in the same order.
package sim

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/deck"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// Outcome says how one game ended.
type Outcome uint8

// The ways a game ends. Only Won and Draw are rules outcomes; the rest are
// the runner's, and the batch counts them separately (the P7 gate wants zero
// Panicked and zero Hung).
const (
	// Won: one seat won.
	Won Outcome = iota
	// Draw: the game ended with no winner, both seats lost at once.
	Draw
	// TurnCap: the turn cap ended the game undecided.
	TurnCap
	// Errored: the engine returned an error (a refusal, GO-7).
	Errored
	// Panicked: the engine panicked; the panic was recovered at the game
	// boundary (GO-7).
	Panicked
	// Hung: the game did not finish within the wall-clock limit. Its
	// goroutine is abandoned, since a running engine cannot be interrupted.
	Hung
)

var outcomeNames = [...]string{"won", "draw", "turn cap", "errored", "panicked", "hung"}

func (o Outcome) String() string { return outcomeNames[o] }

// Deck is one side's main deck, one entry per card (copies repeated).
type Deck struct {
	Name  string
	Cards []string
}

// DeckFromList flattens a parsed decklist's main section.
func DeckFromList(name string, d *deck.Deck) Deck {
	out := Deck{Name: name}
	for _, e := range d.Cards(deck.Main) {
		for i := 0; i < e.Count; i++ {
			out.Cards = append(out.Cards, e.Name)
		}
	}
	return out
}

// Spec is one batch.
type Spec struct {
	DB *compile.DB
	// Decks seat 1 and seat 2.
	Decks [2]Deck
	// NewController makes the controller one game uses; it answers for both
	// seats and is not shared between games.
	NewController func() engine.PlayerController
	// BaseSeed and the game index give each game's seed (SeedFor).
	BaseSeed int64
	// MaxTurns is the turn cap; a game still undecided after that turn's
	// Cleanup ends as TurnCap.
	MaxTurns int
	// GameTimeout bounds one game's wall-clock time; 0 means none.
	GameTimeout time.Duration
}

// GameResult is one game's record.
type GameResult struct {
	Index   int
	Seed    int64
	Outcome Outcome
	// Winner is the winning seat, 1 or 2; 0 when there is none.
	Winner int
	// Turns is the turn number the game ended on.
	Turns int
	// Detail is the error or panic text, empty for a rules outcome.
	Detail string
}

// SeedFor is the seed of game index of a batch: SplitMix64 over the base seed
// and the index, so neighbouring games get unrelated streams.
func SeedFor(base int64, index int) int64 {
	z := uint64(base) + uint64(index+1)*0x9E3779B97F4A7C15
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return int64(z ^ (z >> 31))
}

// RunBatch plays n games on workers goroutines and returns their results
// ordered by game index. A cancelled ctx stops handing out games; games
// already started finish, and the unplayed ones are left out of the result.
func RunBatch(ctx context.Context, spec Spec, n, workers int) ([]GameResult, error) {
	if spec.DB == nil || spec.NewController == nil || spec.MaxTurns < 1 {
		return nil, fmt.Errorf("sim: a batch needs a database, a controller factory and a turn cap")
	}
	if workers < 1 {
		workers = 1
	}
	for i, d := range spec.Decks {
		if len(d.Cards) == 0 {
			return nil, fmt.Errorf("sim: deck %d (%q) is empty", i+1, d.Name)
		}
		for _, name := range d.Cards {
			if _, ok := spec.DB.Card(name); !ok {
				return nil, fmt.Errorf("sim: deck %q: card %q is not in the database", d.Name, name)
			}
		}
	}
	results := make([]GameResult, n)
	played := make([]bool, n)
	next := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				results[i] = playGame(spec, i)
				played[i] = true
			}
		}()
	}
feed:
	for i := 0; i < n; i++ {
		select {
		case next <- i:
		case <-ctx.Done():
			break feed
		}
	}
	close(next)
	wg.Wait()
	out := results[:0]
	for i, r := range results {
		if played[i] {
			out = append(out, r)
		}
	}
	return out, nil
}

// playGame runs game index with its timeout and panic isolation.
func playGame(spec Spec, index int) GameResult {
	seed := SeedFor(spec.BaseSeed, index)
	done := make(chan GameResult, 1)
	go func() {
		res := GameResult{Index: index, Seed: seed}
		defer func() {
			if r := recover(); r != nil {
				res.Outcome = Panicked
				res.Detail = fmt.Sprintf("%v\n%s", r, debug.Stack())
			}
			done <- res
		}()
		runOne(spec, seed, &res)
	}()
	if spec.GameTimeout <= 0 {
		return <-done
	}
	select {
	case res := <-done:
		return res
	case <-time.After(spec.GameTimeout):
		return GameResult{Index: index, Seed: seed, Outcome: Hung, Detail: "no result within " + spec.GameTimeout.String()}
	}
}

// runOne plays one game to its end and fills res.
func runOne(spec Spec, seed int64, res *GameResult) {
	g := engine.NewGame(spec.DB, javarand.New(seed), []string{spec.Decks[0].Name, spec.Decks[1].Name})
	for seat, d := range spec.Decks {
		pid := g.Players()[seat]
		for _, name := range d.Cards {
			def, _ := spec.DB.Card(name)
			g.NewCard(def, pid, engine.Library)
		}
	}
	ctl := spec.NewController()
	first := engine.DealOpeningHands(g, ctl)
	engine.PerformMulligans(g, ctl, first)
	g.StartTurn(first, ctl)
	if err := g.Run(engine.NewRegistry(), ctl, spec.MaxTurns); err != nil {
		res.Outcome, res.Detail = Errored, err.Error()
		return
	}
	res.Turns = g.Turn()
	if err := g.TakePendingError(); err != nil {
		res.Outcome, res.Detail = Errored, err.Error()
		return
	}
	if !g.Over() {
		res.Outcome = TurnCap
		return
	}
	res.Outcome = Draw
	for seat, pid := range g.Players() {
		if g.Player(pid).Won {
			res.Outcome, res.Winner = Won, seat+1
		}
	}
}
