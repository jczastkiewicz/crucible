package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// These tests cover where "enters as a copy" sits among the other
// replacements of one entry (ReplacementLayer order Control, Copy, Other,
// CR 616.1): the Copy layer runs before the card moves, and the player the
// card enters under orders several copy replacements.

// essenceDef is a 6/6 watcher: each other creature its controller puts onto
// the battlefield enters as a copy of it (Essence of the Wild).
func essenceDef(t *testing.T) *compile.Card {
	t.Helper()
	return copyTestDef(t, "Test Essence", "Creature Elf", "6", "6",
		"K:ETBReplacement:Copy:EssenceClone:Mandatory:Battlefield:Creature.Other+YouCtrl",
		"SVar:EssenceClone:DB$ Clone | Defined$ Self | CloneTarget$ ReplacedCard")
}

// TestEntersAsCopyAmongSeveralCopyReplacementsTheEnteringPlayerChooses proves
// CR 616.1: a Metamorph entering beside an Essence of the Wild has two copy
// replacements, and which applies first changes the outcome. Applied first,
// the Metamorph's own makes it an Equipment, which is no creature, so Essence
// no longer applies on the re-gather; applied second, Essence makes it a 6/6
// Essence, which has no copy replacement of the Metamorph's left.
func TestEntersAsCopyAmongSeveralCopyReplacementsTheEnteringPlayerChooses(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		pick     int
		wantName string
		creature bool
	}{
		{"its own first", 0, "Test Bonesplitter", false},
		{"Essence first", 1, "Test Essence", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, other := newTwoPlayerGame(t)
			g.NewCard(essenceDef(t), p, engine.Battlefield)
			bone := g.NewCard(copyTestDef(t, "Test Bonesplitter", "Artifact Equipment", "", ""), other, engine.Battlefield)
			meta := g.NewCard(testCloneDef(t, "Test Metamorph", "Artifact.Other,Creature.Other"), p, engine.Hand)
			sc := engine.NewScriptedController()
			sc.QueueReplacementEffect(tc.pick)
			if tc.pick == 0 {
				sc.QueueConfirmEffect(true)
				sc.QueueCardChoice([]engine.CardID{bone})
			}
			mustCastAndResolve(t, g, p, sc, meta)

			m := g.Card(meta)
			if m.Def.Name != tc.wantName || m.Type().Has(cardtype.Creature) != tc.creature {
				t.Errorf("metamorph entered as %q (creature %v), want %q (creature %v)",
					m.Def.Name, m.Type().Has(cardtype.Creature), tc.wantName, tc.creature)
			}
			if m.UncopiedDef().Name != "Test Metamorph" {
				t.Errorf("own name = %q", m.UncopiedDef().Name)
			}
		})
	}
}

// TestEntersAsCopyChoiceOutOfRangeIsAnError proves a controller bug in the
// CR 616.1 answer is recorded (GO-7) and the first candidate applies.
func TestEntersAsCopyChoiceOutOfRangeIsAnError(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(essenceDef(t), p, engine.Battlefield)
	giant := g.NewCard(giantDef(t), other, engine.Battlefield)
	clone := g.NewCard(testCloneDef(t, "Test Clone", "Creature.Other"), p, engine.Hand)
	sc := engine.NewScriptedController()
	sc.QueueReplacementEffect(5)
	sc.QueueConfirmEffect(true)
	sc.QueueCardChoice([]engine.CardID{giant})
	if err := castAndResolve(t, g, p, sc, clone); err == nil {
		t.Fatal("an out-of-range replacement choice was not an error")
	}
}

// priestDef is Containment Priest: a nontoken creature that would enter
// without having been cast is exiled instead.
func priestDef(t *testing.T) *compile.Card {
	t.Helper()
	return copyTestDef(t, "Test Priest", "Creature Elf", "2", "2",
		"R:Event$ Moved | ActiveZones$ Battlefield | Destination$ Battlefield | ValidCard$ Creature.!token+!wasCast | ReplaceWith$ Exile | Description$ If a nontoken creature would enter and it wasn't cast, exile it instead.",
		"SVar:Exile:DB$ ChangeZone | Hidden$ True | Origin$ All | Destination$ Exile | Defined$ ReplacedCard")
}

// reanimator is a permanent whose ability puts a targeted creature card from
// a graveyard onto the battlefield, uncast.
func reanimator(t *testing.T, g *engine.Game, p engine.PlayerID) engine.CardID {
	t.Helper()
	return g.NewCard(copyTestDef(t, "Test Reanimator", "Enchantment", "", "",
		"A:AB$ ChangeZone | Cost$ 0 | Origin$ Graveyard | Destination$ Battlefield | ValidTgts$ Creature | TgtZone$ Graveyard"),
		p, engine.Battlefield)
}

// TestEntersAsCopyRunsBeforeTheOtherLayer proves the copy layer is ahead of
// Containment Priest's replacement (ReplacementLayer.Copy before Other): a
// reanimated Metamorph that copies a noncreature artifact is no creature
// when the Priest looks, so it enters; one that copies a creature is still a
// creature, is exiled, and its copy goes with the object that never entered.
func TestEntersAsCopyRunsBeforeTheOtherLayer(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		copies string
		zone   engine.ZoneType
	}{
		{"an artifact that is no creature enters", "Test Bonesplitter", engine.Battlefield},
		{"a creature is still exiled", "Copied Giant", engine.Exile},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, other := newTwoPlayerGame(t)
			g.NewCard(priestDef(t), p, engine.Battlefield)
			bone := g.NewCard(copyTestDef(t, "Test Bonesplitter", "Artifact Equipment", "", ""), other, engine.Battlefield)
			giant := g.NewCard(giantDef(t), other, engine.Battlefield)
			host := reanimator(t, g, p)
			meta := g.NewCard(testCloneDef(t, "Test Metamorph", "Artifact.Other,Creature.Other"), p, engine.Graveyard)
			pick := bone
			if tc.copies == "Copied Giant" {
				pick = giant
			}
			sc := engine.NewScriptedController()
			sc.QueueConfirmEffect(true)
			sc.QueueCardChoice([]engine.CardID{pick})
			mustActivate(t, g, p, sc, host, meta)

			m := g.Card(meta)
			if m.Zone != tc.zone {
				t.Fatalf("metamorph is in %v, want %v", m.Zone, tc.zone)
			}
			if tc.zone == engine.Battlefield && (m.Def.Name != tc.copies || !m.IsCopy()) {
				t.Errorf("metamorph on the battlefield is %q (copy %v), want a copy of %q", m.Def.Name, m.IsCopy(), tc.copies)
			}
			if tc.zone == engine.Exile && (m.IsCopy() || m.Def.Name != "Test Metamorph") {
				t.Errorf("exiled metamorph is %q (copy %v), want its own card with no copy effect", m.Def.Name, m.IsCopy())
			}
		})
	}
}

// TestEntersAsCopyFromTheGraveyardMayChooseItself proves the entering card
// is still in the zone it enters from while the replacement runs, as in
// Java's last graveyard state: a Body Double style choice among graveyard
// creatures with no Other offers the card itself.
func TestEntersAsCopyFromTheGraveyardMayChooseItself(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	host := reanimator(t, g, p)
	double := g.NewCard(testCloneDef(t, "Test Double", "Creature | ChoiceZone$ Graveyard"), p, engine.Graveyard)
	sc := engine.NewScriptedController()
	sc.QueueConfirmEffect(true)
	rec := &offerRecorder{ScriptedController: sc}
	sc.QueueCardChoice([]engine.CardID{double})
	mustActivate(t, g, p, rec, host, double)
	if len(rec.offers) != 1 || !containsID(rec.offers[0], double) {
		t.Errorf("offers = %v, want the entering card itself among them", rec.offers)
	}
}
