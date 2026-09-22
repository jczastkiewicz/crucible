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

// etbSacrificeTriggerDefParams builds a *compile.Card whose own "when
// CARDNAME enters" trigger runs DB$ Sacrifice with the given params string
// appended, and whose own SVars carry any further chained abilities --
// sacrificeEffect itself is unexported, so every case here is driven
// through the real cast+resolve pipeline rather than calling it directly
// (TEST-1), etbDiscardTriggerDefParams' own shape (discardeffect_test.go).
func etbSacrificeTriggerDefParams(t *testing.T, name, extraParams string, svars map[string]string) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "1", "1"
	raw.Faces[0].ManaCost = mana.MustParse("G")
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigSac",
	}
	raw.Faces[0].SVars.Set("TrigSac", "DB$ Sacrifice | "+extraParams)
	for name, body := range svars {
		raw.Faces[0].SVars.Set(name, body)
	}

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// castETBSacrifice casts def (built by etbSacrificeTriggerDefParams) for p
// on controller c and resolves the stack, returning the creature's own
// CardID and ResolveStack's own error -- castETBDiscard's own shape
// (discardeffect_test.go).
func castETBSacrifice(t *testing.T, g *engine.Game, p engine.PlayerID, def *compile.Card, c *engine.ScriptedController) (engine.CardID, error) {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	return creature, g.ResolveStack(engine.NewRegistry(), c)
}

// TestSacrificeEffectDefaultSacValidSacrificesHostItself proves
// SacrificeEffect.resolve's own `valid.equals("Self")` branch -- an absent
// SacValid$ defaults to "Self" (Java's own getParamOrDefault), sacrificing
// the ability's own host with no choice asked at all: a wrongly-reached
// controller call would panic on the empty queue.
func TestSacrificeEffectDefaultSacValidSacrificesHostItself(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	creature, err := castETBSacrifice(t, g, p, etbSacrificeTriggerDefParams(t, "Test Self Sac", "", nil), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(creature).Zone != engine.Graveyard {
		t.Errorf("creature zone = %v, want Graveyard -- an absent SacValid$ must sacrifice the host itself", g.Card(creature).Zone)
	}
}

// TestSacrificeEffectSacValidAsksDeciderFromBattlefield proves the
// corpus's own dominant real shape: SacValid$ present (and not "Self")
// asks Defined$'s own player (You) which of their own matching battlefield
// permanents to sacrifice, through the new ChoosePermanentsToSacrifice
// decision -- ChooseCardsToDiscard's own shape (discardeffect.go) reused
// for a second exactly-N-of-a-set choice.
func TestSacrificeEffectSacValidAsksDeciderFromBattlefield(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	other := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueSacrificeChoice([]engine.CardID{other})

	creature, err := castETBSacrifice(t, g, p, etbSacrificeTriggerDefParams(t, "Test SacValid Choice", "SacValid$ Creature | Defined$ You | Amount$ 1", nil), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(other).Zone != engine.Graveyard {
		t.Errorf("other's zone = %v, want Graveyard -- the queued choice must be sacrificed", g.Card(other).Zone)
	}
	if g.Card(creature).Zone != engine.Battlefield {
		t.Errorf("host creature zone = %v, want Battlefield -- SacValid$'s own choice must not sacrifice the host unless it was chosen", g.Card(creature).Zone)
	}
}

// TestSacrificeEffectDefinedOpponentAsksOpponent proves the player-shaped
// Defined$ dispatch reaches the opponent's own battlefield, not the
// caster's -- discardedTriggerMatches' own "Defined$ Opponent" coverage
// reused for a second effect (discardeffect_test.go).
func TestSacrificeEffectDefinedOpponentAsksOpponent(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, opp := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(opp).Life = 20, 20
	oppCreature := g.NewCard(creatureDefPT(t, "1", "1"), opp, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueSacrificeChoice([]engine.CardID{oppCreature})

	if _, err := castETBSacrifice(t, g, p, etbSacrificeTriggerDefParams(t, "Test Defined Opponent", "SacValid$ Creature | Defined$ Opponent | Amount$ 1", nil), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(oppCreature).Zone != engine.Graveyard {
		t.Errorf("opponent's creature zone = %v, want Graveyard", g.Card(oppCreature).Zone)
	}
}

// TestSacrificeEffectDefaultDefinedIsYouWhenAbsent proves
// AbilityUtils.getDefinedPlayers's own null default -- unlike Discard's/
// PutCounter's own Defined$, which is a real error when absent, a
// SacValid$ line naming no Defined$ at all still resolves, reading "You"
// (goblin_firebug.txt's own real "Sacrifice a Land" shape), scryEffect's
// own identical default (scryeffect.go).
func TestSacrificeEffectDefaultDefinedIsYouWhenAbsent(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	other := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueSacrificeChoice([]engine.CardID{other})

	if _, err := castETBSacrifice(t, g, p, etbSacrificeTriggerDefParams(t, "Test No Defined", "SacValid$ Creature | Amount$ 1", nil), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(other).Zone != engine.Graveyard {
		t.Errorf("other's zone = %v, want Graveyard -- an absent Defined$ must default to You", g.Card(other).Zone)
	}
}

// TestSacrificeEffectSkipsControllerWhenNoCandidates proves an empty
// candidate set skips the ChoosePermanentsToSacrifice call entirely
// (Java's own zero-length validTargets falling into `notEnoughTargets ||
// ...`, this port's own len(candidates) == 0 clamp), rather than asking
// for zero permanents and consuming a queued answer that was never
// provided -- TestDiscardEffectSkipsControllerWhenHandEmpty's own shape.
func TestSacrificeEffectSkipsControllerWhenNoCandidates(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	// No QueueSacrificeChoice call: no land exists anywhere, so this must
	// never reach the controller, or it panics on an exhausted queue.
	if _, err := castETBSacrifice(t, g, p, etbSacrificeTriggerDefParams(t, "Test No Candidates", "SacValid$ Land | Defined$ You | Amount$ 1", nil), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
}

// TestSacrificeEffectRememberSacrificedWritesMemory proves
// RememberSacrificed$ writes the sacrificed card onto the ability's own
// host card's Memory (Card.Memory, memory.go) -- Java's own
// `host.addRemembered(lKICopy)`, this port's first real writer of
// Memory.Remember (memory.go's own doc comment: no writer existed before
// this).
func TestSacrificeEffectRememberSacrificedWritesMemory(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	other := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueSacrificeChoice([]engine.CardID{other})

	creature, err := castETBSacrifice(t, g, p, etbSacrificeTriggerDefParams(t, "Test Remember", "SacValid$ Creature | Defined$ You | Amount$ 1 | RememberSacrificed$ True", nil), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	remembered := g.Card(creature).Memory.Remembered()
	if len(remembered) != 1 || remembered[0] != engine.CardEntity(other) {
		t.Errorf("host's Remembered() = %v, want [%v] -- RememberSacrificed$ must write the sacrificed card", remembered, engine.CardEntity(other))
	}
}

// TestSacrificeEffectChainsIntoSubAbility proves SubAbility$ chains through
// resolveSubAbility (subability.go, Registry.Resolve, effect.go) once
// Sacrifice's own body finishes, the identical shape every other M6
// effect already has -- TestDiscardEffectChainsIntoSubAbility's own
// pairing.
func TestSacrificeEffectChainsIntoSubAbility(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := etbSacrificeTriggerDefParams(t, "Test SubAbility",
		"SubAbility$ DBGainLife",
		map[string]string{"DBGainLife": "DB$ GainLife | Defined$ You | LifeAmount$ 2"})
	c := engine.NewScriptedController()
	creature, err := castETBSacrifice(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(creature).Zone != engine.Graveyard {
		t.Errorf("creature zone = %v, want Graveyard -- Sacrifice's own body must still run", g.Card(creature).Zone)
	}
	if g.Player(p).Life != 22 {
		t.Errorf("p's life = %d, want 22 -- the chained GainLife must run too", g.Player(p).Life)
	}
}

// TestSacrificeEffectRejectsUnlessCost proves a real, representative
// non-pure-mana UnlessCost$ shape (resolveUnlessCost's own gate, effect.go
// -- rottenmouth_viper.txt's own real "unless you sacrifice a nonland
// permanent" among them) fails the whole line loudly rather than silently
// sacrificing unconditionally (PORT-8/GO-7). A pure-mana UnlessCost$ paired
// with a resolvable UnlessPayer$ is a real, resolved shape now --
// TestSacrificeEffectResolvesUnlessCostWhenNotPaid/WhenPaid cover it.
func TestSacrificeEffectRejectsUnlessCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	_, err := castETBSacrifice(t, g, p, etbSacrificeTriggerDefParams(t, "Test UnlessCost", "UnlessCost$ Sac<1/Creature> | UnlessPayer$ You", nil), c)
	if err == nil {
		t.Fatal("ResolveStack: got nil error, want one naming UnlessCost")
	}
	if !strings.Contains(err.Error(), "UnlessCost") {
		t.Errorf("ResolveStack error = %q, want it to name UnlessCost$", err.Error())
	}
}

// TestSacrificeEffectFiresWhenConditionCheckSVarIsMet proves
// subAbilityConditionMet (condition.go) gates Sacrifice's own resolution
// the identical way it gates PutCounter's/Pump's/DealDamage's/GainLife's/
// Discard's.
func TestSacrificeEffectFiresWhenConditionCheckSVarIsMet(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	def := etbSacrificeTriggerDefParams(t, "Test Condition Met",
		"ConditionCheckSVar$ X | ConditionSVarCompare$ GE1", map[string]string{"X": "1"})
	creature, err := castETBSacrifice(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(creature).Zone != engine.Graveyard {
		t.Errorf("creature zone = %v, want Graveyard -- ConditionCheckSVar$ X | ConditionSVarCompare$ GE1 holds (X is 1)", g.Card(creature).Zone)
	}
}

// TestSacrificeEffectNoOpsWhenConditionCheckSVarIsNotMet is the negative
// control: X GE1 fails (X is 0), so the ability does nothing.
func TestSacrificeEffectNoOpsWhenConditionCheckSVarIsNotMet(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	def := etbSacrificeTriggerDefParams(t, "Test Condition Unmet",
		"ConditionCheckSVar$ X | ConditionSVarCompare$ GE1", map[string]string{"X": "0"})
	creature, err := castETBSacrifice(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(creature).Zone != engine.Battlefield {
		t.Errorf("creature zone = %v, want Battlefield -- ConditionCheckSVar$ X | ConditionSVarCompare$ GE1 fails (X is 0)", g.Card(creature).Zone)
	}
}

// TestSacrificeEffectFiresOwnSacrificedTrigger proves CR 701.20's own
// Mode$ Sacrificed fires for the sacrificed card's own trigger
// (checkSacrificedTriggers's own "own" half, trigger.go), using the
// card's still-live battlefield state -- fired before the move to
// graveyard, right where Player.addSacrificedThisTurn's own call sits in
// GameAction.sacrifice, ahead of sacrificeDestroy's own moveToGraveyard.
func TestSacrificeEffectFiresOwnSacrificedTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	raw := &carddb.Card{Filename: "Test Own Sacrificed Trigger"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Own Sacrificed Trigger"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "1", "1"
	raw.Faces[0].ManaCost = mana.MustParse("G")
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigSac",
		"Mode$ Sacrificed | ValidCard$ Card.Self | Execute$ TrigGain",
	}
	raw.Faces[0].SVars.Set("TrigSac", "DB$ Sacrifice")
	raw.Faces[0].SVars.Set("TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 1")
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	c := engine.NewScriptedController()
	creature, err := castETBSacrifice(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(creature).Zone != engine.Graveyard {
		t.Errorf("creature zone = %v, want Graveyard", g.Card(creature).Zone)
	}
	if g.Player(p).Life != 21 {
		t.Errorf("p's life = %d, want 21 -- the card's own Mode$ Sacrificed trigger must fire before it leaves the battlefield", g.Player(p).Life)
	}
}

// TestSacrificeEffectFiresWatchingPermanentsSacrificedTrigger proves
// checkSacrificedTriggers' own "other" half (otherSacrificedTriggerMatches,
// trigger.go): a permanent that is not itself sacrificed still sees
// another permanent's own Mode$ Sacrificed trigger, and ValidPlayer$
// narrows whose sacrifice it reacts to (matchesPlayerBase, the same
// dispatch discardedTriggerMatches' own ValidPlayer$ already uses).
func TestSacrificeEffectFiresWatchingPermanentsSacrificedTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, opp := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(opp).Life = 20, 20

	raw := &carddb.Card{Filename: "Test Watcher"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Watcher"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Enchantment")
	raw.Faces[0].Triggers = []string{
		"Mode$ Sacrificed | ValidCard$ Creature | ValidPlayer$ You | Execute$ TrigGain",
	}
	raw.Faces[0].SVars.Set("TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 1")
	watcherDef, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile watcher: %v", err)
	}
	g.NewCard(watcherDef, p, engine.Battlefield)

	oppCreature := g.NewCard(creatureDefPT(t, "1", "1"), opp, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueSacrificeChoice([]engine.CardID{oppCreature})
	if _, err := castETBSacrifice(t, g, p, etbSacrificeTriggerDefParams(t, "Test Sac Opp Creature", "SacValid$ Creature | Defined$ Opponent | Amount$ 1", nil), c); err != nil {
		t.Fatalf("ResolveStack (opponent's creature): %v", err)
	}
	if g.Player(p).Life != 20 {
		t.Errorf("p's life = %d, want unchanged 20 -- ValidPlayer$ You must not fire for the opponent's own sacrifice", g.Player(p).Life)
	}

	myCreature := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	c.QueueSacrificeChoice([]engine.CardID{myCreature})
	if _, err := castETBSacrifice(t, g, p, etbSacrificeTriggerDefParams(t, "Test Sac My Creature", "SacValid$ Creature | Defined$ You | Amount$ 1", nil), c); err != nil {
		t.Fatalf("ResolveStack (my creature): %v", err)
	}
	if g.Player(p).Life != 21 {
		t.Errorf("p's life = %d, want 21 -- the watcher must fire for p's own sacrifice", g.Player(p).Life)
	}
}
