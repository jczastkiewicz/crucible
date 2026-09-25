package engine

//enginelint:allow ability animate card condition control defined effecthelpers game id parts player token zone

import (
	"fmt"
	"strings"
)

// tokenUnresolvedParams are TokenEffect/TokenEffectBase params this port
// does not model: putting the token into combat (TokenAttacking$/
// TokenBlocking$), attaching it (AttachedTo$/AttachAfter$), a counter
// table on entry (WithCountersType$), copied triggers (AddTriggersFrom$),
// chosen-type/-color prototypes (TokenTypes$/TokenColors$), what the token
// remembers (TokenRemembered$/CleanupForEach$/RememberOriginalTokens$), a
// shared zone table (ChangeZoneTable$) and the end-of-turn delayed
// sacrifice/exile (AtEOT$/AtEOTTrig$).
var tokenUnresolvedParams = [...]string{
	"TokenAttacking", "TokenBlocking", "AttachedTo", "AttachAfter", "WithCountersType",
	"WithCountersAmount", "AddTriggersFrom", "TokenTypes", "TokenColors", "TokenRemembered",
	"CleanupForEach", "RememberOriginalTokens", "ChangeZoneTable", "AtEOT", "AtEOTTrig",
	"Condition", "ConditionDefined",
}

// tokenEffect is TokenEffect.java: TokenAmount$ (default 1) of every
// TokenScript$ script for every TokenOwner$ player (getDefinedPlayersOrTargeted:
// TokenOwner$ when named, else the targets, else You; APNAP order),
// created in that order and entered one at a time, then ChangesZoneAll
// once for the batch (triggerChangesZoneAll).
type tokenEffect struct{}

func (tokenEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectParams(a, "Token", tokenUnresolvedParams[:]...); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	amount, err := optionalAmount(g, a, "Token", "TokenAmount", 1)
	if err != nil || amount < 1 {
		return err
	}
	scripts, ok := a.Params.Param("TokenScript")
	if !ok {
		return fmt.Errorf("engine: Token: no TokenScript$")
	}
	owners, err := tokenOwners(g, a)
	if err != nil {
		return err
	}
	base := tokenSpec{Tapped: hasParam(a, "TokenTapped")}
	if base.HasPower = hasParam(a, "TokenPower"); base.HasPower {
		if base.Power, err = optionalAmount(g, a, "Token", "TokenPower", 0); err != nil {
			return err
		}
	}
	if base.HasToughness = hasParam(a, "TokenToughness"); base.HasToughness {
		if base.Toughness, err = optionalAmount(g, a, "Token", "TokenToughness", 0); err != nil {
			return err
		}
	}
	pump, err := tokenPumpKeywords(a)
	if err != nil {
		return err
	}

	var specs []tokenSpec
	for _, owner := range owners {
		if g.Player(owner).Lost {
			continue
		}
		for _, script := range strings.Split(scripts, ",") {
			def, err := tokenScript(g, strings.TrimSpace(script))
			if err != nil {
				return err
			}
			spec := base
			spec.Def, spec.Owner = def, owner
			n := g.tokensReplaced(controller, owner, def, amount)
			for i := 0; i < n; i++ {
				specs = append(specs, spec)
			}
		}
	}
	var created []CardID
	for _, spec := range specs {
		id := g.createToken(controller, spec)
		created = append(created, id)
		if pump != nil {
			g.timestamp++
			g.addAnimate(animateRecord{
				Card: id, Timestamp: g.timestamp, Permanent: pump.permanent, AddKeywords: pump.keywords,
			})
		}
		if hasParam(a, "RememberTokens") {
			source.Memory.Remember(CardEntity(id))
		}
		if hasParam(a, "ImprintTokens") {
			source.Memory.Imprint(id)
		}
		if hasParam(a, "RememberSource") {
			g.Card(id).Memory.Remember(CardEntity(a.Source))
		}
	}
	g.checkChangesZoneAllTriggers(controller, created, None, Battlefield)
	return nil
}

// tokenOwners is getDefinedPlayersOrTargeted(sa, "TokenOwner").
func tokenOwners(g *Game, a *Ability) ([]PlayerID, error) {
	var owners []PlayerID
	if raw, ok := a.Params.Param("TokenOwner"); ok {
		for _, d := range strings.Split(raw, " & ") {
			ps, err := definedPlayers(g, a.Controller, a.Source, d, a.refs())
			if err != nil {
				return nil, fmt.Errorf("engine: Token: %w", err)
			}
			owners = append(owners, ps...)
		}
	} else if hasParam(a, "ValidTgts") {
		for _, e := range a.Targets {
			if pid, ok := e.AsPlayer(); ok {
				owners = append(owners, pid)
			}
		}
	} else {
		owners = []PlayerID{a.Controller}
	}
	return g.inAPNAPOrder(owners), nil
}

// tokenPump is PumpKeywords$ with its PumpDuration$: keywords the created
// token gains, permanently when PumpDuration$ is absent (addPumpUntil
// registers nothing), until end of turn otherwise.
type tokenPump struct {
	keywords  []string
	permanent bool
}

func tokenPumpKeywords(a *Ability) (*tokenPump, error) {
	raw, ok := a.Params.Param("PumpKeywords")
	if !ok {
		return nil, nil
	}
	p := &tokenPump{keywords: strings.Split(raw, " & "), permanent: true}
	if d, ok := a.Params.Param("PumpDuration"); ok {
		if d == "UntilYourNextTurn" {
			return nil, fmt.Errorf("engine: Token: PumpDuration$ %q not resolvable yet", d)
		}
		p.permanent = false
	}
	return p, nil
}
