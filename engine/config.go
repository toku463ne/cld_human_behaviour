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
	MaxFoodItems      int // plants only; carcasses are capped separately
	MaxMeatItems      int
	FoodSpawnRate     float64 // expected number of food items spawned per tick

	// --- state model: vitality, hunger and food ---
	//
	// The three are coupled in one direction only: eating lowers hunger, low
	// hunger lets vitality recover, moving and fighting spend vitality, and
	// being left alone raises hunger which eventually drains vitality. That
	// guarantees every agent has a way back up instead of only decaying.
	MaxVitality float64
	MaxHunger   float64
	HungerRate  float64 // hunger gained per tick by an agent of the average budget

	// What being made of more costs to run. Hunger climbs faster in
	// proportion to how far the agent's budget is above the average one, so
	// twice the budget at BudgetUpkeep 1 means hunger arriving twice as fast.
	//
	// Without a price of some kind the budget ratchets upwards: every gene is
	// worth more than it costs, so the individuals who inherit a larger total
	// out-breed the rest and pass the larger total on. Measured over 50000
	// ticks with no upkeep, the mean budget went from 360 to 588 and was still
	// climbing, which is the budget quietly ceasing to be a constraint.
	BudgetUpkeep float64

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

	// --- what is left when somebody dies ---
	//
	// A carcass is food. How much of it there is scales with how much the dead
	// creature was made of, so bringing down something large is worth more
	// than bringing down something small - which is the only reason a group
	// would ever be better than an individual at it.
	MeatPerBudget   float64 // budget per item of meat a carcass leaves
	MeatClaimTicks  int     // how long the carcass belongs to those who killed it
	HuntCreditTicks int     // how recently a blow must have landed to count as taking part

	// MeatSpoilTicks is how long a carcass lasts before it is gone. Without
	// it, meat nobody can eat piles up until it fills the world's allowance
	// for food and crowds the plants out - which is what happened the first
	// time carcasses went in, and cost a fifth of the population.
	MeatSpoilTicks int

	// Carcasses are counted against MaxMeatItems rather than against the
	// world's allowance for plants. Sharing one allowance meant a spell of
	// heavy dying filled it with meat, no plant could grow, and the species
	// that lives on plants starved - a predator killing its prey by taking up
	// the room its food grows in.

	// PreyValue is how much of a meal a carcass is worth to the one that
	// brings it down, as a multiplier on the ordinary value of a meal. Zero
	// takes the reason to hunt out of the utility formula while leaving
	// carcasses, claims and everything else in place, which is the control
	// the pack hunting question needs: whatever happens at zero is what
	// agents do to each other anyway.
	PreyValue float64

	// --- combat ---
	//
	// Damage is continuous: every tick both sides lose vitality in proportion
	// to the opponent's power and the effort poured in. Striking costs less
	// than being struck, which is what makes hitting somebody who is not
	// hitting back (an ambush, or chasing a fleeing agent) the most efficient
	// use of vitality there is, and a slugging match expensive for both.
	AttackDamage float64 // vitality the target loses per tick, at effort 1 from a mid-power agent
	AttackCost   float64 // vitality the attacker spends per tick, at effort 1

	// What guarding and dodging can do at most, for an agent that spent its
	// whole range on the gene and is using the channel fully, and what each
	// costs to use for a tick.
	//
	// These decide the asymmetry the whole world balances on: the attacker's
	// advantage is what makes hitting somebody worth doing, and a defence that
	// is too good removes it entirely. Change them and look at the population.
	DefenceCap  float64 // fraction of an incoming blow turned aside
	DefenceCost float64
	EvasionCap  float64 // chance of a blow missing altogether
	EvasionCost float64
	FleeEffort  float64 // effort an agent puts into running away

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

	// How much of other people an agent can hold on to.
	//
	// MemoryCapacity is the number of others an agent of average memory can
	// keep a record of; what an individual gets is that scaled by its memory
	// gene. Once it is full, taking somebody new on means giving somebody up,
	// which is what turns forgetting from a timer into a competition - and
	// what puts a ceiling on how many others one agent can be attached to.
	//
	// MemoryBandwidthShare is how many of those records it can take in during
	// a single tick, as a share of the capacity. It is derived from the
	// capacity rather than being a gene of its own: holding and taking in are
	// the same organ.
	//
	// Zero in either turns that limit off, which is the world as it was before
	// stage 9.
	MemoryCapacity       int
	MemoryBandwidthShare float64

	// ContactRefresh keeps what is already known about somebody from going
	// stale while they are still around: seeing them resets where the fading
	// is measured from, without changing what is remembered. The curve itself
	// is untouched. False is the world before stage 9.
	ContactRefresh bool

	// --- affinity ---
	//
	// The first positive thing an agent remembers about anybody. It only ever
	// comes from something that actually happened between them, never from
	// having stood next to each other, and it fades like the risk memory does.
	//
	// There is no hostility to match it: the risk memory already is one, and a
	// second negative record would be the same thing under another name.
	//
	// In this stage affinity buys exactly one thing - it is who an agent is
	// willing to be defenceless around (RestExposureWeight). Making it a
	// positive term in the utility formula waits for stage 12, so that giving
	// and receiving arrive together.
	AffinityPairBond     float64 // gained by both when a pair forms
	AffinityBirth        float64 // ... and again by both when the child arrives
	AffinityKin          float64 // a parent or a child starts this far in
	AffinityDecayPerTick float64
	AffinityTrust        float64 // affinity at which somebody is trusted completely

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

	// RestExposureWeight is how much of the neighbourhood's strength an agent
	// reckons is going to land on it while it lies there recovering. Resting
	// used to cost nothing wherever it was done; this is the price of doing it
	// in the open, discounted by whoever nearby it trusts.
	//
	// It is an estimate made with what the agent already believes - the
	// strength it credits each neighbour with, how close they are, and how
	// fond of them it is - and not a new sense. Zero restores the old
	// behaviour exactly, which is the arm to compare against: too high a value
	// and nobody dares recover, which breaks the one rule the world cannot do
	// without.
	RestExposureWeight float64

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

	// --- growing up, and wearing out ---
	//
	// One curve scales everything an agent inherited (Agent.AgeFactor): it
	// rises while the agent is still growing into itself and falls once it is
	// past its prime. The two ends are the same function, because they are the
	// same thing - how much of its inheritance this body is currently able to
	// express.
	//
	// Growth is bought with food rather than with time (see World.metabolise),
	// so a hungry childhood is a long one. Ageing is bought with nothing: it
	// is the passage of years.
	//
	// ChildAbilityShare = 1 and SenescenceRate = 0 give the world as it was
	// before stage 7d, which is the arm to compare against.
	ChildhoodYears    float64 // years of eating well it takes to finish growing
	ChildAbilityShare float64 // share of its inheritance a newborn can express
	ReproMaturity     float64 // maturity below which offspring is not considered
	SenescenceYears   float64 // age past which the curve starts coming down
	SenescenceRate    float64 // share of expression lost per year past that
	SenescenceFloor   float64 // ... but never below this

	// Being worn down for a long time costs lifespan, on top of what it
	// already costs in vitality. It is deliberately not the same thing as
	// ShockRisk: that one is a danger the agent feels and acts on, this one is
	// a silent tally it has no way of knowing about.
	FrailVitalityShare float64 // below this share of its capacity an agent is failing
	FrailGraceTicks    int     // how long it may be there before it starts to count
	FrailLifespanRate  float64 // lifespan spent per tick after that

	// How long a newborn keeps to the parent that had it, and how far it may
	// stray before it turns back. This is protection and nothing else: there
	// is no feeding, and the parent is not asked to do anything. What the
	// child gets is the company of somebody big who drives rivals off the
	// ground they are both standing on.
	//
	// Zero turns it off entirely, which is how much of any grouping is down to
	// families rather than to anything else.
	ChildRearingTicks int
	RearingRadius     float64

	// --- reproduction ---
	//
	// The two vitality figures are shares of the agent's own capacity, not
	// absolute amounts. Once the vitality gene decides how much a body can
	// hold, an absolute threshold would be a cliff rather than a cost: an
	// agent whose capacity fell below it could never court at all, however
	// well fed, and the gene would stop being a trade and start being a gate.
	// FitnessConditionWeight is how much of how good a mate somebody looks is
	// the condition they are in rather than the advertisement itself; the rest
	// is the attractiveness gene.
	//
	// Condition is in there at all because a dying agent is a poor bet however
	// fine it looks. It is a field rather than a constant because it is the
	// one dial on the direction of sexual selection, and the two ends of it
	// are different worlds: at 0 the looks gene is the only thing anybody
	// judges a mate on, at 1 the gene is decoration nobody reads.
	FitnessConditionWeight float64

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

	// What an enemy is made of. It is the same kind of creature run by the
	// same rules; the difference between the species is the range its budget
	// is drawn from, which is what PLAN.md means by expressing a species as a
	// range of parameters rather than as a second sort of thing.
	//
	// A larger budget makes an enemy harder to bring down and worth more when
	// it is: the carcass scales with the budget. That is the whole of what
	// makes hunting one worth doing together.
	InitialEnemies  int
	EnemyBudgetMean float64
	EnemyBudgetStd  float64

	// Enemies also arrive from outside the map, at one per EnemySpawnTicks
	// while there are fewer than MaxEnemies of them.
	//
	// They are the same creature as any other and they do breed, but a
	// predator population left entirely to itself in a world this small
	// overshoots its prey and starves: measured, forty enemies took the
	// humans from 60 to 17 and were themselves gone by tick 4500. The arrival
	// rate is the same device the food spawn already is - the world's edge
	// standing in for the rest of the world - and it is what keeps the arena
	// there to be measured. Setting it to zero leaves the predators entirely
	// on their own.
	EnemySpawnTicks int
	MaxEnemies      int

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

	// Rare births that get far more to be made of than their parents did.
	//
	// The rates are per birth rather than per tick, so "once in ten years" is
	// only true at the birth rate the world happens to be running at: at the
	// measured rate of roughly 60 births per 5000 ticks, 0.017 is one genius
	// in ten years and 0.0017 one great genius in a century.
	//
	// The budget of an exceptional birth replaces the inherited one rather
	// than multiplying it, so a genius is a step to a level rather than a
	// windfall proportional to its parents. Being inherited from there, the
	// windfall does not vanish with the individual: its children start from
	// the budget it had, less whatever the ordinary wobble takes.
	GeniusRate        float64
	GeniusBudget      float64
	GreatGeniusRate   float64
	GreatGeniusBudget float64

	// TicksPerYear is the unit the rates above are quoted in. Nothing in the
	// rules reads it; it is what turns a measured interval into "once in so
	// many years", and stage 7d fixes its value against the length of a life.
	TicksPerYear int

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
		MaxMeatItems:      140,
		// Food is deliberately the thing in short supply. One agent eats an
		// item roughly every FoodNutrition/HungerRate ticks, so this rate feeds
		// a population of about two hundred and no more: past that, agents are
		// competing, which is what the rest of the rules are about.
		FoodSpawnRate: 0.20,

		MaxVitality:  100,
		MaxHunger:    100,
		HungerRate:   0.05,
		BudgetUpkeep: 1,

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
		// Three thousand, calibrated once growing up and wearing out were both
		// in: it puts wear at about 4% of deaths, which is visible without
		// crowding out starving and fighting. At the old 6000 nobody ever
		// wore out at all, and at 1500 the world collapsed.
		MaxLifespan:         3000,
		StarveLifespanRate:  0.2,
		OverfedHunger:       20,
		OverfedLifespanRate: 0.2,

		MaxSpeed:         1.7,
		MoveCost:         0.035,
		PerceptionRadius: 130,
		GrabRadius:       11,
		CombatRadius:     15,
		BoundaryMargin:   8,

		PreyValue:       1,
		MeatPerBudget:   120, // an ordinary agent leaves 4 items, a large enemy many more
		MeatClaimTicks:  400,
		MeatSpoilTicks:  900,
		HuntCreditTicks: 200,

		AttackDamage:  1.15,
		AttackCost:    0.30,
		DefenceCap:    0.55,
		DefenceCost:   0.12,
		EvasionCap:    0.45,
		EvasionCost:   0.20,
		FleeEffort:    0.95,
		SkirmishTicks: 30,

		JudgementNoise:    40,
		PriorStrength:     50,
		PriorVariance:     420,
		CombatObsVariance: 90,
		SpectateObsFactor: 3.5,
		RiskDecayPerTick:  0.0015,

		// Twelve faces for an average memory, half of them taken in per tick.
		// Twelve is a little under the number of others a crowded agent has in
		// sight at once, so the limit binds where it matters - in a crowd -
		// and never in an empty stretch of world.
		MemoryCapacity:       12,
		MemoryBandwidthShare: 0.5,
		ContactRefresh:       true,

		// A bond is worth about as much as a bad fight costs, and the fading
		// is slower than the risk memory's: what somebody did for you outlasts
		// what they did to you, which is the only reason a group could hold
		// together for longer than a grudge.
		AffinityPairBond:     18,
		AffinityBirth:        18,
		AffinityKin:          22,
		AffinityDecayPerTick: 0.0008,
		AffinityTrust:        20,

		LifeValue:      100,
		OffspringValue: 42,
		TimeCost:       0.012,
		VitalityWeight: 0.55,
		PlanHorizon:    700,
		ShockRisk:      0.55,

		// Small on purpose. It only has to be enough to tell two places to lie
		// down apart, and the price of more than that is the population: at
		// 0.10 the world holds two thirds of the agents it did, and at 0.20
		// half, because agents that will not settle near each other do not
		// meet, pair or breed. At 0.05 the strangers standing over a resting
		// agent fall from 6.4 to 2.0 and the population does not move.
		RestExposureWeight: 0.05,

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

		// Two years of eating well to grow up, against a life whose median
		// length is about ten. A newborn expresses six tenths of what it
		// inherited, so it is visibly a smaller thing that loses fights, and
		// it may look for a mate at nine tenths grown.
		//
		// All four of these started harsher - a third at birth, three years,
		// nothing until fully grown - and that world lost four fifths of its
		// population: childhood is paid for in generation time, and a species
		// whose median life is ten years cannot spend three of them growing.
		// The numbers here are where the world still has a real childhood and
		// still works. See HISTORY.md, 2026-09-02.
		ChildhoodYears:    2,
		ChildAbilityShare: 0.6,
		ReproMaturity:     0.9,
		SenescenceYears:   12,
		SenescenceRate:    0.06,
		SenescenceFloor:   0.40,

		FrailVitalityShare: 0.20,
		FrailGraceTicks:    200,
		FrailLifespanRate:  0.2,

		ChildRearingTicks: 1000, // the two years of childhood

		// As far as the child can see, which is the point: keeping to a
		// parent has to mean staying in the same neighbourhood, not staying
		// within arm's reach. A short leash measurably kills children - they
		// get dragged off the food they were going for (grewUp 0.46 against
		// 0.56 with no leash at all).
		RearingRadius: 130,

		FitnessConditionWeight: 0.35,

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
		InitialEnemies:      10,
		EnemyBudgetMean:     520, // over the human 360, so a carcass feeds several
		EnemyBudgetStd:      90,
		EnemySpawnTicks:     400,
		MaxEnemies:          12,
		BudgetInheritSpread: 30,
		BudgetHeritability:  1,
		GeniusRate:          0.017,
		GeniusBudget:        495, // 55 a gene against the usual 40
		GreatGeniusRate:     0.0017,
		GreatGeniusBudget:   630, // 70 a gene, short of the 900 that would max everything
		TicksPerYear:        500,
		MutationRate:        0.02,
		MutationStd:         40,
		PatienceBase:        25,
		PatienceRationality: 0.5,
		CommitFitness:       78,
		MateRejectDuration:  40,
	}
}
