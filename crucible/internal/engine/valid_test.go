package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// A bare color name matches a card carrying that color, derived from its
// mana cost (Card.Colors' own doc comment) -- and not one it lacks. All
// five, not just one, since colorMatches names each independently.
func TestMatchesColor(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]

	for _, tc := range []struct{ cost, color, other string }{
		{"1 W", "White", "Blue"},
		{"1 U", "Blue", "Black"},
		{"1 B", "Black", "Red"},
		{"1 R", "Red", "Green"},
		{"1 G", "Green", "White"},
	} {
		id := g.NewCard(creatureDefManaCost(t, tc.cost), p, engine.Battlefield)
		if !engine.Matches(g.Card(id), valid.Parse("Creature."+tc.color), p, engine.NoCard) {
			t.Errorf("a %s creature did not match Creature.%s", tc.color, tc.color)
		}
		if engine.Matches(g.Card(id), valid.Parse("Creature."+tc.other), p, engine.NoCard) {
			t.Errorf("a %s creature matched Creature.%s", tc.color, tc.other)
		}
	}
}

// The "non" form of a color negates it -- "nonBlack" matches anything that
// is not black, including a card of a different color entirely.
func TestMatchesNonColor(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	red := g.NewCard(creatureDefManaCost(t, "1 R"), p, engine.Battlefield)
	black := g.NewCard(creatureDefManaCost(t, "1 B"), p, engine.Battlefield)

	if !engine.Matches(g.Card(red), valid.Parse("Creature.nonBlack"), p, engine.NoCard) {
		t.Error("a red creature did not match Creature.nonBlack")
	}
	if engine.Matches(g.Card(black), valid.Parse("Creature.nonBlack"), p, engine.NoCard) {
		t.Error("a black creature matched Creature.nonBlack")
	}
}

// "WhiteSource" is not implemented (colorMatches' own doc comment -- it
// needs a damage-source context Matches does not carry) and must not be
// silently misread as bare "White": a white card should not match it, and
// neither should a card of any other color.
func TestMatchesColorSourceSuffixIsNotImplemented(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	white := g.NewCard(creatureDefManaCost(t, "1 W"), p, engine.Battlefield)

	if engine.Matches(g.Card(white), valid.Parse("Creature.WhiteSource"), p, engine.NoCard) {
		t.Error("a white creature matched the unimplemented WhiteSource property")
	}
}

// Colorless and nonColorless are each other's opposite, and neither is a
// color name -- a colored card is not Colorless, and a colorless one is not
// nonColorless.
func TestMatchesColorlessAndNonColorless(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	colorless := g.NewCard(creatureDefManaCost(t, "3"), p, engine.Battlefield)
	red := g.NewCard(creatureDefManaCost(t, "1 R"), p, engine.Battlefield)

	if !engine.Matches(g.Card(colorless), valid.Parse("Creature.Colorless"), p, engine.NoCard) {
		t.Error("a colorless creature did not match Creature.Colorless")
	}
	if engine.Matches(g.Card(red), valid.Parse("Creature.Colorless"), p, engine.NoCard) {
		t.Error("a red creature matched Creature.Colorless")
	}
	if !engine.Matches(g.Card(red), valid.Parse("Creature.nonColorless"), p, engine.NoCard) {
		t.Error("a red creature did not match Creature.nonColorless")
	}
	if engine.Matches(g.Card(colorless), valid.Parse("Creature.nonColorless"), p, engine.NoCard) {
		t.Error("a colorless creature matched Creature.nonColorless")
	}
}

// MultiColor matches two or more colors, not one and not zero.
func TestMatchesMultiColor(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	mono := g.NewCard(creatureDefManaCost(t, "1 R"), p, engine.Battlefield)
	multi := g.NewCard(creatureDefManaCost(t, "R G"), p, engine.Battlefield)

	if engine.Matches(g.Card(mono), valid.Parse("Creature.MultiColor"), p, engine.NoCard) {
		t.Error("a monocolored creature matched Creature.MultiColor")
	}
	if !engine.Matches(g.Card(multi), valid.Parse("Creature.MultiColor"), p, engine.NoCard) {
		t.Error("a two-color creature did not match Creature.MultiColor")
	}
}

// A generic "non<Type>" property, not one of the five named colors, falls
// to the type-negation fallback -- "nonLand" is "!HasStringType(Land)".
func TestMatchesNonTypeFallback(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	creature := g.NewCard(creatureDef(t), p, engine.Battlefield)

	if !engine.Matches(g.Card(creature), valid.Parse("Card.nonLand"), p, engine.NoCard) {
		t.Error("a creature did not match Card.nonLand")
	}
	if engine.Matches(g.Card(creature), valid.Parse("Card.nonCreature"), p, engine.NoCard) {
		t.Error("a creature matched Card.nonCreature")
	}
}

// A bare type word as a Base matches by type, ignoring properties entirely
// when there are none.
func TestMatchesBaseCoreType(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	aura := g.NewCard(auraDef(t), p, engine.Battlefield)
	equipment := g.NewCard(equipmentDef(t), p, engine.Battlefield)

	if !engine.Matches(g.Card(aura), valid.Parse("Enchantment"), p, engine.NoCard) {
		t.Error("an Aura did not match its own core type")
	}
	if engine.Matches(g.Card(aura), valid.Parse("Creature"), p, engine.NoCard) {
		t.Error("an Aura matched Creature")
	}
	if !engine.Matches(g.Card(equipment), valid.Parse("Artifact"), p, engine.NoCard) {
		t.Error("an Equipment did not match its own core type")
	}
}

// A subtype word, and the vocabulary fallthrough case-sensitivity
// HasStringType already covers, both reach the same code path as a core
// type.
func TestMatchesBaseSubtype(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	aura := g.NewCard(auraDef(t), p, engine.Battlefield)

	if !engine.Matches(g.Card(aura), valid.Parse("Aura"), p, engine.NoCard) {
		t.Error("an Aura did not match its own subtype")
	}
}

// "Permanent" and "Card" are Java's own special Base cases, not a type
// lookup.
func TestMatchesSpecialBases(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	aura := g.NewCard(auraDef(t), p, engine.Battlefield) // Enchantment: a permanent

	if !engine.Matches(g.Card(aura), valid.Parse("Permanent"), p, engine.NoCard) {
		t.Error("an Aura (a permanent) did not match Permanent")
	}
	if !engine.Matches(g.Card(aura), valid.Parse("Card"), p, engine.NoCard) {
		t.Error("an ordinary card did not match Card")
	}
}

// "Any" is Java's own shorthand for "a creature, a planeswalker or a
// battle" -- not a type lookup, and not literally every card.
func TestMatchesBaseAny(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	creature := g.NewCard(creatureDef(t), p, engine.Battlefield)
	aura := g.NewCard(auraDef(t), p, engine.Battlefield)

	if !engine.Matches(g.Card(creature), valid.Parse("Any"), p, engine.NoCard) {
		t.Error("a creature did not match Any")
	}
	if engine.Matches(g.Card(aura), valid.Parse("Any"), p, engine.NoCard) {
		t.Error("an Aura (not a creature, planeswalker or battle) matched Any")
	}
}

// A bare type word used as a property, not a base -- "Permanent.Creature"
// -- reaches the same HasStringType fallthrough a Base does.
func TestMatchesPropertyTypeWordFallthrough(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	creature := g.NewCard(creatureDef(t), p, engine.Battlefield)

	if !engine.Matches(g.Card(creature), valid.Parse("Permanent.Creature"), p, engine.NoCard) {
		t.Error("a creature did not match Permanent.Creature")
	}
	if engine.Matches(g.Card(creature), valid.Parse("Permanent.Land"), p, engine.NoCard) {
		t.Error("a creature matched Permanent.Land")
	}
}

// Spell, Effect, Emblem and Boon are coverage gaps, not matches: nothing
// this port creates is ever one of them yet.
func TestMatchesUnbuiltBasesNeverMatch(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	aura := g.NewCard(auraDef(t), p, engine.Battlefield)

	for _, base := range []string{"Spell", "Effect", "Emblem", "Boon"} {
		if engine.Matches(g.Card(aura), valid.Parse(base), p, engine.NoCard) {
			t.Errorf("an ordinary permanent matched Base %q", base)
		}
	}
}

// YouCtrl and OppCtrl compare the card's controller against sourceController,
// the perspective Matches is called from -- not the card's owner, and not
// some fixed player.
func TestMatchesYouCtrlOppCtrl(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	id := g.NewCard(nil, a, engine.Battlefield) // controlled by a

	if !engine.Matches(g.Card(id), valid.Parse("Card.YouCtrl"), a, engine.NoCard) {
		t.Error("a's own card did not match YouCtrl from a's perspective")
	}
	if engine.Matches(g.Card(id), valid.Parse("Card.YouCtrl"), b, engine.NoCard) {
		t.Error("a's card matched YouCtrl from b's perspective")
	}
	if !engine.Matches(g.Card(id), valid.Parse("Card.OppCtrl"), b, engine.NoCard) {
		t.Error("a's card did not match OppCtrl from b's perspective")
	}
	if engine.Matches(g.Card(id), valid.Parse("Card.OppCtrl"), a, engine.NoCard) {
		t.Error("a's own card matched OppCtrl from a's own perspective")
	}
}

// Self compares the card's own handle against source, not its controller.
func TestMatchesSelf(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	source := g.NewCard(nil, p, engine.Battlefield)
	other := g.NewCard(nil, p, engine.Battlefield)

	if !engine.Matches(g.Card(source), valid.Parse("Card.Self"), p, source) {
		t.Error("the source card did not match Self")
	}
	if engine.Matches(g.Card(other), valid.Parse("Card.Self"), p, source) {
		t.Error("a different card matched Self")
	}
}

// Properties within one alternative are AND: all of them have to match.
func TestMatchesPropertiesAreConjunctive(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	aura := g.NewCard(auraDef(t), a, engine.Battlefield)

	if !engine.Matches(g.Card(aura), valid.Parse("Enchantment.YouCtrl"), a, engine.NoCard) {
		t.Error("an Aura controlled by a did not match Enchantment.YouCtrl from a's perspective")
	}
	if engine.Matches(g.Card(aura), valid.Parse("Enchantment.YouCtrl"), b, engine.NoCard) {
		t.Error("an Aura controlled by a matched Enchantment.YouCtrl from b's perspective")
	}
	if engine.Matches(g.Card(aura), valid.Parse("Creature.YouCtrl"), a, engine.NoCard) {
		t.Error("an Aura matched Creature.YouCtrl even though the base does not match")
	}
}

// Alternatives are OR: matching either is enough.
func TestMatchesAlternativesAreDisjunctive(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	aura := g.NewCard(auraDef(t), p, engine.Battlefield)

	if !engine.Matches(g.Card(aura), valid.Parse("Creature,Enchantment"), p, engine.NoCard) {
		t.Error("an Aura did not match the second alternative of Creature,Enchantment")
	}
	if engine.Matches(g.Card(aura), valid.Parse("Creature,Land"), p, engine.NoCard) {
		t.Error("an Aura matched neither alternative but Matches still reported true")
	}
}

// A `!` on a property negates only that property -- the simple, De Morgan
// case (Card.hasProperty's own wrapper).
func TestMatchesPropertyNegation(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	id := g.NewCard(nil, a, engine.Battlefield)

	if !engine.Matches(g.Card(id), valid.Parse("Card.!OppCtrl"), a, engine.NoCard) {
		t.Error("a's own card did not match Card.!OppCtrl from a's perspective")
	}
	if engine.Matches(g.Card(id), valid.Parse("Card.!OppCtrl"), b, engine.NoCard) {
		t.Error("a's card matched Card.!OppCtrl from b's perspective, where OppCtrl itself holds")
	}
}

// A `!` on the base negates the whole alternative -- base AND every
// property -- not just the base by itself (Card.isValid's testFailed
// short-circuit). "!Creature.YouCtrl" matches everything that is not a
// creature you control, including a creature an opponent controls, not
// only "a non-creature you control".
func TestMatchesBaseNegationCoversTheWholeAlternative(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	yours := g.NewCard(auraDef(t), a, engine.Battlefield)
	theirs := g.NewCard(auraDef(t), b, engine.Battlefield)

	spec := valid.Parse("!Permanent.YouCtrl")

	if engine.Matches(g.Card(yours), spec, a, engine.NoCard) {
		t.Error("a permanent a controls matched !Permanent.YouCtrl from a's own perspective")
	}
	// theirs is a permanent OppCtrl-relative to a, so "Permanent.YouCtrl" is
	// false for it from a's perspective -- and the negated form must then be
	// true, not "false because theirs is still a Permanent".
	if !engine.Matches(g.Card(theirs), spec, a, engine.NoCard) {
		t.Error("a permanent a does not control did not match !Permanent.YouCtrl from a's perspective")
	}
}
