package engine

import "testing"

// The editor's hands change the world and nothing else: the same tick, the
// same bodies, and the same next number out of the generator.
func TestEditingTheGroundChangesTheGroundAndNothingElse(t *testing.T) {
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	w := NewWorld(cfg)
	for i := 0; i < 50; i++ {
		w.Step()
	}
	pop, tick, draws := w.Stats().Population, w.Tick(), w.draws.draws

	w.SetTerrain([]string{"....::::", "....::::", "....::::"})
	if got := w.TerrainAt(700, 300); got.Kind != GroundRough {
		t.Fatalf("the east is %v after being painted rough", got.Kind)
	}
	if got := w.TerrainAt(100, 300); got.Kind != GroundOpen {
		t.Fatalf("the west is %v and should have been left alone", got.Kind)
	}
	if w.Stats().Population != pop || w.Tick() != tick || w.draws.draws != draws {
		t.Fatal("laying down country moved the world on")
	}
	// And it is in what the world would save, so an edited world can be handed on.
	if got := w.Terrain(); len(got) != 3 {
		t.Fatalf("the map reads back as %d rows", len(got))
	}

	// Flat again.
	w.SetTerrain(nil)
	if got := w.TerrainAt(700, 300); got.Cost != 1 {
		t.Fatalf("the world is not flat again: cost %v", got.Cost)
	}
}

// A region's share of the plants is a share: raising one lowers what the rest
// are worth of the same total.
func TestPaintingARegionMovesTheFoodWithoutMakingAny(t *testing.T) {
	w := NewWorld(quietConfig())
	before := w.foodWeight
	i := w.RegionAt(100, 100)
	if err := w.SetRegion(i, 0.5, 2); err != nil {
		t.Fatal(err)
	}
	if w.regions[i].Food != 2 || w.regions[i].Shelter != 0.5 {
		t.Fatalf("region %d reads %+v", i, w.regions[i])
	}
	if w.foodWeight <= before {
		t.Fatal("the total weight did not follow the change")
	}
	// The world still grows the same number of plants a tick: the weight is
	// only how they are shared out (FoodSpawnRate is untouched).
	if w.cfg.FoodSpawnRate != quietConfig().FoodSpawnRate {
		t.Fatal("painting a region changed how much food the world makes")
	}
	if err := w.SetRegion(999, 1, 1); err == nil {
		t.Fatal("painted a region that does not exist")
	}
}

// The short list reports where each rule stands and whether that is inside the
// range it has been measured in - and a value outside is set, with a warning,
// rather than refused.
func TestTunablesWarnRatherThanRefuse(t *testing.T) {
	w := NewWorld(quietConfig())
	list := w.Tunables()
	if len(list) != int(numTunables) {
		t.Fatalf("%d rules on offer, want %d", len(list), numTunables)
	}
	for _, tn := range list {
		if tn.Name == "" || tn.About == "" {
			t.Fatalf("a rule with no name or no explanation: %+v", tn)
		}
	}

	safe, err := w.Tune(TuneFoodSpawnRate, 0.2)
	if err != nil || !safe {
		t.Fatalf("0.2 food is inside the measured range: safe=%v err=%v", safe, err)
	}
	safe, err = w.Tune(TuneFoodSpawnRate, 5)
	if err != nil {
		t.Fatal(err)
	}
	if safe {
		t.Fatal("five plants a tick was called safe")
	}
	if w.cfg.FoodSpawnRate != 5 {
		t.Fatal("the value was refused rather than warned about")
	}
	if _, err := w.Tune(Tunable(99), 1); err == nil {
		t.Fatal("changed a rule that does not exist")
	}
}

// The difficulty presets are the budget and nothing else: the rules a world is
// measured by are not what a game turns down.
func TestDifficultyMovesOnlyTheBudget(t *testing.T) {
	w := NewWorld(quietConfig())
	before := w.cfg
	if !w.SetDifficulty("hard") {
		t.Fatal("no hard preset")
	}
	if w.cfg.GeneBudgetMean >= before.GeneBudgetMean {
		t.Fatalf("hard gives %v, was %v", w.cfg.GeneBudgetMean, before.GeneBudgetMean)
	}
	after := w.cfg
	after.GeneBudgetMean, after.GeneBudgetStd = before.GeneBudgetMean, before.GeneBudgetStd
	if after.FoodSpawnRate != before.FoodSpawnRate || after.CompetitionWeight != before.CompetitionWeight ||
		after.MutationRate != before.MutationRate || after.SkirmishTicks != before.SkirmishTicks {
		t.Fatal("a difficulty changed a rule as well as the budget")
	}
	if w.SetDifficulty("impossible") {
		t.Fatal("accepted a difficulty nobody defined")
	}
	// The middle one is the world the measurements were taken in.
	if !w.SetDifficulty("measured") {
		t.Fatal("no measured preset")
	}
	std := DefaultConfig()
	if w.cfg.GeneBudgetMean != std.GeneBudgetMean || w.cfg.GeneBudgetStd != std.GeneBudgetStd {
		t.Fatalf("the measured preset is %v/%v and the default is %v/%v",
			w.cfg.GeneBudgetMean, w.cfg.GeneBudgetStd, std.GeneBudgetMean, std.GeneBudgetStd)
	}
}
