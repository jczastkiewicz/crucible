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

// etbLoseLifeTriggerDefParams builds a *compile.Card whose own "when
// CARDNAME enters" trigger runs DB$ LoseLife with the given params string
// appended -- loseLifeEffect itself is unexported, so every case here is
// driven through the real cast+resolve pipeline rather than calling it
// directly (TEST-1), the identical reason etbGainLifeTriggerDefParams
// (gainlifeeffect_test.go) is.
func etbLoseLifeTriggerDefParams(t *testing.T, name, extraParams string, svars map[string]string) *compile.Card {
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
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigLoseLife",
	}
	raw.Faces[0].SVars.Set("TrigLoseLife", "DB$ LoseLife | "+extraParams)
	for name, body := range svars {
		raw.Faces[0].SVars.Set(name, body)
	}

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// castETBLoseLife casts def (built by etbLoseLifeTriggerDefParams) for p and
// resolves the stack, returning ResolveStack's own error.
func castETBLoseLife(t *testing.T, g *engine.Game, p engine.PlayerID, def *compile.Card) error {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	c := engine.NewScriptedController()
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	return g.ResolveStack(engine.NewRegistry(), c)
}

// TestLoseLifeEffectDefinedYouDrainsController proves the corpus's single
// largest real Defined$ shape: Defined$ You drains life from the ability's
// own controller, not any opponent.
func TestLoseLifeEffectDefinedYouDrainsController(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20

	if err := castETBLoseLife(t, g, p, etbLoseLifeTriggerDefParams(t, "Test You", "Defined$ You | LifeAmount$ 3", nil)); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Player(p).Life != 17 {
		t.Errorf("p's life = %d, want 17", g.Player(p).Life)
	}
	if g.Player(other).Life != 20 {
		t.Errorf("other's life = %d, want 20 -- Defined$ You must not touch an opponent", g.Player(other).Life)
	}
}

// TestLoseLifeEffectDefinedOpponentDrainsEveryOpponent proves Defined$
// Opponent drains every opponent of the controller, not the controller
// itself -- a three-player game so "every" is observably more than one.
func TestLoseLifeEffectDefinedOpponentDrainsEveryOpponent(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b", "c")
	p, o1, o2 := g.Players()[0], g.Players()[1], g.Players()[2]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(o1).Life, g.Player(o2).Life = 20, 20, 20

	if err := castETBLoseLife(t, g, p, etbLoseLifeTriggerDefParams(t, "Test Opponent", "Defined$ Opponent | LifeAmount$ 2", nil)); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Player(o1).Life != 18 {
		t.Errorf("o1's life = %d, want 18", g.Player(o1).Life)
	}
	if g.Player(o2).Life != 18 {
		t.Errorf("o2's life = %d, want 18", g.Player(o2).Life)
	}
	if g.Player(p).Life != 20 {
		t.Errorf("p's life = %d, want 20 -- Defined$ Opponent must not touch the controller", g.Player(p).Life)
	}
}

// TestLoseLifeEffectResolvesNamedLifeAmountSVar proves LifeAmount$ resolves a
// named SVar reference through resolveNamedAmount (amount.go), not just a
// plain integer.
func TestLoseLifeEffectResolvesNamedLifeAmountSVar(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20

	def := etbLoseLifeTriggerDefParams(t, "Test Named LifeAmount", "Defined$ You | LifeAmount$ X", map[string]string{"X": "5"})
	if err := castETBLoseLife(t, g, p, def); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Player(p).Life != 15 {
		t.Errorf("p's life = %d, want 15 -- LifeAmount$ X must resolve through the named SVar X:5", g.Player(p).Life)
	}
}

// TestLoseLifeEffectMissingLifeAmountErrors proves a missing LifeAmount$ is
// a real error rather than a silent zero-loss no-op (GO-7).
func TestLoseLifeEffectMissingLifeAmountErrors(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20

	err := castETBLoseLife(t, g, p, etbLoseLifeTriggerDefParams(t, "Test No LifeAmount", "Defined$ You", nil))
	if err == nil {
		t.Fatal("ResolveStack: got nil error, want one naming LifeAmount$")
	}
	if !strings.Contains(err.Error(), "LifeAmount") {
		t.Errorf("ResolveStack error = %q, want it to name LifeAmount$", err.Error())
	}
}

// TestLoseLifeEffectRejectsSubAbilityChain proves SubAbility$ (no chaining
// mechanism exists yet) is a real error rather than silently dropping the
// chained ability (PORT-8/GO-7).
func TestLoseLifeEffectRejectsSubAbilityChain(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20

	def := etbLoseLifeTriggerDefParams(t, "Test SubAbility",
		"Defined$ You | LifeAmount$ 1 | SubAbility$ DBCleanup", map[string]string{"DBCleanup": "DB$ Cleanup"})
	err := castETBLoseLife(t, g, p, def)
	if err == nil {
		t.Fatal("ResolveStack: got nil error, want one naming SubAbility")
	}
	if !strings.Contains(err.Error(), "SubAbility") {
		t.Errorf("ResolveStack error = %q, want it to name SubAbility$", err.Error())
	}
	if g.Player(p).Life != 20 {
		t.Errorf("p's life = %d, want 20 -- a rejected line must not drain partial life", g.Player(p).Life)
	}
}

// TestLoseLifeEffectFiresWhenConditionCheckSVarIsMet proves
// subAbilityConditionMet (condition.go) gates LoseLife's own resolution the
// identical way it gates GainLife's/DealDamage's and a checkland's DB$ Tap.
func TestLoseLifeEffectFiresWhenConditionCheckSVarIsMet(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20

	def := etbLoseLifeTriggerDefParams(t, "Test Condition Met",
		"Defined$ You | LifeAmount$ 3 | ConditionCheckSVar$ X | ConditionSVarCompare$ GE1", map[string]string{"X": "1"})
	if err := castETBLoseLife(t, g, p, def); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Player(p).Life != 17 {
		t.Errorf("p's life = %d, want 17 -- ConditionCheckSVar$ X | ConditionSVarCompare$ GE1 holds (X is 1)", g.Player(p).Life)
	}
}

// TestLoseLifeEffectNoOpsWhenConditionCheckSVarIsNotMet proves the negative
// control: X GE1 fails (X is 0), so the ability does nothing -- not an
// error, SpellAbilityCondition.areMet's own "declined by the rules" contract.
func TestLoseLifeEffectNoOpsWhenConditionCheckSVarIsNotMet(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20

	def := etbLoseLifeTriggerDefParams(t, "Test Condition Unmet",
		"Defined$ You | LifeAmount$ 3 | ConditionCheckSVar$ X | ConditionSVarCompare$ GE1", map[string]string{"X": "0"})
	if err := castETBLoseLife(t, g, p, def); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Player(p).Life != 20 {
		t.Errorf("p's life = %d, want 20 -- ConditionCheckSVar$ X | ConditionSVarCompare$ GE1 fails (X is 0), no life lost", g.Player(p).Life)
	}
}

// TestLoseLifeEffectRejectsConditionItself proves Condition$ (the flag
// switch, distinct from the resolved ConditionCheckSVar$/ConditionPresent$
// pair) still fails loudly -- SpellAbilityCondition's own Threshold/
// Metalcraft/... family, no evaluator built for it.
func TestLoseLifeEffectRejectsConditionItself(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20

	def := etbLoseLifeTriggerDefParams(t, "Test Condition Flag", "Defined$ You | LifeAmount$ 1 | Condition$ Threshold", nil)
	err := castETBLoseLife(t, g, p, def)
	if err == nil {
		t.Fatal("ResolveStack: got nil error, want one naming Condition")
	}
	if !strings.Contains(err.Error(), "Condition") {
		t.Errorf("ResolveStack error = %q, want it to name Condition$", err.Error())
	}
	if g.Player(p).Life != 20 {
		t.Errorf("p's life = %d, want 20 -- a rejected line must not drain partial life", g.Player(p).Life)
	}
}

// TestLoseLifeEffectEmitsLifeChangedEvent proves loseLifeEffect emits the
// identical LifeChanged event dealPlayerDamage/gainLifeEffect already emit,
// here with a negative amount for a loss.
func TestLoseLifeEffectEmitsLifeChangedEvent(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	var sink recordingSink
	g.SetSink(&sink)

	if err := castETBLoseLife(t, g, p, etbLoseLifeTriggerDefParams(t, "Test Event", "Defined$ You | LifeAmount$ 3", nil)); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	var saw bool
	for _, e := range sink.events {
		if e.Kind == engine.LifeChanged {
			saw = true
			if e.Amount != -3 {
				t.Errorf("LifeChanged Amount = %d, want -3", e.Amount)
			}
		}
	}
	if !saw {
		t.Error("no LifeChanged event seen")
	}
}
