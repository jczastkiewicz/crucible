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

// etbSurveilTriggerDefParams builds a *compile.Card whose own "when
// CARDNAME enters" trigger runs DB$ Surveil with the given params string
// appended -- surveilEffect itself is unexported, so every case here is
// driven through the real cast+resolve pipeline rather than calling it
// directly (TEST-1), the identical reason etbScryTriggerDefParams
// (scryeffect_test.go) is.
func etbSurveilTriggerDefParams(t *testing.T, name, extraParams string, svars map[string]string) *compile.Card {
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
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigSurveil",
	}
	raw.Faces[0].SVars.Set("TrigSurveil", "DB$ Surveil | "+extraParams)
	for name, body := range svars {
		raw.Faces[0].SVars.Set(name, body)
	}

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// castETBSurveil casts def (built by etbSurveilTriggerDefParams) for p on
// controller c and resolves the stack, returning ResolveStack's own error.
// c is passed in, rather than built fresh, because a scripted surveil
// decision has to be queued on it before this runs.
func castETBSurveil(t *testing.T, g *engine.Game, p engine.PlayerID, def *compile.Card, c *engine.ScriptedController) error {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	return g.ResolveStack(engine.NewRegistry(), c)
}

// TestSurveilEffectDefaultDefinedIsYou proves the corpus's own single
// largest real shape: an absent Defined$ resolves to "You", the identical
// default scryEffect's own doc comment already covers.
func TestSurveilEffectDefaultDefinedIsYou(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	top := g.NewCard(nil, p, engine.Library)

	c := engine.NewScriptedController()
	c.QueueSurveil([]engine.CardID{top}, nil)

	if err := castETBSurveil(t, g, p, etbSurveilTriggerDefParams(t, "Test Default Defined", "Amount$ 1", nil), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Zone(engine.Library, p).Cards(); len(got) != 1 || got[0] != top {
		t.Errorf("library = %v, want [%v] -- an absent Defined$ must default to You", got, top)
	}
}

// TestSurveilEffectDefinedOpponentAffectsOpponentLibrary proves the
// player-shaped Defined$ dispatch reaches the opponent's own library, not
// the caster's.
func TestSurveilEffectDefinedOpponentAffectsOpponentLibrary(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, opp := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(opp).Life = 20, 20
	oppTop := g.NewCard(nil, opp, engine.Library)

	c := engine.NewScriptedController()
	c.QueueSurveil([]engine.CardID{oppTop}, nil)

	if err := castETBSurveil(t, g, p, etbSurveilTriggerDefParams(t, "Test Defined Opponent", "Amount$ 1 | Defined$ Opponent", nil), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Zone(engine.Library, opp).Cards(); len(got) != 1 || got[0] != oppTop {
		t.Errorf("opponent's library = %v, want [%v]", got, oppTop)
	}
}

// TestSurveilEffectArrangesTopAndGraveyard proves the full CR 701.42
// arrangement against a five-card library: the top three are looked at,
// two go back on top in a chosen order and one goes to the graveyard,
// while the two untouched cards underneath stay exactly where they were.
func TestSurveilEffectArrangesTopAndGraveyard(t *testing.T) {
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
	ctrl.QueueSurveil([]engine.CardID{b, a}, []engine.CardID{cc})

	if err := castETBSurveil(t, g, p, etbSurveilTriggerDefParams(t, "Test Arrange", "Amount$ 3", nil), ctrl); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	wantLib := []engine.CardID{b, a, d, e}
	if got := g.Zone(engine.Library, p).Cards(); !slices.Equal(got, wantLib) {
		t.Errorf("library = %v, want %v", got, wantLib)
	}
	if got := g.Zone(engine.Graveyard, p).Cards(); len(got) != 1 || got[0] != cc {
		t.Errorf("graveyard = %v, want [%v]", got, cc)
	}
}

// TestSurveilEffectAllCardsToGraveyard proves the all-graveyard half on its
// own: the surveiled cards all leave to the graveyard, and the untouched
// card below becomes the new top.
func TestSurveilEffectAllCardsToGraveyard(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	a := g.NewCard(nil, p, engine.Library)
	b := g.NewCard(nil, p, engine.Library)
	cc := g.NewCard(nil, p, engine.Library)

	ctrl := engine.NewScriptedController()
	ctrl.QueueSurveil(nil, []engine.CardID{a, b})

	if err := castETBSurveil(t, g, p, etbSurveilTriggerDefParams(t, "Test All Graveyard", "Amount$ 2", nil), ctrl); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Zone(engine.Library, p).Cards(); len(got) != 1 || got[0] != cc {
		t.Errorf("library = %v, want [%v]", got, cc)
	}
	wantGrave := []engine.CardID{a, b}
	if got := g.Zone(engine.Graveyard, p).Cards(); !slices.Equal(got, wantGrave) {
		t.Errorf("graveyard = %v, want %v", got, wantGrave)
	}
}

// TestSurveilEffectDefaultAmountIsOne proves Amount$'s own Java default --
// rather than treating an absent Amount$ as zero cards looked at.
func TestSurveilEffectDefaultAmountIsOne(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	top := g.NewCard(nil, p, engine.Library)
	under := g.NewCard(nil, p, engine.Library)

	c := engine.NewScriptedController()
	c.QueueSurveil(nil, []engine.CardID{top})

	if err := castETBSurveil(t, g, p, etbSurveilTriggerDefParams(t, "Test Default Amount", "", nil), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Zone(engine.Library, p).Cards(); len(got) != 1 || got[0] != under {
		t.Errorf("library = %v, want [%v] -- Amount$'s own default is 1, not 0", got, under)
	}
	if got := g.Zone(engine.Graveyard, p).Cards(); len(got) != 1 || got[0] != top {
		t.Errorf("graveyard = %v, want [%v]", got, top)
	}
}

// TestSurveilEffectResolvesNamedAmountSVar proves Amount$ resolves a named
// SVar reference through resolveNamedAmount (amount.go), not just a plain
// integer: with a three-card library and Amount$ X (X:2), only the top two
// are ever offered to the controller, so sending both to the graveyard must
// leave the untouched third card as the new top.
func TestSurveilEffectResolvesNamedAmountSVar(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	a := g.NewCard(nil, p, engine.Library)
	b := g.NewCard(nil, p, engine.Library)
	cc := g.NewCard(nil, p, engine.Library)

	c := engine.NewScriptedController()
	c.QueueSurveil(nil, []engine.CardID{a, b})

	def := etbSurveilTriggerDefParams(t, "Test Named Amount", "Amount$ X", map[string]string{"X": "2"})
	if err := castETBSurveil(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Zone(engine.Library, p).Cards(); len(got) != 1 || got[0] != cc {
		t.Errorf("library = %v, want [%v] -- Amount$ X must resolve through the named SVar X:2", got, cc)
	}
}

// TestSurveilEffectNoOpsWhenAmountIsZero proves a surveil for 0 does not
// surveil at all, so the controller is never asked (an unconsumed queue
// slot proves it).
func TestSurveilEffectNoOpsWhenAmountIsZero(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	top := g.NewCard(nil, p, engine.Library)

	c := engine.NewScriptedController()
	// No QueueSurveil call: a real surveil-0 must never reach the
	// controller, or this panics on an exhausted queue.
	def := etbSurveilTriggerDefParams(t, "Test Amount Zero", "Amount$ X", map[string]string{"X": "0"})
	if err := castETBSurveil(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Zone(engine.Library, p).Cards(); len(got) != 1 || got[0] != top {
		t.Errorf("library = %v, want unchanged [%v]", got, top)
	}
}

// TestSurveilEffectClampsTopNToLibrarySize proves a library holding fewer
// cards than Amount$ looks at all of them rather than erroring or padding.
func TestSurveilEffectClampsTopNToLibrarySize(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	only := g.NewCard(nil, p, engine.Library)

	c := engine.NewScriptedController()
	c.QueueSurveil([]engine.CardID{only}, nil)

	if err := castETBSurveil(t, g, p, etbSurveilTriggerDefParams(t, "Test Clamp", "Amount$ 5", nil), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Zone(engine.Library, p).Cards(); len(got) != 1 || got[0] != only {
		t.Errorf("library = %v, want unchanged [%v]", got, only)
	}
}

// TestSurveilEffectSkipsControllerWhenLibraryEmpty proves an empty library
// skips the ArrangeForSurveil call entirely, rather than asking about zero
// cards.
func TestSurveilEffectSkipsControllerWhenLibraryEmpty(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	// No QueueSurveil call: an empty library must never reach the
	// controller, or this panics on an exhausted queue.
	if err := castETBSurveil(t, g, p, etbSurveilTriggerDefParams(t, "Test Empty Library", "Amount$ 3", nil), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
}

// TestSurveilEffectRejectsSubAbilityChain proves SubAbility$ (no chaining
// mechanism exists yet) is a real error rather than silently dropping the
// chained ability (PORT-8/GO-7).
func TestSurveilEffectRejectsSubAbilityChain(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := etbSurveilTriggerDefParams(t, "Test SubAbility",
		"Amount$ 1 | SubAbility$ DBCleanup", map[string]string{"DBCleanup": "DB$ Cleanup"})
	c := engine.NewScriptedController()
	err := castETBSurveil(t, g, p, def, c)
	if err == nil {
		t.Fatal("ResolveStack: got nil error, want one naming SubAbility")
	}
	if !strings.Contains(err.Error(), "SubAbility") {
		t.Errorf("ResolveStack error = %q, want it to name SubAbility$", err.Error())
	}
}

// TestSurveilEffectRejectsValidTgts proves a real target (this port's own
// targeting gap) fails loudly.
func TestSurveilEffectRejectsValidTgts(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := etbSurveilTriggerDefParams(t, "Test ValidTgts", "Amount$ 1 | ValidTgts$ Player", nil)
	c := engine.NewScriptedController()
	err := castETBSurveil(t, g, p, def, c)
	if err == nil {
		t.Fatal("ResolveStack: got nil error, want one naming ValidTgts")
	}
	if !strings.Contains(err.Error(), "ValidTgts") {
		t.Errorf("ResolveStack error = %q, want it to name ValidTgts$", err.Error())
	}
}

// TestSurveilEffectFiresWhenConditionCheckSVarIsMet proves
// subAbilityConditionMet (condition.go) gates Surveil's own resolution the
// identical way it gates Scry's/Discard's/PutCounter's.
func TestSurveilEffectFiresWhenConditionCheckSVarIsMet(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	top := g.NewCard(nil, p, engine.Library)

	c := engine.NewScriptedController()
	c.QueueSurveil([]engine.CardID{top}, nil)

	def := etbSurveilTriggerDefParams(t, "Test Condition Met",
		"Amount$ 1 | ConditionCheckSVar$ X | ConditionSVarCompare$ GE1", map[string]string{"X": "1"})
	if err := castETBSurveil(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Zone(engine.Library, p).Cards(); len(got) != 1 || got[0] != top {
		t.Errorf("library = %v, want [%v] -- ConditionCheckSVar$ X | ConditionSVarCompare$ GE1 holds (X is 1)", got, top)
	}
}

// TestSurveilEffectNoOpsWhenConditionCheckSVarIsNotMet proves the negative
// control: X GE1 fails (X is 0), so the ability does nothing -- and, in
// particular, never reaches the controller at all.
func TestSurveilEffectNoOpsWhenConditionCheckSVarIsNotMet(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	top := g.NewCard(nil, p, engine.Library)

	c := engine.NewScriptedController()
	// No QueueSurveil call: the unmet condition must never reach the
	// controller, or this panics on an exhausted queue.
	def := etbSurveilTriggerDefParams(t, "Test Condition Unmet",
		"Amount$ 1 | ConditionCheckSVar$ X | ConditionSVarCompare$ GE1", map[string]string{"X": "0"})
	if err := castETBSurveil(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Zone(engine.Library, p).Cards(); len(got) != 1 || got[0] != top {
		t.Errorf("library = %v, want unchanged [%v]", got, top)
	}
}
