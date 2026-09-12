package engine

import (
	"math"
	"testing"
)

// A world that has not asked for it hands nothing on at a birth, and runs as
// it always did - down to the values taken from the random source.
func TestNoMateTradeLeavesTheWorldExactlyAsItWas(t *testing.T) {
	run := func(chance float64) (Stats, float64) {
		cfg := DefaultConfig()
		cfg.Seed = 103
		cfg.MateLoreChance = chance
		w := NewWorld(cfg)
		for i := 0; i < 3000; i++ {
			w.Step()
		}
		return w.Stats(), w.Mating().Trades
	}
	plain, trades := run(0)
	if trades != 0 {
		t.Fatalf("a world with no rule handed something on %v times", trades)
	}
	if again, _ := run(0); again != plain {
		t.Fatal("the same world twice gave different runs")
	}
	always, alwaysTrades := run(1)
	if alwaysTrades <= 0 {
		t.Fatal("a world that always hands something on never did")
	}
	if always == plain {
		t.Fatal("handing something on at every birth changed nothing at all")
	}
}

// What the birth puts there is the same trade everything else uses: the five
// figures meet in the middle, and neither side can take without giving.
func TestTheBirthTradeIsTheOrdinaryTrade(t *testing.T) {
	cfg := quietConfig()
	cfg.MateLoreChance = 1
	w := NewWorld(cfg)

	// Both added before either pointer is taken: appending can move the slice
	// the agents live in, and a pointer taken before that is a pointer into
	// the old one.
	idA := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Sex: Female,
		Vitality: 100, Genome: filledGenome(50)})
	idB := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Sex: Male,
		Vitality: 100, Genome: filledGenome(50)})
	pa, pb := mustAgent(t, w, idA), mustAgent(t, w, idB)
	pa.lore.riskWeight, pb.lore.riskWeight = 0.10, 0.40
	pa.lore.shockRisk, pb.lore.shockRisk = 0.02, 0.08
	before := pb.lore.riskWeight - pa.lore.riskWeight

	w.tryBirth(pa, pb)

	pa, pb = mustAgent(t, w, pa.ID), mustAgent(t, w, pb.ID)
	after := pb.lore.riskWeight - pa.lore.riskWeight
	if after >= before {
		t.Fatalf("the two are %v apart, and were %v", after, before)
	}
	// Neither side took without giving: both moved by the same step.
	up, down := pa.lore.riskWeight-0.10, 0.40-pb.lore.riskWeight
	if math.Abs(up-down) > 1e-9 {
		t.Fatalf("one moved %v and the other %v", up, down)
	}
	if got := w.Mating().Trades; got != 1 {
		t.Fatalf("the birth put %v trades there, want 1", got)
	}
}

// And the gap is read before the trade, because what it is for is how far
// apart two who breed together were - not how near the rule just put them.
func TestTheGapIsReadBeforeTheTrade(t *testing.T) {
	build := func(chance float64) float64 {
		cfg := quietConfig()
		cfg.MateLoreChance = chance
		w := NewWorld(cfg)
		idA := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Sex: Female,
			Vitality: 100, Genome: filledGenome(50)})
		idB := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Sex: Male,
			Vitality: 100, Genome: filledGenome(50)})
		pa, pb := mustAgent(t, w, idA), mustAgent(t, w, idB)
		pa.lore.riskWeight, pb.lore.riskWeight = 0.10, 0.40
		w.tryBirth(pa, pb)
		return w.Mating().Gap
	}
	if quiet, traded := build(0), build(1); math.Abs(quiet-traded) > 1e-9 {
		t.Fatalf("the gap reads %v with the trade and %v without", traded, quiet)
	}
}

// Nobody was trading with a mate before this stage: while the timer runs the
// pair is only walked together, so no watching happens and no trade can.
func TestABondedPairTradesNothingOnItsOwn(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 107
	w := NewWorld(cfg)
	for i := 0; i < 5000; i++ {
		w.Step()
	}
	if got := w.Mating().Mates; got != 0 {
		t.Fatalf("%v trades happened between two who were bonded", got)
	}
}
