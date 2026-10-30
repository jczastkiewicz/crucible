// Static Mode$ Panharmonicon (StaticAbilityPanharmonicon.java): a triggered
// ability of a matching event triggers an additional time per matching static.
// The enters-the-battlefield ChangesZone paths count it with the event's
// origin, destination and cause (panharmoniconExtra); every other trigger
// match is counted when it is pushed (withPanharmonicon), by the host and zone
// alone, so a static that names an event param (ValidCause$, ValidActivator$,
// ValidTarget$, ...) outside those paths is not applied (game-state.md "Not
// ported yet").

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
	"validactivator": true, "validtarget": true, "validsource": true, "validturned": true,
	"validplayer": true, "combatdamage": true,
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
	if !staticParamsKnown(s, panharmoniconParams) || !g.staticConditionsMet(sh, s) {
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
	m.panhDone = true
	matches = append(matches, m)
	for range extra {
		matches = append(matches, m)
	}
	return matches
}

// withPanharmonicon is handlePanharmonicon (StaticAbilityPanharmonicon.java:
// 31-83) for every trigger match that no site counted: each match whose
// host's trigger a Panharmonicon line covers (ValidMode$ naming the trigger's
// mode or absent, ValidCard$ matching the host, ValidZone$ holding the host's
// zone) is followed by one more copy per such line. A Static$ True trigger,
// a delayed trigger (no host trigger owns its Execute$) and a trigger with an
// activation limit are never doubled (:49-57; the limit is left uncounted).
func (g *Game) withPanharmonicon(matches []Ability) []Ability {
	var lines []struct {
		host *Card
		s    *compile.Ability
	}
	for _, pid := range g.Players() {
		for _, sid := range g.traitHosts(pid) {
			sh := g.Card(sid)
			for _, face := range sh.traitFaces() {
				for _, s := range face.Statics {
					if mode, _ := s.Param("Mode"); mode == "Panharmonicon" {
						lines = append(lines, struct {
							host *Card
							s    *compile.Ability
						}{sh, s})
					}
				}
			}
		}
	}
	if len(lines) == 0 {
		return matches
	}
	var out []Ability
	for _, m := range matches {
		if m.panhDone || m.staticTrigger || m.Params == nil || m.Source == NoCard {
			out = append(out, m)
			continue
		}
		m.panhDone = true
		host := g.Card(m.Source)
		t := ownTrigger(host, m.Params)
		extra := 0
		if t != nil && !panharmoniconHasParam(t, "ActivationLimit", "GameActivationLimit", "ResolvedLimit") {
			for _, l := range lines {
				if g.panharmoniconGenericApplies(l.host, l.s, host, t.Name, &m) {
					extra++
				}
			}
		}
		out = append(out, m)
		for range extra {
			out = append(out, m)
		}
	}
	return out
}

// ownTrigger is the T: line of host whose Execute$ is execute, nil when none
// is (a delayed trigger, an effect's trigger on another card).
func ownTrigger(host *Card, execute *compile.Ability) *compile.Ability {
	for face := range host.triggerFaces {
		for _, t := range face.Triggers {
			for _, sub := range t.Subs {
				if strings.EqualFold(sub.Key, "Execute") && sub.Ability == execute {
					return t
				}
			}
		}
	}
	return nil
}

func (g *Game) panharmoniconGenericApplies(sh *Card, s *compile.Ability, host *Card, mode string, m *Ability) bool {
	if !staticParamsKnown(s, panharmoniconParams) || !g.staticConditionsMet(sh, s) {
		return false
	}
	if v, ok := s.Param("ValidMode"); ok && !containsString(strings.Split(v, ","), mode) {
		return false
	}
	if v, ok := s.Param("ValidCard"); ok && !Matches(g, host, valid.Parse(v), sh.Controller(), sh.ID) {
		return false
	}
	zones := "Battlefield"
	if v, ok := s.Param("ValidZone"); ok {
		zones = v
	}
	if !containsString(strings.Split(zones, ","), host.Zone.String()) {
		return false
	}
	return g.panharmoniconEventMatches(sh, s, mode, m)
}

// panharmoniconEventMatches is applyPanharmoniconAbility's mode branches
// (StaticAbilityPanharmonicon.java:106-217) over what the trigger recorded:
// each mode tests only the params it reads, an absent param passes, and a
// param whose event this port does not record fails the line (ChangesZone and
// the damage modes, whose events only the enter paths and damage sites hold).
func (g *Game) panharmoniconEventMatches(sh *Card, s *compile.Ability, mode string, m *Ability) bool {
	ctrl := sh.Controller()
	cardOK := func(key string, ids ...CardID) bool {
		v, ok := s.Param(key)
		if !ok {
			return true
		}
		spec := valid.Parse(v)
		for _, id := range ids {
			if id != NoCard && Matches(g, g.Card(id), spec, ctrl, sh.ID) {
				return true
			}
		}
		return false
	}
	playerOK := func(key string, pid PlayerID) bool {
		v, ok := s.Param(key)
		if !ok {
			return true
		}
		matched, recognized := matchesPlayerSpec(g, pid, ctrl, sh.ID, v)
		return recognized && matched
	}
	o := m.triggered
	switch strings.ToLower(mode) {
	case "changeszone", "changeszoneall":
		return !panharmoniconHasParam(s, "ValidCause", "Origin", "Destination")
	case "damagedone", "damagedoneonce", "damageall", "damagedealtonce":
		return !panharmoniconHasParam(s, "CombatDamage", "ValidSource", "ValidTarget")
	case "attacks":
		return cardOK("ValidCause", o.attacker)
	case "attackersdeclared", "attackersdeclaredonetarget":
		return cardOK("ValidCause", o.attackers...)
	case "spellcast", "spellcopy", "spellcastorcopy":
		return cardOK("ValidCause", o.card) && playerOK("ValidActivator", o.activator)
	case "becomestarget", "becomestargetonce":
		v, ok := s.Param("ValidTarget")
		if !ok {
			return true
		}
		return o.target != NoEntity && g.entityMatches(o.target, v, valid.Parse(v), ctrl, sh.ID)
	case "turnfaceup":
		return cardOK("ValidTurned", o.card)
	case "lifegained":
		return playerOK("ValidPlayer", o.player)
	}
	return true
}

// panharmoniconHasParam reports whether t carries any of keys.
func panharmoniconHasParam(t *compile.Ability, keys ...string) bool {
	for _, k := range keys {
		if _, ok := t.Param(k); ok {
			return true
		}
	}
	return false
}
