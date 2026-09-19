package engine

// Regions an author draws, rather than a grid the world cuts for itself
// (2026-09-19). Until now the regions were RegionCols x RegionRows equal
// blocks; a map may now name its own, as rectangles.
//
// Why rectangles and not any shape: every rule that reads a region reads it
// through regionIndexAt, and what that has to be is fast and exact, not
// general. A rectangle is enough to say "this valley", "that shore" - and if
// something more is ever wanted, the index below is a lookup table already and
// does not care what filled it in.

// RegionShape is one region as its author drew it: a rectangle in fractions of
// the map (0 to 1), so that the same drawing describes the same country
// whatever size the world is given.
//
// The four figures are the region's own, and nought means "leave what the
// world's spreads drew for it" - so an author can name a place without having
// to decide everything about it.
type RegionShape struct {
	Name                            string
	X, Y, W, H                      float64
	Shelter, Food, Special, Enemies float64

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
}

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
	for r := 0; r < g.rows; r++ {
		// The centre of each cell, in fractions of the map.
		fy := (float64(r) + 0.5) / float64(g.rows)
		for c := 0; c < g.cols; c++ {
			fx := (float64(c) + 0.5) / float64(g.cols)
			// Later rectangles win, which is how a drawing program works:
			// what is drawn on top is what is there.
			for i := range shapes {
				s := &shapes[i]
				if fx >= s.X && fx < s.X+s.W && fy >= s.Y && fy < s.Y+s.H {
					g.at[r*g.cols+c] = i
				}
			}
		}
	}
	w.shapeIndex = g
	return true
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
