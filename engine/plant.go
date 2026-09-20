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

	// Strain is which of Config.PlantKinds this plant came from, plus one,
	// and zero for a world with one kind of plant - which is every world
	// before 2026-09-20 (decision #134).
	//
	// It is a label rather than a quantity, and it lives among the genes
	// rather than beside them because a seedling has to keep it:
	// inheritPlantGenes copies the parent's values, so the tag rides along
	// for nothing. A kind that dissolved after one generation would make
	// "this country grows berries" true of the first plant only.
	Strain uint8
}

// PlantKind is one sort of plant: a name, how much of the crop is it, and
// where its four genes start (decision #134, 2026-09-20).
//
// A kind is a bundle of starting values and not an axis of its own. That was
// the decision, and it is what keeps this from being a new concept: the genes
// already exist, already inherit and already evolve, so a kind is a corner of
// the space they live in for a lineage to start from and wander away from.
//
// It follows that a kind only shows in a world where plants inherit at all.
// With PlantGenetics and PlantDefence both off - which is the default -
// drawPlantGenes hands every plant the same figures whatever kind it is, and
// the table below changes nothing but the tag.
type PlantKind struct {
	// Name is for a map to point at and for the tables to print. No rule
	// reads it.
	Name string

	// Share is how much of the crop is this kind, relative to the other rows.
	// They are normalised, so 1 and 3 means a quarter and three quarters, and
	// a row left at zero never comes up.
	Share float64

	// Where this kind's genes start. Zero means the world's own figure, so a
	// row that only wants to be poisonous says nothing about spread.
	Spread float64
	Regrow float64
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
	// Which sort this one is, when the map names any (decision #134). It is
	// drawn first so that the spread below is a spread around this kind's own
	// figures rather than around the world's, and a world with no kinds takes
	// nothing from the random source here.
	if k := w.pickPlantKind(); k >= 0 {
		row := &cfg.PlantKinds[k]
		g.Strain = uint8(k + 1)
		if row.Spread > 0 {
			g.Spread = row.Spread
		}
		if row.Regrow > 0 {
			g.Regrow = row.Regrow
		}
		g.Poison, g.Signal = row.Poison, row.Signal
	}
	if cfg.PlantGenetics {
		g.Spread = clamp(w.randRange(g.Spread*0.4, g.Spread*1.6), 1, cfg.PlantSpreadMax)
		g.Regrow = clamp(w.randRange(g.Regrow*0.4, g.Regrow*1.6), plantRegrowFloor, plantRegrowMax)
	}
	if cfg.PlantDefence {
		// Drawn independently, which is the whole point: an honest warning
		// has to be something the world arrives at, not something the world
		// was built with.
		//
		// A kind may say otherwise (decision #134), and where it does the
		// draw is skipped - the same "zero means the world's own figure"
		// the two above use. That does let a map build a liar in, which is
		// exactly what this rule was written not to do; the answer is that
		// the default world still arrives at it, and a map that starts a
		// lineage somewhere has only chosen where it starts, not where it
		// ends. Both figures go on evolving from there.
		if g.Poison == 0 {
			g.Poison = clamp(w.rng.Float64(), 0, 1)
		}
		if g.Signal == 0 {
			g.Signal = clamp(w.rng.Float64(), 0, 1)
		}
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
	total := 0.0
	for i := range w.foods {
		if w.foods[i].Kind == FoodPlant {
			total += w.defenceWeight(&w.foods[i])
		}
	}
	if total <= 0 {
		return plantGenes{}, false
	}
	r := w.rng.Float64() * total
	for i := range w.foods {
		if w.foods[i].Kind != FoodPlant {
			continue
		}
		r -= w.defenceWeight(&w.foods[i])
		if r <= 0 {
			return w.foods[i].Genes, true
		}
	}
	return plantGenes{}, false
}

// defenceWeight is how much of the next generation's defences one standing
// plant gets to write.
//
// One is the plain uniform draw the rule started with, and what the world had
// when stage 17b was first measured: nothing weighted the draw, so a plant's
// only way of being picked more often was to still be there. That turned out
// to leave the two genes with nothing to be selected on but survival, and
// survival did not depend on either of them: poison did not save an eaten
// plant, and shouting only ever cost nothing.
//
// PlantSignalCost is the plant's side of the bargain (the condition
// PARAMETERS.md wrote down for reviving this stage): being conspicuous is paid
// for in seed. Without it the signal has a benefit and no price, runs to the
// top, and every plant in the world says "do not eat me" - which at a dose
// that matters is a population that starves next to its food.
// PlantPoisonCost is the same bargain for the other gene, and it is needed for
// the same reason in reverse: once poison saves a plant from being eaten
// (PlantPoisonSaves) it is a benefit with no price, and a crop that is three
// quarters poisonous is a field nobody can eat.
func (w *World) defenceWeight(f *Food) float64 {
	weight := 1.0
	if w.cfg.PlantSignalCost > 0 {
		weight *= math.Max(1-f.Genes.Signal*w.cfg.PlantSignalCost, 0)
	}
	if w.cfg.PlantPoisonCost > 0 {
		weight *= math.Max(1-f.Genes.Poison*w.cfg.PlantPoisonCost, 0)
	}
	return weight
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

// pickPlantKind draws which sort of plant is coming up, or -1 when the map
// names none - which is every world before this and takes nothing from the
// random source.
//
// The shape is stage 59's, down to the normalising: a table of rows with a
// share each, and adding a sort is a row rather than a branch.
func (w *World) pickPlantKind() int {
	kinds := w.cfg.PlantKinds
	if len(kinds) == 0 {
		return -1
	}
	total := 0.0
	for i := range kinds {
		if kinds[i].Share > 0 {
			total += kinds[i].Share
		}
	}
	if total <= 0 {
		return -1
	}
	r := w.rng.Float64() * total
	for i := range kinds {
		if kinds[i].Share <= 0 {
			continue
		}
		r -= kinds[i].Share
		if r <= 0 {
			return i
		}
	}
	return len(kinds) - 1
}

// PlantStrains is how many of each sort are standing in the world, by the
// index of Config.PlantKinds. Read only, and empty in a world with one sort.
func (w *World) PlantStrains() []int {
	if len(w.cfg.PlantKinds) == 0 {
		return nil
	}
	out := make([]int, len(w.cfg.PlantKinds))
	for i := range w.foods {
		f := &w.foods[i]
		if f.Kind != FoodPlant || f.Genes.Strain == 0 {
			continue
		}
		if k := int(f.Genes.Strain) - 1; k < len(out) {
			out[k]++
		}
	}
	return out
}
