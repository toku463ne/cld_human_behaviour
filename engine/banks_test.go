package engine

import (
	"math"
	"testing"
)

// banksWorld is a still world wide enough that two groups can stand well apart
// and the sight rule can be asked about them.
func banksWorld(t *testing.T) *World {
	t.Helper()
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	return NewWorld(cfg)
}

// banksAgent returns the id rather than the agent: adding the next one can
// move the population's backing array, and a pointer taken before that writes
// into a world that is no longer there.
func banksAgent(w *World, x, y float64) int {
	return w.addAgent(Agent{
		Maturity: 1, X: x, Y: y, Vitality: 80, Genome: genomeOf(50, 50, 50),
	})
}

// Two crowds, one on each bank and far apart: nobody sees across, and the
// index says so.
func TestBanksSeeNoCrossingWhenTheSidesKeepApart(t *testing.T) {
	w := banksWorld(t)
	for i := 0; i < 4; i++ {
		banksAgent(w, 100+float64(i), 300)
		banksAgent(w, 700+float64(i), 300)
	}
	b := w.Banks(400)
	if b.West != 4 || b.East != 4 {
		t.Fatalf("banks hold %d and %d, want 4 and 4", b.West, b.East)
	}
	if math.Abs(b.Split-0.5) > 1e-9 {
		t.Fatalf("split is %v, want 0.5", b.Split)
	}
	if b.Pairs == 0 {
		t.Fatalf("nobody could see anybody: the crowds are not standing close enough to measure")
	}
	if b.Cross != 0 || b.CrossIndex != 0 {
		t.Fatalf("crossing share %v (index %v), want none: the crowds are 600 apart", b.Cross, b.CrossIndex)
	}
}

// One crowd standing on the line: everybody sees everybody, so the crossing
// rate is exactly what pairing at random would have given, and the index is 1.
func TestBanksCrossingIndexIsOneWhenTheLineMeansNothing(t *testing.T) {
	w := banksWorld(t)
	for i := 0; i < 3; i++ {
		banksAgent(w, 399, 300)
		banksAgent(w, 401, 300)
	}
	b := w.Banks(400)
	if b.West != 3 || b.East != 3 {
		t.Fatalf("banks hold %d and %d, want 3 and 3", b.West, b.East)
	}
	if want := float64(3*3) / float64(6*5/2); math.Abs(b.Cross-want) > 1e-9 {
		t.Fatalf("crossing share is %v, want %v (every pair is in sight)", b.Cross, want)
	}
	if math.Abs(b.CrossIndex-1) > 1e-9 {
		t.Fatalf("crossing index is %v, want 1: a line through one crowd separates nobody", b.CrossIndex)
	}
}

// The predators are not part of the question. They follow whoever they can
// eat rather than settling anywhere, so counting them would put a body on a
// bank for a reason that has nothing to do with the bank.
func TestBanksCountOnlyHumans(t *testing.T) {
	w := banksWorld(t)
	banksAgent(w, 100, 300)
	mustAgent(t, w, banksAgent(w, 700, 300)).Species = SpeciesEnemy

	b := w.Banks(400)
	if b.West != 1 || b.East != 0 {
		t.Fatalf("banks hold %d and %d, want 1 and 0: the enemy is not a settler", b.West, b.East)
	}
	if b.Split != 0 {
		t.Fatalf("split is %v, want 0: one bank is empty", b.Split)
	}
}

// What the two sides are made of. The same body on both banks is no
// divergence at all; two banks that spent their whole budget on different
// genes have nothing in common.
func TestBanksGeneGapReadsTheSplitOfTheBudget(t *testing.T) {
	w := banksWorld(t)
	banksAgent(w, 100, 300)
	banksAgent(w, 700, 300)
	if got := w.Banks(400).GeneGap; got != 0 {
		t.Fatalf("gene gap between two identical bodies is %v, want 0", got)
	}

	w = banksWorld(t)
	westID, eastID := banksAgent(w, 100, 300), banksAgent(w, 700, 300)
	west, east := mustAgent(t, w, westID), mustAgent(t, w, eastID)
	for g := range west.Genome {
		west.Genome[g], east.Genome[g] = 0, 0
	}
	west.Genome[GeneAttack] = 100
	east.Genome[GeneSpeed] = 100
	if got := w.Banks(400).GeneGap; math.Abs(got-1) > 1e-9 {
		t.Fatalf("gene gap between two bodies with no gene in common is %v, want 1", got)
	}
}

// What the two sides believe about the same ground. The gap is relative to
// what a view is worth, so two banks that disagree by half of what they
// believe read the same whether the world is rich or poor.
func TestBanksCountryGapIsRelativeToWhatIsBelieved(t *testing.T) {
	for _, scale := range []float64{1, 10} {
		w := banksWorld(t)
		westID, eastID := banksAgent(w, 100, 300), banksAgent(w, 700, 300)
		west, east := mustAgent(t, w, westID), mustAgent(t, w, eastID)
		if len(w.regions) == 0 {
			t.Fatal("the world has no regions to hold a view of")
		}
		for _, a := range []*Agent{west, east} {
			a.regions = make([]regionView, len(w.regions))
		}
		// Both have been in region 0 and disagree about it by a third of
		// what they believe; neither has a view of anywhere else, so the gap
		// is taken over that one region.
		west.regions[0].setSeen(4*scale, 1, w.tick)
		east.regions[0].setSeen(8*scale, 1, w.tick)

		b := w.Banks(400)
		if b.CountryRegions != 1 {
			t.Fatalf("the gap was taken over %v regions, want 1", b.CountryRegions)
		}
		if want := 4.0 / 6.0; math.Abs(b.CountryGap-want) > 1e-9 {
			t.Fatalf("country gap at scale %v is %v, want %v", scale, b.CountryGap, want)
		}
	}
}

// The reading is a reading. It writes nothing to the world and draws no random
// number, so a run that is measured is the same run as one that is not.
func TestBanksChangeNothing(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 7
	cfg.TerrainMap = []string{
		"...~~...",
		"...~~...",
		"...~~...",
	}
	quiet, loud := NewWorld(cfg), NewWorld(cfg)
	for i := 0; i < 300; i++ {
		quiet.Step()
		loud.Step()
		loud.Banks(loud.cfg.Width / 2)
	}
	if quiet.draws.draws != loud.draws.draws {
		t.Fatalf("the measured world drew %d random numbers, the unmeasured one %d",
			loud.draws.draws, quiet.draws.draws)
	}
	q, l := quiet.Stats(), loud.Stats()
	if q != l {
		t.Fatalf("measuring the world changed it:\n%+v\n%+v", l, q)
	}
}

// The water of this world is lived in, so a body in the middle of a wide river
// sees both banks. CrossDry is the same reading with those bodies left out.
func TestBanksCrossDryLeavesOutTheBodiesInTheWater(t *testing.T) {
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	// Four columns of water down the middle: cells of 100, so the river is
	// from 300 to 500.
	cfg.TerrainMap = []string{
		"...~~...",
		"...~~...",
		"...~~...",
	}
	w := NewWorld(cfg)
	// One on each bank, far enough apart that they cannot see each other, and
	// one standing in the river between them, who can see both.
	banksAgent(w, 250, 300)
	banksAgent(w, 550, 300)
	banksAgent(w, 399, 300)
	banksAgent(w, 401, 300)

	b := w.Banks(400)
	if b.CrossIndex <= 0 {
		t.Fatalf("crossing index is %v: the two in the river should be seeing across the line", b.CrossIndex)
	}
	if b.CrossDry != 0 {
		t.Fatalf("dry crossing index is %v, want 0: the two banks cannot see each other", b.CrossDry)
	}
}

// The tracker counts a body found on the other side, and remembers that it was
// on both.
func TestBankTrackerCountsCrossings(t *testing.T) {
	w := banksWorld(t)
	id := banksAgent(w, 300, 300)
	tr := NewBankTracker(400)
	tr.Observe(w)

	if got := tr.Result(); got.Ever != 0 || got.Watched != 1 {
		t.Fatalf("after one look the tracker says %+v, want nobody crossed and one body watched", got)
	}

	// Walked across, and back again later.
	for _, x := range []float64{500, 300} {
		mustAgent(t, w, id).X = x
		for i := 0; i < 10; i++ {
			w.Step()
		}
		tr.Observe(w)
	}
	got := tr.Result()
	if got.Ever != 1 {
		t.Fatalf("the share ever on both banks is %v, want 1", got.Ever)
	}
	if got.Rate <= 0 {
		t.Fatalf("the crossing rate is %v, want more than none", got.Rate)
	}
}

// A body that never leaves its bank is watched and counted, and never counted
// as having crossed - including after it dies.
func TestBankTrackerRemembersTheDead(t *testing.T) {
	w := banksWorld(t)
	stay := banksAgent(w, 300, 300)
	tr := NewBankTracker(400)
	tr.Observe(w)
	w.Step()
	tr.Observe(w)

	mustAgent(t, w, stay).Alive = false
	w.Step()
	tr.Observe(w)

	got := tr.Result()
	if got.Watched != 1 || got.Ever != 0 {
		t.Fatalf("the tracker says %+v, want one body watched and none of it on both banks", got)
	}
}
