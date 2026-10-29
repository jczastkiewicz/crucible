package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Mode$ UnspentMana and Mode$ ManaBurn (StaticAbilityUnspentMana.java,
// ManaPool.clearPool): at a phase end Omnath, Locus of Mana keeps only its
// controller's green mana, Upwelling keeps every player's mana, and with
// Yurlok of Scorch Thrash the mana lost is life lost.
func TestUnspentManaAndManaBurnAtPhaseEnd(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		card                 string
		wantPool, wantOther  int
		wantLife, wantOtherL int
	}{
		{"Omnath, Locus of Mana", 2, 0, 20, 20},
		{"Upwelling", 3, 2, 20, 20},
		{"Yurlok of Scorch Thrash", 0, 0, 17, 18},
	} {
		t.Run(tc.card, func(t *testing.T) {
			t.Parallel()
			g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, p, engine.Main1)
			g.NewCard(corpusCard(t, tc.card), p, engine.Battlefield)
			g.Player(p).ManaPool.Add(mana.Green, 2)
			g.Player(p).ManaPool.Add(mana.Red, 1)
			g.Player(other).ManaPool.Add(mana.Blue, 2)
			c := engine.NewScriptedController()

			g.AdvancePhase(c)
			if got := g.Player(p).ManaPool.Total(); got != tc.wantPool {
				t.Errorf("controller's pool %d, want %d", got, tc.wantPool)
			}
			if got := g.Player(other).ManaPool.Total(); got != tc.wantOther {
				t.Errorf("opponent's pool %d, want %d", got, tc.wantOther)
			}
			if got := [2]int{g.Player(p).Life, g.Player(other).Life}; got != [2]int{tc.wantLife, tc.wantOtherL} {
				t.Errorf("life %v, want [%d %d]", got, tc.wantLife, tc.wantOtherL)
			}
		})
	}
}
