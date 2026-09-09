// Ported from forge-game/src/main/java/forge/game/cost/Cost.java
// (parseCostPart).

package cost

// The named cost parts, in the order parseCostPart tests them.
//
// The field limit is Java's own, passed to abCostParse per branch, and it is
// not derivable from the part: `tapXType` splits its body into three, so
// `tapXType<2/Creature;Treasure/creatures and/or Treasures>` keeps the slash
// in its description instead of making a fourth field.
//
// Order is preserved even though no prefix currently shadows another -- the
// trailing `<` is what keeps `Exile<` from claiming `ExileFromHand<` -- because
// the order is Java's contract and a future part without a `<` would depend on
// it. `PromiseGift` is already such a part.
var namedParts = []struct {
	name   string
	prefix string
	fields int
	// exact means Java tests equals rather than startsWith, so `T` is the tap
	// cost and `Teamwork<...>` is not.
	exact bool
}{
	{name: "Mana", prefix: "Mana<", fields: 1},
	{name: "tapXType", prefix: "tapXType<", fields: 3},
	{name: "untapYType", prefix: "untapYType<", fields: 3},
	{name: "SubCounter", prefix: "SubCounter<", fields: 5},
	{name: "AddCounter", prefix: "AddCounter<", fields: 4},
	{name: "AddCounterYou", prefix: "AddCounterYou<", fields: 2},
	{name: "PayLife", prefix: "PayLife<", fields: 2},
	{name: "PayEnergy", prefix: "PayEnergy<", fields: 1},
	{name: "PayShards", prefix: "PayShards<", fields: 1},
	{name: "GainLife", prefix: "GainLife<", fields: 3},
	{name: "GainControl", prefix: "GainControl<", fields: 3},
	{name: "Unattach", prefix: "Unattach<", fields: 2},
	{name: "ChooseColor", prefix: "ChooseColor<", fields: 1},
	{name: "ChooseCreatureType", prefix: "ChooseCreatureType<", fields: 1},
	{name: "DamageYou", prefix: "DamageYou<", fields: 1},
	{name: "Mill", prefix: "Mill<", fields: 1},
	{name: "FlipCoin", prefix: "FlipCoin<", fields: 1},
	{name: "RollDice", prefix: "RollDice<", fields: 4},
	{name: "Discard", prefix: "Discard<", fields: 3},
	{name: "AddMana", prefix: "AddMana<", fields: 3},
	{name: "Sac", prefix: "Sac<", fields: 3},
	{name: "RemoveAnyCounter", prefix: "RemoveAnyCounter<", fields: 4},
	{name: "Exile", prefix: "Exile<", fields: 3},
	{name: "ExileFromHand", prefix: "ExileFromHand<", fields: 3},
	{name: "ExileFromGrave", prefix: "ExileFromGrave<", fields: 3},
	{name: "ExileFromStack", prefix: "ExileFromStack<", fields: 3},
	{name: "ExileFromTop", prefix: "ExileFromTop<", fields: 3},
	{name: "ExileAnyGrave", prefix: "ExileAnyGrave<", fields: 3},
	{name: "ExileSameGrave", prefix: "ExileSameGrave<", fields: 3},
	{name: "ExileCtrlOrGrave", prefix: "ExileCtrlOrGrave<", fields: 3},
	{name: "PromiseGift", prefix: "PromiseGift", fields: 0},
	{name: "Return", prefix: "Return<", fields: 3},
	{name: "ChooseCard", prefix: "ChooseCard<", fields: 3},
	{name: "Reveal", prefix: "Reveal<", fields: 3},
	{name: "RevealFromExile", prefix: "RevealFromExile<", fields: 3},
	{name: "RevealOrChoose", prefix: "RevealOrChoose<", fields: 3},
	{name: "Behold", prefix: "Behold<", fields: 3},
	{name: "BeholdExile", prefix: "BeholdExile<", fields: 3},
	{name: "ExiledMoveToGrave", prefix: "ExiledMoveToGrave<", fields: 3},
	{name: "Draw", prefix: "Draw<", fields: 2},
	{name: "PutCardToLibFromHand", prefix: "PutCardToLibFromHand<", fields: 4},
	{name: "PutCardToLibFromGrave", prefix: "PutCardToLibFromGrave<", fields: 4},
	{name: "PutCardToLibFromSameGrave", prefix: "PutCardToLibFromSameGrave<", fields: 4},
	{name: "PutCardToLibFromBattlefield", prefix: "PutCardToLibFromBattlefield<", fields: 4},
	{name: "Exert", prefix: "Exert<", fields: 3},
	{name: "Enlist", prefix: "Enlist<", fields: 3},
	{name: "CollectEvidence", prefix: "CollectEvidence<", fields: 1},
	{name: "RevealChosen", prefix: "RevealChosen<", fields: 2},
	{name: "Waterbend", prefix: "Waterbend<", fields: 1},
	{name: "Blight", prefix: "Blight<", fields: 1},
	{name: "Teamwork", prefix: "Teamwork<", fields: 1},
	{name: "Forage", prefix: "Forage", fields: 0, exact: true},
	{name: "Untap", prefix: "Untap", fields: 0, exact: true},
	{name: "Q", prefix: "Q", fields: 0, exact: true},
	{name: "T", prefix: "T", fields: 0, exact: true},
}
