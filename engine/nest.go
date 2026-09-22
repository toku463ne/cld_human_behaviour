package engine

import "math"

// What the map's nests are holding (2026-09-22, counting for TODO 18).
//
// The question this exists to answer, before a line of the rule is written: is
// there anything near a nest to give a rate or a cap anything to bite on?
//
// Stage 58 painted where the enemies arrive and the world lost the pattern
// inside one lifetime - prowlGain 0.00 +/- 0.02 over 24 seeds - because a
// beast strays 270 from where it started and a region is 200 wide. If that
// still holds with the nests painted as cells, then a per-nest rate only
// exists at the instant of arrival and a per-nest cap counts a crowd that has
// already walked away. Both shrink to bookkeeping, and the counting has to say
// so before the code does.
//
// Everything here is read only: it draws nothing from the random source and
// writes nothing to the world, so a run measured is the run that would have
// happened anyway.

// nestRoomGrid is how finely Room samples the map. The nests are rectangles
// with a radius round them and they overlap each other, so the covered area is
// counted rather than derived - 80 by 60 over a world of 800 by 600 is one
// sample per ten units, which is well under the smallest radius anything here
// is given.
const nestRoomGrid = 80

// NestUse is what the painted nests hold. Read only.
type NestUse struct {
	// Nests is how many cells the map painted, over every sort.
	Nests int

	// Enemies is how many are alive, and AtCap is one when the world is
	// holding as many as MaxEnemies allows - the reading that says whether a
	// rate can matter at all. A ceiling the world sits against makes every
	// rate above it the same rate.
	Enemies float64
	AtCap   float64

	// Hold is the share of the living enemies standing within their own
	// sort's Roam of a nest of that sort, and Room the share of the map that
	// is within such a radius of any nest at all.
	//
	// The pair is the point. Hold on its own says nothing: two nests with a
	// hundred units of roam cover a third of this world, so a population
	// spread evenly over the map already "holds" a third. Gain is the
	// difference, and it is the figure stage 58's prowlGain is the ancestor
	// of.
	Hold float64
	Room float64
	Gain float64

	// What the caps are doing (TODO 18). Crowd is the mean number of a sort
	// standing within roam of one of its nests, Capped the share of the
	// painted nests that are full, and Stopped the share of the arrivals the
	// world tried to let in that every nest refused.
	//
	// Stopped is the one that says whether the rule fires at all. A cap that
	// is never reached is a cap nobody should make default, and a world that
	// sits against MaxEnemies anyway would show Stopped near nought however
	// tight the nests were drawn.
	Crowd   float64
	Capped  float64
	Stopped float64

	// Rated is the share of everybody standing by a nest who is standing by
	// the heaviest one the map painted, and nought in a world that painted no
	// rates. It is how the weighting is read in the living population rather
	// than at the moment of arrival: two nests painted 1 and 9 hand the heavy
	// one nine arrivals in ten, and this says how many of them are still
	// there.
	Rated float64
}

// Nesting reports what the map's nests are holding. Read only; it is empty in
// a world whose map painted no nests, which is every world before 2026-09-20.
func (w *World) Nesting() NestUse {
	var out NestUse
	for _, cells := range w.enemyKindCells {
		out.Nests += len(cells)
	}
	var enemies, held float64
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive || a.Species != SpeciesEnemy {
			continue
		}
		enemies++
		if w.nearOwnNest(a) {
			held++
		}
	}
	out.Enemies = enemies
	if w.cfg.MaxEnemies > 0 && int(enemies) >= w.cfg.MaxEnemies {
		out.AtCap = 1
	}
	if out.Nests == 0 {
		return out
	}
	if enemies > 0 {
		out.Hold = held / enemies
	}
	out.Room = w.nestRoom()
	out.Gain = out.Hold - out.Room

	var nests, crowd, full float64
	for kind, cells := range w.enemyKindCells {
		for _, c := range cells {
			nests++
			n := w.nestCrowd(kind, c)
			crowd += n
			if w.nestsCapped() && n >= c.cap {
				full++
			}
		}
	}
	if nests > 0 {
		out.Crowd, out.Capped = crowd/nests, full/nests
	}
	if len(w.cfg.NestRateMap) > 0 && crowd > 0 {
		best, at := -1.0, 0.0
		for kind, cells := range w.enemyKindCells {
			for _, c := range cells {
				if c.rate > best {
					best, at = c.rate, w.nestCrowd(kind, c)
				}
			}
		}
		out.Rated = at / crowd
	}
	if w.nestTries > 0 {
		out.Stopped = float64(w.nestRefused) / float64(w.nestTries)
	}
	return out
}

// nearOwnNest is whether this body is standing within its own sort's roam of a
// nest the map painted for that sort.
//
// Its own sort rather than any nest, because that is what a per-nest cap would
// have to count: a nest can only be full of the thing it produces.
func (w *World) nearOwnNest(a *Agent) bool {
	kind := int(a.Kind)
	if kind < 0 || kind >= len(w.enemyKindCells) {
		return false
	}
	roam := w.roamOf(kind)
	for _, c := range w.enemyKindCells[kind] {
		if distToCell(a.X, a.Y, c.cell) <= roam {
			return true
		}
	}
	return false
}

// roamOf is how far this sort goes from its nest for nothing, in the world's
// own units. Half a region when the row says nothing, which is what
// homeRoamOf says in the units the utility is charged in.
func (w *World) roamOf(kind int) float64 {
	kinds := w.enemyKinds()
	if kind >= 0 && kind < len(kinds) && kinds[kind].Roam > 0 {
		return kinds[kind].Roam
	}
	return 0.5 * max(w.cfg.Width/float64(max(w.cfg.RegionCols, 1)), 1)
}

// nestRoom is the share of the map lying within some sort's roam of one of its
// own nests, counted on a grid because the shapes overlap.
func (w *World) nestRoom() float64 {
	rows := max(nestRoomGrid*int(w.cfg.Height)/max(int(w.cfg.Width), 1), 1)
	var inside, all float64
	for r := 0; r < rows; r++ {
		y := (float64(r) + 0.5) * w.cfg.Height / float64(rows)
		for c := 0; c < nestRoomGrid; c++ {
			x := (float64(c) + 0.5) * w.cfg.Width / nestRoomGrid
			all++
			for kind, cells := range w.enemyKindCells {
				roam := w.roamOf(kind)
				near := false
				for _, cell := range cells {
					if distToCell(x, y, cell.cell) <= roam {
						near = true
						break
					}
				}
				if near {
					inside++
					break
				}
			}
		}
	}
	if all == 0 {
		return 0
	}
	return inside / all
}

// distToCell is how far a spot is from the ground a nest stands on - nought
// anywhere on the cell itself, and the distance to its edge outside it.
func distToCell(x, y float64, c cell) float64 {
	dx := math.Max(0, math.Abs(x-c.x)-c.w/2)
	dy := math.Max(0, math.Abs(y-c.y)-c.h/2)
	return math.Hypot(dx, dy)
}
