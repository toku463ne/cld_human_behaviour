package main

import (
	"math"
	"testing"

	"github.com/toku463ne/cld_human_behaviour/engine"
)

// A pile of bodies standing on the same spot comes apart to be drawn, and
// none of them is drawn far from where it is.
//
// The two halves are the whole bargain. Without the first the crowd is a
// smear; without the second the picture stops being about the world, which is
// worse than a smear - a body drawn five units off is standing on ground its
// neighbour is standing on too, and one drawn fifty units off is somewhere
// nobody is.
func TestACrowdIsDrawnApartButNotFarFromItself(t *testing.T) {
	agents := make([]engine.Agent, 24)
	for i := range agents {
		// All of them within a body's width of one spot, and four of them on
		// exactly the same point: a newborn starts where its mother stands,
		// so this is not a contrived case.
		agents[i] = engine.Agent{ID: i + 1, X: 400, Y: 300}
		if i >= 4 {
			agents[i].X += float64(i%5) * 2
			agents[i].Y += float64(i/5) * 2
		}
	}
	g := &game{spread: true}
	g.spreadCrowd(agents)

	at := func(i int) (float64, float64) {
		n := g.nudge[agents[i].ID]
		return agents[i].X + n[0], agents[i].Y + n[1]
	}
	before, after := math.MaxFloat64, math.MaxFloat64
	for i := range agents {
		if d := math.Hypot(g.nudge[agents[i].ID][0], g.nudge[agents[i].ID][1]); d > crowdNudge+1e-9 {
			t.Fatalf("#%d is drawn %.2f from itself, and the most allowed is %.2f",
				agents[i].ID, d, crowdNudge)
		}
		for j := i + 1; j < len(agents); j++ {
			before = math.Min(before, math.Hypot(agents[i].X-agents[j].X, agents[i].Y-agents[j].Y))
			xi, yi := at(i)
			xj, yj := at(j)
			after = math.Min(after, math.Hypot(xi-xj, yi-yj))
		}
	}
	if after <= before {
		t.Fatalf("the closest pair was %.2f apart and is drawn %.2f apart", before, after)
	}
}

// And with the spreading off, nobody is drawn anywhere but where they are.
//
// This is what -huddle is for, and the test is here because the flag is the
// only way to check a drawing rule against the truth it is bending.
func TestHuddlingDrawsEverybodyWhereItReallyIs(t *testing.T) {
	agents := []engine.Agent{
		{ID: 1, X: 400, Y: 300},
		{ID: 2, X: 400, Y: 300},
		{ID: 3, X: 401, Y: 301},
	}
	g := &game{spread: false}
	g.spreadCrowd(agents)
	if len(g.nudge) != 0 {
		t.Fatalf("%d bodies were moved with the spreading off", len(g.nudge))
	}
	for i := range agents {
		if x, y := g.drawnWorldAt(&agents[i]); x != agents[i].X || y != agents[i].Y {
			t.Fatalf("#%d is at %.2f,%.2f and drawn at %.2f,%.2f",
				agents[i].ID, agents[i].X, agents[i].Y, x, y)
		}
	}
}

// The same world gives the same picture twice running.
//
// Bodies exactly on top of one another have no line to part along, so a
// direction is taken from their IDs rather than drawn at random. If that ever
// becomes a random draw the crowd will shiver every frame, which is the one
// way this could be worse than leaving them in a heap.
func TestTheSpreadIsSteadyFromFrameToFrame(t *testing.T) {
	agents := []engine.Agent{
		{ID: 7, X: 400, Y: 300},
		{ID: 9, X: 400, Y: 300},
		{ID: 11, X: 400, Y: 300},
	}
	g := &game{spread: true}
	g.spreadCrowd(agents)
	first := map[int][2]float64{}
	for k, v := range g.nudge {
		first[k] = v
	}
	g.spreadCrowd(agents)
	for k, v := range first {
		if g.nudge[k] != v {
			t.Fatalf("#%d was drawn at %v and then at %v", k, v, g.nudge[k])
		}
	}
	if len(first) != len(agents) {
		t.Fatalf("%d of %d bodies came apart", len(first), len(agents))
	}
}
