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
// of all bonds had been ending in nothing).
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
	{seed: 1, pop: 57, births: 47, deaths: 68, kills: 40, aging: 0, fights: 3397, gen: 3, power: 35.999942, vitality: 62.160429, hunger: 22.471078},
	{seed: 2, pop: 33, births: 28, deaths: 77, kills: 61, aging: 0, fights: 6301, gen: 3, power: 32.869838, vitality: 70.519226, hunger: 14.134146},
	{seed: 3, pop: 36, births: 24, deaths: 70, kills: 51, aging: 0, fights: 4643, gen: 3, power: 27.320301, vitality: 85.507666, hunger: 22.431841},
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
