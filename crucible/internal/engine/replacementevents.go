// The replacement Event$ values past Moved, Untap, DamageDone, Draw, GainLife,
// AddCounter, CreateToken, ProduceMana, DeclareBlocker, GameLoss and GameWin
// (replacement.go, gameloss.go), by the corpus's real R: line count:
//
//   - Counter (118): "can't be countered", Layer$ CantHappen. counterCantHappen.
//   - BeginPhase (21): "skip your draw step", Skip$ True. beginPhaseSkipped.
//   - LifeReduced (8): "reduces it to 1 instead", Worship's family, and
//     Bloodletter of Aclazotz's doubling. lifeReduced.
//   - LoseMana (5): Kruphix and Horizon Stone's "that mana becomes colorless
//     instead". loseManaConversion.
//   - BeginTurn (5): "that player skips that extra turn instead", Skip$ True
//     with ExtraTurn$ True. beginTurnSkipped.
//
// Ported from forge-game/src/main/java/forge/game/replacement/{ReplaceCounter,
// ReplaceBeginPhase,ReplaceLifeReduced,ReplaceLoseMana,ReplaceBeginTurn}.java
// and the call sites in CounterEffect.removeFromStack, PhaseHandler
// (advanceToNextPhase, getNextActivePlayer), Player.loseLife and
// ManaPool.clearPool. A line whose shape is not resolved (an Optional$ skip,
// a Transform substitution) records a pending error when it applies rather
// than being ignored (GO-7).

package engine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/expr"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// replacementIsCantHappen is "Layer$ CantHappen" with no ReplaceWith$: the
// line says the event does not happen, with nothing to run.
func replacementIsCantHappen(r *compile.Ability) bool {
	layer, ok := r.Param("Layer")
	return ok && strings.EqualFold(layer, "CantHappen") && replaceWithSub(r) == nil
}

// counterCantHappen is ReplacementHandler.cantHappenCheck over Event$ Counter
// for spell, called by CounterEffect.removeFromStack before the spell leaves
// the stack: a Layer$ CantHappen line whose ValidCard$ matches the spell and
// whose ValidSA$ matches it as a spell ("Spell", or "Spell.<properties>" read
// off the spell's card) stops the counter.
//
// ActiveZones$ names the zones the host must be in; with none, the line
// applies from anywhere (TriggerReplacementBase.zonesCheck), which is how a
// spell's own "this spell can't be countered" works from the stack. Hosts are
// the spell itself and everything on the battlefield or in the command zone.
// A line this cannot resolve (Guile's ReplaceWith$, a ValidCause$) is an
// error when no resolvable line stopped the counter: the spell could be
// countered wrongly otherwise.
func (g *Game) counterCantHappen(spell CardID) (bool, error) {
	spellCard := g.Card(spell)
	hosts := []CardID{spell}
	for _, pid := range g.Players() {
		for _, z := range replacementZones {
			for _, id := range g.Zone(z, pid).Cards() {
				if id != spell {
					hosts = append(hosts, id)
				}
			}
		}
	}
	unresolved := ""
	for _, id := range hosts {
		h := g.Card(id)
		if h.Def == nil {
			continue
		}
		for _, face := range h.liveTraitFaces() {
			for _, r := range face.Replacements {
				if !strings.EqualFold(r.Name, "Counter") {
					continue
				}
				if _, ok := r.Param("ActiveZones"); ok && !hostInActiveZones(h, r, h.Zone) {
					continue
				}
				if !replacementIsCantHappen(r) || !onlyParams(r, "layer", "validcard", "validsa") {
					unresolved = h.Def.Name
					continue
				}
				if v, ok := r.Param("ValidCard"); ok && !Matches(g, spellCard, valid.Parse(v), h.Controller(), h.ID) {
					continue
				}
				if v, ok := r.Param("ValidSA"); ok {
					matched, recognized := spellValidSA(g, v, spellCard, h)
					if !recognized {
						unresolved = h.Def.Name
						continue
					}
					if !matched {
						continue
					}
				}
				if !replacementRequirementsCheck(g, h, face.Amounts, r) {
					continue
				}
				return true, nil
			}
		}
	}
	if unresolved != "" {
		return false, fmt.Errorf("engine: Counter: %q's Event$ Counter replacement is not resolvable yet", unresolved)
	}
	return false, nil
}

// spellValidSA is ValidSA$ matched against the spell on the stack: each
// comma-separated alternative is "Spell" with optional "+"-joined properties
// read off the spell's card as a Card valid string, "SpellAbility" (any), or
// an ability kind ("Activated", "Triggered") that a spell never is.
func spellValidSA(g *Game, spec string, spell, host *Card) (matched, recognized bool) {
	recognized = true
	for _, alt := range strings.Split(spec, ",") {
		base, props, _ := strings.Cut(alt, ".")
		switch base {
		case "Spell":
			if props == "" || Matches(g, spell, valid.Parse("Card."+props), host.Controller(), host.ID) {
				matched = true
			}
		case "SpellAbility":
			if props != "" {
				recognized = false
				continue
			}
			matched = true
		case "Activated", "Triggered":
		default:
			recognized = false
		}
	}
	return matched, recognized
}

// beginPhaseSkipped is ReplacementType.BeginPhase's run for the active
// player at the start of phase (PhaseHandler.advanceToNextPhase): a
// "Skip$ True" line whose ValidPlayer$ matches the player whose turn it is
// (absent: every player) and whose Phase$ names phase skips it. The step is
// not begun at all, the same as a SkipPhase effect (consumeSkip). A line that
// would apply but is not a plain skip (Fasting's Optional$ ReplaceWith$)
// records a pending error.
func (g *Game) beginPhaseSkipped(phase PhaseType) bool {
	active := g.ActivePlayer()
	skip := false
	g.eachReplacementRule(func(h *Card, z ZoneType, amounts map[string]expr.Amount, r *compile.Ability) {
		if skip || !strings.EqualFold(r.Name, "BeginPhase") || !hostInActiveZones(h, r, z) {
			return
		}
		if v, ok := r.Param("ValidPlayer"); ok {
			if matched, recognized := matchesPlayerSpec(g, active, h.Controller(), h.ID, v); !recognized || !matched {
				return
			}
		}
		if _, ok := r.Param("Phase"); ok && !phaseTriggerMatches(r, "Phase", phase) {
			return
		}
		if !replacementRequirementsCheck(g, h, amounts, r) {
			return
		}
		if v, ok := r.Param("Skip"); !ok || !strings.EqualFold(v, "True") || !onlyParams(r, "validplayer", "phase", "skip", "layer", "hellbent") {
			g.recordPendingError(fmt.Errorf("engine: %q: Event$ BeginPhase: only a plain Skip$ True line is resolvable yet", h.Def.Name))
			return
		}
		skip = true
	})
	return skip
}

// beginTurnSkipped is ReplacementType.BeginTurn's run for pid, who is about
// to take a turn (PhaseHandler.getNextActivePlayer): a "Skip$ True" line
// whose ValidPlayer$ matches pid, and whose ExtraTurn$ (when present) is
// true only for an extra turn, skips the turn. isExtra is Java's
// "!extraTurns.isEmpty()" after the pop: the entry at the bottom of the
// extra-turn stack is the normal turn the extra ones interrupted.
func (g *Game) beginTurnSkipped(pid PlayerID, isExtra bool) bool {
	skip := false
	g.eachReplacementRule(func(h *Card, z ZoneType, amounts map[string]expr.Amount, r *compile.Ability) {
		if skip || !strings.EqualFold(r.Name, "BeginTurn") || !hostInActiveZones(h, r, z) {
			return
		}
		if v, ok := r.Param("ValidPlayer"); ok {
			if matched, recognized := matchesPlayerSpec(g, pid, h.Controller(), h.ID, v); !recognized || !matched {
				return
			}
		}
		if v, ok := r.Param("ExtraTurn"); ok && strings.EqualFold(v, "True") && !isExtra {
			return
		}
		if !replacementRequirementsCheck(g, h, amounts, r) {
			return
		}
		if v, ok := r.Param("Skip"); !ok || !strings.EqualFold(v, "True") || !onlyParams(r, "validplayer", "extraturn", "skip") {
			g.recordPendingError(fmt.Errorf("engine: %q: Event$ BeginTurn: only a plain Skip$ True line is resolvable yet", h.Def.Name))
			return
		}
		skip = true
	})
	return skip
}

// lifeReduced is Player.loseLife's replacement run (ReplacementType
// .LifeReduced): amount is the life pid is about to lose, isDamage whether
// damage causes it. It returns what is lost instead, 0 for nothing.
//
// A Layer$ CantHappen line (Archon of Coronation: "damage doesn't cause you
// to lose life") stops the loss before any choice, as the CantHappen layer
// runs first. The rest are CR 616 candidates: Worship and its family's
// "reduces it to 1 instead" (a ReplaceEffect on Amount limited to life minus
// 1) and Bloodletter of Aclazotz's "twice that much". A ValidPlayer$ such as
// "You.lifeGE1" and a Result$ such as "LT1" (the life left after the loss,
// ReplaceLifeReduced.canReplace) both gate a line.
func (g *Game) lifeReduced(controller PlayerController, pid PlayerID, amount int, isDamage bool) int {
	if amount <= 0 {
		return amount
	}
	matches := func(h *Card, z ZoneType, amounts map[string]expr.Amount, r *compile.Ability, amount int) bool {
		if !strings.EqualFold(r.Name, "LifeReduced") || !hostInActiveZones(h, r, z) {
			return false
		}
		if !onlyParams(r, "validplayer", "isdamage", "result", "layer", "monarch") {
			g.recordPendingError(fmt.Errorf("engine: %q: Event$ LifeReduced: a param is not resolvable yet", h.Def.Name))
			return false
		}
		if v, ok := r.Param("ValidPlayer"); ok {
			if matched, recognized := matchesPlayerSpec(g, pid, h.Controller(), h.ID, v); !recognized || !matched {
				return false
			}
		}
		if v, ok := r.Param("IsDamage"); ok && strings.EqualFold(v, "True") != isDamage {
			return false
		}
		if v, ok := r.Param("Result"); ok {
			n, err := strconv.Atoi(v[min(2, len(v)):])
			if len(v) < 3 || err != nil {
				g.recordPendingError(fmt.Errorf("engine: %q: Event$ LifeReduced: Result$ %q is not resolvable", h.Def.Name, v))
				return false
			}
			if !compareOp(g.Player(pid).Life-amount, v[:2], n) {
				return false
			}
		}
		return replacementRequirementsCheck(g, h, amounts, r)
	}
	cant := false
	g.eachReplacementRule(func(h *Card, z ZoneType, amounts map[string]expr.Amount, r *compile.Ability) {
		if replacementIsCantHappen(r) && matches(h, z, amounts, r, amount) {
			cant = true
		}
	})
	if cant {
		return 0
	}
	g.runReplacements(controller, pid, func() []replacementCandidate {
		var out []replacementCandidate
		g.eachReplacementRule(func(h *Card, z ZoneType, amounts map[string]expr.Amount, r *compile.Ability) {
			sub := replaceWithSub(r)
			if sub == nil || !matches(h, z, amounts, r, amount) {
				return
			}
			out = append(out, replacementCandidate{host: h, rule: r, apply: func() replacementResult {
				ev := replacementEvent{amountName: "Amount", amount: amount}
				if !g.runReplaceWith(controller, h, amounts, sub, &ev) {
					g.recordPendingError(fmt.Errorf("engine: %q: Event$ LifeReduced: ReplaceWith$ %s is not resolvable yet", h.Def.Name, sub.Name))
					return replacementNotReplaced
				}
				amount = ev.amount
				return ev.result
			}})
		})
		return out
	})
	return amount
}

// loseManaConversion is ReplacementType.LoseMana's run for pid's unspent mana
// as a step or phase ends (ManaPool.clearPool): a line with ReplaceWith$ a
// ReplaceMana naming ReplaceType$ turns the mana into that type instead of
// emptying it (Kruphix, God of Horizons: colorless; Ozai: red). ok is false
// when no line applies.
func (g *Game) loseManaConversion(controller PlayerController, pid PlayerID) (color mana.Colors, colorless, ok bool) {
	res := g.runReplacements(controller, pid, func() []replacementCandidate {
		var out []replacementCandidate
		g.eachReplacementRule(func(h *Card, z ZoneType, amounts map[string]expr.Amount, r *compile.Ability) {
			if !strings.EqualFold(r.Name, "LoseMana") || !hostInActiveZones(h, r, z) {
				return
			}
			if v, ok := r.Param("ValidPlayer"); ok {
				if matched, recognized := matchesPlayerSpec(g, pid, h.Controller(), h.ID, v); !recognized || !matched {
					return
				}
			}
			sub := replaceWithSub(r)
			if sub == nil || !replacementRequirementsCheck(g, h, amounts, r) {
				return
			}
			out = append(out, replacementCandidate{host: h, rule: r, apply: func() replacementResult {
				// Java asks with the mana "C" and reads back what ReplaceMana made of it.
				ev := replacementEvent{amountName: "Mana", mana: producedMana{colorless: true, amount: 1}}
				if !g.runReplaceWith(controller, h, amounts, sub, &ev) {
					g.recordPendingError(fmt.Errorf("engine: %q: Event$ LoseMana: ReplaceWith$ %s is not resolvable yet", h.Def.Name, sub.Name))
					return replacementNotReplaced
				}
				color, colorless = ev.mana.color, ev.mana.colorless
				return replacementReplaced
			}})
		})
		return out
	})
	return color, colorless, res == replacementReplaced
}
