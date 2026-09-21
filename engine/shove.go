package engine

import "math"

// Choosing to push (TODO 16, #139).
//
// Stage 13 (#136, knockback.go) made a blow move the body it landed on. That
// was something done to a body, with nothing choosing it. This is the other
// half: a fourth way of carrying yourself in a fight, where the effort goes
// into moving the other one rather than into hurting it.
//
// Three things about the shape of it, all of them decided before a line of it
// was written (PLAN.md, #139).
//
// It is a stance and not a word of its own. ActShove would have been the
// twentieth action, and the twentieth action widens what a rule of thumb can
// be about (numActionKinds), which moves the hint draw in every world whether
// or not it has the rule. A stance costs nothing anybody else has to know
// about - and what is learnt about pushing is learnt as lore rather than as a
// hint, so nothing is lost by the move not being nameable.
//
// What is learnt is one number: how often a push actually opens the gap. That
// is a fact about the world in lore.go's sense - it has a right answer, the
// world knows it, and a body finds it out by living - so it sits beside "does
// the one you hit hit back" and rides the same trade. Two bodies that watch
// each other swap it, and the swap already pays both of them in goodwill
// (AffinityLore), which is the whole of the user's "the knowledge spreads and
// makes friends as it goes": there was nothing to build for it.
//
// The ground behind the other one is read, not remembered. Where a ledge is
// changes every time either body moves, so it is perception; whether pushing
// is worth anything does not, so that is lore. That split is what keeps the
// belief a single number instead of a map.

// shoveProbe is how far ahead of a body the measurement below looks for a
// ledge or a river. It is a measuring stick and not a rule, so it is here
// rather than in Config (CLAUDE.md): counts taken at different reaches are not
// comparable with each other, and the point of this one is to be comparable
// across every map and every arm.
//
// A third of CombatRadius, which is the push the arms of stage 13 used and the
// one a blow of ordinary size gives.
const shoveProbe = 5

// noteShoveGround counts what a chosen push would have to work with: of the
// bodies in sight when somebody decides, how many have a drop or water just
// behind them, on the line from the one looking.
//
// It is the count stage 13 could only take per blow (6.3-14.6% of blows landed
// below a ledge, 0.13-2.05% in water). Per blow is a lower bound on what a
// rule that chooses could aim for, because a body that is going to pick its
// moment sees far more moments than it takes. This counts the moments.
//
// Read only, taken whether or not the rule is on, and it draws no random
// numbers.
func (w *World) noteShoveGround(a *Agent, p *Perception) {
	if w.ground == nil {
		return
	}
	for i := range p.Others {
		o := &p.Others[i]
		dx, dy := o.X-a.X, o.Y-a.Y
		d := math.Hypot(dx, dy)
		if d < 1e-9 {
			continue
		}
		here := w.terrainAt(o.X, o.Y)
		there := w.terrainAt(o.X+dx/d*shoveProbe, o.Y+dy/d*shoveProbe)
		near := d <= w.cfg.CombatRadius

		w.shoveLooks++
		if near {
			w.shoveNear++
		}
		if there.Height < here.Height {
			w.shoveLedge++
			if near {
				w.shoveNearLedge++
			}
		}
		if there.Drown > 0 && here.Drown == 0 {
			w.shoveWater++
			if near {
				w.shoveNearWater++
			}
		}
	}
}

// shoveReachFor is how far a body of this much power would throw one of the
// world's reference size, pushing with everything. It is what the perception
// probes with and what the controller reckons with, so that what a body looks
// at and what it expects are the same figure.
//
// The mass is left out of it. A body cannot see how much there is of the one
// in front of it - MaxVitality is not in AgentView and is not going into it -
// so what it reckons with is a body of ordinary size, and it is wrong about
// every body that is not. That error is the whole reason the belief below has
// anything to find out.
func shoveReachFor(cfg *Config, power float64) float64 {
	if cfg.ShovePush <= 0 || cfg.AttackDamage <= 0 {
		return 0
	}
	m := stanceMix[StanceShoving]
	return cfg.ShovePush * damagePerTick(cfg, power, m.Shove) / cfg.AttackDamage
}

// groundBehind is what this body makes of the ground just behind the one it
// is looking at: the fall a push would cost it, and the water it would land
// in.
//
// It is read rather than remembered. Where a ledge is changes every time
// either body moves, so it cannot be a belief; whether pushing is worth
// anything does not, so that is.
//
// One draw of the ordinary reading error, of the size every other reading of
// the ground carries (stage 100), and the same draw moves both figures
// because a body that misjudges a piece of ground misjudges it in one way
// rather than in two. The draw is made whatever the ground turns out to be,
// so that how many random numbers a look consumes does not depend on the map.
func (w *World) groundBehind(a, o *Agent, unit float64) (fall, drown float64) {
	e := w.noise(unit, w.cfg.GroundAheadNoise)
	dx, dy := o.X-a.X, o.Y-a.Y
	d := math.Hypot(dx, dy)
	if d < 1e-9 {
		return 0, 0
	}
	reach := shoveReachFor(&w.cfg, a.Attack(&w.cfg))
	if reach <= 0 {
		return 0, 0
	}
	here := w.terrainAt(o.X, o.Y)
	there := w.terrainAt(o.X+dx/d*reach, o.Y+dy/d*reach)
	if drop := int(here.Height) - int(there.Height); drop > 0 {
		fall = math.Max(0, w.cfg.KnockbackFall*float64(drop)*(1+e))
	}
	if there.Drown > 0 && here.Drown == 0 {
		drown = math.Max(0, there.Drown*(1+e))
	}
	return fall, drown
}

// readsBehind says whether anybody in this world looks behind the one in
// front of them. False everywhere the rule is off, which is what keeps the
// draw above out of every world that has never heard of pushing.
func (w *World) readsBehind() bool {
	return w.cfg.ShovePush > 0 && w.cfg.ShoveGroundSeen && w.ground != nil
}

// noteShoveOutcome is the one thing a chosen push teaches: whether it opened
// the gap.
//
// "Opened the gap" is the push putting the two outside CombatRadius, read
// straight after the shove. Of the two definitions the design weighed - that
// one, and "the pusher could choose something other than fighting on its next
// decision" - this is the one the utility formula actually uses, so it is the
// one worth learning. A belief about something the formula does not ask about
// would be a number with nowhere to go.
//
// Both sides of it learn by default, which is new: every other belief in this
// world has one door into it. The event is one event and both of them were
// there - the one who pushed and the one who went backwards know the same
// thing about pushing - but a belief that fills from two doors fills twice as
// fast against the same LoreMemory cap, so ShoveLearnBoth exists to run the
// one-door arm beside it.
//
// It draws no random numbers.
func (w *World) noteShoveOutcome(fromID int, to *Agent) {
	from := w.agentByID(fromID)
	broke := false
	if from != nil && from.Alive {
		broke = dist2(from.X, from.Y, to.X, to.Y) > w.cfg.CombatRadius*w.cfg.CombatRadius
	}
	if broke {
		w.shovesBroke++
		w.noteShoveWait(fromID, to.ID)
	} else {
		w.shovesHeld++
	}
	rate := w.cfg.ShoveLearnRate
	if rate <= 0 {
		return
	}
	v := boolValue(broke)
	if from != nil {
		from.lore.shoveWorks.observe(v, rate, w.cfg.LoreMemory)
	}
	if w.cfg.ShoveLearnBoth {
		to.lore.shoveWorks.observe(v, rate, w.cfg.LoreMemory)
	}
}

// shoveWait is a pair a push put out of each other's reach, waiting to be
// back in it. Measurement only.
type shoveWait struct{ from, to, tick int }

// shoveWaitLimit is how long a pair is followed before the world gives up on
// them. A measuring stick rather than a rule: past this the two have plainly
// gone their own ways, and counting the wait as "for ever" would say more
// about how long the run is than about what a push bought.
const shoveWaitLimit = 300

func (w *World) noteShoveWait(from, to int) {
	w.shoveWaits = append(w.shoveWaits, shoveWait{from: from, to: to, tick: w.tick})
}

// trackShoveWaits is how long it actually takes a pair that a push separated
// to be back within reach of each other. That figure is what a chosen push
// buys - the ticks the other one is not hitting you - and the design would
// not commit to a dose without it.
//
// A straight scan of a short list, and it asks where two named bodies are
// rather than asking the spatial index anything, so it costs nothing and
// breaks no rule about when the index may be questioned.
func (w *World) trackShoveWaits() {
	if len(w.shoveWaits) == 0 {
		return
	}
	r2 := w.cfg.CombatRadius * w.cfg.CombatRadius
	keep := 0
	for _, s := range w.shoveWaits {
		from, to := w.agentByID(s.from), w.agentByID(s.to)
		if from == nil || to == nil || !from.Alive || !to.Alive {
			continue
		}
		if dist2(from.X, from.Y, to.X, to.Y) <= r2 {
			w.shoveRejoins++
			w.shoveWaitSum += w.tick - s.tick
			continue
		}
		if w.tick-s.tick >= shoveWaitLimit {
			continue
		}
		w.shoveWaits[keep] = s
		keep++
	}
	w.shoveWaits = w.shoveWaits[:keep]
}

// ShoveGround is what a chosen push would have to aim at. Read only.
type ShoveGround struct {
	// Looks is (decision x body in sight) pairs; Ledge and Water how many of
	// those had a drop or a river a push away, behind the one being looked at.
	Looks, Ledge, Water int

	// The same three counted only over the bodies already within arm's reach,
	// which is where a push can actually be thrown. The share of these is the
	// one a rule fires at; the share of the others is the one it could aim for
	// if a body ever walked somewhere to get the angle.
	Near, NearLedge, NearWater int
}

// ShoveGround reports them. Read only.
func (w *World) ShoveGround() ShoveGround {
	return ShoveGround{
		Looks: w.shoveLooks, Ledge: w.shoveLedge, Water: w.shoveWater,
		Near: w.shoveNear, NearLedge: w.shoveNearLedge, NearWater: w.shoveNearWater,
	}
}

// Shoving is what the chosen push did. Read only.
type Shoving struct {
	// Made is how many pushes were thrown, Broke how many of those put the
	// two out of each other's reach, and Held the rest. Broke over Made is
	// the truth the belief is trying to find out.
	Made, Broke, Held int

	// Wait is how many ticks a separated pair actually took to be back within
	// reach, averaged over the pairs that came back at all; Rejoins is how
	// many did. A pair that never came back is not in either figure, which is
	// why the two are reported together.
	Wait    float64
	Rejoins int
}

// Shoving reports it. Read only.
func (w *World) Shoving() Shoving {
	out := Shoving{Made: w.shovesMade, Broke: w.shovesBroke, Held: w.shovesHeld, Rejoins: w.shoveRejoins}
	if w.shoveRejoins > 0 {
		out.Wait = float64(w.shoveWaitSum) / float64(w.shoveRejoins)
	}
	return out
}
