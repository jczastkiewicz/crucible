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
