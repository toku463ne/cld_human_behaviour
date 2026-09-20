package engine

import "sort"

// Where the world grows things, painted cell by cell (2026-09-20).
//
// What was tangled up before this. Richness was the coarse region's own
// number: twelve blocks of 200x200, drawn from FoodSpread and read through
// regionAt. Two different things were living in that one figure - what an
// agent believes about a stretch of country, which wants to be coarse because
// a memory is small (stage 15b), and where a plant actually comes up, which
// wants to be whatever shape the author drew. This separates them. The map
// below decides where plants land; the region keeps the average of it for the
// beliefs to be about.
//
// The counting that shaped this, before a line of it was written (PLAN.md
// P16-2): cutting the world into 48 or 108 blocks instead of 12 halves
// richGain (0.07 -> 0.03 and 0.04, both **), and the two finer arms cannot be
// told apart. A block smaller than what a body can see stops being somewhere
// to go - whichever way it walks, several blocks are in view and the richness
// it experiences is the world's average. The default region grid is the only
// one whose blocks are bigger than PerceptionRadius, and that is why it is
// still the default.
//
// So this is not a finer grid. It is a paintable one, and what an author gains
// is the shape and the size of the good ground rather than its resolution.

// richOrdinary is the character for ground nobody painted.
const richOrdinary = '.'

// richOf is the whole of the vocabulary of Config.RichMap: a digit is that
// many fifths, so '5' is ordinary ground, '0' grows nothing at all and '9'
// grows nearly twice what ordinary ground does. A dot, a short row and any
// character nobody knows are ordinary - the same forgiveness SpawnMap gives,
// and for the same reason: a drawing may be incomplete without the world
// having to stop.
//
// Fifths rather than tenths so that ordinary ground has a character of its
// own ('5') and the scale reaches the 0..2 the region weights already use.
func richOf(c byte) float64 {
	if c >= '0' && c <= '9' {
		return float64(c-'0') / 5
	}
	return 1
}

// richGrid is the painted map, with the running total kept beside it so that
// drawing a spot is a binary search rather than a walk over every cell. It is
// built once and never changes: the paint is part of Config, and Config is
// fixed when the world is built.
type richGrid struct {
	cols, rows int
	at         []float64
	cum        []float64
	total      float64
}

// buildRich reads Config.RichMap, and returns nil when there is nothing
// painted - which is what keeps every world before this one identical, down
// to the random source, because nil takes this rule out of the run entirely.
func buildRich(cfg *Config) *richGrid {
	rows := cfg.RichMap
	if len(rows) == 0 {
		return nil
	}
	cols := 0
	for _, r := range rows {
		if n := len(r); n > cols {
			cols = n
		}
	}
	if cols == 0 {
		return nil
	}
	g := &richGrid{cols: cols, rows: len(rows)}
	g.at = make([]float64, cols*len(rows))
	g.cum = make([]float64, cols*len(rows))
	for y, row := range rows {
		for x := 0; x < cols; x++ {
			c := byte(richOrdinary)
			if x < len(row) {
				c = row[x]
			}
			v := richOf(c)
			g.at[y*cols+x] = v
			g.total += v
			g.cum[y*cols+x] = g.total
		}
	}
	if g.total <= 0 {
		// A map that grows nothing anywhere is not a map, it is a mistake,
		// and honouring it would stop the world growing at all. Ignored the
		// way a fish painted onto dry land is ignored.
		return nil
	}
	return g
}

// valueAt is how well the ground at this spot grows things.
func (g *richGrid) valueAt(x, y, width, height float64) float64 {
	c := clampInt(int(x/width*float64(g.cols)), 0, g.cols-1)
	r := clampInt(int(y/height*float64(g.rows)), 0, g.rows-1)
	return g.at[r*g.cols+c]
}

// pick draws a cell in proportion to how well it grows things, and returns
// the box that cell covers.
//
// The number comes in rather than being drawn here so that this stays a pure
// function of the map: the world's single random source is the caller's, and
// a rule that reached for its own would break the one thing every test in
// this package leans on.
func (g *richGrid) pick(r, width, height float64) (minX, minY, maxX, maxY float64) {
	i := sort.Search(len(g.cum), func(i int) bool { return g.cum[i] > r })
	if i >= len(g.cum) {
		i = len(g.cum) - 1
	}
	c, row := i%g.cols, i/g.cols
	cw, ch := width/float64(g.cols), height/float64(g.rows)
	return float64(c) * cw, float64(row) * ch, float64(c+1) * cw, float64(row+1) * ch
}

// summariseRich writes the painted map onto the regions.
//
// Everything that reads a region's richness - what agents believe about it
// (regionlore.go), what a skill is worth there (skill.go), how much of the
// world's food the cold reaches (climate.go) - then reads a summary of the
// ground that actually grows things, instead of a number drawn beside it. It
// is the whole point of separating the two: the belief stays coarse because a
// memory is small, and it is still a belief about something true.
//
// It draws nothing from the random source, so a painted world does not depend
// on how many regions it happens to have been cut into.
func (w *World) summariseRich() {
	if w.rich == nil || len(w.regions) == 0 {
		return
	}
	sum := make([]float64, len(w.regions))
	n := make([]float64, len(w.regions))
	for r := 0; r < w.rich.rows; r++ {
		y := (float64(r) + 0.5) / float64(w.rich.rows) * w.cfg.Height
		for c := 0; c < w.rich.cols; c++ {
			x := (float64(c) + 0.5) / float64(w.rich.cols) * w.cfg.Width
			if i := w.regionIndexAt(x, y); i >= 0 && i < len(w.regions) {
				sum[i] += w.rich.at[r*w.rich.cols+c]
				n[i]++
			}
		}
	}
	for i := range w.regions {
		if n[i] > 0 {
			w.regions[i].Food = clamp(sum[i]/n[i], 0, 2)
		}
	}
}

// pickPaintedRich draws a position inside the painted squares, weighted by how
// well each of them grows things.
//
// It exists because the two paintings answer different questions and have to
// compose: SpawnMap says where a plant may come up at all, RichMap says how
// well it does there. Without this a map that painted both would silently
// ignore the second - the mask is checked first, and whatever it picked was
// picked uniformly.
//
// It takes the same three numbers from the random source that the unweighted
// draw does, and falls back to that draw exactly when there is nothing to
// weight by, so a world with a mask and no painting is unchanged.
func (w *World) pickPaintedRich(k spawnKind, margin float64) (float64, float64) {
	cells := w.spawnCells[k]
	if w.rich == nil || len(cells) == 0 {
		return w.pickPainted(k, margin)
	}
	total := 0.0
	for _, c := range cells {
		total += w.rich.valueAt(c.x, c.y, w.cfg.Width, w.cfg.Height)
	}
	if total <= 0 {
		// Every square the mask allows is painted bare. The mask is the more
		// particular thing the author said, so it wins and the squares are
		// drawn between evenly - the alternative is a world that grows
		// nothing at all because two drawings disagreed.
		return w.pickPainted(k, margin)
	}
	r := w.rng.Float64() * total
	pick := cells[len(cells)-1]
	for _, c := range cells {
		r -= w.rich.valueAt(c.x, c.y, w.cfg.Width, w.cfg.Height)
		if r <= 0 {
			pick = c
			break
		}
	}
	x := clamp(pick.x+w.randRange(-pick.w/2, pick.w/2), margin, w.cfg.Width-margin)
	y := clamp(pick.y+w.randRange(-pick.h/2, pick.h/2), margin, w.cfg.Height-margin)
	return x, y
}
