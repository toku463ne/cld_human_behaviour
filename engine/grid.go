package engine

import (
	"math"
	"math/bits"
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
// here has to be invisible in the results. It is derived from the widest
// question anybody asks - which since stage 13 is the block of cells an agent
// can see - so that the two can never drift apart.
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

	// agentMark and foodMark are bitsets of "this index is a candidate",
	// which is how a query puts its answer back into ascending order without
	// comparing anything. See appendNear.
	agentMark []uint64
	foodMark  []uint64

	// How many of each the buckets hold, so that a query covering the whole
	// grid can hand back everything without going through them.
	nAgents, nFoods int
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
	g.nAgents, g.nFoods = len(agents), len(foods)
	g.agentMark = fitMark(g.agentMark, len(agents))
	g.foodMark = fitMark(g.foodMark, len(foods))
}

// fitMark grows a bitset to hold n bits. It never shrinks, and every query
// leaves it clear, so it is only ever cleared once.
func fitMark(mark []uint64, n int) []uint64 {
	for len(mark) < (n+63)/64 {
		mark = append(mark, 0)
	}
	return mark
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
	return g.appendAgentsInBox(dst, x-radius, y-radius, x+radius, y+radius)
}

// appendFoodsNear is appendAgentsNear for food.
func (g *spatialGrid) appendFoodsNear(dst []int, x, y, radius float64) []int {
	return g.appendFoodsInBox(dst, x-radius, y-radius, x+radius, y+radius)
}

// The same two asked for a rectangle instead of a square around a point.
//
// Sight is a block of cells that the agent is not standing in the middle of
// (stage 13), so the square that contains it is nearly four times its area.
// Asking for the block itself is the difference between walking the cells that
// could hold something visible and walking the cells that could hold something
// visible if the agent were somewhere else.
func (g *spatialGrid) appendAgentsInBox(dst []int, minX, minY, maxX, maxY float64) []int {
	return g.appendInBox(g.agents, g.agentMark, g.nAgents, dst, minX, minY, maxX, maxY)
}

func (g *spatialGrid) appendFoodsInBox(dst []int, minX, minY, maxX, maxY float64) []int {
	return g.appendInBox(g.foods, g.foodMark, g.nFoods, dst, minX, minY, maxX, maxY)
}

// appendInBox collects the candidates into a bitset and then reads the bitset
// out, which puts them in ascending order for the price of one bit each.
//
// Sorting them instead is the obvious thing to write, and it is what the first
// version did. It costs a good part of what the index saves. Microseconds per
// tick at populations of 173/364/732/1459/2966 of the same density, best of
// three runs:
//
//	plain scan  101.5  208.6  483.5  1227.8  3656.3
//	bitset      105.3  221.4  435.0  1148.0  2555.2
//	sorted      117.4  256.6  528.4  1252.3  2948.0
//
// Sorted, the index does not overtake the scan it replaced until a couple of
// thousand agents; with the bitset it does so at five or six hundred. The
// indices are small and distinct, which is exactly when a bitset beats a
// comparison sort.
func (g *spatialGrid) appendInBox(buckets [][]int, mark []uint64, n int, dst []int, minX, minY, maxX, maxY float64) []int {
	if maxX < minX {
		minX, maxX = maxX, minX
	}
	if maxY < minY {
		minY, maxY = maxY, minY
	}
	c0, c1 := g.column(minX), g.column(maxX)
	r0, r1 := g.row(minY), g.row(maxY)

	// A question that reaches every cell is not narrowing anything down, which
	// happens whenever the world is small next to the range of sight. Handing
	// back everybody is the same answer for less work: the buckets hold every
	// index exactly once, so this is what the loop below would have built.
	if c0 == 0 && r0 == 0 && c1 == g.cols-1 && r1 == g.rows-1 {
		for i := 0; i < n; i++ {
			dst = append(dst, i)
		}
		return dst
	}

	lo, hi := len(mark), -1
	for r := r0; r <= r1; r++ {
		base := r * g.cols
		for c := c0; c <= c1; c++ {
			for _, i := range buckets[base+c] {
				word := i >> 6
				mark[word] |= 1 << uint(i&63)
				if word < lo {
					lo = word
				}
				if word > hi {
					hi = word
				}
			}
		}
	}
	// Reading the bits out in order, and clearing them on the way, leaves the
	// bitset ready for the next query without a separate pass over it.
	for word := lo; word <= hi; word++ {
		m := mark[word]
		if m == 0 {
			continue
		}
		mark[word] = 0
		for m != 0 {
			dst = append(dst, word<<6+bits.TrailingZeros64(m))
			m &= m - 1 // clear the lowest set bit
		}
	}
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
// Smaller cells hand back fewer candidates per query (the block around a circle
// of radius r shrinks from 9r^2 towards 4r^2) at the price of visiting more of
// them. Measured, that trade does not pay here: halving the cell was slightly
// slower at every population tried, both in the default world and at nine times
// its size, because the cells being walked grow as the square while the
// candidates saved do not.
func gridCellSize(cfg *Config) float64 {
	// The widest question anybody asks. Since stage 13 that is the reach of
	// sight, which for a block of cells is further than the old circle: the
	// index has to be sized to the query, not to the radius the query used to
	// have, or the cells being walked stop covering it.
	// Half the width of the region sight asks for, which for the old circle is
	// exactly PerceptionRadius - the figure this used to be written as. Sight
	// is asked for as a rectangle rather than a radius (appendInBox), so it is
	// the width of that rectangle the cell has to be sized against, and stage
	// 13 made it wider than the circle was.
	// Zero and nonsense are passed straight through: newSpatialGrid turns them
	// into a single bucket, which is the linear scan this replaces - slower,
	// never wrong. Clamping to something tiny here instead would ask it for a
	// grid of billions of cells.
	w := World{cfg: *cfg}
	minX, _, maxX, _ := w.SightBlock(0, 0)
	return (maxX - minX) / 2
}

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
