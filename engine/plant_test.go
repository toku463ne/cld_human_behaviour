package engine

import (
	"math"
	"testing"
)

// The arm this stage is measured against: with the plants' inheritance off,
// the world is the one stage 15a left, down to the values drawn from the
// random source.
func TestNoPlantGeneticsLeavesTheWorldExactlyAsItWas(t *testing.T) {
	run := func(on bool) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 13
		cfg.PlantGenetics = on
		w := NewWorld(cfg)
		for i := 0; i < 1500; i++ {
			w.Step()
		}
		return w.Stats()
	}
	if off, again := run(false), run(false); off != again {
		t.Fatal("the same world twice gave different runs")
	} else if on := run(true); on == off {
		t.Fatal("giving the plants an inheritance changed nothing at all")
	}
}

// How many plants there are is still FoodSpawnRate's business. The genes decide
// whose children they are and where they land, and nothing else - the same
// separation stage 15a made between how much food there is and where it is.
func TestPlantGenesMoveTheFoodWithoutChangingHowMuchThereIs(t *testing.T) {
	count := func(on bool) int {
		cfg := DefaultConfig()
		cfg.Seed = 14
		cfg.PlantGenetics = on
		cfg.MaxFoodItems = 100000
		cfg.InitialPopulation, cfg.InitialEnemies, cfg.InitialFoodItems = 0, 0, 1
		w := NewWorld(cfg)
		for i := 0; i < 3000; i++ {
			w.Step()
		}
		// Plants only. Carcasses are counted separately because the enemies
		// that walk in from off the map die at different moments in the two
		// runs, and how much meat is lying about says nothing about how many
		// plants grew.
		plants := 0
		for _, f := range w.Foods() {
			if f.Kind == FoodPlant {
				plants++
			}
		}
		return plants
	}
	// Exactly the same, not approximately: FoodSpawnRate says how many and
	// nothing in this stage touches it.
	if fixed, evolving := count(false), count(true); fixed != evolving {
		t.Fatalf("a fixed world grew %d plants and an evolving one %d, want exactly the same number",
			fixed, evolving)
	}
}

// A seedling is its parent, most of the time. Rare and large, as the agents'
// inheritance is: a nudge on every seed would leave nothing of the parent in
// the child.
func TestASeedlingIsItsParentMostOfTheTime(t *testing.T) {
	cfg := testConfig()
	// The dispersal genes alone: the defences are a separate rule with a
	// separate switch, and mutating four genes instead of two would make
	// "unchanged" mean something else.
	cfg.PlantGenetics, cfg.PlantDefence = true, false
	w := NewWorld(cfg)
	parent := plantGenes{Spread: 100, Regrow: 1}

	same, moved := 0, 0
	for i := 0; i < 2000; i++ {
		child := w.inheritPlantGenes(parent)
		if child == parent {
			same++
		} else {
			moved++
		}
	}
	if share := float64(same) / 2000; share < 0.85 {
		t.Fatalf("only %.0f%% of seedlings were their parent unchanged, want the great majority", share*100)
	}
	if moved == 0 {
		t.Fatal("no seedling ever differed from its parent")
	}
	// And the jumps, when they come, are worth having.
	big := 0
	for i := 0; i < 20000; i++ {
		if c := w.inheritPlantGenes(parent); math.Abs(c.Spread-parent.Spread) > parent.Spread*0.2 {
			big++
		}
	}
	if big == 0 {
		t.Fatal("no mutation ever moved the spread by as much as a fifth")
	}
}

// Plants seed in proportion to how readily they seed, which is what makes
// Regrow a fitness rather than a subsidy: a plant that seeds twice as readily
// takes the place of one that does not, without the world growing more food.
func TestPlantsSeedInProportionToHowReadilyTheySeed(t *testing.T) {
	cfg := testConfig()
	cfg.FoodSpread = 0 // even ground, so only the gene is speaking
	w := NewWorld(cfg)

	eager := w.addPlant(100, 100, plantGenes{Spread: 5, Regrow: 3})
	idle := w.addPlant(300, 300, plantGenes{Spread: 5, Regrow: 0.3})
	_, _ = eager, idle

	eagerPicks, idlePicks := 0, 0
	for i := 0; i < 4000; i++ {
		switch w.seedFrom() {
		case 0:
			eagerPicks++
		case 1:
			idlePicks++
		}
	}
	if eagerPicks <= idlePicks*3 {
		t.Fatalf("the eager plant was picked %d times and the idle one %d, want roughly ten to one",
			eagerPicks, idlePicks)
	}
}

// A world that has lost its last plant has to be able to grow another. Nothing
// can seed where nothing grows, so without a fallback an empty world is an
// absorbing state - a rule nobody chose.
func TestAWorldWithNoPlantsLeftCanStartAgain(t *testing.T) {
	cfg := testConfig()
	cfg.FoodSpawnRate = 1
	w := NewWorld(cfg)
	if got := w.seedFrom(); got != -1 {
		t.Fatalf("an empty world offered plant %d to seed from", got)
	}
	w.spawnFood()
	if len(w.Foods()) != 1 {
		t.Fatalf("an empty world grew %d plants, want it to start again from the ground", len(w.Foods()))
	}
}

// Dispersal makes thickets, and that is the shape the spread gene exists to
// argue about. Seeds thrown a short way pile up where their parent stood; seeds
// thrown far land where the parent never was.
func TestShortDispersalMakesThicketsAndLongDispersalDoesNot(t *testing.T) {
	clumping := func(spread float64) float64 {
		cfg := DefaultConfig()
		cfg.Seed = 15
		cfg.InitialPopulation, cfg.InitialEnemies = 0, 0
		cfg.InitialFoodItems = 40
		cfg.MaxFoodItems = 300
		cfg.FoodSpread = 0
		cfg.PlantMutationRate = 0 // hold the gene still and watch the shape
		cfg.PlantSpread = spread
		w := NewWorld(cfg)
		// Every plant starts on the gene under test.
		for i := range w.foods {
			w.foods[i].Genes = plantGenes{Spread: spread, Regrow: 1}
		}
		for i := 0; i < 4000; i++ {
			w.Step()
		}
		return w.Plants().Clumping
	}
	near, far := clumping(30), clumping(400)
	if near <= far {
		t.Fatalf("seeds thrown 30 clumped at %.2f and seeds thrown 400 at %.2f, want the short throw to make thickets",
			near, far)
	}
}

// The arm for the defences, separately from the dispersal: with them off the
// world is the one from before them, down to the values drawn.
func TestNoPlantDefenceLeavesTheWorldExactlyAsItWas(t *testing.T) {
	run := func(on bool) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 16
		cfg.PlantDefence = on
		w := NewWorld(cfg)
		for i := 0; i < 1500; i++ {
			w.Step()
		}
		return w.Stats()
	}
	if off, again := run(false), run(false); off != again {
		t.Fatal("the same world twice gave different runs")
	} else if on := run(true); on == off {
		t.Fatal("arming the plants changed nothing at all")
	}
}

// Poison is hidden and the warning is not. What reaches a decision is the
// signal, read with the error the reader's rationality leaves - never the
// poison itself, exactly as an agent's combat power never reaches one.
func TestPoisonIsHiddenAndTheWarningIsNot(t *testing.T) {
	cfg := quietConfig()
	cfg.PlantDefence = true
	cfg.SignalNoise = 0
	w := NewWorld(cfg)

	sharp := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 50, Y: 50, Vitality: 80,
		Genome: genomeOf(50, MaxAbility, 50)}))

	// A plant that means it, and a liar: same warning, different poison.
	honest := Food{Kind: FoodPlant, Genes: plantGenes{Poison: 0.9, Signal: 0.9}}
	liar := Food{Kind: FoodPlant, Genes: plantGenes{Poison: 0, Signal: 0.9}}
	quiet := Food{Kind: FoodPlant, Genes: plantGenes{Poison: 0.9, Signal: 0}}

	if a, b := w.dangerOf(sharp, &honest), w.dangerOf(sharp, &liar); a != b {
		t.Fatalf("an honest plant read %v and a liar with the same warning %v; the poison is leaking through", a, b)
	}
	if got := w.dangerOf(sharp, &quiet); got != 0 {
		t.Fatalf("a silent but poisonous plant read %v of danger, want 0 - it says nothing", got)
	}
}

// Reading the warning is what rationality buys, which is its first job on the
// food's side of the world.
func TestRationalityReadsTheWarningBetter(t *testing.T) {
	cfg := quietConfig()
	cfg.PlantDefence = true
	w := NewWorld(cfg)

	f := Food{Kind: FoodPlant, Genes: plantGenes{Poison: 0.8, Signal: 0.8}}
	err := func(rationality float64) float64 {
		a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 50, Y: 50, Vitality: 80,
			Genome: genomeOf(50, rationality, 50)}))
		total := 0.0
		for i := 0; i < 4000; i++ {
			total += math.Abs(w.dangerOf(a, &f) - f.Genes.Signal)
		}
		return total / 4000
	}
	dull, sharp := err(MinAbility), err(MaxAbility)
	if sharp >= dull {
		t.Fatalf("a rational reader was out by %v and a dull one by %v, want the rational one closer", sharp, dull)
	}
}

// Eating a poisonous plant costs what the plant actually holds, not what it
// said. That is where an agent finds out it was lied to.
func TestEatingAPoisonousPlantCostsWhatItActuallyHolds(t *testing.T) {
	cfg := quietConfig()
	cfg.PlantDefence = true
	w := NewWorld(cfg)

	eat := func(poison, signal float64) float64 {
		a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80,
			Hunger: 90, Genome: genomeOf(50, 50, 50)}))
		id := w.addPlant(101, 100, plantGenes{Poison: poison, Signal: signal})
		before := a.Vitality
		w.eat(a, id)
		return before - a.Vitality
	}

	harmless := eat(0, 0)
	if harmless != 0 {
		t.Fatalf("a harmless plant cost %v of vitality, want none", harmless)
	}
	// The liar costs nothing, however loudly it shouted.
	if got := eat(0, 1); got != 0 {
		t.Fatalf("a plant that only claimed to be poisonous cost %v", got)
	}
	// And the quiet poisonous one costs the full dose, having said nothing.
	if got, want := eat(1, 0), cfg.PoisonDamage; math.Abs(got-want) > 1e-9 {
		t.Fatalf("a silent poisonous plant cost %v of vitality, want the full %v", got, want)
	}
}

// Nothing ties the two genes together. Honesty is something the world may
// arrive at, and the measurement has to be able to report that it has not.
func TestPoisonAndWarningAreDrawnAndMutatedIndependently(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 17
	cfg.PlantDefence = true
	w := NewWorld(cfg)

	// The draw. A whole crop grown in a world would be correlated through
	// shared ancestry however independent the rule is, so the claim - nothing
	// ties the two together - is tested where it is made.
	corr := func(get func() (float64, float64)) float64 {
		var n, sx, sy, sxx, syy, sxy float64
		for i := 0; i < 20000; i++ {
			x, y := get()
			n, sx, sy = n+1, sx+x, sy+y
			sxx, syy, sxy = sxx+x*x, syy+y*y, sxy+x*y
		}
		den := math.Sqrt((n*sxx - sx*sx) * (n*syy - sy*sy))
		if den == 0 {
			return 0
		}
		return (n*sxy - sx*sy) / den
	}

	drawn := corr(func() (float64, float64) {
		g := w.drawPlantGenes()
		return g.Poison, g.Signal
	})
	if math.Abs(drawn) > 0.03 {
		t.Fatalf("the two genes are drawn %v correlated, want nothing tying them together", drawn)
	}

	// And the mutation. A parent that is honest must be as likely to have a
	// lying child as a truthful one.
	cfg2 := w.cfg
	cfg2.PlantMutationRate = 1
	w.cfg = cfg2
	parent := plantGenes{Poison: 0.5, Signal: 0.5}
	moved := corr(func() (float64, float64) {
		c := w.inheritPlantGenes(parent)
		return c.Poison - parent.Poison, c.Signal - parent.Signal
	})
	if math.Abs(moved) > 0.03 {
		t.Fatalf("mutations move the two genes %v together, want them independent", moved)
	}
}
