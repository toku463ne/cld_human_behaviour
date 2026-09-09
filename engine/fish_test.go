package engine

import (
	"math"
	"testing"
)

// riverConfig is a world with a river down the middle of it.
func riverConfig() Config {
	cfg := DefaultConfig()
	cfg.Seed = 7
	cfg.TerrainMap = []string{
		"....~~....",
		"....~~....",
		"....~~....",
		"....~~....",
		"....~~....",
		"....~~....",
	}
	cfg.FishShare = 0.25
	return cfg
}

// A world with no water has no fish in it, and - the part that matters for
// every figure recorded before this stage - it does not so much as draw a
// random number deciding.
func TestAFlatWorldIsUntouchedByFish(t *testing.T) {
	run := func(share float64) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 4
		cfg.FishShare = share
		w := NewWorld(cfg)
		for i := 0; i < 2000; i++ {
			w.Step()
		}
		return w.Stats()
	}
	if fishy, dry := run(0.5), run(0); fishy != dry {
		t.Fatal("a flat world runs differently with fish switched on; something drew from the random source")
	}
}

// The world puts fish in the water and nowhere else.
//
// It says nothing about where a fish ends up: one that has been landed belongs
// to whoever landed it, and a body that dies on the bank drops what it was
// carrying there. What tells the two apart is the clock - the world plants a
// living fish with no clock on it, and landing one starts a carcass's.
func TestTheWorldPutsFishInTheWaterAndNowhereElse(t *testing.T) {
	w := NewWorld(riverConfig())
	for i := 0; i < 3000; i++ {
		w.Step()
	}
	n := 0
	for _, f := range w.Foods() {
		if f.Kind != FoodFish || f.SpoilAt != 0 {
			continue // landed by somebody, and theirs to put down where they like
		}
		n++
		if got := w.terrainAt(f.X, f.Y).Kind; got != GroundWater {
			t.Fatalf("the world grew a fish on ground of kind %v at %.0f,%.0f", got, f.X, f.Y)
		}
	}
	if n == 0 {
		t.Fatal("a world with a quarter of its crop in the water grew no fish at all")
	}
}

// A fish out of the water is dead flesh: it goes off in the hand that holds
// it, and it goes off lying on the bank where a body that died dropped it.
// Nothing about being carried stops the clock (#77) and nothing about the
// ground it lands on starts a second one.
func TestALandedFishGoesOff(t *testing.T) {
	cfg := riverConfig()
	cfg.CarryCapacity = 1
	w := NewWorld(cfg)
	// Somebody standing in the river with a fish in its hand.
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Genome: genomeOf(50, 50, 50)})
	a := mustAgent(t, w, id)
	fishID := w.putFood(Food{X: 101, Y: 100, Kind: FoodFish})
	w.take(a, fishID)
	if a.CarriedCount() != 1 {
		t.Fatal("the fish was not landed")
	}
	if got := a.carried[0].SpoilAt; got != w.tick+cfg.MeatSpoilTicks {
		t.Fatalf("a landed fish spoils at %d, and a carcass landed now would at %d",
			got, w.tick+cfg.MeatSpoilTicks)
	}

	// It goes off in the hand.
	w.tick += cfg.MeatSpoilTicks + 1
	w.clearSpoiled()
	if a.CarriedCount() != 0 {
		t.Fatal("a fish landed a lifetime ago is still in the hand")
	}
	if w.Stats().FishSpoiled != 1 {
		t.Fatalf("%d fish counted as gone off", w.Stats().FishSpoiled)
	}

	// And a fish the world planted has no clock on it at all: nothing here
	// makes a living fish rot in the river.
	living := w.putFood(Food{X: 100, Y: 100, Kind: FoodFish})
	if got := w.foodByID(living).SpoilAt; got != 0 {
		t.Fatalf("a fish still in the river spoils at %d", got)
	}
	w.tick += cfg.MeatSpoilTicks + 1
	w.clearSpoiled()
	if w.foodByID(living) == nil {
		t.Fatal("a fish nobody ever landed rotted where it swam")
	}
}

// A fish dropped by a body that died on dry ground stays where it fell, with
// the clock it was landed with still running.
func TestAFishDroppedOnTheBankLiesThereAndRots(t *testing.T) {
	cfg := riverConfig()
	cfg.CarryCapacity = 1
	w := NewWorld(cfg)
	// Dry ground to die on, with room round it so that the drop lands dry too.
	dx, dy := 0.0, 0.0
	for y := 20.0; y < cfg.Height-20 && dy == 0; y += 8 {
		for x := 20.0; x < cfg.Width-20; x += 8 {
			if dry(w, x, y) && dry(w, x-8, y) && dry(w, x+8, y) {
				dx, dy = x, y
				break
			}
		}
	}
	if dy == 0 {
		t.Fatal("this world is all river")
	}
	id := w.addAgent(Agent{Maturity: 1, X: dx, Y: dy, Vitality: 90,
		Genome: genomeOf(50, 50, 50)})
	a := mustAgent(t, w, id)
	w.take(a, w.putFood(Food{X: dx + 1, Y: dy, Kind: FoodFish}))
	spoilAt := a.carried[0].SpoilAt

	w.dropCarried(a)
	var dropped *Food
	for i := range w.foods {
		if w.foods[i].Kind == FoodFish && w.foods[i].SpoilAt == spoilAt {
			dropped = &w.foods[i]
		}
	}
	if dropped == nil {
		t.Fatal("the fish vanished when the body carrying it dropped it")
	}
	if math.Hypot(dropped.X-dx, dropped.Y-dy) > 8 {
		t.Fatalf("the fish was dropped at %.0f,%.0f and the body was at %.0f,%.0f",
			dropped.X, dropped.Y, dx, dy)
	}
	if !dry(w, dropped.X, dropped.Y) {
		t.Fatal("the fish found its own way back into the water")
	}
	w.tick = spoilAt
	w.clearSpoiled()
	for i := range w.foods {
		if w.foods[i].Kind == FoodFish && w.foods[i].SpoilAt == spoilAt {
			t.Fatal("a fish left on the bank never went off")
		}
	}
}

// A fish takes the place of a plant rather than adding to the world. How much
// food there is stays FoodSpawnRate's business, which is the principle every
// stage since regions arrived has kept.
func TestFishTakeThePlaceOfPlantsRatherThanAddingToTheWorld(t *testing.T) {
	grown := func(share float64) int {
		cfg := riverConfig()
		cfg.FishShare = share
		// Nobody eats and nobody dies, so what is counted is what the world
		// put there; and the allowance is out of reach, so what is counted is
		// how often it planted rather than when it stopped.
		cfg.InitialPopulation, cfg.InitialEnemies, cfg.EnemySpawnTicks = 0, 0, 0
		cfg.MaxFoodItems = 100000
		w := NewWorld(cfg)
		for i := 0; i < 2000; i++ {
			w.Step()
		}
		return w.countKind(FoodPlant) + w.countKind(FoodFish)
	}
	dry, fishy := grown(0), grown(0.25)
	if dry != fishy {
		t.Fatalf("a world with fish holds %d items and one without holds %d", fishy, dry)
	}
}

// Who may eat one is a fact about the species, as every diet in this world is.
// Enemies live on meat; the arm that changes that is a measurement, not a
// default.
func TestOnlyHumansFishUnlessTheWorldSaysOtherwise(t *testing.T) {
	for _, all := range []bool{false, true} {
		cfg := riverConfig()
		cfg.FishForAll = all
		w := NewWorld(cfg)
		human := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Genome: genomeOf(50, 50, 50)}))
		enemy := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 300, Y: 300,
			Genome: genomeOf(50, 50, 50), Species: SpeciesEnemy}))
		fish := Food{X: 100, Y: 100, Kind: FoodFish}

		if !w.canEat(human, &fish) {
			t.Fatal("a human cannot eat a fish")
		}
		if got := w.canEat(enemy, &fish); got != all {
			t.Fatalf("FishForAll=%v: an enemy may eat a fish = %v", all, got)
		}
	}
}

// Fish and plants share one allowance: a fish is a plant that did not come up,
// so two allowances would let the world hold more food than it ever said.
func TestFishAndPlantsShareOneAllowance(t *testing.T) {
	cfg := riverConfig()
	cfg.MaxFoodItems = 4
	w := NewWorld(cfg)
	for i := 0; i < 4; i++ {
		w.addFood(float64(100+i), 100)
	}
	if id := w.addFish(220, 100); id != 0 {
		t.Fatal("a fish went in on top of a full world of plants")
	}
}

// What "a varied diet" means depends on what this world can offer. A world
// with no water has two kinds in it, and a diet split evenly between them is
// as varied as that world allows - which is what keeps every figure recorded
// before fish existed readable.
func TestVarietyIsScaledToWhatTheWorldCanOffer(t *testing.T) {
	varied := func(cfg Config) float64 {
		w := NewWorld(cfg)
		a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Genome: genomeOf(50, 50, 50)}))
		for i := 0; i < 10; i++ {
			w.noteEaten(a, FoodPlant)
			w.noteEaten(a, FoodMeat)
		}
		return w.Diet().Variety
	}
	flat := quietConfig()
	if got := varied(flat); got < 0.99 {
		t.Fatalf("an even split of the two kinds a dry world has scores %v, want 1", got)
	}
	river := riverConfig()
	river.HungerRate, river.RegenRate = 0, 0
	if got := varied(river); got >= 0.99 {
		t.Fatalf("an even split of two kinds where three are on offer scores %v, want less than 1", got)
	}
}

// dry says whether this spot is out of the water.
func dry(w *World, x, y float64) bool {
	return w.terrainAt(x, y).Kind != GroundWater
}

// And the way back to the world as it was before: a landed fish that keeps for
// ever, which is what every figure recorded for stages 42 and 43 was measured
// in.
func TestALandedFishCanBeMadeToKeep(t *testing.T) {
	cfg := riverConfig()
	cfg.CarryCapacity, cfg.LandedFishKeeps = 1, true
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Genome: genomeOf(50, 50, 50)})
	a := mustAgent(t, w, id)
	w.take(a, w.putFood(Food{X: 101, Y: 100, Kind: FoodFish}))
	if got := a.carried[0].SpoilAt; got != 0 {
		t.Fatalf("a landed fish spoils at %d in the world where fish keep", got)
	}
	w.tick += cfg.MeatSpoilTicks * 10
	w.clearSpoiled()
	if a.CarriedCount() != 1 {
		t.Fatal("the fish went off in the world where fish keep")
	}
}
