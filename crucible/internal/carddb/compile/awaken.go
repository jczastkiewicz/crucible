package compile

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/keyword"
)

// buildAwaken is the spell half of `K:Awaken:<N>:<cost>` (CR 702.113),
// CardFactoryUtil.java:2636-2652: a copy of the card's first spell ability
// (its line compiled afresh, so the chain is its own) whose SubAbility$ chain
// ends with a PutCounter of N +1/+1 counters on a target land the caster
// controls and an Animate of that land into a 0/0 Elemental creature with
// haste. Java also writes `Awaken$ True` on the PutCounter, which nothing
// reads; it is left out. The engine casts it for the awaken cost
// (Face.AwakenSpell). A face with no spell ability, or a line with no count,
// gets none.
func (c *faceCompiler) buildAwaken(out *Face, line string) error {
	args := keyword.Parse(line).Args()
	if len(args) < 2 || args[0] == "" {
		return nil
	}
	for i, spell := range out.Abilities {
		if spell.Record != Spell || i >= len(c.face.Abilities) {
			continue
		}
		awaken, err := c.line(c.face.Abilities[i], Spell)
		if err != nil {
			return fmt.Errorf("keyword %q: %w", line, err)
		}
		put, err := c.line("DB$ PutCounter | CounterType$ P1P1 | CounterNum$ "+args[0]+
			" | ValidTgts$ Land.YouCtrl | TgtPrompt$ Select target land you control", SubAbility)
		if err != nil {
			return fmt.Errorf("keyword %q: %w", line, err)
		}
		animate, err := c.line("DB$ Animate | Defined$ ParentTarget | Power$ 0 | Toughness$ 0 | "+
			"Types$ Creature,Elemental | Duration$ Permanent | Keywords$ Haste", SubAbility)
		if err != nil {
			return fmt.Errorf("keyword %q: %w", line, err)
		}
		put.Keyword, animate.Keyword = line, line
		put.Subs = append(put.Subs, SubRef{Key: "SubAbility", SVar: "KWAwakenAnimate", Ability: animate})
		tail := awaken
		for next := chainSub(tail); next != nil; next = chainSub(tail) {
			tail = next
		}
		tail.Subs = append(tail.Subs, SubRef{Key: "SubAbility", SVar: "KWAwakenPut", Ability: put})
		out.AwakenSpell = awaken
		return nil
	}
	return nil
}
