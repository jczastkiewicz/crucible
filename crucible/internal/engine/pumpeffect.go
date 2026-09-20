// Pump: CR 611's own biggest single script-driven effect by real corpus
// count after ChangeZone/Draw -- 4,103 real (AB|DB)$ Pump lines, 1,335 of
// them naming Defined$ Self/Enchanted/Equipped rather than a target, 1,147
// of THOSE resolvable here. This is the first script-driven effect whose own
// contribution outlives its own Resolve call: "until end of turn" is a
// continuous effect this port never needed a duration for before (Card.PT's
// own doc comment, PT.Clear -- "an until end of turn pump wearing off on its
// own is a separate, unbuilt mechanic"), closed by pumpRecord (game.go) and
// applyPumpEffects (continuous.go) rather than anything in this file.
//
// Ported from
// forge-game/src/main/java/forge/game/ability/effects/PumpEffect.java's
// resolve/applyPump.

package engine

import (
	"fmt"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// pumpUnresolvedParams names PumpEffect.resolve's own params past
// NumAtt$/NumDef$/KW$/Duration$/PumpZone$/Defined$ this port does not
// evaluate. Every one fails the whole line loudly rather than applying half
// of it and guessing at the rest (PORT-8/GO-7): SubAbility$ (119 of 1,335
// real Defined$ Self/Enchanted/Equipped lines) -- no ability-chaining
// mechanism exists yet; Condition$/ConditionDefined$/ConditionZone$/
// ConditionPlayerTurn$/ConditionActivationLimit$ (0/19/0/0/4) --
// SpellAbilityCondition's own shapes subAbilityConditionMet does not cover,
// the identical DealDamage/GainLife-shaped gap; PlayerTurn$ (2) -- unclear
// semantics on a Pump line, not worth guessing at from two real lines;
// UnlessCost$/UnlessPayer$/UnlessSwitched$ (6/6/4) -- CR 601.2i's own "unless
// a cost is paid" branch, a further mechanic; CanBlockAmount$/CanBlockAny$
// (4/0) -- an additional-blocker grant this port's own block-legality gate
// (staticability.go) has nowhere to consult a one-shot record from;
// DefinedKW$/KWChoice$/RandomKeyword$/RandomKWNum$/NoRepetition$ (3/3/1/0/0)
// -- a placeholder substitution, an interactive choice, and a random draw,
// none of which this port's own KW$ handling (below) does; SharedKeywordsZone$/
// SharedRestrictions$ (2/2) -- CardFactoryUtil.sharedKeywords' own zone scan,
// a further mechanic; ValidTgts$ (1) -- a real target past the Defined$ card
// this effect already resolves; AtEOT$ (9) -- registerDelayedTrigger, a new
// trigger this effect would silently fail to create; DefinedLandwalk$/
// ForgetObjects$/RememberObjects$/RememberPumped$/LeaveBattlefield$/
// ImprintCards$/ForgetImprinted$ (0/0/0/0/0/1/0) -- each its own further
// tracking mechanic; NoteCards$/NoteCardsFor$/ClearNotedCardsFor$/
// NoteNumber$ (0/0/0/0) -- Player.noteNumberForName's own tracking, nothing
// downstream reads yet; IsPresent$ (6) -- unclear semantics on a resolving
// (not triggering) Pump line, skipped rather than assumed harmless;
// Optional$/OptionQuestion$ (0/0) -- a "may" confirmation this port's own
// PlayerController has no hook for; Radiance$ (0) -- CardUtil.getRadiance's
// own "and everything else that shares a color" fan-out.
var pumpUnresolvedParams = [...]string{
	"SubAbility", "Condition", "ConditionDefined", "ConditionZone", "ConditionPlayerTurn",
	"ConditionActivationLimit", "PlayerTurn", "UnlessCost", "UnlessPayer", "UnlessSwitched",
	"CanBlockAmount", "CanBlockAny", "DefinedKW", "KWChoice", "RandomKeyword", "RandomKWNum",
	"NoRepetition", "SharedKeywordsZone", "SharedRestrictions", "ValidTgts", "AtEOT",
	"DefinedLandwalk", "ForgetObjects", "RememberObjects", "RememberPumped", "LeaveBattlefield",
	"ImprintCards", "ForgetImprinted", "NoteCards", "NoteCardsFor", "ClearNotedCardsFor",
	"NoteNumber", "IsPresent", "Optional", "OptionQuestion", "Radiance",
}

// pumpEffect resolves Mode$/DB$/AB$ Pump for the Defined$ Self/Enchanted/
// Equipped shape -- no target, CR 601.2c's own "no ValidTgts$ on this line"
// reading. ConditionPresent$/ConditionCompare$/ConditionCheckSVar$/
// ConditionSVarCompare$ are resolved through subAbilityConditionMet
// (condition.go), the identical way DealDamage's/GainLife's own do.
type pumpEffect struct{}

func (pumpEffect) Resolve(g *Game, a *Ability, _ PlayerController) error {
	for _, key := range pumpUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: Pump: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}

	permanent := false
	if d, ok := a.Params.Param("Duration"); ok {
		if d != "Permanent" {
			return fmt.Errorf("engine: Pump: Duration$ %q not resolvable yet", d)
		}
		permanent = true
	}

	power, err := pumpAmount(g, "Pump", a, source, "NumAtt")
	if err != nil {
		return err
	}
	toughness, err := pumpAmount(g, "Pump", a, source, "NumDef")
	if err != nil {
		return err
	}

	keywords, err := pumpKeywords("Pump", a.Params)
	if err != nil {
		return err
	}

	if power == 0 && toughness == 0 && len(keywords) == 0 {
		return nil
	}

	defined, _ := a.Params.Param("Defined")
	cards, err := definedCards(source, defined, a.Targets)
	if err != nil {
		return fmt.Errorf("engine: Pump: %w", err)
	}

	g.timestamp++
	timestamp := g.timestamp
	for _, cid := range cards {
		c := g.Card(cid)
		if !pumpZoneMatches(a.Params, c.Zone) {
			continue
		}
		g.pumps = append(g.pumps, pumpRecord{
			Card: cid, Timestamp: timestamp, Power: power, Toughness: toughness,
			Keywords: keywords, Permanent: permanent,
		})
	}
	return nil
}

// pumpAmount reads NumAtt$/NumDef$, defaulting to 0 when the key is absent
// (a KW$-only Pump/PumpAll line, granting no stat change at all). effect
// names the caller (Pump/PumpAll) for the error message. "Double"/"Triple"
// (1 real Pump line, 0 real PumpAll lines -- PumpAllEffect.java's own
// resolve computes NumAtt$/NumDef$ once against the ability's own host, with
// no per-target special case at all) -- the target's own power or toughness
// doubled or tripled, PumpEffect.resolve's own special-cased literal strings
// rather than an SVar name resolveNamedAmount would resolve -- are not
// built.
func pumpAmount(g *Game, effect string, a *Ability, host *Card, key string) (int, error) {
	v, ok := a.Params.Param(key)
	if !ok {
		return 0, nil
	}
	if v == "Double" || v == "Triple" {
		return 0, fmt.Errorf("engine: %s: %s$ %q not resolvable yet", effect, key, v)
	}
	amount, ok := resolveNamedAmount(g, a.Amounts, host, v)
	if !ok {
		return 0, fmt.Errorf("engine: %s: %s$ %q is not resolvable", effect, key, v)
	}
	return amount, nil
}

// pumpZoneMatches reports whether zone is one PumpZone$ permits: absent
// means the Java default, ZoneType.listValueOf("Battlefield") -- Battlefield
// alone -- otherwise zone's own name must appear in the comma list (hasZone,
// trigger.go), the identical shape TriggerZones$ already has for a trigger
// instead of a resolving ability.
func pumpZoneMatches(a *compile.Ability, zone ZoneType) bool {
	if _, ok := a.Param("PumpZone"); !ok {
		return zone == Battlefield
	}
	return hasZone(a, "PumpZone", zone.String())
}

// pumpKeywords reads KW$ -- shared between pumpEffect and pumpAllEffect,
// PumpEffect.java/PumpAllEffect.java's own identical two-line KW$ handling
// (split on " & ", reject a HIDDEN-prefixed token). effect names the caller
// (Pump/PumpAll) for the error message. A KW$ token starting with "HIDDEN"
// (a hidden-keyword phrase, gameCard.addHiddenExtrinsicKeywords -- its own
// separate mechanic) fails loudly rather than granting a normal keyword
// named literally "HIDDEN ...". Absent KW$ (a NumAtt$/NumDef$-only line)
// returns nil, nil -- no keywords granted, not an error.
func pumpKeywords(effect string, a *compile.Ability) ([]string, error) {
	raw, ok := a.Param("KW")
	if !ok {
		return nil, nil
	}
	if strings.Contains(raw, "HIDDEN") {
		return nil, fmt.Errorf("engine: %s: KW$ %q not resolvable yet", effect, raw)
	}
	tokens, ok := keywordTokens(a, "KW")
	if !ok {
		return nil, fmt.Errorf("engine: %s: KW$ %q not resolvable yet", effect, raw)
	}
	return tokens, nil
}
