// SetLife: CR 119.4, M6's own next script-driven effect -- 39 real
// (AB|DB)$ SetLife lines, 31 naming Defined$ and 5 naming ValidTgts$
// instead -- SpellAbilityEffect.getTargetPlayers(sa)'s own either/or
// contract, targetedOrDefinedPlayers (defined.go), millEffect's own first
// caller (milleffect.go) reused wholesale here, "You" the identical default
// when neither is present.
//
// Ported from
// forge-game/src/main/java/forge/game/ability/effects/LifeSetEffect.java's
// resolve (the plain branch; Redistribute$'s own multi-player life-swap
// shape, below, is not) and Player.setLife's own CR 119.5 dispatch: a life
// total set ABOVE its current value is a life GAIN (gainLifeEffect's own
// gainLifePrevented/gainLifeReplaced/checkLifeGainedTriggers machinery,
// gainlifeeffect.go, reused directly rather than re-derived -- Player.
// setLife's own Java body IS exactly `newLife > life ? gainLife(...) :
// newLife < life ? loseLife(...) : no-op`, so this file has nothing of its
// own to port for either direction); set BELOW is a life LOSS
// (loseLifeEffect's own plain `Life -= amount`, no prevention machinery
// built, loselifeeffect.go's own identical gap); set to the SAME value is
// no event at all, CR 119.5's own explicit carve-out.

package engine

//enginelint:allow id card game player ability defined condition control amount event

import (
	"fmt"
	"slices"
)

// setLifeEffect resolves Mode$/DB$/AB$ SetLife for the plain, non-
// Redistribute$ shape. ConditionPresent$/ConditionCompare$/
// ConditionCheckSVar$/ConditionSVarCompare$ are resolved through
// subAbilityConditionMet (condition.go), the identical way every other M6
// effect's own does.
type setLifeEffect struct{}

func (setLifeEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	_, redistribute := a.Params.Param("Redistribute")
	newLife := 0
	if !redistribute {
		lifeAmount, ok := a.Params.Param("LifeAmount")
		if !ok {
			return fmt.Errorf("engine: SetLife: LifeAmount$ missing")
		}
		newLife, ok = resolveNamedAmount(g, a.Amounts, source, lifeAmount)
		if !ok {
			return fmt.Errorf("engine: SetLife: LifeAmount$ %q is not resolvable", lifeAmount)
		}
	}

	var players []PlayerID
	var err error
	if choicesSpec, ok := a.Params.Param("PlayerChoices"); ok {
		players, err = g.setLifeChosenPlayers(a, controller, choicesSpec)
	} else {
		players, err = targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.refs())
	}
	if err != nil {
		return fmt.Errorf("engine: SetLife: %w", err)
	}
	if redistribute {
		return g.redistributeLife(a, controller, players)
	}

	for _, pid := range players {
		// LifeSetEffect.java:72-74 skips a player no longer in the game.
		if g.Player(pid).Lost {
			continue
		}
		setPlayerLife(g, controller, a.Source, pid, newLife)
	}
	return nil
}

// setPlayerLife is Player.setLife's own CR 119.5 dispatch -- above zero
// cards more than its current life is a GAIN (gainLifeEffect's own
// gainLifePrevented/gainLifeReplaced/checkLifeGainedTriggers machinery,
// gainlifeeffect.go, reused directly), below is a LOSS (loseLifeEffect's
// own plain subtract, no prevention machinery built, loselifeeffect.go's
// own identical gap), and the same value is no event at all. Shared with
// exchangeLifeEffect (exchangelifeeffect.go), CR 119.10's own "the loser of
// the exchange has their life SET to the winner's post-loss total" reduced
// to the identical two-branch dispatch.
func setPlayerLife(g *Game, controller PlayerController, source CardID, pid PlayerID, newLife int) {
	p := g.Player(pid)
	switch {
	case newLife > p.Life:
		g.gainLife(controller, pid, newLife-p.Life, source, nil)
	case newLife < p.Life:
		lost := p.Life - newLife
		p.Life = newLife
		g.sink.Emit(Event{Kind: LifeChanged, Source: source, Target: PlayerEntity(pid), Amount: -int32(lost)})
	}
}

// setLifeChosenPlayers is LifeSetEffect.java:35-52: the activator picks from
// PlayerChoices$ (a Defined$ player list) as many as ChoiceAmount$ names,
// exactly one when it is absent, any number (zero included) for Any.
// ChoicePrompt$ is the title of that question and changes nothing.
func (g *Game) setLifeChosenPlayers(a *Ability, controller PlayerController, spec string) ([]PlayerID, error) {
	choices, err := definedPlayers(g, a.Controller, a.Source, spec, a.refs())
	if err != nil {
		return nil, err
	}
	n, lo := 1, 1
	if raw, ok := a.Params.Param("ChoiceAmount"); ok {
		if raw == "Any" {
			n, lo = len(choices), 0
		} else {
			var ok bool
			if n, ok = resolveNamedAmount(g, a.Amounts, g.Card(a.Source), raw); !ok {
				return nil, fmt.Errorf("ChoiceAmount$ %q is not resolvable", raw)
			}
			lo = n
		}
	}
	options := make([]EntityID, len(choices))
	for i, p := range choices {
		options[i] = PlayerEntity(p)
	}
	picked := controller.ChooseEntitiesForEffect(g, a.Controller, a.Source, options, lo, n)
	if len(picked) < lo || len(picked) > n {
		return nil, fmt.Errorf("%d players chosen, want %d to %d", len(picked), lo, n)
	}
	out := make([]PlayerID, 0, len(picked))
	for _, e := range picked {
		p, ok := e.AsPlayer()
		if !ok || !slices.Contains(choices, p) || slices.Contains(out, p) {
			return nil, fmt.Errorf("chosen entity %v is not one of the offered players", e)
		}
		out = append(out, p)
	}
	return out, nil
}

// redistributeLife is LifeSetEffect.java:61-93 for Redistribute$: the chosen
// players' life totals are shuffled among them. Each player in turn gets one
// of the totals still unassigned, the activator naming which among those that
// leave a legal distribution of the rest (getDistribution); a player who cannot
// gain or lose life keeps what a legal answer allows (CR 119.7, 119.8). The
// batched LifeLostAll event Java fires afterwards has no matcher in this port
// (lifelost.go), and its two corpus listeners are not played.
//
// Java drops a player no longer in the game from the iteration but leaves it
// in the player list getDistribution walks and its total in the pool (:63-65,
// :83); reproduced (PORT-7).
func (g *Game) redistributeLife(a *Ability, controller PlayerController, players []PlayerID) error {
	var totals []int
	for _, p := range players {
		if !g.Player(p).Lost {
			totals = append(totals, g.Player(p).Life)
		}
	}
	pending := slices.Clone(players)
	for _, p := range players {
		if g.Player(p).Lost {
			continue
		}
		valid := g.lifeDistribution(pending, true, slices.Clone(totals))
		if len(valid) == 0 {
			return fmt.Errorf("engine: SetLife: no legal redistribution of the life totals")
		}
		lo, hi := slices.Min(valid), slices.Max(valid)
		life := controller.ChooseNumber(g, a.Controller, a.Source, lo, hi)
		if !slices.Contains(valid, life) {
			return fmt.Errorf("engine: SetLife: %d is not one of the legal life totals %v", life, valid)
		}
		setPlayerLife(g, controller, a.Source, p, life)
		totals = slices.Delete(totals, slices.Index(totals, life), slices.Index(totals, life)+1)
		pending = slices.DeleteFunc(pending, func(q PlayerID) bool { return q == p })
	}
	return nil
}

// lifeDistribution is LifeSetEffect.getDistribution: the totals among
// remaining the first player could take so that the others can still be given
// the rest without breaking CR 119.7/119.8 (a player who cannot gain life
// cannot end above, one who cannot lose it below, where it stands). At the top
// of the recursion it is the list of legal answers for the first player;
// deeper it carries a non-empty list back when a distribution of the rest exists.
func (g *Game) lifeDistribution(players []PlayerID, top bool, remaining []int) []int {
	if len(players) == 0 {
		// Java signals success by appending a sentinel 1.
		return append(remaining, 1)
	}
	valid := slices.Clone(remaining)
	for _, p := range players {
		life := g.Player(p).Life
		for _, choice := range remaining {
			if (life < choice && g.cantGainLife(p)) || (life > choice && g.cantLoseLife(p)) {
				if top {
					valid = removeFirstInt(valid, choice)
				}
				continue
			}
			next := slices.DeleteFunc(slices.Clone(players), func(q PlayerID) bool { return q == p })
			nextChoices := removeFirstInt(slices.Clone(remaining), choice)
			nextChoices = g.lifeDistribution(next, false, nextChoices)
			if len(nextChoices) == 0 {
				if top {
					valid = removeFirstInt(valid, choice)
				}
			} else if !top {
				return nextChoices
			}
		}
		if top {
			return valid
		}
	}
	return nil
}

// removeFirstInt is List.remove(Object): the first element equal to v goes.
func removeFirstInt(list []int, v int) []int {
	if i := slices.Index(list, v); i >= 0 {
		return slices.Delete(list, i, i+1)
	}
	return list
}
