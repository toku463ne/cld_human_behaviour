package engine

import (
	"math"
	"testing"
)

// Stage 95: how hard a body's judgement wobbles is its own, and not merely
// what its intelligence says.
//
// Until this, the amplitude of the error a body made scoring an option was
// (MaxAbility - Intelligence) / MaxAbility * ChoiceNoise. One gene set both
// how well a body could tell two options apart and how far it would wander
// from its own ranking, so there was no axis between impulsive and
// calculating for selection to work on.
func TestHowMuchJudgementWobblesVariesBetweenBodies(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 3
	cfg.NoiseWeightSpread = 0.3
	w := NewWorld(cfg)

	seen := map[float64]bool{}
	for i := range w.agents {
		seen[w.agents[i].lore.noiseWeight] = true
	}
	if len(seen) < 2 {
		t.Fatalf("every founder wobbles exactly as much as every other (%d values)", len(seen))
	}

	// And it is the perception that carries it, which is where pick reads it.
	steady, hasty := &w.agents[0], &w.agents[0]
	for i := range w.agents {
		if w.agents[i].lore.noiseWeight > hasty.lore.noiseWeight {
			hasty = &w.agents[i]
		}
		if w.agents[i].lore.noiseWeight < steady.lore.noiseWeight {
			steady = &w.agents[i]
		}
	}
	// One at a time: the world hands out one perception buffer and fills it
	// again for the next body.
	hi := w.perceive(hasty).Self.NoiseWeight
	lo := w.perceive(steady).Self.NoiseWeight
	if hi <= lo {
		t.Fatalf("the hasty body reads %v and the steady one %v", hi, lo)
	}
}

// With no spread asked for the world is the one it always was: every body
// carries one, nothing is drawn for it and nothing mutates. (That the run is
// bit for bit identical is what the golden test says; this says why.)
func TestWithNoSpreadEverybodyWobblesTheSame(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 3
	w := NewWorld(cfg)
	for i := range w.agents {
		if got := w.agents[i].lore.noiseWeight; got != cfg.NoiseWeight {
			t.Fatalf("a founder in a world with no spread wobbles at %v, want %v", got, cfg.NoiseWeight)
		}
	}
	child := w.inheritLore(&w.agents[0], &w.agents[1])
	if child.noiseWeight != cfg.NoiseWeight {
		t.Fatalf("a child in a world with no spread wobbles at %v", child.noiseWeight)
	}
}

// The trait scales the error that is already there, so two bodies of the same
// intelligence differ in how far they stray from their own ranking - and a
// body that can tell every option apart has nothing for a hunch to move.
func TestTheTraitScalesTheErrorAndNotTheRanking(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)

	// Two options a hair apart, and one obviously bad - the same bench stage 6
	// measured intelligence on.
	right := func(intelligence, weight float64) int {
		c := &AIController{}
		p := &Perception{
			Cfg:  &cfg,
			Self: SelfView{Intelligence: intelligence, NoiseWeight: weight},
			Rand: w.rng}
		count := 0
		for i := 0; i < 2000; i++ {
			c.opts = c.opts[:0]
			c.add(Action{Kind: ActRest}, Utility{Life: Goal{Value: 30, Chance: 1}})
			c.add(Action{Kind: ActMove}, Utility{Life: Goal{Value: 20, Chance: 1}})
			c.add(Action{Kind: ActAttack}, Utility{Life: Goal{Value: -40, Chance: 1}})
			if c.pick(p).Kind == ActRest {
				count++
			}
		}
		return count
	}

	steady, plain, hasty := right(MinAbility, 0.25), right(MinAbility, 1), right(MinAbility, 3)
	if !(steady > plain && plain > hasty) {
		t.Fatalf("a dull body got it right %d times steady, %d plain and %d hasty: want the wobble to tell",
			steady, plain, hasty)
	}
	// A body that can tell the options apart has no error for the multiplier
	// to scale, so being hasty costs it nothing.
	if got := right(MaxAbility, 4); got != 2000 {
		t.Fatalf("a fully intelligent body with the largest wobble got it right %d/2000 times", got)
	}
}

// It is inherited, mutated and rubs off like the preferences it sits with -
// the whole point of putting it there was that none of that had to be written
// again.
func TestHowMuchItWobblesIsInheritedAndRubsOff(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 9
	cfg.NoiseWeightSpread = 0.3
	w := NewWorld(cfg)

	pa, pb := &w.agents[0], &w.agents[1]
	pa.lore.noiseWeight, pb.lore.noiseWeight = 0.5, 2
	seen := map[float64]int{}
	for i := 0; i < 200; i++ {
		seen[w.inheritLore(pa, pb).noiseWeight]++
	}
	low, high := 0, 0
	for v, n := range seen {
		if v < 1.25 {
			low += n
		} else {
			high += n
		}
	}
	if low == 0 || high == 0 {
		t.Fatalf("200 children of a 0.5 and a 2 came out %d low and %d high", low, high)
	}
	if len(seen) < 100 {
		t.Fatalf("only %d distinct figures in 200 children: the mutation is not running", len(seen))
	}

	// And two bodies that watch each other move towards each other, by the
	// same amount, at the same moment.
	a, o := &w.agents[2], &w.agents[3]
	a.lore.noiseWeight, o.lore.noiseWeight = 0.5, 2
	w.exchangeLore(a, o)
	if !(a.lore.noiseWeight > 0.5 && o.lore.noiseWeight < 2) {
		t.Fatalf("after watching each other they wobble at %v and %v", a.lore.noiseWeight, o.lore.noiseWeight)
	}
	approx(t, a.lore.noiseWeight-0.5, 2-o.lore.noiseWeight, 1e-9, "an even trade")
}

// The control the stage turns on: handing every body the same raised amplitude
// is a different world from letting them vary around it, and the config has to
// be able to say which. Without this arm there is no telling whether it is the
// variation that matters or simply the amount of noise (stage 54's flat bias).
func TestTheFlatArmRaisesEverybodyWithoutSpreadingThem(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 5
	cfg.NoiseWeight = 1.4
	w := NewWorld(cfg)
	for i := range w.agents {
		if got := w.agents[i].lore.noiseWeight; got != 1.4 {
			t.Fatalf("a founder in the flat arm wobbles at %v, want 1.4", got)
		}
	}
	// And the population reads back as raised with no spread at all.
	lv := w.Lore()
	if math.Abs(lv.NoiseWeight-1.4) > 1e-9 || lv.SdNoiseWeight > 1e-9 {
		t.Fatalf("the flat arm reads mean %v spread %v", lv.NoiseWeight, lv.SdNoiseWeight)
	}
}
