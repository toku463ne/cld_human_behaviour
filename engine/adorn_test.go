package engine

import "testing"

// satedConfig is the ornament world where carrying one satisfies (stage 89).
func satedConfig() Config {
	cfg := trinketConfig()
	cfg.AdornSatiety, cfg.AdornForgetPerTick = 1, 0.05
	return cfg
}

// A body carrying ornaments becomes less interested in another, and one with
// empty hands does not. Until this, the fourth piece was worth exactly what
// the first was - "a hunger that eating does not touch".
func TestCarryingOrnamentsSatisfies(t *testing.T) {
	cfg := satedConfig()
	w := NewWorld(cfg)
	bareID := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Genome: genomeOf(50, 50, 50)})
	fullID := w.addAgent(Agent{Maturity: 1, X: 200, Y: 100, Vitality: 90,
		Genome: genomeOf(50, 50, 50)})
	bare, full := mustAgent(t, w, bareID), mustAgent(t, w, fullID)
	for i := 0; i < 3; i++ {
		full.carried = append(full.carried, Food{Kind: FoodTrinket, Made: 1})
		w.heldKind[FoodTrinket]++
	}

	if got := w.adornSpare(full); got != 1 {
		t.Fatalf("a body is sated before a single tick has passed: %v", got)
	}
	for i := 0; i < 200; i++ {
		w.priceHands()
	}
	if got := w.adornSpare(bare); got != 1 {
		t.Fatalf("a body holding nothing has %v of its want left", got)
	}
	sated := w.adornSpare(full)
	if sated > 0.4 {
		t.Fatalf("a body carrying three has %v of its want left", sated)
	}

	// And the same piece is worth more to the one with empty hands: that
	// asymmetry is the whole of what the rule is for, because it is what
	// puts a buyer and a seller in the same world.
	piece := Food{Kind: FoodTrinket, Made: 1}
	if w.trinketWorth(bare, &piece) <= w.trinketWorth(full, &piece) {
		t.Fatalf("the piece is worth %v to a bare body and %v to a full one",
			w.trinketWorth(bare, &piece), w.trinketWorth(full, &piece))
	}
}

// The want comes back once the hands are empty again - which is what makes
// the spoiling of an ornament a demand that repeats, the one recurring demand
// in this world.
func TestTheWantComesBackWhenTheOrnamentsAreGone(t *testing.T) {
	w := NewWorld(satedConfig())
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Genome: genomeOf(50, 50, 50)})
	a := mustAgent(t, w, id)
	a.carried = append(a.carried, Food{Kind: FoodTrinket, Made: 1})
	w.heldKind[FoodTrinket]++
	for i := 0; i < 200; i++ {
		w.priceHands()
	}
	sated := w.adornSpare(a)

	w.removeCarried(a, 0)
	for i := 0; i < 200; i++ {
		w.priceHands()
	}
	if back := w.adornSpare(a); back <= sated || back < 0.9 {
		t.Fatalf("after losing it the body is back to %v of its want (it was %v)", back, sated)
	}
}

// Unless the world says otherwise: satisfied once and never again is the
// control, and it is what says how much of the rule is the satisfying and how
// much is the forgetting.
func TestSatisfiedForEverNeverWantsAnotherOne(t *testing.T) {
	cfg := satedConfig()
	cfg.AdornKeepsSated = true
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Genome: genomeOf(50, 50, 50)})
	a := mustAgent(t, w, id)
	a.carried = append(a.carried, Food{Kind: FoodTrinket, Made: 1})
	w.heldKind[FoodTrinket]++
	w.priceHands()
	sated := w.adornSpare(a)
	if sated >= 1 {
		t.Fatalf("one tick of holding one left %v of the want", sated)
	}
	w.removeCarried(a, 0)
	for i := 0; i < 500; i++ {
		w.priceHands()
	}
	if back := w.adornSpare(a); back != sated {
		t.Fatalf("the want came back to %v in a world where it should not", back)
	}
}

// A world that does not ask for the rule never writes the ledger and never
// reads it: every figure recorded before this stage is a figure of that
// world, and the fingerprint test is the other half of this claim.
func TestWithoutTheRuleNobodyIsEverSated(t *testing.T) {
	w := NewWorld(trinketConfig())
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Genome: genomeOf(50, 50, 50)})
	a := mustAgent(t, w, id)
	for i := 0; i < 5; i++ {
		a.carried = append(a.carried, Food{Kind: FoodTrinket, Made: 1})
		w.heldKind[FoodTrinket]++
	}
	for i := 0; i < 200; i++ {
		w.priceHands()
	}
	if a.recentAdorn != 0 {
		t.Fatalf("the ledger was written in a world without the rule: %v", a.recentAdorn)
	}
	if got := w.adornSpare(a); got != 1 {
		t.Fatalf("a body carrying five has %v of its want left", got)
	}
}

// What a sated body is willing to do about it: it makes fewer. The option is
// scored with the same figure the piece is priced with, so a body that does
// not want another does not sit down to make one.
func TestASatedBodyIsWorseOffMakingAnother(t *testing.T) {
	cfg := satedConfig()
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Genome: genomeOf(50, 50, 50)})
	a := mustAgent(t, w, id)
	w.priceHands()
	bare := craftWant(&cfg, ptr(w.selfView(a)), 0)

	for i := 0; i < 3; i++ {
		a.carried = append(a.carried, Food{Kind: FoodTrinket, Made: 1})
		w.heldKind[FoodTrinket]++
	}
	for i := 0; i < 200; i++ {
		w.priceHands()
	}
	full := craftWant(&cfg, ptr(w.selfView(a)), 0)
	if full >= bare {
		t.Fatalf("making another is worth %v to a body carrying three and %v to one carrying none", full, bare)
	}
}

func ptr(s SelfView) *SelfView { return &s }
