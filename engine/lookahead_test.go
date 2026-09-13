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

// It is the default since 2026-09-13, and turning it off has to put the
// single window back exactly: that is the world every figure recorded before
// then was measured in.
func TestLookaheadOffIsExactlyOneWindow(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.LookaheadHorizons != 1 {
		t.Fatalf("LookaheadHorizons should default to 1, got %v", cfg.LookaheadHorizons)
	}
	cfg.LookaheadHorizons = 0
	s := aSatiatedWholeBody(&cfg)
	for _, v := range []float64{5, 20, 50, 100} {
		for _, h := range []float64{0, 40, 60, 90} {
			s.Vitality, s.Hunger = v, h
			got := pressure(&cfg, &s, v, h, 0)
			want := oneHorizon(&cfg, &s, v, projectedDrain(&cfg, s.HungerRate, h), true)
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

	cfg.LookaheadHorizons = 0
	if got := keepValue(&cfg, &s, 0, 1, 0, 0); got != 0 {
		t.Fatalf("without lookahead a whole, fed body should read a flat gradient, got %v", got)
	}
	cfg.LookaheadHorizons = 1
	if got := keepValue(&cfg, &s, 0, 1, 0, 0); got <= 0 {
		t.Fatalf("with lookahead it should be worth something, got %v", got)
	}
}

// And it is a dial, not a gate: no threshold anywhere, which is the line the
// design draws against behavioural thresholds.
func TestLookaheadRisesWithTheDose(t *testing.T) {
	cfg := DefaultConfig()
	// The dial on its own: what the second window assumes about the body is
	// stage 72's question and would flatten this one out.
	cfg.LookaheadUpkeep, cfg.LookaheadNeverBlinds = 0, false
	s := aSatiatedWholeBody(&cfg)
	last := -1.0
	for _, dose := range []float64{0, 0.25, 0.5, 1, 2} {
		cfg.LookaheadHorizons = dose
		got := keepValue(&cfg, &s, 0, 1, 0, 0)
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

// The bill the second window pays at the other end, which is why it is still
// off by default (measured 2026-09-13, when it was made the default and put
// back the same day).
//
// The life term is a difference of two chances of dying. One window leaves it
// flat at the top - a satiated whole body cannot die inside it, so nothing is
// worth keeping - and that is what stage 67 fixed. Two windows leave it flat
// at the bottom instead: a body that cannot live out two horizons unfed is
// going to die in both branches, so the difference between them collapses and
// every option looks alike. The goals priced by a constant rather than by that
// gradient - offspring, exploring - do not collapse with it, which is how a
// starving body comes to court instead of eat.
func TestTheSecondWindowFlattensTheBottomEnd(t *testing.T) {
	cfg := DefaultConfig()
	s := aSatiatedWholeBody(&cfg)
	s.Vitality, s.Hunger = 20, cfg.MaxHunger*0.9 // in real trouble

	// What one meal is worth to it: the gap between where it stands and where
	// eating would put it.
	gap := func(dose float64) float64 {
		cfg.LookaheadHorizons = dose
		fed := math.Max(0, s.Hunger-cfg.FoodNutrition)
		return pressure(&cfg, &s, s.Vitality, s.Hunger, 0) -
			pressure(&cfg, &s, s.Vitality, fed, 0)
	}
	one, two := gap(0), gap(1)
	if one <= 0 {
		t.Fatalf("with one window a meal should be worth something to a starving body, got %v", one)
	}
	if two >= one {
		t.Fatalf("the second window was expected to flatten this, got %v against %v", two, one)
	}
	// Not a rounding difference: it is most of the value of the meal.
	if two > one/2 {
		t.Fatalf("the flattening is smaller than it was measured to be: %v against %v", two, one)
	}
}

// Stage 72: what the second window is allowed to assume about the body it is
// carrying forward.
//
// Two things were wrong with carrying it forward on nothing. It ate nothing
// out there, and it mended nothing; and on top of that the standing hazard of
// being worn down was charged in both windows, though ShockRisk is calibrated
// against one. Together those made a body at a seventh of its vitality read
// its own death as settled whatever it did.
func TestUpkeepPutsFleeingBackInAWornBodysReach(t *testing.T) {
	cornered := func(dose, upkeep float64, wornAgain bool) ActionKind {
		cfg := testConfig()
		cfg.LookaheadHorizons, cfg.LookaheadUpkeep = dose, upkeep
		cfg.LookaheadWornAgain = wornAgain
		// Stage 72's own world: reading the life term through the clearer of
		// the two windows (stage 74) is what this scene is broken without.
		cfg.LookaheadNeverBlinds = false
		w := NewWorld(cfg)
		victim := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Sex: Male, Vitality: 14,
			Hunger: 20, Genome: genomeOf(15, 100, 100)})
		bully := w.addAgent(Agent{Maturity: 1, X: 208, Y: 200, Sex: Male, Vitality: 100,
			Hunger: 0, Genome: genomeOf(95, 100, 100)})
		// It has been feeding itself: a meal every four hundred ticks or so.
		v := mustAgent(t, w, victim)
		v.fedSum, v.fedAt = cfg.FoodNutrition*1.75, w.tick
		convinceOf(t, w, victim, bully)
		attackedBy(t, w, victim, bully)
		return aiChoice(w, victim).Kind
	}

	if got := cornered(0, 0, false); got != ActFlee {
		t.Fatalf("one window: a cornered body chose %v, want it to run", got)
	}
	if got := cornered(1, 0, false); got == ActFlee {
		t.Fatal("two windows with nothing assumed: it was expected to give up, and did not")
	}
	if got := cornered(1, 0.75, false); got != ActFlee {
		t.Fatalf("two windows with upkeep: a cornered body chose %v, want it to run", got)
	}
	// And both halves are needed: charging the standing hazard twice puts it
	// back where it was, however well the body has been keeping itself up.
	if got := cornered(1, 0.75, true); got == ActFlee {
		t.Fatal("charging ShockRisk in both windows was expected to cost the escape, and did not")
	}
}

// The top end has to survive it. What stage 67 bought is a satiated whole body
// that can see itself running short; assuming it keeps itself up entirely
// takes that back, so the dial has to leave some of the shortfall in.
func TestUpkeepLeavesTheSatiatedBodyAReasonToKeepFood(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LookaheadHorizons = 1
	s := aSatiatedWholeBody(&cfg)
	s.FedRate, s.RestRate = cfg.HungerRate*3, cfg.RegenRate

	cfg.LookaheadUpkeep = 0.75
	if got := keepValue(&cfg, &s, 0, 1, 0, 0); got <= 0 {
		t.Fatalf("with three quarters of its upkeep assumed it is worth %v to keep a meal", got)
	}
	cfg.LookaheadUpkeep = 1
	if got := keepValue(&cfg, &s, 0, 1, 0, 0); got != 0 {
		t.Fatalf("a body that assumes it keeps itself up entirely should see no shortfall, got %v", got)
	}
}

// Stage 74: the life term is read through whichever of the two windows can
// tell the two states apart.
//
// The term asks whether a body will be dead by the end of the window, and one
// meal moves a starving body's death from tick 594 to tick 1038. Against a
// window of 700 that is inside to outside and the answer changes; against
// 1400 it is inside to inside and the answer does not, so the meal is worth
// nothing and anything with a constant price on it wins. The meal did not
// change - the question did.
func TestTheClearerWindowKeepsAMealWorthSomething(t *testing.T) {
	starving := func(clear bool) (ActionKind, float64) {
		cfg := testConfig()
		cfg.LookaheadHorizons, cfg.LookaheadUpkeep = 1, 0.75
		cfg.LookaheadNeverBlinds = clear
		w := NewWorld(cfg)
		id := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Sex: Male, Vitality: 95,
			Hunger: cfg.MaxHunger * 0.9, Genome: genomeOf(50, 100, 100)})
		w.addAgent(Agent{Maturity: 1, X: 210, Y: 200, Sex: Female, Vitality: 100,
			Hunger: 0, Genome: genomeOf(90, 90, 90)})
		w.addFood(230, 200)
		a := mustAgent(t, w, id)
		a.reproReady = true
		a.fedSum, a.fedAt = cfg.FoodNutrition*1.75, w.tick
		s := w.selfView(a)
		fed := math.Max(0, s.Hunger-cfg.FoodNutrition)
		meal := gap(&cfg, pressures(&cfg, &s, s.Vitality, s.Hunger, 0),
			pressures(&cfg, &s, s.Vitality, fed, 0)) * cfg.LifeValue
		return aiChoice(w, id).Kind, meal
	}
	if got, meal := starving(false); meal != 0 || got == ActEat {
		t.Fatalf("both windows say death: the meal was expected to be worth nothing, got %v and %v", meal, got)
	}
	if got, meal := starving(true); meal <= 0 || got != ActEat {
		t.Fatalf("through the clearer window the meal is worth %v and the body chose %v", meal, got)
	}
}

// And the far window is still the one that counts where it is the only one
// that can see anything: a satiated whole body cannot die inside one window
// at all, so the near view of a meal put by is flat and the far view is not.
// That is stage 67, and the rule must not take it back.
func TestTheClearerWindowKeepsTheTopEnd(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LookaheadHorizons, cfg.LookaheadUpkeep = 1, 0.75
	cfg.LookaheadNeverBlinds = true
	s := aSatiatedWholeBody(&cfg)
	s.FedRate, s.RestRate = cfg.HungerRate*3, cfg.RegenRate

	if got := keepValue(&cfg, &s, 0, 1, 0, 0); got <= 0 {
		t.Fatalf("a satiated whole body sees no reason to keep a meal: %v", got)
	}
	// With one window it sees nothing, which is the thing stage 67 was for.
	one := cfg
	one.LookaheadHorizons = 0
	if got := keepValue(&one, &s, 0, 1, 0, 0); got != 0 {
		t.Fatalf("one window was expected to be flat here, got %v", got)
	}
}

// The swap never costs the body anything it could tell apart before: the
// chain is monotone in the near window, so the rule only ever takes the
// larger of two differences that agree in sign.
func TestTheClearerWindowNeverTellsABodyLess(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LookaheadHorizons, cfg.LookaheadUpkeep = 1, 0.75
	cfg.LookaheadNeverBlinds = true
	s := aSatiatedWholeBody(&cfg)
	s.RestRate = cfg.RegenRate
	for _, v := range []float64{10, 30, 60, 100} {
		for _, h := range []float64{0, 30, 60, 90} {
			s.Vitality, s.Hunger, s.FedRate = v, h, cfg.HungerRate
			before := pressures(&cfg, &s, v, h, 0)
			after := pressures(&cfg, &s, v, math.Max(0, h-cfg.FoodNutrition), 0)
			near, far := before.near-after.near, before.far-after.far
			got := gap(&cfg, before, after)
			if got < near-1e-12 || got < far-1e-12 {
				t.Fatalf("v=%v h=%v: the rule took %v where the two views said %v and %v", v, h, got, near, far)
			}
		}
	}
}
