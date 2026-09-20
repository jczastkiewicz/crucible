// Discard: CR 701.8, Mode$ TgtChoose -- a Defined$ player picks their own
// count of cards out of their own hand to discard -- 285 of the corpus's
// 942 real (AB|DB)$ Discard lines that also name Mode$ TgtChoose and
// Defined$ You/Opponent/Player/Player.Opponent and carry no other
// unresolved param.
//
// Ported from
// forge-game/src/main/java/forge/game/ability/effects/DiscardEffect.java's
// resolve, TgtChoose's own branch only. DiscardEffect.java has eight further
// Mode$ values (Hand, Random, YouChoose, LookYouChoose, RevealYouChoose,
// RevealTgtChoose, RevealDiscardAll, Defined) each its own further shape --
// this port keeps only the corpus's largest one.

package engine

import (
	"fmt"
	"strings"
)

// discardEffect resolves Mode$/DB$/AB$ Discard.
//
// Not ported (every one fails loudly rather than discarding the wrong cards
// from the wrong player, PORT-8/GO-7): every Mode$ other than TgtChoose
// (each its own further shape, above); SubAbility$ -- no ability-chaining
// mechanism exists yet; ValidTgts$/TargetMin$/TargetMax$ -- a real target,
// this port's own targeting gap; Optional$ -- an interactive confirm this
// port's own PlayerController has no hook for; AnyNumber$ -- a variable
// (0..hand size) count, a different shape from ChooseCardsToDiscard's own
// exact-count contract; DiscardValid$/DiscardValidDesc$ -- a filtered choice
// set, the identical gap PutCounter's own Choices$ family already
// documents; UnlessType$ -- a different sub-flow (chooseCardsToDiscardUnlessType,
// Java's own separate controller method); RevealNumber$ -- a reveal-then-
// choose-a-subset step ahead of the discard itself; UnlessCost$/
// UnlessPayer$/UnlessSwitched$/UnlessResolveSubs$ -- "discard unless you pay
// a cost," each its own further mechanic; RememberDiscarded$/
// RememberDiscardingPlayers$/RememberDiscardingPlayer$ -- no SubAbility
// chain exists to ever read a Remembered$ value back, the identical
// "blocked outright rather than silently no-op'd" choice PutCounter's own
// RememberCards$ already made.
var discardUnresolvedParams = [...]string{
	"SubAbility", "ValidTgts", "TargetMin", "TargetMax", "Optional", "AnyNumber",
	"DiscardValid", "DiscardValidDesc", "UnlessType", "RevealNumber",
	"UnlessCost", "UnlessPayer", "UnlessSwitched", "UnlessResolveSubs",
	"RememberDiscarded", "RememberDiscardingPlayers", "RememberDiscardingPlayer",
}

type discardEffect struct{}

func (discardEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	for _, key := range discardUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: Discard: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}

	mode, ok := a.Params.Param("Mode")
	if !ok {
		return fmt.Errorf("engine: Discard: Mode$ missing")
	}
	if !strings.EqualFold(mode, "TgtChoose") {
		return fmt.Errorf("engine: Discard: Mode$ %q not resolvable yet", mode)
	}

	defined, _ := a.Params.Param("Defined")
	players, err := definedPlayers(g, a.Controller, defined, a.Targets)
	if err != nil {
		return fmt.Errorf("engine: Discard: %w", err)
	}

	numCards, ok := a.Params.Param("NumCards")
	if !ok {
		numCards = "1"
	}
	amount, ok := resolveNamedAmount(g, a.Amounts, source, numCards)
	if !ok {
		return fmt.Errorf("engine: Discard: NumCards$ %q is not resolvable", numCards)
	}

	for _, pid := range players {
		hand := g.Zone(Hand, pid).Cards()
		count := amount
		if count > len(hand) {
			count = len(hand)
		}
		if count == 0 {
			continue
		}
		chosen := controller.ChooseCardsToDiscard(g, pid, hand, count)
		for _, id := range chosen {
			g.Move(id, Graveyard, g.Card(id).Owner)
			g.checkDiscardedTriggers(controller, id, pid)
		}
	}
	return nil
}
