package crucible.oracle;

import java.util.function.BooleanSupplier;

import forge.LobbyPlayer;
import forge.game.Game;
import forge.game.player.IGameEntitiesFactory;
import forge.game.player.Player;
import forge.game.player.PlayerController;

/** Seats a player under a {@link ScenarioController} (LobbyPlayerForTests' shape). */
final class ScenarioLobbyPlayer extends LobbyPlayer implements IGameEntitiesFactory {
    private final BooleanSupplier atCheckpoint;
    private final ScenarioAnswers answers;
    private ScenarioController controller;

    ScenarioLobbyPlayer(String name, BooleanSupplier atCheckpoint, ScenarioAnswers answers) {
        super(name);
        this.atCheckpoint = atCheckpoint;
        this.answers = answers;
    }

    @Override
    public Player createIngamePlayer(Game game, int id) {
        Player p = new Player(getName(), game, id);
        controller = new ScenarioController(game, p, this, atCheckpoint, answers);
        p.setFirstController(controller);
        return p;
    }

    // The controlled player's decisions are still the slave's queued lines
    // (Go names them by the player who makes them).
    @Override
    public PlayerController createMindSlaveController(Player master, Player slave) {
        return new ScenarioController(slave.getGame(), slave, this, atCheckpoint, answers);
    }

    @Override
    public void hear(LobbyPlayer player, String message) {
    }
}
