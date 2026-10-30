package engine_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// blockWith declares attackerName for one player, then blockerName (tapped
// when asked, static beside it) blocking it for the other, and returns an
// error unless that block stands: an illegal block is either refused or, with
// no eligible blocker at all, never asked for.
func blockWith(t *testing.T, attackerName, blockerName, static string, tapped bool) error {
	t.Helper()
	g, a, d := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, a, engine.DeclareAttackers)
	if static != "" {
		g.NewCard(corpusCard(t, static), d, engine.Battlefield)
	}
	attacker := g.NewCard(corpusCard(t, attackerName), a, engine.Battlefield)
	g.Card(attacker).SummonSick = false
	blocker := g.NewCard(corpusCard(t, blockerName), d, engine.Battlefield)
	g.Card(blocker).Tapped = tapped
	c := engine.NewScriptedController()
	c.QueueAttackers([]engine.CardID{attacker})
	declareAttackers(t, g, c)
	c.QueueBlocks([]engine.Block{{Attacker: attacker, Blocker: blocker}})
	blocks, err := g.DeclareCombatBlockers(c)
	if err != nil {
		return err
	}
	if len(blocks) != 1 {
		return errors.New("no block declared")
	}
	return nil
}

// Mode$ BlockTapped (Masako the Humorless, CombatUtil.java:478): tapped
// creatures its controller controls can block as though untapped.
func TestBlockTappedLetsATappedCreatureBlock(t *testing.T) {
	t.Parallel()
	if err := blockWith(t, "Grizzly Bears", "Grizzly Bears", "", true); err == nil {
		t.Error("a tapped creature blocked without Masako")
	}
	if err := blockWith(t, "Grizzly Bears", "Grizzly Bears", "Masako the Humorless", true); err != nil {
		t.Errorf("a tapped creature could not block under Masako: %v", err)
	}
}

// Mode$ CanBlockIfReach (Dragon Hunter, StaticAbilityCantAttackBlock.java:246):
// it blocks a flying Dragon as though it had reach.
func TestCanBlockIfReachBlocksAFlyingDragon(t *testing.T) {
	t.Parallel()
	if err := blockWith(t, "Shivan Dragon", "Grizzly Bears", "", false); err == nil {
		t.Error("a creature without reach blocked a flier")
	}
	if err := blockWith(t, "Shivan Dragon", "Dragon Hunter", "", false); err != nil {
		t.Errorf("Dragon Hunter could not block a Dragon: %v", err)
	}
	if err := blockWith(t, "Serra Angel", "Dragon Hunter", "", false); err == nil {
		t.Error("Dragon Hunter blocked a flier that is not a Dragon")
	}
}

// Mode$ AttackVigilance (Johan, PhaseHandler.java:561): with Johan's
// beginning-of-combat effect live and Johan untapped, attacking does not tap
// its controller's creatures.
func TestAttackVigilanceStaticKeepsAttackersUntapped(t *testing.T) {
	t.Parallel()
	for _, accept := range []bool{false, true} {
		g, a, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, a, engine.Main1)
		g.NewCard(corpusCard(t, "Johan"), a, engine.Battlefield)
		bears := g.NewCard(corpusCard(t, "Grizzly Bears"), a, engine.Battlefield)
		g.Card(bears).SummonSick = false
		c := engine.NewScriptedController()
		c.QueueConfirmOptionalTrigger(accept)
		g.AdvancePhase(c)
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatal(err)
		}
		g.AdvancePhase(c)
		c.QueueAttackers([]engine.CardID{bears})
		declareAttackers(t, g, c)
		if got := g.Card(bears).Tapped; got == accept {
			t.Errorf("accept %v: attacker tapped %v", accept, got)
		}
	}
}

// Mode$ IgnoreShroud (Autumn Willow): once its {G} ability names a player,
// that player may target Willow as though it lacked shroud.
func TestIgnoreShroudLetsTheNamedPlayerTarget(t *testing.T) {
	t.Parallel()
	for _, activate := range []bool{false, true} {
		g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
		willow := g.NewCard(corpusCard(t, "Autumn Willow"), other, engine.Battlefield)
		c := &targetRecorder{ScriptedController: engine.NewScriptedController()}
		if activate {
			g.SetTurnState(1, other, engine.Main1)
			g.Player(other).ManaPool.Add(mana.Green, 1)
			c.QueueTargets([]engine.EntityID{engine.PlayerEntity(p)})
			if !g.ActivateAbility(other, willow, 0, c) {
				t.Fatalf("activate failed: %v", g.TakePendingError())
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatal(err)
			}
			c.offers = nil
		}
		g.SetTurnState(1, p, engine.Main1)
		shock := g.NewCard(corpusCard(t, "Shock"), p, engine.Hand)
		g.Player(p).ManaPool.Add(mana.Red, 1)
		c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
		if !g.CastSpell(p, shock, c) {
			t.Fatalf("cast failed: %v", g.TakePendingError())
		}
		offered := len(c.offers) == 1 && slices.Contains(c.offers[0], engine.CardEntity(willow))
		if offered != activate {
			t.Errorf("activated %v: Willow offered as a target = %v", activate, offered)
		}
	}
}
