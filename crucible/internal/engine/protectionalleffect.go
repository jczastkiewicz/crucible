package engine

//enginelint:allow ability animate card condition control defined effecthelpers game id player protectioneffect zone

import "fmt"

// protectionAllEffect is ProtectAllEffect.java: every battlefield card
// matching ValidCards$ gains "Protection from" each Gains$ entry until end
// of turn (Duration$ Permanent keeps it). ValidPlayers$ grants the same
// lines to each Defined player (ProtectAllEffect.java: player.addChangedKeywords).
// Gains$ TargetedCardColor fails closed.
type protectionAllEffect struct{}

func (protectionAllEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	permanent, until, err := animateDuration(a, "ProtectionAll")
	if err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	gains, err := protectionGains(g, a, controller, a.Controller, "ProtectionAll")
	if err != nil || len(gains) == 0 {
		return err
	}
	players, hasPlayers := a.Params.Param("ValidPlayers")
	spec, hasCards := a.Params.Param("ValidCards")
	if !hasCards && !hasPlayers {
		return fmt.Errorf("engine: ProtectionAll: no ValidCards$")
	}
	if hasPlayers {
		ps, err := definedPlayers(g, a.Controller, a.Source, players, a.refs())
		if err != nil {
			return fmt.Errorf("engine: ProtectionAll: %w", err)
		}
		g.timestamp++
		for _, p := range ps {
			g.pumps = append(g.pumps, pumpRecord{
				Timestamp: g.timestamp, Keywords: protectionKeywords(gains, false), OnPlayer: true, Player: p,
				Permanent: permanent,
			})
		}
	}
	if !hasCards {
		return nil
	}
	var cards []CardID
	for _, p := range g.Players() {
		cards = append(cards, g.Zone(Battlefield, p).Cards()...)
	}
	cards = filterValid(g, cards, spec, a.Controller, a.Source)
	if err := g.animateCards(a, animateRecord{Permanent: permanent, until: until, AddKeywords: protectionKeywords(gains, false)}, cards); err != nil {
		return err
	}
	return nil
}
