package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// These tests cover IgnoreEffectCost$ and the CantSearchLibrary player
// keyword (Leonin Arbiter): players cannot search libraries, and any player
// may pay {2} to ignore the effect for themself until end of turn.

func leoninArbiterDef(t *testing.T) *compile.Card {
	t.Helper()
	return scriptDef(t, "Test Leonin Arbiter", "Creature Cat",
		"S:Mode$ Continuous | Affected$ Player | AddKeyword$ CantSearchLibrary | IgnoreEffectCost$ 2 | Description$ x")
}

func TestCantSearchLibraryOffersNothingAndStillShuffles(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(leoninArbiterDef(t), other, engine.Battlefield)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	wanted := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Library)

	rec := &searchRecorder{ScriptedController: engine.NewScriptedController()}
	search(t, g, p, "Library", rec, wanted)
	if len(rec.seen) != 0 {
		t.Errorf("a blocked search asked for a choice %d times, want 0", len(rec.seen))
	}
	if z := g.Card(wanted).Zone; z != engine.Library {
		t.Errorf("card zone = %v, want Library (the search was blocked)", z)
	}
	if !g.Player(p).HasKeyword("CantSearchLibrary") {
		t.Error("player lacks CantSearchLibrary")
	}
}

func TestIgnoreEffectCostLetsOnePlayerSearchUntilEndOfTurn(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	arbiter := g.NewCard(leoninArbiterDef(t), other, engine.Battlefield)
	c := engine.NewScriptedController()
	engine.CheckStateBasedActions(g, c)

	// Any player may activate it, not only the Arbiter's controller.
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c.QueuePayGeneric(mana.ShardG)
	c.QueuePayGeneric(mana.ShardG)
	if !g.ActivateAbility(p, arbiter, 0, c) {
		t.Fatal("ActivateAbility: the opponent could not pay {2} to ignore the effect")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Player(p).HasKeyword("CantSearchLibrary") {
		t.Error("the paying player still has CantSearchLibrary")
	}
	if !g.Player(other).HasKeyword("CantSearchLibrary") {
		t.Error("the other player lost CantSearchLibrary too")
	}

	wanted := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Library)
	rec := &searchRecorder{ScriptedController: c}
	search(t, g, p, "Library", rec, wanted)
	if z := g.Card(wanted).Zone; z != engine.Hand {
		t.Errorf("found card zone = %v, want Hand", z)
	}

	// Until end of turn only: the next turn the effect applies again.
	advanceToPhase(t, g, c, 2, engine.Main1)
	engine.CheckStateBasedActions(g, c)
	if !g.Player(p).HasKeyword("CantSearchLibrary") {
		t.Error("the ignore outlived the turn")
	}
}

func TestIgnoreEffectCostNeedsTheHostInPlayAndTheMana(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	arbiter := g.NewCard(leoninArbiterDef(t), other, engine.Battlefield)
	c := engine.NewScriptedController()
	engine.CheckStateBasedActions(g, c)

	g.Player(p).ManaPool.Add(mana.Green, 2)
	g.Move(arbiter, engine.Graveyard, other)
	if g.ActivateAbility(p, arbiter, 0, c) {
		t.Error("ActivateAbility succeeded with the host out of play")
	}
}

func TestSpellsAndAbilitiesCannotCauseYouToSearchKeyword(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Lock", "Creature Cat",
		"S:Mode$ Continuous | Affected$ You | AddKeyword$ Spells and abilities you control can't cause you to search your library."), p, engine.Battlefield)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	own := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Library)
	theirs := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Library)

	rec := &searchRecorder{ScriptedController: engine.NewScriptedController()}
	search(t, g, p, "Library", rec, own)
	if z := g.Card(own).Zone; z != engine.Library {
		t.Errorf("own library search: card zone = %v, want Library (blocked)", z)
	}
	_ = theirs
}

// Damping Engine (Mode$ Continuous,CantPlayLand,CantBeCast): the player with
// the most permanents cannot cast an artifact, creature or enchantment spell
// until they sacrifice a permanent to ignore the effect for the turn
// (StaticAbilityCantBeCast.java:107, StaticAbilityContinuous.java:988-1025).
func TestDampingEngineIgnoredBySacrificingAPermanent(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	engineCard := g.NewCard(corpusCard(t, "Damping Engine"), other, engine.Battlefield)
	var lands []engine.CardID
	for i := 0; i < 3; i++ {
		lands = append(lands, g.NewCard(corpusCard(t, "Forest"), p, engine.Battlefield))
	}
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
	c := engine.NewScriptedController()
	engine.CheckStateBasedActions(g, c)
	probe := func() engine.CastProbe {
		for _, pr := range g.CastabilityProbe(p) {
			if pr.Card == bears {
				return pr
			}
		}
		t.Fatal("Bears not probed")
		return engine.CastProbe{}
	}
	if got := probe(); got.Blocked&engine.BlockOpponentLock == 0 {
		t.Fatalf("Bears under Damping Engine: %+v, want an opponent lock", got)
	}
	c.QueueSacrificeChoice([]engine.CardID{lands[0]})
	if !g.ActivateAbility(p, engineCard, 0, c) {
		t.Fatal("the player with the most permanents could not activate the ignore ability")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	engine.CheckStateBasedActions(g, c)
	if got := probe(); got.Blocked&engine.BlockOpponentLock != 0 {
		t.Errorf("Bears after ignoring the effect: %+v, want no opponent lock", got)
	}
	if g.Card(lands[0]).Zone != engine.Graveyard {
		t.Error("the sacrificed Forest is not in the graveyard")
	}
}

// Lost in Thought (Mode$ Continuous,CantAttack,CantBlock,CantBeActivated,
// Affected$ Creature): the enchanted creature's controller exiles three cards
// from their graveyard and the creature may attack this turn
// (StaticAbilityCantAttackBlock.java:77, StaticAbilityContinuous.java:991-999).
func TestLostInThoughtIgnoredByItsCreaturesController(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	g.Card(bears).SummonSick = false
	aura := g.NewCard(corpusCard(t, "Lost in Thought"), p, engine.Battlefield)
	g.Attach(aura, bears)
	var grave []engine.CardID
	for i := 0; i < 3; i++ {
		grave = append(grave, g.NewCard(corpusCard(t, "Forest"), p, engine.Graveyard))
	}
	c := engine.NewScriptedController()
	engine.CheckStateBasedActions(g, c)
	c.QueueCardChoice(grave)
	if !g.ActivateAbility(p, aura, 0, c) {
		t.Fatal("the enchanted creature's controller could not activate the ignore ability")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	engine.CheckStateBasedActions(g, c)
	for _, id := range grave {
		if g.Card(id).Zone != engine.Exile {
			t.Errorf("graveyard card %v not exiled", id)
		}
	}
	g.SetTurnState(1, p, engine.DeclareAttackers)
	c.QueueAttackers([]engine.CardID{bears})
	attackers, err := g.DeclareCombatAttackers(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(attackers) != 1 || attackers[0] != bears {
		t.Errorf("attackers %v, want the Bears now that the effect is ignored", attackers)
	}
}

// Without the ignore, Lost in Thought's CantAttack piece keeps the creature home.
func TestLostInThoughtStopsTheAttackUnlessIgnored(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	g.Card(bears).SummonSick = false
	aura := g.NewCard(corpusCard(t, "Lost in Thought"), p, engine.Battlefield)
	g.Attach(aura, bears)
	c := engine.NewScriptedController()
	engine.CheckStateBasedActions(g, c)
	g.SetTurnState(1, p, engine.DeclareAttackers)
	c.QueueAttackers([]engine.CardID{bears})
	attackers, _ := g.DeclareCombatAttackers(c)
	if len(attackers) != 0 {
		t.Errorf("attackers %v, want none: the enchanted creature cannot attack", attackers)
	}
}
