package engine

import "math"

// Stones (stage 45).
//
// The first thing in this world worth picking up that is not worth eating.
// What they are for arrives in stage 46 - something to throw - and they are
// built first for the reason this project builds anything first: before a rule
// about using them can mean anything, it has to be known how many there are
// and how far a body is from one. A ranged attack nobody has a stone for is a
// rule that never fires, and that is worth finding out now rather than after
// it is written.
//
// Three things they deliberately are not.
//
//   - Not a new kind of ownership. What is lying about can be picked up and
//     what is held is held, which is all the rules carrying already has. There
//     is nothing to steal here and nothing to claim (#72).
//   - Not free to carry. A stone takes a hand exactly as a plant does, and
//     with hands as small as this world's that is the whole of the trade: a
//     body carrying a stone is a body not carrying its dinner.
//   - Not spread evenly. They lie on broken ground, the way fish are in water
//     and the awkward crop is where it grows - so a map author decides where
//     the ammunition is, and a world with no rough country has none.

// stoneCells is every cell of the map a stone could be lying in. Worked out
// once when the world is built, like the water: the ground does not move.
func stoneCells(g *terrainGrid) []cell {
	if g == nil {
		return nil
	}
	var out []cell
	for i := range g.cells {
		if g.cells[i].Kind != GroundRough {
			continue
		}
		x, y := i%g.cols, i/g.cols
		out = append(out, cell{
			x: (float64(x) + 0.5) * g.cellW,
			y: (float64(y) + 0.5) * g.cellH,
			w: g.cellW, h: g.cellH,
		})
	}
	return out
}

// scatterStones lays the world's stones out over the broken ground, once, when
// the world is built. A world with no rough country in it, or one whose author
// asked for none, draws nothing at all - so its runs are the runs it had.
func (w *World) scatterStones() {
	if w.cfg.Stones <= 0 || len(w.rubble) == 0 {
		return
	}
	for i := 0; i < w.cfg.Stones; i++ {
		c := w.rubble[w.rng.Intn(len(w.rubble))]
		w.putFood(Food{
			X:    clamp(c.x+w.randRange(-c.w/2, c.w/2), 10, w.cfg.Width-10),
			Y:    clamp(c.y+w.randRange(-c.h/2, c.h/2), 10, w.cfg.Height-10),
			Kind: FoodStone,
		})
	}
}

// StoneUse is what there is to throw and how near it is. Read only.
type StoneUse struct {
	// Lying is how many are on the ground and Held how many are in hands.
	Lying int
	Held  int

	// InSight is the share of the population with one in sight, and Carrying
	// the share holding one. Between them they say whether a rule about
	// throwing stones could ever fire: a body that never sees one and never
	// holds one will never throw one, whatever the rule is worth.
	InSight  float64
	Carrying float64

	// Nearest is the mean distance from a body to the closest stone, counting
	// only the bodies that have one within sight - a mean over the whole
	// population would be a mean over a world, not over a reach.
	Nearest float64
}

// Stones reports the supply. It is O(bodies x stones), so it is asked at the
// sampling interval rather than every tick.
func (w *World) Stones() StoneUse {
	var out StoneUse
	var stones []int
	for i := range w.foods {
		if w.foods[i].Kind == FoodStone {
			out.Lying++
			stones = append(stones, i)
		}
	}
	people, seeing, holding, near := 0.0, 0.0, 0.0, 0.0
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive || a.Species != SpeciesHuman {
			continue
		}
		people++
		for j := range a.carried {
			if a.carried[j].Kind == FoodStone {
				out.Held++
				holding++
				break
			}
		}
		best := -1.0
		for _, s := range stones {
			f := &w.foods[s]
			if !w.canSee(a.X, a.Y, f.X, f.Y) {
				continue
			}
			if d := math.Sqrt(dist2(a.X, a.Y, f.X, f.Y)); best < 0 || d < best {
				best = d
			}
		}
		if best >= 0 {
			seeing++
			near += best
		}
	}
	if people > 0 {
		out.InSight = seeing / people
		out.Carrying = holding / people
	}
	if seeing > 0 {
		out.Nearest = near / seeing
	}
	return out
}

// regionStoneShare is how well supplied with stones one region is, against an
// ordinary share of them: one is twice the average or better, zero is none.
//
// It is the reading behind knowing how to throw (stage 47) - a body born where
// the ammunition is grows up throwing - and it is deliberately taken from the
// stones and not from the ground under them, because the ground already seeds
// knowing how to cross it (stage 38a: not two skills from one reading).
func (w *World) regionStoneShare(i int) float64 {
	if w.cfg.Stones <= 0 || len(w.regions) == 0 {
		return 0
	}
	n := 0
	for j := range w.foods {
		if w.foods[j].Kind != FoodStone {
			continue
		}
		if w.regionIndexAt(w.foods[j].X, w.foods[j].Y) == i {
			n++
		}
	}
	mean := float64(w.cfg.Stones) / float64(len(w.regions))
	if mean <= 0 {
		return 0
	}
	return clamp(float64(n)/(2*mean), 0, 1)
}
