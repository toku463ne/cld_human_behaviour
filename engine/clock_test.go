package engine

import (
	"math"
	"testing"
)

// The arm this stage is measured against: no clock, and the world is the one
// from before it, down to the values drawn from the random source.
func TestNoClockLeavesTheWorldExactlyAsItWas(t *testing.T) {
	run := func(ticksPerDay int) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 21
		cfg.TicksPerDay = ticksPerDay
		w := NewWorld(cfg)
		for i := 0; i < 1500; i++ {
			w.Step()
		}
		return w.Stats()
	}
	if off, again := run(0), run(0); off != again {
		t.Fatal("the same world twice gave different runs")
	} else if on := run(1000); on == off {
		t.Fatal("giving the world a day changed nothing at all")
	}
}

// The day is a circle. An hour just before midnight is next to one just after,
// not most of a day away from it, and a chronotype that did not know that would
// have a seam in it.
func TestTheDayIsACircle(t *testing.T) {
	cfg := quietConfig()
	cfg.TicksPerDay = 1000
	cfg.RestPhaseDepth = 0.5
	w := NewWorld(cfg)

	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 50, Y: 50, Vitality: 50, Genome: genomeOf(50, 50, 50)}))
	a.chronotype = 0.98

	// Just after midnight suits an agent whose hour is just before it.
	w.tick = 20 // phase 0.02
	near := w.restFit(a)
	// And the far side of the day does not.
	w.tick = 480 // phase 0.48
	far := w.restFit(a)

	if near <= 0 || far >= 0 {
		t.Fatalf("an hour four hundredths away fits %v and one half a day away fits %v, want the first positive and the second negative",
			near, far)
	}
	if math.Abs(near-1) > 0.05 {
		t.Fatalf("its own hour fits %v, want nearly 1", near)
	}
}

// Recovery follows the hour, and never stops. A way back from exhaustion is the
// one rule the world cannot do without, so an agent kept up by the clock mends
// less well rather than not at all.
func TestRecoveryFollowsTheHourAndNeverStops(t *testing.T) {
	cfg := quietConfig()
	cfg.TicksPerDay = 1000
	cfg.RegenRate = 0.09
	cfg.RestPhaseDepth = 0.5
	w := NewWorld(cfg)

	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 50, Y: 50, Vitality: 50, Genome: genomeOf(50, 50, 50)}))
	a.chronotype = 0

	w.tick = 0
	best := w.restRate(a)
	w.tick = 500
	worst := w.restRate(a)

	if math.Abs(best-cfg.RegenRate*1.5) > 1e-9 {
		t.Fatalf("at its own hour it mends %v, want %v", best, cfg.RegenRate*1.5)
	}
	if math.Abs(worst-cfg.RegenRate*0.5) > 1e-9 {
		t.Fatalf("at its worst hour it mends %v, want %v", worst, cfg.RegenRate*0.5)
	}
	if worst <= 0 {
		t.Fatal("an agent at its worst hour cannot recover at all; the world has lost its way back")
	}
}

// A chronotype is a phase, not a quantity, so it wraps rather than piles up at
// the ends - and it is not one of the budget genes, so having one costs
// nothing.
func TestAChronotypeWrapsAndCostsNothing(t *testing.T) {
	cfg := quietConfig()
	cfg.TicksPerDay = 1000
	cfg.ChronotypeSpread = 1
	cfg.ChronotypeMutation = 0.5 // large on purpose, to run past both ends
	w := NewWorld(cfg)

	pa := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 10, Y: 10, Vitality: 80, Genome: genomeOf(50, 50, 50)}))
	pb := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 12, Y: 10, Vitality: 80, Genome: genomeOf(50, 50, 50)}))
	pa.chronotype, pb.chronotype = 0.02, 0.98

	low, high := 0, 0
	for i := 0; i < 4000; i++ {
		c := w.inheritChronotype(pa, pb)
		if c < 0 || c >= 1 {
			t.Fatalf("a child keeps hour %v, want it wrapped into 0..1", c)
		}
		if c < 0.1 {
			low++
		}
		if c > 0.9 {
			high++
		}
	}
	// Mutations that run past midnight come out the other side rather than
	// piling up on it.
	if low == 0 || high == 0 {
		t.Fatalf("children landed %d below 0.1 and %d above 0.9; the circle has a seam", low, high)
	}

	// And the budget is untouched: a chronotype is a direction, not an amount.
	budget := pa.Budget()
	pa.chronotype = 0.5
	if pa.Budget() != budget {
		t.Fatal("keeping different hours changed what the body is made of")
	}
}

// The point of the whole stage: a friend can only keep watch while it is awake.
// It is a condition on a term that already existed, and it is what turns
// differing hours into somebody watching without a watchman in the rules.
func TestASleepingFriendCannotKeepWatch(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)

	exposure := func(resting bool) float64 {
		p := &Perception{
			Tick: 1, Cfg: &cfg, Rand: w.rng,
			Self: SelfView{
				ID: 1, X: 200, Y: 200, Vitality: 40, Hunger: 10,
				MaxVitality: cfg.MaxVitality, MaxSpeed: cfg.MaxSpeed,
				HungerRate: cfg.HungerRate, RestRate: cfg.RegenRate},
			Others: []AgentView{{ID: 2, Dist: 20, EstStrength: 70,
				Affinity: cfg.AffinityTrust, Resting: resting}},
		}
		c := &AIController{}
		c.survey(p)
		return c.exposure
	}

	watching, asleep := exposure(false), exposure(true)
	if watching >= asleep {
		t.Fatalf("a waking friend left %v of exposure and a sleeping one %v, want the waking one to cover more",
			watching, asleep)
	}
	if watching != 0 {
		t.Fatalf("a wholly trusted friend, awake, left %v of exposure, want none", watching)
	}
}

// The measurement has to report honestly on a world it cannot act in: with
// everybody on the same clock there is no spread, and any share of
// all-asleep moments is about something else.
func TestVigilanceReportsTheSpreadItIsBasedOn(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 22
	cfg.ChronotypeSpread = 0
	w := NewWorld(cfg)
	for i := 0; i < 1000; i++ {
		w.Step()
	}
	if got := w.Vigilance(DefaultClusterLinkDist).Spread; got != 0 {
		t.Fatalf("a population all on one clock has a spread of %v, want 0", got)
	}

	cfg.ChronotypeSpread = 1
	w2 := NewWorld(cfg)
	for i := 0; i < 1000; i++ {
		w2.Step()
	}
	if got := w2.Vigilance(DefaultClusterLinkDist).Spread; got < 0.5 {
		t.Fatalf("a population scattered over the day has a spread of %v, want it well above 0", got)
	}
}
