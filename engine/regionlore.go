package engine

import "math"

// What an agent has made of the ground (stages 15b and 15c).
//
// The world's regions differ in how well they grow plants (region.go), and
// until now nothing could know that: an agent found good ground by standing on
// it and finding something, which is sorting rather than choosing. This is the
// knowing.
//
// Three things are deliberately not here.
//
//   - No new sense. What an agent learns about a region is how much food it saw
//     while it was in it, which it was already looking at.
//   - No new kind of memory. The estimate fades on being read and its clock is
//     reset by being there, exactly as an opinion about a person does (#22).
//   - No room taken from what it can remember about people (#41). Somewhere is
//     not somebody: an agent that knows a lot of country has not thereby
//     forgotten its neighbours. What the memory gene does buy here is how
//     quickly the estimate moves and how long it lasts.
//
// An agent can only form a view of ground it has actually been on. That is the
// point at which stage 15c earns its keep: hearing about somewhere you have
// never been is the only way to be drawn to it, which makes handing the
// knowledge on worth something rather than merely possible.

// regionView is one agent's estimate of one region.
type regionView struct {
	// seen is how much food it remembers being in sight there, and n how many
	// looks that is averaged over. n of zero is "never been", which is not the
	// same as "nothing there" and is why the two are kept apart.
	seen float64
	n    float64

	// logN is log(n), kept because the test for "has this faded to nothing"
	// is read far more often than it is written - twelve regions on every
	// perception - and n * exp(-rate * elapsed) < 1 is the same question as
	// rate * elapsed > log(n). Storing the log turns twelve transcendentals
	// per decision into twelve multiplications. Written wherever n is; see
	// setSeen.
	logN float64

	lastTick int
}

// setSeen is the only place a view is written, so that logN cannot drift out
// of step with n.
func (v *regionView) setSeen(seen, n float64, tick int) {
	v.seen, v.n, v.lastTick = seen, n, tick
	v.logN = math.Log(n)
}

// faded reports whether this view has been away from long enough to be worth
// nothing, at the rate the holder forgets. It is the hot half of
// regionEstimate, split out so the rate is worked out once for a whole sweep
// of the regions rather than once for each.
func (v *regionView) faded(rate float64, tick int) bool {
	if v.n <= 0 {
		return true
	}
	elapsed := tick - v.lastTick
	if elapsed <= 0 || rate <= 0 {
		return false
	}
	return rate*float64(elapsed) > v.logN
}

// knowsRegions reports whether this agent has been anywhere at all. Cheap
// enough to call before doing any of the work below.
func (a *Agent) knowsRegions() bool { return len(a.regions) > 0 }

// noteRegion folds one look at the ground into what an agent makes of it.
//
// The reading is off by an amount that shrinks with rationality, exactly as
// every other reading of the world is, and how fast it moves the estimate is
// what the memory gene buys (#41).
func (w *World) noteRegion(a *Agent, foodInSight int) {
	cfg := &w.cfg
	if cfg.RegionLearnRate <= 0 || len(w.regions) == 0 {
		return
	}
	i := w.regionIndexAt(a.X, a.Y)
	if a.regions == nil {
		a.regions = make([]regionView, len(w.regions))
	}
	v := &a.regions[i]

	// A misreading of the ground, the same shape as a misreading of anybody's
	// strength: worse for an agent that reads the world badly.
	reading := float64(foodInSight) + w.judgementError(a, cfg.RegionNoise)
	if reading < 0 {
		reading = 0
	}

	// Faster to take in and slower to lose for an agent that spent on
	// remembering. The cap is the same idea as a belief's (lore.go): a
	// lifetime of old looks must not make it unable to notice that the ground
	// has changed.
	scale := a.MemoryScale(cfg)
	n := math.Min(v.n+1, cfg.RegionMemory*scale)
	v.setSeen(v.seen+(reading-v.seen)*cfg.RegionLearnRate/n, n, w.tick)
}

// regionForgetRate is how fast this agent loses country, worked out once for a
// sweep of the regions rather than once per region.
func (w *World) regionForgetRate(a *Agent) float64 {
	return w.cfg.RegionForgetPerTick / math.Max(a.MemoryScale(&w.cfg), 1e-9)
}

// regionEstimate is what an agent makes of a region now, or false if it has
// never been there.
//
// What it remembers fades towards nothing in particular the longer it is since
// it was there, at the rate its memory gene sets. Fading is applied on reading
// rather than every tick, which is how every other memory in the world works.
func (w *World) regionEstimate(a *Agent, i int) (float64, bool) {
	if i < 0 || i >= len(a.regions) {
		return 0, false
	}
	// The confidence fades, not the figure: an old memory of somewhere is
	// still what the agent thinks of it, it just counts for less against
	// anything it hears or sees.
	v := &a.regions[i]
	if v.faded(w.regionForgetRate(a), w.tick) {
		return 0, false
	}
	return v.seen, true
}

// bestKnownRegion is the ground this agent thinks best of, and how much better
// than where it is standing. It returns false when it knows nowhere better.
func (w *World) bestKnownRegion(a *Agent) (idx int, gain float64, ok bool) {
	if !a.knowsRegions() {
		return 0, 0, false
	}
	rate, tick := w.regionForgetRate(a), w.tick
	hereIdx := w.regionIndexAt(a.X, a.Y)
	if hereIdx >= len(a.regions) || a.regions[hereIdx].faded(rate, tick) {
		// It cannot tell whether anywhere is better than where it is if it
		// does not know where it is. It will in a moment: it is standing there.
		return 0, 0, false
	}
	here := a.regions[hereIdx].seen

	best, bestSeen := -1, here
	for i := range a.regions {
		if v := &a.regions[i]; v.seen > bestSeen && !v.faded(rate, tick) {
			best, bestSeen = i, v.seen
		}
	}
	if best < 0 {
		return 0, 0, false
	}
	return best, bestSeen - here, true
}

// --- handing it on (stage 15c) ----------------------------------------------

// exchangeRegions is what the ground is worth talking about, and it rides on
// the same trade as everything else two agents swap (stage 12b): no separate
// path, no "tell" action, and the same meeting in the middle.
//
// Where both have been, they meet in the middle like any other figure. Where
// one has been and the other has not, the other simply takes it - there is
// nothing to average against, and this is the whole reason the stage exists:
// somewhere you have never been is somewhere you can only hear about.
//
// A handed-down view carries less confidence than a seen one. What it is worth
// is the same either way; how readily the next look overturns it is not.
func (w *World) exchangeRegions(a, o *Agent) float64 {
	cfg := &w.cfg
	if cfg.RegionLearnRate <= 0 || cfg.LoreExchangeRate <= 0 || len(w.regions) == 0 {
		return 0
	}
	if a.regions == nil && o.regions == nil {
		return 0
	}
	if a.regions == nil {
		a.regions = make([]regionView, len(w.regions))
	}
	if o.regions == nil {
		o.regions = make([]regionView, len(w.regions))
	}

	moved := 0.0
	for i := range w.regions {
		mine, iKnow := w.regionEstimate(a, i)
		theirs, theyKnow := w.regionEstimate(o, i)
		switch {
		case iKnow && theyKnow:
			gap := theirs - mine
			step := gap * cfg.LoreExchangeRate
			a.regions[i].seen += step
			o.regions[i].seen -= step
			moved += 2 * abs(step)
		case theyKnow && cfg.RegionToldCount > 0:
			a.regions[i].setSeen(theirs, cfg.RegionToldCount, w.tick)
			moved += abs(theirs)
		case iKnow && cfg.RegionToldCount > 0:
			o.regions[i].setSeen(mine, cfg.RegionToldCount, w.tick)
			moved += abs(mine)
		}
	}
	// In the same units the rest of a trade is measured in: a share of the
	// world's own figure for the thing being traded.
	if cfg.RegionPrior > 0 {
		moved /= cfg.RegionPrior
	}
	return moved
}

// --- reading it out ---------------------------------------------------------

// CountryKnownBy is how much of the world one agent has a view of, and how
// much better than where it stands the best of it is believed to be. For the
// viewer; read only.
func (w *World) CountryKnownBy(id int) (known, total int, gain float64) {
	a := w.agentByID(id)
	if a == nil {
		return 0, len(w.regions), 0
	}
	for r := range w.regions {
		if _, ok := w.regionEstimate(a, r); ok {
			known++
		}
	}
	_, gain, _ = w.bestKnownRegion(a)
	return known, len(w.regions), gain
}

// RegionKnowledge is what the population has made of the ground.
type RegionKnowledge struct {
	// Known is how many regions the average agent has a view of, and Told the
	// share of those it was told about rather than stood in.
	Known float64
	Told  float64

	// Rank is how well the population orders the ground: the correlation
	// between what agents believe about a region and how well it actually
	// grows plants, over every agent-region pair anybody has a view on. One is
	// perfect, zero is knowing nothing.
	Rank float64

	// Spread is how much agents disagree about the same region, averaged over
	// regions. It is what says whether a population has come to share a view
	// of its country or each holds its own.
	Spread float64
}

// RegionKnowledge reports what the living population believes about the ground.
// Read only.
func (w *World) RegionKnowledge() RegionKnowledge {
	var out RegionKnowledge
	if len(w.regions) == 0 {
		return out
	}

	var agents, views, told float64
	// Sums for the correlation between belief and truth, and per region for
	// the spread.
	var sx, sy, sxx, syy, sxy float64
	sum := make([]float64, len(w.regions))
	sumSq := make([]float64, len(w.regions))
	count := make([]float64, len(w.regions))

	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		agents++
		for r := range w.regions {
			seen, known := w.regionEstimate(a, r)
			if !known {
				continue
			}
			views++
			if a.regions[r].n <= w.cfg.RegionToldCount {
				told++
			}
			truth := w.regions[r].Food
			sx += seen
			sy += truth
			sxx += seen * seen
			syy += truth * truth
			sxy += seen * truth
			sum[r] += seen
			sumSq[r] += seen * seen
			count[r]++
		}
	}
	if agents == 0 {
		return out
	}
	out.Known = views / agents
	if views > 0 {
		out.Told = told / views
	}
	if views > 1 {
		num := views*sxy - sx*sy
		den := math.Sqrt((views*sxx - sx*sx) * (views*syy - sy*sy))
		if den > 0 {
			out.Rank = num / den
		}
	}
	regions := 0.0
	for r := range w.regions {
		if count[r] < 2 {
			continue
		}
		mean := sum[r] / count[r]
		if v := sumSq[r]/count[r] - mean*mean; v > 0 {
			out.Spread += math.Sqrt(v)
		}
		regions++
	}
	if regions > 0 {
		out.Spread /= regions
	}
	return out
}
