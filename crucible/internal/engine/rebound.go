// K:Rebound where a resolved spell leaves the stack.

package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/keyword"
)

// reboundResolved is K:Rebound's replacement (CardFactoryUtil.java:2422-2451,
// Event$ Moved | ValidLKI$ Card.Self+wasCastFromHand+YouOwn+YouCtrl | Origin$
// Stack | Destination$ Graveyard | Fizzle$ False): a spell cast from its
// owner's hand that resolves is exiled instead and remembered, then the
// replacement's chain registers the next-upkeep delayed cast. A Stack-hosted
// replacement is never in an active zone, so like Buyback this runs where the
// spell leaves the stack. Reports whether it took the spell.
func (g *Game) reboundResolved(controller PlayerController, a Ability, c *Card) bool {
	if c.Owner != a.Controller {
		return false
	}
	if m, known := castFromPropertyMatches(c, "wasCastFromHand", a.Controller); !m || !known {
		return false
	}
	for _, face := range c.liveTraitFaces() {
		for _, r := range face.Replacements {
			if keyword.Parse(r.Keyword).Name != "Rebound" {
				continue
			}
			exile := replaceWithSub(r)
			if exile == nil {
				return false
			}
			var chain *compile.Ability
			for _, sub := range exile.Subs {
				if strings.EqualFold(sub.Key, "SubAbility") {
					chain = sub.Ability
				}
			}
			// The override is a ChangeZone from the stack, which marks the
			// card exiled with its own host before the move
			// (ChangeZoneEffect.java:734-738, SpellAbilityEffect.java:
			// 1084-1117): the spell on the stack, listed on that object.
			stamp, listed := g.hostObjectStamp(a.Source)
			exiler := c.Controller()
			g.Move(a.Source, Exile, c.Owner)
			c.exiledWith = exiledWithMark{host: a.Source, stamp: stamp, listed: listed, exiler: exiler, hasExiler: true}
			c.Memory.Remember(CardEntity(a.Source))
			g.runMovedChain(controller, &movedPost{host: c, amounts: face.Amounts, chain: chain})
			return true
		}
	}
	return false
}
