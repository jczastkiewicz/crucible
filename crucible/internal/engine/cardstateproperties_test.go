package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// Card properties that read c's own state, its mana cost, its graveyard
// position and the source's lists (CardProperty.java and
// CardStateProperty.java: Worthy, AllColors, CostsHybridMana, ManaCost,
// DoubleFaced, equipping, ControllerControls, Above, powerNOTbasePower,
// sharesControllerWith, doesNotShareNameWith, NotDefined<Defined>, ...).
func TestCardStatePropertiesReadTheCardAndTheSourcesLists(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	ogre := g.NewCard(corpusCard(t, "Gray Ogre"), other, engine.Battlefield)
	giant := g.NewCard(corpusCard(t, "Hill Giant"), p, engine.Battlefield)
	isamaru := g.NewCard(corpusCard(t, "Isamaru, Hound of Konda"), p, engine.Battlefield)
	queen := g.NewCard(corpusCard(t, "Sliver Queen"), p, engine.Battlefield)
	probe := g.NewCard(corpusCard(t, "Gitaxian Probe"), p, engine.Hand)
	recruit := g.NewCard(corpusCard(t, "Boros Recruit"), p, engine.Hand)
	thopter := g.NewCard(corpusCard(t, "Ornithopter"), p, engine.Battlefield)
	delver := g.NewCard(corpusCard(t, "Delver of Secrets"), p, engine.Battlefield)
	cathar := g.NewCard(corpusCard(t, "Brutal Cathar"), p, engine.Battlefield)
	sword := g.NewCard(corpusCard(t, "Bonesplitter"), p, engine.Battlefield)
	loose := g.NewCard(corpusCard(t, "Bonesplitter"), p, engine.Battlefield)
	aura := g.NewCard(corpusCard(t, "Pacifism"), p, engine.Hand)
	forest := g.NewCard(corpusCard(t, "Forest"), p, engine.Battlefield)
	desert := g.NewCard(corpusCard(t, "Ipnu Rivulet"), p, engine.Battlefield)
	g.Attach(sword, bears)
	sba(g)
	g.Card(isamaru).Harnessed = true
	first := g.NewCard(corpusCard(t, "Llanowar Elves"), p, engine.Graveyard)
	second := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Graveyard)
	third := g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Graveyard)
	g.Card(giant).Memory.SetChosenColors(mana.Green)
	g.Card(giant).Memory.Remember(engine.CardEntity(ogre))
	g.Card(giant).Damage.Sources = append(g.Card(giant).Damage.Sources, ogre)

	cases := []struct {
		spec   string
		card   engine.CardID
		source engine.CardID
		want   bool
	}{
		{"Card.copiedSpell", bears, engine.NoCard, false},
		{"Card.nonCopiedSpell", bears, engine.NoCard, true},
		{"Card.Daybound", cathar, engine.NoCard, false},
		{"Card.noName", bears, engine.NoCard, false},
		{"Creature.Worthy", isamaru, engine.NoCard, true},
		{"Creature.Worthy", bears, engine.NoCard, false},
		{"Creature.AllColors", queen, engine.NoCard, true},
		{"Creature.AllColors", bears, engine.NoCard, false},
		{"Card.AnyChosenColor", bears, giant, true},
		{"Card.AnyChosenColor", isamaru, giant, false},
		{"Card.CostsPhyrexianMana", probe, engine.NoCard, true},
		{"Card.CostsPhyrexianMana", recruit, engine.NoCard, false},
		{"Card.CostsHybridMana", recruit, engine.NoCard, true},
		{"Card.CostsHybridMana", probe, engine.NoCard, false},
		{"Card.ManaCost0", thopter, engine.NoCard, true},
		{"Card.ManaCost0", bears, engine.NoCard, false},
		{"Card.ManaCostPartialGreen", bears, engine.NoCard, true},
		{"Card.ManaCostPartialBlue", bears, engine.NoCard, false},
		{"Card.DoubleFaced", delver, engine.NoCard, true},
		{"Card.CanTransform", delver, engine.NoCard, true},
		{"Card.DoubleFaced", bears, engine.NoCard, false},
		{"Card.canReceiveCounters P1P1", bears, engine.NoCard, true},
		{"Card.CanEnchantSource", bears, aura, true},
		{"Card.CanEnchantSource", forest, aura, false},
		{"Equipment.equipping", sword, engine.NoCard, true},
		{"Equipment.equipping", loose, engine.NoCard, false},
		{"Creature.ControllerControlsForest", bears, engine.NoCard, true},
		{"Creature.ControllerControlsForest", ogre, engine.NoCard, false},
		{"Creature.ControllerControlsMoreLand", bears, engine.NoCard, false},
		{"Creature.ControllerControlsAtLeastAsManyLand", bears, engine.NoCard, true},
		{"Creature.ControllerControlsAtLeastAsManyLand", ogre, engine.NoCard, false},
		{"Creature.ControllerControlsLand_GE2", bears, engine.NoCard, true},
		{"Creature.ControllerControlsLand_GE3", bears, engine.NoCard, false},
		{"Card.Above", third, first, true},
		{"Card.Above", first, third, false},
		{"Card.DirectlyAbove", second, first, true},
		{"Card.DirectlyAbove", third, first, false},
		{"Card.harnessed", isamaru, engine.NoCard, true},
		{"Card.harnessed", bears, engine.NoCard, false},
		{"Creature.powerNOTbasePower", bears, engine.NoCard, true},
		{"Creature.powerNOTbasePower", giant, engine.NoCard, false},
		{"Land.hasANonBasicLandType", desert, engine.NoCard, true},
		{"Land.hasANonBasicLandType", forest, engine.NoCard, false},
		{"Creature.EnteredUnder You", bears, engine.NoCard, true},
		{"Creature.EnteredUnder You", ogre, engine.NoCard, false},
		{"Creature.Damaged", ogre, giant, true},
		{"Creature.Damaged", bears, giant, false},
		{"Creature.sharesControllerWith", bears, giant, true},
		{"Creature.sharesControllerWith", ogre, giant, false},
		{"Creature.sharesOwnerWith Remembered", ogre, giant, true},
		{"Creature.sharesOwnerWith Remembered", bears, giant, false},
		{"Creature.SharesCMCWith Remembered", bears, giant, false},
		{"Creature.SharesCMCWith Remembered", isamaru, giant, false},
		{"Creature.SharesCMCWith", bears, second, true},
		{"Creature.SharesCMCWith", giant, second, false},
		{"Creature.doesNotShareNameWith YourGraveyard", bears, giant, false},
		{"Creature.doesNotShareNameWith YourGraveyard", ogre, giant, true},
		{"Creature.doesNotShareNameWith OtherYourBattlefield", bears, giant, true},
		{"Creature.NotDefinedRemembered", ogre, giant, false},
		{"Creature.NotDefinedRemembered", bears, giant, true},
		{"Card.ThisTurnCast", bears, engine.NoCard, false},
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

// ThisTurnCast reads the spells cast this turn; Unearth marks the card it
// returns (IsUnearthed, ChangeZoneEffect.java:685).
func TestThisTurnCastAndIsUnearthedReadTheCastAndTheReturn(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardG)
	castThenResolve(t, g, p, bears, c)
	if !engine.Matches(g, g.Card(bears), valid.Parse("Card.ThisTurnCast"), p, engine.NoCard) {
		t.Error("ThisTurnCast false for the Bears just cast")
	}

	elemental := g.NewCard(corpusCard(t, "Hellspark Elemental"), p, engine.Graveyard)
	isUnearthed := func() bool {
		return engine.Matches(g, g.Card(elemental), valid.Parse("Creature.IsUnearthed"), p, engine.NoCard)
	}
	if isUnearthed() {
		t.Fatal("IsUnearthed before the unearth")
	}
	g.Player(p).ManaPool.Add(mana.Red, 2)
	uc := engine.NewScriptedController()
	uc.QueuePayGeneric(mana.ShardR)
	activateOrFail(t, g, p, uc, elemental, 0)
	if g.Card(elemental).Zone != engine.Battlefield || !isUnearthed() {
		t.Errorf("after the unearth: zone %v, IsUnearthed %v; want battlefield, true", g.Card(elemental).Zone, isUnearthed())
	}
}
