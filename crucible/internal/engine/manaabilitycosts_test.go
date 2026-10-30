package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// manaCostGame is a main-phase game on the corpus DB with p to act.
func manaCostGame(t *testing.T) (*engine.Game, engine.PlayerID) {
	t.Helper()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	return g, p
}

// Rushwood Grove (T SubCounter<X/STORAGE>, Amount$ X): X is announced first,
// that many storage counters come off, and each adds {G}.
func TestStorageLandRemovesAnnouncedCountersForMana(t *testing.T) {
	t.Parallel()
	g, p := manaCostGame(t)
	grove := g.NewCard(corpusCard(t, "Rushwood Grove"), p, engine.Battlefield)
	g.Card(grove).Counters.Add("STORAGE", 3)
	c := engine.NewScriptedController()
	c.QueuePayX(2)
	if !g.ActivateManaAbility(p, grove, 1, c) {
		t.Fatalf("activation failed: %v", g.TakePendingError())
	}
	if got := g.Card(grove).Counters.Count("STORAGE"); got != 1 {
		t.Errorf("%d storage counters left, want 1", got)
	}
	if got := g.Player(p).ManaPool.Total(); got != 2 {
		t.Errorf("pool holds %d mana, want 2", got)
	}
	if !g.Card(grove).Tapped {
		t.Error("Grove is untapped, want tapped")
	}
}

// Mana Confluence (T PayLife<1>): the life is paid with the tap.
func TestPayLifeManaAbilityPaysLife(t *testing.T) {
	t.Parallel()
	g, p := manaCostGame(t)
	land := g.NewCard(corpusCard(t, "Mana Confluence"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueManaColor(mana.Red)
	if !g.ActivateManaAbility(p, land, 0, c) {
		t.Fatalf("activation failed: %v", g.TakePendingError())
	}
	if got := g.Player(p).Life; got != 19 {
		t.Errorf("life %d, want 19", got)
	}
	if got := g.Player(p).ManaPool.Total(); got != 1 {
		t.Errorf("pool holds %d mana, want 1", got)
	}
}

// Pili-Pala (2 Q): {Q} untaps the tapped host; an untapped one cannot pay it.
func TestUntapSymbolManaAbility(t *testing.T) {
	t.Parallel()
	g, p := manaCostGame(t)
	pala := g.NewCard(corpusCard(t, "Pili-Pala"), p, engine.Battlefield)
	g.Player(p).ManaPool.Add(mana.White, 2)
	c := engine.NewScriptedController()
	c.QueueManaColor(mana.Blue)
	queueGeneric(c, mana.ShardW, 2)
	const idx = 0
	g.Card(pala).SummonSick = false
	if g.ActivateManaAbility(p, pala, idx, c) {
		t.Fatal("activated untapped, want {Q} to need a tapped host")
	}
	g.Card(pala).Tapped = true
	if !g.ActivateManaAbility(p, pala, idx, c) {
		t.Fatalf("activation failed: %v", g.TakePendingError())
	}
	if g.Card(pala).Tapped {
		t.Error("Pili-Pala is tapped, want untapped by {Q}")
	}
}

// Lion's Eye Diamond (Sac<1/CARDNAME> Discard<0/Hand>): the whole hand is
// discarded, the Diamond sacrificed, and three mana of one color added.
func TestDiscardHandManaAbility(t *testing.T) {
	t.Parallel()
	g, p := manaCostGame(t)
	led := g.NewCard(corpusCard(t, "Lion's Eye Diamond"), p, engine.Battlefield)
	hand := []engine.CardID{
		g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand),
		g.NewCard(corpusCard(t, "Hill Giant"), p, engine.Hand),
	}
	c := engine.NewScriptedController()
	c.QueueManaColor(mana.Black)
	if !g.ActivateManaAbility(p, led, 0, c) {
		t.Fatalf("activation failed: %v", g.TakePendingError())
	}
	for _, id := range hand {
		if z := g.Card(id).Zone; z != engine.Graveyard {
			t.Errorf("a hand card is in %v, want discarded", z)
		}
	}
	if z := g.Card(led).Zone; z != engine.Graveyard {
		t.Errorf("Diamond is in %v, want sacrificed", z)
	}
	if got := g.Player(p).ManaPool.Total(); got != 3 {
		t.Errorf("pool holds %d mana, want 3", got)
	}
}
