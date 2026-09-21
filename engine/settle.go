package engine

// Settling down (2026-09-20).
//
// Why this is not "standing there". The dynasty being built towards asks
// whether one line of descent is living in several places at once (PLAN.md,
// decision #130), and the first thing measured about that was that standing
// in several places happens by itself: 0.92 of the surviving lines stand in
// two of the twelve regions at any moment, 0.85 in three. A win condition
// written on where bodies are standing is met by a player who does nothing.
//
// So settled has to mean more, and this is what it means here: a body is
// settled in a block when most of its recent past has been spent in that
// block. It is the cheapest of the three candidates the plan listed - the
// share of time, the birthplace, and the persistence of the faces around it -
// and it is the one that says something about now. A birthplace never
// changes, so it cannot tell a line that moved from one that did not; the
// faces are already measured by the membership tracker and answer a different
// question, which is whether a group is a group.
//
// What it deliberately is not. It is not a rule: nothing in the world reads
// it, no agent knows whether it is settled, and no utility term mentions it.
// It is an instrument, with the standing every other instrument here has -
// read only, no writes, and not one number taken from the random source.

const (
	// DefaultSettleStep is how often to take a reading, and
	// DefaultSettleWindow how many readings make up "recently". Twenty-five
	// ticks matches the membership tracker's cadence; forty readings is a
	// thousand ticks, which is two years of world time and long enough that
	// crossing a region on the way somewhere does not count as living there.
	DefaultSettleStep   = 25
	DefaultSettleWindow = 40

	// DefaultSettleShare is how much of that window has to have been spent in
	// one block for a body to be settled in it. Above a half, so that a body
	// is settled in at most one place at a time and the word keeps its
	// meaning.
	DefaultSettleShare = 0.6
)

// SettlementUse is what the lines of descent have settled into. Read only.
type SettlementUse struct {
	// Settled is the share of the living that are settled anywhere at all,
	// which says how much of this world's motion is wandering.
	Settled float64

	// Lines is how many lines of descent have at least one settled body,
	// Regions the mean number of blocks such a line is settled in, and InTwo
	// and InThree the shares settled in two and in three blocks at once.
	//
	// InTwo and InThree beside Lineages()' own InTwo and InThree are the
	// whole point: the difference between the pairs is the distance between
	// "is there" and "lives there".
	Lines   int
	Regions float64
	InTwo   float64
	InThree float64
}

// SettlementTracker watches where bodies spend their time.
//
// It keeps a ring of recent readings per body rather than a running tally,
// because the question is about the recent past and a tally would never let
// go of a country a body left a lifetime ago.
type SettlementTracker struct {
	window int
	share  float64

	// seen[id] is that body's last readings, newest last, as region indices.
	seen map[int][]int
}

// NewSettlementTracker makes one. Call Observe every DefaultSettleStep ticks.
func NewSettlementTracker(window int, share float64) *SettlementTracker {
	if window < 1 {
		window = 1
	}
	return &SettlementTracker{window: window, share: share, seen: map[int][]int{}}
}

// Observe takes one reading of where everybody is.
func (t *SettlementTracker) Observe(w *World) {
	if len(w.regions) == 0 {
		return
	}
	alive := make(map[int]bool, len(w.agents))
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		alive[a.ID] = true
		r := w.regionIndexAt(a.X, a.Y)
		s := append(t.seen[a.ID], r)
		if len(s) > t.window {
			s = s[len(s)-t.window:]
		}
		t.seen[a.ID] = s
	}
	// The dead are let go of, or this grows with every body that ever lived.
	for id := range t.seen {
		if !alive[id] {
			delete(t.seen, id)
		}
	}
}

// settledIn is the block this body has spent most of its recent past in, and
// false when no block holds enough of it.
//
// A body with less than a full window of readings is judged on what there is,
// so that a world is not silent for its first thousand ticks - a body that
// has been in one place for every reading it has is settled there.
func (t *SettlementTracker) settledIn(id int) (int, bool) {
	s := t.seen[id]
	if len(s) == 0 {
		return 0, false
	}
	count := map[int]int{}
	best, bestN := 0, 0
	for _, r := range s {
		count[r]++
		if count[r] > bestN {
			best, bestN = r, count[r]
		}
	}
	if float64(bestN)/float64(len(s)) < t.share {
		return 0, false
	}
	return best, true
}

// SettledIn is the block this body has spent most of its recent past in, and
// false when no block holds enough of it. Read only.
//
// It is exported for the game to ask about one body at a time: a win
// condition written on "my line lives in these places" needs to know which
// bodies of that line live where, and Result answers a different question
// (what all the lines are doing on average). The engine still does not know
// which line is anybody's, or that anything is being won.
func (t *SettlementTracker) SettledIn(id int) (int, bool) { return t.settledIn(id) }

// Result reads off what the lines have settled into.
func (t *SettlementTracker) Result(w *World) SettlementUse {
	var out SettlementUse
	living, settled := 0.0, 0.0
	where := map[uint16]map[int]bool{}
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		living++
		r, ok := t.settledIn(a.ID)
		if !ok {
			continue
		}
		settled++
		if a.Lineage == 0 {
			continue
		}
		if where[a.Lineage] == nil {
			where[a.Lineage] = map[int]bool{}
		}
		where[a.Lineage][r] = true
	}
	if living > 0 {
		out.Settled = settled / living
	}
	if len(where) == 0 {
		return out
	}
	out.Lines = len(where)
	regions, two, three := 0.0, 0.0, 0.0
	for _, rs := range where {
		k := float64(len(rs))
		regions += k
		if k >= 2 {
			two++
		}
		if k >= 3 {
			three++
		}
	}
	n := float64(len(where))
	out.Regions, out.InTwo, out.InThree = regions/n, two/n, three/n
	return out
}
