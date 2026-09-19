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
// permanent to enter rather than itself (otherETBTriggerMatches, trigger.go).
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

// TestCastSpellFiresOtherPermanentsWatchingTrigger proves otherETBTriggerMatches
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

// apnapWatcherDef builds a *compile.Card watching for ANY land entering the
// battlefield (ValidCard$ Land, unrestricted -- unlike impactTremorsDef's
// own Creature.YouCtrl, so one entering land fires a copy of this on EVERY
// player's battlefield regardless of who controls it), with a Draw
// sub-ability so which copy actually resolved is directly observable --
// used to prove CR 603.3b's own APNAP ordering (pushTriggeredAbilities,
// trigger.go). PlayLand (land.go), not CastSpell, is the real call site:
// playing a land never touches the stack (CR 305.1, land.go's own doc
// comment), so checkETBTriggers runs synchronously and StackTop() names the
// pushed triggers directly, with no cast-and-resolve step in between to
// obscure the push order the way it would for a cast creature spell.
func apnapWatcherDef(t *testing.T, name string) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Enchantment")
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Land | TriggerZones$ Battlefield | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestPlayLandPushesETBTriggersInAPNAPOrder proves pushTriggeredAbilities
// (trigger.go): a land entering under active player p's control matches
// BOTH p's own watcher and other's, since ValidCard$ Land carries no YouCtrl
// restriction -- otherETBTriggerMatches finds both. CR 603.3b's own APNAP
// order puts the active player's own group on the stack first, then the
// non-active player's group on top of it, so the non-active player's
// (other's) trigger is the one that resolves FIRST -- StackTop() names it
// before anything is resolved, and ResolveStack's own Draw confirms other,
// not p, actually drew.
func TestPlayLandPushesETBTriggersInAPNAPOrder(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(apnapWatcherDef(t, "Active Watcher"), p, engine.Battlefield)
	otherWatcher := g.NewCard(apnapWatcherDef(t, "Other Watcher"), other, engine.Battlefield)
	plains := g.NewCard(landDef(t, "Plains", "Basic Land Plains"), p, engine.Hand)
	pTop := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	otherTop := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Library)

	if !g.PlayLand(p, plains) {
		t.Fatal("PlayLand failed playing a Plains from hand")
	}

	if got := g.StackLen(); got != 2 {
		t.Fatalf("StackLen() = %d, want 2 -- both watchers should have fired", got)
	}
	if top, ok := g.StackTop(); !ok || top.Source != otherWatcher {
		t.Fatalf("StackTop() = %+v, ok=%v, want other's watcher (%v) on top -- CR 603.3b resolves the non-active player's trigger first", top, ok, otherWatcher)
	}

	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(otherTop).Zone != engine.Hand {
		t.Errorf("other's library card zone = %v, want Hand -- other's watcher should have drawn first", g.Card(otherTop).Zone)
	}
	if g.Card(pTop).Zone != engine.Hand {
		t.Errorf("p's library card zone = %v, want Hand -- p's own watcher should also have drawn, just second", g.Card(pTop).Zone)
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

// attacksTriggerAloneDefPT builds a creature whose own Attacks trigger
// carries Alone$ True -- attacksOtherCount's own corpus shape (60 of 1,606
// real lines, every one this exact value; trigger.go's own doc comment).
func attacksTriggerAloneDefPT(t *testing.T, name, power, toughness string) *compile.Card {
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
		"Mode$ Attacks | ValidCard$ Card.Self | Alone$ True | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDeclareCombatAttackersFiresAloneTriggerWhenAttackingAlone proves
// attacksOtherCount (trigger.go) resolves Alone$ True: a lone declared
// attacker satisfies it.
func TestDeclareCombatAttackersFiresAloneTriggerWhenAttackingAlone(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	attacker := g.NewCard(attacksTriggerAloneDefPT(t, "Test Attacker", "2", "2"), p, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- a lone attacker satisfies Alone$ True", g.Card(top).Zone)
	}
}

// TestDeclareCombatAttackersSkipsAloneTriggerWithAnotherAttacker proves the
// same trigger does not fire when a second creature also attacks this
// combat, even though it shares no defender and carries no trigger of its
// own -- Alone$ True means no other declared attacker at all
// (CombatUtil.checkDeclaredAttacker's own AbilityKey.OtherAttackers is every
// other attacker in the whole combat, not just ones sharing this one's
// target).
func TestDeclareCombatAttackersSkipsAloneTriggerWithAnotherAttacker(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	attacker := g.NewCard(attacksTriggerAloneDefPT(t, "Test Attacker", "2", "2"), p, engine.Battlefield)
	second := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker, second})
	g.DeclareCombatAttackers(ac)

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- a second declared attacker fails Alone$ True", got)
	}
}

// attacksTriggerDefendingPlayerPoisonedDefPT builds a creature whose own
// Attacks trigger carries DefendingPlayerPoisoned$ True.
func attacksTriggerDefendingPlayerPoisonedDefPT(t *testing.T, name, power, toughness string) *compile.Card {
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
		"Mode$ Attacks | ValidCard$ Card.Self | DefendingPlayerPoisoned$ True | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDeclareCombatAttackersFiresDefendingPlayerPoisonedTrigger proves
// DefendingPlayerPoisoned$ resolves against defenderOf(attacker)'s own
// poison counters (attack.go's defenderOf, counters.go's Counters.Count).
func TestDeclareCombatAttackersFiresDefendingPlayerPoisonedTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.Player(other).Counters.Add(engine.Poison, 3)
	attacker := g.NewCard(attacksTriggerDefendingPlayerPoisonedDefPT(t, "Test Attacker", "2", "2"), p, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- the defending player has poison counters", g.Card(top).Zone)
	}
}

// TestDeclareCombatAttackersSkipsDefendingPlayerPoisonedTriggerWithNoPoison
// proves the same trigger does not fire against a defending player with no
// poison counters at all.
func TestDeclareCombatAttackersSkipsDefendingPlayerPoisonedTriggerWithNoPoison(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	attacker := g.NewCard(attacksTriggerDefendingPlayerPoisonedDefPT(t, "Test Attacker", "2", "2"), p, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- the defending player has no poison counters", got)
	}
}

// attacksTriggerDifferentPlayersDefPT builds a creature whose own Attacks
// trigger carries AttackDifferentPlayers$ True.
func attacksTriggerDifferentPlayersDefPT(t *testing.T, name, power, toughness string) *compile.Card {
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
		"Mode$ Attacks | ValidCard$ Card.Self | AttackDifferentPlayers$ True | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDeclareCombatAttackersFiresAttackDifferentPlayersTrigger proves
// attacksMultiplePlayers (trigger.go) resolves AttackDifferentPlayers$: in a
// three-player game, one attacker sent at each of the two opponents
// satisfies it for both.
func TestDeclareCombatAttackersFiresAttackDifferentPlayersTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b", "c")
	p, b, c2 := g.Players()[0], g.Players()[1], g.Players()[2]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(b).Life, g.Player(c2).Life = 20, 20, 20
	attacker := g.NewCard(attacksTriggerDifferentPlayersDefPT(t, "Test Attacker", "2", "2"), p, engine.Battlefield)
	second := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker, second})
	ac.QueueAttackTarget(engine.PlayerEntity(b))
	ac.QueueAttackTarget(engine.PlayerEntity(c2))
	g.DeclareCombatAttackers(ac)

	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- the two attackers hit different players", g.Card(top).Zone)
	}
}

// TestDeclareCombatAttackersSkipsAttackDifferentPlayersTriggerAgainstOnePlayer
// proves the same trigger does not fire when every attacker this combat
// hits the same single defending player.
func TestDeclareCombatAttackersSkipsAttackDifferentPlayersTriggerAgainstOnePlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b", "c")
	p, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(b).Life = 20, 20
	attacker := g.NewCard(attacksTriggerDifferentPlayersDefPT(t, "Test Attacker", "2", "2"), p, engine.Battlefield)
	second := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker, second})
	ac.QueueAttackTarget(engine.PlayerEntity(b))
	ac.QueueAttackTarget(engine.PlayerEntity(b))
	g.DeclareCombatAttackers(ac)

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- both attackers hit the same player b", got)
	}
}

// dyingWatcherDef builds a *compile.Card for a real "whenever a creature you
// control dies" trigger (Blood Artist/Zulaport Cutthroat's own corpus shape,
// 205 real cards) -- ValidCard$ Creature.YouCtrl, watching for some OTHER
// permanent to die rather than itself (otherDiesTriggerMatches, trigger.go).
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
// otherDiesTriggerMatches (trigger.go): a watcher already on the battlefield,
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

// TestCastSpellFiresSpellCastTriggerForQualifiedOpponentActivatingPlayer
// proves the dotted form (ValidActivatingPlayer$ Player.Opponent, 12 of the
// real corpus's 25 qualified lines) resolves through matchesPlayerSpec
// (valid.go) to the identical answer the bare "Opponent" form already gets --
// unlike TestCastSpellFiresSpellCastTriggerForOpponentActivatingPlayer, this
// exercises matchesPlayerBase through the Base.Property split rather than
// directly.
func TestCastSpellFiresSpellCastTriggerForQualifiedOpponentActivatingPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.Player(p).ManaPool.Add(mana.Green, 1)
	g.NewCard(spellCastWatcherDef(t, "Test Qualified Opponent Watcher", "Player.Opponent"), other, engine.Battlefield)
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
		t.Errorf("library card zone = %v, want Hand -- other's own ValidActivatingPlayer$ Player.Opponent should match p casting", g.Card(top).Zone)
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
// trigger carries ValidBlocked$ Creature.powerGE4, checked against
// blk.Attacker (checkBlocksTriggers' own doc comment).
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

// TestDeclareCombatBlockersFiresBlocksTriggerWhenValidBlockedMatches proves
// ValidBlocked$ is checked against blk.Attacker: a power-4 attacker
// satisfies Creature.powerGE4, so the trigger fires.
func TestDeclareCombatBlockersFiresBlocksTriggerWhenValidBlockedMatches(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "4", "4"), a, engine.Battlefield)
	blocker := g.NewCard(blocksTriggerWithValidBlockedParamDefPT(t, "Test Blocker", "2", "2"), b, engine.Battlefield)
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
		t.Errorf("library card zone = %v, want Hand -- ValidBlocked$ Creature.powerGE4 should match a power-4 attacker", g.Card(top).Zone)
	}
}

// TestDeclareCombatBlockersSkipsBlocksTriggerWhenValidBlockedDoesNotMatch
// proves the same trigger does NOT fire against a power-2 attacker, which
// Creature.powerGE4 rejects.
func TestDeclareCombatBlockersSkipsBlocksTriggerWhenValidBlockedDoesNotMatch(t *testing.T) {
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
		t.Fatalf("StackLen() = %d, want 0 -- ValidBlocked$ Creature.powerGE4 must reject a power-2 attacker", got)
	}
	if g.Card(top).Zone != engine.Library {
		t.Errorf("library card zone = %v, want Library -- nothing should have drawn it", g.Card(top).Zone)
	}
}

// attackerBlockedTriggerCreatureDefPT builds a creature with a real
// "whenever this becomes blocked" trigger -- Mode$ AttackerBlocked,
// ValidCard$ Card.Self, no ValidBlocker$ (74 of 127 real lines carry
// neither it nor ValidBlockerAmount$).
func attackerBlockedTriggerCreatureDefPT(t *testing.T, name, power, toughness string) *compile.Card {
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
		"Mode$ AttackerBlocked | ValidCard$ Card.Self | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDeclareCombatBlockersFiresAttackerBlockedTrigger proves
// checkAttackerBlockedTriggers (trigger.go) is wired into
// DeclareCombatBlockers (block.go): a blocked attacker's own "when this
// becomes blocked" trigger fires and its Execute$ sub-ability resolves.
func TestDeclareCombatBlockersFiresAttackerBlockedTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(attackerBlockedTriggerCreatureDefPT(t, "Test Attacker", "2", "2"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), a, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)

	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- the AttackerBlocked trigger's own Draw should have resolved", g.Card(top).Zone)
	}
}

// attackerBlockedWatcherDef builds a *compile.Card for a non-creature
// permanent watching for ANY creature its controller controls to become
// blocked (ValidCard$ Creature.YouCtrl) -- checkAttackerBlockedTriggers
// needs no separate "own" and "other" loop, the same as
// checkBlocksTriggers/checkAttacksTriggers.
func attackerBlockedWatcherDef(t *testing.T) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test Attacker Blocked Watcher"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Attacker Blocked Watcher"
	raw.Faces[0].Type = cardtype.Parse(reg, "Enchantment")
	raw.Faces[0].Triggers = []string{
		"Mode$ AttackerBlocked | ValidCard$ Creature.YouCtrl | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// TestDeclareCombatBlockersFiresOtherPermanentsWatchingAttackerBlockedTrigger
// proves checkAttackerBlockedTriggers fires a watcher's own trigger off a
// DIFFERENT creature (the watcher's own controller's attacker) becoming
// blocked: the attacker itself carries no trigger, so the pushed Draw can
// only have come from the watcher.
func TestDeclareCombatBlockersFiresOtherPermanentsWatchingAttackerBlockedTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	g.NewCard(attackerBlockedWatcherDef(t), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), a, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)

	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- the watcher's own Draw should have resolved", g.Card(top).Zone)
	}
}

// attackerBlockedAmountTriggerCreatureDefPT builds a creature whose own
// AttackerBlocked trigger carries ValidBlocker$/ValidBlockerAmount$ --
// checkAttackerBlockedTriggers' own count-the-blocker-group gate
// (validCardsCountMatches).
func attackerBlockedAmountTriggerCreatureDefPT(t *testing.T, name, power, toughness, amount string) *compile.Card {
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
		"Mode$ AttackerBlocked | ValidCard$ Card.Self | ValidBlocker$ Creature | ValidBlockerAmount$ " + amount + " | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDeclareCombatBlockersFiresAttackerBlockedTriggerWhenBlockerAmountMatches
// proves ValidBlockerAmount$ GE2 fires once a gang block gives the attacker
// two blockers.
func TestDeclareCombatBlockersFiresAttackerBlockedTriggerWhenBlockerAmountMatches(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(attackerBlockedAmountTriggerCreatureDefPT(t, "Test Attacker", "4", "4", "GE2"), a, engine.Battlefield)
	blocker1 := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)
	blocker2 := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), a, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker1, Attacker: attacker}, {Blocker: blocker2, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)

	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- two blockers meets ValidBlockerAmount$ GE2", g.Card(top).Zone)
	}
}

// TestDeclareCombatBlockersSkipsAttackerBlockedTriggerWhenBlockerAmountDoesNotMatch
// proves the other direction: a single blocker does not meet GE2.
func TestDeclareCombatBlockersSkipsAttackerBlockedTriggerWhenBlockerAmountDoesNotMatch(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(attackerBlockedAmountTriggerCreatureDefPT(t, "Test Attacker", "4", "4", "GE2"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), a, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- ValidBlockerAmount$ GE2 must reject a single blocker", got)
	}
	if g.Card(top).Zone != engine.Library {
		t.Errorf("library card zone = %v, want Library -- nothing should have drawn it", g.Card(top).Zone)
	}
}

// attackerBlockedByCreatureTriggerCreatureDefPT builds a creature whose own
// AttackerBlockedByCreature trigger carries ValidBlocker$ Creature.powerGE4,
// checked against blk.Blocker directly (checkAttackerBlockedByCreatureTriggers'
// own doc comment).
func attackerBlockedByCreatureTriggerCreatureDefPT(t *testing.T, name, power, toughness string) *compile.Card {
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
		"Mode$ AttackerBlockedByCreature | ValidCard$ Card.Self | ValidBlocker$ Creature.powerGE4 | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDeclareCombatBlockersFiresAttackerBlockedByCreatureTriggerWhenValidBlockerMatches
// proves ValidBlocker$ Creature.powerGE4 fires against a power-4 blocker.
func TestDeclareCombatBlockersFiresAttackerBlockedByCreatureTriggerWhenValidBlockerMatches(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(attackerBlockedByCreatureTriggerCreatureDefPT(t, "Test Attacker", "2", "2"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "4", "4"), b, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), a, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)

	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- a power-4 blocker meets ValidBlocker$ Creature.powerGE4", g.Card(top).Zone)
	}
}

// TestDeclareCombatBlockersSkipsAttackerBlockedByCreatureTriggerWhenValidBlockerDoesNotMatch
// proves the other direction: a power-2 blocker does not meet powerGE4.
func TestDeclareCombatBlockersSkipsAttackerBlockedByCreatureTriggerWhenValidBlockerDoesNotMatch(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(attackerBlockedByCreatureTriggerCreatureDefPT(t, "Test Attacker", "2", "2"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), a, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- ValidBlocker$ Creature.powerGE4 must reject a power-2 blocker", got)
	}
	if g.Card(top).Zone != engine.Library {
		t.Errorf("library card zone = %v, want Library -- nothing should have drawn it", g.Card(top).Zone)
	}
}

// TestDeclareCombatBlockersFiresAttackerBlockedByCreatureTriggerOncePerBlocker
// proves the per-pair granularity: a gang block by two matching blockers
// fires the trigger twice, drawing two cards, not once for the attacker as
// a whole (checkAttackerBlockedTriggers' own whole-group shape, above, is
// the one that fires once).
func TestDeclareCombatBlockersFiresAttackerBlockedByCreatureTriggerOncePerBlocker(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(attackerBlockedByCreatureTriggerCreatureDefPT(t, "Test Attacker", "6", "6"), a, engine.Battlefield)
	blocker1 := g.NewCard(creatureDefPT(t, "4", "4"), b, engine.Battlefield)
	blocker2 := g.NewCard(creatureDefPT(t, "4", "4"), b, engine.Battlefield)
	g.NewCard(creatureDefPT(t, "1", "1"), a, engine.Library)
	g.NewCard(creatureDefPT(t, "1", "1"), a, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker1, Attacker: attacker}, {Blocker: blocker2, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)

	if got := g.StackLen(); got != 2 {
		t.Fatalf("StackLen() = %d, want 2 -- one AttackerBlockedByCreature trigger per matching blocker", got)
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

// damageDoneTriggerCreatureDefPTValidTarget is
// damageDoneTriggerCreatureDefPT plus a ValidTarget$, checked against the
// damaged player through matchesPlayerSpec (valid.go) when the target is a
// *Player rather than a *Card (checkDamageDoneTriggersToPlayer, trigger.go).
func damageDoneTriggerCreatureDefPTValidTarget(t *testing.T, name, power, toughness, validTarget string) *compile.Card {
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
		"Mode$ DamageDone | ValidSource$ Card.Self | ValidTarget$ " + validTarget + " | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDealCombatDamageFiresDamageDoneTriggerToPlayerForQualifiedOther proves
// ValidTarget$ Player.Other -- a property matchesPlayerBase itself has no
// case for, only reachable through matchesPlayerSpec's own dotted dispatch
// (valid.go) -- matches the defending player b, who is not sourceController
// a.
func TestDealCombatDamageFiresDamageDoneTriggerToPlayerForQualifiedOther(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(damageDoneTriggerCreatureDefPTValidTarget(t, "Test Attacker", "3", "3", "Player.Other"), a, engine.Battlefield)
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
		t.Errorf("library card zone = %v, want Hand -- ValidTarget$ Player.Other should match defending player b", g.Card(top).Zone)
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
// DamageDone trigger carries DamageAmount$ damageAmount -- damageAmountMatches's
// own (trigger.go) TriggerDamageDone.performTest port, evaluated for real
// now rather than skipped.
func damageDoneTriggerWithDamageAmountParamDefPT(t *testing.T, name, power, toughness, damageAmount string) *compile.Card {
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
		"Mode$ DamageDone | ValidSource$ Card.Self | DamageAmount$ " + damageAmount + " | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDealCombatDamageFiresDamageDoneTriggerWithMatchingDamageAmount proves
// damageAmountMatches (trigger.go) resolves a plain-integer DamageAmount$
// for real: a 4/4 attacker's unblocked hit deals exactly 4, satisfying
// GE4.
func TestDealCombatDamageFiresDamageDoneTriggerWithMatchingDamageAmount(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(damageDoneTriggerWithDamageAmountParamDefPT(t, "Test Attacker", "4", "4", "GE4"), a, engine.Battlefield)
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
		t.Errorf("library card zone = %v, want Hand -- 4 damage satisfies DamageAmount$ GE4", g.Card(top).Zone)
	}
}

// TestDealCombatDamageSkipsDamageDoneTriggerWithNonMatchingDamageAmount
// proves the same comparison correctly fails to fire when the amount dealt
// does not satisfy it: a 3/3 attacker's hit deals only 3, which fails GE4.
func TestDealCombatDamageSkipsDamageDoneTriggerWithNonMatchingDamageAmount(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(damageDoneTriggerWithDamageAmountParamDefPT(t, "Test Attacker", "3", "3", "GE4"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- 3 damage does not satisfy DamageAmount$ GE4", got)
	}
}

// TestDealCombatDamageFiresDamageDoneTriggerWithMatchingTargetToughness
// proves the "TargetToughness" operand -- the damaged card's own folded
// Toughness(), not a literal integer -- resolves against a blocked
// attacker's own damage to its blocker.
func TestDealCombatDamageFiresDamageDoneTriggerWithMatchingTargetToughness(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(damageDoneTriggerWithDamageAmountParamDefPT(t, "Test Attacker", "3", "3", "EQTargetToughness"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "0", "3"), b, engine.Battlefield)
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
		t.Errorf("library card zone = %v, want Hand -- 3 damage to a 3-toughness blocker satisfies DamageAmount$ EQTargetToughness", g.Card(top).Zone)
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
	for i := 0; i < engine.MaxHandSize; i++ {
		g.NewCard(nil, a, engine.Hand)
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
	for i := 0; i < engine.MaxHandSize; i++ {
		g.NewCard(nil, a, engine.Hand)
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
	for i := 0; i < engine.MaxHandSize; i++ {
		g.NewCard(nil, a, engine.Hand)
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

// tapsForManaTriggerLandDefActivator is tapsForManaTriggerLandDef plus an
// Activator$, checked through matchesPlayerSpec (valid.go) the same way
// checkSpellCastTriggers' own ValidActivatingPlayer is.
func tapsForManaTriggerLandDefActivator(t *testing.T, name, typeLine, activator string) *compile.Card {
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
		"Mode$ TapsForMana | ValidCard$ Card.Self | Activator$ " + activator + " | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestTapLandForManaFiresTapsForManaTriggerForNonActiveActivator proves
// Activator$ Player.NonActive resolves through matchesPlayerSpec's own
// Active/NonActive property (Game.ActivePlayer(), valid.go): other, not the
// active player p, taps their own land, so NonActive matches.
func TestTapLandForManaFiresTapsForManaTriggerForNonActiveActivator(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.SetTurnState(1, p, engine.Main1)
	plains := g.NewCard(tapsForManaTriggerLandDefActivator(t, "Plains", "Basic Land Plains", "Player.NonActive"), other, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Library)

	if !g.TapLandForMana(other, plains, mana.White) {
		t.Fatal("TapLandForMana failed tapping a Plains for white")
	}
	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- Activator$ Player.NonActive should match other, not the active player p", g.Card(top).Zone)
	}
}

// TestTapLandForManaSkipsTapsForManaTriggerForNonActiveActivator proves the
// same trigger does NOT fire when the active player p taps their own land --
// p is Active, not NonActive.
func TestTapLandForManaSkipsTapsForManaTriggerForNonActiveActivator(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.SetTurnState(1, p, engine.Main1)
	plains := g.NewCard(tapsForManaTriggerLandDefActivator(t, "Plains", "Basic Land Plains", "Player.NonActive"), p, engine.Battlefield)

	if !g.TapLandForMana(p, plains, mana.White) {
		t.Fatal("TapLandForMana failed tapping a Plains for white")
	}

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- Activator$ Player.NonActive must reject the active player p", got)
	}
}

// phaseTriggerDefPT builds a *compile.Card for a real "at the beginning of
// a step or phase" trigger, Mode$ Phase -- Phase$ phase, ValidPlayer$
// validPlayer, TriggerZones$ Battlefield, its own Execute$ a Draw the same
// way every other trigger mode's own test proves firing.
func phaseTriggerDefPT(t *testing.T, name, phase, validPlayer string) *compile.Card {
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
		"Mode$ Phase | Phase$ " + phase + " | ValidPlayer$ " + validPlayer + " | TriggerZones$ Battlefield | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestAdvancePhaseFiresPhaseTriggerAtCorrectStep proves checkPhaseTriggers
// (trigger.go) is wired into beginPhase (turn.go): a real "at the beginning
// of your upkeep" trigger fires the moment AdvancePhase reaches Upkeep, with
// no other step's own automatic action (Upkeep's own beginPhase switch case
// is empty) to confuse the signal.
func TestAdvancePhaseFiresPhaseTriggerAtCorrectStep(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(phaseTriggerDefPT(t, "Test Upkeep Watcher", "Upkeep", "You"), p, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	g.SetTurnState(2, p, engine.Untap)

	g.AdvancePhase(engine.NewScriptedController())

	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- Phase$ Upkeep should fire the instant Upkeep begins", g.Card(top).Zone)
	}
}

// TestAdvancePhaseSkipsPhaseTriggerAtWrongStep proves the same trigger does
// NOT fire during a step it does not name: Phase$ Upkeep must not fire when
// Main1 begins.
func TestAdvancePhaseSkipsPhaseTriggerAtWrongStep(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(phaseTriggerDefPT(t, "Test Upkeep Watcher", "Upkeep", "You"), p, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	g.SetTurnState(2, p, engine.Draw)

	g.AdvancePhase(engine.NewScriptedController())

	if g.Card(top).Zone != engine.Library {
		t.Errorf("library card zone = %v, want Library -- Phase$ Upkeep must not fire when Main1 begins", g.Card(top).Zone)
	}
}

// TestAdvancePhaseSkipsPhaseTriggerForNonActivePlayer proves ValidPlayer$
// You resolves against the ACTIVE player, not unconditionally: p's own
// upkeep watcher must not fire during the OTHER player's upkeep.
func TestAdvancePhaseSkipsPhaseTriggerForNonActivePlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(phaseTriggerDefPT(t, "Test Upkeep Watcher", "Upkeep", "You"), p, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	g.SetTurnState(2, other, engine.Untap)

	g.AdvancePhase(engine.NewScriptedController())

	if g.Card(top).Zone != engine.Library {
		t.Errorf("library card zone = %v, want Library -- ValidPlayer$ You must not fire during other's own upkeep", g.Card(top).Zone)
	}
}

// TestAdvancePhaseFiresPhaseTriggerForOpponentValidPlayer proves
// ValidPlayer$ Opponent -- matchesPlayerSpec's own bare-value dispatch --
// fires during the OTHER player's phase rather than the host's own
// controller's.
func TestAdvancePhaseFiresPhaseTriggerForOpponentValidPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(phaseTriggerDefPT(t, "Test Upkeep Watcher", "Upkeep", "Opponent"), p, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	g.SetTurnState(2, other, engine.Untap)

	g.AdvancePhase(engine.NewScriptedController())

	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- ValidPlayer$ Opponent should fire during other's own upkeep", g.Card(top).Zone)
	}
}

// phaseTriggerMainSecondDefPT builds the real `Phase$ Main | PhaseCount$ 2`
// shape -- Survival's own cards, 29 real lines, every one paired this way --
// "at the beginning of your second main phase."
func phaseTriggerMainSecondDefPT(t *testing.T, name, validPlayer string) *compile.Card {
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
		"Mode$ Phase | Phase$ Main | PhaseCount$ 2 | ValidPlayer$ " + validPlayer + " | TriggerZones$ Battlefield | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestAdvancePhaseFiresMainSecondTriggerOnMain2 proves the `Main`/
// `PhaseCount$ 2` alias resolves to Main2 specifically, not Main1.
func TestAdvancePhaseFiresMainSecondTriggerOnMain2(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(phaseTriggerMainSecondDefPT(t, "Test Second Main Watcher", "You"), p, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	g.SetTurnState(2, p, engine.CombatEnd)

	g.AdvancePhase(engine.NewScriptedController())

	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- Phase$ Main/PhaseCount$ 2 should fire entering Main2", g.Card(top).Zone)
	}
}

// TestAdvancePhaseSkipsMainSecondTriggerOnMain1 proves the same trigger does
// NOT fire entering Main1, the turn's FIRST main phase.
func TestAdvancePhaseSkipsMainSecondTriggerOnMain1(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(phaseTriggerMainSecondDefPT(t, "Test Second Main Watcher", "You"), p, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	g.SetTurnState(2, p, engine.Draw)

	g.AdvancePhase(engine.NewScriptedController())

	if g.Card(top).Zone != engine.Library {
		t.Errorf("library card zone = %v, want Library -- Phase$ Main/PhaseCount$ 2 must not fire entering Main1", g.Card(top).Zone)
	}
}

// TestAdvancePhaseFiresPhaseTriggerFromGraveyard proves phaseTriggerZones
// (trigger.go) walks past the battlefield: a card sitting in the graveyard
// with TriggerZones$ Graveyard still fires its own Phase$ Upkeep trigger,
// unlike every other trigger mode this port checks, which only ever walks
// the battlefield.
func TestAdvancePhaseFiresPhaseTriggerFromGraveyard(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test Graveyard Upkeep Watcher"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Graveyard Upkeep Watcher"
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "1", "1"
	raw.Faces[0].Triggers = []string{
		"Mode$ Phase | Phase$ Upkeep | ValidPlayer$ You | TriggerZones$ Graveyard | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	g.NewCard(def, p, engine.Graveyard)
	top := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	g.SetTurnState(2, p, engine.Untap)

	g.AdvancePhase(engine.NewScriptedController())

	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- TriggerZones$ Graveyard should still fire from the graveyard", g.Card(top).Zone)
	}
}

// phaseTriggerWithIsPresentParamDefPT builds a creature whose own Phase
// trigger carries IsPresent$, a param checkPhaseTriggers does not evaluate.
func phaseTriggerWithIsPresentParamDefPT(t *testing.T, name string) *compile.Card {
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
		"Mode$ Phase | Phase$ Upkeep | ValidPlayer$ You | IsPresent$ Card.tapped | TriggerZones$ Battlefield | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestAdvancePhaseSkipsPhaseTriggerWithUnresolvedParam proves a trigger
// carrying a param this port cannot evaluate (IsPresent$) is skipped
// entirely -- never fired unconditionally, which would be silently wrong
// (GO-7).
func TestAdvancePhaseSkipsPhaseTriggerWithUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(phaseTriggerWithIsPresentParamDefPT(t, "Test Conditional Watcher"), p, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	g.SetTurnState(2, p, engine.Untap)

	g.AdvancePhase(engine.NewScriptedController())

	if g.Card(top).Zone != engine.Library {
		t.Errorf("library card zone = %v, want Library -- IsPresent$ is not evaluated, so the trigger must not fire", g.Card(top).Zone)
	}
}

// attackersDeclaredTriggerDef builds a permanent whose own AttackersDeclared
// trigger carries extra beyond the bare Mode$/Execute$ shape -- CR 508.1's
// own "whenever a player attacks" trigger, checkAttackersDeclaredTrigger's
// own doc comment (trigger.go).
func attackersDeclaredTriggerDef(t *testing.T, name, extra string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Enchantment")
	line := "Mode$ AttackersDeclared"
	if extra != "" {
		line += " | " + extra
	}
	line += " | Execute$ TrigDraw"
	raw.Faces[0].Triggers = []string{line}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDeclareCombatAttackersFiresAttackersDeclaredTrigger proves the bare
// shape -- no AttackingPlayer$/AttackedTarget$/ValidAttackers$ at all --
// fires once any attacker at all is declared, regardless of whose
// battlefield the trigger sits on.
func TestDeclareCombatAttackersFiresAttackersDeclaredTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.NewCard(attackersDeclaredTriggerDef(t, "Test Watcher", ""), other, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- the bare trigger must fire whenever any attacker is declared", g.Card(top).Zone)
	}
}

// TestDeclareCombatAttackersSkipsAttackersDeclaredTriggerWithNoAttackers
// proves the guard on an empty Combat.Attackers: an eligible creature the
// controller declines to attack with never reaches the trigger at all --
// PhaseHandler.java's own "if (!combat.getAttackers().isEmpty())".
func TestDeclareCombatAttackersSkipsAttackersDeclaredTriggerWithNoAttackers(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.NewCard(attackersDeclaredTriggerDef(t, "Test Watcher", ""), other, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{})
	g.DeclareCombatAttackers(ac)

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- no attacker was declared, so the trigger must not fire", got)
	}
}

// TestDeclareCombatAttackersFiresAttackingPlayerYouTrigger proves
// AttackingPlayer$ You: the trigger's own host is controlled by the
// attacking player.
func TestDeclareCombatAttackersFiresAttackingPlayerYouTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.NewCard(attackersDeclaredTriggerDef(t, "Test Watcher", "AttackingPlayer$ You"), p, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- AttackingPlayer$ You matches the host's own controller declaring attackers", g.Card(top).Zone)
	}
}

// TestDeclareCombatAttackersSkipsAttackingPlayerYouTriggerForDefender proves
// the same trigger does not fire when the host's own controller is the
// DEFENDING player instead.
func TestDeclareCombatAttackersSkipsAttackingPlayerYouTriggerForDefender(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.NewCard(attackersDeclaredTriggerDef(t, "Test Watcher", "AttackingPlayer$ You"), other, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- AttackingPlayer$ You must not match a host controlled by the defending player", got)
	}
}

// TestDeclareCombatAttackersFiresAttackedTargetYouTrigger proves
// AttackedTarget$ You: the host's own controller is the player actually
// being attacked this combat.
func TestDeclareCombatAttackersFiresAttackedTargetYouTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.NewCard(attackersDeclaredTriggerDef(t, "Test Watcher", "AttackedTarget$ You"), other, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- AttackedTarget$ You matches the host's own controller being attacked", g.Card(top).Zone)
	}
}

// TestDeclareCombatAttackersSkipsAttackedTargetYouTriggerForAttacker proves
// the same trigger does not fire when the host's own controller is the
// ATTACKING player instead of the one being attacked.
func TestDeclareCombatAttackersSkipsAttackedTargetYouTriggerForAttacker(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.NewCard(attackersDeclaredTriggerDef(t, "Test Watcher", "AttackedTarget$ You"), p, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- AttackedTarget$ You must not match a host controlled by the attacking player", got)
	}
}

// TestDeclareCombatAttackersFiresValidAttackersAmountTrigger proves
// ValidAttackers$/ValidAttackersAmount$: two attacking creatures the host's
// own controller controls satisfies ValidAttackersAmount$ GE2.
func TestDeclareCombatAttackersFiresValidAttackersAmountTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	first := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	second := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.NewCard(attackersDeclaredTriggerDef(t, "Test Watcher", "ValidAttackers$ Creature.YouCtrl | ValidAttackersAmount$ GE2"), p, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{first, second})
	g.DeclareCombatAttackers(ac)

	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(top).Zone != engine.Hand {
		t.Errorf("library card zone = %v, want Hand -- two attacking Creature.YouCtrl satisfies ValidAttackersAmount$ GE2", g.Card(top).Zone)
	}
}

// TestDeclareCombatAttackersSkipsValidAttackersAmountTriggerBelowThreshold
// proves the same trigger does not fire with only one qualifying attacker.
func TestDeclareCombatAttackersSkipsValidAttackersAmountTriggerBelowThreshold(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.NewCard(attackersDeclaredTriggerDef(t, "Test Watcher", "ValidAttackers$ Creature.YouCtrl | ValidAttackersAmount$ GE2"), p, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- one attacker fails ValidAttackersAmount$ GE2", got)
	}
}

// TestDeclareCombatAttackersSkipsAttackersDeclaredTriggerWithUnresolvedParam
// proves CheckSVar$ (13 real lines) is skipped entirely, GO-7's usual
// "whole line, not a guess" contract.
func TestDeclareCombatAttackersSkipsAttackersDeclaredTriggerWithUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.NewCard(attackersDeclaredTriggerDef(t, "Test Watcher", "CheckSVar$ X | SVarCompare$ GE1"), other, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- CheckSVar$ is not evaluated, so the trigger must not fire", got)
	}
}

// drawnTriggerDef builds a permanent whose own Drawn trigger carries extra
// beyond the bare Mode$/Execute$ shape -- CR 120.3's own "whenever you draw
// a card" mode, checkDrawnTriggers' own doc comment (trigger.go).
func drawnTriggerDef(t *testing.T, name, extra string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Enchantment")
	line := "Mode$ Drawn"
	if extra != "" {
		line += " | " + extra
	}
	line += " | Execute$ TrigDraw"
	raw.Faces[0].Triggers = []string{line}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDrawCardsFiresDrawnTriggerForCardYouCtrl proves ValidCard$ Card.YouCtrl:
// the drawn card's own controller (its owner, absent any control-changing
// effect) equals the host's own controller.
func TestDrawCardsFiresDrawnTriggerForCardYouCtrl(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(nil, p, engine.Library)
	g.NewCard(nil, p, engine.Library)
	g.NewCard(drawnTriggerDef(t, "Test Watcher", "ValidCard$ Card.YouCtrl"), p, engine.Battlefield)

	g.DrawCards(p, 1)

	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := len(g.Zone(engine.Hand, p).Cards()); got != 2 {
		t.Errorf("p's hand size = %d, want 2 -- the drawn card and the one the trigger's own Draw effect drew", got)
	}
}

// TestDrawCardsSkipsDrawnTriggerForCardYouCtrlWhenHostControlledByOther
// proves the same trigger does not fire when the host is controlled by
// someone other than the player who drew.
func TestDrawCardsSkipsDrawnTriggerForCardYouCtrlWhenHostControlledByOther(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(nil, p, engine.Library)
	g.NewCard(drawnTriggerDef(t, "Test Watcher", "ValidCard$ Card.YouCtrl"), other, engine.Battlefield)

	g.DrawCards(p, 1)

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- the drawn card is controlled by p, not other, so Card.YouCtrl fails against a host other controls", got)
	}
}

// TestDrawCardsFiresDrawnTriggerForValidPlayerOpponent proves ValidPlayer$
// Opponent: the host's own controller is not the player who drew.
func TestDrawCardsFiresDrawnTriggerForValidPlayerOpponent(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(nil, p, engine.Library)
	g.NewCard(nil, other, engine.Library)
	g.NewCard(drawnTriggerDef(t, "Test Watcher", "ValidPlayer$ Opponent"), other, engine.Battlefield)

	g.DrawCards(p, 1)

	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := len(g.Zone(engine.Hand, other).Cards()); got != 1 {
		t.Errorf("other's hand size = %d, want 1 -- ValidPlayer$ Opponent must fire when p (not other) draws", got)
	}
}

// TestDrawCardsSkipsDrawnTriggerForValidPlayerOpponentWhenHostIsDrawer
// proves the same trigger does not fire when the host's own controller IS
// the player who drew.
func TestDrawCardsSkipsDrawnTriggerForValidPlayerOpponentWhenHostIsDrawer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(nil, p, engine.Library)
	g.NewCard(drawnTriggerDef(t, "Test Watcher", "ValidPlayer$ Opponent"), p, engine.Battlefield)

	g.DrawCards(p, 1)

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- ValidPlayer$ Opponent must not fire when the host's own controller is the one who drew", got)
	}
}

// TestDrawCardsFiresDrawnTriggerForMatchingNumber proves Number$ 2: drawing
// two cards in one DrawCards call fires the trigger only on the second,
// CardsDrawnThisTurn incrementing per card the identical way Java's own
// numDrawnThisTurn does.
func TestDrawCardsFiresDrawnTriggerForMatchingNumber(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(nil, p, engine.Library)
	g.NewCard(nil, p, engine.Library)
	g.NewCard(drawnTriggerDef(t, "Test Watcher", "Number$ 2"), p, engine.Battlefield)

	g.DrawCards(p, 2)

	if got := g.StackLen(); got != 1 {
		t.Fatalf("StackLen() = %d, want 1 -- Number$ 2 must fire exactly once, on the second card drawn", got)
	}
}

// TestDrawCardsSkipsDrawnTriggerForNonMatchingNumber proves Number$ 2 does
// not fire on a single draw (CardsDrawnThisTurn reaches only 1).
func TestDrawCardsSkipsDrawnTriggerForNonMatchingNumber(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(nil, p, engine.Library)
	g.NewCard(drawnTriggerDef(t, "Test Watcher", "Number$ 2"), p, engine.Battlefield)

	g.DrawCards(p, 1)

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- a single draw never reaches CardsDrawnThisTurn == 2", got)
	}
}

// TestDrawCardsSkipsDrawnTriggerWithUnresolvedParam proves
// FirstCardInDrawStep$ is skipped entirely, GO-7's usual "whole line, not a
// guess" contract.
func TestDrawCardsSkipsDrawnTriggerWithUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(nil, p, engine.Library)
	g.NewCard(drawnTriggerDef(t, "Test Watcher", "FirstCardInDrawStep$ True"), p, engine.Battlefield)

	g.DrawCards(p, 1)

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- FirstCardInDrawStep$ is not evaluated, so the trigger must not fire", got)
	}
}
