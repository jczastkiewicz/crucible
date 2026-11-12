package telemetry_test

import (
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/store"
	"github.com/jczastkiewicz/crucible/internal/telemetry"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

func TestWilsonIntervalBracketsTheRate(t *testing.T) {
	t.Parallel()
	if _, _, _, ok := telemetry.Wilson(0, 0); ok {
		t.Error("no trials reported ok")
	}
	rate, lo, hi, ok := telemetry.Wilson(50, 100)
	if !ok || rate != 0.5 || !(lo < 0.5 && hi > 0.5) || math.Abs((hi-lo)-0.19) > 0.02 {
		t.Errorf("50/100 = %v [%v, %v], want 0.5 with a ~0.19 wide interval", rate, lo, hi)
	}
	if _, lo, hi, _ := telemetry.Wilson(10, 10); hi > 1 || lo < 0.6 {
		t.Errorf("10/10 interval [%v, %v] left [0,1] or is too wide", lo, hi)
	}
	if _, lo, _, _ := telemetry.Wilson(0, 10); lo != 0 {
		t.Errorf("0/10 low end %v, want 0", lo)
	}
}

func writeRun(t *testing.T, rows func(*store.Shard)) *store.Reader {
	t.Helper()
	dir := t.TempDir()
	run, err := store.Create(dir, store.Manifest{RunID: "r", SchemaVersion: telemetry.SchemaVersion, MetricsVersion: telemetry.MetricsVersion, Shards: 1})
	if err != nil {
		t.Fatal(err)
	}
	sh, _ := run.Shard(0)
	rows(sh)
	if err := sh.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := store.Open(dir, telemetry.SchemaVersion, telemetry.MetricsVersion)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSummarizeCountsMatchupsBySeatAndTurnOrder(t *testing.T) {
	t.Parallel()
	r := writeRun(t, func(sh *store.Shard) {
		// B sits first in game 1: the key is still (A, B), A in seat 2.
		_ = sh.Write(telemetry.KindGame, telemetry.GameRow{Game: 0, Decks: [2]string{"A", "B"}, OnPlay: 1, Outcome: "won", Winner: 1, Turns: 9})
		_ = sh.Write(telemetry.KindGame, telemetry.GameRow{Game: 1, Decks: [2]string{"B", "A"}, OnPlay: 1, Outcome: "won", Winner: 1, Turns: 9})
		_ = sh.Write(telemetry.KindGame, telemetry.GameRow{Game: 2, Decks: [2]string{"A", "B"}, OnPlay: 2, Outcome: "won", Winner: 1, Turns: 9})
		_ = sh.Write(telemetry.KindGame, telemetry.GameRow{Game: 3, Decks: [2]string{"B", "A"}, OnPlay: 2, Outcome: "turn cap", Turns: 30})
		_ = sh.Write(telemetry.KindTenure, telemetry.TenureRow{Game: 0, Seat: 1, Card: "Bolt", TurnsHeld: 2, Exit: "cast"})
		_ = sh.Write(telemetry.KindTenure, telemetry.TenureRow{Game: 0, Seat: 1, Card: "Bolt", TurnsHeld: 4, Exit: "end"})
		_ = sh.Write(telemetry.KindTenure, telemetry.TenureRow{Game: 0, Seat: 2, Card: "Wall", TurnsHeld: 1, Exit: "played"})
		_ = sh.Write(telemetry.KindTenure, telemetry.TenureRow{Game: 1, Seat: 2, Card: "Bolt", TurnsHeld: 0, Exit: "discarded"})
	})
	s, err := telemetry.Summarize(r)
	if err != nil {
		t.Fatal(err)
	}
	keys := s.MatchupKeys()
	if len(keys) != 1 || keys[0] != (telemetry.MatchupKey{A: "A", B: "B"}) {
		t.Fatalf("matchups %v, want one A vs B", keys)
	}
	m := s.Matchups[keys[0]]
	// A won game 0 (seat 1) and game 3 was undecided; B won game 1 (seat 1);
	// A won game 2 (seat 1).
	if m.Games != 4 || m.WinsA != 2 || m.WinsB != 1 || m.Undecided != 1 {
		t.Errorf("matchup %+v, want 4 games, A 2, B 1, 1 undecided", *m)
	}
	if m.OnPlayA != 2 || m.OnPlayWinsA != 1 || m.OnDrawA != 2 || m.OnDrawWinsA != 1 {
		t.Errorf("play/draw %+v, want A on the play twice winning once, on the draw twice winning once", *m)
	}
	if rate, _, _, ok := m.WinRate(); !ok || math.Abs(rate-2.0/3.0) > 1e-9 {
		t.Errorf("win rate %v", rate)
	}
	bolt := s.Cards["A"]["Bolt"]
	// Game 1 seats B first, so its seat-2 tenure is A's.
	if bolt.Tenures != 3 || bolt.Cast != 1 || bolt.Stuck != 1 || bolt.Discarded != 1 || bolt.MeanTurnsHeld() != 2 {
		t.Errorf("A's Bolt %+v", *bolt)
	}
	if s.Cards["B"]["Wall"].Played != 1 || s.Cards["B"]["Bolt"] != nil {
		t.Errorf("B's cards %+v", s.Cards["B"])
	}
	if (telemetry.CardStat{}).MeanTurnsHeld() != 0 {
		t.Error("an empty stat has a mean")
	}
}

func TestSummarizeRefusesWhatItCannotPlace(t *testing.T) {
	t.Parallel()
	orphan := writeRun(t, func(sh *store.Shard) {
		_ = sh.Write(telemetry.KindTenure, telemetry.TenureRow{Game: 9, Seat: 1, Card: "X"})
	})
	if _, err := telemetry.Summarize(orphan); err == nil {
		t.Error("a tenure with no game row did not fail")
	}
	unknown := writeRun(t, func(sh *store.Shard) { _ = sh.Write("mystery", struct{}{}) })
	if _, err := telemetry.Summarize(unknown); err == nil {
		t.Error("an unknown row kind did not fail")
	}
	bad := writeRun(t, func(sh *store.Shard) { _ = sh.Write(telemetry.KindGame, "not a row") })
	if _, err := telemetry.Summarize(bad); err == nil {
		t.Error("a game row that does not decode did not fail")
	}
}

func TestSortedCards(t *testing.T) {
	t.Parallel()
	got := telemetry.SortedCards([]telemetry.TenureRow{{Card: "b"}, {Card: "a"}, {Card: "b"}})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("cards %v", got)
	}
}

func TestRecorderNamesHowACardEnteredAndLeftTheHand(t *testing.T) {
	t.Parallel()
	cards := map[string]*compile.Card{}
	for _, n := range []string{"C"} {
		c, err := compile.Compile(&carddb.Card{Filename: n, Faces: [carddb.NumFaces]carddb.Face{{Present: true, Name: n}}})
		if err != nil {
			t.Fatal(err)
		}
		cards[n] = c
	}
	db := compile.NewDB(cards)
	g := engine.NewGame(db, javarand.New(1), []string{"a", "b"})
	p := g.Players()[0]
	rec := telemetry.NewRecorder(g, 4)
	move := func(id engine.CardID, from, to engine.ZoneType, turn uint16) {
		rec.Emit(engine.Event{Kind: engine.ZoneChanged, Source: id, Actor: p, From: from, To: to, Turn: turn})
	}
	def := cards["C"]
	var ids []engine.CardID
	for i := 0; i < 8; i++ {
		ids = append(ids, g.NewCard(def, p, engine.Library))
	}
	move(ids[0], engine.Library, engine.Hand, 0) // opening
	move(ids[1], engine.Library, engine.Hand, 2) // draw
	move(ids[2], engine.Battlefield, engine.Hand, 3)
	for i, to := range []engine.ZoneType{engine.Stack, engine.Battlefield, engine.Graveyard, engine.Exile, engine.Library, engine.Command} {
		id := ids[3+i%5]
		move(id, engine.Library, engine.Hand, 1)
		move(id, engine.Hand, to, 5)
	}
	move(ids[7], engine.Graveyard, engine.Battlefield, 5) // not a hand move
	move(ids[7], engine.Hand, engine.Hand, 5)             // no change
	move(ids[7], engine.Hand, engine.Graveyard, 5)        // left a hand it never entered
	rec.Emit(engine.Event{Kind: engine.TurnBegan})        // not a zone change
	rows := rec.Finish(9)
	var exits, sources []string
	for _, r := range rows {
		exits = append(exits, r.Exit)
		sources = append(sources, r.Source)
		if r.Game != 4 || r.Seat != 1 || r.Card != "C" {
			t.Errorf("row %+v: want game 4, seat 1, card C", r)
		}
	}
	wantExits := []string{"cast", "played", "discarded", "exiled", "tucked", "other", "end", "end", "end"}
	if !slices.Equal(exits, wantExits) {
		t.Errorf("exits %v, want %v", exits, wantExits)
	}
	if sources[len(sources)-3] != "opening" || sources[len(sources)-2] != "draw" || sources[len(sources)-1] != "other" {
		t.Errorf("sources of the cards held to the end %v, want opening, draw, other", sources[len(sources)-3:])
	}
	if rows[len(rows)-3].TurnsHeld != 9 {
		t.Errorf("the opening card was held %d turns, want 9", rows[len(rows)-3].TurnsHeld)
	}
}

// At the start of the active player's combat phase the recorder runs the
// castability probe and tallies it against every card still in that player's
// hand (MET-5, MET-7). A blank card costs nothing and is castable, so each
// probe adds a playable-unused turn and no blocked one.
func TestRecorderTalliesTheProbeAgainstOpenTenures(t *testing.T) {
	t.Parallel()
	c, err := compile.Compile(&carddb.Card{Filename: "C", Faces: [carddb.NumFaces]carddb.Face{{Present: true, Name: "C"}}})
	if err != nil {
		t.Fatal(err)
	}
	g := engine.NewGame(compile.NewDB(map[string]*compile.Card{"C": c}), javarand.New(1), []string{"a", "b"})
	p := g.Players()[0]
	g.SetTurnState(2, p, engine.CombatBegin)
	id := g.NewCard(c, p, engine.Hand)
	rec := telemetry.NewRecorder(g, 0)
	g.SetSink(rec)
	rec.Emit(engine.Event{Kind: engine.ZoneChanged, Source: id, Actor: p, From: engine.Library, To: engine.Hand, Turn: 1})
	rec.Emit(engine.Event{Kind: engine.PhaseBegan, Phase: engine.CombatBegin, Active: p, Turn: 2})
	rec.Emit(engine.Event{Kind: engine.PhaseBegan, Phase: engine.CombatBegin, Active: p, Turn: 3})
	rec.Emit(engine.Event{Kind: engine.PhaseBegan, Phase: engine.Main1, Active: p, Turn: 3}) // not a probe point
	rows := rec.Finish(4)
	if len(rows) != 1 || rows[0].TurnsPlayableUnused != 2 || rows[0].TurnsBlocked != 0 || len(rows[0].BlockedBy) != 0 || rows[0].DominantBlock != "" {
		t.Fatalf("rows %+v, want one tenure playable-unused two turns and never blocked", rows)
	}
	// Nothing open: the probe has nobody to tally and does not run.
	empty := telemetry.NewRecorder(g, 1)
	empty.Emit(engine.Event{Kind: engine.PhaseBegan, Phase: engine.CombatBegin, Active: p})
	if len(empty.Finish(1)) != 0 {
		t.Error("a probe with no open tenure made rows")
	}
}

func TestSummarizeFoldsTheProbeColumnsSeparately(t *testing.T) {
	t.Parallel()
	r := writeRun(t, func(sh *store.Shard) {
		_ = sh.Write(telemetry.KindGame, telemetry.GameRow{Game: 0, Decks: [2]string{"A", "B"}, Outcome: "won", Winner: 1})
		_ = sh.Write(telemetry.KindTenure, telemetry.TenureRow{Game: 0, Seat: 1, Card: "X", TurnsBlocked: 3, TurnsPlayableUnused: 1, DominantBlock: "ColorUnavailable", MissingColors: map[string]int{"B": 2, "G": 1}})
		_ = sh.Write(telemetry.KindTenure, telemetry.TenureRow{Game: 0, Seat: 1, Card: "X", TurnsBlocked: 1, TurnsPlayableUnused: 4, DominantBlock: "InsufficientMana", MissingColors: map[string]int{"B": 1}})
	})
	s, err := telemetry.Summarize(r)
	if err != nil {
		t.Fatal(err)
	}
	x := s.Cards["A"]["X"]
	if x.BlockedTurns != 4 || x.PlayableUnusedTurns != 5 || x.DeadTenures != 1 || x.DeadRate() != 0.5 {
		t.Errorf("X %+v, want 4 blocked and 5 playable-unused turns, one dead tenure of two", *x)
	}
	if x.Blocks["ColorUnavailable"] != 1 || x.Blocks["InsufficientMana"] != 1 {
		t.Errorf("blocks %v", x.Blocks)
	}
	if x.MissingColors["B"] != 3 || x.MissingColors["G"] != 1 {
		t.Errorf("missing colours %v, want B 3 and G 1", x.MissingColors)
	}
	if (telemetry.CardStat{}).DeadRate() != 0 {
		t.Error("an empty stat has a dead rate")
	}
}

func TestSummarizeFoldsManaHealthPerDeck(t *testing.T) {
	t.Parallel()
	r := writeRun(t, func(sh *store.Shard) {
		_ = sh.Write(telemetry.KindGame, telemetry.GameRow{Game: 0, Decks: [2]string{"A", "B"}, Outcome: "won", Winner: 1})
		// A: one land then stuck on it (screwed on turns 2 and 3).
		for turn, lands := range []int{1, 1, 1} {
			_ = sh.Write(telemetry.KindMana, telemetry.ManaRow{Game: 0, Seat: 1, Turn: 2*turn + 1, LandsInPlay: lands, ManaAvailable: lands, ManaSpent: lands, MissedDrop: map[bool]string{true: "", false: "no_land"}[turn == 0]})
		}
		// B: seven turns, flooding from its sixth turn on.
		for turn := 1; turn <= 7; turn++ {
			row := telemetry.ManaRow{Game: 0, Seat: 2, Turn: 2 * turn, LandsInPlay: turn, ManaAvailable: turn, ManaSpent: min(turn, 3), LandPlayed: true, FloatedLost: turn % 2}
			if turn >= 6 {
				row.LandsInHand = 3
			}
			_ = sh.Write(telemetry.KindMana, row)
		}
	})
	s, err := telemetry.Summarize(r)
	if err != nil {
		t.Fatal(err)
	}
	a, b := s.Mana["A"], s.Mana["B"]
	if a == nil || a.Games != 1 || a.ScrewGames != 1 || a.FloodGames != 0 || a.MissedNoLand != 2 || a.Turns != 3 {
		t.Errorf("A %+v, want one screwed game, two drops missed with no land, three turns", a)
	}
	if b == nil || b.ScrewGames != 0 || b.FloodGames != 1 || a.ScrewRate() != 1 || b.FloodRate() != 1 {
		t.Errorf("B %+v, want one flooded game", b)
	}
	if b.UnusedMana != 0+0+0+1+2+3+4 || b.CurveTurnsMet != 3 {
		t.Errorf("B unused %d curve %d, want 10 and 3", b.UnusedMana, b.CurveTurnsMet)
	}
	if b.FloatedLost != 4 {
		t.Errorf("B floated and lost %d, want 4", b.FloatedLost)
	}
	if (telemetry.ManaStat{}).ScrewRate() != 0 {
		t.Error("an empty mana stat has a rate")
	}
	orphan := writeRun(t, func(sh *store.Shard) {
		_ = sh.Write(telemetry.KindMana, telemetry.ManaRow{Game: 3, Seat: 1})
	})
	if _, err := telemetry.Summarize(orphan); err == nil {
		t.Error("a mana sample with no game row did not fail")
	}
}

// The recorder samples the active player's sources after the untap step and
// closes the sample at their end step (MET-10 to MET-15).
func TestRecorderSamplesManaPerTurn(t *testing.T) {
	t.Parallel()
	land, err := compile.Compile(&carddb.Card{Filename: "L", Faces: [carddb.NumFaces]carddb.Face{{Present: true, Name: "L", Type: cardtype.ParseToken("Basic").Union(cardtype.ParseToken("Land")).Union(cardtype.ParseToken("Forest"))}}})
	if err != nil {
		t.Fatal(err)
	}
	g := engine.NewGame(compile.NewDB(map[string]*compile.Card{"L": land}), javarand.New(1), []string{"a", "b"})
	p := g.Players()[0]
	a := g.NewCard(land, p, engine.Battlefield)
	g.NewCard(land, p, engine.Battlefield)
	g.NewCard(land, p, engine.Hand)
	rec := telemetry.NewRecorder(g, 2)
	rec.Emit(engine.Event{Kind: engine.PhaseBegan, Phase: engine.Upkeep, Active: p, Turn: 3})
	g.Card(a).Tapped = true
	rec.Emit(engine.Event{Kind: engine.PhaseBegan, Phase: engine.EndOfTurn, Active: p, Turn: 3})
	rec.Emit(engine.Event{Kind: engine.PhaseBegan, Phase: engine.EndOfTurn, Active: g.Players()[1], Turn: 4}) // no upkeep sample
	rows := rec.ManaRows()
	if len(rows) != 1 {
		t.Fatalf("rows %+v, want one sample", rows)
	}
	r := rows[0]
	if r.Game != 2 || r.Seat != 1 || r.Turn != 3 || r.LandsInPlay != 2 || r.LandsInHand != 1 || r.ManaAvailable != 2 || r.ManaSpent != 1 {
		t.Errorf("sample %+v, want 2 lands in play, 1 in hand, 2 available, 1 spent", r)
	}
	if r.LandPlayed || r.MissedDrop != "held" || len(r.Colors) != 1 || r.NonLandSources != 0 {
		t.Errorf("sample %+v, want a held land drop and one colour", r)
	}
}

// The active player's lost mana lands on their turn's sample, before the
// end-step sample and after it; another seat's is ignored (MET-13).
func TestRecorderAddsFloatedLostManaToTheTurn(t *testing.T) {
	t.Parallel()
	g := engine.NewGame(compile.NewDB(map[string]*compile.Card{}), javarand.New(1), []string{"a", "b"})
	p, o := g.Players()[0], g.Players()[1]
	rec := telemetry.NewRecorder(g, 0)
	rec.Emit(engine.Event{Kind: engine.PhaseBegan, Phase: engine.Upkeep, Active: p, Turn: 3})
	rec.Emit(engine.Event{Kind: engine.ManaLost, Active: p, Actor: p, Turn: 3, Amount: 2})
	rec.Emit(engine.Event{Kind: engine.ManaLost, Active: p, Actor: o, Turn: 3, Amount: 5})
	rec.Emit(engine.Event{Kind: engine.PhaseBegan, Phase: engine.EndOfTurn, Active: p, Turn: 3})
	rec.Emit(engine.Event{Kind: engine.ManaLost, Active: p, Actor: p, Turn: 3, Amount: 1})
	rows := rec.ManaRows()
	if len(rows) != 1 || rows[0].FloatedLost != 3 {
		t.Fatalf("rows %+v, want one sample with 3 lost", rows)
	}
}

// A card the probe finds short of a colour counts the turn against that
// colour on its tenure (MET-12).
func TestRecorderCountsTheColourACardWaitsOn(t *testing.T) {
	t.Parallel()
	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatal(err)
	}
	cards := map[string]*compile.Card{}
	for name, script := range map[string]string{
		"Blk":   "Name:Blk\nManaCost:B\nTypes:Sorcery\nA:SP$ GainLife | LifeAmount$ 1\n",
		"Woods": "Name:Woods\nTypes:Land\nA:AB$ Mana | Cost$ T | Produced$ G\n",
	} {
		raw, err := carddb.ParseScript(reg, name, []byte(script))
		if err != nil {
			t.Fatal(err)
		}
		if cards[name], err = compile.Compile(raw); err != nil {
			t.Fatal(err)
		}
	}
	g := engine.NewGame(compile.NewDB(cards), javarand.New(1), []string{"a", "b"})
	p := g.Players()[0]
	g.SetTurnState(2, p, engine.CombatBegin)
	g.NewCard(cards["Woods"], p, engine.Battlefield)
	id := g.NewCard(cards["Blk"], p, engine.Hand)
	rec := telemetry.NewRecorder(g, 0)
	g.SetSink(rec)
	rec.Emit(engine.Event{Kind: engine.ZoneChanged, Source: id, Actor: p, From: engine.Library, To: engine.Hand, Turn: 1})
	rec.Emit(engine.Event{Kind: engine.PhaseBegan, Phase: engine.CombatBegin, Active: p, Turn: 2})
	rows := rec.Finish(3)
	if len(rows) != 1 || rows[0].MissingColors["B"] != 1 || len(rows[0].MissingColors) != 1 || rows[0].DominantBlock != "ColorUnavailable" {
		t.Fatalf("rows %+v, want one tenure waiting one turn on black", rows)
	}
}
