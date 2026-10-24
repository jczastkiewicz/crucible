package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// etbMoveCounterTriggerDefParams builds a *compile.Card whose own "when
// CARDNAME enters" trigger runs DB$ MoveCounter with the given params
// string appended -- etbDestroyTriggerDefParams' own shape
// (destroyeffect_test.go).
func etbMoveCounterTriggerDefParams(t *testing.T, name, extraParams string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "1", "1"
	raw.Faces[0].ManaCost = mana.MustParse("G")
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigMoveCounter",
	}
	raw.Faces[0].SVars.Set("TrigMoveCounter", "DB$ MoveCounter | "+extraParams)

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// castETBMoveCounter casts def for p on controller c and resolves the stack
// -- castETBDestroy's own shape (destroyeffect_test.go).
func castETBMoveCounter(t *testing.T, g *engine.Game, p engine.PlayerID, def *compile.Card, c *engine.ScriptedController) (engine.CardID, error) {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	return creature, g.ResolveStack(engine.NewRegistry(), c)
}

// TestMoveCounterEffectSourceSelfMovesLiteralAmountToTarget proves the
// corpus's own dominant real shape: Source$ Self (default) moves a literal
// CounterNum$ of a literal CounterType$ to a ValidTgts$ destination.
func TestMoveCounterEffectSourceSelfMovesLiteralAmountToTarget(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	dest := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	def := etbMoveCounterTriggerDefParams(t, "Test Move Literal", "CounterType$ P1P1 | CounterNum$ 2 | ValidTgts$ Creature")
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(dest)})
	g.Player(p).ManaPool.Add(mana.Green, 1)
	host := g.NewCard(def, p, engine.Hand)
	// preset the host's own counters while it still sits in Hand -- Counters
	// is a plain per-card field, not zone-gated, castETBRemoveCounterSelf's
	// own identical idiom (removecountereffect_test.go).
	g.Card(host).Counters.Add(engine.P1P1, 3)
	if !g.CastSpell(p, host, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Card(host).Counters.Count(engine.P1P1); n != 1 {
		t.Errorf("host P1P1 = %d, want 1 (3 - 2)", n)
	}
	if n := g.Card(dest).Counters.Count(engine.P1P1); n != 2 {
		t.Errorf("dest P1P1 = %d, want 2", n)
	}
}

// TestMoveCounterEffectCounterNumAllMovesEntirePile proves CounterNum$ All
// moves every counter of the named kind, not just one.
func TestMoveCounterEffectCounterNumAllMovesEntirePile(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	dest := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	def := etbMoveCounterTriggerDefParams(t, "Test Move All", "CounterType$ P1P1 | CounterNum$ All | ValidTgts$ Creature")
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(dest)})
	g.Player(p).ManaPool.Add(mana.Green, 1)
	host := g.NewCard(def, p, engine.Hand)
	g.Card(host).Counters.Add(engine.P1P1, 5)
	if !g.CastSpell(p, host, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Card(host).Counters.Count(engine.P1P1); n != 0 {
		t.Errorf("host P1P1 = %d, want 0", n)
	}
	if n := g.Card(dest).Counters.Count(engine.P1P1); n != 5 {
		t.Errorf("dest P1P1 = %d, want 5", n)
	}
}

// TestMoveCounterEffectCounterTypeAllMovesEveryKind proves CounterType$ All
// moves every kind the source carries, each independently capped at its own
// count.
func TestMoveCounterEffectCounterTypeAllMovesEveryKind(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	dest := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	def := etbMoveCounterTriggerDefParams(t, "Test Move AllKinds", "CounterType$ All | CounterNum$ All | ValidTgts$ Creature")
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(dest)})
	g.Player(p).ManaPool.Add(mana.Green, 1)
	host := g.NewCard(def, p, engine.Hand)
	g.Card(host).Counters.Add(engine.P1P1, 2)
	g.Card(host).Counters.Add(engine.Stun, 1)
	if !g.CastSpell(p, host, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Card(dest).Counters.Count(engine.P1P1); n != 2 {
		t.Errorf("dest P1P1 = %d, want 2", n)
	}
	if n := g.Card(dest).Counters.Count(engine.Stun); n != 1 {
		t.Errorf("dest Stun = %d, want 1", n)
	}
}

// TestMoveCounterEffectTwoTargetsMoveFromFirstToSecond proves the TargetMin$ 2
// shape (CountersMoveEffect.java:229-233): the first target is the source.
func TestMoveCounterEffectTwoTargetsMoveFromFirstToSecond(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	from := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	to := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.Card(from).Counters.Add(engine.P1P1, 2)

	def := etbMoveCounterTriggerDefParams(t, "Test Move TwoTarget", "CounterType$ P1P1 | CounterNum$ 1 | ValidTgts$ Creature | TargetMin$ 2 | TargetMax$ 2")
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(from), engine.CardEntity(to)})
	g.Player(p).ManaPool.Add(mana.Green, 1)
	host := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, host, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Card(from).Counters.Count(engine.P1P1); n != 1 {
		t.Errorf("source P1P1 = %d, want 1", n)
	}
	if n := g.Card(to).Counters.Count(engine.P1P1); n != 1 {
		t.Errorf("destination P1P1 = %d, want 1", n)
	}
}

// TestMoveCounterEffectRejectsUnresolvedParam proves
// moveCounterUnresolvedParams' own fail-loud contract (PORT-8/GO-7).
func TestMoveCounterEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	dest := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	def := etbMoveCounterTriggerDefParams(t, "Test Move NonZero", "CounterType$ P1P1 | CounterNum$ 1 | ValidTgts$ Creature | NonZero$ True")
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(dest)})
	g.Player(p).ManaPool.Add(mana.Green, 1)
	host := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, host, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved NonZero$")
	}
}

// TestMoveCounterEffectValidSourceGathersCountersFromEveryMatch proves
// ValidSource$ (Spike Cannibal): every battlefield card matching it, the
// opponent's included, gives its counters to the Defined$ destination.
func TestMoveCounterEffectValidSourceGathersCountersFromEveryMatch(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	own := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	theirs := g.NewCard(creatureDefPT(t, "2", "2"), g.Players()[1], engine.Battlefield)
	g.Card(own).Counters.Add(engine.P1P1, 2)
	g.Card(theirs).Counters.Add(engine.P1P1, 3)

	def := etbMoveCounterTriggerDefParams(t, "Test Move ValidSource", "ValidSource$ Creature | Defined$ Self | CounterType$ P1P1 | CounterNum$ All")
	c := engine.NewScriptedController()
	host, err := castETBMoveCounter(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Card(host).Counters.Count(engine.P1P1); n != 5 {
		t.Errorf("host P1P1 = %d, want 5", n)
	}
	if n := g.Card(own).Counters.Count(engine.P1P1) + g.Card(theirs).Counters.Count(engine.P1P1); n != 0 {
		t.Errorf("sources kept %d P1P1, want 0", n)
	}
}

// TestMoveCounterEffectValidSourceAnyAsksWhichSourcesAndHowMany proves
// CounterNum$ Any with ValidSource$ (Aetherborn Marauder, CountersMoveEffect.
// java:104-148, :355): the activator picks the sources, then a count per
// source; an unpicked source keeps its counters.
func TestMoveCounterEffectValidSourceAnyAsksWhichSourcesAndHowMany(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	picked := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	skipped := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.Card(picked).Counters.Add(engine.P1P1, 3)
	g.Card(skipped).Counters.Add(engine.P1P1, 2)

	def := etbMoveCounterTriggerDefParams(t, "Test Move ValidSource Any", "ValidSource$ Permanent.YouCtrl+Other | Defined$ Self | CounterType$ P1P1 | CounterNum$ Any")
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{picked})
	c.QueueNumberChoice(2)
	host, err := castETBMoveCounter(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Card(host).Counters.Count(engine.P1P1); n != 2 {
		t.Errorf("host P1P1 = %d, want 2", n)
	}
	if n := g.Card(picked).Counters.Count(engine.P1P1); n != 1 {
		t.Errorf("picked source P1P1 = %d, want 1", n)
	}
	if n := g.Card(skipped).Counters.Count(engine.P1P1); n != 2 {
		t.Errorf("unpicked source P1P1 = %d, want 2", n)
	}
}

// TestMoveCounterEffectCounterTypeAllWithAnyAsksPerKind proves Slippery
// Bogbonder's shape (:123-129, :151-153): CounterType$ All with CounterNum$ Any
// asks one number per counter kind of each chosen source.
func TestMoveCounterEffectCounterTypeAllWithAnyAsksPerKind(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	src := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.Card(src).Counters.Add(engine.P1P1, 2)
	g.Card(src).Counters.Add(engine.Stun, 1)

	def := etbMoveCounterTriggerDefParams(t, "Test Move All Any", "ValidSource$ Creature.Other+YouCtrl | Defined$ Self | CounterType$ All | CounterNum$ Any")
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{src})
	for _, n := range []int{1, 1} {
		c.QueueNumberChoice(n)
	}
	host, err := castETBMoveCounter(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(host).Counters.Count(engine.P1P1) + g.Card(host).Counters.Count(engine.Stun); got != 2 {
		t.Errorf("host holds %d counters, want 2 (one of each kind)", got)
	}
	if n := g.Card(src).Counters.Count(engine.P1P1); n != 1 {
		t.Errorf("source P1P1 = %d, want 1", n)
	}
}

// TestMoveCounterEffectTwoTargetsAnyAsksHowMany proves Bioshift's shape: the
// first target is the source and the activator chooses how many move.
func TestMoveCounterEffectTwoTargetsAnyAsksHowMany(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	from := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	to := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.Card(from).Counters.Add(engine.P1P1, 3)

	def := etbMoveCounterTriggerDefParams(t, "Test Move TwoTarget Any", "CounterType$ P1P1 | CounterNum$ Any | ValidTgts$ Creature | TargetMin$ 2 | TargetMax$ 2")
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(from), engine.CardEntity(to)})
	c.QueueNumberChoice(2)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	host := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, host, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got, want := g.Card(to).Counters.Count(engine.P1P1), 2; got != want {
		t.Errorf("destination P1P1 = %d, want %d", got, want)
	}
	if got, want := g.Card(from).Counters.Count(engine.P1P1), 1; got != want {
		t.Errorf("source P1P1 = %d, want %d", got, want)
	}
}

// TestMoveCounterTypeAnyAsksForTheKind proves CounterType$ Any
// (CountersMoveEffect.java:280-306): with CounterNum$ 1 the activator names
// one kind and one counter of it moves; with CounterNum$ Any every kind is
// offered in turn and the activator chooses how many of each.
func TestMoveCounterTypeAnyAsksForTheKind(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, num string
		option    int
		numbers   []int
		wantP1    int
		wantLoy   int
	}{
		{"one counter of the chosen kind", "1", 1, nil, 0, 1},
		{"every kind, chosen amounts", "Any", 0, []int{2, 0}, 2, 0},
	} {
		g := newGame(t, "a", "b")
		p := g.Players()[0]
		g.SetTurnState(1, p, engine.Main1)
		dest := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
		def := etbMoveCounterTriggerDefParams(t, "Test Move Any", "CounterType$ Any | CounterNum$ "+tc.num+" | ValidTgts$ Creature")
		c := engine.NewScriptedController()
		c.QueueTargets([]engine.EntityID{engine.CardEntity(dest)})
		c.QueueOption(tc.option)
		if tc.num == "Any" {
			c.QueueOption(0) // the kind left after the first
		}
		for _, n := range tc.numbers {
			c.QueueNumberChoice(n)
		}
		g.Player(p).ManaPool.Add(mana.Green, 1)
		host := g.NewCard(def, p, engine.Hand)
		g.Card(host).Counters.Add(engine.P1P1, 3)
		g.Card(host).Counters.Add(engine.CounterType("LOYALTY"), 1)
		if !g.CastSpell(p, host, c) {
			t.Fatal("CastSpell failed")
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("%s: ResolveStack: %v", tc.name, err)
		}
		if got := g.Card(dest).Counters.Count(engine.P1P1); got != tc.wantP1 {
			t.Errorf("%s: P1P1 on the target = %d, want %d", tc.name, got, tc.wantP1)
		}
		if got := g.Card(dest).Counters.Count(engine.CounterType("LOYALTY")); got != tc.wantLoy {
			t.Errorf("%s: LOYALTY on the target = %d, want %d", tc.name, got, tc.wantLoy)
		}
	}
}

// ValidDefined$ (Forgotten Ancient): the source gives counters to the
// battlefield cards the spec names, the activator choosing how many each gets
// (CountersMoveEffect.java:164-226).
func TestMoveCounterValidDefinedGivesChosenAmounts(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	dest := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	def := etbMoveCounterTriggerDefParams(t, "Test Move Valid", "Source$ Self | ValidDefined$ Creature.Other | CounterType$ P1P1 | CounterNum$ Any")
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{dest})
	c.QueueNumberChoice(2)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	host := g.NewCard(def, p, engine.Hand)
	g.Card(host).Counters.Add(engine.P1P1, 3)
	if !g.CastSpell(p, host, c) {
		t.Fatal("CastSpell failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Card(dest).Counters.Count(engine.P1P1); got != 2 {
		t.Errorf("receiver counters = %d, want 2", got)
	}
}
