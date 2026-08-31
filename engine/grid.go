package engine

import (
	"math"
	"slices"
)

// This file holds the spatial index: a uniform grid of cells that says who and
// what is roughly where, so that a question about a neighbourhood does not have
// to walk the whole world. It is stage 7a of PLAN.md, whose whole point is that
// it changes nothing: the same seed must give the same run afterwards, to the
// last agent. The grid narrows the field down to a handful of cells and the
// callers still run the same circular distance test they always did.
//
// Nothing about the world's rules lives here. The cell size is not in Config
// for that reason: Config is the list of the rules of the world, and a change
// here has to be invisible in the results. It is derived from PerceptionRadius
// instead, which is the widest question anybody asks, so that the two can never
// drift apart.
//
// The index is a snapshot, not a running total. It is rebuilt from the agent
// and food slices whenever something has moved, been born, eaten or died, and
// the rebuild is lazy: the flag is set on the way past and the work happens at
// the next query. In an ordinary tick that comes to two rebuilds, one for the
// decisions and one after everybody has moved.
//
// The one thing to watch: a query in the middle of the movement loop would find
// the index stale on every call and rebuild it every time, which is worse than
// the linear scan it replaced. Ask before moving, or after, not during.

// maxGridCells caps the grid so that a Config with a tiny perception radius
// cannot ask for millions of buckets. Past the cap the cells simply get bigger,
// which costs speed and nothing else.
const maxGridCells = 1 << 14

// spatialGrid buckets agent and food indices by cell.
//
// It holds indices into World.agents and World.foods rather than IDs, because
// the callers want to look at the agent itself and an index is one step from
// it; that is also why every rebuild has to follow anything that reorders those
// slices (removeDead compacts them, and removing food swaps the last item into
// the hole).
type spatialGrid struct {
	cell       float64
	cols, rows int

	agents [][]int
	foods  [][]int
}

func newSpatialGrid(width, height, cell float64) *spatialGrid {
	if !(width > 0) {
		width = 1
	}
	if !(height > 0) {
		height = 1
	}
	// A cell that is missing or absurd degenerates into a single bucket, which
	// is exactly the linear scan this replaces: slower, never wrong.
	if !(cell > 0) {
		cell = math.Max(width, height)
	}
	for {
		// Points on or past the far edge are clamped into the last cell, so
		// the grid only has to cover the world itself.
		cols := max(int(math.Ceil(width/cell)), 1)
		rows := max(int(math.Ceil(height/cell)), 1)
		if cols*rows <= maxGridCells {
			g := &spatialGrid{cell: cell, cols: cols, rows: rows}
			g.agents = make([][]int, cols*rows)
			g.foods = make([][]int, cols*rows)
			return g
		}
		cell *= 2
	}
}

// column and row clamp a coordinate into the grid. Clamping rather than
// rejecting means a point that has strayed outside the world is still found,
// in the edge cell nearest to it, instead of quietly disappearing from every
// query.
func (g *spatialGrid) column(x float64) int {
	return clampInt(int(math.Floor(x/g.cell)), 0, g.cols-1)
}

func (g *spatialGrid) row(y float64) int {
	return clampInt(int(math.Floor(y/g.cell)), 0, g.rows-1)
}

func (g *spatialGrid) cellIndex(x, y float64) int {
	return g.row(y)*g.cols + g.column(x)
}

// rebuild refills every bucket from scratch. The buckets keep their capacity,
// so a steady population stops allocating after the first few ticks.
func (g *spatialGrid) rebuild(agents []Agent, foods []Food) {
	for i := range g.agents {
		g.agents[i] = g.agents[i][:0]
		g.foods[i] = g.foods[i][:0]
	}
	for i := range agents {
		c := g.cellIndex(agents[i].X, agents[i].Y)
		g.agents[c] = append(g.agents[c], i)
	}
	for i := range foods {
		c := g.cellIndex(foods[i].X, foods[i].Y)
		g.foods[c] = append(g.foods[c], i)
	}
}

// appendAgentsNear appends the indices of every agent in the cells the circle
// of the given radius touches. It is a superset of the answer: the caller keeps
// its own distance test, which is what makes the index invisible in the
// results.
//
// The indices come back in ascending order, which matters more than it looks.
// Perceiving a neighbour draws from the world's random source, so the order the
// neighbours are visited in is part of what a seed reproduces; ascending order
// is the order the linear scan used, and the only one that leaves a run
// unchanged.
func (g *spatialGrid) appendAgentsNear(dst []int, x, y, radius float64) []int {
	return g.appendNear(g.agents, dst, x, y, radius)
}

// appendFoodsNear is appendAgentsNear for food.
func (g *spatialGrid) appendFoodsNear(dst []int, x, y, radius float64) []int {
	return g.appendNear(g.foods, dst, x, y, radius)
}

func (g *spatialGrid) appendNear(buckets [][]int, dst []int, x, y, radius float64) []int {
	if radius < 0 {
		radius = 0
	}
	c0, c1 := g.column(x-radius), g.column(x+radius)
	r0, r1 := g.row(y-radius), g.row(y+radius)
	start := len(dst)
	for r := r0; r <= r1; r++ {
		base := r * g.cols
		for c := c0; c <= c1; c++ {
			dst = append(dst, buckets[base+c]...)
		}
	}
	// Cells are visited row by row, which is not the order the slices are in.
	slices.Sort(dst[start:])
	return dst
}

func clampInt(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// --- the world's copy -------------------------------------------------------

// gridCellSize is the cell the world indexes at: the widest radius anybody
// asks about, so that every query fits inside a three by three block of cells.
//
// Smaller cells would hand back fewer candidates per query (the block around a
// circle of radius r shrinks from 9r^2 towards 4r^2) at the price of visiting
// more of them. Which way that trade lands is a question for the measurement in
// the next item, once there is a caller to measure.
func gridCellSize(cfg *Config) float64 { return cfg.PerceptionRadius }

// invalidateIndex says the grid no longer matches the world. Everything that
// moves an agent, adds or removes one, or touches the food calls this; the
// rebuild waits until somebody actually asks a question.
func (w *World) invalidateIndex() { w.gridStale = true }

// spatialIndex returns the spatial index, rebuilding it first if the world has moved
// on since it was last built.
func (w *World) spatialIndex() *spatialGrid {
	if w.grid == nil {
		w.grid = newSpatialGrid(w.cfg.Width, w.cfg.Height, gridCellSize(&w.cfg))
		w.gridStale = true
	}
	if w.gridStale {
		w.grid.rebuild(w.agents, w.foods)
		w.gridStale = false
	}
	return w.grid
}
