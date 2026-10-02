package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// CR 702.73a: a card with changeling is every creature type, in every zone
// (Changeling's Characteristic-defining static is EffectZone$ All).
func TestChangelingIsEveryCreatureTypeInEveryZone(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	for _, zone := range []engine.ZoneType{engine.Battlefield, engine.Hand, engine.Graveyard} {
		card := g.NewCard(creatureDefPTKeywords(t, "2", "2", "Changeling"), p, zone)
		plain := g.NewCard(creatureDefPT(t, "2", "2"), p, zone)
		sba(g)
		for _, subtype := range []string{"Goblin", "Elf", "Wizard"} {
			if !g.Card(card).Type().HasSubtype(subtype) {
				t.Errorf("changeling in %v is not a %s", zone, subtype)
			}
		}
		if g.Card(plain).Type().HasSubtype("Goblin") {
			t.Errorf("a creature without changeling in %v is a Goblin", zone)
		}
	}
}
