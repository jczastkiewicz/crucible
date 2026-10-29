// Mode$ UnspentMana and Mode$ ManaBurn (StaticAbilityUnspentMana.java):
// which mana a player keeps as steps and phases end (Upwelling, Omnath,
// Locus of Mana), and whether losing unspent mana costs life (Yurlok of
// Scorch Thrash). ManaPool.clearPool reads both (ManaPool.java:119-165),
// PhaseHandler.onPhaseEnd applies the burn (PhaseHandler.java:461-470).

package engine

//enginelint:allow id card game player staticability triggerscan mana valid control event zone turn replacementevents lifelost replacement phase

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// unspentManaKept is StaticAbilityUnspentMana.getManaToKeep for pid: the
// plain buckets (white, blue, black, red, green, colorless) whose mana stays
// in pid's pool. A line's ValidPlayer$ (absent: every player) admits pid;
// its ManaType$ names one color, absent every type.
func (g *Game) unspentManaKept(pid PlayerID) (keep [6]bool) {
	g.eachManaStatic("UnspentMana", pid, func(h *Card, s *compile.Ability) {
		color, ok := s.Param("ManaType")
		if !ok {
			keep = [6]bool{true, true, true, true, true, true}
			return
		}
		for i, name := range [...]string{"White", "Blue", "Black", "Red", "Green", "Colorless"} {
			if strings.EqualFold(color, name) {
				keep[i] = true
			}
		}
	})
	return keep
}

// hasManaBurn is StaticAbilityUnspentMana.hasManaBurn for pid: some ManaBurn
// line's ValidPlayer$ (absent: every player) admits them. Java returns at
// the first line that rejects the player (StaticAbilityUnspentMana.java:38-40);
// the corpus has one line, so every line is read here.
func (g *Game) hasManaBurn(pid PlayerID) bool {
	burn := false
	g.eachManaStatic("ManaBurn", pid, func(*Card, *compile.Ability) { burn = true })
	return burn
}

// eachManaStatic calls fn for every live mode line whose ValidPlayer$ admits
// pid. A line naming a param this port does not read is skipped (GO-7).
func (g *Game) eachManaStatic(mode string, pid PlayerID, fn func(h *Card, s *compile.Ability)) {
	for _, p := range g.Players() {
		for _, host := range g.traitHosts(p) {
			h := g.Card(host)
			if h.Def == nil {
				continue
			}
			for _, face := range h.traitFaces() {
				for _, s := range face.Statics {
					if !strings.EqualFold(s.Name, mode) || !manaStaticReadable(s) || !g.staticConditionsMet(h, s) {
						continue
					}
					if spec, ok := s.Param("ValidPlayer"); ok {
						if matched, known := matchesPlayerSpec(g, pid, h.Controller(), h.ID, spec); !known || !matched {
							continue
						}
					}
					fn(h, s)
				}
			}
		}
	}
}

// manaStaticReadable reports whether s names only params eachManaStatic and
// staticConditionsMet read.
func manaStaticReadable(s *compile.Ability) bool {
	for _, p := range s.Params {
		switch strings.ToLower(p.Key) {
		case "mode", "validplayer", "manatype", "description", "secondary", "effectzone", "condition",
			"ispresent", "presentcompare", "presentzone", "presentplayer", "presentdefined", "checksvar", "svarcompare":
		default:
			return false
		}
	}
	return true
}

// manaBurn is Player.loseLife(lost, false, true) for mana burn: a static that
// stops life loss stops it, an Event$ LifeReduced replacement may change it.
func (g *Game) manaBurn(controller PlayerController, pid PlayerID, lost int) {
	if g.cantLoseLife(pid) {
		return
	}
	lose := g.lifeReduced(controller, pid, lost, false)
	if lose <= 0 {
		return
	}
	g.Player(pid).Life -= lose
	g.sink.Emit(Event{Kind: LifeChanged, Source: NoCard, Target: PlayerEntity(pid), Amount: -int32(lose)})
	g.noteLifeLost(controller, pid, lose)
}

// clearManaPool is ManaPool.clearPool(true) for pid at a phase end
// (ManaPool.java:119-165): the types a Mode$ UnspentMana line keeps stay, at
// every phase end, Cleanup included (:141-143); an Event$ LoseMana
// replacement (Kruphix) turns the pool into one type instead; combat mana
// stays until combat ends; then Mode$ ManaBurn costs the mana lost as life
// (PhaseHandler.java:461-470).
func (g *Game) clearManaPool(controller PlayerController, pid PlayerID) {
	pool := &g.Player(pid).ManaPool
	if pool.Total() == 0 {
		return
	}
	keep := g.unspentManaKept(pid)
	if keep == [6]bool{true, true, true, true, true, true} {
		return
	}
	if color, colorless, ok := g.loseManaConversion(controller, pid); ok {
		pool.convertTo(color, colorless)
		return
	}
	before := pool.Total()
	keepCombat := g.endingPhase != CombatEnd && g.endingPhase != Cleanup
	pool.emptyKeeping(func(u poolUnit) bool {
		return keepCombat && u.combat || keep[int(u.bucket)%bucketSnow]
	})
	if lost := before - pool.Total(); lost > 0 && g.hasManaBurn(pid) {
		g.manaBurn(controller, pid, lost)
	}
}
