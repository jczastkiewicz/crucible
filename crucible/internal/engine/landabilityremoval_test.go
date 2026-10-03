package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// CR 305.7: Blood Moon sets a nonbasic land's subtype to Mountain, so the land
// loses the abilities its own text gave it (Card.hasRemoveIntrinsic,
// CardState.LandTraitChanges) and gains the Mountain's intrinsic one. Crystal
// Vein can no longer tap for its printed {C}; it taps for {R}.
func TestBloodMoonRemovesAPrintedManaAbility(t *testing.T) {
	t.Parallel()

	for _, moon := range []bool{false, true} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		if moon {
			g.NewCard(corpusCard(t, "Blood Moon"), p, engine.Battlefield)
		}
		vein := g.NewCard(corpusCard(t, "Crystal Vein"), p, engine.Battlefield)
		sba(g)
		c := engine.NewScriptedController()
		if got := g.ActivateManaAbility(p, vein, 0, c); got == moon {
			t.Errorf("Blood Moon %v: printed {T}: Add {C} activated = %v, want %v", moon, got, !moon)
		}
		if got := g.TapLandForMana(p, vein, mana.Red, c); got != moon {
			t.Errorf("Blood Moon %v: tap for {R} = %v, want %v", moon, got, moon)
		}
	}
}

// CR 613.8a with CR 305.7: Urborg's "each land is a Swamp" depends on Blood
// Moon, which removes it, so Blood Moon applies first whatever the
// timestamps and Urborg's ability no longer exists. A basic Forest stays a
// Forest only.
func TestBloodMoonRemovesUrborgsAbilityWhateverTheTimestamps(t *testing.T) {
	t.Parallel()

	for _, urborgFirst := range []bool{false, true} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		if urborgFirst {
			g.NewCard(corpusCard(t, "Urborg, Tomb of Yawgmoth"), p, engine.Battlefield)
			g.NewCard(corpusCard(t, "Blood Moon"), p, engine.Battlefield)
		} else {
			g.NewCard(corpusCard(t, "Blood Moon"), p, engine.Battlefield)
			g.NewCard(corpusCard(t, "Urborg, Tomb of Yawgmoth"), p, engine.Battlefield)
		}
		forest := g.NewCard(corpusCard(t, "Forest"), p, engine.Battlefield)
		sba(g)
		if ty := g.Card(forest).Type(); ty.HasSubtype("Swamp") {
			t.Errorf("Urborg first %v: Forest types = %v, want no Swamp (Urborg lost its ability to Blood Moon)", urborgFirst, ty)
		}
	}
}

// CR 305.7 removes printed keywords too, while a Layer 6 grant still applies
// on top: Darksteel Citadel loses its printed indestructible under Blood
// Moon and keeps the hexproof a continuous static gives it.
func TestBloodMoonRemovesPrintedKeywordsButNotGrantedOnes(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	citadel := g.NewCard(corpusCard(t, "Darksteel Citadel"), p, engine.Battlefield)
	g.NewCard(scriptDef(t, "Test Grant", "Enchantment", "S:Mode$ Continuous | Affected$ Land.YouCtrl | AddKeyword$ Hexproof"), p, engine.Battlefield)
	sba(g)
	if !g.Card(citadel).HasKeyword("Indestructible") {
		t.Fatal("Darksteel Citadel lacks its printed indestructible before Blood Moon")
	}
	g.NewCard(corpusCard(t, "Blood Moon"), p, engine.Battlefield)
	sba(g)
	c := g.Card(citadel)
	if c.HasKeyword("Indestructible") {
		t.Error("Darksteel Citadel kept its printed indestructible under Blood Moon")
	}
	if !c.HasKeyword("Hexproof") {
		t.Error("Darksteel Citadel lost the hexproof a Layer 6 static grants")
	}
}
