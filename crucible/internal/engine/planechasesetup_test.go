package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// planarDiceCard is pid's Planar Dice effect card in the Command zone.
func planarDiceCard(t *testing.T, g *engine.Game, pid engine.PlayerID) engine.CardID {
	t.Helper()
	for _, id := range g.Zone(engine.Command, pid).Cards() {
		if g.Card(id).Def.Name == "Planar Dice" {
			return id
		}
	}
	t.Fatalf("player %d has no Planar Dice card", pid)
	return engine.NoCard
}

// Player.initPlane (Player.java:2736-2750): the starting player's top plane
// turns face up, a phenomenon on top going to the bottom first; every player
// gets a Planar Dice card (createPlanechaseEffects).
func TestInitPlanechaseRevealsTheFirstPlaneAndDealsTheDice(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	phen := planarDef(t, "Test Phenomenon", "Phenomenon", nil, nil)
	ids := planarDeck(g, p, phen, plainPlane(t, "First"), plainPlane(t, "Second"))
	planarDeck(g, other, plainPlane(t, "Theirs"))
	if err := g.InitPlanechase(p); err != nil {
		t.Fatal(err)
	}
	if g.ActivePlane() != ids[1] || g.Card(ids[1]).Zone != engine.Command {
		t.Errorf("active plane %v in %v, want First in the Command zone", g.ActivePlane(), g.Card(ids[1]).Zone)
	}
	if deck := g.Zone(engine.PlanarDeck, p).Cards(); len(deck) != 2 || deck[0] != ids[2] || deck[1] != ids[0] {
		t.Errorf("planar deck = %v, want Second then the phenomenon", deck)
	}
	planarDiceCard(t, g, p)
	planarDiceCard(t, g, other)
}

// A planar deck of phenomena only is an error, not Java's endless loop
// (forge-java-defects.md); a game with no planar deck is not Planechase.
func TestInitPlanechaseRefusesPhenomenaOnlyAndSkipsNonPlanechase(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	if err := g.InitPlanechase(p); err != nil || g.Zone(engine.Command, p).Len() != 0 {
		t.Errorf("non-Planechase: err %v, Command zone %d; want nothing", err, g.Zone(engine.Command, p).Len())
	}
	planarDeck(g, p, planarDef(t, "Test Phenomenon", "Phenomenon", nil, nil))
	if err := g.InitPlanechase(p); err == nil {
		t.Error("phenomena-only planar deck: no error")
	}
}

// The planar die special action costs {X}, X the game's rolls this turn,
// counted as each resolves (RollPlanarDiceEffect.java:28-29): the first is
// free, the next costs {1} through either player's card, and the count
// resets with the turn (PhaseHandler.java:526).
func TestPlanarDieSpecialActionCostsOneMorePerRoll(t *testing.T) {
	t.Parallel()

	g, p, other := planarDieGame(t, dieBlank, dieBlank, dieBlank)
	planarDeck(g, p, plainPlane(t, "Here"), plainPlane(t, "There"))
	planarDeck(g, other, plainPlane(t, "Theirs"))
	if err := g.InitPlanechase(p); err != nil {
		t.Fatal(err)
	}
	mine, theirs := planarDiceCard(t, g, p), planarDiceCard(t, g, other)
	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, mine, 0, c) {
		t.Fatal("the first roll was not free")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	for _, card := range []engine.CardID{mine, theirs} {
		unpaid := engine.NewScriptedController()
		unpaid.QueuePayGeneric(mana.ShardG)
		if g.ActivateAbility(p, card, 0, unpaid) {
			t.Fatalf("a second roll through card %d cost nothing", card)
		}
	}
	g.Player(p).ManaPool.Add(mana.Green, 1)
	pay := engine.NewScriptedController()
	pay.QueuePayGeneric(mana.ShardG)
	if !g.ActivateAbility(p, theirs, 0, pay) {
		t.Fatal("the second roll could not be paid with {1}")
	}
	if g.ActivateAbility(other, theirs, 0, engine.NewScriptedController()) {
		t.Error("the non-active player rolled at sorcery speed")
	}
}

// Under the Planechase variant the active plane follows the turn
// (PhaseHandler.java:858-862): controller and Command zone.
func TestActivePlaneFollowsTheTurnUnderThePlanechaseVariant(t *testing.T) {
	t.Parallel()

	for _, variant := range []bool{true, false} {
		g, p, other := newTwoPlayerGame(t)
		for range 4 {
			g.NewCard(nil, p, engine.Library)
			g.NewCard(nil, other, engine.Library)
		}
		if variant {
			g.SetupVariant(engine.VariantPlanechase)
		}
		plane := faceUpPlane(g, p, plainPlane(t, "Here"))
		advanceUntil(t, g, engine.NewScriptedController(), func() bool { return g.ActivePlayer() == other })
		want := p
		if variant {
			want = other
		}
		if c := g.Card(plane); c.ZoneOwner != want || c.Controller() != want || c.Zone != engine.Command {
			t.Errorf("variant %v: plane in %v of %d, controlled by %d; want the Command zone of %d", variant, c.Zone, c.ZoneOwner, c.Controller(), want)
		}
	}
}

// Rolling through the opponent's Planar Dice card (Activator$ Player) is the
// activator's roll: the planeswalker face walks the activator's planar deck.
func TestPlanarDieRolledThroughTheOpponentsCardIsTheActivators(t *testing.T) {
	t.Parallel()

	g, p, other := planarDieGame(t, dieWalk)
	mine := planarDeck(g, p, plainPlane(t, "Here"), plainPlane(t, "There"))
	planarDeck(g, other, plainPlane(t, "Theirs"), plainPlane(t, "Theirs Too"))
	if err := g.InitPlanechase(p); err != nil {
		t.Fatal(err)
	}
	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, planarDiceCard(t, g, other), 0, c) {
		t.Fatal("p could not roll through the opponent's card")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if g.ActivePlane() != mine[1] {
		t.Errorf("active plane %v (%s), want p's There", g.ActivePlane(), g.Card(g.ActivePlane()).Def.Name)
	}
}

// A clone keeps the variants and the planar die count (Game.Clone lists its
// fields one by one).
func TestCloneKeepsVariantsAndPlanarDieCount(t *testing.T) {
	t.Parallel()

	g, p, other := planarDieGame(t, dieBlank)
	g.SetupVariant(engine.VariantCommander | engine.VariantPlanechase)
	planarDeck(g, p, plainPlane(t, "Here"), plainPlane(t, "There"))
	planarDeck(g, other, plainPlane(t, "Theirs"))
	if err := g.InitPlanechase(p); err != nil {
		t.Fatal(err)
	}
	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, planarDiceCard(t, g, p), 0, c) {
		t.Fatal("roll failed")
	}
	cl := g.Clone()
	if !cl.HasVariant(engine.VariantCommander) || !cl.HasVariant(engine.VariantPlanechase) {
		t.Error("the clone lost a variant")
	}
	unpaid := engine.NewScriptedController()
	unpaid.QueuePayGeneric(mana.ShardG)
	if cl.ActivateAbility(p, planarDiceCard(t, cl, p), 0, unpaid) {
		t.Error("the clone's second roll cost nothing: the count was not copied")
	}
}
