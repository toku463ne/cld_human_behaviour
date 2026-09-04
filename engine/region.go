package engine

// Regions are the world's own coarse divisions (decision #38).
//
// There is one kind of region and everything that varies by place uses it: how
// sheltered the resting is here (stage 14), how well the plants grow here
// (stage 15), and whatever a region-by-region reset eventually needs. Making a
// second division for the second use is how a world ends up with three
// overlapping maps that mean nearly the same thing.
//
// A region is not a wall. Nothing stops an agent walking out of one, nothing
// tells it which one it is in, and no rule reads a region ID. What a region
// does is change a number at a place, and an agent finds that out by being
// there.
//
// It is also not the spatial index (grid.go) and not the sight grid
// (sight.go). The index is an optimisation that must not change results; the
// sight grid is how far an agent can see. This is the ground itself, and it is
// coarse on purpose: fine regions would average out under any amount of
// wandering, and there would be no such thing as a good place to be.

// region is one block of the world.
type region struct {
	// Shelter multiplies what resting here is reckoned to cost (stage 14).
	// Below one is somewhere with its back covered; above one is open ground
	// where anybody around is a problem. Exactly one is the ordinary world.
	Shelter float64
}

// regionsOf lays the world out in blocks and draws what each one is like.
//
// Nothing is drawn when the spread is zero: the whole world comes out ordinary
// and, because no number is taken from the random source, the run is identical
// to one from before regions existed. That is the arm this stage is measured
// against, and it has to be exact rather than merely similar.
func (w *World) buildRegions() {
	cfg := &w.cfg
	cols, rows := max(cfg.RegionCols, 1), max(cfg.RegionRows, 1)
	w.regions = make([]region, cols*rows)
	for i := range w.regions {
		w.regions[i] = region{Shelter: 1}
	}
	if cfg.ShelterSpread <= 0 {
		return
	}
	for i := range w.regions {
		w.regions[i].Shelter = clamp(w.randRange(1-cfg.ShelterSpread, 1+cfg.ShelterSpread), 0, 2)
	}
}

// regionAt is which block a position falls in. Positions are kept inside the
// world by the boundary rule, so the clamp is a belt on top of braces.
func (w *World) regionAt(x, y float64) *region {
	if len(w.regions) == 0 {
		return nil
	}
	cols, rows := max(w.cfg.RegionCols, 1), max(w.cfg.RegionRows, 1)
	c := clampInt(int(x/w.cfg.Width*float64(cols)), 0, cols-1)
	r := clampInt(int(y/w.cfg.Height*float64(rows)), 0, rows-1)
	return &w.regions[r*cols+c]
}

// shelterAt is how exposed resting at this spot is reckoned to be, as a
// multiplier on RestExposureWeight. One everywhere is the world as it was.
//
// It is the only thing a region does today, and it does it in one place: the
// rest option's exposure term. There is no second formula and no new state -
// the whole stage is a multiplication (decision #36).
func (w *World) shelterAt(x, y float64) float64 {
	if r := w.regionAt(x, y); r != nil {
		return r.Shelter
	}
	return 1
}

// --- reading it out ---------------------------------------------------------

// RegionView is one block of the world, for the viewer. Read only.
type RegionView struct {
	MinX, MinY, MaxX, MaxY float64
	Shelter                float64
}

// Regions reports the blocks the world is divided into. Read only.
func (w *World) Regions() []RegionView {
	cols, rows := max(w.cfg.RegionCols, 1), max(w.cfg.RegionRows, 1)
	out := make([]RegionView, 0, len(w.regions))
	cw, ch := w.cfg.Width/float64(cols), w.cfg.Height/float64(rows)
	for i := range w.regions {
		c, r := i%cols, i/cols
		out = append(out, RegionView{
			MinX: float64(c) * cw, MinY: float64(r) * ch,
			MaxX: float64(c+1) * cw, MaxY: float64(r+1) * ch,
			Shelter: w.regions[i].Shelter,
		})
	}
	return out
}

// ShelterAtView is how exposed resting at this spot is, for the viewer. Read
// only; agents get the same figure through Perception.
func (w *World) ShelterAtView(x, y float64) float64 { return w.shelterAt(x, y) }

// ShelterUse is where the population actually rests, against where it is.
//
// The two are the point of the stage. A world where resting is cheaper in some
// places should end up with the resting happening in those places, and the
// only way to tell that from "agents are wherever they are" is to compare the
// shelter under the ones lying down with the shelter under everybody.
type ShelterUse struct {
	Resting float64 // mean shelter where the resting agents are
	All     float64 // ... and where the population as a whole is
}

// Shelter reports where the resting is happening. Read only.
func (w *World) Shelter() ShelterUse {
	var out ShelterUse
	var resting, all float64
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		s := w.shelterAt(a.X, a.Y)
		out.All += s
		all++
		if a.Action.Kind == ActRest {
			out.Resting += s
			resting++
		}
	}
	if all > 0 {
		out.All /= all
	}
	if resting > 0 {
		out.Resting /= resting
	} else {
		// Nobody is lying down at this instant, which is no evidence either
		// way rather than evidence of nought. Reporting a zero here is what a
		// first version did, and averaging those zeros over a run produced a
		// confident gain of 0.14 in a world where every region was identical.
		out.Resting = out.All
	}
	return out
}
