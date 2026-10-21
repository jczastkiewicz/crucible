package engine

//enginelint:allow card game ability defined condition control amount parts id additional effecthelpers

import (
	"fmt"
	"math"
)

// chooseNumberUnresolvedParams are ChooseNumberEffect.java's params this
// port cannot honour yet: the Guesser$ branch (GuessCorrect$/GuessWrong$,
// which also needs the RemoveChoices$ list), RemoveChoices$ (reads remembered
// Integers, which Memory cannot hold), and RememberChosen$ (in Java it only
// ever adds the Secretly$ map's Integer values, again not an EntityID).
// Random$ is MyRandom's nextInt(max-min+1) + min (ChooseNumberEffect.java:65-66).
var chooseNumberUnresolvedParams = [...]string{
	"Guesser", "GuessCorrect", "GuessWrong", "RemoveChoices", "RememberChosen"}

// secretNumber is one player's entry in Java's chooseMap.
type secretNumber struct {
	player PlayerID
	n      int
}

// chooseNumberEffect is ChooseNumberEffect.java's open (non-secret) shape:
// each chooser (Defined$, default You) picks an integer between Min$
// (default 0) and Max$ (default 99) and the host records it
// (Memory.SetChosenNumber). ChooseAnyNumber$ is Java's announceRequirements
// path, the identical [lo, hi] contract, so it shares this one decision.
type chooseNumberEffect struct{}

func (chooseNumberEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	for _, key := range chooseNumberUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: ChooseNumber: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	lo, err := chooseNumberBound(g, a, source, "Min", "0")
	if err != nil {
		return err
	}
	hi, err := chooseNumberBound(g, a, source, "Max", "99")
	if err != nil {
		return err
	}
	choosers, err := targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: ChooseNumber: %w", err)
	}
	_, random := a.Params.Param("Random")
	if random && hi < lo {
		// Random.nextInt throws on a non-positive bound (GO-7: an error, not
		// a panic).
		return fmt.Errorf("engine: ChooseNumber: Random$ between %d and %d", lo, hi)
	}
	_, secretly := a.Params.Param("Secretly")
	_, keepSecret := a.Params.Param("KeepSecret")
	var secrets []secretNumber
	for _, pid := range choosers {
		var n int
		if random {
			n = g.randomInt(lo, hi)
		} else {
			n = controller.ChooseNumber(g, pid, a.Source, lo, hi)
			if n < lo || n > hi {
				return fmt.Errorf("engine: ChooseNumber: controller chose %d, want between %d and %d", n, lo, hi)
			}
		}
		switch {
		case secretly && !keepSecret:
			secrets = append(secrets, secretNumber{pid, n})
		default:
			source.Memory.SetChosenNumber(n)
		}
	}
	if len(secrets) == 0 {
		return nil
	}
	return chooseNumberReveal(g, a, controller, source, secrets)
}

// chooseNumberReveal is ChooseNumberEffect.java:131-208: once every player
// has chosen, the highest and lowest numbers pick who each sub-ability runs
// for, the player remembered and the number chosen while it resolves.
// ChooseNumberSubAbility$ runs for everyone, Lowest$ and Highest$ for the
// tied players at that end, NotLowest$ for the rest, MatchedAbility$ /
// UnmatchedAbility$ once (matched: everyone chose the highest number).
// Java walks a HashMap, so its player order is unspecified; the port uses
// the choosers' own order (quirks.md).
func chooseNumberReveal(g *Game, a *Ability, controller PlayerController, source *Card, secrets []secretNumber) error {
	var highestPlayers, lowestPlayers []secretNumber
	highest, lowest := 0, math.MaxInt
	for _, s := range secrets {
		if s.n > highest {
			highestPlayers, highest = nil, s.n
		}
		if s.n == highest {
			highestPlayers = append(highestPlayers, s)
		}
		if s.n < lowest {
			lowestPlayers, lowest = nil, s.n
		}
		if s.n == lowest {
			lowestPlayers = append(lowestPlayers, s)
		}
	}
	run := func(key string, who []secretNumber, setNumber bool) error {
		for _, sub := range additionalAbilities(a.Params, key) {
			for _, s := range who {
				source.Memory.Remember(PlayerEntity(s.player))
				if setNumber {
					source.Memory.SetChosenNumber(s.n)
				}
				err := g.resolveAdditional(a, controller, sub)
				source.Memory.ClearRemembered()
				if err != nil {
					return err
				}
			}
		}
		return nil
	}
	if hasParam(a, "ChooseNumberSubAbility") {
		if err := run("ChooseNumberSubAbility", secrets, true); err != nil {
			return err
		}
	}
	if hasParam(a, "Lowest") {
		if err := run("Lowest", lowestPlayers, true); err != nil {
			return err
		}
	}
	if hasParam(a, "NotLowest") {
		var rest []secretNumber
		for _, s := range secrets {
			if s.n != lowest {
				rest = append(rest, s)
			}
		}
		if err := run("NotLowest", rest, false); err != nil {
			return err
		}
	}
	if hasParam(a, "Highest") {
		if err := run("Highest", highestPlayers, true); err != nil {
			return err
		}
		if hasParam(a, "RememberHighest") {
			for _, s := range highestPlayers {
				source.Memory.Remember(PlayerEntity(s.player))
			}
		}
	}
	key := "UnmatchedAbility"
	if len(highestPlayers) == len(secrets) {
		key = "MatchedAbility"
	}
	for _, sub := range additionalAbilities(a.Params, key) {
		if err := g.resolveAdditional(a, controller, sub); err != nil {
			return err
		}
	}
	return nil
}

func chooseNumberBound(g *Game, a *Ability, source *Card, key, def string) (int, error) {
	raw, ok := a.Params.Param(key)
	if !ok {
		raw = def
	}
	n, ok := resolveNamedAmount(g, a.Amounts, source, raw)
	if !ok {
		return 0, fmt.Errorf("engine: ChooseNumber: %s$ %q not resolvable yet", key, raw)
	}
	return n, nil
}
