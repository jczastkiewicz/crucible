package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Sunglasses of Urza (White->Red): white mana pays a {R} spell, the player
// choosing white the way they choose a hybrid's half. Without it white mana
// pays nothing red.
func TestManaConvertWhitePaysRed(t *testing.T) {
	t.Parallel()

	for _, withSunglasses := range []bool{true, false} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		if withSunglasses {
			g.NewCard(corpusCard(t, "Sunglasses of Urza"), p, engine.Battlefield)
		}
		spell := g.NewCard(creatureDefCost(t, "Red One", "R"), p, engine.Hand)
		g.Player(p).ManaPool.Add(mana.White, 1)
		c := engine.NewScriptedController()
		c.QueueHybridManaColor(mana.White)
		if got := g.CastSpell(p, spell, c); got != withSunglasses {
			t.Errorf("Sunglasses %v: casting {R} with white = %v", withSunglasses, got)
		}
	}
}

// Celestial Dawn (White->AnyColor nonWhite<-C): white mana pays a {G} part
// without a question, since no other type may; green mana pays a generic
// part but no longer a {G} one.
func TestManaConvertCelestialDawn(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		pool mana.Colors
		cost string
		want bool
	}{
		{"white pays green", mana.White, "G", true},
		{"green no longer pays green", mana.Green, "G", false},
		{"green pays generic", mana.Green, "1", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
			g.NewCard(corpusCard(t, "Celestial Dawn"), p, engine.Battlefield)
			spell := g.NewCard(creatureDefCost(t, "Some One", tc.cost), p, engine.Hand)
			g.Player(p).ManaPool.Add(tc.pool, 1)
			c := engine.NewScriptedController()
			c.QueuePayGeneric(mana.ShardG)
			if got := g.CastSpell(p, spell, c); got != tc.want {
				t.Errorf("casting {%s} = %v, want %v", tc.cost, got, tc.want)
			}
		})
	}
}

// False Dawn's effect (White->AnyColor until end of turn) lets white mana pay
// a {U} spell cast after it resolves.
func TestManaConvertFalseDawnEffect(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(creatureDefCost(t, "Library Card", "1"), p, engine.Library)
	dawn := g.NewCard(corpusCard(t, "False Dawn"), p, engine.Hand)
	spell := g.NewCard(creatureDefCost(t, "Blue One", "U"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.White, 3)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardW)
	c.QueueHybridManaColor(mana.White)
	if !g.CastSpell(p, dawn, c) {
		t.Fatal("False Dawn not cast")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if !g.CastSpell(p, spell, c) {
		t.Error("white mana did not pay {U} under False Dawn's effect")
	}
}

// Quicksilver Elemental's line (Blue->AnyColor, ValidCard$ Card.Self, ValidSA$
// Activated) lets blue mana pay its own {R} activation but not a {R} spell.
func TestManaConvertBlueForOwnActivations(t *testing.T) {
	t.Parallel()

	line := "S:Mode$ ManaConvert | ValidPlayer$ You | ValidCard$ Card.Self | ValidSA$ Activated | ManaConversion$ Blue->AnyColor"
	g, p, _ := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.Main1)
	def := creatureDefWithAbility(t, "Red Pinger", "AB$ GainLife | Cost$ R | Defined$ You | LifeAmount$ 1")
	def.Faces[0].Statics = append(def.Faces[0].Statics, scriptDef(t, "x", "Enchantment", line).Faces[0].Statics...)
	pinger := g.NewCard(def, p, engine.Battlefield)
	g.Player(p).ManaPool.Add(mana.Blue, 1)
	c := engine.NewScriptedController()
	c.QueueHybridManaColor(mana.Blue)
	if !g.ActivateAbility(p, pinger, 0, c) {
		t.Error("blue mana did not pay the {R} activation")
	}

	spellDef := creatureDefCost(t, "Red Caster", "R")
	spellDef.Faces[0].Statics = append(spellDef.Faces[0].Statics, scriptDef(t, "x", "Enchantment", line).Faces[0].Statics...)
	spell := g.NewCard(spellDef, p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Blue, 1)
	if g.CastSpell(p, spell, engine.NewScriptedController()) {
		t.Error("blue mana paid a {R} spell under an Activated-only line")
	}
}

// C->AnyColor (Abstruse Appropriation's conversion) lets colorless mana pay
// a {B} part, the player choosing colorless the way they choose a {C/B}
// hybrid's half.
func TestManaConvertColorlessPaysColor(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Rule", "Enchantment", "S:Mode$ ManaConvert | ValidPlayer$ You | ManaConversion$ C->AnyColor"), p, engine.Battlefield)
	spell := g.NewCard(creatureDefCost(t, "Black One", "B"), p, engine.Hand)
	g.Player(p).ManaPool.AddColorless(1)
	c := engine.NewScriptedController()
	c.QueuePayColorlessHybrid(false)
	if !g.CastSpell(p, spell, c) {
		t.Error("colorless mana did not pay {B} under C->AnyColor")
	}
}

// Abstruse Appropriation exiles a permanent; its effect lets the caster cast
// that card through the effect's MayPlay$ and spend colorless mana as though
// it were any color on that cast only (ValidSA$ Spell.MayPlaySource).
func TestManaConvertAbstruseAppropriationCast(t *testing.T) {
	t.Parallel()

	g, p, opp := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	victim := g.NewCard(creatureDefCost(t, "Black One", "B"), opp, engine.Battlefield)
	other := g.NewCard(creatureDefCost(t, "Black Two", "B"), p, engine.Hand)
	spell := g.NewCard(corpusCard(t, "Abstruse Appropriation"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.White, 1)
	g.Player(p).ManaPool.Add(mana.Black, 1)
	g.Player(p).ManaPool.AddColorless(2)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(victim)})
	c.QueuePayGeneric(mana.ShardC)
	c.QueuePayGeneric(mana.ShardC)
	if !g.CastSpell(p, spell, c) {
		t.Fatal("Abstruse Appropriation not cast")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if g.Card(victim).Zone != engine.Exile {
		t.Fatalf("victim in %v, want Exile", g.Card(victim).Zone)
	}

	g.Player(p).ManaPool.AddColorless(1)
	if g.CastSpell(p, other, engine.NewScriptedController()) {
		t.Error("colorless mana paid {B} on a spell not cast through the effect")
	}
	c = engine.NewScriptedController()
	c.QueuePayColorlessHybrid(false)
	if !g.CastSpell(p, victim, c) {
		t.Error("colorless mana did not pay {B} on the exiled card cast through the effect")
	}
}

// An activated ability a Continuous static granted pays under that static's
// own ManaConversion$ (PlaySpellAbility.java:653-656; Grell Philosopher's
// Blue->AnyColor, Drana and Linvala's AnyType->AnyColor): blue mana pays the
// granted ability's {R}; the creature's own {R} ability is not converted.
func TestManaConvertOnGrantingStatic(t *testing.T) {
	t.Parallel()

	for _, conversion := range []string{"Blue->AnyColor", "AnyType->AnyColor"} {
		t.Run(conversion, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			g.SetTurnState(1, p, engine.Main1)
			g.NewCard(scriptDef(t, "Test Grantor", "Enchantment",
				"S:Mode$ Continuous | Affected$ Creature.YouCtrl | AddAbility$ ABPing | ManaConversion$ "+conversion,
				"SVar:ABPing:AB$ GainLife | Cost$ R | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
			pinger := g.NewCard(creatureDefWithAbility(t, "Red Pinger", "AB$ GainLife | Cost$ R | Defined$ You | LifeAmount$ 2"), p, engine.Battlefield)
			sba(g)
			g.Player(p).ManaPool.Add(mana.Blue, 1)
			if g.ActivateAbility(p, pinger, 0, engine.NewScriptedController()) {
				t.Error("blue mana paid the printed {R} ability")
			}
			c := engine.NewScriptedController()
			if conversion == "Blue->AnyColor" {
				c.QueueHybridManaColor(mana.Blue)
			} else {
				c.QueuePayGeneric(mana.ShardU)
			}
			if !g.ActivateAbility(p, pinger, 1, c) {
				t.Errorf("blue mana did not pay the granted {R} ability: %v", g.TakePendingError())
			}
		})
	}
}

// A mana ability's own mana cost pays under ManaConvert too
// (PlaySpellAbility.java:658, mana abilities included): with an
// AnyType->AnyColor line, blue mana pays a filter's {R}.
func TestManaConvertPaysAManaAbilityCost(t *testing.T) {
	t.Parallel()

	for _, withRule := range []bool{true, false} {
		g, p, _ := newTwoPlayerGame(t)
		g.SetTurnState(1, p, engine.Main1)
		if withRule {
			g.NewCard(scriptDef(t, "Test Rule", "Enchantment", "S:Mode$ ManaConvert | ValidPlayer$ You | ManaConversion$ AnyType->AnyColor"), p, engine.Battlefield)
		}
		filter := g.NewCard(creatureDefWithAbility(t, "Red Filter", "AB$ Mana | Cost$ R | Produced$ G"), p, engine.Battlefield)
		g.Player(p).ManaPool.Add(mana.Blue, 1)
		c := engine.NewScriptedController()
		c.QueuePayGeneric(mana.ShardU)
		if got := g.ActivateManaAbility(p, filter, 0, c); got != withRule {
			t.Errorf("rule %v: blue paid the {R} mana ability = %v", withRule, got)
		}
	}
}
