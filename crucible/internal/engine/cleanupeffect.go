package engine

//enginelint:allow id card game ability condition control parts defined

import "fmt"

// cleanupUnresolvedParams are CleanUpEffect.java's params this port cannot
// honour yet. Log$ (CleanUpEffect.java:31-33, :70-72) only
// emits a UI event with the text of what is about to be cleared, so it is
// accepted and changes no state.
var cleanupUnresolvedParams = [...]string{
	"Defined"}

// cleanupEffect is CleanUpEffect.java: it wipes the host card's own Memory
// lists (memory.go) at the end of a chain that wrote to them. 3,011 real
// DB$ Cleanup lines, 2,705 of them ClearRemembered$ -- the single most
// common SubAbility$ tail in the corpus.
type cleanupEffect struct{}

func (cleanupEffect) Resolve(g *Game, a *Ability, _ PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	for _, key := range cleanupUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: Cleanup: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	if spec, ok := a.Params.Param("ForgetDefined"); ok {
		// CleanUpEffect.java: every card or player Defined$ names leaves the
		// host's remembered list.
		entities, err := definedEntities(g, a.Controller, source, spec, a.refs())
		if err != nil {
			return fmt.Errorf("engine: Cleanup: ForgetDefined$: %w", err)
		}
		for _, e := range entities {
			source.Memory.Forget(e)
		}
	}
	if _, ok := a.Params.Param("ClearTriggered"); ok {
		// TriggerHandler.clearDelayedTrigger(card): every delayed trigger
		// the host registered and has not fired is dropped.
		kept := g.delayed[:0]
		for _, d := range g.delayed {
			if d.Host != a.Source {
				kept = append(kept, d)
			}
		}
		g.delayed = kept
	}
	m := &source.Memory
	if _, ok := a.Params.Param("ClearCoinFlips"); ok {
		m.ClearFlipResults()
	}
	if _, ok := a.Params.Param("ClearRemembered"); ok {
		m.ClearRemembered()
	}
	if _, ok := a.Params.Param("ClearImprinted"); ok {
		m.ClearImprinted()
	}
	if _, ok := a.Params.Param("ClearChosenCard"); ok {
		m.ClearChosen()
	}
	if _, ok := a.Params.Param("ClearChosenPlayer"); ok {
		m.SetChosenPlayer(NoPlayer)
	}
	if _, ok := a.Params.Param("ClearChosenColor"); ok {
		m.SetChosenColors(0)
	}
	if _, ok := a.Params.Param("ClearChosenType"); ok {
		m.SetChosenType("", false)
		m.SetChosenType("", true)
	}
	if _, ok := a.Params.Param("ClearNamedCard"); ok {
		m.ClearNamedCards()
	}
	return nil
}
