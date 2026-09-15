// The mana pool: CR 106.4's floating mana, and CR 601.2h/601.2i's payment
// algorithm for the plain-colored-and-generic case.

package engine

import "github.com/jczastkiewicz/crucible/internal/mana"

// Pool is what one player has floating (CR 106.4) -- mana added but not yet
// spent, held separately by the five colors and by the {C} colorless mana
// type. The zero value is empty.
//
// Generic amount is not stored here: CR 106.6's "any type of mana can pay a
// generic cost" means nothing about a floating mana's own color changes
// once it is already in the pool -- the distinction generic payment cares
// about is which shards of the *cost* are colored, not which mana in the
// pool was added as what.
type Pool struct {
	white, blue, black, red, green, colorless int
}

// Add puts n mana of one color in the pool. color must be exactly one of
// mana.White/Blue/Black/Red/Green -- anything else (the zero value, more
// than one bit set) is an engine invariant breach, not something a card
// script can cause, so it panics rather than silently doing nothing (GO-7).
func (p *Pool) Add(color mana.Colors, n int) {
	switch color {
	case mana.White:
		p.white += n
	case mana.Blue:
		p.blue += n
	case mana.Black:
		p.black += n
	case mana.Red:
		p.red += n
	case mana.Green:
		p.green += n
	default:
		panic("engine: Pool.Add wants exactly one color")
	}
}

// AddColorless puts n {C} mana in the pool -- CR 106.3's colorless mana
// symbol, distinct from mana.Colors' own empty set (mana.Colors' own doc
// comment: {C} is a way to pay a cost, not a color, and is not
// representable there at all).
func (p *Pool) AddColorless(n int) { p.colorless += n }

// Total is how much mana of any kind is in the pool.
func (p *Pool) Total() int {
	return p.white + p.blue + p.black + p.red + p.green + p.colorless
}

// Empty clears the pool -- CR 500.4, run once per phase/step transition
// (emptyManaPools, turn.go) for every player, not something a card ability
// triggers.
func (p *Pool) Empty() { *p = Pool{} }

// Pay spends cost from the pool and reports whether it succeeded. The pool
// is unchanged if it did not -- Pay never spends part of a cost it cannot
// finish paying.
//
// Only cost.Generic and the six "pure" shards (ShardW/U/B/R/G/C, CR
// 601.2h's colored-and-generic case, and Java's own ManaCostShard
// declaration order: "shards that offer the fewest ways to be paid come
// first") are handled. A shard with any other atom -- hybrid (ShardWU and
// the rest), Phyrexian ({R/P}), {X}, snow ({S}) -- is a shape this port has
// no substitution rule for yet: which of two colors a hybrid symbol takes,
// whether a Phyrexian symbol is paid with mana or 2 life, is a decision (the
// same category ChooseLegendaryToKeep's own answer is), and this port has
// no PlayerController method to ask it with. Pay reports false for a cost
// containing one, the same "cannot resolve this yet" answer insufficient
// mana gets -- a caller cannot tell the two apart from the bool alone, which
// is deliberate: neither means the pool can safely be spent.
//
// Generic is paid from whatever the pool has left after every pip, in a
// fixed order (colorless, then white/blue/black/red/green) rather than a
// controller's real choice (CR 601.2h grants one) -- nothing decides that
// choice yet, and the order only affects what is left in the pool
// afterward, which nothing currently reads.
func (p *Pool) Pay(cost mana.Cost) bool {
	spend := *p
	for _, s := range cost.Shards() {
		var bucket *int
		switch s {
		case mana.ShardW:
			bucket = &spend.white
		case mana.ShardU:
			bucket = &spend.blue
		case mana.ShardB:
			bucket = &spend.black
		case mana.ShardR:
			bucket = &spend.red
		case mana.ShardG:
			bucket = &spend.green
		case mana.ShardC:
			bucket = &spend.colorless
		default:
			return false
		}
		if *bucket == 0 {
			return false
		}
		*bucket--
	}

	generic := cost.Generic()
	for generic > 0 {
		switch {
		case spend.colorless > 0:
			spend.colorless--
		case spend.white > 0:
			spend.white--
		case spend.blue > 0:
			spend.blue--
		case spend.black > 0:
			spend.black--
		case spend.red > 0:
			spend.red--
		case spend.green > 0:
			spend.green--
		default:
			return false
		}
		generic--
	}

	*p = spend
	return true
}
