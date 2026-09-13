package engine

import "testing"

// dropConfig is the money world with a word for putting things down.
func dropConfig() Config {
	cfg := coinConfig()
	cfg.Dropping = true
	return cfg
}

// What is put down lands on the ground and the world has exactly as much of
// it as it did. A hand is not a place outside the world: what is in one was
// counted in the allowance the moment it was picked up, so putting it down is
// a move and never a loss.
func TestWhatIsPutDownIsStillInTheWorld(t *testing.T) {
	w := NewWorld(dropConfig())
	id := holding(t, w, 100, 100)
	a := mustAgent(t, w, id)
	item := a.Carrying()[0]
	before := w.countKind(item.Kind)

	if !w.dropItem(a, item.ID) {
		t.Fatal("nothing was put down")
	}
	a = mustAgent(t, w, id)
	if a.CarriedCount() != 0 {
		t.Fatalf("the hand still has %d in it", a.CarriedCount())
	}
	if got := w.countKind(item.Kind); got != before {
		t.Fatalf("the world has %d of them and had %d", got, before)
	}
	if w.Stats().Dropped != 1 {
		t.Fatalf("the world counted %d of them", w.Stats().Dropped)
	}
	// And it is where the body is, near enough to be picked up again.
	found := false
	for _, f := range w.Foods() {
		if f.Kind == item.Kind && dist2(f.X, f.Y, a.X, a.Y) <= 64 {
			found = true
		}
	}
	if !found {
		t.Fatal("it is not on the ground where the body is standing")
	}
}

// Without the rule the word is not in the comparison at all - and a world
// that never offers it never runs the arithmetic behind it either.
func TestNobodyPutsAnythingDownWithoutTheWord(t *testing.T) {
	cfg := dropConfig()
	cfg.Dropping = false
	w := NewWorld(cfg)
	id := holding(t, w, 100, 100)
	c, _ := scored(t, w, id)
	for _, o := range c.opts {
		if o.action.Kind == ActDrop {
			t.Fatal("something was offered the chance to put its dinner down")
		}
	}
}

// A body knows its own hands, all of it - the coin included, which is not
// food and is in no list of meals.
func TestABodyCanSeeWhatIsInItsOwnHand(t *testing.T) {
	w := NewWorld(dropConfig())
	id := holdingCoin(t, w, 100, 100)
	p := w.perceive(mustAgent(t, w, id))
	if len(p.Held) != 1 || p.Held[0].Kind != FoodCoin {
		t.Fatalf("the hand reads as %v", p.Held)
	}
	for i := range p.Foods {
		if p.Foods[i].Held {
			t.Fatal("a coin turned up in the list of meals")
		}
	}
}

// The one this word was built for, and the one it cannot do. Money weighs
// nothing, so putting it down saves nothing, and what it is worth is
// CoinValue of a meal against a hand worth need x race of one. The
// comparison therefore refuses, and that is arithmetic rather than luck: it
// is why stage 51a's blocked hands stay blocked.
func TestMoneyIsNeverWorthPuttingDown(t *testing.T) {
	w := NewWorld(dropConfig())
	id := holdingCoin(t, w, 100, 100)
	// Something to want, and somebody to make the patch feel contested.
	w.putFood(Food{X: 110, Y: 100, Kind: FoodPlant})
	w.addAgent(Agent{Maturity: 1, X: 100, Y: 160, Vitality: 90,
		Hunger: 85, Genome: genomeOf(50, 50, 50)})
	a := mustAgent(t, w, id)
	a.Hunger = 85

	c, _ := scoredTraced(t, w, id)
	for i := range c.opts {
		if c.opts[i].action.Kind == ActDrop {
			t.Fatalf("putting a coin down was offered, at %v", c.terms[i].Life.Value)
		}
	}
}

// What it can do: a fed body carrying a meal it puts no value on is paying
// weight for it every step, and that is what putting it down buys back.
func TestAFedBodyPutsDownWhatItIsPayingToCarry(t *testing.T) {
	w := NewWorld(dropConfig())
	id := holding(t, w, 100, 100)
	a := mustAgent(t, w, id)
	a.Hunger = 0 // nothing it could be keeping this for

	c, _ := scoredTraced(t, w, id)
	for i := range c.opts {
		o := &c.opts[i]
		if o.action.Kind != ActDrop {
			continue
		}
		if got := c.terms[i].Life.Value; got <= 0 {
			t.Fatalf("putting down a meal worth nothing to it is worth %v", got)
		}
		return
	}
	t.Fatal("nothing was scored for putting the meal down")
}
