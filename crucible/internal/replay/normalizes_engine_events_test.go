package replay_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/replay"
)

// fakeCards stands in for a replayed game: cards 1-9 are paired with Java ids
// 100 apart, 7 is a token, 8 a planeswalker, 9 unpaired.
type fakeCards struct{}

func (fakeCards) JavaID(c engine.CardID) (int, bool) { return int(c) + 100, c != 9 }
func (fakeCards) Name(c engine.CardID) string {
	return map[engine.CardID]string{1: "Bolt", 9: "Cat"}[c]
}
func (fakeCards) IsToken(c engine.CardID) bool    { return c == 7 }
func (fakeCards) IsCreature(c engine.CardID) bool { return c != 8 }
func (fakeCards) CounterName(d engine.CounterDetail) string {
	if d == engine.CounterDetailLoyalty {
		return "LOYALTY"
	}
	return "P1P1"
}

func TestNormalizerMapsEngineEvents(t *testing.T) {
	t.Parallel()
	n := replay.NewNormalizer(fakeCards{})
	p1, p2 := engine.PlayerID(1), engine.PlayerID(2)
	for _, e := range []engine.Event{
		{Kind: engine.TurnBegan, Active: p2, Turn: 2},
		{Kind: engine.PhaseBegan, Phase: engine.FirstStrikeDamage, Active: p2},
		{Kind: engine.SpellCast, Actor: p1, Source: 1},
		{Kind: engine.AbilityActivated, Actor: p1, Source: 1},
		{Kind: engine.ZoneChanged, Source: 1, From: engine.Hand, To: engine.Stack},
		{Kind: engine.CardDrawn, Actor: p1, Source: 2},
		{Kind: engine.DamageDealt, Source: 1, Target: engine.PlayerEntity(p2), Amount: 3},
		{Kind: engine.DamageDealt, Source: 1, Target: engine.CardEntity(3), Amount: 3},
		{Kind: engine.DamageDealt, Source: 1, Target: engine.CardEntity(8), Amount: 3},
		{Kind: engine.LifeChanged, Target: engine.PlayerEntity(p2), Amount: -3},
		{Kind: engine.LifeChanged, Target: engine.CardEntity(3), Amount: -3},
		{Kind: engine.CounterChanged, Target: engine.CardEntity(8), Amount: -3, Detail: uint32(engine.CounterDetailLoyalty)},
		{Kind: engine.CounterChanged, Target: engine.PlayerEntity(p1), Amount: 2, Detail: uint32(engine.CounterDetailP1P1)},
		{Kind: engine.ZoneChanged, Source: 7, From: engine.Battlefield, To: engine.Graveyard},
		{Kind: engine.ZoneChanged, Source: 7, From: engine.None, To: engine.Battlefield},
		{Kind: engine.ZoneChanged, Source: 9, From: engine.None, To: engine.Battlefield},
		{Kind: engine.AbilityResolved, Actor: p1, Source: 1},
		{Kind: engine.ManaLost, Actor: p1, Amount: 1},
		{Kind: engine.Phased, Source: 3},
		{Kind: engine.GameEnded, Actor: engine.NoPlayer},
	} {
		n.Emit(e)
	}
	bolt := replay.CardRef(101, "Bolt")
	want := []replay.Event{
		{Type: replay.KindTurn, Turn: 2, Active: 1},
		{Type: replay.KindStep, Step: "COMBAT_FIRST_STRIKE_DAMAGE", Active: 1},
		{Type: replay.KindPush, Card: bolt, Controller: 0},
		{Type: replay.KindMove, Card: bolt, From: "Hand", To: "Stack"},
		{Type: replay.KindDamage, Source: bolt, Target: replay.SeatRef(1), Amount: 3},
		{Type: replay.KindDamage, Source: bolt, Target: replay.CardRef(103, ""), Amount: 3},
		{Type: replay.KindLife, Player: 1, Delta: -3},
		{Type: replay.KindCounter, Target: replay.CardRef(108, ""), Counter: "LOYALTY", Delta: -3},
		{Type: replay.KindCounter, Target: replay.SeatRef(0), Counter: "P1P1", Delta: 2},
		{Type: replay.KindMove, Card: replay.CardRef(107, ""), From: "None", To: "Battlefield"},
		{Type: replay.KindMove, Card: replay.Ref{Seat: -1, GoCard: 9, Name: "Cat"}, From: "None", To: "Battlefield"},
		{Type: replay.KindResolve, Card: bolt},
		{Type: replay.KindEnd, Winner: -1},
	}
	got := n.Events()
	if len(got) != len(want) {
		t.Fatalf("%d events, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("event %d = %s, want %s", i, fmt.Sprintf("%#v", got[i]), fmt.Sprintf("%#v", want[i]))
		}
	}
	if got := n.Events()[10].String(); got != "move unmapped Go card 9 (Cat) None -> Battlefield" {
		t.Errorf("unmapped card = %q", got)
	}
}

func TestNormalizerNamesEveryStep(t *testing.T) {
	t.Parallel()
	n := replay.NewNormalizer(fakeCards{})
	for p := engine.Untap; p <= engine.Cleanup; p++ {
		n.Emit(engine.Event{Kind: engine.PhaseBegan, Phase: p, Active: 1})
	}
	var got []string
	for _, e := range n.Events() {
		got = append(got, e.Step)
	}
	want := []string{"UNTAP", "UPKEEP", "DRAW", "MAIN1", "COMBAT_BEGIN", "COMBAT_DECLARE_ATTACKERS",
		"COMBAT_DECLARE_BLOCKERS", "COMBAT_FIRST_STRIKE_DAMAGE", "COMBAT_DAMAGE", "COMBAT_END", "MAIN2",
		"END_OF_TURN", "CLEANUP"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("steps = %q, want %q", got, want)
	}
}

func TestNormalizerReadsRecordedStream(t *testing.T) {
	t.Parallel()
	// The Java recorder's events read back are what a Go stream normalised
	// to the same shape is compared with: the first turn of the fixture.
	log := readFixture(t)
	var first []replay.Event
	for _, e := range log.Events() {
		if e.Type == replay.KindTurn && e.Turn == 2 {
			break
		}
		first = append(first, e)
	}
	n := replay.NewNormalizer(fakeCards{})
	n.Emit(engine.Event{Kind: engine.TurnBegan, Active: 1, Turn: 1})
	n.Emit(engine.Event{Kind: engine.PhaseBegan, Phase: engine.Untap, Active: 1})
	d := replay.Compare(first, n.Events())
	if d == nil || !d.Interval || d.Anchor != -1 {
		t.Fatalf("want a divergence in the opening hands, got %v", d)
	}
}
