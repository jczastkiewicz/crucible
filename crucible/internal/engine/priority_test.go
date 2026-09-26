package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// resolvedOrder returns the Source of every AbilityResolved event in sink,
// in the order they resolved -- the only way to tell CR 117.4's
// one-per-round grain and CR 117.3c's "caster keeps priority" apart from a
// wrong-but-final-state-matches implementation (a mutation that resolves
// the whole stack per round, or one that always advances priority after an
// action, both still land on the same final life/damage totals in these
// scenarios -- only the order tells them apart).
func resolvedOrder(sink *recordingSink) []engine.CardID {
	var order []engine.CardID
	for _, e := range sink.events {
		if e.Kind == engine.AbilityResolved {
			order = append(order, e.Source)
		}
	}
	return order
}

func cardIDsEqual(got, want []engine.CardID) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestPassPriorityRespondsLastInFirstOut proves CR 117.3b/117.4 together:
// A casts a burn sorcery, passes, B responds with an Instant at A's own
// creature, A casts a second Instant before passing again (CR 117.3c). The
// response window between resolutions is what makes the order Bolt, second
// Instant, sorcery rather than the to-empty order Bolt, sorcery, second
// Instant a wrong implementation that resolves everything in one round
// would produce. Neither spell's own target is another spell on the stack
// (CR 608.2b's own general fizzle check is still Aura-only, ADR-0018), so
// this does not depend on the still-unbuilt general re-check.
func TestPassPriorityRespondsLastInFirstOut(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	g.Player(a).Life, g.Player(b).Life = 20, 20
	var sink recordingSink
	g.SetSink(&sink)

	creature := g.NewCard(creatureDefPT(t, "4", "4"), a, engine.Battlefield)
	sorcery := g.NewCard(sorceryDefWithAbility(t, "Test Burn", "0", "SP$ DealDamage | ValidTgts$ Player | NumDmg$ 2"), a, engine.Hand)
	bolt := g.NewCard(instantDefWithAbility(t, "Test Bolt", "0", "SP$ DealDamage | ValidTgts$ Any | NumDmg$ 3"), b, engine.Hand)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(b)})      // a's sorcery targets b
	c.QueueTargets([]engine.EntityID{engine.CardEntity(creature)}) // b's bolt targets a's creature
	c.QueueAction(a, engine.Action{Kind: engine.ActionCast, Card: sorcery})
	c.QueueAction(b, engine.Action{Kind: engine.ActionCast, Card: bolt})

	if err := g.PassPriority(engine.NewRegistry(), c); err != nil {
		t.Fatalf("PassPriority: %v", err)
	}

	want := []engine.CardID{bolt, sorcery}
	if got := resolvedOrder(&sink); !cardIDsEqual(got, want) {
		t.Errorf("resolved order = %v, want %v (Bolt resolves in the response window before the sorcery under it)", got, want)
	}
	if got := g.Card(creature).Damage.Marked; got != 3 {
		t.Errorf("creature damage = %d, want 3", got)
	}
	if got := g.Player(b).Life; got != 18 {
		t.Errorf("b life = %d, want 18 (20 - 2)", got)
	}
	if g.StackLen() != 0 {
		t.Errorf("StackLen() = %d, want 0", g.StackLen())
	}
}

// TestPassPriorityGivesAResponseWindowBetweenEachResolution proves CR
// 117.4's real grain: only the top of the stack resolves per round, and a
// fresh round -- with a real response window -- starts before the next
// item does. a casts a sorcery (S), passes; b responds with a Bolt; both
// pass, so Bolt alone resolves (CR 117.4). Only then does a's own queue
// supply a second Instant (I2), cast into the window CR 117.3b opens
// before S resolves, landing on top of it. An implementation that resolves
// the whole stack in one round instead (S and Bolt together, no window
// before I2 is even asked for) would push I2 onto an empty stack instead,
// producing Bolt, S, I2 -- confirmed by mutating resolveTop's own caller to
// call the to-empty ResolveStack instead, which flips this order.
func TestPassPriorityGivesAResponseWindowBetweenEachResolution(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	g.Player(a).Life, g.Player(b).Life = 20, 20
	var sink recordingSink
	g.SetSink(&sink)

	sorcery := g.NewCard(sorceryDefWithAbility(t, "Test Window Sorcery", "0", "SP$ DealDamage | ValidTgts$ Player | NumDmg$ 2"), a, engine.Hand)
	bolt := g.NewCard(instantDefWithAbility(t, "Test Window Bolt", "0", "SP$ DealDamage | ValidTgts$ Player | NumDmg$ 3"), b, engine.Hand)
	second := g.NewCard(instantDefWithAbility(t, "Test Window Second", "0", "SP$ DealDamage | ValidTgts$ Player | NumDmg$ 1"), a, engine.Hand)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(b)}) // sorcery
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(b)}) // bolt
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(b)}) // second
	c.QueueAction(a, engine.Action{Kind: engine.ActionCast, Card: sorcery})
	c.QueueAction(a, engine.Action{Kind: engine.ActionPass})
	c.QueueAction(a, engine.Action{Kind: engine.ActionPass})
	c.QueueAction(a, engine.Action{Kind: engine.ActionCast, Card: second})
	c.QueueAction(b, engine.Action{Kind: engine.ActionCast, Card: bolt})

	if err := g.PassPriority(engine.NewRegistry(), c); err != nil {
		t.Fatalf("PassPriority: %v", err)
	}

	want := []engine.CardID{bolt, second, sorcery}
	if got := resolvedOrder(&sink); !cardIDsEqual(got, want) {
		t.Errorf("resolved order = %v, want %v (a real response window before each resolution)", got, want)
	}
	if got := g.Player(b).Life; got != 14 {
		t.Errorf("b life = %d, want 14 (20 - 3 - 1 - 2)", got)
	}
}

// TestPassPriorityCasterKeepsPriority proves CR 117.3c in isolation: a
// casts two Instants back to back with nobody else acting in between --
// legal because the caster keeps priority after acting, and both still
// resolve last in, first out (the second one on top) once everyone passes.
func TestPassPriorityCasterKeepsPriority(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	g.Player(a).Life, g.Player(b).Life = 20, 20
	var sink recordingSink
	g.SetSink(&sink)

	first := g.NewCard(instantDefWithAbility(t, "Test First", "0", "SP$ DealDamage | ValidTgts$ Player | NumDmg$ 1"), a, engine.Hand)
	second := g.NewCard(instantDefWithAbility(t, "Test Second", "0", "SP$ DealDamage | ValidTgts$ Player | NumDmg$ 2"), a, engine.Hand)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(b)})
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(b)})
	c.QueueAction(a, engine.Action{Kind: engine.ActionCast, Card: first})
	c.QueueAction(a, engine.Action{Kind: engine.ActionCast, Card: second})

	if err := g.PassPriority(engine.NewRegistry(), c); err != nil {
		t.Fatalf("PassPriority: %v", err)
	}

	want := []engine.CardID{second, first}
	if got := resolvedOrder(&sink); !cardIDsEqual(got, want) {
		t.Errorf("resolved order = %v, want %v (second was pushed on top of first, LIFO)", got, want)
	}
	if got := g.Player(b).Life; got != 17 {
		t.Errorf("b life = %d, want 17 (20 - 1 - 2)", got)
	}
}

// TestPassPriorityErrorsOnIllegalTiming proves ADR-0019 Decision point 5:
// a queued action that fails CR 307.1's own timing check is a hard error,
// not a silent skip -- b, who does not have priority to cast a Sorcery
// (only an Instant may be cast as a response), is queued one anyway.
func TestPassPriorityErrorsOnIllegalTiming(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	g.Player(a).Life, g.Player(b).Life = 20, 20

	illegal := g.NewCard(sorceryDefWithAbility(t, "Test Illegal Sorcery", "0", "SP$ DealDamage | ValidTgts$ Player | NumDmg$ 2"), b, engine.Hand)

	c := engine.NewScriptedController()
	c.QueueAction(b, engine.Action{Kind: engine.ActionCast, Card: illegal})

	if err := g.PassPriority(engine.NewRegistry(), c); err == nil {
		t.Fatal("PassPriority returned nil error for a Sorcery cast outside its own turn/main phase")
	}
}

// TestPassPriorityDeclinesPlaneswalkerAbilityOutsideMainPhase proves CR
// 606.3's own timing restriction, new with ADR-0019: a loyalty ability
// (Planeswalker$) is sorcery speed, the same as an explicit SorcerySpeed$
// activated ability -- queuing one during combat is an illegal-timing
// error, not a successful activation.
func TestPassPriorityDeclinesPlaneswalkerAbilityOutsideMainPhase(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.SetTurnState(1, a, engine.CombatBegin)
	g.Player(a).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := planeswalkerDefWithAbility(t, "Test Loyalty Timing", "4",
		"AB$ GainLife | Cost$ AddCounter<1/LOYALTY> | Planeswalker$ True | Defined$ You | LifeAmount$ 2")
	pw := g.NewCard(def, a, engine.Battlefield)
	g.Card(pw).Counters.Add(engine.Loyalty, 4)

	c := engine.NewScriptedController()
	c.QueueAction(a, engine.Action{Kind: engine.ActionActivate, Card: pw, AbilityIndex: 0})

	if err := g.PassPriority(engine.NewRegistry(), c); err == nil {
		t.Fatal("PassPriority returned nil error for a Planeswalker$ ability activated during combat")
	}
}

// TestPassPriorityActivatesInstantSpeedAbilityAsAResponse exercises
// ActionActivate through PassPriority for the first time: b, with no cards
// in hand, responds to a's cast with an instant-speed activated ability
// instead of a spell.
func TestPassPriorityActivatesInstantSpeedAbilityAsAResponse(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	g.Player(a).Life, g.Player(b).Life = 20, 20
	var sink recordingSink
	g.SetSink(&sink)

	sorcery := g.NewCard(sorceryDefWithAbility(t, "Test Burn", "0", "SP$ DealDamage | ValidTgts$ Player | NumDmg$ 2"), a, engine.Hand)
	responder := g.NewCard(creatureDefWithAbility(t, "Test Responder", "AB$ GainLife | Cost$ T | Defined$ You | LifeAmount$ 1"), b, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(b)})
	c.QueueAction(a, engine.Action{Kind: engine.ActionCast, Card: sorcery})
	c.QueueAction(b, engine.Action{Kind: engine.ActionActivate, Card: responder, AbilityIndex: 0})

	if err := g.PassPriority(engine.NewRegistry(), c); err != nil {
		t.Fatalf("PassPriority: %v", err)
	}

	want := []engine.CardID{responder, sorcery}
	if got := resolvedOrder(&sink); !cardIDsEqual(got, want) {
		t.Errorf("resolved order = %v, want %v (the activated ability resolves before the sorcery under it)", got, want)
	}
	if got := g.Player(b).Life; got != 19 {
		t.Errorf("b life = %d, want 19 (20 + 1 - 2)", got)
	}
}

// TestPassPriorityMatchesResolveStackWithNoResponse proves parity: with
// nothing queued, a full priority round resolves a lone stack item exactly
// the same way ResolveStack already does.
func TestPassPriorityMatchesResolveStackWithNoResponse(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	g.Player(a).Life, g.Player(b).Life = 20, 20

	sorcery := g.NewCard(sorceryDefWithAbility(t, "Test Plain Burn", "0", "SP$ DealDamage | ValidTgts$ Player | NumDmg$ 5"), a, engine.Hand)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(b)})
	if !g.CastSpell(a, sorcery, c) {
		t.Fatal("CastSpell failed")
	}

	if err := g.PassPriority(engine.NewRegistry(), c); err != nil {
		t.Fatalf("PassPriority: %v", err)
	}
	if got := g.Player(b).Life; got != 15 {
		t.Errorf("b life = %d, want 15", got)
	}
	if g.StackLen() != 0 {
		t.Errorf("StackLen() = %d, want 0", g.StackLen())
	}
}
