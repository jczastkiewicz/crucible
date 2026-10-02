package keyword

import (
	"strings"
)

// Expansion is the script text a keyword stands for: the `A:`, `T:`, `S:` and
// `R:` values CardFactoryUtil builds for it (ADR-0038), and the SVars those
// lines name. Each trigger line names its effect by `Execute$` and the matching
// SVar is in SVars, with the placeholder name {n} replaced by the caller so two
// keywords on one card never share an SVar.
type Expansion struct {
	Abilities, Triggers, Statics, Replacements []string
	SVars                                      []SVarDef
}

// SVarDef is one SVar an Expansion defines.
type SVarDef struct{ Name, Value string }

// Slot is the placeholder an expansion's SVar names carry; the compiler
// replaces it with a number unique to the keyword line on the card.
const Slot = "{n}"

// Expand returns what k stands for, ported from the CardFactoryUtil branches
// ADR-0038 calls expressible, or false when k has no template or its details
// carry a shape this port does not translate (ok false leaves the keyword
// inert, never half-expanded, GO-7). The returned lines are Java's own script
// text minus the display-only params (PrecostDesc$, CostDesc$,
// SpellDescription$, TriggerDescription$).
func Expand(k Keyword) (Expansion, bool) {
	switch k.Name {
	case "Equip":
		return expandEquip(k)
	case "Cycling":
		return expandCycling(k)
	case "Prowess":
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ SpellCast | ValidCard$ Card.nonCreature | ValidActivatingPlayer$ You | TriggerZones$ Battlefield | Execute$ KWProwess" + Slot},
			SVars:    []SVarDef{{"KWProwess" + Slot, "DB$ Pump | Defined$ Self | NumAtt$ +1 | NumDef$ +1"}},
		}, true
	case "Exalted":
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ Attacks | ValidCard$ Creature.YouCtrl | Alone$ True | TriggerZones$ Battlefield | Secondary$ True | Execute$ KWExalted" + Slot},
			SVars:    []SVarDef{{"KWExalted" + Slot, "DB$ Pump | Defined$ TriggeredAttackerLKICopy | NumAtt$ +1 | NumDef$ +1"}},
		}, true
	}
	return Expansion{}, false
}

// expandEquip is CardFactoryUtil.java:2852's Equip branch: "Equip:<cost>
// [:<valid>[:<description>[:<extra>[:<extra description>]]]]", an Attach
// ability at sorcery speed aimed at a creature the controller controls (or
// <valid>). <extra> is appended to the ability as written; the two shapes
// whose params change what the ability costs (ReduceCost$, AlternateCost$) are
// not read by any ability this port resolves, so an Equip carrying one is left
// inert rather than activated at the wrong price.
func expandEquip(k Keyword) (Expansion, bool) {
	args := k.Args()
	if len(args) < 1 || args[0] == "" {
		return Expansion{}, false
	}
	valid := "Creature.YouCtrl"
	if len(args) > 1 && args[1] != "" {
		valid = args[1]
	}
	desc := "creature"
	if len(args) > 2 && args[2] != "" {
		desc = args[2]
	}
	line := "AB$ Attach | Cost$ " + args[0] + " | ValidTgts$ " + valid +
		" | TgtPrompt$ Select target " + desc + " you control | SorcerySpeed$ True"
	if len(args) > 3 && args[3] != "" {
		extra := args[3]
		if strings.Contains(extra, "ReduceCost$") || strings.Contains(extra, "AlternateCost$") {
			return Expansion{}, false
		}
		line += " | " + extra
	}
	return Expansion{Abilities: []string{line}}, true
}

// expandCycling is CardFactoryUtil.java:3717's Cycling branch: pay the cost and
// discard the card from hand to draw a card.
func expandCycling(k Keyword) (Expansion, bool) {
	args := k.Args()
	if len(args) < 1 || args[0] == "" {
		return Expansion{}, false
	}
	return Expansion{Abilities: []string{"AB$ Draw | Cost$ " + args[0] + " Discard<1/CARDNAME> | ActivationZone$ Hand"}}, true
}
