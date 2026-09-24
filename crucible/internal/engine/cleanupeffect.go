package engine

import "fmt"

// cleanupUnresolvedParams are CleanUpEffect.java's params this port cannot
// honour yet. ClearChosenType$ and ClearNamedCard$ clear values no effect in
// this port sets (ChooseType and NameCard are not ported); accepting them as
// no-ops would silently go stale the day those land, so the line fails
// closed instead (PORT-8). ForgetDefined$ needs getDefinedEntities' mixed
// card-and-player reading, ClearTriggered$ a delayed-trigger registry,
// ClearCoinFlips$ FlipCoin, Log$ a random-log event -- none built.
var cleanupUnresolvedParams = [...]string{
	"Defined", "ForgetDefined", "ClearTriggered", "ClearCoinFlips",
	"ClearChosenType", "ClearNamedCard", "Log",
	"Condition", "ConditionDefined",
}

// cleanupEffect is CleanUpEffect.java: it wipes the host card's own Memory
// lists (memory.go) at the end of a chain that wrote to them. 3,011 real
// DB$ Cleanup lines, 2,705 of them ClearRemembered$ -- the single most
// common SubAbility$ tail in the corpus.
type cleanupEffect struct{}

func (cleanupEffect) Resolve(g *Game, a *Ability, _ PlayerController) error {
	for _, key := range cleanupUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: Cleanup: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	m := &source.Memory
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
	return nil
}
