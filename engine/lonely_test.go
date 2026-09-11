package engine

import (
	"math"
	"testing"
)

func lonelyConfig() Config {
	cfg := quietConfig()
	cfg.LonelyValue = 20
	cfg.LonelyHalfLife = 400
	return cfg
}

// twoFriends puts a body and somebody it thinks well of on the world, with the
// second far enough off to be moved in and out of sight.
// Ids and not pointers: adding the second one can move the whole slice, and a
// pointer taken before that is a pointer into a copy nobody else can see.
func twoFriends(t *testing.T, cfg Config) (*World, int, int) {
	t.Helper()
	w := NewWorld(cfg)
	aID := w.addAgent(Agent{Maturity: 1, X: 400, Y: 400, Vitality: 90,
		Genome: genomeOf(50, 50, 50)})
	oID := w.addAgent(Agent{Maturity: 1, X: 440, Y: 400, Vitality: 90,
		Genome: genomeOf(50, 50, 50)})
	w.rememberAffinity(mustAgent(t, w, aID), oID, 20)
	return w, aID, oID
}

// Losing sight of them leaves a direction, and seeing them leaves none.
func TestLosingSightLeavesADirection(t *testing.T) {
	w, aID, oID := twoFriends(t, lonelyConfig())
	a, o := mustAgent(t, w, aID), mustAgent(t, w, oID)

	w.perceive(a)
	if got, _, _ := w.missing(a); got != 0 {
		t.Fatalf("with them in sight the body is missing %v", got)
	}
	// Away, in a direction nothing else could have supplied.
	o.X, o.Y = 4000, 400
	w.perceive(a)
	weight, dx, dy := w.missing(a)
	if weight <= 0 {
		t.Fatal("losing sight of them left nothing")
	}
	if math.Abs(dx-1) > 1e-9 || math.Abs(dy) > 1e-9 {
		t.Fatalf("they went east and the direction kept is (%v, %v)", dx, dy)
	}
	// And finding them again clears it: there is nothing to miss.
	o.X, o.Y = 440, 400
	w.perceive(a)
	if got, _, _ := w.missing(a); got != 0 {
		t.Fatalf("with them in sight again the body is still missing %v", got)
	}
}

// It fades, and it is dropped rather than followed for ever.
func TestADirectionFadesAndIsDropped(t *testing.T) {
	w, aID, oID := twoFriends(t, lonelyConfig())
	a, o := mustAgent(t, w, aID), mustAgent(t, w, oID)
	w.perceive(a)
	o.X, o.Y = 4000, 400
	w.perceive(a)

	full, _, _ := w.missing(a)
	w.tick += w.cfg.LonelyHalfLife
	half, _, _ := w.missing(a)
	if half > 0.51*full || half < 0.49*full {
		t.Fatalf("after one half-life the direction is worth %v of %v", half, full)
	}
	w.tick += 10 * w.cfg.LonelyHalfLife
	if got, _, _ := w.missing(a); got != 0 {
		t.Fatalf("after eleven half-lives it is still worth %v", got)
	}
	if a.lostAt != 0 {
		t.Fatal("a direction worth nothing is still being carried")
	}
}

// The control points the other way, at the same weight and the same price.
func TestTheControlPointsTheWrongWay(t *testing.T) {
	cfg := lonelyConfig()
	cfg.LonelyWrongWay = true
	w, aID, oID := twoFriends(t, cfg)
	a, o := mustAgent(t, w, aID), mustAgent(t, w, oID)
	w.perceive(a)
	o.X, o.Y = 4000, 400
	w.perceive(a)
	weight, dx, _ := w.missing(a)
	if weight <= 0 {
		t.Fatal("the control remembers nothing at all")
	}
	if dx > -0.9 {
		t.Fatalf("they went east and the control points %v", dx)
	}
}

// What it does to a decision: one more way to wander, and nothing else.
func TestTheDirectionIsOneMoreCandidate(t *testing.T) {
	cfg := lonelyConfig()
	cfg.Seed = 4
	cfg.ChoiceNoise = 0
	w, aID, oID := twoFriends(t, cfg)
	a, o := mustAgent(t, w, aID), mustAgent(t, w, oID)
	w.TrackDecisions(a.ID, true)
	w.perceive(a) // in sight, so which way they are is known
	o.X, o.Y = 4000, 400
	w.perceive(a) // and now the loss is noted

	w.decide(a, TriggerIdle)
	tr, ok := w.LastDecisionTrace(a.ID)
	if !ok {
		t.Fatal("no trace")
	}
	found := false
	for _, opt := range tr.Options {
		if opt.Action.Kind == ActMove && opt.Action.DX > 0.9 && opt.Utility.Lore.Value > 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("the way they went was never offered as a candidate")
	}
}

// A world with no value on it is the world before this stage, bit for bit.
func TestNoLonelyValueIsTheWorldWithoutIt(t *testing.T) {
	run := func(f func(*Config)) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 6
		f(&cfg)
		w := NewWorld(cfg)
		for i := 0; i < 3000; i++ {
			w.Step()
		}
		return w.Stats()
	}
	off := run(func(c *Config) { c.LonelyValue, c.LonelyHalfLife = 0, 0 })
	zero := run(func(c *Config) { c.LonelyValue = 0 })
	if off != zero {
		t.Fatalf("a world with no direction in it differs:\n %+v\n %+v", off, zero)
	}
}
