package engine

// ActionKind is the shared vocabulary of what an agent can do. Both the AI
// controller and, later, a human player emit the same kinds, so the engine
// never needs to know who is driving a given agent.
type ActionKind uint8

const (
	ActRest    ActionKind = iota
	ActMove               // wander or head in a direction
	ActEat                // go to a food item and eat it
	ActAttack             // close in on an agent and keep hitting it
	ActFlee               // run away from an agent
	ActObserve            // keep an eye on somebody to size them up
	ActCourt              // approach a candidate and try to form a pair

	// ActInvite is calling others in to bring something down (stage 32). It
	// is the first word added to this list since stage 0, and it is added at
	// the end on purpose: the numbers are what a saved world holds, so the
	// existing ones do not move.
	ActInvite

	// ActTake is picking something up to eat later (stage 40). Added at the
	// end for the same reason ActInvite was: the numbers are what a saved
	// world holds.
	//
	// There is no word for putting something down. A body can always eat what
	// it is holding - held food is a meal at no distance - so nothing is ever
	// stuck, and a verb per held item would cost every decision in the world
	// something to buy the ability to swap one plant for another.
	ActTake

	// ActThrow is a stone from out of reach (stage 46). Added at the end like
	// the two before it, for the same reason: the numbers are what a saved
	// world holds.
	ActThrow

	// ActGive is handing what is in one hand to somebody else (stage 48).
	// The eleventh word, added at the end like the three before it.
	ActGive

	// ActOffer is standing still and holding out what is in the hand, so that
	// anybody who can see it knows it is there (stage 49). The twelfth word,
	// added at the end like the four before it.
	ActOffer

	// ActStore is putting what is in the hand into a cache the body knows
	// (stage 50). The thirteenth word, added at the end like the five before
	// it. TargetID is the store's index, which is the only place in this
	// vocabulary where the target is not a food item or an agent - and the
	// reason there is no word for taking something back out is that there is
	// nothing to add: what is in a store this body knows is food in the
	// world, eaten and picked up by the ordinary words.
	ActStore

	// ActBuy is handing a coin to somebody who is holding out a meal, and
	// taking the meal (stage 51). The fourteenth word, added at the end like
	// the six before it.
	//
	// The buyer is the one who acts, because the buyer is the one who wants
	// something. Whether the seller parts with it is not a question put to a
	// controller: it is worked out from the seller's own state when the buyer
	// arrives, the same way a courtship is either accepted or not.
	ActBuy

	// ActCook is standing still and making what is in the hand into
	// something that mends (stage 52). The fifteenth word, added at the end
	// like the seven before it.
	//
	// It is the first word whose whole product is an item that is worth more
	// to somebody else than it was before - which is the thing an exchange
	// needs and the thing stages 48 to 51 could not find. What it costs is
	// time, in the shape stage 32 and stage 49 already use.
	ActCook

	// numActionKinds is how many there are, for the code that has to range
	// over them (the rules of thumb of stage 12c). It is not an action.
	numActionKinds
)

func (k ActionKind) String() string {
	switch k {
	case ActMove:
		return "move"
	case ActEat:
		return "eat"
	case ActAttack:
		return "attack"
	case ActFlee:
		return "flee"
	case ActObserve:
		return "observe"
	case ActCourt:
		return "court"
	case ActInvite:
		return "invite"
	case ActTake:
		return "take"
	case ActThrow:
		return "throw"
	case ActGive:
		return "give"
	case ActOffer:
		return "offer"
	case ActStore:
		return "store"
	case ActBuy:
		return "buy"
	case ActCook:
		return "cook"
	default:
		return "rest"
	}
}

// Action is one decision.
//
// Effort says how much vitality the agent is willing to pour into the action.
// More effort means moving faster or hitting harder, at a cost that grows
// faster than the benefit; going all out also empties the agent's reserve and
// leaves it defenceless, which the utility formula prices in on its own rather
// than through a hardcoded limit.
type Action struct {
	Kind     ActionKind
	TargetID int     // food item for ActEat/ActTake, agent for ActAttack/ActFlee/ActObserve/ActCourt/ActInvite, store index for ActStore
	DX, DY   float64 // unit direction, only used by ActMove
	Effort   float64 // 0..1

	// Stance is how the effort is split between hitting, guarding and getting
	// out of the way. It only means anything for the fighting actions; see
	// stance.go.
	Stance Stance
}

// Controller decides what an agent does. The engine calls Decide only when
// something happened that is worth reconsidering (see World.shouldDecide), not
// on every tick.
//
// Swapping the AI for a human player is a matter of installing a different
// Controller on one agent: the engine drives whatever comes back the same way.
type Controller interface {
	Decide(p *Perception) Action
}
