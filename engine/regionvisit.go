package engine

// How much of the world one body sees in a lifetime (stage 57).
//
// The question this answers is the one that has to be asked before any rule
// claiming to make somewhere worth staying in: how much staying is there
// already? A world where a body spends its life in one block has nothing for
// such a rule to improve, and one where every body walks the whole map has a
// long way to go.
//
// It is a measurement and writes nothing. The world has no idea it is being
// watched, and none of the numbers here reach an agent - which region a body
// is in is not something the body is ever told (region.go).

// DefaultVisitStep is how often the tracker looks, and DefaultVisitSamples how
// many looks a body has to have been seen for before it counts.
//
// The second is the awkward one. A body seen once has stood in exactly one
// region, and counting it would say "most of them never leave" about nothing
// but the sampling. So the report covers bodies that were watched for at least
// DefaultVisitSamples * DefaultVisitStep ticks, and short lives are left out
// of it rather than counted as settled.
const (
	DefaultVisitStep    = 50
	DefaultVisitSamples = 10
)

// RegionVisitTracker follows where each body has stood.
type RegionVisitTracker struct {
	minSamples int
	seen       map[int]*visits
}

type visits struct {
	samples int
	blocks  []int // region indices, in the order first stood in
}

// NewRegionVisitTracker returns a tracker that counts a body once it has been
// seen at least minSamples times.
func NewRegionVisitTracker(minSamples int) *RegionVisitTracker {
	return &RegionVisitTracker{minSamples: max(minSamples, 1), seen: map[int]*visits{}}
}

// Observe records where everybody is standing now. Call it every
// DefaultVisitStep ticks.
func (t *RegionVisitTracker) Observe(w *World) {
	for _, a := range w.Agents() {
		if a.Species != SpeciesHuman {
			continue
		}
		v := t.seen[a.ID]
		if v == nil {
			v = &visits{}
			t.seen[a.ID] = v
		}
		v.samples++
		r := w.regionIndexAt(a.X, a.Y)
		found := false
		for _, b := range v.blocks {
			if b == r {
				found = true
				break
			}
		}
		if !found {
			v.blocks = append(v.blocks, r)
		}
	}
}

// RegionVisits is what the watching came to.
type RegionVisits struct {
	// Mean is how many different regions a body stood in, averaged over the
	// bodies that were watched long enough to have an answer.
	Mean float64

	// Alone is the share of those that only ever stood in one - the figure a
	// rule about staying put is trying to move.
	Alone float64

	// Share is how much of the world one body covers: Mean over the number of
	// regions there are, so that a run with a finer division can be read
	// against a coarser one.
	Share float64

	// Bodies is how many were counted, which says how much the two figures
	// above are worth.
	Bodies int
}

// Result reports what has been seen so far. Read only, and it can be called at
// any time.
func (t *RegionVisitTracker) Result(w *World) RegionVisits {
	var out RegionVisits
	var sum, alone float64
	for _, v := range t.seen {
		if v.samples < t.minSamples {
			continue
		}
		out.Bodies++
		sum += float64(len(v.blocks))
		if len(v.blocks) <= 1 {
			alone++
		}
	}
	if out.Bodies == 0 {
		return out
	}
	out.Mean = sum / float64(out.Bodies)
	out.Alone = alone / float64(out.Bodies)
	if n := len(w.regions); n > 0 {
		out.Share = out.Mean / float64(n)
	}
	return out
}
