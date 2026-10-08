package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// These tests cover the per-turn ledgers the Count$ heads read
// (castrecord.go): zone entries, life gained, attackers, counters put on
// cards and the dungeon, mana pool, Room door and intensity measures.

// meter reads the amount expr as p's permanent sees it: an ability that makes
// the opponent lose that much life, whose loss is returned.
func meter(t *testing.T, g *engine.Game, p, other engine.PlayerID, expr string, lines ...string) int {
	t.Helper()
	lines = append([]string{"A:AB$ LoseLife | Defined$ Opponent | LifeAmount$ X", "SVar:X:" + expr}, lines...)
	host := g.NewCard(copyTestDef(t, "Meter", "Artifact", "", "", lines...), p, engine.Battlefield)
	g.Player(other).Life = 1000
	mustActivate(t, g, p, engine.NewScriptedController(), host)
	return 1000 - g.Player(other).Life
}

// TestLifeYouGainedThisTurnSumsGainsAndEndsAtCleanup proves
// Count$LifeYouGainedThisTurn (Player.getLifeGainedThisTurn): the total of the
// controller's gains, not the opponent's, reset in the cleanup step.
func TestLifeYouGainedThisTurnSumsGainsAndEndsAtCleanup(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	gain := func(pid engine.PlayerID, n string) {
		host := g.NewCard(copyTestDef(t, "Gainer", "Artifact", "", "", "A:AB$ GainLife | Defined$ You | LifeAmount$ "+n), pid, engine.Battlefield)
		mustActivate(t, g, pid, c, host)
	}
	gain(p, "3")
	gain(p, "4")
	gain(other, "9")
	if got := meter(t, g, p, other, "Count$LifeYouGainedThisTurn"); got != 7 {
		t.Errorf("life gained this turn = %d, want 7", got)
	}
	advanceToCleanup(g, c)
	if got := meter(t, g, p, other, "Count$LifeYouGainedThisTurn"); got != 0 {
		t.Errorf("life gained after cleanup = %d, want 0", got)
	}
}

// TestThisTurnEnteredCountsZoneChangesAsLastKnown proves
// Count$ThisTurnEntered_<Dest>[_from_<Origin>]_<valid>: a creature that died
// counts for the player who controlled it on the battlefield, a creature put
// onto the battlefield counts from the zone it came from, a card placed in a
// zone without a move counts for nothing, and the ledger ends at cleanup.
func TestThisTurnEnteredCountsZoneChangesAsLastKnown(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	mine := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	theirs := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	g.NewCard(creatureDefPT(t, "3", "3"), other, engine.Graveyard)
	g.Move(mine, engine.Graveyard, p)
	g.Move(theirs, engine.Graveyard, other)
	entered := g.NewCard(creatureDefPT(t, "4", "4"), p, engine.Hand)
	g.Move(entered, engine.Battlefield, p)

	for _, tc := range []struct {
		expr string
		want int
	}{
		{"Count$ThisTurnEntered_Graveyard_from_Battlefield_Creature", 2},
		{"Count$ThisTurnEntered_Graveyard_from_Battlefield_Creature.YouCtrl", 1},
		{"Count$ThisTurnEntered_Graveyard_from_Battlefield_Creature.OppCtrl", 1},
		{"Count$ThisTurnEntered_Graveyard_Creature", 2},
		{"Count$ThisTurnEntered_Graveyard_from_Hand_Creature", 0},
		{"Count$ThisTurnEntered_Battlefield_from_Hand_Creature.YouCtrl", 1},
		{"Count$ThisTurnEntered_Battlefield_Creature.YouCtrl/Plus.1", 2},
		{"Count$ThisTurnEntered_Battlefield_Land", 0},
		{"Count$ThisTurnEntered_Battlefield_Creature.ControlledBy You", 1},
		{"Count$ThisTurnEntered_Battlefield_Creature.YouCtrl,Land.YouCtrl", 1},
	} {
		if got := meter(t, g, p, other, tc.expr); got != tc.want {
			t.Errorf("%s = %d, want %d", tc.expr, got, tc.want)
		}
	}

	c := engine.NewScriptedController()
	advanceToCleanup(g, c)
	if got := meter(t, g, p, other, "Count$ThisTurnEntered_Graveyard_from_Battlefield_Creature"); got != 0 {
		t.Errorf("entries after cleanup = %d, want 0", got)
	}
}

// TestThisTurnEnteredRefusesWhatJavaReadsAsNull proves a zone name no zone
// has, a "from" with nothing after it and a shape with a `$` are left
// unresolved: the ability that reads it fails rather than counting zero
// (GO-7).
func TestThisTurnEnteredRefusesWhatJavaReadsAsNull(t *testing.T) {
	t.Parallel()

	for _, expr := range []string{
		"Count$ThisTurnEntered_Nowhere_Creature",
		"Count$ThisTurnEntered_Graveyard_from_Nowhere_Creature",
		"Count$ThisTurnEntered_Graveyard_from_Battlefield",
		"Count$ThisTurnEntered_Graveyard",
		"Count$ThisTurnEntered_Graveyard_from_Battlefield_Dalek$CardPower",
	} {
		g, p, other := newTwoPlayerGame(t)
		host := g.NewCard(copyTestDef(t, "Meter", "Artifact", "", "",
			"A:AB$ LoseLife | Defined$ Opponent | LifeAmount$ X", "SVar:X:"+expr), p, engine.Battlefield)
		_ = other
		if err := activate(g, p, engine.NewScriptedController(), host); err == nil {
			t.Errorf("%s resolved, want an unresolved amount", expr)
		}
	}
}

// TestCreaturesAttackedThisTurnCountsDeclaredAttackers proves
// Count$CreaturesAttackedThisTurn <valid>: the controller's attackers of the
// turn matching the valid string, not the opponent's and not a creature that
// stayed home.
func TestCreaturesAttackedThisTurnCountsDeclaredAttackers(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	a := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	b := g.NewCard(creatureDefPT(t, "3", "3"), p, engine.Battlefield)
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	g.Card(a).SummonSick, g.Card(b).SummonSick = false, false
	g.SetTurnState(1, p, engine.DeclareAttackers)
	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{a, b})
	declareAttackers(t, g, ac)

	for _, tc := range []struct {
		expr string
		want int
	}{
		{"Count$CreaturesAttackedThisTurn Creature.YouCtrl", 2},
		{"Count$CreaturesAttackedThisTurn Creature.OppCtrl", 0},
		{"Count$CreaturesAttackedThisTurn Creature.powerGE3", 1},
		{"Count$CreaturesAttackedThisTurn", -1},
	} {
		host := g.NewCard(copyTestDef(t, "Meter", "Artifact", "", "",
			"A:AB$ LoseLife | Defined$ Opponent | LifeAmount$ X", "SVar:X:"+tc.expr), p, engine.Battlefield)
		before := g.Player(other).Life
		err := activate(g, p, ac, host)
		if tc.want < 0 {
			if err == nil {
				t.Errorf("%s resolved, want an unresolved amount", tc.expr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", tc.expr, err)
		}
		if got := before - g.Player(other).Life; got != tc.want {
			t.Errorf("%s = %d, want %d", tc.expr, got, tc.want)
		}
	}
}

// TestCountersAddedThisTurnSumsByTypePutterAndCard proves
// Count$CountersAddedThisTurn <type> <players> <valid>: the counters of the
// type (Any is every type) that the matching putters put on matching cards,
// the cards read as they were when the counters went on, ending at cleanup.
func TestCountersAddedThisTurnSumsByTypePutterAndCard(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	put := func(pid engine.PlayerID, kind string, n string) {
		host := g.NewCard(copyTestDef(t, "Putter", "Creature Elf", "1", "1",
			"A:AB$ PutCounter | Defined$ Self | CounterType$ "+kind+" | CounterNum$ "+n), pid, engine.Battlefield)
		mustActivate(t, g, pid, c, host)
	}
	put(p, "P1P1", "2")
	put(p, "P1P1", "1")
	put(p, "LORE", "4")
	put(other, "P1P1", "8")

	for _, tc := range []struct {
		expr string
		want int
	}{
		{"Count$CountersAddedThisTurn P1P1 You Creature", 3},
		{"Count$CountersAddedThisTurn P1P1 Player Creature", 11},
		{"Count$CountersAddedThisTurn Any You Creature", 7},
		{"Count$CountersAddedThisTurn P1P1 You Creature.OppCtrl", 0},
		{"Count$CountersAddedThisTurn P1P1 Opponent Creature", 8},
		{"Count$CountersAddedThisTurn LORE You Card.Self", 0},
	} {
		if got := meter(t, g, p, other, tc.expr); got != tc.want {
			t.Errorf("%s = %d, want %d", tc.expr, got, tc.want)
		}
	}
	g.Player(other).Life = 1000
	for _, expr := range []string{"Count$CountersAddedThisTurn P1P1 You", "Count$CountersAddedThisTurn P1P1 Nobody Creature"} {
		host := g.NewCard(copyTestDef(t, "Meter", "Artifact", "", "",
			"A:AB$ LoseLife | Defined$ Opponent | LifeAmount$ X", "SVar:X:"+expr), p, engine.Battlefield)
		if err := activate(g, p, c, host); err == nil {
			t.Errorf("%s resolved, want an unresolved amount", expr)
		}
	}
	advanceToCleanup(g, c)
	if got := meter(t, g, p, other, "Count$CountersAddedThisTurn Any Player Creature"); got != 0 {
		t.Errorf("counters after cleanup = %d, want 0", got)
	}
}

// TestManaPoolAndIntensityHeads proves Count$ManaPool:<All|color> over the
// controller's own pool and Count$Intensity over the host's intensity plus
// its Starting intensity keyword.
func TestManaPoolAndIntensityHeads(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.Player(p).ManaPool.Add(mana.Green, 2)
	g.Player(p).ManaPool.AddColorless(1)
	g.Player(other).ManaPool.Add(mana.Green, 5)
	if got := meter(t, g, p, other, "Count$ManaPool:All"); got != 3 {
		t.Errorf("ManaPool:All = %d, want 3", got)
	}
	if got := meter(t, g, p, other, "Count$ManaPool:green"); got != 2 {
		t.Errorf("ManaPool:green = %d, want 2", got)
	}
	host := g.NewCard(copyTestDef(t, "Meter", "Artifact", "", "", "K:Starting intensity:2",
		"A:AB$ LoseLife | Defined$ Opponent | LifeAmount$ X", "SVar:X:Count$Intensity/Plus.1"), p, engine.Battlefield)
	g.Card(host).Intensity = 3
	before := g.Player(other).Life
	mustActivate(t, g, p, engine.NewScriptedController(), host)
	if got := before - g.Player(other).Life; got != 6 {
		t.Errorf("Intensity/Plus.1 = %d, want 3 + 2 + 1", got)
	}
	bad := g.NewCard(copyTestDef(t, "Meter", "Artifact", "", "",
		"A:AB$ LoseLife | Defined$ Opponent | LifeAmount$ X", "SVar:X:Count$ManaPool:plaid"), p, engine.Battlefield)
	if err := activate(g, p, engine.NewScriptedController(), bad); err == nil {
		t.Error("ManaPool:plaid resolved, want an unresolved amount")
	}
}

// TestUnlockedDoorsCountsEveryRoomDoorTheControllerHas proves
// Count$UnlockedDoors (Player.getUnlockedDoors: one entry per unlocked door
// of the Rooms the player controls) and Count$DistinctUnlockedDoors (its
// distinct names).
func TestUnlockedDoorsCountsEveryRoomDoorTheControllerHas(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	castRoom(t, g, p, testRoomDef(t), engine.DoorRight)
	castRoom(t, g, p, testRoomDef(t), engine.DoorRight)
	if got := meter(t, g, p, other, "Count$UnlockedDoors"); got != 2 {
		t.Errorf("UnlockedDoors = %d, want 2", got)
	}
	if got := meter(t, g, p, other, "Count$DistinctUnlockedDoors"); got != 1 {
		t.Errorf("DistinctUnlockedDoors = %d, want 1 (Right Hall twice)", got)
	}
}

// TestDungeonsCompletedMeasuresTheCompletedList proves
// DungeonsCompleted$Amount/DifferentCardNames/Valid over the dungeons the
// controller has completed (AbilityUtils.java:500-501, handlePaid): none is
// 0 whatever the property, one completed Lost Mine is 1 by every measure.
func TestDungeonsCompletedMeasuresTheCompletedList(t *testing.T) {
	t.Parallel()

	g := newDungeonGame(t)
	p, other := g.Players()[0], g.Players()[1]
	if got := meter(t, g, p, other, "DungeonsCompleted$Amount"); got != 0 {
		t.Errorf("Amount before any dungeon = %d, want 0", got)
	}
	lib := libraryCards(t, g, p, 3)
	sc := engine.NewScriptedController()
	sc.QueueOption(1)
	sc.QueueScry([]engine.CardID{lib[0]}, nil)
	steps := []int{-1, 0, 1, -1}
	for i, choice := range steps {
		if choice >= 0 {
			sc.QueueAbilityChoice([]int{choice})
		}
		if err := resolveWith(t, g, p, sc, "DB$ Venture | Defined$ You"); err != nil {
			t.Fatalf("venture %d: %v", i+1, err)
		}
	}
	if len(g.CompletedDungeons(p)) != 1 {
		t.Fatalf("completed = %v, want one dungeon", g.CompletedDungeons(p))
	}
	g.Player(other).Life = 20
	for _, tc := range []struct {
		expr string
		want int
	}{
		{"DungeonsCompleted$Amount", 1},
		{"DungeonsCompleted$DifferentCardNames/Twice", 2},
		{"DungeonsCompleted$Valid Card.namedLost Mine of Phandelver", 1},
		{"DungeonsCompleted$Valid Card.namedTomb of Annihilation", 0},
		{"DungeonsCompleted$Bogus", -1},
	} {
		host := g.NewCard(copyTestDef(t, "Meter", "Artifact", "", "",
			"A:AB$ LoseLife | Defined$ Opponent | LifeAmount$ X", "SVar:X:"+tc.expr), p, engine.Battlefield)
		before := g.Player(other).Life
		err := activate(g, p, sc, host)
		if tc.want < 0 {
			if err == nil {
				t.Errorf("%s resolved, want an unresolved amount", tc.expr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", tc.expr, err)
		}
		if got := before - g.Player(other).Life; got != tc.want {
			t.Errorf("%s = %d, want %d", tc.expr, got, tc.want)
		}
	}
}

// TestLedgersSurviveCloneAndStayIndependent proves Game.Clone copies the turn's
// ledgers: the copy counts what the original had, and entries added to one
// are not seen by the other.
func TestLedgersSurviveCloneAndStayIndependent(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	dying := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	g.Move(dying, engine.Graveyard, p)
	g.Player(p).LifeGainedThisTurn = 5
	clone := g.Clone()
	const died = "Count$ThisTurnEntered_Graveyard_from_Battlefield_Creature"
	if got := meter(t, clone, p, other, died); got != 1 {
		t.Errorf("clone counts %d entries, want 1", got)
	}
	if got := meter(t, clone, p, other, "Count$LifeYouGainedThisTurn"); got != 5 {
		t.Errorf("clone life gained = %d, want 5", got)
	}
	again := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	g.Move(again, engine.Graveyard, p)
	if got := meter(t, clone, p, other, died); got != 1 {
		t.Errorf("clone counts %d entries after the original grew, want 1", got)
	}
	if got := meter(t, g, p, other, died); got != 2 {
		t.Errorf("original counts %d entries, want 2", got)
	}
}
