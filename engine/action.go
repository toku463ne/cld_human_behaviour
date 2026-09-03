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
	TargetID int     // food item for ActEat, agent for ActAttack/ActFlee/ActObserve/ActCourt
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
