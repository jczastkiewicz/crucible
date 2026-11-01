package sim_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/deck"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/sim"
	"github.com/jczastkiewicz/crucible/internal/smoke"
	"github.com/jczastkiewicz/crucible/internal/store"
	"github.com/jczastkiewicz/crucible/internal/telemetry"
)

func blankDB(t *testing.T, names ...string) *compile.DB {
	t.Helper()
	cards := make(map[string]*compile.Card, len(names))
	for _, name := range names {
		raw := &carddb.Card{Filename: name, Faces: [carddb.NumFaces]carddb.Face{{Present: true, Name: name}}}
		c, err := compile.Compile(raw)
		if err != nil {
			t.Fatalf("compile %q: %v", name, err)
		}
		cards[name] = c
	}
	return compile.NewDB(cards)
}

func blankDeck(name string, n int) sim.Deck {
	return sim.Deck{Name: name, Cards: slices.Repeat([]string{"Blank"}, n)}
}

func spec(t *testing.T) sim.Spec {
	t.Helper()
	return sim.Spec{
		DB:            blankDB(t, "Blank"),
		Decks:         [2]sim.Deck{blankDeck("A", 40), blankDeck("B", 40)},
		NewController: func() engine.PlayerController { return &smoke.Controller{} },
		BaseSeed:      7,
		MaxTurns:      200,
	}
}

// Two blank 40-card decks and a controller that only passes: the player who
// draws from an empty library first loses, so every game is won by a seat.
func TestBatchPlaysGamesToTheirEnd(t *testing.T) {
	t.Parallel()
	got, err := sim.RunBatch(context.Background(), spec(t), 6, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 6 {
		t.Fatalf("%d results, want 6", len(got))
	}
	for i, r := range got {
		if r.Index != i || r.Seed != sim.SeedFor(7, i) {
			t.Errorf("result %d = index %d seed %d, want index %d seed %d", i, r.Index, r.Seed, i, sim.SeedFor(7, i))
		}
		if r.Outcome != sim.Won || r.Winner < 1 || r.Winner > 2 {
			t.Errorf("game %d: outcome %v winner %d detail %q, want a seat to win by decking", i, r.Outcome, r.Winner, r.Detail)
		}
	}
}

// The same base seed gives the same results whatever the worker count.
func TestBatchIsReproducibleFromItsSeed(t *testing.T) {
	t.Parallel()
	a, err := sim.RunBatch(context.Background(), spec(t), 8, 1)
	if err != nil {
		t.Fatal(err)
	}
	b, err := sim.RunBatch(context.Background(), spec(t), 8, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a, b) {
		t.Errorf("one worker %v, four workers %v: want the same results", a, b)
	}
}

func TestSeedsOfNeighbouringGamesDiffer(t *testing.T) {
	t.Parallel()
	seen := map[int64]bool{}
	for i := 0; i < 100; i++ {
		s := sim.SeedFor(1, i)
		if seen[s] {
			t.Fatalf("game %d repeats seed %d", i, s)
		}
		seen[s] = true
	}
	if sim.SeedFor(1, 0) == sim.SeedFor(2, 0) {
		t.Error("two base seeds give the same first game seed")
	}
}

func TestTurnCapEndsAnUndecidedGame(t *testing.T) {
	t.Parallel()
	s := spec(t)
	s.MaxTurns = 5
	got, err := sim.RunBatch(context.Background(), s, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range got {
		if r.Outcome != sim.TurnCap || r.Turns != 5 {
			t.Errorf("game %d: outcome %v after %d turns, want the cap at turn 5", r.Index, r.Outcome, r.Turns)
		}
	}
}

// panicker panics in the game its factory call number is panicAt; the others
// play on.
type panicker struct {
	*smoke.Controller
	boom bool
}

func (p panicker) ChooseStartingPlayer(g *engine.Game, decider engine.PlayerID, first bool) engine.PlayerID {
	if p.boom {
		panic("bad card")
	}
	return p.Controller.ChooseStartingPlayer(g, decider, first)
}

func TestAPanickingGameFailsOnlyItself(t *testing.T) {
	t.Parallel()
	s := spec(t)
	var calls atomic.Int32
	s.NewController = func() engine.PlayerController {
		return panicker{Controller: &smoke.Controller{}, boom: calls.Add(1) == 1}
	}
	got, err := sim.RunBatch(context.Background(), s, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Outcome != sim.Panicked || !strings.Contains(got[0].Detail, "bad card") {
		t.Errorf("result %+v, want a recovered panic naming the cause", got[0])
	}
	s.NewController = func() engine.PlayerController { return &smoke.Controller{} }
	if again, _ := sim.RunBatch(context.Background(), s, 2, 2); again[0].Outcome != sim.Won || again[1].Outcome != sim.Won {
		t.Errorf("after a panic the next games are %v and %v, want won", again[0].Outcome, again[1].Outcome)
	}
}

// stuck blocks the game forever on its first decision.
type stuck struct{ *smoke.Controller }

func (s stuck) ChooseStartingPlayer(*engine.Game, engine.PlayerID, bool) engine.PlayerID {
	select {}
}

func TestAHungGameIsReportedAndAbandoned(t *testing.T) {
	t.Parallel()
	s := spec(t)
	s.GameTimeout = 50 * time.Millisecond
	s.NewController = func() engine.PlayerController { return stuck{&smoke.Controller{}} }
	got, err := sim.RunBatch(context.Background(), s, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Outcome != sim.Hung {
		t.Errorf("outcome %v, want hung", got[0].Outcome)
	}
}

func TestBatchRefusesWhatItCannotRun(t *testing.T) {
	t.Parallel()
	s := spec(t)
	s.Decks[1].Cards = []string{"Missing"}
	if _, err := sim.RunBatch(context.Background(), s, 1, 1); err == nil || !strings.Contains(err.Error(), "Missing") {
		t.Errorf("unknown card: err = %v, want it named", err)
	}
	s = spec(t)
	s.Decks[0].Cards = nil
	if _, err := sim.RunBatch(context.Background(), s, 1, 1); err == nil {
		t.Error("an empty deck did not error")
	}
	s = spec(t)
	s.MaxTurns = 0
	if _, err := sim.RunBatch(context.Background(), s, 1, 1); err == nil {
		t.Error("no turn cap did not error")
	}
}

func TestCancelledBatchStopsHandingOutGames(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := sim.RunBatch(ctx, spec(t), 50, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) >= 50 {
		t.Errorf("%d games played after cancel, want fewer than 50", len(got))
	}
}

func TestOutcomeNames(t *testing.T) {
	t.Parallel()
	for o, want := range map[sim.Outcome]string{sim.Won: "won", sim.Draw: "draw", sim.TurnCap: "turn cap", sim.Errored: "errored", sim.Panicked: "panicked", sim.Hung: "hung"} {
		if o.String() != want {
			t.Errorf("%d = %q, want %q", o, o.String(), want)
		}
	}
}

func TestDeckFromListRepeatsEachCopy(t *testing.T) {
	t.Parallel()
	d := deck.Parse("t.dck", []byte("[Main]\n2 Island\n1 Mountain\n[Sideboard]\n3 Forest\n"))
	got := sim.DeckFromList("T", d)
	if want := []string{"Island", "Island", "Mountain"}; !slices.Equal(got.Cards, want) || got.Name != "T" {
		t.Errorf("deck %+v, want main cards %v", got, want)
	}
}

func TestGauntletAlternatesSeatsAndTurnOrder(t *testing.T) {
	t.Parallel()
	gt := sim.Gauntlet{
		Name:   "g",
		Target: sim.Deck{Name: "T", Cards: []string{"Blank"}},
		Opponents: []sim.Opponent{
			{Deck: sim.Deck{Name: "X", Cards: []string{"Blank"}}, Games: 4},
			{Deck: sim.Deck{Name: "Y", Cards: []string{"Blank"}}, Games: 1},
		},
	}
	jobs := gt.Jobs()
	if len(jobs) != 5 {
		t.Fatalf("%d jobs, want 5", len(jobs))
	}
	type want struct {
		matchup, targetSeat, onPlay int
	}
	for i, w := range []want{{0, 1, 1}, {0, 2, 2}, {0, 1, 2}, {0, 2, 1}, {1, 1, 1}} {
		j := jobs[i]
		seat := 1
		if j.Decks[1].Name == "T" {
			seat = 2
		}
		if j.Matchup != w.matchup || seat != w.targetSeat || j.OnPlay != w.onPlay {
			t.Errorf("job %d: matchup %d, target seat %d, on play %d; want %+v", i, j.Matchup, seat, j.OnPlay, w)
		}
	}
	// Over four games the target plays and draws twice from each seat order.
	var play int
	for _, j := range jobs[:4] {
		seat := 1
		if j.Decks[1].Name == "T" {
			seat = 2
		}
		if j.OnPlay == seat {
			play++
		}
	}
	if play != 2 {
		t.Errorf("the target is on the play in %d of 4 games, want 2", play)
	}
}

func TestPlanForcesWhoGoesFirst(t *testing.T) {
	t.Parallel()
	s := spec(t)
	var jobs []sim.Job
	for i := 0; i < 4; i++ {
		jobs = append(jobs, sim.Job{Decks: s.Decks, OnPlay: 1 + i%2})
	}
	dir := t.TempDir()
	run, err := store.Create(dir, store.Manifest{RunID: "r", SchemaVersion: telemetry.SchemaVersion, MetricsVersion: telemetry.MetricsVersion, Shards: 2, Games: 4})
	if err != nil {
		t.Fatal(err)
	}
	_, err = sim.Run(context.Background(), sim.Plan{DB: s.DB, Jobs: jobs, NewController: s.NewController, BaseSeed: 3, MaxTurns: 200, Store: run}, 2)
	if err != nil {
		t.Fatal(err)
	}
	r, err := store.Open(dir, telemetry.SchemaVersion, telemetry.MetricsVersion)
	if err != nil {
		t.Fatal(err)
	}
	var winners, games int
	_ = r.Each(func(row store.Row) error {
		if row.Kind != telemetry.KindGame {
			return nil
		}
		var g telemetry.GameRow
		if err := json.Unmarshal(row.Data, &g); err != nil {
			t.Fatal(err)
		}
		games++
		// Blank decks: the player on the draw draws first and so empties the
		// library first; the player on the play wins.
		if g.Winner == g.OnPlay {
			winners++
		}
		return nil
	})
	if games != 4 || winners != 4 {
		t.Errorf("%d games, the player on the play won %d, want 4 and 4", games, winners)
	}
}

// Rows reach the shards in game order, so the files are the same whatever the
// worker count.
func TestStoredShardsDoNotDependOnWorkers(t *testing.T) {
	t.Parallel()
	read := func(workers int) map[string][]byte {
		s := spec(t)
		dir := t.TempDir()
		run, err := store.Create(dir, store.Manifest{RunID: "r", SchemaVersion: 1, MetricsVersion: 1, Shards: 3, Games: 9})
		if err != nil {
			t.Fatal(err)
		}
		jobs := make([]sim.Job, 9)
		for i := range jobs {
			jobs[i] = sim.Job{Decks: s.Decks}
		}
		if _, err := sim.Run(context.Background(), sim.Plan{DB: s.DB, Jobs: jobs, NewController: s.NewController, BaseSeed: 11, MaxTurns: 200, Store: run}, workers); err != nil {
			t.Fatal(err)
		}
		out := map[string][]byte{}
		files, _ := filepath.Glob(filepath.Join(dir, "shards", "*"))
		for _, f := range files {
			b, _ := os.ReadFile(f)
			out[filepath.Base(f)] = b
		}
		return out
	}
	a, b := read(1), read(5)
	if len(a) != 3 || len(b) != 3 {
		t.Fatalf("%d and %d shards, want 3 each", len(a), len(b))
	}
	for name, bytesA := range a {
		if !bytes.Equal(bytesA, b[name]) {
			t.Errorf("%s differs between one and five workers", name)
		}
	}
}

// The recorder turns zone changes into hand tenures: every drawn card of a
// blank game is held until the game ends or it is discarded.
func TestRecorderKeepsHandTenures(t *testing.T) {
	t.Parallel()
	s := spec(t)
	dir := t.TempDir()
	run, _ := store.Create(dir, store.Manifest{RunID: "r", SchemaVersion: 1, MetricsVersion: 1, Shards: 1, Games: 1})
	if _, err := sim.Run(context.Background(), sim.Plan{DB: s.DB, Jobs: []sim.Job{{Decks: s.Decks}}, NewController: s.NewController, BaseSeed: 2, MaxTurns: 200, Store: run}, 1); err != nil {
		t.Fatal(err)
	}
	r, _ := store.Open(dir, 0, 0)
	sum, err := telemetry.Summarize(r)
	if err != nil {
		t.Fatal(err)
	}
	stat := sum.Cards["A"]["Blank"]
	if stat == nil || stat.Tenures < 7 {
		t.Fatalf("A's Blank tenures %+v, want at least the opening seven", stat)
	}
	if stat.Stuck == 0 {
		t.Errorf("A's Blank %+v: cards still in hand at the end should be counted as stuck", *stat)
	}
	rows := 0
	_ = r.Each(func(row store.Row) error {
		if row.Kind == telemetry.KindTenure {
			rows++
		}
		return nil
	})
	if rows == 0 {
		t.Error("no tenure rows stored")
	}
}
