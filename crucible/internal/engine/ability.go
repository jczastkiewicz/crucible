// The API vocabulary and the value that names one resolvable ability,
// kept apart from effect.go's dispatch machinery on purpose: dispatch
// (Effect.Resolve, Registry) needs *Game, and Game.stack (stack.go) needs
// to name Ability, so Ability and the APIType it carries have to sit below
// both in the dependency graph or the two would depend on each other
// (enginelint).

package engine

import "fmt"

//go:generate go run ../../tools/genapitype -apitype ../../../forge-game/src/main/java/forge/game/ability/ApiType.java

// APIType names an ability API. The constants are generated from Forge's
// ApiType enum, so the set cannot drift from upstream without a build failure.
type APIType uint16

// String returns the API as a card script spells it.
func (a APIType) String() string {
	if int(a) >= numAPITypes {
		return fmt.Sprintf("APIType(%d)", uint16(a))
	}
	return apiNames[a]
}

// APIByName looks an API up by the name a script writes, and reports whether
// it is one. Exact match: an API that silently resolves to something else is
// a card doing the wrong thing rather than nothing.
func APIByName(name string) (APIType, bool) {
	for i, n := range apiNames {
		if n == name {
			return APIType(i), true
		}
	}
	return 0, false
}

// Ability is one resolvable ability on the stack. The cost already paid and
// everything else CR 601-609 tracks per stack object beyond API/Source/
// Controller/Target land here once casting grows enough to fill them; today
// it carries just enough for dispatch, for the stack to know whose it is,
// and for the one target CastSpell's own Aura branch chooses at cast time
// (CR 601.2c).
type Ability struct {
	// API decides which Effect resolves this.
	API APIType
	// Source is the card the ability came from.
	Source CardID
	// Controller is who is resolving it, which is not always the source's
	// controller once control-changing effects are involved.
	Controller PlayerID
	// Target is the single card this ability was announced against at cast
	// time (CR 601.2c) -- NoCard when the ability has none. An Aura's own
	// APIAttach entry is the only thing that sets it today; this port has no
	// representation for a target that is a player (enchantSpec's own doc
	// comment) or for more than one target (no ability needing that shape
	// exists yet), so a single CardID is enough rather than a slice or an
	// EntityID.
	Target CardID
}
