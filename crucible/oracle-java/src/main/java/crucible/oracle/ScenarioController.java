package crucible.oracle;

import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.function.BooleanSupplier;
import java.util.function.Predicate;

import forge.LobbyPlayer;
import forge.card.ICardFace;
import forge.game.Game;
import forge.game.GameEntity;
import forge.game.GameObject;
import forge.game.card.Card;
import forge.game.card.CardCollectionView;
import forge.game.combat.Combat;
import forge.game.combat.CombatUtil;
import forge.game.keyword.Keyword;
import forge.card.mana.ManaCost;
import forge.game.cost.CostPartMana;
import forge.game.mana.ManaConversionMatrix;
import forge.game.mana.ManaCostBeingPaid;
import forge.game.player.PlaySpellAbility;
import forge.game.spellability.AbilityManaPart;
import forge.game.player.Player;
import forge.game.spellability.SpellAbility;
import forge.game.spellability.SpellPermanent;
import forge.game.spellability.TargetRestrictions;
import forge.game.trigger.WrappedAbility;
import forge.game.zone.ZoneType;
import forge.util.ITriggerEvent;
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

    // At its priority the seat makes its queued plays of this turn and phase
    // in actions.log order: tapformana activates the land's mana ability for
    // the named color now, castspell and playland are chosen here and played
    // by playChosenSpellAbility. Another seat's play first, or none: pass.
    @Override
    public List<SpellAbility> chooseSpellAbilityToPlay() {
        if (atCheckpoint.getAsBoolean()) {
            throw new Checkpoint();
        }
        for (ScenarioAnswers.Play play; (play = answers.nextPlayFor(player)) != null;) {
            if (play.verb().equals("pass")) {
                return null;
            }
            Card card = answers.card(play.id());
            if (play.verb().equals("tapformana")) {
                tapForMana(card, play.arg());
                continue;
            }
            if (play.verb().equals("activate") || play.verb().equals("manaability")) {
                SpellAbility sa = printedAbility(card, Integer.parseInt(play.arg()));
                if (sa.isManaAbility() != play.verb().equals("manaability")) {
                    throw new IllegalStateException(play.verb() + " " + play.arg() + " of " + card + " is " + sa);
                }
                sa.setActivatingPlayer(player);
                if (!sa.isManaAbility()) {
                    return List.of(sa);
                }
                if (!PlaySpellAbility.playSpellAbilityNoStack(this, player, sa, true)) {
                    throw new IllegalStateException(card + " did not activate " + sa);
                }
                continue;
            }
            boolean land = play.verb().equals("playland");
            for (SpellAbility sa : card.getAllPossibleAbilities(player, true)) {
                if (land ? sa.isLandAbility() : sa.isSpell()) {
                    return List.of(sa);
                }
            }
            throw new IllegalStateException(player + " cannot " + play.verb() + " " + card);
        }
        return null;
    }

    @Override
    public boolean playChosenSpellAbility(SpellAbility sa) {
        if (!PlaySpellAbility.playSpellAbility(this, player, sa)) {
            throw new IllegalStateException(player + "'s play of " + sa + " was refused");
        }
        return true;
    }

    // Go's abilityAt: the index'th of the card's A: lines, then the
    // abilities its keywords and the statics granting it add, in that order.
    // Java keeps the same order after the basic spell or land ability
    // (CardState.getSpellAbilities, updateSpellAbilities).
    private static SpellAbility printedAbility(Card card, int index) {
        int i = 0;
        for (SpellAbility sa : card.getCurrentState().getSpellAbilities()) {
            if (sa instanceof SpellPermanent || sa.isLandAbility() || sa == card.getCurrentState().getAuraSpell()) {
                continue;
            }
            if (i++ == index) {
                return sa;
            }
        }
        throw new IllegalStateException("ability " + index + " of " + card + " among " + card.getCurrentState().getSpellAbilities());
    }

    // Go's TapLandForMana: the land's mana ability for one color, resolved
    // without the stack (a mana ability, CR 605.3), by its controller.
    private static void tapForMana(Card card, String color) {
        Player player = card.getController();
        for (SpellAbility ma : card.getManaAbilities()) {
            AbilityManaPart part = ma.getManaPart();
            if (!part.canProduce(color, ma)) {
                continue;
            }
            if (part.isAnyMana() || part.isComboMana()) {
                part.setExpressChoice(color);
            }
            ma.setActivatingPlayer(player);
            if (!PlaySpellAbility.playSpellAbilityNoStack(player.getController(), player, ma, true)) {
                throw new IllegalStateException(card + " did not tap for " + color);
            }
            return;
        }
        throw new IllegalStateException(card + " has no mana ability for " + color);
    }

    // Go's targets queue: the next answer, every entity added as chosen.
    // Go asks nothing where there is no candidate (targeting.go
    // resolveTargets: no legal set unless TargetMin$ is 0), nor for an Aura
    // spell with one (castspell.go castAura).
    @Override
    public boolean chooseTargetsFor(SpellAbility currentAbility) {
        TargetRestrictions tgt = currentAbility.getTargetRestrictions();
        if (tgt.getNumCandidates(currentAbility) == 0) {
            return tgt.getMinTargets(currentAbility.getHostCard(), currentAbility) == 0;
        }
        if (currentAbility.isSpell() && currentAbility.getHostCard().isAura()) {
            List<GameEntity> candidates = tgt.getAllCandidates(currentAbility);
            if (candidates.size() == 1) {
                currentAbility.getTargets().add(candidates.get(0));
                return true;
            }
        }
        for (GameEntity e : answers.nextTargets()) {
            // A card id on the stack names the spell (Go's TargetStack).
            GameObject o = e instanceof Card c && c.isInZone(ZoneType.Stack)
                    ? c.getGame().getStack().getSpellMatchingHost(c) : e;
            // Java offers only what can still be chosen next to the targets
            // already picked (a MaxTotalTargetCMC$ sum); Go drops the rest
            // the same way (targeting.go trimTargetSet). A target illegal on
            // its own is kept, as Go keeps it: the spell fizzles.
            if (currentAbility.canTarget(o) || !alone(currentAbility, o)) {
                currentAbility.getTargets().add(o);
            }
        }
        if (currentAbility.isDividedAsYouChoose()) {
            // Go's divideAmongTargets (dividedtargets.go) asks nothing for one
            // target (the total) or one per point (1 each); any other split is
            // queue divide, not mapped yet.
            List<GameObject> chosen = List.copyOf(currentAbility.getTargets());
            int total = currentAbility.getStillToDivide();
            if (chosen.size() != 1 && chosen.size() != total) {
                throw new IllegalStateException("dividing " + total + " among " + chosen + " needs queue divide");
            }
            for (GameObject o : chosen) {
                currentAbility.addDividedAllocation(o, chosen.size() == 1 ? total : 1);
            }
        }
        return true;
    }

    // Whether sa could target o with nothing else chosen.
    private static boolean alone(SpellAbility sa, GameObject o) {
        List<GameObject> chosen = List.copyOf(sa.getTargets());
        sa.getTargets().clear();
        boolean ok = sa.canTarget(o);
        sa.getTargets().addAll(chosen);
        return ok;
    }

    // Mana is paid the way a human pays (PlayerControllerHuman.payManaCost):
    // PlaySpellAbility.payManaCost announces X and applies the cost
    // adjustments (a power-up's reduction, CostAdjustment.java:225), then
    // asks applyManaToCost.
    @Override
    public boolean payManaCost(ManaCost toPay, CostPartMana costPartMana, SpellAbility sa, String prompt,
            ManaConversionMatrix matrix, boolean effect) {
        return PlaySpellAbility.payManaCost(this, toPay, costPartMana, sa, player, prompt, matrix, effect);
    }

    // The cost is paid from the pool the queued tapformana lines filled; a
    // cost the pool cannot pay is an error, never an automatic tap.
    @Override
    public boolean applyManaToCost(ManaCostBeingPaid toPay, SpellAbility ability, String prompt,
            ManaConversionMatrix matrix, boolean effect) {
        if (!player.getManaPool().payManaCostFromPool(toPay, ability, false, ability.getPayingMana())) {
            throw new IllegalStateException(player + "'s pool cannot pay " + toPay + " for " + ability);
        }
        return true;
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

    // Upstream's version puts each trigger on the stack with no targets
    // (prepareSingleSa chooses them only for TargetingPlayer). A trigger is
    // played as a spell is, its targets from the queue
    // (PlayerControllerHuman.orderAndPlaySimultaneousSa); the order stays
    // upstream's, the list's.
    @Override
    public void orderAndPlaySimultaneousSa(List<SpellAbility> activePlayerSAs) {
        for (SpellAbility sa : orderSimultaneousSa(activePlayerSAs)) {
            if (sa.isTrigger() && !sa.isCopied()) {
                PlaySpellAbility.playSpellAbility(this, player, sa);
            } else {
                super.orderAndPlaySimultaneousSa(List.of(sa));
            }
        }
    }

    // Upstream's version calls PlayerController.getAbilityToPlay(Card, List),
    // which calls it back (PlayerControllerForTests.java:494-497,
    // PlayerController.java:99): every PlaySpellAbility.playSpellAbility
    // overflows the stack choosing an additional cost (PlaySpellAbility.java:146).
    // The first is the ability itself, the one Go's castspell plays.
    @Override
    public SpellAbility getAbilityToPlay(Card hostCard, List<SpellAbility> abilities, ITriggerEvent triggerEvent) {
        return abilities.isEmpty() ? null : abilities.get(0);
    }

    // An "as it enters, choose a card name" replacement GameState's setup
    // runs names nothing, as Go's fixture.Load runs none: the setup's
    // NamedCard: is added after (GameState.java:1073-1076). In play a name
    // is a queued answer, not mapped yet.
    @Override
    public String chooseCardName(SpellAbility sa, Predicate<ICardFace> cpp, String valid, String message) {
        if (!answers.bound()) {
            return "";
        }
        throw new IllegalStateException("naming a card for " + sa + " needs queue cardname");
    }

    // A trigger that never uses the stack takes its targets the same way.
    @Override
    public boolean playTrigger(Card host, WrappedAbility wrapperAbility, boolean isMandatory) {
        return PlaySpellAbility.playSpellAbilityNoStack(this, player, wrapperAbility, false);
    }

    // Upstream's version plays only the abilities of a list of named cards
    // and throws for any other (a planeswalker's or a Saga's state trigger,
    // F6). Every one is played, as the game's own controllers do.
    @Override
    public void playSpellAbilityNoStack(SpellAbility effectSA, boolean mayChoseNewTargets) {
        PlaySpellAbility.playSpellAbilityNoStack(this, player, effectSA, !mayChoseNewTargets);
    }
}
