package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// sacOfferRecorder answers every sacrifice choice with the first offered
// candidates and keeps what it was offered, so a test can see which
// permanents SacEachValid$'s removeCandidates left selectable.
type sacOfferRecorder struct {
	*engine.ScriptedController
	offered [][]engine.CardID
}

func (r *sacOfferRecorder) ChoosePermanentsToSacrifice(_ *engine.Game, _ engine.PlayerID, c []engine.CardID, n int) []engine.CardID {
	r.offered = append(r.offered, slices.Clone(c))
	return slices.Clone(c[:n])
}

func (r *sacOfferRecorder) ChoosePermanentsToDestroy(g *engine.Game, p engine.PlayerID, c []engine.CardID, n int) []engine.CardID {
	return r.ChoosePermanentsToSacrifice(g, p, c, n)
}

// TestSacrificeEffectDestroyDestroysTheChosenAndSparesIndestructible:
// Destroy$ (Imperial Edict) has the player choose, then destroys what they
// chose (SacrificeEffect.java:154-160): an indestructible choice stays on the
// battlefield, and the sacrifice-only restriction does not apply.
func TestSacrificeEffectDestroyDestroysTheChosenAndSparesIndestructible(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, opp := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(opp).Life = 20, 20
	victim := g.NewCard(creatureDefPT(t, "1", "1"), opp, engine.Battlefield)
	tough := g.NewCard(indestructibleCreatureDefPT(t, "1", "1"), opp, engine.Battlefield)

	for _, chosen := range []engine.CardID{victim, tough} {
		c := engine.NewScriptedController()
		c.QueueSacrificeChoice([]engine.CardID{chosen})
		if _, err := castETBSacrifice(t, g, p, etbSacrificeTriggerDefParams(t, "Test Destroy", "SacValid$ Creature | Defined$ Opponent | Destroy$ True", nil), c); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
	}
	if g.Card(victim).Zone != engine.Graveyard {
		t.Errorf("chosen creature zone = %v, want Graveyard", g.Card(victim).Zone)
	}
	if g.Card(tough).Zone != engine.Battlefield {
		t.Errorf("indestructible choice zone = %v, want Battlefield", g.Card(tough).Zone)
	}
}

// TestSacrificeEffectDestroyWithOptionalStaysRefused: Destroy$ with Optional$
// or SacEachValid$ has no corpus line and Java prompts a sacrifice there, so
// it fails loudly.
func TestSacrificeEffectDestroyWithOptionalStaysRefused(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)

	_, err := castETBSacrifice(t, g, p, etbSacrificeTriggerDefParams(t, "Test Destroy Optional", "SacValid$ Creature | Destroy$ True | Optional$ True", nil), engine.NewScriptedController())
	if err == nil {
		t.Fatal("ResolveStack: want an error for Destroy$ with Optional$")
	}
}

// TestSacrificeEffectSacEachValidKeepsCardsFreeForLaterAlternatives:
// SacValid$ "Creature & Creature.powerGE2" (SacEachValid$) sacrifices one
// permanent per alternative. The big creature is the only match of the second
// alternative, so removeCandidates (SacrificeEffect.java:229-255) withholds it
// from the first.
func TestSacrificeEffectSacEachValidKeepsCardsFreeForLaterAlternatives(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, opp := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(opp).Life = 20, 20
	big := g.NewCard(creatureDefPT(t, "2", "2"), opp, engine.Battlefield)
	small := g.NewCard(creatureDefPT(t, "1", "1"), opp, engine.Battlefield)

	c := &sacOfferRecorder{ScriptedController: engine.NewScriptedController()}
	if _, err := castETBSacrifice(t, g, p, etbSacrificeTriggerDefParams(t, "Test Each Valid", "SacValid$ Creature & Creature.powerGE2 | Defined$ Opponent | SacEachValid$ True", nil), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if len(c.offered) != 2 || !slices.Equal(c.offered[0], []engine.CardID{small}) || !slices.Equal(c.offered[1], []engine.CardID{big}) {
		t.Fatalf("offers = %v, want [[%d] [%d]]", c.offered, small, big)
	}
	if g.Card(big).Zone != engine.Graveyard || g.Card(small).Zone != engine.Graveyard {
		t.Errorf("zones big=%v small=%v, want both Graveyard", g.Card(big).Zone, g.Card(small).Zone)
	}
}

// TestSacrificeEffectSacEachValidOneMatchSacrificesOnce: a permanent matching
// both alternatives is sacrificed for the first and no longer offered to the
// second, which then has nothing to choose.
func TestSacrificeEffectSacEachValidOneMatchSacrificesOnce(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, opp := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(opp).Life = 20, 20
	only := g.NewCard(creatureDefPT(t, "2", "2"), opp, engine.Battlefield)

	c := &sacOfferRecorder{ScriptedController: engine.NewScriptedController()}
	if _, err := castETBSacrifice(t, g, p, etbSacrificeTriggerDefParams(t, "Test Each Valid One", "SacValid$ Creature & Creature.powerGE2 | Defined$ Opponent | SacEachValid$ True", nil), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if len(c.offered) != 1 {
		t.Fatalf("offers = %v, want one (the second alternative is empty)", c.offered)
	}
	if g.Card(only).Zone != engine.Graveyard {
		t.Errorf("zone = %v, want Graveyard", g.Card(only).Zone)
	}
}
