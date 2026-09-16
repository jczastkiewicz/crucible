package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
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
	return creatureDefPT(t, "2", "2")
}

func creatureDefPT(t *testing.T, power, toughness string) *compile.Card {
	t.Helper()
	def := &compile.Card{Name: "Test Creature"}
	def.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Creature Elf")
	def.Faces[0].Power, def.Faces[0].Toughness = power, toughness
	return def
}

func indestructibleCreatureDefPT(t *testing.T, power, toughness string) *compile.Card {
	t.Helper()
	def := creatureDefPT(t, power, toughness)
	def.Faces[0].Keywords = []string{"Indestructible"}
	return def
}

func creatureDefPTKeywords(t *testing.T, power, toughness string, keywords ...string) *compile.Card {
	t.Helper()
	def := creatureDefPT(t, power, toughness)
	def.Faces[0].Keywords = keywords
	return def
}

func creatureDefManaCost(t *testing.T, cost string) *compile.Card {
	t.Helper()
	def := creatureDefPT(t, "1", "1")
	def.Faces[0].ManaCost = mana.MustParse(cost)
	return def
}

func planeswalkerDefLoyalty(t *testing.T, loyalty string) *compile.Card {
	t.Helper()
	def := &compile.Card{Name: "Test Planeswalker"}
	def.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Legendary Planeswalker Test")
	def.Faces[0].Loyalty = loyalty
	return def
}

func battleDefDefense(t *testing.T, defense string) *compile.Card {
	t.Helper()
	def := &compile.Card{Name: "Test Battle"}
	def.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Battle Siege")
	def.Faces[0].Defense = defense
	return def
}

func legendaryCreatureDef(t *testing.T, name string) *compile.Card {
	t.Helper()
	def := &compile.Card{Name: name}
	def.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Legendary Creature Elf")
	def.Faces[0].Power, def.Faces[0].Toughness = "2", "2"
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

	if !engine.CheckStateBasedActions(g, engine.NewScriptedController()) {
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

	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if !g.Player(a).Lost {
		t.Error("player at negative life did not lose")
	}
}

func TestCheckStateBasedActionsPositiveLifeSurvives(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 1, 20

	if engine.CheckStateBasedActions(g, engine.NewScriptedController()) {
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

	if engine.CheckStateBasedActions(g, engine.NewScriptedController()) {
		t.Fatal("game ended at nine poison counters")
	}

	g.Player(a).Counters.Add(engine.Poison, 1)
	if !engine.CheckStateBasedActions(g, engine.NewScriptedController()) {
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

	if !engine.CheckStateBasedActions(g, engine.NewScriptedController()) {
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

	if engine.CheckStateBasedActions(g, engine.NewScriptedController()) {
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
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if !g.Over() {
		t.Fatal("setup: game did not end")
	}
	if !g.Player(b).Won {
		t.Fatal("setup: player b did not win")
	}

	g.Player(b).Life = 0 // the winner takes damage after the game already ended
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
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

	if engine.CheckStateBasedActions(g, engine.NewScriptedController()) {
		t.Fatal("game ended with two players still standing")
	}
	if !g.Player(a).Lost {
		t.Fatal("setup: player a did not lose")
	}

	// Second call: a is already Lost and must be skipped, not re-marked.
	if engine.CheckStateBasedActions(g, engine.NewScriptedController()) {
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

	if engine.CheckStateBasedActions(g, engine.NewScriptedController()) {
		t.Fatal("game ended over a counter annihilation check")
	}
	if got := c.Counters.Count(engine.P1P1); got != 3 {
		t.Errorf("P1P1 %d, want 3 (5 - min(5,2))", got)
	}
	if got := c.Counters.Count(engine.M1M1); got != 0 {
		t.Errorf("M1M1 %d, want 0", got)
	}
}

// Annihilation emits CounterChanged for both piles it shrinks, sourced from
// the card itself -- the rule is self-inflicted, no other card causes it.
func TestCheckStateBasedActionsAnnihilationEmitsCounterChanged(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	id := g.NewCard(nil, a, engine.Battlefield)
	g.Card(id).Counters.Add(engine.P1P1, 5)
	g.Card(id).Counters.Add(engine.M1M1, 2)
	var sink recordingSink
	g.SetSink(&sink)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	var p1p1, m1m1 *engine.Event
	for i := range sink.events {
		e := &sink.events[i]
		if e.Kind != engine.CounterChanged {
			continue
		}
		switch engine.CounterDetail(e.Detail) {
		case engine.CounterDetailP1P1:
			p1p1 = e
		case engine.CounterDetailM1M1:
			m1m1 = e
		}
	}
	if p1p1 == nil || m1m1 == nil {
		t.Fatalf("CounterChanged for both P1P1 and M1M1 among %d events, got p1p1=%v m1m1=%v", len(sink.events), p1p1, m1m1)
	}
	if p1p1.Source != id || p1p1.Target != engine.CardEntity(id) || p1p1.Amount != -2 {
		t.Errorf("P1P1 event = %+v, want source/target %v, amount -2", *p1p1, id)
	}
	if m1m1.Source != id || m1m1.Target != engine.CardEntity(id) || m1m1.Amount != -2 {
		t.Errorf("M1M1 event = %+v, want source/target %v, amount -2", *m1m1, id)
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

	engine.CheckStateBasedActions(g, engine.NewScriptedController())
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

	engine.CheckStateBasedActions(g, engine.NewScriptedController())
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

	if !engine.CheckStateBasedActions(g, engine.NewScriptedController()) {
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

	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	engine.CheckStateBasedActions(g, engine.NewScriptedController())

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

// CR 704.5: an Aura whose host left the battlefield goes to its owner's
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
	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(aura).Zone; z != engine.Graveyard {
		t.Errorf("aura zone = %v, want Graveyard", z)
	}
	if _, attached := g.Card(aura).AttachedTo(); attached {
		t.Error("aura still reports an attachment after going to the graveyard")
	}
}

// The other half of the same rule: an Aura on the battlefield that was
// never attached at all is equally illegal.
func TestCheckStateBasedActionsUnattachedAuraGoesToGraveyard(t *testing.T) {
	t.Parallel()

	// Two players, both above the loss threshold: a single-player game
	// satisfies CR 104.2a's "one player left standing" trivially and would
	// end the game -- and return -- before ever reaching this check.
	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	aura := g.NewCard(auraDef(t), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(aura).Zone; z != engine.Graveyard {
		t.Errorf("aura zone = %v, want Graveyard", z)
	}
}

// It goes to the owner's graveyard, not the controller's -- the two differ
// once anything takes control of the Aura.
func TestCheckStateBasedActionsAuraGoesToOwnersGraveyard(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	aura := g.NewCard(auraDef(t), a, engine.Battlefield)
	g.Card(aura).Controller = b // controlled by b, still owned by a

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

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

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(aura).Zone; z != engine.Battlefield {
		t.Errorf("legally attached aura zone = %v, want Battlefield", z)
	}
	if got, attached := g.Card(aura).AttachedTo(); !attached || got != host {
		t.Errorf("legally attached aura AttachedTo() = (%d, %v), want (%d, true)", got, attached, host)
	}
}

// CR 704.5's other case: an Equipment whose host left the battlefield
// becomes unattached, and stays on the battlefield -- unlike an Aura, it
// is not put anywhere.
func TestCheckStateBasedActionsEquipmentUnattachesWhenHostLeaves(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	host := g.NewCard(nil, p, engine.Battlefield)
	equipment := g.NewCard(equipmentDef(t), p, engine.Battlefield)
	g.Attach(equipment, host)

	g.Move(host, engine.Graveyard, p)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(equipment).Zone; z != engine.Battlefield {
		t.Errorf("equipment zone = %v, want Battlefield (unattaching does not move it)", z)
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

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(equipment).Zone; z != engine.Battlefield {
		t.Errorf("unattached equipment zone = %v, want Battlefield", z)
	}
}

// BasePower/BaseToughness resolve a plain printed integer, and report false
// -- not zero, not a panic -- for anything they cannot: "*", a Count$
// reference, or a nil Def.
func TestBasePowerToughness(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]

	plain := g.NewCard(creatureDefPT(t, "3", "4"), p, engine.Battlefield)
	if pw, ok := g.Card(plain).BasePower(); !ok || pw != 3 {
		t.Errorf("BasePower() = (%d, %v), want (3, true)", pw, ok)
	}
	if tg, ok := g.Card(plain).BaseToughness(); !ok || tg != 4 {
		t.Errorf("BaseToughness() = (%d, %v), want (4, true)", tg, ok)
	}

	star := g.NewCard(creatureDefPT(t, "*", "1+*"), p, engine.Battlefield)
	if _, ok := g.Card(star).BasePower(); ok {
		t.Error("BasePower() resolved a \"*\" power")
	}
	if _, ok := g.Card(star).BaseToughness(); ok {
		t.Error("BaseToughness() resolved a \"1+*\" toughness")
	}

	noDef := g.NewCard(nil, p, engine.Battlefield)
	if _, ok := g.Card(noDef).BasePower(); ok {
		t.Error("BasePower() resolved a card with no Def")
	}
	if _, ok := g.Card(noDef).BaseToughness(); ok {
		t.Error("BaseToughness() resolved a card with no Def")
	}
}

// CR 704.5f (GameAction.java's own comment): a creature with printed
// toughness zero or less goes to its owner's graveyard.
func TestCheckStateBasedActionsLethalToughnessDies(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	dead := g.NewCard(creatureDefPT(t, "2", "0"), a, engine.Battlefield)
	negative := g.NewCard(creatureDefPT(t, "2", "-1"), a, engine.Battlefield)
	alive := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(dead).Zone; z != engine.Graveyard {
		t.Errorf("zero-toughness creature zone = %v, want Graveyard", z)
	}
	if z := g.Card(negative).Zone; z != engine.Graveyard {
		t.Errorf("negative-toughness creature zone = %v, want Graveyard", z)
	}
	if z := g.Card(alive).Zone; z != engine.Battlefield {
		t.Errorf("positive-toughness creature zone = %v, want Battlefield", z)
	}
}

// A non-creature permanent with the same printed "toughness" text is
// untouched -- this rule is about creatures, not the field being nonempty.
func TestCheckStateBasedActionsLethalToughnessOnlyAppliesToCreatures(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	host := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	aura := g.NewCard(auraDef(t), a, engine.Battlefield)
	g.Attach(aura, host) // legally attached, so the attachment rule leaves it alone too

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(aura).Zone; z != engine.Battlefield {
		t.Errorf("an Aura (no toughness at all) zone = %v, want Battlefield", z)
	}
}

// An unresolvable toughness ("*") is a coverage gap, not a death sentence:
// the creature survives because this port cannot yet tell what its
// toughness actually is.
func TestCheckStateBasedActionsUnresolvableToughnessSurvives(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	star := g.NewCard(creatureDefPT(t, "*", "*"), a, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(star).Zone; z != engine.Battlefield {
		t.Errorf("a creature with unresolvable toughness zone = %v, want Battlefield", z)
	}
}

// A creature dying to lethal toughness in the same pass that cleans up
// dangling attachments must have its own Aura sent along with it --
// destroyLethalToughness has to run before cleanupDanglingAttachments, not
// on a later call, for a single CheckStateBasedActions pass to be enough.
func TestCheckStateBasedActionsLethalToughnessCascadesToAttachments(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	host := g.NewCard(creatureDefPT(t, "2", "0"), a, engine.Battlefield)
	aura := g.NewCard(auraDef(t), a, engine.Battlefield)
	g.Attach(aura, host)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(host).Zone; z != engine.Graveyard {
		t.Fatalf("setup: host zone = %v, want Graveyard", z)
	}
	if z := g.Card(aura).Zone; z != engine.Graveyard {
		t.Errorf("aura zone = %v, want Graveyard (its host died in the same pass)", z)
	}
}

// BaseLoyalty resolves a plain printed integer, on the same terms
// BasePower/BaseToughness already do.
func TestBaseLoyalty(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]

	plain := g.NewCard(planeswalkerDefLoyalty(t, "5"), p, engine.Battlefield)
	if l, ok := g.Card(plain).BaseLoyalty(); !ok || l != 5 {
		t.Errorf("BaseLoyalty() = (%d, %v), want (5, true)", l, ok)
	}

	x := g.NewCard(planeswalkerDefLoyalty(t, "X"), p, engine.Battlefield)
	if _, ok := g.Card(x).BaseLoyalty(); ok {
		t.Error("BaseLoyalty() resolved an \"X\" loyalty")
	}

	noDef := g.NewCard(nil, p, engine.Battlefield)
	if _, ok := g.Card(noDef).BaseLoyalty(); ok {
		t.Error("BaseLoyalty() resolved a card with no Def")
	}
}

// CR 704.5's planeswalker-loyalty rule: a planeswalker with loyalty zero
// or less goes to its owner's graveyard.
func TestCheckStateBasedActionsZeroLoyaltyDies(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	dead := g.NewCard(planeswalkerDefLoyalty(t, "3"), a, engine.Battlefield)
	pastZero := g.NewCard(planeswalkerDefLoyalty(t, "3"), a, engine.Battlefield)
	alive := g.NewCard(planeswalkerDefLoyalty(t, "3"), a, engine.Battlefield)
	g.Card(alive).Counters.Add(engine.Loyalty, 3)
	g.Card(pastZero).Counters.Add(engine.Loyalty, 1)
	g.Card(pastZero).Counters.Add(engine.Loyalty, -2) // paid a loyalty cost it did not have
	// dead is left at the default zero counters -- no ETB hook exists yet
	// to give it its printed starting loyalty (destroyZeroLoyalty's own doc
	// comment), which is itself what this test exercises.

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(dead).Zone; z != engine.Graveyard {
		t.Errorf("zero-loyalty planeswalker zone = %v, want Graveyard", z)
	}
	if z := g.Card(pastZero).Zone; z != engine.Graveyard {
		t.Errorf("planeswalker paid past zero loyalty, zone = %v, want Graveyard", z)
	}
	if z := g.Card(alive).Zone; z != engine.Battlefield {
		t.Errorf("positive-loyalty planeswalker zone = %v, want Battlefield", z)
	}
}

// A non-planeswalker permanent at the same (absent) loyalty count is
// untouched -- this rule is about planeswalkers, not about the counter
// being absent.
func TestCheckStateBasedActionsZeroLoyaltyOnlyAppliesToPlaneswalkers(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	creature := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(creature).Zone; z != engine.Battlefield {
		t.Errorf("a creature (no loyalty counter at all) zone = %v, want Battlefield", z)
	}
}

// HasKeyword matches the head as written, args or no args, and reports
// false for a keyword the card does not carry and for a nil Def.
func TestHasKeyword(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	def := creatureDefPT(t, "2", "2")
	def.Faces[0].Keywords = []string{"Flying", "Ward:2"}
	id := g.NewCard(def, p, engine.Battlefield)

	if !g.Card(id).HasKeyword("Flying") {
		t.Error("HasKeyword(\"Flying\") = false, want true")
	}
	if !g.Card(id).HasKeyword("Ward") {
		t.Error("HasKeyword(\"Ward\") = false for \"Ward:2\", want true -- the head, not the whole line")
	}
	if g.Card(id).HasKeyword("Trample") {
		t.Error("HasKeyword(\"Trample\") = true for a card that does not have it")
	}

	noDef := g.NewCard(nil, p, engine.Battlefield)
	if g.Card(noDef).HasKeyword("Flying") {
		t.Error("HasKeyword resolved a card with no Def")
	}
}

// CR 704.5g: a creature dealt damage at least equal to its toughness dies.
func TestCheckStateBasedActionsLethalDamageDies(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	exact := g.NewCard(creatureDefPT(t, "2", "3"), a, engine.Battlefield)
	excess := g.NewCard(creatureDefPT(t, "2", "3"), a, engine.Battlefield)
	survives := g.NewCard(creatureDefPT(t, "2", "3"), a, engine.Battlefield)
	g.Card(exact).Damage.Mark(3, false)
	g.Card(excess).Damage.Mark(10, false)
	g.Card(survives).Damage.Mark(2, false)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(exact).Zone; z != engine.Graveyard {
		t.Errorf("creature dealt exactly lethal damage, zone = %v, want Graveyard", z)
	}
	if z := g.Card(excess).Zone; z != engine.Graveyard {
		t.Errorf("creature dealt excess damage, zone = %v, want Graveyard", z)
	}
	if z := g.Card(survives).Zone; z != engine.Battlefield {
		t.Errorf("creature dealt sub-lethal damage, zone = %v, want Battlefield", z)
	}
}

// CR 704.5h: any amount of deathtouch damage is lethal on its own,
// regardless of toughness.
func TestCheckStateBasedActionsDeathtouchDamageDies(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	id := g.NewCard(creatureDefPT(t, "2", "10"), a, engine.Battlefield)
	g.Card(id).Damage.Mark(1, true)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(id).Zone; z != engine.Graveyard {
		t.Errorf("a 10-toughness creature dealt 1 deathtouch damage, zone = %v, want Graveyard", z)
	}
}

// Indestructible is the one keyword this port checks anywhere, and it
// stops both halves of the rule: lethal damage and deathtouch damage
// alike leave an indestructible creature on the battlefield.
func TestCheckStateBasedActionsIndestructibleSurvivesLethalAndDeathtouchDamage(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	lethal := g.NewCard(indestructibleCreatureDefPT(t, "2", "3"), a, engine.Battlefield)
	deathtouched := g.NewCard(indestructibleCreatureDefPT(t, "2", "3"), a, engine.Battlefield)
	g.Card(lethal).Damage.Mark(5, false)
	g.Card(deathtouched).Damage.Mark(1, true)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(lethal).Zone; z != engine.Battlefield {
		t.Errorf("indestructible creature dealt lethal damage, zone = %v, want Battlefield", z)
	}
	if z := g.Card(deathtouched).Zone; z != engine.Battlefield {
		t.Errorf("indestructible creature dealt deathtouch damage, zone = %v, want Battlefield", z)
	}
}

// A creature with damage marked but not yet lethal, and a non-creature
// permanent carrying damage, are both untouched.
func TestCheckStateBasedActionsSubLethalDamageAndNonCreaturesSurvive(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	survives := g.NewCard(creatureDefPT(t, "2", "3"), a, engine.Battlefield)
	g.Card(survives).Damage.Mark(2, false)
	host := g.NewCard(creatureDefPT(t, "2", "5"), a, engine.Battlefield)
	aura := g.NewCard(auraDef(t), a, engine.Battlefield)
	g.Attach(aura, host) // legally attached, so 704.5's attachment rule leaves it alone too
	g.Card(aura).Damage.Mark(100, false)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(survives).Zone; z != engine.Battlefield {
		t.Errorf("creature dealt sub-lethal damage, zone = %v, want Battlefield", z)
	}
	if z := g.Card(aura).Zone; z != engine.Battlefield {
		t.Errorf("a non-creature carrying damage, zone = %v, want Battlefield", z)
	}
}

// A creature whose toughness cannot be resolved (an unresolvable "*") is
// left alone even under damage, the same coverage-gap reasoning
// destroyLethalToughness already applies.
func TestCheckStateBasedActionsUnresolvableToughnessSurvivesDamage(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	star := g.NewCard(creatureDefPT(t, "*", "*"), a, engine.Battlefield)
	g.Card(star).Damage.Mark(100, false)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(star).Zone; z != engine.Battlefield {
		t.Errorf("a creature with unresolvable toughness under damage, zone = %v, want Battlefield", z)
	}
}

// The legend rule: two legendary permanents sharing a name under one
// player's control leave only the one the controller chooses.
func TestCheckStateBasedActionsLegendRuleKeepsOnlyOne(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	first := g.NewCard(legendaryCreatureDef(t, "Test Legend"), a, engine.Battlefield)
	second := g.NewCard(legendaryCreatureDef(t, "Test Legend"), a, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueLegendaryToKeep(first)
	engine.CheckStateBasedActions(g, c)

	if z := g.Card(first).Zone; z != engine.Battlefield {
		t.Errorf("kept legend zone = %v, want Battlefield", z)
	}
	if z := g.Card(second).Zone; z != engine.Graveyard {
		t.Errorf("other legend zone = %v, want Graveyard", z)
	}
}

// Two different players may each legally control their own copy of one
// legendary permanent -- the rule is "you control", not "anyone controls".
func TestCheckStateBasedActionsLegendRuleIsPerPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	mine := g.NewCard(legendaryCreatureDef(t, "Test Legend"), a, engine.Battlefield)
	theirs := g.NewCard(legendaryCreatureDef(t, "Test Legend"), b, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(mine).Zone; z != engine.Battlefield {
		t.Errorf("a's own legend zone = %v, want Battlefield", z)
	}
	if z := g.Card(theirs).Zone; z != engine.Battlefield {
		t.Errorf("b's own legend zone = %v, want Battlefield", z)
	}
}

// Legendary permanents with different names never conflict, and a
// legendary creature alone (no duplicate) is untouched.
func TestCheckStateBasedActionsLegendRuleNeedsAMatchingName(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	alone := g.NewCard(legendaryCreatureDef(t, "Solo Legend"), a, engine.Battlefield)
	other := g.NewCard(legendaryCreatureDef(t, "A Different Legend"), a, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(alone).Zone; z != engine.Battlefield {
		t.Errorf("a lone legend zone = %v, want Battlefield", z)
	}
	if z := g.Card(other).Zone; z != engine.Battlefield {
		t.Errorf("a differently-named legend zone = %v, want Battlefield", z)
	}
}

// A non-legendary creature sharing a name with another is untouched -- the
// rule is about the Legendary supertype, not about names in general.
func TestCheckStateBasedActionsLegendRuleOnlyAppliesToLegendaryPermanents(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	first := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	second := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(first).Zone; z != engine.Battlefield {
		t.Errorf("non-legendary creature zone = %v, want Battlefield", z)
	}
	if z := g.Card(second).Zone; z != engine.Battlefield {
		t.Errorf("its same-named non-legendary sibling zone = %v, want Battlefield", z)
	}
}

// A legendary permanent destroyed by the legend rule can leave its own Aura
// dangling in the same pass -- resolveLegendRule has to run before
// cleanupDanglingAttachments for one CheckStateBasedActions call to catch
// both.
func TestCheckStateBasedActionsLegendRuleCascadesToAttachments(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	keep := g.NewCard(legendaryCreatureDef(t, "Test Legend"), a, engine.Battlefield)
	lose := g.NewCard(legendaryCreatureDef(t, "Test Legend"), a, engine.Battlefield)
	aura := g.NewCard(auraDef(t), a, engine.Battlefield)
	g.Attach(aura, lose)

	c := engine.NewScriptedController()
	c.QueueLegendaryToKeep(keep)

	engine.CheckStateBasedActions(g, c)

	if z := g.Card(lose).Zone; z != engine.Graveyard {
		t.Fatalf("setup: losing legend zone = %v, want Graveyard", z)
	}
	if z := g.Card(aura).Zone; z != engine.Graveyard {
		t.Errorf("aura zone = %v, want Graveyard (its host died in the same pass)", z)
	}
}

// BaseDefense resolves a plain printed integer, on the same terms
// BaseLoyalty already does.
func TestBaseDefense(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]

	plain := g.NewCard(battleDefDefense(t, "4"), p, engine.Battlefield)
	if d, ok := g.Card(plain).BaseDefense(); !ok || d != 4 {
		t.Errorf("BaseDefense() = (%d, %v), want (4, true)", d, ok)
	}

	noDef := g.NewCard(nil, p, engine.Battlefield)
	if _, ok := g.Card(noDef).BaseDefense(); ok {
		t.Error("BaseDefense() resolved a card with no Def")
	}
}

// Colors derives from the mana cost absent a Colors: override -- the same
// logic carddb.Face.dumpColors already verifies byte-identical to Forge's
// own dump (M2's P1 gate).
func TestColorsDerivedFromManaCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]

	rg := g.NewCard(creatureDefManaCost(t, "1 R G"), p, engine.Battlefield)
	if got := g.Card(rg).Colors(); got != mana.Red|mana.Green {
		t.Errorf("Colors() = %v, want Red|Green", got)
	}

	colorless := g.NewCard(creatureDefManaCost(t, "3"), p, engine.Battlefield)
	if got := g.Card(colorless).Colors(); !got.IsColorless() {
		t.Errorf("Colors() = %v, want colorless", got)
	}
}

// An explicit Colors: override wins over the mana cost, the same as Forge's
// own dumpColors.
func TestColorsOverride(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]

	def := creatureDefManaCost(t, "1 R")
	def.Faces[0].Colors, def.Faces[0].HasColors = mana.Black, true
	id := g.NewCard(def, p, engine.Battlefield)

	if got := g.Card(id).Colors(); got != mana.Black {
		t.Errorf("Colors() = %v, want Black (the override), not Red (the cost)", got)
	}
}

func TestColorsNilDefIsColorless(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]

	id := g.NewCard(nil, p, engine.Battlefield)
	if got := g.Card(id).Colors(); !got.IsColorless() {
		t.Errorf("Colors() = %v, want colorless for a nil Def", got)
	}
}

// CR 704.5v: a Battle with defense zero or less goes to its owner's
// graveyard.
func TestCheckStateBasedActionsZeroDefenseDies(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	dead := g.NewCard(battleDefDefense(t, "3"), a, engine.Battlefield)
	alive := g.NewCard(battleDefDefense(t, "3"), a, engine.Battlefield)
	g.Card(alive).Counters.Add(engine.Defense, 3)
	// dead is left at the default zero counters -- no ETB hook exists yet
	// to give it its printed starting defense (destroyZeroDefense's own doc
	// comment), which is itself what this test exercises. Both get a
	// protector set directly, isolating destroyZeroDefense's own behavior
	// from assignBattleProtector, a separate state-based action this test
	// is not about (CR 704.5w -- a protectorless Battle would otherwise ask
	// the controller's queue for one, which this test never populates).
	g.Card(dead).ProtectingPlayer = b
	g.Card(alive).ProtectingPlayer = b

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(dead).Zone; z != engine.Graveyard {
		t.Errorf("zero-defense battle zone = %v, want Graveyard", z)
	}
	if z := g.Card(alive).Zone; z != engine.Battlefield {
		t.Errorf("positive-defense battle zone = %v, want Battlefield", z)
	}
}

// A non-Battle permanent at the same (absent) defense count is untouched.
func TestCheckStateBasedActionsZeroDefenseOnlyAppliesToBattles(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	creature := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(creature).Zone; z != engine.Battlefield {
		t.Errorf("a creature (no defense counter at all) zone = %v, want Battlefield", z)
	}
}

// A Battle destroyed by zero defense can leave its own Aura dangling in
// the same pass -- destroyZeroDefense has to run before
// cleanupDanglingAttachments for one CheckStateBasedActions call to catch
// both.
func TestCheckStateBasedActionsZeroDefenseCascadesToAttachments(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	host := g.NewCard(battleDefDefense(t, "3"), a, engine.Battlefield)
	aura := g.NewCard(auraDef(t), a, engine.Battlefield)
	g.Attach(aura, host)
	// A protector set directly isolates destroyZeroDefense from
	// assignBattleProtector, a separate state-based action this test is not
	// about (CR 704.5w).
	g.Card(host).ProtectingPlayer = b

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(host).Zone; z != engine.Graveyard {
		t.Fatalf("setup: host zone = %v, want Graveyard", z)
	}
	if z := g.Card(aura).Zone; z != engine.Graveyard {
		t.Errorf("aura zone = %v, want Graveyard (its host died in the same pass)", z)
	}
}

// CR 704.5w: a Battle with no protector, and nothing attacking it, asks its
// controller to choose one of their opponents.
func TestAssignBattleProtectorAsksWhenNoneSet(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	battle := g.NewCard(battleDefDefense(t, "5"), a, engine.Battlefield)
	g.Card(battle).Counters.Add(engine.Defense, 5) // survive destroyZeroDefense; NewCard seeds battlefield state directly, not through Move's ETB hook

	c := engine.NewScriptedController()
	c.QueueBattleProtector(b)
	engine.CheckStateBasedActions(g, c)

	if got := g.Card(battle).ProtectingPlayer; got != b {
		t.Errorf("ProtectingPlayer = %v, want %v", got, b)
	}
}

// A Battle with a protector already set, still in the game, is never asked
// again -- an empty queue must not panic.
func TestAssignBattleProtectorDoesNotAskWhenAlreadySet(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	battle := g.NewCard(battleDefDefense(t, "5"), a, engine.Battlefield)
	g.Card(battle).Counters.Add(engine.Defense, 5)
	g.Card(battle).ProtectingPlayer = b

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if got := g.Card(battle).ProtectingPlayer; got != b {
		t.Errorf("ProtectingPlayer = %v, want unchanged at %v", got, b)
	}
}

// A protector who has since lost the game no longer counts as one -- the
// Battle asks again.
func TestAssignBattleProtectorAsksAgainWhenProtectorHasLost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b", "c")
	a, b, c2 := g.Players()[0], g.Players()[1], g.Players()[2]
	g.Player(a).Life, g.Player(b).Life, g.Player(c2).Life = 20, 20, 20
	battle := g.NewCard(battleDefDefense(t, "5"), a, engine.Battlefield)
	g.Card(battle).Counters.Add(engine.Defense, 5)
	g.Card(battle).ProtectingPlayer = b
	g.Player(b).Lost = true

	c := engine.NewScriptedController()
	c.QueueBattleProtector(c2)
	engine.CheckStateBasedActions(g, c)

	if got := g.Card(battle).ProtectingPlayer; got != c2 {
		t.Errorf("ProtectingPlayer = %v, want %v (b has left the game)", got, c2)
	}
}

// A Battle currently being attacked is not asked about a protector at all
// -- CR 704.5w's own "no attacking creatures currently attacking that
// battle" condition.
func TestAssignBattleProtectorNotAskedWhileBeingAttacked(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	g.SetTurnState(1, a, engine.Main1)
	battle := g.NewCard(battleDefDefense(t, "5"), b, engine.Battlefield)
	g.Card(battle).Counters.Add(engine.Defense, 5)
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	ac.QueueAttackTarget(engine.CardEntity(battle))
	g.DeclareCombatAttackers(ac)

	// No QueueBattleProtector call: if assignBattleProtector asked anyway,
	// this panics on the empty queue, which is exactly the assertion.
	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if got := g.Card(battle).ProtectingPlayer; got != engine.NoPlayer {
		t.Errorf("ProtectingPlayer = %v, want NoPlayer (still unset while attacked)", got)
	}
}

// CR 704.5x: a Battle whose protector is somehow its own controller
// re-chooses, even though nothing attacks it.
func TestAssignBattleProtectorReassignsWhenProtectorIsOwnController(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	battle := g.NewCard(battleDefDefense(t, "5"), a, engine.Battlefield)
	g.Card(battle).Counters.Add(engine.Defense, 5)
	g.Card(battle).ProtectingPlayer = a // the battle's own controller

	c := engine.NewScriptedController()
	c.QueueBattleProtector(b)
	engine.CheckStateBasedActions(g, c)

	if got := g.Card(battle).ProtectingPlayer; got != b {
		t.Errorf("ProtectingPlayer = %v, want %v", got, b)
	}
}

// A Battle leaving the battlefield loses its protector, the same as any
// other battlefield-only state Move clears.
func TestMoveClearsProtectingPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	battle := g.NewCard(battleDefDefense(t, "5"), a, engine.Battlefield)
	g.Card(battle).ProtectingPlayer = b

	g.Move(battle, engine.Graveyard, a)

	if got := g.Card(battle).ProtectingPlayer; got != engine.NoPlayer {
		t.Errorf("ProtectingPlayer = %v after leaving the battlefield, want NoPlayer", got)
	}
}
