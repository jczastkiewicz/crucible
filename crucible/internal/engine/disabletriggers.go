// Static Mode$ DisableTriggers (StaticAbilityDisableTriggers.java, consulted
// by TriggerHandler.canRunTrigger:431): Torpor Orb, Hushbringer, Elesh Norn.
// The enters, dies and ChangesZoneAll paths consult it; the exile and
// left-to-hand/library paths and the other trigger modes ignore it, listed
// in game-state.md "Not ported yet".

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
	return g.triggerDisabledFor("ChangesZone", host, t, moved, origin, dest)
}

// triggerDisabledAll is the ChangesZoneAll branch (StaticAbilityDisableTriggers
// .java:94-131): it keeps the cards of the batch whose cause no static
// removes, so a partly disabled batch still triggers on the rest.
func (g *Game) triggerDisabledAll(host *Card, t *compile.Ability, cards []CardID, origin, dest ZoneType) []CardID {
	var kept []CardID
	for _, id := range cards {
		if !g.triggerDisabledFor("ChangesZoneAll", host, t, g.Card(id), origin, dest) {
			kept = append(kept, id)
		}
	}
	return kept
}

func (g *Game) triggerDisabledFor(mode string, host *Card, t *compile.Ability, moved *Card, origin, dest ZoneType) bool {
	if isStaticTrigger(t) {
		return false
	}
	for _, pid := range g.Players() {
		for _, sid := range g.traitHosts(pid) {
			sh := g.Card(sid)
			for _, face := range sh.traitFaces() {
				for _, s := range face.Statics {
					if m, _ := s.Param("Mode"); m != "DisableTriggers" {
						continue
					}
					if g.disableTriggersApplies(mode, sh, s, host, t, moved, origin, dest) {
						return true
					}
				}
			}
		}
	}
	return false
}

func (g *Game) disableTriggersApplies(mode string, sh *Card, s *compile.Ability, host *Card, t *compile.Ability, moved *Card, origin, dest ZoneType) bool {
	if _, ok := s.Param("ValidTrigger"); ok {
		return false
	}
	if v, ok := s.Param("ValidCard"); ok && !Matches(g, host, valid.Parse(v), sh.Controller(), sh.ID) {
		return false
	}
	if v, ok := s.Param("ValidMode"); ok && !containsString(strings.Split(v, ","), mode) {
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
	if mode == "ChangesZone" && origin == Battlefield && dest == Graveyard {
		if v, _ := t.Param("ValidCard"); v == "Card.Self" {
			if o, has := t.Param("Origin"); !has || o == "Any" {
				return false
			}
		}
	}
	return true
}
