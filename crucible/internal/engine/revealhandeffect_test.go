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

// etbRevealHandTriggerDefParams builds a *compile.Card whose own "when
// CARDNAME enters" trigger runs DB$ RevealHand with the given params string
// appended -- etbDestroyTriggerDefParams' own shape (destroyeffect_test.go).
func etbRevealHandTriggerDefParams(t *testing.T, name, extraParams string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "1", "1"
	raw.Faces[0].ManaCost = mana.MustParse("G")
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigRevealHand",
	}
	raw.Faces[0].SVars.Set("TrigRevealHand", "DB$ RevealHand | "+extraParams)

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// castETBRevealHand casts def for p on controller c and resolves the stack
// -- castETBDestroy's own shape (destroyeffect_test.go).
func castETBRevealHand(t *testing.T, g *engine.Game, p engine.PlayerID, def *compile.Card, c *engine.ScriptedController) (engine.CardID, error) {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	return creature, g.ResolveStack(engine.NewRegistry(), c)
}

// TestRevealHandEffectRememberRevealedRemembersEveryCardInHand proves
// RememberRevealed$'s own real state change: every card in the revealed
// player's hand lands in the host's own Memory.
func TestRevealHandEffectRememberRevealedRemembersEveryCardInHand(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	c1 := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Hand)
	c2 := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Hand)

	c := engine.NewScriptedController()
	host, err := castETBRevealHand(t, g, p, etbRevealHandTriggerDefParams(t, "Test RevealHand Remember", "Defined$ Opponent | RememberRevealed$ True"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	remembered := g.Card(host).Memory.Remembered()
	if len(remembered) != 2 {
		t.Fatalf("remembered = %v, want 2 entries", remembered)
	}
	got := map[engine.EntityID]bool{remembered[0]: true, remembered[1]: true}
	if !got[engine.CardEntity(c1)] || !got[engine.CardEntity(c2)] {
		t.Errorf("remembered = %v, want both %v and %v", remembered, c1, c2)
	}
}

// TestRevealHandEffectRememberRevealedPlayerRemembersThePlayer proves
// RememberRevealedPlayer$'s own real state change: the revealed player
// itself lands in the host's own Memory.
func TestRevealHandEffectRememberRevealedPlayerRemembersThePlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20

	c := engine.NewScriptedController()
	host, err := castETBRevealHand(t, g, p, etbRevealHandTriggerDefParams(t, "Test RevealHand RememberPlayer", "Defined$ Opponent | RememberRevealedPlayer$ True"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	remembered := g.Card(host).Memory.Remembered()
	if len(remembered) != 1 || remembered[0] != engine.PlayerEntity(other) {
		t.Errorf("remembered = %v, want [%v]", remembered, engine.PlayerEntity(other))
	}
}

// TestRevealHandEffectNoRememberParamIsANoOp proves revealing with neither
// Remember param does nothing this port tracks -- no hidden information to
// actually reveal, revealhandeffect.go's own doc comment.
func TestRevealHandEffectNoRememberParamIsANoOp(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Hand)

	c := engine.NewScriptedController()
	host, err := castETBRevealHand(t, g, p, etbRevealHandTriggerDefParams(t, "Test RevealHand NoRemember", "Defined$ Opponent"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if remembered := g.Card(host).Memory.Remembered(); len(remembered) != 0 {
		t.Errorf("remembered = %v, want none", remembered)
	}
}

// TestRevealHandEffectRejectsUnresolvedParam proves revealHandUnresolvedParams'
// own fail-loud contract (PORT-8/GO-7).
func TestRevealHandEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	_, err := castETBRevealHand(t, g, p, etbRevealHandTriggerDefParams(t, "Test RevealHand Optional", "Defined$ You | RememberRevealed$ True | Optional$ True"), c)
	if err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved Optional$")
	}
}
