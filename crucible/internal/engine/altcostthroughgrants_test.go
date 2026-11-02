package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// A MayPlay option with its own alternative mana cost replaces the mana of
// another AlternativeCost line again (GameActionUtil.java:117-121, 361-363):
// Mistvein Borderpost's "{1} and return a basic land" through a perpetual
// "pay {R} rather than its mana cost" grant costs {R} and the land.
func TestMayPlayAltManaCostReplacesAnAlternativeCostsMana(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	post := g.NewCard(corpusCard(t, "Mistvein Borderpost"), p, engine.Hand)
	island := g.NewCard(corpusCard(t, "Island"), p, engine.Battlefield)
	def := etbChainDef(t, "Test Perpetual Discount",
		"DB$ Animate | ValidTgts$ Card.YouOwn | Duration$ Perpetual | staticAbilities$ PerpAltCost",
		"PerpAltCost", "Mode$ Continuous | EffectZone$ All | MayPlay$ True | MayPlayAltManaCost$ R | MayPlayDontGrantZonePermissions$ True | Affected$ Card.Self | AffectedZone$ Hand,Graveyard,Library,Exile,Command")
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(post)})
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	g.Player(p).ManaPool.Add(mana.Red, 1)
	c.QueueOptionNamed("Cast Mistvein Borderpost for its alternative cost 1 Return<1/Land.Basic> with its mana replaced by {R}")
	c.QueueReturnChoice([]engine.CardID{island})
	if !g.CastSpell(p, post, c) {
		t.Fatalf("CastSpell for {R} and a land failed: %v", g.TakePendingError())
	}
	if got := g.Card(island).Zone; got != engine.Hand {
		t.Errorf("Island in %v, want returned to hand", got)
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("%d mana left, want the {R} paid", got)
	}
}

// Bolas's Citadel: a spell cast from the top of the library costs life equal
// to its mana value instead of its mana cost (MayPlayAltManaCost$
// PayLife<ConvertedManaCost>, the affected card's mana value substituted,
// StaticAbilityContinuous.java:933-939).
func TestBolassCitadelCastsTheTopCardForLife(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Bolas's Citadel"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Library)
	sba(g)
	life := g.Player(p).Life
	c := engine.NewScriptedController()
	if !g.CastSpell(p, bears, c) {
		t.Fatalf("Grizzly Bears not cast from the library: %v", g.TakePendingError())
	}
	if got := life - g.Player(p).Life; got != 2 {
		t.Errorf("paid %d life, want 2", got)
	}
}

// As Foretold's {0} and Bolas's Citadel's life are two ways to cast the top
// card, not one: the option with a part past its mana is its own.
func TestCitadelLifeAndAsForetoldZeroStayTwoOptions(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Bolas's Citadel"), p, engine.Battlefield)
	foretold := g.NewCard(corpusCard(t, "As Foretold"), p, engine.Battlefield)
	g.Card(foretold).Counters.Add(engine.Time, 2)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Library)
	sba(g)
	life := g.Player(p).Life
	c := engine.NewScriptedController()
	c.QueueOptionNamed("Cast Grizzly Bears for {0}")
	if !g.CastSpell(p, bears, c) {
		t.Fatalf("Grizzly Bears not cast for As Foretold's {0}: %v", g.TakePendingError())
	}
	if got := life - g.Player(p).Life; got != 0 {
		t.Errorf("paid %d life for the {0} option, want 0", got)
	}
}
