package engine

import "fmt"

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

	// There was a third, LineageRegionChain: one carrier at a time in each
	// block, taken up by a newborn when no carrier of that line was standing
	// there. It was written for the travelling dynasty and it was deleted on
	// 2026-09-21, because it was not what that game wanted and could not have
	// been.
	//
	// What the game wants is "who is the heir in this country now", and that
	// changes the moment somebody dies, with no birth anywhere near it. A tag
	// is written once, at birth, so it cannot say it: under that rule a
	// second son born while his elder brother stood there was never a carrier
	// and never could become one, and a block whose carrier died stayed empty
	// until the next child happened to be born in it. Measured on the played
	// map, a line held 2.09 blocks of twelve against the tree's 9.25, and one
	// ten-year gap took it to 0.91 with the line gone altogether 0.55 of the
	// time.
	//
	// The thing it was trying to be is LineHeirs below - a question asked of
	// the world rather than a mark left on a body. See docs/history/decision.
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

// LineHeirs is the eldest living carrier of a line in each block of the
// world: the answer to "where in the world does this line still have
// somebody", one body per block.
//
// It is a fact about bodies and about nothing else - who is playing, what
// counts as a win, and how long a player waits after a death are all the
// game's, and stage 19's line is that the engine does not know any of them.
// What this is for is the menu the game puts up, and the count that said how
// often that menu would be empty.
//
// The eldest rather than the nearest or the healthiest, because a line's heir
// in a country is the one who has been there longest; and one per block
// rather than all of them, because the choice being offered is which country
// to carry on in, not which body.
//
// Read only, and it draws nothing from the random source.
func (w *World) LineHeirs(line uint16) map[int]int {
	if line == 0 || len(w.regions) == 0 {
		return nil
	}
	out := map[int]int{}
	age := map[int]int{}
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive || a.Lineage != line {
			continue
		}
		r := w.regionIndexAt(a.X, a.Y)
		if _, held := out[r]; !held || a.Age > age[r] {
			out[r], age[r] = a.ID, a.Age
		}
	}
	return out
}

// lineTakenBy says whether the mother's line already has somebody to carry
// it, and so whether this newborn takes it up.
//
// Nothing keeps an "heir" field. The answer has to change the moment a
// carrier dies, and a field would have to be kept up by every path a body can
// leave the world by - which is the kind of bookkeeping that is right until
// the day it is not. This is a walk, asked once a birth.
func (w *World) lineTaken(mother *Agent) bool {
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
	}
	return false
}

// SetLineage puts a body into a line of descent, and is the second thing in
// this engine that only something outside it ever calls (the first is Endow).
//
// It exists for one thing the dynasty needs and the engine has no opinion
// about. A child takes its mother's line, which is how a family is carried
// here and is right for a world nobody is playing. A player who is male and
// has a child with somebody of another country is therefore, by the tag,
// childless: the child belongs to its mother's house. The game says otherwise
// - what the player is playing for is their own house - and this is where the
// game says it.
//
// No rule reads it back, nothing is drawn, and a world nobody plays never
// calls it. What it costs is that the tag stops being "the line you were born
// into" and becomes "the house you belong to", which is what the game meant
// by it all along.
func (w *World) SetLineage(id int, line uint16) error {
	a := w.agentByID(id)
	if a == nil || !a.Alive {
		return fmt.Errorf("lineage: no body %d", id)
	}
	a.Lineage = line
	return nil
}
