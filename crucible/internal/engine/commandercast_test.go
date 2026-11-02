package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// The Commander Effect's Moved replacement (Player.java:3283-3296, CR 903.9b):
// a commander bounced to its owner's hand goes to the Command zone instead
// when the owner says yes, and to the hand when they say no.
func TestCommanderBoundForTheHandGoesToTheCommandZone(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		confirm bool
		want    engine.ZoneType
	}{
		{"accepted", true, engine.Command},
		{"declined", false, engine.Hand},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
			bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
			g.AddCommander(p, bears)
			c := engine.NewScriptedController()
			c.QueueConfirmReplacementEffect(tc.confirm)
			if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(bears)},
				"DB$ ChangeZone | ValidTgts$ Creature | Origin$ Battlefield | Destination$ Hand"); err != nil {
				t.Fatal(err)
			}
			if got := g.Card(bears).Zone; got != tc.want {
				t.Errorf("commander in %v, want %v", got, tc.want)
			}
			if tc.want == engine.Command {
				if got := g.Zone(engine.Command, p).Cards()[0]; got != bears {
					t.Errorf("Command zone starts with %d, want the commander first (Zone.java:106-114)", got)
				}
			}
		})
	}
}

// A commander in the Command zone is cast through the Commander Effect's
// MayPlay$ static; each cast from there is counted, and the next one costs {2}
// more per earlier cast (CostAdjustment.java:56-61).
func TestCommanderCastFromTheCommandZonePaysTax(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Command)
	g.AddCommander(p, bears)
	sba(g)
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardG)
	if !g.CastSpell(p, bears, c) {
		t.Fatalf("first cast from the Command zone failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	c.QueueConfirmReplacementEffect(true)
	if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(bears)},
		"DB$ ChangeZone | ValidTgts$ Creature | Origin$ Battlefield | Destination$ Hand"); err != nil {
		t.Fatal(err)
	}
	if g.Card(bears).Zone != engine.Command {
		t.Fatalf("commander in %v, want Command", g.Card(bears).Zone)
	}

	// {1}{G} plus {2} tax: three mana is one short, four pays it.
	g.Player(p).ManaPool.Add(mana.Green, 3)
	for range 3 {
		c.QueuePayGeneric(mana.ShardG)
	}
	if g.CastSpell(p, bears, c) {
		t.Fatal("second cast paid no commander tax")
	}
	g.Player(p).ManaPool.Add(mana.Green, 4-g.Player(p).ManaPool.Total())
	for range 3 {
		c.QueuePayGeneric(mana.ShardG)
	}
	if !g.CastSpell(p, bears, c) {
		t.Fatalf("second cast with the tax paid failed: %v", g.TakePendingError())
	}
}

// Liesa, Shroud of Dusk: once cast from the Command zone, her next cast trades
// the {2} tax for 2 life (ReduceCost and RaiseCost PayLife<X> on
// Count$CommanderCastFromCommandZone/Times.2).
func TestLiesaPaysLifeInsteadOfCommanderTax(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	liesa := g.NewCard(corpusCard(t, "Liesa, Shroud of Dusk"), p, engine.Command)
	g.AddCommander(p, liesa)
	sba(g)
	pay := func(c *engine.ScriptedController) {
		g.Player(p).ManaPool.Add(mana.White, 2)
		g.Player(p).ManaPool.Add(mana.Black, 3)
		c.QueuePayGeneric(mana.ShardB)
		c.QueuePayGeneric(mana.ShardB)
	}
	c := engine.NewScriptedController()
	pay(c)
	if !g.CastSpell(p, liesa, c) {
		t.Fatalf("first cast failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	c.QueueConfirmReplacementEffect(true)
	if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(liesa)},
		"DB$ ChangeZone | ValidTgts$ Creature | Origin$ Battlefield | Destination$ Hand"); err != nil {
		t.Fatal(err)
	}
	life := g.Player(p).Life
	pay(c)
	if !g.CastSpell(p, liesa, c) {
		t.Fatalf("second cast failed: %v", g.TakePendingError())
	}
	if got := life - g.Player(p).Life; got != 2 {
		t.Errorf("second cast cost %d life, want 2", got)
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("%d mana left, want the printed {2}{W}{W}{B} paid exactly", got)
	}
}
