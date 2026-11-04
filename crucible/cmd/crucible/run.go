package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/deck"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/sim"
	"github.com/jczastkiewicz/crucible/internal/smoke"
	"github.com/jczastkiewicz/crucible/internal/store"
	"github.com/jczastkiewicz/crucible/internal/telemetry"
)

// runCmd is `crucible run`: the target deck against each opponent deck, games
// played on a worker pool and stored under -out (ADR-0014), then a summary
// printed. The controller is the smoke probe's default-answer one until the
// AI exists (M7), so results measure the pipeline, not the decks.
func runCmd(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	corpus := fs.String("corpus", "../forge-gui/res/cardsfolder", "card script directory")
	tokens := fs.String("tokens", "../forge-gui/res/tokenscripts", "token script directory")
	types := fs.String("types", "../forge-gui/res/lists/TypeLists.txt", "subtype vocabulary")
	target := fs.String("target", "", "the deck under test (.dck)")
	vs := fs.String("vs", "", "opponent decks: .dck files or directories of them, comma-separated")
	games := fs.Int("games", 100, "games per opponent")
	seed := fs.Int64("seed", 1, "base seed; game i plays with SeedFor(seed, i)")
	turns := fs.Int("turns", 60, "turn cap")
	workers := fs.Int("workers", 4, "games played in parallel")
	shards := fs.Int("shards", 4, "shards the rows are written to")
	timeout := fs.Duration("timeout", 2*time.Minute, "wall-clock limit per game before it is reported hung")
	out := fs.String("out", "", "run directory to create (must not exist)")
	top := fs.Int("top", 10, "cards printed per deck")
	variantName := fs.String("variant", "", "game variant: \"commander\" seats each deck's Commander section (ADR-0055); empty for constructed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *target == "" || *vs == "" || *out == "" {
		return fmt.Errorf("run: -target, -vs and -out are required")
	}
	if *games < 1 || *shards < 1 {
		return fmt.Errorf("run: -games and -shards must be at least 1")
	}
	variant, err := parseVariant(*variantName)
	if err != nil {
		return err
	}

	db, err := compile.LoadDB(*corpus, *types)
	if err != nil {
		return err
	}
	tok, err := compile.LoadTokenScripts(*tokens, *types)
	if err != nil {
		return err
	}
	db = db.WithTokens(tok)

	gauntlet, err := buildGauntlet(db, *target, *vs, *games)
	if err != nil {
		return err
	}
	jobs := gauntlet.Jobs()
	run, err := store.Create(*out, store.Manifest{
		RunID: filepath.Base(*out), GitSHA: gitSHA(), SchemaVersion: telemetry.SchemaVersion,
		MetricsVersion: telemetry.MetricsVersion, BaseSeed: *seed, Games: len(jobs), Shards: *shards,
		Gauntlet: gauntlet.Name, Decks: deckInfos(gauntlet), Variant: *variantName,
	})
	if err != nil {
		return err
	}
	start := time.Now()
	results, err := sim.Run(context.Background(), sim.Plan{
		DB: db, Jobs: jobs, NewController: func() engine.PlayerController { return &smoke.Controller{} },
		BaseSeed: *seed, MaxTurns: *turns, GameTimeout: *timeout, Store: run, Variant: variant,
	}, *workers)
	if err != nil {
		return err
	}
	fmt.Printf("%d games in %s, stored in %s\n", len(results), time.Since(start).Round(time.Millisecond), *out)
	printOutcomes(os.Stdout, results)
	reader, err := store.Open(*out, telemetry.SchemaVersion, telemetry.MetricsVersion)
	if err != nil {
		return err
	}
	sum, err := telemetry.Summarize(reader)
	if err != nil {
		return err
	}
	printSummary(os.Stdout, sum, *top)
	return nil
}

// reportCmd is `crucible report`: the summary of a stored run.
func reportCmd(args []string) error {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	runDir := fs.String("run", "", "run directory")
	top := fs.Int("top", 10, "cards printed per deck")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *runDir == "" {
		return fmt.Errorf("report: -run is required")
	}
	reader, err := store.Open(*runDir, telemetry.SchemaVersion, telemetry.MetricsVersion)
	if err != nil {
		return err
	}
	m := reader.Manifest()
	fmt.Printf("run %s: %d games, seed %d, schema %d, metrics %d, git %s\n", m.RunID, m.Games, m.BaseSeed, m.SchemaVersion, m.MetricsVersion, m.GitSHA)
	sum, err := telemetry.Summarize(reader)
	if err != nil {
		return err
	}
	printSummary(os.Stdout, sum, *top)
	return nil
}

// buildGauntlet reads the target deck and the opponent decks and resolves
// every card name against db.
func buildGauntlet(db *compile.DB, target, vs string, games int) (sim.Gauntlet, error) {
	t, err := loadSimDeck(db, target)
	if err != nil {
		return sim.Gauntlet{}, err
	}
	paths, err := deckPaths(vs)
	if err != nil {
		return sim.Gauntlet{}, err
	}
	gt := sim.Gauntlet{Name: t.Name + " gauntlet", Target: t}
	for _, p := range paths {
		d, err := loadSimDeck(db, p)
		if err != nil {
			return sim.Gauntlet{}, err
		}
		gt.Opponents = append(gt.Opponents, sim.Opponent{Deck: d, Games: games})
	}
	return gt, nil
}

// deckPaths expands a comma-separated list of files and directories into the
// .dck files they name, directories sorted.
func deckPaths(list string) ([]string, error) {
	var out []string
	for _, item := range strings.Split(list, ",") {
		item = strings.TrimSpace(item)
		info, err := os.Stat(item)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			out = append(out, item)
			continue
		}
		var found []string
		entries, err := os.ReadDir(item)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) == ".dck" {
				found = append(found, filepath.Join(item, e.Name()))
			}
		}
		sort.Strings(found)
		out = append(out, found...)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("run: no decklists in %q", list)
	}
	return out, nil
}

// loadSimDeck reads a .dck and resolves each main-deck and sticker-sheet name
// to the database's spelling. A name the database lacks is an error naming
// every such card.
func loadSimDeck(db *compile.DB, path string) (sim.Deck, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return sim.Deck{}, err
	}
	d := deck.Parse(strings.TrimSuffix(filepath.Base(path), ".dck"), raw)
	byLower := map[string]string{}
	for _, n := range db.Names() {
		byLower[strings.ToLower(n)] = n
	}
	out := sim.Deck{Name: d.Name}
	var missing []string
	for _, sec := range []struct {
		section deck.Section
		into    *[]string
	}{{deck.Main, &out.Cards}, {deck.Stickers, &out.Stickers}, {deck.Commander, &out.Commanders}} {
		for _, e := range d.Cards(sec.section) {
			name, ok := byLower[strings.ToLower(e.Name)]
			if !ok {
				name, ok = byLower[strings.ToLower(carddb.NormalizeName(e.Name))]
			}
			if !ok {
				missing = append(missing, e.Name)
				continue
			}
			for i := 0; i < e.Count; i++ {
				*sec.into = append(*sec.into, name)
			}
		}
	}
	if len(missing) > 0 {
		return sim.Deck{}, fmt.Errorf("%s: cards not in the database: %s", path, strings.Join(missing, ", "))
	}
	if len(out.Cards) == 0 {
		return sim.Deck{}, fmt.Errorf("%s: no main deck", path)
	}
	return out, nil
}

func deckInfos(gt sim.Gauntlet) []store.DeckInfo {
	decks := []sim.Deck{gt.Target}
	for _, o := range gt.Opponents {
		decks = append(decks, o.Deck)
	}
	infos := make([]store.DeckInfo, len(decks))
	for i, d := range decks {
		sorted := append([]string(nil), d.Cards...)
		sort.Strings(sorted)
		text := strings.Join(sorted, "\n")
		if len(d.Commanders) > 0 {
			// Two lists that differ only in their commander are different
			// decks (ADR-0055); a list with none keeps its old digest.
			cmds := append([]string(nil), d.Commanders...)
			sort.Strings(cmds)
			text += "\n[Commander]\n" + strings.Join(cmds, "\n")
		}
		sum := sha256.Sum256([]byte(text))
		infos[i] = store.DeckInfo{Name: d.Name, Hash: hex.EncodeToString(sum[:8])}
	}
	return infos
}

// gitSHA is the checkout's HEAD, or "unknown" outside a repository.
func gitSHA() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func printOutcomes(w io.Writer, results []sim.GameResult) {
	counts := map[sim.Outcome]int{}
	for _, r := range results {
		counts[r.Outcome]++
	}
	for o := sim.Won; o <= sim.Hung; o++ {
		if counts[o] > 0 {
			_, _ = fmt.Fprintf(w, "  %-9s %d\n", o, counts[o])
		}
	}
}

// printSummary writes the matchups and, per deck, the cards held longest.
func printSummary(w io.Writer, s telemetry.Summary, top int) {
	for _, k := range s.MatchupKeys() {
		m := s.Matchups[k]
		_, _ = fmt.Fprintf(w, "%s vs %s: %d games, %d-%d, %d undecided", k.A, k.B, m.Games, m.WinsA, m.WinsB, m.Undecided)
		if rate, lo, hi, ok := m.WinRate(); ok {
			_, _ = fmt.Fprintf(w, ", %s wins %.1f%% [%.1f, %.1f] (95%% Wilson)", k.A, 100*rate, 100*lo, 100*hi)
		}
		_, _ = fmt.Fprintf(w, "; %s on the play %d/%d won, on the draw %d/%d\n", k.A, m.OnPlayWinsA, m.OnPlayA, m.OnDrawWinsA, m.OnDrawA)
	}
	decks := make([]string, 0, len(s.Cards))
	for d := range s.Cards {
		decks = append(decks, d)
	}
	sort.Strings(decks)
	manaDecks := make([]string, 0, len(s.Mana))
	for d := range s.Mana {
		manaDecks = append(manaDecks, d)
	}
	sort.Strings(manaDecks)
	for _, d := range manaDecks {
		m := s.Mana[d]
		_, _ = fmt.Fprintf(w, "%s mana: %d games, screw %.0f%%, flood %.0f%% (MET-10/11); %d turns, missed land drops %d with none in hand and %d with one held; unused mana %d; curve met %d turns\n",
			d, m.Games, 100*m.ScrewRate(), 100*m.FloodRate(), m.Turns, m.MissedNoLand, m.MissedHeld, m.UnusedMana, m.CurveTurnsMet)
	}
	for _, d := range decks {
		type row struct {
			name string
			stat *telemetry.CardStat
		}
		var rows []row
		for name, st := range s.Cards[d] {
			rows = append(rows, row{name, st})
		}
		sort.Slice(rows, func(i, j int) bool {
			a, b := rows[i].stat.MeanTurnsHeld(), rows[j].stat.MeanTurnsHeld()
			if a != b {
				return a > b
			}
			return rows[i].name < rows[j].name
		})
		if len(rows) > top {
			rows = rows[:top]
		}
		_, _ = fmt.Fprintf(w, "%s: cards held longest (mean turns in hand)\n", d)
		for _, r := range rows {
			_, _ = fmt.Fprintf(w, "  %-32s %5.1f  held %d, cast %d, played %d, discarded %d, in hand at the end %d; blocked turns %d, playable unused turns %d, dead %.0f%%\n",
				r.name, r.stat.MeanTurnsHeld(), r.stat.Tenures, r.stat.Cast, r.stat.Played, r.stat.Discarded, r.stat.Stuck,
				r.stat.BlockedTurns, r.stat.PlayableUnusedTurns, 100*r.stat.DeadRate())
		}
	}
}

// parseVariant reads -variant: empty for a constructed game, "commander"
// for the Commander variant (ADR-0055), anything else an error.
func parseVariant(name string) (engine.Variant, error) {
	switch name {
	case "":
		return 0, nil
	case "commander":
		return engine.VariantCommander, nil
	}
	return 0, fmt.Errorf("run: -variant %q: want \"commander\" or nothing", name)
}
