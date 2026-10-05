package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Mode$ CastWithFlash with IsPresent$ / PresentZone$ / PresentCompare$
// (StaticAbility.checkConditions): the grant holds only while the count
// matches, evaluated for the static's own host.
func TestCastWithFlashStaticReadsIsPresent(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		present bool
		want    bool
	}{
		{"a Human is on the battlefield", true, true},
		{"no Human", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, other := newTwoPlayerGame(t)
			def := creatureDefCost(t, "Self Flash", "G")
			def.Faces[0].Statics = append(def.Faces[0].Statics, scriptDef(t, "x", "Enchantment",
				"S:Mode$ CastWithFlash | ValidCard$ Card.Self | ValidSA$ Spell | IsPresent$ Creature.Human+YouCtrl | EffectZone$ All | Caster$ You").Faces[0].Statics...)
			card := g.NewCard(def, p, engine.Hand)
			if tc.present {
				g.NewCard(scriptDef(t, "Test Human", "Creature Human", "K:Vigilance"), p, engine.Battlefield)
			}
			if got := castOnOpponentsTurn(t, g, p, other, card); got != tc.want {
				t.Errorf("CastSpell on the opponent's turn = %v, want %v", got, tc.want)
			}
		})
	}
}

// Mode$ CastWithFlash with CheckSVar$ (the Count$ amount against SVarCompare$).
func TestCastWithFlashStaticReadsCheckSVar(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		compare string
		want    bool
	}{
		{"holds", "EQ2", true},
		{"fails", "EQ0", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, other := newTwoPlayerGame(t)
			host := scriptDef(t, "Test Host", "Enchantment",
				"S:Mode$ CastWithFlash | ValidCard$ Creature | ValidSA$ Spell | Caster$ You | CheckSVar$ X | SVarCompare$ "+tc.compare,
				"SVar:X:2")
			g.NewCard(host, p, engine.Battlefield)
			card := g.NewCard(creatureDefCost(t, "Mine", "G"), p, engine.Hand)
			if got := castOnOpponentsTurn(t, g, p, other, card); got != tc.want {
				t.Errorf("CastSpell on the opponent's turn = %v, want %v", got, tc.want)
			}
		})
	}
}

// Mode$ CastWithFlash with ValidSA$ Activated.Equip (Equipment of the Quick
// Hands family): the controller may Equip at instant speed. Without the
// static an Equip is sorcery-speed.
func TestCastWithFlashLetsEquipBeActivatedAtInstantSpeed(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		grant bool
		want  bool
	}{
		{"with the static", true, true},
		{"without", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, other := newTwoPlayerGame(t)
			if tc.grant {
				g.NewCard(scriptDef(t, "Test Flash Equip", "Enchantment",
					"S:Mode$ CastWithFlash | ValidSA$ Activated.Equip | Caster$ You"), p, engine.Battlefield)
			}
			def := scriptDef(t, "Test Blade", "Artifact Equipment", "K:Equip:0")
			blade := g.NewCard(def, p, engine.Battlefield)
			creature := g.NewCard(creatureDef(t), p, engine.Battlefield)
			g.SetTurnState(1, other, engine.Main1)
			c := engine.NewScriptedController()
			c.QueueTargets([]engine.EntityID{engine.CardEntity(creature)})
			if got := g.ActivateAbility(p, blade, 0, c); got != tc.want {
				t.Errorf("ActivateAbility Equip on the opponent's turn = %v, want %v", got, tc.want)
			}
		})
	}
}
