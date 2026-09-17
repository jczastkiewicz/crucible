package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// TestCanBlockFlyingAttackerNeedsFlyingOrReach proves the Flying keyword's
// own CantBlockBy synthesis (cantBlockByKeywords, staticability.go),
// mirroring CardFactoryUtil.java's own "Mode$ CantBlockBy | ValidAttacker$
// Creature.Self | ValidBlocker$ Creature.withoutFlying+withoutReach": a
// creature with neither flying nor reach cannot block a flying attacker,
// one with either can.
func TestCanBlockFlyingAttackerNeedsFlyingOrReach(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	attacker := g.NewCard(creatureDefPTKeywords(t, "2", "2", "Flying"), a, engine.Battlefield)
	ground := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)
	flyer := g.NewCard(creatureDefPTKeywords(t, "2", "2", "Flying"), b, engine.Battlefield)
	reacher := g.NewCard(creatureDefPTKeywords(t, "2", "2", "Reach"), b, engine.Battlefield)

	if g.CanBlock(attacker, ground) {
		t.Error("CanBlock(flying attacker, grounded blocker) = true, want false")
	}
	if !g.CanBlock(attacker, flyer) {
		t.Error("CanBlock(flying attacker, flying blocker) = false, want true")
	}
	if !g.CanBlock(attacker, reacher) {
		t.Error("CanBlock(flying attacker, reach blocker) = false, want true")
	}
}

// TestCanBlockOrdinaryAttacker is the sanity check: two plain creatures with
// no relevant keyword or static ability block each other legally, the same
// answer CanBlock's own base rule (untapped creature) already gave before
// CantBlockBy existed.
func TestCanBlockOrdinaryAttacker(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)

	if !g.CanBlock(attacker, blocker) {
		t.Error("CanBlock(plain attacker, plain blocker) = false, want true")
	}
}

// TestCanBlockFearAttacker proves the Fear keyword's own CantBlockBy
// synthesis ("ValidBlocker$ Creature.nonArtifact+nonBlack"): an artifact
// creature can still block a Fear attacker (the restriction only catches
// blockers that are both non-artifact AND non-black), a plain nonartifact
// creature cannot.
func TestCanBlockFearAttacker(t *testing.T) {
	t.Parallel()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\nGolem\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	attacker := g.NewCard(creatureDefPTKeywords(t, "2", "2", "Fear"), a, engine.Battlefield)

	artifactDef := &compile.Card{Name: "Test Artifact Creature"}
	artifactDef.Faces[0].Type = cardtype.Parse(reg, "Artifact Creature Golem")
	artifactDef.Faces[0].Power, artifactDef.Faces[0].Toughness = "2", "2"
	artifactBlocker := g.NewCard(artifactDef, b, engine.Battlefield)

	plainBlocker := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)

	if !g.CanBlock(attacker, artifactBlocker) {
		t.Error("CanBlock(fear attacker, artifact blocker) = false, want true (nonArtifact fails, so Fear does not restrict it)")
	}
	if g.CanBlock(attacker, plainBlocker) {
		t.Error("CanBlock(fear attacker, plain nonartifact blocker) = true, want false")
	}
}

// TestCanBlockIntimidateAttacker proves the Intimidate keyword's own
// CantBlockBy synthesis ("ValidBlocker$ Creature.nonArtifact+!SharesColorWith",
// CardFactoryUtil.java:3940) end to end: an artifact creature can still block
// (nonArtifact fails, same as Fear), a nonartifact creature sharing the
// attacker's own color can still block (!SharesColorWith fails), and a
// nonartifact creature of a different color cannot -- the new SharesColorWith
// property (valid.go) exercised by a real caller for the first time.
func TestCanBlockIntimidateAttacker(t *testing.T) {
	t.Parallel()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\nGolem\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]

	attackerDef := &compile.Card{Name: "Test Intimidate Attacker"}
	attackerDef.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	attackerDef.Faces[0].Power, attackerDef.Faces[0].Toughness = "2", "2"
	attackerDef.Faces[0].Keywords = []string{"Intimidate"}
	attackerDef.Faces[0].ManaCost = mana.MustParse("R")
	attacker := g.NewCard(attackerDef, a, engine.Battlefield)

	artifactDef := &compile.Card{Name: "Test Artifact Blocker"}
	artifactDef.Faces[0].Type = cardtype.Parse(reg, "Artifact Creature Golem")
	artifactDef.Faces[0].Power, artifactDef.Faces[0].Toughness = "2", "2"
	artifactBlocker := g.NewCard(artifactDef, b, engine.Battlefield)

	sameColorDef := &compile.Card{Name: "Test Same Color Blocker"}
	sameColorDef.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	sameColorDef.Faces[0].Power, sameColorDef.Faces[0].Toughness = "2", "2"
	sameColorDef.Faces[0].ManaCost = mana.MustParse("R")
	sameColorBlocker := g.NewCard(sameColorDef, b, engine.Battlefield)

	offColorDef := &compile.Card{Name: "Test Off Color Blocker"}
	offColorDef.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	offColorDef.Faces[0].Power, offColorDef.Faces[0].Toughness = "2", "2"
	offColorDef.Faces[0].ManaCost = mana.MustParse("U")
	offColorBlocker := g.NewCard(offColorDef, b, engine.Battlefield)

	if !g.CanBlock(attacker, artifactBlocker) {
		t.Error("CanBlock(intimidate attacker, artifact blocker) = false, want true (nonArtifact fails, so Intimidate does not restrict it)")
	}
	if !g.CanBlock(attacker, sameColorBlocker) {
		t.Error("CanBlock(intimidate attacker, same-color blocker) = false, want true (SharesColorWith holds, so !SharesColorWith fails)")
	}
	if g.CanBlock(attacker, offColorBlocker) {
		t.Error("CanBlock(intimidate attacker, off-color blocker) = true, want false")
	}
}

// cantBlockByAuraDef builds a *compile.Card for an Aura carrying a real,
// literal S:Mode$ CantBlockBy line written on the Aura itself rather than
// synthesized from a keyword -- ValidAttacker$ Creature.EnchantedBy, the
// second most common real shape after Creature.Self (game-state.md's "Block
// legality" section has the corpus count) -- compiled through the real
// pipeline the same reason etbTriggerCreatureDef (trigger_test.go) is.
func cantBlockByAuraDef(t *testing.T) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test Unblockable Aura"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Unblockable Aura"
	raw.Faces[0].Type = cardtype.Parse(reg, "Enchantment Aura")
	raw.Faces[0].Statics = []string{
		"Mode$ CantBlockBy | ValidAttacker$ Creature.EnchantedBy | Secondary$ True",
	}
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// TestCanBlockRealCantBlockByLineOnAnotherHost proves cantBlockBy
// (staticability.go) walks every battlefield permanent as a possible host,
// not just the attacker's own card: the Aura's own static ability, not a
// keyword the attacker carries, is what makes it unblockable here.
func TestCanBlockRealCantBlockByLineOnAnotherHost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	aura := g.NewCard(cantBlockByAuraDef(t), a, engine.Battlefield)
	g.Attach(aura, attacker)
	blocker := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)

	if g.CanBlock(attacker, blocker) {
		t.Error("CanBlock(enchanted-unblockable attacker, ordinary blocker) = true, want false")
	}

	unenchanted := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	if !g.CanBlock(unenchanted, blocker) {
		t.Error("CanBlock(plain attacker, ordinary blocker) = false, want true -- only the enchanted creature is restricted")
	}
}

// TestDeclareCombatBlockersDropsIllegalCantBlockByPairing proves
// DeclareCombatBlockers (block.go) re-checks the controller's own answer
// against CanBlock and drops an illegal pairing rather than committing it --
// the one exception to "not re-checked for legality" control.go's own doc
// comment states for every other Choose*/Declare* method.
func TestDeclareCombatBlockersDropsIllegalCantBlockByPairing(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPTKeywords(t, "2", "2", "Flying"), a, engine.Battlefield)
	ground := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: ground, Attacker: attacker}})
	got := g.DeclareCombatBlockers(bc)

	if got != nil {
		t.Errorf("DeclareCombatBlockers() = %v, want nil -- a grounded creature can't legally block a flying attacker", got)
	}
	if len(g.Blocks()) != 0 {
		t.Errorf("Blocks() = %v, want none", g.Blocks())
	}
}
