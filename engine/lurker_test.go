package engine

import "testing"

// riverMap is a piece of country with water down the middle.
func riverMap() []string {
	return []string{
		"....~~....",
		"....~~....",
		"....~~....",
	}
}

// A creature of the water arrives in the water, and the water does not take
// it. Both halves are the one flag, because they are the one fact.
func TestALurkerArrivesInTheWaterAndIsNotDrownedByIt(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 89
	cfg.TerrainMap = riverMap()
	cfg.InitialEnemies = 40
	cfg.EnemyKinds = []EnemyKind{{Name: "lurker", Share: 1, Homing: 1, Water: true}}
	w := NewWorld(cfg)

	var wet, dry int
	for i := range w.agents {
		a := &w.agents[i]
		if a.Species != SpeciesEnemy {
			continue
		}
		if w.terrainAt(a.X, a.Y).Kind == GroundWater {
			wet++
		} else {
			dry++
		}
		if got := w.drownChanceFor(a, w.terrainAt(a.X, a.Y)); got != 0 {
			t.Fatalf("the river may drown what lives in it: %v", got)
		}
	}
	if wet == 0 || dry > 0 {
		t.Fatalf("%d of the lurkers came up in the water and %d on dry land", wet, dry)
	}

	// A human in the same river is still at risk in it.
	id := w.addAgent(Agent{Maturity: 1, X: w.water[0].x, Y: w.water[0].y,
		Genome: filledGenome(50)})
	human := mustAgent(t, w, id)
	if got := w.drownChanceFor(human, w.terrainAt(human.X, human.Y)); got <= 0 {
		t.Fatal("the river stopped drowning people")
	}
}

// The fish are food items and a lurker is a body, and the two do not meet
// (#95): a world with fish in it runs exactly as it did.
func TestTheFishAreUntouched(t *testing.T) {
	run := func(kinds []EnemyKind) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 97
		cfg.TerrainMap = riverMap()
		cfg.FishShare = 0.25
		cfg.EnemyKinds = kinds
		w := NewWorld(cfg)
		for i := 0; i < 1500; i++ {
			w.Step()
		}
		return w.Stats()
	}
	dry := run([]EnemyKind{{Name: "brute", Share: 1, Homing: 1}})
	if dry != run([]EnemyKind{{Name: "brute", Share: 1, Homing: 1}}) {
		t.Fatal("the same world twice gave different runs")
	}
	if wet := run([]EnemyKind{{Name: "lurker", Share: 1, Homing: 1, Water: true}}); wet == dry {
		t.Fatal("putting the arrivals in the river changed nothing at all")
	}
}

// A map with no water cannot hold one, and a row that asks for it anyway
// arrives like any other sort - dropping the arrival would change how many
// enemies the world has without saying so.
func TestALurkerInADryWorldArrivesLikeAnyOther(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 101
	cfg.InitialEnemies = 20
	cfg.EnemyKinds = []EnemyKind{{Name: "lurker", Share: 1, Homing: 1, Water: true}}
	w := NewWorld(cfg)

	n := 0
	for i := range w.agents {
		if w.agents[i].Species == SpeciesEnemy {
			n++
		}
	}
	if n != cfg.InitialEnemies {
		t.Fatalf("%d of the %d asked-for enemies arrived", n, cfg.InitialEnemies)
	}
}
