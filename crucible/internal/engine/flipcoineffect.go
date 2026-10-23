package engine

//enginelint:allow ability additional card condition control defined effecthelpers game id parts zone

import (
	"fmt"
	"strings"
)

// flipCoinEffect is FlipCoinEffect.java (CR 705): each Flipper$ player
// (default You) flips Amount$ (default 1) coins -- again after each win
// with FlipUntilYouLose$ -- calling each one first unless NoCall$. A called
// flip resolves WinSubAbility$ once if any was won, with SVar Wins set to
// the count, and LoseSubAbility$ likewise with Losses; NoCall$ resolves
// HeadsSubAbility$/TailsSubAbility$ instead, their count in
// SaveNumFlipsToSVar$ (default X) when Amount$ is named.
// RememberWinner$/RememberLoser$ remember the flipper on the host. Each
// flip is one nextBoolean on the game's stream.
//
// RememberResult$ (NoCall$ only) records each flipper's result for Defined$
// FlippedHeads/FlippedTails until Cleanup's ClearCoinFlips$; RememberNumber$
// Wins/Losses remembers the count as a number once the sub-abilities ran.
// ForEachPlayer$ (without NoCall$) makes each flipper flip once per player
// it names, Amount$ unread, and resolves WinSubAbility$/LoseSubAbility$ once
// with Remembered set to the players won or lost for (SVar Wins/Losses the
// counts). A flip
// fails closed while while a FlipCoinMod/FlipCoinDoubler static (Krark's Thumb, a
// fixed result) is in play: this port evaluates neither static's
// conditions nor the chooseFlipResult decision a doubled flip needs.
type flipCoinEffect struct{}

func (flipCoinEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	if flipCoinModInPlay(g) {
		return fmt.Errorf("engine: FlipCoin: a FlipCoinMod static is in play, not resolvable yet")
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	flipperSpec, ok := a.Params.Param("Flipper")
	if !ok {
		flipperSpec = "You"
	}
	flippers, err := definedPlayers(g, a.Controller, a.Source, flipperSpec, a.refs())
	if err != nil {
		return fmt.Errorf("engine: FlipCoin: %w", err)
	}
	amount, err := optionalAmount(g, a, "FlipCoin", "Amount", 1)
	if err != nil {
		return err
	}
	noCall := hasParam(a, "NoCall")
	untilLose := hasParam(a, "FlipUntilYouLose")
	varName, ok := a.Params.Param("SaveNumFlipsToSVar")
	if !ok {
		varName = "X"
	}

	forEach, hasForEach := a.Params.Param("ForEachPlayer")
	for _, flipper := range flippers {
		if hasForEach && !noCall {
			if err := g.flipForEachPlayer(a, controller, flipper, forEach); err != nil {
				return err
			}
			continue
		}
		wins := 0
		for {
			won := false
			for i := 0; i < amount; i++ {
				call := true
				if !noCall {
					call = controller.CallCoinFlip(g, flipper, a.Source)
				}
				won = g.rand.Bool() == call
				if won {
					wins++
				}
				if !noCall {
					g.checkFlippedCoinTriggers(controller, flipper, won)
				}
			}
			if !untilLose || !won {
				break
			}
		}
		losses := wins - amount
		if losses < 0 {
			losses = -losses
		}
		winKey, loseKey := "WinSubAbility", "LoseSubAbility"
		winVar, loseVar := "Wins", "Losses"
		if noCall {
			winKey, loseKey = "HeadsSubAbility", "TailsSubAbility"
			winVar, loseVar = "", ""
			if hasParam(a, "Amount") {
				winVar, loseVar = varName, varName
			}
		}
		if wins > 0 {
			if noCall && hasParam(a, "RememberResult") {
				// FlipCoinEffect.java:66-68, before the sub-ability reads it.
				source.Memory.AddFlipResult(flipper, "Heads")
			}
			if !noCall && hasParam(a, "RememberWinner") {
				source.Memory.Remember(PlayerEntity(flipper))
			}
			if err := g.resolveFlipSub(a, controller, winKey, winVar, wins); err != nil {
				return err
			}
		}
		if losses > 0 {
			if noCall && hasParam(a, "RememberResult") {
				source.Memory.AddFlipResult(flipper, "Tails")
			}
			if !noCall && hasParam(a, "RememberLoser") {
				source.Memory.Remember(PlayerEntity(flipper))
			}
			if err := g.resolveFlipSub(a, controller, loseKey, loseVar, losses); err != nil {
				return err
			}
		}
		if raw, ok := a.Params.Param("RememberNumber"); ok && !noCall {
			// FlipCoinEffect.java:154-161: the win or loss count, by the word's
			// first letters; any other word remembers nothing.
			switch {
			case strings.HasPrefix(raw, "Win"):
				source.Memory.RememberNumber(wins)
			case strings.HasPrefix(raw, "Loss"):
				source.Memory.RememberNumber(losses)
			}
		}
		if amount > 0 {
			g.checkFlippedCoinOnceTriggers(controller, flipper)
		}
	}
	return nil
}

// flipForEachPlayer is FlipCoinEffect.java:92-129: flipper calls and flips
// once for each ForEachPlayer$ player; the win sub-ability runs with the
// players won for as the host's only Remembered, the lose one with those
// lost for, then the host's own memory comes back.
func (g *Game) flipForEachPlayer(a *Ability, controller PlayerController, flipper PlayerID, spec string) error {
	players, err := definedPlayers(g, a.Controller, a.Source, spec, a.refs())
	if err != nil {
		return fmt.Errorf("engine: FlipCoin: %w", err)
	}
	var wonFor, lostFor []PlayerID
	for _, p := range players {
		call := controller.CallCoinFlip(g, flipper, a.Source)
		won := g.rand.Bool() == call
		g.checkFlippedCoinTriggers(controller, flipper, won)
		g.checkFlippedCoinOnceTriggers(controller, flipper)
		if won {
			wonFor = append(wonFor, p)
		} else {
			lostFor = append(lostFor, p)
		}
	}
	source := g.Card(a.Source)
	for _, side := range [...]struct {
		key, name string
		set       []PlayerID
	}{{"WinSubAbility", "Wins", wonFor}, {"LoseSubAbility", "Losses", lostFor}} {
		if len(side.set) == 0 {
			continue
		}
		saved := source.Memory.Remembered()
		source.Memory.ClearRemembered()
		for _, p := range side.set {
			source.Memory.Remember(PlayerEntity(p))
		}
		err := g.resolveFlipSub(a, controller, side.key, side.name, len(side.set))
		source.Memory.ClearRemembered()
		for _, e := range saved {
			source.Memory.Remember(e)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// resolveFlipSub resolves parent's key AdditionalAbility, if it has one,
// with SVar name (when set) reading n -- Java's sub.setSVar(name,
// "Number$" + n).
func (g *Game) resolveFlipSub(parent *Ability, controller PlayerController, key, name string, n int) error {
	subs := additionalAbilities(parent.Params, key)
	if len(subs) == 0 {
		return nil
	}
	p := *parent
	if name != "" {
		p.Amounts = withAmount(parent.Amounts, name, n)
	}
	return g.resolveAdditional(&p, controller, subs[0])
}

// flipCoinModInPlay reports whether any card in a static-ability zone
// carries a Mode$ FlipCoinMod (fixed result) or FlipCoinDoubler static
// (StaticAbilityFlipCoinMod's scan).
func flipCoinModInPlay(g *Game) bool {
	for _, p := range g.Players() {
		for _, z := range []ZoneType{Battlefield, Command, Graveyard, Exile, Hand} {
			for _, id := range g.Zone(z, p).Cards() {
				c := g.Card(id)
				if c.Def == nil {
					continue
				}
				for _, face := range c.traitFaces() {
					for _, s := range face.Statics {
						if strings.EqualFold(s.Name, "FlipCoinMod") || strings.EqualFold(s.Name, "FlipCoinDoubler") {
							return true
						}
					}
				}
			}
		}
	}
	return false
}
