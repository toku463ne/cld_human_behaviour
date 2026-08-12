package engine

import "testing"

// stepsPerRun is how long a benchmarked world is kept running. Past its
// lifespan the initial population dies out and Step would end up measuring an
// almost empty world, so the world is restarted regularly.
const stepsPerRun = 500

// BenchmarkEngineStep is the baseline for the tick loop: a fixed seed and a
// fixed population, stepping over and over.
//
//	go test ./engine/... -run=^$ -bench=. -benchmem
func BenchmarkEngineStep(b *testing.B) {
	cfg := DefaultConfig()
	cfg.Seed = 1
	cfg.InitialPopulation = 300
	cfg.InitialFoodItems = 120

	w := NewWorld(cfg)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i > 0 && i%stepsPerRun == 0 {
			b.StopTimer()
			w = NewWorld(cfg)
			b.StartTimer()
		}
		w.Step()
	}
}

// TestBenchmarkWorldStaysPopulated guards the benchmark itself: if the world it
// measures ran empty, the numbers would mean nothing.
func TestBenchmarkWorldStaysPopulated(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 1
	cfg.InitialPopulation = 300
	cfg.InitialFoodItems = 120
	w := NewWorld(cfg)

	for i := 0; i < stepsPerRun; i++ {
		w.Step()
	}
	if got := w.Stats().Population; got < 200 {
		t.Fatalf("population after %d steps = %d, too low for the benchmark to be meaningful", stepsPerRun, got)
	}
}
