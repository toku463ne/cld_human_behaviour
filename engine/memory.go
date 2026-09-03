package engine

import "math"

// Opinion is everything one agent believes about another. Three separate things
// are kept, because they answer different questions:
//
//   - Risk answers "what has this one already cost me?" and drives avoidance.
//     It is the vitality lost in fights with them, and it fades slowly: without
//     forgetting, long lived agents end up remembering everybody as a maximum
//     threat and the world seizes up.
//   - Affinity answers "is this one mine?" and is the first positive thing an
//     agent remembers about anybody. It comes from what actually happened
//     between them - a bond, a birth, being somebody's parent or child - and
//     not from having stood next to each other for a while.
//   - Strength answers "how would a fight with them go?" for someone never
//     fought. True combat power is hidden, so each agent carries an estimate
//     with an uncertainty attached.
//
// One record per other agent, and the three share it. Affinity deliberately
// has no store of its own: a separate one would be free, everybody would end
// up fond of everybody, and there would be one group in the world.
type Opinion struct {
	Risk     float64 // accumulated vitality lost to this agent, decayed
	Affinity float64 // accumulated good this one has done it, decayed

	// lastTick is when Risk and Affinity were last brought up to date. Both
	// fade from the same origin, so meeting somebody keeps the whole record
	// fresh rather than only the half of it that happened to change.
	lastTick int

	Strength float64 // estimate of the other's power
	Variance float64 // how unsure that estimate is
	Samples  int     // observations folded in so far, direct or watched
}

// opinion returns what this agent already believes about another, or nil when
// they have never registered. It is the read path: it allocates nothing, takes
// no room in memory and does not count against the tick's bandwidth, because
// recalling something is not the same as learning it.
func (a *Agent) opinion(otherID int) *Opinion {
	if a.opinions == nil {
		return nil
	}
	return a.opinions[otherID]
}

// opinionOf returns this agent's opinion of another, making room for it if
// need be. It is the path for things that are not optional: being hit, and the
// events that start an affinity. See recordOpinion for the path that can fail.
func (w *World) opinionOf(a *Agent, otherID int) *Opinion {
	return w.record(a, otherID, true, -1)
}

// recordOpinion returns a record to write to, and nil when the agent has no
// room to take somebody new on: its memory is full of people who matter to it,
// or it has already learned as much as it can take in this tick.
func (w *World) recordOpinion(a *Agent, otherID int) *Opinion {
	return w.record(a, otherID, false, -1)
}

// recordOpinionSeen is the same for a caller that has already glanced at the
// other this tick: the glance it took is the one the first guess is made from.
func (w *World) recordOpinionSeen(a *Agent, otherID int, seen float64) *Opinion {
	return w.record(a, otherID, false, seen)
}

// record is the one place a memory is taken on.
//
// Capacity is what makes forgetting a competition rather than a timer: an
// agent that has met more people than it can hold has to give one up to take
// another on, and which one it gives up is decided by what they are worth to
// it (see weakestOpinion). A record nothing has happened around - a stranger
// it merely looked at once - is worth nothing and is the first to go.
//
// forced records are the ones the world imposes: a blow landing, a bond, a
// birth. They ignore the bandwidth and will push out the least valuable
// record held even when it is a valuable one, because an agent does not get
// to decline to notice that it was hit.
// seen is the appearance the caller has already read off the other, or a
// negative number when it has not looked yet and record should look for
// itself if it turns out to need to.
func (w *World) record(a *Agent, otherID int, forced bool, seen float64) *Opinion {
	if a.opinions == nil {
		a.opinions = make(map[int]*Opinion, 4)
	}
	if op, ok := a.opinions[otherID]; ok {
		return op
	}

	// Bandwidth first, because it is the cheap question. Looking for room in a
	// full memory means pricing every record in it, and there is no reason to
	// do that for an agent that could not take the answer in this tick anyway:
	// in a crowd, most of what an agent walks past is refused right here.
	if !forced && !w.hasMemoryBudget(a) {
		return nil
	}

	victim, evicting := 0, false
	if capacity := a.MemoryCapacity(&w.cfg); capacity > 0 && len(a.opinions) >= capacity {
		id, ok := w.weakestOpinion(a, forced)
		if !ok {
			// Nothing in there is expendable: somebody who cost or gave the
			// agent something is not thrown out to make room for a face in
			// the crowd. Only the world's own events get to overwrite a
			// memory that still carries weight.
			return nil
		}
		victim, evicting = id, true
	}
	if !forced {
		w.spendMemory(a)
	}
	if evicting {
		delete(a.opinions, victim)
	}

	// What this agent assumes about somebody it has taken no reading of: its
	// own line, read off the stranger's build (appearance.go). The confidence
	// is unchanged - a better guess is still a guess, and the first real
	// reading has to move it as far as it ever did.
	// Deliberately not written as one expression with a fallback: working out
	// what a stranger looks like costs a glance (and a draw from the random
	// source), so it must not happen when the caller has already taken one.
	guess := 0.0
	if seen >= 0 {
		guess = w.strangerFromLooks(a, seen)
	} else {
		guess = w.strangerStrength(a, otherID)
	}
	w.noteFirstSight(a, guess, otherID)
	op := &Opinion{
		Strength: guess,
		Variance: w.cfg.PriorVariance,
		lastTick: w.tick,
	}
	// Family starts part of the way in. Without it an agent would have to
	// build up a relationship with its own child from nothing, exactly as it
	// would with a stranger, and the head start is the whole reason lineage
	// was recorded in the first place. Only one hop counts: parents and
	// children, never grandchildren or siblings, so there is no tree to walk
	// and no generational decay to invent.
	if w.cfg.AffinityKin > 0 && a.isKin(otherID) {
		op.Affinity = w.cfg.AffinityKin
	}
	a.opinions[otherID] = op
	return op
}

// weakestOpinion is the record this agent would give up first, and whether it
// is willing to give any of them up at all. The order is total - value, then
// how long since it was touched, then the ID - so that a full memory forgets
// the same face in every run of the same seed.
//
// Unless the caller is one of the world's own events, only a record that has
// come to nothing can go: an acquaintance who never cost the agent anything
// and never gave it anything.
func (w *World) weakestOpinion(a *Agent, forced bool) (int, bool) {
	riskRate, affinityRate := w.memoryRates(a)
	worstID, worstWeight, worstTick := 0, 0.0, 0
	found := false
	for id, op := range a.opinions {
		elapsed := w.tick - op.lastTick
		weight := decay(op.Risk, riskRate, elapsed) + decay(op.Affinity, affinityRate, elapsed)
		switch {
		case !found,
			weight < worstWeight,
			weight == worstWeight && op.lastTick < worstTick,
			weight == worstWeight && op.lastTick == worstTick && id > worstID:
			worstID, worstWeight, worstTick, found = id, weight, op.lastTick, true
		}
	}
	if found && !forced && worstWeight > 0 {
		return 0, false
	}
	return worstID, found
}

// memoryRates are how fast this agent's two kinds of memory fade. They are the
// world's rates scaled by what it spent on remembering, and are read out
// together because a loop over a whole memory would otherwise work them out
// once per record.
func (w *World) memoryRates(a *Agent) (risk, affinity float64) {
	scale := a.ForgetScale(&w.cfg)
	return w.cfg.RiskDecayPerTick * scale, w.cfg.AffinityDecayPerTick * scale
}

// hasMemoryBudget reports whether this agent could take anything else in this
// tick, and spendMemory takes one unit of that budget. The counter resets
// itself when the tick moves on, so nothing has to walk the population to
// clear it.
func (w *World) hasMemoryBudget(a *Agent) bool {
	limit := a.MemoryBandwidth(&w.cfg)
	if limit <= 0 {
		return true
	}
	if a.memoryTick != w.tick {
		return true
	}
	return a.memoryUsed < limit
}

func (w *World) spendMemory(a *Agent) {
	if a.MemoryBandwidth(&w.cfg) <= 0 {
		return
	}
	if a.memoryTick != w.tick {
		a.memoryTick, a.memoryUsed = w.tick, 0
	}
	a.memoryUsed++
}

// Opinions returns what an agent believes about the others it has met, keyed by
// their ID. The map is a copy with the fading already applied, which is what
// the viewer shows when a node is clicked.
func (w *World) Opinions(id int) map[int]Opinion {
	a := w.agentByID(id)
	if a == nil || len(a.opinions) == 0 {
		return nil
	}
	out := make(map[int]Opinion, len(a.opinions))
	for otherID, op := range a.opinions {
		c := *op
		c.Risk = w.decayedRisk(a, op)
		c.Affinity = w.decayedAffinity(a, op)
		out[otherID] = c
	}
	return out
}

// decayedRisk and decayedAffinity apply the forgetting curve lazily, so that
// nothing has to be walked over every tick. The curve is the same shape it
// always was; what is new is that how fast it runs is the holder's own, drawn
// from what it spent on memory.
func (w *World) decayedRisk(a *Agent, op *Opinion) float64 {
	return decay(op.Risk, w.cfg.RiskDecayPerTick*a.ForgetScale(&w.cfg), w.tick-op.lastTick)
}

func (w *World) decayedAffinity(a *Agent, op *Opinion) float64 {
	return decay(op.Affinity, w.cfg.AffinityDecayPerTick*a.ForgetScale(&w.cfg), w.tick-op.lastTick)
}

func decay(value, rate float64, elapsed int) float64 {
	if elapsed <= 0 || value == 0 || rate <= 0 {
		return value
	}
	return value * math.Exp(-rate*float64(elapsed))
}

// settle brings a record up to the present, so that something can be added to
// it without the addition being faded along with what was already there.
func (w *World) settle(a *Agent, op *Opinion) {
	op.Risk = w.decayedRisk(a, op)
	op.Affinity = w.decayedAffinity(a, op)
	op.lastTick = w.tick
}

// touch resets where the fading is measured from without changing what is
// remembered. Seeing somebody again is not new information about them, but it
// does keep what is already known from going stale: an agent that once got
// hurt by somebody stays wary of them for as long as they are around, and
// forgets them once they are not.
//
// It costs bandwidth, or keeping a memory alive would be the one thing an
// agent could do without limit.
func (w *World) touch(a *Agent, op *Opinion) {
	if !w.cfg.ContactRefresh || op.lastTick == w.tick || !w.hasMemoryBudget(a) {
		return
	}
	w.spendMemory(a)
	op.lastTick = w.tick
}

// rememberDamage records that another agent cost this one some vitality.
func (w *World) rememberDamage(a *Agent, fromID int, damage float64) {
	op := w.opinionOf(a, fromID)
	w.settle(a, op)
	op.Risk += damage
}

// rememberAffinity records that something good passed between them. The
// callers are the events themselves - a pair forming, a child being born - so
// that affinity only ever comes from something that happened.
func (w *World) rememberAffinity(a *Agent, otherID int, amount float64) {
	w.addAffinity(a, otherID, amount, true)
}

// rememberAffinityIfRoom is the same for something that happened *with*
// somebody rather than *to* them: a trade of what the two of them assume
// (stage 12b). It takes the optional path, so an agent whose memory is full of
// people who matter to it does not throw one of them out for a passing
// exchange with a stranger. What it got out of the meeting it got; what it
// does not get is somebody new to remember.
//
// The difference is not only a rule. A trade happens thousands of times in a
// run where a birth happens a handful, and forcing a record every time made
// looking for the least valuable memory to evict - which prices every record
// an agent holds - half the cost of the whole simulation.
func (w *World) rememberAffinityIfRoom(a *Agent, otherID int, amount float64) {
	w.addAffinity(a, otherID, amount, false)
}

func (w *World) addAffinity(a *Agent, otherID int, amount float64, forced bool) {
	if amount <= 0 || otherID == 0 || a.ID == otherID {
		return
	}
	op := w.record(a, otherID, forced, -1)
	if op == nil {
		return
	}
	w.settle(a, op)
	op.Affinity += amount
}

// observeStrength folds one noisy reading of another agent's power into the
// observer's estimate. The reading is off by an amount that shrinks with the
// observer's rationality, and the estimate's variance shrinks with every
// observation, so watching a lot of fights makes an agent hard to surprise.
//
// The reading is lost when there is no room for it: an agent that has taken in
// all it can this tick, or whose memory is full of people who matter more,
// watches the fight and learns nothing about that individual from it. What it
// does come away with either way is the impression - a body of that size hit
// that hard - because that costs no room (appearance.go).
func (w *World) observeStrength(observer *Agent, target *Agent, obsVariance float64) {
	if observer.ID == target.ID {
		return
	}
	noiseStd := (MaxAbility - observer.Rationality(&w.cfg)) / MaxAbility * w.cfg.JudgementNoise
	reading := target.Attack(&w.cfg) + w.rng.NormFloat64()*noiseStd
	w.learnFromLooks(observer, target, reading)

	// Taking a reading is taking something in, whether or not the observer
	// had heard of the target before, so it costs the same either way.
	op := observer.opinion(target.ID)
	if op == nil {
		if op = w.recordOpinion(observer, target.ID); op == nil {
			return
		}
	} else {
		if !w.hasMemoryBudget(observer) {
			return
		}
		w.spendMemory(observer)
	}

	variance := obsVariance + noiseStd*noiseStd

	k := op.Variance / (op.Variance + variance)
	op.Strength += k * (reading - op.Strength)
	op.Variance *= 1 - k
	op.Samples++
}

// --- reading the state of everybody's memory --------------------------------

// MemoryUse is how much of the population's memory is in use, and what it is
// being used on. It is a read only measurement: nothing in the simulation
// reads it back.
type MemoryUse struct {
	Remembered float64 // records held, per agent
	Friends    float64 // of those, how many carry any affinity
	FullShare  float64 // share of agents holding as many records as they can

	// RestNear is how many agents it has no affinity for are within sight of
	// an agent that is resting, averaged over the agents that are resting. It
	// is what the exposure of resting is meant to move: an agent that only
	// lies down among its own keeps this low.
	RestNear float64
}

// MemoryUse walks the population's memories. It is O(population x capacity)
// and meant to be sampled at intervals, like Spacing and Clusters.
func (w *World) MemoryUse() MemoryUse {
	var out MemoryUse
	if len(w.agents) == 0 {
		return out
	}

	var resting, nearby int
	var scratch []int
	g := w.spatialIndex()
	r := w.cfg.PerceptionRadius

	for i := range w.agents {
		a := &w.agents[i]
		out.Remembered += float64(len(a.opinions))
		for _, op := range a.opinions {
			if w.decayedAffinity(a, op) > 0 {
				out.Friends++
			}
		}
		if capacity := a.MemoryCapacity(&w.cfg); capacity > 0 && len(a.opinions) >= capacity {
			out.FullShare++
		}

		if a.Action.Kind != ActRest {
			continue
		}
		resting++
		scratch = g.appendAgentsNear(scratch[:0], a.X, a.Y, r)
		for _, j := range scratch {
			o := &w.agents[j]
			if o.ID == a.ID || !o.Alive || dist2(a.X, a.Y, o.X, o.Y) > r*r {
				continue
			}
			if op := a.opinion(o.ID); op != nil && w.decayedAffinity(a, op) > 0 {
				continue
			}
			nearby++
		}
	}

	n := float64(len(w.agents))
	out.Remembered /= n
	out.Friends /= n
	out.FullShare /= n
	if resting > 0 {
		out.RestNear = float64(nearby) / float64(resting)
	}
	return out
}
