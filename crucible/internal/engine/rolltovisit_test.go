package engine_test

import (
	"slices"
	"strconv"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// A RolledDie trigger naming RolledToVisitAttractions$ (Ferris Wheel's comeback)
// fires for a roll made with ToVisitYourAttractions$ and for no other roll.
func TestRolledToVisitAttractionsTriggerFiresOnlyForTheVisitRoll(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, roll string
		want       int
	}{
		{"visit roll", "DB$ RollDice | Sides$ 6 | ToVisitYourAttractions$ True", 21},
		{"ordinary roll", "DB$ RollDice | Sides$ 6", 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			g.NewCard(planarDef(t, "Watcher", "Artifact", []string{
				"Mode$ RolledDie | RolledToVisitAttractions$ True | ValidPlayer$ You | TriggerZones$ Battlefield | Execute$ Gain",
			}, nil, "Gain", "DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
			resolveLine(t, g, p, engine.NewScriptedController(), tc.roll)
			if got := g.Player(p).Life; got != tc.want {
				t.Errorf("life = %d, want %d", got, tc.want)
			}
		})
	}
}

// attractionDef is an Attraction lit on lights (a Lights: line) whose Visit
// ability gains its controller 1 life.
func attractionDef(t *testing.T, name, lights string) *compile.Card {
	t.Helper()
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Artifact Attraction")
	raw.Faces[0].AttractionLights = lights
	raw.Faces[0].Keywords = []string{"Visit:Gain"}
	raw.Faces[0].SVars.Set("Gain", "DB$ GainLife | Defined$ You | LifeAmount$ 1")
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// CR 703.4g: the active player's precombat main phase starts with a d6 roll
// when they control an Attraction, and each Attraction whose lights show the
// result is visited; with no Attraction nothing is rolled.
func TestMainOneRollsToVisitTheActivePlayersAttractions(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, lights string
		attraction   bool
		life, rolls  int
	}{
		{"every light lit", "1 2 3 4 5 6", true, 21, 1},
		{"no light lit", "", true, 20, 1},
		{"no Attraction", "", false, 20, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, other := newTwoPlayerGame(t)
			if tc.attraction {
				g.NewCard(attractionDef(t, "Test Ride", tc.lights), p, engine.Battlefield)
			}
			// The opponent's Attraction is not theirs to visit on p's turn.
			g.NewCard(attractionDef(t, "Their Ride", "1 2 3 4 5 6"), other, engine.Battlefield)
			c := engine.NewScriptedController()
			g.SetTurnState(2, p, engine.Draw)
			g.AdvancePhase(c)
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := g.ActivePhase(); got != engine.Main1 {
				t.Fatalf("phase = %v, want Main1", got)
			}
			if got := g.Player(p).Life; got != tc.life {
				t.Errorf("life = %d, want %d", got, tc.life)
			}
			if got := g.Player(p).RollsThisTurn; got != tc.rolls {
				t.Errorf("RollsThisTurn = %d, want %d", got, tc.rolls)
			}
			if got := g.Player(other).Life; got != 20 {
				t.Errorf("opponent life = %d, want 20", got)
			}
		})
	}
}

// RollDice ToVisitYourAttractions$ visits with the total (Line Cutter). A
// second visit in one turn fires the Visit trigger and an "whenever you visit
// an Attraction" watcher again (Motion Sickness's shape), but
// AttractionsVisitedThisTurn counts the Attraction once; cleanup zeroes it.
func TestAnAttractionVisitedTwiceInATurnCountsOnce(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(attractionDef(t, "Test Ride", "1 2 3 4 5 6"), p, engine.Battlefield)
	g.NewCard(planarDef(t, "Watcher", "Artifact", []string{
		"Mode$ VisitAttraction | TriggerZones$ Battlefield | ValidCard$ Attraction.YouCtrl | ValidPlayer$ You | Execute$ Gain",
	}, nil, "Gain", "DB$ GainLife | Defined$ You | LifeAmount$ 10"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	roll := "DB$ RollDice | Sides$ 6 | ToVisitYourAttractions$ True"
	resolveLine(t, g, p, c, roll)
	resolveLine(t, g, p, c, roll)
	if got := g.Player(p).Life; got != 42 {
		t.Errorf("life = %d, want 42 (two visits, 1 + 10 each)", got)
	}
	count := "DB$ GainLife | Defined$ You | LifeAmount$ X"
	resolveLine(t, g, p, c, count, "X", "PlayerCountPropertyYou$AttractionsVisitedThisTurn")
	if got := g.Player(p).Life; got != 43 {
		t.Errorf("life = %d, want 43 (one Attraction visited)", got)
	}
	g.SetTurnState(1, p, engine.EndOfTurn)
	g.AdvancePhase(c)
	g.AdvancePhase(c) // leaves Cleanup: Game.onCleanupPhase resets the turn
	g.SetTurnState(2, p, engine.Main1)
	resolveLine(t, g, p, c, count, "X", "PlayerCountPropertyYou$AttractionsVisitedThisTurn")
	if got := g.Player(p).Life; got != 43 {
		t.Errorf("life = %d, want 43 (cleanup zeroed the count)", got)
	}
}

// Player.initVariantsZones seats the Attractions and Contraptions sections
// as their decks, each shuffled with the game's generator; an empty section
// draws nothing from it.
func TestSeatedAttractionAndContraptionDecksAreShuffled(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		zone engine.ZoneType
		seat func(*engine.Game, engine.PlayerID, []*compile.Card)
	}{
		{engine.AttractionDeck, (*engine.Game).SeatAttractionDeck},
		{engine.ContraptionDeck, (*engine.Game).SeatContraptionDeck},
	} {
		t.Run(tc.zone.String(), func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			var defs []*compile.Card
			var names []string
			for i := range 10 {
				name := "Ride " + strconv.Itoa(i)
				defs = append(defs, attractionDef(t, name, ""))
				names = append(names, name)
			}
			tc.seat(g, p, defs)
			var got []string
			for _, id := range g.Zone(tc.zone, p).Cards() {
				got = append(got, g.Card(id).Name())
			}
			if len(got) != len(names) || slices.Equal(got, names) {
				t.Errorf("%v = %v, want the ten cards shuffled", tc.zone, got)
			}
			sorted := slices.Clone(got)
			slices.Sort(sorted)
			if !slices.Equal(sorted, names) {
				t.Errorf("%v holds %v, want %v", tc.zone, sorted, names)
			}
		})
	}
}
