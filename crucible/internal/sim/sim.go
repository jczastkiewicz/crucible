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
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/jczastkiewicz/crucible/internal/boosters"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/deck"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/store"
	"github.com/jczastkiewicz/crucible/internal/telemetry"
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

// Deck is one side's main deck, one entry per card (copies repeated), its
// sticker sheets (the Stickers section, CR 123.2a), its commanders (the
// Commander section), its Vanguard avatars (Avatar), its planar deck
// (Planes) and its Attraction and Contraption decks, seated at the start of
// each game. A variant section is seated
// only under its variant (Plan.Variant): Java builds the game from the main
// deck and reads each section only for its variant (Match.java:344,
// RegisteredPlayer.java:135-181, ADR-0055, ADR-0056).
type Deck struct {
	Name  string
	Cards []string
	// Prints is the edition each of Cards pins, "" where its line pins none
	// (deck.Entry.Edition); nil when no card pins one. Seating plays the
	// latest-art print for "" (ADR-0061).
	Prints     []string
	Stickers   []string
	Commanders []string
	Avatars    []string
	Planes     []string
	// Attractions and Contraptions are the Attraction and Contraption
	// decks, seated in every game (RegisteredPlayer.java:229-241).
	Attractions  []string
	Contraptions []string
}

// DeckFromList flattens a parsed decklist's main, Stickers, Commander,
// Avatar, Planes, Attractions and Contraptions sections.
func DeckFromList(name string, d *deck.Deck) Deck {
	out := Deck{Name: name}
	for _, sec := range []struct {
		section deck.Section
		into    *[]string
	}{
		{deck.Commander, &out.Commanders}, {deck.Avatar, &out.Avatars}, {deck.Planes, &out.Planes},
		{deck.Attractions, &out.Attractions}, {deck.Contraptions, &out.Contraptions},
	} {
		for _, e := range d.Cards(sec.section) {
			for i := 0; i < e.Count; i++ {
				*sec.into = append(*sec.into, e.Name)
			}
		}
	}
	pinned := false
	for _, e := range d.Cards(deck.Main) {
		for i := 0; i < e.Count; i++ {
			out.Cards = append(out.Cards, e.Name)
			out.Prints = append(out.Prints, e.Edition)
			pinned = pinned || e.Edition != ""
		}
	}
	if !pinned {
		out.Prints = nil
	}
	for _, e := range d.Cards(deck.Stickers) {
		for i := 0; i < e.Count; i++ {
			out.Stickers = append(out.Stickers, e.Name)
		}
	}
	return out
}

// defsOf looks up each name, already checked against db by RunBatch.
func defsOf(db *compile.DB, names []string) []*compile.Card {
	defs := make([]*compile.Card, 0, len(names))
	for _, name := range names {
		def, _ := db.Card(name)
		defs = append(defs, def)
	}
	return defs
}

// Job is one game of a plan: the decks in seat order and who goes first.
type Job struct {
	// Matchup numbers the deck pair the game belongs to, for reports.
	Matchup int
	Decks   [2]Deck
	// OnPlay is the seat that goes first, 1 or 2; 0 leaves it to the coin
	// flip.
	OnPlay int
}

// Opponent is one deck a gauntlet plays its target against, and how many
// games.
type Opponent struct {
	Deck  Deck
	Games int
}

// Gauntlet is the target deck against a list of opponents (R23b). Seats and
// turn order alternate game by game within a matchup, so each deck plays and
// draws equally often whatever the games count's parity allows.
type Gauntlet struct {
	Name      string
	Target    Deck
	Opponents []Opponent
}

// Jobs is the gauntlet's schedule, matchup by matchup in the order listed.
// Game g of a matchup seats the target first when g is even and puts the target
// on the play when (g/2) is even, so the four combinations of seat and turn
// order cycle.
func (gt Gauntlet) Jobs() []Job {
	var jobs []Job
	for m, o := range gt.Opponents {
		for g := 0; g < o.Games; g++ {
			j := Job{Matchup: m, Decks: [2]Deck{gt.Target, o.Deck}}
			if g%2 == 1 {
				j.Decks = [2]Deck{o.Deck, gt.Target}
			}
			targetSeat := 1 + g%2
			if (g/2)%2 == 0 {
				j.OnPlay = targetSeat
			} else {
				j.OnPlay = 3 - targetSeat
			}
			jobs = append(jobs, j)
		}
	}
	return jobs
}

// Plan is a batch to run.
type Plan struct {
	DB   *compile.DB
	Jobs []Job
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
	// Store, when set, receives each game's rows. Rows reach their shard in
	// game order whatever the worker count, so the files are reproducible.
	Store *store.Run
	// Variant is the game variant every game is set up for (ADR-0055); zero
	// is a plain constructed game.
	Variant engine.Variant
	// Boosters is the edition and booster data each game reads (Booster$
	// packs, the set<CODE> property); nil leaves those an error (GO-7). It
	// is immutable and shared by every game.
	Boosters *boosters.Table
}

// Spec is one batch of the same deck pair, the simple case of a Plan.
type Spec struct {
	DB    *compile.DB
	Decks [2]Deck
	// NewController is Plan.NewController.
	NewController func() engine.PlayerController
	BaseSeed      int64
	MaxTurns      int
	GameTimeout   time.Duration
	// Variant is Plan.Variant.
	Variant engine.Variant
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

// RunBatch plays n games of one deck pair on workers goroutines and returns
// their results ordered by game index.
func RunBatch(ctx context.Context, spec Spec, n, workers int) ([]GameResult, error) {
	jobs := make([]Job, n)
	for i := range jobs {
		jobs[i] = Job{Decks: spec.Decks}
	}
	return Run(ctx, Plan{
		DB: spec.DB, Jobs: jobs, NewController: spec.NewController, BaseSeed: spec.BaseSeed,
		MaxTurns: spec.MaxTurns, GameTimeout: spec.GameTimeout, Variant: spec.Variant,
	}, workers)
}

// Run plays a plan's games on workers goroutines and returns their results
// ordered by game index. A cancelled ctx stops handing out games; games
// already started finish, and the unplayed ones are left out of the result.
func Run(ctx context.Context, plan Plan, workers int) ([]GameResult, error) {
	if plan.DB == nil || plan.NewController == nil || plan.MaxTurns < 1 {
		return nil, fmt.Errorf("sim: a batch needs a database, a controller factory and a turn cap")
	}
	if workers < 1 {
		workers = 1
	}
	checked := map[string]bool{}
	for i, j := range plan.Jobs {
		for _, d := range j.Decks {
			if len(d.Cards) == 0 {
				return nil, fmt.Errorf("sim: game %d: deck %q is empty", i, d.Name)
			}
			if checked[d.Name] {
				continue
			}
			checked[d.Name] = true
			for _, name := range slices.Concat(d.Cards, d.Stickers, d.Commanders, d.Avatars, d.Planes, d.Attractions, d.Contraptions) {
				if _, ok := plan.DB.Card(name); !ok {
					return nil, fmt.Errorf("sim: deck %q: card %q is not in the database", d.Name, name)
				}
			}
		}
	}
	n := len(plan.Jobs)
	results := make([]GameResult, n)
	played := make([]bool, n)
	commit := make(chan finished)
	committed := make(chan error, 1)
	go func() { committed <- commitRows(plan, commit) }()

	next := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				res, rows, mana := playGame(plan, i)
				results[i] = res
				played[i] = true
				if plan.Store != nil {
					commit <- finished{idx: i, res: res, rows: rows, mana: mana}
				}
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
	close(commit)
	if err := <-committed; err != nil {
		return nil, err
	}
	out := results[:0]
	for i, r := range results {
		if played[i] {
			out = append(out, r)
		}
	}
	return out, nil
}

// finished is one game's result and rows on their way to the store.
type finished struct {
	idx  int
	res  GameResult
	rows []telemetry.TenureRow
	mana []telemetry.ManaRow
}

// commitRows writes games' rows to the store in game order: a game waits
// until every earlier game has been written, so a shard's bytes do not depend
// on which worker finished first.
func commitRows(plan Plan, in <-chan finished) error {
	if plan.Store == nil {
		for range in {
		}
		return nil
	}
	shards := plan.Store.Manifest().Shards
	open := map[int]*store.Shard{}
	var firstErr error
	write := func(f finished) {
		if firstErr != nil {
			return
		}
		n := store.ShardOf(f.idx, shards)
		sh := open[n]
		if sh == nil {
			var err error
			if sh, err = plan.Store.Shard(n); err != nil {
				firstErr = err
				return
			}
			open[n] = sh
		}
		job := plan.Jobs[f.idx]
		row := telemetry.GameRow{
			Game: f.idx, Matchup: job.Matchup, Seed: f.res.Seed, Decks: [2]string{job.Decks[0].Name, job.Decks[1].Name},
			OnPlay: job.OnPlay, Outcome: f.res.Outcome.String(), Winner: f.res.Winner, Turns: f.res.Turns,
		}
		if firstErr = sh.Write(telemetry.KindGame, row); firstErr != nil {
			return
		}
		for _, t := range f.rows {
			if firstErr = sh.Write(telemetry.KindTenure, t); firstErr != nil {
				return
			}
		}
		for _, m := range f.mana {
			if firstErr = sh.Write(telemetry.KindMana, m); firstErr != nil {
				return
			}
		}
	}
	pending := map[int]finished{}
	nextIdx := 0
	for f := range in {
		pending[f.idx] = f
		for {
			p, ok := pending[nextIdx]
			if !ok {
				break
			}
			write(p)
			delete(pending, nextIdx)
			nextIdx++
		}
	}
	// Games skipped by a cancelled run leave gaps; the rest go out in order.
	rest := make([]int, 0, len(pending))
	for idx := range pending {
		rest = append(rest, idx)
	}
	sort.Ints(rest)
	for _, idx := range rest {
		write(pending[idx])
	}
	for _, sh := range open {
		if err := sh.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// playGame runs game index with its timeout and panic isolation.
func playGame(plan Plan, index int) (GameResult, []telemetry.TenureRow, []telemetry.ManaRow) {
	seed := SeedFor(plan.BaseSeed, index)
	type out struct {
		res  GameResult
		rows []telemetry.TenureRow
		mana []telemetry.ManaRow
	}
	done := make(chan out, 1)
	go func() {
		res := GameResult{Index: index, Seed: seed}
		var rows []telemetry.TenureRow
		var mana []telemetry.ManaRow
		defer func() {
			if r := recover(); r != nil {
				res.Outcome = Panicked
				res.Detail = fmt.Sprintf("%v\n%s", r, debug.Stack())
			}
			done <- out{res, rows, mana}
		}()
		rows, mana = runOne(plan, plan.Jobs[index], index, seed, &res)
	}()
	if plan.GameTimeout <= 0 {
		o := <-done
		return o.res, o.rows, o.mana
	}
	select {
	case o := <-done:
		return o.res, o.rows, o.mana
	case <-time.After(plan.GameTimeout):
		return GameResult{Index: index, Seed: seed, Outcome: Hung, Detail: "no result within " + plan.GameTimeout.String()}, nil, nil
	}
}

// firstPlayer makes a controller start the seat a job names.
type firstPlayer struct {
	engine.PlayerController
	seat engine.PlayerID
}

func (f firstPlayer) ChooseStartingPlayer(*engine.Game, engine.PlayerID, bool) engine.PlayerID {
	return f.seat
}

// runOne plays one game to its end and fills res. It returns the hand
// tenures when the plan stores rows.
func runOne(plan Plan, job Job, index int, seed int64, res *GameResult) ([]telemetry.TenureRow, []telemetry.ManaRow) {
	g := engine.NewGame(plan.DB, javarand.New(seed), []string{job.Decks[0].Name, job.Decks[1].Name})
	if plan.Variant != 0 {
		g.SetupVariant(plan.Variant)
	}
	g.SetBoosters(plan.Boosters)
	for seat, d := range job.Decks {
		pid := g.Players()[seat]
		for i, name := range d.Cards {
			def, _ := plan.DB.Card(name)
			id := g.NewCard(def, pid, engine.Library)
			seatPrint(g, plan.Boosters, id, name, d.Prints, i)
		}
		// Player.initVariantsZones, in its order: the Vanguard avatars
		// (Player.java:2978-2985), the planar deck (:3006-3012, shuffled by
		// DealOpeningHands), the commanders (:3016-3025), then the sticker
		// sheets.
		if g.HasVariant(engine.VariantVanguard) {
			for _, name := range d.Avatars {
				def, _ := plan.DB.Card(name)
				g.SeatAvatar(pid, def)
			}
		}
		if g.HasVariant(engine.VariantPlanechase) {
			planes := make([]*compile.Card, 0, len(d.Planes))
			for _, name := range d.Planes {
				def, _ := plan.DB.Card(name)
				planes = append(planes, def)
			}
			g.SeatPlanarDeck(pid, planes)
		}
		if g.HasVariant(engine.VariantCommander) {
			for _, name := range d.Commanders {
				def, _ := plan.DB.Card(name)
				g.SeatCommander(pid, def)
			}
		}
		// Player.initVariantsZones (Player.java:3062-3089): the Attraction
		// deck, three of the deck's sheets at random into the StickerSheets
		// zone, then the Contraption deck, each deck shuffled.
		g.SeatAttractionDeck(pid, defsOf(plan.DB, d.Attractions))
		if len(d.Stickers) > 0 {
			g.SeatStickerSheets(pid, defsOf(plan.DB, d.Stickers))
		}
		g.SeatContraptionDeck(pid, defsOf(plan.DB, d.Contraptions))
	}
	var rec *telemetry.Recorder
	if plan.Store != nil {
		rec = telemetry.NewRecorder(g, index)
		g.SetSink(rec)
	}
	ctl := plan.NewController()
	if job.OnPlay != 0 {
		ctl = firstPlayer{PlayerController: ctl, seat: g.Players()[job.OnPlay-1]}
	}
	first := engine.DealOpeningHands(g, ctl)
	engine.PerformMulligans(g, ctl, first)
	// GameAction.java:2377-2381: the first plane and the planar dice.
	err := g.InitPlanechase(first)
	if err == nil {
		g.StartTurn(first, ctl)
		err = g.Run(engine.NewRegistry(), ctl, plan.MaxTurns)
	}
	res.Turns = g.Turn()
	switch {
	case err != nil:
		res.Outcome, res.Detail = Errored, err.Error()
	default:
		if perr := g.TakePendingError(); perr != nil {
			res.Outcome, res.Detail = Errored, perr.Error()
		} else if !g.Over() {
			res.Outcome = TurnCap
		} else {
			res.Outcome = Draw
			for seat, pid := range g.Players() {
				if g.Player(pid).Won {
					res.Outcome, res.Winner = Won, seat+1
				}
			}
		}
	}
	if rec == nil {
		return nil, nil
	}
	return rec.Finish(res.Turns), rec.ManaRows()
}

// seatPrint records the print of a seated main-deck card (ADR-0061): the
// edition its deck line pins, else the latest-art print Java's default
// preference picks (CardDb.java:1004-1024). Without edition data the card has
// no print.
func seatPrint(g *engine.Game, table *boosters.Table, id engine.CardID, name string, prints []string, i int) {
	if i < len(prints) && prints[i] != "" {
		g.SetPrintEdition(id, prints[i])
		return
	}
	if table == nil {
		return
	}
	if code, ok := table.LatestEdition(name); ok {
		g.SetPrintEdition(id, code)
	}
}
