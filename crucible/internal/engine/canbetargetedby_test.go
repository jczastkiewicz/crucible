package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// featherChoices records the creatures a ChooseCard offers and
// chooses none.
type featherChoices struct {
	*engine.ScriptedController
	offered []engine.CardID
}

func (f *featherChoices) ChooseCardsForEffect(_ *engine.Game, _ engine.PlayerID, _ engine.CardID, options []engine.CardID, _, _ int) []engine.CardID {
	f.offered = slices.Clone(options)
	return nil
}

// Feather offers the other creatures the triggering spell could target
// (CanBeTargetedByTriggeredSpellAbility, CardProperty.java:544-552): not an
// opponent's hexproof one.
func TestCanBeTargetedByTheTriggeringSpell(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	feather := g.NewCard(corpusCard(t, "Feather, Radiant Arbiter"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	ogre := g.NewCard(corpusCard(t, "Gray Ogre"), other, engine.Battlefield)
	scout := g.NewCard(corpusCard(t, "Gladecover Scout"), other, engine.Battlefield)
	growth := g.NewCard(corpusCard(t, "Giant Growth"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	c := &featherChoices{ScriptedController: engine.NewScriptedController()}
	c.QueueTargets([]engine.EntityID{engine.CardEntity(feather)})
	c.QueueConfirmPayCost(false) // no copy: Feather's {2} per chosen creature
	if !g.CastSpell(p, growth, c) {
		t.Fatalf("Giant Growth: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending: %v", err)
	}
	if !slices.Contains(c.offered, bears) || !slices.Contains(c.offered, ogre) ||
		slices.Contains(c.offered, scout) || slices.Contains(c.offered, feather) {
		t.Errorf("offered %v; want Bears %d and Ogre %d, not Scout %d or Feather %d", c.offered, bears, ogre, scout, feather)
	}
}
