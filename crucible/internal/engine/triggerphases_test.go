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

// spellCastWatcherDefExtra is spellCastWatcherDef's own sibling, carrying
// whatever extra general trigger-restriction params (PlayerTurn$/
// NotPlayerTurn$/OpponentTurn$/Phase$) a test needs on top of a bare
// Mode$ SpellCast line -- sentinel_tower.txt's own real "Whenever an
// instant or sorcery spell is cast during your turn, CARDNAME deals 1
// damage to each opponent" is PlayerTurn$ True on exactly this mode, one of
// 43 real lines across six already-built modes that fired unconditionally
// until triggerPhasesCheck (trigger.go) landed.
func spellCastWatcherDefExtra(t *testing.T, name, extraParams string) *compile.Card {
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
		"Mode$ SpellCast | " + extraParams + " | Execute$ TrigGainLife",
	}
	raw.Faces[0].SVars.Set("TrigGainLife", "DB$ GainLife | Defined$ You | LifeAmount$ 5")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestSpellCastFiresTriggerWhenPlayerTurnMatches proves PlayerTurn$ True
// (triggerPhasesCheck, trigger.go, Trigger.phasesCheck's own port): a
// watcher's own controller casting during their own turn passes.
func TestSpellCastFiresTriggerWhenPlayerTurnMatches(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.Player(p).ManaPool.Add(mana.Green, 1)
	g.NewCard(spellCastWatcherDefExtra(t, "Test Watcher", "PlayerTurn$ True"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefManaCost(t, "G"), p, engine.Hand)
	c := engine.NewScriptedController()

	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(p).Life; got != 25 {
		t.Errorf("p life = %d, want 25 -- PlayerTurn$ True must pass on the watcher's own controller's turn", got)
	}
}

// TestSpellCastSkipsTriggerWhenPlayerTurnDoesNotMatch is the regression
// proof: a watcher controlled by other (not the active player) carrying
// PlayerTurn$ True must NOT fire when p, the active player, casts a spell
// -- before triggerPhasesCheck existed, this port had no code path checking
// PlayerTurn$/NotPlayerTurn$/OpponentTurn$/Phase$ at all, so this exact
// shape fired unconditionally, a wrong answer rather than a coverage gap.
func TestSpellCastSkipsTriggerWhenPlayerTurnDoesNotMatch(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.Player(p).ManaPool.Add(mana.Green, 1)
	g.NewCard(spellCastWatcherDefExtra(t, "Test Watcher", "PlayerTurn$ True"), other, engine.Battlefield)
	creature := g.NewCard(creatureDefManaCost(t, "G"), p, engine.Hand)
	c := engine.NewScriptedController()

	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(other).Life; got != 20 {
		t.Errorf("other life = %d, want unchanged 20 -- PlayerTurn$ True must not fire on p's turn for a watcher other controls", got)
	}
}

// TestSpellCastFiresTriggerWhenOpponentTurnMatches proves OpponentTurn$ True
// (23 real lines, SpellCast/Drawn) -- collapses to NotPlayerTurn$'s own
// check in this port's no-team model (matchesPlayerBase's own doc comment):
// a watcher controlled by other fires during p's turn, since p is other's
// only opponent.
func TestSpellCastFiresTriggerWhenOpponentTurnMatches(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.Player(p).ManaPool.Add(mana.Green, 1)
	g.NewCard(spellCastWatcherDefExtra(t, "Test Watcher", "OpponentTurn$ True"), other, engine.Battlefield)
	creature := g.NewCard(creatureDefManaCost(t, "G"), p, engine.Hand)
	c := engine.NewScriptedController()

	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(other).Life; got != 25 {
		t.Errorf("other life = %d, want 25 -- OpponentTurn$ True must fire during p's turn for other's own watcher", got)
	}
}

// TestSpellCastSkipsTriggerWhenOpponentTurnDoesNotMatch is
// TestSpellCastFiresTriggerWhenOpponentTurnMatches' own mirror: a watcher
// controlled by p (the active player) carrying OpponentTurn$ True must not
// fire on p's own turn.
func TestSpellCastSkipsTriggerWhenOpponentTurnDoesNotMatch(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.Player(p).ManaPool.Add(mana.Green, 1)
	g.NewCard(spellCastWatcherDefExtra(t, "Test Watcher", "OpponentTurn$ True"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefManaCost(t, "G"), p, engine.Hand)
	c := engine.NewScriptedController()

	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(p).Life; got != 20 {
		t.Errorf("p life = %d, want unchanged 20 -- OpponentTurn$ True must not fire during p's own turn for p's own watcher", got)
	}
}

// TestSpellCastFiresTriggerWhenPhaseMatches proves the general Phase$ gate
// (dovins_acuity.txt's own real Mode$ SpellCast | ... | Phase$ Main1,Main2
// shape) reusing phaseTriggerMatches -- Mode$ Phase's own dispatch function,
// asked generically here for a different mode entirely.
func TestSpellCastFiresTriggerWhenPhaseMatches(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.Player(p).ManaPool.Add(mana.Green, 1)
	g.NewCard(spellCastWatcherDefExtra(t, "Test Watcher", "Phase$ Main1,Main2"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefManaCost(t, "G"), p, engine.Hand)
	c := engine.NewScriptedController()

	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(p).Life; got != 25 {
		t.Errorf("p life = %d, want 25 -- Phase$ Main1,Main2 must pass during Main1", got)
	}
}

// TestSpellCastSkipsTriggerWhenPhaseDoesNotMatch is the regression proof for
// Phase$: the identical watcher, cast during Main1 (Phase$'s own doc
// comment: this port's CastSpell only ever runs during a main phase, so
// Upkeep is unreachable through it) -- restricted here to Upkeep alone, so
// it must never fire from a main-phase cast.
func TestSpellCastSkipsTriggerWhenPhaseDoesNotMatch(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.Player(p).ManaPool.Add(mana.Green, 1)
	g.NewCard(spellCastWatcherDefExtra(t, "Test Watcher", "Phase$ Upkeep"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefManaCost(t, "G"), p, engine.Hand)
	c := engine.NewScriptedController()

	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(p).Life; got != 20 {
		t.Errorf("p life = %d, want unchanged 20 -- Phase$ Upkeep must not pass during Main1", got)
	}
}

// TestAttacksFiresFirstCombatTrigger proves FirstCombat$ True resolves
// (raph_leo_sibling_rivals.txt's own real Mode$ Attacks | ... |
// FirstCombat$ True shape): always true today, since this port has no
// extra-combat mechanism to ever make a second combat phase reachable in
// the same turn.
func TestAttacksFiresFirstCombatTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test Attacker"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Attacker"
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "1", "1"
	raw.Faces[0].Triggers = []string{
		"Mode$ Attacks | ValidCard$ Card.Self | FirstCombat$ True | Execute$ TrigGainLife",
	}
	raw.Faces[0].SVars.Set("TrigGainLife", "DB$ GainLife | Defined$ You | LifeAmount$ 5")
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	attacker := g.NewCard(def, p, engine.Battlefield)
	g.Card(attacker).SummonSick = false

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	ac.QueueAttackTarget(engine.PlayerEntity(other))
	g.DeclareCombatAttackers(ac)
	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(p).Life; got != 25 {
		t.Errorf("p life = %d, want 25 -- FirstCombat$ True must resolve true (no extra-combat mechanism exists to make it false)", got)
	}
}
