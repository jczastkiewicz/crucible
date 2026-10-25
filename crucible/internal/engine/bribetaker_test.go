package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Bribe Taker: for each kind of counter on permanents you control, the
// placer picks +1/+1 or a counter of that kind, and one is put on it. Here one
// kind (CHARGE) is in play, chosen over +1/+1.
func TestBribeTakerPutsTheChosenKindForEachKindInPlay(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	c := engine.NewScriptedController()
	g.SetTurnState(1, p, engine.Main1)
	holder := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	g.Card(holder).Counters.Add("CHARGE", 1)
	taker := g.NewCard(corpusCard(t, "Bribe Taker"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 6)
	for range 5 {
		c.QueuePayGeneric(mana.ShardG)
	}
	c.QueueOptionNamed("CHARGE")
	if !g.CastSpell(p, taker, c) {
		t.Fatalf("CastSpell failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	if got := g.Card(taker).Counters.Count("CHARGE"); got != 1 {
		t.Errorf("charge counters = %d, want 1", got)
	}
	if got := g.Card(taker).Counters.Count(engine.P1P1); got != 0 {
		t.Errorf("+1/+1 counters = %d, want 0", got)
	}
}

// firstOfferController answers every ChooseCardsForEffect with the first card
// offered, for a test that cannot name a token before it exists.
type firstOfferController struct{ *engine.ScriptedController }

func (firstOfferController) ChooseCardsForEffect(_ *engine.Game, _ engine.PlayerID, _ engine.CardID, offer []engine.CardID, _, _ int) []engine.CardID {
	return offer[:1]
}

// Exotic Pets: two Fish tokens, then for each kind of counter among creatures
// you control the counter goes on the token you choose.
func TestExoticPetsPutsEachKindOnAChosenToken(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	sc := engine.NewScriptedController()
	c := firstOfferController{sc}
	g.SetTurnState(1, p, engine.Main1)
	holder := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	g.Card(holder).Counters.Add("CHARGE", 1)
	spell := g.NewCard(corpusCard(t, "Exotic Pets"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.White, 1)
	g.Player(p).ManaPool.Add(mana.Blue, 2)
	sc.QueuePayGeneric(mana.ShardU)
	if !g.CastSpell(p, spell, c) {
		t.Fatalf("CastSpell failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	charged := 0
	for _, id := range g.Zone(engine.Battlefield, p).Cards() {
		charged += g.Card(id).Counters.Count("CHARGE")
	}
	if charged != 2 {
		t.Errorf("charge counters on my permanents = %d, want 2 (the original and one token)", charged)
	}
}
