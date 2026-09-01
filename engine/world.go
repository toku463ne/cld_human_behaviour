package engine

import (
	"math"
	"math/rand"
)

const (
	// How much of how good a mate somebody looks is the condition they are in
	// rather than the advertisement itself. The rest is the attractiveness
	// gene. Condition is in there at all because a dying agent is a poor bet
	// however fine it looks, and it is only a third of it because otherwise
	// "attractive" would just mean "recently fed".
	fitnessConditionWeight = 0.35

	// Speed of an agent that is following its partner rather than chasing a
	// target of its own.
	pairFollowEffort = 0.35

	// How often the onlookers around a fight, and the two in it, take a fresh
	// reading of each other. Updating on every single tick of a long fight
	// would cost a lot and tell them almost nothing new.
	spectateInterval = 20

	// How long ActObserve takes before it has told the watcher anything.
	observeTicks = 10
)

// Stats is an aggregated view of the world, cheap enough to compute per frame.
type Stats struct {
	Tick          int
	Population    int
	Males         int
	Females       int
	FoodItems     int
	Births        int
	Geniuses      int // births that drew an exceptional budget
	GreatGeniuses int // ... and the rarer, larger kind. Not counted in Geniuses
	Deaths        int
	Kills         int
	AgingDeaths   int // subset of Deaths caused by Lifespan reaching zero
	Fights        int
	MaxGeneration int

	AvgPower        float64
	AvgRationality  float64
	AvgIntelligence float64
	AvgVitality     float64
	AvgHunger       float64
}

// attack is one blow queued during a tick, applied once everybody has acted so
// that two agents hitting each other trade damage simultaneously.
type attack struct {
	fromID, toID int
	effort       float64
}

// World holds the whole simulation state. It knows nothing about rendering or
// networking: callers drive it with Step and read it back with Agents, Foods
// and Stats.
type World struct {
	cfg Config

	// rng is the single source of randomness of the simulation. Everything,
	// including the controllers, draws from it, so a given seed always
	// reproduces the same run.
	rng *rand.Rand

	agents []Agent
	foods  []Food

	// index maps an agent ID to its position in agents, and foodIndex does the
	// same for food, so that following a target does not scan everything.
	index     map[int]int
	foodIndex map[int]int

	// grid answers "who is near here" without walking the whole world, and
	// gridStale says it has not caught up with the world yet. It is built on
	// demand rather than kept up to date, so a tick that moves everybody pays
	// for one rebuild instead of a few hundred updates; see grid.go.
	grid      *spatialGrid
	gridStale bool

	// nearAgents and nearFoods are the candidate lists the index hands back,
	// reused between queries so that perceiving somebody allocates nothing.
	nearAgents []int
	nearFoods  []int

	// nearScratch is a third candidate list, for the two queries that happen
	// outside perceive: the food check that decides whether to think again,
	// and the onlookers of a fight. Neither overlaps with perceive or with the
	// other, so one buffer does for both.
	nearScratch []int

	// newborns buffers the children of this tick. They are appended after the
	// agent loop, because appending during it could move the backing array
	// while it is being walked through pointers.
	newborns []Agent

	// ai is the controller every agent uses unless one was installed for it.
	ai *AIController

	// perception is rebuilt for each decision instead of allocated.
	perception Perception

	// attacks buffers the blows of the current tick.
	attacks []attack

	// traces holds the decision log of the agents somebody asked to follow,
	// keyed by agent ID. Empty in a normal run: tracing is a debugging tool.
	traces map[int]*traceLog

	nextAgentID int
	nextFoodID  int

	tick      int
	foodAccum float64

	births        int
	geniuses      int
	greatGeniuses int
	deaths        int
	kills         int
	agingDeaths   int
	fights        int
	maxGeneration int
}

// NewWorld creates a world populated according to cfg. The same cfg (same seed
// included) always yields the same simulation.
func NewWorld(cfg Config) *World {
	w := &World{
		cfg:         cfg,
		rng:         rand.New(rand.NewSource(cfg.Seed)),
		agents:      make([]Agent, 0, cfg.InitialPopulation),
		foods:       make([]Food, 0, cfg.InitialFoodItems),
		index:       make(map[int]int, cfg.InitialPopulation),
		foodIndex:   make(map[int]int, cfg.InitialFoodItems),
		ai:          &AIController{},
		nextAgentID: 1,
		nextFoodID:  1,
	}
	for i := 0; i < cfg.InitialPopulation; i++ {
		w.addAgent(w.randomAgent())
	}
	for i := 0; i < cfg.InitialFoodItems; i++ {
		w.spawnFood()
	}
	return w
}

// Config returns the parameters this world runs with.
func (w *World) Config() Config { return w.cfg }

// Tick returns how many steps have been simulated.
func (w *World) Tick() int { return w.tick }

// Agents returns the living population. The slice is owned by the world and
// stays valid until the next Step.
func (w *World) Agents() []Agent { return w.agents }

// Foods returns the food items lying around. The slice is owned by the world
// and stays valid until the next Step.
func (w *World) Foods() []Food { return w.foods }

// AgentByID returns a copy of the agent with the given ID.
func (w *World) AgentByID(id int) (Agent, bool) {
	a := w.agentByID(id)
	if a == nil {
		return Agent{}, false
	}
	return *a, true
}

// SetController installs a controller on one agent. This is the seam the game
// will use: hand one node to a human player, and when that node dies hand the
// same controller to one of its children.
func (w *World) SetController(id int, c Controller) bool {
	a := w.agentByID(id)
	if a == nil {
		return false
	}
	a.controller = c
	a.requestDecision(TriggerControllerSet)
	return true
}

// Stats summarises the current population.
func (w *World) Stats() Stats {
	s := Stats{
		Tick:          w.tick,
		Population:    len(w.agents),
		FoodItems:     len(w.foods),
		Births:        w.births,
		Geniuses:      w.geniuses,
		GreatGeniuses: w.greatGeniuses,
		Deaths:        w.deaths,
		Kills:         w.kills,
		AgingDeaths:   w.agingDeaths,
		Fights:        w.fights,
		MaxGeneration: w.maxGeneration,
	}
	var sumPower, sumRationality, sumIntelligence, sumVitality, sumHunger float64
	for i := range w.agents {
		a := &w.agents[i]
		if a.Sex == Female {
			s.Females++
		} else {
			s.Males++
		}
		sumPower += a.Attack()
		sumRationality += a.Rationality()
		sumIntelligence += a.Intelligence()
		sumVitality += a.Vitality
		sumHunger += a.Hunger
	}
	if s.Population > 0 {
		n := float64(s.Population)
		s.AvgPower = sumPower / n
		s.AvgRationality = sumRationality / n
		s.AvgIntelligence = sumIntelligence / n
		s.AvgVitality = sumVitality / n
		s.AvgHunger = sumHunger / n
	}
	return s
}

// Step advances the simulation by one tick.
//
// The order matters: everybody decides on the world as it was, then everybody
// acts, then the blows land together. Otherwise the agent that happens to sit
// early in the slice would fight a world that has already moved.
func (w *World) Step() {
	w.tick++
	w.clearSpoiled()
	w.spawnFoodOfTick()

	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive || a.PartnerID != 0 {
			continue
		}
		if t := w.decisionTrigger(a); t != TriggerNone {
			w.decide(a, t)
		}
	}

	w.attacks = w.attacks[:0]
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		a.Age++
		a.effortSpent = 0
		a.actionTicks++
		if a.CooldownTimer > 0 {
			a.CooldownTimer--
		}

		if a.PartnerID != 0 {
			w.stepPaired(a)
		} else {
			w.perform(a)
		}
		a.pruneRejected(w.tick)
		w.keepInBounds(a)
	}

	w.resolveAttacks()
	w.metabolise()

	w.commitNewborns()
	w.removeDead()
}

// --- deciding --------------------------------------------------------------

// decisionTrigger reports what, if anything, is asking this agent to think
// again, and TriggerNone when nothing is. Deciding is trigger driven rather
// than continuous, both because it is the expensive part and because an agent
// that re-planned every tick would never follow a plan through.
func (w *World) decisionTrigger(a *Agent) Trigger {
	// Goal reached, goal lost, or some other event that already said so.
	if a.needsDecision {
		if a.pendingTrigger == TriggerNone {
			return TriggerRequested
		}
		return a.pendingTrigger
	}
	if a.lastAttackTick == w.tick-1 {
		return TriggerAttacked
	}
	// A noticeable dent in the vitality. This says "think again", not "run
	// away": what to do about it is up to the utility comparison.
	if a.vitalityAtDecision-a.Vitality >= w.cfg.TriggerVitalityDrop {
		return TriggerVitalityDrop
	}
	// Nothing has happened for a while, so an agent does not stay stuck on a
	// decision the world has moved past.
	if w.tick-a.lastDecisionTick >= w.cfg.TriggerIdleTicks {
		return TriggerIdle
	}
	// Food came into view while the agent had nothing better to do.
	if a.Action.Kind == ActRest || a.Action.Kind == ActMove {
		if w.nearestFoodInSight(a) >= 0 {
			return TriggerFoodInSight
		}
		// So did somebody worth crossing the world for. This adds no action:
		// courting is scored by the same utility comparison as everything
		// else. It only stops an agent walking past a candidate because it
		// happened not to be thinking at that moment.
		if a.CanReproduce(&w.cfg) && w.strikingCandidateInSight(a) {
			return TriggerMateInSight
		}
	}
	return TriggerNone
}

func (w *World) decide(a *Agent, trigger Trigger) {
	c := a.controller
	if c == nil {
		c = w.ai
	}

	p := w.perceive(a)
	// Only an agent somebody asked to follow records anything. The controller
	// fills in the options it compared; the world fills in the rest, so that a
	// controller which ignores the trace still leaves a usable record.
	p.Trace = nil
	if a.trace != nil {
		p.Trace = a.trace.begin(w.tick, a, trigger, p.Self)
	}

	a.Action = c.Decide(p)
	if p.Trace != nil {
		p.Trace.Action = a.Action
	}

	a.lastDecisionTick = w.tick
	a.vitalityAtDecision = a.Vitality
	a.needsDecision = false
	a.pendingTrigger = TriggerNone
	a.actionTicks = 0

	switch a.Action.Kind {
	case ActCourt:
		if a.State != StateSeekMate {
			a.courtStartTick = w.tick
		}
		a.State = StateSeekMate
	case ActAttack:
		a.State = StateFighting
	case ActFlee:
		a.State = StateFleeing
	case ActRest:
		a.State = StateResting
	default:
		a.State = StateForage
	}
}

// --- acting ----------------------------------------------------------------

func (w *World) perform(a *Agent) {
	switch a.Action.Kind {
	case ActRest:
		// Doing nothing is what lets a satiated agent recover.

	case ActMove:
		w.moveDir(a, a.Action.DX, a.Action.DY, a.Action.Effort)

	case ActEat:
		f := w.foodByID(a.Action.TargetID)
		if f == nil {
			a.requestDecision(TriggerTargetLost) // somebody else got it
			return
		}
		if dist2(a.X, a.Y, f.X, f.Y) > w.cfg.GrabRadius*w.cfg.GrabRadius {
			w.moveToward(a, f.X, f.Y, a.Action.Effort)
			return
		}
		w.eat(a, f.ID)
		a.requestDecision(TriggerGoalReached)

	case ActAttack:
		o := w.agentByID(a.Action.TargetID)
		if o == nil || !o.Alive {
			a.requestDecision(TriggerTargetLost)
			return
		}
		if dist2(a.X, a.Y, o.X, o.Y) > w.cfg.CombatRadius*w.cfg.CombatRadius {
			w.moveToward(a, o.X, o.Y, a.Action.Effort)
			return
		}
		w.attacks = append(w.attacks, attack{fromID: a.ID, toID: o.ID, effort: a.Action.Effort})
		a.Vitality -= w.cfg.AttackCost * a.Action.Effort
		a.effortSpent = math.Max(a.effortSpent, a.Action.Effort)

	case ActFlee:
		o := w.agentByID(a.Action.TargetID)
		if o == nil || !o.Alive {
			a.requestDecision(TriggerTargetLost)
			return
		}
		if dist2(a.X, a.Y, o.X, o.Y) > w.cfg.PerceptionRadius*w.cfg.PerceptionRadius {
			a.requestDecision(TriggerGoalReached) // out of sight, out of danger
			return
		}
		w.moveDir(a, a.X-o.X, a.Y-o.Y, a.Action.Effort)

	case ActObserve:
		o := w.agentByID(a.Action.TargetID)
		if o == nil || !o.Alive {
			a.requestDecision(TriggerTargetLost)
			return
		}
		if dist2(a.X, a.Y, o.X, o.Y) > w.cfg.PerceptionRadius*w.cfg.PerceptionRadius {
			w.moveToward(a, o.X, o.Y, a.Action.Effort)
			return
		}
		if a.actionTicks >= observeTicks {
			w.observeStrength(a, o, w.cfg.CombatObsVariance*w.cfg.SpectateObsFactor)
			a.requestDecision(TriggerGoalReached)
		}

	case ActCourt:
		w.court(a)
	}
}

// court walks up to a candidate and, once there, sees whether both sides are
// ready. Committing costs time, so an agent compares for a while first, and a
// pair only forms when both agree.
func (w *World) court(a *Agent) {
	o := w.agentByID(a.Action.TargetID)
	if o == nil || !o.Alive || o.PartnerID != 0 || o.Sex == a.Sex {
		a.requestDecision(TriggerTargetLost)
		return
	}
	if dist2(a.X, a.Y, o.X, o.Y) > w.cfg.GrabRadius*w.cfg.GrabRadius {
		w.moveToward(a, o.X, o.Y, a.Action.Effort)
		return
	}
	if !o.CanReproduce(&w.cfg) {
		// Approached somebody who has themselves to look after first.
		a.requestDecision(TriggerTargetLost)
		return
	}
	// Both sides have to be convinced, each on its own comparison clock.
	if w.willCommit(a, w.perceivedFitness(a, o)) && w.willCommit(o, w.perceivedFitness(o, a)) {
		w.bond(a, o)
		return
	}
	// Not convinced yet: put this one aside and go and see the others. As far
	// as the next decision is concerned this candidate is gone.
	a.reject(o.ID, w.tick+w.cfg.MateRejectDuration)
	a.requestDecision(TriggerTargetLost)
}

// stepPaired keeps two partners together until the bond has run its course,
// which is when their child is born.
func (w *World) stepPaired(a *Agent) {
	partner := w.agentByID(a.PartnerID)
	if partner == nil || !partner.Alive {
		w.releaseFromBond(a, w.cfg.MatingCooldown/2)
		return
	}
	// The bond is settled before anybody moves, so that both partners are
	// treated alike: otherwise whichever of them the loop reached first would
	// have paid for a step the other never took.
	a.PairTimer--
	if a.PairTimer <= 0 {
		// The lower ID of the pair performs the birth, so it happens once.
		if a.ID < partner.ID {
			w.tryBirth(a, partner)
		}
		w.releaseFromBond(a, w.cfg.MatingCooldown)
		w.releaseFromBond(partner, w.cfg.MatingCooldown)
		return
	}
	w.moveToward(a, partner.X, partner.Y, pairFollowEffort)
}

// --- combat ----------------------------------------------------------------

// resolveAttacks lands every blow of this tick at once.
//
// The asymmetry is the whole point: the target loses AttackDamage while the
// attacker only paid AttackCost. Two agents laying into each other therefore
// both pay both, and hitting somebody who is not hitting back — an ambush, or
// running down someone who is fleeing — is by far the cheapest damage
// available.
func (w *World) resolveAttacks() {
	for i := range w.attacks {
		at := &w.attacks[i]
		from, to := w.agentByID(at.fromID), w.agentByID(at.toID)
		if from == nil || to == nil || !from.Alive || !to.Alive {
			continue
		}
		w.fights++

		damage := damagePerTick(&w.cfg, from.Attack(), at.effort)
		to.noteHit(from.ID, w.tick)
		to.Vitality -= damage

		// The one taking the hits remembers exactly what they cost.
		w.rememberDamage(to, from.ID, damage)
		to.attackerID = from.ID
		to.lastAttackTick = w.tick

		if w.tick%spectateInterval == 0 {
			w.exchangeReadings(from, to)
		}
	}
}

// exchangeReadings updates what the two fighters and everybody watching believe
// about the strength of those involved. Fighting somebody teaches you the most
// about them; watching from the sidelines teaches you less, but it is free and
// it adds up.
func (w *World) exchangeReadings(x, y *Agent) {
	w.observeStrength(x, y, w.cfg.CombatObsVariance)
	w.observeStrength(y, x, w.cfg.CombatObsVariance)

	r := w.cfg.PerceptionRadius
	r2 := r * r
	spectated := w.cfg.CombatObsVariance * w.cfg.SpectateObsFactor
	// This runs after everybody has moved, so it is the second and last time
	// in a tick that the index is rebuilt. Nothing in the blows that follow
	// moves anybody, so the rest of the fights this tick reuse it.
	w.nearScratch = w.spatialIndex().appendAgentsNear(w.nearScratch[:0], x.X, x.Y, r)
	for _, i := range w.nearScratch {
		o := &w.agents[i]
		if !o.Alive || o.ID == x.ID || o.ID == y.ID {
			continue
		}
		if dist2(o.X, o.Y, x.X, x.Y) > r2 {
			continue
		}
		w.observeStrength(o, x, spectated)
		w.observeStrength(o, y, spectated)
	}
}

// --- metabolism ------------------------------------------------------------

// metabolise runs the one directional coupling between the three state axes:
// hunger climbs on its own, high hunger drains vitality, and an agent that is
// both fed and not exerting itself slowly recovers.
func (w *World) metabolise() {
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}

		a.Hunger = math.Min(w.cfg.MaxHunger, a.Hunger+a.HungerRate(&w.cfg))

		if drain := hungerDrain(&w.cfg, a.Hunger); drain > 0 {
			a.Vitality -= drain
		} else if a.Hunger <= w.cfg.SatiatedHunger {
			a.Vitality += w.cfg.RegenRate * (1 - clamp(a.effortSpent, 0, 1))
		}
		a.Vitality = math.Min(a.Vitality, a.MaxVitality(&w.cfg))

		w.spendLifespan(a)
		if !a.Alive {
			continue
		}

		// The comparison clock starts when an agent first has the means to
		// think past staying alive.
		if ready := a.CanReproduce(&w.cfg); ready != a.reproReady {
			a.reproReady = ready
			if ready {
				a.courtStartTick = w.tick
			}
		}

		if a.lastAttackTick < w.tick {
			a.attackerID = 0
		}
		if a.Vitality <= 0 {
			w.kill(a)
		}
	}
}

// spendLifespan is the only place Lifespan is ever touched. Chronic
// undernutrition and chronic overeating both wear it down, gated by the same
// hunger thresholds the vitality rules already use (StarveHunger) or a
// dedicated one for overeating (OverfedHunger). This is pure background
// bookkeeping: it is not part of Perception and never enters the utility
// formula, so nothing here is a decision an agent makes.
func (w *World) spendLifespan(a *Agent) {
	if a.Hunger > w.cfg.StarveHunger {
		a.Lifespan -= w.cfg.StarveLifespanRate
	}
	if a.Hunger < w.cfg.OverfedHunger {
		a.Lifespan -= w.cfg.OverfedLifespanRate
	}
	if a.Lifespan <= 0 {
		w.agingDeaths++
		w.kill(a)
	}
}

// --- reproduction ----------------------------------------------------------

func (w *World) bond(a, b *Agent) {
	a.State, b.State = StatePaired, StatePaired
	a.PartnerID, b.PartnerID = b.ID, a.ID
	a.PairTimer, b.PairTimer = w.cfg.PairBondDuration, w.cfg.PairBondDuration
}

func (w *World) releaseFromBond(a *Agent, cooldown int) {
	a.State = StateForage
	a.PartnerID = 0
	a.PairTimer = 0
	a.CooldownTimer = cooldown
	a.requestDecision(TriggerBondEnded)
}

// inheritGene draws one ability for a child: the value of one parent or the
// other, chosen by a coin, and now and then a mutation on top.
//
// The coin is thrown separately for every gene, so a child can take its power
// from one parent and its wits from the other. There are no linkage groups and
// no chromosome tied to sex; a gene is inherited on its own.
//
// This is particulate inheritance, and the reason for it is that the obvious
// alternative does not work. Averaging the parents halves the variance of every
// ability each generation, so within a few generations the only variation left
// is whatever mutation has just put in, and selection has almost nothing to
// choose between. Taking one parent's value whole keeps the variation in the
// population instead of averaging it away.
func (w *World) inheritGene(pa, pb float64) float64 {
	gene := pa
	if w.rng.Intn(2) == 1 {
		gene = pb
	}
	// Mutation is rare and large. Adding a little to every gene of every child
	// would undo what taking a parent's value whole is for: the value would
	// drift a bit at every birth and no parent's number would survive intact.
	// The two shapes can be set to inject the same variance per birth
	// (MutationRate x MutationStd^2), so the choice between them is about how
	// that variance arrives, not how much of it there is.
	if w.cfg.MutationRate <= 0 || w.rng.Float64() >= w.cfg.MutationRate {
		return gene
	}
	return gene + w.rng.NormFloat64()*w.cfg.MutationStd
}

// inheritBudget decides how much a child gets to be made of.
//
// It comes from one parent or the other, chosen by a coin, and never from the
// average of the two: an average halves the variance of the budget every
// generation, which is exactly what blending inheritance was dropped for.
// BudgetHeritability mixes that in against the population mean, so that a
// world where the budget is not inherited at all can be run from the same
// binary.
//
// The genome is then scaled onto the result, so what a child inherits gene by
// gene is the shape of its parents and what it inherits here is the size. A
// mutation moves the split rather than adding to the total.
func (w *World) inheritBudget(pa, pb *Agent) float64 {
	from := pa
	if w.rng.Intn(2) == 1 {
		from = pb
	}
	h := clamp(w.cfg.BudgetHeritability, 0, 1)
	budget := h*from.Budget() + (1-h)*w.cfg.GeneBudgetMean

	// Now and then somebody is born with far more to be made of than either
	// parent had. The roll is taken every birth so that the random source is
	// consumed the same way whether or not it lands.
	switch roll := w.rng.Float64(); {
	case roll < w.cfg.GreatGeniusRate:
		budget = w.cfg.GreatGeniusBudget
		w.greatGeniuses++
	case roll < w.cfg.GreatGeniusRate+w.cfg.GeniusRate:
		budget = w.cfg.GeniusBudget
		w.geniuses++
	}
	return budget + w.rng.NormFloat64()*w.cfg.BudgetInheritSpread
}

// tryBirth produces a child that takes each ability from one parent or the
// other, plus a mutation. Over the generations this is what makes the
// population evolve.
func (w *World) tryBirth(pa, pb *Agent) {
	if len(w.agents)+len(w.newborns) >= w.cfg.MaxPopulation {
		return
	}
	share := w.cfg.BirthVitalityCost / 2
	if pa.Vitality <= share || pb.Vitality <= share {
		return
	}
	pa.Vitality -= share
	pb.Vitality -= share

	// Drawn into variables rather than inline, so that the order the random
	// source is consumed in is on the page instead of in the argument
	// evaluation order.
	// Every gene, not just the ones with rules today: a gene nobody reads is
	// still paid for out of the budget, and it still has to reach the
	// generation that gives it a job.
	genome := make([]float64, max(len(pa.Genome), len(pb.Genome)))
	for i := range genome {
		genome[i] = w.inheritGene(pa.Gene(Gene(i)), pb.Gene(Gene(i)))
	}
	fitBudget(genome, w.inheritBudget(pa, pb))

	child := w.newAgent(
		(pa.X+pb.X)/2+w.randRange(-8, 8),
		(pa.Y+pb.Y)/2+w.randRange(-8, 8),
		w.randomSex(),
		genome,
		max(pa.Generation, pb.Generation)+1,
	)
	child.ParentIDs = [2]int{pa.ID, pb.ID}

	if child.Generation > w.maxGeneration {
		w.maxGeneration = child.Generation
	}
	w.newborns = append(w.newborns, child)
	w.births++
}

func (w *World) kill(a *Agent) {
	a.Alive = false
	w.deaths++
	w.dropMeat(a)
	if a.lastAttackTick >= w.tick-1 {
		w.kills++
	}
	if a.PartnerID != 0 {
		if p := w.agentByID(a.PartnerID); p != nil && p.Alive {
			w.releaseFromBond(p, w.cfg.MatingCooldown/2)
		}
		a.PartnerID = 0
	}
}

func (w *World) eat(a *Agent, foodID int) {
	f := w.foodByID(foodID)
	if f == nil || !w.canEat(a, f) {
		a.requestDecision(TriggerTargetLost)
		return
	}
	a.Hunger = math.Max(0, a.Hunger-w.cfg.FoodNutrition)
	w.removeFoodByID(foodID)
}

// --- judgement -------------------------------------------------------------

// fitness is how good a mate an agent is: what it can pass on, and the shape it
// is in to raise a child.
func fitness(a *Agent, cfg *Config) float64 {
	condition := 0.0
	if maxV := a.MaxVitality(cfg); maxV > 0 {
		condition = clamp(a.Vitality/maxV, 0, 1) * MaxAbility
	}
	return a.Gene(GeneAttractiveness)*(1-fitnessConditionWeight) +
		condition*fitnessConditionWeight
}

func (w *World) perceivedFitness(observer, target *Agent) float64 {
	return fitness(target, &w.cfg) + w.judgementError(observer, w.cfg.JudgementNoise*0.5)
}

// patienceTicks is how long an agent keeps comparing candidates before it is
// willing to settle. A rational agent can afford to wait and compare longer.
func (w *World) patienceTicks(a *Agent) int {
	return int(w.cfg.PatienceBase + a.Rationality()*w.cfg.PatienceRationality)
}

// willCommit reports whether an agent accepts the candidate in front of it:
// either it has compared long enough, or the candidate is an obvious catch.
func (w *World) willCommit(a *Agent, candidateFitness float64) bool {
	if w.tick-a.courtStartTick >= w.patienceTicks(a) {
		return true
	}
	return candidateFitness >= w.cfg.CommitFitness
}

// --- neighbourhood queries -------------------------------------------------
//
// nearestFoodInSight returns the index of the closest food item within
// perception range, or -1.
//
// Ties go to the last item examined, as they did when this walked the whole
// list, which is why the candidates have to arrive in the order they sit in.
func (w *World) nearestFoodInSight(a *Agent) int {
	r := w.cfg.PerceptionRadius
	best, bestDist := -1, r*r
	w.nearScratch = w.spatialIndex().appendFoodsNear(w.nearScratch[:0], a.X, a.Y, r)
	for _, i := range w.nearScratch {
		f := &w.foods[i]
		if !w.canEat(a, f) {
			continue
		}
		if d := dist2(a.X, a.Y, f.X, f.Y); d <= bestDist {
			bestDist, best = d, i
		}
	}
	return best
}

// strikingCandidateInSight reports whether somebody this agent would settle
// for at once has come into view: the same bar the patience rule uses, so an
// agent is only interrupted for a candidate it would not have compared against
// others anyway.
func (w *World) strikingCandidateInSight(a *Agent) bool {
	r := w.cfg.PerceptionRadius
	w.nearScratch = w.spatialIndex().appendAgentsNear(w.nearScratch[:0], a.X, a.Y, r)
	for _, i := range w.nearScratch {
		o := &w.agents[i]
		if !o.Alive || o.ID == a.ID || o.Sex == a.Sex || o.PartnerID != 0 || a.isRejected(o.ID) {
			continue
		}
		if dist2(a.X, a.Y, o.X, o.Y) > r*r {
			continue
		}
		if w.perceivedFitness(a, o) >= w.cfg.CommitFitness {
			return true
		}
	}
	return false
}

func (w *World) agentByID(id int) *Agent {
	i, ok := w.index[id]
	if !ok {
		return nil
	}
	return &w.agents[i]
}

func (w *World) foodByID(id int) *Food {
	i, ok := w.foodIndex[id]
	if !ok {
		return nil
	}
	return &w.foods[i]
}

// --- movement --------------------------------------------------------------

// moveToward walks one tick towards a point. Speed grows with the square root
// of the effort while the cost grows linearly, so covering ground in a hurry
// costs more vitality per unit of distance than taking it steady.
func (w *World) moveToward(a *Agent, tx, ty, effort float64) {
	w.moveDir(a, tx-a.X, ty-a.Y, effort)
}

func (w *World) moveDir(a *Agent, dx, dy, effort float64) {
	d := math.Hypot(dx, dy)
	if d < 1e-9 {
		return
	}
	effort = clamp(effort, 0, 1)
	speed := speedAt(a.MaxSpeed(&w.cfg), effort)
	a.X += dx / d * speed
	a.Y += dy / d * speed
	a.VX, a.VY = dx/d, dy/d
	a.Vitality -= moveCostAt(&w.cfg, effort)
	a.effortSpent = math.Max(a.effortSpent, effort)
	w.invalidateIndex()
}

func (w *World) keepInBounds(a *Agent) {
	m := w.cfg.BoundaryMargin
	if a.X < m || a.X > w.cfg.Width-m || a.Y < m || a.Y > w.cfg.Height-m {
		w.invalidateIndex()
	}
	if a.X < m {
		a.X, a.VX = m, math.Abs(a.VX)
	}
	if a.X > w.cfg.Width-m {
		a.X, a.VX = w.cfg.Width-m, -math.Abs(a.VX)
	}
	if a.Y < m {
		a.Y, a.VY = m, math.Abs(a.VY)
	}
	if a.Y > w.cfg.Height-m {
		a.Y, a.VY = w.cfg.Height-m, -math.Abs(a.VY)
	}
}

// --- population bookkeeping ------------------------------------------------

// newAgent builds an agent without inserting it into the world.
func (w *World) newAgent(x, y float64, sex Sex, genome []float64, generation int) Agent {
	for i := range genome {
		genome[i] = clamp(genome[i], MinAbility, MaxAbility)
	}
	a := Agent{
		X: x, Y: y,
		VX:         w.randRange(-0.3, 0.3),
		VY:         w.randRange(-0.3, 0.3),
		Sex:        sex,
		Genome:     genome,
		Vitality:   0, // filled in below, once the genome says how big it is
		Hunger:     w.cfg.ChildHunger,
		Generation: generation,
		Alive:      true,
		Lifespan:   w.cfg.MaxLifespan,
	}
	a.Vitality = w.cfg.ChildVitalityShare * a.MaxVitality(&w.cfg)
	return a
}

func (w *World) randomAgent() Agent {
	a := w.newAgent(
		w.randRange(20, w.cfg.Width-20),
		w.randRange(20, w.cfg.Height-20),
		w.randomSex(),
		w.drawGenome(),
		0,
	)
	a.Vitality = w.randRange(a.MaxVitality(&w.cfg)*0.6, a.MaxVitality(&w.cfg))
	a.Hunger = w.randRange(0, w.cfg.SatiatedHunger)
	// Founders are spread across a range of remaining lifespan too, the same
	// way they are already spread across vitality and hunger, so the first
	// generation does not all reach zero at once.
	a.Lifespan = w.randRange(w.cfg.MaxLifespan*0.5, w.cfg.MaxLifespan)
	return a
}

// addAgent inserts an agent into the world and returns its assigned ID.
func (w *World) addAgent(a Agent) int {
	a.ID = w.nextAgentID
	w.nextAgentID++
	a.Alive = true
	a.requestDecision(TriggerSpawned)
	if a.Vitality <= 0 {
		a.Vitality = a.MaxVitality(&w.cfg)
	}
	if a.Lifespan <= 0 {
		a.Lifespan = w.cfg.MaxLifespan
	}
	// Every agent owns its genome: a literal built by a test may not have one
	// at all, and two agents must never end up sharing a backing array.
	// An agent arriving without a genome at all - a test literal, mostly - is
	// an average one rather than a creature with nothing in it, which with a
	// vitality gene of zero would be a body unable to hold any vitality.
	bare := len(a.Genome) == 0
	a.Genome = cloneGenome(a.Genome)
	if bare {
		for i := range a.Genome {
			a.Genome[i] = midAbility
		}
	}
	w.index[a.ID] = len(w.agents)
	w.agents = append(w.agents, a)
	w.invalidateIndex()
	return a.ID
}

// commitNewborns adds this tick's children and closes the lineage links, so
// that a parent can be followed to its descendants later.
func (w *World) commitNewborns() {
	if len(w.newborns) == 0 {
		return
	}
	for i := range w.newborns {
		id := w.addAgent(w.newborns[i])
		for _, parentID := range w.newborns[i].ParentIDs {
			if p := w.agentByID(parentID); p != nil {
				p.ChildIDs = append(p.ChildIDs, id)
			}
		}
	}
	w.newborns = w.newborns[:0]
}

// removeDead compacts the population in place. Doing it every tick keeps
// Agents() a list of living agents only.
func (w *World) removeDead() {
	n := 0
	for i := range w.agents {
		if !w.agents[i].Alive {
			continue
		}
		if n != i {
			w.agents[n] = w.agents[i]
		}
		n++
	}
	if n == len(w.agents) {
		return
	}
	w.agents = w.agents[:n]
	clear(w.index)
	for i := range w.agents {
		w.index[w.agents[i].ID] = i
	}
	// Compacting moved everybody who was behind a corpse, so every index the
	// grid holds past that point is now somebody else.
	w.invalidateIndex()
}

// --- food ------------------------------------------------------------------

// spawnFoodOfTick grows food at the configured rate, carrying the fractional
// part over to the next tick.
func (w *World) spawnFoodOfTick() {
	w.foodAccum += w.cfg.FoodSpawnRate
	for w.foodAccum >= 1 {
		w.spawnFood()
		w.foodAccum--
	}
}

func (w *World) spawnFood() {
	// Checked before drawing the position so that a full world does not consume
	// randomness and shift the rest of the run.
	if len(w.foods) >= w.cfg.MaxFoodItems {
		return
	}
	w.addFood(w.randRange(10, w.cfg.Width-10), w.randRange(10, w.cfg.Height-10))
}

// addFood puts a food item at the given position and returns its ID, or 0 when
// the world already holds as many items as it may.
func (w *World) addFood(x, y float64) int {
	if len(w.foods) >= w.cfg.MaxFoodItems {
		return 0
	}
	f := Food{ID: w.nextFoodID, X: x, Y: y}
	w.nextFoodID++
	w.foodIndex[f.ID] = len(w.foods)
	w.foods = append(w.foods, f)
	w.invalidateIndex()
	return f.ID
}

// removeFoodByID drops an item. The order of w.foods carries no meaning, so the
// last item is swapped into the hole.
func (w *World) removeFoodByID(id int) {
	i, ok := w.foodIndex[id]
	if !ok {
		return
	}
	last := len(w.foods) - 1
	w.foods[i] = w.foods[last]
	w.foodIndex[w.foods[i].ID] = i
	w.foods = w.foods[:last]
	delete(w.foodIndex, id)
	// The last item was swapped into the hole, so two indices changed meaning.
	w.invalidateIndex()
}

// --- small helpers ---------------------------------------------------------

func (w *World) randRange(lo, hi float64) float64 {
	return lo + w.rng.Float64()*(hi-lo)
}

func (w *World) randomSex() Sex {
	if w.rng.Float64() < 0.5 {
		return Male
	}
	return Female
}

func dist2(ax, ay, bx, by float64) float64 {
	dx, dy := ax-bx, ay-by
	return dx*dx + dy*dy
}

func clamp(v, lo, hi float64) float64 {
	return math.Min(hi, math.Max(lo, v))
}
