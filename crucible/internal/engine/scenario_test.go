package engine_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/fixture"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// TestScenarios is TEST-5's fixture directory walk (Plan Section 3.3, Layer
// 2): one Go test, adding a case is adding a directory.
//
// Each scenario directory is:
//
//	setup.state    GameState text (fixture.Parse/Load)
//	actions.log    ordered, explicit decisions (fixture.RunActions) -- optional
//	expect.state   GameState text describing the game the scenario should
//	               produce
//
// Because no AI is involved (ScriptedController answers every decision the
// scenario itself queues), any mismatch against expect.state is a rules bug,
// never an AI one -- the reasoning that makes this harness worth having
// rather than just a pile of unit tests (Plan Section 3.3).
//
// setup.state and expect.state are loaded against the real corpus, not a
// synthetic three-card database: a scenario naming a card the corpus does
// not have is a scenario with a typo, and this is what would catch it. The
// corpus loads once per test binary run, not once per scenario -- 33,697
// cards is too much to pay for twice, let alone per case.
func TestScenarios(t *testing.T) {
	root := filepath.Join(crucibleModuleRoot(t), "testdata", "scenarios")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("no scenarios yet")
		}
		t.Fatalf("read %s: %v", root, err)
	}

	db := scenarioDB(t)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			runScenario(t, db, filepath.Join(root, entry.Name()))
		})
	}
}

func runScenario(t *testing.T, db *compile.DB, dir string) {
	t.Helper()

	setup := loadScenarioFile(t, db, filepath.Join(dir, "setup.state"))

	actionsPath := filepath.Join(dir, "actions.log")
	controller := engine.NewScriptedController()
	if raw, err := os.ReadFile(actionsPath); err == nil {
		if err := fixture.RunActions(bytes.NewReader(raw), setup, controller); err != nil {
			t.Fatalf("run actions.log: %v", err)
		}
	} else if !os.IsNotExist(err) {
		t.Fatalf("read actions.log: %v", err)
	}

	want := loadScenarioFile(t, db, filepath.Join(dir, "expect.state"))
	compareGames(t, setup.Game, want.Game)
}

func loadScenarioFile(t *testing.T, db *compile.DB, path string) *fixture.Loaded {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	st, err := fixture.Parse(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	l, err := fixture.Load(st, db, javarand.New(1))
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	return l
}

// compareGames checks got against want directly, rather than through
// fixture.Dump: Dump's Id: is a CardID (game-state-fixture.md), and got and
// want are two independently loaded games whose CardIDs were never going to
// agree by number. Comparing the games themselves sidesteps that entirely,
// and reaches fields Dump does not carry at all.
//
// Lost, Won and Over are compared too, now that fixture.State has lost=,
// won= and over= keys for them (Crucible-only -- GameState.java's own format
// has none, since a fixture is always a still-being-played snapshot there).
// Before those existed, an expect.state loaded fresh always reported them
// false regardless of what a scenario intended, so comparing them would have
// failed every scenario that legitimately ends the game.
//
// Players are matched by position in seating order (Players()[i] against
// Players()[i]), which holds as long as setup.state and expect.state name
// the same players in the same slots -- true of every scenario this harness
// is meant to run.
func compareGames(t *testing.T, got, want *engine.Game) {
	t.Helper()

	if got.Turn() != want.Turn() {
		t.Errorf("turn = %d, want %d", got.Turn(), want.Turn())
	}
	if got.ActivePhase() != want.ActivePhase() {
		t.Errorf("activephase = %v, want %v", got.ActivePhase(), want.ActivePhase())
	}
	if got.Over() != want.Over() {
		t.Errorf("over = %v, want %v", got.Over(), want.Over())
	}

	gotPlayers, wantPlayers := got.Players(), want.Players()
	if len(gotPlayers) != len(wantPlayers) {
		t.Fatalf("player count = %d, want %d", len(gotPlayers), len(wantPlayers))
	}
	for i := range gotPlayers {
		gp, wp := got.Player(gotPlayers[i]), want.Player(wantPlayers[i])
		if gp.Name != wp.Name {
			t.Errorf("player %d name = %q, want %q", i, gp.Name, wp.Name)
			continue
		}
		gotIsActive := got.ActivePlayer() == gotPlayers[i]
		wantIsActive := want.ActivePlayer() == wantPlayers[i]
		if gotIsActive != wantIsActive {
			t.Errorf("%s: is the active player = %v, want %v", gp.Name, gotIsActive, wantIsActive)
		}
		if gp.Lost != wp.Lost {
			t.Errorf("%s: lost = %v, want %v", gp.Name, gp.Lost, wp.Lost)
		}
		if gp.Won != wp.Won {
			t.Errorf("%s: won = %v, want %v", gp.Name, gp.Won, wp.Won)
		}
		if gp.Life != wp.Life {
			t.Errorf("%s: life = %d, want %d", gp.Name, gp.Life, wp.Life)
		}
		if got, want := gp.ManaPool.Breakdown(), wp.ManaPool.Breakdown(); got != want {
			t.Errorf("%s: mana pool = %v, want %v", gp.Name, got, want)
		}
		if got, want := gp.ManaPool.SnowBreakdown(), wp.ManaPool.SnowBreakdown(); got != want {
			t.Errorf("%s: mana pool snow breakdown = %v, want %v", gp.Name, got, want)
		}
		if got, want := gp.LandsPlayed, wp.LandsPlayed; got != want {
			t.Errorf("%s: lands played = %d, want %d", gp.Name, got, want)
		}
		if got, want := gp.LandsPlayedLastTurn, wp.LandsPlayedLastTurn; got != want {
			t.Errorf("%s: lands played last turn = %d, want %d", gp.Name, got, want)
		}
		compareCounters(t, gp.Name, gp.Counters, wp.Counters)
		for _, zone := range []engine.ZoneType{
			engine.Battlefield, engine.Hand, engine.Graveyard, engine.Library,
			engine.Exile, engine.Command, engine.Sideboard,
		} {
			compareZoneCards(t, gp.Name, zone, got, gotPlayers[i], want, wantPlayers[i])
		}
	}
}

func compareZoneCards(t *testing.T, playerName string, zone engine.ZoneType, got *engine.Game, gpid engine.PlayerID, want *engine.Game, wpid engine.PlayerID) {
	t.Helper()

	gc, wc := got.Zone(zone, gpid).Cards(), want.Zone(zone, wpid).Cards()
	if len(gc) != len(wc) {
		t.Errorf("%s %s has %d cards, want %d", playerName, zone, len(gc), len(wc))
		return
	}
	for i := range gc {
		gcard, wcard := got.Card(gc[i]), want.Card(wc[i])
		label := fmt.Sprintf("%s %s[%d]", playerName, zone, i)

		if gcard.Def.Name != wcard.Def.Name {
			t.Errorf("%s name = %q, want %q", label, gcard.Def.Name, wcard.Def.Name)
			continue
		}
		if gcard.Tapped != wcard.Tapped {
			t.Errorf("%s tapped = %v, want %v", label, gcard.Tapped, wcard.Tapped)
		}
		if gcard.SummonSick != wcard.SummonSick {
			t.Errorf("%s summonsick = %v, want %v", label, gcard.SummonSick, wcard.SummonSick)
		}
		if gcard.Damage.Marked != wcard.Damage.Marked {
			t.Errorf("%s damage = %d, want %d", label, gcard.Damage.Marked, wcard.Damage.Marked)
		}
		_, gAttached := gcard.AttachedTo()
		_, wAttached := wcard.AttachedTo()
		if gAttached != wAttached {
			t.Errorf("%s attached = %v, want %v", label, gAttached, wAttached)
		}
		// By name, not PlayerID: got and want are two independently loaded
		// games, the same reason nothing here compares CardIDs directly
		// either.
		gProtector, wProtector := "", ""
		if gcard.ProtectingPlayer != engine.NoPlayer {
			gProtector = got.Player(gcard.ProtectingPlayer).Name
		}
		if wcard.ProtectingPlayer != engine.NoPlayer {
			wProtector = want.Player(wcard.ProtectingPlayer).Name
		}
		if gProtector != wProtector {
			t.Errorf("%s protector = %q, want %q", label, gProtector, wProtector)
		}
		compareCounters(t, label, gcard.Counters, wcard.Counters)
	}
}

func compareCounters(t *testing.T, label string, got, want engine.Counters) {
	t.Helper()

	seen := map[engine.CounterType]bool{}
	for _, k := range got.Kinds() {
		seen[k] = true
		if got.Count(k) != want.Count(k) {
			t.Errorf("%s: counters[%s] = %d, want %d", label, k, got.Count(k), want.Count(k))
		}
	}
	for _, k := range want.Kinds() {
		if !seen[k] {
			t.Errorf("%s: missing counters[%s], want %d", label, k, want.Count(k))
		}
	}
}

var (
	scenarioDBOnce sync.Once
	scenarioDBVal  *compile.DB
	scenarioDBErr  error
)

// scenarioDB loads the real corpus once per test binary run and shares it
// across every scenario -- 33,697 cards is too much to pay for per case.
func scenarioDB(t *testing.T) *compile.DB {
	t.Helper()
	scenarioDBOnce.Do(func() {
		root := scenarioRepoRoot(t)
		scenarioDBVal, scenarioDBErr = compile.LoadDB(
			filepath.Join(root, "forge-gui", "res", "cardsfolder"),
			filepath.Join(root, "forge-gui", "res", "lists", "TypeLists.txt"),
		)
	})
	if scenarioDBErr != nil {
		t.Fatalf("load corpus: %v", scenarioDBErr)
	}
	return scenarioDBVal
}

func scenarioRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed; cannot locate the repository root")
	}
	// crucible/internal/engine/<file> -> repository root, where forge-gui
	// lives (upstream, outside the Go module -- ADR-0001).
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

// crucibleModuleRoot is one level shallower than scenarioRepoRoot:
// testdata/scenarios is the Go module's own path (Plan Section 3.1), not
// upstream's, so it does not live where forge-gui does.
func crucibleModuleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed; cannot locate the module root")
	}
	// crucible/internal/engine/<file> -> crucible/.
	return filepath.Join(filepath.Dir(file), "..", "..")
}
