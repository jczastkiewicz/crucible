// Animate-shaped continuous effects: the one-shot counterpart to a Mode$
// Continuous line, created by a resolving Animate/AnimateAll/Debuff/
// Protection/ProtectionAll ability rather than a card's own S: line.
//
// Ported from forge-game/src/main/java/forge/game/ability/effects/
// AnimateEffectBase.java's doAnimate and the addChanged* calls
// DebuffEffect/ProtectEffect make.

package engine

import (
	"fmt"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// animateRecord is one resolved effect's contribution to one card, every
// layer it touches under one timestamp: Layer 4 (Types), Layer 5 (Colors),
// Layer 6 (Keywords), Layer 7b (SetPower/SetToughness -- Java's addNewPT).
// Permanent is Duration$ Permanent; anything else ends at cleanup, the
// "until end of turn" default every one of these effects shares
// (SpellAbilityEffect.addUntilCommand).
type animateRecord struct {
	Card      CardID
	Timestamp uint64
	Permanent bool

	Types TypeEffect

	Colors          mana.Colors
	HasColors       bool
	OverwriteColors bool

	AddKeywords    []string
	RemoveKeywords []string
	RemoveAllKW    bool

	Power, Toughness       int
	HasPower, HasToughness bool
}

// changesTypes reports whether r carries any Layer 4 change.
func (r *animateRecord) changesTypes() bool {
	t := r.Types
	return !t.AddTypes.IsEmpty() || !t.RemoveTypes.IsEmpty() || t.RemoveCardTypes ||
		t.RemoveSuperTypes || t.RemoveSubTypes || t.DropSubtype != nil
}

// applyAnimateEffects re-adds every animateRecord into its card's layer
// mods -- applyPumpEffects' own contract (continuous.go): called from
// CheckStateBasedActions after the Mode$ Continuous appliers have cleared
// and rebuilt every battlefield card's mods for this pass. A record whose
// card is not on the battlefield is skipped.
func applyAnimateEffects(g *Game) {
	for i := range g.animates {
		r := &g.animates[i]
		c := g.Card(r.Card)
		if c.Zone != Battlefield {
			continue
		}
		if r.changesTypes() {
			te := r.Types
			te.Timestamp = r.Timestamp
			c.TypeMod.Add(te)
		}
		if r.HasColors {
			c.ColorMod.Add(ColorEffect{Timestamp: r.Timestamp, Colors: r.Colors, Overwrite: r.OverwriteColors})
		}
		if len(r.AddKeywords) > 0 || len(r.RemoveKeywords) > 0 || r.RemoveAllKW {
			c.KeywordMod.Add(KeywordEffect{
				Timestamp: r.Timestamp, AddKeywords: r.AddKeywords,
				RemoveKeywords: r.RemoveKeywords, RemoveAll: r.RemoveAllKW,
			})
		}
		if r.HasPower || r.HasToughness {
			c.PT.Add(PTEffect{
				Layer: LayerSetPT, Timestamp: r.Timestamp,
				Power: r.Power, Toughness: r.Toughness,
				HasPower: r.HasPower, HasToughness: r.HasToughness,
			})
		}
	}
}

// addAnimate records r and applies it at once, so the card's
// characteristics change as the effect resolves rather than at the next
// state-based-action pass -- a SubAbility$ chained after it (or a trigger
// checked right after) already sees the animated card, the way Java's
// addChanged* calls take effect immediately.
func (g *Game) addAnimate(r animateRecord) {
	g.animates = append(g.animates, r)
	saved := g.animates
	g.animates = g.animates[len(g.animates)-1:]
	applyAnimateEffects(g)
	g.animates = saved
}

// clearAnimates drops every animateRecord naming id -- clearPumps' own
// reason (game.go): a CardID survives a zone change here, so a Permanent
// record would otherwise reapply to the card when it returns.
func (g *Game) clearAnimates(id CardID) {
	kept := g.animates[:0]
	for _, r := range g.animates {
		if r.Card != id {
			kept = append(kept, r)
		}
	}
	g.animates = kept
}

// endAnimatesAtCleanup drops every non-Permanent record: CR 514.2's "until
// end of turn" effects ending, the same cleanup Pump records get.
func (g *Game) endAnimatesAtCleanup() {
	kept := g.animates[:0]
	for _, r := range g.animates {
		if r.Permanent {
			kept = append(kept, r)
		}
	}
	g.animates = kept
}

// subtypeCategoryDrop builds DropSubtype for the Remove*Types$ category
// params, reading the corpus' subtype vocabulary off the game's DB. A
// category removal with no vocabulary to test against is unresolvable.
func subtypeCategoryDrop(g *Game, land, creature, artifact, enchantment bool) (func(string) bool, bool) {
	if !land && !creature && !artifact && !enchantment {
		return nil, true
	}
	reg := g.db.Types()
	if reg == nil {
		return nil, false
	}
	return func(s string) bool {
		return (land && reg.IsLandType(s)) ||
			(creature && reg.IsCreatureType(s)) ||
			(artifact && reg.Is(cardtype.CategoryArtifact, s)) ||
			(enchantment && reg.Is(cardtype.CategoryEnchantment, s))
	}, true
}

// animateUnresolvedParams are the doAnimate/AnimateEffect params this port
// does not model: granted abilities, triggers, replacements, statics and
// SVars (each needs a runtime-parsed trait, PORT-2), hidden keywords,
// "can't have" keywords, removing abilities, all creature types, a
// renaming, mana-cost changes, a revert cost, a leave-the-battlefield
// replacement, what the animated card remembers or imprints, an Optional$
// confirmation, and the end-of-turn delayed trigger (AtEOT$).
var animateUnresolvedParams = [...]string{
	"Abilities", "Triggers", "Replacements", "staticAbilities", "sVars", "HiddenKeywords",
	"CantHaveKeyword", "RemoveAllAbilities", "RemoveNonManaAbilities", "RemoveThisAbility",
	"AddAllCreatureTypes", "Name", "ManaCost", "Incorporate", "RevertCost", "LeaveBattlefield",
	"RememberObjects", "ImprintCards", "Optional", "OptionQuestion", "AtEOT", "TgtZone",
	"Condition", "ConditionDefined",
}

// animateDuration reads Duration$: absent is until end of turn, Permanent
// never ends; every other duration (Perpetual, UntilHostLeavesPlay,
// UntilYourNextTurn, ...) needs a lifetime this port does not track.
func animateDuration(a *Ability, api string) (bool, error) {
	d, ok := a.Params.Param("Duration")
	if !ok {
		return false, nil
	}
	if d != "Permanent" {
		return false, fmt.Errorf("engine: %s: Duration$ %q not resolvable yet", api, d)
	}
	return true, nil
}

// buildAnimate is AnimateEffect.resolve's and AnimateAllEffect.resolve's
// shared read of the characteristic params, before doAnimate: Power$/
// Toughness$ (Layer 7b), Types$/RemoveTypes$ (comma lists) and the
// Remove*Types$ flags (Layer 4), Colors$ -- a comma list of color names,
// ChosenColor or All -- with OverwriteColors$ (Layer 5), and Keywords$/
// RemoveKeywords$ (" & " lists, Layer 6). Types$ ChosenType and a
// keyword naming one of the host's SVars (Java substitutes its text) are
// not resolved.
func buildAnimate(g *Game, a *Ability, api string) (animateRecord, error) {
	var r animateRecord
	if err := rejectParams(a, api, animateUnresolvedParams[:]...); err != nil {
		return r, err
	}
	permanent, err := animateDuration(a, api)
	if err != nil {
		return r, err
	}
	r.Permanent = permanent
	if r.HasPower = hasParam(a, "Power"); r.HasPower {
		if r.Power, err = optionalAmount(g, a, api, "Power", 0); err != nil {
			return r, err
		}
	}
	if r.HasToughness = hasParam(a, "Toughness"); r.HasToughness {
		if r.Toughness, err = optionalAmount(g, a, api, "Toughness", 0); err != nil {
			return r, err
		}
	}
	for _, key := range [...]string{"Types", "RemoveTypes"} {
		raw, ok := a.Params.Param(key)
		if !ok {
			continue
		}
		var line cardtype.Line
		for _, word := range strings.Split(raw, ",") {
			word = strings.TrimSpace(word)
			if strings.HasPrefix(word, "ChosenType") {
				return r, fmt.Errorf("engine: %s: %s$ %q not resolvable yet", api, key, raw)
			}
			line = line.Union(cardtype.ParseToken(word))
		}
		if key == "Types" {
			r.Types.AddTypes = line
		} else {
			r.Types.RemoveTypes = line
		}
	}
	r.Types.RemoveCardTypes = hasParam(a, "RemoveCardTypes")
	r.Types.RemoveSuperTypes = hasParam(a, "RemoveSuperTypes")
	r.Types.RemoveSubTypes = hasParam(a, "RemoveSubTypes")
	drop, ok := subtypeCategoryDrop(g, hasParam(a, "RemoveLandTypes"), hasParam(a, "RemoveCreatureTypes"),
		hasParam(a, "RemoveArtifactTypes"), hasParam(a, "RemoveEnchantmentTypes"))
	if !ok {
		return r, fmt.Errorf("engine: %s: a Remove*Types$ category needs the subtype vocabulary, which this game's DB lacks", api)
	}
	r.Types.DropSubtype = drop

	if raw, ok := a.Params.Param("Colors"); ok {
		r.HasColors = true
		r.OverwriteColors = hasParam(a, "OverwriteColors")
		switch raw {
		case "ChosenColor":
			r.Colors = g.Card(a.Source).Memory.ChosenColors()
		case "All":
			r.Colors = mana.AllColors
		default:
			for _, name := range strings.Split(raw, ",") {
				name = strings.TrimSpace(name)
				if strings.EqualFold(name, "Colorless") {
					continue
				}
				c, ok := colorFromName(strings.ToUpper(name[:1]) + strings.ToLower(name[1:]))
				if !ok {
					return r, fmt.Errorf("engine: %s: Colors$ %q not resolvable", api, raw)
				}
				r.Colors |= c
			}
		}
	}
	if raw, ok := a.Params.Param("Keywords"); ok {
		for _, k := range strings.Split(raw, " & ") {
			if _, isSVar := a.Amounts[strings.ToLower(k)]; isSVar || strings.Contains(k, "HIDDEN") {
				return r, fmt.Errorf("engine: %s: Keywords$ %q not resolvable yet", api, k)
			}
			r.AddKeywords = append(r.AddKeywords, k)
		}
	}
	if raw, ok := a.Params.Param("RemoveKeywords"); ok {
		r.RemoveKeywords = strings.Split(raw, " & ")
	}
	return r, nil
}

// empty reports whether r changes nothing at all.
func (r *animateRecord) empty() bool {
	return !r.changesTypes() && !r.HasColors && len(r.AddKeywords) == 0 && len(r.RemoveKeywords) == 0 &&
		!r.RemoveAllKW && !r.HasPower && !r.HasToughness
}

// animateCards applies template to each card on the battlefield under one
// fresh timestamp (every Animate event takes getNextTimestamp once).
func (g *Game) animateCards(template animateRecord, cards []CardID) {
	g.timestamp++
	ts := g.timestamp
	for _, id := range cards {
		if g.Card(id).Zone != Battlefield {
			continue
		}
		r := template
		r.Card, r.Timestamp = id, ts
		g.addAnimate(r)
	}
}
