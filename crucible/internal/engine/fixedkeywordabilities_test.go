package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// The keywords whose CardFactoryUtil branch is a fixed trigger, static or
// replacement (ADR-0038): Ingest, Provoke, Decayed, Gravestorm, Undaunted and
// Ravenous, played through the real corpus cards.

// attackTo runs a's combat with c answering every decision: attackers,
// blocks, combat damage, then the end of combat step, resolving the stack
// after each.
func attackTo(t *testing.T, g *engine.Game, a engine.PlayerID, c *engine.ScriptedController, attackers ...engine.CardID) {
	t.Helper()
	for _, id := range attackers {
		g.Card(id).SummonSick = false
	}
	resolve := func(step string) {
		t.Helper()
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("ResolveStack after %s: %v", step, err)
		}
	}
	if _, err := g.DeclareCombatAttackers(c); err != nil {
		t.Fatalf("DeclareCombatAttackers: %v", err)
	}
	resolve("attackers")
	if _, err := g.DeclareCombatBlockers(c); err != nil {
		t.Fatalf("DeclareCombatBlockers: %v", err)
	}
	resolve("blockers")
	g.DealCombatDamage(c)
	resolve("damage")
	g.SetTurnState(1, a, engine.CombatDamage)
	g.AdvancePhase(c)
	resolve("end of combat")
}

// Ingest (CR 702.115a): combat damage to a player exiles the top card of
// their library.
func TestIngestExilesTheTopCard(t *testing.T) {
	t.Parallel()

	g, a, b := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, a, 3)
	lib := libraryCards(t, g, b, 3)
	g.SetTurnState(1, a, engine.Main1)
	crawler := g.NewCard(corpusCard(t, "Sludge Crawler"), a, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueAttackers([]engine.CardID{crawler})
	c.QueueBlocks(nil)
	attackTo(t, g, a, c, crawler)
	exiled := 0
	for _, id := range lib {
		if g.Card(id).Zone == engine.Exile {
			exiled++
		}
	}
	if exiled != 1 {
		t.Errorf("%d of the defender's library cards exiled, want 1", exiled)
	}
}

// Provoke (CR 702.39a): the attacker's controller may have a creature the
// defending player controls untap and block it.
func TestProvokeUntapsTheTargetToBlock(t *testing.T) {
	t.Parallel()

	g, a, b := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, a, engine.Main1)
	deftblade := g.NewCard(corpusCard(t, "Deftblade Elite"), a, engine.Battlefield)
	bear := g.NewCard(corpusCard(t, "Grizzly Bears"), b, engine.Battlefield)
	g.Card(bear).Tapped = true
	c := engine.NewScriptedController()
	c.QueueAttackers([]engine.CardID{deftblade})
	c.QueueTargets([]engine.EntityID{engine.CardEntity(bear)})
	c.QueueConfirmOptionalTrigger(true)
	c.QueueBlocks([]engine.Block{{Blocker: bear, Attacker: deftblade}})
	attackTo(t, g, a, c, deftblade)
	// A tapped creature cannot block (CR 509.1a): the block above was legal
	// only because provoke untapped the Bears, which then killed the 1/1.
	if got := g.Card(deftblade).Zone; got != engine.Graveyard {
		t.Errorf("Deftblade Elite in %v, want Graveyard (blocked by the untapped Bears)", got)
	}
}

// Decayed (CR 702.147a): a creature with decayed is sacrificed at end of
// combat after it attacks.
func TestDecayedIsSacrificedAfterAttacking(t *testing.T) {
	t.Parallel()

	g, a, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, a, engine.Main1)
	rakshasa := g.NewCard(corpusCard(t, "Rot-Curse Rakshasa"), a, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueAttackers([]engine.CardID{rakshasa})
	c.QueueBlocks(nil)
	attackTo(t, g, a, c, rakshasa)
	if got := g.Card(rakshasa).Zone; got != engine.Graveyard {
		t.Errorf("Rot-Curse Rakshasa in %v after combat, want Graveyard", got)
	}
}

// Gravestorm (CR 702.69a): the spell is copied once per permanent put into a
// graveyard from the battlefield this turn.
func TestGravestormCopiesPerPermanentThatDied(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	ctl := engine.NewScriptedController()
	for range 2 {
		victim := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
		if _, err := resolveNow(t, g, p, ctl, []engine.EntityID{engine.CardEntity(victim)}, "DB$ Destroy | ValidTgts$ Creature"); err != nil {
			t.Fatalf("Destroy: %v", err)
		}
	}
	spell := g.NewCard(corpusCard(t, "Follow the Bodies"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Blue, 3)
	ctl.QueuePayGeneric(mana.ShardU)
	ctl.QueuePayGeneric(mana.ShardU)
	castThenResolve(t, g, p, spell, ctl)
	clues := 0
	for _, id := range g.Zone(engine.Battlefield, p).Cards() {
		if g.Card(id).IsToken {
			clues++
		}
	}
	if clues != 3 {
		t.Errorf("%d Clues, want 3 (the spell and two copies)", clues)
	}
}

// Undaunted (CR 702.125a): the spell costs {1} less for each opponent.
func TestUndauntedCostsOneLessPerOpponent(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	spell := g.NewCard(corpusCard(t, "Sublime Exhalation"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.White, 6)
	ctl := engine.NewScriptedController()
	for range 5 {
		ctl.QueuePayGeneric(mana.ShardW)
	}
	castThenResolve(t, g, p, spell, ctl)
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("%d mana left, want 0 (6W less 1 for the one opponent)", got)
	}
}

// Ravenous (CR 702.156a): the creature enters with X +1/+1 counters and, with
// X 5 or more, its controller draws a card.
func TestRavenousEntersWithXCountersAndDraws(t *testing.T) {
	t.Parallel()

	for _, x := range []int{2, 5} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		libraryCards(t, g, p, 2)
		g.SetTurnState(1, p, engine.Main1)
		ravener := g.NewCard(corpusCard(t, "Hormagaunt Horde"), p, engine.Hand)
		g.Player(p).ManaPool.Add(mana.Green, x+1)
		ctl := engine.NewScriptedController()
		ctl.QueuePayX(x)
		for range x {
			ctl.QueuePayGeneric(mana.ShardG)
		}
		hand := len(g.Zone(engine.Hand, p).Cards())
		castThenResolve(t, g, p, ravener, ctl)
		if err := g.TakePendingError(); err != nil {
			t.Fatalf("X=%d: %v", x, err)
		}
		if got := g.Card(ravener).Counters.Count(engine.P1P1); got != x {
			t.Errorf("X=%d: +1/+1 counters = %d, want %d", x, got, x)
		}
		drew := len(g.Zone(engine.Hand, p).Cards()) - (hand - 1)
		if want := map[bool]int{true: 1, false: 0}[x >= 5]; drew != want {
			t.Errorf("X=%d: drew %d, want %d", x, drew, want)
		}
	}
}
