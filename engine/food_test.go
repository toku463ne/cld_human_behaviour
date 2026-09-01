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

// An enemy is the same creature run by the same rules: what differs is the
// range its budget comes from and what it can digest.
func TestAnEnemyIsANodeWithADifferentBudgetAndDiet(t *testing.T) {
	cfg := testConfig()
	cfg.InitialPopulation = 200
	cfg.InitialEnemies = 200
	w := NewWorld(cfg)

	var human, enemy []float64
	for _, a := range w.Agents() {
		if a.Species == SpeciesEnemy {
			enemy = append(enemy, a.Budget())
		} else {
			human = append(human, a.Budget())
		}
	}
	if len(human) != 200 || len(enemy) != 200 {
		t.Fatalf("founded %d humans and %d enemies, want 200 of each", len(human), len(enemy))
	}
	mean := func(xs []float64) float64 {
		var s float64
		for _, x := range xs {
			s += x
		}
		return s / float64(len(xs))
	}
	if mean(enemy) <= mean(human) {
		t.Fatalf("enemies average %.0f of budget against the humans' %.0f: they are not the larger kind",
			mean(enemy), mean(human))
	}
	if !eatsPlants(SpeciesHuman) || eatsPlants(SpeciesEnemy) {
		t.Fatal("the diets are the wrong way round: enemies must have to hunt")
	}
}

// Prey is worth killing for what it leaves. Nothing says "hunt"; the carcass
// simply turns up in the same stake term as a contested meal.
func TestPreyIsWorthKillingForTheCarcass(t *testing.T) {
	cfg := testConfig()
	cfg.MaxPopulation = 100
	w := NewWorld(cfg)
	hunter := w.addAgent(Agent{X: 200, Y: 200, Vitality: 90, Hunger: 80,
		Genome: filledGenome(60), Species: SpeciesEnemy})
	prey := w.addAgent(Agent{X: 215, Y: 200, Vitality: 30, Genome: filledGenome(40)})

	p := w.perceive(mustAgent(t, w, hunter))
	var view *AgentView
	for i := range p.Others {
		if p.Others[i].ID == prey {
			view = &p.Others[i]
		}
	}
	if view == nil {
		t.Fatal("the hunter cannot see the prey")
	}
	if !view.Prey || view.Meat <= 0 {
		t.Fatalf("prey %v meat %.1f: a creature of another kind is not being seen as food", view.Prey, view.Meat)
	}

	// The same situation between two of a kind is not a hunt.
	sibling := w.addAgent(Agent{X: 185, Y: 200, Vitality: 30,
		Genome: filledGenome(40), Species: SpeciesEnemy})
	p = w.perceive(mustAgent(t, w, hunter))
	for i := range p.Others {
		if p.Others[i].ID == sibling && p.Others[i].Prey {
			t.Fatal("an enemy sees its own kind as food")
		}
	}
}

// Courting stops at the species boundary, on both sides of the decision.
func TestNobodyCourtsAnotherSpecies(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	man := w.addAgent(Agent{X: 200, Y: 200, Sex: Male, Hunger: 0,
		Genome: genomeOf(40, 100, 100)})
	w.agentByID(man).Vitality = w.agentByID(man).MaxVitality(&cfg)
	she := w.addAgent(Agent{X: 205, Y: 200, Sex: Female, Hunger: 0,
		Genome: genomeOf(40, 100, 100), Species: SpeciesEnemy})
	w.agentByID(she).Vitality = w.agentByID(she).MaxVitality(&cfg)

	for i := 0; i < 400; i++ {
		w.Step()
		if mustAgent(t, w, man).PartnerID != 0 || mustAgent(t, w, she).PartnerID != 0 {
			t.Fatal("a pair formed across the species boundary")
		}
	}
}
