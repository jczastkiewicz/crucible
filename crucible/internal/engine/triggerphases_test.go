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

// auraWatcherDefExtra builds an Aura carrying a Mode$ Phase trigger with the
// given extra ValidPlayer$ restriction -- righteous_authority.txt's own real
// "At the beginning of the draw step of enchanted creature's controller,
// that player draws an additional card" shape, ValidPlayer$
// Player.EnchantedController (matchesPlayerProperty, valid.go). The trigger
// executes DB$ GainLife on the Aura's own controller regardless of who
// satisfied ValidPlayer$ -- checkPhaseTriggers' own ValidPlayer$ check only
// gates whether the trigger fires at all, so this isolates that question
// from Execute$'s own separate "who receives the effect" one, which this
// port does not thread a checkPhaseTriggers-set TriggeredPlayer through yet.
func auraWatcherDefExtra(t *testing.T, name, extraParams string) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Enchantment Aura")
	raw.Faces[0].Triggers = []string{
		"Mode$ Phase | " + extraParams + " | TriggerZones$ Battlefield | Execute$ TrigGainLife",
	}
	raw.Faces[0].SVars.Set("TrigGainLife", "DB$ GainLife | Defined$ You | LifeAmount$ 5")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestAdvancePhaseFiresPhaseTriggerWhenEnchantedControllerMatches proves
// ValidPlayer$ Player.EnchantedController (matchesPlayerProperty, valid.go):
// an Aura controlled by p enchants a creature controlled by other: when
// other is the active player entering the draw step, EnchantedController
// resolves to other, matching g.activePlayer, so the trigger fires and p (the
// Aura's own controller) gains life.
func TestAdvancePhaseFiresPhaseTriggerWhenEnchantedControllerMatches(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	creature := g.NewCard(creatureDef(t), other, engine.Battlefield)
	host := g.NewCard(auraWatcherDefExtra(t, "Test Enchant Watcher", "Phase$ Draw | ValidPlayer$ Player.EnchantedController"), p, engine.Battlefield)
	g.Attach(host, creature)
	g.SetTurnState(1, other, engine.Upkeep)

	g.AdvancePhase(engine.NewScriptedController())
	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(p).Life; got != 25 {
		t.Errorf("p life = %d, want 25 -- Player.EnchantedController must match the enchanted creature's controller entering their own draw step", got)
	}
}

// TestAdvancePhaseSkipsPhaseTriggerWhenEnchantedControllerDoesNotMatch is the
// regression proof: the identical Aura/creature pair, but p (the Aura's own
// controller, not the enchanted creature's) is the active player entering
// the draw step -- EnchantedController resolves to other, which does not
// match g.activePlayer (p), so the trigger must not fire.
func TestAdvancePhaseSkipsPhaseTriggerWhenEnchantedControllerDoesNotMatch(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	creature := g.NewCard(creatureDef(t), other, engine.Battlefield)
	host := g.NewCard(auraWatcherDefExtra(t, "Test Enchant Watcher", "Phase$ Draw | ValidPlayer$ Player.EnchantedController"), p, engine.Battlefield)
	g.Attach(host, creature)
	g.SetTurnState(1, p, engine.Upkeep)

	g.AdvancePhase(engine.NewScriptedController())
	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(p).Life; got != 20 {
		t.Errorf("p life = %d, want unchanged 20 -- Player.EnchantedController must not match p's own draw step, since p does not control the enchanted creature", got)
	}
}

// TestMoveSetsDescendedThisTurnForPermanentCardToGraveyard proves Game.Move's
// own "descend" hook (CR's own descend mechanic, Zone.add's own
// "!rollback" branch in Java): a permanent card moved into its owner's
// graveyard from anywhere marks that owner as having descended this turn.
func TestMoveSetsDescendedThisTurnForPermanentCardToGraveyard(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	creature := g.NewCard(creatureDef(t), p, engine.Library)

	g.Move(creature, engine.Graveyard, p)

	if !g.Player(p).DescendedThisTurn {
		t.Error("DescendedThisTurn = false, want true -- a permanent card moved to the graveyard must set it")
	}
}

// nonPermanentDef builds just enough of a *compile.Card for Card.Type() to
// answer "not a permanent" -- an Instant, CR 111.2's own non-permanent card
// type, the shape Game.Move's own descend hook must not set
// DescendedThisTurn for.
func nonPermanentDef(t *testing.T, name string) *compile.Card {
	t.Helper()
	def := &compile.Card{Name: name}
	def.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Instant")
	return def
}

// TestMoveDoesNotSetDescendedThisTurnForNonPermanentCardToGraveyard is the
// regression proof: an Instant resolving to the graveyard is not a permanent
// card, so it must not count toward descend even though it did move into a
// graveyard.
func TestMoveDoesNotSetDescendedThisTurnForNonPermanentCardToGraveyard(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	spell := g.NewCard(nonPermanentDef(t, "Test Instant"), p, engine.Stack)

	g.Move(spell, engine.Graveyard, p)

	if g.Player(p).DescendedThisTurn {
		t.Error("DescendedThisTurn = true, want false -- an Instant is not a permanent card, so it must not set it")
	}
}

// phaseWatcherDefDescended builds a watcher whose own Mode$ Phase trigger
// carries ValidPlayer$ You.descended -- ruin_lurker_bat.txt's own real "at
// the beginning of your end step, if you descended this turn" shape.
func phaseWatcherDefDescended(t *testing.T, name string) *compile.Card {
	t.Helper()

	reg := attachmentTypeRegistry(t)
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "1", "1"
	raw.Faces[0].Triggers = []string{
		"Mode$ Phase | Phase$ End of Turn | ValidPlayer$ You.descended | TriggerZones$ Battlefield | Execute$ TrigGainLife",
	}
	raw.Faces[0].SVars.Set("TrigGainLife", "DB$ GainLife | Defined$ You | LifeAmount$ 5")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestAdvancePhaseFiresPhaseTriggerWhenDescendedThisTurn proves
// ValidPlayer$ You.descended (matchesPlayerProperty, valid.go): a permanent
// card of p's own already moved to p's graveyard this turn, so You.descended
// matches p entering p's own end step.
func TestAdvancePhaseFiresPhaseTriggerWhenDescendedThisTurn(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(phaseWatcherDefDescended(t, "Test Descend Watcher"), p, engine.Battlefield)
	dead := g.NewCard(creatureDef(t), p, engine.Library)
	g.Move(dead, engine.Graveyard, p)
	g.SetTurnState(1, p, engine.Main2)

	g.AdvancePhase(engine.NewScriptedController())
	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(p).Life; got != 25 {
		t.Errorf("p life = %d, want 25 -- You.descended must match p's own end step after a permanent card reached p's graveyard this turn", got)
	}
}

// TestAdvancePhaseSkipsPhaseTriggerWhenNotDescendedThisTurn is the
// regression proof: with no card having reached p's graveyard this turn,
// You.descended must not match.
func TestAdvancePhaseSkipsPhaseTriggerWhenNotDescendedThisTurn(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(phaseWatcherDefDescended(t, "Test Descend Watcher"), p, engine.Battlefield)
	g.SetTurnState(1, p, engine.Main2)

	g.AdvancePhase(engine.NewScriptedController())
	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(p).Life; got != 20 {
		t.Errorf("p life = %d, want unchanged 20 -- You.descended must not match with nothing reaching p's graveyard this turn", got)
	}
}

// TestCleanupStepResetsDescendedThisTurn proves cleanupStep's own reset
// (turn.go): DescendedThisTurn does not persist into the next turn, the
// identical per-turn-counter shape LandsPlayed/CardsDrawnThisTurn already
// have.
func TestCleanupStepResetsDescendedThisTurn(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	dead := g.NewCard(creatureDef(t), p, engine.Library)
	g.Move(dead, engine.Graveyard, p)
	g.SetTurnState(1, p, engine.EndOfTurn)

	g.AdvancePhase(engine.NewScriptedController())

	if g.Player(p).DescendedThisTurn {
		t.Error("DescendedThisTurn = true, want false -- cleanupStep must reset it for the next turn")
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
