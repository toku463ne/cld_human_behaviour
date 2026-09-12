package engine

import (
	"math"
	"testing"
)

// One sort is the world as it was, down to the values taken from the random
// source: nothing is chosen when there is nothing to choose between.
func TestOneKindOfEnemyIsTheWorldAsItWas(t *testing.T) {
	run := func(kinds []EnemyKind) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 41
		cfg.EnemyKinds = kinds
		w := NewWorld(cfg)
		for i := 0; i < 1500; i++ {
			w.Step()
		}
		return w.Stats()
	}

	plain := run(nil)
	// A table with one row saying exactly what the world already said.
	same := run([]EnemyKind{{
		Name: "enemy", Share: 1,
		BudgetMean: DefaultConfig().EnemyBudgetMean,
		BudgetStd:  DefaultConfig().EnemyBudgetStd,
		Homing:     1,
	}})
	if plain != same {
		t.Fatalf("spelling out the only sort changed the run: %+v against %+v", same, plain)
	}
	// A row left blank falls back to the world's figures, so it is the same
	// world again.
	if blank := run([]EnemyKind{{Name: "enemy", Share: 1, Homing: 1}}); blank != plain {
		t.Fatalf("a row with nothing on it changed the run: %+v against %+v", blank, plain)
	}
}

// Two sorts arrive in the proportion the table asks for and are built the way
// it says. Nothing else in the world knows there are two.
func TestTwoKindsArriveAsTheTableSays(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 43
	cfg.EnemyKinds = []EnemyKind{
		{Name: "stray", Share: 3, BudgetMean: 380, BudgetStd: 60, Homing: 0},
		{Name: "brute", Share: 1, BudgetMean: 700, BudgetStd: 90, Homing: 1},
	}
	w := NewWorld(cfg)
	for i := 0; i < 8000; i++ {
		w.Step()
	}

	var counts [2]float64
	var budgets [2]float64
	for _, a := range w.Agents() {
		if a.Species != SpeciesEnemy {
			continue
		}
		counts[a.Kind]++
		budgets[a.Kind] += a.Budget()
	}
	if counts[0] == 0 || counts[1] == 0 {
		t.Fatalf("one of the sorts never turned up: %v", counts)
	}
	if budgets[0]/counts[0] >= budgets[1]/counts[1] {
		t.Fatalf("the heavy sort is not heavier: %v against %v",
			budgets[1]/counts[1], budgets[0]/counts[0])
	}

	use := w.Kinds()
	if use.Kinds != 2 {
		t.Fatalf("the world reports %d sorts, want 2", use.Kinds)
	}
	if use.BudgetGap <= 1.2 {
		t.Fatalf("the two rows produced bodies %v apart, want clearly different", use.BudgetGap)
	}
}

// Adding a sort is a row and no code. This is the test that says so: three
// rows, none of them special-cased anywhere, and all three turn up.
func TestAThirdKindNeedsNothingButARow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 47
	cfg.EnemySpawnTicks = 150 // enough arrivals to see all three
	cfg.EnemyKinds = []EnemyKind{
		{Name: "stray", Share: 1, BudgetMean: 380, Homing: 0},
		{Name: "brute", Share: 1, BudgetMean: 700, Homing: 1},
		{Name: "skulker", Share: 1, BudgetMean: 520, Homing: 0.5},
	}
	w := NewWorld(cfg)

	seen := map[uint8]bool{}
	for i := 0; i < 8000; i++ {
		w.Step()
		for _, a := range w.Agents() {
			if a.Species == SpeciesEnemy {
				seen[a.Kind] = true
			}
		}
	}
	for k := uint8(0); k < 3; k++ {
		if !seen[k] {
			t.Fatalf("sort %d never appeared", k)
		}
	}
	if got := w.Kinds().Kinds; got != 3 {
		t.Fatalf("the world reports %d sorts, want 3", got)
	}
}

// A kind is not a species: the sorts do meet, and their young are one sort or
// the other rather than something in between. Which one is the same coin the
// budget is taken with.
func TestAYoungOneIsOneOfItsParentsSorts(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 53
	cfg.EnemyKinds = []EnemyKind{
		{Name: "stray", Share: 1, BudgetMean: 380, Homing: 0},
		{Name: "brute", Share: 1, BudgetMean: 700, Homing: 1},
	}
	w := NewWorld(cfg)
	for i := 0; i < 8000; i++ {
		w.Step()
	}
	for _, a := range w.Agents() {
		if a.Species != SpeciesEnemy || a.Generation == 0 || len(a.ParentIDs) == 0 {
			continue
		}
		fromAParent, known := false, false
		for _, id := range a.ParentIDs {
			p, ok := w.AgentByID(id)
			if !ok {
				continue
			}
			if p.Kind == a.Kind {
				fromAParent = true
			}
			known = true
		}
		if known && !fromAParent {
			t.Fatalf("a %d was born to parents of other sorts", a.Kind)
		}
	}
}

// The one figure that has to mean the same thing whatever the table's length
// is: how far the population's make-up is from what was asked for.
func TestTheMixIsTheOneTheTableAskedFor(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 59
	cfg.EnemySpawnTicks = 150
	cfg.EnemyKinds = []EnemyKind{
		{Name: "stray", Share: 1, Homing: 0},
		{Name: "brute", Share: 1, Homing: 0},
	}
	w := NewWorld(cfg)
	for i := 0; i < 12000; i++ {
		w.Step()
	}
	if got := w.Kinds().MixError; math.Abs(got) > 0.2 {
		t.Fatalf("the population is %v away from the shares asked for", got)
	}
}
