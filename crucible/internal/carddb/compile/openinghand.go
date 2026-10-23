package compile

import (
	"errors"
	"fmt"
	"strings"
)

// ErrBadOpeningHand is a `K:MayEffectFromOpeningHand` keyword without the SVar
// ability it names.
var ErrBadOpeningHand = errors.New("bad MayEffectFromOpeningHand keyword")

// OpeningHand is one `K:MayEffectFromOpeningHand:<SVar>[:!PlayFirst]` keyword:
// the ability a player may run from their opening hand
// (GameAction.runOpeningHandActions, GameAction.java:2507-2551).
type OpeningHand struct {
	// Keyword is the keyword line.
	Keyword string
	// NotFirst is the `!PlayFirst` marker: the card is not offered to the
	// player who goes first (GameAction.java:2518-2522).
	NotFirst bool
	// Ability is the SVar the keyword names, run without the stack.
	Ability *Ability
}

// expandOpeningHand compiles the SVar a MayEffectFromOpeningHand keyword names
// (CardFactoryUtil never builds an ability for it: GameAction reads the SVar
// off the card when the game starts, GameAction.java:2524-2527).
func (c *faceCompiler) expandOpeningHand(out *Face, line string) error {
	parts := strings.Split(line, ":")
	if len(parts) < 2 || parts[1] == "" {
		return fmt.Errorf("%w: %q", ErrBadOpeningHand, line)
	}
	sub, err := c.reference("MayEffectFromOpeningHand", parts[1])
	if err != nil {
		return fmt.Errorf("%w: %q: %w", ErrBadOpeningHand, line, err)
	}
	out.OpeningHand = append(out.OpeningHand, OpeningHand{
		Keyword:  line,
		NotFirst: len(parts) > 2 && parts[2] == "!PlayFirst",
		Ability:  sub.Ability,
	})
	return nil
}
