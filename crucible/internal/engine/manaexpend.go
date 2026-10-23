// Mode$ ManaExpend: the trigger a spell's payment sets off as the player's
// count of mana spent this turn reaches Amount$ (Player.addExpentThisTurn,
// Player.java:4002-4015, called from MagicStack.add, MagicStack.java:348-360).

//enginelint:allow id card game ability trigger statictrigger staticability

package engine

import (
	"strconv"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// manaExpendParams are the params a ManaExpend trigger may carry that are
// evaluated.
var manaExpendParams = map[string]bool{
	"mode": true, "execute": true, "triggerdescription": true, "triggerzones": true, "optionaldecider": true,
	"secondary": true, "amount": true, "player": true, "checksvar": true, "svarcompare": true,
}

// expendMatches adds spent mana to pid's count this turn and returns the
// ManaExpend triggers that fire: one run for every threshold in (old, old+spent],
// ascending, a threshold skipped inside one payment still firing
// (Player.java:4006-4012). Only a non-copied spell counts
// (MagicStack.java:348), so the cast paths call it and nothing else does.
func (g *Game) expendMatches(pid PlayerID, spent int) []Ability {
	if spent <= 0 {
		return nil
	}
	p := g.Player(pid)
	start := p.expendThisTurn
	p.expendThisTurn += spent
	var matches []Ability
	for i := start + 1; i <= p.expendThisTurn; i++ {
		matches = append(matches, g.scanTriggers([]string{"ManaExpend"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
			if !paramsResolvable(t, manaExpendParams) || !g.triggerPlayerMatches(h, t, "Player", pid) {
				return triggeredObjects{}, false
			}
			// TriggerManaExpend.java:43 parses Amount$ unguarded: a line without
			// one throws in Java; here it never fires.
			amount, _ := t.Param("Amount")
			if want, err := strconv.Atoi(amount); err != nil || want != i {
				return triggeredObjects{}, false
			}
			return triggeredObjects{player: pid}, true
		})...)
	}
	return matches
}
