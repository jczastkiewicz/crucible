package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// The per-turn flags a draw, discard, mill and surveil leave on the card
// they moved (CardProperty.java: DrawnThisTurn, DiscardedThisTurn,
// milledThisTurn, surveilledThisTurn), and the crew records
// (CrewedThisTurn, CrewedBySourceThisTurn).
func TestTurnFlagPropertiesReadWhatMovedTheCard(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	for range 4 {
		g.NewCard(corpusCard(t, "Island"), p, engine.Library)
	}
	kept := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
	is := func(spec string, id engine.CardID) bool {
		t.Helper()
		got := engine.Matches(g, g.Card(id), valid.Parse(spec), p, engine.NoCard)
		if err := g.TakePendingError(); err != nil {
			t.Fatalf("%s: %v", spec, err)
		}
		return got
	}

	drawn := g.Zone(engine.Library, p).Cards()[0]
	g.DrawCards(p, 1, engine.NewScriptedController())
	if !is("Card.DrawnThisTurn", drawn) || is("Card.DrawnThisTurn", kept) {
		t.Errorf("DrawnThisTurn: drawn %v, kept %v; want true, false", is("Card.DrawnThisTurn", drawn), is("Card.DrawnThisTurn", kept))
	}

	dc := engine.NewScriptedController()
	dc.QueueCardOrder([]engine.CardID{kept, drawn})
	if _, err := resolveNow(t, g, p, dc, nil, "DB$ Discard | Defined$ You | Mode$ Hand"); err != nil {
		t.Fatal(err)
	}
	if !is("Card.DiscardedThisTurn", kept) || is("Card.milledThisTurn", kept) {
		t.Error("the discarded Bears: want DiscardedThisTurn and not milledThisTurn")
	}

	milled := g.Zone(engine.Library, p).Cards()[0]
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, "DB$ Mill | Defined$ You | NumCards$ 1"); err != nil {
		t.Fatal(err)
	}
	if !is("Card.milledThisTurn", milled) || is("Card.DiscardedThisTurn", milled) {
		t.Error("the milled Island: want milledThisTurn and not DiscardedThisTurn")
	}

	surveilled := g.Zone(engine.Library, p).Cards()[0]
	sc := engine.NewScriptedController()
	sc.QueueSurveil(nil, []engine.CardID{surveilled})
	if _, err := resolveNow(t, g, p, sc, nil, "DB$ Surveil | Defined$ You | Amount$ 1"); err != nil {
		t.Fatal(err)
	}
	if !is("Card.surveilledThisTurn", surveilled) || is("Card.surveilledThisTurn", milled) {
		t.Error("surveilledThisTurn: want the surveilled Island only")
	}

	vehicle := g.NewCard(scriptDef(t, "Test Vehicle", "Artifact Vehicle", "K:Crew:1"), p, engine.Battlefield)
	crewer := g.NewCard(corpusCard(t, "Hill Giant"), p, engine.Battlefield)
	idle := g.NewCard(corpusCard(t, "Gray Ogre"), p, engine.Battlefield)
	sba(g)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{crewer})
	activateOrFail(t, g, p, c, vehicle, 0)
	for _, tc := range []struct {
		spec         string
		card, source engine.CardID
		want         bool
	}{
		{"Creature.CrewedThisTurn", crewer, vehicle, true},
		{"Creature.CrewedThisTurn", idle, vehicle, false},
		{"Card.CrewedBySourceThisTurn", vehicle, crewer, true},
		{"Card.CrewedBySourceThisTurn", vehicle, idle, false},
	} {
		if got := engine.Matches(g, g.Card(tc.card), valid.Parse(tc.spec), p, tc.source); got != tc.want {
			t.Errorf("%s on %s = %v, want %v", tc.spec, g.Card(tc.card).Name(), got, tc.want)
		}
	}
}

// startedTheTurnUntapped is noted as each turn begins, before the untap
// step; the cleanup ends DrawnThisTurn for a card still in hand
// (PhaseHandler.java:855-857, Player.java:2493-2495).
func TestStartedTheTurnUntappedAndDrawnEndWithTheTurn(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	for range 6 {
		g.NewCard(corpusCard(t, "Island"), p, engine.Library)
		g.NewCard(corpusCard(t, "Island"), other, engine.Library)
	}
	tapped := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	untapped := g.NewCard(corpusCard(t, "Gray Ogre"), p, engine.Battlefield)
	g.Card(tapped).Tapped = true
	drawn := g.Zone(engine.Library, p).Cards()[0]
	g.DrawCards(p, 1, engine.NewScriptedController())
	c := engine.NewScriptedController()
	advanceUntil(t, g, c, func() bool { return g.Turn() == 2 && g.ActivePhase() == engine.Main1 })
	started := func(id engine.CardID) bool {
		return engine.Matches(g, g.Card(id), valid.Parse("Creature.startedTheTurnUntapped"), p, engine.NoCard)
	}
	if started(tapped) || !started(untapped) {
		t.Errorf("startedTheTurnUntapped: tapped %v, untapped %v; want false, true", started(tapped), started(untapped))
	}
	if engine.Matches(g, g.Card(drawn), valid.Parse("Card.DrawnThisTurn"), p, engine.NoCard) {
		t.Error("DrawnThisTurn still true on the next turn")
	}
}

// A creature's combat damage to a player shows in its damage history: this
// combat, this turn and for the object's life (CardProperty.java:1140-1184).
func TestDamageHistoryPropertiesReadTheCombatDamageDealt(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	attacker := g.NewCard(corpusCard(t, "Hill Giant"), p, engine.Battlefield)
	idle := g.NewCard(corpusCard(t, "Gray Ogre"), p, engine.Battlefield)
	declareLoneAttacker(t, g, attacker)
	declareBlockers(t, g, engine.NewScriptedController())
	g.DealCombatDamage(engine.NewScriptedController())
	if g.Player(other).Life != 17 {
		t.Fatalf("defender at %d life, want 17", g.Player(other).Life)
	}
	for _, tc := range []struct {
		spec string
		card engine.CardID
		want bool
	}{
		{"Creature.dealtDamagetoAny", attacker, true},
		{"Creature.dealtCombatDamagetoAny", attacker, true},
		{"Creature.dealtCombatDamageThisCombat", attacker, true},
		{"Creature.dealtDamageToOppThisTurn", attacker, true},
		{"Creature.dealtCombatDamageThisTurn Player", attacker, true},
		{"Creature.dealtCombatDamageThisTurn Creature", attacker, false},
		{"Creature.dealtDamagetoAny", idle, false},
		{"Creature.dealtCombatDamageThisCombat", idle, false},
		{"Creature.dealtDamageToOppThisTurn", idle, false},
	} {
		if got := engine.Matches(g, g.Card(tc.card), valid.Parse(tc.spec), p, engine.NoCard); got != tc.want {
			t.Errorf("%s on %s = %v, want %v", tc.spec, g.Card(tc.card).Name(), got, tc.want)
		}
		if err := g.TakePendingError(); err != nil {
			t.Errorf("%s: %v", tc.spec, err)
		}
	}
	advanceUntil(t, g, engine.NewScriptedController(), func() bool { return g.ActivePhase() == engine.Main2 })
	if engine.Matches(g, g.Card(attacker), valid.Parse("Creature.dealtCombatDamageThisCombat"), p, engine.NoCard) {
		t.Error("dealtCombatDamageThisCombat after combat ended")
	}
	if !engine.Matches(g, g.Card(attacker), valid.Parse("Creature.dealtCombatDamagetoAny"), p, engine.NoCard) {
		t.Error("dealtCombatDamagetoAny lost after combat ended")
	}
}
