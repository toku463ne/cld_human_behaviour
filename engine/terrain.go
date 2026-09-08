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

	// Drown is the chance that a tick spent on this cell is the last one
	// (stage 34). Water is the only ground that carries one today.
	//
	// It sits on the cell rather than in a table of hazards because one
	// example is not enough to know what the table should look like. When a
	// second kind of dangerous ground turns up - a cliff, a bog - the two of
	// them will say what they have in common, and that is the moment to
	// gather them up (#60).
	Drown float64

	Kind Ground
}

// flatGround is what a world with no map is made of, and what every cell off
// the edge of a map is.
var flatGround = terrain{Cost: 1, Kind: GroundOpen}

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
		return terrain{Cost: cfg.RoughMoveCost, Kind: GroundRough}
	case c == '~':
		return terrain{Cost: cfg.WaterMoveCost, Drown: cfg.DrownChancePerTick, Kind: GroundWater}
	case c >= '1' && c <= '9':
		return terrain{Cost: 1, Height: int8(c - '0'), Kind: GroundOpen}
	case c >= 'A' && c <= 'I':
		return terrain{Cost: cfg.SlopeMoveCost, Height: int8(c-'A') + 1, Slope: true, Kind: GroundSlope}
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
	return moveCostAt(&w.cfg, effort) * w.groundCostFor(a, w.terrainAt(x, y))
}

// groundCostFor is what this ground costs this body: the terrain's own figure,
// less whatever the body knows about crossing it.
//
// Only the excess over level ground is relieved, so no amount of skill makes
// broken country cheaper than a field. A body with no skill, or a world with
// the rule off, gets the terrain's figure unchanged, which is why a world
// without skills runs exactly as it did.
func (w *World) groundCostFor(a *Agent, t terrain) float64 {
	if t.Cost <= 1 || w.cfg.SkillRoughRelief <= 0 || a == nil {
		return t.Cost
	}
	relief := clamp(a.skillAt(&w.cfg, SkillRough)*w.cfg.SkillRoughRelief, 0, 1)
	return 1 + (t.Cost-1)*(1-relief)
}

// canStep says whether a body standing on one spot may put itself on another.
//
// Level ground is always passable, whatever it is made of: water is dear, not
// forbidden. What is forbidden is a change of level anywhere but a ramp, and
// more than one level at a time anywhere at all. That one rule gives high
// ground its edge (there are only so many ways in), gives slopes their job,
// and stacks: getting to the third level means finding a ramp on the second.
func (w *World) canStep(fromX, fromY, toX, toY float64) bool {
	if w.ground == nil {
		return true
	}
	from, to := w.terrainAt(fromX, fromY), w.terrainAt(toX, toY)
	if from.Height == to.Height {
		return true
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
type GroundView struct {
	Kind   Ground
	Cost   float64
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
	return GroundView{Kind: t.Kind, Cost: t.Cost, Height: int(t.Height), Slope: t.Slope, Drown: t.Drown}
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
	if t.Drown <= 0 || w.cfg.SkillSwimRelief <= 0 || a == nil {
		return t.Drown
	}
	relief := clamp(a.skillAt(&w.cfg, SkillSwim)*w.cfg.SkillSwimRelief, 0, 1)
	return t.Drown * (1 - relief)
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
			a.drowned = true
			// Before the body is taken out of the world, because the ones who
			// are about to learn from it are the ones who can see it where it
			// is (stage 35).
			w.witnessDrowning(a)
			w.kill(a)
		}
	}
}
