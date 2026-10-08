package engine_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/fixture"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// TestResolvedThisTurnTransformsSephirothOnTheFourthDeath is Count$
// ResolvedThisTurn (AbilityUtils.java:1843) end to end: Sephiroth's SetState
// link reads X = how often it has resolved this turn, and EQ4 holds only on the
// fourth death trigger. The back face's Event$ Transform replacement then puts
// his emblem in the Command zone, which GameState text cannot name, so this is
// a Go test rather than a scenario (the three-death case is one:
// count-resolved-this-turn-sephiroth-stays-after-three-deaths).
func TestResolvedThisTurnTransformsSephirothOnTheFourthDeath(t *testing.T) {
	t.Parallel()
	st, err := fixture.Parse(strings.NewReader(`humanlife=20
ailife=20
humanbattlefield=Sephiroth, Fabled SOLDIER|Id:1
aibattlefield=Grizzly Bears|Id:2|Damage:2;Grizzly Bears|Id:3|Damage:2;Grizzly Bears|Id:4|Damage:2;Grizzly Bears|Id:5|Damage:2
`))
	if err != nil {
		t.Fatal(err)
	}
	setup, err := fixture.Load(st, scenarioDB(t), javarand.New(1))
	if err != nil {
		t.Fatal(err)
	}
	actions := `queue targets ai
queue targets ai
queue targets ai
queue targets ai
startturn human
resolvestack
resolvestack
resolvestack
resolvestack
`
	if err := fixture.RunActions(bytes.NewReader([]byte(actions)), setup, engine.NewScriptedController()); err != nil {
		t.Fatalf("run actions: %v", err)
	}
	g := setup.Game
	human, ai := g.Players()[0], g.Players()[1]
	sephiroth := g.Card(g.Zone(engine.Battlefield, human).Cards()[0])
	if !sephiroth.InTransformedState() {
		t.Error("Sephiroth did not transform on the fourth resolution")
	}
	if got := g.Player(ai).Life; got != 16 {
		t.Errorf("opponent life = %d, want 16", got)
	}
	if got := len(g.Zone(engine.Command, human).Cards()); got != 1 {
		t.Errorf("human command zone has %d cards, want Sephiroth's emblem", got)
	}
	if strings.TrimSpace(sephiroth.Name()) == "" {
		t.Error("transformed Sephiroth has no name")
	}
}
