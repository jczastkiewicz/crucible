package engine

//enginelint:allow card game ability control effecthelpers defined condition textwords valid

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// changeTextEffect is ChangeTextEffect.java: a color word and/or a basic land
// or creature type word is replaced by another in the text of each target
// (CR 612), recorded as a timestamped word change the next Layer 3 pass folds
// into the card's definition (textwords.go, ADR-0039). Each word is a literal
// or a Choose token the resolving player answers: ChangeColorWord$ picks from
// the five colors (the new one never the original), ChangeTypeWord$ from the
// basic land or creature types (ForbiddenNewTypes$ and the original removed
// from the new one's list). Only Duration$ Permanent is permanent; any other
// value, absent included, ends at cleanup (ChangeTextEffect.java:29), which is
// kept.
type changeTextEffect struct{}

func (changeTextEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	cards, err := targetedOrDefinedCards(source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: ChangeText: %w", err)
	}
	g.timestamp++
	ts := g.timestamp
	duration, _ := a.Params.Param("Duration")

	var colorFrom, colorTo, typeFrom, typeTo string
	if spec, ok := a.Params.Param("ChangeColorWord"); ok {
		if colorFrom, colorTo, err = changeTextColorWords(g, a, controller, spec); err != nil {
			return err
		}
	}
	if spec, ok := a.Params.Param("ChangeTypeWord"); ok {
		if typeFrom, typeTo, err = changeTextTypeWords(g, a, controller, spec); err != nil {
			return err
		}
	}
	for _, id := range cards {
		if colorFrom != "" && colorTo != "" {
			g.addTextWord(textWordRecord{Card: id, Timestamp: ts, Kind: wordColor, From: colorFrom, To: colorTo, Permanent: duration == "Permanent"})
		}
		if typeFrom != "" && typeTo != "" {
			g.addTextWord(textWordRecord{Card: id, Timestamp: ts, Kind: wordType, From: typeFrom, To: typeTo, Permanent: duration == "Permanent"})
		}
	}
	return nil
}

// changeTextColorWords reads ChangeColorWord$ "<from> <to>": from is Choose,
// a color name or Any; to is Choose or a color name. Choose asks the resolving
// player for one of the five colors, and for the new word one other than the
// original (every color when the original is Any), ChangeTextEffect.java:32-60.
func changeTextColorWords(g *Game, a *Ability, controller PlayerController, spec string) (from, to string, err error) {
	toks := strings.Fields(spec)
	if len(toks) != 2 {
		return "", "", fmt.Errorf("engine: ChangeText: ChangeColorWord$ %q is not two words", spec)
	}
	var original mana.Colors
	switch toks[0] {
	case "Choose":
		c := controller.ChooseColors(g, a.Controller, a.Source, mana.AllColors, 1, 1)
		if c.Count() != 1 {
			return "", "", fmt.Errorf("engine: ChangeText: ChangeColorWord$ chose %d colors, want 1", c.Count())
		}
		original = c
		from, _ = colorWordOf(c)
	case "Any":
		from = "Any"
	default:
		word, ok := colorWordFor(toks[0])
		if !ok {
			return "", "", fmt.Errorf("engine: ChangeText: ChangeColorWord$ %q is not a color", toks[0])
		}
		original, _ = colorFromName(word)
		from = word
	}
	if toks[1] != "Choose" {
		if to, ok := colorWordFor(toks[1]); ok && toks[1] != "Any" {
			return from, to, nil
		}
		return "", "", fmt.Errorf("engine: ChangeText: ChangeColorWord$ %q is not a color", toks[1])
	}
	options := mana.AllColors
	if original != 0 {
		options = mana.AllColors &^ original
	}
	c := controller.ChooseColors(g, a.Controller, a.Source, options, 1, 1)
	if c.Count() != 1 || c&options != c {
		return "", "", fmt.Errorf("engine: ChangeText: ChangeColorWord$ chose an invalid new color")
	}
	to, _ = colorWordOf(c)
	return from, to, nil
}

// changeTextTypeWords reads ChangeTypeWord$ "<from> <to>": from is
// ChooseBasicLandType, ChooseCreatureType or a literal; to is any Choose token
// or a literal. A Choose pick is made from the game DB's subtype vocabulary
// (ChangeTextEffect.java:62-99); the new word's list loses the original and
// ForbiddenNewTypes$, which a literal new word ignores.
func changeTextTypeWords(g *Game, a *Ability, controller PlayerController, spec string) (from, to string, err error) {
	toks := strings.Fields(spec)
	if len(toks) != 2 {
		return "", "", fmt.Errorf("engine: ChangeText: ChangeTypeWord$ %q is not two words", spec)
	}
	var forbidden []string
	if raw, ok := a.Params.Param("ForbiddenNewTypes"); ok {
		forbidden = strings.Split(raw, ",")
	}
	if strings.HasPrefix(toks[0], "Choose") {
		options, err := changeTextTypeOptions(g, toks[0])
		if err != nil {
			return "", "", err
		}
		if from, err = pickTypeWord(g, a, controller, options); err != nil {
			return "", "", err
		}
	} else {
		from = toks[0]
	}
	if !strings.HasPrefix(toks[1], "Choose") {
		return from, toks[1], nil
	}
	options, err := changeTextTypeOptions(g, toks[1])
	if err != nil {
		return "", "", err
	}
	drop := slices.Concat(forbidden, []string{from})
	kept := options[:0:0]
	for _, t := range options {
		if !containsString(drop, t) {
			kept = append(kept, t)
		}
	}
	if to, err = pickTypeWord(g, a, controller, kept); err != nil {
		return "", "", err
	}
	return from, to, nil
}

// changeTextTypeOptions is the type list a Choose token names.
func changeTextTypeOptions(g *Game, token string) ([]string, error) {
	reg := g.db.Types()
	if reg == nil {
		return nil, fmt.Errorf("engine: ChangeText: %s needs the type vocabulary, which this game's DB lacks", token)
	}
	switch token {
	case "ChooseBasicLandType":
		return reg.Members(cardtype.CategoryBasic), nil
	case "ChooseCreatureType":
		return reg.Members(cardtype.CategoryCreature), nil
	}
	return nil, fmt.Errorf("engine: ChangeText: %s not resolvable yet", token)
}

// pickTypeWord asks the resolving player for one of options.
func pickTypeWord(g *Game, a *Ability, controller PlayerController, options []string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("engine: ChangeText: no types to choose from")
	}
	i := controller.ChooseOption(g, a.Controller, a.Source, options)
	if i < 0 || i >= len(options) {
		return "", fmt.Errorf("engine: ChangeText: choice %d out of range", i)
	}
	return options[i], nil
}
