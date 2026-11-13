package replay_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/replay"
)

var (
	bolt    = replay.CardRef(31, "Lightning Bolt")
	cat     = replay.CardRef(157, "Cat Token")
	turn3   = replay.Event{Type: replay.KindTurn, Turn: 3, Active: 0}
	damage  = replay.Event{Type: replay.KindStep, Step: "COMBAT_DAMAGE", Active: 0}
	main2   = replay.Event{Type: replay.KindStep, Step: "MAIN2", Active: 0}
	push    = replay.Event{Type: replay.KindPush, Card: bolt, Controller: 0}
	resolve = replay.Event{Type: replay.KindResolve, Card: bolt}
	end     = replay.Event{Type: replay.KindEnd, Winner: 0}
)

func life(seat, delta int) replay.Event {
	return replay.Event{Type: replay.KindLife, Player: seat, Delta: delta}
}

func hit(src replay.Ref, seat, n int) replay.Event {
	return replay.Event{Type: replay.KindDamage, Source: src, Target: replay.SeatRef(seat), Amount: n}
}

func move(c replay.Ref, from, to string) replay.Event {
	return replay.Event{Type: replay.KindMove, Card: c, From: from, To: to}
}

func counter(target replay.Ref, delta int) replay.Event {
	return replay.Event{Type: replay.KindCounter, Target: target, Counter: "P1P1", Delta: delta}
}

func TestCompareIgnoresOrderAndBatchingBetweenAnchors(t *testing.T) {
	t.Parallel()
	// Java batches the damage step's life loss and orders hits by hash;
	// Go loses life per hit. Names are never compared.
	java := []replay.Event{turn3, damage, hit(cat, 1, 1), hit(bolt, 1, 3), life(1, -4), counter(cat, 1),
		counter(cat, 1), push, move(replay.CardRef(31, ""), "Hand", "Stack"), resolve, end}
	goEvents := []replay.Event{turn3, damage, hit(bolt, 1, 3), life(1, -3), counter(cat, 2), hit(cat, 1, 1),
		life(1, -1), life(0, 2), life(0, -2), push, move(bolt, "Hand", "Stack"), resolve, end}
	if d := replay.Compare(java, goEvents); d != nil {
		t.Errorf("unexpected divergence:\n%s", d)
	}
}

func TestCompareReportsFirstDivergence(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		java, goEvents []replay.Event
		anchor         int
		interval       bool
		step           string
		javaOnly       []string
		goOnly         []string
	}{
		"anchor": {
			java:     []replay.Event{turn3, damage, push},
			goEvents: []replay.Event{turn3, main2, push},
			anchor:   1, step: "",
			javaOnly: []string{"step COMBAT_DAMAGE, seat 0 active"},
			goOnly:   []string{"step MAIN2, seat 0 active"},
		},
		"go stream ends": {
			java:     []replay.Event{turn3, damage, push, resolve},
			goEvents: []replay.Event{turn3, damage, push},
			anchor:   3, step: "COMBAT_DAMAGE",
			javaOnly: []string{"resolve card 31 (Lightning Bolt)"},
		},
		"java stream ends": {
			java:     []replay.Event{turn3},
			goEvents: []replay.Event{turn3, end},
			anchor:   1,
			goOnly:   []string{"end, seat 0 wins"},
		},
		"missing move": {
			java:     []replay.Event{turn3, damage, push, move(bolt, "Hand", "Stack"), move(bolt, "Hand", "Stack")},
			goEvents: []replay.Event{turn3, damage, push, move(bolt, "Hand", "Stack")},
			anchor:   2, interval: true, step: "COMBAT_DAMAGE",
			javaOnly: []string{"move card 31 (Lightning Bolt) Hand -> Stack"},
		},
		"extra damage": {
			java:     []replay.Event{turn3, damage},
			goEvents: []replay.Event{turn3, damage, hit(cat, 1, 1)},
			anchor:   1, interval: true, step: "COMBAT_DAMAGE",
			goOnly: []string{"damage card 157 (Cat Token) -> seat 1 1"},
		},
		"net life": {
			java:     []replay.Event{turn3, damage, life(1, -4), counter(cat, 1)},
			goEvents: []replay.Event{turn3, damage, life(1, -3), counter(cat, -1), counter(cat, 1)},
			anchor:   1, interval: true, step: "COMBAT_DAMAGE",
			javaOnly: []string{"counter card 157 (Cat Token) P1P1 1", "life seat 1 -4"},
			goOnly:   []string{"life seat 1 -3"},
		},
		"before first anchor": {
			java:     []replay.Event{move(cat, "Library", "Hand"), turn3},
			goEvents: []replay.Event{turn3},
			anchor:   -1, interval: true,
			javaOnly: []string{"move card 157 (Cat Token) Library -> Hand"},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := replay.Compare(c.java, c.goEvents)
			if d == nil {
				t.Fatal("no divergence")
			}
			if d.Anchor != c.anchor || d.Interval != c.interval || d.Step != c.step {
				t.Errorf("anchor %d interval %v step %q, want %d %v %q", d.Anchor, d.Interval, d.Step, c.anchor,
					c.interval, c.step)
			}
			if got := texts(d.Java); strings.Join(got, "|") != strings.Join(c.javaOnly, "|") {
				t.Errorf("java only = %q, want %q", got, c.javaOnly)
			}
			if got := texts(d.Go); strings.Join(got, "|") != strings.Join(c.goOnly, "|") {
				t.Errorf("go only = %q, want %q", got, c.goOnly)
			}
		})
	}
}

func texts(evs []replay.Event) []string {
	var out []string
	for _, e := range evs {
		out = append(out, e.String())
	}
	return out
}

func TestDivergenceReport(t *testing.T) {
	t.Parallel()
	d := replay.Compare([]replay.Event{turn3, damage, push, life(1, -3), resolve},
		[]replay.Event{turn3, damage, push, resolve})
	want := "events after anchor 2 differ: turn 3, seat 0 active, step COMBAT_DAMAGE, after push card 31 " +
		"(Lightning Bolt) by seat 0\n  java only:\n    life seat 1 -3\n  go only: nothing"
	if got := d.String(); got != want {
		t.Errorf("report =\n%s\nwant\n%s", got, want)
	}
	d = replay.Compare([]replay.Event{{Type: replay.KindEnd, Winner: -1}}, nil)
	if got := d.String(); !strings.HasPrefix(got, "anchor 0 differ: turn 0") || !strings.Contains(got, "end, draw") {
		t.Errorf("report = %q", got)
	}
}

func TestEventTextNamesEveryKind(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		e    replay.Event
		want string
	}{
		{turn3, "turn 3, seat 0 active"},
		{counter(replay.SeatRef(0), -1), "counter seat 0 P1P1 -1"},
		{replay.Event{Type: "phased"}, `unknown event "phased"`},
	} {
		if got := c.e.String(); got != c.want {
			t.Errorf("%q, want %q", got, c.want)
		}
	}
	for k, anchor := range map[replay.Kind]bool{replay.KindTurn: true, replay.KindEnd: true, replay.KindMove: false,
		replay.KindCounter: false} {
		if k.Anchor() != anchor {
			t.Errorf("%s anchor = %v", k, !anchor)
		}
	}
}

func TestCompareSkipsStepsWhereNothingHappened(t *testing.T) {
	t.Parallel()
	step := func(name string) replay.Event { return replay.Event{Type: replay.KindStep, Step: name} }
	draw := move(cat, "Library", "Hand")
	// Java fires the first turn's draw step and then skips it; Go never
	// begins it.
	java := []replay.Event{turn3, step("UPKEEP"), step("DRAW"), step("MAIN1"), draw, end}
	goEvents := []replay.Event{turn3, step("UPKEEP"), step("MAIN1"), draw, end}
	if d := replay.Compare(java, goEvents); d != nil {
		t.Errorf("unexpected divergence:\n%s", d)
	}
	// A push in a different step still diverges: its step is not idle.
	d := replay.Compare([]replay.Event{turn3, step("UPKEEP"), step("DRAW"), push}, []replay.Event{turn3, step("UPKEEP"), push})
	if d == nil || d.Anchor != 1 || d.Java[0].Step != "DRAW" {
		t.Errorf("divergence = %v", d)
	}
}
