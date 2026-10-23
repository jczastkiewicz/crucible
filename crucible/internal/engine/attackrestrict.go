package engine

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// attackLimits is GlobalAttackRestrictions (GlobalAttackRestrictions.java:
// 28-76) for one declaration: the most creatures that may attack at all, and
// the most that may attack each defender some Mode$ AttackRestrict static
// names.
type attackLimits struct {
	// max is the global cap, -1 when none.
	max         int
	perDefender map[EntityID]int
}

// attackLimitsFor is getGlobalRestrictions: the smallest MaxAttackers of the
// statics without ValidDefender$ is the global cap; each possible defender's
// own cap is the smallest of the statics whose ValidDefender$ names it. When
// every possible defender has a cap their sum also caps the whole attack.
func (g *Game) attackLimitsFor(defenders []EntityID) (attackLimits, error) {
	l := attackLimits{max: -1, perDefender: map[EntityID]int{}}
	lower := func(cur *int, n int) {
		if *cur < 0 || n < *cur {
			*cur = n
		}
	}
	for _, d := range defenders {
		l.perDefender[d] = -1
	}
	err := g.eachCombatStatic("AttackRestrict", func(h *Card, face *compile.Face, s *compile.Ability) error {
		n := 1
		if v, ok := s.Param("MaxAttackers"); ok {
			var resolved bool
			if n, resolved = resolveNamedAmount(g, face.Amounts, h, v); !resolved {
				return fmt.Errorf("engine: Mode$ AttackRestrict: MaxAttackers$ %q not resolvable yet", v)
			}
		}
		spec, hasDefender := s.Param("ValidDefender")
		if !hasDefender {
			lower(&l.max, n)
			return nil
		}
		parsed := valid.Parse(spec)
		for _, d := range defenders {
			if g.entityMatches(d, spec, parsed, h.Controller(), h.ID) {
				cur := l.perDefender[d]
				lower(&cur, n)
				l.perDefender[d] = cur
			}
		}
		return nil
	})
	if err != nil {
		return attackLimits{}, err
	}
	sum, all := 0, len(defenders) > 0
	for _, d := range defenders {
		if l.perDefender[d] < 0 {
			all = false
			break
		}
		sum += l.perDefender[d]
	}
	if all {
		lower(&l.max, sum)
	}
	return l, nil
}

// legal is isLegal (GlobalAttackRestrictions.java:31-47): no more attackers
// than the global cap, and none beyond a defender's own cap (a cap of 0 allows
// no attacker at that defender).
func (l attackLimits) legal(attack map[CardID]EntityID) bool {
	if l.max >= 0 && len(attack) > l.max {
		return false
	}
	count := map[EntityID]int{}
	for _, d := range attack {
		count[d]++
	}
	for d, n := range count {
		if limit, ok := l.perDefender[d]; ok && limit >= 0 && n > limit {
			return false
		}
	}
	return true
}
