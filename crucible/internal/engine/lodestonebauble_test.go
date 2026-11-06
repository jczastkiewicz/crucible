package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Lodestone Bauble: the delayed upkeep trigger remembers the targeted cards'
// owner (RememberObjects$ TargetedOwner & ChosenPlayer), and its Draw reads
// that player back through Defined$ DelayTriggerRemembered
// (AbilityUtils.getDefinedPlayers' addPlayer over the trigger's remembered
// objects): the graveyard's owner draws the basic land put on top.
func TestLodestoneBaubleOwnerDrawsAtTheNextUpkeep(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	libraryCards(t, g, p, 5)
	libraryCards(t, g, other, 5)
	bauble := g.NewCard(corpusCard(t, "Lodestone Bauble"), p, engine.Battlefield)
	forest := g.NewCard(corpusCard(t, "Forest"), other, engine.Graveyard)
	g.Player(p).ManaPool.AddColorless(1)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(forest)})
	c.QueuePayGeneric(mana.ShardC)
	if !g.ActivateAbility(p, bauble, 0, c) {
		t.Fatalf("Lodestone Bauble not activated: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Card(forest).Zone; got != engine.Library {
		t.Fatalf("Forest in %v, want Library", got)
	}
	advanceUntil(t, g, c, func() bool { return g.Turn() == 2 && g.ActivePhase() == engine.Upkeep })
	if got := g.Card(forest).Zone; got != engine.Hand {
		t.Errorf("Forest in %v at turn 2's upkeep, want drawn into its owner's hand by the delayed trigger", got)
	}
}
