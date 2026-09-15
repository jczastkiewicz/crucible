// Cards. Identity and location here; the mutable detail is split out by
// concern (ADR-0009).

package engine

import (
	"sort"
	"strconv"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/keyword"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/pkg/collect"
)

// Card is one card in one game.
//
// It holds no back-reference: no *Game, no *Player. Its controller is a
// PlayerID and its attachments are CardIDs, so a game copies with a slice copy
// and nothing has to be remapped (ADR-0009). Every operation that needs the
// rest of the game takes *Game as a parameter.
type Card struct {
	// ID is this card's own handle, so a Card passed by value still knows what
	// it is.
	ID CardID
	// Def is the compiled script, shared and immutable across every game in
	// the process (ADR-0007). Never nil for a real card.
	Def *compile.Card
	// Owner never changes. Controller does, and the two differ whenever
	// something has taken control of the card.
	Owner      PlayerID
	Controller PlayerID
	// Zone is where the card is. The zone's own list is the ordering
	// authority; this is the reverse index, kept in step by the move
	// operations.
	Zone      ZoneType
	ZoneOwner PlayerID
	// Timestamp orders continuous effects and is what breaks ties between
	// otherwise simultaneous events. Assigned from the game's counter on every
	// zone change, never reused.
	Timestamp uint64

	// The mutable detail, split by concern rather than flattened onto Card:
	// 8,105 lines of Java's Card has to land somewhere, and these are the
	// parts with their own invariants (ADR-0009).
	Counters Counters
	Damage   Damage
	Memory   Memory
	PT       PT

	// Tapped and SummonSick are the two pieces of battlefield state every
	// permanent carries that are not "how much of something" -- everything
	// else that shape (Renowned, Monstrous, PhasedOut, and the rest of
	// GameState's per-card annotation grammar) waits on the mechanic that
	// reads it, which is card-type-specific and not built yet
	// (porting/port-log/game-state-fixture.md).
	Tapped     bool
	SummonSick bool

	// ProtectingPlayer is CR 122.1/704.5w's protector: the opponent
	// defending a Battle. NoPlayer for anything that is not a Battle, or a
	// Battle that has not been assigned one yet (assignBattleProtector,
	// action.go).
	ProtectingPlayer PlayerID

	// attachedTo is the card this one is attached to, and attachments is the
	// reverse. Both are unexported because they are two representations of one
	// fact and only Game.Attach and Game.Unattach may write either.
	attachedTo  CardID
	attachments *collect.OrderedSet[CardID]
}

// Type is the card's printed type line: its own primary face's, since
// CardState -- which face is current for a transformed, flipped or melded
// card -- is not modeled yet (game-state.md's "Not ported yet"), so every
// card reports the one it entered the game with. A synthetic card built
// with a nil Def (most engine tests) reports the zero Line, which matches
// nothing -- consistent with "no Def" already meaning "no name" elsewhere.
func (c *Card) Type() cardtype.Line {
	if c.Def == nil {
		return cardtype.Line{}
	}
	return c.Def.Faces[0].Type
}

// HasKeyword reports whether the card's primary face carries the named
// keyword, exact match against keyword.Parse's own Name (the head as
// written -- "Indestructible" for a bare line, "Ward" for "Ward:2"), so a
// keyword written with arguments is still found by its bare name. Every
// keyword this grants from a script that has not been read is invisible
// here the same way BasePower is blind to "*": a card whose keywords a
// continuous effect currently changes reports the printed set, not the
// one in play (game-state.md's "Not ported yet").
func (c *Card) HasKeyword(name string) bool {
	if c.Def == nil {
		return false
	}
	for _, line := range c.Def.Faces[0].Keywords {
		if keyword.Parse(line).Name == name {
			return true
		}
	}
	return false
}

// BasePower and BaseToughness are the card's printed power and toughness --
// CR 613's Layer 0, before anything in Layer 7 or a counter has applied.
// Java calls these getBasePower/getBaseToughness for the same reason:
// "base" is a named concept in the rules, distinct from "current"
// (getNetPower) -- Power/Toughness, below, is this port's getNetPower.
//
// ok is false for anything that is not a plain integer: "*", "1+*", a
// Count$ reference, or a card with no printed toughness at all (an
// instant, a nil Def). Resolving those needs `internal/expr` and a game,
// neither of which this reaches yet -- a coverage gap, not a wrong answer,
// the same category CheckStateBasedActions's own gaps are in.
func (c *Card) BasePower() (int, bool) {
	if c.Def == nil {
		return 0, false
	}
	n, err := strconv.Atoi(c.Def.Faces[0].Power)
	return n, err == nil
}

// BaseToughness is BasePower's counterpart; see its doc comment.
func (c *Card) BaseToughness() (int, bool) {
	if c.Def == nil {
		return 0, false
	}
	n, err := strconv.Atoi(c.Def.Faces[0].Toughness)
	return n, err == nil
}

// BaseLoyalty is a planeswalker's printed starting loyalty -- CR 121.5's
// own words, since unlike power and toughness a planeswalker's loyalty is
// not something Layer 7 recomputes on every check. It exists once, the
// moment the permanent enters the battlefield, as that many loyalty
// counters (Loyalty, counters.go); everything after that -- gaining,
// losing, paying loyalty costs -- is ordinary counter addition and
// removal, which Card.Counters already handles. ok is false on the same
// terms as BasePower/BaseToughness: anything past a plain printed integer,
// or a nil Def.
func (c *Card) BaseLoyalty() (int, bool) {
	if c.Def == nil {
		return 0, false
	}
	n, err := strconv.Atoi(c.Def.Faces[0].Loyalty)
	return n, err == nil
}

// BaseDefense is BaseLoyalty's counterpart for a Battle: its printed
// starting defense, entered as that many Defense counters (counters.go)
// the same way a planeswalker's loyalty is, not a Layer 7 value.
func (c *Card) BaseDefense() (int, bool) {
	if c.Def == nil {
		return 0, false
	}
	n, err := strconv.Atoi(c.Def.Faces[0].Defense)
	return n, err == nil
}

// Colors is the card's color identity for rules purposes (CR 105, valid.go's
// White/Blue/Black/Red/Green/Colorless/MultiColor properties): a script's
// explicit `Colors:` override, or (absent one) whatever its mana cost's own
// colored symbols say -- the exact "override, else derive" logic
// carddb.Face.dumpColors already carries out and M2's P1 gate already
// verifies byte-identical to Forge's own dump, not a new derivation. A
// color-changing effect (Layer 5) is not folded in, the same "printed
// value only" limit every other characteristic on this type has until the
// rest of the continuous-effect layer system lands (game-state.md's "Not
// ported yet"). A nil Def reports the zero value, mana.Colors' own
// "colorless" -- consistent with every other Def-derived accessor here.
func (c *Card) Colors() mana.Colors {
	if c.Def == nil {
		return 0
	}
	f := c.Def.Faces[0]
	if f.HasColors {
		return f.Colors
	}
	return f.ManaCost.Colors()
}

// Power and Toughness are the card's current power and toughness: Layer 0
// (BasePower/BaseToughness) with Layer 7's continuous effects (PT) folded
// in, plus +1/+1 and -1/-1 counters, in CR 613.4's own order -- counters
// apply after every layer, not as one themselves.
//
// ok is false wherever BasePower/BaseToughness's own ok is, unless a
// LayerCharacteristic effect supplies a value of its own: a
// characteristic-defining ability's whole point is replacing an
// unresolvable printed value ("*") with a computed one, so PT can turn an
// unresolvable base into a resolvable current value, never the reverse.
func (c *Card) Power() (int, bool) {
	base, ok := c.BasePower()
	v, ok := foldPT(base, ok, c.PT.effects, func(e PTEffect) int { return e.Power })
	if !ok {
		return 0, false
	}
	return v + c.Counters.Count(P1P1) - c.Counters.Count(M1M1), true
}

// Toughness is Power's counterpart; see its doc comment.
func (c *Card) Toughness() (int, bool) {
	base, ok := c.BaseToughness()
	v, ok := foldPT(base, ok, c.PT.effects, func(e PTEffect) int { return e.Toughness })
	if !ok {
		return 0, false
	}
	return v + c.Counters.Count(P1P1) - c.Counters.Count(M1M1), true
}

// foldPT applies Layer 7's own sub-layers in order (CR 613.4):
// LayerCharacteristic and LayerSetPT each replace the running value,
// LayerModifyPT adds to it. Ties within a layer break by Timestamp,
// ascending -- CR 613.7's own tiebreak once dependency reordering (CR
// 613.8) is not in play, which it cannot be: nothing here has more than
// one continuous effect on the same card yet to depend on another.
func foldPT(base int, baseOK bool, effects []PTEffect, pick func(PTEffect) int) (int, bool) {
	sorted := append([]PTEffect(nil), effects...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Layer != sorted[j].Layer {
			return sorted[i].Layer < sorted[j].Layer
		}
		return sorted[i].Timestamp < sorted[j].Timestamp
	})
	value, ok := base, baseOK
	for _, e := range sorted {
		switch e.Layer {
		case LayerCharacteristic, LayerSetPT:
			value, ok = pick(e), true
		case LayerModifyPT:
			if ok {
				value += pick(e)
			}
		}
	}
	return value, ok
}

// AttachedTo is what this card is attached to, and whether it is attached at
// all. Auras, Equipment and Fortifications all use it.
func (c *Card) AttachedTo() (CardID, bool) {
	if c.attachedTo == NoCard {
		return NoCard, false
	}
	return c.attachedTo, true
}

// Attachments returns what is attached to this card, in the order it was
// attached. Order decides which Aura's continuous effect applies first when
// two share a timestamp.
func (c *Card) Attachments() []CardID {
	if c.attachments == nil {
		return nil
	}
	return c.attachments.All()
}
