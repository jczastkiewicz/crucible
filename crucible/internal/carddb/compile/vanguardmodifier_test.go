package compile_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// HandLifeModifier:+H/+L becomes the avatar's deltas (CardRules.java:
// 480-486); a card without one has none, and a malformed value is an error.
func TestHandLifeModifierBecomesTheAvatarsDeltas(t *testing.T) {
	t.Parallel()

	got := compileScript(t, "Name:Test Avatar\nManaCost:no cost\nTypes:Vanguard\nHandLifeModifier:-1/+10\n")
	if got.HandDelta != -1 || got.LifeDelta != 10 {
		t.Errorf("deltas = %d/%d, want -1/+10", got.HandDelta, got.LifeDelta)
	}
	plain := compileScript(t, "Name:Test Plain\nManaCost:1\nTypes:Artifact\n")
	if plain.HandDelta != 0 || plain.LifeDelta != 0 {
		t.Errorf("deltas = %d/%d, want none", plain.HandDelta, plain.LifeDelta)
	}
	for _, bad := range []string{"+1", "x/+1", "+1/y"} {
		card, err := carddb.ParseScript(testRegistry(t), "fixture", []byte("Name:Bad\nManaCost:no cost\nTypes:Vanguard\nHandLifeModifier:"+bad+"\n"))
		if err != nil {
			t.Fatalf("ParseScript: %v", err)
		}
		if _, err := compile.Compile(card); err == nil {
			t.Errorf("HandLifeModifier %q compiled", bad)
		}
	}
}
