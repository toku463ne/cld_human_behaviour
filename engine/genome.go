package engine

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
// leaving the rest at zero. It is what the world used to hold as three fields.
func genomeOf(attack, rationality, intelligence float64) []float64 {
	g := newGenome()
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
