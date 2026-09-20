package engine

import (
	"math"
	"testing"
)

// richConfig is a world with nobody in it that grows one plant a tick, so a
// test can watch where the plants land and nothing else.
func richConfig() Config {
	cfg := testConfig()
	cfg.FoodSpawnRate = 1
	cfg.MaxFoodItems = 100000
	return cfg
}

// plantsLeftAndRight counts how many plants came up in each half.
func plantsLeftAndRight(w *World) (left, right int) {
	for _, f := range w.Foods() {
		if f.Kind != FoodPlant {
			continue
		}
		if f.X < w.cfg.Width/2 {
			left++
		} else {
			right++
		}
	}
	return
}

func grow(cfg Config, ticks int) *World {
	w := NewWorld(cfg)
	for i := 0; i < ticks; i++ {
		w.Step()
	}
	return w
}

func TestAnUnpaintedWorldGrowsExactlyAsItDid(t *testing.T) {
	// Not "nearly": the paint has to be out of the run entirely when there is
	// none, down to how many numbers have been taken from the random source.
	a := grow(richConfig(), 500)
	cfg := richConfig()
	cfg.RichMap = nil
	b := grow(cfg, 500)
	if len(a.Foods()) != len(b.Foods()) {
		t.Fatalf("%d plants against %d", len(a.Foods()), len(b.Foods()))
	}
	if a.draws.draws != b.draws.draws {
		t.Fatalf("%d draws from the random source against %d", a.draws.draws, b.draws.draws)
	}
	for i := range a.Foods() {
		if a.Foods()[i].X != b.Foods()[i].X || a.Foods()[i].Y != b.Foods()[i].Y {
			t.Fatalf("plant %d came up somewhere else", i)
		}
	}
}

func TestRichnessSaysWhereAndNotHowMany(t *testing.T) {
	// Stage 15a's conservation, inside the painting: the world grows exactly
	// as many plants as FoodSpawnRate says whatever the map looks like.
	plain := grow(richConfig(), 500)
	cfg := richConfig()
	cfg.RichMap = []string{"91"}
	painted := grow(cfg, 500)
	if got, want := len(painted.Foods()), len(plain.Foods()); got != want {
		t.Fatalf("a painted world grew %d plants, an unpainted one %d", got, want)
	}
}

func TestPlantsComeUpInProportionToThePainting(t *testing.T) {
	cfg := richConfig()
	cfg.RichMap = []string{"91"} // 1.8 on the left, 0.2 on the right
	w := grow(cfg, 2000)
	left, right := plantsLeftAndRight(w)
	if right == 0 {
		t.Fatal("nothing came up on the poor half, which is not what 1 means")
	}
	ratio := float64(left) / float64(right)
	if math.Abs(ratio-9) > 2 {
		t.Fatalf("%d plants on the rich half against %d on the poor one (ratio %.1f, want about 9)",
			left, right, ratio)
	}
}

func TestNothingComesUpWhereTheGroundIsBare(t *testing.T) {
	cfg := richConfig()
	cfg.RichMap = []string{"50"} // ordinary on the left, nothing on the right
	w := grow(cfg, 1000)
	left, right := plantsLeftAndRight(w)
	if right != 0 {
		t.Fatalf("%d plants came up on ground painted '0'", right)
	}
	if left == 0 {
		t.Fatal("nothing came up at all")
	}
}

func TestTheRegionsKeepTheMeanOfThePainting(t *testing.T) {
	// The belief stays coarse because a memory is small, and it is still a
	// belief about something true: what regionlore compares itself against is
	// the average of the ground plants actually come up on.
	cfg := richConfig()
	cfg.RegionCols, cfg.RegionRows = 2, 1
	cfg.RichMap = []string{"91"}
	w := NewWorld(cfg)
	got := w.Regions()
	if len(got) != 2 {
		t.Fatalf("%d regions, want 2", len(got))
	}
	approx(t, got[0].Food, 1.8, 0.01, "the rich half")
	approx(t, got[1].Food, 0.2, 0.01, "the poor half")
}

func TestAPaintingThatGrowsNothingIsIgnored(t *testing.T) {
	// A map that says the whole world is bare is a mistake, not a world, and
	// honouring it would stop anything growing at all.
	cfg := richConfig()
	cfg.RichMap = []string{"00", "00"}
	w := grow(cfg, 200)
	if len(w.Foods()) == 0 {
		t.Fatal("a world painted bare everywhere grew nothing")
	}
}

func TestTheVocabularyOfTheRichMap(t *testing.T) {
	for _, c := range []struct {
		ch   byte
		want float64
	}{
		{'0', 0}, {'5', 1}, {'9', 1.8}, {'.', 1}, {'x', 1}, {' ', 1},
	} {
		if got := richOf(c.ch); got != c.want {
			t.Fatalf("%q is %v, want %v", c.ch, got, c.want)
		}
	}
}

func TestAShortRowIsOrdinaryGround(t *testing.T) {
	g := buildRich(&Config{RichMap: []string{"9", "99"}})
	if g == nil {
		t.Fatal("no grid")
	}
	if got := g.at[1]; got != 1 {
		t.Fatalf("the cell the row did not reach is %v, want 1", got)
	}
}

func TestAMapMayPaintHowWellTheGroundGrows(t *testing.T) {
	// The same map twice: one tile ordinary, one rich, one bare. What the
	// loader hands back is the vocabulary Config.RichMap takes.
	data := []byte(`{"width":3,"height":1,"tilewidth":32,"tileheight":32,
	 "layers":[
	  {"type":"tilelayer","name":"ground","width":3,"height":1,"data":[1,1,1]},
	  {"type":"tilelayer","name":"rich","width":3,"height":1,"data":[2,3,4]}],
	 "tilesets":[{"firstgid":1,"name":"t","tiles":[
	   {"id":0,"properties":[{"name":"kind","type":"string","value":"flat"}]},
	   {"id":1,"properties":[{"name":"rich","type":"float","value":1}]},
	   {"id":2,"properties":[{"name":"rich","type":"float","value":1.8}]},
	   {"id":3,"properties":[{"name":"rich","type":"float","value":0}]}]}]}`)
	m, err := ParseTiled(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Rich) != 1 || m.Rich[0] != "590" {
		t.Fatalf("rich rows %q, want [\"590\"]", m.Rich)
	}
	cfg := richConfig()
	m.Apply(&cfg)
	if len(cfg.RichMap) != 1 || cfg.RichMap[0] != "590" {
		t.Fatalf("Config.RichMap %q", cfg.RichMap)
	}
}

func TestAMapThatNeverMentionsRichnessPaintsNone(t *testing.T) {
	// Every tile is ordinary ground with no "rich" property on it. Reading
	// that as a painting of ordinary richness would quietly switch off the
	// world's own weighting, which is a different world.
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
	if m.Rich != nil {
		t.Fatalf("rich rows %q, want none", m.Rich)
	}
}

func TestOneLayerMaySayBothWhereAndHowWell(t *testing.T) {
	data := []byte(`{"width":2,"height":1,"tilewidth":32,"tileheight":32,
	 "layers":[
	  {"type":"tilelayer","name":"ground","width":2,"height":1,"data":[1,1]},
	  {"type":"tilelayer","name":"food","width":2,"height":1,"data":[2,3]}],
	 "tilesets":[{"firstgid":1,"name":"t","tiles":[
	   {"id":0,"properties":[{"name":"kind","type":"string","value":"flat"}]},
	   {"id":1,"properties":[{"name":"spawn","type":"string","value":"plant"},
	                         {"name":"rich","type":"float","value":1.8}]},
	   {"id":2,"properties":[{"name":"spawn","type":"string","value":"plant"},
	                         {"name":"rich","type":"float","value":0.2}]}]}]}`)
	m, err := ParseTiled(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Spawn) != 1 || m.Spawn[0] != "pp" {
		t.Fatalf("spawn rows %q, want [\"pp\"]", m.Spawn)
	}
	if len(m.Rich) != 1 || m.Rich[0] != "91" {
		t.Fatalf("rich rows %q, want [\"91\"]", m.Rich)
	}
}

func TestTheMaskAndThePaintingCompose(t *testing.T) {
	// The mask says plants may come up anywhere; the painting says the west
	// grows nine times what the east does. Both have to be true at once.
	cfg := richConfig()
	cfg.SpawnMap = []string{"pp"}
	cfg.RichMap = []string{"91"}
	w := grow(cfg, 2000)
	left, right := plantsLeftAndRight(w)
	if right == 0 {
		t.Fatal("nothing came up on the poor half")
	}
	if ratio := float64(left) / float64(right); math.Abs(ratio-9) > 2.5 {
		t.Fatalf("%d against %d (ratio %.1f, want about 9)", left, right, ratio)
	}
}

func TestTheMaskWinsWhereThePaintingSaysBare(t *testing.T) {
	// A square the mask allows and the painting calls bare: the mask is what
	// the author said about that square in particular, and a world where two
	// drawings disagree still has to grow something.
	cfg := richConfig()
	cfg.SpawnMap = []string{".p"} // only the east may grow
	cfg.RichMap = []string{"90"}  // and the east is painted bare
	w := grow(cfg, 500)
	left, right := plantsLeftAndRight(w)
	if left != 0 {
		t.Fatalf("%d plants came up where the mask forbids it", left)
	}
	if right == 0 {
		t.Fatal("two drawings disagreed and the world grew nothing")
	}
}

func TestAMaskWithNoPaintingIsUnchanged(t *testing.T) {
	cfg := richConfig()
	cfg.SpawnMap = []string{"pp"}
	a := grow(cfg, 500)
	cfg2 := richConfig()
	cfg2.SpawnMap = []string{"pp"}
	cfg2.RichMap = nil
	b := grow(cfg2, 500)
	if a.draws.draws != b.draws.draws {
		t.Fatalf("%d draws against %d", a.draws.draws, b.draws.draws)
	}
	for i := range a.Foods() {
		if a.Foods()[i].X != b.Foods()[i].X {
			t.Fatalf("plant %d came up somewhere else", i)
		}
	}
}
