// Command l2diff is ADR-0060's Go half of the L2 scenario runner.
//
//	go run ./tools/l2diff emit -out DIR     # Go post-states, manifest.tsv, random.txt
//	crucible/oracle-java/l2-run.sh DIR/manifest.tsv DIR/java
//	go run ./tools/l2diff diff -dir DIR     # report.txt and one .diff per differs
//	go run ./tools/l2diff diff -dir DIR -baseline testdata/l2-baseline.txt
//
// emit plays every testdata/scenarios fixture the way TestScenarios does and
// writes DIR/go/<name>.state (fixture.Dump), DIR/manifest.tsv (the scenario
// directory, Go's final turn and phase and the timed actions file below, for
// the Java runner),
// DIR/actions/<name>.log (each actions.log line with the turn and phase Go
// ran it in, so the runner plays a cast where Go did) and
// DIR/random.txt (the scenarios that drew from the game's generator after
// setup). diff normalises each pair (normalize.go, l2-verbs.md) and sorts the
// scenarios into same, differs, random and not runnable. With -baseline it is
// ADR-0060's phase 2 gate: it fails when the differs list is not exactly the
// baseline's scenarios.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/fixture"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: l2diff emit|diff [flags]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "emit":
		fs := flag.NewFlagSet("emit", flag.ExitOnError)
		scenarios := fs.String("scenarios", "testdata/scenarios", "scenario fixtures")
		res := fs.String("res", "../forge-gui/res", "Forge's res directory")
		out := fs.String("out", "", "output directory")
		only := fs.String("only", "", "comma list of scenario names (default all)")
		_ = fs.Parse(os.Args[2:])
		err = emit(*scenarios, *res, *out, *only)
	case "diff":
		fs := flag.NewFlagSet("diff", flag.ExitOnError)
		dir := fs.String("dir", "", "emit's output directory, with the Java run in java/")
		baseline := fs.String("baseline", "", "fail unless differs is exactly this file's scenarios")
		_ = fs.Parse(os.Args[2:])
		err = diff(*dir, *baseline)
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "l2diff:", err)
		os.Exit(1)
	}
}

func emit(scenarios, res, out, only string) error {
	if out == "" {
		return errors.New("emit: -out is required")
	}
	types := filepath.Join(res, "lists", "TypeLists.txt")
	db, err := compile.LoadDB(filepath.Join(res, "cardsfolder"), types)
	if err != nil {
		return err
	}
	tokens, err := compile.LoadTokenScripts(filepath.Join(res, "tokenscripts"), types)
	if err != nil {
		return err
	}
	db = db.WithTokens(tokens)
	if err := clearDir(filepath.Join(out, "go"), "*.state"); err != nil {
		return err
	}
	if err := clearDir(filepath.Join(out, "actions"), "*.log"); err != nil {
		return err
	}
	entries, err := os.ReadDir(scenarios)
	if err != nil {
		return err
	}
	var manifest, random, failed []string
	for _, e := range entries {
		if !e.IsDir() || only != "" && !slices.Contains(strings.Split(only, ","), e.Name()) {
			continue
		}
		dir, err := filepath.Abs(filepath.Join(scenarios, e.Name()))
		if err != nil {
			return err
		}
		r, err := playGo(dir, db)
		if err != nil {
			failed = append(failed, e.Name()+"\t"+err.Error())
			continue
		}
		if err := os.WriteFile(filepath.Join(out, "go", e.Name()+".state"), r.state, 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, "actions", e.Name()+".log"), []byte(joinLines(r.timed)), 0o644); err != nil {
			return err
		}
		timed, err := filepath.Abs(filepath.Join(out, "actions", e.Name()+".log"))
		if err != nil {
			return err
		}
		manifest = append(manifest, fmt.Sprintf("%s\t%d\t%s\t%s", dir, r.turn, JavaPhase(r.phase), timed))
		if r.random {
			random = append(random, e.Name())
		}
	}
	for name, lines := range map[string][]string{"manifest.tsv": manifest, "random.txt": random, "go-failed.txt": failed} {
		if err := os.WriteFile(filepath.Join(out, name), []byte(joinLines(lines)), 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("emit: %d scenarios, %d random, %d failed in Go\n", len(manifest), len(random), len(failed))
	return nil
}

type goRun struct {
	state  []byte
	turn   int
	phase  string
	random bool
	// timed is each actions.log line as "<turn>\t<Java phase>\t<line>", the
	// point Go ran it at.
	timed []string
}

// playGo is TestScenarios' runScenario without the expect.state compare: the
// post-state, the point Java must stop at, and whether the run drew from the
// seeded generator after setup.
func playGo(dir string, db *compile.DB) (r goRun, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
		}
	}()
	raw, err := os.ReadFile(filepath.Join(dir, "setup.state"))
	if err != nil {
		return r, err
	}
	st, err := fixture.Parse(bytes.NewReader(raw))
	if err != nil {
		return r, err
	}
	rng := javarand.New(1)
	l, err := fixture.Load(st, db, rng)
	if err != nil {
		return r, err
	}
	afterSetup := rng.State()
	actions, err := os.ReadFile(filepath.Join(dir, "actions.log"))
	if err != nil && !os.IsNotExist(err) {
		return r, err
	}
	// One line at a time, the controller's queue carried across, so each
	// line's turn and phase is known.
	c := engine.NewScriptedController()
	for _, line := range strings.Split(string(actions), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		r.timed = append(r.timed, fmt.Sprintf("%d\t%s\t%s", l.Game.Turn(), JavaPhase(l.Game.ActivePhase().String()), line))
		if err := fixture.RunActions(strings.NewReader(line), l, c); err != nil {
			return r, err
		}
	}
	var buf bytes.Buffer
	if err := fixture.Write(&buf, fixture.Dump(l)); err != nil {
		return r, err
	}
	r.state, r.turn, r.phase, r.random = buf.Bytes(), l.Game.Turn(), l.Game.ActivePhase().String(), rng.State() != afterSetup
	return r, nil
}

func diff(dir, baseline string) error {
	if dir == "" {
		return errors.New("diff: -dir is required")
	}
	randomRaw, err := os.ReadFile(filepath.Join(dir, "random.txt"))
	if err != nil {
		return err
	}
	random := strings.Fields(string(randomRaw))
	goFiles, err := filepath.Glob(filepath.Join(dir, "go", "*.state"))
	if err != nil {
		return err
	}
	if err := clearDir(filepath.Join(dir, "diffs"), "*.diff"); err != nil {
		return err
	}
	lists := map[string][]string{}
	var errored []string
	for _, gf := range goFiles {
		name := strings.TrimSuffix(filepath.Base(gf), ".state")
		kind, detail, err := classify(dir, name, gf, slices.Contains(random, name))
		if err != nil {
			return err
		}
		lists[kind] = append(lists[kind], name+detail)
		if strings.HasPrefix(detail, "\terror:") {
			errored = append(errored, name)
		}
	}
	var b strings.Builder
	order := []string{"same", "differs", "random", "not runnable"}
	for _, k := range order {
		fmt.Fprintf(&b, "%s: %d\n", k, len(lists[k]))
	}
	for _, k := range order[1:] {
		fmt.Fprintf(&b, "\n## %s\n%s", k, joinLines(lists[k]))
	}
	if err := os.WriteFile(filepath.Join(dir, "report.txt"), []byte(b.String()), 0o644); err != nil {
		return err
	}
	for _, k := range order {
		fmt.Printf("%s: %d\n", k, len(lists[k]))
	}
	if baseline == "" {
		return nil
	}
	return checkBaseline(lists["differs"], errored, baseline)
}

// checkBaseline is the gate: every differs scenario and every Java error is
// in the baseline and every baseline scenario still differs or errs, so the
// file is the exact list of accepted differences. A line is
// "<scenario><TAB><the quirks.md or forge-java-defects.md row that explains
// it>"; # starts a comment.
func checkBaseline(differs, errored []string, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var accepted []string
	for i, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, cite, _ := strings.Cut(line, "\t")
		if strings.TrimSpace(cite) == "" {
			return fmt.Errorf("%s:%d: %s cites no quirks.md or forge-java-defects.md row", path, i+1, name)
		}
		accepted = append(accepted, name)
	}
	var problems []string
	for _, name := range differs {
		if !slices.Contains(accepted, name) {
			problems = append(problems, name+": differs, not in the baseline (diffs/"+name+".diff)")
		}
	}
	for _, name := range errored {
		if !slices.Contains(accepted, name) {
			problems = append(problems, name+": Java error, not in the baseline (java/"+name+".error)")
		}
	}
	for _, name := range accepted {
		if !slices.Contains(differs, name) && !slices.Contains(errored, name) {
			problems = append(problems, name+": in the baseline, no longer differs or errs")
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("baseline %s:\n%s", path, strings.Join(problems, "\n"))
	}
	return nil
}

// classify sorts one scenario: not runnable when Java wrote no state (an
// unmapped line or a Java error, the first line as the detail), same when the
// normalised states agree, otherwise random or differs; a differs scenario's
// normalised pair goes to diffs/<name>.diff.
func classify(dir, name, goFile string, random bool) (kind, detail string, err error) {
	javaBase := filepath.Join(dir, "java", name)
	for _, ext := range []string{".unmapped", ".error"} {
		if raw, err := os.ReadFile(javaBase + ext); err == nil {
			first, _, _ := strings.Cut(string(raw), "\n")
			return "not runnable", "\t" + strings.TrimPrefix(ext, ".") + ": " + first, nil
		}
	}
	javaRaw, err := os.ReadFile(javaBase + ".state")
	if os.IsNotExist(err) {
		return "not runnable", "\tno Java output", nil
	}
	if err != nil {
		return "", "", err
	}
	goRaw, err := os.ReadFile(goFile)
	if err != nil {
		return "", "", err
	}
	j, err := Normalize(string(javaRaw), JavaSide)
	if err != nil {
		return "", "", fmt.Errorf("%s java: %w", name, err)
	}
	g, err := Normalize(string(goRaw), GoSide)
	if err != nil {
		return "", "", fmt.Errorf("%s go: %w", name, err)
	}
	if j == g {
		return "same", "", nil
	}
	if err := os.WriteFile(filepath.Join(dir, "diffs", name+".diff"), []byte(lineDiff(g, j)), 0o644); err != nil {
		return "", "", err
	}
	if random {
		return "random", "", nil
	}
	return "differs", "", nil
}

// lineDiff lists the normalised lines only Go has ("- ") and only Java has
// ("+ "); both texts are sorted by key (N8), so a changed key shows as a pair.
func lineDiff(goText, javaText string) string {
	g, j := strings.Split(goText, "\n"), strings.Split(javaText, "\n")
	var b strings.Builder
	for _, l := range g {
		if l != "" && !slices.Contains(j, l) {
			b.WriteString("- go   " + l + "\n")
		}
	}
	for _, l := range j {
		if l != "" && !slices.Contains(g, l) {
			b.WriteString("+ java " + l + "\n")
		}
	}
	return b.String()
}

// clearDir makes dir and removes the files an earlier run left there that
// match pattern, so a scenario deleted or reclassified since does not linger.
func clearDir(dir, pattern string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	stale, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return err
	}
	for _, f := range stale {
		if err := os.Remove(f); err != nil {
			return err
		}
	}
	return nil
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}
