// Mutate (CR 702.140): "You may cast this spell for its mutate cost by
// targeting a non-Human creature you own. If it resolves, it merges with that
// creature" -- one permanent whose characteristics are the top component's
// and whose abilities are every component's (ADR-0047).
//
// Ported from forge-game/src/main/java/forge/game/card/CardFactoryUtil.java
// (the Mutate keyword branch, :3144-3159), ability/effects/MutateEffect.java,
// card/CardFactory.java's getMutatedCloneStates (:757-783), the fizzle branch
// of zone/MagicStack.java (:628-633) and GameAction.changeZone's split on
// leaving the battlefield.

package engine

//enginelint:allow id zone card game control effecthelpers player ability staticability targeting triggerscan cloneeffect castoptions castspell event trigger

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/expr"
	"github.com/jczastkiewicz/crucible/internal/keyword"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// mutateCost is the mana cost of c's Mutate keyword. ok is false without the
// keyword or for a cost that is not plain mana without X: such a Mutate is not
// offered, never cast for a guessed cost (GO-7).
func mutateCost(c *Card) (mana.Cost, bool) {
	for _, line := range c.KeywordLines() {
		k := keyword.Parse(line)
		if k.Name != "Mutate" {
			continue
		}
		if k.Details == "" || strings.Contains(k.Details, ":") {
			return mana.Cost{}, false
		}
		mc, err := mana.Parse(k.Details)
		if err != nil || mc.CountX() > 0 {
			return mana.Cost{}, false
		}
		return mc, true
	}
	return mana.Cost{}, false
}

// mutateHostOK is the part of "Creature.sharesOwnerWith+nonHuman"
// (CardFactoryUtil.java:3151) that reads the host, plus the shapes this port
// does not merge into and so never offers (ADR-0047): a face-down,
// stickered or copied permanent. A permanent that is a copy only because a
// mutate merged into it already is fine.
func mutateHostOK(t *Card, owner PlayerID) bool {
	if t.Zone != Battlefield || t.Owner != owner || !t.Type().Has(cardtype.Creature) || t.Type().HasSubtype("Human") {
		return false
	}
	if t.IsFaceDown() || len(t.stickers) > 0 {
		return false
	}
	if len(t.merged) > 0 {
		return len(t.copies) == 1
	}
	return len(t.copies) == 0
}

// mutateTargets is every creature pid may target casting card for its mutate
// cost: a non-Human creature on the battlefield owned by card's owner.
func (g *Game) mutateTargets(pid PlayerID, card CardID) []CardID {
	spell := g.Card(card)
	ask := targetAsk{kind: causeSpell, casting: true}
	var out []CardID
	for _, p := range g.Players() {
		for _, id := range g.Zone(Battlefield, p).Cards() {
			t := g.Card(id)
			if mutateHostOK(t, spell.Owner) && !cardCantBeTargetedBy(g, t, pid, card, ask) {
				out = append(out, id)
			}
		}
	}
	return out
}

// mutateOption is the Mutate way to cast c from hand, offered while a creature
// can be targeted. A copy of a card (token) never has one.
func (g *Game) mutateOption(pid PlayerID, c *Card) (castOption, bool) {
	mc, ok := mutateCost(c)
	if !ok || !castableAsPermanent(c) || !c.Type().Has(cardtype.Creature) || len(g.mutateTargets(pid, c.ID)) == 0 {
		return castOption{}, false
	}
	return castOption{hasAlt: true, alt: mc, mutate: true}, true
}

// castMutate is the cast of a creature spell for its mutate cost: the target is
// chosen as the spell is announced (CR 601.2c), the cost paid, and the spell
// goes on the stack as a permanent spell carrying the target.
func (g *Game) castMutate(pid PlayerID, card CardID, c *Card, controller PlayerController, opts castOpts) bool {
	eligible := g.mutateTargets(pid, card)
	if len(eligible) == 0 {
		return false
	}
	target := eligible[0]
	if len(eligible) > 1 {
		ents := make([]EntityID, len(eligible))
		for i, id := range eligible {
			ents[i] = CardEntity(id)
		}
		chosen := controller.ChooseTargets(g, pid, ents, 1, 1)
		if len(chosen) != 1 {
			return false
		}
		id, ok := chosen[0].AsCard()
		if !ok || !slices.Contains(eligible, id) {
			g.recordPendingError(fmt.Errorf("engine: Mutate: %v is not a legal target", chosen[0]))
			return false
		}
		target = id
	}
	if !g.optionalPartsPayable(pid, card, opts.optional) {
		return false
	}
	x, paid := g.payCastCost(pid, c, controller, opts)
	if !paid || !g.payOptionalParts(controller, pid, card, opts.optional) {
		return false
	}
	g.putSpellOnStack(card, pid)
	cast := Ability{API: APIPermanentCreature, Source: card, Controller: pid, Target: target, spell: true, mutate: true}
	x.setOn(&cast)
	g.Card(card).castX = x.value
	g.PushAbility(cast)
	g.sink.Emit(Event{Kind: SpellCast, Phase: g.activePhase, Active: g.activePlayer, Actor: pid, Turn: uint16(g.turn), Source: card})
	g.recordSpellCast(pid, card)
	g.checkSpellCastTriggers(controller, card, pid, append(g.castExitMatches(), g.expendMatches(pid, len(x.sources))...))
	tgts := []EntityID{CardEntity(target)}
	matches := g.checkBecomesTargetTriggers(tgts, true, pid, card)
	matches = append(matches, g.checkWardTriggers(tgts, card, pid)...)
	matches = append(matches, g.crimeTriggers(cast)...)
	g.pushTriggeredAbilities(controller, matches)
	return true
}

// resolveMutate is MutateEffect.resolve for the spell a. It reports false when
// the target is no longer legal: the spell then resolves as a plain creature
// spell (MagicStack.java:628-633, CR 702.140b).
func (g *Game) resolveMutate(a *Ability, controller PlayerController) (bool, error) {
	spell := g.Card(a.Source)
	t := g.Card(a.Target)
	if a.Target == NoCard || !mutateHostOK(t, spell.Owner) ||
		cardCantBeTargetedBy(g, t, a.Controller, a.Source, targetAsk{kind: causeSpell}) {
		return false, nil
	}
	if spell.IsCopiedSpell {
		return false, fmt.Errorf("engine: Mutate: a copy of a mutate spell not resolvable yet")
	}
	view := []CardID{a.Source, a.Target}
	top := controller.ChooseCardsForEffect(g, a.Controller, a.Source, view, 1, 1)
	if err := checkChoice(top, view, 1, 1); err != nil {
		return true, fmt.Errorf("engine: Mutate: %w", err)
	}
	g.mergeInto(a.Source, a.Target, top[0] == a.Source)
	g.pushTriggeredAbilities(controller, g.mutatesMatches(a.Target))
	return true, nil
}

// mergeInto is the body of MutateEffect.resolve past the choice: the spell card
// joins the target's component list on top or below, the target takes the
// merged characteristics as one Layer 1 copy effect, and the spell card leaves
// for its owner's Merged zone.
func (g *Game) mergeInto(spell, target CardID, onTop bool) {
	t := g.Card(target)
	if len(t.merged) == 0 {
		t.merged = []CardID{target}
	}
	defs := make([]*compile.Card, 0, len(t.merged)+1)
	parts := slices.Clone(t.merged)
	if onTop {
		parts = append([]CardID{spell}, parts...)
	} else {
		parts = append(parts, spell)
	}
	for _, id := range parts {
		defs = append(defs, g.mergedComponentDef(id, target))
	}
	merged := mergedDef(defs)
	t.merged = parts
	if len(t.copies) > 0 {
		t.setCopies(nil)
	}
	g.timestamp++
	t.addCopy(copyEffect{Timestamp: g.timestamp, def: merged, permanent: true})
	g.Card(spell).mergedTo = target
	g.Move(spell, Merged, g.Card(spell).Owner)
	t.timesMutated++
}

// mergedComponentDef is the definition component id brings to the merge: its
// own printed one, and for the permanent the one under the merged copy effect.
func (g *Game) mergedComponentDef(id, target CardID) *compile.Card {
	c := g.Card(id)
	if id == target && len(c.copies) > 0 {
		return c.uncopiedDef
	}
	return c.Def
}

// mergedDef is CardFactory.getMutatedCloneStates: the top component's
// definition with the abilities, triggers, statics, replacements and keywords
// of every other component added (CardState.addAbilitiesFrom). The slices
// are new, the nodes shared. On an SVar clash the top's wins.
func mergedDef(defs []*compile.Card) *compile.Card {
	out := *defs[0]
	face := out.Faces[0]
	amounts := make(map[string]expr.Amount, len(face.Amounts))
	for _, d := range defs[1:] {
		for k, v := range d.Faces[0].Amounts {
			amounts[k] = v
		}
	}
	for k, v := range face.Amounts {
		amounts[k] = v
	}
	face.Abilities = slices.Clone(face.Abilities)
	face.Triggers = slices.Clone(face.Triggers)
	face.Statics = slices.Clone(face.Statics)
	face.Replacements = slices.Clone(face.Replacements)
	face.Keywords = slices.Clone(face.Keywords)
	for _, d := range defs[1:] {
		f := d.Faces[0]
		face.Abilities = append(face.Abilities, f.Abilities...)
		face.Triggers = append(face.Triggers, f.Triggers...)
		face.Statics = append(face.Statics, f.Statics...)
		face.Replacements = append(face.Replacements, f.Replacements...)
		face.Keywords = append(face.Keywords, f.Keywords...)
	}
	face.Amounts = amounts
	out.Faces[0] = face
	return &out
}

// moveMerged moves a merged permanent off the battlefield: the permanent and
// every component in the Merged zone go to the destination, each to its own
// owner's zone (GameAction.java:400-528). Deviation: top first, not in an order
// the owner chooses, and only the permanent's own dies/leaves triggers fire.
func (g *Game) moveMerged(id CardID, kind ZoneType, owner PlayerID) CardID {
	t := g.Card(id)
	parts := t.merged
	t.merged, t.timesMutated = nil, 0
	res := g.Move(id, kind, owner)
	for _, p := range parts {
		if p == id {
			continue
		}
		g.Card(p).mergedTo = NoCard
		g.Move(p, kind, g.Card(p).Owner)
	}
	return res
}

// mutatesParams are the params a Mutates trigger may carry that are evaluated.
var mutatesParams = map[string]bool{
	"mode": true, "execute": true, "triggerdescription": true, "triggerzones": true, "optionaldecider": true,
	"secondary": true, "validcard": true,
}

// mutatesMatches is TriggerType.Mutates for target (MutateEffect.java:100),
// once for the whole merge.
func (g *Game) mutatesMatches(target CardID) []Ability {
	t := g.Card(target)
	return g.scanTriggers([]string{"Mutates"}, func(h *Card, _ triggerFace, tr *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(tr, mutatesParams) || !g.triggerCardMatches(h, tr, "ValidCard", t) {
			return triggeredObjects{}, false
		}
		return triggeredObjects{card: target}, true
	})
}
