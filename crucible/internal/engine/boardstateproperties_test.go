package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// Card properties that read the board around c: what is attached to it and
// whose card the source is attached to, its marked damage and power and
// toughness, and the highest mana value of a type on the battlefield
// (CardProperty.java: EquippedBy<defined>, EnchantedController, isDamaged,
// equalPT, yardGreatestPower, greatestCMC_).
func TestCardPropertiesReadAttachmentsDamageAndBoardMaxima(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	ogre := g.NewCard(corpusCard(t, "Gray Ogre"), other, engine.Battlefield)
	giant := g.NewCard(corpusCard(t, "Hill Giant"), other, engine.Battlefield)
	sword := g.NewCard(corpusCard(t, "Bonesplitter"), p, engine.Battlefield)
	aura := g.NewCard(corpusCard(t, "Pacifism"), p, engine.Battlefield)
	g.Attach(sword, bears)
	g.Attach(aura, ogre)
	g.Card(giant).Damage.Mark(1, false)
	g.NewCard(corpusCard(t, "Llanowar Elves"), p, engine.Graveyard)
	sba(g)

	cases := []struct {
		spec   string
		card   engine.CardID
		source engine.CardID
		want   bool
	}{
		{"Creature.EquippedByValid Equipment", bears, aura, true},
		{"Creature.EquippedByValid Equipment", ogre, aura, false},
		{"Creature.EnchantedControllerCtrl", giant, aura, true},
		{"Creature.EnchantedControllerCtrl", bears, aura, false},
		{"Creature.isDamaged", giant, engine.NoCard, true},
		{"Creature.isDamaged", ogre, engine.NoCard, false},
		{"Creature.equalPT", bears, engine.NoCard, false},
		{"Creature.equalPT", giant, engine.NoCard, true},
		{"Creature.yardGreatestPower", ogre, engine.NoCard, true},
		{"Creature.greatestCMC_Creature", giant, engine.NoCard, true},
		{"Creature.greatestCMC_Creature", ogre, engine.NoCard, false},
		{"Creature.greatestCMC_CreatureControlledByYou", bears, engine.NoCard, true},
	}
	for _, tc := range cases {
		if got := engine.Matches(g, g.Card(tc.card), valid.Parse(tc.spec), p, tc.source); got != tc.want {
			t.Errorf("%s on %s = %v, want %v", tc.spec, g.Card(tc.card).Name(), got, tc.want)
		}
		if err := g.TakePendingError(); err != nil {
			t.Errorf("%s: %v", tc.spec, err)
		}
	}
}

// NoAbilities is Card.hasNoAbilities: a vanilla creature and a spell whose
// only ability is its mana-cost spell have none; a keyword, an activated
// ability or a basic land type's mana ability is one (Card.java:3446-3468).
func TestNoAbilitiesMatchesVanillaCardsOnly(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	for _, tc := range []struct {
		name string
		zone engine.ZoneType
		want bool
	}{
		{"Grizzly Bears", engine.Battlefield, true},
		{"Lightning Bolt", engine.Hand, true},
		{"Llanowar Elves", engine.Battlefield, false},
		{"Serra Angel", engine.Battlefield, false},
		{"Forest", engine.Battlefield, false},
	} {
		id := g.NewCard(corpusCard(t, tc.name), p, tc.zone)
		if got := engine.Matches(g, g.Card(id), valid.Parse("Card.NoAbilities"), p, engine.NoCard); got != tc.want {
			t.Errorf("%s NoAbilities = %v, want %v", tc.name, got, tc.want)
		}
	}
}
