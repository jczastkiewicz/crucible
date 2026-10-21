package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Mode$ AttackRequirement (Viashino Bey): when the host attacks, every other
// creature its controller has must attack too if able. The forced creatures
// carry no requirement of their own, so only the host attacking alone leaves
// one unmet (AttackRequirement.java:84-115).
func TestAttackRequirementForcesOthersWhenTheHostAttacks(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		pick  func(bey, other engine.CardID) []engine.CardID
		legal bool
	}{
		{"host alone", func(bey, _ engine.CardID) []engine.CardID { return []engine.CardID{bey} }, false},
		{"host and the other", func(bey, other engine.CardID) []engine.CardID { return []engine.CardID{bey, other} }, true},
		{"the other alone", func(_, other engine.CardID) []engine.CardID { return []engine.CardID{other} }, true},
		{"nobody", func(_, _ engine.CardID) []engine.CardID { return nil }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, a, _ := combatGame(t)
			bey := g.NewCard(combatCreatureDef(t, "Bey",
				[]string{"Mode$ AttackRequirement | ValidCard$ Card.Self | ValidAttacker$ Creature.YouCtrl"}, nil), a, engine.Battlefield)
			other := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
			ac := engine.NewScriptedController()
			ac.QueueAttackers(tc.pick(bey, other))
			_, err := g.DeclareCombatAttackers(ac)
			if tc.legal && err != nil {
				t.Fatalf("err = %v, want a legal declaration", err)
			}
			if !tc.legal {
				wantIllegal(t, err, "CR 508.1d")
			}
		})
	}
}

// A forced creature that cannot attack (it is tapped) is no violation.
func TestAttackRequirementIgnoresAForcedCreatureThatCannotAttack(t *testing.T) {
	t.Parallel()

	g, a, _ := combatGame(t)
	bey := g.NewCard(combatCreatureDef(t, "Bey",
		[]string{"Mode$ AttackRequirement | ValidCard$ Card.Self | ValidAttacker$ Creature.YouCtrl"}, nil), a, engine.Battlefield)
	other := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	g.Card(other).Tapped = true
	declareAttacking(t, g, bey)
}

// Mode$ PlayerMustAttack (Seeker of Slaanesh): the active player must attack
// one of the named defenders if any creature can; nobody attacking leaves the
// static unmet, whoever is able.
func TestPlayerMustAttackWantsAnAttackOnANamedDefender(t *testing.T) {
	t.Parallel()

	g, a, b := combatGame(t)
	g.NewCard(combatCreatureDef(t, "Seeker",
		[]string{"Mode$ PlayerMustAttack | ValidPlayer$ Opponent"}, nil), b, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers(nil)
	_, err := g.DeclareCombatAttackers(ac)
	wantIllegal(t, err, "CR 508.1d")

	declareAttacking(t, g, attacker)
}

// MustAttack$ narrows the defenders: a static naming only its controller is
// unmet by an attack on someone else, and met by none of them attacking when
// nothing can reach that defender (the same +1 on both sides cancels).
func TestPlayerMustAttackNamedDefenderOnly(t *testing.T) {
	t.Parallel()

	g, a, b := combatGame(t)
	g.NewCard(combatCreatureDef(t, "Trove",
		[]string{"Mode$ PlayerMustAttack | ValidPlayer$ Opponent | MustAttack$ You"}, nil), b, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	declareAttacking(t, g, attacker)
}
