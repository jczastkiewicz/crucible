// Block costs (CombatUtil.getBlockCost, CombatUtil.java:333-348): a Mode$
// CantBlockUnless static -- Hipparion, Qal Sisma Behemoth, Cowed by Wisdom,
// Heat Wave -- lets a creature be declared as a blocker only to be removed
// from that block again unless its controller pays a cost.
//
// Ported from StaticAbility.getBlockCost (StaticAbility.java:323-327),
// StaticAbilityCantAttackBlock.getBlockCost (:378-399) and the payment loop
// of PhaseHandler.declareBlockersTurnBasedAction (PhaseHandler.java:689-697).
// The cost is paid as attackcost.go pays an attack cost: ConfirmPayCost, then
// the UnlessCost$ parts.

//enginelint:allow id zone card game ability control unlesscost attackcost attackconstraints combat amount valid

package engine

import (
	"fmt"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// blockCostParams are the params a Mode$ CantBlockUnless line may carry and
// this port resolves; anything else is an error (GO-7).
var blockCostParams = map[string]bool{
	"mode": true, "validcard": true, "attacker": true, "cost": true,
	"effectzone": true, "condition": true, "ispresent": true, "presentcompare": true, "presentzone": true,
	"presentplayer": true, "description": true, "triggerdescription": true, "secondary": true,
}

// blockCost is CombatUtil.getBlockCost(game, blocker, attacker): every Mode$
// CantBlockUnless static whose conditions hold, whose ValidCard$ names the
// blocker and whose Attacker$ names the attacker adds its Cost$ (an absent
// param matches, matchesValidParam). The parts are joined as Cost.add sums
// them; costed is false when none applies.
func (g *Game) blockCost(blocker, attacker CardID) (text string, costed bool, err error) {
	var parts []string
	for _, pid := range g.Players() {
		for _, host := range g.traitHosts(pid) {
			h := g.Card(host)
			if h.Def == nil {
				continue
			}
			for _, face := range h.liveTraitFaces() {
				for _, s := range face.Statics {
					if !strings.EqualFold(s.Name, "CantBlockUnless") {
						continue
					}
					part, applies, err := g.staticBlockCost(h, &face, s, blocker, attacker)
					if err != nil {
						return "", false, err
					}
					if applies {
						parts = append(parts, part)
					}
				}
			}
		}
	}
	return strings.Join(parts, " "), len(parts) > 0, nil
}

func (g *Game) staticBlockCost(h *Card, face *compile.Face, s *compile.Ability, blocker, attacker CardID) (string, bool, error) {
	for _, p := range s.Params {
		if !blockCostParams[strings.ToLower(p.Key)] {
			return "", false, fmt.Errorf("engine: Mode$ CantBlockUnless: %s$ not resolvable yet", p.Key)
		}
	}
	if c, ok := s.Param("Condition"); ok && !knownStaticCondition(c) {
		return "", false, fmt.Errorf("engine: Mode$ CantBlockUnless: Condition$ %q not resolvable yet", c)
	}
	if !g.staticConditionsMet(h, s) || !staticValidMatches(g, h, s, "ValidCard", blocker) ||
		!staticValidMatches(g, h, s, "Attacker", attacker) {
		return "", false, nil
	}
	text, ok := s.Param("Cost")
	if !ok {
		return "", false, fmt.Errorf("engine: Mode$ CantBlockUnless without Cost$")
	}
	text, err := g.attackCostAmount(h, face, s, "CantBlockUnless", text, blocker)
	if err != nil {
		return "", false, err
	}
	return text, true, nil
}

// payBlockCosts is PhaseHandler.java:689-697 over one defender's declared
// blocks: for each (blocker, attacker) pair in turn -- per pair, not per
// blocker, so a creature blocking two attackers pays twice (PORT-7) -- its
// controller is asked to pay, and a pair not paid is removed. It returns the
// blocks left. A cost the UnlessCost$ parts do not cover is an error before
// anything is paid.
func (g *Game) payBlockCosts(controller PlayerController, defender PlayerID, blocks []Block) ([]Block, error) {
	costs := make([]*unlessCost, len(blocks))
	anyCost := false
	for i, blk := range blocks {
		if g.Card(blk.Blocker).Controller() != defender {
			continue
		}
		text, costed, err := g.blockCost(blk.Blocker, blk.Attacker)
		if err != nil {
			return nil, err
		}
		if !costed {
			continue
		}
		uc, ok := parseUnlessCost(text)
		if !ok {
			return nil, fmt.Errorf("engine: Mode$ CantBlockUnless: Cost$ %q not resolvable yet", text)
		}
		costs[i], anyCost = &uc, true
	}
	if !anyCost {
		return blocks, nil
	}
	kept := blocks[:0:0]
	for i, blk := range blocks {
		if costs[i] == nil {
			kept = append(kept, blk)
			continue
		}
		// An earlier payment can change a later amount (a sacrificed
		// creature lowers a count), as Java's per-pair getBlockCost call sees.
		uc := costs[i]
		if text, costed, err := g.blockCost(blk.Blocker, blk.Attacker); err != nil {
			return nil, err
		} else if costed {
			if fresh, ok := parseUnlessCost(text); ok {
				uc = &fresh
			}
		}
		if g.payAttackCost(controller, blk.Blocker, *uc) {
			kept = append(kept, blk)
		}
	}
	return kept, nil
}
