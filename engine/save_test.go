package engine

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sort"
	"testing"
)

// digest is what a world has come to, in enough detail that any divergence
// shows: every body's place, condition and inheritance, every meal, and the
// running totals.
//
// It is deliberately not the snapshot. Comparing two saved files would only
// compare what the save writes, and the whole question is whether the save
// writes everything - so what is compared is what the world does afterwards.
func digest(w *World) string {
	h := sha256.New()
	s := w.Stats()
	fmt.Fprintf(h, "tick=%d pop=%d births=%d deaths=%d kills=%d drowned=%d fights=%d gen=%d\n",
		s.Tick, s.Population, s.Births, s.Deaths, s.Kills, s.DrownDeaths, s.Fights, s.MaxGeneration)
	for i := range w.agents {
		a := &w.agents[i]
		fmt.Fprintf(h, "a %d %.17g %.17g %.17g %.17g %.17g %v %d %d %v\n",
			a.ID, a.X, a.Y, a.Vitality, a.Hunger, a.Maturity, a.Alive,
			a.PartnerID, a.Action.TargetID, a.Genome)
		// Opinions come back as a map, so they are read in a fixed order:
		// a hash over a map's own order is a hash of nothing.
		ops := w.Opinions(a.ID)
		ids := make([]int, 0, len(ops))
		for id := range ops {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		for _, id := range ids {
			o := ops[id]
			fmt.Fprintf(h, "  o %d %.17g %.17g %.17g %d\n",
				id, o.Risk, o.Affinity, o.Strength, o.Samples)
		}
	}
	for i := range w.foods {
		f := &w.foods[i]
		fmt.Fprintf(h, "f %d %.17g %.17g %d %d %d\n", f.ID, f.X, f.Y, f.Kind, f.SpoilAt, f.Store)
	}
	// The caches and who can find them (stage 50). Knowledge of a place is
	// state, and a world that came back with everybody having forgotten where
	// things are kept would part company with the saved one as soon as
	// anybody was hungry.
	for _, st := range w.Stores() {
		fmt.Fprintf(h, "s %d %.17g %.17g %d\n", st.Index, st.X, st.Y, st.Held)
	}
	for i := range w.agents {
		a := &w.agents[i]
		for j := range a.stores {
			m := &a.stores[j]
			fmt.Fprintf(h, "k %d %d %.17g %d\n", a.ID, j, m.n, m.lastTick)
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// The acceptance condition of stage 21, in the shape the fingerprint test uses:
// a world saved, read back and run on is the world that was never saved.
//
// Anything left out of the snapshot shows up here as a divergence, which is
// why this is the test rather than a comparison of two files.
func TestASavedWorldComesBackTheSameWorld(t *testing.T) {
	for _, ticks := range []int{200, 3000} {
		cfg := DefaultConfig()
		cfg.Seed = 5
		// Something of everything in the file: country to cross, plants with
		// genes of their own, and beliefs that are being learned rather than
		// standing still.
		cfg.TerrainMap = []string{
			"..~.::::", "..~.::A1", "..~.::11",
		}
		cfg.TerrainFoodCorrelation = 1
		cfg.HighGroundCover = 0.3
		cfg.PlantGenetics = true
		cfg.LearningRate = 0.05
		// And caches with things in them, and bodies that know where some of
		// them are (stage 50).
		cfg.OfferTicks = 30

		w := NewWorld(cfg)
		for _, at := range [][2]float64{{60, 40}, {200, 90}, {500, 300}} {
			if _, err := w.SetStore(at[0], at[1]); err != nil {
				t.Fatalf("laying out a store: %v", err)
			}
		}
		for i := 0; i < ticks; i++ {
			w.Step()
		}

		var buf bytes.Buffer
		if err := w.Save(&buf); err != nil {
			t.Fatalf("saving after %d ticks: %v", ticks, err)
		}
		saved := buf.String()

		back, err := Load(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("loading after %d ticks: %v", ticks, err)
		}
		if got, want := digest(back), digest(w); got != want {
			t.Fatalf("the world came back different after %d ticks", ticks)
		}

		// And it goes on being the same world, which is the part that catches
		// a field that is only read later.
		for i := 0; i < 500; i++ {
			w.Step()
			back.Step()
		}
		if got, want := digest(back), digest(w); got != want {
			t.Fatalf("the two worlds parted ways within 500 ticks of a save at %d", ticks)
		}
		_ = saved
	}
}

// A file this build cannot read is refused rather than read wrongly.
func TestAWorldFromAnotherFormatIsRefused(t *testing.T) {
	if _, err := Load(bytes.NewReader([]byte(`{"format":999}`))); err == nil {
		t.Fatal("read a world from a format this build knows nothing about")
	}
	if _, err := Load(bytes.NewReader([]byte("not a world at all"))); err == nil {
		t.Fatal("read a world out of nonsense")
	}
}

// The generator comes back where it was. Everything else rests on this, so it
// is worth its own test rather than being left to the digest above.
func TestTheGeneratorComesBackWhereItWas(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 12
	w := NewWorld(cfg)
	for i := 0; i < 400; i++ {
		w.Step()
	}
	if w.draws.draws == 0 {
		t.Fatal("four hundred ticks and nothing was drawn: the counter is not counting")
	}

	var buf bytes.Buffer
	if err := w.Save(&buf); err != nil {
		t.Fatal(err)
	}
	back, err := Load(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if back.draws.draws != w.draws.draws {
		t.Fatalf("came back at %d draws, was at %d", back.draws.draws, w.draws.draws)
	}
	for i := 0; i < 100; i++ {
		if a, b := w.rng.Float64(), back.rng.Float64(); a != b {
			t.Fatalf("draw %d differs: %v against %v", i, a, b)
		}
	}
}

// A population can leave its world and arrive in another one carrying what it
// is, and nothing that names anybody.
func TestAPopulationTravelsWithoutItsWorld(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 3
	old := NewWorld(cfg)
	for i := 0; i < 4000; i++ {
		old.Step()
	}
	nodes := old.Nodes()
	if len(nodes) < 5 {
		t.Fatalf("only %d bodies to take: not much of a population", len(nodes))
	}

	// Through a file, since that is the point of it.
	var buf bytes.Buffer
	if err := SaveNodes(&buf, nodes); err != nil {
		t.Fatal(err)
	}
	back, err := LoadNodes(&buf)
	if err != nil {
		t.Fatal(err)
	}

	fresh := DefaultConfig()
	fresh.Seed = 99 // a different world entirely
	w := NewWorld(fresh)
	if got := w.Repopulate(back); got != len(nodes) {
		t.Fatalf("%d of %d arrived", got, len(nodes))
	}

	live := w.Agents()
	if len(live) != len(nodes) {
		t.Fatalf("the new world holds %d bodies, want %d", len(live), len(nodes))
	}
	for i := range live {
		a := &live[i]
		if got, want := a.Genome, nodes[i].Genome; len(got) != len(want) {
			t.Fatalf("body %d arrived with %d genes, want %d", i, len(got), len(want))
		} else {
			for g := range got {
				if got[g] != want[g] {
					t.Fatalf("body %d gene %d arrived as %v, was %v", i, g, got[g], want[g])
				}
			}
		}
		if got, want := a.Assumes().RiskWeight, nodes[i].Lore.RiskWeight; got != want {
			t.Fatalf("body %d wants risk x%v, was x%v", i, got, want)
		}
		// And nothing that names anybody came with it.
		if len(w.Opinions(a.ID)) != 0 {
			t.Fatalf("body %d arrived remembering somebody from another world", i)
		}
		if a.ParentIDs != [2]int{} || len(a.ChildIDs) != 0 {
			t.Fatalf("body %d arrived with a line from another world: %v %v",
				i, a.ParentIDs, a.ChildIDs)
		}
		if a.PartnerID != 0 {
			t.Fatalf("body %d arrived still paired with somebody who is not here", i)
		}
	}

	// And the world it arrived in still works.
	for i := 0; i < 500; i++ {
		w.Step()
	}
	if w.Stats().Population == 0 {
		t.Fatal("the arrivals died out within five hundred ticks")
	}
}
