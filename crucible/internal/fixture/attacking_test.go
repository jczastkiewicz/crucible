package fixture_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/fixture"
)

// A declared attacker dumps as |Attacking (GameState.java:463-470), bare
// when it attacks a player.
func TestAttackingDumpsTheDeclaredAttacker(t *testing.T) {
	t.Parallel()

	l := load(t, bearDB(t), "humanlife=20\nailife=20\nhumanbattlefield=Test Bear|Id:1\n")
	c := engine.NewScriptedController()
	if err := runActions(t, l, c, "startturn human\nadvance 5\nqueue attackers 1\ndeclareattackers\n"); err != nil {
		t.Fatalf("RunActions: %v", err)
	}
	var buf strings.Builder
	if err := fixture.Write(&buf, fixture.Dump(l)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !strings.Contains(buf.String(), "|Attacking\n") {
		t.Errorf("dump lacks a bare |Attacking:\n%s", buf.String())
	}
}

// An expect.state's |Attacking is read into Loaded.Attacking, the attacked
// card by its Id:, and builds no combat.
func TestAttackingIsReadWithoutACombat(t *testing.T) {
	t.Parallel()

	l := load(t, bearDB(t), "humanlife=20\nailife=20\nhumanbattlefield=Test Bear|Id:1|Attacking;Test Bear|Id:2|Attacking:3\naibattlefield=Test Bear|Id:3\n")
	if got := l.Attacking[l.CardByFixtureID[1]]; got != engine.NoCard {
		t.Errorf("card 1 attacks %v, want a player (NoCard)", got)
	}
	if got := l.Attacking[l.CardByFixtureID[2]]; got != l.CardByFixtureID[3] {
		t.Errorf("card 2 attacks %v, want card 3", got)
	}
	if n := len(l.Game.Attackers()); n != 0 {
		t.Errorf("Load declared %d attackers, want none", n)
	}
}

// A setup.state that names an attacker is refused: Java's setup puts it into
// combat (GameState.java:1429-1435), Load does not.
func TestAttackingInASetupIsRefused(t *testing.T) {
	t.Parallel()

	l := load(t, bearDB(t), "humanlife=20\nailife=20\nhumanbattlefield=Test Bear|Id:1|Attacking\n")
	if err := runActions(t, l, engine.NewScriptedController(), "startturn human\n"); err == nil {
		t.Error("RunActions accepted a setup attacker")
	}
}

// An Attacking: id no card has is an error, as AttachedTo:'s is.
func TestAttackingAnUnknownIDErrors(t *testing.T) {
	t.Parallel()

	for _, text := range []string{"Test Bear|Id:1|Attacking:9", "Test Bear|Id:1|Attacking:x"} {
		st, err := fixture.Parse(strings.NewReader("humanlife=20\nailife=20\nhumanbattlefield=" + text + "\n"))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if _, err := fixture.Load(st, bearDB(t), nil); err == nil {
			t.Errorf("%s: Load accepted it", text)
		}
	}
}
