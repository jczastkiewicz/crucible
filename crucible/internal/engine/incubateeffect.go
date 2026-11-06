package engine

//enginelint:allow ability card condition control defined effecthelpers game id parts token zone tokentable

// incubateEffect is IncubateEffect.java: Times$ (default 1) times per
// target or Defined$ player (default You), an Incubator token
// (incubator_c_0_0_a_phyrexian) enters with Amount$ (default 1) +1/+1
// counters -- the WithCountersType$ P1P1 the Java effect injects.
type incubateEffect struct{}

func (incubateEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	if err := rejectParams(a, "Incubate"); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	times, err := optionalAmount(g, a, "Incubate", "Times", 1)
	if err != nil {
		return err
	}
	amount, err := optionalAmount(g, a, "Incubate", "Amount", 1)
	if err != nil {
		return err
	}
	def, err := tokenScript(g, "incubator_c_0_0_a_phyrexian")
	if err != nil {
		return err
	}
	players, err := targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.refs())
	if err != nil {
		return err
	}
	for _, p := range g.inAPNAPOrder(players) {
		for i := 0; i < times; i++ {
			ids := g.makeTokens(controller, tokenCell{creator: p, spec: tokenSpec{Def: def, Owner: p, P1P1: amount}, n: 1})
			g.checkChangesZoneAllTriggers(controller, ids, None, Battlefield)
		}
	}
	return nil
}
