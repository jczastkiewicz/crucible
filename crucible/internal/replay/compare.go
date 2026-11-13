package replay

import (
	"fmt"
	"slices"
	"strings"
)

// Divergence is the first place two event streams differ (ADR-0063).
type Divergence struct {
	// Anchor is the index of the anchor that differs, or of the anchor that
	// opens the interval that differs; -1 for the interval before the first
	// anchor.
	Anchor int
	// Interval is false when the anchors differ or one stream ended, true
	// when the events between two agreeing anchors differ.
	Interval bool
	// Turn, Active and Step are where it happened, from the anchors both
	// streams agree on: zero and "" before the first turn and step.
	Turn   int
	Active int
	Step   string
	// Last is the last anchor both streams agree on, nil if none.
	Last *Event
	// Java and Go are what one stream has and the other lacks: the differing
	// anchor (none when that stream ended), or the interval's moves and
	// damage one side lacks and its net life and counter changes that
	// differ, each with its net delta.
	Java, Go []Event
}

// String is the divergence for a report.
func (d *Divergence) String() string {
	var b strings.Builder
	what := "anchor"
	if d.Interval {
		what = "events after anchor"
	}
	fmt.Fprintf(&b, "%s %d differ: turn %d, seat %d active, step %s", what, d.Anchor, d.Turn, d.Active, d.Step)
	if d.Last != nil {
		fmt.Fprintf(&b, ", after %s", d.Last)
	}
	for _, side := range []struct {
		name string
		evs  []Event
	}{{"java", d.Java}, {"go", d.Go}} {
		fmt.Fprintf(&b, "\n  %s only:", side.name)
		if len(side.evs) == 0 {
			b.WriteString(" nothing")
		}
		for _, e := range side.evs {
			b.WriteString("\n    " + e.String())
		}
	}
	return b.String()
}

// segment is an anchor and the events after it, up to the next anchor. The
// first segment has no anchor.
type segment struct {
	anchor  *Event
	between []Event
}

func split(evs []Event) []segment {
	segs := []segment{{}}
	for i := range evs {
		if evs[i].Type.Anchor() {
			segs = append(segs, segment{anchor: &evs[i]})
			continue
		}
		last := &segs[len(segs)-1]
		last.between = append(last.between, evs[i])
	}
	return segs
}

// Compare returns the first divergence between java's and goEvents'
// streams, nil when they agree: anchors in order, and between two anchors
// moves and damage as a multiset, life and counters as one net change per
// entity (ADR-0063).
func Compare(java, goEvents []Event) *Divergence {
	js, gs := split(java), split(goEvents)
	d := &Divergence{Anchor: -1}
	for i := 0; i < len(js) || i < len(gs); i++ {
		if i > 0 {
			d.Anchor = i - 1
			ja, ga := anchorAt(js, i), anchorAt(gs, i)
			if ja == nil || ga == nil || ja.key() != ga.key() {
				d.Java, d.Go = optional(ja), optional(ga)
				return d
			}
			switch ja.Type {
			case KindTurn:
				d.Turn, d.Active, d.Step = ja.Turn, ja.Active, ""
			case KindStep:
				d.Active, d.Step = ja.Active, ja.Step
			}
			d.Last = ja
		}
		if jonly, gonly := diffInterval(js[i].between, gs[i].between); len(jonly)+len(gonly) > 0 {
			d.Interval, d.Java, d.Go = true, jonly, gonly
			return d
		}
	}
	return nil
}

func anchorAt(segs []segment, i int) *Event {
	if i >= len(segs) {
		return nil
	}
	return segs[i].anchor
}

func optional(e *Event) []Event {
	if e == nil {
		return nil
	}
	return []Event{*e}
}

// diffInterval is what each interval has that the other lacks, sorted by key
// so a report does not depend on either engine's order within it.
func diffInterval(java, goEvents []Event) (javaOnly, goOnly []Event) {
	type tally struct {
		javaN, goN int
		sample     Event
	}
	byKey := map[string]*tally{}
	add := func(e Event, java bool) {
		k := string(e.Type) + "|" + e.key()
		t := byKey[k]
		if t == nil {
			t = &tally{sample: e}
			byKey[k] = t
		}
		n := 1
		if e.Type.net() {
			n = e.Delta
		}
		if java {
			t.javaN += n
		} else {
			t.goN += n
		}
	}
	for _, e := range java {
		add(e, true)
	}
	for _, e := range goEvents {
		add(e, false)
	}
	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		t := byKey[k]
		if t.javaN == t.goN {
			continue
		}
		if t.sample.Type.net() {
			javaOnly = appendNet(javaOnly, t.sample, t.javaN)
			goOnly = appendNet(goOnly, t.sample, t.goN)
			continue
		}
		for range t.javaN - t.goN {
			javaOnly = append(javaOnly, t.sample)
		}
		for range t.goN - t.javaN {
			goOnly = append(goOnly, t.sample)
		}
	}
	return javaOnly, goOnly
}

func appendNet(out []Event, sample Event, delta int) []Event {
	if delta == 0 {
		return out
	}
	sample.Delta = delta
	return append(out, sample)
}
