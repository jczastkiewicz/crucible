package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Duration$ AsLongAsControl on Pump (Aegis Angel's shape): the keyword stays
// past cleanup, and goes when the host leaves play or changes controller
// (addUntilCommand, SpellAbilityEffect.java:1010-1013).
func TestPumpAsLongAsControlEndsWithTheHostsControl(t *testing.T) {
	t.Parallel()

	const line = "DB$ Pump | ValidTgts$ Creature.OppCtrl | KW$ Indestructible | Duration$ AsLongAsControl"
	for _, how := range []string{"cleanup", "dies", "stolen"} {
		g, p, other := newTwoPlayerGame(t)
		victim := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
		host := stealUntil(t, g, p, victim, line)
		sba(g)
		if !g.Card(victim).HasKeyword("Indestructible") {
			t.Fatalf("%s: the target lacks Indestructible", how)
		}
		want := true
		switch how {
		case "cleanup":
			endTurn(g, 1, p)
		case "dies":
			g.Move(host, engine.Graveyard, p)
			want = false
		case "stolen":
			stealUntil(t, g, other, host, "DB$ GainControl | ValidTgts$ Creature.OppCtrl")
			want = false
		}
		sba(g)
		if got := g.Card(victim).HasKeyword("Indestructible"); got != want {
			t.Errorf("%s: Indestructible = %v, want %v", how, got, want)
		}
	}
}

// checkValidDuration: a host controlled by someone other than the activator
// applies nothing (SpellAbilityEffect.java:1052).
func TestPumpAsLongAsControlNeedsTheActivatorToControlTheHost(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	victim := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	def := etbChainDef(t, "Test Duration", "DB$ Pump | ValidTgts$ Creature.OppCtrl | KW$ Indestructible | Duration$ AsLongAsControl")
	host := g.NewCard(def, other, engine.Battlefield)
	c := engine.NewScriptedController()
	// p activates it, the host is other's.
	a := engine.Ability{API: engine.APIPump, Source: host, Controller: p, Targets: []engine.EntityID{engine.CardEntity(victim)},
		Params: def.Faces[0].Triggers[0].Subs[0].Ability, Amounts: def.Faces[0].Amounts}
	g.PushAbility(a)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	sba(g)
	if g.Card(victim).HasKeyword("Indestructible") {
		t.Error("the keyword applied although the activator does not control the host")
	}
}

// Duration$ AsLongAsControl on Animate: the characteristic change outlives
// cleanup and ends with the host (AnimateEffectBase's closure through
// addUntilCommand).
func TestAnimateAsLongAsControlEndsWithTheHostsControl(t *testing.T) {
	t.Parallel()

	const line = "DB$ Animate | ValidTgts$ Creature.OppCtrl | Power$ 5 | Toughness$ 5 | Duration$ AsLongAsControl"
	for _, how := range []string{"cleanup", "dies"} {
		g, p, other := newTwoPlayerGame(t)
		victim := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
		host := stealUntil(t, g, p, victim, line)
		sba(g)
		want := 5
		if how == "cleanup" {
			endTurn(g, 1, p)
		} else {
			g.Move(host, engine.Graveyard, p)
			want = 2
		}
		sba(g)
		if pw, _ := g.Card(victim).Power(); pw != want {
			t.Errorf("%s: power = %d, want %d", how, pw, want)
		}
	}
}

// Duration$ AsLongAsControl on Goad (Vislor Turlough's shape): the goad lasts
// past the goader's next turn and ends with the host.
func TestGoadAsLongAsControlEndsWithTheHostsControl(t *testing.T) {
	t.Parallel()

	const line = "DB$ Goad | ValidTgts$ Creature.OppCtrl | Duration$ AsLongAsControl"
	g, p, other := newTwoPlayerGame(t)
	victim := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	host := stealUntil(t, g, p, victim, line)
	if !g.Card(victim).IsGoaded() {
		t.Fatal("the target is not goaded")
	}
	g.SetTurnState(3, p, engine.Untap)
	g.AdvancePhase(engine.NewScriptedController())
	if !g.Card(victim).IsGoaded() {
		t.Error("the goad ended at the goader's next turn")
	}
	g.Move(host, engine.Graveyard, p)
	if g.Card(victim).IsGoaded() {
		t.Error("the goad outlived its host")
	}
}

// LoseControl$ StaticCommandCheck (Old Man of the Sea's shape): control ends at
// the first state-based pass where the target's SVar compares true against the
// host's (GameAction.java:1180-1198), here the target's power above the host's.
func TestGainControlStaticCommandCheckEndsWhenTheComparisonHolds(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	victim := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Battlefield)
	stealUntil(t, g, p, victim,
		"DB$ GainControl | ValidTgts$ Creature.OppCtrl | LoseControl$ StaticCommandCheck | StaticCommandCheckSVar$ Y | StaticCommandSVarCompare$ GTX",
		"X", "Count$CardPower", "Y", "Count$CardPower")
	sba(g)
	if got := g.Card(victim).Controller(); got != p {
		t.Fatalf("controller = %v, want the thief %v while the target is no stronger", got, p)
	}
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(victim)},
		"DB$ Pump | ValidTgts$ Creature | NumAtt$ 2"); err != nil {
		t.Fatal(err)
	}
	sba(g)
	if got := g.Card(victim).Controller(); got != other {
		t.Errorf("controller = %v, want the owner %v once the target outpowers the host", got, other)
	}
}

// StaticCommandCheck without the SVar and comparison it reads is an error, and
// so is a comparison shorter than an operator and an operand.
func TestGainControlStaticCommandCheckNeedsItsParams(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"DB$ GainControl | ValidTgts$ Creature.OppCtrl | LoseControl$ StaticCommandCheck",
		"DB$ GainControl | ValidTgts$ Creature.OppCtrl | LoseControl$ StaticCommandCheck | StaticCommandCheckSVar$ Y | StaticCommandSVarCompare$ GT",
		"DB$ GainControl | ValidTgts$ Creature.OppCtrl | LoseControl$ StaticCommandCheck | StaticCommandCheckSVar$ Nope | StaticCommandSVarCompare$ GTX",
		"DB$ GainControl | ValidTgts$ Creature.OppCtrl | LoseControl$ UntilSourceUnattached",
	} {
		g, p, other := newTwoPlayerGame(t)
		victim := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Battlefield)
		if _, err := resolveNow(t, g, p, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(victim)},
			line, "X", "Count$CardPower", "Y", "Count$CardPower"); err == nil {
			t.Errorf("%q resolved, want an error", line)
		}
	}
}

// A StaticCommandCheck whose comparison never holds keeps the hold, and the
// entry dies with the host (a zone change builds a new Card in Java).
func TestGainControlStaticCommandCheckEntryDiesWithTheHost(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	victim := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Battlefield)
	host := stealUntil(t, g, p, victim,
		"DB$ GainControl | ValidTgts$ Creature.OppCtrl | LoseControl$ StaticCommandCheck | StaticCommandCheckSVar$ Y | StaticCommandSVarCompare$ GTX",
		"X", "Count$CardPower", "Y", "Count$CardPower")
	g.Move(host, engine.Graveyard, p)
	g.Move(host, engine.Battlefield, p)
	sba(g)
	// The host's entry is gone, so even a stronger target stays stolen: the
	// control change itself has no LeavesPlay token to end it.
	if got := g.Card(victim).Controller(); got != p {
		t.Errorf("controller = %v, want %v: the check list of the old object is gone", got, p)
	}
}
