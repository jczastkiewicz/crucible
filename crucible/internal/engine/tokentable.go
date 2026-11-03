// The token table a token-making effect fills before any token exists, and
// the CreateToken replacements run over it.
//
// Ported from forge-game/src/main/java/forge/game/card/TokenCreateTable.java
// (a Guava HashBasedTable: creator -> prototype -> count, both maps in
// insertion order) and TokenEffectBase.makeTokenTable
// (TokenEffectBase.java:74-98).

package engine

//enginelint:allow id card game control token replacement replacementchoice replaceeffect takeinitiativeeffect valid zone

import (
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/expr"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// tokenCell is one table cell: n tokens of one prototype (spec) that creator
// makes. creator owns them; spec.Controller is who they enter under.
// original marks a prototype the effect itself named, not one a replacement
// put in (RememberOriginalTokens$).
type tokenCell struct {
	creator  PlayerID
	spec     tokenSpec
	n        int
	original bool
}

// tokenTable is TokenCreateTable: cells grouped by creator, rows in the order
// their creator first appeared, cells in a row in the order they were put.
// Java's cellSet walks it in that order, and so does the creation loop.
type tokenTable struct {
	cells []tokenCell
}

// put adds c at the end of its creator's row, or as a new last row.
func (t *tokenTable) put(c tokenCell) {
	at := -1
	for i, e := range t.cells {
		if e.creator == c.creator {
			at = i
		}
	}
	if at < 0 {
		t.cells = append(t.cells, c)
		return
	}
	t.cells = append(t.cells, tokenCell{})
	copy(t.cells[at+2:], t.cells[at+1:])
	t.cells[at+1] = c
}

// creators is rowKeySet: each creator once, in row order.
func (t *tokenTable) creators() []PlayerID {
	var out []PlayerID
	for _, c := range t.cells {
		if !containsPlayer(out, c.creator) {
			out = append(out, c.creator)
		}
	}
	return out
}

// removeRow drops every cell creator makes.
func (t *tokenTable) removeRow(creator PlayerID) {
	kept := t.cells[:0]
	for _, c := range t.cells {
		if c.creator != creator {
			kept = append(kept, c)
		}
	}
	t.cells = kept
}

// proto is c's prototype as a valid string sees it: the token as it would
// enter, owned by the creator and controlled by spec.Controller, in no zone.
// It has no arena slot, so a property that looks a card up by ID cannot read
// it; every ValidToken$ line in the corpus is a type, name or control check.
func (c tokenCell) proto() *Card {
	ctrl := c.spec.Controller
	if ctrl == NoPlayer {
		ctrl = c.creator
	}
	return &Card{Def: c.spec.Def, Owner: c.creator, controller: ctrl, IsToken: true, Zone: None}
}

// filterAmount is TokenCreateTable.getFilterAmount for a CreateToken line r
// hosted by h: with neither ValidPlayer$ nor ValidToken$, every cell's count;
// otherwise 0 when no row (ValidPlayer$) or no prototype (ValidToken$)
// matches. With only one of the two, Java sums every cell of the table, not
// only the matching ones (TokenCreateTable.java:59-73); with both, the cells
// matching both. canReplace only asks whether it is above 0.
func (g *Game) filterAmount(t *tokenTable, h *Card, r *compile.Ability) int {
	vp, namesPlayer := r.Param("ValidPlayer")
	vt, namesToken := r.Param("ValidToken")
	rowOK := func(p PlayerID) bool {
		m, known := matchesPlayerSpec(g, p, h.Controller(), h.ID, vp)
		return known && m
	}
	var tokenValid valid.Spec
	if namesToken {
		tokenValid = valid.Parse(vt)
	}
	cellOK := func(c tokenCell) bool { return Matches(g, c.proto(), tokenValid, h.Controller(), h.ID) }
	sum, anyRow, anyCell := 0, !namesPlayer, !namesToken
	for _, c := range t.cells {
		sum += c.n
		anyRow = anyRow || rowOK(c.creator)
		anyCell = anyCell || cellOK(c)
	}
	if !anyRow || !anyCell {
		return 0
	}
	if !namesPlayer || !namesToken {
		return sum
	}
	both := 0
	for _, c := range t.cells {
		if rowOK(c.creator) && cellOK(c) {
			both += c.n
		}
	}
	return both
}

// tokenCreateParams are the CreateToken params tokensReplaced evaluates; a
// line naming any other is skipped (onlyParams).
var tokenCreateParams = []string{"validtoken", "validplayer", "effectonly", "optional", "optionaldecider", "layer"}

// tokensReplaced is makeTokenTable's replacement step: for each creator in
// the table (a snapshot of its rows), the Event$ CreateToken replacements
// run with that creator as Affected and the whole table as Token, in CR 616
// order (runReplacements, Layer$ first). A ReplaceWith$ updates the table in
// place; one that replaced the event outright drops the row. ValidPlayer$ is
// matched against the creator, ValidToken$ through filterAmount.
func (g *Game) tokensReplaced(controller PlayerController, t *tokenTable) {
	for _, p := range t.creators() {
		res := g.runReplacements(controller, p, func() []replacementCandidate {
			var cands []replacementCandidate
			g.eachReplacement("CreateToken", func(h *Card, amounts map[string]expr.Amount, r *compile.Ability) bool {
				if !onlyParams(r, tokenCreateParams...) {
					return false
				}
				if v, ok := r.Param("ValidPlayer"); ok {
					if m, known := matchesPlayerSpec(g, p, h.Controller(), h.ID, v); !known || !m {
						return false
					}
				}
				if g.filterAmount(t, h, r) <= 0 || !replacementRequirementsCheck(g, h, amounts, r) {
					return false
				}
				sub := replaceWithSub(r)
				if sub == nil {
					return false
				}
				cands = append(cands, replacementCandidate{host: h, rule: r, layer: replacementLayerOf(r), apply: func() replacementResult {
					ev := replacementEvent{amountName: "TokenNum", amount: g.filterAmount(t, h, r), tokens: t, player: p}
					if !g.runReplaceWith(controller, h, amounts, sub, &ev) {
						return replacementNotReplaced
					}
					return ev.result
				}})
				return false
			})
			return cands
		})
		if res == replacementReplaced {
			t.removeRow(p)
		}
	}
}

// makeTokens is makeTokenTable for the effects that name no Token params of
// their own (Investigate, Incubate, Recruit, Empower, Amass, Endure): the
// cells, the CreateToken replacements over them, then one token per count of
// each cell in table order. The first cell's TokenTapped$ and enter-with
// counters are the effect's, so a cell a replacement put in gets them too
// (Incubate's +1/+1 counters, TokenEffectBase.java:128-146); its power and
// toughness are its prototype's own (Endure's X/X Spirit).
func (g *Game) makeTokens(controller PlayerController, cells ...tokenCell) []CardID {
	if len(cells) == 0 {
		return nil
	}
	base := cells[0].spec
	var t tokenTable
	for _, c := range cells {
		c.original = true
		t.put(c)
	}
	g.tokensReplaced(controller, &t)
	var created []CardID
	for _, c := range t.cells {
		spec := c.spec
		spec.Owner = c.creator
		if !c.original {
			spec.Tapped, spec.P1P1 = base.Tapped, base.P1P1
		}
		for range c.n {
			created = append(created, g.createToken(controller, spec))
		}
	}
	return created
}
