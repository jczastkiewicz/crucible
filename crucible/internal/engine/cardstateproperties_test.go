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
		{"Card.Chosen", bears, giant, false},
		{"Card.ManaSpentEQ0", bears, engine.NoCard, false},
		{"Creature.sharesCardTypeWithOther Remembered", bears, giant, true},
		{"Creature.sharesCardTypeWithOther Remembered", ogre, giant, false},
		{"Creature.sharesAllCardTypesWithOther Remembered", bears, giant, true},
		{"Creature.SharesColorWithOther Remembered", giant, giant, true},
		{"Creature.SharesColorWithOther Remembered", bears, giant, false},
		{"Creature.CanBeSacrificedBy", bears, giant, false},
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

// CastSaSource is the cast of the source itself (Card.getCastSA), kept as the
// spell resolves into a permanent; Count$ThisTurnCast_<valid>$CardManaCost
// sums the mana values of the spells the valid string keeps (Call Forth the
// Tempest's "other spells").
func TestCastSaSourceNamesTheSourcesOwnCast(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
	giant := g.NewCard(corpusCard(t, "Hill Giant"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardG)
	castThenResolve(t, g, p, bears, c)
	g.Player(p).ManaPool.Add(mana.Red, 4)
	c = engine.NewScriptedController()
	for range 3 {
		c.QueuePayGeneric(mana.ShardR)
	}
	castThenResolve(t, g, p, giant, c)
	for _, tc := range []struct {
		card engine.CardID
		want bool
	}{{giant, true}, {bears, false}} {
		if got := engine.Matches(g, g.Card(tc.card), valid.Parse("Card.CastSaSource"), p, giant); got != tc.want {
			t.Errorf("CastSaSource on %s = %v, want %v", g.Card(tc.card).Name(), got, tc.want)
		}
	}
	before := g.Player(p).Life
	resolveLine(t, g, p, engine.NewScriptedController(), "DB$ GainLife | Defined$ You | LifeAmount$ X",
		"X", "Count$ThisTurnCast_Card.YouCtrl+!CastSaSource$CardManaCost")
	if got := g.Player(p).Life - before; got != 6 {
		t.Errorf("gained %d, want 6 (the Bears' 2 and the Giant's 4)", got)
	}
}

// NameNotEnchantingEnchantedPlayer: no card attached to the player the source
// enchants has c's name (CardProperty.java:455-459, Curse of Misfortunes).
func TestNameNotEnchantingEnchantedPlayerReadsThePlayersAttachments(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	curse := g.NewCard(corpusCard(t, "Curse of Misfortunes"), p, engine.Battlefield)
	g.AttachToPlayer(curse, other)
	twin := g.NewCard(corpusCard(t, "Curse of Misfortunes"), p, engine.Library)
	heart := g.NewCard(corpusCard(t, "Curse of the Pierced Heart"), p, engine.Library)
	spec := valid.Parse("Aura.Curse+NameNotEnchantingEnchantedPlayer")
	if engine.Matches(g, g.Card(twin), spec, p, curse) || !engine.Matches(g, g.Card(heart), spec, p, curse) {
		t.Errorf("twin %v, other curse %v; want false, true",
			engine.Matches(g, g.Card(twin), spec, p, curse), engine.Matches(g, g.Card(heart), spec, p, curse))
	}
}

// IsTriggerRemembered reads the remembered list of the trigger in context
// while its targets are chosen: Espers to Magicite's reflexive trigger may
// target only a creature card it exiled (CardProperty.java:164-176).
func TestIsTriggerRememberedNamesTheReflexiveTriggersCards(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	ogre := g.NewCard(corpusCard(t, "Gray Ogre"), other, engine.Graveyard)
	espers := g.NewCard(corpusCard(t, "Espers to Magicite"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Black, 4)
	c := engine.NewScriptedController()
	for range 3 {
		c.QueuePayGeneric(mana.ShardB)
	}
	c.QueueTargets([]engine.EntityID{engine.CardEntity(ogre)})
	castThenResolve(t, g, p, espers, c)
	if err := g.TakePendingError(); err != nil {
		t.Fatal(err)
	}
	if g.Card(ogre).Zone != engine.Exile {
		t.Fatalf("Gray Ogre in %v, want exile", g.Card(ogre).Zone)
	}
	tokens := 0
	for _, id := range g.Zone(engine.Battlefield, p).Cards() {
		if g.Card(id).IsToken && g.Card(id).Name() == "Gray Ogre" {
			tokens++
		}
	}
	if tokens != 1 {
		t.Errorf("%d Gray Ogre tokens, want 1", tokens)
	}
}

// ExiledWithEffectSource reads the effect card's maker: a card exiled with the
// host matches from the effect the same host made (CardProperty.java:414-420).
func TestExiledWithEffectSourceReadsTheEffectsMaker(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	ogre := g.NewCard(corpusCard(t, "Gray Ogre"), other, engine.Graveyard)
	bolt := g.NewCard(corpusCard(t, "Lightning Bolt"), other, engine.Exile)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil,
		"DB$ ChangeZoneAll | ChangeType$ Creature | Origin$ Graveyard | Destination$ Exile | SubAbility$ DBEffect",
		"DBEffect", "DB$ Effect | Duration$ Permanent"); err != nil {
		t.Fatal(err)
	}
	var effect engine.CardID
	for _, id := range g.Zone(engine.Command, p).Cards() {
		if g.Card(id).IsEffect {
			effect = id
		}
	}
	if effect == engine.NoCard {
		t.Fatal("no effect card made")
	}
	spec := valid.Parse("Card.ExiledWithEffectSource")
	if !engine.Matches(g, g.Card(ogre), spec, p, effect) || engine.Matches(g, g.Card(bolt), spec, p, effect) {
		t.Errorf("exiled Ogre %v, other exiled card %v; want true, false",
			engine.Matches(g, g.Card(ogre), spec, p, effect), engine.Matches(g, g.Card(bolt), spec, p, effect))
	}
}

// A trigger's ValidCard$ reads a filtered Defined$ off its host with no
// ability in context: Thought Prison's SharesCMCWith
// Imprinted.ExiledWithSource (AbilityUtils.java:317-322).
func TestSharesCMCWithReadsAFilteredDefinedOutsideAnAbility(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.NewCard(corpusCard(t, "Gray Ogre"), other, engine.Graveyard)
	prison, err := resolveNow(t, g, p, engine.NewScriptedController(), nil,
		"DB$ ChangeZoneAll | ChangeType$ Creature | Origin$ Graveyard | Destination$ Exile | Imprint$ True")
	if err != nil {
		t.Fatal(err)
	}
	spec := valid.Parse("Card.SharesCMCWith Imprinted.ExiledWithSource")
	for _, tc := range []struct {
		name string
		want bool
	}{{"Gray Ogre", true}, {"Grizzly Bears", false}} {
		c := g.NewCard(corpusCard(t, tc.name), other, engine.Hand)
		if got := engine.Matches(g, g.Card(c), spec, p, prison); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
		if err := g.TakePendingError(); err != nil {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
}

// set<CODE> (CardProperty.java:1960-1969) reads the name's print from its
// oldest edition; without edition data it is an error.
func TestSetPropertyReadsTheOldestPrint(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	efreet := g.NewCard(corpusCard(t, "Serendib Efreet"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	engine.Matches(g, g.Card(bears), valid.Parse("Card.setLEA"), p, engine.NoCard)
	if err := g.TakePendingError(); err == nil {
		t.Error("set<CODE> without edition data: no error")
	}

	g.SetBoosters(boosterTable(t))
	for _, tc := range []struct {
		spec string
		card engine.CardID
		want bool
	}{
		{"Card.setARN", efreet, true},
		{"Card.setARN", bears, false},
		{"Card.setLEA", bears, true},
	} {
		if got := engine.Matches(g, g.Card(tc.card), valid.Parse(tc.spec), p, engine.NoCard); got != tc.want {
			t.Errorf("%s on %s = %v, want %v", tc.spec, g.Card(tc.card).Name(), got, tc.want)
		}
		if err := g.TakePendingError(); err != nil {
			t.Errorf("%s: %v", tc.spec, err)
		}
	}
}

// BorderColor<Color> (CardProperty.java:87-89, Card.borderColor) reads the
// border of the card's print edition (ADR-0061): Black without a print or
// edition data and for a basic land, else the edition file's Border= key.
func TestBorderColorReadsThePrintsEdition(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	white := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	silver := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	forest := g.NewCard(corpusCard(t, "Forest"), p, engine.Battlefield)
	g.SetPrintEdition(white, "4ED")
	g.SetPrintEdition(silver, "UNH")
	g.SetPrintEdition(forest, "4ED")
	if !engine.Matches(g, g.Card(white), valid.Parse("Card.BorderColorBlack"), p, engine.NoCard) {
		t.Error("a 4ED print without edition data: not black")
	}

	g.SetBoosters(boosterTable(t))
	for _, tc := range []struct {
		card  engine.CardID
		color string
		want  bool
	}{
		{bears, "Black", true}, {bears, "White", false},
		{white, "White", true}, {white, "Black", false},
		{silver, "Silver", true}, {silver, "White", false},
		{forest, "Black", true}, {forest, "White", false},
	} {
		if got := engine.Matches(g, g.Card(tc.card), valid.Parse("Card.BorderColor"+tc.color), p, engine.NoCard); got != tc.want {
			t.Errorf("BorderColor%s on card %d = %v, want %v", tc.color, tc.card, got, tc.want)
		}
	}
	if err := g.TakePendingError(); err != nil {
		t.Error(err)
	}
}

// ExiledWithSourceLKI (CardProperty.java:374-397) reads the record of the
// card entering its zone this turn: the object that left, so a card that
// left the source's exile for the graveyard is, and one still in that
// exile or put onto the battlefield (a new object's record) is not.
func TestExiledWithSourceLKIReadsTheZoneEntryRecord(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	ogre := g.NewCard(corpusCard(t, "Gray Ogre"), other, engine.Graveyard)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Graveyard)
	c := engine.NewScriptedController()
	c.QueueCardOrder([]engine.CardID{ogre, bears})
	host, err := resolveNow(t, g, p, c, nil,
		"DB$ ChangeZoneAll | ChangeType$ Creature | Origin$ Graveyard | Destination$ Exile")
	if err != nil {
		t.Fatal(err)
	}
	spec := valid.Parse("Card.ExiledWithSourceLKI")
	is := func(id engine.CardID) bool { return engine.Matches(g, g.Card(id), spec, p, host) }
	if is(ogre) {
		t.Error("a card still in the exile it went to matched")
	}
	g.Move(ogre, engine.Graveyard, other)
	g.Move(bears, engine.Battlefield, other)
	if !is(ogre) || is(bears) {
		t.Errorf("left exile for the graveyard %v, for the battlefield %v; want true, false", is(ogre), is(bears))
	}
	if engine.Matches(g, g.Card(ogre), spec, p, bears) {
		t.Error("matched against a source that did not exile it")
	}
}

// Mana Web's canProduceSameManaTypeWith (Card.java:3375-3380): a land that
// can produce a color the tapped land can.
func TestCanProduceSameManaTypeWithComparesProducibleColors(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	island := g.NewCard(corpusCard(t, "Island"), p, engine.Battlefield)
	for _, tc := range []struct {
		name string
		want bool
	}{{"Island", true}, {"Underground River", true}, {"Mountain", false}, {"Grizzly Bears", false}} {
		c := g.NewCard(corpusCard(t, tc.name), p, engine.Battlefield)
		if got := engine.Matches(g, g.Card(c), valid.Parse("Card.canProduceSameManaTypeWith"), p, island); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}
