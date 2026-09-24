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
// (each its own further shape, above); ValidTgts$/TargetMin$/TargetMax$ --
// a real target, this port's own targeting gap; Optional$ -- an interactive
// confirm this port's own PlayerController has no hook for; AnyNumber$ -- a
// variable (0..hand size) count, a different shape from
// ChooseCardsToDiscard's own exact-count contract; DiscardValid$/
// DiscardValidDesc$ -- a filtered choice set, the identical gap PutCounter's
// own Choices$ family already documents; UnlessType$ -- a different
// sub-flow (chooseCardsToDiscardUnlessType, Java's own separate controller
// method); RevealNumber$ -- a reveal-then-choose-a-subset step ahead of the
// discard itself; RememberDiscarded$/RememberDiscardingPlayers$/
// RememberDiscardingPlayer$ -- no Defined$ Remembered resolver exists to
// ever read the value back (chaining itself existing does not help here:
// defined.go has no "Remembered" case), the identical "blocked outright
// rather than silently no-op'd" choice PutCounter's own RememberCards$
// already made.
//
// SubAbility$ no longer blocks: resolveSubAbility (subability.go) chains it
// through Registry.Resolve (effect.go) once this effect's own body
// finishes, whether or not subAbilityConditionMet let it run at all. 11 of
// the corpus's own 254 real SVar-defined Discard lines naming SubAbility$
// chain to an already-built leaf ability and resolve end to end.
//
// UnlessCost$/UnlessPayer$/UnlessSwitched$/UnlessResolveSubs$ no longer
// block either: resolveUnlessCost (effect.go) gates the whole ability
// before Registry.Resolve ever reaches it -- CR's own "discard unless you
// pay a cost." 0 of the corpus's own 16 real Discard lines naming
// UnlessCost$ resolve, though: rhystic_scrying.txt's own real pure-mana
// "{2}" is the only one clearing resolveUnlessCost's own pure-mana-cost/
// resolvable-payer filter, and its own DB$ Discard is reached only by
// chaining out of a top-level A:SP$ Draw line on a Sorcery -- CastSpell
// does not cast an instant or sorcery at all (castspell.go's own doc
// comment); every other real line names a PayEnergy<.../Sac<.../
// Return<.../... cost part or a controller-derived UnlessPayer$
// (RememberedController, ReplacedPlayer, ...) this port cannot resolve.
var discardUnresolvedParams = [...]string{
	"ValidTgts", "TargetMin", "TargetMax", "Optional", "AnyNumber",
	"DiscardValid", "DiscardValidDesc", "UnlessType", "RevealNumber",
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
	players, err := definedPlayers(g, a.Controller, a.Source, defined, a.refs())
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
		discardCards(g, controller, chosen, pid)
	}
	return nil
}

// discardCards moves each of ids from its owner's hand to their graveyard
// and fires Mode$ Discarded (checkDiscardedTriggers), CR 701.8's own "move
// directly from hand to graveyard" -- shared between this effect's own body
// and a Discard<N/Card> activation cost (ActivateAbility,
// activateability.go), which discards the identical way regardless of
// whether a script effect or a cost triggered it.
func discardCards(g *Game, controller PlayerController, ids []CardID, pid PlayerID) {
	for _, id := range ids {
		g.Move(id, Graveyard, g.Card(id).Owner)
		g.checkDiscardedTriggers(controller, id, pid)
	}
}
