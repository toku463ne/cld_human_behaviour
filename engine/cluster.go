package engine

import (
	"math"
	"sort"
)

// This file holds the group structure of the population: how many clumps of
// agents there are and how big they are. It is the first of the "should be"
// measurements (PLAN.md stage 6.5), and like Spacing it is read only: nothing
// here feeds back into a decision, and no agent carries a cluster id. Grouping
// has to be visible as a number before any rule can be credited with causing
// it.
//
// Clusters are single linkage: two agents are in the same cluster when they are
// within LinkDist of each other, and that relation is transitive. PLAN.md calls
// for connected components on the grid; until stage 7a brings a grid in, the
// naive pair scan below stands in for it and gives the same answer.

// DefaultClusterLinkDist is the linking distance the experiment runner uses.
//
// It is not in Config because Config is the list of the rules of the world, and
// this changes nothing about how the world behaves; it is a knob on the ruler,
// not on the thing being measured. It is a package constant so that every run
// is measured against the same ruler: a cluster count taken at one linking
// distance is not comparable with one taken at another.
//
// 30 is twice CombatRadius, so two agents count as linked when they are close
// enough to be within reach of each other, and it is far below PerceptionRadius
// (130). Linking everybody who can merely see each other is useless: single
// linkage percolates long before that. Measured over the last 10000 ticks of
// eight 50000 tick runs of the defaults, at a population around 144:
//
//	link   groups  avgSize  grouped  largest
//	  15    24.05     2.71    0.450    0.046
//	  30    25.65     4.42    0.774    0.115
//	  40    20.43     6.43    0.880    0.196
//	  60     9.72    15.19    0.967    0.407
//	 130     1.24   129.51    0.999    0.984
//
// At 130 the whole population is one component. 30 sits near the peak of the
// group count with the largest component still holding only about a tenth of
// the population, which leaves room for real grouping to show up as a rise.
//
// Like Spacing.Clumping, the counts depend on how many agents there are: the
// same layout in a denser world links up more. Compare runs of the same world.
const DefaultClusterLinkDist = 30

// Clustering is the group structure of the population at one moment.
type Clustering struct {
	// LinkDist is the distance the clusters were built with. A count is only
	// comparable with another count taken at the same distance.
	LinkDist float64

	// Sizes holds every connected component, largest first. It sums to the
	// population, so it is the size distribution in full; the fields below are
	// the summaries of it that the experiment runner prints.
	Sizes []int

	// Labels says which component each agent belongs to, as an index into
	// Sizes, in the order Agents returns them. It is what a viewer needs to
	// colour the population by cluster; the engine itself never reads it, and
	// no agent carries a cluster id of its own.
	Labels []int

	// Groups counts the components with two or more members. A lone agent is
	// not a group, so singletons are counted separately rather than inflating
	// this.
	Groups     int
	Singletons int

	// Largest is the size of the biggest component and LargestShare is that as
	// a fraction of the population. The share is the one to watch: single
	// linkage chains, so a share near 1 means the population has percolated
	// into one blob and the count below it says nothing about grouping.
	Largest      int
	LargestShare float64

	// AvgGroupSize is the mean size of the components counted in Groups, and
	// GroupedShare is the fraction of the population that is in one of them.
	AvgGroupSize float64
	GroupedShare float64
}

// Clusters measures the group structure at the given linking distance. It is
// O(n^2) in the population, like Spacing, and is meant to be sampled every so
// often rather than every tick.
func (w *World) Clusters(linkDist float64) Clustering {
	c := Clustering{LinkDist: linkDist}
	n := len(w.agents)
	if n == 0 {
		return c
	}

	// Union-find over the agents, joining every pair within linkDist.
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]] // path halving
			i = parent[i]
		}
		return i
	}
	link2 := linkDist * linkDist
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if find(i) == find(j) {
				continue
			}
			if dist2(w.agents[i].X, w.agents[i].Y, w.agents[j].X, w.agents[j].Y) <= link2 {
				parent[find(i)] = find(j)
			}
		}
	}

	counts := make(map[int]int, n)
	for i := 0; i < n; i++ {
		counts[find(i)]++
	}
	// Order the components largest first, breaking ties by the lowest member so
	// that the labels of a given layout do not depend on map iteration order.
	roots := make([]int, 0, len(counts))
	for root := range counts {
		roots = append(roots, root)
	}
	sort.Slice(roots, func(i, j int) bool {
		if counts[roots[i]] != counts[roots[j]] {
			return counts[roots[i]] > counts[roots[j]]
		}
		return roots[i] < roots[j]
	})
	c.Sizes = make([]int, len(roots))
	rank := make(map[int]int, len(roots))
	for k, root := range roots {
		c.Sizes[k] = counts[root]
		rank[root] = k
	}
	c.Labels = make([]int, n)
	for i := 0; i < n; i++ {
		c.Labels[i] = rank[find(i)]
	}

	grouped := 0
	for _, size := range c.Sizes {
		if size == 1 {
			c.Singletons++
			continue
		}
		c.Groups++
		grouped += size
	}
	c.Largest = c.Sizes[0]
	c.LargestShare = float64(c.Largest) / float64(n)
	c.GroupedShare = float64(grouped) / float64(n)
	if c.Groups > 0 {
		c.AvgGroupSize = float64(grouped) / float64(c.Groups)
	}
	return c
}

// ClusterGaps is how far the groups keep from each other.
//
// It is the distance part of stage 6.5's last layout question: whether nodes of
// different groups either fight or stay away from each other. The fighting half
// is in fights.go; this is the staying away half.
//
// Singletons take no part, neither as a group nor as something to be far from.
// A lone agent is not a group, and counting one would turn "how far apart are
// the groups" into "how empty is the world between them".
type ClusterGaps struct {
	LinkDist float64

	// Gaps holds, for each group, the distance to the nearest agent of any
	// other group, in ascending order: the distribution in full. Every entry
	// is greater than LinkDist, because two agents any closer than that would
	// be one group by definition. The interesting end is the low one, where
	// groups are approaching each other.
	Gaps []float64

	Mean, Median, P10 float64

	// Relative is Mean divided by the nearest neighbour distance the same
	// number of groups would have if they were scattered at random over the
	// world, which takes out the fact that a crowded world has everything
	// closer to everything else.
	//
	// Unlike Spacing.Clumping this one does read against 1: a layout with no
	// structure comes out at 0.98 (TestClusterGapsRelativeOfARandomLayout), so
	// below 1 means the groups really are sitting closer together than chance
	// and above 1 means they are keeping apart. The reference treats a group
	// as a point while a gap is measured edge to edge, which is harmless while
	// the groups are small next to the gaps between them and would stop being
	// so if they grew.
	Relative float64
}

// ClusterGaps measures how far each group is from the nearest other one. Like
// Clusters it is O(n^2) and meant to be sampled rather than run every tick.
func (w *World) ClusterGaps(linkDist float64) ClusterGaps {
	out := ClusterGaps{LinkDist: linkDist}
	c := w.Clusters(linkDist)
	if len(c.Sizes) < 2 {
		return out
	}

	nearest := make([]float64, len(c.Sizes))
	for i := range nearest {
		nearest[i] = math.Inf(1)
	}
	inGroup := func(label int) bool { return c.Sizes[label] > 1 }

	for i := range w.agents {
		if !inGroup(c.Labels[i]) {
			continue
		}
		for j := i + 1; j < len(w.agents); j++ {
			if c.Labels[i] == c.Labels[j] || !inGroup(c.Labels[j]) {
				continue
			}
			d2 := dist2(w.agents[i].X, w.agents[i].Y, w.agents[j].X, w.agents[j].Y)
			if d2 < nearest[c.Labels[i]] {
				nearest[c.Labels[i]] = d2
			}
			if d2 < nearest[c.Labels[j]] {
				nearest[c.Labels[j]] = d2
			}
		}
	}

	for label, d2 := range nearest {
		if !inGroup(label) || math.IsInf(d2, 1) {
			continue
		}
		out.Gaps = append(out.Gaps, math.Sqrt(d2))
	}
	if len(out.Gaps) == 0 {
		return out
	}
	sort.Float64s(out.Gaps)

	var sum float64
	for _, g := range out.Gaps {
		sum += g
	}
	out.Mean = sum / float64(len(out.Gaps))
	out.Median = quantile(out.Gaps, 0.5)
	out.P10 = quantile(out.Gaps, 0.1)

	// What the mean would be if this many groups were scattered evenly: the
	// expected nearest neighbour distance of k random points in an area is
	// half the square root of the area per point.
	if area := w.cfg.Width * w.cfg.Height; area > 0 {
		if expected := 0.5 * math.Sqrt(area/float64(len(out.Gaps))); expected > 0 {
			out.Relative = out.Mean / expected
		}
	}
	return out
}

// quantile reads a fraction of the way into an ascending slice, interpolating
// between the two entries it falls between.
func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	pos := q * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	return sorted[lo] + (sorted[hi]-sorted[lo])*(pos-float64(lo))
}
