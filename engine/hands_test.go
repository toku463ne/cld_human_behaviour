package engine

import "testing"

// handsConfig is the money world with no gate on the hand and the second
// thing in it worth less than the first.
func handsConfig() Config {
	cfg := coinConfig()
	cfg.CarrySlotted = false
	cfg.CarryDiminishes = true
	return cfg
}

// With no gate, what a body may hold is what it is willing to carry.
func TestWithNoGateAHandIsNotASlot(t *testing.T) {
	cfg := handsConfig()
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	a := mustAgent(t, w, id)
	if slots := a.carrySlots(&cfg); slots != 1 {
		t.Fatalf("this body has %d slots and the test wants the world's usual one", slots)
	}
	for i := 0; i < 4; i++ {
		w.take(mustAgent(t, w, id), w.addFood(101, 100))
	}
	if got := mustAgent(t, w, id).CarriedCount(); got != 4 {
		t.Fatalf("a body with no gate is holding %d of the four it picked up", got)
	}

	// And the weight goes on climbing: a fifth item has to cost more than a
	// second, or nothing stops a body taking the whole world.
	a = mustAgent(t, w, id)
	four := a.burden(&cfg)
	w.take(mustAgent(t, w, id), w.addFood(101, 100))
	if five := mustAgent(t, w, id).burden(&cfg); five <= four {
		t.Fatalf("the fifth item is free: burden %v then %v", four, five)
	}

	// With the gate on, the same body stops at one.
	cfg.CarrySlotted = true
	slotted := NewWorld(cfg)
	id = slotted.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	for i := 0; i < 4; i++ {
		slotted.take(mustAgent(t, slotted, id), slotted.addFood(101, 100))
	}
	if got := mustAgent(t, slotted, id).CarriedCount(); got != 1 {
		t.Fatalf("a body with the gate on is holding %d", got)
	}
}

// Taking the gate off does not move what a hunting party claims of a carcass
// (stage 41). That rule asks a different question through the same figure -
// how much a party could carry away - and it is not what a hand will take.
func TestTheSurplusRuleDoesNotDependOnTheGate(t *testing.T) {
	cfg := handsConfig()
	slotted := cfg
	slotted.CarrySlotted = true
	w, v := NewWorld(cfg), NewWorld(slotted)
	a := &Agent{Maturity: 1, Genome: genomeOf(50, 50, 50)}
	if got, want := a.carrySlots(&w.cfg), a.carrySlots(&v.cfg); got != want {
		t.Fatalf("a party of one claims %d with no gate and %d with one", got, want)
	}
}

// A body only runs short once, so the second meal in a hand is worth less
// than the first - and a body holding its dinner puts next to nothing on a
// coin, which is the whole of why money stops pushing food out of hands.
func TestTheSecondThingInAHandIsWorthLess(t *testing.T) {
	cfg := handsConfig()
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 40,
		Hunger: 85, Genome: genomeOf(50, 50, 50)})
	s := w.selfView(mustAgent(t, w, id))

	first := keepValue(&cfg, &s, 0, 1, 0, 0, 0)
	second := keepValue(&cfg, &s, 0, 1, 0, 1, 0)
	if first <= 0 {
		t.Fatalf("the first meal is worth %v to a hungry body", first)
	}
	if second >= first {
		t.Fatalf("the second meal is worth %v against the first at %v", second, first)
	}
	if coin := coinWorth(&cfg, &s, 1); coin >= coinWorth(&cfg, &s, 0) {
		t.Fatal("a coin is worth as much to a body that is already holding its dinner")
	}

	// And with the rule off, the tenth is worth exactly what the first is.
	cfg.CarryDiminishes = false
	if got := keepValue(&cfg, &s, 0, 1, 0, 10, 0); got != first {
		t.Fatalf("with the rule off the tenth is worth %v and the first %v", got, first)
	}
}

// What one more item costs to lug leaves out what weighs nothing. A body
// holding a coin is carrying no weight, and used to be told otherwise.
func TestMoneyIsNotAWeightWhenReckoningTheNextItem(t *testing.T) {
	cfg := coinConfig()
	// Two hands, so that the over-charge is visible: with one, both sides
	// are at the top of the load either way.
	cfg.CarryCapacity = 2
	w := NewWorld(cfg)
	empty := w.selfView(mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100,
		Vitality: 90, Hunger: 20, Genome: genomeOf(50, 50, 50)})))
	withCoin := w.selfView(mustAgent(t, w, holdingCoin(t, w, 200, 100)))

	if a, b := burdenWith(&cfg, &empty, 1), burdenWith(&cfg, &withCoin, 1); a != b {
		t.Fatalf("one more item costs %v with empty hands and %v holding a coin", a, b)
	}
	// And the old arithmetic is still there to be asked for.
	cfg.BurdenIgnoresWeightless = false
	if a, b := burdenWith(&cfg, &empty, 1), burdenWith(&cfg, &withCoin, 1); a == b {
		t.Fatal("the old over-charge cannot be put back")
	}
}
