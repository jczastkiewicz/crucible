package engine_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Unyaro, the real card: "At the beginning of your end step, if you planeswalked
// to Unyaro this turn, untap all creatures. They phase out until a player
// planeswalks." Its CheckSVar$ X reads PlaneswalkedToThisTurn Unyaro, so the
// trigger fires only on the turn its controller walked there.
func TestUnyaroUntapsAndPhasesOutCreaturesAfterWalkingThere(t *testing.T) {
	t.Parallel()

	for _, walked := range []bool{true, false} {
		g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
		c := engine.NewScriptedController()
		if walked {
			planarDeck(g, p, corpusCard(t, "Unyaro"))
		} else {
			// Face up from the start of the game, never walked to this turn.
			faceUpPlane(g, p, corpusCard(t, "Unyaro"))
			planarDeck(g, p, plainPlane(t, "Elsewhere"))
		}
		g.SetTurnState(1, p, engine.Main2)
		ours := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
		theirs := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Battlefield)
		g.Card(ours).Tapped, g.Card(theirs).Tapped = true, true
		if walked {
			resolveLine(t, g, p, c, "DB$ Planeswalk")
		}
		g.AdvancePhase(c) // End of Turn
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("walked=%v: ResolveStack: %v", walked, err)
		}
		bf := func(id engine.CardID) bool {
			return slices.Contains(g.Zone(engine.Battlefield, g.Card(id).Controller()).Cards(), id)
		}
		if got := !bf(ours) && !bf(theirs); got != walked {
			t.Errorf("walked=%v: creatures phased out = %v", walked, got)
		}
		if got := !g.Card(ours).Tapped && !g.Card(theirs).Tapped; got != walked {
			t.Errorf("walked=%v: creatures untapped = %v", walked, got)
		}
	}
}

// "They phase out until a player planeswalks": the next planeswalk by anyone
// phases the creatures back in (the effect's PlaneswalkedTo trigger).
func TestUnyaroCreaturesPhaseBackInWhenAPlayerPlaneswalks(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	c := engine.NewScriptedController()
	planarDeck(g, p, corpusCard(t, "Unyaro"), plainPlane(t, "Elsewhere"))
	g.SetTurnState(1, p, engine.Main2)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	resolveLine(t, g, p, c, "DB$ Planeswalk")
	g.AdvancePhase(c)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if slices.Contains(g.Zone(engine.Battlefield, p).Cards(), bears) {
		t.Fatal("the creature did not phase out")
	}
	g.SetTurnState(2, p, engine.Main1)
	resolveLine(t, g, p, c, "DB$ Planeswalk")
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack after the second planeswalk: %v", err)
	}
	if !slices.Contains(g.Zone(engine.Battlefield, p).Cards(), bears) {
		t.Error("the creature stayed phased out after a planeswalk")
	}
}

// "Whenever chaos ensues, create two 2/2 white and blue Knight creature tokens
// with vigilance."
func TestUnyaroChaosCreatesTwoKnights(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	c := engine.NewScriptedController()
	faceUpPlane(g, p, corpusCard(t, "Unyaro"))
	planarDeck(g, p, plainPlane(t, "Elsewhere")) // seating a planar deck makes it a Planechase game
	g.SetTurnState(1, p, engine.Main1)
	if err := resolveChaos(t, g, p, c, nil, "DB$ ChaosEnsues"); err != nil {
		t.Fatalf("ChaosEnsues: %v", err)
	}
	knights := 0
	var names []string
	for _, id := range g.Zone(engine.Battlefield, p).Cards() {
		names = append(names, g.Card(id).Def.Name)
		if strings.Contains(g.Card(id).Def.Name, "Knight") {
			knights++
		}
	}
	if knights != 2 {
		t.Errorf("chaos created %d Knights, want 2 (battlefield %v, active plane %v)", knights, names, g.ActivePlane())
	}
}
