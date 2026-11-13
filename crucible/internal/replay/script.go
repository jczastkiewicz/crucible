package replay

import (
	"encoding/json"
	"fmt"
)

// abilityID is an ADR-0059 ability identity as the recorder writes it.
type abilityID struct {
	Host    Ref    `json:"host"`
	Section string `json:"section"`
	State   string `json:"state"`
	Index   *int   `json:"index"`
	Sub     int    `json:"sub"`
	API     string `json:"api"`
	Keyword string `json:"keyword"`
	// By is "params" for a trait Java found by its script params, not its
	// per-game id: a copy, such as the spell an alternative cost casts.
	By      string     `json:"by"`
	Wrapped *abilityID `json:"wrapped"`
	Grantor *abilityID `json:"grantor"`
}

func (a abilityID) String() string {
	s := fmt.Sprintf("%s %s", a.Host, a.Section)
	if a.Keyword != "" {
		s += " keyword " + a.Keyword
	}
	if a.Index != nil {
		s += fmt.Sprintf(" #%d", *a.Index)
	}
	if a.API != "" {
		s += " " + a.API
	}
	return s
}

// call is a controller call record (ADR-0059).
type call struct {
	Seq     int             `json:"seq"`
	Turn    int             `json:"turn"`
	Phase   string          `json:"phase"`
	Decider Ref             `json:"decider"`
	Method  string          `json:"method"`
	Depth   int             `json:"depth"`
	Offered json.RawMessage `json:"offered"`
	Answer  json.RawMessage `json:"answer"`
	used    bool
}

// play is a play record (ADR-0062): what the AI chose for one play.
type play struct {
	Seq     int       `json:"seq"`
	Call    int       `json:"call"`
	Decider Ref       `json:"decider"`
	Ability abilityID `json:"ability"`
	Chain   []struct {
		Ability abilityID         `json:"ability"`
		Targets []json.RawMessage `json:"targets"`
	} `json:"chain"`
	X *int `json:"x"`
	// Paid is what the play's non-mana costs paid, by cost part (Exiled,
	// Sacrificed, Discarded ...).
	Paid []struct {
		Part  string `json:"part"`
		Cards []Ref  `json:"cards"`
	} `json:"paid"`
	ManaAbilities []abilityID `json:"manaAbilities"`
	PayingMana    []struct {
		Source Ref `json:"source"`
		Color  int `json:"color"`
	} `json:"payingMana"`
}

// payment is a payment record (ADR-0062): a payment made outside a play.
type payment struct {
	Seq           int         `json:"seq"`
	Call          int         `json:"call"`
	Payer         Ref         `json:"payer"`
	ManaAbilities []abilityID `json:"manaAbilities"`
}

// shuffle is a shuffle's outcome: the library's order, top first.
type shuffle struct {
	Player Ref   `json:"player"`
	Order  []int `json:"order"`
}

// coin is a coin flip's outcome; the starting toss is not an effect's flip.
type coin struct {
	Heads        bool `json:"heads"`
	StartingToss bool `json:"startingToss"`
}

// draw is one java.util.Random.next call the recorder could not name an
// outcome for (CardLists.shuffle's RestRandomOrder$, AtRandom$).
type draw struct {
	Turn  int    `json:"turn"`
	Phase string `json:"phase"`
	Bits  uint   `json:"bits"`
	Value int32  `json:"value"`
}

func (d draw) when() when { return when{d.Turn, javaSteps[d.Phase]} }

// die is a die roll's outcome.
type die struct {
	Sides int `json:"sides"`
	Face  int `json:"face"`
}

// script is a log's records decoded for the replay controller.
type script struct {
	calls    []*call
	plays    []*play
	payments []*payment
	// shuffles is each seat's shuffles in Java's order.
	shuffles map[int][]shuffle
	// coins and dice are the effects' flips and rolls in Java's order.
	coins []bool
	dice  []die
	// draws are the other draws, in Java's order.
	draws []draw
}

func newScript(log *Log) (*script, error) {
	s := &script{shuffles: map[int][]shuffle{}}
	for _, r := range log.Records {
		var err error
		switch r.Kind {
		case "call":
			c := &call{}
			err = json.Unmarshal(r.Raw, c)
			s.calls = append(s.calls, c)
		case "play":
			p := &play{}
			err = json.Unmarshal(r.Raw, p)
			s.plays = append(s.plays, p)
		case "payment":
			p := &payment{}
			err = json.Unmarshal(r.Raw, p)
			s.payments = append(s.payments, p)
		case "shuffle":
			var sh shuffle
			err = json.Unmarshal(r.Raw, &sh)
			s.shuffles[sh.Player.Seat] = append(s.shuffles[sh.Player.Seat], sh)
		case "coin":
			var c coin
			err = json.Unmarshal(r.Raw, &c)
			if !c.StartingToss {
				s.coins = append(s.coins, c.Heads)
			}
		case "die":
			var d die
			err = json.Unmarshal(r.Raw, &d)
			s.dice = append(s.dice, d)
		case "draw":
			var d draw
			err = json.Unmarshal(r.Raw, &d)
			s.draws = append(s.draws, d)
		}
		if err != nil {
			return nil, fmt.Errorf("replay: record %d (%s): %w", r.Seq, r.Kind, err)
		}
	}
	return s, nil
}

// paymentFor is the payment record of the pay call seq.
func (s *script) paymentFor(seq int) *payment {
	for _, p := range s.payments {
		if p.Call == seq {
			return p
		}
	}
	return nil
}

// javaSteps orders Java's PhaseType names within a turn.
var javaSteps = map[string]int{
	"UNTAP": 1, "UPKEEP": 2, "DRAW": 3, "MAIN1": 4, "COMBAT_BEGIN": 5, "COMBAT_DECLARE_ATTACKERS": 6,
	"COMBAT_DECLARE_BLOCKERS": 7, "COMBAT_FIRST_STRIKE_DAMAGE": 8, "COMBAT_DAMAGE": 9, "COMBAT_END": 10,
	"MAIN2": 11, "END_OF_TURN": 12, "CLEANUP": 13,
}

// when is a (turn, step) position comparable across both engines.
type when struct{ turn, step int }

func (w when) before(o when) bool {
	return w.turn < o.turn || w.turn == o.turn && w.step < o.step
}

func (c *call) when() when { return when{c.Turn, javaSteps[c.Phase]} }
