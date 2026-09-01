package engine

import (
	"math"
	"slices"
	"testing"
)

// bruteAgentsNear is the scan the grid replaces: every agent within radius, in
// the order they sit in the slice.
func bruteAgentsNear(w *World, x, y, radius float64) []int {
	var out []int
	r2 := radius * radius
	for i := range w.agents {
		if dist2(x, y, w.agents[i].X, w.agents[i].Y) <= r2 {
			out = append(out, i)
		}
	}
	return out
}

// gridAgentsNear is the same answer reached through the index: narrow to the
// cells, then run the same circular test.
func gridAgentsNear(w *World, x, y, radius float64) []int {
	var out []int
	r2 := radius * radius
	for _, i := range w.spatialIndex().appendAgentsNear(nil, x, y, radius) {
		if dist2(x, y, w.agents[i].X, w.agents[i].Y) <= r2 {
			out = append(out, i)
		}
	}
	return out
}

func bruteFoodsNear(w *World, x, y, radius float64) []int {
	var out []int
	r2 := radius * radius
	for i := range w.foods {
		if dist2(x, y, w.foods[i].X, w.foods[i].Y) <= r2 {
			out = append(out, i)
		}
	}
	return out
}

func gridFoodsNear(w *World, x, y, radius float64) []int {
	var out []int
	r2 := radius * radius
	for _, i := range w.spatialIndex().appendFoodsNear(nil, x, y, radius) {
		if dist2(x, y, w.foods[i].X, w.foods[i].Y) <= r2 {
			out = append(out, i)
		}
	}
	return out
}

// sameIndexAsAFreshBuild reports whether what the index claims matches what a
// grid built from the world right now would hold, bucket for bucket.
func sameIndexAsAFreshBuild(w *World) bool {
	fresh := newSpatialGrid(w.cfg.Width, w.cfg.Height, gridCellSize(&w.cfg))
	fresh.rebuild(w.agents, w.foods)
	g := w.grid
	if g.cols != fresh.cols || g.rows != fresh.rows {
		return false
	}
	for c := range fresh.agents {
		if !slices.Equal(g.agents[c], fresh.agents[c]) || !slices.Equal(g.foods[c], fresh.foods[c]) {
			return false
		}
	}
	return true
}

func TestGridPutsEveryoneInTheCellTheyStandIn(t *testing.T) {
	cfg := testConfig()
	cfg.PerceptionRadius = 100 // a 400x400 world in cells of 100: 4 by 4
	w := NewWorld(cfg)
	w.addAgent(Agent{X: 10, Y: 10, Vitality: 100})   // cell (0,0)
	w.addAgent(Agent{X: 250, Y: 10, Vitality: 100})  // cell (2,0)
	w.addAgent(Agent{X: 250, Y: 350, Vitality: 100}) // cell (2,3)
	w.addFood(90, 199)                               // cell (0,1)

	g := w.spatialIndex()
	if g.cols != 4 || g.rows != 4 {
		t.Fatalf("grid is %dx%d, want 4x4", g.cols, g.rows)
	}
	want := map[int][]int{
		g.cellIndex(10, 10):   {0},
		g.cellIndex(250, 10):  {1},
		g.cellIndex(250, 350): {2}}
	for cell, ids := range want {
		if !slices.Equal(g.agents[cell], ids) {
			t.Fatalf("cell %d holds agents %v, want %v", cell, g.agents[cell], ids)
		}
	}
	if !slices.Equal(g.foods[g.cellIndex(90, 199)], []int{0}) {
		t.Fatalf("the food item is not in its own cell: %v", g.foods)
	}
	// And nothing is anywhere else.
	total := 0
	for c := range g.agents {
		total += len(g.agents[c]) + len(g.foods[c])
	}
	if total != 4 {
		t.Fatalf("%d entries across the grid, want 4", total)
	}
}

// The point of the whole exercise: asking the index gives exactly the answer
// the linear scan gave, in exactly the same order.
func TestGridQueryMatchesABruteForceScan(t *testing.T) {
	cfg := testConfig()
	cfg.InitialPopulation = 120
	cfg.InitialFoodItems = 80
	w := NewWorld(cfg)
	for i := 0; i < 120; i++ {
		w.addAgent(w.randomAgent(SpeciesHuman))
	}
	for i := 0; i < 80; i++ {
		w.addFood(w.randRange(0, cfg.Width), w.randRange(0, cfg.Height))
	}

	radii := []float64{0, 5, cfg.GrabRadius, cfg.CombatRadius, cfg.PerceptionRadius, 2 * cfg.PerceptionRadius}
	for _, r := range radii {
		for i := range w.agents {
			x, y := w.agents[i].X, w.agents[i].Y
			if got, want := gridAgentsNear(w, x, y, r), bruteAgentsNear(w, x, y, r); !slices.Equal(got, want) {
				t.Fatalf("agents within %.0f of agent %d: %v, want %v", r, i, got, want)
			}
			if got, want := gridFoodsNear(w, x, y, r), bruteFoodsNear(w, x, y, r); !slices.Equal(got, want) {
				t.Fatalf("food within %.0f of agent %d: %v, want %v", r, i, got, want)
			}
		}
	}
}

// Ascending order is not cosmetic: perceiving a neighbour draws from the
// world's random source, so a run only stays reproducible while the neighbours
// are visited in the order the linear scan visited them.
func TestGridQueryReturnsAscendingIndices(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	// Placed back to front, so that cell order and index order disagree.
	for i := 9; i >= 0; i-- {
		w.addAgent(Agent{X: 20 + float64(i)*35, Y: 200, Vitality: 100})
	}

	got := w.spatialIndex().appendAgentsNear(nil, 200, 200, cfg.PerceptionRadius)
	if !slices.IsSorted(got) {
		t.Fatalf("candidates came back as %v, want ascending", got)
	}
	if len(got) < 2 {
		t.Fatalf("only %d candidates: the query is not exercising several cells", len(got))
	}
}

// A point outside the world is clamped into the nearest edge cell rather than
// dropped, so that nothing can vanish from every query at once.
func TestGridClampsPointsOutsideTheWorld(t *testing.T) {
	cfg := testConfig()
	cfg.PerceptionRadius = 100
	w := NewWorld(cfg)
	w.addAgent(Agent{X: -50, Y: -50, Vitality: 100})
	w.addAgent(Agent{X: cfg.Width + 500, Y: cfg.Height + 500, Vitality: 100})

	g := w.spatialIndex()
	if !slices.Equal(g.agents[g.cellIndex(0, 0)], []int{0}) {
		t.Fatal("the agent outside the near corner is not in the first cell")
	}
	last := g.cols*g.rows - 1
	if !slices.Equal(g.agents[last], []int{1}) {
		t.Fatal("the agent outside the far corner is not in the last cell")
	}
	if got := gridAgentsNear(w, -50, -50, 10); !slices.Equal(got, []int{0}) {
		t.Fatalf("querying from outside the world found %v, want [0]", got)
	}
}

// Every way the world can move on has to say so, or a query later in the tick
// answers from a picture that no longer holds.
func TestEverythingThatMovesTheWorldInvalidatesTheIndex(t *testing.T) {
	cases := []struct {
		name string
		do   func(w *World)
	}{
		// Far enough to cross into the next cell, so that failing to say so
		// cannot be excused by nothing having changed.
		{"moving", func(w *World) {
			for i := 0; i < 60; i++ {
				w.moveDir(&w.agents[0], 1, 0, 1)
			}
		}},
		{"bouncing off the boundary", func(w *World) {
			w.agents[0].X = -100
			w.grid.rebuild(w.agents, w.foods) // the write above is not the engine's
			w.keepInBounds(&w.agents[0])
		}},
		{"a birth", func(w *World) { w.addAgent(Agent{X: 50, Y: 50, Vitality: 100}) }},
		{"a death", func(w *World) { w.kill(&w.agents[0]); w.removeDead() }},
		{"food appearing", func(w *World) { w.addFood(60, 60) }},
		{"food being eaten", func(w *World) { w.removeFoodByID(w.foods[0].ID) }}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := NewWorld(testConfig())
			w.addAgent(Agent{X: 100, Y: 100, Vitality: 100, Genome: genomeOf(50, 0, 0)})
			w.addAgent(Agent{X: 140, Y: 100, Vitality: 100, Genome: genomeOf(50, 0, 0)})
			w.addFood(120, 120)
			w.spatialIndex() // up to date

			c.do(w)
			if w.gridStale {
				return
			}
			// Not invalidating is only safe if nothing actually moved.
			if !sameIndexAsAFreshBuild(w) {
				t.Fatalf("%s changed the world without invalidating the index", c.name)
			}
		})
	}
}

// The same thing from the other end: run the default world and check that what
// the index hands back always matches a grid built from the world right then.
func TestGridStaysTrueThroughARun(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 3
	w := NewWorld(cfg)
	for i := 0; i < 300; i++ {
		w.Step()
		w.spatialIndex()
		if !sameIndexAsAFreshBuild(w) {
			t.Fatalf("the index disagreed with the world at tick %d", w.Tick())
		}
		// And the answers it gives still match the scan it replaced.
		if len(w.agents) > 0 {
			a := &w.agents[w.Tick()%len(w.agents)]
			got := gridAgentsNear(w, a.X, a.Y, cfg.PerceptionRadius)
			want := bruteAgentsNear(w, a.X, a.Y, cfg.PerceptionRadius)
			if !slices.Equal(got, want) {
				t.Fatalf("tick %d: neighbours of agent %d were %v, want %v", w.Tick(), a.ID, got, want)
			}
		}
	}
}

// A world with no perception radius still has to work; it just stops being an
// index. Same for one whose radius would ask for millions of cells.
func TestGridSurvivesAbsurdCellSizes(t *testing.T) {
	cfg := testConfig()
	cfg.PerceptionRadius = 0
	w := NewWorld(cfg)
	w.addAgent(Agent{X: 10, Y: 10, Vitality: 100})
	w.addAgent(Agent{X: 390, Y: 390, Vitality: 100})

	g := w.spatialIndex()
	if g.cols != 1 || g.rows != 1 {
		t.Fatalf("grid is %dx%d, want a single cell", g.cols, g.rows)
	}
	if got := gridAgentsNear(w, 10, 10, 1000); !slices.Equal(got, []int{0, 1}) {
		t.Fatalf("single cell query found %v, want both agents", got)
	}

	tiny := newSpatialGrid(100000, 100000, 0.5)
	if n := tiny.cols * tiny.rows; n > maxGridCells {
		t.Fatalf("grid asked for %d cells, more than the cap of %d", n, maxGridCells)
	}
	if tiny.cell <= 0.5 {
		t.Fatal("the cell size should have grown to stay inside the cap")
	}
}

// The index must not change what the world does. Same seed, same run.
func TestIndexingLeavesTheRunUnchanged(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 11

	plain := NewWorld(cfg)
	indexed := NewWorld(cfg)
	for i := 0; i < 500; i++ {
		plain.Step()
		indexed.Step()
		indexed.spatialIndex() // querying is what a caller will do
	}

	a, b := plain.Stats(), indexed.Stats()
	if a != b {
		t.Fatalf("stats diverged:\n plain   %+v\n indexed %+v", a, b)
	}
	for i := range plain.agents {
		p, q := &plain.agents[i], &indexed.agents[i]
		if p.ID != q.ID || math.Abs(p.X-q.X) > 0 || math.Abs(p.Y-q.Y) > 0 {
			t.Fatalf("agent %d moved differently: %v,%v vs %v,%v", p.ID, p.X, p.Y, q.X, q.Y)
		}
	}
}
