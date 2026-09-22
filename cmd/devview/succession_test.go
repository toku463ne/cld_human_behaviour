package main

import (
	"image/color"
	"testing"

	"github.com/toku463ne/cld_human_behaviour/engine"
)

// A family is one colour, and the played line is white (2026-09-22).

func light(c color.RGBA) float64 {
	return 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
}

func TestAChildIsDrawnInItsFamilysColour(t *testing.T) {
	g := &game{}
	mother := &engine.Agent{ID: 1, Lineage: 3, Genome: make([]float64, engine.NumGenes)}
	child := &engine.Agent{ID: 2, Lineage: 3, Genome: make([]float64, engine.NumGenes)}
	// Bodies of the same family are drawn alike however differently they are
	// built: the colour says who they are, not what they can do.
	for i := range child.Genome {
		mother.Genome[i], child.Genome[i] = 40, 40
	}
	child.Genome[engine.GeneAttack] = 200
	if a, b := g.bodyPaint(mother, false), g.bodyPaint(child, false); a != b {
		t.Fatalf("a mother is painted %v and her child %v", a, b)
	}
	other := &engine.Agent{ID: 3, Lineage: 4, Genome: mother.Genome}
	if a, b := g.bodyPaint(mother, false), g.bodyPaint(other, false); a == b {
		t.Fatalf("two families are both painted %v", a)
	}
}

func TestEveryFamilyIsDrawnAtTheSameLightness(t *testing.T) {
	// Brightness answers one question - is this my line - and a family that
	// happened to be pale would answer it wrong.
	var first float64
	for tag := uint16(1); tag < 500; tag++ {
		c, ok := lineColour(tag)
		if !ok {
			t.Fatalf("family %d has no colour", tag)
		}
		if l := light(c); tag == 1 {
			first = l
		} else if l < first-2 || l > first+2 {
			t.Fatalf("family %d is drawn at %.0f and family 1 at %.0f", tag, l, first)
		}
		if light(c) >= light(colorOwnLine)*0.8 {
			t.Fatalf("family %d is painted %v, too close to the played line", tag, c)
		}
	}
	if _, ok := lineColour(0); ok {
		t.Fatal("a body with no family was given a family's colour")
	}
}

func TestNeighbouringFamiliesAreNotNeighbouringColours(t *testing.T) {
	near := 0
	for tag := uint16(1); tag < 60; tag++ {
		a, _ := lineColour(tag)
		b, _ := lineColour(tag + 1)
		d := gap(int(a.R), int(b.R)) + gap(int(a.G), int(b.G)) + gap(int(a.B), int(b.B))
		if d < 20 {
			near++
		}
	}
	if near > 2 {
		t.Fatalf("%d of the first sixty families are the same colour as the next one", near)
	}
}

func gap(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}

// family is a hand-built stand-in for a world: who is alive and whose child
// each one is. Growing a body with three children and a grandchild inside a
// real world takes thousands of ticks and gives no control over the order
// they arrived in, which is the whole of what the rule turns on.
type family struct {
	kids map[int][]int
	dead map[int]bool
}

func (f *family) AgentByID(id int) (engine.Agent, bool) {
	if f.dead[id] {
		return engine.Agent{}, false
	}
	if _, ok := f.kids[id]; !ok {
		return engine.Agent{}, false
	}
	return engine.Agent{ID: id, Alive: true, ChildIDs: f.kids[id], ParentIDs: f.parentsOf(id)}, true
}

func (f *family) parentsOf(id int) [2]int {
	for parent, kids := range f.kids {
		for _, kid := range kids {
			if kid == id {
				return [2]int{parent}
			}
		}
	}
	return [2]int{}
}

func (f *family) Agents() []engine.Agent {
	var out []engine.Agent
	for id := range f.kids {
		if a, ok := f.AgentByID(id); ok {
			out = append(out, a)
		}
	}
	return out
}

// #1 is the player, #2 #3 #4 its children in birth order, and #5 a child of #2.
func lineFamily() *family {
	return &family{
		kids: map[int][]int{1: {2, 3, 4}, 2: {5}, 3: nil, 4: nil, 5: nil},
		dead: map[int]bool{},
	}
}

func wears(f *family, played, id int) bool {
	if id == played {
		return true
	}
	return successionIn(f, played)[id]
}

func TestOnlyTheEldestChildWearsThePlayersColour(t *testing.T) {
	f := lineFamily()
	if !wears(f, 1, 2) {
		t.Fatal("the eldest child does not wear it")
	}
	for _, id := range []int{3, 4} {
		if wears(f, 1, id) {
			t.Fatalf("#%d wears it while its elder is alive", id)
		}
	}
	// One a generation: the eldest child's own eldest child carries it on.
	if !wears(f, 1, 5) {
		t.Fatal("the eldest child's child does not wear it")
	}
}

func TestTheColourPassesTheMomentTheEldestDies(t *testing.T) {
	f := lineFamily()
	f.dead[2] = true
	if wears(f, 1, 2) {
		t.Fatal("a body that is gone still wears it")
	}
	if !wears(f, 1, 3) {
		t.Fatal("the second child did not take it when the first died")
	}
	if wears(f, 1, 4) {
		t.Fatal("the third took it while the second is alive")
	}
	// The dead eldest's own child does not jump the queue: living brothers
	// and sisters come first.
	if wears(f, 1, 5) {
		t.Fatal("a grandchild took it while an uncle is alive")
	}
}

func TestWhenNoChildIsLeftItPassesToTheNextGeneration(t *testing.T) {
	f := lineFamily()
	f.dead[2], f.dead[3], f.dead[4] = true, true, true
	if !wears(f, 1, 5) {
		t.Fatal("with every child gone, the grandchild did not take it")
	}
}

func TestThePlayedBodyAlwaysWearsIt(t *testing.T) {
	g := &game{played: 7}
	if !g.carriesMyColour(7) {
		t.Fatal("the played body does not wear its own colour")
	}
	none := &game{}
	if none.carriesMyColour(7) {
		t.Fatal("a world with nobody played has somebody wearing the player's colour")
	}
}

func TestTheBudgetColourIsStillThere(t *testing.T) {
	// -paint budget puts back what the colour meant until 2026-09-22: where
	// this body's budget went.
	body := &engine.Agent{ID: 9, Lineage: 3, Genome: make([]float64, engine.NumGenes)}
	for i := range body.Genome {
		body.Genome[i] = 40
	}
	byLine := (&game{}).bodyPaint(body, false)
	byBudget := (&game{paintBy: paintByBudget}).bodyPaint(body, false)
	if byLine == byBudget {
		t.Fatalf("both rules paint this body %v", byLine)
	}
	if byBudget != strangerColour(body) {
		t.Fatalf("-paint budget gives %v, and the budget reading is %v", byBudget, strangerColour(body))
	}
}
