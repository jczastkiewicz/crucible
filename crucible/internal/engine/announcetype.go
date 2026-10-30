// AnnounceType$ (PlaySpellAbility.announceType, PlaySpellAbility.java:787-815):
// a spell's own announcement of a creature type, a number between Min$ and
// Max$, or an opponent, made as it is cast before X and targets, and kept on
// the host (setChosenType, setChosenNumber, setChosenPlayer) for the
// ChosenType target spec, Count$ChosenNumber and Defined$ ChosenPlayer to
// read. A copy of a spell is not announced again (it is never cast).

package engine

//enginelint:allow id card game control compile choosetypeeffect effecthelpers

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// announceTypeFor makes the announcements line's AnnounceType$ names and
// reports false when the caster cannot or does not (no option to choose from,
// a bad answer, a name this port does not read: a pending error, GO-7).
func (g *Game) announceTypeFor(pid PlayerID, c *Card, controller PlayerController, line *compile.Ability) bool {
	raw, ok := line.Param("AnnounceType")
	if !ok {
		return true
	}
	for _, name := range strings.Split(raw, ",") {
		switch strings.TrimSpace(name) {
		case "CreatureType":
			types, err := chooseTypeOptions(g, "Creature")
			if err != nil {
				g.recordPendingError(err)
				return false
			}
			if len(types) == 0 {
				return false
			}
			i := controller.ChooseOption(g, pid, c.ID, types)
			if i < 0 || i >= len(types) {
				g.recordPendingError(fmt.Errorf("engine: AnnounceType$ CreatureType: choice %d out of range", i))
				return false
			}
			c.Memory.SetChosenType(types[i], false)
		case "ChooseNumber":
			lo, errLo := strconv.Atoi(paramOr(line, "Min", ""))
			hi, errHi := strconv.Atoi(paramOr(line, "Max", ""))
			if errLo != nil || errHi != nil {
				g.recordPendingError(fmt.Errorf("engine: AnnounceType$ ChooseNumber: Min$ and Max$ must be numbers"))
				return false
			}
			c.Memory.SetChosenNumber(controller.ChooseNumber(g, pid, c.ID, lo, hi))
		case "Opponent":
			opponents := g.giftOpponents(pid)
			if len(opponents) == 0 {
				return false
			}
			chosen := controller.ChoosePlayerForEffect(g, pid, c.ID, opponents)
			if err := checkChoice([]PlayerID{chosen}, opponents, 1, 1); err != nil {
				g.recordPendingError(fmt.Errorf("engine: AnnounceType$ Opponent: %w", err))
				return false
			}
			c.Memory.SetChosenPlayer(chosen)
		default:
			g.recordPendingError(fmt.Errorf("engine: AnnounceType$ %q not resolvable yet", name))
			return false
		}
	}
	return true
}

// paramOr is line's param key, or def when absent.
func paramOr(line *compile.Ability, key, def string) string {
	if v, ok := line.Param(key); ok {
		return v
	}
	return def
}
