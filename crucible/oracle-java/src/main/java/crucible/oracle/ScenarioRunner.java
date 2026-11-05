package crucible.oracle;

import java.io.PrintWriter;
import java.io.StringWriter;
import java.lang.reflect.Field;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.Random;
import java.util.Set;
import java.util.concurrent.CountDownLatch;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

import forge.GuiDesktop;
import forge.StaticData;
import forge.deck.Deck;
import forge.game.Game;
import forge.game.GameRules;
import forge.game.GameStage;
import forge.game.GameState;
import forge.game.GameType;
import forge.game.card.Card;
import forge.game.Match;
import forge.game.phase.PhaseHandler;
import forge.game.phase.PhaseType;
import forge.game.player.Player;
import forge.game.player.RegisteredPlayer;
import forge.gui.GuiBase;
import forge.localinstance.properties.ForgePreferences.FPref;
import forge.model.FModel;
import forge.util.MyRandom;
import forge.util.ThreadUtil;

/**
 * ADR-0060's L2 runner: plays one testdata/scenarios fixture on the Java engine
 * and prints the post-state as GameState text, for crucible/tools/l2diff to
 * compare with Go's fixture.Dump of the same run.
 *
 * Usage: ScenarioRunner <manifest> <out dir>. Each manifest line is a scenario
 * directory, the Go run's final turn and its phase, tab-separated (l2diff
 * emit); the runner steps Java's own loop (PhaseHandler.mainLoopStep) until it
 * reaches that point (S8, l2-verbs.md), and writes <name>.state,
 * <name>.unmapped (the lines no l2-verbs.md row maps, or a setup card
 * GameState cannot load) or <name>.error. One JVM
 * runs the whole manifest: loading the card database takes most of a run.
 */
public final class ScenarioRunner {
    private static final int MAX_STEPS = 100_000;
    private static final int DEFERRED_ID = 900_000;
    // The actions.log verbs l2-verbs.md maps; a scenario using any other is
    // reported unmapped and not run (ADR-0060).
    private static final Set<String> MAPPED = Set.of("startturn", "advance", "step");
    // A player seat in GameState text: a p<i> key prefix, the activeplayer
    // value, or the P<i> value of a player token (GameState.getPlayerString).
    // Never a counter type such as P1P1.
    // A setup zone line (GameState.ZONES) and its card list.
    private static final Pattern ZONE_LINE = Pattern.compile(
            "^(?:human|ai|p\\d+)(?:battlefield|hand|graveyard|library|exile|command|sideboard)=(.+)$");
    private static final Pattern SEAT = Pattern.compile(
            "(^p|^activeplayer=p|(?:Owner|EnchantingPlayer|Protector):P)(-?\\d+)");

    private ScenarioRunner() {
    }

    public static void main(String[] args) throws Exception {
        if (args.length != 2) {
            System.err.println("usage: ScenarioRunner <manifest> <out dir>");
            System.exit(2);
        }
        List<String> manifest = Files.readAllLines(Path.of(args[0]));
        Path out = Path.of(args[1]);
        Files.createDirectories(out);

        GuiBase.setInterface(new GuiDesktop());
        FModel.initialize(null, preferences -> {
            preferences.setPref(FPref.LOAD_CARD_SCRIPTS_LAZILY, false);
            preferences.setPref(FPref.UI_LANGUAGE, "en-US");
            return null;
        });
        for (String entry : manifest) {
            if (entry.isBlank()) {
                continue;
            }
            String[] f = entry.split("\t");
            Path dir = Path.of(f[0]);
            String name = dir.getFileName().toString();
            String[] result = runOnGameThread(dir, Integer.parseInt(f[1]), f[2]);
            Files.writeString(out.resolve(name + "." + result[0]), result[1]);
        }
        System.exit(0);
    }

    // GameAction.invoke runs state changes on Forge's game thread (ThreadUtil),
    // so each run goes there and the caller waits. The result is a kind -- state,
    // unmapped or error -- and its text.
    private static String[] runOnGameThread(Path dir, int turn, String phase) throws InterruptedException {
        String[][] result = new String[1][];
        CountDownLatch done = new CountDownLatch(1);
        ThreadUtil.invokeInGameThread(() -> {
            try {
                result[0] = run(dir, turn, phase);
            } catch (Throwable t) {
                StringWriter trace = new StringWriter();
                t.printStackTrace(new PrintWriter(trace));
                result[0] = new String[] {"error", trace.toString()};
            } finally {
                done.countDown();
            }
        });
        done.await();
        return result[0];
    }

    private static String[] run(Path dir, int turn, String phaseName) throws Exception {
        // PhaseType's constants read the Localizer FModel loads.
        PhaseType phase = PhaseType.smartValueOf(phaseName);

        List<String> actions = new ArrayList<>();
        for (String raw : Files.readAllLines(dir.resolve("actions.log"))) {
            String line = raw.strip();
            if (!line.isEmpty() && !line.startsWith("#")) {
                actions.add(line);
            }
        }

        List<String> unmapped = new ArrayList<>();
        for (String line : actions) {
            if (!MAPPED.contains(line.split("\\s+")[0])) {
                unmapped.add(line);
            }
        }
        if (!unmapped.isEmpty()) {
            return new String[] {"unmapped", String.join("\n", unmapped) + "\n"};
        }

        List<String> setupLines = new ArrayList<>(Files.readAllLines(dir.resolve("setup.state")));
        String lost = notInCommonCards(setupLines);
        if (lost != null) {
            return new String[] {"unmapped", "setup card " + lost + " is not in GameState's card pool\n"};
        }
        Target target = new Target(turn, phase);
        Game game = newGame(playerCount(setupLines), target);
        target.game = game;
        if (setupLines.stream().noneMatch(l -> l.startsWith("activeplayer="))) {
            // GameState.applyGameOnThread indexes the active player's state
            // and fails without one; Go's fixture.Load needs none. The first
            // startturn names the player Go starts (l2-verbs.md).
            actions.stream().filter(l -> l.startsWith("startturn ")).findFirst()
                    .ifPresent(l -> setupLines.add("activeplayer=" + l.split("\\s+")[1]));
        }
        Map<Integer, Integer> damage = deferDamage(setupLines);
        GameState setup = new GameState();
        setup.parse(setupLines);
        setup.applyToGame(game);
        if (markDamage(setup, damage)) {
            game.getAction().checkStateEffects(true);
        }
        // The seed TestScenarios gives javarand (ADR-0060).
        MyRandom.setRandom(new Random(1));

        for (String line : actions) {
            String[] f = line.split("\\s+");
            switch (f[0]) {
                case "startturn" -> {
                    // Go's Game.StartTurn begins turn 1 at Untap: Java's first
                    // turn, counted up from 0 by advanceToNextPhase.
                    Player p = player(game, f[1]);
                    game.getPhaseHandler().devModeSet(null, p, false, 0);
                    game.getPhaseHandler().setupFirstTurn(p, null);
                }
                // Checkpoints: Java's own loop reaches the phase Go walks to.
                case "advance", "step" -> {
                }
                default -> throw new IllegalStateException("unmapped: " + line);
            }
        }

        // S8: the checkpoint is the first priority Java gives at Go's final
        // turn and phase, after its state-based actions and with the triggers
        // they raised on the stack -- where Go's advance stops; after a final
        // step, the first one with the stack empty. Untap and Cleanup give
        // none (CR 502.4, 514.3), so there it is reaching them.
        PhaseHandler ph = game.getPhaseHandler();
        target.drain = !actions.isEmpty() && actions.get(actions.size() - 1).split("\\s+")[0].equals("step");
        target.armed = true;
        int steps = 0;
        try {
            while (!game.isGameOver()) {
                if (target.reached() && (ph.is(PhaseType.UNTAP) || ph.is(PhaseType.CLEANUP))) {
                    break;
                }
                if (ph.getTurn() > turn) {
                    throw new IllegalStateException("passed turn " + turn + " " + phase + " without a priority there");
                }
                if (++steps > MAX_STEPS) {
                    throw new IllegalStateException("never reached turn " + turn + " " + phase);
                }
                ph.mainLoopStep();
            }
        } catch (ScenarioController.Checkpoint reached) {
            // the controller stopped the loop at the checkpoint
        }

        GameState post = new GameState();
        post.initFromGame(game);
        return new String[] {"state", registeredIndexes(game, post.toString()) + crucibleKeys(game)};
    }

    // GameState.processCardsForZone looks a card up in the common cards only
    // (GameState.java:1305) and drops any other, a Scheme or a Plane, with
    // a line on stderr: the Java run would play a different board.
    private static String notInCommonCards(List<String> setupLines) {
        for (String line : setupLines) {
            Matcher m = ZONE_LINE.matcher(line);
            if (!m.matches()) {
                continue;
            }
            for (String card : m.group(1).split(";")) {
                String name = card.split("\\|")[0];
                if (!name.startsWith("t:") && !name.startsWith("T:") && StaticData.instance().getCommonCards().getCard(name) == null) {
                    return name;
                }
            }
        }
        return null;
    }

    // GameState.initFromGame numbers players by Game.getPlayers, which drops a
    // player who lost, so the survivors' p<i> shift. Each p<i> key prefix,
    // activeplayer value and P<i> token goes back to the registered seat;
    // the lost player's own keys are not written at all.
    private static String registeredIndexes(Game game, String text) {
        List<Player> in = game.getPlayers();
        List<Player> all = game.getRegisteredPlayers();
        StringBuilder sb = new StringBuilder();
        for (String line : text.split("\n", -1)) {
            if (line.startsWith("activeplayer=")) {
                // initFromGame writes p-1 when the active player has lost.
                Player turn = game.getPhaseHandler().getPlayerTurn();
                sb.append("activeplayer=p").append(all.indexOf(turn)).append('\n');
                continue;
            }
            Matcher m = SEAT.matcher(line);
            StringBuilder fixed = new StringBuilder();
            while (m.find()) {
                int i = Integer.parseInt(m.group(2));
                int seat = i >= 0 && i < in.size() ? all.indexOf(in.get(i)) : i;
                m.appendReplacement(fixed, m.group(1) + seat);
            }
            m.appendTail(fixed);
            sb.append(fixed).append('\n');
        }
        return sb.substring(0, sb.length() - 1);
    }

    // The Crucible-only keys fixture.Dump writes (game-state-fixture.md),
    // written here from Java's own model so they are compared, not dropped
    // (ADR-0060): over=, <player>lost=, <player>won=, monarch=, initiative=.
    private static String crucibleKeys(Game game) {
        StringBuilder sb = new StringBuilder();
        if (game.isGameOver()) {
            sb.append("over=true\n");
        }
        for (int i = 0; i < game.getRegisteredPlayers().size(); i++) {
            Player p = game.getRegisteredPlayers().get(i);
            if (p.hasLost()) {
                sb.append("p").append(i).append("lost=true\n");
            }
            // Player.hasWon reads an outcome that only means a win once the
            // game is over; Go writes won= only then.
            if (game.isGameOver() && p.hasWon()) {
                sb.append("p").append(i).append("won=true\n");
            }
        }
        if (game.getMonarch() != null) {
            sb.append("monarch=p").append(game.getRegisteredPlayers().indexOf(game.getMonarch())).append("\n");
        }
        if (game.getHasInitiative() != null) {
            sb.append("initiative=p").append(game.getRegisteredPlayers().indexOf(game.getHasInitiative())).append("\n");
        }
        return sb.toString();
    }

    // S11: a setup card's Damage: is marked after applyToGame, then the
    // runner runs the load-time checkStateEffects(true) once more.
    // GameState.applyGameOnThread marks it and runs that pass before its own
    // copyLastState, so no Moved replacement sees the lethal damage (Rest in
    // Peace's Bears reach the graveyard; the same board settles with the
    // replacement at the first priority). fixture.ApplyStateEffects runs the
    // pass with the replacements in effect. Each damaged card gets an Id:
    // handle if it has none, from DEFERRED_ID up.
    private static Map<Integer, Integer> deferDamage(List<String> setupLines) {
        Map<Integer, Integer> damage = new HashMap<>();
        int next = DEFERRED_ID;
        for (int i = 0; i < setupLines.size(); i++) {
            String line = setupLines.get(i);
            int eq = line.indexOf('=');
            if (eq < 0 || !line.contains("Damage:")) {
                continue;
            }
            List<String> cards = new ArrayList<>();
            for (String card : line.substring(eq + 1).split(";")) {
                List<String> toks = new ArrayList<>(List.of(card.split("\\|")));
                Integer dmg = null;
                Integer id = null;
                for (String t : toks) {
                    if (t.startsWith("Damage:")) {
                        dmg = Integer.parseInt(t.substring(7));
                    } else if (t.startsWith("Id:")) {
                        id = Integer.parseInt(t.substring(3));
                    }
                }
                if (dmg != null) {
                    toks.removeIf(t -> t.startsWith("Damage:"));
                    if (id == null) {
                        id = next++;
                        toks.add("Id:" + id);
                    }
                    damage.put(id, dmg);
                }
                cards.add(String.join("|", toks));
            }
            setupLines.set(i, line.substring(0, eq + 1) + String.join(";", cards));
        }
        return damage;
    }

    // Reads GameState's private idToCard (ADR-0060 allows reflection in the
    // runner; a renamed field fails here, loudly).
    @SuppressWarnings("unchecked")
    private static boolean markDamage(GameState setup, Map<Integer, Integer> damage) throws ReflectiveOperationException {
        if (damage.isEmpty()) {
            return false;
        }
        Field f = GameState.class.getDeclaredField("idToCard");
        f.setAccessible(true);
        Map<Integer, Card> idToCard = (Map<Integer, Card>) f.get(setup);
        for (Map.Entry<Integer, Integer> e : damage.entrySet()) {
            idToCard.get(e.getKey()).setDamage(e.getValue());
        }
        return true;
    }

    // Two seats, or more when the setup names a p<i> slot past them
    // (fixture.playerSlot's p2, p3).
    private static int playerCount(List<String> setupLines) {
        int n = 2;
        for (String l : setupLines) {
            if (l.length() > 2 && l.charAt(0) == 'p' && Character.isDigit(l.charAt(1))) {
                n = Math.max(n, l.charAt(1) - '0' + 1);
            }
        }
        return n;
    }

    // The run's checkpoint, which the controllers read at each priority.
    // Armed once the actions are applied, so setup and startturn never stop.
    private static final class Target {
        final int turn;
        final PhaseType phase;
        Game game;
        boolean armed;
        // Go's step plays the step's priority round out (engine.Game.Step):
        // after a final step the checkpoint waits for an empty stack.
        boolean drain;

        Target(int turn, PhaseType phase) {
            this.turn = turn;
            this.phase = phase;
        }

        boolean reached() {
            PhaseHandler ph = game.getPhaseHandler();
            return armed && ph.getTurn() == turn && ph.is(phase) && (!drain || game.getStack().isEmpty());
        }
    }

    private static Game newGame(int seats, Target target) {
        List<RegisteredPlayer> players = new ArrayList<>();
        for (int i = 0; i < seats; i++) {
            String name = i == 0 ? "human" : i == 1 ? "ai" : "p" + i;
            RegisteredPlayer rp = new RegisteredPlayer(new Deck(name));
            rp.setPlayer(new ScenarioLobbyPlayer(name, target::reached));
            players.add(rp);
        }
        Match match = new Match(new GameRules(GameType.Constructed), players, "L2");
        Game game = match.createGame();
        game.setAge(GameStage.Play);
        return game;
    }

    // By registered seat: getPlayers drops a player who lost, and
    // applyGameOnThread's late setLife can make one lose at load.
    private static Player player(Game game, String name) {
        return switch (name) {
            case "human" -> game.getRegisteredPlayers().get(0);
            case "ai" -> game.getRegisteredPlayers().get(1);
            default -> throw new IllegalStateException("player " + name);
        };
    }
}
