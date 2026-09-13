package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// attachmentTypeRegistry is the minimal type vocabulary a test needs to build
// a cardtype.Line: LoadRegistry only insists on a CreatureTypes section
// existing at all (its own sanity check against a wrong file), and Parse
// treats any other word -- Aura, Equipment, Fortification included -- as a
// subtype whether or not the registry names it (cardtype.Parse's own doc
// comment), so nothing else needs to be here.
func attachmentTypeRegistry(t *testing.T) *cardtype.Registry {
	t.Helper()
	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	return reg
}

// auraDef and permanentDef build just enough of a *compile.Card for
// Card.Type() to answer "is this an Aura" -- the only thing
// cleanupDanglingAttachments reads off a card's definition.
func auraDef(t *testing.T) *compile.Card {
	t.Helper()
	def := &compile.Card{Name: "Test Aura"}
	def.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Enchantment Aura")
	return def
}

func equipmentDef(t *testing.T) *compile.Card {
	t.Helper()
	def := &compile.Card{Name: "Test Equipment"}
	def.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Artifact Equipment")
	return def
}

func creatureDef(t *testing.T) *compile.Card {
	t.Helper()
	def := &compile.Card{Name: "Test Creature"}
	def.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Creature Elf")
	return def
}

// CR 704.5a: a player at zero life loses. The other player, now the only one
// left standing, wins and the game ends (CR 104.2a).
func TestCheckStateBasedActionsLifeAtZero(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life = 0
	g.Player(b).Life = 20

	if !engine.CheckStateBasedActions(g) {
		t.Fatal("game did not end")
	}
	if !g.Player(a).Lost {
		t.Error("player at 0 life did not lose")
	}
	if !g.Player(b).Won {
		t.Error("the only player left did not win")
	}
	if !g.Over() {
		t.Error("Over() does not reflect the game ending")
	}
}

// Negative life loses too -- the check is <= 0, not == 0.
func TestCheckStateBasedActionsNegativeLife(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = -3, 20

	engine.CheckStateBasedActions(g)
	if !g.Player(a).Lost {
		t.Error("player at negative life did not lose")
	}
}

func TestCheckStateBasedActionsPositiveLifeSurvives(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 1, 20

	if engine.CheckStateBasedActions(g) {
		t.Error("game ended with both players above the loss threshold")
	}
	if g.Player(a).Lost {
		t.Error("player at 1 life lost")
	}
}

// CR 704.5c: ten or more poison counters loses. Nine does not.
func TestCheckStateBasedActionsPoison(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	g.Player(a).Counters.Add(engine.Poison, 9)

	if engine.CheckStateBasedActions(g) {
		t.Fatal("game ended at nine poison counters")
	}

	g.Player(a).Counters.Add(engine.Poison, 1)
	if !engine.CheckStateBasedActions(g) {
		t.Fatal("game did not end at ten poison counters")
	}
	if !g.Player(a).Lost {
		t.Error("player with ten poison counters did not lose")
	}
}

// Both players losing in the same check is a draw: nobody is left standing,
// so nobody wins, but the game still ends.
func TestCheckStateBasedActionsDraw(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 0, 0

	if !engine.CheckStateBasedActions(g) {
		t.Fatal("game did not end")
	}
	if g.Player(a).Won || g.Player(b).Won {
		t.Error("a draw declared a winner")
	}
	if !g.Player(a).Lost || !g.Player(b).Lost {
		t.Error("a draw did not record both players as having lost")
	}
}

// A game past three or more players in contention does not end.
func TestCheckStateBasedActionsGameContinues(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b", "c")
	for _, id := range g.Players() {
		g.Player(id).Life = 20
	}
	g.Player(g.Players()[0]).Life = 0

	if engine.CheckStateBasedActions(g) {
		t.Fatal("game ended with two players still standing")
	}
	if g.Over() {
		t.Error("Over() reported true while two players remain")
	}
}

// Once the game has ended, a later life change must not resurrect it: the
// check short-circuits rather than re-deriving from current life totals.
func TestCheckStateBasedActionsStaysOverOnceOver(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 0, 20
	engine.CheckStateBasedActions(g)
	if !g.Over() {
		t.Fatal("setup: game did not end")
	}
	if !g.Player(b).Won {
		t.Fatal("setup: player b did not win")
	}

	g.Player(b).Life = 0 // the winner takes damage after the game already ended
	engine.CheckStateBasedActions(g)
	if !g.Player(b).Won {
		t.Error("the recorded winner changed after the game had already ended")
	}
}

// A player who already lost is skipped on a later check, in a game that has
// not ended yet -- losing does not clear their counters or life, so without
// the skip a lingering 0 life would be re-evaluated every call for no reason.
func TestCheckStateBasedActionsSkipsAlreadyLostPlayers(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b", "c")
	for _, id := range g.Players() {
		g.Player(id).Life = 20
	}
	a := g.Players()[0]
	g.Player(a).Life = 0

	if engine.CheckStateBasedActions(g) {
		t.Fatal("game ended with two players still standing")
	}
	if !g.Player(a).Lost {
		t.Fatal("setup: player a did not lose")
	}

	// Second call: a is already Lost and must be skipped, not re-marked.
	if engine.CheckStateBasedActions(g) {
		t.Fatal("game ended on the second check with two players still standing")
	}
}

// CR 704.5q: N +1/+1 and N -1/-1 counters annihilate together, where N is
// the smaller pile.
func TestCheckStateBasedActionsAnnihilatesCounters(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	id := g.NewCard(nil, a, engine.Battlefield)
	c := g.Card(id)
	c.Counters.Add(engine.P1P1, 5)
	c.Counters.Add(engine.M1M1, 2)

	if engine.CheckStateBasedActions(g) {
		t.Fatal("game ended over a counter annihilation check")
	}
	if got := c.Counters.Count(engine.P1P1); got != 3 {
		t.Errorf("P1P1 %d, want 3 (5 - min(5,2))", got)
	}
	if got := c.Counters.Count(engine.M1M1); got != 0 {
		t.Errorf("M1M1 %d, want 0", got)
	}
}

// Only one kind present is untouched -- there is nothing to annihilate
// against.
func TestCheckStateBasedActionsOneKindOfCounterSurvives(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	id := g.NewCard(nil, a, engine.Battlefield)
	c := g.Card(id)
	c.Counters.Add(engine.P1P1, 4)

	engine.CheckStateBasedActions(g)
	if got := c.Counters.Count(engine.P1P1); got != 4 {
		t.Errorf("P1P1 %d, want 4 (untouched)", got)
	}
}

// A permanent off the battlefield does not annihilate -- the rule is about
// permanents, and a card in hand or the graveyard is not one.
func TestCheckStateBasedActionsCounterAnnihilationIsBattlefieldOnly(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	id := g.NewCard(nil, a, engine.Graveyard)
	c := g.Card(id)
	c.Counters.Add(engine.P1P1, 3)
	c.Counters.Add(engine.M1M1, 3)

	engine.CheckStateBasedActions(g)
	if got := c.Counters.Count(engine.P1P1); got != 3 {
		t.Errorf("P1P1 %d, want 3 (untouched off the battlefield)", got)
	}
}

// Once the game has ended, the counter loop must not run at all -- the same
// as Java's checkStateEffects returning before its creature loop once
// checkGameOverCondition finds the game over.
func TestCheckStateBasedActionsSkipsCounterCheckWhenGameOver(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 0, 20
	id := g.NewCard(nil, b, engine.Battlefield)
	c := g.Card(id)
	c.Counters.Add(engine.P1P1, 2)
	c.Counters.Add(engine.M1M1, 2)

	if !engine.CheckStateBasedActions(g) {
		t.Fatal("game did not end")
	}
	if got := c.Counters.Count(engine.P1P1); got != 2 {
		t.Errorf("P1P1 %d, want 2 (the counter loop must not have run)", got)
	}
}

// GameEnded fires exactly once, on the call that actually ends the game --
// SetOver is fixture loading's tool, the same relationship SetTurnState has
// to StartTurn/AdvancePhase: a state injection, not something real play
// calls. CheckStateBasedActions is the only thing that sets Over as a side
// effect of actually deciding a game has ended.
func TestSetOver(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	if g.Over() {
		t.Fatal("a fresh game reports Over")
	}
	g.SetOver(true)
	if !g.Over() {
		t.Error("SetOver(true) did not take")
	}
	g.SetOver(false)
	if g.Over() {
		t.Error("SetOver(false) did not take")
	}
}

// not on a later call finding it already over, which the top-of-function
// short circuit skips entirely.
func TestCheckStateBasedActionsEmitsGameEndedOnce(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 0, 20
	var sink recordingSink
	g.SetSink(&sink)

	engine.CheckStateBasedActions(g)
	engine.CheckStateBasedActions(g)
	engine.CheckStateBasedActions(g)

	var ended []engine.Event
	for _, e := range sink.events {
		if e.Kind == engine.GameEnded {
			ended = append(ended, e)
		}
	}
	if len(ended) != 1 {
		t.Fatalf("saw %d GameEnded events across three calls, want 1: %+v", len(ended), ended)
	}
	if ended[0].Actor != b {
		t.Errorf("GameEnded actor %v, want %v (the winner)", ended[0].Actor, b)
	}
}

// Clone must not let the clone's poison counters write back to the original
// -- the same sharing bug Counters, Memory and attachments were already
// guarded against.
func TestCloneCopiesPlayerCounters(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.Player(a).Counters.Add(engine.Poison, 3)

	c := g.Clone()
	c.Player(a).Counters.Add(engine.Poison, 5)

	if got := g.Player(a).Counters.Count(engine.Poison); got != 3 {
		t.Errorf("original's poison became %d after the clone's changed", got)
	}
}

// A synthetic card built with a nil Def -- every card the rest of this
// package builds -- reports the zero type line, which HasSubtype("Aura")
// correctly reads as "not an Aura": nothing else in this file has to worry
// about cleanupDanglingAttachments mistaking an ordinary test card for one.
func TestCardTypeNilDefIsEmpty(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	id := g.NewCard(nil, g.Players()[0], engine.Battlefield)
	if got := g.Card(id).Type(); got.HasSubtype("Aura") {
		t.Errorf("a card with no Def reported Aura: %+v", got)
	}
}

// CR 704.5f: an Aura whose host left the battlefield goes to its owner's
// graveyard.
func TestCheckStateBasedActionsAuraGoesToGraveyardWhenHostLeaves(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	host := g.NewCard(nil, a, engine.Battlefield)
	aura := g.NewCard(auraDef(t), a, engine.Battlefield)
	g.Attach(aura, host)

	g.Move(host, engine.Graveyard, a)
	engine.CheckStateBasedActions(g)

	if z := g.Card(aura).Zone; z != engine.Graveyard {
		t.Errorf("aura zone = %v, want Graveyard", z)
	}
	if _, attached := g.Card(aura).AttachedTo(); attached {
		t.Error("aura still reports an attachment after going to the graveyard")
	}
}

// CR 704.5f's other half: an Aura on the battlefield that was never attached
// at all is equally illegal.
func TestCheckStateBasedActionsUnattachedAuraGoesToGraveyard(t *testing.T) {
	t.Parallel()

	// Two players, both above the loss threshold: a single-player game
	// satisfies CR 104.2a's "one player left standing" trivially and would
	// end the game -- and return -- before ever reaching this check.
	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	aura := g.NewCard(auraDef(t), p, engine.Battlefield)

	engine.CheckStateBasedActions(g)

	if z := g.Card(aura).Zone; z != engine.Graveyard {
		t.Errorf("aura zone = %v, want Graveyard", z)
	}
}

// CR 704.5f goes to the owner's graveyard, not the controller's -- the two
// differ once anything takes control of the Aura.
func TestCheckStateBasedActionsAuraGoesToOwnersGraveyard(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	aura := g.NewCard(auraDef(t), a, engine.Battlefield)
	g.Card(aura).Controller = b // controlled by b, still owned by a

	engine.CheckStateBasedActions(g)

	if z, owner := g.Card(aura).Zone, g.Card(aura).ZoneOwner; z != engine.Graveyard || owner != a {
		t.Errorf("aura ended in %v/%d, want Graveyard/%d (the owner, not the controller)", z, owner, a)
	}
}

// A legally attached Aura is untouched.
func TestCheckStateBasedActionsLegalAuraSurvives(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	host := g.NewCard(nil, p, engine.Battlefield)
	aura := g.NewCard(auraDef(t), p, engine.Battlefield)
	g.Attach(aura, host)

	engine.CheckStateBasedActions(g)

	if z := g.Card(aura).Zone; z != engine.Battlefield {
		t.Errorf("legally attached aura zone = %v, want Battlefield", z)
	}
	if got, attached := g.Card(aura).AttachedTo(); !attached || got != host {
		t.Errorf("legally attached aura AttachedTo() = (%d, %v), want (%d, true)", got, attached, host)
	}
}

// CR 704.5m: an Equipment whose host left the battlefield becomes
// unattached, and stays on the battlefield -- unlike an Aura, it is not put
// anywhere.
func TestCheckStateBasedActionsEquipmentUnattachesWhenHostLeaves(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	host := g.NewCard(nil, p, engine.Battlefield)
	equipment := g.NewCard(equipmentDef(t), p, engine.Battlefield)
	g.Attach(equipment, host)

	g.Move(host, engine.Graveyard, p)
	engine.CheckStateBasedActions(g)

	if z := g.Card(equipment).Zone; z != engine.Battlefield {
		t.Errorf("equipment zone = %v, want Battlefield (704.5m does not move it)", z)
	}
	if _, attached := g.Card(equipment).AttachedTo(); attached {
		t.Error("equipment still reports an attachment after its host left")
	}
}

// An Equipment that was never attached is legal on its own -- unlike an
// Aura, sitting unattached on the battlefield is not itself illegal for it.
func TestCheckStateBasedActionsUnattachedEquipmentSurvives(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	equipment := g.NewCard(equipmentDef(t), p, engine.Battlefield)

	engine.CheckStateBasedActions(g)

	if z := g.Card(equipment).Zone; z != engine.Battlefield {
		t.Errorf("unattached equipment zone = %v, want Battlefield", z)
	}
}
