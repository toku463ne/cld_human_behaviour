package engine

import (
	"math"
	"testing"
)

// anglerWorld is a world with a river down the middle and fish in it.
func anglerWorld(t *testing.T) (*World, Config) {
	t.Helper()
	cfg := riverConfig()
	cfg.HungerRate, cfg.RegenRate, cfg.StarveRate = 0, 0, 0
	w := NewWorld(cfg)
	return w, cfg
}

// learn gives a body a skill outright, which is what the three paths in
// skill.go do more slowly.
func learn(w *World, a *Agent, kind SkillKind, mastery float64) {
	a.hintSlots++
	w.learnSkill(a, kind, mastery)
}

// Fishing from the bank is reach and nothing else: a body that has it takes a
// fish it could not otherwise have got to, from where it stands.
func TestFishingFromTheBankReachesFurther(t *testing.T) {
	w, cfg := anglerWorld(t)
	// Far enough that the ordinary reach does not cover it.
	const dist = 20.0
	if dist <= cfg.GrabRadius {
		t.Fatal("this test needs a distance beyond the ordinary reach")
	}

	took := func(mastery float64) bool {
		a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
			Hunger: 80, Genome: filledGenome(100)}))
		learn(w, a, SkillFishLand, mastery)
		id := w.putFood(Food{X: 100 + dist, Y: 100, Kind: FoodFish})
		a.Action = Action{Kind: ActEat, TargetID: id}
		before := a.Hunger
		w.perform(a)
		return a.Hunger < before
	}
	if took(0) {
		t.Fatal("a body with no skill ate a fish it should have had to walk to")
	}
	if !took(1) {
		t.Fatal("a body that knows how to fish from the bank could not reach one")
	}
}

// ... and only for fish. It is a way of taking something out of the water, not
// a longer arm.
func TestTheLongerReachIsOnlyForFish(t *testing.T) {
	w, cfg := anglerWorld(t)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Hunger: 80, Genome: filledGenome(100)}))
	learn(w, a, SkillFishLand, 1)

	if got := w.fishReach(a, FoodPlant); got != cfg.GrabRadius {
		t.Fatalf("a plant is reachable from %v, want the ordinary %v", got, cfg.GrabRadius)
	}
	if got := w.fishReach(a, FoodFish); got <= cfg.GrabRadius {
		t.Fatalf("a fish is reachable from %v, want more than %v", got, cfg.GrabRadius)
	}
}

// Fishing in the water is yield, and only while standing in it. That is the
// trade: the bank is safe and ordinary, the water is dangerous and good.
func TestWadingPaysOnlyWhileStandingInTheWater(t *testing.T) {
	w, cfg := anglerWorld(t)

	// Where the water is on this map: the middle columns.
	wet, dry := 0.0, 0.0
	for x := 10.0; x < cfg.Width-10; x += 5 {
		if w.terrainAt(x, 300).Kind == GroundWater {
			wet = x
		} else if dry == 0 {
			dry = x
		}
	}
	if wet == 0 || dry == 0 {
		t.Fatal("this map has no bank and no water on it")
	}

	fed := func(x float64) float64 {
		a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: x, Y: 300, Vitality: 90,
			Hunger: 90, Genome: filledGenome(100)}))
		learn(w, a, SkillFishWater, 1)
		id := w.putFood(Food{X: x, Y: 300, Kind: FoodFish})
		before := a.Hunger
		w.eat(a, id)
		return before - a.Hunger
	}
	inWater, onLand := fed(wet), fed(dry)
	if inWater <= onLand {
		t.Fatalf("a fish taken in the water fed %v and one taken from the bank %v", inWater, onLand)
	}
	if math.Abs(onLand-cfg.FoodNutrition) > 1e-9 {
		t.Fatalf("a fish taken from the bank fed %v, want the ordinary %v", onLand, cfg.FoodNutrition)
	}
}

// It does not touch the drowning. That is swimming's figure, and two skills
// paying for the same thing means neither can be measured.
func TestWadingDoesNotMakeTheWaterSafer(t *testing.T) {
	w, cfg := anglerWorld(t)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Genome: filledGenome(100)}))
	learn(w, a, SkillFishWater, 1)

	ground := terrain{Drown: cfg.DrownChancePerTick, Kind: GroundWater, Cost: cfg.WaterMoveCost}
	if got := w.drownFelt(a, ground); math.Abs(got-cfg.DrownChancePerTick) > 1e-12 {
		t.Fatalf("a body that fishes in the water drowns at %v, want the world's %v",
			got, cfg.DrownChancePerTick)
	}
}

// The two are seeded from different readings of a place: being all water
// teaches wading and teaches nobody to fish from a bank there is none of.
func TestTheTwoAreSeededFromDifferentGround(t *testing.T) {
	cfg := riverConfig()
	// Water across the top, dry land all the way down: the top third of the
	// map is nothing but river, the bottom third is nowhere near it.
	cfg.TerrainMap = []string{
		"~~~~~~~~~~",
		"~~~~~~~~~~",
		"~~~~~~~~~~",
		"~~~~~~~~~~",
		"..........",
		"..........",
		"..........",
		"..........",
		"..........",
		"..........",
		"..........",
		"..........",
	}
	w := NewWorld(cfg)

	allWater := w.regionIndexAt(cfg.Width*0.5, cfg.Height*0.1)
	farInland := w.regionIndexAt(cfg.Width*0.5, cfg.Height*0.9)
	if w.regionWaterShare(allWater) <= w.regionWaterShare(farInland) {
		t.Fatal("the map is not laid out as this test assumes")
	}
	if got := w.regionBankShare(allWater); got != 0 {
		t.Fatalf("a region that is all water has a bank share of %v, want 0", got)
	}
	if got := w.regionBankShare(farInland); got != 0 {
		t.Fatalf("a region nowhere near water has a bank share of %v, want 0", got)
	}
	// And the region where the two meet has one: it is neither of the above,
	// which is the whole point of reading it separately.
	edge := w.regionIndexAt(cfg.Width*0.5, cfg.Height*0.5)
	if got := w.regionBankShare(edge); got <= 0 {
		t.Fatalf("the region where land meets water has a bank share of %v", got)
	}
}

// Each is capped by the gene the doing belongs to: reading the water from
// outside is rationality, working in it is how long a body can stay.
func TestEachWayOfFishingIsCappedByItsOwnGene(t *testing.T) {
	cfg := riverConfig()
	w := NewWorld(cfg)
	sharp := genomeOf(50, 100, 50)
	sharp[GeneVitality] = 10
	tough := genomeOf(50, 10, 50)
	tough[GeneVitality] = 100

	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Genome: sharp}))
	b := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 300, Y: 300, Genome: tough}))
	for _, x := range []*Agent{a, b} {
		learn(w, x, SkillFishLand, 1)
		learn(w, x, SkillFishWater, 1)
	}
	if a.skillAt(&cfg, SkillFishLand) <= b.skillAt(&cfg, SkillFishLand) {
		t.Fatal("the clear-headed body is no better at fishing from the bank")
	}
	if b.skillAt(&cfg, SkillFishWater) <= a.skillAt(&cfg, SkillFishWater) {
		t.Fatal("the tough body is no better at working the water")
	}
}
