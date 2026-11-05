package crucible.oracle;

import java.util.List;
import java.util.function.BooleanSupplier;

import forge.LobbyPlayer;
import forge.game.Game;
import forge.game.player.PlaySpellAbility;
import forge.game.player.Player;
import forge.game.spellability.SpellAbility;
import forge.gamesimulationtests.util.PlayerControllerForTests;

/**
 * ADR-0060's controller: upstream's PlayerControllerForTests, which answers
 * all of PlayerController, with the overrides the L2 runner needs.
 *
 * At priority it passes, unless the run has reached its checkpoint (the Go
 * run's final turn and phase): then it throws {@link Checkpoint} so the runner
 * captures the state right there, after Java's state-based actions and with
 * the triggers they raised on the stack -- the point Go's harness stops at.
 */
final class ScenarioController extends PlayerControllerForTests {
    /** Thrown at the checkpoint's priority; the runner catches it and dumps. */
    static final class Checkpoint extends RuntimeException {
        private static final long serialVersionUID = 1L;

        Checkpoint() {
            super(null, null, false, false);
        }
    }

    private final BooleanSupplier atCheckpoint;

    ScenarioController(Game game, Player player, LobbyPlayer lobbyPlayer, BooleanSupplier atCheckpoint) {
        super(game, player, lobbyPlayer);
        this.atCheckpoint = atCheckpoint;
    }

    @Override
    public List<SpellAbility> chooseSpellAbilityToPlay() {
        if (atCheckpoint.getAsBoolean()) {
            throw new Checkpoint();
        }
        return null;
    }

    // Upstream's version plays only the abilities of a list of named cards
    // and throws for any other (a planeswalker's or a Saga's state trigger,
    // F6). Every one is played, as the game's own controllers do.
    @Override
    public void playSpellAbilityNoStack(SpellAbility effectSA, boolean mayChoseNewTargets) {
        PlaySpellAbility.playSpellAbilityNoStack(this, player, effectSA, !mayChoseNewTargets);
    }
}
