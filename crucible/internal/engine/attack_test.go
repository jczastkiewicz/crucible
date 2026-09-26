package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// A plain untapped, non-summoning-sick creature is eligible and, once
// declared, taps -- CR 508.1a/508.1f, the ordinary case.
func TestDeclareCombatAttackersTapsADeclaredAttacker(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.SetTurnState(1, a, engine.Main1)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueAttackers([]engine.CardID{creature})

	got := declareAttackers(t, g, c)

	if len(got) != 1 || got[0] != creature {
		t.Fatalf("DeclareCombatAttackers() = %v, want [%v]", got, creature)
	}
	if !g.Card(creature).Tapped {
		t.Error("declared attacker did not tap")
	}
	attackers := g.Attackers()
	if len(attackers) != 1 || attackers[0] != creature {
		t.Errorf("Attackers() = %v, want [%v]", attackers, creature)
	}
}

// Vigilance keeps a declared attacker untapped (CR 508.1f).
func TestDeclareCombatAttackersVigilanceStaysUntapped(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.SetTurnState(1, a, engine.Main1)
	creature := g.NewCard(creatureDefPTKeywords(t, "2", "2", "Vigilance"), a, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueAttackers([]engine.CardID{creature})
	declareAttackers(t, g, c)

	if g.Card(creature).Tapped {
		t.Error("a vigilance attacker tapped")
	}
}

// A summoning-sick creature is not eligible unless it has haste (CR 302.6,
// 508.1a).
func TestDeclareCombatAttackersSummoningSickIsIneligibleWithoutHaste(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.SetTurnState(1, a, engine.Main1)
	sick := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	g.Card(sick).SummonSick = true
	haste := g.NewCard(creatureDefPTKeywords(t, "2", "2", "Haste"), a, engine.Battlefield)
	g.Card(haste).SummonSick = true

	c := engine.NewScriptedController()
	c.QueueAttackers([]engine.CardID{haste})
	got := declareAttackers(t, g, c)

	if len(got) != 1 || got[0] != haste {
		t.Errorf("DeclareCombatAttackers() = %v, want [%v] (only the hasty one was ever offered)", got, haste)
	}
}

// A tapped creature is never eligible, haste or not.
func TestDeclareCombatAttackersTappedIsIneligible(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.SetTurnState(1, a, engine.Main1)
	tapped := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	g.Card(tapped).Tapped = true

	// No QueueAttackers call: if DeclareCombatAttackers asked anyway, this
	// panics on an empty queue, which is exactly the assertion -- nothing
	// eligible means nothing asked.
	c := engine.NewScriptedController()
	got := declareAttackers(t, g, c)

	if got != nil {
		t.Errorf("DeclareCombatAttackers() = %v, want nil", got)
	}
}

// A creature controlled by the non-active player is never offered, even if
// it would otherwise be eligible.
func TestDeclareCombatAttackersOnlyOffersActivePlayersCreatures(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)

	c := engine.NewScriptedController()
	got := declareAttackers(t, g, c)

	if got != nil {
		t.Errorf("DeclareCombatAttackers() = %v, want nil (the eligible creature belongs to the defending player)", got)
	}
}

// No eligible creature means the controller is never asked at all -- an
// empty queue must not panic.
func TestDeclareCombatAttackersAsksNoOneWhenNothingIsEligible(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.SetTurnState(1, a, engine.Main1)

	c := engine.NewScriptedController()
	got := declareAttackers(t, g, c)

	if got != nil {
		t.Errorf("DeclareCombatAttackers() = %v, want nil", got)
	}
}

// Declining to attack with anything is itself a legal, queueable answer.
func TestDeclareCombatAttackersCanDeclineWithEligibleCreatures(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.SetTurnState(1, a, engine.Main1)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueAttackers(nil)
	got := declareAttackers(t, g, c)

	if got != nil {
		t.Errorf("DeclareCombatAttackers() = %v, want nil", got)
	}
	if g.Card(creature).Tapped {
		t.Error("a creature that was not declared as an attacker tapped anyway")
	}
}

// A cloned game's combat state is its own slice: declaring on the clone
// must not write back to the original.
func TestCloneCopiesCombat(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.SetTurnState(1, a, engine.Main1)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)

	clone := g.Clone()
	c := engine.NewScriptedController()
	c.QueueAttackers([]engine.CardID{creature})
	declareAttackers(t, clone, c)

	if len(g.Attackers()) != 0 {
		t.Errorf("original Attackers() = %v after the clone's changed, want none", g.Attackers())
	}
	if len(clone.Attackers()) != 1 {
		t.Errorf("clone Attackers() = %v, want one", clone.Attackers())
	}
}

// creatureDefWithOptionalAttackCost builds a *compile.Card for a 2/2 Elf
// creature carrying one real S:Mode$ OptionalAttackCost line (CR 508.1c,
// "you may exert this as it attacks") -- the corpus's own real
// Cost$ Exert<1/CARDNAME> shape, with an optional payoff SVar for its own
// Trigger$ param, exercised through the real compiled param parser the
// same way creatureDefWithAbility already does for A:AB$ (activateability_test.go,
// TEST-1).
func creatureDefWithOptionalAttackCost(t *testing.T, name string, payoffSVar, payoffBody string) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "2", "2"
	staticLine := "Mode$ OptionalAttackCost | ValidCard$ Card.Self | Cost$ Exert<1/CARDNAME>"
	if payoffSVar != "" {
		staticLine += " | Trigger$ " + payoffSVar
		raw.Faces[0].SVars.Set(payoffSVar, payoffBody)
	}
	raw.Faces[0].Statics = []string{staticLine}

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// CR 508.1c: exerting a declared attacker sets Card.Exerted and runs its own
// OptionalAttackCost static ability's own Trigger$ payoff (Gust Walker's own
// shape: "it gets +1/+1 and gains flying until end of turn").
func TestDeclareCombatAttackersExertsAndRunsPayoff(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.SetTurnState(1, a, engine.Main1)
	g.Player(a).Life, g.Player(g.Players()[1]).Life = 20, 20
	def := creatureDefWithOptionalAttackCost(t, "Gust Walker", "TrigPump",
		"DB$ Pump | Defined$ Self | NumAtt$ +1 | NumDef$ +1 | KW$ Flying")
	creature := g.NewCard(def, a, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueAttackers([]engine.CardID{creature})
	c.QueueExertAttackers([]engine.CardID{creature})

	declareAttackers(t, g, c)

	if !g.Card(creature).Exerted {
		t.Error("chosen attacker did not become Exerted")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	pw, _ := g.Card(creature).Power()
	tg, _ := g.Card(creature).Toughness()
	if pw != 3 || tg != 3 {
		t.Errorf("Pump payoff PT = %d/%d, want 3/3 (base 2/2 +1/+1)", pw, tg)
	}
	if !g.Card(creature).HasKeyword("Flying") {
		t.Error("Pump payoff did not grant Flying")
	}
}

// Declining every offer is legal (CR 508.1c's own "may") -- the creature
// attacks normally, un-exerted, with no payoff.
func TestDeclareCombatAttackersDeclinesExert(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.SetTurnState(1, a, engine.Main1)
	def := creatureDefWithOptionalAttackCost(t, "Gust Walker", "TrigPump",
		"DB$ Pump | Defined$ Self | NumAtt$ +1 | NumDef$ +1 | KW$ Flying")
	creature := g.NewCard(def, a, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueAttackers([]engine.CardID{creature})
	c.QueueExertAttackers(nil)

	declareAttackers(t, g, c)

	if g.Card(creature).Exerted {
		t.Error("declined attacker became Exerted anyway")
	}
	if pw, _ := g.Card(creature).Power(); pw != 2 {
		t.Errorf("Pump payoff ran despite decline, Power = %d, want base 2", pw)
	}
}

// A creature carrying no OptionalAttackCost static is never offered at all
// -- ExertAttackers is not called (a ScriptedController with an empty queue
// would panic if it were).
func TestDeclareCombatAttackersSkipsExertOfferForOrdinaryCreature(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.SetTurnState(1, a, engine.Main1)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueAttackers([]engine.CardID{creature})

	declareAttackers(t, g, c)

	if g.Card(creature).Exerted {
		t.Error("an ordinary creature became Exerted")
	}
}

// A card whose own OptionalAttackCost names no Trigger$ at all (5 of the
// corpus's 28 real lines, resolute_survivors.txt's own shape) still exerts
// cleanly -- checkExertedTriggers alone covers whatever payoff exists
// elsewhere, and resolveOptionalAttackCostPayoff finds nothing to run.
func TestDeclareCombatAttackersExertsWithNoPayoff(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.SetTurnState(1, a, engine.Main1)
	def := creatureDefWithOptionalAttackCost(t, "Resolute Survivors", "", "")
	creature := g.NewCard(def, a, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueAttackers([]engine.CardID{creature})
	c.QueueExertAttackers([]engine.CardID{creature})

	declareAttackers(t, g, c)

	if !g.Card(creature).Exerted {
		t.Error("chosen attacker did not become Exerted")
	}
}
