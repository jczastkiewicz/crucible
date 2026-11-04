package keyword_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/keyword"
)

func TestExpand(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		line     string
		ok       bool
		contains string // a fragment of the first expanded line
	}{
		{"Equip:1", true, "AB$ Attach | Cost$ 1 | ValidTgts$ Creature.YouCtrl | TgtPrompt$ Select target creature you control | SorcerySpeed$ True"},
		{"Equip:3:Creature.Legendary+YouCtrl:legendary creature", true, "ValidTgts$ Creature.Legendary+YouCtrl | TgtPrompt$ Select target legendary creature you control"},
		{"Equip:0:::ActivationLimit$ 1:Activate only once each turn", true, "| ActivationLimit$ 1"},
		{"Equip:2:Flavor Spirit of the Whalaqee", true, "Cost$ 2 |"},
		{"Equip:3:::ReduceCost$ X:This ability costs {X} less", true, "SorcerySpeed$ True | ReduceCost$ X"},
		{"Craft:1 U ExileCtrlOrGrave<1/Artifact.Other>", true, "AB$ ChangeZone | Cost$ Exile<1/CARDNAME> 1 U ExileCtrlOrGrave<1/Artifact.Other> | Origin$ Exile | Destination$ Battlefield | Transformed$ True | Defined$ CorrectedSelf | SorcerySpeed$ True"},
		{"Craft:6 ExileCtrlOrGrave<2/Permanent.Other+withSharedCardType/permanent>:two that share a card type:the two", true, "Cost$ Exile<1/CARDNAME> 6 ExileCtrlOrGrave<2/Permanent.Other+withSharedCardType/permanent> |"},
		{"Craft", false, ""},
		{"Equip:3:::AlternateCost$ Discard<1/Card>", true, "Cost$ 3 | ValidTgts$ Creature.YouCtrl | TgtPrompt$ Select target creature you control | SorcerySpeed$ True | AlternateCost$ Discard<1/Card>"},
		{"Equip", false, ""},
		{"Cycling:2", true, "AB$ Draw | Cost$ 2 Discard<1/CARDNAME> | ActivationZone$ Hand"},
		{"Cycling", false, ""},
		{"Station:6", true, "AB$ PutCounter | Cost$ tapXType<1/Creature.Other> | Defined$ Self | CounterType$ CHARGE | CounterNum$ StationX{n} | SorcerySpeed$ True"},
		{"Embalm:3 W", true, "AB$ CopyPermanent | Cost$ 3 W ExileFromGrave<1/CARDNAME> | ActivationZone$ Graveyard | SorcerySpeed$ True | RemoveCost$ True | AddTypes$ Zombie | Defined$ Self | SetColor$ White"},
		{"Eternalize:2 B B", true, "SetColor$ Black | SetPower$ 4 | SetToughness$ 4"},
		{"Embalm", false, ""},
		{"Unearth:B", true, "AB$ ChangeZone | Cost$ B | Defined$ Self | Origin$ Graveyard | Destination$ Battlefield | SorcerySpeed$ True | ActivationZone$ Graveyard"},
		{"Unearth", false, ""},
		{"Encore:4 B", true, "AB$ CopyPermanent | Cost$ 4 B ExileFromGrave<1/CARDNAME> | ActivationZone$ Graveyard"},
		{"Encore", false, ""},
		{"Encore:4 B:AILogic$ X", false, ""},
		{"Vanishing:3", true, "Mode$ Phase | Phase$ Upkeep | ValidPlayer$ You | TriggerZones$ Battlefield | IsPresent$ Card.Self+counters_GE1_TIME"},
		{"Vanishing", true, "Mode$ Phase | Phase$ Upkeep"},
		{"Vanishing:X", false, ""},
		{"Living Weapon", true, "Mode$ ChangesZone | Destination$ Battlefield | ValidCard$ Card.Self"},
		{"Living Weapon:1", false, ""},
		{"Myriad", true, "Mode$ Attacks | ValidCard$ Card.Self | Secondary$ True"},
		{"Myriad:1", false, ""},
		{"Strive:2 R", true, "Mode$ RaiseCost | ValidCard$ Card.Self | Type$ Spell | Amount$ Strive | Cost$ 2 R | EffectZone$ All"},
		{"Strive", false, ""},
		{"Strive:2 R:extra", false, ""},
		{"Firebending:2", true, "Mode$ Attacks | ValidCard$ Card.Self"},
		{"Firebending:X:, where X is CARDNAME's power.", true, "Mode$ Attacks"},
		{"Firebending:Y", false, ""},
		{"Firebending", false, ""},
		{"Cipher", true, "Mode$ Continuous | EffectZone$ Exile | Affected$ Card.EncodedWithSource"},
		{"Cipher:1", false, ""},
		{"Hideaway:4", true, "Mode$ ChangesZone | Destination$ Battlefield | ValidCard$ Card.Self"},
		{"Hideaway", false, ""},
		{"Double team", true, "Mode$ Attacks | ValidCard$ Card.Self+!token"},
		{"Double team:1", false, ""},
		{"Reconfigure:2", true, "AB$ Attach | ValidTgts$ Creature.YouCtrl+Other"},
		{"Reconfigure:2:PayEnergy<3>", true, "Cost$ 2 | SorcerySpeed$ True | Secondary$ True | AlternateCost$ PayEnergy<3>"},
		{"Reconfigure:2:PayEnergy<3>:extra", false, ""},
		{"Mobilize:2", true, "Mode$ Attacks | ValidCard$ Card.Self | Execute$ KWMobilize{n}"},
		{"Mobilize:X", true, "Mode$ Attacks"},
		{"Mobilize:Y", false, ""},
		{"For Mirrodin", true, "Mode$ ChangesZone | Destination$ Battlefield | ValidCard$ Card.Self"},
		{"For Mirrodin:1", false, ""},
		{"Living metal", true, "Mode$ Continuous | Affected$ Card.Self | AddType$ Creature | Condition$ PlayerTurn"},
		{"Living metal:1", false, ""},
		{"Fading:3", true, "Mode$ Phase | Phase$ Upkeep | ValidPlayer$ You"},
		{"Fading", false, ""},
		{"Rampage:2", true, "Mode$ AttackerBlocked | ValidCard$ Card.Self"},
		{"Rampage", false, ""},
		{"Graft:2", true, "IsPresent$ Card.Self+counters_GE1_P1P1"},
		{"Graft", false, ""},
		{"Unleash", true, "Mode$ CantBlock | ValidCard$ Creature.Self+counters_GE1_P1P1"},
		{"Unleash:1", false, ""},
		{"Daybound", true, "Mode$ Always | TriggerZones$ Battlefield | Static$ True | DayTime$ Neither | Secondary$ True | Execute$ KWDayBoundSet{n}"},
		{"Nightbound", true, "DayTime$ Neither | IsPresent$ Card.Daybound | PresentCompare$ EQ0"},
		{"Specialize:2", true, "Mode$ Specialize | SorcerySpeed$ True | ActivationZone$ Hand"},
		{"Specialize:2:Flavor:Condition:ActivationLimit$ 1", true, "| ActivationLimit$ 1"},
		{"Specialize", false, ""},
		{"Champion:Goblin,Shaman", true, "Mode$ ChangesZone | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ KWChampion"},
		{"Champion", false, ""},
		{"Enlist", true, "Mode$ OptionalAttackCost | ValidCard$ Card.Self | Cost$ Enlist<1/CARDNAME/creature>"},
		{"Enlist:1", false, ""},
		{"Prowess", true, "Mode$ SpellCast | ValidCard$ Card.nonCreature"},
		{"Exalted", true, "Mode$ Attacks | ValidCard$ Creature.YouCtrl | Alone$ True"},
		{"Prowess:1", false, ""},
		{"Annihilator:2", true, "Mode$ Attacks | ValidCard$ Card.Self"},
		{"Annihilator", false, ""},
		{"Bushido:1", true, "Mode$ Blocks | ValidCard$ Card.Self"},
		{"Bushido:x", false, ""},
		{"Afterlife:2", true, "Mode$ ChangesZone | Origin$ Battlefield | Destination$ Graveyard"},
		{"TypeCycling:Plains:2", true, "Cost$ 2 Discard<1/CARDNAME> | ActivationZone$ Hand | Origin$ Library | Destination$ Hand | ChangeType$ Plains"},
		{"TypeCycling:Basic", false, ""},
		{"Crew:2", true, "AB$ Animate | Cost$ tapXType<Any/Creature.Other+withTotalPowerGE2> | Defined$ Self | Types$ Artifact,Creature"},
		{"Crew", false, ""},
		{"Affinity:Artifact", true, ""},
		{"Affinity", false, ""},
		{"Persist", true, "counters_EQ0_M1M1"},
		{"Undying", true, "counters_EQ0_P1P1"},
		{"Undying:1", false, ""},
		{"Battle cry", true, "Mode$ Attacks | ValidCard$ Card.Self"},
		{"Battle cry:1", false, ""},
		{"Dethrone", true, "Attacked$ Player.withMostLife"},
		{"Flanking", true, "ValidBlocker$ Creature.withoutFlanking"},
		{"Afflict:2", true, "Mode$ AttackerBlocked | ValidCard$ Card.Self"},
		{"Afflict", false, ""},
		{"Soulshift:3", true, "OptionalDecider$ You"},
		{"Soulbond", true, "IsPresent$ Creature.Other+YouCtrl+!Paired"},
		{"Soulbond:1", false, ""},
		{"Soulshift:x", false, ""},
		{"Mentor", true, "Mode$ Attacks | ValidCard$ Card.Self"},
		{"Training", true, "IsPresent$ Creature.attacking+Other+powerGTKWPower"},
		{"Evolve", true, "Condition$ Evolve"},
		{"Chapter:3:A,B,C", true, "Chapter$ 1 | CounterType$ LORE | CounterAmount$ EQ1 | Execute$ A"},
		{"Chapter:3:A,B", false, ""},
		{"Chapter:x:A", false, ""},
		{"Chapter:2:A,", false, ""},
		{"Chapter", false, ""},
		{"Modular:1", true, "OptionalDecider$ You"},
		{"Modular:Sunburst", true, "OptionalDecider$ You"},
		{"Modular", false, ""},
		{"Renown:2", true, "IsPresent$ Card.Self+!IsRenowned"},
		{"Renown", false, ""},
		{"Dethrone:1", false, ""},
		{"Flanking:1", false, ""},
		{"Mentor:1", false, ""},
		{"Training:1", false, ""},
		{"Evolve:1", false, ""},
		{"Extort:1", false, ""},
		{"Fabricate:2", true, "Mode$ ChangesZone | Destination$ Battlefield | ValidCard$ Card.Self"},
		{"Fabricate", false, ""},
		{"Extort", true, "Mode$ SpellCast | ValidActivatingPlayer$ You"},
		{"Echo:1 R", true, "IsPresent$ Card.Self+cameUnderControlSinceLastUpkeep"},
		{"Echo", false, ""},
		{"Dash:1 R", true, "Affected$ Card.Self+dashed+castKeyword | AddKeyword$ Haste"},
		{"Dash", false, ""},
		{"Blitz:2 B B PayLife<2>", true, "AddTrigger$ KWBlitzDies"},
		{"Blitz", false, ""},
		{"Blitz:CardManaCost:Spell.Creature+cmcGE4", false, ""},
		{"Cumulative upkeep:PayLife<1>", true, "Mode$ Phase | Phase$ Upkeep | ValidPlayer$ You"},
		{"Cumulative upkeep", false, ""},
		{"Cascade", true, "Mode$ SpellCast | ValidCard$ Card.Self | TriggerZones$ Stack"},
		{"Cascade:1", false, ""},
		{"Storm", true, "Mode$ SpellCast | ValidCard$ Card.Self | TriggerZones$ Stack"},
		{"Storm:1", false, ""},
		{"Exploit", true, "Mode$ ChangesZone | ValidCard$ Card.Self | Destination$ Battlefield"},
		{"Exploit:1", false, ""},
		{"Flying", false, ""},
	} {
		t.Run(tc.line, func(t *testing.T) {
			t.Parallel()

			exp, ok := keyword.Expand(keyword.Parse(tc.line))
			if ok != tc.ok {
				t.Fatalf("Expand ok = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			lines := append(append(append([]string(nil), exp.Abilities...), exp.Triggers...), exp.Statics...)
			if len(lines) == 0 || !strings.Contains(lines[0], tc.contains) {
				t.Errorf("first line %q does not contain %q", lines, tc.contains)
			}
		})
	}
}

// Devour's type and multiplier land in the sacrifice and the counter amount
// (CardFactoryUtil.java:2211-2238).
func TestExpandDevourTypeAndMultiplier(t *testing.T) {
	t.Parallel()

	exp, ok := keyword.Expand(keyword.Parse("Devour:3:Land"))
	if !ok {
		t.Fatal("Devour:3:Land did not expand")
	}
	if len(exp.SVars) != 3 || !strings.Contains(exp.SVars[0].Value, "SacValid$ Land.Other") ||
		!strings.Contains(exp.SVars[0].Value, "Optional$ True") {
		t.Errorf("SVars %v: want a Sacrifice of Land.Other, optional", exp.SVars)
	}
	want := map[string]string{
		"KWDevourSacX{n}": "Count$Valid Land.YouCtrl+Other",
		"KWDevourX{n}":    "Count$RememberedSize/Times.3",
	}
	for _, a := range exp.Amounts {
		if want[a.Name] != a.Value {
			t.Errorf("amount %s = %q, want %q", a.Name, a.Value, want[a.Name])
		}
	}
	if len(exp.Amounts) != len(want) {
		t.Errorf("amounts %v, want %v", exp.Amounts, want)
	}
}

func TestExpandReplacements(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		line string
		ok   bool
		want string // the replacement line
	}{
		{"etbCounter:P1P1:2", true, "Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield | Secondary$ True | ReplacementResult$ Updated | ReplaceWith$ KWEtbCounter{n}"},
		{"etbCounter:P1P1:X:no Condition:CARDNAME enters with X counters.", true, "ReplaceWith$ KWEtbCounter{n}"},
		{"etbCounter:P1P1:X:CheckSVar$ WasKicked:desc", true, "ReplaceWith$ KWEtbCounter{n} | CheckSVar$ WasKicked"},
		{"etbCounter:EACH :1", false, ""},
		{"Bloodthirst:2", true, "Bloodthirst$ True"},
		{"Bloodthirst:X", true, "ReplaceWith$ KWBloodthirst{n} | Bloodthirst$ True"},
		{"Bloodthirst:y", false, ""},
		{"Bloodthirst", false, ""},
		{"etbCounter:P1P1", false, ""},
		{"Devour:2", true, "Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield | ReplacementResult$ Updated | ReplaceWith$ KWDevour{n}"},
		{"Devour:3:Land", true, "ReplaceWith$ KWDevour{n}"},
		{"Devour:X::, where X is the number of creatures devoured this way", true, "ReplaceWith$ KWDevour{n}"},
		{"Devour:y", false, ""},
		{"Devour", false, ""},
		{"Madness:X R", true, "Event$ Moved | ActiveZones$ Hand | ValidCard$ Card.Self | Discard$ True | Secondary$ True | ReplaceWith$ KWMadnessExile{n}"},
		{"Madness", false, ""},
		{"Modular:Sunburst", true, "ReplaceWith$ KWModularEtb{n}"},
		{"Riot", true, "ReplaceWith$ KWRiot{n}"},
		{"Riot:1", false, ""},
		{"Dredge:2", true, "Event$ Draw | ActiveZones$ Graveyard | ValidPlayer$ You | Secondary$ True | Optional$ True | CheckSVar$ KWDredgeCheckLib{n} | SVarCompare$ GE2"},
		{"Dredge", false, ""},
		{"Umbra armor", true, "Event$ Destroy | ActiveZones$ Battlefield | ValidCard$ Card.EnchantedBy"},
		{"Umbra armor:1", false, ""},
		{"Rebound", true, "Event$ Moved | ValidLKI$ Card.Self+wasCastFromHand+YouOwn+YouCtrl | Origin$ Stack | Destination$ Graveyard | Fizzle$ False | ReplaceWith$ KWReboundExile{n}"},
	} {
		t.Run(tc.line, func(t *testing.T) {
			t.Parallel()

			exp, ok := keyword.Expand(keyword.Parse(tc.line))
			if ok != tc.ok {
				t.Fatalf("Expand ok = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if len(exp.Replacements) != 1 || !strings.Contains(exp.Replacements[0], tc.want) {
				t.Errorf("replacements %q do not contain %q", exp.Replacements, tc.want)
			}
		})
	}
}

// Sunburst's counter follows the face's printed type (CardFactoryUtil.java:
// 2480): +1/+1 for a creature, charge otherwise.
// Suspend's play trigger gives a permanent card haste through a Remembered
// pump (CardFactoryUtil.java:1856-1866); an instant or sorcery is only cast.
func TestExpandSuspendHasteForPermanents(t *testing.T) {
	t.Parallel()
	for _, permanent := range []bool{true, false} {
		exp, ok := keyword.ExpandSuspend(keyword.Parse("Suspend:1:R"), permanent)
		if !ok || len(exp.Triggers) != 2 {
			t.Fatalf("permanent %v: ok %v, %d triggers, want 2", permanent, ok, len(exp.Triggers))
		}
		haste := false
		for _, sv := range exp.SVars {
			haste = haste || strings.Contains(sv.Value, "KW$ Haste")
		}
		if haste != permanent {
			t.Errorf("permanent %v: haste pump %v", permanent, haste)
		}
	}
	if _, ok := keyword.ExpandSuspend(keyword.Parse("Flying"), true); ok {
		t.Error("ExpandSuspend accepted Flying")
	}
	if exp, _ := keyword.Expand(keyword.Parse("Suspend:1:R")); len(exp.Triggers) != 0 {
		t.Error("Expand handles Suspend without the face's type")
	}
}

func TestExpandSunburstCounterByType(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		line     string
		creature bool
		ok       bool
		counter  string
	}{
		{"Sunburst", true, true, "CounterType$ P1P1"},
		{"Sunburst", false, true, "CounterType$ CHARGE"},
		{"Sunburst:1", true, false, ""},
		{"Flying", true, false, ""},
	} {
		exp, ok := keyword.ExpandSunburst(keyword.Parse(tc.line), tc.creature)
		if ok != tc.ok {
			t.Fatalf("%s creature %v: ok %v, want %v", tc.line, tc.creature, ok, tc.ok)
		}
		if ok && !strings.Contains(exp.SVars[0].Value, tc.counter) {
			t.Errorf("%s creature %v: %q lacks %q", tc.line, tc.creature, exp.SVars[0].Value, tc.counter)
		}
	}
	if exp, _ := keyword.Expand(keyword.Parse("Sunburst")); len(exp.Replacements) != 0 {
		t.Error("Expand handles Sunburst without the face's type")
	}
}
