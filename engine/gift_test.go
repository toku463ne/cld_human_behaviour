package engine

import "testing"

// giftConfig is a still world where hands can hold one thing and a gift is
// worth the ordinary goodwill.
func giftConfig() Config {
	cfg := quietConfig()
	cfg.CarryCapacity = 1
	cfg.AffinityGift = 6
	return cfg
}

// holding puts a body somewhere with a plant in its hand and returns its id.
//
// An id and not a pointer: adding another agent can move the whole slice, and
// a pointer taken before that is a pointer into a copy nobody else can see.
func holding(t *testing.T, w *World, x, y float64) int {
	t.Helper()
	id := w.addAgent(Agent{Maturity: 1, X: x, Y: y, Vitality: 90,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	w.take(mustAgent(t, w, id), w.addFood(x+1, y))
	if mustAgent(t, w, id).CarriedCount() != 1 {
		t.Fatal("the body was given something and is not holding it")
	}
	return id
}

// The whole of the rule: the thing moves, and both of them think better of
// each other for it.
func TestGivingMovesTheThingAndEarnsGoodwillBothWays(t *testing.T) {
	cfg := giftConfig()
	w := NewWorld(cfg)
	giverID := holding(t, w, 100, 100)
	takerID := w.addAgent(Agent{Maturity: 1, X: 104, Y: 100, Vitality: 90,
		Genome: genomeOf(50, 50, 50)})
	giver, taker := mustAgent(t, w, giverID), mustAgent(t, w, takerID)

	before := w.countKind(FoodPlant)
	giver.Action = Action{Kind: ActGive, TargetID: taker.ID, Effort: 1}
	w.perform(giver)

	if giver.CarriedCount() != 0 || taker.CarriedCount() != 1 {
		t.Fatalf("after the gift the giver holds %d and the taker %d",
			giver.CarriedCount(), taker.CarriedCount())
	}
	if got := w.countKind(FoodPlant); got != before {
		t.Fatalf("the world holds %d plants and held %d: a gift made or lost one", got, before)
	}
	if w.Stats().Gifts != 1 {
		t.Fatalf("%d gifts counted", w.Stats().Gifts)
	}
	if a := w.Opinions(taker.ID)[giver.ID].Affinity; a <= 0 {
		t.Fatalf("the one given to thinks %v of the giver", a)
	}
	if a := w.Opinions(giver.ID)[taker.ID].Affinity; a <= 0 {
		t.Fatalf("the giver thinks %v of the one it gave to", a)
	}
}

// A full hand cannot be given to. There is nothing to refuse - receiving costs
// nothing - so what stops it is room and only room.
func TestNothingIsHandedToAFullPairOfHands(t *testing.T) {
	cfg := giftConfig()
	w := NewWorld(cfg)
	giverID, takerID := holding(t, w, 100, 100), holding(t, w, 104, 100)
	giver, taker := mustAgent(t, w, giverID), mustAgent(t, w, takerID)

	giver.Action = Action{Kind: ActGive, TargetID: taker.ID, Effort: 1}
	w.perform(giver)

	if giver.CarriedCount() != 1 || taker.CarriedCount() != 1 {
		t.Fatal("something moved into a hand that was already full")
	}
	if w.Stats().Gifts != 0 {
		t.Fatal("a gift was counted that did not happen")
	}
}

// The world where a gift buys nothing is the arm this stage is read against:
// the word is there and no body ever finds a use for it.
func TestAGiftWorthNothingIsNeverOffered(t *testing.T) {
	cfg := giftConfig()
	cfg.AffinityGift = 0
	w := NewWorld(cfg)
	giverID := holding(t, w, 100, 100)
	w.addAgent(Agent{Maturity: 1, X: 104, Y: 100, Vitality: 90, Genome: genomeOf(50, 50, 50)})

	if offered(t, w, mustAgent(t, w, giverID)) {
		t.Fatal("a gift that earns nothing was offered as an option")
	}
}

// And a gift to somebody already close is worth nothing either, because the
// goodwill it would buy is goodwill they already have. That is what makes the
// rule point at strangers.
func TestAGiftToSomebodyAlreadyCloseIsWorthNothing(t *testing.T) {
	cfg := giftConfig()
	w := NewWorld(cfg)
	giverID := holding(t, w, 100, 100)
	friendID := w.addAgent(Agent{Maturity: 1, X: 104, Y: 100, Vitality: 90,
		Genome: genomeOf(50, 50, 50)})

	if !offered(t, w, mustAgent(t, w, giverID)) {
		t.Fatal("a stranger nearby is not worth giving anything to")
	}
	w.rememberAffinity(mustAgent(t, w, giverID), friendID, cfg.AffinityTrust*2)
	if offered(t, w, mustAgent(t, w, giverID)) {
		t.Fatal("somebody already trusted is still worth buying goodwill from")
	}
}

// offered reports whether the comparison this body would run contains a gift.
func offered(t *testing.T, w *World, a *Agent) bool {
	t.Helper()
	p := w.perceive(a)
	c := &AIController{}
	c.addAgents(p, depthReactive)
	for _, o := range c.opts {
		if o.action.Kind == ActGive {
			return true
		}
	}
	return false
}
