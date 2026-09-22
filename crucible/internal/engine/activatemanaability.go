// Activating a mana ability past a basic land's own intrinsic one: CR 605.3,
// the corpus's own real A:AB$ Mana lines a card actually prints (rocks,
// dorks, Treasures, ...) -- TapLandForMana (manaability.go) is CR 305.6's
// synthesized version of the identical rule for the one case that needs no
// script line at all.
//
// A mana ability resolves immediately, with no stack (CR 605.3a) -- unlike
// ActivateAbility (activateability.go), this port's own CR 601.3a
// sorcery-speed simplification does not apply here at all. TapLandForMana
// already carries no phase restriction of its own -- paying for something is
// the caller's own job (a fixture, or PayManaCost's own eventual caller),
// not a turn-structure decision this port's own missing priority window
// would otherwise gate -- and ActivateManaAbility keeps the identical
// contract for every other permanent's own printed mana ability.
package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/cost"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// manaAbilityAllowedParams names every Key$ ActivateManaAbility actually
// reads or can safely ignore -- SpellDescription$ describes the ability to a
// human and this port never renders it. Any other key present fails the
// whole line loudly rather than producing the wrong mana or ignoring a real
// restriction (PORT-8/GO-7): RestrictValid$ (53 real lines clearing every
// other gate here) is a mana-pool spending restriction this port's own Pool
// has no way to tag mana with; SubAbility$ (28) chains a further ability
// this immediate, no-stack resolution has nowhere to route through
// Registry.Resolve; IsPresent$/ConditionCheckSVar$/ConditionSVarCompare$/
// PresentCompare$/CheckSVar$/SVarCompare$/OpponentTurn$ (26 combined) are an
// "activate only if..." restriction this port has no general resolving-
// ability gate for (distinct from subAbilityConditionMet's own Condition-
// prefixed pair, which a *resolving* Mana ability never carries in the real
// corpus); TriggersWhenSpent$/AddsKeywords$/AddsKeywordsValid$/
// AddsKeywordsUntil$ (16 combined) tag the mana itself with a further effect
// this port's own Pool cannot carry; AILogic$/AINoRecursiveCheck$/
// PrecostDesc$/Activation$ (13 combined) are AI hinting or a further
// "Activate only as..." cost-description gate, neither read here.
var manaAbilityAllowedParams = map[string]bool{
	"ab": true, "cost": true, "spelldescription": true, "produced": true, "amount": true,
}

// manaAbilityParamsResolvable reports whether a is nothing but the params
// manaAbilityAllowedParams admits.
func manaAbilityParamsResolvable(a *compile.Ability) bool {
	for _, p := range a.Params {
		if !manaAbilityAllowedParams[strings.ToLower(p.Key)] {
			return false
		}
	}
	return true
}

// producedManaColor reads Produced$'s own literal single-symbol shape -- one
// of WUBRG, or C for colorless -- 1,005 of the corpus's own 2,156 real
// A:AB$ Mana lines, the dominant shape past "Any" (CR 605.3b's own "choose a
// color," resolved separately in ActivateManaAbility through
// ChooseManaColor) and "Combo <letters>" (parseComboColors, below, a
// restricted-set version of the identical choice). "Chosen" (a color picked
// earlier in the same resolution, an SVar-like reference this port does not
// follow) stays unresolved.
func producedManaColor(produced string) (color mana.Colors, colorless bool, ok bool) {
	switch produced {
	case "W":
		return mana.White, false, true
	case "U":
		return mana.Blue, false, true
	case "B":
		return mana.Black, false, true
	case "R":
		return mana.Red, false, true
	case "G":
		return mana.Green, false, true
	case "C":
		return 0, true, true
	}
	return 0, false, false
}

// parseComboColors reads Produced$'s own "Combo <letters>" shape -- CR
// 605.3b's own restricted-choice version of "Any," a dual/tri-land's real
// "Add W or U"/"Add G, U, or R" -- into the set of colors offered. Every
// token past "Combo" must be a single literal WUBRG letter (a repeat is
// harmless -- options is a set, ORing the same bit twice changes nothing);
// anything else -- "Combo Any"/"Combo AnyDifferent" (CR 605.3b's own
// "in any combination of colors," a per-unit independent choice this
// single-color-per-activation dispatch does not model), "ColorIdentity"
// (Commander's own color-identity set, a format concept this port does not
// track), a "Chosen" token (producedManaColor's own identical unresolved
// reference) -- fails the whole match rather than guessing a subset
// (PORT-8/GO-7).
func parseComboColors(produced string) (mana.Colors, bool) {
	tokens := strings.Fields(produced)
	if len(tokens) < 2 || tokens[0] != "Combo" {
		return 0, false
	}
	var options mana.Colors
	for _, tok := range tokens[1:] {
		if len(tok) != 1 {
			return 0, false
		}
		color, ok := mana.ColorFromLetter(tok[0])
		if !ok {
			return 0, false
		}
		options |= color
	}
	return options, true
}

// ActivateManaAbility is CR 605.3: pay index's own Cost$, then add
// Produced$'s own mana to the pool at once, no stack involved. index selects
// among card's own compiled `A:` lines by position, the identical
// "caller already knows the card's own script" contract ActivateAbility
// already has.
//
// Reports whether the mana was produced. false covers not pid's own
// permanent, not on the battlefield, index not naming an Activated API
// "Mana" line at all, a param past manaAbilityAllowedParams, a Cost$ past
// ActivationShape (internal/cost, ActivateAbility's own identical gate,
// reused outright -- ActivateAbility itself refuses API "Mana" and this
// function refuses anything else, so the two never overlap) -- including a
// Discard component, which this function declines outright rather than
// silently skipping (PORT-8/GO-7): 0 real corpus A:AB$ Mana lines name
// Discard<...> at all, so ActivateAbility's own Discard payment
// (activateability.go) has nothing here to reuse, and letting the shape
// through unhandled would mean claiming the cost was paid in full while
// never actually discarding anything -- a Tap-self cost declined by CR
// 602.5b/302.6 (SummonSick/Haste,
// DeclareCombatAttackers' own gate, reused), a Produced$ past
// producedManaColor's own literal shape, "Any" or parseComboColors' own
// literal-letters-only "Combo" shape, ChooseManaColor answering with
// anything but exactly one color from the set it was offered -- not
// re-checked by the interface itself (ChooseManaColor's own doc comment,
// control.go), so this is where that trust ends rather than at
// [Pool.Add]'s own panic -- an Amount$ that does not resolve to a positive
// integer (resolveNamedAmount, amount.go -- pumpAmount's own identical
// plain-integer-or-SVar reading), or an unaffordable mana half of the cost.
//
// Payment order matches ActivateAbility's own: mana first, tap second,
// self-sac last (activateability.go's own doc comment has the CR 601.2h
// reasoning), reusing sacrificeCards (sacrificeeffect.go) the identical way.
// A Tap-self cost also fires CR 603's own "taps for mana" trigger
// (checkTapsForManaTriggers, trigger.go) alongside the ordinary "becomes
// tapped" one -- TapLandForMana's own pairing, ported here rather than
// duplicated, and skipped when the cost has no Tap component at all (a
// Treasure-style pure self-sac cost taps nothing, so nothing "becomes
// tapped to produce mana").
func (g *Game) ActivateManaAbility(pid PlayerID, card CardID, index int, controller PlayerController) bool {
	c := g.Card(card)
	if c.Controller() != pid || c.Zone != Battlefield {
		return false
	}
	abilities := c.Def.Faces[0].Abilities
	if index < 0 || index >= len(abilities) {
		return false
	}
	ability := abilities[index]
	if ability.Record != compile.Activated || ability.Name != "Mana" {
		return false
	}
	if !manaAbilityParamsResolvable(ability) {
		return false
	}
	costText, ok := ability.Param("Cost")
	if !ok {
		return false
	}
	parsed := cost.Parse(costText)
	shape, ok := parsed.ActivationShape()
	if !ok || shape.DiscardN > 0 {
		return false
	}
	if shape.Tap && (c.Tapped || (c.SummonSick && !c.HasKeyword("Haste"))) {
		return false
	}
	produced, ok := ability.Param("Produced")
	if !ok {
		return false
	}
	var color mana.Colors
	var colorless bool
	switch {
	case produced == "Any":
		color = controller.ChooseManaColor(g, pid, card, mana.AllColors)
		if color.Count() != 1 {
			return false
		}
	case strings.HasPrefix(produced, "Combo "):
		options, comboOK := parseComboColors(produced)
		if !comboOK {
			return false
		}
		color = controller.ChooseManaColor(g, pid, card, options)
		if color.Count() != 1 || !options.Has(color) {
			return false
		}
	default:
		color, colorless, ok = producedManaColor(produced)
		if !ok {
			return false
		}
	}
	amount := 1
	if amountText, hasAmount := ability.Param("Amount"); hasAmount {
		amount, ok = resolveNamedAmount(g, c.Def.Faces[0].Amounts, c, amountText)
		if !ok || amount <= 0 {
			return false
		}
	}
	costMana, err := mana.Parse(strings.Join(parsed.Mana, " "))
	if err != nil {
		return false
	}
	if !g.PayManaCost(pid, costMana, controller) {
		return false
	}
	if shape.Tap {
		c.Tapped = true
		g.checkTapsTriggers(controller, card, pid, false)
	}
	if shape.SelfSac {
		sacrificeCards(g, controller, &Ability{Source: card, Controller: pid, Params: ability}, []CardID{card})
	}

	snow := c.Type().HasSupertype(cardtype.Snow)
	pool := &g.Player(pid).ManaPool
	switch {
	case colorless && snow:
		pool.AddSnowColorless(amount)
	case colorless:
		pool.AddColorless(amount)
	case snow:
		pool.AddSnow(color, amount)
	default:
		pool.Add(color, amount)
	}

	if shape.Tap {
		g.checkTapsForManaTriggers(controller, card, pid)
	}
	return true
}
