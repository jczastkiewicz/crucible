package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Zameck Guildmage (SubCounter<1/P1P1/Creature.YouCtrl>): the cost removes a
// counter from a creature its controller chooses, not from the source
// (CostRemoveCounter's typed branch), paid through activateGeneral.
func TestActivationRemovesACounterFromAChosenCreature(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	libraryCards(t, g, p, 3)
	mage := g.NewCard(corpusCard(t, "Zameck Guildmage"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	g.Card(bears).Counters.Add(engine.P1P1, 2)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	g.Player(p).ManaPool.Add(mana.Blue, 1)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{bears})
	if !g.ActivateAbility(p, mage, 1, c) {
		t.Fatalf("activation failed: %v", g.TakePendingError())
	}
	if got := g.Card(bears).Counters.Count(engine.P1P1); got != 1 {
		t.Errorf("Bears' +1/+1 counters = %d, want 1", got)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := len(g.Zone(engine.Hand, p).Cards()); got != 1 {
		t.Errorf("hand %d, want 1 card drawn", got)
	}
}

// Relic Amulet (SubCounter<All/CHARGE>, NumDmg$ SVar$CostCountersRemoved):
// every charge counter comes off and the damage is that many.
func TestActivationRemovesAllCountersAndReadsHowMany(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	amulet := g.NewCard(corpusCard(t, "Relic Amulet"), p, engine.Battlefield)
	g.Card(amulet).Counters.Add("CHARGE", 3)
	giant := g.NewCard(corpusCard(t, "Hill Giant"), other, engine.Battlefield)
	g.Player(p).ManaPool.Add(mana.Red, 2)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(giant)})
	c.QueuePayGeneric(mana.ShardR)
	c.QueuePayGeneric(mana.ShardR)
	index := len(g.Card(amulet).Def.Faces[0].Abilities) - 1
	if !g.ActivateAbility(p, amulet, index, c) {
		t.Fatalf("activation failed: %v", g.TakePendingError())
	}
	if got := g.Card(amulet).Counters.Count("CHARGE"); got != 0 {
		t.Errorf("charge counters = %d, want 0", got)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Card(giant).Zone; got != engine.Graveyard {
		t.Errorf("Hill Giant in %v, want dealt 3 and dead", got)
	}
}

// Silkbind Faerie ({1}{W/U}, {Q}): the untap symbol untaps the source as the
// cost, so a tapped Faerie can activate and an untapped one cannot.
func TestActivationUntapSymbol(t *testing.T) {
	t.Parallel()
	for _, tapped := range []bool{false, true} {
		g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		faerie := g.NewCard(corpusCard(t, "Silkbind Faerie"), p, engine.Battlefield)
		g.Card(faerie).SummonSick = false
		g.Card(faerie).Tapped = tapped
		bears := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Battlefield)
		g.Player(p).ManaPool.Add(mana.White, 2)
		c := engine.NewScriptedController()
		c.QueueTargets([]engine.EntityID{engine.CardEntity(bears)})
		c.QueueHybridManaColor(mana.White)
		c.QueuePayGeneric(mana.ShardW)
		got := g.ActivateAbility(p, faerie, 0, c)
		if got != tapped {
			t.Fatalf("tapped %v: activated %v (%v)", tapped, got, g.TakePendingError())
		}
		if tapped && g.Card(faerie).Tapped {
			t.Error("the Faerie is still tapped, want untapped by {Q}")
		}
	}
}

// Skyscribing's Forecast (SpellAbilityEffect.buildSpellAbility): activated
// from the hand during its controller's upkeep, once each turn, revealing
// itself; not in a main phase.
func TestForecastActivatesFromHandDuringUpkeepOnce(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 4)
	libraryCards(t, g, other, 4)
	scribe := g.NewCard(corpusCard(t, "Skyscribing"), p, engine.Hand)
	index := len(g.Card(scribe).Def.Faces[0].Abilities) - 1
	c := engine.NewScriptedController()
	pay := func() {
		g.Player(p).ManaPool.Add(mana.Blue, 3)
		c.QueuePayGeneric(mana.ShardU)
		c.QueuePayGeneric(mana.ShardU)
	}

	g.SetTurnState(1, p, engine.Main1)
	pay()
	if g.ActivateAbility(p, scribe, index, c) {
		t.Fatal("Forecast activated in a main phase")
	}
	g.Player(p).ManaPool = engine.Pool{}

	g.SetTurnState(1, p, engine.Upkeep)
	// The main-phase attempt was refused before paying: its queued payments
	// are still there for this one.
	g.Player(p).ManaPool.Add(mana.Blue, 3)
	if !g.ActivateAbility(p, scribe, index, c) {
		t.Fatalf("Forecast failed in the upkeep: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := len(g.Zone(engine.Hand, other).Cards()); got != 1 {
		t.Errorf("opponent's hand %d, want 1 card drawn", got)
	}
	pay()
	if g.ActivateAbility(p, scribe, index, c) {
		t.Error("Forecast activated twice in one turn")
	}
}

// Guardian Archon (RevealChosen<Player>): payable only once a player was
// chosen, and revealing changes nothing (CostRevealChosen.canPay/payAsDecided).
func TestRevealChosenCostNeedsAChosenPlayer(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	archon := g.NewCard(corpusCard(t, "Guardian Archon"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	index := len(g.Card(archon).Def.Faces[0].Abilities) - 1
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(bears)})
	if g.ActivateAbility(p, archon, index, c) {
		t.Fatal("activated with no chosen player")
	}
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("an unpayable cost is a silent refusal, got %v", err)
	}
	g.Card(archon).Memory.SetChosenPlayer(other)
	if !g.ActivateAbility(p, archon, index, c) {
		t.Fatalf("activation failed: %v", g.TakePendingError())
	}
}

// Kyscu Drake (Sac<1/CARDNAME> Sac<1/Creature.namedSpitting Drake>): two Sac
// parts, each paid in turn; without the named creature nothing is sacrificed.
func TestActivationPaysTwoSacrificeParts(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	drake := g.NewCard(corpusCard(t, "Kyscu Drake"), p, engine.Battlefield)
	index := len(g.Card(drake).Def.Faces[0].Abilities) - 1
	c := engine.NewScriptedController()
	if g.ActivateAbility(p, drake, index, c) {
		t.Fatal("activated with no Spitting Drake")
	}
	if z := g.Card(drake).Zone; z != engine.Battlefield {
		t.Fatalf("Kyscu Drake is in %v after a refused activation", z)
	}
	spit := g.NewCard(corpusCard(t, "Spitting Drake"), p, engine.Battlefield)
	c.QueueSacrificeChoice([]engine.CardID{spit})
	if !g.ActivateAbility(p, drake, index, c) {
		t.Fatalf("activation failed: %v", g.TakePendingError())
	}
	for _, id := range []engine.CardID{drake, spit} {
		if z := g.Card(id).Zone; z != engine.Graveyard {
			t.Errorf("a sacrificed creature is in %v", z)
		}
	}
}

// Corpseweft (XMin$ 1, ExileFromGrave<X/Creature>): X cannot be announced
// below the floor, and the cards exiled are X of the graveyard's creatures.
func TestActivationHonoursXMinParam(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	weft := g.NewCard(corpusCard(t, "Corpseweft"), p, engine.Battlefield)
	dead := []engine.CardID{
		g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Graveyard),
		g.NewCard(corpusCard(t, "Hill Giant"), p, engine.Graveyard),
	}
	g.Player(p).ManaPool.Add(mana.Black, 1)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	c := engine.NewScriptedController()
	c.QueuePayX(0)
	if g.ActivateAbility(p, weft, 0, c) {
		t.Fatal("activated with X 0 under XMin$ 1")
	}
	c.QueuePayX(2)
	c.QueueCardChoice(dead)
	c.QueuePayGeneric(mana.ShardG)
	if !g.ActivateAbility(p, weft, 0, c) {
		t.Fatalf("activation failed: %v", g.TakePendingError())
	}
	for _, id := range dead {
		if z := g.Card(id).Zone; z != engine.Exile {
			t.Errorf("a creature card is in %v, want exiled", z)
		}
	}
}

// Hourglass of the Lost (T SubCounter<X/TIME> Exile<1/CARDNAME>): the source
// itself is exiled as part of the cost.
func TestActivationExilesItsOwnSource(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	glass := g.NewCard(corpusCard(t, "Hourglass of the Lost"), p, engine.Battlefield)
	g.Card(glass).Counters.Add("TIME", 2)
	c := engine.NewScriptedController()
	c.QueuePayX(2)
	if !g.ActivateAbility(p, glass, 1, c) {
		t.Fatalf("activation failed: %v", g.TakePendingError())
	}
	if z := g.Card(glass).Zone; z != engine.Exile {
		t.Errorf("Hourglass is in %v, want exiled by its cost", z)
	}
	if got := g.Card(glass).Counters.Count("TIME"); got != 0 {
		t.Errorf("%d time counters left, want 0", got)
	}
}

// Halo Fountain (W T untapYType<1/Creature.YouCtrl>): a tapped creature is
// untapped as the cost; with none tapped the cost cannot be paid.
func TestActivationUntapsAChosenCreature(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	fountain := g.NewCard(corpusCard(t, "Halo Fountain"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	g.Player(p).ManaPool.Add(mana.White, 1)
	c := engine.NewScriptedController()
	if g.ActivateAbility(p, fountain, 0, c) {
		t.Fatal("activated with no tapped creature")
	}
	g.Card(bears).Tapped = true
	c.QueueCardChoice([]engine.CardID{bears})
	if !g.ActivateAbility(p, fountain, 0, c) {
		t.Fatalf("activation failed: %v", g.TakePendingError())
	}
	if g.Card(bears).Tapped {
		t.Error("the Bears are still tapped")
	}
}

// Illuminated Folio (1 T Reveal<2/SameColor>): two hand cards sharing a color
// pay it; two of different colors do not.
func TestActivationRevealsSameColorCards(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	libraryCards(t, g, p, 3)
	folio := g.NewCard(corpusCard(t, "Illuminated Folio"), p, engine.Battlefield)
	g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
	g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.White, 1)
	c := engine.NewScriptedController()
	if g.ActivateAbility(p, folio, 0, c) {
		t.Fatal("activated with a green and a red card")
	}
	g.NewCard(corpusCard(t, "Giant Growth"), p, engine.Hand)
	c.QueuePayGeneric(mana.ShardW)
	if !g.ActivateAbility(p, folio, 0, c) {
		t.Fatalf("activation failed: %v", g.TakePendingError())
	}
}

// The Capitoline Triad (ExileFromGrave<1+/Card.Historic+withTotalCMCGE30>):
// historic graveyard cards totalling 30 mana value are exiled.
func TestActivationExilesGraveCardsReachingATotalManaValue(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	triad := g.NewCard(corpusCard(t, "The Capitoline Triad"), p, engine.Battlefield)
	index := len(g.Card(triad).Def.Faces[0].Abilities) - 1
	c := engine.NewScriptedController()
	little := g.NewCard(corpusCard(t, "Sol Ring"), p, engine.Graveyard)
	if g.ActivateAbility(p, triad, index, c) {
		t.Fatal("activated with a graveyard worth 1 mana value")
	}
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("unpayable total should refuse silently: %v", err)
	}
	big := g.NewCard(corpusCard(t, "Emrakul, the Aeons Torn"), p, engine.Graveyard)
	big2 := g.NewCard(corpusCard(t, "Emrakul, the Aeons Torn"), p, engine.Graveyard)
	c.QueueCardChoice([]engine.CardID{big, big2})
	if !g.ActivateAbility(p, triad, index, c) {
		t.Fatalf("activation failed: %v", g.TakePendingError())
	}
	for _, id := range []engine.CardID{big, big2} {
		if z := g.Card(id).Zone; z != engine.Exile {
			t.Errorf("an Emrakul is in %v, want exiled", z)
		}
	}
	if z := g.Card(little).Zone; z != engine.Graveyard {
		t.Errorf("Sol Ring is in %v, want left alone", z)
	}
}

// Night Soil (1 ExileSameGrave<2/Creature>) takes both creature cards from one
// graveyard; Thelon of Havenwood (B G ExileAnyGrave<1/Fungus>) takes one from
// any graveyard.
func TestActivationExilesFromOneOrAnyGraveyard(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	soil := g.NewCard(corpusCard(t, "Night Soil"), p, engine.Battlefield)
	mine := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Graveyard)
	theirs := g.NewCard(corpusCard(t, "Hill Giant"), other, engine.Graveyard)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	c := engine.NewScriptedController()
	if g.ActivateAbility(p, soil, 0, c) {
		t.Fatal("activated with one creature card in each graveyard")
	}
	pair := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Graveyard)
	c.QueuePlayerChoice(other)
	c.QueueCardChoice([]engine.CardID{theirs, pair})
	c.QueuePayGeneric(mana.ShardG)
	if !g.ActivateAbility(p, soil, 0, c) {
		t.Fatalf("Night Soil: %v", g.TakePendingError())
	}
	if g.Card(theirs).Zone != engine.Exile || g.Card(pair).Zone != engine.Exile || g.Card(mine).Zone != engine.Graveyard {
		t.Error("Night Soil did not exile exactly the two cards of the other graveyard")
	}

	thelon := g.NewCard(corpusCard(t, "Thelon of Havenwood"), p, engine.Battlefield)
	shroom := g.NewCard(corpusCard(t, "Thallid"), other, engine.Graveyard)
	g.Player(p).ManaPool.Add(mana.Black, 1)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	c.QueueCardChoice([]engine.CardID{shroom})
	if !g.ActivateAbility(p, thelon, 0, c) {
		t.Fatalf("Thelon: %v", g.TakePendingError())
	}
	if z := g.Card(shroom).Zone; z != engine.Exile {
		t.Errorf("the Fungus is in %v, want exiled", z)
	}
}

// Fabrication Foundry (Announce$ X, Exile<1+/Artifact.Other+withTotalCMCEQX>):
// other artifacts whose mana values add up to the announced X are exiled.
func TestActivationExilesPermanentsTotallingTheAnnouncedX(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	foundry := g.NewCard(corpusCard(t, "Fabrication Foundry"), p, engine.Battlefield)
	ring := g.NewCard(corpusCard(t, "Sol Ring"), p, engine.Battlefield)
	g.NewCard(corpusCard(t, "Mind Stone"), p, engine.Battlefield)
	target := g.NewCard(corpusCard(t, "Sol Ring"), p, engine.Graveyard)
	g.Player(p).ManaPool.Add(mana.White, 3)
	c := engine.NewScriptedController()
	c.QueuePayX(1)
	c.QueueCardChoice([]engine.CardID{ring})
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
	queueGeneric(c, mana.ShardW, 2)
	if !g.ActivateAbility(p, foundry, 1, c) {
		t.Fatalf("activation failed: %v", g.TakePendingError())
	}
	if z := g.Card(ring).Zone; z != engine.Exile {
		t.Errorf("Sol Ring (mana value 1) is in %v, want exiled for X 1", z)
	}
}

// Ooze Flux (XMin1, RemoveAnyCounter<X1+/P1P1/Creature>): X counters come off
// the creatures the payer picks and the token is X/X.
func TestActivationRemovesAnnouncedCountersFromCreatures(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	flux := g.NewCard(corpusCard(t, "Ooze Flux"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	g.Card(bears).Counters.Add(engine.P1P1, 3)
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c := engine.NewScriptedController()
	c.QueuePayX(2)
	c.QueueCardChoice([]engine.CardID{bears})
	c.QueueCardChoice([]engine.CardID{bears})
	c.QueuePayGeneric(mana.ShardG)
	if !g.ActivateAbility(p, flux, 0, c) {
		t.Fatalf("activation failed: %v", g.TakePendingError())
	}
	if got := g.Card(bears).Counters.Count(engine.P1P1); got != 1 {
		t.Errorf("%d +1/+1 counters left, want 1", got)
	}
}

// Jandor's Ring (2 T Discard<1/LastDrawn>): only the card drawn last can be
// discarded, and only while it is still in hand.
func TestActivationDiscardsTheLastDrawnCard(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	libraryCards(t, g, p, 4)
	ring := g.NewCard(corpusCard(t, "Jandor's Ring"), p, engine.Battlefield)
	g.Player(p).ManaPool.Add(mana.White, 2)
	c := engine.NewScriptedController()
	if g.ActivateAbility(p, ring, 0, c) {
		t.Fatal("activated before any card was drawn")
	}
	g.DrawCards(p, 1, c)
	hand := g.Zone(engine.Hand, p).Cards()
	drawn := hand[len(hand)-1]
	queueGeneric(c, mana.ShardW, 2)
	if !g.ActivateAbility(p, ring, 0, c) {
		t.Fatalf("activation failed: %v", g.TakePendingError())
	}
	if z := g.Card(drawn).Zone; z != engine.Graveyard {
		t.Errorf("the drawn card is in %v, want discarded", z)
	}
}

// Skeleton Scavengers (Mana<1\NumTimes>, NumTimes = its +1/+1 counters): {1}
// is paid once for each counter.
func TestActivationPaysManaOncePerNumTimes(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	skeleton := g.NewCard(corpusCard(t, "Skeleton Scavengers"), p, engine.Battlefield)
	g.Card(skeleton).Counters.Add(engine.P1P1, 2)
	g.Player(p).ManaPool.Add(mana.Black, 5)
	c := engine.NewScriptedController()
	queueGeneric(c, mana.ShardB, 2)
	if !g.ActivateAbility(p, skeleton, 0, c) {
		t.Fatalf("activation failed: %v", g.TakePendingError())
	}
	if got := g.Player(p).ManaPool.Total(); got != 3 {
		t.Errorf("%d mana left, want 3 (two paid)", got)
	}
}

// Back from the Brink (ExileFromGrave<1/Creature> Mana<0\Exiled>): the
// creature card exiled is also paid for with its mana cost.
func TestActivationPaysTheExiledCardsManaCost(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	brink := g.NewCard(corpusCard(t, "Back from the Brink"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Graveyard)
	g.Player(p).ManaPool.Add(mana.Green, 4)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{bears})
	queueGeneric(c, mana.ShardG, 2)
	if !g.ActivateAbility(p, brink, 0, c) {
		t.Fatalf("activation failed: %v", g.TakePendingError())
	}
	if got := g.Player(p).ManaPool.Total(); got != 2 {
		t.Errorf("%d mana left, want 2 (Grizzly Bears costs {1}{G})", got)
	}
}

// Clay Golem (6 RollDice<1/8/X>, Monstrosity$ X): the d8 rolled while paying
// is X, and Monstrosity X puts that many +1/+1 counters on the Golem.
func TestActivationRollsADieForX(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	golem := g.NewCard(corpusCard(t, "Clay Golem"), p, engine.Battlefield)
	g.Player(p).ManaPool.Add(mana.White, 6)
	c := engine.NewScriptedController()
	queueGeneric(c, mana.ShardW, 6)
	c.QueueTargets([]engine.EntityID{engine.CardEntity(g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield))})
	if !g.ActivateAbility(p, golem, 0, c) {
		t.Fatalf("activation failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if n := g.Card(golem).Counters.Count(engine.P1P1); n < 1 || n > 8 {
		t.Errorf("%d +1/+1 counters, want the result of one d8 (1-8)", n)
	}
}

// Martyr of Bones (Reveal<X/Card.Black>, X the number of targets): the cost is
// read after the targets are chosen.
func TestActivationReadsXFromTheTargets(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	martyr := g.NewCard(corpusCard(t, "Martyr of Bones"), p, engine.Battlefield)
	g.NewCard(corpusCard(t, "Dark Ritual"), p, engine.Hand)
	g.NewCard(corpusCard(t, "Dark Ritual"), p, engine.Hand)
	a := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Graveyard)
	b := g.NewCard(corpusCard(t, "Hill Giant"), other, engine.Graveyard)
	g.Player(p).ManaPool.Add(mana.Black, 1)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(a), engine.CardEntity(b)})
	c.QueuePayGeneric(mana.ShardB)
	if !g.ActivateAbility(p, martyr, 0, c) {
		t.Fatalf("activation failed: %v", g.TakePendingError())
	}
	if z := g.Card(martyr).Zone; z != engine.Graveyard {
		t.Errorf("Martyr of Bones is in %v, want sacrificed", z)
	}
}

// Nivmagus Elemental (ExileFromStack<1/Spell.Instant+YouCtrl;Spell.Sorcery+YouCtrl>):
// the controller's own instant leaves the stack for exile uncountered; with no
// such spell the ability cannot be activated.
func TestActivationExilesASpellFromTheStack(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	elemental := g.NewCard(corpusCard(t, "Nivmagus Elemental"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	if g.ActivateAbility(p, elemental, 0, c) {
		t.Fatal("activated with no spell on the stack")
	}
	bolt := g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 1)
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	if !g.CastSpell(p, bolt, c) {
		t.Fatalf("cast failed: %v", g.TakePendingError())
	}
	c.QueueCardChoice([]engine.CardID{bolt})
	if !g.ActivateAbility(p, elemental, 0, c) {
		t.Fatalf("activation failed: %v", g.TakePendingError())
	}
	if z := g.Card(bolt).Zone; z != engine.Exile {
		t.Errorf("the Bolt is in %v, want exiled", z)
	}
	if got := g.Card(elemental).Counters.Count("P1P1"); got != 0 {
		t.Errorf("%d counters before resolution, want 0", got)
	}
}

// Sunforger (R W Unattach<CARDNAME/CARDNAME>): the Equipment unattaches
// itself as the cost; unattached it cannot pay it.
func TestActivationUnattachesItsOwnEquipment(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	sunforger := g.NewCard(corpusCard(t, "Sunforger"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	g.Player(p).ManaPool.Add(mana.Red, 1)
	g.Player(p).ManaPool.Add(mana.White, 1)
	if g.ActivateAbility(p, sunforger, 0, c) {
		t.Fatal("activated while unattached")
	}
	g.Attach(sunforger, bears)
	if !g.ActivateAbility(p, sunforger, 0, c) {
		t.Fatalf("activation failed: %v", g.TakePendingError())
	}
	if got := g.Card(bears).Attachments(); len(got) != 0 {
		t.Errorf("Bears still carry %v, want the Sunforger unattached", got)
	}
}

// Heartseeker (granted "{T}, Unattach CARDNAME": Unattach<OriginalHost>): the
// Equipment that granted the ability unattaches as the cost.
func TestActivationUnattachesTheGrantingEquipment(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	seeker := g.NewCard(corpusCard(t, "Heartseeker"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	victim := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Battlefield)
	g.Card(bears).SummonSick = false
	g.Attach(seeker, bears)
	sba(g)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(victim)})
	if !g.ActivateAbility(p, bears, 0, c) {
		t.Fatalf("activation failed: %v", g.TakePendingError())
	}
	if got := g.Card(bears).Attachments(); len(got) != 0 {
		t.Errorf("Bears still carry %v, want the Heartseeker unattached", got)
	}
	if !g.Card(bears).Tapped {
		t.Error("the Bears did not tap for the cost")
	}
}
