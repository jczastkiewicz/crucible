package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/valid"
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

// castResolved casts card from p's hand paying generic pips with shard, then
// resolves the stack.
func castResolved(t *testing.T, g *engine.Game, p engine.PlayerID, card engine.CardID, c *engine.ScriptedController, generic int, shard mana.Shard) {
	t.Helper()
	for range generic {
		c.QueuePayGeneric(shard)
	}
	if !g.CastSpell(p, card, c) {
		t.Fatalf("cast failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	engine.CheckStateBasedActions(g, c)
}

// tokensOf lists p's tokens on the battlefield.
func tokensOf(g *engine.Game, p engine.PlayerID) []engine.CardID {
	var out []engine.CardID
	for _, id := range g.Zone(engine.Battlefield, p).Cards() {
		if g.Card(id).IsToken {
			out = append(out, id)
		}
	}
	return out
}

// Fading (CR 702.32, CardFactoryUtil.java:1054-1075): Blastoderm enters with
// three fade counters, loses one each upkeep, and is sacrificed at the upkeep
// it has none to remove.
func TestFadingCountsDownThenSacrifices(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 8)
	libraryCards(t, g, other, 8)
	g.SetTurnState(1, p, engine.Main1)
	derm := g.NewCard(corpusCard(t, "Blastoderm"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 4)
	c := engine.NewScriptedController()
	castResolved(t, g, p, derm, c, 2, mana.ShardG)
	fade := engine.CounterType("FADE")
	if got := g.Card(derm).Counters.Count(fade); got != 3 {
		t.Fatalf("entered with %d fade counters, want 3", got)
	}
	for upkeep, want := range []int{2, 1, 0} {
		toStep(t, g, c, p, engine.Upkeep, nil)
		if got := g.Card(derm).Counters.Count(fade); g.Card(derm).Zone != engine.Battlefield || got != want {
			t.Fatalf("upkeep %d: zone %v, %d fade counters, want %d on the battlefield", upkeep+1, g.Card(derm).Zone, got, want)
		}
	}
	toStep(t, g, c, p, engine.Upkeep, nil)
	if got := g.Card(derm).Zone; got != engine.Graveyard {
		t.Errorf("with no fade counter to remove: Blastoderm in %v, want the graveyard", got)
	}
}

// Rampage 2 (CR 702.23, CardFactoryUtil.java:1622-1640): Wolverine Pack
// blocked by three creatures gets +4/+4, +2/+2 for each blocker beyond the
// first (Count$Valid Creature.blockingTriggeredAttacker/Minus.1).
func TestRampagePumpsPerExtraBlocker(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	pack := g.NewCard(corpusCard(t, "Wolverine Pack"), p, engine.Battlefield)
	idle := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Battlefield)
	var blocks []engine.Block
	for range 3 {
		blocks = append(blocks, engine.Block{Blocker: g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Battlefield), Attacker: pack})
	}
	g.SetTurnState(2, p, engine.DeclareAttackers)
	c := engine.NewScriptedController()
	attackAlone(t, g, c, pack, engine.PlayerEntity(other))
	g.AdvancePhase(c)
	c.QueueBlocks(blocks)
	declareBlockers(t, g, c)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if pw, _ := g.Card(pack).Power(); pw != 6 {
		t.Errorf("Wolverine Pack power %d, want 2 + 2*2", pw)
	}
	if engine.Matches(g, g.Card(idle), valid.Parse("Creature.blockingSource"), p, pack) {
		t.Error("an idle creature reads blockingSource")
	}
	if !engine.Matches(g, g.Card(blocks[0].Blocker), valid.Parse("Creature.blockingSource"), p, pack) {
		t.Error("a blocker of the source does not read blockingSource")
	}
}

// Graft (CR 702.58, CardFactoryUtil.java:1152-1170): Simic Initiate enters
// with a +1/+1 counter and may move it onto another creature entering; with
// none left it dies as a 0/0.
func TestGraftMovesACounterOntoAnEnteringCreature(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	first := g.NewCard(corpusCard(t, "Simic Initiate"), p, engine.Hand)
	second := g.NewCard(corpusCard(t, "Simic Initiate"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c := engine.NewScriptedController()
	castResolved(t, g, p, first, c, 0, mana.ShardG)
	c.QueueConfirmOptionalTrigger(true)
	castResolved(t, g, p, second, c, 0, mana.ShardG)
	if got := g.Card(second).Counters.Count(engine.P1P1); got != 2 {
		t.Errorf("the second Initiate has %d +1/+1 counters, want its own and the grafted one", got)
	}
	if got := g.Card(first).Zone; got != engine.Graveyard {
		t.Errorf("the first Initiate in %v, want the graveyard as a 0/0", got)
	}
}

// Riot (CR 702.136, CardFactoryUtil.java:2470-2476): Gruul Spellbreaker
// enters with a +1/+1 counter when its controller pays AddCounter<1/P1P1>,
// with haste otherwise.
func TestRiotCounterOrHaste(t *testing.T) {
	t.Parallel()
	for _, counter := range []bool{true, false} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		ogre := g.NewCard(corpusCard(t, "Gruul Spellbreaker"), p, engine.Hand)
		g.Player(p).ManaPool.Add(mana.Red, 1)
		g.Player(p).ManaPool.Add(mana.Green, 2)
		c := engine.NewScriptedController()
		c.QueueConfirmPayCost(counter)
		castResolved(t, g, p, ogre, c, 1, mana.ShardG)
		o := g.Card(ogre)
		if got := o.Counters.Count(engine.P1P1) == 1; got != counter {
			t.Errorf("counter chosen %v: has a +1/+1 counter %v", counter, got)
		}
		if got := o.HasKeyword("Haste"); got == counter {
			t.Errorf("counter chosen %v: has haste %v", counter, got)
		}
	}
}

// Unleash (CR 702.98, CardFactoryUtil.java:2523-2528, :4030-4032):
// Gore-House Chainwalker may enter with a +1/+1 counter, and can't block
// while it has one.
func TestUnleashCounterStopsBlocking(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	walker := g.NewCard(corpusCard(t, "Gore-House Chainwalker"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 2)
	c := engine.NewScriptedController()
	c.QueueConfirmReplacementEffect(true)
	castResolved(t, g, p, walker, c, 1, mana.ShardR)
	if got := g.Card(walker).Counters.Count(engine.P1P1); got != 1 {
		t.Fatalf("%d +1/+1 counters, want Unleash's 1", got)
	}
	attacker := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Battlefield)
	if g.CanBlock(attacker, walker) {
		t.Error("an unleashed Chainwalker with a counter can block")
	}
}

// Dredge 2 (CR 702.52, CardFactoryUtil.java:2177-2200): with Golgari
// Brownscale in the graveyard, a draw may become milling two and returning it
// to the hand.
func TestDredgeReplacesADraw(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	libraryCards(t, g, p, 5)
	lizard := g.NewCard(corpusCard(t, "Golgari Brownscale"), p, engine.Graveyard)
	c := engine.NewScriptedController()
	c.QueueConfirmReplacementEffect(true)
	g.DrawCards(p, 1, c)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Card(lizard).Zone; got != engine.Hand {
		t.Errorf("Brownscale in %v, want the hand", got)
	}
	if got := len(g.Zone(engine.Library, p).Cards()); got != 3 {
		t.Errorf("library %d, want 5 less two milled", got)
	}
	if got := len(g.Zone(engine.Hand, p).Cards()); got != 1 {
		t.Errorf("hand %d, want only Brownscale (no card drawn)", got)
	}
}

// Umbra armor (CR 702.89, CardFactoryUtil.java:2501-2525): a creature wearing
// Hyena Umbra that would be destroyed by lethal damage loses the damage and
// the Umbra is destroyed instead.
func TestUmbraArmorDestroysItselfInstead(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	bear := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	umbra := g.NewCard(corpusCard(t, "Hyena Umbra"), p, engine.Battlefield)
	g.Attach(umbra, bear)
	g.Card(bear).Damage.Marked = 5
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if got := g.Card(bear).Zone; got != engine.Battlefield {
		t.Fatalf("the creature in %v, want it saved", got)
	}
	if got := g.Card(bear).Damage.Marked; got != 0 {
		t.Errorf("damage %d, want removed", got)
	}
	if got := g.Card(umbra).Zone; got != engine.Graveyard {
		t.Errorf("Hyena Umbra in %v, want the graveyard", got)
	}
}

// Mobilize 1 (CR 702.181, CardFactoryUtil.java:1476-1489): Shock Brigade
// attacking makes a 1/1 Warrior tapped and attacking, sacrificed at end step.
func TestMobilizeMakesAnAttackingWarrior(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 3)
	libraryCards(t, g, other, 3)
	brigade := g.NewCard(corpusCard(t, "Shock Brigade"), p, engine.Battlefield)
	g.SetTurnState(2, p, engine.DeclareAttackers)
	c := engine.NewScriptedController()
	attackAlone(t, g, c, brigade, engine.PlayerEntity(other))
	tokens := tokensOf(g, p)
	if len(tokens) != 1 {
		t.Fatalf("%d tokens, want one Warrior", len(tokens))
	}
	w := g.Card(tokens[0])
	if !w.Tapped || g.AttackTarget(tokens[0]) != engine.PlayerEntity(other) {
		t.Errorf("Warrior tapped %v attacking %v, want tapped and attacking", w.Tapped, g.AttackTarget(tokens[0]))
	}
	toStep(t, g, c, other, engine.Upkeep, nil)
	if got := w.Zone; got == engine.Battlefield {
		t.Error("the Warrior outlived the end step")
	}
}

// For Mirrodin! (CR 702.163, CardFactoryUtil.java:1108-1131): Hexgold
// Halberd enters, makes a 2/2 Rebel and is attached to it.
func TestForMirrodinMakesARebelAndEquipsIt(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	halberd := g.NewCard(corpusCard(t, "Hexgold Halberd"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 2)
	c := engine.NewScriptedController()
	castResolved(t, g, p, halberd, c, 1, mana.ShardR)
	tokens := tokensOf(g, p)
	if len(tokens) != 1 {
		t.Fatalf("%d tokens, want one Rebel", len(tokens))
	}
	if got, _ := g.Card(halberd).AttachedTo(); got != tokens[0] {
		t.Errorf("Hexgold Halberd attached to %v, want the Rebel", got)
	}
}

// Living metal (CR 702.161, CardFactoryUtil.java:3947-3949): Prowl, Pursuit
// Vehicle is a creature during its controller's turn only.
func TestLivingMetalIsACreatureOnYourTurn(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	prowl := g.NewCard(corpusCard(t, "Prowl, Stoic Strategist"), p, engine.Battlefield)
	if !g.Transform(prowl) {
		t.Fatal("Prowl did not convert")
	}
	g.SetTurnState(1, p, engine.Main1)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if !g.Card(prowl).Type().Has(cardtype.Creature) {
		t.Error("the Vehicle is not a creature on its controller's turn")
	}
	g.SetTurnState(2, other, engine.Main1)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if g.Card(prowl).Type().Has(cardtype.Creature) {
		t.Error("the Vehicle is a creature on the opponent's turn")
	}
}

// Reconfigure (CR 702.151, CardFactoryUtil.java:3374-3395, Card.java:
// 3973-3985): Lizard Blades attaches to a creature for {2}, stops being a
// creature while attached, and becomes one again when unattached for {2}.
func TestReconfigureAttachesAndStopsBeingACreature(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	blades := g.NewCard(corpusCard(t, "Lizard Blades"), p, engine.Battlefield)
	bear := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.Player(p).ManaPool.Add(mana.Red, 4)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardR)
	c.QueuePayGeneric(mana.ShardR)
	c.QueueTargets([]engine.EntityID{engine.CardEntity(bear)})
	if !g.ActivateAbility(p, blades, 0, c) {
		t.Fatalf("reconfigure attach failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	engine.CheckStateBasedActions(g, c)
	if got, _ := g.Card(blades).AttachedTo(); got != bear {
		t.Fatalf("Lizard Blades attached to %v, want the creature", got)
	}
	if g.Card(blades).Type().Has(cardtype.Creature) {
		t.Error("attached Lizard Blades is still a creature")
	}
	if !g.Card(bear).HasKeyword("Double Strike") {
		t.Error("the equipped creature lacks double strike")
	}
	c.QueuePayGeneric(mana.ShardR)
	c.QueuePayGeneric(mana.ShardR)
	if !g.ActivateAbility(p, blades, 1, c) {
		t.Fatalf("reconfigure unattach failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	engine.CheckStateBasedActions(g, c)
	if _, attached := g.Card(blades).AttachedTo(); attached {
		t.Error("Lizard Blades still attached")
	}
	if !g.Card(blades).Type().Has(cardtype.Creature) {
		t.Error("unattached Lizard Blades is not a creature again")
	}
}

// Double team (CR 702.185, CardFactoryUtil.java:945-962): Tiefling Outcasts
// attacking conjures a duplicate into its controller's hand, and both lose
// double team.
func TestDoubleTeamConjuresADuplicate(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	outcasts := g.NewCard(corpusCard(t, "Tiefling Outcasts"), p, engine.Battlefield)
	g.SetTurnState(2, p, engine.DeclareAttackers)
	c := engine.NewScriptedController()
	attackAlone(t, g, c, outcasts, engine.PlayerEntity(other))
	hand := g.Zone(engine.Hand, p).Cards()
	if len(hand) != 1 || g.Card(hand[0]).Name() != "Tiefling Outcasts" {
		t.Fatalf("hand %v, want a conjured Tiefling Outcasts", hand)
	}
	for _, id := range []engine.CardID{outcasts, hand[0]} {
		if g.Card(id).HasKeyword("Double team") {
			t.Errorf("card %v still has double team", id)
		}
	}
}

// Hideaway 5 (CR 702.75, CardFactoryUtil.java:1273-1305): Fight Rigging
// entering exiles one of the top five cards face down and puts the other four
// on the bottom.
func TestHideawayExilesOneFaceDown(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	libraryCards(t, g, p, 6)
	top := g.Zone(engine.Library, p).Cards()[0]
	rig := g.NewCard(corpusCard(t, "Fight Rigging"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 3)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{top})
	castResolved(t, g, p, rig, c, 2, mana.ShardG)
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	exiled := g.Zone(engine.Exile, p).Cards()
	if len(exiled) != 1 || exiled[0] != top || !g.Card(exiled[0]).IsFaceDown() {
		t.Fatalf("exile %v, want one face-down card", exiled)
	}
	if got := len(g.Zone(engine.Library, p).Cards()); got != 5 {
		t.Errorf("library %d, want 5", got)
	}
	if !engine.Matches(g, g.Card(exiled[0]), valid.Parse("Card.ExiledWithSource"), p, rig) {
		t.Error("the hidden card is not exiled with Fight Rigging")
	}
}

// Sunburst (CR 702.44, CardFactoryUtil.java:2477-2489): Suntouched Myr, a
// creature, enters with a +1/+1 counter per color spent; Pentad Prism, not a
// creature, with charge counters.
func TestSunburstCountsColorsSpent(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	myr := g.NewCard(corpusCard(t, "Suntouched Myr"), p, engine.Hand)
	prism := g.NewCard(corpusCard(t, "Pentad Prism"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.White, 2)
	g.Player(p).ManaPool.Add(mana.Blue, 1)
	g.Player(p).ManaPool.Add(mana.Black, 1)
	g.Player(p).ManaPool.Add(mana.Red, 1)
	c := engine.NewScriptedController()
	for _, s := range []mana.Shard{mana.ShardW, mana.ShardU, mana.ShardB} {
		c.QueuePayGeneric(s)
	}
	castResolved(t, g, p, myr, c, 0, mana.ShardW)
	if got := g.Card(myr).Counters.Count(engine.P1P1); got != 3 {
		t.Errorf("Suntouched Myr has %d +1/+1 counters, want 3", got)
	}
	c.QueuePayGeneric(mana.ShardW)
	castResolved(t, g, p, prism, c, 1, mana.ShardR)
	if got := g.Card(prism).Counters.Count(engine.CounterType("CHARGE")); got != 2 {
		t.Errorf("Pentad Prism has %d charge counters, want 2", got)
	}
}

// Cipher (CR 702.99, CardFactoryUtil.java:2714-2720, :3796-3821,
// EncodeEffect.java): Shadow Slice resolves, is exiled and encoded on a
// creature; that creature's combat damage to a player lets its controller cast
// a copy, and the encoding ends when the creature leaves.
func TestCipherEncodesAndCastsACopyOnCombatDamage(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 3)
	libraryCards(t, g, other, 3)
	g.SetTurnState(1, p, engine.Main1)
	bear := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	slice := g.NewCard(corpusCard(t, "Shadow Slice"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Black, 5)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	c.QueueConfirmEffect(true)
	c.QueueCardChoice([]engine.CardID{bear})
	castResolved(t, g, p, slice, c, 4, mana.ShardB)
	if got := g.Player(other).Life; got != 17 {
		t.Fatalf("opponent life %d after the cast, want 17", got)
	}
	if got := g.Card(slice).Zone; got != engine.Exile {
		t.Fatalf("Shadow Slice in %v, want exile", got)
	}
	if !engine.Matches(g, g.Card(bear), valid.Parse("Card.EncodedWithSource"), p, slice) {
		t.Fatal("the creature is not encoded with Shadow Slice")
	}
	g.SetTurnState(3, p, engine.DeclareAttackers)
	attackAlone(t, g, c, bear, engine.PlayerEntity(other))
	c.QueueBlocks(nil)
	declareBlockers(t, g, c)
	c.QueueConfirmOptionalTrigger(true)
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	g.DealCombatDamage(c)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	if got := g.Player(other).Life; got != 12 {
		t.Errorf("opponent life %d after combat, want 17 - 2 damage - 3 from the copy", got)
	}
	if got := g.Card(slice).Zone; got != engine.Exile {
		t.Errorf("Shadow Slice in %v after the copy, want still exiled", got)
	}
	g.Move(bear, engine.Graveyard, p)
	if engine.Matches(g, g.Card(bear), valid.Parse("Card.EncodedWithSource"), p, slice) {
		t.Error("the encoding outlived the creature leaving the battlefield")
	}
}

// More Than Meets the Eye (CR 702.162, CardFactoryUtil.java:3129-3143): Prowl
// is cast converted from the hand for {2}{W} and enters as its back face,
// Prowl, Pursuit Vehicle; converting it turns it to its front face.
func TestMoreThanMeetsTheEyeCastsConverted(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	prowl := g.NewCard(corpusCard(t, "Prowl, Stoic Strategist"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.White, 3)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardW)
	c.QueuePayGeneric(mana.ShardW)
	if !g.CastFace(p, prowl, c) {
		t.Fatalf("converted cast failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	engine.CheckStateBasedActions(g, c)
	pr := g.Card(prowl)
	if pr.Zone != engine.Battlefield || pr.Name() != "Prowl, Pursuit Vehicle" {
		t.Fatalf("zone %v name %q, want the back face on the battlefield", pr.Zone, pr.Name())
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("pool %d, want the {2}{W} paid", got)
	}
	if !g.Transform(prowl) {
		t.Fatal("Prowl did not convert")
	}
	if got := pr.Name(); got != "Prowl, Stoic Strategist" {
		t.Errorf("converted to %q, want the front face", got)
	}
	g.Move(prowl, engine.Graveyard, p)
	if g.CastFace(p, prowl, c) {
		t.Error("cast converted from the graveyard")
	}
	// CR 118.6 (Spell.java:82): a mana cost of "no cost" is never paid, so
	// Ancestral Vision is not cast from the hand.
	vision := g.NewCard(corpusCard(t, "Ancestral Vision"), p, engine.Hand)
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(p)})
	if g.CastSpell(p, vision, c) {
		t.Error("Ancestral Vision cast from the hand")
	}
}

// Suspend (CR 702.62, CardFactoryUtil.java:1829-1873, :3547-3607): Rift Bolt
// suspended for {R} is exiled with a time counter; at its owner's next upkeep
// the counter comes off and it is cast without paying its mana cost.
func TestSuspendCastsWhenTheLastCounterIsRemoved(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 4)
	libraryCards(t, g, other, 4)
	g.SetTurnState(1, p, engine.Main1)
	bolt := g.NewCard(corpusCard(t, "Rift Bolt"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 1)
	c := engine.NewScriptedController()
	if !g.Suspend(p, bolt, c) {
		t.Fatalf("suspend failed: %v", g.TakePendingError())
	}
	b := g.Card(bolt)
	if b.Zone != engine.Exile || b.Counters.Count(engine.Time) != 1 {
		t.Fatalf("zone %v, %d time counters, want exiled with 1", b.Zone, b.Counters.Count(engine.Time))
	}
	if !engine.Matches(g, b, valid.Parse("Card.suspended"), p, bolt) {
		t.Error("Rift Bolt is not suspended")
	}
	c.QueueConfirmEffect(true)
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	toStep(t, g, c, p, engine.Upkeep, nil)
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	if got := g.Player(other).Life; got != 17 {
		t.Errorf("opponent life %d, want 17 from the free Rift Bolt", got)
	}
	if got := b.Zone; got != engine.Graveyard {
		t.Errorf("Rift Bolt in %v, want the graveyard", got)
	}
}

// A suspended creature card cast as its last counter goes has haste (CR
// 702.62a): Nantuko Shaman. Suspend is offered only at its cast timing, and
// not for a card outside the hand.
func TestSuspendedCreatureHasHaste(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 4)
	libraryCards(t, g, other, 4)
	g.SetTurnState(1, p, engine.Upkeep)
	shaman := g.NewCard(corpusCard(t, "Nantuko Shaman"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 4)
	c := engine.NewScriptedController()
	if g.Suspend(p, shaman, c) {
		t.Fatal("a creature suspended in the upkeep")
	}
	g.SetTurnState(1, p, engine.Main1)
	c.QueuePayGeneric(mana.ShardG)
	c.QueuePayGeneric(mana.ShardG)
	if !g.Suspend(p, shaman, c) {
		t.Fatalf("suspend failed: %v", g.TakePendingError())
	}
	if g.Suspend(p, shaman, c) {
		t.Error("suspended again from exile")
	}
	c.QueueConfirmEffect(true)
	toStep(t, g, c, p, engine.Upkeep, nil)
	s := g.Card(shaman)
	if s.Zone != engine.Battlefield {
		t.Fatalf("Nantuko Shaman in %v, want the battlefield: %v", s.Zone, g.TakePendingError())
	}
	if !s.HasKeyword("Haste") {
		t.Error("the suspended creature has no haste")
	}
}
