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
	case "TypeCycling":
		// TypeCycling:<type>:<cost>, CardFactoryUtil.java:3733.
		args := k.Args()
		if len(args) != 2 || args[0] == "" || args[1] == "" {
			return Expansion{}, false
		}
		return Expansion{Abilities: []string{"AB$ ChangeZone | Cost$ " + args[1] + " Discard<1/CARDNAME> | ActivationZone$ Hand | Origin$ Library | Destination$ Hand | ChangeType$ " + args[0]}}, true
	case "Crew":
		// Crew:<power>[:<extra>], CardFactoryUtil.java:3701: tap any number of
		// other creatures with total power N or more to make it an artifact
		// creature until end of turn.
		args := k.Args()
		if len(args) < 1 || strings.Trim(args[0], "0123456789") != "" || args[0] == "" {
			return Expansion{}, false
		}
		line := "AB$ Animate | Cost$ tapXType<Any/Creature.Other+withTotalPowerGE" + args[0] + "> | Defined$ Self | Types$ Artifact,Creature"
		if len(args) > 1 && args[1] != "" {
			line += " | " + args[1]
		}
		return Expansion{Abilities: []string{line}}, true
	case "Prowess":
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ SpellCast | ValidCard$ Card.nonCreature | ValidActivatingPlayer$ You | TriggerZones$ Battlefield | Execute$ KWProwess" + Slot},
			SVars:    []SVarDef{{"KWProwess" + Slot, "DB$ Pump | Defined$ Self | NumAtt$ +1 | NumDef$ +1"}},
		}, true
	case "Annihilator":
		n, ok := amountDetail(k)
		if !ok {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ Attacks | ValidCard$ Card.Self | TriggerZones$ Battlefield | Secondary$ True | Execute$ KWAnnihilator" + Slot},
			SVars:    []SVarDef{{"KWAnnihilator" + Slot, "DB$ Sacrifice | Defined$ TriggeredDefendingPlayer | SacValid$ Permanent | Amount$ " + n}},
		}, true
	case "Bushido":
		n, ok := amountDetail(k)
		if !ok {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{
				"Mode$ Blocks | ValidCard$ Card.Self | Secondary$ True | Execute$ KWBushido" + Slot,
				"Mode$ AttackerBlocked | ValidCard$ Card.Self | Secondary$ True | Execute$ KWBushido" + Slot,
			},
			SVars: []SVarDef{{"KWBushido" + Slot, "DB$ Pump | Defined$ Self | NumAtt$ " + n + " | NumDef$ " + n}},
		}, true
	case "Afterlife":
		n, ok := amountDetail(k)
		if !ok {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ ChangesZone | Origin$ Battlefield | Destination$ Graveyard | ValidCard$ Card.Self | Secondary$ True | Execute$ KWAfterlife" + Slot},
			SVars:    []SVarDef{{"KWAfterlife" + Slot, "DB$ Token | TokenAmount$ " + n + " | TokenScript$ wb_1_1_spirit_flying"}},
		}, true
	case "Persist", "Undying":
		if k.Details != "" {
			return Expansion{}, false
		}
		counter := "M1M1"
		if k.Name == "Undying" {
			counter = "P1P1"
		}
		return Expansion{
			Triggers: []string{"Mode$ ChangesZone | Origin$ Battlefield | Destination$ Graveyard | ValidCard$ Card.Self+counters_EQ0_" + counter + " | TriggerZones$ Battlefield | Secondary$ True | Execute$ KW" + k.Name + Slot},
			SVars:    []SVarDef{{"KW" + k.Name + Slot, "DB$ ChangeZone | Defined$ TriggeredNewCardLKICopy | Origin$ Graveyard | Destination$ Battlefield | WithCountersType$ " + counter}},
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

// amountDetail is the first detail of an Amount-shaped keyword ("Bushido:2"),
// false when it is missing or not a plain number.
func amountDetail(k Keyword) (string, bool) {
	args := k.Args()
	if len(args) < 1 || args[0] == "" || strings.Trim(args[0], "0123456789") != "" {
		return "", false
	}
	return args[0], true
}
