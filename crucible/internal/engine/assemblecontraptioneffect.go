package engine

//enginelint:allow ability amount card condition control effecthelpers game id memory parts zone zonemove

import "fmt"

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
// DefinedAssembler$'s own real corpus values are Self (2 lines) and
// ReplacedCause (1, a ReplacementType$ AssembleContraption's own "and
// assembles two instead" rewrite -- replacement types this port does not
// dispatch by name, effecthelpers.go's own gap); every other real line
// (20 of 23) names none and gets Java's own default, host.isCreature() ?
// "Self" : "You". Both resolve to the same player here: the ability's own
// Controller (a.Controller) is always the creature/spell's own controller
// at the one point this effect runs (an ETB/damage/cast trigger on the
// assembling permanent itself, or the spell's own activator) -- ValidTgts$-
// shaped assemblers (letting a different player's creature assemble) are 0
// real corpus lines, so "You" and "Self" never need to diverge here.
type assembleContraptionEffect struct{}

func (assembleContraptionEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectParams(a, "AssembleContraption", "DefinedContraption", "Reassemble"); err != nil {
		return err
	}
	if assembler, ok := a.Params.Param("DefinedAssembler"); ok && assembler != "Self" && assembler != "You" {
		return fmt.Errorf("engine: AssembleContraption: DefinedAssembler$ %q not resolvable yet", assembler)
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}

	amount := 1
	if v, ok := a.Params.Param("Amount"); ok {
		if v == "Result" {
			return fmt.Errorf("engine: AssembleContraption: Amount$ Result not resolvable yet")
		}
		parsed, ok := resolveNamedAmount(g, a.Amounts, source, v)
		if !ok {
			return fmt.Errorf("engine: AssembleContraption: Amount$ %q is not resolvable", v)
		}
		amount = parsed
	}
	remember := hasParam(a, "Remember")

	player := a.Controller
	var assembled []CardID
	for i := 0; i < amount; i++ {
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
