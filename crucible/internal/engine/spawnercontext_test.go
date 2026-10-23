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

// TestOriginalHostIsTheAuraThatGrantedTheTrigger proves Object$ OriginalHost
// (SpellAbility.getOriginalHost): Nettlevine Blight grants its enchanted
// permanent "at the beginning of your end step, sacrifice this permanent and
// attach Nettlevine Blight to a creature or land you control" -- "you" is the
// permanent's controller. The granted
// trigger's Attach moves the Aura, not the sacrificed permanent that holds it.
func TestOriginalHostIsTheAuraThatGrantedTheTrigger(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 5)
	libraryCards(t, g, other, 5)
	cursed := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Battlefield)
	theirs := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Battlefield)
	blight := g.NewCard(corpusCard(t, "Nettlevine Blight"), p, engine.Battlefield)
	g.Attach(blight, cursed)
	g.SetTurnState(1, other, engine.Main1)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{theirs})
	advanceUntil(t, g, c, func() bool { return g.ActivePhase() == engine.EndOfTurn })
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	if z := g.Card(cursed).Zone; z != engine.Graveyard {
		t.Errorf("enchanted creature zone = %v, want Graveyard (sacrificed)", z)
	}
	if z := g.Card(blight).Zone; z != engine.Battlefield {
		t.Fatalf("Nettlevine Blight zone = %v, want Battlefield", z)
	}
	if host, _ := g.Card(blight).AttachedTo(); host != theirs {
		t.Errorf("Nettlevine Blight attached to %v, want %v", host, theirs)
	}
}
