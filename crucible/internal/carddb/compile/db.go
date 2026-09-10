// The compiled card database: every script, compiled once, shared read-only.

package compile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
)

// DB is every card in the corpus, compiled.
//
// One per process, built at startup and never written afterwards, so games
// running on separate goroutines share it by pointer with no synchronisation
// (ADR-0005). Nothing here is guarded, and nothing should be: a mutex would
// mean something is writing.
type DB struct {
	byName map[string]*Card
	names  []string
}

// LoadDB parses and compiles every script under root.
//
// It stops at the first failure rather than skipping it. A card that will not
// compile is an upstream defect to report, not a card to run without
// (PORT-8), and the corpus gates keep that count at zero.
func LoadDB(root, typeList string) (*DB, error) {
	f, err := os.Open(typeList)
	if err != nil {
		return nil, err
	}
	reg, err := cardtype.LoadRegistry(f)
	_ = f.Close()
	if err != nil {
		return nil, err
	}

	db := &DB{byName: map[string]*Card{}}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".txt" {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		parsed, err := carddb.ParseScript(reg, strings.TrimSuffix(filepath.Base(path), ".txt"), raw)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		card, err := Compile(parsed)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		name := parsed.Faces[0].Name
		if name == "" {
			return fmt.Errorf("%s: no name", path)
		}
		db.byName[name] = card
		db.names = append(db.names, name)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return db, nil
}

// Card looks a card up by its printed name, and reports whether the database
// has it. Names are matched exactly: a decklist naming a card the database
// does not have is a decklist error worth surfacing, not a near-miss to guess
// at.
func (d *DB) Card(name string) (*Card, bool) {
	c, ok := d.byName[name]
	return c, ok
}

// Len is how many cards the database holds.
func (d *DB) Len() int { return len(d.byName) }

// Names returns every card name, in the order the corpus walk found them,
// which is the filesystem's lexical order.
func (d *DB) Names() []string { return d.names }
