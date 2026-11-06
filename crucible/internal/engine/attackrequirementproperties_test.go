package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// hadToAttackThisCombat (CardProperty.java:1693-1697) names the creatures
// that had an attack requirement as attackers were declared, and
// couldAttackButNotAttacking (CombatUtil.java:95-119) the active player's
// creatures left home that could have attacked with no more requirements
// broken than the best attack.
func TestAttackRequirementPropertiesReadTheDeclaration(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(2, p, engine.DeclareAttackers)
	forced := g.NewCard(combatCreatureDef(t, "Test Cyclops", []string{"Mode$ MustAttack | ValidCreature$ Card.Self"}, nil), p, engine.Battlefield)
	attacker := g.NewCard(corpusCard(t, "Hill Giant"), p, engine.Battlefield)
	home := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	sick := g.NewCard(corpusCard(t, "Gray Ogre"), p, engine.Battlefield)
	defender := g.NewCard(corpusCard(t, "Gray Ogre"), other, engine.Battlefield)
	for _, id := range []engine.CardID{forced, attacker, home, defender} {
		g.Card(id).SummonSick = false
	}
	g.Card(sick).SummonSick = true
	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{forced, attacker})
	declareAttackers(t, g, ac)

	for _, tc := range []struct {
		spec string
		card engine.CardID
		want bool
	}{
		{"Creature.hadToAttackThisCombat", forced, true},
		{"Creature.hadToAttackThisCombat", attacker, false},
		{"Creature.couldAttackButNotAttacking", home, true},
		{"Creature.couldAttackButNotAttacking", attacker, false},
		{"Creature.couldAttackButNotAttacking", sick, false},
		{"Creature.couldAttackButNotAttacking", defender, false},
	} {
		if got := engine.Matches(g, g.Card(tc.card), valid.Parse(tc.spec), p, engine.NoCard); got != tc.want {
			t.Errorf("%s on %s = %v, want %v", tc.spec, g.Card(tc.card).Name(), got, tc.want)
		}
		if err := g.TakePendingError(); err != nil {
			t.Errorf("%s: %v", tc.spec, err)
		}
	}

	// A new object is not the one that had to attack.
	g.Move(forced, engine.Hand, p)
	g.Move(forced, engine.Battlefield, p)
	if engine.Matches(g, g.Card(forced), valid.Parse("Creature.hadToAttackThisCombat"), p, engine.NoCard) {
		t.Error("hadToAttackThisCombat survived a zone change")
	}
}
