// Sacrifice: CR 701.20, SacrificeEffect.java's own resolve -- 522 of the
// corpus's 792 real (AB|DB)$ Sacrifice lines, 51 of them past
// resolveUnlessCost's own new gate (effect.go, below), 6 of them naming
// Planeswalker$ too, no longer blocked (below): CR 606.3's own
// loyalty-ability restriction is a cost-side gate (activateability.go),
// never a restriction on how the effect it pays for resolves. The
// dominant real shape is
// SacValid$ (569 of 792), not the "no SacValid$ at all" default this port's
// own effects usually resolve first: SacValid$ absent or the literal value
// "Self" sacrifices the ability's own host card outright, no choice asked
// (Java's own `valid.equals("Self")` branch); every other SacValid$ value
// asks each of Defined$'s players (default "You", AbilityUtils.
// getDefinedPlayers's own null default) to choose Amount$ of their own
// battlefield permanents matching it -- Java's own
// `p.getController().choosePermanentsToSacrifice` branch.
//
// Ported from
// forge-game/src/main/java/forge/game/ability/effects/SacrificeEffect.java's
// resolve; GameAction.sacrifice/sacrificeDestroy for the actual zone
// change, shared with the "Self" branch through sacrificeCards, below.

package engine

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/valid"
)

// sacrificeUnresolvedParams names SacrificeEffect.resolve's own params this
// port does not evaluate. Every one fails the whole line loudly rather than
// sacrificing the wrong permanent, the wrong count, or silently skipping a
// choice (PORT-8/GO-7): ChangeNum$ (5) --
// ChangeZone's param, never read by this ApiType at all, so its
// presence marks a line this port would misclassify rather than one it can
// safely ignore; ValidCard$ (3) -- SacrificeEffect.java
// never reads this key at all, so its real meaning on the handful of lines
// naming it is unclear; Random$ (1) -- Aggregates.random, a randomized choice this
// port's own ChoosePermanentsToSacrifice contract does not carry. Destroy$
// (CR 701.7) and SacEachValid$ resolve (below). StrictAmount$
// resolves: "sacrifice nothing rather than fewer than Amount$" (Lotus Vale).
//
// Echo$ and CumulativeUpkeep$ are SacrificeEffect.java's two leading branches,
// "sacrifice unless you pay" for an upkeep trigger (CR 702.30, 702.24): the
// controller is asked to pay the cost (Cumulative upkeep's, multiplied by the
// AGE counters it now holds, one more each time) and the permanent is
// sacrificed when it is not paid, unless its controller has changed since the
// trigger. Not ported: the PayEcho/PayCumulativeUpkeep triggers each fires (5
// corpus cards), and the player keyword "You may pay 0 rather than pay the
// echo cost" (1 card).
//
// SubAbility$ chains through resolveSubAbility (subability.go,
// Registry.Resolve, effect.go) once this effect's own body finishes,
// whether or not subAbilityConditionMet let it run at all, the identical
// shape every other M6 effect already has -- not named here because it
// never blocks.
//
// UnlessCost$/UnlessPayer$/UnlessResolveSubs$/UnlessSwitched$ no longer
// block either: resolveUnlessCost (effect.go) gates the whole ability --
// this effect's own body included -- before Registry.Resolve ever reaches
// it, CR's own "sacrifice unless a cost is paid." 51 of the corpus's own
// 155 real Sacrifice lines naming UnlessCost$ resolve past that gate and
// are actually reachable at all (resolveUnlessCost's own doc comment has
// the pure-mana-cost/resolvable-payer accounting); 6 of the 57 that clear
// that filter still are not -- CardFactoryUtil's own real "other
// enchantments/creatures have 'sacrifice this unless you pay...'" shape
// (a Layer 6 S:Mode$ Continuous | AddTrigger$ line granting the actual
// T:Mode$ Phase trigger to every OTHER permanent it names, aura_flux.txt's/
// magus_of_the_tabernacle.txt's own real cards among them) -- AddTrigger$
// is not a built continuous-effect param, so the trigger it would grant
// never exists in this port's own game at all.
var sacrificeUnresolvedParams = [...]string{
	"ChangeNum", "ValidCard", "Random",
}

type sacrificeEffect struct{}

func (sacrificeEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	for _, key := range sacrificeUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: Sacrifice: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	if proceed, err := g.upkeepCostPaid(controller, a, source); err != nil || !proceed {
		return err
	}

	sacValid, hasSacValid := a.Params.Param("SacValid")
	_, optional := a.Params.Param("Optional")
	if !hasSacValid || sacValid == "Self" {
		// Card.canBeSacrificedBy refuses a phased-out permanent
		// (Card.java:6909).
		if source.Zone != Battlefield || source.IsPhasedOut() || source.Controller() != a.Controller || g.cantSacrifice(source, true, a) {
			return nil
		}
		if optional && !controller.ConfirmEffect(g, a.Controller, a.Source) {
			return nil
		}
		sacrificeCards(g, controller, a, []CardID{source.ID})
		return nil
	}

	amountParam, ok := a.Params.Param("Amount")
	if !ok {
		amountParam = "1"
	}
	amount, ok := resolveNamedAmount(g, a.Amounts, source, amountParam)
	if !ok {
		return fmt.Errorf("engine: Sacrifice: Amount$ %q is not resolvable", amountParam)
	}

	var players []PlayerID
	var err error
	if _, hasValidTgts := a.Params.Param("ValidTgts"); hasValidTgts {
		for _, e := range a.Targets {
			if pid, ok := e.AsPlayer(); ok {
				players = append(players, pid)
			}
		}
	} else {
		defined, ok := a.Params.Param("Defined")
		if !ok {
			defined = "You"
		}
		players, err = definedPlayers(g, a.Controller, a.Source, defined, a.refs())
		if err != nil {
			return fmt.Errorf("engine: Sacrifice: %w", err)
		}
	}

	_, destroy := a.Params.Param("Destroy")
	_, eachValid := a.Params.Param("SacEachValid")
	if destroy && (eachValid || optional) {
		// SacEachValid$ asks choosePermanentsToSacrifice and then destroys
		// what it chose (SacrificeEffect.java:124, 154), and Optional$ words
		// its confirmation as a sacrifice: no corpus line combines them.
		return fmt.Errorf("engine: Sacrifice: Destroy$ with SacEachValid$ or Optional$ not resolvable yet")
	}
	if eachValid {
		for _, pid := range players {
			sacrificeCards(g, controller, a, g.sacrificeEachValid(controller, a, pid, sacValid, amount))
		}
		return nil
	}
	spec := valid.Parse(sacValid)
	_, strict := a.Params.Param("StrictAmount")
	for _, pid := range players {
		var candidates []CardID
		for _, cid := range g.Zone(Battlefield, pid).Cards() {
			// A destroyed permanent need not be sacrificable
			// (SacrificeEffect.java:132-135).
			if Matches(g, g.Card(cid), spec, source.Controller(), a.Source) && (destroy || !g.cantSacrifice(g.Card(cid), true, a)) {
				candidates = append(candidates, cid)
			}
		}
		// StrictAmount$ (SacrificeEffect.java:137-144): fewer candidates than
		// Amount$ means nothing is sacrificed, and the player is not asked.
		if strict && len(candidates) < amount {
			continue
		}
		// Optional$: the player is asked first (Java asks even with no
		// candidate, SacrificeEffect.java:143).
		if optional && !controller.ConfirmEffect(g, pid, a.Source) {
			continue
		}
		n := amount
		if n > len(candidates) {
			n = len(candidates)
		}
		if n == 0 {
			continue
		}
		if !optional || strict {
			// minTargets = amount (SacrificeEffect.java:138): exactly Amount$
			// of the candidates, or all of them when fewer.
			if destroy {
				g.destroyChosen(controller, a, controller.ChoosePermanentsToDestroy(g, pid, candidates, n))
				continue
			}
			sacrificeCards(g, controller, a, controller.ChoosePermanentsToSacrifice(g, pid, candidates, n))
			continue
		}
		// Optional$ without StrictAmount$ (Exploit, Devour, "you may sacrifice
		// up to N"): minTargets is 0 (SacrificeEffect.java:138), so having
		// confirmed, the player still chooses from none up to Amount$
		// (choosePermanentsToSacrifice(sa, 0, amount, ...), :148). The answer
		// is checked, a wrong one an error (GO-7).
		chosen := controller.ChooseCardsForEffect(g, pid, a.Source, candidates, 0, n)
		if !isSubset(chosen, candidates) || hasDuplicate(chosen) || len(chosen) > n {
			return fmt.Errorf("engine: Sacrifice: optional answer %v is not up to %d of the %d offered", chosen, n, len(candidates))
		}
		sacrificeCards(g, controller, a, chosen)
	}
	return nil
}

// sacrificeEachValid is SacrificeEffect.java's SacEachValid$ branch
// (:109-130) for one player: SacValid$ splits on " & " into alternatives, and
// the player chooses Amount$ permanents for each, a permanent chosen for one
// alternative no longer offered to the later ones. removeCandidates keeps an
// early choice from starving a later alternative.
func (g *Game) sacrificeEachValid(controller PlayerController, a *Ability, pid PlayerID, sacValid string, amount int) []CardID {
	source := g.Card(a.Source)
	alts := strings.Split(sacValid, " & ")
	lists := make([][]CardID, len(alts))
	for i, alt := range alts {
		spec := valid.Parse(alt)
		for _, cid := range g.Zone(Battlefield, pid).Cards() {
			if Matches(g, g.Card(cid), spec, source.Controller(), a.Source) && !g.cantSacrifice(g.Card(cid), true, a) {
				lists[i] = append(lists[i], cid)
			}
		}
	}
	var chosen []CardID
	for i, cands := range lists {
		if len(cands) == 0 {
			continue
		}
		if len(cands) > 1 && i < len(lists)-1 {
			cands = removeCandidates(cands, lists, map[CardID]bool{}, i+1, 0, amount)
		}
		picked := controller.ChoosePermanentsToSacrifice(g, pid, cands, min(amount, len(cands)))
		for j := i + 1; j < len(lists); j++ {
			lists[j] = slices.DeleteFunc(lists[j], func(id CardID) bool { return slices.Contains(picked, id) })
		}
		chosen = append(chosen, picked...)
	}
	return chosen
}

// removeCandidates is SacrificeEffect.removeCandidates (:229-255): it drops
// from targets every card that must stay free for a later alternative, i.e.
// each one whose removal-set union of later alternatives would be too small
// to cover them all. union is never mutated, only cloned.
func removeCandidates(targets []CardID, lists [][]CardID, union map[CardID]bool, index, included, amount int) []CardID {
	if index >= len(lists) {
		if len(union) <= included*amount {
			return slices.DeleteFunc(targets, func(id CardID) bool { return union[id] })
		}
		return targets
	}
	targets = removeCandidates(targets, lists, union, index+1, included, amount)
	candidate := lists[index]
	if len(candidate) == 0 {
		return targets
	}
	if len(union) == 0 {
		set := make(map[CardID]bool, len(candidate))
		for _, id := range candidate {
			set[id] = true
		}
		if len(candidate) <= amount {
			return slices.DeleteFunc(targets, func(id CardID) bool { return set[id] })
		}
		return removeCandidates(targets, lists, set, index+1, included+1, amount)
	}
	clone := maps.Clone(union)
	for _, id := range candidate {
		clone[id] = true
	}
	return removeCandidates(targets, lists, clone, index+1, included+1, amount)
}

// destroyChosen destroys each of ids (Destroy$, SacrificeEffect.java:154-160):
// GameAction.destroy, so an indestructible or regenerating permanent stays.
// RememberSacrificed$ remembers only what was destroyed.
func (g *Game) destroyChosen(controller PlayerController, a *Ability, ids []CardID) {
	_, remember := a.Params.Param("RememberSacrificed")
	var destroyed []CardID
	for _, id := range ids {
		c := g.Card(id)
		if c.Zone != Battlefield || !canBeDestroyed(c) {
			continue
		}
		if g.destroyInstead(controller, id) || g.regenerate(controller, id) {
			continue
		}
		if remember {
			g.Card(a.Source).Memory.Remember(CardEntity(id))
		}
		g.moveToGraveyard(controller, id)
		g.checkDiesTriggers(controller, id)
		destroyed = append(destroyed, id)
	}
	g.checkChangesZoneAllTriggers(controller, destroyed, Battlefield, Graveyard)
}

// sacrificeCards actually sacrifices each of ids, GameAction.sacrifice's
// own per-card loop: fires Mode$ Sacrificed (checkSacrificedTriggers,
// trigger.go) before the zone change, using the card's still-live
// battlefield state -- a card being sacrificed cannot itself change
// controller mid-loop, so this port's live read stands in for Java's own
// LKI copy without needing one -- remembers it (RememberSacrificed$) if
// asked, then moves it to its owner's graveyard and fires the "dies" half
// of Mode$ ChangesZone (checkDiesTriggers, trigger.go),
// Player.addSacrificedThisTurn's own ordering ahead of sacrificeDestroy's
// own moveToGraveyard call, both ported directly. A card no longer on the
// battlefield (already sacrificed earlier in the same chosen slice, or
// moved away by an earlier link in a SubAbility$ chain) is skipped, the
// identical "already gone" guard checkDiesTriggers' own callers give an
// SBA-driven death. Once every card in ids has actually been sacrificed,
// Mode$ ChangesZoneAll fires once for the whole batch
// (checkChangesZoneAllTriggers, trigger.go) -- SacrificeEffect.java's/
// SacrificeAllEffect.java's own trailing `zoneMovements.
// triggerChangesZoneAll(game, sa)` call, ported directly, whether the
// batch held one card (the plain Sacrifice effect) or several
// (SacrificeAll).
func sacrificeCards(g *Game, controller PlayerController, a *Ability, ids []CardID) {
	sacrificeCardsFor(g, controller, a, ids, true)
}

// sacrificeCardsFor is sacrificeCards for a sacrifice paid as a cost (effect
// false) or done by an effect (true): a card a CantSacrifice static covers is
// not sacrificed (Card.canBeSacrificedBy). It answers the cards it did
// sacrifice.
func sacrificeCardsFor(g *Game, controller PlayerController, a *Ability, ids []CardID, effect bool) []CardID {
	remember, imprint := false, false
	if a.Params != nil { // a state-based sacrifice (a Saga's) has no ability behind it
		_, remember = a.Params.Param("RememberSacrificed")
		// SacrificeAllEffect.java:86-87 alone reads ImprintSacrificed$.
		_, imprint = a.Params.Param("ImprintSacrificed")
		imprint = imprint && a.API == APISacrificeAll
	}
	var sacrificed []CardID
	for _, id := range ids {
		c := g.Card(id)
		if c.Zone != Battlefield || c.IsPhasedOut() || g.cantSacrifice(c, effect, a) {
			continue
		}
		pid := c.Controller()
		g.recordSacrificed(pid, id)
		g.checkSacrificedTriggers(controller, id, pid)
		if remember {
			g.Card(a.Source).Memory.Remember(CardEntity(id))
		}
		if imprint {
			g.Card(a.Source).Memory.Imprint(id)
		}
		g.moveToGraveyard(controller, id)
		g.checkDiesTriggers(controller, id)
		sacrificed = append(sacrificed, id)
		if isDevourAbility(a) {
			g.noteDevoured(controller, a.Source, id)
		}
		if isExploitAbility(a) {
			g.checkExploitedTriggers(controller, a.Source, id)
		}
	}
	g.checkChangesZoneAllTriggers(controller, sacrificed, Battlefield, Graveyard)
	return sacrificed
}
