package engine

import "testing"

// kinConfig is the gift world with a metabolism in it: quietConfig stops
// hunger and starving, and a world where going hungry costs nothing is a world
// where nothing in a hand is worth anything. Stages 92a and 92b are both about
// what the thing is worth, so they need the rates back.
func kinConfig() Config {
	cfg := giftConfig()
	def := DefaultConfig()
	cfg.HungerRate, cfg.StarveRate, cfg.RegenRate = def.HungerRate, def.StarveRate, def.RegenRate
	return cfg
}

// giftValue is what the best hand-over to this one is scored at, and whether
// it was scored at all.
func giftValue(t *testing.T, w *World, giver *Agent, to int) (float64, bool) {
	t.Helper()
	p := w.perceive(giver)
	c := &AIController{tracing: true} // terms are only kept for a body being followed
	c.addAgents(p, depthReactive)
	best, found := 0.0, false
	for i, o := range c.opts {
		if o.action.Kind != ActGive || o.action.TargetID != to || i >= len(c.terms) {
			continue
		}
		if v := c.terms[i].Lore.Value; !found || v > best {
			best, found = v, true
		}
	}
	return best, found
}

// Stage 92a: what a gift costs its giver, at whatever share of it the world
// says a body weighs. The share is a share, and the whole of it is what
// GiftPriced does as a switch.
//
// The shares here are hundredths, and that is the stage in one line: what a
// body holds is worth tens of these units to it and the goodwill a gift buys
// is worth 2.7, so anything but a sliver of the price takes the gift off the
// list altogether. That is not a fault in the dial - it is the measurement
// stages 84d and 88 both got, arrived at from the arithmetic rather than from
// a population.
func TestPricingAGiftIsADialAndOneIsTheOldSwitch(t *testing.T) {
	worth := func(cfg Config) (float64, float64, bool) {
		w := NewWorld(cfg)
		giverID := holding(t, w, 100, 100)
		// A body that would mind losing what it holds: what a thing in the
		// hand is worth is a gap in this body's own chance of dying, and for
		// a whole satiated one that gap is nought.
		mustAgent(t, w, giverID).Hunger = w.cfg.StarveHunger - 5
		taker := w.addAgent(Agent{Maturity: 1, X: 112, Y: 100, Vitality: 90,
			Hunger: 20, Genome: genomeOf(50, 50, 50)})
		giver := mustAgent(t, w, giverID)
		// What it would be giving up, worked out here rather than read off
		// the perception: the hand is only put in there where a rule reads
		// it, so in the arm with nothing priced it is empty.
		p := w.perceive(giver)
		spare := 0.0
		for _, v := range w.handViews(giver, nil) {
			if worth := handWorth(&w.cfg, &p.Self, &v); spare == 0 || worth < spare {
				spare = worth
			}
		}
		v, ok := giftValue(t, w, giver, taker)
		return v, spare, ok
	}

	free, spare, ok := worth(kinConfig())
	if !ok || free <= 0 {
		t.Fatalf("a gift to a stranger is worth %v with nothing priced", free)
	}
	if spare <= free {
		t.Fatalf("what the giver holds is worth %v to it and the goodwill %v: "+
			"this test assumes the price is the larger", spare, free)
	}

	cfg := kinConfig()
	cfg.GiftSelfLookahead = 0.01
	thin, _, thinOK := worth(cfg)
	cfg.GiftSelfLookahead = 0.02
	twice, _, twiceOK := worth(cfg)
	if !thinOK || !twiceOK {
		t.Fatalf("a hundredth of the price took the gift off the list (%v, %v)", thinOK, twiceOK)
	}
	approx(t, free-thin, spare*0.01, 1e-9, "a hundredth of the price")
	approx(t, free-twice, spare*0.02, 1e-9, "two hundredths of the price")

	// And the whole of it is the old switch, both of which take this gift off
	// the list: what is held is worth more than the goodwill it would buy.
	cfg.GiftSelfLookahead = 1
	_, _, wholeOK := worth(cfg)
	cfg = kinConfig()
	cfg.GiftPriced = true
	_, _, oldOK := worth(cfg)
	if wholeOK || oldOK {
		t.Fatalf("at the whole price the gift is still scored (dial %v, switch %v)", wholeOK, oldOK)
	}
}

// Stage 92b: a gift to one's own child is worth something even where the
// goodwill is not, and that is what the trust could never say.
func TestAGiftToOnesOwnIsWorthSomethingWhenTrustIsFull(t *testing.T) {
	cfg := kinConfig()
	cfg.GiftKinWeight = 1
	w := NewWorld(cfg)
	giverID := holding(t, w, 100, 100)
	childID := w.addAgent(Agent{Maturity: 1, X: 112, Y: 100, Vitality: 20,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	strangerID := w.addAgent(Agent{Maturity: 1, X: 100, Y: 112, Vitality: 20,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	giver := mustAgent(t, w, giverID)
	giver.ChildIDs = append(giver.ChildIDs, childID)
	mustAgent(t, w, childID).ParentIDs[0] = giverID

	// Both are already trusted to the hilt, so the goodwill buys nothing from
	// either: kin start over the line, which is why this had to be a second
	// channel rather than a larger first one.
	w.rememberAffinity(giver, childID, cfg.AffinityTrust*2)
	w.rememberAffinity(giver, strangerID, cfg.AffinityTrust*2)

	kin, kinOK := giftValue(t, w, mustAgent(t, w, giverID), childID)
	other, otherOK := giftValue(t, w, mustAgent(t, w, giverID), strangerID)
	if !kinOK || kin <= 0 {
		t.Fatalf("handing something to one's own child is worth %v (scored %v)", kin, kinOK)
	}
	if otherOK {
		t.Fatalf("a stranger this body already trusts is still worth %v", other)
	}

	// And it is relatedness and not goodwill that says so: with the rule off,
	// the same pair of bodies is the same pair of nothings.
	cfg.GiftKinWeight = 0
	flat := NewWorld(cfg)
	flatGiver := mustAgent(t, flat, holding(t, flat, 100, 100))
	flatChild := flat.addAgent(Agent{Maturity: 1, X: 112, Y: 100, Vitality: 20,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	flatGiver.ChildIDs = append(flatGiver.ChildIDs, flatChild)
	mustAgent(t, flat, flatChild).ParentIDs[0] = flatGiver.ID
	flat.rememberAffinity(flatGiver, flatChild, cfg.AffinityTrust*2)
	if _, ok := giftValue(t, flat, flatGiver, flatChild); ok {
		t.Fatal("with the kin channel off, a child a body already trusts is still worth giving to")
	}
}

// And what it is worth is read off the receiver and not off the giver: the
// same body, holding the same thing, scores the same gift differently for two
// children in different condition, and identically for two givers in different
// condition.
//
// Which way round it goes is the world's answer and not a rule: what a meal
// does for a body is a gap in its chance of dying, and for one that is nearly
// worn through, food does not touch the thing that is killing it. So the
// receiver this world says has most to gain is the one that can still be kept
// going, not the one nearest death.
func TestTheKinTermIsReadOffTheReceiver(t *testing.T) {
	worth := func(giverVitality, childVitality float64) float64 {
		cfg := kinConfig()
		cfg.GiftKinWeight = 1
		w := NewWorld(cfg)
		giverID := holding(t, w, 100, 100)
		childID := w.addAgent(Agent{Maturity: 1, X: 112, Y: 100, Vitality: childVitality,
			Hunger: 20, Genome: genomeOf(50, 50, 50)})
		giver := mustAgent(t, w, giverID)
		giver.Vitality = giverVitality
		giver.ChildIDs = append(giver.ChildIDs, childID)
		mustAgent(t, w, childID).ParentIDs[0] = giverID
		w.rememberAffinity(giver, childID, cfg.AffinityTrust*2)
		v, ok := giftValue(t, w, mustAgent(t, w, giverID), childID)
		if !ok {
			return 0
		}
		return v
	}
	whole, worn := worth(90, 95), worth(90, 40)
	if whole == worn {
		t.Fatalf("a child at 95 and one at 40 are both worth %v to give to", whole)
	}
	if a, b := worth(90, 60), worth(30, 60); a != b {
		t.Fatalf("the same gift to the same child is worth %v from a whole giver and %v from a worn one", a, b)
	}
}
