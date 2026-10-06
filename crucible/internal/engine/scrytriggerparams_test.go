package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// xLifeWatcherDef is an enchantment with one trigger whose Execute$ gains its
// controller life equal to X, with X the given SVar body ("1" for a flat 1).
func xLifeWatcherDef(t *testing.T, line, x string) *compile.Card {
	t.Helper()
	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Life Watcher"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Life Watcher"
	raw.Faces[0].Type = cardtype.Parse(reg, "Enchantment")
	raw.Faces[0].Triggers = []string{line + " | Execute$ TrigLife"}
	raw.Faces[0].SVars.Set("TrigLife", "DB$ GainLife | Defined$ You | LifeAmount$ X")
	raw.Faces[0].SVars.Set("X", x)
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// xLifeWatcherGame is a two-player game where p holds a life watcher and a
// library of libSize cards.
func xLifeWatcherGame(t *testing.T, line, x string, libSize int) (*engine.Game, engine.PlayerID, engine.PlayerID) {
	t.Helper()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.NewCard(xLifeWatcherDef(t, line, x), p, engine.Battlefield)
	for range libSize {
		g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	}
	g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Library)
	return g, p, other
}

// Scry's ToBottom$ (TriggerScry.java:58-64): the trigger fires only when the
// scry put at least one card on the bottom.
func TestScryTriggerToBottomNeedsABottomedCard(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		bottomed   bool
		wantLife   int
		scryAmount string
	}{{"one card to the bottom", true, 21, "1"}, {"all cards kept on top", false, 20, "1"}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := xLifeWatcherGame(t, "Mode$ Scry | ValidPlayer$ You | ToBottom$ True | TriggerZones$ Battlefield", "1", 3)
			c := engine.NewScriptedController()
			if tc.bottomed {
				c.QueueScry(nil, []engine.CardID{topOf(g, p)})
			} else {
				c.QueueScry([]engine.CardID{topOf(g, p)}, nil)
			}
			def := etbChainDef(t, "Test Scryer", "DB$ Scry | ScryNum$ "+tc.scryAmount)
			if _, err := castETBChain(t, g, p, def, c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := g.Player(p).Life; got != tc.wantLife {
				t.Errorf("life = %d, want %d", got, tc.wantLife)
			}
		})
	}
}

// TriggerCount$ScryNum and ScryBottom (Elvish Mariner's and the exiling
// watcher's X): ScryNum counts every card put on top or bottom, ScryBottom
// only the bottomed ones.
func TestScryTriggerCountsScryNumAndScryBottom(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		key  string
		want int
	}{{"ScryNum", 3 + 20}, {"ScryBottom", 1 + 20}} {
		t.Run(tc.key, func(t *testing.T) {
			t.Parallel()
			g, p, _ := xLifeWatcherGame(t, "Mode$ Scry | ValidPlayer$ You | TriggerZones$ Battlefield",
				"TriggerCount$"+tc.key, 5)
			lib := g.Zone(engine.Library, p).Cards()
			c := engine.NewScriptedController()
			c.QueueScry([]engine.CardID{lib[0], lib[1]}, []engine.CardID{lib[2]})
			def := etbChainDef(t, "Test Scryer", "DB$ Scry | ScryNum$ 3")
			if _, err := castETBChain(t, g, p, def, c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := g.Player(p).Life; got != tc.want {
				t.Errorf("life = %d, want %d", got, tc.want)
			}
		})
	}
}

// GameAction.scry keeps a player whose library is empty (the amount is above
// 0), so the Scry trigger still fires, with ScryNum 0 (GameAction.java:2605,
// :2652-2655).
func TestScryOnAnEmptyLibraryStillFiresTheTrigger(t *testing.T) {
	t.Parallel()

	g, p, _ := xLifeWatcherGame(t, "Mode$ Scry | ValidPlayer$ You | TriggerZones$ Battlefield", "1", 0)
	def := etbChainDef(t, "Test Scryer", "DB$ Scry | ScryNum$ 2")
	if _, err := castETBChain(t, g, p, def, engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 21 {
		t.Errorf("life = %d, want 21 (the trigger fires on an empty library)", got)
	}
}

// Surveil's FirstTime$ (TriggerSurveil.java:62-66, Player.java:1084): only
// the player's first surveil of the turn fires it, and an empty library still
// counts as a surveil.
func TestSurveilFirstTimeFiresOncePerTurn(t *testing.T) {
	t.Parallel()

	g, p, _ := xLifeWatcherGame(t, "Mode$ Surveil | ValidPlayer$ You | FirstTime$ True | TriggerZones$ Battlefield", "1", 0)
	c := engine.NewScriptedController()
	for i, want := range []int{21, 21} {
		def := etbChainDef(t, "Test Surveiler", "DB$ Surveil | Amount$ 1")
		if _, err := castETBChain(t, g, p, def, c); err != nil {
			t.Fatalf("surveil %d: ResolveStack: %v", i+1, err)
		}
		if got := g.Player(p).Life; got != want {
			t.Errorf("after surveil %d life = %d, want %d", i+1, got, want)
		}
	}
}

// TurnFaceUp's ValidCause$ (TriggerTurnFaceUp.java:56-58, 2 real lines):
// the ability that turned the permanent up is matched as a SpellAbility.
func TestTurnFaceUpTriggerRespectsValidCause(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		cause string
		want  int
	}{{"cause you control", "SpellAbility.YouCtrl", 21}, {"cause an opponent controls", "SpellAbility.OppCtrl", 20}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := xLifeWatcherGame(t,
				"Mode$ TurnFaceUp | ValidCard$ Permanent | ValidCause$ "+tc.cause+" | TriggerZones$ Battlefield", "1", 1)
			def := etbChainDef(t, "Test Manifester",
				"DB$ Manifest | RememberManifested$ True | SubAbility$ DBUp",
				"DBUp", "DB$ SetState | Mode$ TurnFaceUp | Defined$ Remembered")
			if _, err := castETBChain(t, g, p, def, engine.NewScriptedController()); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := g.Player(p).Life; got != tc.want {
				t.Errorf("life = %d, want %d", got, tc.want)
			}
		})
	}
}
