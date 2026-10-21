package compile

import (
	"errors"
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
)

// ErrBadGift is a `K:Gift` keyword on a card without the GiftAbility SVar the
// promise runs.
var ErrBadGift = errors.New("bad Gift keyword")

// expandGift is the `K:Gift` keyword (CR 702.174), CardFactoryUtil.java:1131-
// 1151. The GiftAbility SVar becomes the card's first spell ability's
// additional ability (setAdditionalAbility, :1135), which AbilityUtils.
// resolvePreAbilities runs before an instant or sorcery's own effect when the
// gift was promised (:1349-1357). A permanent has no spell ability to hold it:
// its GiftAbility is the overriding ability of a synthesized "when this enters,
// if the gift was promised" trigger instead (:1144-1151). The trigger is added
// to every card with the keyword, an instant's or sorcery's never being able
// to fire, as Java returns before building it (:1137-1140).
func (c *faceCompiler) expandGift(out *Face, line string) error {
	sub, err := c.reference("GiftAbility", "GiftAbility")
	if err != nil {
		return fmt.Errorf("%w: %q: %w", ErrBadGift, line, err)
	}
	if c.face.Type.Has(cardtype.Instant) || c.face.Type.Has(cardtype.Sorcery) {
		for _, spell := range out.Abilities {
			if spell.Record == Spell {
				sub.Ability.Keyword = line
				spell.Subs = append(spell.Subs, sub)
				return nil
			}
		}
		return fmt.Errorf("%w: %q: no spell ability to hold GiftAbility", ErrBadGift, line)
	}
	t, err := c.line("Mode$ ChangesZone | Destination$ Battlefield | ValidCard$ Card.Self+PromisedGift | Secondary$ True | Execute$ GiftAbility", Trigger)
	if err != nil {
		return fmt.Errorf("keyword %q: %w", line, err)
	}
	t.Keyword = line
	for _, ref := range t.Subs {
		ref.Ability.Keyword = line
	}
	out.Triggers = append(out.Triggers, t)
	return nil
}
