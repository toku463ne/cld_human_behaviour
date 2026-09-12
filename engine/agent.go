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

	// Nursing says a child of this one's is within the rearing radius right
	// now (stage 66), which is what slows it down. Written once a tick by
	// World.nurse and read by Agent.speedNow.
	//
	// Not a state axis and nothing accumulates in it: like the footing of
	// stage 57 it is a copy of a fact about this tick, kept on the agent so
	// that the methods which know only the config can reach it.
	Nursing bool

	// HomeRegion is the block of the world this body came into it in (stage
	// 64), as an index, and -1 for a body from a world with no blocks.
	//
	// Fixed once, like Kind: where it arrived or was born, never where it has
	// got to since. Nothing moves it and nothing reads it but the cost of
	// being away from it - a region is still not a wall.
	HomeRegion int

	// Kind is which row of Config.EnemyKinds this body came from (stage 59).
	// Zero for humans, and zero for every enemy in a world with one kind.
	//
	// Fixed when it arrives and never touched again - the same standing as
	// GuardianID: a reference decided at the start, not a state that
	// accumulates. Newborns take their parents' row, so a kind breeds true.
	Kind uint8

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

	// drowned says the ground took this one, and is read once, by kill, so
	// that the death is not also counted as a killing (stage 34). It lives
	// only for the tick the body dies in, which is why it is not saved.
	drowned bool

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

	// The one this body thinks best of and can see, and which way it went
	// when it could not (stage 55a). A direction and never a place: dearID is
	// only who is being kept track of this moment, and what outlives the
	// sighting is the unit vector and when it was taken.
	dearID         int
	lostDX, lostDY float64
	lostAt         int // the tick it was lost, plus one; zero means in sight

	// How this body is doing, and when that was last brought up to date
	// (stage 54). Two scalars for the whole body rather than one per face:
	// a record of who frightened it would be a grudge, and this world has
	// refused to keep one since stage 5. They fade on reading, in the same
	// shape as the risk and affinity it holds about other bodies, so this is
	// bookkeeping and not a new thing a body is made of.
	dread, cheer float64
	moodAt       int

	// carried is what this body is holding (stage 40): the fifth state axis,
	// and the only one whose contents are the world's own items rather than a
	// number. It is not exported - the viewer asks through Perception like
	// everything else - and what is in it counts against the world's
	// allowance for food, so carrying moves food about without making more of
	// it.
	carried []Food

	// sawFoodTick is the last tick this body had anything it could eat in
	// sight (stage 40). Read only, and only by the tally that says how many
	// bodies starve within reach of something - the count that says what
	// carrying could be worth.
	sawFoodTick int

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

	// sawFood and sawMate are what was in sight when this agent was last
	// asked whether anything had turned up, so that "something came into
	// view" can be told from "something is in view". Without them the two
	// sighting triggers fire on every tick an agent spends walking through a
	// place with food in it, having decided to keep walking past that same
	// food a tick earlier.
	sawFood bool
	sawMate bool

	// footing is what the ground this agent is standing on does to each of its
	// genes (stage 57). Written once a tick by World.standOnGround and read by
	// Agent.Ability; a zero entry means nobody has said, and reads as one.
	//
	// It is not a state axis and nothing accumulates in it: it is a copy of a
	// property of the ground, kept here because Ability is on the hot path and
	// an agent knows nothing about the world it is standing in. An array
	// rather than a slice so that a copy of an Agent carries its own.
	footing [NumGenes]float64

	// chronotype is the hour of the world's day this agent sleeps best at, on
	// a circle from 0 to 1 (stage 18, clock.go). Deliberately not one of the
	// budget genes: those are quantities you can buy more of, and an hour is a
	// direction rather than an amount.
	chronotype float64

	// seed is a plant this agent ate that survived being eaten, and seedDueAt
	// when it comes up (stage 17c). Zero means it is carrying nothing. One at
	// a time: a gut is not a granary.
	seed      plantGenes
	seedDueAt int

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

	// stores is which of the world's caches this agent could find (stage 50).
	// Allocated the first time it learns one, never counted against what it
	// can remember about people, and it fades at the same rate country does:
	// somewhere is not somebody, and a place nobody goes to is forgotten.
	stores []storeMemory

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

	// openUntil is the tick up to which a blow that found nothing has left this
	// agent off balance. It is bookkeeping and not a state axis: nothing reads
	// it but the resolution of blows, it is not in the perception, and no
	// agent can plan around it.
	openUntil int

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

	// criedAt is when this agent last started crying its wares (stage 49).
	// Nothing in the world reads it: it is there so that a gift can be told
	// apart from a gift that followed an advertisement.
	criedAt int

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

	// courtedBy is whoever is standing here waiting for an answer, and
	// courtedTick when they arrived. Only an agent whose controller answers
	// proposals for itself ever has one: for everybody else the answer is the
	// agent's own rule and is given on the spot (courtship.go).
	courtedBy   int
	courtedTick int

	// lastCourt is how the last courtship this one walked up to came out. It
	// is bookkeeping for whoever is watching - no rule reads it - and it is
	// kept here rather than worked out afterwards because by the time anybody
	// looks, the two answers and the numbers behind them are gone.
	lastCourt CourtView

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

// declaredFor is what this agent has taken on and anybody watching can see:
// what it is calling others in against, or what it is already fighting.
//
// It is read off the current action and stored nowhere, exactly as
// AttackingMe and CourtingMe are. A call that outlasted the calling would be a
// piece of state that could go stale - an agent that called and then wandered
// off would still be advertising a hunt - and it would have to be saved,
// cleared and reasoned about. There is no gap for it to fill: going in after
// what it called about is itself a declaration.
func (a *Agent) declaredFor() int {
	switch a.Action.Kind {
	case ActInvite, ActAttack:
		return a.Action.TargetID
	}
	return 0
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

// CanReproduce reports whether courting is open to this agent at all.
//
// There are two different things in here and stage 25 split them, because they
// answer to different rules of the design.
//
//   - What a body cannot do. Growing up takes real time; a child costs real
//     vitality, and one that would not survive paying cannot pay. A cooldown
//     after a birth is the same kind of thing. None of this is a judgement,
//     and it is why courting can be refused outright rather than merely
//     scored badly.
//   - What an agent judges is not worth it now. Being hungry, or battered but
//     alive, is a reason not to court - and reasons belong in the utility
//     comparison, not in a threshold that stops the option being considered.
//     The design note is explicit that no hardcoded behavioural threshold
//     should exist, and this one had been sitting under the priority rule.
//
// CourtNeedsSurplus keeps the old behaviour, where the judgement was made here
// rather than in the comparison. See HISTORY.md for what each costs.
func (a *Agent) CanReproduce(cfg *Config) bool {
	if !a.IsAdult(cfg) || a.CooldownTimer > 0 {
		return false
	}
	// The parents share the cost of the birth, so this is what one of them
	// has to have and still be alive afterwards.
	if a.Vitality <= cfg.BirthVitalityCost/2 {
		return false
	}
	if !cfg.CourtNeedsSurplus {
		return true
	}
	return a.Hunger < cfg.ReproHunger &&
		a.Vitality >= cfg.ReproVitalityShare*a.MaxVitality(cfg)
}

// IsAdult reports whether the agent has finished growing up. It is the one
// place the line between a child and an adult is drawn: courting is not
// considered before it (CanReproduce), and a player's controller is not handed
// to a child before it either (World.Heirs).
func (a *Agent) IsAdult(cfg *Config) bool { return a.Maturity >= cfg.ReproMaturity }

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

	// Store is which cache this item is in, plus one, and zero for the far
	// more usual case of lying in the open (stage 50). It is a place and not
	// a container: an item in a store is in the world's own list, counts
	// against the world's allowance, and goes off at the ordinary rate. The
	// only thing being in one does to it is hide it from anybody who does not
	// know the place.
	Store int

	// Genes is what a plant inherited from the one it grew from (plant.go).
	// Meat has none: a carcass is not a lineage.
	Genes plantGenes

	// Special marks the awkward crop of stage 44: a plant that takes knowing
	// to get out of the ground. It is a property of the item rather than a
	// kind of its own, because it feeds a body exactly what a plant feeds a
	// body - what differs is how often the attempt comes off.
	Special bool

	// Cooked is how well this item was prepared (stage 52), zero for
	// everything the world grew and everything nobody has done anything to.
	// It is a property of the item for the same reason Special is, and it
	// holds a figure rather than a flag because how good it is is the whole
	// point: it is set from the cook's own skill, so what changes hands is
	// somebody else's work and not merely somebody else's dinner.
	Cooked float64
}
