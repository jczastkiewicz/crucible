package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

func studyHost(t *testing.T, g *engine.Game, p engine.PlayerID, extra string) engine.CardID {
	t.Helper()
	line := "Event$ Draw | ActiveZones$ Battlefield | ValidPlayer$ You | Optional$ True | " + extra + "ReplaceWith$ Cnt | Description$ May."
	return g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Study", line, "Cnt",
		"DB$ PutCounter | Defined$ Self | CounterType$ STUDY | CounterNum$ 1"), p, engine.Battlefield)
}

// TestOptionalReplacementAsksTheNamedDecider proves an Optional$ replacement
// with OptionalDecider$ You asks that player (ConfirmReplacementEffect) and
// applies on a yes, and that an OptionalDecider$ the port cannot resolve
// records a pending error and leaves the draw as it was.
func TestOptionalReplacementAsksTheNamedDecider(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.NewCard(nil, p, engine.Library)
	host := studyHost(t, g, p, "OptionalDecider$ You | ")
	c := engine.NewScriptedController()
	c.QueueConfirmReplacementEffect(true)
	g.DrawCards(p, 1, c)
	if got := g.Card(host).Counters.Count("STUDY"); got != 1 {
		t.Errorf("study counters = %d, want 1 after confirming", got)
	}
	if got := len(g.Zone(engine.Hand, p).Cards()); got != 0 {
		t.Errorf("hand = %d, want 0: the draw was replaced", got)
	}

	g2 := newGame(t, "a")
	p2 := g2.Players()[0]
	g2.NewCard(nil, p2, engine.Library)
	studyHost(t, g2, p2, "OptionalDecider$ Bogus | ")
	g2.DrawCards(p2, 1, engine.NewScriptedController())
	if g2.TakePendingError() == nil {
		t.Error("an unresolvable OptionalDecider$ recorded no pending error")
	}
	if got := len(g2.Zone(engine.Hand, p2).Cards()); got != 1 {
		t.Errorf("hand = %d, want 1: the line did not apply", got)
	}
}

// TestOptionalReplacementWithoutAControllerDeclines proves a nil controller
// answers "no" to a "you may" replacement.
func TestOptionalReplacementWithoutAControllerDeclines(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.NewCard(nil, p, engine.Library)
	host := studyHost(t, g, p, "")
	g.DrawCards(p, 1, nil)
	if got := g.Card(host).Counters.Count("STUDY"); got != 0 {
		t.Errorf("study counters = %d, want 0", got)
	}
}

// TestDrawCardsReplacementNumberGate proves Number$ GE2 gates an Event$
// DrawCards line on the count, and a malformed Number$ records a pending error.
func TestDrawCardsReplacementNumberGate(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		number    string
		draw      int
		wantHand  int
		wantError bool
	}{
		"met":       {"GE2", 2, 4, false},
		"not met":   {"GE2", 1, 1, false},
		"malformed": {"GE", 2, 2, true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			g := newGame(t, "a")
			p := g.Players()[0]
			for range 6 {
				g.NewCard(nil, p, engine.Library)
			}
			g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Doubler",
				"Event$ DrawCards | ActiveZones$ Battlefield | ValidPlayer$ You | Number$ "+tc.number+" | ReplaceWith$ More",
				"More", "DB$ Draw | Defined$ You | NumCards$ ReplaceCount$Number/Twice"), p, engine.Battlefield)
			g.DrawCards(p, tc.draw, engine.NewScriptedController())
			if got := len(g.Zone(engine.Hand, p).Cards()); got != tc.wantHand {
				t.Errorf("hand = %d, want %d", got, tc.wantHand)
			}
			if err := g.TakePendingError(); (err != nil) != tc.wantError {
				t.Errorf("pending error = %v, want error %v", err, tc.wantError)
			}
		})
	}
}

// TestCounterReplacementValidCause proves counterReplaced reads ValidCause$
// against the countering ability's controller: ".OppCtrl" does not match the
// host controller's own counterspell (the spell is countered normally), and a
// form the port cannot read records a pending error and skips the line.
func TestCounterReplacementValidCause(t *testing.T) {
	t.Parallel()

	for name, cause := range map[string]string{
		"OppCtrl does not match": "SpellAbility.OppCtrl",
		"unreadable":             "Weird",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			g, p, other := newTwoPlayerGame(t)
			g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Guile",
				"Event$ Counter | ActiveZones$ Battlefield | ValidSA$ Spell | ValidCause$ "+cause+" | ReplaceWith$ Rm",
				"Rm", "DB$ ChangeZone | Defined$ ReplacedCard | Origin$ Stack | Destination$ Exile"), p, engine.Battlefield)
			spell := g.NewCard(creatureDefCost(t, "Spell", "G"), other, engine.Hand)
			g.SetTurnState(1, other, engine.Main1)
			g.Player(other).ManaPool.Add(mana.Green, 1)
			if !g.CastSpell(other, spell, engine.NewScriptedController()) {
				t.Fatal("cast failed")
			}
			def := etbChainDef(t, "Counterer", "DB$ Counter | TargetType$ Spell | ValidTgts$ Card")
			host := g.NewCard(def, p, engine.Battlefield)
			sub := def.Faces[0].Triggers[0].Subs[0].Ability
			g.PushAbility(engine.Ability{API: engine.APICounter, Source: host, Controller: p, Params: sub,
				Targets: []engine.EntityID{engine.CardEntity(spell)}})
			// A pending error surfaces as ResolveStack's own error (ADR-0020).
			if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); (err != nil) != (cause == "Weird") {
				t.Errorf("ResolveStack error = %v, want one only for the unreadable cause", err)
			}
			if z := g.Card(spell).Zone; z != engine.Graveyard {
				t.Errorf("spell zone = %v, want Graveyard: the replacement must not have applied", z)
			}
		})
	}
}

// TestDestroyReplacementUnreadableParamRecordsAnError proves destroyInstead
// skips a line naming a param it does not read and records the error rather
// than guessing.
func TestDestroyReplacementUnreadableParamRecordsAnError(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Egress",
		"Event$ Destroy | ActiveZones$ Battlefield | Weird$ True | ReplaceWith$ Rm", "Rm", "DB$ Sacrifice"), p, engine.Battlefield)
	victim := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	def := etbChainDef(t, "Destroyer", "DB$ Destroy | Defined$ Targeted | ValidTgts$ Creature")
	host := g.NewCard(def, p, engine.Battlefield)
	sub := def.Faces[0].Triggers[0].Subs[0].Ability
	g.PushAbility(engine.Ability{API: engine.APIDestroy, Source: host, Controller: p, Params: sub,
		Targets: []engine.EntityID{engine.CardEntity(victim)}})
	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err == nil {
		t.Error("no pending error for the unreadable param")
	}
	if g.Card(victim).Zone != engine.Graveyard {
		t.Errorf("victim zone = %v, want Graveyard", g.Card(victim).Zone)
	}
}
