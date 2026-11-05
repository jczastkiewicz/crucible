package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
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

// SeatAvatar (Player.java:2978-2985, RegisteredPlayer.java:207-213): Urza,
// -1/+10, sits in the Command zone and its owner starts at 30 life with a
// six-card opening hand.
func TestSeatedAvatarChangesStartingLifeAndHand(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	for range 10 {
		g.NewCard(nil, p, engine.Library)
		g.NewCard(nil, other, engine.Library)
	}
	avatar := g.SeatAvatar(p, corpusCard(t, "Urza"))
	if got := g.Card(avatar).Zone; got != engine.Command {
		t.Errorf("avatar in %v, want the Command zone", got)
	}
	if pl := g.Player(p); pl.Life != 30 || pl.StartingLife != 30 || pl.StartingHandSize != 6 {
		t.Errorf("life %d, starting life %d, starting hand %d; want 30, 30, 6", pl.Life, pl.StartingLife, pl.StartingHandSize)
	}
	c := engine.NewScriptedController()
	c.QueueStartingPlayer(p)
	engine.DealOpeningHands(g, c)
	if got := g.Zone(engine.Hand, p).Len(); got != 6 {
		t.Errorf("p's opening hand = %d, want 6", got)
	}
	if got := g.Zone(engine.Hand, other).Len(); got != 7 {
		t.Errorf("other's opening hand = %d, want 7", got)
	}
}

// A Subgame seats a fresh copy of each registered avatar
// (SubgameEffect.java:63-68) and starts at the seat's registered life and
// hand, the deltas already in them.
func TestSubgameSeatsTheRegisteredAvatar(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.SeatAvatar(p, corpusCard(t, "Urza"))
	subgameLands(t, g, p, 2)
	subgameLands(t, g, other, 2)
	obs := &avatarObserver{ScriptedController: subgameController(p)}
	g.Player(p).ManaPool.Add(mana.Green, 1)
	host := g.NewCard(etbChainDef(t, "Test Shape", "DB$ Subgame | RememberPlayers$ Win"), p, engine.Hand)
	if !g.CastSpell(p, host, obs) {
		t.Fatal("cast failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), obs); err != nil {
		t.Fatal(err)
	}
	if obs.avatars != 1 || obs.life != 30 || obs.hand != 6 {
		t.Errorf("subgame avatars %d, life %d, starting hand %d; want 1, 30, 6", obs.avatars, obs.life, obs.hand)
	}
}

// avatarObserver records the subgame's first seat when it asks who starts.
type avatarObserver struct {
	*engine.ScriptedController
	avatars, life, hand int
}

func (o *avatarObserver) ChooseStartingPlayer(g *engine.Game, decider engine.PlayerID, first bool) engine.PlayerID {
	pid := g.Players()[0]
	o.avatars = g.Zone(engine.Command, pid).Len()
	o.life, o.hand = g.Player(pid).Life, g.Player(pid).StartingHandSize
	return o.ScriptedController.ChooseStartingPlayer(g, decider, first)
}
