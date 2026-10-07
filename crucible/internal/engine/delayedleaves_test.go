package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// A delayed Mode$ ChangesZone watch from the battlefield with Destination$ Any
// fires for every way the watched permanent leaves, bounce and library moves
// included (TriggerChangesZone.performTest), and a static leaves-the-
// battlefield trigger naming Library fires for a tuck.
func TestDelayedChangesZoneWatchFiresWhenThePermanentIsBouncedOrTucked(t *testing.T) {
	t.Parallel()

	const watch = "DB$ DelayedTrigger | Mode$ ChangesZone | Origin$ Battlefield | Destination$ Any | ValidCard$ Card.IsTriggerRemembered | RememberObjects$ Targeted | Execute$ DBGain"
	for _, dest := range []string{"Hand", "Library", "Graveyard", "Exile"} {
		g, p, other := newTwoPlayerGame(t)
		victim := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
		target := []engine.EntityID{engine.CardEntity(victim)}
		if _, err := resolveNow(t, g, p, engine.NewScriptedController(), target, watch,
			"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ 3"); err != nil {
			t.Fatalf("%s: %v", dest, err)
		}
		if _, err := resolveNow(t, g, p, engine.NewScriptedController(), target,
			"DB$ ChangeZone | ValidTgts$ Creature | Origin$ Battlefield | Destination$ "+dest); err != nil {
			t.Fatalf("%s: %v", dest, err)
		}
		if got := g.Player(p).Life; got != 23 {
			t.Errorf("%s: life = %d, want 23", dest, got)
		}
	}
}

// A watch for a different permanent does not fire, and fires once only.
func TestDelayedChangesZoneWatchIgnoresOtherPermanentsAndFiresOnce(t *testing.T) {
	t.Parallel()

	const watch = "DB$ DelayedTrigger | Mode$ ChangesZone | Origin$ Battlefield | Destination$ Any | ValidCard$ Card.IsTriggerRemembered | RememberObjects$ Targeted | Execute$ DBGain"
	g, p, other := newTwoPlayerGame(t)
	watched := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	bystander := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(watched)}, watch,
		"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ 3"); err != nil {
		t.Fatal(err)
	}
	bounce := func(id engine.CardID) {
		t.Helper()
		if _, err := resolveNow(t, g, p, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(id)},
			"DB$ ChangeZone | ValidTgts$ Creature | Origin$ Battlefield | Destination$ Hand"); err != nil {
			t.Fatal(err)
		}
	}
	bounce(bystander)
	if got := g.Player(p).Life; got != 20 {
		t.Fatalf("life = %d after an unwatched bounce, want 20", got)
	}
	bounce(watched)
	g.Move(watched, engine.Battlefield, other)
	bounce(watched)
	if got := g.Player(p).Life; got != 23 {
		t.Errorf("life = %d, want 23: the watch fires once", got)
	}
}
