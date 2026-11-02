// Mode$ ManaConvert (StaticAbilityManaConvert.manaConvert) for casting a
// spell: a static that lets its player spend mana as though it were mana of
// any color ("AnyType->AnyColor") or of any type ("AnyType->AnyType") on the
// spells its ValidCard$ and ValidSA$ name. Java builds a ManaConversionMatrix
// the payment reads; here the same effect rewrites the cost the way a
// MayPlayIgnoreColor$/MayPlayIgnoreType$ grant does (anyColorCost,
// anyTypeCost), so the payment code stays one path.
//
// Activated-ability costs read the same lines (manaConvertForActivation).
// The per-color conversions (White->Red, C->AnyColor, ...) are Java's
// ManaConversionMatrix itself (manaColorMatrix), which payManaCostX applies to
// each colored shard. Optional$ (North Star) is asked as the spell's cost is
// paid (StaticAbilityManaConvert.java:41-47).

//enginelint:allow ability id zone card game valid player continuous staticability zonemove cantbecast saspec castspell castoptions control amount

package engine

import (
	"fmt"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// manaConvertParams are the params a ManaConvert line may carry that this
// port evaluates; any other makes the line unread. AffectedZone$ (Abstruse
// Appropriation) is read by nothing in StaticAbilityManaConvert, so it is
// accepted and ignored the same way.
var manaConvertParams = map[string]bool{
	"mode": true, "validplayer": true, "validcard": true, "validsa": true, "manaconversion": true, "optional": true,
	"effectzone": true, "condition": true, "description": true, "secondary": true, "affectedzone": true,
}

// manaConversionKind is what a ManaConversion$ value does to a spell's cost:
// anyColor and anyType rewrite the cost, matrix is a per-color conversion the
// payment applies, none anything unread.
type manaConversionKind uint8

const (
	conversionNone manaConversionKind = iota
	conversionAnyColor
	conversionAnyType
	conversionMatrix
)

// readManaConversion is the cost rewrite s's ManaConversion$ asks for.
func readManaConversion(s *compile.Ability) manaConversionKind {
	v, _ := s.Param("ManaConversion")
	switch v = strings.TrimSpace(v); v {
	case "AnyType->AnyColor":
		return conversionAnyColor
	case "AnyType->AnyType":
		return conversionAnyType
	}
	if _, ok := parseManaConversion(v); ok {
		return conversionMatrix
	}
	return conversionNone
}

// Mana type bits of a colorMatrix row: the five colors as mana.Colors has
// them, colorless last (ManaAtom.MANATYPES order).
const (
	matrixColorless uint8 = 1 << 5
	matrixAllColors uint8 = 0x1f
	matrixAllTypes  uint8 = 0x3f
)

// colorMatrix is ManaConversionMatrix: for each mana type paid (rows W, U, B,
// R, G, C), the conversion row ORs in what it may pay as and the restriction
// row ANDs out what it may not. A row's possible uses are both together
// (getPossibleColorUses).
type colorMatrix struct {
	conv, restr [6]uint8
}

// identityColorMatrix is restoreColorReplacements: each type pays only
// itself, nothing restricted.
func identityColorMatrix() colorMatrix {
	var m colorMatrix
	for i := range m.conv {
		m.conv[i] = 1 << i
		m.restr[i] = matrixAllTypes
	}
	return m
}

// uses is getPossibleColorUses for the type of row.
func (m colorMatrix) uses(row int) uint8 {
	return m.conv[row] & m.restr[row]
}

// isIdentity reports a matrix no conversion changed.
func (m colorMatrix) isIdentity() bool {
	return m == identityColorMatrix()
}

// manaConversionStep is one pair of a ManaConversion$ value: the rows it
// touches, the types it adds (->) or keeps (<-).
type manaConversionStep struct {
	rows, types uint8
	additive    bool
}

// parseManaConversion is AbilityUtils.applyManaColorConversion's parse
// (AbilityUtils.java:2935-2957): space-separated pairs, "A->B" lets A pay as
// B, "A<-B" restricts A to B. ok is false for a word ManaAtom.fromConversion
// would read as no type at all.
func parseManaConversion(v string) ([]manaConversionStep, bool) {
	var out []manaConversionStep
	for _, pair := range strings.Fields(v) {
		additive := strings.Contains(pair, "->")
		sep := "<-"
		if additive {
			sep = "->"
		}
		from, to, found := strings.Cut(pair, sep)
		if !found {
			return nil, false
		}
		rows, ok := manaConversionTypes(from)
		if !ok {
			return nil, false
		}
		types, ok := manaConversionTypes(to)
		if !ok {
			return nil, false
		}
		out = append(out, manaConversionStep{rows: rows, types: types, additive: additive})
	}
	return out, len(out) > 0
}

// manaConversionTypes is ManaAtom.fromConversion (ManaAtom.java:62-81) as
// matrix bits: AnyColor, AnyType, non<type> (every other type), a color word
// or a run of type letters.
func manaConversionTypes(s string) (uint8, bool) {
	switch s {
	case "AnyColor":
		return matrixAllColors, true
	case "AnyType":
		return matrixAllTypes, true
	}
	if rest, ok := strings.CutPrefix(s, "non"); ok {
		b, ok := manaConversionTypes(rest)
		return b ^ matrixAllTypes, ok && b != 0
	}
	if strings.EqualFold(s, "colorless") {
		return matrixColorless, true
	}
	if len(s) > 2 {
		if c, ok := manaAtomColor(s); ok {
			return uint8(c), true
		}
	}
	var b uint8
	for _, r := range strings.ToUpper(s) {
		if r == 'C' {
			b |= matrixColorless
			continue
		}
		c, ok := mana.ColorFromLetter(byte(r))
		if !ok {
			return 0, false
		}
		b |= uint8(c)
	}
	return b, b != 0
}

// apply is adjustColorReplacement for every row step names
// (ManaConversionMatrix.java:34-43).
func (m *colorMatrix) apply(steps []manaConversionStep) {
	for _, st := range steps {
		for row := range m.conv {
			if st.rows&(1<<row) == 0 {
				continue
			}
			if st.additive {
				m.conv[row] |= st.types
			} else {
				m.restr[row] &= st.types
			}
		}
	}
}

// manaConvertSupported reports whether a static line is a ManaConvert this
// port reads for a spell's cost: only evaluated params and a conversion
// readManaConversion knows. A per-color line with Optional$ (no card has
// one) is not read: the matrix is applied as the mana is spent, where no
// question is asked. animate.go grants only such lines, so a grant is never
// inert.
func manaConvertSupported(s *compile.Ability) bool {
	switch readManaConversion(s) {
	case conversionNone:
		return false
	case conversionMatrix:
		if _, optional := s.Param("Optional"); optional {
			return false
		}
	}
	for _, p := range s.Params {
		if !manaConvertParams[strings.ToLower(p.Key)] {
			return false
		}
	}
	return true
}

// manaConvertForSpell is the ManaConvert statics' verdict on pid casting card:
// mana of any type pays the whole cost (anyType), or a colored part
// (anyColor). AnyType wins when both apply.
//
// controller is nil while the cost is only measured; when the cost is paid it
// answers an Optional$ line, which applies only on a yes.
func (g *Game) manaConvertForSpell(pid PlayerID, card CardID, controller PlayerController) (anyType, anyColor bool) {
	return g.manaConvertFor(pid, card, &Ability{Source: card, Controller: pid, spell: true}, controller)
}

// manaConvertForActivation is cost rewritten by the same verdict for an
// activated ability of card: Java applies ManaConvert to every cost paid through
// PlaySpellAbility.payManaCost (:644-658), so ValidSA$ Activated lines
// (Manascape Refractor, Agatha's Soul Cauldron) and lines naming no ValidSA$
// (Chromatic Orrery) reach it.
func (g *Game) manaConvertForActivation(pid PlayerID, card CardID, a *Ability, cost mana.Cost) mana.Cost {
	switch anyType, anyColor := g.manaConvertFor(pid, card, a, nil); {
	case anyType:
		return anyTypeCost(cost)
	case anyColor:
		return anyColorCost(cost)
	}
	return cost
}

// manaConvertFor is the shared walk: every live ManaConvert line that
// applies to sa, the spell or activated ability of card being paid.
func (g *Game) manaConvertFor(pid PlayerID, card CardID, spell *Ability, controller PlayerController) (anyType, anyColor bool) {
	c := g.Card(card)
	// Prismatic (CardFactoryUtil.java:3953-3955) is a ManaConvert static on the
	// card itself: EffectZone$ Stack, ValidPlayer$ You, ValidCard$ Card.Self,
	// ValidSA$ Spell, AnyType->AnyColor. The cards gain it perpetually through
	// Animate Keywords$, which grants no keyword statics here, so the keyword
	// is read directly.
	if spell.spell && c.HasKeyword("Prismatic") {
		anyColor = true
	}
	for _, host := range g.staticHostsWith(card) {
		h := g.Card(host)
		if h.Def == nil {
			continue
		}
		for _, face := range h.traitFaces() {
			for _, s := range face.Statics {
				if !strings.EqualFold(s.Name, "ManaConvert") || !manaConvertSupported(s) ||
					readManaConversion(s) == conversionMatrix || !g.manaConvertApplies(pid, c, h, s, spell) {
					continue
				}
				if _, optional := s.Param("Optional"); optional && controller != nil {
					// StaticAbilityManaConvert.java:41-47: the host forgets,
					// asks, and on a yes remembers the spell (North Star's
					// ForgetOnCast$ Card.IsRemembered then ends its effect).
					h.Memory.ClearRemembered()
					if !controller.ConfirmEffect(g, pid, host) {
						continue
					}
					h.Memory.Remember(CardEntity(card))
				}
				if readManaConversion(s) == conversionAnyType {
					anyType = true
				} else {
					anyColor = true
				}
			}
		}
	}
	// PlaySpellAbility.java:653-656: an activated ability a static granted
	// pays under that static's own ManaConversion$ (Drana and Linvala,
	// Scheming Fence; Grell Philosopher's per-color line is manaColorMatrix's).
	if grantor := grantorConversion(c, spell); grantor != nil {
		switch readManaConversion(grantor) {
		case conversionAnyType:
			anyType = true
		case conversionAnyColor:
			anyColor = true
		}
	}
	return anyType, anyColor
}

// grantorConversion is the static that granted a, an activated ability of c,
// when it carries ManaConversion$; nil otherwise.
func grantorConversion(c *Card, a *Ability) *compile.Ability {
	if a == nil || !a.activated || a.Params == nil {
		return nil
	}
	grantor := c.grantorStatic(a.Params)
	if grantor == nil {
		return nil
	}
	if _, ok := grantor.Param("ManaConversion"); !ok {
		return nil
	}
	return grantor
}

// manaColorMatrix is the per-color part of StaticAbilityManaConvert.manaConvert
// for a payment of pid's: the matrix every applying per-color line builds.
// sa is the spell or ability paid for, nil for an effect's payment
// (PlaySpellAbility.java:493-497 passes no SpellAbility), which a ValidSA$
// line does not reach; card is the paid ability's host.
func (g *Game) manaColorMatrix(pid PlayerID, card CardID, sa *Ability) colorMatrix {
	m := identityColorMatrix()
	c := g.Card(card)
	for _, host := range g.staticHostsWith(card) {
		h := g.Card(host)
		if h.Def == nil {
			continue
		}
		for _, face := range h.traitFaces() {
			for _, s := range face.Statics {
				if !strings.EqualFold(s.Name, "ManaConvert") || !manaConvertSupported(s) ||
					readManaConversion(s) != conversionMatrix {
					continue
				}
				if sa == nil {
					if _, has := s.Param("ValidSA"); has {
						continue
					}
				}
				probe := sa
				if probe == nil {
					probe = &Ability{Source: card, Controller: pid}
				}
				if !g.manaConvertApplies(pid, c, h, s, probe) {
					continue
				}
				v, _ := s.Param("ManaConversion")
				steps, _ := parseManaConversion(strings.TrimSpace(v))
				m.apply(steps)
			}
		}
	}
	if grantor := grantorConversion(c, sa); grantor != nil && readManaConversion(grantor) == conversionMatrix {
		v, _ := grantor.Param("ManaConversion")
		steps, _ := parseManaConversion(strings.TrimSpace(v))
		m.apply(steps)
	}
	return m
}

// manaConvertApplies is StaticAbilityManaConvert.checkManaConvert for a spell:
// ValidPlayer$, ValidCard$ and ValidSA$ each match when absent, and the line's
// Condition$ holds. A spec this port cannot recognize leaves the line unread.
func (g *Game) manaConvertApplies(pid PlayerID, c, host *Card, s *compile.Ability, spell *Ability) bool {
	// The card being cast is on the stack when its mana is paid, though Go-side
	// it is still in hand until the spell is put there; its own statics are read
	// from the Stack, every other host from where it is.
	// An activated ability's own host stays where it is.
	zone := host.Zone
	if host.ID == c.ID && spell.spell {
		zone = Stack
	}
	if !staticZoneOK(zone, s) || !g.staticOtherConditionsMet(host, s) {
		return false
	}
	if spec, ok := s.Param("ValidPlayer"); ok {
		matched, recognized := matchesPlayerSpec(g, pid, host.Controller(), host.ID, spec)
		if !recognized || !matched {
			return false
		}
	}
	if spec, ok := s.Param("ValidCard"); ok && !Matches(g, c, valid.Parse(spec), host.Controller(), host.ID) {
		return false
	}
	if spec, ok := s.Param("ValidSA"); ok {
		matched, recognized := g.spellAbilityMatches(spell, spec, host, host.Controller(), host.abilityAmounts(s))
		return recognized && matched
	}
	return true
}

// payMatrix is the matrix of the payment g.payFor describes: identity when
// none does or for a special action, sa nil for an unless-cost paid while its
// ability resolves.
func (g *Game) payMatrix(pid PlayerID) colorMatrix {
	ctx := g.payFor
	if ctx == nil || ctx.item == nil || ctx.special != "" {
		return identityColorMatrix()
	}
	sa := ctx.item
	if ctx.resolving {
		sa = nil
	}
	return g.manaColorMatrix(pid, ctx.item.Source, sa)
}

// convertShard is the shard a colored or colorless requirement s becomes
// under m, the mana pid spends for it. The pool types whose possible uses
// cover s pay it (ManaPool.canPayForShardWithColor): only s's own type
// leaves it as it is, one other type is that type's shard, several colors
// are a hybrid of them and a color plus colorless a colorless hybrid, each
// chosen the way the printed hybrid would be. ok is false when no type may
// pay s, or for colorless with several colors, a choice no PlayerController
// method asks (a pending error, GO-7).
func (g *Game) convertShard(m colorMatrix, s mana.Shard, pid PlayerID, controller PlayerController) (mana.Shard, bool) {
	var need uint8
	switch {
	case s == mana.ShardC:
		need = matrixColorless
	case s.Colors().Count() == 1:
		need = uint8(s.Colors())
	default:
		return s, true
	}
	var payers uint8
	for row := range m.conv {
		if m.uses(row)&need != 0 {
			payers |= 1 << row
		}
	}
	colors := mana.Colors(payers & matrixAllColors)
	switch {
	case payers == need:
		return s, true
	case payers == 0:
		return 0, false
	case payers == matrixColorless:
		return mana.ShardC, true
	case payers&matrixColorless == 0:
		if colors.Count() > 1 {
			colors = controller.ChooseHybridManaColor(g, pid, colors)
		}
		return mana.PureShard(colors)
	case colors.Count() == 1:
		if controller.ChoosePayColorlessHybrid(g, pid, colors) {
			return mana.PureShard(colors)
		}
		return mana.ShardC, true
	}
	g.recordPendingError(fmt.Errorf("engine: ManaConvert lets colorless and %d colors pay one shard", colors.Count()))
	return 0, false
}
