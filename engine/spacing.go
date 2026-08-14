package engine

import "math"

// This file holds the measurement of how the population is laid out. It exists
// for the cooperation work: whether agents come together in groups has to be
// visible as a number before any rule can be credited with causing it, and
// "they look like they are grouping" from watching cmd/devview is not evidence.
//
// It is kept out of Stats deliberately. Stats is read once per drawn frame and
// walks the population once; this walks every pair, and only the experiment
// runner needs it.

// Spacing says how spread out the population is.
type Spacing struct {
	// AvgNeighbours is how many others the average agent has within its
	// perception radius.
	AvgNeighbours float64

	// AvgNearestDist is how far the average agent is from the nearest other
	// one. It falls both when the population clumps together and when it
	// merely grows, which is why it is not the headline number.
	AvgNearestDist float64

	// Clumping is AvgNeighbours divided by what it would be if the same
	// population were scattered evenly over the whole world, which takes the
	// population size out of it: a bigger crowd has more neighbours without
	// being any more grouped.
	//
	// It is a relative scale, not an absolute one. The circle around an agent
	// near an edge is cut off by it, and the divisor here does not account for
	// that, so an evenly scattered population reads below one rather than at
	// it. Compare the figure between runs of the same world size; do not read
	// anything into its distance from 1.
	Clumping float64
}

// Spacing measures the current layout of the population. It is O(n^2) in the
// population and is meant to be sampled every so often, not every tick.
func (w *World) Spacing() Spacing {
	n := len(w.agents)
	if n < 2 {
		return Spacing{}
	}

	r2 := w.cfg.PerceptionRadius * w.cfg.PerceptionRadius
	var neighbours, nearestSum float64
	for i := range w.agents {
		a := &w.agents[i]
		count := 0
		nearest := math.Inf(1)
		for j := range w.agents {
			if i == j {
				continue
			}
			d2 := dist2(a.X, a.Y, w.agents[j].X, w.agents[j].Y)
			if d2 <= r2 {
				count++
			}
			if d2 < nearest {
				nearest = d2
			}
		}
		neighbours += float64(count)
		nearestSum += math.Sqrt(nearest)
	}

	s := Spacing{
		AvgNeighbours:  neighbours / float64(n),
		AvgNearestDist: nearestSum / float64(n),
	}
	if area := w.cfg.Width * w.cfg.Height; area > 0 {
		if expected := float64(n-1) * math.Pi * r2 / area; expected > 0 {
			s.Clumping = s.AvgNeighbours / expected
		}
	}
	return s
}
