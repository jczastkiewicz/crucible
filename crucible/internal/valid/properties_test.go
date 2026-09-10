package valid_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/keyword"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// exactProperty and prefixProperty match the two ways Java's chain tests a
// property name. The distinction is load-bearing: `equals("Attacking")` accepts
// exactly that word, while `startsWith("AttachedTo")` accepts
// `AttachedTo Creature.YouCtrl` and everything else with an argument, and
// treating the second as exact rejects 400 properties Forge handles.
var (
	exactProperty  = regexp.MustCompile(`(?:property|restriction)\.(?:equals|equalsIgnoreCase)\("([^"]+)"`)
	prefixProperty = regexp.MustCompile(`(?:property|restriction)\.(?:startsWith|contains)\("([^"]+)"`)
)

// propertyChains are every file holding one. There are four, not the two the
// grammar doc names: a valid string is matched against a card, its current
// state, a player, or a spell on the stack, and each has its own chain.
// TrackableProperty is a GUI enum and is not one of them.
var propertyChains = []string{
	"forge-game/src/main/java/forge/game/card/CardProperty.java",
	"forge-game/src/main/java/forge/game/card/CardStateProperty.java",
	"forge-game/src/main/java/forge/game/player/PlayerProperty.java",
	"forge-game/src/main/java/forge/game/spellability/SpellAbilityProperty.java",
}

// colourProperties are the colour tests CardProperty spells out in code rather
// than as string literals, plus the `<Colour>Source` family.
var colourProperties = []string{
	"White", "Blue", "Black", "Red", "Green", "Colorless",
	"Monocolored", "MultiColor", "Multicolor", "AllColors",
	"AnyChosenColor", "ChosenColor",
	"WhiteSource", "BlueSource", "BlackSource", "RedSource", "GreenSource", "ColorlessSource",
}

// TestEveryPropertyIsAccountedFor is M3 item 19 for the property vocabulary.
//
// `valid.Parse` cannot reject a property, because `CardProperty` cannot: it
// walks its branches and returns false at the end, so a property the port has
// not implemented is indistinguishable from one that simply does not match.
// The gate has to come from outside the parser.
//
// Acceptance mirrors Java's fallthrough order rather than testing a flat set:
// the explicit branches first, then a bare type name, then a keyword name.
// Getting the order wrong would attribute a property that is both -- `Aura` is
// a subtype and `Bestow` is a keyword -- to the wrong source, and the point of
// the gate is to say which code has to implement it.
func TestEveryPropertyIsAccountedFor(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	exact, prefixes := scrapeProperties(t, root)
	reg := testRegistry(t)

	keywords := map[string]bool{}
	for _, k := range keyword.Defined {
		keywords[k.Name] = true
	}
	colours := map[string]bool{}
	for _, c := range colourProperties {
		colours[c] = true
	}
	types := map[string]bool{}
	for _, n := range cardtype.CoreTypeNames() {
		types[n] = true
	}
	for _, n := range cardtype.SupertypeNames() {
		types[n] = true
	}

	accounted := func(p string) bool {
		// `non` is baked into a distinct property name in Java's table, so
		// `nonLand` is checked as itself first and only then as a negated
		// type. The `!` sign is already off: Parse keeps it in Negated.
		switch {
		case exact[p], colours[p], types[p], reg.Known(p), keywords[p]:
			return true
		}
		for _, pre := range prefixes {
			if strings.HasPrefix(p, pre) {
				return true
			}
		}
		if rest, ok := strings.CutPrefix(p, "non"); ok {
			return colours[rest] || types[rest] || reg.Known(rest)
		}
		return false
	}

	seen := map[string]bool{}
	unaccounted := map[string]string{} // property -> a card that writes it
	forEachValidString(t, func(card, _, value string) {
		// `ManaReflected` reads its own Valid$ form. CardUtil.java:261 tests
		// `validCard.startsWith("Defined.")` and treats the rest as a defined
		// name, so `Defined.Sacrificed` never reaches CardProperty and its
		// second half is not a property at all. Nine cards use it.
		if strings.HasPrefix(value, "Defined.") {
			return
		}
		for _, alt := range valid.Parse(value).Alternatives {
			for _, p := range alt.Properties {
				// A numeric comparison carries its own operator and operand,
				// so `powerGE1` is accounted for by being parsed, not by
				// appearing in any table.
				if p.Compare != nil {
					continue
				}
				if seen[p.Name] {
					continue
				}
				seen[p.Name] = true
				if !accounted(p.Name) {
					unaccounted[p.Name] = card
				}
			}
		}
	})

	if len(unaccounted) == 0 {
		return
	}
	names := make([]string, 0, len(unaccounted))
	for n := range unaccounted {
		names = append(names, n)
	}
	sort.Strings(names)
	for i, n := range names {
		if i == 15 {
			t.Errorf("... and %d more", len(names)-15)
			break
		}
		t.Errorf("property %q (%s) matches no CardProperty branch, type, colour or keyword",
			n, unaccounted[n])
	}
	t.Errorf("%d of %d distinct properties unaccounted for", len(unaccounted), len(seen))
}

// scrapeProperties reads the property names out of Java's two property chains,
// keeping the exact tests and the prefix tests apart.
func scrapeProperties(t *testing.T, root string) (exact map[string]bool, prefixes []string) {
	t.Helper()

	exact = map[string]bool{}
	seen := map[string]bool{}
	for _, rel := range propertyChains {
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		text := string(raw)
		for _, m := range exactProperty.FindAllStringSubmatch(text, -1) {
			exact[m[1]] = true
		}
		for _, m := range prefixProperty.FindAllStringSubmatch(text, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				prefixes = append(prefixes, m[1])
			}
		}
	}
	if len(exact)+len(prefixes) < 300 {
		t.Fatalf("scraped only %d property names; the chain's shape changed", len(exact)+len(prefixes))
	}
	sort.Strings(prefixes)
	return exact, prefixes
}
