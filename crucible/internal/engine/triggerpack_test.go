package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// modeWatcherDef is an enchantment whose one trigger is line and whose
// Execute$ draws a card for its controller.
func modeWatcherDef(t *testing.T, line string) *compile.Card {
	t.Helper()
	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Mode Watcher"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Mode Watcher"
	raw.Faces[0].Type = cardtype.Parse(reg, "Enchantment")
	raw.Faces[0].Triggers = []string{line + " | Execute$ TrigDraw"}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// watcherGame is a two-player game on the real corpus where a holds a
// watcher with the given trigger and a library of two cards.
func watcherGame(t *testing.T, line string) (*engine.Game, engine.PlayerID, engine.PlayerID) {
	t.Helper()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.NewCard(modeWatcherDef(t, line), p, engine.Battlefield)
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Library)
	return g, p, other
}

// topOf is the top card of pid's library.
func topOf(g *engine.Game, pid engine.PlayerID) engine.CardID {
	return g.Zone(engine.Library, pid).Cards()[0]
}

// Mode$ Scry (TriggerScry, 24 real lines): the controller's own scry fires a
// ValidPlayer$ You watcher; an opponent's scry does not.
func TestScryTriggerFiresForTheScryingPlayerOnly(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		opponent bool
		wantHand int
	}{{"own scry", false, 1}, {"opponent scry", true, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, other := watcherGame(t, "Mode$ Scry | ValidPlayer$ You | TriggerZones$ Battlefield")
			scryer := p
			if tc.opponent {
				scryer = other
			}
			def := etbChainDef(t, "Test Scryer", "DB$ Scry | ScryNum$ 1 | Defined$ Player.Opponent")
			if !tc.opponent {
				def = etbChainDef(t, "Test Scryer", "DB$ Scry | ScryNum$ 1")
			}
			c := engine.NewScriptedController()
			c.QueueScry([]engine.CardID{topOf(g, scryer)}, nil)
			if _, err := castETBChain(t, g, p, def, c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := g.Zone(engine.Hand, p).Len(); got != tc.wantHand {
				t.Errorf("hand after %v scried = %d, want %d", scryer, got, tc.wantHand)
			}
		})
	}
}

// ToBottom$ is a check this port does not evaluate: the line is not fired.
func TestScryTriggerWithToBottomIsNotFired(t *testing.T) {
	t.Parallel()

	g, p, _ := watcherGame(t, "Mode$ Scry | ValidPlayer$ You | ToBottom$ True | TriggerZones$ Battlefield")
	c := engine.NewScriptedController()
	c.QueueScry([]engine.CardID{topOf(g, p)}, nil)
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Scryer", "DB$ Scry | ScryNum$ 1"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Zone(engine.Hand, p).Len(); got != 0 {
		t.Errorf("hand = %d, want 0 (ToBottom$ not evaluated)", got)
	}
}

// Mode$ Surveil (TriggerSurveil, 16 real lines).
func TestSurveilTriggerFiresForTheSurveilingPlayer(t *testing.T) {
	t.Parallel()

	g, p, _ := watcherGame(t, "Mode$ Surveil | ValidPlayer$ You | TriggerZones$ Battlefield")
	c := engine.NewScriptedController()
	c.QueueSurveil([]engine.CardID{topOf(g, p)}, nil)
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Surveiler", "DB$ Surveil | Amount$ 1"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Zone(engine.Hand, p).Len(); got != 1 {
		t.Errorf("hand = %d, want 1", got)
	}
}

// Mode$ Transformed (TriggerTransformed, 35 real lines): the real Delver of
// Secrets transformed by a SetState effect fires a ValidCard$ Card watcher.
func TestTransformedTriggerFiresWhenAPermanentTransforms(t *testing.T) {
	t.Parallel()

	g, p, _ := watcherGame(t, "Mode$ Transformed | ValidCard$ Card | TriggerZones$ Battlefield")
	delver := g.NewCard(corpusCard(t, "Delver of Secrets"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(delver)})
	def := etbChainDef(t, "Test Transformer", "DB$ SetState | ValidTgts$ Creature | Mode$ Transform")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(delver).Def.Faces[0].Name; got == "Delver of Secrets" {
		t.Fatalf("Delver did not transform (still %q)", got)
	}
	if got := g.Zone(engine.Hand, p).Len(); got != 1 {
		t.Errorf("hand = %d, want 1 (the Transformed watcher drew)", got)
	}
}

// A line the ValidCard$ rejects does not fire.
func TestTransformedTriggerRespectsValidCard(t *testing.T) {
	t.Parallel()

	g, p, _ := watcherGame(t, "Mode$ Transformed | ValidCard$ Card.nonCreature | TriggerZones$ Battlefield")
	delver := g.NewCard(corpusCard(t, "Delver of Secrets"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(delver)})
	def := etbChainDef(t, "Test Transformer", "DB$ SetState | ValidTgts$ Creature | Mode$ Transform")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Zone(engine.Hand, p).Len(); got != 0 {
		t.Errorf("hand = %d, want 0 (a creature is not Card.nonCreature)", got)
	}
}

// Mode$ TurnFaceUp (TriggerTurnFaceUp, 126 real lines): a manifested creature
// card turned face up by SetState fires a ValidCard$ Card watcher.
func TestTurnFaceUpTriggerFiresWhenAManifestTurnsFaceUp(t *testing.T) {
	t.Parallel()

	g, p, _ := watcherGame(t, "Mode$ TurnFaceUp | ValidCard$ Card | TriggerZones$ Battlefield")
	g.NewCard(creatureDefPT(t, "3", "3"), p, engine.Library) // the card to manifest, on top
	def := etbChainDef(t, "Test Manifester",
		"DB$ Manifest | RememberManifested$ True | SubAbility$ DBUp",
		"DBUp", "DB$ SetState | Mode$ TurnFaceUp | Defined$ Remembered")
	if _, err := castETBChain(t, g, p, def, engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Zone(engine.Hand, p).Len(); got != 1 {
		t.Errorf("hand = %d, want 1 (the TurnFaceUp watcher drew)", got)
	}
}
