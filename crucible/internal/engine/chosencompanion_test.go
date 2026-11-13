package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// companionAsker records the companions it is offered and declines them.
type companionAsker struct {
	*engine.ScriptedController
	offered []engine.EntityID
}

func (a *companionAsker) ChooseEntitiesForEffect(_ *engine.Game, _ engine.PlayerID, _ engine.CardID, options []engine.EntityID, _, _ int) []engine.EntityID {
	a.offered = options
	return nil
}

// A sideboard companion is offered when its condition holds for every card
// of the starting library (Player.assignCompanion), and not otherwise.
func TestCompanionIsOfferedWhenTheDeckMeetsItsCondition(t *testing.T) {
	t.Parallel()
	bolts := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = "Lightning Bolt"
		}
		return out
	}
	for _, tc := range []struct {
		companion string
		library   []string
		offered   bool
	}{
		{"Kaheera, the Orphanguard", []string{"Lightning Bolt", "Savannah Lions"}, true},
		{"Kaheera, the Orphanguard", []string{"Lightning Bolt", "Grizzly Bears"}, false},
		{"Lutri, the Spellchaser", []string{"Lightning Bolt", "Shock", "Mountain", "Mountain"}, true},
		{"Lutri, the Spellchaser", []string{"Lightning Bolt", "Lightning Bolt"}, false},
		{"Jegantha, the Wellspring", []string{"Lightning Helix", "Mountain"}, true},
		{"Jegantha, the Wellspring", []string{"Counterspell"}, false},
		{"Umori, the Collector", []string{"Lightning Bolt", "Shock", "Mountain"}, true},
		{"Umori, the Collector", []string{"Lightning Bolt", "Grizzly Bears"}, false},
		{"Gyruda, Doom of Depths", []string{"Grizzly Bears", "Mountain"}, true},
		{"Gyruda, Doom of Depths", []string{"Lightning Bolt"}, false},
		{"Lurrus of the Dream-Den", []string{"Grizzly Bears", "Counterspell", "Mountain"}, true},
		{"Lurrus of the Dream-Den", []string{"Hill Giant"}, false},
		{"Yorion, Sky Nomad", bolts(80), true},
		{"Yorion, Sky Nomad", bolts(79), false},
	} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		for _, name := range tc.library {
			g.NewCard(corpusCard(t, name), p, engine.Library)
		}
		companion := g.NewCard(corpusCard(t, tc.companion), p, engine.Sideboard)
		asker := &companionAsker{ScriptedController: engine.NewScriptedController()}
		if err := g.AssignCompanion(p, asker); err != nil {
			t.Fatalf("%s: %v", tc.companion, err)
		}
		if got := slices.Contains(asker.offered, engine.CardEntity(companion)); got != tc.offered {
			t.Errorf("%s over %d cards: offered %v, want %v", tc.companion, len(tc.library), got, tc.offered)
		}
		if g.Card(companion).Zone != engine.Sideboard {
			t.Errorf("%s: declined, but in %v", tc.companion, g.Card(companion).Zone)
		}
	}
}

// The chosen companion waits in the Command zone with its Companion Effect,
// whose granted special action puts it into the hand for {3} any time its
// owner could cast a sorcery (CR 702.139c).
func TestChosenCompanionGoesToHandForThree(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, other, 4)
	for range 4 {
		g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Library)
	}
	kaheera := g.NewCard(corpusCard(t, "Kaheera, the Orphanguard"), p, engine.Sideboard)
	c := engine.NewScriptedController()
	c.QueueEntityChoice([]engine.EntityID{engine.CardEntity(kaheera)})
	if err := g.AssignCompanion(p, c); err != nil {
		t.Fatal(err)
	}
	if got := g.Card(kaheera).Zone; got != engine.Command {
		t.Fatalf("the chosen companion is in %v, want the command zone", got)
	}
	var effect string
	for _, id := range g.Zone(engine.Command, p).Cards() {
		if g.Card(id).IsEffect {
			effect = g.Card(id).Name()
		}
	}
	if effect != "Kaheera, the Orphanguard's Companion Effect" {
		t.Errorf("effect card %q", effect)
	}

	g.SetTurnState(1, other, engine.Main1)
	engine.CheckStateBasedActions(g, c)
	g.Player(p).ManaPool.AddColorless(3)
	index := g.Card(kaheera).AbilityCount() - 1
	if g.ActivateAbility(p, kaheera, index, c) {
		t.Fatal("companion put into the hand on another player's turn")
	}

	g.SetTurnState(2, p, engine.Main1)
	engine.CheckStateBasedActions(g, c)
	for range 3 {
		c.QueuePayGeneric(mana.ShardC)
	}
	if !g.ActivateAbility(p, kaheera, index, c) {
		t.Fatal("the companion's special action was not taken")
	}
	if got := g.Card(kaheera).Zone; got != engine.Hand {
		t.Errorf("after the special action the companion is in %v, want the hand", got)
	}
	if g.Player(p).ManaPool.Total() != 0 || g.StackLen() != 0 {
		t.Errorf("pool %d, stack %d: the action costs {3} and uses no stack", g.Player(p).ManaPool.Total(), g.StackLen())
	}
}
