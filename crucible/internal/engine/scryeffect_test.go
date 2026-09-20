package engine_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// etbScryTriggerDefParams builds a *compile.Card whose own "when CARDNAME
// enters" trigger runs DB$ Scry with the given params string appended --
// scryEffect itself is unexported, so every case here is driven through the
// real cast+resolve pipeline rather than calling it directly (TEST-1), the
// identical reason etbDiscardTriggerDefParams (discardeffect_test.go) is.
func etbScryTriggerDefParams(t *testing.T, name, extraParams string, svars map[string]string) *compile.Card {
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
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigScry",
	}
	raw.Faces[0].SVars.Set("TrigScry", "DB$ Scry | "+extraParams)
	for name, body := range svars {
		raw.Faces[0].SVars.Set(name, body)
	}

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// castETBScry casts def (built by etbScryTriggerDefParams) for p on
// controller c and resolves the stack, returning ResolveStack's own error.
// c is passed in, rather than built fresh, because a scripted scry decision
// has to be queued on it before this runs.
func castETBScry(t *testing.T, g *engine.Game, p engine.PlayerID, def *compile.Card, c *engine.ScriptedController) error {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	return g.ResolveStack(engine.NewRegistry(), c)
}

// TestScryEffectDefaultDefinedIsYou proves the corpus's own single largest
// real shape: an absent Defined$ resolves to "You" (AbilityUtils.
// getDefinedPlayers's own `changedDef = (def == null) ? "You" : ...`), not
// a rejected line the way every other M6 effect so far treats a missing
// Defined$.
func TestScryEffectDefaultDefinedIsYou(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	top := g.NewCard(nil, p, engine.Library)

	c := engine.NewScriptedController()
	c.QueueScry([]engine.CardID{top}, nil)

	if err := castETBScry(t, g, p, etbScryTriggerDefParams(t, "Test Default Defined", "ScryNum$ 1", nil), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Zone(engine.Library, p).Cards(); len(got) != 1 || got[0] != top {
		t.Errorf("library = %v, want [%v] -- an absent Defined$ must default to You", got, top)
	}
}

// TestScryEffectDefinedOpponentAffectsOpponentLibrary proves the
// player-shaped Defined$ dispatch reaches the opponent's own library, not
// the caster's.
func TestScryEffectDefinedOpponentAffectsOpponentLibrary(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, opp := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(opp).Life = 20, 20
	oppTop := g.NewCard(nil, opp, engine.Library)

	c := engine.NewScriptedController()
	c.QueueScry([]engine.CardID{oppTop}, nil)

	if err := castETBScry(t, g, p, etbScryTriggerDefParams(t, "Test Defined Opponent", "ScryNum$ 1 | Defined$ Opponent", nil), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Zone(engine.Library, opp).Cards(); len(got) != 1 || got[0] != oppTop {
		t.Errorf("opponent's library = %v, want [%v]", got, oppTop)
	}
}

// TestScryEffectArrangesTopAndBottomInOrder proves the full CR 701.19
// arrangement against a five-card library: the top three are looked at,
// two go back on top in a chosen order and one goes to the bottom, while
// the two untouched cards underneath stay exactly where they were.
func TestScryEffectArrangesTopAndBottomInOrder(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	a := g.NewCard(nil, p, engine.Library)
	b := g.NewCard(nil, p, engine.Library)
	cc := g.NewCard(nil, p, engine.Library)
	d := g.NewCard(nil, p, engine.Library)
	e := g.NewCard(nil, p, engine.Library)

	ctrl := engine.NewScriptedController()
	ctrl.QueueScry([]engine.CardID{b, a}, []engine.CardID{cc})

	if err := castETBScry(t, g, p, etbScryTriggerDefParams(t, "Test Arrange", "ScryNum$ 3", nil), ctrl); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	want := []engine.CardID{b, a, d, e, cc}
	if got := g.Zone(engine.Library, p).Cards(); !slices.Equal(got, want) {
		t.Errorf("library = %v, want %v", got, want)
	}
}

// TestScryEffectAllCardsToBottom proves the all-bottom half on its own: the
// scried cards all leave, in the given order, and the untouched cards below
// become the new top.
func TestScryEffectAllCardsToBottom(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	a := g.NewCard(nil, p, engine.Library)
	b := g.NewCard(nil, p, engine.Library)
	cc := g.NewCard(nil, p, engine.Library)
	d := g.NewCard(nil, p, engine.Library)

	ctrl := engine.NewScriptedController()
	ctrl.QueueScry(nil, []engine.CardID{a, b, cc})

	if err := castETBScry(t, g, p, etbScryTriggerDefParams(t, "Test All Bottom", "ScryNum$ 3", nil), ctrl); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	want := []engine.CardID{d, a, b, cc}
	if got := g.Zone(engine.Library, p).Cards(); !slices.Equal(got, want) {
		t.Errorf("library = %v, want %v", got, want)
	}
}

// TestScryEffectDefaultScryNumIsOne proves ScryNum$'s own Java default --
// rather than treating an absent ScryNum$ as zero cards looked at.
func TestScryEffectDefaultScryNumIsOne(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	top := g.NewCard(nil, p, engine.Library)
	under := g.NewCard(nil, p, engine.Library)

	c := engine.NewScriptedController()
	c.QueueScry(nil, []engine.CardID{top})

	if err := castETBScry(t, g, p, etbScryTriggerDefParams(t, "Test Default ScryNum", "", nil), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	want := []engine.CardID{under, top}
	if got := g.Zone(engine.Library, p).Cards(); !slices.Equal(got, want) {
		t.Errorf("library = %v, want %v -- ScryNum$'s own default is 1, not 0", got, want)
	}
}

// TestScryEffectResolvesNamedScryNumSVar proves ScryNum$ resolves a named
// SVar reference through resolveNamedAmount (amount.go), not just a plain
// integer: with a three-card library and ScryNum$ X (X:2), only the top two
// are ever offered to the controller, so sending both to the bottom must
// leave the untouched third card as the new top -- a wrong resolution (0,
// 1 or 3) would produce a different final order.
func TestScryEffectResolvesNamedScryNumSVar(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	a := g.NewCard(nil, p, engine.Library)
	b := g.NewCard(nil, p, engine.Library)
	cc := g.NewCard(nil, p, engine.Library)

	c := engine.NewScriptedController()
	c.QueueScry(nil, []engine.CardID{a, b})

	def := etbScryTriggerDefParams(t, "Test Named ScryNum", "ScryNum$ X", map[string]string{"X": "2"})
	if err := castETBScry(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	want := []engine.CardID{cc, a, b}
	if got := g.Zone(engine.Library, p).Cards(); !slices.Equal(got, want) {
		t.Errorf("library = %v, want %v -- ScryNum$ X must resolve through the named SVar X:2", got, want)
	}
}

// TestScryEffectNoOpsWhenScryNumIsZero proves CR 701.22b: a player
// instructed to scry 0 does not scry at all, so the controller is never
// asked (an unconsumed queue slot proves it).
func TestScryEffectNoOpsWhenScryNumIsZero(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	top := g.NewCard(nil, p, engine.Library)

	c := engine.NewScriptedController()
	// No QueueScry call: a real scry-0 must never reach the controller, or
	// this panics on an exhausted queue.
	def := etbScryTriggerDefParams(t, "Test Scry Zero", "ScryNum$ X", map[string]string{"X": "0"})
	if err := castETBScry(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Zone(engine.Library, p).Cards(); len(got) != 1 || got[0] != top {
		t.Errorf("library = %v, want unchanged [%v]", got, top)
	}
}

// TestScryEffectClampsTopNToLibrarySize proves a library holding fewer
// cards than ScryNum$ looks at all of them rather than erroring or padding.
func TestScryEffectClampsTopNToLibrarySize(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	only := g.NewCard(nil, p, engine.Library)

	c := engine.NewScriptedController()
	c.QueueScry([]engine.CardID{only}, nil)

	if err := castETBScry(t, g, p, etbScryTriggerDefParams(t, "Test Clamp", "ScryNum$ 5", nil), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Zone(engine.Library, p).Cards(); len(got) != 1 || got[0] != only {
		t.Errorf("library = %v, want unchanged [%v]", got, only)
	}
}

// TestScryEffectSkipsControllerWhenLibraryEmpty proves an empty library
// skips the ArrangeForScry call entirely, rather than asking about zero
// cards.
func TestScryEffectSkipsControllerWhenLibraryEmpty(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	// No QueueScry call: an empty library must never reach the controller,
	// or this panics on an exhausted queue.
	if err := castETBScry(t, g, p, etbScryTriggerDefParams(t, "Test Empty Library", "ScryNum$ 3", nil), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
}

// TestScryEffectRejectsSubAbilityChain proves SubAbility$ (no chaining
// mechanism exists yet) is a real error rather than silently dropping the
// chained ability (PORT-8/GO-7).
func TestScryEffectRejectsSubAbilityChain(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := etbScryTriggerDefParams(t, "Test SubAbility",
		"ScryNum$ 1 | SubAbility$ DBCleanup", map[string]string{"DBCleanup": "DB$ Cleanup"})
	c := engine.NewScriptedController()
	err := castETBScry(t, g, p, def, c)
	if err == nil {
		t.Fatal("ResolveStack: got nil error, want one naming SubAbility")
	}
	if !strings.Contains(err.Error(), "SubAbility") {
		t.Errorf("ResolveStack error = %q, want it to name SubAbility$", err.Error())
	}
}

// TestScryEffectRejectsValidTgts proves scryEffect itself still rejects a
// real target: resolveTargets (targeting.go) now resolves ValidTgts$
// generically before this ability is even pushed, so the target is chosen
// without issue, but scryEffect has not been extended to consume Targeted
// (defined.go) yet -- its own blocked-param list still names ValidTgts$,
// and this proves that check still fires.
func TestScryEffectRejectsValidTgts(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := etbScryTriggerDefParams(t, "Test ValidTgts", "ScryNum$ 1 | ValidTgts$ Player", nil)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(p)})
	err := castETBScry(t, g, p, def, c)
	if err == nil {
		t.Fatal("ResolveStack: got nil error, want one naming ValidTgts")
	}
	if !strings.Contains(err.Error(), "ValidTgts") {
		t.Errorf("ResolveStack error = %q, want it to name ValidTgts$", err.Error())
	}
}

// TestScryEffectRejectsOptional proves Optional$ (an interactive confirm
// this port's own PlayerController has no hook for) fails loudly rather
// than silently treating the scry as mandatory.
func TestScryEffectRejectsOptional(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := etbScryTriggerDefParams(t, "Test Optional", "ScryNum$ 1 | Optional$ True", nil)
	c := engine.NewScriptedController()
	err := castETBScry(t, g, p, def, c)
	if err == nil {
		t.Fatal("ResolveStack: got nil error, want one naming Optional")
	}
	if !strings.Contains(err.Error(), "Optional") {
		t.Errorf("ResolveStack error = %q, want it to name Optional$", err.Error())
	}
}

// TestScryEffectFiresWhenConditionCheckSVarIsMet proves subAbilityConditionMet
// (condition.go) gates Scry's own resolution the identical way it gates
// PutCounter's/Discard's/Pump's/DealDamage's/GainLife's.
func TestScryEffectFiresWhenConditionCheckSVarIsMet(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	top := g.NewCard(nil, p, engine.Library)

	c := engine.NewScriptedController()
	c.QueueScry([]engine.CardID{top}, nil)

	def := etbScryTriggerDefParams(t, "Test Condition Met",
		"ScryNum$ 1 | ConditionCheckSVar$ X | ConditionSVarCompare$ GE1", map[string]string{"X": "1"})
	if err := castETBScry(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Zone(engine.Library, p).Cards(); len(got) != 1 || got[0] != top {
		t.Errorf("library = %v, want [%v] -- ConditionCheckSVar$ X | ConditionSVarCompare$ GE1 holds (X is 1)", got, top)
	}
}

// TestScryEffectNoOpsWhenConditionCheckSVarIsNotMet proves the negative
// control: X GE1 fails (X is 0), so the ability does nothing -- and, in
// particular, never reaches the controller at all.
func TestScryEffectNoOpsWhenConditionCheckSVarIsNotMet(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	top := g.NewCard(nil, p, engine.Library)

	c := engine.NewScriptedController()
	// No QueueScry call: the unmet condition must never reach the
	// controller, or this panics on an exhausted queue.
	def := etbScryTriggerDefParams(t, "Test Condition Unmet",
		"ScryNum$ 1 | ConditionCheckSVar$ X | ConditionSVarCompare$ GE1", map[string]string{"X": "0"})
	if err := castETBScry(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Zone(engine.Library, p).Cards(); len(got) != 1 || got[0] != top {
		t.Errorf("library = %v, want unchanged [%v]", got, top)
	}
}
