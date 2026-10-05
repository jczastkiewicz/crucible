package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// ascendGame seats nine Forests plus extra permanents the caller adds, in a
// two-player game on the real corpus, human to move in Main1.
func ascendGame(t *testing.T, forests int) (*engine.Game, engine.PlayerID, engine.PlayerID, []engine.CardID) {
	t.Helper()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	var lands []engine.CardID
	for range forests {
		lands = append(lands, g.NewCard(corpusCard(t, "Forest"), p, engine.Battlefield))
	}
	return g, p, other, lands
}

// TestAscendNeedsTenPermanents pins CardFactoryUtil.java:629-650: a permanent
// with Ascend gives its controller the city's blessing once they control
// ten permanents, and not before.
func TestAscendNeedsTenPermanents(t *testing.T) {
	t.Parallel()

	g, p, _, _ := ascendGame(t, 8)
	g.NewCard(corpusCard(t, "Skymarcher Aspirant"), p, engine.Battlefield)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if g.Player(p).Blessing {
		t.Fatal("nine permanents gave the city's blessing")
	}
	g.NewCard(corpusCard(t, "Forest"), p, engine.Battlefield)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if !g.Player(p).Blessing {
		t.Fatal("ten permanents did not give the city's blessing")
	}
}

// TestBlessingTurnsOnSkymarcherAspirantsFlying: `Condition$ Blessing` on a
// static, and the flyer cannot be blocked by a creature without flying or
// reach.
func TestBlessingTurnsOnSkymarcherAspirantsFlying(t *testing.T) {
	t.Parallel()

	g, p, other, _ := ascendGame(t, 8)
	aspirant := g.NewCard(corpusCard(t, "Skymarcher Aspirant"), p, engine.Battlefield)
	lion := g.NewCard(corpusCard(t, "Silvercoat Lion"), other, engine.Battlefield)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if g.Card(aspirant).HasKeyword("Flying") || !g.CanBlock(aspirant, lion) {
		t.Fatal("the Aspirant flies without the city's blessing")
	}
	g.NewCard(corpusCard(t, "Forest"), p, engine.Battlefield)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if !g.Card(aspirant).HasKeyword("Flying") {
		t.Error("the Aspirant does not fly with the city's blessing")
	}
	if g.CanBlock(aspirant, lion) {
		t.Error("a creature without flying or reach blocked the flying Aspirant")
	}
}

// TestActivationBlessingRefusesWithoutTheBlessing: Arch of Orazca's draw
// (Activation$ Blessing) is refused even when payable, until the controller
// has the blessing. The ten permanents Ascend needs are Arch and nine Forests;
// the blessing is set by hand in the refused half so the permanent count
// cannot be what refuses.
func TestActivationBlessingRefusesWithoutTheBlessing(t *testing.T) {
	t.Parallel()

	g, p, _, lands := ascendGame(t, 5)
	arch := g.NewCard(corpusCard(t, "Arch of Orazca"), p, engine.Battlefield)
	g.NewCard(corpusCard(t, "Island"), p, engine.Library)

	c := engine.NewScriptedController()
	green, _ := mana.ParseShard("G")
	for _, land := range lands {
		g.TapLandForMana(p, land, mana.Green, c)
		c.QueuePayGeneric(green)
	}
	if g.ActivateAbility(p, arch, 1, c) {
		t.Fatal("Arch's draw activated without the city's blessing")
	}
	if g.Card(arch).Tapped {
		t.Error("a refused activation tapped Arch")
	}
}

// TestActivationSolvedNeedsASolvedCase: Case of the Uneaten Feast's Solved
// ability (Activation$ Solved) needs the Case to be solved.
func TestActivationSolvedNeedsASolvedCase(t *testing.T) {
	t.Parallel()

	g, p, _, _ := ascendGame(t, 0)
	feast := g.NewCard(corpusCard(t, "Case of the Uneaten Feast"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	if g.ActivateAbility(p, feast, 0, c) {
		t.Fatal("the Solved ability activated on an unsolved Case")
	}
	g.Card(feast).Solved = true
	if !g.ActivateAbility(p, feast, 0, c) {
		t.Error("the Solved ability did not activate on a solved Case")
	}
}

// restrictedLandDef is a land with one A: line.
func restrictedLandDef(t *testing.T, line string) *compile.Card {
	t.Helper()
	raw := &carddb.Card{Filename: "restricted_land"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Restricted Land"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Land")
	raw.Faces[0].Abilities = []string{line}
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", line, err)
	}
	return c
}

// TestActivationGameTypesNeedsAVariant: an ability naming only Commander-
// family variants (ActivationGameTypes$, SpellAbilityRestriction.java:526)
// never activates in a constructed game, which has none applied; the same
// line without the key does.
func TestActivationGameTypesNeedsAVariant(t *testing.T) {
	t.Parallel()

	g, p, _, _ := ascendGame(t, 0)
	gated := g.NewCard(restrictedLandDef(t, "AB$ GainLife | Cost$ T | LifeAmount$ 1 | ActivationGameTypes$ Commander,Brawl"), p, engine.Battlefield)
	open := g.NewCard(restrictedLandDef(t, "AB$ GainLife | Cost$ T | LifeAmount$ 1"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	if g.ActivateAbility(p, gated, 0, c) {
		t.Error("an ActivationGameTypes$ Commander line activated in a game with no variant")
	}
	if !g.ActivateAbility(p, open, 0, c) {
		t.Error("the same line without ActivationGameTypes$ did not activate")
	}
}

// TestActivationClassLevelIsRefused: no Class level is tracked, so a
// ClassLevel$ restriction refuses rather than being ignored (GO-7).
func TestActivationClassLevelIsRefused(t *testing.T) {
	t.Parallel()

	g, p, _, _ := ascendGame(t, 0)
	land := g.NewCard(restrictedLandDef(t, "AB$ GainLife | Cost$ T | LifeAmount$ 1 | ClassLevel$ GE2"), p, engine.Battlefield)
	if g.ActivateAbility(p, land, 0, engine.NewScriptedController()) {
		t.Error("a ClassLevel$ line activated with no Class level tracked")
	}
}
