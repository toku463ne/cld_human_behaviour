package engine

import "testing"

func kindConfig() Config {
	cfg := testConfig()
	cfg.FoodSpawnRate = 1
	cfg.MaxFoodItems = 100000
	return cfg
}

func TestAWorldWithNoPlantKindsIsUnchanged(t *testing.T) {
	// The table has to be out of the run entirely when it is empty, down to
	// how many numbers have been taken from the random source.
	a := grow(kindConfig(), 400)
	cfg := kindConfig()
	cfg.PlantKinds = nil
	b := grow(cfg, 400)
	if a.draws.draws != b.draws.draws {
		t.Fatalf("%d draws against %d", a.draws.draws, b.draws.draws)
	}
	for _, f := range a.Foods() {
		if f.Genes.Strain != 0 {
			t.Fatal("a plant is tagged with a kind in a world that has none")
		}
	}
	if w := a.PlantStrains(); w != nil {
		t.Fatalf("PlantStrains is %v, want nothing", w)
	}
}

func TestTheKindsComeUpInProportionToTheirShare(t *testing.T) {
	cfg := kindConfig()
	cfg.PlantKinds = []PlantKind{
		{Name: "berry", Share: 3},
		{Name: "root", Share: 1},
	}
	w := grow(cfg, 4000)
	got := w.PlantStrains()
	if len(got) != 2 {
		t.Fatalf("%d kinds counted, want 2", len(got))
	}
	ratio := float64(got[0]) / float64(got[1])
	if ratio < 2.5 || ratio > 3.5 {
		t.Fatalf("%d berries against %d roots (ratio %.2f, want about 3)", got[0], got[1], ratio)
	}
}

func TestARowWithNoShareNeverComesUp(t *testing.T) {
	cfg := kindConfig()
	cfg.PlantKinds = []PlantKind{
		{Name: "common", Share: 1},
		{Name: "never", Share: 0},
	}
	w := grow(cfg, 500)
	if got := w.PlantStrains(); got[1] != 0 {
		t.Fatalf("%d of a kind whose share is zero", got[1])
	}
}

func TestAKindIsWhereItsGenesStart(t *testing.T) {
	cfg := kindConfig()
	cfg.PlantDefence = true
	// No mutation, so that what a seedling inherits is exactly what its
	// parent had and the kind's figures can be checked on every plant. With
	// mutation on they drift, which is the point of them being genes.
	cfg.PlantMutationRate = 0
	cfg.PlantKinds = []PlantKind{
		{Name: "deadly", Share: 1, Poison: 0.9, Signal: 0.1},
	}
	w := grow(cfg, 50)
	for _, f := range w.Foods() {
		if f.Kind != FoodPlant {
			continue
		}
		// The first plants of a world are what the kind says. What their
		// seedlings become is up to the world.
		if f.Genes.Poison != 0.9 || f.Genes.Signal != 0.1 {
			t.Fatalf("a deadly plant came up at poison %.2f signal %.2f",
				f.Genes.Poison, f.Genes.Signal)
		}
	}
}

func TestASeedlingKeepsItsParentsKind(t *testing.T) {
	// The tag rides with the genes, so a kind does not dissolve in a
	// generation - which is what would make "this country grows berries" true
	// of the first plant only.
	parent := plantGenes{Spread: 10, Regrow: 1, Strain: 2}
	cfg := kindConfig()
	cfg.PlantMutationRate = 1
	cfg.PlantGenetics = true
	w := NewWorld(cfg)
	child := w.inheritPlantGenes(parent)
	if child.Strain != 2 {
		t.Fatalf("the seedling is kind %d, want 2", child.Strain)
	}
}

func TestTheSpreadIsAroundTheKindsOwnFigures(t *testing.T) {
	// With PlantGenetics on, a kind that seeds twice as far as the world's
	// figure has to come out scattered around its own number, not the
	// world's, or the table would say nothing at all.
	cfg := kindConfig()
	cfg.PlantGenetics = true
	cfg.PlantSpreadMax = 1000
	cfg.PlantKinds = []PlantKind{{Name: "far", Share: 1, Spread: 200}}
	w := grow(cfg, 200)
	sum, n := 0.0, 0.0
	for _, f := range w.Foods() {
		if f.Kind == FoodPlant {
			sum += f.Genes.Spread
			n++
		}
	}
	if n == 0 {
		t.Fatal("no plants")
	}
	if mean := sum / n; mean < 120 || mean > 280 {
		t.Fatalf("mean spread %.0f, want it scattered around the kind's 200", mean)
	}
}
