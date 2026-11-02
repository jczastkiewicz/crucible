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
