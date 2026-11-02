package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// A Mode$ AlternativeCost static on another card (Fist of Suns, Kentaro,
// Rooftop Storm, Dream Halls; StaticAbilityAlternativeCost) is one more way
// to cast a matching spell from the hand, without the OptionalCost.AltCost
// mark a spell's own line gives. A spell's own EffectZone$ Graveyard line
// (Worldheart Phoenix, Scourge of Nel Toth) is its cast from the graveyard,
// never from the hand.

// optionsSeen is a scripted controller that records every ChooseOption list.
type optionsSeen struct {
	*engine.ScriptedController
	seen [][]string
}

func (o *optionsSeen) ChooseOption(g *engine.Game, p engine.PlayerID, card engine.CardID, options []string) int {
	o.seen = append(o.seen, slices.Clone(options))
	return o.ScriptedController.ChooseOption(g, p, card, options)
}

func addWUBRG(g *engine.Game, p engine.PlayerID) {
	for _, col := range []mana.Colors{mana.White, mana.Blue, mana.Black, mana.Red, mana.Green} {
		g.Player(p).ManaPool.Add(col, 1)
	}
}

// Fist of Suns casts Grizzly Bears for {W}{U}{B}{R}{G}. On Bringer of Green
// Zenith's Twilight it is a third way past the Bringer's own: X drops with
// the mana cost and reads 0, and the cast is not the Bringer's own
// alternative cost, so no counters (Count$AltCost reads only that).
func TestOtherHostAlternativeCostFistOfSuns(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Fist of Suns"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
	addWUBRG(g, p)
	c := engine.NewScriptedController()
	c.QueueOption(1)
	castThenResolve(t, g, p, bears, c)
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("%d mana left after the Bears, want 0", got)
	}

	bringer := g.NewCard(corpusCard(t, "Bringer of Green Zenith's Twilight"), p, engine.Hand)
	addWUBRG(g, p)
	rc := &optionsSeen{ScriptedController: engine.NewScriptedController()}
	rc.QueueOption(2)
	if !g.CastSpell(p, bringer, rc) {
		t.Fatal("cast the Bringer through Fist of Suns failed")
	}
	if len(rc.seen) != 1 || len(rc.seen[0]) != 3 {
		t.Fatalf("options %q, want the mana cost, the Bringer's own and Fist's", rc.seen)
	}
	if err := g.ResolveStack(engine.NewRegistry(), rc.ScriptedController); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(bringer).Counters.Count(engine.P1P1); got != 0 {
		t.Errorf("%d +1/+1 counters, want 0: X is 0 and the cast is not the Bringer's own", got)
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("%d mana left after the Bringer, want 0", got)
	}
}

// Kentaro's {X} is the Samurai spell's mana value, never announced: Auron
// (3W) casts for four blue mana. Grizzly Bears, no Samurai, is offered only
// its mana cost.
func TestOtherHostAlternativeCostKentaroSamurai(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Kentaro, the Smiling Cat"), p, engine.Battlefield)
	auron := g.NewCard(corpusCard(t, "Auron, Venerated Guardian"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Blue, 4)
	c := engine.NewScriptedController()
	c.QueueOption(1)
	for range 4 {
		c.QueuePayGeneric(mana.ShardU)
	}
	castThenResolve(t, g, p, auron, c)
	if got := g.Card(auron).Zone; got != engine.Battlefield {
		t.Errorf("Auron in %v, want Battlefield", got)
	}

	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Blue, 2)
	rc := &optionsSeen{ScriptedController: engine.NewScriptedController()}
	rc.QueuePayGeneric(mana.ShardU)
	if g.CastSpell(p, bears, rc) {
		t.Fatal("cast Grizzly Bears for blue mana through Kentaro")
	}
	if len(rc.seen) != 0 {
		t.Errorf("options %q offered for a non-Samurai spell", rc.seen)
	}
}

// Rooftop Storm's {0} is for Zombie creature spells only.
func TestOtherHostAlternativeCostRooftopStorm(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Rooftop Storm"), p, engine.Battlefield)
	digger := g.NewCard(corpusCard(t, "Gravedigger"), p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueOption(1)
	castThenResolve(t, g, p, digger, c)
	if got := g.Card(digger).Zone; got != engine.Battlefield {
		t.Errorf("Gravedigger in %v, want Battlefield", got)
	}
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
	bc := engine.NewScriptedController()
	bc.QueuePayGeneric(mana.ShardG)
	if g.CastSpell(p, bears, bc) {
		t.Fatal("cast Grizzly Bears for {0} through Rooftop Storm")
	}
}

// Dream Halls applies to every caster (no ValidPlayer$), here under the
// opponent's control. The discard must share a color with the spell and is
// never the spell itself: Lightning Bolt alone is not offered it, a colorless
// spell never is, and with a Shock beside it the Shock is discarded.
func TestOtherHostAlternativeCostDreamHalls(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Dream Halls"), other, engine.Battlefield)
	bolt := g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Hand)
	island := g.NewCard(corpusCard(t, "Island"), p, engine.Hand)
	alone := &optionsSeen{ScriptedController: engine.NewScriptedController()}
	alone.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	if g.CastSpell(p, bolt, alone) {
		t.Fatal("cast Lightning Bolt with no red card to discard and no mana")
	}
	if len(alone.seen) != 0 {
		t.Errorf("options %q offered with no other red card", alone.seen)
	}

	shock := g.NewCard(corpusCard(t, "Shock"), p, engine.Hand)
	gnomes := g.NewCard(corpusCard(t, "Bottle Gnomes"), p, engine.Hand)
	colorless := &optionsSeen{ScriptedController: engine.NewScriptedController()}
	for range 3 {
		colorless.QueuePayGeneric(mana.ShardR)
	}
	if g.CastSpell(p, gnomes, colorless) {
		t.Fatal("cast colorless Bottle Gnomes through Dream Halls")
	}
	if len(colorless.seen) != 0 {
		t.Errorf("options %q offered for a colorless spell", colorless.seen)
	}

	c := &optionsSeen{ScriptedController: engine.NewScriptedController()}
	c.QueueOption(1)
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	c.QueueDiscardChoice([]engine.CardID{shock})
	castThenResolve(t, g, p, bolt, c.ScriptedController)
	if got := g.Player(other).Life; got != 17 {
		t.Errorf("opponent at %d, want 17", got)
	}
	if got := g.Card(shock).Zone; got != engine.Graveyard {
		t.Errorf("Shock in %v, want Graveyard: discarded", got)
	}
	if got := g.Card(island).Zone; got != engine.Hand {
		t.Errorf("Island in %v, want Hand", got)
	}
}

// Scourge of Nel Toth's {B}{B} and two creatures is its cast from the
// graveyard: from the hand only the mana cost is offered.
func TestOwnGraveyardAlternativeCostNotFromHand(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	first := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	second := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	scourge := g.NewCard(corpusCard(t, "Scourge of Nel Toth"), p, engine.Hand)
	rc := &optionsSeen{ScriptedController: engine.NewScriptedController()}
	for range 5 {
		rc.QueuePayGeneric(mana.ShardB)
	}
	if g.CastSpell(p, scourge, rc) {
		t.Fatal("cast Scourge of Nel Toth from the hand for its graveyard cost")
	}
	if len(rc.seen) != 0 {
		t.Errorf("options %q offered from the hand", rc.seen)
	}

	scourge = g.NewCard(corpusCard(t, "Scourge of Nel Toth"), p, engine.Graveyard)
	g.Player(p).ManaPool.Add(mana.Black, 2)
	c := engine.NewScriptedController()
	c.QueueSacrificeChoice([]engine.CardID{first, second})
	castThenResolve(t, g, p, scourge, c)
	if got := g.Card(scourge).Zone; got != engine.Battlefield {
		t.Errorf("Scourge in %v, want Battlefield", got)
	}
	for _, id := range []engine.CardID{first, second} {
		if got := g.Card(id).Zone; got != engine.Graveyard {
			t.Errorf("sacrificed Bears in %v, want Graveyard", got)
		}
	}
}

// Worldheart Phoenix cast from the graveyard for {W}{U}{B}{R}{G} is its own
// alternative cost (AltCostPaid) and enters with two +1/+1 counters.
func TestOwnGraveyardAlternativeCostRecordsTheCast(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	phoenix := g.NewCard(corpusCard(t, "Worldheart Phoenix"), p, engine.Graveyard)
	addWUBRG(g, p)
	c := engine.NewScriptedController()
	castThenResolve(t, g, p, phoenix, c)
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	if got := g.Card(phoenix).Zone; got != engine.Battlefield {
		t.Fatalf("Phoenix in %v, want Battlefield", got)
	}
	if got := g.Card(phoenix).Counters.Count(engine.P1P1); got != 2 {
		t.Errorf("%d +1/+1 counters, want 2", got)
	}
}
