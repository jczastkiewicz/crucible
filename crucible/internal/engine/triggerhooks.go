// Trigger modes whose effect or cost this port already resolves, fired from
// that effect: Clashed, Proliferate, PayLife, Foretell, SearchedLibrary and
// ManifestDread. Each is a player event with a few params tested against it,
// fired through scanTriggers (triggerscan.go).

package engine

//enginelint:allow id card ability trigger triggerscan triggerpack staticability game control valid

import "github.com/jczastkiewicz/crucible/internal/carddb/compile"

var (
	clashedParams         = effectEventParams("validplayer", "won")
	proliferateParams     = effectEventParams("validplayer")
	payLifeParams         = effectEventParams("validplayer")
	foretellParams        = effectEventParams("validplayer")
	searchedLibraryParams = effectEventParams("validplayer", "searchownlibrary")
	manifestDreadParams   = effectEventParams("validplayer")
)

// checkClashedTriggers is Mode$ Clashed (TriggerClashed.java:49-62, 6 real
// lines), run for the clashing player, then for the opponent
// (ClashEffect.java:55-60), after the winner's sub-ability resolved.
// ValidPlayer$ against the clasher; Won$ compared as a string with "True" or
// "False", so a tie (no winner) is False for both. TriggerClashed exposes no
// triggering object.
func (g *Game) checkClashedTriggers(controller PlayerController, pid PlayerID, won bool) {
	result := "False"
	if won {
		result = "True"
	}
	matches := g.scanTriggers([]string{"Clashed"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, clashedParams) || !g.triggerPlayerMatches(h, t, "ValidPlayer", pid) {
			return triggeredObjects{}, false
		}
		if v, ok := t.Param("Won"); ok && v != result {
			return triggeredObjects{}, false
		}
		return triggeredObjects{}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}

// checkProliferateTriggers is Mode$ Proliferate (TriggerProliferate.java:14-22,
// 7 real lines), run once per proliferate, chosen set empty or not
// (CountersProliferateEffect.java:74). The player is the activator, not a
// counter's owner (PORT-7), and is TriggeredPlayer.
func (g *Game) checkProliferateTriggers(controller PlayerController, pid PlayerID) {
	g.pushTriggeredAbilities(controller, g.playerEventMatches("Proliferate", proliferateParams, pid))
}

// checkPayLifeTriggers is Mode$ PayLife (TriggerPayLife.java, 1 real line),
// run by Player.payLife (Player.java:600), whose one caller is CostPayLife:
// life paid as a cost, never life lost to an effect. amount is the life the
// cost named, TriggeredLifeAmount.
func (g *Game) checkPayLifeTriggers(controller PlayerController, pid PlayerID, amount int) {
	matches := g.scanTriggers([]string{"PayLife"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, payLifeParams) || !g.triggerPlayerMatches(h, t, "ValidPlayer", pid) {
			return triggeredObjects{}, false
		}
		return triggeredObjects{player: pid, counts: triggerCounts{life: amount, set: countLife}}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}

// checkForetellTriggers is Mode$ Foretell (TriggerForetell.java, 1 real line),
// run by Player.addForetoldThisTurn (Player.java:1494-1499) after the card was
// exiled foretold. OnlyFirst$ is in no corpus line, so a line carrying it is
// not fired (GO-7).
func (g *Game) checkForetellTriggers(controller PlayerController, pid PlayerID) {
	g.pushTriggeredAbilities(controller, g.playerEventMatches("Foretell", foretellParams, pid))
}

// checkSearchedLibraryTriggers is Mode$ SearchedLibrary
// (TriggerSearchedLibrary.java:57-74, 4 real lines), run once per searched
// library, found anything or not (ChangeZoneEffect.java:1078-1081):
// ValidPlayer$ against the searcher; SearchOwnLibrary$ needs the searched
// library to be the searcher's own. Only the searcher is TriggeredPlayer.
func (g *Game) checkSearchedLibraryTriggers(controller PlayerController, searcher, owner PlayerID) {
	matches := g.scanTriggers([]string{"SearchedLibrary"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, searchedLibraryParams) || !g.triggerPlayerMatches(h, t, "ValidPlayer", searcher) {
			return triggeredObjects{}, false
		}
		if _, ok := t.Param("SearchOwnLibrary"); ok && owner != searcher {
			return triggeredObjects{}, false
		}
		return triggeredObjects{player: searcher}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}

// checkManifestDreadTriggers is Mode$ ManifestDread
// (TriggerManifestDread.java:13-22, 1 real line), run once per manifest dread,
// with an empty library too (ManifestDreadEffect.java:36-39, PORT-7).
// ValidPlayer$ against the player; the cards put into the graveyard are
// TriggeredCards, the only object Java exposes.
func (g *Game) checkManifestDreadTriggers(controller PlayerController, pid PlayerID, toGrave []CardID) {
	matches := g.scanTriggers([]string{"ManifestDread"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, manifestDreadParams) || !g.triggerPlayerMatches(h, t, "ValidPlayer", pid) {
			return triggeredObjects{}, false
		}
		return triggeredObjects{cards: append([]CardID(nil), toGrave...)}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}
