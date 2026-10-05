package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// curseOfVitalityGame is a three-player game on the real corpus: the
// active player attacks the player the real Curse of Vitality (controlled by
// curseOwner) enchants, with one creature. Everyone starts at 20 life.
func curseOfVitalityGame(t *testing.T, curseOwner int) (g *engine.Game, players []engine.PlayerID) {
	t.Helper()
	g = engine.NewGame(scenarioDB(t), javarand.New(1), []string{"a", "b", "c"})
	players = g.Players()
	a, c := players[0], players[2]
	g.SetTurnState(1, a, engine.Main1)
	for _, p := range players {
		g.Player(p).Life = 20
	}
	curse := g.NewCard(corpusCard(t, "Curse of Vitality"), players[curseOwner], engine.Battlefield)
	g.AttachToPlayer(curse, c)
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	ac.QueueAttackTarget(engine.PlayerEntity(c))
	declareAttackers(t, g, ac)
	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	return g, players
}

// Defined$ TriggeredAttackingPlayer.Opponent+controlsCreature.attacking
// Player.EnchantedBy (Curse of Vitality, AbilityUtils.java:1186-1196): the
// curse's controller gains 2 for the attack, and so does the attacking
// opponent, who controls a creature attacking the enchanted player.
func TestTriggeredAttackingPlayerFilterGivesTheAttackingOpponentLife(t *testing.T) {
	t.Parallel()

	g, p := curseOfVitalityGame(t, 1)
	if got := g.Player(p[1]).Life; got != 22 {
		t.Errorf("curse controller life = %d, want 22", got)
	}
	if got := g.Player(p[0]).Life; got != 22 {
		t.Errorf("attacking opponent life = %d, want 22 (Opponent+controlsCreature.attacking Player.EnchantedBy)", got)
	}
	if got := g.Player(p[2]).Life; got != 20 {
		t.Errorf("enchanted player life = %d, want 20", got)
	}
}

// The filter's Opponent clause is relative to the ability's controller: when
// the curse's own controller is the attacker, "each opponent attacking that
// player" is nobody, so the player gains the 2 from the first half alone.
func TestTriggeredAttackingPlayerFilterExcludesTheCurseControllerItself(t *testing.T) {
	t.Parallel()

	g, p := curseOfVitalityGame(t, 0)
	if got := g.Player(p[0]).Life; got != 22 {
		t.Errorf("curse controller/attacker life = %d, want 22 (gained once, not as its own opponent)", got)
	}
	if got := g.Player(p[1]).Life; got != 20 {
		t.Errorf("bystander life = %d, want 20", got)
	}
}
