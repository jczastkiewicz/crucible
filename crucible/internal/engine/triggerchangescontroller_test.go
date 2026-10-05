package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// stealWatcher casts a creature for thief whose enters trigger gains control
// of the watcher, and resolves it.
func stealWatcher(t *testing.T, g *engine.Game, thief engine.PlayerID, watcher engine.CardID) {
	t.Helper()
	g.SetTurnState(1, thief, engine.Main1) // a creature spell needs its caster's own main phase
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(watcher)})
	def := etbChainDef(t, "Test Thief", "DB$ GainControl | ValidTgts$ Creature.OppCtrl | TgtPrompt$ Select a creature")
	if _, err := castETBChain(t, g, thief, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
}

// castDelayedWatcher casts a creature for p whose enters trigger is trigger,
// with a TrigDraw SVar drawing one card for its controller.
func castDelayedWatcher(t *testing.T, g *engine.Game, p engine.PlayerID, trigger string) engine.CardID {
	t.Helper()
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test Watcher", trigger, "TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")
	watcher, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	return watcher
}

// A delayed Mode$ ChangesController trigger (DelayedTriggerEffect, 4 real
// lines) fires once when its remembered card changes controller away from the
// trigger's controller, and is gone afterwards (TriggerHandler's
// runWaitingTrigger removes a delayed trigger it fires).
func TestDelayedChangesControllerFiresOnceWhenTheRememberedCardChangesController(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	watcher := castDelayedWatcher(t, g, p,
		"DB$ DelayedTrigger | Mode$ ChangesController | ValidCard$ Card.IsTriggerRemembered | ValidOriginalController$ You | RememberObjects$ Self | Execute$ TrigDraw")

	stealWatcher(t, g, other, watcher)
	if got := g.Zone(engine.Hand, p).Len(); got != 1 {
		t.Fatalf("hand after the steal = %d, want 1 (the delayed trigger's draw)", got)
	}

	// The trigger is spent: stealing the creature back draws nothing.
	stealWatcher(t, g, p, watcher)
	if got := g.Zone(engine.Hand, p).Len(); got != 1 {
		t.Errorf("hand after control returned = %d, want still 1 (the delayed trigger fires once)", got)
	}
}

// ValidOriginalController$ Opponent: the trigger's controller is not the
// player the card left, so the delayed trigger stays registered and does not fire.
func TestDelayedChangesControllerSkipsWhenOriginalControllerDoesNotMatch(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	watcher := castDelayedWatcher(t, g, p,
		"DB$ DelayedTrigger | Mode$ ChangesController | ValidCard$ Card.IsTriggerRemembered | ValidOriginalController$ Opponent | RememberObjects$ Self | Execute$ TrigDraw")

	stealWatcher(t, g, other, watcher)
	if got := g.Zone(engine.Hand, p).Len(); got != 0 {
		t.Errorf("hand = %d, want 0 (the card left the trigger controller, not an opponent)", got)
	}
}

// ValidCard$ Card.IsTriggerRemembered: another creature changing controller
// is not the remembered card.
func TestDelayedChangesControllerIgnoresACardItDidNotRemember(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	bystander := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	castDelayedWatcher(t, g, p,
		"DB$ DelayedTrigger | Mode$ ChangesController | ValidCard$ Card.IsTriggerRemembered | ValidOriginalController$ You | RememberObjects$ Self | Execute$ TrigDraw")

	stealWatcher(t, g, other, bystander)
	if got := g.Zone(engine.Hand, p).Len(); got != 0 {
		t.Errorf("hand = %d, want 0 (the stolen creature is not the remembered one)", got)
	}
}

// ValidCard$ Card.StrictlySelf names the DelayedTrigger's own host (Seraph's
// "when you lose control of Seraph"), a valid string, not the remembered card.
func TestDelayedChangesControllerStrictlySelfNamesTheHost(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	watcher := castDelayedWatcher(t, g, p,
		"DB$ DelayedTrigger | Mode$ ChangesController | ValidCard$ Card.StrictlySelf | ValidOriginalController$ You | RememberObjects$ Self | Execute$ TrigDraw")

	stealWatcher(t, g, other, watcher)
	if got := g.Zone(engine.Hand, p).Len(); got != 1 {
		t.Errorf("hand = %d, want 1 (the host changed controller)", got)
	}
}

// IsPresent$ (Stolen Uniform) reads Card.IsTriggerRemembered, which no
// evaluator resolves: registering the trigger fails closed (GO-7).
func TestDelayedChangesControllerRefusesIsPresent(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test Watcher",
		"DB$ DelayedTrigger | Mode$ ChangesController | ValidCard$ Card.IsTriggerRemembered | IsPresent$ Card.IsTriggerRemembered+AttachedTo Creature.YouCtrl | RememberObjects$ Self | Execute$ TrigDraw",
		"TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")
	if _, err := castETBChain(t, g, p, def, c); err == nil {
		t.Error("ResolveStack succeeded, want a not-resolvable error for IsPresent$")
	}
}
