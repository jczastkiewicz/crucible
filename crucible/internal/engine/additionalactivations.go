// Mode$ Activations statics: StaticAbilityAdditionalActivations.getLimit
// raises the per-turn (Boast) or per-game (Exhaust, Power-up) activation
// limit of a spell ability (Birgi, Elvish Refueler, Wonder Man).
//
// Ported from forge-game/src/main/java/forge/game/staticability/
// StaticAbilityAdditionalActivations.java (getLimit :15-41).

//enginelint:allow ability amount card game id valid
package engine

import (
	"math"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// activationLimit is getLimit(card, sa, activator, def): def, raised to the
// largest MinLimit$ of an applying line (-1 is unlimited and ends the scan),
// plus every Additional$. ok is false when a line's amount cannot be read, so
// the activation is refused rather than guessed (GO-7). Java scans Battlefield,
// Graveyard, Exile, Command and Stack; traitHosts is Battlefield and Command
// (quirks.md).
func (g *Game) activationLimit(activator PlayerID, c *Card, ability *compile.Ability, def int) (int, bool) {
	sa := &Ability{Source: c.ID, Controller: activator, Params: ability, activated: true}
	result, additional := def, 0
	for _, host := range g.staticHostsWith(c.ID) {
		h := g.Card(host)
		if h.Def == nil {
			continue
		}
		for _, face := range h.traitFaces() {
			for _, s := range face.Statics {
				if !strings.EqualFold(s.Name, "Activations") || !g.staticConditionsMet(h, s) {
					continue
				}
				if spec, ok := s.Param("ValidCard"); ok {
					if !Matches(g, c, valid.Parse(spec), h.Controller(), h.ID) {
						continue
					}
				}
				amounts := h.abilityAmounts(s)
				if spec, ok := s.Param("ValidSA"); ok {
					if matched, known := g.spellAbilityMatches(sa, spec, h, h.Controller(), amounts); !known || !matched {
						continue
					}
				}
				if spec, ok := s.Param("ValidPlayer"); ok {
					if matched, known := matchesPlayerSpec(g, activator, h.Controller(), h.ID, spec); !known || !matched {
						continue
					}
				}
				if raw, ok := s.Param("MinLimit"); ok {
					n, ok := resolveNamedAmount(g, amounts, h, raw)
					if !ok {
						return 0, false
					}
					if n == -1 {
						return math.MaxInt, true
					}
					result = max(result, n)
				}
				if raw, ok := s.Param("Additional"); ok {
					n, ok := resolveNamedAmount(g, amounts, h, raw)
					if !ok {
						return 0, false
					}
					additional += n
				}
			}
		}
	}
	return result + additional, true
}
