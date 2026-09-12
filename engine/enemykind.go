package engine

// Kinds of enemy (stage 59).
//
// The world has two species and it is going to keep two: a kind is not a third
// species but a row in a table, in exactly the way stage 11 already said the
// difference between a human and an enemy is - the same node, the same
// decision engine, and a different range to draw its budget from.
//
// Adding a kind is meant to cost one entry in Config.EnemyKinds and nothing
// else. Every rule below reads the table rather than counting rows, an unset
// field falls back to the world's own figure, and the shares do not have to
// add up to anything in particular. Two kinds is what stages 60 to 64 are
// measured with; the machinery does not know that.
//
// What a kind must never become is a branch in the decision engine. If a rule
// ever wants to ask "which kind is this", the question belongs in a number on
// the row - what it eats, how far it strays, how much of a region's arrivals
// it takes - so that a new kind is a new row rather than a new case.

// EnemyKind is one sort of enemy: what it is built like and where it turns up.
type EnemyKind struct {
	// Name is for the viewer and for the experiment's tables. It changes
	// nothing.
	Name string

	// Share is how many of the world's arriving enemies are this kind,
	// relative to the other rows. They are normalised, so 1 and 3 means a
	// quarter and three quarters, and a row left at zero never arrives.
	Share float64

	// BudgetMean and BudgetStd are the range this kind's body is drawn from.
	// Zero means the world's own figure (EnemyBudgetMean, EnemyBudgetStd), so
	// a row that only wants to change where it lives says nothing about size.
	BudgetMean float64
	BudgetStd  float64

	// Homing is how much this kind follows the map's own weighting of where
	// enemies arrive (stage 58, region.Enemies). One is the full weighting,
	// zero is anywhere at all, and in between is a kind that prefers its
	// country without being tied to it.
	//
	// It is a blend rather than a second map: a kind does not get a weighting
	// of its own to draw, because the map's dangerous country is one place
	// and two kinds disagreeing about where it is would make it two.
	Homing float64
}

// enemyKinds is the table as the world actually uses it: the configured rows,
// or one row standing for the enemy the world had before this stage.
//
// The single default row matters more than it looks. Everything below is
// written so that a world with one kind draws from the random source exactly
// as it did before kinds existed - no choice is made when there is nothing to
// choose between - which is what keeps every figure recorded before 2026-09-12
// readable.
func (w *World) enemyKinds() []EnemyKind {
	if len(w.cfg.EnemyKinds) > 0 {
		return w.cfg.EnemyKinds
	}
	return []EnemyKind{{
		Name:       "enemy",
		Share:      1,
		BudgetMean: w.cfg.EnemyBudgetMean,
		BudgetStd:  w.cfg.EnemyBudgetStd,
		Homing:     1,
	}}
}

// pickEnemyKind draws which kind is arriving. It takes nothing from the random
// source when there is only one, which is the default world.
func (w *World) pickEnemyKind() int {
	kinds := w.enemyKinds()
	if len(kinds) <= 1 {
		return 0
	}
	total := 0.0
	for i := range kinds {
		total += max(kinds[i].Share, 0)
	}
	if total <= 0 {
		return 0
	}
	r := w.rng.Float64() * total
	for i := range kinds {
		r -= max(kinds[i].Share, 0)
		if r <= 0 {
			return i
		}
	}
	return len(kinds) - 1
}

// kindOf is the row an agent belongs to, and the first row for anything else -
// humans included, which never read it.
func (w *World) kindOf(a *Agent) EnemyKind {
	kinds := w.enemyKinds()
	if int(a.Kind) >= len(kinds) {
		return kinds[0]
	}
	return kinds[a.Kind]
}

// budgetRangeFor is the range a founder of this species and kind is drawn
// from. An unset figure on the row falls back to the world's own.
func (w *World) budgetRangeFor(species Species, kind int) (mean, std float64) {
	if species != SpeciesEnemy {
		return w.cfg.GeneBudgetMean, w.cfg.GeneBudgetStd
	}
	kinds := w.enemyKinds()
	if kind < 0 || kind >= len(kinds) {
		kind = 0
	}
	mean, std = kinds[kind].BudgetMean, kinds[kind].BudgetStd
	if mean <= 0 {
		mean = w.cfg.EnemyBudgetMean
	}
	if std <= 0 {
		std = w.cfg.EnemyBudgetStd
	}
	return mean, std
}

// KindUse is what the table actually produced (stage 59).
//
// It is written to say something whatever the table's length is: a run with
// one kind reads one and nothing else, and a run with six needs no new
// columns. What it reports is whether the mix is the one the map asked for and
// whether the rows really differ.
type KindUse struct {
	// Kinds is how many rows the world is running.
	Kinds int

	// MixError is how far the arrivals are from the shares the table asked
	// for: the mean absolute difference over the rows. Zero is a world that
	// came out as written.
	//
	// It is about what the world puts in rather than what is standing about
	// later, because with a handful of enemies alive at any moment the
	// standing mix drifts far enough to say nothing about the table.
	MixError float64

	// Toughest over Weakest is how far apart the rows turned out in the
	// bodies they actually produced. One is a table whose rows are the same
	// creature under different names.
	BudgetGap float64

	// Homed is how much better the kinds ended up sitting in their own
	// country than the map's average - the same reading as Prowl.EnemyGain,
	// weighted by each kind's Homing, so that a table of stay-at-homes and a
	// table of wanderers do not average into nothing.
	Homed float64
}

// Kinds reports what the table produced. Read only.
func (w *World) Kinds() KindUse {
	kinds := w.enemyKinds()
	out := KindUse{Kinds: len(kinds), BudgetGap: 1}

	counts := make([]float64, len(kinds))
	budgets := make([]float64, len(kinds))
	var enemies float64
	var homed, homedWeight float64
	mean := 0.0
	if n := len(w.regions); n > 0 {
		mean = w.enemyWeight / float64(n)
	}
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive || a.Species != SpeciesEnemy {
			continue
		}
		k := int(a.Kind)
		if k >= len(kinds) {
			k = 0
		}
		counts[k]++
		budgets[k] += a.Budget()
		enemies++
		if h := kinds[k].Homing; h > 0 {
			homed += h * (w.prowlAt(a.X, a.Y) - mean)
			homedWeight += h
		}
	}
	if enemies == 0 {
		return out
	}

	wanted := 0.0
	for i := range kinds {
		wanted += max(kinds[i].Share, 0)
	}
	if wanted > 0 && w.enemyArrivals > 0 {
		for i := range kinds {
			arrived := 0.0
			if i < len(w.enemyArrivalsByKind) {
				arrived = w.enemyArrivalsByKind[i]
			}
			out.MixError += abs(arrived/w.enemyArrivals - max(kinds[i].Share, 0)/wanted)
		}
		out.MixError /= float64(len(kinds))
	}

	low, high := 0.0, 0.0
	for i := range kinds {
		if counts[i] == 0 {
			continue
		}
		b := budgets[i] / counts[i]
		if low == 0 || b < low {
			low = b
		}
		if b > high {
			high = b
		}
	}
	if low > 0 {
		out.BudgetGap = high / low
	}
	if homedWeight > 0 {
		out.Homed = homed / homedWeight
	}
	return out
}

// KindNameOf is which sort of enemy this is, for the viewer, and empty for a
// human or for a world with only one sort in it - where a name on every enemy
// would say nothing.
func (w *World) KindNameOf(id int) string {
	kinds := w.enemyKinds()
	if len(kinds) <= 1 {
		return ""
	}
	a := w.agentByID(id)
	if a == nil || a.Species != SpeciesEnemy {
		return ""
	}
	return w.kindOf(a).Name
}
