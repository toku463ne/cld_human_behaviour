package engine

import (
	"math"
	"testing"
)

// Stage 67: one step of lookahead in the utility formula.

// aSatiatedWholeBody is the body the stage was written for: nothing wrong with
// it, nothing about to be. Before stage 67 it read a flat gradient, which is
// why it put no value on keeping anything for later.
func aSatiatedWholeBody(cfg *Config) SelfView {
	return SelfView{
		MaxVitality: cfg.MaxVitality,
		Vitality:    cfg.MaxVitality,
		Hunger:      cfg.SatiatedHunger,
		HungerRate:  cfg.HungerRate,
		ShockRisk:   cfg.ShockRisk,
		MaxSpeed:    cfg.MaxSpeed,
	}
}

func TestLookaheadIsOffByDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.LookaheadHorizons != 0 {
		t.Fatalf("LookaheadHorizons should default to 0, got %v", cfg.LookaheadHorizons)
	}
	s := aSatiatedWholeBody(&cfg)
	for _, v := range []float64{5, 20, 50, 100} {
		for _, h := range []float64{0, 40, 60, 90} {
			s.Vitality, s.Hunger = v, h
			got := pressure(&cfg, &s, v, h, 0)
			want := oneHorizon(&cfg, &s, v, projectedDrain(&cfg, s.HungerRate, h))
			if got != want {
				t.Fatalf("with the lookahead off, pressure must be the one window exactly: v=%v h=%v got %v want %v", v, h, got, want)
			}
		}
	}
}

// The whole point of the stage: a body with something to spare can tell the
// difference between having a meal put by and not having one.
func TestLookaheadGivesASatiatedBodyAReasonToKeepFood(t *testing.T) {
	cfg := DefaultConfig()
	s := aSatiatedWholeBody(&cfg)

	if got := keepValue(&cfg, &s, 0, 1, 0); got != 0 {
		t.Fatalf("without lookahead a whole, fed body should read a flat gradient, got %v", got)
	}
	cfg.LookaheadHorizons = 1
	if got := keepValue(&cfg, &s, 0, 1, 0); got <= 0 {
		t.Fatalf("with lookahead it should be worth something, got %v", got)
	}
}

// And it is a dial, not a gate: no threshold anywhere, which is the line the
// design draws against behavioural thresholds.
func TestLookaheadRisesWithTheDose(t *testing.T) {
	cfg := DefaultConfig()
	s := aSatiatedWholeBody(&cfg)
	last := -1.0
	for _, dose := range []float64{0, 0.25, 0.5, 1, 2} {
		cfg.LookaheadHorizons = dose
		got := keepValue(&cfg, &s, 0, 1, 0)
		if got < last {
			t.Fatalf("looking further ahead should not be worth less: dose %v gave %v after %v", dose, got, last)
		}
		last = got
	}
	if last <= 0 {
		t.Fatal("the dose sweep never got off zero")
	}
}

// The line that keeps a fight readable: what is hitting the body now belongs
// to the window it was measured in. If the second window carried it too, every
// candidate in a fight would drain the tank to nothing and score alike.
func TestLookaheadLeavesIncomingDamageInTheFirstWindow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LookaheadHorizons = 1
	s := aSatiatedWholeBody(&cfg)
	s.Vitality = 60

	// Two options that differ only in how much of the blow they take.
	heavy := pressure(&cfg, &s, s.Vitality, s.Hunger, 0.5)
	light := pressure(&cfg, &s, s.Vitality, s.Hunger, 0.2)
	if !(heavy > light) {
		t.Fatalf("taking more damage must still look worse: heavy %v light %v", heavy, light)
	}
	if heavy >= 1 || light >= 1 {
		t.Fatalf("neither should be flattened to certain death: heavy %v light %v", heavy, light)
	}

	// And the second window itself must not be reading the damage: with the
	// body's own metabolism unchanged, the gap between the two is the gap the
	// single window already had.
	cfg.LookaheadHorizons = 0
	flatHeavy := pressure(&cfg, &s, s.Vitality, s.Hunger, 0.5)
	flatLight := pressure(&cfg, &s, s.Vitality, s.Hunger, 0.2)
	if flatHeavy <= flatLight {
		t.Fatal("the single window should already tell the two apart")
	}
}

// A body with nothing left is dead either way, and the chain must not push
// anything past certainty.
func TestLookaheadStaysAProbability(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LookaheadHorizons = 2
	s := aSatiatedWholeBody(&cfg)
	for _, v := range []float64{-1, 0, 1, 5, 50, 100} {
		for _, h := range []float64{0, 50, 100} {
			got := pressure(&cfg, &s, v, h, 0.3)
			if got < 0 || got > 1 || math.IsNaN(got) {
				t.Fatalf("pressure out of range: v=%v h=%v got %v", v, h, got)
			}
		}
	}
	if got := pressure(&cfg, &s, 0, 50, 0); got != 1 {
		t.Fatalf("a body with nothing left should read 1, got %v", got)
	}
}

// The recovery path has to survive the change: resting must still be worth
// something to a battered, fed body (the first of the principles that must not
// be broken).
func TestLookaheadKeepsRestingWorthwhile(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LookaheadHorizons = 1
	s := aSatiatedWholeBody(&cfg)
	s.Vitality = cfg.MaxVitality * 0.4
	s.Hunger = cfg.SatiatedHunger - 10 // recoverable returns nothing at the line itself
	s.RestRate = cfg.RegenRate

	before := pressure(&cfg, &s, s.Vitality, s.Hunger, 0)
	mended := s.Vitality + recoverable(&cfg, s.MaxVitality, s.HungerRate, s.Vitality, s.Hunger, 0, s.RestRate)
	after := pressure(&cfg, &s, mended, s.Hunger, 0)
	if !(after < before) {
		t.Fatalf("resting must still lower the odds of dying: before %v after %v", before, after)
	}
}

// What the stage actually changes about a decision: the option to pick
// something up is scored off keepValue, and keepValue is the figure that was
// flat. Scored directly, so that the claim is about the rule and not about how
// a whole world happened to come out.
func TestLookaheadPutsAPriceOnPickingSomethingUp(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)

	keep := func(dose float64) float64 {
		cfg := cfg
		cfg.LookaheadHorizons = dose
		p := &Perception{
			Tick: 1, Cfg: &cfg, Rand: w.rng,
			Self: SelfView{
				ID: 1, X: 200, Y: 200,
				Vitality: cfg.MaxVitality, Hunger: cfg.SatiatedHunger,
				MaxVitality: cfg.MaxVitality, MaxSpeed: cfg.MaxSpeed,
				HungerRate: cfg.HungerRate, RestRate: cfg.RegenRate,
				ShockRisk: cfg.ShockRisk, CarryRoom: true,
				CarryCapacity: 1, FoodScarcity: 3,
				Nutrition: [NumFoodKinds]float64{1, 1, 1, 1},
			},
			Foods: []FoodView{{ID: 9, Dist: 20, RivalDist: math.Inf(1), Nutrition: 1}},
		}
		c := &AIController{tracing: true}
		c.addFood(p)
		best := 0.0
		for i, o := range c.opts {
			if o.action.Kind == ActTake && c.terms[i].Life.Value > best {
				best = c.terms[i].Life.Value
			}
		}
		return best
	}

	off, on := keep(0), keep(1)
	if off != 0 {
		t.Fatalf("a whole, fed body had no reason to pick anything up before stage 67, got %v", off)
	}
	if on <= 0 {
		t.Fatalf("with one step of lookahead it should have one, got %v", on)
	}
}

// And the world still runs with it on: the knob is a Config field, so both
// arms live in the same binary and can be put against the same seeds.
func TestLookaheadWorldStillRuns(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 7
	cfg.LookaheadHorizons = 1
	w := NewWorld(cfg)
	for i := 0; i < 4000; i++ {
		w.Step()
	}
	if len(w.agents) == 0 {
		t.Fatal("the world emptied")
	}
}
