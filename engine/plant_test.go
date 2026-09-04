package engine

import (
	"math"
	"testing"
)

// The arm this stage is measured against: with the plants' inheritance off,
// the world is the one stage 15a left, down to the values drawn from the
// random source.
func TestNoPlantGeneticsLeavesTheWorldExactlyAsItWas(t *testing.T) {
	run := func(on bool) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 13
		cfg.PlantGenetics = on
		w := NewWorld(cfg)
		for i := 0; i < 1500; i++ {
			w.Step()
		}
		return w.Stats()
	}
	if off, again := run(false), run(false); off != again {
		t.Fatal("the same world twice gave different runs")
	} else if on := run(true); on == off {
		t.Fatal("giving the plants an inheritance changed nothing at all")
	}
}

// How many plants there are is still FoodSpawnRate's business. The genes decide
// whose children they are and where they land, and nothing else - the same
// separation stage 15a made between how much food there is and where it is.
func TestPlantGenesMoveTheFoodWithoutChangingHowMuchThereIs(t *testing.T) {
	count := func(on bool) int {
		cfg := DefaultConfig()
		cfg.Seed = 14
		cfg.PlantGenetics = on
		cfg.MaxFoodItems = 100000
		cfg.InitialPopulation, cfg.InitialEnemies, cfg.InitialFoodItems = 0, 0, 1
		w := NewWorld(cfg)
		for i := 0; i < 3000; i++ {
			w.Step()
		}
		// Plants only. Carcasses are counted separately because the enemies
		// that walk in from off the map die at different moments in the two
		// runs, and how much meat is lying about says nothing about how many
		// plants grew.
		plants := 0
		for _, f := range w.Foods() {
			if f.Kind == FoodPlant {
				plants++
			}
		}
		return plants
	}
	// Exactly the same, not approximately: FoodSpawnRate says how many and
	// nothing in this stage touches it.
	if fixed, evolving := count(false), count(true); fixed != evolving {
		t.Fatalf("a fixed world grew %d plants and an evolving one %d, want exactly the same number",
			fixed, evolving)
	}
}

// A seedling is its parent, most of the time. Rare and large, as the agents'
// inheritance is: a nudge on every seed would leave nothing of the parent in
// the child.
func TestASeedlingIsItsParentMostOfTheTime(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	parent := plantGenes{Spread: 100, Regrow: 1}

	same, moved := 0, 0
	for i := 0; i < 2000; i++ {
		child := w.inheritPlantGenes(parent)
		if child == parent {
			same++
		} else {
			moved++
		}
	}
	if share := float64(same) / 2000; share < 0.85 {
		t.Fatalf("only %.0f%% of seedlings were their parent unchanged, want the great majority", share*100)
	}
	if moved == 0 {
		t.Fatal("no seedling ever differed from its parent")
	}
	// And the jumps, when they come, are worth having.
	big := 0
	for i := 0; i < 20000; i++ {
		if c := w.inheritPlantGenes(parent); math.Abs(c.Spread-parent.Spread) > parent.Spread*0.2 {
			big++
		}
	}
	if big == 0 {
		t.Fatal("no mutation ever moved the spread by as much as a fifth")
	}
}

// Plants seed in proportion to how readily they seed, which is what makes
// Regrow a fitness rather than a subsidy: a plant that seeds twice as readily
// takes the place of one that does not, without the world growing more food.
func TestPlantsSeedInProportionToHowReadilyTheySeed(t *testing.T) {
	cfg := testConfig()
	cfg.FoodSpread = 0 // even ground, so only the gene is speaking
	w := NewWorld(cfg)

	eager := w.addPlant(100, 100, plantGenes{Spread: 5, Regrow: 3})
	idle := w.addPlant(300, 300, plantGenes{Spread: 5, Regrow: 0.3})
	_, _ = eager, idle

	eagerPicks, idlePicks := 0, 0
	for i := 0; i < 4000; i++ {
		switch w.seedFrom() {
		case 0:
			eagerPicks++
		case 1:
			idlePicks++
		}
	}
	if eagerPicks <= idlePicks*3 {
		t.Fatalf("the eager plant was picked %d times and the idle one %d, want roughly ten to one",
			eagerPicks, idlePicks)
	}
}

// A world that has lost its last plant has to be able to grow another. Nothing
// can seed where nothing grows, so without a fallback an empty world is an
// absorbing state - a rule nobody chose.
func TestAWorldWithNoPlantsLeftCanStartAgain(t *testing.T) {
	cfg := testConfig()
	cfg.FoodSpawnRate = 1
	w := NewWorld(cfg)
	if got := w.seedFrom(); got != -1 {
		t.Fatalf("an empty world offered plant %d to seed from", got)
	}
	w.spawnFood()
	if len(w.Foods()) != 1 {
		t.Fatalf("an empty world grew %d plants, want it to start again from the ground", len(w.Foods()))
	}
}

// Dispersal makes thickets, and that is the shape the spread gene exists to
// argue about. Seeds thrown a short way pile up where their parent stood; seeds
// thrown far land where the parent never was.
func TestShortDispersalMakesThicketsAndLongDispersalDoesNot(t *testing.T) {
	clumping := func(spread float64) float64 {
		cfg := DefaultConfig()
		cfg.Seed = 15
		cfg.InitialPopulation, cfg.InitialEnemies = 0, 0
		cfg.InitialFoodItems = 40
		cfg.MaxFoodItems = 300
		cfg.FoodSpread = 0
		cfg.PlantMutationRate = 0 // hold the gene still and watch the shape
		cfg.PlantSpread = spread
		w := NewWorld(cfg)
		// Every plant starts on the gene under test.
		for i := range w.foods {
			w.foods[i].Genes = plantGenes{Spread: spread, Regrow: 1}
		}
		for i := 0; i < 4000; i++ {
			w.Step()
		}
		return w.Plants().Clumping
	}
	near, far := clumping(30), clumping(400)
	if near <= far {
		t.Fatalf("seeds thrown 30 clumped at %.2f and seeds thrown 400 at %.2f, want the short throw to make thickets",
			near, far)
	}
}
