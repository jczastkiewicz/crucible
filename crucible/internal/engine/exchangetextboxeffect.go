package engine

//enginelint:allow card game ability condition effecthelpers defined textwords control zone

import "fmt"

// exchangeTextBoxEffect is TextBoxExchangeEffect.java (Java's class name; the
// API is ExchangeTextBox): two cards swap their intrinsic text boxes (CR 612,
// Exchange of Words, Deadpool, Trading Card). Each card keeps the other's
// abilities, triggers, statics, replacement effects and keywords as they
// stood at resolution, word changes included, and every earlier word change on
// both cards no longer applies. A resolution with fewer than two cards does
// nothing. No Duration$ is permanent; AsLongAsInPlay and UntilHostLeavesPlay
// end with the host (textBoxRecord), and a host that already left, or phased
// out for the first, applies nothing (checkValidDuration). Any other
// Duration$ is not resolvable yet.
type exchangeTextBoxEffect struct{}

func (exchangeTextBoxEffect) Resolve(g *Game, a *Ability, _ PlayerController) error {
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	var bound, phaseEnds bool
	if d, ok := a.Params.Param("Duration"); ok {
		switch d {
		case "AsLongAsInPlay":
			bound, phaseEnds = true, true
		case "UntilHostLeavesPlay":
			bound = true
		default:
			return fmt.Errorf("engine: ExchangeTextBox: Duration$ %q not resolvable yet", d)
		}
		if source.Zone != Battlefield && source.Zone != Stack || phaseEnds && source.IsPhasedOut() {
			return nil
		}
	}
	cards, err := targetedOrDefinedCards(source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: ExchangeTextBox: %w", err)
	}
	if len(cards) < 2 {
		return nil
	}
	first, second := g.Card(cards[0]), g.Card(cards[1])
	if first.Def == nil || second.Def == nil {
		return fmt.Errorf("engine: ExchangeTextBox: a card without a definition")
	}
	g.timestamp++
	ts := g.timestamp
	for _, pair := range [2][2]*Card{{first, second}, {second, first}} {
		g.textBoxes = append(g.textBoxes, textBoxRecord{
			Card: pair[0].ID, Timestamp: ts, Source: pair[1].Def.Faces[0],
			hostBound: bound, phaseEnds: phaseEnds, host: source.ID, hostObject: source.zoneStamp,
		})
	}
	return nil
}
