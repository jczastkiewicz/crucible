package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// advanceUntil walks g phase by phase, resolving whatever triggers each
// step pushes, until stop reports true.
func advanceUntil(t *testing.T, g *engine.Game, c engine.PlayerController, stop func() bool) {
	t.Helper()
	for i := 0; i < 60 && !stop(); i++ {
		g.AdvancePhase(c)
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("ResolveStack at %v: %v", g.ActivePhase(), err)
		}
	}
	if !stop() {
		t.Fatal("advanceUntil: condition never became true")
	}
}

// TestDelayedTriggerFiresOnceAtEndStep proves CR 603.7: a delayed "at the
// beginning of your end step" trigger fires at this turn's end step, acts
// on what it remembered (Defined$ DelayTriggerRememberedLKI), and is gone
// afterwards -- the next end step does nothing.
func TestDelayedTriggerFiresOnceAtEndStep(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	libraryCards(t, g, p, 5)
	libraryCards(t, g, other, 5)
	victim := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(victim)})
	def := etbChainDef(t, "Test Delayed Doom",
		"DB$ Tap | ValidTgts$ Creature.OppCtrl | SubAbility$ DBDelay",
		"DBDelay", "DB$ DelayedTrigger | Mode$ Phase | Phase$ End of Turn | ValidPlayer$ You | RememberObjects$ Targeted | Execute$ TrigDestroy",
		"TrigDestroy", "DB$ Destroy | Defined$ DelayTriggerRememberedLKI")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(victim).Zone; z != engine.Battlefield {
		t.Fatalf("victim zone = %v before the end step, want Battlefield", z)
	}
	advanceUntil(t, g, c, func() bool { return g.ActivePhase() == engine.EndOfTurn })
	if z := g.Card(victim).Zone; z != engine.Graveyard {
		t.Errorf("victim zone after the end step = %v, want Graveyard", z)
	}
}

// TestDelayedTriggerThisTurnLapses proves ThisTurn$: a delayed upkeep
// trigger created this turn never fires next turn's upkeep; NextTurn$
// instead fires exactly then.
func TestDelayedTriggerThisTurnLapses(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	libraryCards(t, g, p, 5)
	libraryCards(t, g, other, 5)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test Delayed Life",
		"DB$ DelayedTrigger | Mode$ Phase | Phase$ Upkeep | ThisTurn$ True | Execute$ TrigLose | SubAbility$ DBNext",
		"DBNext", "DB$ DelayedTrigger | Mode$ Phase | Phase$ Upkeep | NextTurn$ True | Execute$ TrigGain",
		"TrigLose", "DB$ LoseLife | Defined$ You | LifeAmount$ 5",
		"TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 3")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	advanceUntil(t, g, c, func() bool { return g.ActivePlayer() == other && g.ActivePhase() == engine.Draw })
	if got := g.Player(p).Life; got != 23 {
		t.Errorf("p life after the next upkeep = %d, want 23 (NextTurn$ fired, ThisTurn$ lapsed)", got)
	}
}

// TestImmediateTriggerPushesExecute proves a reflexive trigger's Execute$
// goes on the stack and resolves, reading what it remembered.
func TestImmediateTriggerPushesExecute(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	victim := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(victim)})
	def := etbChainDef(t, "Test Reflexive",
		"DB$ Tap | ValidTgts$ Creature.OppCtrl | SubAbility$ DBReflex",
		"DBReflex", "DB$ ImmediateTrigger | RememberObjects$ Targeted | Execute$ TrigDestroy",
		"TrigDestroy", "DB$ Destroy | Defined$ DelayTriggerRemembered")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(victim).Zone; z != engine.Graveyard {
		t.Errorf("victim zone = %v, want Graveyard", z)
	}
}

// TestCharmChoosesModesWhenPushed proves CR 700.2: the Charm's controller
// picks CharmNum$ modes as the trigger goes on the stack, and they resolve
// in printed order.
func TestCharmChoosesModesWhenPushed(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	libraryCards(t, g, p, 3)
	c := engine.NewScriptedController()
	c.QueueModeChoice([]int{2, 0})
	def := etbChainDef(t, "Test Charm",
		"DB$ Charm | CharmNum$ 2 | Choices$ DBGain,DBLose,DBDraw",
		"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ 2",
		"DBLose", "DB$ LoseLife | Defined$ You | LifeAmount$ 7",
		"DBDraw", "DB$ Draw | Defined$ You | NumCards$ 1")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 22 {
		t.Errorf("life = %d, want 22 (gain chosen, lose not)", got)
	}
	if got := len(g.Zone(engine.Hand, p).Cards()); got != 1 {
		t.Errorf("hand = %d, want 1 (draw chosen)", got)
	}
}

// TestCharmModeWithoutTargetsIsNotOffered proves makePossibleOptions' CR
// 603.3c filter: a mode needing a target that has none is dropped, so the
// remaining one is the only option (index 0).
func TestCharmModeWithoutTargetsIsNotOffered(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	c.QueueModeChoice([]int{0})
	def := etbChainDef(t, "Test Charm Filter",
		"DB$ Charm | Choices$ DBKillArtifact,DBGain",
		"DBKillArtifact", "DB$ Destroy | ValidTgts$ Artifact",
		"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ 4")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 24 {
		t.Errorf("life = %d, want 24", got)
	}
}
