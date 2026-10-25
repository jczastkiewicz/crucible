package engine

//enginelint:allow card game ability defined condition control amount parts id additional effecthelpers

import (
	"fmt"
	"math"
	"slices"
)

// chooseNumberUnresolvedParams are ChooseNumberEffect.java's params this
// port cannot honour yet; none is left. Random$ is MyRandom's
// nextInt(max-min+1) + min (ChooseNumberEffect.java:65-66).
var chooseNumberUnresolvedParams = [...]string{}

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
	// choices is Java's list of numbers RemoveChoices$ leaves, shared by every
	// chooser of the effect (ChooseNumberEffect.java:51-52, :80-90): the Guesser$
	// guesses among it, so without RemoveChoices$ it stays empty and nobody guesses.
	var choices []int
	var guess *numberGuess
	for _, pid := range choosers {
		var n int
		switch {
		case hasParam(a, "RemoveChoices") && !random:
			for i := lo; i <= hi; i++ {
				choices = append(choices, i)
			}
			// "currently we always remove remembered numbers" (:83-85).
			for _, r := range source.Memory.RememberedNumbers() {
				if i := slices.Index(choices, r); i >= 0 {
					choices = slices.Delete(choices, i, i+1)
				}
			}
			if len(choices) == 0 {
				continue
			}
			n = controller.ChooseNumber(g, pid, a.Source, slices.Min(choices), slices.Max(choices))
			if !slices.Contains(choices, n) {
				return fmt.Errorf("engine: ChooseNumber: controller chose %d, not one of %v", n, choices)
			}
		case random:
			n = g.randomInt(lo, hi)
		default:
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
		if raw, ok := a.Params.Param("Guesser"); ok {
			var err error
			if guess, err = g.chooseNumberGuess(a, controller, pid, raw, choices); err != nil {
				return err
			}
		}
	}
	if len(secrets) > 0 {
		if err := chooseNumberReveal(g, a, controller, source, secrets, guess); err != nil {
			return err
		}
	}
	if hasParam(a, "RememberChosen") {
		// :207 adds the chooseMap's Integers to the host's remembered set.
		for _, s := range secrets {
			source.Memory.RememberNumber(s.n)
		}
	}
	return nil
}

// numberGuess is Java's guessPair: who guessed and what.
type numberGuess struct {
	player PlayerID
	n      int
}

// chooseNumberGuess is ChooseNumberEffect.java:113-125: the chooser names the
// guesser among Guesser$'s players (nobody guesses when no choices are left to
// guess among), and the guesser names a number among choices.
func (g *Game) chooseNumberGuess(a *Ability, controller PlayerController, chooser PlayerID, spec string, choices []int) (*numberGuess, error) {
	guessers, err := definedPlayers(g, a.Controller, a.Source, spec, a.refs())
	if err != nil {
		return nil, fmt.Errorf("engine: ChooseNumber: Guesser$: %w", err)
	}
	if len(choices) == 0 || len(guessers) == 0 {
		return nil, nil
	}
	guesser := guessers[0]
	if len(guessers) > 1 {
		options := make([]EntityID, len(guessers))
		for i, p := range guessers {
			options[i] = PlayerEntity(p)
		}
		picked := controller.ChooseEntitiesForEffect(g, chooser, a.Source, options, 1, 1)
		if len(picked) != 1 {
			return nil, fmt.Errorf("engine: ChooseNumber: %d guessers chosen, want 1", len(picked))
		}
		p, ok := picked[0].AsPlayer()
		if !ok || !slices.Contains(guessers, p) {
			return nil, fmt.Errorf("engine: ChooseNumber: chosen guesser %v is not offered", picked[0])
		}
		guesser = p
	}
	n := controller.ChooseNumber(g, guesser, a.Source, slices.Min(choices), slices.Max(choices))
	if !slices.Contains(choices, n) {
		return nil, fmt.Errorf("engine: ChooseNumber: guess %d is not one of %v", n, choices)
	}
	return &numberGuess{player: guesser, n: n}, nil
}

// chooseNumberReveal is ChooseNumberEffect.java:131-208: once every player
// has chosen, the highest and lowest numbers pick who each sub-ability runs
// for, the player remembered and the number chosen while it resolves.
// ChooseNumberSubAbility$ runs for everyone, Lowest$ and Highest$ for the
// tied players at that end, NotLowest$ for the rest, MatchedAbility$ /
// UnmatchedAbility$ once (matched: everyone chose the highest number).
// Java walks a HashMap, so its player order is unspecified; the port uses
// the choosers' own order (quirks.md).
func chooseNumberReveal(g *Game, a *Ability, controller PlayerController, source *Card, secrets []secretNumber, guess *numberGuess) error {
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
	// GuessCorrect$ before GuessWrong$ (:178-191); a wrong guess runs with the
	// guess as the chosen number and the guesser remembered, both dropped after.
	correct := false
	if guess != nil {
		for _, s := range secrets {
			correct = correct || s.n == guess.n
		}
	}
	if correct {
		for _, sub := range additionalAbilities(a.Params, "GuessCorrect") {
			if err := g.resolveAdditional(a, controller, sub); err != nil {
				return err
			}
		}
	}
	if guess != nil && !correct {
		for _, sub := range additionalAbilities(a.Params, "GuessWrong") {
			source.Memory.SetChosenNumber(guess.n)
			source.Memory.Remember(PlayerEntity(guess.player))
			err := g.resolveAdditional(a, controller, sub)
			source.Memory.ClearChosenNumber()
			source.Memory.Forget(PlayerEntity(guess.player))
			if err != nil {
				return err
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
