package engine

import (
	"math"
	"testing"
)

// A world that did not buy room for an opinion has none, pays for none, and
// runs exactly as it always did - which the fingerprint test says for the
// default world and this says for the parts of it a fingerprint cannot reach.
func TestOpinionsAboutThingsAreOffByDefault(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 3
	w := NewWorld(cfg)
	for i := 0; i < 300; i++ {
		w.Step()
	}
	got := w.ItemLore()
	if got.Slots != 0 || got.Held != 0 || got.Learnt != 0 {
		t.Fatalf("the default world learnt something about things: %+v", got)
	}
	for i := range w.agents {
		if len(w.agents[i].itemLore) != 0 || len(w.agents[i].itemHolds) != 0 {
			t.Fatal("the default world hung an opinion off a body")
		}
	}
}

// The room comes out of the same budget the genes are fitted to, which is the
// conservation law: learning cannot make a body better for free.
func TestRoomForOpinionsCostsBudget(t *testing.T) {
	sum := func(slots int) float64 {
		cfg := DefaultConfig()
		cfg.Seed = 11
		cfg.ItemSlots = slots
		w := NewWorld(cfg)
		total, n := 0.0, 0.0
		for i := range w.agents {
			a := &w.agents[i]
			if a.itemSlots != slots {
				continue // only the bodies that bought the lot
			}
			for _, g := range a.Genome {
				total += g
			}
			n++
		}
		if n == 0 {
			t.Fatal("no body bought the whole range")
		}
		return total / n
	}
	rich, poor := sum(0), sum(3)
	if poor >= rich {
		t.Fatalf("three opinions cost nothing: genes sum to %.2f with room and %.2f without",
			poor, rich)
	}
}

// What a body learns is what followed having one, over and above how its life
// was going anyway, in the formula's own units and with both signs.
func TestAThingIsWorthWhatFollowedIt(t *testing.T) {
	w, a := itemWorld(t)
	a.itemSlots = 3
	w.Step()

	// One arrives, good things happen to its holder, then it goes.
	a.carried = append(a.carried, Food{Kind: FoodStone})
	w.Step()
	for i := 0; i < 3; i++ {
		w.fireCorr(a, CorrFed, 10)
	}
	a.carried = a.carried[:0]
	w.Step()
	stone := a.itemValue(int(FoodStone))
	if stone <= 0 {
		t.Fatalf("a stone followed by three meals is worth %.3f, want better than nothing", stone)
	}

	// And one that is followed by trouble loses what one followed by a meal
	// gains: there is no label on an outcome.
	a.carried = append(a.carried, Food{Kind: FoodBook})
	w.Step()
	w.fireCorr(a, CorrHurt, -20)
	a.carried = a.carried[:0]
	w.Step()
	if got := a.itemValue(int(FoodBook)); got >= 0 {
		t.Fatalf("a book followed by a blow is worth %.3f, want less than nothing", got)
	}
}

// And the baseline is doing its work: this world's events average out well
// above nought, so without it anything held long enough would collect a
// fortune it had nothing to do with.
//
// Driven by hand rather than by Step, because what is being checked is the
// arithmetic of the estimator: a life going along at a steady rate, and a
// thing held across the same steady rate, has to come out at nothing.
func TestAThingThatChangesNothingIsWorthNothing(t *testing.T) {
	w, a := itemWorld(t)
	a.itemSlots = 3
	pass := func(v float64) {
		w.creditItemHolds(a, v)
		w.tick++
		a.lifeTicks++
	}
	for i := 0; i < 20; i++ {
		pass(5)
	}
	a.itemHolds = append(a.itemHolds, itemHold{kind: int(FoodStone), at: w.tick})
	for i := 0; i < 5; i++ {
		pass(5)
	}
	w.bankItemHold(a, int(FoodStone))
	if got := a.itemValue(int(FoodStone)); math.Abs(got) > 1e-9 {
		t.Fatalf("a thing that changed nothing is worth %.3f, want nothing", got)
	}
}

// A coat is not an ornament. They are the same FoodKind, and the counting found
// forty points between them, so the opinions have to be kept apart.
func TestACoatIsNotAnOrnament(t *testing.T) {
	w, a := itemWorld(t)
	a.itemSlots = 4
	w.Step()
	a.carried = append(a.carried,
		Food{Kind: FoodTrinket},
		Food{Kind: FoodTrinket, Ward: 0.5})
	w.Step()
	w.fireCorr(a, CorrFed, 8)
	a.carried = a.carried[:0]
	w.Step()

	if a.itemValue(int(FoodTrinket)) == 0 || a.itemValue(corrCoat) == 0 {
		t.Fatal("one of the two was not learnt at all")
	}
	held, _ := a.ItemValues()
	names := map[string]bool{}
	for _, o := range held {
		names[o.Name] = true
	}
	if !names["trinket"] || !names["coat"] {
		t.Fatalf("the two were not kept apart: %+v", held)
	}
}

// Room is the scarce thing, and a body that filled what it bought is finished.
func TestOpinionsStopAtTheRoomBought(t *testing.T) {
	w, a := itemWorld(t)
	a.itemSlots = 1
	w.Step()
	a.carried = append(a.carried, Food{Kind: FoodStone}, Food{Kind: FoodBook})
	w.Step()
	w.fireCorr(a, CorrFed, 5)
	a.carried = a.carried[:0]
	w.Step()
	if len(a.itemLore) != 1 {
		t.Fatalf("a body with one slot holds %d opinions", len(a.itemLore))
	}
}

// And it decides nothing on its own: what it does is add to a score.
func TestAnOpinionOnlyAddsToAScore(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ItemValueWeight = 2
	var u Utility
	u.Life = Goal{Value: 10, Chance: 1}
	before := u.Total()
	u.Thing = 7
	if got := u.Total() - before; math.Abs(got-7) > 1e-9 {
		t.Fatalf("an opinion moved the score by %.3f, want 7", got)
	}
}

// An opinion spreads the way a rule of thumb does: into an empty slot, never
// over the top of one, and as a claim rather than as the watching behind it.
func TestAnOpinionIsCopiedIntoAnEmptySlot(t *testing.T) {
	w, a := itemWorld(t)
	b := &w.agents[1]
	a.itemSlots, b.itemSlots = 2, 2
	a.itemLore = []itemOpinion{{kind: int(FoodStone), b: belief{mean: 12, n: 9}}}

	if n := w.exchangeItemValues(b, a); n != 1 {
		t.Fatalf("copied %d opinions, want 1", n)
	}
	if got := b.itemValue(int(FoodStone)); got != 12 {
		t.Fatalf("the copy is worth %.3f, want 12", got)
	}
	if b.itemLore[0].b.n != 1 {
		t.Fatalf("being told made it sure: n = %.1f, want 1", b.itemLore[0].b.n)
	}
	if n := w.exchangeItemValues(b, a); n != 0 {
		t.Fatal("the same opinion was copied twice")
	}
}

func itemWorld(t *testing.T) (*World, *Agent) {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Seed = 5
	cfg.ItemSlots = 3
	cfg.InitialPopulation = 4
	cfg.InitialEnemies = 0
	w := NewWorld(cfg)
	if len(w.agents) < 2 {
		t.Fatal("the world came up empty")
	}
	return w, &w.agents[0]
}

// The residual: what is learnt is what happened less what the formula had
// already made of the thing, so that adding it to a score the formula has
// already priced does not count the same worth twice.
func TestTheResidualTakesOffWhatTheFormulaExpected(t *testing.T) {
	learn := func(residual bool) float64 {
		cfg := DefaultConfig()
		cfg.Seed = 5
		cfg.ItemSlots = 3
		cfg.ItemValueResidual = residual
		cfg.InitialPopulation = 4
		cfg.InitialEnemies = 0
		w := NewWorld(cfg)
		a := &w.agents[0]
		a.itemSlots = 3
		w.Step()
		// A meal, which is the one kind the formula has a firm figure for.
		a.Hunger = w.cfg.MaxHunger * 0.8
		a.carried = append(a.carried, Food{Kind: FoodPlant})
		w.Step()
		w.fireCorr(a, CorrFed, 40)
		a.carried = a.carried[:0]
		w.Step()
		return a.itemValue(int(FoodPlant))
	}
	raw, residual := learn(false), learn(true)
	if residual >= raw {
		t.Fatalf("the residual is %.3f against the whole of %.3f: nothing was taken off",
			residual, raw)
	}
}
