package engine

import "math"

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

	// Enemies is how many of the world's arriving enemies turn up here (stage
	// 58), relative to an equal share. Like Food it moves where they come
	// from and not how many there are.
	//
	// A region with a lot of them is the map's dangerous country. Nothing
	// tells an agent which region that is - what it can do is meet them.
	Enemies float64

	// Favour is the same thing per gene (stage 57c): what this ground does to
	// each of the nine, with the nine scaled so that their mean is one.
	//
	// It exists because the scalar above cancels. It is common to everybody
	// standing here, and everybody a body ever compares itself with is
	// standing here too, so it drops out of every fight and every race. This
	// does not drop out: no region is better than another, but a region suits
	// some builds and not others, and two neighbours built differently get
	// different factors.
	//
	// Nil in a world without the rule, and read through favourOf so that nil
	// reads as ones.
	Favour []float64
}

// favourOf is what this region does to one gene. One when the world has no
// such rule in it.
func (r *region) favourOf(g Gene) float64 {
	if int(g) >= len(r.Favour) || r.Favour[g] <= 0 {
		return 1
	}
	return r.Favour[g]
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
		w.regions[i] = region{Shelter: 1, Food: 1, Special: 1, Ability: 1, Enemies: 1}
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
	// Where the enemies turn up (stage 58), drawn like the plants and skipped
	// at zero for the same reason.
	if cfg.EnemySpread > 0 {
		for i := range w.regions {
			w.regions[i].Enemies = clamp(w.randRange(1-cfg.EnemySpread, 1+cfg.EnemySpread), 0, 2)
		}
	}
	// And what it favours, gene by gene (stage 57c). The nine are scaled to a
	// mean of one afterwards, which is what keeps this about which build the
	// ground suits rather than about how good the ground is: how good it is
	// already has a rule, and that one is the scalar above.
	if cfg.RegionFavourSpread > 0 {
		for i := range w.regions {
			f := make([]float64, NumGenes)
			sum := 0.0
			for g := range f {
				f[g] = clamp(w.randRange(1-cfg.RegionFavourSpread, 1+cfg.RegionFavourSpread), 0, 2)
				sum += f[g]
			}
			if sum <= 0 {
				continue
			}
			scale := float64(NumGenes) / sum
			for g := range f {
				f[g] *= scale
			}
			w.regions[i].Favour = f
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

	w.tolls = make([]regionToll, len(w.regions))
	w.foodWeight = 0
	w.enemyWeight = 0
	for i := range w.regions {
		w.foodWeight += w.regions[i].Food
		w.enemyWeight += w.regions[i].Enemies
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

// regionToll is what has died in one block of the world and how much of it
// was violent (stage 58). A measurement: no rule reads it.
type regionToll struct {
	deaths float64
	kills  float64
}

// tollAt is the tally for the block this spot is in, or nil in a world with no
// blocks in it.
func (w *World) tollAt(x, y float64) *regionToll {
	if len(w.tolls) == 0 {
		return nil
	}
	return &w.tolls[w.regionIndexAt(x, y)]
}

// correlation is Pearson's, and zero when there is nothing to correlate.
func correlation(xs, ys []float64) float64 {
	n := float64(len(xs))
	if n < 3 {
		return 0
	}
	var sx, sy, sxx, syy, sxy float64
	for i := range xs {
		sx += xs[i]
		sy += ys[i]
		sxx += xs[i] * xs[i]
		syy += ys[i] * ys[i]
		sxy += xs[i] * ys[i]
	}
	den := math.Sqrt((n*sxx - sx*sx) * (n*syy - sy*sy))
	if den <= 0 {
		return 0
	}
	return (n*sxy - sx*sy) / den
}

// pickEnemyRegion draws a region for an arriving enemy, in proportion to how
// many of them turn up there. Like pickFoodRegion it is only reached when the
// regions actually differ, so a world without the rule keeps the old uniform
// draw down to the number of values taken from the random source.
func (w *World) pickEnemyRegion() int { return w.pickEnemyRegionFor(1) }

// pickEnemyRegionFor is the same draw blended towards an even one (stage 59):
// a weight of 1 + homing*(w-1), which is the map's own weighting at one and a
// flat map at zero.
func (w *World) pickEnemyRegionFor(homing float64) int {
	if w.enemyWeight <= 0 {
		return 0
	}
	weight := func(i int) float64 {
		return max(1+homing*(w.regions[i].Enemies-1), 0)
	}
	total := 0.0
	for i := range w.regions {
		total += weight(i)
	}
	if total <= 0 {
		return 0
	}
	r := w.rng.Float64() * total
	for i := range w.regions {
		r -= weight(i)
		if r <= 0 {
			return i
		}
	}
	return len(w.regions) - 1
}

// spawnSpot is where something the world puts in from outside arrives.
//
// Humans never come through here - they are born - so the weighting is the
// enemies' own. With no spread it is the uniform draw the world always made,
// in the same order and the same number of draws.
func (w *World) spawnSpot(species Species) (float64, float64) {
	return w.spawnSpotFor(species, 0)
}

// spawnSpotFor is the same for one sort of enemy (stage 59). How closely it
// follows the map's weighting is the row's own Homing: one is the full
// weighting of stage 58, zero is anywhere at all.
//
// A kind that follows nothing still draws its region - it just draws it
// evenly - because a table where one row takes a value from the random source
// and another does not would make the arms depend on the order the sorts
// happened to arrive in.
func (w *World) spawnSpotFor(species Species, kind int) (float64, float64) {
	if species != SpeciesEnemy || len(w.regions) == 0 {
		x := w.randRange(20, w.cfg.Width-20)
		y := w.randRange(20, w.cfg.Height-20)
		return x, y
	}
	homing := 1.0
	if kinds := w.enemyKinds(); kind >= 0 && kind < len(kinds) {
		homing = kinds[kind].Homing
	}
	// Nothing to follow: no weighting in the map, or a kind that ignores it.
	// The old uniform draw, down to the number of values taken.
	if w.cfg.EnemySpread <= 0 || homing <= 0 {
		x := w.randRange(20, w.cfg.Width-20)
		y := w.randRange(20, w.cfg.Height-20)
		return x, y
	}
	minX, minY, maxX, maxY := w.regionBounds(w.pickEnemyRegionFor(homing))
	x := clamp(w.randRange(minX, maxX), 20, w.cfg.Width-20)
	y := clamp(w.randRange(minY, maxY), 20, w.cfg.Height-20)
	return x, y
}

// prowlAt is how many of the world's enemies turn up where this is.
func (w *World) prowlAt(x, y float64) float64 {
	if r := w.regionAt(x, y); r != nil && r.Enemies > 0 {
		return r.Enemies
	}
	return 1
}

// Prowl is where the dangerous country is and what being in it comes to
// (stage 58).
//
// The stage's own figures are the first two: the enemies should end up where
// the map says they arrive, and the question worth asking is whether the
// humans end up anywhere else. Nothing tells them where the enemies come
// from, so HumanGain moving at all would be them feeling it rather than
// knowing it.
//
// Bite is what it costs to be there: across regions, how much the share of
// deaths that were kills goes with how many enemies arrive. It is cumulative
// - a region's tally over the whole run - because deaths are rare enough that
// an instant says nothing.
type Prowl struct {
	EnemyGain float64 // mean arrival weight where the enemies are, less the map's mean
	HumanGain float64 // ... and where the humans are
	Crowding  float64 // how unevenly the enemies are spread: 1 is even, higher is piled up
	Bite      float64 // correlation across regions between arrivals and the kill share of deaths

	// ArriveGain is the weighting as it goes in: the mean weight where the
	// enemies were put, less the map's mean. It is what the rule did, and
	// EnemyGain against it is how much of that the world still has - a
	// figure that has to be read as a pair, because a rule that works
	// perfectly at the moment it fires can still leave nothing behind.
	ArriveGain float64

	// BornShare is how many of the enemies the world made itself rather than
	// put in, which is the part of them the weighting never touched.
	BornShare float64
}

// Roaming is how far the enemies have got from where they came into the world
// (stage 64), and how often the walk back is what they chose.
//
// It is the figure the stage turns on. Stage 58 put the arrivals where the map
// asked and the world lost them within a lifetime - mean distance from where
// they started was more than a region wide - so what has to move here is this,
// and Prowl.EnemyGain is what it would buy.
type Roaming struct {
	Away   float64 // mean distance from home, in region widths
	AtHome float64 // share of the enemies standing in their own block
	Draws  float64 // share of decisions that were "head back"
}

// Roaming reports how far the enemies have strayed. Read only.
func (w *World) Roaming() Roaming {
	var out Roaming
	span := max(w.cfg.Width/float64(max(w.cfg.RegionCols, 1)), 1)
	var enemies, away, home float64
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive || a.Species != SpeciesEnemy {
			continue
		}
		enemies++
		if a.HomeRegion < 0 || a.HomeRegion >= len(w.regions) {
			continue
		}
		minX, minY, maxX, maxY := w.regionBounds(a.HomeRegion)
		hx, hy := (minX+maxX)/2, (minY+maxY)/2
		away += math.Hypot(a.X-hx, a.Y-hy) / span
		if w.regionIndexAt(a.X, a.Y) == a.HomeRegion {
			home++
		}
	}
	if enemies == 0 {
		return out
	}
	out.Away, out.AtHome = away/enemies, home/enemies
	if w.decisions > 0 {
		out.Draws = float64(w.homeDraws) / float64(w.decisions)
	}
	return out
}

// Prowl reports where the enemies are and what it costs to be near them. Read
// only.
func (w *World) Prowl() Prowl {
	out := Prowl{Crowding: 1}
	n := len(w.regions)
	if n == 0 {
		return out
	}
	mean := w.enemyWeight / float64(n)

	counts := make([]float64, n)
	var humans, enemies, humanWeight, enemyWeight float64
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		r := w.regionIndexAt(a.X, a.Y)
		if a.Species == SpeciesEnemy {
			enemies++
			enemyWeight += w.regions[r].Enemies
			counts[r]++
		} else {
			humans++
			humanWeight += w.regions[r].Enemies
		}
	}
	// Nobody of a kind is no evidence about where that kind stands (stage
	// 14's mistake, not made twice).
	if enemies > 0 {
		out.EnemyGain = enemyWeight/enemies - mean
		share := 0.0
		for _, c := range counts {
			p := c / enemies
			share += p * p
		}
		out.Crowding = share * float64(n)
	}
	if humans > 0 {
		out.HumanGain = humanWeight/humans - mean
	}
	out.Bite = w.killShareGoesWithArrivals()
	if w.enemyArrivals > 0 {
		out.ArriveGain = w.enemyArrivalSum/w.enemyArrivals - mean
	}
	if made := w.enemyArrivals + w.enemyBorn; made > 0 {
		out.BornShare = w.enemyBorn / made
	}
	return out
}

// killShareGoesWithArrivals correlates, across regions, how many enemies turn
// up there with how much of the dying there was violent.
func (w *World) killShareGoesWithArrivals() float64 {
	var xs, ys []float64
	for i := range w.regions {
		t := w.tolls[i]
		if t.deaths < 8 {
			// Too few to say anything about, and a region nobody died in
			// would otherwise read as a perfectly peaceful one.
			continue
		}
		xs = append(xs, w.regions[i].Enemies)
		ys = append(ys, t.kills/t.deaths)
	}
	return correlation(xs, ys)
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

	// Enemies is how many of the world's arriving enemies turn up here
	// (stage 58), relative to an equal share.
	Enemies float64
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
			Enemies: w.regions[i].Enemies,
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
	if !w.groundHasAnOpinion() || w.cfg.RegionAbilityCarried {
		return
	}
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		w.footOn(a)
	}
}

// groundHasAnOpinion says whether any of this stage's rules are switched on.
func (w *World) groundHasAnOpinion() bool {
	return w.cfg.RegionAbilitySpread > 0 || w.cfg.RegionFavourSpread > 0
}

// footOn writes what the ground under this body is worth to each of its genes.
func (w *World) footOn(a *Agent) {
	r := w.regionAt(a.X, a.Y)
	if r == nil {
		return
	}
	scalar := 1.0
	if r.Ability > 0 {
		scalar = r.Ability
	}
	for g := 0; g < NumGenes; g++ {
		a.footing[g] = scalar * r.favourOf(Gene(g))
	}
}

// carriedFooting is a body's own footing when that is a thing it owns rather
// than a copy of the ground beneath it, and nil otherwise (stage 57). Only the
// carried arm has any, which is the only arm where saving a world and loading
// it again would otherwise lose something.
func carriedFooting(a *Agent) []float64 {
	out := make([]float64, 0, NumGenes)
	any := false
	for _, f := range a.footing {
		out = append(out, f)
		if f > 0 {
			any = true
		}
	}
	if !any {
		return nil
	}
	return out
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
// the viewer: the factors on its nine genes, weighted by how much of itself
// each of them is. One is ordinary ground and the whole of a world without the
// rule; an agent that is not there at all reads one as well.
//
// Weighting by the body's own build is what makes one number honest under
// 57c, where the ground favours some genes and not others: ground that is
// kind to a gene this body barely has is not kind to this body.
func (w *World) FootingOf(id int) float64 {
	a := w.agentByID(id)
	if a == nil {
		return 1
	}
	return weighted(a, func(g Gene) float64 { return a.groundFactor(g) })
}

// weighted averages something per gene over a body's own build: how much of
// its budget each gene is.
func weighted(a *Agent, of func(Gene) float64) float64 {
	budget := a.Budget()
	if budget <= 0 {
		return 1
	}
	out := 0.0
	for g := 0; g < NumGenes; g++ {
		out += a.Gene(Gene(g)) / budget * of(Gene(g))
	}
	return out
}

// Suits is whether bodies are standing where the ground suits them (stage
// 57c).
//
// It is the figure 57a could not have: with one factor for everybody, "good
// ground" means the same thing to every body and the measurement is just
// where the population is. Here it is per body - what this ground does to
// this build - so the question is whether the two have found each other.
//
// Gain is a body's own footing where it stands, less the same body's footing
// averaged over every region there is. A world that sorts reads above zero; a
// world where bodies walk about regardless reads zero however far apart the
// regions are.
type Suits struct {
	Here float64 // mean footing where the bodies actually are
	All  float64 // ... and the same bodies averaged over the whole map
	Gain float64 // Here - All, as a percentage of an ordinary body

	// Ceiling is how much there was to be had: how much better each body's
	// best region would be than the average one, on the same scale. Gain
	// against Ceiling is the share of the sorting that actually happened,
	// which is the reading that means anything - a gain of a tenth of a per
	// cent says nothing until it is set against what was on offer.
	Ceiling float64
}

// Suits reports whether the ground and the bodies on it have found each other.
// Read only.
func (w *World) Suits() Suits {
	out := Suits{Here: 1, All: 1}
	if len(w.regions) == 0 {
		return out
	}
	var bodies float64
	var here, all, best float64
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive || a.Species != SpeciesHuman {
			continue
		}
		bodies++
		here += weighted(a, func(g Gene) float64 { return a.groundFactor(g) })
		// The same body, laid over every region in turn: what it would be
		// worth anywhere, which is what "where it is" has to be read against.
		mean := 0.0
		for r := range w.regions {
			scalar := 1.0
			if w.regions[r].Ability > 0 {
				scalar = w.regions[r].Ability
			}
			mean += weighted(a, func(g Gene) float64 { return scalar * w.regions[r].favourOf(g) })
		}
		mean /= float64(len(w.regions))
		all += mean
		// And the best this body could have done, which is what its actual
		// ground has to be read against.
		top := -1.0
		for r := range w.regions {
			scalar := 1.0
			if w.regions[r].Ability > 0 {
				scalar = w.regions[r].Ability
			}
			v := weighted(a, func(g Gene) float64 { return scalar * w.regions[r].favourOf(g) })
			if v > top {
				top = v
			}
		}
		best += top - mean
	}
	if bodies == 0 {
		return out
	}
	out.Here, out.All = here/bodies, all/bodies
	// In percent, because the whole of this stage lives in the third decimal
	// place of a multiplier and a table of zeroes says nothing.
	out.Gain = 100 * (out.Here - out.All)
	out.Ceiling = 100 * best / bodies
	return out
}

// homeFor is where this body came into the world and what being away from it
// costs it (stage 64). A pull of zero is a body tied to nowhere: every human,
// and every enemy in a world that has not asked for the rule.
//
// Home is the middle of the block it came into the world in rather than the
// exact spot: the finest thing this world says about a place is a region
// (#53), and a body that wandered ten paces from where it was born is not
// away from home.
func (w *World) homeFor(a *Agent) (x, y, pull float64) {
	if w.cfg.EnemyHomeCost <= 0 || a.Species != SpeciesEnemy {
		return 0, 0, 0
	}
	if a.HomeRegion < 0 || a.HomeRegion >= len(w.regions) {
		return 0, 0, 0
	}
	homely := w.kindOf(a).Homely
	if homely <= 0 {
		return 0, 0, 0
	}
	minX, minY, maxX, maxY := w.regionBounds(a.HomeRegion)
	return (minX + maxX) / 2, (minY + maxY) / 2, w.cfg.EnemyHomeCost * homely
}

// AwayFromHome is how far this body has got from the country it came into the
// world in, in the width of a region, and what it is being charged for it
// (stage 64). For the viewer; a pull of zero is a body tied to nowhere.
func (w *World) AwayFromHome(id int) (away, pull float64) {
	a := w.agentByID(id)
	if a == nil {
		return 0, 0
	}
	hx, hy, pull := w.homeFor(a)
	if pull <= 0 {
		return 0, 0
	}
	span := max(w.cfg.Width/float64(max(w.cfg.RegionCols, 1)), 1)
	return math.Hypot(a.X-hx, a.Y-hy) / span, pull
}

// ProwlAt is how many of the world's arriving enemies turn up around this
// spot, relative to an equal share (stage 58), for the viewer. One everywhere
// in a world where they arrive alike. No agent is ever told this.
func (w *World) ProwlAt(x, y float64) float64 { return w.prowlAt(x, y) }

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
