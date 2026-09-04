package engine

import (
	"math"
	"testing"
)

// The whole world is level ground today, and the point of the test is that
// changing that is a change to one function: if terrainAt ever starts varying,
// this is what says so.
func TestTheWholeWorldIsLevelGround(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	for x := 0.0; x <= cfg.Width; x += cfg.Width / 8 {
		for y := 0.0; y <= cfg.Height; y += cfg.Height / 8 {
			if got := w.terrainAt(x, y); got != flatGround {
				t.Fatalf("the ground at (%.0f, %.0f) is %+v, want %+v", x, y, got, flatGround)
			}
		}
	}
}

// Movement is charged through the ground, so that giving the world hills is a
// change in terrain.go and nowhere else. With level ground it comes to exactly
// what it always did.
func TestMovementIsChargedThroughTheGround(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)

	for _, effort := range []float64{0.4, 1} {
		want := moveCostAt(&cfg, effort)
		if got := w.moveCostOn(100, 100, effort); got != want {
			t.Fatalf("crossing level ground at effort %.1f cost %v, want the flat %v", effort, got, want)
		}
	}

	// And what an agent actually pays when it moves is that same figure.
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80, Genome: genomeOf(50, 50, 50)}))
	before := a.Vitality
	w.moveDir(a, 1, 0, 1)
	if paid, want := before-a.Vitality, moveCostAt(&cfg, 1); math.Abs(paid-want) > 1e-9 {
		t.Fatalf("moving cost %v of vitality, want %v", paid, want)
	}
}

// Ground that costs more to cross takes more out of whoever crosses it. There
// is nowhere in the world like that yet, so it is checked against the query
// itself: this is the shape the rule will have.
func TestRoughGroundWouldCostMoreToCross(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)

	level := w.moveCostOn(100, 100, 1)
	rough := moveCostAt(&cfg, 1) * terrain{Cost: 2}.Cost
	if rough <= level {
		t.Fatalf("rough ground costs %v and level ground %v, want crossing the rough to cost more", rough, level)
	}
}
