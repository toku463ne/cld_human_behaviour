package engine

import (
	"math"
	"testing"
)

// soakWorld is a still world with a river down the middle that takes something
// out of whoever is in it.
func soakWorld(t *testing.T, drain float64) *World {
	t.Helper()
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	cfg.TerrainMap = []string{
		"...~~...",
		"...~~...",
		"...~~...",
	}
	cfg.WaterDrain = drain
	return NewWorld(cfg)
}

// The default is every world measured before this stage: the water is a toll
// on movement and nothing at all to a body standing in it.
func TestTheWaterTakesNothingByDefault(t *testing.T) {
	if got := DefaultConfig().WaterDrain; got != 0 {
		t.Fatalf("a tick in the water takes %v by default, want 0", got)
	}
	w := soakWorld(t, 0)
	a := mustAgent(t, w, swimmer(t, w, 400, 300, 0))
	if got := w.soakOf(a); got != 0 {
		t.Fatalf("the rule is off and the water takes %v", got)
	}
	before := a.Vitality
	w.metabolise()
	if a.Vitality != before {
		t.Fatalf("a still body in a still world lost %v", before-a.Vitality)
	}
}

// With the rule on, standing in the water costs - which is the whole point:
// the movement toll is only charged on a tick the body actually moved.
func TestStandingInTheWaterCosts(t *testing.T) {
	w := soakWorld(t, 0.05)
	wetID, dryID := swimmer(t, w, 400, 300, 0), swimmer(t, w, 100, 300, 0)
	wet, dry := mustAgent(t, w, wetID), mustAgent(t, w, dryID)
	wetWas, dryWas := wet.Vitality, dry.Vitality
	w.metabolise()

	if got, want := wetWas-wet.Vitality, 0.05; math.Abs(got-want) > 1e-9 {
		t.Fatalf("a tick standing in the river took %v, want %v", got, want)
	}
	if dry.Vitality != dryWas {
		t.Fatalf("a tick standing on the bank took %v", dryWas-dry.Vitality)
	}
}

// A creature of the water is not drained by the water (stage 63): the same
// flag that keeps it from drowning and from being dragged.
func TestACreatureOfTheWaterIsNotDrained(t *testing.T) {
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	cfg.TerrainMap = []string{"...~~...", "...~~...", "...~~..."}
	cfg.WaterDrain = 0.05
	cfg.EnemyKinds = []EnemyKind{{Name: "lurker", Share: 1, Water: true}}
	w := NewWorld(cfg)
	beast := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 400, Y: 300,
		Vitality: 90, Species: SpeciesEnemy, Genome: genomeOf(50, 50, 50)}))
	if got := w.soakOf(beast); got != 0 {
		t.Fatalf("the river drains what lives in it by %v", got)
	}
}

// The body reckons with it: the figure is in its own view of itself and it
// rides into the lookahead exactly as the cold does. The control leaves the
// water taking just as much and the body unable to feel it.
func TestABodyFeelsWhatTheWaterIsTaking(t *testing.T) {
	w := soakWorld(t, 0.05)
	a := mustAgent(t, w, swimmer(t, w, 400, 300, 0))

	self := w.perceive(a).Self
	if math.Abs(self.Soak-0.05) > 1e-12 {
		t.Fatalf("it feels %v of the water, want 0.05", self.Soak)
	}
	if got, want := placeDrain(&self), self.Chill+self.Soak; got != want {
		t.Fatalf("the place costs %v per tick, want %v", got, want)
	}

	w.cfg.WaterDrainKnown = false
	if got := w.perceive(a).Self.Soak; got != 0 {
		t.Fatalf("with the feeling off it still reads %v", got)
	}
	if got := w.soakOf(a); got <= 0 {
		t.Fatal("switching off the feeling stopped the water taking anything")
	}
}

// What the stage is for: a place that takes something makes RESTING there
// worth less, while the other options only see the same common shift. This is
// the one thing that tells the options apart, so it is fixed here.
func TestTheDrainMakesRestingHereWorthLess(t *testing.T) {
	restValue := func(drain float64) float64 {
		cfg := testConfig()
		cfg.Width, cfg.Height = 800, 600
		cfg.TerrainMap = []string{"...~~...", "...~~...", "...~~..."}
		cfg.WaterDrain = drain
		w := NewWorld(cfg)
		a := mustAgent(t, w, swimmer(t, w, 400, 300, 0))
		a.Vitality, a.Hunger = a.MaxVitality(&w.cfg)*0.5, 0
		w.TrackDecisions(a.ID, true)
		var c AIController
		p := w.perceive(a)
		p.Trace = &DecisionTrace{}
		c.Decide(p)
		for _, o := range p.Trace.Options {
			if o.Action.Kind == ActRest {
				return o.Utility.Total()
			}
		}
		t.Fatal("resting was never scored")
		return 0
	}
	dry, wet := restValue(0), restValue(0.05)
	if !(wet < dry) {
		t.Fatalf("resting in a draining river scores %v and in a free one %v: want less", wet, dry)
	}
}

// The counters are counters: asking the world about them changes nothing.
func TestAskingAboutTheSoakChangesNothing(t *testing.T) {
	run := func(ask bool) Stats {
		cfg := quietConfig()
		cfg.Seed = 7
		cfg.TerrainMap = []string{"...~~...", "...~~...", "...~~..."}
		cfg.WaterDrain = 0.05
		w := NewWorld(cfg)
		for i := 0; i < 500; i++ {
			w.Step()
			if ask {
				w.Drowning()
			}
		}
		return w.Stats()
	}
	if quiet, asked := run(false), run(true); quiet != asked {
		t.Fatalf("asking changed the world: %+v vs %+v", quiet, asked)
	}
}
