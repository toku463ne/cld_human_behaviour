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
