package engine

//enginelint:allow id card game ability defined condition control effecthelpers valid zone parts

import (
	"fmt"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/valid"
	"github.com/jczastkiewicz/crucible/pkg/collect"
)

// chooseSourceUnresolvedParams are ChooseSourceEffect.java's params this
// port does not honour: Amount$ (0 of 67 real lines; Java's own do/while
// never ends once the pool runs dry before Amount$ picks are made) and
// TargetControls$ (0 of 67; Java reads only its presence, then indexes
// tgtPlayers.get(0) unguarded, ChooseSourceEffect.java:84-89).
var chooseSourceUnresolvedParams = [...]string{"Amount", "TargetControls"}

// chooseSourceUnresolvedChoices are the Choices$ properties Matches has no
// evaluator for yet: each would otherwise read false for every card and
// leave an empty pool, a silent no-op rather than a failed game (GO-7).
// ChosenColor (2 real lines) needs the host's chosen color, and
// SharesColorWith's own suffixed forms (Imprinted, ActivationColor -- 1
// line each) a remembered-list or cast-time color comparison.
var chooseSourceUnresolvedChoices = [...]string{"ChosenColor", "SharesColorWith "}

// chooseSourceEffect is ChooseSourceEffect.java: each chooser (Defined$/
// ValidTgts$, default You) picks one source -- a permanent, a spell or
// ability's source on the stack, an object one refers to, or a face-up
// Command-zone card, each group filtered by Choices$ -- and that pick
// becomes the host's chosen card (Memory.Choose), for the SubAbility$ it
// chains into (a DB$ Effect in 65 of 67 real lines).
//
// Ported from forge-game/src/main/java/forge/game/ability/effects/ChooseSourceEffect.java's resolve.
type chooseSourceEffect struct{}

func (chooseSourceEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	for _, key := range chooseSourceUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: ChooseSource: %s$ not resolvable yet", key)
		}
	}
	choicesParam, hasChoices := a.Params.Param("Choices")
	for _, prop := range chooseSourceUnresolvedChoices {
		if hasChoices && strings.Contains(choicesParam, prop) {
			return fmt.Errorf("engine: ChooseSource: Choices$ %q not resolvable yet", choicesParam)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	choosers, err := targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: ChooseSource: %w", err)
	}

	var spec valid.Spec
	if hasChoices {
		spec = valid.Parse(choicesParam)
	}
	if hasChoices && strings.Contains(choicesParam, "Source") && colorlessDamageSourceInPlay(g) && chooseSourceOffStack(g, a) {
		// A ColorlessDamageSource static's Spell.<Color>+inZoneStack
		// clause cannot match yet (baseMatches' own Spell case), so a
		// spell on the stack would still read as its printed color.
		return fmt.Errorf("engine: ChooseSource: Choices$ %q under a ColorlessDamageSource static with a spell on the stack not resolvable yet", choicesParam)
	}
	pool := chooseSourcePool(g, a, func(id CardID) bool {
		return !hasChoices || Matches(g, g.Card(id), spec, a.Controller, a.Source)
	})
	if pool.Len() == 0 {
		return nil
	}

	for _, pid := range choosers {
		if pool.Len() == 0 {
			// Java's do/while never ends here: only divider cards are
			// left, and it rejects every one (ChooseSourceEffect.java:
			// 131-133) -- a Forge hang, failed loudly instead (PORT-8).
			return fmt.Errorf("engine: ChooseSource: no source left for chooser %d", pid)
		}
		choices := append([]CardID(nil), pool.All()...)
		picked := controller.ChooseCardsForEffect(g, pid, a.Source, choices, 1, 1)
		if err := checkChoice(picked, choices, 1, 1); err != nil {
			return fmt.Errorf("engine: ChooseSource: %w", err)
		}
		pool.Remove(picked[0])
		// Java's setChosenCards sits inside the per-chooser loop and
		// replaces rather than appends: with several choosers only the
		// last one's pick stays chosen (PORT-7), while RememberChosen$
		// keeps every one.
		m := &source.Memory
		m.ClearChosen()
		m.Choose(picked[0])
		if _, ok := a.Params.Param("RememberChosen"); ok {
			m.Remember(CardEntity(picked[0]))
		}
	}
	return nil
}

// chooseSourceOffStack reports whether any stack item's source -- the
// resolving ability's included -- is off the battlefield, i.e. a spell.
func chooseSourceOffStack(g *Game, a *Ability) bool {
	if g.Card(a.Source).Zone != Battlefield {
		return true
	}
	for i := range g.stack {
		if g.Card(g.stack[i].Source).Zone != Battlefield {
			return true
		}
	}
	return false
}

// chooseSourcePool is ChooseSourceEffect.java's sourcesToChooseFrom, in
// Java's own group order: battlefield permanents, then each stack item's
// source, then the objects stack items refer to, then face-up
// Command-zone cards, each group filtered by keep (Choices$). The pool is a
// CardCollection in Java, so a card already listed stays at its first
// position -- an activated ability's source, or a targeted permanent, is
// offered once, among the permanents. Java's four "--PERMANENTS:--"-style
// divider cards are left out: its own do/while rejects every pick naming
// one, so they never reach the chosen list.
//
// Java walks game.getStack() top first with the resolving ability still on
// it (MagicStack.resolveStack removes it only after resolving); this port
// pops a first (ResolveStack, stack.go), so a stands in as that first
// item. Of the objects a stack item refers to, only its targeted card is
// read: this port's Ability carries no triggering or replacing objects
// (getTriggeringObjects/getReplacingObjects), a gap port-log names.
func chooseSourcePool(g *Game, a *Ability, keep func(CardID) bool) *collect.OrderedSet[CardID] {
	pool := collect.NewOrderedSet[CardID](0)
	add := func(ids ...CardID) {
		for _, id := range ids {
			if keep(id) {
				pool.Add(id)
			}
		}
	}
	for _, pid := range g.Players() {
		add(g.Zone(Battlefield, pid).Cards()...)
	}
	items := []*Ability{a}
	for i := len(g.stack) - 1; i >= 0; i-- {
		items = append(items, &g.stack[i])
	}
	for _, it := range items {
		add(it.Source)
	}
	for _, it := range items {
		if id, ok := stackTargetCard(it); ok {
			add(id)
		}
	}
	for _, pid := range g.Players() {
		for _, id := range g.Zone(Command, pid).Cards() {
			if !g.Card(id).IsFaceDown() {
				add(id)
			}
		}
	}
	return pool
}

// stackTargetCard is SpellAbility.getTargetCard: the first card a stack
// item targets, if any.
func stackTargetCard(a *Ability) (CardID, bool) {
	for _, e := range a.Targets {
		if id, ok := e.AsCard(); ok {
			return id, true
		}
	}
	if a.Target != NoCard {
		return a.Target, true
	}
	return NoCard, false
}
