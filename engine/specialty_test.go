package engine

import (
	"math"
	"testing"
)

// cropConfig is a still world that grows the awkward crop everywhere.
func cropConfig() Config {
	cfg := quietConfig()
	cfg.SpecialtyShare, cfg.SpecialtySpread = 1, 0
	cfg.SpecialtyCatch = 0.25
	return cfg
}

// A world that grows none of it is the world from before, down to the random
// draws: a spread with no crop behind it must not decide anything.
func TestAWorldWithoutTheCropIsUntouched(t *testing.T) {
	run := func(spread float64) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 6
		cfg.SpecialtySpread = spread
		w := NewWorld(cfg)
		for i := 0; i < 2000; i++ {
			w.Step()
		}
		return w.Stats()
	}
	if spread, none := run(0.6), run(0); spread != none {
		t.Fatal("a world with no awkward crop ran differently for having a spread set for one")
	}
}

// How much of what comes up needs knowing follows from where it comes up.
func TestTheCropComesUpWhereTheRegionGrowsIt(t *testing.T) {
	cfg := quietConfig()
	cfg.SpecialtyShare, cfg.SpecialtySpread = 0.5, 0
	cfg.InitialPopulation, cfg.InitialEnemies, cfg.EnemySpawnTicks = 0, 0, 0
	w := NewWorld(cfg)
	// One region grows nothing but the awkward crop, another none of it.
	w.regions[0].Special, w.regions[1].Special = 2, 0

	special, ordinary := 0, 0
	for i := 0; i < 400; i++ {
		w.spawnFood()
	}
	for _, f := range w.Foods() {
		switch w.regionIndexAt(f.X, f.Y) {
		case 0:
			if f.Special {
				special++
			}
		case 1:
			if f.Special {
				ordinary++
			}
		}
	}
	if special == 0 {
		t.Fatal("the region that grows nothing else grew none of it")
	}
	if ordinary != 0 {
		t.Fatalf("%d awkward plants came up where the crop does not grow", ordinary)
	}
}

// It can be reached and still not had, and what a body knows lifts the chance
// without ever being the whole of it: a green body still gets some.
func TestKnowingTheTrickLandsMoreOfThemAndIsNeverTheOnlyWay(t *testing.T) {
	cfg := cropConfig()
	w := NewWorld(cfg)
	green := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Genome: filledGenome(50)}))
	adept := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 300, Y: 300, Genome: filledGenome(100)}))
	adept.hintSlots++
	w.learnSkill(adept, SkillHarvest, 1)
	crop := Food{X: 100, Y: 100, Kind: FoodPlant, Special: true}

	greenOdds, adeptOdds := w.harvestCatch(green, &crop), w.harvestCatch(adept, &crop)
	if greenOdds <= 0 {
		t.Fatal("a body that has never learned the trick can never get one: that is a gate")
	}
	if adeptOdds <= greenOdds {
		t.Fatalf("knowing the trick lands %v of them and not knowing it %v", adeptOdds, greenOdds)
	}
	if math.Abs(greenOdds-cfg.SpecialtyCatch) > 1e-9 {
		t.Fatalf("a green body lands %v of them, want the world's %v", greenOdds, cfg.SpecialtyCatch)
	}
	// And an ordinary plant is simply picked, whoever picks it.
	plain := Food{X: 100, Y: 100, Kind: FoodPlant}
	if got := w.harvestCatch(green, &plain); got != 1 {
		t.Fatalf("an ordinary plant is landed %v of the time", got)
	}
}

// A failed attempt leaves it where it is: it is rooted, so what the failure
// costs is the tick and having to decide again. Nothing is destroyed.
func TestAFailedHarvestLeavesThePlantStanding(t *testing.T) {
	cfg := cropConfig()
	cfg.SpecialtyCatch = 0 // nothing is ever got out
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80,
		Hunger: 90, Genome: filledGenome(50)}))
	id := w.putFood(Food{X: 101, Y: 100, Kind: FoodPlant, Special: true})

	a.Action = Action{Kind: ActEat, TargetID: id}
	w.perform(a)

	if a.Hunger != 90 {
		t.Fatal("a plant nobody could get out of the ground fed somebody")
	}
	f := w.foodByID(id)
	if f == nil {
		t.Fatal("the plant is gone after an attempt that failed")
	}
	if f.X != 101 || f.Y != 100 {
		t.Fatal("a rooted plant moved")
	}
	if w.Stats().HarvestMissed == 0 {
		t.Fatal("the failed attempt was not counted")
	}
	if !a.needsDecision {
		t.Fatal("the body was not asked to think again after failing")
	}
}

// Once it is out of the ground it is a plant like any other: it feeds a body
// what a plant feeds a body, and the diet ledger has not learned a new word.
func TestTheCropIsAPlantInEveryOtherWay(t *testing.T) {
	cfg := cropConfig()
	cfg.SpecialtyCatch = 1 // this test is about the meal, not the odds
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80,
		Hunger: 90, Genome: filledGenome(50)}))
	id := w.putFood(Food{X: 101, Y: 100, Kind: FoodPlant, Special: true})

	before := a.Hunger
	w.eat(a, id)
	if got := before - a.Hunger; math.Abs(got-cfg.FoodNutrition) > 1e-9 {
		t.Fatalf("the awkward crop fed %v, want what a plant feeds: %v", got, cfg.FoodNutrition)
	}
	if w.recentlyEaten(a, FoodPlant) == 0 {
		t.Fatal("the ledger did not count it as a plant")
	}
	if w.recentlyEaten(a, FoodFish) != 0 {
		t.Fatal("the ledger counted it as something else")
	}
}

// Born where it grows is how a line comes to know the trick without anybody
// having taught it - and the cap on what a body makes of it is the gene the
// doing belongs to.
func TestBornWhereItGrowsAndCappedByWits(t *testing.T) {
	cfg := quietConfig()
	cfg.SpecialtyShare, cfg.SpecialtySpread, cfg.SkillBirthplace = 0.5, 0, 1
	w := NewWorld(cfg)
	w.regions[0].Special, w.regions[1].Special = 2, 0

	x0, y0, _, _ := w.regionBounds(0)
	x1, y1, _, _ := w.regionBounds(1)
	if born := w.skillFromBirthplace(SkillHarvest, x0+10, y0+10); born <= 0 {
		t.Fatalf("a body born where the crop grows knows %v of the trick", born)
	}
	if born := w.skillFromBirthplace(SkillHarvest, x1+10, y1+10); born != 0 {
		t.Fatalf("a body born where it does not grow knows %v of the trick", born)
	}

	sharp := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Genome: genomeOf(50, 50, 100)}))
	dull := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 300, Y: 300, Genome: genomeOf(50, 50, 10)}))
	for _, a := range []*Agent{sharp, dull} {
		a.hintSlots++
		w.learnSkill(a, SkillHarvest, 1)
	}
	if sharp.skillAt(&cfg, SkillHarvest) <= dull.skillAt(&cfg, SkillHarvest) {
		t.Fatal("working out how to get an awkward thing out of the ground is not capped by wits")
	}
}
