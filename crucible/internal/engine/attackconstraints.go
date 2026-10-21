// Attack legality: CR 508.1c-d's "obey as many requirements as possible"
// check on a declaration of attackers (ADR-0024 Decision 1).
//
// Ported from forge-game/src/main/java/forge/game/combat/AttackConstraints.java
// (getLegalAttackers, collectLegalAttackers, getSortedFilteredRequirements,
// countViolations), AttackRequirement.java (constructor, countViolations,
// getSortedRequirements), CombatUtil.validateAttackers (CombatUtil.java:82)
// and StaticAbilityMustAttack.entitiesMustAttack.
//
// Java's AttackConstraints also carries restrictions this port has no
// source for: AttackRestriction's types (NEED_TWO_OTHERS, NOT_ALONE,
// ONLY_ALONE, NEED_BLACK_OR_GREEN, NEED_GREATER_POWER -- "can't attack
// alone" keywords and Mode$ CantAttack statics), GlobalAttackRestrictions'
// maximum attacker counts. Mode$ AttackRequirement (causesToAttack) and
// Mode$ PlayerMustAttack (playerRequirements) are ported. Attack costs (Propaganda, Mode$
// CantAttackUnless, attackcost.go) are ported: legalAttack skips a costed
// pair. With the rest absent, getLegalAttackers' search collapses
// to its unrestricted path: every creature with a requirement attacks the
// defender it has the most requirements toward, and the search's branches
// that only a restriction or a maximum reaches (the ONLY_ALONE loop,
// isLimited's "try both with and without", predicate reservations) are not
// ported.

package engine

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// attackRequirement is AttackRequirement.java for one creature: how many
// requirements it has toward each possible defender (defenderSpecific,
// insertion-ordered like Java's LinkedHashMap).
type attackRequirement struct {
	attacker  CardID
	defenders []EntityID
	counts    []int
	// causes is causesToAttack (Mode$ AttackRequirement): the creatures this
	// one attacking forces, one entry per static that does, in static order.
	causes []CardID
}

// causeCount is causesToAttack.get(forced).size(): the statics forcing it.
func (r *attackRequirement) causeCount(forced CardID) int {
	n := 0
	for _, c := range r.causes {
		if c == forced {
			n++
		}
	}
	return n
}

// causedCreatures is causesToAttack.asMap().keySet(): each forced creature
// once, in first-static order.
func (r *attackRequirement) causedCreatures() []CardID {
	var out []CardID
	for _, c := range r.causes {
		if !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

// add is defenderSpecific.merge(e, n, Integer::sum).
func (r *attackRequirement) add(e EntityID, n int) {
	for i, d := range r.defenders {
		if d == e {
			r.counts[i] += n
			return
		}
	}
	r.defenders = append(r.defenders, e)
	r.counts = append(r.counts, n)
}

// count is defenderSpecific.getOrDefault(e, 0).
func (r *attackRequirement) count(e EntityID) int {
	for i, d := range r.defenders {
		if d == e {
			return r.counts[i]
		}
	}
	return 0
}

// violations is AttackRequirement.countViolations: every requirement it
// has, less those toward the defender it attacks (ok false: it stays home),
// plus, when it attacks, each creature it forces to attack that is not in
// attack although it could, at no attack cost to this creature's defender
// (AttackRequirement.java:111 asks about defender, not the forced creature's
// own). hasRequirement() counts causes alone as a requirement
// (AttackRequirement.java:84-86). The ONLY_ALONE and maximum-attackers gates
// of :105-113 have no source in this port (the file's doc comment).
func (g *Game) requirementViolations(r *attackRequirement, defender EntityID, attacking bool, attack map[CardID]EntityID) (int, error) {
	total := 0
	for _, n := range r.counts {
		total += n
	}
	if total == 0 && len(r.causes) == 0 {
		return 0, nil
	}
	if !attacking {
		return total, nil
	}
	total -= r.count(defender)
	for _, forced := range r.causedCreatures() {
		if _, in := attack[forced]; in || !g.canAttackAtAll(forced) {
			continue
		}
		_, costed, err := g.attackCost(forced, defender)
		if err != nil {
			return 0, err
		}
		if !costed {
			total += r.causeCount(forced)
		}
	}
	return total, nil
}

// newAttackRequirement is AttackRequirement's constructor: one requirement
// toward every possible defender per distinct goading player (CR 701.15b,
// AttackRequirement.java:36-38 -- Card.getGoaded is a deduplicating
// PlayerCollection) and per "attacks each combat if able" Mode$ MustAttack
// static, one toward the named entity per Mode$ MustAttack static naming
// one (MustAttack$), then every entry naming a player no longer in the game
// or a card that is no longer an opposing planeswalker or a battle removed.
func (g *Game) newAttackRequirement(attacker CardID, possibleAttackers []CardID, possibleDefenders []EntityID) (attackRequirement, error) {
	r := attackRequirement{attacker: attacker}
	causes, err := g.attackRequirementCauses(attacker, possibleAttackers)
	if err != nil {
		return r, err
	}
	r.causes = causes
	anything := len(g.Card(attacker).goaders())
	mustAttack, err := g.entitiesMustAttack(attacker)
	if err != nil {
		return r, err
	}
	for _, e := range mustAttack {
		if e == CardEntity(attacker) {
			anything++
		} else {
			r.add(e, 1)
		}
	}
	for _, d := range possibleDefenders {
		r.add(d, anything)
	}
	var defenders []EntityID
	var counts []int
	for i, e := range r.defenders {
		if g.attackRequirementEntityGone(attacker, e) {
			continue
		}
		defenders = append(defenders, e)
		counts = append(counts, r.counts[i])
	}
	r.defenders, r.counts = defenders, counts
	return r, nil
}

// attackRequirementEntityGone is AttackRequirement's removal test: a player
// who lost, or a card that is not on the battlefield, whose controller
// lost, or that is neither a battle nor controlled by an opponent of
// attacker's controller.
func (g *Game) attackRequirementEntityGone(attacker CardID, e EntityID) bool {
	if pid, ok := e.AsPlayer(); ok {
		return g.Player(pid).Lost
	}
	cid, ok := e.AsCard()
	if !ok {
		return true
	}
	c := g.Card(cid)
	if c.Zone != Battlefield || g.Player(c.Controller()).Lost {
		return true
	}
	return !c.Type().Has(cardtype.Battle) && c.Controller() == g.Card(attacker).Controller()
}

// goaders is Card.getGoaded: the distinct players goading c, in goad order.
func (c *Card) goaders() []PlayerID {
	var out []PlayerID
	for _, gd := range c.goadedBy {
		if !containsPlayer(out, gd.By) {
			out = append(out, gd.By)
		}
	}
	return out
}

// entitiesMustAttack is StaticAbilityMustAttack.entitiesMustAttack: for
// every Mode$ MustAttack static in play whose ValidCreature$ matches
// attacker, the entities its MustAttack$ names -- less any the active
// player controls or is (CR 506.2) -- or attacker itself when it names
// none ("attacks each combat if able"). Hosts are traitHosts: the
// battlefield and effect cards in the Command zone, where an effect's
// "target creature attacks this turn if able" lives (DB$ Effect |
// StaticAbilities$ MustAttack).
func (g *Game) entitiesMustAttack(attacker CardID) ([]EntityID, error) {
	var out []EntityID
	err := g.eachCombatStatic("MustAttack", func(h *Card, _ *compile.Face, s *compile.Ability) error {
		if !staticValidMatches(g, h, s, "ValidCreature", attacker) {
			return nil
		}
		spec, ok := s.Param("MustAttack")
		if !ok {
			out = append(out, CardEntity(attacker))
			return nil
		}
		defs, err := definedEntities(g, h.Controller(), h, spec, abilityRefs{})
		if err != nil {
			return fmt.Errorf("engine: Mode$ MustAttack: MustAttack$: %w", err)
		}
		for _, e := range defs {
			if pid, ok := e.AsPlayer(); ok && pid == g.activePlayer {
				continue
			}
			if cid, ok := e.AsCard(); ok && g.Card(cid).Controller() == g.activePlayer {
				continue
			}
			out = append(out, e)
		}
		return nil
	})
	return out, err
}

// attackRequirementCauses is StaticAbilityMustAttack.getAttackRequirements:
// for every Mode$ AttackRequirement static whose ValidCard$ matches attacker,
// each other possible attacker its ValidAttacker$ matches, once per static.
func (g *Game) attackRequirementCauses(attacker CardID, possibleAttackers []CardID) ([]CardID, error) {
	var out []CardID
	err := g.eachCombatStatic("AttackRequirement", func(h *Card, _ *compile.Face, s *compile.Ability) error {
		if !staticValidMatches(g, h, s, "ValidCard", attacker) {
			return nil
		}
		for _, other := range possibleAttackers {
			if other != attacker && staticValidMatches(g, h, s, "ValidAttacker", other) {
				out = append(out, other)
			}
		}
		return nil
	})
	return out, err
}

// mustAttackSpecific is StaticAbilityMustAttack.mustAttackSpecific, inverted
// the way AttackConstraints.countViolations reads it: for every Mode$
// PlayerMustAttack static whose ValidPlayer$ matches the active player, the
// possible defenders its MustAttack$ matches (an absent MustAttack$ matches
// them all). A static matching none is absent from Java's multimap.
func (g *Game) mustAttackSpecific(defenders []EntityID) ([][]EntityID, error) {
	var out [][]EntityID
	err := g.eachCombatStatic("PlayerMustAttack", func(h *Card, _ *compile.Face, s *compile.Ability) error {
		if spec, ok := s.Param("ValidPlayer"); ok {
			if matched, _ := matchesPlayerSpec(g, g.activePlayer, h.Controller(), h.ID, spec); !matched {
				return nil
			}
		}
		var set []EntityID
		spec, hasSpec := s.Param("MustAttack")
		for _, d := range defenders {
			if !hasSpec || g.entityMatches(d, spec, valid.Parse(spec), h.Controller(), h.ID) {
				set = append(set, d)
			}
		}
		if len(set) > 0 {
			out = append(out, set)
		}
		return nil
	})
	return out, err
}

// attackConstraints is AttackConstraints.java over the active player's
// creatures: every one's requirements, in battlefield order.
type attackConstraints struct {
	requirements []attackRequirement
	// playerRequirements is Java's playerRequirements multimap, one entry per
	// static: the defenders it names. Attacking any of them meets it.
	playerRequirements [][]EntityID
	defenders          []EntityID
}

// newAttackConstraints is AttackConstraints' constructor. Every creature
// the active player controls is a possible attacker, tapped or not, the
// way Java's getCreaturesInPlay is: an unavoidable violation (a tapped
// creature that must attack) counts toward the declared attack and the best
// possible one alike, so it cancels out.
func (g *Game) newAttackConstraints() (attackConstraints, error) {
	defenders := g.eligibleAttackTargets()
	ac := attackConstraints{defenders: defenders}
	var err error
	if ac.playerRequirements, err = g.mustAttackSpecific(defenders); err != nil {
		return attackConstraints{}, err
	}
	possible := g.creaturesInPlay(g.activePlayer)
	for _, id := range possible {
		r, err := g.newAttackRequirement(id, possible, defenders)
		if err != nil {
			return attackConstraints{}, err
		}
		ac.requirements = append(ac.requirements, r)
	}
	return ac, nil
}

// countViolations is AttackConstraints.countViolations: the requirements
// attack (attacker -> defender) leaves unmet, summed over every possible
// attacker, plus the PlayerMustAttack statics it leaves unmet. Restrictions (Java's -1) are not checked here: an attack that
// breaks one never reaches this -- DeclareCombatAttackers rejects an
// ineligible attacker or an unoffered defender before validating, and
// legalAttack only builds attacks from eligible pairs.
func (g *Game) countViolations(ac attackConstraints, attack map[CardID]EntityID) (int, error) {
	n := 0
	for i := range ac.requirements {
		r := &ac.requirements[i]
		d, attacking := attack[r.attacker]
		v, err := g.requirementViolations(r, d, attacking, attack)
		if err != nil {
			return 0, err
		}
		n += v
	}
	// AttackConstraints.java:397-402: one more per PlayerMustAttack static no
	// declared defender meets, whether or not anyone could attack.
	for _, set := range ac.playerRequirements {
		met := false
		for _, d := range attack {
			if containsEntity(set, d) {
				met = true
				break
			}
		}
		if !met {
			n++
		}
	}
	return n, nil
}

// attackOption is AttackConstraints.Attack: one creature attacking one
// defender, and how many of its requirements that meets.
type attackOption struct {
	attacker     CardID
	defender     EntityID
	requirements int
}

// sortedFilteredRequirements is getSortedFilteredRequirements: every
// (creature, defender) pair the creature can legally attack, each
// creature's pairs in ascending requirement order (getSortedRequirements,
// a stable sort), then the whole list stably sorted descending
// (Comparator.reverseOrder over a TimSort). canAttack(attacker, defender)
// is the restriction side this port has: eligible, and not a goader while
// another player can be attacked (goadTargets).
func (g *Game) sortedFilteredRequirements(ac attackConstraints) ([]attackOption, error) {
	targets := g.eligibleAttackTargets()
	var out []attackOption
	for _, r := range ac.requirements {
		if !g.canAttackAtAll(r.attacker) {
			continue
		}
		allowed := g.goadTargets(r.attacker, targets)
		var own []attackOption
		for i, d := range r.defenders {
			if containsEntity(allowed, d) {
				own = append(own, attackOption{attacker: r.attacker, defender: d, requirements: r.counts[i]})
			}
		}
		sort.SliceStable(own, func(i, j int) bool { return own[i].requirements < own[j].requirements })
		out = append(out, own...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].requirements > out[j].requirements })
	return g.addPlayerRequirementBonus(ac, out)
}

// addPlayerRequirementBonus is the tail of getSortedFilteredRequirements
// (AttackConstraints.java:324-344): the defender the most PlayerMustAttack
// statics name gets the best unused option toward it that costs nothing to
// attack with, which then meets all of them; a defender no such option
// reaches is dropped alone, its statics staying under their other defenders.
// Java's max over a hash-keyed map breaks a tie by an identity hash no port
// can reproduce; here the first defender in eligibleAttackTargets order wins.
func (g *Game) addPlayerRequirementBonus(ac attackConstraints, out []attackOption) ([]attackOption, error) {
	alive := make([]bool, len(ac.playerRequirements))
	for i := range alive {
		alive[i] = true
	}
	dropped := map[EntityID]bool{}
	var used []CardID
	for {
		var key EntityID
		var best []int
		for _, d := range ac.defenders {
			if dropped[d] {
				continue
			}
			var statics []int
			for i, set := range ac.playerRequirements {
				if alive[i] && containsEntity(set, d) {
					statics = append(statics, i)
				}
			}
			if len(statics) > len(best) {
				key, best = d, statics
			}
		}
		if len(best) == 0 {
			break
		}
		match := -1
		for i, o := range out {
			if slices.Contains(used, o.attacker) || o.defender != key {
				continue
			}
			_, costed, err := g.attackCost(o.attacker, o.defender)
			if err != nil {
				return nil, err
			}
			if !costed {
				match = i
				break
			}
		}
		if match < 0 {
			dropped[key] = true
			continue
		}
		out[match].requirements += len(best)
		used = append(used, out[match].attacker)
		for _, i := range best {
			alive[i] = false
		}
	}
	if len(used) > 0 {
		sort.SliceStable(out, func(i, j int) bool { return out[i].requirements > out[j].requirements })
	}
	return out, nil
}

// legalAttack is collectLegalAttackers' unrestricted path (the file's doc
// comment has why no other path is reachable): walk the sorted options,
// skip one that meets no requirement or whose pair has an attack cost
// (AttackConstraints.java:195-197, "has to pay a cost: skip!" -- a zero
// cost too, getAttackCost being non-null), otherwise send its creature at
// its defender and drop the creature's remaining options. A costed pair is
// skipped alone: the creature's other options stay, as Java removes only
// that Attack from reqs.
func (g *Game) legalAttack(ac attackConstraints, options []attackOption) (map[CardID]EntityID, error) {
	attack := map[CardID]EntityID{}
	options = slices.Clone(options)
	for i := range options {
		o := options[i]
		if _, done := attack[o.attacker]; done || o.requirements == 0 {
			continue
		}
		_, costed, err := g.attackCost(o.attacker, o.defender)
		if err != nil {
			return nil, err
		}
		if costed {
			continue
		}
		// AttackConstraints.java:218-224: a creature that forces others to
		// attack adds the statics forcing each to every option the forced one
		// still has. The list is not re-sorted (Java sorts only after the
		// player bonus).
		for _, r := range ac.requirements {
			if r.attacker != o.attacker {
				continue
			}
			for _, forced := range r.causedCreatures() {
				for j := i + 1; j < len(options); j++ {
					if options[j].attacker == forced {
						options[j].requirements += r.causeCount(forced)
					}
				}
			}
		}
		attack[o.attacker] = o.defender
	}
	return attack, nil
}

// attackValidation is a combat's AttackConstraints as Combat.initConstraints
// builds it, once, before anything taps: the requirements and the sorted
// options. PhaseHandler.declareAttackersTurnBasedAction re-validates against
// the same constraints after an attacker is dropped for an unpaid attack cost
// (PhaseHandler.java:585), while the declared attackers sit provisionally
// tapped; rebuilding them then would read every one of those as unable to
// attack (canAttackAtAll's untapped test).
type attackValidation struct {
	ac      attackConstraints
	options []attackOption
}

// newAttackValidation builds the constraints of the current board.
func (g *Game) newAttackValidation() (attackValidation, error) {
	ac, err := g.newAttackConstraints()
	if err != nil {
		return attackValidation{}, err
	}
	options, err := g.sortedFilteredRequirements(ac)
	if err != nil {
		return attackValidation{}, err
	}
	return attackValidation{ac: ac, options: options}, nil
}

// checkAttackers is CombatUtil.validateAttackers (CombatUtil.java:82):
// the declared attack may leave no more requirements unmet than the best
// attack getLegalAttackers finds -- the one collectLegalAttackers builds, or
// nobody attacking, whichever violates fewer. On failure the error names
// every creature the declaration leaves with more unmet requirements than
// the best attack does. Attack costs are read live, as Java's getAttackCost
// call inside collectLegalAttackers is.
func (g *Game) checkAttackers(v attackValidation, attackers []CardID, targets map[CardID]EntityID) error {
	ac := v.ac
	declared := make(map[CardID]EntityID, len(attackers))
	for _, id := range attackers {
		declared[id] = targets[id]
	}
	best, err := g.legalAttack(ac, v.options)
	if err != nil {
		return err
	}
	bestN, err := g.countViolations(ac, best)
	if err != nil {
		return err
	}
	empty, err := g.countViolations(ac, nil)
	if err != nil {
		return err
	}
	if empty < bestN {
		best, bestN = nil, empty
	}
	declaredN, err := g.countViolations(ac, declared)
	if err != nil {
		return err
	}
	if declaredN <= bestN {
		return nil
	}
	var cards []CardID
	for _, r := range ac.requirements {
		d, attacking := declared[r.attacker]
		bd, bestAttacking := best[r.attacker]
		dv, err := g.requirementViolations(&r, d, attacking, declared)
		if err != nil {
			return err
		}
		bv, err := g.requirementViolations(&r, bd, bestAttacking, best)
		if err != nil {
			return err
		}
		if dv > bv {
			cards = append(cards, r.attacker)
		}
	}
	return illegal("CR 508.1d", "attack requirements left unmet that could have been met", cards...)
}

// eachCombatStatic calls f for every static ability of the given Mode$ on a
// traitHost (battlefield permanents and Command-zone effect cards) whose
// conditions hold, in player then host order, stopping at f's first error.
// A param staticCombatParams does not allow, or a Condition$ value
// continuousConditionMet does not know, is an error: the static would
// otherwise be silently dropped or applied without its restriction (GO-7).
func (g *Game) eachCombatStatic(mode string, f func(h *Card, face *compile.Face, s *compile.Ability) error) error {
	for _, pid := range g.Players() {
		for _, host := range g.traitHosts(pid) {
			h := g.Card(host)
			if h.Def == nil {
				continue
			}
			for fi := range h.traitFaces() {
				face := &h.traitFaces()[fi]
				for _, s := range face.Statics {
					if !strings.EqualFold(s.Name, mode) {
						continue
					}
					ok, err := combatStaticConditionsMet(g, h, face, s)
					if err != nil {
						return err
					}
					if !ok {
						continue
					}
					if err := f(h, face, s); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// staticCombatParams are the params a combat-requirement static may carry
// and this port resolves: the mode, what it applies to, the MustAttack$
// target, its descriptions, AffectedZone$ Battlefield (where every creature
// it could apply to already is), Condition$ (continuousConditionMet's
// values) and IsPresent$ with its PresentCompare$/PresentZone$/
// PresentPlayer$ (isPresentMatches), and MinMaxBlocker's Min$/Max$.
var staticCombatParams = [...]string{
	"Mode", "ValidCreature", "ValidCard", "ValidAttacker", "ValidPlayer", "MustAttack", "Description", "Secondary", "AffectedZone",
	"Condition", "IsPresent", "PresentCompare", "PresentZone", "PresentPlayer", "Min", "Max",
}

// combatStaticConditionsMet is StaticAbility.checkConditions for the
// combat-requirement modes: every param allowed (staticCombatParams),
// Condition$ one continuousConditionMet knows, then both gates evaluated.
func combatStaticConditionsMet(g *Game, h *Card, face *compile.Face, s *compile.Ability) (bool, error) {
	for _, p := range s.Params {
		known := false
		for _, k := range staticCombatParams {
			if strings.EqualFold(p.Key, k) {
				known = true
				break
			}
		}
		// ValidAttacker$ is read by Mode$ AttackRequirement alone, ValidPlayer$
		// by Mode$ PlayerMustAttack alone (StaticAbilityMustAttack.java).
		if strings.EqualFold(p.Key, "ValidAttacker") && !strings.EqualFold(s.Name, "AttackRequirement") ||
			strings.EqualFold(p.Key, "ValidPlayer") && !strings.EqualFold(s.Name, "PlayerMustAttack") {
			known = false
		}
		if !known {
			return false, fmt.Errorf("engine: Mode$ %s: %s$ not resolvable yet", s.Name, p.Key)
		}
	}
	if z, ok := s.Param("AffectedZone"); ok && z != "Battlefield" {
		return false, fmt.Errorf("engine: Mode$ %s: AffectedZone$ %q not resolvable yet", s.Name, z)
	}
	if c, ok := s.Param("Condition"); ok && !knownStaticCondition(c) {
		return false, fmt.Errorf("engine: Mode$ %s: Condition$ %q not resolvable yet", s.Name, c)
	}
	if !continuousConditionMet(g, h, s) {
		return false, nil
	}
	return isPresentMatches(g, h, face.Amounts, s, "IsPresent", "PresentCompare", "PresentDefined", "PresentZone", "PresentPlayer"), nil
}

// knownStaticCondition reports whether continuousConditionMet evaluates c
// rather than reading it as unmet.
func knownStaticCondition(c string) bool {
	switch c {
	case "PlayerTurn", "NotPlayerTurn", "Threshold", "Hellbent", "Metalcraft", "Delirium", "FatefulHour",
		"Blessing", "Monarch", "EnduringStory":
		return true
	}
	return false
}

// staticValidMatches is StaticAbility.matchesValidParam(key, card): an
// absent param matches every card.
func staticValidMatches(g *Game, h *Card, s *compile.Ability, key string, card CardID) bool {
	spec, ok := s.Param(key)
	if !ok {
		return true
	}
	return Matches(g, g.Card(card), valid.Parse(spec), h.Controller(), h.ID)
}
