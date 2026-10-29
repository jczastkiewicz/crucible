package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Mode$ BlockRestrict | MaxBlockers$ 1 (Dueling Grounds, which also lets only
// one creature attack): a second blocker is an illegal declaration, one
// blocker is fine (CombatUtil.java:434-435).
func TestBlockRestrictCapsTheBlockers(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		blocks  int
		wantErr bool
	}{{"one", 1, false}, {"two", 2, true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, a, d := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, a, engine.DeclareAttackers)
			g.NewCard(corpusCard(t, "Dueling Grounds"), a, engine.Battlefield)
			attacker := g.NewCard(corpusCard(t, "Grizzly Bears"), a, engine.Battlefield)
			g.Card(attacker).SummonSick = false
			c := engine.NewScriptedController()
			c.QueueAttackers([]engine.CardID{attacker})
			declareAttackers(t, g, c)
			var blocks []engine.Block
			for range tc.blocks {
				blocker := g.NewCard(corpusCard(t, "Grizzly Bears"), d, engine.Battlefield)
				blocks = append(blocks, engine.Block{Attacker: attacker, Blocker: blocker})
			}
			c.QueueBlocks(blocks)
			_, err := g.DeclareCombatBlockers(c)
			if (err != nil) != tc.wantErr {
				t.Errorf("DeclareCombatBlockers error %v, want error %v", err, tc.wantErr)
			}
		})
	}
}
