package engine

import (
	"math"
	"testing"
)

// The flat gradient, and what the rate reading does to it (stage 93).
//
// A whole, fed body cannot die inside a planning window whatever it does, so
// every difference it is asked about comes out at nought: this is the wall
// stages 50, 51, 67, 81, 87 and 116 all ran into from different sides. Reading
// the countdown as a rate rather than as a deadline gives that body a gradient
// without moving it anywhere near death.
func TestTheRateReadingGivesAWholeBodyAGradient(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 5
	w := NewWorld(cfg)
	hot := cfg
	hot.LookaheadReadsRate = 1

	flat, opened, whole := 0, 0, 0
	for tick := 0; tick < 4000; tick++ {
		w.Step()
		if tick < 2000 || tick%40 != 0 {
			continue
		}
		for i := range w.agents {
			a := &w.agents[i]
			if !a.Alive || a.Species != SpeciesHuman {
				continue
			}
			s := &w.perceive(a).Self
			if s.Hunger > cfg.SatiatedHunger || s.Vitality < 0.95*s.MaxVitality {
				continue // the wall is about bodies in no trouble at all
			}
			whole++
			if keepValue(&cfg, s, 0, 1, 0, 0, 0) > 0 {
				continue
			}
			flat++
			if keepValue(&hot, s, 0, 1, 0, 0, 0) > 0 {
				opened++
			}
		}
	}
	if whole == 0 {
		t.Fatal("no satiated, whole body turned up to ask")
	}
	if flat == 0 {
		t.Fatalf("not one of %d satiated whole bodies read a flat gradient, "+
			"so this test is no longer about the wall it was written for", whole)
	}
	if opened != flat {
		t.Fatalf("%d of %d flat readings stayed flat under the rate reading", flat-opened, flat)
	}
}

// The reading itself: nought leaves oneHorizon exactly as it was, the weight
// mixes the two, and what it mixes in falls as the tank lasts longer without
// ever reaching nought.
func TestTheRateReadingIsABlendOfTwoReadings(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ShockRisk = 0 // the other half of pressure, left out so this is one thing
	s := &SelfView{MaxVitality: 100, ShockRisk: 0}

	at := func(w, ticksLeft float64) float64 {
		c := cfg
		c.LookaheadReadsRate = w
		// A drain that empties the tank in this many ticks.
		return oneHorizon(&c, s, 100, 100/ticksLeft, false)
	}

	// Outside the window the deadline reading says nothing at all, whatever
	// the drain, and that is the whole of what this stage is about.
	for _, t2 := range []float64{cfg.PlanHorizon, 2 * cfg.PlanHorizon, 20 * cfg.PlanHorizon} {
		if got := at(0, t2); got != 0 {
			t.Fatalf("the deadline reading gives %v to a body lasting %v ticks", got, t2)
		}
		if got := at(1, t2); got <= 0 {
			t.Fatalf("the rate reading gives %v to a body lasting %v ticks", got, t2)
		}
	}

	// It falls as the tank lasts longer, and never reaches nought.
	far, further := at(1, 4*cfg.PlanHorizon), at(1, 40*cfg.PlanHorizon)
	if !(far > further && further > 0) {
		t.Fatalf("lasting ten times as long reads %v against %v", further, far)
	}

	// And the blend is a blend, inside the window as well as outside it.
	inside := cfg.PlanHorizon / 2
	dead, rate, mixed := at(0, inside), at(1, inside), at(0.25, inside)
	if math.Abs(mixed-(0.75*dead+0.25*rate)) > 1e-12 {
		t.Fatalf("a quarter of the way between %v and %v reads %v", dead, rate, mixed)
	}
}
