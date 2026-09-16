// Mana payment with a real choice: CR 601.2h's two-colour, monocoloured and
// colourless hybrid symbols, CR 118.4's single-colour and hybrid Phyrexian
// mana, and CR 106.6's choice of which mana type pays a generic amount,
// layered on top of Pool.Pay's already-tested plain algorithm (mana.go).

package engine

import "github.com/jczastkiewicz/crucible/internal/mana"

// PayManaCost pays cost from decider's mana pool, asking controller the
// value of X if the cost carries one (CR 601.2b), how to resolve each
// two-colour hybrid, monocoloured hybrid, colourless hybrid, single-colour
// Phyrexian and hybrid Phyrexian shard (CR 601.2h, CR 118.4), then which
// mana type covers each unit of the cost's generic amount (CR 106.6),
// before handing the result to [Pool.Pay]. Reports whether it succeeded;
// the pool is unchanged if it did not, the same guarantee Pay itself
// makes -- and life is deducted only after Pay reports success, so a
// Phyrexian shard resolved to life never costs life on a payment that fails
// for an unrelated shard.
//
// A shard offering anything else -- snow -- is not resolved here: it passes
// through unchanged, and Pay's own "unresolvable shard" branch fails the
// whole payment for it, exactly as it already does today. It is a real
// decision with no PlayerController method built for it yet --
// ChooseHybridManaColor, ChoosePayMonocoloredHybrid, ChoosePayColorlessHybrid,
// ChoosePayPhyrexian, ChoosePayHybridPhyrexian, ChoosePayGeneric and
// ChoosePayX are the first seven, not the last (game-state.md's "Mana pool
// and payment" section).
func (g *Game) PayManaCost(decider PlayerID, cost mana.Cost, controller PlayerController) bool {
	resolved := make([]mana.Shard, 0, len(cost.Shards())+cost.Generic())
	generic := cost.Generic()
	life := 0

	// CR 601.2b: X is announced once, before any other part of the cost is
	// paid, and every X symbol the cost carries stands for that same
	// announced value -- not one value each.
	if countX := cost.CountX(); countX > 0 {
		x := controller.ChoosePayX(g, decider, cost)
		if x < 0 {
			return false
		}
		generic += x * countX
	}

	for _, s := range cost.Shards() {
		switch {
		case s.IsX():
			// Resolved above, ahead of this loop: every X symbol already
			// folded into generic, so it contributes nothing here.
		case isTwoColorHybrid(s):
			choice := controller.ChooseHybridManaColor(g, decider, s.Colors())
			pure, ok := mana.PureShard(choice)
			if !ok {
				return false
			}
			resolved = append(resolved, pure)
		case s.IsOr2Generic():
			color := s.Colors()
			if controller.ChoosePayMonocoloredHybrid(g, decider, color, s.CMC()) {
				pure, ok := mana.PureShard(color)
				if !ok {
					return false
				}
				resolved = append(resolved, pure)
			} else {
				generic += s.CMC()
			}
		case isColorlessHybrid(s):
			color := s.Colors()
			if controller.ChoosePayColorlessHybrid(g, decider, color) {
				pure, ok := mana.PureShard(color)
				if !ok {
					return false
				}
				resolved = append(resolved, pure)
			} else {
				resolved = append(resolved, mana.ShardC)
			}
		case isSingleColorPhyrexian(s):
			color := s.Colors()
			if controller.ChoosePayPhyrexian(g, decider, color) {
				pure, ok := mana.PureShard(color)
				if !ok {
					return false
				}
				resolved = append(resolved, pure)
			} else {
				life += 2
			}
		case isHybridPhyrexian(s):
			choice := controller.ChoosePayHybridPhyrexian(g, decider, s.Colors())
			if choice.Count() == 0 {
				life += 2
			} else {
				pure, ok := mana.PureShard(choice)
				if !ok {
					return false
				}
				resolved = append(resolved, pure)
			}
		default:
			resolved = append(resolved, s)
		}
	}

	for i := 0; i < generic; i++ {
		resolved = append(resolved, controller.ChoosePayGeneric(g, decider))
	}

	if !g.Player(decider).ManaPool.Pay(mana.FromShards(resolved, 0)) {
		return false
	}
	if life > 0 {
		g.Player(decider).Life -= life
		g.sink.Emit(Event{Kind: LifeChanged, Source: NoCard, Target: PlayerEntity(decider), Amount: int32(-life)})
	}
	return true
}

// isTwoColorHybrid reports whether s is a plain two-colour hybrid symbol
// ({W/U}, not {2/W}, {C/W}, {W/P} or {B/G/P}) -- the only shape with exactly
// two colours and no other atom asking a different kind of question.
func isTwoColorHybrid(s mana.Shard) bool {
	return s.Colors().Count() == 2 && !s.IsColorless() && !s.IsOr2Generic() && !s.IsPhyrexian()
}

// isColorlessHybrid reports whether s is a colourless hybrid symbol
// ({C/W}) -- exactly one colour plus the colourless atom, and nothing else.
// ShardC itself carries the colourless atom with zero colours (excluded by
// Colors().Count() == 1); a monocoloured hybrid never also carries the
// colourless atom (shard.go's table has no shard combining the two).
func isColorlessHybrid(s mana.Shard) bool {
	return s.Colors().Count() == 1 && s.IsColorless() && !s.IsOr2Generic() && !s.IsPhyrexian()
}

// isSingleColorPhyrexian reports whether s is a plain Phyrexian mana symbol
// ({W/P}, not {B/G/P}) -- exactly one colour plus the or-2-life atom, and
// nothing else. A hybrid Phyrexian shard has two colour bits set alongside
// the same atom, so Colors().Count() == 1 already excludes it.
func isSingleColorPhyrexian(s mana.Shard) bool {
	return s.Colors().Count() == 1 && s.IsPhyrexian() && !s.IsColorless() && !s.IsOr2Generic()
}

// isHybridPhyrexian reports whether s is a hybrid Phyrexian mana symbol
// ({B/G/P}) -- exactly two colours plus the or-2-life atom. isTwoColorHybrid
// already excludes this shape (it requires !IsPhyrexian()), so the two
// predicates never both match the same shard.
func isHybridPhyrexian(s mana.Shard) bool {
	return s.Colors().Count() == 2 && s.IsPhyrexian()
}
