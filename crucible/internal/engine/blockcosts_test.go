package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// These tests cover block costs: Mode$ CantBlockUnless statics, paid after
// blockers are declared (CombatUtil.getBlockCost,
// PhaseHandler.java:689-697).

// blockCostGame is a two-player game on the real corpus in a's declare
// blockers step with a's attacker and b's Qal Sisma Behemoth.
func blockCostGame(t *testing.T) (g *engine.Game, a, b engine.PlayerID, attacker, behemoth engine.CardID) {
	t.Helper()
	g, a, b = newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, a, engine.DeclareAttackers)
	attacker = g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	behemoth = g.NewCard(corpusCard(t, "Qal Sisma Behemoth"), b, engine.Battlefield)
	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	declareAttackers(t, g, ac)
	return g, a, b, attacker, behemoth
}

// Qal Sisma Behemoth: the block stands once its controller pays {2}.
func TestABlockCostIsPaidAndTheBlockStands(t *testing.T) {
	t.Parallel()
	g, _, b, attacker, behemoth := blockCostGame(t)

	c := engine.NewScriptedController()
	c.QueueBlocks([]engine.Block{{Blocker: behemoth, Attacker: attacker}})
	c.QueueConfirmPayCost(true)
	payColorless(g, c, b, 2)
	got := declareBlockers(t, g, c)

	if len(got) != 1 || got[0].Blocker != behemoth {
		t.Fatalf("blocks = %v, want the Behemoth blocking", got)
	}
	if n := g.Player(b).ManaPool.Total(); n != 0 {
		t.Errorf("pool after the cost = %d, want 0", n)
	}
}

// Declining the cost removes that block and nothing else.
func TestDecliningABlockCostRemovesTheBlock(t *testing.T) {
	t.Parallel()
	g, _, _, attacker, behemoth := blockCostGame(t)

	c := engine.NewScriptedController()
	c.QueueBlocks([]engine.Block{{Blocker: behemoth, Attacker: attacker}})
	c.QueueConfirmPayCost(false)
	got := declareBlockers(t, g, c)

	if len(got) != 0 {
		t.Fatalf("blocks = %v, want none", got)
	}
	if blocks := g.Blocks(); len(blocks) != 0 {
		t.Errorf("Blocks() = %v, want none", blocks)
	}
}
