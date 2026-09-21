package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// gainLifeReplacementLockDef builds an Enchantment carrying a single
// Event$ GainLife | ReplaceWith$ replacement line and the SVar(s) it
// references -- replacementEnchantmentDefWithSVar's own sibling, taking a
// second SVar for the ReplaceCount$LifeGained reference every real corpus
// line this dispatch resolves needs on top of its own target ability.
func gainLifeReplacementLockDef(t *testing.T, name, replacement string, svars map[string]string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Enchantment")
	raw.Faces[0].Replacements = []string{replacement}
	for svar, body := range svars {
		raw.Faces[0].SVars.Set(svar, body)
	}

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestGainLifeReplacedByDrawInstead proves gainLifeReplaced (replacement.go)
// resolves lich.txt's/nefarious_lich.txt's own real "if you would gain
// life, draw that many cards instead" (ValidPlayer$ You, ReplaceWith$
// naming a plain DB$ Draw | Defined$ You | NumCards$ <SVar naming
// ReplaceCount$LifeGained>): the raw LifeAmount$ (3, not a fixed 1) must
// thread through as the number of cards drawn, and Life must not change at
// all.
func TestGainLifeReplacedByDrawInstead(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	for i := 0; i < 3; i++ {
		g.NewCard(nil, p, engine.Library)
	}
	g.NewCard(gainLifeReplacementLockDef(t, "Test Lich",
		"Event$ GainLife | ActiveZones$ Battlefield | ValidPlayer$ You | ReplaceWith$ Draw | Description$ Draw instead of gaining life.",
		map[string]string{
			"Draw": "DB$ Draw | Defined$ You | NumCards$ Y",
			"Y":    "ReplaceCount$LifeGained",
		}), p, engine.Battlefield)

	if err := castETBGainLife(t, g, p, etbGainLifeTriggerDefParams(t, "Test Gainer", "Defined$ You | LifeAmount$ 3", nil)); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(p).Life; got != 20 {
		t.Errorf("p life = %d, want unchanged 20 -- the gain must be replaced entirely", got)
	}
	if got := len(g.Zone(engine.Hand, p).Cards()); got != 3 {
		t.Errorf("hand has %d cards, want 3 -- ReplaceCount$LifeGained must resolve to the raw LifeAmount$ (3)", got)
	}
}

// TestGainLifeNotReplacedWhenValidPlayerDoesNotMatch is the regression
// proof: the identical lock controlled by other, with p (not other) gaining
// life -- ValidPlayer$ You means "the lock's own controller," so this must
// not fire for p's own gain.
func TestGainLifeNotReplacedWhenValidPlayerDoesNotMatch(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(gainLifeReplacementLockDef(t, "Test Lich",
		"Event$ GainLife | ActiveZones$ Battlefield | ValidPlayer$ You | ReplaceWith$ Draw | Description$ Draw instead of gaining life.",
		map[string]string{
			"Draw": "DB$ Draw | Defined$ You | NumCards$ Y",
			"Y":    "ReplaceCount$LifeGained",
		}), other, engine.Battlefield)

	if err := castETBGainLife(t, g, p, etbGainLifeTriggerDefParams(t, "Test Gainer", "Defined$ You | LifeAmount$ 3", nil)); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(p).Life; got != 23 {
		t.Errorf("p life = %d, want 23 -- other's own lock must not replace p's own gain", got)
	}
	if got := len(g.Zone(engine.Hand, p).Cards()); got != 0 {
		t.Errorf("hand has %d cards, want 0 -- the gain must not have been replaced", got)
	}
}

// TestGainLifeReplacedByLoseLifeInstead proves tainted_remedy.txt's/
// plague_drone.txt's own real "if an opponent would gain life, that player
// loses that much life instead" (ValidPlayer$ Opponent, ReplaceWith$ naming
// a plain DB$ LoseLife | LifeAmount$ <ReplaceCount$LifeGained> | Defined$
// ReplacedPlayer): other is the lock's own opponent gaining 4 life, which
// must become a 4-life loss instead.
func TestGainLifeReplacedByLoseLifeInstead(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, other, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(gainLifeReplacementLockDef(t, "Test Tainted Remedy",
		"Event$ GainLife | ActiveZones$ Battlefield | ValidPlayer$ Opponent | ReplaceWith$ RLoseLife | Description$ Opponents lose life instead of gaining it.",
		map[string]string{
			"RLoseLife": "DB$ LoseLife | LifeAmount$ X | Defined$ ReplacedPlayer",
			"X":         "ReplaceCount$LifeGained",
		}), p, engine.Battlefield)

	if err := castETBGainLife(t, g, other, etbGainLifeTriggerDefParams(t, "Test Gainer", "Defined$ You | LifeAmount$ 4", nil)); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(other).Life; got != 16 {
		t.Errorf("other life = %d, want 16 -- the gain must become a 4-life loss instead", got)
	}
}

// TestGainLifeNotReplacedByUnrecognizedSourceRestriction proves
// rain_of_gore.txt's own real restriction shape -- ValidSource$
// SpellAbility | SourceController$ True, no ValidPlayer$ at all, a
// restriction on what CAUSED the event rather than who it affects -- skips
// the whole line rather than guessing (GO-7): the gain proceeds normally.
func TestGainLifeNotReplacedByUnrecognizedSourceRestriction(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(gainLifeReplacementLockDef(t, "Test Rain of Gore",
		"Event$ GainLife | ActiveZones$ Battlefield | ValidSource$ SpellAbility | SourceController$ True | ReplaceWith$ RLoseLife | Description$ Life gain becomes life loss.",
		map[string]string{
			"RLoseLife": "DB$ LoseLife | LifeAmount$ X | Defined$ ReplacedPlayer",
			"X":         "ReplaceCount$LifeGained",
		}), p, engine.Battlefield)

	if err := castETBGainLife(t, g, p, etbGainLifeTriggerDefParams(t, "Test Gainer", "Defined$ You | LifeAmount$ 3", nil)); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(p).Life; got != 23 {
		t.Errorf("p life = %d, want 23 -- ValidSource$/SourceController$ is unrecognized, so the gain must proceed normally", got)
	}
}

// TestGainLifeDoubledByReplaceEffect proves applyGainLifeReplaceEffect
// (replacement.go) resolves rhox_faithmender.txt's/the_wind_crystal.txt's/
// selenia_the_cursed_heart.txt's/alhammarrets_archive.txt's/
// doctor_strange_surgeon.txt's/boon_reflection.txt's/phial_of_galadriel.txt's
// own real "if you would gain life, you gain twice that much life instead"
// (DB$ ReplaceEffect | VarName$ LifeGained | VarValue$ X,
// X:ReplaceCount$LifeGained/Twice): a 3-life gain doubles to 6, and the
// normal gain path still runs (LifeGainedTimesThisTurn/the LifeGained
// trigger), unlike a full substitution -- CR 616's own "Updated" outcome,
// not "Replaced".
func TestGainLifeDoubledByReplaceEffect(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(gainLifeReplacementLockDef(t, "Test Rhox Faithmender",
		"Event$ GainLife | ActiveZones$ Battlefield | ValidPlayer$ You | ReplaceWith$ GainDouble | Description$ Double life gain.",
		map[string]string{
			"GainDouble": "DB$ ReplaceEffect | VarName$ LifeGained | VarValue$ X",
			"X":          "ReplaceCount$LifeGained/Twice",
		}), p, engine.Battlefield)

	if err := castETBGainLife(t, g, p, etbGainLifeTriggerDefParams(t, "Test Gainer", "Defined$ You | LifeAmount$ 3", nil)); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(p).Life; got != 26 {
		t.Errorf("p life = %d, want 26 -- 3 life doubled to 6 on top of the starting 20", got)
	}
	if got := g.Player(p).LifeGainedTimesThisTurn; got != 1 {
		t.Errorf("LifeGainedTimesThisTurn = %d, want 1 -- a resized gain must still run the normal gain path, unlike a full substitution", got)
	}
}

// TestGainLifePlusOneByReplaceEffect proves angel_of_vitality.txt's/
// heron_of_hope.txt's/honor_troll.txt's/cleric_class.txt's/
// bilbo_birthday_celebrant.txt's/knight_of_dawns_light.txt's/
// leyline_of_hope.txt's/pest_rescuer.txt's own real "if you would gain life,
// you gain that much life plus 1 instead" (VarValue$ X,
// X:ReplaceCount$LifeGained/Plus.1) -- the Plus operator, not just Twice.
func TestGainLifePlusOneByReplaceEffect(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(gainLifeReplacementLockDef(t, "Test Angel of Vitality",
		"Event$ GainLife | ActiveZones$ Battlefield | ValidPlayer$ You | ReplaceWith$ GainPlusOne | Description$ Gain 1 extra life.",
		map[string]string{
			"GainPlusOne": "DB$ ReplaceEffect | VarName$ LifeGained | VarValue$ X",
			"X":           "ReplaceCount$LifeGained/Plus.1",
		}), p, engine.Battlefield)

	if err := castETBGainLife(t, g, p, etbGainLifeTriggerDefParams(t, "Test Gainer", "Defined$ You | LifeAmount$ 3", nil)); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(p).Life; got != 24 {
		t.Errorf("p life = %d, want 24 -- 3 life plus 1 on top of the starting 20", got)
	}
}

// TestGainLifeNotReplacedByChainedReplaceEffect proves a DB$ ReplaceEffect
// target naming its own SubAbility$ is refused outright too, the identical
// GO-7 refusal TestGainLifeNotReplacedByTargetAbilityWithSubAbility already
// proves for a full-substitution target: the gain proceeds unresized.
func TestGainLifeNotReplacedByChainedReplaceEffect(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(gainLifeReplacementLockDef(t, "Test Chained Double",
		"Event$ GainLife | ActiveZones$ Battlefield | ValidPlayer$ You | ReplaceWith$ GainDouble | Description$ Double life gain.",
		map[string]string{
			"GainDouble": "DB$ ReplaceEffect | VarName$ LifeGained | VarValue$ X | SubAbility$ DBLoseLife",
			"X":          "ReplaceCount$LifeGained/Twice",
			"DBLoseLife": "DB$ LoseLife | Defined$ You | LifeAmount$ 1",
		}), p, engine.Battlefield)

	if err := castETBGainLife(t, g, p, etbGainLifeTriggerDefParams(t, "Test Gainer", "Defined$ You | LifeAmount$ 3", nil)); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(p).Life; got != 23 {
		t.Errorf("p life = %d, want 23 -- a chained ReplaceWith$ target must not dispatch, so the gain must proceed unresized", got)
	}
}

// TestGainLifeNotReplacedByTargetAbilityWithSubAbility proves a ReplaceWith$
// target naming its own SubAbility$ is refused outright rather than run
// with the chained half silently dropped (GO-7): the gain proceeds
// normally.
func TestGainLifeNotReplacedByTargetAbilityWithSubAbility(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(gainLifeReplacementLockDef(t, "Test Chained Lich",
		"Event$ GainLife | ActiveZones$ Battlefield | ValidPlayer$ You | ReplaceWith$ Draw | Description$ Draw instead of gaining life.",
		map[string]string{
			"Draw":       "DB$ Draw | Defined$ You | NumCards$ Y | SubAbility$ DBLoseLife",
			"Y":          "ReplaceCount$LifeGained",
			"DBLoseLife": "DB$ LoseLife | Defined$ You | LifeAmount$ 1",
		}), p, engine.Battlefield)

	if err := castETBGainLife(t, g, p, etbGainLifeTriggerDefParams(t, "Test Gainer", "Defined$ You | LifeAmount$ 3", nil)); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(p).Life; got != 23 {
		t.Errorf("p life = %d, want 23 -- a chained ReplaceWith$ target must not dispatch, so the gain must proceed normally", got)
	}
}
