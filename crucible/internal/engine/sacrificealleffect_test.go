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

// etbSacrificeAllTriggerDefParams builds a *compile.Card whose own "when
// CARDNAME enters" trigger runs DB$ SacrificeAll with the given params
// string appended, and whose own SVars carry any further chained
// abilities -- sacrificeAllEffect itself is unexported, so every case here
// is driven through the real cast+resolve pipeline rather than calling it
// directly (TEST-1), etbSacrificeTriggerDefParams' own shape
// (sacrificeeffect_test.go).
func etbSacrificeAllTriggerDefParams(t *testing.T, name, extraParams string, svars map[string]string) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "1", "1"
	raw.Faces[0].ManaCost = mana.MustParse("G")
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigSacAll",
	}
	raw.Faces[0].SVars.Set("TrigSacAll", "DB$ SacrificeAll | "+extraParams)
	for name, body := range svars {
		raw.Faces[0].SVars.Set(name, body)
	}

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// artifactDef builds just enough of a *compile.Card for Card.Type() to
// answer "is this an Artifact, not a Creature" -- ValidCards$'s own type
// filter needs a card of the wrong type present to prove it actually
// filters rather than sacrificing everything on the battlefield.
func artifactDef(t *testing.T) *compile.Card {
	t.Helper()
	def := &compile.Card{Name: "Test Artifact"}
	def.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Artifact")
	return def
}

// TestSacrificeAllEffectValidCardsSacrificesMatchingBattlefieldWide proves
// the corpus's own dominant real shape: an absent Defined$ scans every
// battlefield in the game (Java's own `game.getCardsIn(Battlefield)`), not
// just the caster's own, and ValidCards$ filters it by type -- "Other"
// excludes the ability's own host, proving the scan is genuinely
// blanket-wide rather than happening to include the host by coincidence.
func TestSacrificeAllEffectValidCardsSacrificesMatchingBattlefieldWide(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, opp := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(opp).Life = 20, 20
	myCreature := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	oppCreature := g.NewCard(creatureDefPT(t, "1", "1"), opp, engine.Battlefield)
	myArtifact := g.NewCard(artifactDef(t), p, engine.Battlefield)

	c := engine.NewScriptedController()
	def := etbSacrificeAllTriggerDefParams(t, "Test ValidCards Blanket", "ValidCards$ Creature.Other", nil)
	host, err := castETBSacrifice(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(myCreature).Zone != engine.Graveyard {
		t.Errorf("my creature zone = %v, want Graveyard", g.Card(myCreature).Zone)
	}
	if g.Card(oppCreature).Zone != engine.Graveyard {
		t.Errorf("opponent's creature zone = %v, want Graveyard -- the scan must cover every battlefield, not just the caster's", g.Card(oppCreature).Zone)
	}
	if g.Card(myArtifact).Zone != engine.Battlefield {
		t.Errorf("my artifact zone = %v, want Battlefield -- ValidCards$ Creature must not match a non-creature", g.Card(myArtifact).Zone)
	}
	if g.Card(host).Zone != engine.Battlefield {
		t.Errorf("host zone = %v, want Battlefield -- Creature.Other must exclude the ability's own host", g.Card(host).Zone)
	}
}

// TestSacrificeAllEffectControllerNarrowsToOnePlayer proves Controller$
// filters the already-gathered set down to cards controlled by one of its
// own resolved players (definedPlayers), on top of ValidCards$'s own
// filter, rather than replacing it.
func TestSacrificeAllEffectControllerNarrowsToOnePlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, opp := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(opp).Life = 20, 20
	myCreature := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	oppCreature := g.NewCard(creatureDefPT(t, "1", "1"), opp, engine.Battlefield)

	c := engine.NewScriptedController()
	def := etbSacrificeAllTriggerDefParams(t, "Test Controller Filter", "ValidCards$ Creature.Other | Controller$ You", nil)
	if _, err := castETBSacrifice(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(myCreature).Zone != engine.Graveyard {
		t.Errorf("my creature zone = %v, want Graveyard", g.Card(myCreature).Zone)
	}
	if g.Card(oppCreature).Zone != engine.Battlefield {
		t.Errorf("opponent's creature zone = %v, want Battlefield -- Controller$ You must narrow the sacrifice to my own permanents", g.Card(oppCreature).Zone)
	}
}

// TestSacrificeAllEffectDefinedSacrificesSpecificCards proves the Defined$
// branch itself (definedCards, defined.go), SacrificeAllEffect.resolve's
// own `hasParam("Defined")` half -- a code path distinct from the plain
// Sacrifice effect's own "Self" default even though both name the host
// here.
func TestSacrificeAllEffectDefinedSacrificesSpecificCards(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	def := etbSacrificeAllTriggerDefParams(t, "Test Defined Self", "Defined$ Self", nil)
	host, err := castETBSacrifice(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(host).Zone != engine.Graveyard {
		t.Errorf("host zone = %v, want Graveyard -- Defined$ Self must sacrifice the host", g.Card(host).Zone)
	}
}

// TestSacrificeAllEffectRejectsUnresolvedDefinedValue proves a real
// Defined$ value definedCards does not recognize (TriggeredObjectLKICopy,
// a real corpus value on other SacrificeAll lines) fails the whole line
// loudly rather than silently sacrificing nothing (PORT-8/GO-7).
func TestSacrificeAllEffectRejectsUnresolvedDefinedValue(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	def := etbSacrificeAllTriggerDefParams(t, "Test Unresolved Defined", "Defined$ TriggeredObjectLKICopy", nil)
	_, err := castETBSacrifice(t, g, p, def, c)
	if err == nil {
		t.Fatal("ResolveStack: got nil error, want one naming Defined")
	}
	if !strings.Contains(err.Error(), "Defined") {
		t.Errorf("ResolveStack error = %q, want it to name Defined$", err.Error())
	}
}

// TestSacrificeAllEffectRejectsUnlessCost proves a real, representative
// unresolved param (UnlessCost$, sacrificeAllUnresolvedParams' own list)
// fails the whole line loudly rather than silently sacrificing
// unconditionally (PORT-8/GO-7).
func TestSacrificeAllEffectRejectsUnlessCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	def := etbSacrificeAllTriggerDefParams(t, "Test UnlessCost", "ValidCards$ Creature | UnlessCost$ 1", nil)
	_, err := castETBSacrifice(t, g, p, def, c)
	if err == nil {
		t.Fatal("ResolveStack: got nil error, want one naming UnlessCost")
	}
	if !strings.Contains(err.Error(), "UnlessCost") {
		t.Errorf("ResolveStack error = %q, want it to name UnlessCost$", err.Error())
	}
}

// TestSacrificeAllEffectChainsIntoSubAbility proves SubAbility$ chains
// through resolveSubAbility (subability.go, Registry.Resolve, effect.go)
// once SacrificeAll's own body finishes, the identical shape every other
// M6 effect already has.
func TestSacrificeAllEffectChainsIntoSubAbility(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := etbSacrificeAllTriggerDefParams(t, "Test SubAbility",
		"Defined$ Self | SubAbility$ DBGainLife",
		map[string]string{"DBGainLife": "DB$ GainLife | Defined$ You | LifeAmount$ 2"})
	c := engine.NewScriptedController()
	host, err := castETBSacrifice(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(host).Zone != engine.Graveyard {
		t.Errorf("host zone = %v, want Graveyard", g.Card(host).Zone)
	}
	if g.Player(p).Life != 22 {
		t.Errorf("p's life = %d, want 22 -- the chained GainLife must run too", g.Player(p).Life)
	}
}

// TestSacrificeAllEffectRememberSacrificedWritesMemoryForEveryCard proves
// RememberSacrificed$ writes every sacrificed card, not just one, onto the
// ability's own host card's Memory (Card.Memory, memory.go) --
// sacrificeCards' own per-card loop (sacrificeeffect.go), shared with the
// plain Sacrifice effect, called once per card in the whole blanket set.
func TestSacrificeAllEffectRememberSacrificedWritesMemoryForEveryCard(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	other1 := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	other2 := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	def := etbSacrificeAllTriggerDefParams(t, "Test Remember Every Card", "ValidCards$ Creature.Other | RememberSacrificed$ True", nil)
	host, err := castETBSacrifice(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	remembered := g.Card(host).Memory.Remembered()
	want := map[engine.EntityID]bool{engine.CardEntity(other1): true, engine.CardEntity(other2): true}
	if len(remembered) != 2 {
		t.Fatalf("host's Remembered() = %v, want 2 entries", remembered)
	}
	for _, e := range remembered {
		if !want[e] {
			t.Errorf("host's Remembered() contains unexpected entity %v", e)
		}
	}
}

// TestSacrificeAllEffectFiresSacrificedTriggerPerCard proves Mode$
// Sacrificed (checkSacrificedTriggers, trigger.go) fires once per card
// actually sacrificed in the blanket set, not once for the whole
// SacrificeAll ability -- a watching permanent's own life-gain trigger
// fires twice for two sacrificed creatures.
func TestSacrificeAllEffectFiresSacrificedTriggerPerCard(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	raw := &carddb.Card{Filename: "Test Watcher"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Watcher"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Enchantment")
	raw.Faces[0].Triggers = []string{
		"Mode$ Sacrificed | ValidCard$ Creature | Execute$ TrigGain",
	}
	raw.Faces[0].SVars.Set("TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 1")
	watcherDef, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile watcher: %v", err)
	}
	g.NewCard(watcherDef, p, engine.Battlefield)
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	def := etbSacrificeAllTriggerDefParams(t, "Test Trigger Per Card", "ValidCards$ Creature.Other", nil)
	if _, err := castETBSacrifice(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Player(p).Life != 22 {
		t.Errorf("p's life = %d, want 22 -- Mode$ Sacrificed must fire once for each of the two sacrificed creatures", g.Player(p).Life)
	}
}
