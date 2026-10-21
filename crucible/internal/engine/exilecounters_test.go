package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestExileCountersOnHiddenChangeZoneAndDig proves the counters a card gets
// as it is exiled: a hidden-origin ChangeZone adds WithCountersType$ after the
// move (ChangeZoneEffect.java:1508-1514), and Dig adds one ExileWithCounters$
// counter per exiled card (DigEffect.java:414, :490).
func TestExileCountersOnHiddenChangeZoneAndDig(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, line string
		hand       bool
		want       int
	}{
		{"hidden ChangeZone amount", "DB$ ChangeZone | Hidden$ True | Origin$ Hand | Destination$ Exile | ChangeType$ Card | WithCountersType$ TIME | WithCountersAmount$ 3", true, 3},
		{"hidden ChangeZone default amount", "DB$ ChangeZone | Hidden$ True | Origin$ Hand | Destination$ Exile | ChangeType$ Card | WithCountersType$ TIME", true, 1},
		{"Dig ExileWithCounters", "DB$ Dig | DigNum$ 1 | DestinationZone$ Exile | SkipReorder$ True | ExileWithCounters$ TIME", false, 1},
		{"Dig without the param", "DB$ Dig | DigNum$ 1 | DestinationZone$ Exile | SkipReorder$ True", false, 0},
	} {
		g, p, other := newTwoPlayerGame(t)
		libraryCards(t, g, p, 1)
		libraryCards(t, g, other, 1)
		c := engine.NewScriptedController()
		if tc.hand {
			held := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Hand)
			c.QueueCardChoice([]engine.CardID{held})
		} else {
			c.QueueCardChoice(g.Zone(engine.Library, p).Cards()[:1])
		}
		def := etbChainDef(t, "Test Exile Counters", tc.line)
		if _, err := castETBChain(t, g, p, def, c); err != nil {
			t.Fatalf("%s: ResolveStack: %v", tc.name, err)
		}
		exiled := g.Zone(engine.Exile, p).Cards()
		if len(exiled) != 1 {
			t.Fatalf("%s: exiled %d cards, want 1", tc.name, len(exiled))
		}
		if got := g.Card(exiled[0]).Counters.Count(engine.CounterType("TIME")); got != tc.want {
			t.Errorf("%s: TIME counters = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// TestSetStateFaceDownStats proves FaceDownPower$/FaceDownToughness$/
// FaceDownSetType$ on SetState Mode$ TurnFaceDown (CardFactoryUtil.java:
// 4129-4146): the face-down permanent has the named size and types instead of
// the plain 2/2 Creature.
func TestSetStateFaceDownStats(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	bear := g.NewCard(creatureDefPT(t, "5", "5"), p, engine.Battlefield)
	def := etbChainDef(t, "Test Turner",
		"DB$ SetState | ValidTgts$ Creature | Mode$ TurnFaceDown | FaceDownPower$ 3 | FaceDownToughness$ 4 | FaceDownSetType$ Artifact & Creature & Cyberman")
	host := g.NewCard(def, p, engine.Battlefield)
	g.PushAbility(engine.Ability{API: engine.APISetState, Source: host, Controller: p,
		Params:  def.Faces[0].Triggers[0].Subs[0].Ability,
		Targets: []engine.EntityID{engine.CardEntity(bear)}})
	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	c := g.Card(bear)
	if !c.IsFaceDown() {
		t.Fatal("not face down")
	}
	pw, _ := c.Power()
	tg, _ := c.Toughness()
	if pw != 3 || tg != 4 {
		t.Errorf("size = %d/%d, want 3/4", pw, tg)
	}
	if !c.Type().HasSubtype("Cyberman") || !c.Type().Has(cardtype.Artifact) {
		t.Errorf("type = %v, want Artifact Creature Cyberman", c.Type())
	}
}
