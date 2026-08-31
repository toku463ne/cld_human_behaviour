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
	id := w.addAgent(Agent{X: 100, Y: 100, Vitality: 80, Hunger: 10})
	w.SetController(id, fixedController{Action{Kind: ActRest}})

	w.Step()

	approx(t, mustAgent(t, w, id).Hunger, 10+cfg.HungerRate, 1e-9, "hunger after one tick")
}

func TestHighHungerDrainsVitality(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	id := w.addAgent(Agent{X: 100, Y: 100, Vitality: 80, Hunger: cfg.MaxHunger})
	w.SetController(id, fixedController{Action{Kind: ActRest}})

	w.Step()

	// At maximum hunger the drain is the full rate.
	approx(t, mustAgent(t, w, id).Vitality, 80-cfg.StarveRate, 1e-9, "vitality while starving")
}

func TestStarvationKillsAgent(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	id := w.addAgent(Agent{X: 100, Y: 100, Vitality: cfg.StarveRate / 2, Hunger: cfg.MaxHunger})
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
	id := w.addAgent(Agent{X: 100, Y: 100, Vitality: 50, Hunger: 0})
	w.SetController(id, fixedController{Action{Kind: ActRest}})

	w.Step()

	approx(t, mustAgent(t, w, id).Vitality, 50+cfg.RegenRate, 1e-9, "vitality after resting")
}

// Exerting yourself is what stops the recovery, not hunger alone.
func TestExertionBlocksRecovery(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	id := w.addAgent(Agent{X: 100, Y: 100, Vitality: 50, Hunger: 0})
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
	id := w.addAgent(Agent{X: 100, Y: 100, Vitality: 50, Hunger: 50})
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
	id := w.addAgent(Agent{X: 100, Y: 100, Vitality: 80, Hunger: 0, Age: 1_000_000})
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
	id := w.addAgent(Agent{X: 100, Y: 100, Vitality: 80, Hunger: hunger, Lifespan: cfg.MaxLifespan})
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
	id := w.addAgent(Agent{X: 100, Y: 100, Vitality: 80, Hunger: cfg.StarveHunger + 1, Lifespan: cfg.MaxLifespan})
	w.SetController(id, fixedController{Action{Kind: ActRest}})

	w.Step()

	approx(t, mustAgent(t, w, id).Lifespan, cfg.MaxLifespan-cfg.StarveLifespanRate, 1e-9, "lifespan after one tick of chronic hunger")
}

func TestChronicOverfeedingSpendsLifespan(t *testing.T) {
	cfg := testConfig()
	cfg.HungerRate = 0
	w := NewWorld(cfg)
	id := w.addAgent(Agent{X: 100, Y: 100, Vitality: 80, Hunger: cfg.OverfedHunger - 1, Lifespan: cfg.MaxLifespan})
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
	id := w.addAgent(Agent{X: 100, Y: 100, Vitality: 80, Hunger: cfg.StarveHunger + 1, Lifespan: cfg.StarveLifespanRate * 3.5})
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

// Most of this file's tests build agents from a bare Agent{...} literal and
// never mention Lifespan. They still get a full budget, the same fallback
// Vitality already has, so that omitting it does not silently mean "already
// dying of old age".
func TestBareAgentGetsAFullLifespanByDefault(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	id := w.addAgent(Agent{X: 100, Y: 100, Vitality: 80, Hunger: 0})

	approx(t, mustAgent(t, w, id).Lifespan, cfg.MaxLifespan, 1e-9, "lifespan of a freshly added bare agent")
}

// --- movement and effort ---------------------------------------------------

// More effort buys speed with diminishing returns, and costs more vitality for
// every unit of distance covered.
func TestEffortTradesVitalityForSpeed(t *testing.T) {
	cfg := quietConfig()

	distance := func(effort float64) (dist, spent float64) {
		w := NewWorld(cfg)
		id := w.addAgent(Agent{X: 10, Y: 100, Vitality: 90, Hunger: 0})
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
	ida := w.addAgent(Agent{X: 100, Y: 100, Power: aPower, Vitality: 90, Hunger: 0})
	idb := w.addAgent(Agent{X: 105, Y: 100, Power: bPower, Vitality: 90, Hunger: 0})
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
	approx(t, attackerLoss, cfg.AttackCost, 1e-9, "cost of throwing a punch")
}

// Trading blows costs both sides both halves of the exchange, so a slugging
// match is worse for everybody than an ambush is for the ambusher.
func TestTradingBlowsCostsBothSides(t *testing.T) {
	cfg := quietConfig()
	_, x, y := brawl(t, cfg, 50, 50, Action{Kind: ActAttack, Effort: 1}, Action{Kind: ActAttack, Effort: 1})

	want := damagePerTick(&cfg, 50, 1) + cfg.AttackCost
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
	killer := w.addAgent(Agent{X: 100, Y: 100, Power: 100, Vitality: 90, Hunger: 0})
	victim := w.addAgent(Agent{X: 105, Y: 100, Power: 10, Vitality: 1, Hunger: 0})
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
	x := w.addAgent(Agent{X: 100, Y: 100, Power: 50, Vitality: lethal, Hunger: 0})
	y := w.addAgent(Agent{X: 105, Y: 100, Power: 50, Vitality: lethal, Hunger: 0})
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
	bully := w.addAgent(Agent{X: 100, Y: 100, Power: 60, Vitality: 90, Hunger: 0})
	victim := w.addAgent(Agent{X: 105, Y: 100, Power: 50, Vitality: 90, Hunger: 0})
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
	victim := w.addAgent(Agent{X: 100, Y: 100, Vitality: 90, Hunger: 0})
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
	observer := &Agent{ID: 1, Rationality: 100}
	target := &Agent{ID: 2, Power: 80}

	start := w.opinionOf(observer, target.ID)
	startVariance := start.Variance
	if math.Abs(start.Strength-w.cfg.PriorStrength) > 1e-9 {
		t.Fatalf("a stranger is not assumed to be average: %v", start.Strength)
	}

	for i := 0; i < 200; i++ {
		w.observeStrength(observer, target, w.cfg.CombatObsVariance)
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
	target := &Agent{ID: 9, Power: 70}

	spread := func(rationality float64) float64 {
		total := 0.0
		for i := 0; i < 400; i++ {
			observer := &Agent{ID: 1, Rationality: rationality}
			w.observeStrength(observer, target, w.cfg.CombatObsVariance)
			total += math.Abs(w.opinionOf(observer, target.ID).Strength - 70)
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
	x := w.addAgent(Agent{X: 100, Y: 100, Power: 90, Rationality: 100, Vitality: 90, Hunger: 0})
	y := w.addAgent(Agent{X: 105, Y: 100, Power: 20, Rationality: 100, Vitality: 90, Hunger: 0})
	watcher := w.addAgent(Agent{X: 150, Y: 100, Power: 50, Rationality: 100, Vitality: 90, Hunger: 0})
	stranger := w.addAgent(Agent{X: 380, Y: 380, Power: 50, Rationality: 100, Vitality: 90, Hunger: 0})

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
	id := w.addAgent(Agent{X: 100, Y: 100, Vitality: 90, Hunger: 0})
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
	id := w.addAgent(Agent{X: 100, Y: 100, Vitality: 90, Hunger: 0})
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
	victim := w.addAgent(Agent{X: 100, Y: 100, Vitality: 90, Hunger: 0})
	bully := w.addAgent(Agent{X: 105, Y: 100, Power: 50, Vitality: 90, Hunger: 0})
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
		id := w.addAgent(Agent{
			X: 200, Y: 200, Sex: Male, Vitality: 95, Hunger: hunger,
			Power: 50, Rationality: 100, Intelligence: 100,
		})
		w.addAgent(Agent{
			X: 210, Y: 200, Sex: Female, Vitality: 100, Hunger: 0,
			Power: 90, Rationality: 90, Intelligence: 90,
		})
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
		victim := w.addAgent(Agent{
			X: 200, Y: 200, Sex: Male, Vitality: victimVitality, Hunger: 0,
			Power: 20, Rationality: 100, Intelligence: 100,
		})
		attacker := w.addAgent(Agent{
			X: 208, Y: 200, Sex: Male, Vitality: 100, Hunger: 0,
			Power: attackerPower, Rationality: 100, Intelligence: 100,
		})
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
	weak := w.addAgent(Agent{
		X: 200, Y: 200, Vitality: 60, Hunger: 50,
		Power: 5, Rationality: 100, Intelligence: 100,
	})
	giant := w.addAgent(Agent{X: 206, Y: 200, Vitality: 100, Hunger: 0, Power: 100})
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
				Power: 80, Rationality: 100, Intelligence: 100,
				FoodScarcity: scarcity,
			},
			Rand: w.rng,
		}
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
		id := w.addAgent(Agent{
			X: 200, Y: 200, Sex: Male, Vitality: 100, Hunger: 0,
			Power: 50, Rationality: 100, Intelligence: intelligence,
		})
		w.addAgent(Agent{X: 210, Y: 200, Sex: Female, Vitality: 100, Hunger: 0, Power: 50})
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
			Rand: w.rng,
		}
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
	id := w.addAgent(Agent{X: 200, Y: 200, Vitality: 90, Hunger: 0})
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
	id := w.addAgent(Agent{X: 200, Y: 200, Vitality: 90, Hunger: 0, Rationality: 100})
	spy := &spyController{}
	w.SetController(id, spy)
	w.addAgent(Agent{X: 220, Y: 200, Vitality: 90, Hunger: 0, Power: 97})

	w.Step()

	if len(spy.others) != 1 {
		t.Fatalf("saw %d others, want 1", len(spy.others))
	}
	if spy.others[0].EstStrength != cfg.PriorStrength {
		t.Fatalf("a stranger's strength came through as %v, want the prior %v and not the true 97",
			spy.others[0].EstStrength, cfg.PriorStrength)
	}
	if spy.self.Power != 0 {
		t.Fatalf("self power = %v, want the agent's own value", spy.self.Power)
	}
}

// --- lineage ---------------------------------------------------------------

// pairAboutToGiveBirth returns a world with a bonded couple whose bond ends on
// the next step.
func pairAboutToGiveBirth(t *testing.T, cfg Config) (*World, int, int) {
	t.Helper()
	w := NewWorld(cfg)
	male := w.addAgent(Agent{
		X: 100, Y: 100, Sex: Male, Power: 40, Rationality: 60, Intelligence: 30,
		Vitality: 90, Hunger: 0, PairTimer: 1, State: StatePaired, Generation: 2,
	})
	female := w.addAgent(Agent{
		X: 110, Y: 100, Sex: Female, Power: 60, Rationality: 80, Intelligence: 50,
		Vitality: 90, Hunger: 0, PairTimer: 1, State: StatePaired, Generation: 5,
	})
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

func TestChildInheritsParentsAverage(t *testing.T) {
	cfg := testConfig()
	cfg.MutationStd = 0 // isolate inheritance from mutation
	w, male, female := pairAboutToGiveBirth(t, cfg)

	w.Step()

	if got := w.Stats().Births; got != 1 {
		t.Fatalf("births = %d, want 1", got)
	}
	child := findChild(t, w, male, female)
	approx(t, child.Power, 50, 1e-9, "child power")             // (40 + 60) / 2
	approx(t, child.Rationality, 70, 1e-9, "child rationality") // (60 + 80) / 2
	approx(t, child.Intelligence, 40, 1e-9, "child intelligence")
	approx(t, child.Vitality, cfg.ChildVitality, 1e-9, "child vitality")
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

func TestMutationVariesChildAbility(t *testing.T) {
	cfg := testConfig()
	cfg.MutationStd = 4
	w := NewWorld(cfg)
	pa := &Agent{Power: 50, Rationality: 50, Intelligence: 50, Vitality: 90}
	pb := &Agent{Power: 50, Rationality: 50, Intelligence: 50, Vitality: 90}

	for i := 0; i < 50; i++ {
		pa.Vitality, pb.Vitality = 90, 90
		w.tryBirth(pa, pb)
	}
	if len(w.newborns) != 50 {
		t.Fatalf("newborns = %d, want 50", len(w.newborns))
	}
	varied := false
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
	pa := &Agent{Power: MaxAbility, Rationality: MinAbility, Intelligence: MaxAbility, Vitality: 90}
	pb := &Agent{Power: MaxAbility, Rationality: MinAbility, Intelligence: MaxAbility, Vitality: 90}

	for i := 0; i < 300; i++ {
		pa.Vitality, pb.Vitality = 90, 90
		w.tryBirth(pa, pb)
	}
	for i := range w.newborns {
		c := &w.newborns[i]
		for _, v := range []float64{c.Power, c.Rationality, c.Intelligence} {
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
	if w.patienceTicks(&Agent{Rationality: 90}) <= w.patienceTicks(&Agent{Rationality: 10}) {
		t.Fatal("the rational agent did not compare candidates for longer")
	}
}

// courting builds two agents next to each other, both able to reproduce and
// neither an obvious catch.
func courting(t *testing.T, cfg Config) (*World, int, int) {
	t.Helper()
	w := NewWorld(cfg)
	make := func(x float64, sex Sex) int {
		return w.addAgent(Agent{
			X: x, Y: 200, Sex: sex, Power: 40, Rationality: 100, Intelligence: 100,
			Vitality: cfg.ReproVitality + 5, Hunger: 0,
		})
	}
	male, female := make(200, Male), make(205, Female)
	if f := fitness(mustAgent(t, w, male)); f >= cfg.CommitFitness {
		t.Fatalf("test setup: fitness %v is an obvious catch, the pair would form instantly", f)
	}
	return w, male, female
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
	survivor := w.addAgent(Agent{
		X: 100, Y: 100, Sex: Male, Vitality: 90, Hunger: 0,
		State: StatePaired, PairTimer: 100,
	})
	dying := w.addAgent(Agent{
		X: 110, Y: 100, Sex: Female, Vitality: cfg.StarveRate / 2, Hunger: cfg.MaxHunger,
		State: StatePaired, PairTimer: 100,
	})
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

	for i := 0; i < 8000; i++ {
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
			w.addAgent(Agent{X: x, Y: y, Vitality: 100, Power: 50})
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
			w.addAgent(Agent{X: 20 + float64(i%16)*24, Y: 20 + float64(i/16)*24, Vitality: 100, Power: 50})
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
		w.addAgent(Agent{X: x, Y: y, Vitality: 100, Power: 50})
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
			w.addAgent(Agent{X: xs[i], Y: ys[i], Vitality: 100, Power: 50})
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
