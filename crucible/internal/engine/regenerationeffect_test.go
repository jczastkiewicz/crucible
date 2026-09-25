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

// regenerationCreatureDef builds a 2/2 creature carrying a real
// mossbridge_troll.txt-shaped R:Event$ Destroy | Regeneration$ True line
// (or, for the negative cases, whatever replacements is instead) plus an
// ETB trigger that runs trig against itself -- destroySelf's own shape,
// reused so every case here drives the actual destroy call sites
// (destroyEffect/destroyDamagedCreatures) rather than calling
// destroyReplacedByRegeneration directly (regeneration.go's own functions
// are unexported, TEST-1).
func regenerationCreatureDef(t *testing.T, name, trig string, replacements []string) *compile.Card {
	t.Helper()
	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "2", "2"
	raw.Faces[0].ManaCost = mana.MustParse("G")
	raw.Faces[0].Replacements = replacements
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ Trig",
	}
	raw.Faces[0].SVars.Set("Trig", trig)
	raw.Faces[0].SVars.Set("DBRegen", "DB$ Regeneration | Defined$ ReplacedCard")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

const mossbridgeReplacement = "Event$ Destroy | ActiveZones$ Battlefield | ValidCard$ Card.Self | Regeneration$ True | ReplaceWith$ DBRegen"

// TestRegenerationReplacementSurvivesDestroy proves mossbridge_troll.txt:5-6's
// own real shape end to end: an ordinary DB$ Destroy is replaced with
// regeneration -- the creature stays on the battlefield, tapped, with its
// damage cleared, and no shield (RegenShields) is ever granted or spent.
func TestRegenerationReplacementSurvivesDestroy(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	def := regenerationCreatureDef(t, "Test Mossbridge",
		"DB$ Destroy | Defined$ Self", []string{mossbridgeReplacement})
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	card := g.Card(host)
	if card.Zone != engine.Battlefield {
		t.Fatalf("zone = %v, want Battlefield -- the destroy should have been replaced", card.Zone)
	}
	if !card.Tapped {
		t.Error("Tapped = false, want true -- regeneration taps the permanent")
	}
	if card.RegenShields != 0 {
		t.Errorf("RegenShields = %d, want 0 -- this shape never grants or spends a shield", card.RegenShields)
	}
}

// TestRegenerationReplacementAcceptsDefinedSelf proves Defined$ Self --
// 0 real corpus lines, but the identical substitution
// AbilityUtils.getDefinedCards would make for ReplacedCard given every real
// line's own ValidCard$ Card.Self -- is recognized too.
func TestRegenerationReplacementAcceptsDefinedSelf(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test Mossbridge Defined Self"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = raw.Filename
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "2", "2"
	raw.Faces[0].ManaCost = mana.MustParse("G")
	raw.Faces[0].Replacements = []string{mossbridgeReplacement}
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ Trig",
	}
	raw.Faces[0].SVars.Set("Trig", "DB$ Destroy | Defined$ Self")
	raw.Faces[0].SVars.Set("DBRegen", "DB$ Regeneration | Defined$ Self")
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(host).Zone; z != engine.Battlefield {
		t.Errorf("zone = %v, want Battlefield -- Defined$ Self must regenerate too", z)
	}
}

// TestRegenerationCantRegenerateBlocksStaticReplacement proves Mode$
// CantRegenerate (knight_of_the_holy_nimbus.txt's/clergy_of_the_holy_nimbus
// .txt's own opponent-only "{N}: CARDNAME can't be regenerated this turn")
// blocks the free static replacement too, not just a shield -- Java's own
// Card.canRegenerate checks it before either source.
func TestRegenerationCantRegenerateBlocksStaticReplacement(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test Mossbridge Locked"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = raw.Filename
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "2", "2"
	raw.Faces[0].ManaCost = mana.MustParse("G")
	raw.Faces[0].Replacements = []string{mossbridgeReplacement}
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ Trig",
	}
	raw.Faces[0].SVars.Set("Trig", "DB$ Effect | RememberObjects$ Self | StaticAbilities$ NoRegen | SubAbility$ DBDestroy")
	raw.Faces[0].SVars.Set("NoRegen", "Mode$ CantRegenerate | ValidCard$ Card.IsRemembered")
	raw.Faces[0].SVars.Set("DBDestroy", "DB$ Destroy | Defined$ Self")
	raw.Faces[0].SVars.Set("DBRegen", "DB$ Regeneration | Defined$ ReplacedCard")
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(host).Zone; z != engine.Graveyard {
		t.Errorf("zone = %v, want Graveyard -- CantRegenerate must override the free static replacement too", z)
	}
}

// TestRegenerationCantRegenerateRequiresValidCard proves a Mode$
// CantRegenerate static naming no ValidCard$ at all (0 real corpus lines,
// but cardCantRegenerate's own defensive branch) does not block anything --
// it is simply skipped, not treated as a blanket ban.
func TestRegenerationCantRegenerateRequiresValidCard(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test Mossbridge Bare CantRegen"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = raw.Filename
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "2", "2"
	raw.Faces[0].ManaCost = mana.MustParse("G")
	raw.Faces[0].Replacements = []string{mossbridgeReplacement}
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ Trig",
	}
	raw.Faces[0].SVars.Set("Trig", "DB$ Effect | StaticAbilities$ NoRegen | SubAbility$ DBDestroy")
	raw.Faces[0].SVars.Set("NoRegen", "Mode$ CantRegenerate")
	raw.Faces[0].SVars.Set("DBDestroy", "DB$ Destroy | Defined$ Self")
	raw.Faces[0].SVars.Set("DBRegen", "DB$ Regeneration | Defined$ ReplacedCard")
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(host).Zone; z != engine.Battlefield {
		t.Errorf("zone = %v, want Battlefield -- a ValidCard$-less CantRegenerate must not block anything", z)
	}
}

// TestRegenerationReplacementClearsDamage proves the state-based lethal
// damage destruction (destroyDamagedCreatures, action.go) is replaced too,
// and that the marked damage is actually removed, not just the destroy
// skipped.
func TestRegenerationReplacementClearsDamage(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	def := regenerationCreatureDef(t, "Test Mossbridge Damage",
		"DB$ DealDamage | Defined$ Self | NumDmg$ 5", []string{mossbridgeReplacement})
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	card := g.Card(host)
	if card.Zone != engine.Battlefield || card.Damage.Marked != 0 {
		t.Errorf("zone %v damage %d, want Battlefield 0", card.Zone, card.Damage.Marked)
	}
	if !card.Tapped {
		t.Error("Tapped = false, want true")
	}
}

// TestRegenerationReplacementSkippedByNoRegen proves NoRegen$ on the Destroy
// ability itself still overrides this replacement the same way it overrides
// a shield (TestRegenerateEffectNoRegenIgnoresShield) -- the destroy call
// sites never call regenerate at all once NoRegen$ is set, so the static
// replacement never gets a chance to apply either.
func TestRegenerationReplacementSkippedByNoRegen(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	def := regenerationCreatureDef(t, "Test Mossbridge NoRegen",
		"DB$ Destroy | Defined$ Self | NoRegen$ True", []string{mossbridgeReplacement})
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(host).Zone; z != engine.Graveyard {
		t.Errorf("zone = %v, want Graveyard -- NoRegen$ must skip this replacement too", z)
	}
}

// TestRegenerationReplacementIgnoresUnrelatedReplacement proves the
// replacement loop (destroyReplacedByRegeneration) correctly skips a
// same-card replacement whose Event$ does not match ("Moved" here) and
// still finds the real Destroy/Regeneration$ True line further down the
// list -- exercising both the false and true return of
// regenerationReplacementMatches's own Name check in one card.
func TestRegenerationReplacementIgnoresUnrelatedReplacement(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	def := regenerationCreatureDef(t, "Test Mossbridge Plus Moved",
		"DB$ Destroy | Defined$ Self", []string{
			"Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield | ReplaceWith$ DBRegen",
			mossbridgeReplacement,
		})
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(host).Zone; z != engine.Battlefield {
		t.Errorf("zone = %v, want Battlefield", z)
	}
}

// TestRegenerationReplacementRejectsUnrecognizedDefined proves a
// ReplaceWith$ ability naming a Defined$ value past ReplacedCard/Self is
// refused rather than guessed at (GO-7): the destroy proceeds normally.
func TestRegenerationReplacementRejectsUnrecognizedDefined(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test Mossbridge Bad Defined"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Mossbridge Bad Defined"
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "2", "2"
	raw.Faces[0].ManaCost = mana.MustParse("G")
	raw.Faces[0].Replacements = []string{mossbridgeReplacement}
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ Trig",
	}
	raw.Faces[0].SVars.Set("Trig", "DB$ Destroy | Defined$ Self")
	raw.Faces[0].SVars.Set("DBRegen", "DB$ Regeneration | Defined$ TargetedCard")
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(host).Zone; z != engine.Graveyard {
		t.Errorf("zone = %v, want Graveyard -- an unrecognized Defined$ must not regenerate", z)
	}
}

// TestRegenerationReplacementRejectsUnexpectedParam proves an R:Event$
// Destroy | Regeneration$ True line naming a param past the corpus's own
// real six (Event/ActiveZones/ValidCard/Regeneration/ReplaceWith/
// Description) is refused rather than applied unconditionally (GO-7).
func TestRegenerationReplacementRejectsUnexpectedParam(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	def := regenerationCreatureDef(t, "Test Mossbridge Extra Param",
		"DB$ Destroy | Defined$ Self", []string{
			mossbridgeReplacement + " | PlayerTurn$ True",
		})
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(host).Zone; z != engine.Graveyard {
		t.Errorf("zone = %v, want Graveyard -- an unrecognized param must not regenerate", z)
	}
}

// TestRegenerationReplacementRequiresValidCard proves a line naming no
// ValidCard$ at all, or one that does not match host, leaves the
// replacement inert -- Matches's own false return and the missing-param
// branch both covered.
func TestRegenerationReplacementRequiresValidCard(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"NoValidCard":     "Event$ Destroy | Regeneration$ True | ReplaceWith$ DBRegen",
		"NonMatchingCard": "Event$ Destroy | ValidCard$ Card.Token | Regeneration$ True | ReplaceWith$ DBRegen",
	}
	for name, replacement := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			c := engine.NewScriptedController()
			def := regenerationCreatureDef(t, "Test Mossbridge "+name,
				"DB$ Destroy | Defined$ Self", []string{replacement})
			host, err := castETBChain(t, g, p, def, c)
			if err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if z := g.Card(host).Zone; z != engine.Graveyard {
				t.Errorf("zone = %v, want Graveyard", z)
			}
		})
	}
}

// TestRegenerationReplacementRequiresRegenerationTrue proves a line naming
// Event$ Destroy but no Regeneration$ True (r.Param("Regeneration")'s own
// !ok/not-True branches) does not regenerate.
func TestRegenerationReplacementRequiresRegenerationTrue(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"Missing": "Event$ Destroy | ValidCard$ Card.Self | ReplaceWith$ DBRegen",
		"False":   "Event$ Destroy | ValidCard$ Card.Self | Regeneration$ False | ReplaceWith$ DBRegen",
	}
	for name, replacement := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			c := engine.NewScriptedController()
			def := regenerationCreatureDef(t, "Test Mossbridge Regen"+name,
				"DB$ Destroy | Defined$ Self", []string{replacement})
			host, err := castETBChain(t, g, p, def, c)
			if err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if z := g.Card(host).Zone; z != engine.Graveyard {
				t.Errorf("zone = %v, want Graveyard", z)
			}
		})
	}
}

// TestRegenerationSubAbilityAcceptsBareOrDefinedOmitted proves a bare
// `DB$ Regeneration` with no Defined$ at all defaults to Self
// (AbilityUtils.getDefinedCards' own default, the identical convention
// targetedOrDefinedCards already has) and still regenerates.
func TestRegenerationSubAbilityAcceptsBareOrDefinedOmitted(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test Mossbridge Bare"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = raw.Filename
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "2", "2"
	raw.Faces[0].ManaCost = mana.MustParse("G")
	raw.Faces[0].Replacements = []string{mossbridgeReplacement}
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ Trig",
	}
	raw.Faces[0].SVars.Set("Trig", "DB$ Destroy | Defined$ Self")
	raw.Faces[0].SVars.Set("DBRegen", "DB$ Regeneration")
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(host).Zone; z != engine.Battlefield {
		t.Errorf("zone = %v, want Battlefield -- a bare DB$ Regeneration defaults Defined$ to Self", z)
	}
}

// TestRegenerationSubAbilityRejectsWrongNameAndExtraParams proves a
// ReplaceWith$ pointing to something other than DB$ Regeneration, and one
// pointing to DB$ Regeneration with a param past Defined$, both leave the
// replacement unrecognized (GO-7): the destroy proceeds normally.
func TestRegenerationSubAbilityRejectsWrongNameAndExtraParams(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	for _, tc := range []struct {
		name string
		sub  string
	}{
		{"WrongName", "DB$ Tap | Defined$ ReplacedCard"},
		{"ExtraParam", "DB$ Regeneration | Defined$ ReplacedCard | RememberObjects$ Self"},
		{"SubAbilityChain", "DB$ Regeneration | Defined$ ReplacedCard | SubAbility$ DBNoop"},
	} {
		raw := &carddb.Card{Filename: "Test Mossbridge " + tc.name}
		raw.Faces[0].Present = true
		raw.Faces[0].Name = raw.Filename
		raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
		raw.Faces[0].Power, raw.Faces[0].Toughness = "2", "2"
		raw.Faces[0].ManaCost = mana.MustParse("G")
		raw.Faces[0].Replacements = []string{mossbridgeReplacement}
		raw.Faces[0].Triggers = []string{
			"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ Trig",
		}
		raw.Faces[0].SVars.Set("Trig", "DB$ Destroy | Defined$ Self")
		raw.Faces[0].SVars.Set("DBRegen", tc.sub)
		raw.Faces[0].SVars.Set("DBNoop", "DB$ Cleanup")
		def, err := compile.Compile(raw)
		if err != nil {
			t.Fatalf("compile %s: %v", tc.name, err)
		}
		host, err := castETBChain(t, g, p, def, c)
		if err != nil {
			t.Fatalf("%s: ResolveStack: %v", tc.name, err)
		}
		if z := g.Card(host).Zone; z != engine.Graveyard {
			t.Errorf("%s: zone = %v, want Graveyard", tc.name, z)
		}
	}
}

// TestRegenerationReplacementRequiresActiveZone proves an ActiveZones$
// naming a zone the host is not actually in (hostInActiveZones,
// replacement.go) leaves the replacement inert.
func TestRegenerationReplacementRequiresActiveZone(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	def := regenerationCreatureDef(t, "Test Mossbridge Wrong Zone",
		"DB$ Destroy | Defined$ Self", []string{
			"Event$ Destroy | ActiveZones$ Command | ValidCard$ Card.Self | Regeneration$ True | ReplaceWith$ DBRegen",
		})
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(host).Zone; z != engine.Graveyard {
		t.Errorf("zone = %v, want Graveyard -- ActiveZones$ Command must not apply on the battlefield", z)
	}
}
