package main

import "github.com/toku463ne/cld_human_behaviour/engine"

// Who wears the player's colour (2026-09-22, the user's ask).
//
// The played body, and after it one body per generation: the eldest living
// child, then that one's eldest living child, and so on. The second child does
// not wear it while the first is alive, and takes it the moment the first
// dies - which is the whole point of computing it fresh rather than handing it
// out at birth. PLAN.md learned this once already and the note is still in
// dynasty.go: who the heir is changes the moment somebody dies, and a birth is
// nowhere near that moment, so it has to be asked and not stored.
//
// None of it is in the engine and none of it touches a decision. It is a
// colour: the rule reads ChildIDs, which the world has recorded since the
// blood links went in, and writes nothing.

// successionDepth is how far down the line the colour is followed. A line
// deeper than this in one run would be remarkable; the bound is here so that
// a cycle in the links could never hang the viewer.
const successionDepth = 64

// carriesMyColour reports whether this body is the played one or stands in the
// line of succession from it.
func (g *game) carriesMyColour(id int) bool {
	if g.played == 0 {
		return false
	}
	if id == g.played {
		return true
	}
	if g.world == nil {
		return false
	}
	// Worked out once a tick. The world only changes when it steps, so this
	// is exact rather than a staleness anybody could see - and it is not free
	// enough to do once per body per frame.
	if g.heirsAt != g.world.Tick() || g.heirsFrom != g.played || g.heirs == nil {
		g.heirs = g.successionFrom(g.played)
		g.heirsAt, g.heirsFrom = g.world.Tick(), g.played
	}
	return g.heirs[id]
}

// lineReader is the whole of what working out a succession needs: who is
// alive, and who their children are. It is an interface so that the rule can
// be pinned against a family built by hand - a world where somebody has three
// children and a grandchild is a slow thing to grow on purpose.
type lineReader interface {
	AgentByID(id int) (engine.Agent, bool)
	Agents() []engine.Agent
}

// successionFrom walks the line: one heir a generation, for as long as there
// is one.
func (g *game) successionFrom(id int) map[int]bool { return successionIn(g.world, id) }

func successionIn(w lineReader, id int) map[int]bool {
	out := map[int]bool{}
	for at, depth := id, 0; depth < successionDepth; depth++ {
		next := heirIn(w, at)
		if next == 0 || out[next] {
			break
		}
		out[next] = true
		at = next
	}
	return out
}

// heirOf is the eldest living child, and when none of them is alive, the
// eldest living descendant of the eldest child that had any.
//
// Living brothers and sisters come before the next generation, in the order
// they were born: three sons, the eldest dies and the second is the heir, he
// dies and the third is, and only when the last of them is gone does it pass
// to a child. Birth order is ID order - the world hands them out in turn - so
// no age has to be compared.
func heirIn(w lineReader, id int) int {
	a, alive := w.AgentByID(id)
	if !alive {
		return 0
	}
	kids := append([]int(nil), a.ChildIDs...)
	sortInts(kids)
	for _, kid := range kids {
		if _, ok := w.AgentByID(kid); ok {
			return kid
		}
	}
	// Nobody of that generation is left, so the eldest line that still has
	// anybody carries it on.
	for _, kid := range kids {
		if heir := heirOfDeadIn(w, kid, successionDepth); heir != 0 {
			return heir
		}
	}
	return 0
}

// heirOfDead looks past a child that is gone, for whoever it left.
//
// The world keeps no record of the dead, so their children are unreachable
// from out here: a body that is gone takes its ChildIDs with it. What is left
// is the living bodies' own parent links, and those are searched instead - the
// eldest living body whose line runs back through this one.
func heirOfDeadIn(w lineReader, id, depth int) int {
	if depth <= 0 {
		return 0
	}
	best := 0
	for _, a := range w.Agents() {
		if !a.Alive {
			continue
		}
		if best != 0 && a.ID > best {
			continue
		}
		for _, p := range a.ParentIDs {
			if p == id {
				best = a.ID
				break
			}
		}
	}
	return best
}

// sortInts is an insertion sort, because the lists here are a handful of
// children and pulling in a package for that would be the longer line.
func sortInts(v []int) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}
