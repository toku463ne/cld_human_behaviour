package engine

import "testing"

// Stage 94: what a child is worth is the fourth preference, and the first one
// that is about a goal rather than about a danger.
//
// The three that were there - what a mauling puts you off, what a rival is
// worth removing, how frightening running on empty feels - all price what an
// option might cost. None of them can make one body keener on offspring than
// another, so there was no axis for selection to work on there at all.
func TestWhatAChildIsWorthVariesBetweenBodies(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 3
	cfg.MateWeightSpread = 0.3
	w := NewWorld(cfg)

	seen := map[float64]bool{}
	for i := range w.agents {
		seen[w.agents[i].lore.mateWeight] = true
	}
	if len(seen) < 2 {
		t.Fatalf("every founder wants a child exactly as much as every other (%d values)", len(seen))
	}

	// And what varies is what the utility reads: two bodies looking at the
	// same candidate price the child differently.
	keen, cool := &w.agents[0], &w.agents[0]
	for i := range w.agents {
		if w.agents[i].lore.mateWeight > keen.lore.mateWeight {
			keen = &w.agents[i]
		}
		if w.agents[i].lore.mateWeight < cool.lore.mateWeight {
			cool = &w.agents[i]
		}
	}
	// One at a time: the world hands out one perception buffer and fills it
	// again for the next body, so holding two of them is holding one twice.
	hi := w.perceive(keen).Self.MateWeight
	lo := w.perceive(cool).Self.MateWeight
	if hi <= lo {
		t.Fatalf("the keener body reads %v for a child and the cooler one %v", hi, lo)
	}
}

// With the spread at nought the world is the one it always was: no value is
// drawn for this, nothing mutates, and every body carries the world's own
// figure. (That the run is bit for bit identical is what the golden test
// says; this says why.)
func TestWithNoSpreadNobodyWantsAChildDifferently(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 3
	w := NewWorld(cfg)
	for i := range w.agents {
		if got := w.agents[i].lore.mateWeight; got != cfg.OffspringValue {
			t.Fatalf("a founder in a world with no spread wants a child at %v, want %v",
				got, cfg.OffspringValue)
		}
	}
	// And a child of two of them is born with the same figure rather than a
	// mutated one.
	child := w.inheritLore(&w.agents[0], &w.agents[1])
	if child.mateWeight != cfg.OffspringValue {
		t.Fatalf("a child in a world with no spread wants one at %v", child.mateWeight)
	}
}

// It is inherited, mutated and traded like the preferences it sits with - the
// whole point of putting it there was that none of that had to be written
// again.
func TestWhatAChildIsWorthIsInheritedAndRubsOff(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 9
	cfg.MateWeightSpread = 0.3
	w := NewWorld(cfg)

	pa, pb := &w.agents[0], &w.agents[1]
	pa.lore.mateWeight, pb.lore.mateWeight = 20, 80
	seen := map[float64]int{}
	for i := 0; i < 200; i++ {
		seen[w.inheritLore(pa, pb).mateWeight]++
	}
	near20, near80 := 0, 0
	for v, n := range seen {
		if v < 50 {
			near20 += n
		} else {
			near80 += n
		}
	}
	if near20 == 0 || near80 == 0 {
		t.Fatalf("200 children of a 20 and an 80 came out %d low and %d high", near20, near80)
	}
	if len(seen) < 100 {
		t.Fatalf("only %d distinct figures in 200 children: the mutation is not running", len(seen))
	}

	// And two bodies that watch each other move towards each other, by the
	// same amount, at the same moment.
	a, o := &w.agents[2], &w.agents[3]
	a.lore.mateWeight, o.lore.mateWeight = 20, 80
	w.exchangeLore(a, o)
	if !(a.lore.mateWeight > 20 && o.lore.mateWeight < 80) {
		t.Fatalf("after watching each other they want a child at %v and %v",
			a.lore.mateWeight, o.lore.mateWeight)
	}
	approx(t, a.lore.mateWeight-20, 80-o.lore.mateWeight, 1e-9, "an even trade")
}
