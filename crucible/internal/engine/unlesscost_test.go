package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// TestUnlessCostDeclinedRunsAbility proves resolveUnlessCost's own default
// (UnlessSwitched$ absent) shape: nobody paying means the ability's body
// runs, handleUnlessCost's own `alreadyPaid == isSwitched` with both false --
// nicol_bolas.txt's own real "sacrifice CARDNAME unless you pay {U}{B}{R}"
// shape, trimmed to a resolvable {G}{G}.
func TestUnlessCostDeclinedRunsAbility(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	c.QueueConfirmPayCost(false)
	def := etbSacrificeTriggerDefParams(t, "Test UnlessCost Declined", "UnlessCost$ G G | UnlessPayer$ You", nil)
	creature, err := castETBSacrifice(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(creature).Zone != engine.Graveyard {
		t.Errorf("creature zone = %v, want Graveyard -- declining the cost must let Sacrifice run", g.Card(creature).Zone)
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("mana pool total = %d, want 0 -- a decline must never touch the pool", got)
	}
}

// TestUnlessCostPaidPreventsAbility proves the other half: a confirmed,
// successful payment prevents the ability's own body from running at all,
// and the mana pool is actually charged (PayManaCost, manapay.go) rather
// than only asked about.
func TestUnlessCostPaidPreventsAbility(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	c.QueueConfirmPayCost(true)
	c.QueuePayGeneric(mana.ShardG)
	c.QueuePayGeneric(mana.ShardG)
	def := etbSacrificeTriggerDefParams(t, "Test UnlessCost Paid", "UnlessCost$ 2 | UnlessPayer$ You", nil)

	g.Player(p).ManaPool.Add(mana.Green, 2)
	creature, err := castETBSacrifice(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(creature).Zone != engine.Battlefield {
		t.Errorf("creature zone = %v, want Battlefield -- paying the cost must prevent Sacrifice from running", g.Card(creature).Zone)
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("mana pool total = %d, want 0 -- the generic {2} cost must actually be charged", got)
	}
}

// TestUnlessCostConfirmedButUnaffordableStillRunsAbility proves
// ConfirmPayCost and PayManaCost are two separate steps (resolveUnlessCost's
// own doc comment, effect.go): a payer who says yes but cannot actually
// afford the cost has not paid, the identical "declined by the rules, not a
// bug" outcome PayManaCost's own failure already carries everywhere else.
func TestUnlessCostConfirmedButUnaffordableStillRunsAbility(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	c.QueueConfirmPayCost(true)
	def := etbSacrificeTriggerDefParams(t, "Test UnlessCost Unaffordable", "UnlessCost$ G | UnlessPayer$ You", nil)

	// No mana added to the pool at all: PayManaCost must fail.
	creature, err := castETBSacrifice(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(creature).Zone != engine.Graveyard {
		t.Errorf("creature zone = %v, want Graveyard -- a failed payment must not prevent Sacrifice", g.Card(creature).Zone)
	}
}

// TestUnlessCostSwitchedInvertsOutcome proves UnlessSwitched$'s own flip:
// the ability now runs when the cost IS paid, and does not when it is not
// -- handleUnlessCost's own `alreadyPaid == isSwitched` with isSwitched
// true.
func TestUnlessCostSwitchedInvertsOutcome(t *testing.T) {
	t.Parallel()

	t.Run("paid runs the ability", func(t *testing.T) {
		t.Parallel()
		g := newGame(t, "a", "b")
		p := g.Players()[0]
		g.SetTurnState(1, p, engine.Main1)
		g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

		c := engine.NewScriptedController()
		c.QueueConfirmPayCost(true)
		g.Player(p).ManaPool.Add(mana.Green, 1)
		def := etbSacrificeTriggerDefParams(t, "Test Switched Paid", "UnlessCost$ G | UnlessPayer$ You | UnlessSwitched$ True", nil)

		creature, err := castETBSacrifice(t, g, p, def, c)
		if err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if g.Card(creature).Zone != engine.Graveyard {
			t.Errorf("creature zone = %v, want Graveyard -- UnlessSwitched$ makes paying run the ability", g.Card(creature).Zone)
		}
	})

	t.Run("declined does not run the ability", func(t *testing.T) {
		t.Parallel()
		g := newGame(t, "a", "b")
		p := g.Players()[0]
		g.SetTurnState(1, p, engine.Main1)
		g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

		c := engine.NewScriptedController()
		c.QueueConfirmPayCost(false)
		def := etbSacrificeTriggerDefParams(t, "Test Switched Declined", "UnlessCost$ G | UnlessPayer$ You | UnlessSwitched$ True", nil)

		creature, err := castETBSacrifice(t, g, p, def, c)
		if err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if g.Card(creature).Zone != engine.Battlefield {
			t.Errorf("creature zone = %v, want Battlefield -- UnlessSwitched$ makes declining skip the ability", g.Card(creature).Zone)
		}
	})
}

// TestUnlessCostResolveSubsWhenNotPaidGatesTheChain proves
// UnlessResolveSubs$'s own "WhenNotPaid" value: the chained SubAbility$
// runs only on the branch it names, not the identical unconditional
// trailing chain every other ability gets (Registry.Resolve's own doc
// comment, effect.go).
func TestUnlessCostResolveSubsWhenNotPaidGatesTheChain(t *testing.T) {
	t.Parallel()

	svars := map[string]string{"DBGainLife": "DB$ GainLife | Defined$ You | LifeAmount$ 2"}
	extra := "UnlessCost$ G | UnlessPayer$ You | UnlessResolveSubs$ WhenNotPaid | SubAbility$ DBGainLife"

	t.Run("declined chains the sub", func(t *testing.T) {
		t.Parallel()
		g := newGame(t, "a", "b")
		p := g.Players()[0]
		g.SetTurnState(1, p, engine.Main1)
		g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

		c := engine.NewScriptedController()
		c.QueueConfirmPayCost(false)
		def := etbSacrificeTriggerDefParams(t, "Test ResolveSubs Declined", extra, svars)

		creature, err := castETBSacrifice(t, g, p, def, c)
		if err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if g.Card(creature).Zone != engine.Graveyard {
			t.Errorf("creature zone = %v, want Graveyard", g.Card(creature).Zone)
		}
		if g.Player(p).Life != 22 {
			t.Errorf("p's life = %d, want 22 -- WhenNotPaid must chain the sub on a decline", g.Player(p).Life)
		}
	})

	t.Run("paid skips the sub", func(t *testing.T) {
		t.Parallel()
		g := newGame(t, "a", "b")
		p := g.Players()[0]
		g.SetTurnState(1, p, engine.Main1)
		g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

		c := engine.NewScriptedController()
		c.QueueConfirmPayCost(true)
		g.Player(p).ManaPool.Add(mana.Green, 1)
		def := etbSacrificeTriggerDefParams(t, "Test ResolveSubs Paid", extra, svars)

		creature, err := castETBSacrifice(t, g, p, def, c)
		if err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if g.Card(creature).Zone != engine.Battlefield {
			t.Errorf("creature zone = %v, want Battlefield", g.Card(creature).Zone)
		}
		if g.Player(p).Life != 20 {
			t.Errorf("p's life = %d, want 20 -- WhenNotPaid must not chain the sub once paid", g.Player(p).Life)
		}
	})
}

// TestUnlessCostAsksEveryDefinedPlayerAccumulatingWithOr proves
// UnlessPayer$ Player asks every player in turn (definedPlayers, reused)
// and one payer succeeding is enough to prevent the ability -- Java's own
// `alreadyPaid |= payer.getController().payCostToPreventEffect(...)`.
func TestUnlessCostAsksEveryDefinedPlayerAccumulatingWithOr(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20

	c := engine.NewScriptedController()
	// p (first in turn order) declines; other pays.
	c.QueueConfirmPayCost(false)
	c.QueueConfirmPayCost(true)
	g.Player(other).ManaPool.Add(mana.Green, 1)

	def := etbSacrificeTriggerDefParams(t, "Test Player Payer", "UnlessCost$ G | UnlessPayer$ Player", nil)
	creature, err := castETBSacrifice(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(creature).Zone != engine.Battlefield {
		t.Errorf("creature zone = %v, want Battlefield -- either player paying must prevent Sacrifice", g.Card(creature).Zone)
	}
	if got := g.Player(other).ManaPool.Total(); got != 0 {
		t.Errorf("other's mana pool total = %d, want 0 -- other's own payment must actually be charged", got)
	}
}

// TestUnlessCostRejectsAbsentUnlessPayer proves an absent UnlessPayer$ --
// Java's own "TargetedController" default -- is not resolved (resolveUnlessCost's
// own doc comment, effect.go), so the whole line fails loudly (PORT-8/GO-7)
// rather than guessing who is asked.
func TestUnlessCostRejectsAbsentUnlessPayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	def := etbSacrificeTriggerDefParams(t, "Test Absent UnlessPayer", "UnlessCost$ 1", nil)
	_, err := castETBSacrifice(t, g, p, def, c)
	if err == nil {
		t.Fatal("ResolveStack: got nil error, want one naming UnlessPayer")
	}
	if !strings.Contains(err.Error(), "UnlessPayer") {
		t.Errorf("ResolveStack error = %q, want it to name UnlessPayer$", err.Error())
	}
}
