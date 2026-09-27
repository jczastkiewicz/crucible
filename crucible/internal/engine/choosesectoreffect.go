package engine

//enginelint:allow id ability game control effecthelpers card condition parts

import "fmt"

// chooseSectorEffect is ChooseSectorEffect.java: the host's controller -- not
// the activator, card.getController() at ChooseSectorEffect.java:12 -- picks
// one of Space Beleren's three sectors, and the host records it
// (Card.setChosenSector). AILogic$ is an AI hint and Ultimate$ feeds only
// AchievementTracker.java:23, so neither changes how this resolves.
//
// Only the write is ported: the Creature.ChosenSector/DifferentSector reads
// (CardProperty.java:119-127) and the per-creature sector that
// CR 704.5u's state-based action assigns (GameAction.java:1801-1825) do not
// exist yet (port-log effects-choosesector.md).
//
// Ported from forge-game/src/main/java/forge/game/ability/effects/ChooseSectorEffect.java's resolve.
type chooseSectorEffect struct{}

func (chooseSectorEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectParams(a, "ChooseSector", "Condition"); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	// PlayerController.chooseSector's own fixed list and order
	// (PlayerController.java:255), built per call: a package-level slice
	// would be shared mutable state (GO-2).
	options := []string{"Alpha", "Beta", "Gamma"}
	i := controller.ChooseSector(g, source.Controller(), NoCard, options)
	if i < 0 || i >= len(options) {
		return fmt.Errorf("engine: ChooseSector: choice %d out of range", i)
	}
	source.Memory.SetChosenSector(options[i])
	return nil
}
