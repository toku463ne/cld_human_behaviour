package engine

import (
	"bytes"
	"math"
	"testing"
)

// The arm this stage is measured against: with no spread the enemies arrive
// where they always did, and the world takes the same values from the random
// source in the same order.
func TestNoEnemySpreadLeavesTheWorldExactlyAsItWas(t *testing.T) {
	run := func(spread float64) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 13
		cfg.EnemySpread = spread
		w := NewWorld(cfg)
		for i := 0; i < 2000; i++ {
			w.Step()
		}
		return w.Stats()
	}
	flat := run(0)
	if flat != run(0) {
		t.Fatal("the same world twice gave different runs")
	}
	if varied := run(0.6); varied == flat {
		t.Fatal("sending the enemies somewhere in particular changed nothing at all")
	}

	cfg := DefaultConfig()
	w := NewWorld(cfg)
	for _, r := range w.Regions() {
		if r.Enemies != 1 {
			t.Fatalf("a region takes %v of the arrivals with no spread, want 1", r.Enemies)
		}
	}
}

// Where they arrive changes; how many arrive does not. That is stage 15a's
// rule applied to the other thing the world puts in from outside.
func TestTheEnemySpreadMovesWhereTheyArriveAndNotHowMany(t *testing.T) {
	count := func(spread float64) (enemies int, humans int) {
		cfg := DefaultConfig()
		cfg.Seed = 17
		cfg.EnemySpread = spread
		w := NewWorld(cfg)
		for i := 0; i < 4000; i++ {
			w.Step()
		}
		for _, a := range w.Agents() {
			if a.Species == SpeciesEnemy {
				enemies++
			} else {
				humans++
			}
		}
		return
	}
	// Not the same world - the draws differ - so this is about the order of
	// things rather than about equality: sending them to one half of the map
	// must not double or halve how many there are.
	flatEnemies, _ := count(0)
	skewedEnemies, _ := count(0.8)
	if flatEnemies == 0 {
		t.Fatal("no enemies in the world at all")
	}
	if ratio := float64(skewedEnemies) / float64(flatEnemies); ratio < 0.5 || ratio > 2 {
		t.Fatalf("skewing the arrivals changed how many there are: %d against %d",
			skewedEnemies, flatEnemies)
	}
}

// They really do arrive where the map says. Where they are found later is
// another question entirely, and the answer turned out to be "nearly
// anywhere" - so what is pinned here is what the rule does, not what the
// world keeps of it (see HISTORY, 2026-09-12).
func TestEnemiesArriveWhereTheMapSendsThem(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 23
	cfg.EnemySpread = 0.8
	w := NewWorld(cfg)
	for i := 0; i < 6000; i++ {
		w.Step()
	}
	p := w.Prowl()
	if p.ArriveGain <= 0.05 {
		t.Fatalf("the arrivals land on ground worth %v more than average, want well above 0",
			p.ArriveGain)
	}
	// And what the weighting never touches: the world's own young start where
	// their parents were.
	if p.BornShare <= 0 || p.BornShare >= 1 {
		t.Fatalf("%v of the enemies were born here, want some of both", p.BornShare)
	}

	// A world where every region takes the same share says nothing.
	cfg.EnemySpread = 0
	flat := NewWorld(cfg)
	for i := 0; i < 6000; i++ {
		flat.Step()
	}
	if got := flat.Prowl(); math.Abs(got.ArriveGain) > 1e-9 || math.Abs(got.EnemyGain) > 1e-9 {
		t.Fatalf("even ground reports arrivals %v and standing %v", got.ArriveGain, got.EnemyGain)
	}
}

// The tallies the stage is measured with have to come back with a saved
// world: a run that came back with its past reset would report a different
// one.
func TestRegionTollsSurviveBeingSaved(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 29
	cfg.EnemySpread = 0.6
	w := NewWorld(cfg)
	for i := 0; i < 2000; i++ {
		w.Step()
	}
	before := append([]regionToll(nil), w.tolls...)

	var buf bytes.Buffer
	if err := w.Save(&buf); err != nil {
		t.Fatalf("saving: %v", err)
	}
	back, err := Load(&buf)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	for i := range before {
		if back.tolls[i] != before[i] {
			t.Fatalf("block %d came back with %v rather than %v", i, back.tolls[i], before[i])
		}
	}
}
