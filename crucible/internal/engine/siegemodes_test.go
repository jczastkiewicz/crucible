package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// "As this enters, choose Mirran or Phyrexian" (Mirrodin Besieged, the
// Sieges): a GenericChoice with SetChosenMode$ runs before the move, records
// the chosen ability's description on the card (ChooseGenericEffect.java:
// 120-122), and the statics keyed on Card.ChosenMode<X> grant that mode's
// trigger only.
func TestSiegeChosenModeGrantsThatModesTrigger(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		pick  int
		mode  string
		myrs  int
		other string
	}{
		{"Mirran", 0, "Mirran", 1, "Phyrexian"},
		{"Phyrexian", 1, "Phyrexian", 0, "Mirran"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
			siege := g.NewCard(corpusCard(t, "Mirrodin Besieged"), p, engine.Hand)
			g.Player(p).ManaPool.Add(mana.Blue, 3)
			c := engine.NewScriptedController()
			c.QueuePayGeneric(mana.ShardU)
			c.QueuePayGeneric(mana.ShardU)
			c.QueueAbilityChoice([]int{tc.pick})
			if err := castEntrant(t, g, p, siege, c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := g.Card(siege).Memory.ChosenMode(); got != tc.mode {
				t.Fatalf("chosen mode = %q, want %q", got, tc.mode)
			}
			if !engine.Matches(g, g.Card(siege), valid.Parse("Card.ChosenMode"+tc.mode), p, siege) ||
				engine.Matches(g, g.Card(siege), valid.Parse("Card.ChosenMode"+tc.other), p, siege) {
				t.Errorf("Card.ChosenMode%s does not read the %s choice alone", tc.mode, tc.mode)
			}

			ring := g.NewCard(corpusCard(t, "Sol Ring"), p, engine.Hand)
			g.Player(p).ManaPool.Add(mana.Blue, 1)
			cast := engine.NewScriptedController()
			cast.QueuePayGeneric(mana.ShardU)
			if !g.CastSpell(p, ring, cast) {
				t.Fatal("could not cast Sol Ring")
			}
			if err := g.ResolveStack(engine.NewRegistry(), cast); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			myrs := 0
			for _, id := range g.Zone(engine.Battlefield, p).Cards() {
				if g.Card(id).IsToken {
					myrs++
				}
			}
			if myrs != tc.myrs {
				t.Errorf("Myr tokens = %d, want %d", myrs, tc.myrs)
			}
		})
	}
}

// Archangel of Strife: each player chooses war or peace as it enters (a
// GenericChoice with TempRemember$ whose Pumps note the card for the chooser,
// NoteCards$), and the creatures of the players who chose war get +3/+0 while
// the peace ones get +0/+3.
func TestArchangelOfStrifeNotesEachPlayersChoice(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	angel := g.NewCard(corpusCard(t, "Archangel of Strife"), p, engine.Hand)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Battlefield)
	g.Player(p).ManaPool.Add(mana.White, 7)
	c := engine.NewScriptedController()
	for range 7 {
		c.QueuePayGeneric(mana.ShardW)
	}
	c.QueueAbilityChoice([]int{0}) // the caster chooses war
	c.QueueAbilityChoice([]int{1}) // the opponent chooses peace
	if err := castEntrant(t, g, p, angel, c); err != nil {
		t.Fatalf("Archangel of Strife: %v", err)
	}
	engine.CheckStateBasedActions(g, c)
	if got, ok := g.Card(angel).Power(); !ok || got != 9 {
		t.Errorf("Archangel of Strife power = %d (%v), want 9: its controller chose war", got, ok)
	}
	if got, ok := g.Card(bears).Toughness(); !ok || got != 5 {
		t.Errorf("the opponent's Grizzly Bears toughness = %d (%v), want 5: its controller chose peace", got, ok)
	}
}
