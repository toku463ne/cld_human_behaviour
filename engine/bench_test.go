package engine

import "testing"

// benchWorld builds a world of a fixed size from a fixed seed, so that two runs
// of the benchmark measure the same work.
func benchWorld(agents, foods int, spawn float64) *World {
	cfg := DefaultConfig()
	cfg.Seed = 99
	cfg.InitialPopulation = agents
	cfg.InitialFoodItems = foods
	cfg.MaxPopulation = agents * 2
	cfg.MaxFoodItems = foods * 2
	cfg.FoodSpawnRate = spawn
	return NewWorld(cfg)
}

// How long a world is run before it is measured, and how far it is measured
// for before being built again.
//
// The settling is so the measurement is not dominated by the opening moments,
// where nobody has met anybody yet. The window is the part that matters: see
// benchSteps.
const (
	benchSettle = 500
	benchWindow = 400
)

// benchSteps measures Step over a fixed stretch of a world, rebuilt as often
// as it takes to stay on that stretch.
//
// The obvious way to write this - settle one world, then Step it b.N times -
// does not measure a fixed amount of work, and the mistake is invisible. The
// world goes on evolving while it is being measured, so a longer run is
// measuring somewhere else entirely: whatever the population has become by
// tick 50000 is not what it was at tick 1000. The same code reported anything
// between 82us and 758us depending on how long the benchmark happened to run,
// which made every before-and-after comparison worthless unless both sides
// happened to land on the same b.N.
//
// So the world is thrown away and rebuilt every benchWindow ticks, with the
// clock stopped while that happens. However long the benchmark runs, it is
// averaging over the same stretch of the same world.
func benchSteps(b *testing.B, build func() *World) {
	settle := func() *World {
		w := build()
		for i := 0; i < benchSettle; i++ {
			w.Step()
		}
		return w
	}

	b.ReportAllocs()
	w := settle()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i > 0 && i%benchWindow == 0 {
			b.StopTimer()
			w = settle()
			b.StartTimer()
		}
		w.Step()
	}
}

// BenchmarkEngineStep is the baseline: a few hundred agents in a world with
// enough room and food that most of the cost is the ordinary tick loop.
func BenchmarkEngineStep(b *testing.B) {
	benchSteps(b, func() *World { return benchWorld(300, 200, 0.5) })
}

// BenchmarkEngineStepCrowded squeezes the same population into a fraction of
// the space with hardly any food. Everybody can see everybody, contests and
// fights are constant, and decisions fire in bursts, which is where the
// trigger based approach is most expensive.
func BenchmarkEngineStepCrowded(b *testing.B) {
	benchSteps(b, func() *World {
		cfg := DefaultConfig()
		cfg.Seed = 99
		cfg.Width, cfg.Height = 300, 260
		cfg.InitialPopulation = 300
		cfg.InitialFoodItems = 60
		cfg.MaxPopulation = 600
		cfg.MaxFoodItems = 70
		cfg.FoodSpawnRate = 0.45
		return NewWorld(cfg)
	})
}

// BenchmarkDecide isolates the utility comparison itself, which is the part
// expected to grow as more kinds of move are added.
// It does not need the rebuilding the others do: it never steps the world, so
// the state it measures cannot drift.
func BenchmarkDecide(b *testing.B) {
	w := benchWorld(300, 200, 0.5)
	for i := 0; i < benchSettle; i++ {
		w.Step()
	}
	a := &w.agents[0]
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.ai.Decide(w.perceive(a))
	}
}

// BenchmarkEngineStepLarge is the same world nine times over: the same density
// of agents and food, spread across nine times the area. It is here because the
// spatial index is meant to pay off as the population grows, and the default
// benchmark is too small to show whether it does.
func BenchmarkEngineStepLarge(b *testing.B) {
	benchSteps(b, func() *World {
		cfg := DefaultConfig()
		cfg.Seed = 99
		cfg.Width, cfg.Height = 2400, 1800
		cfg.InitialPopulation = 1200
		cfg.MaxPopulation = 2400
		cfg.InitialFoodItems = 800
		cfg.MaxFoodItems = 1600
		cfg.FoodSpawnRate = 1.8
		return NewWorld(cfg)
	})
}
