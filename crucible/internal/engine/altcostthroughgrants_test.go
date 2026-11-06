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

// Hama, the Bloodbender: the exiled card is cast by waterbending {X}, X its
// mana value (MayPlayAltManaCost$ Waterbend<ConvertedManaCost>). Waterbend is
// mana, so the caster may tap artifacts and creatures for its generic
// (CostWaterbend, CostAdjustment.adjustCostByWaterbend): Divination is cast
// with no mana by tapping Hama and two Bears.
func TestHamaCastsTheExiledCardByWaterbending(t *testing.T) {
	t.Parallel()

	g, p, opp := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	hama := g.NewCard(corpusCard(t, "Hama, the Bloodbender"), p, engine.Hand)
	div := g.NewCard(corpusCard(t, "Divination"), opp, engine.Graveyard)
	libraryCards(t, g, opp, 3)
	libraryCards(t, g, p, 3)
	g.Player(p).ManaPool.Add(mana.Blue, 5)
	c := keepingOrder()
	for range 3 {
		c.QueueHybridManaColor(mana.Blue)
	}
	for range 2 {
		c.QueuePayGeneric(mana.ShardU)
	}
	if !g.CastSpell(p, hama, c) {
		t.Fatalf("Hama not cast: %v", g.TakePendingError())
	}
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(opp)})
	c.QueueConfirmEffect(true)
	c.QueueCardChoice([]engine.CardID{div})
	for range 2 {
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
	}
	sba(g)
	if got := g.Card(div).Zone; got != engine.Exile {
		t.Fatalf("Divination in %v, want exiled by Hama's trigger", got)
	}
	bears := []engine.CardID{
		g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield),
		g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield),
	}
	tappers := append([]engine.CardID{hama}, bears...)
	c.QueueCardChoice(tappers)
	if !g.CastSpell(p, div, c) {
		t.Fatalf("Divination not cast by waterbending: %v", g.TakePendingError())
	}
	for _, id := range tappers {
		if !g.Card(id).Tapped {
			t.Errorf("%s untapped, want tapped to waterbend", g.Card(id).Def.Name)
		}
	}
}

// Marshland Bloodcaster: the next spell costs life equal to its mana value
// instead of its mana cost (Cost$ PayLife<ConvertedManaCost>, the spell's mana
// value substituted, StaticAbilityAlternativeCost.java:40-41), X counted as 0
// (CR 107.3b): Blaze cast this way costs 1 life and deals 0, and the spell
// after it costs its mana again.
func TestMarshlandBloodcasterCastsTheNextSpellForLife(t *testing.T) {
	t.Parallel()

	g, p, opp := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	caster := g.NewCard(corpusCard(t, "Marshland Bloodcaster"), p, engine.Battlefield)
	blaze := g.NewCard(corpusCard(t, "Blaze"), p, engine.Hand)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Black, 2)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardB)
	if !g.ActivateAbility(p, caster, 0, c) {
		t.Fatalf("Marshland Bloodcaster's ability not activated: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	life, oppLife := g.Player(p).Life, g.Player(opp).Life
	c.QueueOptionNamed("Cast Blaze for its alternative cost PayLife<1>")
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(opp)})
	if !g.CastSpell(p, blaze, c) {
		t.Fatalf("Blaze not cast for life: %v", g.TakePendingError())
	}
	if got := life - g.Player(p).Life; got != 1 {
		t.Errorf("paid %d life, want 1", got)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(opp).Life; got != oppLife {
		t.Errorf("opponent at %d life, want %d: X is 0", got, oppLife)
	}
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c.QueuePayGeneric(mana.ShardG)
	if !g.CastSpell(p, bears, c) {
		t.Fatalf("Grizzly Bears not cast for its mana: %v", g.TakePendingError())
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("%d mana left, want {1}{G} paid", got)
	}
}
