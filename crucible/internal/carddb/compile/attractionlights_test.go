package compile_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// A Lights: line becomes the face's lit numbers (CardFace.setAttractionLights);
// a variant's own Lights: is not read, and a non-number is an error.
func TestLightsBecomeTheAttractionsLitNumbers(t *testing.T) {
	t.Parallel()

	got := compileScript(t, "Name:Test Ride\nManaCost:no cost\nTypes:Artifact Attraction\nLights:4 5 6\n")
	if want := []int{4, 5, 6}; !slices.Equal(got.Faces[0].Lights, want) {
		t.Errorf("Lights = %v, want %v", got.Faces[0].Lights, want)
	}
	variant := compileScript(t, "Name:Test Variant\nManaCost:no cost\nTypes:Artifact Attraction\nVariant:A:Lights:2 6\n")
	if len(variant.Faces[0].Lights) != 0 {
		t.Errorf("Lights = %v, want none (variant lights are not read)", variant.Faces[0].Lights)
	}
	card, err := carddb.ParseScript(testRegistry(t), "fixture", []byte("Name:Bad\nManaCost:no cost\nTypes:Artifact Attraction\nLights:2 x\n"))
	if err != nil {
		t.Fatalf("ParseScript: %v", err)
	}
	if _, err := compile.Compile(card); err == nil {
		t.Error("Lights:2 x compiled")
	}
}
