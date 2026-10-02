package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// ADR-0038: a printed Equip, Cycling, Prowess or Exalted line compiles into the
// traits CardFactoryUtil builds for it, and the engine plays them.

// Equip is sorcery speed (CR 702.6a): on the opponent's turn the ability cannot
// be activated; on its own turn it attaches, and re-equipping moves it.
func TestEquipAttachesAtSorcerySpeedAndMoves(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	sword := g.NewCard(corpusCard(t, "Bonesplitter"), p, engine.Battlefield)
	first := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	second := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)

	equip := func(target engine.CardID) bool {
		g.Player(p).ManaPool.Add(mana.Red, 1)
		c := engine.NewScriptedController()
		c.QueuePayGeneric(mana.ShardR)
		c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
		if !g.ActivateAbility(p, sword, 0, c) {
			return false
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		return true
	}

	g.SetTurnState(1, other, engine.Main1)
	if equip(first) {
		t.Fatal("Equip activated on the opponent's turn")
	}
	g.Player(p).ManaPool = engine.Pool{}
	g.SetTurnState(2, p, engine.Main1)
	if !equip(first) {
		t.Fatal("Equip could not be activated at sorcery speed")
	}
	if got, _ := g.Card(sword).AttachedTo(); got != first {
		t.Fatalf("Bonesplitter attached to %v, want the first Bears", got)
	}
	if !equip(second) {
		t.Fatal("re-equip failed")
	}
	if got, _ := g.Card(sword).AttachedTo(); got != second {
		t.Errorf("Bonesplitter attached to %v after re-equip, want the second Bears", got)
	}
	sba(g)
	if power, _ := g.Card(second).Power(); power != 4 {
		t.Errorf("equipped Bears power = %d, want 4", power)
	}
}

// Cycling (CR 702.29a): pay the cost, discard the card from hand, draw a card.
func TestCyclingDiscardsAndDraws(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	cycler := g.NewCard(corpusCard(t, "Barkhide Mauler"), p, engine.Hand)
	top := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Library)
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardG)
	c.QueuePayGeneric(mana.ShardG)
	if !g.ActivateAbility(p, cycler, 0, c) {
		t.Fatal("cycling could not be activated from hand")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(cycler).Zone != engine.Graveyard || g.Card(top).Zone != engine.Hand {
		t.Errorf("cycler in %v, drawn card in %v; want Graveyard and Hand", g.Card(cycler).Zone, g.Card(top).Zone)
	}
}

// Prowess (CR 702.108a): a noncreature spell cast by the controller gives +1/+1
// until end of turn.
func TestProwessPumpsOnANoncreatureSpell(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	monk := g.NewCard(corpusCard(t, "Monastery Swiftspear"), p, engine.Battlefield)
	bolt := g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 1)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(g.Players()[1])})
	if !g.CastSpell(p, bolt, c) {
		t.Fatal("cast failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if power, _ := g.Card(monk).Power(); power != 2 {
		t.Errorf("Swiftspear power = %d, want 2 after one prowess trigger", power)
	}
}

// Exalted (CR 702.83a): a creature attacking alone gets +1/+1 for each
// instance among permanents its controller controls.
func TestExaltedPumpsALoneAttacker(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	squire := g.NewCard(corpusCard(t, "Akrasan Squire"), p, engine.Battlefield)
	g.Card(squire).SummonSick = false
	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{squire})
	if _, err := g.DeclareCombatAttackers(ac); err != nil {
		t.Fatalf("DeclareCombatAttackers: %v", err)
	}
	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if power, _ := g.Card(squire).Power(); power != 2 {
		t.Errorf("Akrasan Squire power = %d, want 2", power)
	}
}

// Bushido (CR 702.45a): a creature that blocks or becomes blocked gets +N/+N.
func TestBushidoPumpsWhenBlocked(t *testing.T) {
	t.Parallel()

	g, a, b := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, a, engine.Main1)
	g.Player(a).Life, g.Player(b).Life = 20, 20
	samurai := g.NewCard(corpusCard(t, "Devoted Retainer"), a, engine.Battlefield)
	g.Card(samurai).SummonSick = false
	wall := g.NewCard(creatureDefPT(t, "0", "4"), b, engine.Battlefield)
	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{samurai})
	if _, err := g.DeclareCombatAttackers(ac); err != nil {
		t.Fatalf("DeclareCombatAttackers: %v", err)
	}
	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: wall, Attacker: samurai}})
	if _, err := g.DeclareCombatBlockers(bc); err != nil {
		t.Fatalf("DeclareCombatBlockers: %v", err)
	}
	if err := g.ResolveStack(engine.NewRegistry(), bc); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if power, _ := g.Card(samurai).Power(); power != 2 {
		t.Errorf("blocked Devoted Retainer power = %d, want 2", power)
	}
}

// Afterlife (CR 702.135a): when the creature dies, create that many 1/1 white
// and black flying Spirit tokens.
func TestAfterlifeCreatesSpirits(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	priest := g.NewCard(corpusCard(t, "Ministrant of Obligation"), p, engine.Battlefield)
	g.Card(priest).Damage.Mark(9, false)
	c := engine.NewScriptedController()
	sba(g)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	var spirits int
	for _, id := range g.Zone(engine.Battlefield, p).Cards() {
		if g.Card(id).IsToken && g.Card(id).Type().HasSubtype("Spirit") {
			spirits++
		}
	}
	if spirits != 2 {
		t.Errorf("Spirit tokens = %d, want 2", spirits)
	}
}

// Persist and Undying (CR 702.79a, 702.93a): a creature with no -1/-1 (+1/+1)
// counter that dies returns to the battlefield with one; with one it stays dead.
func TestPersistAndUndyingReturnOnce(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		card    string
		counter engine.CounterType
	}{
		{"persist", "Safehold Elite", engine.M1M1},
		{"undying", "Strangleroot Geist", engine.P1P1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
			creature := g.NewCard(corpusCard(t, tc.card), p, engine.Battlefield)
			c := engine.NewScriptedController()
			kill := func() {
				g.Card(creature).Damage.Mark(20, false)
				sba(g)
				if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
					t.Fatalf("ResolveStack: %v", err)
				}
			}
			find := func() (engine.CardID, bool) {
				for _, id := range g.Zone(engine.Battlefield, p).Cards() {
					if g.Card(id).Def == g.Card(creature).Def {
						return id, true
					}
				}
				return engine.NoCard, false
			}
			kill()
			back, ok := find()
			if !ok {
				t.Fatal("the creature did not return")
			}
			if n := g.Card(back).Counters.Count(tc.counter); n != 1 {
				t.Errorf("returned with %d counters, want 1", n)
			}
			creature = back
			kill()
			if _, ok := find(); ok {
				t.Error("the creature returned a second time with a counter on it")
			}
		})
	}
}

// Annihilator N (CR 702.86a): whenever the creature attacks, the defending
// player sacrifices N permanents.
func TestAnnihilatorMakesTheDefenderSacrifice(t *testing.T) {
	t.Parallel()

	g, a, b := newTwoPlayerGameOn(t, scenarioDB(t))
	crusher := g.NewCard(corpusCard(t, "Ulamog's Crusher"), a, engine.Battlefield)
	g.Card(crusher).SummonSick = false
	first := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)
	second := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)
	third := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueAttackers([]engine.CardID{crusher})
	c.QueueSacrificeChoice([]engine.CardID{first, second})
	if _, err := g.DeclareCombatAttackers(c); err != nil {
		t.Fatalf("DeclareCombatAttackers: %v", err)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(first).Zone != engine.Graveyard || g.Card(second).Zone != engine.Graveyard || g.Card(third).Zone != engine.Battlefield {
		t.Errorf("zones %v %v %v, want the first two sacrificed and the third kept",
			g.Card(first).Zone, g.Card(second).Zone, g.Card(third).Zone)
	}
}

// TypeCycling (CR 702.29e): pay the cost and discard the card to search the
// library for a card of the type and put it in hand.
func TestTypeCyclingFetchesALandOfTheType(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	dragon := g.NewCard(corpusCard(t, "Timeless Dragon"), p, engine.Hand)
	plains := g.NewCard(corpusCard(t, "Plains"), p, engine.Library)
	forest := g.NewCard(corpusCard(t, "Forest"), p, engine.Library)
	g.Player(p).ManaPool.Add(mana.White, 2)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardW)
	c.QueuePayGeneric(mana.ShardW)
	c.QueueCardChoice([]engine.CardID{plains})
	if !g.ActivateAbility(p, dragon, 0, c) {
		t.Fatal("Plainscycling could not be activated from hand")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(dragon).Zone != engine.Graveyard || g.Card(plains).Zone != engine.Hand || g.Card(forest).Zone != engine.Library {
		t.Errorf("dragon %v, Plains %v, Forest %v; want Graveyard, Hand, Library",
			g.Card(dragon).Zone, g.Card(plains).Zone, g.Card(forest).Zone)
	}
}

// Flashback (CR 702.34a): the owner may cast the card from their graveyard for
// its flashback cost, and it is exiled as it leaves the stack. Firebolt is
// {R} sorcery, flashback {4}{R}, 2 damage to any target.
func TestFlashbackCastsFromTheGraveyardAndExiles(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	bolt := g.NewCard(corpusCard(t, "Firebolt"), p, engine.Graveyard)
	plain := g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Graveyard)
	g.Player(p).ManaPool.Add(mana.Red, 5)
	c := engine.NewScriptedController()
	for range 4 {
		c.QueuePayGeneric(mana.ShardR)
	}
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	if g.CastSpell(p, plain, c) {
		t.Fatal("a graveyard card without flashback was cast")
	}
	if !g.CastSpell(p, bolt, c) {
		t.Fatal("Firebolt could not be flashed back")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(bolt).Zone != engine.Exile {
		t.Errorf("flashed-back Firebolt in %v, want Exile", g.Card(bolt).Zone)
	}
	if got := g.Player(other).Life; got != 18 {
		t.Errorf("target life = %d, want 18", got)
	}
}

// Crew N (CR 702.122a): tap any number of other untapped creatures with total
// power N or more to make the Vehicle an artifact creature until end of turn.
func TestCrewTapsCreaturesAndAnimatesTheVehicle(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	copter := g.NewCard(corpusCard(t, "Smuggler's Copter"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	sba(g)
	if g.Card(copter).Type().Has(cardtype.Creature) {
		t.Fatal("the Vehicle is a creature before it is crewed")
	}
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{bears})
	if !g.ActivateAbility(p, copter, 0, c) {
		t.Fatal("Crew could not be activated")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	sba(g)
	if !g.Card(bears).Tapped {
		t.Error("the crewing creature was not tapped")
	}
	if !g.Card(copter).Type().Has(cardtype.Creature) {
		t.Error("the crewed Vehicle is not a creature")
	}
}

// Crew needs enough total power: a creature with less power than the Crew
// number cannot crew alone.
func TestCrewRefusesInsufficientPower(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	cart := g.NewCard(corpusCard(t, "Smuggler's Copter"), p, engine.Battlefield)
	weak := g.NewCard(creatureDefPT(t, "0", "1"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{weak})
	if g.ActivateAbility(p, cart, 0, c) {
		t.Error("Crew 1 was paid with a 0-power creature")
	}
}
