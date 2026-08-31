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

// BenchmarkEngineStep is the baseline: a few hundred agents in a world with
// enough room and food that most of the cost is the ordinary tick loop.
func BenchmarkEngineStep(b *testing.B) {
	w := benchWorld(300, 200, 0.5)
	// Let the world settle so the measurement is not dominated by the opening
	// moments, where nobody has met anybody yet.
	for i := 0; i < 500; i++ {
		w.Step()
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.Step()
	}
}

// BenchmarkEngineStepCrowded squeezes the same population into a fraction of
// the space with hardly any food. Everybody can see everybody, contests and
// fights are constant, and decisions fire in bursts, which is where the
// trigger based approach is most expensive.
func BenchmarkEngineStepCrowded(b *testing.B) {
	cfg := DefaultConfig()
	cfg.Seed = 99
	cfg.Width, cfg.Height = 300, 260
	cfg.InitialPopulation = 300
	cfg.InitialFoodItems = 60
	cfg.MaxPopulation = 600
	cfg.MaxFoodItems = 70
	cfg.FoodSpawnRate = 0.45
	w := NewWorld(cfg)
	for i := 0; i < 500; i++ {
		w.Step()
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.Step()
	}
}

// BenchmarkDecide isolates the utility comparison itself, which is the part
// expected to grow as more kinds of move are added.
func BenchmarkDecide(b *testing.B) {
	w := benchWorld(300, 200, 0.5)
	for i := 0; i < 500; i++ {
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
	cfg := DefaultConfig()
	cfg.Seed = 99
	cfg.Width, cfg.Height = 2400, 1800
	cfg.InitialPopulation = 1200
	cfg.MaxPopulation = 2400
	cfg.InitialFoodItems = 800
	cfg.MaxFoodItems = 1600
	cfg.FoodSpawnRate = 1.8
	w := NewWorld(cfg)
	for i := 0; i < 500; i++ {
		w.Step()
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.Step()
	}
}
