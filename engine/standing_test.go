package engine

import (
	"bytes"
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
		if a.groundFactor(GeneAttack) != 1 {
			t.Fatalf("an agent stands on ground worth %v with no spread, want 1", a.groundFactor(GeneAttack))
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
		before := a.groundFactor(GeneAttack)
		a.X, a.Y = highX, highY
		w.Step()
		after := mustAgent(t, w, id).groundFactor(GeneAttack)

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

// 57c: the ground favours some genes over others, and the nine average to one
// so that no region is simply better than another. That is what stops it
// cancelling - two neighbours built differently get different factors, where
// the scalar gave them the same one.
func TestGroundFavoursBuildsWithoutFavouringRegions(t *testing.T) {
	cfg := quietConfig()
	cfg.RegionFavourSpread = 0.5
	w := NewWorld(cfg)

	for _, r := range w.regions {
		if len(r.Favour) != NumGenes {
			t.Fatalf("a region favours %d genes, want %d", len(r.Favour), NumGenes)
		}
		sum := 0.0
		for _, f := range r.Favour {
			sum += f
		}
		if mean := sum / float64(NumGenes); math.Abs(mean-1) > 1e-9 {
			t.Fatalf("a region's favour averages %v, want 1", mean)
		}
	}

	// Two bodies in the same place, built oppositely, do not get the same
	// factor - which is the whole of the difference from 57a.
	regions := w.Regions()
	mid := regions[0]
	x, y := (mid.MinX+mid.MaxX)/2, (mid.MinY+mid.MaxY)/2

	fighter := newGenome()
	thinker := newGenome()
	for g := range fighter {
		fighter[g], thinker[g] = 1, 1
	}
	fighter[GeneAttack], thinker[GeneIntelligence] = 100, 100

	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: x, Y: y, Genome: fighter}))
	b := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: x, Y: y, Genome: thinker}))
	w.Step()
	a, b = mustAgent(t, w, a.ID), mustAgent(t, w, b.ID)

	if math.Abs(w.FootingOf(a.ID)-w.FootingOf(b.ID)) < 1e-6 {
		t.Fatalf("two different builds on the same ground both read %v", w.FootingOf(a.ID))
	}
}

// And the arm it is measured against takes nothing from the random source.
func TestNoFavourSpreadLeavesTheWorldExactlyAsItWas(t *testing.T) {
	run := func(spread float64) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 9
		cfg.RegionFavourSpread = spread
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
	if varied := run(0.5); varied == flat {
		t.Fatal("giving the regions a taste in builds changed nothing at all")
	}

	cfg := DefaultConfig()
	w := NewWorld(cfg)
	if got := w.Suits(); math.Abs(got.Gain) > 1e-9 {
		t.Fatalf("ground with no taste reports a gain of %v", got.Gain)
	}
}

// A carried footing is the one thing this stage adds that the world cannot
// work out again from where a body is standing, so it has to survive being
// saved (stage 21's rule: a saved world comes back the same world).
func TestACarriedFootingSurvivesBeingSaved(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 11
	cfg.RegionFavourSpread = 0.5
	cfg.RegionAbilityCarried = true
	w := NewWorld(cfg)
	for i := 0; i < 300; i++ {
		w.Step()
	}

	var buf bytes.Buffer
	if err := w.Save(&buf); err != nil {
		t.Fatalf("saving: %v", err)
	}
	back, err := Load(&buf)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}

	for _, a := range w.Agents() {
		got := back.agentByID(a.ID)
		if got == nil {
			t.Fatalf("agent %d did not come back", a.ID)
		}
		for g := 0; g < NumGenes; g++ {
			if got.footing[g] != a.footing[g] {
				t.Fatalf("agent %d came back standing on %v rather than %v",
					a.ID, got.footing[g], a.footing[g])
			}
		}
	}
}
