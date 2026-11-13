package replay_test

import (
	"os"
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/replay"
)

const (
	fixture = "../../testdata/replay/boros-energy-vs-azorius-control-seed1.jsonl"
	// counterFixture has planeswalkers and energy: loyalty and energy
	// counters, combat damage to a planeswalker.
	counterFixture = "../../testdata/replay/jeskai-energy-vs-boros-energy-seed7.jsonl"
	// rakdosFixture replays with every question answered.
	rakdosFixture = "../../testdata/replay/boros-energy-vs-rakdos-midrange-seed3.jsonl"
)

func readFixture(t *testing.T) *replay.Log {
	t.Helper()
	return readLog(t, fixture)
}

func readLog(t *testing.T, path string) *replay.Log {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	log, err := replay.Read(f)
	if err != nil {
		t.Fatal(err)
	}
	return log
}

func TestReadDecodesRecordedGame(t *testing.T) {
	t.Parallel()
	log := readFixture(t)
	h := log.Header
	if h.Version != replay.FormatVersion || h.Seed != 1 || len(h.Seats) != 2 || h.UnresolvedIdentities != 0 {
		t.Fatalf("header = %+v", h)
	}
	if len(h.Seats[0].Cards) == 0 || h.Seats[0].Cards[0].Name == "" || h.Seats[0].Cards[0].Seat != -1 {
		t.Errorf("seat 0 cards = %v", h.Seats[0].Cards)
	}
	if len(log.Records) != h.Records {
		t.Errorf("%d records, header %d", len(log.Records), h.Records)
	}
	if len(log.Records[0].Raw) == 0 {
		t.Error("record 0 has no raw line")
	}
	evs := log.Events()
	kinds := map[replay.Kind]int{}
	for _, e := range evs {
		kinds[e.Type]++
	}
	for _, k := range []replay.Kind{replay.KindTurn, replay.KindStep, replay.KindPush, replay.KindResolve,
		replay.KindEnd, replay.KindMove, replay.KindDamage, replay.KindLife} {
		if kinds[k] == 0 {
			t.Errorf("no %s event in %v", k, kinds)
		}
	}
	if kinds[replay.KindTurn] != h.Turns {
		t.Errorf("%d turn events, header %d turns", kinds[replay.KindTurn], h.Turns)
	}
	last := evs[len(evs)-1]
	if last.Type != replay.KindEnd || last.Winner != h.Winner {
		t.Errorf("last event %v, header winner %d", last, h.Winner)
	}
	if d := replay.Compare(evs, evs); d != nil {
		t.Errorf("a stream diverges from itself:\n%s", d)
	}
}

func TestReadRefusesBadLogs(t *testing.T) {
	t.Parallel()
	ev := `{"seq":0,"kind":"event","turn":1,"event":{"type":"turn","turn":1,"active":0}}`
	cases := map[string]struct{ log, want string }{
		"empty":          {"", "empty log"},
		"header":         {"{", "header"},
		"version 1":      {`{"version":1}`, "log version 1"},
		"event failures": {`{"version":2,"eventFailures":2}`, "failed to write 2 events"},
		"record":         {`{"version":2,"records":1}` + "\n{", "line 2"},
		"sequence":       {`{"version":2,"records":1}` + "\n" + `{"seq":3,"kind":"call"}`, "out of sequence"},
		"no event":       {`{"version":2,"records":1}` + "\n" + `{"seq":0,"kind":"event"}`, "no event"},
		"truncated":      {`{"version":2,"records":2}` + "\n" + ev, "1 records, the header says 2"},
		"bad entity":     {`{"version":2,"records":1}` + "\n" + `{"seq":0,"kind":"event","event":{"type":"move","card":{}}}`, "neither a card nor a seat"},
		"entity syntax":  {`{"version":2,"records":1}` + "\n" + `{"seq":0,"kind":"event","event":{"type":"move","card":3}}`, "line 2"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := replay.Read(strings.NewReader(c.log))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want %q", err, c.want)
			}
		})
	}
}

func TestReadAcceptsNullEntity(t *testing.T) {
	t.Parallel()
	log, err := replay.Read(strings.NewReader(`{"version":2,"records":1}` + "\n" +
		`{"seq":0,"kind":"event","event":{"type":"damage","source":null,"target":{"seat":1},"amount":2}}`))
	if err != nil {
		t.Fatal(err)
	}
	e := log.Events()[0]
	if got := e.String(); got != "damage none -> seat 1 2" {
		t.Errorf("event = %q", got)
	}
}

func TestReadDecodesCounters(t *testing.T) {
	t.Parallel()
	evs := readLog(t, counterFixture).Events()
	byCounter := map[string]int{}
	for i, e := range evs {
		if e.Type != replay.KindCounter {
			continue
		}
		byCounter[e.Counter]++
		if e.Counter == "ENERGY" && e.Target.Seat < 0 {
			t.Errorf("energy on a card: %v", e)
		}
		// Combat damage to a planeswalker is its loyalty loss alone, as
		// Normalizer drops Go's DamageDealt to a non-creature.
		if e.Counter == "LOYALTY" && e.Delta < 0 && i > 0 && evs[i-1].Type == replay.KindDamage &&
			evs[i-1].Target == e.Target {
			t.Errorf("planeswalker damage event before %v", e)
		}
	}
	if byCounter["LOYALTY"] == 0 || byCounter["ENERGY"] == 0 {
		t.Errorf("counters = %v, want LOYALTY and ENERGY", byCounter)
	}
}
