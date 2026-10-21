package engine

//enginelint:allow ability card condition control defined effecthelpers game phase player turn untilcommand controlcommands

import (
	"fmt"
	"strings"
)

// skipPhaseEffect is SkipPhaseEffect.java: each target or Defined$ player
// (default You) skips their next Phase$/Step$ (PhaseType.parseRange --
// "Main" is both main phases, BeginCombat the whole combat phase). Duration$
// EndOfTurn skips every such phase for the rest of the turn; NextThisTurn
// skips the next one and lapses at end of turn. Start$ FromNext with Duration$
// UntilTheEndOfTargetedNextTurn starts the skip at the player's next upkeep
// (Empty City Ruse, False Peace); any other Start$ fails closed.
type skipPhaseEffect struct{}

func (skipPhaseEffect) Resolve(g *Game, a *Ability, _ PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	name, ok := a.Params.Param("Phase")
	if !ok {
		name, ok = a.Params.Param("Step")
	}
	if !ok {
		return fmt.Errorf("engine: SkipPhase: no Phase$ or Step$")
	}
	phases, ok := parsePhaseRange(name)
	if !ok {
		return fmt.Errorf("engine: SkipPhase: phase %q not resolvable", name)
	}
	rec := skipPhase{Phases: phases}
	start := false
	if st, ok := a.Params.Param("Start"); ok {
		if d, _ := a.Params.Param("Duration"); st != "FromNext" || d != "UntilTheEndOfTargetedNextTurn" {
			return fmt.Errorf("engine: SkipPhase: Start$ %q with Duration$ %q not resolvable yet", st, d)
		}
		start = true
	}
	if d, ok := a.Params.Param("Duration"); ok {
		switch {
		case start:
			// Each phase of the targeted player's next turn, then it lapses.
			rec.Each, rec.UntilCleanup = true, true
		case d == "NextThisTurn":
			rec.UntilCleanup = true
		case strings.EqualFold(d, "EndOfTurn"):
			rec.Each, rec.UntilCleanup = true, true
		default:
			return fmt.Errorf("engine: SkipPhase: Duration$ %q not resolvable yet", d)
		}
	}
	players, err := targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.refs())
	if err != nil {
		return err
	}
	for _, p := range g.inAPNAPOrder(players) {
		r := rec
		r.Player = p
		if start {
			// SkipPhaseEffect.java:99-100: the effect enters the Command zone
			// when p's upkeep begins.
			g.timestamp++
			g.pendingSkips = append(g.pendingSkips, pendingSkip{ID: g.timestamp, Skip: r})
			g.phaseCmds = append(g.phaseCmds, phaseCommand{
				cardCommand: cardCommand{Kind: commandBeginSkip, Timestamp: g.timestamp, Host: a.Source},
				When:        untilUpkeepBegin, Player: p,
			})
			continue
		}
		g.skips = append(g.skips, r)
	}
	return nil
}
