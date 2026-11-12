package crucible.oracle;

import forge.ai.LobbyPlayerAi;
import forge.game.Game;
import forge.game.player.Player;
import forge.game.player.PlayerController;

/**
 * Seats a Java AI under a {@link RecordingPlayerController}. No AIOption is
 * passed, so the AI never simulates (ADR-0062).
 */
final class RecordingLobbyPlayer extends LobbyPlayerAi {
    private final java.util.function.Supplier<ReplayRecorder> recorder;

    RecordingLobbyPlayer(String name, java.util.function.Supplier<ReplayRecorder> recorder) {
        super(name, null);
        setAiProfile("Default");
        this.recorder = recorder;
    }

    @Override
    public Player createIngamePlayer(Game game, int id) {
        Player p = new Player(getName(), game, id);
        p.setFirstController(new RecordingPlayerController(game, p, this, recorder));
        return p;
    }

    @Override
    public PlayerController createMindSlaveController(Player master, Player slave) {
        return new RecordingPlayerController(slave.getGame(), slave, this, recorder);
    }
}
