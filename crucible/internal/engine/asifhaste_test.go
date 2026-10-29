package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Mode$ CanAttackIfHaste | ValidTarget$ Opponent,Planeswalker.OppCtrl
// (Frenzied Saddlebrute): a creature that came under its controller's
// control this turn may attack an opponent (CombatUtil.isAttackerSick,
// StaticAbilityCantAttackBlock.java:400-428).
func TestCanAttackIfHasteLetsASickCreatureAttack(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		static string
		want   bool
	}{{"", false}, {"Frenzied Saddlebrute", true}} {
		t.Run("with "+tc.static, func(t *testing.T) {
			t.Parallel()
			g, a, _ := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, a, engine.DeclareAttackers)
			if tc.static != "" {
				g.NewCard(corpusCard(t, tc.static), a, engine.Battlefield)
			}
			bears := g.NewCard(corpusCard(t, "Grizzly Bears"), a, engine.Battlefield)
			g.Card(bears).SummonSick = true
			c := engine.NewScriptedController()
			c.QueueAttackers([]engine.CardID{bears})

			got, err := g.DeclareCombatAttackers(c)
			if attacked := err == nil && len(got) == 1; attacked != tc.want {
				t.Errorf("attacked = %v (err %v), want %v", attacked, err, tc.want)
			}
		})
	}
}

// Mode$ ActivateAbilityAsIfHaste | ValidCard$ Creature.YouCtrl (Tyvar,
// Jubilant Brawler): a summoning-sick Prodigal Sorcerer may use its {T}
// ability (Card.isAbilitySick, StaticAbilityActivateAbilityAsIfHaste.java).
func TestActivateAbilityAsIfHasteIgnoresSummoningSickness(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		static string
		want   bool
	}{{"", false}, {"Tyvar, Jubilant Brawler", true}} {
		t.Run("with "+tc.static, func(t *testing.T) {
			t.Parallel()
			g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, p, engine.Main1)
			if tc.static != "" {
				g.NewCard(corpusCard(t, tc.static), p, engine.Battlefield)
			}
			sorcerer := g.NewCard(corpusCard(t, "Prodigal Sorcerer"), p, engine.Battlefield)
			g.Card(sorcerer).SummonSick = true
			c := engine.NewScriptedController()
			c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})

			if got := g.ActivateAbility(p, sorcerer, 0, c); got != tc.want {
				t.Errorf("ActivateAbility = %v, want %v", got, tc.want)
			}
		})
	}
}
