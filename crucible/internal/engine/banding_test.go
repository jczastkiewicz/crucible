package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// damageDeciderRecorder is a ScriptedController that notes who each
// AssignCombatDamage call asks.
type damageDeciderRecorder struct {
	*engine.ScriptedController
	deciders []engine.PlayerID
}

func (d *damageDeciderRecorder) AssignCombatDamage(g *engine.Game, decider engine.PlayerID, attacker engine.CardID, blockers []engine.CardID) []engine.DamageAssignment {
	d.deciders = append(d.deciders, decider)
	return d.ScriptedController.AssignCombatDamage(g, decider, attacker, blockers)
}

// A band of Benalish Hero and Grizzly Bears (CR 702.22): a creature blocking
// the Bears blocks the Hero too (CR 702.22h), and the attacking player, whose
// band has banding, divides the blocker's damage between them (CR 702.22j,
// Combat.java:780-781).
func TestBandIsBlockedTogetherAndItsSideDividesTheBlockersDamage(t *testing.T) {
	t.Parallel()

	g, a, b := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, a, engine.Main1)
	hero := g.NewCard(corpusCard(t, "Benalish Hero"), a, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), a, engine.Battlefield)
	wall := g.NewCard(corpusCard(t, "Grizzly Bears"), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{hero, bears})
	ac.QueueBands([][]engine.CardID{{hero, bears}})
	declareAttackers(t, g, ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: wall, Attacker: bears}})
	blocks := declareBlockers(t, g, bc)
	if len(blocks) != 2 || blocks[1] != (engine.Block{Blocker: wall, Attacker: hero}) {
		t.Fatalf("blocks %v, want the block on the Bears and one on the Hero", blocks)
	}

	dc := &damageDeciderRecorder{ScriptedController: engine.NewScriptedController()}
	dc.QueueDamageAssignment([]engine.DamageAssignment{{Blocker: hero, Amount: 2}})
	g.DealCombatDamage(dc)
	if len(dc.deciders) != 1 || dc.deciders[0] != a {
		t.Errorf("blocker damage decided by %v, want the attacking player %v", dc.deciders, a)
	}
	if got := [2]int{g.Card(hero).Damage.Marked, g.Card(bears).Damage.Marked}; got != [2]int{2, 0} {
		t.Errorf("Hero and Bears damage %v, want [2 0]", got)
	}
	if got := g.Player(b).Life; got != 20 {
		t.Errorf("defender life %d: a blocked band deals no damage to the player", got)
	}
}

// Two creatures without banding cannot share a band with one that has it
// (AttackingBand.isValidBand), a band member must be a declared attacker,
// and with no banding creature attacking nobody is asked.
func TestInvalidBandIsAnIllegalDeclaration(t *testing.T) {
	t.Parallel()

	g, a, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, a, engine.Main1)
	hero := g.NewCard(corpusCard(t, "Benalish Hero"), a, engine.Battlefield)
	x := g.NewCard(corpusCard(t, "Grizzly Bears"), a, engine.Battlefield)
	y := g.NewCard(corpusCard(t, "Grizzly Bears"), a, engine.Battlefield)

	for name, bands := range map[string][][]engine.CardID{
		"two without banding": {{hero, x, y}},
		"not an attacker":     {{hero, y}},
		"in two bands":        {{hero, x}, {x}},
	} {
		c := engine.NewScriptedController()
		attackers := []engine.CardID{hero, x, y}
		if name == "not an attacker" {
			attackers = []engine.CardID{hero, x}
		}
		c.QueueAttackers(attackers)
		c.QueueBands(bands)
		_, err := g.DeclareCombatAttackers(c)
		wantIllegal(t, err, "CR 702.22c")
		if g.Card(hero).Tapped {
			t.Fatalf("%s: the refused declaration tapped the Hero", name)
		}
	}

	c := engine.NewScriptedController()
	c.QueueAttackers([]engine.CardID{x})
	c.QueueBands([][]engine.CardID{{x, y}})
	declareAttackers(t, g, c)
}

// Blockers sharing banding divide the blocked attacker's damage: the
// defending player assigns it (CR 702.22k, Combat.java:836-841).
func TestBandingBlockersDivideTheAttackersDamage(t *testing.T) {
	t.Parallel()

	g, a, b := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, a, engine.Main1)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), a, engine.Battlefield)
	hero := g.NewCard(corpusCard(t, "Benalish Hero"), b, engine.Battlefield)
	other := g.NewCard(corpusCard(t, "Grizzly Bears"), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{bears})
	declareAttackers(t, g, ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: hero, Attacker: bears}, {Blocker: other, Attacker: bears}})
	declareBlockers(t, g, bc)

	dc := &damageDeciderRecorder{ScriptedController: engine.NewScriptedController()}
	dc.QueueDamageAssignment([]engine.DamageAssignment{{Blocker: other, Amount: 2}})
	g.DealCombatDamage(dc)
	if len(dc.deciders) != 1 || dc.deciders[0] != b {
		t.Errorf("attacker damage decided by %v, want the defending player %v", dc.deciders, b)
	}
}

// "Bands with other Dinosaur" forms a band only when every member is a
// Dinosaur (AttackingBand.java:45-48): Old Fogey with Colossal Dreadmaw, not
// with Grizzly Bears.
func TestBandsWithOtherNeedsEveryMemberToMatch(t *testing.T) {
	t.Parallel()

	g, a, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, a, engine.Main1)
	fogey := g.NewCard(corpusCard(t, "Old Fogey"), a, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), a, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueAttackers([]engine.CardID{fogey, bears})
	c.QueueBands([][]engine.CardID{{fogey, bears}})
	_, err := g.DeclareCombatAttackers(c)
	wantIllegal(t, err, "CR 702.22c")

	g2, a2, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g2.SetTurnState(1, a2, engine.Main1)
	fogey2 := g2.NewCard(corpusCard(t, "Old Fogey"), a2, engine.Battlefield)
	dino := g2.NewCard(corpusCard(t, "Colossal Dreadmaw"), a2, engine.Battlefield)
	c2 := engine.NewScriptedController()
	c2.QueueAttackers([]engine.CardID{fogey2, dino})
	c2.QueueBands([][]engine.CardID{{fogey2, dino}})
	declareAttackers(t, g2, c2)
}
