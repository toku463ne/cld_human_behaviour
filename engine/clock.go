package engine

import "math"

// The world's clock, and the fact that not everybody keeps the same hours
// (stage 18, decision #46).
//
// Two things had to be decided here that the plan deliberately left open.
//
// What varies with the hour: how well an agent rests. Not sight, not the food
// spawn, not the exposure. Resting is the one thing the world already prices
// per agent and per moment, so putting the hour into it needs no new formula -
// the recovery an agent expects from lying down is multiplied by how close the
// hour is to its own, and everything downstream of recovery follows. Making
// sight or food vary with the hour would have been a change to the world for
// everybody at once, which is a different rule and not this one: what is being
// asked here is whether agents differing from each other produces anything.
//
// Whether the chronotype is a budget gene: no, and this is the more important
// of the two. The nine genes are quantities - you can buy more attack, more
// memory, more speed, and the budget is what stops you buying all of them. A
// chronotype is not a quantity. It is a phase on a circle, and 0.9 is not more
// than 0.1, it is next to it. Putting it in the budget would say that being a
// night owl costs you the ability to fight, which is not a trade anything in
// biology makes. So it is inherited the way the preferences in lore.go are -
// from one parent, whole, with a mutation - and costs nothing.
//
// What makes it matter rather than decorate: a neighbour you trust only
// discounts the danger of lying down if it is awake (see AIController.survey).
// That is a condition on a term that already existed rather than a new one,
// and it is what turns "we happen to sleep at different times" into "somebody
// is watching" without anybody being assigned to watch.

// phase is where the world is in its day, from 0 to 1. A world with no clock
// is always at nought, which is the same for everybody and therefore no clock
// at all.
func (w *World) phase() float64 {
	if w.cfg.TicksPerDay <= 0 {
		return 0
	}
	return float64(w.tick%w.cfg.TicksPerDay) / float64(w.cfg.TicksPerDay)
}

// restFit is how well this hour suits this agent for sleeping, from -1 (its
// opposite hour) through 0 to +1 (its own).
//
// Circular, because the day is. The distance between 0.95 and 0.05 is a tenth
// of a day and not nine tenths, and a chronotype that did not know that would
// have a seam in it at midnight.
func (w *World) restFit(a *Agent) float64 {
	if w.cfg.TicksPerDay <= 0 {
		return 0
	}
	return math.Cos(2 * math.Pi * (w.phase() - a.chronotype))
}

// restRate is what an agent actually recovers per tick at this hour: the
// world's rate, scaled by how well the hour suits it.
//
// It never goes to nothing. An agent kept awake by the clock still mends, just
// less well - the world's one rule that must not be broken is that there is
// always a way back (CLAUDE.md), and an hour that made recovery impossible
// would break it for half the population half the time.
func (w *World) restRate(a *Agent) float64 {
	if w.cfg.TicksPerDay <= 0 || w.cfg.RestPhaseDepth <= 0 {
		return w.cfg.RegenRate
	}
	depth := clamp(w.cfg.RestPhaseDepth, 0, 0.9)
	return w.cfg.RegenRate * (1 + depth*w.restFit(a))
}

// drawChronotype is the hour a founder keeps. Spread zero puts everybody on the
// same clock, which is the arm this stage is measured against - and draws
// nothing, so that world is the one from before the clock existed.
func (w *World) drawChronotype() float64 {
	if w.cfg.TicksPerDay <= 0 || w.cfg.ChronotypeSpread <= 0 {
		return 0
	}
	return w.rng.Float64() * clamp(w.cfg.ChronotypeSpread, 0, 1)
}

// inheritChronotype is the hour a child keeps: one parent's, whole, with a
// mutation. Particulate, as the preferences are, and for the same reason -
// averaging two parents would leave everybody on the same clock within a few
// generations, which is precisely the thing being tested.
func (w *World) inheritChronotype(pa, pb *Agent) float64 {
	if w.cfg.TicksPerDay <= 0 || w.cfg.ChronotypeSpread <= 0 {
		return 0
	}
	c := pa.chronotype
	if w.rng.Float64() < 0.5 {
		c = pb.chronotype
	}
	if w.cfg.ChronotypeMutation > 0 {
		c += w.rng.NormFloat64() * w.cfg.ChronotypeMutation
	}
	// Wrapped rather than clamped: an hour is a point on a circle, and
	// clamping would pile every mutation that ran past midnight onto midnight.
	return c - math.Floor(c)
}

// --- reading it out ---------------------------------------------------------

// Hour is where the world is in its day, from 0 to 1, and 0 when it has no
// day. For the viewer; read only.
func (w *World) Hour() float64 { return w.phase() }

// ClockOf is the hour one agent sleeps best at and how well the present hour
// suits it, from -1 to 1. For the viewer; read only.
func (w *World) ClockOf(id int) (chronotype, fit float64) {
	a := w.agentByID(id)
	if a == nil {
		return 0, 0
	}
	return a.chronotype, w.restFit(a)
}

// Vigilance is whether anybody is ever awake.
//
// It is the completion condition of the stage, and it is measured over groups
// rather than over the world: the claim is that a group is rarely all asleep at
// once, and a world where half the population is awake in a different valley is
// not the same thing.
type Vigilance struct {
	// AllResting is the share of groups, over the moments sampled, in which
	// every member was resting at the same time. The Hadza result this is
	// modelled on is that such moments are close to absent.
	AllResting float64

	// Groups is how many group-moments went into that, so a share off almost
	// nothing can be told from one off plenty.
	Groups float64

	// Spread is how varied the population's clocks actually are, on a circle:
	// 0 when everybody keeps the same hours, 1 when they are scattered evenly
	// round the day. Without it a share means nothing - a population that all
	// sleeps at once may simply have converged.
	Spread float64
}

// Vigilance reports how often a group is entirely asleep. Read only, and it
// walks the clusters, so sample it rather than calling it every tick.
func (w *World) Vigilance(linkDist float64) Vigilance {
	var out Vigilance
	cl := w.Clusters(linkDist)

	members := make(map[int][]int)
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive || i >= len(cl.Labels) {
			continue
		}
		if g := cl.Labels[i]; g > 0 {
			members[g] = append(members[g], i)
		}
	}
	for _, group := range members {
		if len(group) < 2 {
			continue
		}
		out.Groups++
		allAsleep := true
		for _, i := range group {
			if w.agents[i].Action.Kind != ActRest {
				allAsleep = false
				break
			}
		}
		if allAsleep {
			out.AllResting++
		}
	}
	if out.Groups > 0 {
		out.AllResting /= out.Groups
	}

	// The spread of the clocks, as the length of the mean direction on a
	// circle: 1 minus that, so 0 is everybody together and 1 is scattered.
	var sx, sy, n float64
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		sx += math.Cos(2 * math.Pi * a.chronotype)
		sy += math.Sin(2 * math.Pi * a.chronotype)
		n++
	}
	if n > 0 {
		out.Spread = 1 - math.Hypot(sx, sy)/n
	}
	return out
}
