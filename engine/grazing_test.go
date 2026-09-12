package engine

import "testing"

// grazers is a one-row table whose enemy can eat what the humans live on.
func grazers(appetite float64) []EnemyKind {
	return []EnemyKind{{
		Name: "grazer", Share: 1, Homing: 1, Homely: 1,
		CanEatPlants: true, PlantAppetite: appetite,
	}}
}

// What a body cannot digest is a gate on the candidates; what is not worth
// eating is left to the comparison. Stage 25's division, applied to a mouth.
func TestWhatAnEnemyCanEatIsItsRowAndNotItsSpecies(t *testing.T) {
	cfg := quietConfig()
	plain := NewWorld(cfg)
	cfg.EnemyKinds = grazers(0.25)
	grazing := NewWorld(cfg)

	plant := Food{Kind: FoodPlant, X: 100, Y: 100}
	for _, w := range []*World{plain, grazing} {
		id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Species: SpeciesEnemy,
			Genome: filledGenome(50)})
		a := mustAgent(t, w, id)
		if got := w.canEat(a, &plant); got != (w == grazing) {
			t.Fatalf("an enemy that %s eat plants reports %v",
				map[bool]string{true: "can", false: "cannot"}[w == grazing], got)
		}
	}
}

// And the appetite is one multiplier, passed through by both the estimate and
// the mouthful - a body that expected more than it got would be a body lied
// to about itself.
func TestAPlantIsWorthLessToAnEnemyThanToAHuman(t *testing.T) {
	cfg := quietConfig()
	cfg.EnemyKinds = grazers(0.25)
	w := NewWorld(cfg)

	human := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 50, Y: 50,
		Genome: filledGenome(50)}))
	enemy := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 60, Y: 50,
		Species: SpeciesEnemy, Genome: filledGenome(50)}))

	hv := w.dietValue(human, FoodPlant)
	ev := w.dietValue(enemy, FoodPlant)
	if ev >= hv {
		t.Fatalf("a plant is worth %v to the enemy and %v to the human", ev, hv)
	}
	if got := ev / hv; got < 0.2 || got > 0.3 {
		t.Fatalf("the enemy's appetite came out at %v, want about a quarter", got)
	}
	// Meat is untouched: the row says nothing about it.
	if got := w.appetiteFor(enemy, FoodMeat); got != 1 {
		t.Fatalf("the row moved what meat is worth: %v", got)
	}
	// And the perception carries the same figure the mouthful will.
	if got := w.mealValues(enemy)[FoodPlant]; got != ev*w.meatWorth(FoodPlant) {
		t.Fatalf("the estimate says %v and the mouthful %v", got, ev)
	}
}

// No coin flip anywhere. The appetite moves the score and the comparison
// decides, so the same body at the same hunger eats a plant when it is worth
// a mouthful of meat and leaves it when it is worth a quarter of one - and
// the option is scored either way, which is what "not a gate" means.
func TestTheAppetiteMovesTheScoreAndNotTheCandidates(t *testing.T) {
	choose := func(appetite float64) (Action, *DecisionTrace) {
		cfg := testConfig()
		cfg.EnemyKinds = grazers(appetite)
		w := NewWorld(cfg)
		id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Species: SpeciesEnemy,
			Genome: filledGenome(50)})
		a := mustAgent(t, w, id)
		a.Hunger = w.cfg.MaxHunger * 0.95
		w.addFood(102, 100)
		p := w.perceive(a)
		trace := &DecisionTrace{}
		p.Trace = trace
		return w.ai.Decide(p), trace
	}

	full, fullTrace := choose(1)
	if full.Kind != ActEat {
		t.Fatalf("a starving grazer that likes plants chose %v with one at its feet", full.Kind)
	}
	picky, pickyTrace := choose(0.25)
	if picky.Kind == ActEat {
		t.Fatal("a grazer that thinks little of plants ate one it could afford to ignore")
	}
	// But it was offered, and scored lower rather than being kept off the list.
	eats := func(tr *DecisionTrace) (int, float64) {
		n, best := 0, 0.0
		for _, o := range tr.Options {
			if o.Action.Kind != ActEat {
				continue
			}
			n++
			if o.Score > best {
				best = o.Score
			}
		}
		return n, best
	}
	nFull, scoreFull := eats(fullTrace)
	nPicky, scorePicky := eats(pickyTrace)
	if nPicky == 0 || nPicky != nFull {
		t.Fatalf("the picky one was offered %d meals and the other %d", nPicky, nFull)
	}
	if scorePicky >= scoreFull {
		t.Fatalf("thinking less of a plant scored it %v against %v", scorePicky, scoreFull)
	}
}

// A world whose table says nothing about plants is the world as it was, down
// to the values taken from the random source.
func TestNoAppetiteLeavesTheWorldExactlyAsItWas(t *testing.T) {
	run := func(kinds []EnemyKind) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 73
		cfg.EnemyKinds = kinds
		w := NewWorld(cfg)
		for i := 0; i < 1500; i++ {
			w.Step()
		}
		return w.Stats()
	}
	plain := run([]EnemyKind{{Name: "enemy", Share: 1, Homing: 1, Homely: 1}})
	if plain != run(nil) {
		t.Fatal("spelling out the only sort changed the run")
	}
	if grazing := run(grazers(0.25)); grazing == plain {
		t.Fatal("letting the enemies eat plants changed nothing at all")
	}
}
