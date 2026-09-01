package engine

import "testing"

// A carcass is left where somebody died, and how much of it there is scales
// with how much the dead creature was made of. That is the whole reason a
// group would be better at hunting than an individual.
func TestACarcassScalesWithTheSizeOfWhatDied(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	small := w.agentByID(w.addAgent(Agent{X: 100, Y: 100, Vitality: 10,
		Genome: filledGenome(20), Species: Species(1)}))
	big := w.agentByID(w.addAgent(Agent{X: 300, Y: 300, Vitality: 10,
		Genome: filledGenome(80), Species: Species(1)}))

	before := len(w.Foods())
	w.kill(small)
	afterSmall := len(w.Foods()) - before
	w.kill(big)
	afterBig := len(w.Foods()) - before - afterSmall

	if afterSmall == 0 {
		t.Fatal("a body left nothing to eat")
	}
	if afterBig <= afterSmall {
		t.Fatalf("a body of %.0f left %d items and one of %.0f left %d: the drop does not scale",
			small.Budget(), afterSmall, big.Budget(), afterBig)
	}
	for _, f := range w.Foods() {
		if f.Kind != FoodMeat || f.From != Species(1) {
			t.Fatalf("carcass item is %v from %v, want meat from species 1", f.Kind, f.From)
		}
	}
}

// Nobody eats its own dead, and everybody else may - once the claim has run
// out. Without the claim, standing back and letting somebody else make the
// kill would be the best move there is.
func TestACarcassBelongsToWhoeverBroughtItDown(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	prey := w.agentByID(w.addAgent(Agent{X: 100, Y: 100, Vitality: 10, Species: Species(1)}))
	hunter := w.agentByID(w.addAgent(Agent{X: 105, Y: 100, Vitality: 90}))
	idler := w.agentByID(w.addAgent(Agent{X: 110, Y: 100, Vitality: 90}))
	cannibal := w.agentByID(w.addAgent(Agent{X: 115, Y: 100, Vitality: 90, Species: Species(1)}))

	prey.noteHit(hunter.ID, w.tick)
	w.kill(prey)

	meat := &w.foods[0]
	if !w.canEat(hunter, meat) {
		t.Error("the agent that made the kill may not eat it")
	}
	if w.canEat(idler, meat) {
		t.Error("an onlooker may eat a kill it took no part in")
	}
	if w.canEat(cannibal, meat) {
		t.Error("a species is eating its own dead")
	}

	// Once the claim has run out it is anybody's.
	w.tick += cfg.MeatClaimTicks + 1
	if !w.canEat(idler, meat) {
		t.Error("the claim never runs out")
	}
	if w.canEat(cannibal, meat) {
		t.Error("cannibalism became possible once the claim ran out")
	}
}

// Only recent blows count as taking part, so a scratch landed long before the
// kill is not a share of it.
func TestOnlyRecentBlowsCountAsTakingPart(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	prey := w.agentByID(w.addAgent(Agent{X: 100, Y: 100, Vitality: 10, Species: Species(1)}))

	prey.noteHit(7, 0)
	w.tick = cfg.HuntCreditTicks + 10
	prey.noteHit(9, w.tick)

	got := prey.recentAttackers(w.tick, cfg.HuntCreditTicks)
	if len(got) != 1 || got[0] != 9 {
		t.Fatalf("claim %v, want only the recent attacker", got)
	}
}

// Food an agent may not eat is not food to it: it does not appear in what it
// can see, and trying to take it anyway does nothing.
func TestInedibleFoodIsInvisible(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	a := w.agentByID(w.addAgent(Agent{X: 100, Y: 100, Vitality: 90, Hunger: 50}))
	id := w.addFood(105, 100)
	f := w.foodByID(id)
	f.Kind, f.From = FoodMeat, SpeciesHuman // its own kind

	if got := len(w.perceive(a).Foods); got != 0 {
		t.Fatalf("saw %d items of food it cannot eat", got)
	}
	if got := w.nearestFoodInSight(a); got >= 0 {
		t.Fatal("the nearest food is one it cannot eat")
	}
	hunger := a.Hunger
	w.eat(a, id)
	if a.Hunger != hunger || len(w.Foods()) != 1 {
		t.Fatal("ate something it cannot eat")
	}
}

// Meat that nobody eats has to go, or the world fills up with carcasses and
// nothing can grow.
func TestUneatenMeatSpoils(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	dead := w.agentByID(w.addAgent(Agent{X: 100, Y: 100, Vitality: 10, Species: Species(1)}))
	w.kill(dead)
	if len(w.Foods()) == 0 {
		t.Fatal("no carcass to spoil")
	}

	plant := w.addFood(200, 200)
	for i := 0; i < cfg.MeatSpoilTicks+1; i++ {
		w.Step()
	}
	if len(w.Foods()) != 1 || w.Foods()[0].ID != plant {
		t.Fatalf("food left after the meat spoiled: %v", w.Foods())
	}
}
