package engine

import "testing"

// populate replaces the population with counts[s] agents of species s, and
// moves the world's clock to tick. The census only counts labels, so the
// agents themselves need nothing beyond being alive.
func populate(w *World, tick int, counts ...int) {
	w.tick = tick
	w.agents = w.agents[:0]
	w.index = make(map[int]int, 8)
	for s, n := range counts {
		for i := 0; i < n; i++ {
			w.addAgent(Agent{X: 10, Y: 10, Vitality: 100, Genome: genomeOf(50, 0, 0), Species: Species(s)})
		}
	}
}

func speciesEntry(t *testing.T, c Census, s Species) SpeciesCensus {
	t.Helper()
	for _, e := range c.Species {
		if e.Species == s {
			return e
		}
	}
	t.Fatalf("census has no entry for %v", s)
	return SpeciesCensus{}
}

func TestCensusCountsEachSpeciesSeparately(t *testing.T) {
	w := NewWorld(testConfig())
	c := NewCensusTracker(1000)
	for tick := 0; tick <= 100; tick += 25 {
		populate(w, tick, 3, 2)
		c.Observe(w)
	}

	got := c.Result()
	if len(got.Species) != 2 {
		t.Fatalf("species counted = %d, want 2", len(got.Species))
	}
	approx(t, got.Population, 5, 1e-9, "mean total population")
	approx(t, speciesEntry(t, got, 0).Mean, 3, 1e-9, "mean of species 0")
	approx(t, speciesEntry(t, got, 1).Mean, 2, 1e-9, "mean of species 1")
	approx(t, speciesEntry(t, got, 0).Share, 0.6, 1e-9, "share of species 0")
	approx(t, speciesEntry(t, got, 1).Share, 0.4, 1e-9, "share of species 1")
	if got.Living() != 2 {
		t.Fatalf("living species = %d, want 2", got.Living())
	}
}

// A label nobody has ever worn is not a species: an enemy that was never
// spawned must not turn up as one that has died out.
func TestCensusIgnoresSpeciesThatNeverAppeared(t *testing.T) {
	w := NewWorld(testConfig())
	c := NewCensusTracker(1000)
	populate(w, 0, 4, 0, 0)
	c.Observe(w)

	got := c.Result()
	if len(got.Species) != 1 {
		t.Fatalf("species counted = %d, want 1: only species 0 ever existed", len(got.Species))
	}
}

// The window is what makes this a measure of coexistence rather than of one
// frame: readings that have fallen out of it stop counting.
func TestCensusWindowForgetsOlderReadings(t *testing.T) {
	w := NewWorld(testConfig())
	c := NewCensusTracker(100)
	for tick := 0; tick <= 100; tick += 25 {
		populate(w, tick, 10)
		c.Observe(w)
	}
	// Five readings of 10, then five of 20 spread over the next 125 ticks, by
	// which point the window holds only the latter.
	for tick := 125; tick <= 225; tick += 25 {
		populate(w, tick, 20)
		c.Observe(w)
	}

	got := c.Result()
	if got.Window != 100 {
		t.Fatalf("window = %d ticks, want 100", got.Window)
	}
	if got.Samples != 5 {
		t.Fatalf("samples = %d, want 5", got.Samples)
	}
	approx(t, speciesEntry(t, got, 0).Mean, 20, 1e-9, "mean once the old readings have aged out")
}

// The point of the trough. Two runs with the same mean, one steady and one
// swinging down to a handful, are the same number to a mean and different
// worlds to criterion A, because only one of them is a bad seed away from
// having no species left.
func TestCensusTroughSeparatesASteadyPopulationFromASwingingOne(t *testing.T) {
	steady := NewCensusTracker(1000)
	swinging := NewCensusTracker(1000)
	w := NewWorld(testConfig())

	swing := []int{70, 130, 70, 10, 70}
	for i, n := range swing {
		tick := i * 25
		populate(w, tick, 70)
		steady.Observe(w)
		populate(w, tick, n)
		swinging.Observe(w)
	}

	a := speciesEntry(t, steady.Result(), 0)
	b := speciesEntry(t, swinging.Result(), 0)
	approx(t, a.Mean, b.Mean, 1e-9, "the two runs have the same mean")

	approx(t, a.Trough, 1, 1e-9, "trough of a flat population")
	approx(t, a.Swing, 0, 1e-9, "swing of a flat population")
	approx(t, b.Trough, 10.0/70.0, 1e-9, "trough of the swinging population")
	approx(t, b.Swing, 120.0/70.0, 1e-9, "swing of the swinging population")
}

// Extinction is remembered for the whole run, not read off the window: a
// species that died early leaves nothing in a window that has moved past it,
// and its absence must not read as a shorter table.
func TestCensusRemembersASpeciesThatDiedBeforeTheWindow(t *testing.T) {
	w := NewWorld(testConfig())
	c := NewCensusTracker(100)
	populate(w, 0, 40, 5)
	c.Observe(w)
	populate(w, 25, 40, 0)
	c.Observe(w)
	for tick := 50; tick <= 400; tick += 25 {
		populate(w, tick, 40, 0)
		c.Observe(w)
	}

	got := c.Result()
	if len(got.Species) != 2 {
		t.Fatalf("species counted = %d, want 2: the lost one keeps its entry", len(got.Species))
	}
	lost := speciesEntry(t, got, 1)
	if !lost.Extinct {
		t.Fatal("species 1 is at zero and was alive: want Extinct")
	}
	if lost.ExtinctTick != 25 {
		t.Fatalf("extinct at tick %d, want 25", lost.ExtinctTick)
	}
	if got.Living() != 1 {
		t.Fatalf("living species = %d, want 1", got.Living())
	}
	if speciesEntry(t, got, 0).ExtinctTick != -1 {
		t.Fatal("species 0 never emptied, so it has no extinction tick")
	}

	// And it is the one Rarest reports: coexistence is decided by the species
	// that is doing worst, not by the healthy one next to it.
	rarest, ok := got.Rarest()
	if !ok || rarest.Species != 1 {
		t.Fatalf("rarest = %v (ok=%v), want species 1", rarest.Species, ok)
	}
	if rarest.Trough != 0 || rarest.Share != 0 {
		t.Fatalf("rarest trough/share = %.2f/%.2f, want 0/0", rarest.Trough, rarest.Share)
	}
}

func TestCensusRarestIsTheSmallestPopulation(t *testing.T) {
	w := NewWorld(testConfig())
	c := NewCensusTracker(1000)
	populate(w, 0, 30, 4, 12)
	c.Observe(w)

	rarest, ok := c.Result().Rarest()
	if !ok {
		t.Fatal("three species were counted, so there is a rarest")
	}
	if rarest.Species != 1 {
		t.Fatalf("rarest = %v, want species 1", rarest.Species)
	}
	approx(t, rarest.Share, 4.0/46.0, 1e-9, "share of the rarest species")
}

func TestCensusOfAWorldWithNobodyInIt(t *testing.T) {
	w := NewWorld(testConfig()) // no founders of either species
	c := NewCensusTracker(1000)
	c.Observe(w)

	got := c.Result()
	if len(got.Species) != 0 {
		t.Fatalf("species counted = %d, want 0", len(got.Species))
	}
	if _, ok := got.Rarest(); ok {
		t.Fatal("an empty world has no rarest species")
	}
	if got.Population != 0 || got.Living() != 0 {
		t.Fatalf("population/living = %.2f/%d, want 0/0", got.Population, got.Living())
	}
}

func TestCensusWithNoReadings(t *testing.T) {
	got := NewCensusTracker(1000).Result()
	if got.Samples != 0 || len(got.Species) != 0 {
		t.Fatalf("samples/species = %d/%d, want 0/0", got.Samples, len(got.Species))
	}
}

// The default world holds humans and enemies, and the census is what says
// whether both are still there. Criterion A is asked of this table.
func TestCensusOfTheDefaultWorldHoldsBothSpecies(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 7
	w := NewWorld(cfg)
	c := NewCensusTracker(DefaultCensusWindow)
	for i := 0; i < 4000; i++ {
		w.Step()
		if w.Tick()%DefaultCensusStep == 0 {
			c.Observe(w)
		}
	}

	got := c.Result()
	if len(got.Species) != 2 {
		t.Fatalf("species = %v, want humans and enemies", got.Species)
	}
	for _, e := range got.Species {
		if e.Extinct || e.Min == 0 {
			t.Fatalf("%v died out inside %d ticks: min %d over the window", e.Species, 4000, e.Min)
		}
		if e.Mean <= 0 || e.Trough <= 0 || e.Trough > 1 {
			t.Fatalf("%v: mean %.1f trough %.2f, want a positive mean and a trough in (0, 1]",
				e.Species, e.Mean, e.Trough)
		}
	}
	if h := speciesEntry(t, got, SpeciesHuman); h.Share < 0.5 {
		t.Fatalf("humans are %.2f of the population: the enemies have taken over", h.Share)
	}
}
