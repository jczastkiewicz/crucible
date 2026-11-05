package crucible.oracle;

import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.function.BooleanSupplier;

import forge.LobbyPlayer;
import forge.game.Game;
import forge.game.GameEntity;
import forge.game.card.Card;
import forge.game.card.CardCollectionView;
import forge.game.combat.Combat;
import forge.game.combat.CombatUtil;
import forge.game.keyword.Keyword;
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
 * Combat declarations pop the queued answers ({@link ScenarioAnswers}); with
 * none queued the seat declares nothing.
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
    private final ScenarioAnswers answers;

    ScenarioController(Game game, Player player, LobbyPlayer lobbyPlayer, BooleanSupplier atCheckpoint,
            ScenarioAnswers answers) {
        super(game, player, lobbyPlayer);
        this.atCheckpoint = atCheckpoint;
        this.answers = answers;
    }

    @Override
    public List<SpellAbility> chooseSpellAbilityToPlay() {
        if (atCheckpoint.getAsBoolean()) {
            throw new Checkpoint();
        }
        return null;
    }

    // Go's DeclareCombatAttackers: each queued attacker attacks the only
    // defender there is, or the next queued attack target when there are
    // several (attack.go chooseAttackTargets).
    @Override
    public void declareAttackers(Player attacker, Combat combat) {
        List<Integer> ids = answers.nextAttackers();
        if (ids == null) {
            return;
        }
        for (int id : ids) {
            List<GameEntity> defenders = List.copyOf(combat.getDefenders());
            GameEntity defender = defenders.size() == 1 ? defenders.get(0) : answers.nextAttackTarget();
            combat.addAttacker(answers.card(id), defender);
        }
        if (!CombatUtil.validateAttackers(combat)) {
            throw new IllegalStateException("illegal attack declaration " + ids);
        }
    }

    // Go's DeclareCombatBlockers: the queued blocker=attacker pairs.
    @Override
    public void declareBlockers(Player defender, Combat combat) {
        List<int[]> pairs = answers.nextBlocks();
        if (pairs == null) {
            return;
        }
        for (int[] pair : pairs) {
            Card blocker = answers.card(pair[0]);
            Card attacker = answers.card(pair[1]);
            if (!CombatUtil.canBlock(attacker, blocker)) {
                throw new IllegalStateException(blocker + " can't block " + attacker);
            }
            combat.addBlocker(attacker, blocker);
        }
        String invalid = CombatUtil.validateBlocks(combat, defender);
        if (invalid != null) {
            throw new IllegalStateException(invalid);
        }
    }

    // Go's dealAttackerDamage (combatdamage.go): a lone blocker takes all of
    // it, or with trample the lethal part and the rest goes to what is
    // attacked (the null key, Combat.assignAttackersDamage). Dividing among
    // several is a queued damage answer, not mapped yet.
    @Override
    public Map<Card, Integer> assignCombatDamage(Card attacker, CardCollectionView blockers, CardCollectionView remaining,
            int damageDealt, GameEntity defender, boolean overrideOrder) {
        if (blockers.size() != 1) {
            throw new IllegalStateException("dividing " + attacker + "'s damage among " + blockers + " needs queue damage");
        }
        Card blocker = blockers.get(0);
        int toBlocker = damageDealt;
        // The blocker's half (Combat.assignBlockersDamage) has no trample.
        if (attacker.hasKeyword(Keyword.TRAMPLE) && attacker.isAttacking()) {
            int lethal = attacker.hasKeyword(Keyword.DEATHTOUCH) ? 1 : Math.max(0, blocker.getLethalDamage());
            toBlocker = Math.min(lethal, damageDealt);
        }
        Map<Card, Integer> map = new HashMap<>();
        map.put(blocker, toBlocker);
        if (damageDealt > toBlocker) {
            map.put(null, damageDealt - toBlocker);
        }
        return map;
    }

    // Upstream's version plays only the abilities of a list of named cards
    // and throws for any other (a planeswalker's or a Saga's state trigger,
    // F6). Every one is played, as the game's own controllers do.
    @Override
    public void playSpellAbilityNoStack(SpellAbility effectSA, boolean mayChoseNewTargets) {
        PlaySpellAbility.playSpellAbilityNoStack(this, player, effectSA, !mayChoseNewTargets);
    }
}
