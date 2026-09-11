package engine

import "math"

// Which way somebody went (stage 55a).
//
// Everything this world has built for finding a place again works on regions:
// the richness of one (15b), the cost of crossing one (29), how likely one is
// to drown you (35). All three are the same shape, all three are read at the
// grain of a twelfth of the world, and all three ended the same way - the
// belief grows accurate, it spreads, and nobody's feet move (the four times
// counted at stages 30b, 31, 35 and 49).
//
// So this is the cheapest thing that could serve instead, and it is not a
// place at all. When the body it thinks best of goes out of sight, it keeps
// the direction that body was last in - one unit vector, fading - and while
// that is still worth anything there is one extra way to wander: that way.
//
// What it deliberately is not.
//
//   - Not a coordinate. Nothing here can be walked to, checked, or told to
//     anybody. A remembered place would need a new kind of memory and a way to
//     head for somewhere unseen (#70, still shelved); a direction needs
//     neither, and it goes stale on its own.
//   - Not a rule about children or mates. Stage 7d decided the survival of a
//     child is in no parent's utility, and that stands: what this follows is
//     whoever the body thinks best of, which is often kin and often not.
//   - Not an override. It is one more candidate in the same comparison, in
//     exactly the shape stage 15b's walk to better country already takes -
//     scored, costed, and beaten by anything better.
//   - Not held against time. A direction with nothing behind it would walk a
//     body off the edge of the world for ever, so it fades by half every
//     LonelyHalfLife and is dropped when it is worth less than a twentieth.

// dearestInSight is the id of the body this one thinks best of and can see,
// and where it is. Zero when there is nobody it thinks well of in sight -
// which, counted before this was built, is 46% of the time.
func (w *World) noteDearest(a *Agent, id int, x, y float64) {
	if w.cfg.LonelyValue <= 0 {
		return
	}
	if id != 0 {
		// In sight: there is nothing to miss, and the only thing worth
		// keeping is which way they are. The direction is taken now, while
		// they can be seen, rather than reconstructed afterwards - which is
		// what keeps this a direction and never a place. There is no moment
		// at which a coordinate is written down.
		a.dearID, a.lostAt = id, 0
		if dx, dy := x-a.X, y-a.Y; math.Hypot(dx, dy) > 1e-9 {
			d := math.Hypot(dx, dy)
			a.lostDX, a.lostDY = dx/d, dy/d
		}
		return
	}
	if a.dearID == 0 || a.lostAt != 0 {
		return // nobody was being kept track of, or the loss is already noted
	}
	// It was in sight last time and is not now. What is left is the way it
	// was last, which has been kept up to date all along.
	if a.lostDX != 0 || a.lostDY != 0 {
		a.lostAt = w.tick + 1 // plus one, so that tick zero is not "in sight"
		w.lostSight++
	}
	a.dearID = 0
}

// missing is how much the last direction is still worth, from 0 to 1, and
// which way it points. Zero once it has faded past the floor - and the memory
// is dropped then, so nothing goes on pointing at an empty quarter of the
// world for the rest of a life.
func (w *World) missing(a *Agent) (float64, float64, float64) {
	if w.cfg.LonelyValue <= 0 || a.lostAt == 0 || w.cfg.LonelyHalfLife <= 0 {
		return 0, 0, 0
	}
	weight := math.Exp(-math.Ln2 * float64(w.tick-(a.lostAt-1)) / float64(w.cfg.LonelyHalfLife))
	if weight < lonelyFloor {
		a.lostAt, a.lostDX, a.lostDY = 0, 0, 0
		return 0, 0, 0
	}
	dx, dy := a.lostDX, a.lostDY
	if w.cfg.LonelyWrongWay {
		// The control (stage 54's shape): the same candidate, the same weight,
		// the same cost, and the one thing that differs is that the direction
		// is wrong. It draws no random number, so the two arms run down the
		// same stream - which the arm that drew a fresh direction would not.
		dx, dy = -dx, -dy
	}
	return weight, dx, dy
}

// lonelyFloor is where a direction stops being worth carrying. Not in Config:
// it is the point at which the term is smaller than the noise on any score
// that would use it, rather than a knob on the world.
const lonelyFloor = 0.05

// Lonely is what the population is doing about it. Read only.
type Lonely struct {
	// Lost is how many times a body lost sight of the one it thinks best of,
	// Missing the share of the living carrying a direction, and Draws the
	// share of decisions that were a walk along one.
	Lost    int
	Missing float64
	Draws   float64
}

// Loneliness reports what the missing came to.
func (w *World) Loneliness() Lonely {
	out := Lonely{Lost: w.lostSight}
	var n, carrying float64
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		n++
		if weight, _, _ := w.missing(a); weight > 0 {
			carrying++
		}
	}
	if n > 0 {
		out.Missing = carrying / n
	}
	if w.decisions > 0 {
		out.Draws = float64(w.lonelyDraws) / float64(w.decisions)
	}
	return out
}
