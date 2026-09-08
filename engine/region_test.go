package engine

import (
	"math"
	"testing"
)

// The arm this stage is measured against has to be exact, not merely similar:
// with no spread the whole world is ordinary ground and nothing is taken from
// the random source, so the run is the one from before regions existed.
func TestNoShelterSpreadLeavesTheWorldExactlyAsItWas(t *testing.T) {
	run := func(spread float64) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 5
		cfg.ShelterSpread = spread
		w := NewWorld(cfg)
		for i := 0; i < 1500; i++ {
			w.Step()
		}
		return w.Stats()
	}

	flat, alsoFlat := run(0), run(0)
	if flat != alsoFlat {
		t.Fatal("the same world twice gave different runs")
	}
	if varied := run(0.6); varied == flat {
		t.Fatal("giving the regions different shelter changed nothing at all")
	}

	// And with no spread every region really is ordinary.
	cfg := DefaultConfig()
	cfg.ShelterSpread = 0
	w := NewWorld(cfg)
	for _, r := range w.Regions() {
		if r.Shelter != 1 {
			t.Fatalf("a region has shelter %v with no spread, want 1", r.Shelter)
		}
	}
}

// A region is a patch of ground, not a wall and not a label. What it does is
// change one number at a place.
func TestRegionsCoverTheWorldAndOnlyChangeShelter(t *testing.T) {
	cfg := testConfig()
	cfg.RegionCols, cfg.RegionRows = 4, 3
	cfg.ShelterSpread = 0.6
	w := NewWorld(cfg)

	regions := w.Regions()
	if len(regions) != 12 {
		t.Fatalf("the world is in %d regions, want 12", len(regions))
	}

	// Every point of the world is in exactly one region, and the shelter it
	// reports is that region's.
	for _, r := range regions {
		midX, midY := (r.MinX+r.MaxX)/2, (r.MinY+r.MaxY)/2
		if got := w.shelterAt(midX, midY); got != r.Shelter {
			t.Fatalf("the middle of a region reports shelter %v, want %v", got, r.Shelter)
		}
	}
	// Including the far corners, which is where an off-by-one would show.
	for _, p := range [][2]float64{{0, 0}, {cfg.Width, cfg.Height}, {cfg.Width, 0}, {0, cfg.Height}} {
		if got := w.shelterAt(p[0], p[1]); got <= 0 {
			t.Fatalf("the corner (%v, %v) has no region", p[0], p[1])
		}
	}
	// The regions are not all alike, or there is nothing to find.
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, r := range regions {
		lo, hi = math.Min(lo, r.Shelter), math.Max(hi, r.Shelter)
	}
	if hi-lo < 0.2 {
		t.Fatalf("shelter runs from %v to %v, too flat to be a difference", lo, hi)
	}
}

// Sheltered ground makes lying down worth more, and it does it through the one
// term it is allowed to touch: the exposure of resting. With nobody around
// there is no exposure to discount, so the ground makes no difference at all -
// which is the check that it has not leaked into anything else.
func TestShelteredGroundIsOnlyWorthSomethingWhenSomebodyIsAround(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)

	restValue := func(shelter float64, neighbours int) float64 {
		p := &Perception{
			Tick: 1, Cfg: &cfg, Rand: w.rng,
			Self: SelfView{
				ID: 1, X: 200, Y: 200, Vitality: 40, Hunger: 10,
				MaxVitality: cfg.MaxVitality, MaxSpeed: cfg.MaxSpeed,
				HungerRate: cfg.HungerRate, RestRate: cfg.RegenRate, Shelter: shelter,
				Retaliation: cfg.Retaliation, AcceptChance: cfg.AcceptChance,
				RiskWeight: cfg.RiskWeight, CompetitionWeight: cfg.CompetitionWeight,
				ShockRisk: cfg.ShockRisk},
		}
		for i := 0; i < neighbours; i++ {
			p.Others = append(p.Others, AgentView{ID: 2 + i, Dist: 20, EstStrength: 70})
		}
		c := &AIController{}
		c.survey(p)
		c.addRest(p)
		return c.opts[0].util
	}

	covered, open := restValue(0.4, 3), restValue(1.6, 3)
	if covered <= open {
		t.Fatalf("resting scored %v under cover and %v in the open, want cover to be worth more", covered, open)
	}
	if alone, exposed := restValue(0.4, 0), restValue(1.6, 0); alone != exposed {
		t.Fatalf("with nobody around the ground still mattered: %v against %v", alone, exposed)
	}
}

// A moment with nobody lying down is no evidence about where the resting
// happens, and it must not be recorded as evidence that it happens on the
// worst ground there is. Averaging that mistake over a run reported a
// confident preference for shelter in a world where every region was the same.
func TestAnInstantWithNobodyRestingIsNotEvidence(t *testing.T) {
	cfg := quietConfig()
	cfg.ShelterSpread = 0.6
	w := NewWorld(cfg)
	w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80, Genome: genomeOf(50, 50, 50)})
	w.agents[0].Action = Action{Kind: ActMove}

	got := w.Shelter()
	if got.Resting != got.All {
		t.Fatalf("with nobody resting the shelter under the resting reads %v against %v overall, want no difference",
			got.Resting, got.All)
	}

	// And a world where every region is the same reports no preference,
	// whatever anybody is doing.
	flat := DefaultConfig()
	flat.Seed = 3
	flat.ShelterSpread = 0
	fw := NewWorld(flat)
	for i := 0; i < 3000; i++ {
		fw.Step()
		if s := fw.Shelter(); math.Abs(s.All-s.Resting) > 1e-9 {
			t.Fatalf("at tick %d a world of identical regions reported a shelter gain of %v",
				i, s.All-s.Resting)
		}
	}
}

// The regions change where the plants come up and never how many. That
// distinction is the whole safety of the stage: FoodSpawnRate is the most
// selection-sensitive figure in the world, so a rule that quietly altered the
// total would be measuring something else entirely.
func TestRegionsMoveTheFoodWithoutChangingHowMuchThereIs(t *testing.T) {
	count := func(spread float64) (total int, inRichest int, richest float64) {
		cfg := DefaultConfig()
		cfg.Seed = 11
		cfg.FoodSpread = spread
		cfg.MaxFoodItems = 100000 // let it pile up, so nothing is capped away
		cfg.InitialPopulation, cfg.InitialEnemies = 0, 0
		cfg.InitialFoodItems = 0
		w := NewWorld(cfg)
		for i := 0; i < 4000; i++ {
			w.Step()
		}
		best := 0
		for i := range w.regions {
			if w.regions[i].Food > w.regions[best].Food {
				best = i
			}
		}
		minX, minY, maxX, maxY := w.regionBounds(best)
		for _, f := range w.Foods() {
			total++
			if f.X >= minX && f.X < maxX && f.Y >= minY && f.Y < maxY {
				inRichest++
			}
		}
		return total, inRichest, w.regions[best].Food
	}

	flatTotal, flatBest, _ := count(0)
	variedTotal, variedBest, richest := count(0.6)

	// The same number of plants, give or take the odd one at the margin.
	if d := math.Abs(float64(flatTotal - variedTotal)); d > float64(flatTotal)*0.02 {
		t.Fatalf("a flat world grew %d plants and a varied one %d, want the same amount",
			flatTotal, variedTotal)
	}
	// But more of them on the best ground.
	if variedBest <= flatBest {
		t.Fatalf("the richest region got %d plants against a flat world's %d for the same block (its multiplier is %.2f)",
			variedBest, flatBest, richest)
	}
}

// With no spread the food is laid out exactly as it always was, down to the
// values taken from the random source.
func TestNoFoodSpreadLeavesTheWorldExactlyAsItWas(t *testing.T) {
	run := func(spread float64) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 9
		cfg.ShelterSpread = 0
		cfg.FoodSpread = spread
		w := NewWorld(cfg)
		for i := 0; i < 1500; i++ {
			w.Step()
		}
		return w.Stats()
	}
	if flat, again := run(0), run(0); flat != again {
		t.Fatal("the same world twice gave different runs")
	} else if varied := run(0.6); varied == flat {
		t.Fatal("moving the food about changed nothing at all")
	}
}

// Nobody of a kind standing anywhere is no evidence about where that kind
// stands. Stage 14 made this mistake with resting and it is not being made
// again.
func TestRichnessOfAKindThatIsNotThereIsNotZero(t *testing.T) {
	cfg := testConfig()
	cfg.FoodSpread = 0.6
	w := NewWorld(cfg)
	w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80, Genome: genomeOf(50, 50, 50)})

	got := w.Richness()
	if got.Enemies != got.All {
		t.Fatalf("with no predators alive their ground reads %v against %v overall, want no difference",
			got.Enemies, got.All)
	}
	// And a world of identical regions reports no preference for anybody.
	flat := DefaultConfig()
	flat.Seed = 4
	flat.FoodSpread = 0
	fw := NewWorld(flat)
	for i := 0; i < 2000; i++ {
		fw.Step()
		r := fw.Richness()
		if math.Abs(r.Humans-1) > 1e-9 || math.Abs(r.Enemies-1) > 1e-9 || math.Abs(r.All-1) > 1e-9 {
			t.Fatalf("at tick %d a world of identical regions reported richness %+v", i, r)
		}
	}
}

// --- where the food grows against the ground it grows on (stage 33) ---------

// linkedConfig lays a rough west and an open east under the region grid, so
// that whole regions are dear or cheap.
func linkedConfig(correlation float64) Config {
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	cfg.FoodSpread = 0 // the draw off, so what moves is this rule and not chance
	cfg.RoughMoveCost = 3
	cfg.TerrainFoodCorrelation = correlation
	cfg.TerrainMap = []string{
		"....::::",
		"....::::",
		"....::::",
	}
	return cfg
}

// With a positive correlation the dear ground grows less, and the cheap ground
// grows more by exactly as much: the world grows what it always did.
func TestHardGroundGrowsLessAndTheTotalIsUnchanged(t *testing.T) {
	flat := NewWorld(linkedConfig(0))
	tied := NewWorld(linkedConfig(1))

	var flatTotal, tiedTotal float64
	for i := range flat.regions {
		flatTotal += flat.regions[i].Food
		tiedTotal += tied.regions[i].Food
	}
	if math.Abs(flatTotal-tiedTotal) > 1e-9 {
		t.Fatalf("the world grows %v with the rule and %v without it", tiedTotal, flatTotal)
	}

	// West is open, east is rough.
	west := tied.regionIndexAt(100, 300)
	east := tied.regionIndexAt(700, 300)
	if !(tied.regions[east].Food < tied.regions[west].Food) {
		t.Fatalf("the rough ground grows %v and the open %v, want the rough to grow less",
			tied.regions[east].Food, tied.regions[west].Food)
	}
	// And the untied world has them level, which is what the rule is against.
	if math.Abs(flat.regions[east].Food-flat.regions[west].Food) > 1e-9 {
		t.Fatalf("without the rule the two differ: %v against %v",
			flat.regions[east].Food, flat.regions[west].Food)
	}
}

// The other sign says the other thing: the hard ground is where the food is.
func TestANegativeCorrelationPutsTheFoodOnTheHardGround(t *testing.T) {
	w := NewWorld(linkedConfig(-1))
	west, east := w.regionIndexAt(100, 300), w.regionIndexAt(700, 300)
	if !(w.regions[east].Food > w.regions[west].Food) {
		t.Fatalf("the rough ground grows %v and the open %v, want the rough to grow more",
			w.regions[east].Food, w.regions[west].Food)
	}
}

// A world with no map is untouched at any correlation: nothing is above or
// below the average when everywhere costs the same.
func TestAFlatWorldIsUntouchedByTheCorrelation(t *testing.T) {
	cfg := quietConfig()
	cfg.FoodSpread = 0.6
	plain := NewWorld(cfg)
	cfg.TerrainFoodCorrelation = 1
	tied := NewWorld(cfg)

	for i := range plain.regions {
		if plain.regions[i].Food != tied.regions[i].Food {
			t.Fatalf("region %d grows %v with the rule and %v without, in a world with no map",
				i, tied.regions[i].Food, plain.regions[i].Food)
		}
	}
	if plain.foodWeight != tied.foodWeight {
		t.Fatalf("the world's total is %v with the rule and %v without", tied.foodWeight, plain.foodWeight)
	}
}

// --- stage 36: the ground beside the water ---------------------------------

// A river down the middle, and the bank grows more than the rest - but the
// world grows exactly as much as it did. Moving the food and adding to it are
// different things, and only the first is what this stage does.
func TestWatersideGroundGrowsMoreWithoutGrowingMore(t *testing.T) {
	base := quietConfig()
	base.Width, base.Height = 800, 600
	base.TerrainMap = []string{
		"...~~...",
		"...~~...",
		"...~~...",
	}

	plain := NewWorld(base)
	rich := base
	rich.WatersideFood = 1
	wet := NewWorld(rich)

	var plainTotal, wetTotal float64
	for i := range plain.regions {
		plainTotal += plain.regions[i].Food
		wetTotal += wet.regions[i].Food
	}
	if math.Abs(plainTotal-wetTotal) > 1e-9 {
		t.Fatalf("the world grows %v with a rich bank against %v without: the total moved", wetTotal, plainTotal)
	}

	// And the food went where the water is.
	var onWater, dry float64
	var nWater, nDry float64
	for i := range wet.regions {
		if wet.regionWaterShare(i) > 0 {
			onWater += wet.regions[i].Food
			nWater++
		} else {
			dry += wet.regions[i].Food
			nDry++
		}
	}
	if nWater == 0 || nDry == 0 {
		t.Fatalf("the map has %v watery regions and %v dry ones", nWater, nDry)
	}
	if !(onWater/nWater > dry/nDry) {
		t.Fatalf("the bank grows %v per region against %v inland", onWater/nWater, dry/nDry)
	}
}

// Negative is the other landscape: the same rule with the sign turned over,
// and the same total.
func TestABarrenBankIsTheSameRuleTurnedOver(t *testing.T) {
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	cfg.TerrainMap = []string{"...~~...", "...~~...", "...~~..."}
	cfg.WatersideFood = -1
	w := NewWorld(cfg)

	var onWater, dry, nWater, nDry float64
	for i := range w.regions {
		if w.regionWaterShare(i) > 0 {
			onWater += w.regions[i].Food
			nWater++
		} else {
			dry += w.regions[i].Food
			nDry++
		}
	}
	if !(onWater/nWater < dry/nDry) {
		t.Fatalf("a barren bank grows %v per region against %v inland", onWater/nWater, dry/nDry)
	}
}

// A world with no map has no water in it, so the rule cannot fire however it
// is set - and, drawing no random number, such a world runs to the state it
// always did.
func TestAFlatWorldHasNoBank(t *testing.T) {
	run := func(side float64) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 11
		cfg.WatersideFood = side
		w := NewWorld(cfg)
		for i := 0; i < 400; i++ {
			w.Step()
		}
		return w.Stats()
	}
	if off, on := run(0), run(2); off != on {
		t.Fatalf("a flat world ran differently with a rich bank:\n off %+v\n on  %+v", off, on)
	}
}

// The two ways the ground can be tied to the food are separate rules and can
// be asked for together: hard country poor, and the bank rich, at once.
func TestTheBankAndTheGoingAreAskedForSeparately(t *testing.T) {
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	// Rough in the west, a river in the middle, open in the east.
	cfg.TerrainMap = []string{"::..~~..", "::..~~..", "::..~~.."}
	cfg.TerrainFoodCorrelation = 1
	cfg.WatersideFood = 1
	w := NewWorld(cfg)

	rough := w.regionIndexAt(50, 300)
	river := w.regionIndexAt(500, 300)
	open := w.regionIndexAt(750, 300)
	if rough == river || river == open {
		t.Fatal("the three pieces of country are not three regions")
	}
	// Stage 33 makes dear ground poor, which the river is; stage 36 gives the
	// bank some of it back. What must hold is that they are two rules and not
	// one: the rough is poorer than the open (33 is doing its work), and the
	// same river is richer with the bank asked for than without it (36 is
	// doing its own, on top).
	if !(w.regions[rough].Food < w.regions[open].Food) {
		t.Fatalf("rough %v is not poorer than open %v", w.regions[rough].Food, w.regions[open].Food)
	}
	cfg.WatersideFood = 0
	without := NewWorld(cfg)
	if !(w.regions[river].Food > without.regions[river].Food) {
		t.Fatalf("the bank grows %v with stage 36 and %v without it",
			w.regions[river].Food, without.regions[river].Food)
	}
}
