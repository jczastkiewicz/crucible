package engine

//enginelint:allow ability amount card condition control effecthelpers game id memory parts zone zonemove

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
)

// assembleContraptionEffect is AssembleContraptionEffect.java (Unfinity's
// "assemble the Contraption," CR 725): the assembler puts the top Amount$
// (default 1) cards of their own Contraption deck onto the battlefield,
// each one dialed to a freshly chosen sprocket (1, 2 or 3).
//
// Ported from forge-game/src/main/java/forge/game/ability/effects/
// AssembleContraptionEffect.java's resolve, minus its DefinedContraption$
// branch: 0 real corpus lines carry it bare, and the 2 that do
// (basalt_gargoyle... no -- reassembling a specific already-assembled
// Contraption, Reassemble$ True) are a distinct "rewire this one's own
// dial" shape this port does not build (rejected below, GO-7/PORT-8's
// "reject, don't guess").
//
// DefinedAssembler$ is Java's default, host.isCreature() ? "Self" : "You"
// (AssembleContraptionEffect.java:75-77), or ReplacedCause, the card a
// ReplacementType$ AssembleContraption replaced (Steamflogger Boss's "it
// assembles two instead"). A card assembler assembles for its controller and
// is the replacement's cause; a player assembler leaves no cause card.
type assembleContraptionEffect struct{}

func (assembleContraptionEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectParams(a, "AssembleContraption", "DefinedContraption", "Reassemble"); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	assembler := "You"
	if source.Type().Has(cardtype.Creature) {
		assembler = "Self"
	}
	if v, ok := a.Params.Param("DefinedAssembler"); ok {
		assembler = v
	}
	player, cause := a.Controller, NoCard
	switch assembler {
	case "You":
	case "Self":
		cause = a.Source
	case "ReplacedCause":
		if a.replacing == nil || a.replacing.card == NoCard {
			return nil
		}
		cause = a.replacing.card
	default:
		return fmt.Errorf("engine: AssembleContraption: DefinedAssembler$ %q not resolvable yet", assembler)
	}
	if cause != NoCard {
		player = g.Card(cause).Controller()
	}

	amount := 1
	if v, ok := a.Params.Param("Amount"); ok {
		parsed, ok := resolveNamedAmount(g, a.Amounts, source, v)
		if !ok {
			return fmt.Errorf("engine: AssembleContraption: Amount$ %q is not resolvable", v)
		}
		amount = parsed
	}
	remember := hasParam(a, "Remember")

	var assembled []CardID
	for i := 0; i < amount; i++ {
		// AssembleContraptionEffect.java:135-137: each assemble is its own
		// replacement event.
		if g.assembleReplaced(controller, player, cause) {
			continue
		}
		deck := g.Zone(ContraptionDeck, player)
		cards := deck.Cards()
		if len(cards) == 0 {
			break
		}
		id := cards[0]
		g.moveByEffect(controller, id, Battlefield, 0, player, false)
		sprocket := controller.ChooseNumber(g, g.Card(id).Controller(), id, 1, 3)
		g.Card(id).Sprocket = sprocket
		assembled = append(assembled, id)
		if remember {
			source.Memory.Remember(CardEntity(id))
		}
	}
	g.checkChangesZoneAllTriggers(controller, assembled, ContraptionDeck, Battlefield)
	return nil
}
