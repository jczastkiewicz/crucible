package engine

//enginelint:allow ability card control game id trigger zone

import (
	"slices"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// rollToVisitAttractions is CR 703.4g's turn-based roll
// (PhaseHandler.java:291-294, Player.rollToVisitAttractions,
// Player.java:4044-4046): at the start of the active player's precombat main
// phase, if that player controls an Attraction, they roll a six-sided die
// with no source (RollDiceEffect.rollDiceForPlayerToVisitAttractions, sa
// null) and visit each Attraction they control whose lights show the result.
func (g *Game) rollToVisitAttractions(controller PlayerController, pid PlayerID) {
	if !slices.ContainsFunc(g.Zone(Battlefield, pid).Cards(), func(id CardID) bool {
		return g.Card(id).Type().HasSubtype("Attraction")
	}) {
		return
	}
	roll := &Ability{Source: NoCard, Controller: pid, Params: &compile.Ability{}}
	result, err := g.rollDice(roll, controller, pid, 1, 6, 0, 0, true)
	if err != nil {
		g.recordPendingError(err)
		return
	}
	g.visitAttractions(controller, pid, result)
}

// visitAttractions is Player.visitAttractions (Player.java:4035-4042): every
// Attraction pid controls whose lights include light is visited
// (Card.visitAttraction, Card.java:6847-6852), marked visited this turn, and
// fires Mode$ VisitAttraction with AbilityKey.Card and AbilityKey.Player
// (TriggerVisitAttraction.java). AttractionsVisitedThisTurn counts an
// Attraction only on its first visit of the turn, while the trigger fires on
// every visit. The lights read are the face's own Lights: line
// (compile.Face.Lights); a printing variant's are not.
func (g *Game) visitAttractions(controller PlayerController, pid PlayerID, light int) {
	var matches []Ability
	for _, id := range g.Zone(Battlefield, pid).Cards() {
		c := g.Card(id)
		if c.Def == nil || !c.Type().HasSubtype("Attraction") || !slices.Contains(c.Def.Faces[0].Lights, light) {
			continue
		}
		if !c.visitedThisTurn {
			g.Player(pid).attractionsVisitedThisTurn++
		}
		c.visitedThisTurn = true
		matches = append(matches, g.scanTriggers([]string{"VisitAttraction"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
			if !g.triggerCardMatches(h, t, "ValidCard", c) || !g.triggerPlayerMatches(h, t, "ValidPlayer", pid) {
				return triggeredObjects{}, false
			}
			return triggeredObjects{player: pid, card: id}, true
		})...)
	}
	g.pushTriggeredAbilities(controller, matches)
}
