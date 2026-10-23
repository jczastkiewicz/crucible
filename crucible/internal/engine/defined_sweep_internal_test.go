package engine

// package engine, not engine_test (TEST-2's "invariant not observable from
// outside" row): definedPlayers and definedCards answer from an
// abilityRefs record only the resolver builds. Every Defined$ keyword is read
// twice, against refs where the trigger recorded nothing (each reader must
// report it, or answer none, never panic) and against refs where every
// object is recorded.

import (
	"slices"
	"strings"
	"testing"
)

var definedKeywords = []string{
	"ActivePlayer", "AttackingPlayer", "BottomOfLibrary", "CardController", "CardOwner", "CastSA>Collected",
	"Caster", "ChoosingPlayer", "ChosenCard", "ChosenCardController", "ChosenCardOwner", "ChosenPlayer",
	"Collected", "Convoked", "CorrectedSelf", "DefendingPlayer", "DelayTriggerRemembered",
	"DelayTriggerRememberedController", "DelayTriggerRememberedLKI", "DirectRemembered", "Discarded",
	"DiscardedCards", "EffectSource", "Enchanted", "EnchantedController", "EnchantedOwner", "EnchantedPlayer",
	"Equipped", "Exiled", "ExiledCards", "FlippedHeads", "FlippedTails", "Imprinted", "ImprintedController",
	"ImprintedLKI", "ImprintedOwner", "NextOpponentToYourLeft", "NextOpponentToYourRight",
	"NextPlayerToYourLeft", "NextPlayerToYourRight", "Opponent", "OriginalHost", "ParentTarget",
	"ParentTargetedController", "Player", "Player.Opponent", "Promised", "Registered", "Remembered",
	"RememberedCard", "RememberedController", "RememberedFirst", "RememberedLKI", "RememberedLast",
	"RememberedOwner", "RememberedPlayer", "ReplacedCard", "ReplacedDefendingPlayer", "ReplacedPlayer",
	"ReplacedTarget", "Returned", "ReturnedCards", "Sacrificed", "SacrificedCards", "Self",
	"Self.StrictlySelf", "ShieldEffectTarget", "SourceController", "Tapped", "Targeted", "TargetedAndYou",
	"TargetedCard", "TargetedController", "TargetedOrController", "TargetedOwner", "TargetedPlayer",
	"TargetedSource", "ThisTargetedCard", "ThisTargetedController", "ThisTargetedOwner", "ThisTargetedPlayer",
	"TopOfGraveyard", "TopOfLibrary", "TopThirdOfLibrary", "TopThirdRoundedDownOfLibrary", "TriggeredActivator",
	"TriggeredAttackedTarget", "TriggeredAttacker", "TriggeredAttackerController", "TriggeredAttackerLKICopy",
	"TriggeredAttackerOwner", "TriggeredAttackers", "TriggeredAttackingPlayer", "TriggeredBlocker",
	"TriggeredBlockerController", "TriggeredBlockerLKICopy", "TriggeredBlockerOwner", "TriggeredBlockers",
	"TriggeredCard", "TriggeredCardController", "TriggeredCardLKICopy", "TriggeredCardOpponent",
	"TriggeredCardOwner", "TriggeredCards", "TriggeredCardsLKICopy", "TriggeredDefender",
	"TriggeredDefendingPlayer", "TriggeredNewCard", "TriggeredNewCardLKICopy", "TriggeredOriginalController",
	"TriggeredPlayer", "TriggeredSource", "TriggeredSourceController", "TriggeredSourceLKICopy",
	"TriggeredSources", "TriggeredSourcesLKICopy", "TriggeredTarget", "TriggeredTargetLKICopy",
	"TriggeredTargets", "You", "Spawner>TriggeredTarget", "Bogus Keyword",
}

func definedSweepFixture(t *testing.T) (g *Game, host *Card, full abilityRefs) {
	t.Helper()
	g = targetCandidatesTestGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, Main1)
	def := targetCandidatesTestCreature(t)
	hostID := g.NewCard(def, a, Battlefield)
	c1 := g.NewCard(def, a, Battlefield)
	c2 := g.NewCard(def, b, Battlefield)
	aura := g.NewCard(def, a, Battlefield)
	g.Attach(aura, c1)
	for range 4 {
		g.NewCard(def, a, Library)
	}
	g.NewCard(def, a, Graveyard)
	host = g.Card(hostID)
	host.Memory.Choose(c1)
	host.Memory.SetChosenPlayer(b)
	host.Memory.Imprint(c2)
	host.Memory.Remember(CardEntity(c1))
	host.Memory.Remember(PlayerEntity(b))
	host.Memory.Remember(CardEntity(c2))
	full = abilityRefs{
		targets:           []EntityID{CardEntity(c1), PlayerEntity(b)},
		parentTargets:     []EntityID{CardEntity(c2)},
		ancestorTargeting: true,
		triggerRemembered: []EntityID{CardEntity(c1), PlayerEntity(b)},
		triggered: triggeredObjects{
			source: CardEntity(c1), sourceController: a, player: b, blocker: c2, attacker: c1,
			card: c2, cardLKIController: b, cardLKIOwner: a, activator: a, originalController: b,
			attackingPlayer: a, originalHost: aura, cards: []CardID{c1, c2},
			sources: []CardID{c1}, sourcesSet: true,
			attackers: []CardID{c1}, attackersRecorded: true, blockers: []CardID{c2},
			attackedTargets: []EntityID{PlayerEntity(b), CardEntity(c2)}, attackedTargetsRecorded: true,
			damagedTargets: []EntityID{PlayerEntity(b), CardEntity(c2)}, damagedTargetsRecorded: true,
			target: PlayerEntity(b),
		},
		paid: paidLists{sacrificed: []CardID{c1}, sacrificedMoved: []CardID{c1}, exiled: []CardID{c2},
			discarded: []CardID{c1}, tapped: []CardID{c2}, returned: []CardID{c1}, recorded: true},
		replaced: c1, replacedPlayer: b, replacedDefendingPlayer: b, replacedTarget: PlayerEntity(b),
		shieldTarget: CardEntity(c1), choosing: b, hasChoosing: true,
		game: g, activator: a,
	}
	full.spawnerTriggered = &full.triggered
	return g, host, full
}

func TestDefinedKeywordsReadEmptyAndFullRefs(t *testing.T) {
	t.Parallel()

	g, host, full := definedSweepFixture(t)
	a := g.Players()[0]
	empty := abilityRefs{game: g, activator: a}
	for _, refs := range []abilityRefs{empty, full} {
		for _, kw := range definedKeywords {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("%q panicked: %v", kw, r)
					}
				}()
				players, perr := definedPlayers(g, a, host.ID, kw, refs)
				cards, cerr := definedCards(host, kw, refs)
				ents, eerr := definedEntities(g, a, host, kw, refs)
				if perr != nil && len(players) != 0 {
					t.Errorf("%q: definedPlayers answered %v with error %v", kw, players, perr)
				}
				if cerr != nil && len(cards) != 0 {
					t.Errorf("%q: definedCards answered %v with error %v", kw, cards, cerr)
				}
				if eerr == nil && len(ents) != len(players)+len(cards) && perr == nil && cerr == nil {
					t.Errorf("%q: definedEntities has %d, want %d players + %d cards", kw, len(ents), len(players), len(cards))
				}
			}()
		}
	}
}

func TestDefinedFullRefsAnswerWhatTheTriggerRecorded(t *testing.T) {
	t.Parallel()

	g, host, full := definedSweepFixture(t)
	a, b := g.Players()[0], g.Players()[1]
	players, err := definedPlayers(g, a, host.ID, "TriggeredTargets", full)
	if err != nil || !slices.Equal(players, []PlayerID{b}) {
		t.Errorf("TriggeredTargets players = %v, %v; want [%v]", players, err, b)
	}
	cards, err := definedCards(host, "TriggeredTargets", full)
	if err != nil || len(cards) != 1 {
		t.Errorf("TriggeredTargets cards = %v, %v; want the one damaged card", cards, err)
	}
	cards, err = definedCards(host, "OriginalHost", full)
	if err != nil || !slices.Equal(cards, []CardID{full.triggered.originalHost}) {
		t.Errorf("OriginalHost = %v, %v; want the granting card", cards, err)
	}
	cards, err = definedCards(host, "OriginalHost", abilityRefs{})
	if err != nil || !slices.Equal(cards, []CardID{host.ID}) {
		t.Errorf("OriginalHost of a printed trigger = %v, %v; want the host", cards, err)
	}
	if _, err = definedCards(host, "Spawner>TriggeredTarget", abilityRefs{}); err == nil || !strings.Contains(err.Error(), "no ImmediateTrigger") {
		t.Errorf("Spawner> with no spawner: err = %v, want the no-spawner error", err)
	}
	third, err := definedCards(host, "TopThirdOfLibrary", full)
	if err != nil || len(third) != 2 {
		t.Errorf("TopThirdOfLibrary of 4 cards = %v, %v; want 2", third, err)
	}
	down, err := definedCards(host, "TopThirdRoundedDownOfLibrary", full)
	if err != nil || len(down) != 1 {
		t.Errorf("TopThirdRoundedDownOfLibrary of 4 cards = %v, %v; want 1", down, err)
	}
	first, _ := definedCards(host, "RememberedFirst", full)
	last, _ := definedCards(host, "RememberedLast", full)
	if len(first) != 1 || len(last) != 1 || first[0] == last[0] {
		t.Errorf("RememberedFirst/Last = %v/%v, want two different cards", first, last)
	}
}
