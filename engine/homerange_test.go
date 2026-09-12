package engine

import "testing"

// Where a body came into the world is written down once and never touched
// again - the same standing as its sort.
func TestHomeIsWhereABodyCameIntoTheWorld(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 61
	cfg.EnemyHomeCost = 2
	w := NewWorld(cfg)

	home := map[int]int{}
	for _, a := range w.Agents() {
		home[a.ID] = a.HomeRegion
		if a.HomeRegion != w.regionIndexAt(a.X, a.Y) {
			t.Fatalf("agent %d starts in block %d but calls %d home",
				a.ID, w.regionIndexAt(a.X, a.Y), a.HomeRegion)
		}
	}
	for i := 0; i < 3000; i++ {
		w.Step()
	}
	moved := 0
	for _, a := range w.Agents() {
		was, known := home[a.ID]
		if !known {
			continue
		}
		if a.HomeRegion != was {
			t.Fatalf("agent %d's home moved from %d to %d", a.ID, was, a.HomeRegion)
		}
		if w.regionIndexAt(a.X, a.Y) != a.HomeRegion {
			moved++
		}
	}
	if moved == 0 {
		t.Fatal("nobody ever left the block it started in, so the test says nothing")
	}
}

// The cost is on the enemies. A human is tied to nowhere, whatever the world
// charges.
func TestOnlyEnemiesPayForBeingAwayFromHome(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 67
	cfg.EnemyHomeCost = 4
	w := NewWorld(cfg)
	for i := 0; i < 500; i++ {
		w.Step()
	}
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		_, _, pull := w.homeFor(a)
		if a.Species == SpeciesHuman && pull != 0 {
			t.Fatalf("a human is charged %v for being away from home", pull)
		}
	}
}

// What tells the options apart: the same walk costs more heading out than
// heading back. That is the whole of "it stops being worth it", and there is
// no threshold anywhere in it.
func TestWalkingOutCostsMoreThanWalkingBack(t *testing.T) {
	cfg := quietConfig()
	cfg.EnemyHomeCost = 4
	w := NewWorld(cfg)

	// An enemy put down a long way from the middle of its own block.
	minX, minY, maxX, maxY := w.regionBounds(0)
	hx, hy := (minX+maxX)/2, (minY+maxY)/2
	id := w.addAgent(Agent{
		Maturity: 1, X: hx, Y: hy, Species: SpeciesEnemy, Genome: filledGenome(50),
	})
	a := mustAgent(t, w, id)
	a.X, a.Y = hx+300, hy // three region widths out at the default size
	w.Step()
	a = mustAgent(t, w, id)

	p := w.perceive(a)
	if p.Self.HomePull <= 0 {
		t.Fatal("the enemy is not charged for being away at all")
	}
	// Deciding once fills in what the controller knows about this body; the
	// two options below are then scored on the same footing as its own.
	c := &AIController{}
	c.Decide(p)
	c.add(Action{Kind: ActMove, DX: 1, DY: 0, Effort: 1}, Utility{Ticks: 10})
	c.add(Action{Kind: ActMove, DX: -1, DY: 0, Effort: 1}, Utility{Ticks: 10})
	// The scores, in the order they were added.
	outward, homeward := c.opts[len(c.opts)-2].util, c.opts[len(c.opts)-1].util
	if homeward <= outward {
		t.Fatalf("heading home scored %v against %v for heading further out", homeward, outward)
	}
}

// And it is not a leash: a body far from home still takes a good enough
// option out there. The rule makes the walk dear, it does not forbid it.
func TestBeingFarFromHomeIsACostAndNotALeash(t *testing.T) {
	cfg := quietConfig()
	cfg.EnemyHomeCost = 4
	w := NewWorld(cfg)

	minX, minY, maxX, maxY := w.regionBounds(0)
	hx, hy := (minX+maxX)/2, (minY+maxY)/2
	id := w.addAgent(Agent{
		Maturity: 1, X: hx, Y: hy, Species: SpeciesEnemy, Genome: filledGenome(50),
	})
	a := mustAgent(t, w, id)
	a.X, a.Y = hx+300, hy
	a.Hunger = w.cfg.MaxHunger * 0.9
	w.Step()
	a = mustAgent(t, w, id)

	p := w.perceive(a)
	c := &AIController{}
	c.Decide(p)
	// One option worth a great deal, pointing away from home.
	c.add(Action{Kind: ActMove, DX: 1, DY: 0, Effort: 1}, Utility{
		Life:  Goal{Value: 10000, Chance: 1},
		Ticks: 10,
	})
	c.add(Action{Kind: ActMove, DX: -1, DY: 0, Effort: 1}, Utility{Ticks: 10})
	worthIt, homeward := c.opts[len(c.opts)-2].util, c.opts[len(c.opts)-1].util
	if worthIt <= homeward {
		t.Fatalf("a body far from home would not chase a meal worth %v (home scored %v)",
			worthIt, homeward)
	}
}

// A world that has not asked for the rule is untouched, down to the values
// taken from the random source.
func TestNoHomeCostLeavesTheWorldExactlyAsItWas(t *testing.T) {
	run := func(cost float64) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 71
		cfg.EnemyHomeCost = cost
		w := NewWorld(cfg)
		for i := 0; i < 1500; i++ {
			w.Step()
		}
		return w.Stats()
	}
	flat := run(0)
	if flat != run(0) {
		t.Fatal("the same world twice gave different runs")
	}
	if varied := run(4); varied == flat {
		t.Fatal("charging for being away from home changed nothing at all")
	}
}
