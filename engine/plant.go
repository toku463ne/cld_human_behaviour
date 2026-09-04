package engine

import "math"

// Plants that inherit something (stage 17a, decision #42).
//
// Until now a plant was a position and nothing else, and where plants appeared
// was a property of the ground (stage 15a). It still is - poor ground seeds
// less - but where a plant comes up is now mostly a matter of where its parent
// stood and how far that parent throws its seed.
//
// What a plant deliberately does not get:
//
//   - The nine-gene budget. Agents share it because they trade combat against
//     sight against memory; a plant has no fights, no senses and no decisions,
//     so the whole apparatus would be scaffolding around two numbers (#42).
//   - A Decide(). Seeding is not a choice. It rides on the food spawn that was
//     already there, as a probabilistic event of the world, and all that is new
//     is which plant the event copies.
//
// What it does get is the inheritance the agents already use: the parent's
// values, unchanged most of the time, with a rare large jump (stage 7b). Two
// genes, because two is what the question needs.
type plantGenes struct {
	// Spread is how far a seed lands from its parent. Small means a thicket
	// where the parent stood; large means the offspring are scattered and
	// mostly land somewhere their parent knew nothing about.
	Spread float64

	// Regrow is how readily this plant seeds at all, relative to its
	// neighbours. It decides which plant the next seeding copies and never how
	// many seedings there are: the world grows exactly as much food as
	// FoodSpawnRate says, and this only says whose children they are.
	//
	// That is what makes it a fitness rather than a subsidy. A plant that
	// seeds twice as readily takes the place of one that does not.
	Regrow float64

	// Poison is what eating this one costs, and Signal is how loudly it says
	// so - smell and conspicuousness together, which is one thing rather than
	// two because that is how they are argued about (#43).
	//
	// They are two genes and nothing ties them together. Whether a warning is
	// honest is left to the world to decide: a plant that is all signal and no
	// poison is trading on the reputation of ones that mean it, and if that
	// pays it will happen. Nothing here is arranged to make it happen and
	// nothing is arranged to stop it.
	//
	// Poison is hidden, as every ability of an agent is. What an eater gets is
	// the signal, read with the error its rationality leaves it - which is
	// what gives rationality a job on the food's side of the world for the
	// first time.
	Poison float64
	Signal float64
}

// drawPlantGenes is what the first plants of a world are. They are drawn around
// the world's figures with a spread, for the reason the founders' preferences
// are (lore.go): a population whose members are all identical has nothing for
// selection to work on.
func (w *World) drawPlantGenes() plantGenes {
	cfg := &w.cfg
	g := plantGenes{Spread: cfg.PlantSpread, Regrow: 1}
	if cfg.PlantGenetics {
		g.Spread = clamp(w.randRange(cfg.PlantSpread*0.4, cfg.PlantSpread*1.6), 1, cfg.PlantSpreadMax)
		g.Regrow = clamp(w.randRange(0.4, 1.6), plantRegrowFloor, plantRegrowMax)
	}
	if cfg.PlantDefence {
		// Drawn independently, which is the whole point: an honest warning
		// has to be something the world arrives at, not something the world
		// was built with.
		g.Poison = clamp(w.rng.Float64(), 0, 1)
		g.Signal = clamp(w.rng.Float64(), 0, 1)
	}
	return g
}

// The range a plant's readiness to seed is kept in. A floor above zero because
// a lineage that cannot seed at all is not a lineage; a ceiling because without
// one the only thing selection would ever find is "seed harder", and the
// interesting question is what it does with the spread.
const (
	plantRegrowFloor = 0.1
	plantRegrowMax   = 3
)

// inheritPlantGenes is what a seedling is. The parent's values, unchanged most
// of the time, with the occasional large jump - the same shape of inheritance
// the agents got at stage 7b, and for the same reason: a nudge on every seed
// would leave nothing of the parent in the child.
func (w *World) inheritPlantGenes(parent plantGenes) plantGenes {
	cfg := &w.cfg
	out := parent
	if cfg.PlantMutationRate <= 0 {
		return out
	}
	if cfg.PlantGenetics {
		if w.rng.Float64() < cfg.PlantMutationRate {
			out.Spread = clamp(out.Spread*(1+w.rng.NormFloat64()*cfg.PlantMutationStd), 1, cfg.PlantSpreadMax)
		}
		if w.rng.Float64() < cfg.PlantMutationRate {
			out.Regrow = clamp(out.Regrow*(1+w.rng.NormFloat64()*cfg.PlantMutationStd),
				plantRegrowFloor, plantRegrowMax)
		}
	}
	if cfg.PlantDefence {
		// Additive rather than proportional, because these run 0 to 1 and a
		// proportional jump can never lift a plant off zero.
		if w.rng.Float64() < cfg.PlantMutationRate {
			out.Poison = clamp(out.Poison+w.rng.NormFloat64()*cfg.PlantMutationStd, 0, 1)
		}
		if w.rng.Float64() < cfg.PlantMutationRate {
			out.Signal = clamp(out.Signal+w.rng.NormFloat64()*cfg.PlantMutationStd, 0, 1)
		}
	}
	return out
}

// defenceParent picks a standing plant for a new one to take its defences
// from, uniformly.
//
// Uniform is what makes this selection at all. Nothing weights the draw, so a
// plant's only way of being picked more often is to still be there - and what
// keeps a plant standing is not being eaten. Poison is selected for by
// surviving, which is the mechanism it exists for, rather than by any rule
// that says poison is good.
//
// It is deliberately separate from seedFrom. What broke the world at 17a was
// where plants come up, not what they pass on, so the defences can be
// inherited on a map that still puts plants where the ground is good.
func (w *World) defenceParent() (plantGenes, bool) {
	n := 0
	for i := range w.foods {
		if w.foods[i].Kind == FoodPlant {
			n++
		}
	}
	if n == 0 {
		return plantGenes{}, false
	}
	k := w.rng.Intn(n)
	for i := range w.foods {
		if w.foods[i].Kind != FoodPlant {
			continue
		}
		if k == 0 {
			return w.foods[i].Genes, true
		}
		k--
	}
	return plantGenes{}, false
}

// dangerOf is what an eater makes of a plant: the warning it can see, read with
// the error its own rationality leaves it.
//
// It reads the signal and believes it. Nothing here checks whether the plant
// meant it, which is exactly the opening a liar needs - and whether anything
// takes it is the question the two genes were left uncorrelated to ask.
func (w *World) dangerOf(observer *Agent, f *Food) float64 {
	if !w.cfg.PlantDefence || f.Kind != FoodPlant {
		return 0
	}
	return clamp(f.Genes.Signal+w.judgementError(observer, w.cfg.SignalNoise), 0, 1)
}

// seedFrom picks which plant the next one grows from.
//
// Weighted by how readily each seeds and by how well the ground it stands on
// grows things, so that stage 15a's regions still say where the food is
// plentiful while the plants themselves say where it goes next. Returns -1 when
// there is nothing to seed from, which is how a world with no plants left
// starts again from the ground rather than from nothing.
func (w *World) seedFrom() int {
	total := 0.0
	for i := range w.foods {
		if f := &w.foods[i]; f.Kind == FoodPlant {
			total += f.Genes.Regrow * w.richnessAt(f.X, f.Y)
		}
	}
	if total <= 0 {
		return -1
	}
	r := w.rng.Float64() * total
	for i := range w.foods {
		f := &w.foods[i]
		if f.Kind != FoodPlant {
			continue
		}
		r -= f.Genes.Regrow * w.richnessAt(f.X, f.Y)
		if r <= 0 {
			return i
		}
	}
	return -1
}

// --- carried in a gut (stage 17c, decision #44) ------------------------------
//
// A seed that survives being eaten comes up wherever the animal that ate it
// happens to be a while later. It rides on the existing eat and adds no action
// and no "carrying" behaviour: an agent that has eaten simply has a seed in it
// for a time, and where it walks in the meantime is where the plant ends up.
//
// It matters because wind dispersal alone is a rich-get-richer process. A seed
// lands near its parent, so ground with plants gets more and ground without
// gets none, and a region that empties can never be seeded into again - the
// absorbing state PLAN.md asked to watch for. Animals are the only thing in
// this world that goes from where the food is to where it is not.
//
// The count is untouched, as everywhere else in the food rules: a carried seed
// takes the place of the world's next planting rather than adding to it.

// noteSeedEaten gives an agent a seed to carry, some of the time.
func (w *World) noteSeedEaten(a *Agent, f *Food) {
	cfg := &w.cfg
	if f.Kind != FoodPlant || cfg.SeedSurvival <= 0 {
		return
	}
	if w.rng.Float64() >= cfg.SeedSurvival {
		return
	}
	// One at a time. A gut is not a granary, and keeping a queue per agent
	// would be a new kind of state for a rule that is meant to ride on the
	// eating that was already happening.
	a.seed, a.seedDueAt = f.Genes, w.tick+cfg.SeedGutTicks
}

// dropSeeds plants whatever has finished its journey. Called once a tick, after
// everybody has moved, so a seed comes up where its carrier ended up rather
// than where it started.
func (w *World) dropSeeds() {
	if w.cfg.SeedSurvival <= 0 {
		return
	}
	for i := range w.agents {
		a := &w.agents[i]
		if a.seedDueAt == 0 || w.tick < a.seedDueAt {
			continue
		}
		a.seedDueAt = 0
		if !a.Alive {
			continue
		}
		w.pendingSeeds = append(w.pendingSeeds, pendingSeed{x: a.X, y: a.Y, genes: a.seed})
	}
}

// pendingSeed is a seed that has been carried somewhere and is waiting for the
// world's next planting to be the one that comes up.
type pendingSeed struct {
	x, y  float64
	genes plantGenes
}

// takePendingSeed hands back the oldest seed waiting to come up, if any.
func (w *World) takePendingSeed() (pendingSeed, bool) {
	if len(w.pendingSeeds) == 0 {
		return pendingSeed{}, false
	}
	// Oldest first, and the queue is capped: a world that is not planting
	// fast enough must not accumulate seeds for ever.
	if len(w.pendingSeeds) > maxPendingSeeds {
		w.pendingSeeds = w.pendingSeeds[len(w.pendingSeeds)-maxPendingSeeds:]
	}
	s := w.pendingSeeds[0]
	w.pendingSeeds = w.pendingSeeds[1:]
	return s, true
}

const maxPendingSeeds = 64

// --- reading it out ---------------------------------------------------------

// PlantLife is what the plants have become.
type PlantLife struct {
	Spread float64 // mean of how far a seed is thrown
	Regrow float64 // mean of how readily one is thrown at all

	// Clumping is how much more crowded a plant's neighbourhood is than it
	// would be if the same number were scattered at random. One is scattered;
	// above one is thickets. It is the shape dispersal makes, and the reason
	// the spread gene is worth having at all.
	Clumping float64

	// Poison and Signal are what the standing crop has become, and Honesty the
	// correlation between them across it. Honesty is the one that answers the
	// question the two genes were left uncorrelated to ask: at zero the
	// warnings mean nothing, near one the world has arrived at telling the
	// truth, and below zero it has arrived at lying.
	Poison  float64
	Signal  float64
	Honesty float64

	// Carried is how many seeds are in transit inside an animal right now
	// (stage 17c). Zero when nothing is carried.
	Carried float64

	// Empty is the share of the world's regions with no plants in them at all.
	// Zero is an absorbing state for a region under wind dispersal - nothing
	// can seed where nothing grows - so this is watched the way the rarer
	// species' trough is.
	Empty float64
}

// Plants reports what the plants have become. Read only, and O(n^2) in the
// clumping, so sample it rather than calling it every tick.
func (w *World) Plants() PlantLife {
	var out PlantLife
	var xs, ys []float64
	for i := range w.foods {
		f := &w.foods[i]
		if f.Kind != FoodPlant {
			continue
		}
		out.Spread += f.Genes.Spread
		out.Regrow += f.Genes.Regrow
		xs = append(xs, f.X)
		ys = append(ys, f.Y)
	}
	n := float64(len(xs))
	if n == 0 {
		out.Empty = 1
		return out
	}
	out.Spread /= n
	out.Regrow /= n

	// What the crop is defended with, and whether its warnings mean anything.
	var sp, ss, spp, sss, sps float64
	for i := range w.foods {
		f := &w.foods[i]
		if f.Kind != FoodPlant {
			continue
		}
		p, g := f.Genes.Poison, f.Genes.Signal
		sp, ss = sp+p, ss+g
		spp, sss, sps = spp+p*p, sss+g*g, sps+p*g
	}
	out.Poison, out.Signal = sp/n, ss/n
	if n > 1 {
		den := math.Sqrt((n*spp - sp*sp) * (n*sss - ss*ss))
		if den > 0 {
			out.Honesty = (n*sps - sp*ss) / den
		}
	}

	// How many other plants are within a plant's own spread of it, against how
	// many there would be if the same number were scattered evenly.
	r := w.cfg.PlantSpread
	near := 0.0
	for i := range xs {
		for j := range xs {
			if i != j && dist2(xs[i], ys[i], xs[j], ys[j]) <= r*r {
				near++
			}
		}
	}
	if area := w.cfg.Width * w.cfg.Height; area > 0 {
		expected := (n - 1) * math.Pi * r * r / area
		if expected > 0 {
			out.Clumping = near / n / expected
		}
	}

	for i := range w.agents {
		if a := &w.agents[i]; a.Alive && a.seedDueAt > 0 {
			out.Carried++
		}
	}

	if len(w.regions) > 0 {
		occupied := make([]bool, len(w.regions))
		for i := range w.foods {
			if f := &w.foods[i]; f.Kind == FoodPlant {
				occupied[w.regionIndexAt(f.X, f.Y)] = true
			}
		}
		empty := 0
		for _, ok := range occupied {
			if !ok {
				empty++
			}
		}
		out.Empty = float64(empty) / float64(len(occupied))
	}
	return out
}
