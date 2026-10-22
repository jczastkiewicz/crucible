// Class levels (CR 716): Card.classLevel, the ClassLevelUp effect and Mode$
// ClassLevelGained. K:Class lines compile into a level-up ability and a
// Continuous static per level (carddb/compile/class.go).

package engine

//enginelint:allow id card ability trigger triggerscan game control valid effecthelpers condition triggerpack staticability

import (
	"fmt"
	"strconv"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// ClassLevel is Card.getClassLevel: 1 until a ClassLevelUp resolves; the
// level ends with the object (a new Card in Java), cleared on leaving the
// battlefield.
func (c *Card) ClassLevel() int {
	if c.classLevel == 0 {
		return 1
	}
	return int(c.classLevel)
}

// classLevelOperand splits a ClassLevel$ restriction ("EQ1", "GE2") into
// its operator and operand, SpellAbilityRestriction.java:177-180.
func classLevelOperand(v string) (op string, n int, ok bool) {
	if len(v) < 3 {
		return "", 0, false
	}
	n, err := strconv.Atoi(v[2:])
	return v[:2], n, err == nil
}

// classLevelRestrictionMet is SpellAbilityRestriction.java:518-523 for host c.
func classLevelRestrictionMet(c *Card, v string) bool {
	op, n, ok := classLevelOperand(v)
	return ok && compareOp(c.ClassLevel(), op, n)
}

// classLevelStaticMet is StaticAbility.checkConditions' ClassLevel$ test
// (:454-460): the host's level is at least the line's.
func classLevelStaticMet(c *Card, v string) bool {
	n, err := strconv.Atoi(v)
	return err == nil && c.ClassLevel() >= n
}

// classLevelGainedParams are the params of a ClassLevelGained trigger this
// port evaluates (TriggerClassLevelGained.java:16-28).
var classLevelGainedParams = effectEventParams("validcard", "classlevel")

// classLevelUpEffect is ClassLevelUpEffect.java: when the host's level is
// the one this ability's ClassLevel$ EQ<n> names it becomes n+1, statics
// are re-evaluated and Mode$ ClassLevelGained fires; at any other level the
// ability does nothing and its cost stays paid.
type classLevelUpEffect struct{}

func (classLevelUpEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	host := g.Card(a.Source)
	v, _ := a.Params.Param("ClassLevel")
	op, n, ok := classLevelOperand(v)
	if !ok || op != "EQ" {
		return fmt.Errorf("engine: ClassLevelUp: ClassLevel$ %q not resolvable", v)
	}
	if host.ClassLevel() != n {
		return nil
	}
	host.classLevel = uint8(n + 1)
	g.refreshLayers()
	level := host.ClassLevel()
	matches := g.scanTriggers([]string{"ClassLevelGained"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, classLevelGainedParams) || !g.triggerCardMatches(h, t, "ValidCard", host) {
			return triggeredObjects{}, false
		}
		if want, ok := t.Param("ClassLevel"); ok {
			if w, err := strconv.Atoi(want); err != nil || w != level {
				return triggeredObjects{}, false
			}
		}
		return triggeredObjects{card: host.ID}, true
	})
	g.pushTriggeredAbilities(controller, matches)
	return nil
}
