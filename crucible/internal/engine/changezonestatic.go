package engine

//enginelint:allow game ability card animate valid effecthelpers id zone presentcheck

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
)

// changeZoneStaticTemplate is GameAction.setupStaticEffect's static
// (GameAction.java:679-730) for a ChangeZone's StaticEffect$: the SVar it
// names is a Mode$ Continuous static over Affected$ Card.IsRemembered, which
// the moved card alone satisfies. The characteristics it changes are read once,
// before anything moves, into the animateRecord each moved card takes
// (applyChangeZoneStatic); nil when the line names no StaticEffect$.
//
// Only the Affected$ and characteristic params every corpus line writes
// (AddType, AddColor, SetColor, SetPower, SetToughness, AddKeyword,
// RemoveCardTypes, RemoveSubTypes, RemoveCreatureTypes, RemoveAllAbilities,
// AddAbility) are read; any
// other is refused by name (GO-7); StaticEffectCheckSVar$ and
// StaticEffectSVarCompare$ gate the static (GameAction.java:686-694).
func changeZoneStaticTemplate(g *Game, a *Ability) (*animateRecord, error) {
	if _, ok := a.Params.Param("StaticEffect"); !ok {
		return nil, nil
	}
	// GameAction.java:686-694: StaticEffectCheckSVar$ against StaticEffectSVarCompare$
	// (default GE1) gates the whole static, read once before any card moves
	// (Dance of the Manse: X of 6 or more makes the cards creatures); a failed
	// check applies no static.
	if !checkSVarMatches(g, g.Card(a.Source), a.Amounts, a.Params, "StaticEffectCheckSVar", "StaticEffectSVarCompare", "") {
		return nil, nil
	}
	var st *compile.Ability
	for _, sub := range a.Params.Subs {
		if strings.EqualFold(sub.Key, "StaticEffect") {
			st = sub.Ability
		}
	}
	if st == nil {
		return nil, fmt.Errorf("engine: ChangeZone: StaticEffect$ names an SVar that is not a static ability")
	}
	if !strings.EqualFold(st.Name, "Continuous") {
		return nil, fmt.Errorf("engine: ChangeZone: StaticEffect$ Mode$ %s not resolvable yet", st.Name)
	}
	r := &animateRecord{Permanent: true}
	var removeCreature bool
	for _, p := range st.Params {
		switch strings.ToLower(p.Key) {
		case "mode", "description", "stackdescription":
		case "affected":
			if p.Value != "Card.IsRemembered" {
				return nil, fmt.Errorf("engine: ChangeZone: StaticEffect$ Affected$ %q not resolvable yet", p.Value)
			}
		case "addtype":
			var line cardtype.Line
			for _, word := range strings.Split(p.Value, " & ") {
				line = line.Union(cardtype.ParseToken(strings.TrimSpace(word)))
			}
			r.Types.AddTypes = line
		case "addcolor", "setcolor":
			if r.HasColors {
				return nil, fmt.Errorf("engine: ChangeZone: StaticEffect$ names more than one color change, not resolvable yet")
			}
			r.HasColors, r.OverwriteColors = true, strings.EqualFold(p.Key, "SetColor")
			for _, name := range strings.Split(p.Value, ",") {
				name = strings.TrimSpace(name)
				c, ok := colorFromName(strings.ToUpper(name[:1]) + strings.ToLower(name[1:]))
				if !ok {
					return nil, fmt.Errorf("engine: ChangeZone: StaticEffect$ %s$ %q not resolvable", p.Key, p.Value)
				}
				r.Colors |= c
			}
		case "setpower", "settoughness":
			n, err := strconv.Atoi(p.Value)
			if err != nil {
				return nil, fmt.Errorf("engine: ChangeZone: StaticEffect$ %s$ %q not resolvable yet", p.Key, p.Value)
			}
			if strings.EqualFold(p.Key, "SetPower") {
				r.Power, r.HasPower = n, true
			} else {
				r.Toughness, r.HasToughness = n, true
			}
		case "addkeyword":
			r.AddKeywords = append(r.AddKeywords, strings.Split(p.Value, " & ")...)
		case "removecardtypes":
			r.Types.RemoveCardTypes = true
		case "removesubtypes":
			r.Types.RemoveSubTypes = true
		case "addability":
			// Supper for Spiders' "Food artifacts with {2},{T}, Sacrifice": each
			// named SVar is a granted activated ability (Layer 6).
			var granted int
			for _, sub := range st.Subs {
				if strings.EqualFold(sub.Key, "AddAbility") {
					r.abilities = append(r.abilities, sub.Ability)
					granted++
				}
			}
			if granted != len(strings.Split(p.Value, " & ")) {
				return nil, fmt.Errorf("engine: ChangeZone: StaticEffect$ AddAbility$ %q names an SVar that is not an ability", p.Value)
			}
		case "removecreaturetypes":
			removeCreature = true
		case "removeallabilities":
			r.removeAbilities, r.RemoveAllKW = removalAll, true
		default:
			return nil, fmt.Errorf("engine: ChangeZone: StaticEffect$ %s$ not resolvable yet", p.Key)
		}
	}
	if removeCreature {
		drop, ok := subtypeCategoryDrop(g, false, true, false, false)
		if !ok {
			return nil, fmt.Errorf("engine: ChangeZone: StaticEffect$ RemoveCreatureTypes$ needs the subtype vocabulary, which this game's DB lacks")
		}
		r.Types.DropSubtype = drop
	}
	return r, nil
}

// applyChangeZoneStatic gives a card ChangeZone just moved the template's
// characteristic change for as long as it stays on the battlefield
// (setupStaticEffect leaves play with the card, :729-730; clearAnimates drops
// a Permanent record at the zone change). A card that did not end up on the
// battlefield has nothing to change.
func (g *Game) applyChangeZoneStatic(tmpl *animateRecord, id CardID) {
	if tmpl == nil || g.Card(id).Zone != Battlefield {
		return
	}
	g.timestamp++
	r := *tmpl
	r.Card, r.Timestamp = id, g.timestamp
	g.addAnimate(r)
}
