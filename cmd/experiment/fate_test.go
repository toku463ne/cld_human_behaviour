package main

import "testing"

// series builds a population series at a fixed cadence, the way measure does.
func series(interval int, pops ...int) []sample {
	out := make([]sample, len(pops))
	for i, p := range pops {
		out[i] = sample{tick: i * interval, pop: p}
	}
	return out
}

func TestAWorldThatKeepsGoingHasNotFallen(t *testing.T) {
	f := fateOf(series(100, 70, 120, 200, 180, 150, 160, 155, 150, 148, 152), 1000, 10)
	if f.collapsed {
		t.Fatal("a world holding 150 bodies read as collapsed")
	}
	if f.peak != 200 {
		t.Fatalf("peak %v, want 200", f.peak)
	}
	// Never fell, so fellAt is the length of the run and not an estimate of
	// anything - the same censoring the half-life uses.
	if f.fellAt != 1000 {
		t.Fatalf("fellAt %d, want the run length 1000", f.fellAt)
	}
}

func TestAWorldThatFellAndStayedDownHasCollapsed(t *testing.T) {
	f := fateOf(series(100, 70, 150, 238, 110, 231, 30, 7, 4, 5, 4), 1000, 10)
	if !f.collapsed {
		t.Fatal("a world down to four bodies did not read as collapsed")
	}
	if f.peak != 238 {
		t.Fatalf("peak %v, want 238", f.peak)
	}
	// The first sample under the line, which is the one at tick 600 - not the
	// first dip at 30, which is above it.
	if f.fellAt != 600 {
		t.Fatalf("fellAt %d, want 600", f.fellAt)
	}
}

func TestAWorldThatFellAndCameBackIsNotCollapsed(t *testing.T) {
	f := fateOf(series(100, 70, 40, 8, 25, 80, 140, 160, 150, 155, 148), 1000, 10)
	if f.collapsed {
		t.Fatal("a world that recovered read as collapsed")
	}
	// It did fall, and that is worth being able to see: fallen but not
	// collapsed is exactly the oscillation these worlds go through.
	if f.fellAt != 200 {
		t.Fatalf("fellAt %d, want 200", f.fellAt)
	}
}

func TestCollapseIsJudgedOnTheTailAndNotTheLastTick(t *testing.T) {
	// Ten samples, so the tail is the last two. A world sitting at four that
	// happens to be caught mid-rebound on its very last sample is still a
	// collapsed world, and a run read at one tick would say otherwise.
	f := fateOf(series(100, 70, 150, 200, 60, 20, 5, 4, 3, 4, 12), 1000, 10)
	if !f.collapsed {
		t.Fatal("a rebound on the last sample hid a collapsed world")
	}
}

func TestAnEmptySeriesSaysNothing(t *testing.T) {
	f := fateOf(nil, 1000, 10)
	if f.collapsed || f.peak != 0 || f.fellAt != 1000 {
		t.Fatalf("empty series read as %+v", f)
	}
}
