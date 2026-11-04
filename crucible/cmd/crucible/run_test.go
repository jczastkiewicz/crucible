package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/sim"
	"github.com/jczastkiewicz/crucible/internal/store"
	"github.com/jczastkiewicz/crucible/internal/telemetry"
)

func writeDeckFile(t *testing.T, dir, name, text string) string {
	t.Helper()
	path := filepath.Join(dir, name+".dck")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadSimDeckResolvesNamesAndNamesTheMissing(t *testing.T) {
	t.Parallel()
	db := smokeDB(t)
	dir := t.TempDir()
	ok := writeDeckFile(t, dir, "burn", "[Main]\n2 lightning bolt\n3 Mountain\n[Sideboard]\n5 Island\n")
	d, err := loadSimDeck(db, ok)
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "burn" || len(d.Cards) != 5 || d.Cards[0] != "Lightning Bolt" || d.Cards[4] != "Mountain" {
		t.Errorf("deck %+v, want 2 Lightning Bolt then 3 Mountain, no sideboard", d)
	}
	stickered := writeDeckFile(t, dir, "stickered", "[Main]\n1 Mountain\n[Stickers]\n1 unstable robot dragon\n")
	if d, err := loadSimDeck(db, stickered); err != nil || len(d.Stickers) != 1 || d.Stickers[0] != "Unstable Robot Dragon" {
		t.Errorf("sticker sheets %+v (err %v), want Unstable Robot Dragon", d.Stickers, err)
	}
	led := writeDeckFile(t, dir, "led", "[Main]\n1 Mountain\n[Commander]\n1 krenko, mob boss\n")
	if d, err := loadSimDeck(db, led); err != nil || len(d.Commanders) != 1 || d.Commanders[0] != "Krenko, Mob Boss" {
		t.Errorf("commanders %+v (err %v), want Krenko, Mob Boss", d.Commanders, err)
	}
	bad := writeDeckFile(t, dir, "bad", "[Main]\n1 Not A Real Card\n1 Also Fake\n")
	if _, err := loadSimDeck(db, bad); err == nil || !strings.Contains(err.Error(), "Not A Real Card") || !strings.Contains(err.Error(), "Also Fake") {
		t.Errorf("missing cards: err = %v, want both named", err)
	}
	empty := writeDeckFile(t, dir, "empty", "[Sideboard]\n1 Island\n")
	if _, err := loadSimDeck(db, empty); err == nil {
		t.Error("a deck with no main did not fail")
	}
	if _, err := loadSimDeck(db, filepath.Join(dir, "nope.dck")); err == nil {
		t.Error("a missing file did not fail")
	}
}

func TestDeckPathsExpandsDirectoriesSorted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeDeckFile(t, dir, "b", "[Main]\n1 Island\n")
	writeDeckFile(t, dir, "a", "[Main]\n1 Island\n")
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	solo := writeDeckFile(t, t.TempDir(), "solo", "[Main]\n1 Island\n")
	got, err := deckPaths(dir + "," + solo)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || filepath.Base(got[0]) != "a.dck" || filepath.Base(got[1]) != "b.dck" || got[2] != solo {
		t.Errorf("paths %v", got)
	}
	if _, err := deckPaths(t.TempDir()); err == nil {
		t.Error("an empty directory did not fail")
	}
	if _, err := deckPaths(filepath.Join(dir, "missing")); err == nil {
		t.Error("a missing path did not fail")
	}
}

// run plays the gauntlet, stores it, and report reads it back; both print the
// same matchup line.
func TestRunStoresAGauntletAndReportReadsItBack(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	decks := t.TempDir()
	target := writeDeckFile(t, decks, "reds", "[Main]\n30 Mountain\n10 Lightning Bolt\n")
	vs := writeDeckFile(t, decks, "blues", "[Main]\n40 Island\n")
	out := filepath.Join(t.TempDir(), "run1")
	args := []string{
		"-corpus", filepath.Join(root, "forge-gui", "res", "cardsfolder"),
		"-tokens", filepath.Join(root, "forge-gui", "res", "tokenscripts"),
		"-types", filepath.Join(root, "forge-gui", "res", "lists", "TypeLists.txt"),
		"-target", target, "-vs", vs, "-games", "4", "-turns", "100", "-workers", "2", "-shards", "2", "-out", out,
	}
	if err := runCmd(args); err != nil {
		t.Fatalf("run: %v", err)
	}
	if err := reportCmd([]string{"-run", out}); err != nil {
		t.Fatalf("report: %v", err)
	}
	reader, err := store.Open(out, telemetry.SchemaVersion, telemetry.MetricsVersion)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := telemetry.Summarize(reader)
	if err != nil {
		t.Fatal(err)
	}
	// The controller never plays a land, so a Mountain in hand is castable
	// every turn the probe runs and never blocked.
	mountain := sum.Cards["reds"]["Mountain"]
	if mountain == nil || mountain.PlayableUnusedTurns == 0 || mountain.BlockedTurns != 0 {
		t.Errorf("reds' Mountain %+v, want playable-unused turns and no blocked turns", mountain)
	}
	// The controller never plays a land: every game is mana-screwed and every
	// missed drop is one with a land held.
	if m := sum.Mana["reds"]; m == nil || m.Games != 4 || m.ScrewGames != 4 || m.MissedHeld == 0 || m.MissedNoLand != 0 {
		t.Errorf("reds' mana %+v, want 4 screwed games with land drops held", m)
	}
	// With no land ever played, a Lightning Bolt is blocked on quantity.
	bolt := sum.Cards["reds"]["Lightning Bolt"]
	if bolt == nil || bolt.BlockedTurns == 0 || bolt.Blocks["InsufficientMana"] == 0 {
		t.Errorf("reds' Lightning Bolt %+v, want blocked turns dominated by InsufficientMana", bolt)
	}
	if err := runCmd(args); err == nil {
		t.Error("a second run over the same directory did not fail")
	}
	if err := reportCmd([]string{"-run", filepath.Join(out, "missing")}); err == nil {
		t.Error("report of a missing run did not fail")
	}
	for _, bad := range [][]string{{}, {"-target", target, "-vs", vs}, {"-target", target, "-vs", vs, "-out", out + "x", "-games", "0"}, {"-target", target, "-vs", vs, "-out", out + "y", "-variant", "brawl"}} {
		if err := runCmd(bad); err == nil {
			t.Errorf("run %v: want an error", bad)
		}
	}
	if err := reportCmd(nil); err == nil {
		t.Error("report with no -run did not fail")
	}
}

func TestPrintSummaryNamesMatchupsAndCards(t *testing.T) {
	t.Parallel()
	s := telemetry.Summary{
		Matchups: map[telemetry.MatchupKey]*telemetry.Matchup{
			{A: "A", B: "B"}: {Games: 4, WinsA: 3, WinsB: 1, OnPlayA: 2, OnPlayWinsA: 2, OnDrawA: 2, OnDrawWinsA: 1},
			{A: "A", B: "C"}: {Games: 1, Undecided: 1},
		},
		Cards: map[string]map[string]*telemetry.CardStat{
			"A": {"Bolt": {Tenures: 2, HeldTotal: 6, Cast: 2}, "Wall": {Tenures: 1, HeldTotal: 1, Stuck: 1}},
		},
	}
	var buf bytes.Buffer
	printSummary(&buf, s, 1)
	got := buf.String()
	for _, want := range []string{"A vs B: 4 games, 3-1", "75.0%", "on the play 2/2 won, on the draw 1/2", "A vs C: 1 games, 0-0, 1 undecided", "Bolt"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Wall") {
		t.Errorf("top 1 printed a second card:\n%s", got)
	}
	var out bytes.Buffer
	printOutcomes(&out, []sim.GameResult{{Outcome: sim.Won}, {Outcome: sim.Won}, {Outcome: sim.TurnCap}})
	if !strings.Contains(out.String(), "won") || !strings.Contains(out.String(), "turn cap") {
		t.Errorf("outcomes %q", out.String())
	}
}

func TestParseVariantReadsCommanderOrNothing(t *testing.T) {
	t.Parallel()
	if v, err := parseVariant(""); err != nil || v != 0 {
		t.Errorf(`parseVariant("") = %v, %v; want 0, nil`, v, err)
	}
	if v, err := parseVariant("commander"); err != nil || v != engine.VariantCommander {
		t.Errorf(`parseVariant("commander") = %v, %v; want VariantCommander`, v, err)
	}
	if _, err := parseVariant("brawl"); err == nil {
		t.Error(`parseVariant("brawl"): want an error`)
	}
}
