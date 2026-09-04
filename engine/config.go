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

	// --- what an agent can see (stage 13, sight.go) ---
	//
	// SightGrid swaps the circle above for a block of cells: the cell the agent
	// is in, plus SightCells rings around it. False restores the circle, which
	// is the arm this is measured against.
	//
	// A block is not a rounder or coarser circle. It sees different distances
	// in different directions, and how far it sees depends on where in its cell
	// the agent happens to be standing - which is what makes running away and
	// racing for food different, and is the whole point of the change.
	//
	// SightCellSize is calibrated so that a block covers about the same ground
	// as the circle did: (2N+1)^2 * size^2 = pi * radius^2. Without that the
	// change would be "agents can see more" or "agents can see less", and the
	// shape - the thing being tested - would not be separable from it.
	SightGrid     bool
	SightCellSize float64
	SightCells    int

	// --- the world's own regions (stage 14, region.go) ---
	//
	// One coarse division of the world that everything varying by place uses:
	// how sheltered the resting is (this stage), how well the plants grow
	// (stage 15), and whatever a region-by-region reset needs later. Not a
	// wall, not a thing agents can see or name - just a number that differs
	// from place to place.
	//
	// ShelterSpread is how much the regions differ in how exposed resting in
	// them is: each draws a multiplier on RestExposureWeight from
	// 1 +/- spread. Zero makes every region ordinary AND takes nothing from
	// the random source, so the run is identical to one from before regions
	// existed - which is the arm this is measured against.
	RegionCols    int
	RegionRows    int
	ShelterSpread float64

	// FoodSpread is how much the regions differ in how well they grow plants
	// (stage 15a). It changes where the food comes up and never how much: the
	// world grows exactly as much as it did. That is deliberate, because
	// FoodSpawnRate is the most selection-sensitive figure in the world and a
	// rule that quietly changed the total would be measuring something else.
	//
	// Both spreads are skipped entirely at zero, taking nothing from the
	// random source, so either can be turned off and give the run the world
	// had before that rule existed.
	FoodSpread float64

	// --- what an agent makes of the ground (stages 15b, 15c) ---
	//
	// RegionLearnRate is how far one look moves an agent's estimate of the
	// ground it is standing on. Zero stops agents knowing anything about
	// where they are, which leaves stage 15a's sorting and nothing else - the
	// arm this is measured against.
	//
	// An agent can only form a view of ground it has been on. Hearing about
	// somewhere it has never been is what the trade is for (stage 15c), and
	// it is the only way to be drawn to a place it has not found by accident.
	RegionLearnRate float64

	// RegionMemory caps how many looks an estimate is worth, and
	// RegionForgetPerTick how fast it fades once the agent has left. Both are
	// scaled by the memory gene: what that gene buys here is how well the
	// country is held, not how many people are (#41).
	RegionMemory        float64
	RegionForgetPerTick float64

	// RegionNoise is how badly the ground is read, in the same units as every
	// other misreading: worse for an agent with less rationality.
	RegionNoise float64

	// RegionToldCount is what a handed-down view of somewhere is worth in
	// looks. Low on purpose: being told about a place is a starting point,
	// and the first look the agent takes for itself should overturn it.
	RegionToldCount float64

	// RegionPrior is roughly how much food is in sight on ordinary ground. It
	// is not a rule - nothing reads it to decide anything - only the scale
	// that puts a trade in the ground into the same units as a trade in
	// anything else.
	RegionPrior float64

	// RegionDrawValue is what heading for better ground is worth, per unit of
	// how much better it is believed to be. Zero leaves agents knowing about
	// the country and never acting on it, which separates knowing from
	// choosing.
	RegionDrawValue float64

	// --- living on one thing (stage 16, diet.go) ---
	//
	// SamenessPenalty is the most a mouthful can be discounted for being the
	// same as everything else the agent has been eating. Zero turns the rule
	// off, which is the arm it is measured against.
	//
	// It can never take the whole value: being sick of something is a reason
	// to look for something else, not a reason to starve beside food, and an
	// agent with only one thing available must still eat it.
	SamenessPenalty float64

	// DietSatiety is how many of one kind it takes to reach half the penalty,
	// and DietForgetPerTick how fast having eaten it fades once the agent
	// stops.
	DietSatiety       float64
	DietForgetPerTick float64

	// --- plants that inherit something (stage 17a, plant.go) ---
	//
	// PlantGenetics turns the plants' own inheritance on. False leaves them
	// where stage 15a had them - appearing wherever the ground is good, with
	// no parent and nothing passed on - and false is the default.
	//
	// It is off because it was measured. The mechanism works - the plants
	// evolve, readily and in a direction - and it takes two thirds of the
	// population with it, by two separate routes that no setting escapes
	// together. Seeds thrown a short way make thickets and leave half the
	// world's regions with nothing growing in them, which is an absorbing
	// state because nothing can seed where nothing grows. Seeds thrown far
	// keep every region stocked and destroy what stage 15 built the
	// population on: food that stays put long enough to be worth learning
	// about. See HISTORY.md.
	//
	// A plant has two genes and no budget and no decisions (#42). How many
	// plants there are is still FoodSpawnRate's business and nothing here
	// changes it; what the genes decide is whose children they are and where
	// they land.
	PlantGenetics bool

	// PlantSpread is how far a seed lands from its parent to begin with, and
	// PlantSpreadMax the furthest evolution may take that. It is also the
	// distance the plants' clumping is measured at.
	PlantSpread    float64
	PlantSpreadMax float64

	// Rare and large, as the agents' mutation is and for the same reason: a
	// nudge on every seed would leave nothing of the parent in the child.
	PlantMutationRate float64
	PlantMutationStd  float64

	// SeedSurvival is how often a seed lives through being eaten, and
	// SeedGutTicks how long it is carried before it comes up (stage 17c,
	// #44). Zero for the first turns carrying off, and zero is the default.
	//
	// It rides on the eating that was already happening: no new action, no
	// carrying behaviour, and the count of plants is untouched - a carried
	// seed takes the place of the world's next planting rather than adding to
	// it.
	//
	// It was built to be the answer to what wind dispersal does to the map,
	// animals being the only thing here that travels from where the food is to
	// where it is not. It is not: with it on, the share of regions with
	// nothing growing in them is 0.55 against 0.53 without, which is no
	// difference at all. And on its own, with the plants inheriting nothing,
	// it costs a third of the population - a carried seed comes up where an
	// animal has just been, which is ground that has just been grazed, so it
	// plants food where the food has already gone rather than where the
	// ground is good.
	SeedSurvival float64
	SeedGutTicks int

	// --- poison and warning (stage 17b, #43) ---
	//
	// PlantDefence gives plants two more genes: what eating one costs, and how
	// loudly it says so. They are drawn and inherited independently and
	// nothing ties them together - whether a warning is honest is left to the
	// world to arrive at, or not.
	//
	// It is separate from PlantGenetics on purpose. What took the population
	// apart at 17a was where plants come up; what they pass on is a different
	// question, so the defences can be inherited on a map that still puts
	// plants where the ground is good. Their parent is drawn uniformly from
	// the standing crop, so the only way to be picked more often is to still
	// be standing - which is to say, not to have been eaten.
	//
	// False by default, because it costs nine tenths of the population at
	// every dose tried, down to a ninth of the default one. Agents refuse food
	// over warnings and starve, and the warnings are worth refusing over:
	// poison and signal both sit near where they were drawn (0.52 and 0.49)
	// with a correlation of about nothing, so a warning predicts nothing and
	// the crop is neither honest nor consistently deceitful. It is noise that
	// the eaters treat as information.
	//
	// It does raise the selection pressure on rationality, which nothing has
	// managed since stage 9 (shRationality 0.07 -> 0.11 ***). Do not read that
	// as reading warnings finally paying: with the signals made unreadable it
	// rises exactly as much. What is selecting for rationality is the harsher
	// world, not the warnings in it - the same confound stage 12a found when
	// telling agents the truth about retaliation.
	PlantDefence bool

	// PoisonDamage is the vitality a fully poisonous plant takes off whoever
	// eats it, and SignalNoise how badly the warning is read - scaled, like
	// every other misreading, by what the reader spent on rationality. This is
	// the first job rationality has ever had on the food's side of the world.
	PoisonDamage float64
	SignalNoise  float64

	// --- the world's day, and who keeps which hours (stage 18, clock.go) ---
	//
	// TicksPerDay gives the world a cycle. Zero is a world with no clock,
	// which is the one from before this stage and draws nothing extra from
	// the random source.
	//
	// What varies with the hour is how well an agent rests, and nothing else.
	// Resting is already priced per agent and per moment, so the hour needs no
	// new formula; making sight or the food spawn vary would be a change to
	// the world for everybody at once, which is a different question from the
	// one being asked here.
	TicksPerDay int

	// RestPhaseDepth is how much better an agent recovers at its own hour than
	// at its opposite one: recovery is the world's rate times 1 +/- this. It
	// never reaches zero, because a way back from exhaustion is the one rule
	// the world cannot do without.
	RestPhaseDepth float64

	// ChronotypeSpread is how much of the day the population's clocks are
	// scattered over, and ChronotypeMutation how far a child's drifts from its
	// parent's. Spread zero puts everybody on the same clock, which is the arm
	// the stage is measured against: a world with a day in it but no
	// disagreement about when to sleep.
	ChronotypeSpread   float64
	ChronotypeMutation float64

	// SleepingWatch makes a trusted neighbour discount the danger of lying
	// down whether or not it is awake, which is how it worked before this
	// stage. It is the control that says whether keeping watch is worth
	// anything: with it true, differing hours are decoration.
	SleepingWatch  bool
	GrabRadius     float64 // how close an agent must be to eat
	CombatRadius   float64 // how close an agent must be to land a blow
	BoundaryMargin float64

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

	PriorStrength     float64 // what an agent assumes about a stranger it cannot size up
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

	// --- learning from looks ---
	//
	// What can be seen of a body (Agent.Appearance: how big it is and how fast
	// it moves) and what each agent has learned that it means. LearnFromLooks
	// false is the world before stage 10, where every stranger was assumed to
	// be PriorStrength however they looked.
	//
	// LooksSlope false leaves the learning on but flattens the line, so an
	// agent learns what the strengths around it average to and nothing more.
	// It is the control that separates the two halves of the gain: knowing
	// what the world is like, and being able to read one body in it.
	//
	// AppearanceNoise is the spread of a fully irrational agent's misreading
	// of a build, as JudgementNoise is for strength. It is much the smaller of
	// the two on purpose: how big something is is easier to see than how hard
	// it hits, and if it were not, appearance would carry nothing.
	//
	// AppearanceMinReads is how many readings an agent needs before it goes by
	// its own line rather than the flat prior. A line through two points is
	// not knowledge.
	// AppearanceSlopePrior is how many readings' worth of "a build says
	// nothing" an agent starts with. The fitted slope is pulled back towards
	// zero by it, so a line only becomes steep once there is enough evidence
	// to hold it up. Zero is the raw fit.
	LearnFromLooks       bool
	LooksSlope           bool
	AppearanceNoise      float64
	AppearanceMinReads   int
	AppearanceSlopePrior float64

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

	// AffinityHunt is what each pair that brought a carcass down together
	// gains of each other. It is the one source that has to be earned with
	// somebody who is not family: a bond and a birth need a partner, and kin
	// is given at birth, so without this the only strangers an agent can come
	// to trust are the ones it mates with.
	//
	// Who counts as having taken part is not a new rule: it is the same list
	// the carcass is shared out to (Agent.recentAttackers, filtered to those
	// who can eat it), so an agent cannot earn a friend by hitting something
	// it has no use for, and hanging back until the kill is made earns
	// nothing either.
	AffinityHunt float64

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

	// --- what an agent assumes (lore.go) ---
	//
	// Two of the figures the utility formula leans on are claims about the
	// world rather than about the agent: how often somebody who is hit hits
	// back, and how often a proposal is accepted. They used to be constants in
	// the controller, the same for everybody and right by construction. They
	// are now what each agent has made of what it has seen, and these two are
	// the world's own figures - the starting assumption a founder is given and
	// a child falls back on, not a fact the engine consults.
	//
	// ShockRisk, CompetitionWeight and RiskWeight above become centres in the
	// same way: an agent's own value is drawn around them and inherited from
	// there.
	Retaliation  float64 // assumed chance the one you hit hits back
	AcceptChance float64 // assumed chance a courtship is accepted

	// LearningRate is how far one observation moves a belief, as a share of
	// the move a plain running mean would make. Zero stops every agent
	// learning anything, and zero is the default.
	//
	// It is off because it was measured, not because it does not work: it
	// works, and that is the problem. The world's true retaliation rate is
	// about 0.15, against the 0.7 the controller used to assume, and agents
	// that find that out halve the population - not by killing each other,
	// but by brawling, scattering and then starving. The 0.7 was not a fact
	// about the world, it was a deterrent holding it together. Turning this
	// on is a decision for when the world has a real one; see HISTORY.md.
	LearningRate float64

	// LoreMemory caps the evidence behind a belief. Without a cap an old agent
	// could not notice that the world had changed, for the same reason its
	// memory of individuals fades.
	LoreMemory float64

	// LorePriorCount is how much evidence the starting assumption is worth. It
	// is what stops the first surprise from throwing a belief across the
	// range; a handful of observations still move it.
	LorePriorCount float64

	// LoreInitSpread is the spread of a founder's preferences around the
	// world's figure, and LoreMutationStd the spread of the jog a child's gets
	// on inheritance. Both are proportional, so one number does for values of
	// different sizes. Zero for both gives a population that wants exactly the
	// same things and has nothing for selection to work on - the arm that says
	// how much of what follows is selection on preference.
	LoreInitSpread  float64
	LoreMutationStd float64

	// --- trading it (stage 12b) ---
	//
	// LoreExchangeRate is how far each side moves towards the other when two
	// agents have stood together long enough to trade what they assume. It
	// rides on ActObserve and has no cost of its own. Both move by the same
	// fraction of the gap at the same moment, so a half meets in the middle
	// and nobody can take without giving. Zero turns handing anything on off
	// entirely, which is the arm that says whether what spreads through the
	// population spread by being taught or by being survived.
	LoreExchangeRate float64

	// AffinityLore is what a trade is worth to the two in it, per unit of what
	// actually changed hands (measured as a share of the world's own figure,
	// so that the five values, which are on quite different scales, can be
	// added up). Two agents who already agree trade nothing and think no more
	// of each other for it - there is no test for that, it is what the
	// arithmetic does.
	AffinityLore float64

	// LoreValue is what watching somebody you are fond of is worth on top of
	// sizing them up: the second path by which another agent being alive and
	// nearby is worth something (the first is a carcass too big to bring down
	// alone). It is the term that has to explain why a group lasts, as against
	// why one forms.
	//
	// An agent cannot see what anybody else assumes, so what it goes on is
	// affinity - who it has got something out of before. Zero removes the term
	// and leaves the trade happening without anybody seeking it out.
	LoreValue float64

	// --- rules of thumb (stage 12c, hint.go) ---
	//
	// A hint is a situation, a move, and a weight: the relations the designer
	// did not write down, left to selection to find. They only ever add to an
	// option's score, and nothing anywhere branches on one.
	//
	// HintSlots is the most room any agent may have. Zero removes them
	// entirely, which is the arm the stage is measured against - and it has to
	// be run as a pair with the rest, because room costs budget and a world
	// with hints in it is made of slightly smaller agents.
	HintSlots int

	// HintSlotCost is what one slot takes out of the budget the genes are
	// then fitted to. It is charged for room, not for ideas: an empty slot
	// costs the same, which is what makes carrying a lot of them a real
	// trade against being big, fast or dangerous.
	HintSlotCost float64

	// The spread a weight is drawn and mutated with, and the range it is kept
	// inside. Both are in the units an option is scored in.
	HintWeightStd float64
	HintWeightMax float64

	// HintTradeWorth is what handing somebody an idea they did not have
	// counts for, in the same units as the rest of a trade (a share of one of
	// the five figures). Passing on a trick is worth more than shading a
	// number towards each other, which is why it is not simply 1.
	HintTradeWorth float64

	// HintsSpread is whether an idea can be copied from one agent to another
	// at all. True by default; false leaves hints existing, costing the same
	// and being inherited the same, but only ever passed down a bloodline.
	// It is the arm that says whether a good trick actually spreads by being
	// copied, which is the one claim stage 12c makes that inheritance alone
	// could not.
	HintsSpread bool

	// RaceOnDistance judges a race for food by who is nearer rather than by
	// who would arrive first: the rule as it stood before the groundwork for
	// terrain. It ignores how fast either body is and how hard the agent is
	// about to try, which is what left the speed gene with nothing to buy.
	// True restores it, as the arm to compare against.
	RaceOnDistance bool

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
	// passed to its child, on top of what it inherits: it applies to the
	// beliefs about the world in lore.go, which are the only things an agent
	// finds out rather than is born with.
	//
	// Zero by default, and that is the point of it. At zero every child has to
	// discover the world for itself, so whatever the population comes to
	// believe it believes because the ones who believed it lived. Turn it up
	// and the same belief can spread by being handed down. Running both is the
	// only way to tell those two apart.
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

		// One ring of cells, as asked for. The size follows from matching the
		// circle's area: 9 * size^2 = pi * 130^2 gives 76.8.
		SightGrid:     true,
		SightCellSize: 76.8,
		SightCells:    1,

		// Twelve blocks of 200x200 in an 800x600 world: about seven sight
		// cells each, which is coarse enough that walking across one takes a
		// while. Finer regions would average out under any amount of
		// wandering and there would be no such thing as a good place to be.
		RegionCols:    4,
		RegionRows:    3,
		ShelterSpread: 0.6,
		FoodSpread:    0.6,

		// A look moves the estimate the whole way a running mean would, and
		// forty looks is what an ordinary memory holds - a few visits' worth,
		// not a lifetime. The fade is slow enough that country learned young
		// is still worth something later, and fast enough that a world which
		// changed would be noticed.
		RegionLearnRate:     1.0,
		RegionMemory:        40,
		RegionForgetPerTick: 0.0004,
		RegionNoise:         2.0,
		RegionToldCount:     2,
		RegionPrior:         3,
		RegionDrawValue:     4,

		// Small on purpose. This rule has almost nothing to bite on yet - a
		// human's food is plants, near enough always - so it is set where it
		// would matter if there were a choice, and measured now to have a
		// baseline for when stage 17 provides one.
		SamenessPenalty:   0.35,
		DietSatiety:       4,
		DietForgetPerTick: 0.002,

		// Both off, and measured rather than guessed. See the fields for what
		// happens when they are on; the short of it is that this world's
		// population had come to depend on the food geography being stable
		// and learnable, and plants that decide their own whereabouts take
		// that away. The figures below are what they run at when turned on.
		PlantGenetics:     false,
		PlantSpread:       90,
		PlantSpreadMax:    500,
		PlantMutationRate: 0.05,
		PlantMutationStd:  0.4,
		SeedSurvival:      0,
		SeedGutTicks:      120,

		// A full dose costs about a fifth of an ordinary body's vitality:
		// enough to be worth reading the warning for, not enough to make one
		// mouthful fatal. The signal is read about as badly as a stranger's
		// build is (stage 10), which is to say clearly but not exactly.
		// A day of a thousand ticks: a life of about ten years at five hundred
		// ticks a year runs to five thousand days, which is a creature that
		// has seen a great many of them rather than a handful.
		TicksPerDay:        1000,
		RestPhaseDepth:     0.5,
		ChronotypeSpread:   1,
		ChronotypeMutation: 0.08,

		PlantDefence:   false,
		PoisonDamage:   18,
		SignalNoise:    0.25,
		GrabRadius:     11,
		CombatRadius:   15,
		BoundaryMargin: 8,

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
		// Size is read far more sharply than strength (6 against 40), and
		// eight readings is where an agent starts trusting its own line. The
		// sharpness had to be measured rather than guessed: at 12 the signal
		// in a build is swamped entirely and reading it is worth nothing
		// (HISTORY.md 2026-09-03).
		LearnFromLooks:       true,
		LooksSlope:           true,
		AppearanceNoise:      6,
		AppearanceMinReads:   8,
		AppearanceSlopePrior: 60,

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
		AffinityHunt:         6,
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

		// The two figures the controller used to hardcode, unchanged in value:
		// this is the same world it was, with the numbers now somewhere an
		// agent can disagree with them.
		Retaliation:  0.7,
		AcceptChance: 0.6,

		// Off, for the reason written against the field. The two figures
		// below are what it runs at when it is turned on: a belief moves the
		// whole way a running mean would, and it takes about sixty
		// observations to be worth as much as the founding assumption is
		// worth eight. Both are ordinary numbers of fights and proposals
		// within one life, so a belief is a lifetime's impression and not a
		// tally of everything that ever happened.
		LearningRate:   0,
		LoreMemory:     60,
		LorePriorCount: 8,

		// How much the population disagrees about what is worth doing.
		// Calibrated rather than guessed: at 0.25 it costs a fifth of the
		// population, at 0.15 the cost is inside the noise (-4.2 +/- 4.4 over
		// 24 seeds) and the spread still standing at the end of a run is
		// real. The jog on inheritance is smaller than the gene mutation
		// because it happens at every birth rather than one in fifty.
		LoreInitSpread:  0.15,
		LoreMutationStd: 0.08,

		// Two percent of the gap, not the half that meeting in the middle
		// would suggest. The symmetry is in the rule, not in the size: both
		// sides move by the same amount whatever this is, and what it sets is
		// how much one meeting counts for.
		//
		// It has to be small because meetings are not rare. A run holds
		// something like nineteen thousand of them, and at a half the
		// population converges on one set of values within a couple of
		// generations - which is the thing stage 12a measured as harmful (a
		// population that all wants the same thing stops rewarding reading
		// the world accurately). At a half the spread of preferences falls
		// from 0.089 to 0.055 and the population halves over a long run; at
		// 0.02 both are back where they were without it, and agents still
		// end up with three times as many others they are fond of.
		LoreExchangeRate: 0.02,

		// What one trade is worth in affinity. Swept: between 1 and 30 it
		// moves neither the population nor how many others an agent ends up
		// fond of by anything the noise does not cover, because what the
		// trade is worth is dominated by how often it happens rather than by
		// this. Left at the figure calibrated against a birth (18): a trade
		// in which each side gives up a tenth of one of its five figures is
		// worth about a third of a child.
		AffinityLore: 30,
		LoreValue:    9,

		// Four ideas at most, at five budget apiece. The price was swept
		// against a world with no room for ideas at all: at fifteen a full
		// set costs a gene and a half and the population pays for it (-3.9
		// over 48 seeds), at ten it still does (-2.7), and at five the cost
		// is inside the noise (+1.5) while the ideas still have to be paid
		// for out of the same budget as the body. Free would have been
		// simpler and would have removed the trade the stage is about.
		HintSlots:      4,
		HintSlotCost:   5,
		HintWeightStd:  6,
		HintWeightMax:  20,
		HintTradeWorth: 0.5,
		HintsSpread:    true,
	}
}
