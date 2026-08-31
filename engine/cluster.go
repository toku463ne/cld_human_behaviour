package engine

import "sort"

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
