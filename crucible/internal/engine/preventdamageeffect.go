package engine

//enginelint:allow control ability game effecthelpers card condition amount id defined zone additional

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/expr"
)

// preventDamageEffect is DamagePreventEffect.java: each targeted (or
// Defined$) player, and each targeted card still on the battlefield, gets a
// shield preventing the next Amount$ damage that would be dealt to it this
// turn. Java builds a command-zone effect per target holding a DamageDone
// replacement with PreventionEffect$ NextN, which drops away at end of turn
// and, for a card, when it leaves the battlefield; this port keeps the same
// shield as a preventShield record (applyPreventShields). A
// DividedAsYouChoose$ target gets a shield of its own share, announced with
// the targets (DamagePreventEffect.java:105, perTargetAmounts):
// Remedy's "prevent the next 5 damage ... to any number of targets, divided
// as you choose". PreventionSubAbility$ is the shield's follow-up, run each
// time the shield prevents damage with PreventedDamage the amount that
// event prevented (ReplaceDamageEffect.java:37-58, a per-event count), and
// ShieldEffectTarget$ the entity it reads as Defined$ ShieldEffectTarget,
// frozen when the shield is made (:141-152). CardChoices$/PlayerChoices$ are
// not resolved.
type preventDamageEffect struct{}

func (preventDamageEffect) Resolve(g *Game, a *Ability, _ PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	if err := rejectParams(a, "PreventDamage", "CardChoices", "PlayerChoices"); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	raw, ok := a.Params.Param("Amount")
	if !ok {
		return fmt.Errorf("engine: PreventDamage: Amount$ missing")
	}
	n, ok := resolveNamedAmount(g, a.Amounts, source, raw)
	if !ok {
		return fmt.Errorf("engine: PreventDamage: Amount$ %q not resolvable", raw)
	}
	var targets []EntityID
	if hasParam(a, "ValidTgts") {
		targets = a.Targets
	} else {
		def, ok := a.Params.Param("Defined")
		if !ok {
			def = "Self"
		}
		var err error
		targets, err = definedEntities(g, a.Controller, source, def, a.refs())
		if err != nil {
			return fmt.Errorf("engine: PreventDamage: %w", err)
		}
	}
	amounts, err := a.perTargetAmounts("PreventDamage", targets, n)
	if err != nil {
		return err
	}
	tmpl, err := a.shieldTemplate(g, source)
	if err != nil {
		return err
	}
	for i, t := range targets {
		if id, isCard := t.AsCard(); isCard && g.Card(id).Zone != Battlefield {
			continue
		}
		s := tmpl
		s.Target, s.Remaining = t, amounts[i]
		g.preventShields = append(g.preventShields, s)
	}
	// Radiance$ (DamagePreventEffect.java:102, :115-119).
	for _, id := range g.radianceCards(a) {
		s := tmpl
		s.Target, s.Remaining = CardEntity(id), n
		g.preventShields = append(g.preventShields, s)
	}
	return nil
}

// shieldTemplate is what every shield of a carries beside its target and size:
// the PreventionSubAbility$ chain with its host's amounts, and the
// ShieldEffectTarget$ entity it reads, resolved now against a (the last one
// named wins, DamagePreventEffect.java:144-150, quirks.md). A script naming
// ShieldEffectTarget$ with nothing behind it fails instead of running a
// Defined$ with nothing in it (:152).
func (a *Ability) shieldTemplate(g *Game, source *Card) (preventShield, error) {
	var s preventShield
	subs := additionalAbilities(a.Params, "PreventionSubAbility")
	if len(subs) == 0 {
		if hasParam(a, "ShieldEffectTarget") {
			return s, fmt.Errorf("engine: PreventDamage: ShieldEffectTarget$ without PreventionSubAbility$ not resolvable yet")
		}
		return s, nil
	}
	s.sub, s.source, s.controller, s.amounts = &subs[0], a.Source, a.Controller, a.Amounts
	if raw, ok := a.Params.Param("ShieldEffectTarget"); ok {
		ents, err := definedEntities(g, a.Controller, source, raw, a.refs())
		if err != nil {
			return s, fmt.Errorf("engine: PreventDamage: ShieldEffectTarget$: %w", err)
		}
		if len(ents) == 0 {
			return s, fmt.Errorf("engine: PreventDamage: ShieldEffectTarget$ %q names nothing", raw)
		}
		s.entity = ents[len(ents)-1]
	}
	return s, nil
}

// preventShield is one "prevent the next N damage" shield: what is left of
// it, and what it protects.
type preventShield struct {
	Target    EntityID
	Remaining int
	// sub is PreventionSubAbility$'s chain; nil for a plain shield. source and
	// controller are the spell's, amounts its script's SVars, entity the frozen
	// ShieldEffectTarget$.
	sub        *compile.SubRef
	source     CardID
	controller PlayerID
	amounts    map[string]expr.Amount
	entity     EntityID
}

// shieldRun is one prevention a shield's sub-ability answers: prevented is
// that event's prevented amount (PreventedDamage).
type shieldRun struct {
	shield    preventShield
	prevented int
}

// applyPreventShields spends target's shields, oldest first, against amount
// damage, answering what is left. A spent shield is removed.
//
// A shield with a PreventionSubAbility$ answers it once per event it prevented
// something in, after the shields are settled: the chain can deal damage of its
// own (Acolyte's Reward), which must see the spent shield.
func (g *Game) applyPreventShields(controller PlayerController, target EntityID, amount int) int {
	kept := g.preventShields[:0]
	var runs []shieldRun
	for _, s := range g.preventShields {
		if s.Target == target && amount > 0 {
			used := s.Remaining
			if used > amount {
				used = amount
			}
			amount -= used
			s.Remaining -= used
			if s.sub != nil && used > 0 {
				runs = append(runs, shieldRun{shield: s, prevented: used})
			}
		}
		if s.Remaining > 0 {
			kept = append(kept, s)
		}
	}
	g.preventShields = kept
	for _, r := range runs {
		if err := g.runShieldSub(controller, r); err != nil {
			g.recordPendingError(err)
		}
	}
	return amount
}

// runShieldSub resolves a shield's PreventionSubAbility$ chain as the
// replacement's own ability (ReplaceDamageEffect.java:37-58): its host's
// amounts plus PreventedDamage, Defined$ ShieldEffectTarget the frozen target.
func (g *Game) runShieldSub(controller PlayerController, r shieldRun) error {
	s := r.shield
	api, ok := APIByName(s.sub.Ability.Name)
	if !ok {
		return fmt.Errorf("engine: PreventionSubAbility$ %s: unrecognized API %q", s.sub.SVar, s.sub.Ability.Name)
	}
	if g.registry == nil {
		return fmt.Errorf("engine: PreventionSubAbility$ %s: no Registry is resolving", s.sub.SVar)
	}
	amounts := make(map[string]expr.Amount, len(s.amounts)+1)
	for k, v := range s.amounts {
		amounts[k] = v
	}
	amounts[preventedDamageSVar] = expr.Amount{Kind: expr.Literal, Value: r.prevented}
	child := Ability{API: api, Source: s.source, Controller: s.controller, Params: s.sub.Ability, Amounts: amounts,
		shieldTarget: s.entity}
	reg := g.registry
	err := reg.Resolve(g, &child, controller)
	g.registry = reg
	return err
}

// preventedDamageSVar is the SVar ReplaceDamageEffect writes on the shield's
// effect card (PreventedDamage=Number$n); amounts are keyed lower-cased.
const preventedDamageSVar = "preventeddamage"

// dropPreventShields removes every shield on card id (its effect's
// forget-on-moved trigger, when it leaves the battlefield).
func (g *Game) dropPreventShields(id CardID) {
	kept := g.preventShields[:0]
	for _, s := range g.preventShields {
		if s.Target != CardEntity(id) {
			kept = append(kept, s)
		}
	}
	g.preventShields = kept
}
