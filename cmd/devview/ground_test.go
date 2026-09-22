package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"image"
	_ "image/png"
	"os"
	"strings"
	"testing"

	"github.com/toku463ne/cld_human_behaviour/engine"
)

// The country drawn with pictures (ground.go).
//
// What is worth testing here is not that a cell gets a picture - it always
// does - but that the rim of a block goes round it the right way. That is the
// part with arithmetic in it (a turn is a number added to a side), it is easy
// to get wrong by one, and at thirty-two pixels a corner turned the wrong way
// looks like art rather than like a bug.

// a little map, written the way engine.Config.TerrainMap is written, read
// through the engine so that this tests the runes the map files actually use
// rather than a copy of them kept here.
func country(t *testing.T, rows ...string) func(col, row int) spot {
	t.Helper()
	cfg := engine.DefaultConfig()
	cfg.TerrainMap = rows
	cfg.InitialPopulation = 0
	w := engine.NewWorld(cfg)
	cols, count, cw, ch := w.TerrainSize()
	if cols != len([]rune(rows[0])) || count != len(rows) {
		t.Fatalf("the map came back %dx%d and it was written %dx%d",
			cols, count, len([]rune(rows[0])), len(rows))
	}
	return func(col, row int) spot {
		if col < 0 || row < 0 || col >= cols || row >= count {
			return spot{}
		}
		g := w.TerrainAt((float64(col)+0.5)*cw, (float64(row)+0.5)*ch)
		return spot{kind: g.Kind, height: g.Height, slope: g.Slope}
	}
}

// what one cell is drawn with, on such a map.
func drawnAt(at func(int, int) spot, col, row int) stamp {
	side := [4]spot{at(col, row-1), at(col+1, row), at(col, row+1), at(col-1, row)}
	diag := [4]spot{at(col+1, row-1), at(col+1, row+1), at(col-1, row+1), at(col-1, row-1)}
	return groundStamp(at(col, row), side, diag, col, row)
}

// A block of high ground: face in the middle, rim round the outside, and the
// rim turned to the side it looks out over.
//
// The map has a notch in it so that all three pieces are wanted at once,
// which is the point: a set of rim pieces that only ever needs the straight
// one would not tell us the corners are right.
func TestTheRimOfAPlateauGoesRoundTheRightWay(t *testing.T) {
	at := country(t,
		".......",
		".11111.",
		".1.111.",
		".11111.",
		".11111.",
		".11111.",
		".......")
	for _, c := range []struct {
		col, row int
		clip     string
		turn     int
		why      string
	}{
		{3, 1, clipCliff + ".side", north, "the top edge of the plateau looks north"},
		{1, 3, clipCliff + ".side", west, "the left edge looks west"},
		{3, 6 - 1, clipCliff + ".side", south, "the bottom edge looks south"},
		{5, 3, clipCliff + ".side", east, "the right edge looks east"},
		{1, 1, clipCliff + ".outer", 0, "the north-west corner has two sides out"},
		{5, 1, clipCliff + ".outer", 1, "the north-east corner is that corner, turned once"},
		{5, 5, clipCliff + ".outer", 2, "the south-east corner, twice"},
		{1, 5, clipCliff + ".outer", 3, "the south-west corner, three times"},
		{3, 3, clipCliff + ".inner", 0, "whole on all four sides, and the notch at its north-west"},
	} {
		got := drawnAt(at, c.col, c.row)
		if got.clip != c.clip || got.turn != c.turn {
			t.Errorf("(%d,%d) is drawn %s turned %d, want %s turned %d: %s",
				c.col, c.row, got.clip, got.turn, c.clip, c.turn, c.why)
		}
	}
	// And the inside of it, which is the one cell with nothing to look out
	// over: a face, and not a rim.
	if got := drawnAt(at, 4, 3); !strings.HasPrefix(got.clip, clipHigh) {
		t.Errorf("the middle of the plateau is drawn %s, want one of %s*", got.clip, clipHigh)
	}
}

// The shore is drawn on the land, and turned to the water.
func TestTheShoreIsDrawnOnTheLandSide(t *testing.T) {
	at := country(t,
		"~~~..",
		"~~...",
		"~....",
		".....")
	for _, c := range []struct {
		col, row int
		clip     string
		turn     int
	}{
		{3, 0, clipBank + ".side", west}, // water to the west of it
		{2, 1, clipBank + ".outer", 0},   // water north and west
		{1, 2, clipBank + ".outer", 0},   // the same again, further down
		{1, 3, clipBank + ".inner", 0},   // only the corner is water
	} {
		got := drawnAt(at, c.col, c.row)
		if got.clip != c.clip || got.turn != c.turn {
			t.Errorf("(%d,%d) is drawn %s turned %d, want %s turned %d",
				c.col, c.row, got.clip, got.turn, c.clip, c.turn)
		}
	}
	// The water itself is water, whatever is beside it: the line between the
	// two is drawn once, on the land, or it would be drawn twice.
	if got := drawnAt(at, 0, 0); !strings.HasPrefix(got.clip, clipWater) {
		t.Errorf("the water is drawn %s, want %s*", got.clip, clipWater)
	}
}

// A ramp faces the way the map says is up, and gives up where nothing does.
//
// The engine does not know which way a ramp runs - a slope cell is a height
// and a flag - so this is the map being read, not the rules.
func TestARampFacesTheWayUp(t *testing.T) {
	at := country(t, "1A.")
	got := drawnAt(at, 1, 0)
	if !strings.HasPrefix(got.clip, clipSlope) || got.turn != west {
		t.Errorf("the ramp is drawn %s turned %d, want a ramp turned %d (the high side is west)",
			got.clip, got.turn, west)
	}
	if got.clip == clipSlope+".free" {
		t.Errorf("the ramp knows which way is up and is drawn without a direction")
	}
	// Level all round, and nothing says which way it climbs.
	flat := country(t, "AAA", "AAA", "AAA")
	if got := drawnAt(flat, 1, 1); got.clip != clipSlope+".free" {
		t.Errorf("a ramp with nothing higher beside it is drawn %s, want %s",
			got.clip, clipSlope+".free")
	}
}

// From level three, the ground is a thing standing there rather than a place,
// and from level five that thing is stone.
//
// The levels are a convention with whoever draws a map (tiled/properties.md)
// and the reason they can be one is a rule: canStep refuses a climb of two,
// so nothing reaches a level-three cell without three ramps in a row.
func TestHighEnoughGroundIsDrawnAsAThingRatherThanAPlace(t *testing.T) {
	trees := country(t,
		"33333",
		"33333",
		"33333")
	if got := drawnAt(trees, 2, 1); !strings.HasPrefix(got.clip, clipBlock) {
		t.Errorf("the middle of a great tree is drawn %s, want %s*", got.clip, clipBlock)
	}
	if got := drawnAt(trees, 2, 0); got.clip != clipRoot+".side" || got.turn != north {
		t.Errorf("the foot of it is drawn %s turned %d, want %s turned %d",
			got.clip, got.turn, clipRoot+".side", north)
	}
	rocks := country(t,
		"55555",
		"55555",
		"55555")
	if got := drawnAt(rocks, 2, 1); got.clip != clipBlock+"rock" {
		t.Errorf("the middle of a boulder is drawn %s, want %s", got.clip, clipBlock+"rock")
	}
	// Fallen rock at the foot of a boulder, roots at the foot of a tree, and
	// neither of them the other: a rock ringed with roots is not a rock, and
	// the ledge's own rim round one would put a pale halo on the floor.
	if got := drawnAt(rocks, 2, 0); got.clip != clipScree+".side" || got.turn != north {
		t.Errorf("the foot of a boulder is drawn %s turned %d, want %s turned %d",
			got.clip, got.turn, clipScree+".side", north)
	}
	// Two levels up is still somewhere to stand, and drawn as ground.
	ledge := country(t,
		"22222",
		"22222",
		"22222")
	if got := drawnAt(ledge, 2, 1); !strings.HasPrefix(got.clip, clipHigh) {
		t.Errorf("a ledge two levels up is drawn %s, want %s*", got.clip, clipHigh)
	}
}

// The plain ground is laid down in eight orientations, from where the cell
// is and from nothing else.
//
// Both halves matter. The turning is what stopped three pictures of forest
// floor reading as wallpaper, and it being the coordinates is what keeps the
// viewer from touching the world: a drawing that drew a random number would
// move every measurement ever made here.
func TestThePlainGroundIsTurnedAboutByWhereItIs(t *testing.T) {
	at := country(t, strings.Repeat(".", 12), strings.Repeat(".", 12), strings.Repeat(".", 12))
	seen := map[stamp]int{}
	for row := 0; row < 3; row++ {
		for col := 0; col < 12; col++ {
			seen[drawnAt(at, col, row)]++
		}
	}
	if len(seen) < 8 {
		t.Errorf("36 cells of level ground are drawn %d ways; the art has 3 pictures in 8 orientations", len(seen))
	}
	// Twice through, the same answer.
	for row := 0; row < 3; row++ {
		for col := 0; col < 12; col++ {
			if a, b := drawnAt(at, col, row), drawnAt(at, col, row); a != b {
				t.Fatalf("(%d,%d) is drawn %+v and then %+v", col, row, a, b)
			}
		}
	}
}

// Everything the drawing can ask for is something the gap report knows to
// look for.
//
// The report is what says a set of art is short of something (artgaps.go),
// and it walks groundClips. A clip this file can reach and that list does not
// name would be a hole that reports itself as complete - which is the exact
// failure the report was written for.
func TestEveryGroundPictureIsOneTheReportKnowsAbout(t *testing.T) {
	known := map[string]bool{}
	for _, name := range groundClips() {
		known[name] = true
	}
	at := country(t,
		"..:~~..33..",
		".:~~~.333..",
		"..~~..333..",
		"1A....555..",
		"11.~~.555..",
		"111~~.555..",
		"2211....~..",
		"...........")
	for row := 0; row < 8; row++ {
		for col := 0; col < 11; col++ {
			if got := drawnAt(at, col, row); !known[got.clip] {
				t.Errorf("(%d,%d) is drawn %q and the gap report does not know to look for it",
					col, row, got.clip)
			}
		}
	}
}

// And the set the viewer draws with has every one of them.
//
// The country is all or nothing (tileset.hasGround): a set missing one piece
// paints the whole world in washes instead, so "mostly drawn" is a state this
// cannot be in and a missing piece is worth failing over.
func TestTheSetItDrawsWithCanDrawTheWholeCountry(t *testing.T) {
	have := map[string]int{}
	for _, c := range theManifest(t).Clips {
		have[c.Name] = c.Frames
	}
	for _, name := range groundClips() {
		if have[name] == 0 {
			t.Errorf("the set it draws with has no %s, so the country falls back to the washes", name)
		}
	}
	// The water is the one thing that moves, and it moves because it has two
	// frames of one cell. One frame is a still river.
	for _, name := range []string{clipWater + "a", clipWater + "b"} {
		if have[name] < 2 {
			t.Errorf("%s has %d frames, want 2: the river is animated", name, have[name])
		}
	}
}

// The Tiled tileset a map is painted with draws what its tiles say they are.
//
// Two lists have to agree and they are written in two languages: the tileset
// (Python, tiled/tools/pack_sprites.py) says "this tile is high ground two
// levels up" and this viewer (Go, ground.go) decides that two levels up is
// somewhere to stand and three is a tree. Nothing but a test can hold them
// together, and the cost of them drifting is a map whose author painted a
// ledge and got a trunk.
//
// It reads the tileset the way Tiled will, off the file that ships.
func TestTheTilesetAMapIsPaintedWithDrawsWhatItSays(t *testing.T) {
	const dir = "../../tiled/samples/tilesets/002_mononoke/"
	raw, err := os.ReadFile(dir + "terrain.tsj")
	if err != nil {
		t.Fatalf("the tileset a map is drawn with is not there: %v", err)
	}
	var set struct {
		Columns    int    `json:"columns"`
		Image      string `json:"image"`
		ImageWidth int    `json:"imagewidth"`
		TileCount  int    `json:"tilecount"`
		TileWidth  int    `json:"tilewidth"`
		Tiles      []struct {
			ID         int    `json:"id"`
			Class      string `json:"class"`
			Properties []struct {
				Name  string          `json:"name"`
				Value json.RawMessage `json:"value"`
			} `json:"properties"`
		} `json:"tiles"`
	}
	if err := json.Unmarshal(raw, &set); err != nil {
		t.Fatalf("terrain.tsj: %v", err)
	}
	if set.TileCount != len(set.Tiles) || set.TileCount == 0 {
		t.Fatalf("the tileset says %d tiles and describes %d", set.TileCount, len(set.Tiles))
	}
	if set.ImageWidth != set.TileCount*set.TileWidth {
		t.Errorf("the tileset says %d tiles of %d and its picture is %d wide",
			set.TileCount, set.TileWidth, set.ImageWidth)
	}
	// The picture beside it is the one it describes. A tileset whose image
	// was regenerated with one more tile and whose json was not is a map
	// editor showing the wrong ground.
	png, err := os.Open(dir + set.Image)
	if err != nil {
		t.Fatalf("the tileset names a picture that is not there: %v", err)
	}
	defer png.Close()
	cfg, _, err := image.DecodeConfig(png)
	if err != nil {
		t.Fatalf("%s: %v", set.Image, err)
	}
	if cfg.Width != set.ImageWidth || cfg.Height != set.TileWidth {
		t.Errorf("%s is %dx%d and the tileset says %dx%d",
			set.Image, cfg.Width, cfg.Height, set.ImageWidth, set.TileWidth)
	}

	// What each class of tile has to come out as. The engine reads the
	// properties; this is the other end of them.
	wants := map[string]string{
		"field":   clipFlat,
		"broken":  clipRough,
		"stream":  clipWater,
		"ramp 1":  clipSlope,
		"ramp 2":  clipSlope,
		"ledge 1": clipHigh,
		"ledge 2": clipHigh,
		"tree":    clipBlock + "a", // one of the barks; the column picks which
		"boulder": clipBlock + "rock",
	}
	seen := map[string]bool{}
	for _, tile := range set.Tiles {
		kind, height := "flat", 0
		for _, p := range tile.Properties {
			switch p.Name {
			case "kind":
				_ = json.Unmarshal(p.Value, &kind)
			case "height":
				_ = json.Unmarshal(p.Value, &height)
			}
		}
		want, known := wants[tile.Class]
		if !known {
			t.Errorf("tile %d is a %q and this test does not know what that should look like",
				tile.ID, tile.Class)
			continue
		}
		seen[tile.Class] = true
		here := spot{height: height}
		switch kind {
		case "water":
			here.kind = engine.GroundWater
		case "rough":
			here.kind = engine.GroundRough
		case "slope":
			here.slope = true
		}
		// In the middle of a patch of its own, so that no rim piece is
		// wanted and the face itself is what comes back.
		side := [4]spot{here, here, here, here}
		got := groundStamp(here, side, side, 3, 5)
		if !strings.HasPrefix(got.clip, want) {
			t.Errorf("a %q (kind %q, height %d) is drawn %s, want %s*",
				tile.Class, kind, height, got.clip, want)
		}
	}
	for class := range wants {
		if !seen[class] {
			t.Errorf("nothing in the tileset is a %q any more, and this test still expects one", class)
		}
	}
}

// The tilesets a person paints with say the same as the map that ships.
//
// There are two copies of every property now: the .tsx files beside the
// pictures, which Tiled reads when somebody adds the tileset to a new map,
// and the copy embedded in 001_3division.tmj, which is what the engine
// reads (it refuses a map whose tilesets are not embedded). Two copies of
// anything drift, and the way this one would drift is the worst kind: a
// map drawn with the tileset would behave differently from the sample that
// was measured, and nothing would say so.
func TestThePaintedTilesetsSayWhatTheSampleMapSays(t *testing.T) {
	const dir = "../../tiled/samples/tilesets/001_simple/"
	raw, err := os.ReadFile("../../tiled/samples/001_3division.tmj")
	if err != nil {
		t.Fatalf("the sample map is not there: %v", err)
	}
	var m struct {
		Tilesets []struct {
			Name  string `json:"name"`
			Tiles []struct {
				ID         int `json:"id"`
				Properties []struct {
					Name  string          `json:"name"`
					Value json.RawMessage `json:"value"`
				} `json:"properties"`
			} `json:"tiles"`
		} `json:"tilesets"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("001_3division.tmj: %v", err)
	}
	type tileset struct {
		Tiles []struct {
			ID         int `xml:"id,attr"`
			Properties []struct {
				Name  string `xml:"name,attr"`
				Value string `xml:"value,attr"`
			} `xml:"properties>property"`
		} `xml:"tile"`
	}
	for _, in := range m.Tilesets {
		want := map[string]string{}
		for _, tile := range in.Tiles {
			for _, p := range tile.Properties {
				want[fmt.Sprintf("%d.%s", tile.ID, p.Name)] =
					strings.Trim(string(p.Value), `"`)
			}
		}
		painted, err := os.ReadFile(dir + in.Name + ".tsx")
		if err != nil {
			t.Errorf("%s is in the map and there is no tileset to paint it with: %v", in.Name, err)
			continue
		}
		var out tileset
		if err := xml.Unmarshal(painted, &out); err != nil {
			t.Errorf("%s.tsx: %v", in.Name, err)
			continue
		}
		got := map[string]string{}
		for _, tile := range out.Tiles {
			for _, p := range tile.Properties {
				got[fmt.Sprintf("%d.%s", tile.ID, p.Name)] = p.Value
			}
		}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s.tsx tile %s = %q, and the sample map says %q", in.Name, k, got[k], v)
			}
			delete(got, k)
		}
		for k, v := range got {
			t.Errorf("%s.tsx tile %s = %q, and the sample map says nothing about it", in.Name, k, v)
		}
	}
}
