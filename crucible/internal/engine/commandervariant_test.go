package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// commanderAttack has a's commander of the given power hit b unblocked once
// and returns it. b's life is set high enough that only commander damage
// can end the game.
func commanderAttack(t *testing.T, g *engine.Game, a, b engine.PlayerID, power string, commander bool) engine.CardID {
	t.Helper()
	g.Player(b).Life = 100
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, power, power), a, engine.Battlefield)
	if commander {
		g.AddCommander(a, attacker)
	}
	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	declareAttackers(t, g, ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	declareBlockers(t, g, bc)
	g.DealCombatDamage(engine.NewScriptedController())
	return attacker
}

// RegisteredPlayer.forVariants (RegisteredPlayer.java:155-157): a Commander
// game starts every seat at 40, and that is the starting life cards read.
func TestCommanderVariantStartsEverySeatAtForty(t *testing.T) {
	t.Parallel()

	g, a, b := newTwoPlayerGameOn(t, scenarioDB(t))
	if g.HasVariant(engine.VariantCommander) || g.Player(a).StartingLife != 20 {
		t.Fatalf("before setup: variant %v, starting life %d", g.HasVariant(engine.VariantCommander), g.Player(a).StartingLife)
	}
	g.SetupVariant(engine.VariantCommander)
	if !g.HasVariant(engine.VariantCommander) {
		t.Fatal("variant not applied")
	}
	for _, pid := range []engine.PlayerID{a, b} {
		if p := g.Player(pid); p.Life != 40 || p.StartingLife != 40 {
			t.Errorf("player %d: life %d, starting life %d, want 40 and 40", pid, p.Life, p.StartingLife)
		}
	}
}

// Player.java:2077-2083: under the Commander variant, 21 combat damage from
// one commander loses the game whatever the life total.
func TestTwentyOneCommanderDamageLosesTheGame(t *testing.T) {
	t.Parallel()

	g, a, b := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetupVariant(engine.VariantCommander)
	cmd := commanderAttack(t, g, a, b, "21", true)
	if got := g.CommanderDamage(b, cmd); got != 21 {
		t.Fatalf("commander damage = %d, want 21", got)
	}
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if !g.Player(b).Lost || !g.Player(a).Won || !g.Over() {
		t.Errorf("lost %v, won %v, over %v; want b lost, a won", g.Player(b).Lost, g.Player(a).Won, g.Over())
	}
}

// Twenty is not enough, and damage from a creature that is not a commander
// is not commander damage.
func TestCommanderDamageBelowTwentyOneOrFromANonCommanderDoesNotLose(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		power     string
		commander bool
		want      int
	}{
		{"twenty", "20", true, 20},
		{"not a commander", "25", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, a, b := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetupVariant(engine.VariantCommander)
			src := commanderAttack(t, g, a, b, tc.power, tc.commander)
			if got := g.CommanderDamage(b, src); got != tc.want {
				t.Errorf("commander damage = %d, want %d", got, tc.want)
			}
			engine.CheckStateBasedActions(g, engine.NewScriptedController())
			if g.Player(b).Lost {
				t.Error("b lost")
			}
		})
	}
}

// The loss is gated on the variant (Player.java:2077); the damage is
// counted either way (Player.java:690).
func TestCommanderDamageWithoutTheVariantIsCountedButNeverLoses(t *testing.T) {
	t.Parallel()

	g, a, b := newTwoPlayerGameOn(t, scenarioDB(t))
	cmd := commanderAttack(t, g, a, b, "21", true)
	if got := g.CommanderDamage(b, cmd); got != 21 {
		t.Errorf("commander damage = %d, want 21", got)
	}
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if g.Player(b).Lost {
		t.Error("b lost without the Commander variant")
	}
}

// You're in Command applies the Commander variant mid-game, as
// AlterAttributeEffect.java:119-124 does, and leaves life alone.
func TestAlterAttributeCommanderAppliesTheVariant(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	target := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	spell := g.NewCard(corpusCard(t, "You're in Command"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.White, 2)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
	c.QueuePayGeneric(mana.ShardW)
	castThenResolve(t, g, p, spell, c)
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	if !g.HasVariant(engine.VariantCommander) {
		t.Error("variant not applied")
	}
	if g.Player(p).StartingLife != 20 {
		t.Errorf("starting life = %d, want 20", g.Player(p).StartingLife)
	}
}

// CR 903.9a (GameAction.stateBasedAction_Commander): a commander put into a
// graveyard or exile since the last check goes to the Command zone when its
// owner agrees; once declined it is not asked about again.
func TestCommanderInGraveyardOrExileReturnsToTheCommandZoneIfItsOwnerAgrees(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		zone   engine.ZoneType
		agree  bool
		wantIn engine.ZoneType
	}{
		{"graveyard, agreed", engine.Graveyard, true, engine.Command},
		{"exile, agreed", engine.Exile, true, engine.Command},
		{"graveyard, declined", engine.Graveyard, false, engine.Graveyard},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetupVariant(engine.VariantCommander)
			cmd := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
			g.AddCommander(p, cmd)
			g.Move(cmd, tc.zone, p)
			c := engine.NewScriptedController()
			c.QueueConfirmEffect(tc.agree)
			engine.CheckStateBasedActions(g, c)
			if got := g.Card(cmd).Zone; got != tc.wantIn {
				t.Fatalf("commander in %v, want %v", got, tc.wantIn)
			}
			// A second check asks nothing: the scripted controller panics on
			// an unqueued confirm.
			engine.CheckStateBasedActions(g, engine.NewScriptedController())
			if got := g.Card(cmd).Zone; got != tc.wantIn {
				t.Errorf("after a second check: commander in %v, want %v", got, tc.wantIn)
			}
		})
	}
}

// Without the variant the return is never offered (GameAction.java:1561).
func TestCommanderInGraveyardWithoutTheVariantStays(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	cmd := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.AddCommander(p, cmd)
	g.Move(cmd, engine.Graveyard, p)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if got := g.Card(cmd).Zone; got != engine.Graveyard {
		t.Errorf("commander in %v, want the graveyard", got)
	}
}

// Player.initVariantsZones (Player.java:3016-3025): a seated commander is in
// its owner's Command zone, designated, with the Commander Effect beside it.
func TestSeatCommanderPutsADesignatedCommanderInTheCommandZone(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	cmd := g.SeatCommander(p, creatureDefPT(t, "3", "3"))
	if got := g.Card(cmd).Zone; got != engine.Command {
		t.Errorf("seated commander in %v, want the Command zone", got)
	}
	if !g.IsCommander(cmd) {
		t.Error("seated card is not a commander")
	}
	if got := g.Zone(engine.Command, p).Cards(); len(got) != 2 || got[0] != cmd {
		t.Errorf("Command zone = %v, want the commander then its effect", got)
	}
}

// subgameObserver records what the subgame looks like when it asks who
// starts.
type subgameObserver struct {
	*engine.ScriptedController
	variant bool
	life    []int
}

func (o *subgameObserver) ChooseStartingPlayer(g *engine.Game, decider engine.PlayerID, first bool) engine.PlayerID {
	o.variant = g.HasVariant(engine.VariantCommander)
	for _, pid := range g.Players() {
		o.life = append(o.life, g.Player(pid).Life)
	}
	return o.ScriptedController.ChooseStartingPlayer(g, decider, first)
}

// A subgame shares the main game's rules (SubgameEffect.java:38) and, with
// no StartingLife$, starts each seat at its own starting life
// (Game.java:375-378): 40 inside a Commander game.
func TestSubgameOfACommanderGamePlaysCommanderAtForty(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.SetupVariant(engine.VariantCommander)
	subgameLands(t, g, p, 2)
	subgameLands(t, g, other, 2)
	obs := &subgameObserver{ScriptedController: subgameController(p)}
	g.Player(p).ManaPool.Add(mana.Green, 1)
	host := g.NewCard(etbChainDef(t, "Test Shape", "DB$ Subgame | RememberPlayers$ Win"), p, engine.Hand)
	if !g.CastSpell(p, host, obs) {
		t.Fatal("cast failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), obs); err != nil {
		t.Fatal(err)
	}
	if !obs.variant {
		t.Error("subgame lost the Commander variant")
	}
	if len(obs.life) != 2 || obs.life[0] != 40 || obs.life[1] != 40 {
		t.Errorf("subgame life = %v, want [40 40]", obs.life)
	}
}

// Two commanders in one graveyard both go back when both owners' answers
// agree: the walk is over a snapshot of the zone (threadSafeIterable).
func TestTwoCommandersInOneGraveyardBothReturn(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetupVariant(engine.VariantCommander)
	var cmds []engine.CardID
	for range 2 {
		id := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
		g.AddCommander(p, id)
		cmds = append(cmds, id)
	}
	for _, id := range cmds {
		g.Move(id, engine.Graveyard, p)
	}
	c := engine.NewScriptedController()
	c.QueueConfirmEffect(true)
	c.QueueConfirmEffect(true)
	engine.CheckStateBasedActions(g, c)
	for _, id := range cmds {
		if got := g.Card(id).Zone; got != engine.Command {
			t.Errorf("commander %d in %v, want the Command zone", id, got)
		}
	}
}

// planeObserver records whether the subgame had a face-up plane and Planar
// Dice by the time a player first took an action in it.
type planeObserver struct {
	*engine.ScriptedController
	main          *engine.Game
	plane, looked bool
}

func (o *planeObserver) TakeAction(g *engine.Game, pid engine.PlayerID) engine.Action {
	if g != o.main && !o.looked {
		o.looked, o.plane = true, g.ActivePlane() != engine.NoCard
	}
	return o.ScriptedController.TakeAction(g, pid)
}

// A Subgame of a Planechase game copies the planar decks and, under the
// shared rules, turns its first plane up after the mulligans
// (SubgameEffect.java:115, GameAction.java:2377-2381).
func TestSubgameOfAPlanechaseGameTurnsAPlaneUp(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.SetupVariant(engine.VariantPlanechase)
	planarDeck(g, p, plainPlane(t, "Here"), plainPlane(t, "There"))
	subgameLands(t, g, p, 2)
	subgameLands(t, g, other, 2)
	obs := &planeObserver{ScriptedController: subgameController(p), main: g}
	g.Player(p).ManaPool.Add(mana.Green, 1)
	host := g.NewCard(etbChainDef(t, "Test Shape", "DB$ Subgame | RememberPlayers$ Win"), p, engine.Hand)
	if !g.CastSpell(p, host, obs) {
		t.Fatal("cast failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), obs); err != nil {
		t.Fatal(err)
	}
	if !obs.looked || !obs.plane {
		t.Errorf("subgame acted %v, plane face up %v; want both", obs.looked, obs.plane)
	}
}
