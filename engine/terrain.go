package engine

// This file is where the ground an agent is standing on will go.
//
// Nothing here does anything yet: every query returns flat ground, and the
// simulation runs exactly as it did without it. It is here for the same reason
// the Species label was put in before there was a second species - so that the
// place the rule will live is already decided, and adding it later is a change
// to one function rather than to every caller.
//
// What it is for. Speed is the most bought gene there is (0.230 of the budget)
// and it still does not trade against anything: being fast is simply good, so
// there is no "fast and frail against slow and tough" to be found. What is
// missing is somewhere that being fast does not help. Ground that costs more
// to cross is the cheapest way to make one, and it is the only one that acts
// on speed rather than on judgement (an obstacle is a question about route
// finding, which is intelligence; a narrow place is a question about being
// cornered, which is defence - see PLAN.md).
//
// Deliberately not a region. Stage 14 gives the world regions - blocks that
// hold how good the resting is and how well the plants grow - and terrain is
// not one of those. A region is a unit of what the world provides; terrain is
// a map of what movement costs. They will overlap on the ground and stay
// separate in the code.

// terrain is what the ground does to an agent crossing it.
//
// One field for now. Whatever is added later (what it does to sight, whether
// it can be crossed at all) belongs here, so that the callers keep asking one
// question rather than growing a new one each time.
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
}

// flatGround is what the whole world is made of today.
var flatGround = terrain{Cost: 1}

// terrainAt is the one place the ground is asked about. Everything that moves
// goes through it, so that giving the world hills is a change here and nowhere
// else.
//
// It takes a position it does not use yet. That is the point: a version that
// took no arguments would have to be found and changed at every call site the
// day the ground stops being uniform.
func (w *World) terrainAt(x, y float64) terrain {
	_, _ = x, y
	return flatGround
}

// moveCostOn is what a tick of movement at this effort costs on this ground.
// The controller works its plans out with moveCostAt, which knows nothing
// about the ground: an agent plans as though the country ahead were as easy as
// the country it has crossed, and finds out otherwise by paying. When terrain
// arrives that gap becomes a real thing agents can be wrong about, and closing
// it - if it should be closed - means putting the ground into Perception,
// which is the terrain stage's job and not this groundwork's.
func (w *World) moveCostOn(x, y, effort float64) float64 {
	return moveCostAt(&w.cfg, effort) * w.terrainAt(x, y).Cost
}
