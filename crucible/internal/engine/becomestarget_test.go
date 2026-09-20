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

// becomesTargetCreatureDef builds a *compile.Card for a creature carrying a
// real "whenever CARDNAME becomes the target of a spell or ability" trigger
// (Illusionary Servant's/Task Force's own real shape) that gains its
// controller 5 life when it fires -- an inert, always-succeeding Execute$ so
// a test can tell "the trigger fired" apart from "the effect it named
// happened to also work," the identical reason draweffect_test.go's/
// pumpeffect_test.go's own ETB helpers use a single simple effect rather
// than whatever the real card's own Execute$ names.
func becomesTargetCreatureDef(t *testing.T, name, validTarget, extraTriggerParams string) *compile.Card {
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
	trigger := "Mode$ BecomesTarget | ValidTarget$ " + validTarget
	if extraTriggerParams != "" {
		trigger += " | " + extraTriggerParams
	}
	trigger += " | Execute$ TrigGainLife"
	raw.Faces[0].Triggers = []string{trigger}
	raw.Faces[0].SVars.Set("TrigGainLife", "DB$ GainLife | Defined$ You | LifeAmount$ 5")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestPushTriggeredAbilitiesFiresBecomesTargetOnCardTarget proves
// checkBecomesTargetTriggers (trigger.go) is wired into pushTriggeredAbilities
// right after PushAbility: an ETB trigger's own DB$ LoseLife | ValidTgts$
// Creature.YouCtrl chooses a creature target (resolveTargets, targeting.go),
// and that target's own "becomes the target" trigger fires as a direct
// result -- LoseLife itself finds no player among a card-shaped target and
// does nothing, isolating the life change to the BecomesTarget trigger's own
// GainLife alone.
func TestPushTriggeredAbilitiesFiresBecomesTargetOnCardTarget(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20

	watcher := g.NewCard(becomesTargetCreatureDef(t, "Test Watcher", "Card.Self", ""), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(watcher)})
	def := etbLoseLifeTriggerDefParams(t, "Test BecomesTarget", "ValidTgts$ Creature.YouCtrl | LifeAmount$ 1", nil)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if !g.Card(watcher).BecameTargetThisTurn {
		t.Error("watcher.BecameTargetThisTurn = false, want true")
	}
	if got := g.Player(p).Life; got != 25 {
		t.Errorf("p life = %d, want 25 (20 + 5 from the BecomesTarget-triggered GainLife)", got)
	}
}

// TestCastAuraFiresBecomesTargetTriggerOnEnchantedCreature proves
// castAura's own checkBecomesTargetTriggers call (castspell.go): an Aura's
// own cast-time attach target is CR 115's "becomes the target of a spell or
// ability" exactly as much as a triggered ability's chosen target is.
func TestCastAuraFiresBecomesTargetTriggerOnEnchantedCreature(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life = 20

	target := g.NewCard(becomesTargetCreatureDef(t, "Test Watcher", "Card.Self", ""), p, engine.Battlefield)
	aura := g.NewCard(auraDefWithEnchant(t, "Creature"), p, engine.Hand)
	c := engine.NewScriptedController()

	if !g.CastSpell(p, aura, c) {
		t.Fatal("CastSpell failed casting an Aura with exactly one legal target")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if !g.Card(target).BecameTargetThisTurn {
		t.Error("target.BecameTargetThisTurn = false, want true")
	}
	if got := g.Player(p).Life; got != 25 {
		t.Errorf("p life = %d, want 25 (20 + 5 from the BecomesTarget-triggered GainLife)", got)
	}
}

// TestBecomesTargetFirstTimeOnlyFiresOnceEachTurn proves FirstTime$ reads
// the new Card.BecameTargetThisTurn (card.go): Glyph Keeper's own real
// "for the first time each turn" shape fires on the first targeting event
// and stays silent on a second one the same turn.
func TestBecomesTargetFirstTimeOnlyFiresOnceEachTurn(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20

	watcher := g.NewCard(becomesTargetCreatureDef(t, "Test Watcher", "Card.Self", "FirstTime$ True"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	g.Player(p).ManaPool.Add(mana.Green, 2)

	c.QueueTargets([]engine.EntityID{engine.CardEntity(watcher)})
	first := g.NewCard(etbLoseLifeTriggerDefParams(t, "Test First", "ValidTgts$ Creature.YouCtrl | LifeAmount$ 1", nil), p, engine.Hand)
	if !g.CastSpell(p, first, c) {
		t.Fatal("CastSpell failed (first cast)")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack (first): %v", err)
	}
	if got := g.Player(p).Life; got != 25 {
		t.Fatalf("p life after first targeting = %d, want 25", got)
	}

	c.QueueTargets([]engine.EntityID{engine.CardEntity(watcher)})
	second := g.NewCard(etbLoseLifeTriggerDefParams(t, "Test Second", "ValidTgts$ Creature.YouCtrl | LifeAmount$ 1", nil), p, engine.Hand)
	if !g.CastSpell(p, second, c) {
		t.Fatal("CastSpell failed (second cast)")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack (second): %v", err)
	}
	if got := g.Player(p).Life; got != 25 {
		t.Errorf("p life after second targeting this turn = %d, want unchanged 25 -- "+
			"FirstTime$ must not fire a second time in the same turn", got)
	}
}

// TestBecomesTargetSkipsWhenValidTargetDoesNotMatch proves the
// Matches-driven skip every other trigger mode already has: a watcher whose
// own ValidTarget$ names the opponent's creatures never fires for a card the
// watcher's own controller targets. The card still genuinely became a
// target (BecameTargetThisTurn is set unconditionally, Java's own
// addTargetFromThisTurn runs before any trigger is even checked) -- only the
// trigger itself stays silent.
func TestBecomesTargetSkipsWhenValidTargetDoesNotMatch(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20

	watcher := g.NewCard(becomesTargetCreatureDef(t, "Test Watcher", "Creature.OppCtrl", ""), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(watcher)})
	def := etbLoseLifeTriggerDefParams(t, "Test No Match", "ValidTgts$ Creature.YouCtrl | LifeAmount$ 1", nil)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(p).Life; got != 20 {
		t.Errorf("p life = %d, want unchanged 20 -- ValidTarget$ Creature.OppCtrl must not match a creature its own controller targeted", got)
	}
	if !g.Card(watcher).BecameTargetThisTurn {
		t.Error("watcher.BecameTargetThisTurn = false, want true -- the card still became a target even though no trigger matched it")
	}
}

// TestBecomesTargetSkipsUnresolvedValidSource proves the PORT-8 skip for
// ValidSource$ (77 of the corpus's own 118 real Mode$ BecomesTarget lines):
// a line naming it never fires, rather than matching as if the restriction
// were not there.
func TestBecomesTargetSkipsUnresolvedValidSource(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20

	watcher := g.NewCard(becomesTargetCreatureDef(t, "Test Watcher", "Card.Self", "ValidSource$ Spell.OppCtrl"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(watcher)})
	def := etbLoseLifeTriggerDefParams(t, "Test ValidSource", "ValidTgts$ Creature.YouCtrl | LifeAmount$ 1", nil)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(p).Life; got != 20 {
		t.Errorf("p life = %d, want unchanged 20 -- ValidSource$ is unresolved and must skip the whole line, not match unconditionally", got)
	}
}
