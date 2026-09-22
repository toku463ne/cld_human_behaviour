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
	// SpawnMap is where things are allowed to come up, painted one character
	// per cell (2026-09-19): 'p' plants, 'f' fish, 'e' enemies, 'F' plants and
	// fish, '*' all three, '.' nothing. A short row, and any character nobody
	// knows, are bare ground.
	//
	// Empty is every world before it: plants land where the region weights
	// send them, fish anywhere in the water, enemies where stage 58's
	// weighting puts them.
	//
	// It says where and never how much. FoodSpawnRate still decides how many
	// plants a tick and EnemySpawnTicks how often an enemy walks in, so the
	// conservation stage 15a insisted on holds inside the painted squares
	// rather than over the whole map.
	//
	// A fish painted onto dry land is ignored rather than refused: stage 42's
	// invariant is that fish are grown in water, and a drawing may be wrong
	// about where the water is without the world having to stop.
	SpawnMap []string

	// PlantKinds are the sorts of plant this map grows (decision #134,
	// plant.go). Empty is every world before it: one sort, and the tag on
	// each plant stays zero.
	//
	// A kind is a bundle of starting values for the four genes a plant
	// already has, not an axis of its own - so it changes nothing at all in a
	// world where plants do not inherit (PlantGenetics and PlantDefence are
	// both off by default). Adding a sort is a row and no code, the way
	// EnemyKinds is.
	PlantKinds []PlantKind

	// PlantKindMap says which sort grows where, one character a cell, each
	// character being a PlantKind's Key (2026-09-20). A dot, a short row and
	// a character nobody claimed are "nothing painted here", and there the
	// sort is drawn by Share as it always was.
	//
	// The painting wins over the draw where there is one, the same order the
	// spawn mask and the richness are read in: it is the more particular
	// thing the author said about that cell.
	//
	// It only decides what comes up out of the ground. A seedling keeps its
	// parent's sort wherever it lands, because the tag rides with the genes -
	// otherwise a lineage would change sort by walking, which is not what a
	// kind means.
	PlantKindMap []string

	// FishSpawnRate and MaxFishItems give the water its own pool
	// (2026-09-20). Zero on both is every world before it: a fish comes up
	// *instead of* a plant at the FishShare rate, and the two share one
	// allowance, so FoodSpawnRate alone says how much the world grows.
	//
	// Above zero, fish come up on their own schedule and against their own
	// ceiling, and FishShare is not read at all. The world then grows more
	// food than FoodSpawnRate says, which is a deliberate loosening of stage
	// 15a's conservation and the reason both default to zero: conservation
	// holds inside each pool rather than over the two together.
	//
	// It means every figure recorded for stages 42, 43 and 36 was taken in a
	// world where the water took food from the land. Turning this on does not
	// make those numbers wrong; it makes them numbers about a different
	// world, and they have to be read again before being quoted.
	FishSpawnRate float64
	MaxFishItems  int

	// FishRichMap is how well each cell of water grows fish, painted the way
	// RichMap is (2026-09-20). Empty is every world before it: a fish is put
	// in a water cell drawn evenly, so region by region there is exactly as
	// much fish as there is water.
	//
	// It is a separate painting from RichMap because the two answer different
	// questions about different ground, and one map saying both would make a
	// rich shore mean "more plants here" and "more fish here" at once with no
	// way to say only one of them.
	//
	// Painted onto dry land it does nothing: the draw is over water cells, so
	// a richness away from the water is never read. That is the same
	// forgiveness SpawnMap gives a fish drawn on a hillside.
	FishRichMap []string

	// RichMap is how well each cell grows things, painted one character per
	// cell (2026-09-20): a digit is that many fifths, so '5' is ordinary
	// ground, '0' grows nothing and '9' grows nearly twice as much. A dot, a
	// short row and any character nobody knows are ordinary.
	//
	// Empty is every world before it, down to the random source: the regions
	// draw their own richness from FoodSpread and nothing here runs.
	//
	// It says where and never how much. FoodSpawnRate still decides how many
	// plants a tick, so stage 15a's conservation holds inside the painting
	// rather than over the world's own blocks.
	//
	// Painting it replaces the world's own weighting rather than adding to
	// it: FoodSpread is not drawn, and TerrainFoodCorrelation and
	// WatersideFood do not run. The author said where the food is, and two
	// answers to that question would leave neither of them true.
	//
	// It is a separate picture from the regions on purpose (PLAN.md P16-2).
	// A region is what an agent believes about a stretch of country, and that
	// wants to stay coarse because a memory is small; where a plant comes up
	// wants to be whatever shape the author drew. The regions keep the mean
	// of this map, so a belief is still a belief about something true.
	//
	// Counted before it was built: finer is worse, not better. Cutting the
	// world into 48 or 108 blocks instead of 12 halves richGain, and the two
	// finer arms cannot be told apart - a block smaller than PerceptionRadius
	// stops being somewhere to go. What this is for is the shape and size of
	// the good ground, not its resolution.
	RichMap []string

	// RegionMap paints which cell belongs to which region, one character per
	// cell, each character being a RegionShape's Key (2026-09-20). A dot, a
	// short row and a character nobody claimed are "no region painted here".
	//
	// It is the other way to draw a region, and the better one: a painted
	// region may be any shape, while a rectangle can only be a rectangle.
	// Where both say something, the painting wins - painting a cell is the
	// more particular thing to have said about it.
	RegionMap []string

	// RegionShapes are the regions an author drew: rectangles in fractions of
	// the map (2026-09-19), or - when one carries a Key - the cells RegionMap
	// paints with that character (2026-09-20). Empty is every world before
	// that: the regions are RegionCols x RegionRows equal blocks, cut by the
	// world itself.
	//
	// A drawing need not cover the map - whatever is left over becomes one
	// more region, "everywhere else" - and a rectangle that names nothing but
	// a place keeps whatever the world's spreads drew there.
	//
	// It is a Config field rather than something a map file owns, because
	// this is the same standing TerrainMap has: the world is described, and
	// whoever describes it may be a test, a flag or a file read by something
	// that is not the engine.
	RegionShapes []RegionShape

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

	// FoodRenormalize keeps the world growing exactly as much food after the
	// map has had its say as before (stage 15a's rule, applied by stages 33
	// and 36). True is every figure recorded before stage 61.
	//
	// FoodTotalFromMap is the other half and the one that actually loosens
	// anything: with it on, how much the world grows follows the map's own
	// weights rather than being fixed by FoodSpawnRate alone, so a poor
	// country is poor rather than merely poorer than its neighbours.
	//
	// The pair exists because the first one on its own turned out to do
	// almost nothing (measured 2026-09-12): the region weights are only ever
	// used as proportions, so scaling them all back to their old total leaves
	// every proportion where it was. What it changes is the clamp, and
	// nothing else.
	FoodRenormalize  bool
	FoodTotalFromMap bool

	// EnemyKinds are the sorts of enemy this map has (stage 59, enemykind.go).
	//
	// Empty is the world as it was: one sort, drawn from EnemyBudgetMean and
	// EnemyBudgetStd and arriving wherever EnemySpread sends it. Filling it in
	// is how a map says "there are weak ones everywhere and heavy ones in the
	// bad country", and adding another sort later is another entry and no
	// code.
	//
	// It is not a third species. What tells one kind from another is what is
	// on its row - the same way stage 11 tells a human from an enemy - and no
	// rule anywhere asks which kind a body is.
	EnemyKinds []EnemyKind

	// LineageRule is how a line of descent is handed down (2026-09-20). It
	// changes nothing in the world - no rule reads a line - and everything
	// about what "my people" would mean to a game.
	//
	// The three were measured against each other; see HISTORY.md, and the
	// figures in the constants' own comments.
	LineageRule LineageRule

	// NestInherited makes a newborn's home its parent's nest rather than the
	// spot it was born on (2026-09-20). Off is every world before it, and off
	// is what it stayed after being measured.
	//
	// It was built to make a map's own weighting last. It does not. Over 24
	// seeds against the same seeds without it, prowlGain moves -0.00 +/- 0.02
	// and prowlKept +0.03 +/- 0.32 - nothing - while killShare rises +0.04 **
	// for reasons this stage did not explain. What erases the weighting is
	// not birth but wandering: stage 58 measured an enemy ending up 270 from
	// where it started against a region 200 wide, so the bias is gone inside
	// one life and there is nothing left for the next one to inherit.
	//
	// It is kept because a map that also holds its enemies near their nests -
	// EnemyHomeCost, which is off by default too - is a different question,
	// and this is the only way to ask it.
	NestInherited bool

	// EnemyKindMap says which sort of enemy comes into the world where, one
	// character a cell, each character being an EnemyKind's Key
	// (2026-09-20). A dot, a short row and a character nobody claimed are
	// "nothing painted here".
	//
	// It decides where a sort arrives and never how many of it there are:
	// which sort is coming is still its Share, and only then is the place
	// drawn from what the map painted for it. A sort the map paints nowhere
	// arrives the way it always did.
	//
	// It is the per-cell form of what EnemySpread and Homing already do with
	// whole regions, and it wins over them where it is painted - the same
	// order every other painting is read in.
	EnemyKindMap []string

	// NestRateMap is how often each painted nest is the one an arrival comes
	// out of, one character a cell over the same grid EnemyKindMap uses
	// (2026-09-22, TODO 18). A digit is that many fifths, so '5' is an
	// ordinary nest, '9' is one that sends out nearly twice as many and '0'
	// one that sends none; a dot, a short row and a character nobody knows
	// are ordinary. Empty is every world before this: the nests of a sort
	// share its arrivals alike.
	//
	// It is a share and not a rate, which is the whole of decision #143's
	// answer to "how many". How many enemies a tick the world lets in is
	// still EnemySpawnTicks and how many it holds is still MaxEnemies; what
	// a map paints here is which of its nests they come out of. The map
	// carries the proportion and Config carries the figure it is a
	// proportion of (#133), exactly as chill and heat do.
	//
	// Counted before it was built: the world sits against MaxEnemies in 0.53
	// to 0.57 of readings, and doubling that ceiling buys only 2.34 more
	// enemies before the world itself stops them. A per-nest rate therefore
	// moves arrivals about; it does not make a country's beasts more
	// numerous.
	NestRateMap []string

	// NestCap is how many bodies of its own sort an ordinary nest keeps
	// within Roam of itself before it stops sending any out, and NestCapMap
	// paints the same in fifths of it per cell (2026-09-22, TODO 18). Zero -
	// the default - is no cap at all, which is every world before this.
	//
	// What it does not do is stop anything being born. A nest that is full
	// simply does not let another one in from outside; the beasts already
	// there go on breeding, and that asymmetry is the point of the rule
	// rather than a limitation of it.
	//
	// It is a cap on what the world puts in, on the shelf MaxEnemies and
	// MaxPopulation and MaxFoodItems are on, and not a threshold on anybody's
	// behaviour: no body is told about it and no decision reads it.
	//
	// Counting a crowd by where it stands rather than by where it came from
	// is deliberate - an enemy carries no mark of which nest it belongs to,
	// NestInherited having been measured and left off - and it is also what
	// a player means by "this place is crowded". The cost is that it only
	// means much in a world that also charges for being away from home:
	// measured, the share of enemies standing within a nest's roam is 0.41
	// against 0.34 of the map at EnemyHomeCost 0, and 0.54 at EnemyHomeCost 2.
	NestCap    float64
	NestCapMap []string

	// BossBudget is what the master of a nest is built with, as a multiple of
	// what its row would have given an ordinary one of its sort (2026-09-22,
	// TODO 19, decision #141). Nought - the default - is a world with no
	// masters in it, and then nothing in boss.go so much as counts a body.
	//
	// It is one number and there is no gate anywhere near it. What makes that
	// safe is the ceiling the world already has: nine genes at MaxAbility is
	// a budget of 900, so the hardest body this world can hold is 1.73 of the
	// mean beast and 2.16 of the mean body. Asking for three times or ten
	// times gives the same creature as asking for twice. A master is a hard
	// fight by construction and cannot become a wall by a figure being
	// mistyped.
	//
	// Counted before it was built: humans have a hand in 0.95 of the beasts
	// that are killed, 29.75 of them a run, in parties averaging 1.70. A
	// master has to be a thing one or two bodies can bring down, because
	// nothing bigger than that gathers in this world.
	BossBudget float64

	// BossRouseTicks is how long a master stays out before it will go back
	// in, and NestQuietYears how long its nest sends nobody out after losing
	// it. NestQuietMap paints the second per cell, in fifths of it.
	//
	// Going back in wants both a time and a place: this many ticks gone by,
	// and the body standing on its own nest. Neither is a state of the body,
	// which is deliberate - "it turns back when it is hurt" or "when the
	// player is far away" would be the first behavioural threshold in this
	// project, and the reason a master walks home at all is the price of
	// being away from one (EnemyHomeCost, stage 64), which is a cost and not
	// a leash.
	//
	// Five years is the default because the thing it is measured against is
	// the dynasty's own clock: holding the goal blocks takes a thousand ticks
	// of settled living, which is two years.
	BossRouseTicks int
	NestQuietYears float64
	NestQuietMap   []string

	// HumanNests are the places people come into the world, one row each, and
	// HumanNestMap is where they are - one character a cell, each character
	// being a row's Key (2026-09-22, TODO 20, decision #142). Both empty is
	// every world before this one: people come from a birth and from the
	// first tick's InitialPopulation, and from nowhere else.
	//
	// The row carries the figures and the map carries the name and the place
	// (#133), which is the arrangement the beasts' sorts already have.
	//
	// What it is not is a second way of being born. Nothing here touches
	// birth: no parents, no vitality paid, no cooldown. The world lets a grown
	// body in, exactly as it does with a beast - so this is on
	// EnemySpawnTicks' shelf and not on Endow's, which is only ever called
	// from outside the engine.
	//
	// It is also the first time people in this world come from anywhere but a
	// birth, so a run with a nest in it is not comparable body for body with
	// one without. An arm that uses one says so in its name.
	HumanNests   []HumanNest
	HumanNestMap []string

	// HumanNestTicks and HumanNestLife are what a nest gets when its row says
	// nothing: one body a year, for ten years. The figures are here rather
	// than in the row for the reason every other table in this Config has a
	// world-level fallback - a row that only wants to name a place should not
	// have to carry numbers it does not care about.
	HumanNestTicks int
	HumanNestLife  int

	// EnemyAmbientShare is how many of the arrivals ignore the painted nests
	// and come in from wherever the world would otherwise have put them
	// (2026-09-22, TODO 18). Zero - the default - is every world since the
	// nests were painted: a map with nests on it receives every arrival
	// through them.
	//
	// It exists for the world where every nest is quiet or full. Arrivals
	// from outside the map are the reason a country cannot be emptied of
	// beasts for good by clearing it once, and a map that wants that
	// guarantee turns this up; a map that wants its nests to be the whole
	// story leaves it at nought.
	EnemyAmbientShare float64

	// HumanHomeCost is the same for a person (TODO 6, #130), and zero - the
	// default - is every world before it.
	//
	// Stage 64 gave a home range to enemies only, because the question then
	// was why a beast does not wander off the map it belongs to. The question
	// the dynasty asks is the opposite and it is about people: measured, an
	// ordinary life already crosses seven of the twelve blocks, so "travel to
	// a country nobody of your line has been to" is what every body does
	// without being asked, and a player doing it is indistinguishable from a
	// player doing nothing. A pull towards where you were born is what makes
	// leaving a decision.
	//
	// It is charged exactly as the enemies' is - per region's width, per tick
	// out there, scored against everything else - so it is a cost and not a
	// leash, and no threshold comes with it.
	HumanHomeCost float64

	// EnemyHomeCost is what being far from where it came into the world costs
	// an enemy (stage 64), per region's width of distance and per tick spent
	// out there. Zero is the world before this rule, and the default.
	//
	// It is a cost and not a leash. The rearing radius pulls a child back by
	// hand, which is fine for a child that has no say in anything, but an
	// enemy runs the same decision engine as everybody else: a hard edge
	// would be exactly the sort of threshold this world refuses to write, and
	// it would show as bodies turning round on the spot at a border (the
	// problem stage 13 avoided when the sight went to a grid).
	//
	// So it is charged the way the water is (stage 34): so much per tick,
	// worse the further out, and the option of heading back is scored against
	// everything else. A short chase past the border is affordable and a long
	// one stops being worth it.
	EnemyHomeCost float64

	// EnemySpread is how much the regions differ in how many of the world's
	// enemies turn up in them (stage 58). Each region draws a weight from
	// 1 +/- spread and arrivals are shared out in proportion, exactly as the
	// plants are (FoodSpread). Zero keeps the old uniform draw down to the
	// number of values taken from the random source.
	//
	// It changes where they arrive and never how many: the world puts in
	// exactly as many enemies as it did, so this is the map's own dangerous
	// country rather than a harder world (stage 15a's rule, and the test that
	// pins it).
	//
	// Only enemies are placed this way. Humans are not put into the world
	// after it starts - they are born into it - so there is nothing to weight.
	//
	// It is deliberately not called danger: that word is taken by what an
	// agent believes about a region killing it (regionlore.go, stage 35), and
	// the two must not be confused. This one is a fact about where enemies
	// come from and nothing an agent is ever told.
	EnemySpread float64

	// RegionAbilitySpread is how much the regions differ in what a body
	// standing in them can do (stage 57): each draws a multiplier on every
	// expressed gene from 1 +/- spread. Zero makes every region ordinary and
	// takes nothing from the random source, like the two spreads above.
	//
	// What it is for. Five skills were tried as a reason to stay somewhere and
	// none of them was one, because a skill is carried: it goes on working
	// wherever its holder walks, so it can make a body better without making
	// one place better than another. This is the first thing the ground gives
	// that is lost by leaving.
	//
	// What it does not touch is what a body holds rather than how well it does
	// something - MaxVitality, MaxSpeed, the memory's room and the hands (see
	// Agent.capacity). A cap that moved with the ground would mean a body
	// whose vitality is over its own maximum after one step across a border,
	// and speed is kept out for the older reason too: terrain was deliberately
	// put on the cost of moving rather than on the speed of it (stage 20), so
	// that effort and the ground are not counted twice.
	RegionAbilitySpread float64

	// RegionFavourSpread is the second try at the same question (stage 57c),
	// and the answer to why the first one did nothing.
	//
	// The scalar above is common to everybody standing in a region, and every
	// comparison that decides anything in this world is with somebody standing
	// in the same region: every pair close enough to fight is in one, and 97%
	// of pairs within the clustering distance are. A factor on both sides of a
	// fight cancels, so a third of a body's strength can hang on the ground
	// and nothing happens.
	//
	// This one does not cancel, because it is not the same for everybody: each
	// region draws a multiplier per gene from 1 +/- spread and the nine are
	// then scaled so that their mean is exactly one. No region is better than
	// another - a region suits some builds and not others, and two neighbours
	// with different builds get different factors.
	//
	// Zero takes nothing from the random source, like every other spread here.
	RegionFavourSpread float64

	// RegionAbilityCarried is the control the stage turns on: the same
	// multiplier, drawn from the ground a body was born on, and then carried
	// for life wherever it goes.
	//
	// That is exactly what the five skills were - something the ground gives
	// and the body keeps - so this arm has the variation without the reason to
	// stay. It draws from the random source in the same places and the same
	// order as the arm it controls, so the two run on the same stream and
	// differ in one thing only: whether leaving costs anything.
	RegionAbilityCarried bool

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
	MeatPerBudget  float64 // budget per item of meat a carcass leaves
	MeatClaimTicks int     // how long the carcass belongs to those who killed it

	// MeatFromKills leaves a carcass only where something brought the body
	// down. A body that starved, wore out or was taken by the river leaves
	// nothing to eat. On since 2026-09-22, and it moved the baseline.
	//
	// It went in because a beast lying there as dinner after dying of old
	// age is odd to look at, and it was measured because it is a rule about
	// the food supply: this world had fed on every death since stage 11, and
	// the beasts eat nothing else. Every prediction written down before the
	// run was wrong, and wrong the same way (12 seeds x 200000 ticks):
	//
	//	pop         161 -> 215 (+53 *)     enemies   41 -> 57 (+16 **)
	//	starved     -507 **                collapsed 0.08 -> 0.00 *
	//	meatDropped +594, not significant  spoiled   0.21 -> 0.03 ***
	//
	// The supply did not fall at all. What fell was waste. Meat left by a
	// quiet death lies wherever that body happened to be, which is nowhere
	// in particular, and a fifth of it rotted untouched; meat left by a kill
	// lies where the killer is already standing, and 0.97 of it is eaten
	// against 0.78 before. The same meat, in the mouths it was meant for,
	// feeds a bigger and steadier world - and the world it feeds leaves more
	// kills, which is where the rest of the supply came back from.
	//
	// The arm that puts the old world back is "meatalways".
	MeatFromKills   bool
	HuntCreditTicks int // how recently a blow must have landed to count as taking part

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

	// SpecialtyShare is how much of what a region grows is the awkward crop
	// of stage 44, and SpecialtySpread how unevenly that is spread over the
	// regions. Both zero by default: an ordinary world grows one sort of
	// plant and everybody can pick it.
	//
	// SpecialtyShare is also the ceiling on everything this stage can do.
	// Stage 38 closed with "a skill is never worth more than the rule it
	// cancels", so the share of the crop that needs knowing is written down
	// as a figure rather than left to emerge: it is the target, and it can be
	// read straight off the config.
	SpecialtyShare  float64
	SpecialtySpread float64

	// SpecialtyCatch is the chance a body that has never learned the trick
	// gets one out of the ground anyway; skill lifts it towards certainty.
	// One is a crop nobody has to know anything about.
	//
	// It is never zero, and that is deliberate: a skill that switched a food
	// on and off would be a discrete gate on a continuous gene, which is the
	// trap of StrategyDepthUnlock and of the discrete-sight proposal.
	SpecialtyCatch float64

	// SkillThrowRelief scales how much of the distance a good throw takes
	// back (stage 47). Zero is the control: the skill is learned, takes the
	// room, and buys nothing.
	//
	// The size of what it can buy is written down rather than left to be
	// discovered, the way stage 44's was: at the world's figures a throw from
	// arm's length lands nine times in ten and the measured rate over all
	// throws is 0.64, so distance is costing about twenty-six points of
	// accuracy and that is the whole of what this skill can win back.
	SkillThrowRelief float64

	// SkillHarvestRelief scales what knowing the trick is worth, the same way
	// the other reliefs do. Zero is the control: the skill is learned, takes
	// the room, and buys nothing.
	SkillHarvestRelief float64

	// --- money (stage 51) ---

	// Coins is how many are scattered over the world when it is built, and
	// zero is a world with no money in it. They are laid down uniformly and
	// belong to no region on purpose (#73): money in one corner of the map is
	// one more reason to be somewhere that is not food, and four measurements
	// say food is the only thing that moves anybody.
	//
	// They are conserved. Nothing spawns them afterwards and nothing consumes
	// them: a coin changes hands, is dropped when its holder dies, and is
	// still there.
	Coins int

	// CoinValue is the discount on what a coin will buy: a meal at the moment
	// this body runs short, times this (#73). Zero takes the value out and
	// leaves the coins lying there, which is the placebo arm.
	//
	// This one figure decides the stage, which is what the plan predicted it
	// would. At exactly one a coin is worth precisely the meal it claims and
	// neither side of a sale gains anything by making it. Above one money is
	// worth more than what it buys - hoarding for its own sake, a third goal
	// by the back door, which #73 rules out. Below one every purchase is
	// worth making to the buyer and none is worth making to the seller,
	// except for the one thing left over: a coin weighs nothing (#66), and in
	// a world whose only cost of holding a thing is its weight, that is what
	// liquidity is.
	//
	// What saves it from being a knife edge is that the coin is valued by
	// each side in its own terms. A meal is worth much more to a hungry body
	// than to a fed one, so the same coin is worth more to the buyer than to
	// the seller, and a sale between the two is not zero-sum. That is the
	// oldest reason for trade there is, and it is the only one in here.
	CoinValue float64

	// CoinPricedCertain puts back the world in which money was scored as a
	// sure thing (stage 51 as it was measured, found in stage 67).
	//
	// Picking a coin up was given a chance of one and no race, and it was
	// charged no weight because it has none. A berry on the same ground was
	// scored at CarryValue x the meal, times the odds of getting there first,
	// less what lugging it costs - so a coin paid one discount where two
	// conditions have to hold (this body running short, and somebody willing
	// to sell when it does), and beat an identical meal seven times out of
	// ten. What that bought was hands full of money and empty of dinner, and
	// with them went the giving, the cooking and the caches.
	//
	// True is that world, kept because every figure recorded for stages 51
	// and 68 was measured in it.
	CoinPricedCertain bool

	// Dropping is whether there is a word for putting something down (stage
	// 70). False is every world measured before it, and in those worlds the
	// word is never offered and the arithmetic behind it is never run.
	//
	// Stage 40 left it out on purpose and was right to: a body can eat what
	// it is holding, so a hand full of food is not a hand that is stuck. What
	// changed is that three things which cannot be eaten have gone into hands
	// since - a stone, a coin, a book - and a hand holding one of those stays
	// full until the thing is sold or its owner dies. Stage 51a measured the
	// consequence: money in two thirds of the hands in the world, and the
	// giving, the cooking and the caching down with it.
	Dropping bool

	// CarrySlotted is whether a hand is a slot as well as a weight (stage 71).
	//
	// True is every world measured before it: a body may hold carrySlots
	// things and no more, whatever they weigh. False leaves only the price
	// #66 argued for - weight, continuous, out of the vitality gene - and
	// lets a body hold what it is willing to carry.
	//
	// The gate was never argued for anywhere. canCarryMore defends the floor
	// (anything with hands can hold one thing, so that a gene is not a
	// discrete gate) and says nothing about the ceiling, and the one sweep
	// that supports it - capacity 2 costing 5.73 population - was measured on
	// 2026-09-09, in the world where what a held thing is worth came out with
	// the wrong sign. Not one of 138,310 carry options scored in that world
	// had a positive value, so picking things up happened only by
	// misjudgement, and more room to misjudge in is naturally worse.
	//
	// Against it there are three stages: money filling two thirds of the
	// hands in the world and stopping the giving and the cooking (67), books
	// doing the same (69), and a word for putting things down that costs the
	// economy and never touches the money anyway (70).
	//
	// Counted before it was changed: the mean number of slots in this world
	// is exactly 1.00 and no body has ever held two - CarryCapacity = 1 needs
	// a vitality gene of 101 for a second slot. The sentence "the gene buys
	// the second slot" has never once been true.
	CarrySlotted bool

	// Trinkets is whether this world has anything in it that is wanted for
	// itself (stage 82). Off by default: the map's author puts them in, the
	// same standing stones, fish, caches and money are on.
	//
	// It is the one deliberate exception to the rule that every want here is
	// instrumental (#73, #110), and it is made rather than scattered, priced
	// per piece rather than per kind, and it goes off. See trinket.go.
	Trinkets bool

	// TrinketValue is what a perfect, fresh one is worth, as a share of a
	// life. It is the whole of the want: there is nothing behind it to work
	// out.
	//
	// The scale to set it against is what a coin is worth (about a fifth of a
	// life at the default) and what a stone is worth (a thousandth). Too low
	// and nobody ever chooses to make one, so there is no supply to trade;
	// too high and a body spends its life making ornaments, which is the
	// shape of a third goal that outranks eating.
	TrinketValue float64

	// CraftTicks is how long making one takes and CraftVitality what it
	// costs, charged once when the thing appears. Both are the price stage
	// 17b's lesson asks for: a benefit with no price runs to the ceiling.
	CraftTicks    int
	CraftVitality float64

	// TrinketSpoilTicks is how long one lasts, on the clock meat already uses
	// (#77 - a hand does not stop it). Zero for one that never goes off.
	//
	// What it buys is a ceiling on holding: a body that keeps one for ever
	// has to be wrong about something, and this is what makes it wrong. It
	// also means what it is worth falls continuously rather than at a
	// deadline, so passing one on while it is still worth something is an
	// ordinary comparison.
	TrinketSpoilTicks int

	// TrinketSpread is how much one piece differs from the next, either way,
	// as a share of what its maker would average. Zero makes every piece by
	// the same hand identical, which is the arm that says whether a scattered
	// price came from the pieces or from the bodies.
	TrinketSpread float64

	// TrinketsVanish is the control for what the making actually buys: the
	// body spends the time and the vitality and ends up with nothing.
	//
	// It is the shape stage 49's deaf cry has - the same time spent, the
	// thing itself gone - and it is here because a want that is satisfied by
	// standing still is a reason not to wander, and not wandering is worth
	// something in this world quite apart from what is being made.
	TrinketsVanish bool

	// TrinketTaste is how differently two bodies want the same piece (stage
	// 84). Zero is the world stage 82 measured, where a trinket is worth what
	// it is worth to anybody; at one, a piece on the far side of the circle
	// from a body's own taste is worth nothing to it and one on its own is
	// worth twice.
	//
	// One by default, on the standing ChronotypeSpread is on: bodies differing
	// from each other is what this world assumes, and a population that all
	// wants the same thing is the special case. It is only ever read where a
	// map's author has put ornaments in, so no world that had none is changed
	// by it (2026-09-15).
	//
	// It moves want about rather than adding any: the multiplier averages one
	// over a piece drawn at random, so the world holds the same amount of
	// wanting however far this is turned up. That is stage 15a's rule for
	// food applied to a want - change the distribution, not the total - and
	// it is what makes the dose readable.
	//
	// It is the first figure in this world that depends on who is holding the
	// object rather than on what the object is. That is the whole of what it
	// is for: two bodies that cannot disagree about a thing have no reason to
	// trade it, which is the wall stage 79 wrote down in arithmetic (a seller
	// is short by (1 - CoinValue) on every sale, because both sides price the
	// same thing the same way).
	TrinketTaste float64

	// TrinketTasteMutation is how far a child's taste drifts from the parent
	// it took it from, on that circle. The same standing ChronotypeMutation
	// is on, and wrapped rather than clamped for the same reason.
	TrinketTasteMutation float64

	// TrinketStyleAimed is the control that says whether a market is possible
	// at all: with it on, a maker turns out the very thing it likes, so
	// nobody needs anybody.
	//
	// It is the self-sufficiency of the food economy (#100) in miniature and
	// under a switch. Off, a piece comes out how it comes out, which is the
	// one shortage in this world that neither walking nor working can fix:
	// you cannot go anywhere for the style you want and you cannot aim at it
	// when you make one.
	TrinketStyleAimed bool

	// AdornNeedsSurvival is whether an ornament is only wanted by a body that
	// expects to be there for it (stage 84).
	//
	// It is the distinction between a meal and an ornament, and it is not a
	// new one: staying alive is priced as a change in the chance of dying, so
	// it grows as a body runs out, while a want with nothing behind it was
	// priced as a constant and therefore won exactly when it should lose.
	// That is what stage 82 measured at three times the dose - a world where
	// a quarter of all decisions were making ornaments - and it is the same
	// mistake stage 73 found in the price of a child.
	//
	// So an ornament goes through survives(), like a child: what is left of
	// it after the chance of not being there. It is on the worth rather than
	// on the goal's chance because parting with one has to read the same
	// figure (stage 77).
	//
	// On by default (2026-09-15). It is a correction of the same class as
	// GoalsNeedSurvival, which has been on since stage 73, and it is what
	// makes a want worth having affordable: at three times the dose it turns
	// a world that lost a third of its population into one that loses none.
	AdornNeedsSurvival bool

	// ClimateMap is the map's picture of its weather, laid over the world at
	// its own grain (stage 85, #135). Empty is the world as it was, which is
	// the default: a world that says nothing about its weather has none.
	//
	// One line per row and one rune per cell, the way TerrainMap is written -
	// '.' for the ordinary world, '1'-'9' for that much cold, 'a'-'i' for that
	// much heat - and the size of a cell comes from the picture, so a three by
	// three picture is three by three blocks of weather and a forty by thirty
	// one is forty by thirty.
	//
	// It is a separate picture from TerrainMap on purpose. The ground and the
	// weather are two different things about a place - a cold river and a
	// warm one are both rivers - and stage 22 already settled that the terrain
	// and the regions are separate maps for the same reason.
	//
	// Until #135 it was read at the middle of each region instead, which tied
	// how finely an author could draw the weather to how many regions the
	// world was cut into - and the region count is a difficulty dial, so an
	// easy first map could hold three temperatures. Nothing learns or passes
	// on the weather (stage 86 measured that the three beliefs a body keeps
	// about a region are richness, going and danger), so the region had no
	// claim on it.
	ClimateMap []string

	// ChillAheadSeen is whether a body reads the weather one cell along the
	// way it reads the ground (#135). True in the ordinary world, and nought
	// either way in a world with no weather in it.
	//
	// It is the half of #135 that can move anybody. Stage 86 found that the
	// cold takes more vitality than hunger does and moves nobody, and named
	// the reason: what a body reads is the cold underfoot, so it lifts every
	// candidate alike and cancels out of the comparison. Stage 100 reads the
	// cell ahead and is the one rule in this project that moved where bodies
	// live. This puts the weather on that path - the difference between the
	// weather here and the weather a step away, charged over the ticks the
	// option takes, in the same place and the same unit as stage 99's drain.
	//
	// Off is the control: the same weather, taking exactly as much, read only
	// where the body already is.
	ChillAheadSeen bool

	// WardShare is how many of the ornaments a body makes come out answering
	// the weather, and WardStrength how much of it a perfect one keeps off
	// (stage 87a). Zero is the world stage 86 measured, which is the default.
	//
	// One is enough: what a body wards is the best single thing it holds, not
	// the sum of them (see wardsOff). That is stage 81's shape - a max over
	// branches, no weights, each capping itself - and it is the only one that
	// does not run to a ceiling (stage 17b). It also means the second coat is
	// worth nothing to its owner and everything to somebody in the cold,
	// which is the asymmetry stage 69 found in a read book and the one thing
	// that makes a seller part with something willingly.
	//
	// What a warm thing is worth is not given here. It comes out of the
	// survival gradient, like a meal: the drain it takes off, read through
	// the same chance of dying everything else in this world is priced by. So
	// it is worth a great deal in the cold, nothing at all in the warm, and
	// nothing to a body that already has one - and none of those three needed
	// a rule.
	WardShare, WardStrength float64

	// GiftWorthScaled is whether the goodwill a gift earns is scaled by what
	// the thing was worth to whoever received it (TODO 12, stage 89b, #119).
	// False is every world before this, where a gift buys the same goodwill
	// whatever it was and whoever got it.
	//
	// The giver cannot aim at this. It reckons on the standard, the way the
	// legs of stage 49 and the price of stage 80 do, so what is rewarded is
	// the outcome rather than the intent - which is the only shape this
	// world has ever managed to learn anything from.
	//
	// The scale is what a coin claims (CoinValue x LifeValue), because that
	// is the only standard here and inventing a second one for generosity
	// is exactly the sort of number this project does not add.
	GiftWorthScaled bool

	// TrinketFancyTicks is how often a body draws a new "what I like just
	// now", and TrinketFancySpread how far that strays from the taste it
	// inherited (TODO 12, stage 90). Zero is every world before this, where
	// a body's taste is what it was born with and never moves.
	//
	// Two things kept apart on purpose. The inherited taste stays exactly
	// what it was - a disposition, on the circle, carried whole from one
	// parent with a drift (stage 84) - and the fancy is drawn from it. So
	// nothing about heredity changes and the fancy costs no budget: it is
	// the difference between what a body is like and what it happens to
	// want this week.
	//
	// The interval is the whole of the risk. A body decides every 14.5
	// ticks, and 2026-09-04 measured what happens when something is drawn
	// afresh every time a thing is seen: the population nearly halved,
	// because the judgement noise is bigger than the gap between the top
	// two options and a body that re-draws faster than it can carry
	// anything out does nothing at all. Counted before this was built: an
	// event-driven redraw (a piece made, a piece lost) would fire about six
	// times in a life, one every 400 ticks or so - 27 decisions apart, well
	// clear of that. A tick figure below about a hundred is asking for the
	// oscillation back, and the arm that does it is in cmd/experiment.
	TrinketFancyTicks  int
	TrinketFancySpread float64

	// AdornSatiety is how many ornaments in a hand halve the want for
	// another, AdornForgetPerTick how fast the want comes back, and
	// AdornKeepsSated the far end of it - a body satisfied once and never
	// again (TODO 12, stage 89). Zero is every world before this, where the
	// fourth piece is worth exactly what the first was.
	//
	// The shape is the diet's (stage 16) and deliberately so: one decaying
	// ledger, read where the want is read, saturating rather than linear so
	// that the twentieth of a thing is no worse than the tenth. What is
	// counted is what the body is holding rather than what it has just been
	// given, because the thing being modelled is having and not getting -
	// and that is also what makes the spoiling (TrinketSpoilTicks) bring the
	// want back, which is the one recurring demand this world has.
	//
	// Counted before it was built (TODO 12): bodies hold 2.71 ornaments each
	// (4.75 among those holding any) and 43% hold none, so there is room for
	// this to bite; and a body lives 2,475 ticks against a piece's 2,000, so
	// the want has time to come back about once in a life.
	AdornSatiety       float64
	AdornForgetPerTick float64
	AdornKeepsSated    bool

	// HidePerBudget is how much of a dead beast's budget makes one hide, and
	// WardNeedsHide whether a warm thing can be made without one (TODO 8).
	// Zero and false are the world stage 87a measured, which is the default.
	//
	// The pair is the two halves of "materials and working them". The first
	// is a supply: hides come off the beasts and off nothing else, so how
	// many there are is set by how much hunting the world does rather than
	// by how much anybody wants one. The second is what turns that supply
	// into a shortage - with it on, a piece comes out warding only where the
	// maker had a hide, and making it uses the hide up.
	//
	// Counted before it was built (TODO 8's "count first"): 59.4 beasts die
	// in a 20,000-tick run and a human was among the attackers for 22.1 of
	// them, against 3,104 coats made. So the material cuts the supply by a
	// factor of ten or more whatever the dose, and the dose is the figure
	// here rather than a constant: what is being asked is whether the
	// shortage moves the trade, and a shortage that kills everybody answers
	// nothing (coatrare took the population from 92.0 to 52.9).
	//
	// Hides are not a new kind of ownership (#72, the stone's rule). They
	// lie where the carcass fell and whoever picks one up has it; the body
	// that made the kill is standing on it, which is all the advantage it
	// needs. The claim machinery that shares out meat is about eating, and
	// nothing here eats.
	HidePerBudget float64
	WardNeedsHide bool

	// MaxHideItems is the ceiling on how many are lying about, the safety net
	// MaxMeatItems is. Hides do not go off - a skin keeps - so without a
	// ceiling a world where nobody wants them would fill up with them.
	MaxHideItems int

	// ChillGradient is whether a body can feel which way it gets warmer
	// (stage 86b). Off by default, which is the world stage 86 measured.
	//
	// Stage 86 found that the cold takes more vitality than hunger does and
	// moves nobody at all, and named the reason: what a body reads is the
	// cold underfoot, so it lifts every candidate alike and cancels out of
	// the comparison. What it did not test is whether a body that could tell
	// one direction from another would go the warmer way - the control there
	// took away the level, not a gradient, because there was no gradient to
	// take away.
	//
	// So this is a sense rather than knowledge: no map, no memory, no telling
	// anybody. It is the wind on a face - which way is colder, and by how
	// much - and it is charged the way stage 64 charges a move for where it
	// would take the body, in the same one place every option passes through.
	ChillGradient bool

	// SeasonTicks is how long the weather takes to come back round to where it
	// started (stage 87b). Zero is a world whose weather never moves, which is
	// the default.
	//
	// What turns is the picture, slid sideways: the cold end of the world
	// becomes the warm one and back again. A winter that deepened everywhere
	// at once would give every body the same want at the same time, which is
	// one demand and no trade; a weather that travels puts the want where the
	// things that answer it are not, over and over (#117).
	//
	// The scale to set it against is TicksPerYear (500) and a life (about six
	// of those): a season shorter than a body's growing is weather, and one
	// longer than its life is a different world it will never see.
	SeasonTicks int

	// HeatDrain is the same for the second weather (stage 88): what the
	// hottest place in the world costs a body per tick. Zero by default, and
	// a map that draws no heat has none either way.
	HeatDrain float64

	// ChillKnown is whether a body can feel the weather it is standing in
	// (stage 86). True in the ordinary world.
	//
	// It is the control stage 34 used for drowning and it answers the same
	// question: whether what a rule does to a population is selection or
	// choice. With it off the cold takes exactly as much vitality as before -
	// the body simply does not read it in its own state, so nothing it
	// decides can be about it. If the population leaves the cold either way,
	// it is leaving because the ones who stayed died.
	ChillKnown bool

	// ChillDrain is what the coldest place in the world costs a body in
	// vitality per tick, before anything that answers it. Zero is the world
	// as it was, which is the default.
	//
	// The scale to set it against is HungerRate's drain: cold that costs more
	// than starving does is a place nothing can live in, and this world's one
	// rule that must not be broken is that there is always a way back.
	ChillDrain float64

	// SaleAnchor is how far towards what it paid a body holds out when asked
	// to sell something it bought (stage 83, #111).
	//
	// It is the disposition effect, and it is the first thing in this world
	// that is a bias rather than a memory of something real: everything else
	// a body carries is either true or its own preference, and this is a body
	// refusing an offer it would take if the same object had cost it nothing.
	// The line is crossed deliberately (the precedent is stage 54's mood,
	// which tilts a preference rather than a fact, and is also off by
	// default).
	//
	// What washes it out is being in trouble: it is scaled by the chance this
	// body has of still being here at the end of its planning window, so a
	// whole body holds out for what it paid and a starving one takes what it
	// can get. That is where the real effect gets its shape - a price that is
	// sticky while nobody is desperate.
	//
	// Zero by default, and it was measured to fire zero times: counted before
	// it was built and again after, a thing that was bought is never once
	// offered for sale again (resaleAsked = 0.00, up to the largest market
	// these rules allow). It is kept because the count is worth reproducing,
	// and because the condition for it to matter is now a measurable one.
	SaleAnchor float64

	// CoinBuysWarding is whether a coin is a claim on something that answers
	// the weather as well as on a meal and on mending (#116, stage 88).
	//
	// It is stage 81's step taken once more, and for the reason that stage's
	// measurement left behind: a coin is worth CoinValue times what it would
	// buy, and what it would buy is priced off this body's own gradient - so
	// to a body that is not short of a meal it is worth nothing at all, and
	// nine sellers in ten therefore have a floor of zero (counted in stage
	// 84). A body standing in the cold is short of something whether or not
	// it is hungry, so with more than one thing to claim the claim is almost
	// never nothing.
	//
	// No weights, for stage 81's reason: each branch caps itself. A whole
	// body gets nothing from the mending branch, a warm body nothing from
	// this one, and a body already wearing a coat nothing again.
	CoinBuysWarding bool

	// CoinBuysOnlyMeals puts back the world in which the only thing money
	// could buy was food (found in stage 84).
	//
	// It was never a rule: addBuy asked what was on the counter for its
	// nutrition, and a book and an ornament have none, so the option was
	// never scored for either - while the price, the seller's side and the
	// hand-over all worked. Every figure recorded for stages 51, 68, 69, 80
	// and 82 came from that world, which is why it is still reachable.
	CoinBuysOnlyMeals bool

	// HandOverCheapest is whether what a body holds out, hands over and sells
	// is the thing it can most afford to lose, rather than whatever is first
	// in its hands (stage 84).
	//
	// The principle is not new - it is written in firstForSale's own comment,
	// where it is served by a fixed order of kinds (a meal, then a book, then
	// an ornament). A fixed order was the whole of it while every value in
	// this world was per kind: two pieces of the same kind were the same
	// thing, so there was nothing to choose between them. With a taste there
	// is, and the order now says the wrong thing - a body would hand over the
	// very piece it likes because that is the one it made first.
	//
	// It also settles a disagreement that was already there: what a crier
	// holds out is carried[0] and what it sells is the first sellable thing,
	// which are not always the same item.
	HandOverCheapest bool

	// GiftPriced is whether giving something away costs what it was worth
	// (stage 84).
	//
	// It is off by default because every measurement of the giving since
	// stage 48 was taken without it, and it is here because of what those
	// measurements keep saying: the ornaments went out as gifts (+269) and
	// not as sales (-0.55), and the same is true of the food. A sale prices
	// what the seller gives up (willSell) and so does putting something down
	// (stage 70), but the gift option prices only the walk - so handing a
	// thing to somebody is free and selling it is not, and a body with
	// something to spare has no reason to hold out for a coin.
	GiftPriced bool

	// GiftSelfLookahead is how much of what a gift costs its giver the giver
	// weighs (stage 92a). Zero is the world where giving is free; one is the
	// whole of it, which is what GiftPriced does as a switch.
	//
	// The figure is the same one GiftPriced subtracts - what the thing in the
	// hand is worth to its holder, which for food, money and a coat is a
	// difference in this body's own chance of dying and so already the
	// lookahead read on a body without it. What this adds is a dial and the
	// company it is measured in: pricing the giving on its own has been run
	// twice and stopped the exchange both times (stage 84d, stage 88), so it
	// is meant to be read against GiftKinWeight rather than alone.
	//
	// What is priced is the cheapest thing in the hand, which is the thing
	// that would go where HandOverCheapest is on and not otherwise - the two
	// are meant to be run together, and the arm without it says what the
	// mismatch costs.
	GiftSelfLookahead float64

	// GiftKinWeight is what a gift to one's own parent or child is worth
	// beyond the goodwill it buys (stage 92b): Hamilton's rB, with the
	// relatedness of the only relation this world has - one hop, parent to
	// child - folded into the weight, since a constant half tells nobody
	// anything they did not already know from the flag.
	//
	// It is a second channel and not a bigger first one. The trust a gift
	// buys saturates, which is why 95% of gifts go to strangers (stage 48),
	// and that is a real force rather than a fault: raising it would fight
	// the selection already there. This adds the term that is missing
	// instead - what the gift does for whoever receives it - and lets it
	// count only where the receiver carries this body's own genes.
	//
	// What the receiver gets is reckoned on the standard, not looked up: a
	// stranger's hunger is hidden (only its vitality shows), so the giver
	// prices the thing as it would price it for itself and scales it by how
	// far down the receiver looks. Nobody aims; the world pays out.
	GiftKinWeight float64

	// SkillTrinketRelief is how much of what a piece could be worth is the
	// maker's skill rather than anybody's hands. At zero anybody makes a
	// perfect one; at one a body with no skill makes nothing worth having.
	SkillTrinketRelief float64

	// CoinPrices is whether a sale has a price in coins rather than being one
	// coin for one thing (stage 80).
	//
	// Until stage 80a there was nothing for a price to be: the seller wants
	// 1/CoinValue coins for an ordinary meal - two, at the default - and a
	// buyer could hold one. Counted over 691 opportunities, the share where a
	// whole price fits between the two sides' limits AND the buyer holds that
	// many was 0.0159, which is exactly the set that trades at a fixed price
	// of one. With 80a on it is 0.1571.
	//
	// The price is settled at the counter, where willSell has always been
	// asked, and out of figures that already exist: the seller's floor is
	// willSell rearranged, the buyer's ceiling is addBuy rearranged. Nothing
	// is advertised and neither side learns anything about the other - a
	// buyer walking over reckons on the standard price (standardPrice), the
	// same way it reckons a rival's speed at the world's standard speed.
	CoinPrices bool

	// CoinPriceBlind is the control for the reckoning half of stage 80: the
	// sale is priced, but a buyer sets out as though it would pay one coin.
	//
	// The stage does two things at once - sellers can be paid more, and
	// buyers stop walking to sellers they cannot pay - and this takes the
	// second away while leaving the first. It is the shape stage 49's deaf
	// control had: the same time spent, the information gone.
	CoinPriceBlind bool

	// SalePriceSplit is whether the price is the middle of the range rather
	// than the least the seller would take (stage 80).
	//
	// Coins do not divide, so the rounding is the sharing: the cheapest whole
	// price that clears the seller's floor leaves the rest of the surplus
	// with the buyer, and the middle splits it. Counted before it was
	// written, this decides very little - the median number of whole prices
	// that fit between the two limits is one.
	//
	// On by default since 2026-09-15, because in a world with something worth
	// haggling over in it, it decides the one thing that made stage 80 worth
	// doing: with the price at the seller's floor, nine sellers in ten put
	// nothing on money at all, so the floor is nought and the price is one
	// coin whatever the buyer would have paid. Sharing the surplus is the
	// whole of what makes a price a variable with more than one value (the
	// share of sales going for more than one coin, 0.15 -> 0.40), and it
	// costs nothing measurable. It is only read where prices are on.
	SalePriceSplit bool

	// CarrySlotsWeigh is whether a hand is taken up only by what weighs
	// something (stage 80a).
	//
	// Holding something is charged twice here: by its weight, which is what
	// #66 argued for and which comes continuously out of the vitality gene,
	// and by the slot, which is discrete and which stage 71 could find no
	// argument for anywhere. A coin weighs nothing (#66) and a book weighs
	// nothing, so the slot is the only thing charging them - and what it
	// charges them is the scarcest thing in this world, the hand that would
	// otherwise hold dinner.
	//
	// With this on, a thing with no weight takes no hand. It is not a rule
	// about money: what counts as weightless is exactly the set the weight
	// itself leaves out, so a coin and a book go the same way because they
	// weigh the same nothing. A meal still takes a hand and still costs what
	// it weighs, so nothing here lets a body carry more food.
	//
	// It is not CarrySlotted = false, which lets a body hold any number of
	// meals: that was measured in stage 76 at rareTrough -0.16 *** in a map
	// with no money in it at all, which is to say the price of it was the
	// carrying of food. This leaves that price where it was.
	//
	// Counted before it was written (played map, six runs): a buyer holds
	// 1.000 coins and never two, while the world has 2.36 coins per human
	// and 38 of its 60 lying on the ground - so what stops a second coin is
	// the hand and not the supply.
	CarrySlotsWeigh bool

	// CarryDiminishes is whether the second thing in a hand is worth less
	// than the first (stage 71).
	//
	// What a thing kept for later is worth is the meal it would be at the
	// moment this body runs short - and a body only runs short once inside a
	// horizon. With this on, what is already in hand is taken off the hunger
	// that moment is reckoned at, so the second meal is worth less than the
	// first and a body holding its dinner puts almost nothing on a coin. That
	// is the same crowding stage 70 tried to buy with a word and could not.
	//
	// What counts as already in hand includes money, at CoinValue a coin,
	// because that is exactly what a coin is priced as: a claim on a meal.
	CarryDiminishes bool

	// BurdenIgnoresWeightless is whether reckoning what one more item would
	// cost to carry leaves out the things that weigh nothing (stage 71).
	//
	// True is right and is the default. burdenWith counted everything in the
	// hand while the weight actually charged leaves out coins and books, so a
	// body holding a coin was over-charged for picking up a berry. False puts
	// the old arithmetic back.
	BurdenIgnoresWeightless bool

	// CarryPricedBackwards puts back the world in which what a held item was
	// worth came out the wrong way round (stage 40, found in stage 50).
	//
	// What a thing kept for later is worth was written as the difference
	// between how this body stands now and how it will stand when it runs
	// short - which is a loss, not a gain - where what was meant was the good
	// that eating it then would do. Measured before the fix: of 138,310 carry
	// options scored in one run of the default world, not one had a positive
	// value. Every figure recorded for stages 40 to 49 was measured in that
	// world, which is why the way back is kept.
	CarryPricedBackwards bool

	// --- cooking (stage 52) ---

	// EatTicks is how long a body stands at a meal before it has it, and
	// EatTicksKnown is whether it knows that when it is choosing. Zero is
	// every world before this: a meal reached is a meal had, in the tick it
	// was reached.
	//
	// It is the occupancy that was left out when the words were written. The
	// price is time and only time, in the shape the call, the cry, the
	// cooking and the writing already use (#76) - the same actionTicks, and
	// no new parallel machinery. An interrupted meal is time spent for
	// nothing, exactly as an interrupted cry is, and nothing here stops a
	// body abandoning one: being hit already asks it to think again, so what
	// happens at a meal that turns dangerous comes out of the ordinary
	// comparison rather than out of a rule about meals.
	//
	// The time is spent at the food and not on the way to it, which is why
	// the counter is held at nought while the body is still walking. It was
	// counted before this was built: only 0.13% of a life is spent standing
	// at a meal, against 10.8% walking to one, so occupancy is small however
	// it is dosed - but eating is scored in 49.8% of decisions, so what the
	// extra time does to the comparison is not small.
	//
	// EatTicksKnown is the control in the shape stage 34 and stage 86 both
	// needed: the world holds the body either way, and turning it off leaves
	// the utility pricing a meal at one tick. Without it there is no telling
	// whether what a measurement shows is bodies choosing differently or
	// simply being held still.
	EatTicks      int
	EatTicksKnown bool

	// CookTicks is how long a body stands there preparing what is in its
	// hand. Zero takes the word out of the world: nothing is scored and
	// nothing is ever cooked.
	//
	// The price is time and only time, in the shape stage 32's call and stage
	// 49's cry already use (#76). No new parallel machinery, and nothing is
	// half-cooked: an interrupted cooking is time spent for nothing, exactly
	// as an interrupted cry is.
	CookTicks int

	// CookVitality is what one cooked item mends, as a share of the eater's
	// own ceiling - the same units MeatVitality is in, and deliberately the
	// same figure by default.
	//
	// Equal figures mean cooking a carcass buys nothing, because the two do
	// not stack (cook.go). That is not an oversight: it puts the cooking on
	// the food everybody has instead of the food a hunting party has, which
	// is the only kind an exchange could be built on.
	//
	// Zero is the placebo arm. The word still exists and still costs the
	// vocabulary what it costs, nobody ever chooses it, and no random number
	// is drawn either way - so an arm with this at zero is the world without
	// this stage, bit for bit. Stage 50 showed how much that is worth: it is
	// what makes a difference somewhere else believable.
	CookVitality float64

	// CookQuality is how well a body that has learned nothing about it cooks,
	// from 0 to 1. One by default - anybody can cook - and lowering it is the
	// map-maker's to do, the same way stage 43 leaves the two fishing chances
	// at one until a world is laid out that wants them lower.
	//
	// It is a floor and not a gate (#80). A body that knows nothing still
	// cooks; what it does not get is the part SkillCook buys back.
	CookQuality float64

	// --- who rears, and who may feed (stage 53) ---

	// GuardianIsMother makes the mother the parent a child keeps close to.
	// False is the world before stage 53a, where it was whichever parent came
	// first and a father had the job 50.5% of the time - every figure
	// recorded before 2026-09-11 is from that world.
	//
	// It is structure rather than decoration: the whole of stage 53's gene
	// bias is derived from what being tied to a radius asks of a body, so the
	// role has to exist before the bias means anything.
	GuardianIsMother bool

	// ParentFeedByKin lets the parent that is not the guardian feed a child
	// too, as long as it is that child's parent and near enough to hand it
	// anything (stage 53b).
	//
	// It exists because stage 53a would otherwise shut fathers out of feeding
	// altogether: mouthsToFeed asked whether the eater was the guardian, and
	// with the mother always the guardian the answer for a father is never.
	// Nothing new is built for it - the same passive share() of stage 7d, the
	// same radius, no ownership and no decision.
	//
	// It is not a small repair. Counted before it was written: a guardian is
	// inside the radius 0.774 of the time and the other parent 0.475 (0.516
	// of the time it is alive at all), so this is something like 1.6 times as
	// many chances to feed anybody, and it has to be measured on its own.
	ParentFeedByKin bool

	// SexBias is how far each gene leans by sex, as a fraction: the expressed
	// value is what was inherited times one plus this for a female and one
	// minus it for a male (stage 53). Positive favours the mother.
	//
	// All zero by default, which is the world where sex does nothing but
	// decide who can pair with whom - every figure recorded before
	// 2026-09-11 - and it is deliberately measured against that arm rather
	// than made a default, because a difference put in by hand is not a
	// finding.
	//
	// The directions come from the roles of 53a and not from anywhere else
	// (#85); see sexFactor for the derivation. Vitality, evasion and looks
	// are left alone: both roles want them equally, and looks in particular
	// would touch stage 26's line that no real ability enters what a body is
	// worth as a mate.
	SexBias [NumGenes]float64

	// --- which way somebody went (stage 55a) ---

	// LonelyValue is what heading after the body this one thinks best of is
	// worth, when it has gone out of sight. Zero takes it out of the world:
	// nothing is remembered, no candidate is offered, and no random number is
	// drawn either way.
	//
	// It is scored as one more way to wander, in exactly the shape stage 15b's
	// walk to better country takes - value times how much the direction is
	// still worth, less what walking costs - so it is beaten by anything
	// better and overrides nothing.
	LonelyValue float64

	// LonelyHalfLife is how long that direction takes to be worth half as
	// much. It is what stops a body walking after somebody for the rest of its
	// life: below a twentieth the memory is dropped outright.
	LonelyHalfLife int

	// LonelyWrongWay is the control, in the shape stage 54 found works: the
	// same candidate, the same weight, the same cost, pointing the wrong way.
	// If that does as well, what the option buys is having somewhere to go
	// rather than having remembered where they went.
	LonelyWrongWay bool

	// --- how a body is feeling (stage 54) ---

	// MoodWeight is how far a mood leans what a body makes of being worn
	// down. Zero is the placebo: the two scalars stay at nothing, no lean is
	// applied, no random number is drawn, and the world is the one before
	// this stage bit for bit.
	//
	// A negative figure is the control this stage turns on, and it is the
	// cheapest one there is: the same size of lean with the sign reversed, so
	// that a frightened body grows bold. If that does as well, then what
	// matters is the size of the bias and not its structure - which is the
	// difference between a feeling and a coin toss.
	//
	// It leans a preference and not a fact. ShockRisk is one of the three
	// quantities stage 12a set aside as having no right answer in the world,
	// so a body reading it high is not wrong about anything it could check.
	MoodWeight float64

	// MoodDreadGain and MoodCheerGain are how much one blow and one mouthful
	// move the two scalars, in units of the body's own ceiling: a blow worth
	// a tenth of a body adds a tenth times the first to its dread. Both
	// scalars are capped at one, so no run of luck either way can take a body
	// past being wholly one thing.
	//
	// They are separate because the two sides of this turned out to be
	// nothing like the same size. Measured: cheer sits at 0.12 against dread
	// at 0.02, because meals are frequent and large in a body's own units
	// while blows are rare and small - so with both on, the mood is a
	// near-constant lean towards boldness rather than a signal. Setting the
	// cheer to nothing is the arm that asks stage 12a's question properly:
	// does a fear of something that is not there hold a world together?
	MoodDreadGain float64
	MoodCheerGain float64

	// MoodHalfLife is how long the lean takes to fade by half. It is the whole
	// design, and the count taken before this was built says why: the share of
	// body-samples inside so many ticks of a beating runs 0.199 (50), 0.346
	// (150), 0.466 (300), 0.585 (600), 0.673 (1200) against a ceiling of 0.713
	// for having ever been hit at all. A mood that stands two thirds of the
	// time is a constant, and a constant changes no ranking - so a long memory
	// for fear should do less than a short one, not more.
	MoodHalfLife int

	// CookSurvivesHands is whether what has been prepared stays prepared when
	// it changes hands. True, because a cooked plant is a cooked plant
	// whoever is holding it.
	//
	// False is the control arm this stage turns on, and it is the same shape
	// as stage 49's WaresSeen: the act still happens, at the same price, and
	// only what it is worth to somebody else is taken away. A difference
	// between the two arms is cooking as a trade; no difference is cooking as
	// a private act that nobody else was ever going to benefit from.
	CookSurvivesHands bool

	// --- a place to put things (stage 50) ---

	// StoreCapacity is how many items one store holds. Zero takes stores out
	// of the world altogether: none can be laid out, so no agent is offered
	// the option and nobody's hands are read for one.
	//
	// A world has no stores unless whoever lays it out puts them there
	// (SetStore), so this being non-zero changes nothing on its own - the same
	// footing the terrain and the regions are on.
	StoreCapacity int

	// MaxStores is how many a world may have. It is the bound that makes
	// #41's exemption hold: a place is not a person and takes no room from
	// what a body can remember about people, which is a fair trade only while
	// the count stays in the same order as the regions (twelve).
	MaxStores int

	// StoresKnownToAll is the control this stage is read against: the stores
	// are there and used, and every body knows where all of them are from
	// birth. It leaves the storing and takes away the knowing, the way stage
	// 49's WaresSeen leaves the crying and takes away the hearing - and stage
	// 49 is the reason it exists at all, because without that control this
	// project would have recorded a finding that the control took back.
	StoresKnownToAll bool

	// StoreFindLooks is how many looks at a cache it does not know it takes
	// for a body to notice it. Zero closes the path.
	//
	// The plan named three ways of coming to know a place - inheritance,
	// seeing somebody use it, and hearing it cried - and writing them showed
	// that none of them can start: every one of the three needs somebody who
	// already knows. This is the path that starts it, and the only one that
	// needs nobody else.
	//
	// It counts looks rather than drawing a chance on each of them, and that
	// is a measurement decision rather than a modelling one. A draw here
	// would be a draw the world without caches does not make, so the two arms
	// would run on different random streams and no small difference between
	// them could be read - which is the trap stage 45 fell into and had to
	// warn about. Counting is deterministic, so a world with caches and a
	// world without diverge only where the rule actually bites.
	StoreFindLooks int

	// What one telling is worth, down each of the three paths a place can be
	// learned (#75). They are strengths on the same record a region's
	// knowledge is kept in, so they are counted in the same units as a look
	// at the ground, and the record fades at RegionForgetPerTick - no new
	// rate is invented for forgetting a place.
	//
	// Inheriting is worth the most because a child is with its parent for the
	// whole of its rearing; seeing somebody reach into a cache is worth a
	// look; hearing it cried is worth least, for the reason every second-hand
	// figure in this world is worth less than a first-hand one. Any of them
	// at zero closes that path, which is how they are told apart.
	StoreInheritStrength float64
	StoreWitnessStrength float64
	StoreCryStrength     float64

	// StoreValue is what a body thinks a store is for: how much of a meal
	// kept for later it reckons on getting back out of one. It is the same
	// discount carrying uses (CarryValue) and it is separate from it because
	// the two are not the same bet - what is in a hand cannot be taken by
	// somebody else, and what is in a store can.
	StoreValue float64

	// OfferTicks is how long a cry lasts and how long it takes (stage 49).
	// For that many ticks the body stands there doing nothing else, and for
	// that many ticks what is in its hand is visible to everybody who can see
	// it. Zero takes the word out of the vocabulary: no agent is offered the
	// option, none is ever seen crying, and nobody's hands are read - which is
	// the arm the whole stage is measured against.
	//
	// It is the price and the reach at once, which is what makes it the one
	// figure this stage has. A longer cry costs more time and is worth more,
	// because the estimate on both sides is the same question: can whoever
	// would come get here before it stops? Past TriggerIdleTicks a cry is cut
	// short by boredom, which is a rule of the world rather than an oversight:
	// nothing here is allowed to hold a body still indefinitely.
	//
	// It is zero by default, and not because nothing happens - the word is
	// spoken, heard and walked to. It is zero because the control that takes
	// the information out and leaves the standing still (WaresSeen) is worth
	// as much: what this rule buys is not the advertisement.
	OfferTicks int

	// WaresSeen is whether a held-out item can be read by anybody looking
	// (stage 49). True is the rule; false is the control that separates what
	// the advertisement does from what standing still does. In the false arm
	// the cry is still scored, still chosen and still costs its ticks - the
	// only thing missing is that nobody can see what is being held up.
	//
	// It is the same shape of control as stage 34's DrownKnown and stage 39's
	// meat that mends without anybody knowing it does: the rule fires either
	// way, and what is taken away is the knowing.
	WaresSeen bool

	// AffinityGift is what handing something over earns, both ways (stage
	// 48). Zero takes the reason to give anything away out of the world while
	// leaving the word in it, which is the arm this stage is read against.
	//
	// It is the same quantity a shared kill earns (AffinityHunt) and it is
	// written into the same place, because being on good terms is the only
	// currency this world has: it makes resting near somebody cheap and
	// counting on them in a fight possible. Nothing new is invented to price
	// a gift with.
	AffinityGift float64

	// AffinitySale is what a sale earns, both ways (stage 68). Zero is the
	// world stage 51 measured, where a sale deliberately earned nothing: a
	// gift was held to be one-sided and a sale not a favour, and writing
	// goodwill into both would have made the measurement unreadable.
	//
	// That measurement is in, and it says the market fails on the seller's
	// side. willSell compares CoinValue x keep against max(meal, keep) less
	// what carrying costs, and since max(meal, keep) is never below keep, the
	// seller is short by (1 - CoinValue) x keep every time, with only the lug
	// to make it up. Counted on the played map before this was written: 54.3%
	// of the moments a body was holding something to sell, it would have
	// refused, and the mean shortfall then was 15.78.
	//
	// So what this opens is the seller's side, in the only currency the world
	// has. A sale is a hand-over too, and it earns what a hand-over earns.
	// The ceiling is not a free choice: trust saturates at AffinityTrust, so
	// the most this can ever be worth is LoreValue, whatever the figure here.
	AffinitySale float64

	// SaleGoodwillKept says whether the goodwill a sale earns is actually
	// written into what the two of them think of each other, or only priced
	// (stage 68). False is the control: the seller weighs the same figure and
	// sells for the same reason, and nothing reaches anybody's memory.
	//
	// It exists because being on good terms does other work in this world -
	// it is what makes lying down near somebody look cheap (stage 9) - and
	// the record already says that is a bad bargain on balance. So when the
	// population moves, this arm says whether it moved because trades happened
	// or because goodwill was written, which are two different rules wearing
	// one name. The same shape as ParentFeedWasted and DrownKnown.
	SaleGoodwillKept bool

	// --- books (stage 69) ---

	// Books is whether anything in this world can be written down. Off is
	// every world before 2026-09-13, and it draws no random numbers, so a
	// world with it off is bit for bit the world that was measured.
	Books bool

	// BookSubject is what books are about: a skill, or where the caches are.
	// Two, because the target was counted first and the obvious one has
	// nowhere to land - a body has a free hint slot 0.000 of the time.
	BookSubject BookSubject

	// WriteTicks is what setting something down costs, in time and nothing
	// else - the same shape as crying one's wares and cooking. Zero takes
	// writing out of the world while leaving the word in it.
	WriteTicks int

	// BookFidelity is how much of what its writer knows a book carries.
	// Below one because being told at second hand is weaker than seeing for
	// yourself everywhere else here, and writing is the second hand at its
	// longest.
	BookFidelity float64

	// BookValue is the discount on what reading one is worth, the same kind
	// of figure as CarryValue, StoreValue and CoinValue: what is being bought
	// is knowing, and what it is priced in is LoreValue.
	BookValue float64

	// BookSurvivesReading is the whole asymmetry. True and a read book still
	// says what it says, so it is worth nothing to the one who has read it
	// and something to everybody else - which is the first thing in this
	// world of which that is true, and the thing stage 68 found a market
	// needs. False is the control: a book that is used up is a rival good and
	// behaves like a meal.
	BookSurvivesReading bool

	// Throwing is whether a body with a stone in its hand can throw it (stage
	// 46). False, and deliberately so on the first pass.
	//
	// Everything about how this world holds together runs through the cost of
	// starting a fight, and stage 12a measured what that cost is made of: the
	// belief that one gets hit back 0.7 of the time, against a world that
	// does it 0.15 of the time. Teaching agents the truth halves the
	// population. Throwing lowers the truth further - a stone from out of
	// reach cannot be answered until the thrown-at body has crossed the gap -
	// so this is switched on in an arm, measured with TrueRetaliation printed
	// beside the population, and only then given a default.
	Throwing bool

	// ThrowRange is how far a stone carries, clamped at sight: a rule about
	// hitting what cannot be seen would break the one promise stage 19 made
	// about what may be aimed at. ThrowDamage is what one does as a multiple
	// of a tick's worth of the same body's melee, ThrowHit the chance it
	// finds its mark from arm's length, and ThrowFalloff how much of that
	// distance takes away at the far end - which is the only part of it the
	// skill of stage 47 may touch (#71).
	ThrowRange   float64
	ThrowDamage  float64
	ThrowHit     float64
	ThrowFalloff float64

	// Stones is how many are scattered over the broken ground when the world
	// is built (stage 45). Zero by default, and zero in effect on any map
	// with no rough country: where the ammunition is is the map author's to
	// say, as what grows and what swims are.
	//
	// They are laid out once and not replenished. Nothing consumes one yet -
	// what a stone is for arrives in stage 46 - so a world's supply is what
	// it was given, moved between the ground and whatever hands pick it up.
	Stones int

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

	// LandedFishKeeps puts the world back the way it was before a fish out of
	// the water was dead flesh: false is the rule (landing one starts a
	// carcass's clock on it), true is the world every figure recorded for
	// stages 42 and 43 was measured in.
	//
	// It shares MeatSpoilTicks rather than having a figure of its own. A dead
	// fish and a carcass are the same thing wearing different names, and a
	// second number would have to be told apart from the first by a
	// measurement nobody has a reason to want.
	LandedFishKeeps bool

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

	// KnockbackDist is how far a blow moves the one it lands on, in world
	// units, for a blow of AttackDamage against a body of the world's
	// reference size (knockback.go, #136). Nought is off, and off is the
	// default: it is the map author's rule as much as the world's, because
	// what it does depends entirely on what is behind the one being hit.
	//
	// The distance scales with the damage that actually landed and inversely
	// with how big the body is, so everything already priced into a blow is
	// priced into the push without being named twice.
	//
	// The figure to hold it against is CombatRadius (15): a push shorter
	// than that leaves the two in reach of each other and the fight goes on,
	// and a longer one breaks the fight off and makes the striker walk back.
	// A step is about two units, so even a short push is several ticks of
	// walking undone.
	KnockbackDist float64

	// KnockbackFall is what one level of drop takes out of a body that was
	// pushed off an edge. Held against AttackDamage (1.15): a fall worth
	// several blows turns a cliff from ground into a door, which is the
	// thing stage 20 refused to build.
	KnockbackFall float64

	// KnockbackThrown says whether a thrown stone pushes too (stage 46).
	// False by default: an arm's length blow has a body behind it and a
	// stone does not.
	KnockbackThrown bool

	// --- choosing to push (shove.go, #139) -----------------------------
	//
	// ShovePush is how far a body throws another one when it spends a tick
	// pushing instead of hitting, for a body of the world's reference size.
	// Nought is off, and off is the default: the fourth stance is not even
	// scored, so a world without the rule draws the random numbers it always
	// did.
	//
	// It is kept apart from KnockbackDist on purpose. That one is what a blow
	// does to a body whatever either of them wanted; this one is a move
	// somebody chose, priced and learnt about. Sharing a figure would have
	// meant turning on the older rule to measure the newer one, and stage
	// 13's default is off.
	//
	// The figure to hold it against is CombatRadius (15), the same as
	// KnockbackDist's: a push shorter than the slack between two bodies
	// leaves them in reach of each other and buys nothing at all, which is
	// what the belief below is for finding out.
	ShovePush float64

	// ShoveCost is what a tick of pushing at full effort takes out of the one
	// pushing, beside AttackCost (0.30) and DefenceCost (0.12). It is about
	// what swinging costs: a heave is exertion, and what the stance gives up
	// is the blow rather than the price.
	ShoveCost float64

	// ShoveWorks is what a body starts life believing about a push: how often
	// one puts the other out of reach. It is a fact in lore.go's sense - the
	// world has a right answer and a body finds it out by pushing - so this
	// is only where the finding out starts from.
	ShoveWorks float64

	// ShoveLearnBoth says both sides of a push learn from it: the one who
	// pushed, and the one who was pushed. True by default, because the event
	// is one event and both of them were there.
	//
	// It is switchable because this is the first belief in the world with two
	// doors into it, and a belief that fills twice as fast is not the same
	// belief. The arm with it false is the control that says which of the two
	// is doing the work.
	ShoveLearnBoth bool

	// ShoveLearnRate is how far one push moves that belief, and it is a
	// figure of its own rather than the world's LearningRate. Nought stops
	// everybody learning it and stops it being traded, which is the control
	// for "is it the pushing or the knowing about pushing" that this project
	// has needed five times before (stages 31, 35, 49, 100).
	//
	// It is separate because LearningRate is off by default and the reason it
	// is off is about retaliation and nothing else: the 0.7 the controller
	// assumes about being hit back is a deterrent holding the world together
	// rather than a fact, and bodies that find out the truth brawl and starve
	// (see its own comment). Nothing of the sort is true of pushing - there
	// is no figure here that is quietly doing a second job - so tying the two
	// together would have meant a rule that can only be measured inside a
	// world already known to be broken.
	ShoveLearnRate float64

	// --- learning how bodies die (lesson.go, #137) ---------------------
	//
	// LessonSlots is the most room for lessons a body may buy, and nought -
	// the default - is a world where nobody learns anything from watching
	// anybody die. It is the ceiling on the draw and on the inheritance, the
	// way HintSlots is for rules of thumb.
	//
	// The default has to be nought rather than a number, because room is paid
	// for out of the budget the genes are fitted to: a world that bought two
	// slots would be a world of slightly smaller bodies, and every figure
	// ever measured would move.
	LessonSlots int

	// LessonSlotCost is what one of those slots takes out of that budget.
	// The figure to hold it against is HintSlotCost, which buys the other
	// kind of room.
	LessonSlotCost float64

	// LessonWeight is how hard one lesson pushes against the move it is
	// about. It is a magnitude: a lesson is always a mark against, because
	// what it was learnt from is a body that stopped.
	//
	// Nought is the control this project has needed five times over: the
	// slots are still bought, the deaths are still watched, the patterns
	// still ripen and take room, and none of it reaches a decision. That is
	// "the knowing without the acting", and it is the only way to tell the
	// rule apart from the budget it costs.
	LessonWeight float64

	// LessonRipeTwice says a pattern has to be seen twice before it takes a
	// slot. True by default: one death of a kind is an accident. False
	// promotes on the first, which is the arm that says whether the second
	// sighting is what the rule is waiting for.
	LessonRipeTwice bool

	// LessonsSpread says a lesson can be copied from one body to another on
	// the back of watching somebody, the way a rule of thumb is. True by
	// default, and the arm with it false is "learnt but never taught".
	LessonsSpread bool

	// ShoveGroundSeen says a body reads the ground just behind the one in
	// front of it - the drop or the river a push would put it in. True by
	// default where the rule is on at all; false is the arm where pushing
	// works exactly as well and nobody can see what it would push anybody
	// into.
	//
	// Nothing is read and no random number is drawn unless ShovePush is set,
	// which is what keeps every world before this one consuming the random
	// source as it did.
	ShoveGroundSeen bool

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

	// Whose side a body is on (TODO 14, #138, sides.go). Five figures, all
	// nought or false in every world before them, and they are meant to be
	// switched on together: measured apart, the first three are a rule that
	// lowers a number nobody reads.
	//
	// FightTrustCost is what swinging at somebody costs in their goodwill -
	// (i), and the only one of the five that touches the utility formula.
	// Until it, affinity decided where a body rested, what it handed over and
	// who it would call in, and decided nothing at all about who it fought:
	// two bodies racing for the same plant were priced identically whether
	// they had grown up together or met a moment ago.
	//
	// It is charged as the goodwill it would lose, by the same yardstick a
	// gift earns goodwill with (trustBought), so the two sit in one term and
	// on one measuring stick (stage 77). What it must not become is a flat
	// tax on fighting, which is what AttackCost already is: the floor on
	// affinity stays at nought here, so a stranger and somebody already
	// disliked cost the same nothing, and all this rule adds is the
	// difference a friend makes.
	FightTrustCost float64

	// AffinityNegative allows a record to go below nought - (a). On its own
	// it changes nothing at all: every reader of affinity clamps at nought,
	// so a body at -10 is read exactly like a stranger. What it buys is
	// somewhere for the losses to go, and it moves three seams that would
	// otherwise work backwards, all of them in this file's neighbours: which
	// record a full memory throws out first, what a gift to somebody
	// disliked buys, and whether a loss is allowed to make room for itself.
	AffinityNegative bool

	// AffinitySnatched is what being beaten to the item you were walking
	// towards costs the one who got there first - (vi). It fires in eat, once
	// per body that had that item as its target, drawing no random numbers.
	//
	// The event is deliberately narrow: not being near somebody, not being in
	// a race with them, but having chosen that item and been on the way to
	// it. A rule keyed on proximity would lower everybody's opinion of
	// everybody wherever food was thick on the ground.
	AffinitySnatched float64

	// SnatchForgiveness is how much of the goodwill already held buys a body
	// out of taking offence at all (#141). Nought is the rule as it was first
	// written - every snatch costs the taker, whoever they are - and one makes
	// the chance of minding fall the whole way to nothing for somebody already
	// trusted completely.
	//
	//	chance of minding = 1 - SnatchForgiveness * (affinity / AffinityTrust)
	//
	// It is a coin, and this world allows one only where it is tied to
	// something (#124): here it is tied to the figure everything else about
	// trust is measured against. What it is for is what the first measurement
	// found - that being robbed grinds down the friendships that already
	// exist, because the bodies you race for food are the bodies you live next
	// to - and it answers that at the far end, by letting a friend let it go,
	// rather than at the near end, by making the body stand back.
	//
	// It draws nothing while it is nought, and nothing for a body with no
	// goodwill to forgive, so a world without it consumes the random source
	// exactly as it did.
	SnatchForgiveness float64

	// SnatchPriced says whether the goodwill a snatch would cost is also
	// charged before the race, in the deciding (#140). True is the form
	// measured first. Turning it off leaves the ledger written and nobody
	// reading it - worth having as an arm precisely because that is the shape
	// this project has now measured seven times.
	SnatchPriced bool

	// ClaimRetriggers wakes the bodies already walking towards an item when
	// somebody they are fond of sets out for it too (#142), so that the
	// weighing #140 put in the deciding happens while the race is on.
	//
	// Without it a body finds out in one of two ways, both after the fact: at
	// its next idle rethink, forty ticks later, or when the meal is gone and
	// what fires is the rule that lowers an opinion.
	//
	// It sends nothing and invents no range: the wake reaches only bodies
	// that can see the one that chose, which is the same reach the claim
	// itself has.
	ClaimRetriggers bool

	// AffinityAlly is what going in on somebody's side is worth to the two of
	// them - (iii) - and, read the other way round, what a body can expect to
	// buy by going in - (iv). One figure for both because they are one
	// quantity: what the joining is worth is what it pays.
	//
	// The machinery for (ii) has been in the world since stage 32 - a friend
	// swinging at something already raises the odds this body scores for
	// swinging at it too - so what these two add is the motive and the
	// reward, not the act.
	AffinityAlly float64

	// AllyPaidOnBlows says which of the two the payment is priced off (#140):
	// swings that landed in the same fight, or a declared intention to fight.
	// True is the blow-priced form and the default.
	//
	// The intention-priced form has two faults the count found. It pays the
	// same pair again every time either of them thinks about the fight again
	// - 13954 payments a run against joins at 3% of decisions - and it pays a
	// body that called a fight and never swung. It is kept so that the two can
	// be run against each other on the same seeds.
	AllyPaidOnBlows bool

	// AllyPaidOncePerFight settles the debt with one body rather than with
	// every body already swinging (#141): whoever's side was actually taken -
	// the one being set upon if there is one, otherwise the first into the
	// fight.
	//
	// False is the form measured first, where a brawl of four pays six pairs
	// and the minting grows with the square of who turns up. That measurement
	// is what raises the question: joining moved joinShare by 0.01, while the
	// sheer volume of goodwill minted moved fightLiked, claimLiked and
	// snatchFriend. This is the arm that tells the two apart.
	AllyPaidOncePerFight bool

	// AffinityKilledMine is what killing somebody this body was fond of costs
	// the killer in its goodwill, as a multiple of what the dead one was
	// worth to it (TODO 14, #138). Nought in every world before it, and it
	// needs AffinityNegative the way (vi) does.
	//
	// In proportion rather than flat, because what is being recorded is a
	// loss and not a judgement: killing a stranger in front of somebody costs
	// nothing here. That is also what keeps it clear of "no grudges" - a
	// fallen opinion buys no revenge, it only takes away the discount (i)
	// gives a friend.
	AffinityKilledMine float64

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

	// LookaheadHorizons is how many more planning horizons a body looks past
	// the first one, in multiples of PlanHorizon (stage 67). Zero is the
	// default and is the world every measurement so far was taken in: one
	// window, and a body that cannot die inside it
	// reads a flat gradient - which is why a satiated body put no value at all
	// on keeping food for later, and why carrying (stage 40), caches (stage
	// 50) and money (stage 51) all hit the same wall.
	//
	// Why it is not on, although it measures well. Over 96 seeds on the
	// largest single rule this world has had, and it is monotone in the dose:
	// a quarter of a window is worth 26 population, half is worth 39, a whole
	// one 50.5 (95.97 -> 146.49). Nothing gets worse except starving
	// (+0.38 ***) and the generation count (-0.38 **), and the death rate is
	// down overall - so the starving is replacing being killed, not adding to
	// it. Killing is down a fifth (killRate -0.55 ***), groups hold together
	// half again as long (halfLife +25.32 ***), the rarer species' trough is
	// better rather than worse (+0.03 *), and nothing goes extinct at any
	// dose. On the played map it is worth 11.38 ** as long as there is no
	// money on it.
	//
	// The one world where it costs is a world with coins in it (-6.18 *), and
	// there it does not merely cost population: it stops the exchange economy
	// (sales -4.28 ***, gifts -152.78 ***, cooking -66.68 ***) because a coin
	// becomes worth holding and one hand is all there is. A map with money
	// scattered on it may therefore also take the gate off the hand
	// (CarrySlotted false, CarryDiminishes true, stage 71), which brings the
	// market back - at a population cost of its own, and at a cost to how
	// evenly the two species share the world. Money, lookahead and a steady
	// pair of species is a combination this world does not yet have.
	//
	// Stage 76 re-measured that over 96 seeds with the three fixes of stages
	// 72 to 74 in, and found why it is a combination and not a setting. The
	// market comes back (sales 1.76 -> 5.23 ***) and rareTrough still falls
	// 0.16 *** - the fixes softened it by nothing - and it falls by the same
	// 0.17 *** on a map with no money on it at all, so what is being paid for
	// is the carrying and not the coins. The control says the same from the
	// other side: with the weight taken off as well, the cost disappears
	// (rareTrough -0.02 +/- 0.04) and so does the market (sales 0.00),
	// because willSell is coin > food - lug and coin is at most food, so the
	// whole of a seller's margin lives inside the lug. The thing that makes a
	// sale possible and the thing the world pays for are one quantity.
	//
	// And the reason it is still off: with the second window a body that
	// cannot live out two horizons unfed reads every option as equally
	// hopeless, because the life term is a difference of two death
	// probabilities and both saturate. A starving body then courts instead of
	// eating and a cornered one lies down instead of running - two things the
	// design says must never stop holding. The goals priced by a constant
	// (offspring, exploring) do not shrink with the gradient, so they win.
	//
	// What it costs to run: 11% of a decision, measured on one settled world
	// with nothing changed but this figure. Timing the benchmark the obvious
	// way says the opposite, because the world with the second window has half
	// again as many bodies in it and the benchmark is then timing a different
	// body.
	//
	// The second window is the body's own metabolism and nothing else: hunger
	// climbs at its own rate and vitality drains at what that hunger costs.
	// Whatever is hitting it now stays in the first window, where it was
	// measured - being hit today is not a fact about seven hundred ticks from
	// now, and carrying it forward would empty the tank in every candidate
	// alike and leave nothing to choose between (the flattening stage 55a found
	// from the other side).
	LookaheadHorizons float64

	// LookaheadNeverBlinds is whether looking further ahead is allowed to tell
	// a body less than looking closer did (stage 74). False is stage 67 as it
	// was built: the chained window is the only one the life term is read
	// through, saturation and all.
	//
	// The life term asks whether this body will be dead by the end of the
	// window. One meal moves a starving body's death from tick 594 to tick
	// 1038: against a window of 700 that is inside to outside and the answer
	// changes, against 1400 it is inside to inside and the answer does not.
	// The meal did not change - the question did - and a meal worth nothing
	// loses to anything with a constant price on it.
	//
	// With this on, a difference is read through whichever of the two windows
	// separates the states further, and nothing is lost by it: the chain is
	// monotone in the near window, so wherever the far one discriminates at
	// all it ranks the same way. What it fixes is the exchange rate between
	// the life goal and everything that is not priced in death.
	LookaheadNeverBlinds bool

	// LookaheadReadsRate is how much of the chance of starving inside a window
	// is read as a rate rather than as a deadline (stage 93). Zero is every
	// world before it.
	//
	// What it is aimed at is one line in oneHorizon. The chance of running out
	// is 1 - ticksLeft/PlanHorizon while the tank lasts less than a window,
	// and nought otherwise - so a body that outlasts the window reads nought
	// however fast it is going, and a body with room to spare has no gradient
	// at all. Every dead end P14 ran into is that flat: a coin, a price, an
	// ornament and a coat were all worth nothing to a body in no trouble,
	// because everything here is priced as a difference in one chance of
	// dying and the difference was zero.
	//
	// The rate reading is the same countdown taken as a hazard - how many
	// times over a window this body would run out at this drain - which never
	// reaches zero and keeps its shape out past the horizon. It is blended
	// rather than swapped in, because a tail that does not vanish raises what
	// every body reads, and a uniform rise in the risk is a rise in LifeValue,
	// which stage 67 measured on its own and which costs population. So the
	// weight is a dial, and the arm to read it against is the flat one.
	LookaheadReadsRate float64

	// GoalsNeedSurvival is whether a goal that happens later is discounted by
	// the chance of this body being there for it (stage 73).
	//
	// False is every world measured before it, and in those worlds a child and
	// a walk are worth the same to a body about to die as to a whole one.
	// What that costs shows up as soon as the formula looks further ahead:
	// staying alive is priced as a difference of two chances of dying and so
	// shrinks as the window lengthens, while a constant does not, so the
	// longer the window the more surely the constants win. It is why a
	// starving body courted instead of eating with the second window on, and
	// it is the last thing standing between that window and being the default.
	GoalsNeedSurvival bool

	// CoinBuysMending is whether a coin is a claim on the better of the two
	// meals this world sells rather than on a plain one (stage 81).
	//
	// Why it is not a third thing money is good for, and so does not fall foul
	// of #73: a carcass is food. What changes is which food the claim is on.
	//
	// It exists because of what stage 79 measured. The seller compares
	// CoinValue x keepValue(a meal) against at least keepValue(what it holds),
	// so for an ordinary meal the ratio is exactly CoinValue whatever the
	// body's state - the same figure on both sides of the comparison, cancelling
	// - and the seller is short by half, every time. A claim that can be spent
	// on mending is the one thing that breaks that, because what a body is
	// holding is usually a plant and a plant does not mend.
	//
	// The plan asked for the claim to be shared out over what the body needs
	// now, with weights taken from its hunger and from what is hitting it. No
	// weights are needed: mealValueAt caps mending at what the body is actually
	// missing, so a whole body gets nothing from the second branch and a
	// half-dead one gets all of it. Measured across states, the coin is worth
	// the same to a whole body, 1.20x to one at seven tenths, 1.96x at four
	// tenths and 2.52x at two.
	//
	// What the plan asked for and did not get is a stone. A coin cannot buy
	// one: nobody offers a stone for sale (stage 77) and nobody would walk up
	// for one, and a stone in hand is worth 0.106 against a coin's 18.506 -
	// 175 times too small to win a comparison the moment it entered one.
	CoinBuysMending bool

	// LookaheadHolds is whether the second window knows about the food in this
	// body's hands (stage 78). False is stages 67 to 74 as they were built:
	// out there a body is carried forward on its own metabolism and on what it
	// has been managing to find (LookaheadUpkeep), and what it is holding at
	// this moment - the one thing it is certain of - counts for nothing.
	//
	// It is a level and not a rate, because a meal is one drop in hunger and
	// not a slower climb, and it is discounted by LookaheadUpkeep rather than
	// by a figure of its own: eating what is in your hand is part of keeping
	// yourself up, and this world does not need a second number for it.
	//
	// The safety is structural rather than argued. The chain is p + (1-p) x
	// next, so where the first window dominates the second contributes almost
	// nothing - and holding something changes the first window not at all,
	// because a body that has not eaten is exactly as hungry as it was. A
	// starving body with a meal in its hand still reads itself as starving.
	//
	// And it is off by default for a reason worth stating plainly: it is the
	// same knob as LookaheadUpkeep pointed at food in hand, and at one that
	// knob takes stage 67 with it. The shortfall the second window sees is
	// exactly what makes a satiated body value keeping anything; a body that
	// assumes the thing in its hand fills that shortfall has that much less
	// reason to be holding it.
	LookaheadHolds bool

	// LookaheadSpoils is whether keeping something is worth nothing when the
	// thing will have gone off by the time it is wanted (stage 78). What it
	// reads is the item's own clock, which is not hidden - how near a thing is
	// to turning is a fact about the thing, like what it is and what has been
	// done to it - against the wait keepValue already values the thing at.
	//
	// It is not a taper. Spoiled food leaves this world entirely, so at the
	// moment of need the thing is either there or it is not.
	LookaheadSpoils bool

	// LookaheadUpkeep is how much of its own metabolism a body assumes it
	// will go on covering inside the second window (stage 72). Zero is stage
	// 67 as it was built: the body eats nothing out there, which is what
	// makes a satiated one value a meal kept for later and what makes a body
	// in trouble read every option as equally hopeless.
	//
	// What it multiplies is what this body has actually been getting, capped
	// at its own metabolism - looking ahead is what happens if things go on
	// as they are, not what happens if they go well - so a body that has been
	// feeding itself sees hunger climb at (1 - this) of its usual rate, and
	// one that has been going hungry sees no relief at all. It is the same
	// shape of discount as CarryValue, StoreValue and CoinValue: what you
	// will get, less than in full.
	LookaheadUpkeep float64

	// LookaheadWornAgain is whether the second window charges the standing
	// hazard of being worn down a second time (stage 72). False is right and
	// is the default where the lookahead is used at all.
	//
	// ShockRisk is the chance a body with nothing left in the tank does not
	// survive the next thing that happens to it, and it is calibrated against
	// one planning horizon. Chaining two windows charged it twice, so a
	// wounded body was told it was half again as doomed as the rule says -
	// and since the life term is a difference of two chances of dying, that
	// is what made escaping worth 0.63 where one window made it worth 46.23.
	// Starving is a countdown and does belong in both windows; being worn is
	// a standing hazard and belongs in the window it was measured for.
	LookaheadWornAgain bool

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
	LoreInitSpread float64

	// MateWeightSpread is how far apart bodies are in what a child is worth to
	// them (stage 94). Zero is every world before it: the figure is the
	// world's own for everybody, nothing is drawn for it and nothing mutates,
	// so a world that has not asked for the rule runs bit for bit as it did.
	//
	// It is the fourth preference and the first one about a goal rather than
	// about a danger - the three that were there price what an option might
	// cost, and none of them can make one body keener on offspring than
	// another. It sits outside the gene budget for the reason a chronotype
	// and a taste do: it is not a quantity of anything a body could buy more
	// of, so putting it in the budget would claim a trade nothing in biology
	// makes.
	//
	// Spread and mutation are one switch rather than two because what is
	// being asked is whether the population varies at all. LoreMutationStd
	// sets how far a child drifts from its parent, as it does for the others.
	MateWeightSpread float64

	// NoiseWeight and NoiseWeightSpread are how hard a body's judgement
	// wobbles, and how far apart bodies are in it (stage 95). One is every
	// world before it, and a spread of nought means the figure is one for
	// everybody: nothing is drawn for it and nothing mutates.
	//
	// Until this, the spread of the error a body makes scoring an option was
	// (MaxAbility - Intelligence) / MaxAbility * ChoiceNoise, so one gene set
	// both how well a body could tell two options apart and how far it would
	// wander from its own ranking. Those are different things - a clever body
	// that goes on hunches and a dull one that sticks to its ranking are both
	// buildable animals - and with one number doing both, "impulsive" and
	// "calculating" were not two ways of being built.
	//
	// The two fields are separate because the control arm needs them to be:
	// handing every body the same raised amplitude is what says whether it is
	// the variation that matters or simply the amount of noise (the same
	// flat-bias control stage 54 needed). It sits outside the gene budget with
	// the chronotype, the taste and what a child is worth, for the same reason
	// - it is not a quantity of anything a body could buy more of.
	NoiseWeight       float64
	NoiseWeightSpread float64

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

	// NursingSpeedShare is how fast a mother goes while a child of hers is
	// within the rearing radius, as a share of her own speed (stage 66). One
	// is every world before it, and the default.
	//
	// It is not a leash and not a rule about families: what it changes is one
	// number in her body, and whether the child stays beside her is left to
	// the comparison it was always left to (see nursing.go).
	NursingSpeedShare float64

	// NursingAlways is the control (stage 66): a guardian is slowed for the
	// whole of the rearing whether or not its child is anywhere near.
	//
	// The same slowness with the structure taken out of it, which is stage
	// 54's shape of control: if the flat arm does as well, what the rule buys
	// is a slower mother and not time spent beside a child. Set the share so
	// that the two arms slow by the same amount on average - the structured
	// one is only in force while the child is within the radius, which is
	// about four ticks in five.
	NursingAlways bool

	// MateLoreChance is how often a birth also hands something on between the
	// two parents (stage 65). Zero is every world before it, and the default.
	//
	// It exists because a bonded pair trades nothing at all. While the timer
	// runs, stepPaired only walks them together - no decision is taken and no
	// watching happens - so exchangeLore is never reached, and the count of
	// trades between two who are bonded is exactly nought (measured). And
	// since mating and birth are the same instant in this world, tryBirth is
	// the only place that instant can be pointed at.
	//
	// What it does is call the same exchangeLore everything else calls. There
	// is no separate trade for mates and nothing chosen for them to hand on:
	// the one new thing here is how often it is called. That is deliberate -
	// the function has never asked who two agents are to each other, and a
	// rule about families would be the thing this stage must not become.
	MateLoreChance float64

	// MateLoreTrades is how many trades that one moment is worth (stage 65),
	// and one unless a measurement asks otherwise. It is the arm that says
	// whether what limits this rule is the rate rather than the rule: a bond
	// that ran its whole timer watching would manage several.
	MateLoreTrades int

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

	// AllyValue and AllyFlat are wanting goodwill for its own sake (stage 96),
	// and the control that says whether the wanting needs a reason. Both are
	// nought in every world before it.
	//
	// What the goodwill an act would buy is worth has been LoreValue flat
	// since stage 48: the same figure to a body with nobody in the world and
	// to one standing among friends. AllyValue adds to it, and the addition
	// falls away as this body already has trust in sight -
	//
	//	LoreValue + AllyValue/(1 + trust standing near)
	//
	// - so the first one a body is on terms with is worth the most and the
	// fifth almost nothing. That shape is the answer to the trap this stage
	// was warned about (#112(c)): a want with nothing behind it hoards, and a
	// want that is sated by having any at all cannot.
	//
	// It is also what keeps the rule away from the wall this world has
	// measured six times over: goodwill makes resting look safe, so a body
	// with friends about is a body that rests among them and dies of it. The
	// damping means the rule reaches hardest into the bodies that have none -
	// 48% of decisions, counted before it was built - and least into the ones
	// already on the wrong side of that.
	//
	// AllyFlat is the same addition with no damping at all, and it is the arm
	// that has to be run beside it: set to AllyValue times the mean damping,
	// the two arms differ in whether the wanting has a reason and not in how
	// much of it there is (the control stage 54 and stage 95 both needed).
	//
	// Selling is left on LoreValue. The two ends of a sale have to read the
	// same figure (stage 68), the seller's is a rule of the world with no
	// walk over the neighbours to hand, and AffinitySale is nought by default,
	// so no world runs both.
	AllyValue float64
	AllyFlat  float64

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

	// SkillWardRelief is how much of a warded beast's blow a fully mastered,
	// fully suited body turns aside (stage 62), and how much it knocks off
	// what standing near one looks like it will cost. Zero is a world where
	// knowing the beasts buys nothing, which is the arm this is measured
	// against.
	SkillWardRelief float64

	// SkillSwimRelief is how much of the chance that a tick in the water is
	// the last one (stage 34) a fully mastered, fully suited body escapes.
	//
	// The gene behind it is vitality, because what stage 34 found decides who
	// comes out of a river is how long a body stays in it rather than how
	// fast it crosses. Zero is the same kind of arm as the one above.
	SkillSwimRelief float64

	// SkillSwimSpeedRelief is how much of the water's drag on a body's speed
	// a fully mastered, fully suited one gets back (stage 97). Zero is the
	// arm where knowing the water keeps nobody quick in it.
	//
	// A second figure rather than a second job for SkillSwimRelief, because
	// the two are two effects and not one effect and its price. Stage 62's
	// SkillWard shares one number between its two halves for exactly that
	// reason - there, what a blow does and what standing near one looks like
	// it will cost are the same fact wearing two hats. Not drowning and not
	// being dragged are different things, so the control that switches one
	// off has to leave the other standing.
	SkillSwimSpeedRelief float64

	// SkillCookRelief is how much of what an ignorant cook wastes a fully
	// mastered, fully suited body gets back (stage 52b). It is a yield and
	// not a speed: what being good at this means is that what comes out is
	// better, not that it takes less time - the correction stage 43 had to
	// make about fishing, kept here from the start.
	//
	// The gene behind it is intelligence, which is the gene the awkward crop
	// already hangs on: knowing a way of doing something is a matter of how
	// well the body judges the doing. Two skills on one gene is not new
	// (throwing and fishing from the bank both hang on rationality).
	//
	// And this is the one skill with nothing to read off the ground. Every
	// other one is seeded by where a body was born - the rough share, the
	// water share, what grows there - and five measurements running have said
	// that a skill so seeded does not move where bodies live. Cooking is
	// seeded flat, at SkillBirthplace for everybody, so the whole of the
	// spread in it comes from the leaps and the copying. It is the first
	// skill in this world whose distribution owes geography nothing.
	SkillCookRelief float64

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

	// WaterSpeedShare is how much of its speed a body keeps while it is in
	// the water (stage 97), as a multiplier. One is the world before this
	// stage - dear to cross and no slower - and it is still the default, so
	// every map measured until now runs as it did.
	//
	// This is the one place terrain acts on speed, and stage 20 said it never
	// should. Both of that decision's reasons have been looked at again
	// (PLAN.md, stage 97): the first - that a multiplier on speed could not be
	// told apart from effort in a measurement - is answered by an arm that
	// slows the water without making it dear; the second - that putting the
	// figure on the cost is what would finally set speed against vitality -
	// was measured three times and came back at nought (stages 20, 33, 57).
	// The cost stays where it is: water is slow AND dear, and because the
	// cost is charged by the tick, crossing at half speed pays it twice.
	//
	// It goes on Agent.speedNow, which is what a body can do today, and never
	// on Agent.MaxSpeed, which is what it is built for. Stage 57 drew that
	// line: a ceiling that moves with the ground makes a body that steps over
	// a boundary and finds itself above its own limit.
	WaterSpeedShare float64

	// WaterSlowKnown says whether a body feels how slow the water it is
	// standing in has made it (stage 97). True is the ordinary world: the
	// figure is in Perception.Self.MaxSpeed, so every option that involves
	// covering ground is priced with the legs the body actually has. False
	// leaves the water just as slow and the body reckoning with legs it has
	// not got, which is the control that says how much of whatever moves is
	// the choosing and how much is simply what happened to the ones who were
	// in there. The same shape as DrownKnown and ChillKnown.
	WaterSlowKnown bool

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

	// GroundAheadSeen says whether a body prices an option with the ground one
	// cell toward where that option would take it, rather than with the ground
	// under its own feet (stage 100).
	//
	// False is every world before it. Since stage 20 the utility has assumed
	// "the country ahead is like the country I am standing on", which is the
	// most an animal without a map can do - and it is also why nothing about
	// terrain has ever changed where a body stands: five stages (34, 35, 63,
	// 98, 99) put a cost or a danger on the water and none of them could be
	// told from the arm where the body could not feel it, because no term in
	// the comparison knows where an option would take the body.
	//
	// What is read is the two figures that are vitality - the crossing cost
	// and the drain - and not the drowning chance, which stage 34 priced per
	// tick of the option's duration. Putting a destination's chance into that
	// term would change what Hazard means and make four stages of recorded
	// figures unreadable; it belongs in its own stage.
	//
	// The read is applied to the whole of an option's duration, which is the
	// same assumption as today's with one cell more in it: "the country ahead
	// is like the next cell" rather than "like this one". It is not a path.
	GroundAheadSeen bool

	// GroundAheadNoise is how badly that reading goes, scaled the way every
	// other reading of the world is: (MaxAbility - rationality)/MaxAbility
	// times this, as the spread of one normal draw in units of the crossing
	// multiplier (stage 100).
	//
	// Rationality and not intelligence, because this is reading the world
	// rather than choosing among what has been read - the same split that has
	// kept the two genes from taking on each other's work since stage 6. The
	// error is added, not scaled, so a body that reads badly enough can think
	// a river is a field, which is the failure worth being able to see.
	//
	// Zero is the arm that separates "being able to look" from "looking
	// correctly".
	GroundAheadNoise float64

	// GroundAheadBlind is the control for the above: the body looks ahead,
	// draws the same random numbers and prices the option the same way, but
	// what it reads is the world's average ground rather than the cell it is
	// about to step onto (stage 100).
	//
	// Stage 35 is why this exists. There, a belief that correlated 0.77 with
	// the truth raised the population - and so did an arm whose belief was
	// wrong, by the same amount. Without this control, "the information did
	// it" cannot be told from "the numbers moved".
	GroundAheadBlind bool

	// WaterDrain is what a tick spent in the water takes out of a body in
	// vitality, whatever the body is doing (stage 99). Nought is every world
	// measured before it, where the water was a toll on movement and nothing
	// at all to a body standing still in it.
	//
	// A drain rather than a heavier toll, for two reasons that are the whole
	// of the stage. It reaches the 37% of wet ticks a body spends standing
	// still, which cost nothing until now - the hole stage 97 measured from
	// the other side. And a drain is the one shape the lookahead reads
	// already (pressures adds the place's figure to the metabolism), so the
	// body reckons with it without a line of new code - which is exactly what
	// stage 85 built that path for.
	//
	// What it is expected to do, and how it differs from the cold that failed
	// (stage 86): a place's drain is subtracted inside recoverable, so what
	// gets dearer is RESTING here in particular, while every other option
	// only sees the same common shift. The cold could not act on that because
	// a region is 400 wide and walking does not get a body out of it; a wet
	// cell is 77 wide and two steps do.
	WaterDrain float64

	// WaterDrainKnown says whether a body feels what the water is taking out
	// of it (stage 99). False leaves the water taking just as much and the
	// body unable to reckon with it - the control that says how much of
	// whatever moves is the choosing and how much is the ones who stayed in
	// the river being gone. The same shape as DrownKnown, ChillKnown and
	// WaterSlowKnown.
	WaterDrainKnown bool

	// DrownUnskilledFactor is how much more likely the water is to be the end
	// of a body that cannot swim at all than of one that knows it (stage 98).
	//
	// One is the world before this stage - every body faces the ground's own
	// figure, whatever it knows - and nothing here draws a random number, so a
	// world at one runs identically.
	//
	// What this stage is FOR is the individual difference, not the danger.
	// Raising DrownChancePerTick for everybody was measured at stage 34 and
	// takes the world with it (five times over leaves a population of 18 and
	// no enemies at all), so the arm this has to beat is a flat rise by the
	// same mean multiplier - if it cannot, what it bought was the level and
	// not the spread, which is what stage 95 found about the wobble.
	//
	// It is one curve with SkillSwimRelief and not a second knob on the same
	// effect: the multiplier below runs from this figure at no skill down to
	// one, and the relief carries on down from there. Stage 97 kept its own
	// figure for the drag because not drowning and not being dragged are two
	// effects; here there is one effect and two ends of it.
	DrownUnskilledFactor float64

	// DrownSkillFull is the realised swimming at which the penalty above is
	// gone and the water is as dangerous as it has always been.
	//
	// It exists because the realised figures in this world are small and
	// two-humped: counted before this was built, 71% of the bodies standing in
	// a river hold no swimming at all and the rest are up around 0.5, so a
	// curve that only reaches its floor at a mastery of one would leave even
	// the ones who know the water paying most of the penalty - which is to say
	// it would be the flat rise it has to be told apart from. One is that
	// linear curve, and it is an arm rather than the default.
	DrownSkillFull float64

	// DrownBeliefPerBody says whether what an agent believes a place will do
	// to it is scaled by its own swimming as well (stage 98).
	//
	// False is the ordinary world, and the reason is what the belief is: a
	// place is as dangerous as it is, and stage 35's rule learns that from the
	// ground underfoot and from watching somebody go under. Scaling it per
	// body makes "it is dangerous over there" a different number for the one
	// who says it and the one who hears it, which is a change to what is being
	// handed on and not a change to what is true. True is the arm that says
	// how much that distinction is worth.
	DrownBeliefPerBody bool

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

		PreyValue:        1,
		MeatPerBudget:    120,  // an ordinary agent leaves 4 items, a large enemy many more
		MeatFromKills:    true, // only what something brought down (2026-09-22)
		CarryCapacity:    1,
		CarryCost:        0.5,
		CarryValue:       0.5,
		FishShare:        0, // stage 42: what a map holds is the map author's to say
		SpecialtyShare:   0, // stage 44: the same
		SpecialtySpread:  0.6,
		SpecialtyCatch:   0.25,
		Stones:           0, // stage 45: the map author scatters them
		AffinityGift:     6, // stage 48: the same as a shared kill
		AffinitySale:     0, // stage 68: off, as stage 51 measured it
		SaleGoodwillKept: true,
		// Stage 69: off, and the map author or the experiment turns it on.
		Books:               false,
		BookSubject:         BookSkills,
		WriteTicks:          40,
		BookFidelity:        0.7,
		BookValue:           0.5,
		BookSurvivesReading: true,
		Throwing:            false, // stage 46: measured before it is given a default
		ThrowRange:          70,    // longer than an arm (15), shorter than sight (130)
		ThrowDamage:         6,
		ThrowHit:            0.9,
		ThrowFalloff:        0.6,
		MeatSurplusFree:     true,
		MeatVitality:        0.5, // stage 39: half of the eater's own ceiling. 0 is the world before it
		MeatHealKnown:       true,
		MeatNutrition:       1,
		MeatClaimTicks:      400,
		MeatSpoilTicks:      900,
		LandedFishKeeps:     false, // a fish out of the water is dead flesh
		HuntCreditTicks:     200,

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

		// Off by default, and the two figures below are what an arm sets
		// when it is on: a push of a third of CombatRadius (the fight goes
		// on) and a fall worth about two blows.
		KnockbackDist:   0,
		KnockbackFall:   2.3,
		KnockbackThrown: false,

		// Off by default for the same reason knocking back is: what a push
		// is worth depends entirely on what is behind the one being pushed,
		// which is the map author's business. The figures below are what an
		// arm sets when it is on.
		ShovePush:       0,
		ShoveCost:       0.20,
		ShoveWorks:      0.5,
		ShoveLearnBoth:  true,
		ShoveLearnRate:  1,
		ShoveGroundSeen: true,

		// Off by default because room costs budget: see LessonSlots. The
		// figures below are what an arm sets when it is on - two slots at
		// half what a rule of thumb's room costs, since a lesson is one
		// pair rather than a pair and a weight, and a push of about a
		// tenth of what a meal is worth.
		LessonSlots:     0,
		LessonSlotCost:  2.5,
		LessonWeight:    3,
		LessonRipeTwice: true,
		LessonsSpread:   true,

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
		AffinityPairBond: 18,
		AffinityBirth:    18,
		AffinityKin:      22,
		AffinityHunt:     6,
		// Whose side a body is on (TODO 14): all five off. Counted before
		// they were written - a friend is fighting somebody liked less in
		// 16-22% of decisions, and an item is taken from under somebody
		// 1100-1900 times a run, nine tenths of them by a stranger - so the
		// target is there; what these are waiting on is the measurement of
		// what they do to the population.
		FightTrustCost:       0,
		AffinityNegative:     false,
		AffinitySnatched:     0,
		SnatchForgiveness:    0,
		SnatchPriced:         true,
		ClaimRetriggers:      false,
		AffinityAlly:         0,
		AllyPaidOnBlows:      true,
		AllyPaidOncePerFight: false,
		AffinityKilledMine:   0,
		AffinityWitnessKill:  2,
		KillWitnessFactor:    2,
		KillWitnessLooks:     true,
		CallTicks:            30,
		Coins:                0, // stage 51: the map author scatters them
		CoinValue:            0.5,
		CoinPricedCertain:    false, // stage 51's pricing, put right
		Dropping:             false, // stage 70: the map author turns it on

		// Stage 71: the hand as a slot, and what a second thing in it is
		// worth. The first two are the world as it was; the third is a fix.
		Trinkets:                false,
		TrinketValue:            0.1,
		CraftTicks:              30,
		CraftVitality:           2,
		TrinketSpoilTicks:       2000,
		TrinketSpread:           0.5,
		TrinketsVanish:          false,
		SkillTrinketRelief:      0.5,
		TrinketTaste:            1,
		TrinketTasteMutation:    0.08,
		TrinketStyleAimed:       false,
		AdornNeedsSurvival:      true,
		ClimateMap:              nil,
		ChillAheadSeen:          true,
		ChillDrain:              0,
		HeatDrain:               0,
		SeasonTicks:             0,
		ChillKnown:              true,
		ChillGradient:           false,
		WardShare:               0,
		WardStrength:            0,
		GiftWorthScaled:         false, // TODO 12: a gift earns the same whoever gets it
		TrinketFancyTicks:       0,     // TODO 12: a taste is what a body was born with
		TrinketFancySpread:      0,
		AdornSatiety:            0, // TODO 12: the fourth piece is worth what the first was
		AdornForgetPerTick:      0,
		AdornKeepsSated:         false,
		HidePerBudget:           0, // TODO 8: no material, and every world before it
		WardNeedsHide:           false,
		MaxHideItems:            140,
		SaleAnchor:              0,
		CoinBuysWarding:         false,
		CoinBuysOnlyMeals:       false,
		HandOverCheapest:        false,
		GiftPriced:              false,
		GiftSelfLookahead:       0, // stage 92a: giving is free unless the map says otherwise
		GiftKinWeight:           0, // stage 92b: and buys nothing extra for one's own
		CoinPrices:              false,
		CoinPriceBlind:          false,
		SalePriceSplit:          true,
		CarrySlotted:            true,
		CarrySlotsWeigh:         false,
		CarryDiminishes:         false,
		BurdenIgnoresWeightless: true,
		// Stage 52. The word costs the vocabulary whether or not anybody uses
		// it, so a world without cooking is CookVitality at zero rather than
		// a world with a shorter list.
		// Stage: occupancy. Nought is every world before it - a meal reached
		// is a meal had - and the control defaults to the body knowing.
		EatTicks:      0,
		EatTicksKnown: true,

		CookTicks:            20,
		CookVitality:         0.5, // the same as a carcass, so the two never stack
		CookQuality:          1,   // anybody can cook; a map that wants otherwise says so
		CookSurvivesHands:    true,
		LonelyValue:          0, // stage 55a: measured before it is a default
		LonelyHalfLife:       400,
		GuardianIsMother:     true,                // stage 53a
		SexBias:              [NumGenes]float64{}, // stage 53: measured before it is a default
		ParentFeedByKin:      true,                // stage 53b: and never one without the other
		MoodWeight:           0,                   // stage 54: measured before it is a default
		MoodDreadGain:        1,
		MoodCheerGain:        1,
		MoodHalfLife:         150,
		CarryPricedBackwards: false,
		StoreCapacity:        6,
		MaxStores:            16,
		StoresKnownToAll:     false,
		StoreFindLooks:       50,
		StoreInheritStrength: 3,
		StoreWitnessStrength: 1,
		StoreCryStrength:     0.5,
		StoreValue:           0.5,
		OfferTicks:           0,
		WaresSeen:            true,
		AllyTrustWeight:      1,
		AffinityDecayPerTick: 0.0008,
		AffinityTrust:        20,

		LifeValue:      100,
		OffspringValue: 42,
		TimeCost:       0.012,
		VitalityWeight: 0.55,
		PlanHorizon:    700,
		ShockRisk:      0.55,

		CoinBuysMending:      false, // stage 81: measured on its own
		LookaheadHolds:       false, // stage 78: measured, and not the default
		LookaheadSpoils:      false, // stage 78
		LookaheadNeverBlinds: true,  // stage 74: on with the window
		LookaheadReadsRate:   0,     // stage 93: a deadline, not a rate
		GoalsNeedSurvival:    true,  // stage 73: on with the window
		LookaheadWornAgain:   false, // stage 72

		// Three quarters of its own upkeep assumed in the second window
		// (stage 72). At zero it eats and mends nothing out there, which is
		// stage 67 as it was built; at one it sees no shortfall coming at all
		// and stage 67's whole effect goes with it.
		LookaheadUpkeep: 0.75,

		// One window further out (stage 67), which has been the default since
		// 2026-09-13. Zero is the world every figure recorded before that was
		// measured in, and every arm of cmd/experiment that does not say
		// otherwise now runs with it on.
		//
		// It took three attempts. The population always said yes - over 96
		// seeds it is the largest single rule this world has had - and twice
		// it had to be put back the same day, because
		// scenarios the design turns on stopped holding - a starving body
		// courting instead of eating, a cornered one lying down instead of
		// running. Three things were wrong, and all three are fixed here
		// rather than settled for:
		//
		//   - the second window carried the body forward eating and mending
		//     nothing, so anything worn or hungry read every option as
		//     equally hopeless (LookaheadUpkeep, stage 72);
		//   - it charged ShockRisk in both windows, though that figure is
		//     calibrated against one (LookaheadWornAgain, stage 72);
		//   - and the life term asks whether a body will be dead by the end
		//     of the window, so a meal that moves death from tick 594 to tick
		//     1038 registers as nothing at all once the window is 1400 - the
		//     meal did not change, the question did (LookaheadNeverBlinds,
		//     stage 74).
		//
		// With those in, the scenes come back and the world is worth 46.39 ***
		// population over 96 seeds, with killing down 0.43 ***, groups holding
		// together half again as long, intelligence bought much harder and the
		// rarer species' trough untouched. It costs 11% of a decision.
		LookaheadHorizons: 1,

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

		// Water slows nobody by default (stage 97): a map's author turns it
		// on, the way they turn on everything else the ground can do.
		WaterSpeedShare: 1,
		WaterSlowKnown:  true,

		// Calibrated against how long a body actually spends in the water,
		// which was counted before the rule was written: about a seventh of
		// all agent-ticks on the river map, in stays averaging 127 ticks. At
		// this rate a stay of that length is survived nineteen times in
		// twenty and drowning is a few per cent of all deaths - see
		// HISTORY.md, 2026-09-07.
		DrownChancePerTick: 0.0002,
		DrownKnown:         true,
		// Nought is off: the water is a toll on movement and nothing to a body
		// standing in it, as it has been since stage 34.
		WaterDrain:      0,
		WaterDrainKnown: true,
		// On since 2026-09-19 (stage 100): an option is priced with the ground
		// one cell toward where it would take the body. It is the third
		// deliberate reset of the measuring baseline, after stage 7b and the
		// lookahead - every terrain figure recorded before that date was
		// measured with the ground underfoot standing in for the ground
		// ahead, and cmd/experiment's groundunread arm puts that world back.
		//
		// A world with no map reads nothing and draws nothing, so the flat
		// default runs exactly as it always did.
		GroundAheadSeen:  true,
		GroundAheadNoise: 1,
		// One is off: the water takes the swimmer and the sinker alike, as it
		// has since stage 34. The reachable-mastery point is set even so, so
		// that turning the rule on is one figure and not two.
		DrownUnskilledFactor: 1,
		DrownSkillFull:       0.5,

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
		NursingSpeedShare:   1,    // a mother is as quick as anybody (stage 66)
		FoodRenormalize:     true, // the world grows as much as it did (stage 15a)
		EnemyBudgetMean:     520,  // over the human 360, so a carcass feeds several
		EnemyBudgetStd:      90,
		HumanNestTicks:      500,  // one a year
		HumanNestLife:       5000, // for ten years
		BossRouseTicks:      500,  // a year out before it will walk back in
		NestQuietYears:      5,
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
		LoreInitSpread:   0.15,
		MateWeightSpread: 0, // stage 94: everybody wants a child the same amount
		// Stage 95: everybody's judgement wobbles exactly as much as its
		// intelligence says, which is every world before it.
		NoiseWeight:       1,
		NoiseWeightSpread: 0,
		LoreMutationStd:   0.08,

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
		// Stage 96: goodwill is worth the same to a body with nobody as to one
		// among friends, which is every world before it.
		AllyValue: 0,
		AllyFlat:  0,

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
			SkillWard:   GeneDefence,
			// Fishing from the bank is patience and a good eye - reading
			// where the fish is from outside the water - so it is capped by
			// rationality, the gene for how well a body reads the world.
			// Fishing in the water is capped by vitality, the same as
			// swimming: what decides how much of the river a body can work
			// is how long it can stay in it.
			SkillFishLand:  GeneRationality,
			SkillFishWater: GeneVitality,
			// Getting an awkward thing out of the ground is working out how,
			// which is what intelligence is for in this world - and it is the
			// one gene no skill has been hung on yet, so what it can support
			// has never been asked.
			SkillHarvest: GeneIntelligence,
			// Putting a stone where it was meant to go is reading the world
			// accurately, which is what rationality is for here - and never
			// power, which would make the skill a second name for the gene
			// that already decides what the stone does on arrival (#71).
			SkillThrow: GeneRationality,
			// Knowing a way of doing something is a matter of how well the
			// body judges the doing, which is the same reading that put the
			// awkward crop on this gene.
			SkillCook: GeneIntelligence,
			// Setting down what is held is the memory gene by meaning.
			// Which gene the world will actually pay for is stage 69's
			// own question, and both are run as arms.
			SkillScribe: GeneMemory,
			// Making something to be looked at is capped by attractiveness: the only
			// gene in this world about how a body is seen, and the only one
			// no skill has ever asked anything of. What it can support has
			// never been measured, and stage 26 put it among the least bought
			// things the world pays for at all - which makes it the honest
			// gene to hang a want with no survival value on.
			SkillTrinket: GeneAttractiveness,
		},
		SkillForageRelief:    1,
		SkillSwimRelief:      1,
		SkillSwimSpeedRelief: 1,
		SkillPoisonRelief:    1,
		SkillWardRelief:      0.5,
		SkillFishReach:       2,
		FishCatchWater:       1,
		FishCatchBank:        1,
		SkillFishRelief:      1,
		SkillHarvestRelief:   1,
		SkillCookRelief:      1,
		SkillThrowRelief:     1,
		SkillsSpread:         true,
		HintSlotCost:         5,
		HintWeightStd:        6,
		HintWeightMax:        20,
		HintTradeWorth:       0.5,
		HintsSpread:          true,
	}
}
