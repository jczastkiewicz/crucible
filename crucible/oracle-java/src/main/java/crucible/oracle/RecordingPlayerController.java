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

    // The calls that pay a cost, and which argument is the paying ability:
    // payments made while an ability resolves ("unless" costs, a mana cost an
    // effect asks for) are not the played ability's.
    private static final java.util.Map<String, Integer> PAYS = java.util.Map.of("payManaCost", 2,
            "payCostToPreventEffect", 1, "payCombatCost", 2, "payCostDuringRoll", 1);

    // The paying ability's mana-ability count at each open pay call.
    private final java.util.ArrayDeque<Integer> payMark = new java.util.ArrayDeque<>();

    @Override
    void before(String method, Object[] args) {
        Object[] offered = args;
        if (method.equals("chooseSpellAbilityToPlay")) {
            // The call takes no arguments: the options are what the seat can
            // play now, as the AI's own enumeration finds them.
            offered = new Object[] {playable()};
        }
        JsonObject r = recorder.get().call(getPlayer(), method, offered);
        open.push(r == null ? new JsonObject() : r);
        if (PAYS.containsKey(method)) {
            SpellAbility sa = (SpellAbility) args[PAYS.get(method)];
            payMark.push(sa == null ? 0 : sa.getPayingManaAbilities().size());
        }
    }

    @Override
    <R> R after(String method, Object[] args, R answer) {
        JsonObject r = open.pop();
        JsonObject rec = r.has("seq") ? r : null;
        recorder.get().answer(rec, answer, args);
        int seq = rec == null ? -1 : rec.get("seq").getAsInt();
        if (PLAYS.contains(method)) {
            for (SpellAbility sa : played(method, args)) {
                recorder.get().play(getPlayer(), sa, seq);
            }
        }
        if (PAYS.containsKey(method)) {
            int mark = payMark.pop();
            recorder.get().payment(getPlayer(), (SpellAbility) args[PAYS.get(method)], mark, seq);
        }
        return answer;
    }

    // ComputerUtilAbility.getSpellAbilities' cards and abilities, mana
    // abilities included: the AI does answer one at priority (Urza's Saga's
    // granted {T}: Add {C}). getAllPossibleAbilities sets each ability's
    // activating player to this seat, as the AI's own enumeration does next.
    private List<SpellAbility> playable() {
        List<SpellAbility> out = new java.util.ArrayList<>();
        for (forge.game.card.Card c : forge.ai.ComputerUtilAbility.getAvailableCards(getGame(), getPlayer())) {
            out.addAll(c.getAllPossibleAbilities(getPlayer(), true));
        }
        return out;
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
