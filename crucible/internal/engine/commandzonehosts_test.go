package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// planeWithStatic is a Plane whose one static is line.
func planeWithStatic(t *testing.T, line string) *compile.Card {
	t.Helper()
	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "test_plane"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Plane"
	raw.Faces[0].Type = cardtype.Parse(reg, "Plane")
	raw.Faces[0].Statics = []string{line}
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// A face-up plane is a Command-zone host (ADR-0056): Krosa's EffectZone$
// Command static pumps every creature, and the same card in the planar deck
// does nothing (StaticAbility.java:338-352).
func TestFaceUpPlaneStaticAppliesFromTheCommandZoneOnly(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		zone engine.ZoneType
		want int
	}{{engine.Command, 4}, {engine.PlanarDeck, 2}} {
		g, p, _ := newTwoPlayerGame(t)
		bear := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
		g.NewCard(corpusCard(t, "Krosa"), p, tc.zone)
		engine.CheckStateBasedActions(g, engine.NewScriptedController())
		if got, _ := g.Card(bear).Power(); got != tc.want {
			t.Errorf("Krosa in %v: power %d, want %d", tc.zone, got, tc.want)
		}
	}
}

// A plane static with no EffectZone$ functions only in play, as in Java (The
// Great Forest's script, card-script-defects.md).
func TestPlaneStaticWithoutEffectZoneDoesNotApply(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	bear := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	faceUpPlane(g, p, planeWithStatic(t, "Mode$ Continuous | Affected$ Creature | AddPower$ 5"))
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if got, _ := g.Card(bear).Power(); got != 2 {
		t.Errorf("power %d, want 2", got)
	}
}

// A Vanguard avatar's ActivationZone$ Command ability activates for its
// controller (SpellAbilityRestriction.java:99-100): Chronatog Avatar draws
// three.
func TestVanguardAvatarActivatesFromTheCommandZone(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	for range 3 {
		g.NewCard(nil, p, engine.Library)
	}
	avatar := g.NewCard(corpusCard(t, "Chronatog Avatar"), p, engine.Command)
	if g.ActivateAbility(other, avatar, 0, engine.NewScriptedController()) {
		t.Fatal("the opponent activated p's avatar")
	}
	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, avatar, 0, c) {
		t.Fatal("ActivateAbility from the Command zone failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Zone(engine.Hand, p).Len(); got != 3 {
		t.Errorf("hand = %d, want 3", got)
	}
}

// A face-up plane's upkeep trigger fires once: the plane is both a trait host
// and a Command-zone card the phase walk reads.
func TestFaceUpPlaneUpkeepTriggerFiresOnce(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	for range 4 {
		g.NewCard(nil, p, engine.Library)
		g.NewCard(nil, other, engine.Library)
	}
	faceUpPlane(g, p, planarDef(t, "Test Upkeep Plane", "Plane",
		[]string{"Mode$ Phase | Phase$ Upkeep | ValidPlayer$ You | TriggerZones$ Command | Execute$ TrigGain"}, nil,
		"TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 1"))
	g.SetTurnState(1, other, engine.Main2)
	c := engine.NewScriptedController()
	advanceUntil(t, g, c, func() bool { return g.ActivePlayer() == p && g.ActivePhase() == engine.Draw })
	if got := g.Player(p).Life; got != 21 {
		t.Errorf("life = %d, want 21", got)
	}
}
