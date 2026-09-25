package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// TestReplaceSplitDamageRedirectsTheNextOneDamage proves eladamri.txt's
// shape: an effect card remembering a creature redirects the next 1 damage
// dealt to it to its controller, then -- its one point spent -- exiles
// itself (ReplaceSplitDamageEffect's isImmutable branch).
func TestReplaceSplitDamageRedirectsTheNextOneDamage(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	host := resolveLine(t, g, p, engine.NewScriptedController(),
		"DB$ Effect | RememberObjects$ Self | ReplacementEffects$ RedirectDamage | ExileOnMoved$ Battlefield | Duration$ Permanent | SubAbility$ DBDamage",
		"RedirectDamage", "Event$ DamageDone | ValidTarget$ Creature.IsRemembered | ReplaceWith$ RedirectDmg | DamageTarget$ You | Description$ The next 1 damage is dealt to you instead.",
		"RedirectDmg", "DB$ ReplaceSplitDamage | DamageTarget$ You",
		"DBDamage", "DB$ DealDamage | Defined$ Self | NumDmg$ 3")
	if got := g.Player(p).Life; got != 19 {
		t.Errorf("controller life = %d, want 19 -- 1 of the 3 damage redirected", got)
	}
	if z := g.Card(host).Zone; z != engine.Graveyard {
		t.Errorf("1/1 creature zone = %v, want Graveyard -- the other 2 damage kill it", z)
	}
	for _, id := range g.Zone(engine.None, p).Cards() {
		if g.Card(id).IsEffect {
			return
		}
	}
	t.Error("no removed effect card, want the spent redirection gone")
}

// TestReplaceDamageShieldDepletes proves ReplaceDamage's Number$ shield on
// an effect card: 3 points of prevention absorb a 2-damage hit whole, then
// the first 1 of the next, and the spent effect is exiled.
func TestReplaceDamageShieldDepletes(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	resolveLine(t, g, p, engine.NewScriptedController(),
		"DB$ Effect | ReplacementEffects$ Shield | Duration$ Permanent | SubAbility$ DBDamage",
		"Shield", "Event$ DamageDone | ValidTarget$ You | ReplaceWith$ DBShield | PreventionEffect$ True | Description$ Prevent the next 3 damage.",
		"DBShield", "DB$ ReplaceDamage | Amount$ ShieldAmount",
		"ShieldAmount", "Number$3",
		"DBDamage", "DB$ DealDamage | Defined$ You | NumDmg$ 2 | SubAbility$ DBDamage2",
		"DBDamage2", "DB$ DealDamage | Defined$ You | NumDmg$ 2")
	if got := g.Player(p).Life; got != 19 {
		t.Errorf("life = %d, want 19 -- 3 of the 4 damage prevented", got)
	}
	if n := len(commandEffects(g, p)); n != 0 {
		t.Errorf("command zone holds %d, want the spent shield exiled", n)
	}
}

// TestReplaceTokenDoublesTokens proves doubling_season.txt's token half:
// Event$ CreateToken with ValidToken$ Card.YouCtrl, ReplaceWith$ naming
// ReplaceToken | Type$ Amount (default Twice). An opponent's tokens are not
// doubled.
func TestReplaceTokenDoublesTokens(t *testing.T) {
	t.Parallel()

	g, p, other := newTokenGame(t)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Doubling Season",
		"Event$ CreateToken | ActiveZones$ Battlefield | ValidToken$ Card.YouCtrl | ReplaceWith$ DoubleToken | Description$ Twice that many tokens.",
		"DoubleToken", "DB$ ReplaceToken | Type$ Amount"), p, engine.Battlefield)
	resolveLine(t, g, p, engine.NewScriptedController(),
		"DB$ Token | TokenScript$ w_1_1_soldier | TokenAmount$ 2 | SubAbility$ DBTheirs",
		"DBTheirs", "DB$ Token | TokenOwner$ Opponent | TokenScript$ w_1_1_soldier")
	if n := len(tokensOn(g, p, "Soldier Token")); n != 4 {
		t.Errorf("p's soldiers = %d, want 4", n)
	}
	if n := len(tokensOn(g, other, "Soldier Token")); n != 1 {
		t.Errorf("opponent's soldiers = %d, want 1", n)
	}
}

// TestReplaceCounterAddsOneMore proves hardened_scales.txt: +1/+1 counters
// put on a creature you control become that many plus one; another counter
// kind is untouched.
func TestReplaceCounterAddsOneMore(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(replacementEnchantmentDefWithSVars(t, "Test Hardened Scales",
		"Event$ AddCounter | ActiveZones$ Battlefield | ValidCard$ Creature.YouCtrl+inZoneBattlefield | ValidCounterType$ P1P1 | ReplaceWith$ AddOneMoreCounters | Description$ One more.",
		map[string]string{
			"AddOneMoreCounters": "DB$ ReplaceCounter | ValidCounterType$ P1P1 | ChooseCounter$ True | Amount$ X",
			"X":                  "ReplaceCount$CounterNum/Plus.1",
		}), p, engine.Battlefield)
	host := resolveLine(t, g, p, engine.NewScriptedController(),
		"DB$ PutCounter | Defined$ Self | CounterType$ P1P1 | CounterNum$ 2 | SubAbility$ DBStun",
		"DBStun", "DB$ PutCounter | Defined$ Self | CounterType$ STUN | CounterNum$ 2")
	if got := g.Card(host).Counters.Count(engine.P1P1); got != 3 {
		t.Errorf("+1/+1 counters = %d, want 3", got)
	}
	if got := g.Card(host).Counters.Count(engine.CounterType("STUN")); got != 2 {
		t.Errorf("stun counters = %d, want 2", got)
	}
}

// TestReplaceManaChangesTypeAndAmount proves infernal_darkness.txt's
// ReplaceType$ (a land's mana becomes black) and mana_reflection.txt's
// ReplaceAmount$ (twice as much), each through Event$ ProduceMana.
func TestReplaceManaChangesTypeAndAmount(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Infernal Darkness",
		"Event$ ProduceMana | ActiveZones$ Battlefield | ValidCard$ Land | ReplaceWith$ ProduceB | Description$ Black instead.",
		"ProduceB", "DB$ ReplaceMana | ReplaceType$ B"), p, engine.Battlefield)
	forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
	if !g.TapLandForMana(p, forest, mana.Green, engine.NewScriptedController()) {
		t.Fatal("TapLandForMana failed")
	}
	if got := g.Player(p).ManaPool.Breakdown(); got != [6]int{0, 0, 1, 0, 0, 0} {
		t.Errorf("pool = %v, want one black", got)
	}

	g2 := newGame(t, "a", "b")
	q := g2.Players()[0]
	g2.NewCard(replacementEnchantmentDefWithSVar(t, "Test Mana Reflection",
		"Event$ ProduceMana | ActiveZones$ Battlefield | ValidActivator$ You | ValidCard$ Permanent | ReplaceWith$ ProduceTwice | Description$ Twice as much.",
		"ProduceTwice", "DB$ ReplaceMana | ReplaceAmount$ 2"), q, engine.Battlefield)
	plains := g2.NewCard(landDef(t, "Plains", "Basic Land Plains"), q, engine.Battlefield)
	if !g2.TapLandForMana(q, plains, mana.White, engine.NewScriptedController()) {
		t.Fatal("TapLandForMana failed")
	}
	if got := g2.Player(q).ManaPool.Breakdown(); got != [6]int{2, 0, 0, 0, 0, 0} {
		t.Errorf("pool = %v, want two white", got)
	}
}

// TestReplaceEffectUnportedVarTypeLeavesDamage proves a ReplaceWith$ the
// effect rejects is skipped whole: VarType$ Card swaps the event's object,
// which this port does not model, so the damage happens unchanged.
func TestReplaceEffectUnportedVarTypeLeavesDamage(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	resolveLine(t, g, p, engine.NewScriptedController(),
		"DB$ Effect | ReplacementEffects$ Swap | SubAbility$ DBDamage",
		"Swap", "Event$ DamageDone | ValidTarget$ You | ReplaceWith$ DBSwap | Description$ Swap.",
		"DBSwap", "DB$ ReplaceEffect | VarName$ Affected | VarValue$ Self | VarType$ Card",
		"DBDamage", "DB$ DealDamage | Defined$ You | NumDmg$ 2")
	if got := g.Player(p).Life; got != 18 {
		t.Errorf("life = %d, want 18 -- the unported replacement skipped", got)
	}
}

// TestReplaceTokenAmountOperators proves doXMath's other operators past the
// default Twice: Thrice, HalfUp, HalfDown and Plus/Minus.N.
func TestReplaceTokenAmountOperators(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		op   string
		want int
	}{
		{"Thrice", 9},
		{"HalfUp", 2},
		{"HalfDown", 1},
		{"Plus.2", 5},
		{"Minus.2", 1},
	} {
		t.Run(tc.op, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTokenGame(t)
			g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Amount Op",
				"Event$ CreateToken | ActiveZones$ Battlefield | ValidToken$ Card.YouCtrl | ReplaceWith$ Op | Description$ Op.",
				"Op", "DB$ ReplaceToken | Type$ Amount | Amount$ "+tc.op), p, engine.Battlefield)
			resolveLine(t, g, p, engine.NewScriptedController(),
				"DB$ Token | TokenScript$ w_1_1_soldier | TokenAmount$ 3")
			if n := len(tokensOn(g, p, "Soldier Token")); n != tc.want {
				t.Errorf("soldiers = %d, want %d", n, tc.want)
			}
		})
	}
}

// TestReplaceManaSingleSymbolReplacesType proves ReplaceMana$ with a plain
// color letter (not Any) swaps the produced mana's type outright.
func TestReplaceManaSingleSymbolReplacesType(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Replace Mana Symbol",
		"Event$ ProduceMana | ActiveZones$ Battlefield | ValidCard$ Land | ReplaceWith$ ProduceR | Description$ Red instead.",
		"ProduceR", "DB$ ReplaceMana | ReplaceMana$ R"), p, engine.Battlefield)
	forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
	if !g.TapLandForMana(p, forest, mana.Green, engine.NewScriptedController()) {
		t.Fatal("TapLandForMana failed")
	}
	if got := g.Player(p).ManaPool.Breakdown(); got != [6]int{0, 0, 0, 1, 0, 0} {
		t.Errorf("pool = %v, want one red", got)
	}
}

// TestReplaceManaAnyAsksControllerToChoose proves ReplaceMana$ Any defers to
// ChooseManaColor rather than picking a fixed color.
func TestReplaceManaAnyAsksControllerToChoose(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Replace Mana Any",
		"Event$ ProduceMana | ActiveZones$ Battlefield | ValidCard$ Land | ReplaceWith$ ProduceAny | Description$ Any color instead.",
		"ProduceAny", "DB$ ReplaceMana | ReplaceMana$ Any"), p, engine.Battlefield)
	forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueManaColor(mana.Blue)
	if !g.TapLandForMana(p, forest, mana.Green, c) {
		t.Fatal("TapLandForMana failed")
	}
	if got := g.Player(p).ManaPool.Breakdown(); got != [6]int{0, 1, 0, 0, 0, 0} {
		t.Errorf("pool = %v, want one blue", got)
	}
}

// TestReplaceManaColorRecolorsOnlyMatchingUnits proves ReplaceColor$ (a
// color word, or Chosen) recolors the produced mana, but ReplaceOnly$ skips
// units of a different color and a colorless unit is never recolored.
func TestReplaceManaColorRecolorsOnlyMatchingUnits(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Replace Color",
		"Event$ ProduceMana | ActiveZones$ Battlefield | ValidCard$ Land | ReplaceWith$ Recolor | Description$ Recolor.",
		"Recolor", "DB$ ReplaceMana | ReplaceColor$ Blue | ReplaceOnly$ Green"), p, engine.Battlefield)
	forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
	if !g.TapLandForMana(p, forest, mana.Green, engine.NewScriptedController()) {
		t.Fatal("TapLandForMana failed")
	}
	if got := g.Player(p).ManaPool.Breakdown(); got != [6]int{0, 1, 0, 0, 0, 0} {
		t.Errorf("pool = %v, want one blue (Green recolored via ReplaceOnly$ Green)", got)
	}

	g2 := newGame(t, "a", "b")
	q := g2.Players()[0]
	g2.NewCard(replacementEnchantmentDefWithSVar(t, "Test Replace Color Only",
		"Event$ ProduceMana | ActiveZones$ Battlefield | ValidCard$ Land | ReplaceWith$ Recolor | Description$ Recolor.",
		"Recolor", "DB$ ReplaceMana | ReplaceColor$ Blue | ReplaceOnly$ Red"), q, engine.Battlefield)
	plains := g2.NewCard(landDef(t, "Plains", "Basic Land Plains"), q, engine.Battlefield)
	if !g2.TapLandForMana(q, plains, mana.White, engine.NewScriptedController()) {
		t.Fatal("TapLandForMana failed")
	}
	if got := g2.Player(q).ManaPool.Breakdown(); got != [6]int{1, 0, 0, 0, 0, 0} {
		t.Errorf("pool = %v, want the white untouched (ReplaceOnly$ Red does not name it)", got)
	}
}

// TestReplaceCounterConditionGateBlocksReplacement proves
// subAbilityConditionMet gates ReplaceCounter's own resolution the same way
// it gates PutCounter's: an unmet ConditionCheckSVar$/ConditionSVarCompare$
// leaves the counters untouched rather than silently always applying.
func TestReplaceCounterConditionGateBlocksReplacement(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(replacementEnchantmentDefWithSVars(t, "Test Condition Gated Scales",
		"Event$ AddCounter | ActiveZones$ Battlefield | ValidCard$ Creature.YouCtrl | ValidCounterType$ P1P1 | ReplaceWith$ AddOneMore | Description$ One more, when armed.",
		map[string]string{
			"AddOneMore": "DB$ ReplaceCounter | ValidCounterType$ P1P1 | ChooseCounter$ True | Amount$ X | ConditionCheckSVar$ Armed | ConditionSVarCompare$ GE1",
			"X":          "ReplaceCount$CounterNum/Plus.1",
			"Armed":      "0",
		}), p, engine.Battlefield)
	host := resolveLine(t, g, p, engine.NewScriptedController(),
		"DB$ PutCounter | Defined$ Self | CounterType$ P1P1 | CounterNum$ 2")
	if got := g.Card(host).Counters.Count(engine.P1P1); got != 2 {
		t.Errorf("+1/+1 counters = %d, want 2 -- ConditionCheckSVar$ Armed GE1 fails (Armed is 0)", got)
	}
}

// TestReplaceCounterValidCounterTypeMismatchStillLetsAMatchingReplacementRun
// proves a ValidCounterType$ mismatch reports "not replaced" rather than
// "applied" -- countersReplaced's eachReplacement must keep offering the
// event to a second, later replacement that does match the counter kind.
func TestReplaceCounterValidCounterTypeMismatchStillLetsAMatchingReplacementRun(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Wrong Type First",
		"Event$ AddCounter | ActiveZones$ Battlefield | ValidCard$ Creature.YouCtrl | ReplaceWith$ Untouched | Description$ Never matches P1P1.",
		"Untouched", "DB$ ReplaceCounter | ValidCounterType$ STUN | Amount$ 99"), p, engine.Battlefield)
	g.NewCard(replacementEnchantmentDefWithSVars(t, "Test Right Type Second",
		"Event$ AddCounter | ActiveZones$ Battlefield | ValidCard$ Creature.YouCtrl | ValidCounterType$ P1P1 | ReplaceWith$ DoubleIt | Description$ Doubles P1P1.",
		map[string]string{
			"DoubleIt": "DB$ ReplaceCounter | ValidCounterType$ P1P1 | Amount$ X",
			"X":        "ReplaceCount$CounterNum/Twice",
		}), p, engine.Battlefield)
	host := resolveLine(t, g, p, engine.NewScriptedController(),
		"DB$ PutCounter | Defined$ Self | CounterType$ P1P1 | CounterNum$ 2")
	if got := g.Card(host).Counters.Count(engine.P1P1); got != 4 {
		t.Errorf("+1/+1 counters = %d, want 4 -- the STUN-only replacement must not block the P1P1 one behind it", got)
	}
}

// TestReplaceSplitDamageRejectsDivideShield proves DivideShield$ (a shape
// this port does not model) makes the replacement fail to resolve, so
// eachReplacement treats it as not applied and the damage lands whole --
// rather than silently redirecting as if DivideShield$ were not there.
func TestReplaceSplitDamageRejectsDivideShield(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	host := resolveLine(t, g, p, engine.NewScriptedController(),
		"DB$ Effect | RememberObjects$ Self | ReplacementEffects$ RedirectDamage | Duration$ Permanent | SubAbility$ DBDamage",
		"RedirectDamage", "Event$ DamageDone | ValidTarget$ Creature.IsRemembered | ReplaceWith$ RedirectDmg | DamageTarget$ You | Description$ The next 1 damage is dealt to you instead.",
		"RedirectDmg", "DB$ ReplaceSplitDamage | DamageTarget$ You | DivideShield$ True",
		"DBDamage", "DB$ DealDamage | Defined$ Self | NumDmg$ 3")
	if got := g.Player(p).Life; got != 20 {
		t.Errorf("controller life = %d, want 20 -- DivideShield$ True makes the replacement fail to resolve, none redirected", got)
	}
	if z := g.Card(host).Zone; z != engine.Graveyard {
		t.Errorf("1/1 creature zone = %v, want Graveyard -- all 3 damage hits it, unredirected", z)
	}
}

// TestReplaceEffectFailsClosedOnMalformedLines proves ReplaceEffect's own
// error branches leave the event unedited rather than propagating: a
// rejected VarKey$, an unmet Condition gate, a VarName$ that does not name
// the event being replaced, and a missing VarValue$ each make the
// replacement fail to resolve, so eachReplacement treats it as not applied
// and the damage lands whole.
func TestReplaceEffectFailsClosedOnMalformedLines(t *testing.T) {
	t.Parallel()

	for name, replaceWith := range map[string]string{
		"VarKeyRejected":    "DB$ ReplaceEffect | VarKey$ Bogus | VarName$ DamageAmount | VarValue$ 0",
		"ConditionUnmet":    "DB$ ReplaceEffect | VarName$ DamageAmount | VarValue$ 0 | ConditionCheckSVar$ Armed | ConditionSVarCompare$ GE1",
		"VarNameMismatched": "DB$ ReplaceEffect | VarName$ Bogus | VarValue$ 0",
		"NoVarValue":        "DB$ ReplaceEffect | VarName$ DamageAmount",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			g.NewCard(replacementEnchantmentDefWithSVars(t, "Test "+name,
				"Event$ DamageDone | ValidTarget$ You | ReplaceWith$ Swap | Description$ Swap.",
				map[string]string{"Swap": replaceWith, "Armed": "0"}), p, engine.Battlefield)
			resolveLine(t, g, p, engine.NewScriptedController(), "DB$ DealDamage | Defined$ You | NumDmg$ 2")
			if got := g.Player(p).Life; got != 18 {
				t.Errorf("life = %d, want 18 -- the malformed replacement must fail closed, unapplied", got)
			}
		})
	}
}

// TestReplaceDamageFailsClosedAndDefaultsAmountToOne proves an unmet
// Condition gate leaves the damage unedited, a mismatched event (this API
// wired to something other than DamageDone) fails to resolve rather than
// misreading a different event's amount, and Amount$'s own default of 1
// applies when the param is absent.
func TestReplaceDamageFailsClosedAndDefaultsAmountToOne(t *testing.T) {
	t.Parallel()

	t.Run("ConditionUnmet", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGame(t)
		g.NewCard(replacementEnchantmentDefWithSVars(t, "Test Condition Unmet Shield",
			"Event$ DamageDone | ValidTarget$ You | ReplaceWith$ Shield | Description$ Shield.",
			map[string]string{"Shield": "DB$ ReplaceDamage | Amount$ 1 | ConditionCheckSVar$ Armed | ConditionSVarCompare$ GE1", "Armed": "0"}), p, engine.Battlefield)
		resolveLine(t, g, p, engine.NewScriptedController(), "DB$ DealDamage | Defined$ You | NumDmg$ 2")
		if got := g.Player(p).Life; got != 18 {
			t.Errorf("life = %d, want 18 -- unmet condition, no prevention", got)
		}
	})

	t.Run("WrongEventWiring", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGame(t)
		g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Wrong Event",
			"Event$ AddCounter | ActiveZones$ Battlefield | ValidCard$ Creature.YouCtrl | ValidCounterType$ P1P1 | ReplaceWith$ WrongAPI | Description$ Wrong API for this event.",
			"WrongAPI", "DB$ ReplaceDamage | Amount$ 1"), p, engine.Battlefield)
		host := resolveLine(t, g, p, engine.NewScriptedController(),
			"DB$ PutCounter | Defined$ Self | CounterType$ P1P1 | CounterNum$ 2")
		if got := g.Card(host).Counters.Count(engine.P1P1); got != 2 {
			t.Errorf("+1/+1 counters = %d, want 2 -- ReplaceDamage wired to AddCounter must fail closed, not misread CounterNum as damage", got)
		}
	})

	t.Run("AmountDefaultsToOne", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGame(t)
		g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Amount Defaults",
			"Event$ DamageDone | ValidTarget$ You | ReplaceWith$ Shield | Description$ Prevent the next 1 damage.",
			"Shield", "DB$ ReplaceDamage"), p, engine.Battlefield)
		resolveLine(t, g, p, engine.NewScriptedController(), "DB$ DealDamage | Defined$ You | NumDmg$ 2")
		if got := g.Player(p).Life; got != 19 {
			t.Errorf("life = %d, want 19 -- Amount$ absent defaults to 1", got)
		}
	})

	t.Run("AmountUnresolvable", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGame(t)
		g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Amount Unresolvable",
			"Event$ DamageDone | ValidTarget$ You | ReplaceWith$ Shield | Description$ Shield.",
			"Shield", "DB$ ReplaceDamage | Amount$ Bogus"), p, engine.Battlefield)
		resolveLine(t, g, p, engine.NewScriptedController(), "DB$ DealDamage | Defined$ You | NumDmg$ 2")
		if got := g.Player(p).Life; got != 18 {
			t.Errorf("life = %d, want 18 -- Amount$ Bogus is not resolvable, replacement fails closed", got)
		}
	})
}

// TestReplaceSplitDamageDefaultsAndFailsClosed proves VarName$'s own
// default of 1, a Condition gate, and DamageTarget$'s two failure shapes
// (absent, unresolvable) each leave the damage whole rather than redirected.
func TestReplaceSplitDamageDefaultsAndFailsClosed(t *testing.T) {
	t.Parallel()

	for name, replaceWith := range map[string]string{
		"ConditionUnmet":      "DB$ ReplaceSplitDamage | DamageTarget$ You | ConditionCheckSVar$ Armed | ConditionSVarCompare$ GE1",
		"NoDamageTarget":      "DB$ ReplaceSplitDamage",
		"DamageTargetBogus":   "DB$ ReplaceSplitDamage | DamageTarget$ BogusSpec",
		"VarNameUnresolvable": "DB$ ReplaceSplitDamage | DamageTarget$ You | VarName$ Bogus",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			host := resolveLine(t, g, p, engine.NewScriptedController(),
				"DB$ Effect | RememberObjects$ Self | ReplacementEffects$ RedirectDamage | Duration$ Permanent | SubAbility$ DBDamage",
				"RedirectDamage", "Event$ DamageDone | ValidTarget$ Creature.IsRemembered | ReplaceWith$ RedirectDmg | Description$ Redirect.",
				"RedirectDmg", replaceWith,
				"Armed", "0",
				"DBDamage", "DB$ DealDamage | Defined$ Self | NumDmg$ 3")
			if got := g.Player(p).Life; got != 20 {
				t.Errorf("controller life = %d, want 20 -- the malformed/gated replacement must not redirect", got)
			}
			if z := g.Card(host).Zone; z != engine.Graveyard {
				t.Errorf("1/1 creature zone = %v, want Graveyard -- all 3 damage hits it, unredirected", z)
			}
		})
	}
}

// TestReplaceTokenFailsClosedOnUnportedTypeAndBadAmount proves Type$ naming
// something other than Amount, and Amount$ naming an unresolvable op, both
// fail the replacement rather than silently leaving the token count alone
// or crashing.
func TestReplaceTokenFailsClosedOnUnportedTypeAndBadAmount(t *testing.T) {
	t.Parallel()

	t.Run("TypeNotAmount", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTokenGame(t)
		g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Type Not Amount",
			"Event$ CreateToken | ActiveZones$ Battlefield | ValidToken$ Card.YouCtrl | ReplaceWith$ Op | Description$ Op.",
			"Op", "DB$ ReplaceToken | Type$ ReplaceController"), p, engine.Battlefield)
		resolveLine(t, g, p, engine.NewScriptedController(), "DB$ Token | TokenScript$ w_1_1_soldier | TokenAmount$ 3")
		if n := len(tokensOn(g, p, "Soldier Token")); n != 3 {
			t.Errorf("soldiers = %d, want 3 -- Type$ ReplaceController is not ported, replacement fails closed", n)
		}
	})

	t.Run("AmountUnresolvable", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTokenGame(t)
		g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Bad Amount Op",
			"Event$ CreateToken | ActiveZones$ Battlefield | ValidToken$ Card.YouCtrl | ReplaceWith$ Op | Description$ Op.",
			"Op", "DB$ ReplaceToken | Type$ Amount | Amount$ Bogus"), p, engine.Battlefield)
		resolveLine(t, g, p, engine.NewScriptedController(), "DB$ Token | TokenScript$ w_1_1_soldier | TokenAmount$ 3")
		if n := len(tokensOn(g, p, "Soldier Token")); n != 3 {
			t.Errorf("soldiers = %d, want 3 -- Amount$ Bogus is not resolvable, replacement fails closed", n)
		}
	})

	t.Run("PlusOperandNotANumber", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTokenGame(t)
		g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Bad Plus Operand",
			"Event$ CreateToken | ActiveZones$ Battlefield | ValidToken$ Card.YouCtrl | ReplaceWith$ Op | Description$ Op.",
			"Op", "DB$ ReplaceToken | Type$ Amount | Amount$ Plus.Bogus"), p, engine.Battlefield)
		resolveLine(t, g, p, engine.NewScriptedController(), "DB$ Token | TokenScript$ w_1_1_soldier | TokenAmount$ 3")
		if n := len(tokensOn(g, p, "Soldier Token")); n != 3 {
			t.Errorf("soldiers = %d, want 3 -- Plus.Bogus's operand is not a number, replacement fails closed", n)
		}
	})

	t.Run("ConditionUnmet", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTokenGame(t)
		g.NewCard(replacementEnchantmentDefWithSVars(t, "Test Token Condition Unmet",
			"Event$ CreateToken | ActiveZones$ Battlefield | ValidToken$ Card.YouCtrl | ReplaceWith$ Op | Description$ Op.",
			map[string]string{"Op": "DB$ ReplaceToken | Type$ Amount | ConditionCheckSVar$ Armed | ConditionSVarCompare$ GE1", "Armed": "0"}), p, engine.Battlefield)
		resolveLine(t, g, p, engine.NewScriptedController(), "DB$ Token | TokenScript$ w_1_1_soldier | TokenAmount$ 3")
		if n := len(tokensOn(g, p, "Soldier Token")); n != 3 {
			t.Errorf("soldiers = %d, want 3 -- unmet condition, no doubling", n)
		}
	})

	t.Run("OutsideAReplacementErrors", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGame(t)
		_, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, "DB$ ReplaceToken | Type$ Amount")
		if err == nil || !strings.Contains(err.Error(), "not resolving as a replacement") {
			t.Errorf("err = %v, want a not-resolving-as-a-replacement error", err)
		}
	})
}

// TestReplaceCounterFailsClosedOnMissingAmount proves ReplaceCounter with
// no Amount$ fails the line rather than defaulting to something.
func TestReplaceCounterFailsClosedOnMissingAmount(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test No Amount",
		"Event$ AddCounter | ActiveZones$ Battlefield | ValidCard$ Creature.YouCtrl | ValidCounterType$ P1P1 | ReplaceWith$ NoAmount | Description$ No Amount$.",
		"NoAmount", "DB$ ReplaceCounter"), p, engine.Battlefield)
	host := resolveLine(t, g, p, engine.NewScriptedController(),
		"DB$ PutCounter | Defined$ Self | CounterType$ P1P1 | CounterNum$ 2")
	if got := g.Card(host).Counters.Count(engine.P1P1); got != 2 {
		t.Errorf("+1/+1 counters = %d, want 2 -- no Amount$, replacement fails closed", got)
	}
}

// TestReplaceCounterFailsClosedOnValidSourceAndWrongEventWiring proves
// ValidSource$ is rejected (not silently ignored) and an amountName
// mismatch (this API wired to a non-AddCounter event) fails to resolve.
func TestReplaceCounterFailsClosedOnValidSourceAndWrongEventWiring(t *testing.T) {
	t.Parallel()

	t.Run("ValidSourceRejected", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGame(t)
		g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Valid Source",
			"Event$ AddCounter | ActiveZones$ Battlefield | ValidCard$ Creature.YouCtrl | ValidCounterType$ P1P1 | ReplaceWith$ Src | Description$ Src.",
			"Src", "DB$ ReplaceCounter | ValidSource$ Spell | Amount$ 99"), p, engine.Battlefield)
		host := resolveLine(t, g, p, engine.NewScriptedController(),
			"DB$ PutCounter | Defined$ Self | CounterType$ P1P1 | CounterNum$ 2")
		if got := g.Card(host).Counters.Count(engine.P1P1); got != 2 {
			t.Errorf("+1/+1 counters = %d, want 2 -- ValidSource$ rejected, replacement fails closed", got)
		}
	})

	t.Run("AmountUnresolvable", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGame(t)
		g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Amount Unresolvable Counter",
			"Event$ AddCounter | ActiveZones$ Battlefield | ValidCard$ Creature.YouCtrl | ValidCounterType$ P1P1 | ReplaceWith$ Bad | Description$ Bad.",
			"Bad", "DB$ ReplaceCounter | Amount$ Bogus"), p, engine.Battlefield)
		host := resolveLine(t, g, p, engine.NewScriptedController(),
			"DB$ PutCounter | Defined$ Self | CounterType$ P1P1 | CounterNum$ 2")
		if got := g.Card(host).Counters.Count(engine.P1P1); got != 2 {
			t.Errorf("+1/+1 counters = %d, want 2 -- Amount$ Bogus is not resolvable, replacement fails closed", got)
		}
	})

	t.Run("WrongEventWiring", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGame(t)
		g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Wrong Event Counter",
			"Event$ DamageDone | ValidTarget$ You | ReplaceWith$ WrongAPI | Description$ Wrong API for this event.",
			"WrongAPI", "DB$ ReplaceCounter | Amount$ 99"), p, engine.Battlefield)
		resolveLine(t, g, p, engine.NewScriptedController(), "DB$ DealDamage | Defined$ You | NumDmg$ 2")
		if got := g.Player(p).Life; got != 18 {
			t.Errorf("life = %d, want 18 -- ReplaceCounter wired to DamageDone must fail closed", got)
		}
	})
}

// TestReplaceManaFailsClosedOnBadAmountAndUnresolvableColor proves
// ReplaceAmount$'s own non-numeric error and manaTypeByName's own rejection
// of an unrecognized color name each fail closed, the mana produced
// unedited.
func TestReplaceManaFailsClosedOnBadAmountAndUnresolvableColor(t *testing.T) {
	t.Parallel()

	t.Run("ReplaceAmountNotANumber", func(t *testing.T) {
		t.Parallel()
		g := newGame(t, "a", "b")
		p := g.Players()[0]
		g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Bad Replace Amount",
			"Event$ ProduceMana | ActiveZones$ Battlefield | ValidCard$ Land | ReplaceWith$ Bad | Description$ Bad.",
			"Bad", "DB$ ReplaceMana | ReplaceAmount$ Bogus"), p, engine.Battlefield)
		forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
		if !g.TapLandForMana(p, forest, mana.Green, engine.NewScriptedController()) {
			t.Fatal("TapLandForMana failed")
		}
		if got := g.Player(p).ManaPool.Breakdown(); got != [6]int{0, 0, 0, 0, 1, 0} {
			t.Errorf("pool = %v, want one green unedited -- ReplaceAmount$ Bogus fails closed", got)
		}
	})

	t.Run("UnrecognizedColorName", func(t *testing.T) {
		t.Parallel()
		g := newGame(t, "a", "b")
		p := g.Players()[0]
		g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Bad Color Name",
			"Event$ ProduceMana | ActiveZones$ Battlefield | ValidCard$ Land | ReplaceWith$ Bad | Description$ Bad.",
			"Bad", "DB$ ReplaceMana | ReplaceType$ Bogus"), p, engine.Battlefield)
		forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
		if !g.TapLandForMana(p, forest, mana.Green, engine.NewScriptedController()) {
			t.Fatal("TapLandForMana failed")
		}
		if got := g.Player(p).ManaPool.Breakdown(); got != [6]int{0, 0, 0, 0, 1, 0} {
			t.Errorf("pool = %v, want one green unedited -- ReplaceType$ Bogus fails closed", got)
		}
	})
}

// TestReplaceManaMoreShapes covers ReplaceMana's remaining branches:
// ConditionCheckSVar$ gating, wrong-event wiring, ReplaceColor$ Chosen
// (with and without a chosen color), ReplaceOnly$ skipping a
// non-matching unit, a colorless unit never recoloring, no
// ReplaceMana$/ReplaceType$/ReplaceColor$/ReplaceAmount$ at all, and
// ReplaceMana$ Any without a controller or with an unusable answer.
func TestReplaceManaMoreShapes(t *testing.T) {
	t.Parallel()

	t.Run("ConditionUnmet", func(t *testing.T) {
		t.Parallel()
		g := newGame(t, "a", "b")
		p := g.Players()[0]
		g.NewCard(replacementEnchantmentDefWithSVars(t, "Test Mana Condition Unmet",
			"Event$ ProduceMana | ActiveZones$ Battlefield | ValidCard$ Land | ReplaceWith$ ProduceB | Description$ Black instead.",
			map[string]string{"ProduceB": "DB$ ReplaceMana | ReplaceType$ B | ConditionCheckSVar$ Armed | ConditionSVarCompare$ GE1", "Armed": "0"}), p, engine.Battlefield)
		forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
		if !g.TapLandForMana(p, forest, mana.Green, engine.NewScriptedController()) {
			t.Fatal("TapLandForMana failed")
		}
		if got := g.Player(p).ManaPool.Breakdown(); got != [6]int{0, 0, 0, 0, 1, 0} {
			t.Errorf("pool = %v, want one green unedited -- unmet condition", got)
		}
	})

	t.Run("WrongEventWiring", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGame(t)
		g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Wrong Event Mana",
			"Event$ DamageDone | ValidTarget$ You | ReplaceWith$ WrongAPI | Description$ Wrong API for this event.",
			"WrongAPI", "DB$ ReplaceMana | ReplaceType$ B"), p, engine.Battlefield)
		resolveLine(t, g, p, engine.NewScriptedController(), "DB$ DealDamage | Defined$ You | NumDmg$ 2")
		if got := g.Player(p).Life; got != 18 {
			t.Errorf("life = %d, want 18 -- ReplaceMana wired to DamageDone must fail closed", got)
		}
	})

	t.Run("NoReplaceParamErrors", func(t *testing.T) {
		t.Parallel()
		g := newGame(t, "a", "b")
		p := g.Players()[0]
		g.NewCard(replacementEnchantmentDefWithSVar(t, "Test No Replace Param",
			"Event$ ProduceMana | ActiveZones$ Battlefield | ValidCard$ Land | ReplaceWith$ Nothing | Description$ Nothing.",
			"Nothing", "DB$ ReplaceMana"), p, engine.Battlefield)
		forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
		if !g.TapLandForMana(p, forest, mana.Green, engine.NewScriptedController()) {
			t.Fatal("TapLandForMana failed")
		}
		if got := g.Player(p).ManaPool.Breakdown(); got != [6]int{0, 0, 0, 0, 1, 0} {
			t.Errorf("pool = %v, want one green unedited -- no replace param at all", got)
		}
	})

	t.Run("ChosenColorApplies", func(t *testing.T) {
		t.Parallel()
		g := newGame(t, "a", "b")
		p := g.Players()[0]
		g.Card(g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)).Memory.SetChosenColors(mana.Blue)
		host := g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Chosen Color",
			"Event$ ProduceMana | ActiveZones$ Battlefield | ValidCard$ Land | ReplaceWith$ Recolor | Description$ Recolor.",
			"Recolor", "DB$ ReplaceMana | ReplaceColor$ Chosen"), p, engine.Battlefield)
		g.Card(host).Memory.SetChosenColors(mana.Blue)
		forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
		if !g.TapLandForMana(p, forest, mana.Green, engine.NewScriptedController()) {
			t.Fatal("TapLandForMana failed")
		}
		if got := g.Player(p).ManaPool.Breakdown(); got != [6]int{0, 1, 0, 0, 0, 0} {
			t.Errorf("pool = %v, want one blue (ReplaceColor$ Chosen)", got)
		}
	})

	t.Run("ChosenColorWithNoneChosenErrors", func(t *testing.T) {
		t.Parallel()
		g := newGame(t, "a", "b")
		p := g.Players()[0]
		g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Chosen Color None",
			"Event$ ProduceMana | ActiveZones$ Battlefield | ValidCard$ Land | ReplaceWith$ Recolor | Description$ Recolor.",
			"Recolor", "DB$ ReplaceMana | ReplaceColor$ Chosen"), p, engine.Battlefield)
		forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
		if !g.TapLandForMana(p, forest, mana.Green, engine.NewScriptedController()) {
			t.Fatal("TapLandForMana failed")
		}
		if got := g.Player(p).ManaPool.Breakdown(); got != [6]int{0, 0, 0, 0, 1, 0} {
			t.Errorf("pool = %v, want one green unedited -- Chosen with no chosen color fails closed", got)
		}
	})

	t.Run("ReplaceColorUnresolvableName", func(t *testing.T) {
		t.Parallel()
		g := newGame(t, "a", "b")
		p := g.Players()[0]
		g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Bad Color Name Recolor",
			"Event$ ProduceMana | ActiveZones$ Battlefield | ValidCard$ Land | ReplaceWith$ Recolor | Description$ Recolor.",
			"Recolor", "DB$ ReplaceMana | ReplaceColor$ Bogus"), p, engine.Battlefield)
		forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
		if !g.TapLandForMana(p, forest, mana.Green, engine.NewScriptedController()) {
			t.Fatal("TapLandForMana failed")
		}
		if got := g.Player(p).ManaPool.Breakdown(); got != [6]int{0, 0, 0, 0, 1, 0} {
			t.Errorf("pool = %v, want one green unedited -- ReplaceColor$ Bogus fails closed", got)
		}
	})

	t.Run("ColorlessUnitNeverRecolors", func(t *testing.T) {
		t.Parallel()
		g := newGame(t, "a", "b")
		p := g.Players()[0]
		g.SetTurnState(1, p, engine.Main1)
		g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
		g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Colorless Untouched",
			"Event$ ProduceMana | ActiveZones$ Battlefield | ReplaceWith$ Recolor | Description$ Recolor.",
			"Recolor", "DB$ ReplaceMana | ReplaceColor$ Blue"), p, engine.Battlefield)
		rock := g.NewCard(creatureDefWithAbility(t, "Test Mana Rock", "AB$ Mana | Cost$ T | Produced$ C"), p, engine.Battlefield)
		if !g.ActivateManaAbility(p, rock, 0, engine.NewScriptedController()) {
			t.Fatal("ActivateManaAbility returned false, want true")
		}
		if got := g.Player(p).ManaPool.Breakdown(); got != [6]int{0, 0, 0, 0, 0, 1} {
			t.Errorf("pool = %v, want one colorless untouched -- ReplaceColor$ never recolors {C}", got)
		}
	})

	t.Run("AnyWithoutControllerErrors", func(t *testing.T) {
		t.Parallel()
		g := newGame(t, "a", "b")
		p := g.Players()[0]
		g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Any No Controller",
			"Event$ ProduceMana | ActiveZones$ Battlefield | ValidCard$ Land | ReplaceWith$ ProduceAny | Description$ Any color.",
			"ProduceAny", "DB$ ReplaceMana | ReplaceMana$ Any"), p, engine.Battlefield)
		forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
		if !g.TapLandForMana(p, forest, mana.Green, nil) {
			t.Fatal("TapLandForMana failed")
		}
		if got := g.Player(p).ManaPool.Breakdown(); got != [6]int{0, 0, 0, 0, 1, 0} {
			t.Errorf("pool = %v, want one green unedited -- Any with no controller to ask fails closed", got)
		}
	})

	t.Run("AnyWithUnusableAnswerErrors", func(t *testing.T) {
		t.Parallel()
		g := newGame(t, "a", "b")
		p := g.Players()[0]
		g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Any Bad Answer",
			"Event$ ProduceMana | ActiveZones$ Battlefield | ValidCard$ Land | ReplaceWith$ ProduceAny | Description$ Any color.",
			"ProduceAny", "DB$ ReplaceMana | ReplaceMana$ Any"), p, engine.Battlefield)
		forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
		c := engine.NewScriptedController()
		c.QueueManaColor(mana.Blue | mana.Red)
		if !g.TapLandForMana(p, forest, mana.Green, c) {
			t.Fatal("TapLandForMana failed")
		}
		if got := g.Player(p).ManaPool.Breakdown(); got != [6]int{0, 0, 0, 0, 1, 0} {
			t.Errorf("pool = %v, want one green unedited -- two colors is not exactly one color, fails closed", got)
		}
	})

	t.Run("WordColorNames", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name string
			word string
			want [6]int
		}{
			{"White", "White", [6]int{1, 0, 0, 0, 0, 0}},
			{"Black", "Black", [6]int{0, 0, 1, 0, 0, 0}},
			{"Colorless", "Colorless", [6]int{0, 0, 0, 0, 0, 1}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				g := newGame(t, "a", "b")
				p := g.Players()[0]
				g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Word Color "+tc.name,
					"Event$ ProduceMana | ActiveZones$ Battlefield | ValidCard$ Land | ReplaceWith$ ProduceWord | Description$ Word color.",
					"ProduceWord", "DB$ ReplaceMana | ReplaceType$ "+tc.word), p, engine.Battlefield)
				forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
				if !g.TapLandForMana(p, forest, mana.Green, engine.NewScriptedController()) {
					t.Fatal("TapLandForMana failed")
				}
				if got := g.Player(p).ManaPool.Breakdown(); got != tc.want {
					t.Errorf("pool = %v, want %v", got, tc.want)
				}
			})
		}
	})
}

// TestReplaceEffectsOutsideAReplacement proves the Replace* effects'
// behaviour on the stack: ReplaceEffect has no event to edit and fails,
// the other five do nothing (`if (!sa.isReplacementAbility()) return;`).
func TestReplaceEffectsOutsideAReplacement(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	_, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, "DB$ ReplaceEffect | VarName$ DamageAmount | VarValue$ 2")
	if err == nil || !strings.Contains(err.Error(), "not resolving as a replacement") {
		t.Errorf("ReplaceEffect err = %v, want not resolving as a replacement", err)
	}
	for _, line := range []string{
		"DB$ ReplaceDamage | Amount$ 1",
		"DB$ ReplaceSplitDamage | DamageTarget$ You",
		"DB$ ReplaceCounter | Amount$ 1",
		"DB$ ReplaceMana | ReplaceType$ B",
	} {
		g, p, _ := newTwoPlayerGame(t)
		if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, line); err != nil {
			t.Errorf("%q: err = %v, want a silent no-op", line, err)
		}
	}
}
