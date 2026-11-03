package keyword

import (
	"strconv"
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
	case "Specialize":
		return expandSpecialize(k)
	case "TypeCycling":
		// TypeCycling:<type>:<cost>, CardFactoryUtil.java:3733.
		args := k.Args()
		if len(args) < 2 || args[0] == "" || args[1] == "" {
			return Expansion{}, false
		}
		return Expansion{Abilities: []string{"AB$ ChangeZone | Cost$ " + args[1] + " Discard<1/CARDNAME> | ActivationZone$ Hand | Origin$ Library | Destination$ Hand | ChangeType$ " + args[0]}}, true
	case "Craft":
		// Craft:<cost>[:<description>:<reminder object>], CardFactoryUtil.java:
		// 2734-2743 (Craft.java:28-66): exile this permanent and the craft
		// materials, then return it transformed at sorcery speed. Defined$
		// CorrectedSelf is the card now in exile (AbilityUtils.java:84-85).
		// The other two fields are display only.
		args := k.Args()
		if len(args) < 1 || args[0] == "" {
			return Expansion{}, false
		}
		return Expansion{Abilities: []string{"AB$ ChangeZone | Cost$ Exile<1/CARDNAME> " + args[0] +
			" | Origin$ Exile | Destination$ Battlefield | Transformed$ True | Defined$ CorrectedSelf | SorcerySpeed$ True"}}, true
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
	case "Saddle":
		// Saddle:<power>, CardFactoryUtil.java:3415-3421: tap any number of other
		// creatures with total power N or more to make it saddled until end of
		// turn, as a sorcery.
		args := k.Args()
		if len(args) < 1 || strings.Trim(args[0], "0123456789") != "" || args[0] == "" {
			return Expansion{}, false
		}
		return Expansion{Abilities: []string{"AB$ AlterAttribute | Cost$ tapXType<Any/Creature.Other+withTotalPowerGE" + args[0] + "> | Attributes$ Saddled | Defined$ Self | SorcerySpeed$ True"}}, true
	case "Station":
		// Station:<N>, CardFactoryUtil.java:3468-3476: tap another creature to
		// put charge counters on this permanent equal to its power, as a
		// sorcery. N is reminder text only; the STATION N+ thresholds are
		// the card's own S: lines.
		return Expansion{
			Abilities: []string{"AB$ PutCounter | Cost$ tapXType<1/Creature.Other> | Defined$ Self | CounterType$ CHARGE | CounterNum$ StationX{n} | SorcerySpeed$ True"},
			Amounts:   []SVarDef{{"StationX" + Slot, "TappedCards$TapPowerValue"}},
		}, true
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
	case "Daybound", "Nightbound":
		// CardFactoryUtil.java:879-895 and :1522-1533: two state triggers. With
		// neither day nor night, a daybound permanent makes it day (a
		// nightbound one night, unless a daybound permanent is out); as it
		// becomes night a front-face daybound permanent transforms, as it
		// becomes day a back-face nightbound one does. Each keyword's static
		// lets only its own ability transform the creature (:3867-3868,
		// :3950-3951), and Daybound's replacement has it enter transformed at
		// night (:2201-2208).
		day, set, side := "Night", "Day", "FrontSide"
		guard := ""
		if k.Name == "Nightbound" {
			day, set, side = "Day", "Night", "BackSide"
			guard = " | IsPresent$ Card.Daybound | PresentCompare$ EQ0"
		}
		exp := Expansion{
			Triggers: []string{
				"Mode$ Always | TriggerZones$ Battlefield | Static$ True | DayTime$ Neither" + guard + " | Secondary$ True | Execute$ KWDayBoundSet" + Slot,
				"Mode$ Always | TriggerZones$ Battlefield | Static$ True | DayTime$ " + day + " | IsPresent$ Card.Self+" + side + " | Secondary$ True | Execute$ KWDayBoundTransform" + Slot,
			},
			Statics: []string{"Mode$ CantTransform | ValidCard$ Creature.Self | ExceptCause$ SpellAbility." + k.Name + " | Secondary$ True"},
			SVars: []SVarDef{
				{"KWDayBoundSet" + Slot, "DB$ DayTime | Value$ " + set},
				{"KWDayBoundTransform" + Slot, "DB$ SetState | Mode$ Transform"},
			},
		}
		if k.Name == "Daybound" {
			exp.Replacements = []string{"Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield | DayTime$ Night | Secondary$ True | Layer$ Transform | ReplacementResult$ Updated | ReplaceWith$ KWDayBoundETB" + Slot}
			exp.SVars = append(exp.SVars, SVarDef{"KWDayBoundETB" + Slot, "DB$ SetState | Defined$ ReplacedCard | Mode$ Transform | ETB$ True"})
		}
		return exp, true
	case "Embalm", "Eternalize":
		// CardFactoryUtil.java:2822-2834 and :2914-2941: exile this card from
		// your graveyard and pay the cost to create a token copy of it, as a
		// sorcery, with no mana cost: a white Zombie (Embalm) or a black 4/4
		// Zombie (Eternalize).
		args := k.Args()
		if len(args) < 1 || args[0] == "" {
			return Expansion{}, false
		}
		line := "AB$ CopyPermanent | Cost$ " + args[0] + " ExileFromGrave<1/CARDNAME> | ActivationZone$ Graveyard | SorcerySpeed$ True | RemoveCost$ True | AddTypes$ Zombie | Defined$ Self"
		if k.Name == "Embalm" {
			line += " | SetColor$ White"
		} else {
			line += " | SetColor$ Black | SetPower$ 4 | SetToughness$ 4"
		}
		return Expansion{Abilities: []string{line}}, true
	case "Encore":
		// CardFactoryUtil.java:3478-3508: exile this card from your graveyard
		// and pay the cost, as a sorcery: for each opponent, a token copy with
		// haste that remembers that opponent and attacks it this turn if able
		// (each token's effect remembers it and its opponent), sacrificed at
		// the beginning of the next end step. Java appends a third field as
		// more params; no corpus line has one, so it is not expanded (GO-7).
		cost := costDetail(k)
		if cost == "" || len(k.Args()) > 1 {
			return Expansion{}, false
		}
		return Expansion{
			Abilities: []string{"AB$ CopyPermanent | Cost$ " + cost + " ExileFromGrave<1/CARDNAME> | ActivationZone$ Graveyard | SorcerySpeed$ True | Defined$ Self | PumpKeywords$ Haste | RememberTokens$ True | ForEach$ Opponent | AtEOT$ Sacrifice | SubAbility$ KWEncoreRepeat" + Slot},
			SVars: []SVarDef{
				{"KWEncoreRepeat" + Slot, "DB$ RepeatEach | DefinedCards$ Remembered | UseImprinted$ True | RepeatSubAbility$ KWEncoreEffect" + Slot},
				{"KWEncoreEffect" + Slot, "DB$ Effect | RememberObjects$ Imprinted & ImprintedRemembered | ExileOnMoved$ Battlefield | StaticAbilities$ KWEncoreAttack" + Slot + " | SubAbility$ KWEncoreClean" + Slot},
				{"KWEncoreAttack" + Slot, "Mode$ MustAttack | ValidCreature$ Card.IsRemembered | MustAttack$ RememberedPlayer | Description$ This token copy attacks that opponent this turn if able."},
				{"KWEncoreClean" + Slot, "DB$ Cleanup | Defined$ Imprinted | ForgetDefined$ Remembered"},
			},
		}, true
	case "Unearth":
		// CardFactoryUtil.java:3636-3648: return this card from your graveyard
		// to the battlefield, as a sorcery. The haste, the exile instead of
		// leaving and the end-step exile are the engine's, keyed on the
		// ability's keyword (ChangeZoneEffect.java:680-698, unearthReturned).
		cost := costDetail(k)
		if cost == "" {
			return Expansion{}, false
		}
		return Expansion{Abilities: []string{"AB$ ChangeZone | Cost$ " + cost + " | Defined$ Self | Origin$ Graveyard | Destination$ Battlefield | SorcerySpeed$ True | ActivationZone$ Graveyard"}}, true
	case "Rebound":
		// CardFactoryUtil.java:2422-2451: a spell cast from your hand that
		// resolves (Fizzle$ False: a countered spell carries no Fizzle key) is
		// exiled instead of going to the graveyard, and you may cast it without
		// paying its mana cost at your next upkeep.
		return Expansion{
			Replacements: []string{"Event$ Moved | ValidLKI$ Card.Self+wasCastFromHand+YouOwn+YouCtrl | Origin$ Stack | Destination$ Graveyard | Fizzle$ False | ReplaceWith$ KWReboundExile" + Slot},
			SVars: []SVarDef{
				{"KWReboundExile" + Slot, "DB$ ChangeZone | Defined$ ReplacedCard | Origin$ Stack | Destination$ Exile | RememberChanged$ True | SubAbility$ KWReboundDelay" + Slot},
				{"KWReboundDelay" + Slot, "DB$ DelayedTrigger | Mode$ Phase | Phase$ Upkeep | ValidPlayer$ You | OptionalDecider$ You | RememberObjects$ Remembered | Execute$ KWReboundPlay" + Slot},
				{"KWReboundPlay" + Slot, "DB$ Play | Defined$ DelayTriggerRememberedLKI | WithoutManaCost$ True | Optional$ True"},
			},
		}, true
	case "Fabricate":
		// CardFactoryUtil.java:1034: when this enters, put N +1/+1 counters on
		// it unless you create N 1/1 Servo artifact creature tokens -- as
		// scripted, create the tokens unless you pay "put the counters".
		n, ok := amountDetail(k)
		if !ok {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ ChangesZone | Destination$ Battlefield | ValidCard$ Card.Self | Secondary$ True | Execute$ KWFabricate" + Slot},
			SVars:    []SVarDef{{"KWFabricate" + Slot, "DB$ Token | TokenAmount$ " + n + " | TokenScript$ c_1_1_a_servo | UnlessCost$ AddCounter<" + n + "/P1P1> | UnlessPayer$ You"}},
		}, true
	case "Extort":
		// CardFactoryUtil.java:1016: whenever you cast a spell, you may pay
		// {W/B}; if you do, each opponent loses 1 life and you gain that much.
		// Java scopes AFLifeLost to the ability and defaults it to 0; the
		// default never reads, since the gain only runs after the loss.
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ SpellCast | ValidActivatingPlayer$ You | TriggerZones$ Battlefield | Secondary$ True | Execute$ KWExtort" + Slot},
			SVars: []SVarDef{
				{"KWExtort" + Slot, "AB$ LoseLife | Cost$ WB | Defined$ Player.Opponent | LifeAmount$ 1 | SubAbility$ KWExtortGain" + Slot},
				{"KWExtortGain" + Slot, "DB$ GainLife | Defined$ You | LifeAmount$ AFLifeLost"},
			},
		}, true
	case "Cascade":
		// CardFactoryUtil.java:711: when you cast this spell, exile cards from
		// the top of your library until a nonland card with lesser mana value;
		// you may cast it without paying its mana cost, then the exiled cards
		// go to the bottom in a random order.
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ SpellCast | ValidCard$ Card.Self | TriggerZones$ Stack | Secondary$ True | Execute$ KWCascade" + Slot},
			SVars: []SVarDef{
				{"KWCascade" + Slot, "DB$ DigUntil | Defined$ You | Amount$ 1 | Valid$ Card.nonLand+cmcLTKWCascadeX" + Slot + " | FoundDestination$ Exile | RevealedDestination$ Exile | ImprintFound$ True | RememberRevealed$ True | SubAbility$ KWCascadeCast" + Slot},
				{"KWCascadeCast" + Slot, "DB$ Play | Defined$ Imprinted | WithoutManaCost$ True | Optional$ True | ValidSA$ Spell.cmcLTKWCascadeX" + Slot + " | SubAbility$ KWCascadeLib" + Slot},
				{"KWCascadeLib" + Slot, "DB$ ChangeZoneAll | ChangeType$ Card.IsRemembered,Card.IsImprinted | Origin$ Exile | Destination$ Library | RandomOrder$ True | LibraryPosition$ -1 | SubAbility$ KWCascadeClean" + Slot},
				{"KWCascadeClean" + Slot, "DB$ Cleanup | ClearRemembered$ True | ClearImprinted$ True"},
			},
			Amounts: []SVarDef{{"KWCascadeX" + Slot, "Count$CardManaCost"}},
		}, true
	case "Storm":
		// CardFactoryUtil.java:1814: when you cast this spell, copy it for each
		// spell cast before it this turn; you may choose new targets.
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ SpellCast | ValidCard$ Card.Self | TriggerZones$ Stack | Secondary$ True | Execute$ KWStorm" + Slot},
			SVars: []SVarDef{
				{"KWStorm" + Slot, "DB$ CopySpellAbility | Defined$ TriggeredSpellAbility | Amount$ KWStormCount" + Slot + " | MayChooseTarget$ True"},
			},
			Amounts: []SVarDef{{"KWStormCount" + Slot, "TriggerCount$CurrentStormCount/Minus.1"}},
		}, true
	case "Exploit":
		// CardFactoryUtil.java:1006: when this enters, you may sacrifice a
		// creature.
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ ChangesZone | ValidCard$ Card.Self | Destination$ Battlefield | Secondary$ True | Execute$ KWExploit" + Slot},
			SVars:    []SVarDef{{"KWExploit" + Slot, "DB$ Sacrifice | SacValid$ Creature | Optional$ True"}},
		}, true
	case "Dash":
		// CardFactoryUtil.java:3864-3867: a permanent cast for its dash cost
		// has haste. The cast itself is the engine's (altkeywords.go), which
		// makes it for a lone cost field only; another shape stays inert, as
		// its cast is never offered.
		if k.Details == "" || strings.Contains(k.Details, ":") {
			return Expansion{}, false
		}
		return Expansion{Statics: []string{"Mode$ Continuous | Affected$ Card.Self+dashed+castKeyword | AddKeyword$ Haste"}}, true
	case "Blitz":
		// CardFactoryUtil.java:3767-3786: a permanent cast for its blitz cost
		// has haste and "When this creature dies, draw a card." A third field
		// (ValidAfterStack$, the CardManaCost grants of Henzie, The Caldaia and
		// Riveteers Provocateur) is a cast the engine does not offer, so it
		// stays inert rather than add a second static to a blitzed creature.
		if k.Details == "" || strings.Contains(k.Details, ":") {
			return Expansion{}, false
		}
		return Expansion{
			Statics: []string{"Mode$ Continuous | Affected$ Card.Self+blitzed+castKeyword | AddKeyword$ Haste | AddTrigger$ KWBlitzDies" + Slot + " | Secondary$ True"},
			SVars: []SVarDef{
				{"KWBlitzDies" + Slot, "Mode$ ChangesZone | Origin$ Battlefield | Destination$ Graveyard | ValidCard$ Card.Self | Execute$ KWBlitzDraw" + Slot + " | Secondary$ True"},
				{"KWBlitzDraw" + Slot, "DB$ Draw"},
			},
		}, true
	case "Echo":
		// CardFactoryUtil.java:963: at the beginning of your upkeep, if this
		// came under your control since your last upkeep, sacrifice it unless
		// you pay its echo cost (SacrificeEffect's Echo$).
		cost := costDetail(k)
		if cost == "" {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ Phase | Phase$ Upkeep | ValidPlayer$ You | TriggerZones$ Battlefield | IsPresent$ Card.Self+cameUnderControlSinceLastUpkeep | Secondary$ True | Execute$ KWEcho" + Slot},
			SVars:    []SVarDef{{"KWEcho" + Slot, "DB$ Sacrifice | SacValid$ Self | Echo$ " + cost}},
		}, true
	case "Cumulative upkeep":
		// CardFactoryUtil.java:860: at the beginning of your upkeep, put an age
		// counter on this, then sacrifice it unless you pay its cost once for
		// each age counter (SacrificeEffect's CumulativeUpkeep$).
		cost := costDetail(k)
		if cost == "" {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ Phase | Phase$ Upkeep | ValidPlayer$ You | TriggerZones$ Battlefield | IsPresent$ Card.Self | Secondary$ True | Execute$ KWCumulativeUpkeep" + Slot},
			SVars:    []SVarDef{{"KWCumulativeUpkeep" + Slot, "DB$ Sacrifice | SacValid$ Self | CumulativeUpkeep$ " + cost}},
		}, true
	case "Chapter":
		// CardFactoryUtil.java:812: Chapter:<N>:<SVar>,<SVar>,...: one
		// CounterAdded trigger per chapter, running that chapter's own SVar when
		// the lore counters reach its number. The Saga's lore counter on entering
		// and each turn, and its sacrifice, are native (saga.go).
		args := k.Args()
		if len(args) < 2 {
			return Expansion{}, false
		}
		final, err := strconv.Atoi(args[0])
		abilities := strings.Split(args[1], ",")
		if err != nil || final < 1 || len(abilities) != final {
			return Expansion{}, false
		}
		var triggers []string
		for i, ability := range abilities {
			if ability == "" {
				return Expansion{}, false
			}
			n := strconv.Itoa(i + 1)
			triggers = append(triggers, "Mode$ CounterAdded | ValidCard$ Card.Self | TriggerZones$ Battlefield | Chapter$ "+n+" | CounterType$ LORE | CounterAmount$ EQ"+n+" | Execute$ "+ability)
		}
		return Expansion{Triggers: triggers}, true
	case "Champion":
		// CardFactoryUtil.java:741-795 (CR 702.72): when it enters, exile
		// another permanent of a listed type you control or sacrifice it;
		// when it leaves, the exiled card returns. The exiled card is the
		// host's remembered card; ChangeZone's Champion$ fires Championed.
		if k.Details == "" {
			return Expansion{}, false
		}
		var types []string
		for _, v := range strings.Split(k.Details, ",") {
			types = append(types, strings.TrimSpace(v)+".YouCtrl+Other")
		}
		return Expansion{
			Triggers: []string{
				"Mode$ ChangesZone | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ KWChampion" + Slot,
				"Mode$ ChangesZone | Origin$ Battlefield | ValidCard$ Card.Self | Secondary$ True | Execute$ KWChampionReturn" + Slot,
			},
			SVars: []SVarDef{
				{"KWChampion" + Slot, "DB$ ChangeZone | Origin$ Battlefield | Destination$ Exile | RememberChanged$ True | Champion$ True | Hidden$ True | Optional$ True | ChangeType$ " + strings.Join(types, ",") + " | SubAbility$ KWChampionSac" + Slot},
				{"KWChampionSac" + Slot, "DB$ Sacrifice | ConditionDefined$ Remembered | ConditionPresent$ Card | ConditionCompare$ EQ0"},
				{"KWChampionReturn" + Slot, "DB$ ChangeZone | Defined$ Remembered | Origin$ Exile | Destination$ Battlefield | SubAbility$ KWChampionClear" + Slot},
				{"KWChampionClear" + Slot, "DB$ Cleanup | ClearRemembered$ True"},
			},
		}, true
	case "Enlist":
		// CardFactoryUtil.java:3899-3905 (CR 702.154): as it attacks, you may
		// tap a nonattacking creature you control without summoning sickness;
		// when you do, it gets +X/+0, X that creature's power.
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{
			Statics: []string{"Mode$ OptionalAttackCost | ValidCard$ Card.Self | Cost$ Enlist<1/CARDNAME/creature> | Secondary$ True | Trigger$ KWEnlist" + Slot},
			SVars:   []SVarDef{{"KWEnlist" + Slot, "DB$ Pump | NumAtt$ TriggerRemembered$CardPower"}},
		}, true
	case "Squad":
		// CardFactoryUtil.java:1777-1789: when it enters having been cast with
		// the Squad cost paid, create that many token copies of it. The
		// ValidCard$ linkedCastSA test is the amount itself: the cast's
		// record ends when the card leaves the battlefield.
		return Expansion{
			Triggers: []string{"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | CheckSVar$ KWSquad" + Slot + " | Secondary$ True | Execute$ KWSquadCopy" + Slot},
			SVars:    []SVarDef{{"KWSquadCopy" + Slot, "DB$ CopyPermanent | Defined$ TriggeredCardLKICopy | NumCopies$ KWSquad" + Slot}},
			Amounts:  []SVarDef{{"KWSquad" + Slot, "Count$OptionalKeywordAmount.Squad"}},
		}, true
	case "Replicate":
		// CardFactoryUtil.java:1698-1706: when you cast it, copy it for each time
		// its replicate cost was paid.
		return Expansion{
			Triggers: []string{"Mode$ SpellCast | ValidCard$ Card.Self | CheckSVar$ KWReplicate" + Slot + " | Secondary$ True | Execute$ KWReplicateCopy" + Slot},
			SVars:    []SVarDef{{"KWReplicateCopy" + Slot, "DB$ CopySpellAbility | Defined$ TriggeredSpellAbility | Amount$ KWReplicate" + Slot + " | MayChooseTarget$ True"}},
			Amounts:  []SVarDef{{"KWReplicate" + Slot, "Count$OptionalKeywordAmount.Replicate"}},
		}, true
	case "Casualty", "Conspire":
		// CardFactoryUtil.java:795-810, :851-860: when you cast it having paid
		// the keyword's cost, copy it. Casualty X carries trailing copy params
		// (K:Casualty:X:NonLegendary$ True | SetLoyalty$ Casualty:text, k[2]),
		// whose SetLoyalty$ reads the amount recorded under the keyword's name.
		extra := ""
		if k.Name == "Casualty" {
			n, ok := amountDetail(k)
			if args := k.Args(); !ok && len(args) >= 2 && args[0] == "X" && args[1] != "" {
				extra = " | " + strings.ReplaceAll(args[1], "SetLoyalty$ Casualty", "SetLoyalty$ KWCasualty"+Slot)
			} else if !ok || n == "X" {
				return Expansion{}, false
			}
		}
		return Expansion{
			Triggers: []string{"Mode$ SpellCast | ValidCard$ Card.Self | CheckSVar$ KW" + k.Name + Slot + " | Secondary$ True | Execute$ KW" + k.Name + "Copy" + Slot},
			SVars:    []SVarDef{{"KW" + k.Name + "Copy" + Slot, "DB$ CopySpellAbility | Defined$ TriggeredSpellAbility | MayChooseTarget$ True" + extra}},
			Amounts:  []SVarDef{{"KW" + k.Name + Slot, "Count$OptionalKeywordAmount." + k.Name}},
		}, true
	case "Offspring":
		// CardFactoryUtil.java:1539-1556: when it enters having been cast with
		// the Offspring cost paid, create a token copy that is 1/1.
		return Expansion{
			Triggers: []string{"Mode$ ChangesZone | Destination$ Battlefield | ValidCard$ Card.Self | CheckSVar$ KWOffspring" + Slot + " | Secondary$ True | Execute$ KWOffspringCopy" + Slot},
			SVars:    []SVarDef{{"KWOffspringCopy" + Slot, "DB$ CopyPermanent | Defined$ TriggeredCardLKICopy | SetPower$ 1 | SetToughness$ 1"}},
			Amounts:  []SVarDef{{"KWOffspring" + Slot, "Count$OptionalKeywordAmount.Offspring"}},
		}, true
	case "Renown":
		// CardFactoryUtil.java:1655: when this deals combat damage to a player, if
		// it is not renowned, put N +1/+1 counters on it and it becomes renowned
		// (PutCounter sets that when its ability is this keyword's).
		n, ok := amountDetail(k)
		if !ok {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ DamageDone | ValidSource$ Card.Self | ValidTarget$ Player | IsPresent$ Card.Self+!IsRenowned | CombatDamage$ True | Secondary$ True | Execute$ KWRenown" + Slot},
			SVars:    []SVarDef{{"KWRenown" + Slot, "DB$ PutCounter | Defined$ Self | CounterType$ P1P1 | CounterNum$ " + n}},
		}, true
	case "Bloodthirst":
		// CardFactoryUtil.java:2119: enters with N +1/+1 counters if an opponent
		// was dealt damage this turn; Bloodthirst X counts that damage.
		args := k.Args()
		if len(args) < 1 || args[0] == "" {
			return Expansion{}, false
		}
		n := args[0]
		exp := Expansion{Replacements: []string{"Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield | Secondary$ True | ReplacementResult$ Updated | ReplaceWith$ KWBloodthirst" + Slot + " | Bloodthirst$ True"}}
		if n == "X" {
			n = "KWBloodthirstX" + Slot
			exp.Amounts = []SVarDef{{n, "Count$BloodthirstAmount"}}
		} else if strings.Trim(n, "0123456789") != "" {
			return Expansion{}, false
		}
		exp.SVars = []SVarDef{{"KWBloodthirst" + Slot, "DB$ PutCounter | Defined$ Self | CounterType$ P1P1 | ETB$ True | CounterNum$ " + n}}
		return exp, true
	case "Devour":
		return expandDevour(k)
	case "Madness":
		return expandMadness(k)
	case "Modular":
		// CardFactoryUtil.java:1490 and :2377: enters with N +1/+1 counters, and
		// when it dies you may put a +1/+1 counter on target artifact creature for
		// each +1/+1 counter on it. Java's decider is the triggered card's
		// controller, which for this trigger on the dying card itself is "You".
		// Modular:Sunburst counts the colors spent (Count$Converge, :2386).
		n, ok := amountDetail(k)
		amounts := []SVarDef{{"KWModularX" + Slot, "TriggeredCard$CardCounters.P1P1"}}
		if k.Details == "Sunburst" {
			n, ok = "KWModularSunburst"+Slot, true
			amounts = append(amounts, SVarDef{n, "Count$Converge"})
		}
		if !ok {
			return Expansion{}, false
		}
		return Expansion{
			Replacements: []string{"Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield | Secondary$ True | ReplacementResult$ Updated | ReplaceWith$ KWModularEtb" + Slot},
			Triggers:     []string{"Mode$ ChangesZone | Origin$ Battlefield | Destination$ Graveyard | ValidCard$ Card.Self | OptionalDecider$ You | Secondary$ True | Execute$ KWModular" + Slot},
			SVars: []SVarDef{
				{"KWModularEtb" + Slot, "DB$ PutCounter | Defined$ Self | CounterType$ P1P1 | ETB$ True | CounterNum$ " + n},
				{"KWModular" + Slot, "DB$ PutCounter | ValidTgts$ Artifact.Creature | TgtPrompt$ Select target artifact creature | CounterType$ P1P1 | CounterNum$ KWModularX" + Slot},
			},
			Amounts: amounts,
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
	case "Soulbond":
		// CardFactoryUtil.java:1740-1760: when it or another creature enters
		// under your control and one of the pair is unpaired, you may pair
		// them. The Execute$ SVar is Java's setOverridingAbility.
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{
				"Mode$ ChangesZone | Destination$ Battlefield | ValidCard$ Card.Self | IsPresent$ Creature.Other+YouCtrl+!Paired | Secondary$ True | Execute$ KWSoulbondSelf" + Slot,
				"Mode$ ChangesZone | Destination$ Battlefield | ValidCard$ Creature.Other+YouCtrl | TriggerZones$ Battlefield | IsPresent$ Creature.Self+!Paired | Secondary$ True | Execute$ KWSoulbondOther" + Slot,
			},
			SVars: []SVarDef{
				{"KWSoulbondSelf" + Slot, "DB$ Bond | Defined$ TriggeredCardLKICopy | ValidCards$ Creature.Other+YouCtrl+!Paired"},
				{"KWSoulbondOther" + Slot, "DB$ Bond | Defined$ TriggeredCardLKICopy | ValidCards$ Creature.Self+!Paired"},
			},
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
	case "Vanishing":
		// CardFactoryUtil.java:1910-1930 and :2529-2538: Vanishing:N enters
		// with N time counters (makeEtbCounter); every form removes one at
		// its controller's upkeep and is sacrificed when the last is removed.
		// A bare Vanishing (its card counts the counters itself) has only the
		// two triggers.
		exp := Expansion{
			Triggers: []string{
				"Mode$ Phase | Phase$ Upkeep | ValidPlayer$ You | TriggerZones$ Battlefield | IsPresent$ Card.Self+counters_GE1_TIME | Secondary$ True | Execute$ KWVanishingRemove" + Slot,
				"Mode$ CounterRemoved | TriggerZones$ Battlefield | ValidCard$ Card.Self | NewCounterAmount$ 0 | CounterType$ TIME | Secondary$ True | Execute$ KWVanishingSac" + Slot,
			},
			SVars: []SVarDef{
				{"KWVanishingRemove" + Slot, "DB$ RemoveCounter | Defined$ Self | CounterType$ TIME | CounterNum$ 1"},
				{"KWVanishingSac" + Slot, "DB$ Sacrifice | SacValid$ Self"},
			},
		}
		if k.Details != "" {
			n, ok := amountDetail(k)
			if !ok {
				return Expansion{}, false
			}
			exp.Replacements = []string{"Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield | Secondary$ True | ReplacementResult$ Updated | ReplaceWith$ KWVanishingETB" + Slot}
			exp.SVars = append(exp.SVars, SVarDef{"KWVanishingETB" + Slot, "DB$ PutCounter | Defined$ Self | CounterType$ TIME | ETB$ True | CounterNum$ " + n})
		}
		return exp, true
	case "Living Weapon":
		// CardFactoryUtil.java:1361-1385: when this Equipment enters, create a
		// 0/0 black Phyrexian Germ token and attach this to it.
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ ChangesZone | Destination$ Battlefield | ValidCard$ Card.Self | Secondary$ True | Execute$ KWLivingWeapon" + Slot},
			SVars: []SVarDef{
				{"KWLivingWeapon" + Slot, "DB$ Token | TokenScript$ b_0_0_phyrexian_germ | TokenOwner$ You | RememberTokens$ True | SubAbility$ KWLivingWeaponAttach" + Slot},
				{"KWLivingWeaponAttach" + Slot, "DB$ Attach | Defined$ Remembered | SubAbility$ KWLivingWeaponCleanup" + Slot},
				{"KWLivingWeaponCleanup" + Slot, "DB$ Cleanup | ClearRemembered$ True"},
			},
		}, true
	case "Myriad":
		// CardFactoryUtil.java:1508-1521: whenever this attacks, for each
		// opponent other than the defending player you may create a token copy
		// tapped and attacking that player or a planeswalker they control,
		// exiled at end of combat.
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ Attacks | ValidCard$ Card.Self | Secondary$ True | Execute$ KWMyriad" + Slot},
			SVars:    []SVarDef{{"KWMyriad" + Slot, "DB$ CopyPermanent | Defined$ Self | TokenTapped$ True | OptionalForEach$ True | TokenAttacking$ RememberedPlayer & Valid Planeswalker.ControlledBy Remembered | ForEach$ OppNonDefendingPlayer | AtEOT$ ExileCombat | CleanupForEach$ True"}},
		}, true
	case "Strive":
		// CardFactoryUtil.java:4019-4025: the spell costs <cost> more for each
		// target beyond the first (CostAdjustment.java:139-142, Amount$ Strive).
		cost := costDetail(k)
		if cost == "" || len(k.Args()) > 1 {
			return Expansion{}, false
		}
		return Expansion{Statics: []string{"Mode$ RaiseCost | ValidCard$ Card.Self | Type$ Spell | Amount$ Strive | Cost$ " + cost + " | EffectZone$ All"}}, true
	case "Firebending":
		// CardFactoryUtil.java:1075-1094: whenever this attacks, add N {R};
		// the mana lasts until end of combat (CombatMana$). The third field is
		// the X description, display only; Amount$ X reads the card's SVar.
		args := k.Args()
		if len(args) < 1 || (args[0] != "X" && strings.Trim(args[0], "0123456789") != "") || args[0] == "" {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ Attacks | ValidCard$ Card.Self | Execute$ KWFirebending" + Slot},
			SVars:    []SVarDef{{"KWFirebending" + Slot, "DB$ Mana | Defined$ You | CombatMana$ True | Produced$ R | Amount$ " + args[0]}},
		}, true
	case "Mobilize":
		// CardFactoryUtil.java:1476-1489: whenever this attacks, create N 1/1
		// red Warrior tokens tapped and attacking, sacrificed at the next end
		// step. Mobilize:X reads the card's own SVar X.
		args := k.Args()
		if len(args) < 1 || args[0] == "" || (args[0] != "X" && strings.Trim(args[0], "0123456789") != "") {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ Attacks | ValidCard$ Card.Self | Execute$ KWMobilize" + Slot},
			SVars:    []SVarDef{{"KWMobilize" + Slot, "DB$ Token | TokenAmount$ " + args[0] + " | TokenScript$ r_1_1_warrior | TokenTapped$ True | TokenAttacking$ True | AtEOT$ Sacrifice"}},
		}, true
	case "For Mirrodin":
		// CardFactoryUtil.java:1108-1131: when this Equipment enters, create a
		// 2/2 red Rebel token and attach this to it.
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ ChangesZone | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ KWForMirrodin" + Slot},
			SVars: []SVarDef{
				{"KWForMirrodin" + Slot, "DB$ Token | TokenScript$ r_2_2_rebel | TokenOwner$ You | RememberTokens$ True | SubAbility$ KWForMirrodinAttach" + Slot},
				{"KWForMirrodinAttach" + Slot, "DB$ Attach | Defined$ Remembered | SubAbility$ KWForMirrodinCleanup" + Slot},
				{"KWForMirrodinCleanup" + Slot, "DB$ Cleanup | ClearRemembered$ True"},
			},
		}, true
	case "Living metal":
		// CardFactoryUtil.java:3947-3949: during your turn, this is an
		// artifact creature.
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{Statics: []string{"Mode$ Continuous | Affected$ Card.Self | AddType$ Creature | Condition$ PlayerTurn | Secondary$ True"}}, true
	case "Fading":
		// CardFactoryUtil.java:2239-2252 (makeEtbCounter) and :1054-1075: this
		// enters with N fade counters; at your upkeep remove one, and if you
		// can't, sacrifice it.
		n, ok := amountDetail(k)
		if !ok {
			return Expansion{}, false
		}
		return Expansion{
			Replacements: []string{"Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield | Secondary$ True | ReplacementResult$ Updated | ReplaceWith$ KWFadingETB" + Slot},
			Triggers:     []string{"Mode$ Phase | Phase$ Upkeep | ValidPlayer$ You | TriggerZones$ Battlefield | Secondary$ True | Execute$ KWFadingRemove" + Slot},
			SVars: []SVarDef{
				{"KWFadingETB" + Slot, "DB$ PutCounter | Defined$ Self | CounterType$ FADE | ETB$ True | CounterNum$ " + n},
				{"KWFadingRemove" + Slot, "DB$ RemoveCounter | Defined$ Self | CounterType$ FADE | CounterNum$ 1 | RememberRemoved$ True | SubAbility$ KWFadingSac" + Slot},
				{"KWFadingSac" + Slot, "DB$ Sacrifice | SacValid$ Self | ConditionCheckSVar$ KWFadingCheck" + Slot + " | ConditionSVarCompare$ EQ0 | SubAbility$ KWFadingCleanup" + Slot},
				{"KWFadingCleanup" + Slot, "DB$ Cleanup | ClearRemembered$ True"},
			},
			Amounts: []SVarDef{{"KWFadingCheck" + Slot, "Count$RememberedSize"}},
		}, true
	case "Rampage":
		// CardFactoryUtil.java:1622-1640: whenever this becomes blocked, it
		// gets +N/+N for each creature blocking it beyond the first.
		n, ok := amountDetail(k)
		if !ok {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ AttackerBlocked | ValidCard$ Card.Self | TriggerZones$ Battlefield | ValidBlocker$ Creature | Secondary$ True | Execute$ KWRampage" + Slot},
			SVars:    []SVarDef{{"KWRampage" + Slot, "DB$ Pump | Defined$ TriggeredAttackerLKICopy | NumAtt$ KWRampagePump" + Slot + " | NumDef$ KWRampagePump" + Slot}},
			Amounts: []SVarDef{
				{"KWRampagePump" + Slot, "SVar$KWRampageCount" + Slot + "/Times." + n},
				{"KWRampageCount" + Slot, "Count$Valid Creature.blockingTriggeredAttacker/Minus.1"},
			},
		}, true
	case "Graft":
		// CardFactoryUtil.java:2303-2319 (makeEtbCounter) and :1152-1170: this
		// enters with N +1/+1 counters; whenever another creature enters, you
		// may move a +1/+1 counter from this onto it. AILogic$ is the AI's.
		n, ok := amountDetail(k)
		if !ok {
			return Expansion{}, false
		}
		return Expansion{
			Replacements: []string{"Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield | Secondary$ True | ReplacementResult$ Updated | ReplaceWith$ KWGraftETB" + Slot},
			Triggers:     []string{"Mode$ ChangesZone | ValidCard$ Creature.Other | Origin$ Any | Destination$ Battlefield | TriggerZones$ Battlefield | OptionalDecider$ You | IsPresent$ Card.Self+counters_GE1_P1P1 | Secondary$ True | Execute$ KWGraftMove" + Slot},
			SVars: []SVarDef{
				{"KWGraftETB" + Slot, "DB$ PutCounter | Defined$ Self | CounterType$ P1P1 | ETB$ True | CounterNum$ " + n},
				{"KWGraftMove" + Slot, "DB$ MoveCounter | Source$ Self | Defined$ TriggeredCardLKICopy | CounterType$ P1P1 | CounterNum$ 1"},
			},
		}, true
	case "Riot":
		// CardFactoryUtil.java:2470-2476 (createETBReplacement): this enters
		// with a +1/+1 counter, or else it gains haste.
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{
			Replacements: []string{"Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield | ReplacementResult$ Updated | Secondary$ True | ReplaceWith$ KWRiot" + Slot},
			SVars:        []SVarDef{{"KWRiot" + Slot, "DB$ Animate | Defined$ Self | Keywords$ Haste | Duration$ Permanent | UnlessCost$ AddCounter<1/P1P1> | UnlessPayer$ You"}},
		}, true
	case "Unleash":
		// CardFactoryUtil.java:2523-2528 and :4030-4032: you may have this
		// enter with a +1/+1 counter; it can't block while it has one.
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{
			Replacements: []string{"Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield | ReplacementResult$ Updated | Optional$ True | Secondary$ True | ReplaceWith$ KWUnleash" + Slot},
			Statics:      []string{"Mode$ CantBlock | ValidCard$ Creature.Self+counters_GE1_P1P1 | Secondary$ True"},
			SVars:        []SVarDef{{"KWUnleash" + Slot, "DB$ PutCounter | Defined$ Self | CounterType$ P1P1 | ETB$ True | CounterNum$ 1"}},
		}, true
	case "Dredge":
		// CardFactoryUtil.java:2177-2200: if you would draw a card while this
		// is in your graveyard, you may mill N instead (only with N or more
		// cards in your library) and return this to your hand.
		n, ok := amountDetail(k)
		if !ok {
			return Expansion{}, false
		}
		return Expansion{
			Replacements: []string{"Event$ Draw | ActiveZones$ Graveyard | ValidPlayer$ You | Secondary$ True | Optional$ True | CheckSVar$ KWDredgeCheckLib" + Slot + " | SVarCompare$ GE" + n + " | ReplaceWith$ KWDredgeMill" + Slot},
			SVars: []SVarDef{
				{"KWDredgeMill" + Slot, "DB$ Mill | Defined$ You | NumCards$ " + n + " | SubAbility$ KWDredgeReturn" + Slot},
				{"KWDredgeReturn" + Slot, "DB$ ChangeZone | Origin$ Graveyard | Destination$ Hand | Defined$ Self"},
			},
			Amounts: []SVarDef{{"KWDredgeCheckLib" + Slot, "Count$ValidLibrary Card.YouOwn"}},
		}, true
	case "Hideaway":
		// CardFactoryUtil.java:1273-1305: when this enters, look at the top N
		// cards of your library, exile one face down and put the rest on the
		// bottom in a random order; an Effect lets you look at the exiled card.
		n, ok := amountDetail(k)
		if !ok {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ ChangesZone | Destination$ Battlefield | ValidCard$ Card.Self | Secondary$ True | Execute$ KWHideaway" + Slot},
			SVars: []SVarDef{
				{"KWHideaway" + Slot, "DB$ Dig | Defined$ You | DigNum$ " + n + " | DestinationZone$ Exile | ExileFaceDown$ True | RememberChanged$ True | RestRandomOrder$ True | SubAbility$ KWHideawayEffect" + Slot},
				{"KWHideawayEffect" + Slot, "DB$ Effect | StaticAbilities$ KWHideawayLook" + Slot + " | ForgetOnMoved$ Exile | RememberObjects$ Remembered | Duration$ Permanent | SubAbility$ KWHideawayCleanup" + Slot},
				{"KWHideawayLook" + Slot, "Mode$ Continuous | Affected$ Card.IsRemembered | MayLookAt$ EffectSourceController | EffectZone$ Command | AffectedZone$ Exile"},
				{"KWHideawayCleanup" + Slot, "DB$ Cleanup | ClearRemembered$ True"},
			},
		}, true
	case "Double team":
		// CardFactoryUtil.java:945-962: when this attacks, if it's not a token,
		// conjure a duplicate of it into your hand; then both lose double team
		// perpetually.
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{
			Triggers: []string{"Mode$ Attacks | ValidCard$ Card.Self+!token | TriggerZones$ Battlefield | Secondary$ True | Execute$ KWDoubleTeam" + Slot},
			SVars: []SVarDef{
				{"KWDoubleTeam" + Slot, "DB$ MakeCard | DefinedName$ Self | Zone$ Hand | RememberMade$ True | Conjure$ True | SubAbility$ KWDoubleTeamSelf" + Slot},
				{"KWDoubleTeamSelf" + Slot, "DB$ Pump | RememberObjects$ Self | SubAbility$ KWDoubleTeamLose" + Slot},
				{"KWDoubleTeamLose" + Slot, "DB$ Animate | Duration$ Perpetual | Defined$ Remembered | RemoveKeywords$ Double team | SubAbility$ KWDoubleTeamCleanup" + Slot},
				{"KWDoubleTeamCleanup" + Slot, "DB$ Cleanup | ClearRemembered$ True"},
			},
		}, true
	case "Reconfigure":
		// CardFactoryUtil.java:3374-3395: attach to target creature you control
		// and unattach, each at sorcery speed. A second field is an
		// AlternateCost$ (Razorfield Ripper's PayEnergy<3>), which no Attach
		// this port resolves reads: left inert, as Equip's is.
		args := k.Args()
		if len(args) != 1 || args[0] == "" {
			return Expansion{}, false
		}
		return Expansion{Abilities: []string{
			"AB$ Attach | ValidTgts$ Creature.YouCtrl+Other | TgtPrompt$ Select target creature you control | AILogic$ Pump | Cost$ " + args[0] + " | SorcerySpeed$ True | Secondary$ True",
			"AB$ Unattach | Defined$ Self | IsPresent$ Card.Self+AttachedTo Creature | Cost$ " + args[0] + " | SorcerySpeed$ True | Secondary$ True",
		}}, true
	case "Cipher":
		// CardFactoryUtil.java:3796-3821: while this card is exiled and
		// encoded on a creature, that creature has "whenever this deals combat
		// damage to a player, its controller may cast a copy of the encoded
		// card without paying its mana cost". The DB$ Encode the spell ends
		// with (:2714-2720) is appended by the compiler.
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{
			Statics: []string{"Mode$ Continuous | EffectZone$ Exile | Affected$ Card.EncodedWithSource | AddTrigger$ KWCipherTrigger" + Slot},
			SVars: []SVarDef{
				{"KWCipherTrigger" + Slot, "Mode$ DamageDone | ValidSource$ Card.Self | ValidTarget$ Player | Execute$ KWCipherPlay" + Slot + " | CombatDamage$ True | OptionalDecider$ You"},
				{"KWCipherPlay" + Slot, "DB$ Play | Defined$ OriginalHost | WithoutManaCost$ True | CopyCard$ True"},
			},
		}, true
	case "Umbra armor":
		// CardFactoryUtil.java:2501-2525: if the enchanted permanent would be
		// destroyed, instead remove all damage from it and destroy this.
		if k.Details != "" {
			return Expansion{}, false
		}
		return Expansion{
			Replacements: []string{"Event$ Destroy | ActiveZones$ Battlefield | ValidCard$ Card.EnchantedBy | Secondary$ True | ReplaceWith$ KWUmbraHeal" + Slot},
			SVars: []SVarDef{
				{"KWUmbraHeal" + Slot, "DB$ HealDamage | Defined$ ReplacedCard | SubAbility$ KWUmbraDestroy" + Slot},
				{"KWUmbraDestroy" + Slot, "DB$ Destroy | Defined$ Self"},
			},
		}, true
	}
	return Expansion{}, false
}

// ExpandSunburst is CardFactoryUtil.java:2477-2489's Sunburst: the permanent
// enters with a +1/+1 counter for each color of mana spent to cast it when the
// card is a creature, a charge counter otherwise. Java picks the counter from
// the card as the keyword is added (host.isCreature()), so the caller passes
// the face's printed type; the amount is Count$Converge (:2487).
func ExpandSunburst(k Keyword, creature bool) (Expansion, bool) {
	if k.Name != "Sunburst" || k.Details != "" {
		return Expansion{}, false
	}
	counter := "CHARGE"
	if creature {
		counter = "P1P1"
	}
	return Expansion{
		Replacements: []string{"Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield | Secondary$ True | ReplacementResult$ Updated | ReplaceWith$ KWSunburst" + Slot},
		SVars:        []SVarDef{{"KWSunburst" + Slot, "DB$ PutCounter | Defined$ Self | CounterType$ " + counter + " | ETB$ True | CounterNum$ KWSunburstX" + Slot}},
		Amounts:      []SVarDef{{"KWSunburstX" + Slot, "Count$Converge"}},
	}, true
}

// ExpandSuspend is CardFactoryUtil.java:1829-1873's Suspend triggers, for
// every Suspend line (a granted bare one too): at your upkeep, remove a time
// counter from it while it is suspended; as the last is removed, you may cast
// it without paying its mana cost, and a permanent spell's creature gains
// haste for as long as you control it. Java builds the haste half only when
// the card is a permanent (:1851), so the caller passes the face's printed
// type. The special action is the engine's (suspend.go).
func ExpandSuspend(k Keyword, permanent bool) (Expansion, bool) {
	if k.Name != "Suspend" {
		return Expansion{}, false
	}
	play := "DB$ Play | Defined$ Self | WithoutManaCost$ True | Optional$ True"
	svars := []SVarDef{{"KWSuspendRemove" + Slot, "DB$ RemoveCounter | Defined$ Self | CounterType$ TIME | CounterNum$ 1"}}
	if permanent {
		play += " | RememberPlayed$ True | SubAbility$ KWSuspendHaste" + Slot
		svars = append(svars,
			SVarDef{"KWSuspendHaste" + Slot, "DB$ Pump | Defined$ Remembered | KW$ Haste | PumpZone$ Stack | ConditionDefined$ Remembered | ConditionPresent$ Creature | Duration$ UntilLoseControlOfHost | SubAbility$ KWSuspendCleanup" + Slot},
			SVarDef{"KWSuspendCleanup" + Slot, "DB$ Cleanup | ClearRemembered$ True"})
	}
	svars = append(svars, SVarDef{"KWSuspendPlay" + Slot, play})
	return Expansion{
		Triggers: []string{
			"Mode$ Phase | Phase$ Upkeep | ValidPlayer$ You | TriggerZones$ Exile | IsPresent$ Card.Self+suspended | PresentZone$ Exile | Secondary$ True | Execute$ KWSuspendRemove" + Slot,
			"Mode$ CounterRemoved | TriggerZones$ Exile | ValidCard$ Card.Self | CounterType$ TIME | NewCounterAmount$ 0 | Secondary$ True | Execute$ KWSuspendPlay" + Slot,
		},
		SVars: svars,
	}, true
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

// expandDevour is CardFactoryUtil.java:2211-2238's Devour branch,
// "Devour:<N|X>[:<type>[:<extra description>]]" (Devour.java:32-51): as the
// permanent enters, its controller may sacrifice any number of other
// permanents of the type (Creature by default), then it enters with N +1/+1
// counters for each, and the remembered sacrifices are cleared. Java scopes
// DevourSacX and DevourX to the two abilities; here they are slotted face
// amounts. SacMessage$ is display only and dropped. An X amount reads the
// card's own SVar X (Thromok the Insatiable); any other non-number is not
// translated.
func expandDevour(k Keyword) (Expansion, bool) {
	args := k.Args()
	if len(args) < 1 || args[0] == "" || (args[0] != "X" && strings.Trim(args[0], "0123456789") != "") {
		return Expansion{}, false
	}
	valid := "Creature"
	if len(args) > 1 && args[1] != "" {
		valid = args[1]
	}
	return Expansion{
		Replacements: []string{"Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield | ReplacementResult$ Updated | ReplaceWith$ KWDevour" + Slot},
		SVars: []SVarDef{
			{"KWDevour" + Slot, "DB$ Sacrifice | Defined$ You | Amount$ KWDevourSacX" + Slot + " | RememberSacrificed$ True | Optional$ True | SacValid$ " + valid + ".Other | SubAbility$ KWDevourCounters" + Slot},
			{"KWDevourCounters" + Slot, "DB$ PutCounter | ETB$ True | Defined$ Self | CounterType$ P1P1 | CounterNum$ KWDevourX" + Slot + " | SubAbility$ KWDevourCleanup" + Slot},
			{"KWDevourCleanup" + Slot, "DB$ Cleanup | ClearRemembered$ True"},
		},
		Amounts: []SVarDef{
			{"KWDevourSacX" + Slot, "Count$Valid " + valid + ".YouCtrl+Other"},
			{"KWDevourX" + Slot, "Count$RememberedSize/Times." + args[0]},
		},
	}, true
}

// expandMadness is CardFactoryUtil's two Madness branches, "Madness:<cost>":
// the replacement "if you discard this card, discard it into exile"
// (CardFactoryUtil.java:2345-2362), and the Mode$ Exiled trigger "when you
// do, cast it for its madness cost or put it into your graveyard"
// (:1386-1420), whose ability is a Play of the card for PlayCost$ <cost>, then
// a move of the card from exile to the graveyard when it was not cast.
// The lines are Java's text minus display-only params. TriggerExiled fires
// the trigger only for the exile this
// keyword's own replacement caused (TriggerExiled.java:87-94); the engine
// reads that off the trigger's keyword.
func expandMadness(k Keyword) (Expansion, bool) {
	cost := costDetail(k)
	if cost == "" {
		return Expansion{}, false
	}
	return Expansion{
		Replacements: []string{"Event$ Moved | ActiveZones$ Hand | ValidCard$ Card.Self | Discard$ True | Secondary$ True | ReplaceWith$ KWMadnessExile" + Slot},
		Triggers:     []string{"Mode$ Exiled | ValidCard$ Card.Self | Secondary$ True | Execute$ KWMadnessPlay" + Slot},
		SVars: []SVarDef{
			{"KWMadnessExile" + Slot, "DB$ ChangeZone | Hidden$ True | Origin$ All | Destination$ Exile | Defined$ ReplacedCard"},
			{"KWMadnessPlay" + Slot, "DB$ Play | Defined$ Self | ValidSA$ Spell | PlayCost$ " + cost + " | ConditionDefined$ Self | ConditionPresent$ Card.StrictlySelf+inZoneExile | Optional$ True | RememberPlayed$ True | SubAbility$ KWMadnessGrave" + Slot},
			{"KWMadnessGrave" + Slot, "DB$ ChangeZone | Defined$ Self.StrictlySelf | Origin$ Exile | Destination$ Graveyard | TrackDiscarded$ True | ConditionDefined$ Remembered | ConditionPresent$ Card | ConditionCompare$ EQ0 | SubAbility$ KWMadnessCleanup" + Slot},
			{"KWMadnessCleanup" + Slot, "DB$ Cleanup | ClearRemembered$ True"},
		},
	}, true
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

// expandSpecialize is CardFactoryUtil.java:3507's Specialize branch:
// Specialize:<cost>[:<flavor>[:<condition text>[:<extra params>]]]. The cost,
// a color named for good and a card of that color or its basic land type
// discarded, turn the card into that color's face at sorcery speed. The flavor
// and the condition text are display only.
func expandSpecialize(k Keyword) (Expansion, bool) {
	args := k.Args()
	if len(args) < 1 || args[0] == "" {
		return Expansion{}, false
	}
	line := "AB$ SetState | Cost$ " + args[0] + " ChooseColor<1> Discard<1/Card.ChosenColor;Card.AssociatedWithChosenColor> | Mode$ Specialize | SorcerySpeed$ True | ActivationZone$ Hand"
	if len(args) > 3 && args[3] != "" {
		line += " | " + args[3]
	}
	return Expansion{Abilities: []string{line}}, true
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

// costDetail is the cost an Echo-shaped keyword names: its first detail, the
// whole text up to the next colon.
func costDetail(k Keyword) string {
	args := k.Args()
	if len(args) < 1 {
		return ""
	}
	return args[0]
}
