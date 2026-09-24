package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// TestEachDamageEffectEverySourceHitsTarget proves the default shape: each
// ValidCards$ source deals NumDmg$ to each Defined$ target.
func TestEachDamageEffectEverySourceHitsTarget(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test EachDamage", "DB$ EachDamage | ValidCards$ Creature.YouCtrl | NumDmg$ 2 | Defined$ Opponent")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(other).Life; got != 16 {
		t.Errorf("opponent life = %d, want 16 (two sources x 2)", got)
	}
}

// TestEachDamageEffectToEachOther proves ToEachOther$: every named card
// damages every other one.
func TestEachDamageEffectToEachOther(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	a := g.NewCard(creatureDefPT(t, "3", "10"), other, engine.Battlefield)
	b := g.NewCard(creatureDefPT(t, "3", "10"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{a, b})
	def := etbChainDef(t, "Test Fight All",
		"DB$ ChooseCard | Choices$ Creature.OppCtrl | Amount$ 2 | Mandatory$ True | SubAbility$ DBEach",
		"DBEach", "DB$ EachDamage | NumDmg$ 1 | ToEachOther$ ChosenCard")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(a).Damage.Marked != 1 || g.Card(b).Damage.Marked != 1 {
		t.Errorf("damage = %d/%d, want 1/1", g.Card(a).Damage.Marked, g.Card(b).Damage.Marked)
	}
}

// TestHealDamageEffectRemovesDamage proves HealDamage: marked damage is
// removed before state-based actions would see it.
func TestHealDamageEffectRemovesDamage(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test Heal", "DB$ DealDamage | Defined$ Self | NumDmg$ 3 | SubAbility$ DBHeal",
		"DBHeal", "DB$ HealDamage | Defined$ Self")
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if card := g.Card(host); card.Zone != engine.Battlefield || card.Damage.Marked != 0 {
		t.Errorf("host zone %v damage %d, want Battlefield 0", card.Zone, card.Damage.Marked)
	}
}

// TestDrainManaEffectMovesPool proves DrainMana with DrainMana$: the
// target's pool empties into the activator's.
func TestDrainManaEffectMovesPool(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.Player(other).ManaPool.Add(mana.Red, 2)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test Drain", "DB$ DrainMana | Defined$ Opponent | DrainMana$ True")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(other).ManaPool.Total(); got != 0 {
		t.Errorf("opponent pool = %d, want 0", got)
	}
	if got := g.Player(p).ManaPool.Total(); got != 2 {
		t.Errorf("own pool = %d, want 2", got)
	}
}

// TestDrainManaEffectRejectsRememberAmount proves RememberDrainedMana$
// fails closed.
func TestDrainManaEffectRejectsRememberAmount(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	def := etbChainDef(t, "Test Drain Bad", "DB$ DrainMana | Defined$ Player | RememberDrainedMana$ True")
	if _, err := castETBChain(t, g, p, def, engine.NewScriptedController()); err == nil {
		t.Fatal("ResolveStack succeeded, want an error")
	}
}

// TestEachDamageEffectTargetShapes proves the other target readings:
// ValidTgts$ targets, a Defined$ card list, EachToItself$, and a
// DefinedDamagers$ source list.
func TestEachDamageEffectTargetShapes(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	a := g.NewCard(creatureDefPT(t, "2", "10"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(a)})
	def := etbChainDef(t, "Test Each Tgt", "DB$ EachDamage | DefinedDamagers$ Self | NumDmg$ 1 | ValidTgts$ Creature.OppCtrl | SubAbility$ DBDefined",
		"DBDefined", "DB$ EachDamage | DefinedDamagers$ Self | NumDmg$ 1 | Defined$ Targeted | SubAbility$ DBSelf",
		"DBSelf", "DB$ EachDamage | ValidCards$ Creature.OppCtrl | NumDmg$ 2 | EachToItself$ True")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(a).Damage.Marked; got != 4 {
		t.Errorf("damage = %d, want 4", got)
	}
}
