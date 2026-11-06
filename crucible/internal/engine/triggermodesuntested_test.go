package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Mode$ Fight (TriggerFight): Foe-Razer Regent sees a creature its controller
// controls fight and puts two +1/+1 counters on it at the next end step
// (DelayTriggerRememberedLKI of TriggeredFighterLKICopy).
func TestFightTriggerMarksTheFighterForTheEndStep(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Foe-Razer Regent"), p, engine.Battlefield)
	wall := g.NewCard(creatureDefPT(t, "0", "4"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	fighter, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(wall)}, "DB$ Fight | Defined$ Self | ValidTgts$ Creature.YouDontCtrl")
	if err != nil {
		t.Fatal(err)
	}
	if got := g.Card(fighter).Counters.Count(engine.P1P1); got != 0 {
		t.Fatalf("%d counters right after the fight, want them at the end step", got)
	}
	toStep(t, g, c, p, engine.EndOfTurn, nil)
	if got := g.Card(fighter).Counters.Count(engine.P1P1); got != 2 {
		t.Errorf("fighter has %d +1/+1 counters at the end step, want 2", got)
	}
	if got := g.Card(wall).Counters.Count(engine.P1P1); got != 0 {
		t.Errorf("the opponent's creature has %d counters, want 0 (ValidCard$ Creature.YouCtrl)", got)
	}
}

// Mode$ Mentored (TriggerMentored): Aegis of the Legion grants mentor, and
// when the equipped creature mentors a creature that creature also gets a
// shield counter.
func TestMentoredTriggerFollowsTheMentorCounter(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	aegis := g.NewCard(corpusCard(t, "Aegis of the Legion"), p, engine.Battlefield)
	mentor := g.NewCard(creatureDefPT(t, "3", "3"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	g.Attach(aegis, mentor)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	for _, id := range []engine.CardID{mentor, bears} {
		g.Card(id).SummonSick = false
	}
	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{mentor, bears})
	ac.QueueTargets([]engine.EntityID{engine.CardEntity(bears)})
	if _, err := g.DeclareCombatAttackers(ac); err != nil {
		t.Fatalf("DeclareCombatAttackers: %v", err)
	}
	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(bears).Counters.Count(engine.P1P1); got != 1 {
		t.Errorf("Bears have %d +1/+1 counters, want 1 from mentor", got)
	}
	if got := g.Card(bears).Counters.Count("SHIELD"); got != 1 {
		t.Errorf("Bears have %d shield counters, want 1 from the Mentored trigger", got)
	}
}

// Mode$ Trains (TriggerTrains): Savior of Ollenbock training (attacking beside
// a creature with greater power) exiles up to one other target creature.
func TestTrainsTriggerFiresWhenTheCreatureTrains(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	savior := g.NewCard(corpusCard(t, "Savior of Ollenbock"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	victim := g.NewCard(creatureDefPT(t, "4", "4"), other, engine.Battlefield)
	for _, id := range []engine.CardID{savior, bears} {
		g.Card(id).SummonSick = false
	}
	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{savior, bears})
	ac.QueueTargets([]engine.EntityID{engine.CardEntity(victim)})
	if _, err := g.DeclareCombatAttackers(ac); err != nil {
		t.Fatalf("DeclareCombatAttackers: %v", err)
	}
	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(savior).Counters.Count(engine.P1P1); got != 1 {
		t.Errorf("Savior has %d +1/+1 counters, want 1 from training", got)
	}
	if got := g.Card(victim).Zone; got != engine.Exile {
		t.Errorf("the 4/4 in %v, want exiled by the Trains trigger", got)
	}
}
