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

//enginelint:allow id card game player ability defined amount condition control zone valid

import (
	"fmt"
	"strconv"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// SubAbility$ no longer blocks: resolveSubAbility (subability.go) chains it
// through Registry.Resolve (effect.go) once this effect's own body
// finishes, whether or not subAbilityConditionMet let it run at all. 2 of
// the corpus's own 15 real SVar-defined Surveil lines naming SubAbility$
// chain to an already-built leaf ability and resolve end to end.
var surveilUnresolvedParams = [...]string{
	"ValidTgts", "TargetMin", "TargetMax", "Optional",
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
	players, err := definedPlayers(g, a.Controller, a.Source, defined, a.refs())
	if err != nil {
		return fmt.Errorf("engine: Surveil: %w", err)
	}

	for _, pid := range players {
		lib := g.Zone(Library, pid).Cards()
		// Player.surveil: num += StaticAbilitySurveilNum.surveilNumMod.
		n := min(num+g.surveilNumMod(controller, pid), len(lib))
		// An empty library still counts as a surveil: Player.surveil skips only
		// the arranging (Player.java:1049) and runs the trigger and the
		// per-turn count after it (:1083-1090).
		if n > 0 {
			topN := append([]CardID(nil), lib[:n]...)
			toTop, toGraveyard := controller.ArrangeForSurveil(g, pid, topN)
			_, rememberMoved := a.Params.Param("RememberMoved")
			_, rememberKept := a.Params.Param("RememberKept")
			for _, id := range toGraveyard {
				g.moveToGraveyard(controller, id)
				if rememberMoved {
					source.Memory.Remember(CardEntity(id))
				}
			}
			for i := len(toTop) - 1; i >= 0; i-- {
				g.MoveToLibraryTop(toTop[i], pid)
			}
			if rememberKept {
				for _, id := range toTop {
					source.Memory.Remember(CardEntity(id))
				}
			}
		}
		first := g.Player(pid).surveilThisTurn == 0
		g.checkSurveilTriggers(controller, pid, first)
		g.Player(pid).surveilThisTurn++
	}
	return nil
}

// surveilNumParams are the params a Mode$ SurveilNum line may carry that
// surveilNumMod reads.
var surveilNumParams = map[string]bool{"num": true, "validplayer": true, "optional": true}

// surveilNumMod is StaticAbilitySurveilNum.surveilNumMod: the sum of the Num$
// of every live Mode$ SurveilNum line whose ValidPlayer$ admits pid (Enhanced
// Surveillance), an Optional$ one only when pid accepts it. Java asks
// confirmStaticApplication; ConfirmEffect stands for it here.
func (g *Game) surveilNumMod(controller PlayerController, pid PlayerID) int {
	mod := 0
	g.anyStatic("SurveilNum", surveilNumParams, func(h *Card, s *compile.Ability) bool {
		if spec, ok := s.Param("ValidPlayer"); ok {
			if matched, known := matchesPlayerSpec(g, pid, h.Controller(), h.ID, spec); !known || !matched {
				return false
			}
		}
		raw, _ := s.Param("Num")
		n, err := strconv.Atoi(raw)
		if err != nil {
			return false
		}
		if _, optional := s.Param("Optional"); optional && !controller.ConfirmEffect(g, pid, h.ID) {
			return false
		}
		mod += n
		return false
	})
	return mod
}
