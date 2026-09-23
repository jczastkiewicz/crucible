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

// etbDamageAllTriggerDefParams builds a *compile.Card whose own "when
// CARDNAME enters" trigger runs DB$ DamageAll with the given params string
// appended -- etbDestroyTriggerDefParams' own shape (destroyeffect_test.go).
func etbDamageAllTriggerDefParams(t *testing.T, name, extraParams string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "1", "10"
	raw.Faces[0].ManaCost = mana.MustParse("G")
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigDamageAll",
	}
	raw.Faces[0].SVars.Set("TrigDamageAll", "DB$ DamageAll | "+extraParams)

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// castETBDamageAll casts def for p on controller c and resolves the stack --
// castETBDestroy's own shape (destroyeffect_test.go).
func castETBDamageAll(t *testing.T, g *engine.Game, p engine.PlayerID, def *compile.Card, c *engine.ScriptedController) (engine.CardID, error) {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	return creature, g.ResolveStack(engine.NewRegistry(), c)
}

// TestDamageAllEffectValidCardsHitsEveryMatchingCreature proves ValidCards$'
// own battlefield-wide scan: every creature this port can see, not just the
// activator's own, per the identical Creature.StrictlyOther-shaped real
// corpus line.
func TestDamageAllEffectValidCardsHitsEveryMatchingCreature(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	mine := g.NewCard(creatureDefPT(t, "2", "10"), p, engine.Battlefield)
	theirs := g.NewCard(creatureDefPT(t, "2", "10"), other, engine.Battlefield)

	c := engine.NewScriptedController()
	_, err := castETBDamageAll(t, g, p, etbDamageAllTriggerDefParams(t, "Test DamageAll Cards", "NumDmg$ 3 | ValidCards$ Creature.StrictlyOther"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(mine).Damage.Marked; got != 3 {
		t.Errorf("mine damage = %d, want 3", got)
	}
	if got := g.Card(theirs).Damage.Marked; got != 3 {
		t.Errorf("theirs damage = %d, want 3", got)
	}
}

// TestDamageAllEffectValidPlayersHitsEveryMatchingPlayer proves
// ValidPlayers$' own player-side sweep, independent of ValidCards$.
func TestDamageAllEffectValidPlayersHitsEveryMatchingPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20

	c := engine.NewScriptedController()
	_, err := castETBDamageAll(t, g, p, etbDamageAllTriggerDefParams(t, "Test DamageAll Players", "NumDmg$ 2 | ValidPlayers$ Player.Opponent"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(other).Life; got != 18 {
		t.Errorf("other life = %d, want 18", got)
	}
	if got := g.Player(p).Life; got != 20 {
		t.Errorf("p life = %d, want 20 (unaffected, not their own opponent)", got)
	}
}

// TestDamageAllEffectValidCardsScopesToOneMatch proves ValidCards$'s own
// spec is a real filter, not a blanket "every creature" sweep --
// Creature.Self matches only the ability's own host, leaving an unrelated
// creature on the same battlefield untouched. DamageSource$ Self here
// matches definedCards's own default (the file doc comment's own reason
// 288 of 307 real lines omit it), so this doubles as that default's own
// proof too.
func TestDamageAllEffectValidCardsScopesToOneMatch(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	other := g.NewCard(creatureDefPT(t, "9", "10"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	_, err := castETBDamageAll(t, g, p, etbDamageAllTriggerDefParams(t, "Test DamageAll Source", "NumDmg$ 4 | ValidCards$ Creature.Self | DamageSource$ Self"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(other).Damage.Marked; got != 0 {
		t.Errorf("other damage = %d, want 0 (ValidCards$ Creature.Self only matches the host)", got)
	}
}

// TestDamageAllEffectRejectsUnresolvedParam proves damageAllUnresolvedParams'
// own fail-loud contract (PORT-8/GO-7).
func TestDamageAllEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	_, err := castETBDamageAll(t, g, p, etbDamageAllTriggerDefParams(t, "Test DamageAll Ultimate", "NumDmg$ 1 | ValidPlayers$ Player | Ultimate$ True"), c)
	if err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved Ultimate$")
	}
}
