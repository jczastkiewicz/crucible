package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// ringOf is p's "The Ring" card, failing the test when p has none.
func ringOf(t *testing.T, g *engine.Game, p engine.PlayerID) engine.CardID {
	t.Helper()
	cards := namedIn(g, engine.Command, p, "The Ring")
	if len(cards) != 1 {
		t.Fatalf("p's Command zone = %v, want one The Ring card", g.Zone(engine.Command, p).Cards())
	}
	if !g.Card(cards[0]).IsEffect {
		t.Fatal("The Ring is not an effect card")
	}
	return cards[0]
}

// ringBearerGame is a two-player game in p's first main phase where the
// Ring has tempted p level times and bearer, a power/toughness creature p
// controls, is p's Ring-bearer.
func ringBearerGame(t *testing.T, level int, power, toughness string) (*engine.Game, engine.PlayerID, engine.PlayerID, engine.CardID) {
	t.Helper()
	g, p, other := newTwoPlayerGame(t)
	bearer := g.NewCard(creatureDefPT(t, power, toughness), p, engine.Battlefield)
	g.SetRingTemptedYou(p, level)
	g.SetRingBearer(p, bearer)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	return g, p, other, bearer
}

// pushAndResolveErr resolves line as p's ability from a noncreature host
// (an Enchantment whose one trigger never fires), so the host is no
// Ring-bearer candidate, and returns ResolveStack's error.
func pushAndResolveErr(t *testing.T, g *engine.Game, p engine.PlayerID, c engine.PlayerController, line string, targets ...engine.EntityID) error {
	t.Helper()
	def := triggerWatcherDef(t, "Tempter", "Mode$ Drawn | ValidCard$ Card.Self | Execute$ Trig", "Trig", line)
	host := g.NewCard(def, p, engine.Battlefield)
	face := def.Faces[0]
	for _, sub := range face.Triggers[0].Subs {
		if strings.EqualFold(sub.Key, "Execute") {
			api, ok := engine.APIByName(sub.Ability.Name)
			if !ok {
				t.Fatalf("unknown API %q", sub.Ability.Name)
			}
			g.PushAbility(engine.Ability{API: api, Source: host, Controller: p, Params: sub.Ability, Amounts: face.Amounts, Targets: targets})
			return g.ResolveStack(engine.NewRegistry(), c)
		}
	}
	t.Fatal("no Execute$")
	return nil
}

// pushAndResolve is pushAndResolveErr failing the test on an error.
func pushAndResolve(t *testing.T, g *engine.Game, p engine.PlayerID, c engine.PlayerController, line string, targets ...engine.EntityID) {
	t.Helper()
	if err := pushAndResolveErr(t, g, p, c, line, targets...); err != nil {
		t.Fatalf("%q: %v", line, err)
	}
}

// attackWith declares attacker as p's only attacker and blocks answers
// the defender's block declaration.
func attackWith(t *testing.T, g *engine.Game, p engine.PlayerID, attacker engine.CardID, blocks []engine.Block, c *engine.ScriptedController) {
	t.Helper()
	g.SetTurnState(g.Turn(), p, engine.DeclareAttackers)
	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	declareAttackers(t, g, ac)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack after attacks: %v", err)
	}
	bc := engine.NewScriptedController()
	bc.QueueBlocks(blocks)
	declareBlockers(t, g, bc)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack after blocks: %v", err)
	}
}

// TestRingTemptsYouMakesTheRingAndTheOnlyCreatureItsBearer proves the
// dominant shape (41 of 49 corpus lines, a bare "DB$ RingTemptsYou" off an
// ETB trigger, Nazgûl's): the Ring tempts the activator, "The Ring" enters
// their Command zone as an effect card, the one creature they control
// becomes their Ring-bearer without a choice, and level 1 makes it
// legendary.
func TestRingTemptsYouMakesTheRingAndTheOnlyCreatureItsBearer(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	host := resolveLine(t, g, p, engine.NewScriptedController(), "DB$ RingTemptsYou")
	ringOf(t, g, p)
	if n := g.RingTemptedYou(p); n != 1 {
		t.Errorf("RingTemptedYou = %d, want 1", n)
	}
	if b := g.RingBearer(p); b != host {
		t.Fatalf("RingBearer = %v, want the only creature %v", b, host)
	}
	if b := g.RingBearer(other); b != engine.NoCard {
		t.Errorf("opponent's RingBearer = %v, want none", b)
	}
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if !g.Card(host).Type().HasSupertype(cardtype.Legendary) {
		t.Error("Ring-bearer is not legendary at level 1")
	}
	if !engine.Matches(g, g.Card(host), valid.Parse("Card.IsRingbearer"), p, host) {
		t.Error("IsRingbearer does not match the Ring-bearer")
	}

	// A second temptation keeps the same Ring card and raises the count.
	ring := ringOf(t, g, p)
	again := engine.NewScriptedController()
	again.QueueCardChoice([]engine.CardID{host})
	resolveLine(t, g, p, again, "DB$ RingTemptsYou")
	if got := ringOf(t, g, p); got != ring {
		t.Errorf("Ring card = %v after a second temptation, want the same card %v", got, ring)
	}
	if n := g.RingTemptedYou(p); n != 2 {
		t.Errorf("RingTemptedYou = %d, want 2", n)
	}
}

// TestRingTemptsYouAsksWhichCreatureAmongSeveral proves the choice: with
// more than one creature the activator picks the Ring-bearer from exactly
// their own creatures, and may move it to another creature next time.
func TestRingTemptsYouAsksWhichCreatureAmongSeveral(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	first := g.NewCard(creatureDef(t), p, engine.Battlefield)
	g.NewCard(creatureDef(t), other, engine.Battlefield)
	g.NewCard(typedDef(t, "Artifact"), p, engine.Battlefield)

	rec := &offerRecorder{ScriptedController: engine.NewScriptedController()}
	rec.QueueCardChoice([]engine.CardID{first})
	resolveLine(t, g, p, rec.ScriptedController, "DB$ RingTemptsYou")
	if b := g.RingBearer(p); b != first {
		t.Fatalf("RingBearer = %v, want the chosen %v", b, first)
	}

	c := &offerRecorder{ScriptedController: engine.NewScriptedController()}
	second := g.NewCard(creatureDef(t), p, engine.Battlefield)
	c.QueueCardChoice([]engine.CardID{second})
	pushAndResolve(t, g, p, c, "DB$ RingTemptsYou")
	if len(c.offers) != 1 {
		t.Fatalf("offered %d times, want once", len(c.offers))
	}
	for _, id := range c.offers[0] {
		if g.Card(id).Controller() != p || !g.Card(id).Type().Has(cardtype.Creature) {
			t.Errorf("offered %v, not a creature p controls", id)
		}
	}
	if b := g.RingBearer(p); b != second {
		t.Errorf("RingBearer = %v, want the newly chosen %v", b, second)
	}
	if engine.Matches(g, g.Card(first), valid.Parse("Card.IsRingbearer"), p, first) {
		t.Error("the previous Ring-bearer still matches IsRingbearer")
	}

	bad := engine.NewScriptedController()
	bad.QueueCardChoice([]engine.CardID{g.Zone(engine.Battlefield, other).Cards()[0]})
	if err := pushAndResolveErr(t, g, p, bad, "DB$ RingTemptsYou"); err == nil {
		t.Error("choosing an opponent's creature as Ring-bearer resolved, want an error")
	}
}

// TestRingTemptsYouWithNoCreatureKeepsTheCount proves Java's
// setRingBearer(null) early return: with no creature, the count still
// rises and nobody becomes the Ring-bearer.
func TestRingTemptsYouWithNoCreatureKeepsTheCount(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	pushAndResolve(t, g, p, engine.NewScriptedController(), "DB$ RingTemptsYou")
	if n := g.RingTemptedYou(p); n != 1 {
		t.Errorf("RingTemptedYou = %d, want 1", n)
	}
	if b := g.RingBearer(p); b != engine.NoCard {
		t.Errorf("RingBearer = %v, want none", b)
	}
	ringOf(t, g, p)
}

// TestRingBearerEndsWhenItLeavesOrChangesControl proves CR 701.54a's
// "until another player gains control of it" and RingTemptsYouEffect's
// leaves-play command: either one ends the designation, and regaining
// control does not bring it back.
func TestRingBearerEndsWhenItLeavesOrChangesControl(t *testing.T) {
	t.Parallel()

	g, p, other, bearer := ringBearerGame(t, 1, "2", "2")
	c := engine.NewScriptedController()
	pushAndResolve(t, g, other, c, "DB$ GainControl | ValidTgts$ Creature", engine.CardEntity(bearer))
	if b := g.RingBearer(p); b != engine.NoCard {
		t.Fatalf("RingBearer = %v after the opponent gained control, want none", b)
	}
	pushAndResolve(t, g, p, c, "DB$ GainControl | ValidTgts$ Creature", engine.CardEntity(bearer))
	if b := g.RingBearer(p); b != engine.NoCard {
		t.Errorf("RingBearer = %v after control came back, want none", b)
	}

	g2, p2, _, bearer2 := ringBearerGame(t, 1, "2", "2")
	g2.Move(bearer2, engine.Graveyard, p2)
	g2.Move(bearer2, engine.Battlefield, p2)
	if b := g2.RingBearer(p2); b != engine.NoCard {
		t.Errorf("RingBearer = %v after it left and came back, want none", b)
	}
}

// TestRingLevelOneStopsGreaterPowerBlockers proves the level-1 CantBlockBy
// (ValidBlockerRelative$ Creature.powerGTX, X = Count$CardPower): the
// Ring-bearer can't be blocked by a creature with greater power, can be by
// one with equal power, and a creature that is not the Ring-bearer is
// unaffected.
func TestRingLevelOneStopsGreaterPowerBlockers(t *testing.T) {
	t.Parallel()

	g, p, other, bearer := ringBearerGame(t, 1, "2", "2")
	bigger := g.NewCard(creatureDefPT(t, "3", "3"), other, engine.Battlefield)
	equal := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	plain := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	if g.CanBlock(bearer, bigger) {
		t.Error("a 3-power creature can block a 2-power Ring-bearer")
	}
	if !g.CanBlock(bearer, equal) {
		t.Error("a 2-power creature cannot block a 2-power Ring-bearer")
	}
	if !g.CanBlock(plain, bigger) {
		t.Error("a 3-power creature cannot block a creature that is not the Ring-bearer")
	}
}

// TestCantBlockByUnresolvedRelativeIsSkipped proves the other side of
// blockerRelativeMatches: a ValidBlockerRelative$ this port does not
// evaluate (Space Beleren's Creature.DifferentSector) skips the static,
// rather than making the attacker unblockable by everything.
func TestCantBlockByUnresolvedRelativeIsSkipped(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	resolveLine(t, g, p, engine.NewScriptedController(), "DB$ Effect | StaticAbilities$ Sector",
		"Sector", "Mode$ CantBlockBy | ValidAttacker$ Creature | ValidBlockerRelative$ Creature.DifferentSector")
	attacker := g.NewCard(creatureDef(t), p, engine.Battlefield)
	blocker := g.NewCard(creatureDef(t), other, engine.Battlefield)
	if !g.CanBlock(attacker, blocker) {
		t.Error("an unresolved ValidBlockerRelative$ made the attacker unblockable")
	}
}

// TestRingLevelTwoLootsWhenTheBearerAttacks proves level 2: whenever the
// Ring-bearer attacks, its controller draws a card, then discards a card.
func TestRingLevelTwoLootsWhenTheBearerAttacks(t *testing.T) {
	t.Parallel()

	g, p, _, bearer := ringBearerGame(t, 2, "2", "2")
	top := libraryCards(t, g, p, 1)[0]
	c := engine.NewScriptedController()
	c.QueueDiscardChoice([]engine.CardID{top})
	attackWith(t, g, p, bearer, nil, c)
	if z := g.Card(top).Zone; z != engine.Graveyard {
		t.Errorf("drawn card zone = %v, want Graveyard (drawn, then discarded)", z)
	}
	if n := len(g.Zone(engine.Hand, p).Cards()); n != 0 {
		t.Errorf("hand holds %d cards, want 0", n)
	}
}

// TestRingLevelThreeSacrificesTheBlockerAtEndOfCombat proves level 3:
// whenever the Ring-bearer becomes blocked by a creature, that creature's
// controller sacrifices it at end of combat (a delayed trigger remembering
// TriggeredBlockerLKICopy).
func TestRingLevelThreeSacrificesTheBlockerAtEndOfCombat(t *testing.T) {
	t.Parallel()

	g, p, other, bearer := ringBearerGame(t, 3, "2", "2")
	blocker := g.NewCard(creatureDefPT(t, "0", "4"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueDiscardChoice(libraryCards(t, g, p, 1))
	attackWith(t, g, p, bearer, []engine.Block{{Attacker: bearer, Blocker: blocker}}, c)
	if z := g.Card(blocker).Zone; z != engine.Battlefield {
		t.Fatalf("blocker zone = %v before end of combat, want Battlefield", z)
	}
	g.SetTurnState(g.Turn(), p, engine.CombatDamage)
	g.AdvancePhase(c)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack at end of combat: %v", err)
	}
	if z := g.Card(blocker).Zone; z != engine.Graveyard {
		t.Errorf("blocker zone = %v after end of combat, want Graveyard", z)
	}
	if z := g.Card(bearer).Zone; z != engine.Battlefield {
		t.Errorf("Ring-bearer zone = %v, want Battlefield", z)
	}
}

// TestRingLevelFourDrainsOnCombatDamage proves level 4: whenever the
// Ring-bearer deals combat damage to a player, each opponent loses 3 life.
func TestRingLevelFourDrainsOnCombatDamage(t *testing.T) {
	t.Parallel()

	g, p, other, bearer := ringBearerGame(t, 4, "2", "2")
	c := engine.NewScriptedController()
	c.QueueDiscardChoice(libraryCards(t, g, p, 1))
	attackWith(t, g, p, bearer, nil, c)
	g.SetTurnState(g.Turn(), p, engine.CombatDamage)
	g.DealCombatDamage(c)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack after damage: %v", err)
	}
	if life := g.Player(other).Life; life != 15 {
		t.Errorf("opponent life = %d, want 15 (2 combat damage, then 3 lost)", life)
	}
	if life := g.Player(p).Life; life != 20 {
		t.Errorf("controller life = %d, want 20", life)
	}
}

// TestRingTemptsYouTriggerMode proves Mode$ RingTemptsYou (9 corpus
// lines): ValidPlayer$ against whom the Ring tempted, ValidCard$ against
// the Ring-bearer just chosen (Creature.YouCtrl+Other: "if you chose a
// creature other than CARDNAME"), and TriggerZones$ Graveyard (Ringwraiths).
func TestRingTemptsYouTriggerMode(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(triggerWatcherDef(t, "Mine",
		"Mode$ RingTemptsYou | ValidPlayer$ You | TriggerZones$ Battlefield | Execute$ TrigGain",
		"TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
	g.NewCard(triggerWatcherDef(t, "Other Bearer",
		"Mode$ RingTemptsYou | ValidCard$ Creature.YouCtrl+Other | TriggerZones$ Battlefield | Execute$ TrigGain",
		"TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 10"), p, engine.Battlefield)
	g.NewCard(triggerWatcherDef(t, "From Graveyard",
		"Mode$ RingTemptsYou | ValidPlayer$ You | TriggerZones$ Graveyard | Execute$ TrigGain",
		"TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 100"), p, engine.Graveyard)
	g.NewCard(triggerWatcherDef(t, "Theirs",
		"Mode$ RingTemptsYou | ValidPlayer$ You | TriggerZones$ Battlefield | Execute$ TrigGain",
		"TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 1000"), other, engine.Battlefield)

	// No creature: ValidCard$ matches nothing.
	pushAndResolve(t, g, p, engine.NewScriptedController(), "DB$ RingTemptsYou")
	if got := g.Player(p).Life; got != 121 {
		t.Errorf("life = %d after a temptation with no creature, want 121 (1 + 100, not the ValidCard$ line)", got)
	}
	if got := g.Player(other).Life; got != 20 {
		t.Errorf("opponent life = %d, want 20: the Ring tempted p, not them", got)
	}

	g.NewCard(creatureDef(t), p, engine.Battlefield)
	pushAndResolve(t, g, p, engine.NewScriptedController(), "DB$ RingTemptsYou")
	if got := g.Player(p).Life; got != 121+111 {
		t.Errorf("life = %d after choosing another creature, want %d", got, 121+111)
	}
}

// TestRingTemptsYouRejectsUnresolvedShapes proves the rejections, before
// anything happens: ConditionDefined$, and a RingTemptsYou trigger in play
// whose Execute$ carries a Cost$ (Call of the Ring's PayLife<2>).
func TestRingTemptsYouRejectsUnresolvedShapes(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	err := pushAndResolveErr(t, g, p, engine.NewScriptedController(),
		"DB$ RingTemptsYou | ConditionDefined$ Targeted | ConditionPresent$ Spell.Legendary")
	if err == nil || !strings.Contains(err.Error(), "ConditionDefined$ not resolvable yet") {
		t.Errorf("err = %v, want ConditionDefined$ rejected", err)
	}

	g.NewCard(triggerWatcherDef(t, "Paid Draw",
		"Mode$ RingTemptsYou | ValidCard$ Creature.YouCtrl | TriggerZones$ Battlefield | Execute$ TrigDraw",
		"TrigDraw", "AB$ Draw | Cost$ PayLife<2>"), p, engine.Battlefield)
	g.NewCard(creatureDef(t), p, engine.Battlefield)
	err = pushAndResolveErr(t, g, p, engine.NewScriptedController(), "DB$ RingTemptsYou")
	if err == nil || !strings.Contains(err.Error(), "Cost$ not resolvable yet") {
		t.Errorf("err = %v, want the Cost$ trigger rejected", err)
	}
	if n := g.RingTemptedYou(p); n != 0 {
		t.Errorf("RingTemptedYou = %d after a rejected temptation, want 0", n)
	}
}

// TestRingTemptsYouIgnoresACostTriggerThatCannotFire proves the Cost$
// rejection is narrow: an opponent's Call of the Ring (ValidCard$
// Creature.YouCtrl, read against its own controller) can never fire on
// your temptation, so it does not stop it; your own does, once you have a
// creature it matches.
func TestRingTemptsYouIgnoresACostTriggerThatCannotFire(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(triggerWatcherDef(t, "Their Paid Draw",
		"Mode$ RingTemptsYou | ValidCard$ Creature.YouCtrl | TriggerZones$ Battlefield | Execute$ TrigDraw",
		"TrigDraw", "AB$ Draw | Cost$ PayLife<2>"), other, engine.Battlefield)
	g.NewCard(creatureDef(t), p, engine.Battlefield)
	pushAndResolve(t, g, p, engine.NewScriptedController(), "DB$ RingTemptsYou")
	if n := g.RingTemptedYou(p); n != 1 {
		t.Fatalf("RingTemptedYou = %d, want 1", n)
	}

	g.NewCard(triggerWatcherDef(t, "My Paid Draw",
		"Mode$ RingTemptsYou | ValidCard$ Creature.YouCtrl | TriggerZones$ Battlefield | Execute$ TrigDraw",
		"TrigDraw", "AB$ Draw | Cost$ PayLife<2>"), p, engine.Battlefield)
	if err := pushAndResolveErr(t, g, p, engine.NewScriptedController(), "DB$ RingTemptsYou"); err == nil {
		t.Error("tempting with your own Cost$ trigger able to fire resolved, want an error")
	}
}

// TestRingBearerEndsUnderAStaticControlChange proves the Layer 2 half of
// CR 701.54a: a Mode$ Continuous GainControl$ static handing the
// Ring-bearer to the opponent ends the designation at the next
// state-based-action check (GameAction.checkStaticAbilities'
// controllerChangeZoneCorrection), for good.
func TestRingBearerEndsUnderAStaticControlChange(t *testing.T) {
	t.Parallel()

	g, p, other, bearer := ringBearerGame(t, 1, "2", "2")
	thief := g.NewCard(continuousDef(t, "Test Thief", "Mode$ Continuous | Affected$ Creature | GainControl$ You"), other, engine.Battlefield)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if got := g.Card(bearer).Controller(); got != other {
		t.Fatalf("bearer controller = %v, want the static's controller %v", got, other)
	}
	g.Move(thief, engine.Graveyard, other)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if got := g.Card(bearer).Controller(); got != p {
		t.Fatalf("bearer controller = %v after the static left, want %v", got, p)
	}
	if b := g.RingBearer(p); b != engine.NoCard {
		t.Errorf("RingBearer = %v after control came back, want none: the static change ended it", b)
	}
}

// TestRingStateSurvivesClone proves Game.Clone copies the Ring's state:
// the clone's count and bearer match, and tempting the clone leaves the
// original alone.
func TestRingStateSurvivesClone(t *testing.T) {
	t.Parallel()

	g, p, _, bearer := ringBearerGame(t, 1, "2", "2")
	clone := g.Clone()
	if clone.RingTemptedYou(p) != 1 || clone.RingBearer(p) != bearer {
		t.Fatalf("clone count/bearer = %d/%v, want 1/%v", clone.RingTemptedYou(p), clone.RingBearer(p), bearer)
	}
	pushAndResolve(t, clone, p, engine.NewScriptedController(), "DB$ RingTemptsYou")
	if g.RingTemptedYou(p) != 1 {
		t.Errorf("original count = %d after tempting the clone, want 1", g.RingTemptedYou(p))
	}
}
