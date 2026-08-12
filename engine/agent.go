package engine

// Ability values (power, rationality) are always kept inside this range.
const (
	MinAbility = 1.0
	MaxAbility = 100.0
)

// Sex of an agent. Only opposite sexes can form a pair.
type Sex uint8

const (
	Male Sex = iota
	Female
)

func (s Sex) String() string {
	if s == Female {
		return "female"
	}
	return "male"
}

// State is what an agent is currently doing. Survival (foraging) always takes
// priority over reproduction, so an agent only reaches StateSeekMate once it has
// enough food stored.
type State uint8

const (
	StateForage State = iota
	StateSeekMate
	StatePaired
)

func (s State) String() string {
	switch s {
	case StateSeekMate:
		return "seek_mate"
	case StatePaired:
		return "paired"
	default:
		return "forage"
	}
}

// rejectKind tells apart the two things an agent can decide to walk away from.
type rejectKind uint8

const (
	rejectFood rejectKind = iota
	rejectMate
)

type rejectKey struct {
	kind rejectKind
	id   int
}

// Agent is one human, simplified to a single node.
type Agent struct {
	ID   int
	X, Y float64

	// VX, VY are only used while wandering, so that an agent without a target
	// keeps a smooth direction instead of jittering in place.
	VX, VY float64

	Sex Sex

	// Abilities.
	Power       float64
	Rationality float64

	// Resources. Food is both the fuel to stay alive and an advantage in a
	// contest; time is spent implicitly, by staying in a state.
	Food float64

	Age        int
	MaxAge     int
	Generation int
	Alive      bool

	State         State
	CooldownTimer int // ticks left before this agent may look for a mate again
	PairTimer     int // ticks left in the current bond
	PartnerID     int // 0 when the agent has no partner

	// courtStartTick is when the agent started comparing candidates. The longer
	// it has been comparing, the more willing it is to commit.
	courtStartTick int

	// rejected holds food items and candidates this agent recently walked away
	// from, mapped to the tick at which they become interesting again. The map
	// is allocated lazily: most agents never reject anything.
	rejected map[rejectKey]int
}

func (a *Agent) reject(kind rejectKind, id, until int) {
	if a.rejected == nil {
		a.rejected = make(map[rejectKey]int, 4)
	}
	a.rejected[rejectKey{kind, id}] = until
}

func (a *Agent) isRejected(kind rejectKind, id int) bool {
	if a.rejected == nil {
		return false
	}
	_, ok := a.rejected[rejectKey{kind, id}]
	return ok
}

// pruneRejected drops the entries whose cooldown has expired. Iterating a map is
// order dependent, but only deletions happen here, so it does not affect
// reproducibility.
func (a *Agent) pruneRejected(tick int) {
	for k, until := range a.rejected {
		if tick > until {
			delete(a.rejected, k)
		}
	}
}

// Food is one edible item lying in the world.
type Food struct {
	ID   int
	X, Y float64
}
