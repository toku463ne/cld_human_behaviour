package engine

import (
	"math"
	"testing"
)

// aheadWorld is a still world with a river down the middle, read ahead.
func aheadWorld(t *testing.T, noise float64, seen bool) *World {
	t.Helper()
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	cfg.TerrainMap = []string{
		"...~~...",
		"...~~...",
		"...~~...",
	}
	cfg.GroundAheadSeen, cfg.GroundAheadNoise = seen, noise
	return NewWorld(cfg)
}

// Since 2026-09-19 the default is to read, and a world with no map reads
// nothing at all - which is what keeps every flat world running as it did.
func TestAFlatWorldReadsNothing(t *testing.T) {
	if !DefaultConfig().GroundAheadSeen {
		t.Fatal("bodies no longer read the ground ahead by default")
	}
	cfg := quietConfig() // no TerrainMap
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Genome: genomeOf(50, 50, 50)}))
	self := w.perceive(a).Self
	if self.AroundSeen {
		t.Fatal("a world with no ground filled the readings in")
	}
	if got := moveCostDir(&w.cfg, &self, 0.5, 1, 0); got != moveCost(&w.cfg, &self, 0.5) {
		t.Fatalf("a flat world prices a direction at %v, want the flat figure", got)
	}
}

// And the arm that puts the old world back prices every option with the
// ground underfoot, whatever direction it points.
func TestTheGroundAheadCanBePutBack(t *testing.T) {
	w := aheadWorld(t, 0, false)
	a := mustAgent(t, w, swimmer(t, w, 250, 300, 0)) // on the bank, river to the east
	self := w.perceive(a).Self
	if self.AroundSeen {
		t.Fatal("the rule is off and the readings were filled in")
	}
	east := moveCostDir(&w.cfg, &self, 0.5, 1, 0)
	west := moveCostDir(&w.cfg, &self, 0.5, -1, 0)
	if east != west {
		t.Fatalf("with the rule off, stepping toward the river costs %v and away %v", east, west)
	}
	if east != moveCost(&w.cfg, &self, 0.5) {
		t.Fatal("with the rule off the figure is not the one every world before used")
	}
}

// With it on and no error, walking into the river is priced as the river.
func TestWalkingIntoTheRiverIsPricedAsTheRiver(t *testing.T) {
	w := aheadWorld(t, 0, true)
	a := mustAgent(t, w, swimmer(t, w, 250, 300, 0)) // bank; water is one cell east
	self := w.perceive(a).Self
	if !self.AroundSeen {
		t.Fatal("the rule is on and nothing was read")
	}
	east := moveCostDir(&w.cfg, &self, 0.5, 1, 0)
	west := moveCostDir(&w.cfg, &self, 0.5, -1, 0)
	if !(east > west) {
		t.Fatalf("toward the river %v, away from it %v: want the river dearer", east, west)
	}
	if got, want := east/west, w.cfg.WaterMoveCost; math.Abs(got-want) > 1e-9 {
		t.Fatalf("the river reads %v times dearer, want %v", got, want)
	}
	// And a target on the far side is priced through the water it points at.
	if got := moveCostTo(&w.cfg, &self, 0.5, 700, 300); math.Abs(got-east) > 1e-9 {
		t.Fatalf("a target across the river costs %v, want the river's %v", got, east)
	}
}

// A body standing IN the river reads the bank as cheaper, which is the thing
// no rule before this stage could say.
func TestFromTheRiverTheBankReadsCheaper(t *testing.T) {
	w := aheadWorld(t, 0, true)
	// The river is two cells wide (3 and 4 of eight), so a body in the near
	// half has dry ground one cell west and more water one cell east.
	a := mustAgent(t, w, swimmer(t, w, 350, 300, 0))
	self := w.perceive(a).Self
	out := moveCostDir(&w.cfg, &self, 0.5, -1, 0)   // west, to the bank
	deeper := moveCostDir(&w.cfg, &self, 0.5, 1, 0) // east, into the far half
	if !(out < deeper) {
		t.Fatalf("leaving costs %v and staying in costs %v: want leaving cheaper", out, deeper)
	}
}

// What the ground ahead takes per tick is charged as the difference, so that
// the drain a body is already paying is not counted twice (stage 99).
func TestTheDrainAheadIsChargedAsADifference(t *testing.T) {
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	cfg.TerrainMap = []string{"...~~...", "...~~...", "...~~..."}
	cfg.GroundAheadSeen, cfg.GroundAheadNoise, cfg.WaterDrain = true, 0, 0.05
	w := NewWorld(cfg)

	dry := mustAgent(t, w, swimmer(t, w, 250, 300, 0))
	self := w.perceive(dry).Self
	toWater := moveCostDir(&w.cfg, &self, 0.5, 1, 0)
	toLand := moveCostDir(&w.cfg, &self, 0.5, -1, 0)
	// crossing multiplier aside, the difference carries the whole drain
	if got := toWater - toLand - (moveCostAt(&w.cfg, 0.5)*(w.cfg.WaterMoveCost-1))*selfBurden(&self); math.Abs(got-0.05) > 1e-9 {
		t.Fatalf("the drain ahead adds %v a tick, want %v", got, 0.05)
	}

	wet := mustAgent(t, w, swimmer(t, w, 350, 300, 0))
	wetSelf := w.perceive(wet).Self
	// deeper in: same drain as here, so nothing extra is charged for it
	deeper := moveCostDir(&w.cfg, &wetSelf, 0.5, 1, 0)
	want := moveCostAt(&w.cfg, 0.5) * w.cfg.WaterMoveCost * selfBurden(&wetSelf)
	if math.Abs(deeper-want) > 1e-9 {
		t.Fatalf("moving within the water costs %v, want %v (no drain charged twice)", deeper, want)
	}
}

func selfBurden(s *SelfView) float64 {
	if s.Burden <= 0 {
		return 1
	}
	return s.Burden
}

// The reading carries the reader's own error, and a body that reads badly
// enough can take a river for a field.
func TestReadingTheGroundAheadCarriesTheReadersError(t *testing.T) {
	cfg := quietConfig()
	cfg.Seed = 3
	cfg.Width, cfg.Height = 800, 600
	cfg.TerrainMap = []string{"...~~...", "...~~...", "...~~..."}
	cfg.GroundAheadSeen, cfg.GroundAheadNoise = true, 1
	w := NewWorld(cfg)

	sharpID := swimmer(t, w, 250, 300, 0)
	dullID := swimmer(t, w, 250, 300, 0)
	sharp, dull := mustAgent(t, w, sharpID), mustAgent(t, w, dullID)
	sharp.Genome[GeneRationality], dull.Genome[GeneRationality] = 100, 1

	var sharpErr, dullErr float64
	for i := 0; i < 200; i++ {
		s1 := w.perceive(sharp).Self
		sharpErr += math.Abs(s1.Around[0].Cost - w.cfg.WaterMoveCost)
		s2 := w.perceive(dull).Self
		dullErr += math.Abs(s2.Around[0].Cost - w.cfg.WaterMoveCost)
	}
	if !(sharpErr < dullErr/2) {
		t.Fatalf("the sharp reader is out by %v and the dull one by %v", sharpErr/200, dullErr/200)
	}
	if sharpErr != 0 {
		t.Fatalf("a fully rational reader is out by %v, want nothing", sharpErr/200)
	}
}

// The blind arm looks, draws the same numbers, and reads the world instead of
// the cell: the control that says whether the information did anything.
func TestTheBlindArmReadsTheWorldNotTheCell(t *testing.T) {
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	cfg.TerrainMap = []string{"...~~...", "...~~...", "...~~..."}
	cfg.GroundAheadSeen, cfg.GroundAheadNoise, cfg.GroundAheadBlind = true, 0, true
	w := NewWorld(cfg)
	a := mustAgent(t, w, swimmer(t, w, 250, 300, 0))
	self := w.perceive(a).Self

	east := moveCostDir(&w.cfg, &self, 0.5, 1, 0)  // river
	west := moveCostDir(&w.cfg, &self, 0.5, -1, 0) // open ground
	if math.Abs(east-west) > 1e-12 {
		t.Fatalf("the blind arm still tells the river (%v) from the field (%v)", east, west)
	}
	mean, _ := w.groundMean()
	if mean <= 1 || mean >= w.cfg.WaterMoveCost {
		t.Fatalf("the world's mean ground is %v, want between a field and the river", mean)
	}
}
