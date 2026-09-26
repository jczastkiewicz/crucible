package engine_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/valid"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// phasingDef compiles a card with typeLine and the given K:, T: and S:
// lines, plus name/value SVar pairs. A creature is a 2/2.
func phasingDef(t *testing.T, name, typeLine string, keywords, triggers, statics []string, svars ...string) *compile.Card {
	t.Helper()
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), typeLine)
	if strings.Contains(typeLine, "Creature") {
		raw.Faces[0].Power, raw.Faces[0].Toughness = "2", "2"
	}
	raw.Faces[0].Keywords = keywords
	raw.Faces[0].Triggers = triggers
	raw.Faces[0].Statics = statics
	for i := 0; i+1 < len(svars); i += 2 {
		raw.Faces[0].SVars.Set(svars[i], svars[i+1])
	}
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// pushLine puts line on the stack as p's ability hosted by host, with
// targets already chosen, without resolving it.
func pushLine(t *testing.T, g *engine.Game, p engine.PlayerID, source engine.CardID, line string, targets ...engine.EntityID) {
	t.Helper()
	def := etbChainDef(t, "Test Pushed", line)
	for _, sub := range def.Faces[0].Triggers[0].Subs {
		if !strings.EqualFold(sub.Key, "Execute") {
			continue
		}
		api, ok := engine.APIByName(sub.Ability.Name)
		if !ok {
			t.Fatalf("unknown API %q", sub.Ability.Name)
		}
		g.PushAbility(engine.Ability{API: api, Source: source, Controller: p, Params: sub.Ability, Amounts: def.Faces[0].Amounts, Targets: targets})
		return
	}
	t.Fatal("no Execute$")
}

// advanceToUntap walks g from the end of active's turn into the next
// player's untap step.
func advanceToUntap(t *testing.T, g *engine.Game, active engine.PlayerID, c *engine.ScriptedController) {
	t.Helper()
	g.SetTurnState(g.Turn(), active, engine.Cleanup)
	g.AdvancePhase(c)
	if g.ActivePhase() != engine.Untap || g.ActivePlayer() == active {
		t.Fatalf("advance: phase %v active %v, want the next player's untap", g.ActivePhase(), g.ActivePlayer())
	}
}

// phaseOut phases id out for p through the fixture seam.
func phaseOut(t *testing.T, g *engine.Game, id engine.CardID, p engine.PlayerID) {
	t.Helper()
	if err := g.SetPhasedOut(id, p); err != nil {
		t.Fatal(err)
	}
}

func TestPhasedOutPermanentLeavesTheEnumerationButNotTheZone(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	a := g.NewCard(creatureDef(t), p, engine.Battlefield)
	b := g.NewCard(creatureDef(t), p, engine.Battlefield)
	c := g.NewCard(creatureDef(t), p, engine.Battlefield)
	bf := g.Zone(engine.Battlefield, p)
	if &bf.Cards()[0] != &bf.CardsIncludingPhasedOut()[0] {
		t.Error("with nothing phased out, Cards() should be the zone's own slice")
	}

	phaseOut(t, g, b, p)
	if got, want := bf.Cards(), []engine.CardID{a, c}; !slices.Equal(got, want) {
		t.Errorf("Cards() = %v, want %v", got, want)
	}
	if got, want := bf.CardsIncludingPhasedOut(), []engine.CardID{a, b, c}; !slices.Equal(got, want) {
		t.Errorf("CardsIncludingPhasedOut() = %v, want %v", got, want)
	}
	card := g.Card(b)
	if card.Zone != engine.Battlefield || !bf.Contains(b) || bf.Len() != 3 {
		t.Errorf("zone %v contains %v len %d, want Battlefield/true/3", card.Zone, bf.Contains(b), bf.Len())
	}
	if !card.IsPhasedOut() || card.PhasedOutFor() != p {
		t.Errorf("IsPhasedOut %v for %v, want true for %v", card.IsPhasedOut(), card.PhasedOutFor(), p)
	}

	if err := g.SetPhasedOut(b, engine.NoPlayer); err != nil {
		t.Fatal(err)
	}
	if got, want := bf.Cards(), []engine.CardID{a, b, c}; !slices.Equal(got, want) {
		t.Errorf("after phasing in, Cards() = %v, want %v (order kept)", got, want)
	}

	hand := g.NewCard(creatureDef(t), p, engine.Hand)
	if err := g.SetPhasedOut(hand, p); err == nil {
		t.Error("SetPhasedOut on a card in hand: got nil error")
	}
}

func TestCloneCopiesPhasedOutStateIndependently(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	a := g.NewCard(creatureDef(t), p, engine.Battlefield)
	phaseOut(t, g, a, p)

	clone := g.Clone()
	if err := clone.SetPhasedOut(a, engine.NoPlayer); err != nil {
		t.Fatal(err)
	}
	if !g.Card(a).IsPhasedOut() || len(g.Zone(engine.Battlefield, p).Cards()) != 0 {
		t.Error("phasing in on the clone changed the original")
	}
	if len(clone.Zone(engine.Battlefield, p).Cards()) != 1 {
		t.Error("clone still hides the permanent it phased in")
	}
}

func TestLeavingTheBattlefieldPhasedOutEndsPhasingButLKIKeepsIt(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	a := g.NewCard(creatureDef(t), p, engine.Battlefield)
	phaseOut(t, g, a, p)
	g.Move(a, engine.Graveyard, p)
	if g.Card(a).IsPhasedOut() {
		t.Error("card in the graveyard still phased out")
	}
	if lki := g.LKI(a); lki == nil || !lki.IsPhasedOut() {
		t.Error("LKI lost the phased-out state (GameAction.java:977 reads it)")
	}
	g.Move(a, engine.Battlefield, p)
	if got := g.Zone(engine.Battlefield, p).Cards(); !slices.Equal(got, []engine.CardID{a}) {
		t.Errorf("returned card hidden: Cards() = %v", got)
	}

	b := g.NewCard(creatureDef(t), p, engine.Battlefield)
	phaseOut(t, g, b, p)
	g.MoveToLibraryTop(b, p)
	if g.Card(b).IsPhasedOut() {
		t.Error("card on top of the library still phased out")
	}
}

func TestPhasesDefinedSelfIsNotAZoneChange(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	sink := &recordingSink{}
	g.SetSink(sink)
	host, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, "DB$ Phases | Defined$ Self")
	if err != nil {
		t.Fatal(err)
	}
	if !g.Card(host).IsPhasedOut() || g.Card(host).Zone != engine.Battlefield {
		t.Fatalf("host phased out %v zone %v, want phased out on the battlefield", g.Card(host).IsPhasedOut(), g.Card(host).Zone)
	}
	var phased []engine.Event
	for _, e := range sink.events {
		if e.Kind == engine.ZoneChanged {
			t.Errorf("phasing emitted ZoneChanged: %+v", e)
		}
		if e.Kind == engine.Phased {
			phased = append(phased, e)
		}
	}
	if len(phased) != 1 || phased[0].Source != host || phased[0].Detail != uint32(engine.PhaseDetailOut) {
		t.Errorf("Phased events = %+v, want one PhaseDetailOut for %v", phased, host)
	}
	if engine.Phased.String() != "Phased" {
		t.Errorf("Phased.String() = %q", engine.Phased.String())
	}

	// Already phased out: phasing out again does nothing.
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, "DB$ Phases | Defined$ Remembered"); err != nil {
		t.Fatal(err)
	}
}

func TestPhasedOutTargetFizzlesTheSpellWaitingOnIt(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	sink := &recordingSink{}
	g.SetSink(sink)
	victim := g.NewCard(creatureDef(t), other, engine.Battlefield)
	bystander := g.NewCard(creatureDef(t), other, engine.Battlefield)
	removal := g.NewCard(creatureDef(t), p, engine.Battlefield)
	sturdy := g.NewCard(creatureDef(t), p, engine.Battlefield)
	charm := g.NewCard(creatureDef(t), p, engine.Battlefield)

	// Bottom to top: a Destroy on the victim alone (fizzles), a CantFizzle
	// Destroy on it (resolves, destroying nothing), a two-target Destroy (the
	// bystander still dies), a Charm whose one mode targets the victim.
	pushLine(t, g, p, removal, "DB$ Destroy | ValidTgts$ Creature", engine.CardEntity(victim))
	pushLine(t, g, p, sturdy, "DB$ Destroy | ValidTgts$ Creature | CantFizzle$ True", engine.CardEntity(victim))
	pushLine(t, g, p, removal, "DB$ Destroy | ValidTgts$ Creature | TargetMax$ 2", engine.CardEntity(victim), engine.CardEntity(bystander))
	modeDef := etbChainDef(t, "Test Mode", "DB$ Destroy | ValidTgts$ Creature")
	var mode engine.Ability
	for _, sub := range modeDef.Faces[0].Triggers[0].Subs {
		if strings.EqualFold(sub.Key, "Execute") {
			mode = engine.Ability{API: engine.APIDestroy, Source: charm, Controller: p, Params: sub.Ability, Targets: []engine.EntityID{engine.CardEntity(victim)}}
		}
	}
	g.PushAbility(engine.Ability{API: engine.APICharm, Source: charm, Controller: p, Params: mode.Params, Modes: []engine.Ability{mode}})

	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(victim)}, "DB$ Phases | ValidTgts$ Creature"); err != nil {
		t.Fatal(err)
	}
	if !g.Card(victim).IsPhasedOut() || g.Card(victim).Zone != engine.Battlefield {
		t.Errorf("victim phased out %v zone %v, want still on the battlefield, phased out", g.Card(victim).IsPhasedOut(), g.Card(victim).Zone)
	}
	if g.Card(bystander).Zone != engine.Graveyard {
		t.Errorf("bystander zone %v, want Graveyard: one legal target keeps the spell alive", g.Card(bystander).Zone)
	}
	resolved := map[engine.CardID]int{}
	for _, e := range sink.events {
		if e.Kind == engine.AbilityResolved {
			resolved[e.Source]++
		}
	}
	if resolved[charm] != 0 {
		t.Errorf("Charm with its one target phased out resolved %d times, want it to fizzle", resolved[charm])
	}
	if resolved[sturdy] != 1 {
		t.Errorf("CantFizzle$ Destroy resolved %d times, want 1", resolved[sturdy])
	}
	if resolved[removal] != 1 {
		t.Errorf("removal resolved %d times, want 1 (the two-target one only)", resolved[removal])
	}
}

func TestAuraSpellFizzlesWhenItsTargetPhasesOut(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	host := g.NewCard(creatureDef(t), p, engine.Battlefield)
	aura := g.NewCard(auraDef(t), p, engine.Stack)
	g.PushAbility(engine.Ability{API: engine.APIAttach, Source: aura, Controller: p, Target: host})
	phaseOut(t, g, host, p)
	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatal(err)
	}
	if g.Card(aura).Zone == engine.Battlefield {
		t.Error("Aura entered attached to a phased-out host")
	}
}

func TestPhasingTakesAttachmentsAndTheUntapStepReturnsThem(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	creature := g.NewCard(creatureDef(t), p, engine.Battlefield)
	aura := g.NewCard(auraDef(t), p, engine.Battlefield)
	g.Attach(aura, creature)
	g.Card(creature).Tapped = true

	if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(creature)}, "DB$ Phases | ValidTgts$ Creature"); err != nil {
		t.Fatal(err)
	}
	if !g.Card(creature).IsPhasedOut() || !g.Card(aura).IsPhasedOut() {
		t.Fatalf("creature %v aura %v, want both phased out", g.Card(creature).IsPhasedOut(), g.Card(aura).IsPhasedOut())
	}

	advanceToUntap(t, g, p, c)
	if !g.Card(creature).IsPhasedOut() {
		t.Error("phased in on the other player's untap step")
	}
	advanceToUntap(t, g, other, c)
	cr, au := g.Card(creature), g.Card(aura)
	if cr.IsPhasedOut() || au.IsPhasedOut() {
		t.Fatalf("creature %v aura %v, want both phased in on %v's untap step", cr.IsPhasedOut(), au.IsPhasedOut(), p)
	}
	if host, ok := au.AttachedTo(); !ok || host != creature {
		t.Error("Aura came back unattached")
	}
	if cr.Tapped {
		t.Error("creature phased in and was not untapped: phasing must come before untapping")
	}
}

func TestPhasingInUnattachesFromAHostThatLeft(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	creature := g.NewCard(creatureDef(t), p, engine.Battlefield)
	equipment := g.NewCard(equipmentDef(t), p, engine.Battlefield)
	g.Attach(equipment, creature)
	if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(equipment)}, "DB$ Phases | ValidTgts$ Artifact"); err != nil {
		t.Fatal(err)
	}
	g.Move(creature, engine.Graveyard, p)
	advanceToUntap(t, g, p, c)
	advanceToUntap(t, g, other, c)
	if g.Card(equipment).IsPhasedOut() {
		t.Fatal("equipment still phased out")
	}
	if _, ok := g.Card(equipment).AttachedTo(); ok {
		t.Error("equipment phased in still attached to a card in the graveyard")
	}
}

func TestWontPhaseInNormalStaysOutAtUntap(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	creature := g.NewCard(creatureDef(t), p, engine.Battlefield)
	if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(creature)}, "DB$ Phases | ValidTgts$ Creature | WontPhaseInNormal$ True"); err != nil {
		t.Fatal(err)
	}
	advanceToUntap(t, g, p, c)
	advanceToUntap(t, g, other, c)
	if !g.Card(creature).IsPhasedOut() {
		t.Error("WontPhaseInNormal$ permanent phased in on its controller's untap step")
	}
}

func TestKeywordPhasingAlternatesAndFiresPhaseTriggers(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	reg := engine.NewRegistry()
	imp := g.NewCard(phasingDef(t, "Test Imp", "Creature Elf", []string{"Phasing"},
		[]string{
			"Mode$ PhaseOut | ValidCard$ Card.phasedOutSelf | Execute$ TrigOut",
			"Mode$ PhaseIn | ValidCard$ Card.Self | Execute$ TrigIn",
		}, nil,
		"TrigOut", "DB$ GainLife | Defined$ You | LifeAmount$ 1",
		"TrigIn", "DB$ GainLife | Defined$ You | LifeAmount$ 10"), p, engine.Battlefield)
	g.NewCard(phasingDef(t, "Test Watcher", "Creature Elf", nil,
		[]string{"Mode$ PhaseOutAll | ValidCards$ Permanent.phasedOutOther | TriggerZones$ Battlefield | Execute$ TrigAll"}, nil,
		"TrigAll", "DB$ GainLife | Defined$ You | LifeAmount$ 100"), p, engine.Battlefield)
	// An Aura with phasing on the phasing creature phases out with it,
	// indirectly (CR 702.26h), not on its own.
	aura := g.NewCard(phasingDef(t, "Test Phasing Aura", "Enchantment Aura", []string{"Phasing"}, nil, nil), p, engine.Battlefield)
	g.Attach(aura, imp)

	advanceToUntap(t, g, other, c) // p's untap step
	if err := g.ResolveStack(reg, c); err != nil {
		t.Fatal(err)
	}
	if !g.Card(imp).IsPhasedOut() || !g.Card(aura).IsPhasedOut() {
		t.Fatalf("imp %v aura %v, want both phased out", g.Card(imp).IsPhasedOut(), g.Card(aura).IsPhasedOut())
	}
	if got := g.Player(p).Life; got != 20+1+100 {
		t.Errorf("life = %d, want 121 (PhaseOut 1, PhaseOutAll 100)", got)
	}

	advanceToUntap(t, g, p, c) // other's untap step
	if !g.Card(imp).IsPhasedOut() {
		t.Fatal("phasing creature phased in on the other player's untap step")
	}
	advanceToUntap(t, g, other, c) // p's untap step again
	if err := g.ResolveStack(reg, c); err != nil {
		t.Fatal(err)
	}
	if g.Card(imp).IsPhasedOut() || g.Card(aura).IsPhasedOut() {
		t.Fatalf("imp %v aura %v, want both phased in", g.Card(imp).IsPhasedOut(), g.Card(aura).IsPhasedOut())
	}
	if got := g.Player(p).Life; got != 121+10 {
		t.Errorf("life = %d, want 131 (PhaseIn 10)", got)
	}
}

func TestPhaseInOrOutFlipsEachWayAndTapsWhatPhasesIn(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	out := g.NewCard(creatureDef(t), p, engine.Battlefield)
	in := g.NewCard(creatureDef(t), p, engine.Battlefield)
	phaseOut(t, g, out, p)

	// Time and Tide's own spelling: a phased-out creature has no "Other".
	if _, err := resolveNow(t, g, p, c, nil, "DB$ Phases | AllValid$ Card.phasedOutCreature,Creature.Other | PhaseInOrOut$ True | Tapped$ True | WontPhaseInNormal$ True"); err != nil {
		t.Fatal(err)
	}
	if o := g.Card(out); o.IsPhasedOut() || !o.Tapped {
		t.Errorf("phased-out creature: phased out %v tapped %v, want phased in, tapped", o.IsPhasedOut(), o.Tapped)
	}
	if !g.Card(in).IsPhasedOut() {
		t.Error("phased-in creature did not phase out")
	}

	g.Card(in).Tapped = true
	if _, err := resolveNow(t, g, p, c, nil, "DB$ Phases | AllValid$ Card.phasedOutCreature | PhaseInOrOut$ True | Untapped$ True"); err != nil {
		t.Fatal(err)
	}
	if g.Card(in).IsPhasedOut() || g.Card(in).Tapped {
		t.Errorf("second flip: phased out %v tapped %v, want phased in, untapped", g.Card(in).IsPhasedOut(), g.Card(in).Tapped)
	}
}

func TestPhasesAllValidRemembersAndHonoursAnyNumber(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	hidden := g.NewCard(creatureDef(t), p, engine.Battlefield)
	shown := g.NewCard(creatureDef(t), p, engine.Battlefield)
	theirs := g.NewCard(creatureDef(t), other, engine.Battlefield)
	phaseOut(t, g, hidden, p)

	host, err := resolveNow(t, g, p, c, nil, "DB$ Phases | AllValid$ Creature.YouCtrl+Other | RememberAffected$ True | RememberValids$ True")
	if err != nil {
		t.Fatal(err)
	}
	if !g.Card(shown).IsPhasedOut() || g.Card(theirs).IsPhasedOut() {
		t.Error("AllValid$ phased the wrong permanents")
	}
	var remembered []engine.CardID
	for _, e := range g.Card(host).Memory.Remembered() {
		if id, ok := e.AsCard(); ok {
			remembered = append(remembered, id)
		}
	}
	// RememberAffected$ adds shown; RememberValids$ adds every valid one, shown
	// again (a no-op) -- the phased-out one was never valid without
	// PhaseInOrOut$.
	if !slices.Equal(remembered, []engine.CardID{shown}) {
		t.Errorf("remembered = %v, want [%v]", remembered, shown)
	}

	a := g.NewCard(creatureDef(t), other, engine.Battlefield)
	c.QueueCardChoice([]engine.CardID{a})
	if _, err := resolveNow(t, g, p, c, nil, "DB$ Phases | AllValid$ Creature.OppCtrl | AnyNumber$ True"); err != nil {
		t.Fatal(err)
	}
	if !g.Card(a).IsPhasedOut() || g.Card(theirs).IsPhasedOut() {
		t.Error("AnyNumber$ phased something other than the pick")
	}

	c.QueueCardChoice([]engine.CardID{hidden})
	if _, err := resolveNow(t, g, p, c, nil, "DB$ Phases | AllValid$ Creature.OppCtrl | AnyNumber$ True"); err == nil {
		t.Error("AnyNumber$ pick outside the options: got nil error")
	}
}

func TestPhasesTargetedPlayerCtrlAndDefinedValid(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	mine := g.NewCard(creatureDef(t), p, engine.Battlefield)
	theirs := g.NewCard(creatureDef(t), other, engine.Battlefield)

	if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.PlayerEntity(other)}, "DB$ Phases | ValidTgts$ Player | AllValid$ Creature.TargetedPlayerCtrl"); err != nil {
		t.Fatal(err)
	}
	if !g.Card(theirs).IsPhasedOut() || g.Card(mine).IsPhasedOut() {
		t.Errorf("TargetedPlayerCtrl: theirs %v mine %v, want only theirs out", g.Card(theirs).IsPhasedOut(), g.Card(mine).IsPhasedOut())
	}

	// Teferi's Protection's own line: every permanent you control, the host
	// included.
	host, err := resolveNow(t, g, p, c, nil, "DB$ Phases | Defined$ Valid Permanent.YouCtrl & Self")
	if err != nil {
		t.Fatal(err)
	}
	if !g.Card(mine).IsPhasedOut() || !g.Card(host).IsPhasedOut() {
		t.Error("Defined$ Valid Permanent.YouCtrl left a permanent phased in")
	}

	for _, line := range []string{
		"DB$ Phases | ValidTgts$ Player | AllValid$ Creature.!TargetedPlayerCtrl",
		"DB$ Phases | Defined$ Valid Creature.!TargetedPlayerCtrl",
		"DB$ Phases | Defined$ Bogus",
	} {
		if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.PlayerEntity(other)}, line); err == nil {
			t.Errorf("%q: got nil error", line)
		}
	}
}

func TestMatchesReadsAPhasedOutPermanentThroughItsPhasedOutPrefix(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	out := g.NewCard(creatureDef(t), p, engine.Battlefield)
	in := g.NewCard(creatureDef(t), p, engine.Battlefield)
	phaseOut(t, g, out, p)
	token := g.NewCard(creatureDef(t), p, engine.Battlefield)
	g.Card(token).IsToken = true

	for _, tc := range []struct {
		card   engine.CardID
		spec   string
		source engine.CardID
		want   bool
	}{
		{out, "Creature", engine.NoCard, true},
		{out, "Creature.YouCtrl", engine.NoCard, false},
		{out, "Creature.powerGE1", engine.NoCard, false},
		{out, "Creature.phasedOutYouCtrl", engine.NoCard, true},
		{out, "Card.phasedOutSelf", out, true},
		{out, "Creature.!YouCtrl", engine.NoCard, true},
		{out, "Card.phasedOutPermanent", engine.NoCard, true},
		{out, "Card.phasedOutphasedIn", engine.NoCard, false},
		{in, "Card.phasedOutSelf", in, false},
		{in, "Card.phasedIn", engine.NoCard, true},
		{in, "Card.Permanent", engine.NoCard, true},
		{in, "Card.token", engine.NoCard, false},
		{token, "Card.token", engine.NoCard, true},
		{in, "Creature.powerGE1", engine.NoCard, true},
	} {
		if got := engine.Matches(g, g.Card(tc.card), valid.Parse(tc.spec), p, tc.source); got != tc.want {
			t.Errorf("Matches(%v, %q) = %v, want %v", tc.card, tc.spec, got, tc.want)
		}
	}
}

func TestCantPhaseStaticsHoldAPermanentWhereItIs(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	g.NewCard(phasingDef(t, "Test Disciple", "Creature Elf", nil, nil,
		[]string{"Mode$ CantPhaseIn | ValidCard$ Card.phasedOutPermanent"}), other, engine.Battlefield)
	stuck := g.NewCard(creatureDef(t), p, engine.Battlefield)
	phaseOut(t, g, stuck, p)

	advanceToUntap(t, g, other, c)
	if !g.Card(stuck).IsPhasedOut() {
		t.Error("CantPhaseIn static let a permanent phase in at untap")
	}
	// Time and Tide's own spelling: a phased-out creature has no YouCtrl.
	if _, err := resolveNow(t, g, p, c, nil, "DB$ Phases | AllValid$ Card.phasedOutCreature | PhaseInOrOut$ True"); err != nil {
		t.Fatal(err)
	}
	if !g.Card(stuck).IsPhasedOut() {
		t.Error("CantPhaseIn static let PhaseInOrOut$ phase a permanent in")
	}
}

func TestCantPhaseOutStaticStopsPhasesAndUntapPhasing(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	g.NewCard(phasingDef(t, "Test Binding", "Enchantment", nil, nil,
		[]string{"Mode$ CantPhaseOut | ValidCard$ Creature.YouCtrl"}), p, engine.Battlefield)
	wurm := g.NewCard(phasingDef(t, "Test Wurm", "Creature Elf", []string{"Phasing"}, nil, nil), p, engine.Battlefield)
	aura := g.NewCard(auraDef(t), p, engine.Battlefield)
	g.Attach(aura, wurm)

	advanceToUntap(t, g, other, c)
	if g.Card(wurm).IsPhasedOut() {
		t.Error("CantPhaseOut static let K:Phasing phase a creature out")
	}
	if _, err := resolveNow(t, g, p, c, nil, "DB$ Phases | AllValid$ Creature.YouCtrl+Other | PhaseInOrOut$ True"); err != nil {
		t.Fatal(err)
	}
	if g.Card(wurm).IsPhasedOut() {
		t.Error("CantPhaseOut static let PhaseInOrOut$ phase a creature out")
	}

	// The creature cannot phase out, but the Aura on it can, and does on
	// its own when the Aura is what phases.
	if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(aura)}, "DB$ Phases | ValidTgts$ Enchantment"); err != nil {
		t.Fatal(err)
	}
	if !g.Card(aura).IsPhasedOut() || g.Card(wurm).IsPhasedOut() {
		t.Errorf("aura %v wurm %v, want the Aura alone out", g.Card(aura).IsPhasedOut(), g.Card(wurm).IsPhasedOut())
	}
}

func TestCantPhaseStaticSkipsAnAttachmentThatCannotPhaseOut(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	g.NewCard(phasingDef(t, "Test Binding", "Enchantment", nil, nil,
		[]string{"Mode$ CantPhaseOut | ValidCard$ Artifact"}), p, engine.Battlefield)
	creature := g.NewCard(creatureDef(t), p, engine.Battlefield)
	equipment := g.NewCard(equipmentDef(t), p, engine.Battlefield)
	g.Attach(equipment, creature)
	if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(creature)}, "DB$ Phases | ValidTgts$ Creature"); err != nil {
		t.Fatal(err)
	}
	if !g.Card(creature).IsPhasedOut() || g.Card(equipment).IsPhasedOut() {
		t.Errorf("creature %v equipment %v, want the equipment left behind", g.Card(creature).IsPhasedOut(), g.Card(equipment).IsPhasedOut())
	}
}

// TestPandoricaCantPhaseInWhileItsSourceIsTapped is the_pandorica.txt's
// chain: the effect's CantPhaseIn static reads IsPresent$
// Card.EffectSource+tapped, and ForgetOnPhasedIn$ exiles the effect once
// the permanent comes back.
func TestPandoricaCantPhaseInWhileItsSourceIsTapped(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	victim := g.NewCard(creatureDef(t), other, engine.Battlefield)
	host, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(victim)},
		"DB$ Phases | ValidTgts$ Creature | SubAbility$ DBEffect",
		"DBEffect", "DB$ Effect | StaticAbilities$ CantPhaseIn | RememberObjects$ Targeted | Duration$ Permanent | ForgetOnPhasedIn$ True",
		"CantPhaseIn", "Mode$ CantPhaseIn | ValidCard$ Card.phasedOutIsRemembered | IsPresent$ Card.EffectSource+tapped")
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Zone(engine.Command, p).Cards()) != 1 {
		t.Fatalf("command zone %v, want the effect", g.Zone(engine.Command, p).Cards())
	}
	g.Card(host).Tapped = true

	advanceToUntap(t, g, p, c) // other's untap step
	if !g.Card(victim).IsPhasedOut() {
		t.Fatal("phased in while the effect source is tapped")
	}
	g.Card(host).Tapped = false
	advanceToUntap(t, g, other, c)
	advanceToUntap(t, g, p, c)
	if g.Card(victim).IsPhasedOut() {
		t.Fatal("still phased out after the effect source untapped")
	}
	if n := len(g.Zone(engine.Command, p).Cards()); n != 0 {
		t.Errorf("command zone holds %d cards, want the effect exiled once it forgot its last card", n)
	}
}

func TestForgetOnPhasedInCreatesNoEffectWithNothingRemembered(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil,
		"DB$ Effect | RememberObjects$ Remembered | Duration$ Permanent | ForgetOnPhasedIn$ True"); err != nil {
		t.Fatal(err)
	}
	if n := len(g.Zone(engine.Command, p).Cards()); n != 0 {
		t.Errorf("command zone holds %d cards, want no effect (EffectEffect.java:94)", n)
	}
}

func TestPhasedOutPumpDoesNotStack(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	creature, err := resolveNow(t, g, p, c, nil,
		"DB$ Pump | Defined$ Self | NumAtt$ 2 | Duration$ Permanent | SubAbility$ DBAnimate",
		"DBAnimate", "DB$ Animate | Defined$ Self | Types$ Artifact | Duration$ Permanent")
	if err != nil {
		t.Fatal(err)
	}
	phaseOut(t, g, creature, p)
	for i := 0; i < 3; i++ {
		engine.CheckStateBasedActions(g, c)
	}
	if err := g.SetPhasedOut(creature, engine.NoPlayer); err != nil {
		t.Fatal(err)
	}
	engine.CheckStateBasedActions(g, c)
	if pw, _ := g.Card(creature).Power(); pw != 3 {
		t.Errorf("power = %d, want 3: one +2 pump on a 1/1, not one per state-based pass", pw)
	}
	if !g.Card(creature).Type().Has(cardtype.Artifact) {
		t.Error("the Animate effect did not come back with the permanent")
	}
}

func TestPhasedOutPermanentIgnoresNonTargetedEffects(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	for _, tc := range []struct {
		name, line string
		tapped     bool
		check      func(*engine.Card) bool
	}{
		{"destroy", "DB$ Destroy | Defined$ Remembered", false, func(x *engine.Card) bool { return x.Zone == engine.Battlefield }},
		{"bounce", "DB$ ChangeZone | Defined$ Remembered | Origin$ Battlefield | Destination$ Hand", false, func(x *engine.Card) bool { return x.Zone == engine.Battlefield }},
		{"tap", "DB$ Tap | Defined$ Remembered", false, func(x *engine.Card) bool { return !x.Tapped }},
		{"untap", "DB$ Untap | Defined$ Remembered", true, func(x *engine.Card) bool { return x.Tapped }},
	} {
		victim := g.NewCard(creatureDef(t), p, engine.Battlefield)
		g.Card(victim).Tapped = tc.tapped
		if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(victim)},
			"DB$ Phases | ValidTgts$ Creature | RememberAffected$ True | SubAbility$ DBNext", "DBNext", tc.line); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !tc.check(g.Card(victim)) {
			t.Errorf("%s acted on a phased-out permanent", tc.name)
		}
	}

	host, err := resolveNow(t, g, p, c, nil, "DB$ Phases | Defined$ Self | SubAbility$ DBSac", "DBSac", "DB$ Sacrifice")
	if err != nil {
		t.Fatal(err)
	}
	if g.Card(host).Zone != engine.Battlefield {
		t.Error("a phased-out permanent was sacrificed")
	}
}

func TestCleanupClearsDamageOnPhasedOutPermanents(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	creature := g.NewCard(creatureDef(t), p, engine.Battlefield)
	g.Card(creature).Damage.Mark(1, false)
	phaseOut(t, g, creature, p)
	g.SetTurnState(1, p, engine.EndOfTurn)
	g.AdvancePhase(engine.NewScriptedController())
	if g.ActivePhase() != engine.Cleanup {
		t.Fatalf("phase %v, want Cleanup", g.ActivePhase())
	}
	if m := g.Card(creature).Damage.Marked; m != 0 {
		t.Errorf("damage = %d, want 0 (PhaseHandler.java:400 clears phased-out permanents too)", m)
	}
}

func TestPhasingOutRemovesFromCombat(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	attacker := g.NewCard(creatureDef(t), p, engine.Battlefield)
	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	if len(g.Attackers()) != 1 {
		t.Fatalf("setup: attackers %v", g.Attackers())
	}
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(attacker)}, "DB$ Phases | ValidTgts$ Creature"); err != nil {
		t.Fatal(err)
	}
	if len(g.Attackers()) != 0 {
		t.Errorf("attackers = %v, want the phased-out creature removed from combat", g.Attackers())
	}
}

// TestOublietteReturnsItsCreatureTapped runs oubliette.txt from the corpus:
// its ETB phases the target out with WontPhaseInNormal$ and an effect card
// watching Oubliette; Oubliette leaving phases the creature back in, tapped
// (PhaseInOrOut$ | Tapped$), and the effect exiles itself (ChangeZone
// Origin$ Command, GameAction.java:100-106).
func TestOublietteReturnsItsCreatureTapped(t *testing.T) {
	t.Parallel()

	db := scenarioDB(t)
	g := engine.NewGame(db, javarand.New(1), []string{"human", "ai"})
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	oubDef, ok := db.Card("Oubliette")
	bearsDef, ok2 := db.Card("Grizzly Bears")
	if !ok || !ok2 {
		t.Fatal("corpus lacks Oubliette or Grizzly Bears")
	}
	bears := g.NewCard(bearsDef, other, engine.Battlefield)
	oub := g.NewCard(oubDef, p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Black, 3)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardB)
	c.QueueTargets([]engine.EntityID{engine.CardEntity(bears)})
	if !g.CastSpell(p, oub, c) {
		t.Fatal("CastSpell Oubliette failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if !g.Card(bears).IsPhasedOut() || len(g.Zone(engine.Command, p).Cards()) != 1 {
		t.Fatalf("after ETB: phased out %v, command %v; want phased out and one effect", g.Card(bears).IsPhasedOut(), g.Zone(engine.Command, p).Cards())
	}

	pushLine(t, g, p, g.NewCard(creatureDef(t), p, engine.Battlefield), "DB$ Destroy | ValidTgts$ Enchantment", engine.CardEntity(oub))
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	b := g.Card(bears)
	if b.IsPhasedOut() || !b.Tapped {
		t.Errorf("after Oubliette left: phased out %v tapped %v, want phased in, tapped", b.IsPhasedOut(), b.Tapped)
	}
	if n := len(g.Zone(engine.Command, p).Cards()); n != 0 {
		t.Errorf("command zone holds %d cards, want the effect exiled", n)
	}
}

// TestChangeZoneFromCommandIsOnlyAnEffectExilingItself: Origin$ Command
// resolves only for an effect card going to exile; any other destination,
// or a card in the Command zone that is not an effect, is refused.
func TestChangeZoneFromCommandIsOnlyAnEffectExilingItself(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	for _, line := range []string{
		"DB$ ChangeZone | Defined$ Self | Origin$ Command | Destination$ Hand",
		"DB$ ChangeZone | Defined$ Remembered | Origin$ Command | Destination$ Exile",
	} {
		_, err := resolveNow(t, g, p, engine.NewScriptedController(), nil,
			"DB$ Pump | Defined$ Self | SubAbility$ DBMove", "DBMove", line)
		if strings.Contains(line, "Hand") && (err == nil || !strings.Contains(err.Error(), "not resolvable yet")) {
			t.Errorf("%q: err = %v, want not resolvable yet", line, err)
		}
		if !strings.Contains(line, "Hand") && err != nil {
			t.Errorf("%q: %v", line, err)
		}
	}

	// A plain card sitting in the Command zone, remembered by the host.
	host, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, "DB$ BlankLine")
	if err != nil {
		t.Fatal(err)
	}
	plain := g.NewCard(creatureDef(t), p, engine.Command)
	g.Card(host).Memory.Remember(engine.CardEntity(plain))
	pushLine(t, g, p, host, "DB$ ChangeZone | Defined$ Remembered | Origin$ Command | Destination$ Exile")
	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err == nil || !strings.Contains(err.Error(), "not an effect") {
		t.Errorf("non-effect card from Command: err = %v, want not resolvable yet", err)
	}
}

// TestForgetOnPhasedInKeepsTheEffectWhileItRemembersAnother is Out of Time's
// shape with two creatures: the first to phase in is forgotten, the effect
// stays for the second.
func TestForgetOnPhasedInKeepsTheEffectWhileItRemembersAnother(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	a := g.NewCard(creatureDef(t), p, engine.Battlefield)
	b := g.NewCard(creatureDef(t), p, engine.Battlefield)
	if _, err := resolveNow(t, g, p, c, nil,
		"DB$ Phases | AllValid$ Creature.Other | RememberAffected$ True | WontPhaseInNormal$ True | SubAbility$ DBEffect",
		"DBEffect", "DB$ Effect | RememberObjects$ Remembered | Duration$ Permanent | ForgetOnPhasedIn$ True"); err != nil {
		t.Fatal(err)
	}
	if !g.Card(a).IsPhasedOut() || !g.Card(b).IsPhasedOut() || len(g.Zone(engine.Command, p).Cards()) != 1 {
		t.Fatal("setup: want both creatures phased out and one effect")
	}
	// A phased-out creature cannot be targeted, so pick it instead.
	c.QueueCardChoice([]engine.CardID{a})
	if _, err := resolveNow(t, g, p, c, nil, "DB$ Phases | AllValid$ Card.phasedOutCreature | PhaseInOrOut$ True | AnyNumber$ True"); err != nil {
		t.Fatal(err)
	}
	cmd := g.Zone(engine.Command, p).Cards()
	if g.Card(a).IsPhasedOut() || len(cmd) != 1 {
		t.Fatalf("after one phased in: phased out %v, command %v; want it in and the effect kept", g.Card(a).IsPhasedOut(), cmd)
	}
	for _, e := range g.Card(cmd[0]).Memory.Remembered() {
		if id, ok := e.AsCard(); ok && id == a {
			t.Error("effect still remembers the creature that phased in")
		}
	}
}
