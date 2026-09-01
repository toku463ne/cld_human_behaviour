package engine

import (
	"math"
	"sort"
	"testing"
)

// The gamma sampler is written here rather than taken from a library, so it is
// checked against what the distribution is supposed to be: mean and variance
// both equal the shape.
func TestGammaMatchesItsDistribution(t *testing.T) {
	w := NewWorld(testConfig())
	for _, shape := range []float64{0.3, 0.8, 1, 3, 12} {
		const n = 40000
		var sum, sq float64
		for i := 0; i < n; i++ {
			v := w.gamma(shape)
			if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("shape %.1f produced %v", shape, v)
			}
			sum += v
			sq += v * v
		}
		mean := sum / n
		variance := sq/n - mean*mean
		// Three standard errors of the mean is about 0.005*sqrt(shape); a
		// couple of percent is a loose enough bar to never flake and a tight
		// enough one to catch a wrong sampler.
		if math.Abs(mean-shape) > 0.05*shape {
			t.Errorf("shape %.1f: mean %.3f, want %.3f", shape, mean, shape)
		}
		if math.Abs(variance-shape) > 0.12*shape {
			t.Errorf("shape %.1f: variance %.3f, want %.3f", shape, variance, shape)
		}
	}
}

func TestDirichletSplitsAWhole(t *testing.T) {
	w := NewWorld(testConfig())
	for i := 0; i < 200; i++ {
		g := w.dirichlet(NumGenes, 0.8)
		var sum float64
		for _, v := range g {
			if v < 0 {
				t.Fatalf("negative share %v", v)
			}
			sum += v
		}
		approx(t, sum, 1, 1e-9, "the shares of one draw")
	}
}

// What alpha is for: below one the budget lands mostly on a few genes, above
// it every gene gets a similar share. The whole point of drawing the founders
// this way is that the first generation contains different kinds of
// individual, not nine near-equal shares of the same shape.
func TestDirichletAlphaDecidesHowLopsidedTheSplitIs(t *testing.T) {
	w := NewWorld(testConfig())
	topShare := func(alpha float64) float64 {
		var sum float64
		const n = 2000
		for i := 0; i < n; i++ {
			g := w.dirichlet(NumGenes, alpha)
			sort.Float64s(g)
			sum += g[len(g)-1]
		}
		return sum / n
	}
	lopsided, even := topShare(0.3), topShare(8)
	// Nine even shares would be 0.11 each; at alpha 0.3 the biggest gene
	// measures about 0.48 of the budget, and at alpha 8 about 0.17.
	if lopsided < 0.4 {
		t.Errorf("alpha 0.3 put only %.2f of the budget on the biggest gene", lopsided)
	}
	if even > 0.25 {
		t.Errorf("alpha 8 put %.2f on the biggest gene, want a near-even split", even)
	}
	if lopsided <= even {
		t.Errorf("alpha did nothing: %.2f against %.2f", lopsided, even)
	}
}

func TestFitBudgetKeepsTheTotalInsideTheRange(t *testing.T) {
	w := NewWorld(testConfig())
	for i := 0; i < 500; i++ {
		budget := w.randRange(float64(NumGenes)*MinAbility, float64(NumGenes)*MaxAbility)
		g := w.dirichlet(NumGenes, 0.4) // lopsided, so the clip bites
		for j := range g {
			g[j] *= budget
		}
		fitBudget(g, budget)

		var sum float64
		for _, v := range g {
			if v < MinAbility-1e-9 || v > MaxAbility+1e-9 {
				t.Fatalf("gene %v is outside the range", v)
			}
			sum += v
		}
		approx(t, sum, budget, 1e-6, "the budget after fitting")
	}
}

// A budget nobody could spend is pulled back to one that can be: nine genes
// cannot add up to less than nine or more than nine hundred.
func TestFitBudgetHandlesABudgetThatCannotBeSpent(t *testing.T) {
	for _, budget := range []float64{-500, 0, 1e6} {
		g := make([]float64, NumGenes)
		for i := range g {
			g[i] = 40
		}
		fitBudget(g, budget)
		var sum float64
		for _, v := range g {
			if v < MinAbility-1e-9 || v > MaxAbility+1e-9 {
				t.Fatalf("budget %v left gene %v outside the range", budget, v)
			}
			sum += v
		}
		want := clamp(budget, float64(NumGenes)*MinAbility, float64(NumGenes)*MaxAbility)
		approx(t, sum, want, 1e-6, "the budget after fitting an impossible one")
	}
}

func TestFoundersAreDrawnAroundTheBudget(t *testing.T) {
	cfg := testConfig()
	cfg.InitialPopulation = 2000
	w := NewWorld(cfg)

	var sum, sq float64
	for _, a := range w.Agents() {
		b := a.Budget()
		if len(a.Genome) != NumGenes {
			t.Fatalf("genome has %d genes, want %d", len(a.Genome), NumGenes)
		}
		sum += b
		sq += b * b
	}
	n := float64(len(w.Agents()))
	mean := sum / n
	sd := math.Sqrt(sq/n - mean*mean)
	if math.Abs(mean-cfg.GeneBudgetMean) > 5 {
		t.Errorf("mean budget %.1f, want about %.0f", mean, cfg.GeneBudgetMean)
	}
	if math.Abs(sd-cfg.GeneBudgetStd) > 6 {
		t.Errorf("budget spread %.1f, want about %.0f", sd, cfg.GeneBudgetStd)
	}
}

// Founders differ in kind, not only in total: some of them are mostly one
// thing. Nine equal shares would be 0.11 each.
func TestFoundersSpendTheirBudgetDifferently(t *testing.T) {
	cfg := testConfig()
	cfg.InitialPopulation = 500
	w := NewWorld(cfg)

	var topSum float64
	for _, a := range w.Agents() {
		top := 0.0
		for _, v := range a.Genome {
			top = math.Max(top, v)
		}
		topSum += top / a.Budget()
	}
	top := topSum / float64(len(w.Agents()))
	if top < 0.2 {
		t.Errorf("the biggest gene of the average founder is %.2f of its budget: the split is too even to be interesting", top)
	}
}

// A gene with no rules yet still has to reach the generation that gives it one.
func TestChildInheritsGenesThatNothingReadsYet(t *testing.T) {
	cfg := testConfig()
	cfg.MutationRate = 0
	cfg.BudgetInheritSpread = 0
	w := NewWorld(cfg)
	pa := &Agent{Vitality: 90, Genome: filledGenome(30)}
	pb := &Agent{Vitality: 90, Genome: filledGenome(50)}

	for i := 0; i < 100; i++ {
		pa.Vitality, pb.Vitality = 90, 90
		w.tryBirth(pa, pb)
	}
	for i := range w.newborns {
		c := &w.newborns[i]
		if len(c.Genome) != NumGenes {
			t.Fatalf("child has %d genes, want %d", len(c.Genome), NumGenes)
		}
		// Scaled onto the inherited budget, so it is the share that came from
		// one parent or the other.
		scale := pickedFromOneParent(t, c, pa, pb)
		for g := range c.Genome {
			oneOf(t, c.Genome[g]/scale, 30, 50, Gene(g).String())
		}
	}
}

// A genius is rare, and it is a step to a level rather than a windfall
// proportional to what its parents had.
func TestGeniusBirthsAreRareAndLarge(t *testing.T) {
	cfg := testConfig()
	cfg.MutationRate = 0
	cfg.BudgetInheritSpread = 0
	w := NewWorld(cfg)
	pa := &Agent{Vitality: 90, Genome: filledGenome(20)}
	pb := &Agent{Vitality: 90, Genome: filledGenome(20)}
	ordinary := pa.Budget()

	const births = 20000
	counts := map[float64]int{}
	for i := 0; i < births; i++ {
		pa.Vitality, pb.Vitality = 90, 90
		w.newborns = w.newborns[:0]
		w.tryBirth(pa, pb)
		for j := range w.newborns {
			counts[math.Round(w.newborns[j].Budget())]++
		}
	}

	genius := counts[math.Round(cfg.GeniusBudget)]
	great := counts[math.Round(cfg.GreatGeniusBudget)]
	plain := counts[math.Round(ordinary)]
	if plain+genius+great != births {
		t.Fatalf("births came out at budgets other than the three expected: %v", counts)
	}
	rate := float64(genius) / births
	if rate < cfg.GeniusRate*0.7 || rate > cfg.GeniusRate*1.3 {
		t.Errorf("genius rate %.4f, want about %.4f", rate, cfg.GeniusRate)
	}
	greatRate := float64(great) / births
	if greatRate < cfg.GreatGeniusRate*0.4 || greatRate > cfg.GreatGeniusRate*1.8 {
		t.Errorf("great genius rate %.5f, want about %.5f", greatRate, cfg.GreatGeniusRate)
	}
	if got, want := w.Stats().Geniuses, genius; got != want {
		t.Errorf("Stats counted %d geniuses, want %d", got, want)
	}
	if got, want := w.Stats().GreatGeniuses, great; got != want {
		t.Errorf("Stats counted %d great geniuses, want %d", got, want)
	}
}

// The windfall is inherited like any other budget: a genius's children start
// from what it had, not from what its grandparents had.
func TestAGeniusPassesItsBudgetOn(t *testing.T) {
	cfg := testConfig()
	cfg.MutationRate = 0
	cfg.BudgetInheritSpread = 0
	cfg.GeniusRate, cfg.GreatGeniusRate = 0, 0
	w := NewWorld(cfg)

	genius := &Agent{Vitality: 90, Genome: filledGenome(cfg.GeniusBudget / float64(NumGenes))}
	plain := &Agent{Vitality: 90, Genome: filledGenome(20)}
	for i := 0; i < 200; i++ {
		genius.Vitality, plain.Vitality = 90, 90
		w.tryBirth(genius, plain)
	}
	rich := 0
	for i := range w.newborns {
		if math.Abs(w.newborns[i].Budget()-cfg.GeniusBudget) < 1e-6 {
			rich++
		}
	}
	if rich == 0 {
		t.Fatal("no child of a genius inherited its budget")
	}
	if rich == len(w.newborns) {
		t.Fatal("every child inherited the genius budget: the coin between the parents is not being thrown")
	}
}
