package engine

import (
	"math"
	"math/rand"
)

// Weights of the traits that make an agent an attractive mate: a partner with
// high ability and resources gives the offspring a better starting position.
const (
	fitnessPowerWeight       = 0.5
	fitnessRationalityWeight = 0.2
	fitnessFoodWeight        = 0.3

	// Stored food also helps in a fight over the next meal.
	contestFoodWeight = 0.15

	// Food beyond this amount stops adding any advantage.
	usefulFoodCap = 100.0

	// Speed factor of a paired agent, which follows its partner instead of
	// chasing a target of its own.
	pairFollowSpeedFactor = 0.6
)

// Stats is an aggregated view of the world, cheap enough to compute per frame.
type Stats struct {
	Tick           int
	Population     int
	Males          int
	Females        int
	FoodItems      int
	Births         int
	Deaths         int
	MaxGeneration  int
	AvgPower       float64
	AvgRationality float64
}

// World holds the whole simulation state. It knows nothing about rendering or
// networking: callers drive it with Step and read it back with Agents, Foods
// and Stats.
type World struct {
	cfg Config

	// rng is the single source of randomness of the simulation. Everything goes
	// through it, so a given seed always produces the same run.
	rng *rand.Rand

	agents []Agent
	foods  []Food

	// index maps an agent ID to its position in agents, so that looking up a
	// partner does not scan the whole population.
	index map[int]int

	// newborns buffers the children of this tick. They are appended after the
	// agent loop, because appending during the loop could move the backing
	// array while it is being walked through pointers.
	newborns []Agent

	nextAgentID int
	nextFoodID  int

	tick      int
	foodAccum float64

	births        int
	deaths        int
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

// Stats summarises the current population.
func (w *World) Stats() Stats {
	s := Stats{
		Tick:          w.tick,
		Population:    len(w.agents),
		FoodItems:     len(w.foods),
		Births:        w.births,
		Deaths:        w.deaths,
		MaxGeneration: w.maxGeneration,
	}
	var sumPower, sumRationality float64
	for i := range w.agents {
		a := &w.agents[i]
		if a.Sex == Female {
			s.Females++
		} else {
			s.Males++
		}
		sumPower += a.Power
		sumRationality += a.Rationality
	}
	if s.Population > 0 {
		n := float64(s.Population)
		s.AvgPower = sumPower / n
		s.AvgRationality = sumRationality / n
	}
	return s
}

// Step advances the simulation by one tick.
func (w *World) Step() {
	w.tick++
	w.spawnFoodOfTick()

	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}

		a.Age++
		a.Food -= w.cfg.Metabolism

		if a.Food <= 0 || a.Age >= a.MaxAge {
			w.kill(a)
			continue
		}

		if a.CooldownTimer > 0 {
			a.CooldownTimer--
		}

		if a.State == StatePaired {
			w.stepPaired(a)
			continue
		}

		// Priority 1 is staying alive: a hungry agent forages, no matter what
		// else is around. Priority 2, looking for a mate, is only reachable
		// with a comfortable reserve and after the previous bond's cooldown.
		hungry := a.Food < w.cfg.FoodLowThreshold
		canSeekMate := !hungry && a.Food >= w.cfg.ReproFoodThreshold && a.CooldownTimer <= 0
		if canSeekMate {
			if a.State != StateSeekMate {
				a.State = StateSeekMate
				a.courtStartTick = w.tick
			}
		} else {
			a.State = StateForage
		}

		if a.State == StateSeekMate {
			w.stepSeekMate(a)
		} else {
			w.stepForage(a)
		}

		a.pruneRejected(w.tick)
		w.keepInBounds(a)
	}

	w.commitNewborns()
	w.removeDead()
}

// --- behaviour -------------------------------------------------------------

// stepForage looks for the nearest food. If somebody else is already on it, the
// agent first estimates whether the fight is winnable; a contest it cannot win
// is not worth entering, so it goes looking somewhere else instead.
func (w *World) stepForage(a *Agent) {
	fi := w.nearestFood(a)
	if fi < 0 {
		w.wander(a)
		return
	}

	f := &w.foods[fi]
	w.moveToward(a, f.X, f.Y, w.cfg.AgentSpeed)
	if dist2(a.X, a.Y, f.X, f.Y) > w.cfg.GrabRadius*w.cfg.GrabRadius {
		return
	}

	rival := w.contestantFor(a, f)
	if rival == nil {
		w.eat(a, fi)
		return
	}

	own := w.effectiveContestPower(a)
	if w.perceivedPower(a, rival) > own*w.cfg.ContestAvoidMargin {
		// Judged too strong: do not fight on that ground, look elsewhere.
		a.reject(rejectFood, f.ID, w.tick+w.cfg.FoodRejectDuration)
		return
	}

	own += w.rng.NormFloat64() * w.cfg.ContestNoise
	theirs := w.effectiveContestPower(rival) + w.rng.NormFloat64()*w.cfg.ContestNoise
	if own >= theirs {
		w.eat(a, fi)
	} else {
		a.Food = math.Max(0, a.Food-w.cfg.ContestLossPenalty)
	}
}

// stepSeekMate walks towards the most promising candidate in sight. Committing
// costs time, so an agent compares candidates for a while before settling, and
// a pair is only formed when both sides agree.
func (w *World) stepSeekMate(a *Agent) {
	best, bestFitness := w.bestCandidate(a)
	if best == nil {
		w.wander(a)
		return
	}

	w.moveToward(a, best.X, best.Y, w.cfg.AgentSpeed)
	if dist2(a.X, a.Y, best.X, best.Y) > w.cfg.GrabRadius*w.cfg.GrabRadius {
		return
	}
	if best.State != StateSeekMate || best.PartnerID != 0 {
		return
	}

	if w.willCommit(a, bestFitness) && w.willCommit(best, w.perceivedFitness(best, a)) {
		w.bond(a, best)
		return
	}
	// Not convinced yet: keep this one aside and go and see the others.
	a.reject(rejectMate, best.ID, w.tick+w.cfg.MateRejectDuration)
}

// stepPaired keeps the two partners together until the bond has run its course,
// which is when their child is born.
func (w *World) stepPaired(a *Agent) {
	partner := w.agentByID(a.PartnerID)
	if partner == nil || !partner.Alive {
		w.releaseFromBond(a, w.cfg.MatingCooldown/2)
		return
	}

	w.moveToward(a, partner.X, partner.Y, w.cfg.AgentSpeed*pairFollowSpeedFactor)
	a.PairTimer--
	if a.PairTimer <= 0 {
		// The lower ID of the pair performs the birth, so it happens once.
		if a.ID < partner.ID {
			w.tryBirth(a, partner)
		}
		w.releaseFromBond(a, w.cfg.MatingCooldown)
		w.releaseFromBond(partner, w.cfg.MatingCooldown)
	}
	w.keepInBounds(a)
}

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
}

// tryBirth produces a child whose abilities are the average of its parents plus
// a mutation. Over the generations this is what makes the population evolve.
func (w *World) tryBirth(pa, pb *Agent) {
	if len(w.agents)+len(w.newborns) >= w.cfg.MaxPopulation {
		return
	}
	if pa.Food+pb.Food < w.cfg.BirthCost {
		return
	}

	share := w.cfg.BirthCost / 2
	pa.Food -= share
	pb.Food -= share

	child := w.newAgent(
		(pa.X+pb.X)/2+w.randRange(-8, 8),
		(pa.Y+pb.Y)/2+w.randRange(-8, 8),
		w.randomSex(),
		(pa.Power+pb.Power)/2+w.rng.NormFloat64()*w.cfg.MutationStd,
		(pa.Rationality+pb.Rationality)/2+w.rng.NormFloat64()*w.cfg.MutationStd,
		w.cfg.ChildInitialFood,
		max(pa.Generation, pb.Generation)+1,
	)

	if child.Generation > w.maxGeneration {
		w.maxGeneration = child.Generation
	}
	w.newborns = append(w.newborns, child)
	w.births++
}

func (w *World) kill(a *Agent) {
	a.Alive = false
	w.deaths++
	if a.PartnerID != 0 {
		if p := w.agentByID(a.PartnerID); p != nil && p.Alive {
			w.releaseFromBond(p, w.cfg.MatingCooldown/2)
		}
		a.PartnerID = 0
	}
}

func (w *World) eat(a *Agent, foodIndex int) {
	a.Food = math.Min(w.cfg.MaxFoodStore, a.Food+w.cfg.FoodNutrition)
	w.removeFood(foodIndex)
}

// --- judgement -------------------------------------------------------------

// fitness is how good a mate an agent is: ability plus the resources it brings.
func fitness(a *Agent) float64 {
	return a.Power*fitnessPowerWeight +
		a.Rationality*fitnessRationalityWeight +
		math.Min(a.Food, usefulFoodCap)*fitnessFoodWeight
}

// effectiveContestPower is the real strength an agent brings to a fight over
// food. Holding resources is an advantage in itself.
func (w *World) effectiveContestPower(a *Agent) float64 {
	return a.Power + math.Min(a.Food, usefulFoodCap)*contestFoodWeight
}

// judgementError draws the error an observer makes when sizing up somebody
// else. Being able to read the situation is an ability of its own: the higher
// the rationality, the smaller the error.
func (w *World) judgementError(observer *Agent, scale float64) float64 {
	std := (MaxAbility - observer.Rationality) / MaxAbility * scale
	if std <= 0 {
		return 0
	}
	return w.rng.NormFloat64() * std
}

func (w *World) perceivedPower(observer, rival *Agent) float64 {
	return w.effectiveContestPower(rival) + w.judgementError(observer, w.cfg.JudgementNoise)
}

func (w *World) perceivedFitness(observer, target *Agent) float64 {
	return fitness(target) + w.judgementError(observer, w.cfg.JudgementNoise*0.5)
}

// patienceTicks is how long an agent keeps comparing candidates before it is
// willing to settle. A rational agent can afford to wait and compare longer.
func (w *World) patienceTicks(a *Agent) int {
	return int(w.cfg.PatienceBase + a.Rationality*w.cfg.PatienceRationality)
}

// willCommit reports whether an agent accepts the candidate it is standing next
// to: either it has compared long enough, or the candidate is obviously good.
func (w *World) willCommit(a *Agent, candidateFitness float64) bool {
	if w.tick-a.courtStartTick >= w.patienceTicks(a) {
		return true
	}
	return candidateFitness >= w.cfg.CommitFitness
}

// --- neighbourhood queries -------------------------------------------------
//
// These are plain linear scans, which is enough for the few hundred agents the
// simulation runs with. If the population grows, only the bodies of these
// functions need to be replaced by a spatial index.

// nearestFood returns the index in w.foods of the closest food item the agent
// is still interested in, or -1.
func (w *World) nearestFood(a *Agent) int {
	best, bestDist := -1, math.Inf(1)
	for i := range w.foods {
		f := &w.foods[i]
		if a.isRejected(rejectFood, f.ID) {
			continue
		}
		if d := dist2(a.X, a.Y, f.X, f.Y); d < bestDist {
			bestDist, best = d, i
		}
	}
	return best
}

// contestantFor returns another agent already standing on the food item, if any.
func (w *World) contestantFor(a *Agent, f *Food) *Agent {
	r2 := w.cfg.GrabRadius * w.cfg.GrabRadius
	for i := range w.agents {
		o := &w.agents[i]
		if !o.Alive || o.ID == a.ID {
			continue
		}
		if dist2(o.X, o.Y, f.X, f.Y) <= r2 {
			return o
		}
	}
	return nil
}

// bestCandidate returns the most attractive candidate within perception range,
// as this agent perceives it, together with that perceived fitness.
func (w *World) bestCandidate(a *Agent) (*Agent, float64) {
	r2 := w.cfg.PerceptionRadius * w.cfg.PerceptionRadius
	var best *Agent
	bestFitness := math.Inf(-1)
	for i := range w.agents {
		o := &w.agents[i]
		if !o.Alive || o.ID == a.ID || o.Sex == a.Sex {
			continue
		}
		if o.State == StatePaired || o.CooldownTimer > 0 {
			continue
		}
		if a.isRejected(rejectMate, o.ID) {
			continue
		}
		if dist2(a.X, a.Y, o.X, o.Y) > r2 {
			continue
		}
		if f := w.perceivedFitness(a, o); f > bestFitness {
			bestFitness, best = f, o
		}
	}
	return best, bestFitness
}

func (w *World) agentByID(id int) *Agent {
	i, ok := w.index[id]
	if !ok {
		return nil
	}
	return &w.agents[i]
}

// --- movement --------------------------------------------------------------

func (w *World) moveToward(a *Agent, tx, ty, speed float64) {
	dx, dy := tx-a.X, ty-a.Y
	d := math.Hypot(dx, dy)
	if d < 1e-9 {
		return
	}
	a.X += dx / d * speed
	a.Y += dy / d * speed
}

// wander is what an agent does with no target in sight.
func (w *World) wander(a *Agent) {
	a.VX = clamp(a.VX+w.randRange(-0.25, 0.25), -1, 1)
	a.VY = clamp(a.VY+w.randRange(-0.25, 0.25), -1, 1)
	a.X += a.VX
	a.Y += a.VY
}

func (w *World) keepInBounds(a *Agent) {
	m := w.cfg.BoundaryMargin
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
func (w *World) newAgent(x, y float64, sex Sex, power, rationality, food float64, generation int) Agent {
	return Agent{
		X: x, Y: y,
		VX:          w.randRange(-0.3, 0.3),
		VY:          w.randRange(-0.3, 0.3),
		Sex:         sex,
		Power:       clamp(power, MinAbility, MaxAbility),
		Rationality: clamp(rationality, MinAbility, MaxAbility),
		Food:        clamp(food, 0, w.cfg.MaxFoodStore),
		MaxAge:      w.cfg.MinLifespan + w.rng.Intn(w.cfg.MaxLifespan-w.cfg.MinLifespan+1),
		Generation:  generation,
		Alive:       true,
	}
}

func (w *World) randomAgent() Agent {
	return w.newAgent(
		w.randRange(20, w.cfg.Width-20),
		w.randRange(20, w.cfg.Height-20),
		w.randomSex(),
		w.randRange(25, 75),
		w.randRange(25, 75),
		w.randRange(55, 90),
		0,
	)
}

// addAgent inserts an agent into the world and returns its assigned ID.
func (w *World) addAgent(a Agent) int {
	a.ID = w.nextAgentID
	w.nextAgentID++
	a.Alive = true
	if a.MaxAge <= 0 {
		a.MaxAge = w.cfg.MaxLifespan
	}
	w.index[a.ID] = len(w.agents)
	w.agents = append(w.agents, a)
	return a.ID
}

func (w *World) commitNewborns() {
	if len(w.newborns) == 0 {
		return
	}
	for i := range w.newborns {
		w.addAgent(w.newborns[i])
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
	w.reindex()
}

func (w *World) reindex() {
	clear(w.index)
	for i := range w.agents {
		w.index[w.agents[i].ID] = i
	}
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
	w.foods = append(w.foods, f)
	return f.ID
}

// removeFood drops the item at the given index. The order of w.foods carries no
// meaning, so the last item is swapped in.
func (w *World) removeFood(i int) {
	last := len(w.foods) - 1
	w.foods[i] = w.foods[last]
	w.foods = w.foods[:last]
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
