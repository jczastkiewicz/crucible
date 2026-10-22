// Static Mode$ DisableTriggers (StaticAbilityDisableTriggers.java, consulted
// by TriggerHandler.canRunTrigger:431): Torpor Orb, Hushbringer, Elesh Norn.
// Only the ChangesZone paths that enter and leave the battlefield (enter,
// dies) consult it here; every other trigger path ignores it, listed in
// game-state.md "Not ported yet".

package engine

//enginelint:allow id card game valid statictrigger zone effecthelpers

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// triggerDisabled reports whether a DisableTriggers static on a permanent
// stops trigger t of host from firing for moved going origin -> dest
// (StaticAbilityDisableTriggers.isDisabled, ChangesZone branch,
// :46-92). Static triggers are exempt (TriggerHandler.java:431). A static
// that names ValidTrigger$ (Nowhere to Run's ward line) never matches a
// plain ChangesZone trigger, which has no overriding ability.
func (g *Game) triggerDisabled(host *Card, t *compile.Ability, moved *Card, origin, dest ZoneType) bool {
	if isStaticTrigger(t) {
		return false
	}
	for _, pid := range g.Players() {
		for _, sid := range g.traitHosts(pid) {
			sh := g.Card(sid)
			for _, face := range sh.traitFaces() {
				for _, s := range face.Statics {
					if mode, _ := s.Param("Mode"); mode != "DisableTriggers" {
						continue
					}
					if g.disableTriggersApplies(sh, s, host, t, moved, origin, dest) {
						return true
					}
				}
			}
		}
	}
	return false
}

func (g *Game) disableTriggersApplies(sh *Card, s *compile.Ability, host *Card, t *compile.Ability, moved *Card, origin, dest ZoneType) bool {
	if _, ok := s.Param("ValidTrigger"); ok {
		return false
	}
	if v, ok := s.Param("ValidCard"); ok && !Matches(g, host, valid.Parse(v), sh.Controller(), sh.ID) {
		return false
	}
	if v, ok := s.Param("ValidMode"); ok && !containsString(strings.Split(v, ","), "ChangesZone") {
		return false
	}
	if v, ok := s.Param("ValidCause"); ok && !Matches(g, moved, valid.Parse(v), sh.Controller(), sh.ID) {
		return false
	}
	if v, ok := s.Param("Destination"); ok && !containsString(strings.Split(v, ","), dest.String()) {
		return false
	}
	if v, ok := s.Param("Origin"); ok && !containsString(strings.Split(v, ","), origin.String()) {
		return false
	}
	// StaticAbilityDisableTriggers.java:84-92: a dying creature's own
	// "when CARDNAME dies" line keeps firing, though the static names
	// Battlefield -> Graveyard.
	if origin == Battlefield && dest == Graveyard {
		if v, _ := t.Param("ValidCard"); v == "Card.Self" {
			if o, has := t.Param("Origin"); !has || o == "Any" {
				return false
			}
		}
	}
	return true
}
