package engine

import "math"

// Fish: food that is only in the water (stage 42).
//
// Water has been three things so far and none of them a reason to be there.
// It is dear to cross (stage 20), it drowns bodies (stage 34), and the land
// beside it can be made rich (stage 36) - which is the only one of the three
// that ever moved where anybody stood. This puts something in the water
// itself, so that the danger and the reward are finally in the same cells
// rather than in neighbouring ones.
//
// Four things it deliberately is not.
//
//   - Not part of stage 36. That rule makes the land near water grow more;
//     mixing this into it would put a second meaning into one figure and make
//     the total-preserving renormalisation say two things at once (#69).
//   - Not extra food. A fish takes the place of a plant that would otherwise
//     have come up, so FoodSpawnRate still says how much the world grows. A
//     world that got richer would show up as a bigger population that had
//     nothing to do with where anybody was standing (the principle stage 15a
//     set and every stage since has kept).
//   - Not a species change. Only humans take it by default: enemies live on
//     meat, and giving them a second food would make every population figure
//     since stage 11 a different measurement. The arm that lets them is here
//     to be run, not to be assumed.
//   - Not a new mechanism for "where food is". The amount of it in a region
//     follows from how much of that region is water, because a fish is placed
//     in a water cell drawn uniformly - so region-by-region it is exactly
//     proportional to the water there, with nothing to tune.

// eatsFish is the diet rule for the third kind. Humans fish; enemies do not,
// unless the world says otherwise.
func (w *World) eatsFish(s Species) bool {
	return s == SpeciesHuman || w.cfg.FishForAll
}

// waterCells is every cell of the map that is water, as positions. Worked out
// once when the world is built: the ground does not move, and a world with no
// water has an empty list, which is what keeps a flat world from drawing a
// single random number for fish.
func waterCells(g *terrainGrid) []cell {
	if g == nil {
		return nil
	}
	var out []cell
	for i := range g.cells {
		if g.cells[i].Kind != GroundWater {
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

// cell is one square of the map, by its middle and its size.
type cell struct{ x, y, w, h float64 }

// spawnFish puts one in the water, in place of the plant this spawn would
// otherwise have been. It reports whether it did: a world with no water, or
// one that has not been given any fish, says no without touching the random
// source, so its runs are the runs it always had.
func (w *World) spawnFish() bool {
	if w.cfg.FishShare <= 0 || len(w.water) == 0 {
		return false
	}
	if w.rng.Float64() >= w.cfg.FishShare {
		return false
	}
	c := w.water[w.rng.Intn(len(w.water))]
	x := clamp(c.x+w.randRange(-c.w/2, c.w/2), 10, w.cfg.Width-10)
	y := clamp(c.y+w.randRange(-c.h/2, c.h/2), 10, w.cfg.Height-10)
	return w.addFish(x, y) != 0
}

// addFish puts one in the world. Fish and plants share one allowance, because
// a fish is a plant that did not come up: two allowances would let the world
// hold more food than FoodSpawnRate ever said it could.
func (w *World) addFish(x, y float64) int {
	if w.growingFood() >= w.cfg.MaxFoodItems {
		return 0
	}
	return w.putFood(Food{X: x, Y: y, Kind: FoodFish})
}

// FishUse is what the water is providing. Read only.
type FishUse struct {
	// Items is how many fish are in the world, Share the part of everything
	// eaten that was fish, and InWater the share of the world's food that is
	// standing in water - the figure that says whether the reward and the
	// danger are in the same place.
	Items   int
	InWater float64
}

// Fish reports what the water holds.
func (w *World) Fish() FishUse {
	var out FishUse
	n := 0.0
	for i := range w.foods {
		n++
		if w.foods[i].Kind == FoodFish {
			out.Items++
		}
		if w.terrainAt(w.foods[i].X, w.foods[i].Y).Kind == GroundWater {
			out.InWater++
		}
	}
	if n > 0 {
		out.InWater /= n
	}
	return out
}

// --- the two ways of taking it (stage 43) ------------------------------------

// fishReach is how close this body has to get to take a fish: the ordinary
// reach, and further for a body that knows how to fish from the bank.
//
// It is the whole of what that skill does. Reaching further is not more food -
// a fish is a fish - it is not having to stand in the water for it, and what
// that is worth is exactly what drowning costs. Paying it here as well would
// have two skills buying the same thing, and then neither could be measured.
func (w *World) fishReach(a *Agent, kind FoodKind) float64 {
	r := w.cfg.GrabRadius
	if kind != FoodFish || w.cfg.SkillFishReach <= 0 || w.cfg.SkillFishRelief <= 0 {
		return r
	}
	return r * (1 + w.cfg.SkillFishReach*w.cfg.SkillFishRelief*a.skillAt(&w.cfg, SkillFishLand))
}

// fishCatch is the chance this body lands a fish it has reached, from where it
// is standing.
//
// This is what being good at fishing means (rewritten 2026-09-09). The first
// version of this stage made a fish worth more calories to a skilled body,
// which is the shape stage 38b's foraging rule has - and it inherited that
// rule's consequence: a body that gets more out of each fish needs fewer of
// them, so being good at working the water made bodies spend less time in it.
// That is a sensible thing to say about a stomach and a silly thing to say
// about fishing. What a good fisher gets is more fish.
//
// The two halves of the trade are the two grounds. Standing in the river,
// where the drowning rule charges by the tick, most attempts land; reaching in
// from dry ground, where nothing charges anything, most do not. Each skill
// lifts its own side towards certainty, and neither touches the other's - so
// what a body has learned decides which of the two ways of fishing is worth
// its time, rather than how little of it it needs.
func (w *World) fishCatch(a *Agent, kind FoodKind) float64 {
	if kind != FoodFish {
		return 1
	}
	if w.terrainAt(a.X, a.Y).Kind == GroundWater {
		return w.catchWading(a)
	}
	return w.catchFromBank(a)
}

// catchWading and catchFromBank are the two sides of it.
func (w *World) catchWading(a *Agent) float64 {
	return catchWith(w.cfg.FishCatchWater, w.cfg.SkillFishRelief*a.skillAt(&w.cfg, SkillFishWater))
}

func (w *World) catchFromBank(a *Agent) float64 {
	return catchWith(w.cfg.FishCatchBank, w.cfg.SkillFishRelief*a.skillAt(&w.cfg, SkillFishLand))
}

// catchExpected is what a body reckons its chances are at a fish it has not
// reached yet: the better of its two ways of going about it.
//
// Which of them it will actually use is decided by where it stops, and that is
// decided by how far it can reach - so a body good at working from the bank
// stops on the bank and a body good in the water goes in. Reading the chance
// from the ground it happens to be standing on when it decides was tried
// first, and it says the wrong thing in the ordinary case: a body on dry land
// far from the river would rate every fish in it at the bank's odds, when what
// it is about to do is wade in.
func (w *World) catchExpected(a *Agent, f *Food) float64 {
	if f.Special {
		return w.harvestCatch(a, f) // stage 44: rooted, so there is no choice of side
	}
	if f.Kind != FoodFish {
		return 1
	}
	return math.Max(w.catchWading(a), w.catchFromBank(a))
}

// catchWith lifts a base chance towards one in proportion to mastery: no skill
// leaves the ground's own figure, and complete mastery lands everything.
func catchWith(base, mastery float64) float64 {
	return clamp(base+(1-base)*clamp(mastery, 0, 1), 0, 1)
}

// theFishGetsAway is what happens when an attempt fails: it darts off to
// somewhere else in the water rather than being destroyed.
//
// Nothing is lost from the world, which is the rule every food rule in here
// keeps - what the failure costs is the walk, and having to find it again.
func (w *World) theFishGetsAway(f *Food) {
	w.fishMissed++
	if len(w.water) == 0 {
		return
	}
	c := w.water[w.rng.Intn(len(w.water))]
	x := clamp(c.x+w.randRange(-c.w/2, c.w/2), 10, w.cfg.Width-10)
	y := clamp(c.y+w.randRange(-c.h/2, c.h/2), 10, w.cfg.Height-10)
	w.moveFood(f.ID, x, y)
}

// AnglerUse is where the two kinds of fisher actually stand. Read only.
//
// The average share of the population standing in water cannot answer the
// question this stage asks - one number over two kinds of body says nothing
// about whether they have parted. This splits it by which of the two skills a
// body is better at, which is the only sense in which this world has a bank
// fisher and a wader at all.
type AnglerUse struct {
	// InWater is the share of the wading-leaning bodies standing in water,
	// OnBank the same for the bank-leaning ones, and Split the difference: a
	// population that has divided into the two trades stands apart.
	InWater float64
	OnBank  float64
	Split   float64
	// Waders and Bankers are how many of each there are, because a split
	// worked out from three bodies is not a split.
	Waders  float64
	Bankers float64
}

// Anglers reports where the two kinds of fisher stand.
func (w *World) Anglers() AnglerUse {
	var out AnglerUse
	var inWater, onBank float64
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive || a.Species != SpeciesHuman {
			continue
		}
		wade, bank := a.skillAt(&w.cfg, SkillFishWater), a.skillAt(&w.cfg, SkillFishLand)
		if wade == bank {
			continue // no trade of its own
		}
		wet := 0.0
		if w.terrainAt(a.X, a.Y).Kind == GroundWater {
			wet = 1
		}
		if wade > bank {
			out.Waders++
			inWater += wet
		} else {
			out.Bankers++
			onBank += wet
		}
	}
	if out.Waders > 0 {
		out.InWater = inWater / out.Waders
	}
	if out.Bankers > 0 {
		out.OnBank = onBank / out.Bankers
	}
	out.Split = out.InWater - out.OnBank
	return out
}
