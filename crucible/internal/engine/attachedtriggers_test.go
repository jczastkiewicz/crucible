package engine_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/fixture"
)

// TestPsychicPaperRenamesTheEquippedCreature proves the result of the
// replacement-attached-psychic-paper scenario that expect.state cannot say:
// GameState text names a card by its paper name, so the Grizzly Bears' new
// name (SetName$ ChosenName, the NameCard pick of Event$ Attached) and creature
// type (AddType$ ChosenType, the ChooseType pick) are read off the game.
func TestPsychicPaperRenamesTheEquippedCreature(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(crucibleModuleRoot(t), "testdata", "scenarios", "replacement-attached-psychic-paper-names-the-equipped-creature")
	setup := loadScenarioFile(t, scenarioDB(t), filepath.Join(dir, "setup.state"))
	raw, err := os.ReadFile(filepath.Join(dir, "actions.log"))
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.RunActions(bytes.NewReader(raw), setup, engine.NewScriptedController()); err != nil {
		t.Fatalf("run actions.log: %v", err)
	}
	g := setup.Game
	bf := g.Zone(engine.Battlefield, g.Players()[0]).Cards()
	bears := g.Card(bf[1])
	if bears.PrintedDef().Name != "Grizzly Bears" {
		t.Fatalf("battlefield[1] is %q, want the Grizzly Bears", bears.PrintedDef().Name)
	}
	if got := bears.Name(); got != "Hill Giant" {
		t.Errorf("name = %q, want Hill Giant", got)
	}
	if !bears.Type().HasSubtype("Goblin") || bears.Type().HasSubtype("Bear") {
		t.Errorf("type = %v, want a Goblin and no longer a Bear", bears.Type())
	}
}
