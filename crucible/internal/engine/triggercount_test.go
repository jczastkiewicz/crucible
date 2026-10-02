package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// triggerCountWatcher is an enchantment whose one trigger runs a LoseLife of X
// on its opponent, X being `TriggerCount$<key>` of the event that fired it.
func triggerCountWatcher(t *testing.T, trigger, key string) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: "count-watcher"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Count Watcher"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Enchantment")
	raw.Faces[0].Triggers = []string{trigger + " | TriggerZones$ Battlefield | Execute$ Drain"}
	raw.Faces[0].SVars.Set("Drain", "DB$ LoseLife | Defined$ Player.Opponent | LifeAmount$ X")
	raw.Faces[0].SVars.Set("X", "TriggerCount$"+key)
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// TriggerCount$DamageAmount (236 corpus lines) is the damage the trigger saw.
func TestTriggerCountDamageAmountIsTheDamageDealt(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(triggerCountWatcher(t, "Mode$ DamageDone | ValidSource$ Card | ValidTarget$ Opponent", "DamageAmount"), p, engine.Battlefield)
	bolt := g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 1)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	castThenResolve(t, g, p, bolt, c)
	// The Bolt's 3 damage, then the watcher's 3 more.
	if got := g.Player(other).Life; got != 14 {
		t.Errorf("opponent life = %d, want 14: 3 damage and a 3-life loss", got)
	}
}

// TriggerCount$LifeAmount is the life the player just gained.
func TestTriggerCountLifeAmountIsTheLifeGained(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(triggerCountWatcher(t, "Mode$ LifeGained | ValidPlayer$ You", "LifeAmount"), p, engine.Battlefield)
	spell := g.NewCard(gainInstant(t, "Gain", "W", "4"), p, engine.Hand)
	c := engine.NewScriptedController()
	castOn(t, g, p, spell, c)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(other).Life; got != 16 {
		t.Errorf("opponent life = %d, want 16: the 4 life gained", got)
	}
}

// A key the trigger mode did not record is unresolved, not 0: the ability
// fails instead of draining nothing (GO-7).
func TestTriggerCountOfAnUnrecordedKeyFailsClosed(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(triggerCountWatcher(t, "Mode$ LifeGained | ValidPlayer$ You", "DamageAmount"), p, engine.Battlefield)
	spell := g.NewCard(gainInstant(t, "Gain", "W", "4"), p, engine.Hand)
	c := engine.NewScriptedController()
	castOn(t, g, p, spell, c)
	if err := g.ResolveStack(engine.NewRegistry(), c); err == nil {
		t.Error("resolved a TriggerCount$ key the trigger never recorded")
	}
	if got := g.Player(other).Life; got != 20 {
		t.Errorf("opponent life = %d, want 20", got)
	}
}

// TriggerCount$Amount of a CounterAddedOnce trigger is the number of counters
// the one placement put on the card.
func TestTriggerCountAmountIsTheCountersPlaced(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(triggerCountWatcher(t, "Mode$ CounterAddedOnce | ValidCard$ Creature", "Amount"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	raw := &carddb.Card{Filename: "counter-spell"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Counter Spell"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Instant")
	raw.Faces[0].ManaCost = mana.MustParse("G")
	raw.Faces[0].Abilities = []string{"SP$ PutCounter | ValidTgts$ Creature | CounterType$ P1P1 | CounterNum$ 2"}
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	spell := g.NewCard(def, p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(bears)})
	castThenResolve(t, g, p, spell, c)
	if got := g.Player(other).Life; got != 18 {
		t.Errorf("opponent life = %d, want 18: two counters in one placement", got)
	}
}
