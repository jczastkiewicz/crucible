package compile

import "fmt"

// appendCipherEncode is the spell half of `K:Cipher` (CR 702.99),
// CardFactoryUtil.java:2714-2720: a DB$ Encode appended to the end of the
// card's first spell ability's SubAbility$ chain (appendSubAbility), so the
// card is exiled and encoded as the spell's last instruction. A face with no
// spell ability gets nothing, as Java's getFirstSpellAbility finds none. Every
// sub is compiled afresh for the ability naming it (reference), so the tail
// edited here belongs to this spell alone.
func (c *faceCompiler) appendCipherEncode(out *Face, line string) error {
	for _, spell := range out.Abilities {
		if spell.Record != Spell {
			continue
		}
		encode, err := c.line("DB$ Encode", SubAbility)
		if err != nil {
			return fmt.Errorf("keyword %q: %w", line, err)
		}
		encode.Keyword = line
		tail := spell
		for next := chainSub(tail); next != nil; next = chainSub(tail) {
			tail = next
		}
		tail.Subs = append(tail.Subs, SubRef{Key: "SubAbility", SVar: "KWCipherEncode", Ability: encode})
		return nil
	}
	return nil
}

// chainSub is the ability a's SubAbility$ names, nil at the end of the chain.
func chainSub(a *Ability) *Ability {
	for _, sub := range a.Subs {
		if sub.Key == "SubAbility" {
			return sub.Ability
		}
	}
	return nil
}
