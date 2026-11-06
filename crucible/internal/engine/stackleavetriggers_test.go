package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// A spell an effect moves off the stack to its owner's hand (Remand's
// ChangeZone) fires the Origin$ Stack | Destination$ Any lines, as a spell
// put into the graveyard does (Goblin Artisans' and Geth's Summons' cleanups).
func TestSpellReturnedFromTheStackFiresStackOriginTriggers(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(scriptDef(t, "Stack Watcher", "Enchantment",
		"T:Mode$ ChangesZone | Origin$ Stack | Destination$ Any | ValidCard$ Card | TriggerZones$ Battlefield | Execute$ TrigGain",
		"SVar:TrigGain:DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardG)
	if !g.CastSpell(p, bears, c) {
		t.Fatalf("Grizzly Bears not cast: %v", g.TakePendingError())
	}
	life := g.Player(p).Life
	if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(bears)},
		"DB$ ChangeZone | ValidTgts$ Card | TgtZone$ Stack | Origin$ Stack | Fizzle$ True | Destination$ Hand"); err != nil {
		t.Fatal(err)
	}
	if got := g.Card(bears).Zone; got != engine.Hand {
		t.Fatalf("Grizzly Bears in %v, want Hand", got)
	}
	if got := g.Player(p).Life - life; got != 1 {
		t.Errorf("gained %d life, want the Origin$ Stack trigger's 1", got)
	}
}
