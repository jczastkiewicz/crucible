// Static Mode$ Panharmonicon (StaticAbilityPanharmonicon.java): a triggered
// ability of a matching event triggers an additional time per matching static.
// Only the enters-the-battlefield ChangesZone paths consult it here (the
// corpus's dominant shape); the other trigger modes, and statics that name
// ValidMode$ without ChangesZone, are listed in game-state.md "Not ported yet".

package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// panharmoniconParams are the params of a Panharmonicon line this port reads;
// a line with another is not applied.
var panharmoniconParams = map[string]bool{
	"mode": true, "validmode": true, "validcard": true, "validzone": true, "validcause": true,
	"origin": true, "destination": true, "description": true, "secondary": true,
	"ispresent": true, "presentcompare": true, "condition": true, "effectzone": true,
}

// panharmoniconExtra is countPanharmonicon for a ChangesZone trigger t of host
// whose event moved a card origin -> dest: how many extra times it triggers
// (StaticAbilityPanharmonicon.java:40-83; each matching static adds one). A
// static trigger is never doubled (:49). The raw count is returned: no
// ActivationLimit$ trigger reaches this port (scanTriggers skips them).
func (g *Game) panharmoniconExtra(host *Card, t *compile.Ability, moved *Card, origin, dest ZoneType) int {
	if isStaticTrigger(t) {
		return 0
	}
	n := 0
	for _, pid := range g.Players() {
		for _, sid := range g.traitHosts(pid) {
			sh := g.Card(sid)
			for _, face := range sh.traitFaces() {
				for _, s := range face.Statics {
					if mode, _ := s.Param("Mode"); mode == "Panharmonicon" && g.panharmoniconApplies(sh, s, host, moved, origin, dest) {
						n++
					}
				}
			}
		}
	}
	return n
}

func (g *Game) panharmoniconApplies(sh *Card, s *compile.Ability, host, moved *Card, origin, dest ZoneType) bool {
	if !paramsResolvable(s, panharmoniconParams) || !g.staticConditionsMet(sh, s) {
		return false
	}
	if v, ok := s.Param("ValidCard"); ok && !Matches(g, host, valid.Parse(v), sh.Controller(), sh.ID) {
		return false
	}
	if v, ok := s.Param("ValidMode"); ok && !containsString(strings.Split(v, ","), "ChangesZone") {
		return false
	}
	zones := "Battlefield"
	if v, ok := s.Param("ValidZone"); ok {
		zones = v
	}
	if !containsString(strings.Split(zones, ","), host.Zone.String()) {
		return false
	}
	if v, ok := s.Param("ValidCause"); ok && !Matches(g, moved, valid.Parse(v), sh.Controller(), sh.ID) {
		return false
	}
	if v, ok := s.Param("Origin"); ok && !containsString(strings.Split(v, ","), origin.String()) {
		return false
	}
	if v, ok := s.Param("Destination"); ok && !containsString(strings.Split(v, ","), dest.String()) {
		return false
	}
	return true
}

// withExtraCopies is m followed by extra further copies of it.
func withExtraCopies(matches []Ability, m Ability, extra int) []Ability {
	matches = append(matches, m)
	for range extra {
		matches = append(matches, m)
	}
	return matches
}
