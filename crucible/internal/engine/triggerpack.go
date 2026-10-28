// Trigger modes fired by effects this port already has: Scry, Surveil,
// Transformed and TurnFaceUp, each a few params tested against the event,
// fired through scanTriggers (triggerscan.go).

package engine

//enginelint:allow id card ability trigger staticability triggerscan game control valid stack

import (
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// effectEventTriggerParams are the general params a trigger of these four
// modes may carry besides its own; the intervening-if and limit params are
// read by triggerEffectAPI, so naming them needs no check here. Anything else
// (Scry's ToBottom$, Surveil's FirstTime$, TurnFaceUp's ValidCause$) is a
// check this port has no evidence for, so the line is not fired (GO-7).
var effectEventTriggerParams = map[string]bool{
	"mode": true, "execute": true, "triggerdescription": true, "triggerzones": true, "optionaldecider": true,
	"secondary": true, "activationlimit": true, "resolvedlimit": true, "gameactivationlimit": true,
	"ispresent": true, "presentdefined": true, "presentcompare": true, "presentzone": true, "presentplayer": true,
	"playerturn": true,
}

func effectEventParams(own ...string) map[string]bool {
	m := make(map[string]bool, len(effectEventTriggerParams)+len(own))
	for k, v := range effectEventTriggerParams {
		m[k] = v
	}
	for _, k := range own {
		m[k] = true
	}
	return m
}

var (
	scryTriggerParams     = effectEventParams("validplayer", "tobottom")
	surveilTriggerParams  = effectEventParams("validplayer", "firsttime")
	flippedCoinParams     = effectEventParams("validplayer", "validresult")
	flippedCoinOnceParams = effectEventParams("validplayer")
	transformedParams     = effectEventParams("validcard")
	turnedFaceUpParams    = effectEventParams("validcard", "validcause")
	specializesParams     = effectEventParams("validcard")
)

// checkScryTriggers is Mode$ Scry (TriggerScry.performTest, 24 real lines),
// run once per player that scried (GameAction.java:2652-2655): ValidPlayer$
// against the scrying player; ToBottom$ needs at least one card put on the
// bottom. looked is ScryNum (cards put anywhere), bottom is ScryBottom, both
// read by TriggerCount$.
func (g *Game) checkScryTriggers(controller PlayerController, pid PlayerID, looked, bottom int) {
	matches := g.scanTriggers([]string{"Scry"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, scryTriggerParams) || !g.triggerPlayerMatches(h, t, "ValidPlayer", pid) {
			return triggeredObjects{}, false
		}
		if _, ok := t.Param("ToBottom"); ok && bottom <= 0 {
			return triggeredObjects{}, false
		}
		return triggeredObjects{player: pid,
			counts: triggerCounts{scryNum: looked, scryBottom: bottom, set: countScry}}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}

// checkSurveilTriggers is Mode$ Surveil (TriggerSurveil.performTest, 16 real
// lines, Player.java:1083-1088): ValidPlayer$ against the surveiling player;
// FirstTime$ needs first, the player's first surveil this turn.
func (g *Game) checkSurveilTriggers(controller PlayerController, pid PlayerID, first bool) {
	matches := g.scanTriggers([]string{"Surveil"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, surveilTriggerParams) || !g.triggerPlayerMatches(h, t, "ValidPlayer", pid) {
			return triggeredObjects{}, false
		}
		if _, ok := t.Param("FirstTime"); ok && !first {
			return triggeredObjects{}, false
		}
		return triggeredObjects{player: pid}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}

var investigatedParams = effectEventParams("validplayer", "firsttime")

// checkInvestigatedTriggers is Mode$ Investigated (TriggerInvestigated.
// performTest, 2 real lines, Player.java:2284-2289): ValidPlayer$ against the
// investigating player; FirstTime$ needs first, their first investigate this turn.
func (g *Game) checkInvestigatedTriggers(controller PlayerController, pid PlayerID, first bool) {
	matches := g.scanTriggers([]string{"Investigated"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, investigatedParams) || !g.triggerPlayerMatches(h, t, "ValidPlayer", pid) {
			return triggeredObjects{}, false
		}
		if _, ok := t.Param("FirstTime"); ok && !first {
			return triggeredObjects{}, false
		}
		return triggeredObjects{player: pid}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}

var connivesParams = effectEventParams("validcard")

// checkConnivesTriggers is Mode$ Connives (TriggerConnives.performTest, 3 real
// lines, ConniveEffect.java:109): ValidCard$ against the conniver, once per
// conniver the Connive replacement did not stop, after its discard.
func (g *Game) checkConnivesTriggers(controller PlayerController, card CardID) {
	g.pushTriggeredAbilities(controller, g.cardEventMatches("Connives", connivesParams, card))
}

var counteredParams = effectEventParams("validcard", "validcause", "validsa")

// counteredMatches is Mode$ Countered (TriggerCountered.performTest, 4 real
// lines, CounterEffect.java:301): once per countered spell or ability, after the
// spell moved; the caller works the matches out while the spell is still on the
// stack, as Java tests the card it passes (still cast), and pushes them after
// the move. ValidCard$ against the host card, ValidCause$ against the
// countering ability, ValidSA$ against the countered one. A spec the port
// cannot evaluate leaves the line unfired (GO-7).
func (g *Game) counteredMatches(card CardID, countered, cause *Ability) []Ability {
	if countered == nil || cause == nil {
		return nil
	}
	c := g.Card(card)
	return g.scanTriggers([]string{"Countered"}, func(h *Card, face triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, counteredParams) || !g.triggerCardMatches(h, t, "ValidCard", c) {
			return triggeredObjects{}, false
		}
		for key, a := range map[string]*Ability{"ValidCause": cause, "ValidSA": countered} {
			if spec, ok := t.Param(key); ok {
				if m, known := g.spellAbilityMatches(a, spec, h, h.Controller(), face.Amounts); !known || !m {
					return triggeredObjects{}, false
				}
			}
		}
		return triggeredObjects{card: card}, true
	})
}

var destroyedParams = effectEventParams("validcard", "validcauser")

// checkDestroyedTriggers is Mode$ Destroyed (TriggerDestroyed.performTest, 3 real
// lines, GameAction.java:2170-2189): once per card the Destroy replacement and
// regeneration let through, while it is still on the battlefield. ValidCard$
// against it, ValidCauser$ against the player whose ability destroyed it;
// causer is NoPlayer for state-based destruction, which matches no ValidCauser$.
func (g *Game) checkDestroyedTriggers(controller PlayerController, card CardID, causer PlayerID) {
	c := g.Card(card)
	matches := g.scanTriggers([]string{"Destroyed"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, destroyedParams) || !g.triggerCardMatches(h, t, "ValidCard", c) {
			return triggeredObjects{}, false
		}
		if _, ok := t.Param("ValidCauser"); ok && (causer == NoPlayer || !g.triggerPlayerMatches(h, t, "ValidCauser", causer)) {
			return triggeredObjects{}, false
		}
		return triggeredObjects{card: card}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}

var tokenCreatedParams = effectEventParams("validplayer", "validtoken", "onlyfirst")

// checkTokenCreatedTriggers is Mode$ TokenCreated (TriggerTokenCreated.
// performTest, 3 real lines, Player.java:1475-1481): once per token that
// entered, for its creator. ValidPlayer$ against the creator, ValidToken$
// against the token, OnlyFirst$ needing it to be the creator's first this turn
// (num is the creator's running count).
func (g *Game) checkTokenCreatedTriggers(controller PlayerController, creator PlayerID, token CardID, num int) {
	c := g.Card(token)
	matches := g.scanTriggers([]string{"TokenCreated"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, tokenCreatedParams) || !g.triggerPlayerMatches(h, t, "ValidPlayer", creator) || !g.triggerCardMatches(h, t, "ValidToken", c) {
			return triggeredObjects{}, false
		}
		if _, ok := t.Param("OnlyFirst"); ok && num != 1 {
			return triggeredObjects{}, false
		}
		return triggeredObjects{player: creator, card: token}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}

var tokenCreatedOnceParams = effectEventParams("validtoken", "onlyfirst")

// checkTokenCreatedOnceTriggers is Mode$ TokenCreatedOnce (TriggerTokenCreatedOnce.
// performTest, 2 real lines, CardZoneTable.java:146-152): once per batch of
// tokens created together, before the ChangesZoneAll triggers. ValidToken$
// needs one token of the batch to match; OnlyFirst$ names players (You) of
// whom at least one created their first token this turn in the batch.
func (g *Game) checkTokenCreatedOnceTriggers(controller PlayerController, cards []CardID) {
	var tokens []CardID
	firsts := map[PlayerID]bool{}
	for _, id := range cards {
		if c := g.Card(id); c.IsToken {
			tokens = append(tokens, id)
			if c.firstOfCreator {
				firsts[c.Owner] = true
			}
		}
	}
	if len(tokens) == 0 {
		return
	}
	matches := g.scanTriggers([]string{"TokenCreatedOnce"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, tokenCreatedOnceParams) {
			return triggeredObjects{}, false
		}
		var kept []CardID
		for _, id := range tokens {
			if g.triggerCardMatches(h, t, "ValidToken", g.Card(id)) {
				kept = append(kept, id)
			}
		}
		if len(kept) == 0 {
			return triggeredObjects{}, false
		}
		if spec, ok := t.Param("OnlyFirst"); ok {
			hit := false
			for p := range firsts {
				if m, rec := matchesPlayerSpec(g, p, h.Controller(), h.ID, spec); rec && m {
					hit = true
				}
			}
			if !hit {
				return triggeredObjects{}, false
			}
		}
		return triggeredObjects{card: kept[0]}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}

var shuffledParams = effectEventParams("validplayer", "shufflefromeffect", "shufflebyselfcontrolled")

// checkShuffledTriggers is Mode$ Shuffled (TriggerShuffled.performTest, 4 real
// lines, Player.java:1645): ValidPlayer$ against the shuffling player;
// ShuffleFromEffect$ needs a spell or ability behind the shuffle,
// ShuffleBySelfControlled$ that its activator is the shuffling player.
func (g *Game) checkShuffledTriggers(controller PlayerController, pid PlayerID, cause *Ability) {
	matches := g.scanTriggers([]string{"Shuffled"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, shuffledParams) || !g.triggerPlayerMatches(h, t, "ValidPlayer", pid) {
			return triggeredObjects{}, false
		}
		if _, ok := t.Param("ShuffleFromEffect"); ok && cause == nil {
			return triggeredObjects{}, false
		}
		if _, ok := t.Param("ShuffleBySelfControlled"); ok && (cause == nil || cause.Controller != pid) {
			return triggeredObjects{}, false
		}
		return triggeredObjects{player: pid}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}

var milledParams = effectEventParams("validplayer", "validcard")

// checkMilledTriggers is Mode$ Milled (TriggerMilled.performTest, 2 real lines,
// Player.java:1603-1607): once per milled card, after it moved. ValidPlayer$
// against the milling player, ValidCard$ against the card.
func (g *Game) checkMilledTriggers(controller PlayerController, pid PlayerID, card CardID) {
	c := g.Card(card)
	matches := g.scanTriggers([]string{"Milled"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, milledParams) || !g.triggerPlayerMatches(h, t, "ValidPlayer", pid) || !g.triggerCardMatches(h, t, "ValidCard", c) {
			return triggeredObjects{}, false
		}
		return triggeredObjects{player: pid, card: card}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}

// milledCardsMatching is the cards of one mill batch a ValidCard$ spec admits;
// a trigger with none does not fire (matchesValidParam over a collection is
// true when any member matches, and the Cards it hands on are the matching ones).
func (g *Game) milledCardsMatching(h *Card, t *compile.Ability, cards []CardID) []CardID {
	var kept []CardID
	for _, id := range cards {
		if g.triggerCardMatches(h, t, "ValidCard", g.Card(id)) {
			kept = append(kept, id)
		}
	}
	return kept
}

// checkMilledOnceTriggers is Mode$ MilledOnce (TriggerMilledOnce.performTest, 2
// real lines, Player.java:1609-1613): once per player's mill, if it milled any.
func (g *Game) checkMilledOnceTriggers(controller PlayerController, pid PlayerID, cards []CardID) {
	if len(cards) == 0 {
		return
	}
	matches := g.scanTriggers([]string{"MilledOnce"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, milledParams) || !g.triggerPlayerMatches(h, t, "ValidPlayer", pid) {
			return triggeredObjects{}, false
		}
		kept := g.milledCardsMatching(h, t, cards)
		if len(kept) == 0 {
			return triggeredObjects{}, false
		}
		return triggeredObjects{player: pid, card: kept[0]}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}

// checkMilledAllTriggers is Mode$ MilledAll (TriggerMilledAll.performTest, 3
// real lines, GameAction.java:2715-2718): once per mill effect, if any card was
// milled. ValidCard$ against the whole batch; ValidPlayer$ is carried by the
// corpus lines but never read by Java (TriggerMilledAll.java:36-42), so it is
// accepted and ignored.
func (g *Game) checkMilledAllTriggers(controller PlayerController, cards []CardID) {
	if len(cards) == 0 {
		return
	}
	matches := g.scanTriggers([]string{"MilledAll"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, milledParams) {
			return triggeredObjects{}, false
		}
		kept := g.milledCardsMatching(h, t, cards)
		if len(kept) == 0 {
			return triggeredObjects{}, false
		}
		return triggeredObjects{card: kept[0]}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}

var collectEvidenceParams = effectEventParams("validplayer")

// checkCollectEvidenceTriggers is Mode$ CollectEvidence
// (TriggerCollectEvidence.performTest, CostCollectEvidence.java:78): ValidPlayer$
// against the player who paid the cost; the player is TriggeredPlayer.
func (g *Game) checkCollectEvidenceTriggers(controller PlayerController, pid PlayerID) {
	g.pushTriggeredAbilities(controller, g.playerEventMatches("CollectEvidence", collectEvidenceParams, pid))
}

// commitCrimeParams are the params a Mode$ CommitCrime trigger may carry
// (TriggerCommitCrime.performTest reads ValidPlayer$ only; the 21 corpus
// lines add ActivationLimit$ and PlayerTurn$, read by triggerEffectAPI).
var commitCrimeParams = effectEventParams("validplayer")

// crimeTriggers is the Mode$ CommitCrime run of Player.commitCrime
// (Player.java:3962-3968, TriggerCommitCrime.java:17-28) for pushed, the
// stack item just added: none when pushing it was no crime (isCrime, the
// check noteCrime counted), else every trigger whose ValidPlayer$ admits its
// controller, who is TriggeredPlayer. A caller appends these to the push's
// BecomesTarget batch, after it, as MagicStack.add runs the crime check after
// the BecomesTarget triggers (MagicStack.java:408-451).
func (g *Game) crimeTriggers(pushed Ability) []Ability {
	if !g.isCrime(pushed) {
		return nil
	}
	return g.playerEventMatches("CommitCrime", commitCrimeParams, pushed.Controller)
}

func (g *Game) playerEventMatches(mode string, params map[string]bool, pid PlayerID) []Ability {
	return g.scanTriggers([]string{mode}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, params) || !g.triggerPlayerMatches(h, t, "ValidPlayer", pid) {
			return triggeredObjects{}, false
		}
		return triggeredObjects{player: pid}, true
	})
}

// checkTransformedTriggers is Mode$ Transformed (TriggerTransformed, 35 real
// lines): ValidCard$ against the permanent that just transformed.
func (g *Game) checkTransformedTriggers(controller PlayerController, card CardID) {
	g.pushTriggeredAbilities(controller, g.cardEventMatches("Transformed", transformedParams, card))
}

// checkSpecializesTriggers is Mode$ Specializes (TriggerSpecializes, 47 real
// lines): ValidCard$ against the card that just specialized
// (SetStateEffect.java:213-215, TriggerSpecializes.java:46-64).
func (g *Game) checkSpecializesTriggers(controller PlayerController, card CardID) {
	g.pushTriggeredAbilities(controller, g.cardEventMatches("Specializes", specializesParams, card))
}

// checkTurnedFaceUpTriggers is Mode$ TurnFaceUp (TriggerTurnFaceUp, 126 real
// lines): ValidCard$ against the permanent just turned face up, ValidCause$
// against the ability that turned it (nil for a special action, which matches
// no ValidCause$ -- matchesValidParam is false for a null object).
func (g *Game) checkTurnedFaceUpTriggers(controller PlayerController, card CardID, cause *Ability) {
	c := g.Card(card)
	matches := g.scanTriggers([]string{"TurnFaceUp"}, func(h *Card, face triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, turnedFaceUpParams) || !g.triggerCardMatches(h, t, "ValidCard", c) {
			return triggeredObjects{}, false
		}
		if spec, ok := t.Param("ValidCause"); ok {
			if cause == nil {
				return triggeredObjects{}, false
			}
			if m, known := g.spellAbilityMatches(cause, spec, h, h.Controller(), face.Amounts); !known || !m {
				return triggeredObjects{}, false
			}
		}
		return triggeredObjects{card: card}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}

var attachedTriggerParams = effectEventParams("validsource", "validtarget", "targetrelativetosource")

// checkAttachedTriggers is Mode$ Attached (TriggerAttached, 4 real lines),
// fired by Card.attachToEntity after the Event$ Attached replacements
// (Card.java:3949-3953): ValidSource$ against the attachment, ValidTarget$ and
// TargetRelativeToSource$ against what it became attached to -- a card or a
// player. TargetRelativeToSource$ is matchesValidParam(param, target, source)
// with the attachment as the source card, so a `cmcLEX` operand is the
// attachment's own value, read through the trigger host's SVar X
// (Game.relativeAmounts): Eriette, the Beguiler's `Count$CardManaCost` is the
// Aura's mana value. The attachment is Defined$ TriggeredSource, the host
// TriggeredTarget (TriggerAttached.setTriggeringObjects).
func (g *Game) checkAttachedTriggers(controller PlayerController, attachment CardID, target EntityID) {
	src := g.Card(attachment)
	matches := g.scanTriggers([]string{"Attached"}, func(h *Card, face triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, attachedTriggerParams) || !g.triggerCardMatches(h, t, "ValidSource", src) {
			return triggeredObjects{}, false
		}
		if spec, ok := t.Param("ValidTarget"); ok && !g.attachTargetMatches(h, spec, h.ID, target) {
			return triggeredObjects{}, false
		}
		if spec, ok := t.Param("TargetRelativeToSource"); ok && !g.attachTargetRelative(h, face, spec, attachment, target) {
			return triggeredObjects{}, false
		}
		return face.objects(triggeredObjects{source: CardEntity(attachment), sourceController: src.Controller(), target: target}), true
	})
	g.pushTriggeredAbilities(controller, matches)
}

// attachTargetMatches is matchesValid(target, spec, srcCard) on a trigger of
// h with src as the source card: a card target against the valid string, a
// player target against the player spec. A player spec this port cannot read
// does not match (GO-7).
func (g *Game) attachTargetMatches(h *Card, spec string, src CardID, target EntityID) bool {
	if id, ok := target.AsCard(); ok {
		return Matches(g, g.Card(id), valid.Parse(spec), h.Controller(), src)
	}
	pid, _ := target.AsPlayer()
	matched, recognized := matchesPlayerSpec(g, pid, h.Controller(), src, spec)
	return recognized && matched
}

// attachTargetRelative is TargetRelativeToSource$: attachTargetMatches with
// the attachment as the source card and the trigger host's SVars answering a
// comparison operand (AbilityUtils.calculateAmount(source, "X", trigger)
// reads the SVar off the trigger, the amount off the source).
func (g *Game) attachTargetRelative(h *Card, face triggerFace, spec string, attachment CardID, target EntityID) bool {
	prev := g.relativeAmounts
	g.relativeAmounts = face.Amounts
	defer func() { g.relativeAmounts = prev }()
	return g.attachTargetMatches(h, spec, attachment, target)
}

func (g *Game) cardEventMatches(mode string, params map[string]bool, card CardID) []Ability {
	c := g.Card(card)
	return g.scanTriggers([]string{mode}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, params) || !g.triggerCardMatches(h, t, "ValidCard", c) {
			return triggeredObjects{}, false
		}
		return triggeredObjects{card: card}, true
	})
}

// checkFlippedCoinTriggers is Mode$ FlippedCoin (TriggerFlippedCoin.java:56-70,
// run from FlipCoinEffect.flipCoin :235-239): ValidPlayer$ against the
// flipper, ValidResult$ "Win" meaning the flip was won and any other value
// meaning it was lost (PORT-7, the method does not match the result it
// names). An absent ValidResult$ matches both. The player is TriggeredPlayer.
func (g *Game) checkFlippedCoinTriggers(controller PlayerController, flipper PlayerID, won bool) {
	matches := g.scanTriggers([]string{"FlippedCoin"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, flippedCoinParams) || !g.triggerPlayerMatches(h, t, "ValidPlayer", flipper) {
			return triggeredObjects{}, false
		}
		if v, ok := t.Param("ValidResult"); ok && (v == "Win") != won {
			return triggeredObjects{}, false
		}
		return triggeredObjects{player: flipper}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}

// checkFlippedCoinOnceTriggers is Mode$ FlippedCoinOnce
// (TriggerFlippedCoinOnce.java:20, FlipCoinEffect.java:164-167): once per
// flipper after its sub-abilities, whatever the results.
func (g *Game) checkFlippedCoinOnceTriggers(controller PlayerController, flipper PlayerID) {
	matches := g.scanTriggers([]string{"FlippedCoinOnce"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, flippedCoinOnceParams) || !g.triggerPlayerMatches(h, t, "ValidPlayer", flipper) {
			return triggeredObjects{}, false
		}
		return triggeredObjects{player: flipper}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}

var conjureAllParams = effectEventParams("validplayer", "validcard")

// checkConjureAllTriggers is Mode$ ConjureAll (TriggerConjureAll.java:19-46,
// fired from MakeCardEffect.java:219-224) once per conjuring player: ValidPlayer$
// against that player, ValidCard$ must match some made card, and the trigger
// sees only the matching cards. Without ValidCard$ it fires on an empty batch.
func (g *Game) checkConjureAllTriggers(controller PlayerController, pid PlayerID, made []CardID) {
	matches := g.scanTriggers([]string{"ConjureAll"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, conjureAllParams) || !g.triggerPlayerMatches(h, t, "ValidPlayer", pid) {
			return triggeredObjects{}, false
		}
		kept := made
		if spec, ok := t.Param("ValidCard"); ok {
			kept = nil
			for _, id := range made {
				if Matches(g, g.Card(id), valid.Parse(spec), h.Controller(), h.ID) {
					kept = append(kept, id)
				}
			}
			if len(kept) == 0 {
				return triggeredObjects{}, false
			}
		}
		return triggeredObjects{player: pid, cards: kept}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}

var losesGameParams = effectEventParams("validplayer")

// checkLosesGameTriggers is Mode$ LosesGame (TriggerLosesGame.java:34-46,
// queued by Game.onPlayerLost, Game.java:1025-1029) for a player the state-based
// actions just eliminated: ValidPlayer$ against them. Java clears the active
// triggers of cards the loser both owns and controls first (Game.java:897-902,
// CR 800.4d), so those hosts never see their own loss.
func (g *Game) checkLosesGameTriggers(controller PlayerController, loser PlayerID) {
	matches := g.scanTriggers([]string{"LosesGame"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if h.Owner == loser && h.Controller() == loser {
			return triggeredObjects{}, false
		}
		if !paramsResolvable(t, losesGameParams) || !g.triggerPlayerMatches(h, t, "ValidPlayer", loser) {
			return triggeredObjects{}, false
		}
		return triggeredObjects{player: loser}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}
