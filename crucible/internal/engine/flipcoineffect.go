package engine

//enginelint:allow ability additional card condition control defined effecthelpers game id parts zone valid

import (
	"fmt"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
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
// counts). A FlipCoinMod static fixes every flip of a round
// (flipCoinFixedResult); a flip fails closed while a FlipCoinDoubler (Krark's
// Thumb) is in play, whose chooseFlipResult decision is not ported.
type flipCoinEffect struct{}

func (flipCoinEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	if flipCoinDoublerInPlay(g) {
		return fmt.Errorf("engine: FlipCoin: a FlipCoinDoubler static is in play, not resolvable yet")
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
			// FlipCoinEffect.java:177-179: the fixed result is read once per
			// round of flips, so a Two-Headed Coin's "first time" covers them all.
			fixed := g.flipCoinFixedResult(flipper)
			for i := 0; i < amount; i++ {
				won = g.flipOneCoin(controller, a, flipper, noCall, fixed)
				if won {
					wins++
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
		won := g.flipOneCoin(controller, a, flipper, false, g.flipCoinFixedResult(flipper))
		g.checkFlippedCoinOnceTriggers(controller, flipper)
		if won {
			wonFor = append(wonFor, p)
		} else {
			lostFor = append(lostFor, p)
		}
	}
	for _, side := range [...]struct {
		key, name string
		set       []PlayerID
	}{{"WinSubAbility", "Wins", wonFor}, {"LoseSubAbility", "Losses", lostFor}} {
		if len(side.set) == 0 {
			continue
		}
		// Re-read the host around the sub-ability: it may grow the card arena
		// and leave an earlier *Card stale (playRecord's note).
		saved := g.Card(a.Source).Memory.Remembered()
		g.Card(a.Source).Memory.ClearRemembered()
		for _, p := range side.set {
			g.Card(a.Source).Memory.Remember(PlayerEntity(p))
		}
		err := g.resolveFlipSub(a, controller, side.key, side.name, len(side.set))
		g.Card(a.Source).Memory.ClearRemembered()
		for _, e := range saved {
			g.Card(a.Source).Memory.Remember(e)
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

// flipOneCoin is FlipCoinEffect.flipCoin (FlipCoinEffect.java:203-241): a
// fixed result (FlipCoinMod) asks no call and draws nothing from the RNG;
// otherwise the flipper calls unless NoCall$ and the coin is the RNG's.
// Every flip counts toward flipsThisTurn; FlippedCoin fires for a called or a
// fixed flip.
func (g *Game) flipOneCoin(controller PlayerController, a *Ability, flipper PlayerID, noCall bool, fixed *bool) bool {
	call, result := true, false
	if fixed != nil {
		result = *fixed
	} else {
		if !noCall {
			call = controller.CallCoinFlip(g, flipper, a.Source)
		}
		result = g.flipCoin(flipper)
	}
	won := result == call
	g.Player(flipper).flipsThisTurn++
	if !noCall || fixed != nil {
		g.checkFlippedCoinTriggers(controller, flipper, won)
	}
	return won
}

// flipCoinStatics calls each live static of mode on a card in a
// static-ability source zone (ZoneType.STATIC_ABILITIES_SOURCE_ZONES:
// battlefield, graveyard, exile, Command, stack) until each returns false.
func (g *Game) flipCoinStatics(mode string, each func(h *Card, s *compile.Ability) bool) {
	for _, p := range g.Players() {
		for _, z := range []ZoneType{Battlefield, Graveyard, Exile, Command, Stack} {
			for _, id := range g.Zone(z, p).Cards() {
				c := g.Card(id)
				if c.Def == nil {
					continue
				}
				for _, face := range c.liveTraitFaces() {
					for _, s := range face.Statics {
						if strings.EqualFold(s.Name, mode) && !each(c, s) {
							return
						}
					}
				}
			}
		}
	}
}

// flipCoinFixedResult is StaticAbilityFlipCoinMod.fixedResult
// (StaticAbilityFlipCoinMod.java:14-19): the Result$ of the first FlipCoinMod
// whose conditions hold (Edgar, King of Figaro's CheckSVar$ YouFlipThisTurn
// EQ0) and whose ValidPlayer$ admits flipper; nil for none.
func (g *Game) flipCoinFixedResult(flipper PlayerID) *bool {
	var out *bool
	g.flipCoinStatics("FlipCoinMod", func(h *Card, s *compile.Ability) bool {
		if !g.staticOtherConditionsMet(h, s) {
			return true
		}
		if spec, ok := s.Param("ValidPlayer"); ok {
			if m, known := matchesPlayerSpec(g, flipper, h.Controller(), h.ID, spec); !known || !m {
				return true
			}
		}
		raw, _ := s.Param("Result")
		v := strings.EqualFold(raw, "true")
		out = &v
		return false
	})
	return out
}

// flipCoinDoublerInPlay reports whether a FlipCoinDoubler static (Krark's
// Thumb) is in play: its chooseFlipResult decision is not ported, so a flip
// then fails closed.
func flipCoinDoublerInPlay(g *Game) bool {
	found := false
	g.flipCoinStatics("FlipCoinDoubler", func(*Card, *compile.Ability) bool {
		found = true
		return false
	})
	return found
}
