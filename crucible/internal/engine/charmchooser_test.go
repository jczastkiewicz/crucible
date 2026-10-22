package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// TestCharmChooserPicksOpponentAndModeReadsChoosingPlayer proves Chooser$
// (CharmEffect.java:266-273): the activator names an opponent, the mode runs,
// and Defined$ ChoosingPlayer / ControlledBy ChoosingPlayer in the mode and
// its linked sub-ability read that opponent (Misfortune's shape).
func TestCharmChooserPicksOpponentAndModeReadsChoosingPlayer(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	mine := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	theirs := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	charm := g.NewCard(spellDefWith(t, "Test Misfortune", "Sorcery", "W", "SP$ Charm | Chooser$ Opponent | Choices$ Fortune,Misfortune",
		"Fortune", "DB$ GainLife | LifeAmount$ 4",
		"Misfortune", "DB$ PutCounterAll | ValidCards$ Creature.ControlledBy ChoosingPlayer | CounterType$ M1M1 | CounterNum$ 1 | SubAbility$ DBDamage",
		"DBDamage", "DB$ DealDamage | Defined$ ChoosingPlayer | NumDmg$ 4"), p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueuePlayerChoice(other)
	c.QueueModeChoice([]int{1})
	g.Player(p).ManaPool.Add(mana.White, 1)
	if !g.CastSpell(p, charm, c) {
		t.Fatal("CastSpell failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Player(other).Life; got != 16 {
		t.Errorf("chooser life = %d, want 16", got)
	}
	if got := g.Player(p).Life; got != 20 {
		t.Errorf("caster life = %d, want 20", got)
	}
	if pw, _ := g.Card(theirs).Power(); pw != 1 {
		t.Errorf("opponent's creature power = %d, want 1", pw)
	}
	if pw, _ := g.Card(mine).Power(); pw != 2 {
		t.Errorf("caster's creature power = %d, want 2", pw)
	}
}
