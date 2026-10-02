// Player-state predicates shared across mechanisms that gate on them: a
// continuous effect's own Condition$ (continuous.go), a trigger's own
// meetsCommonRequirements-style boolean flags (trigger.go). Neither owns
// these outright, so they live here rather than in either -- the same
// "shared, so neither" reason amount.go's own resolveAmount does.

package engine

import "github.com/jczastkiewicz/crucible/internal/cardtype"

// opponentWasDamaged is Player.hasBloodthirst: an opponent of pid has been dealt
// damage this turn.
func (g *Game) opponentWasDamaged(pid PlayerID) bool {
	return g.bloodthirstAmount(pid) > 0
}

// bloodthirstAmount is Player.getBloodthirstAmount: the damage pid's opponents
// have been dealt this turn, together.
func (g *Game) bloodthirstAmount(pid PlayerID) int {
	n := 0
	for _, other := range g.Players() {
		if other != pid && !g.Player(other).Lost {
			n += g.Player(other).DamageReceivedThisTurn
		}
	}
	return n
}

// battlefieldArtifactCount is Metalcraft's own CardLists.count(..., ARTIFACTS)
// -- every battlefield permanent controller controls whose current (Layer
// 4-folded) type line carries Artifact.
func battlefieldArtifactCount(g *Game, controller PlayerID) int {
	n := 0
	for _, id := range g.Zone(Battlefield, controller).Cards() {
		if g.Card(id).Type().Has(cardtype.Artifact) {
			n++
		}
	}
	return n
}

// graveyardCoreTypeCount is Delirium's own countCardTypesFromList(graveyard,
// false) -- the count of distinct core types (not supertypes, not subtypes)
// across every card in controller's own graveyard, each card's current type
// line unioned into one running Line rather than counted per card.
func graveyardCoreTypeCount(g *Game, controller PlayerID) int {
	var seen cardtype.Line
	for _, id := range g.Zone(Graveyard, controller).Cards() {
		seen = seen.Union(g.Card(id).Type())
	}
	return len(seen.CoreTypes())
}
