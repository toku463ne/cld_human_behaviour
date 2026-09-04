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
// (stage 13).
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
	{seed: 1, pop: 39, births: 23, deaths: 65, kills: 49, aging: 0, fights: 4633, gen: 3, power: 39.864408, vitality: 64.830745, hunger: 17.914921},
	{seed: 2, pop: 34, births: 13, deaths: 61, kills: 36, aging: 0, fights: 3516, gen: 2, power: 39.236518, vitality: 69.947191, hunger: 17.491620},
	{seed: 3, pop: 20, births: 9, deaths: 71, kills: 57, aging: 0, fights: 3377, gen: 2, power: 50.693566, vitality: 75.960540, hunger: 23.059721},
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
