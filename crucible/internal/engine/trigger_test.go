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

// attacksTriggerCreatureDefPT builds a *compile.Card for a creature with a
// real "when CARDNAME attacks, draw a card" trigger (Mode$ Attacks,
// ValidCard$ Card.Self), compiled through the real pipeline.
func attacksTriggerCreatureDefPT(t *testing.T, name, power, toughness string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = power, toughness
	raw.Faces[0].Triggers = []string{
		"Mode$ Attacks | ValidCard$ Card.Self | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDeclareCombatAttackersFiresAttacksTrigger proves checkAttacksTriggers
// (trigger.go) is wired into DeclareCombatAttackers (attack.go): a declared
// attacker's own "when this attacks" trigger is detected and its Execute$
// sub-ability (Draw) actually resolves, the same real end-to-end path
// TestCastSpellFiresETBTrigger proves for entering.
func TestDeclareCombatAttackersFiresAttacksTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	attacker := g.NewCard(attacksTriggerCreatureDefPT(t, "Test Attacker", "2", "2"), p, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- the Attacks trigger's own Draw should have resolved", g.Card(top).Zone)
	}
}

// attacksWatcherDef builds a *compile.Card for a non-creature permanent
// watching for ANY creature its controller controls to attack (ValidCard$
// Creature.YouCtrl), not tied to attacking itself -- checkAttacksTriggers
// needs no separate "own" and "other" loop the way checkETBTriggers does, so
// this proves the same check handles both shapes.
func attacksWatcherDef(t *testing.T) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test Attacks Watcher"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Attacks Watcher"
	raw.Faces[0].Type = cardtype.Parse(reg, "Enchantment")
	raw.Faces[0].Triggers = []string{
		"Mode$ Attacks | ValidCard$ Creature.YouCtrl | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// TestDeclareCombatAttackersFiresOtherPermanentsWatchingAttackTrigger proves
// checkAttacksTriggers fires a watcher's own trigger off a DIFFERENT
// creature attacking: the attacker itself carries no trigger, so the pushed
// Draw can only have come from the watcher.
func TestDeclareCombatAttackersFiresOtherPermanentsWatchingAttackTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(attacksWatcherDef(t), p, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- the watcher's own Draw should have resolved", g.Card(top).Zone)
	}
}

// attacksTriggerWithAttackedParamDefPT builds a creature whose own Attacks
// trigger carries Attacked$, a param checkAttacksTriggers does not evaluate.
func attacksTriggerWithAttackedParamDefPT(t *testing.T, name, power, toughness string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = power, toughness
	raw.Faces[0].Triggers = []string{
		"Mode$ Attacks | ValidCard$ Card.Self | Attacked$ You | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDeclareCombatAttackersSkipsTriggerWithUnresolvedParam proves a trigger
// carrying a param this port cannot evaluate (Attacked$) is skipped entirely
// -- never fired unconditionally, which would be silently wrong (GO-7).
func TestDeclareCombatAttackersSkipsTriggerWithUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	attacker := g.NewCard(attacksTriggerWithAttackedParamDefPT(t, "Test Attacker", "2", "2"), p, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- Attacked$ is not evaluated, so the trigger must not fire", got)
	}
	if g.Card(top).Zone != engine.Library {
		t.Errorf("library card zone = %v, want Library -- nothing should have drawn it", g.Card(top).Zone)
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

// spellCastWatcherDef builds a *compile.Card for a non-creature permanent
// carrying a real "whenever a player casts a spell" trigger restricted by
// ValidActivatingPlayer to activatingPlayer ("You" or "Opponent") --
// checkSpellCastTriggers' own dominant corpus param (1,216 of 1,435 real
// lines), matched by matchesActivatingPlayer rather than Matches (valid.go),
// since a Player is not a Card.
func spellCastWatcherDef(t *testing.T, name, activatingPlayer string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Enchantment")
	raw.Faces[0].Triggers = []string{
		"Mode$ SpellCast | ValidActivatingPlayer$ " + activatingPlayer + " | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestCastSpellFiresSpellCastTriggerForControllerActivatingPlayer proves
// checkSpellCastTriggers (trigger.go) is wired into CastSpell: a battlefield
// permanent watching "whenever YOU cast a spell" (ValidActivatingPlayer$
// You) fires the instant its own controller casts one, no ValidCard
// restriction needed (absent is a pass, matchesValidParam's own contract).
func TestCastSpellFiresSpellCastTriggerForControllerActivatingPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.Player(p).ManaPool.Add(mana.Green, 1)
	g.NewCard(spellCastWatcherDef(t, "Test You Watcher", "You"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefManaCost(t, "G"), p, engine.Hand)
	top := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	c := engine.NewScriptedController()

	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- the watcher's own SpellCast trigger should have resolved", g.Card(top).Zone)
	}
}

// TestCastSpellSkipsSpellCastTriggerForNonControllerActivatingPlayer proves
// the "You" restriction actually excludes another player: a watcher
// controlled by other, restricted to ValidActivatingPlayer$ You, does not
// fire when p (not other) casts the spell.
func TestCastSpellSkipsSpellCastTriggerForNonControllerActivatingPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.Player(p).ManaPool.Add(mana.Green, 1)
	g.NewCard(spellCastWatcherDef(t, "Test You Watcher", "You"), other, engine.Battlefield)
	creature := g.NewCard(creatureDefManaCost(t, "G"), p, engine.Hand)
	top := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Library)
	c := engine.NewScriptedController()

	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Library {
		t.Errorf("library card zone = %v, want Library -- other's own ValidActivatingPlayer$ You must not match p casting", g.Card(top).Zone)
	}
}

// TestCastSpellFiresSpellCastTriggerForOpponentActivatingPlayer proves the
// "Opponent" value: other's watcher fires when p, not other, casts --
// valid.go's own OppCtrl/OppOwn no-team simplification carried over to a
// Player rather than a Card.
func TestCastSpellFiresSpellCastTriggerForOpponentActivatingPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.Player(p).ManaPool.Add(mana.Green, 1)
	g.NewCard(spellCastWatcherDef(t, "Test Opponent Watcher", "Opponent"), other, engine.Battlefield)
	creature := g.NewCard(creatureDefManaCost(t, "G"), p, engine.Hand)
	top := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Library)
	c := engine.NewScriptedController()

	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- other's own ValidActivatingPlayer$ Opponent should match p casting", g.Card(top).Zone)
	}
}

// spellCastWatcherWithTargetsValidDef builds a *compile.Card carrying
// TargetsValid$, a param checkSpellCastTriggers does not evaluate (no
// per-trigger target-inspection hook this port has), alongside an otherwise
// matching ValidActivatingPlayer$ You.
func spellCastWatcherWithTargetsValidDef(t *testing.T) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test TargetsValid Watcher"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test TargetsValid Watcher"
	raw.Faces[0].Type = cardtype.Parse(reg, "Enchantment")
	raw.Faces[0].Triggers = []string{
		"Mode$ SpellCast | ValidActivatingPlayer$ You | TargetsValid$ Creature.YouCtrl | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// TestCastSpellSkipsSpellCastTriggerWithUnresolvedParam proves a trigger
// carrying a param this port cannot evaluate (TargetsValid$) is skipped
// entirely -- never fired unconditionally, which would be silently wrong
// (GO-7) -- even though ValidActivatingPlayer$ You would otherwise match.
func TestCastSpellSkipsSpellCastTriggerWithUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.Player(p).ManaPool.Add(mana.Green, 1)
	g.NewCard(spellCastWatcherWithTargetsValidDef(t), p, engine.Battlefield)
	creature := g.NewCard(creatureDefManaCost(t, "G"), p, engine.Hand)
	top := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	c := engine.NewScriptedController()

	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Library {
		t.Errorf("library card zone = %v, want Library -- TargetsValid$ is not evaluated, so this must not fire", g.Card(top).Zone)
	}
}

// blocksTriggerCreatureDefPT builds a *compile.Card for a creature with a
// real "when CARDNAME blocks" trigger -- Mode$ Blocks, ValidCard$ Card.Self.
func blocksTriggerCreatureDefPT(t *testing.T, name, power, toughness string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = power, toughness
	raw.Faces[0].Triggers = []string{
		"Mode$ Blocks | ValidCard$ Card.Self | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDeclareCombatBlockersFiresBlocksTrigger proves checkBlocksTriggers
// (trigger.go) is wired into DeclareCombatBlockers (block.go): a declared
// blocker's own "when this blocks" trigger is detected and its Execute$
// sub-ability (Draw) actually resolves.
func TestDeclareCombatBlockersFiresBlocksTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	blocker := g.NewCard(blocksTriggerCreatureDefPT(t, "Test Blocker", "2", "2"), b, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)

	if err := g.ResolveStack(engine.NewRegistry(), bc); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- the Blocks trigger's own Draw should have resolved", g.Card(top).Zone)
	}
}

// blocksWatcherDef builds a *compile.Card for a non-creature permanent
// watching for ANY creature its controller controls to block (ValidCard$
// Creature.YouCtrl), not tied to blocking itself -- checkBlocksTriggers
// needs no separate "own" and "other" loop, the same as checkAttacksTriggers.
func blocksWatcherDef(t *testing.T) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test Blocks Watcher"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Blocks Watcher"
	raw.Faces[0].Type = cardtype.Parse(reg, "Enchantment")
	raw.Faces[0].Triggers = []string{
		"Mode$ Blocks | ValidCard$ Creature.YouCtrl | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// TestDeclareCombatBlockersFiresOtherPermanentsWatchingBlockTrigger proves
// checkBlocksTriggers fires a watcher's own trigger off a DIFFERENT
// creature blocking: the blocker itself carries no trigger, so the pushed
// Draw can only have come from the watcher.
func TestDeclareCombatBlockersFiresOtherPermanentsWatchingBlockTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	g.NewCard(blocksWatcherDef(t), b, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)

	if err := g.ResolveStack(engine.NewRegistry(), bc); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- the watcher's own Draw should have resolved", g.Card(top).Zone)
	}
}

// blocksTriggerWithValidBlockedParamDefPT builds a creature whose own Blocks
// trigger carries ValidBlocked$, a param checkBlocksTriggers does not
// evaluate.
func blocksTriggerWithValidBlockedParamDefPT(t *testing.T, name, power, toughness string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = power, toughness
	raw.Faces[0].Triggers = []string{
		"Mode$ Blocks | ValidCard$ Card.Self | ValidBlocked$ Creature.powerGE4 | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDeclareCombatBlockersSkipsBlocksTriggerWithUnresolvedParam proves a
// trigger carrying a param this port cannot evaluate (ValidBlocked$) is
// skipped entirely -- never fired unconditionally, which would be silently
// wrong (GO-7).
func TestDeclareCombatBlockersSkipsBlocksTriggerWithUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	blocker := g.NewCard(blocksTriggerWithValidBlockedParamDefPT(t, "Test Blocker", "2", "2"), b, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- ValidBlocked$ is not evaluated, so the trigger must not fire", got)
	}
	if g.Card(top).Zone != engine.Library {
		t.Errorf("library card zone = %v, want Library -- nothing should have drawn it", g.Card(top).Zone)
	}
}

// damageDoneTriggerCreatureDefPT builds a *compile.Card for a creature with
// a real "whenever this deals damage" trigger -- Mode$ DamageDone,
// ValidSource$ Card.Self, no ValidTarget (absent is a pass, matchesValidParam's
// own contract).
func damageDoneTriggerCreatureDefPT(t *testing.T, name, power, toughness string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = power, toughness
	raw.Faces[0].Triggers = []string{
		"Mode$ DamageDone | ValidSource$ Card.Self | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDealCombatDamageFiresDamageDoneTriggerToPlayer proves
// checkDamageDoneTriggersToPlayer (trigger.go) is wired into dealPlayerDamage
// (combatdamage.go): an unblocked attacker's own "when this deals damage"
// trigger fires the instant it hits the defending player, and its Execute$
// sub-ability (Draw) actually resolves.
func TestDealCombatDamageFiresDamageDoneTriggerToPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(damageDoneTriggerCreatureDefPT(t, "Test Attacker", "3", "3"), a, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), a, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- the DamageDone trigger's own Draw should have resolved", g.Card(top).Zone)
	}
}

// TestDealCombatDamageFiresDamageDoneTriggerToCard proves the same trigger
// fires against a *Card target (a blocker) too, ValidTarget matched by
// Matches (valid.go) rather than matchesPlayerBase.
func TestDealCombatDamageFiresDamageDoneTriggerToCard(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(damageDoneTriggerCreatureDefPT(t, "Test Attacker", "3", "3"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "3", "3"), b, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), a, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- the DamageDone trigger's own Draw should have resolved", g.Card(top).Zone)
	}
}

// damageDoneWatcherDef builds a *compile.Card for a non-creature permanent
// watching for ANY creature its controller controls to deal damage to a
// player (ValidSource$ Creature.YouCtrl, ValidTarget$ Player) -- neither
// checkDamageDoneTriggersToCard nor ToPlayer needs a separate "own"/"other"
// loop, the same single-walk shape checkAttacksTriggers/checkBlocksTriggers
// already established.
func damageDoneWatcherDef(t *testing.T) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test DamageDone Watcher"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test DamageDone Watcher"
	raw.Faces[0].Type = cardtype.Parse(reg, "Enchantment")
	raw.Faces[0].Triggers = []string{
		"Mode$ DamageDone | ValidSource$ Creature.YouCtrl | ValidTarget$ Player | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// TestDealCombatDamageFiresOtherPermanentsWatchingDamageDoneTrigger proves
// checkDamageDoneTriggersToPlayer fires a watcher's own trigger off a
// DIFFERENT creature's damage: the attacker itself carries no trigger, so
// the pushed Draw can only have come from the watcher.
func TestDealCombatDamageFiresOtherPermanentsWatchingDamageDoneTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	g.SetTurnState(1, a, engine.Main1)
	g.NewCard(damageDoneWatcherDef(t), a, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), a, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- the watcher's own Draw should have resolved", g.Card(top).Zone)
	}
}

// damageDoneTriggerWithDamageAmountParamDefPT builds a creature whose own
// DamageDone trigger carries DamageAmount$, a param checkDamageDoneTriggersToPlayer/
// ToCard does not evaluate.
func damageDoneTriggerWithDamageAmountParamDefPT(t *testing.T, name, power, toughness string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = power, toughness
	raw.Faces[0].Triggers = []string{
		"Mode$ DamageDone | ValidSource$ Card.Self | DamageAmount$ GE4 | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDealCombatDamageSkipsDamageDoneTriggerWithUnresolvedParam proves a
// trigger carrying a param this port cannot evaluate (DamageAmount$) is
// skipped entirely -- never fired unconditionally, which would be silently
// wrong (GO-7).
func TestDealCombatDamageSkipsDamageDoneTriggerWithUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(damageDoneTriggerWithDamageAmountParamDefPT(t, "Test Attacker", "3", "3"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- DamageAmount$ is not evaluated, so the trigger must not fire", got)
	}
}

// discardedTriggerCreatureDefPT builds a *compile.Card for a creature with a
// real "when CARDNAME is discarded" trigger -- Mode$ Discarded, ValidCard$
// Card.Self.
func discardedTriggerCreatureDefPT(t *testing.T, name string) *compile.Card {
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
	raw.Faces[0].Triggers = []string{
		"Mode$ Discarded | ValidCard$ Card.Self | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestCleanupFiresDiscardedTrigger proves checkDiscardedTriggers (trigger.go)
// is wired into cleanupStep (turn.go): a discarded card's own "when this is
// discarded" trigger is detected and its Execute$ sub-ability (Draw)
// actually resolves.
func TestCleanupFiresDiscardedTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	discarded := g.NewCard(discardedTriggerCreatureDefPT(t, "Test Discarded Creature"), a, engine.Hand)
	var hand []engine.CardID
	for i := 0; i < engine.MaxHandSize; i++ {
		hand = append(hand, g.NewCard(nil, a, engine.Hand))
	}
	top := g.NewCard(creatureDefPT(t, "1", "1"), a, engine.Library)

	c := engine.NewScriptedController()
	c.QueueDiscard([]engine.CardID{discarded})
	g.SetTurnState(1, a, engine.EndOfTurn)
	g.AdvancePhase(c) // -> Cleanup

	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- the Discarded trigger's own Draw should have resolved", g.Card(top).Zone)
	}
}

// discardedWatcherDef builds a *compile.Card for a non-creature permanent
// watching for ANY card its controller discards (ValidCard$ Card.YouCtrl),
// not tied to being discarded itself -- checkDiscardedTriggers needs no
// separate "own" and "other" loop, the same as checkAttacksTriggers/
// checkBlocksTriggers.
func discardedWatcherDef(t *testing.T) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test Discarded Watcher"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Discarded Watcher"
	raw.Faces[0].Type = cardtype.Parse(reg, "Enchantment")
	raw.Faces[0].Triggers = []string{
		"Mode$ Discarded | ValidCard$ Card.YouCtrl | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// TestCleanupFiresOtherPermanentsWatchingDiscardedTrigger proves
// checkDiscardedTriggers fires a watcher's own trigger off a DIFFERENT card
// being discarded: the discarded card itself carries no trigger, so the
// pushed Draw can only have come from the watcher.
func TestCleanupFiresOtherPermanentsWatchingDiscardedTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	g.NewCard(discardedWatcherDef(t), a, engine.Battlefield)
	discarded := g.NewCard(nil, a, engine.Hand)
	var hand []engine.CardID
	for i := 0; i < engine.MaxHandSize; i++ {
		hand = append(hand, g.NewCard(nil, a, engine.Hand))
	}
	top := g.NewCard(creatureDefPT(t, "1", "1"), a, engine.Library)

	c := engine.NewScriptedController()
	c.QueueDiscard([]engine.CardID{discarded})
	g.SetTurnState(1, a, engine.EndOfTurn)
	g.AdvancePhase(c) // -> Cleanup

	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- the watcher's own Draw should have resolved", g.Card(top).Zone)
	}
}

// discardedTriggerWithValidCauseParamDefPT builds a creature whose own
// Discarded trigger carries ValidCause$, a param checkDiscardedTriggers does
// not evaluate.
func discardedTriggerWithValidCauseParamDefPT(t *testing.T, name string) *compile.Card {
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
	raw.Faces[0].Triggers = []string{
		"Mode$ Discarded | ValidCard$ Card.Self | ValidCause$ SpellAbility.Madness | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestCleanupSkipsDiscardedTriggerWithUnresolvedParam proves a trigger
// carrying a param this port cannot evaluate (ValidCause$) is skipped
// entirely -- never fired unconditionally, which would be silently wrong
// (GO-7).
func TestCleanupSkipsDiscardedTriggerWithUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	discarded := g.NewCard(discardedTriggerWithValidCauseParamDefPT(t, "Test Discarded Creature"), a, engine.Hand)
	var hand []engine.CardID
	for i := 0; i < engine.MaxHandSize; i++ {
		hand = append(hand, g.NewCard(nil, a, engine.Hand))
	}

	c := engine.NewScriptedController()
	c.QueueDiscard([]engine.CardID{discarded})
	g.SetTurnState(1, a, engine.EndOfTurn)
	g.AdvancePhase(c) // -> Cleanup

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- ValidCause$ is not evaluated, so the trigger must not fire", got)
	}
}

// tapsTriggerCreatureDefPT builds a *compile.Card for a creature with a real
// "when CARDNAME becomes tapped" trigger -- Mode$ Taps, ValidCard$ Card.Self.
func tapsTriggerCreatureDefPT(t *testing.T, name, power, toughness string, keywords ...string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = power, toughness
	raw.Faces[0].Keywords = keywords
	raw.Faces[0].Triggers = []string{
		"Mode$ Taps | ValidCard$ Card.Self | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDeclareCombatAttackersFiresTapsTrigger proves checkTapsTriggers
// (trigger.go) is wired into DeclareCombatAttackers (attack.go): a declared
// attacker's own "when this becomes tapped" trigger fires the instant
// tapping it for attacking actually happens.
func TestDeclareCombatAttackersFiresTapsTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	attacker := g.NewCard(tapsTriggerCreatureDefPT(t, "Test Attacker", "2", "2"), p, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- the Taps trigger's own Draw should have resolved", g.Card(top).Zone)
	}
}

// TestDeclareCombatAttackersSkipsTapsTriggerForVigilantAttacker proves the
// Taps trigger does NOT fire for a Vigilance attacker: CR 508.1f itself
// never taps it, so there is no tap event for checkTapsTriggers to detect.
func TestDeclareCombatAttackersSkipsTapsTriggerForVigilantAttacker(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	attacker := g.NewCard(tapsTriggerCreatureDefPT(t, "Test Vigilant Attacker", "2", "2", "Vigilance"), p, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- Vigilance means no tap, so no Taps trigger should have fired", got)
	}
}

// tapsWatcherDef builds a *compile.Card for a non-creature permanent
// watching for ANY land its controller controls to become tapped
// (ValidCard$ Land.YouCtrl), not tied to becoming tapped itself --
// checkTapsTriggers needs no separate "own" and "other" loop, the same as
// checkAttacksTriggers/checkBlocksTriggers/checkDamageDoneTriggersToCard.
func tapsWatcherDef(t *testing.T) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test Taps Watcher"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Taps Watcher"
	raw.Faces[0].Type = cardtype.Parse(reg, "Enchantment")
	raw.Faces[0].Triggers = []string{
		"Mode$ Taps | ValidCard$ Land.YouCtrl | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// TestTapLandForManaFiresOtherPermanentsWatchingTapsTrigger proves
// checkTapsTriggers fires a watcher's own trigger off a land tapping for
// mana too, not just combat's own tap site: the land itself carries no
// trigger, so the pushed Draw can only have come from the watcher.
func TestTapLandForManaFiresOtherPermanentsWatchingTapsTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(tapsWatcherDef(t), p, engine.Battlefield)
	plains := g.NewCard(landDef(t, "Plains", "Basic Land Plains"), p, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)

	if !g.TapLandForMana(p, plains, mana.White) {
		t.Fatal("TapLandForMana failed tapping a Plains for white")
	}
	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- the watcher's own Draw should have resolved", g.Card(top).Zone)
	}
}

// tapsTriggerWithFirstTimeParamDefPT builds a creature whose own Taps
// trigger carries FirstTime$, a param checkTapsTriggers does not evaluate.
func tapsTriggerWithFirstTimeParamDefPT(t *testing.T, name, power, toughness string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = power, toughness
	raw.Faces[0].Triggers = []string{
		"Mode$ Taps | ValidCard$ Card.Self | FirstTime$ True | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDeclareCombatAttackersSkipsTapsTriggerWithUnresolvedParam proves a
// trigger carrying a param this port cannot evaluate (FirstTime$) is skipped
// entirely -- never fired unconditionally, which would be silently wrong
// (GO-7).
func TestDeclareCombatAttackersSkipsTapsTriggerWithUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	attacker := g.NewCard(tapsTriggerWithFirstTimeParamDefPT(t, "Test Attacker", "2", "2"), p, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- FirstTime$ is not evaluated, so the trigger must not fire", got)
	}
}

// tapsForManaTriggerLandDef builds a *compile.Card for a land with a real
// "whenever this taps for mana" trigger -- Mode$ TapsForMana, ValidCard$
// Card.Self.
func tapsForManaTriggerLandDef(t *testing.T, name, typeLine string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, typeLine)
	raw.Faces[0].Triggers = []string{
		"Mode$ TapsForMana | ValidCard$ Card.Self | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestTapLandForManaFiresTapsForManaTrigger proves checkTapsForManaTriggers
// (trigger.go) is wired into TapLandForMana (manaability.go): a land's own
// "whenever this taps for mana" trigger fires, distinct from the general
// Taps trigger.
func TestTapLandForManaFiresTapsForManaTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	plains := g.NewCard(tapsForManaTriggerLandDef(t, "Plains", "Basic Land Plains"), p, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)

	if !g.TapLandForMana(p, plains, mana.White) {
		t.Fatal("TapLandForMana failed tapping a Plains for white")
	}
	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- the TapsForMana trigger's own Draw should have resolved", g.Card(top).Zone)
	}
}

// TestDeclareCombatAttackersDoesNotFireTapsForManaTrigger proves
// checkTapsForManaTriggers is narrower than checkTapsTriggers: attacking is
// not a mana ability, so a land's own TapsForMana trigger must not fire off
// an attack tap (there is none here -- the land does not even attack -- but
// the point is checkTapsForManaTriggers is never called from attack.go at
// all, only checkTapsTriggers is).
func TestDeclareCombatAttackersDoesNotFireTapsForManaTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test TapsForMana Creature"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test TapsForMana Creature"
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "2", "2"
	raw.Faces[0].Triggers = []string{
		"Mode$ TapsForMana | ValidCard$ Card.Self | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	attacker := g.NewCard(def, p, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- attacking is not a mana ability, so TapsForMana must not fire", got)
	}
}
