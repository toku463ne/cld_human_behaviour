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

	// HighGroundCover is the chance a blow thrown up at somebody misses
	// because of the ground rather than because of what they did (stage 30).
	// Zero is off, and a world with no map comes to zero whatever it is set
	// to, since nothing there is above anything.
	//
	// A floor rather than a multiplier on the body's own evasion, because
	// evasion is a stance channel: a body has one only while it is fighting or
	// running, so a multiplier would be multiplying zero for the six ticks in
	// seven that it is doing something else. Being behind a bank helps the
	// one eating as much as the one guarding.
	//
	// Capped with everything else at EvasionCap, and a step rather than a
	// slope - two levels up is not twice as hard to reach as one - because
	// what a level buys is the advantage itself.
	HighGroundCover float64

	// WatersideFood makes the ground beside water grow more of it (stage 36).
	// Positive is the river bank of the real world - the one piece of country
	// that is not simply worse for being dear - and negative is the other
	// landscape, where the water is a barren strip.
	//
	// A term of its own rather than part of TerrainFoodCorrelation (#62): the
	// two say opposite things about the same ground (hard country is poor;
	// the bank is rich), and folded into one multiplier a map's author could
	// not ask for both.
	//
	// Zero is the world before this stage, and so is any value in a world
	// with no map. Like stage 33 it moves the food and does not add any: the
	// regions are scaled back to the total they had.
	//
	// The default is zero for the reason stage 33's is: which landscape this
	// is belongs to whoever draws the map, and making it a rule of the world
	// would mean no recorded measurement could be reproduced from any arm.
	WatersideFood float64

	// What an agent makes of how dangerous a region is (stage 35).
	//
	// RegionDangerTicks is how long a stay the belief is priced for. The
	// chance itself is believed in the units the ground carries (a chance per
	// tick), and the comparison between one piece of country and another runs
	// in food-in-sight, so something has to convert. Nothing is invented for
	// it: the chance is priced exactly as the hazard underfoot is priced in
	// the utility formula - the chance, times the ticks it is run for, times
	// LifeValue - and this is the ticks. Zero turns the belief out of the
	// comparison while leaving it formed, which is the arm that says what
	// knowing is worth.
	//
	// The default is a planning horizon, which is what every other "how far
	// ahead is this agent reckoning" in the world comes to.
	RegionDangerTicks float64

	// RegionDangerTold says whether the danger travels between agents on the
	// same trade the rest of the country does (stage 35). False leaves it
	// learnable only by having been there, which is the control that says
	// what handing it on adds - the same pair stages 15c and 29b were
	// measured with.
	RegionDangerTold bool

	// DrownWitnessLooks is how many ordinary looks at a region seeing
	// somebody drown in it is worth (stage 35). It is the whole of "the
	// drowning is news": without it an agent can only learn that water kills
	// by standing in it, and the ones it kills do not come back to say so.
	//
	// Heavier than a look because a death is worth more than a stroll, and
	// weighed in looks rather than in a new unit so that there is only one
	// scale in this belief. Zero turns witnessing off and leaves the belief
	// to first-hand experience.
	DrownWitnessLooks float64

	// TerrainFoodCorrelation ties where the plants come up to how hard the
	// ground is to cross (stage 33). Positive is the ordinary reading of a
	// landscape - broken country is also poor country - and negative is the
	// other bargain, where the hard ground is where the food is.
	//
	// Zero is the world before this stage. So is any value in a world with no
	// map: every region costs the same, so nothing is above or below the
	// average and no region's share moves.
	//
	// It changes where the food grows and not how much of it there is: the
	// regions are scaled back to the total they had (region.go), for the same
	// reason stage 15a did.
	TerrainFoodCorrelation float64

	// What an agent makes of how hard a region is to cross (stage 29).
	//
	// RegionCostWeight puts the going and the food in the same units: what one
	// extra multiple of crossing cost is worth in food-in-sight. Zero is the
	// world before stage 29 - the belief is still formed, and nothing reads
	// it. In a world with no map every cost is 1, so no weight changes
	// anything: a flat world is bit for bit what it was.
	//
	// RegionCostForgetPerTick is how fast the belief fades, and it is zero
	// because the ground does not move while an agent is away. It exists so
	// that a world where the ground does move - erosion, an administrator
	// redrawing the map - only has to set a rate rather than grow a mechanism.
	//
	// RegionCostTold is whether the going is handed on with everything else
	// two agents trade. False is stage 29a without 29b, which is the control
	// that says what hearing about it is worth.
	RegionCostWeight        float64
	RegionCostForgetPerTick float64
	RegionCostTold          bool

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

	// The two prices that were missing when stage 17b was first measured, and
	// the reason it was left off (2026-09-08).
	//
	// PlantPoisonSaves is the chance, in proportion to how poisonous it is,
	// that a plant survives being bitten: the eater takes the dose and gets
	// nothing, and the plant is still standing. Without it poison cannot be
	// selected for at all - an eaten plant is gone whatever it was carrying,
	// so what a plant passes on has nothing to do with what it was defended
	// with, and the gene can only drift. It was drifting: 0.55 after twenty
	// thousand ticks, from a starting mean of 0.5.
	//
	// PlantSignalCost is what being conspicuous costs in seed. Without it
	// shouting is a benefit with no price: loud plants are avoided, avoided
	// plants stand, and the whole crop ends up shouting - which at a dose
	// that matters is a population that starves beside its food. It is the
	// plant-side price PARAMETERS.md named as the condition for reviving this
	// stage.
	//
	// PlantPoisonCost is what being poisonous costs in seed, the same way
	// PlantSignalCost is what being loud costs. Poison with a benefit and no
	// price wins outright - measured at 0.75 of the maximum, with three
	// bites in four coming to nothing and the population starving beside a
	// full field - so a defence that saves a plant has to be paid for in the
	// only currency a plant has.
	//
	// All three are zero by default, which is the world stage 17b was
	// measured in.
	PlantPoisonSaves float64
	PlantSignalCost  float64
	PlantPoisonCost  float64

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

	// CarryCapacity is how many items a body of ordinary build can hold at
	// once (stage 40). Zero is the world before carrying, where food was only
	// ever in the ground or in a stomach.
	//
	// One, because one is where the sweep changes sign. A body that can hold
	// a single item starves a quarter less often and the world grows; at two
	// and at three the population falls, and it falls further the more
	// carrying there is. What costs is not the weight (an arm with the load
	// free costs the same) nor the bookkeeping (taking held items out of the
	// world's allowance costs more, not less) - it is the carrying itself:
	// a body that walks to a meal to pick it up rather than to eat it has
	// spent the trip and is no less hungry.
	//
	// It scales with the vitality gene rather than with a gene of its own
	// (#65): a tenth gene would draw on the same budget as the nine and move
	// every share at once, which would make every figure recorded since stage
	// 7c a different measurement.
	CarryCapacity float64

	// CarryCost is what a full load adds to the cost of moving, as a
	// multiplier (0.5 means a fully laden body pays half as much again).
	// Weight against vitality, never against power (#66): power is combat
	// efficiency and nothing else, and a second job would make the two
	// pressures on it impossible to tell apart.
	//
	// It goes on the cost and not on the speed, because effort is already on
	// the speed - the same reason stage 20 put the ground's figure there.
	CarryCost float64

	// CarryValue is how much of a future meal an agent reckons a held item is
	// worth: one would be a body that values food in hand exactly as much as
	// food in its stomach, which would leave it holding a meal while it
	// starved. It is the discount that keeps eating ahead of hoarding.
	CarryValue float64

	// CarryOffTheBooks takes what is held out of the world's allowance for
	// food. False is the rule: an item in a pocket is food the world still
	// has, so it counts, and carrying moves food about rather than making
	// room for more of it.
	//
	// The arm exists because the two readings of the allowance are different
	// claims about what it is. Counting says the allowance is how much food
	// exists; not counting says it is how much lies about on the ground, and
	// a plant in somebody's hand does not stop another growing. Which one
	// the world runs on decides whether carrying withdraws food from
	// circulation, so it is measured rather than assumed.
	CarryOffTheBooks bool

	// CarriedMeatKeeps stops the spoiling clock for what is being carried.
	// False is the rule, and the arm is here to settle #77 now that carrying
	// exists: measured on the ground beforehand, meat that never rots at all
	// left the population where it was, but that was meat nobody came for
	// rather than meat somebody chose.
	CarriedMeatKeeps bool

	// FishShare is how much of what the world grows comes up as fish in the
	// water instead of as a plant on land (stage 42). Zero is a world with no
	// fish in it, which is every world before this and every flat world after
	// it - with no water there is nowhere for one, and no random number is
	// drawn deciding.
	//
	// It takes the place of a plant rather than adding to the world, so
	// FoodSpawnRate still says how much food there is (#69). How much of it
	// is in any one region follows from how much of that region is water,
	// with nothing to tune: a fish goes in a water cell drawn uniformly.
	//
	// Zero by default, the same as the other two rules that say what a
	// landscape provides (TerrainFoodCorrelation, WatersideFood): what is in
	// the water is a property of the country somebody drew, not of the world.
	// It is also the only figure measured so far that moves where bodies
	// stand by a lot - a quarter of the crop in the water takes the share of
	// the population standing in it from 0.13 to 0.24 *** - and it costs a
	// quarter of the population to do it, which is a price a map should ask
	// for deliberately rather than one every measurement inherits.
	FishShare float64

	// SkillFishReach is how much further a body can take a fish for each
	// point of skill at fishing from the bank (stage 43): at 2 and full
	// mastery, three times the ordinary reach, which is far enough to keep
	// dry beside a river cell. With no skill the reach is the ordinary one,
	// so a world where nobody has learned it is exactly the world before.
	//
	// It buys no food. What it buys is not being in the water, which is
	// worth exactly what drowning costs - and that is swimming's figure, not
	// this one. The two skills must not both pay for the same thing or
	// neither can be measured.
	SkillFishReach float64

	// FishCatchWater and FishCatchBank are the chances of landing a fish that
	// has been reached, standing in the river and reaching in from dry ground
	// (stage 43). Each skill lifts its own side towards certainty.
	//
	// This is where the two halves of the trade meet. In the water most
	// attempts land, and the drowning rule charges by the tick for being
	// there; from the bank nothing charges anything and most attempts fail.
	// What a body has learned decides which of the two is worth its time.
	//
	// The first version of this stage made a fish worth more calories to a
	// skilled body instead, borrowing foraging's shape - and inherited its
	// consequence, that a body needing fewer fish spends less time fishing.
	// Being good at something in this world had only ever meant needing less
	// of it; here it means getting more, which is what fishing is.
	// Both are one by default: a fish that has been reached is a fish taken,
	// which is the world stage 42 measured. What a landscape's fish are like
	// to catch is the map author's to say, as what grows there is.
	FishCatchWater float64
	FishCatchBank  float64

	// SkillFishRelief scales what the two fishing skills are worth, the same
	// way SkillSwimRelief and the rest do for theirs. Zero is the control the
	// stage is read against: the skills are learned, take the room, and buy
	// nothing.
	SkillFishRelief float64

	// FishForAll lets enemies fish too. False, because a second food for the
	// species that lives on meat would make every population figure since
	// stage 11 a different measurement; the arm is here to be run.
	FishForAll bool

	// MeatSurplusFree opens what a kill leaves beyond what those who made it
	// can carry away (stage 41). True, because it wastes less and costs
	// nothing: the share of meat that rots falls by 0.03 **, starvation per
	// lifetime by 0.16 *, and the population does not move. False is the
	// world where a claim covers the whole carcass.
	//
	// What it buys grows with how much meat there is. In a world where a
	// carcass leaves twice as much, opening the surplus is worth 10.90 *
	// population and 0.25 *** on the size of a hunting party - and that same
	// doubling, which cost 13 population when it was measured in stage 39a,
	// is worth 23.94 *** once carcasses mend and the surplus is open. The
	// figure did not change; the rules around it did. The claim itself is unchanged - it is still
	// "whoever brought it down eats first" - and what changes is how much of
	// the carcass it covers.
	//
	// The definition costs no new figure: the surplus is what is left after
	// every participant's carrying capacity, so a party of strong bodies
	// leaves little and a party of weak ones leaves a good deal (#68). And
	// nobody is robbed by it: what the party cannot take away was never
	// theirs to wait for, so the rule needs no spite, no witnessing and no
	// competition of its own - the food rules already have all of that.
	MeatSurplusFree bool

	// MeatVitality is what one item of meat mends, as a share of the eater's
	// own vitality ceiling (stage 39). Zero is the world as it was up to here,
	// where food only ever took hunger away and vitality came back from
	// resting alone; this is the first food that touches vitality directly.
	//
	// Half is the default because it is what finally made pack hunting appear
	// after four attempts that did not (party size 1.36 -> 1.81, joint kills
	// 34 -> 95 over 48 seeds), and because a carcass that merely fills a
	// stomach twice as full does almost nothing (+0.06): what brings bodies
	// in on the same animal is that the animal mends them. A whole ceiling
	// measures the same within noise and is not the default only because the
	// smaller dose leaves the loop - kill, mend, kill again - with less room
	// to run away in the stages that come after this one.
	//
	// It needs no new term in the utility formula. The healing goes in where
	// resting's does - as vitality the body expects to have afterwards - so
	// the existing ceiling does the rest: a nearly whole body scores a
	// carcass low, which is "save it for later" without a state that saves
	// anything.
	MeatVitality float64

	// MeatHealKnown is whether what a carcass mends reaches the decision.
	// True is the ordinary case - a body knows what a meal does for it, the
	// same way it knows it is sick of something (stage 16). False is the
	// control stage 34 paid for: the meat mends exactly as much and no
	// utility formula is told, which separates a rule that changes what
	// agents choose from one that merely changes who survives.
	MeatHealKnown bool

	// MeatNutrition multiplies what one item of meat takes off hunger. One is
	// the ordinary item. It exists for the control the stage needs: a carcass
	// that fills a stomach by as much as the healing was worth and mends
	// nothing separates "meat became nourishing" from "meat became medicine".
	MeatNutrition float64

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

	// A blow that finds nothing leaves the one who threw it off balance:
	// for OpeningTicks afterwards its own guard and dodge are worth
	// OpeningGuard of what they would be. It is the first rule in the world
	// that prices missing, and it is deliberately not something anybody can
	// see coming - the evasion gene is a hidden parameter, so nothing can
	// weigh "this one is hard to hit" before swinging. Only the outcome
	// changes, and selection does the rest.
	//
	// OpeningTicks = 0 turns it off completely, and off it draws no random
	// numbers and touches nothing, which is what makes the pair comparable.
	OpeningTicks int
	OpeningGuard float64

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
	// LooksShowBulk makes what an observer sees of a body its total size
	// rather than its vitality and speed genes averaged (stage 10, see
	// Agent.Appearance). Attack stays hidden either way; the difference is
	// whether the visible thing is one of attack's competitors for the budget
	// or the budget itself.
	LooksShowBulk bool

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

	// AffinityWitnessKill is what an onlooker comes to think of somebody it
	// has just watched kill a creature of another kind (stage 31).
	//
	// It is the third party's version of AffinityHunt: that one is earned by
	// being on the carcass's claim, this one by having been there to see it.
	// Which way the sign goes is not a second rule - see witnessKill - and
	// the record is taken only if there is room for it, the way a trade of
	// assumptions is: seeing something happen is not the same as having it
	// done to you, and it must not throw out somebody who matters.
	//
	// Zero turns the affinity half of stage 31 off; how often it had the
	// chance to fire is counted either way.
	AffinityWitnessKill float64

	// KillWitnessFactor is how much coarser a reading taken from watching
	// somebody kill is than one an agent paid ActObserve for (stage 31). It
	// multiplies the variance of an onlooker's reading, so above one is worth
	// less than a look and below one would be worth more.
	//
	// It must stay above one. Observing costs vitality and time and can only
	// be spent on one person at a time, and a world where standing there and
	// seeing it happen teaches as much is a world where nobody would ever
	// choose to watch anybody (#55). The share of decisions that are
	// ActObserve is measured for exactly this reason.
	//
	// Zero turns the reading half of stage 31 off.
	KillWitnessFactor float64

	// --- calling others in (stage 32) ---

	// CallTicks is how long the calling itself takes and how long the call
	// stands afterwards. Zero takes the word out of the vocabulary: no agent
	// is offered the option and none is ever seen calling, which is the arm
	// the whole stage is measured against.
	CallTicks int

	// AllyTrustWeight is how much of somebody else's strength counts towards
	// a fight this agent is thinking about, when that somebody has declared
	// for the same target - by calling for it, or by already hitting it.
	//
	// What it is multiplied by is trust, not affinity: an ally is worth its
	// strength times how sure this agent is that it will still be there when
	// the blows land (Affinity/AffinityTrust, the figure resting already uses
	// - see #56). That is the whole of the rule. There is no bonus for
	// joining in, no threshold above which an invitation is accepted, and no
	// gene for being cooperative: whether to come is scored by the same
	// comparison as everything else, and what the affinity does is make the
	// arithmetic of a shared fight add up.
	//
	// Zero means nobody is ever counted on, which leaves the invitation
	// pointless but still spoken - the two halves are measured apart.
	AllyTrustWeight float64

	// AllyPreyOnly restricts who may be counted on to fights against another
	// kind of creature - a hunt rather than a quarrel.
	//
	// It is here to answer one question the measurement asked: the rule was
	// written for bringing something down together, but nothing in it says
	// the target has to be prey, so two agents ganging up on a third get the
	// same arithmetic. Which of the two the world actually got its gain from
	// is not a thing to reason about from an armchair.
	AllyPreyOnly bool

	// KillWitnessLooks says whether a reading taken from watching a killing
	// also goes into what that agent thinks a build says about a blow
	// (appearance.go), the way every other reading does.
	//
	// It has a switch of its own because the readings from a killing are the
	// only ones in the world that are not a sample of who is about: every one
	// of them is of somebody who has just won a fight. Stage 10 found that
	// nearly all of what the line is worth is in its level - what the average
	// body hits like - and a level learned from winners is not that average.
	KillWitnessLooks bool

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

	// --- skills (stage 38a) ---

	// SkillRoughRelief is how much of the extra cost of broken country a
	// fully mastered, fully suited body is spared. Zero takes skills out of
	// the world - nothing is drawn, learned or copied - which is the arm the
	// stage is measured against.
	//
	// It applies to the excess over level ground and not to the cost itself,
	// so no amount of skill makes crossing rough country cheaper than
	// crossing a field: what is learned is how not to be slowed by it, not
	// how to walk for free.
	SkillRoughRelief float64

	// SkillBirthplace is how much of the country a body was born into it
	// knows by having been born there: the share of that region which is hard
	// going, times this. It is the only one of the three paths to a skill
	// that needs nobody to have known it first, so at zero no skill ever
	// enters the world and nothing else about skills can fire.
	//
	// Zero is the default, and that is the same call stages 30a, 33 and 36
	// made: a rule that only means anything where there is terrain is
	// something the map's author turns on, not something the physics does.
	// It keeps every measurement taken on rough, river and country before
	// this stage readable, and cmd/devview -terrain passes 0.5 like it passes
	// the other three.
	SkillBirthplace float64

	// SkillAptitude is, for each skill, the gene its realised value is capped
	// by: how much of a nominal figure this body can support.
	//
	// The rule for choosing is the gene the doing already belongs to. Crossing
	// ground is legs (speed); getting more out of what has been found is what
	// the body keeps of what it has learned about it (memory), which is the
	// first thing in this world to ask anything of that gene.
	//
	// It is a table and not a constant because the choice is a real fork:
	// capping rough going with toughness instead would make broken country
	// the place where slow, tough bodies do well - the niche stage 20 went
	// looking for and did not find - and whether that happens is a question
	// for a measurement rather than for the design.
	SkillAptitude [NumSkillKinds]Gene

	// SkillForageRelief is how much of the discount for eating the same thing
	// over and over (stage 16) a fully mastered, fully suited body escapes.
	// Zero leaves the skill able to be learned and worth nothing, which is
	// the arm that separates what it costs from what it buys.
	SkillForageRelief float64

	// SkillPoisonRelief is how much of a plant's dose a fully mastered, fully
	// suited body escapes - and, because a body knows its own stomach, how
	// much less it prices the warning at when deciding whether to eat.
	//
	// The second half is the point. Counted before it was written: at a dose
	// of 2 the poison itself takes 2.1% of what a body recovers, which is
	// less than the discount foraging failed to move. What the crop actually
	// costs the population is the avoiding - starving is up 66% and the
	// population down 17% - so a defence worth having is one that makes a
	// body willing to eat, not one that makes the mouthful cheaper.
	SkillPoisonRelief float64

	// SkillSwimRelief is how much of the chance that a tick in the water is
	// the last one (stage 34) a fully mastered, fully suited body escapes.
	//
	// The gene behind it is vitality, because what stage 34 found decides who
	// comes out of a river is how long a body stays in it rather than how
	// fast it crosses. Zero is the same kind of arm as the one above.
	SkillSwimRelief float64

	// SkillGeniusJump is how much further than its line a genius child goes
	// at something the line already does (GeniusRate, world.go). It reuses
	// the event the world already has for a rare, large change rather than
	// inventing a second one, and it cannot invent a category: where there is
	// no rough country there is nothing to be a genius at crossing.
	SkillGeniusJump float64

	// SkillsSpread is whether a skill can be copied by watching somebody who
	// has it, the way an idea can (HintsSpread). It is separate because the
	// copying is not the same copying: an idea goes into an empty slot only,
	// a mastery is compared and may overwrite a worse one, and that
	// difference is the whole of what the stage claims about diffusion.
	SkillsSpread bool

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

	// SightingRetriggers makes the two sighting triggers fire on every tick
	// something is in view rather than on the tick it comes into view. That
	// is how they behaved before the edge was put in, and it is the arm this
	// is measured against.
	//
	// The level version asks an agent crossing a place with food in it to
	// think again on every tick of the crossing, about a thing it decided to
	// walk past a tick earlier. It costs 2.4x the whole simulation and buys
	// the agent a second look at food it has already considered.
	SightingRetriggers bool

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

	// --- the ground (stage 20) ---
	//
	// TerrainMap is the country the world is laid out on, one string per row
	// and one rune per cell (terrain.go says which rune is what). Empty is a
	// flat world, which is what every measurement before this stage was taken
	// in and still the default: ground is something a map gives a world, not
	// something the world has by nature.
	//
	// The three costs are multipliers on what a tick of movement takes out of
	// a body. They are costs and not speeds on purpose - see terrain.go.
	TerrainMap    []string
	RoughMoveCost float64
	WaterMoveCost float64
	SlopeMoveCost float64

	// DrownChancePerTick is the chance that a tick spent in the water is the
	// last one (stage 34). It is what makes a river something other than a
	// dear stretch of ground: the first thing the country itself does that
	// kills.
	//
	// A chance of dying outright rather than damage that accumulates, for
	// three reasons. It adds no multiplication to power or defence, so no
	// ability takes on a second role. It rewards the speed an agent already
	// has without inventing a swimming one: crossing in fewer ticks is
	// throwing the dice fewer times. And it needs no threshold anywhere -
	// whether to go into the water comes out of the same comparison as
	// everything else, with the chance priced into the options that would
	// keep the body there (controller.go).
	//
	// Zero is the world before this stage, and so is any value in a world
	// with no map: a flat world has no water in it, draws no random number
	// here and runs identically.
	DrownChancePerTick float64

	// DrownKnown says whether an agent feels how dangerous the ground it is
	// standing on is (stage 34). True is the ordinary world: the chance is in
	// Perception.Self.Drown and priced into every option that would keep the
	// body there. False leaves the water just as deadly and the agent unable
	// to reckon with it at all, which is the control arm that says how much
	// of what changes is the choosing and how much is simply the ones who
	// stayed in the river being gone.
	DrownKnown bool

	// CourtAnswerTicks is how long a suitor stands and waits when the one it
	// has proposed to answers proposals for itself (a person, that is: no AI
	// controller does). When it runs out the agent's own rule answers, which
	// is what would have happened straight away otherwise.
	CourtAnswerTicks int

	// BirthNeedsLowerIDFirst restores the bug fixed on 2026-09-06: a bond that
	// had run its course only produced a child when the loop happened to
	// reach the lower-numbered of the two partners first, which was about half
	// the time. False is the fixed world; true is every measurement taken
	// before that date, and is the only way to compare against them.
	BirthNeedsLowerIDFirst bool

	// ParentFeedShare is how much of every mouthful goes to the children this
	// agent is still rearing instead of into itself. Nobody chooses it and
	// nobody can refuse: an agent that eats with a child of its own beside it
	// simply gets less of the meal (provision.go). What is divided is the
	// mouthful, not the item, so nothing about owning or racing for food
	// changes; and the parent is told, so its perception is of half meals.
	//
	// Zero is childcare as it was before 2026-09-06: the child does the
	// staying and the parent is asked for nothing.
	ParentFeedShare float64

	// The two controls the rule is measured against, because it does two
	// things at once and they have to be told apart.
	//
	// ParentFeedWasted throws the child's share away instead of giving it to
	// the child: the parent pays exactly the same and nobody is fed, which is
	// what says how much of any change is the cost rather than the feeding.
	//
	// ParentFeedKnown is whether the parent's own perception says its meals
	// are half meals. True is honest and is the default; false leaves it
	// planning as though it ate the lot, which is the arm that says whether
	// knowing is worth anything.
	ParentFeedWasted bool
	ParentFeedKnown  bool

	// RearingUntilGrown ties the leash to the child rather than to the clock:
	// it keeps to the parent until it has finished growing, however long that
	// takes, instead of for ChildRearingTicks. The two are not the same length
	// even on paper - growth is bought with food, so a hungry child is still a
	// child long after the fixed timer would have let go of it.
	//
	// False keeps the fixed count, which is what every measurement before
	// 2026-09-06 was made under.
	RearingUntilGrown bool

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

	// Whether those two are a gate on the option or a reason inside the
	// comparison. True is the world as it was: a hungry agent is not offered
	// courting at all. False leaves the judgement to the utility formula,
	// which prices the birth itself (see AIController.addCourt) - and to
	// whoever is playing the node, which is the point of it.
	//
	// What is never a judgement is whether the body can pay for a birth at
	// all; that stays a refusal either way.
	CourtNeedsSurplus  bool
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
	MutationRate float64
	MutationStd  float64
	// Whether the comparison clock restarts every time an agent takes up
	// courting again, or runs from when it first became able to. True is the
	// world as it was, and it means "patience" measures the current attempt
	// rather than the search: an agent turned down goes back to the beginning,
	// so it never runs out and never settles.
	CourtClockResets bool

	PatienceBase        float64 // ticks of comparison before committing to a mate
	PatienceRationality float64 // extra patience per point of rationality
	CommitFitness       float64 // a candidate this good is worth committing to at once
	// CommitFloor is what an agent will still not settle for, however long it
	// has been comparing. At 0 there is no floor and a patient agent accepts
	// anybody, which is the world before stage 26.
	CommitFloor        float64
	MateRejectDuration int // ticks a passed over candidate is left aside

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

		// Off. Turned on for the terrain arms and measured there (stage 30);
		// in a flat world the figure never applies.
		HighGroundCover: 0,

		// Zero: the ground and the food are drawn independently, which is
		// what every measurement before stage 33 was taken in.
		TerrainFoodCorrelation: 0,

		// Two food items in sight is worth about as much as ground that costs
		// twice as much to cross. A first guess, to be swept (stage 29).
		// A planning horizon's worth of stay, which is what the rest of the
		// world reckons over. See HISTORY.md, 2026-09-07.
		WatersideFood: 0, // the landscape's own business; see HISTORY.md, 2026-09-07

		RegionDangerTicks: 700,
		RegionDangerTold:  true,
		DrownWitnessLooks: 20,

		RegionCostWeight:        2.0,
		RegionCostForgetPerTick: 0, // the country does not move
		RegionCostTold:          true,
		RegionDrawValue:         4,

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
		CarryCapacity:   1,
		CarryCost:       0.5,
		CarryValue:      0.5,
		FishShare:       0, // stage 42: what a map holds is the map author's to say
		MeatSurplusFree: true,
		MeatVitality:    0.5, // stage 39: half of the eater's own ceiling. 0 is the world before it
		MeatHealKnown:   true,
		MeatNutrition:   1,
		MeatClaimTicks:  400,
		MeatSpoilTicks:  900,
		HuntCreditTicks: 200,

		AttackDamage: 1.15,
		AttackCost:   0.30,
		DefenceCap:   0.55,
		DefenceCost:  0.12,
		EvasionCap:   0.45,
		EvasionCost:  0.20,
		FleeEffort:   0.95,

		// Three ticks is about a fifth of the SkirmishTicks a fight is priced
		// over, so an opening is a moment inside a fight rather than the shape
		// of the whole thing. Guard at 0.4 leaves something rather than
		// nothing: an agent that misses is exposed, not helpless.
		OpeningTicks:  3,
		OpeningGuard:  0.4,
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
		AffinityWitnessKill:  2,
		KillWitnessFactor:    2,
		KillWitnessLooks:     true,
		CallTicks:            30,
		AllyTrustWeight:      1,
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

		BirthNeedsLowerIDFirst: false, // see HISTORY.md, 2026-09-06

		// No map: a flat world, exactly as it was. The costs are what the
		// runes mean when a map does turn up.
		TerrainMap:    nil,
		RoughMoveCost: 2.0,
		WaterMoveCost: 3.0,
		SlopeMoveCost: 2.0,

		// Calibrated against how long a body actually spends in the water,
		// which was counted before the rule was written: about a seventh of
		// all agent-ticks on the river map, in stays averaging 127 ticks. At
		// this rate a stay of that length is survived nineteen times in
		// twenty and drowning is a few per cent of all deaths - see
		// HISTORY.md, 2026-09-07.
		DrownChancePerTick: 0.0002,
		DrownKnown:         true,

		// Long enough to read the proposal and decide, short enough that a
		// player who has walked away does not hold a stranger in place. The
		// same sixty ticks a question in the asked mode stands for.
		CourtAnswerTicks: 60,

		// Zero: see HISTORY.md, 2026-09-06.
		ParentFeedShare:  0,
		ParentFeedWasted: false,
		ParentFeedKnown:  true,

		// False: see HISTORY.md, 2026-09-06. Tying the leash to growing up
		// instead costs the world population without buying the cohesion it
		// looks like it should.
		RearingUntilGrown: false,

		// As far as the child can see, which is the point: keeping to a
		// parent has to mean staying in the same neighbourhood, not staying
		// within arm's reach. A short leash measurably kills children - they
		// get dragged off the food they were going for (grewUp 0.46 against
		// 0.56 with no leash at all).
		RearingRadius: 130,

		FitnessConditionWeight: 0.35,

		// False: the judgement is the comparison's, and the player's. See
		// HISTORY.md - the threshold was standing in for a term that was
		// missing from the formula, and with the term there it costs more
		// than it buys.
		CourtNeedsSurplus:   false,
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
		// False: the clock measures the search, not the latest attempt. See
		// HISTORY.md - restarting it on every attempt meant a rejection put an
		// agent back at the beginning of its patience, so it held out for an
		// obvious catch for ever and nobody settled for anybody.
		CourtClockResets:    false,
		PatienceBase:        25,
		PatienceRationality: 0.5,
		CommitFitness:       78,
		// About the bottom quarter of the mate fitness in a running world
		// (p25 was 37.5 when this was calibrated). Measured, it costs no
		// population at all and buys a less violent world, fewer children
		// dying, longer lives, and the first real selection on the looks
		// gene. A floor of 50 - above the median - starts costing.
		CommitFloor:        38,
		MateRejectDuration: 40,

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
		HintSlots:        4,
		SkillRoughRelief: 0.6,
		SkillBirthplace:  0,
		SkillGeniusJump:  0.4,
		SkillAptitude: [NumSkillKinds]Gene{
			SkillRough:  GeneSpeed,
			SkillForage: GeneMemory,
			SkillSwim:   GeneVitality,
			SkillPoison: GeneVitality,
			// Fishing from the bank is patience and a good eye - reading
			// where the fish is from outside the water - so it is capped by
			// rationality, the gene for how well a body reads the world.
			// Fishing in the water is capped by vitality, the same as
			// swimming: what decides how much of the river a body can work
			// is how long it can stay in it.
			SkillFishLand:  GeneRationality,
			SkillFishWater: GeneVitality,
		},
		SkillForageRelief: 1,
		SkillSwimRelief:   1,
		SkillPoisonRelief: 1,
		SkillFishReach:    2,
		FishCatchWater:    1,
		FishCatchBank:     1,
		SkillFishRelief:   1,
		SkillsSpread:      true,
		HintSlotCost:      5,
		HintWeightStd:     6,
		HintWeightMax:     20,
		HintTradeWorth:    0.5,
		HintsSpread:       true,
	}
}
