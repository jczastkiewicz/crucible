// Morph, Megamorph and Disguise (CR 702.37, 702.168): a card with one of them
// may be cast face down as a 2/2 creature spell for {3}, and its face-down
// permanent turned face up for the keyword's cost; a manifested or cloaked
// creature card turns face up for its mana cost (CR 701.34c, 701.58c).
//
// Ported from CardFactoryUtil.java: abilityCastFaceDown (:80-121) is the {3}
// spell ADR-0048's cast path makes; abilityTurnFaceUp (:130-157) is an ST$
// SetState, an AbilityStatic that MagicStack.add resolves at once
// (MagicStack.java:289-292), so turning face up is a special action that
// never uses the stack (CR 116.2b). The Morph (:3119-3123), Megamorph
// (:3124-3128, its replacement putting a +1/+1 counter :2363-2376) and
// Disguise (:2764-2779, ward {2} face down and an optional ReduceCost$
// SVar) branches build them; Card.java:7588-7599 offers them while the card
// is face down, from its face-up state, plus ManifestUp/CloakUp for a
// manifested or cloaked creature card with a mana cost (CardState.java:
// 1288-1298). Card.turnFaceUp (Card.java:878-932) turns it up.

package engine

//enginelint:allow game card id ability control zone manapay manarestrict unlesscost staticability valid replacementfaces triggerpack counteradded facedown illusionarymask castoptions altcost parts amount

import (
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/keyword"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// morphCastCost is the {3} of abilityCastFaceDown (CardFactoryUtil.java:81).
var morphCastCost = mana.MustParse("3")

// faceDownCastOption is the face-down cast of c from the hand
// (abilityCastFaceDown): {3} in place of its mana cost, the spell the
// face-down 2/2, with ward {2} for Disguise. ok is false without one of the
// three keywords.
func faceDownCastOption(c *Card) (castOption, bool) {
	for _, line := range c.KeywordLines() {
		switch keyword.Parse(line).Name {
		case "Morph", "Megamorph":
			return castOption{hasAlt: true, alt: morphCastCost, faceDown: true}, true
		case "Disguise":
			return castOption{hasAlt: true, alt: morphCastCost, faceDown: true, faceDownWard: true}, true
		}
	}
	return castOption{}, false
}

// faceUpWay is one turn-face-up ability of a face-down permanent: its
// SpellAbility flag (MorphUp, DisguiseUp, ManifestUp or CloakUp), its cost
// text, and for Megamorph the +1/+1 counter it puts on the permanent. reduce
// is a Disguise's ReduceCost$ SVar, "" for none.
type faceUpWay struct {
	key       string
	cost      string
	megamorph bool
	reduce    string
}

// faceUpWays are the ways c, a face-down permanent, may be turned face up,
// in Java's order: the face-up state's keyword abilities, then the
// manifested or cloaked creature card's mana cost (Card.java:7588-7599).
// Reading the face-up definition is CR 702.37e's test: a permanent that
// would have no morph cost face up has no way. A cost naming X is not
// offered (GO-7: Java reads the card's own X SVar, no corpus card is played
// that way here, game-state.md).
func faceUpWays(c *Card) []faceUpWay {
	if !c.IsFaceDown() || c.faceUpDef == nil {
		return nil
	}
	var out []faceUpWay
	for _, line := range c.faceUpDef.Faces[0].Keywords {
		k := keyword.Parse(line)
		args := k.Args()
		if len(args) == 0 || args[0] == "" {
			continue
		}
		w := faceUpWay{cost: args[0]}
		switch k.Name {
		case "Morph":
			w.key = "MorphUp"
		case "Megamorph":
			w.key, w.megamorph = "MorphUp", true
		case "Disguise":
			w.key = "DisguiseUp"
			if len(args) > 1 {
				w.reduce = args[1]
			}
		default:
			continue
		}
		if costNamesX(w.cost) {
			continue
		}
		out = append(out, w)
	}
	face := &c.faceUpDef.Faces[0]
	if (c.Manifested || c.Cloaked) && face.Type.Has(cardtype.Creature) && !face.ManaCost.IsNoCost() && face.ManaCost.CountX() == 0 {
		key := "ManifestUp"
		if c.Cloaked {
			key = "CloakUp"
		}
		out = append(out, faceUpWay{key: key, cost: face.ManaCost.String()})
	}
	return out
}

// TurnFaceUp is the turn-face-up special action (CR 116.2b, 702.37e): pid
// pays the cost of c's way'th turn-face-up ability (faceUpWays) and turns
// their face-down permanent face up, any time they have priority. False when
// card is not pid's face-down permanent, has no such way, a Layer$
// CantHappen Event$ TurnFaceUp line forbids it (AbilityStatic.java:61-63),
// or the cost is not paid.
func (g *Game) TurnFaceUp(pid PlayerID, card CardID, way int, controller PlayerController) bool {
	c := g.Card(card)
	if c.Zone != Battlefield || c.Controller() != pid || !c.IsFaceDown() || !g.canBeTurnedFaceUp(card) {
		return false
	}
	ways := faceUpWays(c)
	if way < 0 || way >= len(ways) {
		return false
	}
	w := ways[way]
	uc, ok := parseUnlessCost(w.cost)
	if !ok {
		return false
	}
	act := &Ability{Source: card, Controller: pid}
	if hasNonManaParts(uc) {
		if !g.unlessPayable(pid, card, uc) || !g.payUnlessParts(controller, act, pid, uc) {
			return false
		}
	}
	if uc.hasMana {
		mc := g.faceUpCost(pid, c, w, uc.mana)
		restore := g.payingFor(&payContext{item: act, cost: mc, special: w.key, turnFaceUp: true})
		_, paid := g.payManaCostX(pid, mc, controller)
		restore()
		if !paid {
			return false
		}
	}
	g.turnUp(controller, card)
	if w.megamorph && g.Card(card).Zone == Battlefield {
		// The Megamorph replacement (CardFactoryUtil.java:2363-2376) runs
		// with Card.turnFaceUp's other TurnFaceUp replacements, before its
		// trigger; turnUp ran those, the counter is added here.
		g.addCardCounters(controller, pid, card, card, P1P1, 1)
	}
	g.checkTurnedFaceUpTriggers(controller, card, act)
	return true
}

// turnUp is Card.turnFaceUp's state change (Card.java:878-922): the card's
// own characteristics, a new timestamp (CR 613.7f), turned face up this turn,
// then the Event$ TurnFaceUp replacements. The trigger is the caller's.
func (g *Game) turnUp(controller PlayerController, card CardID) {
	c := g.Card(card)
	c.turnFaceUp()
	g.timestamp++
	c.Timestamp = g.timestamp
	c.turnedFaceUpTurn = g.turn
	g.faceChangeReplaced(controller, "TurnFaceUp", card)
}

// faceUpCostParams are the params a ReduceCost or RaiseCost line naming
// Static.isTurnFaceUp or Static.<way> may carry that faceUpCost reads.
var faceUpCostParams = map[string]bool{
	"validspell": true, "validcard": true, "amount": true, "activator": true, "affectedzone": true, "type": true, "cost": true,
}

// faceUpCost is CostAdjustment.adjust over a turn-face-up ability: each
// live RaiseCost and ReduceCost line whose ValidSpell$ names
// Static.isTurnFaceUp or the way's own flag (Exiled Doomsayer's
// Static.MorphUp), with ValidCard$ and Activator$ matching, adds or takes off
// its numeric Amount$ in generic mana; then a Disguise's own ReduceCost$
// (CostAdjustment.java:214-224).
func (g *Game) faceUpCost(pid PlayerID, c *Card, w faceUpWay, mc mana.Cost) mana.Cost {
	generic := mc.Generic()
	shards := append([]mana.Shard(nil), mc.Shards()...)
	reduce := 0
	for _, mode := range [...]string{"RaiseCost", "ReduceCost"} {
		g.anyStatic(mode, faceUpCostParams, func(h *Card, s *compile.Ability) bool {
			spec, _ := s.Param("ValidSpell")
			named := false
			for _, alt := range strings.Split(spec, ",") {
				if alt == "Static.isTurnFaceUp" || alt == "Static."+w.key {
					named = true
				}
			}
			if !named {
				return false
			}
			if v, ok := s.Param("Activator"); ok {
				if matched, known := matchesPlayerSpec(g, pid, h.Controller(), h.ID, v); !known || !matched {
					return false
				}
			}
			if v, ok := s.Param("ValidCard"); ok && !Matches(g, c, valid.Parse(v), h.Controller(), h.ID) {
				return false
			}
			raw, _ := s.Param("Amount")
			n, err := strconv.Atoi(raw)
			if err != nil {
				return false
			}
			if mode == "RaiseCost" {
				text, ok := s.Param("Cost")
				if !ok {
					text = "1"
				}
				add, err := mana.Parse(text)
				if err != nil {
					return false
				}
				for range n {
					generic += add.Generic()
					shards = append(shards, add.Shards()...)
				}
				return false
			}
			reduce += n
			return false
		})
	}
	if w.reduce != "" && c.faceUpDef != nil {
		if n, ok := resolveNamedAmount(g, c.faceUpDef.Faces[0].Amounts, c, w.reduce); ok {
			reduce += n
		}
	}
	return mana.FromShards(shards, max(generic-reduce, 0))
}
