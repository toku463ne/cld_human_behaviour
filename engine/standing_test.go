package engine

import (
	"math"
	"testing"
)

// findRegions returns the middles of two blocks whose ground differs, so that
// a test can put a body on one and then on the other.
func twoDifferentGrounds(t *testing.T, w *World) (lowX, lowY, highX, highY float64) {
	t.Helper()
	regions := w.Regions()
	low, high := 0, 0
	for i := range regions {
		if regions[i].Ability < regions[low].Ability {
			low = i
		}
		if regions[i].Ability > regions[high].Ability {
			high = i
		}
	}
	if regions[high].Ability-regions[low].Ability < 0.1 {
		t.Fatal("no two regions differ enough to tell apart")
	}
	l, h := regions[low], regions[high]
	return (l.MinX + l.MaxX) / 2, (l.MinY + l.MaxY) / 2,
		(h.MinX + h.MaxX) / 2, (h.MinY + h.MaxY) / 2
}

// With no spread the ground has no opinion, and an agent reads exactly as it
// did before the rule existed - including taking nothing from the random
// source, which is what makes this an arm rather than something similar.
func TestNoGroundSpreadLeavesTheWorldExactlyAsItWas(t *testing.T) {
	run := func(spread float64) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 7
		cfg.RegionAbilitySpread = spread
		w := NewWorld(cfg)
		for i := 0; i < 1500; i++ {
			w.Step()
		}
		return w.Stats()
	}

	flat := run(0)
	if flat != run(0) {
		t.Fatal("the same world twice gave different runs")
	}
	if varied := run(0.4); varied == flat {
		t.Fatal("giving the regions different footing changed nothing at all")
	}

	cfg := DefaultConfig()
	cfg.RegionAbilitySpread = 0
	w := NewWorld(cfg)
	for _, r := range w.Regions() {
		if r.Ability != 1 {
			t.Fatalf("a region multiplies ability by %v with no spread, want 1", r.Ability)
		}
	}
	for _, a := range w.Agents() {
		if a.groundFactor() != 1 {
			t.Fatalf("an agent stands on ground worth %v with no spread, want 1", a.groundFactor())
		}
	}
}

// What the ground moves is what a body can do today. What it must not move is
// what the body holds: a maximum that shifted underfoot would leave an agent
// whose vitality is over its own ceiling after one step across a border.
func TestTheGroundMovesWhatABodyDoesAndNotWhatItHolds(t *testing.T) {
	cfg := quietConfig()
	cfg.RegionAbilitySpread = 0.4
	w := NewWorld(cfg)
	lowX, lowY, highX, highY := twoDifferentGrounds(t, w)

	id := w.addAgent(Agent{Maturity: 1, X: lowX, Y: lowY, Genome: filledGenome(50)})
	a := mustAgent(t, w, id)
	w.Step()
	a = mustAgent(t, w, id)
	lowAttack := a.Attack(&w.cfg)
	maxVitality, maxSpeed := a.MaxVitality(&w.cfg), a.MaxSpeed(&w.cfg)
	memory := a.MemoryCapacity(&w.cfg)

	a.X, a.Y = highX, highY
	w.Step()
	a = mustAgent(t, w, id)

	if a.Attack(&w.cfg) <= lowAttack {
		t.Fatalf("moving to better ground left attack at %v (was %v)",
			a.Attack(&w.cfg), lowAttack)
	}
	if got := a.MaxVitality(&w.cfg); math.Abs(got-maxVitality) > 1e-9 {
		t.Fatalf("the ground moved how much vitality the body holds: %v, was %v", got, maxVitality)
	}
	if got := a.MaxSpeed(&w.cfg); math.Abs(got-maxSpeed) > 1e-9 {
		t.Fatalf("the ground moved how fast the body can go: %v, was %v", got, maxSpeed)
	}
	if got := a.MemoryCapacity(&w.cfg); got != memory {
		t.Fatalf("the ground moved how many faces the body holds: %d, was %d", got, memory)
	}
	// And an agent never holds more vitality than it can hold, which is the
	// failure this split was made to avoid.
	if a.Vitality > a.MaxVitality(&w.cfg)+1e-9 {
		t.Fatalf("vitality %v is over the ceiling %v", a.Vitality, a.MaxVitality(&w.cfg))
	}
}

// The control the stage turns on: the same multiplier drawn from the ground a
// body was born on, and then carried. That is what a skill is, and a carried
// one cannot make anywhere worth staying in because leaving costs nothing.
func TestACarriedGroundIsTheOneThatDoesNotChangeWhenTheBodyMoves(t *testing.T) {
	build := func(carried bool) (*World, int, float64, float64) {
		cfg := quietConfig()
		cfg.RegionAbilitySpread = 0.4
		cfg.RegionAbilityCarried = carried
		w := NewWorld(cfg)
		lowX, lowY, highX, highY := twoDifferentGrounds(t, w)
		id := w.addAgent(Agent{Maturity: 1, X: lowX, Y: lowY, Genome: filledGenome(50)})
		w.Step()
		return w, id, highX, highY
	}

	for _, carried := range []bool{false, true} {
		w, id, highX, highY := build(carried)
		a := mustAgent(t, w, id)
		before := a.groundFactor()
		a.X, a.Y = highX, highY
		w.Step()
		after := mustAgent(t, w, id).groundFactor()

		moved := math.Abs(after-before) > 1e-9
		if carried && moved {
			t.Fatalf("a carried factor changed from %v to %v when the body moved", before, after)
		}
		if !carried && !moved {
			t.Fatalf("standing somewhere else left the factor at %v", after)
		}
	}
}

// Standing is the measurement the stage turns on, and it says nothing at all
// in a world where every region is the same.
func TestStandingReadsNothingWhereThereIsNothingToRead(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 3
	w := NewWorld(cfg)
	for i := 0; i < 200; i++ {
		w.Step()
	}
	if got := w.Standing(); math.Abs(got.Gain) > 1e-9 {
		t.Fatalf("flat ground reports a gain of %v", got.Gain)
	}

	cfg.RegionAbilitySpread = 0.4
	w = NewWorld(cfg)
	stand := w.Standing()
	if stand.All <= 0 {
		t.Fatalf("a world with varied ground reports an average of %v", stand.All)
	}
}
