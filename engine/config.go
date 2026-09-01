package engine

// Config holds every tunable parameter of the simulation.
//
// Behaviour constants live here instead of package level constants so that a
// test can neutralize one rule at a time (for example MutationStd = 0 makes a
// child's ability the exact average of its parents, and FoodSpawnRate = 0 keeps
// the food layout under the test's control). Config doubles as the list of
// everything the rules depend on.
type Config struct {
	// --- world ---
	Width  float64
	Height float64
	Seed   int64

	InitialPopulation int
	InitialFoodItems  int
	MaxPopulation     int
	MaxFoodItems      int
	FoodSpawnRate     float64 // expected number of food items spawned per tick

	// --- state model: vitality, hunger and food ---
	//
	// The three are coupled in one direction only: eating lowers hunger, low
	// hunger lets vitality recover, moving and fighting spend vitality, and
	// being left alone raises hunger which eventually drains vitality. That
	// guarantees every agent has a way back up instead of only decaying.
	MaxVitality float64
	MaxHunger   float64
	HungerRate  float64 // hunger gained per tick

	FoodNutrition  float64 // hunger removed by eating one item
	StarveHunger   float64 // above this hunger, vitality starts to drain
	StarveRate     float64 // vitality lost per tick at maximum hunger
	SatiatedHunger float64 // below this hunger, a resting agent recovers
	RegenRate      float64 // vitality regained per tick while satiated and idle

	// --- lifespan: background wear, spent by metabolise alone ---
	//
	// This is deliberately mechanical. Lifespan never appears in Perception and
	// never enters the utility formula (no goal term reads it), so no agent
	// knows its own remaining lifespan or that ageing is a cause of death.
	// Selection on it can only act the slow way: across generations, through
	// who happens to survive long enough to reproduce more.
	MaxLifespan float64 // lifespan budget an agent starts with

	// Undernourished: chronic hunger wears lifespan down, on top of the
	// vitality it already drains, using the same StarveHunger threshold.
	StarveLifespanRate float64 // lifespan lost per tick while Hunger > StarveHunger

	// Overfed: eating well past "80% full" wears lifespan down too, so that
	// gorging is not free. OverfedHunger sits below SatiatedHunger on purpose
	// (satiation stops costing vitality well before it starts costing
	// lifespan), which is what leaves a band where eating is simply free.
	OverfedHunger       float64 // below this hunger, an agent is eating past "80% full"
	OverfedLifespanRate float64 // lifespan lost per tick while Hunger < OverfedHunger

	// --- movement and effort ---
	//
	// Effort is the decision variable of every physical action. Speed grows
	// with the square root of effort while its cost grows linearly, so going
	// all out is fast but expensive per unit of distance.
	MaxSpeed         float64 // speed at effort 1
	MoveCost         float64 // vitality per tick at effort 1
	PerceptionRadius float64
	GrabRadius       float64 // how close an agent must be to eat
	CombatRadius     float64 // how close an agent must be to land a blow
	BoundaryMargin   float64

	// --- combat ---
	//
	// Damage is continuous: every tick both sides lose vitality in proportion
	// to the opponent's power and the effort poured in. Striking costs less
	// than being struck, which is what makes hitting somebody who is not
	// hitting back (an ambush, or chasing a fleeing agent) the most efficient
	// use of vitality there is, and a slugging match expensive for both.
	AttackDamage float64 // vitality the target loses per tick, at effort 1 from a mid-power agent
	AttackCost   float64 // vitality the attacker spends per tick, at effort 1
	FleeEffort   float64 // effort an agent puts into running away

	// SkirmishTicks is how long an agent expects a fight to last before one
	// side gives up. Fights are only settled by a death when neither side
	// breaks off, so pricing every fight as a fight to the death would make a
	// scuffle over a meal look suicidal and nobody would ever contest anything.
	SkirmishTicks float64

	// --- judgement, memory and estimation ---
	JudgementNoise float64 // spread of a fully irrational agent's misreading

	PriorStrength     float64 // what an agent assumes about a stranger
	PriorVariance     float64 // how unsure it is about that assumption
	CombatObsVariance float64 // noise of one observation made while fighting
	SpectateObsFactor float64 // watching others fight is worth less than fighting (>1)
	RiskDecayPerTick  float64 // fraction of the risk memory forgotten per tick

	// --- utility weights ---
	//
	// Every action is scored with the same formula:
	//   utility = LifeValue * gain in survival probability
	//           + OffspringValue * chance of offspring
	//           - vitality cost * VitalityCostWeight
	//           - ticks * TimeCost
	// Life outranks offspring; time is a cost, never a goal.
	LifeValue      float64
	OffspringValue float64
	TimeCost       float64
	VitalityWeight float64
	PlanHorizon    float64 // ticks an agent looks ahead when judging its odds

	// ShockRisk is how dangerous being low on vitality is in itself, on top of
	// starving: a depleted agent has nothing left to absorb the next fight.
	// Without it, spending vitality would look free to anybody who is not
	// hungry, and the world turns into a brawl.
	ShockRisk float64

	CompetitionWeight float64 // value of removing a future rival for food
	RiskWeight        float64 // how much past damage from somebody puts an agent off
	InfoValue         float64 // value of shrinking the uncertainty about somebody
	ExploreValue      float64 // value of wandering when nothing is in sight

	// --- decision triggers ---
	//
	// Agents do not re-decide every tick, only when something happens. The
	// vitality drop is a "think again" trigger, not a "run away" threshold:
	// there is no hardcoded flee condition anywhere.
	TriggerVitalityDrop float64
	TriggerIdleTicks    int

	// --- intelligence ---
	//
	// Two separate handles, matching the two halves of the ability: how many
	// kinds of move an agent can think of at all, and how well it tells the
	// ones it thought of apart. The second is deliberately not the same as
	// rationality: rationality is misreading the world, this is misjudging a
	// move once the world has been read.
	// StrategyDepthUnlock is the ability points needed to unlock each lookahead
	// level. Zero turns the gate off: everybody can think of everything, and
	// intelligence is left acting through ChoiceNoise alone.
	StrategyDepthUnlock float64
	ChoiceNoise         float64 // spread of the error a mindless agent makes scoring an option

	// --- reproduction ---
	//
	// The two vitality figures are shares of the agent's own capacity, not
	// absolute amounts. Once the vitality gene decides how much a body can
	// hold, an absolute threshold would be a cliff rather than a cost: an
	// agent whose capacity fell below it could never court at all, however
	// well fed, and the gene would stop being a trade and start being a gate.
	ReproHunger        float64 // an agent only courts below this hunger
	ReproVitalityShare float64 // ... and above this share of its own capacity
	PairBondDuration   int     // ticks a pair stays together before the child is born
	MatingCooldown     int     // ticks of rest after a bond ends
	BirthVitalityCost  float64 // vitality the parents share to produce a child
	ChildVitalityShare float64 // vitality a newborn starts with, as a share of its capacity
	ChildHunger        float64
	// The budget an agent is made of, and how a founder splits it.
	//
	// Everything heritable is paid for out of one total, so being better at
	// one thing costs being worse at another. Only founders draw a budget;
	// after that it is inherited.
	//
	// GeneInitAlpha is the Dirichlet concentration the founders' split is
	// drawn with: below 1 the split is lopsided, which is what puts genuinely
	// different kinds of individual into the first generation rather than nine
	// near-equal shares of the same shape.
	GeneBudgetMean float64
	GeneBudgetStd  float64
	GeneInitAlpha  float64

	// What a child's budget is made of. It comes from one parent or the
	// other, never the average of the two: averaging halves the variance of
	// the budget every generation, which is the thing blending inheritance was
	// dropped for.
	//
	// BudgetHeritability is how much of the parent's budget carries over,
	// against the population mean: 1 inherits it, 0 draws afresh at every
	// birth (which is what the world did before budgets were inherited), and
	// the values between are regression towards the mean.
	BudgetInheritSpread float64
	BudgetHeritability  float64

	// Mutation is rare and large rather than constant and small: a gene is
	// copied from the parent unchanged most of the time, and now and then it
	// jumps. MutationRate is the chance per gene, MutationStd the spread of
	// the jump when it happens. Setting MutationRate to 0 stops new variation
	// without stopping selection on the variation already there.
	//
	// The pair is calibrated to keep about as much variation standing in the
	// population as a nudge on every birth used to. Matching the nominal
	// variance (rate x std^2) was not enough: at 1% and std 40 the spread came
	// out about 15% short, because a large jump from anywhere near the middle
	// of the range is clipped at 1 or 100 and part of it is thrown away. 2%
	// restores it, and still copies 98 genes in 100 unchanged.
	MutationRate        float64
	MutationStd         float64
	PatienceBase        float64 // ticks of comparison before committing to a mate
	PatienceRationality float64 // extra patience per point of rationality
	CommitFitness       float64 // a candidate this good is worth committing to at once
	MateRejectDuration  int     // ticks a passed over candidate is left aside

	// LamarckRate is how much of what a parent learned in its lifetime is
	// passed to its child, on top of what it inherits. It is here and unused:
	// there is nothing learned to pass on until stage 12 gives agents their
	// own values for the assumptions in the utility formula. The slot is set
	// aside now so that the inheritance code has one place for it, rather than
	// being rearranged later to make room.
	LamarckRate float64
}

// DefaultConfig returns the parameters the simulation runs with. The numbers
// are a starting point tuned by watching cmd/devview, not a fixed part of the
// rules.
func DefaultConfig() Config {
	return Config{
		Width:  800,
		Height: 600,
		Seed:   1,

		InitialPopulation: 60,
		InitialFoodItems:  70,
		MaxPopulation:     240,
		MaxFoodItems:      110,
		// Food is deliberately the thing in short supply. One agent eats an
		// item roughly every FoodNutrition/HungerRate ticks, so this rate feeds
		// a population of about two hundred and no more: past that, agents are
		// competing, which is what the rest of the rules are about.
		FoodSpawnRate: 0.20,

		MaxVitality: 100,
		MaxHunger:   100,
		HungerRate:  0.05,

		FoodNutrition:  34,
		StarveHunger:   60,
		StarveRate:     0.16,
		SatiatedHunger: 40,
		RegenRate:      0.09,

		// Starting points, not yet tuned with cmd/experiment. A chronically
		// starving or chronically overfed agent exhausts MaxLifespan in about
		// 30000 ticks of that condition, an order of magnitude slower than
		// acute starvation (a few hundred ticks from full vitality) so that
		// the two death causes stay distinguishable.
		MaxLifespan:         6000,
		StarveLifespanRate:  0.2,
		OverfedHunger:       20,
		OverfedLifespanRate: 0.2,

		MaxSpeed:         1.7,
		MoveCost:         0.035,
		PerceptionRadius: 130,
		GrabRadius:       11,
		CombatRadius:     15,
		BoundaryMargin:   8,

		AttackDamage:  1.15,
		AttackCost:    0.30,
		FleeEffort:    0.95,
		SkirmishTicks: 30,

		JudgementNoise:    40,
		PriorStrength:     50,
		PriorVariance:     420,
		CombatObsVariance: 90,
		SpectateObsFactor: 3.5,
		RiskDecayPerTick:  0.0015,

		LifeValue:      100,
		OffspringValue: 42,
		TimeCost:       0.012,
		VitalityWeight: 0.55,
		PlanHorizon:    700,
		ShockRisk:      0.55,

		CompetitionWeight: 0.10,
		RiskWeight:        0.22,
		InfoValue:         1.2,
		ExploreValue:      6,

		TriggerVitalityDrop: 4,
		TriggerIdleTicks:    40,

		// The gate is off by default. Measured over 24 seeds it was the half of
		// intelligence that did not work: spacing it at 16 left only one of its
		// three thresholds inside the range abilities actually occupy, and
		// tightening it to 20 turned the selection on intelligence negative.
		// Intelligence now acts through ChoiceNoise alone, which trebles the
		// pressure on it. See cmd/experiment and the design log for the numbers.
		StrategyDepthUnlock: 0,
		ChoiceNoise:         20,

		ReproHunger:         35,
		ReproVitalityShare:  0.70,
		PairBondDuration:    150,
		MatingCooldown:      140,
		BirthVitalityCost:   40,
		ChildVitalityShare:  0.58,
		ChildHunger:         18,
		GeneBudgetMean:      360, // nine genes, so a mean of 40 each
		GeneBudgetStd:       30,
		GeneInitAlpha:       0.8,
		BudgetInheritSpread: 30,
		BudgetHeritability:  1,
		MutationRate:        0.02,
		MutationStd:         40,
		PatienceBase:        25,
		PatienceRationality: 0.5,
		CommitFitness:       78,
		MateRejectDuration:  40,
	}
}
