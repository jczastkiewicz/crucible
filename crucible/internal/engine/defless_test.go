package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// A permanent with no definition (an engine-made placeholder) sits in every
// trigger and static scan; each scan skips it instead of dereferencing the
// missing script (GO-7: one bad object must not stop the game).
func TestDefinitionlessPermanentsAreSkippedByEveryScan(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	g.SetTurnState(1, a, engine.Main1)
	g.NewCard(nil, a, engine.Battlefield)
	g.NewCard(nil, b, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)
	spell := g.NewCard(creatureDefCost(t, "Spell", "G"), a, engine.Hand)
	discarded := g.NewCard(creatureDefPT(t, "1", "1"), a, engine.Hand)
	g.NewCard(creatureDefPT(t, "1", "1"), a, engine.Library)

	g.Player(a).ManaPool.Add(mana.Green, 1)
	if !g.CastSpell(a, spell, engine.NewScriptedController()) {
		t.Fatal("CastSpell failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	g.Move(discarded, engine.Graveyard, a)
	g.DrawCards(a, 1, engine.NewScriptedController())
	g.Card(attacker).Tapped = false

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	declareAttackers(t, g, ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	declareBlockers(t, g, bc)
	g.DealCombatDamage(engine.NewScriptedController())
}

func TestDefinitionlessObjectsAreSkippedWhileEffectsRun(t *testing.T) {
	t.Parallel()

	g, a, b := newTwoPlayerGame(t)
	g.NewCard(nil, a, engine.Battlefield)
	g.NewCard(nil, b, engine.Battlefield)
	g.NewCard(nil, a, engine.Graveyard)
	g.NewCard(nil, a, engine.Hand)
	for i := 0; i < 8; i++ {
		g.NewCard(creatureDefPT(t, "1", "1"), a, engine.Library)
		g.NewCard(creatureDefPT(t, "1", "1"), a, engine.Hand)
	}
	for _, line := range []string{
		"DB$ Tap | Defined$ Self",
		"DB$ Untap | Defined$ Self",
		"DB$ PutCounter | Defined$ Self | CounterType$ P1P1 | CounterNum$ 1",
		"DB$ DealDamage | Defined$ You | NumDmg$ 1",
		"DB$ LoseLife | Defined$ You | LifeAmount$ 1",
		"DB$ GainLife | Defined$ You | LifeAmount$ 1",
		"DB$ Discard | Defined$ You | NumCards$ 1 | Mode$ Random",
		"DB$ Draw | Defined$ You | NumCards$ 1",
		"DB$ Mill | Defined$ You | NumCards$ 1",
		"DB$ Phases | Defined$ Self",
		"DB$ Destroy | Defined$ Self",
		"DB$ Sacrifice | Defined$ You | SacValid$ Self",
		"DB$ ChangeZone | Origin$ Battlefield | Destination$ Exile | Defined$ Self",
		"DB$ GainControl | Defined$ Self | NewController$ Player.Opponent",
	} {
		if _, err := castETBChain(t, g, a, etbChainDef(t, "Test Defless Battery", line), engine.NewScriptedController()); err != nil {
			t.Logf("%q: %v", line, err)
		}
	}
}
