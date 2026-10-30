package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Cards the smoke probe cannot cast on its fixed board (smoke-probe.md,
// inconclusive): each is cast here with the board it needs.

// Detonate destroys the artifact whose mana value is X and deals X damage to
// its controller.
func TestDetonateDestroysTheArtifactWithManaValueX(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	art := g.NewCard(artifactDefManaCost(t, "3"), other, engine.Battlefield)
	spell := g.NewCard(corpusCard(t, "Detonate"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 4)
	c := engine.NewScriptedController()
	c.QueuePayX(3)
	for range 3 {
		c.QueuePayGeneric(mana.ShardR)
	}
	c.QueueTargets([]engine.EntityID{engine.CardEntity(art)})
	castThenResolve(t, g, p, spell, c)
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	if got := g.Card(art).Zone; got != engine.Graveyard {
		t.Errorf("artifact in %v, want Graveyard", got)
	}
	if got := g.Player(other).Life; got != 17 {
		t.Errorf("life = %d, want 17 (X = 3 damage)", got)
	}
}

// Blazing Hope exiles a creature whose power is at least its caster's life
// total.
func TestBlazingHopeExilesAPowerfulCreature(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life = 4
	big := g.NewCard(creatureDefPT(t, "5", "5"), other, engine.Battlefield)
	spell := g.NewCard(corpusCard(t, "Blazing Hope"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.White, 1)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(big)})
	castThenResolve(t, g, p, spell, c)
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	if got := g.Card(big).Zone; got != engine.Exile {
		t.Errorf("creature in %v, want Exile", got)
	}
}

// Selective Snare returns X target creatures of the chosen creature type.
func TestSelectiveSnareReturnsXCreaturesOfTheChosenType(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	elf1 := g.NewCard(amountDef(t, "Test Elf One", "Creature Elf", "G", "1", "1", nil), other, engine.Battlefield)
	elf2 := g.NewCard(amountDef(t, "Test Elf Two", "Creature Elf", "G", "1", "1", nil), other, engine.Battlefield)
	bear := g.NewCard(amountDef(t, "Test Bear", "Creature Bear", "G", "2", "2", nil), other, engine.Battlefield)
	spell := g.NewCard(corpusCard(t, "Selective Snare"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Blue, 3)
	c := engine.NewScriptedController()
	c.QueuePayX(2)
	c.QueuePayGeneric(mana.ShardU)
	c.QueuePayGeneric(mana.ShardU)
	c.QueueOptionNamed("Elf")
	c.QueueTargets([]engine.EntityID{engine.CardEntity(elf1), engine.CardEntity(elf2)})
	castThenResolve(t, g, p, spell, c)
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	for _, id := range []engine.CardID{elf1, elf2} {
		if got := g.Card(id).Zone; got != engine.Hand {
			t.Errorf("elf %d in %v, want Hand", id, got)
		}
	}
	if got := g.Card(bear).Zone; got != engine.Battlefield {
		t.Errorf("bear in %v, want Battlefield", got)
	}
}

// Liquid Fire: the caster announces a number from 0 to 5 as it is cast; the
// creature takes that much damage and its controller 5 minus it.
func TestLiquidFireAnnouncesItsNumber(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	victim := g.NewCard(creatureDefPT(t, "1", "9"), other, engine.Battlefield)
	spell := g.NewCard(corpusCard(t, "Liquid Fire"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 6)
	c := engine.NewScriptedController()
	c.QueueNumberChoice(2)
	for range 5 {
		c.QueuePayGeneric(mana.ShardR)
	}
	c.QueueTargets([]engine.EntityID{engine.CardEntity(victim)})
	castThenResolve(t, g, p, spell, c)
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	// 2 to the creature, 5 - 2 = 3 to its controller: five damage in all.
	if got := g.Card(victim).Damage.Marked; got != 2 {
		t.Errorf("creature has %d damage, want 2", got)
	}
	if got := g.Player(other).Life; got != 17 {
		t.Errorf("controller has %d life, want 17", got)
	}
}

// Aphetto Dredging returns up to three creature cards of the announced type
// from the caster's graveyard to their hand.
func TestAphettoDredgingReturnsCreaturesOfTheAnnouncedType(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	elf := g.NewCard(amountDef(t, "Test Elf", "Creature Elf", "G", "1", "1", nil), p, engine.Graveyard)
	bear := g.NewCard(amountDef(t, "Test Bear", "Creature Bear", "G", "2", "2", nil), p, engine.Graveyard)
	spell := g.NewCard(corpusCard(t, "Aphetto Dredging"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Black, 4)
	c := engine.NewScriptedController()
	for range 3 {
		c.QueuePayGeneric(mana.ShardB)
	}
	c.QueueOptionNamed("Elf")
	c.QueueTargets([]engine.EntityID{engine.CardEntity(elf)})
	castThenResolve(t, g, p, spell, c)
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	if got := g.Card(elf).Zone; got != engine.Hand {
		t.Errorf("elf in %v, want Hand", got)
	}
	if got := g.Card(bear).Zone; got != engine.Graveyard {
		t.Errorf("bear in %v, want Graveyard", got)
	}
}

// Spoils of War: its X is the chosen opponent's creature and artifact cards in
// their graveyard, not announced, and the spell distributes that many +1/+1
// counters among its targets.
func TestSpoilsOfWarReadsXFromTheChosenOpponentsGraveyard(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(amountDef(t, "Test Elf", "Creature Elf", "G", "1", "1", nil), other, engine.Graveyard)
	g.NewCard(amountDef(t, "Test Bear", "Creature Bear", "G", "2", "2", nil), other, engine.Graveyard)
	mine := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	spell := g.NewCard(corpusCard(t, "Spoils of War"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Black, 3)
	c := engine.NewScriptedController()
	c.QueuePlayerChoice(other)
	for range 2 {
		c.QueuePayGeneric(mana.ShardB)
	}
	c.QueueTargets([]engine.EntityID{engine.CardEntity(mine)})
	castThenResolve(t, g, p, spell, c)
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("%d mana left, want 0 (X = 2)", got)
	}
	if got := g.Card(mine).Counters.Count(engine.P1P1); got != 2 {
		t.Errorf("creature has %d +1/+1 counters, want 2", got)
	}
}

// Bargaining Table announces an opponent as it is activated; its X is the
// cards in that opponent's hand.
func TestBargainingTableReadsXFromTheAnnouncedOpponentsHand(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	table := g.NewCard(corpusCard(t, "Bargaining Table"), p, engine.Battlefield)
	libraryCards(t, g, p, 3)
	for range 2 {
		g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Hand)
	}
	g.Player(p).ManaPool.AddColorless(2)
	c := engine.NewScriptedController()
	c.QueuePlayerChoice(other)
	c.QueuePayGeneric(mana.ShardC)
	c.QueuePayGeneric(mana.ShardC)
	if !g.ActivateAbility(p, table, 0, c) {
		t.Fatalf("ActivateAbility returned false (pending error: %v)", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("%d mana left, want 0 (X = 2)", got)
	}
	if got := len(g.Zone(engine.Hand, p).Cards()); got != 1 {
		t.Errorf("hand has %d cards, want 1 (the draw)", got)
	}
}

// Myr Superion (Mode$ ManaRestriction, ValidSource$ Creature): only mana
// produced by creatures casts it.
func TestMyrSuperionSpendsOnlyCreatureMana(t *testing.T) {
	t.Parallel()

	for _, fromCreatures := range []bool{false, true} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main2)
		c := engine.NewScriptedController()
		if fromCreatures {
			for range 2 {
				elf := g.NewCard(corpusCard(t, "Llanowar Elves"), p, engine.Battlefield)
				g.Card(elf).SummonSick = false
				if !g.ActivateManaAbility(p, elf, 0, c) {
					t.Fatalf("Llanowar Elves did not tap for mana: %v", g.TakePendingError())
				}
			}
		} else {
			g.Player(p).ManaPool.Add(mana.Green, 2)
		}
		superion := g.NewCard(corpusCard(t, "Myr Superion"), p, engine.Hand)
		for range 2 {
			c.QueuePayGeneric(mana.ShardG)
		}
		if got := g.CastSpell(p, superion, c); got != fromCreatures {
			t.Errorf("from creatures %v: cast %v (%v)", fromCreatures, got, g.TakePendingError())
		}
	}
}

// Ceremonious Rejection counters a colorless spell.
func TestCeremoniousRejectionCountersAColorlessSpell(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, other, engine.Main1)
	thopter := g.NewCard(corpusCard(t, "Ornithopter"), other, engine.Hand)
	oc := engine.NewScriptedController()
	if !g.CastSpell(other, thopter, oc) {
		t.Fatal("the opponent could not cast Ornithopter")
	}
	spell := g.NewCard(corpusCard(t, "Ceremonious Rejection"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Blue, 1)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(thopter)})
	castThenResolve(t, g, p, spell, c)
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	if got := g.Card(thopter).Zone; got != engine.Graveyard {
		t.Errorf("Ornithopter in %v, want Graveyard: countered", got)
	}
}

// Haakon can be cast from the graveyard and from nowhere else.
func TestHaakonIsCastOnlyFromTheGraveyard(t *testing.T) {
	t.Parallel()

	for _, zone := range []engine.ZoneType{engine.Hand, engine.Graveyard} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		haakon := g.NewCard(corpusCard(t, "Haakon, Stromgald Scourge"), p, zone)
		sba(g)
		g.Player(p).ManaPool.Add(mana.Black, 3)
		c := engine.NewScriptedController()
		for range 3 {
			c.QueuePayGeneric(mana.ShardB)
		}
		if got := g.CastSpell(p, haakon, c); got != (zone == engine.Graveyard) {
			t.Errorf("cast from %v = %v (%v)", zone, got, g.TakePendingError())
		}
	}
}

// Nahiri's Sacrifice sacrifices a creature or artifact with mana value X as
// part of its cost and deals X damage divided among its targets.
func TestNahirisSacrificeSacrificesForX(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	fodder := g.NewCard(amountDef(t, "Test Fodder", "Creature Elf", "1 G", "1", "1", nil), p, engine.Battlefield)
	victim := g.NewCard(creatureDefPT(t, "1", "5"), other, engine.Battlefield)
	spell := g.NewCard(corpusCard(t, "Nahiri's Sacrifice"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 6)
	c := engine.NewScriptedController()
	c.QueuePayX(2)
	c.QueuePayGeneric(mana.ShardR)
	c.QueueSacrificeChoice([]engine.CardID{fodder})
	c.QueueTargets([]engine.EntityID{engine.CardEntity(victim)})
	castThenResolve(t, g, p, spell, c)
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	if got := g.Card(fodder).Zone; got != engine.Graveyard {
		t.Errorf("fodder in %v, want Graveyard", got)
	}
	if got := g.Card(victim).Damage.Marked; got != 2 {
		t.Errorf("creature has %d damage, want 2", got)
	}
}

// Reverberation redirects all damage a target sorcery spell would deal to that
// spell's controller.
func TestReverberationRedirectsASorcerysDamage(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, other, engine.Main1)
	blaze := g.NewCard(corpusCard(t, "Blaze"), other, engine.Hand)
	g.Player(other).ManaPool.Add(mana.Red, 3)
	oc := engine.NewScriptedController()
	oc.QueuePayX(2)
	oc.QueuePayGeneric(mana.ShardR)
	oc.QueuePayGeneric(mana.ShardR)
	oc.QueueTargets([]engine.EntityID{engine.PlayerEntity(p)})
	if !g.CastSpell(other, blaze, oc) {
		t.Fatalf("the opponent could not cast Blaze: %v", g.TakePendingError())
	}
	rev := g.NewCard(corpusCard(t, "Reverberation"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Blue, 4)
	c := engine.NewScriptedController()
	for range 3 {
		c.QueuePayGeneric(mana.ShardU)
	}
	c.QueueTargets([]engine.EntityID{engine.CardEntity(blaze)})
	if !g.CastSpell(p, rev, c) {
		t.Fatalf("could not cast Reverberation: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	if got := g.Player(p).Life; got != 20 {
		t.Errorf("caster life = %d, want 20", got)
	}
	if got := g.Player(other).Life; got != 18 {
		t.Errorf("Blaze's controller life = %d, want 18 (2 damage redirected)", got)
	}
}
