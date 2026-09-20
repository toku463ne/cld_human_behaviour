package engine

// Lines of descent (2026-09-20).
//
// Every founder starts a line and every child takes its mother's, so a tag
// says which of the world's original families a body comes from. Nothing in
// the engine reads it. It is here because the game being built towards asks
// whether one line is living in several places at once (PLAN.md, decision
// #130), and a tag that was not laid down from the first tick could never be
// worked out afterwards.
//
// It is a fact about the body and not about who is playing. Stage 19 put the
// player outside the engine and this keeps that: the tags are there in a run
// nobody is watching, and what counts as "my people" is the game's question.
//
// Measured and never enforced: nothing groups by tag, nobody prefers their
// own line, and a line is not a species. Two lines meet and breed exactly as
// any two bodies do, and their child belongs to one of them.

// LineageRule is how a line is handed down.
type LineageRule int

const (
	// LineageTree: every child of a carrier carries it, so a line is the
	// whole tree of descent. The default, and every world before there was
	// a choice.
	//
	// Measured: 4.67 lines survive 20,000 ticks out of 70, the largest holds
	// 0.55 of the living, and 0.92 of them stand in two regions at once. So
	// "my people live in several places" happens without anybody trying.
	LineageTree LineageRule = iota

	// LineageChain: one carrier at a time. A newborn takes its mother's line
	// only when no living child of hers carries it - the eldest has it, and
	// when the eldest dies the next one born takes it up.
	//
	// Measured: 0.25 lines survive out of 70 - nine seeds in twelve lose the
	// line altogether - and 0.00 of them ever stand in two regions, because
	// the carriers are a parent and a child and a parent and a child stand
	// together. A line handed to one heir is a critical branching process,
	// and those die out with probability one.
	LineageChain

	// LineageRegionChain: one carrier at a time in each region. A newborn
	// takes its mother's line when no living carrier of it is in the block
	// the child is born in.
	//
	// It is the rule a travelling dynasty asks for: going somewhere new and
	// leaving descendants there is what makes a carrier there, and a line
	// standing in three places means three heirs who were left in three
	// places.
	LineageRegionChain
)

// LineageUse is what the lines of descent are doing. Read only.
type LineageUse struct {
	// Living is how many of the founding lines still have somebody alive,
	// and Biggest is the largest one's share of the living population.
	Living  int
	Biggest float64

	// Regions is how many of the world's blocks the mean surviving line
	// stands in at this moment, and InTwo and InThree are the shares of
	// lines standing in two and in three at once.
	//
	// Those two are the figure the dynasty is about: whether a line being in
	// several places at once is something that happens on its own, and how
	// often, before anything is built to ask a player for it.
	Regions float64
	InTwo   float64
	InThree float64
}

// Lineages reports what the lines of descent are doing. It draws nothing from
// the random source and writes nothing, the same standing every other
// measuring instrument in this package has.
func (w *World) Lineages() LineageUse {
	var out LineageUse
	living := 0.0
	count := map[uint16]float64{}
	where := map[uint16]map[int]bool{}
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive || a.Lineage == 0 {
			continue
		}
		living++
		count[a.Lineage]++
		if len(w.regions) > 0 {
			r := w.regionIndexAt(a.X, a.Y)
			if where[a.Lineage] == nil {
				where[a.Lineage] = map[int]bool{}
			}
			where[a.Lineage][r] = true
		}
	}
	if living == 0 {
		return out
	}
	out.Living = len(count)
	regions, two, three := 0.0, 0.0, 0.0
	for tag, n := range count {
		if share := n / living; share > out.Biggest {
			out.Biggest = share
		}
		k := float64(len(where[tag]))
		regions += k
		if k >= 2 {
			two++
		}
		if k >= 3 {
			three++
		}
	}
	lines := float64(len(count))
	out.Regions = regions / lines
	out.InTwo, out.InThree = two/lines, three/lines
	return out
}

// lineTakenBy says whether the mother's line already has somebody to carry
// it, and so whether this newborn takes it up.
//
// Nothing keeps an "heir" field, in either rule. The answer has to change the
// moment a carrier dies, and a field would have to be kept up by every path a
// body can leave the world by - which is the kind of bookkeeping that is
// right until the day it is not. Both of these are walks, asked once a birth.
func (w *World) lineTaken(mother *Agent, atX, atY float64) bool {
	if mother.Lineage == 0 {
		return false
	}
	switch w.cfg.LineageRule {
	case LineageChain:
		// One at a time in the whole world: does a living child of hers have
		// it already?
		for _, id := range mother.ChildIDs {
			if c := w.agentByID(id); c != nil && c.Alive && c.Lineage == mother.Lineage {
				return true
			}
		}
		return false
	case LineageRegionChain:
		// One at a time in each block: is anybody of this line standing in
		// the block this child is being born in? Anybody, not only her own
		// children - a line's heir in a country is whoever of that line is
		// there, which is what makes moving somewhere and leaving descendants
		// the way to have one there.
		if len(w.regions) == 0 {
			return false
		}
		here := w.regionIndexAt(atX, atY)
		for i := range w.agents {
			a := &w.agents[i]
			// Not the mother herself: she is standing where the birth is by
			// construction, so counting her would mean no child ever takes a
			// line up. What is being asked is whether she is leaving an heir
			// in this country or one is already here.
			if a.ID == mother.ID || !a.Alive || a.Lineage != mother.Lineage {
				continue
			}
			if w.regionIndexAt(a.X, a.Y) == here {
				return true
			}
		}
		return false
	}
	return false
}
