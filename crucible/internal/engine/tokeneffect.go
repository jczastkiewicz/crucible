package engine

//enginelint:allow ability action animate card condition control defined effecthelpers game id parts player token valid zone changecombatantseffect ateot

import (
	"fmt"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/keyword"
)

// tokenUnresolvedParams are TokenEffect/TokenEffectBase params this port
// does not model (TokenAttacking$ and TokenBlocking$ resolve,
// addToCombatAttacking and addToCombatBlocking): copied triggers (AddTriggersFrom$), what the token remembers
// (TokenRemembered$/CleanupForEach$), a shared zone
// table (ChangeZoneTable$) and the end-of-turn delayed sacrifice/exile
// (AtEOT$/AtEOTTrig$).
var tokenUnresolvedParams = [...]string{
	"AddTriggersFrom", "TokenRemembered",
	"CleanupForEach", "ChangeZoneTable"}

// tokenEffect is TokenEffect.java: TokenAmount$ (default 1) of every
// TokenScript$ script for every TokenOwner$ player (getDefinedPlayersOrTargeted:
// TokenOwner$ when named, else the targets, else You; APNAP order),
// created in that order and entered one at a time, then ChangesZoneAll
// once for the batch (triggerChangesZoneAll).
type tokenEffect struct{}

func (tokenEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
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
	if kind, ok := a.Params.Param("WithCountersType"); ok {
		// TokenEffectBase.java:140-146: the counter table the token enters with,
		// WithCountersAmount$ (default 1) of the type. Only +1/+1 counters are
		// read here (Incubob, Printlifter Ooze).
		if kind != "P1P1" {
			return fmt.Errorf("engine: Token: WithCountersType$ %q not resolvable yet", kind)
		}
		if base.P1P1, err = optionalAmount(g, a, "Token", "WithCountersAmount", 1); err != nil {
			return err
		}
	}
	pump, err := tokenPumpKeywords(a, "Token")
	if err != nil {
		return err
	}

	chosen, err := tokenChosenTraits(a, source)
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
	attachTo, attachPlayer := NoCard, NoPlayer
	hasAttach := false
	if raw, ok := a.Params.Param("AttachedTo"); ok {
		hasAttach = true
		if attachTo, attachPlayer, err = tokenAttachHost(g, a, source, raw); err != nil {
			return err
		}
	}
	var created []CardID
	for _, spec := range specs {
		// CR 303.4i: an Aura token that cannot be attached to its host is not
		// created (TokenEffectBase.java:136).
		if hasAttach && attachTo != NoCard && !tokenCanAttach(g, a, spec, attachTo) && isAuraDef(spec.Def) {
			continue
		}
		if hasAttach && attachPlayer != NoPlayer && isAuraDef(spec.Def) && !tokenCanAttachPlayer(g, a, spec, attachPlayer) {
			continue
		}
		id := g.createToken(controller, spec)
		created = append(created, id)
		if loc, ok := a.Params.Param("AtEOTTrig"); ok {
			if err := g.addSelfTrigger(a, loc, id); err != nil {
				return err
			}
		}
		if hasAttach && attachTo != NoCard && tokenCanAttach(g, a, spec, attachTo) && g.Card(id).Zone == Battlefield {
			g.attachTo(controller, id, attachTo)
		}
		if hasAttach && attachPlayer != NoPlayer && tokenCanAttachPlayer(g, a, spec, attachPlayer) && g.Card(id).Zone == Battlefield {
			g.attachToPlayer(controller, id, attachPlayer)
		}
		if chosen != nil {
			g.timestamp++
			rec := *chosen
			rec.Card, rec.Timestamp = id, g.timestamp
			rec.Types.Timestamp = g.timestamp
			g.addAnimate(rec)
		}
		if pump != nil {
			g.timestamp++
			g.addAnimate(animateRecord{
				Card: id, Timestamp: g.timestamp, Permanent: pump.permanent, AddKeywords: pump.keywords,
			})
		}
		// TokenEffectBase.java:188: tapped and attacking (CR 506.3b), or
		// blocking, each token as it is made.
		if err := g.addToCombatAttacking(controller, a, id, "TokenAttacking"); err != nil {
			return fmt.Errorf("engine: Token: %w", err)
		}
		if err := g.addToCombatBlocking(controller, a, id, "TokenBlocking"); err != nil {
			return fmt.Errorf("engine: Token: %w", err)
		}
		// RememberOriginalTokens$ keeps the tokens of the scripts the effect
		// named, not ones a replacement swapped in; a CreateToken replacement
		// here only changes the count of the same script (quirks.md).
		if hasParam(a, "RememberTokens") || hasParam(a, "RememberOriginalTokens") {
			source.Memory.Remember(CardEntity(id))
		}
		if hasParam(a, "ImprintTokens") {
			source.Memory.Imprint(id)
		}
		if hasParam(a, "RememberSource") {
			g.Card(id).Memory.Remember(CardEntity(a.Source))
		}
	}
	g.checkCreatedZoneAllTriggers(controller, created)
	return registerAtEOT(g, a, created)
}

// tokenChosenTraits is TokenInfo.getProtoType's TokenTypes$ and TokenColors$
// (TokenInfo.java:309-320): the host's chosen type is added to the script's
// types and replaces its name outright (token "Elf", not "Elf Token"), and the
// host's chosen color replaces its colors. Only the ChosenType/ChosenColor
// values the corpus uses (Riptide Replicator, Volrath's Laboratory) resolve.
// Nil when neither param is present. The record lands after the token has
// entered, so its ETB sees the script's own characteristics (quirks.md).
func tokenChosenTraits(a *Ability, source *Card) (*animateRecord, error) {
	types, hasTypes := a.Params.Param("TokenTypes")
	colors, hasColors := a.Params.Param("TokenColors")
	if !hasTypes && !hasColors {
		return nil, nil
	}
	rec := &animateRecord{Permanent: true}
	if hasTypes {
		if types != "ChosenType" {
			return nil, fmt.Errorf("engine: Token: TokenTypes$ %q not resolvable yet", types)
		}
		chosen := source.Memory.ChosenType(false)
		if chosen != "" {
			rec.Types.AddTypes = cardtype.ParseToken(chosen)
		}
		rec.Name = chosen
	}
	if hasColors {
		if colors != "ChosenColor" {
			return nil, fmt.Errorf("engine: Token: TokenColors$ %q not resolvable yet", colors)
		}
		rec.HasColors, rec.OverwriteColors = true, true
		rec.Colors = source.Memory.ChosenColors()
	}
	return rec, nil
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

// api names the effect in an error: Token and Clone share this grant
// (TokenEffectBase.addPumpUntil is called from both).
func tokenPumpKeywords(a *Ability, api string) (*tokenPump, error) {
	raw, ok := a.Params.Param("PumpKeywords")
	if !ok {
		return nil, nil
	}
	p := &tokenPump{keywords: strings.Split(raw, " & "), permanent: true}
	if d, ok := a.Params.Param("PumpDuration"); ok {
		if d == "UntilYourNextTurn" {
			return nil, fmt.Errorf("engine: %s: PumpDuration$ %q not resolvable yet", api, d)
		}
		p.permanent = false
	}
	return p, nil
}

// tokenAttachHost is the first entity AttachedTo$ names (attachTokenTo reads the
// first defined entity): a card or a player (a Curse token on a player);
// NoCard and NoPlayer when it names none.
func tokenAttachHost(g *Game, a *Ability, source *Card, raw string) (CardID, PlayerID, error) {
	objects, err := definedEntities(g, a.Controller, source, raw, a.refs())
	if err != nil {
		return NoCard, NoPlayer, fmt.Errorf("engine: Token: AttachedTo$: %w", err)
	}
	if len(objects) == 0 {
		return NoCard, NoPlayer, nil
	}
	if id, ok := objects[0].AsCard(); ok {
		return id, NoPlayer, nil
	}
	if pid, ok := objects[0].AsPlayer(); ok {
		return NoCard, pid, nil
	}
	return NoCard, NoPlayer, fmt.Errorf("engine: Token: AttachedTo$ %q names neither a card nor a player", raw)
}

// tokenCanAttachPlayer is attachTokenTo's check for a player host: the token
// is an Aura whose Enchant keyword names Player or Opponent for its owner, the
// player is in the game and no Protection of theirs refuses it.
func tokenCanAttachPlayer(g *Game, a *Ability, spec tokenSpec, pid PlayerID) bool {
	if spec.Def == nil || g.Player(pid).Lost || !isAuraDef(spec.Def) {
		return false
	}
	for _, line := range spec.Def.Faces[0].Keywords {
		k := keyword.Parse(line)
		if k.Name != "Enchant" {
			continue
		}
		kind, _, _ := strings.Cut(k.Details, ":")
		if kind != "Player" && kind != "Opponent" {
			return false
		}
		matched, _ := matchesPlayerSpec(g, pid, spec.Owner, a.Source, kind)
		return matched
	}
	return false
}

// isAuraDef reports whether def is an Aura.
func isAuraDef(def *compile.Card) bool {
	return def != nil && def.Faces[0].Type.HasSubtype("Aura")
}

// tokenCanAttach is attachTokenTo's check: the token is an attachment, and an
// Aura's Enchant restriction and the host's protection allow it.
func tokenCanAttach(g *Game, a *Ability, spec tokenSpec, host CardID) bool {
	def := spec.Def
	if def == nil {
		return false
	}
	t := def.Faces[0].Type
	if !t.HasSubtype("Aura") && !t.HasSubtype("Equipment") && !t.HasSubtype("Fortification") {
		return false
	}
	if !t.HasSubtype("Aura") {
		return true
	}
	h := g.Card(host)
	if h.Zone != Battlefield {
		return false
	}
	if restriction, ok := enchantSpecOf(def); ok && !Matches(g, h, restriction, spec.Owner, a.Source) {
		return false
	}
	return true
}
