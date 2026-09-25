// DealDamage: CR 119/120.1, M6's second script-driven effect. Trimmed to the
// corpus's single largest resolvable shape: a plain-or-named-SVar NumDmg$
// dealt to a Defined$ player or the ability's own host, sourced from that
// same host -- 65 of the corpus's 2,219 real (AB|DB)$ DealDamage lines that
// also name Defined$ You/Player.Opponent/Opponent/Self and carry no other
// unresolved param, out of 822 total naming any Defined$ value at all. 3 of
// the 65 also name UnlessCost$ -- resolveUnlessCost's own new gate
// (effect.go) runs ahead of this file entirely now, below. 72 resolve as of
// Planeswalker$ no longer blocking (below) -- CR 606.3's own loyalty-ability
// marker, ActivateAbility's/ActivateManaAbility's own cost-side gate
// (activateability.go), never itself a restriction on how the effect it
// pays for resolves.
//
// Ported from
// forge-game/src/main/java/forge/game/ability/effects/DamageDealEffect.java's
// resolve.

package engine

//enginelint:allow id card game player ability combatdamage defined amount condition control trigger effecthelpers damageresolveeffect

import "fmt"

// dealDamageEffect resolves Mode$/DB$/AB$ DealDamage. Reuses the exact
// damage machinery combat already built: dealPermanentDamage/
// dealPlayerDamage (combatdamage.go) mark the damage, check CR 614's own
// "prevent all of this damage" replacement effects and CR 603's own "deals
// damage" trigger identically whether the source is a blocker or a script --
// isCombat threaded through as false is the one thing that tells the two
// apart (FlagCombat's own doc comment, event.go). A local damageTable
// accumulates every dealPlayerDamage call this one resolution makes (more
// than one when Defined$ names several players at once), consumed by
// checkDamageDoneOnceTriggers (trigger.go) once the whole resolution's own
// damage is dealt -- combatdamage.go's own doc comment on damageTable has
// the reason a single script-driven ability's own damage is one batch too,
// not just a combat damage step's.
//
// Not ported (every one fails loudly rather than dealing the wrong amount to
// the wrong thing, PORT-8/GO-7): DamageSource$ (17 of 822 real Defined$
// lines -- a source other than the ability's own host, needing a reference
// vocabulary this file does not have); ValidTgts$/
// TriggeredSpellAbility$/DamageMap$/CounterNum$/Optional$/TgtPrompt$ (each
// its own further mechanic); NoPrevention$ (1 -- this port's own
// damagePrevented/damagePreventedPlayer would otherwise apply where Java's
// own AbilityKey.NoPreventDamage says not to, a wrong answer rather than a
// missing one). SubAbility$ no longer blocks: resolveSubAbility
// (subability.go) chains it through Registry.Resolve (effect.go) once this
// effect's own body finishes, whether or not subAbilityConditionMet below
// let it run at all -- sword_of_fire_and_ice_and_war_and_peace.txt's own
// real DealDamage-chaining-into-GainLife shape is why. 9 of the corpus's
// own 316 real SVar-defined DealDamage lines naming SubAbility$ chain to an
// already-built leaf ability and resolve end to end. UnlessPayer$/
// UnlessCost$/UnlessResolveSubs$ no longer block either: resolveUnlessCost
// (effect.go) gates the whole ability, this effect's own body included,
// before Registry.Resolve ever reaches it -- force_of_nature.txt's own real
// "deals 8 damage to you unless you pay {G}{G}{G}{G}" is the shape.
//
// ConditionPresent$/ConditionCompare$/ConditionCheckSVar$/
// ConditionSVarCompare$ -- SpellAbilityCondition's own gate on the ability
// itself, distinct from CardTraitBase's own meetsCommonRequirements a
// trigger already has -- are resolved now too (subAbilityConditionMet,
// condition.go), 5 more of the 822 real Defined$ lines: a met condition lets
// this run as normal, an unmet one is not an error, the ability simply does
// nothing (SpellAbilityCondition.areMet's own contract), the identical
// "declined by the rules" outcome PlayLand/PayManaCost already report as a
// plain false/nil rather than an error. Condition$ itself (SpellAbilityCondition's
// own separate Threshold/Metalcraft/... flag switch) and ConditionDefined$
// (an arbitrary reference this port has no Defined$-to-objects resolver for)
// stay in dealDamageUnresolvedParams below, failing loudly the identical way
// DamageSource$ already does -- condition.go's own generic
// subAbilityConditionMet would otherwise silently no-op a card naming either,
// which this file's own established contract (every unresolvable param fails
// loudly by name, never silently) does not allow.
type dealDamageEffect struct{}

var dealDamageUnresolvedParams = [...]string{
	"DamageSource", "Condition", "ConditionDefined",
	"ValidTgts", "TriggeredSpellAbility", "CounterNum",
	"NoPrevention", "Optional", "TgtPrompt",
}

func (dealDamageEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	for _, key := range dealDamageUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: DealDamage: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	numDmg, ok := a.Params.Param("NumDmg")
	if !ok {
		return fmt.Errorf("engine: DealDamage: NumDmg$ missing")
	}
	dmg, ok := resolveNamedAmount(g, a.Amounts, source, numDmg)
	if !ok {
		return fmt.Errorf("engine: DealDamage: NumDmg$ %q is not resolvable", numDmg)
	}
	deathtouch := source.HasKeyword("Deathtouch")
	if hasParam(a, "DamageMap") && a.damageMap == nil {
		a.damageMap = &pendingDamage{}
	}

	defined, _ := a.Params.Param("Defined")
	if a.damageMap != nil {
		// DamageMap$: the damage is recorded, and dealt all at once by a
		// later DamageResolve (DamageDealEffect's usedDamageMap).
		if defined == "Self" {
			a.damageMap.add(a.Source, CardEntity(a.Source), dmg)
			return nil
		}
		players, err := definedPlayers(g, a.Controller, a.Source, defined, a.refs())
		if err != nil {
			return fmt.Errorf("engine: DealDamage: %w", err)
		}
		for _, pid := range players {
			a.damageMap.add(a.Source, PlayerEntity(pid), dmg)
		}
		return nil
	}
	if defined == "Self" {
		var table damageTable
		g.dealPermanentDamage(controller, a.Source, a.Source, dmg, deathtouch, false, &table)
		g.checkDamageTableTriggers(controller, table, false)
		return nil
	}
	players, err := definedPlayers(g, a.Controller, a.Source, defined, a.refs())
	if err != nil {
		return fmt.Errorf("engine: DealDamage: %w", err)
	}
	var table damageTable
	for _, pid := range players {
		g.dealPlayerDamage(controller, a.Source, pid, dmg, false, &table)
	}
	g.checkDamageTableTriggers(controller, table, false)
	return nil
}
