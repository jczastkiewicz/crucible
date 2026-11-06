package engine

//enginelint:allow id card game ability condition control zone cloneeffect land cantbecast priority player

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// playLandVariantEffect is PlayLandVariantEffect.java: the host, a colored
// card, is played as its land drop as a copy of a basic land card chosen at
// random among those that make one of its colors (Dakkon Blackblade Avatar's
// grant, the one corpus line).
//
// Ported from forge-game/src/main/java/forge/game/ability/effects/
// PlayLandVariantEffect.java's resolve (:25-65). The candidates are the
// basic and snow-covered basic land names of the host's colors
// (MagicColor.java:138-139) that the card database holds as basic lands, in
// the database's name order (CardDb.java:56). Each try draws one
// Aggregates.random pick (nextInt(size), no draw for a single candidate,
// Aggregates.java:131-143), makes the card (CardFactory.getCard: a new card
// id, here a card in no zone) and drops it from the list, until one can be
// played (Player.canPlayLand on the fresh copy: the activator's turn,
// sorcery timing, a land drop left, no CantPlayLand static). A colorless host
// or no playable candidate does nothing, as in Java. The host then takes the
// land's copiable values as a permanent copy effect (addCloneState) and is
// played as a land (Player.playLand: the land drop, LandPlayed).
//
// Clone$ BasicLand is the only value Java filters on; another value, or a
// Defined$, is refused (Java ignores both).
type playLandVariantEffect struct{}

// basicLandNames and snowLandNames are MagicColor.BASIC_LANDS and SNOW_LANDS,
// indexed like WUBRG.
var (
	basicLandNames = [...]string{"Plains", "Island", "Swamp", "Mountain", "Forest"}
	snowLandNames  = [...]string{"Snow-Covered Plains", "Snow-Covered Island", "Snow-Covered Swamp", "Snow-Covered Mountain", "Snow-Covered Forest"}
	wubrg          = [...]mana.Colors{mana.White, mana.Blue, mana.Black, mana.Red, mana.Green}
)

func (playLandVariantEffect) Resolve(g *Game, a *Ability, c PlayerController) error {
	if _, ok := a.Params.Param("Defined"); ok {
		return fmt.Errorf("engine: PlayLandVariant: Defined$ not resolvable (Java plays the host)")
	}
	if clone, _ := a.Params.Param("Clone"); clone != "BasicLand" {
		return fmt.Errorf("engine: PlayLandVariant: Clone$ %q not resolvable yet", clone)
	}
	if !subAbilityConditionMet(g, g.Card(a.Source), a.Amounts, a.Params) {
		return nil
	}
	source := a.Source
	activator := a.Controller
	colors := g.Card(source).Colors()
	var names []string
	for i, col := range wubrg {
		if !colors.Has(col) {
			continue
		}
		for _, name := range [...]string{basicLandNames[i], snowLandNames[i]} {
			def, ok := g.db.Card(name)
			if ok && def.Faces[0].Type.HasSupertype(cardtype.Basic) && def.Faces[0].Type.Has(cardtype.Land) {
				names = append(names, name)
			}
		}
	}
	sort.SliceStable(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	var chosen CardID
	for len(names) > 0 {
		pick := 0
		if len(names) > 1 {
			pick = int(g.rand.Int32n(int32(len(names))))
		}
		def, _ := g.db.Card(names[pick])
		cand := g.NewCard(def, activator, None)
		names = append(names[:pick:pick], names[pick+1:]...)
		if g.canActSorcerySpeed(activator) && g.hasLandDrop(activator) && !g.cantPlayLand(activator, cand) {
			chosen = cand
			break
		}
	}
	if chosen == NoCard {
		return nil
	}
	g.timestamp++
	g.Card(source).addCopy(copyEffect{Timestamp: g.timestamp, def: g.Card(chosen).Def, permanent: true})
	origin := g.Card(source).Zone
	g.playLandNow(c, activator, source)
	if g.Card(source).Zone == Battlefield {
		g.checkChangesZoneAllTriggers(c, []CardID{source}, origin, Battlefield)
	}
	return nil
}
