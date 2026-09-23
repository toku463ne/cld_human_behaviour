package engine

import (
	"math"
	"testing"
)

// The one test this instrument has to pass. Everything else it reports is
// only worth reading if the world it read was the world that would have run
// anyway: a measurement that moves what it measures is not a measurement.
//
// It is checked against the same figures the fingerprint uses, over the same
// three seeds, with the counting on in one world and off in the other.
func TestCountingDoesNotChangeTheWorld(t *testing.T) {
	for _, seed := range []int64{1, 2, 3} {
		quiet := DefaultConfig()
		quiet.Seed = seed
		loud := quiet
		loud.Correlate = true

		a, b := NewWorld(quiet), NewWorld(loud)
		for i := 0; i < 2000; i++ {
			a.Step()
			b.Step()
		}
		x, y := a.Stats(), b.Stats()
		if x.Population != y.Population || x.Births != y.Births || x.Deaths != y.Deaths ||
			x.Kills != y.Kills || x.Fights != y.Fights {
			t.Fatalf("seed %d: counting moved the world: %+v vs %+v", seed, x, y)
		}
		if math.Abs(x.AvgPower-y.AvgPower) > 1e-9 ||
			math.Abs(x.AvgVitality-y.AvgVitality) > 1e-9 ||
			math.Abs(x.AvgHunger-y.AvgHunger) > 1e-9 {
			t.Fatalf("seed %d: counting moved the averages", seed)
		}
		// And the generator is where it would have been, which is the
		// stronger statement: the two worlds would go on agreeing for ever.
		if a.draws.draws != b.draws.draws {
			t.Fatalf("seed %d: counting drew %d numbers", seed, b.draws.draws-a.draws.draws)
		}
	}
}

// A world that did not ask for the counting has none of it, and pays for none
// of it: no tables, and nothing hanging off any body.
func TestCountingIsOffByDefault(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 7
	w := NewWorld(cfg)
	for i := 0; i < 200; i++ {
		w.Step()
	}
	if got := w.Correlate(); got.Decisions != 0 || got.Live != 0 || len(got.Top) != 0 {
		t.Fatalf("default world counted something: %+v", got)
	}
	if w.corr.cells != nil {
		t.Fatal("default world allocated the tables")
	}
	for i := range w.agents {
		if w.agents[i].corr != nil {
			t.Fatal("default world hung a counter off a body")
		}
	}
}

// Hunger only ever climbs on its own, so a fall in it is a meal - and that is
// the whole cut, with no figure saying how big a meal has to be.
func TestEatingIsCountedAsAnEvent(t *testing.T) {
	w, a := correlateWorld(t)
	a.Hunger = 40
	w.Step()
	a.Hunger = 10
	w.Step()

	got := w.Correlate()
	if got.Events[CorrFed] == 0 {
		t.Fatalf("a fall in hunger was not read as a meal: %+v", got.Events)
	}
	if got.Value[CorrFed] <= 0 {
		t.Fatalf("a meal was worth %.3f, want better than nothing", got.Value[CorrFed])
	}
}

// A body that stops is the one event with no next state to price, and what it
// costs is what the formula pays for the whole of the odds.
func TestStoppingIsCountedAndPricedAsALife(t *testing.T) {
	w, a := correlateWorld(t)
	w.Step()
	// Starved rather than set to nought: a body on nought with an empty
	// stomach mends itself back up again, which is the recovery path this
	// world is built around.
	a.Hunger = w.cfg.MaxHunger
	a.Vitality = 0.01
	w.Step()

	got := w.Correlate()
	if got.Events[CorrDied] != 1 {
		t.Fatalf("deaths counted = %d, want 1", got.Events[CorrDied])
	}
	if want := -w.cfg.LifeValue; math.Abs(got.Value[CorrDied]-want) > 1e-9 {
		t.Fatalf("a life was worth %.3f, want %.3f", got.Value[CorrDied], want)
	}
}

// The point of the trace: an event is credited to the decisions that came
// before it, not only to whatever the body happened to be doing at the time.
func TestAnEventIsCreditedToTheDecisionsBeforeIt(t *testing.T) {
	w, a := correlateWorld(t)
	w.Step()
	ac := w.corrOf(a)
	if len(ac.trace) == 0 {
		t.Fatal("no decision was filed")
	}
	before := len(ac.trace)
	a.Hunger = 0
	w.Step()

	got := w.Correlate()
	if got.Live == 0 {
		t.Fatal("the meal was credited to no decision at all")
	}
	if before == 0 {
		t.Fatal("nothing was eligible")
	}
}

// A decision older than the window is no longer eligible, which is what stops
// every event in a life being about every decision in it.
func TestTheWindowLetsOldDecisionsGo(t *testing.T) {
	w, a := correlateWorld(t)
	w.cfg.CorrelateWindow = 2
	w.Step()
	for i := 0; i < 6; i++ {
		w.tick++
	}
	a.Hunger = 0
	w.fireCorr(a, CorrFed, 1)
	if got := w.Correlate(); got.Live != 0 {
		t.Fatalf("a decision %d ticks old was still eligible", 6)
	}
}

// Two events over the same body inside the window are what a composed link
// would be built from, and this is the table that says whether any pair ever
// fires at all.
func TestOneEventAfterAnotherIsCounted(t *testing.T) {
	w, a := correlateWorld(t)
	w.Step()
	w.fireCorr(a, CorrCoin, 0)
	w.tick++
	w.fireCorr(a, CorrFed, 5)

	got := w.Correlate()
	if got.Pairs[CorrCoin][CorrFed] != 1 {
		t.Fatalf("coin followed by a meal counted %d times, want 1", got.Pairs[CorrCoin][CorrFed])
	}
	if got.CoinToFed != 1 {
		t.Fatalf("CoinToFed = %d, want 1", got.CoinToFed)
	}
}

// A body that meets the same (situation, move, event) twice is what any
// promotion rule would be waiting for, and it costs one bit to know.
func TestTheSameKeyTwiceIsCounted(t *testing.T) {
	w, a := correlateWorld(t)
	w.Step()
	ac := w.corrOf(a)
	ac.trace = []corrTrace{{key: 3, at: w.tick}}
	w.fireCorr(a, CorrFed, 1)
	if w.corr.repeats != 0 {
		t.Fatalf("the first time counted as a repeat")
	}
	ac.trace = []corrTrace{{key: 3, at: w.tick}}
	w.fireCorr(a, CorrFed, 1)
	if w.corr.repeats != 1 {
		t.Fatalf("repeats = %d, want 1", w.corr.repeats)
	}
}

// A gift answered by a gift the other way is the cheapest thing in this world
// that looks like an exchange, and it needs no new word to count.
func TestAGiftGivenBackIsCounted(t *testing.T) {
	w, a := correlateWorld(t)
	b := &w.agents[1]
	w.noteGift(a, b)
	w.tick += 40
	w.noteGift(b, a)

	got := w.Correlate()
	if got.Gifts != 2 {
		t.Fatalf("gifts = %d, want 2", got.Gifts)
	}
	if got.Back != 1 {
		t.Fatalf("returned gifts = %d, want 1", got.Back)
	}
	if got.BackTicks != 40 {
		t.Fatalf("a gift took %.0f ticks to be answered, want 40", got.BackTicks)
	}
}

// Two bodies each holding an ornament the other would rather have: the double
// coincidence of wants, which is the whole of whether the barter this world
// skipped could ever fire.
func TestASwapBothSidesGainFromIsFound(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 11
	cfg.Correlate = true
	cfg.Trinkets = true
	cfg.TrinketTaste = 1
	w := NewWorld(cfg)
	a, b := &w.agents[0], &w.agents[1]
	b.X, b.Y = a.X+1, a.Y

	// Two pieces at opposite ends of the circle of styles, one in each hand,
	// each in the hand that likes it least.
	ta, tb := w.tasteOf(a), w.tasteOf(b)
	a.carried = []Food{{Kind: FoodTrinket, Made: 1, Style: math.Mod(tb+0.5, 1)}}
	b.carried = []Food{{Kind: FoodTrinket, Made: 1, Style: math.Mod(ta+0.5, 1)}}
	if w.corrSwapGain(a, b) <= 0 {
		// Styles are drawn from the body's own taste, so the pair above is
		// the worst each could hold; if this ever stops being a gain, the
		// swap has nothing to be about.
		t.Skip("this world's tastes leave nothing to swap")
	}

	w.tick = cfg.CorrelateSample
	w.sampleWants()
	if got := w.Correlate(); got.Swap <= 0 || got.Gain <= 0 {
		t.Fatalf("a swap both sides gain from was not found: swap=%.3f gain=%.3f", got.Swap, got.Gain)
	}
}

// correlateWorld is a small world with the counting on and nothing else
// changed, and the first body in it.
func correlateWorld(t *testing.T) (*World, *Agent) {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Seed = 5
	cfg.Correlate = true
	cfg.InitialPopulation = 4
	cfg.InitialEnemies = 0
	w := NewWorld(cfg)
	if len(w.agents) < 2 {
		t.Fatal("the world came up empty")
	}
	return w, &w.agents[0]
}
