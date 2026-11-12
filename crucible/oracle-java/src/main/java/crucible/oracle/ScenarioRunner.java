package crucible.oracle;

import java.io.PrintWriter;
import java.io.StringWriter;
import java.lang.reflect.Field;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.Arrays;
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
import forge.game.zone.ZoneType;
import forge.game.mulligan.MulliganService;
import forge.util.Aggregates;
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
 * directory, the Go run's final turn and its phase, and the file with each
 * actions.log line after the turn and phase Go ran it in, tab-separated
 * (l2diff emit); the runner steps Java's own loop (PhaseHandler.mainLoopStep) until it
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
    private static final Set<String> MAPPED = Set.of("startturn", "advance", "step", "queue",
            "declareattackers", "declareblockers", "firststrikedamage", "combatdamage", "resolvestack",
            "tapformana", "castspell", "playland", "passpriority", "expecterror", "paymanacost", "dealopeninghands",
            "mulligan", "foretell", "plot", "suspend", "turnfaceup", "concede", "run");
    // The lines a seat makes at its priority, where Go ran them (S13).
    private static final Set<String> PLAYS = Set.of("tapformana", "castspell", "playland", "foretell", "plot", "suspend",
            "turnfaceup", "concede");
    // The special actions among them (S26), which a refused expecterror does not cover.
    static final Set<String> SPECIAL = Set.of("foretell", "plot", "suspend", "turnfaceup");
    // A player seat in GameState text: a p<i> key prefix, the activeplayer
    // value, or the P<i> value of a player token (GameState.getPlayerString).
    // Never a counter type such as P1P1.
    // A setup zone line (GameState.ZONES) and its card list.
    private static final Pattern ZONE_LINE = Pattern.compile(
            "^(?:human|ai|p\\d+)(?:battlefield|hand|graveyard|library|exile|command|sideboard)=(.+)$");
    private static final Pattern BATTLEFIELD_LINE = Pattern.compile("^p(\\d+)battlefield=(.*)$");
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
            String[] result = runOnGameThread(dir, Integer.parseInt(f[1]), f[2], Path.of(f[3]));
            Files.writeString(out.resolve(name + "." + result[0]), result[1]);
        }
        System.exit(0);
    }

    // GameAction.invoke runs state changes on Forge's game thread (ThreadUtil),
    // so each run goes there and the caller waits. The result is a kind -- state,
    // unmapped or error -- and its text.
    private static String[] runOnGameThread(Path dir, int turn, String phase, Path timed) throws InterruptedException {
        String[][] result = new String[1][];
        CountDownLatch done = new CountDownLatch(1);
        ThreadUtil.invokeInGameThread(() -> {
            try {
                result[0] = run(dir, turn, phase, timed);
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

    // The turn and phase of the priority round that takes the action queued
    // at line i: the next passpriority's, or the step a step line enters,
    // then that line's index. Null when no round takes it.
    private static String[] roundAfter(List<String> actions, List<String[]> when, int i) {
        for (int j = i + 1; j < actions.size(); j++) {
            String verb = actions.get(j).split("\\s+")[0];
            if (verb.equals("passpriority")) {
                return new String[] {when.get(j)[0], when.get(j)[1], String.valueOf(j)};
            }
            if (verb.equals("step")) {
                // The step it enters, the one after where it ran.
                PhaseType from = PhaseType.smartValueOf(when.get(j)[1]);
                int t = Integer.parseInt(when.get(j)[0]);
                return from == PhaseType.CLEANUP ? new String[] {String.valueOf(t + 1), PhaseType.UNTAP.name(), String.valueOf(j)}
                        : new String[] {String.valueOf(t), PhaseType.values()[from.ordinal() + 1].name(), String.valueOf(j)};
            }
        }
        return null;
    }

    // timed is l2diff emit's actions file: each actions.log line after the
    // turn and phase Go ran it in.
    private static String[] run(Path dir, int turn, String phaseName, Path timed) throws Exception {
        // PhaseType's constants read the Localizer FModel loads.
        PhaseType phase = PhaseType.smartValueOf(phaseName);

        List<String> actions = new ArrayList<>();
        List<String[]> when = new ArrayList<>();
        for (String raw : Files.readAllLines(timed)) {
            if (!raw.isBlank()) {
                String[] t = raw.split("\t", 3);
                when.add(t);
                actions.add(t[2]);
            }
        }

        List<String> unmapped = new ArrayList<>();
        ScenarioAnswers answers = new ScenarioAnswers();
        for (int i = 0; i < actions.size(); i++) {
            String line = actions.get(i);
            String[] f = line.split("\\s+", 3);
            try {
                if (f[0].equals("queue") && f.length == 3 && f[1].equals("action")) {
                    // S15: Go takes it at the next priority round, a
                    // passpriority's or the one a step plays in the step it
                    // enters.
                    String[] at = roundAfter(actions, when, i);
                    if (at == null) {
                        unmapped.add(line);
                    } else {
                        answers.action(i, f[2].split("\\s+"), Integer.parseInt(at[2]), Integer.parseInt(at[0]),
                                PhaseType.smartValueOf(at[1]));
                    }
                } else if (!MAPPED.contains(f[0]) || f[0].equals("queue") && (f.length < 3 || !ScenarioAnswers.KINDS.contains(f[1]))) {
                    unmapped.add(line);
                } else if (f[0].equals("queue") && Set.of("startingplayer", "keephand", "tuck").contains(f[1])
                        && actions.subList(0, i).stream().anyMatch(l -> l.startsWith("startturn "))) {
                    // S24: an opening-hand answer after the game started is
                    // for a startGame inside it (a subgame, a restart), which
                    // Java plays on its own.
                    unmapped.add(line);
                } else if (f[0].equals("startturn") && actions.subList(0, i).stream().anyMatch(l -> l.startsWith("startturn "))) {
                    // A second startturn restarts Go's turn mid-game; Java
                    // sets up its first turn once (PhaseHandler.setupFirstTurn).
                    unmapped.add(line);
                } else if ((PLAYS.contains(f[0]) || f[0].equals("expecterror") || f[0].equals("paymanacost"))
                        && when.get(i)[0].equals("0")) {
                    // S19: before any startturn there is no turn, so no
                    // priority to make a play at; Go's harness acts anyway.
                    unmapped.add(line);
                } else if ((PLAYS.contains(f[0]) || f[0].equals("expecterror")) && when.get(i)[0].equals("1")
                        && when.get(i)[1].equals("DRAW")) {
                    // S18: Go's advance walks through the first player's
                    // skipped draw step (CR 103.8a); Java skips it, so a play
                    // there has no priority to be made at.
                    unmapped.add(line);
                } else if (f[0].equals("expecterror")) {
                    // S17: only a refused cast or land play is mapped.
                    String[] all = line.split("\\s+");
                    if (all.length == 3 && (all[2].equals("declareattackers") || all[2].equals("declareblockers"))) {
                        // S23: Go refused the declaration queued last.
                        answers.refuseDeclaration(all[2]);
                    } else if (all.length != 5 || !all[1].equals("refused") || !PLAYS.contains(all[2]) || all[2].equals("tapformana")
                            || SPECIAL.contains(all[2]) || all[2].equals("concede")) {
                        unmapped.add(line);
                    } else {
                        answers.refusedPlay(i, all[2], Arrays.copyOfRange(all, 3, all.length), Integer.parseInt(when.get(i)[0]),
                                PhaseType.smartValueOf(when.get(i)[1]));
                    }
                } else if (f[0].equals("paymanacost")) {
                    String[] all = line.split("\\s+");
                    answers.payManaCost(i, Arrays.copyOfRange(all, 1, all.length), Integer.parseInt(when.get(i)[0]),
                            PhaseType.smartValueOf(when.get(i)[1]));
                } else if (f[0].equals("queue")) {
                    answers.queue(f[1], f[2]);
                } else if (f[0].equals("resolvestack") && when.get(i)[1].equals("UNTAP")) {
                    // S14: Go's harness resolves where Java gives no priority
                    // (CR 502.4); Java's triggers wait for the upkeep. Cleanup
                    // gives priority once a trigger or state-based action
                    // happens there (CR 514.3a, PhaseHandler.java:428-452).
                    unmapped.add(line);
                } else if (f[0].equals("resolvestack") || f[0].equals("passpriority")) {
                    answers.resolveStack();
                } else if (PLAYS.contains(f[0])) {
                    String[] all = line.split("\\s+");
                    answers.play(i, f[0], Arrays.copyOfRange(all, 1, all.length), Integer.parseInt(when.get(i)[0]),
                            PhaseType.smartValueOf(when.get(i)[1]));
                }
            } catch (IllegalArgumentException e) {
                unmapped.add(line);
            }
        }
        if (!unmapped.isEmpty()) {
            return new String[] {"unmapped", String.join("\n", unmapped) + "\n"};
        }

        List<String> setupLines = new ArrayList<>(Files.readAllLines(dir.resolve("setup.state")));
        String lost = notInCardPools(setupLines);
        if (lost != null) {
            return new String[] {"unmapped", "setup card " + lost + " is not in GameState's card pool\n"};
        }
        Target target = new Target(turn, phase, answers);
        Game game = newGame(playerCount(setupLines), target, answers);
        target.game = game;
        if (setupLines.stream().noneMatch(l -> l.startsWith("activeplayer="))) {
            // GameState.applyGameOnThread indexes the active player's state
            // and fails without one; Go's fixture.Load needs none. The first
            // startturn names the player Go starts (l2-verbs.md).
            // With none (S24), the first seat: the game never starts.
            setupLines.add("activeplayer=" + actions.stream().filter(l -> l.startsWith("startturn ")).findFirst()
                    .map(l -> l.split("\\s+")[1]).orElse("human"));
        }
        Map<Integer, Integer> damage = deferDamage(setupLines);
        GameState setup = new GameState();
        setup.parse(setupLines);
        answers.attach(game, idToCard(setup));
        setup.applyToGame(game);
        answers.bind();
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
                // S24: Go's DealOpeningHands, GameAction.startGame's first
                // steps: a random player (or, after a game, its loser) picks
                // who goes first, then each player shuffles and draws.
                case "dealopeninghands" -> {
                    List<Player> players = game.getPlayers();
                    boolean after = f.length == 3 && f[1].equals("after");
                    Player decider = after ? player(game, f[2]) : Aggregates.random(players);
                    decider.getController().chooseStartingPlayer(!after);
                    for (Player p : players) {
                        p.getZone(ZoneType.Library).shuffle();
                        p.drawCards(p.getStartingHandSize());
                    }
                }
                case "mulligan" -> new MulliganService(player(game, f[1])).perform();
                // Checkpoints: Java's own loop reaches the phase Go walks to,
                // asking the controllers for the queued answers on the way.
                case "advance", "step", "queue", "declareattackers", "declareblockers", "firststrikedamage",
                        "combatdamage", "resolvestack", "tapformana", "castspell", "playland", "passpriority",
                        "expecterror", "paymanacost", "foretell", "plot", "suspend", "turnfaceup", "concede",
                        "run" -> {
                }
                default -> throw new IllegalStateException("unmapped: " + line);
            }
        }

        // S8: the checkpoint is the first priority Java gives at Go's final
        // turn and phase, after its state-based actions and with the triggers
        // they raised on the stack -- where Go's advance stops; after a final
        // step, the first one with the stack empty. In a step that gives none
        // -- Untap, Cleanup, a damage step with no damage (CR 502.4, 514.3,
        // PhaseHandler.java:321-344) -- it is reaching the step.
        PhaseHandler ph = game.getPhaseHandler();
        target.drain = !actions.isEmpty()
                && Set.of("step", "resolvestack", "passpriority").contains(actions.get(actions.size() - 1).split("\\s+")[0]);
        target.armed = true;
        int steps = 0;
        try {
            // S24: a scenario that never starts a turn ends before the game
            // does; Java's loop would start it.
            while (turn > 0 && !game.isGameOver()) {
                if (target.reachedStep() && !givesPriority(ph)) {
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
        // A card choice is popped only when it fits a question (S16); an
        // answer left over means Java never asked what Go did.
        answers.checkAnswersUsed();
        if (turn == 0) {
            // S24: GameState.initFromGame writes the phase, which a game that
            // never started has none of.
            ph.devModeSet(PhaseType.UNTAP, ph.getPlayerTurn(), false, 0);
        }

        GameState post = new GameState();
        post.initFromGame(game);
        String text = registeredIndexes(game, post.toString());
        if (turn == 0) {
            // S24: the turn, active player and phase of a game that never
            // started are the runner's, set for initFromGame; Go writes none.
            text = text.replaceAll("(?m)^(turn|activeplayer|activephase)=.*\\R", "");
        }
        return new String[] {"state", withProtectors(game, text) + crucibleKeys(game)};
    }

    // GameState.processCardsForZone looks a card up in the common cards, then
    // the variant cards (GameState.java:1306-1309, upstream-patches.md), and
    // drops any other with a line on stderr: the Java run would play a
    // different board.
    private static String notInCardPools(List<String> setupLines) {
        for (String line : setupLines) {
            Matcher m = ZONE_LINE.matcher(line);
            if (!m.matches()) {
                continue;
            }
            for (String card : m.group(1).split(";")) {
                String name = card.split("\\|")[0];
                if (!name.startsWith("t:") && !name.startsWith("T:") && StaticData.instance().getCommonCards().getCard(name) == null
                        && StaticData.instance().getVariantCards().getCard(name) == null) {
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

    // A battle's Protector:P<seat>, Crucible-only like crucibleKeys' (Go's
    // Card.ProtectingPlayer; GameState.toString writes none). The card is
    // found by name among its controller's battles, so a name two of them
    // share is left without one and shows as a diff.
    private static String withProtectors(Game game, String text) {
        List<Player> all = game.getRegisteredPlayers();
        StringBuilder sb = new StringBuilder();
        for (String line : text.split("\n", -1)) {
            Matcher m = BATTLEFIELD_LINE.matcher(line);
            if (m.matches() && !m.group(2).isEmpty()) {
                Player controller = all.get(Integer.parseInt(m.group(1)));
                List<String> cards = new ArrayList<>();
                for (String card : m.group(2).split(";")) {
                    String name = card.split("\\|")[0];
                    List<Card> battles = controller.getCardsIn(ZoneType.Battlefield).stream()
                            .filter(c -> c.isBattle() && c.getName().equals(name)).toList();
                    if (battles.size() == 1 && battles.get(0).getProtectingPlayer() != null) {
                        card += "|Protector:P" + all.indexOf(battles.get(0).getProtectingPlayer());
                    }
                    cards.add(card);
                }
                line = "p" + m.group(1) + "battlefield=" + String.join(";", cards);
            }
            sb.append(line).append('\n');
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
            // The game's own outcome (Game.setGameOver): Player.hasWon also
            // asks cantWin, so a player left alone under "you can't win"
            // (Abyssal Persecutor) would read as no winner of a game Java
            // ended with AllOpponentsLost. Go writes won= only at game end.
            if (game.isGameOver() && game.getOutcome() != null && game.getOutcome().isWinner(p.getRegisteredPlayer())) {
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
    private static boolean markDamage(GameState setup, Map<Integer, Integer> damage) throws ReflectiveOperationException {
        if (damage.isEmpty()) {
            return false;
        }
        Map<Integer, Card> idToCard = idToCard(setup);
        for (Map.Entry<Integer, Integer> e : damage.entrySet()) {
            idToCard.get(e.getKey()).setDamage(e.getValue());
        }
        return true;
    }

    // PhaseHandler.givePriorityToPlayer, private: whether the step just
    // begun gives priority (onPhaseBegin clears it for one that does not).
    private static boolean givesPriority(PhaseHandler ph) throws ReflectiveOperationException {
        Field f = PhaseHandler.class.getDeclaredField("givePriorityToPlayer");
        f.setAccessible(true);
        return f.getBoolean(ph);
    }

    // GameState's setup Id: handle to card map, private (ADR-0060).
    @SuppressWarnings("unchecked")
    private static Map<Integer, Card> idToCard(GameState setup) throws ReflectiveOperationException {
        Field f = GameState.class.getDeclaredField("idToCard");
        f.setAccessible(true);
        return (Map<Integer, Card>) f.get(setup);
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
        final ScenarioAnswers answers;
        Game game;
        boolean armed;
        // Go's step plays the step's priority round out (engine.Game.Step):
        // after a final step the checkpoint waits for an empty stack.
        boolean drain;

        Target(int turn, PhaseType phase, ScenarioAnswers answers) {
            this.turn = turn;
            this.phase = phase;
            this.answers = answers;
        }

        // At a priority: every queued action taken too.
        boolean reached() {
            return reachedStep() && answers.actionsDone();
        }

        // In a step without priority (S15): Go's queue keeps what no round
        // took.
        boolean reachedStep() {
            PhaseHandler ph = game.getPhaseHandler();
            return armed && answers.playsDone() && ph.getTurn() == turn && ph.is(phase)
                    && (!drain || game.getStack().isEmpty());
        }
    }

    private static Game newGame(int seats, Target target, ScenarioAnswers answers) {
        List<RegisteredPlayer> players = new ArrayList<>();
        for (int i = 0; i < seats; i++) {
            String name = i == 0 ? "human" : i == 1 ? "ai" : "p" + i;
            RegisteredPlayer rp = new RegisteredPlayer(new Deck(name));
            rp.setPlayer(new ScenarioLobbyPlayer(name, target::reached, answers));
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
