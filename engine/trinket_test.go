package engine

import "testing"

// trinketConfig is the money world with ornaments in it (stage 82).
func trinketConfig() Config {
	cfg := coinConfig()
	cfg.CarrySlotsWeigh = true
	cfg.Trinkets = true
	return cfg
}

// Nobody eats one, and nobody cooks one. This is the rule that was broken when
// the kind was added: the list of what is not food was enumerated, the new
// kind fell through it, and bodies made ornaments and had them for dinner.
func TestNobodyEatsAnOrnament(t *testing.T) {
	w := NewWorld(trinketConfig())
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Hunger: 85, Genome: genomeOf(50, 50, 50)})
	a := mustAgent(t, w, id)
	w.putFood(Food{X: 101, Y: 100, Kind: FoodTrinket, Made: 1})
	if w.canEat(a, w.foodByID(w.perceive(a).Trinkets[0].ID)) {
		t.Fatal("an ornament can be eaten")
	}
	p := w.perceive(a)
	if len(p.Foods) != 0 {
		t.Fatalf("a hungry body sees %d things to eat and the only thing there is an ornament", len(p.Foods))
	}
	if len(p.Trinkets) != 1 {
		t.Fatalf("%d ornaments in sight", len(p.Trinkets))
	}
	// Nor in the hand.
	a.carried = append(a.carried, Food{Kind: FoodTrinket, Made: 1})
	if got := len(w.perceive(mustAgent(t, w, id)).Foods); got != 0 {
		t.Fatalf("an ornament in the hand is %d meals", got)
	}
	if w.canCook(mustAgent(t, w, id)) {
		t.Fatal("an ornament can be cooked")
	}
}

// Making one costs the time and the vitality, and what comes out varies from
// piece to piece.
func TestMakingOneCostsAndVaries(t *testing.T) {
	cfg := trinketConfig()
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	worths := map[float64]bool{}
	for n := 0; n < 6; n++ {
		a := mustAgent(t, w, id)
		before := a.Vitality
		a.Action = Action{Kind: ActCraft}
		for i := 0; i < cfg.CraftTicks; i++ {
			a.actionTicks = i
			w.craft(a)
		}
		a.actionTicks = cfg.CraftTicks
		w.craft(a)
		if got := a.Vitality; got != before-cfg.CraftVitality {
			t.Fatalf("making one took %v vitality, and the price is %v", before-got, cfg.CraftVitality)
		}
		if len(a.carried) != n+1 {
			t.Fatalf("after %d makings the hand holds %d things", n+1, len(a.carried))
		}
		made := a.carried[len(a.carried)-1] // the one just finished
		if made.Kind != FoodTrinket {
			t.Fatalf("the making produced a %v", made.Kind)
		}
		worths[made.Made] = true
		if made.SpoilAt <= w.tick {
			t.Fatal("an ornament that is already gone")
		}
	}
	if len(worths) < 2 {
		t.Fatalf("six pieces by the same hand came out in %d kinds", len(worths))
	}
}

// What one is worth falls as it goes off, rather than stopping at a cliff.
func TestAnOrnamentIsWorthLessAsItGoes(t *testing.T) {
	cfg := trinketConfig()
	w := NewWorld(cfg)
	f := Food{Kind: FoodTrinket, Made: 1, SpoilAt: w.tick + cfg.TrinketSpoilTicks}
	fresh := w.trinketWorth(nil, &f)
	if fresh <= 0 {
		t.Fatalf("a new one is worth %v", fresh)
	}
	for i := 0; i < cfg.TrinketSpoilTicks/2; i++ {
		w.tick++
	}
	half := w.trinketWorth(nil, &f)
	if half >= fresh || half <= 0 {
		t.Fatalf("half way through its life one is worth %v against %v new", half, fresh)
	}
	for i := 0; i < cfg.TrinketSpoilTicks; i++ {
		w.tick++
	}
	if gone := w.trinketWorth(nil, &f); gone != 0 {
		t.Fatalf("one that is gone is worth %v", gone)
	}
}

// A world that has not asked for them has none, and nobody is ever offered the
// making of one.
func TestAWorldWithoutThemNeverOffersTheMaking(t *testing.T) {
	cfg := trinketConfig()
	cfg.Trinkets = false
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	a := mustAgent(t, w, id)
	if w.canCraft(a) {
		t.Fatal("a world with no ornaments in it offers the making of one")
	}
	if !w.TrackDecisions(id, true) {
		t.Fatalf("cannot follow agent %d", id)
	}
	w.Step()
	tr, ok := w.LastDecisionTrace(id)
	if !ok {
		t.Fatal("the body decided nothing")
	}
	for _, o := range tr.Options {
		if o.Action.Kind == ActCraft {
			t.Fatal("the making was scored in a world that has none")
		}
	}
}

// They do not count against what the world may grow, for the reason stones and
// money do not: a world given a lot of them would stop growing plants.
func TestOrnamentsDoNotCrowdOutThePlants(t *testing.T) {
	cfg := trinketConfig()
	w := NewWorld(cfg)
	for i := 0; i < cfg.MaxFoodItems; i++ {
		w.putFood(Food{X: float64(10 + i), Y: 50, Kind: FoodTrinket, Made: 1})
	}
	plants := w.countKind(FoodPlant)
	for i := 0; i < 400; i++ {
		w.spawnFood()
	}
	if got := w.countKind(FoodPlant); got <= plants {
		t.Fatalf("a world full of ornaments grew %d plants, from %d", got-plants, plants)
	}
}
