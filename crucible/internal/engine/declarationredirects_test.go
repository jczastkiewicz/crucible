package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// recompute runs the pass that rebuilds every player's Layer 8 state.
func recompute(g *engine.Game) {
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
}

// TestDeclarersAreThePlayersThemselvesByDefault proves that with no
// DeclaresAttackers$/DeclaresBlockers$ static in play each player declares
// their own attack and blocks (PhaseHandler.java:535, 662).
func TestDeclarersAreThePlayersThemselvesByDefault(t *testing.T) {
	t.Parallel()

	g, a, b := newTwoPlayerGame(t)
	recompute(g)

	for _, p := range []engine.PlayerID{a, b} {
		if got := g.AttackDeclarer(p); got != p {
			t.Errorf("AttackDeclarer(%v) = %v, want the player themselves", p, got)
		}
		if got := g.BlockDeclarer(p); got != p {
			t.Errorf("BlockDeclarer(%v) = %v, want the player themselves", p, got)
		}
	}
}

// TestDeclaresBlockersStaticRedirectsOnlyAffectedPlayers proves a
// DeclaresBlockers$ static names who blocks for each player its Affected$
// matches, and leaves the attack declaration alone.
func TestDeclaresBlockersStaticRedirectsOnlyAffectedPlayers(t *testing.T) {
	t.Parallel()

	g, a, b := newTwoPlayerGame(t)
	g.NewCard(continuousDef(t, "Test Puppeteer",
		"Mode$ Continuous | Affected$ Player.Opponent | DeclaresBlockers$ You"), a, engine.Battlefield)
	recompute(g)

	if got := g.BlockDeclarer(b); got != a {
		t.Errorf("BlockDeclarer(opponent) = %v, want the static's controller %v", got, a)
	}
	if got := g.BlockDeclarer(a); got != a {
		t.Errorf("BlockDeclarer(controller) = %v, want themselves", got)
	}
	if got := g.AttackDeclarer(b); got != b {
		t.Errorf("AttackDeclarer(opponent) = %v, want themselves: only blocks are redirected", got)
	}
}

// TestDeclaresAttackersStaticRedirectsTheAttackDeclaration is the same for
// DeclaresAttackers$.
func TestDeclaresAttackersStaticRedirectsTheAttackDeclaration(t *testing.T) {
	t.Parallel()

	g, a, b := newTwoPlayerGame(t)
	g.NewCard(continuousDef(t, "Test Warmonger",
		"Mode$ Continuous | Affected$ Player.Opponent | DeclaresAttackers$ You"), b, engine.Battlefield)
	recompute(g)

	if got := g.AttackDeclarer(a); got != b {
		t.Errorf("AttackDeclarer(opponent) = %v, want the static's controller %v", got, b)
	}
	if got := g.BlockDeclarer(a); got != a {
		t.Errorf("BlockDeclarer(opponent) = %v, want themselves: only attacks are redirected", got)
	}
}

// TestNewestDeclaresBlockersEffectWins proves Player.getDeclaresBlockers
// takes the entry with the newest timestamp: with two statics both
// redirecting a's blocks, the later permanent's controller declares, and the
// older one's again once the newer leaves.
func TestNewestDeclaresBlockersEffectWins(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b", "c")
	a, b, c := g.Players()[0], g.Players()[1], g.Players()[2]
	g.SetTurnState(1, a, engine.Main1)
	for _, p := range g.Players() {
		g.Player(p).Life = 20
	}
	g.NewCard(continuousDef(t, "Test Older Puppeteer", "Mode$ Continuous | Affected$ Player | DeclaresBlockers$ You"), b, engine.Battlefield)
	newer := g.NewCard(continuousDef(t, "Test Newer Puppeteer", "Mode$ Continuous | Affected$ Player | DeclaresBlockers$ You"), c, engine.Battlefield)
	recompute(g)

	if got := g.BlockDeclarer(a); got != c {
		t.Fatalf("BlockDeclarer(a) = %v, want the newer static's controller %v", got, c)
	}

	g.Move(newer, engine.Graveyard, c)
	recompute(g)
	if got := g.BlockDeclarer(a); got != b {
		t.Errorf("BlockDeclarer(a) after the newer static left = %v, want the older static's controller %v", got, b)
	}
}

// TestDeclaresBlockersEndsWhenItsSourceLeaves proves the redirect is
// recomputed each pass, as Java's StaticEffect removes it when the effect
// ends (StaticEffect.java:204-205).
func TestDeclaresBlockersEndsWhenItsSourceLeaves(t *testing.T) {
	t.Parallel()

	g, a, b := newTwoPlayerGame(t)
	puppeteer := g.NewCard(continuousDef(t, "Test Puppeteer",
		"Mode$ Continuous | Affected$ Player.Opponent | DeclaresBlockers$ You"), a, engine.Battlefield)
	recompute(g)
	if got := g.BlockDeclarer(b); got != a {
		t.Fatalf("setup: BlockDeclarer(b) = %v, want %v", got, a)
	}

	g.Move(puppeteer, engine.Graveyard, a)
	recompute(g)

	if got := g.BlockDeclarer(b); got != b {
		t.Errorf("BlockDeclarer(b) after the static left = %v, want b themselves", got)
	}
}

// TestAttackingPlayerNamesNobodyOutsideCombat proves Defined$
// AttackingPlayer (Odric, Master Tactician) resolves to the active player
// during combat and to nobody outside it, leaving the redirect unrecorded
// (AbilityUtils.java:1122-1125).
func TestAttackingPlayerNamesNobodyOutsideCombat(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		phase engine.PhaseType
		want  func(a, b engine.PlayerID) engine.PlayerID
	}{
		{engine.Main1, func(_, b engine.PlayerID) engine.PlayerID { return b }},
		{engine.DeclareAttackers, func(a, _ engine.PlayerID) engine.PlayerID { return a }},
	} {
		t.Run(tc.phase.String(), func(t *testing.T) {
			t.Parallel()

			g, a, b := newTwoPlayerGame(t)
			g.SetTurnState(1, a, tc.phase)
			g.NewCard(continuousDef(t, "Test Odric",
				"Mode$ Continuous | Affected$ Player | DeclaresBlockers$ AttackingPlayer"), b, engine.Battlefield)
			recompute(g)

			if got, want := g.BlockDeclarer(b), tc.want(a, b); got != want {
				t.Errorf("BlockDeclarer(b) in %v = %v, want %v", tc.phase, got, want)
			}
		})
	}
}

// TestCamouflageAsksTheRedirectedDeclarer proves a DeclaresBlockers$ static
// changes Camouflage's ReplacedPlayer: the redirected player, not the
// defender, divides the defender's creatures into piles
// (PhaseHandler.java:662-667, CamouflageEffect).
func TestCamouflageAsksTheRedirectedDeclarer(t *testing.T) {
	t.Parallel()
	g, a, b := camouflageGame(t)
	g.NewCard(continuousDef(t, "Test Puppeteer",
		"Mode$ Continuous | Affected$ Player.Opponent | DeclaresBlockers$ You"), a, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)
	x := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)
	declareAttacking(t, g, attacker)
	recompute(g)

	c := newCamouflageController(t)
	c.QueueCardChoice([]engine.CardID{x})
	got := declareBlockers(t, g, c)

	if len(c.deciders) != 1 || c.deciders[0] != a {
		t.Errorf("deciders = %v, want the redirected declarer %v, not the defender %v", c.deciders, a, b)
	}
	if len(c.offers) != 1 || !slices.Equal(c.offers[0], []engine.CardID{x}) {
		t.Errorf("offers = %v, want the defender's creature %v", c.offers, x)
	}
	if !slices.Equal(blocksOf(got, attacker), []engine.CardID{x}) {
		t.Errorf("blocks = %v, want %v blocking", got, x)
	}
}
