package engine

import "math"

// The ground an agent is standing on (stage 20).
//
// What it is for. Speed is the most bought gene there is (0.230 of the budget)
// and until now it did not trade against anything: being fast was simply good,
// so "fast and frail against slow and tough" never appeared. What was missing
// was somewhere that being fast does not help. Ground that costs more to cross
// is the cheapest way to make one, and it is the only kind of terrain that
// acts on speed rather than on judgement (an obstacle is a question about
// route finding, which is intelligence; a narrow place is a question about
// being cornered, which is defence - see PLAN.md).
//
// Two decisions were left to this stage, and here they are.
//
// Height is an axis on the plane, not a second layer. Every cell carries how
// high it is, so a plateau is a patch of larger numbers and a plateau on a
// plateau is larger ones still. The alternative - keeping levels as separate
// maps - would make every other rule in the world say which level it meant
// (where food grew, who could see whom, who was within reach), and nothing
// else in the world is layered. A step that changes level is only allowed
// through a slope, which is the whole of "you can only get up there by the
// ramp" and needs no new concept: a slope is a cell, like every other cell.
//
// Perception carries the ground underfoot and nothing else, and it carries the
// same thing for a player as for the AI (stage 19's promise). An agent knows
// what it is standing on - that is its own body against its own ground - and
// assumes the country ahead is like it. That assumption is wrong exactly as
// often as the world is varied, which is the point: terrain is a cost you find
// out about by paying it, not a re-priced straight line. Giving the AI a map
// it could plan over would be giving it route finding, and route finding is a
// different stage with a different gene behind it.

// Ground is what a cell is made of. It is not what it costs - two kinds can
// cost the same - but what an interface draws and what a map file spells.
type Ground uint8

const (
	GroundOpen  Ground = iota // level, cheap, the whole world before this stage
	GroundRough               // broken country: crossing it costs more
	GroundWater               // a river: crossable, and dear
	GroundSlope               // a ramp: the only way between two levels
)

// terrain is what the ground does to an agent crossing it.
type terrain struct {
	// Cost multiplies what a tick of movement takes out of an agent. 1 is
	// level open ground; above 1 is ground that punishes crossing it.
	//
	// It is a multiplier on the cost and not on the speed on purpose. Slowing
	// an agent down is already what effort does, and doubling up would make
	// the two impossible to tell apart in a measurement. Making the ground
	// expensive rather than slow is also what puts speed and vitality on
	// opposite sides for the first time: crossing rough country quickly is
	// exactly the trade a fast, frail body loses and a slow, tough one wins.
	Cost float64

	// Height is how many levels above the bottom this cell sits, and Slope
	// says it may be entered from the level below. A step between cells of
	// different height is allowed only when the higher of the two is a slope,
	// which makes a ramp the only way up and the only way down.
	Height int8
	Slope  bool

	// Slow is what this cell does to a body's speed, as a multiplier (stage
	// 97). One - or nought, which is what a cell that never set it holds - is
	// ground that does nothing to a pace; below one is ground that drags.
	//
	// It sits on the cell beside Cost rather than being worked out from the
	// Kind, so that the second slow ground needs a line in cellFor and
	// nothing else. Cost and Slow are deliberately two figures: the same
	// cell can be dear without being slow (broken country takes it out of a
	// body, but a body crosses it at a walk) or slow without being dear.
	Slow float64

	// Drown is the chance that a tick spent on this cell is the last one
	// (stage 34). Water is the only ground that carries one today.
	//
	// It sits on the cell rather than in a table of hazards because one
	// example is not enough to know what the table should look like. When a
	// second kind of dangerous ground turns up - a cliff, a bog - the two of
	// them will say what they have in common, and that is the moment to
	// gather them up (#60).
	Drown float64

	// Drain is what a tick spent on this cell takes out of a body in vitality,
	// whatever it is doing (stage 99). Water is the only ground that carries
	// one today.
	//
	// A drain and not another multiplier on the cost, because the cost is only
	// charged on a tick the body actually moved: standing in a river was free
	// until this, which is the hole stage 97 measured from the other side
	// ("standing still is the one thing that does not get slower"). It is also
	// the only shape the lookahead reads without a line of new code - the
	// chill (stage 85) goes into the same figure - so a place that takes
	// something is a place a body can reckon with.
	Drain float64

	Kind Ground
}

// flatGround is what a world with no map is made of, and what every cell off
// the edge of a map is.
var flatGround = terrain{Cost: 1, Slow: 1, Kind: GroundOpen}

// terrainGrid is the map: cells of equal size laid over the world.
//
// The size of a cell comes from the map itself - a map of 20 by 15 over an 800
// by 600 world gives cells of 40 - so that the same file describes the same
// country whatever the world's dimensions are.
type terrainGrid struct {
	cols, rows   int
	cellW, cellH float64
	cells        []terrain
}

func (g *terrainGrid) at(x, y float64) terrain {
	if g == nil || g.cols == 0 || g.rows == 0 {
		return flatGround
	}
	cx := int(math.Floor(x / g.cellW))
	cy := int(math.Floor(y / g.cellH))
	if cx < 0 || cy < 0 || cx >= g.cols || cy >= g.rows {
		return flatGround
	}
	return g.cells[cy*g.cols+cx]
}

// buildTerrain reads a map into a grid. Each string is a row and each rune a
// cell:
//
//	.       open ground at the bottom level
//	:       rough country: the cost of crossing it is RoughMoveCost
//	~       water: WaterMoveCost, and still crossable - a river is dear, not
//	        a wall, because a wall is an obstacle and obstacles are about route
//	        finding rather than about speed
//	1 - 9   open ground that many levels up
//	A - I   a slope up to that many levels (A is level 1), SlopeMoveCost
//
// Anything else is read as open ground, and a nil or empty map is a flat
// world - which is the default, so a world that says nothing about its ground
// runs exactly as it did before this stage.
func buildTerrain(cfg *Config) *terrainGrid {
	rows := cfg.TerrainMap
	if len(rows) == 0 {
		return nil
	}
	cols := 0
	for _, r := range rows {
		if n := len([]rune(r)); n > cols {
			cols = n
		}
	}
	if cols == 0 {
		return nil
	}
	g := &terrainGrid{
		cols: cols, rows: len(rows),
		cellW: cfg.Width / float64(cols), cellH: cfg.Height / float64(len(rows)),
		cells: make([]terrain, cols*len(rows)),
	}
	for y, row := range rows {
		runes := []rune(row)
		for x := 0; x < cols; x++ {
			c := '.'
			if x < len(runes) {
				c = runes[x]
			}
			g.cells[y*cols+x] = cellFor(c, cfg)
		}
	}
	return g
}

func cellFor(c rune, cfg *Config) terrain {
	switch {
	case c == ':':
		return terrain{Cost: cfg.RoughMoveCost, Slow: 1, Kind: GroundRough}
	case c == '~':
		return terrain{Cost: cfg.WaterMoveCost, Slow: cfg.WaterSpeedShare,
			Drown: cfg.DrownChancePerTick, Drain: cfg.WaterDrain, Kind: GroundWater}
	case c >= '1' && c <= '9':
		return terrain{Cost: 1, Slow: 1, Height: int8(c - '0'), Kind: GroundOpen}
	case c >= 'A' && c <= 'I':
		return terrain{Cost: cfg.SlopeMoveCost, Slow: 1, Height: int8(c-'A') + 1, Slope: true, Kind: GroundSlope}
	default:
		return flatGround
	}
}

// terrainAt is the one place the ground is asked about. Everything that moves
// goes through it.
func (w *World) terrainAt(x, y float64) terrain {
	return w.ground.at(x, y)
}

// moveCostOn is what a tick of movement at this effort costs one body on this
// ground.
//
// The body is in here because of stage 38a: what broken country takes out of
// somebody who knows how to cross it is less than what it takes out of
// somebody who does not. The multiplication is the same one that was already
// here - the skill lowers the ground's figure, it does not add a factor of its
// own to the agent.
func (w *World) moveCostOn(a *Agent, x, y, effort float64) float64 {
	// ... and what it is carrying (stage 40). The load goes on the cost and
	// not on the speed, for the same reason the ground's figure does: effort
	// is already on the speed, and two things there cannot be told apart in a
	// measurement.
	return moveCostAt(&w.cfg, effort) * w.groundCostFor(a, w.terrainAt(x, y)) * a.burden(&w.cfg)
}

// groundCostFor is what this ground costs this body: the terrain's own figure,
// less whatever the body knows about crossing it.
//
// Only the excess over level ground is relieved, so no amount of skill makes
// broken country cheaper than a field. A body with no skill, or a world with
// the rule off, gets the terrain's figure unchanged, which is why a world
// without skills runs exactly as it did.
func (w *World) groundCostFor(a *Agent, t terrain) float64 {
	// A flier pays its own price instead of the ground's, whatever the ground
	// is (2026-09-20). Not nothing: no ground is impassable and none is free.
	if a != nil && a.Species == SpeciesEnemy {
		if k := w.kindOf(a); k.Flies {
			if k.FlyCost > 0 {
				return k.FlyCost
			}
			return 1
		}
	}
	if t.Cost <= 1 || w.cfg.SkillRoughRelief <= 0 || a == nil {
		return t.Cost
	}
	relief := clamp(a.skillAt(&w.cfg, SkillRough)*w.cfg.SkillRoughRelief, 0, 1)
	return 1 + (t.Cost-1)*(1-relief)
}

// --- what the ground does to a pace (stage 97) -------------------------------

// groundSpeedFor is what this ground does to this body's speed: the terrain's
// own figure, less whatever the body knows about getting through it.
//
// Only the shortfall below level ground is relieved, so no amount of skill
// makes the water quicker than a field - the mirror of groundCostFor, and the
// same reason: a skill takes the ground's penalty off, it does not add a
// factor of its own to the agent.
func (w *World) groundSpeedFor(a *Agent, t terrain) float64 {
	// Nothing on the ground drags a flier (2026-09-20).
	if a != nil && a.Species == SpeciesEnemy && w.kindOf(a).Flies {
		return 1
	}
	slow := t.Slow
	if slow <= 0 || slow >= 1 {
		return 1
	}
	if a == nil {
		return slow
	}
	// A creature of the water is not dragged by the water (stage 63). The
	// same flag that puts it there, and the same one fact: what lives in the
	// river does not wade through it.
	if a.Species == SpeciesEnemy && w.kindOf(a).Water {
		return 1
	}
	if w.cfg.SkillSwimSpeedRelief <= 0 {
		return slow
	}
	relief := clamp(a.skillAt(&w.cfg, SkillSwim)*w.cfg.SkillSwimSpeedRelief, 0, 1)
	return slow + (1-slow)*relief
}

// wade tells every living body what the water under it is doing to its pace,
// once a tick and before anybody decides anything.
//
// It is held on the agent for the reason standOnGround (stage 57) holds
// footing there: Agent.speedNow is asked many times a tick and knows nothing
// about the world it is in, while the four places that read a speed - walking,
// dodging, holding a stance, and what a body knows about itself - all have to
// read the same one. A tick's worth of staleness is the price, and it is the
// price everything else about this tick pays: what a body decides now, it
// decides about where it was when the tick began.
//
// A world whose water does not slow anybody never enters the loop, so it is
// not merely unchanged but untouched.
func (w *World) wade() {
	if w.ground == nil || w.cfg.WaterSpeedShare >= 1 {
		return
	}
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		a.wading = w.groundSpeedFor(a, w.terrainAt(a.X, a.Y))
	}
}

// speedFelt is what a body makes of its own legs: the truth, or the legs it
// would have on dry ground in the arm that takes the feeling away and leaves
// the water exactly as slow (stage 97, and the same shape as drownFelt).
func (w *World) speedFelt(a *Agent) float64 {
	speed := a.speedNow(&w.cfg)
	if !w.cfg.WaterSlowKnown && a.wading > 0 {
		speed /= a.wading
	}
	return speed
}

// Wading is what the water is doing to the bodies that are in it (stage 97).
// Read only, and it writes nothing and draws no random number.
type Wading struct {
	// In is the share of the living standing in water that drags.
	In float64

	// Pace is the mean multiplier those bodies are actually moving at, and
	// Floor what the ground alone would have made it. The gap between the two
	// is what knowing the water bought, measured on the bodies that are in it
	// rather than over a population most of which is on dry land - which is
	// the distinction stage 62 found mattered (a skill's reach is its holding
	// times its realised value times the size of what it acts on).
	//
	// Both are one when nobody is in the water, which is the reading that
	// composes: a sample where nothing is being dragged says "no drag", and
	// averaging such samples with the rest cannot pull the figure below the
	// floor the ground actually sets.
	Pace, Floor float64
}

// Wading reports it.
func (w *World) Wading() Wading {
	out := Wading{Pace: 1, Floor: 1}
	var n, all float64
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		all++
		t := w.terrainAt(a.X, a.Y)
		if t.Slow <= 0 || t.Slow >= 1 {
			continue
		}
		if n == 0 {
			out.Pace, out.Floor = 0, 0
		}
		n++
		out.Pace += w.groundSpeedFor(a, t)
		out.Floor += t.Slow
	}
	if all > 0 {
		out.In = n / all
	}
	if n > 0 {
		out.Pace /= n
		out.Floor /= n
	}
	return out
}

// canStep says whether a body standing on one spot may put itself on another.
//
// Level ground is always passable, whatever it is made of: water is dear, not
// forbidden. What is forbidden is a change of level anywhere but a ramp, and
// more than one level at a time anywhere at all. That one rule gives high
// ground its edge (there are only so many ways in), gives slopes their job,
// and stacks: getting to the third level means finding a ramp on the second.
func (w *World) canStep(a *Agent, fromX, fromY, toX, toY float64) bool {
	if w.ground == nil {
		return true
	}
	from, to := w.terrainAt(fromX, fromY), w.terrainAt(toX, toY)
	if from.Height == to.Height {
		return true
	}
	// How this body gets about (2026-09-20). A flier is over the ground and
	// a level is nothing to it; a climber goes up and down a cliff but still
	// one level at a time, which is what keeps a stack of them a route and
	// not a door.
	if a != nil && a.Species == SpeciesEnemy {
		k := w.kindOf(a)
		// How high it flies, when its row says (2026-09-20). Above its
		// ceiling it is turned back and falls through to the ground's own
		// rules below: a flier is never worse off than a walker.
		if k.Flies && (k.FlyHeight <= 0 || int(to.Height) <= k.FlyHeight) {
			return true
		}
		if k.Climbs {
			diff := int(from.Height) - int(to.Height)
			return diff <= 1 && diff >= -1
		}
	}
	if diff := int(from.Height) - int(to.Height); diff > 1 || diff < -1 {
		return false
	}
	if from.Height > to.Height {
		return from.Slope
	}
	return to.Slope
}

// GroundView is what an interface or a measurement may read about one spot.
// The agents get the cost and nothing else (Perception.Self.Ground); this is
// for whoever is drawing the world or counting who stands where.
// GroundRead is what a body makes of a piece of ground it has not reached:
// what crossing it would multiply the cost by, and what standing on it would
// drain per tick (stage 100). Both carry the reader's own error.
// Chill is the weather one cell along, on the map the weather is on (#135),
// after whatever the body is carrying answers it. It is beside Drain rather
// than added into it because the two are different facts on different maps -
// the ground is wet, the place is cold - and an author sets them separately.
type GroundRead struct{ Cost, Drain, Chill float64 }

type GroundView struct {
	Kind   Ground
	Cost   float64
	Slow   float64
	Height int
	Slope  bool
	Drown  float64
}

// TerrainAt reports what the country is at a position. Read only.
//
// Named for the terrain rather than the ground because World.GroundAt was
// already taken by the region's answer to a different question - how sheltered
// the resting is here and how well the plants grow (region.go). The two maps
// are deliberately separate: a region is a unit of what the world provides, a
// terrain cell is what movement costs.
func (w *World) TerrainAt(x, y float64) GroundView {
	t := w.terrainAt(x, y)
	slow := t.Slow
	if slow <= 0 {
		slow = 1
	}
	return GroundView{Kind: t.Kind, Cost: t.Cost, Slow: slow,
		Height: int(t.Height), Slope: t.Slope, Drown: t.Drown}
}

// TerrainSize is how many cells the map has, and how big one is. Zero when the
// world is flat.
func (w *World) TerrainSize() (cols, rows int, cellW, cellH float64) {
	if w.ground == nil {
		return 0, 0, 0, 0
	}
	return w.ground.cols, w.ground.rows, w.ground.cellW, w.ground.cellH
}

// --- what the ground kills (stage 34) ---------------------------------------

// drownChanceFor is what this water may do to this body: the ground's own
// figure, less whatever the body knows about being in it (stage 38b).
//
// It is the hazard that is relieved and not the cost of crossing, for the
// reason stage 34 gave: what decides who comes out is how many ticks are spent
// in there, and a body that knows the water spends them at a lower rate rather
// than getting across sooner.
func (w *World) drownChanceFor(a *Agent, t terrain) float64 {
	if a == nil || t.Drown <= 0 {
		return t.Drown
	}
	// A creature of the water is not at risk in the water (stage 63). It is
	// the same flag that puts it there: what lives in the river is not
	// something the river takes.
	if k := w.kindOf(a); a.Species == SpeciesEnemy && (k.Water || k.Flies) {
		return 0
	}
	chance := t.Drown * w.drownFactorFor(a)
	if w.cfg.SkillSwimRelief <= 0 {
		return chance
	}
	relief := clamp(a.skillAt(&w.cfg, SkillSwim)*w.cfg.SkillSwimRelief, 0, 1)
	return chance * (1 - relief)
}

// drownFactorFor is the other end of the same curve (stage 98): what the water
// asks of a body that cannot swim, over what it asks of one that can.
//
// One curve and not a second rule. It runs from DrownUnskilledFactor at no
// swimming down to one at DrownSkillFull, and SkillSwimRelief carries it on
// down from there - so a body that knows the water faces exactly the chance it
// faced before this stage, and everything the ground is charged for is still
// the ground's own figure times something the body brings.
//
// Reaching one at a mastery that bodies in this world actually attain is the
// whole of the shape. Counted before it was built: 71% of the bodies standing
// in a river hold no swimming at all, and the ones that do are up around 0.5,
// so a curve that only got there at a mastery of one would charge the ones who
// know the water nearly the full penalty - which is the flat rise this stage
// has to be told apart from.
func (w *World) drownFactorFor(a *Agent) float64 {
	f := w.cfg.DrownUnskilledFactor
	if f <= 1 || a == nil {
		return 1
	}
	learned := 0.0
	if full := w.cfg.DrownSkillFull; full > 0 {
		learned = clamp(a.skillAt(&w.cfg, SkillSwim)/full, 0, 1)
	}
	return 1 + (f-1)*(1-learned)
}

// Drowning is what the water is asking of the bodies that are in it (stage
// 98). Read only: nothing here writes to the world or draws a random number.
type Drowning struct {
	// In is the share of the living standing in water at all.
	In float64

	// Chance is the mean chance a tick in there is the last one, as the bodies
	// standing in it actually face it, and Floor the ground's own figure for
	// those same bodies. Chance/Floor is the multiplier the population is
	// actually paying, which is the figure a flat arm has to be set to: an arm
	// that raises the ground's figure for everybody by the same mean says how
	// much of whatever moves is the spread rather than the level.
	Chance, Floor float64

	// Skill is the mean realised swimming of those bodies and Unskilled the
	// share of them holding none at all. The second is the one that matters
	// here - what this world has is not a low mean but two humps.
	Skill, Unskilled float64

	// TakenSkill is the mean realised swimming of every body the water has
	// taken since the world began. Below Skill means the river is sorting
	// them; equal to it means the rule is not reaching who drowns.
	TakenSkill float64

	// Still is the share of all the body-ticks spent in the water where the
	// body did not move, and Soaked the vitality the wet ground has taken
	// (stage 99). Still is what that stage aims at: until it, those ticks
	// were free, and stage 97 measured bodies drifting into exactly them.
	Still, Soaked float64
}

// Drowning reports it.
func (w *World) Drowning() Drowning {
	var out Drowning
	var n, all float64
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		all++
		t := w.terrainAt(a.X, a.Y)
		if t.Drown <= 0 {
			continue
		}
		n++
		out.Chance += w.drownChanceFor(a, t)
		out.Floor += t.Drown
		s := a.skillAt(&w.cfg, SkillSwim)
		out.Skill += s
		if s <= 0 {
			out.Unskilled++
		}
	}
	if all > 0 {
		out.In = n / all
	}
	if n > 0 {
		out.Chance /= n
		out.Floor /= n
		out.Skill /= n
		out.Unskilled /= n
	}
	if w.drownDeaths > 0 {
		out.TakenSkill = w.drownTakenSwim / float64(w.drownDeaths)
	}
	if w.wetTicks > 0 {
		out.Still = w.wetStill / w.wetTicks
	}
	out.Soaked = w.soakTaken
	return out
}

// aroundDirs is the eight directions a body reads the ground in, in the order
// SelfView.Around holds them: E, NE, N, NW, W, SW, S, SE.
var aroundDirs = [8][2]float64{
	{1, 0}, {1, -1}, {0, -1}, {-1, -1}, {-1, 0}, {-1, 1}, {0, 1}, {1, 1},
}

// readAround fills in what this body makes of the ground one cell away in each
// direction (stage 100).
//
// One draw per direction, of the size every other reading of the world is
// scaled by - (MaxAbility - rationality)/MaxAbility times GroundAheadNoise -
// and the same draw moves both figures, because a body that misjudges a piece
// of ground misjudges it in one way rather than in two. The error is added to
// the multiplier so that a bad enough reader can take a river for a field, and
// scaled on the drain, which has no natural unit of its own.
//
// Nothing is drawn and nothing is written when the rule is off, which is what
// keeps every world before this one consuming the random source as it did.
func (w *World) readAround(a *Agent, s *SelfView) {
	// A world with neither a ground map nor a weather map has nothing one cell
	// away that is not underfoot, so it reads nothing and draws nothing - which
	// is what keeps every flat world running as it did.
	if !w.cfg.GroundAheadSeen || (w.ground == nil && w.climate == nil) {
		s.AroundSeen = false
		return
	}
	s.AroundSeen = true
	scale := w.judgementScale(a)
	cw, ch := w.cfg.Width, w.cfg.Height
	if w.ground != nil {
		cw, ch = w.ground.cellW, w.ground.cellH
	}
	meanCost, meanDrain := 0.0, 0.0
	if w.cfg.GroundAheadBlind {
		meanCost, meanDrain = w.groundMean()
	}
	// What the weather is doing where the body stands, so that a direction
	// with the same weather as here reads as no change at all (#135). The
	// whole of the ahead reading is differences.
	chillHere := w.chillFelt(a)
	cwx, chy := cw, ch
	if w.climate != nil {
		// A step of the weather's own map, which may be coarser or finer than
		// the ground's: a step shorter than a cell reads the same cell twice
		// and says the weather is flat.
		cwx, chy = w.climate.cellW, w.climate.cellH
	}
	for i, d := range aroundDirs {
		cost, drain := meanCost, meanDrain
		if !w.cfg.GroundAheadBlind {
			t := w.terrainAt(a.X+d[0]*cw, a.Y+d[1]*ch)
			cost, drain = t.Cost, t.Drain
		}
		chill := chillHere
		if w.cfg.ChillAheadSeen && !w.cfg.GroundAheadBlind {
			chill = w.chillAheadFelt(a,
				clamp(a.X+d[0]*cwx, 0, w.cfg.Width-1e-9),
				clamp(a.Y+d[1]*chy, 0, w.cfg.Height-1e-9))
		}
		e := w.noise(scale, w.cfg.GroundAheadNoise)
		s.Around[i] = GroundRead{
			Cost:  math.Max(0.1, cost+e),
			Drain: math.Max(0, drain*(1+e)),
			// The same one draw, for the same reason: a body that misreads a
			// piece of country misreads it in one way rather than in three.
			Chill: math.Max(0, chill*(1+e)),
		}
	}
}

// groundMean is the world's average cell, for the blind arm above: the arm
// looks, draws the same numbers and prices the same way, and what it reads is
// the world rather than the cell it is about to step onto (stage 35's lesson).
func (w *World) groundMean() (cost, drain float64) {
	if w.ground == nil || len(w.ground.cells) == 0 {
		return 1, 0
	}
	for i := range w.ground.cells {
		cost += w.ground.cells[i].Cost
		drain += w.ground.cells[i].Drain
	}
	n := float64(len(w.ground.cells))
	return cost / n, drain / n
}

// soakOf is what the ground under this body takes out of it per tick, whatever
// it is doing (stage 99): the one place a wet cell becomes vitality.
//
// It is kept apart from the weather's own figure (chillOf, stage 85) because
// the two are different facts on different maps - the climate is a property of
// a region and this is a property of a cell - and the author of a map sets
// them separately. What they share is the unit and where they land: both are
// subtracted in metabolise, and both ride into the lookahead through the same
// field, so nothing here needs a rule of its own to be reckoned with.
//
// A creature of the water is not drained by the water (stage 63). Same flag,
// same one fact as not drowning and not being dragged.
func (w *World) soakOf(a *Agent) float64 {
	if a == nil || w.ground == nil || w.cfg.WaterDrain <= 0 {
		return 0
	}
	if k := w.kindOf(a); a.Species == SpeciesEnemy && (k.Water || k.Flies) {
		return 0
	}
	return w.terrainAt(a.X, a.Y).Drain
}

// soakFelt is what this body reads of it: the whole of it in the ordinary
// world, and nothing in the arm where the ground takes just as much and no
// body can feel that it does (the same shape as DrownKnown and ChillKnown).
func (w *World) soakFelt(a *Agent) float64 {
	if !w.cfg.WaterDrainKnown {
		return 0
	}
	return w.soakOf(a)
}

// drownings is the whole of the rule. Every body standing in the water at the
// end of a tick throws once, and the ones that lose are simply gone.
//
// Nothing here is graded and nothing accumulates. A river does not wear a body
// down the way a fight does - which is the point of the shape: damage would
// have had to be divided by defence, and defence would have picked up a second
// job. What decides who comes through is how many ticks were spent in there,
// and that is speed, which the world already has.
//
// There is no threshold and no rule saying to keep out. Whether to be in the
// water at all comes out of the same comparison as everything else, with the
// chance priced into the options that would keep the body there.
//
// A flat world draws no random number here at all, which is why it runs to the
// same fingerprint it always did.
func (w *World) drownings() {
	if w.ground == nil || w.cfg.DrownChancePerTick <= 0 {
		return
	}
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		p := w.drownChanceFor(a, w.terrainAt(a.X, a.Y))
		if p <= 0 {
			continue
		}
		if w.rng.Float64() < p {
			w.drownDeaths++
			w.drownTakenSwim += a.skillAt(&w.cfg, SkillSwim)
			a.drowned = true
			// Before the body is taken out of the world, because the ones who
			// are about to learn from it are the ones who can see it where it
			// is (stage 35).
			w.witnessDrowning(a)
			w.kill(a)
		}
	}
}
