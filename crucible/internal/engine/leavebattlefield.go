// LeaveBattlefield$ and Incorporate$ of the Animate family.
//
// LeaveBattlefield$ (SpellAbilityEffect.addLeaveBattlefieldReplacement,
// SpellAbilityEffect.java:618-650): "if it would leave the battlefield, exile it
// instead of putting it anywhere else", for the card an Animate, Pump or
// ChangeZone just changed. Java makes an effect card in the Command zone that
// remembers the card and carries an Event$ Moved replacement for it, and has it
// forget the card once it leaves the battlefield (addExileOnMovedTrigger); this
// builds the same tree directly (PORT-2). The replacement is read by
// movedDestination.

//enginelint:allow id zone card game ability effecteffect effecthelpers replacedying

package engine

import (
	"fmt"
	"slices"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/carddb/vocab"
	"github.com/jczastkiewicz/crucible/internal/keyword"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// leaveBattlefieldZone reads LeaveBattlefield$: the zone the card goes to
// instead. Only Exile has a corpus line; another zone is refused (GO-7).
func leaveBattlefieldZone(a *Ability, api string) (zone string, has bool, err error) {
	zone, has = a.Params.Param("LeaveBattlefield")
	if has && zone != "Exile" {
		return "", true, fmt.Errorf("engine: %s: LeaveBattlefield$ %q not resolvable yet", api, zone)
	}
	return zone, has, nil
}

// addLeaveBattlefieldReplacement installs the effect card for card, which must
// be on the battlefield; a card that has left already has nothing to replace.
func (g *Game) addLeaveBattlefieldReplacement(a *Ability, card CardID, zone string) {
	g.leaveBattlefieldEffect(a, card, zone, effectHostName(g.Card(a.Source))+"'s Effect")
}

// unearthReturned is ChangeZoneEffect.java:680-698: a card an Unearth ability
// (the keyword.Expand template, CardFactoryUtil.java:3636-3648) put onto the
// battlefield gains haste, is exiled instead if it would leave the
// battlefield (one effect card carries both), and is exiled at the beginning
// of the next end step. Card.setUnearthed has no reader outside a Vanguard
// avatar (Malfegor Avatar's IsUnearthed), so it is not kept.
func (g *Game) unearthReturned(a *Ability, card CardID) error {
	if keyword.Parse(a.Params.Keyword).Name != "Unearth" || g.Card(card).Zone != Battlefield {
		return nil
	}
	haste := &compile.Ability{Record: compile.Static, Name: "Continuous", Params: []vocab.Param{
		{Key: "Mode", Value: "Continuous"}, {Key: "Affected", Value: "Card.IsRemembered"},
		{Key: "EffectZone", Value: "Command"}, {Key: "AddKeyword", Value: "Haste"},
	}}
	g.leaveBattlefieldEffect(a, card, "Exile", "Unearth Effect", haste)
	return g.registerDelayedLeave(a.Source, a.Controller, a.Amounts, "ChangeZone", "Exile", "", a.Params.Keyword, []CardID{card})
}

// leaveBattlefieldEffect makes the effect card named name that remembers card
// and replaces its leaving the battlefield with a move to zone, with statics
// beside the replacement.
func (g *Game) leaveBattlefieldEffect(a *Ability, card CardID, zone, name string, statics ...*compile.Ability) {
	if g.Card(card).Zone != Battlefield {
		return
	}
	send := &compile.Ability{Record: compile.SubAbility, Name: "ChangeZone", Params: []vocab.Param{
		{Key: "DB", Value: "ChangeZone"}, {Key: "Defined", Value: "ReplacedCard"},
		{Key: "Origin", Value: "Battlefield"}, {Key: "Destination", Value: zone},
	}}
	moved := &compile.Ability{Record: compile.Replacement, Name: "Moved", Params: []vocab.Param{
		{Key: "Event", Value: "Moved"}, {Key: "ValidCard", Value: "Card.IsRemembered"},
		{Key: "Origin", Value: "Battlefield"}, {Key: "ExcludeDestination", Value: zone},
		{Key: "Description", Value: "If it would leave the battlefield, exile it instead of putting it anywhere else."},
	}, Subs: []compile.SubRef{{Key: "ReplaceWith", Ability: send}}}
	def := &compile.Card{Name: name}
	face := &def.Faces[0]
	face.Name = name
	face.Amounts = a.Amounts
	face.Replacements = []*compile.Ability{moved}
	face.Statics = statics
	id := g.NewCard(def, a.Controller, Command)
	eff := g.Card(id)
	eff.IsEffect = true
	eff.effectLife = effectLifetime{duration: effectPermanent, player: a.Controller, host: a.Source, forgetOnMoved: zoneMask(1) << Battlefield}
	eff.Memory.Remember(CardEntity(card))
}

// leaveBattlefieldFor installs LeaveBattlefield$'s replacement for each of
// cards still on the battlefield, nothing for a line without the param.
func (g *Game) leaveBattlefieldFor(a *Ability, api string, cards []CardID) error {
	zone, has, err := leaveBattlefieldZone(a, api)
	if err != nil || !has {
		return err
	}
	for _, id := range cards {
		g.addLeaveBattlefieldReplacement(a, id, zone)
	}
	return nil
}

// incorporateInto is AnimateEffectBase.doAnimate's ManaCost$ (:103-110) and
// Incorporate$ (:92-98, an Alchemy "incorporate" cost): each card still in a zone gains the cost for
// good, its mana cost the printed one plus it and its colors the cost's.
func (g *Game) incorporateInto(a *Ability, api string, cards []CardID) error {
	if raw, ok := a.Params.Param("ManaCost"); ok {
		// AnimateEffectBase.java:103-110: only a Perpetual Animate changes the
		// cost; any other Duration$ does nothing with it.
		if d, _ := a.Params.Param("Duration"); d == "Perpetual" {
			cost, err := mana.Parse(raw)
			if err != nil {
				return fmt.Errorf("engine: %s: ManaCost$ %q: %w", api, raw, err)
			}
			for _, id := range cards {
				if c := g.Card(id); c.Zone != None && !c.IsPhasedOut() {
					c.costOverride = &cost
				}
			}
		}
	}
	raw, ok := a.Params.Param("Incorporate")
	if !ok {
		return nil
	}
	inc, err := mana.Parse(raw)
	if err != nil {
		return fmt.Errorf("engine: %s: Incorporate$ %q: %w", api, raw, err)
	}
	for _, id := range cards {
		if c := g.Card(id); c.Zone != None && !c.IsPhasedOut() {
			c.incorporated = append(slices.Clone(c.incorporated), inc)
		}
	}
	return nil
}
