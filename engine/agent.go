package engine

import (
	"slices"
	"strconv"
)

// Ability values are always kept inside this range.
const (
	MinAbility = 1.0
	MaxAbility = 100.0

	// midAbility is the reference an ability is measured against, so that an
	// agent of average power does exactly Config.AttackDamage per tick.
	midAbility = (MinAbility + MaxAbility) / 2
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

// Species is which kind of node an agent is.
//
// Only humans exist today, and nothing in the engine branches on this: no rule
// reads it, and it is not in Perception. It exists ahead of stage 11 (the
// generalised species and the enemies) because the census in census.go counts
// per species, and a measurement that has to be redefined once there is
// something to measure would lose its baseline. The zero value is a human, so
// every agent built without naming a species is one; when enemies arrive they
// will have to say so.
type Species uint8

const (
	SpeciesHuman Species = 0

	// SpeciesEnemy is the other kind of creature: bigger, living on meat, and
	// run by exactly the same rules. It is not a special case in the engine -
	// only the range its budget is drawn from and what it can digest differ.
	SpeciesEnemy Species = 1
)

func (s Species) String() string {
	switch s {
	case SpeciesHuman:
		return "human"
	case SpeciesEnemy:
		return "enemy"
	}
	return "species " + strconv.Itoa(int(s))
}

// State is a summary of what an agent is up to, derived from its current
// action. It carries no rules of its own; it exists so that the viewer and the
// tests can talk about behaviour without inspecting actions.
type State uint8

const (
	StateForage State = iota
	StateSeekMate
	StatePaired
	StateFighting
	StateFleeing
	StateResting
)

func (s State) String() string {
	switch s {
	case StateSeekMate:
		return "seek_mate"
	case StatePaired:
		return "paired"
	case StateFighting:
		return "fighting"
	case StateFleeing:
		return "fleeing"
	case StateResting:
		return "resting"
	default:
		return "forage"
	}
}

// Agent is one human, simplified to a single node.
type Agent struct {
	ID   int
	X, Y float64

	// VX, VY hold the direction of an agent that is only wandering, so that it
	// keeps a smooth heading instead of jittering in place.
	VX, VY float64

	Sex Sex

	// Species is what kind of node this is; see the type. Always human today.
	Species Species

	// Genome is everything this agent inherited, addressed by Gene; see
	// genome.go. The roles are kept apart: attack is how much damage a unit of
	// effort buys, rationality is how accurately the agent reads the world,
	// and intelligence is how good a move it can pick once it has read it.
	//
	// Read it through Gene or the named accessors rather than by index. It is
	// a slice, so a copy of an Agent shares it: never write to a genome
	// through a copy of an agent, only through the one the world holds.
	//
	// When ageing arrives the expressed value becomes "inherited talent x age
	// factor"; for now the age factor is always 1.
	Genome []float64

	// The three state axes. Food is not stored: it lies in the world and
	// eating it lowers hunger.
	Vitality float64
	Hunger   float64

	Age        int
	Generation int
	Alive      bool

	// Maturity is how far through growing up this agent is, 0 at birth and 1
	// when it is done. Food moves it, not time: see World.metabolise. It is
	// the young end of AgeFactor, and it is why a child is weak.
	//
	// Agents the world puts into it fully grown - the founders, and the
	// enemies that walk in from off the map - start at 1.
	Maturity float64

	// Lifespan is a background wear budget, spent only by World.metabolise
	// (chronic starving or overeating). It is deliberately absent from
	// Perception and from the utility formula: an agent has no way to know
	// it, plan around it, or act because of it. Reaching zero kills the
	// agent, the same as Vitality reaching zero, but the two are independent
	// causes of death.
	Lifespan float64

	// Lineage. Recorded from the start so that a player can later take over one
	// of their own descendants when the agent they were playing dies.
	ParentIDs [2]int
	ChildIDs  []int

	// GuardianID is the parent this one is still keeping close to, and
	// RearingTimer how much longer for. Nothing is fed and nothing is given:
	// the child simply does not wander off, and what it gets out of it is
	// whatever its parent does to the rivals standing around them.
	GuardianID   int
	RearingTimer int

	// frailTicks counts how long this agent has been below the vitality it
	// takes to be alright, which is what the slow lifespan cost is charged on.
	frailTicks int

	State  State
	Action Action

	// Pairing.
	PartnerID     int // 0 when the agent has no partner
	PairTimer     int // ticks left in the current bond
	CooldownTimer int // ticks before this agent may look for a mate again

	// courtStartTick is when the agent became well enough off to think about
	// offspring, which is when it starts comparing candidates. The longer it
	// has been comparing, the more willing it is to settle. reproReady is the
	// previous tick's answer, so the clock is only restarted on the way in.
	courtStartTick int
	reproReady     bool

	// controller is what decides this agent's actions. Nil means the world's
	// shared AI controller.
	controller Controller

	// Decision bookkeeping. An agent re-decides on a trigger, not every tick.
	// pendingTrigger says which one raised needsDecision, so that a trace can
	// report why the agent was asked rather than only what it answered.
	lastDecisionTick   int
	vitalityAtDecision float64
	needsDecision      bool
	pendingTrigger     Trigger

	// trace is where this agent's decisions are recorded, nil unless somebody
	// asked to follow it (World.TrackDecisions).
	trace *traceLog

	// attackerID is who hit this agent last tick, 0 if nobody. Being attacked
	// is itself a trigger to think again.
	attackerID     int
	lastAttackTick int

	// lore is what this agent assumes about the world and what it wants out
	// of it: the figures the utility formula used to take from the config, now
	// its own. See lore.go for which of them it can be wrong about.
	lore lore

	// hints are this agent's rules of thumb (stage 12c, hint.go), and
	// hintSlots the room it paid budget for. The two are not the same number:
	// an empty slot still cost, and is what somebody else's idea can be
	// copied into.
	hints     []Hint
	hintSlots int

	// recentFood is how much of each kind this agent has eaten lately, and
	// dietTick when that was last written (stage 16, diet.go). It fades on
	// reading, like every other quantity that fades. It is not a state axis:
	// there are still three of those, and this is bookkeeping.
	recentFood [NumFoodKinds]float64
	dietTick   int

	// regions is what this agent has made of the ground it has been on (stage
	// 15b, regionlore.go). Allocated on the first look, and never counted
	// against what it can remember about people: somewhere is not somebody.
	regions []regionView

	// timesTaught is how often this agent has been in a trade of what it
	// assumes (stage 12b), on either side of it. Nothing reads it: it is there
	// so that the measurement can ask whether a few agents are teaching
	// everybody, which is the failure mode the exchange was shaped to avoid.
	timesTaught int

	// looks is what this agent has made of appearance: the line it fits from
	// how big somebody is to how hard they hit. It is not a memory of anybody
	// in particular, so it is not held in opinions and does not compete for
	// room there.
	looks looksModel

	// Who this agent is currently laying into and since when. It is what makes
	// "did that one hit back" a question about an engagement rather than about
	// a tick: the answer is read once, a few ticks in, so that a long one
	// sided beating counts once and not eighty times. engageLast is the last
	// tick a blow landed, which is how breaking off and coming back later is
	// told apart from carrying on.
	engageID    int
	engageStart int
	engageLast  int

	// hitBy remembers everybody who has landed a blow recently and when, so
	// that a carcass can be left to the ones who brought it down rather than
	// to whoever happens to be standing nearby. Allocated on the first blow.
	hitBy map[int]int

	// effortSpent is the effort actually used this tick, which is what stops an
	// agent from recovering while it is exerting itself.
	effortSpent float64

	// actionTicks counts how long the current action has been running, which is
	// what makes an action that takes time (watching somebody) possible.
	actionTicks int

	// opinions is what this agent believes about others: how much it has been
	// hurt by them, what they have given it, and how strong it reckons they
	// are. Allocated lazily, because a young agent has met nobody. There is
	// only room for so many of them: see MemoryCapacity.
	opinions map[int]*Opinion

	// noSpareMemory says a search for a record to give up has already been
	// made and found nothing worth giving up. See World.record: it is a cache
	// of an answer that cannot change until the set of records does, and it is
	// cleared whenever one joins or leaves.
	noSpareMemory bool

	// memoryUsed is how many records have been taken in during memoryTick,
	// which is what the bandwidth is counted against. The pair resets itself
	// when the tick moves on, so no loop has to clear it.
	memoryTick int
	memoryUsed int

	// rejected holds candidates recently passed over, mapped to the tick at
	// which they become interesting again. Also lazily allocated.
	rejected map[int]int
}

// Controller returns the controller driving this agent, nil when it is run by
// the world's shared AI.
func (a *Agent) Controller() Controller { return a.controller }

// requestDecision asks for a fresh decision on the next tick and records what
// prompted it.
func (a *Agent) requestDecision(t Trigger) {
	a.needsDecision = true
	a.pendingTrigger = t
}

func (a *Agent) reject(id, until int) {
	if a.rejected == nil {
		a.rejected = make(map[int]int, 4)
	}
	a.rejected[id] = until
}

func (a *Agent) isRejected(id int) bool {
	if a.rejected == nil {
		return false
	}
	_, ok := a.rejected[id]
	return ok
}

// pruneRejected drops entries whose cooldown has expired. Map iteration order
// is undefined, but only deletions happen here, so reproducibility is safe.
func (a *Agent) pruneRejected(tick int) {
	for id, until := range a.rejected {
		if tick > until {
			delete(a.rejected, id)
		}
	}
}

// isKin reports whether the other agent is this one's parent or its child.
// One hop only: there is no tree to walk here, and no generational decay to
// pick a coefficient for.
func (a *Agent) isKin(otherID int) bool {
	if otherID == 0 || otherID == a.ID {
		return false
	}
	if a.ParentIDs[0] == otherID || a.ParentIDs[1] == otherID {
		return true
	}
	return slices.Contains(a.ChildIDs, otherID)
}

// noteHit records a blow for the purpose of who has a claim on the carcass.
func (a *Agent) noteHit(from, tick int) {
	if a.hitBy == nil {
		a.hitBy = make(map[int]int, 4)
	}
	a.hitBy[from] = tick
}

// recentAttackers is everybody who has hit this agent within the last window
// ticks, in ascending order of ID so that a carcass is deterministic.
func (a *Agent) recentAttackers(tick, window int) []int {
	if len(a.hitBy) == 0 {
		return nil
	}
	out := make([]int, 0, len(a.hitBy))
	for id, at := range a.hitBy {
		if tick-at <= window {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

// CanReproduce reports whether the agent is well enough off to spend time on
// priority 2. Staying alive comes first: a hungry or battered agent does not
// court, however attractive the candidate next to it. Nor does one that has
// not finished growing up - childhood is a real span of the world's time, and
// what it costs is the generations it holds up.
func (a *Agent) CanReproduce(cfg *Config) bool {
	return a.Maturity >= cfg.ReproMaturity &&
		a.Hunger < cfg.ReproHunger &&
		a.Vitality >= cfg.ReproVitalityShare*a.MaxVitality(cfg) &&
		a.CooldownTimer <= 0
}

// Food is one edible item lying in the world.
type Food struct {
	ID   int
	X, Y float64

	// What it is, and for meat, whose kind it came from: nobody eats its own
	// dead. See food.go.
	Kind FoodKind
	From Species

	// Claim is who brought the carcass down, and ClaimUntil is when it stops
	// mattering. An empty claim is anybody's.
	Claim      []int
	ClaimUntil int

	// SpoilAt is when meat is gone, 0 for anything that does not spoil.
	SpoilAt int
}
