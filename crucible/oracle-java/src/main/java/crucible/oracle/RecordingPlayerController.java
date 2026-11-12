package crucible.oracle;

import java.util.List;
import java.util.Set;

import com.google.gson.JsonObject;

import forge.LobbyPlayer;
import forge.game.Game;
import forge.game.card.CardCollectionView;
import forge.game.player.Player;
import forge.game.spellability.SpellAbility;

/**
 * R21a's RecordingPlayerController: the Java AI, with every PlayerController
 * call logged (ADR-0059) and, after each call that makes the AI play
 * something, a play record of the choices it made inside forge-ai (ADR-0062).
 */
final class RecordingPlayerController extends RecordingControllerBase {
    // ADR-0062: the calls after which the played ability holds the AI's own
    // choices, and which argument is that ability.
    private static final Set<String> PLAYS = Set.of("playChosenSpellAbility", "playSpellAbilityNoStack",
            "playSaFromPlayEffect", "chooseTargetsFor", "playTrigger", "orderAndPlaySimultaneousSa");

    // Resolved at each call: Game creates its players before the recorder,
    // which needs the game, exists.
    private final java.util.function.Supplier<ReplayRecorder> recorder;
    // The open records, innermost last: a call nests inside another when the
    // AI asks its own controller while answering.
    private final java.util.ArrayDeque<JsonObject> open = new java.util.ArrayDeque<>();

    RecordingPlayerController(Game game, Player p, LobbyPlayer lp,
            java.util.function.Supplier<ReplayRecorder> recorder) {
        super(game, p, lp);
        this.recorder = recorder;
    }

    @Override
    void before(String method, Object[] args) {
        JsonObject r = recorder.get().call(getPlayer(), method, args);
        open.push(r == null ? new JsonObject() : r);
    }

    @Override
    <R> R after(String method, Object[] args, R answer) {
        JsonObject r = open.pop();
        recorder.get().answer(r.has("seq") ? r : null, answer, args);
        if (PLAYS.contains(method)) {
            for (SpellAbility sa : played(method, args)) {
                recorder.get().play(getPlayer(), sa);
            }
        }
        return answer;
    }

    private static List<SpellAbility> played(String method, Object[] args) {
        return switch (method) {
            case "playTrigger" -> List.of((SpellAbility) args[1]);
            case "orderAndPlaySimultaneousSa" -> List.copyOf((List<SpellAbility>) args[0]);
            default -> args[0] == null ? List.of() : List.of((SpellAbility) args[0]);
        };
    }

    // Not abstract in PlayerController, so not generated: Player.shuffle
    // passes the shuffled library through it, and its answer is the
    // shuffle's outcome (ADR-0059).
    @Override
    public CardCollectionView cheatShuffle(CardCollectionView list) {
        CardCollectionView order = super.cheatShuffle(list);
        recorder.get().shuffle(getPlayer(), order);
        return order;
    }
}
