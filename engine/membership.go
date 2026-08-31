package engine

// This file measures how long the same faces stay together. It is the second of
// the "should be" measurements (PLAN.md stage 6.5), and the one that tells a
// group apart from a crowd: Clusters counts the clumps in a single frame, and a
// clump of agents that happen to be walking past each other looks exactly like
// a group in that frame. Only watching whether the same pairs are still
// together later separates the two.
//
// It is measured pairwise on purpose. Clusters merge and split from tick to
// tick, so "the same cluster, later" has no meaning that survives a merge,
// while "these two are still in one cluster" does.
//
// Like everything else in the measurement files this is read only: it takes
// snapshots of a world it never writes to, and no agent knows it is being
// watched.

import "math"

// The cadence the experiment runner watches at: an observation every
// DefaultMembershipStep ticks, following each cohort of pairs for
// DefaultMembershipLags of those steps.
//
// They are constants for the same reason DefaultClusterLinkDist is: a survival
// curve sampled at one cadence cannot be compared with one sampled at another,
// and the window has to be wide enough that the curve reaches half before it
// runs out (see Membership.Censored).
const (
	DefaultMembershipStep = 25
	DefaultMembershipLags = 80
)

// Membership is the measured survival of shared cluster membership.
type Membership struct {
	// Step is the lag between entries of Survival, in ticks.
	Step int

	// Survival[k] is the fraction of pairs that were in one cluster and are
	// still in one cluster k*Step ticks later, counting only the pairs where
	// both agents are still alive. Survival[0] is 1 by definition.
	//
	// Deaths are excluded rather than counted as partings, so that this
	// measures how long agents stay together and not how long they live.
	Survival []float64

	// HalfLife is how long it takes for half the pairs to have parted, in
	// ticks, read off Survival by interpolating between the two entries it
	// crosses 0.5 between.
	HalfLife float64

	// Censored says the curve never reached 0.5 inside the window, in which
	// case HalfLife is zero and says nothing: the answer is "longer than
	// Step*len(Survival)" and the window has to be widened to get a number.
	Censored bool

	// Pairs is how many pair observations the curve rests on, so that a
	// half-life measured on a handful of pairs can be recognised as one.
	Pairs int
}

// At is the survival at an arbitrary lag in ticks, interpolated between the
// entries either side of it. Unlike HalfLife it is always defined, which makes
// it the figure to compare when the half-life is censored in one arm of an
// experiment and not in the other.
func (m Membership) At(lag int) float64 {
	if m.Step <= 0 || len(m.Survival) == 0 {
		return 0
	}
	if lag <= 0 {
		return m.Survival[0]
	}
	k := lag / m.Step
	if k >= len(m.Survival)-1 {
		return m.Survival[len(m.Survival)-1]
	}
	t := float64(lag-k*m.Step) / float64(m.Step)
	return m.Survival[k] + (m.Survival[k+1]-m.Survival[k])*t
}

// pair is two agent IDs, smallest first, so that a pair has one spelling.
type pair struct{ a, b int }

func makePair(x, y int) pair {
	if x > y {
		x, y = y, x
	}
	return pair{x, y}
}

type cohort struct {
	tick  int
	pairs []pair
}

// MembershipTracker follows cohorts of pairs through a run.
//
// Every observation both starts a new cohort (every pair that shares a cluster
// right now) and checks the cohorts already being followed, so a run of N
// observations contributes N-1 measurements at the shortest lag rather than
// one. That is what makes the curve out of a single run.
type MembershipTracker struct {
	linkDist float64
	step     int
	lags     int

	cohorts []cohort

	together []int
	totals   []int

	// Scratch reused between observations so that watching a run does not
	// allocate a set of pairs every time.
	alive   map[int]struct{}
	cur     map[pair]struct{}
	byLabel [][]int
}

// NewMembershipTracker follows pairs for lags observations of step ticks each,
// clustering at linkDist.
func NewMembershipTracker(linkDist float64, step, lags int) *MembershipTracker {
	if step < 1 {
		step = 1
	}
	if lags < 1 {
		lags = 1
	}
	return &MembershipTracker{
		linkDist: linkDist,
		step:     step,
		lags:     lags,
		together: make([]int, lags+1),
		totals:   make([]int, lags+1),
		alive:    make(map[int]struct{}),
		cur:      make(map[pair]struct{}),
	}
}

// Observe takes one reading of w. Call it every step ticks: the lag of a
// reading is taken from the world's own tick, so an irregular cadence only
// costs resolution, but a cadence coarser than step throws readings away.
func (m *MembershipTracker) Observe(w *World) {
	tick := w.Tick()
	agents := w.Agents()

	clear(m.alive)
	for i := range agents {
		m.alive[agents[i].ID] = struct{}{}
	}

	// Every pair that shares a cluster right now. Singletons contribute
	// nothing, which is what we want: a lone agent has nobody to stay with.
	c := w.Clusters(m.linkDist)
	m.byLabel = m.byLabel[:0]
	for range c.Sizes {
		m.byLabel = append(m.byLabel, nil)
	}
	for i, label := range c.Labels {
		m.byLabel[label] = append(m.byLabel[label], i)
	}
	clear(m.cur)
	for _, members := range m.byLabel {
		for x := 0; x < len(members); x++ {
			for y := x + 1; y < len(members); y++ {
				m.cur[makePair(agents[members[x]].ID, agents[members[y]].ID)] = struct{}{}
			}
		}
	}

	for _, co := range m.cohorts {
		dt := tick - co.tick
		if dt <= 0 {
			continue
		}
		k := (dt + m.step/2) / m.step // nearest whole number of steps
		if k < 1 || k > m.lags {
			continue
		}
		for _, p := range co.pairs {
			if _, ok := m.alive[p.a]; !ok {
				continue
			}
			if _, ok := m.alive[p.b]; !ok {
				continue
			}
			m.totals[k]++
			if _, ok := m.cur[p]; ok {
				m.together[k]++
			}
		}
	}

	pairs := make([]pair, 0, len(m.cur))
	for p := range m.cur {
		pairs = append(pairs, p)
	}
	m.cohorts = append(m.cohorts, cohort{tick: tick, pairs: pairs})

	// Drop the cohorts that have aged out of the window.
	keep := 0
	for _, co := range m.cohorts {
		if tick-co.tick <= m.lags*m.step {
			m.cohorts[keep] = co
			keep++
		}
	}
	m.cohorts = m.cohorts[:keep]
}

// Result reads off the survival curve gathered so far.
func (m *MembershipTracker) Result() Membership {
	out := Membership{Step: m.step, Survival: make([]float64, 0, m.lags+1)}
	out.Survival = append(out.Survival, 1)
	for k := 1; k <= m.lags; k++ {
		if m.totals[k] == 0 {
			break // no readings this far out yet: the curve ends here
		}
		out.Pairs += m.totals[k]
		out.Survival = append(out.Survival, float64(m.together[k])/float64(m.totals[k]))
	}

	out.Censored = true
	for k := 1; k < len(out.Survival); k++ {
		if out.Survival[k] > 0.5 {
			continue
		}
		// Where the segment from the previous reading crosses a half.
		prev, cur := out.Survival[k-1], out.Survival[k]
		t := 1.0
		if prev > cur {
			t = (prev - 0.5) / (prev - cur)
		}
		out.HalfLife = (float64(k-1) + t) * float64(m.step)
		out.Censored = false
		break
	}
	if math.IsNaN(out.HalfLife) {
		out.HalfLife, out.Censored = 0, true
	}
	return out
}
