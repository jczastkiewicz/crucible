package crucible.oracle;

import java.util.List;

import forge.ai.AiCostDecision;
import forge.game.card.Card;
import forge.game.card.CardCollectionView;
import forge.game.card.CardLists;
import forge.game.card.CardPredicates;
import forge.game.cost.CostDiscard;
import forge.game.cost.CostExile;
import forge.game.cost.CostForage;
import forge.game.cost.CostPutCounter;
import forge.game.cost.CostReveal;
import forge.game.cost.CostSacrifice;
import forge.game.cost.PaymentDecision;
import forge.game.player.Player;
import forge.game.spellability.SpellAbility;
import forge.game.zone.ZoneType;

/**
 * The cost decisions upstream's test controller makes with AiCostDecision,
 * except the cards a cost part takes: Go asks ChooseCardsForEffect for them
 * (chosencosts.go), so a queued card choice that fits the part's options
 * picks them (S16). A part with no fitting answer keeps the AI's pick.
 */
final class ScenarioCostDecision extends AiCostDecision {
    private final ScenarioAnswers answers;

    ScenarioCostDecision(Player player, SpellAbility sa, boolean effect, ScenarioAnswers answers) {
        super(player, sa, effect);
        this.answers = answers;
    }

    // ExileFromGrave<1/Creature>: Redemptor Dreadnought's optional cost.
    @Override
    public PaymentDecision visit(CostExile cost) {
        PaymentDecision ai = super.visit(cost);
        if (cost.payCostFromSource() || cost.getType().equals("All")) {
            return ai;
        }
        return pick(ai, valid(player.getCardsIn(cost.getFrom()), cost.getType()), cost.getAbilityAmount(ability));
    }

    // Forage: three graveyard cards exiled (Corpseberry Cultivator); a Food
    // sacrificed instead keeps the AI's pick.
    @Override
    public PaymentDecision visit(CostForage cost) {
        PaymentDecision ai = super.visit(cost);
        CardCollectionView graveyard = CardLists.filter(player.getCardsIn(ZoneType.Graveyard),
                CardPredicates.canExiledBy(ability, isEffect()));
        return graveyard.size() < 3 ? ai : pick(ai, graveyard, 3);
    }

    // Blight<1> is a CostPutCounter (CostBlight): Cinder Strike's optional cost.
    @Override
    public PaymentDecision visit(CostPutCounter cost) {
        PaymentDecision ai = super.visit(cost);
        if (cost.payCostFromSource()) {
            return ai;
        }
        CardCollectionView options = CardLists.filter(
                valid(player.getGame().getCardsIn(ZoneType.Battlefield), cost.getType()),
                CardPredicates.canReceiveCounters(cost.getCounter()));
        return pick(ai, options, 1);
    }

    // RevealOrChoose<1/Dragon>: Dragon's Fire's optional cost.
    @Override
    public PaymentDecision visit(CostReveal cost) {
        PaymentDecision ai = super.visit(cost);
        if (cost.payCostFromSource() || cost.getType().equals("Hand")) {
            return ai;
        }
        return pick(ai, valid(player.getCardsIn(cost.getRevealFrom()), cost.getType()),
                cost.getAbilityAmount(ability));
    }

    // Discard<1/Card>: Go's ChooseCardsToDiscard (costparts.go,
    // chosencosts.go). A random or whole-hand discard keeps the AI's pick.
    @Override
    // The queued answer is read before the AI's pick: AiCostDecision's own
    // discard and sacrifice visits cast the controller to PlayerControllerAi.
    public PaymentDecision visit(CostDiscard cost) {
        String type = cost.getType();
        if (cost.payCostFromSource() || type.equals("Hand") || type.equals("Random") || type.contains("WithSameName")) {
            return super.visit(cost);
        }
        int n = cost.getAbilityAmount(ability);
        List<Card> chosen = answers.nextDiscardChoice(valid(player.getCardsIn(ZoneType.Hand), type), n, n);
        return chosen != null ? PaymentDecision.card(chosen) : super.visit(cost);
    }

    // Sac<1/Creature>: Go's ChoosePermanentsToSacrifice (chosencosts.go).
    @Override
    public PaymentDecision visit(CostSacrifice cost) {
        if (cost.payCostFromSource() || cost.getType().equals("All")) {
            return super.visit(cost);
        }
        int n = cost.getAbilityAmount(ability);
        List<Card> chosen = answers.nextSacrificeChoice(
                valid(player.getCardsIn(ZoneType.Battlefield), cost.getType()), n, n);
        return chosen != null ? PaymentDecision.card(chosen) : super.visit(cost);
    }

    private CardCollectionView valid(CardCollectionView cards, String type) {
        return CardLists.getValidCards(cards, type.split(";"), player, source, ability);
    }

    private PaymentDecision pick(PaymentDecision ai, CardCollectionView options, int n) {
        List<Card> chosen = answers.nextCardChoice(options, n, n);
        return chosen != null ? PaymentDecision.card(chosen) : ai;
    }
}
