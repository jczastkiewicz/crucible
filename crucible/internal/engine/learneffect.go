package engine

//enginelint:allow game ability control effecthelpers card condition defined player id zone discardeffect scheduledaction valid replacementevents

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// learnEffect is LearnEffect.java through Player.learnLesson: each target
// player (the activator by default) may reveal a Lesson card from their
// sideboard and put it into their hand, or discard a card to draw a card,
// or do neither -- one pick among their sideboard Lessons and hand cards.
// A player another player controls is offered their hand only.
// A Learn replacement (Retriever Phoenix) replaces the whole learn for its
// player (Player.learnLesson, Player.java:3940-3943).
type learnEffect struct{}

func (learnEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	if err := rejectParams(a, "Learn"); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	players, err := targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: Learn: %w", err)
	}
	for _, p := range players {
		if g.Player(p).Lost {
			continue
		}
		if g.learnReplaced(controller, p) {
			continue
		}
		var list []CardID
		// Player.java:3906: a player another player controls cannot reach
		// outside the game (CR 800.4b, ADR-0030).
		if !g.IsControlled(p) {
			for _, id := range g.Zone(Sideboard, p).Cards() {
				if g.Card(id).Type().HasSubtype("Lesson") {
					list = append(list, id)
				}
			}
		}
		list = append(list, g.Zone(Hand, p).Cards()...)
		if len(list) == 0 {
			continue
		}
		chosen := controller.ChooseCardsForEffect(g, p, a.Source, list, 0, 1)
		if err := checkChoice(chosen, list, 0, 1); err != nil {
			return fmt.Errorf("engine: Learn: %w", err)
		}
		if len(chosen) == 0 {
			continue
		}
		c := chosen[0]
		if g.Card(c).Zone == Sideboard {
			g.moveByEffect(controller, c, Hand, 0, NoPlayer, false)
			g.checkChangesZoneAllTriggers(controller, []CardID{c}, Sideboard, Hand)
			continue
		}
		discardCards(g, controller, []CardID{c}, p)
		g.DrawCards(p, 1, controller)
	}
	return nil
}

// learnReplaced is ReplacementType.Learn's run for player p
// (Player.java:3940-3943): a line whose ValidPlayer$ matches p replaces the
// learn by its ReplaceWith$ ability, after the Optional$ confirmation.
func (g *Game) learnReplaced(controller PlayerController, p PlayerID) bool {
	return g.runBeginReplacements(controller, p, "Learn", []string{"validplayer", "optional", "optionaldecider"},
		func(h *Card, r *compile.Ability) bool {
			if v, ok := r.Param("ValidPlayer"); ok {
				if matched, recognized := matchesPlayerSpec(g, p, h.Controller(), h.ID, v); !recognized || !matched {
					return false
				}
			}
			return true
		})
}
