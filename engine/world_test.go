package engine

import (
	"math"
	"testing"
)

// testConfig returns a world that does nothing on its own: no initial
// population, no food growth. Each test builds exactly the situation it needs.
func testConfig() Config {
	cfg := DefaultConfig()
	cfg.Seed = 12345
	cfg.Width, cfg.Height = 400, 400
	cfg.InitialPopulation = 0
	cfg.InitialFoodItems = 0
	cfg.FoodSpawnRate = 0
	return cfg
}

func mustAgent(t *testing.T, w *World, id int) *Agent {
	t.Helper()
	a := w.agentByID(id)
	if a == nil {
		t.Fatalf("agent %d not found", id)
	}
	return a
}

func approx(t *testing.T, got, want, tol float64, what string) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s = %v, want %v (+/- %v)", what, got, want, tol)
	}
}

// --- determinism -----------------------------------------------------------

func TestSameSeedGivesSameRun(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 42

	a, b := NewWorld(cfg), NewWorld(cfg)
	for i := 0; i < 500; i++ {
		a.Step()
		b.Step()
	}

	if a.Stats() != b.Stats() {
		t.Fatalf("stats diverged:\n a = %+v\n b = %+v", a.Stats(), b.Stats())
	}
	for i := range a.Agents() {
		x, y := a.Agents()[i], b.Agents()[i]
		if x.ID != y.ID || x.X != y.X || x.Y != y.Y || x.Power != y.Power {
			t.Fatalf("agent %d diverged: %+v vs %+v", i, x, y)
		}
	}
}

func TestDifferentSeedsGiveDifferentRuns(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 1
	a := NewWorld(cfg)
	cfg.Seed = 2
	b := NewWorld(cfg)
	for i := 0; i < 200; i++ {
		a.Step()
		b.Step()
	}
	if a.Stats() == b.Stats() {
		t.Fatal("two different seeds produced identical statistics")
	}
}

// --- survival --------------------------------------------------------------

func TestStarvationKillsAgent(t *testing.T) {
	w := NewWorld(testConfig())
	id := w.addAgent(Agent{X: 100, Y: 100, Power: 50, Rationality: 50, Food: 0.01})

	w.Step()

	if _, ok := w.AgentByID(id); ok {
		t.Fatal("starved agent is still in the world")
	}
	if got := w.Stats().Deaths; got != 1 {
		t.Fatalf("deaths = %d, want 1", got)
	}
	if got := w.Stats().Population; got != 0 {
		t.Fatalf("population = %d, want 0", got)
	}
}

func TestOldAgeKillsAgent(t *testing.T) {
	w := NewWorld(testConfig())
	id := w.addAgent(Agent{X: 100, Y: 100, Power: 50, Rationality: 50, Food: 100, Age: 4, MaxAge: 5})

	w.Step()

	if _, ok := w.AgentByID(id); ok {
		t.Fatal("agent past its lifespan is still alive")
	}
}

// --- foraging and contests -------------------------------------------------

func TestLoneAgentEatsFood(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	id := w.addAgent(Agent{X: 100, Y: 100, Power: 50, Rationality: 50, Food: 20})
	w.addFood(105, 100)

	w.Step()

	if got := len(w.Foods()); got != 0 {
		t.Fatalf("food items left = %d, want 0", got)
	}
	approx(t, mustAgent(t, w, id).Food, 20-cfg.Metabolism+cfg.FoodNutrition, 1e-9, "food after eating")
}

func TestStrongerAgentWinsContest(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	strong := w.addAgent(Agent{X: 100, Y: 100, Sex: Male, Power: 90, Rationality: 90, Food: 20})
	weak := w.addAgent(Agent{X: 103, Y: 100, Sex: Male, Power: 10, Rationality: 90, Food: 20})
	w.addFood(105, 100)

	w.Step()

	if got := len(w.Foods()); got != 0 {
		t.Fatalf("food items left = %d, want 0 (the contest should have been settled)", got)
	}
	approx(t, mustAgent(t, w, strong).Food, 20-cfg.Metabolism+cfg.FoodNutrition, 1e-9, "winner food")
	if got := mustAgent(t, w, weak).Food; got >= 20 {
		t.Fatalf("loser food = %v, want less than its starting 20", got)
	}
}

// A rival that is clearly out of reach is not fought: the agent walks away and
// looks for food somewhere else.
func TestWeakAgentAvoidsHopelessContest(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	// Rationality 100 removes the judgement error, making the decision exact.
	weak := w.addAgent(Agent{X: 100, Y: 100, Sex: Male, Power: 10, Rationality: 100, Food: 20})
	w.addAgent(Agent{X: 103, Y: 100, Sex: Male, Power: 90, Rationality: 100, Food: 100})
	foodID := w.addFood(105, 100)

	w.Step()

	if got := len(w.Foods()); got != 1 {
		t.Fatalf("food items left = %d, want 1 (nobody should have taken it)", got)
	}
	a := mustAgent(t, w, weak)
	approx(t, a.Food, 20-cfg.Metabolism, 1e-9, "food of the agent that walked away")
	if !a.isRejected(rejectFood, foodID) {
		t.Fatal("the contested food was not put aside")
	}
}

// Holding resources is an advantage of its own in a contest.
func TestStoredFoodStrengthensInContest(t *testing.T) {
	w := NewWorld(testConfig())
	poor := &Agent{Power: 50, Food: 0}
	rich := &Agent{Power: 50, Food: 100}

	if w.effectiveContestPower(rich) <= w.effectiveContestPower(poor) {
		t.Fatal("stored food gave no advantage in a contest")
	}
	approx(t, w.effectiveContestPower(rich), 50+100*contestFoodWeight, 1e-9, "contest power with resources")
}

// Rationality is an ability of its own: it is what makes the estimate of a
// rival accurate.
func TestRationalityMakesJudgementAccurate(t *testing.T) {
	w := NewWorld(testConfig())
	rival := &Agent{Power: 50, Food: 0}
	sharp := &Agent{Rationality: 95}
	dull := &Agent{Rationality: 5}
	truth := w.effectiveContestPower(rival)

	var sharpError, dullError float64
	for i := 0; i < 2000; i++ {
		sharpError += math.Abs(w.perceivedPower(sharp, rival) - truth)
		dullError += math.Abs(w.perceivedPower(dull, rival) - truth)
	}

	if sharpError*3 >= dullError {
		t.Fatalf("rational agent was not clearly more accurate: %v vs %v", sharpError, dullError)
	}
}

// The flip side of the rule above: a rash agent picks fights it cannot win.
func TestLowRationalityPicksHopelessFights(t *testing.T) {
	fights := func(rationality float64) int {
		count := 0
		for i := 0; i < 300; i++ {
			cfg := testConfig()
			cfg.Seed = int64(1000 + i)
			w := NewWorld(cfg)
			weak := w.addAgent(Agent{X: 100, Y: 100, Sex: Male, Power: 40, Rationality: rationality, Food: 20})
			w.addAgent(Agent{X: 103, Y: 100, Sex: Male, Power: 60, Rationality: 100, Food: 100})
			foodID := w.addFood(105, 100)

			w.Step()

			// Not putting the food aside means the agent engaged the rival.
			if !mustAgent(t, w, weak).isRejected(rejectFood, foodID) {
				count++
			}
		}
		return count
	}

	rash, careful := fights(5), fights(95)
	if careful != 0 {
		t.Fatalf("a highly rational agent entered %d hopeless contests, want 0", careful)
	}
	if rash < 10 {
		t.Fatalf("a low rationality agent entered only %d contests, expected it to misjudge far more often", rash)
	}
}

// --- survival comes before reproduction ------------------------------------

func TestSurvivalTakesPriorityOverMating(t *testing.T) {
	cfg := testConfig()

	seekMate := func(food float64) bool {
		w := NewWorld(cfg)
		id := w.addAgent(Agent{X: 200, Y: 200, Sex: Male, Power: 50, Rationality: 50, Food: food})
		w.addAgent(Agent{X: 210, Y: 200, Sex: Female, Power: 90, Rationality: 90, Food: 100})
		w.addFood(300, 300)
		w.Step()
		return mustAgent(t, w, id).State == StateSeekMate
	}

	// Hungry: the attractive candidate right next to it changes nothing.
	if seekMate(cfg.FoodLowThreshold - 1) {
		t.Fatal("a hungry agent went looking for a mate instead of food")
	}
	// Well fed: reproduction becomes possible.
	if !seekMate(cfg.ReproFoodThreshold + 10) {
		t.Fatal("a well fed agent did not start looking for a mate")
	}
}

// --- mate choice -----------------------------------------------------------

func TestMateChoicePrefersFitterCandidate(t *testing.T) {
	w := NewWorld(testConfig())
	// Rationality 100 removes the judgement error, so the choice is exact.
	id := w.addAgent(Agent{X: 200, Y: 200, Sex: Male, Power: 50, Rationality: 100, Food: 80})
	w.addAgent(Agent{X: 200, Y: 150, Sex: Female, Power: 90, Rationality: 90, Food: 100}) // fitter
	w.addAgent(Agent{X: 200, Y: 250, Sex: Female, Power: 10, Rationality: 10, Food: 62})

	w.Step()

	a := mustAgent(t, w, id)
	if a.State != StateSeekMate {
		t.Fatalf("state = %v, want seek_mate", a.State)
	}
	if a.Y >= 200 {
		t.Fatalf("y = %v, want the agent to have moved towards the fitter candidate at y=150", a.Y)
	}
}

func TestPatienceGrowsWithRationality(t *testing.T) {
	w := NewWorld(testConfig())
	rash := &Agent{Rationality: 10}
	careful := &Agent{Rationality: 90}

	if w.patienceTicks(careful) <= w.patienceTicks(rash) {
		t.Fatalf("patience: careful = %d, rash = %d, want the rational agent to compare longer",
			w.patienceTicks(careful), w.patienceTicks(rash))
	}
}

// courting builds two agents standing next to each other, both able to
// reproduce and both unremarkable enough that neither is an obvious catch.
func courting(t *testing.T) (*World, int, int) {
	t.Helper()
	w := NewWorld(testConfig())
	male := w.addAgent(Agent{X: 200, Y: 200, Sex: Male, Power: 40, Rationality: 100, Food: 100})
	female := w.addAgent(Agent{X: 205, Y: 200, Sex: Female, Power: 40, Rationality: 100, Food: 100})
	if f := fitness(mustAgent(t, w, male)); f >= testConfig().CommitFitness {
		t.Fatalf("test setup: fitness %v is an obvious catch, the pair would form instantly", f)
	}
	return w, male, female
}

func TestAgentsDoNotPairInstantly(t *testing.T) {
	w, male, female := courting(t)

	// Two ticks: one to enter the mate seeking state, one to meet as such.
	w.Step()
	w.Step()

	for _, id := range []int{male, female} {
		a := mustAgent(t, w, id)
		if a.State != StateSeekMate {
			t.Fatalf("agent %d state = %v, want seek_mate", id, a.State)
		}
		if a.PartnerID != 0 {
			t.Fatalf("agent %d committed to a partner immediately", id)
		}
	}
}

func TestPairFormsOnceBothHaveCompared(t *testing.T) {
	w, male, female := courting(t)

	paired := -1
	for i := 0; i < 500 && paired < 0; i++ {
		w.Step()
		if mustAgent(t, w, male).State == StatePaired && mustAgent(t, w, female).State == StatePaired {
			paired = w.Tick()
		}
	}

	if paired < 0 {
		t.Fatal("the pair never formed")
	}
	a := mustAgent(t, w, male)
	if patience := w.patienceTicks(a); paired < patience {
		t.Fatalf("pair formed at tick %d, before the %d ticks of comparison", paired, patience)
	}
	if a.PartnerID != female || mustAgent(t, w, female).PartnerID != male {
		t.Fatal("the pair is not mutual")
	}
	if a.PairTimer <= 0 {
		t.Fatal("the pair has no time left to raise a child")
	}
}

// --- birth, inheritance and mutation ---------------------------------------

// pairAboutToGiveBirth returns a world with a bonded couple whose bond ends on
// the next step.
func pairAboutToGiveBirth(t *testing.T, cfg Config) (*World, int, int) {
	t.Helper()
	w := NewWorld(cfg)
	male := w.addAgent(Agent{
		X: 100, Y: 100, Sex: Male, Power: 40, Rationality: 60, Food: 50,
		State: StatePaired, PairTimer: 1, Generation: 2,
	})
	female := w.addAgent(Agent{
		X: 110, Y: 100, Sex: Female, Power: 60, Rationality: 80, Food: 50,
		State: StatePaired, PairTimer: 1, Generation: 5,
	})
	w.agentByID(male).PartnerID = female
	w.agentByID(female).PartnerID = male
	return w, male, female
}

func TestChildInheritsParentsAverage(t *testing.T) {
	cfg := testConfig()
	cfg.MutationStd = 0 // isolate inheritance from mutation
	w, male, female := pairAboutToGiveBirth(t, cfg)

	w.Step()

	if got := w.Stats().Births; got != 1 {
		t.Fatalf("births = %d, want 1", got)
	}
	var child *Agent
	for i := range w.Agents() {
		if a := &w.Agents()[i]; a.ID != male && a.ID != female {
			child = a
		}
	}
	if child == nil {
		t.Fatal("no child was born")
	}

	approx(t, child.Power, 50, 1e-9, "child power")             // (40 + 60) / 2
	approx(t, child.Rationality, 70, 1e-9, "child rationality") // (60 + 80) / 2
	approx(t, child.Food, cfg.ChildInitialFood, 1e-9, "child food")
	if child.Generation != 6 { // max(2, 5) + 1
		t.Fatalf("child generation = %d, want 6", child.Generation)
	}
	if w.Stats().MaxGeneration != 6 {
		t.Fatalf("max generation = %d, want 6", w.Stats().MaxGeneration)
	}

	// Raising the child costs both parents half of the birth cost.
	want := 50 - cfg.BirthCost/2 - cfg.Metabolism
	for _, id := range []int{male, female} {
		p := mustAgent(t, w, id)
		approx(t, p.Food, want, 1e-9, "parent food")
		if p.State != StateForage || p.PartnerID != 0 {
			t.Fatalf("parent %d was not released from the bond: %+v", id, p)
		}
		if p.CooldownTimer <= 0 {
			t.Fatalf("parent %d got no cooldown after reproducing", id)
		}
	}
}

func TestMutationVariesChildAbility(t *testing.T) {
	cfg := testConfig()
	cfg.MutationStd = 4
	w := NewWorld(cfg)
	pa := &Agent{Power: 50, Rationality: 50, Food: 100}
	pb := &Agent{Power: 50, Rationality: 50, Food: 100}

	varied := false
	for i := 0; i < 50; i++ {
		pa.Food, pb.Food = 100, 100
		w.tryBirth(pa, pb)
	}
	if len(w.newborns) != 50 {
		t.Fatalf("newborns = %d, want 50", len(w.newborns))
	}
	for i := range w.newborns {
		if w.newborns[i].Power != 50 {
			varied = true
		}
	}
	if !varied {
		t.Fatal("no child differed from the parents' average, mutation is not applied")
	}
}

func TestChildAbilityStaysInRange(t *testing.T) {
	cfg := testConfig()
	cfg.MutationStd = 50 // extreme, so the bounds are actually hit
	w := NewWorld(cfg)
	pa := &Agent{Power: MaxAbility, Rationality: MinAbility, Food: 100}
	pb := &Agent{Power: MaxAbility, Rationality: MinAbility, Food: 100}

	for i := 0; i < 300; i++ {
		pa.Food, pb.Food = 100, 100
		w.tryBirth(pa, pb)
	}
	for i := range w.newborns {
		c := &w.newborns[i]
		if c.Power < MinAbility || c.Power > MaxAbility {
			t.Fatalf("child power %v is out of range", c.Power)
		}
		if c.Rationality < MinAbility || c.Rationality > MaxAbility {
			t.Fatalf("child rationality %v is out of range", c.Rationality)
		}
	}
}

func TestBirthNeedsEnoughFood(t *testing.T) {
	cfg := testConfig()
	w, male, female := pairAboutToGiveBirth(t, cfg)
	mustAgent(t, w, male).Food = cfg.BirthCost / 4
	mustAgent(t, w, female).Food = cfg.BirthCost / 4

	w.Step()

	if got := w.Stats().Births; got != 0 {
		t.Fatalf("births = %d, want 0: the parents could not afford a child", got)
	}
}

func TestPopulationCapStopsBirths(t *testing.T) {
	cfg := testConfig()
	cfg.MaxPopulation = 2
	w, _, _ := pairAboutToGiveBirth(t, cfg)

	w.Step()

	if got := w.Stats().Population; got != 2 {
		t.Fatalf("population = %d, want it capped at 2", got)
	}
	if got := w.Stats().Births; got != 0 {
		t.Fatalf("births = %d, want 0", got)
	}
}

func TestPartnerDeathReleasesSurvivor(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	survivor := w.addAgent(Agent{
		X: 100, Y: 100, Sex: Male, Power: 50, Rationality: 50, Food: 80,
		State: StatePaired, PairTimer: 100,
	})
	dying := w.addAgent(Agent{
		X: 110, Y: 100, Sex: Female, Power: 50, Rationality: 50, Food: 0.01,
		State: StatePaired, PairTimer: 100,
	})
	w.agentByID(survivor).PartnerID = dying
	w.agentByID(dying).PartnerID = survivor

	w.Step()

	a := mustAgent(t, w, survivor)
	if a.State != StateForage {
		t.Fatalf("state = %v, want forage after losing a partner", a.State)
	}
	if a.PartnerID != 0 {
		t.Fatalf("partner id = %d, want 0", a.PartnerID)
	}
	if a.CooldownTimer != cfg.MatingCooldown/2 {
		t.Fatalf("cooldown = %d, want %d", a.CooldownTimer, cfg.MatingCooldown/2)
	}
}

// --- food supply -----------------------------------------------------------

func TestFoodSpawnRateAndCap(t *testing.T) {
	cfg := testConfig()
	cfg.FoodSpawnRate = 2
	cfg.MaxFoodItems = 10
	w := NewWorld(cfg)

	for i := 0; i < 3; i++ {
		w.Step()
	}
	if got := len(w.Foods()); got != 6 {
		t.Fatalf("food items = %d, want 6 after 3 ticks at rate 2", got)
	}

	for i := 0; i < 20; i++ {
		w.Step()
	}
	if got := len(w.Foods()); got != cfg.MaxFoodItems {
		t.Fatalf("food items = %d, want the cap %d", got, cfg.MaxFoodItems)
	}
}

func TestFractionalFoodRateAccumulates(t *testing.T) {
	cfg := testConfig()
	cfg.FoodSpawnRate = 0.5
	w := NewWorld(cfg)

	for i := 0; i < 10; i++ {
		w.Step()
	}
	if got := len(w.Foods()); got != 5 {
		t.Fatalf("food items = %d, want 5 after 10 ticks at rate 0.5", got)
	}
}

// --- whole simulation ------------------------------------------------------

// A default world must be able to run on its own: agents forage, pair up and
// raise children, and the population evolves over generations.
func TestDefaultWorldReachesNewGenerations(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 7
	w := NewWorld(cfg)

	for i := 0; i < 3000; i++ {
		w.Step()
		s := w.Stats()
		if s.Population > cfg.MaxPopulation {
			t.Fatalf("population %d exceeded the cap %d at tick %d", s.Population, cfg.MaxPopulation, s.Tick)
		}
		if len(w.Foods()) > cfg.MaxFoodItems {
			t.Fatalf("food items %d exceeded the cap %d at tick %d", len(w.Foods()), cfg.MaxFoodItems, s.Tick)
		}
	}

	s := w.Stats()
	if s.Births == 0 {
		t.Fatal("no child was ever born in a default world")
	}
	if s.MaxGeneration < 1 {
		t.Fatalf("max generation = %d, want at least 1", s.MaxGeneration)
	}
	if s.Population == 0 {
		t.Fatal("the population died out entirely")
	}
	if s.Males+s.Females != s.Population {
		t.Fatalf("sex counts %d + %d do not add up to the population %d", s.Males, s.Females, s.Population)
	}
}
