package engine

import (
	"math"
	"math/rand"
	"slices"
	"sort"
	"testing"
)

// testConfig returns a world that does nothing on its own: no initial
// population, no food growth. Each test builds exactly the situation it needs
// and switches off whichever rules would otherwise get in the way.
func testConfig() Config {
	cfg := DefaultConfig()
	cfg.Seed = 12345
	cfg.Width, cfg.Height = 400, 400
	cfg.InitialPopulation = 0
	cfg.InitialEnemies = 0
	cfg.InitialFoodItems = 0
	cfg.FoodSpawnRate = 0
	return cfg
}

// quietConfig additionally stops the metabolism, so that a test can watch one
// thing move without hunger and regeneration underneath it.
func quietConfig() Config {
	cfg := testConfig()
	cfg.HungerRate = 0
	cfg.RegenRate = 0
	cfg.StarveRate = 0
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

// fixedController always does the same thing, so a test can hold one agent's
// behaviour still and watch what the rules do around it.
type fixedController struct{ action Action }

func (f fixedController) Decide(*Perception) Action { return f.action }

// spyController records what it was shown and what the AI would have done.
type spyController struct {
	ai      AIController
	last    Action
	self    SelfView
	others  []AgentView
	foods   []FoodView
	decided int
}

func (s *spyController) Decide(p *Perception) Action {
	s.self = p.Self
	s.others = append(s.others[:0], p.Others...)
	s.foods = append(s.foods[:0], p.Foods...)
	s.decided++
	s.last = s.ai.Decide(p)
	return s.last
}

// aiChoice runs one AI decision for an agent and returns the action, without
// stepping the world.
func aiChoice(w *World, id int) Action {
	a := w.agentByID(id)
	return w.ai.Decide(w.perceive(a))
}

// --- determinism -----------------------------------------------------------

func TestSameSeedGivesSameRun(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 42

	a, b := NewWorld(cfg), NewWorld(cfg)
	for i := 0; i < 400; i++ {
		a.Step()
		b.Step()
	}

	if a.Stats() != b.Stats() {
		t.Fatalf("stats diverged:\n a = %+v\n b = %+v", a.Stats(), b.Stats())
	}
	for i := range a.Agents() {
		x, y := a.Agents()[i], b.Agents()[i]
		if x.ID != y.ID || x.X != y.X || x.Y != y.Y || x.Vitality != y.Vitality {
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

// --- the three state axes --------------------------------------------------

func TestHungerRisesOnItsOwn(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	// A body of exactly the average budget, which is the one HungerRate is
	// quoted for.
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80, Hunger: 10,
		Genome: filledGenome(cfg.GeneBudgetMean / float64(NumGenes))})
	w.SetController(id, fixedController{Action{Kind: ActRest}})

	w.Step()

	approx(t, mustAgent(t, w, id).Hunger, 10+cfg.HungerRate, 1e-9, "hunger after one tick")
}

// A body made of more costs more to run. It is the only price the budget pays,
// and without it the budget climbs for ever.
func TestABiggerBodyGetsHungryFaster(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	small := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80, Hunger: 10,
		Genome: filledGenome(cfg.GeneBudgetMean / float64(NumGenes))})
	big := w.addAgent(Agent{Maturity: 1, X: 300, Y: 300, Vitality: 80, Hunger: 10,
		Genome: filledGenome(2 * cfg.GeneBudgetMean / float64(NumGenes))})
	for _, id := range []int{small, big} {
		w.SetController(id, fixedController{Action{Kind: ActRest}})
	}

	w.Step()

	gainSmall := mustAgent(t, w, small).Hunger - 10
	gainBig := mustAgent(t, w, big).Hunger - 10
	approx(t, gainSmall, cfg.HungerRate, 1e-9, "hunger of an average body")
	// Twice the budget at an upkeep of 1 is twice the rate.
	approx(t, gainBig, cfg.HungerRate*(1+cfg.BudgetUpkeep), 1e-9, "hunger of a body twice the size")

	off := testConfig()
	off.BudgetUpkeep = 0
	w2 := NewWorld(off)
	id := w2.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80, Hunger: 10,
		Genome: filledGenome(2 * off.GeneBudgetMean / float64(NumGenes))})
	w2.SetController(id, fixedController{Action{Kind: ActRest}})
	w2.Step()
	approx(t, mustAgent(t, w2, id).Hunger-10, off.HungerRate, 1e-9, "hunger with the upkeep switched off")
}

func TestHighHungerDrainsVitality(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80, Hunger: cfg.MaxHunger})
	w.SetController(id, fixedController{Action{Kind: ActRest}})

	w.Step()

	// At maximum hunger the drain is the full rate.
	approx(t, mustAgent(t, w, id).Vitality, 80-cfg.StarveRate, 1e-9, "vitality while starving")
}

func TestStarvationKillsAgent(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: cfg.StarveRate / 2, Hunger: cfg.MaxHunger})
	w.SetController(id, fixedController{Action{Kind: ActRest}})

	w.Step()

	if _, ok := w.AgentByID(id); ok {
		t.Fatal("starved agent is still in the world")
	}
	if got := w.Stats().Deaths; got != 1 {
		t.Fatalf("deaths = %d, want 1", got)
	}
}

func TestSatiatedRestingAgentRecovers(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 50, Hunger: 0})
	w.SetController(id, fixedController{Action{Kind: ActRest}})

	w.Step()

	approx(t, mustAgent(t, w, id).Vitality, 50+cfg.RegenRate, 1e-9, "vitality after resting")
}

// Exerting yourself is what stops the recovery, not hunger alone.
func TestExertionBlocksRecovery(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 50, Hunger: 0})
	w.SetController(id, fixedController{Action{Kind: ActMove, DX: 1, Effort: 1}})

	w.Step()

	a := mustAgent(t, w, id)
	if a.Vitality >= 50 {
		t.Fatalf("vitality = %v, want less than 50: moving at full effort should cost more than it recovers", a.Vitality)
	}
	approx(t, a.Vitality, 50-cfg.MoveCost, 1e-9, "vitality after a tick of full effort movement")
}

// Food is not a store an agent carries: eating lowers hunger, and only that.
func TestEatingLowersHungerAndNotVitality(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 50, Hunger: 50})
	foodID := w.addFood(103, 100)
	w.SetController(id, fixedController{Action{Kind: ActEat, TargetID: foodID}})

	w.Step()

	a := mustAgent(t, w, id)
	approx(t, a.Hunger, 50-cfg.FoodNutrition, 1e-9, "hunger after eating")
	approx(t, a.Vitality, 50, 1e-9, "vitality after eating")
	if len(w.Foods()) != 0 {
		t.Fatalf("food items left = %d, want 0", len(w.Foods()))
	}
}

// Age itself carries no rule: only Lifespan (below) can kill from the passage
// of time, and only indirectly, through chronic bad eating.
func TestAgeDoesNotKill(t *testing.T) {
	w := NewWorld(quietConfig())
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80, Hunger: 0, Age: 1_000_000})
	w.SetController(id, fixedController{Action{Kind: ActRest}})

	for i := 0; i < 50; i++ {
		w.Step()
	}
	if _, ok := w.AgentByID(id); !ok {
		t.Fatal("an old agent died: Age is not supposed to matter on its own")
	}
}

// --- lifespan (background wear) --------------------------------------------
//
// Lifespan is spent only inside metabolise, gated on the same kind of hunger
// that already drains vitality for a different reason. None of this is a
// decision: there is no controller action for it, and Perception never
// mentions Lifespan (see perception.go), so nothing here can be read off a
// trace either.

// Between OverfedHunger and StarveHunger sits a band where eating is simply
// free: neither chronic rule fires.
func TestModerateHungerDoesNotSpendLifespan(t *testing.T) {
	cfg := testConfig()
	cfg.HungerRate = 0
	w := NewWorld(cfg)
	hunger := (cfg.OverfedHunger + cfg.StarveHunger) / 2
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80, Hunger: hunger, Lifespan: cfg.MaxLifespan})
	w.SetController(id, fixedController{Action{Kind: ActRest}})

	for i := 0; i < 100; i++ {
		w.Step()
	}

	approx(t, mustAgent(t, w, id).Lifespan, cfg.MaxLifespan, 1e-9, "lifespan after 100 ticks in the free band")
}

func TestChronicHungerSpendsLifespan(t *testing.T) {
	cfg := testConfig()
	cfg.HungerRate = 0
	cfg.StarveRate = 0 // isolate lifespan from the vitality death it would otherwise race
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80, Hunger: cfg.StarveHunger + 1, Lifespan: cfg.MaxLifespan})
	w.SetController(id, fixedController{Action{Kind: ActRest}})

	w.Step()

	approx(t, mustAgent(t, w, id).Lifespan, cfg.MaxLifespan-cfg.StarveLifespanRate, 1e-9, "lifespan after one tick of chronic hunger")
}

func TestChronicOverfeedingSpendsLifespan(t *testing.T) {
	cfg := testConfig()
	cfg.HungerRate = 0
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80, Hunger: cfg.OverfedHunger - 1, Lifespan: cfg.MaxLifespan})
	w.SetController(id, fixedController{Action{Kind: ActRest}})

	w.Step()

	approx(t, mustAgent(t, w, id).Lifespan, cfg.MaxLifespan-cfg.OverfedLifespanRate, 1e-9, "lifespan after one tick of chronic overeating")
}

func TestLifespanExhaustionKillsAndIsCountedSeparatelyFromCombat(t *testing.T) {
	cfg := testConfig()
	cfg.HungerRate = 0
	cfg.StarveRate = 0
	w := NewWorld(cfg)
	// Enough for three ticks of chronic hunger, not a fourth.
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80, Hunger: cfg.StarveHunger + 1, Lifespan: cfg.StarveLifespanRate * 3.5})
	w.SetController(id, fixedController{Action{Kind: ActRest}})

	for i := 0; i < 4; i++ {
		w.Step()
	}

	if _, ok := w.AgentByID(id); ok {
		t.Fatal("agent with exhausted lifespan is still in the world")
	}
	stats := w.Stats()
	if stats.AgingDeaths != 1 {
		t.Fatalf("aging deaths = %d, want 1", stats.AgingDeaths)
	}
	if stats.Kills != 0 {
		t.Fatalf("kills = %d, want 0: this was not combat", stats.Kills)
	}
	if stats.Deaths != 1 {
		t.Fatalf("deaths = %d, want 1", stats.Deaths)
	}
}

// Most of this file's tests build agents from a bare Agent{Maturity: 1, ...} literal and
// never mention Lifespan. They still get a full budget, the same fallback
// Vitality already has, so that omitting it does not silently mean "already
// dying of old age".
func TestBareAgentGetsAFullLifespanByDefault(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80, Hunger: 0})

	approx(t, mustAgent(t, w, id).Lifespan, cfg.MaxLifespan, 1e-9, "lifespan of a freshly added bare agent")
}

// --- movement and effort ---------------------------------------------------

// More effort buys speed with diminishing returns, and costs more vitality for
// every unit of distance covered.
func TestEffortTradesVitalityForSpeed(t *testing.T) {
	cfg := quietConfig()

	distance := func(effort float64) (dist, spent float64) {
		w := NewWorld(cfg)
		id := w.addAgent(Agent{Maturity: 1, X: 10, Y: 100, Vitality: 90, Hunger: 0})
		w.SetController(id, fixedController{Action{Kind: ActMove, DX: 1, Effort: effort}})
		for i := 0; i < 20; i++ {
			w.Step()
		}
		a := mustAgent(t, w, id)
		return a.X - 10, 90 - a.Vitality
	}

	slowDist, slowCost := distance(0.25)
	fastDist, fastCost := distance(1.0)

	if fastDist <= slowDist {
		t.Fatalf("full effort covered %v, quarter effort %v: more effort should be faster", fastDist, slowDist)
	}
	if fastCost <= slowCost {
		t.Fatalf("full effort cost %v, quarter effort %v: more effort should cost more", fastCost, slowCost)
	}
	if fastDist/slowDist >= 4 {
		t.Fatalf("speed scaled by %v for 4x the effort, want diminishing returns", fastDist/slowDist)
	}
	if fastCost/fastDist <= slowCost/slowDist {
		t.Fatal("hurrying was not more expensive per unit of distance")
	}
}

// --- combat ----------------------------------------------------------------

// brawl puts two agents next to each other and lets each of them do one fixed
// thing for a single tick.
func brawl(t *testing.T, cfg Config, aPower, bPower float64, aAct, bAct Action) (*World, *Agent, *Agent) {
	t.Helper()
	w := NewWorld(cfg)
	ida := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Genome: genomeOf(aPower, 0, 0), Vitality: 90, Hunger: 0})
	idb := w.addAgent(Agent{Maturity: 1, X: 105, Y: 100, Genome: genomeOf(bPower, 0, 0), Vitality: 90, Hunger: 0})
	aAct.TargetID, bAct.TargetID = idb, ida
	w.SetController(ida, fixedController{aAct})
	w.SetController(idb, fixedController{bAct})
	w.Step()
	return w, mustAgent(t, w, ida), mustAgent(t, w, idb)
}

func TestDamageScalesWithPowerAndEffort(t *testing.T) {
	cfg := quietConfig()

	_, _, weakHit := brawl(t, cfg, 25, 50, Action{Kind: ActAttack, Effort: 1}, Action{Kind: ActRest})
	_, _, hardHit := brawl(t, cfg, 100, 50, Action{Kind: ActAttack, Effort: 1}, Action{Kind: ActRest})
	_, _, halfHit := brawl(t, cfg, 100, 50, Action{Kind: ActAttack, Effort: 0.5}, Action{Kind: ActRest})

	approx(t, 90-hardHit.Vitality, cfg.AttackDamage*100/midAbility, 1e-9, "damage from a powerful attacker")
	if 90-hardHit.Vitality <= 90-weakHit.Vitality {
		t.Fatal("power did not make the blow hurt more")
	}
	approx(t, 90-halfHit.Vitality, (90-hardHit.Vitality)/2, 1e-9, "damage at half effort")
}

// The attacker pays less than the one being hit. That asymmetry is what makes
// hitting somebody who is not hitting back the best value there is.
func TestAttackingCostsLessThanBeingAttacked(t *testing.T) {
	cfg := quietConfig()
	_, attacker, victim := brawl(t, cfg, 50, 50, Action{Kind: ActAttack, Effort: 1}, Action{Kind: ActRest})

	attackerLoss, victimLoss := 90-attacker.Vitality, 90-victim.Vitality
	if attackerLoss >= victimLoss {
		t.Fatalf("attacker lost %v and the victim %v, want the attacker to come off better", attackerLoss, victimLoss)
	}
	// A punch costs the whole stance, not only the swing: an aggressive one
	// keeps a little guard up and pays for that too.
	approx(t, attackerLoss, stanceCost(&cfg, StanceAggressive), 1e-9, "cost of throwing a punch")
}

// Trading blows costs both sides both halves of the exchange, so a slugging
// match is worse for everybody than an ambush is for the ambusher.
func TestTradingBlowsCostsBothSides(t *testing.T) {
	cfg := quietConfig()
	_, x, y := brawl(t, cfg, 50, 50, Action{Kind: ActAttack, Effort: 1}, Action{Kind: ActAttack, Effort: 1})

	// Each side pays for its stance and takes what the other's guard did not
	// turn aside.
	guard := 1 - cfg.DefenceCap*(midAbility/MaxAbility)*stanceMix[StanceAggressive].Defence
	want := damagePerTick(&cfg, 50, stanceMix[StanceAggressive].Attack)*guard + stanceCost(&cfg, StanceAggressive)
	approx(t, 90-x.Vitality, want, 1e-9, "cost to one side of a mutual fight")
	approx(t, 90-y.Vitality, want, 1e-9, "cost to the other side of a mutual fight")

	_, ambusher, _ := brawl(t, cfg, 50, 50, Action{Kind: ActAttack, Effort: 1}, Action{Kind: ActRest})
	if 90-x.Vitality <= 90-ambusher.Vitality {
		t.Fatal("fighting back cost the aggressor no more than a free hit did")
	}
}

func TestFightingCanKill(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)
	killer := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Genome: genomeOf(100, 0, 0), Vitality: 90, Hunger: 0})
	victim := w.addAgent(Agent{Maturity: 1, X: 105, Y: 100, Genome: genomeOf(10, 0, 0), Vitality: 1, Hunger: 0})
	w.SetController(killer, fixedController{Action{Kind: ActAttack, TargetID: victim, Effort: 1}})
	w.SetController(victim, fixedController{Action{Kind: ActRest}})

	w.Step()

	if _, ok := w.AgentByID(victim); ok {
		t.Fatal("the victim survived a blow bigger than its remaining vitality")
	}
	if got := w.Stats().Kills; got != 1 {
		t.Fatalf("kills = %d, want 1", got)
	}
}

// Blows land together, so being early in the slice is not an advantage.
func TestBlowsLandSimultaneously(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)
	// Both would die from the other's blow. Neither may be spared by ordering.
	lethal := cfg.AttackDamage / 2
	x := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Genome: genomeOf(50, 0, 0), Vitality: lethal, Hunger: 0})
	y := w.addAgent(Agent{Maturity: 1, X: 105, Y: 100, Genome: genomeOf(50, 0, 0), Vitality: lethal, Hunger: 0})
	w.SetController(x, fixedController{Action{Kind: ActAttack, TargetID: y, Effort: 1}})
	w.SetController(y, fixedController{Action{Kind: ActAttack, TargetID: x, Effort: 1}})

	w.Step()

	if got := w.Stats().Population; got != 0 {
		t.Fatalf("population = %d, want 0: both blows should have landed", got)
	}
}

// --- memory and estimation -------------------------------------------------

func TestRiskMemoryRecordsWhatAFightCost(t *testing.T) {
	cfg := quietConfig()
	cfg.RiskDecayPerTick = 0 // forgetting is tested separately
	w := NewWorld(cfg)
	bully := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Genome: genomeOf(60, 0, 0), Vitality: 90, Hunger: 0})
	victim := w.addAgent(Agent{Maturity: 1, X: 105, Y: 100, Genome: genomeOf(50, 0, 0), Vitality: 90, Hunger: 0})
	w.SetController(bully, fixedController{Action{Kind: ActAttack, TargetID: victim, Effort: 1}})
	w.SetController(victim, fixedController{Action{Kind: ActRest}})

	for i := 0; i < 5; i++ {
		w.Step()
	}

	op, ok := w.Opinions(victim)[bully]
	if !ok {
		t.Fatal("the victim does not remember its attacker")
	}
	approx(t, op.Risk, 5*damagePerTick(&cfg, 60, 1), 1e-9, "remembered risk")

	// Nobody remembers being hit by somebody who never hit them.
	if op, ok := w.Opinions(bully)[victim]; ok && op.Risk != 0 {
		t.Fatalf("the aggressor remembers a risk of %v from an agent that never fought back", op.Risk)
	}
}

// Old fights fade. Without that, a long lived population ends up remembering
// everybody as a maximum threat and nothing ever happens again.
func TestRiskMemoryDecays(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)
	victim := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90, Hunger: 0})
	w.SetController(victim, fixedController{Action{Kind: ActRest}})
	w.rememberDamage(mustAgent(t, w, victim), 999, 40)

	fresh := w.Opinions(victim)[999].Risk
	for i := 0; i < 1000; i++ {
		w.Step()
	}
	faded := w.Opinions(victim)[999].Risk

	if faded >= fresh {
		t.Fatalf("risk went from %v to %v, want it to fade", fresh, faded)
	}
	approx(t, faded, 40*math.Exp(-cfg.RiskDecayPerTick*1000), 1e-6, "faded risk")
}

func TestStrengthEstimateConvergesOnTheTruth(t *testing.T) {
	w := NewWorld(testConfig())
	// Rationality 100 removes the reading error, leaving only the noise of the
	// observation itself.
	observer := &Agent{Maturity: 1, ID: 1, Genome: genomeOf(0, 100, 0)}
	target := &Agent{Maturity: 1, ID: 2, Genome: genomeOf(80, 0, 0)}

	start := w.opinionOf(observer, target.ID)
	startVariance := start.Variance
	if math.Abs(start.Strength-w.cfg.PriorStrength) > 1e-9 {
		t.Fatalf("a stranger is not assumed to be average: %v", start.Strength)
	}

	// A tick per reading: taking one in costs bandwidth, and this test is
	// about where the estimate ends up, not about how fast it can arrive.
	for i := 0; i < 200; i++ {
		w.observeStrength(observer, target, w.cfg.CombatObsVariance)
		w.tick++
	}

	op := w.opinionOf(observer, target.ID)
	if math.Abs(op.Strength-80) > 3 {
		t.Fatalf("estimate = %v after 200 observations, want close to the true 80", op.Strength)
	}
	if op.Variance >= startVariance/10 {
		t.Fatalf("variance only fell from %v to %v, want it to shrink with observations", startVariance, op.Variance)
	}
	if op.Samples != 200 {
		t.Fatalf("samples = %d, want 200", op.Samples)
	}
}

// Reading the world accurately is rationality's job.
func TestRationalityMakesEstimatesAccurate(t *testing.T) {
	w := NewWorld(testConfig())
	target := &Agent{Maturity: 1, ID: 9, Genome: genomeOf(70, 0, 0)}

	spread := func(rationality float64) float64 {
		total := 0.0
		for i := 0; i < 400; i++ {
			observer := &Agent{Maturity: 1, ID: 1, Genome: genomeOf(0, rationality, 0)}
			w.observeStrength(observer, target, w.cfg.CombatObsVariance)
			total += math.Abs(w.opinionOf(observer, target.ID).Strength - 70)
			w.tick++
		}
		return total / 400
	}

	sharp, dull := spread(100), spread(5)
	if sharp >= dull {
		t.Fatalf("error was %v for a rational agent and %v for a rash one, want the rational one closer", sharp, dull)
	}
}

// An agent learns about people it has never fought by watching others fight.
func TestSpectatorsLearnFromOtherPeoplesFights(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)
	x := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Genome: genomeOf(90, 100, 0), Vitality: 90, Hunger: 0})
	y := w.addAgent(Agent{Maturity: 1, X: 105, Y: 100, Genome: genomeOf(20, 100, 0), Vitality: 90, Hunger: 0})
	watcher := w.addAgent(Agent{Maturity: 1, X: 150, Y: 100, Genome: genomeOf(50, 100, 0), Vitality: 90, Hunger: 0})
	stranger := w.addAgent(Agent{Maturity: 1, X: 380, Y: 380, Genome: genomeOf(50, 100, 0), Vitality: 90, Hunger: 0})

	w.SetController(x, fixedController{Action{Kind: ActAttack, TargetID: y, Effort: 0.2}})
	w.SetController(y, fixedController{Action{Kind: ActAttack, TargetID: x, Effort: 0.2}})
	w.SetController(watcher, fixedController{Action{Kind: ActRest}})
	w.SetController(stranger, fixedController{Action{Kind: ActRest}})

	for i := 0; i < 400; i++ {
		w.Step()
	}

	seen := w.Opinions(watcher)
	if seen[x].Samples == 0 || seen[y].Samples == 0 {
		t.Fatalf("the onlooker learned nothing: %+v", seen)
	}
	if seen[x].Strength <= seen[y].Strength {
		t.Fatalf("onlooker rates the strong fighter %v and the weak one %v, want the strong one higher",
			seen[x].Strength, seen[y].Strength)
	}
	if seen[x].Variance >= w.cfg.PriorVariance {
		t.Fatal("watching a long fight did not make the onlooker any more sure")
	}
	// Somebody on the other side of the world saw nothing.
	if op, ok := w.Opinions(stranger)[x]; ok && op.Samples != 0 {
		t.Fatal("an agent out of range still learned from the fight")
	}
}

// --- decision triggers -----------------------------------------------------

// Deciding is driven by events, not by the clock.
func TestDecisionsAreTriggeredNotContinuous(t *testing.T) {
	cfg := quietConfig()
	cfg.TriggerIdleTicks = 1000 // out of the way for this test
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90, Hunger: 0})
	spy := &spyController{}
	w.SetController(id, spy)

	for i := 0; i < 30; i++ {
		w.Step()
	}
	if spy.decided != 1 {
		t.Fatalf("controller ran %d times in 30 quiet ticks, want 1", spy.decided)
	}

	// A dent in the vitality is a reason to think again.
	mustAgent(t, w, id).Vitality -= cfg.TriggerVitalityDrop + 1
	w.Step()
	if spy.decided != 2 {
		t.Fatalf("controller ran %d times, want it to reconsider after losing vitality", spy.decided)
	}
}

func TestIdlingTriggersAFreshDecision(t *testing.T) {
	cfg := quietConfig()
	cfg.TriggerIdleTicks = 10
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90, Hunger: 0})
	spy := &spyController{}
	w.SetController(id, spy)

	for i := 0; i < 31; i++ {
		w.Step()
	}
	if spy.decided < 3 {
		t.Fatalf("controller ran %d times in 31 ticks with a 10 tick idle trigger, want at least 3", spy.decided)
	}
}

func TestBeingAttackedTriggersADecision(t *testing.T) {
	cfg := quietConfig()
	cfg.TriggerIdleTicks = 1000
	cfg.TriggerVitalityDrop = 1000
	w := NewWorld(cfg)
	victim := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90, Hunger: 0})
	bully := w.addAgent(Agent{Maturity: 1, X: 105, Y: 100, Genome: genomeOf(50, 0, 0), Vitality: 90, Hunger: 0})
	spy := &spyController{}
	w.SetController(victim, spy)
	w.SetController(bully, fixedController{Action{Kind: ActAttack, TargetID: victim, Effort: 1}})

	w.Step() // the first blow lands
	before := spy.decided
	w.Step() // ... and is noticed
	if spy.decided <= before {
		t.Fatal("being hit did not make the victim reconsider")
	}
}

// --- what the utility comparison produces ----------------------------------

// Priority 1 is staying alive: a hungry agent goes for food even with an
// attractive candidate standing right next to it.
func TestSurvivalTakesPriorityOverMating(t *testing.T) {
	cfg := testConfig()

	choose := func(hunger float64) ActionKind {
		w := NewWorld(cfg)
		id := w.addAgent(Agent{Maturity: 1,
			X: 200, Y: 200, Sex: Male, Vitality: 95, Hunger: hunger,
			Genome: genomeOf(50, 100, 100)})
		w.addAgent(Agent{Maturity: 1,
			X: 210, Y: 200, Sex: Female, Vitality: 100, Hunger: 0,
			Genome: genomeOf(90, 90, 90)})
		w.addFood(230, 200)
		mustAgent(t, w, id).reproReady = true
		return aiChoice(w, id).Kind
	}

	if got := choose(cfg.MaxHunger * 0.9); got != ActEat {
		t.Fatalf("a starving agent chose %v, want it to go for the food", got)
	}
	if got := choose(0); got != ActCourt {
		t.Fatalf("a well fed agent chose %v, want it to court the candidate", got)
	}
}

// Running away is not a threshold: it is what wins the comparison when the
// damage coming in is what is about to kill the agent, and loses when it is not.
func TestFleeingEmergesFromTheComparison(t *testing.T) {
	cfg := testConfig()

	underAttack := func(victimVitality, attackerPower float64) ActionKind {
		w := NewWorld(cfg)
		victim := w.addAgent(Agent{Maturity: 1,
			X: 200, Y: 200, Sex: Male, Vitality: victimVitality, Hunger: 0,
			Genome: genomeOf(20, 100, 100)})
		attacker := w.addAgent(Agent{Maturity: 1,
			X: 208, Y: 200, Sex: Male, Vitality: 100, Hunger: 0,
			Genome: genomeOf(attackerPower, 100, 100)})
		w.SetController(attacker, fixedController{Action{Kind: ActAttack, TargetID: victim, Effort: 1}})
		w.SetController(victim, fixedController{Action{Kind: ActRest}})
		w.Step() // take a hit, so the victim knows it is under attack
		return aiChoice(w, victim).Kind
	}

	if got := underAttack(12, 100); got != ActFlee {
		t.Fatalf("a nearly dead agent facing a far stronger attacker chose %v, want it to run", got)
	}
	if got := underAttack(100, 8); got == ActFlee {
		t.Fatal("a healthy agent ran from a feeble attacker: fleeing should not be automatic")
	}
}

// A rival that is clearly out of reach is not fought. Nothing says so: winning
// is simply unlikely, which makes the fight a bad deal.
func TestHopelessFightsAreNotPicked(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	weak := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Vitality: 60, Hunger: 50,
		Genome: genomeOf(5, 100, 100)})
	giant := w.addAgent(Agent{Maturity: 1, X: 206, Y: 200, Vitality: 100, Hunger: 0, Genome: genomeOf(100, 0, 0)})
	w.addFood(210, 200)

	// Make the weak agent certain about how strong the giant is.
	for i := 0; i < 200; i++ {
		w.observeStrength(mustAgent(t, w, weak), mustAgent(t, w, giant), 1)
	}

	if got := aiChoice(w, weak).Kind; got == ActAttack {
		t.Fatal("a hopelessly outmatched agent picked a fight")
	}
}

// Pre-emptive removal of a competitor is a food problem, not a grudge: make
// food plentiful and the reason to do it disappears.
func TestPreemptionFadesWhenFoodIsPlentiful(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)

	attackValue := func(scarcity float64) float64 {
		p := &Perception{
			Tick: 1,
			Cfg:  &cfg,
			Self: SelfView{
				ID: 1, X: 200, Y: 200, Vitality: 100, Hunger: 20,
				Attack: 80, Rationality: 100, Intelligence: 100,
				MaxVitality: cfg.MaxVitality, MaxSpeed: cfg.MaxSpeed,
				FoodScarcity: scarcity,
				// What the agent assumes, which since stage 12a comes from the
				// agent and not from the config. A perception built by hand has
				// to say so, or it is one that believes nothing is worth
				// anything.
				Retaliation:       cfg.Retaliation,
				AcceptChance:      cfg.AcceptChance,
				RiskWeight:        cfg.RiskWeight,
				CompetitionWeight: cfg.CompetitionWeight,
				ShockRisk:         cfg.ShockRisk},
			Rand: w.rng}
		victim := AgentView{ID: 2, Dist: 10, Vitality: 40, EstStrength: 20}

		c := &AIController{}
		c.addAttack(p, &victim)
		best := math.Inf(-1)
		for _, o := range c.opts {
			best = math.Max(best, o.util)
		}
		return best
	}

	tight, plentiful := attackValue(3), attackValue(0)
	if tight <= plentiful {
		t.Fatalf("attacking scored %v when food was tight and %v when it was plentiful, want scarcity to matter",
			tight, plentiful)
	}
}

// --- intelligence ----------------------------------------------------------

// Intelligence gates which kinds of move an agent can even think of, when the
// gate is switched on. It is off in DefaultConfig, so this has to ask for it:
// what is being checked here is that the mechanism still works, not that the
// world is run with it.
func TestIntelligenceGatesTheStrategiesAvailable(t *testing.T) {
	cfg := testConfig()
	cfg.StrategyDepthUnlock = 16

	kinds := func(intelligence float64) map[ActionKind]bool {
		w := NewWorld(cfg)
		id := w.addAgent(Agent{Maturity: 1,
			X: 200, Y: 200, Sex: Male, Vitality: 100, Hunger: 0,
			Genome: genomeOf(50, 100, intelligence)})
		w.addAgent(Agent{Maturity: 1, X: 210, Y: 200, Sex: Female, Vitality: 100, Hunger: 0, Genome: genomeOf(50, 0, 0)})
		mustAgent(t, w, id).reproReady = true

		c := &AIController{}
		c.Decide(w.perceive(mustAgent(t, w, id)))
		out := map[ActionKind]bool{}
		for _, o := range c.opts {
			out[o.action.Kind] = true
		}
		return out
	}

	dull := kinds(MinAbility)
	if dull[ActCourt] || dull[ActAttack] || dull[ActObserve] {
		t.Fatalf("a mindless agent considered moves beyond its reach: %v", dull)
	}
	if !dull[ActEat] && !dull[ActRest] {
		t.Fatal("a mindless agent could not think of the basics either")
	}

	bright := kinds(MaxAbility)
	for _, k := range []ActionKind{ActCourt, ActAttack, ActObserve} {
		if !bright[k] {
			t.Fatalf("a clever agent did not consider %v", k)
		}
	}
}

// The other half of intelligence: telling the options you thought of apart.
func TestIntelligenceMakesTheChoiceReliable(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)

	// Two options a hair apart, and one obviously bad.
	hits := func(intelligence float64) int {
		c := &AIController{}
		p := &Perception{
			Cfg:  &cfg,
			Self: SelfView{Intelligence: intelligence},
			Rand: w.rng}
		count := 0
		for i := 0; i < 2000; i++ {
			c.opts = c.opts[:0]
			c.add(Action{Kind: ActRest}, Utility{Life: Goal{Value: 30, Chance: 1}})
			c.add(Action{Kind: ActMove}, Utility{Life: Goal{Value: 20, Chance: 1}})
			c.add(Action{Kind: ActAttack}, Utility{Life: Goal{Value: -40, Chance: 1}})
			if c.pick(p).Kind == ActRest {
				count++
			}
		}
		return count
	}

	bright, dull := hits(MaxAbility), hits(MinAbility)
	if bright != 2000 {
		t.Fatalf("a fully intelligent agent took the best option %d/2000 times, want every time", bright)
	}
	if dull >= bright {
		t.Fatalf("dull agent got it right %d times and the bright one %d, want the dull one to slip up", dull, bright)
	}
	if dull < 500 {
		t.Fatalf("dull agent got it right only %d/2000 times, want mistakes to stay proportionate", dull)
	}
}

// --- the controller seam ---------------------------------------------------

// The engine drives whatever the controller hands back and does not care who
// wrote it. This is the seam a human player will be plugged into.
func TestControllerDrivesTheAgent(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Vitality: 90, Hunger: 0})
	if !w.SetController(id, fixedController{Action{Kind: ActMove, DX: 0, DY: -1, Effort: 1}}) {
		t.Fatal("could not install a controller")
	}

	w.Step()

	a := mustAgent(t, w, id)
	approx(t, a.Y, 200-cfg.MaxSpeed, 1e-9, "position after one commanded move")
	approx(t, a.X, 200, 1e-9, "sideways drift")

	if w.SetController(9999, fixedController{}) {
		t.Fatal("installed a controller on an agent that does not exist")
	}
}

// What a controller is shown never includes the true ability of anybody else:
// combat power is a hidden parameter and all anyone gets is their own estimate.
func TestPerceptionHidesTrueStrength(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Vitality: 90, Hunger: 0, Genome: genomeOf(0, 100, 0)})
	spy := &spyController{}
	w.SetController(id, spy)
	w.addAgent(Agent{Maturity: 1, X: 220, Y: 200, Vitality: 90, Hunger: 0, Genome: genomeOf(97, 0, 0)})

	w.Step()

	if len(spy.others) != 1 {
		t.Fatalf("saw %d others, want 1", len(spy.others))
	}
	if spy.others[0].EstStrength != cfg.PriorStrength {
		t.Fatalf("a stranger's strength came through as %v, want the prior %v and not the true 97",
			spy.others[0].EstStrength, cfg.PriorStrength)
	}
	if spy.self.Attack != 0 {
		t.Fatalf("self power = %v, want the agent's own value", spy.self.Attack)
	}
}

// --- lineage ---------------------------------------------------------------

// pairAboutToGiveBirth returns a world with a bonded couple whose bond ends on
// the next step.
func pairAboutToGiveBirth(t *testing.T, cfg Config) (*World, int, int) {
	t.Helper()
	w := NewWorld(cfg)
	male := w.addAgent(Agent{Maturity: 1,
		X: 100, Y: 100, Sex: Male, Genome: genomeOf(40, 60, 30),
		Vitality: 90, Hunger: 0, PairTimer: 1, State: StatePaired, Generation: 2})
	female := w.addAgent(Agent{Maturity: 1,
		X: 110, Y: 100, Sex: Female, Genome: genomeOf(60, 80, 50),
		Vitality: 90, Hunger: 0, PairTimer: 1, State: StatePaired, Generation: 5})
	w.agentByID(male).PartnerID = female
	w.agentByID(female).PartnerID = male
	return w, male, female
}

func findChild(t *testing.T, w *World, parents ...int) *Agent {
	t.Helper()
	for i := range w.Agents() {
		a := &w.Agents()[i]
		known := false
		for _, p := range parents {
			known = known || a.ID == p
		}
		if !known {
			return a
		}
	}
	t.Fatal("no child was born")
	return nil
}

// Recorded from the start, because a player will later need to follow a dead
// agent to one of its own descendants.
func TestBirthRecordsLineageBothWays(t *testing.T) {
	cfg := testConfig()
	w, male, female := pairAboutToGiveBirth(t, cfg)

	w.Step()

	child := findChild(t, w, male, female)
	if child.ParentIDs != [2]int{male, female} {
		t.Fatalf("child parents = %v, want %v", child.ParentIDs, [2]int{male, female})
	}
	for _, id := range []int{male, female} {
		p := mustAgent(t, w, id)
		if len(p.ChildIDs) != 1 || p.ChildIDs[0] != child.ID {
			t.Fatalf("parent %d children = %v, want [%d]", id, p.ChildIDs, child.ID)
		}
	}
}

// --- inheritance and mutation ----------------------------------------------

func TestChildTakesEachAbilityFromOneParentOrTheOther(t *testing.T) {
	cfg := testConfig()
	cfg.MutationStd = 0 // isolate inheritance from mutation
	w, male, female := pairAboutToGiveBirth(t, cfg)

	w.Step()

	if got := w.Stats().Births; got != 1 {
		t.Fatalf("births = %d, want 1", got)
	}
	child := findChild(t, w, male, female)
	// One parent's value or the other's for each gene, all scaled by the one
	// factor that fits them onto the budget the child inherited. The average
	// of the two - 50 for power - is what must never appear.
	scale := pickedFromOneParent(t, child, mustAgent(t, w, male), mustAgent(t, w, female))
	// The inherited values, not what the newborn can express with them: a
	// child is born small (Maturity 0) and grows into its genome.
	oneOf(t, child.Gene(GeneAttack)/scale, 40, 60, "child power")
	oneOf(t, child.Gene(GeneRationality)/scale, 60, 80, "child rationality")
	oneOf(t, child.Gene(GeneIntelligence)/scale, 30, 50, "child intelligence")
	approx(t, child.Vitality, cfg.ChildVitalityShare*child.MaxVitality(&cfg), 1e-9, "child vitality")
	if child.Generation != 6 { // max(2, 5) + 1
		t.Fatalf("child generation = %d, want 6", child.Generation)
	}

	// Raising a child costs both parents vitality, equally.
	want := 90 - cfg.BirthVitalityCost/2 + cfg.RegenRate
	for _, id := range []int{male, female} {
		p := mustAgent(t, w, id)
		approx(t, p.Vitality, want, 1e-9, "parent vitality")
		if p.PartnerID != 0 || p.CooldownTimer <= 0 {
			t.Fatalf("parent %d was not released from the bond with a rest: %+v", id, p)
		}
	}
}

// pickedFromOneParent checks that every gene of the child is one parent's
// value or the other's after a single common scaling, and returns that scale.
//
// The scaling is what fits the inherited genes onto the budget the child
// inherited separately: what passes down gene by gene is the shape of the
// parent, and what passes down as a budget is the size.
func pickedFromOneParent(t *testing.T, child, pa, pb *Agent) float64 {
	t.Helper()
	free := func(v float64) bool { return v > MinAbility+1e-9 && v < MaxAbility-1e-9 }
	fits := func(scale float64) bool {
		for i := range child.Genome {
			if !free(child.Genome[i]) {
				continue // pinned at a bound: the scale cannot be read off it
			}
			a, b := pa.Gene(Gene(i)), pb.Gene(Gene(i))
			if math.Abs(child.Genome[i]-scale*a) > 1e-6 && math.Abs(child.Genome[i]-scale*b) > 1e-6 {
				return false
			}
		}
		return true
	}
	for i := range child.Genome {
		if !free(child.Genome[i]) {
			continue
		}
		for _, candidate := range []float64{pa.Gene(Gene(i)), pb.Gene(Gene(i))} {
			if candidate <= 0 {
				continue
			}
			if scale := child.Genome[i] / candidate; fits(scale) {
				return scale
			}
		}
	}
	t.Fatalf("child genome %v is not a per gene pick from %v and %v at any one scale",
		child.Genome, pa.Genome, pb.Genome)
	return 0
}

// filledGenome is a genome with every gene at the same value, which is what a
// test wants when it is asking about inheritance rather than about abilities.
func filledGenome(v float64) []float64 {
	g := newGenome()
	for i := range g {
		g[i] = v
	}
	return g
}

func oneOf(t *testing.T, got, a, b float64, what string) {
	t.Helper()
	if math.Abs(got-a) > 1e-9 && math.Abs(got-b) > 1e-9 {
		t.Fatalf("%s = %v, want %v or %v", what, got, a, b)
	}
}

// Each gene is thrown for on its own: a child can have its father's power and
// its mother's wits. Genes travelling together would show up here as children
// that are only ever one parent or the other, all the way across.
func TestChildDrawsEachAbilityIndependently(t *testing.T) {
	cfg := testConfig()
	cfg.MutationStd = 0
	w := NewWorld(cfg)
	// Values close enough together that fitting a child onto its inherited
	// budget never pushes a gene onto a bound, so the pick stays readable.
	pa := &Agent{Maturity: 1, Genome: filledGenome(30), Vitality: 90}
	pb := &Agent{Maturity: 1, Genome: filledGenome(50), Vitality: 90}

	seen := map[[3]bool]int{}
	for i := 0; i < 400; i++ {
		pa.Vitality, pb.Vitality = 90, 90
		w.tryBirth(pa, pb)
	}
	for i := range w.newborns {
		c := &w.newborns[i]
		// The genome is scaled onto the inherited budget, so which parent a
		// gene came from is read against the scaled candidates.
		scale := pickedFromOneParent(t, c, pa, pb)
		from := func(g Gene) bool { return math.Abs(c.Gene(g)-scale*50) < 1e-6 }
		seen[[3]bool{from(GeneAttack), from(GeneRationality), from(GeneIntelligence)}]++
	}
	// All eight combinations of three coins, none of them rare.
	if len(seen) != 8 {
		t.Fatalf("saw %d of the 8 combinations of parents: %v", len(seen), seen)
	}
	for combo, n := range seen {
		if n < 400/8/3 {
			t.Fatalf("combination %v turned up only %d times in 400 births: the genes are not independent", combo, n)
		}
	}
}

// The point of the whole change. Under blending, the spread of an ability
// halves every generation and is gone within a handful of them; drawing whole
// values keeps it, and all that eats away at it is drift.
func TestParticulateInheritanceKeepsTheSpread(t *testing.T) {
	cfg := testConfig()
	cfg.MutationStd = 0         // no new variation: whatever survives came from the start
	cfg.BudgetInheritSpread = 0 // and no wandering budget on top of it
	w := NewWorld(cfg)

	// Whole genomes rather than one varying gene: fitting a child onto the
	// budget it inherited scales every gene together, so a population that
	// differs in one gene alone would be measuring the scaling rather than the
	// inheritance.
	const n = 60
	pop := make([]Agent, 0, n)
	for i := 0; i < n; i++ {
		pop = append(pop, Agent{Maturity: 1, Genome: w.drawGenome(), Vitality: 90})
	}
	before := spread(pop)

	// Twenty generations of pairing at random, with nobody selected for.
	for gen := 0; gen < 20; gen++ {
		w.newborns = w.newborns[:0]
		for len(w.newborns) < n {
			pa := &pop[w.rng.Intn(n)]
			pb := &pop[w.rng.Intn(n)]
			pa.Vitality, pb.Vitality = 90, 90
			w.tryBirth(pa, pb)
		}
		pop = append(pop[:0], w.newborns[:n]...)
	}
	after := spread(pop)

	// Blending would leave 2^-20 of it. Drift in a population of 60 costs a
	// few percent a generation, so more than half is the honest bar.
	if after < before*0.5 {
		t.Fatalf("spread fell from %.2f to %.2f over 20 generations: inheritance is losing variation", before, after)
	}
}

// spread is how varied the population is in what it spends on attack, as a
// share of the budget. The share is the figure to watch now that the budget is
// inherited separately: the raw value moves when the budget does.
func spread(pop []Agent) float64 {
	share := func(a *Agent) float64 { return a.Gene(GeneAttack) / a.Budget() }
	var sum float64
	for i := range pop {
		sum += share(&pop[i])
	}
	mean := sum / float64(len(pop))
	var sq float64
	for i := range pop {
		d := share(&pop[i]) - mean
		sq += d * d
	}
	return math.Sqrt(sq / float64(len(pop)))
}

func TestMutationVariesChildAbility(t *testing.T) {
	cfg := testConfig()
	cfg.MutationRate = 1 // every gene, so fifty births is enough to see it
	cfg.MutationStd = 4
	w := NewWorld(cfg)
	pa := &Agent{Maturity: 1, Genome: genomeOf(50, 50, 50), Vitality: 90}
	pb := &Agent{Maturity: 1, Genome: genomeOf(50, 50, 50), Vitality: 90}

	for i := 0; i < 50; i++ {
		pa.Vitality, pb.Vitality = 90, 90
		w.tryBirth(pa, pb)
	}
	if len(w.newborns) != 50 {
		t.Fatalf("newborns = %d, want 50", len(w.newborns))
	}
	varied := false
	for i := range w.newborns {
		if w.newborns[i].Attack(&w.cfg) != 50 {
			varied = true
		}
	}
	if !varied {
		t.Fatal("no child differed from its parents, mutation is not applied")
	}
}

// Mutation is rare and large: most children carry their parent's number
// untouched, and the ones that do not have moved a long way.
func TestMutationIsRareAndLarge(t *testing.T) {
	cfg := testConfig()
	cfg.MutationRate = 0.01
	cfg.MutationStd = 40
	cfg.BudgetInheritSpread = 0 // so that only mutation moves a gene
	w := NewWorld(cfg)
	pa := &Agent{Maturity: 1, Genome: filledGenome(50), Vitality: 90}
	pb := &Agent{Maturity: 1, Genome: filledGenome(50), Vitality: 90}

	// Emptied every time, because the world stops accepting newborns once it
	// is full and twenty thousand births are needed to count a 1% event.
	//
	// The count is per child rather than per gene: a mutation is fitted back
	// onto the inherited budget, so the gene that jumped goes up and every
	// other gene comes down a little to pay for it. What "1% per gene" shows
	// up as is a child whose genome is not its parents', which for nine genes
	// is about 1 - 0.99^9 = 8.6% of children.
	const births = 20000
	touched, jumped, total := 0, 0, 0
	for i := 0; i < births; i++ {
		pa.Vitality, pb.Vitality = 90, 90
		w.newborns = w.newborns[:0]
		w.tryBirth(pa, pb)
		for j := range w.newborns {
			total++
			biggest := 0.0
			for _, v := range w.newborns[j].Genome {
				biggest = math.Max(biggest, math.Abs(v-50))
			}
			if biggest > 1e-9 {
				touched++
				if biggest > 10 {
					jumped++
				}
			}
		}
	}
	rate := float64(touched) / float64(total)
	if rate < 0.05 || rate > 0.13 {
		t.Fatalf("%.1f%% of children carry a mutation, want about 8.6%%", rate*100)
	}
	// A jump of std 40 clears ten points about four times in five, so most of
	// the mutations that happen are big ones rather than a nudge.
	if float64(jumped)/float64(touched) < 0.6 {
		t.Fatalf("only %d of %d mutations moved more than 10 points: these are not jumps", jumped, touched)
	}
}

// Rate zero stops new variation without stopping inheritance: children still
// take one parent's value or the other's, just never anything new.
func TestMutationRateZeroLeavesTheParentsValuesUntouched(t *testing.T) {
	cfg := testConfig()
	cfg.MutationRate = 0
	cfg.MutationStd = 40 // would be obvious if it were applied
	w := NewWorld(cfg)
	pa := &Agent{Maturity: 1, Genome: filledGenome(30), Vitality: 90}
	pb := &Agent{Maturity: 1, Genome: filledGenome(50), Vitality: 90}

	cfg.BudgetInheritSpread = 0
	w = NewWorld(cfg)
	for i := 0; i < 200; i++ {
		pa.Vitality, pb.Vitality = 90, 90
		w.tryBirth(pa, pb)
	}
	for i := range w.newborns {
		c := &w.newborns[i]
		scale := pickedFromOneParent(t, c, pa, pb)
		oneOf(t, c.Gene(GeneAttack)/scale, 30, 50, "child power")
		oneOf(t, c.Gene(GeneRationality)/scale, 30, 50, "child rationality")
		oneOf(t, c.Gene(GeneIntelligence)/scale, 30, 50, "child intelligence")
	}
}

func TestChildAbilityStaysInRange(t *testing.T) {
	cfg := testConfig()
	cfg.MutationRate = 1
	cfg.MutationStd = 50 // extreme, so the bounds are actually hit
	w := NewWorld(cfg)
	pa := &Agent{Maturity: 1, Genome: genomeOf(MaxAbility, MinAbility, MaxAbility), Vitality: 90}
	pb := &Agent{Maturity: 1, Genome: genomeOf(MaxAbility, MinAbility, MaxAbility), Vitality: 90}

	for i := 0; i < 300; i++ {
		pa.Vitality, pb.Vitality = 90, 90
		w.tryBirth(pa, pb)
	}
	for i := range w.newborns {
		c := &w.newborns[i]
		for _, v := range []float64{c.Gene(GeneAttack), c.Gene(GeneRationality), c.Gene(GeneIntelligence)} {
			if v < MinAbility || v > MaxAbility {
				t.Fatalf("child ability %v is out of range", v)
			}
		}
	}
}

func TestBirthNeedsEnoughVitality(t *testing.T) {
	cfg := testConfig()
	w, male, female := pairAboutToGiveBirth(t, cfg)
	mustAgent(t, w, male).Vitality = cfg.BirthVitalityCost / 4
	mustAgent(t, w, female).Vitality = cfg.BirthVitalityCost / 4

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
}

// --- mate choice -----------------------------------------------------------

func TestPatienceGrowsWithRationality(t *testing.T) {
	w := NewWorld(testConfig())
	if w.patienceTicks(&Agent{Maturity: 1, Genome: genomeOf(0, 10, 0)}) <= w.patienceTicks(&Agent{Maturity: 1}) {
		t.Fatal("the rational agent did not compare candidates for longer")
	}
}

// courting builds two agents next to each other, both able to reproduce and
// neither an obvious catch.
func courting(t *testing.T, cfg Config) (*World, int, int) {
	t.Helper()
	w := NewWorld(cfg)
	make := func(x float64, sex Sex) int {
		// Attractive enough to be worth courting, short of the bar that makes
		// a candidate an instant catch: the point of the scenario is the
		// comparison, so both sides have to want to start it.
		g := genomeOf(40, 100, 100)
		g[GeneAttractiveness] = 70
		return w.addAgent(Agent{Maturity: 1,
			X: x, Y: 200, Sex: sex, Genome: g,
			Vitality: cfg.ReproVitalityShare*cfg.MaxVitality + 5, Hunger: 0})
	}
	male, female := make(200, Male), make(205, Female)
	if f := fitness(mustAgent(t, w, male), &cfg); f >= cfg.CommitFitness {
		t.Fatalf("test setup: fitness %v is an obvious catch, the pair would form instantly", f)
	}
	return w, male, female
}

// What a mate looks like is an advertisement and the condition behind it, and
// nothing else: two agents identical in everything but the attractiveness gene
// must not look alike, and two identical in that gene must, however different
// their fighting or their wits.
func TestFitnessIsTheAdvertisementNotTheAbility(t *testing.T) {
	cfg := testConfig()
	feeble := &Agent{Maturity: 1, Genome: genomeOf(1, 1, 1), Vitality: 50}
	mighty := &Agent{Maturity: 1, Genome: genomeOf(100, 100, 100), Vitality: 50}
	feeble.Genome[GeneAttractiveness] = 60
	mighty.Genome[GeneAttractiveness] = 60
	if a, b := fitness(feeble, &cfg), fitness(mighty, &cfg); math.Abs(a-b) > 1e-9 {
		t.Fatalf("fitness %v against %v: ability is leaking into how good a mate somebody looks", a, b)
	}

	plain, showy := &Agent{Maturity: 1, Genome: genomeOf(50, 50, 50), Vitality: 50}, &Agent{Maturity: 1, Genome: genomeOf(50, 50, 50), Vitality: 50}
	plain.Genome[GeneAttractiveness] = 10
	showy.Genome[GeneAttractiveness] = 90
	if fitness(showy, &cfg) <= fitness(plain, &cfg) {
		t.Fatal("the attractiveness gene does nothing")
	}

	// Condition still counts for something: a dying agent is a poor bet
	// however fine it looks.
	dying := &Agent{Maturity: 1, Genome: cloneGenome(showy.Genome), Vitality: 1}
	if fitness(dying, &cfg) >= fitness(showy, &cfg) {
		t.Fatal("condition is not in fitness at all")
	}
}

// Somebody worth crossing the world for is a reason to think again. It adds no
// action: courting is scored by the same comparison as everything else, and
// this only stops an agent walking past a candidate because it happened not to
// be thinking at that moment.
func TestAStrikingCandidateIsAReasonToThinkAgain(t *testing.T) {
	cfg := testConfig()
	cfg.TriggerIdleTicks = 100000 // so idling cannot be what fires
	w := NewWorld(cfg)
	ready := func(x float64, sex Sex, looks float64) int {
		g := genomeOf(50, 50, 50)
		g[GeneAttractiveness] = looks
		id := w.addAgent(Agent{Maturity: 1, X: x, Y: 200, Sex: sex, Genome: g, Hunger: 0})
		a := w.agentByID(id)
		a.Vitality = a.MaxVitality(&cfg)
		a.Action = Action{Kind: ActRest}
		a.lastDecisionTick = w.tick
		a.vitalityAtDecision = a.Vitality
		a.needsDecision = false // it has just decided; only a new event may interrupt
		a.pendingTrigger = TriggerNone
		return id
	}
	watcher := ready(200, Male, 50)
	ready(240, Female, 100)

	if got := w.decisionTrigger(mustAgent(t, w, watcher)); got != TriggerMateInSight {
		t.Fatalf("trigger = %v, want mate in sight", got)
	}

	// Somebody ordinary is not an interruption.
	w2 := NewWorld(cfg)
	w = w2
	watcher = ready(200, Male, 50)
	ready(240, Female, 20)
	if got := w.decisionTrigger(mustAgent(t, w, watcher)); got == TriggerMateInSight {
		t.Fatal("an ordinary candidate should not interrupt")
	}
}

func TestPairNeedsBothSidesToAgreeAndTakesTime(t *testing.T) {
	cfg := testConfig()
	w, male, female := courting(t, cfg)

	w.Step()
	w.Step()
	for _, id := range []int{male, female} {
		if mustAgent(t, w, id).PartnerID != 0 {
			t.Fatalf("agent %d committed immediately, without comparing", id)
		}
	}

	paired := -1
	for i := 0; i < 800 && paired < 0; i++ {
		w.Step()
		if mustAgent(t, w, male).PartnerID != 0 {
			paired = w.Tick()
		}
	}
	if paired < 0 {
		t.Fatal("the pair never formed")
	}
	if patience := w.patienceTicks(mustAgent(t, w, male)); paired < patience {
		t.Fatalf("pair formed at tick %d, before the %d ticks of comparison", paired, patience)
	}
	if mustAgent(t, w, male).PartnerID != female || mustAgent(t, w, female).PartnerID != male {
		t.Fatal("the pair is not mutual")
	}
}

func TestPartnerDeathReleasesSurvivor(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	survivor := w.addAgent(Agent{Maturity: 1,
		X: 100, Y: 100, Sex: Male, Vitality: 90, Hunger: 0,
		State: StatePaired, PairTimer: 100})
	dying := w.addAgent(Agent{Maturity: 1,
		X: 110, Y: 100, Sex: Female, Vitality: cfg.StarveRate / 2, Hunger: cfg.MaxHunger,
		State: StatePaired, PairTimer: 100})
	w.agentByID(survivor).PartnerID = dying
	w.agentByID(dying).PartnerID = survivor

	w.Step()

	a := mustAgent(t, w, survivor)
	if a.PartnerID != 0 {
		t.Fatalf("partner id = %d, want 0 after losing a partner", a.PartnerID)
	}
	if a.CooldownTimer <= 0 {
		t.Fatal("the survivor got no rest after losing its partner")
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

func TestEatenFoodStaysFindable(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)
	first := w.addFood(100, 100)
	second := w.addFood(200, 200)
	third := w.addFood(300, 300)

	w.removeFoodByID(first)

	for _, id := range []int{second, third} {
		f := w.foodByID(id)
		if f == nil || f.ID != id {
			t.Fatalf("food %d went missing after another item was removed: %+v", id, f)
		}
	}
	if w.foodByID(first) != nil {
		t.Fatal("the eaten item is still there")
	}
}

// --- whole simulation ------------------------------------------------------

// A default world must be able to run on its own: agents forage, fight over
// what is scarce, pair up and raise children, and the population evolves over
// generations without dying out or exploding.
func TestDefaultWorldSustainsItself(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 7
	w := NewWorld(cfg)

	// Twelve thousand ticks rather than eight: with the credit for a shared
	// kill in it the world breeds more slowly (measured: about a generation
	// fewer per fifty thousand ticks), and on this seed the second generation
	// arrives at tick 9943 instead of 3777. The claim being made here is that
	// the world keeps itself going, not that it does so by any particular
	// tick.
	for i := 0; i < 12000; i++ {
		w.Step()
		s := w.Stats()
		if s.Population > cfg.MaxPopulation {
			t.Fatalf("population %d exceeded the cap %d at tick %d", s.Population, cfg.MaxPopulation, s.Tick)
		}
		// The two kinds have separate allowances: plants grow up to one, and
		// carcasses pile up to the other.
		plants, meat := 0, 0
		for _, f := range w.Foods() {
			if f.Kind == FoodMeat {
				meat++
			} else {
				plants++
			}
		}
		if plants > cfg.MaxFoodItems {
			t.Fatalf("plants %d exceeded the cap %d at tick %d", plants, cfg.MaxFoodItems, s.Tick)
		}
		if meat > cfg.MaxMeatItems {
			t.Fatalf("meat %d exceeded the cap %d at tick %d", meat, cfg.MaxMeatItems, s.Tick)
		}
	}

	s := w.Stats()
	if s.Population == 0 {
		t.Fatal("the population died out entirely")
	}
	if s.Births == 0 || s.MaxGeneration < 2 {
		t.Fatalf("births = %d, generations = %d, want the population to be reproducing", s.Births, s.MaxGeneration)
	}
	if s.Fights == 0 {
		t.Fatal("nothing was ever contested in a world where food runs short")
	}
	if s.Males+s.Females != s.Population {
		t.Fatalf("sex counts %d + %d do not add up to the population %d", s.Males, s.Females, s.Population)
	}
}

// --- spacing ---------------------------------------------------------------

// Clumping has to answer the question the cooperation work asks of it: a crowd
// packed into one corner reads higher than the same crowd spread out, whatever
// the population size.
func TestClumpingSeparatesAHuddleFromASpread(t *testing.T) {
	const n = 40

	layout := func(place func(i int) (float64, float64)) Spacing {
		w := NewWorld(testConfig())
		for i := 0; i < n; i++ {
			x, y := place(i)
			w.addAgent(Agent{Maturity: 1, X: x, Y: y, Vitality: 100, Genome: genomeOf(50, 0, 0)})
		}
		return w.Spacing()
	}

	// Everybody inside one perception radius of everybody else.
	huddle := layout(func(i int) (float64, float64) {
		return 200 + float64(i%8)*5, 200 + float64(i/8)*5
	})
	// The same number of agents over the whole 400x400 world.
	spread := layout(func(i int) (float64, float64) {
		return 20 + float64(i%8)*45, 20 + float64(i/8)*70
	})

	if huddle.AvgNeighbours <= spread.AvgNeighbours {
		t.Fatalf("a huddle should have more neighbours: %.1f vs %.1f",
			huddle.AvgNeighbours, spread.AvgNeighbours)
	}
	if huddle.Clumping <= spread.Clumping {
		t.Fatalf("clumping %.2f (huddle) should beat %.2f (spread)", huddle.Clumping, spread.Clumping)
	}
	if huddle.AvgNearestDist >= spread.AvgNearestDist {
		t.Fatalf("a huddle should sit closer together: %.1f vs %.1f",
			huddle.AvgNearestDist, spread.AvgNearestDist)
	}
}

// Clumping is meant to be readable across populations of different sizes, which
// is the whole reason it is a ratio: doubling the crowd at the same spread must
// not look like grouping.
func TestClumpingDoesNotRiseWithPopulationAlone(t *testing.T) {
	spread := func(n int) float64 {
		w := NewWorld(testConfig())
		for i := 0; i < n; i++ {
			// The same lattice either way; the larger population just fills
			// more of it.
			w.addAgent(Agent{Maturity: 1, X: 20 + float64(i%16)*24, Y: 20 + float64(i/16)*24, Vitality: 100, Genome: genomeOf(50, 0, 0)})
		}
		return w.Spacing().Clumping
	}

	small, large := spread(32), spread(128)
	if math.Abs(large-small)/small > 0.35 {
		t.Fatalf("clumping moved from %.2f to %.2f on population alone", small, large)
	}
}

// --- clustering -------------------------------------------------------------

func clusteredWorld(t *testing.T, linkDist float64, place func(i int) (float64, float64), n int) Clustering {
	t.Helper()
	w := NewWorld(testConfig())
	for i := 0; i < n; i++ {
		x, y := place(i)
		w.addAgent(Agent{Maturity: 1, X: x, Y: y, Vitality: 100, Genome: genomeOf(50, 0, 0)})
	}
	return w.Clusters(linkDist)
}

// Three groups of four, each group far enough from the others that no pair
// across two of them is within the linking distance.
func TestClustersFindsSeparatedGroups(t *testing.T) {
	c := clusteredWorld(t, 20, func(i int) (float64, float64) {
		group := i / 4
		return 40 + float64(group)*150 + float64(i%4)*5, 40 + float64(i%4)*5
	}, 12)

	if c.Groups != 3 {
		t.Fatalf("groups = %d, want 3 (sizes %v)", c.Groups, c.Sizes)
	}
	if c.Singletons != 0 {
		t.Fatalf("singletons = %d, want 0", c.Singletons)
	}
	if c.Largest != 4 || c.AvgGroupSize != 4 {
		t.Fatalf("largest %d, avg %.1f, want 4 and 4", c.Largest, c.AvgGroupSize)
	}
	if c.GroupedShare != 1 {
		t.Fatalf("grouped share = %.2f, want 1", c.GroupedShare)
	}
	if got := sum(c.Sizes); got != 12 {
		t.Fatalf("sizes %v sum to %d, want the population 12", c.Sizes, got)
	}
}

// An agent nobody is close to is a singleton, not a group of one.
func TestClustersCountsLonersSeparately(t *testing.T) {
	c := clusteredWorld(t, 20, func(i int) (float64, float64) {
		if i < 3 {
			return 40 + float64(i)*5, 40
		}
		return 40 + float64(i)*90, 300 // spread far apart from each other too
	}, 6)

	if c.Groups != 1 || c.Singletons != 3 {
		t.Fatalf("groups %d, singletons %d, want 1 and 3 (sizes %v)", c.Groups, c.Singletons, c.Sizes)
	}
	if c.Largest != 3 {
		t.Fatalf("largest = %d, want 3", c.Largest)
	}
	if c.GroupedShare != 0.5 {
		t.Fatalf("grouped share = %.2f, want 0.5", c.GroupedShare)
	}
}

// Linking is transitive: a chain of agents each within the linking distance of
// the next is one cluster, even though its ends are far apart.
func TestClustersLinkTransitively(t *testing.T) {
	c := clusteredWorld(t, 20, func(i int) (float64, float64) {
		return 20 + float64(i)*15, 200
	}, 10)

	if c.Groups != 1 || c.Largest != 10 {
		t.Fatalf("a chain should be one cluster of 10, got %d groups, largest %d", c.Groups, c.Largest)
	}
	if c.LargestShare != 1 {
		t.Fatalf("largest share = %.2f, want 1", c.LargestShare)
	}
}

// The linking distance is the ruler, so widening it can only merge clusters,
// never split them.
func TestClustersMergeAsLinkDistanceGrows(t *testing.T) {
	place := func(i int) (float64, float64) {
		return 30 + float64(i%6)*60, 30 + float64(i/6)*60
	}
	tight := clusteredWorld(t, 20, place, 36)
	loose := clusteredWorld(t, 70, place, 36)

	if tight.Largest != 1 || tight.Singletons != 36 {
		t.Fatalf("at 20 the grid should be all singletons, got largest %d, singletons %d",
			tight.Largest, tight.Singletons)
	}
	if loose.Largest != 36 {
		t.Fatalf("at 70 the grid should be one cluster, got largest %d (sizes %v)",
			loose.Largest, loose.Sizes)
	}
}

func TestClustersOfAnEmptyWorld(t *testing.T) {
	c := clusteredWorld(t, 20, func(i int) (float64, float64) { return 0, 0 }, 0)
	if c.Groups != 0 || c.Singletons != 0 || c.Largest != 0 || len(c.Sizes) != 0 {
		t.Fatalf("an empty world should have no clusters, got %+v", c)
	}
}

// Cross check of the union-find against the obvious quadratic labelling, on
// random layouts at a distance where clusters actually form.
func TestClustersMatchBruteForceLabelling(t *testing.T) {
	const linkDist = 40.0
	rng := rand.New(rand.NewSource(7))

	for trial := 0; trial < 20; trial++ {
		w := NewWorld(testConfig())
		n := 3 + rng.Intn(40)
		xs := make([]float64, n)
		ys := make([]float64, n)
		for i := 0; i < n; i++ {
			xs[i], ys[i] = rng.Float64()*400, rng.Float64()*400
			w.addAgent(Agent{Maturity: 1, X: xs[i], Y: ys[i], Vitality: 100, Genome: genomeOf(50, 0, 0)})
		}

		// Label by repeatedly spreading each agent's label to everybody within
		// range of it, until nothing changes.
		label := make([]int, n)
		for i := range label {
			label[i] = i
		}
		for changed := true; changed; {
			changed = false
			for i := 0; i < n; i++ {
				for j := 0; j < n; j++ {
					if i == j || label[i] == label[j] {
						continue
					}
					if dist2(xs[i], ys[i], xs[j], ys[j]) <= linkDist*linkDist {
						lo := min(label[i], label[j])
						label[i], label[j] = lo, lo
						changed = true
					}
				}
			}
		}
		counts := map[int]int{}
		for _, l := range label {
			counts[l]++
		}
		want := make([]int, 0, len(counts))
		for _, c := range counts {
			want = append(want, c)
		}
		sort.Sort(sort.Reverse(sort.IntSlice(want)))

		got := w.Clusters(linkDist)
		if !slices.Equal(got.Sizes, want) {
			t.Fatalf("trial %d (n=%d): sizes %v, brute force says %v", trial, n, got.Sizes, want)
		}

		// The labels must agree with the brute force labelling too: two agents
		// share a label exactly when they share a brute force label.
		if len(got.Labels) != n {
			t.Fatalf("trial %d: %d labels for %d agents", trial, len(got.Labels), n)
		}
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				if (got.Labels[i] == got.Labels[j]) != (label[i] == label[j]) {
					t.Fatalf("trial %d: agents %d and %d labelled %d/%d, brute force says %d/%d",
						trial, i, j, got.Labels[i], got.Labels[j], label[i], label[j])
				}
			}
		}
		// A label is an index into Sizes, and the component it points at is the
		// one the agent is actually in.
		seen := make([]int, len(got.Sizes))
		for _, l := range got.Labels {
			if l < 0 || l >= len(got.Sizes) {
				t.Fatalf("trial %d: label %d out of range for %d clusters", trial, l, len(got.Sizes))
			}
			seen[l]++
		}
		if !slices.Equal(seen, got.Sizes) {
			t.Fatalf("trial %d: labels count %v, sizes say %v", trial, seen, got.Sizes)
		}
	}
}

func sum(xs []int) int {
	total := 0
	for _, x := range xs {
		total += x
	}
	return total
}

// --- membership half-life ---------------------------------------------------

// place puts the agents where the test wants them and moves the clock on, so
// that a tracker can be fed a sequence of situations without stepping the world
// and letting the rules move anybody.
func placeAt(w *World, tick int, xs ...float64) {
	w.tick = tick
	for i := range w.agents {
		w.agents[i].X = xs[i*2]
		w.agents[i].Y = xs[i*2+1]
	}
}

// Two pairs that never move stay together forever, which the tracker has to
// report as censored rather than as a half-life of zero.
func TestMembershipOfAWorldThatNeverMovesIsCensored(t *testing.T) {
	w := NewWorld(testConfig())
	for i := 0; i < 4; i++ {
		w.addAgent(Agent{Maturity: 1, X: 100 + float64(i/2)*200, Y: 100 + float64(i%2)*10, Vitality: 100, Genome: genomeOf(50, 0, 0)})
	}

	m := NewMembershipTracker(30, 10, 5)
	for tick := 0; tick <= 100; tick += 10 {
		w.tick = tick
		m.Observe(w)
	}

	got := m.Result()
	if !got.Censored {
		t.Fatalf("half-life = %.1f, want censored: nobody moved", got.HalfLife)
	}
	for k, s := range got.Survival {
		if s != 1 {
			t.Fatalf("survival at lag %d = %.2f, want 1", k*got.Step, s)
		}
	}
	if got.Pairs == 0 {
		t.Fatal("no pair observations were counted")
	}
}

// A pair that parts between two observations has a half-life inside the first
// step, and the curve is flat at zero after it.
func TestMembershipOfAPairThatParts(t *testing.T) {
	w := NewWorld(testConfig())
	w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 100, Genome: genomeOf(50, 0, 0)})
	w.addAgent(Agent{Maturity: 1, X: 110, Y: 100, Vitality: 100, Genome: genomeOf(50, 0, 0)})

	m := NewMembershipTracker(30, 10, 5)
	m.Observe(w)
	for tick := 10; tick <= 50; tick += 10 {
		placeAt(w, tick, 100, 100, 300, 300)
		m.Observe(w)
	}

	got := m.Result()
	if got.Censored {
		t.Fatal("the pair parted, so the half-life is not censored")
	}
	// Survival is 1 at lag 0 and 0 at lag 10, so the half is crossed halfway.
	approx(t, got.HalfLife, 5, 1e-9, "half-life of a pair that parts at once")
	if got.Survival[1] != 0 {
		t.Fatalf("survival at lag 10 = %.2f, want 0", got.Survival[1])
	}
}

// Half the pairs parting puts the half-life at the step where they parted.
func TestMembershipHalfLifeSitsWhereHalfThePairsPart(t *testing.T) {
	w := NewWorld(testConfig())
	// Four pairs, each far from the others.
	for i := 0; i < 8; i++ {
		w.addAgent(Agent{Maturity: 1, X: 60 + float64(i/2)*90, Y: 60 + float64(i%2)*10, Vitality: 100, Genome: genomeOf(50, 0, 0)})
	}

	m := NewMembershipTracker(30, 10, 4)
	m.Observe(w)

	// At the second reading two of the four pairs have split up.
	placeAt(w, 10,
		60, 60, 60, 70, // together
		150, 60, 150, 70, // together
		240, 60, 240, 300, // parted
		330, 60, 330, 300, // parted
	)
	m.Observe(w)

	got := m.Result()
	approx(t, got.Survival[1], 0.5, 1e-9, "survival at lag 10")
	// Survival goes 1 -> 0.5 across the first step, so it reaches a half at
	// the end of that step.
	approx(t, got.HalfLife, 10, 1e-9, "half-life")
	if got.Pairs != 4 {
		t.Fatalf("pair observations = %d, want 4", got.Pairs)
	}
}

// A pair broken up by a death is not a pair that drifted apart. Counting it as
// one would measure how long agents live instead of how long they stay
// together.
func TestMembershipIgnoresPairsBrokenByDeath(t *testing.T) {
	w := NewWorld(testConfig())
	for i := 0; i < 4; i++ {
		w.addAgent(Agent{Maturity: 1, X: 100 + float64(i/2)*200, Y: 100 + float64(i%2)*10, Vitality: 100, Genome: genomeOf(50, 0, 0)})
	}

	m := NewMembershipTracker(30, 10, 4)
	m.Observe(w)

	w.kill(&w.agents[3]) // one half of the second pair
	w.removeDead()
	w.tick = 10
	m.Observe(w)

	got := m.Result()
	if got.Pairs != 1 {
		t.Fatalf("pair observations = %d, want 1: the pair a death broke up should not count", got.Pairs)
	}
	approx(t, got.Survival[1], 1, 1e-9, "survival at lag 10")
	if !got.Censored {
		t.Fatalf("half-life = %.1f, want censored: the surviving pair never parted", got.HalfLife)
	}
}

// Every observation starts a cohort of its own, so a run of readings measures
// the shortest lag many times over instead of once.
func TestMembershipPoolsEveryCohort(t *testing.T) {
	w := NewWorld(testConfig())
	w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 100, Genome: genomeOf(50, 0, 0)})
	w.addAgent(Agent{Maturity: 1, X: 110, Y: 100, Vitality: 100, Genome: genomeOf(50, 0, 0)})

	m := NewMembershipTracker(30, 10, 3)
	for tick := 0; tick <= 40; tick += 10 {
		w.tick = tick
		m.Observe(w)
	}

	// Five readings ten ticks apart: four pairs of readings are one step
	// apart, three are two steps apart, two are three steps apart.
	got := m.Result()
	if got.Pairs != 4+3+2 {
		t.Fatalf("pair observations = %d, want 9", got.Pairs)
	}
}

// At interpolates between the readings, and is defined past the end of them.
func TestMembershipAtInterpolates(t *testing.T) {
	m := Membership{Step: 10, Survival: []float64{1, 0.5, 0.25}}
	approx(t, m.At(0), 1, 1e-9, "At(0)")
	approx(t, m.At(5), 0.75, 1e-9, "At(5)")
	approx(t, m.At(10), 0.5, 1e-9, "At(10)")
	approx(t, m.At(15), 0.375, 1e-9, "At(15)")
	approx(t, m.At(1000), 0.25, 1e-9, "At past the end of the curve")
}

// --- fight rates by companionship -------------------------------------------

// A pair that has been together since before the lag is a companion pair, and
// one that has just arrived is a stranger pair, even though both meetings look
// identical at the moment they are counted.
func TestFightRatesSplitCompanionsFromStrangers(t *testing.T) {
	w := NewWorld(testConfig())
	a := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 100, Genome: genomeOf(50, 0, 0)})
	w.addAgent(Agent{Maturity: 1, X: 105, Y: 100, Vitality: 100, Genome: genomeOf(50, 0, 0)})
	c := w.addAgent(Agent{Maturity: 1, X: 300, Y: 300, Vitality: 100, Genome: genomeOf(50, 0, 0)})

	f := NewFightTracker(30, 10, 20)
	for tick := 0; tick <= 20; tick += 10 {
		w.tick = tick
		f.Observe(w)
	}

	// The stranger walks into the pair and starts hitting one of them; the two
	// companions are not fighting each other.
	w.agentByID(c).X, w.agentByID(c).Y = 108, 100
	w.agentByID(c).Action = Action{Kind: ActAttack, TargetID: a}
	w.tick = 30
	f.Observe(w)

	got := f.Result()
	// The pair met as companions three times over, and the newcomer makes two
	// stranger meetings at the last reading, one of which is a fight.
	if got.CompanionFights != 0 {
		t.Fatalf("companion fights = %d, want 0", got.CompanionFights)
	}
	if got.StrangerMeetings != 2 || got.StrangerFights != 1 {
		t.Fatalf("stranger meetings %d, fights %d, want 2 and 1",
			got.StrangerMeetings, got.StrangerFights)
	}
	approx(t, got.Stranger, 0.5, 1e-9, "stranger fight rate")
	approx(t, got.Companion, 0, 1e-9, "companion fight rate")
	if got.Ratio != 0 {
		t.Fatalf("ratio = %v, want 0 when companions never fight", got.Ratio)
	}
}

// Both rates are per meeting, so companions being near each other far more
// often does not by itself make them look violent.
func TestFightRatesAreCountedPerMeeting(t *testing.T) {
	w := NewWorld(testConfig())
	a := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 100, Genome: genomeOf(50, 0, 0)})
	b := w.addAgent(Agent{Maturity: 1, X: 105, Y: 100, Vitality: 100, Genome: genomeOf(50, 0, 0)})

	f := NewFightTracker(30, 10, 20)
	for tick := 0; tick <= 20; tick += 10 {
		w.tick = tick
		f.Observe(w)
	}
	// The companions meet four more times and fight in one of them.
	for i, tick := range []int{30, 40, 50, 60} {
		if i == 0 {
			w.agentByID(a).Action = Action{Kind: ActAttack, TargetID: b}
		} else {
			w.agentByID(a).Action = Action{Kind: ActRest}
		}
		w.tick = tick
		f.Observe(w)
	}

	got := f.Result()
	if got.CompanionMeetings != 5 || got.CompanionFights != 1 {
		t.Fatalf("companion meetings %d, fights %d, want 5 and 1",
			got.CompanionMeetings, got.CompanionFights)
	}
	approx(t, got.Companion, 0.2, 1e-9, "companion fight rate")
}

// Agents out of reach of each other are not meeting at all, whatever their
// clusters say, and an attack action aimed at somebody else is not a fight
// between these two.
func TestFightRatesIgnoreWhatIsOutOfReach(t *testing.T) {
	w := NewWorld(testConfig())
	a := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 100, Genome: genomeOf(50, 0, 0)})
	w.addAgent(Agent{Maturity: 1, X: 125, Y: 100, Vitality: 100, Genome: genomeOf(50, 0, 0)}) // linked, but out of reach

	f := NewFightTracker(30, 10, 20)
	for tick := 0; tick <= 30; tick += 10 {
		w.agentByID(a).Action = Action{Kind: ActAttack, TargetID: 999}
		w.tick = tick
		f.Observe(w)
	}

	got := f.Result()
	if got.CompanionMeetings != 0 || got.StrangerMeetings != 0 {
		t.Fatalf("meetings companion %d, stranger %d, want none: they are %v apart, CombatRadius is %v",
			got.CompanionMeetings, got.StrangerMeetings, 25.0, w.cfg.CombatRadius)
	}
}

// An agent that was not around a lag ago has no history to be judged by, so its
// meetings are skipped rather than guessed at.
func TestFightRatesSkipAgentsWithNoHistory(t *testing.T) {
	w := NewWorld(testConfig())
	w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 100, Genome: genomeOf(50, 0, 0)})

	f := NewFightTracker(30, 10, 20)
	for tick := 0; tick <= 20; tick += 10 {
		w.tick = tick
		f.Observe(w)
	}
	w.addAgent(Agent{Maturity: 1, X: 105, Y: 100, Vitality: 100, Genome: genomeOf(50, 0, 0)}) // born just now
	w.tick = 30
	f.Observe(w)

	got := f.Result()
	if got.CompanionMeetings+got.StrangerMeetings != 0 {
		t.Fatalf("meetings = %d, want 0: the newcomer has no history",
			got.CompanionMeetings+got.StrangerMeetings)
	}
}

// --- distance between groups ------------------------------------------------

// Three groups in a row: the middle one is close to both of its neighbours, the
// outer two are close only to the middle. Every gap is the distance to the
// nearest agent of another group, not between centres.
func TestClusterGapsMeasureTheNearestOtherGroup(t *testing.T) {
	w := NewWorld(testConfig())
	// Groups at x = 40, 140 and 340, each two agents ten apart.
	for _, x := range []float64{40, 140, 340} {
		w.addAgent(Agent{Maturity: 1, X: x, Y: 100, Vitality: 100, Genome: genomeOf(50, 0, 0)})
		w.addAgent(Agent{Maturity: 1, X: x + 10, Y: 100, Vitality: 100, Genome: genomeOf(50, 0, 0)})
	}

	got := w.ClusterGaps(20)
	if len(got.Gaps) != 3 {
		t.Fatalf("gaps %v, want one per group", got.Gaps)
	}
	// Nearest edges: 40s to 140s is 90, 140s to 340s is 190.
	want := []float64{90, 90, 190}
	for i, g := range got.Gaps {
		approx(t, g, want[i], 1e-9, "gap")
	}
	approx(t, got.Median, 90, 1e-9, "median gap")
	approx(t, got.Mean, (90+90+190)/3.0, 1e-9, "mean gap")
}

// A lone agent is not a group: it neither has a gap of its own nor makes
// anybody else's smaller.
func TestClusterGapsIgnoreSingletons(t *testing.T) {
	w := NewWorld(testConfig())
	for _, x := range []float64{40, 340} {
		w.addAgent(Agent{Maturity: 1, X: x, Y: 100, Vitality: 100, Genome: genomeOf(50, 0, 0)})
		w.addAgent(Agent{Maturity: 1, X: x + 10, Y: 100, Vitality: 100, Genome: genomeOf(50, 0, 0)})
	}
	w.addAgent(Agent{Maturity: 1, X: 190, Y: 100, Vitality: 100, Genome: genomeOf(50, 0, 0)}) // wanderer in between

	got := w.ClusterGaps(20)
	if len(got.Gaps) != 2 {
		t.Fatalf("gaps %v, want one per group and none for the loner", got.Gaps)
	}
	// 50 to 340 rather than 50 to the wanderer at 190.
	for _, g := range got.Gaps {
		approx(t, g, 290, 1e-9, "gap across the wanderer")
	}
}

// Single linkage puts anybody closer than the linking distance in the same
// group, so no gap can be shorter than it. The measure is bounded below, and
// the tests that read the low end have to know it.
func TestClusterGapsAreNeverShorterThanTheLinkDistance(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 3
	w := NewWorld(cfg)
	for i := 0; i < 4000; i++ {
		w.Step()
	}

	got := w.ClusterGaps(DefaultClusterLinkDist)
	if len(got.Gaps) < 2 {
		t.Fatalf("expected several groups, got %v", got.Gaps)
	}
	if got.Gaps[0] <= DefaultClusterLinkDist {
		t.Fatalf("shortest gap %.2f is not above the linking distance %v",
			got.Gaps[0], float64(DefaultClusterLinkDist))
	}
}

// Fewer than two groups means there is nothing to measure a distance between.
func TestClusterGapsOfASingleGroup(t *testing.T) {
	w := NewWorld(testConfig())
	w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 100, Genome: genomeOf(50, 0, 0)})
	w.addAgent(Agent{Maturity: 1, X: 110, Y: 100, Vitality: 100, Genome: genomeOf(50, 0, 0)})

	got := w.ClusterGaps(20)
	if len(got.Gaps) != 0 || got.Mean != 0 || got.Relative != 0 {
		t.Fatalf("one group should give no gaps, got %+v", got)
	}
}

// Spreading the same groups over the same world can only push them apart.
func TestClusterGapsGrowWhenGroupsSpreadOut(t *testing.T) {
	build := func(spacing float64) ClusterGaps {
		w := NewWorld(testConfig())
		for i := 0; i < 5; i++ {
			x := 30 + float64(i)*spacing
			w.addAgent(Agent{Maturity: 1, X: x, Y: 100, Vitality: 100, Genome: genomeOf(50, 0, 0)})
			w.addAgent(Agent{Maturity: 1, X: x + 10, Y: 100, Vitality: 100, Genome: genomeOf(50, 0, 0)})
		}
		return w.ClusterGaps(20)
	}

	tight, loose := build(60), build(90)
	if !(loose.Mean > tight.Mean && loose.P10 > tight.P10) {
		t.Fatalf("spreading out should raise the gaps: mean %.1f -> %.1f, p10 %.1f -> %.1f",
			tight.Mean, loose.Mean, tight.P10, loose.P10)
	}
}

// Relative divides out the density, and its reference is the nearest neighbour
// distance of that many points while a gap is measured between the edges of two
// patches of agents. At the size the groups actually come out, the patches are
// small next to the spacing and the two coincide: a layout with no structure
// reads at 1. That is what makes a number from a real run readable on its own,
// so it is worth pinning down. It would stop holding if the groups grew large
// next to the gaps between them.
func TestClusterGapsRelativeOfARandomLayout(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	var rel float64
	const trials = 40
	for range trials {
		w := NewWorld(testConfig())
		w.cfg.Width, w.cfg.Height = 800, 600
		// The shape the default world reports: 26 groups of four, plus loners.
		for g := 0; g < 26; g++ {
			cx, cy := rng.Float64()*760+20, rng.Float64()*560+20
			for k := 0; k < 4; k++ {
				w.addAgent(Agent{Maturity: 1, X: cx + rng.Float64()*16 - 8, Y: cy + rng.Float64()*16 - 8,
					Vitality: 100, Genome: genomeOf(50, 0, 0)})
			}
		}
		for i := 0; i < 33; i++ {
			w.addAgent(Agent{Maturity: 1, X: rng.Float64()*760 + 20, Y: rng.Float64()*560 + 20,
				Vitality: 100, Genome: genomeOf(50, 0, 0)})
		}
		rel += w.ClusterGaps(DefaultClusterLinkDist).Relative
	}
	rel /= trials

	t.Logf("relative gap of a structureless layout: %.3f", rel)
	if rel < 0.9 || rel > 1.1 {
		t.Fatalf("relative gap of a structureless layout = %.3f, want about 1: "+
			"the scale is only readable on its own if no structure means no signal", rel)
	}
}
