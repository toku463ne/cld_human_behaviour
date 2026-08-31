package engine

// This file counts how many of each species there are. It is the last of the
// "should be" measurements (PLAN.md stage 6.5), and the one stage 11 is judged
// by: criterion A asks that the different species each hold a certain number,
// read as the mean over a window, with oscillation allowed.
//
// Three things follow from how that criterion is worded, and they are the whole
// design of this file.
//
// A count at one tick is not an answer. Two species can share a frame and still
// be a fortnight from being one species: whether they coexist is a property of
// a stretch of time, not of a moment. So the readings are kept in a sliding
// window and reported as a mean over it.
//
// Oscillation being allowed means the swing itself does not condemn a run, so
// the amplitude is not the figure to read. What matters is how close the low
// point of the swing comes to zero, because that is where a species stops
// coming back: with no immigration, zero is absorbing. Trough is that figure.
//
// And a species that is already gone leaves no trace in a window that has moved
// past its death, so extinction is remembered for the whole run rather than
// read off the window.
//
// Today there is one species, so what this measures is the human population's
// own swing. That is not a placeholder value: it is the baseline the two
// species world will be compared against, and it is measured with the same
// ruler beforehand.
//
// Like the rest of the measurement files it only reads. Nothing here feeds back
// into a decision, and no agent knows which species it belongs to.

// The cadence and window the experiment runner censuses at.
//
// They are constants for the same reason DefaultClusterLinkDist is: Min and Max
// over a window depend on how many readings the window holds, so a trough taken
// at one cadence is not comparable with one taken at another. The window is the
// 5000 ticks PLAN.md names in criterion A.
const (
	DefaultCensusStep   = 25
	DefaultCensusWindow = 5000
)

// SpeciesCensus is how one species fared over the window.
type SpeciesCensus struct {
	Species Species

	// Mean, Min and Max are the population over the window. Mean is the figure
	// criterion A puts a band around; Min is the one that says whether the
	// species nearly went out.
	Mean     float64
	Min, Max int

	// Share is Mean as a fraction of the mean total population.
	Share float64

	// Trough is Min divided by Mean: how far down the low point of the swing
	// goes, as a fraction of the ordinary level. 1 is a flat population, and 0
	// is one that touched zero. This is the figure to read, not Swing: the
	// criterion allows oscillation, so amplitude on its own says nothing, and
	// only the low point decides whether a species survives it.
	Trough float64

	// Swing is (Max - Min) / Mean: the size of the oscillation, kept because
	// "allowed to oscillate" is worth being able to see rather than only
	// tolerate.
	Swing float64

	// Extinct says the species was at zero at the last reading, having been
	// alive at some point, and ExtinctTick is the first reading at which it
	// was found empty (-1 if it never was).
	//
	// They are two facts rather than one because nothing today can bring a
	// species back, but a later stage might (PLAN.md's regional reset drops
	// fresh individuals in), and a species that came back should not still be
	// reported as extinct while the tick it emptied at stays worth knowing.
	Extinct     bool
	ExtinctTick int
}

// Census is the population of every species over a window of the run.
type Census struct {
	// Window is the span the readings cover, in ticks, and Samples is how many
	// there were. A mean over three readings is not a mean over a window.
	Window  int
	Samples int

	// Population is the mean total over the window, which the shares are
	// fractions of.
	Population float64

	// Species holds one entry per species ever seen alive, in species order.
	// A species that has died out keeps its entry, with Extinct set: dropping
	// it would turn the loss of a species into a table that simply got shorter.
	Species []SpeciesCensus
}

// Living counts the species that were still there at the last reading.
func (c Census) Living() int {
	n := 0
	for _, s := range c.Species {
		if !s.Extinct {
			n++
		}
	}
	return n
}

// Rarest is the species with the smallest mean population, which is the one
// coexistence stands or falls on: the others can look healthy while it goes.
// ok is false when no species has been seen at all.
func (c Census) Rarest() (SpeciesCensus, bool) {
	if len(c.Species) == 0 {
		return SpeciesCensus{}, false
	}
	rarest := c.Species[0]
	for _, s := range c.Species[1:] {
		if s.Mean < rarest.Mean {
			rarest = s
		}
	}
	return rarest, true
}

type censusReading struct {
	tick   int
	counts []int // indexed by species
}

// CensusTracker counts the population of each species through a run.
//
// It keeps no roster of the species there are: it counts whatever labels it
// finds on the agents and grows to fit. Stage 11 can add a species without
// touching this file, and a species that never appears never appears in the
// results either.
type CensusTracker struct {
	window   int
	readings []censusReading

	// seen and emptyTick are kept for the whole run, not the window, because
	// the death of a species is invisible once the window has moved past it.
	seen      []bool
	emptyTick []int
}

// NewCensusTracker keeps the readings of the last window ticks.
func NewCensusTracker(window int) *CensusTracker {
	if window < 1 {
		window = 1
	}
	return &CensusTracker{window: window}
}

// Observe takes one count of w. Call it every DefaultCensusStep ticks: the
// window is measured in ticks, so an irregular cadence only costs resolution,
// but Min and Max are only as good as the readings that were taken.
func (t *CensusTracker) Observe(w *World) {
	agents := w.Agents()
	counts := make([]int, len(t.seen))
	for i := range agents {
		s := int(agents[i].Species)
		for len(counts) <= s {
			counts = append(counts, 0)
		}
		counts[s]++
	}
	t.grow(len(counts))

	tick := w.Tick()
	for s := range t.seen {
		switch {
		case s < len(counts) && counts[s] > 0:
			t.seen[s] = true
		case t.seen[s] && t.emptyTick[s] < 0:
			t.emptyTick[s] = tick
		}
	}
	t.readings = append(t.readings, censusReading{tick: tick, counts: counts})

	// Drop the readings that have fallen out of the window.
	keep := 0
	for _, r := range t.readings {
		if tick-r.tick <= t.window {
			t.readings[keep] = r
			keep++
		}
	}
	t.readings = t.readings[:keep]
}

// grow makes room for n species.
func (t *CensusTracker) grow(n int) {
	for len(t.seen) < n {
		t.seen = append(t.seen, false)
		t.emptyTick = append(t.emptyTick, -1)
	}
}

// Result reads off the window gathered so far.
func (t *CensusTracker) Result() Census {
	out := Census{Samples: len(t.readings)}
	if out.Samples == 0 {
		return out
	}
	out.Window = t.readings[len(t.readings)-1].tick - t.readings[0].tick

	n := len(t.seen)
	sums := make([]int, n)
	mins := make([]int, n)
	maxs := make([]int, n)
	for s := range mins {
		mins[s] = -1
	}
	for _, r := range t.readings {
		for s := 0; s < n; s++ {
			c := 0
			if s < len(r.counts) {
				c = r.counts[s]
			}
			sums[s] += c
			if mins[s] < 0 || c < mins[s] {
				mins[s] = c
			}
			if c > maxs[s] {
				maxs[s] = c
			}
		}
	}

	last := t.readings[len(t.readings)-1]
	samples := float64(out.Samples)
	for s := 0; s < n; s++ {
		if !t.seen[s] {
			continue // a label nobody has ever worn is not a species
		}
		e := SpeciesCensus{
			Species:     Species(s),
			Mean:        float64(sums[s]) / samples,
			Min:         mins[s],
			Max:         maxs[s],
			ExtinctTick: t.emptyTick[s],
		}
		if e.Mean > 0 {
			e.Trough = float64(e.Min) / e.Mean
			e.Swing = float64(e.Max-e.Min) / e.Mean
		}
		e.Extinct = s >= len(last.counts) || last.counts[s] == 0
		out.Population += e.Mean
		out.Species = append(out.Species, e)
	}
	if out.Population > 0 {
		for i := range out.Species {
			out.Species[i].Share = out.Species[i].Mean / out.Population
		}
	}
	return out
}
