package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// attackAlone declares attacker against the next queued target and resolves
// the attack triggers.
func attackAlone(t *testing.T, g *engine.Game, c *engine.ScriptedController, attacker engine.CardID, target engine.EntityID) {
	t.Helper()
	c.QueueAttackers([]engine.CardID{attacker})
	c.QueueAttackTarget(target)
	declareAttackers(t, g, c)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
}

// Vanishing (CR 702.63, CardFactoryUtil.java:1910-1930, :2529-2538): Ravaging
// Riftwurm enters with two time counters, loses one each upkeep and is
// sacrificed as the last one goes.
func TestVanishingCountsDownAndSacrifices(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 5)
	libraryCards(t, g, other, 5)
	g.SetTurnState(1, p, engine.Main1)
	wurm := g.NewCard(corpusCard(t, "Ravaging Riftwurm"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 3)
	c := engine.NewScriptedController()
	c.QueueConfirmPayCost(false)
	c.QueuePayGeneric(mana.ShardG)
	if !g.CastSpell(p, wurm, c) {
		t.Fatalf("cast failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Card(wurm).Counters.Count(engine.Time); got != 2 {
		t.Fatalf("entered with %d time counters, want 2", got)
	}
	for turn, want := range []int{1, 0} {
		toStep(t, g, c, p, engine.Upkeep, nil)
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatal(err)
		}
		engine.CheckStateBasedActions(g, c)
		if want > 0 {
			if got := g.Card(wurm).Counters.Count(engine.Time); got != want {
				t.Fatalf("upkeep %d: %d time counters, want %d", turn+1, got, want)
			}
			continue
		}
		if got := g.Card(wurm).Zone; got != engine.Graveyard {
			t.Errorf("after the last counter: Riftwurm in %v, want the graveyard", got)
		}
	}
}

// Living Weapon (CR 702.92, CardFactoryUtil.java:1361-1385): Flayer Husk
// enters, makes a 0/0 Phyrexian Germ and is attached to it; the Germ lives as
// a 1/1.
func TestLivingWeaponMakesAGermAndEquipsIt(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	husk := g.NewCard(corpusCard(t, "Flayer Husk"), p, engine.Hand)
	g.Player(p).ManaPool.AddColorless(1)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardC)
	if !g.CastSpell(p, husk, c) {
		t.Fatalf("cast failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	engine.CheckStateBasedActions(g, c)
	var germ engine.CardID
	for _, id := range g.Zone(engine.Battlefield, p).Cards() {
		if g.Card(id).IsToken {
			germ = id
		}
	}
	if germ == engine.NoCard {
		t.Fatal("no Germ token on the battlefield")
	}
	if got, _ := g.Card(husk).AttachedTo(); got != germ {
		t.Errorf("Flayer Husk attached to %v, want the Germ", got)
	}
	if pw, _ := g.Card(germ).Power(); pw != 1 {
		t.Errorf("Germ power %d, want 1", pw)
	}
}

// Myriad (CR 702.116, CardFactoryUtil.java:1508-1521): Gnoll War Band attacks
// one opponent; the other gets a tapped, attacking token copy, exiled at end
// of combat.
func TestMyriadCopiesAttackTheOtherOpponents(t *testing.T) {
	t.Parallel()
	g := engine.NewGame(scenarioDB(t), javarand.New(1), []string{"a", "b", "c"})
	ids := g.Players()
	p, b, other := ids[0], ids[1], ids[2]
	gnoll := g.NewCard(corpusCard(t, "Gnoll War Band"), p, engine.Battlefield)
	g.SetTurnState(2, p, engine.DeclareAttackers)
	c := engine.NewScriptedController()
	c.QueueConfirmEffect(true)
	c.QueueAttackers([]engine.CardID{gnoll})
	c.QueueAttackTarget(engine.PlayerEntity(b))
	c.QueueAttackTarget(engine.PlayerEntity(other))
	declareAttackers(t, g, c)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	var token engine.CardID
	for _, id := range g.Zone(engine.Battlefield, p).Cards() {
		if g.Card(id).IsToken {
			token = id
		}
	}
	if token == engine.NoCard {
		t.Fatal("no Myriad token")
	}
	if !g.Card(token).Tapped {
		t.Error("the Myriad token is untapped")
	}
	if got := g.AttackTarget(token); got != engine.PlayerEntity(other) {
		t.Errorf("the Myriad token attacks %v, want the other opponent", got)
	}
	toStep(t, g, c, p, engine.Main2, nil)
	if got := g.Card(token).Zone; got == engine.Battlefield {
		t.Error("the Myriad token outlived combat")
	}
}

// Firebending N (CardFactoryUtil.java:1075-1094): Fire Sages attacking adds
// {R} that stays through the combat steps and empties at end of combat.
func TestFirebendingManaLastsThroughCombat(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	sages := g.NewCard(corpusCard(t, "Fire Sages"), p, engine.Battlefield)
	g.SetTurnState(2, p, engine.DeclareAttackers)
	c := engine.NewScriptedController()
	attackAlone(t, g, c, sages, engine.PlayerEntity(other))
	if got := g.Player(p).ManaPool.Total(); got != 1 {
		t.Fatalf("pool %d after attacking, want 1", got)
	}
	g.AdvancePhase(c)
	if got := g.Player(p).ManaPool.Total(); got != 1 {
		t.Errorf("pool %d in the next combat step, want 1", got)
	}
	toStep(t, g, c, p, engine.Main2, nil)
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("pool %d after combat, want 0", got)
	}
}

// Strive (CR 702.108, CostAdjustment.java:139-142): Rouse the Mob costs {2}{R}
// more for each target past the first.
func TestStriveRaisesPerExtraTarget(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	a := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	b := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	rouse := g.NewCard(corpusCard(t, "Rouse the Mob"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 3)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(a), engine.CardEntity(b)})
	c.QueuePayGeneric(mana.ShardR)
	c.QueuePayGeneric(mana.ShardR)
	if g.CastSpell(p, rouse, c) {
		t.Fatal("two targets cast for {R}; Strive wants {2}{R} more")
	}
	g.Player(p).ManaPool = engine.Pool{}
	g.Player(p).ManaPool.Add(mana.Red, 4)
	c = engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(a), engine.CardEntity(b)})
	c.QueuePayGeneric(mana.ShardR)
	c.QueuePayGeneric(mana.ShardR)
	if !g.CastSpell(p, rouse, c) {
		t.Fatalf("two targets for {2}{R}{R} failed: %v", g.TakePendingError())
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("pool %d after the cast, want 0", got)
	}
}

// Firebending is a firebend (ManaEffect.java:195-197): Avatar Aang attacking
// draws a card through its ElementalBend trigger, and with one bend of four
// this turn (Count$AllFourBend) it does not transform.
func TestFirebendingFiresElementalBend(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 3)
	aang := g.NewCard(corpusCard(t, "Avatar Aang"), p, engine.Battlefield)
	g.SetTurnState(2, p, engine.DeclareAttackers)
	c := engine.NewScriptedController()
	attackAlone(t, g, c, aang, engine.PlayerEntity(other))
	if err := g.TakePendingError(); err != nil {
		t.Fatal(err)
	}
	if got := g.Zone(engine.Hand, p).Len(); got != 1 {
		t.Errorf("hand %d after firebending, want 1 drawn", got)
	}
	if got := g.Card(aang).Name(); got != "Avatar Aang" {
		t.Errorf("Aang is %q, want untransformed with one bend of four", got)
	}
}

// An ability with a Waterbend cost is a waterbend (MagicStack.java:397-399):
// activating Ruthless Waterbender's Waterbend {2} makes Avatar Aang draw.
func TestWaterbendCostFiresElementalBend(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 3)
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Avatar Aang"), p, engine.Battlefield)
	bender := g.NewCard(corpusCard(t, "Ruthless Waterbender"), p, engine.Battlefield)
	g.Player(p).ManaPool.Add(mana.Black, 2)
	c := engine.NewScriptedController()
	c.QueueCardChoice(nil)
	c.QueuePayGeneric(mana.ShardB)
	c.QueuePayGeneric(mana.ShardB)
	if !g.ActivateAbility(p, bender, 0, c) {
		t.Fatalf("Waterbend {2} activation failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Zone(engine.Hand, p).Len(); got != 1 {
		t.Errorf("hand %d after waterbending, want Aang's 1 card", got)
	}
}

// A spell cast paying a Waterbend cost is a waterbend: Ruinous Waterbending
// with its optional waterbend {4} makes Avatar Aang draw.
func TestWaterbendSpellFiresElementalBend(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 3)
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Avatar Aang"), p, engine.Battlefield)
	ruin := g.NewCard(corpusCard(t, "Ruinous Waterbending"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Black, 7)
	c := engine.NewScriptedController()
	c.QueueConfirmPayCost(true)
	c.QueueCardChoice(nil)
	for range 5 {
		c.QueuePayGeneric(mana.ShardB)
	}
	if !g.CastSpell(p, ruin, c) {
		t.Fatalf("cast with waterbend {4} failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Zone(engine.Hand, p).Len(); got != 1 {
		t.Errorf("hand %d after waterbending, want Aang's 1 card", got)
	}
}
