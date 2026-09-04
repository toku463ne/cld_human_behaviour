// Command experiment runs the simulation headless and compares variants of it.
//
// It answers the question cmd/devview cannot: watching a run shows that the
// world holds together, but not whether a rule change moved the selection
// pressure on an ability by +3 or by -2. That takes many seeds and arithmetic.
//
// Every variant is run on the same set of seeds, so the comparison is paired:
// the reported difference is the average of the per seed differences, which
// cancels out the enormous seed to seed variation and needs far fewer runs than
// comparing two independent groups would.
//
//	go run ./cmd/experiment -variants baseline,nogate -seeds 16 -ticks 20000
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/toku463ne/cld_human_behaviour/engine"
)

// A variant is one arm of an experiment: a name, why it exists, and what it
// changes about the default configuration.
type variant struct {
	name  string
	about string
	apply func(*engine.Config)
}

// The arms available. New rules under test get an entry here rather than a
// branch in the engine, so that both arms live in the same binary and can be
// run against the same seeds.
var variants = []variant{
	{
		name:  "baseline",
		about: "the current defaults",
		apply: func(*engine.Config) {},
	},
	{
		name:  "nogate",
		about: "strategy depth gate off; intelligence acts through ChoiceNoise alone",
		apply: func(c *engine.Config) { c.StrategyDepthUnlock = 0 },
	},
	{
		name:  "gate20",
		about: "depth gate spaced to 20/40/60, so the top level lands inside the range abilities occupy",
		apply: func(c *engine.Config) { c.StrategyDepthUnlock = 20 },
	},
	{
		name:  "noquality",
		about: "choice noise off; intelligence acts through the depth gate alone",
		apply: func(c *engine.Config) { c.ChoiceNoise = 0 },
	},
	// The two food arms are the control the cooperation work needs. Killing
	// falls whenever food is easier to come by, so a rule that both feeds the
	// world and makes agents group up would look like cooperation without
	// being any. These say how much of a fall plain calories buy.
	{
		name:  "morefood",
		about: "food spawning at 0.30 instead of 0.20: how much of the killing is simply scarcity",
		apply: func(c *engine.Config) { c.FoodSpawnRate = 0.30 },
	},
	{
		name:  "scarce",
		about: "food spawning at 0.12: the same question from the other side",
		apply: func(c *engine.Config) { c.FoodSpawnRate = 0.12 },
	},
	// The shape of mutation, at the same variance injected per birth
	// (MutationRate x MutationStd^2 = 16 either way). Rare and large is the
	// default because it leaves a parent's number intact most of the time,
	// which is what taking a whole value from one parent is for; this arm is
	// the constant nudge it replaced.
	{
		name:  "jitter",
		about: "mutation on every gene at std 4 instead of 1% of them at std 40",
		apply: func(c *engine.Config) { c.MutationRate, c.MutationStd = 1, 4 },
	},
	// The calibration arms. The default was set by measuring these: matching
	// the nominal variance injected per birth (rate x std^2 = 16, which is
	// 1% at std 40) left the standing spread about 15% short of the jitter
	// world, because a large jump from anywhere near the middle of the range
	// is clipped at 1 or 100.
	{
		name:  "jump1",
		about: "mutation at 1% instead of 2%: the calibration by nominal variance, which came out short",
		apply: func(c *engine.Config) { c.MutationRate = 0.01 },
	},
	{
		name:  "jump5",
		about: "mutation five times as often (5%) at std 18: smaller jumps, less of them clipped",
		apply: func(c *engine.Config) { c.MutationRate, c.MutationStd = 0.05, 18 },
	},
	// The budget arms. Heritability 0 draws a fresh budget at every birth,
	// which is how the world worked before budgets were inherited, and is the
	// control for whether the average budget drifts upwards once it can.
	{
		name:  "nobudgetinherit",
		about: "budget drawn afresh every birth instead of inherited from a parent",
		apply: func(c *engine.Config) { c.BudgetHeritability = 0 },
	},
	{
		name:  "narrowbudget",
		about: "budget inherited with a quarter of the usual wobble",
		apply: func(c *engine.Config) { c.BudgetInheritSpread = 7.5 },
	},
	{
		name:  "widebudget",
		about: "budget inherited with twice the usual wobble",
		apply: func(c *engine.Config) { c.BudgetInheritSpread = 60 },
	},
	// How lopsided the founders are. The allocation is only drawn once, for
	// the first generation, so these arms ask whether that draw still shows
	// after twenty thousand ticks of selection and mutation.
	// The one dial on the direction of sexual selection: how much of looking
	// like a good mate is the state somebody is in rather than the gene they
	// are advertising.
	{
		name:  "looksonly",
		about: "mates judged on the attractiveness gene alone, condition ignored",
		apply: func(c *engine.Config) { c.FitnessConditionWeight = 0 },
	},
	{
		name:  "conditionheavy",
		about: "mates judged mostly on condition: the looks gene is a third of it",
		apply: func(c *engine.Config) { c.FitnessConditionWeight = 0.7 },
	},
	{
		name:  "lopsidedfounders",
		about: "founders drawn with Dirichlet alpha 0.3: most of the budget on a few genes",
		apply: func(c *engine.Config) { c.GeneInitAlpha = 0.3 },
	},
	{
		name:  "evenfounders",
		about: "founders drawn with Dirichlet alpha 2.0: nine near-equal shares",
		apply: func(c *engine.Config) { c.GeneInitAlpha = 2.0 },
	},
	// The budget pushes attack to the ceiling, which doubles the damage a
	// typical blow does compared with the world these constants were tuned
	// in. These arms ask what that is worth undoing.
	// How much of a body there is to go round. The starting point of 360 lets
	// an agent hold the ceiling in attack and still have a working body, which
	// is what makes attack the only purchase worth making.
	// What a big body costs to run. Without it the budget ratchets upwards
	// for ever, because every gene is worth more than it costs; these arms are
	// how the price was set.
	{
		name:  "noupkeep",
		about: "a large body costs no more to run than a small one",
		apply: func(c *engine.Config) { c.BudgetUpkeep = 0 },
	},
	{
		name:  "halfupkeep",
		about: "half the upkeep for being made of more than average",
		apply: func(c *engine.Config) { c.BudgetUpkeep = 0.5 },
	},
	{
		name:  "pullback",
		about: "budget inherited at 0.9, the last tenth pulled back towards the average",
		apply: func(c *engine.Config) { c.BudgetHeritability = 0.9 },
	},
	{
		name:  "pullback95",
		about: "budget inherited at 0.95",
		apply: func(c *engine.Config) { c.BudgetHeritability = 0.95 },
	},
	{
		name:  "leanbudget",
		about: "GeneBudgetMean 270 instead of 360: thirty a gene rather than forty",
		apply: func(c *engine.Config) { c.GeneBudgetMean = 270 },
	},
	{
		name:  "fatbudget",
		about: "GeneBudgetMean 450",
		apply: func(c *engine.Config) { c.GeneBudgetMean = 450 },
	},
	{
		name:  "softblows",
		about: "AttackDamage cut to 0.72, so a typical blow lands as it did before the budget",
		apply: func(c *engine.Config) { c.AttackDamage = 0.72 },
	},
	// The control for the pack hunting question: carcasses, claims and
	// enemies all stay, but killing something is no longer worth a meal.
	// Whatever party size turns up here is what agents do to each other
	// anyway, and only the difference is hunting.
	// The channels of stage 8. Without them a fight is decided by who pours
	// more into hitting, which is what made attack the only gene worth
	// buying; these arms take the two answers to that away again.
	{
		name:  "fewerenemies",
		about: "enemies arrive every 500 ticks instead of 400: what the world was before the channels",
		apply: func(c *engine.Config) { c.EnemySpawnTicks = 500 },
	},
	{
		name:  "noguard",
		about: "guarding turns nothing aside",
		apply: func(c *engine.Config) { c.DefenceCap = 0 },
	},
	{
		name:  "nododge",
		about: "no blow ever misses",
		apply: func(c *engine.Config) { c.EvasionCap = 0 },
	},
	{
		name:  "nochannels",
		about: "neither guarding nor dodging does anything: the world as stage 7c left it",
		apply: func(c *engine.Config) { c.DefenceCap, c.EvasionCap = 0, 0 },
	},
	// How big the other species is. The size distribution is the one thing
	// stage 11 left undecided, and what it has to answer is whether a party
	// of two ever beats hunting alone: the carcass grows with the budget
	// while what one agent can bring down does not.
	// Stage 10: what a stranger is assumed to be. "nolooks" is the world
	// before it - everybody worth PriorStrength (50) whatever they looked
	// like - and is the arm the stage is measured against. "flatlooks" keeps
	// the learning but flattens the line, which separates learning what the
	// world averages to from being able to read one body in it.
	{
		name:  "nolooks",
		about: "a stranger is worth the flat prior however they look: the world before stage 10",
		apply: func(c *engine.Config) { c.LearnFromLooks = false },
	},
	// Stage 12a: the figures the utility formula leans on stop being the same
	// for everybody. "nolore" is the world before it - one set of numbers,
	// right by construction - and it is the arm the stage is measured against.
	// The others take the halves apart: whether an agent may want different
	// things from its neighbour, and whether it may find the world out.
	{
		name:  "nolore",
		about: "everybody assumes exactly the same and learns nothing: the world before stage 12a",
		apply: func(c *engine.Config) {
			c.LearningRate, c.LoreInitSpread, c.LoreMutationStd = 0, 0, 0
		},
	},
	{
		name:  "onepreference",
		about: "everybody wants the same things; the preferences do not vary or mutate",
		apply: func(c *engine.Config) { c.LoreInitSpread, c.LoreMutationStd = 0, 0 },
	},
	{
		// The costly one, and the reason learning is off by default. It works
		// - the belief lands far closer to what the world actually does - and
		// the world cannot afford what it finds out.
		name:  "learning",
		about: "agents find out how often the one you hit hits back, instead of assuming 0.7",
		apply: func(c *engine.Config) { c.LearningRate = 1 },
	},
	{
		name:  "lamarck",
		about: "learning on, and a child starts where its parent's beliefs got to",
		apply: func(c *engine.Config) { c.LearningRate, c.LamarckRate = 1, 1 },
	},
	// Stage 12b: what agents assume becomes tradeable. "no12b" is the world
	// before it - the values are still each agent's own, but nothing hands
	// them on and nobody seeks anybody out for them. The other two take the
	// halves apart: the trade happening, and it being worth going after.
	// The groundwork for terrain: a race for food is judged on who would
	// arrive first rather than on who is nearer, so how fast a body is and how
	// hard it is trying both count. "racedistance" is the world before it.
	// The sighting triggers. "resighting" is how they behaved before the edge
	// was put in: fire on every tick something is in view rather than on the
	// tick it comes into view. It costs 2.4x the simulation, and this says
	// what it bought.
	{
		name:  "resighting",
		about: "an agent reconsiders every tick food is in view, not just when it arrives",
		apply: func(c *engine.Config) { c.SightingRetriggers = true },
	},
	// Stage 18: the world gets a day, and not everybody keeps the same hours.
	// "oneclock" is the arm the stage is measured against - a world with a day
	// in it and no disagreement about when to sleep - and "noclock" is the
	// world from before there was a day at all.
	{
		name:  "noclock",
		about: "the world has no day: the world before stage 18",
		apply: func(c *engine.Config) { c.TicksPerDay = 0 },
	},
	{
		name:  "oneclock",
		about: "a day, and everybody sleeps at the same hour of it",
		apply: func(c *engine.Config) { c.ChronotypeSpread = 0 },
	},
	{
		name:  "sleepingwatch",
		about: "a friend keeps watch even while asleep: what is being awake worth?",
		apply: func(c *engine.Config) { c.SleepingWatch = true },
	},
	{
		name:  "longday",
		about: "sweep: a day of four thousand ticks rather than one thousand",
		apply: func(c *engine.Config) { c.TicksPerDay = 4000 },
	},
	// Stage 17a: the plants inherit where they came from. "noplantgenes" is
	// the world stage 15a left, where a plant appeared wherever the ground was
	// good and had no parent at all.
	{
		name:  "noplantgenes",
		about: "plants appear wherever the ground is good and pass nothing on: the world before stage 17a",
		apply: func(c *engine.Config) { c.PlantGenetics = false },
	},
	{
		name:  "plantdefence",
		about: "plants carry poison and a warning about it (stage 17b, off by default)",
		apply: func(c *engine.Config) { c.PlantDefence = true },
	},
	{
		name:  "blindtowarnings",
		about: "the warnings are there and unreadable: what is reading them worth?",
		apply: func(c *engine.Config) { c.PlantDefence, c.SignalNoise = true, 4 },
	},
	{
		name:  "poison8",
		about: "sweep: a full dose costs 8 of vitality rather than 18",
		apply: func(c *engine.Config) { c.PlantDefence, c.PoisonDamage = true, 8 },
	},
	{
		name:  "poison4",
		about: "sweep: a full dose costs 4 of vitality",
		apply: func(c *engine.Config) { c.PlantDefence, c.PoisonDamage = true, 4 },
	},
	{
		name:  "poison2",
		about: "sweep: a full dose costs 2 of vitality",
		apply: func(c *engine.Config) { c.PlantDefence, c.PoisonDamage = true, 2 },
	},
	{
		name:  "noseedcarry",
		about: "nothing survives being eaten: wind dispersal alone (17a without 17c)",
		apply: func(c *engine.Config) { c.SeedSurvival = 0 },
	},
	{
		name:  "carryonly",
		about: "seeds only travel in animals; the plants themselves throw nothing far",
		apply: func(c *engine.Config) { c.PlantSpread, c.PlantSpreadMax = 20, 40 },
	},
	{
		name:  "shortseed",
		about: "sweep: seeds land close to the parent and cannot evolve far",
		apply: func(c *engine.Config) { c.PlantSpread, c.PlantSpreadMax = 30, 60 },
	},
	{
		name:  "longseed",
		about: "sweep: seeds are thrown across the world from the start",
		apply: func(c *engine.Config) { c.PlantSpread = 300 },
	},
	{
		name:  "frozenplants",
		about: "plants have genes but never mutate: the shape without the evolution",
		apply: func(c *engine.Config) { c.PlantMutationRate = 0 },
	},
	// Stage 16: living on one thing is worth progressively less. Measured on
	// its own and expected to do almost nothing, because a human's food is
	// plants near enough always - this is the baseline for when stage 17
	// gives them something to choose between.
	{
		name:  "nodietrule",
		about: "no penalty for living on one thing: the world before stage 16",
		apply: func(c *engine.Config) { c.SamenessPenalty = 0 },
	},
	{
		name:  "harshdiet",
		about: "sweep: living on one thing costs most of what it is worth",
		apply: func(c *engine.Config) { c.SamenessPenalty = 0.8 },
	},
	// Stages 15b and 15c: agents learn what the ground they walk is like, and
	// trade it. "noregionlore" is the world 15a left - the food is still
	// uneven and agents still find it by standing on it, and that is all.
	{
		name:  "noregionlore",
		about: "agents never learn what the ground is like: the world after stage 15a and before 15b",
		apply: func(c *engine.Config) { c.RegionLearnRate = 0 },
	},
	{
		name:  "knownodraw",
		about: "agents learn the country and never act on it: what is knowing worth without choosing?",
		apply: func(c *engine.Config) { c.RegionDrawValue = 0 },
	},
	{
		name:  "notoldground",
		about: "agents learn the country but cannot hand it on: what does hearing about it add? (15c off)",
		apply: func(c *engine.Config) { c.RegionToldCount = 0 },
	},
	// Stage 15a: the regions differ in how well they grow plants. "noplantgap"
	// is the world before it, exactly. The amount of food is unchanged in
	// every arm - only where it comes up moves - because FoodSpawnRate is the
	// most selection-sensitive figure there is.
	{
		name:  "noplantgap",
		about: "plants come up evenly across the world: the world before stage 15a",
		apply: func(c *engine.Config) { c.FoodSpread = 0 },
	},
	{
		name:  "sharpplantgap",
		about: "sweep: the good ground grows far more than the bad",
		apply: func(c *engine.Config) { c.FoodSpread = 1 },
	},
	{
		// The control decision #39 asks for. The food is still uneven, but
		// every reason to seek other agents out is off: no positive term for
		// watching somebody you trust, and no discount for resting among
		// friends. Whatever gathering is left is the plants pulling, not
		// anything social.
		name:  "richnosocial",
		about: "uneven plants but no reason to seek anybody out: is gathering on good ground social at all?",
		apply: func(c *engine.Config) { c.LoreValue, c.AffinityTrust = 0, 0 },
	},
	// Stage 14: the world gets regions, and they differ in how exposed lying
	// down in them is. "noshelter" is the world before it, exactly - with no
	// spread nothing is drawn from the random source at all.
	{
		name:  "noshelter",
		about: "every region is ordinary ground: the world before stage 14",
		apply: func(c *engine.Config) { c.ShelterSpread = 0 },
	},
	{
		name:  "sharpshelter",
		about: "sweep: regions differ far more in how safe resting in them is",
		apply: func(c *engine.Config) { c.ShelterSpread = 1 },
	},
	// The stage 9 carry-over: memory capacity has never earned a positive
	// selection pressure (shMemory 0.07-0.08 at stages 9, 12 and 15b). The
	// diagnosis written down at 15b was that twelve regions is too few for
	// capacity to bind, since agents already know eight of them. These arms
	// test that, rather than testing it again in the same world.
	// Why extra memory is never worth buying. The suspicion is that contact
	// refresh (#22) already keeps every record that matters: an agent never
	// forgets anybody still standing near it, so more room only buys memories
	// of people who have gone, which are worth nothing. These arms take the
	// refreshing away and ask whether room starts paying.
	{
		name:  "staleandsmall",
		about: "no contact refresh, room for six: does forgetting the living start to hurt?",
		apply: func(c *engine.Config) { c.ContactRefresh, c.MemoryCapacity = false, 6 },
	},
	{
		name:  "staleandbig",
		about: "no contact refresh, room for twenty four: is room worth buying once it is needed?",
		apply: func(c *engine.Config) { c.ContactRefresh, c.MemoryCapacity = false, 24 },
	},
	{
		name:  "manyregions",
		about: "the world cut into 108 regions: far more country than a memory can hold",
		apply: func(c *engine.Config) { c.RegionCols, c.RegionRows = 12, 9 },
	},
	{
		name:  "shortcountry",
		about: "the country is forgotten ten times as fast: does holding it become worth paying for?",
		apply: func(c *engine.Config) { c.RegionForgetPerTick = 0.004 },
	},
	{
		name:  "manyshortregions",
		about: "both: a lot of country, forgotten fast",
		apply: func(c *engine.Config) {
			c.RegionCols, c.RegionRows = 12, 9
			c.RegionForgetPerTick = 0.004
		},
	},
	{
		name:  "fineregions",
		about: "sweep: the same world cut into 48 small regions rather than 12 big ones",
		apply: func(c *engine.Config) { c.RegionCols, c.RegionRows = 8, 6 },
	},
	// Stage 13: sight stops being a circle and becomes the cell an agent is
	// standing in plus the ring around it. "sightcircle" is the world before
	// it, and the two are calibrated to cover the same ground so that the
	// shape is the only difference.
	{
		name:  "sightcircle",
		about: "sight is a circle of 130 in every direction: the world before stage 13",
		apply: func(c *engine.Config) { c.SightGrid = false },
	},
	{
		name:  "sight2cells",
		about: "sweep: two rings of smaller cells, the same ground seen through a finer grid",
		apply: func(c *engine.Config) { c.SightCells, c.SightCellSize = 2, 46.1 },
	},
	{
		name:  "sightblind",
		about: "sweep: only the cell it is standing in, which is a ninth of the ground",
		apply: func(c *engine.Config) { c.SightCells, c.SightCellSize = 0, 76.8 },
	},
	{
		name:  "racedistance",
		about: "a race for food goes to whoever is nearer, whatever their legs: the rule before the terrain groundwork",
		apply: func(c *engine.Config) { c.RaceOnDistance = true },
	},
	{
		name:  "no12b",
		about: "nothing is handed on and nobody is sought out for it: the world before stage 12b",
		apply: func(c *engine.Config) { c.LoreExchangeRate, c.LoreValue = 0, 0 },
	},
	{
		name:  "notrust",
		about: "the trade happens when agents happen to stand together, but nobody seeks it out",
		apply: func(c *engine.Config) { c.LoreValue = 0 },
	},
	{
		name:  "noexchange",
		about: "agents seek each other out but nothing passes between them: the control for the positive term",
		apply: func(c *engine.Config) { c.LoreExchangeRate = 0 },
	},
	// Stage 12c: rules of thumb. "nohints" is the world before it, and it has
	// to be read as a pair with the rest - room for an idea costs budget, so
	// a world with hints in it is made of slightly smaller agents and the
	// absolute ability figures are not comparable to any earlier baseline.
	{
		name:  "nohints",
		about: "no room for rules of thumb and nothing charged for them: the world before stage 12c",
		apply: func(c *engine.Config) { c.HintSlots = 0 },
	},
	{
		name:  "hints10",
		about: "sweep: room for an idea costs 10 of budget rather than 5",
		apply: func(c *engine.Config) { c.HintSlotCost = 10 },
	},
	{
		name:  "dearhints",
		about: "sweep: room for an idea costs 15 of budget: where the population starts to pay",
		apply: func(c *engine.Config) { c.HintSlotCost = 15 },
	},
	{
		name:  "quiethints",
		about: "sweep: rules of thumb are drawn at half the weight",
		apply: func(c *engine.Config) { c.HintWeightStd, c.HintWeightMax = 3, 10 },
	},
	{
		name:  "loudhints",
		about: "sweep: rules of thumb are drawn at twice the weight",
		apply: func(c *engine.Config) { c.HintWeightStd, c.HintWeightMax = 12, 40 },
	},
	{
		name:  "nohintspread",
		about: "hints exist and are inherited but never copied: does a good trick spread sideways?",
		apply: func(c *engine.Config) { c.HintsSpread = false },
	},
	{
		// The control the stage turns on. Room is bought and charged for
		// exactly as it is in the default world, and the ideas in it say
		// nothing at all. Any difference from the default is what having
		// rules of thumb is worth, with the price already paid on both sides.
		name:  "deadhints",
		about: "rules of thumb cost the same and carry no weight: what are they worth net of what they cost?",
		apply: func(c *engine.Config) { c.HintWeightStd, c.HintWeightMax = 0, 0 },
	},
	{
		name:  "trust3",
		about: "sweep: watching somebody you trust is worth 3 rather than 9",
		apply: func(c *engine.Config) { c.LoreValue = 3 },
	},
	{
		name:  "trust6",
		about: "sweep: watching somebody you trust is worth 6 rather than 9",
		apply: func(c *engine.Config) { c.LoreValue = 6 },
	},
	{
		name:  "learning12b",
		about: "stage 12b in place and agents also find the world out for themselves",
		apply: func(c *engine.Config) { c.LearningRate = 1 },
	},
	{
		name:  "spread10",
		about: "sweep: preferences drawn 10% around the world's figure instead of 15%",
		apply: func(c *engine.Config) { c.LoreInitSpread = 0.10 },
	},
	{
		name:  "spread25",
		about: "sweep: preferences drawn 25% around: where the population starts to go",
		apply: func(c *engine.Config) { c.LoreInitSpread = 0.25 },
	},
	// What the assumption is worth as a plain number, with nobody learning
	// anything. The world's true retaliation rate is about 0.15, and these
	// say what the controller's 0.7 was buying: the population, all of it.
	{
		name:  "retal45",
		about: "sweep: everybody assumes 0.45 rather than 0.7, nobody learns",
		apply: func(c *engine.Config) {
			c.LoreInitSpread, c.LoreMutationStd = 0, 0
			c.Retaliation = 0.45
		},
	},
	{
		name:  "retal20",
		about: "sweep: everybody assumes the truth (0.2), nobody learns",
		apply: func(c *engine.Config) {
			c.LoreInitSpread, c.LoreMutationStd = 0, 0
			c.Retaliation = 0.2
		},
	},
	{
		// The diagnostic pair for where the population goes. A learned guess
		// about a stranger is lower than the flat 50 was, and the estimate of
		// a stranger is what the exposure of resting is made of, so the first
		// suspect is that agents simply rest more. Paired against
		// "norestexposure", which is the same world with the learning in it.
		name:  "restsafenolooks",
		about: "resting safe anywhere and no learning from looks: is the cost of learning all in the resting?",
		apply: func(c *engine.Config) { c.RestExposureWeight, c.LearnFromLooks = 0, false },
	},
	{
		// Learning splits into two things: what an agent comes to assume about
		// a stranger on average, and its being able to tell one stranger from
		// another. This arm has neither, but moves the flat prior to what the
		// learners settle on (about 36), which is how the two are told apart:
		// whatever this arm does to the world is the level, not the learning.
		name:  "lowprior",
		about: "no learning, but a stranger is assumed to be 36 rather than 50",
		apply: func(c *engine.Config) { c.LearnFromLooks, c.PriorStrength = false, 36 },
	},
	{
		name:  "flatlooks",
		about: "an agent learns what strengths around it average to, but not to read a build",
		apply: func(c *engine.Config) { c.LooksSlope = false },
	},
	{
		name:  "rawslope",
		about: "the fitted slope taken as it comes, with nothing pulling it back to zero",
		apply: func(c *engine.Config) { c.AppearanceSlopePrior = 0 },
	},
	{
		name:  "softslope",
		about: "sweep: the slope pulled back by 200 readings' worth of doubt instead of 60",
		apply: func(c *engine.Config) { c.AppearanceSlopePrior = 200 },
	},
	{
		name:  "quickslope",
		about: "sweep: pulled back by only 20 readings' worth",
		apply: func(c *engine.Config) { c.AppearanceSlopePrior = 20 },
	},
	{
		name:  "sharplooks",
		about: "a build is seen exactly: the ceiling of what appearance can be worth",
		apply: func(c *engine.Config) { c.AppearanceNoise = 0 },
	},
	{
		name:  "quicktrust",
		about: "sweep: an agent goes by its own line after 3 readings rather than 8",
		apply: func(c *engine.Config) { c.AppearanceMinReads = 3 },
	},
	{
		name:  "slowtrust",
		about: "sweep: after 20 readings",
		apply: func(c *engine.Config) { c.AppearanceMinReads = 20 },
	},
	{
		name:  "dulllooks",
		about: "sweep: a build misread by 12 rather than 6, which is enough to swamp it",
		apply: func(c *engine.Config) { c.AppearanceNoise = 12 },
	},
	{
		name:  "vaguelooks",
		about: "a build is as hard to read as a strength (noise 40)",
		apply: func(c *engine.Config) { c.AppearanceNoise = 40 },
	},
	{
		name:  "bigenemies",
		about: "enemies drawn at 700 instead of 520: a carcass worth nearly six of an ordinary body",
		apply: func(c *engine.Config) { c.EnemyBudgetMean = 700 },
	},
	{
		name:  "smallenemies",
		about: "enemies drawn at 400: barely more than a human",
		apply: func(c *engine.Config) { c.EnemyBudgetMean = 400 },
	},
	{
		name:  "variedenemies",
		about: "the same average enemy, drawn twice as widely (std 180): more of the very large ones",
		apply: func(c *engine.Config) { c.EnemyBudgetStd = 180 },
	},
	{
		name:  "noprey",
		about: "a carcass is worth nothing to whoever brings it down",
		apply: func(c *engine.Config) { c.PreyValue = 0 },
	},
	{
		name:  "noenemies",
		about: "the world without a second species at all",
		apply: func(c *engine.Config) { c.InitialEnemies, c.EnemySpawnTicks = 0, 0 },
	},
	{
		name:  "nomutation",
		about: "no new variation at all: selection works on what the founders had",
		apply: func(c *engine.Config) { c.MutationRate = 0 },
	},
	// Lifespan consumption (chronic starving/overfeeding wearing down
	// Lifespan in the background) is brand new and its defaults are an
	// untuned starting point: at the default MaxLifespan/rates it almost
	// never fires within a normal run, because the agents that would
	// otherwise accumulate enough bad ticks to hit zero are usually killed
	// or starved first. This arm shrinks the budget so the mechanism is
	// visible at all, as a lever for tuning it later.
	// The memory of stage 9. The world before it held an unbounded record of
	// everybody ever seen, forgot at one rate for everybody, and let an agent
	// lie down anywhere. "stage8" is all of that at once and is the arm the
	// stage as a whole is measured against; the others take one piece away.
	{
		name:  "stage8",
		about: "memory unbounded, nothing fresh kept by contact, resting safe anywhere: the world before stage 9",
		apply: func(c *engine.Config) {
			c.MemoryCapacity, c.MemoryBandwidthShare = 0, 0
			c.ContactRefresh = false
			c.AffinityPairBond, c.AffinityBirth, c.AffinityKin, c.AffinityHunt = 0, 0, 0, 0
			c.RestExposureWeight = 0
		},
	},
	{
		name:  "nomemorylimit",
		about: "an agent can hold everybody it has ever seen",
		apply: func(c *engine.Config) { c.MemoryCapacity, c.MemoryBandwidthShare = 0, 0 },
	},
	{
		name:  "smallmemory",
		about: "room for six faces instead of twelve",
		apply: func(c *engine.Config) { c.MemoryCapacity = 6 },
	},
	{
		name:  "bigmemory",
		about: "room for twenty four",
		apply: func(c *engine.Config) { c.MemoryCapacity = 24 },
	},
	{
		name:  "nocontactrefresh",
		about: "seeing somebody again does not keep the memory of them fresh",
		apply: func(c *engine.Config) { c.ContactRefresh = false },
	},
	{
		name:  "noaffinity",
		about: "nothing good is ever remembered about anybody",
		apply: func(c *engine.Config) {
			c.AffinityPairBond, c.AffinityBirth, c.AffinityKin, c.AffinityHunt = 0, 0, 0, 0
		},
	},
	// The last piece of stage 11: bringing a carcass down together is
	// remembered. "nohuntaffinity" is the world as stage 11 left it, and is
	// the arm the rule is measured against; the two sweeps are how the size
	// of the credit was set.
	{
		name:  "nohuntaffinity",
		about: "hunting something down together is not remembered: the world before the stage 11 remainder",
		apply: func(c *engine.Config) { c.AffinityHunt = 0 },
	},
	{
		// The diagnostic pair for what the credit actually does. Affinity buys
		// exactly one thing at this stage - who an agent will lie down next to
		// - so with rest exposure off the credit should buy nothing at all.
		// Paired against "norestexposure", which is the same world with the
		// credit still in it.
		name:  "restsafenohunt",
		about: "resting safe anywhere and no credit for a shared kill: is the credit felt anywhere but rest?",
		apply: func(c *engine.Config) { c.RestExposureWeight, c.AffinityHunt = 0, 0 },
	},
	{
		name:  "midhunt",
		about: "sweep: twice the credit for a shared kill (12), two of them enough to trust somebody",
		apply: func(c *engine.Config) { c.AffinityHunt = 12 },
	},
	{
		name:  "bighunt",
		about: "sweep: a shared kill worth more than a bond (24), one of them enough to trust somebody",
		apply: func(c *engine.Config) { c.AffinityHunt = 24 },
	},
	{
		name:  "norestexposure",
		about: "resting in the open is as safe as resting among your own",
		apply: func(c *engine.Config) { c.RestExposureWeight = 0 },
	},
	{
		name:  "hardrestexposure",
		about: "four times the price for lying down among strangers: where the population starts to go",
		apply: func(c *engine.Config) { c.RestExposureWeight = 0.20 },
	},
	// Stage 7d: growing up, wearing out, and keeping to a parent. "nochildhood"
	// is all of the growing-up half at once and is the arm the stage is
	// measured against; the others take one piece away.
	{
		name:  "nochildhood",
		about: "born fully grown, nobody has to wait to breed, no childcare: the world before stage 7d",
		apply: func(c *engine.Config) {
			c.ChildAbilityShare, c.ReproMaturity = 1, 0
			c.ChildRearingTicks = 0
		},
	},
	{
		name:  "strongchildren",
		about: "a newborn expresses six tenths of what it inherited instead of a third",
		apply: func(c *engine.Config) { c.ChildAbilityShare = 0.6 },
	},
	{
		name:  "longchildhood",
		about: "five years of eating well to grow up instead of three",
		apply: func(c *engine.Config) { c.ChildhoodYears = 5 },
	},
	{
		name:  "quickchildhood",
		about: "sweep: eighteen months of growing up",
		apply: func(c *engine.Config) { c.ChildhoodYears = 1.5 },
	},
	{
		name:  "earlybreeding",
		about: "sweep: offspring considered at seven tenths grown",
		apply: func(c *engine.Config) { c.ReproMaturity = 0.7 },
	},
	{
		name:  "mildchildhood",
		about: "sweep: half strength at birth, two years, breeding at 0.8",
		apply: func(c *engine.Config) {
			c.ChildAbilityShare, c.ChildhoodYears, c.ReproMaturity = 0.5, 2, 0.8
		},
	},
	{
		name:  "norearing",
		about: "children do not keep to a parent",
		apply: func(c *engine.Config) { c.ChildRearingTicks = 0 },
	},
	{
		name:  "wideleash",
		about: "sweep: a child may stray as far as it can see (130) rather than 45",
		apply: func(c *engine.Config) { c.RearingRadius = 130 },
	},
	{
		name:  "mild2",
		about: "sweep: 0.6 at birth, two years, breeding at 0.9, wide leash",
		apply: func(c *engine.Config) {
			c.ChildAbilityShare, c.ChildhoodYears, c.ReproMaturity = 0.6, 2, 0.9
			c.RearingRadius = 130
		},
	},
	{
		name:  "mild3",
		about: "sweep: 0.6 at birth, 1.5 years, breeding at 0.9, wide leash",
		apply: func(c *engine.Config) {
			c.ChildAbilityShare, c.ChildhoodYears, c.ReproMaturity = 0.6, 1.5, 0.9
			c.RearingRadius = 130
		},
	},
	{
		name:  "onethreshold",
		about: "the growth cap and the overfeeding cost at the same place: eating to the full costs nothing",
		apply: func(c *engine.Config) { c.OverfedHunger = 0 },
	},
	{
		name:  "growthonly",
		about: "childhood but no decline and no cost for being worn down",
		apply: func(c *engine.Config) { c.SenescenceRate, c.FrailLifespanRate = 0, 0 },
	},
	{
		name:  "nosenescence",
		about: "nobody ever declines with age",
		apply: func(c *engine.Config) { c.SenescenceRate = 0 },
	},
	{
		name:  "nofrailty",
		about: "a long spell of being worn down costs no lifespan",
		apply: func(c *engine.Config) { c.FrailLifespanRate = 0 },
	},
	{
		name:  "life4000",
		about: "sweep: MaxLifespan 4000",
		apply: func(c *engine.Config) { c.MaxLifespan = 4000 },
	},
	{
		name:  "life3000",
		about: "sweep: MaxLifespan 3000",
		apply: func(c *engine.Config) { c.MaxLifespan = 3000 },
	},
	{
		name:  "fastwear",
		about: "sweep: the same budget spent three times as fast",
		apply: func(c *engine.Config) {
			c.StarveLifespanRate, c.OverfedLifespanRate, c.FrailLifespanRate = 0.6, 0.6, 0.6
		},
	},
	{
		name:  "brittlelifespan",
		about: "MaxLifespan cut to 1500: how visible aging death becomes when the budget is tight",
		apply: func(c *engine.Config) { c.MaxLifespan = 1500 },
	},
}

func variantByName(name string) (variant, bool) {
	for _, v := range variants {
		if v.name == name {
			return v, true
		}
	}
	return variant{}, false
}

// --- what a run reports -----------------------------------------------------

// The measurements taken from a finished run, in the order they are printed.
//
// The three deltas are the ones the intelligence experiment turns on: how far
// an ability moved over the whole run is the selection pressure on it, and its
// sign is the thing that has flipped on a parameter change before.
// The layout and killing figures are the ones the cooperation work turns on:
// whether agents come together, and whether they stop killing each other. Both
// are measured before any rule is written for them, so that there is something
// to compare against afterwards.
var metricNames = []string{
	"pop", "gen", "births", "deaths", "starved", "killed", "killShare", "aged", "agedShare", "fights",
	"age", "maturity", "ageFactor", "childShare", "grewUp", "childDeathShare",
	"birthRate", "deathRate", "killRate", "starveRate", "fightRate",
	"clumping", "neighbours", "nearest",
	"clusters", "clusterSize", "grouped", "largestShare",
	"gap", "gapP10", "gapRel",
	"halfLife", "together", "censored",
	"fightCompanion", "fightStranger", "fightRatio",
	"species", "rareShare", "rareTrough", "rareSwing",
	"remembered", "friends", "memFull", "restNear",
	"power", "rationality", "intelligence",
	"sdPower", "sdRationality", "sdIntelligence",
	"dPower", "dRationality", "dIntelligence",
	"budget", "sdBudget", "dBudget",
	"shAttack", "shDefence", "shVitality", "shSpeed", "shEvasion",
	"shMemory", "shRationality", "shIntelligence", "shLooks",
	"geniuses", "greatGeniuses", "geniusYears", "greatGeniusYears",
	"priorErr", "priorErrFlat", "priorErrFixed", "slopeGain", "learnGain",
	"priorErrAll", "priorErrLearned", "priorErrGreen", "learnedShare", "firstSights",
	"hunts", "jointHunts", "packSize", "evadedShare",
	"flees", "escapeShare",
	"restShelter", "shelterAll", "shelterGain",
	"humanRich", "enemyRich", "richGain", "enemyRichGain",
	"regionKnown", "regionTold", "regionRank", "regionSpread",
	"dietVariety", "dietDiscount",
	"plantSpread", "plantRegrow", "plantClump", "plantEmpty", "seedsCarried",
	"plantPoison", "plantSignal", "plantHonesty",
	"allAsleep", "clockSpread",
	"retal", "trueRetal", "retalErr", "accept", "trueAccept", "acceptErr",
	"loreRate", "taught", "teachTop",
	"hintSlots", "hintsHeld", "hintKinds", "hintEntropy", "hintCopyRate",
	"riskWeight", "sdRiskWeight", "competition", "sdCompetition", "shock", "sdShock",
	"extinct",
}

type sample struct {
	tick                    int
	pop                     int
	power, rat, intel       float64
	sdPower, sdRat, sdIntel float64
	foods                   int
	births, deaths, fights  int

	// How the population is laid out at this moment.
	clumping, neighbours, nearest float64

	// How it is grouped: the number of clusters of two or more, their mean
	// size, the share of the population inside one, and the share inside the
	// biggest single one. The last is the check on the other three: single
	// linkage chains, so a share near 1 means the population has merged into
	// one blob rather than formed groups.
	clusters, clusterSize, grouped, largestShare float64

	// How far the groups keep from each other: the mean gap to the nearest
	// other group, the close approach end of that distribution, and the mean
	// again with the population density divided out.
	gap, gapP10, gapRel float64

	// What the population is made of: the mean budget, how varied it is, and
	// how the average agent splits it between the genes.
	//
	// The shares are what to compare between arms. A raw gene moves when the
	// budget moves, so a rise in attack could be a population that fights more
	// or one that is simply bigger; the share only moves when the trade
	// between the genes moves.
	budget, sdBudget float64
	shares           [engine.NumGenes]float64

	// How old the population is, how much of it has finished growing, and how
	// much of what they inherited they can express today.
	age, maturity, ageFactor, childShare float64

	// What the population's memory is doing: how many others the average agent
	// holds a record of, how many of those records carry affinity, the share of
	// agents that have run out of room, and how many agents it is not fond of
	// are standing over the ones that are resting.
	remembered, friends, memFull, restNear float64

	// What the population assumes: the mean of each of the five figures the
	// utility formula used to take from the config, and the spread of the
	// three that are preferences rather than claims about the world. The
	// spread is the one that matters - a mean says which way a population
	// leans, only a spread says whether there is anything left to select on.
	retal, accept                        float64
	riskWeight, competition, shock       float64
	sdRiskWeight, sdCompetition, sdShock float64

	// How the trading of assumptions is spread: trades per agent per thousand
	// ticks alive, and the share of it done by the busiest fifth. The second
	// is the one the design is on the hook for - a rule meant to spread
	// something around must not end up with three teachers and a crowd.
	taught, teachTop float64

	// Where the resting happens, against where the population is. If a good
	// place to lie down is worth anything, the first should be below the
	// second: shelter is a multiplier on what resting costs, so lower is
	// better ground.
	restShelter, shelterAll float64

	// Where each kind of creature stands, measured by how well the ground
	// grows plants. The predators are counted apart because they are drawn to
	// the humans rather than to the plants, so their figure should follow the
	// humans' with nothing of its own in it.
	humanRich, enemyRich, allRich float64

	// What the population has made of the ground: how many regions the
	// average agent has a view of, how many of those it was told about rather
	// than walked, how well the views line up with the truth, and how much
	// agents disagree about the same place.
	regionKnown, regionTold, regionRank, regionSpread float64

	// What the population is living on: how mixed the average diet is, and
	// what the average mouthful is actually worth after the discount for
	// sameness. A rule that never fires leaves the second at one.
	dietVariety, dietDiscount float64

	// What the plants have become: how far they throw a seed, how readily
	// they throw one, the thickets that makes, and the share of the world's
	// regions with nothing growing in them. The last is watched the way the
	// rarer species' trough is - nothing can seed where nothing grows, so an
	// empty region is an absorbing state.
	plantSpread, plantRegrow, plantClump, plantEmpty, seedsCarried float64

	// What the crop is defended with, and whether its warnings mean anything.
	// Honesty is the one the stage turns on: the two genes are drawn and
	// mutated independently, so any correlation between them is something the
	// world arrived at.
	plantPoison, plantSignal, plantHonesty float64

	// The completion condition of stage 18: the share of groups caught with
	// every member asleep at once, and how varied the population's clocks
	// are. The first means nothing without the second.
	allAsleep, clockSpread float64

	// What the population is making of its rules of thumb: room bought and
	// ideas carried per agent, how many distinct ones are alive at all, and
	// how evenly they are spread over those. The last two are the ones the
	// stage is on the hook for - a population can carry plenty of hints and
	// have them all be the same one.
	hintSlots, hintsHeld, hintKinds, hintEntropy float64
}

// perAgentLifetime converts a count of events into a rate per ten thousand
// person-ticks, one person-tick being one agent living one tick. Ten thousand
// of them is about a hundred agents living a hundred ticks, which puts the
// figures in a range the summary's two decimals can tell apart - the same
// reason the fight rates above are printed as percentages.
//
// It exists because the counts above are levels, not rates, and two arms whose
// worlds are different sizes cannot be compared on levels. A world holding half
// as many agents produces half as many children at exactly the same birth rate
// per agent, and reading that as "they are not breeding" is a mistake that has
// already been made once here (see the stage 9 entry in HISTORY.md).
//
// The window is the same final fifth the abilities are averaged over: the
// opening stretch is the population finding its size, and its birth and death
// rates belong to a transient rather than to the world being compared.
//
// Both species are in the numerator and in the denominator. Enemies breed and
// die like anybody else, but the ones that walk in from off the map are not
// births, so this is a rate for the world rather than a rate for humans. Use
// the noenemies arm when a per-human figure is what is wanted.
// windowMean is a total divided by the count that went into it, and zero when
// nothing did.
func windowMean(total float64, n int) float64 {
	if n <= 0 {
		return 0
	}
	return total / float64(n)
}

func perAgentLifetime(events int, personTicks float64) float64 {
	if personTicks <= 0 {
		return 0
	}
	return float64(events) / personTicks * 10000
}

// togetherLag is the lag the "together" metric reads the survival curve at.
// The half-life is the headline figure, but it is undefined when the curve
// never falls to a half inside the window; survival at a fixed lag always is,
// so it is the one to compare when an arm turns out to be censored.
const togetherLag = 500

type run struct {
	variant string
	seed    int64
	metrics map[string]float64
	series  []sample
}

// measure runs one world to the end and reads off what it did.
//
// The abilities are averaged over the final fifth of the run rather than read at
// the last tick: a population is small enough that a couple of deaths move the
// average by more than a run's worth of selection does.
func measure(v variant, seed int64, ticks, interval int, keepSeries bool) run {
	cfg := engine.DefaultConfig()
	cfg.Seed = seed
	v.apply(&cfg)

	w := engine.NewWorld(cfg)
	start := w.Stats()

	// The membership tracker watches only the final fifth, for the same reason
	// the abilities are averaged over it: the early ticks are the population
	// finding its size, and how long agents stay together during that is not
	// what the run is being asked about.
	member := engine.NewMembershipTracker(
		engine.DefaultClusterLinkDist, engine.DefaultMembershipStep, engine.DefaultMembershipLags)
	fights := engine.NewFightTracker(
		engine.DefaultClusterLinkDist, engine.DefaultMembershipStep, engine.DefaultCompanionLag)
	// The census runs over the whole run rather than the tail: its window
	// already trims it to the last stretch, and losing a species is a thing
	// that has to be caught when it happens, because a window that has moved
	// past a death cannot see it.
	census := engine.NewCensusTracker(engine.DefaultCensusWindow)
	watchFrom := ticks - max(ticks/5, 1)

	var series []sample
	record := func() {
		s := w.Stats()
		sp := w.Spacing()
		cl := w.Clusters(engine.DefaultClusterLinkDist)
		gaps := w.ClusterGaps(engine.DefaultClusterLinkDist)
		sdP, sdR, sdI := abilitySpread(w)
		budget, sdBudget, shares := budgetSplit(w)
		mem := w.MemoryUse()
		lore := w.Lore()
		teach := w.Teaching()
		hints := w.HintUse()
		shelter := w.Shelter()
		rich := w.Richness()
		known := w.RegionKnowledge()
		diet := w.Diet()
		plants := w.Plants()
		vig := w.Vigilance(engine.DefaultClusterLinkDist)
		series = append(series, sample{
			taught: teach.Rate, teachTop: teach.TopShare,
			restShelter: shelter.Resting, shelterAll: shelter.All,
			humanRich: rich.Humans, enemyRich: rich.Enemies, allRich: rich.All,
			regionKnown: known.Known, regionTold: known.Told,
			regionRank: known.Rank, regionSpread: known.Spread,
			dietVariety: diet.Variety, dietDiscount: diet.Discount,
			plantSpread: plants.Spread, plantRegrow: plants.Regrow,
			plantClump: plants.Clumping, plantEmpty: plants.Empty,
			seedsCarried: plants.Carried,
			plantPoison:  plants.Poison, plantSignal: plants.Signal,
			plantHonesty: plants.Honesty,
			allAsleep:    vig.AllResting, clockSpread: vig.Spread,
			hintSlots: hints.Slots, hintsHeld: hints.Held,
			hintKinds: hints.Kinds, hintEntropy: hints.Entropy,
			retal: lore.Retaliation, accept: lore.Accept,
			riskWeight: lore.RiskWeight, competition: lore.Competition, shock: lore.ShockRisk,
			sdRiskWeight: lore.SdRiskWeight, sdCompetition: lore.SdCompetition, sdShock: lore.SdShockRisk,
			budget: budget, sdBudget: sdBudget, shares: shares,
			age: s.AvgAge, maturity: s.AvgMaturity, ageFactor: s.AvgAgeFactor,
			childShare: share(s.Children, s.Population),
			remembered: mem.Remembered, friends: mem.Friends,
			memFull: mem.FullShare, restNear: mem.RestNear,
			tick: s.Tick, pop: s.Population,
			power: s.AvgPower, rat: s.AvgRationality, intel: s.AvgIntelligence,
			sdPower: sdP, sdRat: sdR, sdIntel: sdI,
			foods: s.FoodItems, births: s.Births, deaths: s.Deaths, fights: s.Fights,
			clumping: sp.Clumping, neighbours: sp.AvgNeighbours, nearest: sp.AvgNearestDist,
			clusters: float64(cl.Groups), clusterSize: cl.AvgGroupSize,
			grouped: cl.GroupedShare, largestShare: cl.LargestShare,
			gap: gaps.Mean, gapP10: gaps.P10, gapRel: gaps.Relative,
		})
	}
	// Person-ticks over the tail, and where the counters stood when it began,
	// so that the rates below cover the same window as the abilities.
	var personTicks float64
	tailStart := w.Stats()

	record()
	for i := 0; i < ticks; i++ {
		w.Step()
		if i == watchFrom {
			tailStart = w.Stats()
		}
		if i >= watchFrom {
			personTicks += float64(len(w.Agents()))
		}
		if (i+1)%interval == 0 {
			record()
		}
		if i >= watchFrom && w.Tick()%engine.DefaultMembershipStep == 0 {
			member.Observe(w)
			fights.Observe(w)
		}
		if w.Tick()%engine.DefaultCensusStep == 0 {
			census.Observe(w)
		}
	}

	end := w.Stats()
	// What the world actually did over the whole run, which is what the
	// beliefs above are trying to find out. It is not a property of the arm:
	// two arms fight different amounts and so answer the question differently.
	endLore := w.Lore()
	tail := tailAverage(series)
	mem := member.Result()
	fr := fights.Result()
	cen := census.Result()

	// The rarest species is the one coexistence stands on: the others can look
	// healthy while it goes. With humans alone it is the human population, and
	// its trough is how close the world came to ending.
	rare, _ := cen.Rarest()

	// A censored half-life is a lower bound, not a zero: report the edge of the
	// window and let the censored metric say how often that happened.
	halfLife := mem.HalfLife
	if mem.Censored {
		halfLife = float64(engine.DefaultMembershipStep * engine.DefaultMembershipLags)
	}

	r := run{variant: v.name, seed: seed, metrics: map[string]float64{
		"pop":       float64(end.Population),
		"gen":       float64(end.MaxGeneration),
		"births":    float64(end.Births),
		"deaths":    float64(end.Deaths),
		"starved":   float64(end.Deaths - end.Kills - end.AgingDeaths),
		"killed":    float64(end.Kills),
		"killShare": share(end.Kills, end.Deaths),
		"aged":      float64(end.AgingDeaths),
		"agedShare": share(end.AgingDeaths, end.Deaths),
		"fights":    float64(end.Fights),
		// Growing up and wearing out. "grewUp" is the share of everybody ever
		// born that lived long enough to finish growing, which is the figure
		// that says whether childhood is survivable at all.
		"age":             tail.age / float64(max(cfg.TicksPerYear, 1)),
		"maturity":        tail.maturity,
		"ageFactor":       tail.ageFactor,
		"childShare":      tail.childShare,
		"grewUp":          share(end.Matured, end.Births),
		"childDeathShare": share(end.ChildDeaths, end.Deaths),
		// The same events as levels, divided by how many agent-lifetimes the
		// window actually contained. These are what to compare between arms
		// whose populations differ; the levels above cannot be.
		"birthRate":    perAgentLifetime(end.Births-tailStart.Births, personTicks),
		"deathRate":    perAgentLifetime(end.Deaths-tailStart.Deaths, personTicks),
		"killRate":     perAgentLifetime(end.Kills-tailStart.Kills, personTicks),
		"starveRate":   perAgentLifetime((end.Deaths-end.Kills-end.AgingDeaths)-(tailStart.Deaths-tailStart.Kills-tailStart.AgingDeaths), personTicks),
		"fightRate":    perAgentLifetime(end.Fights-tailStart.Fights, personTicks),
		"clumping":     tail.clumping,
		"neighbours":   tail.neighbours,
		"nearest":      tail.nearest,
		"clusters":     tail.clusters,
		"clusterSize":  tail.clusterSize,
		"grouped":      tail.grouped,
		"largestShare": tail.largestShare,
		"gap":          tail.gap,
		"gapP10":       tail.gapP10,
		"gapRel":       tail.gapRel,
		"halfLife":     halfLife,
		"together":     mem.At(togetherLag),
		"censored":     boolToFloat(mem.Censored),
		// As percentages: the rates themselves are a percent or two, and the
		// summary prints two decimals, which would round them into each other.
		"fightCompanion": fr.Companion * 100,
		"fightStranger":  fr.Stranger * 100,
		"fightRatio":     fr.Ratio,
		"species":        float64(cen.Living()),
		"rareShare":      rare.Share,
		"rareTrough":     rare.Trough,
		"rareSwing":      rare.Swing,
		"remembered":     tail.remembered,
		"friends":        tail.friends,
		"memFull":        tail.memFull,
		"restNear":       tail.restNear,
		// What the population assumes, and what the world actually did. The
		// two "Err" figures are the ones to read: a belief is only worth
		// anything if it is closer to the truth than the constant it replaced,
		// and the truth is a figure of the run, not of the arm.
		"retal":         tail.retal,
		"trueRetal":     endLore.TrueRetaliation,
		"retalErr":      math.Abs(tail.retal - endLore.TrueRetaliation),
		"accept":        tail.accept,
		"trueAccept":    endLore.TrueAccept,
		"acceptErr":     math.Abs(tail.accept - endLore.TrueAccept),
		"riskWeight":    tail.riskWeight,
		"sdRiskWeight":  tail.sdRiskWeight,
		"competition":   tail.competition,
		"sdCompetition": tail.sdCompetition,
		"shock":         tail.shock,
		"sdShock":       tail.sdShock,
		// How often what an agent assumes actually changes hands, and how
		// evenly it is spread. A weight explains nothing if the rule hardly
		// ever fires, and a rule meant to spread something around must not
		// end up with three teachers and a crowd.
		"loreRate": perAgentLifetime(end.Exchanges-tailStart.Exchanges, personTicks),
		"taught":   tail.taught,
		"teachTop": tail.teachTop,
		// What the population is making of its rules of thumb. The last two
		// are the ones the stage is on the hook for: a population can carry
		// plenty of hints and have them all be the same one.
		"hintSlots":      tail.hintSlots,
		"hintsHeld":      tail.hintsHeld,
		"hintKinds":      tail.hintKinds,
		"hintEntropy":    tail.hintEntropy,
		"hintCopyRate":   perAgentLifetime(end.HintsCopied-tailStart.HintsCopied, personTicks),
		"power":          tail.power,
		"rationality":    tail.rat,
		"intelligence":   tail.intel,
		"sdPower":        tail.sdPower,
		"sdRationality":  tail.sdRat,
		"sdIntelligence": tail.sdIntel,
		"dPower":         tail.power - start.AvgPower,
		"dRationality":   tail.rat - start.AvgRationality,
		"dIntelligence":  tail.intel - start.AvgIntelligence,
		"budget":         tail.budget,
		"sdBudget":       tail.sdBudget,
		"dBudget":        tail.budget - cfg.GeneBudgetMean,
		// How often an exceptional birth happened, in years of the world's own
		// clock. The rate is per birth, so this is the figure that says what
		// that comes to at the birth rate the world actually ran at.
		// Kills that fed somebody, and how many had a share of each. A party
		// size above one is pack hunting; it is the thing stage 11 was built
		// to find out about, and nothing in the rules asks for it.
		// What an agent assumes about somebody it has never taken a reading
		// of, and how far out it was. "priorErr" is the tail window, so that
		// it covers the same span as the abilities; "priorErrAll" is the whole
		// run, and the gap between the two says whether the world got better
		// at it as it went.
		"priorErr":    windowMean(end.FirstSightError-tailStart.FirstSightError, end.FirstSights-tailStart.FirstSights),
		"priorErrAll": windowMean(end.FirstSightError, end.FirstSights),
		"firstSights": float64(end.FirstSights),
		// The same error split by the observer: the ones going by a line they
		// fitted themselves, and the ones still on the flat prior because they
		// have not seen enough. Both halves are in the same world, so the gap
		// between them is the one comparison the arms cannot confound. With
		// the learning off, "learnedShare" is zero and "priorErrLearned" is
		// meaningless.
		"priorErrLearned": windowMean(end.FirstSightErrorLearned-tailStart.FirstSightErrorLearned,
			end.FirstSightsLearned-tailStart.FirstSightsLearned),
		"priorErrGreen": windowMean(
			(end.FirstSightError-tailStart.FirstSightError)-(end.FirstSightErrorLearned-tailStart.FirstSightErrorLearned),
			(end.FirstSights-tailStart.FirstSights)-(end.FirstSightsLearned-tailStart.FirstSightsLearned)),
		"learnedShare": share(end.FirstSightsLearned-tailStart.FirstSightsLearned, end.FirstSights-tailStart.FirstSights),
		// The same encounters as "priorErr", scored by the estimators it is
		// meant to beat. These three are the comparison that arms cannot make:
		// two arms meet different creatures, and that difference is bigger
		// than the one being looked for.
		"priorErrFlat": windowMean(end.FirstSightErrorFlat-tailStart.FirstSightErrorFlat,
			end.FirstSights-tailStart.FirstSights),
		"priorErrFixed": windowMean(end.FirstSightErrorFixed-tailStart.FirstSightErrorFixed,
			end.FirstSights-tailStart.FirstSights),
		// Positive means the thing on the left of the name paid: "slopeGain"
		// is what reading the build bought over ignoring it, "learnGain" what
		// learning bought over the flat prior.
		"slopeGain": windowMean(end.FirstSightErrorFlat-tailStart.FirstSightErrorFlat, end.FirstSights-tailStart.FirstSights) -
			windowMean(end.FirstSightError-tailStart.FirstSightError, end.FirstSights-tailStart.FirstSights),
		"learnGain": windowMean(end.FirstSightErrorFixed-tailStart.FirstSightErrorFixed, end.FirstSights-tailStart.FirstSights) -
			windowMean(end.FirstSightError-tailStart.FirstSightError, end.FirstSights-tailStart.FirstSights),
		"hunts":       float64(end.Hunts),
		"jointHunts":  float64(end.JointHunts),
		"evadedShare": share(end.Evaded, end.Fights),
		// Whether running away works: attempts to flee over the tail window,
		// and the share of them that ended with the pursuer out of sight. It
		// is what stage 13 turns on - a circle has to be left in every
		// direction at once, a block only has to be crossed out of.
		"flees":       perAgentLifetime(end.Flees-tailStart.Flees, personTicks),
		"escapeShare": share(end.Escapes-tailStart.Escapes, end.Flees-tailStart.Flees),
		// Where the resting happens against where the population is. Shelter
		// multiplies what resting costs, so lower is better ground and a
		// positive shelterGain means agents are lying down in the good places.
		"restShelter": tail.restShelter,
		"shelterAll":  tail.shelterAll,
		"shelterGain": tail.shelterAll - tail.restShelter,
		// Where each kind stands, by how well the ground grows plants. A
		// positive gain is a kind that has ended up on the good ground.
		"humanRich":     tail.humanRich,
		"enemyRich":     tail.enemyRich,
		"richGain":      tail.humanRich - tail.allRich,
		"enemyRichGain": tail.enemyRich - tail.allRich,
		// What the population has made of the ground. regionRank is the one
		// that says whether any of it is true: the correlation between what
		// agents believe about a region and how well it actually grows.
		"regionKnown":  tail.regionKnown,
		"regionTold":   tail.regionTold,
		"regionRank":   tail.regionRank,
		"regionSpread": tail.regionSpread,
		// What the population lives on. dietDiscount at one is a rule that
		// never fires; dietVariety at zero is a world with nothing to vary.
		"dietVariety":  tail.dietVariety,
		"dietDiscount": tail.dietDiscount,
		// What the plants have become. plantEmpty is the one to watch: a
		// region with nothing growing in it cannot be seeded into, so zero is
		// absorbing there exactly as it is for a species.
		"plantSpread":  tail.plantSpread,
		"plantRegrow":  tail.plantRegrow,
		"plantClump":   tail.plantClump,
		"plantEmpty":   tail.plantEmpty,
		"seedsCarried": tail.seedsCarried,
		// What the crop is defended with. plantHonesty is the correlation
		// between poison and warning across the standing crop: nothing in the
		// rules ties them together, so whatever it says the world found.
		"plantPoison":  tail.plantPoison,
		"plantSignal":  tail.plantSignal,
		"plantHonesty": tail.plantHonesty,
		// Stage 18. allAsleep is the share of groups caught with everybody
		// asleep at once; clockSpread is how varied the population's hours
		// are, without which the first says nothing.
		"allAsleep":   tail.allAsleep,
		"clockSpread": tail.clockSpread,
		"packSize":    share(end.HuntParty, end.Hunts) * 1, // mean per hunt
		// The counts as well as the intervals: an interval worked out from
		// zero events is the length of the run, which reads exactly like an
		// estimate and is not one. Same censoring the half-life has.
		"geniuses":         float64(end.Geniuses),
		"greatGeniuses":    float64(end.GreatGeniuses),
		"geniusYears":      years(ticks, end.Geniuses, cfg.TicksPerYear),
		"greatGeniusYears": years(ticks, end.GreatGeniuses, cfg.TicksPerYear),
		"extinct":          boolToFloat(end.Population == 0),
	}}
	for g := 0; g < engine.NumGenes; g++ {
		r.metrics[shareMetric[g]] = tail.shares[g]
	}
	if keepSeries {
		r.series = series
	}
	checkMetricsComplete(r.metrics)
	return r
}

// checkMetricsComplete fails loudly when a name in metricNames has nothing
// behind it.
//
// It is here because the alternative is what actually happened, twice: a
// metric was added to the list and to the sample but not to the map that fills
// it in, and it printed a confident 0.00 in every arm. A measurement that is
// silently absent is worse than one that is missing, because it gets read.
func checkMetricsComplete(m map[string]float64) {
	var missing []string
	for _, name := range metricNames {
		if _, ok := m[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		panic("metrics listed but never filled in: " + strings.Join(missing, ", "))
	}
}

// tailAverage averages the last fifth of the samples, ignoring ticks where the
// population had died out and there was nothing to average.
func tailAverage(series []sample) sample {
	from := len(series) - max(len(series)/5, 1)
	var out sample
	n := 0
	for _, s := range series[from:] {
		if s.pop == 0 {
			continue
		}
		out.power += s.power
		out.rat += s.rat
		out.intel += s.intel
		out.sdPower += s.sdPower
		out.sdRat += s.sdRat
		out.sdIntel += s.sdIntel
		out.budget += s.budget
		out.sdBudget += s.sdBudget
		for g := range out.shares {
			out.shares[g] += s.shares[g]
		}
		out.clumping += s.clumping
		out.neighbours += s.neighbours
		out.nearest += s.nearest
		out.clusters += s.clusters
		out.clusterSize += s.clusterSize
		out.grouped += s.grouped
		out.largestShare += s.largestShare
		out.gap += s.gap
		out.gapP10 += s.gapP10
		out.gapRel += s.gapRel
		out.age += s.age
		out.maturity += s.maturity
		out.ageFactor += s.ageFactor
		out.childShare += s.childShare
		out.remembered += s.remembered
		out.friends += s.friends
		out.memFull += s.memFull
		out.restNear += s.restNear
		out.retal += s.retal
		out.accept += s.accept
		out.riskWeight += s.riskWeight
		out.competition += s.competition
		out.shock += s.shock
		out.sdRiskWeight += s.sdRiskWeight
		out.sdCompetition += s.sdCompetition
		out.sdShock += s.sdShock
		out.taught += s.taught
		out.teachTop += s.teachTop
		out.restShelter += s.restShelter
		out.shelterAll += s.shelterAll
		out.humanRich += s.humanRich
		out.enemyRich += s.enemyRich
		out.allRich += s.allRich
		out.regionKnown += s.regionKnown
		out.regionTold += s.regionTold
		out.regionRank += s.regionRank
		out.regionSpread += s.regionSpread
		out.dietVariety += s.dietVariety
		out.dietDiscount += s.dietDiscount
		out.plantSpread += s.plantSpread
		out.plantRegrow += s.plantRegrow
		out.plantClump += s.plantClump
		out.plantEmpty += s.plantEmpty
		out.seedsCarried += s.seedsCarried
		out.plantPoison += s.plantPoison
		out.plantSignal += s.plantSignal
		out.plantHonesty += s.plantHonesty
		out.allAsleep += s.allAsleep
		out.clockSpread += s.clockSpread
		out.hintSlots += s.hintSlots
		out.hintsHeld += s.hintsHeld
		out.hintKinds += s.hintKinds
		out.hintEntropy += s.hintEntropy
		n++
	}
	if n == 0 {
		return sample{}
	}
	d := float64(n)
	out.power /= d
	out.rat /= d
	out.intel /= d
	out.sdPower /= d
	out.sdRat /= d
	out.sdIntel /= d
	out.budget /= d
	out.sdBudget /= d
	for g := range out.shares {
		out.shares[g] /= d
	}
	out.clumping /= d
	out.neighbours /= d
	out.nearest /= d
	out.clusters /= d
	out.clusterSize /= d
	out.grouped /= d
	out.largestShare /= d
	out.gap /= d
	out.gapP10 /= d
	out.gapRel /= d
	out.age /= d
	out.maturity /= d
	out.ageFactor /= d
	out.childShare /= d
	out.remembered /= d
	out.friends /= d
	out.memFull /= d
	out.restNear /= d
	out.retal /= d
	out.accept /= d
	out.riskWeight /= d
	out.competition /= d
	out.shock /= d
	out.sdRiskWeight /= d
	out.sdCompetition /= d
	out.sdShock /= d
	out.taught /= d
	out.teachTop /= d
	out.restShelter /= d
	out.shelterAll /= d
	out.humanRich /= d
	out.enemyRich /= d
	out.allRich /= d
	out.regionKnown /= d
	out.regionTold /= d
	out.regionRank /= d
	out.regionSpread /= d
	out.dietVariety /= d
	out.dietDiscount /= d
	out.plantSpread /= d
	out.plantRegrow /= d
	out.plantClump /= d
	out.plantEmpty /= d
	out.seedsCarried /= d
	out.plantPoison /= d
	out.plantSignal /= d
	out.plantHonesty /= d
	out.allAsleep /= d
	out.clockSpread /= d
	out.hintSlots /= d
	out.hintsHeld /= d
	out.hintKinds /= d
	out.hintEntropy /= d
	return out
}

// abilitySpread is the standard deviation of each ability across the living
// population: how much variation there is for selection to work on.
//
// It is the figure the inheritance rule decides. Blending inheritance (a child
// is the average of its parents) halves the variance every generation, so the
// spread settles wherever mutation alone can hold it; drawing each gene from
// one parent or the other keeps it. The mean says which way selection is
// pushing, and this says how much it has left to push.
// The inherited values, not the expressed ones: selection acts on what is
// passed on, and a world that happens to hold a lot of children would
// otherwise look like a world that had bred weaker agents.
func abilitySpread(w *engine.World) (power, rationality, intelligence float64) {
	agents := w.Agents()
	n := float64(len(agents))
	if n < 2 {
		return 0, 0, 0
	}
	var mp, mr, mi float64
	for i := range agents {
		mp += agents[i].Gene(engine.GeneAttack)
		mr += agents[i].Gene(engine.GeneRationality)
		mi += agents[i].Gene(engine.GeneIntelligence)
	}
	mp, mr, mi = mp/n, mr/n, mi/n
	var sp, sr, si float64
	for i := range agents {
		sp += (agents[i].Gene(engine.GeneAttack) - mp) * (agents[i].Gene(engine.GeneAttack) - mp)
		sr += (agents[i].Gene(engine.GeneRationality) - mr) * (agents[i].Gene(engine.GeneRationality) - mr)
		si += (agents[i].Gene(engine.GeneIntelligence) - mi) * (agents[i].Gene(engine.GeneIntelligence) - mi)
	}
	return math.Sqrt(sp / n), math.Sqrt(sr / n), math.Sqrt(si / n)
}

// budgetSplit is what the population is made of: the mean budget, its spread,
// and the mean share of it each gene takes.
func budgetSplit(w *engine.World) (mean, sd float64, shares [engine.NumGenes]float64) {
	agents := w.Agents()
	if len(agents) == 0 {
		return 0, 0, shares
	}
	n := float64(len(agents))
	var sum, sq float64
	for i := range agents {
		b := agents[i].Budget()
		sum += b
		sq += b * b
		if b <= 0 {
			continue
		}
		for g := 0; g < engine.NumGenes; g++ {
			shares[g] += agents[i].Gene(engine.Gene(g)) / b
		}
	}
	mean = sum / n
	sd = math.Sqrt(math.Max(0, sq/n-mean*mean))
	for g := range shares {
		shares[g] /= n
	}
	return mean, sd, shares
}

// share is what fraction of the deaths were killings. It is the headline
// figure for the cooperation work: the point of that work is to get it down
// without simply feeding everybody, which is why it sits next to starved.
func share(part, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return float64(part) / float64(whole)
}

// shareMetric names the per gene share metrics in the order the genes are in.
var shareMetric = []string{
	"shAttack", "shDefence", "shVitality", "shSpeed", "shEvasion",
	"shMemory", "shRationality", "shIntelligence", "shLooks",
}

// years is how long, in years of the world's clock, between one of these
// events and the next. Zero events is reported as the length of the run, which
// is a lower bound rather than an answer.
// years is how long the world went, on average, between events of some kind.
//
// With no events at all there is no interval to report, and what comes back is
// the length of the whole run - a lower bound, not an estimate. It looks
// exactly like a real figure, so read it next to the count of events: a
// geniusYears of 40 in a forty year run means "never", not "once".
func years(ticks, events, ticksPerYear int) float64 {
	if ticksPerYear <= 0 {
		return 0
	}
	if events <= 0 {
		return float64(ticks) / float64(ticksPerYear)
	}
	return float64(ticks) / float64(events) / float64(ticksPerYear)
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// --- statistics -------------------------------------------------------------

// meanStderr is the average of a set of readings and how far the average itself
// is likely to be off, which is what says whether a difference is worth
// believing.
func meanStderr(xs []float64) (float64, float64) {
	if len(xs) == 0 {
		return 0, 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	mean := sum / float64(len(xs))
	if len(xs) < 2 {
		return mean, 0
	}
	var sq float64
	for _, x := range xs {
		sq += (x - mean) * (x - mean)
	}
	return mean, math.Sqrt(sq/float64(len(xs)-1)) / math.Sqrt(float64(len(xs)))
}

// verdict marks how far a paired difference is from zero in standard errors.
// It is a rule of thumb for reading the table, not a test: with a dozen seeds
// two standard errors is about the point where a difference is worth acting on.
func verdict(mean, stderr float64) string {
	if stderr == 0 {
		return ""
	}
	switch t := math.Abs(mean / stderr); {
	case t >= 3:
		return "***"
	case t >= 2:
		return "**"
	case t >= 1:
		return "*"
	default:
		return ""
	}
}

// --- output -----------------------------------------------------------------

func printSummary(out *tabwriter.Writer, names []string, byVariant map[string][]run) {
	fmt.Fprint(out, "metric")
	for _, n := range names {
		fmt.Fprintf(out, "\t%s", n)
	}
	fmt.Fprintln(out)

	for _, m := range metricNames {
		fmt.Fprintf(out, "%s", m)
		for _, n := range names {
			mean, se := meanStderr(values(byVariant[n], m))
			fmt.Fprintf(out, "\t%.2f +/-%.2f", mean, se)
		}
		fmt.Fprintln(out)
	}
}

// printPaired shows each variant against the reference on the same seeds. The
// pairing is the point: seed to seed variation is far larger than anything a
// rule change does, and it cancels out here.
func printPaired(out *tabwriter.Writer, base string, names []string, byVariant map[string][]run) {
	others := make([]string, 0, len(names))
	for _, n := range names {
		if n != base {
			others = append(others, n)
		}
	}
	if len(others) == 0 {
		return
	}

	fmt.Fprintf(out, "metric")
	for _, n := range others {
		fmt.Fprintf(out, "\t%s - %s", n, base)
	}
	fmt.Fprintln(out)

	for _, m := range metricNames {
		fmt.Fprintf(out, "%s", m)
		for _, n := range others {
			diffs := pairedDiffs(byVariant[n], byVariant[base], m)
			mean, se := meanStderr(diffs)
			fmt.Fprintf(out, "\t%+.2f +/-%.2f %s", mean, se, verdict(mean, se))
		}
		fmt.Fprintln(out)
	}
}

func values(runs []run, metric string) []float64 {
	out := make([]float64, 0, len(runs))
	for _, r := range runs {
		out = append(out, r.metrics[metric])
	}
	return out
}

// pairedDiffs lines the two arms up by seed and subtracts.
func pairedDiffs(a, b []run, metric string) []float64 {
	base := make(map[int64]float64, len(b))
	for _, r := range b {
		base[r.seed] = r.metrics[metric]
	}
	out := make([]float64, 0, len(a))
	for _, r := range a {
		if v, ok := base[r.seed]; ok {
			out = append(out, r.metrics[metric]-v)
		}
	}
	return out
}

func writeCSV(path string, runs []run) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"variant", "seed", "tick", "pop", "power", "rationality", "intelligence", "foods", "births", "deaths", "fights", "clumping", "neighbours", "nearest", "clusters", "clusterSize", "grouped", "largestShare", "gap", "gapP10", "gapRel"}); err != nil {
		return err
	}
	for _, r := range runs {
		for _, s := range r.series {
			row := []string{
				r.variant, strconv.FormatInt(r.seed, 10), strconv.Itoa(s.tick), strconv.Itoa(s.pop),
				strconv.FormatFloat(s.power, 'f', 2, 64),
				strconv.FormatFloat(s.rat, 'f', 2, 64),
				strconv.FormatFloat(s.intel, 'f', 2, 64),
				strconv.Itoa(s.foods), strconv.Itoa(s.births), strconv.Itoa(s.deaths), strconv.Itoa(s.fights),
				strconv.FormatFloat(s.clumping, 'f', 3, 64),
				strconv.FormatFloat(s.neighbours, 'f', 2, 64),
				strconv.FormatFloat(s.nearest, 'f', 2, 64),
				strconv.FormatFloat(s.clusters, 'f', 0, 64),
				strconv.FormatFloat(s.clusterSize, 'f', 2, 64),
				strconv.FormatFloat(s.grouped, 'f', 3, 64),
				strconv.FormatFloat(s.largestShare, 'f', 3, 64),
				strconv.FormatFloat(s.gap, 'f', 2, 64),
				strconv.FormatFloat(s.gapP10, 'f', 2, 64),
				strconv.FormatFloat(s.gapRel, 'f', 3, 64),
			}
			if err := w.Write(row); err != nil {
				return err
			}
		}
	}
	return w.Error()
}

// --- driving ----------------------------------------------------------------

type job struct {
	v    variant
	seed int64
}

func main() {
	names := flag.String("variants", "baseline,nogate", "comma separated arms to run")
	base := flag.String("base", "", "arm the others are compared against (default: the first one)")
	seeds := flag.Int("seeds", 12, "how many seeds each arm is run on")
	firstSeed := flag.Int64("seed0", 1, "first seed; the arms all use seed0 .. seed0+seeds-1")
	ticks := flag.Int("ticks", 20000, "ticks per run")
	interval := flag.Int("interval", 200, "ticks between samples")
	csvPath := flag.String("csv", "", "write the sampled time series here")
	jobs := flag.Int("jobs", runtime.NumCPU(), "runs in parallel")
	list := flag.Bool("list", false, "list the arms and exit")
	flag.Parse()

	if *list {
		for _, v := range variants {
			fmt.Printf("%-12s %s\n", v.name, v.about)
		}
		return
	}

	chosen := make([]variant, 0, 4)
	for _, n := range strings.Split(*names, ",") {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		v, ok := variantByName(n)
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown variant %q; -list shows what there is\n", n)
			os.Exit(1)
		}
		chosen = append(chosen, v)
	}
	if len(chosen) == 0 {
		fmt.Fprintln(os.Stderr, "no variants selected")
		os.Exit(1)
	}
	if *base == "" {
		*base = chosen[0].name
	}

	queue := make([]job, 0, len(chosen)**seeds)
	for _, v := range chosen {
		for i := 0; i < *seeds; i++ {
			queue = append(queue, job{v: v, seed: *firstSeed + int64(i)})
		}
	}

	fmt.Printf("%d arms x %d seeds x %d ticks, %d in parallel\n\n", len(chosen), *seeds, *ticks, *jobs)

	results := make([]run, len(queue))
	var wg sync.WaitGroup
	next := make(chan int)
	go func() {
		for i := range queue {
			next <- i
		}
		close(next)
	}()
	for i := 0; i < max(*jobs, 1); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range next {
				j := queue[idx]
				results[idx] = measure(j.v, j.seed, *ticks, *interval, *csvPath != "")
			}
		}()
	}
	wg.Wait()

	byVariant := make(map[string][]run, len(chosen))
	for _, r := range results {
		byVariant[r.variant] = append(byVariant[r.variant], r)
	}
	names2 := make([]string, 0, len(chosen))
	for _, v := range chosen {
		rs := byVariant[v.name]
		sort.Slice(rs, func(i, j int) bool { return rs[i].seed < rs[j].seed })
		names2 = append(names2, v.name)
	}

	out := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(out, "== per arm, mean +/- standard error over seeds ==")
	printSummary(out, names2, byVariant)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "== paired difference on the same seeds (* = 1 stderr, ** = 2, *** = 3) ==")
	printPaired(out, *base, names2, byVariant)
	out.Flush()

	if *csvPath != "" {
		if err := writeCSV(*csvPath, results); err != nil {
			fmt.Fprintf(os.Stderr, "csv: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\ntime series written to %s\n", *csvPath)
	}
}
