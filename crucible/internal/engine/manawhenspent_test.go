package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// TriggersWhenSpent$ (AbilityManaPart.java:333-339): Pyromancer's Goggles'
// {R}, spent on a red instant, registers its trigger for that spell, which
// copies it. The copy deals 3 more.
func TestGogglesManaCopiesTheRedInstantItPaysFor(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	goggles := g.NewCard(corpusCard(t, "Pyromancer's Goggles"), p, engine.Battlefield)
	bolt := g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Hand)
	c := engine.NewScriptedController()
	if !g.ActivateManaAbility(p, goggles, 0, c) {
		t.Fatalf("Goggles' mana ability not activated: %v", g.TakePendingError())
	}
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	if !g.CastSpell(p, bolt, c) {
		t.Fatalf("Lightning Bolt not cast: %v", g.TakePendingError())
	}
	c.QueueConfirmEffect(false)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	if got := g.Player(other).Life; got != 14 {
		t.Errorf("opponent at %d life, want 14 (the Bolt and its copy)", got)
	}
}

// The trigger is the spell's the mana paid for: Goggles' {R} spent on a
// non-red spell copies nothing, and a later red instant paid with other mana
// is not copied either.
func TestGogglesManaTriggersOnlyForItsOwnSpell(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	goggles := g.NewCard(corpusCard(t, "Pyromancer's Goggles"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
	bolt := g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Hand)
	c := engine.NewScriptedController()
	if !g.ActivateManaAbility(p, goggles, 0, c) {
		t.Fatalf("Goggles' mana ability not activated: %v", g.TakePendingError())
	}
	g.Player(p).ManaPool.Add(mana.Green, 1)
	c.QueuePayGeneric(mana.ShardR)
	if !g.CastSpell(p, bears, c) {
		t.Fatalf("Grizzly Bears not cast: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	g.Player(p).ManaPool.Add(mana.Red, 1)
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	if !g.CastSpell(p, bolt, c) {
		t.Fatalf("Lightning Bolt not cast: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(other).Life; got != 17 {
		t.Errorf("opponent at %d life, want 17 (the Bolt alone)", got)
	}
}

// SnowSpentForCardsColor$ (TriggerSpellAbilityCastOrCopy.java:205-219): Boreal
// Outrider's counter needs snow mana of one of the creature's colors spent
// casting it. Green snow mana gives the green Bears the counter; snow
// colorless mana does not.
func TestBorealOutriderCountsSnowManaOfTheSpellsColor(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		snowLand string
		color    mana.Colors
		want     int
	}{
		{"green snow", "Snow-Covered Forest", mana.Green, 1},
		{"colorless snow", "Snow-Covered Wastes", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, p, engine.Main1)
			g.NewCard(corpusCard(t, "Boreal Outrider"), p, engine.Battlefield)
			land := g.NewCard(corpusCard(t, tc.snowLand), p, engine.Battlefield)
			bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
			c := engine.NewScriptedController()
			if !g.TapLandForMana(p, land, tc.color, c) {
				t.Fatalf("%s not tapped", tc.snowLand)
			}
			g.Player(p).ManaPool.Add(mana.Green, 1)
			if tc.color == mana.Green {
				c.QueuePayGeneric(mana.ShardG)
			} else {
				c.QueuePayGeneric(mana.ShardC)
			}
			if !g.CastSpell(p, bears, c) {
				t.Fatalf("Grizzly Bears not cast: %v", g.TakePendingError())
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := g.Card(bears).Counters.Count(engine.CounterType("P1P1")); got != tc.want {
				t.Errorf("Bears has %d +1/+1 counters, want %d", got, tc.want)
			}
		})
	}
}

// A pool holding restricted mana pays through a view of its usable units
// (Pool.payUsable): Goggles' {R} keeps its trigger there, and is found
// again when the spent units are removed. Eldrazi Temple's {C}{C} may pay
// only for colorless Eldrazi, so it stays in the pool.
func TestGogglesManaKeepsItsTriggerBesideRestrictedMana(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	temple := g.NewCard(corpusCard(t, "Eldrazi Temple"), p, engine.Battlefield)
	goggles := g.NewCard(corpusCard(t, "Pyromancer's Goggles"), p, engine.Battlefield)
	bolt := g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Hand)
	c := engine.NewScriptedController()
	if !g.ActivateManaAbility(p, temple, 1, c) {
		t.Fatalf("Eldrazi Temple's restricted mana ability not activated: %v", g.TakePendingError())
	}
	if !g.ActivateManaAbility(p, goggles, 0, c) {
		t.Fatalf("Goggles' mana ability not activated: %v", g.TakePendingError())
	}
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	if !g.CastSpell(p, bolt, c) {
		t.Fatalf("Lightning Bolt not cast: %v", g.TakePendingError())
	}
	c.QueueConfirmEffect(false)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(other).Life; got != 14 {
		t.Errorf("opponent at %d life, want 14 (the Bolt and its copy)", got)
	}
	if got := g.Player(p).ManaPool.Breakdown(); got[5] != 2 {
		t.Errorf("pool %v, want the Temple's {C}{C} left", got)
	}
}

// Mana spent to activate an ability registers its trigger for that ability
// (MagicStack.java:385 runs for every stack item): Sunken Palace's shape, a
// Mode$ SpellAbilityCast copy, on a synthetic {U} source. The activated
// ability gains 1 life, and its copy 1 more.
func TestWhenSpentManaCopiesTheAbilityItPaysFor(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	source := g.NewCard(scriptDef(t, "Test Palace", "Land",
		"A:AB$ Mana | Cost$ T | Produced$ U | TriggersWhenSpent$ TrigCopy | SpellDescription$ Add {U}.",
		"SVar:TrigCopy:Mode$ SpellAbilityCast | ValidSA$ Spell,Activated | ValidActivatingPlayer$ You | Execute$ TrigCopyMain | TriggerDescription$ Copy it.",
		"SVar:TrigCopyMain:DB$ CopySpellAbility | Defined$ TriggeredSpellAbility"), p, engine.Battlefield)
	shrine := g.NewCard(scriptDef(t, "Test Shrine", "Artifact",
		"A:AB$ GainLife | Cost$ U | Defined$ You | LifeAmount$ 1 | SpellDescription$ You gain 1 life."), p, engine.Battlefield)
	c := engine.NewScriptedController()
	if !g.ActivateManaAbility(p, source, 0, c) {
		t.Fatalf("mana ability not activated: %v", g.TakePendingError())
	}
	if !g.ActivateAbility(p, shrine, 0, c) {
		t.Fatalf("Test Shrine not activated: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	if got := g.Player(p).Life; got != 22 {
		t.Errorf("life %d, want 22 (the ability and its copy)", got)
	}
}
