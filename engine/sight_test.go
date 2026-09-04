package engine

import (
	"math"
	"math/rand"
	"testing"
)

// With the grid off, sight is exactly the circle it always was.
func TestSightGridOffIsTheOldCircle(t *testing.T) {
	cfg := testConfig()
	cfg.SightGrid = false
	w := NewWorld(cfg)

	r := cfg.PerceptionRadius
	for _, d := range []float64{0, r * 0.5, r - 0.01, r + 0.01, r * 2} {
		want := d <= r
		if got := w.canSee(200, 200, 200+d, 200); got != want {
			t.Fatalf("at %v away the circle says %v, want %v", d, got, want)
		}
	}
	if got := w.sightRange(); got != r {
		t.Fatalf("the circle reaches %v, want %v", got, r)
	}
}

// The block is the agent's own cell and the rings around it, and what decides
// it is which cell each of them is in - not how far apart they are.
func TestSightIsAWholeBlockOfCells(t *testing.T) {
	cfg := testConfig()
	cfg.SightGrid = true
	cfg.SightCellSize = 100
	cfg.SightCells = 1
	w := NewWorld(cfg)

	// Standing just inside the cell that runs 200..300.
	x, y := 205.0, 205.0
	cases := []struct {
		bx, by float64
		want   bool
		about  string
	}{
		{250, 250, true, "its own cell"},
		{150, 150, true, "the cell diagonally back"},
		{395, 395, true, "the far corner of the cell diagonally on"},
		{405, 205, false, "two cells along, though only 200 away"},
		{205, 405, false, "two cells up"},
		{95, 205, false, "two cells back"},
	}
	for _, c := range cases {
		if got := w.canSee(x, y, c.bx, c.by); got != c.want {
			t.Fatalf("looking at %s (%v, %v): %v, want %v", c.about, c.bx, c.by, got, c.want)
		}
	}
}

// The whole point of a block: it does not see the same distance in every
// direction, and how far it sees depends on where in its cell the agent
// happens to be standing. Both of those are false of a circle.
func TestSightIsNotTheSameInEveryDirection(t *testing.T) {
	cfg := testConfig()
	cfg.SightGrid = true
	cfg.SightCellSize = 100
	cfg.SightCells = 1
	w := NewWorld(cfg)

	// An agent at the very corner of its cell.
	x, y := 201.0, 201.0

	// It can see a long way into the corner it backs onto...
	if !w.canSee(x, y, 105, 105) {
		t.Fatal("cannot see something 136 away in the corner it backs onto")
	}
	// ... and not nearly as far the other way, though that is nearer.
	if w.canSee(x, y, 405, 201) {
		t.Fatal("can see something 204 away straight ahead, which is two cells off")
	}

	// And two agents the same distance apart get different answers depending
	// on where in their cells they stand.
	const d = 150
	atCorner := w.canSee(299, 299, 299+d, 299) // about to cross into the next cell
	atStart := w.canSee(201, 201, 201+d, 201)  // just entered this one
	if atCorner == atStart {
		t.Fatal("where in its cell an agent stands made no difference; the block is behaving like a circle")
	}
}

// sightRange is what the spatial index is queried with, so it has to be a
// bound and never an estimate: anything the query misses is invisible whatever
// canSee would have said. This is the test that stops agents going silently
// blind in some directions.
func TestNothingVisibleIsFurtherAwayThanSightReaches(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	for _, cells := range []int{0, 1, 2} {
		cfg := testConfig()
		cfg.SightGrid = true
		cfg.SightCellSize = 76.8
		cfg.SightCells = cells
		w := NewWorld(cfg)
		reach := w.sightRange()

		worst := 0.0
		for i := 0; i < 200000; i++ {
			ax, ay := rng.Float64()*cfg.Width, rng.Float64()*cfg.Height
			bx, by := rng.Float64()*cfg.Width, rng.Float64()*cfg.Height
			if !w.canSee(ax, ay, bx, by) {
				continue
			}
			if d := math.Hypot(ax-bx, ay-by); d > worst {
				worst = d
			}
		}
		if worst > reach {
			t.Fatalf("with %d rings something visible was %v away but the index is only asked for %v",
				cells, worst, reach)
		}
		// And it must not be wildly loose either, or every query drags in the
		// whole world.
		if worst < reach*0.6 {
			t.Fatalf("with %d rings the furthest visible thing was %v and the index is asked for %v: too loose",
				cells, worst, reach)
		}
	}
}

// The default cell size is calibrated so that a block covers about as much
// ground as the circle it replaced. Without that, the change would be "agents
// see more" or "agents see less", and the shape - the thing being tested -
// could not be separated from it.
func TestABlockCoversAboutAsMuchGroundAsTheCircle(t *testing.T) {
	cfg := DefaultConfig()
	block := math.Pow(float64(2*cfg.SightCells+1)*cfg.SightCellSize, 2)
	circle := math.Pi * cfg.PerceptionRadius * cfg.PerceptionRadius
	if ratio := block / circle; ratio < 0.97 || ratio > 1.03 {
		t.Fatalf("a block covers %.0f and the circle %.0f, a ratio of %.3f; want them within 3%%",
			block, circle, ratio)
	}
}
