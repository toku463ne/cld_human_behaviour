package engine

import "testing"

// stoneConfig is a world with broken ground and stones on it.
func stoneConfig() Config {
	cfg := quietConfig()
	cfg.TerrainMap = []string{
		"....::....",
		"....::....",
		"....::....",
		"....::....",
		"....::....",
		"....::....",
	}
	cfg.Stones = 20
	return cfg
}

// A world with nowhere for them, or none asked for, is the world from before -
// down to the random draws.
func TestAWorldWithNoBrokenGroundHasNoStones(t *testing.T) {
	run := func(stones int) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 9
		cfg.Stones = stones
		w := NewWorld(cfg)
		for i := 0; i < 1500; i++ {
			w.Step()
		}
		return w.Stats()
	}
	if many, none := run(120), run(0); many != none {
		t.Fatal("a flat world ran differently for being asked for stones it has nowhere to put")
	}
}

// They lie on the broken ground and nowhere else: where the ammunition is is
// a fact about the map.
func TestStonesLieOnBrokenGround(t *testing.T) {
	w := NewWorld(stoneConfig())
	n := 0
	for _, f := range w.Foods() {
		if f.Kind != FoodStone {
			continue
		}
		n++
		if got := w.terrainAt(f.X, f.Y).Kind; got != GroundRough {
			t.Fatalf("a stone is lying on ground of kind %v", got)
		}
	}
	if n != 20 {
		t.Fatalf("the world was given 20 stones and holds %d", n)
	}
}

// Nothing eats one, and the rules about eating have not learned the word.
func TestNothingEatsAStone(t *testing.T) {
	cfg := stoneConfig()
	w := NewWorld(cfg)
	human := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Genome: filledGenome(50)}))
	enemy := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 300, Y: 300,
		Genome: filledGenome(50), Species: SpeciesEnemy}))
	stone := Food{X: 100, Y: 100, Kind: FoodStone}

	if w.canEat(human, &stone) || w.canEat(enemy, &stone) {
		t.Fatal("somebody is eating a stone")
	}
	// The diet ledger counts what can be eaten, and a stone is not among it:
	// a world with two edible kinds still scores an even split of them as a
	// varied diet.
	if got := w.kindsOnOffer(); got != int(NumEdibleKinds)-1 {
		t.Fatalf("a world with no fish offers %d kinds of food", got)
	}
}

// One can be picked up, and it takes a hand exactly as a plant does: a body
// carrying a stone is a body not carrying its dinner.
func TestAStoneTakesAHand(t *testing.T) {
	cfg := stoneConfig()
	cfg.CarryCapacity = 1
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Hunger: 50, Genome: genomeOf(50, 50, 50)}))
	id := w.putFood(Food{X: 101, Y: 100, Kind: FoodStone})

	w.take(a, id)
	if a.CarriedCount() != 1 {
		t.Fatalf("holding %d after picking up a stone", a.CarriedCount())
	}
	if w.foodByID(id) != nil {
		t.Fatal("the stone is in the world and in a hand at once")
	}
	// And there is no room for anything else.
	w.take(a, w.addFood(102, 100))
	if a.CarriedCount() != 1 {
		t.Fatal("a stone and a dinner fitted in one hand")
	}
	// Dying puts it back, like everything else held.
	w.kill(a)
	if a.CarriedCount() != 0 {
		t.Fatal("the stone went out of the world with the body")
	}
}

// They do not take the room the plants grow in. A world full of stones still
// grows what it always grew - which is not what happened the first time they
// went in, and is why the check that guards the growing counts what grows.
func TestStonesDoNotCrowdOutThePlants(t *testing.T) {
	grown := func(stones int) int {
		cfg := stoneConfig()
		cfg.Stones = stones
		cfg.InitialPopulation, cfg.InitialEnemies, cfg.EnemySpawnTicks = 0, 0, 0
		cfg.MaxFoodItems = 30
		w := NewWorld(cfg)
		for i := 0; i < 3000; i++ {
			w.Step()
		}
		return w.countKind(FoodPlant)
	}
	if few, many := grown(0), grown(120); few != many {
		t.Fatalf("a world with 120 stones grew %d plants and one with none grew %d", many, few)
	}
}

// A body sees them, and they are kept apart from the food: what is edible is
// what says how contested a patch is and how rich the ground looks.
func TestAStoneIsSeenButIsNotFood(t *testing.T) {
	cfg := stoneConfig()
	w := NewWorld(cfg)
	// Standing on the broken ground, where the stones are.
	x, y := cfg.Width*0.45, cfg.Height*0.5
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: x, Y: y, Vitality: 90,
		Genome: genomeOf(50, 50, 50)}))
	w.putFood(Food{X: x + 5, Y: y, Kind: FoodStone})

	p := w.perceive(a)
	found := false
	for _, s := range p.Stones {
		if s.Kind == FoodStone {
			found = true
		}
	}
	if !found {
		t.Fatalf("a body standing next to a stone sees %d of them", len(p.Stones))
	}
	for _, f := range p.Foods {
		if f.Kind == FoodStone {
			t.Fatal("a stone turned up among the food")
		}
	}
}
