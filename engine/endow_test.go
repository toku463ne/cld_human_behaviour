package engine

import "testing"

// The one gift in the engine. What the tests hold in place is that it is a
// gift and not a trade: the other genes do not pay for it.
func TestEndowPaysWithNewBudget(t *testing.T) {
	w := NewWorld(quietConfig())
	g := genomeOf(50, 100, 100)
	g[GeneSpeed] = 21
	id := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Sex: Male,
		Vitality: 80, Hunger: 10, Genome: g})

	before := mustAgent(t, w, id).Budget()
	beforeGenes := append([]float64(nil), mustAgent(t, w, id).Genome...)

	added, err := w.Endow(id, GeneSpeed, 82)
	if err != nil {
		t.Fatal(err)
	}
	if added != 61 {
		t.Fatalf("added %v, want 61", added)
	}
	a := mustAgent(t, w, id)
	if a.Gene(GeneSpeed) != 82 {
		t.Fatalf("speed is %v, want 82", a.Gene(GeneSpeed))
	}
	approx(t, a.Budget(), before+61, 1e-9, "budget after the gift")
	for i := range beforeGenes {
		if Gene(i) == GeneSpeed {
			continue
		}
		if a.Genome[i] != beforeGenes[i] {
			t.Fatalf("gene %d paid for the gift: %v -> %v", i, beforeGenes[i], a.Genome[i])
		}
	}

	// Asking again for the same thing gives nothing.
	if again, err := w.Endow(id, GeneSpeed, 82); err != nil || again != 0 {
		t.Fatalf("a second gift added %v (err %v), want nothing", again, err)
	}
	// And it never goes past what a gene can be.
	if _, err := w.Endow(id, GeneSpeed, MaxAbility*10); err != nil {
		t.Fatal(err)
	}
	if got := mustAgent(t, w, id).Gene(GeneSpeed); got != MaxAbility {
		t.Fatalf("speed is %v, want it clamped to %v", got, MaxAbility)
	}
}

// Nothing in the simulation hands anybody anything. A world nobody is playing
// runs exactly as it did.
func TestNothingInTheWorldEndowsAnybody(t *testing.T) {
	run := func() []Agent {
		cfg := DefaultConfig()
		cfg.Seed = 5
		w := NewWorld(cfg)
		for i := 0; i < 1500; i++ {
			w.Step()
		}
		return w.Agents()
	}
	a, b := run(), run()
	if len(a) != len(b) {
		t.Fatalf("two identical runs differ in population: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Budget() != b[i].Budget() {
			t.Fatalf("agent %d: budgets differ between identical runs", i)
		}
	}
}
