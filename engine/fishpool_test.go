package engine

import (
	"math"
	"testing"
)

// fishConfig is a world with nobody in it, laid on a map that is half water,
// so a test can watch what the water grows.
func fishConfig() Config {
	cfg := testConfig()
	cfg.TerrainMap = []string{
		"....~~~~",
		"....~~~~",
		"....~~~~",
		"....~~~~",
	}
	cfg.FoodSpawnRate = 1
	cfg.MaxFoodItems = 100000
	return cfg
}

func countFood(w *World) (plants, fish int) {
	for _, f := range w.Foods() {
		switch f.Kind {
		case FoodPlant:
			plants++
		case FoodFish:
			fish++
		}
	}
	return
}

func TestWithoutItsOwnPoolAFishTakesAPlantsPlace(t *testing.T) {
	// The world before this: FishShare of the one crop comes up wet, and the
	// two together are what FoodSpawnRate said.
	cfg := fishConfig()
	cfg.FishShare = 0.25
	w := grow(cfg, 400)
	plants, fish := countFood(w)
	if fish == 0 {
		t.Fatal("no fish at all")
	}
	if got, want := plants+fish, 400; got != want {
		t.Fatalf("the world grew %d things, want %d", got, want)
	}
}

func TestItsOwnPoolIsExtraFoodAndSaysSo(t *testing.T) {
	// The deliberate loosening: the water grows its own crop beside the
	// land's, so the world holds more than FoodSpawnRate alone says.
	cfg := fishConfig()
	cfg.FishShare = 0.25 // not read once the pool is on
	cfg.FishSpawnRate = 0.5
	w := grow(cfg, 400)
	plants, fish := countFood(w)
	if got, want := plants, 400; got != want {
		t.Fatalf("the land grew %d plants, want the full %d", got, want)
	}
	if got, want := fish, 200; got != want {
		t.Fatalf("the water grew %d fish, want %d", got, want)
	}
}

func TestTheTwoPoolsHaveTheirOwnCeilings(t *testing.T) {
	cfg := fishConfig()
	cfg.MaxFoodItems = 30
	cfg.MaxFishItems = 10
	cfg.FishSpawnRate = 1
	w := grow(cfg, 500)
	plants, fish := countFood(w)
	if plants != 30 {
		t.Fatalf("%d plants against a ceiling of 30", plants)
	}
	if fish != 10 {
		t.Fatalf("%d fish against a ceiling of 10", fish)
	}
}

func TestAWorldWithNoFishPoolIsUnchanged(t *testing.T) {
	cfg := fishConfig()
	cfg.FishShare = 0.25
	a := grow(cfg, 300)
	cfg2 := fishConfig()
	cfg2.FishShare = 0.25
	cfg2.FishSpawnRate, cfg2.MaxFishItems, cfg2.FishRichMap = 0, 0, nil
	b := grow(cfg2, 300)
	if a.draws.draws != b.draws.draws {
		t.Fatalf("%d draws against %d", a.draws.draws, b.draws.draws)
	}
	for i := range a.Foods() {
		if a.Foods()[i].X != b.Foods()[i].X || a.Foods()[i].Kind != b.Foods()[i].Kind {
			t.Fatalf("item %d is not where it was", i)
		}
	}
}

func TestTheWaterMayBePaintedRich(t *testing.T) {
	// The eastern half is water; paint its northern half rich and its
	// southern half poor, and the fish follow.
	cfg := fishConfig()
	cfg.FishSpawnRate = 1
	cfg.FishRichMap = []string{"99999999", "99999999", "11111111", "11111111"}
	w := grow(cfg, 2000)
	north, south := 0, 0
	for _, f := range w.Foods() {
		if f.Kind != FoodFish {
			continue
		}
		if f.Y < cfg.Height/2 {
			north++
		} else {
			south++
		}
	}
	if south == 0 {
		t.Fatal("nothing came up in the poorer water")
	}
	if ratio := float64(north) / float64(south); math.Abs(ratio-9) > 2.5 {
		t.Fatalf("%d fish north against %d south (ratio %.1f, want about 9)", north, south, ratio)
	}
}

func TestPaintingTheLandDoesNothingToTheFish(t *testing.T) {
	// The western half is dry. A richness painted there is never read,
	// because the draw is over water cells.
	cfg := fishConfig()
	cfg.FishSpawnRate = 1
	cfg.FishRichMap = []string{"90000000", "90000000", "90000000", "90000000"}
	w := grow(cfg, 300)
	_, fish := countFood(w)
	if fish == 0 {
		t.Fatal("a richness painted on dry land stopped the water growing")
	}
}
