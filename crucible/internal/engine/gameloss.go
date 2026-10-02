// Losing and winning the game as events (CR 104): the Event$ GameLoss and
// Event$ GameWin replacement effects with Layer$ CantHappen ("you can't lose
// the game", "your opponents can't win"), and conceding. Ported from
// Player.loseConditionMet / cantLoseCheck / cantWin (Player.java:1974-2020),
// ReplaceGameLoss.canReplace and ReplacementHandler.cantHappenCheck. A
// concession is not an event: it just loses (Player.concede, "No cantLose
// checks - just lose").
//
// Only the CantHappen layer resolves. The other GameLoss replacements
// (ReplaceWith$ DrawSeven, ExileSetLife: Lich's Mirror, Lich's Mastery) need
// a controller decision and a resolving ability, and are skipped, never
// guessed at (GO-7).

package engine

import (
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
	if g.gameEventCantHappen("GameLoss", pid, reason) {
		return false
	}
	g.Player(pid).Lost = true
	return true
}

// cantWin is Player.cantWin: a CantHappen GameWin replacement covers pid.
func (g *Game) cantWin(pid PlayerID) bool {
	return g.gameEventCantHappen("GameWin", pid, "")
}

// Concede is Player.concede: pid loses at once, whatever stops them losing
// otherwise. The game ends at the next state-based check.
func (g *Game) Concede(pid PlayerID) {
	p := g.Player(pid)
	p.Lost, p.Conceded = true, true
}

// gameEventCantHappen is ReplacementHandler.cantHappenCheck over Event$ event
// for pid, reason being the GameLossReason name ("" for a win).
func (g *Game) gameEventCantHappen(event string, pid PlayerID, reason string) bool {
	if g.Player(pid).Conceded {
		return false
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
						if !gameEventMatches(g, r, event, host, z, reason, pid, face.Amounts) {
							continue
						}
						return true
					}
				}
			}
		}
	}
	return false
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
