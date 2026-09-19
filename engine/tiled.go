package engine

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
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
			// After the ground, a tile layer is whichever of the two it
			// paints, not whichever comes first (2026-09-20): a map may draw
			// its regions and no painted layer, or the other way about, and
			// numbering the layers would have read one as the other.
			//
			// The ground is still the first layer rather than the layer that
			// paints no region and no spawn, because the same tile may say
			// both what it is and what comes up on it - a water tile that
			// fish come up on - and then there is nothing to tell apart.
			if out.RegionMap == nil {
				rows, shapes, err := regionRows(l, f.Width, f.Height, places)
				if err != nil {
					return nil, err
				}
				if len(shapes) > 0 {
					out.RegionMap, out.Regions = rows, append(out.Regions, shapes...)
					continue
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
	height := int(propNumber(props, "height"))
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

func propNumber(props []tiledProperty, name string) float64 {
	for _, p := range props {
		if p.Name != name {
			continue
		}
		switch v := p.Value.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case bool:
			if v {
				return 1
			}
			return 0
		}
	}
	return 0
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
	if len(t.Regions) > 0 {
		cfg.RegionShapes = append([]RegionShape(nil), t.Regions...)
	}
	if len(t.RegionMap) > 0 {
		cfg.RegionMap = append([]string(nil), t.RegionMap...)
	}
}
