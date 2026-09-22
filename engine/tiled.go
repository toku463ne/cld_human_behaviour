package engine

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Reading a map drawn in Tiled (.tmj, which is JSON).
//
// The engine knows nothing about files: this takes the bytes and hands back
// what a Config needs, exactly as save.go does. Whoever read the file - a
// devview flag today, a fetch in the browser later - keeps that job.
//
// Tile layers become the terrain the world is laid on, the painted layer that
// says where things may come up, and the regions; an object layer's rectangles
// are the other way to draw a region. All of them end up in the fields stages
// 14, 20 and 56 already have, so nothing downstream learns that a map came
// from a drawing program rather than from a literal in a test.

// TiledWorld is a map as drawn: the terrain as the one-character-per-cell rows
// Config.TerrainMap takes, and the regions as fractions of the map (0 to 1),
// so that the same drawing describes the same country whatever size the world
// is given. Apply turns the fractions into a Config's own coordinates.
type TiledWorld struct {
	Terrain []string
	Regions []RegionShape

	// RegionMap is the painted region layer: which cell belongs to which of
	// Regions, one character per cell (2026-09-20). Nil when the author drew
	// the regions as rectangles instead, or drew none.
	RegionMap []string

	// Spawn is the painted layer: where plants, fish and enemies are allowed
	// to come up, one character per cell (2026-09-19). Nil when the map does
	// not paint one, and then everything comes up where it always did.
	Spawn []string

	// PlantKindMap says which sort of plant grows where, one character per
	// cell (2026-09-20), and PlantKinds are the sorts it names - names only,
	// because a map names a row and never carries its figures (decision
	// #133). Nil when no tile carries a "plant" property.
	PlantKindMap []string
	PlantKinds   []PlantKind

	// EnemyKindMap says which sort of enemy comes into the world where, one
	// character per cell, and EnemyKinds are the sorts it names - names only,
	// for the reason the plants' are (decision #133). Nil when no tile
	// carries an "enemy" property.
	EnemyKindMap []string
	EnemyKinds   []EnemyKind

	// Rich is how well each cell grows things, one character per cell
	// (2026-09-20), in the vocabulary Config.RichMap takes. Nil when no tile
	// carries a "rich" property, and then the world draws its own richness as
	// it always did.
	//
	// It is read off the same layers as the rest: a tile may say both where
	// plants may come up and how well they grow there, and one layer painted
	// with such tiles fills both.
	Rich []string

	// NestRate and NestCap are what a nest tile says about itself
	// (2026-09-22, TODO 18), in the vocabulary Config.NestRateMap and
	// Config.NestCapMap take: fifths, so '5' is an ordinary nest. Nil when no
	// tile carries a "rate" or a "cap", and then every painted nest is alike,
	// which is every world before this.
	//
	// They are read off whatever layer the nests themselves are on, since a
	// tile that says which sort comes out here is the natural one to say how
	// often and how many.
	NestRate  []string
	NestCap   []string
	NestQuiet []string

	// HumanNests is where people come into the world, by name (2026-09-22,
	// TODO 20), and HumanNestMap which cell is which - the same pair the
	// beasts' sorts have, and the same division of labour: the map brings the
	// name and the place, Config brings the figures (#133).
	HumanNests   []HumanNest
	HumanNestMap []string

	// Climate is what the weather is like in each cell, one character per
	// cell (#135), in the vocabulary Config.ClimateMap takes. Nil when no
	// tile carries a "chill" or a "heat" property, and then the world has no
	// weather, which is the default.
	//
	// It is read off a layer of its own in practice - an author paints the
	// weather over the ground rather than into it - but nothing here requires
	// that: as with the richness, a tile may say both what it is and how cold
	// it is, and one layer painted with such tiles fills both.
	Climate []string

	// Cols and Rows are the map's size in tiles, kept for the error messages
	// and for whoever wants to check a drawing against a world.
	Cols, Rows int
}

// tiled* are the parts of Tiled's JSON this reads. Everything else in the file
// is ignored on purpose: a map may carry as many decorative layers, tilesets
// and custom fields as its author likes.
type tiledFile struct {
	Width      int           `json:"width"`
	Height     int           `json:"height"`
	TileWidth  int           `json:"tilewidth"`
	TileHeight int           `json:"tileheight"`
	Layers     []tiledLayer  `json:"layers"`
	Tilesets   []tiledTilset `json:"tilesets"`
}

type tiledLayer struct {
	Type    string        `json:"type"`
	Name    string        `json:"name"`
	Width   int           `json:"width"`
	Height  int           `json:"height"`
	Data    []int64       `json:"data"`
	Objects []tiledObject `json:"objects"`
	Visible *bool         `json:"visible"`
}

type tiledObject struct {
	Name       string          `json:"name"`
	X          float64         `json:"x"`
	Y          float64         `json:"y"`
	Width      float64         `json:"width"`
	Height     float64         `json:"height"`
	Point      bool            `json:"point"`
	Ellipse    bool            `json:"ellipse"`
	Properties []tiledProperty `json:"properties"`
}

type tiledTilset struct {
	FirstGID int         `json:"firstgid"`
	Source   string      `json:"source"`
	Tiles    []tiledTile `json:"tiles"`
}

type tiledTile struct {
	ID         int             `json:"id"`
	Properties []tiledProperty `json:"properties"`
}

type tiledProperty struct {
	Name  string      `json:"name"`
	Value interface{} `json:"value"`
}

// tiledFlipMask clears the three flags Tiled packs into the top of a tile id
// when a tile is drawn flipped or rotated. They say how to draw it, which is
// no business of this engine's.
const tiledFlipMask = 0x1FFFFFFF

// ParseTiled reads a .tmj (Tiled's JSON) and returns the world it draws.
//
// What it understands:
//   - the first visible tile layer, as the terrain. Each tile says what it is
//     through its own properties in the tileset: "kind" is flat, rough, water,
//     slope or high, and "height" is how many levels up it sits. A tile with
//     no properties, and an empty cell, are level open ground.
//   - a later visible tile layer whose tiles carry "spawn", as the painted
//     layer: where plants, fish and enemies may come up (plant, fish, enemy,
//     food or all). A map that paints nothing has no such layer, and then
//     everything comes up where it always did.
//   - a later visible tile layer whose tiles carry "region", as the regions:
//     the property is the region's name, and tiles with the same name are one
//     region however far apart they are painted. The same tiles may carry
//     "shelter", "food", "special", "enemies" and "goal". A painted region may
//     be any shape at all, which is what a rectangle cannot be.
//   - every object layer's rectangles, as the regions. A rectangle may carry
//     "shelter", "food", "special" and "enemies" as properties, which are the
//     region's own figures; anything it leaves out keeps whatever the world's
//     spreads drew for it. It may also carry "goal", which marks a place the
//     game asks something of - the engine carries that and never reads it.
//     Where a map draws both, the painted cells win over the rectangles.
//
// What it refuses: a compressed or base64 tile layer, and a tileset kept in a
// separate file. Both are Tiled options rather than requirements, and reading
// them would mean either a compression dependency or file access - and the
// engine has neither.
func ParseTiled(data []byte) (*TiledWorld, error) {
	var f tiledFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("tiled: not a .tmj file: %w", err)
	}
	if f.Width <= 0 || f.Height <= 0 {
		return nil, fmt.Errorf("tiled: the map is %dx%d tiles", f.Width, f.Height)
	}
	out := &TiledWorld{Cols: f.Width, Rows: f.Height}

	kinds, spawns, err := tileKinds(f.Tilesets)
	if err != nil {
		return nil, err
	}
	riches, anyRich := tileRiches(f.Tilesets)
	rates, anyRate := tileFifths(f.Tilesets, "rate")
	caps, anyCap := tileFifths(f.Tilesets, "cap")
	quiets, anyQuiet := tileFifths(f.Tilesets, "quiet")
	climates, anyClimate := tileClimates(f.Tilesets)
	plants := tilePlantKinds(f.Tilesets)
	beasts := tileNamed(f.Tilesets, "enemy")
	folk := tileNamed(f.Tilesets, "human")
	places := tileRegions(f.Tilesets)
	for _, l := range f.Layers {
		switch l.Type {
		case "tilelayer":
			if l.Visible != nil && !*l.Visible {
				continue
			}
			if out.Terrain == nil {
				rows, err := terrainRows(l, f.Width, f.Height, kinds)
				if err != nil {
					return nil, err
				}
				out.Terrain = rows
				continue
			}
			// After the ground, a tile layer is whatever it paints, not
			// whatever comes first (2026-09-20): a map may draw its regions
			// and no painted layer, or the other way about, and numbering the
			// layers would have read one as the other.
			//
			// One layer may paint more than one of them, so none of these
			// three stops the others being tried: a region tile that also
			// says how well its country grows is one tile saying two true
			// things, and the first version of this read the region and threw
			// the richness away.
			//
			// The ground is still the first layer rather than the layer that
			// paints nothing else, because the same tile may say both what it
			// is and what comes up on it - a water tile that fish come up on
			// - and then there is nothing to tell apart.
			if out.RegionMap == nil {
				rows, shapes, err := regionRows(l, f.Width, f.Height, places)
				if err != nil {
					return nil, err
				}
				if len(shapes) > 0 {
					out.RegionMap, out.Regions = rows, append(out.Regions, shapes...)
				}
			}
			if out.Spawn == nil {
				rows, err := terrainRows(l, f.Width, f.Height, spawns)
				if err != nil {
					return nil, err
				}
				if paintedAnything(rows) {
					out.Spawn = rows
				}
			}
			// And which sort of enemy comes in here, off the same layer
			// again - the tile that says enemies may arrive is the natural
			// one to say which sort they are.
			if out.EnemyKindMap == nil {
				rows, kinds, err := enemyKindRows(l, f.Width, f.Height, beasts)
				if err != nil {
					return nil, err
				}
				if len(kinds) > 0 {
					out.EnemyKindMap, out.EnemyKinds = rows, kinds
				}
			}
			// And which sort of plant grows here, off the same layer again:
			// the tile that says plants may come up is the natural one to
			// say which sort they are.
			// And where a people starts, off the same layer again.
			if out.HumanNestMap == nil {
				rows, nests, err := humanNestRows(l, f.Width, f.Height, folk)
				if err != nil {
					return nil, err
				}
				if len(nests) > 0 {
					out.HumanNestMap, out.HumanNests = rows, nests
				}
			}
			if out.PlantKindMap == nil {
				rows, kinds, err := plantKindRows(l, f.Width, f.Height, plants)
				if err != nil {
					return nil, err
				}
				if len(kinds) > 0 {
					out.PlantKindMap, out.PlantKinds = rows, kinds
				}
			}
			// And what the weather is like here (#135), which an author
			// paints on a layer of its own: the weather is a map over the
			// same ground rather than a property of it, so that a world cut
			// into three regions can still have its cold drawn finely.
			if out.Climate == nil && anyClimate {
				rows, err := terrainRows(l, f.Width, f.Height, climates)
				if err != nil {
					return nil, err
				}
				if paintedClimate(rows) {
					out.Climate = rows
				}
			}
			// And how well the ground grows things, off the same layer if
			// that is where the author put it: a tile may say both.
			if out.Rich == nil && anyRich {
				rows, err := terrainRows(l, f.Width, f.Height, riches)
				if err != nil {
					return nil, err
				}
				if paintedRichness(rows) {
					out.Rich = rows
				}
			}
			// And what a nest sends out, off the layer the nests are on.
			if out.NestRate == nil && anyRate {
				rows, err := terrainRows(l, f.Width, f.Height, rates)
				if err != nil {
					return nil, err
				}
				if paintedRichness(rows) {
					out.NestRate = rows
				}
			}
			if out.NestCap == nil && anyCap {
				rows, err := terrainRows(l, f.Width, f.Height, caps)
				if err != nil {
					return nil, err
				}
				if paintedRichness(rows) {
					out.NestCap = rows
				}
			}
			if out.NestQuiet == nil && anyQuiet {
				rows, err := terrainRows(l, f.Width, f.Height, quiets)
				if err != nil {
					return nil, err
				}
				if paintedRichness(rows) {
					out.NestQuiet = rows
				}
			}
		case "objectgroup":
			out.Regions = append(out.Regions, regionShapes(l, f)...)
		}
	}
	if out.Terrain == nil {
		return nil, fmt.Errorf("tiled: no tile layer to read the ground from")
	}
	return out, nil
}

// paintedAnything says whether a painted layer allows anything anywhere. A
// layer of tiles that say nothing about spawning is not a painting, and
// treating it as one would stop the world growing altogether.
func paintedAnything(rows []string) bool {
	for _, r := range rows {
		for i := 0; i < len(r); i++ {
			if len(spawnKindsOf(r[i])) > 0 {
				return true
			}
		}
	}
	return false
}

// tileRiches turns the tilesets into "this tile id grows this much", and says
// whether any tile carried the property at all - a map that never mentions
// richness must not end up with a layer of ordinary ground standing in for
// the world's own weighting.
func tileRiches(sets []tiledTilset) (map[int]byte, bool) {
	out, any := map[int]byte{}, false
	for _, s := range sets {
		for _, t := range s.Tiles {
			c, ok := richChar(t.Properties)
			out[s.FirstGID+t.ID] = c
			any = any || ok
		}
	}
	return out, any
}

// tileFifths turns the tilesets into "this tile id says this much of the
// thing named", as the character a fifths-grid takes, and says whether any
// tile carried the property at all (2026-09-22, TODO 18).
//
// It is tileRiches with the name of the property handed in, because the nests
// have two of these and the weather already showed what happens when each
// vocabulary grows its own copy of the same walk.
//
// The scale is the one every painted grid in this engine uses: the number on
// the tile is a multiple of the world's own figure, rounded to the nearest
// fifth, so 1 is ordinary and 0 is none. The absolute is in Config (#133).
func tileFifths(sets []tiledTilset, name string) (map[int]byte, bool) {
	out, any := map[int]byte{}, false
	for _, s := range sets {
		for _, t := range s.Tiles {
			c := byte(richOrdinary)
			if v, ok := propNumberOK(t.Properties, name); ok {
				d := int(math.Round(clamp(v, 0, 2) * 5))
				c, any = byte('0'+clampInt(d, 0, 9)), true
			}
			out[s.FirstGID+t.ID] = c
		}
	}
	return out, any
}

// tilePlantKinds turns the tilesets into "this tile id grows this sort", by
// name. The figures are not here and never will be: a map names a row of
// Config.PlantKinds and the row carries what the sort is like (decision
// #133), so that the same number never lives in two places.
func tilePlantKinds(sets []tiledTilset) map[int]string { return tileNamed(sets, "plant") }

// tileNamed turns the tilesets into "this tile id names this thing", for the
// properties whose value is a name pointing at a row of a table in Config.
func tileNamed(sets []tiledTilset, prop string) map[int]string {
	out := map[int]string{}
	for _, s := range sets {
		for _, t := range s.Tiles {
			if name, ok := propString(t.Properties, prop); ok && name != "" {
				out[s.FirstGID+t.ID] = name
			}
		}
	}
	return out
}

// plantKindRows reads a tile layer as "which sort grows where": the rows
// saying which cell grows what, and the sorts themselves, named only.
//
// Sorts are numbered in the order they are first met reading the layer top to
// bottom, left to right - the same rule the regions use, so that the drawing
// and not the drawing program decides.
func plantKindRows(l tiledLayer, cols, rows int, named map[int]string) ([]string, []PlantKind, error) {
	painted, kinds, err := namedRows(l, cols, rows, named)
	if err != nil || len(kinds) == 0 {
		return nil, nil, err
	}
	out := make([]PlantKind, len(kinds))
	for i, k := range kinds {
		out[i] = PlantKind{Name: k.name, Key: k.key}
	}
	return painted, out, nil
}

// plantKindUnpainted is the character for a cell no sort was painted on.
const plantKindUnpainted = '.'

// enemyKindRows is plantKindRows for the beasts. Two readers rather than one
// generic one, because the two tables are different types and Go would want
// an interface to join them - which would cost more than the twenty lines it
// saved.
func enemyKindRows(l tiledLayer, cols, rows int, named map[int]string) ([]string, []EnemyKind, error) {
	painted, kinds, err := namedRows(l, cols, rows, named)
	if err != nil || len(kinds) == 0 {
		return nil, nil, err
	}
	out := make([]EnemyKind, len(kinds))
	for i, k := range kinds {
		out[i] = EnemyKind{Name: k.name, Key: k.key}
	}
	return painted, out, nil
}

// humanNestRows is enemyKindRows for the places people come out of.
func humanNestRows(l tiledLayer, cols, rows int, named map[int]string) ([]string, []HumanNest, error) {
	painted, nests, err := namedRows(l, cols, rows, named)
	if err != nil || len(nests) == 0 {
		return nil, nil, err
	}
	out := make([]HumanNest, len(nests))
	for i, n := range nests {
		out[i] = HumanNest{Name: n.name, Key: n.key}
	}
	return painted, out, nil
}

// namedThing is one row a layer named: what it is called and the character it
// is painted with.
type namedThing struct {
	name string
	key  byte
}

// namedRows is the walk both readers share: which cell names which thing, and
// the things in the order they are first met reading top to bottom, left to
// right - the rule the regions use, so the drawing and not the drawing
// program decides.
func namedRows(l tiledLayer, cols, rows int, named map[int]string) ([]string, []namedThing, error) {
	if len(named) == 0 {
		return nil, nil, nil
	}
	keys := map[int]byte{}
	byName := map[string]byte{}
	var things []namedThing
	tooMany := false
	take := func(gid int) byte {
		if k, ok := keys[gid]; ok {
			return k
		}
		name, ok := named[gid]
		if !ok {
			keys[gid] = plantKindUnpainted
			return plantKindUnpainted
		}
		if k, ok := byName[name]; ok {
			keys[gid] = k
			return k
		}
		if len(things) >= len(regionKeys) {
			tooMany = true
			keys[gid] = plantKindUnpainted
			return plantKindUnpainted
		}
		k := regionKeys[len(things)]
		things = append(things, namedThing{name: name, key: k})
		keys[gid], byName[name] = k, k
		return k
	}
	painted, err := terrainRowsFunc(l, cols, rows, take)
	if err != nil {
		return nil, nil, err
	}
	if len(things) == 0 {
		return nil, nil, nil
	}
	if tooMany {
		return nil, nil, fmt.Errorf("tiled: the layer %q names more than %d sorts",
			l.Name, len(regionKeys))
	}
	return painted, things, nil
}

// paintedRichness says whether a layer read as richness says anything at all.
// A layer of ordinary ground is not a painting, and treating it as one would
// quietly switch off the world's own weighting.
func paintedRichness(rows []string) bool {
	for _, r := range rows {
		for i := 0; i < len(r); i++ {
			if richOf(r[i]) != 1 {
				return true
			}
		}
	}
	return false
}

// ParseTiledFrom is the same from a reader, for whoever has one.
func ParseTiledFrom(r io.Reader) (*TiledWorld, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return ParseTiled(data)
}

// spawnChar is the other vocabulary a tile may carry: what is allowed to come
// up on it (2026-09-19). A tile that says nothing about spawning is bare
// ground, which is what lets one tileset describe both layers.
func spawnChar(props []tiledProperty) (byte, error) {
	kind, ok := propString(props, "spawn")
	if !ok || kind == "" {
		return spawnNone, nil
	}
	switch kind {
	case "plant", "plants":
		return spawnPlant, nil
	case "fish":
		return spawnFish, nil
	case "enemy", "enemies":
		return spawnEnemy, nil
	case "food":
		return spawnFood, nil
	case "all", "any":
		return spawnAll, nil
	case "none":
		return spawnNone, nil
	}
	return 0, fmt.Errorf("spawn %q is not one of plant, fish, enemy, food, all", kind)
}

// richChar is the third vocabulary a tile may carry: how well the ground
// grows things (2026-09-20). The property is a number, and what it means is
// the multiplier Config.RichMap's digits stand for - so 1 is ordinary ground,
// 0 grows nothing and 2 is as rich as this world goes.
//
// It is rounded to the nearest fifth because RichMap holds one character per
// cell, which is the form every other painted layer in this engine takes. A
// map wanting more resolution than that wants a different kind of file.
func richChar(props []tiledProperty) (byte, bool) {
	v, ok := propNumberOK(props, "rich")
	if !ok {
		return richOrdinary, false
	}
	d := int(math.Round(clamp(v, 0, 2) * 5))
	return byte('0' + clampInt(d, 0, 9)), true
}

// tileClimates turns the tilesets into "this tile id is this much weather", as
// the character Config.ClimateMap takes, and says whether any tile carried one
// at all.
//
// Two properties, one per weather, because that is what a weather is here: a
// tile that is both cold and hot is neither, and the last one read wins rather
// than the two being summed - summing them would make a character that means
// something else entirely.
func tileClimates(sets []tiledTilset) (map[int]byte, bool) {
	out, any := map[int]byte{}, false
	for _, s := range sets {
		for _, t := range s.Tiles {
			c, ok := climateChar(t.Properties)
			out[s.FirstGID+t.ID] = c
			any = any || ok
		}
	}
	return out, any
}

// climateChar is the whole of the weather's vocabulary: '1'-'9' for that much
// cold, 'a'-'i' for that much heat, '.' for the ordinary world.
//
// The scale is the share, not the dose. What the coldest place in the world
// costs a body is Config.ChillDrain and it is not on the map, because a map
// carries names and places and never figures (decision #133) - so the same
// drawing is a mild winter or a killing one depending on the world it is put
// in, and the author of a first stage can turn it down without redrawing.
func climateChar(props []tiledProperty) (byte, bool) {
	if v, ok := propNumberOK(props, "chill"); ok {
		if n := clampInt(int(math.Round(v)), 0, 9); n > 0 {
			return byte('0' + n), true
		}
		return climateOrdinary, true
	}
	if v, ok := propNumberOK(props, "heat"); ok {
		if n := clampInt(int(math.Round(v)), 0, 9); n > 0 {
			return byte('a' + n - 1), true
		}
		return climateOrdinary, true
	}
	return climateOrdinary, false
}

// climateOrdinary is a cell the author said nothing about the weather in.
const climateOrdinary = '.'

// paintedClimate says whether a layer puts any weather anywhere. A layer of
// tiles that say nothing about the weather is not a weather map, and taking it
// for one would give the world a climate of nothing at every grain - which is
// the same as none, but would stop the layer that does carry one being read.
func paintedClimate(rows []string) bool {
	for _, r := range rows {
		for i := 0; i < len(r); i++ {
			if r[i] != climateOrdinary {
				return true
			}
		}
	}
	return false
}

// tileKinds turns the tilesets into "this tile id is this piece of ground".
func tileKinds(sets []tiledTilset) (ground, spawn map[int]byte, err error) {
	ground, spawn = map[int]byte{}, map[int]byte{}
	for _, s := range sets {
		if s.Source != "" {
			return nil, nil, fmt.Errorf("tiled: the tileset %q is in another file; "+
				"embed it in the map (Tiled: Map > Embed tilesets)", s.Source)
		}
		for _, t := range s.Tiles {
			c, err := groundChar(t.Properties)
			if err != nil {
				return nil, nil, fmt.Errorf("tiled: tile %d: %w", s.FirstGID+t.ID, err)
			}
			ground[s.FirstGID+t.ID] = c
			sc, err := spawnChar(t.Properties)
			if err != nil {
				return nil, nil, fmt.Errorf("tiled: tile %d: %w", s.FirstGID+t.ID, err)
			}
			spawn[s.FirstGID+t.ID] = sc
		}
	}
	return ground, spawn, nil
}

// groundChar is the whole of the vocabulary: what a tile's properties say it
// is, as the character Config.TerrainMap uses (stage 20).
func groundChar(props []tiledProperty) (byte, error) {
	kind, _ := propString(props, "kind")
	h, ok := propNumberOK(props, "height")
	if ok && h == 0 {
		// There and nought: either the author wrote nought, which means
		// nothing here, or they wrote something that is not a number. Both
		// are worth saying out loud, because the silent version of this made
		// a map drawn three levels high come out flat.
		if s, isText := propStringRaw(props, "height"); isText {
			return 0, fmt.Errorf("height %q is not a number", s)
		}
	}
	height := int(h)
	switch kind {
	case "", "flat", "open":
		if height > 0 {
			return heightChar(height, false)
		}
		return '.', nil
	case "rough":
		return ':', nil
	case "water":
		return '~', nil
	case "high":
		if height <= 0 {
			height = 1
		}
		return heightChar(height, false)
	case "slope", "ramp":
		if height <= 0 {
			height = 1
		}
		return heightChar(height, true)
	}
	return 0, fmt.Errorf("kind %q is not one of flat, rough, water, high, slope", kind)
}

func heightChar(height int, slope bool) (byte, error) {
	if height < 1 || height > 9 {
		return 0, fmt.Errorf("height %d is outside 1 to 9", height)
	}
	if slope {
		return byte('A' + height - 1), nil
	}
	return byte('0' + height), nil
}

// terrainRows reads the tile layer into the rows the world is laid on.
func terrainRows(l tiledLayer, cols, rows int, kinds map[int]byte) ([]string, error) {
	return terrainRowsFunc(l, cols, rows, func(gid int) byte {
		ch, ok := kinds[gid]
		if !ok {
			ch = '.' // an empty cell, or a tile nobody described
		}
		return ch
	})
}

// terrainRowsFunc is the walk itself, with what a tile means left to the
// caller: the ground, what comes up, or which region it is.
func terrainRowsFunc(l tiledLayer, cols, rows int, of func(gid int) byte) ([]string, error) {
	if len(l.Data) == 0 {
		return nil, fmt.Errorf("tiled: the layer %q has no readable data; "+
			"save the map with CSV or uncompressed tile layers", l.Name)
	}
	if l.Width > 0 {
		cols = l.Width
	}
	if l.Height > 0 {
		rows = l.Height
	}
	if len(l.Data) != cols*rows {
		return nil, fmt.Errorf("tiled: the layer %q holds %d tiles for a %dx%d map",
			l.Name, len(l.Data), cols, rows)
	}
	out := make([]string, rows)
	line := make([]byte, cols)
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			line[c] = of(int(l.Data[r*cols+c] & tiledFlipMask))
		}
		out[r] = string(line)
	}
	return out, nil
}

// tiledRegion is what a tile says about the region it paints.
type tiledRegion struct {
	shape RegionShape
	named bool // the tile carried a "region" property at all
}

// tileRegions turns the tilesets into "this tile id paints this region".
func tileRegions(sets []tiledTilset) map[int]tiledRegion {
	out := map[int]tiledRegion{}
	for _, s := range sets {
		for _, t := range s.Tiles {
			name, ok := propString(t.Properties, "region")
			if !ok {
				continue
			}
			out[s.FirstGID+t.ID] = tiledRegion{
				named: true,
				shape: RegionShape{
					Name:    name,
					Shelter: propNumber(t.Properties, "shelter"),
					Food:    propNumber(t.Properties, "food"),
					Special: propNumber(t.Properties, "special"),
					Enemies: propNumber(t.Properties, "enemies"),
					Goal:    propNumber(t.Properties, "goal") > 0,
					Price:   propNumber(t.Properties, "price"),
					Years:   propNumber(t.Properties, "years"),
				},
			}
		}
	}
	return out
}

// regionKeys are the characters a painted region map is written in. They are
// printable and in a fixed order so that a map read twice reads the same, and
// so that a RegionMap can be pasted into a test and read by eye.
const regionKeys = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// regionRows reads a tile layer as the painted regions: the rows saying which
// cell is whose, and the regions themselves.
//
// Tiles with the same name are one region however far apart they are painted,
// which is the point of painting them - a shore that bends, a valley that
// forks. Two tiles with the same name and different figures is the author
// saying one thing twice, so the first painted wins.
func regionRows(l tiledLayer, cols, rows int, places map[int]tiledRegion) ([]string, []RegionShape, error) {
	if len(places) == 0 {
		return nil, nil, nil
	}
	keys := map[int]byte{} // tile id -> the character it paints
	byName := map[string]byte{}
	var shapes []RegionShape
	tooMany := false
	// Regions are numbered in the order they are first met reading the layer
	// top to bottom, left to right - the same rule the rectangles are sorted
	// by, so that the drawing and not the drawing program decides.
	take := func(gid int) byte {
		if k, ok := keys[gid]; ok {
			return k
		}
		p, ok := places[gid]
		if !ok || !p.named {
			keys[gid] = regionUnpainted
			return regionUnpainted
		}
		if k, ok := byName[p.shape.Name]; ok && p.shape.Name != "" {
			keys[gid] = k
			return k
		}
		if len(shapes) >= len(regionKeys) {
			tooMany = true
			keys[gid] = regionUnpainted
			return regionUnpainted
		}
		k := regionKeys[len(shapes)]
		shape := p.shape
		shape.Key = k
		shapes = append(shapes, shape)
		keys[gid] = k
		if shape.Name != "" {
			byName[shape.Name] = k
		}
		return k
	}
	painted, err := terrainRowsFunc(l, cols, rows, take)
	if err != nil {
		return nil, nil, err
	}
	if len(shapes) == 0 {
		return nil, nil, nil // not a region layer at all
	}
	// The count that matters is how many regions were painted, not how many
	// the tileset could paint: a tileset may hold a region for every map its
	// author ever drew.
	if tooMany {
		return nil, nil, fmt.Errorf("tiled: the layer %q paints more than %d regions",
			l.Name, len(regionKeys))
	}
	return painted, shapes, nil
}

// regionShapes reads an object layer's rectangles as regions, in fractions of
// the map so that the drawing does not care how big the world is.
func regionShapes(l tiledLayer, f tiledFile) []RegionShape {
	w := float64(f.Width * max(f.TileWidth, 1))
	h := float64(f.Height * max(f.TileHeight, 1))
	if w <= 0 || h <= 0 {
		return nil
	}
	var out []RegionShape
	for _, o := range l.Objects {
		if o.Point || o.Ellipse || o.Width <= 0 || o.Height <= 0 {
			continue // points are for stage 4's nests, not for regions
		}
		out = append(out, RegionShape{
			Name:    o.Name,
			X:       o.X / w,
			Y:       o.Y / h,
			W:       o.Width / w,
			H:       o.Height / h,
			Shelter: propNumber(o.Properties, "shelter"),
			Food:    propNumber(o.Properties, "food"),
			Special: propNumber(o.Properties, "special"),
			Enemies: propNumber(o.Properties, "enemies"),
			Goal:    propNumber(o.Properties, "goal") > 0,
			Price:   propNumber(o.Properties, "price"),
			Years:   propNumber(o.Properties, "years"),
		})
	}
	// The order a drawing program happens to write its objects in is not a
	// thing the world should depend on: regions are numbered top to bottom,
	// left to right, so the same drawing always gives the same numbering.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Y != out[j].Y {
			return out[i].Y < out[j].Y
		}
		return out[i].X < out[j].X
	})
	return out
}

// propStringRaw says whether this property is there and is text, for the
// messages that want to quote what the author actually wrote.
func propStringRaw(props []tiledProperty, name string) (string, bool) {
	for _, p := range props {
		if p.Name == name {
			s, ok := p.Value.(string)
			return s, ok
		}
	}
	return "", false
}

func propString(props []tiledProperty, name string) (string, bool) {
	for _, p := range props {
		if p.Name != name {
			continue
		}
		if s, ok := p.Value.(string); ok {
			return s, true
		}
	}
	return "", false
}

// propNumberOK is propNumber that also says whether the property was there at
// all, which is how "rich: 0" (bare ground) is told from "no rich property".
//
// A number typed as a string is read as a number (2026-09-20). Tiled lets an
// author pick the type of a custom property, and picking "string" for a
// figure is an easy thing to do and an invisible thing to have done: the
// engine used to read it as nought, so a tile drawn as height 3 became
// height 1 and the map looked flat for no stated reason. Whether the string
// is a number at all is the caller's business - groundChar refuses one that
// is not, rather than quietly rounding it to nought.
func propNumberOK(props []tiledProperty, name string) (float64, bool) {
	for _, p := range props {
		if p.Name != name {
			continue
		}
		switch v := p.Value.(type) {
		case float64:
			return v, true
		case int:
			return float64(v), true
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				return f, true
			}
			return 0, true // there, and not a number: the caller decides
		}
	}
	return 0, false
}

func propNumber(props []tiledProperty, name string) float64 {
	for _, p := range props {
		if p.Name != name {
			continue
		}
		if b, ok := p.Value.(bool); ok {
			if b {
				return 1
			}
			return 0
		}
	}
	v, _ := propNumberOK(props, name)
	return v
}

// Apply lays the drawing into a Config: the ground, and the regions if the
// map drew any.
//
// It draws no random numbers and touches nothing else, which is the same
// standing SetTerrain and SetRegion have (stage 22): a world laid out from a
// drawing runs exactly as one laid out by hand.
func (t *TiledWorld) Apply(cfg *Config) {
	if t == nil {
		return
	}
	cfg.TerrainMap = append([]string(nil), t.Terrain...)
	if len(t.Spawn) > 0 {
		cfg.SpawnMap = append([]string(nil), t.Spawn...)
	}
	if len(t.Rich) > 0 {
		cfg.RichMap = append([]string(nil), t.Rich...)
	}
	if len(t.NestRate) > 0 {
		cfg.NestRateMap = append([]string(nil), t.NestRate...)
	}
	if len(t.NestCap) > 0 {
		cfg.NestCapMap = append([]string(nil), t.NestCap...)
	}
	if len(t.NestQuiet) > 0 {
		cfg.NestQuietMap = append([]string(nil), t.NestQuiet...)
	}
	if len(t.Climate) > 0 {
		cfg.ClimateMap = append([]string(nil), t.Climate...)
	}
	t.applyPlantKinds(cfg)
	t.applyEnemyKinds(cfg)
	t.applyHumanNests(cfg)
	if len(t.Regions) > 0 {
		cfg.RegionShapes = append([]RegionShape(nil), t.Regions...)
	}
	if len(t.RegionMap) > 0 {
		cfg.RegionMap = append([]string(nil), t.RegionMap...)
	}
}

// applyPlantKinds merges what the map named into the table the Config brought.
//
// The two halves meet here and nowhere else: the map knows the names and
// where they are, the table knows what each sort is like, and a name in both
// is one sort (decision #133). A name the table has never heard of still
// becomes a row, so that a map may paint a country that grows something
// without the world having to be told in advance what it is - it comes up as
// an ordinary plant wearing that name, which is exactly what an unfilled row
// means anyway.
func (t *TiledWorld) applyPlantKinds(cfg *Config) {
	if len(t.PlantKinds) == 0 {
		return
	}
	at := map[string]int{}
	for i := range cfg.PlantKinds {
		at[cfg.PlantKinds[i].Name] = i
	}
	for _, k := range t.PlantKinds {
		if i, ok := at[k.Name]; ok {
			cfg.PlantKinds[i].Key = k.Key
			continue
		}
		cfg.PlantKinds = append(cfg.PlantKinds, k)
	}
	cfg.PlantKindMap = append([]string(nil), t.PlantKindMap...)
}

// applyHumanNests merges the places the map named into the table the Config
// brought, the same way the sorts do: a name in both is one nest, and a name
// the table never heard of becomes a row that takes the world's own figures.
//
// And it empties InitialPopulation, which is the one place a map does more
// than fill a table in. A nest is the author saying where this people starts;
// scattering sixty strangers over the same map as well would answer that with
// something else, and the author has no second switch to turn the strangers
// off - the -villages flag has done exactly this since TODO 20, and a painted
// nest means the same thing as that flag. A map that wants people spread
// about paints no nest.
func (t *TiledWorld) applyHumanNests(cfg *Config) {
	if len(t.HumanNests) == 0 {
		return
	}
	cfg.InitialPopulation = 0
	at := map[string]int{}
	for i := range cfg.HumanNests {
		at[cfg.HumanNests[i].Name] = i
	}
	for _, n := range t.HumanNests {
		if i, ok := at[n.Name]; ok {
			cfg.HumanNests[i].Key = n.Key
			continue
		}
		cfg.HumanNests = append(cfg.HumanNests, n)
	}
	cfg.HumanNestMap = append([]string(nil), t.HumanNestMap...)
}

// applyEnemyKinds is applyPlantKinds for the beasts, and merges the same way:
// a name the table already has keeps its figures and gains the character it is
// painted with, and one it has never heard of becomes a row of its own.
func (t *TiledWorld) applyEnemyKinds(cfg *Config) {
	if len(t.EnemyKinds) == 0 {
		return
	}
	at := map[string]int{}
	for i := range cfg.EnemyKinds {
		at[cfg.EnemyKinds[i].Name] = i
	}
	for _, k := range t.EnemyKinds {
		if i, ok := at[k.Name]; ok {
			cfg.EnemyKinds[i].Key = k.Key
			continue
		}
		// A sort nobody described still needs a reason to arrive at all, and
		// a row with no Share never does. One, so that a map may name a
		// country's beast without also having to say how common it is.
		k.Share = 1
		cfg.EnemyKinds = append(cfg.EnemyKinds, k)
	}
	cfg.EnemyKindMap = append([]string(nil), t.EnemyKindMap...)
}
