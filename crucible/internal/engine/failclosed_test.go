package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestEffectsFailClosedOnUnresolvedShapes proves each effect of this pack
// reports an error, rather than guessing, for a param or value it cannot
// honour yet (PORT-8, GO-7) -- one representative line per branch.
func TestEffectsFailClosedOnUnresolvedShapes(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"DB$ ChangeZone | Defined$ Self | Origin$ Nowhere | Destination$ Hand",
		"DB$ ChangeZone | Defined$ Self | Origin$ Battlefield | Destination$ Hand | GainControl$ TriggeredPlayer",
		"DB$ ChangeZone | Defined$ TriggeredCard | Origin$ Battlefield | Destination$ Hand",
		"DB$ ChangeZone | Origin$ Library | Destination$ Hand | DefinedPlayer$ TriggeredPlayer",
		"DB$ ChangeZone | Origin$ Library | Destination$ Hand | ChangeNum$ Bogus",
		"DB$ ChangeZone | Origin$ Library | Destination$ Hand | Defined$ TriggeredCard",
		"DB$ ChangeZone | Origin$ Library | Destination$ Library | LibraryPosition$ Bogus",
		"DB$ ChangeZoneAll | ChangeType$ Card | Origin$ Hand | Destination$ Nowhere",
		"DB$ ChangeZoneAll | ChangeType$ Card | Origin$ Nowhere | Destination$ Hand",
		"DB$ ChangeZoneAll | ChangeType$ Card | Defined$ TriggeredPlayer | Origin$ Hand | Destination$ Graveyard",
		"DB$ ChangeZoneAll | ChangeType$ Card | Origin$ Hand | Destination$ Library | LibraryPosition$ 3",
		"DB$ Dig | DigNum$ 2 | DestinationZone$ Nowhere",
		"DB$ Dig | DigNum$ 2 | DestinationZone2$ Nowhere",
		"DB$ Dig | DigNum$ 2 | LibraryPosition2$ 5",
		"DB$ Dig | DigNum$ 2 | ChangeNum$ Bogus",
		"DB$ Dig | DigNum$ 2 | Defined$ TriggeredPlayer",
		"DB$ DigUntil | Valid$ Card | Amount$ Bogus | FoundDestination$ Hand",
		"DB$ DigUntil | Valid$ Card | MaxRevealed$ Bogus | FoundDestination$ Hand",
		"DB$ DigUntil | Valid$ Card | FoundDestination$ Nowhere",
		"DB$ DigUntil | Valid$ Card | RevealedDestination$ Nowhere",
		"DB$ DigUntil | Valid$ Card | NoneFoundDestination$ Nowhere",
		"DB$ DigUntil | Valid$ Card | OptionalNoDestination$ Nowhere",
		"DB$ DigUntil | Valid$ Card | FoundDestination$ Library | FoundLibraryPosition$ 4",
		"DB$ DigUntil | Valid$ Card | RevealedDestination$ Library | RevealedLibraryPosition$ 4",
		"DB$ DigUntil | Valid$ Card | NoneFoundDestination$ Library | NoneFoundLibraryPosition$ 4",
		"DB$ DigUntil | Valid$ Card | Defined$ TriggeredPlayer",
		"DB$ DigUntil | Valid$ Creature | FoundDestination$ Hand | Shuffle$ True | ShuffleCondition$ Always",
		"DB$ RearrangeTopOfLibrary | NumCards$ Bogus",
		"DB$ RearrangeTopOfLibrary | NumCards$ 2 | RearrangePlayer$ Opponent",
		"DB$ RearrangeTopOfLibrary | NumCards$ 2 | Defined$ TriggeredPlayer",
		"DB$ Explore | Num$ Bogus",
		"DB$ Explore | Defined$ TriggeredCard",
		"DB$ Explore | Condition$ Kicked",
		"DB$ LookAt | Defined$ Self | Condition$ Kicked",
		"DB$ Branch | BranchConditionSVar$ 1 | BranchConditionSVarCompare$ GE",
		"DB$ Branch | BranchConditionSVar$ 1 | BranchConditionSVarCompare$ GEBogus",
		"DB$ Branch | BranchConditionSVar$ 1 | PlayerTurn$ True",
		"DB$ GenericChoice | Defined$ You | ChoiceAmount$ Bogus",
		"DB$ GenericChoice | Defined$ TriggeredPlayer",
		"DB$ Repeat | MaxRepeat$ Bogus",
		"DB$ Repeat | RepeatDefined$ Self",
		"DB$ RepeatEach | RepeatCards$ Creature | Zone$ Nowhere",
		"DB$ RepeatEach | DefinedCards$ TriggeredCard",
		"DB$ RepeatEach | RepeatPlayers$ TriggeredPlayer",
		"DB$ Regenerate | Defined$ Self | RegenerationAbility$ DBX",
		"DB$ Regenerate | Defined$ TriggeredCard",
		"DB$ Fog | Condition$ Kicked",
		"DB$ AddTurn | Defined$ You | NumTurns$ Bogus",
		"DB$ AddTurn | Defined$ TriggeredPlayer | NumTurns$ 1",
		"DB$ SkipTurn | Defined$ You | NumTurns$ Bogus",
		"DB$ SkipTurn | Defined$ TriggeredPlayer | NumTurns$ 1",
		"DB$ SkipTurn | Defined$ You | NumTurns$ 1 | Condition$ Kicked",
		"DB$ GainControl | Defined$ Self | NewController$ TriggeredPlayer",
		"DB$ GainControl | Defined$ TriggeredCard",
		"DB$ ExchangeControl | Defined$ TriggeredCard",
		"DB$ ExchangeControl | Defined$ Self | TargetsWithSharedCardType$ Creature",
		"DB$ HealDamage | Defined$ TriggeredCard",
		"DB$ HealDamage | Defined$ Self | Condition$ Kicked",
		"DB$ EachDamage | DefinedDamagers$ TriggeredCard | Defined$ You",
		"DB$ EachDamage | DefinedDamagers$ Self | NumDmg$ Bogus | Defined$ You",
		"DB$ EachDamage | DefinedDamagers$ Self | ToEachOther$ TriggeredCard",
		"DB$ EachDamage | DefinedDamagers$ Self | Defined$ TriggeredCard",
		"DB$ EachDamage | DefinedDamagers$ Self | Defined$ You | Ultimate$ True",
		"DB$ DrainMana | Defined$ TriggeredPlayer",
	} {
		g, p, _ := newTwoPlayerGame(t)
		libraryCards(t, g, p, 3)
		c := engine.NewScriptedController()
		def := etbChainDef(t, "Test Fail Closed", line, "DBX", "DB$ GainLife | Defined$ You | LifeAmount$ 1")
		if _, err := castETBChain(t, g, p, def, c); err == nil {
			t.Errorf("%q: ResolveStack succeeded, want an error", line)
		}
	}
}

// TestTokenAnimateTriggerPackFailsClosed is the same guarantee for the
// token, Animate-shaped, delayed-trigger, dispatch and phase effects: one
// representative unresolved line per branch.
func TestTokenAnimateTriggerPackFailsClosed(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"DB$ Token | TokenScript$ w_1_1_soldier | TokenAttacking$ True",
		"DB$ Token | TokenScript$ w_1_1_soldier | PumpKeywords$ Haste | PumpDuration$ UntilYourNextTurn",
		"DB$ Token | TokenScript$ w_1_1_soldier | TokenOwner$ TriggeredPlayer",
		"DB$ Token",
		"DB$ Investigate | Optional$ True",
		"DB$ Amass | Type$ Elf | Num$ 1",
		"DB$ Amass | Num$ 1",
		"DB$ Incubate | Amount$ Bogus",
		"DB$ Animate | Defined$ Self | Triggers$ DBX",
		"DB$ Animate | Defined$ Self | Power$ 1 | Duration$ Perpetual",
		"DB$ Animate | Defined$ Self | Types$ ChosenType",
		"DB$ Animate | Defined$ Self | Colors$ Plaid",
		"DB$ Animate | Defined$ Self | Keywords$ HIDDEN CARDNAME can't block.",
		"DB$ AnimateAll | ValidCards$ Creature | Zone$ Graveyard | Power$ 1",
		"DB$ Debuff | Defined$ Self | Keywords$ Forestwalk | AllSuffixKeywords$ walk",
		"DB$ Protection | Defined$ Self | Gains$ TargetedCardColor",
		"DB$ Protection | Defined$ Self | Gains$ Choice | Choices$ AnyColor | Choser$ Controller",
		"DB$ ProtectionAll | ValidCards$ Creature | ValidPlayers$ You | Gains$ red",
		"DB$ DelayedTrigger | Mode$ ChangesZone | Execute$ DBX",
		"DB$ DelayedTrigger | Mode$ Phase | Phase$ Upkeep | RememberNumber$ True | Execute$ DBX",
		"DB$ DelayedTrigger | Mode$ Phase | Phase$ Upkeep",
		"DB$ ImmediateTrigger | RememberSVarAmount$ X | Execute$ DBX",
		"DB$ ImmediateTrigger | TriggerAmount$ Bogus | Execute$ DBX",
		"DB$ Charm | ChoiceRestriction$ ThisGame | Choices$ DBX",
		"DB$ FlipCoin | ForEachPlayer$ Player | WinSubAbility$ DBX",
		"DB$ RollDice | Sides$ 6 | RerollResults$ True",
		"DB$ Clash | Defined$ TriggeredPlayer",
		"DB$ Seek | DefinedCards$ Self",
		"DB$ StoreSVar | SVar$ X | Type$ Targeted | Expression$ CardPower",
		"DB$ StoreSVar | SVar$ EachPlayer | Type$ Number | Expression$ 1",
		"DB$ StoreSVar | SVar$ X | Type$ Calculate | Expression$ Bogus",
		"DB$ Balance | Valid$ Creature | Zone$ Graveyard",
		"DB$ AddPhase | ExtraPhase$ Bogus",
		"DB$ AddPhase | ExtraPhase$ Combat | ExtraPhaseDelayedTrigger$ DBX",
		"DB$ SkipPhase | Defined$ You | Step$ Draw | Start$ True",
		"DB$ SkipPhase | Defined$ You | Step$ Draw | Duration$ UntilYourNextTurn",
		"DB$ SkipPhase | Defined$ You | Step$ Bogus",
	} {
		g, p, _ := newTokenGame(t)
		libraryCards(t, g, p, 3)
		c := engine.NewScriptedController()
		def := etbChainDef(t, "Test Fail Closed", line, "DBX", "DB$ GainLife | Defined$ You | LifeAmount$ 1")
		if _, err := castETBChain(t, g, p, def, c); err == nil {
			t.Errorf("%q: ResolveStack succeeded, want an error", line)
		}
	}
}
