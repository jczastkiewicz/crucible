package engine

//enginelint:allow id zone card game player ability defined condition control parts valid effecthelpers phasing takeinitiativeeffect

import (
	"fmt"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/valid"
)

// phasesEffect is PhasesEffect.java: permanents phase out (CR 702.26) --
// or, with PhaseInOrOut$, each one phases the other way, the phased-out
// ones back in (Time and Tide, Oubliette's return). WontPhaseInNormal$ keeps
// a permanent it phased out from phasing in on its controller's untap step;
// Tapped$/Untapped$ set a permanent's tapped state as PhaseInOrOut$ phases
// it in, without a tap or untap trigger.
//
// Which permanents: AllValid$ (every battlefield permanent matching it --
// phased-out ones included under PhaseInOrOut$, PhasesEffect.java:47-48,
// ADR-0021's Phases opt-in), else Defined$ (getDefinedCardsOrTargeted,
// Defined$ ahead of targets), else the chosen targets, else the host.
// AnyNumber$ lets the activator pick any number of them. RememberAffected$
// remembers each permanent it phased out on the host, RememberValids$ every
// permanent it considered.
//
// A permanent no longer on the battlefield is skipped: Java's
// getCardState/equalsWithGameTimestamp check (PhasesEffect.java:75-79,
// :98-102), less the timestamp half -- a CardID survives a zone change here
// (ADR-0009), so a permanent that left and came back reads as the same one.
//
// Ported from forge-game/src/main/java/forge/game/ability/effects/PhasesEffect.java's resolve.
type phasesEffect struct{}

func (phasesEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	phaseInOrOut := hasParam(a, "PhaseInOrOut")
	wontPhaseInNormal := hasParam(a, "WontPhaseInNormal")

	cards, err := phasesCards(g, a, phaseInOrOut)
	if err != nil {
		return err
	}
	if hasParam(a, "AnyNumber") {
		chosen := controller.ChooseCardsForEffect(g, a.Controller, a.Source, cards, 0, len(cards))
		if err := checkChoice(chosen, cards, 0, len(cards)); err != nil {
			return fmt.Errorf("engine: Phases: AnyNumber$: %w", err)
		}
		cards = chosen
	}

	b := g.beginPhasing()
	var phasedOut []CardID
	if phaseInOrOut {
		// Every permanent's own direction is decided before any of them
		// phases (PhasesEffect.java:63-73).
		var toPhase []CardID
		for _, id := range cards {
			c := g.Card(id)
			if c.IsPhasedOut() && g.cantPhase(id, "CantPhaseIn") || !c.IsPhasedOut() && g.cantPhase(id, "CantPhaseOut") {
				continue
			}
			toPhase = append(toPhase, id)
		}
		_, tapped := a.Params.Param("Tapped")
		_, untapped := a.Params.Param("Untapped")
		for _, id := range toPhase {
			c := g.Card(id)
			if c.Zone != Battlefield {
				continue
			}
			g.phase(b, id, false, true)
			if c.IsPhasedOut() {
				phasedOut = append(phasedOut, id)
				c.wontPhaseInNormal = wontPhaseInNormal
				continue
			}
			switch {
			case tapped:
				c.Tapped = true
			case untapped:
				c.Tapped = false
			}
			c.wontPhaseInNormal = false
		}
	} else {
		_, remember := a.Params.Param("RememberAffected")
		for _, id := range cards {
			c := g.Card(id)
			if c.Zone != Battlefield || c.IsPhasedOut() || g.cantPhase(id, "CantPhaseOut") {
				continue
			}
			g.phase(b, id, false, true)
			if !c.IsPhasedOut() {
				continue
			}
			if remember {
				g.Card(a.Source).Memory.Remember(CardEntity(id))
			}
			phasedOut = append(phasedOut, id)
			c.wontPhaseInNormal = wontPhaseInNormal
		}
	}
	if hasParam(a, "RememberValids") {
		for _, id := range cards {
			g.Card(a.Source).Memory.Remember(CardEntity(id))
		}
	}
	g.endPhasing(controller, b, phasedOut)
	return nil
}

// phasesCards is the permanents Phases considers, in order: AllValid$, else
// getDefinedCardsOrTargeted -- Defined$ (each " & " part) when present, the
// chosen targets when not, the host when neither.
func phasesCards(g *Game, a *Ability, phaseInOrOut bool) ([]CardID, error) {
	if spec, ok := a.Params.Param("AllValid"); ok {
		return phasesValidCards(g, a, spec, phaseInOrOut)
	}
	defined, ok := a.Params.Param("Defined")
	if !ok {
		if _, targeted := a.Params.Param("ValidTgts"); targeted {
			var cards []CardID
			for _, e := range a.Targets {
				if id, ok := e.AsCard(); ok {
					cards = append(cards, id)
				}
			}
			return cards, nil
		}
		defined = "Self"
	}
	var cards []CardID
	for _, part := range strings.Split(defined, " & ") {
		part = strings.TrimSpace(part)
		if rest, ok := strings.CutPrefix(part, "Valid "); ok {
			found, err := phasesValidCards(g, a, rest, false)
			if err != nil {
				return nil, err
			}
			cards = append(cards, found...)
			continue
		}
		found, err := definedCards(g.Card(a.Source), part, a.refs())
		if err != nil {
			return nil, fmt.Errorf("engine: Phases: %w", err)
		}
		cards = append(cards, found...)
	}
	return cards, nil
}

// phasesValidCards is every battlefield permanent matching spec, seat then
// zone order -- phased-out ones included only when includePhasedOut
// (Game.getCardsIncludePhasingIn, PhasesEffect.java:48).
//
// TargetedPlayerCtrl (CardProperty.java:277-280) needs the ability's own
// targets, which Matches does not see, so it is answered here: an
// alternative carrying it matches a permanent whose controller is one of
// the targeted players, and only a phased-in one (CardProperty.java:50's
// guard). Negated, it is not resolvable yet.
func phasesValidCards(g *Game, a *Ability, spec string, includePhasedOut bool) ([]CardID, error) {
	parsed := valid.Parse(spec)
	type alternative struct {
		alt                valid.Alternative
		targetedPlayerCtrl bool
	}
	alts := make([]alternative, 0, len(parsed.Alternatives))
	for _, alt := range parsed.Alternatives {
		out := alternative{alt: valid.Alternative{Base: alt.Base}}
		for _, p := range alt.Properties {
			if p.Name != "TargetedPlayerCtrl" {
				out.alt.Properties = append(out.alt.Properties, p)
				continue
			}
			if p.Negated || alt.Base.Negated {
				return nil, fmt.Errorf("engine: Phases: %q: a negated TargetedPlayerCtrl not resolvable yet", spec)
			}
			out.targetedPlayerCtrl = true
		}
		alts = append(alts, out)
	}
	var targeted []PlayerID
	for _, e := range a.Targets {
		if pid, ok := e.AsPlayer(); ok {
			targeted = append(targeted, pid)
		}
	}
	var cards []CardID
	for _, pid := range g.Players() {
		z := g.Zone(Battlefield, pid)
		zoneCards := z.Cards()
		if includePhasedOut {
			zoneCards = z.CardsIncludingPhasedOut()
		}
		for _, id := range zoneCards {
			c := g.Card(id)
			for _, alt := range alts {
				if !altMatches(g, c, alt.alt, a.Controller, a.Source) {
					continue
				}
				if alt.targetedPlayerCtrl && (c.IsPhasedOut() || !containsPlayer(targeted, c.Controller())) {
					continue
				}
				cards = append(cards, id)
				break
			}
		}
	}
	return cards, nil
}
