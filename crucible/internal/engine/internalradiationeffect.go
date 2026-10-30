// InternalRadiation: CR 121.13's consequence of rad counters -- "at the
// beginning of your precombat main phase, if you have any rad counters, mill
// that many cards. For each nonland card milled this way, you lose 1 life and
// a rad counter." Java gives a player with rad counters a "Radiation" effect
// card in the Command zone (Player.createRadiationEffect, Player.java:3606-3628)
// whose Mode$ Phase | Phase$ Main1 trigger runs DB$ InternalRadiation
// (InternalRadiationEffect.java). No card script names the API: the effect
// card is its only caller.

package engine

//enginelint:allow id card game player ability event control parts zone becomemonarcheffect

import (
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/carddb/vocab"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
)

// internalRadiationEffect resolves DB$ InternalRadiation for the effect
// card's owner: mill as many cards as they have rad counters, then for each
// nonland card milled lose 1 life -- or gain it, under a Mode$
// GainLifeRadiation static (Strong, the Brutish Thespian) -- and remove that
// many rad counters.
type internalRadiationEffect struct{}

func (internalRadiationEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	pid := a.Controller
	milled := g.millCards(controller, pid, g.Player(pid).Counters.Count(Radiation))
	g.checkMilledAllTriggers(controller, milled)
	g.checkLeftZoneTriggers(controller, milled, Library)
	g.checkChangesZoneAllTriggers(controller, milled, Library, Graveyard)
	n := 0
	for _, id := range milled {
		if !g.Card(id).Type().Has(cardtype.Land) {
			n++
		}
	}
	if g.gainLifeRadiation(pid) {
		g.gainLife(controller, pid, n, a.Source, a)
	} else {
		g.loseLife(controller, pid, n, a.Source)
	}
	// Player.removeRadCounters: subtractCounter takes no more than the player has.
	if have := g.Player(pid).Counters.Count(Radiation); n > 0 && have > 0 {
		removed := min(n, have)
		g.Player(pid).Counters.Add(Radiation, -removed)
		emitCounterChanged(g, a.Source, PlayerEntity(pid), Radiation, -removed)
	}
	g.syncRadiationEffect(pid)
	return nil
}

// gainLifeRadiation is StaticAbilityGainLifeRadiation.gainLifeRadiation: a
// live Mode$ GainLifeRadiation line whose ValidPlayer$ admits pid.
func (g *Game) gainLifeRadiation(pid PlayerID) bool {
	return g.playerStatic(pid, "GainLifeRadiation", nil)
}

// syncRadiationEffects runs syncRadiationEffect for every player. Java makes
// or removes the card inside Player.addCounter/setCounters; this port checks
// at each state-based-action check too, so a rad counter any path places or
// removes (PutCounter, Proliferate, RemoveCounter, a fixture) is seen before
// the next precombat main phase begins.
func (g *Game) syncRadiationEffects() {
	for _, pid := range g.Players() {
		g.syncRadiationEffect(pid)
	}
}

// syncRadiationEffect is Player.createRadiationEffect/removeRadiationEffect:
// pid's "Radiation" effect card is in their Command zone exactly while they
// have a rad counter. One card per player for the whole game, as in Java.
func (g *Game) syncRadiationEffect(pid PlayerID) {
	p := g.Player(pid)
	if p.Counters.Count(Radiation) > 0 {
		p.radiationCard = g.putDesignationCard(p.radiationCard, pid, radiationEffectDef)
		return
	}
	g.removeDesignationCard(p.radiationCard)
}

// radiationEffectDef is the card Player.createRadiationEffect builds:
//
//	Mode$ Phase | Phase$ Main1 | ValidPlayer$ You | TriggerZones$ Command
//	  -> DB$ InternalRadiation
func radiationEffectDef() *compile.Card {
	trig := designationTrigger("Phase", []vocab.Param{
		{Key: "Phase", Value: "Main1"}, {Key: "ValidPlayer", Value: "You"}, {Key: "TriggerZones", Value: "Command"},
	}, "InternalRadiation", nil)
	return designationDef("Radiation", trig)
}
