package engine

//enginelint:allow game ability control effecthelpers condition card id zone trigger triggerscan staticability  triggerpack

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// DayTime is Game.daytime (CR 726): neither until an effect or a daybound
// permanent makes it day or night.
type DayTime uint8

// The three day/night states.
const (
	DayNeither DayTime = iota
	Day
	Night
)

// DayTime reports whether it is day, night, or neither.
func (g *Game) DayTime() DayTime { return g.dayTime }

// dayTimeEffect is DayTimeEffect.java: Value$ Day or Night sets it, Switch
// flips it (neither counts as day). Daybound and nightbound permanents turn
// over when it changes, which needs transforming this port does not model,
// so the effect fails while one is on the battlefield; a CantChangeDayTime
// static fails it the same way.
type dayTimeEffect struct{}

func (dayTimeEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	if err := rejectParams(a, "DayTime"); err != nil {
		return err
	}
	if !subAbilityConditionMet(g, g.Card(a.Source), a.Amounts, a.Params) {
		return nil
	}
	if g.daynightBoundInPlay() || battlefieldStaticNames(g, "CantChangeDayTime") {
		return fmt.Errorf("engine: DayTime: daybound/nightbound permanents or CantChangeDayTime not resolvable yet")
	}
	value, _ := a.Params.Param("Value")
	switch value {
	case "Day":
		g.setDayTime(controller, Day)
	case "Night":
		g.setDayTime(controller, Night)
	case "Switch":
		if g.dayTime == Night {
			g.setDayTime(controller, Day)
		} else {
			g.setDayTime(controller, Night)
		}
	default:
		return fmt.Errorf("engine: DayTime: Value$ %q not resolvable", value)
	}
	return nil
}

// dayTimeAtUntap is Untap.doDayTime (CR 726.3a): as a turn's untap step
// begins, day becomes night if the previous turn's active player cast no
// spells that turn, and night becomes day if they cast two or more.
func (g *Game) dayTimeAtUntap(controller PlayerController) {
	if g.previousPlayer == NoPlayer {
		return
	}
	switch {
	case g.dayTime == Day && g.previousPlayerSpells == 0:
		g.setDayTime(controller, Night)
	case g.dayTime == Night && g.previousPlayerSpells > 1:
		g.setDayTime(controller, Day)
	}
}

// setDayTime is Game.setDayTime (Game.java:1423-1433): Mode$ DayTimeChanges
// fires when the value flips between day and night. Going from neither
// (Java's null) to a value is not a change, so the first day is silent.
func (g *Game) setDayTime(controller PlayerController, v DayTime) {
	prev := g.dayTime
	g.dayTime = v
	if prev == DayNeither || prev == v {
		return
	}
	matches := g.scanTriggers([]string{"DayTimeChanges"}, func(_ *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		return triggeredObjects{}, paramsResolvable(t, effectEventParams())
	})
	g.pushTriggeredAbilities(controller, matches)
}

// daynightBoundInPlay reports whether a daybound or nightbound permanent is
// on the battlefield.
func (g *Game) daynightBoundInPlay() bool {
	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			c := g.Card(id)
			if c.HasKeyword("Daybound") || c.HasKeyword("Nightbound") {
				return true
			}
		}
	}
	return false
}
