package engine_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// etbTriggerCreatureDef builds a *compile.Card for a creature with a real
// "when CARDNAME enters" trigger -- Elvish Visionary's own shape
// (Mode$ ChangesZone, Destination$ Battlefield, ValidCard$ Card.Self,
// Execute$ referencing a DB$ Draw sub-ability), compiled through the real
// pipeline (compile.Compile) rather than hand-built field by field, so a
// change to how M3 compiles a T:/SVar: pair into compile.Ability/SubRef would
// break this test the same way it would break any other card.
func etbTriggerCreatureDef(t *testing.T, name, cost string) *compile.Card {
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
	raw.Faces[0].ManaCost = mana.MustParse(cost)
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestCastSpellFiresETBTrigger casts a creature carrying a real "when
// CARDNAME enters, draw a card" trigger and proves the whole pipeline end to
// end: checkETBTriggers (trigger.go) detects it and pushes its own Execute$
// sub-ability (Draw) onto the stack with Ability.Params carrying
// Defined$/NumCards$ (ability.go), and ResolveStack finds it on top the
// instant the permanent spell's own resolution returns and resolves it for
// real -- drawEffect (draweffect.go), M6's first implemented effect and
// trigger.go's first Execute$ sub-ability to do more than surface
// ErrUnimplemented by name.
//
// Two players, not one, and both with Life set explicitly: CheckStateBasedActions
// (run after every resolved ability, stack.go) declares a lone remaining
// player the winner and sets Game.Over the moment it finds exactly one --
// true in a one-player game as soon as the first ability resolves, and just
// as true in a two-player game if either player's own Life is left at
// NewGame's own zero value (0 <= 0 loses immediately, action.go). Either
// way, Over stops ResolveStack's own loop (`for len(g.stack) > 0 &&
// !g.over`) before it ever reaches the pushed trigger, and this test would
// pass for the wrong reason: not because the trigger correctly drew a card,
// but because the game ended first.
func TestCastSpellFiresETBTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.Player(p).ManaPool.Add(mana.Green, 1)
	g.Player(p).ManaPool.AddColorless(1)
	creature := g.NewCard(etbTriggerCreatureDef(t, "Test Visionary", "1 G"), p, engine.Hand)
	topOfLibrary := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardC)

	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}

	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(creature).Zone != engine.Battlefield {
		t.Errorf("creature zone = %v, want Battlefield", g.Card(creature).Zone)
	}
	if g.Card(topOfLibrary).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- the ETB trigger's own Draw should have resolved", g.Card(topOfLibrary).Zone)
	}
	if g.StackLen() != 0 {
		t.Errorf("StackLen() = %d, want 0", g.StackLen())
	}
}

// A trigger whose ValidCard does not match the card that entered never
// pushes anything -- Matches (valid.go) doing its job, the same evaluator
// every other trigger-adjacent check (state-based actions, enchantSpec)
// already trusts.
func TestCastSpellSkipsNonMatchingTrigger(t *testing.T) {
	t.Parallel()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test Watcher"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Watcher"
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "1", "1"
	raw.Faces[0].ManaCost = mana.MustParse("G")
	// ValidCard$ Card.Wolf never matches this card (an Elf), so the trigger
	// this port checks (self-ETB only, trigger.go's own doc comment) never
	// fires for it.
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Wolf | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	c := engine.NewScriptedController()

	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(creature).Zone != engine.Battlefield {
		t.Errorf("creature zone = %v, want Battlefield", g.Card(creature).Zone)
	}
}

// diesTriggerCreatureDefPT builds a *compile.Card for a creature with a real
// "when CARDNAME dies" trigger -- the corpus's own Mode$ ChangesZone,
// Origin$ Battlefield, Destination$ Graveyard, ValidCard$ Card.Self shape
// (Rotting Regisaur and hundreds like it), compiled through the real
// pipeline for the same reason etbTriggerCreatureDef is.
func diesTriggerCreatureDefPT(t *testing.T, power, toughness string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test Dies Creature"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Dies Creature"
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = power, toughness
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Battlefield | Destination$ Graveyard | ValidCard$ Card.Self | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// TestDestroyLethalToughnessFiresDiesTrigger proves checkDiesTriggers
// (trigger.go) is wired into destroyLethalToughness (action.go): a creature
// that dies to CR 704.5f gets its own "when CARDNAME dies" trigger detected
// and queued, the identical "mechanism now, content later" contract
// checkETBTriggers already established -- Draw is still unresolved (M6's
// job), so this only checks the trigger reached the stack, not that it ran.
func TestDestroyLethalToughnessFiresDiesTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	dead := g.NewCard(diesTriggerCreatureDefPT(t, "2", "0"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(dead).Zone; z != engine.Graveyard {
		t.Fatalf("creature zone = %v, want Graveyard", z)
	}
	if got := g.StackLen(); got != 1 {
		t.Fatalf("StackLen() = %d, want 1 (the Dies trigger's own Execute$ sub-ability)", got)
	}
	top, ok := g.StackTop()
	if !ok {
		t.Fatal("StackTop() = false, want an ability on top")
	}
	if top.Source != dead {
		t.Errorf("pushed ability Source = %v, want %v (the dead creature itself)", top.Source, dead)
	}
	if top.Controller != p {
		t.Errorf("pushed ability Controller = %v, want %v", top.Controller, p)
	}
}

// A Dies trigger whose ValidCard does not match the dying card never pushes
// anything, the same Matches-driven skip TestCastSpellSkipsNonMatchingTrigger
// already proves for entering.
func TestDestroyLethalToughnessSkipsNonMatchingDiesTrigger(t *testing.T) {
	t.Parallel()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test Watcher"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Watcher"
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "2", "0"
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Battlefield | Destination$ Graveyard | ValidCard$ Card.Wolf | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	dead := g.NewCard(def, p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(dead).Zone; z != engine.Graveyard {
		t.Fatalf("creature zone = %v, want Graveyard", z)
	}
	if got := g.StackLen(); got != 0 {
		t.Errorf("StackLen() = %d, want 0 -- ValidCard$ Card.Wolf never matches this Elf", got)
	}
}

// impactTremorsDef builds a *compile.Card for Impact Tremors' own real
// "whenever a creature you control enters, deal 1 damage to each opponent"
// trigger -- compiled through the real pipeline, watching for another
// permanent to enter rather than itself (checkOtherETBTriggers, trigger.go).
func impactTremorsDef(t *testing.T) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Impact Tremors"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Impact Tremors"
	raw.Faces[0].Type = cardtype.Parse(reg, "Enchantment")
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Creature.YouCtrl | TriggerZones$ Battlefield | Execute$ TrigDmg",
	}
	raw.Faces[0].SVars.Set("TrigDmg", "DB$ DealDamage | Defined$ Player.Opponent | NumDmg$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile Impact Tremors: %v", err)
	}
	return c
}

// TestCastSpellFiresOtherPermanentsWatchingTrigger proves checkOtherETBTriggers
// (trigger.go): Impact Tremors, already on the battlefield, carries no
// trigger of its own tied to itself entering -- its trigger watches for some
// OTHER creature to enter under its controller. Casting one that carries no
// ETB trigger of its own still reaches Impact Tremors' own Execute$
// sub-ability (DealDamage): the only source that error can possibly name,
// since nothing else on the board could have produced it. Same
// "mechanism now, content later" shape as TestCastSpellFiresETBTrigger --
// checking the resolution reached the right unimplemented effect, not
// resolving it (M6's job), and the same reason that test does not inspect
// StackTop either: ResolveStack pops an ability before resolving it (its own
// doc comment), so nothing is left on the stack to inspect once the error
// naming it comes back.
func TestCastSpellFiresOtherPermanentsWatchingTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.Player(p).ManaPool.Add(mana.Green, 1)
	g.NewCard(impactTremorsDef(t), p, engine.Battlefield)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Hand)
	c := engine.NewScriptedController()

	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}

	err := g.ResolveStack(engine.NewRegistry(), c)
	if !errors.Is(err, engine.ErrUnimplemented) {
		t.Fatalf("ResolveStack error = %v, want ErrUnimplemented (DealDamage)", err)
	}
	if !strings.Contains(err.Error(), "DealDamage") {
		t.Errorf("ResolveStack error = %q, want it to name DealDamage", err.Error())
	}
	if g.Card(creature).Zone != engine.Battlefield {
		t.Errorf("creature zone = %v, want Battlefield -- the permanent spell itself should still have resolved", g.Card(creature).Zone)
	}
	if g.StackLen() != 0 {
		t.Errorf("StackLen() = %d, want 0 -- the failed trigger was popped before its own Resolve ran", g.StackLen())
	}
}

// dyingWatcherDef builds a *compile.Card for a real "whenever a creature you
// control dies" trigger (Blood Artist/Zulaport Cutthroat's own corpus shape,
// 205 real cards) -- ValidCard$ Creature.YouCtrl, watching for some OTHER
// permanent to die rather than itself (checkOtherDiesTriggers, trigger.go).
func dyingWatcherDef(t *testing.T) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test Dies Watcher"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Dies Watcher"
	raw.Faces[0].Type = cardtype.Parse(reg, "Enchantment")
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Battlefield | Destination$ Graveyard | ValidCard$ Creature.YouCtrl | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// TestDestroyLethalToughnessFiresOtherPermanentsWatchingDiesTrigger proves
// checkOtherDiesTriggers (trigger.go): a watcher already on the battlefield,
// carrying no dies trigger tied to itself, still detects some OTHER
// creature dying under its own controller. The dying creature itself
// carries no trigger of its own (plain creatureDefPT), so the pushed
// ability can only have come from the watcher.
func TestDestroyLethalToughnessFiresOtherPermanentsWatchingDiesTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	watcher := g.NewCard(dyingWatcherDef(t), p, engine.Battlefield)
	dead := g.NewCard(creatureDefPT(t, "2", "0"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if z := g.Card(dead).Zone; z != engine.Graveyard {
		t.Fatalf("creature zone = %v, want Graveyard", z)
	}
	if got := g.StackLen(); got != 1 {
		t.Fatalf("StackLen() = %d, want 1 (the watcher's own Execute$ sub-ability)", got)
	}
	top, ok := g.StackTop()
	if !ok {
		t.Fatal("StackTop() = false, want an ability on top")
	}
	if top.Source != watcher {
		t.Errorf("pushed ability Source = %v, want %v (the watcher, not the dying creature)", top.Source, watcher)
	}
	if top.Controller != p {
		t.Errorf("pushed ability Controller = %v, want %v", top.Controller, p)
	}
}
