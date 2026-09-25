package engine

//enginelint:allow ability animate card condition control effecthelpers game id protectioneffect zone

import "fmt"

// protectionAllEffect is ProtectAllEffect.java: every battlefield card
// matching ValidCards$ gains "Protection from" each Gains$ entry until end
// of turn (Duration$ Permanent keeps it). ValidPlayers$ (player keywords)
// and Gains$ TargetedCardColor fail closed.
type protectionAllEffect struct{}

func (protectionAllEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectParams(a, "ProtectionAll", "ValidPlayers", "Condition", "ConditionDefined"); err != nil {
		return err
	}
	permanent, err := animateDuration(a, "ProtectionAll")
	if err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	gains, err := protectionGains(g, a, controller, "ProtectionAll")
	if err != nil || len(gains) == 0 {
		return err
	}
	spec, ok := a.Params.Param("ValidCards")
	if !ok {
		return fmt.Errorf("engine: ProtectionAll: no ValidCards$")
	}
	var cards []CardID
	for _, p := range g.Players() {
		cards = append(cards, g.Zone(Battlefield, p).Cards()...)
	}
	cards = filterValid(g, cards, spec, a.Controller, a.Source)
	g.animateCards(animateRecord{Permanent: permanent, AddKeywords: protectionKeywords(gains, false)}, cards)
	return nil
}
