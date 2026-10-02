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
	// Amounts are SVars that are values (Count$ ...), not abilities: the
	// compiler parses them into the face's Amounts, where a param such as
	// Amount$ KWAffinity{n} finds them.
	Amounts []SVarDef
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
		if len(args) < 2 || args[0] == "" || args[1] == "" {
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
	case "Affinity":
		// Affinity:<type>, CardFactoryUtil.java:3753: this spell costs {1} less
		// for each <type> you control.
		args := k.Args()
		if len(args) < 1 || args[0] == "" {
			return Expansion{}, false
		}
		sep := "."
		if strings.Contains(args[0], ".") {
			sep = "+"
		}
		return Expansion{
			Statics: []string{"Mode$ ReduceCost | ValidCard$ Card.Self | Type$ Spell | Amount$ KWAffinity" + Slot + " | EffectZone$ All"},
			Amounts: []SVarDef{{"KWAffinity" + Slot, "Count$Valid " + args[0] + sep + "YouCtrl"}},
		}, true
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
	case "Battle cry":
		// CardFactoryUtil.java:677.
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ Attacks | ValidCard$ Card.Self | TriggerZones$ Battlefield | Secondary$ True | Execute$ KWBattleCry" + Slot},
			SVars:    []SVarDef{{"KWBattleCry" + Slot, "DB$ PumpAll | ValidCards$ Creature.attacking+Other | NumAtt$ 1"}},
		}, true
	case "Dethrone":
		// CardFactoryUtil.java:934.
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ Attacks | ValidCard$ Card.Self | Attacked$ Player.withMostLife | Secondary$ True | TriggerZones$ Battlefield | Execute$ KWDethrone" + Slot},
			SVars:    []SVarDef{{"KWDethrone" + Slot, "DB$ PutCounter | Defined$ Self | CounterType$ P1P1 | CounterNum$ 1"}},
		}, true
	case "Flanking":
		// CardFactoryUtil.java:1096.
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ AttackerBlockedByCreature | ValidCard$ Card.Self | ValidBlocker$ Creature.withoutFlanking | TriggerZones$ Battlefield | Secondary$ True | Execute$ KWFlanking" + Slot},
			SVars:    []SVarDef{{"KWFlanking" + Slot, "DB$ Pump | Defined$ TriggeredBlockerLKICopy | NumAtt$ -1 | NumDef$ -1"}},
		}, true
	case "etbCounter":
		// CardFactoryUtil.makeEtbCounter (545): etbCounter:<type>:<amount>
		// [:<extra replacement params>|no Condition[:<description>]]. The
		// description is display only. An EACH type is a different
		// ability (CounterTypes$) this port does not translate.
		args := k.Args()
		if len(args) < 2 || args[0] == "" || args[1] == "" || strings.HasPrefix(args[0], "EACH") {
			return Expansion{}, false
		}
		line := "Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield | Secondary$ True | ReplacementResult$ Updated | ReplaceWith$ KWEtbCounter" + Slot
		if len(args) > 2 && args[2] != "" && args[2] != "no Condition" {
			line += " | " + args[2]
		}
		return Expansion{
			Replacements: []string{line},
			SVars:        []SVarDef{{"KWEtbCounter" + Slot, "DB$ PutCounter | Defined$ Self | CounterType$ " + args[0] + " | ETB$ True | CounterNum$ " + args[1]}},
		}, true
	case "Evolve":
		// CardFactoryUtil.java:987. Trigger.java:377 reads the keyword to
		// check CR 702.100c; the script spelling of that check is Condition$
		// Evolve, which the two cards that script it by hand already use.
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ ChangesZone | Destination$ Battlefield | ValidCard$ Creature.YouCtrl+Other | Condition$ Evolve | TriggerZones$ Battlefield | Secondary$ True | Execute$ KWEvolve" + Slot},
			SVars:    []SVarDef{{"KWEvolve" + Slot, "DB$ PutCounter | Defined$ Self | CounterType$ P1P1 | CounterNum$ 1"}},
		}, true
	case "Mentor":
		// CardFactoryUtil.java:1434: put a +1/+1 counter on target attacking
		// creature with lesser power. Java scopes SVar X to the ability; here
		// it is a slotted name, so the card's own X is never shadowed.
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ Attacks | ValidCard$ Card.Self | TriggerZones$ Battlefield | Secondary$ True | Execute$ KWMentor" + Slot},
			SVars:    []SVarDef{{"KWMentor" + Slot, "DB$ PutCounter | CounterType$ P1P1 | CounterNum$ 1 | ValidTgts$ Creature.attacking+powerLTKWPower" + Slot}},
			Amounts:  []SVarDef{{"KWPower" + Slot, "Count$CardPower"}},
		}, true
	case "Training":
		// CardFactoryUtil.java:1874: when this attacks with another creature
		// with greater power, put a +1/+1 counter on it. The ability's own
		// Training$ param is read by nothing in Forge and is dropped.
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ Attacks | ValidCard$ Card.Self | Secondary$ True | IsPresent$ Creature.attacking+Other+powerGTKWPower" + Slot + " | NoResolvingCheck$ True | Execute$ KWTraining" + Slot},
			SVars:    []SVarDef{{"KWTraining" + Slot, "DB$ PutCounter | CounterType$ P1P1 | CounterNum$ 1 | Defined$ Self"}},
			Amounts:  []SVarDef{{"KWPower" + Slot, "Count$CardPower"}},
		}, true
	case "Afflict":
		// CardFactoryUtil.java:588.
		n, ok := amountDetail(k)
		if !ok {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ AttackerBlocked | ValidCard$ Card.Self | TriggerZones$ Battlefield | ValidBlocker$ Creature | Secondary$ True | Execute$ KWAfflict" + Slot},
			SVars:    []SVarDef{{"KWAfflict" + Slot, "DB$ LoseLife | Defined$ TriggeredDefendingPlayer | LifeAmount$ " + n}},
		}, true
	case "Soulshift":
		// CardFactoryUtil.java:1763: when this dies, you may return target
		// Spirit card with mana value N or less from your graveyard to hand.
		n, ok := amountDetail(k)
		if !ok {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ ChangesZone | Origin$ Battlefield | Destination$ Graveyard | Secondary$ True | OptionalDecider$ You | ValidCard$ Card.Self | Execute$ KWSoulshift" + Slot},
			SVars:    []SVarDef{{"KWSoulshift" + Slot, "DB$ ChangeZone | Origin$ Graveyard | Destination$ Hand | ValidTgts$ Spirit.YouOwn+cmcLE" + n}},
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
