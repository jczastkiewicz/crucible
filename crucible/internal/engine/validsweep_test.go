package engine_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// TestEveryCorpusValidStringMatchesWithoutPanicking reads each distinct
// Valid*$ / Affected$ / ChangeType$ string the card scripts write against a
// board of cards in every zone: a property the matcher cannot answer is a
// pending error (GO-7), never a panic that would end a 100k-game batch.
func TestEveryCorpusValidStringMatchesWithoutPanicking(t *testing.T) {
	t.Parallel()

	root := filepath.Join(scenarioRepoRoot(t), "forge-gui", "res", "cardsfolder")
	param := regexp.MustCompile(`(?:Valid\w*|Affected|ChangeType|Condition\w*Present\w*|IsPresent\d?|Choices|Defined\w*)\$ ([^|\n]+)`)
	seen := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".txt" {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range param.FindAllSubmatch(raw, -1) {
			seen[string(m[1])] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	specs := make([]string, 0, len(seen))
	for s := range seen {
		specs = append(specs, s)
	}
	sort.Strings(specs)

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	aura := g.NewCard(corpusCard(t, "Pacifism"), other, engine.Battlefield)
	g.Attach(aura, bears)
	cards := []engine.CardID{
		bears, aura,
		g.NewCard(corpusCard(t, "Sol Ring"), other, engine.Battlefield),
		g.NewCard(corpusCard(t, "Forest"), p, engine.Battlefield),
		g.NewCard(corpusCard(t, "Jace Beleren"), other, engine.Battlefield),
		g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Hand),
		g.NewCard(corpusCard(t, "Isamaru, Hound of Konda"), other, engine.Graveyard),
		g.NewCard(corpusCard(t, "Llanowar Elves"), p, engine.Exile),
		g.NewCard(corpusCard(t, "Counterspell"), other, engine.Library),
	}
	g.Card(bears).Memory.Remember(engine.CardEntity(cards[2]))
	g.Card(bears).Memory.Imprint(cards[6])
	for _, s := range specs {
		spec := valid.Parse(s)
		for _, c := range cards {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("Matches(%q, %s) panicked: %v", s, g.Card(c).Def.Name, r)
					}
				}()
				engine.Matches(g, g.Card(c), spec, p, bears)
			}()
		}
	}
	_ = g.TakePendingError()
}
