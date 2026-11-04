package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// These tests cover how counters are put: an activation cost's AddCounter
// part (CostPutCounter.java:182, :201-207) goes through the AddCounter
// replacements as a cost, not an effect, and fires CounterAdded like any other
// placement; ReplaceCounter's ValidSource$ reads the player placing them.

// activateOrFail activates host's ability index for p and resolves the stack.
func activateOrFail(t *testing.T, g *engine.Game, p engine.PlayerID, c engine.PlayerController, host engine.CardID, index int) {
	t.Helper()
	if !g.ActivateAbility(p, host, index, c) {
		t.Fatalf("ActivateAbility failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
}

// TestLoyaltyCostIsReplacedOnlyByCostReplacements proves Ajani Goldmane's +1
// beside Doubling Season, whose counter line is EffectOnly$, raises loyalty
// by one, and beside Vorinclex, Monstrous Raider, whose line is not, by two
// (ReplaceAddCounter.java:30-35).
func TestLoyaltyCostIsReplacedOnlyByCostReplacements(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		doubler string
		want    int
	}{
		{"Doubling Season", 1},
		{"Vorinclex, Monstrous Raider", 2},
	} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		g.NewCard(corpusCard(t, tc.doubler), p, engine.Battlefield)
		ajani := g.NewCard(corpusCard(t, "Ajani Goldmane"), p, engine.Battlefield)
		before := g.Card(ajani).Counters.Count(engine.Loyalty)
		activateOrFail(t, g, p, engine.NewScriptedController(), ajani, 0)
		if got := g.Card(ajani).Counters.Count(engine.Loyalty) - before; got != tc.want {
			t.Errorf("with %s: loyalty raised by %d, want %d", tc.doubler, got, tc.want)
		}
	}
}

// TestCounterCostRunsReplacementsAndTriggers proves Devoted Druid's "put a
// -1/-1 counter on Devoted Druid: untap": Vizier of Remedies puts one fewer,
// so the Druid untaps with no counter, and without it the counter goes on and
// a CounterAddedOnce trigger fires.
func TestCounterCostRunsReplacementsAndTriggers(t *testing.T) {
	t.Parallel()

	for _, vizier := range []bool{true, false} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		if vizier {
			g.NewCard(corpusCard(t, "Vizier of Remedies"), p, engine.Battlefield)
		}
		g.NewCard(planarDef(t, "Test Watcher", "Enchantment",
			[]string{"Mode$ CounterAddedOnce | ValidCard$ Creature.YouCtrl | CounterType$ M1M1 | TriggerZones$ Battlefield | Execute$ TrigGain"},
			nil, "TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
		druid := g.NewCard(corpusCard(t, "Devoted Druid"), p, engine.Battlefield)
		g.Card(druid).Tapped = true
		life := g.Player(p).Life
		activateOrFail(t, g, p, engine.NewScriptedController(), druid, 1)
		want, gained := 1, 1
		if vizier {
			want, gained = 0, 0
		}
		if got := g.Card(druid).Counters.Count(engine.M1M1); got != want {
			t.Errorf("vizier %v: -1/-1 counters = %d, want %d", vizier, got, want)
		}
		if got := g.Player(p).Life - life; got != gained {
			t.Errorf("vizier %v: CounterAddedOnce gained %d, want %d", vizier, got, gained)
		}
		if g.Card(druid).Tapped {
			t.Errorf("vizier %v: the Druid stayed tapped", vizier)
		}
	}
}

// TestChosenCounterCostIsACost proves the general payment path: an
// AddCounter<1/P1P1/Creature.YouCtrl> cost puts its counter on the chosen
// creature through Hardened Scales (one more) but not Doubling Season.
func TestChosenCounterCostIsACost(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		replacer string
		want     int
	}{
		{"Hardened Scales", 2},
		{"Doubling Season", 1},
	} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		g.NewCard(corpusCard(t, tc.replacer), p, engine.Battlefield)
		bear := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
		host := g.NewCard(copyTestDef(t, "Feeder", "Artifact", "", "",
			"A:AB$ GainLife | Cost$ AddCounter<1/P1P1/Creature.YouCtrl/a creature you control> | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
		c := engine.NewScriptedController()
		c.QueueCardChoice([]engine.CardID{bear})
		activateOrFail(t, g, p, c, host, 0)
		if got := g.Card(bear).Counters.Count(engine.P1P1); got != tc.want {
			t.Errorf("with %s: +1/+1 counters = %d, want %d", tc.replacer, got, tc.want)
		}
	}
}

// TestReplaceCounterValidSourceReadsThePlacer proves ReplaceCounter's
// ValidSource$ (ReplaceCounterEffect.java:63-65): Vorinclex, Monstrous Raider
// doubles the counters its controller puts and halves, rounded down, the ones
// an opponent puts; Halving Season halves only an opponent's.
func TestReplaceCounterValidSourceReadsThePlacer(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		replacer       string
		mine, opponent int
	}{
		{"Vorinclex, Monstrous Raider", 6, 1},
		{"Halving Season", 3, 1},
	} {
		g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		g.NewCard(corpusCard(t, tc.replacer), p, engine.Battlefield)
		c := engine.NewScriptedController()
		put := func(pid engine.PlayerID) engine.CardID {
			host := g.NewCard(copyTestDef(t, "Putter", "Creature Elf", "1", "1",
				"A:AB$ PutCounter | Defined$ Self | CounterType$ P1P1 | CounterNum$ 3"), pid, engine.Battlefield)
			mustActivate(t, g, pid, c, host)
			return host
		}
		if got := g.Card(put(p)).Counters.Count(engine.P1P1); got != tc.mine {
			t.Errorf("%s: your 3 counters became %d, want %d", tc.replacer, got, tc.mine)
		}
		if got := g.Card(put(other)).Counters.Count(engine.P1P1); got != tc.opponent {
			t.Errorf("%s: an opponent's 3 counters became %d, want %d", tc.replacer, got, tc.opponent)
		}
	}
}

// TestPrintedLoyaltyAndDefenseAreEntryCounters proves a planeswalker's
// printed loyalty and a battle's defense are etbCounter replacements of the
// card's own (CardState.java:595-600), placed by its controller: they enter
// with the permanent, emit CounterChanged from it, are doubled by Vorinclex,
// Monstrous Raider and come back fresh when the card enters again.
func TestPrintedLoyaltyAndDefenseAreEntryCounters(t *testing.T) {
	t.Parallel()

	for _, vorinclex := range []bool{false, true} {
		g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		if vorinclex {
			g.NewCard(corpusCard(t, "Vorinclex, Monstrous Raider"), p, engine.Battlefield)
		}
		walker := g.NewCard(planeswalkerDefLoyalty(t, "5"), p, engine.Hand)
		battle := g.NewCard(battleDefDefense(t, "4"), p, engine.Hand)
		var sink recordingSink
		g.SetSink(&sink)
		c := engine.NewScriptedController()
		c.QueueCardOrder([]engine.CardID{walker, battle})
		c.QueueBattleProtector(other)
		resolveLine(t, g, p, c, "DB$ ChangeZoneAll | ChangeType$ Planeswalker.YouOwn,Battle.YouOwn | Origin$ Hand | Destination$ Battlefield")
		loyalty, defense := 5, 4
		if vorinclex {
			loyalty, defense = 10, 8
		}
		if got := g.Card(walker).Counters.Count(engine.Loyalty); got != loyalty {
			t.Errorf("vorinclex %v: loyalty = %d, want %d", vorinclex, got, loyalty)
		}
		if got := g.Card(battle).Counters.Count(engine.Defense); got != defense {
			t.Errorf("vorinclex %v: defense = %d, want %d", vorinclex, got, defense)
		}
		found := false
		for _, e := range sink.events {
			if e.Kind == engine.CounterChanged && e.Source == walker && e.Target == engine.CardEntity(walker) &&
				int(e.Amount) == loyalty && e.Detail == uint32(engine.CounterDetailLoyalty) {
				found = true
			}
		}
		if !found {
			t.Errorf("vorinclex %v: no CounterChanged for the walker's %d loyalty", vorinclex, loyalty)
		}
		if vorinclex {
			continue
		}
		g.Card(walker).Counters.Add(engine.Loyalty, -5)
		g.Move(walker, engine.Graveyard, p)
		resolveLine(t, g, p, c, "DB$ ChangeZoneAll | ChangeType$ Planeswalker.YouOwn | Origin$ Graveyard | Destination$ Battlefield")
		if got := g.Card(walker).Counters.Count(engine.Loyalty); got != 5 {
			t.Errorf("loyalty after entering again = %d, want 5", got)
		}
	}
}
