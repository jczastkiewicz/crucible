package crucible.oracle;

import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.CountDownLatch;

import com.google.gson.JsonArray;
import com.google.gson.JsonObject;

import forge.deck.Deck;
import forge.deck.io.DeckSerializer;
import forge.game.Game;
import forge.game.GameRules;
import forge.game.GameType;
import forge.game.Match;
import forge.game.player.RegisteredPlayer;
import forge.gui.GuiBase;
import forge.localinstance.properties.ForgePreferences.FPref;
import forge.model.FModel;
import forge.GuiDesktop;
import forge.util.MyRandom;
import forge.util.ThreadUtil;

/**
 * R21a: plays one Java AI-vs-AI game and writes its replay log (ADR-0059,
 * ADR-0062).
 *
 * Usage: GameRecorder <deck A .dck> <deck B .dck> <seed> <forge commit> <out .jsonl>.
 * Seat 0 plays deck A. The seed feeds MyRandom; the log, not the seed, is
 * what the Go replay reads, so a rerun need not reproduce it byte for byte.
 */
public final class GameRecorder {
    private GameRecorder() {
    }

    public static void main(String[] args) throws Exception {
        if (args.length != 5) {
            System.err.println("usage: GameRecorder <deck A> <deck B> <seed> <forge commit> <out .jsonl>");
            System.exit(2);
        }
        GuiBase.setInterface(new GuiDesktop());
        FModel.initialize(null, preferences -> {
            preferences.setPref(FPref.LOAD_CARD_SCRIPTS_LAZILY, false);
            preferences.setPref(FPref.UI_LANGUAGE, "en-US");
            return null;
        });
        long seed = Long.parseLong(args[2]);
        ReplayRecorder.RecordingRandom random = new ReplayRecorder.RecordingRandom(seed);
        MyRandom.setRandom(random);

        Deck[] decks = {DeckSerializer.fromFile(Path.of(args[0]).toFile()),
                DeckSerializer.fromFile(Path.of(args[1]).toFile())};
        ReplayRecorder[] holder = new ReplayRecorder[1];
        List<RegisteredPlayer> players = new ArrayList<>();
        for (int i = 0; i < decks.length; i++) {
            RegisteredPlayer rp = new RegisteredPlayer(decks[i]);
            rp.setPlayer(new RecordingLobbyPlayer("seat" + i, () -> holder[0]));
            players.add(rp);
        }
        Match match = new Match(new GameRules(GameType.Constructed), players, "R21a");
        Game game = match.createGame();
        game.setNoGUIUser();
        // ADR-0062: flat Monte Carlo rollouts would run on copied games.
        game.AI_FLAT_MONTE_CARLO = false;
        holder[0] = new ReplayRecorder(game);
        random.attach(holder[0]);

        Throwable[] failed = new Throwable[1];
        CountDownLatch done = new CountDownLatch(1);
        ThreadUtil.invokeInGameThread(() -> {
            try {
                match.startGame(game);
            } catch (Throwable t) {
                failed[0] = t;
            } finally {
                done.countDown();
            }
        });
        done.await();
        if (failed[0] != null) {
            failed[0].printStackTrace();
            System.exit(1);
        }

        JsonObject header = new JsonObject();
        header.addProperty("version", ReplayRecorder.FORMAT_VERSION);
        header.addProperty("forge", args[3]);
        header.addProperty("seed", seed);
        JsonArray seats = new JsonArray();
        for (int i = 0; i < decks.length; i++) {
            JsonObject s = new JsonObject();
            s.addProperty("seat", i);
            s.addProperty("deck", Path.of(args[i]).getFileName().toString());
            s.add("cards", holder[0].seatCards.get(i));
            seats.add(s);
        }
        header.add("seats", seats);
        header.addProperty("winner", game.getOutcome() == null || game.getOutcome().isDraw() ? -1
                : players.indexOf(game.getOutcome().getWinningPlayer()));
        header.addProperty("turns", game.getPhaseHandler().getTurn());
        holder[0].write(Path.of(args[4]), header);
        System.err.printf("recorded %s: %d turns, %d unresolved identities, %d unattributed draws%n", args[4],
                game.getPhaseHandler().getTurn(), holder[0].ids.unresolved, holder[0].unattributedDraws);
        // R21c maps every identity; one it cannot name makes the log unusable.
        System.exit(holder[0].ids.unresolved > 0 ? 3 : 0);
    }
}
