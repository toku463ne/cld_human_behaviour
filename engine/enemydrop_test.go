package engine

import "testing"

// carcassConfig is a world with one human and one enemy in it and nothing else
// moving, so a test can kill the enemy and count what it leaves.
func carcassConfig() Config {
	cfg := quietConfig()
	cfg.MeatPerBudget = 100
	return cfg
}

// meatLeftBy kills the agent and counts the items its carcass became.
func meatLeftBy(t *testing.T, w *World, a *Agent) int {
	t.Helper()
	before := 0
	for _, f := range w.Foods() {
		if f.Kind == FoodMeat {
			before++
		}
	}
	w.dropMeat(a)
	after := 0
	for _, f := range w.Foods() {
		if f.Kind == FoodMeat {
			after++
		}
	}
	return after - before
}

func TestEverySortLeavesTheSameMeatUntilASortSaysOtherwise(t *testing.T) {
	cfg := carcassConfig()
	cfg.EnemyKinds = []EnemyKind{
		{Name: "plain", Share: 1},
		{Name: "also plain", Share: 1},
	}
	w := NewWorld(cfg)
	a := w.randomAgent(SpeciesEnemy)
	a.Kind = 0
	idA := w.addAgent(a)
	b := w.randomAgent(SpeciesEnemy)
	b.Kind = 1
	b.Genome = append([]float64(nil), a.Genome...)
	idB := w.addAgent(b)
	one := meatLeftBy(t, w, mustAgent(t, w, idA))
	two := meatLeftBy(t, w, mustAgent(t, w, idB))
	if one != two {
		t.Fatalf("two sorts of the same size left %d and %d items", one, two)
	}
	if one == 0 {
		t.Fatal("no meat at all")
	}
}

func TestASortMayBeWorthMoreMeatThanItsSize(t *testing.T) {
	cfg := carcassConfig()
	cfg.EnemyKinds = []EnemyKind{
		{Name: "lean", Share: 1},
		{Name: "fat", Share: 1, Meat: 3},
	}
	w := NewWorld(cfg)
	lean := w.randomAgent(SpeciesEnemy)
	lean.Kind = 0
	idLean := w.addAgent(lean)
	fat := w.randomAgent(SpeciesEnemy)
	fat.Kind = 1
	fat.Genome = append([]float64(nil), lean.Genome...)
	idFat := w.addAgent(fat)
	thin := meatLeftBy(t, w, mustAgent(t, w, idLean))
	heavy := meatLeftBy(t, w, mustAgent(t, w, idFat))
	if thin == 0 {
		t.Fatal("the lean one left nothing")
	}
	if heavy < thin*2 {
		t.Fatalf("the fat one left %d items against the lean one's %d, want about three times", heavy, thin)
	}
}

func TestAHumanIsNeverReadAsASort(t *testing.T) {
	// kindOf hands back the first row for anything that is not an enemy of a
	// named sort, humans included. A generous first row must not make human
	// carcasses generous too.
	cfg := carcassConfig()
	cfg.EnemyKinds = []EnemyKind{{Name: "fat", Share: 1, Meat: 5}}
	w := NewWorld(cfg)
	h := w.randomAgent(SpeciesHuman)
	body := mustAgent(t, w, w.addAgent(h))
	want := int(body.Bulk(&cfg) / cfg.MeatPerBudget)
	if got := meatLeftBy(t, w, body); got != want {
		t.Fatalf("a human left %d items, want %d - the sort's multiplier reached it", got, want)
	}
}

// crossConfig is a world laid on a cliff with no ramp: level ground on the
// left, one level up on the right, and nothing in between.
func crossConfig() Config {
	cfg := quietConfig()
	cfg.TerrainMap = []string{"..11", "..11", "..11", "..11"}
	return cfg
}

// enemyOfKind puts one enemy of the given row into the world and returns it.
func enemyOfKind(t *testing.T, w *World, kind uint8, x, y float64) *Agent {
	t.Helper()
	a := w.randomAgent(SpeciesEnemy)
	a.Kind = kind
	a.X, a.Y = x, y
	return mustAgent(t, w, w.addAgent(a))
}

func TestAWalkerIsStoppedByACliff(t *testing.T) {
	cfg := crossConfig()
	cfg.EnemyKinds = []EnemyKind{{Name: "walker", Share: 1}}
	w := NewWorld(cfg)
	a := enemyOfKind(t, w, 0, cfg.Width*0.3, cfg.Height/2)
	if w.canStep(a, a.X, a.Y, cfg.Width*0.7, a.Y) {
		t.Fatal("a walker stepped up a cliff with no ramp")
	}
}

func TestAClimberGoesUpACliffAndPaysForTheGroundLikeAnyoneElse(t *testing.T) {
	cfg := crossConfig()
	cfg.RoughMoveCost = 4
	cfg.TerrainMap = []string{"..11", "::11", "..11", "..11"}
	cfg.EnemyKinds = []EnemyKind{{Name: "climber", Share: 1, Climbs: true}}
	w := NewWorld(cfg)
	a := enemyOfKind(t, w, 0, cfg.Width*0.3, cfg.Height/2)
	if !w.canStep(a, a.X, a.Y, cfg.Width*0.7, a.Y) {
		t.Fatal("a climber was stopped by a cliff")
	}
	// Broken country still costs it what it costs anybody.
	rough := w.terrainAt(cfg.Width*0.1, cfg.Height*0.375)
	if got := w.groundCostFor(a, rough); got != rough.Cost {
		t.Fatalf("a climber pays %v on ground that costs %v", got, rough.Cost)
	}
}

func TestAFlierIgnoresTheGroundButNotItsOwnPrice(t *testing.T) {
	cfg := crossConfig()
	cfg.TerrainMap = []string{"..~~", "..~~", "..33", "..33"}
	cfg.DrownChancePerTick = 0.5
	cfg.WaterSpeedShare = 0.25
	cfg.WaterDrain = 3
	cfg.EnemyKinds = []EnemyKind{{Name: "flier", Share: 1, Flies: true, FlyCost: 2}}
	w := NewWorld(cfg)
	a := enemyOfKind(t, w, 0, cfg.Width*0.7, cfg.Height*0.25) // over the water
	water := w.terrainAt(a.X, a.Y)
	if water.Kind != GroundWater {
		t.Fatalf("the test is not over water: %v", water.Kind)
	}
	// A level is nothing to it, however many at once.
	if !w.canStep(a, a.X, a.Y, cfg.Width*0.7, cfg.Height*0.875) {
		t.Fatal("a flier was stopped by three levels")
	}
	// It runs no risk, is not dragged, is not drained...
	if got := w.drownChanceFor(a, water); got != 0 {
		t.Fatalf("a flier drowns at %v", got)
	}
	if got := w.groundSpeedFor(a, water); got != 1 {
		t.Fatalf("a flier is dragged to %v", got)
	}
	if got := w.soakOf(a); got != 0 {
		t.Fatalf("a flier is drained by %v", got)
	}
	// ... and pays its own price, which is not nothing.
	if got := w.groundCostFor(a, water); got != 2 {
		t.Fatalf("a flier pays %v, want its own 2", got)
	}
}

func TestAFlierWithNoPriceOfItsOwnStillPaysForOpenGround(t *testing.T) {
	// No ground is impassable (stage 20), and its mirror: none is free.
	cfg := crossConfig()
	cfg.EnemyKinds = []EnemyKind{{Name: "flier", Share: 1, Flies: true}}
	w := NewWorld(cfg)
	a := enemyOfKind(t, w, 0, cfg.Width*0.3, cfg.Height/2)
	if got := w.groundCostFor(a, w.terrainAt(a.X, a.Y)); got != 1 {
		t.Fatalf("a flier with no price of its own pays %v, want 1", got)
	}
}

func TestHowABodyGetsAboutIsAnEnemysBusinessOnly(t *testing.T) {
	// kindOf hands back the first row for anything that is not an enemy of a
	// named sort. A world of fliers must not let the humans fly.
	cfg := crossConfig()
	cfg.EnemyKinds = []EnemyKind{{Name: "flier", Share: 1, Flies: true}}
	w := NewWorld(cfg)
	h := w.randomAgent(SpeciesHuman)
	h.X, h.Y = cfg.Width*0.3, cfg.Height/2
	a := mustAgent(t, w, w.addAgent(h))
	if w.canStep(a, a.X, a.Y, cfg.Width*0.7, a.Y) {
		t.Fatal("a human flew up a cliff")
	}
}

func TestAMapMaySayWhereEachSortComesIn(t *testing.T) {
	cfg := quietConfig()
	cfg.InitialEnemies = 0
	cfg.EnemyKinds = []EnemyKind{
		{Name: "west", Share: 1, Key: 'a'},
		{Name: "east", Share: 1, Key: 'b'},
	}
	cfg.EnemyKindMap = []string{"ab"}
	w := NewWorld(cfg)
	for i := 0; i < 200; i++ {
		a := w.randomAgent(SpeciesEnemy)
		want := cfg.Width / 2
		if a.Kind == 0 && a.X >= want {
			t.Fatalf("a western sort arrived at x=%.0f", a.X)
		}
		if a.Kind == 1 && a.X < want {
			t.Fatalf("an eastern sort arrived at x=%.0f", a.X)
		}
	}
}

func TestASortTheMapPaintsNowhereArrivesAsItAlwaysDid(t *testing.T) {
	cfg := quietConfig()
	cfg.InitialEnemies = 0
	cfg.EnemyKinds = []EnemyKind{
		{Name: "painted", Share: 1, Key: 'a'},
		{Name: "unpainted", Share: 1},
	}
	cfg.EnemyKindMap = []string{"a."}
	w := NewWorld(cfg)
	east := 0
	for i := 0; i < 200; i++ {
		a := w.randomAgent(SpeciesEnemy)
		if a.Kind == 1 && a.X >= cfg.Width/2 {
			east++
		}
		if a.Kind == 0 && a.X >= cfg.Width/2 {
			t.Fatalf("the painted sort left its country at x=%.0f", a.X)
		}
	}
	if east == 0 {
		t.Fatal("the unpainted sort never arrived in the east, so it is not free")
	}
}

func TestAWorldWithNoEnemyPaintingIsUnchanged(t *testing.T) {
	cfg := quietConfig()
	cfg.EnemyKinds = []EnemyKind{{Name: "one", Share: 1}}
	a := NewWorld(cfg)
	for i := 0; i < 50; i++ {
		a.Step()
	}
	cfg2 := quietConfig()
	cfg2.EnemyKinds = []EnemyKind{{Name: "one", Share: 1}}
	cfg2.EnemyKindMap = nil
	b := NewWorld(cfg2)
	for i := 0; i < 50; i++ {
		b.Step()
	}
	if a.draws.draws != b.draws.draws {
		t.Fatalf("%d draws against %d", a.draws.draws, b.draws.draws)
	}
}

func TestAMapMayNameTheBeastsItsCountryHolds(t *testing.T) {
	data := []byte(`{"width":2,"height":1,"tilewidth":32,"tileheight":32,
	 "layers":[
	  {"type":"tilelayer","name":"ground","width":2,"height":1,"data":[1,1]},
	  {"type":"tilelayer","name":"beasts","width":2,"height":1,"data":[2,3]}],
	 "tilesets":[{"firstgid":1,"name":"t","tiles":[
	   {"id":0,"properties":[{"name":"kind","type":"string","value":"flat"}]},
	   {"id":1,"properties":[{"name":"spawn","type":"string","value":"enemy"},
	                         {"name":"enemy","type":"string","value":"brute"}]},
	   {"id":2,"properties":[{"name":"spawn","type":"string","value":"enemy"},
	                         {"name":"enemy","type":"string","value":"lurker"}]}]}]}`)
	m, err := ParseTiled(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.EnemyKindMap) != 1 || m.EnemyKindMap[0] != "ab" {
		t.Fatalf("the painting is %q", m.EnemyKindMap)
	}
	if len(m.EnemyKinds) != 2 || m.EnemyKinds[0].Name != "brute" {
		t.Fatalf("the sorts are %+v", m.EnemyKinds)
	}
	// Names and places only - never figures.
	if m.EnemyKinds[0].BudgetMean != 0 || m.EnemyKinds[0].Share != 0 {
		t.Fatalf("the map carried figures: %+v", m.EnemyKinds[0])
	}

	cfg := quietConfig()
	cfg.EnemyKinds = []EnemyKind{{Name: "brute", Share: 3, BudgetMean: 700}}
	m.Apply(&cfg)
	if len(cfg.EnemyKinds) != 2 {
		t.Fatalf("%d rows, want 2", len(cfg.EnemyKinds))
	}
	if cfg.EnemyKinds[0].BudgetMean != 700 || cfg.EnemyKinds[0].Key != 'a' {
		t.Fatalf("brute is %+v", cfg.EnemyKinds[0])
	}
	// A sort nobody described still has to be able to arrive.
	if cfg.EnemyKinds[1].Name != "lurker" || cfg.EnemyKinds[1].Share != 1 {
		t.Fatalf("lurker is %+v", cfg.EnemyKinds[1])
	}
}
