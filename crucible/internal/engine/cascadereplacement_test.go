package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Averna, the Chaos Bloom: as its controller cascades, a land from among the
// exiled cards may go onto the battlefield tapped (Event$ Cascade,
// ReplacedCards.Land). The dig exiles both Forests before Grizzly Bears, and
// the Cascade replacement is handed every exiled card (a Java test on
// upstream/master puts one Forest onto the battlefield).
func TestAvernaPutsAnExiledLandOntoTheBattlefield(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Averna, the Chaos Bloom"), p, engine.Battlefield)
	var library []engine.CardID
	for _, name := range []string{"Forest", "Forest", "Grizzly Bears", "Island"} {
		library = append(library, g.NewCard(corpusCard(t, name), p, engine.Library))
	}
	if top := g.Zone(engine.Library, p).Cards()[0]; top != library[0] {
		t.Fatalf("library top is %d, want the first Forest %d", top, library[0])
	}
	elf := g.NewCard(corpusCard(t, "Bloodbraid Elf"), p, engine.Hand)
	sba(g)
	g.Player(p).ManaPool.Add(mana.Red, 2)
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardR)
	c.QueuePayGeneric(mana.ShardG)
	c.QueueCardChoice([]engine.CardID{library[1]})
	c.QueueConfirmEffect(false) // do not cast Grizzly Bears
	if !g.CastSpell(p, elf, c) {
		t.Fatalf("Bloodbraid Elf not cast: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if err := g.TakePendingError(); err != nil {
		t.Fatal(err)
	}
	forest := g.Card(library[1])
	if forest.Zone != engine.Battlefield || !forest.Tapped {
		t.Errorf("chosen Forest in %v (tapped %v), want tapped on the battlefield", forest.Zone, forest.Tapped)
	}
	if got := g.Card(library[0]).Zone; got != engine.Library {
		t.Errorf("other Forest in %v, want back in the library", got)
	}
}
