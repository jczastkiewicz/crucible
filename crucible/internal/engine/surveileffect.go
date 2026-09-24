// Surveil: CR 701.42, SurveilEffect.java + Player.surveil -- ArrangeForScry's
// own sibling decision, reusing scryEffect's own shape (scryeffect.go) for a
// second reordering effect: look at the top Amount$ cards, put some back on
// top in a chosen order, and the rest into the graveyard rather than the
// bottom of the library. 187 of the corpus's 208 real (AB|DB)$ Surveil lines
// that also name Defined$ You/Opponent/Player/Player.Opponent or no Defined$
// at all -- the identical "absent Defined$ means You" default scryEffect's
// own doc comment already covers -- and carry no other unresolved param,
// resolve; 4 of them name Planeswalker$ too, no longer blocked (CR 606.3's
// own loyalty-ability restriction is a cost-side gate, activateability.go,
// never a restriction on how the effect it pays for resolves).
//
// Ported from
// forge-game/src/main/java/forge/game/ability/effects/SurveilEffect.java's
// resolve and forge-game/src/main/java/forge/game/player/Player.java's own
// surveil. CR 702's own Surveil-number static modifier
// (StaticAbilitySurveilNum.surveilNumMod) is not ported -- 0 real lines
// carry the qualifying keyword -- and, the identical reason scryEffect's own
// doc comment gives for Mode$ Scry, the corpus's own T:Mode$ Surveil is 0
// real lines too, so nothing here checks a trigger at all.

package engine

import (
	"fmt"
)

// SubAbility$ no longer blocks: resolveSubAbility (subability.go) chains it
// through Registry.Resolve (effect.go) once this effect's own body
// finishes, whether or not subAbilityConditionMet let it run at all. 2 of
// the corpus's own 15 real SVar-defined Surveil lines naming SubAbility$
// chain to an already-built leaf ability and resolve end to end.
var surveilUnresolvedParams = [...]string{
	"ValidTgts", "TargetMin", "TargetMax", "Optional",
	"RememberMoved", "RememberKept",
}

type surveilEffect struct{}

func (surveilEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	for _, key := range surveilUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: Surveil: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}

	amount, ok := a.Params.Param("Amount")
	if !ok {
		amount = "1"
	}
	num, ok := resolveNamedAmount(g, a.Amounts, source, amount)
	if !ok {
		return fmt.Errorf("engine: Surveil: Amount$ %q is not resolvable", amount)
	}
	if num <= 0 {
		return nil
	}

	defined, ok := a.Params.Param("Defined")
	if !ok {
		defined = "You"
	}
	players, err := definedPlayers(g, a.Controller, a.Source, defined, a.Targets)
	if err != nil {
		return fmt.Errorf("engine: Surveil: %w", err)
	}

	for _, pid := range players {
		lib := g.Zone(Library, pid).Cards()
		n := num
		if n > len(lib) {
			n = len(lib)
		}
		if n == 0 {
			continue
		}
		topN := append([]CardID(nil), lib[:n]...)

		toTop, toGraveyard := controller.ArrangeForSurveil(g, pid, topN)
		for _, id := range toGraveyard {
			g.Move(id, Graveyard, g.Card(id).Owner)
		}
		for i := len(toTop) - 1; i >= 0; i-- {
			g.MoveToLibraryTop(toTop[i], pid)
		}
	}
	return nil
}
