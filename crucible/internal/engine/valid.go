// Valid-string matching: deciding whether a card satisfies a parsed
// internal/valid.Spec. internal/valid only parses the grammar (it must not
// import the engine, ADR-0003); this is the evaluation half its own doc
// comment says lands here.
//
// Ported from forge-game/src/main/java/forge/game/card/Card.java's
// isValid/hasProperty and the slice of CardProperty.java's 2,135-line
// cardHasProperty (plus, for color, CardStateProperty.java's own chain --
// colorMatches's own doc comment has the reason color lives there instead)
// this covers. CardProperty, CardStateProperty, PlayerProperty and
// SpellAbilityProperty together answer 928 residual property names
// (docs/crucible/porting/port-log/valid-strings.md); this is:
// ChosenCard/ChosenCardStrict/nonChosenCard, IsRemembered and IsImprinted --
// membership in source's own Memory lists (sourceCard's own doc comment) --
// controller/owner relative to sourceController (YouCtrl, YouDontCtrl,
// OppCtrl, YouOwn, YouDontOwn, OppOwn), identity relative to source (Self,
// Other, StrictlyOther), the five colors plus Colorless and MultiColor, a
// generic keyword check under three spellings (with/without/hasKeyword),
// tapped/untapped, the numeric comparisons (power, toughness, cmc and the
// rest of compareFields, crossed with LT/LE/EQ/GE/GT/NE/M2 -- compareMatches'
// own doc comment) for a plain-integer operand, the generic `non<Type>`
// fallback every chain shares, and the bare type/supertype/subtype
// fallthrough every chain ends on. The rest is M5-M6, corpus-frequency order
// (tools/vocabscan -kind validProperty), the same shape effect.go's Registry
// was always going to grow in (ADR-0011).
package engine

import (
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// Matches decides whether c satisfies spec, from sourceController's
// perspective, with source as the card the spec is written on -- a card's
// own `Enchant`/`ValidCard`, an ability's `ValidTgts`. Both PlayerID/CardID
// parameters are exactly `Ability.Controller`/`Ability.Source` where the spec
// comes from a resolving ability, but Matches does not require one:
// `cleanupDanglingAttachments` (action.go)'s eventual `Enchant`-restriction
// check would call this with the Aura's own controller and the Aura itself,
// no `Ability` in sight. g resolves source to its own *Card when a property
// needs to read something off it (a Remembered/Imprinted/Chosen list --
// sourceCard's own doc comment); nothing else here needs the game.
//
// An alternative matches when its base and every property do (`Spec`'s own
// doc comment: alternatives are OR, properties within one are AND). A `!`
// on the base negates that whole AND, not just the base -- Java's
// `testFailed` short-circuit, reproduced exactly in altMatches, because
// getting this backwards silently inverts every negated valid string in the
// corpus. A `!` on a property negates only that property, which is the
// simple case (`Card.hasProperty`'s own wrapper).
func Matches(g *Game, c *Card, spec valid.Spec, sourceController PlayerID, source CardID) bool {
	for _, alt := range spec.Alternatives {
		if altMatches(g, c, alt, sourceController, source) {
			return true
		}
	}
	return false
}

func altMatches(g *Game, c *Card, alt valid.Alternative, sourceController PlayerID, source CardID) bool {
	if !baseMatches(c, alt.Base.Name) {
		return alt.Base.Negated
	}
	for _, p := range alt.Properties {
		ok := propertyMatches(g, c, p, sourceController, source)
		if p.Negated {
			ok = !ok
		}
		if !ok {
			return alt.Base.Negated
		}
	}
	return !alt.Base.Negated
}

// baseMatches is Card.isValid's own switch on the token before the first
// `.`. The named cases are Java's special ones; everything else falls
// through to a type/supertype/subtype check, the same fallthrough
// `getType().hasStringType(incR[0])` is in Java.
//
// Spell, Effect, Emblem and Boon never match: this port has nothing on the
// stack, no continuous-effect objects, no emblems and no boons yet, so a
// restriction naming one is a coverage gap, not a wrong answer -- the same
// as any SBA this port has not reached.
func baseMatches(c *Card, name string) bool {
	switch name {
	case "Permanent":
		return c.Type().IsPermanent()
	case "card", "Card":
		// Java excludes isImmutable() objects (Effect, Emblem, Boon) here.
		// Nothing this port creates is ever one, so every real card matches.
		return true
	case "Any":
		return c.Type().Has(cardtype.Creature) || c.Type().Has(cardtype.Planeswalker) || c.Type().Has(cardtype.Battle)
	case "Spell", "Effect", "Emblem", "Boon":
		return false
	default:
		return c.Type().HasStringType(name)
	}
}

// propertyMatches is the slice of CardProperty.cardHasProperty (and, for
// color, CardStateProperty.hasProperty -- colorMatches's own doc comment)
// this port answers. p.Compare is checked first and, when set, dispatches
// straight to compareMatches: internal/valid already parsed a numeric
// comparison out of p.Name at load time, so nothing below ever needs to
// re-derive one from the name string. `ChosenCard`/`IsRemembered`/
// `IsImprinted` come next, the one family here that reads source's own
// Card rather than c's or sourceController's -- sourceCard's own doc
// comment has the NoCard case. Ownership/control and identity
// branches are ported by name with `strings.HasPrefix` rather than `==`,
// because Java's own chain
// tests with `startsWith` (a property can carry a suffix argument on other
// branches this port does not reach, and reproducing the match style is
// what keeps a future addition from silently behaving differently on the
// bare token). `tapped`/`untapped` are exact-matched instead, the same
// choice colorMatches' own tokens make, since Java's `startsWith` there has
// no observed suffixed form in the corpus to preserve. Keyword, color and
// the generic `non<Type>` fallback come next, and a final fallthrough
// covers a bare type/supertype/subtype word used as a property, the same
// fallthrough CardState.hasProperty eventually reaches for one
// (`internal/cardtype.CoreTypeNames`'s own doc comment).
//
// Java's `YouCtrl`/`OppCtrl`/`YouOwn`/`OppOwn` compare against the
// controller/owner `game.getChangeZoneLKIInfo` resolves, not
// `card.getController()`/`card.getOwner()` directly -- last-known
// information for a card whose own zone change is mid-resolution. This
// port has no LKI tracking (game-state.md's "Not ported yet"), so these
// read `c.Controller`/`c.Owner` as of now, which agrees with Java's LKI
// everywhere except the one moment a card's own leaving is what a property
// is trying to describe. `Self`/`Other`/`StrictlyOther` carry the same
// simplification one step further: Java's "Strictly" forms are
// game-timestamp-aware (telling a card from a same-named copy of itself
// apart), which this port also has no tracking for, so they read
// identically to their non-"Strictly" counterparts.
//
// `OppCtrl`/`OppOwn` are `X.getOpponents().contains(sourceController)` in
// Java, which is team-aware. This port has no team system, so both read as
// "controlled/owned by anyone other than sourceController" -- correct for
// every game this port can play today (two players, or free-for-all with
// no teams), wrong only once a team variant exists to disagree with it.
func propertyMatches(g *Game, c *Card, p valid.Property, sourceController PlayerID, source CardID) bool {
	name := p.Name
	if p.Compare != nil {
		return compareMatches(c, *p.Compare)
	}
	switch {
	case strings.HasPrefix(name, "ChosenCard"):
		// ChosenCardStrict collapses to ChosenCard: Java's "Strict" form
		// additionally checks equalsWithGameTimestamp, telling a chosen card
		// from a same-named copy of itself apart across a zone change this
		// port has no game-timestamp tracking for -- the same simplification
		// Self/StrictlyOther's own doc comment already makes.
		sc, ok := sourceCard(g, source)
		return ok && containsCard(sc.Memory.Chosen(), c.ID)
	case name == "nonChosenCard":
		sc, ok := sourceCard(g, source)
		return ok && !containsCard(sc.Memory.Chosen(), c.ID)
	case name == "IsRemembered":
		sc, ok := sourceCard(g, source)
		return ok && containsEntity(sc.Memory.Remembered(), CardEntity(c.ID))
	case name == "IsImprinted":
		sc, ok := sourceCard(g, source)
		return ok && containsCard(sc.Memory.Imprinted(), c.ID)
	case strings.HasPrefix(name, "YouCtrl"):
		return c.Controller == sourceController
	case strings.HasPrefix(name, "YouDontCtrl"):
		return c.Controller != sourceController
	case strings.HasPrefix(name, "OppCtrl"):
		return c.Controller != sourceController
	case strings.HasPrefix(name, "YouDontOwn"):
		return c.Owner != sourceController
	case strings.HasPrefix(name, "YouOwn"):
		return c.Owner == sourceController
	case strings.HasPrefix(name, "OppOwn"):
		return c.Owner != sourceController
	case strings.HasPrefix(name, "StrictlyOther"), strings.HasPrefix(name, "Other"):
		// StrictlySelf/StrictlyOther are Java's game-timestamp-aware forms
		// of Self/Other, for telling a card from a same-named copy of
		// itself apart. This port has no LKI/game-timestamp tracking
		// (game-state.md's "Not ported yet"), so both read as plain
		// identity, the same simplification Self's own doc comment already
		// makes for YouCtrl/OppCtrl's LKI gap.
		return c.ID != source
	case strings.HasPrefix(name, "Self"):
		return c.ID == source
	case name == "tapped":
		return c.Tapped
	case name == "untapped":
		return !c.Tapped
	}
	if rest, ok := strings.CutPrefix(name, "without"); ok {
		return !c.HasKeyword(rest)
	}
	if rest, ok := strings.CutPrefix(name, "with"); ok {
		// "without" is checked first: it also starts with "with", and
		// stripping the shorter prefix from it would leave "out<Keyword>"
		// instead of the keyword name, the same ordering mistake Java's own
		// nested if avoids by checking the longer prefix first.
		return c.HasKeyword(rest)
	}
	if rest, ok := strings.CutPrefix(name, "hasKeyword"); ok {
		return c.HasKeyword(rest)
	}
	if color, mustHave, ok := colorMatches(name); ok {
		return mustHave == c.Colors().Has(color)
	}
	switch name {
	case "Colorless":
		return c.Colors().IsColorless()
	case "nonColorless":
		return !c.Colors().IsColorless()
	case "MultiColor":
		return c.Colors().Count() > 1
	}
	if rest, ok := strings.CutPrefix(name, "non"); ok {
		// CardStateProperty.java's own generic tail, reached once none of
		// its named branches (color included, checked first, above) claim
		// the property -- "nonLand", "nonCreature", "nonArtifact", and
		// every other `non<Type>` this port's `cardtype` recognizes.
		return !c.Type().HasStringType(rest)
	}
	return c.Type().HasStringType(name)
}

// colorMatches is CardStateProperty.hasProperty's color branch (White,
// Blue, Black, Red, Green, each with a `non` form), exact-matched rather
// than Java's `Contains`/prefix-stripped form: this port does not
// implement the "Source" suffix (`WhiteSource`, a damage-context check
// needing a source distinct from the candidate card, which propertyMatches
// has no context for), and an exact match is what keeps that gap honest --
// "WhiteSource" falls through to propertyMatches' own type-name
// fallthrough (false for every card, the same as any other unimplemented
// property) rather than being silently misread as bare "White".
//
// mustHave mirrors Java's own local of the same name: false for the `non`
// form, meaning the card must lack the color rather than carry it.
func colorMatches(name string) (color mana.Colors, mustHave bool, ok bool) {
	mustHave = true
	colorName := name
	if rest, isNon := strings.CutPrefix(name, "non"); isNon {
		mustHave, colorName = false, rest
	}
	switch colorName {
	case "White":
		return mana.White, mustHave, true
	case "Blue":
		return mana.Blue, mustHave, true
	case "Black":
		return mana.Black, mustHave, true
	case "Red":
		return mana.Red, mustHave, true
	case "Green":
		return mana.Green, mustHave, true
	}
	return 0, false, false
}

// sourceCard resolves source to its own *Card, for the properties that read
// something off the card the spec is written on rather than the candidate c
// -- ChosenCard, IsRemembered, IsImprinted. Game.Card panics on NoCard
// (GO-7: that is an engine invariant breach everywhere else it is called),
// but a Matches caller legitimately passes NoCard when there is no
// meaningful source at all (Matches' own doc comment: the base/property
// checks that do not need one). ok is false in exactly that case, so a
// property that needs a source but was not given one matches nothing, the
// same "false for every card" answer any other unresolvable property gives,
// rather than panicking on a caller that was never wrong to omit one.
func sourceCard(g *Game, source CardID) (*Card, bool) {
	if source == NoCard {
		return nil, false
	}
	return g.Card(source), true
}

// containsCard and containsEntity are linear membership checks over a
// Memory list (Chosen/Imprinted/Remembered) -- these lists hold at most a
// handful of entries (memory.go's own doc comment: "the overwhelming
// majority of cards remember nothing"), so a set is not worth building for
// them the way collect.OrderedSet already is for the list itself.
func containsCard(list []CardID, id CardID) bool {
	for _, x := range list {
		if x == id {
			return true
		}
	}
	return false
}

func containsEntity(list []EntityID, e EntityID) bool {
	for _, x := range list {
		if x == e {
			return true
		}
	}
	return false
}

// compareMatches is the numeric-comparison branch of CardProperty.java:1423
// ("power"/"basePower"/"toughness"/"baseToughness"/"cmc"/"totalPT"/
// "numColors"/"numTypes", each crossed with LT/LE/EQ/GE/GT/NE/M2) --
// internal/valid.parseCompare already split the property into Field,
// Operator and Operand at load time (that package's own doc comment says
// evaluation waits here).
//
// Operand is only handled when it is a plain base-10 integer. Java resolves
// it with AbilityUtils.calculateAmount, which also accepts "X", "Chosen"
// (source.getChosenNumber()) and an SVar name -- none of which this port can
// resolve without an ability-context evaluator internal/expr does not have
// yet (compare.go's own doc comment: "resolving it needs a game"). A
// non-numeric Operand is a coverage gap, so the property matches nothing,
// the same as any other unimplemented property -- not a wrong answer for
// the common numeric case, which is what the corpus mostly uses these for.
func compareMatches(c *Card, cmp valid.Compare) bool {
	operand, err := strconv.Atoi(cmp.Operand)
	if err != nil {
		return false
	}
	value, ok := compareFieldValue(c, cmp.Field)
	if !ok {
		return false
	}
	return compareOp(value, cmp.Operator, operand)
}

// compareFieldValue reads the measured field CardProperty.java:1432-1451
// names. power/toughness are the full current values (Power/Toughness,
// Layer 7 and counters both folded in -- Java's getNetPower/getNetToughness).
// basePower/baseToughness are Layer 7 folded in but counters not yet added
// (layer7Power/layer7Toughness's own doc comment has the reason this is not
// BasePower/BaseToughness despite the name). totalPT is full power plus full
// toughness. numColors and numTypes have no unresolvable form, so they are
// always ok.
//
// ok is false wherever the underlying accessor's is -- an unresolvable "*"
// or Count$ printed value this port has no expr evaluator to resolve
// (BasePower's own doc comment), propagated rather than guessed at.
func compareFieldValue(c *Card, field string) (int, bool) {
	switch field {
	case "power":
		return c.Power()
	case "basePower":
		return c.layer7Power()
	case "toughness":
		return c.Toughness()
	case "baseToughness":
		return c.layer7Toughness()
	case "cmc":
		return c.CMC(), true
	case "totalPT":
		p, okP := c.Power()
		t, okT := c.Toughness()
		return p + t, okP && okT
	case "numColors":
		return c.Colors().Count(), true
	case "numTypes":
		return len(c.Type().CoreTypes()), true
	}
	return 0, false
}

// compareOp is Expressions.compare (forge/util/Expressions.java), ported
// operator for operator including M2's modulo-2 equality (a creature's power
// and an operand agreeing on even/odd, not on value).
func compareOp(left int, operator string, right int) bool {
	switch operator {
	case "LT":
		return left < right
	case "LE":
		return left <= right
	case "EQ":
		return left == right
	case "GE":
		return left >= right
	case "GT":
		return left > right
	case "NE":
		return left != right
	case "M2":
		return left%2 == right%2
	}
	return false
}
