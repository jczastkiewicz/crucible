package compile

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/carddb"
)

// Player.createCommanderEffect (Player.java:3254-3300) for a game with no
// Commander variant (ADR-0053): the Command-zone effect card every player
// with a commander owns. Its two traits are Java's own strings; Java sets the
// replacement's ChangeZone as an overriding ability, here the ReplaceWith$
// SVar it would be. Compiled once per DB (PORT-2), shared by every game.
const commanderEffectName = "Commander Effect"

const (
	commanderEffectReplacement = "Event$ Moved | ValidCard$ Card.IsCommander+YouOwn | Secondary$ True | Optional$ True | OptionalDecider$ You | CommanderMoveReplacement$ True | Destination$ Hand,Library | ReplaceWith$ CommanderMoveToCommand | Description$ If a commander would be put into its owner's hand or library from anywhere, its owner may put it into the command zone instead."
	commanderEffectMove        = "DB$ ChangeZone | Origin$ Battlefield,Graveyard,Exile,Library,Hand | Destination$ Command | Defined$ ReplacedCard"
	commanderEffectMayPlay     = "Mode$ Continuous | EffectZone$ Command | MayPlay$ True | Affected$ Card.IsCommander+YouOwn | AffectedZone$ Command"
)

// compileCommanderEffect builds the Commander Effect's definition. Its text
// is fixed, so an error is a broken build, not a broken corpus.
func compileCommanderEffect() *Card {
	raw := &carddb.Card{Filename: "commander_effect"}
	f := &raw.Faces[0]
	f.Present = true
	f.Name = commanderEffectName
	f.Replacements = []string{commanderEffectReplacement}
	f.Statics = []string{commanderEffectMayPlay}
	f.SVars.Set("CommanderMoveToCommand", commanderEffectMove)
	card, err := Compile(raw)
	if err != nil {
		panic(fmt.Sprintf("compile: Commander Effect: %v", err))
	}
	return card
}

// CommanderEffect is the Commander Effect card's definition (ADR-0053). A DB
// built by neither constructor (a nil or zero DB in a test) compiles its own.
func (db *DB) CommanderEffect() *Card {
	if db == nil || db.commanderEffect == nil {
		return compileCommanderEffect()
	}
	return db.commanderEffect
}

// Player.createPlanechaseEffects (Player.java:3306-3331, ADR-0056): the
// "Planar Dice" effect card every Planechase player owns. Java parses the
// trigger and sets the planeswalk as its overriding ability; here it is the
// Execute$ SVar it would be. The special action's X is the game's rolls this
// turn. Compiled once per DB (PORT-2).
const planarDiceName = "Planar Dice"

const (
	planarDiceTrigger = "Mode$ PlanarDice | Result$ Planeswalk | TriggerZones$ Command | ValidPlayer$ You | Secondary$ True | Execute$ PlanarDiceWalk | TriggerDescription$ Whenever you roll the Planeswalker symbol on the planar die, planeswalk."
	planarDiceWalk    = "DB$ Planeswalk | Cause$ PlanarDie"
	planarDiceAction  = "ST$ RollPlanarDice | Cost$ X | SorcerySpeed$ True | Activator$ Player | SpecialAction$ True | ActivationZone$ Command | SpellDescription$ Roll the planar dice. X is equal to the number of times you have previously taken this action this turn. | CostDesc$ {X}: "
)

// compilePlanarDiceEffect builds the Planar Dice card's definition; its text
// is fixed, so an error is a broken build.
func compilePlanarDiceEffect() *Card {
	raw := &carddb.Card{Filename: "planar_dice"}
	f := &raw.Faces[0]
	f.Present = true
	f.Name = planarDiceName
	f.Triggers = []string{planarDiceTrigger}
	f.Abilities = []string{planarDiceAction}
	f.SVars.Set("PlanarDiceWalk", planarDiceWalk)
	f.SVars.Set("X", "Count$PlanarDiceSpecialActionThisTurn")
	card, err := Compile(raw)
	if err != nil {
		panic(fmt.Sprintf("compile: Planar Dice: %v", err))
	}
	return card
}

// PlanarDiceEffect is the Planar Dice card's definition (ADR-0056). A DB
// built by neither constructor compiles its own.
func (db *DB) PlanarDiceEffect() *Card {
	if db == nil || db.planarDiceEffect == nil {
		return compilePlanarDiceEffect()
	}
	return db.planarDiceEffect
}

// Player.createCompanionEffect (Player.java:3310-3322): the Command-zone
// effect card a player's chosen companion comes with. Its static grants the
// companion, while it is in the Command zone, the special action that puts
// it into its owner's hand for {3} (CR 702.139c). Java names the card after
// its companion; the engine renames its copy of this definition. Compiled
// once per DB (PORT-2).
const (
	companionEffectStatic = "Mode$ Continuous | EffectZone$ Command | Affected$ Card.YouOwn+EffectSource | AffectedZone$ Command | AddAbility$ MoveToHand"
	companionEffectAction = "ST$ ChangeZone | Cost$ 3 | Defined$ Self | Origin$ Command | Destination$ Hand | SorcerySpeed$ True | ActivationZone$ Command | SpellDescription$ Companion - Put CARDNAME in to your hand"
)

// compileCompanionEffect builds the Companion Effect's definition; its text
// is fixed, so an error is a broken build.
func compileCompanionEffect() *Card {
	raw := &carddb.Card{Filename: "companion_effect"}
	f := &raw.Faces[0]
	f.Present = true
	f.Name = "Companion Effect"
	f.Statics = []string{companionEffectStatic}
	f.SVars.Set("MoveToHand", companionEffectAction)
	card, err := Compile(raw)
	if err != nil {
		panic(fmt.Sprintf("compile: Companion Effect: %v", err))
	}
	return card
}

// CompanionEffect is the Companion Effect card's definition. A DB built by
// neither constructor compiles its own.
func (db *DB) CompanionEffect() *Card {
	if db == nil || db.companionEffect == nil {
		return compileCompanionEffect()
	}
	return db.companionEffect
}
