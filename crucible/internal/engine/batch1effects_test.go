package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// resolveNow puts a host for trig (etbChainDef) straight onto p's
// battlefield and resolves trig as p's ability, in whatever phase the game
// is in -- the combat-step shape CastSpell's main-phase timing cannot reach.
func resolveNow(t *testing.T, g *engine.Game, p engine.PlayerID, c *engine.ScriptedController, targets []engine.EntityID, trig string, svars ...string) (engine.CardID, error) {
	t.Helper()
	def := etbChainDef(t, "Test Now", trig, svars...)
	host := g.NewCard(def, p, engine.Battlefield)
	face := def.Faces[0]
	for _, sub := range face.Triggers[0].Subs {
		if !strings.EqualFold(sub.Key, "Execute") {
			continue
		}
		api, ok := engine.APIByName(sub.Ability.Name)
		if !ok {
			t.Fatalf("unknown API %q", sub.Ability.Name)
		}
		g.PushAbility(engine.Ability{API: api, Source: host, Controller: p, Params: sub.Ability, Amounts: face.Amounts, Targets: targets})
		return host, g.ResolveStack(engine.NewRegistry(), c)
	}
	t.Fatal("no Execute$")
	return 0, nil
}

func TestGameDrawnEndsTheGameWithNoWinner(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	resolveLine(t, g, p, engine.NewScriptedController(), "DB$ GameDrawn")
	if !g.Over() || g.Player(p).Won || g.Player(other).Won {
		t.Errorf("over=%v won=%v/%v, want a draw", g.Over(), g.Player(p).Won, g.Player(other).Won)
	}
}

func TestBlankLineRunsItsChain(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	resolveLine(t, g, p, engine.NewScriptedController(), "DB$ BlankLine | SubAbility$ DBGain",
		"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ 3")
	if got := g.Player(p).Life; got != 23 {
		t.Errorf("life = %d, want 23", got)
	}
}

func TestRemoveFromGameLeavesNoTrace(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	host := resolveLine(t, g, p, engine.NewScriptedController(), "DB$ RemoveFromGame | Defined$ Self")
	if z := g.Card(host).Zone; z != engine.None {
		t.Errorf("zone = %v, want None", z)
	}
}

func TestReverseTurnOrderWalksSeatsBackwards(t *testing.T) {
	t.Parallel()
	g := newGame(t, "a", "b", "c")
	ps := g.Players()
	g.SetTurnState(1, ps[0], engine.Main1)
	for _, p := range ps {
		g.Player(p).Life = 20
	}
	c := engine.NewScriptedController()
	resolveLine(t, g, ps[0], c, "DB$ ReverseTurnOrder")
	g.SetTurnState(1, ps[0], engine.Cleanup)
	g.AdvancePhase(c)
	if got := g.ActivePlayer(); got != ps[2] {
		t.Errorf("next active = %v, want %v (reversed)", got, ps[2])
	}
}

func TestChangeSpeedClampsBetweenOneAndFour(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	g.Player(p).Speed = 3
	resolveLine(t, g, p, engine.NewScriptedController(), "DB$ ChangeSpeed | Defined$ You | SubAbility$ DBUp",
		"DBUp", "DB$ ChangeSpeed | Defined$ You")
	if got := g.Player(p).Speed; got != 4 {
		t.Errorf("speed = %d, want 4 (capped)", got)
	}
	g.Player(p).Speed = 1
	resolveLine(t, g, p, engine.NewScriptedController(), "DB$ ChangeSpeed | Defined$ You | Mode$ Decrease")
	if got := g.Player(p).Speed; got != 1 {
		t.Errorf("speed = %d, want 1 (floor)", got)
	}
}

func TestEndTurnSkipsToCleanup(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	resolveLine(t, g, p, c, "DB$ EndTurn")
	if got := g.ActivePhase(); got != engine.Cleanup {
		t.Errorf("phase = %v, want Cleanup", got)
	}
}

func TestEndTurnOptionalDeclined(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	c.QueueConfirmEffect(false)
	resolveLine(t, g, p, c, "DB$ EndTurn | Optional$ True | Defined$ You")
	if got := g.ActivePhase(); got != engine.Main1 {
		t.Errorf("phase = %v, want Main1", got)
	}
}

func TestEndCombatPhaseMovesToMain2(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.DeclareAttackers)
	c := engine.NewScriptedController()
	if _, err := resolveNow(t, g, p, c, nil, "DB$ EndCombatPhase"); err != nil {
		t.Fatal(err)
	}
	if got := g.ActivePhase(); got != engine.Main2 {
		t.Errorf("phase = %v, want Main2", got)
	}
	g.SetTurnState(1, p, engine.Main1)
	if _, err := resolveNow(t, g, p, c, nil, "DB$ EndCombatPhase"); err != nil {
		t.Fatal(err)
	}
	if got := g.ActivePhase(); got != engine.Main1 {
		t.Errorf("outside combat phase = %v, want Main1", got)
	}
}

func TestChooseEvenOddDrivesValidProperty(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	two := g.NewCard(creatureDefCost(t, "Two", "1 G"), other, engine.Battlefield)
	one := g.NewCard(creatureDefCost(t, "One", "G"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueBinary(false) // even
	resolveLine(t, g, p, c, "DB$ ChooseEvenOdd | Defined$ You | SubAbility$ DBDestroy",
		"DBDestroy", "DB$ DestroyAll | ValidCards$ Creature.cmcChosenEvenOdd+OppCtrl")
	if g.Card(two).Zone != engine.Graveyard || g.Card(one).Zone != engine.Battlefield {
		t.Errorf("zones two=%v one=%v, want even one destroyed", g.Card(two).Zone, g.Card(one).Zone)
	}
}

func TestChooseDirectionRecordsPick(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	c.QueueBinary(false)
	host := resolveLine(t, g, p, c, "DB$ ChooseDirection")
	if got := g.Card(host).Memory.ChosenDirection(); got != "Right" {
		t.Errorf("direction = %q, want Right", got)
	}
}

func TestGainOwnershipChangesOwnerOnly(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	host := resolveLine(t, g, p, engine.NewScriptedController(), "DB$ GainOwnership | Defined$ Self | DefinedPlayer$ Opponent")
	if c := g.Card(host); c.Owner != other || c.Zone != engine.Battlefield {
		t.Errorf("owner=%v zone=%v, want %v on battlefield", c.Owner, c.Zone, other)
	}
}

func TestExchangeLifeVariantSetsToughness(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	g.Player(p).Life = 7
	host := resolveLine(t, g, p, engine.NewScriptedController(), "DB$ ExchangeLifeVariant | Defined$ You | Mode$ Toughness")
	if got := g.Player(p).Life; got != 1 {
		t.Errorf("life = %d, want 1 (host toughness)", got)
	}
	if got, _ := g.Card(host).Toughness(); got != 7 {
		t.Errorf("toughness = %d, want 7", got)
	}
}

func TestExchangePowerUntilEndOfTurn(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	big := g.NewCard(creatureDefPT(t, "5", "5"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(big)})
	host := resolveLine(t, g, p, c, "DB$ ExchangePower | ValidTgts$ Creature")
	if hp, _ := g.Card(host).Power(); hp != 5 {
		t.Errorf("host power = %d, want 5", hp)
	}
	if bp, _ := g.Card(big).Power(); bp != 1 {
		t.Errorf("target power = %d, want 1", bp)
	}
}

func TestTapOrUntapAllChoosesOnce(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	a := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	b := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	g.Card(b).Tapped = true
	c := engine.NewScriptedController()
	c.QueueBinary(true)
	resolveLine(t, g, p, c, "DB$ TapOrUntapAll | ValidCards$ Creature.OppCtrl")
	if !g.Card(a).Tapped || !g.Card(b).Tapped {
		t.Errorf("tapped a=%v b=%v, want both", g.Card(a).Tapped, g.Card(b).Tapped)
	}
}

func TestAddOrRemoveCounter(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	c.QueueBinary(true)  // add second
	c.QueueBinary(false) // remove on the each-existing pass
	host := resolveLine(t, g, p, c, "DB$ AddOrRemoveCounter | Defined$ Self | CounterType$ P1P1 | CounterNum$ 2 | SubAbility$ DBAgain",
		"DBAgain", "DB$ AddOrRemoveCounter | Defined$ Self | CounterType$ P1P1 | SubAbility$ DBEach",
		"DBEach", "DB$ AddOrRemoveCounter | Defined$ Self | EachExistingCounter$ True | CounterNum$ 3")
	if got := g.Card(host).Counters.Count(engine.P1P1); got != 0 {
		t.Errorf("+1/+1 counters = %d, want 0 (2 put, 1 added, 3 removed)", got)
	}
}

func TestReorderZoneChosenAndRandom(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	x := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Graveyard)
	y := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Graveyard)
	c := engine.NewScriptedController()
	c.QueueCardOrder([]engine.CardID{y, x})
	resolveLine(t, g, p, c, "DB$ ReorderZone | Zone$ Graveyard | Defined$ You")
	if got := g.Zone(engine.Graveyard, p).Cards(); len(got) != 2 || got[0] != y {
		t.Errorf("graveyard = %v, want y first", got)
	}
	resolveLine(t, g, p, engine.NewScriptedController(), "DB$ ReorderZone | Zone$ Graveyard | Defined$ You | Random$ True")
	if n := len(g.Zone(engine.Graveyard, p).Cards()); n != 2 {
		t.Errorf("graveyard size = %d, want 2", n)
	}
}

func TestGainControlVariantCardOwner(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	stolen := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(stolen)})
	resolveLine(t, g, p, c, "DB$ GainControl | ValidTgts$ Creature.OppCtrl")
	if got := g.Card(stolen).Controller(); got != p {
		t.Fatalf("controller after GainControl = %v, want %v", got, p)
	}
	resolveLine(t, g, p, engine.NewScriptedController(), "DB$ GainControlVariant | AllValid$ Creature | ChangeController$ CardOwner")
	if got := g.Card(stolen).Controller(); got != other {
		t.Errorf("controller = %v, want owner %v", got, other)
	}
}

func TestBlockAndBecomesBlocked(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	atk := g.NewCard(creatureDefPT(t, "3", "3"), p, engine.Battlefield)
	atk2 := g.NewCard(creatureDefPT(t, "4", "4"), p, engine.Battlefield)
	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{atk, atk2})
	declareAttackers(t, g, ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	declareBlockers(t, g, bc)
	g.SetTurnState(1, p, engine.DeclareBlockers)

	c := engine.NewScriptedController()
	if _, err := resolveNow(t, g, other, c, []engine.EntityID{engine.CardEntity(atk2)},
		"DB$ BecomesBlocked | ValidTgts$ Creature.attacking | SubAbility$ DBBlock",
		"DBBlock", "DB$ Block | DefinedAttacker$ Targeted | DefinedBlocker$ Self"); err != nil {
		t.Fatal(err)
	}
	if got := g.Blocks(); len(got) != 1 || got[0].Attacker != atk2 {
		t.Fatalf("blocks = %v, want the host blocking atk2", got)
	}
	g.DealCombatDamage(engine.NewScriptedController())
	if got := g.Player(other).Life; got != 17 {
		t.Errorf("defender life = %d, want 17 (only the unblocked 3/3 connects)", got)
	}
}

func TestChangeCombatantsAddsAttacker(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.DeclareBlockers)
	extra := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueAttackTarget(engine.PlayerEntity(other))
	if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(extra)},
		"DB$ ChangeCombatants | ValidTgts$ Creature | Attacking$ True"); err != nil {
		t.Fatal(err)
	}
	if got := g.Attackers(); len(got) != 1 || got[0] != extra {
		t.Errorf("attackers = %v, want [%v]", got, extra)
	}
}

func creatureDefCost(t *testing.T, name, cost string) *compile.Card {
	t.Helper()
	def := creatureDefPT(t, "1", "1")
	def.Name = name
	def.Faces[0].ManaCost = mana.MustParse(cost)
	return def
}

func TestBecomesBlockedWithoutBlockerDealsNothing(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	atk := g.NewCard(creatureDefPT(t, "3", "3"), p, engine.Battlefield)
	tramp := g.NewCard(creatureDefPTKeywords(t, "2", "2", "Trample"), p, engine.Battlefield)
	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{atk, tramp})
	declareAttackers(t, g, ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	declareBlockers(t, g, bc)
	g.SetTurnState(1, p, engine.DeclareBlockers)
	if _, err := resolveNow(t, g, other, engine.NewScriptedController(),
		[]engine.EntityID{engine.CardEntity(atk), engine.CardEntity(tramp)},
		"DB$ BecomesBlocked | ValidTgts$ Creature.attacking | TargetMax$ 2"); err != nil {
		t.Fatal(err)
	}
	g.DealCombatDamage(engine.NewScriptedController())
	if got := g.Player(other).Life; got != 18 {
		t.Errorf("life = %d, want 18 (blocked trampler still connects, the other deals nothing)", got)
	}
}
