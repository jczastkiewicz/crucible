// Choosing targets: CR 601.2c (a spell) / 603.3b (a triggered ability) --
// "choices... including targets... are made" the moment the ability is put
// on the stack. This port has exactly one place that happens today,
// pushTriggeredAbilities (trigger.go), so resolveTargets is called from
// there rather than from a separate step of its own; a future cast path for
// an Instant/Sorcery naming its own top-level ValidTgts$ (not built --
// CastSpell, castspell.go, only casts a permanent or an Aura today) would
// call it from wherever that lands too.

package engine

import (
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// targetUnresolvedParams names ValidTgts$'s own structural siblings this
// port does not parse: Radiance$ (4 real corpus lines -- "and each other
// permanent that shares a color with it," a second, derived candidate set
// no single ValidTgts$ evaluation produces), TargetsForEachPlayer$/
// TargetsWithDefinedController$/TargetUnique$ (0 real lines each) -- every
// one its own further mechanic. A line naming any of these is treated the
// same as CR 603.3c's own "no legal targets" case (below) rather than
// erroring: both mean the ability does not do anything, and this port has
// no way to tell the difference from outside without building the shape
// (PORT-8/GO-7's "skip rather than guess," folded into the identical bucket
// a genuinely empty candidate set already uses since the two are
// observationally identical).
var targetUnresolvedParams = [...]string{
	"Radiance", "TargetsForEachPlayer", "TargetsWithDefinedController", "TargetUnique",
}

// resolveTargets is CR 601.2c/603.3b's own "choose targets," and reports
// whether a is still eligible to be pushed onto the stack at all. true with
// a.Targets left nil means there was nothing to target in the first place --
// no ValidTgts$ named. false means either CR 603.3c's own real rule ("if
// the ability requires a target and there are no legal targets, it doesn't
// go on the stack") or a target shape targetUnresolvedParams names above --
// the two are folded into the same outcome rather than given an error
// return, because a caller cannot act on "an ability the game decided not
// to put on the stack" any differently than "an ability this port cannot
// parse the targeting for": either way, nothing happens, and GO-7's own
// "fail one game, not the batch" reasoning does not apply to a card that
// was never going to do anything this port can tell.
//
// TargetMin$/TargetMax$ (1/1 when neither is named, TargetRestrictions.
// java's own getOrDefault) resolve through resolveNamedAmount exactly as
// every other numeric param already does. ValidTgts$'s own shape decides
// whether candidates are players or cards -- never both, since no real
// corpus line this port has read mixes the two in one ValidTgts$ string --
// by trying matchesPlayerSpec (valid.go) first: ok reports whether the
// spec's own base token is player-shaped at all (matchesPlayerBase's own
// contract), regardless of which candidate is asked, so one trial call
// settles the shape for the whole spec.
func (g *Game) resolveTargets(controller PlayerController, a *Ability) bool {
	validTgts, ok := a.Params.Param("ValidTgts")
	if !ok {
		return true
	}
	for _, key := range targetUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return false
		}
	}

	source := g.Card(a.Source)
	minStr, ok := a.Params.Param("TargetMin")
	if !ok {
		minStr = "1"
	}
	maxStr, ok := a.Params.Param("TargetMax")
	if !ok {
		maxStr = "1"
	}
	targetMin, ok := resolveNamedAmount(g, a.Amounts, source, minStr)
	if !ok {
		return false
	}
	targetMax, ok := resolveNamedAmount(g, a.Amounts, source, maxStr)
	if !ok {
		return false
	}

	candidates := g.targetCandidates(a.Controller, a.Source, validTgts)
	if len(candidates) == 0 {
		return false
	}
	chosen := controller.ChooseTargets(g, a.Controller, candidates, targetMin, targetMax)
	a.Targets = chosen
	return true
}

// targetCandidates evaluates spec against every player still in the game
// (matchesPlayerSpec, valid.go; a player who has lost is never a legal
// target, the identical exclusion definedPlayers's own `if (!p.isInGame())`
// reading already makes), if spec is player-shaped at all, else against
// every card on any player's battlefield (Matches, valid.go) -- CR's own
// implicit "target creature" scope, and the only zone 0 real corpus
// TgtZone$ lines ever ask this port to look anywhere else than.
func (g *Game) targetCandidates(controller PlayerID, source CardID, spec string) []EntityID {
	if _, ok := matchesPlayerSpec(g, controller, controller, source, spec); ok {
		var candidates []EntityID
		for _, pid := range g.Players() {
			if g.Player(pid).Lost {
				continue
			}
			if matched, _ := matchesPlayerSpec(g, pid, controller, source, spec); matched {
				candidates = append(candidates, PlayerEntity(pid))
			}
		}
		return candidates
	}

	parsed := valid.Parse(spec)
	var candidates []EntityID
	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			if Matches(g, g.Card(id), parsed, controller, source) {
				candidates = append(candidates, CardEntity(id))
			}
		}
	}
	return candidates
}
