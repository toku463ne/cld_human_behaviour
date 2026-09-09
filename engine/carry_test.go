package engine

import (
	"math"
	"testing"
)

// carryConfig is a still world with hands: no hunger, no healing, so what a
// test does to an agent is the only thing that happens to it.
// bodyOf is a genome with one gene set: carrying hangs on vitality, and these
// tests need to vary it without saying anything about the rest.
func bodyOf(vitality float64) []float64 {
	g := genomeOf(50, 50, 50)
	g[GeneVitality] = vitality
	return g
}

func carryConfig() Config {
	cfg := quietConfig()
	cfg.CarryCapacity = 1
	cfg.MeatVitality = 0
	return cfg
}

// The world before this stage: no hands, and the word for picking things up
// is never worth anything.
func TestWithoutHandsNothingIsEverPickedUp(t *testing.T) {
	cfg := carryConfig()
	cfg.CarryCapacity = 0
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80,
		Hunger: 50, Genome: genomeOf(50, 50, 50)}))
	id := w.addFood(101, 100)

	w.take(a, id)
	if a.CarriedCount() != 0 {
		t.Fatalf("a body with no hands is holding %d items", a.CarriedCount())
	}
	if w.foodByID(id) == nil {
		t.Fatal("the item left the world anyway")
	}
}

// How much a body can hold comes out of the vitality gene, not out of a gene
// of its own: buying a body that holds more vitality buys a body that carries
// more, and that is paid for out of the same budget as everything else.
func TestBiggerBodiesHoldMore(t *testing.T) {
	cfg := carryConfig()
	cfg.CarryCapacity = 4
	w := NewWorld(cfg)
	small := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Genome: bodyOf(20)}))
	big := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 300, Y: 300, Genome: bodyOf(90)}))

	if small.carryCapacity(&cfg) >= big.carryCapacity(&cfg) {
		t.Fatalf("a body of %v holds %v and one of %v holds %v",
			small.Gene(GeneVitality), small.carryCapacity(&cfg),
			big.Gene(GeneVitality), big.carryCapacity(&cfg))
	}
	if len(small.Genome) != NumGenes || len(big.Genome) != NumGenes {
		t.Fatal("carrying added a gene; it must come out of the nine there are")
	}
}

// Picking something up moves it: out of the world, into a pair of hands, and
// the world's total is what it was.
func TestTakingMovesFoodRatherThanMakingIt(t *testing.T) {
	cfg := carryConfig()
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80,
		Hunger: 50, Genome: genomeOf(50, 50, 50)}))
	id := w.addFood(101, 100)
	before := w.countKind(FoodPlant)

	w.take(a, id)
	if a.CarriedCount() != 1 {
		t.Fatalf("holding %d items after picking one up", a.CarriedCount())
	}
	if w.foodByID(id) != nil {
		t.Fatal("the item is in the world and in a hand at once")
	}
	if got := w.countKind(FoodPlant); got != before {
		t.Fatalf("the world holds %d plants, it held %d: carrying made or lost food", got, before)
	}
}

// Hands are finite, and what may be picked up is what may be eaten: a claim on
// a carcass and the rule against eating one's own kind are the same rules here
// as at the mouth.
func TestHandsAreFiniteAndTheClaimStillHolds(t *testing.T) {
	cfg := carryConfig()
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80,
		Hunger: 50, Genome: genomeOf(50, 50, 50)}))

	w.take(a, w.addFood(101, 100))
	w.take(a, w.addFood(102, 100))
	if a.CarriedCount() != 1 {
		t.Fatalf("a body with room for one is holding %d", a.CarriedCount())
	}

	// Somebody else's kill, and its own kind's carcass.
	claimed := w.putFood(Food{X: 101, Y: 100, Kind: FoodMeat, From: Species(9),
		Claim: []int{a.ID + 999}, ClaimUntil: w.tick + 100})
	own := w.putFood(Food{X: 101, Y: 100, Kind: FoodMeat, From: a.Species})
	b := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80,
		Hunger: 50, Genome: genomeOf(50, 50, 50)}))
	w.take(b, claimed)
	w.take(b, own)
	if b.CarriedCount() != 0 {
		t.Fatalf("picked up %d items it could not have eaten", b.CarriedCount())
	}
}

// A load costs, and what it is measured against is vitality: the same item is
// a lighter burden on a bigger body. Power is not in this anywhere - it is
// combat efficiency and nothing else.
func TestALoadCostsAndTheDenominatorIsVitality(t *testing.T) {
	cfg := carryConfig()
	w := NewWorld(cfg)
	small := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Genome: bodyOf(30)}))
	big := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 300, Y: 300, Genome: bodyOf(90)}))

	empty := w.moveCostOn(small, 100, 100, 1)
	w.take(small, w.addFood(101, 100))
	w.take(big, w.addFood(301, 300))
	laden := w.moveCostOn(small, 100, 100, 1)

	if laden <= empty {
		t.Fatalf("a laden body pays %v to move and an empty one %v", laden, empty)
	}
	if bigLaden := w.moveCostOn(big, 300, 300, 1); bigLaden >= laden {
		t.Fatalf("the same item costs the bigger body %v and the smaller %v", bigLaden, laden)
	}
	// And a body with strong arms and a weak frame is not helped by them.
	brawn := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 500, Y: 500,
		Genome: func() []float64 { g := bodyOf(30); g[GeneAttack] = 100; return g }()}))
	w.take(brawn, w.addFood(501, 500))
	if got := w.moveCostOn(brawn, 500, 500, 1); math.Abs(got-laden) > 1e-9 {
		t.Fatalf("a powerful body pays %v where an ordinary one of the same build pays %v", got, laden)
	}
}

// Eating out of one's own hands is the same meal as any other.
func TestEatingOutOfItsOwnHandsIsTheSameMeal(t *testing.T) {
	cfg := carryConfig()
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80,
		Hunger: 90, Genome: genomeOf(50, 50, 50)}))
	id := w.addFood(101, 100)
	w.take(a, id)

	before := a.Hunger
	a.Action = Action{Kind: ActEat, TargetID: id}
	w.perform(a)
	if got := before - a.Hunger; math.Abs(got-cfg.FoodNutrition) > 1e-9 {
		t.Fatalf("a meal out of its own hands took %v off its hunger, want %v", got, cfg.FoodNutrition)
	}
	if a.CarriedCount() != 0 {
		t.Fatal("the item is still in hand after being eaten")
	}
	if w.recentlyEaten(a, FoodPlant) == 0 {
		t.Fatal("the diet ledger did not notice a meal eaten out of a hand")
	}
}

// A body that dies leaves what it was holding where it fell. Food carried out
// of the world would be a leak in a total the world has kept fixed since
// regions arrived.
func TestWhatIsHeldFallsWhenTheBodyDoes(t *testing.T) {
	cfg := carryConfig()
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80,
		Hunger: 50, Genome: genomeOf(50, 50, 50)}))
	w.take(a, w.addFood(101, 100))
	before := w.countKind(FoodPlant)

	w.kill(a)
	if a.CarriedCount() != 0 {
		t.Fatal("the dead body is still holding something")
	}
	if got := w.countKind(FoodPlant); got != before {
		t.Fatalf("%d plants after a death, %d before", got, before)
	}
}

// The spoiling clock does not stop for being carried. Stopping it would be a
// benefit with no price, and measured with hands it costs population (#77).
func TestCarriedMeatStillRots(t *testing.T) {
	for _, keeps := range []bool{false, true} {
		cfg := carryConfig()
		cfg.CarriedMeatKeeps = keeps
		w := NewWorld(cfg)
		a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80,
			Hunger: 50, Genome: genomeOf(50, 50, 50)}))
		id := w.putFood(Food{X: 101, Y: 100, Kind: FoodMeat, From: Species(9),
			SpoilAt: w.tick + 10})
		w.take(a, id)

		w.tick += 20
		w.clearSpoiled()
		if held := a.CarriedCount(); (held == 0) == keeps {
			t.Fatalf("CarriedMeatKeeps=%v left %d items in hand", keeps, held)
		}
	}
}

// What is held is food to the one holding it and to nobody else: it is in its
// perception as a meal at no distance, and it is not in anybody's reading of
// what the ground provides.
func TestHeldFoodIsAMealAtNoDistanceAndNotAReadingOfTheGround(t *testing.T) {
	cfg := carryConfig()
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80,
		Hunger: 50, Genome: genomeOf(50, 50, 50)}))
	id := w.addFood(101, 100)
	w.take(a, id)

	p := w.perceive(a)
	if len(p.Foods) != 1 || !p.Foods[0].Held || p.Foods[0].Dist != 0 {
		t.Fatalf("what it holds reaches it as %+v", p.Foods)
	}
	if p.Self.Carried != 1 || p.Self.Burden <= 1 {
		t.Fatalf("the body reads its own hands as %d items and a burden of %v",
			p.Self.Carried, p.Self.Burden)
	}
	// How contested this patch is counts what is on the ground, and an item
	// in a hand is not a fact about the ground. With one item lying there and
	// two others in sight, the reading is two rivals per item - and it would
	// be one if the hand counted.
	b := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 105, Y: 100, Vitality: 80,
		Hunger: 50, Genome: genomeOf(50, 50, 50)}))
	mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 108, Y: 100, Vitality: 80,
		Hunger: 50, Genome: genomeOf(50, 50, 50)}))
	w.addFood(103, 100)
	if got := w.perceive(a).Self.FoodScarcity; math.Abs(got-2) > 1e-9 {
		t.Fatalf("the ground reads as %v rivals per item, want 2: a hand counted as ground", got)
	}
	// And nobody else sees what is in the hand: one item on the ground is all.
	if q := w.perceive(b); len(q.Foods) != 1 || q.Foods[0].Held {
		t.Fatalf("somebody else sees %d items, and held=%v", len(q.Foods), len(q.Foods) > 0 && q.Foods[0].Held)
	}
}

// A world where nothing is ever worth picking up is the world without the
// word. This is the control the stage is measured against, and it holds to the
// bit: adding a verb costs nothing until something is worth doing with it.
func TestAWordNobodyUsesCostsNothing(t *testing.T) {
	run := func(value float64) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 5
		cfg.CarryValue = value
		w := NewWorld(cfg)
		for i := 0; i < 2000; i++ {
			w.Step()
		}
		return w.Stats()
	}
	off := run(0)
	if off.Taken != 0 {
		t.Fatalf("%d items were picked up in a world where holding one is worth nothing", off.Taken)
	}
	cfg := DefaultConfig()
	cfg.Seed = 5
	cfg.CarryCapacity = 0
	w := NewWorld(cfg)
	for i := 0; i < 2000; i++ {
		w.Step()
	}
	hands := w.Stats()
	// The tallies that describe hands are allowed to differ: how much of a
	// carcass a party could have carried away is a figure about capacity, not
	// about anything that happened. Everything else has to match to the bit.
	hands.MeatKeepable, off.MeatKeepable = 0, 0
	hands.MeatEatenHeld, off.MeatEatenHeld = 0, 0
	hands.MeatEatenFree, off.MeatEatenFree = 0, 0
	if hands != off {
		t.Fatal("a world with hands nobody fills differs from a world with no hands")
	}
}
