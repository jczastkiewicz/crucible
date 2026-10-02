package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// A card's own S:Mode$ CantTarget line (StaticAbilityCantTarget): Gaea's
// Revenge "can't be the target of nongreen spells or abilities from nongreen
// sources" refuses a red spell and admits a green one.
func TestCantTargetStaticFiltersBySourceColor(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, cost string
		color      mana.Colors
		want       bool
	}{
		{"red spell", "R", mana.Red, false},
		{"green spell", "G", mana.Green, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, p, engine.Main1)
			g.Player(p).ManaPool.Add(tc.color, 1)
			revenge := g.NewCard(corpusCard(t, "Gaea's Revenge"), other, engine.Battlefield)
			spell := g.NewCard(instantDefWithAbility(t, "Test Spell", tc.cost, "SP$ Destroy | ValidTgts$ Creature"), p, engine.Hand)
			c := engine.NewScriptedController()
			c.QueueTargets([]engine.EntityID{engine.CardEntity(revenge)})
			if got := g.CastSpell(p, spell, c); got != tc.want {
				t.Errorf("CastSpell targeting Gaea's Revenge = %v, want %v", got, tc.want)
			}
		})
	}
}

// AffectedZone$ Graveyard (Ground Seal): cards in graveyards can't be the
// target of spells or abilities, so Raise Dead has nothing to choose while
// the enchantment is out.
func TestCantTargetStaticAffectedZoneGraveyard(t *testing.T) {
	t.Parallel()

	for _, seal := range []bool{false, true} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		g.Player(p).ManaPool.Add(mana.Black, 1)
		if seal {
			g.NewCard(corpusCard(t, "Ground Seal"), p, engine.Battlefield)
		}
		dead := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Graveyard)
		raise := g.NewCard(instantDefWithAbility(t, "Test Raise", "B",
			"SP$ ChangeZone | Origin$ Graveyard | Destination$ Hand | ValidTgts$ Creature.YouCtrl"), p, engine.Hand)
		c := engine.NewScriptedController()
		c.QueueTargets([]engine.EntityID{engine.CardEntity(dead)})
		if got, want := g.CastSpell(p, raise, c), !seal; got != want {
			t.Errorf("with Ground Seal = %v: CastSpell = %v, want %v", seal, got, want)
		}
	}
}

// "Hexproof from activated abilities" and "from triggered abilities"
// (Volatile Stormdrake) name the kind of ability, not a source: an opponent's
// spell can still target it, an activated ability cannot.
func TestHexproofFromActivatedAbilitiesAdmitsSpells(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).ManaPool.Add(mana.Red, 1)
	drake := g.NewCard(corpusCard(t, "Volatile Stormdrake"), other, engine.Battlefield)
	sba(g)
	spell := g.NewCard(instantDefWithAbility(t, "Test Spell", "R", "SP$ Destroy | ValidTgts$ Creature"), p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(drake)})
	if !g.CastSpell(p, spell, c) {
		t.Error("an opponent's spell cannot target a creature with hexproof from abilities")
	}

	line := "DB$ PutCounter | ValidTgts$ Creature | CounterType$ P1P1 | CounterNum$ 1"
	if err := resolveTargeting(t, g, p, c, []engine.EntityID{engine.CardEntity(drake)}, line); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := g.Card(drake).Counters.Count(engine.P1P1); got != 0 {
		t.Errorf("an activated ability put %d counters on a creature with hexproof from activated abilities", got)
	}
}
