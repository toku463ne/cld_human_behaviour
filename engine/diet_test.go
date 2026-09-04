package engine

import (
	"math"
	"testing"
)

// The arm the stage is measured against: no penalty, and the world is the one
// from before it.
func TestNoSamenessPenaltyLeavesTheWorldExactlyAsItWas(t *testing.T) {
	run := func(penalty float64) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 8
		cfg.SamenessPenalty = penalty
		w := NewWorld(cfg)
		for i := 0; i < 1500; i++ {
			w.Step()
		}
		return w.Stats()
	}
	if off, again := run(0), run(0); off != again {
		t.Fatal("the same world twice gave different runs")
	} else if on := run(0.35); on == off {
		t.Fatal("the diet rule changed nothing at all")
	}
}

// Living on one thing makes the next of it worth less, and it recovers once
// the agent stops.
func TestEatingTheSameThingIsWorthLessAndThenRecovers(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80, Genome: genomeOf(50, 50, 50)}))

	fresh := w.dietValue(a, FoodPlant)
	if fresh != 1 {
		t.Fatalf("an agent that has eaten nothing values a plant at %v, want 1", fresh)
	}
	for i := 0; i < 12; i++ {
		w.noteEaten(a, FoodPlant)
	}
	sick := w.dietValue(a, FoodPlant)
	if sick >= fresh {
		t.Fatalf("after twelve plants one is worth %v, want less than %v", sick, fresh)
	}
	// Never worthless: an agent with one thing available must still eat it.
	if sick <= 0 {
		t.Fatalf("a plant became worth %v; being sick of something is not a reason to starve beside it", sick)
	}
	// And the other kind is untouched - it is sick of plants, not of eating.
	if meat := w.dietValue(a, FoodMeat); meat != 1 {
		t.Fatalf("living on plants made meat worth %v, want 1", meat)
	}
	// It comes back once it stops.
	w.tick += 3000
	if recovered := w.dietValue(a, FoodPlant); recovered <= sick {
		t.Fatalf("after three thousand ticks off plants one is worth %v, want more than %v", recovered, sick)
	}
}

// The discount is what actually reaches the agent: a mouthful it is sick of
// takes less off its hunger.
func TestAMouthfulYouAreSickOfFeedsYouLess(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)

	eatOne := func(prime int) float64 {
		a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80,
			Hunger: 90, Genome: genomeOf(50, 50, 50)}))
		for i := 0; i < prime; i++ {
			w.noteEaten(a, FoodPlant)
		}
		id := w.addFood(101, 100)
		before := a.Hunger
		w.eat(a, id)
		return before - a.Hunger
	}

	fed, sickOfIt := eatOne(0), eatOne(12)
	if sickOfIt >= fed {
		t.Fatalf("a plant took %v off the hunger of an agent living on them and %v off a fresh one",
			sickOfIt, fed)
	}
	if math.Abs(fed-cfg.FoodNutrition) > 1e-9 {
		t.Fatalf("a fresh agent got %v from a plant, want the world's %v", fed, cfg.FoodNutrition)
	}
}

// It is not hidden. Unlike lifespan, which an agent has no way to know about,
// this reaches the utility comparison like hunger does.
func TestBeingSickOfSomethingIsVisibleToTheDecision(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)

	foodValue := func(nutrition float64) float64 {
		p := &Perception{
			Tick: 1, Cfg: &cfg, Rand: w.rng,
			Self: SelfView{
				ID: 1, X: 100, Y: 100, Vitality: 60, Hunger: 70,
				MaxVitality: cfg.MaxVitality, MaxSpeed: cfg.MaxSpeed,
				HungerRate:  cfg.HungerRate,
				Retaliation: cfg.Retaliation, AcceptChance: cfg.AcceptChance,
				RiskWeight: cfg.RiskWeight, CompetitionWeight: cfg.CompetitionWeight,
				ShockRisk: cfg.ShockRisk},
			Foods: []FoodView{{ID: 9, Dist: 20, Kind: FoodPlant,
				Nutrition: nutrition, RivalDist: math.Inf(1)}},
		}
		c := &AIController{}
		c.addFood(p)
		best := math.Inf(-1)
		for _, o := range c.opts {
			best = math.Max(best, o.util)
		}
		return best
	}

	fresh, sickOfIt := foodValue(1), foodValue(0.65)
	if sickOfIt >= fresh {
		t.Fatalf("a plant scored %v to an agent sick of them and %v to a fresh one; the discount is not reaching the decision",
			sickOfIt, fresh)
	}
}

// The measurement has to read zero in the world it cannot act on, or it is no
// use as a baseline. With one kind of food eaten there is no variety to have.
func TestDietVarietyIsZeroForAnAgentLivingOnOneThing(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80, Genome: genomeOf(50, 50, 50)}))
	for i := 0; i < 10; i++ {
		w.noteEaten(a, FoodPlant)
	}
	if got := w.Diet().Variety; got != 0 {
		t.Fatalf("an agent living on plants alone scores %v of variety, want 0", got)
	}
	for i := 0; i < 10; i++ {
		w.noteEaten(a, FoodMeat)
	}
	if got := w.Diet().Variety; math.Abs(got-1) > 1e-9 {
		t.Fatalf("an even split of both kinds scores %v of variety, want 1", got)
	}
}
