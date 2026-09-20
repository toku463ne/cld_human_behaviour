package engine

import (
	"math"
	"strings"
	"testing"
)

// A small map drawn the way Tiled writes one: two tilesets' worth of tiles
// described by their own properties, a tile layer, and an object layer with
// two rectangles.
const tiledSample = `{
 "width": 4, "height": 3, "tilewidth": 32, "tileheight": 32,
 "tilesets": [{"firstgid": 1, "tiles": [
   {"id": 0, "properties": [{"name":"kind","value":"flat"}]},
   {"id": 1, "properties": [{"name":"kind","value":"water"}]},
   {"id": 2, "properties": [{"name":"kind","value":"rough"}]},
   {"id": 3, "properties": [{"name":"kind","value":"high"},{"name":"height","value":2}]},
   {"id": 4, "properties": [{"name":"kind","value":"slope"},{"name":"height","value":1}]}
 ]}],
 "layers": [
  {"type":"tilelayer","name":"ground","width":4,"height":3,
   "data":[1,2,3,4, 1,2,5,4, 1,1,1,1]},
  {"type":"objectgroup","name":"regions","objects":[
    {"name":"shore","x":0,"y":0,"width":64,"height":96,
     "properties":[{"name":"food","value":1.5}]},
    {"name":"hills","x":64,"y":0,"width":64,"height":96}
  ]}
 ]}`

func TestReadingAMapDrawnInTiled(t *testing.T) {
	m, err := ParseTiled([]byte(tiledSample))
	if err != nil {
		t.Fatalf("could not read the map: %v", err)
	}
	want := []string{".~:2", ".~A2", "...."}
	if got := strings.Join(m.Terrain, "|"); got != strings.Join(want, "|") {
		t.Fatalf("the ground reads %q, want %q", got, want)
	}
	if len(m.Regions) != 2 {
		t.Fatalf("read %d regions, want 2", len(m.Regions))
	}
	// Fractions of the map, so the drawing does not care how big the world is.
	if r := m.Regions[0]; r.Name != "shore" || r.X != 0 || math.Abs(r.W-0.5) > 1e-9 || r.Food != 1.5 {
		t.Fatalf("the first region reads %+v", r)
	}
	if r := m.Regions[1]; r.Name != "hills" || math.Abs(r.X-0.5) > 1e-9 {
		t.Fatalf("the second region reads %+v", r)
	}
}

// A tile nobody described, and an empty cell, are level open ground - so a map
// may carry as much decoration as its author likes.
func TestUndescribedTilesAreOpenGround(t *testing.T) {
	m, err := ParseTiled([]byte(`{"width":2,"height":1,"tilewidth":8,"tileheight":8,
	 "tilesets":[{"firstgid":1,"tiles":[{"id":0,"properties":[{"name":"kind","value":"water"}]}]}],
	 "layers":[{"type":"tilelayer","width":2,"height":1,"data":[0,99]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Terrain[0]; got != ".." {
		t.Fatalf("an empty cell and an unknown tile read %q, want %q", got, "..")
	}
}

// The flags Tiled packs into a tile id to say "draw this flipped" are about
// drawing, which is no business of the engine's.
func TestAFlippedTileIsTheSameGround(t *testing.T) {
	const flipped = 1 | 0x80000000 // horizontal flip on tile 1
	m, err := ParseTiled([]byte(`{"width":1,"height":1,"tilewidth":8,"tileheight":8,
	 "tilesets":[{"firstgid":1,"tiles":[{"id":0,"properties":[{"name":"kind","value":"rough"}]}]}],
	 "layers":[{"type":"tilelayer","width":1,"height":1,"data":[` +
		itoa(flipped) + `]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.Terrain[0] != ":" {
		t.Fatalf("a flipped rough tile reads %q", m.Terrain[0])
	}
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}

// What it refuses, and why the message says what to do about it.
func TestWhatTiledMapsThisCannotRead(t *testing.T) {
	for _, tc := range []struct{ name, data, want string }{
		{"a tileset in another file",
			`{"width":1,"height":1,"tilesets":[{"firstgid":1,"source":"trees.tsx"}],
			 "layers":[{"type":"tilelayer","width":1,"height":1,"data":[1]}]}`,
			"another file"},
		{"a compressed layer",
			`{"width":1,"height":1,"tilesets":[],
			 "layers":[{"type":"tilelayer","width":1,"height":1,"data":null}]}`,
			"no readable data"},
		{"no ground at all",
			`{"width":1,"height":1,"tilesets":[],"layers":[]}`,
			"no tile layer"},
		{"a kind nobody knows",
			`{"width":1,"height":1,"tilesets":[{"firstgid":1,"tiles":[
			  {"id":0,"properties":[{"name":"kind","value":"lava"}]}]}],
			 "layers":[{"type":"tilelayer","width":1,"height":1,"data":[1]}]}`,
			"not one of"},
	} {
		_, err := ParseTiled([]byte(tc.data))
		if err == nil {
			t.Fatalf("%s: read it without complaint", tc.name)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: said %q, want it to mention %q", tc.name, err, tc.want)
		}
	}
}

// A drawn world is a world: laying one out gives exactly the same run as
// writing the same thing by hand, and draws no random numbers of its own.
func TestADrawnWorldIsTheSameAsAWrittenOne(t *testing.T) {
	m, err := ParseTiled([]byte(tiledSample))
	if err != nil {
		t.Fatal(err)
	}
	drawn := quietConfig()
	drawn.Seed = 11
	m.Apply(&drawn)

	written := quietConfig()
	written.Seed = 11
	written.TerrainMap = []string{".~:2", ".~A2", "...."}
	written.RegionShapes = []RegionShape{
		{Name: "shore", X: 0, Y: 0, W: 0.5, H: 1, Food: 1.5},
		{Name: "hills", X: 0.5, Y: 0, W: 0.5, H: 1},
	}

	a, b := NewWorld(drawn), NewWorld(written)
	for i := 0; i < 500; i++ {
		a.Step()
		b.Step()
	}
	if a.Stats() != b.Stats() {
		t.Fatalf("a drawn world ran to %+v and a written one to %+v", a.Stats(), b.Stats())
	}
}

// The regions are the rectangles, and everything outside them is one more.
func TestDrawnRegionsCoverTheWorld(t *testing.T) {
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	cfg.RegionShapes = []RegionShape{
		{Name: "west", X: 0, Y: 0, W: 0.25, H: 1, Food: 1.5},
		{Name: "east", X: 0.75, Y: 0, W: 0.25, H: 1},
	}
	w := NewWorld(cfg)

	if got, want := len(w.regions), 3; got != want {
		t.Fatalf("%d regions, want %d (the two drawn, and everywhere else)", got, want)
	}
	if got := w.regionIndexAt(40, 300); got != 0 {
		t.Fatalf("the far west is region %d, want 0", got)
	}
	if got := w.regionIndexAt(760, 300); got != 1 {
		t.Fatalf("the far east is region %d, want 1", got)
	}
	if got := w.regionIndexAt(400, 300); got != 2 {
		t.Fatalf("the middle is region %d, want 2 (everywhere else)", got)
	}
	if got := w.regions[0].Food; got != 1.5 {
		t.Fatalf("the west grows %v, want the 1.5 it was given", got)
	}
	if got := w.RegionNames(); got[0] != "west" || got[2] != "elsewhere" {
		t.Fatalf("the regions are called %v", got)
	}
}

// Drawing regions changes nothing about a world that draws none.
func TestAWorldWithNoDrawnRegionsIsUnchanged(t *testing.T) {
	cfg := quietConfig()
	cfg.Seed = 3
	plain := NewWorld(cfg)
	if plain.shapeIndex != nil {
		t.Fatal("a world with no drawn regions built a lookup for them")
	}
	if got, want := len(plain.regions), cfg.RegionCols*cfg.RegionRows; got != want {
		t.Fatalf("%d regions, want the grid's %d", got, want)
	}
}

// A drawn world saves and loads like any other: the rectangles are part of the
// Config, so stage 21's rule - save, load, run on, and it is the same run -
// holds for them too.
func TestADrawnWorldSavesAndLoads(t *testing.T) {
	cfg := quietConfig()
	cfg.Seed = 4
	cfg.Width, cfg.Height = 800, 600
	cfg.RegionShapes = []RegionShape{{Name: "west", X: 0, Y: 0, W: 0.5, H: 1, Food: 1.4}}
	w := NewWorld(cfg)
	for i := 0; i < 200; i++ {
		w.Step()
	}
	var buf strings.Builder
	if err := w.Save(&buf); err != nil {
		t.Fatal(err)
	}
	back, err := Load(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(back.cfg.RegionShapes), 1; got != want {
		t.Fatalf("the loaded world has %d drawn regions, want %d", got, want)
	}
	if back.shapeIndex == nil {
		t.Fatal("the loaded world did not build the lookup for its drawn regions")
	}
	if got := back.regionIndexAt(40, 300); got != 0 {
		t.Fatalf("the west of the loaded world is region %d, want 0", got)
	}
	for i := 0; i < 300; i++ {
		w.Step()
		back.Step()
	}
	if w.Stats() != back.Stats() {
		t.Fatalf("saved-and-loaded ran to %+v, the original to %+v", back.Stats(), w.Stats())
	}
}

// The painted layer: things come up only where the author said they may, and
// how many come up does not change.
func TestThingsComeUpOnlyWhereTheyArePainted(t *testing.T) {
	cfg := testConfig()
	cfg.Seed = 9
	cfg.Width, cfg.Height = 800, 400
	cfg.FoodSpawnRate = 1
	cfg.MaxFoodItems = 400
	// Plants on the west half only.
	cfg.SpawnMap = []string{
		"pppp....",
		"pppp....",
	}
	w := NewWorld(cfg)
	for i := 0; i < 2000; i++ {
		w.Step()
	}
	plants := 0
	for _, f := range w.Foods() {
		if f.Kind != FoodPlant {
			continue
		}
		plants++
		if f.X > cfg.Width/2 {
			t.Fatalf("a plant came up at x=%v, east of the painted ground", f.X)
		}
	}
	if plants == 0 {
		t.Fatal("nothing came up at all")
	}
}

// How much comes up is untouched: the same rate grows the same number of
// plants, painted or not (stage 15a's conservation, now inside the painting).
func TestPaintingSaysWhereAndNotHowMany(t *testing.T) {
	grow := func(paint bool) int {
		cfg := testConfig()
		cfg.Seed = 4
		cfg.Width, cfg.Height = 800, 400
		cfg.FoodSpawnRate = 1
		cfg.MaxFoodItems = 10000 // no ceiling in the way
		if paint {
			cfg.SpawnMap = []string{"pppp....", "pppp...."}
		}
		w := NewWorld(cfg)
		for i := 0; i < 500; i++ {
			w.Step()
		}
		n := 0
		for _, f := range w.Foods() {
			if f.Kind == FoodPlant {
				n++
			}
		}
		return n
	}
	painted, plain := grow(true), grow(false)
	if painted != plain {
		t.Fatalf("painted grew %d plants and unpainted %d: want the same count", painted, plain)
	}
}

// Fish come up only on painted water, and a fish painted onto dry land is
// ignored rather than obeyed.
func TestFishArePaintedOnlyIntoWater(t *testing.T) {
	cfg := testConfig()
	cfg.Seed = 2
	cfg.Width, cfg.Height = 800, 300
	cfg.FoodSpawnRate, cfg.MaxFoodItems = 1, 400
	cfg.FishShare = 1 // every spawn is a fish
	cfg.TerrainMap = []string{
		"..~~....",
		"..~~....",
		"..~~....",
	}
	// Fish painted on the western water, and on dry ground in the east.
	cfg.SpawnMap = []string{
		"..ff.ff.",
		"..ff.ff.",
		"..ff.ff.",
	}
	w := NewWorld(cfg)
	for i := 0; i < 500; i++ {
		w.Step()
	}
	fish := 0
	for _, f := range w.Foods() {
		if f.Kind != FoodFish {
			continue
		}
		fish++
		if w.terrainAt(f.X, f.Y).Kind != GroundWater {
			t.Fatalf("a fish came up on dry land at %v,%v", f.X, f.Y)
		}
		if f.X > 500 {
			t.Fatalf("a fish came up at x=%v, where the painting is on dry ground", f.X)
		}
	}
	if fish == 0 {
		t.Fatal("no fish came up at all")
	}
}

// Enemies arrive only where they are painted.
func TestEnemiesArriveWhereTheyArePainted(t *testing.T) {
	cfg := testConfig()
	cfg.Seed = 6
	cfg.Width, cfg.Height = 800, 400
	cfg.EnemySpawnTicks, cfg.MaxEnemies = 5, 40
	cfg.SpawnMap = []string{"....eeee", "....eeee"}
	w := NewWorld(cfg)
	for i := 0; i < 400; i++ {
		w.Step()
	}
	n := 0
	for _, a := range w.Agents() {
		if a.Species != SpeciesEnemy {
			continue
		}
		n++
		// Only arrivals are painted: a cub is born where its mother is.
		if len(a.ParentIDs) > 0 {
			continue
		}
		if a.X < cfg.Width/2-40 {
			t.Fatalf("an enemy turned up at x=%v, west of the painted ground", a.X)
		}
	}
	if n == 0 {
		t.Fatal("no enemies arrived at all")
	}
}

// An unpainted world is the world as it was, down to the random numbers.
func TestAnUnpaintedWorldIsUnchanged(t *testing.T) {
	run := func(paint bool) Stats {
		cfg := testConfig()
		cfg.Seed = 8
		cfg.FoodSpawnRate, cfg.InitialPopulation, cfg.InitialFoodItems = 1, 20, 40
		if paint {
			cfg.SpawnMap = []string{"........"} // painted, and nothing on it
		}
		w := NewWorld(cfg)
		for i := 0; i < 600; i++ {
			w.Step()
		}
		return w.Stats()
	}
	if blank, plain := run(true), run(false); blank != plain {
		t.Fatalf("a blank painting ran to %+v and no painting to %+v", blank, plain)
	}
}

// A map may paint a second tile layer: where things are allowed to come up.
func TestReadingThePaintedLayer(t *testing.T) {
	const drawing = `{
	 "width":4,"height":2,"tilewidth":16,"tileheight":16,
	 "tilesets":[{"firstgid":1,"tiles":[
	   {"id":0,"properties":[{"name":"kind","value":"flat"}]},
	   {"id":1,"properties":[{"name":"kind","value":"water"}]},
	   {"id":2,"properties":[{"name":"spawn","value":"plant"}]},
	   {"id":3,"properties":[{"name":"spawn","value":"fish"}]},
	   {"id":4,"properties":[{"name":"spawn","value":"enemy"}]}
	 ]}],
	 "layers":[
	  {"type":"tilelayer","name":"ground","width":4,"height":2,"data":[1,1,2,2, 1,1,2,2]},
	  {"type":"tilelayer","name":"spawns","width":4,"height":2,"data":[3,0,4,0, 3,5,4,0]}
	 ]}`
	m, err := ParseTiled([]byte(drawing))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(m.Terrain, "|"), "..~~|..~~"; got != want {
		t.Fatalf("the ground reads %q, want %q", got, want)
	}
	if got, want := strings.Join(m.Spawn, "|"), "p.f.|pef."; got != want {
		t.Fatalf("the painting reads %q, want %q", got, want)
	}

	cfg := quietConfig()
	m.Apply(&cfg)
	if got, want := len(cfg.SpawnMap), 2; got != want {
		t.Fatalf("the config took %d painted rows, want %d", got, want)
	}
}

// A second tile layer that paints nothing is not a painting: a map with two
// layers of scenery still grows food everywhere.
func TestASecondLayerThatPaintsNothingIsNotAPainting(t *testing.T) {
	m, err := ParseTiled([]byte(`{
	 "width":2,"height":1,"tilewidth":16,"tileheight":16,
	 "tilesets":[{"firstgid":1,"tiles":[{"id":0,"properties":[{"name":"kind","value":"rough"}]}]}],
	 "layers":[
	  {"type":"tilelayer","width":2,"height":1,"data":[1,1]},
	  {"type":"tilelayer","width":2,"height":1,"data":[1,1]}
	 ]}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.Spawn != nil {
		t.Fatalf("scenery was read as a painting: %q", m.Spawn)
	}
}

// A rectangle may be marked as a goal: the engine carries the mark and never
// reads it, and whoever is running the game asks for the list.
func TestAMapMayMarkGoalRegions(t *testing.T) {
	m, err := ParseTiled([]byte(`{
	 "width":2,"height":1,"tilewidth":16,"tileheight":16,
	 "tilesets":[{"firstgid":1,"tiles":[{"id":0,"properties":[{"name":"kind","value":"flat"}]}]}],
	 "layers":[
	  {"type":"tilelayer","width":2,"height":1,"data":[1,1]},
	  {"type":"objectgroup","objects":[
	    {"name":"the meadow","x":0,"y":0,"width":16,"height":16,
	     "properties":[{"name":"goal","type":"bool","value":true}]},
	    {"name":"the scree","x":16,"y":0,"width":16,"height":16}
	  ]}
	 ]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !m.Regions[0].Goal || m.Regions[1].Goal {
		t.Fatalf("the goals read %v and %v", m.Regions[0].Goal, m.Regions[1].Goal)
	}

	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	m.Apply(&cfg)
	w := NewWorld(cfg)
	got := w.GoalRegions()
	if len(got) != 1 || got[0] != 0 {
		t.Fatalf("the world reports goals %v, want [0]", got)
	}
	// Marking a goal changes nothing about how the world runs.
	plain := cfg
	plain.RegionShapes = append([]RegionShape(nil), cfg.RegionShapes...)
	for i := range plain.RegionShapes {
		plain.RegionShapes[i].Goal = false
	}
	a, b := NewWorld(cfg), NewWorld(plain)
	for i := 0; i < 300; i++ {
		a.Step()
		b.Step()
	}
	if a.Stats() != b.Stats() {
		t.Fatalf("a marked world ran to %+v and an unmarked one to %+v", a.Stats(), b.Stats())
	}
}

// Regions may be painted cell by cell instead of boxed in by a rectangle, and
// then they may be any shape at all - here an L, which no rectangle can be.
func TestRegionsMayBePaintedCellByCell(t *testing.T) {
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 400
	cfg.RegionShapes = []RegionShape{
		{Name: "the bend", Key: 'a', Food: 1.6},
		{Name: "the scree", Key: 'b'},
	}
	cfg.RegionMap = []string{
		"aa......",
		"aa....bb",
		"aaaa..bb",
		"aaaa..bb",
	}
	w := NewWorld(cfg)

	if got, want := len(w.regions), 3; got != want {
		t.Fatalf("%d regions, want %d (the two painted, and everywhere else)", got, want)
	}
	for _, tc := range []struct {
		x, y float64
		want int
		what string
	}{
		{50, 50, 0, "the top of the bend"},
		{350, 350, 0, "the foot of the bend, where no rectangle could reach both"},
		{350, 50, 2, "above the foot, which is outside it"},
		{750, 250, 1, "the scree"},
		{500, 200, 2, "the gap between them"},
	} {
		if got := w.regionIndexAt(tc.x, tc.y); got != tc.want {
			t.Fatalf("%s is region %d, want %d", tc.what, got, tc.want)
		}
	}
	if got := w.regions[0].Food; got != 1.6 {
		t.Fatalf("the bend grows %v, want the 1.6 it was painted with", got)
	}
	if got := w.RegionNames(); got[0] != "the bend" || got[2] != "elsewhere" {
		t.Fatalf("the regions are called %v", got)
	}
}

// Where a map says both, the painting wins: painting a cell is the more
// particular thing to have said about it.
func TestAPaintedCellWinsOverARectangle(t *testing.T) {
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 400
	cfg.RegionShapes = []RegionShape{
		{Name: "the west", X: 0, Y: 0, W: 0.5, H: 1},
		{Name: "the spring", Key: 'a'},
	}
	cfg.RegionMap = []string{"..a.....", "..a....."}
	w := NewWorld(cfg)
	if got := w.regionIndexAt(100, 200); got != 0 {
		t.Fatalf("the unpainted west is region %d, want 0", got)
	}
	if got := w.regionIndexAt(250, 200); got != 1 {
		t.Fatalf("the spring inside the west is region %d, want 1", got)
	}
}

// A painted region layer, read from a drawing: tiles that carry "region", with
// the same name being one region however far apart it is painted.
func TestReadingAPaintedRegionLayer(t *testing.T) {
	const drawing = `{
	 "width":4,"height":2,"tilewidth":16,"tileheight":16,
	 "tilesets":[{"firstgid":1,"tiles":[
	   {"id":0,"properties":[{"name":"kind","value":"flat"}]},
	   {"id":1,"properties":[{"name":"region","value":"the shore"},{"name":"food","value":1.5},
	                         {"name":"goal","type":"bool","value":true}]},
	   {"id":2,"properties":[{"name":"region","value":"the scree"},{"name":"shelter","value":0.5}]},
	   {"id":3,"properties":[{"name":"region","value":"the shore"}]}
	 ]}],
	 "layers":[
	  {"type":"tilelayer","name":"ground","width":4,"height":2,"data":[1,1,1,1, 1,1,1,1]},
	  {"type":"tilelayer","name":"regions","width":4,"height":2,"data":[2,0,3,0, 4,0,3,0]}
	 ]}`
	m, err := ParseTiled([]byte(drawing))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(m.RegionMap, "|"), "a.b.|a.b."; got != want {
		t.Fatalf("the painted regions read %q, want %q", got, want)
	}
	if len(m.Regions) != 2 {
		t.Fatalf("read %d regions, want 2: the two tiles called \"the shore\" are one", len(m.Regions))
	}
	if r := m.Regions[0]; r.Name != "the shore" || r.Key != 'a' || r.Food != 1.5 || !r.Goal {
		t.Fatalf("the first region reads %+v", r)
	}
	if r := m.Regions[1]; r.Name != "the scree" || r.Key != 'b' || r.Shelter != 0.5 {
		t.Fatalf("the second region reads %+v", r)
	}

	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 400
	m.Apply(&cfg)
	w := NewWorld(cfg)
	if got := w.regionIndexAt(50, 200); got != 0 {
		t.Fatalf("the painted shore is region %d, want 0", got)
	}
	if got := w.regionIndexAt(250, 200); got != 2 {
		t.Fatalf("the unpainted gap is region %d, want 2 (everywhere else)", got)
	}
	if got := w.GoalRegions(); len(got) != 1 || got[0] != 0 {
		t.Fatalf("the world reports goals %v, want [0]", got)
	}
}

// A tile layer after the ground is read as whichever of the two it paints, not
// as whichever came first: a map may draw regions and no painted spawns.
func TestAMapMayDrawRegionsWithNoPaintedSpawns(t *testing.T) {
	m, err := ParseTiled([]byte(`{
	 "width":2,"height":1,"tilewidth":16,"tileheight":16,
	 "tilesets":[{"firstgid":1,"tiles":[
	   {"id":0,"properties":[{"name":"kind","value":"flat"}]},
	   {"id":1,"properties":[{"name":"region","value":"the meadow"}]}
	 ]}],
	 "layers":[
	  {"type":"tilelayer","name":"ground","width":2,"height":1,"data":[1,1]},
	  {"type":"tilelayer","name":"regions","width":2,"height":1,"data":[2,0]}
	 ]}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.Spawn != nil {
		t.Fatalf("the region layer was read as a painting of spawns: %q", m.Spawn)
	}
	if len(m.Regions) != 1 || m.RegionMap == nil {
		t.Fatalf("read %d regions and the map %q", len(m.Regions), m.RegionMap)
	}
}

// A painted world saves and loads like any other: the painting is part of the
// Config, and the lookup is built again from it.
func TestAPaintedRegionWorldSavesAndLoads(t *testing.T) {
	cfg := quietConfig()
	cfg.Seed = 5
	cfg.Width, cfg.Height = 800, 400
	cfg.RegionShapes = []RegionShape{{Name: "the bend", Key: 'a', Food: 1.4}}
	cfg.RegionMap = []string{"aa......", "aaaa...."}
	w := NewWorld(cfg)
	for i := 0; i < 200; i++ {
		w.Step()
	}
	var buf strings.Builder
	if err := w.Save(&buf); err != nil {
		t.Fatal(err)
	}
	back, err := Load(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatal(err)
	}
	if got := back.regionIndexAt(50, 300); got != 0 {
		t.Fatalf("the painted bend of the loaded world is region %d, want 0", got)
	}
	if got := back.regionIndexAt(700, 300); got != 1 {
		t.Fatalf("the unpainted east of the loaded world is region %d, want 1", got)
	}
	for i := 0; i < 300; i++ {
		w.Step()
		back.Step()
	}
	if w.Stats() != back.Stats() {
		t.Fatalf("saved-and-loaded ran to %+v, the original to %+v", back.Stats(), w.Stats())
	}
}

// A world that paints no regions is the world as it was, down to the random
// numbers.
func TestAWorldWithNoPaintedRegionsIsUnchanged(t *testing.T) {
	run := func(paint bool) Stats {
		cfg := testConfig()
		cfg.Seed = 7
		if paint {
			cfg.RegionMap = []string{"........"} // painted, and nothing on it
		}
		w := NewWorld(cfg)
		for i := 0; i < 400; i++ {
			w.Step()
		}
		return w.Stats()
	}
	if blank, plain := run(true), run(false); blank != plain {
		t.Fatalf("a blank painting ran to %+v and no painting to %+v", blank, plain)
	}
}

// The weather is painted on a layer of its own (#135): tiles that say how
// cold or how hot they are, over a ground layer that says nothing about it.
func TestTheWeatherIsPaintedOnItsOwnLayer(t *testing.T) {
	m, err := ParseTiled([]byte(`{"width":4,"height":2,"tilewidth":8,"tileheight":8,
	 "tilesets":[
	   {"firstgid":1,"tiles":[{"id":0,"properties":[{"name":"kind","value":"flat"}]}]},
	   {"firstgid":10,"tiles":[
	     {"id":0,"properties":[{"name":"chill","value":9}]},
	     {"id":1,"properties":[{"name":"chill","value":4}]},
	     {"id":2,"properties":[{"name":"heat","value":3}]}]}],
	 "layers":[
	  {"type":"tilelayer","name":"ground","width":4,"height":2,"data":[1,1,1,1, 1,1,1,1]},
	  {"type":"tilelayer","name":"weather","width":4,"height":2,"data":[10,11,0,12, 10,11,0,12]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"94.c", "94.c"}
	if got := strings.Join(m.Climate, "|"); got != strings.Join(want, "|") {
		t.Fatalf("the weather reads %q, want %q", got, want)
	}
	// The ground is untouched by it: the two are separate maps over the same
	// country, which is the reason the weather has a grain of its own.
	if got := strings.Join(m.Terrain, "|"); got != "....|...." {
		t.Fatalf("the ground reads %q", got)
	}
	// And it lands in the Config the way the rest of the drawing does.
	cfg := testConfig()
	m.Apply(&cfg)
	cfg.ChillDrain = 0.5
	w := NewWorld(cfg)
	cold := w.weatherAt(cfg.Width*0.1, cfg.Height*0.5, WeatherChill)
	mild := w.weatherAt(cfg.Width*0.35, cfg.Height*0.5, WeatherChill)
	none := w.weatherAt(cfg.Width*0.6, cfg.Height*0.5, WeatherChill)
	if !(cold > mild && mild > 0 && none == 0) {
		t.Fatalf("the four columns came out at %v, %v, %v", cold, mild, none)
	}
	if got := w.weatherAt(cfg.Width*0.9, cfg.Height*0.5, WeatherHeat); got <= 0 {
		t.Fatalf("the hot column came out at %v heat", got)
	}
}

// A map that says nothing about the weather has none, which is the default -
// so every map drawn before this reads exactly as it did.
func TestAMapWithNoWeatherHasNone(t *testing.T) {
	m, err := ParseTiled([]byte(tiledSample))
	if err != nil {
		t.Fatal(err)
	}
	if m.Climate != nil {
		t.Fatalf("a map with no weather on it came out with %q", m.Climate)
	}
	cfg := testConfig()
	cfg.ClimateMap = []string{"9999"}
	m.Apply(&cfg)
	if got := strings.Join(cfg.ClimateMap, "|"); got != "9999" {
		t.Fatalf("applying it overwrote the world's own weather with %q", got)
	}
}
