package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Daybound and Nightbound (CardFactoryUtil.java:879-895, :1522-1533): with
// neither day nor night a daybound permanent makes it day; when it becomes
// night the daybound front face transforms, and when it becomes day again the
// nightbound back face transforms back.
func TestDayboundTransformsWithDayAndNight(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	admirer := g.NewCard(corpusCard(t, "Bird Admirer"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	engine.CheckStateBasedActions(g, c)
	if g.DayTime() != engine.Day {
		t.Fatalf("daytime = %v, want Day: a daybound permanent makes it day", g.DayTime())
	}
	if g.Card(admirer).InTransformedState() {
		t.Fatal("Bird Admirer transformed by day")
	}

	g.SetTurnState(1, p, engine.Cleanup)
	for _, pid := range g.Players() {
		g.Player(pid).SpellsCastThisTurn = 0
	}
	g.AdvancePhase(c)
	engine.CheckStateBasedActions(g, c)
	if g.DayTime() != engine.Night {
		t.Fatalf("daytime = %v, want Night after a turn with no spells", g.DayTime())
	}
	if !g.Card(admirer).InTransformedState() || g.Card(admirer).Name() != "Wing Shredder" {
		t.Fatalf("at night the card is %q (transformed %v), want Wing Shredder", g.Card(admirer).Name(), g.Card(admirer).InTransformedState())
	}

	if _, err := resolveNow(t, g, g.ActivePlayer(), c, nil, "DB$ DayTime | Value$ Day"); err != nil {
		t.Fatal(err)
	}
	engine.CheckStateBasedActions(g, c)
	if g.Card(admirer).InTransformedState() {
		t.Errorf("by day the card is still %q, want Bird Admirer", g.Card(admirer).Name())
	}
}

// Curse of Leeches: by night it is Leeching Lurker; when day comes it
// transforms back into an Aura Curse, and its Transform replacement attaches
// it to a player its controller chooses (CR 701.28f).
func TestCurseOfLeechesAttachesToAChosenPlayerAsItTransformsBack(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	curse := g.NewCard(corpusCard(t, "Curse of Leeches"), p, engine.Battlefield)
	g.AttachToPlayer(curse, p)
	c := engine.NewScriptedController()
	if _, err := resolveNow(t, g, p, c, nil, "DB$ DayTime | Value$ Night"); err != nil {
		t.Fatal(err)
	}
	engine.CheckStateBasedActions(g, c)
	if !g.Card(curse).InTransformedState() || g.Card(curse).Name() != "Leeching Lurker" {
		t.Fatalf("at night (%v) the card is %q in %v, want Leeching Lurker", g.DayTime(), g.Card(curse).Name(), g.Card(curse).Zone)
	}

	c.QueuePlayerChoice(other)
	if _, err := resolveNow(t, g, p, c, nil, "DB$ DayTime | Value$ Day"); err != nil {
		t.Fatal(err)
	}
	engine.CheckStateBasedActions(g, c)
	cc := g.Card(curse)
	if cc.InTransformedState() || cc.Zone != engine.Battlefield {
		t.Fatalf("by day the card is %q in %v, want Curse of Leeches on the battlefield", cc.Name(), cc.Zone)
	}
	if pid, ok := cc.AttachedToPlayer(); !ok || pid != other {
		t.Errorf("Curse of Leeches attached to player %v (%v), want the chosen opponent", pid, ok)
	}
}

// Ludevic's Test Subject: each activation adds a hatchling counter; the fifth
// one removes them all and transforms it into Ludevic's Abomination.
func TestLudevicsTestSubjectHatchesOnTheFifthCounter(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	egg := g.NewCard(corpusCard(t, "Ludevic's Test Subject"), p, engine.Battlefield)
	for i := 1; i <= 5; i++ {
		c := engine.NewScriptedController()
		g.Player(p).ManaPool.Add(mana.Blue, 2)
		c.QueuePayGeneric(mana.ShardU)
		if !g.ActivateAbility(p, egg, 0, c) {
			t.Fatalf("activation %d: %v", i, g.TakePendingError())
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatal(err)
		}
		e := g.Card(egg)
		if i < 5 {
			if e.InTransformedState() || e.Counters.Count("HATCHLING") != i {
				t.Fatalf("after %d: transformed %v, %d hatchling counters", i, e.InTransformedState(), e.Counters.Count("HATCHLING"))
			}
			continue
		}
		if !e.InTransformedState() || e.Name() != "Ludevic's Abomination" || e.Counters.Count("HATCHLING") != 0 {
			t.Errorf("after 5: %q transformed %v with %d counters, want Ludevic's Abomination with none", e.Name(), e.InTransformedState(), e.Counters.Count("HATCHLING"))
		}
	}
}
