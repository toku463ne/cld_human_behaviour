package engine

import "testing"

// dropFrom kills a body of the given size in front of the given attackers and
// returns what its carcass left.
func dropFrom(t *testing.T, w *World, bulk float64, attackers ...*Agent) []Food {
	t.Helper()
	prey := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 400, Y: 400, Vitality: 1,
		Genome: filledGenome(bulk), Species: Species(9)}))
	for _, a := range attackers {
		prey.noteHit(a.ID, w.tick)
	}
	before := len(w.foods)
	w.kill(prey)
	return w.foods[before:]
}

// hunter is a meat eater of a given build, standing where the kill happens.
func hunter(t *testing.T, w *World, vitality float64, x float64) *Agent {
	t.Helper()
	g := genomeOf(50, 50, 50)
	g[GeneVitality] = vitality
	return mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: x, Y: 400, Vitality: 90, Genome: g}))
}

// With the surplus closed, a carcass belongs to those who brought it down,
// every item of it - which is the world stages 11 to 40 were measured in.
func TestWithTheSurplusClosedTheWholeCarcassIsClaimed(t *testing.T) {
	cfg := carryConfig()
	cfg.MeatSurplusFree = false
	w := NewWorld(cfg)
	a := hunter(t, w, 50, 400)

	items := dropFrom(t, w, 80, a)
	if len(items) < 2 {
		t.Fatalf("the carcass left %d items; this test needs a few", len(items))
	}
	for i := range items {
		if !items[i].heldBy(a.ID) {
			t.Fatalf("item %d of %d is not claimed", i, len(items))
		}
	}
}

// With it open, the party keeps what it could carry away and the rest is
// nobody's. Nothing about the claim itself changes - what changes is how much
// of the carcass it covers.
func TestAPartyKeepsWhatItCouldCarryAndTheRestIsNobodys(t *testing.T) {
	cfg := carryConfig()
	w := NewWorld(cfg)
	a := hunter(t, w, 50, 400)

	items := dropFrom(t, w, 80, a)
	kept, free := 0, 0
	for i := range items {
		if items[i].heldBy(a.ID) {
			kept++
		} else {
			free++
		}
	}
	if kept == 0 || free == 0 {
		t.Fatalf("%d items kept and %d free: a carcass of this size should leave both", kept, free)
	}
	// And what is free is free now, to anybody that eats meat - no waiting,
	// and no new kind of ownership on it.
	other := hunter(t, w, 50, 402)
	for i := range items {
		if items[i].heldBy(a.ID) {
			continue
		}
		if !w.canEat(other, &items[i]) {
			t.Fatal("what the party left is still not anybody else's")
		}
		if items[i].ClaimUntil != 0 || len(items[i].Claim) != 0 {
			t.Fatalf("a free item still carries a claim: %+v", items[i])
		}
	}
}

// The definition costs no new figure: how much is left over follows from who
// did the killing. A party of weak bodies leaves more behind than a strong one.
func TestAWeakPartyLeavesMoreBehind(t *testing.T) {
	cfg := carryConfig()
	cfg.CarryCapacity = 3 // room for the builds to tell each other apart
	w := NewWorld(cfg)

	held := func(vitality float64) int {
		a := hunter(t, w, vitality, 400)
		items := dropFrom(t, w, 90, a)
		n := 0
		for i := range items {
			if items[i].heldBy(a.ID) {
				n++
			}
		}
		return n
	}
	weak, strong := held(20), held(100)
	if weak >= strong {
		t.Fatalf("a weak body kept %d items and a strong one %d", weak, strong)
	}
}

// Two hands keep more than one. The party is the unit, which is what ties the
// surplus to hunting together rather than to being large.
func TestTwoOfThemKeepMoreThanOne(t *testing.T) {
	cfg := carryConfig()
	w := NewWorld(cfg)
	a, b := hunter(t, w, 50, 400), hunter(t, w, 50, 402)

	count := func(items []Food, ids ...int) int {
		n := 0
		for i := range items {
			for _, id := range ids {
				if items[i].heldBy(id) {
					n++
					break
				}
			}
		}
		return n
	}
	alone := count(dropFrom(t, w, 90, a), a.ID)
	together := count(dropFrom(t, w, 90, a, b), a.ID, b.ID)
	if together <= alone {
		t.Fatalf("one of them kept %d items and two of them kept %d", alone, together)
	}
}

// The rules that were already there are untouched: nobody eats its own kind,
// and what the party did keep is still theirs until the claim runs out.
func TestOpeningTheSurplusChangesNoOtherRule(t *testing.T) {
	cfg := carryConfig()
	w := NewWorld(cfg)
	a := hunter(t, w, 50, 400)
	other := hunter(t, w, 50, 402)

	items := dropFrom(t, w, 80, a)
	for i := range items {
		if items[i].heldBy(a.ID) && w.canEat(other, &items[i]) {
			t.Fatal("somebody else may eat what the party kept")
		}
	}
	// A human's carcass is nobody's meal however much of it is surplus.
	human := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 600, Y: 600, Vitality: 1,
		Genome: filledGenome(80)}))
	before := len(w.foods)
	w.kill(human)
	for i := before; i < len(w.foods); i++ {
		if w.canEat(other, &w.foods[i]) {
			t.Fatal("a human is eating a human because the surplus was opened")
		}
	}
}

// The tally that says whether there is a surplus at all is kept in both
// worlds. A premise has to be measurable in the arm that does not act on it.
func TestTheSurplusIsCountedEvenWhereItIsNotOpened(t *testing.T) {
	for _, open := range []bool{false, true} {
		cfg := carryConfig()
		cfg.MeatSurplusFree = open
		w := NewWorld(cfg)
		a := hunter(t, w, 50, 400)
		dropFrom(t, w, 90, a)

		s := w.Stats()
		if s.MeatItems == 0 || s.MeatKeepable == 0 || s.MeatKeepable >= s.MeatItems {
			t.Fatalf("open=%v: %d items offered, %d of them keepable", open, s.MeatItems, s.MeatKeepable)
		}
	}
}
