package engine

import (
	"math"
	"testing"
)

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

func TestThePaintingSaysWhichSortGrowsWhere(t *testing.T) {
	cfg := kindConfig()
	cfg.PlantKinds = []PlantKind{
		{Name: "berry", Share: 1, Key: 'a'},
		{Name: "root", Share: 1, Key: 'b'},
	}
	cfg.PlantKindMap = []string{"ab"} // berries west, roots east
	w := grow(cfg, 1000)
	for _, f := range w.Foods() {
		if f.Kind != FoodPlant {
			continue
		}
		want := uint8(1)
		if f.X >= cfg.Width/2 {
			want = 2
		}
		if f.Genes.Strain != want {
			t.Fatalf("a plant at x=%.0f is kind %d, want %d", f.X, f.Genes.Strain, want)
		}
	}
	got := w.PlantStrains()
	if got[0] == 0 || got[1] == 0 {
		t.Fatalf("one half grew nothing: %v", got)
	}
}

func TestAnUnpaintedCellStillDrawsByShare(t *testing.T) {
	// A map may name the country it cares about and leave the rest to the
	// table: painting the western half only still leaves the east drawing.
	cfg := kindConfig()
	cfg.PlantKinds = []PlantKind{
		{Name: "berry", Share: 0, Key: 'a'}, // never drawn, only painted
		{Name: "root", Share: 1},
	}
	cfg.PlantKindMap = []string{"a."}
	w := grow(cfg, 1000)
	west, east := 0, 0
	for _, f := range w.Foods() {
		if f.Kind != FoodPlant {
			continue
		}
		if f.X < cfg.Width/2 {
			if f.Genes.Strain != 1 {
				t.Fatalf("the painted half grew kind %d", f.Genes.Strain)
			}
			west++
		} else {
			if f.Genes.Strain != 2 {
				t.Fatalf("the unpainted half grew kind %d, want the drawn one", f.Genes.Strain)
			}
			east++
		}
	}
	if west == 0 || east == 0 {
		t.Fatalf("west %d east %d", west, east)
	}
}

func TestASeedlingKeepsItsSortWhereverItLands(t *testing.T) {
	// The painting decides what comes out of the ground, not what a lineage
	// turns into by walking.
	cfg := kindConfig()
	cfg.PlantGenetics = true
	cfg.PlantKinds = []PlantKind{{Name: "berry", Share: 1, Key: 'a'}}
	cfg.PlantKindMap = []string{"a."}
	w := NewWorld(cfg)
	child := w.inheritPlantGenes(plantGenes{Spread: 10, Regrow: 1, Strain: 1})
	if child.Strain != 1 {
		t.Fatalf("the seedling is kind %d, want 1", child.Strain)
	}
}

func TestAMapMaySayWhichSortGrowsWhere(t *testing.T) {
	data := []byte(`{"width":2,"height":1,"tilewidth":32,"tileheight":32,
	 "layers":[
	  {"type":"tilelayer","name":"ground","width":2,"height":1,"data":[1,1]},
	  {"type":"tilelayer","name":"crops","width":2,"height":1,"data":[2,3]}],
	 "tilesets":[{"firstgid":1,"name":"t","tiles":[
	   {"id":0,"properties":[{"name":"kind","type":"string","value":"flat"}]},
	   {"id":1,"properties":[{"name":"spawn","type":"string","value":"plant"},
	                         {"name":"plant","type":"string","value":"berry"}]},
	   {"id":2,"properties":[{"name":"spawn","type":"string","value":"plant"},
	                         {"name":"plant","type":"string","value":"root"}]}]}]}`)
	m, err := ParseTiled(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.PlantKindMap) != 1 || m.PlantKindMap[0] != "ab" {
		t.Fatalf("the painting is %q, want [\"ab\"]", m.PlantKindMap)
	}
	if len(m.PlantKinds) != 2 || m.PlantKinds[0].Name != "berry" || m.PlantKinds[1].Name != "root" {
		t.Fatalf("the sorts are %+v", m.PlantKinds)
	}
	// The map brings names and where they are; it never brings figures.
	if m.PlantKinds[0].Share != 0 || m.PlantKinds[0].Poison != 0 {
		t.Fatalf("the map carried figures: %+v", m.PlantKinds[0])
	}

	// A table that already knows one of them keeps its figures and gains the
	// key; one it has never heard of becomes a row of its own.
	cfg := kindConfig()
	cfg.PlantKinds = []PlantKind{{Name: "berry", Share: 2, Poison: 0.5}}
	m.Apply(&cfg)
	if len(cfg.PlantKinds) != 2 {
		t.Fatalf("%d rows, want 2", len(cfg.PlantKinds))
	}
	if cfg.PlantKinds[0].Poison != 0.5 || cfg.PlantKinds[0].Key != 'a' {
		t.Fatalf("berry is %+v, want its figures kept and key 'a'", cfg.PlantKinds[0])
	}
	if cfg.PlantKinds[1].Name != "root" || cfg.PlantKinds[1].Key != 'b' {
		t.Fatalf("root is %+v", cfg.PlantKinds[1])
	}
	if len(cfg.PlantKindMap) != 1 || cfg.PlantKindMap[0] != "ab" {
		t.Fatalf("Config.PlantKindMap is %q", cfg.PlantKindMap)
	}
}

func TestAMapThatNamesNoPlantsPaintsNone(t *testing.T) {
	data := []byte(`{"width":2,"height":1,"tilewidth":32,"tileheight":32,
	 "layers":[
	  {"type":"tilelayer","name":"ground","width":2,"height":1,"data":[1,1]},
	  {"type":"tilelayer","name":"spawn","width":2,"height":1,"data":[2,2]}],
	 "tilesets":[{"firstgid":1,"name":"t","tiles":[
	   {"id":0,"properties":[{"name":"kind","type":"string","value":"flat"}]},
	   {"id":1,"properties":[{"name":"spawn","type":"string","value":"plant"}]}]}]}`)
	m, err := ParseTiled(data)
	if err != nil {
		t.Fatal(err)
	}
	if m.PlantKindMap != nil || m.PlantKinds != nil {
		t.Fatalf("painted %q / %+v with nothing named", m.PlantKindMap, m.PlantKinds)
	}
}

// How filling a sort is (2026-09-22). The table says it and the map never
// does, which is the same division the rest of the row already has.
func TestASortMaySayHowFillingItIs(t *testing.T) {
	ate := func(nutrition float64) float64 {
		cfg := quietConfig()
		cfg.PlantKinds = []PlantKind{{Name: "food1", Share: 1, Nutrition: nutrition}}
		w := NewWorld(cfg)
		id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Genome: filledGenome(50)})
		a := mustAgent(t, w, id)
		a.Hunger = w.cfg.MaxHunger
		id2 := w.addFood(100, 100)
		if f := w.foodByID(id2); f.Genes.Strain != 1 {
			t.Fatalf("the plant is of sort %d, want the only one there is", f.Genes.Strain)
		}
		before := a.Hunger
		w.eat(a, id2)
		return before - a.Hunger
	}
	ordinary := ate(0) // a row that says nothing takes the world's own figure
	if ordinary <= 0 {
		t.Fatalf("an ordinary plant took %v off a starving body", ordinary)
	}
	if got := ate(1); got != ordinary {
		t.Fatalf("a sort that says one is worth %v, and an ordinary plant %v", got, ordinary)
	}
	if got := ate(2); math.Abs(got-2*ordinary) > 1e-9 {
		t.Fatalf("a sort that says two is worth %v, want %v", got, 2*ordinary)
	}
	if got := ate(0.5); math.Abs(got-ordinary/2) > 1e-9 {
		t.Fatalf("a sort that says a half is worth %v, want %v", got, ordinary/2)
	}
}

// And what a body sees of it is what it gets. The utility is scored off the
// perception, so a sort that fills two stomachs has to look like one before
// it is eaten - otherwise the world would be one thing and the reasoning
// about it another.
func TestABodySeesHowFillingASortIs(t *testing.T) {
	cfg := quietConfig()
	cfg.PlantKinds = []PlantKind{
		{Name: "food1", Share: 0, Key: 'a', Nutrition: 2},
		{Name: "food2", Share: 0, Key: 'b'},
	}
	cfg.PlantKindMap = []string{"ab"}
	w := NewWorld(cfg)
	// Standing on the border, with one country's crop to its left and the
	// other's to its right, both within sight.
	id := w.addAgent(Agent{Maturity: 1, X: cfg.Width / 2, Y: cfg.Height / 2,
		Genome: filledGenome(50)})
	a := mustAgent(t, w, id)
	a.Hunger = w.cfg.MaxHunger
	// Grown where they stand, which is what reads the painting: addFood is
	// for a plant with no whereabouts yet and takes nothing from the map.
	plant := func(x, y float64) {
		w.addPlant(x, y, w.drawPlantGenesAt(x, y))
	}
	plant(cfg.Width/2-20, cfg.Height/2) // in the country that grows food1
	plant(cfg.Width/2+20, cfg.Height/2) // and in the one that grows food2
	p := w.perceive(a)
	if len(p.Foods) != 2 {
		t.Fatalf("%d items in sight, want 2", len(p.Foods))
	}
	rich, plain := p.Foods[0], p.Foods[1]
	if rich.X > plain.X {
		rich, plain = plain, rich
	}
	if math.Abs(rich.Nutrition-2*plain.Nutrition) > 1e-9 {
		t.Fatalf("the filling sort looks worth %v and the ordinary one %v",
			rich.Nutrition, plain.Nutrition)
	}
}

// A world whose table says nothing about it is the world every figure before
// this was measured in, down to the random source.
func TestSayingNothingAboutFillingChangesNothing(t *testing.T) {
	cfg := kindConfig()
	cfg.PlantKinds = []PlantKind{{Name: "berry", Share: 1}}
	a := grow(cfg, 400)
	cfg.PlantKinds = []PlantKind{{Name: "berry", Share: 1, Nutrition: 0}}
	b := grow(cfg, 400)
	if a.draws.draws != b.draws.draws {
		t.Fatalf("%d draws against %d", a.draws.draws, b.draws.draws)
	}
	if len(a.foods) != len(b.foods) {
		t.Fatalf("%d plants against %d", len(a.foods), len(b.foods))
	}
}
