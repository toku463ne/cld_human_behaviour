package engine

import (
	"math"
	"math/rand"
)

const (
	// Speed of an agent that is following its partner rather than chasing a
	// target of its own.
	pairFollowEffort = 0.35

	// How often the onlookers around a fight, and the two in it, take a fresh
	// reading of each other. Updating on every single tick of a long fight
	// would cost a lot and tell them almost nothing new.
	spectateInterval = 20

	// How long ActObserve takes before it has told the watcher anything.
	observeTicks = 10

	// How long after the first blow of an engagement the question "did that
	// one hit back" is answered, and how long a gap ends the engagement.
	//
	// Not on the first blow: being hit is what makes the other side think
	// again, and it cannot do that until the next tick, so the first blow
	// always reads "no" whoever it lands on. Five ticks is long enough for an
	// agent that was eating or running to turn round, and short enough that
	// the answer is still about this fight.
	retaliationSampleTicks = 5
	engagementGapTicks     = 20
)

// Stats is an aggregated view of the world, cheap enough to compute per frame.
type Stats struct {
	Tick       int
	Population int
	Males      int
	Females    int
	FoodItems  int
	Births     int
	Evaded     int // blows dodged entirely
	Hunts      int // kills that fed somebody: a carcass with a claim on it
	HuntParty  int // how many took part, summed over those kills
	JointHunts int // of those, the ones more than one agent had a hand in

	// FirstSights is how many times an agent has had to assume something about
	// somebody it had never taken a reading of, and FirstSightError the total
	// distance those assumptions were off by. The mean of the two is what
	// learning from looks is meant to bring down.
	FirstSights     int
	FirstSightError float64

	// The same two counted over the agents that had learned enough to use
	// their own line. In a world with the learning off, these stay at zero.
	FirstSightsLearned     int
	FirstSightErrorLearned float64

	// What the same guesses would have cost with the build ignored, and with
	// the flat prior the world used before stage 10. Counterfactuals over the
	// same encounters, so the three are directly comparable.
	FirstSightErrorFlat  float64
	FirstSightErrorFixed float64

	Geniuses      int // births that drew an exceptional budget
	GreatGeniuses int // ... and the rarer, larger kind. Not counted in Geniuses
	Deaths        int
	Kills         int
	AgingDeaths   int // subset of Deaths caused by Lifespan reaching zero

	// DrownDeaths is a bucket of its own on purpose (stage 34). Every
	// measurement of this world reads the causes of death apart from one
	// another - starving is Deaths less the rest - so a new way to die that
	// was folded into an old one would turn up as a change in starvation or
	// in killing in every A/B taken from here on.
	DrownDeaths int

	// KillWitnesses is how many readings onlookers have taken of somebody
	// they watched kill, and AvengeWitnesses how many times one of them
	// thought better of its own kind for killing something else (stage 31).
	// Observes is how many decisions were "watch somebody", against Decisions.
	KillWitnesses   int
	AvengeWitnesses int
	KillLessons     int
	Observes        int

	// SkillsLearned is how many times somebody took on or improved a skill,
	// and SkillsCopied how many of those came from watching (stage 38a).
	SkillsLearned int
	SkillsCopied  int
	SkillsBorn    int
	SkillsLeapt   int

	// What becomes of the meat (stage 39): how many items carcasses left, how
	// many of those rotted where they lay, how many were eaten, and how many
	// plants were eaten beside them. The share of meals that are meat is what
	// says whether a rule about meat has anything to bite on.
	// MeatItems is how much carcasses have offered and MeatKeepable how much
	// of it those who made the kills could have carried away (stage 41); the
	// difference is the surplus. MeatEatenHeld and MeatEatenFree split the
	// meat actually eaten by whether the eater had a claim on it.
	MeatItems     int
	MeatKeepable  int
	MeatEatenHeld int
	MeatEatenFree int

	// FishEaten is how many mouthfuls came out of the water (stage 42).
	FishEaten int

	// FishMissed is how many attempts at one ended with it getting away
	// (stage 43), and FishSpoiled how many landed ones went off before
	// anybody ate them: a fish out of the river is dead flesh and keeps no
	// better than a carcass.
	FishMissed  int
	FishSpoiled int

	// HarvestMissed is how many attempts at the awkward crop came to nothing
	// (stage 44).
	HarvestMissed int

	// Gifts is how many times something changed hands (stage 48), and
	// GiftsCried how many of those were made by a body that had been crying
	// its wares (stage 49).
	Gifts      int
	GiftsCried int

	// Cries is how many decisions were "here is what I have" (stage 49),
	// OffersHeard how many were taken with somebody's wares in sight, and
	// OfferDraws how many of those were a walk towards one.
	Cries       int
	OffersHeard int
	OfferDraws  int

	// Throws is how many stones have been thrown and ThrowHits how many of
	// them landed (stage 46).
	Throws    int
	ThrowHits int

	MeatDropped int
	MeatSpoiled int
	// MeatHealing is all the vitality carcasses have mended (stage 39).
	MeatHealing float64

	// StarvedDeaths is the same figure cmd/experiment works out from the
	// other buckets, and StarvedFoodSeen how many of those bodies had food in
	// sight within a planning horizon of dying (stage 40). SightTicks and
	// SpareTicks are ticks with food in sight, and those of them where the
	// body was not hungry - the room there is to pick something up for later.
	// Taken is how many times something has been picked up (stage 40).
	Taken int

	StarvedDeaths   int
	StarvedFoodSeen int
	StarvedFoodNear int
	SightTicks      int
	SpareTicks      int
	MeatEaten       int
	PlantsEaten     int

	// PlantsSpat is how many bites came to nothing because the plant was
	// poisonous enough to be dropped (stage 17b), and PoisonLoss all the
	// vitality the crop has taken off the population.
	PlantsSpat int
	PoisonLoss float64

	// Calls is how many decisions were an invitation, and Joins how many
	// attacks were aimed at something another agent had already declared for
	// (stage 32).
	Calls int
	Joins int

	// DrownWitnesses is how many times an agent has seen the ground take
	// somebody (stage 35), summed over witnesses: one drowning in front of
	// three of them counts three times.
	DrownWitnesses int

	// Decisions is how many times a controller has been asked for an action,
	// and RegionDraws how many of those were the one option that acts on a
	// belief about somewhere else (stage 15b). Measurement only.
	Decisions     int
	RegionDraws   int
	Matured       int // agents that finished growing up
	ChildDeaths   int // subset of Deaths of agents that never got there
	Children      int // alive right now and not grown yet
	Fights        int
	MaxGeneration int

	// Flees is how many times an agent decided to run from somebody, and
	// Escapes how many of those ended with the pursuer out of sight. The
	// share of the two is what says whether running away works, which is what
	// changes when sight stops being a circle: a block can be left by crossing
	// one line, and how far that is depends on where the agent was standing.
	Flees   int
	Escapes int

	// Exchanges is how many times two agents have traded what they assume
	// (stage 12b). Counted so that the rate can be read next to the effect:
	// a rule that hardly ever fires explains nothing whatever its weight.
	Exchanges int

	// HintsCopied is how many rules of thumb have passed from one agent to
	// another rather than down a bloodline (stage 12c).
	HintsCopied int

	AvgPower        float64
	AvgRationality  float64
	AvgIntelligence float64
	AvgVitality     float64
	AvgHunger       float64

	// What the population is made of age-wise: how old the average agent is,
	// how far through growing up it is, and how much of its inheritance it can
	// express today. The last is the one that says whether the world is full
	// of children, of adults, or of the old.
	AvgAge       float64
	AvgMaturity  float64
	AvgAgeFactor float64
}

// attack is one blow queued during a tick, applied once everybody has acted so
// that two agents hitting each other trade damage simultaneously.
type attack struct {
	fromID, toID int
	effort       float64

	// thrown says this one was a stone from out of reach (stage 46), and hit
	// the chance it finds its mark before the target's guard and footwork are
	// asked about. A blow at arm's length always arrives; a thrown one may
	// not, and that is the only difference in how the two resolve.
	thrown bool
	hit    float64
}

// World holds the whole simulation state. It knows nothing about rendering or
// networking: callers drive it with Step and read it back with Agents, Foods
// and Stats.
type World struct {
	cfg Config

	// rng is the single source of randomness of the simulation. Everything,
	// including the controllers, draws from it, so a given seed always
	// reproduces the same run.
	//
	// draws is the same generator's counter (save.go). The state of Go's
	// source cannot be written out, so what a snapshot stores is the seed and
	// how many numbers have come out of it; counting is what makes that
	// possible, and it forwards the stream unchanged.
	rng   *rand.Rand
	draws *countingSource

	agents []Agent
	foods  []Food

	// ground is the country the world is laid out on (terrain.go), nil for a
	// flat world. Drawn once from the config and never changed by the
	// simulation: agents cross it, nothing reshapes it.
	ground *terrainGrid

	// water is every cell of the map that is water, worked out once when the
	// world is built (stage 42). Fish go in one of these, drawn uniformly,
	// which is what makes how many a region has follow from how much of it is
	// water without a figure to tune. Empty in a flat world.
	water []cell

	// rubble is every cell of broken ground, where the stones of stage 45
	// lie. Worked out the same way and for the same reason as the water.
	rubble []cell

	// regions is the world's own coarse division of itself (region.go). It is
	// drawn once and never changes; foodWeight is the sum of what the blocks
	// grow, kept so that drawing a place for a plant does not add them up
	// again every time.
	regions    []region
	foodWeight float64

	// enemyWeight is the same sum for where the enemies arrive (stage 58),
	// and tolls is what has died in each region and how much of it was
	// violent. The tolls are a measurement and nothing in the world reads
	// them: they are here because deaths are too rare to read at an instant,
	// so the only way to ask what a dangerous region costs is to keep count.
	enemyWeight float64
	tolls       []regionToll

	// What the weighting actually put in, against what is standing about
	// later (stage 58): how many enemies the world has put in from outside,
	// the arrival weight summed over them, and how many were born here
	// instead. Measurements; no rule reads them.
	enemyArrivals   float64
	enemyArrivalSum float64
	enemyBorn       float64

	// And how many of each sort arrived (stage 59). The mix the table asks
	// for is about what the world puts in; what is standing about later is
	// the world's own doing, and with a handful of enemies alive it drifts
	// far enough to say nothing.
	enemyArrivalsByKind []float64

	// pendingSeeds are seeds that have been carried somewhere in an animal
	// and are waiting for the world's next planting (stage 17c).
	pendingSeeds []pendingSeed

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

	births     int
	evaded     int // blows that missed because the target got out of the way
	hunts      int // kills whose carcass went to somebody that eats it
	huntParty  int // and how many had a share, summed over those kills
	jointHunts int // and how many of the kills had more than one hand in them

	// What the assumptions about strangers cost in accuracy: how many first
	// sights there have been and the total error across them. Read only - the
	// completion condition of stage 10 is that the mean of these two falls.
	firstSights     int
	firstSightError float64
	// ... and the subset made by an agent with enough readings to go by its
	// own line rather than the flat prior.
	firstSightsLearned     int
	firstSightErrorLearned float64
	// The same encounters scored by the two estimators the learned one is
	// measured against (appearance.go).
	firstSightErrorFlat  float64
	firstSightErrorFixed float64
	geniuses             int
	greatGeniuses        int
	deaths               int
	kills                int
	agingDeaths          int
	drownDeaths          int
	// drownWitnesses is how many times somebody has watched the ground take
	// somebody else (stage 35). Counted because a rule that hardly ever fires
	// explains nothing whatever its weight - the lesson of stage 24.
	drownWitnesses int

	// The same count for a killing seen (stage 31), split by which way the
	// sign went: killWitnesses is readings taken of somebody who killed,
	// avengeWitnesses is onlookers who thought better of one of their own for
	// killing something else. Both are counted whatever the weights are set
	// to, so that an arm with the rule off still says how often it could have
	// fired.
	killWitnesses   int
	avengeWitnesses int

	// killLessons is how many of those readings were actually taken in. Most
	// are not: a memory full of people who matter has no room for a face in
	// the crowd, whatever it was just seen doing.
	killLessons int

	// skillsLearned is how many times a skill was taken on or improved on,
	// and skillsCopied how many of those came from watching somebody rather
	// than from a parent or a birthplace. A rule that spreads has to be shown
	// to spread.
	skillsLearned int
	skillsCopied  int
	skillsBorn    int
	skillsLeapt   int

	// What becomes of the meat (stage 39). A carcass is the only food this
	// world makes rather than grows, and before changing what one is worth it
	// has to be said how much of it there is, how much of it is eaten and how
	// much of it rots: a rule about meat can do nothing about a mouthful
	// nobody was ever going to take.
	// heldKind is how many items of each kind are in somebody's hands (stage
	// 40), and taken how many times something has been picked up. What is
	// held counts against the world's allowance: carrying moves food about,
	// it does not make room for more of it.
	heldKind [NumFoodKinds]int
	taken    int

	// Counting the target before building carrying (stage 40, #67). A body
	// that starves with food it had seen a moment ago is a death an item in
	// hand would have prevented; a body that starves having seen nothing for
	// a long while is one that carrying could not have saved. The share of
	// the first is what the whole stage can be worth.
	starvedDeaths   int
	starvedFoodSeen int
	// ... and the same with a window short enough to mean "it was right
	// there": a body that saw a meal a whole planning horizon ago is not one
	// an item in hand would obviously have saved.
	starvedFoodNear int
	// And the room there is to pick anything up: ticks spent with food in
	// sight and no hunger worth speaking of, against ticks in sight at all.
	sightTicks int
	spareTicks int

	// meatHealing is all the vitality carcasses have put back into the
	// population (stage 39). Counted whatever MeatVitality is set to, so an
	// arm with the rule off still says how much of a difference it could
	// have made.
	meatHealing float64

	// What a carcass offers against what those who made it could take away
	// (stage 41). Counted in every arm, including the one where the surplus
	// stays closed, because "is there a surplus at all" is the premise the
	// whole stage rests on.
	meatItems    int
	meatKeepable int
	// And who ends up eating it: somebody with a claim on it, or somebody
	// who came upon what was left.
	meatEatenHeld int
	meatEatenFree int
	fishEaten     int // stage 42: mouthfuls that came out of the water
	fishMissed    int // stage 43: attempts that ended with the fish getting away
	// fishSpoiled is how many landed fish went off before anybody ate them -
	// in a hand, or lying where the body that carried them died. A fish still
	// in the river has no clock on it at all.
	fishSpoiled int
	// What has been handed over (stage 48), and to whom.
	gifts            int
	giftsToKin       int
	giftsToMates     int
	giftsToStrangers int
	giftStones       int
	// ... and how much of it followed a cry (stage 49): gifts made by a body
	// that had been crying its wares within the last two cries' worth of
	// ticks. It is the closest this world can come to asking whether the
	// advertisement is what brought the two of them together.
	giftsCried int

	// cries is how many decisions were "here is what I have" (stage 49),
	// offersHeard how many were taken with somebody's wares in sight, and
	// offerDraws how many of those were a walk towards one. The last is the
	// one the stage turns on: a cry nobody walks to is a noise.
	cries       int
	offersHeard int
	offerDraws  int

	// The caches whoever laid this world out put on the ground (stage 50),
	// and what has come of them: items put in, items taken out, and how many
	// times anybody came to know a place, split by the path it came down.
	stores    []store
	stored    int
	withdrawn int
	// What the money came to (stage 51): sales made, and buyers turned away
	// by somebody who would not part with what it was holding. The second is
	// the one the stage turns on - it says whether the market failed for want
	// of buyers or for want of sellers.
	sales        int
	salesRefused int

	// Losing sight of the one a body thinks best of (stage 55a), and how
	// often a decision was a walk along the direction that left.
	lostSight   int
	lonelyDraws int

	// What the cooking came to (stage 52): items prepared, how much of it was
	// a carcass, and where the cooked thing ended up. The last two are the
	// monopoly question - cooking that never leaves the cook is cooking no
	// exchange can be built on.
	cooked       int
	cookedMeat   int
	cookedEaten  int
	cookedHanded int

	storeLearned int
	storeFound   int
	storeSeen    int
	storeTold    int
	// sawOffer is set by perceive when this look turned up somebody's wares,
	// and read by the decision that look was for. It is a scratch flag rather
	// than a second scan of the neighbours: the scan that builds the
	// perception has already been over every one of them.
	sawOffer bool

	throws        int // stage 46: stones thrown
	throwHits     int // ... of those, the ones that landed
	harvestMissed int // stage 44: attempts that failed to get the crop out

	meatDropped int // items left by carcasses
	meatSpoiled int // ... of those, the ones nobody got to in time
	meatEaten   int // ... and the ones somebody did
	plantsEaten int // for the share: meat against everything eaten

	// plantsSpat is how many bites failed because what was bitten was
	// poisonous enough to be dropped (stage 17b, 2026-09-08).
	plantsSpat int

	// poisonLoss is all the vitality the crop has taken off the population.
	// Counted so that a skill against it can be weighed before it is written:
	// what a rule undoes is the ceiling on what undoing it is worth.
	poisonLoss float64

	// calls is how many decisions were "come and help me bring this down"
	// (stage 32), and joins how many attacks were on something somebody else
	// had already declared for. The second is the one the stage turns on: a
	// call nobody answers is a word, not a hunt.
	calls int
	joins int

	// observes is how many decisions were "watch somebody" - the share of
	// them is what says whether a rule that teaches for free has killed off
	// the action that teaches for a price (#55).
	observes int

	// decisions is how many times a controller has been asked, and
	// regionDraws how many of those answers were "go to better country". The
	// share of the two is the ceiling on what any belief about a place can do
	// to where a body stands (stage 35).
	decisions     int
	regionDraws   int
	matured       int
	childDeaths   int
	fights        int
	maxGeneration int

	// What the world actually does, against which what the agents believe can
	// be checked: how many blows were sampled and how many of them were
	// answered, and how many proposals were made and accepted. Counted over
	// the whole run and read only by the measurement (World.Lore); no rule
	// consults them, and no agent has access to them.
	blowsSeen          int
	blowsAnswered      int
	courtships         int
	courtshipsAccepted int

	// Attempts to run away, and the ones that ended with the pursuer out of
	// sight rather than with the agent dead or thinking better of it.
	flees   int
	escapes int

	// How many trades of what agents assume have taken place (stage 12b), and
	// how many ideas were copied in the course of them (stage 12c).
	exchanges   int
	hintsCopied int
}

// NewWorld creates a world populated according to cfg. The same cfg (same seed
// included) always yields the same simulation.
func NewWorld(cfg Config) *World {
	w := &World{
		cfg:         cfg,
		agents:      make([]Agent, 0, cfg.InitialPopulation),
		foods:       make([]Food, 0, cfg.InitialFoodItems),
		index:       make(map[int]int, cfg.InitialPopulation),
		foodIndex:   make(map[int]int, cfg.InitialFoodItems),
		ai:          &AIController{},
		nextAgentID: 1,
		nextFoodID:  1,
	}
	w.rng, w.draws = newCountingRand(cfg.Seed)
	// Before anybody is put in it, because what the ground is like is not
	// something the population decides.
	w.ground = buildTerrain(&w.cfg)
	w.water = waterCells(w.ground)
	w.rubble = stoneCells(w.ground)
	w.buildRegions()
	// The stones are laid out before anybody arrives (stage 45): they are
	// part of what the ground is, not something the world keeps producing.
	w.scatterStones()
	w.scatterCoins()
	for i := 0; i < cfg.InitialPopulation; i++ {
		w.addAgent(w.randomAgent(SpeciesHuman))
	}
	for i := 0; i < cfg.InitialEnemies; i++ {
		w.addAgent(w.randomAgent(SpeciesEnemy))
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

// FoodByID returns a copy of the food item with the given ID. An interface that
// has to say whether the meal somebody went after is still there needs it, and
// walking Foods() for one item is the sort of thing that ends up in a draw loop.
func (w *World) FoodByID(id int) (Food, bool) {
	f := w.foodByID(id)
	if f == nil {
		return Food{}, false
	}
	return *f, true
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
		Tick:       w.tick,
		Population: len(w.agents),
		FoodItems:  len(w.foods),
		Births:     w.births,
		Evaded:     w.evaded,
		Hunts:      w.hunts,
		JointHunts: w.jointHunts,

		FirstSights:     w.firstSights,
		FirstSightError: w.firstSightError,

		FirstSightsLearned:     w.firstSightsLearned,
		FirstSightErrorLearned: w.firstSightErrorLearned,
		FirstSightErrorFlat:    w.firstSightErrorFlat,
		FirstSightErrorFixed:   w.firstSightErrorFixed,
		HuntParty:              w.huntParty,
		Geniuses:               w.geniuses,
		GreatGeniuses:          w.greatGeniuses,
		Deaths:                 w.deaths,
		Kills:                  w.kills,
		AgingDeaths:            w.agingDeaths,
		DrownDeaths:            w.drownDeaths,
		DrownWitnesses:         w.drownWitnesses,
		KillWitnesses:          w.killWitnesses,
		KillLessons:            w.killLessons,
		AvengeWitnesses:        w.avengeWitnesses,
		Observes:               w.observes,
		SkillsLearned:          w.skillsLearned,
		SkillsCopied:           w.skillsCopied,
		SkillsBorn:             w.skillsBorn,
		SkillsLeapt:            w.skillsLeapt,
		MeatHealing:            w.meatHealing,
		Taken:                  w.taken,
		StarvedDeaths:          w.starvedDeaths,
		StarvedFoodSeen:        w.starvedFoodSeen,
		StarvedFoodNear:        w.starvedFoodNear,
		SightTicks:             w.sightTicks,
		SpareTicks:             w.spareTicks,
		MeatItems:              w.meatItems,
		MeatKeepable:           w.meatKeepable,
		MeatEatenHeld:          w.meatEatenHeld,
		MeatEatenFree:          w.meatEatenFree,
		FishEaten:              w.fishEaten,
		FishMissed:             w.fishMissed,
		FishSpoiled:            w.fishSpoiled,
		HarvestMissed:          w.harvestMissed,
		Gifts:                  w.gifts,
		GiftsCried:             w.giftsCried,
		Cries:                  w.cries,
		OffersHeard:            w.offersHeard,
		OfferDraws:             w.offerDraws,
		Throws:                 w.throws,
		ThrowHits:              w.throwHits,
		MeatDropped:            w.meatDropped,
		MeatSpoiled:            w.meatSpoiled,
		MeatEaten:              w.meatEaten,
		PlantsEaten:            w.plantsEaten,
		PlantsSpat:             w.plantsSpat,
		PoisonLoss:             w.poisonLoss,
		Calls:                  w.calls,
		Joins:                  w.joins,
		Decisions:              w.decisions,
		RegionDraws:            w.regionDraws,
		Matured:                w.matured,
		ChildDeaths:            w.childDeaths,
		Fights:                 w.fights,
		MaxGeneration:          w.maxGeneration,
		Flees:                  w.flees,
		Escapes:                w.escapes,
		Exchanges:              w.exchanges,
		HintsCopied:            w.hintsCopied,
	}
	var sumPower, sumRationality, sumIntelligence, sumVitality, sumHunger float64
	var sumAge, sumMaturity, sumFactor float64
	for i := range w.agents {
		a := &w.agents[i]
		if a.Sex == Female {
			s.Females++
		} else {
			s.Males++
		}
		sumPower += a.Gene(GeneAttack)
		sumRationality += a.Gene(GeneRationality)
		sumIntelligence += a.Gene(GeneIntelligence)
		sumVitality += a.Vitality
		sumHunger += a.Hunger
		sumAge += float64(a.Age)
		sumMaturity += a.Maturity
		if a.Maturity < 1 {
			s.Children++
		}
		sumFactor += a.AgeFactor(&w.cfg)
	}
	if s.Population > 0 {
		n := float64(s.Population)
		s.AvgPower = sumPower / n
		s.AvgRationality = sumRationality / n
		s.AvgIntelligence = sumIntelligence / n
		s.AvgVitality = sumVitality / n
		s.AvgHunger = sumHunger / n
		s.AvgAge = sumAge / n
		s.AvgMaturity = sumMaturity / n
		s.AvgAgeFactor = sumFactor / n
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
	w.spawnEnemyOfTick()

	// What the ground under each body is worth to it this tick (stage 57),
	// before anybody decides anything with it.
	w.standOnGround()

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
		} else if !w.keepToGuardian(a) {
			w.perform(a)
		}
		a.pruneRejected(w.tick)
		w.forgetProposalIfGone(a)
		w.keepInBounds(a)
	}

	w.resolveAttacks()
	w.metabolise()

	// What the ground itself does, settled with the rest of the tick's deaths
	// rather than in the middle of the movement (stage 34).
	w.drownings()

	// After everybody has moved, so a carried seed comes up where its carrier
	// ended up rather than where it set off (stage 17c).
	w.dropSeeds()

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
	//
	// Came into view, not is in view. The difference is the whole cost of
	// this branch: an agent crossing a place with food in it sees food on
	// every tick of the crossing, and firing on each of them asks it to think
	// again about a thing it decided to walk past a tick ago. Rising edges
	// only, so it is asked once per arrival.
	if a.Action.Kind == ActRest || a.Action.Kind == ActMove {
		sees := w.nearestFoodInSight(a) >= 0
		arrived := sees && (!a.sawFood || w.cfg.SightingRetriggers)
		a.sawFood = sees
		if arrived {
			return TriggerFoodInSight
		}
		// So did somebody worth crossing the world for. This adds no action:
		// courting is scored by the same utility comparison as everything
		// else. It only stops an agent walking past a candidate because it
		// happened not to be thinking at that moment.
		if a.CanReproduce(&w.cfg) {
			seesMate := w.strikingCandidateInSight(a)
			arrivedMate := seesMate && (!a.sawMate || w.cfg.SightingRetriggers)
			a.sawMate = seesMate
			if arrivedMate {
				return TriggerMateInSight
			}
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
	p.Trigger = trigger
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
	// How often a decision is "go to country I think better of" (stage 15b),
	// which is the only door a belief about a place has into a body. Counted
	// for the world's own controller; a hand-driven node is not asked this
	// question. Measurement only.
	w.decisions++
	if a.Action.Kind == ActObserve {
		w.observes++
	}
	if ai, ok := c.(*AIController); ok {
		if ai.ChoseBetterGround {
			w.regionDraws++
		}
		if ai.ChoseMissing {
			w.lonelyDraws++
		}
		if ai.JoinedDeclared {
			w.joins++
		}
		if ai.WentToOffer {
			w.offerDraws++
		}
	}
	if w.sawOffer {
		w.offersHeard++
	}

	a.lastDecisionTick = w.tick
	a.vitalityAtDecision = a.Vitality
	a.needsDecision = false
	a.pendingTrigger = TriggerNone
	a.actionTicks = 0

	if a.Action.Kind == ActInvite {
		w.calls++
	}
	if a.Action.Kind == ActOffer {
		w.cries++
		a.criedAt = w.tick
		// Somebody advertising is somebody saying where the goods are (stage
		// 50, the pair to stage 49): everybody who can see the cry picks up
		// the places the crier knows. It is the one path by which knowledge
		// of a cache crosses a line of descent without anybody watching it be
		// used.
		w.tellStores(a)
	}

	switch a.Action.Kind {
	case ActCourt:
		// The comparison clock. It is meant to say how long this agent has
		// been looking for a mate, and the metabolism starts it when the agent
		// first becomes able to look (see reproReady). Restarting it here as
		// well makes it say something else - how long this attempt has been
		// going - and since a rejection ends the attempt, an agent that is
		// turned down goes back to the beginning of its patience and holds out
		// for an obvious catch for ever.
		if w.cfg.CourtClockResets && a.State != StateSeekMate {
			a.courtStartTick = w.tick
		}
		a.State = StateSeekMate
	case ActAttack:
		a.State = StateFighting
	case ActFlee:
		a.State = StateFleeing
		w.flees++
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
		// Out of its own hands, if that is where it is (stage 40). No
		// distance to cover and nobody to race: the only thing carrying
		// changes about a meal is where it was a moment before.
		if a.carriedIndex(a.Action.TargetID) >= 0 {
			w.eatCarried(a, a.Action.TargetID)
			return
		}
		f := w.foodByID(a.Action.TargetID)
		if f == nil {
			a.requestDecision(TriggerTargetLost) // somebody else got it
			return
		}
		// How close it has to get. Ordinarily the world's reach; for a body
		// that has learned to fish from the bank, far enough to keep its feet
		// dry (stage 43).
		if reach := w.fishReach(a, f.Kind); dist2(a.X, a.Y, f.X, f.Y) > reach*reach {
			w.moveToward(a, f.X, f.Y, a.Action.Effort)
			return
		}
		// And whether it comes off. Two foods in this world can be reached and
		// still not had: a fish, which darts away (stage 43), and the awkward
		// crop, which stays where it is and has to be tried again (stage 44).
		if f.Kind == FoodFish && w.rng.Float64() >= w.fishCatch(a, f.Kind) {
			w.theFishGetsAway(f)
			a.requestDecision(TriggerTargetLost)
			return
		}
		if f.Special && w.rng.Float64() >= w.harvestCatch(a, f) {
			w.itCameApart(a)
			return
		}
		w.eat(a, f.ID)
		a.requestDecision(TriggerGoalReached)

	case ActTake:
		f := w.foodByID(a.Action.TargetID)
		if f == nil {
			a.requestDecision(TriggerTargetLost)
			return
		}
		if reach := w.fishReach(a, f.Kind); dist2(a.X, a.Y, f.X, f.Y) > reach*reach {
			w.moveToward(a, f.X, f.Y, a.Action.Effort)
			return
		}
		if f.Kind == FoodFish && w.rng.Float64() >= w.fishCatch(a, f.Kind) {
			w.theFishGetsAway(f)
			a.requestDecision(TriggerTargetLost)
			return
		}
		if f.Special && w.rng.Float64() >= w.harvestCatch(a, f) {
			w.itCameApart(a)
			return
		}
		w.take(a, f.ID)

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
		// Every channel this stance is using costs something, whether or not
		// it is the one that lands the blow.
		a.Vitality -= stanceCost(&w.cfg, a.Action.Stance) * a.Action.Effort
		a.effortSpent = math.Max(a.effortSpent, a.Action.Effort)

	case ActThrow:
		o := w.agentByID(a.Action.TargetID)
		if o == nil || !o.Alive {
			a.requestDecision(TriggerTargetLost)
			return
		}
		// Out of sight is out of reach, whatever the range says (stage 19's
		// promise, kept for the AI too). Too far is walked at, exactly as a
		// body closes on somebody it means to hit.
		r := w.throwRange()
		if r <= 0 || !a.canThrow(&w.cfg) {
			a.requestDecision(TriggerTargetLost)
			return
		}
		if !w.canSee(a.X, a.Y, o.X, o.Y) || dist2(a.X, a.Y, o.X, o.Y) > r*r {
			w.moveToward(a, o.X, o.Y, a.Action.Effort)
			return
		}
		w.throwStone(a, o)
		a.Vitality -= stanceCost(&w.cfg, a.Action.Stance) * a.Action.Effort
		a.effortSpent = math.Max(a.effortSpent, a.Action.Effort)
		// One stone, one decision: what to do next is a fresh question, and
		// the hand is empty now.
		a.requestDecision(TriggerGoalReached)

	case ActOffer:
		w.cry(a)

	case ActCook:
		// Making something of what is in the hand (stage 52). Nothing to walk
		// to and nobody to agree with: the whole of the price is standing
		// there, the same shape the cry uses.
		w.cook(a)

	case ActBuy:
		// Buying (stage 51). Close enough to put one thing in each other's
		// hands, which is the reach everything else is handed over at, and
		// then the seller's own reckoning decides. A refusal is not an
		// argument: the buyer is simply asked to think again.
		o := w.agentByID(a.Action.TargetID)
		if o == nil || !o.Alive {
			a.requestDecision(TriggerTargetLost)
			return
		}
		if dist2(a.X, a.Y, o.X, o.Y) > w.cfg.GrabRadius*w.cfg.GrabRadius {
			w.moveToward(a, o.X, o.Y, a.Action.Effort)
			return
		}
		w.sell(a, o)
		a.requestDecision(TriggerGoalReached)

	case ActStore:
		// Putting something in a cache (stage 50). Close enough to reach into
		// it, which is the reach everything else in the world is put down and
		// picked up at, and nothing to agree with: a place has no opinion.
		i := a.Action.TargetID
		if len(a.carried) == 0 || i < 0 || i >= len(w.stores) || !w.knowsStore(a, i) {
			a.requestDecision(TriggerTargetLost)
			return
		}
		st := &w.stores[i]
		if dist2(a.X, a.Y, st.X, st.Y) > w.cfg.GrabRadius*w.cfg.GrabRadius {
			w.moveToward(a, st.X, st.Y, a.Action.Effort)
			return
		}
		w.putInStore(a, i)
		a.requestDecision(TriggerGoalReached)

	case ActGive:
		// Handing something over (stage 48). Close enough to put it in their
		// hand, which is the same reach eating and courting use, and nothing
		// to agree on: receiving costs nothing, so there is nothing to refuse.
		o := w.agentByID(a.Action.TargetID)
		if o == nil || !o.Alive || len(a.carried) == 0 {
			a.requestDecision(TriggerTargetLost)
			return
		}
		if dist2(a.X, a.Y, o.X, o.Y) > w.cfg.GrabRadius*w.cfg.GrabRadius {
			w.moveToward(a, o.X, o.Y, a.Action.Effort)
			return
		}
		w.giveItem(a, o)
		a.requestDecision(TriggerGoalReached)

	case ActFlee:
		o := w.agentByID(a.Action.TargetID)
		if o == nil || !o.Alive {
			a.requestDecision(TriggerTargetLost)
			return
		}
		if !w.canSee(a.X, a.Y, o.X, o.Y) {
			w.escapes++
			a.requestDecision(TriggerGoalReached) // out of sight, out of danger
			return
		}
		w.moveDir(a, a.X-o.X, a.Y-o.Y, a.Action.Effort)

	case ActInvite:
		// Calling others in (stage 32). The whole of it is a moment spent
		// making a noise about something: the call is set the instant the
		// action starts, so that anybody who looks this tick sees it, and
		// after CallTicks the agent is asked again - by then whoever was
		// coming is visibly coming, and the fight it was calling about is
		// scored with them in it.
		//
		// Nothing here forms a party, hands out a share, or records who
		// answered. There is no accepting: an agent that comes is an agent
		// that decided the fight was worth it, which is the same comparison
		// it makes about everything else.
		o := w.agentByID(a.Action.TargetID)
		if o == nil || !o.Alive {
			a.requestDecision(TriggerTargetLost)
			return
		}
		if a.actionTicks >= w.cfg.CallTicks {
			a.requestDecision(TriggerGoalReached)
		}

	case ActObserve:
		o := w.agentByID(a.Action.TargetID)
		if o == nil || !o.Alive {
			a.requestDecision(TriggerTargetLost)
			return
		}
		if !w.canSee(a.X, a.Y, o.X, o.Y) {
			w.moveToward(a, o.X, o.Y, a.Action.Effort)
			return
		}
		if a.actionTicks >= observeTicks {
			w.observeStrength(a, o, w.cfg.CombatObsVariance*w.cfg.SpectateObsFactor)
			// Standing with somebody long enough to size them up is also
			// standing with them long enough to trade what you each assume
			// (stage 12b). It is the same act, so it gets no cost of its own.
			w.exchangeLore(a, o)
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
	if o == nil || !o.Alive || o.PartnerID != 0 || o.Sex == a.Sex || o.Species != a.Species {
		a.requestDecision(TriggerTargetLost)
		return
	}
	if dist2(a.X, a.Y, o.X, o.Y) > w.cfg.GrabRadius*w.cfg.GrabRadius {
		w.moveToward(a, o.X, o.Y, a.Action.Effort)
		return
	}
	// Both bodies have to be able to pay for a birth, or the bond produces
	// nothing and both of them have spent the time for it. The suitor is
	// checked here and not only when it set out, because it has been walking
	// since then and a walk costs vitality.
	if !o.CanReproduce(&w.cfg) || !a.CanReproduce(&w.cfg) {
		a.requestDecision(TriggerTargetLost)
		return
	}
	// Both sides have to be convinced, each on its own comparison clock. The
	// two answers are worked out separately rather than in one condition
	// because only one of them is news about the world: whether the other side
	// agreed. The suitor changing its own mind teaches it nothing about how
	// often a proposal is accepted, so that is not what it learns from.
	sawInThem, sawInIt := w.perceivedFitness(a, o), w.perceivedFitness(o, a)
	mine := w.willCommit(a, sawInThem)
	// The other side's answer is the other side's to give: their own rule
	// unless their controller takes the question (courtship.go). Until it is
	// answered the suitor stands here, which costs it time and nothing else.
	theirs, answered := w.askAboutCourtship(o, a, sawInIt)
	if !answered {
		return
	}
	// Both sides remember how it went. This is not new knowledge on either
	// side: the world already teaches the suitor whether the other agreed
	// (noteCourtship, which is where AcceptChance comes from), and its own
	// answer was its own. Nothing reads it back - it is for whoever is
	// watching, which until now had to guess which side had said no.
	a.lastCourt = CourtView{TargetID: o.ID, Tick: w.tick,
		Accepted: mine, TheyAccepted: theirs, Fitness: sawInThem, Bar: w.commitBar(a)}
	o.lastCourt = CourtView{TargetID: a.ID, Tick: w.tick,
		Accepted: theirs, TheyAccepted: mine, Fitness: sawInIt, Bar: w.commitBar(o)}
	w.noteCourtship(a, theirs)
	if mine && theirs {
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
		// Whichever of the two the tick reached first is standing here, and
		// the birth belongs to the bond rather than to that one: releasing
		// both is what makes it happen once (the other is no longer paired
		// when the loop gets to it, so it never comes back through here).
		//
		// It used to be written as "the lower ID performs the birth", which
		// looked like the same thing and was not: both partners run down the
		// same clock, so when the higher ID was reached first it hit zero
		// first, skipped the birth on the ID test, and released the pair.
		// About half of all bonds ended in nothing at all, and the numbers
		// still went up and down enough that nobody looked (2026-09-06).
		//
		// The arguments stay in ID order so that which parent is "the first"
		// - whose position the child is born at, whose lore it takes first,
		// which one it keeps to - does not depend on the loop either.
		if w.cfg.BirthNeedsLowerIDFirst && a.ID > partner.ID {
			// The world as it was, for measuring against.
		} else if a.ID < partner.ID {
			w.tryBirth(a, partner)
		} else {
			w.tryBirth(partner, a)
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

		// Only the part of the effort that went into the blow lands, so an
		// attacker that is also guarding hits for less. A thrown stone is one
		// event rather than a tick of an exchange, and it may simply miss
		// before anybody has to dodge it (stage 46).
		damage := damagePerTick(&w.cfg, from.Attack(&w.cfg), at.effort*from.mix().Attack)
		if at.thrown {
			damage = w.throwDamage(from, at.effort)
			if w.rng.Float64() >= at.hit {
				// Wide. Nothing is off balance for it: the reason a missed
				// swing leaves an opening (stage 24) does not reach across a
				// gap this size.
				to.noteHit(from.ID, w.tick)
				continue
			}
			w.throwHits++
		}

		// The one being hit is meanwhile doing whatever it chose: turning the
		// blow aside, not being there, or neither if it was eating - and all
		// of it worth less if its own last blow found nothing.
		to.noteHit(from.ID, w.tick)
		composure := to.composure(&w.cfg, w.tick)
		// And the ground it is standing on, if it is standing above the one
		// swinging at it (stage 30). Capped with everything else at
		// EvasionCap: high ground is an edge, not a place nothing reaches.
		chance := clamp(math.Max(to.evasion(&w.cfg)*composure, w.cover(to, from)), 0, w.cfg.EvasionCap)
		if chance > 0 && w.rng.Float64() < chance {
			w.evaded++
			// Swinging at somebody who was not there leaves the swinger open.
			// Nothing sees this coming: how hard somebody is to hit is a
			// hidden parameter, so a fight can only be found out by having it.
			if w.cfg.OpeningTicks > 0 {
				from.openUntil = w.tick + w.cfg.OpeningTicks
			}
			continue
		}
		damage *= 1 - to.defence(&w.cfg)*composure
		to.Vitality -= damage

		// The one taking the hits remembers exactly what they cost - and is
		// shaken by it, which is a different thing (stage 54): the memory is
		// of one body, and the fright is of the world.
		w.rememberDamage(to, from.ID, damage)
		w.frighten(to, damage)
		to.attackerID = from.ID
		to.lastAttackTick = w.tick

		// One reading of this fight per engagement, taken a few ticks in: what
		// the utility formula wants to know is whether starting on somebody
		// gets you hit back, which is a question about picking a fight and not
		// about a tick of one. Sampling every blow instead would answer it
		// mostly from long one sided beatings - exactly the fights where
		// nobody is hitting back - and the world would come out three times
		// milder than it is.
		if from.engageID != to.ID || w.tick-from.engageLast > engagementGapTicks {
			from.engageID, from.engageStart = to.ID, w.tick
		}
		from.engageLast = w.tick
		if w.tick-from.engageStart == retaliationSampleTicks {
			hitBack := to.Action.Kind == ActAttack && to.Action.TargetID == from.ID
			w.blowsSeen++
			if hitBack {
				w.blowsAnswered++
			}
			w.noteEngagement(from, to, hitBack)
		}

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

	spectated := w.cfg.CombatObsVariance * w.cfg.SpectateObsFactor
	// This runs after everybody has moved, so it is the second and last time
	// in a tick that the index is rebuilt. Nothing in the blows that follow
	// moves anybody, so the rest of the fights this tick reuse it.
	w.nearScratch = w.appendAgentsInSight(w.nearScratch[:0], x.X, x.Y)
	for _, i := range w.nearScratch {
		o := &w.agents[i]
		if !o.Alive || o.ID == x.ID || o.ID == y.ID {
			continue
		}
		if !w.canSee(o.X, o.Y, x.X, x.Y) {
			continue
		}
		w.observeStrength(o, x, spectated)
		w.observeStrength(o, y, spectated)
	}
}

// noteEngagement carries the answer to the other question a fight settles -
// did the one being hit hit back - to whoever is in a position to have seen
// it: the one throwing the punches, and the onlookers. Not the one being hit:
// its own choice is not evidence about how the world tends to answer, and an
// agent that learned from it would only be agreeing with itself.
//
// It walks the neighbourhood the way the strength readings do, but once per
// engagement rather than every twenty ticks, so it is the cheaper of the two.
func (w *World) noteEngagement(attacker, target *Agent, hitBack bool) {
	w.noteRetaliation(attacker, hitBack)
	if w.cfg.LearningRate <= 0 {
		return
	}

	w.nearScratch = w.appendAgentsInSight(w.nearScratch[:0], attacker.X, attacker.Y)
	for _, i := range w.nearScratch {
		o := &w.agents[i]
		if !o.Alive || o.ID == attacker.ID || o.ID == target.ID {
			continue
		}
		if !w.canSee(o.X, o.Y, attacker.X, attacker.Y) {
			continue
		}
		w.noteRetaliation(o, hitBack)
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
			// At the rate this hour suits this agent (stage 18): the same
			// recovery it always had, scaled by how close the world's clock
			// is to its own.
			a.Vitality += w.restRate(a) * (1 - clamp(a.effortSpent, 0, 1))
		}
		a.Vitality = math.Min(a.Vitality, a.MaxVitality(&w.cfg))

		w.grow(a)
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
	// And being worn down for a long stretch, which is a different thing from
	// being hungry: an agent can be well fed and still spend its life being
	// beaten below the vitality it takes to be alright. The counter resets the
	// moment it climbs back out, so this is the cost of a long bad spell
	// rather than a tally of every bad tick it ever had.
	if w.cfg.FrailLifespanRate > 0 && w.cfg.FrailVitalityShare > 0 {
		if a.Vitality < w.cfg.FrailVitalityShare*a.MaxVitality(&w.cfg) {
			a.frailTicks++
			if a.frailTicks > w.cfg.FrailGraceTicks {
				a.Lifespan -= w.cfg.FrailLifespanRate
			}
		} else {
			a.frailTicks = 0
		}
	}

	if a.Lifespan <= 0 {
		w.agingDeaths++
		w.kill(a)
	}
}

// grow moves an agent along towards being fully itself.
//
// Food buys it, not time: an agent grows at the rate it is fed, so a hungry
// childhood is a long one and a starving agent does not grow at all. Nothing
// here is a decision - there is no growing action and no growth term in the
// utility formula. An agent eats because it is hungry, exactly as before, and
// growing is what happens to a young body that manages it.
//
// The two lines this sits between are deliberately different: growth keeps
// improving all the way to a full stomach (hunger 0), while the lifespan cost
// of overeating starts at OverfedHunger, which is well before that. A child
// therefore has something to gain from the last few mouthfuls that an adult
// only pays for.
func (w *World) grow(a *Agent) {
	if a.Maturity >= 1 || w.cfg.ChildhoodYears <= 0 || w.cfg.TicksPerYear <= 0 {
		return
	}
	fed := clamp((w.cfg.SatiatedHunger-a.Hunger)/w.cfg.SatiatedHunger, 0, 1)
	if fed <= 0 {
		return
	}
	a.Maturity += fed / (w.cfg.ChildhoodYears * float64(w.cfg.TicksPerYear))
	if a.Maturity >= 1 {
		a.Maturity = 1
		w.matured++
	}
}

// keepToGuardian walks a child back towards the parent it was born to when it
// has wandered too far, and reports whether that is what it spent the tick on.
//
// This is the whole of childcare, and it is deliberately one-sided: the child
// does the staying, and nothing is asked of the parent. There is no feeding,
// no new action, and no term in anybody's utility formula for the survival of
// a child. What the arrangement is worth has to come out of the rules that
// already exist - a parent drives rivals off the ground it is standing on, and
// after stage 9 the two of them are the only pair in the neighbourhood willing
// to lie down and recover next to each other.
//
// A child whose parent has died is simply on its own from that tick.
func (w *World) keepToGuardian(a *Agent) bool {
	if !w.stillReared(a) {
		return false
	}

	guardian := w.agentByID(a.GuardianID)
	if guardian == nil || !guardian.Alive {
		a.RearingTimer, a.GuardianID = 0, 0
		return false
	}
	if dist2(a.X, a.Y, guardian.X, guardian.Y) <= w.cfg.RearingRadius*w.cfg.RearingRadius {
		return false
	}
	w.moveToward(a, guardian.X, guardian.Y, pairFollowEffort)
	a.State = StateForage
	return true
}

// stillReared reports whether this one is still keeping to a parent, and spends
// the tick of childhood if the rule is the timed one.
//
// There are two ways of saying when childcare ends and they are not the same
// question. The count is a length of time the world hands out at birth; being
// grown is a state of the child, which it buys with food (World.grow), so a
// hungry child stays a child. Under RearingUntilGrown the timer is not spent at
// all: nothing about it would mean anything, and leaving it standing at what it
// was born with says plainly that the clock is not what is being read.
func (w *World) stillReared(a *Agent) bool {
	if w.cfg.RearingUntilGrown {
		if a.GuardianID == 0 || a.Maturity >= 1 {
			a.GuardianID, a.RearingTimer = 0, 0
			return false
		}
		return true
	}
	if a.RearingTimer <= 0 {
		return false
	}
	a.RearingTimer--
	return true
}

// --- reproduction ----------------------------------------------------------

func (w *World) bond(a, b *Agent) {
	a.State, b.State = StatePaired, StatePaired
	a.PartnerID, b.PartnerID = b.ID, a.ID
	a.PairTimer, b.PairTimer = w.cfg.PairBondDuration, w.cfg.PairBondDuration

	// The first good thing either of them remembers about anybody. It is the
	// event that puts it there, not the time they go on to spend side by side:
	// standing next to somebody is not a relationship, and if it were, every
	// crowded agent would be fond of every other and there would be one group
	// in the world.
	w.rememberAffinity(a, b.ID, w.cfg.AffinityPairBond)
	w.rememberAffinity(b, a.ID, w.cfg.AffinityPairBond)
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
// It also reports whether this was one of those births, because stage 12c
// reuses the same rare event for the other kind of leap it needs: a rule of
// thumb that is about something else entirely. A new idea and a body built to
// a different scale are the same sort of event, and the world already had a
// name for it.
func (w *World) inheritBudget(pa, pb *Agent) (float64, bool) {
	from := pa
	if w.rng.Intn(2) == 1 {
		from = pb
	}
	h := clamp(w.cfg.BudgetHeritability, 0, 1)
	budget := h*from.Budget() + (1-h)*w.cfg.GeneBudgetMean

	// Now and then somebody is born with far more to be made of than either
	// parent had. The roll is taken every birth so that the random source is
	// consumed the same way whether or not it lands.
	genius := false
	switch roll := w.rng.Float64(); {
	case roll < w.cfg.GreatGeniusRate:
		budget = w.cfg.GreatGeniusBudget
		w.greatGeniuses++
		genius = true
	case roll < w.cfg.GreatGeniusRate+w.cfg.GeniusRate:
		budget = w.cfg.GeniusBudget
		w.geniuses++
		genius = true
	}
	return budget + w.rng.NormFloat64()*w.cfg.BudgetInheritSpread, genius
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
	// Room for rules of thumb is bought out of the same budget the body is,
	// and it is bought first: what is left over is what the genes are fitted
	// to. That is the whole economy of stage 12c - an agent carrying four
	// ideas is visibly smaller, slower or weaker than one carrying none, and
	// whether that trade is worth making is what selection is asked.
	budget, genius := w.inheritBudget(pa, pb)
	slots := w.inheritHintSlots(pa, pb, genius)
	hints := w.inheritHints(pa, pb, slots, genius)
	fitBudget(genome, budget-w.hintCost(slots))

	child := w.newAgent(
		(pa.X+pb.X)/2+w.randRange(-8, 8),
		(pa.Y+pb.Y)/2+w.randRange(-8, 8),
		w.randomSex(),
		genome,
		max(pa.Generation, pb.Generation)+1,
		0, // and whoever is born has it all to do
	)
	child.Species = pa.Species
	// And its sort (stage 59), from one parent or the other - the same coin
	// the budget is taken with (#2). Kinds are not species: courtship is
	// within a species and pays no attention to the row, so two sorts do meet
	// and their young are one or the other rather than something between.
	//
	// The coin is only tossed when the parents differ, so a world with one
	// sort takes nothing from the random source.
	child.Kind = pa.Kind
	if pa.Kind != pb.Kind && w.rng.Float64() < 0.5 {
		child.Kind = pb.Kind
	}
	child.ParentIDs = [2]int{pa.ID, pb.ID}
	child.lore = w.inheritLore(pa, pb)
	child.chronotype = w.inheritChronotype(pa, pb)
	child.hintSlots, child.hints = slots, hints
	// What it knows for having been born where it was, merged with what it
	// inherited by the one comparison there is (skill.go). A genius child
	// goes further with what it already holds - a leap is about something the
	// line already does, not a category nobody has ever seen.
	w.learnFromBirthplace(&child)
	if genius {
		w.leapSkill(&child)
	}
	// And where its parents keep things (stage 50). This is the widest of the
	// three ways of coming to know a place, and the reason a cache can end up
	// belonging to a line rather than to the world.
	w.inheritStores(&child, pa, pb)

	// It starts as a small thing that keeps to one of the two, and which one
	// is now a rule (stage 53a): the mother. Bearing and rearing being the
	// same body's work is the asymmetry the rest of this stage is derived
	// from - a body tied to a radius reads its neighbours, and a body free to
	// range brings things back - so it is put in as structure first and the
	// genes follow from it.
	//
	// Before this it was whichever of the two came first, with a comment
	// saying it mattered to no rule, and measured that made a father the
	// guardian 50.5% of the time. Zero ticks still means no childcare under
	// either rule, which is what the arm that takes childcare away is: naming
	// a guardian at all is what RearingUntilGrown reads.
	if w.cfg.ChildRearingTicks > 0 {
		child.GuardianID = pa.ID
		if w.cfg.GuardianIsMother {
			if pa.Sex == Female {
				child.GuardianID = pa.ID
			} else {
				child.GuardianID = pb.ID
			}
		}
	}
	child.RearingTimer = w.cfg.ChildRearingTicks

	if child.Generation > w.maxGeneration {
		w.maxGeneration = child.Generation
	}
	// Having got through it together counts for something on top of the bond
	// itself. The child's side of it is not recorded here: it has no ID until
	// the newborns are committed, and the parent link is what seeds it anyway
	// (see World.record).
	w.rememberAffinity(pa, pb.ID, w.cfg.AffinityBirth)
	w.rememberAffinity(pb, pa.ID, w.cfg.AffinityBirth)
	w.newborns = append(w.newborns, child)
	w.births++
	// How many of the enemies the world has are its own rather than arrivals
	// (stage 58): what is born starts where its parents were, so the map's
	// weighting says nothing about it.
	if child.Species == SpeciesEnemy {
		w.enemyBorn++
	}
}

func (w *World) kill(a *Agent) {
	a.Alive = false
	w.deaths++
	// Which block of the world it died in, for the measurement (stage 58).
	// Counted for both kinds: what a dangerous region costs is a question
	// about everything living in it.
	toll := w.tollAt(a.X, a.Y)
	if toll != nil {
		toll.deaths++
	}
	// Whatever it was holding falls where it fell (stage 40). Food carried
	// out of the world would be a leak in a total the world has kept fixed
	// since stage 15a.
	w.dropCarried(a)
	if a.Maturity < 1 {
		w.childDeaths++
	}
	w.dropMeat(a)
	// A body the river took is the river's, even if somebody had been hitting
	// it a moment before. The buckets have to stay exclusive: what is read as
	// starvation is everything the other counters do not claim.
	// Which deaths carrying could have answered (stage 40, #67). The buckets
	// here are the ones cmd/experiment reads, so starving is what nothing
	// else claims - and among those, the ones that had food in sight lately
	// are the target. Read only: nothing in the world turns on it.
	if !a.drowned && a.lastAttackTick < w.tick-1 && a.Lifespan > 0 {
		w.starvedDeaths++
		if a.sawFoodTick > 0 && float64(w.tick-a.sawFoodTick) <= w.cfg.PlanHorizon {
			w.starvedFoodSeen++
		}
		if a.sawFoodTick > 0 && float64(w.tick-a.sawFoodTick) <= w.cfg.PlanHorizon/7 {
			w.starvedFoodNear++
		}
	}
	if !a.drowned && a.lastAttackTick >= w.tick-1 {
		w.kills++
		if toll != nil {
			toll.kills++
		}
		// What the people who were standing there make of it (stage 31).
		// Before the body is taken out of the world, for the same reason a
		// drowning is told to the bank while the victim is still in the
		// water: the onlookers are the ones who can see it where it is.
		//
		// Who did it is not a new rule - it is the same list the carcass is
		// shared out to (recentAttackers), before the filter that keeps only
		// those who can eat it, because a human that kills another human is
		// still the one who killed it.
		w.witnessKill(a, a.recentAttackers(w.tick, w.cfg.HuntCreditTicks))
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
	// If it came out of a cache, that is worth noticing (stage 50): it keeps
	// the place fresh in this body's mind and teaches whoever saw it happen.
	// Taking is not a rule of its own - this is eating, by the ordinary path.
	w.tookFromStore(a, f)
	// Part of it goes to the children it is rearing, if the rule is on
	// (provision.go). What is divided is the mouthful, not the item: nothing
	// about owning, racing for or fighting over food changes.
	kept := w.share(a, f)
	hungerBefore := a.Hunger
	// Worth less if it is the same as everything else it has been living on
	// (stage 16). Nothing else changes: hunger falls by less, and everything
	// downstream of hunger follows from that on its own.
	a.Hunger = math.Max(0, a.Hunger-kept*w.cfg.FoodNutrition*w.dietValue(a, f.Kind)*w.meatWorth(f.Kind))
	// And whatever it was defended with (stage 17b). The plant's poison is a
	// hidden parameter: this is where an agent finds out what it actually ate,
	// as against what the warning said.
	if w.cfg.PlantDefence && f.Kind == FoodPlant {
		dose := kept * f.Genes.Poison * w.cfg.PoisonDamage * (1 - w.poisonResist(a))
		a.Vitality -= dose
		w.poisonLoss += dose
		// And whether the bite failed: a poisonous plant may be spat out and
		// left standing (2026-09-08). The eater has taken the dose and got
		// nothing for it, and nothing is added to the world's food - the
		// plant that is still there is the one that was already there.
		//
		// This is the only way poison can be selected for. Before it, an
		// eaten plant was gone whatever it carried, so being poisonous did
		// nothing for the plant and the gene drifted.
		if w.cfg.PlantPoisonSaves > 0 &&
			w.rng.Float64() < f.Genes.Poison*w.cfg.PlantPoisonSaves {
			a.Hunger = hungerBefore
			w.plantsSpat++
			return
		}
	}
	// And what a carcass mends (stage 39). This is the only food that gives
	// vitality back directly; with MeatVitality at zero nothing here fires
	// and the world is the one that came before.
	//
	// After the bite that fails, not before it: nothing meat carries can be
	// spat out today, but a mouthful that was never swallowed must not mend
	// anybody the day something can.
	// What the mouthful actually did, for a body's sense of how things are
	// going (stage 54). After the bite that fails, for the same reason the
	// mending is: a plant that was spat out fed nobody.
	w.please(a, hungerBefore-a.Hunger, 0)
	w.mend(a, f, kept)
	if f.Cooked > 0 {
		w.cookedEaten++
	}
	if f.Kind == FoodFish {
		w.fishEaten++
	}
	if f.Kind == FoodMeat {
		w.meatEaten++
		// And whether the eater was one of those who brought it down, or
		// somebody who came upon what was left (stage 41).
		if f.heldBy(a.ID) {
			w.meatEatenHeld++
		} else {
			w.meatEatenFree++
		}
	} else {
		w.plantsEaten++
	}
	w.noteEaten(a, f.Kind)
	// Some of what it swallows lives through the journey (stage 17c).
	w.noteSeedEaten(a, f)
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
	w := clamp(cfg.FitnessConditionWeight, 0, 1)
	return a.Gene(GeneAttractiveness)*(1-w) + condition*w
}

func (w *World) perceivedFitness(observer, target *Agent) float64 {
	return fitness(target, &w.cfg) + w.judgementError(observer, w.cfg.JudgementNoise*0.5)
}

// patienceTicks is how long an agent keeps comparing candidates before it is
// willing to settle. A rational agent can afford to wait and compare longer.
func (w *World) patienceTicks(a *Agent) int {
	return int(w.cfg.PatienceBase + a.Rationality(&w.cfg)*w.cfg.PatienceRationality)
}

// willCommit reports whether an agent accepts the candidate in front of it.
//
// Waiting lowers what it will settle for, from CommitFitness to CommitFloor -
// and CommitFloor is where the lowering stops. Before stage 26 the bar did not
// drop, it vanished: past its patience an agent accepted anybody at all,
// including somebody too worn out to see a birth through.
//
// That override was the one place a candidate's condition stopped counting,
// and condition is a third of how good a mate looks (FitnessConditionWeight).
// So the floor is not a new rule about health - it is what lets the rule that
// was already there reach the agents who have been waiting.
//
// CommitFloor = 0 is the world as it was, exactly: every fitness clears zero.
func (w *World) willCommit(a *Agent, candidateFitness float64) bool {
	return candidateFitness >= w.commitBar(a)
}

// commitBar is what this agent is holding out for right now: the full figure
// while it is still comparing, the floor once its patience has run out.
func (w *World) commitBar(a *Agent) float64 {
	if w.tick-a.courtStartTick >= w.patienceTicks(a) {
		return w.cfg.CommitFloor
	}
	return w.cfg.CommitFitness
}

// --- neighbourhood queries -------------------------------------------------
//
// nearestFoodInSight returns the index of the closest food item within
// perception range, or -1.
//
// Ties go to the last item examined, as they did when this walked the whole
// list, which is why the candidates have to arrive in the order they sit in.
func (w *World) nearestFoodInSight(a *Agent) int {
	best, bestDist := -1, math.Inf(1)
	w.nearScratch = w.appendFoodsInSight(w.nearScratch[:0], a.X, a.Y)
	for _, i := range w.nearScratch {
		f := &w.foods[i]
		if !w.canSee(a.X, a.Y, f.X, f.Y) {
			continue
		}
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
	w.nearScratch = w.appendAgentsInSight(w.nearScratch[:0], a.X, a.Y)
	for _, i := range w.nearScratch {
		o := &w.agents[i]
		if !o.Alive || o.ID == a.ID || o.Species != a.Species || o.Sex == a.Sex ||
			o.PartnerID != 0 || a.isRejected(o.ID) {
			continue
		}
		if !w.canSee(a.X, a.Y, o.X, o.Y) {
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
	stepX, stepY := dx/d*speed, dy/d*speed
	a.VX, a.VY = dx/d, dy/d

	// Where the ground lets it go. A step that would climb or drop a level
	// anywhere but a ramp is refused, and a body refused head-on tries the two
	// halves of the step in turn - which is what walking along the foot of a
	// cliff looks like, and what stops one becoming flypaper. A body that can
	// go nowhere stands still and pays nothing; nothing is asked of it, and
	// the idle trigger will get round to it like any other agent with nothing
	// happening (terrain.go).
	moved := false
	switch {
	case w.canStep(a.X, a.Y, a.X+stepX, a.Y+stepY):
		a.X, a.Y, moved = a.X+stepX, a.Y+stepY, true
	case stepX != 0 && w.canStep(a.X, a.Y, a.X+stepX, a.Y):
		a.X, moved = a.X+stepX, true
	case stepY != 0 && w.canStep(a.X, a.Y, a.X, a.Y+stepY):
		a.Y, moved = a.Y+stepY, true
	}
	if !moved {
		return
	}
	// Charged for the ground it is standing on at the end of the step, which
	// is the ground it spent the tick crossing.
	a.Vitality -= w.moveCostOn(a, a.X, a.Y, effort)
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
// newAgent builds one. Maturity is a parameter rather than a default because
// it decides how big the body is, and the vitality it starts with is a share
// of that: a newborn has to be small from its first tick, and a founder has to
// be grown from its first tick.
func (w *World) newAgent(x, y float64, sex Sex, genome []float64, generation int, maturity float64) Agent {
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
		Maturity:   maturity,
	}
	a.Vitality = w.cfg.ChildVitalityShare * a.MaxVitality(&w.cfg)
	return a
}

func (w *World) randomAgent(species Species) Agent {
	// Where it turns up. Enemies arrive where the map says they do (stage
	// 58); nothing else in the world comes in from outside.
	// Which sort of enemy this is (stage 59), before anything else: where it
	// turns up and what it is built like both come off its row. A world with
	// one sort chooses nothing and so draws nothing.
	kind := 0
	if species == SpeciesEnemy {
		kind = w.pickEnemyKind()
	}
	x, y := w.spawnSpotFor(species, kind)
	if species == SpeciesEnemy {
		w.enemyArrivals++
		w.enemyArrivalSum += w.prowlAt(x, y)
		for len(w.enemyArrivalsByKind) <= kind {
			w.enemyArrivalsByKind = append(w.enemyArrivalsByKind, 0)
		}
		w.enemyArrivalsByKind[kind]++
	}
	a := w.newAgent(
		x,
		y,
		w.randomSex(),
		w.drawGenomeOf(species, kind),
		0,
		1, // whoever the world puts in from outside arrives grown
	)
	a.Species = species
	a.Kind = uint8(kind)
	a.lore = w.newLore()
	a.chronotype = w.drawChronotype()
	a.hintSlots = w.drawHintSlots()
	a.hints = w.drawHints(a.hintSlots)
	// And whatever the country it arrived in has to teach (stage 38a). The
	// same rule a newborn gets, applied to where the world put it: nobody
	// draws a skill out of nothing, so a flat world never contains one.
	w.learnFromBirthplace(&a)
	// Room for ideas comes out of the same budget the body does, for founders
	// as for everybody else.
	fitBudget(a.Genome, a.Budget()-w.hintCost(a.hintSlots))
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
	// Likewise for what it assumes: an agent built as a literal, which is
	// almost always a test, gets the world's own figures rather than a set of
	// zeroes that would have it believe nobody ever hits back. It is given
	// them straight, without the founder's spread, so that handing the world
	// an agent draws nothing from the random source.
	if a.lore.unset() {
		a.lore = w.plainLore()
	}
	// And what the ground where it arrived does to it (stage 57), so that a
	// body that is read before the world has taken a step is read right.
	if w.groundHasAnOpinion() {
		w.footOn(&a)
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

// spawnEnemyOfTick lets one enemy in from outside the map now and then. See
// Config.EnemySpawnTicks for why the world does this rather than leaving the
// predators entirely to their own breeding.
func (w *World) spawnEnemyOfTick() {
	if w.cfg.EnemySpawnTicks <= 0 || w.tick%w.cfg.EnemySpawnTicks != 0 {
		return
	}
	n := 0
	for i := range w.agents {
		if w.agents[i].Species == SpeciesEnemy {
			n++
		}
	}
	if n >= w.cfg.MaxEnemies {
		return
	}
	w.addAgent(w.randomAgent(SpeciesEnemy))
}

func (w *World) spawnFood() {
	// Checked before drawing the position so that a full world does not consume
	// randomness and shift the rest of the run.
	//
	// Stones do not count against it (stage 45), and neither does money
	// (stage 51). They are in the same list because that list is "things
	// lying about", and a world given a lot of them grew nothing at all the
	// first time the stones went in - which is the bug this line is here to
	// have already fixed by the time the coins arrived.
	//
	// Everything else in the list still does, carcasses included. That is
	// older than this stage and looks like an oversight - the two allowances
	// are meant to be separate (see MaxMeatItems) - but it is the world every
	// figure in HISTORY.md was measured in, so it stays until it is changed
	// on purpose and measured.
	if len(w.foods)-w.countKind(FoodStone)-w.countKind(FoodCoin) >= w.cfg.MaxFoodItems {
		return
	}
	// One of them comes up in the water instead (stage 42). It is asked first
	// and it takes the place of this planting rather than adding to it, which
	// is what keeps how much the world grows FoodSpawnRate's business alone.
	// A world with no water says no without drawing anything.
	if w.spawnFish() {
		return
	}

	// A seed that has been carried somewhere first, if one is waiting. It
	// takes the place of this planting rather than adding to it, which is what
	// keeps the count of plants FoodSpawnRate's business alone.
	if s, ok := w.takePendingSeed(); ok {
		w.addPlant(clamp(s.x, 10, w.cfg.Width-10), clamp(s.y, 10, w.cfg.Height-10),
			w.inheritPlantGenes(s.genes))
		return
	}

	// A plant's defences are inherited even when its whereabouts are not: the
	// parent is drawn uniformly from what is still standing, so staying
	// uneaten is the only way to be picked more often (stage 17b).
	defended := func(x, y float64) {
		genes := w.drawPlantGenes()
		if w.cfg.PlantDefence {
			if parent, ok := w.defenceParent(); ok {
				inherited := w.inheritPlantGenes(parent)
				genes.Poison, genes.Signal = inherited.Poison, inherited.Signal
			}
		}
		w.addPlant(x, y, genes)
	}

	// A seedling of whichever plant the world picked to seed from, which is
	// weighted by how readily each seeds and by how good its ground is. How
	// many appear is untouched - FoodSpawnRate still says that - and all this
	// decides is whose child it is and where it lands.
	//
	// With nothing left to seed from, the world falls back to putting one
	// wherever the ground is good. Otherwise a world that lost its last plant
	// could never grow another, which is an absorbing state and not a rule
	// anybody chose.
	if w.cfg.PlantGenetics {
		if i := w.seedFrom(); i >= 0 {
			parent := &w.foods[i]
			genes := w.inheritPlantGenes(parent.Genes)
			angle := w.rng.Float64() * 2 * math.Pi
			// Uniform over the disc rather than over the radius, or seeds
			// would pile up at the centre of every parent's reach.
			d := genes.Spread * math.Sqrt(w.rng.Float64())
			w.addPlant(
				clamp(parent.X+math.Cos(angle)*d, 10, w.cfg.Width-10),
				clamp(parent.Y+math.Sin(angle)*d, 10, w.cfg.Height-10),
				genes)
			return
		}
	}

	if w.cfg.FoodSpread <= 0 {
		defended(w.randRange(10, w.cfg.Width-10), w.randRange(10, w.cfg.Height-10))
		return
	}
	// Where a plant comes up is drawn in proportion to how well the ground
	// grows things, and then uniformly inside that block. How many come up is
	// untouched: FoodSpawnRate still says how many, and this only says where.
	minX, minY, maxX, maxY := w.regionBounds(w.pickFoodRegion())
	defended(
		w.randRange(math.Max(minX, 10), math.Min(maxX, w.cfg.Width-10)),
		w.randRange(math.Max(minY, 10), math.Min(maxY, w.cfg.Height-10)))
}

// addFood grows a plant at the given position and returns its ID, or 0 when
// the world already holds as many plants as it may.
func (w *World) addFood(x, y float64) int {
	return w.addPlant(x, y, w.drawPlantGenes())
}

// addPlant is addFood for a plant whose parentage is already decided.
func (w *World) addPlant(x, y float64, genes plantGenes) int {
	if w.countKind(FoodPlant) >= w.cfg.MaxFoodItems {
		return 0
	}
	// Whether this one is the awkward crop (stage 44). It is decided where it
	// comes up, because that is the whole of what makes the crop a local
	// thing - and a world that grows none of it draws nothing deciding.
	special := false
	if share := w.specialShareAt(w.regionIndexAt(x, y)); share > 0 {
		special = w.rng.Float64() < share
	}
	return w.putFood(Food{X: x, Y: y, Kind: FoodPlant, Genes: genes, Special: special})
}

// countKind is how many items of one kind are lying about. The two kinds have
// separate allowances, so this is asked before either is added.
func (w *World) countKind(kind FoodKind) int {
	n := 0
	if !w.cfg.CarryOffTheBooks {
		n = w.heldKind[kind]
	}
	for i := range w.foods {
		if w.foods[i].Kind == kind {
			n++
		}
	}
	return n
}

// growingFood is how many of the things lying about are food that grew there:
// plants and fish, which share one allowance because a fish is a plant that
// did not come up.
func (w *World) growingFood() int {
	return w.countKind(FoodPlant) + w.countKind(FoodFish)
}

// kindAllowance is how many items of a kind the world will hold at once.
// Plants and carcasses have separate ones: sharing an allowance meant a spell
// of heavy dying filled it with meat and no plant could grow.
func (w *World) kindAllowance(kind FoodKind) int {
	if kind == FoodMeat {
		return w.cfg.MaxMeatItems
	}
	return w.cfg.MaxFoodItems
}

func (w *World) putFood(f Food) int {
	f.ID = w.nextFoodID
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

// moveFood puts an item somewhere else. Only a fish that got away uses it
// (stage 43): food does not otherwise move, and the spatial index has to be
// told, because everything that asks it where things are would otherwise be
// answering about where this one used to be.
func (w *World) moveFood(id int, x, y float64) {
	i, ok := w.foodIndex[id]
	if !ok {
		return
	}
	w.foods[i].X, w.foods[i].Y = x, y
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
