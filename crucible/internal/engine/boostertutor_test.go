package engine_test

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/boosters"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

var (
	boosterTableOnce sync.Once
	boosterTableVal  *boosters.Table
	boosterTableErr  error
)

// boosterTable is the real booster data, loaded once (it takes a few seconds).
func boosterTable(t *testing.T) *boosters.Table {
	t.Helper()
	boosterTableOnce.Do(func() {
		root := scenarioRepoRoot(t)
		boosterTableVal, boosterTableErr = boosters.Load(filepath.Join(root, "forge-gui", "res", "editions"),
			filepath.Join(root, "forge-gui", "res", "blockdata"), scenarioDB(t))
	})
	if boosterTableErr != nil {
		t.Fatal(boosterTableErr)
	}
	return boosterTableVal
}

// Booster Tutor opens a pack (the game's random stream picks the template and
// the cards, as the oracle's packs do), reveals it and puts the card the
// caster picks into their hand.
func TestBoosterTutorPutsAPackCardInTheHand(t *testing.T) {
	t.Parallel()

	tab := boosterTable(t)
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetBoosters(tab)

	// The same stream, replayed on its own, is the pack the card will offer.
	_, pack, err := boosters.NewGenerator(tab).OpenRandom(javarand.New(1))
	if err != nil {
		t.Fatal(err)
	}
	pickIndex := 3
	want := pack[pickIndex].Rules().Faces[0].Name

	spell := g.NewCard(corpusCard(t, "Booster Tutor"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Black, 1)
	c := engine.NewScriptedController()
	c.QueueOption(pickIndex)
	castThenResolve(t, g, p, spell, c)
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	var got []string
	for _, id := range g.Zone(engine.Hand, p).Cards() {
		got = append(got, g.Card(id).Def.Name)
	}
	if len(got) != 1 || got[0] != want {
		t.Errorf("hand = %v, want just %q (card %d of the pack)", got, want, pickIndex)
	}
}

func TestBoosterTutorFailsWithoutBoosterData(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	spell := g.NewCard(corpusCard(t, "Booster Tutor"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Black, 1)
	c := engine.NewScriptedController()
	if !g.CastSpell(p, spell, c) {
		t.Fatal("cast failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err == nil {
		t.Error("Booster Tutor resolved with no booster data")
	}
}
