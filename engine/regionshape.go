package engine

import "math"

// Regions an author draws, rather than a grid the world cuts for itself
// (2026-09-19). Until now the regions were RegionCols x RegionRows equal
// blocks; a map may now name its own, drawn two ways: as rectangles, or
// painted cell by cell in Config.RegionMap.
//
// Both end in the same place, because every rule that reads a region reads it
// through regionIndexAt, and what that has to be is fast and exact, not
// general: a lookup table over a fixed grid, which does not care whether a
// rectangle or a painted cell filled it in. That is why painting cost nothing
// to add - a rectangle was never the cheap shape, it was the cheap thing to
// write.

// RegionShape is one region as its author drew it: either a rectangle in
// fractions of the map (0 to 1), so that the same drawing describes the same
// country whatever size the world is given, or - when Key is set - every cell
// Config.RegionMap paints with that character.
//
// The four figures are the region's own, and nought means "leave what the
// world's spreads drew for it" - so an author can name a place without having
// to decide everything about it.
type RegionShape struct {
	Name                            string
	X, Y, W, H                      float64
	Shelter, Food, Special, Enemies float64

	// Key is the character that stands for this region in Config.RegionMap,
	// for a region that is painted cell by cell rather than boxed in by a
	// rectangle (2026-09-20). Nought means the rectangle above says where it
	// is.
	//
	// Painting is the better of the two and the rectangle is kept because it
	// is the cheaper to write: a painted region may be any shape at all - a
	// valley that forks, a shore that bends - while a rectangle says "the
	// western quarter" in one line. What a painted region cannot do is be two
	// regions with the same figures, because the figures belong to the
	// character; that wants two characters.
	Key byte

	// Goal marks a region the game asks something of - the places a player's
	// line has to be living in at once, in the dynasty this is being built
	// towards (PLAN.md, decision #130).
	//
	// The engine never reads it. It is carried the way Endow and SetTerrain
	// are carried: data the world holds and the simulation does not consume,
	// so that a shipped snapshot brings its own goals with it instead of
	// needing a second file that can disagree with the map. Whether a goal is
	// met, and what that means, belongs to the game layer - engine has no
	// player and no winning (stage 19).
	Goal bool

	// Price is what the game asks for a village here, in coins, and Years how
	// long a village founded here keeps its line up. Both are carried exactly
	// as Goal is: the engine never reads either of them (2026-09-22).
	//
	// They are figures, and figures are usually kept off a map (#133). The
	// line that decision draws is about the simulation's own rules - what a
	// beast is like, what a crop is worth - and these are neither. They are
	// the game's asking price for a country, which is a thing only the map
	// can know: a rich valley and a bare shelf are not worth the same, and
	// the author is the one who drew the difference. Nothing in engine reads
	// them, so nothing in engine can be moved by them.
	Price float64
	Years float64
}

// regionIndexGrid is how a position becomes a region when the regions are
// drawn rather than cut. It is a lookup table over a fixed grid, filled in
// once when the world is built, so the answer stays one multiplication and one
// array read - which is what regionIndexAt was before.
//
// Counted before this was built: regionIndexAt runs 3 to 10 times a tick, so
// its cost was never the thing to protect. What does scale with the number of
// regions is worthOfRegion (15 a tick at twelve regions, 1,900 at 768) and the
// memory each agent carries (48 bytes a region, so 576 bytes at twelve and
// 36 KB at 768). The rule of thumb that follows: draw a dozen regions, not a
// hundred.
type regionIndexGrid struct {
	cols, rows int
	at         []int

	// bounds is the box each region is inside, in fractions of the map, kept
	// because a viewer and a map editor want somewhere to draw a region's
	// outline (2026-09-20). It is worked out once here rather than each time
	// it is asked for, and it is only a box: a painted region may be any
	// shape, and the box round an L is not the L.
	bounds []regionBox
}

type regionBox struct{ minX, minY, maxX, maxY float64 }

// regionShapeGridSize is how finely the lookup table is cut. It is not a rule
// of the world - nothing about the simulation reads it - so it is a constant
// rather than a Config field, exactly as the spatial index's cell size is.
// At this size a rectangle's edge lands within a two-hundredth of the world.
const regionShapeGridSize = 200

// buildRegionShapes lays out the regions an author drew, and returns false
// when there are none - in which case the world keeps the grid it always had
// and nothing at all changes.
//
// The last region is "everywhere else": a drawing need not cover the map, and
// a body standing outside every rectangle still has to be somewhere.
func (w *World) buildRegionShapes() bool {
	shapes := w.cfg.RegionShapes
	if len(shapes) == 0 {
		w.shapeIndex = nil
		return false
	}
	g := &regionIndexGrid{cols: regionShapeGridSize, rows: regionShapeGridSize}
	g.at = make([]int, g.cols*g.rows)
	rest := len(shapes) // "everywhere else" sits after the drawn ones
	for i := range g.at {
		g.at[i] = rest
	}
	byKey := map[byte]int{}
	for i := range shapes {
		if k := shapes[i].Key; k != 0 && k != regionUnpainted {
			byKey[k] = i
		}
	}
	painted := w.cfg.RegionMap
	for r := 0; r < g.rows; r++ {
		// The centre of each cell, in fractions of the map.
		fy := (float64(r) + 0.5) / float64(g.rows)
		for c := 0; c < g.cols; c++ {
			fx := (float64(c) + 0.5) / float64(g.cols)
			// Later rectangles win, which is how a drawing program works:
			// what is drawn on top is what is there.
			for i := range shapes {
				s := &shapes[i]
				if s.W <= 0 || s.H <= 0 {
					continue // a painted region: the map below says where
				}
				if fx >= s.X && fx < s.X+s.W && fy >= s.Y && fy < s.Y+s.H {
					g.at[r*g.cols+c] = i
				}
			}
			// And the painted map over the top of them, because painting a
			// cell is the more particular thing to have said about it.
			if k := paintedKeyAt(painted, fx, fy); k != 0 {
				if i, ok := byKey[k]; ok {
					g.at[r*g.cols+c] = i
				}
			}
		}
	}
	g.measure(len(shapes) + 1)
	w.shapeIndex = g
	return true
}

// measure works out the box round each region, "everywhere else" included.
func (g *regionIndexGrid) measure(n int) {
	g.bounds = make([]regionBox, n)
	for i := range g.bounds {
		g.bounds[i] = regionBox{minX: 1, minY: 1}
	}
	for r := 0; r < g.rows; r++ {
		y0, y1 := float64(r)/float64(g.rows), float64(r+1)/float64(g.rows)
		for c := 0; c < g.cols; c++ {
			i := g.at[r*g.cols+c]
			if i < 0 || i >= n {
				continue
			}
			x0, x1 := float64(c)/float64(g.cols), float64(c+1)/float64(g.cols)
			b := &g.bounds[i]
			b.minX, b.minY = math.Min(b.minX, x0), math.Min(b.minY, y0)
			b.maxX, b.maxY = math.Max(b.maxX, x1), math.Max(b.maxY, y1)
		}
	}
}

// regionUnpainted is the character for a cell no region was painted on.
const regionUnpainted = '.'

// paintedKeyAt is the character painted at a point of the map, in fractions.
// A short row, and a row past the end, are unpainted - the same forgiveness
// the spawn map gives.
func paintedKeyAt(rows []string, fx, fy float64) byte {
	if len(rows) == 0 {
		return 0
	}
	r := clampInt(int(fy*float64(len(rows))), 0, len(rows)-1)
	row := rows[r]
	if len(row) == 0 {
		return 0
	}
	c := clampInt(int(fx*float64(len(row))), 0, len(row)-1)
	if k := row[c]; k != regionUnpainted {
		return k
	}
	return 0
}

// regionShapeAt is the lookup. Positions are kept inside the world by the
// boundary rule, so the clamp is a belt on top of braces.
func (w *World) regionShapeAt(x, y float64) int {
	g := w.shapeIndex
	c := clampInt(int(x/w.cfg.Width*float64(g.cols)), 0, g.cols-1)
	r := clampInt(int(y/w.cfg.Height*float64(g.rows)), 0, g.rows-1)
	return g.at[r*g.cols+c]
}

// shapedRegionCount is how many regions a drawing asks for: the ones drawn,
// plus the one that is everywhere else.
func (w *World) shapedRegionCount() int { return len(w.cfg.RegionShapes) + 1 }

// applyRegionShapes writes what the author set onto the regions, after the
// world's own spreads have drawn theirs. Nought is left alone, so a rectangle
// that only names a place keeps the country the world drew there.
func (w *World) applyRegionShapes() {
	for i := range w.cfg.RegionShapes {
		s := &w.cfg.RegionShapes[i]
		if i >= len(w.regions) {
			break
		}
		if s.Shelter > 0 {
			w.regions[i].Shelter = clamp(s.Shelter, 0, 2)
		}
		if s.Food > 0 {
			w.regions[i].Food = clamp(s.Food, 0, 2)
		}
		if s.Special > 0 {
			w.regions[i].Special = clamp(s.Special, 0, 2)
		}
		if s.Enemies > 0 {
			w.regions[i].Enemies = clamp(s.Enemies, 0, 2)
		}
	}
}

// GoalRegions is which regions the map marked as goals, by index. Read only,
// and the simulation never asks: it is here for whoever is running the game.
func (w *World) GoalRegions() []int {
	var out []int
	for i := range w.cfg.RegionShapes {
		if w.cfg.RegionShapes[i].Goal && i < len(w.regions) {
			out = append(out, i)
		}
	}
	return out
}

// GoalPrice is what the game asks for a village in this region, in coins, and
// GoalYears how long one founded there keeps its line up. Read only, and zero
// where the author said nothing - the game decides what a silent map means.
func (w *World) GoalPrice(region int) float64 {
	if region < 0 || region >= len(w.cfg.RegionShapes) {
		return 0
	}
	return w.cfg.RegionShapes[region].Price
}

func (w *World) GoalYears(region int) float64 {
	if region < 0 || region >= len(w.cfg.RegionShapes) {
		return 0
	}
	return w.cfg.RegionShapes[region].Years
}

// RegionCentre is a spot inside one block, for an interface that has to point
// at it - a label, a sign, a camera.
//
// It is not the middle of the block's bounding box, which is the obvious
// answer and the wrong one: a painted region may be two patches in opposite
// corners, and the middle of what they span is somewhere neither of them is.
// This is the middle of the cells the block actually holds, moved to the
// nearest one of them when that middle falls outside.
//
// Read only, and it draws nothing: the grid it walks is the one the world
// built when it was laid out.
func (w *World) RegionCentre(i int) (float64, float64) {
	const step = 64 // a coarse sweep: this is for pointing, not for measuring
	var sumX, sumY, n float64
	type spot struct{ x, y float64 }
	var cells []spot
	for gy := 0; gy < step; gy++ {
		y := (float64(gy) + 0.5) / step * w.cfg.Height
		for gx := 0; gx < step; gx++ {
			x := (float64(gx) + 0.5) / step * w.cfg.Width
			if w.regionIndexAt(x, y) != i {
				continue
			}
			cells = append(cells, spot{x, y})
			sumX, sumY, n = sumX+x, sumY+y, n+1
		}
	}
	if n == 0 {
		minX, minY, maxX, maxY := w.regionBounds(i)
		return (minX + maxX) / 2, (minY + maxY) / 2
	}
	cx, cy := sumX/n, sumY/n
	if w.regionIndexAt(cx, cy) == i {
		return cx, cy
	}
	best, bestD := cells[0], math.Inf(1)
	for _, c := range cells {
		if d := (c.x-cx)*(c.x-cx) + (c.y-cy)*(c.y-cy); d < bestD {
			best, bestD = c, d
		}
	}
	return best.x, best.y
}

// RegionBounds is the ground one block covers, for an interface drawing it.
// Read only.
func (w *World) RegionBounds(i int) (minX, minY, maxX, maxY float64) {
	return w.regionBounds(i)
}

// RegionNames is what the author called each region, for a map editor or a
// panel to show. Read only; the simulation never sees a name.
func (w *World) RegionNames() []string {
	out := make([]string, len(w.regions))
	for i := range w.cfg.RegionShapes {
		if i < len(out) {
			out[i] = w.cfg.RegionShapes[i].Name
		}
	}
	if len(w.cfg.RegionShapes) > 0 && len(out) > len(w.cfg.RegionShapes) {
		out[len(w.cfg.RegionShapes)] = "elsewhere"
	}
	return out
}

// DrawnRegions says whether this world's regions were drawn by an author -
// rectangles or painted cells - rather than cut into equal blocks. Read only,
// for a viewer that has to know whether a region is a block on a grid before
// it draws one.
func (w *World) DrawnRegions() bool { return w.shapeIndex != nil }
