package engine

import "testing"

// lightHandConfig is the money world with the hand priced by weight alone
// (stage 80a): what weighs nothing takes no hand.
func lightHandConfig() Config {
	cfg := coinConfig()
	cfg.CarrySlotsWeigh = true
	return cfg
}

// A coin takes no hand, so a body with its dinner in it can still pick money
// up - which is the whole rule.
func TestACoinTakesNoHand(t *testing.T) {
	cfg := lightHandConfig()
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	w.take(mustAgent(t, w, id), w.addFood(101, 100))
	if got := mustAgent(t, w, id).CarriedCount(); got != 1 {
		t.Fatalf("the body is holding %d things and was handed one meal", got)
	}
	for i := 0; i < 3; i++ {
		w.take(mustAgent(t, w, id), w.putFood(Food{X: 101, Y: 100, Kind: FoodCoin}))
	}
	a := mustAgent(t, w, id)
	if got := a.CarriedCount(); got != 4 {
		t.Fatalf("a meal and three coins is %d things in hand", got)
	}
	if got := a.heavyCarried(); got != 1 {
		t.Fatalf("%d of them weigh something, and only the meal does", got)
	}

	// And the meal still takes one: a second one does not go in.
	w.take(mustAgent(t, w, id), w.addFood(101, 100))
	if got := mustAgent(t, w, id).CarriedCount(); got != 4 {
		t.Fatalf("a second meal went into a hand that was full of the first: %d things held", got)
	}
}

// With the rule off, which is every world before it, a coin fills the hand.
func TestWithoutTheRuleACoinFillsTheHand(t *testing.T) {
	w := NewWorld(coinConfig())
	id := holdingCoin(t, w, 100, 100)
	w.take(mustAgent(t, w, id), w.addFood(101, 100))
	if got := mustAgent(t, w, id).CarriedCount(); got != 1 {
		t.Fatalf("the hand held a coin and now holds %d things", got)
	}
}

// The gift goes through too: a coin can be handed to somebody whose hand is
// full of dinner, because it needs no hand of its own.
func TestACoinCanBeHandedToAFullHand(t *testing.T) {
	cfg := lightHandConfig()
	w := NewWorld(cfg)
	giver := holdingCoin(t, w, 100, 100)
	taker := w.addAgent(Agent{Maturity: 1, X: 101, Y: 100, Vitality: 90,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	w.take(mustAgent(t, w, taker), w.addFood(102, 100))
	if got := mustAgent(t, w, taker).CarriedCount(); got != 1 {
		t.Fatal("the receiver was given a meal and is not holding it")
	}
	if !w.giveItem(mustAgent(t, w, giver), mustAgent(t, w, taker)) {
		t.Fatal("a coin could not be handed to a body holding its dinner")
	}
	if got := mustAgent(t, w, taker).CarriedCount(); got != 2 {
		t.Fatalf("the receiver holds %d things after being handed a coin", got)
	}

	// A meal to the same full hand still does not go.
	other := w.addAgent(Agent{Maturity: 1, X: 101, Y: 100, Vitality: 90,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	w.take(mustAgent(t, w, other), w.addFood(103, 100))
	if w.giveItem(mustAgent(t, w, other), mustAgent(t, w, taker)) {
		t.Fatal("a second meal went into a hand that already held one")
	}
}

// And the body knows it: the two rooms are the same bool in every world
// before this rule, and differ only where it is on.
func TestWhatTheBodyKnowsAboutItsHands(t *testing.T) {
	for _, on := range []bool{false, true} {
		cfg := coinConfig()
		cfg.CarrySlotsWeigh = on
		w := NewWorld(cfg)
		id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
			Hunger: 20, Genome: genomeOf(50, 50, 50)})
		w.take(mustAgent(t, w, id), w.addFood(101, 100))
		s := w.selfView(mustAgent(t, w, id))
		if s.CarryRoom {
			t.Fatalf("rule=%v: a hand holding a meal has room for another", on)
		}
		if s.LightRoom != on {
			t.Fatalf("rule=%v: LightRoom is %v", on, s.LightRoom)
		}
	}
}

// Money is still not free: what it costs is the walk to it and what it is
// worth when it gets there, and picking one up is still scored against
// everything else. A body that has run right down does not go shopping.
func TestTheRuleIsNotAReasonToWantMoney(t *testing.T) {
	cfg := lightHandConfig()
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 12,
		Hunger: 95, Genome: genomeOf(50, 50, 50)})
	w.putFood(Food{X: 160, Y: 100, Kind: FoodCoin})
	w.addFood(104, 100)
	a := mustAgent(t, w, id)
	act := (&AIController{}).Decide(w.perceive(a))
	if act.Kind == ActTake {
		if f := w.foodByID(act.TargetID); f != nil && f.Kind == FoodCoin {
			t.Fatal("a starving body walked past a meal to pick up money")
		}
	}
}
