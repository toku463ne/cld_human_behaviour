package engine

import (
	"math"
	"testing"
)

// The arm this stage is measured against: with no learning the world is the
// one stage 15a left, down to the values taken from the random source.
func TestNoRegionLearningLeavesTheWorldExactlyAsItWas(t *testing.T) {
	run := func(rate float64) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 6
		cfg.RegionLearnRate = rate
		w := NewWorld(cfg)
		for i := 0; i < 1500; i++ {
			w.Step()
		}
		return w.Stats()
	}
	if off, again := run(0), run(0); off != again {
		t.Fatal("the same world twice gave different runs")
	} else if on := run(1); on == off {
		t.Fatal("letting agents learn the ground changed nothing at all")
	}
}

// An agent forms a view of the ground it is standing on, and of no other. That
// is what makes being told about somewhere worth anything.
func TestAnAgentOnlyLearnsTheGroundItStandsOn(t *testing.T) {
	cfg := quietConfig()
	cfg.RegionNoise = 0
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 50, Y: 50, Vitality: 80, Genome: genomeOf(50, 50, 50)}))

	here := w.regionIndexAt(a.X, a.Y)
	for i := 0; i < 20; i++ {
		w.noteRegion(a, 5)
	}
	seen, known := w.regionEstimate(a, here)
	if !known {
		t.Fatal("stood somewhere twenty times and knows nothing about it")
	}
	if math.Abs(seen-5) > 0.5 {
		t.Fatalf("reckons it saw %v food there, want about 5", seen)
	}
	// Everywhere else is still unknown - not "believed empty".
	for r := range w.regions {
		if r == here {
			continue
		}
		if _, known := w.regionEstimate(a, r); known {
			t.Fatalf("has a view of region %d without ever going there", r)
		}
	}
	if _, _, ok := w.bestKnownRegion(a); ok {
		t.Fatal("knows somewhere better than here, having been nowhere else")
	}
}

// Somewhere you have never been is somewhere you can only hear about, and
// hearing it is a starting point rather than a lifetime of looking.
func TestBeingToldAboutSomewhereYouHaveNeverBeen(t *testing.T) {
	cfg := quietConfig()
	cfg.RegionNoise = 0
	w := NewWorld(cfg)

	traveller := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 50, Y: 50, Vitality: 80, Genome: genomeOf(50, 50, 50)}))
	stayer := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 55, Y: 50, Vitality: 80, Genome: genomeOf(50, 50, 50)}))

	// The traveller has been somewhere good, far from here.
	far := len(w.regions) - 1
	traveller.regions = make([]regionView, len(w.regions))
	traveller.regions[far].setSeen(9, 30, w.tick)

	if _, known := w.regionEstimate(stayer, far); known {
		t.Fatal("knew about it before being told")
	}
	w.exchangeRegions(stayer, traveller)

	seen, known := w.regionEstimate(stayer, far)
	if !known {
		t.Fatal("was told about somewhere and took nothing from it")
	}
	if math.Abs(seen-9) > 1e-9 {
		t.Fatalf("was told 9 and came away with %v", seen)
	}
	// But holds it lightly: one look of its own should nearly overturn it.
	if n := stayer.regions[far].n; n > cfg.RegionToldCount {
		t.Fatalf("a handed-down view is worth %v looks, want no more than %v", n, cfg.RegionToldCount)
	}
	// Where both have been, they meet in the middle instead.
	both := 0
	traveller.regions[both].setSeen(8, 30, w.tick)
	stayer.regions[both].setSeen(2, 30, w.tick)
	w.exchangeRegions(stayer, traveller)
	mine, _ := w.regionEstimate(stayer, both)
	theirs, _ := w.regionEstimate(traveller, both)
	if !(mine > 2 && theirs < 8 && mine < theirs) {
		t.Fatalf("after trading they believe %v and %v, want them to have moved towards each other", mine, theirs)
	}
}

// Knowing where the good ground is only matters if it can be acted on. The
// draw is one more option in the same comparison - a direction for one step,
// not a plan - and it is worth more the better the remembered ground is.
func TestAnAgentHeadsForGroundItRemembersAsBetter(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)

	exploreOptions := func(gain, drawValue float64) []option {
		cfg := cfg
		cfg.RegionDrawValue = drawValue
		p := &Perception{
			Tick: 1, Cfg: &cfg, Rand: w.rng,
			Self: SelfView{
				ID: 1, X: 100, Y: 100, Vitality: 60, Hunger: cfg.MaxHunger * 0.8,
				MaxVitality: cfg.MaxVitality, MaxSpeed: cfg.MaxSpeed,
				HungerRate:    cfg.HungerRate,
				BetterGround:  gain,
				BetterGroundX: 300, BetterGroundY: 100,
				Retaliation: cfg.Retaliation, AcceptChance: cfg.AcceptChance,
				RiskWeight: cfg.RiskWeight, CompetitionWeight: cfg.CompetitionWeight,
				ShockRisk: cfg.ShockRisk},
		}
		c := &AIController{}
		c.addExplore(p)
		return c.opts
	}

	if got := exploreOptions(0, cfg.RegionDrawValue); len(got) != 1 {
		t.Fatalf("with nowhere better known there were %d wandering options, want just the aimless one", len(got))
	}
	if got := exploreOptions(3, 0); len(got) != 1 {
		t.Fatalf("with the draw switched off there were %d options, want just the aimless one", len(got))
	}

	opts := exploreOptions(3, cfg.RegionDrawValue)
	if len(opts) != 2 {
		t.Fatalf("knowing somewhere better gave %d options, want the aimless one and the aimed one", len(opts))
	}
	aimed := opts[1]
	if aimed.action.DX <= 0.99 {
		t.Fatalf("headed %v,%v, want due east towards the remembered ground", aimed.action.DX, aimed.action.DY)
	}
	if aimed.util <= opts[0].util {
		t.Fatalf("heading for better ground scored %v against wandering at %v", aimed.util, opts[0].util)
	}
	// And the better the ground is remembered to be, the more it is worth.
	if small, large := exploreOptions(1, cfg.RegionDrawValue)[1], exploreOptions(6, cfg.RegionDrawValue)[1]; large.util <= small.util {
		t.Fatalf("a slightly better place scored %v and a much better one %v", small.util, large.util)
	}
}

// What the memory gene buys here is how well the country is held - not how
// many people are, which is a separate organ's worth of room (#41).
func TestAGoodMemoryHoldsTheCountryLongerWithoutCrowdingOutPeople(t *testing.T) {
	cfg := quietConfig()
	cfg.RegionNoise = 0
	w := NewWorld(cfg)

	learn := func(memory float64) *Agent {
		g := genomeOf(50, 50, 50)
		g[GeneMemory] = memory
		a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 50, Y: 50, Vitality: 80, Genome: g}))
		for i := 0; i < 60; i++ {
			w.noteRegion(a, 5)
		}
		return a
	}
	poor, good := learn(10), learn(95)
	here := w.regionIndexAt(50, 50)

	// The one that spent on memory holds more looks behind the same estimate.
	if poor.regions[here].n >= good.regions[here].n {
		t.Fatalf("a poor memory holds %v looks and a good one %v", poor.regions[here].n, good.regions[here].n)
	}

	// And keeps it longer once it has walked away. Five thousand ticks is ten
	// years: long past what a poor memory holds country for, well inside what
	// a good one does.
	w.tick += 5000
	_, poorKnows := w.regionEstimate(poor, here)
	_, goodKnows := w.regionEstimate(good, here)
	if poorKnows && !goodKnows {
		t.Fatal("the poor memory outlasted the good one")
	}
	if !goodKnows {
		t.Fatal("even a good memory lost the country entirely")
	}

	// Knowing the country costs nothing in room for people.
	if len(good.opinions) != 0 {
		t.Fatalf("learning the ground took up %d of its memory of people", len(good.opinions))
	}
}
