// Trigger modes whose effect or cost this port already resolves, fired from
// that effect: Clashed, Proliferate, PayLife, Foretell, SearchedLibrary,
// ManifestDread, GiveGift, Vote, BecomesTargetOnce, TapAll, AbilityTriggered
// and AbilityResolves. Each is a player event with a few params tested against it,
// fired through scanTriggers (triggerscan.go).

package engine

//enginelint:allow id card ability trigger triggerscan triggerpack staticability game control valid

import (
	"slices"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/keyword"
)

var (
	clashedParams           = effectEventParams("validplayer", "won")
	proliferateParams       = effectEventParams("validplayer")
	payLifeParams           = effectEventParams("validplayer")
	foretellParams          = effectEventParams("validplayer")
	searchedLibraryParams   = effectEventParams("validplayer", "searchownlibrary")
	manifestDreadParams     = effectEventParams("validplayer")
	giveGiftParams          = effectEventParams("validplayer")
	voteParams              = effectEventParams("list")
	becomesTargetOnceParams = effectEventParams("validtarget", "validsource", "validcause")
	tapAllParams            = effectEventParams("validcards")
	abilityTriggeredParams  = effectEventParams("validmode", "validdestination", "validspellability", "validsource", "validcause", "triggeredownability")
	abilityResolvesParams   = effectEventParams("validspellability", "validsource")
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

// checkGiveGiftTriggers is Mode$ GiveGift (TriggerGiveGift.java:54-66, 1 real
// line), run as an ability carrying the Gift keyword starts to resolve, before
// its conditions and its unless cost (AbilityUtils.resolveApiAbility,
// AbilityUtils.java:1390-1391). That ability is an instant's or sorcery's
// GiftAbility, resolved only when the gift was promised, or a permanent's
// gift trigger, whose enters condition needs the promise
// (compile.expandGift). ValidPlayer$ against the resolving ability's
// controller, TriggeredPlayer.
func (g *Game) checkGiveGiftTriggers(controller PlayerController, a *Ability) {
	if keyword.Parse(a.Params.Keyword).Name != "Gift" {
		return
	}
	g.pushTriggeredAbilities(controller, g.playerEventMatches("GiveGift", giveGiftParams, a.Controller))
}

// checkVoteTriggers is Mode$ Vote (TriggerVote.java:58-121, 3 real lines),
// run once the players finished voting, before any option's ability resolves
// (VoteEffect.java:126-128). performTest is always true; List$ is stack text
// only. votes is each option's voters, one entry per ballot, in option order
// (Java's multimap order is arbitrary). For each host, the voters of every
// option its controller did not vote for, then of every option it did, each
// narrowed to the controller's opponents still in the game, are
// TriggeredOpponentVotedDiff and TriggeredOpponentVotedSame.
func (g *Game) checkVoteTriggers(controller PlayerController, votes [][]PlayerID) {
	matches := g.scanTriggers([]string{"Vote"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, voteParams) {
			return triggeredObjects{}, false
		}
		ctrl := h.Controller()
		return triggeredObjects{
			opponentVotedDiff: g.opponentVoters(ctrl, votes, true),
			opponentVotedSame: g.opponentVoters(ctrl, votes, false),
			votesRecorded:     true,
		}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}

// opponentVoters is TriggerVote.getVoters with isOpponent: the voters of each
// option ctrl voted for (other false) or did not (other true), deduplicated in
// first-seen order, then only ctrl's opponents still in the game.
func (g *Game) opponentVoters(ctrl PlayerID, votes [][]PlayerID, other bool) []PlayerID {
	var out []PlayerID
	for _, voters := range votes {
		if other == slices.Contains(voters, ctrl) {
			continue
		}
		for _, v := range voters {
			if v != ctrl && !g.Player(v).Lost && !slices.Contains(out, v) {
				out = append(out, v)
			}
		}
	}
	return out
}

// becomesTargetOnceMatches is Mode$ BecomesTargetOnce
// (TriggerBecomesTargetOnce.java:56-80, 4 real lines), run once per spell or
// ability put on the stack with a target, after its BecomesTarget triggers
// (MagicStack.java:474-486). targets is every distinct object it targets.
// ValidTarget$ matches when any of them does, ValidSource$ is the targeting
// ability's kind and controller (becomesTargetSourceMatches), ValidCause$ the
// card it comes from. Random$ (the targets were chosen at random, Chira) is
// not modelled, so its line is not fired (GO-7). The cause card is
// TriggeredSource, the ability's controller TriggeredSourceSAController and
// the targets TriggeredTargets.
func (g *Game) becomesTargetOnceMatches(targets []EntityID, isSpellSource bool, sourceController PlayerID, cause CardID) []Ability {
	if len(targets) == 0 {
		return nil
	}
	return g.scanTriggers([]string{"BecomesTargetOnce"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, becomesTargetOnceParams) {
			return triggeredObjects{}, false
		}
		if spec, ok := t.Param("ValidTarget"); ok && !attackedTargetMatches(g, h, targets, spec) {
			return triggeredObjects{}, false
		}
		if spec, ok := t.Param("ValidSource"); ok && !becomesTargetSourceMatches(isSpellSource, sourceController, h.Controller(), spec) {
			return triggeredObjects{}, false
		}
		if _, ok := t.Param("ValidCause"); ok && (cause == NoCard || !g.triggerCardMatches(h, t, "ValidCause", g.Card(cause))) {
			return triggeredObjects{}, false
		}
		o := triggeredObjects{sourceSAController: sourceController, damagedTargets: append([]EntityID(nil), targets...), damagedTargetsRecorded: true}
		if cause != NoCard {
			o.source, o.sourceController = CardEntity(cause), g.Card(cause).Controller()
		}
		return o, true
	})
}

// checkTapAllTriggers is Mode$ TapAll (TriggerTapAll.java, 2 real lines), run
// once per batch of permanents one action tapped, after their Taps triggers:
// the attackers of a declaration (PhaseHandler.java:614), a {T} cost
// (CostTap.java:78), a tapXType or convoke payment (CostTapType.java:214,
// CostAdjustment.java:376), and the Tap, TapAll, TapOrUntap, TapOrUntapAll
// and Regenerate effects. ValidCards$ matches when any tapped card does; the
// matching cards are TriggeredCards.
func (g *Game) checkTapAllTriggers(controller PlayerController, tapped []CardID) {
	if len(tapped) == 0 {
		return
	}
	matches := g.scanTriggers([]string{"TapAll"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, tapAllParams) {
			return triggeredObjects{}, false
		}
		kept := tapped
		if _, ok := t.Param("ValidCards"); ok {
			kept = nil
			for _, id := range tapped {
				if g.triggerCardMatches(h, t, "ValidCards", g.Card(id)) {
					kept = append(kept, id)
				}
			}
			if len(kept) == 0 {
				return triggeredObjects{}, false
			}
		}
		return triggeredObjects{cards: append([]CardID(nil), kept...)}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}

// abilityTriggeredMatches is Mode$ AbilityTriggered
// (TriggerAbilityTriggered.java:48-130, 4 real lines) for pushed, a
// triggered ability just put on the stack (TriggerHandler.java:545).
// ValidMode$ against the mode of the trigger it came from (a site that did
// not record its trigger matches no mode); ValidDestination$ against the zone
// a ChangesZone or ChangesZoneAll trigger's card(s) moved to (Java splits a
// missing one and throws, so here a trigger without one does not fire);
// ValidSpellAbility$ against pushed, ValidSource$ against its host,
// ValidCause$ against any of the cards that caused it; TriggeredOwnAbility$
// needs the host among them. The causes are getRunParams': the moved card
// (ChangesZone), the batch (ChangesZoneAll), the attacker (Attacks), the
// attackers its ValidAttackers$ keeps (AttackersDeclared). pushed is
// TriggeredSpellAbility, its host TriggeredSource.
func (g *Game) abilityTriggeredMatches(pushed Ability) []Ability {
	mode := ""
	if pushed.trigger != nil {
		mode = pushed.trigger.Name
	}
	return g.scanTriggers([]string{"AbilityTriggered"}, func(h *Card, face triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, abilityTriggeredParams) {
			return triggeredObjects{}, false
		}
		if v, ok := t.Param("ValidMode"); ok && !slices.Contains(strings.Split(v, ","), mode) {
			return triggeredObjects{}, false
		}
		if v, ok := t.Param("ValidDestination"); ok {
			dest := pushed.triggered.destination
			if dest == "" || !slices.ContainsFunc(strings.Split(dest, ","), func(d string) bool { return slices.Contains(strings.Split(v, ","), d) }) {
				return triggeredObjects{}, false
			}
		}
		if spec, ok := t.Param("ValidSpellAbility"); ok {
			if matched, known := g.spellAbilityMatches(&pushed, spec, h, h.Controller(), face.Amounts); !known || !matched {
				return triggeredObjects{}, false
			}
		}
		if !g.triggerCardMatches(h, t, "ValidSource", g.Card(pushed.Source)) {
			return triggeredObjects{}, false
		}
		causes := g.abilityTriggeredCauses(pushed, mode)
		if _, ok := t.Param("ValidCause"); ok && !slices.ContainsFunc(causes, func(id CardID) bool { return g.triggerCardMatches(h, t, "ValidCause", g.Card(id)) }) {
			return triggeredObjects{}, false
		}
		if _, ok := t.Param("TriggeredOwnAbility"); ok && !slices.Contains(causes, pushed.Source) {
			return triggeredObjects{}, false
		}
		return triggeredObjects{spellAbility: pushed.ID, saHost: pushed.Source,
			source: CardEntity(pushed.Source), sourceController: g.Card(pushed.Source).Controller()}, true
	})
}

// abilityTriggeredCauses is TriggerAbilityTriggered.getRunParams' Cause for a
// trigger of mode: nil for a mode it records none for.
func (g *Game) abilityTriggeredCauses(pushed Ability, mode string) []CardID {
	o := pushed.triggered
	switch mode {
	case "ChangesZone":
		if o.card != NoCard {
			return []CardID{o.card}
		}
	case "ChangesZoneAll":
		return o.zoneCause
	case "Attacks":
		if o.attacker != NoCard {
			return []CardID{o.attacker}
		}
	case "AttackersDeclared", "AttackersDeclaredOneTarget":
		if pushed.trigger == nil {
			return nil
		}
		if _, ok := pushed.trigger.Param("ValidAttackers"); !ok {
			return o.attackers
		}
		h := g.Card(pushed.Source)
		var kept []CardID
		for _, id := range o.attackers {
			if g.triggerCardMatches(h, pushed.trigger, "ValidAttackers", g.Card(id)) {
				kept = append(kept, id)
			}
		}
		return kept
	}
	return nil
}

// checkAbilityResolvesTriggers is Mode$ AbilityResolves
// (TriggerAbilityResolves.java:33-57, 2 real lines), run once a spell or
// ability resolved, before a spell moves off the stack (MagicStack.java:
// 712-713). ValidSpellAbility$ against it, ValidSource$ against its host,
// which is TriggeredSource; the ability is TriggeredSpellAbility.
func (g *Game) checkAbilityResolvesTriggers(controller PlayerController, a *Ability) {
	if a.Source == NoCard || int(a.Source) >= len(g.cards) || g.over {
		return
	}
	src := g.Card(a.Source)
	matches := g.scanTriggers([]string{"AbilityResolves"}, func(h *Card, face triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, abilityResolvesParams) {
			return triggeredObjects{}, false
		}
		if spec, ok := t.Param("ValidSpellAbility"); ok {
			if matched, known := g.spellAbilityMatches(a, spec, h, h.Controller(), face.Amounts); !known || !matched {
				return triggeredObjects{}, false
			}
		}
		if !g.triggerCardMatches(h, t, "ValidSource", src) {
			return triggeredObjects{}, false
		}
		return triggeredObjects{saHost: a.Source, source: CardEntity(a.Source), sourceController: src.Controller()}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}
