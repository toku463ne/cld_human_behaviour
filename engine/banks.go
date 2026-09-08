package engine

import "math"

// Whether a river cuts a world in two (stage 37).
//
// This is a measurement and nothing else. No rule reads it, no agent carries
// which bank it is on, and the region grid is not redrawn to follow the water
// (#61): terrain and regions are deliberately separate maps, and bending one
// to fit the other to make a number come out would be measuring the ruler.
//
// The question it answers is the one stage 36 left behind. The bank draws a
// population to it (onWater moved for the first time when the food followed
// the water), and Spacing said the gathering was there while Clusters said it
// was not - a crowd along a river is a ribbon, and single linkage over a
// ribbon is one long component, not a group. So the shape has to be asked
// about directly: do the two sides meet, and have they drifted apart?
//
// Two readings, and both of them need the paired control to mean anything.
//
//   - How often the two sides run into each other. A settlement on each bank
//     would show as a low crossing rate.
//   - How far the two sides have drifted: what they are made of, and what they
//     believe about the same ground. Independent lines diverge.
//
// The line is given rather than found. A world with the river taken out has no
// water to derive it from, and the control has to be measured with the same
// ruler as the arm it is the control for, so the caller passes the same x to
// both.

// Banks is what the two sides of a line are doing. Read only.
type Banks struct {
	// Divide is the line the reading was taken across. Two readings taken
	// across different lines are not comparable.
	Divide float64

	// West and East are how many living humans stood on each side. Only
	// humans: the question is about settlements, and the predators are drawn
	// to whoever they can eat rather than to a place.
	//
	// A body in the water is on the side of the line it is on. The river is
	// two cells wide, so this puts the near half of it on each bank, which is
	// the same choice made twice rather than a rule about water.
	West, East int

	// Split is the smaller side over the whole, so 0.5 is an even population
	// and 0 is everybody on one bank. Read it before anything below: a
	// crossing rate over an empty bank is zero for a reason that has nothing
	// to do with the river.
	Split float64

	// Pairs is how many pairs of humans could see each other, and Cross is
	// the share of those that straddled the line.
	Pairs, Cross float64

	// CrossIndex is Cross over the share there would have been if the same
	// population had been paired off without regard to which bank each was
	// standing on - every pair of the two, over every pair there is. One is a
	// line that means nothing; zero is two populations that never meet.
	//
	// It is NOT to be read on its own, for the reason gapRel taught (stage
	// 12c): a rule that moves the density moves the denominator too. Agents
	// cluster locally whatever the ground is, so even a flat world scores
	// well under one here. What the number is for is the difference between
	// an arm and its control.
	CrossIndex float64

	// CrossDry is CrossIndex again over the bodies standing on dry ground
	// only. The water of this world is lived in rather than crossed (stage
	// 34: the average stay in it is 127 ticks), so a body in the middle of a
	// wide river sees both banks and is counted as meeting the far side. This
	// is the reading with those bodies left out: if the two shores keep apart
	// and only the river people mix, the two figures come apart.
	CrossDry float64

	// GeneGap is how differently the two banks are built: half the sum of the
	// absolute differences between their mean budget splits, so 0 is the same
	// body on both sides and 1 is no gene in common. Shares rather than raw
	// genes, for the usual reason - a bank with more food grows bigger
	// bodies, and that is not divergence.
	GeneGap float64

	// CountryGap is how differently the two banks see the same ground: the
	// mean absolute difference between their beliefs about a region, over the
	// regions both sides have a view of, divided by the mean belief. Relative
	// because a poorer arm believes in smaller numbers everywhere, which
	// would read as agreement.
	//
	// CountryRegions is how many regions that was over. With a low count the
	// gap is a couple of agents' opinions.
	CountryGap, CountryRegions float64
}

// Banks reads the two sides of a vertical line. O(n^2) in the population, like
// Clusters, and meant to be sampled every so often rather than every tick.
func (w *World) Banks(divideX float64) Banks {
	out := Banks{Divide: divideX}

	west := make([]int, 0, len(w.agents))
	east := make([]int, 0, len(w.agents))
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive || a.Species != SpeciesHuman {
			continue
		}
		if a.X < divideX {
			west = append(west, i)
		} else {
			east = append(east, i)
		}
	}
	out.West, out.East = len(west), len(east)
	total := out.West + out.East
	if total == 0 {
		return out
	}
	out.Split = float64(min(out.West, out.East)) / float64(total)

	out.Pairs, out.Cross, out.CrossIndex = w.crossings(west, east)
	_, _, out.CrossDry = w.crossings(w.onDryGround(west), w.onDryGround(east))
	out.GeneGap = w.geneGap(west, east)
	out.CountryGap, out.CountryRegions = w.countryGap(west, east)
	return out
}

// crossings counts the pairs that can see each other and how many of them
// straddle the line.
func (w *World) crossings(west, east []int) (pairs, cross, index float64) {
	all := make([]int, 0, len(west)+len(east))
	all = append(all, west...)
	all = append(all, east...)
	side := make(map[int]bool, len(all))
	for _, i := range west {
		side[i] = true
	}
	for x := 0; x < len(all); x++ {
		a := &w.agents[all[x]]
		for y := x + 1; y < len(all); y++ {
			b := &w.agents[all[y]]
			if !w.canSee(a.X, a.Y, b.X, b.Y) {
				continue
			}
			pairs++
			if side[all[x]] != side[all[y]] {
				cross++
			}
		}
	}
	if pairs == 0 {
		return 0, 0, 0
	}
	cross /= pairs
	// The null is over pairs and not over agents: with everybody in sight of
	// everybody, W*E of the n(n-1)/2 pairs straddle the line, and that is the
	// share a line that separates nothing produces.
	n := float64(len(all))
	if all := n * (n - 1) / 2; all > 0 {
		if expected := float64(len(west)) * float64(len(east)) / all; expected > 0 {
			index = cross / expected
		}
	}
	return pairs, cross, index
}

// onDryGround drops the bodies standing in the water.
func (w *World) onDryGround(idx []int) []int {
	out := make([]int, 0, len(idx))
	for _, i := range idx {
		a := &w.agents[i]
		if w.terrainAt(a.X, a.Y).Kind == GroundWater {
			continue
		}
		out = append(out, i)
	}
	return out
}

// geneGap is the total variation distance between what the two banks spend
// their budgets on.
func (w *World) geneGap(west, east []int) float64 {
	shareOf := func(idx []int) ([NumGenes]float64, bool) {
		var sum [NumGenes]float64
		var n float64
		for _, i := range idx {
			a := &w.agents[i]
			b := a.Budget()
			if b <= 0 {
				continue
			}
			for g := 0; g < NumGenes; g++ {
				sum[g] += a.Gene(Gene(g)) / b
			}
			n++
		}
		if n == 0 {
			return sum, false
		}
		for g := range sum {
			sum[g] /= n
		}
		return sum, true
	}
	sw, okW := shareOf(west)
	se, okE := shareOf(east)
	if !okW || !okE {
		return 0
	}
	var d float64
	for g := range sw {
		d += math.Abs(sw[g] - se[g])
	}
	return d / 2
}

// countryGap is how far the two banks' views of the same region have drifted,
// relative to what a view is worth on average.
func (w *World) countryGap(west, east []int) (gap, regions float64) {
	if len(w.regions) == 0 {
		return 0, 0
	}
	mean := func(idx []int, r int) (float64, bool) {
		var sum, n float64
		for _, i := range idx {
			a := &w.agents[i]
			seen, known := w.regionEstimate(a, r)
			if !known {
				continue
			}
			sum += seen
			n++
		}
		if n == 0 {
			return 0, false
		}
		return sum / n, true
	}
	var total, views float64
	for r := range w.regions {
		mw, okW := mean(west, r)
		me, okE := mean(east, r)
		if !okW || !okE {
			continue
		}
		gap += math.Abs(mw - me)
		regions++
		total += mw + me
		views += 2
	}
	if regions == 0 {
		return 0, 0
	}
	gap /= regions
	if views > 0 && total > 0 {
		gap /= total / views
	}
	return gap, regions
}

// --- how often a body changes sides ------------------------------------------

// BankTracker follows the same population over time and counts how often a
// body is found on the other bank from where it was.
//
// It is here because the readings above cannot tell two things apart. Two
// banks whose beliefs and bodies stay in step could be two settlements that
// talk over the water, or one population that walks through it. Counting the
// crossings says which.
//
// Like every other tracker in these files it writes nothing to the world, and
// it undercounts on purpose: a body that crosses and comes back between two
// observations is not seen to have moved at all. That makes it a floor rather
// than an estimate, which is all it is asked to be - a floor above zero
// already settles the question.
type BankTracker struct {
	divide   float64
	watching map[int]*bankHistory
	lastTick int
	started  bool

	moves, personTicks  float64
	gone, goneBothBanks float64
}

type bankHistory struct {
	west      bool
	bothBanks bool
	seen      bool
}

// BankMoves is what the tracker saw.
type BankMoves struct {
	// Rate is crossings per ten thousand person-ticks, the same unit the
	// experiment runner puts births and killings in.
	Rate float64

	// Ever is the share of the bodies watched that were seen on both banks at
	// some point. It is the one to read: a rate says how busy the ford is, a
	// share says whether an ordinary life stays on one side.
	Ever float64

	// Watched is how many bodies that is over.
	Watched float64
}

// NewBankTracker starts watching across a line. The line has to be the same
// one Banks is read at, or the two readings are of different worlds.
func NewBankTracker(divide float64) *BankTracker {
	return &BankTracker{divide: divide, watching: map[int]*bankHistory{}}
}

// Observe takes one look. Call it on a fixed cadence.
func (t *BankTracker) Observe(w *World) {
	dt := 0
	if t.started {
		dt = w.Tick() - t.lastTick
	}
	t.lastTick, t.started = w.Tick(), true

	for _, h := range t.watching {
		h.seen = false
	}
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive || a.Species != SpeciesHuman {
			continue
		}
		west := a.X < t.divide
		h, ok := t.watching[a.ID]
		if !ok {
			t.watching[a.ID] = &bankHistory{west: west, seen: true}
			continue
		}
		h.seen = true
		t.personTicks += float64(dt)
		if h.west != west {
			h.west = west
			h.bothBanks = true
			t.moves++
		}
	}
	// A body that is no longer there has finished its life on one bank or on
	// both, and its answer is kept while the record itself is dropped.
	for id, h := range t.watching {
		if h.seen {
			continue
		}
		t.gone++
		if h.bothBanks {
			t.goneBothBanks++
		}
		delete(t.watching, id)
	}
}

// Result is what has been seen so far.
func (t *BankTracker) Result() BankMoves {
	out := BankMoves{}
	if t.personTicks > 0 {
		out.Rate = t.moves / t.personTicks * 10000
	}
	both, watched := t.goneBothBanks, t.gone
	for _, h := range t.watching {
		watched++
		if h.bothBanks {
			both++
		}
	}
	out.Watched = watched
	if watched > 0 {
		out.Ever = both / watched
	}
	return out
}
