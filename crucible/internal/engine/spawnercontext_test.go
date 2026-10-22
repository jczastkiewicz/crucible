package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestSpawnerDefinedReadsSpawnersTriggeredTarget proves
// AbilityUtils.adjustTriggerContext: a reflexive trigger's `Defined$
// Spawner>TriggeredTarget` is the damaged player of the DamageDone trigger
// that spawned it (Biting-Palm Ninja's "target opponent" reveal), not the
// ImmediateTrigger's own (empty) triggering objects.
func TestSpawnerDefinedReadsSpawnersTriggeredTarget(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Watcher", "Enchantment",
		"T:Mode$ DamageDone | ValidTarget$ Player | TriggerZones$ Battlefield | Execute$ TrigReflex",
		"SVar:TrigReflex:DB$ ImmediateTrigger | Execute$ TrigLose",
		"SVar:TrigLose:DB$ LoseLife | Defined$ Spawner>TriggeredTarget | LifeAmount$ 3"), p, engine.Battlefield)
	dealFrom(t, g, p, engine.PlayerEntity(other), "2")
	sba(g)
	if got := g.Player(other).Life; got != 15 {
		t.Errorf("damaged player life = %d, want 15 (2 damage, then 3 lost)", got)
	}
	if got := g.Player(p).Life; got != 20 {
		t.Errorf("watcher's controller life = %d, want 20", got)
	}
}

// TestDamageAllRecordsFilteredTargets proves TriggerDamageAll.
// setTriggeringObjects: AbilityKey.Targets is the damaged players the
// ValidTarget$ filter kept, so Hordewing Skaab's X
// (TriggeredPlayersTargets$Amount) counts the opponents dealt damage, here
// one, and not the creature that was hit by the same action.
func TestDamageAllRecordsFilteredTargets(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Skaab", "Enchantment",
		"T:Mode$ DamageAll | ValidTarget$ Opponent | TriggerZones$ Battlefield | Execute$ TrigGain",
		"SVar:TrigGain:DB$ GainLife | Defined$ You | LifeAmount$ X",
		"SVar:X:TriggeredPlayersTargets$Amount"), p, engine.Battlefield)
	dealFrom(t, g, p, engine.PlayerEntity(other), "2")
	sba(g)
	if got := g.Player(p).Life; got != 21 {
		t.Errorf("life = %d, want 21 (one opponent damaged, X = 1)", got)
	}
}
