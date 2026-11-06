package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// Scotty, Miracle Worker grants Beam me up to its owner's graveyard instants
// and sorceries, and its CantBeBeamedUp static (ValidSA$ Spell.KeywordSource,
// StaticAbilityCantBeBeamedUp.java:27-31) keeps Scotty itself from being the
// creature returned for a spell it granted the keyword to: with only Scotty
// in play the spell cannot be cast; with another creature, that one returns.
func TestScottyCannotBeBeamedUpForTheKeywordItGrants(t *testing.T) {
	t.Parallel()

	for _, withBears := range []bool{false, true} {
		g, p, other := newTwoPlayerGame(t)
		g.SetTurnState(1, p, engine.Main1)
		scotty := g.NewCard(corpusCard(t, "Scotty, Miracle Worker"), p, engine.Battlefield)
		var bears engine.CardID
		if withBears {
			bears = g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
		}
		bolt := g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Graveyard)
		sba(g)
		g.Player(p).ManaPool.Add(mana.Red, 1)
		c := engine.NewScriptedController()
		if withBears {
			c.QueueReturnChoice([]engine.CardID{bears})
		}
		c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
		cast := g.CastSpell(p, bolt, c)
		if cast != withBears {
			t.Fatalf("with Grizzly Bears %v: cast = %v, want %v", withBears, cast, withBears)
		}
		if g.Card(scotty).Zone != engine.Battlefield {
			t.Errorf("with Grizzly Bears %v: Scotty in %v, want Battlefield", withBears, g.Card(scotty).Zone)
		}
		if withBears && g.Card(bears).Zone != engine.Hand {
			t.Errorf("Grizzly Bears in %v, want Hand", g.Card(bears).Zone)
		}
	}
}

// Scotty's CantBeBeamedUp covers only a Beam me up Scotty granted
// (ValidSA$ Spell.KeywordSource): for Open Communications' printed keyword,
// outside the main phase where Scotty grants nothing, Scotty can be beamed up.
func TestScottyCanBeBeamedUpForAPrintedBeamMeUp(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.Upkeep)
	scotty := g.NewCard(corpusCard(t, "Scotty, Miracle Worker"), p, engine.Battlefield)
	comms := g.NewCard(corpusCard(t, "Open Communications"), p, engine.Graveyard)
	bolt := g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Graveyard)
	sba(g)
	spec := valid.Parse("Creature.YouCtrl+canBeBeamedUp")
	if !engine.Matches(g, g.Card(scotty), spec, p, comms) {
		t.Error("Scotty cannot be beamed up for Open Communications' printed Beam me up")
	}
	g.SetTurnState(1, p, engine.Main1)
	sba(g)
	if engine.Matches(g, g.Card(scotty), spec, p, bolt) {
		t.Error("Scotty can be beamed up for the Beam me up it grants Lightning Bolt")
	}
}
