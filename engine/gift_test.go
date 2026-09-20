package engine

import (
	"math"
	"testing"
)

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

// --- what a gift is worth to whoever gets it (stage 89b) --------------------

// giftWorthConfig is giftConfig with a metabolism, because what a meal is
// worth is a difference between two chances of dying and a body that cannot
// starve values nothing at all. (The quiet world is right for testing that a
// thing changed hands; it is the wrong world for testing what it was worth.)
func giftWorthConfig() Config {
	cfg := testConfig()
	cfg.CarryCapacity = 1
	cfg.AffinityGift = 6
	return cfg
}

// The goodwill a gift earns is scaled by what the thing was worth to the body
// receiving it. Which bodies value a plant most is the world's business and
// not this rule's - what is tested here is that the two move together, and
// that the goodwill is the ordinary figure times that share.
func TestAGiftEarnsWhatItWasWorthToWhoeverGotIt(t *testing.T) {
	cfg := giftWorthConfig()
	cfg.GiftWorthScaled = true
	w := NewWorld(cfg)

	thanks := func(hunger float64) (float64, float64) {
		giverID := holding(t, w, 100, 100)
		takerID := w.addAgent(Agent{Maturity: 1, X: 104, Y: 100, Vitality: 90,
			Hunger: hunger, Genome: genomeOf(50, 50, 50)})
		giver, taker := mustAgent(t, w, giverID), mustAgent(t, w, takerID)
		meal := giver.carried[0]
		want := w.giftWorthShare(taker, &meal)
		giver.Action = Action{Kind: ActGive, TargetID: taker.ID, Effort: 1}
		w.perform(giver)
		if taker.CarriedCount() != 1 {
			t.Fatal("nothing changed hands")
		}
		return want, w.Opinions(taker.ID)[giver.ID].Affinity
	}

	wantA, gotA := thanks(5)
	wantB, gotB := thanks(90)
	if wantA == wantB {
		t.Fatalf("both bodies valued the same plant at %v, so there is nothing to tell apart", wantA)
	}
	for _, c := range []struct{ want, got float64 }{{wantA, gotA}, {wantB, gotB}} {
		if diff := math.Abs(c.got - cfg.AffinityGift*c.want); diff > 1e-9 {
			t.Fatalf("a gift worth %v of the standard earned %v, want %v",
				c.want, c.got, cfg.AffinityGift*c.want)
		}
	}
	if (wantA > wantB) != (gotA > gotB) {
		t.Fatalf("worth %v/%v earned %v/%v: the two do not move together",
			wantA, wantB, gotA, gotB)
	}
}

// Without the rule it is the same goodwill whoever gets it, which is every
// world before this one.
func TestWithoutTheRuleEveryGiftEarnsTheSame(t *testing.T) {
	cfg := giftWorthConfig()
	w := NewWorld(cfg)

	thanks := func(hunger float64) float64 {
		giverID := holding(t, w, 100, 100)
		takerID := w.addAgent(Agent{Maturity: 1, X: 104, Y: 100, Vitality: 90,
			Hunger: hunger, Genome: genomeOf(50, 50, 50)})
		giver, taker := mustAgent(t, w, giverID), mustAgent(t, w, takerID)
		giver.Action = Action{Kind: ActGive, TargetID: taker.ID, Effort: 1}
		w.perform(giver)
		return w.Opinions(taker.ID)[giver.ID].Affinity
	}
	if full, starving := thanks(5), thanks(90); full != starving {
		t.Fatalf("a full body thanked %v and a starving one %v", full, starving)
	}
}

// And what the measurement says it is worth is capped, so that one enormous
// gift cannot buy a lifetime of goodwill.
func TestWhatAGiftIsWorthToItsReceiverIsCapped(t *testing.T) {
	cfg := giftWorthConfig()
	cfg.CoinValue = 0.001 // a standard so small that anything dwarfs it
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 20,
		Hunger: 95, Genome: genomeOf(50, 50, 50)})
	a := mustAgent(t, w, id)
	meal := Food{Kind: FoodPlant}
	if got := w.giftWorthShare(a, &meal); got != 2 {
		t.Fatalf("an enormous gift is worth %v of the standard, and the cap is 2", got)
	}
}
