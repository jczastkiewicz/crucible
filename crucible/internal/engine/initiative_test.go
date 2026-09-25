package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// takeInitiativeNow resolves "DB$ TakeInitiative" for p, answering Undercity's entrance: the dungeon choice (Undercity, the only
// option) and Secret Entrance's library search (nothing found).
func takeInitiativeNow(t *testing.T, g *engine.Game, p engine.PlayerID, c *engine.ScriptedController) {
	t.Helper()
	c.QueueOption(0)
	c.QueueCardChoice(nil)
	if err := resolveWith(t, g, p, c, "DB$ TakeInitiative"); err != nil {
		t.Fatalf("TakeInitiative: %v", err)
	}
}

// TestTakeInitiativeVenturesIntoUndercity proves the dominant shape (20 of
// 23 corpus lines, a bare "DB$ TakeInitiative"): the activator takes the
// initiative, "The Initiative" enters their Command zone, and its
// TakesInitiative trigger ventures them into Undercity. Taking it again
// while holding it ventures again (CR 725.2); the card is not duplicated.
func TestTakeInitiativeVenturesIntoUndercity(t *testing.T) {
	t.Parallel()

	g := newDungeonGame(t)
	p := g.Players()[0]
	c := engine.NewScriptedController()
	takeInitiativeNow(t, g, p, c)
	if g.Initiative() != p {
		t.Fatalf("Initiative = %v, want %v", g.Initiative(), p)
	}
	cards := namedIn(g, engine.Command, p, "The Initiative")
	if len(cards) != 1 || !g.Card(cards[0]).IsEffect {
		t.Fatalf("p's Command zone = %v, want one The Initiative effect card", g.Zone(engine.Command, p).Cards())
	}
	d := dungeonOf(g, p)
	if d == engine.NoCard || g.Card(d).Def.Name != "Undercity" || g.Card(d).CurrentRoom != "Secret Entrance" {
		t.Fatalf("dungeon %v, want Undercity on Secret Entrance", d)
	}

	c.QueueAbilityChoice([]int{1})
	c.QueueScry(nil, nil)
	if err := resolveWith(t, g, p, c, "DB$ TakeInitiative"); err != nil {
		t.Fatalf("TakeInitiative again: %v", err)
	}
	if g.Card(d).CurrentRoom != "Lost Well" {
		t.Errorf("room = %q, want Lost Well after taking the initiative again", g.Card(d).CurrentRoom)
	}
	if n := len(namedIn(g, engine.Command, p, "The Initiative")); n != 1 {
		t.Errorf("%d The Initiative cards, want 1", n)
	}
}

// TestInitiativeVenturesAtUpkeep proves The Initiative's Phase trigger:
// at the beginning of its holder's upkeep, they venture into Undercity.
func TestInitiativeVenturesAtUpkeep(t *testing.T) {
	t.Parallel()

	g := newDungeonGame(t)
	p := g.Players()[0]
	c := engine.NewScriptedController()
	takeInitiativeNow(t, g, p, c)
	d := dungeonOf(g, p)

	g.SetTurnState(3, p, engine.Untap)
	c.QueueAbilityChoice([]int{1})
	c.QueueScry(nil, nil)
	g.AdvancePhase(c)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(d).CurrentRoom != "Lost Well" {
		t.Errorf("room = %q, want Lost Well after the upkeep venture", g.Card(d).CurrentRoom)
	}
}

// TestInitiativeTakenByCombatDamage proves The Initiative's
// DamageDoneOnceByController trigger (CR 725.2): combat damage from two
// creatures one player controls makes that player take the initiative
// once (Defined$ TriggeredSource), and they venture into their own
// Undercity.
func TestInitiativeTakenByCombatDamage(t *testing.T) {
	t.Parallel()

	g := newDungeonGame(t)
	p, other := g.Players()[0], g.Players()[1]
	c := engine.NewScriptedController()
	takeInitiativeNow(t, g, p, c)
	mine := namedIn(g, engine.Command, p, "The Initiative")[0]

	// Lines that must not fire: noncombat only, the wrong target, a source
	// spec this port cannot read.
	for _, params := range []string{
		"ValidSource$ Player | ValidTarget$ You | CombatDamage$ False",
		"ValidSource$ Player | ValidTarget$ Opponent",
		"ValidSource$ Player.Chosen | ValidTarget$ You",
	} {
		g.NewCard(triggerWatcherDef(t, "Watcher",
			"Mode$ DamageDoneOnceByController | "+params+" | Execute$ TrigGain",
			"TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 100"), p, engine.Battlefield)
	}

	g.SetTurnState(2, other, engine.Main1)
	a1 := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Battlefield)
	a2 := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Battlefield)
	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{a1, a2})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)
	c.QueueOption(0)
	c.QueueCardChoice(nil)
	g.DealCombatDamage(c)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if g.Initiative() != other {
		t.Fatalf("Initiative = %v, want the attackers' controller", g.Initiative())
	}
	if got := g.Player(p).Life; got != 18 {
		t.Errorf("p life = %d, want 18 -- none of the non-matching watchers fire", got)
	}
	if g.Card(mine).Zone != engine.None {
		t.Errorf("old holder's card zone = %v, want None", g.Card(mine).Zone)
	}
	if d := dungeonOf(g, other); d == engine.NoCard || g.Card(d).CurrentRoom != "Secret Entrance" {
		t.Errorf("opponent's dungeon %v, want Undercity on Secret Entrance -- one venture for one trigger", d)
	}
}

// TestInitiativePassesWhenTheHolderLoses proves CR 725.4: a holder who is
// not the active player and loses passes the initiative to the active
// player, who ventures into Undercity.
func TestInitiativePassesWhenTheHolderLoses(t *testing.T) {
	t.Parallel()

	g := newDungeonGame(t, "a", "b", "c")
	a, b := g.Players()[0], g.Players()[1]
	c := engine.NewScriptedController()
	c.QueueOption(0)
	c.QueueCardChoice(nil)
	if err := resolveTargeting(t, g, a, c, []engine.EntityID{engine.PlayerEntity(b)}, "DB$ TakeInitiative | ValidTgts$ Player"); err != nil {
		t.Fatalf("TakeInitiative: %v", err)
	}
	if g.Initiative() != b {
		t.Fatalf("Initiative = %v, want the target %v", g.Initiative(), b)
	}

	g.Player(b).Life = 0
	c.QueueOption(0)
	c.QueueCardChoice(nil)
	engine.CheckStateBasedActions(g, c)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Initiative() != a {
		t.Fatalf("Initiative = %v, want the active player %v", g.Initiative(), a)
	}
	if dungeonOf(g, a) == engine.NoCard {
		t.Error("the active player did not venture on taking the initiative")
	}
}

// TestInitiativeToALostActivePlayerReproducesJava pins
// GameAction.java:2568-2573 as written (forge-java-defects.md): when the
// holder and the active player lose in one state-based-action pass, the
// initiative goes to the active player anyway, after a recursive call that
// finds no one new -- the holder is next -- so the lost active player ends
// holding it.
func TestInitiativeToALostActivePlayerReproducesJava(t *testing.T) {
	t.Parallel()

	g := newDungeonGame(t, "a", "b", "c")
	a, b := g.Players()[0], g.Players()[1]
	c := engine.NewScriptedController()
	g.SetInitiative(b)

	g.Player(a).Life, g.Player(b).Life = 0, 0
	engine.CheckStateBasedActions(g, c)
	if g.Initiative() != a {
		t.Errorf("Initiative = %v, want the lost active player %v, as Java leaves it", g.Initiative(), a)
	}
}

// TestInitiativeSurvivesCloneAndRestores proves Game.Clone copies the
// designation, and SetInitiative restores and clears it with no trigger.
func TestInitiativeSurvivesCloneAndRestores(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.SetInitiative(p)
	clone := g.Clone()
	clone.SetInitiative(other)
	if g.Initiative() != p || clone.Initiative() != other {
		t.Errorf("initiative original=%v clone=%v, want %v and %v", g.Initiative(), clone.Initiative(), p, other)
	}
	card := namedIn(clone, engine.Command, other, "The Initiative")
	if len(card) != 1 || !clone.IsDesignationCard(card[0]) {
		t.Fatalf("clone cards = %v, want other holding The Initiative", card)
	}
	if dungeonOf(clone, other) != engine.NoCard {
		t.Error("SetInitiative ventured")
	}
	clone.SetInitiative(engine.NoPlayer)
	if clone.Initiative() != engine.NoPlayer || clone.Card(card[0]).Zone != engine.None {
		t.Errorf("after clearing: initiative=%v zone=%v", clone.Initiative(), clone.Card(card[0]).Zone)
	}
}

// TestTakeInitiativeFailsClosedAndSkips proves the rejected ConditionDefined$,
// a Defined$ the port cannot read, an unmet condition and a lost target.
func TestTakeInitiativeFailsClosedAndSkips(t *testing.T) {
	t.Parallel()

	g := newDungeonGame(t, "a", "b", "c")
	a, b := g.Players()[0], g.Players()[1]
	c := engine.NewScriptedController()
	for _, line := range []string{
		"DB$ TakeInitiative | ConditionDefined$ Remembered | ConditionPresent$ Card",
		"DB$ TakeInitiative | Defined$ TriggeredSource",
	} {
		if err := resolveWith(t, g, a, c, line); err == nil {
			t.Errorf("%q: err = nil", line)
		}
	}
	if err := resolveWith(t, g, a, c, "DB$ TakeInitiative | ConditionPlayerTurn$ False"); err != nil {
		t.Fatalf("unmet condition: %v", err)
	}
	g.Player(b).Lost = true
	if err := resolveTargeting(t, g, a, c, []engine.EntityID{engine.PlayerEntity(b)}, "DB$ TakeInitiative | ValidTgts$ Player"); err != nil {
		t.Fatalf("lost target: %v", err)
	}
	if g.Initiative() != engine.NoPlayer {
		t.Errorf("Initiative = %v, want nobody", g.Initiative())
	}
}
