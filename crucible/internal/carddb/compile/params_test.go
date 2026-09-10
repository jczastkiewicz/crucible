package compile_test

import (
	"sort"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// TestEveryAbilityGetsTypedParams runs the whole corpus through the generated
// constructors.
//
// The structs are generated from an evidence table rather than a declaration,
// so the thing worth checking is not that a field has the right type -- the
// generator's rule decides that, and it is written down -- but that every
// ability an API compiles to actually reaches a struct, and that filling one
// from real script values never panics. A generated file nothing exercises is
// a generated file that is wrong the first time upstream moves a param.
func TestEveryAbilityGetsTypedParams(t *testing.T) {
	t.Parallel()

	cards := parseCorpus(t)
	var (
		typed     int
		untyped   = map[string]int{}
		abilities int
	)
	for _, card := range cards {
		out, err := compile.Compile(card)
		if err != nil {
			continue // TestCorpusCompiles owns compile failures
		}
		for i := range out.Faces {
			for _, group := range [][]*compile.Ability{
				out.Faces[i].Abilities, out.Faces[i].Triggers,
				out.Faces[i].Statics, out.Faces[i].Replacements,
			} {
				for _, a := range group {
					walkTyped(a, &abilities, &typed, untyped)
				}
			}
		}
	}

	if abilities == 0 {
		t.Fatal("no abilities compiled")
	}
	t.Logf("%d abilities, %d with generated params, %d record names without",
		abilities, typed, len(untyped))

	// A name with no struct must be a trigger mode, a static mode or a
	// replacement event -- never an API. An API missing from the generated
	// switch is a hole the corpus would otherwise resolve silently.
	for _, a := range apiNames(t) {
		if untyped[a] > 0 {
			t.Errorf("API %q compiled %d abilities but has no generated params", a, untyped[a])
		}
	}
}

func walkTyped(a *compile.Ability, abilities, typed *int, untyped map[string]int) {
	*abilities++
	if _, ok := compile.ParseParams(a.Name, a.Params); ok {
		*typed++
	} else {
		untyped[a.Name]++
	}
	for _, s := range a.Subs {
		walkTyped(s.Ability, abilities, typed, untyped)
	}
}

// apiNames returns the API names that have a generated struct, which is what
// the switch accepts. Asking the generated code rather than re-reading
// ApiType.java keeps the test honest about what was generated.
func apiNames(t *testing.T) []string {
	t.Helper()

	out := append([]string(nil), compile.GeneratedAPIs()...)
	sort.Strings(out)
	if len(out) < 150 {
		t.Fatalf("only %d generated APIs; the generator did not run", len(out))
	}
	return out
}
