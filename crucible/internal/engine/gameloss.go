// Losing and winning the game as events (CR 104): the Event$ GameLoss and
// Event$ GameWin replacement effects with Layer$ CantHappen ("you can't lose
// the game", "your opponents can't win"), and conceding. Ported from
// Player.loseConditionMet / cantLoseCheck / cantWin (Player.java:1974-2020),
// ReplaceGameLoss.canReplace and ReplacementHandler.cantHappenCheck. A
// concession is not an event: it just loses (Player.concede, "No cantLose
// checks - just lose").
//
// Only the CantHappen layer resolves. The other GameLoss replacements
// (ReplaceWith$ DrawSeven, ExileSetLife: Lich's Mirror, Exquisite Archangel)
// need a controller decision and a resolving ability: while one covers the
// player the loss is not applied and a pending error is recorded (GO-7).

package engine

import (
	"fmt"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/expr"
)

// Loss reasons (GameLossReason): the one a state-based action or effect gives
// ValidLoseReason$.
const (
	lossLifeReachedZero = "LifeReachedZero"
	lossMilled          = "Milled"
	lossPoisoned        = "Poisoned"
	lossSpellEffect     = "SpellEffect"
)

// loseConditionMet is Player.loseConditionMet: pid loses the game for reason
// unless a CantHappen GameLoss replacement stops it. False when it did.
func (g *Game) loseConditionMet(pid PlayerID, reason string) bool {
	cant, unresolved := g.gameEventCantHappen("GameLoss", pid, reason)
	if unresolved {
		// A live GameLoss replacement this port cannot apply (Lich's Mirror,
		// Exquisite Archangel): Java replaces the loss, so ending the game
		// here would be a guess (GO-7). The player stays in and the pending
		// error names why.
		g.recordPendingError(fmt.Errorf("engine: player %d would lose the game (%s) under a GameLoss replacement not resolvable yet", pid, reason))
		return false
	}
	if cant {
		return false
	}
	g.Player(pid).Lost = true
	return true
}

// cantWin is Player.cantWin: a CantHappen GameWin replacement covers pid.
func (g *Game) cantWin(pid PlayerID) bool {
	cant, _ := g.gameEventCantHappen("GameWin", pid, "")
	return cant
}

// Concede is Player.concede: pid loses at once, whatever stops them losing
// otherwise. The game ends at the next state-based check.
func (g *Game) Concede(pid PlayerID) {
	p := g.Player(pid)
	p.Lost, p.Conceded = true, true
}

// gameEventCantHappen is ReplacementHandler.cantHappenCheck over Event$ event
// for pid, reason being the GameLossReason name ("" for a win).
func (g *Game) gameEventCantHappen(event string, pid PlayerID, reason string) (cant, unresolved bool) {
	if g.Player(pid).Conceded {
		return false, false
	}
	for _, owner := range g.Players() {
		for _, z := range replacementZones {
			for _, host := range g.Zone(z, owner).Cards() {
				h := g.Card(host)
				if h.Def == nil {
					continue
				}
				for _, face := range h.Def.Faces {
					for _, r := range face.Replacements {
						if gameEventMatches(g, r, event, host, z, reason, pid, face.Amounts) {
							return true, false
						}
						if gameEventApplies(g, r, event, host, z, reason, pid) {
							unresolved = true
						}
					}
				}
			}
		}
	}
	return false, unresolved
}

// gameEventApplies is whether r is a live replacement of event covering pid
// and reason, whatever its layer or other params: one gameEventMatches could
// not resolve is a replacement this port cannot apply.
func gameEventApplies(g *Game, r *compile.Ability, event string, host CardID, hostZone ZoneType, reason string, pid PlayerID) bool {
	if !strings.EqualFold(r.Name, event) {
		return false
	}
	h := g.Card(host)
	if spec, ok := r.Param("ValidPlayer"); ok {
		if matched, recognized := matchesPlayerSpec(g, pid, h.Controller(), host, spec); recognized && !matched {
			return false
		}
	}
	if spec, ok := r.Param("ValidLoseReason"); ok && !containsString(strings.Split(spec, ","), reason) {
		return false
	}
	return hostInActiveZones(h, r, hostZone)
}

// gameEventMatches is ReplaceGameLoss/ReplaceGameWin.canReplace for a CantHappen
// line: r names event, ValidPlayer$ matches pid from the host's point of view,
// ValidLoseReason$ (when present) names reason, and the host is in an
// ActiveZones$ zone with its other requirements met. A param outside this
// shape's allow-list skips the line (GO-7).
func gameEventMatches(g *Game, r *compile.Ability, event string, host CardID, hostZone ZoneType, reason string, pid PlayerID, amounts map[string]expr.Amount) bool {
	if !strings.EqualFold(r.Name, event) {
		return false
	}
	if layer, ok := r.Param("Layer"); !ok || !strings.EqualFold(layer, "CantHappen") {
		return false
	}
	for _, p := range r.Params {
		switch strings.ToLower(p.Key) {
		case "event", "layer", "description", "validplayer", "validlosereason", "activezones", "secondary",
			"playerturn", "activephases", "checksvar", "svarcompare",
			"ispresent", "presentcompare", "presentzone", "presentplayer", "presentdefined":
		default:
			return false
		}
	}
	h := g.Card(host)
	if spec, ok := r.Param("ValidPlayer"); ok {
		matched, recognized := matchesPlayerSpec(g, pid, h.Controller(), host, spec)
		if !recognized || !matched {
			return false
		}
	}
	if spec, ok := r.Param("ValidLoseReason"); ok && !containsString(strings.Split(spec, ","), reason) {
		return false
	}
	return hostInActiveZones(h, r, hostZone) && replacementRequirementsCheck(g, h, amounts, r)
}
