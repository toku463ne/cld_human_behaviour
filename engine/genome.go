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
// Not every gene has a job yet. A gene with no job still costs what is spent
// on it, which is the honest thing for it to do: adding a gene later would
// move the budget under every measurement taken before it.
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

// AgeFactor is how much of what this agent inherited it can express right now.
//
// It is one curve with two ends. The young end is growth: a newborn expresses
// ChildAbilityShare of its inheritance and works up to all of it as it grows
// (Maturity, which food buys). The old end is senescence: past
// SenescenceYears the same curve comes back down, towards but never below
// SenescenceFloor. Growing up and wearing out are not two mechanisms here,
// they are the two sides of one question - what can this body do today.
func (a *Agent) AgeFactor(cfg *Config) float64 {
	// The common case is an adult in its prime, and this is read several times
	// per gene per decision, so it gets a way out before any arithmetic: past
	// the growing and short of the declining, the answer is one.
	prime := float64(cfg.SenescenceYears) * float64(cfg.TicksPerYear)
	if a.Maturity >= 1 && (cfg.SenescenceRate <= 0 || prime <= 0 || float64(a.Age) <= prime) {
		return 1
	}

	f := cfg.ChildAbilityShare + (1-cfg.ChildAbilityShare)*clamp(a.Maturity, 0, 1)
	if cfg.SenescenceRate > 0 && prime > 0 {
		if past := (float64(a.Age) - prime) / float64(cfg.TicksPerYear); past > 0 {
			f *= math.Max(cfg.SenescenceFloor, 1-past*cfg.SenescenceRate)
		}
	}
	return f
}

// Ability is what one gene is worth to this agent today: what it inherited,
// scaled by how much of itself it has grown into and how much of that it has
// since lost. It is the only place that multiplication happens - everything
// below and every rule elsewhere goes through here, so there is one answer to
// "how strong is this one" and not two.
//
// Gene is the other reading: the inheritance itself, untouched by age. That is
// what breeding passes on and what the experiments measure, because selection
// acts on what is inherited, not on how old the holder happens to be.
func (a *Agent) Ability(g Gene, cfg *Config) float64 {
	return a.Gene(g) * a.AgeFactor(cfg)
}

// The genes that have a job, named so that the rules read as rules rather than
// as array indexing. All of them are the expressed value, not the inherited
// one; that is why they need the config.
func (a *Agent) Attack(cfg *Config) float64       { return a.Ability(GeneAttack, cfg) }
func (a *Agent) Rationality(cfg *Config) float64  { return a.Ability(GeneRationality, cfg) }
func (a *Agent) Intelligence(cfg *Config) float64 { return a.Ability(GeneIntelligence, cfg) }

// MaxVitality and MaxSpeed are the world's reference figures scaled by what
// this agent spent on being big and on being quick. A gene at the middle of
// the range buys exactly the reference, so an average agent is the agent the
// rest of the parameters were tuned around.
//
// They are the two places the budget bites hardest: a body that holds more
// vitality is a body that is slower, unless its budget stretches to both.
func (a *Agent) MaxVitality(cfg *Config) float64 {
	return cfg.MaxVitality * a.Ability(GeneVitality, cfg) / midAbility
}

func (a *Agent) MaxSpeed(cfg *Config) float64 {
	return cfg.MaxSpeed * a.Ability(GeneSpeed, cfg) / midAbility
}

// MemoryCapacity is how many others this agent can hold an opinion about at
// once, and MemoryBandwidth how many records it can take in during one tick.
//
// Both come from the memory gene, and the bandwidth from the capacity rather
// than from a gene of its own: how much can be held and how fast it arrives
// are two faces of the same organ, and a second gene would only make the
// budget pay for the same thing twice.
//
// Zero means no limit, which is the world as it was before stage 9 and the
// control arm to compare against.
func (a *Agent) MemoryCapacity(cfg *Config) int {
	if cfg.MemoryCapacity <= 0 {
		return 0
	}
	return max(int(float64(cfg.MemoryCapacity)*a.Ability(GeneMemory, cfg)/midAbility), 1)
}

func (a *Agent) MemoryBandwidth(cfg *Config) int {
	capacity := a.MemoryCapacity(cfg)
	if capacity <= 0 || cfg.MemoryBandwidthShare <= 0 {
		return 0
	}
	return max(int(float64(capacity)*cfg.MemoryBandwidthShare), 1)
}

// ForgetScale is how fast this agent's memories fade, as a multiple of the
// world's rate. It is public because it is the one part of a memory a viewer
// has to be told: why one node has forgotten a face another still knows. An agent with an average memory forgets at exactly the world's
// rate, which is what keeps the parameter meaning what it meant before the
// curve became an individual thing; one that spent nothing on memory forgets
// twice as fast, and one at the ceiling barely forgets at all.
func (a *Agent) ForgetScale(cfg *Config) float64 {
	return clamp((MaxAbility-a.Ability(GeneMemory, cfg))/(MaxAbility-midAbility), 0, 2)
}

// MemoryScale is what this agent's memory is worth as a multiple of an
// ordinary one: the other face of ForgetScale, for the things that are held
// better rather than lost slower. An average memory scores exactly one.
//
// It is what the memory gene buys for an agent's view of the ground (stage
// 15b): how many looks its estimate is worth, and how long it lasts. Somewhere
// is not somebody, so knowing the country takes no room away from knowing the
// neighbours (#41) - but it is still the same organ doing it.
func (a *Agent) MemoryScale(cfg *Config) float64 {
	return clamp(a.Ability(GeneMemory, cfg)/midAbility, 0.2, 2)
}

// HungerRate is how fast this agent gets hungry: the world's rate, plus
// whatever it costs to run a body made of more than the average one. It is the
// price of the budget, and the only thing stopping the budget from climbing
// for ever.
func (a *Agent) HungerRate(cfg *Config) float64 {
	if cfg.GeneBudgetMean <= 0 {
		return cfg.HungerRate
	}
	over := a.Bulk(cfg)/cfg.GeneBudgetMean - 1
	return cfg.HungerRate * math.Max(0, 1+cfg.BudgetUpkeep*over)
}

// Bulk is how much body there actually is here today: the budget, scaled the
// way every other expressed value is. It is what the world charges upkeep on
// and what a carcass is worth, because both of those are about the body in
// front of you rather than about what it was born with.
func (a *Agent) Bulk(cfg *Config) float64 { return a.Budget() * a.AgeFactor(cfg) }

// Budget is what this agent's genome adds up to: the total it has to spend
// across everything it could be. Inherited, so untouched by age.
func (a *Agent) Budget() float64 {
	var sum float64
	for _, v := range a.Genome {
		sum += v
	}
	return sum
}

// newGenome returns a genome of the standard length with every gene at zero.
func newGenome() []float64 { return make([]float64, NumGenes) }

// genomeOf builds a genome from the three abilities the older rules were
// written around, leaving every other gene at the middle of its range. It is
// what the world used to hold as three fields, and what a test that is not
// asking about the other genes wants: average in everything it did not name.
func genomeOf(attack, rationality, intelligence float64) []float64 {
	g := newGenome()
	for i := range g {
		g[i] = midAbility
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
func (w *World) drawGenome() []float64 { return w.drawGenomeFor(SpeciesHuman) }

// drawGenomeFor draws a founder of the given species. The species decides only
// the range the budget comes from; the allocation is drawn the same way for
// everybody, and every rule downstream reads the same genes.
func (w *World) drawGenomeFor(species Species) []float64 {
	mean, std := w.cfg.GeneBudgetMean, w.cfg.GeneBudgetStd
	if species == SpeciesEnemy {
		mean, std = w.cfg.EnemyBudgetMean, w.cfg.EnemyBudgetStd
	}
	budget := mean + w.rng.NormFloat64()*std
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
