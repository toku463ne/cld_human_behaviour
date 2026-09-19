package engine

// Where things are allowed to come up, as the author painted it (2026-09-19).
//
// One more layer on the map, one character per cell, saying which spawns may
// use that square. Empty is every world before this: plants come up wherever
// the region weights send them, fish anywhere in the water, enemies wherever
// stage 58's weighting puts them.
//
// What it does NOT change is how much comes up. FoodSpawnRate still says how
// many plants a tick, EnemySpawnTicks still says how often an enemy walks in:
// this only says where they may land, so the conservation stage 15a insisted
// on holds inside the painted squares instead of over the whole map.

// The vocabulary of Config.SpawnMap. One character per cell, and a row may be
// short - anything past its end, and any character nobody knows, is bare
// ground where nothing comes up.
const (
	spawnNone   = '.'
	spawnPlant  = 'p'
	spawnFish   = 'f'
	spawnEnemy  = 'e'
	spawnFood   = 'F' // plants and fish both
	spawnAll    = '*'
	spawnAllAlt = 'a'
)

// spawnKind is which list a cell belongs to.
type spawnKind int

const (
	spawnKindPlant spawnKind = iota
	spawnKindFish
	spawnKindEnemy
	numSpawnKinds
)

// buildSpawnCells reads the painted layer into one list of cells per kind.
//
// It is built once when the world is built, like the water cells it mirrors:
// the map does not move, and an unpainted world gets three empty lists, which
// is what keeps every world before this drawing exactly the random numbers it
// always did.
//
// The cells are the terrain's own squares when there is a map, so that a
// painted square is exactly the square the author painted. Without terrain the
// painted layer is laid over the world in its own right, which lets a flat
// world be painted too.
func (w *World) buildSpawnCells() {
	for i := range w.spawnCells {
		w.spawnCells[i] = nil
	}
	rows := w.cfg.SpawnMap
	if len(rows) == 0 {
		return
	}
	cols := 0
	for _, r := range rows {
		cols = max(cols, len(r))
	}
	if cols == 0 {
		return
	}
	cw := w.cfg.Width / float64(cols)
	chh := w.cfg.Height / float64(len(rows))
	for r, row := range rows {
		for c := 0; c < cols; c++ {
			var painted byte = spawnNone
			if c < len(row) {
				painted = row[c]
			}
			kinds := spawnKindsOf(painted)
			if len(kinds) == 0 {
				continue
			}
			square := cell{
				x: (float64(c) + 0.5) * cw,
				y: (float64(r) + 0.5) * chh,
				w: cw, h: chh,
			}
			for _, k := range kinds {
				w.spawnCells[k] = append(w.spawnCells[k], square)
			}
		}
	}
}

// spawnKindsOf is what one painted character allows.
func spawnKindsOf(c byte) []spawnKind {
	switch c {
	case spawnPlant:
		return []spawnKind{spawnKindPlant}
	case spawnFish:
		return []spawnKind{spawnKindFish}
	case spawnEnemy:
		return []spawnKind{spawnKindEnemy}
	case spawnFood:
		return []spawnKind{spawnKindPlant, spawnKindFish}
	case spawnAll, spawnAllAlt:
		return []spawnKind{spawnKindPlant, spawnKindFish, spawnKindEnemy}
	}
	return nil
}

// paintedFor says whether the author painted anywhere for this kind. When
// nothing is painted the old rule stands, untouched.
func (w *World) paintedFor(k spawnKind) bool { return len(w.spawnCells[k]) > 0 }

// pickPainted draws a position inside one of the painted squares, uniformly
// over the squares and uniformly inside the one it lands on.
//
// Two draws, the same two a weighted region draw takes, so a world that paints
// its map spends the random source the way a world that weights its regions
// does.
func (w *World) pickPainted(k spawnKind, margin float64) (float64, float64) {
	cells := w.spawnCells[k]
	c := cells[w.rng.Intn(len(cells))]
	x := clamp(c.x+w.randRange(-c.w/2, c.w/2), margin, w.cfg.Width-margin)
	y := clamp(c.y+w.randRange(-c.h/2, c.h/2), margin, w.cfg.Height-margin)
	return x, y
}

// paintedWater is the painted fish squares that are actually water. A fish
// painted onto dry land is ignored rather than refused: the map is a drawing,
// and a drawing may be wrong about where the water is without the world having
// to stop.
func (w *World) paintedWater() []cell {
	cells := w.spawnCells[spawnKindFish]
	if len(cells) == 0 || w.ground == nil {
		return nil
	}
	out := make([]cell, 0, len(cells))
	for _, c := range cells {
		if w.terrainAt(c.x, c.y).Kind == GroundWater {
			out = append(out, c)
		}
	}
	return out
}
