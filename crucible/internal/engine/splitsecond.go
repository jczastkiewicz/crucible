// Split second (CR 702.61): while a spell with split second is on the stack,
// players cannot cast other spells or activate abilities that are not mana
// abilities. Java has no static for it: MagicStack.isSplitSecondOnStack
// (MagicStack.java:122-129) is asked by Spell.canPlayFromHost (Spell.java:96)
// and the activated abilities' canPlay (Ability.java:56,
// AbilityActivated.java:85). Land plays, special actions and an effect that
// casts a spell while it resolves are not asked.

package engine

//enginelint:allow game ability card id

// splitSecondOnStack is MagicStack.isSplitSecondOnStack: a spell on the stack
// whose card has split second.
func (g *Game) splitSecondOnStack() bool {
	for i := range g.stack {
		if g.stack[i].spell && g.Card(g.stack[i].Source).HasKeyword("Split second") {
			return true
		}
	}
	return false
}
