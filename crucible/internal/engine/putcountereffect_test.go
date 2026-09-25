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

// etbPutCounterTriggerDefParams builds a *compile.Card whose own "when
// CARDNAME enters" trigger runs DB$ PutCounter with the given params string
// appended -- putCounterEffect itself is unexported, so every case here is
// driven through the real cast+resolve pipeline rather than calling it
// directly (TEST-1), the identical reason etbPumpTriggerDefParams
// (pumpeffect_test.go) is.
func etbPutCounterTriggerDefParams(t *testing.T, name, extraParams string, svars map[string]string) *compile.Card {
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
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigCounter",
	}
	raw.Faces[0].SVars.Set("TrigCounter", "DB$ PutCounter | "+extraParams)
	for name, body := range svars {
		raw.Faces[0].SVars.Set(name, body)
	}

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// castETBPutCounter casts def (built by etbPutCounterTriggerDefParams) for p
// and resolves the stack, returning the creature's own CardID and
// ResolveStack's own error.
func castETBPutCounter(t *testing.T, g *engine.Game, p engine.PlayerID, def *compile.Card) (engine.CardID, error) {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	c := engine.NewScriptedController()
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	return creature, g.ResolveStack(engine.NewRegistry(), c)
}

// TestPutCounterEffectDefinedSelfAddsCounter proves the corpus's single
// largest real Defined$ shape: Defined$ Self adds CounterNum$ counters of
// CounterType$ to the ability's own host.
func TestPutCounterEffectDefinedSelfAddsCounter(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	creature, err := castETBPutCounter(t, g, p, etbPutCounterTriggerDefParams(t, "Test Self Counter", "Defined$ Self | CounterType$ P1P1 | CounterNum$ 2", nil))
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Card(creature).Counters.Count(engine.P1P1); n != 2 {
		t.Errorf("P1P1 count = %d, want 2", n)
	}
}

// TestPutCounterEffectDefaultCounterNumIsOne proves CounterNum$'s own Java
// default -- getParamOrDefault("CounterNum", "1") -- rather than treating an
// absent CounterNum$ as zero counters.
func TestPutCounterEffectDefaultCounterNumIsOne(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	creature, err := castETBPutCounter(t, g, p, etbPutCounterTriggerDefParams(t, "Test Default CounterNum", "Defined$ Self | CounterType$ P1P1", nil))
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Card(creature).Counters.Count(engine.P1P1); n != 1 {
		t.Errorf("P1P1 count = %d, want 1 (CounterNum$'s own default)", n)
	}
}

// TestPutCounterEffectDefinedYouAddsPlayerCounter proves the player-shaped
// Defined$ dispatch: Defined$ You adds to the ability's own controller
// directly, not to any card.
func TestPutCounterEffectDefinedYouAddsPlayerCounter(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	if _, err := castETBPutCounter(t, g, p, etbPutCounterTriggerDefParams(t, "Test Player Counter", "CounterType$ ENERGY | CounterNum$ 3 | Defined$ You", nil)); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Player(p).Counters.Count(engine.CounterType("ENERGY")); n != 3 {
		t.Errorf("p's ENERGY count = %d, want 3", n)
	}
}

// TestPutCounterEffectCanonicalizesCounterTypeCase proves CounterType$ is
// uppercased before it becomes a Counters key: a mixed-case corpus value
// ("Stun", 74 real lines) must land on the identical key an all-caps one
// ("STUN", 23 real lines) would, not a separate kind of counter tracked
// alongside it.
func TestPutCounterEffectCanonicalizesCounterTypeCase(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	creature, err := castETBPutCounter(t, g, p, etbPutCounterTriggerDefParams(t, "Test Mixed Case", "Defined$ Self | CounterType$ Stun | CounterNum$ 2", nil))
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Card(creature).Counters.Count(engine.Stun); n != 2 {
		t.Errorf("Stun count (canonical STUN key) = %d, want 2", n)
	}
	if n := g.Card(creature).Counters.Count(engine.CounterType("Stun")); n != 0 {
		t.Errorf("literal %q key count = %d, want 0 -- CounterType$ must be uppercased before storage", "Stun", n)
	}
}

// TestPutCounterEffectResolvesNamedCounterNumSVar proves CounterNum$
// resolves a named SVar reference through resolveNamedAmount (amount.go),
// not just a plain integer.
func TestPutCounterEffectResolvesNamedCounterNumSVar(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := etbPutCounterTriggerDefParams(t, "Test Named CounterNum", "Defined$ Self | CounterType$ P1P1 | CounterNum$ X", map[string]string{"X": "5"})
	creature, err := castETBPutCounter(t, g, p, def)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Card(creature).Counters.Count(engine.P1P1); n != 5 {
		t.Errorf("P1P1 count = %d, want 5 -- CounterNum$ X must resolve through the named SVar X:5", n)
	}
}

// TestPutCounterEffectMissingCounterTypeErrors proves a missing CounterType$
// is a real error rather than a silent no-op (GO-7).
func TestPutCounterEffectMissingCounterTypeErrors(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	_, err := castETBPutCounter(t, g, p, etbPutCounterTriggerDefParams(t, "Test No CounterType", "CounterNum$ 1", nil))
	if err == nil {
		t.Fatal("ResolveStack: got nil error, want one naming CounterType")
	}
	if !strings.Contains(err.Error(), "CounterType") {
		t.Errorf("ResolveStack error = %q, want it to name CounterType$", err.Error())
	}
}

// TestPutCounterEffectRejectsCommaListCounterType proves a multi-type list
// (an interactive choice among several kinds, this port's own missing
// PlayerController hook) fails loudly rather than picking the first one.
func TestPutCounterEffectRejectsCommaListCounterType(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	creature, err := castETBPutCounter(t, g, p, etbPutCounterTriggerDefParams(t, "Test Comma List", "CounterType$ P1P1,M1M1 | CounterNum$ 1", nil))
	if err == nil {
		t.Fatal("ResolveStack: got nil error, want one naming CounterType")
	}
	if !strings.Contains(err.Error(), "CounterType") {
		t.Errorf("ResolveStack error = %q, want it to name CounterType$", err.Error())
	}
	if g.Card(creature).Counters.Any() {
		t.Error("creature has a counter, want none -- a rejected line must not guess which type to add")
	}
}

// TestPutCounterEffectRejectsSubAbilityChain proves SubAbility$ (no chaining
// mechanism exists yet) is a real error rather than silently dropping the
// chained ability (PORT-8/GO-7).
// TestPutCounterEffectChainsIntoSubAbility proves SubAbility$ no longer
// blocks PutCounter's own resolution now that resolveSubAbility
// (subability.go) exists -- well_rested.txt's own real shape (PutCounter
// chaining into GainLife), Defined$ Self named explicitly on both this
// port's own line and the real card's (Defined$ absent on the real card's
// own GainLife half defaults to You in Java, a gap this port's own
// definedPlayers does not close yet -- naming it explicitly here sidesteps
// that separate, unrelated issue).
func TestPutCounterEffectChainsIntoSubAbility(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := etbPutCounterTriggerDefParams(t, "Test SubAbility",
		"Defined$ Self | CounterType$ P1P1 | CounterNum$ 2 | SubAbility$ DBGainLife",
		map[string]string{"DBGainLife": "DB$ GainLife | Defined$ You | LifeAmount$ 3"})
	creature, err := castETBPutCounter(t, g, p, def)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Card(creature).Counters.Count("P1P1"); n != 2 {
		t.Errorf("P1P1 counters = %d, want 2 -- PutCounter's own body must still run", n)
	}
	if g.Player(p).Life != 23 {
		t.Errorf("p's life = %d, want 23 -- the chained GainLife must run too", g.Player(p).Life)
	}
}

// TestPutCounterEffectRejectsValidTgts proves putCounterEffect itself still
// rejects a real target: resolveTargets (targeting.go) now resolves
// ValidTgts$ generically before this ability is even pushed, so the target
// (the casting creature itself, the only "Creature" on the battlefield at
// push time) is chosen without issue, but putCounterEffect has not been
// extended to consume Targeted (defined.go) yet -- its own blocked-param
// list still names ValidTgts$, and this proves that check still fires.
func TestPutCounterEffectRejectsValidTgts(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := etbPutCounterTriggerDefParams(t, "Test ValidTgts", "CounterType$ P1P1 | CounterNum$ 1 | ValidTgts$ Creature", nil)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(creature)})
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	err := g.ResolveStack(engine.NewRegistry(), c)
	if err == nil {
		t.Fatal("ResolveStack: got nil error, want one naming ValidTgts")
	}
	if !strings.Contains(err.Error(), "ValidTgts") {
		t.Errorf("ResolveStack error = %q, want it to name ValidTgts$", err.Error())
	}
}

// TestPutCounterEffectFiresWhenConditionCheckSVarIsMet proves
// subAbilityConditionMet (condition.go) gates PutCounter's own resolution
// the identical way it gates Pump's/DealDamage's/GainLife's.
func TestPutCounterEffectFiresWhenConditionCheckSVarIsMet(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := etbPutCounterTriggerDefParams(t, "Test Condition Met",
		"Defined$ Self | CounterType$ P1P1 | CounterNum$ 1 | ConditionCheckSVar$ X | ConditionSVarCompare$ GE1", map[string]string{"X": "1"})
	creature, err := castETBPutCounter(t, g, p, def)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Card(creature).Counters.Count(engine.P1P1); n != 1 {
		t.Errorf("P1P1 count = %d, want 1 -- ConditionCheckSVar$ X | ConditionSVarCompare$ GE1 holds (X is 1)", n)
	}
}

// TestPutCounterEffectNoOpsWhenConditionCheckSVarIsNotMet proves the
// negative control: X GE1 fails (X is 0), so the ability does nothing.
func TestPutCounterEffectNoOpsWhenConditionCheckSVarIsNotMet(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := etbPutCounterTriggerDefParams(t, "Test Condition Unmet",
		"CounterType$ P1P1 | CounterNum$ 1 | ConditionCheckSVar$ X | ConditionSVarCompare$ GE1", map[string]string{"X": "0"})
	creature, err := castETBPutCounter(t, g, p, def)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(creature).Counters.Any() {
		t.Error("creature has a counter, want none -- ConditionCheckSVar$ X | ConditionSVarCompare$ GE1 fails (X is 0)")
	}
}

// TestPutCounterEffectEmitsCounterChangedEventForNamedType proves
// putCounterEffect wires into emitCounterChanged (event.go) for a
// CounterType among the eight named constants.
func TestPutCounterEffectEmitsCounterChangedEventForNamedType(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	var sink recordingSink
	g.SetSink(&sink)

	if _, err := castETBPutCounter(t, g, p, etbPutCounterTriggerDefParams(t, "Test Event", "Defined$ Self | CounterType$ P1P1 | CounterNum$ 2", nil)); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	var saw bool
	for _, e := range sink.events {
		if e.Kind == engine.CounterChanged {
			saw = true
			if e.Amount != 2 {
				t.Errorf("CounterChanged Amount = %d, want 2", e.Amount)
			}
		}
	}
	if !saw {
		t.Error("no CounterChanged event seen for a named CounterType")
	}
}

// TestPutCounterEffectSkipsEventForUnnamedType proves the documented
// counterDetail gap (event.go): a script-written CounterType past the nine
// named constants still gets the counter (Counters.Add has no such limit)
// but emits no CounterChanged event, rather than one whose own Detail lies
// about what kind changed.
func TestPutCounterEffectSkipsEventForUnnamedType(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	var sink recordingSink
	g.SetSink(&sink)

	creature, err := castETBPutCounter(t, g, p, etbPutCounterTriggerDefParams(t, "Test Unnamed Type", "Defined$ Self | CounterType$ EXPERIENCE | CounterNum$ 1", nil))
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Card(creature).Counters.Count(engine.CounterType("EXPERIENCE")); n != 1 {
		t.Errorf("EXPERIENCE count = %d, want 1 -- the counter itself must still be added", n)
	}
	for _, e := range sink.events {
		if e.Kind == engine.CounterChanged {
			t.Error("saw a CounterChanged event for CounterType$ EXPERIENCE, want none -- counterDetail's own closed set does not cover it")
		}
	}
}

// TestPutCounterEffectSkipsZeroedCounterAfterReplacement proves a
// replacement that zeroes the amount (a "counters can't be placed" effect
// modeled as Amount$ 0) neither calls Counters.Add nor emits CounterChanged
// -- Java fires no counter-added event for zero counters, and Add(ct, 0)
// would be a visible no-op mutation for nothing.
func TestPutCounterEffectSkipsZeroedCounterAfterReplacement(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test No More Counters",
		"Event$ AddCounter | ActiveZones$ Battlefield | ValidCard$ Creature.YouCtrl | ValidCounterType$ P1P1 | ReplaceWith$ Nullify | Description$ No more counters.",
		"Nullify", "DB$ ReplaceCounter | Amount$ 0"), p, engine.Battlefield)
	var sink recordingSink
	g.SetSink(&sink)

	host := resolveLine(t, g, p, engine.NewScriptedController(),
		"DB$ PutCounter | Defined$ Self | CounterType$ P1P1 | CounterNum$ 3")
	if got := g.Card(host).Counters.Count(engine.P1P1); got != 0 {
		t.Errorf("P1P1 count = %d, want 0", got)
	}
	for _, e := range sink.events {
		if e.Kind == engine.CounterChanged {
			t.Error("saw a CounterChanged event for a zeroed counter placement, want none")
		}
	}
}
