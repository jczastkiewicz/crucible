package engine

//enginelint:allow id card game ability defined condition control effecthelpers token counters zone parts event

import (
	"fmt"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
)

// empowerEffect is EmpowerEffect.java: the first target or Defined$ player
// (default You) creates a "<Type$> Token" planeswalker from the u_empower
// token script unless they already control a token of Type$, then puts Num$
// (default 1) loyalty counters on one token of Type$ they control -- their
// choice when there are several.
//
// Counter placement is a direct Counters.Add, the same as putCounterEffect:
// GameEntityCounterTable.replaceCounterEffect's own counter replacement
// pass is not ported (putcountereffect.go's own doc comment).
//
// Ported from forge-game/src/main/java/forge/game/ability/effects/EmpowerEffect.java's resolve.
type empowerEffect struct{}

func (empowerEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	players, err := targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: Empower: %w", err)
	}
	if len(players) == 0 {
		return nil
	}
	p := players[0]
	amount, err := optionalAmount(g, a, "Empower", "Num", 1)
	if err != nil {
		return err
	}
	typ, ok := a.Params.Param("Type")
	if !ok {
		return fmt.Errorf("engine: Empower: Type$ missing")
	}

	if len(empowerTokens(g, p, typ)) == 0 {
		def, err := empowerTokenDef(g, typ)
		if err != nil {
			return err
		}
		id := g.createToken(controller, tokenSpec{Def: def, Owner: p})
		g.checkChangesZoneAllTriggers(controller, []CardID{id}, None, Battlefield)
	}

	tokens := empowerTokens(g, p, typ)
	if len(tokens) == 0 {
		return nil
	}
	picked := controller.ChooseCardsForEffect(g, p, a.Source, tokens, 1, 1)
	if err := checkChoice(picked, tokens, 1, 1); err != nil {
		return fmt.Errorf("engine: Empower: %w", err)
	}
	g.Card(picked[0]).Counters.Add(Loyalty, amount)
	emitCounterChanged(g.sink, a.Source, CardEntity(picked[0]), Loyalty, amount)
	return nil
}

// empowerTokens is every token of type typ on p's battlefield, in zone
// order -- Java's CardPredicates.isType(type).and(TOKEN) filter.
func empowerTokens(g *Game, p PlayerID, typ string) []CardID {
	var out []CardID
	for _, id := range g.Zone(Battlefield, p).Cards() {
		c := g.Card(id)
		if c.IsToken && c.Type().HasStringType(typ) {
			out = append(out, id)
		}
	}
	return out
}

// empowerTokenDef is the prototype EmpowerEffect.java builds: token script
// u_empower_<type> when the DB holds one, else the generic u_empower (the
// corpus ships only the generic one), with Type$ added to its type line and
// the name "<Type$> Token". The script's own compiled Card is shared,
// immutable, so the prototype is a copy.
func empowerTokenDef(g *Game, typ string) (*compile.Card, error) {
	base, ok := g.db.Token("u_empower_" + strings.ToLower(typ))
	if !ok {
		var err error
		if base, err = tokenScript(g, "u_empower"); err != nil {
			return nil, err
		}
	}
	def := *base
	def.Name = typ + " Token"
	def.Faces[0].Name = def.Name
	def.Faces[0].Type = def.Faces[0].Type.Union(cardtype.ParseToken(typ))
	return &def, nil
}
