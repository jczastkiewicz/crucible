package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// optionalDeciderTriggerDef builds an Enchantment carrying one Mode$
// AttackersDeclared trigger naming decider (OptionalDecider$'s own value,
// "You" or an unresolved reference), Execute$ chaining into a Draw that
// itself chains a SubAbility$ LoseLife -- CR 616's own "then" pairing
// (SubAbility chaining, subability.go), reused here to prove a declined
// "may" skips the WHOLE ability, not just its own top-level body.
func optionalDeciderTriggerDef(t *testing.T, name, decider string) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Enchantment")
	raw.Faces[0].Triggers = []string{
		"Mode$ AttackersDeclared | OptionalDecider$ " + decider + " | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1 | SubAbility$ TrigLoseLife")
	raw.Faces[0].SVars.Set("TrigLoseLife", "DB$ LoseLife | Defined$ You | LifeAmount$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestConfirmedOptionalTriggerRunsWholeAbilityChain proves a confirmed
// OptionalDecider$ You trigger runs its own body AND its own SubAbility$
// chain -- WrappedAbility.resolve()'s own confirm happens once, before
// playSpellAbilityNoStack, which then runs the whole chain normally exactly
// as if the ability had never been optional (Registry.Resolve's own doc
// comment, effect.go).
func TestConfirmedOptionalTriggerRunsWholeAbilityChain(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	g.NewCard(optionalDeciderTriggerDef(t, "Test Optional Chain", "You"), p, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	ac.QueueConfirmOptionalTrigger(true)
	g.DeclareCombatAttackers(ac)
	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := len(g.Zone(engine.Hand, p).Cards()); got != 1 {
		t.Errorf("hand has %d cards, want 1 -- a confirmed ability must draw", got)
	}
	if got := g.Player(p).Life; got != 19 {
		t.Errorf("p life = %d, want 19 -- a confirmed ability's own SubAbility$ chain must run too", got)
	}
}

// TestDeclinedOptionalTriggerSkipsWholeAbilityChain is the same trigger's
// own negative twin: declined, neither the top-level Draw nor the chained
// LoseLife runs at all.
func TestDeclinedOptionalTriggerSkipsWholeAbilityChain(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	g.NewCard(optionalDeciderTriggerDef(t, "Test Optional Chain", "You"), p, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	ac.QueueConfirmOptionalTrigger(false)
	g.DeclareCombatAttackers(ac)
	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := len(g.Zone(engine.Hand, p).Cards()); got != 0 {
		t.Errorf("hand has %d cards, want 0 -- a declined ability must not draw", got)
	}
	if got := g.Player(p).Life; got != 20 {
		t.Errorf("p life = %d, want unchanged 20 -- a declined ability must not chain into its own SubAbility$ either", got)
	}
}

// TestUnresolvedOptionalDeciderSkipsTriggerWithoutAsking proves the PORT-8
// fail-closed half: a decider this port cannot resolve (anything other than
// "You") skips the whole trigger line before ever reaching
// ConfirmOptionalTrigger -- if it asked, the scripted controller's own empty
// queue would panic, so a clean run with nothing drawn or lost proves the
// question was never posed at all, not merely answered no.
func TestUnresolvedOptionalDeciderSkipsTriggerWithoutAsking(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	g.NewCard(optionalDeciderTriggerDef(t, "Test Optional Chain", "TriggeredCardController"), p, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := len(g.Zone(engine.Hand, p).Cards()); got != 0 {
		t.Errorf("hand has %d cards, want 0 -- an unresolved OptionalDecider$ value must skip the whole line", got)
	}
}
