package engine

import "testing"

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

// Fish are in the water and nowhere else.
func TestFishAreOnlyInTheWater(t *testing.T) {
	w := NewWorld(riverConfig())
	for i := 0; i < 3000; i++ {
		w.Step()
	}
	n := 0
	for _, f := range w.Foods() {
		if f.Kind != FoodFish {
			continue
		}
		n++
		if got := w.terrainAt(f.X, f.Y).Kind; got != GroundWater {
			t.Fatalf("a fish is on ground of kind %v at %.0f,%.0f", got, f.X, f.Y)
		}
	}
	if n == 0 {
		t.Fatal("a world with a quarter of its crop in the water grew no fish at all")
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
