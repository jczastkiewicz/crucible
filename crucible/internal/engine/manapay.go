// Mana payment with a real choice: CR 601.2h's two-colour hybrid symbol,
// layered on top of Pool.Pay's already-tested plain algorithm (mana.go).

package engine

import "github.com/jczastkiewicz/crucible/internal/mana"

// PayManaCost pays cost from decider's mana pool, asking controller how to
// resolve each two-colour hybrid shard (CR 601.2h) before handing the
// result to [Pool.Pay]. Reports whether it succeeded; the pool is unchanged
// if it did not, the same guarantee Pay itself makes.
//
// A shard offering anything but exactly two plain colours -- a monocoloured
// hybrid ({2/W}), a colourless hybrid ({C/W}), Phyrexian ({W/P}), a hybrid
// Phyrexian ({B/G/P}), {X} or snow -- is not resolved here: it passes
// through unchanged, and Pay's own "unresolvable shard" branch fails the
// whole payment for it, exactly as it already does today. Each is a real
// decision (which colour, mana or life, mana or generic) with no
// PlayerController method built for it yet -- ChooseHybridManaColor is the
// first, not the last (game-state.md's "Mana pool and payment" section).
func (g *Game) PayManaCost(decider PlayerID, cost mana.Cost, controller PlayerController) bool {
	resolved := make([]mana.Shard, 0, len(cost.Shards()))
	for _, s := range cost.Shards() {
		if isTwoColorHybrid(s) {
			choice := controller.ChooseHybridManaColor(g, decider, s.Colors())
			pure, ok := mana.PureShard(choice)
			if !ok {
				return false
			}
			resolved = append(resolved, pure)
			continue
		}
		resolved = append(resolved, s)
	}
	return g.Player(decider).ManaPool.Pay(mana.FromShards(resolved, cost.Generic()))
}

// isTwoColorHybrid reports whether s is a plain two-colour hybrid symbol
// ({W/U}, not {2/W}, {C/W}, {W/P} or {B/G/P}) -- the only shape with exactly
// two colours and no other atom asking a different kind of question.
func isTwoColorHybrid(s mana.Shard) bool {
	return s.Colors().Count() == 2 && !s.IsColorless() && !s.IsOr2Generic() && !s.IsPhyrexian()
}
