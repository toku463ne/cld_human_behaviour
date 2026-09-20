package engine

import "testing"

// settleWorld is a world cut into two blocks with nobody moving on their own,
// so a test can put bodies where it likes and take readings.
func settleWorld(t *testing.T) (*World, Config) {
	t.Helper()
	cfg := quietConfig()
	cfg.RegionCols, cfg.RegionRows = 2, 1
	return NewWorld(cfg), cfg
}

func TestABodyThatStaysPutIsSettled(t *testing.T) {
	w, cfg := settleWorld(t)
	id := w.addAgent(Agent{Maturity: 1, Vitality: 90, Genome: filledGenome(30),
		Lineage: 1, X: cfg.Width * 0.25, Y: cfg.Height / 2})
	tr := NewSettlementTracker(DefaultSettleWindow, DefaultSettleShare)
	for i := 0; i < 10; i++ {
		tr.Observe(w)
	}
	if r, ok := tr.settledIn(id); !ok || r != 0 {
		t.Fatalf("settled in %d (%v), want the western block", r, ok)
	}
	got := tr.Result(w)
	if got.Settled != 1 {
		t.Fatalf("%.2f of the living are settled, want all of them", got.Settled)
	}
}

func TestABodyThatKeepsMovingIsSettledNowhere(t *testing.T) {
	w, cfg := settleWorld(t)
	id := w.addAgent(Agent{Maturity: 1, Vitality: 90, Genome: filledGenome(30),
		Lineage: 1, X: cfg.Width * 0.25, Y: cfg.Height / 2})
	tr := NewSettlementTracker(DefaultSettleWindow, DefaultSettleShare)
	for i := 0; i < 20; i++ {
		a := mustAgent(t, w, id)
		if i%2 == 0 {
			a.X = cfg.Width * 0.25
		} else {
			a.X = cfg.Width * 0.75
		}
		tr.Observe(w)
	}
	if _, ok := tr.settledIn(id); ok {
		t.Fatal("a body that spent half its time in each block is settled in one")
	}
	if got := tr.Result(w); got.Settled != 0 {
		t.Fatalf("%.2f of the living are settled", got.Settled)
	}
}

func TestPassingThroughIsNotLivingThere(t *testing.T) {
	// Most of the window in the west and a short crossing of the east: the
	// west is home, and the crossing is not a second home.
	w, cfg := settleWorld(t)
	id := w.addAgent(Agent{Maturity: 1, Vitality: 90, Genome: filledGenome(30),
		Lineage: 1, X: cfg.Width * 0.25, Y: cfg.Height / 2})
	tr := NewSettlementTracker(10, DefaultSettleShare)
	for i := 0; i < 10; i++ {
		a := mustAgent(t, w, id)
		a.X = cfg.Width * 0.25
		if i >= 8 {
			a.X = cfg.Width * 0.75
		}
		tr.Observe(w)
	}
	r, ok := tr.settledIn(id)
	if !ok || r != 0 {
		t.Fatalf("settled in %d (%v), want the west it spent 8 readings in", r, ok)
	}
	if got := tr.Result(w); got.InTwo != 0 {
		t.Fatalf("%.2f of the lines are settled in two blocks", got.InTwo)
	}
}

func TestALineIsSettledWhereItsBodiesAre(t *testing.T) {
	w, cfg := settleWorld(t)
	for _, p := range []struct {
		x   float64
		tag uint16
	}{
		{cfg.Width * 0.25, 1}, // one of line 1 in the west
		{cfg.Width * 0.75, 1}, // and one in the east
		{cfg.Width * 0.25, 2}, // line 2 is only in the west
	} {
		w.addAgent(Agent{Maturity: 1, Vitality: 90, Genome: filledGenome(30),
			Lineage: p.tag, X: p.x, Y: cfg.Height / 2})
	}
	tr := NewSettlementTracker(DefaultSettleWindow, DefaultSettleShare)
	for i := 0; i < 10; i++ {
		tr.Observe(w)
	}
	got := tr.Result(w)
	if got.Lines != 2 {
		t.Fatalf("%d lines have settled anywhere, want 2", got.Lines)
	}
	if got.InTwo != 0.5 {
		t.Fatalf("%.2f of the lines are settled in two blocks, want 0.5", got.InTwo)
	}
	if got.Regions != 1.5 {
		t.Fatalf("the mean settled line is in %.2f blocks, want 1.5", got.Regions)
	}
}

func TestTheDeadAreLetGoOf(t *testing.T) {
	// Otherwise the readings grow with every body that ever lived.
	w, cfg := settleWorld(t)
	id := w.addAgent(Agent{Maturity: 1, Vitality: 90, Genome: filledGenome(30),
		Lineage: 1, X: cfg.Width * 0.25, Y: cfg.Height / 2})
	tr := NewSettlementTracker(DefaultSettleWindow, DefaultSettleShare)
	tr.Observe(w)
	if len(tr.seen) != 1 {
		t.Fatalf("%d bodies watched, want 1", len(tr.seen))
	}
	w.kill(mustAgent(t, w, id))
	tr.Observe(w)
	if len(tr.seen) != 0 {
		t.Fatalf("%d bodies still watched after the only one died", len(tr.seen))
	}
}

func TestAWorldWithNoBlocksSaysNothing(t *testing.T) {
	cfg := quietConfig()
	cfg.RegionCols, cfg.RegionRows = 0, 0
	cfg.FoodSpread, cfg.ShelterSpread = 0, 0
	w := NewWorld(cfg)
	tr := NewSettlementTracker(DefaultSettleWindow, DefaultSettleShare)
	tr.Observe(w)
	if got := tr.Result(w); got.Lines != 0 || got.Settled != 0 {
		t.Fatalf("a world with no blocks read as %+v", got)
	}
}
