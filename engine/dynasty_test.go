package engine

import "testing"

// A quiet world with a region grid and nothing happening, so that where a
// body stands is the only thing that decides anything.
func lineWorld(t *testing.T) (*World, func(x, y float64, line uint16, age int) *Agent) {
	t.Helper()
	cfg := DefaultConfig()
	cfg.InitialPopulation, cfg.InitialFoodItems, cfg.FoodSpawnRate = 0, 0, 0
	cfg.EnemySpawnTicks = 0
	w := NewWorld(cfg)
	mid := make([]float64, NumGenes)
	for i := range mid {
		mid[i] = midAbility
	}
	place := func(x, y float64, line uint16, age int) *Agent {
		a := mustAgent(t, w, w.addAgent(w.newAgent(x, y, Male, append([]float64(nil), mid...), 0, 1)))
		a.Lineage, a.Age = line, age
		return a
	}
	return w, place
}

func TestLineHeirsIsTheEldestInEachBlock(t *testing.T) {
	w, place := lineWorld(t)
	cfg := w.Config()
	// Two blocks far enough apart to be different ones, and three bodies of
	// the line: two in the first block, one in the second.
	young := place(20, 20, 7, 100)
	old := place(40, 30, 7, 900)
	far := place(cfg.Width-20, cfg.Height-20, 7, 50)
	other := place(25, 25, 9, 5000) // a different line, however old

	heirs := w.LineHeirs(7)
	r1, r2 := w.RegionAt(old.X, old.Y), w.RegionAt(far.X, far.Y)
	if r1 == r2 {
		t.Fatalf("the two corners fell in the same block (%d)", r1)
	}
	if len(heirs) != 2 {
		t.Fatalf("LineHeirs = %v, want one per block over two blocks", heirs)
	}
	if heirs[r1] != old.ID {
		t.Fatalf("block %d gave #%d, want the elder #%d (not #%d)", r1, heirs[r1], old.ID, young.ID)
	}
	if heirs[r2] != far.ID {
		t.Fatalf("block %d gave #%d, want #%d", r2, heirs[r2], far.ID)
	}
	if heirs[r1] == other.ID {
		t.Fatalf("a body of another line was offered as this line's heir")
	}
}

func TestLineHeirsForgetsTheDead(t *testing.T) {
	w, place := lineWorld(t)
	a := place(20, 20, 7, 100)
	if len(w.LineHeirs(7)) != 1 {
		t.Fatalf("the living one was not found")
	}
	a.Alive = false
	if got := w.LineHeirs(7); len(got) != 0 {
		t.Fatalf("LineHeirs = %v after the only carrier died, want none", got)
	}
	// And a line nobody carries is not an error, it is an empty answer -
	// which is the game's "your line has nobody left anywhere".
	if got := w.LineHeirs(0); got != nil {
		t.Fatalf("LineHeirs(0) = %v, want nil", got)
	}
}

func TestSettledInIsReadableOneBodyAtATime(t *testing.T) {
	w, place := lineWorld(t)
	a := place(20, 20, 7, 100)
	tr := NewSettlementTracker(DefaultSettleWindow, DefaultSettleShare)

	// Never observed: settled nowhere, and no panic for an unknown body.
	if _, ok := tr.SettledIn(a.ID); ok {
		t.Fatalf("settled before anything was observed")
	}
	if _, ok := tr.SettledIn(9999); ok {
		t.Fatalf("a body that does not exist is settled somewhere")
	}

	for i := 0; i < DefaultSettleWindow; i++ {
		tr.Observe(w)
	}
	r, ok := tr.SettledIn(a.ID)
	if !ok {
		t.Fatalf("a body that never moved is not settled")
	}
	if want := w.RegionAt(a.X, a.Y); r != want {
		t.Fatalf("settled in block %d, want %d", r, want)
	}

	// And a body that spends its recent past wandering is settled nowhere:
	// three blocks in rotation, so none of them holds the share.
	cfg := w.Config()
	stops := []float64{cfg.Width * 0.1, cfg.Width * 0.45, cfg.Width * 0.85}
	for i := 0; i < DefaultSettleWindow; i++ {
		a.X, a.Y = stops[i%len(stops)], 20
		tr.Observe(w)
	}
	if _, ok := tr.SettledIn(a.ID); ok {
		t.Fatalf("a body that crossed the world is settled somewhere")
	}
}

func TestReadingTheLinesChangesNothing(t *testing.T) {
	// Both of these are instruments. Asking them must not move the world or
	// the random source, which is the standing every measurement here has.
	cfg := DefaultConfig()
	cfg.Seed = 5
	quiet := NewWorld(cfg)
	asked := NewWorld(cfg)
	tr := NewSettlementTracker(DefaultSettleWindow, DefaultSettleShare)
	for i := 0; i < 400; i++ {
		quiet.Step()
		asked.Step()
		asked.LineHeirs(1)
		asked.Lineages()
		tr.Observe(asked)
		for _, a := range asked.Agents() {
			tr.SettledIn(a.ID)
		}
	}
	q, a := quiet.Stats(), asked.Stats()
	if q.Population != a.Population || q.Births != a.Births || q.Kills != a.Kills {
		t.Fatalf("asking moved the world: %+v against %+v", a, q)
	}
}

// The succession the game is actually for, spelled out as the user described
// it: three sons in one block, and the line coming down through them without
// waiting for anybody else to be born.
//
// This is the test the deleted LineageRegionChain could not have passed. That
// rule handed the tag out at birth, so the second and third sons - born while
// their elder brother stood there - were never carriers at all.
func TestTheHeirPassesDownWithoutWaitingForABirth(t *testing.T) {
	w, place := lineWorld(t)
	first := place(60, 60, 7, 900)
	second := place(62, 60, 7, 700)
	third := place(64, 60, 7, 500)
	r := w.RegionAt(60, 60)

	heir := func() int { return w.LineHeirs(7)[r] }
	if heir() != first.ID {
		t.Fatalf("heir is #%d, want the eldest #%d", heir(), first.ID)
	}
	first.Alive = false
	if heir() != second.ID {
		t.Fatalf("with the eldest gone the heir is #%d, want the second son #%d", heir(), second.ID)
	}
	second.Alive = false
	if heir() != third.ID {
		t.Fatalf("with two gone the heir is #%d, want the third son #%d", heir(), third.ID)
	}

	// The third son has a child. While he lives the heir is still him.
	grandchild := place(66, 60, 7, 100)
	if heir() != third.ID {
		t.Fatalf("a child was born and the heir moved to #%d, want the father #%d", heir(), third.ID)
	}
	third.Alive = false
	if heir() != grandchild.ID {
		t.Fatalf("with the third son gone the heir is #%d, want his child #%d", heir(), grandchild.ID)
	}

	// And the whole line gone is the run lost, with nothing to wait for.
	grandchild.Alive = false
	if got := w.LineHeirs(7); len(got) != 0 {
		t.Fatalf("LineHeirs = %v with nobody of the line alive", got)
	}
}
