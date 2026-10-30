package engine

//enginelint:allow id card game ability control valid player parts staticability

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/valid"
)

// untapRestriction is one UntapAdjust:<Type>:<N> keyword line of the untapping
// player (Untap.java:104-109): at most n permanents matching spec untap.
type untapRestriction struct {
	spec string
	n    int
}

// untapPlayerKeywords reads the untapping player's UntapAdjust and
// OnlyUntapChosen keyword lines (Untap.java:104-115), in keyword order. A type
// listed twice keeps the smaller count; a line without a number is skipped.
func (g *Game) untapPlayerKeywords(pid PlayerID) (restrictions []untapRestriction, onlyChosen [][]string) {
	for _, line := range g.Player(pid).KeywordLines() {
		parts := strings.Split(line, ":")
		switch {
		case parts[0] == "UntapAdjust" && len(parts) >= 3:
			n, err := strconv.Atoi(parts[2])
			if err != nil {
				continue
			}
			merged := false
			for i := range restrictions {
				if restrictions[i].spec == parts[1] {
					restrictions[i].n = min(restrictions[i].n, n)
					merged = true
				}
			}
			if !merged {
				restrictions = append(restrictions, untapRestriction{spec: parts[1], n: n})
			}
		case parts[0] == "OnlyUntapChosen" && len(parts) >= 2:
			onlyChosen = append(onlyChosen, strings.Split(parts[1], ","))
		}
	}
	return restrictions, onlyChosen
}

// untapCandidates is Untap.doUntap's untapList after the keyword
// restrictions (Untap.java:104-145): candidates is the active player's
// battlefield, in order, already cut to what can untap. OnlyUntapChosen has the
// player pick one of its types, keeping that type only. Each UntapAdjust type
// sets its matching cards aside; the player then picks them one at a time
// until every restricted type has used up its count (a count of 0 drops that
// type) or they decline, and the picks follow the unrestricted cards.
func (g *Game) untapCandidates(controller PlayerController, active PlayerID, candidates []CardID) []CardID {
	restrictions, onlyChosen := g.untapPlayerKeywords(active)
	for _, types := range onlyChosen {
		i := controller.ChooseOption(g, active, NoCard, types)
		if i < 0 || i >= len(types) {
			g.recordPendingError(fmt.Errorf("engine: OnlyUntapChosen: type choice %d out of range", i))
			return nil
		}
		parsed := valid.Parse(types[i])
		kept := candidates[:0:0]
		for _, id := range candidates {
			if Matches(g, g.Card(id), parsed, active, NoCard) {
				kept = append(kept, id)
			}
		}
		candidates = kept
	}
	if len(restrictions) == 0 {
		return candidates
	}
	var untapList, restrictList []CardID
	for _, id := range candidates {
		restricted := false
		for _, r := range restrictions {
			if Matches(g, g.Card(id), valid.Parse(r.spec), active, NoCard) {
				restricted = true
			}
		}
		if restricted {
			restrictList = append(restrictList, id)
		} else {
			untapList = append(untapList, id)
		}
	}
	for len(restrictList) > 0 {
		kept := restrictions[:0:0]
		for _, r := range restrictions {
			if r.n != 0 {
				kept = append(kept, r)
				continue
			}
			spec := valid.Parse(r.spec)
			restrictList = filterCards(restrictList, func(id CardID) bool {
				return !Matches(g, g.Card(id), spec, active, NoCard)
			})
		}
		restrictions = kept
		if len(restrictList) == 0 {
			break
		}
		picked := controller.ChooseCardsForEffect(g, active, NoCard, restrictList, 1, 1)
		if len(picked) != 1 || !containsCard(restrictList, picked[0]) {
			break
		}
		for i := range restrictions {
			if Matches(g, g.Card(picked[0]), valid.Parse(restrictions[i].spec), active, NoCard) {
				restrictions[i].n--
			}
		}
		untapList = append(untapList, picked[0])
		restrictList = filterCards(restrictList, func(id CardID) bool { return id != picked[0] })
	}
	return untapList
}

// untapOtherPlayerAllows is StaticAbilityUntapOtherPlayer.untap
// (StaticAbilityUntapOtherPlayer.java:24-47): some Mode$ UntapOtherPlayer
// static lets c untap in active's untap step, c matching its ValidCard$ and
// active its ValidPlayer$ (each optional) from the static host's side.
func (g *Game) untapOtherPlayerAllows(c *Card, active PlayerID) bool {
	for _, p := range g.Players() {
		for _, host := range g.traitHosts(p) {
			h := g.Card(host)
			if h.Def == nil {
				continue
			}
			for _, face := range h.traitFaces() {
				for _, s := range face.Statics {
					if !strings.EqualFold(s.Name, "UntapOtherPlayer") || !g.staticConditionsMet(h, s) {
						continue
					}
					if !staticParamsKnown(s, untapOtherPlayerParams) {
						continue
					}
					if v, ok := s.Param("ValidCard"); ok && !Matches(g, c, valid.Parse(v), h.Controller(), h.ID) {
						continue
					}
					if v, ok := s.Param("ValidPlayer"); ok {
						if matched, recognized := matchesPlayerSpec(g, active, h.Controller(), h.ID, v); !matched || !recognized {
							continue
						}
					}
					return true
				}
			}
		}
	}
	return false
}

var untapOtherPlayerParams = map[string]bool{
	"mode": true, "validcard": true, "validplayer": true, "effectzone": true, "description": true,
	"ispresent": true, "presentzone": true, "presentcompare": true, "presentplayer": true,
}

// filterCards is ids without the cards keep reports false for, as a new slice.
func filterCards(ids []CardID, keep func(CardID) bool) []CardID {
	return slices.DeleteFunc(slices.Clone(ids), func(id CardID) bool { return !keep(id) })
}
