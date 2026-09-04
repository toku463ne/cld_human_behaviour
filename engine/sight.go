package engine

import "math"

// What an agent can see (stage 13).
//
// Sight used to be a circle of PerceptionRadius around the agent. It is now a
// block of cells: the cell the agent is standing in, and SightCells rings of
// cells around it. The circle is still there behind SightGrid=false, as the arm
// to compare against.
//
// The point of the change is that a block is not a circle, and the difference
// is a real thing rather than an approximation of one. A circle sees the same
// distance in every direction and does not care where in the world the agent is
// standing. A block does both: an agent that has just crossed into a new cell
// sees a long way back the way it came and hardly any of the way it is going,
// and everything in the far corners is visible while things nearer but in the
// wrong direction are not. That anisotropy is what changes running away and
// racing for food, which is what the stage is measured on.
//
// The sight grid is deliberately NOT the spatial index (grid.go). The index is
// an optimisation whose whole contract is that it cannot change the results,
// which is why its cell size is derived rather than configured. Sight is a rule
// of the world, so it gets its own size in Config. They will not line up on the
// ground and they do not need to.

// sightCell is which cell of the sight grid a position falls in. Negative
// coordinates would round towards zero rather than down, so it uses Floor: the
// world has a margin and nothing should be outside it, but a cell index that
// folded two strips of the world into one would be a silent, one-sided error.
func sightCell(v, size float64) int {
	if size <= 0 {
		return 0
	}
	return int(math.Floor(v / size))
}

// canSee reports whether something at (bx, by) is inside the field of view of
// an agent at (ax, ay).
//
// It takes positions rather than agents because half the callers are asking
// about food, and because the answer must not depend on anything but where the
// two things are.
func (w *World) canSee(ax, ay, bx, by float64) bool {
	cfg := &w.cfg
	if !cfg.SightGrid {
		return dist2(ax, ay, bx, by) <= cfg.PerceptionRadius*cfg.PerceptionRadius
	}
	s := cfg.SightCellSize
	dx := sightCell(ax, s) - sightCell(bx, s)
	dy := sightCell(ay, s) - sightCell(by, s)
	return abs(float64(dx)) <= float64(cfg.SightCells) && abs(float64(dy)) <= float64(cfg.SightCells)
}

// sightRange is how far away the furthest visible thing could possibly be. It
// is what the spatial index is queried with, so it has to be a bound and never
// an estimate: anything the query misses is invisible to the agent whatever
// canSee would have said about it.
//
// For the block, the agent may be standing anywhere in its own cell, so the
// visible region reaches SightCells+1 cells away along each axis in the worst
// case, and the far corner of that is the diagonal of it.
func (w *World) sightRange() float64 {
	cfg := &w.cfg
	if !cfg.SightGrid {
		return cfg.PerceptionRadius
	}
	return float64(cfg.SightCells+1) * cfg.SightCellSize * math.Sqrt2
}

// appendAgentsInSight and appendFoodsInSight ask the index for everything that
// could possibly be visible from this position. They are the only way the sight
// callers should reach the index: asking for a radius instead would cover the
// block four times over, because the agent is not standing in the middle of it.
func (w *World) appendAgentsInSight(dst []int, x, y float64) []int {
	minX, minY, maxX, maxY := w.SightBlock(x, y)
	return w.spatialIndex().appendAgentsInBox(dst, minX, minY, maxX, maxY)
}

func (w *World) appendFoodsInSight(dst []int, x, y float64) []int {
	minX, minY, maxX, maxY := w.SightBlock(x, y)
	return w.spatialIndex().appendFoodsInBox(dst, minX, minY, maxX, maxY)
}

// SightBlock is the corner of the region an agent at this position can see, for
// the viewer. It is the whole region for the block, and the bounding box of the
// circle otherwise. Read only.
func (w *World) SightBlock(x, y float64) (minX, minY, maxX, maxY float64) {
	cfg := &w.cfg
	if !cfg.SightGrid {
		r := cfg.PerceptionRadius
		return x - r, y - r, x + r, y + r
	}
	s := cfg.SightCellSize
	cx, cy := sightCell(x, s), sightCell(y, s)
	n := cfg.SightCells
	return float64(cx-n) * s, float64(cy-n) * s,
		float64(cx+n+1) * s, float64(cy+n+1) * s
}
