package engine

// Regions are the world's own coarse divisions (decision #38).
//
// There is one kind of region and everything that varies by place uses it: how
// sheltered the resting is here (stage 14), how well the plants grow here
// (stage 15), and whatever a region-by-region reset eventually needs. Making a
// second division for the second use is how a world ends up with three
// overlapping maps that mean nearly the same thing.
//
// A region is not a wall. Nothing stops an agent walking out of one, nothing
// tells it which one it is in, and no rule reads a region ID. What a region
// does is change a number at a place, and an agent finds that out by being
// there.
//
// It is also not the spatial index (grid.go) and not the sight grid
// (sight.go). The index is an optimisation that must not change results; the
// sight grid is how far an agent can see. This is the ground itself, and it is
// coarse on purpose: fine regions would average out under any amount of
// wandering, and there would be no such thing as a good place to be.

// region is one block of the world.
type region struct {
	// Shelter multiplies what resting here is reckoned to cost (stage 14).
	// Below one is somewhere with its back covered; above one is open ground
	// where anybody around is a problem. Exactly one is the ordinary world.
	Shelter float64

	// Food is how much of the world's plant growth happens here (stage 15a),
	// relative to an equal share. It changes where the plants come up and not
	// how many: the world grows exactly as much food as it did, and the only
	// difference is that some of the ground grows more of it than the rest.
	//
	// That distinction matters more than it looks. FoodSpawnRate is the most
	// selection-sensitive number in the world - the whole design keeps the
	// place deliberately short of food - so a change that quietly altered the
	// total would be measuring something else entirely.
	Food float64

	// Special is how much of what grows here is the awkward crop of stage 44,
	// relative to an equal share. It is drawn like Food and it moves nothing
	// about how much grows: what it changes is how much of the growing needs
	// knowing.
	Special float64

	// Ability multiplies what a body standing here can do (stage 57): its
	// attack, its defence, how well it reads the world, how good a move it
	// picks. One is the ordinary world.
	//
	// It is the one thing the ground gives that cannot be carried away. A
	// skill goes on working wherever its holder walks - which is why five of
	// them in a row failed to make anywhere worth staying in - and this stops
	// working the moment its holder leaves.
	//
	// What it does not move is what a body holds: see Agent.capacity.
	Ability float64
}

// regionsOf lays the world out in blocks and draws what each one is like.
//
// Nothing is drawn when the spread is zero: the whole world comes out ordinary
// and, because no number is taken from the random source, the run is identical
// to one from before regions existed. That is the arm this stage is measured
// against, and it has to be exact rather than merely similar.
func (w *World) buildRegions() {
	cfg := &w.cfg
	cols, rows := max(cfg.RegionCols, 1), max(cfg.RegionRows, 1)
	w.regions = make([]region, cols*rows)
	for i := range w.regions {
		w.regions[i] = region{Shelter: 1, Food: 1, Special: 1, Ability: 1}
	}
	// Each spread is skipped entirely when it is zero, so a world with one of
	// them turned off consumes the random source exactly as a world without
	// the rule at all did. Approximately the same is not an arm.
	if cfg.ShelterSpread > 0 {
		for i := range w.regions {
			w.regions[i].Shelter = clamp(w.randRange(1-cfg.ShelterSpread, 1+cfg.ShelterSpread), 0, 2)
		}
	}
	if cfg.FoodSpread > 0 {
		for i := range w.regions {
			w.regions[i].Food = clamp(w.randRange(1-cfg.FoodSpread, 1+cfg.FoodSpread), 0, 2)
		}
	}
	if cfg.RegionAbilitySpread > 0 {
		for i := range w.regions {
			w.regions[i].Ability = clamp(w.randRange(1-cfg.RegionAbilitySpread, 1+cfg.RegionAbilitySpread), 0, 2)
		}
	}
	// Where the awkward crop grows (stage 44). Drawn like the rest, and
	// skipped entirely unless the world actually grows any - a spread with
	// no crop behind it would take numbers from the random source to decide
	// something nothing ever reads, and every figure recorded before this
	// stage would move for nothing.
	if cfg.SpecialtyShare > 0 && cfg.SpecialtySpread > 0 {
		for i := range w.regions {
			w.regions[i].Special = clamp(w.randRange(1-cfg.SpecialtySpread, 1+cfg.SpecialtySpread), 0, 2)
		}
	}

	w.tieFoodToTheGround()
	w.tieFoodToTheWater()

	w.foodWeight = 0
	for i := range w.regions {
		w.foodWeight += w.regions[i].Food
	}
}

// tieFoodToTheGround makes where the plants come up depend on how hard the
// ground is to cross (stage 33).
//
// Why the world does this rather than the agents. Stage 29 gave agents a
// belief about the going and a belief about the food, and they still walked
// into rough country: avoiding it always meant giving up the food that grows
// there, because the two were laid out independently. Nothing in an agent
// needs to change to fix that - correlate the world's own two figures and the
// comparison they already run finds the relationship on its own. There is no
// third belief to learn and nothing new to hand on.
//
// Two things it deliberately does not do.
//
//   - It does not change how much food the world grows. Every region is
//     scaled back to the total the draw above produced, because FoodSpawnRate
//     is the most selection-sensitive number in the world and a rule that
//     quietly moved the total would be measuring something else (stage 15a's
//     rule, and the test that pins it).
//   - It does not read the map more finely than a region. What an agent can
//     hold about the country is one figure per region (#53), so a relationship
//     drawn at any finer grain would be one no agent could ever act on.
//
// A world with no map comes out untouched: every region costs the same, so
// every deviation is zero whatever the correlation is set to.
func (w *World) tieFoodToTheGround() {
	if w.cfg.TerrainFoodCorrelation == 0 || w.ground == nil || len(w.regions) == 0 {
		return
	}
	cost := make([]float64, len(w.regions))
	mean := 0.0
	for i := range w.regions {
		cost[i] = w.regionMeanCost(i)
		mean += cost[i]
	}
	mean /= float64(len(w.regions))
	if mean <= 0 {
		return
	}

	before := 0.0
	for i := range w.regions {
		before += w.regions[i].Food
	}

	// Positive: ground that is dearer than the world's average grows less.
	// The deviation is relative, so the rule says the same thing whatever
	// units the map's costs happen to be in.
	after := 0.0
	for i := range w.regions {
		dev := (cost[i] - mean) / mean
		w.regions[i].Food = clamp(w.regions[i].Food*(1-w.cfg.TerrainFoodCorrelation*dev), 0, 2)
		after += w.regions[i].Food
	}
	if after <= 0 || before <= 0 {
		return
	}
	for i := range w.regions {
		w.regions[i].Food = clamp(w.regions[i].Food*before/after, 0, 2)
	}
}

// tieFoodToTheWater makes the ground beside water grow more of it (stage 36).
//
// This is the one piece of country that is not simply worse for being dear.
// Water costs three times as much to cross and drowns whoever lingers in it
// (stages 20 and 34), and now it also feeds them: the first place in this
// world where the danger and the reward are the same place.
//
// It is a term of its own rather than part of the correlation with cost
// (#62), because the two say opposite things about the same ground. Cost says
// hard country is poor country; this says the river bank is rich. Folded into
// one multiplier they would cancel, and a map's author could not ask for both.
//
// What it reads is how much of the region is water, which is the finest grain
// a region-sized belief could ever act on (#53) - a band along the bank is not
// something an agent can hold or be told. Negative is the other landscape,
// where the water is a barren strip; it is the same rule with the sign turned
// over and is measured as its own condition.
//
// Like stage 33 it changes where the food comes up and not how much of it
// there is: the regions are scaled back to the total the draw produced.
func (w *World) tieFoodToTheWater() {
	if w.cfg.WatersideFood == 0 || w.ground == nil || len(w.regions) == 0 {
		return
	}
	before := 0.0
	for i := range w.regions {
		before += w.regions[i].Food
	}
	if before <= 0 {
		return
	}

	after := 0.0
	for i := range w.regions {
		share := w.regionWaterShare(i)
		w.regions[i].Food = clamp(w.regions[i].Food*(1+w.cfg.WatersideFood*share), 0, 2)
		after += w.regions[i].Food
	}
	if after <= 0 {
		return
	}
	for i := range w.regions {
		w.regions[i].Food = clamp(w.regions[i].Food*before/after, 0, 2)
	}
}

// pickFoodRegion draws a region for a plant to come up in, in proportion to how
// well the ground there grows things. It is only reached when the regions
// actually differ; a flat world keeps the old uniform draw, down to the number
// of values taken from the random source.
func (w *World) pickFoodRegion() int {
	if w.foodWeight <= 0 {
		return 0
	}
	r := w.rng.Float64() * w.foodWeight
	for i := range w.regions {
		r -= w.regions[i].Food
		if r <= 0 {
			return i
		}
	}
	return len(w.regions) - 1
}

// regionBounds is the ground one block covers.
func (w *World) regionBounds(i int) (minX, minY, maxX, maxY float64) {
	cols, rows := max(w.cfg.RegionCols, 1), max(w.cfg.RegionRows, 1)
	cw, ch := w.cfg.Width/float64(cols), w.cfg.Height/float64(rows)
	c, r := i%cols, i/cols
	return float64(c) * cw, float64(r) * ch, float64(c+1) * cw, float64(r+1) * ch
}

// regionIndexAt is which block a position falls in, as an index.
func (w *World) regionIndexAt(x, y float64) int {
	cols, rows := max(w.cfg.RegionCols, 1), max(w.cfg.RegionRows, 1)
	c := clampInt(int(x/w.cfg.Width*float64(cols)), 0, cols-1)
	r := clampInt(int(y/w.cfg.Height*float64(rows)), 0, rows-1)
	return r*cols + c
}

// regionAt is which block a position falls in. Positions are kept inside the
// world by the boundary rule, so the clamp is a belt on top of braces.
func (w *World) regionAt(x, y float64) *region {
	if len(w.regions) == 0 {
		return nil
	}
	return &w.regions[w.regionIndexAt(x, y)]
}

// shelterAt is how exposed resting at this spot is reckoned to be, as a
// multiplier on RestExposureWeight. One everywhere is the world as it was.
//
// It is the only thing a region does today, and it does it in one place: the
// rest option's exposure term. There is no second formula and no new state -
// the whole stage is a multiplication (decision #36).
func (w *World) shelterAt(x, y float64) float64 {
	if r := w.regionAt(x, y); r != nil {
		return r.Shelter
	}
	return 1
}

// --- reading it out ---------------------------------------------------------

// RegionView is one block of the world, for the viewer. Read only.
type RegionView struct {
	MinX, MinY, MaxX, MaxY float64
	Shelter                float64
	Food                   float64

	// Ability is what standing here does to what a body can do (stage 57).
	Ability float64
}

// Regions reports the blocks the world is divided into. Read only.
func (w *World) Regions() []RegionView {
	cols, rows := max(w.cfg.RegionCols, 1), max(w.cfg.RegionRows, 1)
	out := make([]RegionView, 0, len(w.regions))
	cw, ch := w.cfg.Width/float64(cols), w.cfg.Height/float64(rows)
	for i := range w.regions {
		c, r := i%cols, i/cols
		out = append(out, RegionView{
			MinX: float64(c) * cw, MinY: float64(r) * ch,
			MaxX: float64(c+1) * cw, MaxY: float64(r+1) * ch,
			Shelter: w.regions[i].Shelter,
			Food:    w.regions[i].Food,
			Ability: w.regions[i].Ability,
		})
	}
	return out
}

// richnessAt is how well this ground grows plants, relative to an equal share.
func (w *World) richnessAt(x, y float64) float64 {
	if r := w.regionAt(x, y); r != nil {
		return r.Food
	}
	return 1
}

// abilityAt is what the ground at this spot does to what a body can do
// (stage 57). One everywhere in a world without the rule, which is why an
// agent that has never been told otherwise carries a factor of one.
func (w *World) abilityAt(x, y float64) float64 {
	if r := w.regionAt(x, y); r != nil && r.Ability > 0 {
		return r.Ability
	}
	return 1
}

// standOnGround tells every living body what the ground under it is worth to
// it today (stage 57), once a tick and before anybody decides anything.
//
// It is held on the agent rather than looked up inside Agent.Ability because
// Ability is asked hundreds of times a tick and knows nothing about the world
// it is in. A tick's worth of staleness is the price, and it is the same price
// the rest of the tick pays: everything an agent decides this tick, it decides
// about where it was when the tick began.
//
// A world with no such difference in it never enters the loop, so it is not
// only unchanged but untouched.
func (w *World) standOnGround() {
	if w.cfg.RegionAbilitySpread <= 0 || w.cfg.RegionAbilityCarried {
		return
	}
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		a.regionBias = w.abilityAt(a.X, a.Y)
	}
}

// Standing is where the population is, measured by what the ground there does
// to it (stage 57).
//
// Gain is the figure the stage turns on, and it is the same shape as stage
// 44's: how much better the ground under the humans is than the ground taken
// at random. A population that has settled where it is at its best reads above
// zero; one that walks about regardless reads zero however large the spread
// is. Five carried skills all read zero on their own version of this.
type Standing struct {
	Humans float64 // mean ground ability where the humans are
	All    float64 // ... and averaged over the whole world
	Gain   float64 // Humans - All
}

// Standing reports where the population stands. Read only.
func (w *World) Standing() Standing {
	out := Standing{Humans: 1, All: 1}
	if len(w.regions) == 0 {
		return out
	}
	sum := 0.0
	for i := range w.regions {
		sum += w.regions[i].Ability
	}
	out.All = sum / float64(len(w.regions))

	var humans, ground float64
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive || a.Species != SpeciesHuman {
			continue
		}
		humans++
		ground += w.abilityAt(a.X, a.Y)
	}
	// Nobody alive is no evidence about where the living stand, rather than
	// evidence that they stand on the worst ground there is (stage 14's
	// mistake, which is not being made twice).
	if humans > 0 {
		out.Humans = ground / humans
	} else {
		out.Humans = out.All
	}
	out.Gain = out.Humans - out.All
	return out
}

// FootingOf is what the ground is doing to this body right now (stage 57), for
// the viewer. One is ordinary ground and the whole of a world without the
// rule; an agent that is not there at all reads one as well.
func (w *World) FootingOf(id int) float64 {
	if a := w.agentByID(id); a != nil {
		return a.groundFactor()
	}
	return 1
}

// GroundAt is what the ground at this spot is like, for the viewer: how
// exposed resting on it is, and how well it grows plants. Read only. An agent
// gets the first of those through Perception and finds the second out by
// looking for something to eat.
func (w *World) GroundAt(x, y float64) (shelter, food float64) {
	return w.shelterAt(x, y), w.richnessAt(x, y)
}

// ShelterUse is where the population actually rests, against where it is.
//
// The two are the point of the stage. A world where resting is cheaper in some
// places should end up with the resting happening in those places, and the
// only way to tell that from "agents are wherever they are" is to compare the
// shelter under the ones lying down with the shelter under everybody.
type ShelterUse struct {
	Resting float64 // mean shelter where the resting agents are
	All     float64 // ... and where the population as a whole is
}

// Richness is where each kind of creature is standing, measured by how well the
// ground there grows plants.
//
// It is the check on decision #39: if the humans end up on the good ground that
// is food pulling them, and it says nothing about whether they are grouping.
// The
// predators are counted separately because they are drawn to the humans rather
// than to the plants, so their figure should follow the humans' with nothing
// of its own in it.
type Richness struct {
	Humans  float64 // mean plant growth where the humans are
	Enemies float64 // ... and where the predators are
	All     float64 // ... and averaged over the whole world
}

// Richness reports where the two kinds of creature stand. Read only.
func (w *World) Richness() Richness {
	out := Richness{All: 1}
	if len(w.regions) > 0 {
		out.All = w.foodWeight / float64(len(w.regions))
	}
	var humans, enemies float64
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		r := w.richnessAt(a.X, a.Y)
		if a.Species == SpeciesEnemy {
			out.Enemies += r
			enemies++
		} else {
			out.Humans += r
			humans++
		}
	}
	// Nobody of a kind is no evidence about where that kind stands, not
	// evidence that it stands on the worst ground there is (stage 14's
	// mistake, which is not being made twice).
	if humans > 0 {
		out.Humans /= humans
	} else {
		out.Humans = out.All
	}
	if enemies > 0 {
		out.Enemies /= enemies
	} else {
		out.Enemies = out.All
	}
	return out
}

// Shelter reports where the resting is happening. Read only.
func (w *World) Shelter() ShelterUse {
	var out ShelterUse
	var resting, all float64
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		s := w.shelterAt(a.X, a.Y)
		out.All += s
		all++
		if a.Action.Kind == ActRest {
			out.Resting += s
			resting++
		}
	}
	if all > 0 {
		out.All /= all
	}
	if resting > 0 {
		out.Resting /= resting
	} else {
		// Nobody is lying down at this instant, which is no evidence either
		// way rather than evidence of nought. Reporting a zero here is what a
		// first version did, and averaging those zeros over a run produced a
		// confident gain of 0.14 in a world where every region was identical.
		out.Resting = out.All
	}
	return out
}

// regionMeanCost is what crossing this block really costs on average, sampled
// over the terrain under it (stage 29). One in a world with no map.
//
// For the measurement rather than for the agents: an agent learns the going by
// walking it, and this is what its belief is scored against.
// regionMean is what one reading of the ground comes to over a whole region,
// sampled on a fixed lattice. Three things ask for it - what crossing costs
// (stage 29), what a tick there risks (stage 35), and how much of it is water
// (stage 36) - and they differ only in what they read off a cell, so they
// share the sampling rather than each carrying a copy of it.
//
// The lattice is deliberately fixed and coarse: this is the world's own truth
// about a region, used to lay the food out and to score what agents believe,
// and nothing in the simulation loop calls it.
func (w *World) regionMean(i int, of func(terrain) float64) float64 {
	minX, minY, maxX, maxY := w.regionBounds(i)
	const steps = 8
	sum, n := 0.0, 0.0
	for sx := 0; sx < steps; sx++ {
		for sy := 0; sy < steps; sy++ {
			x := minX + (maxX-minX)*(float64(sx)+0.5)/steps
			y := minY + (maxY-minY)*(float64(sy)+0.5)/steps
			sum += of(w.terrainAt(x, y))
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / n
}

// regionMeanDrown is what a tick spent anywhere in this region really risks
// (stage 35). It is the truth the population's belief is scored against; no
// rule reads it.
func (w *World) regionMeanDrown(i int) float64 {
	if w.ground == nil {
		return 0
	}
	return w.regionMean(i, func(t terrain) float64 { return t.Drown })
}

// regionWaterShare is how much of a region is water (stage 36).
func (w *World) regionWaterShare(i int) float64 {
	if w.ground == nil {
		return 0
	}
	return w.regionMean(i, func(t terrain) float64 {
		if t.Kind == GroundWater {
			return 1
		}
		return 0
	})
}

// regionBankShare is how much of a region is dry ground with water beside it
// (stage 43). It is the reading behind fishing from the bank, and it is
// deliberately not the water share: a region that is all river teaches nobody
// to fish from dry land, and neither does one with no water in it at all.
//
// "Beside" is one sample step in each of the four directions, which is the
// same grain everything else about a region is read at.
func (w *World) regionBankShare(i int) float64 {
	if w.ground == nil {
		return 0
	}
	minX, minY, maxX, maxY := w.regionBounds(i)
	const steps = 8
	dx, dy := (maxX-minX)/steps, (maxY-minY)/steps
	sum, n := 0.0, 0.0
	for sx := 0; sx < steps; sx++ {
		for sy := 0; sy < steps; sy++ {
			x := minX + dx*(float64(sx)+0.5)
			y := minY + dy*(float64(sy)+0.5)
			n++
			if w.terrainAt(x, y).Kind == GroundWater {
				continue // standing in it is the other skill's business
			}
			if w.terrainAt(x+dx, y).Kind == GroundWater ||
				w.terrainAt(x-dx, y).Kind == GroundWater ||
				w.terrainAt(x, y+dy).Kind == GroundWater ||
				w.terrainAt(x, y-dy).Kind == GroundWater {
				sum++
			}
		}
	}
	if n == 0 {
		return 0
	}
	return sum / n
}

func (w *World) regionMeanCost(i int) float64 {
	if w.ground == nil {
		return 1
	}
	return w.regionMean(i, func(t terrain) float64 { return t.Cost })
}
