// Mana planning: which untapped permanents a player can tap for mana, and
// which of them pay a given cost. It is the part of Java's ComputerUtilMana
// that decides payability (R22a) and the engine side of the castability probe
// (MET-5, MET-6): it reads the board and changes nothing.
//
// A source here is a permanent with a mana ability that costs only {T}
// (Produced$ one colour, Any, or Combo <colours>, one mana each) or a land's
// intrinsic basic-land ability (CR 305.6). Mana abilities with other costs or
// amounts, and mana already in the pool, are not planned (a plan may be
// conservative, never wrong: it only promises mana the sources make).

package engine

//enginelint:allow ability control card game player id zone activatemanaability manaability cantbecast activateability castspell parts

import (
	"sort"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// ManaSource is one way a permanent makes one mana.
type ManaSource struct {
	Card CardID
	// Ability is the index of the mana ability (Card.abilityAt), or -1 for a
	// land's intrinsic basic-land ability.
	Ability int
	// Colors are the colours one activation can make; Colorless adds {C}.
	Colors    mana.Colors
	Colorless bool
}

// ManaSources lists pid's untapped permanents that can be tapped for one
// mana now, in battlefield order.
func (g *Game) ManaSources(pid PlayerID) []ManaSource {
	var out []ManaSource
	for _, id := range g.Zone(Battlefield, pid).Cards() {
		c := g.Card(id)
		if c.Tapped || c.IsPhasedOut() || c.isDetained() || c.Controller() != pid {
			continue
		}
		var land mana.Colors
		for color, sub := range basicLandType {
			if c.Type().HasSubtype(sub) {
				land |= color
			}
		}
		if land != 0 {
			out = append(out, ManaSource{Card: id, Ability: -1, Colors: land})
		}
		sick := c.SummonSick && c.Type().Has(cardtype.Creature) && !c.HasKeyword("Haste")
		for i := 0; i < 64; i++ {
			ab, _, ok := c.abilityAt(i)
			if !ok || ab.Name != "Mana" || !g.manaAbilityParamsResolvable(ab) {
				continue
			}
			if cost, _ := ab.Param("Cost"); strings.TrimSpace(cost) != "T" || sick {
				continue
			}
			if _, has := ab.Param("Amount"); has {
				continue
			}
			if _, has := ab.Param("SubAbility"); has {
				continue
			}
			if !g.timingRestrictionsMet(pid, ab) || g.cantBeActivated(pid, c, true, false) {
				continue
			}
			produced, _ := ab.Param("Produced")
			src := ManaSource{Card: id, Ability: i}
			switch {
			case produced == "Any":
				src.Colors = mana.AllColors
			case strings.HasPrefix(produced, "Combo "):
				options, ok := parseComboColors(produced)
				if !ok {
					continue
				}
				src.Colors = options
			default:
				color, colorless, ok := producedManaColor(produced)
				if !ok {
					continue
				}
				src.Colors, src.Colorless = color, colorless
			}
			out = append(out, src)
		}
	}
	return out
}

// PlannedTap is one source tapped for one colour (or for colourless).
type PlannedTap struct {
	Source    ManaSource
	Color     mana.Colors
	Colorless bool
}

// ManaPlan is a way to pay a cost from the sources: the taps, and the life a
// Phyrexian symbol is paid with.
type ManaPlan struct {
	Taps []PlannedTap
	Life int
}

// PlanMana reports whether pid can pay cost from their untapped sources and
// how. X shards count as zero. Snow and "colourless-only" shards are paid as
// the plan's sources allow: {C} needs a source that makes {C}, {S} any source.
func (g *Game) PlanMana(pid PlayerID, cost mana.Cost) (ManaPlan, bool) {
	return planMana(g.ManaSources(pid), cost, false)
}

// PlanManaAnyColor is PlanMana with every source making every colour: the
// hypothetical pool of the same size MET-6 solves against, to tell a card
// blocked on colour from one blocked on quantity.
func (g *Game) PlanManaAnyColor(pid PlayerID, cost mana.Cost) (ManaPlan, bool) {
	return planMana(g.ManaSources(pid), cost, true)
}

// manaNeed is one coloured requirement of a cost.
type manaNeed struct {
	colors    mana.Colors
	colorless bool
	// orGeneric2 marks {2/C}: paid with the colour, or with two generic.
	orGeneric2 bool
	// life marks a Phyrexian symbol: paid with the colour, or 2 life.
	life bool
}

func planMana(sources []ManaSource, cost mana.Cost, anyColor bool) (ManaPlan, bool) {
	if cost.IsNoCost() {
		return ManaPlan{}, false
	}
	var needs []manaNeed
	generic := cost.Generic()
	for _, sh := range cost.Shards() {
		switch {
		case sh.IsX():
		case sh.IsGeneric():
			generic++
		case sh.IsSnow():
			generic++
		case sh.IsColorless():
			needs = append(needs, manaNeed{colorless: true})
		default:
			needs = append(needs, manaNeed{colors: sh.Colors(), orGeneric2: sh.IsOr2Generic(), life: sh.IsPhyrexian()})
		}
	}
	makes := func(s ManaSource, n manaNeed) bool {
		if n.colorless {
			return s.Colorless
		}
		return anyColor || s.Colors&n.colors != 0
	}
	// Convert needs that cannot be matched into their fallback until the
	// matching and the generic count both work.
	skipped := make([]bool, len(needs))
	for round := 0; round <= len(needs); round++ {
		matchOf := make([]int, len(sources)) // source -> manaNeed, -1 free
		for i := range matchOf {
			matchOf[i] = -1
		}
		var try func(ni int, seen []bool) bool
		try = func(ni int, seen []bool) bool {
			for si, s := range sources {
				if seen[si] || !makes(s, needs[ni]) {
					continue
				}
				seen[si] = true
				if matchOf[si] == -1 || try(matchOf[si], seen) {
					matchOf[si] = ni
					return true
				}
			}
			return false
		}
		failed := -1
		for ni := range needs {
			if skipped[ni] {
				continue
			}
			if !try(ni, make([]bool, len(sources))) {
				failed = ni
				break
			}
		}
		if failed >= 0 {
			if !needs[failed].orGeneric2 && !needs[failed].life {
				return ManaPlan{}, false
			}
			skipped[failed] = true
			continue
		}
		extraGeneric, life := generic, 0
		for ni, n := range needs {
			if skipped[ni] {
				if n.life {
					life += 2
				} else {
					extraGeneric += 2
				}
			}
		}
		var plan ManaPlan
		plan.Life = life
		used := make([]bool, len(sources))
		for si, ni := range matchOf {
			if ni < 0 {
				continue
			}
			used[si] = true
			s, n := sources[si], needs[ni]
			t := PlannedTap{Source: s}
			switch {
			case n.colorless:
				t.Colorless = true
			case anyColor:
				t.Color = n.colors & mana.AllColors
				t.Color = lowestColor(t.Color)
			default:
				t.Color = lowestColor(s.Colors & n.colors)
			}
			plan.Taps = append(plan.Taps, t)
		}
		// Generic: the least flexible free sources first, so flexible ones stay
		// for the next spell.
		var free []int
		for si := range sources {
			if !used[si] {
				free = append(free, si)
			}
		}
		sort.SliceStable(free, func(a, b int) bool {
			return flexibility(sources[free[a]]) < flexibility(sources[free[b]])
		})
		if len(free) < extraGeneric {
			// A {2/C} paid with two generic found too few sources: pay the
			// colour instead if a later round can, else the cost is out of reach.
			ok := false
			for ni, n := range needs {
				if skipped[ni] && n.orGeneric2 {
					skipped[ni] = false
					ok = true
					break
				}
			}
			if ok {
				continue
			}
			return ManaPlan{}, false
		}
		for _, si := range free[:extraGeneric] {
			s := sources[si]
			t := PlannedTap{Source: s, Color: lowestColor(s.Colors)}
			if s.Colors == 0 {
				t.Colorless = true
			}
			plan.Taps = append(plan.Taps, t)
		}
		return plan, true
	}
	return ManaPlan{}, false
}

func flexibility(s ManaSource) int {
	n := s.Colors.Count()
	if s.Colorless {
		n++
	}
	return n
}

// lowestColor is the first colour bit of cs in WUBRG order, or cs itself when
// it is empty.
func lowestColor(cs mana.Colors) mana.Colors {
	for _, c := range [...]mana.Colors{mana.White, mana.Blue, mana.Black, mana.Red, mana.Green} {
		if cs&c != 0 {
			return c
		}
	}
	return cs
}

// planColor answers a mana ability's colour choice with the colour the plan
// picked for it.
type planColor struct {
	PlayerController
	color mana.Colors
}

func (p planColor) ChooseManaColor(g *Game, decider PlayerID, source CardID, options mana.Colors) mana.Colors {
	if options.Has(p.color) {
		return p.color
	}
	return p.PlayerController.ChooseManaColor(g, decider, source, options)
}

// TapForPlan taps every source of plan, so the mana is in pid's pool. It
// reports whether every tap happened; a failed tap leaves the earlier ones
// done (the mana stays in the pool).
func (g *Game) TapForPlan(pid PlayerID, plan ManaPlan, controller PlayerController) bool {
	for _, t := range plan.Taps {
		if t.Source.Ability < 0 {
			if !g.TapLandForMana(pid, t.Source.Card, t.Color, controller) {
				return false
			}
			continue
		}
		if !g.ActivateManaAbility(pid, t.Source.Card, t.Source.Ability, planColor{controller, t.Color}) {
			return false
		}
	}
	return true
}
