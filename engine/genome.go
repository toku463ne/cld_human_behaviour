package engine

import "math"

// This file holds what an agent is born with. Everything heritable lives in one
// vector, addressed by name, rather than in a field per ability.
//
// The point of the vector is the budget that comes with it (stage 7c of
// PLAN.md): the genes are meant to be paid for out of a total an agent inherits,
// so that being better at one thing costs being worse at another. A field per
// ability cannot express that - there is nothing to add up - and every rule that
// wants "the sum of what this agent is" would have to name the fields it knows
// about, which is exactly what breaks when a gene is added.
//
// Not every gene has a job yet. Defence and evasion wait for the channels of
// stage 8 and memory for the capacity of stage 9; they are here from the start
// because adding a gene later moves the budget under every measurement taken
// before it. A gene with no job still costs what is spent on it, which is the
// honest thing for it to do.
//
// The vector is longer than NumGenes when stage 12 adds hint slots to it, so
// nothing here may assume its length.

// Gene names one entry of the genome.
type Gene int

const (
	// What an agent is made of, in the order PLAN.md lists them.
	GeneAttack Gene = iota
	GeneDefence
	GeneVitality
	GeneSpeed
	GeneEvasion
	GeneMemory
	GeneRationality
	GeneIntelligence
	GeneAttractiveness

	// NumGenes is how many of them there are. It is not the length of a
	// genome: stage 12 appends hint slots, which are inherited the same way
	// and paid for out of the same budget.
	NumGenes = int(iota)
)

func (g Gene) String() string {
	switch g {
	case GeneAttack:
		return "attack"
	case GeneDefence:
		return "defence"
	case GeneVitality:
		return "vitality"
	case GeneSpeed:
		return "speed"
	case GeneEvasion:
		return "evasion"
	case GeneMemory:
		return "memory"
	case GeneRationality:
		return "rationality"
	case GeneIntelligence:
		return "intelligence"
	case GeneAttractiveness:
		return "attractiveness"
	}
	return "gene"
}

// GeneNames lists the genes in order, for anything that prints a genome.
var GeneNames = func() []string {
	names := make([]string, NumGenes)
	for g := 0; g < NumGenes; g++ {
		names[g] = Gene(g).String()
	}
	return names
}()

// Gene reads one gene, and is safe on an agent whose genome is short or
// missing: a gene that is not there is not expressed.
func (a *Agent) Gene(g Gene) float64 {
	if int(g) >= len(a.Genome) {
		return 0
	}
	return a.Genome[g]
}

// The genes that already have a job, named so that the rules read as rules
// rather than as array indexing.
func (a *Agent) Attack() float64       { return a.Gene(GeneAttack) }
func (a *Agent) Rationality() float64  { return a.Gene(GeneRationality) }
func (a *Agent) Intelligence() float64 { return a.Gene(GeneIntelligence) }

// Budget is what this agent's genome adds up to: the total it has to spend
// across everything it could be.
func (a *Agent) Budget() float64 {
	var sum float64
	for _, v := range a.Genome {
		sum += v
	}
	return sum
}

// newGenome returns a genome of the standard length with every gene at zero.
func newGenome() []float64 { return make([]float64, NumGenes) }

// genomeOf builds a genome from the three abilities that have rules today,
// leaving the rest at the least a gene may hold. It is what the world used to
// hold as three fields.
func genomeOf(attack, rationality, intelligence float64) []float64 {
	g := newGenome()
	for i := range g {
		g[i] = MinAbility
	}
	g[GeneAttack] = attack
	g[GeneRationality] = rationality
	g[GeneIntelligence] = intelligence
	return g
}

// cloneGenome copies a genome. Agents are held and passed by value, so two
// agents sharing one backing array would quietly share their genes; every
// agent gets its own.
func cloneGenome(src []float64) []float64 {
	out := make([]float64, max(len(src), NumGenes))
	copy(out, src)
	return out
}

// --- drawing a genome -------------------------------------------------------

// drawGenome draws a founder: a budget, and a way of spending it.
//
// The two are drawn separately on purpose. The budget says how much this
// individual got to be made of, and the allocation says what it was spent on;
// keeping them apart is what lets the experiment runner ask about one without
// the other, and what makes "share of the budget" the figure to compare
// between agents instead of the raw gene.
func (w *World) drawGenome() []float64 {
	budget := w.cfg.GeneBudgetMean + w.rng.NormFloat64()*w.cfg.GeneBudgetStd
	g := w.dirichlet(NumGenes, w.cfg.GeneInitAlpha)
	for i := range g {
		g[i] *= budget
	}
	fitBudget(g, budget)
	return g
}

// dirichlet draws a random split of one whole into n parts.
//
// Below alpha 1 the split is lopsided - most of the budget on a few genes -
// and above it every part tends towards an equal share. Normalising n uniform
// draws, which is the thing everybody writes first, is not a Dirichlet at all:
// it is far too even, and it cannot be made lopsided.
func (w *World) dirichlet(n int, alpha float64) []float64 {
	if alpha <= 0 {
		alpha = 1
	}
	out := make([]float64, n)
	var sum float64
	for i := range out {
		out[i] = w.gamma(alpha)
		sum += out[i]
	}
	if sum <= 0 {
		// Every draw underflowed, which a very small alpha can do. An even
		// split is a poor answer but a defined one.
		for i := range out {
			out[i] = 1 / float64(n)
		}
		return out
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

// gamma draws from the gamma distribution with the given shape and scale 1,
// by Marsaglia and Tsang's method. There is no sampler in the standard
// library and this project does not add dependencies for arithmetic; it needs
// nothing but the normal and uniform draws the world already has, so the run
// stays reproducible from its seed.
func (w *World) gamma(shape float64) float64 {
	if shape < 1 {
		// Boost a shape below one into the range the method covers, then
		// shrink the result back.
		g := w.gamma(shape + 1)
		u := w.rng.Float64()
		if u <= 0 {
			return 0
		}
		return g * math.Pow(u, 1/shape)
	}
	d := shape - 1.0/3.0
	c := 1 / math.Sqrt(9*d)
	for {
		x := w.rng.NormFloat64()
		v := 1 + c*x
		if v <= 0 {
			continue
		}
		v = v * v * v
		u := w.rng.Float64()
		if u < 1-0.0331*x*x*x*x {
			return d * v
		}
		if math.Log(u) < 0.5*x*x+d*(1-v+math.Log(v)) {
			return d * v
		}
	}
}

// fitBudget pulls a genome back onto its budget after the per gene range has
// been applied.
//
// Clipping alone would not do: a lopsided allocation puts more than MaxAbility
// on a gene, the clip throws the excess away, and the agent quietly ends up
// with less than it was given - which would make "share of the budget"
// meaningless exactly for the extreme individuals the budget is there to
// allow. So the genes that are not pinned at a bound are rescaled to take up
// the slack, repeatedly, because rescaling can push another gene onto a bound.
func fitBudget(g []float64, budget float64) {
	if len(g) == 0 {
		return
	}
	lo, hi := float64(len(g))*MinAbility, float64(len(g))*MaxAbility
	budget = clamp(budget, lo, hi)

	for round := 0; round < 32; round++ {
		var pinned, free float64
		for i := range g {
			g[i] = clamp(g[i], MinAbility, MaxAbility)
			if g[i] <= MinAbility || g[i] >= MaxAbility {
				pinned += g[i]
			} else {
				free += g[i]
			}
		}
		gap := budget - (pinned + free)
		if math.Abs(gap) < 1e-9 {
			return
		}
		if free <= 0 {
			// Everything sits on a bound. Nudge them all and let the next
			// round's clamp sort out which ones can actually move.
			for i := range g {
				g[i] += gap / float64(len(g))
			}
			continue
		}
		scale := (free + gap) / free
		for i := range g {
			if g[i] > MinAbility && g[i] < MaxAbility {
				g[i] *= scale
			}
		}
	}
}
