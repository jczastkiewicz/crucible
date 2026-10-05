// Trigger modes fired by effects this port already has: Scry, Surveil,
// Transformed and TurnFaceUp, each a few params tested against the event,
// fired through scanTriggers (triggerscan.go).

package engine

//enginelint:allow id card ability trigger staticability triggerscan game control

import (
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// effectEventTriggerParams are the general params a trigger of these four
// modes may carry besides its own; the intervening-if and limit params are
// read by triggerEffectAPI, so naming them needs no check here. Anything else
// (Scry's ToBottom$, Surveil's FirstTime$, TurnFaceUp's ValidCause$) is a
// check this port has no evidence for, so the line is not fired (GO-7).
var effectEventTriggerParams = map[string]bool{
	"mode": true, "execute": true, "triggerdescription": true, "triggerzones": true, "optionaldecider": true,
	"secondary": true, "activationlimit": true, "resolvedlimit": true, "gameactivationlimit": true,
	"ispresent": true, "presentdefined": true, "presentcompare": true, "presentzone": true, "presentplayer": true,
	"playerturn": true,
}

func effectEventParams(own ...string) map[string]bool {
	m := make(map[string]bool, len(effectEventTriggerParams)+len(own))
	for k, v := range effectEventTriggerParams {
		m[k] = v
	}
	for _, k := range own {
		m[k] = true
	}
	return m
}

var (
	scryTriggerParams    = effectEventParams("validplayer")
	surveilTriggerParams = effectEventParams("validplayer")
	transformedParams    = effectEventParams("validcard")
	turnedFaceUpParams   = effectEventParams("validcard")
)

// checkScryTriggers is Mode$ Scry (TriggerScry.performTest, 24 real lines),
// run once per player that scried (GameAction.java:2652-2655): ValidPlayer$
// against the scrying player. ToBottom$ lines are not fired.
func (g *Game) checkScryTriggers(controller PlayerController, pid PlayerID) {
	g.pushTriggeredAbilities(controller, g.playerEventMatches("Scry", scryTriggerParams, pid))
}

// checkSurveilTriggers is Mode$ Surveil (TriggerSurveil.performTest, 16 real
// lines, Player.java:1083-1088): ValidPlayer$ against the surveiling player.
// FirstTime$ lines are not fired.
func (g *Game) checkSurveilTriggers(controller PlayerController, pid PlayerID) {
	g.pushTriggeredAbilities(controller, g.playerEventMatches("Surveil", surveilTriggerParams, pid))
}

func (g *Game) playerEventMatches(mode string, params map[string]bool, pid PlayerID) []Ability {
	return g.scanTriggers([]string{mode}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, params) || !g.triggerPlayerMatches(h, t, "ValidPlayer", pid) {
			return triggeredObjects{}, false
		}
		return triggeredObjects{player: pid}, true
	})
}

// checkTransformedTriggers is Mode$ Transformed (TriggerTransformed, 35 real
// lines): ValidCard$ against the permanent that just transformed.
func (g *Game) checkTransformedTriggers(controller PlayerController, card CardID) {
	g.pushTriggeredAbilities(controller, g.cardEventMatches("Transformed", transformedParams, card))
}

// checkTurnedFaceUpTriggers is Mode$ TurnFaceUp (TriggerTurnFaceUp, 126 real
// lines): ValidCard$ against the permanent just turned face up. ValidCause$
// lines (2) are not fired.
func (g *Game) checkTurnedFaceUpTriggers(controller PlayerController, card CardID) {
	g.pushTriggeredAbilities(controller, g.cardEventMatches("TurnFaceUp", turnedFaceUpParams, card))
}

func (g *Game) cardEventMatches(mode string, params map[string]bool, card CardID) []Ability {
	c := g.Card(card)
	return g.scanTriggers([]string{mode}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, params) || !g.triggerCardMatches(h, t, "ValidCard", c) {
			return triggeredObjects{}, false
		}
		return triggeredObjects{card: card}, true
	})
}
