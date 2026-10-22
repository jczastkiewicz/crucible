package compile

import (
	"errors"
	"fmt"
	"strings"
)

// ErrBadClass is a `K:Class:<level>:<cost>:<params>` keyword that is missing
// one of its three parts.
var ErrBadClass = errors.New("bad Class keyword")

// expandClass is the `K:Class` keyword (CR 716), the two places CardFactoryUtil
// builds it: addSpellAbility (CardFactoryUtil.java:2721-2733) and
// addStaticAbility (:3822-3865). Each line stands for one level N:
//
//   - the level-up ability, `{cost}: Level N`, sorcery speed and only while the
//     Class is at level N-1 (ClassLevel$ EQ<N-1>); and
//   - a static that grants the level's abilities (AddTrigger$,
//     AddStaticAbility$, AddReplacementEffect$ from the line's own params)
//     while the Class is at level N or above (ClassLevel$ N, read as "at
//     least" by StaticAbility.checkConditions, :454-460).
//
// Java also builds a Description$ from the granted SVars' own descriptions; it
// is display only and not built here.
func (c *faceCompiler) expandClass(out *Face, line string) error {
	parts := strings.SplitN(strings.TrimPrefix(line, "Class:"), ":", 3)
	if len(parts) != 3 {
		return fmt.Errorf("%w: %q needs a level, a cost and params", ErrBadClass, line)
	}
	level, cost, params := parts[0], parts[1], parts[2]
	var n int
	if _, err := fmt.Sscanf(level, "%d", &n); err != nil || n < 2 {
		return fmt.Errorf("%w: %q has level %q", ErrBadClass, line, level)
	}
	up, err := c.line(fmt.Sprintf("AB$ ClassLevelUp | Cost$ %s | ClassLevel$ EQ%d | SorcerySpeed$ True | StackDescription$ SpellDescription | SpellDescription$ Level %d", cost, n-1, n), Spell)
	if err != nil {
		return fmt.Errorf("keyword %q: %w", line, err)
	}
	up.Keyword = line
	out.Abilities = append(out.Abilities, up)

	st, err := c.line("Mode$ Continuous | Affected$ Card.Self | ClassLevel$ "+level+" | "+params, StaticEffect)
	if err != nil {
		return fmt.Errorf("keyword %q: %w", line, err)
	}
	st.Keyword = line
	out.Statics = append(out.Statics, st)
	return nil
}
