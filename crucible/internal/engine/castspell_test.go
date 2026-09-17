package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

func artifactDefManaCost(t *testing.T, cost string) *compile.Card {
	t.Helper()
	def := &compile.Card{Name: "Test Artifact"}
	def.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Artifact")
	def.Faces[0].ManaCost = mana.MustParse(cost)
	return def
}

// CastSpell moves a creature spell to the stack and pays its cost; resolving
// it with the registry CastSpell's own effect is in moves it from the stack
// to the battlefield -- CR 601.2i's casting, then CR 608.2m/608.3g's
// resolution, as two separate calls the same way declareattackers and
// combatdamage are separate steps.
func TestCastSpellMovesCreatureThroughStackToBattlefield(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).ManaPool.Add(mana.Red, 1)
	creature := g.NewCard(creatureDefManaCost(t, "R"), p, engine.Hand)
	c := engine.NewScriptedController()

	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if g.Card(creature).Zone != engine.Stack {
		t.Fatalf("card zone after casting = %v, want Stack", g.Card(creature).Zone)
	}
	if g.StackLen() != 1 {
		t.Fatalf("StackLen() = %d, want 1", g.StackLen())
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("pool total after paying the cost = %d, want 0", got)
	}

	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(creature).Zone != engine.Battlefield {
		t.Errorf("card zone after resolving = %v, want Battlefield", g.Card(creature).Zone)
	}
	if g.StackLen() != 0 {
		t.Errorf("StackLen() after resolving = %d, want 0", g.StackLen())
	}
}

// CastSpell fires SpellCast on a successful cast -- the event schema
// (ADR-0013) has named this kind since M4, with nothing to emit it until
// now (game-state.md's "Not wired" list).
func TestCastSpellFiresSpellCast(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).ManaPool.Add(mana.Red, 1)
	creature := g.NewCard(creatureDefManaCost(t, "R"), p, engine.Hand)
	c := engine.NewScriptedController()
	var sink recordingSink
	g.SetSink(&sink)

	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	var sawCast bool
	for _, e := range sink.events {
		if e.Kind == engine.SpellCast && e.Source == creature && e.Actor == p {
			sawCast = true
		}
	}
	if !sawCast {
		t.Errorf("events = %v, want a SpellCast for %v cast by %v", sink.events, creature, p)
	}
}

// A noncreature permanent (an artifact here) resolves through
// APIPermanentNoncreature instead of APIPermanentCreature, but NewRegistry
// answers both the same way -- permanentEffect does not care which API
// named it.
func TestCastSpellMovesNoncreaturePermanentThroughStackToBattlefield(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).ManaPool.AddColorless(2)
	artifact := g.NewCard(artifactDefManaCost(t, "2"), p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardC)
	c.QueuePayGeneric(mana.ShardC)

	if !g.CastSpell(p, artifact, c) {
		t.Fatal("CastSpell failed casting an artifact with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(artifact).Zone != engine.Battlefield {
		t.Errorf("card zone after resolving = %v, want Battlefield", g.Card(artifact).Zone)
	}
}

// An unaffordable cost declines the cast -- the card never leaves hand, and
// nothing reaches the stack.
func TestCastSpellFailsWhenCostCannotBePaid(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	creature := g.NewCard(creatureDefManaCost(t, "R"), p, engine.Hand)
	c := engine.NewScriptedController()

	if g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell succeeded with no mana in the pool")
	}
	if g.Card(creature).Zone != engine.Hand {
		t.Errorf("card zone after a failed cast = %v, want Hand", g.Card(creature).Zone)
	}
	if g.StackLen() != 0 {
		t.Errorf("StackLen() after a failed cast = %d, want 0", g.StackLen())
	}
}

// An Aura needs a target to attach to at cast time (CR 601.2c), which this
// port cannot ask for yet -- castableAsPermanent declines it rather than
// casting it with nothing to attach to.
func TestCastSpellFailsForAnAura(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	aura := g.NewCard(auraDef(t), p, engine.Hand)
	c := engine.NewScriptedController()

	if g.CastSpell(p, aura, c) {
		t.Fatal("CastSpell succeeded on an Aura")
	}
}

// A land is never a spell at all (CR 305.1) -- CastSpell declines it, the
// same as PlayLand would decline a creature.
func TestCastSpellFailsForALand(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	plains := g.NewCard(landDef(t, "Plains", "Basic Land Plains"), p, engine.Hand)
	c := engine.NewScriptedController()

	if g.CastSpell(p, plains, c) {
		t.Fatal("CastSpell succeeded on a land")
	}
}

// CR 601.3a's own default timing (sorcery speed): only the active player,
// only in a main phase, only with an empty stack.
func TestCastSpellFailsWhenNotActivePlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	active, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, active, engine.Main1)
	creature := g.NewCard(creatureDefManaCost(t, "R"), other, engine.Hand)
	g.Player(other).ManaPool.Add(mana.Red, 1)
	c := engine.NewScriptedController()

	if g.CastSpell(other, creature, c) {
		t.Fatal("CastSpell succeeded for a player who is not the active player")
	}
}

func TestCastSpellFailsOutsideAMainPhase(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.CombatDamage)
	g.Player(p).ManaPool.Add(mana.Red, 1)
	creature := g.NewCard(creatureDefManaCost(t, "R"), p, engine.Hand)
	c := engine.NewScriptedController()

	if g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell succeeded outside a main phase")
	}
}

func TestCastSpellFailsWhenStackIsNotEmpty(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).ManaPool.Add(mana.Red, 1)
	creature := g.NewCard(creatureDefManaCost(t, "R"), p, engine.Hand)
	g.PushAbility(engine.Ability{})
	c := engine.NewScriptedController()

	if g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell succeeded with something already on the stack")
	}
}

func TestCastSpellFailsWhenCardIsNotInHand(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).ManaPool.Add(mana.Red, 1)
	creature := g.NewCard(creatureDefManaCost(t, "R"), p, engine.Battlefield)
	c := engine.NewScriptedController()

	if g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell succeeded on a card already on the battlefield")
	}
}

// A lone eligible target is assigned automatically -- CastSpell's own
// "nothing meaningful to decide" reasoning, the same convention
// assignAttackTargets (attack.go) already applies to a single attack target.
func TestCastSpellAuraSingleEligibleTargetAutoAssigns(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	target := g.NewCard(creatureDef(t), p, engine.Battlefield)
	aura := g.NewCard(auraDefWithEnchant(t, "Creature"), p, engine.Hand)
	c := engine.NewScriptedController()

	if !g.CastSpell(p, aura, c) {
		t.Fatal("CastSpell failed casting an Aura with exactly one legal target")
	}
	if g.Card(aura).Zone != engine.Stack {
		t.Fatalf("aura zone after casting = %v, want Stack", g.Card(aura).Zone)
	}

	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(aura).Zone != engine.Battlefield {
		t.Errorf("aura zone after resolving = %v, want Battlefield", g.Card(aura).Zone)
	}
	if host, ok := g.Card(aura).AttachedTo(); !ok || host != target {
		t.Errorf("aura attached to %v, %v, want %v, true", host, ok, target)
	}
}

// More than one eligible target asks the controller (CR 601.2c) --
// ChooseEnchantTarget's own answer, not the first one found.
func TestCastSpellAuraMultipleEligibleTargetsAsksController(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	_ = g.NewCard(creatureDef(t), p, engine.Battlefield)
	chosen := g.NewCard(creatureDef(t), p, engine.Battlefield)
	aura := g.NewCard(auraDefWithEnchant(t, "Creature"), p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueEnchantTarget(chosen)

	if !g.CastSpell(p, aura, c) {
		t.Fatal("CastSpell failed casting an Aura with two legal targets")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if host, ok := g.Card(aura).AttachedTo(); !ok || host != chosen {
		t.Errorf("aura attached to %v, %v, want %v, true", host, ok, chosen)
	}
}

// CR 601.2c: a spell requiring a target with no legal one is illegal to
// cast, not cast with nothing to point at.
func TestCastSpellAuraFailsWithNoLegalTarget(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	aura := g.NewCard(auraDefWithEnchant(t, "Creature"), p, engine.Hand)
	c := engine.NewScriptedController()

	if g.CastSpell(p, aura, c) {
		t.Fatal("CastSpell succeeded casting an Aura with no creature on the battlefield")
	}
	if g.Card(aura).Zone != engine.Hand {
		t.Errorf("aura zone after a failed cast = %v, want Hand", g.Card(aura).Zone)
	}
}

// "Enchant Player"/"Enchant Opponent" has no checkable valid.Spec
// (enchantSpec's own doc comment) -- this port cannot tell a legal host from
// an illegal one, so it declines rather than guessing.
func TestCastSpellAuraFailsForEnchantPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	aura := g.NewCard(auraDefWithEnchant(t, "Player"), p, engine.Hand)
	c := engine.NewScriptedController()

	if g.CastSpell(p, aura, c) {
		t.Fatal("CastSpell succeeded casting an Enchant Player Aura")
	}
}

// An unaffordable cost declines the cast even after a target was already
// chosen (CR 601.2c precedes 601.2i) -- the card never leaves hand, and the
// target it would have enchanted is untouched.
func TestCastSpellAuraFailsWhenCostCannotBePaid(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	target := g.NewCard(creatureDef(t), p, engine.Battlefield)
	aura := g.NewCard(auraDefWithEnchant(t, "Creature"), p, engine.Hand)
	g.Card(aura).Def.Faces[0].ManaCost = mana.MustParse("W")
	c := engine.NewScriptedController()

	if g.CastSpell(p, aura, c) {
		t.Fatal("CastSpell succeeded with no mana in the pool")
	}
	if g.Card(aura).Zone != engine.Hand {
		t.Errorf("aura zone after a failed cast = %v, want Hand", g.Card(aura).Zone)
	}
	if atts := g.Card(target).Attachments(); len(atts) != 0 {
		t.Errorf("target's attachments = %v, want none", atts)
	}
}
