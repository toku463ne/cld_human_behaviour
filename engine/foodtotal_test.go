package engine

import (
	"math"
	"testing"
)

// A river map, the piece of country the tie to the water actually bites on.
func wetMap() []string {
	return []string{
		"....~~....",
		"....~~....",
		"....~~....",
	}
}

// What stage 61 found before it built anything: scaling every region's weight
// back to the total it had is not a rule about how much food there is. The
// weights are only ever used as proportions, so multiplying them all by one
// number leaves every proportion where it was - and the run identical.
//
// The exception is the clamp: a weight pushed past two comes back at two, and
// then the proportions really have moved. That is the whole of what the
// renormalisation does.
func TestRenormalisingWeightsIsAScalingAndNotARule(t *testing.T) {
	run := func(renorm bool) (Stats, []float64) {
		cfg := DefaultConfig()
		cfg.Seed = 3
		cfg.TerrainMap = wetMap()
		cfg.WatersideFood = 1
		cfg.FoodRenormalize = renorm
		w := NewWorld(cfg)
		for i := 0; i < 2000; i++ {
			w.Step()
		}
		var weights []float64
		for _, r := range w.Regions() {
			weights = append(weights, r.Food)
		}
		return w.Stats(), weights
	}
	on, wOn := run(true)
	off, wOff := run(false)
	if on != off {
		t.Fatalf("the same world came out differently: %+v against %+v", off, on)
	}

	// And the weights differ by one constant, which is what "a scaling" means.
	ratio := wOff[0] / wOn[0]
	for i := range wOn {
		if got := wOff[i] / wOn[i]; math.Abs(got-ratio) > 1e-9 {
			t.Fatalf("block %d scaled by %v, block 0 by %v", i, got, ratio)
		}
	}
	if math.Abs(ratio-1) < 1e-9 {
		t.Fatal("the two arms produced the same weights, so the test says nothing")
	}
}

// What the stage actually built: with the total following the map, a country
// the map calls rich grows more than the same country with the total held
// fixed.
func TestTheMapCanDecideHowMuchGrows(t *testing.T) {
	run := func(fromMap bool) (float64, float64) {
		cfg := DefaultConfig()
		cfg.Seed = 5
		cfg.TerrainMap = wetMap()
		cfg.WatersideFood = 1
		cfg.FoodRenormalize = !fromMap
		cfg.FoodTotalFromMap = fromMap
		w := NewWorld(cfg)
		for i := 0; i < 4000; i++ {
			w.Step()
		}
		mean := 0.0
		for _, r := range w.Regions() {
			mean += r.Food
		}
		return w.plantRate(), mean / float64(len(w.Regions()))
	}
	fixed, _ := run(false)
	followed, meanWeight := run(true)

	if meanWeight <= 1.05 {
		t.Fatalf("the wet map's weights average %v, so this test has no dose in it", meanWeight)
	}
	if followed <= fixed {
		t.Fatalf("a map the weights call rich grows %v a tick against %v", followed, fixed)
	}
	if got := followed / fixed; math.Abs(got-meanWeight) > 1e-9 {
		t.Fatalf("the world grows %v times as much on weights averaging %v", got, meanWeight)
	}
}

// And a world that has not asked for it is untouched, down to the values
// taken from the random source.
func TestTheTotalIsFixedUnlessTheMapIsAskedToDecideIt(t *testing.T) {
	run := func(fromMap bool) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 11
		cfg.TerrainMap = wetMap()
		cfg.WatersideFood = 1
		cfg.FoodTotalFromMap = fromMap
		w := NewWorld(cfg)
		for i := 0; i < 1500; i++ {
			w.Step()
		}
		return w.Stats()
	}
	fixed := run(false)
	if fixed != run(false) {
		t.Fatal("the same world twice gave different runs")
	}
	if followed := run(true); followed == fixed {
		t.Fatal("letting the map decide the total changed nothing at all")
	}
}
