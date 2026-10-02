package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// creatureWithKeywords compiles a vanilla creature carrying the given K: lines
// through the real compiler, so ADR-0038's keyword expansion runs on them.
func creatureWithKeywords(t *testing.T, power, toughness string, keywords ...string) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: "keyword-test"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Keyword Test"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = power, toughness
	raw.Faces[0].Keywords = keywords
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// ADR-0038: the combat and death keywords whose CardFactoryUtil branch is one
// trigger and one effect compile into that trigger, and the engine plays it.

// combatOf declares attackers on a's turn (resolving what that triggers), then
// blocks, then resolves what the blocks trigger. A nil block list skips the
// declare-blockers step's decisions.
func combatOf(t *testing.T, g *engine.Game, attackers []engine.CardID, blocks []engine.Block) {
	t.Helper()
	for _, id := range attackers {
		g.Card(id).SummonSick = false
	}
	ac := engine.NewScriptedController()
	ac.QueueAttackers(attackers)
	if _, err := g.DeclareCombatAttackers(ac); err != nil {
		t.Fatalf("DeclareCombatAttackers: %v", err)
	}
	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack after attackers: %v", err)
	}
	bc := engine.NewScriptedController()
	bc.QueueBlocks(blocks)
	if _, err := g.DeclareCombatBlockers(bc); err != nil {
		t.Fatalf("DeclareCombatBlockers: %v", err)
	}
	if err := g.ResolveStack(engine.NewRegistry(), bc); err != nil {
		t.Fatalf("ResolveStack after blockers: %v", err)
	}
}

func powerOf(t *testing.T, g *engine.Game, id engine.CardID) int {
	t.Helper()
	p, ok := g.Card(id).Power()
	if !ok {
		t.Fatalf("power of %v unresolved", id)
	}
	return p
}

// Battle cry (CR 702.91a): each other attacking creature gets +1/+0.
func TestBattleCryPumpsTheOtherAttackers(t *testing.T) {
	t.Parallel()

	g, a, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, a, engine.Main1)
	pest := g.NewCard(corpusCard(t, "Signal Pest"), a, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), a, engine.Battlefield)
	idle := g.NewCard(corpusCard(t, "Grizzly Bears"), a, engine.Battlefield)
	combatOf(t, g, []engine.CardID{pest, bears}, nil)
	if got := powerOf(t, g, bears); got != 3 {
		t.Errorf("attacking Bears power = %d, want 3", got)
	}
	if got := powerOf(t, g, pest); got != 0 {
		t.Errorf("Signal Pest power = %d, want 0: battle cry says other creatures", got)
	}
	if got := powerOf(t, g, idle); got != 2 {
		t.Errorf("non-attacking Bears power = %d, want 2", got)
	}
}

// Dethrone (CR 702.105a): attacking the player with the most life earns a
// +1/+1 counter; attacking a player with less life does not.
func TestDethroneNeedsTheDefenderToHaveTheMostLife(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		mine, theirs int
		want         int
	}{
		{"defender has more life", 10, 20, 1},
		{"defender has the same life", 20, 20, 1},
		{"defender has less life", 20, 10, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, a, b := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, a, engine.Main1)
			g.Player(a).Life, g.Player(b).Life = tc.mine, tc.theirs
			smuggler := g.NewCard(corpusCard(t, "Marchesa's Smuggler"), a, engine.Battlefield)
			combatOf(t, g, []engine.CardID{smuggler}, nil)
			if got := g.Card(smuggler).Counters.Count(engine.P1P1); got != tc.want {
				t.Errorf("+1/+1 counters = %d, want %d", got, tc.want)
			}
		})
	}
}

// Flanking (CR 702.25a): a blocking creature without flanking gets -1/-1;
// one with flanking is untouched.
func TestFlankingShrinksABlockerWithoutFlanking(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		blocker      string
		power, tough int
	}{
		{"plain 2/2 blocker", "Grizzly Bears", 1, 1},
		{"flanking 1/1 blocker", "Bogardan Lancer", 1, 1},
		{"plain 3/3 blocker", "Hill Giant", 2, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, a, b := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, a, engine.Main1)
			lancer := g.NewCard(corpusCard(t, "Bogardan Lancer"), a, engine.Battlefield)
			blocker := g.NewCard(corpusCard(t, tc.blocker), b, engine.Battlefield)
			combatOf(t, g, []engine.CardID{lancer}, []engine.Block{{Blocker: blocker, Attacker: lancer}})
			if got := powerOf(t, g, blocker); got != tc.power {
				t.Errorf("blocker power = %d, want %d", got, tc.power)
			}
			if got, _ := g.Card(blocker).Toughness(); got != tc.tough {
				t.Errorf("blocker toughness = %d, want %d", got, tc.tough)
			}
		})
	}
}

// Afflict (CR 702.130a): when the creature becomes blocked, the defending
// player loses N life.
func TestAfflictDrainsTheDefender(t *testing.T) {
	t.Parallel()

	g, a, b := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, a, engine.Main1)
	g.Player(a).Life, g.Player(b).Life = 20, 20
	eternal := g.NewCard(corpusCard(t, "Wildfire Eternal"), a, engine.Battlefield)
	wall := g.NewCard(creatureDefPT(t, "0", "9"), b, engine.Battlefield)
	combatOf(t, g, []engine.CardID{eternal}, []engine.Block{{Blocker: wall, Attacker: eternal}})
	if got := g.Player(b).Life; got != 16 {
		t.Errorf("defender life = %d, want 16", got)
	}
}

// Soulshift (CR 702.46a): when the creature dies, its controller may return a
// Spirit card with mana value N or less from their graveyard to hand.
func TestSoulshiftReturnsACheapSpirit(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	grafter := g.NewCard(corpusCard(t, "Burr Grafter"), p, engine.Battlefield)
	cheap := g.NewCard(corpusCard(t, "Ageless Guardian"), p, engine.Graveyard)
	dear := g.NewCard(corpusCard(t, "Burr Grafter"), p, engine.Graveyard)
	g.Card(grafter).Damage.Mark(9, false)
	c := engine.NewScriptedController()
	c.QueueConfirmOptionalTrigger(true)
	c.QueueTargets([]engine.EntityID{engine.CardEntity(cheap)})
	engine.CheckStateBasedActions(g, c)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(cheap).Zone; got != engine.Hand {
		t.Errorf("Ageless Guardian in %v, want Hand", got)
	}
	if got := g.Card(dear).Zone; got != engine.Graveyard {
		t.Errorf("the mana value 4 Spirit is in %v, want Graveyard", got)
	}
}

// Mentor (CR 702.134a): when the creature attacks, put a +1/+1 counter on
// target attacking creature with lesser power.
func TestMentorTargetsAnAttackerWithLesserPower(t *testing.T) {
	t.Parallel()

	g, a, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, a, engine.Main1)
	sergeant := g.NewCard(corpusCard(t, "Barging Sergeant"), a, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), a, engine.Battlefield)
	big := g.NewCard(creatureDefPT(t, "5", "5"), a, engine.Battlefield)
	for _, id := range []engine.CardID{sergeant, bears, big} {
		g.Card(id).SummonSick = false
	}
	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{sergeant, bears, big})
	ac.QueueTargets([]engine.EntityID{engine.CardEntity(bears)})
	if _, err := g.DeclareCombatAttackers(ac); err != nil {
		t.Fatalf("DeclareCombatAttackers: %v", err)
	}
	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(bears).Counters.Count(engine.P1P1); got != 1 {
		t.Errorf("Bears have %d +1/+1 counters, want 1", got)
	}
	if got := g.Card(big).Counters.Count(engine.P1P1); got != 0 {
		t.Errorf("the 5/5 has %d counters, want 0: its power is not lesser", got)
	}
}

// Training (CR 702.149a): attacking with another creature with greater power
// earns a +1/+1 counter; attacking with only lesser creatures does not.
func TestTrainingNeedsAnAttackerWithGreaterPower(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, other string
		want        int
	}{
		{"greater power", "Grizzly Bears", 1},
		{"equal power", "Hopeful Initiate", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, a, _ := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, a, engine.Main1)
			trainee := g.NewCard(corpusCard(t, "Hopeful Initiate"), a, engine.Battlefield)
			other := g.NewCard(corpusCard(t, tc.other), a, engine.Battlefield)
			combatOf(t, g, []engine.CardID{trainee, other}, nil)
			if got := g.Card(trainee).Counters.Count(engine.P1P1); got != tc.want {
				t.Errorf("+1/+1 counters = %d, want %d", got, tc.want)
			}
		})
	}
}

// Evolve (CR 702.100a): a creature that enters under your control with
// greater power or greater toughness puts a +1/+1 counter on the evolving
// creature; a smaller one does not.
func TestEvolveNeedsAGreaterEnteringCreature(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		power, tough string
		want         int
	}{
		{"greater power", "3", "1", 1},
		{"greater toughness", "1", "2", 1},
		{"equal", "2", "1", 0},
		{"smaller", "1", "1", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, p, engine.Main1)
			evolver := g.NewCard(corpusCard(t, "Battering Krasis"), p, engine.Battlefield)
			entering := g.NewCard(creatureDefPT(t, tc.power, tc.tough), p, engine.Hand)
			c := engine.NewScriptedController()
			if !g.CastSpell(p, entering, c) {
				t.Fatal("cast failed")
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := g.Card(evolver).Counters.Count(engine.P1P1); got != tc.want {
				t.Errorf("+1/+1 counters = %d, want %d", got, tc.want)
			}
		})
	}
}

// CR 702.100c is an intervening "if": a second Evolve trigger for the same
// entering creature checks again as it resolves, and does nothing once the
// first has made the evolving creature as big.
func TestEvolveChecksAgainOnResolution(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	twice := g.NewCard(creatureWithKeywords(t, "1", "1", "Evolve", "Evolve"), p, engine.Battlefield)
	entering := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Hand)
	c := engine.NewScriptedController()
	if !g.CastSpell(p, entering, c) {
		t.Fatal("cast failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(twice).Counters.Count(engine.P1P1); got != 1 {
		t.Errorf("+1/+1 counters = %d, want 1: the second trigger's condition fails on resolution", got)
	}
}
