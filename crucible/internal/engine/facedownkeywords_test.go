package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// castFaceDown gives p three white mana and casts card face down for {3}
// (abilityCastFaceDown), resolving it.
func castFaceDown(t *testing.T, g *engine.Game, p engine.PlayerID, card engine.CardID) {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.White, 3)
	c := engine.NewScriptedController()
	c.QueueOption(1) // the hand cast, then the face-down one
	for range 3 {
		c.QueuePayGeneric(mana.ShardW)
	}
	if !g.CastSpell(p, card, c) {
		t.Fatalf("face-down cast failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
}

// Megamorph (CR 702.37b): Hidden Dragonslayer cast face down for {3} is a
// nameless 2/2; turning it face up for {2}{W} is a special action that puts a
// +1/+1 counter on it and fires its "turned face up" trigger.
func TestMegamorphCastsFaceDownAndTurnsUpWithACounter(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	slayer := g.NewCard(corpusCard(t, "Hidden Dragonslayer"), p, engine.Hand)
	big := g.NewCard(creatureDefPT(t, "4", "4"), other, engine.Battlefield)
	castFaceDown(t, g, p, slayer)
	s := g.Card(slayer)
	if s.Zone != engine.Battlefield || !s.IsFaceDown() || s.Name() != "" {
		t.Fatalf("after the cast: zone %v face down %v name %q, want a nameless face-down permanent", s.Zone, s.IsFaceDown(), s.Name())
	}
	if pw, _ := s.Power(); pw != 2 {
		t.Errorf("face-down power %d, want 2", pw)
	}
	c := engine.NewScriptedController()
	g.Player(p).ManaPool.Add(mana.White, 3)
	c.QueuePayGeneric(mana.ShardW)
	c.QueuePayGeneric(mana.ShardW)
	c.QueueTargets([]engine.EntityID{engine.CardEntity(big)})
	if !g.TurnFaceUp(p, slayer, 0, c) {
		t.Fatalf("turn face up failed: %v", g.TakePendingError())
	}
	if s.IsFaceDown() || s.Name() != "Hidden Dragonslayer" {
		t.Fatalf("after turning up: face down %v name %q", s.IsFaceDown(), s.Name())
	}
	if got := s.Counters.Count(engine.P1P1); got != 1 {
		t.Errorf("%d +1/+1 counters, want Megamorph's 1", got)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Card(big).Zone; got != engine.Graveyard {
		t.Errorf("the 4/4 in %v, want destroyed by the turned-face-up trigger", got)
	}
}

// Exiled Doomsayer makes every morph cost {2} more.
func TestMorphCostRaisedByExiledDoomsayer(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	slayer := g.NewCard(corpusCard(t, "Hidden Dragonslayer"), p, engine.Hand)
	castFaceDown(t, g, p, slayer)
	g.NewCard(corpusCard(t, "Exiled Doomsayer"), other, engine.Battlefield)
	g.Player(p).ManaPool.Add(mana.White, 6)
	c := engine.NewScriptedController()
	for range 4 {
		c.QueuePayGeneric(mana.ShardW)
	}
	if !g.TurnFaceUp(p, slayer, 0, c) {
		t.Fatalf("turn face up for {4}{W} failed: %v", g.TakePendingError())
	}
	if got := g.Player(p).ManaPool.Total(); got != 1 {
		t.Errorf("pool %d after turning up, want 1 of 6 left ({2}{W} and {2} more)", got)
	}
}

// Disguise (CR 702.168): the face-down Museum Nightwatch has ward {2}.
func TestDisguiseFaceDownHasWard(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	watch := g.NewCard(corpusCard(t, "Museum Nightwatch"), p, engine.Hand)
	castFaceDown(t, g, p, watch)
	if !slices.Contains(g.Card(watch).KeywordLines(), "Ward:2") {
		t.Errorf("face-down keywords %v, want Ward:2", g.Card(watch).KeywordLines())
	}
}

// A manifested creature card turns face up for its mana cost (CR 701.34c),
// and Growing Dread's "whenever you turn a permanent face up" puts a +1/+1
// counter on it; Karlov Watchdog stops it during its controller's turn.
func TestManifestTurnsUpForItsManaCost(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Growing Dread"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Library)
	ctl := engine.NewScriptedController()
	resolveLine(t, g, p, ctl, "DB$ Manifest")
	if b := g.Card(bears); b.Zone != engine.Battlefield || !b.IsFaceDown() {
		t.Fatalf("manifest: zone %v face down %v", b.Zone, b.IsFaceDown())
	}
	dog := g.NewCard(corpusCard(t, "Karlov Watchdog"), other, engine.Battlefield)
	g.SetTurnState(2, other, engine.Main1)
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardG)
	if g.TurnFaceUp(p, bears, 0, c) {
		t.Fatal("turned face up during Karlov Watchdog's controller's turn")
	}
	g.Move(dog, engine.Graveyard, other)
	g.SetTurnState(3, p, engine.Main1)
	if !g.TurnFaceUp(p, bears, 0, c) {
		t.Fatalf("turn face up for {1}{G} failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Card(bears).Counters.Count(engine.P1P1); got != 1 {
		t.Errorf("%d +1/+1 counters, want Growing Dread's 1", got)
	}
	if !engine.Matches(g, g.Card(bears), valid.Parse("Creature.turnedFaceUpThisTurn"), p, bears) {
		t.Error("Grizzly Bears is not turned face up this turn")
	}
}

// Qarsi Deceiver's {C} is spendable only on a face-down creature spell or a
// morph cost (RestrictValid$ Spell.isCastFaceDown+Creature,...,
// Static.MorphUp): it pays both for Hidden Dragonslayer, and Dream Chisel
// takes {1} off the face-down cast (ValidSpell$ Spell.isCastFaceDown).
func TestRestrictedManaPaysFaceDownCastAndMorphCost(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	qarsi := g.NewCard(corpusCard(t, "Qarsi Deceiver"), p, engine.Battlefield)
	g.NewCard(corpusCard(t, "Dream Chisel"), p, engine.Battlefield)
	slayer := g.NewCard(corpusCard(t, "Hidden Dragonslayer"), p, engine.Hand)
	c := engine.NewScriptedController()
	if !g.ActivateManaAbility(p, qarsi, 0, c) {
		t.Fatal("Qarsi Deceiver made no mana")
	}
	g.Player(p).ManaPool.Add(mana.White, 1)
	c.QueueOption(1)
	c.QueuePayGeneric(mana.ShardC)
	c.QueuePayGeneric(mana.ShardW)
	if !g.CastSpell(p, slayer, c) {
		t.Fatalf("face-down cast for {2} with Qarsi's {C}: %v", g.TakePendingError())
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("pool %d after the face-down cast, want 0", got)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	g.SetTurnState(3, p, engine.Main1)
	g.Card(qarsi).Tapped = false
	if !g.ActivateManaAbility(p, qarsi, 0, c) {
		t.Fatal("Qarsi Deceiver made no mana")
	}
	g.Player(p).ManaPool.Add(mana.White, 2)
	c.QueuePayGeneric(mana.ShardC)
	c.QueuePayGeneric(mana.ShardW)
	c.QueueTargets(nil)
	if !g.TurnFaceUp(p, slayer, 0, c) {
		t.Fatalf("morph cost {2}{W} with Qarsi's {C}: %v", g.TakePendingError())
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("pool %d after turning up, want 0", got)
	}
}

// Overgrown Zealot's two mana of one color are spendable only on turning
// permanents face up (RestrictValid$ Static.isTurnFaceUp): they pay a
// manifested Grizzly Bears' {1}{G}.
func TestTurnFaceUpRestrictedManaPays(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	zealot := g.NewCard(corpusCard(t, "Overgrown Zealot"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Library)
	ctl := engine.NewScriptedController()
	resolveLine(t, g, p, ctl, "DB$ Manifest")
	c := engine.NewScriptedController()
	c.QueueManaColor(mana.Green)
	if !g.ActivateManaAbility(p, zealot, 1, c) {
		t.Fatalf("Overgrown Zealot made no mana: %v", g.TakePendingError())
	}
	c.QueuePayGeneric(mana.ShardG)
	if !g.TurnFaceUp(p, bears, 0, c) {
		t.Fatalf("turn face up with Zealot's mana: %v", g.TakePendingError())
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("pool %d after turning up, want 0", got)
	}
}
