// Command silentgap lists the silent GO-7 candidates of R19: a param the Java
// effect of an API reads (apiscan -api), that real corpus lines of that API
// write, and that the Go effect's own file never names. A refused param is
// named in a reject list and so counts as named; one that is neither read nor
// refused is a card that plays and misplays, which the smoke probe cannot see.
//
//	go run ./tools/silentgap            # TSV: API, key, corpus lines, example card
//
// The Go side is the file that carries the API's `//crucible:register`; a key
// read through another file's helper shows up as a candidate and is dismissed
// by reading that helper, so the list is a worklist, not a verdict. "absent"
// rows name a key no engine file mentions at all and come first.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	registryRE = regexp.MustCompile(`r\[API(\w+)\] = (\w+)\{`)
	lineAPIRE  = regexp.MustCompile(`(?:AB|SP|DB|Mode)\$ (\w+)`)
	keyRE      = regexp.MustCompile(`\|\s*(\w+)\$`)
)

func main() {
	engine := flag.String("engine", "internal/engine", "engine package directory")
	corpus := flag.String("corpus", "../forge-gui/res/cardsfolder", "card script directory")
	fewest := flag.Int("min", 1, "fewest corpus lines to report")
	flag.Parse()
	if err := run(*engine, *corpus, *fewest); err != nil {
		fmt.Fprintln(os.Stderr, "silentgap:", err)
		os.Exit(1)
	}
}

func run(engine, corpus string, minLines int) error {
	out, err := exec.Command("go", "run", "./tools/apiscan", "-api").Output()
	if err != nil {
		return fmt.Errorf("apiscan: %w", err)
	}
	javaKeys := map[string][]string{}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		f := strings.Split(sc.Text(), "\t")
		if len(f) == 4 && f[0] == "api" && f[3] != "" {
			javaKeys[f[1]] = strings.Split(f[3], ",")
		}
	}
	files, err := filepath.Glob(filepath.Join(engine, "*.go"))
	if err != nil {
		return err
	}
	texts := map[string]string{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		texts[f] = string(b)
	}
	reg, ok := texts[filepath.Join(engine, "registry_gen.go")]
	if !ok {
		return fmt.Errorf("no registry_gen.go in %s", engine)
	}
	source := map[string]string{} // API -> text of the file declaring its effect type
	for _, m := range registryRE.FindAllStringSubmatch(reg, -1) {
		decl := regexp.MustCompile(`(?m)^type ` + m[2] + ` `)
		for _, text := range texts {
			if decl.MatchString(text) {
				source[m[1]] += text
			}
		}
	}
	type use struct {
		n    int
		card string
	}
	uses := map[string]map[string]*use{} // API -> key -> use
	err = filepath.WalkDir(corpus, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".txt") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(b), "\n") {
			m := lineAPIRE.FindStringSubmatch(line)
			if m == nil || strings.HasPrefix(line, "Oracle:") {
				continue
			}
			for _, k := range keyRE.FindAllStringSubmatch(line, -1) {
				if uses[m[1]] == nil {
					uses[m[1]] = map[string]*use{}
				}
				u := uses[m[1]][k[1]]
				if u == nil {
					u = &use{card: strings.TrimSuffix(filepath.Base(p), ".txt")}
					uses[m[1]][k[1]] = u
				}
				u.n++
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	type row struct {
		api, key, card string
		n              int
		elsewhere      bool
	}
	all := ""
	for _, t := range texts {
		all += t
	}
	all = strings.ToLower(all)
	var rows []row
	for api, keys := range javaKeys {
		src, ok := source[api]
		if !ok {
			continue
		}
		src = strings.ToLower(src)
		for _, k := range keys {
			u := uses[api][k]
			if u == nil || u.n < minLines || named(src, k) {
				continue
			}
			rows = append(rows, row{api, k, u.card, u.n, named(all, k)})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].elsewhere != rows[j].elsewhere {
			return !rows[i].elsewhere
		}
		if rows[i].n != rows[j].n {
			return rows[i].n > rows[j].n
		}
		return rows[i].api+rows[i].key < rows[j].api+rows[j].key
	})
	for _, r := range rows {
		where := "absent"
		if r.elsewhere {
			where = "elsewhere"
		}
		fmt.Printf("%s\t%s\t%d\t%s\t%s\n", r.api, r.key, r.n, r.card, where)
	}
	return nil
}

// named reports whether the lowercased text names key: a string literal (the
// engine's param switches fold case) or a "Key$" in a comment that records a
// deliberate decision about it.
func named(lowerText, key string) bool {
	k := strings.ToLower(key)
	return strings.Contains(lowerText, `"`+k+`"`) || strings.Contains(lowerText, k+"$")
}
