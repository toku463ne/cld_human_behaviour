package engine

import (
	"math"
	"testing"
)

// The world's own fingerprint, first recorded before the spatial index was
// wired into anything and rewritten whenever a rule deliberately changes.
//
// Rewritten on 2026-09-01, when inheritance stopped being the average of the
// parents, and again the same day when mutation became rare and large instead
// of a nudge on every birth (both stage 7b), and again when the genes started
// being paid for out of a budget (stage 7c), and again when memory became
// finite and resting in the open stopped being free (stage 9), and again when
// agents started being born small and growing into themselves (stage 7d), and
// again when bringing a carcass down together started being remembered (the
// remainder of stage 11), and again when a stranger stopped being worth the
// same flat prior to everybody (stage 10), and again when what an agent wants
// out of the world stopped being the same for everybody (stage 12a), and again
// when agents started trading it and carrying rules of thumb (stages 12b and
// 12c), and again when a race for food started being judged on who would
// arrive first rather than on who was nearer (the groundwork for terrain), and
// again when sight stopped being a circle and became a block of cells
// (stage 13), and again when the world got regions that differ in how sheltered
// resting in them is (stage 14), and again when a blow that finds nothing
// started leaving the one who threw it off balance (stage 24), and again when
// courting stopped being gated by a hunger threshold and started being priced
// in the comparison like everything else (stage 25), and again when patience
// stopped removing the bar a candidate has to clear and started only lowering
// it (stage 26), and again when the comparison clock stopped restarting on
// every fresh attempt at courting (stage 28), and again when a bond that had
// run its course stopped only producing a child if the loop happened to reach
// the lower-numbered of the two partners first (2026-09-06, a bug: about half
// of all bonds had been ending in nothing), and again when a killing started
// leaving something with the people who saw it (stage 31, 2026-09-08).
//
// The last one is worth a word, because the numbers below moved by more than
// the rule turned out to be worth. Measured over 48 seeds, the whole of stage
// 31 moves no figure of the world outside the noise; three seeds of five
// thousand ticks is not a measurement but a fingerprint, and a fingerprint
// moves whenever anything at all draws differently from the random source.
//
// And again when a carcass started mending whoever ate it (stage 39,
// 2026-09-09). That is the largest deliberate move these numbers have made in
// a while, and it is the one worth remembering: average vitality is up about
// twenty points in all three seeds, because the world gained a second way of
// getting better that does not require lying down. Measured over 48 seeds it
// is also the change that finally made pack hunting appear - party size 1.36
// to 1.81 - after four attempts that did not.
//
// And again when a body could pick something up and eat it later (stage 40,
// 2026-09-09). Part of that move is not carrying at all: a ninth word in the
// vocabulary changes what a rule of thumb can be about, so the hints drawn at
// every birth differ from the first tick, exactly as they did when calling
// others in was added.
//
// And again when what a body cannot eat stopped being a meal in its hand
// (found in stage 51, 2026-09-10). That is a rule change and it moves the
// world on purpose: gifts asked whether the receiver had a hand free and not
// whether it could eat the thing, so an enemy could be handed a plant and eat
// it (17,066 body-ticks of holding one in five thousand ticks) and a human
// could be handed human meat. Both are refused at the mouth and always have
// been. Now a gift is refused if the receiver could do nothing with it, and
// what cannot be eaten is not a meal in the hand either.
//
// And again when money went in (stage 51, 2026-09-10). A fourteenth word, and
// a world with no coins in it is bit-identical - coins are scattered by
// whoever lays the world out and the default world has none. What moves the
// numbers is the width of the vocabulary a rule of thumb can be about, as at
// stages 32, 40, 46, 49 and 50; held at its old width the default world does
// not move, which is how that was checked.
//
// And again when a body got a word for putting something in a cache, and when
// what a thing kept for later is worth stopped coming out as a loss (stage 50,
// 2026-09-10). Two changes in one place.
//
// The word is the thirteenth, and a world with no cache in it is bit-identical
// - caches are laid out by whoever lays the world out, and the default world
// has none. What the word moves is the width of the vocabulary a rule of thumb
// can be about, as at stages 32, 40, 46 and 49; held at its old width the
// default world does not move, which is how that was checked.
//
// The other one is a rule change and moves the world on purpose. Stage 40
// wrote what a held item is worth as the difference between how a body stands
// now and how it will stand when it runs short - a loss - where what was meant
// was the good that eating it then would do. Of 138,310 carry options scored
// in one run of this world, not one had a positive value. Measured over 48
// seeds the fix is worth a quarter more picking up (takeRate +0.92 ***) and no
// change to anything else; CarryPricedBackwards puts the old world back, which
// is where every figure for stages 40 to 49 was measured.
//
// And again when a body got a word for crying what is in its hand (stage 49,
// 2026-09-10). A twelfth word, and this one is off by default: no agent cries
// in these runs and OfferTicks is zero. What moves the numbers is only the
// width of the vocabulary a rule of thumb can be about, exactly as at stages
// 32, 40 and 46 - held at its old width, the world with the rule off is
// bit-identical, which is how that was checked.
//
// And again when a body could hand what it was holding to somebody else
// (stage 48, 2026-09-09). An eleventh word, and this one is used: measured
// over 48 seeds it is worth about nine population in the world these numbers
// come from, and ninety-five per cent of what changes hands goes to a
// stranger.
//
// And again when a tenth word was added to the vocabulary - throwing a stone
// (stage 46, 2026-09-09). The rule itself is off by default and no stone is
// ever thrown in these runs; what moves the numbers is that a rule of thumb
// can now be about one more kind of action, so the hints drawn at every birth
// differ from the first tick, exactly as they did at stages 32 and 40.
//
// And again when what a kill leaves beyond what its party can carry away
// stopped being theirs to wait for (stage 41, 2026-09-09).
//
// And again when agents got a word for calling others in against something,
// and started counting on whoever had declared for the same target (stage 32,
// 2026-09-08). That one does move the world: measured over 48 seeds the
// population is up about thirteen.
//
// Stage 7a is a change of how the neighbours are found, not of what happens,
// and its completion condition is that the same seed gives the same run. That
// cannot be checked by running two copies of the same binary against each other
// - both would be wrong together. It needs numbers from the world as it was, so
// here they are.
//
// These are also the numbers to look at when a later stage changes the rules on
// purpose: they will move, and the diff says by how much. Rewrite them when
// that happens, in the commit that changes the rule, and never to make a red
// test go green.
var goldenRuns = []struct {
	seed                              int64
	pop, births, deaths, kills, aging int
	fights, gen                       int
	power, vitality, hunger           float64
}{
	{seed: 1, pop: 41, births: 24, deaths: 65, kills: 47, aging: 0, fights: 4810, gen: 2, power: 42.054951, vitality: 102.714171, hunger: 27.566267},
	{seed: 2, pop: 47, births: 39, deaths: 69, kills: 56, aging: 0, fights: 5742, gen: 3, power: 39.088412, vitality: 93.314940, hunger: 26.678677},
	{seed: 3, pop: 45, births: 32, deaths: 69, kills: 51, aging: 0, fights: 3055, gen: 3, power: 40.145344, vitality: 93.556899, hunger: 28.012042},
}

const goldenTicks = 5000

func TestDefaultWorldStillRunsTheSameWayItAlwaysHas(t *testing.T) {
	for _, want := range goldenRuns {
		cfg := DefaultConfig()
		cfg.Seed = want.seed
		w := NewWorld(cfg)
		for i := 0; i < goldenTicks; i++ {
			w.Step()
		}

		got := w.Stats()
		seed := want.seed
		check := func(name string, got, want int) {
			t.Helper()
			if got != want {
				t.Errorf("seed %d after %d ticks: %s = %d, want %d", seed, goldenTicks, name, got, want)
			}
		}
		check("population", got.Population, want.pop)
		check("births", got.Births, want.births)
		check("deaths", got.Deaths, want.deaths)
		check("kills", got.Kills, want.kills)
		check("aging deaths", got.AgingDeaths, want.aging)
		check("fights", got.Fights, want.fights)
		check("generations", got.MaxGeneration, want.gen)

		// The averages catch a divergence that happens to leave the counts
		// alone, which the counts on their own would miss.
		checkFloat := func(name string, got, want float64) {
			t.Helper()
			if math.Abs(got-want) > 1e-6 {
				t.Errorf("seed %d after %d ticks: %s = %.6f, want %.6f", seed, goldenTicks, name, got, want)
			}
		}
		checkFloat("average power", got.AvgPower, want.power)
		checkFloat("average vitality", got.AvgVitality, want.vitality)
		checkFloat("average hunger", got.AvgHunger, want.hunger)
	}
}
