package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// combatCreature is a creature for the declaration-legality tests: name,
// power/toughness and any K:/S:/Cost: lines (copyTestDef).
func combatCreature(t *testing.T, name, power, toughness string, lines ...string) *compile.Card {
	t.Helper()
	return copyTestDef(t, name, "Creature Elf", power, toughness, lines...)
}

// declareAttack declares attackers for the active player and returns what
// was declared and the error the declaration left, if any.
func declareAttack(g *engine.Game, attackers ...engine.CardID) ([]engine.CardID, error) {
	c := engine.NewScriptedController()
	c.QueueAttackers(attackers)
	return g.DeclareCombatAttackers(c)
}

// wantAttackError asserts a declaration was rejected with an error naming
// rule, and changed nothing: no attacker, nothing tapped.
func wantAttackError(t *testing.T, g *engine.Game, rule string, attackers ...engine.CardID) {
	t.Helper()
	before := append([]engine.CardID(nil), g.Attackers()...)
	var wasTapped []bool
	for _, id := range attackers {
		wasTapped = append(wasTapped, g.Card(id).Tapped)
	}
	got, err := declareAttack(g, attackers...)
	if err == nil || !strings.Contains(err.Error(), rule) {
		t.Fatalf("declare %v: error = %v, want one containing %q", attackers, err, rule)
	}
	if after := g.Attackers(); got != nil || len(after) != len(before) {
		t.Fatalf("rejected declaration declared %v, combat now %v", got, after)
	}
	for i, id := range attackers {
		if g.Card(id).Tapped != wasTapped[i] {
			t.Fatalf("rejected declaration tapped %v", id)
		}
	}
}

// wantAttackLegal asserts a declaration went through as declared.
func wantAttackLegal(t *testing.T, g *engine.Game, attackers ...engine.CardID) {
	t.Helper()
	got, err := declareAttack(g, attackers...)
	if err != nil {
		t.Fatalf("declare %v: %v", attackers, err)
	}
	if len(got) != len(attackers) {
		t.Fatalf("declared %v, want %v", got, attackers)
	}
}

func TestMustAttackConditionGatesTheRequirement(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.DeclareAttackers)
	brute := g.NewCard(combatCreature(t, "Brute", "3", "3", "S:Mode$ MustAttack | ValidCreature$ Card.Self | Condition$ Hellbent"), p, engine.Battlefield)
	g.NewCard(combatCreature(t, "Card In Hand", "1", "1"), p, engine.Hand)

	wantAttackLegal(t, g)
	g.Move(g.Zone(engine.Hand, p).Cards()[0], engine.Graveyard, p)
	wantAttackError(t, g, "508.1d")
	wantAttackLegal(t, g, brute)
}

func TestMustAttackIsPresentGatesTheRequirement(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.DeclareAttackers)
	g.NewCard(combatCreature(t, "Brute", "3", "3", "S:Mode$ MustAttack | ValidCreature$ Card.Self | IsPresent$ Wall.YouCtrl | PresentCompare$ EQ0"), p, engine.Battlefield)
	g.NewCard(copyTestDef(t, "Wall", "Creature Wall", "0", "4"), p, engine.Battlefield)

	wantAttackLegal(t, g)
}

// MustAttack$ names the entity to attack (CR 508.1d): attacking anyone
// else leaves that requirement unmet.
func TestMustAttackNamedDefender(t *testing.T) {
	t.Parallel()
	g := newGame(t, "a", "b", "c")
	ps := g.Players()
	g.SetTurnState(1, ps[0], engine.DeclareAttackers)
	// The static's host belongs to b, so MustAttack$ You is b.
	g.NewCard(continuousDef(t, "Taunt", "Mode$ MustAttack | ValidCreature$ Creature.OppCtrl | MustAttack$ You"), ps[1], engine.Battlefield)
	brute := g.NewCard(combatCreature(t, "Brute", "3", "3"), ps[0], engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueAttackers([]engine.CardID{brute})
	c.QueueAttackTarget(engine.PlayerEntity(ps[2]))
	if got, err := g.DeclareCombatAttackers(c); got != nil || err == nil || !strings.Contains(err.Error(), "508.1d") {
		t.Fatalf("declared %v, error %v, want the attack on c rejected by CR 508.1d", got, err)
	}
	c.QueueAttackers([]engine.CardID{brute})
	c.QueueAttackTarget(engine.PlayerEntity(ps[1]))
	if got, err := g.DeclareCombatAttackers(c); err != nil || len(got) != 1 {
		t.Fatalf("declared %v, error %v, want the attack on b", got, err)
	}
}

// CR 506.2: the active player's own entity is skipped as a MustAttack$
// target, leaving no requirement.
func TestMustAttackSkipsTheActivePlayer(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.DeclareAttackers)
	g.NewCard(combatCreature(t, "Brute", "3", "3", "S:Mode$ MustAttack | ValidCreature$ Card.Self | MustAttack$ You"), p, engine.Battlefield)

	wantAttackLegal(t, g)
}

func TestAttackRestrictionTwoOthersMetByThree(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.DeclareAttackers)
	shy := g.NewCard(combatCreature(t, "Shy", "1", "1", "K:CARDNAME can't attack unless at least two other creatures attack."), p, engine.Battlefield)
	a := g.NewCard(combatCreature(t, "A", "1", "1"), p, engine.Battlefield)
	b := g.NewCard(combatCreature(t, "B", "1", "1"), p, engine.Battlefield)
	wantAttackLegal(t, g, shy, a, b)
}
