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

// The test maps of stage 20. They are laid out by hand rather than generated,
// because the point of the first measurement is to know exactly what country
// the world is and to be able to look at it in cmd/devview.
//
// Sixteen columns by twelve rows over 800 by 600 is a cell of 50.
var (
	// Broken country over the western half: a body crossing it pays twice.
	// Split east and west rather than scattered, so that "live in the open"
	// and "live in the rough" are places an agent can actually stay in.
	mapRough = []string{
		"::::::::........",
		"::::::::........",
		"::::::::........",
		"::::::::........",
		"::::::::........",
		"::::::::........",
		"::::::::........",
		"::::::::........",
		"::::::::........",
		"::::::::........",
		"::::::::........",
		"::::::::........",
	}

	// A river north to south, two cells wide, with no bridge: crossing is
	// dear, not impossible, which is the difference between a cost and an
	// obstacle.
	mapRiver = []string{
		".......~~.......",
		".......~~.......",
		".......~~.......",
		".......~~.......",
		".......~~.......",
		".......~~.......",
		".......~~.......",
		".......~~.......",
		".......~~.......",
		".......~~.......",
		".......~~.......",
		".......~~.......",
	}

	// The same river, moved to the western edge (stage 35). The point of it
	// is that nothing lies beyond it: a body drawn to better country is never
	// drawn THROUGH this one, so what it separates is whether a belief about
	// a dangerous place keeps anybody out of it, from whether the place is
	// simply on the way to everywhere.
	mapRiverEdge = []string{
		".~~.............",
		".~~.............",
		".~~.............",
		".~~.............",
		".~~.............",
		".~~.............",
		".~~.............",
		".~~.............",
		".~~.............",
		".~~.............",
		".~~.............",
		".~~.............",
	}

	// The same water, not in a line (stage 37). Twenty-four cells of it, the
	// same count as the river, laid out as six pools spread over the map: the
	// same area to cross, the same cost to cross it, the same amount of
	// ground that can drown a body - and nothing dividing the world.
	//
	// It is the control the stage turns on. A river against open country
	// compares two worlds that differ in two ways at once (there is more dear
	// ground, AND it lies in a line), and only the second of them is the
	// question.
	mapPonds = []string{
		"................",
		".~~.............",
		".~~.........~~..",
		"............~~..",
		".....~~.........",
		".....~~.........",
		"................",
		".........~~.....",
		"..~~.....~~.....",
		"..~~........~~..",
		"............~~..",
		"................",
	}

	// A river wide enough to be wider than sight (stage 37). Four cells is
	// 200 across, against a sight block of about 230 that an agent is
	// standing somewhere inside: a body on one bank can see the far bank only
	// from the water's edge. It is the arm that says whether a river that
	// does not divide a world failed to because it was too narrow to hide the
	// other side.
	mapGorge = []string{
		"......~~~~......",
		"......~~~~......",
		"......~~~~......",
		"......~~~~......",
		"......~~~~......",
		"......~~~~......",
		"......~~~~......",
		"......~~~~......",
		"......~~~~......",
		"......~~~~......",
		"......~~~~......",
		"......~~~~......",
	}

	// Forty-eight cells again, in twelve pools: the control for the gorge, the
	// way mapPonds is the control for the river.
	mapPools = []string{
		"~~......~~......",
		"~~......~~......",
		"....~~......~~..",
		"....~~......~~..",
		"..~~......~~....",
		"..~~......~~....",
		"......~~......~~",
		"......~~......~~",
		"~~......~~......",
		"~~......~~......",
		"....~~......~~..",
		"....~~......~~..",
	}

	// High ground in the north-east, stacked: a first level with two ramps up
	// to it, and a second level on top of it with one ramp of its own. The
	// only ways in are the ramps.
	mapPlateau = []string{
		"..........111111",
		"..........122221",
		"..........1B2221",
		"..........122221",
		"..........111111",
		"..........A.....",
		"................",
		"................",
		"..........A11111",
		"..........111111",
		"..........111111",
		"..........111111",
	}

	// All three at once, which is the arm a world would actually be played on.
	mapCountry = []string{
		"::::...~~.111111",
		"::::...~~.122221",
		"::::...~~.1B2221",
		"::::...~~.122221",
		"::::...~~.111111",
		"::::...~~.A.....",
		"::::...~~.......",
		"::::...~~.......",
		"::::...~~.A11111",
		"::::...~~.111111",
		"::::...~~.111111",
		"::::...~~.111111",
	}
)

// sexSplit leans the genes by sex the way stage 53's roles ask for: the mother
// reads the ground around her, takes what comes to her and depends on who is
// nearby; the father goes where it is dangerous, covers the distance and picks
// between ways of doing it. Positive favours the mother, so a negative figure
// hands each role the other one's body.
func sexSplit(c *engine.Config, by float64) {
	c.SexBias[engine.GeneRationality] = by
	c.SexBias[engine.GeneDefence] = by
	c.SexBias[engine.GeneMemory] = by
	c.SexBias[engine.GeneAttack] = -by
	c.SexBias[engine.GeneSpeed] = -by
	c.SexBias[engine.GeneIntelligence] = -by
}

// playedMap is the world cmd/devview lays out to be played on: terrain, food
// tied to the ground and the water, fish, the awkward crop and stones. Stage
// 44 showed that a rule which spends food can be worth a population on the
// flat world and cost one here, so anything about food is read on both.
func playedMap(c *engine.Config) {
	c.TerrainMap, c.TerrainFoodCorrelation, c.WatersideFood = mapCountry, 1, 1
	c.FishShare, c.SpecialtyShare, c.SpecialtyCatch = 0.25, 0.08, 0.5
	c.Stones = 60
}

// A variant is one arm of an experiment: a name, why it exists, and what it
// changes about the default configuration.
type variant struct {
	name  string
	about string
	apply func(*engine.Config)

	// stores lays caches on the world after it is built (stage 50). They are
	// not a Config field: they are put there by whoever lays the world out,
	// the same footing the editor is on, so an arm that wants them has to
	// place them.
	stores func(*engine.World)
}

// Where the caches go. Spread out rather than clustered, so that no one body
// can know all of them by standing still, and away from the edges.
func playedStores(w *engine.World) {
	for _, at := range [][2]float64{
		{200, 130}, {200, 500}, {640, 130}, {640, 500}, {1080, 130}, {1080, 500},
	} {
		w.SetStore(at[0], at[1])
	}
}

func flatStores(w *engine.World) { playedStores(w) }

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
	// Missing costs something. A blow that finds nothing leaves the one who
	// threw it off balance, which is the first rule in the world that prices
	// a miss. Nothing can see it coming, so what these arms ask is what
	// selection makes of a cost nobody can plan around.
	// Whether "I am too hungry to court" is a gate on the option or a reason
	// inside the comparison. The design says no hardcoded behavioural
	// threshold should exist; the priority rule says life comes before
	// offspring. This is where the two meet.
	// What a patient agent will still not settle for. Past its patience it used
	// to accept anybody, which is the one place a candidate's condition - a
	// third of how good a mate looks - stopped counting.
	{
		name:  "steadypatience",
		about: "the comparison clock runs from when an agent could first look, not from its latest attempt",
		apply: func(c *engine.Config) { c.CourtClockResets = false },
	},
	{
		name:  "nofloor",
		about: "no floor at all: a patient agent settles for anybody, the world before stage 26",
		apply: func(c *engine.Config) { c.CommitFloor = 0 },
	},
	{
		name:  "softfloor",
		about: "a floor of 25, well under the population's median fitness",
		apply: func(c *engine.Config) { c.CommitFloor = 25 },
	},
	{
		name:  "pickyfloor",
		about: "a patient agent still turns down a candidate under 38 (about the bottom quarter)",
		apply: func(c *engine.Config) { c.CommitFloor = 38 },
	},
	{
		name:  "pickyhard",
		about: "... and under 50, which is above the median",
		apply: func(c *engine.Config) { c.CommitFloor = 50 },
	},
	{
		name:  "courtjudge",
		about: "courting is always on offer if the body can pay for a birth: the formula judges, not a threshold",
		apply: func(c *engine.Config) { c.CourtNeedsSurplus = false },
	},
	{
		name:  "noopening",
		about: "a blow that misses costs the swinger nothing: the world before the opening",
		apply: func(c *engine.Config) { c.OpeningTicks = 0 },
	},
	{
		name:  "wideopening",
		about: "a miss leaves the swinger fully open for six ticks",
		apply: func(c *engine.Config) { c.OpeningTicks, c.OpeningGuard = 6, 0 },
	},
	{
		name:  "briefopening",
		about: "a miss leaves the swinger barely open, for one tick",
		apply: func(c *engine.Config) { c.OpeningTicks, c.OpeningGuard = 1, 0.7 },
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
		name:  "poisonsaves",
		about: "a poisonous plant may be spat out and left standing: the first thing poison ever did for the plant (17b revived)",
		apply: func(c *engine.Config) {
			c.PlantDefence, c.PoisonDamage, c.PlantPoisonSaves = true, 2, 1
		},
	},
	{
		name:  "poisoncost",
		about: "shouting costs seed: the plant-side price the signal never had",
		apply: func(c *engine.Config) {
			c.PlantDefence, c.PoisonDamage, c.PlantSignalCost = true, 2, 0.8
		},
	},
	{
		name:  "poisonboth",
		about: "both prices at once: poison saves the plant, and being loud costs it seed (17b as revived)",
		apply: func(c *engine.Config) {
			c.PlantDefence, c.PoisonDamage = true, 2
			c.PlantPoisonSaves, c.PlantSignalCost = 1, 0.8
		},
	},
	{
		name:  "poisonpriced",
		about: "both defences priced in seed: poison saves the plant and costs it, and so does shouting (17b revived)",
		apply: func(c *engine.Config) {
			c.PlantDefence, c.PoisonDamage = true, 2
			c.PlantPoisonSaves, c.PlantSignalCost, c.PlantPoisonCost = 1, 0.8, 0.8
		},
	},
	{
		name:  "poisonpricedhard",
		about: "the same with a dose that hurts: does the priced world hold at 8?",
		apply: func(c *engine.Config) {
			c.PlantDefence, c.PoisonDamage = true, 8
			c.PlantPoisonSaves, c.PlantSignalCost, c.PlantPoisonCost = 1, 0.8, 0.8
		},
	},
	{
		name:  "poisonpricedonly",
		about: "control: poison priced but shouting free - which price is doing the work?",
		apply: func(c *engine.Config) {
			c.PlantDefence, c.PoisonDamage = true, 2
			c.PlantPoisonSaves, c.PlantPoisonCost = 1, 0.8
		},
	},
	{
		name:  "poisonbothhard",
		about: "the same with a dose that hurts: does the revived world hold at 8?",
		apply: func(c *engine.Config) {
			c.PlantDefence, c.PoisonDamage = true, 8
			c.PlantPoisonSaves, c.PlantSignalCost = 1, 0.8
		},
	},
	{
		name:  "poisontol",
		about: "the revived crop (dose 2, shouting priced) and bodies that learn what they can stomach (38b)",
		apply: func(c *engine.Config) {
			c.PlantDefence, c.PoisonDamage, c.PlantSignalCost = true, 2, 0.8
			c.SkillBirthplace = 0.5
		},
	},
	{
		name:  "poisontolloud",
		about: "tolerance in the world where the crop still shouts (no signal price): is the fear what it was for?",
		apply: func(c *engine.Config) {
			c.PlantDefence, c.PoisonDamage = true, 2
			c.SkillBirthplace = 0.5
		},
	},
	{
		name:  "poisontoldead",
		about: "control: the tolerance is learned and takes the room, and turns nothing aside",
		apply: func(c *engine.Config) {
			c.PlantDefence, c.PoisonDamage, c.PlantSignalCost = true, 2, 0.8
			c.SkillBirthplace, c.SkillPoisonRelief = 0.5, 0
		},
	},
	{
		name:  "poisontolnoteach",
		about: "control: what a body can stomach is born with and inherited, not caught",
		apply: func(c *engine.Config) {
			c.PlantDefence, c.PoisonDamage, c.PlantSignalCost = true, 2, 0.8
			c.SkillBirthplace, c.SkillsSpread = 0.5, false
		},
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
		// The stage 10 carry-over. Reading a build is worth nothing because
		// the visible genes are attack's competitors for the budget: every
		// point spent on being big is a point not spent on hitting. This arm
		// shows the total instead of the split - size visible, allocation
		// still hidden - and asks whether there is a signal there at all.
		name:  "bulklooks",
		about: "what is visible of a body is its size rather than two of its genes",
		apply: func(c *engine.Config) { c.LooksShowBulk = true },
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
	// Stage 20: the ground. One kind of country at a time, because "add
	// terrain" is four different rules wearing one word - dear ground acts on
	// speed, an obstacle on route finding, a narrow place on defence, cover on
	// evasion (PLAN.md). These arms are the first of those, plus the height
	// that makes high ground and ramps out of the same map.
	//
	// The maps are 16 by 12 over the 800 by 600 world, so a cell is 50 by 50 -
	// a little under half the 130 an agent can see, which keeps a patch big
	// enough to live in and small enough that several fit in the world.
	{
		name:  "rough",
		about: "half the world is broken country that costs twice as much to cross",
		apply: func(c *engine.Config) { c.TerrainMap = mapRough },
	},
	{
		name:  "river",
		about: "a river down the middle: crossable, and three times the cost",
		apply: func(c *engine.Config) { c.TerrainMap = mapRiver },
	},
	{
		name:  "plateau",
		about: "high ground in one corner, reachable only by its two ramps",
		apply: func(c *engine.Config) { c.TerrainMap = mapPlateau },
	},
	{
		name:  "plateaulink",
		about: "the plateau with the food following the ground (33 on): is high ground poor ground?",
		apply: func(c *engine.Config) { c.TerrainMap, c.TerrainFoodCorrelation = mapPlateau, 1 },
	},
	{
		name:  "country",
		about: "all three at once: rough ground, a river, and a stacked plateau",
		apply: func(c *engine.Config) { c.TerrainMap = mapCountry },
	},
	// Stage 34: what the water does. The pair to read is river against
	// riverdry - the same map with the drowning turned off, which is the
	// world stage 20 measured - and the two sweeps say how much of what
	// happens is the rate rather than the rule.
	{
		name:  "riverdry",
		about: "control: the same river, and nobody ever drowns in it (34 off)",
		apply: func(c *engine.Config) { c.TerrainMap, c.DrownChancePerTick = mapRiver, 0 },
	},
	{
		name:  "drownlow",
		about: "sweep: the river at a quarter of the drowning rate",
		apply: func(c *engine.Config) { c.TerrainMap, c.DrownChancePerTick = mapRiver, 0.00005 },
	},
	{
		name:  "drownhigh",
		about: "sweep: the river at five times the drowning rate",
		apply: func(c *engine.Config) { c.TerrainMap, c.DrownChancePerTick = mapRiver, 0.001 },
	},
	// Stage 35: the drowning as news, and the danger as a belief about a
	// place. Three arms so the three ways of learning it come apart: nobody
	// weighs it, everybody weighs what they felt themselves, and everybody
	// also learns from watching. river is the base - it is stage 34's world.
	{
		name:  "dangernone",
		about: "control: the river of stage 34 - the danger is learned and never weighed (35 off)",
		apply: func(c *engine.Config) { c.TerrainMap, c.RegionDangerTicks = mapRiver, 0 },
	},
	{
		name:  "dangerlore",
		about: "the river, and agents believe how dangerous a region is (35: felt, watched and told)",
		apply: func(c *engine.Config) { c.TerrainMap = mapRiver },
	},
	{
		name:  "dangerfelt",
		about: "control: the danger is learned by standing in it and never watched or told (35 alone)",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.DrownWitnessLooks, c.RegionDangerTold = mapRiver, 0, false
		},
	},
	{
		name:  "dangernottold",
		about: "control: felt and watched, but never handed on (what does telling add?)",
		apply: func(c *engine.Config) { c.TerrainMap, c.RegionDangerTold = mapRiver, false },
	},
	{
		name:  "dangerheavy",
		about: "sweep: the same belief priced over four planning horizons instead of one",
		apply: func(c *engine.Config) { c.TerrainMap, c.RegionDangerTicks = mapRiver, 2800 },
	},
	// Stage 36: the bank. The base is river - stages 34 and 35 as they stand,
	// with the food laid out without reference to the ground - and the arms
	// ask what happens when the water is also where the food is, when it is
	// the opposite, and when the two ways of tying food to ground are asked
	// for together.
	{
		name:  "waterrich",
		about: "the river bank grows more: the first country that both kills and feeds",
		apply: func(c *engine.Config) { c.TerrainMap, c.WatersideFood = mapRiver, 1 },
	},
	{
		name:  "waterbarren",
		about: "the other landscape: the same strip, and nothing grows beside it",
		apply: func(c *engine.Config) { c.TerrainMap, c.WatersideFood = mapRiver, -1 },
	},
	{
		name:  "waterrichblind",
		about: "control: a rich bank that nobody fears (35 priced out, 36 on)",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.WatersideFood, c.RegionDangerTicks = mapRiver, 1, 0
		},
	},
	{
		name:  "waterrichlink",
		about: "both ties at once: hard country poor (33) and the bank rich (36)",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.WatersideFood, c.TerrainFoodCorrelation = mapRiver, 1, 1
		},
	},
	// Stage 32: calling others in. The base is baseline - the rule is on by
	// default - and the arms take it apart: the word without anybody being
	// counted on, being counted on without the word, and the trust that
	// decides how much an ally is worth.
	{
		name:  "nocall",
		about: "control: no word for calling others in, but an ally already swinging still counts (32a off)",
		apply: func(c *engine.Config) { c.CallTicks = 0 },
	},
	{
		name:  "noally",
		about: "control: nobody is ever counted on, so the call is spoken and means nothing (32b off)",
		apply: func(c *engine.Config) { c.AllyTrustWeight = 0 },
	},
	{
		name:  "noteam",
		about: "control: neither half - the world as it was before stage 32",
		apply: func(c *engine.Config) { c.CallTicks, c.AllyTrustWeight = 0, 0 },
	},
	{
		name:  "preyonly",
		about: "only a hunt counts: two agents ganging up on a third get no help from each other",
		apply: func(c *engine.Config) { c.AllyPreyOnly = true },
	},
	{
		name:  "trusteasy",
		about: "sweep: half the affinity buys complete trust in an ally",
		apply: func(c *engine.Config) { c.AffinityTrust = 10 },
	},
	{
		name:  "trusthard",
		about: "sweep: twice the affinity for complete trust - does it matter that trust reaches 1?",
		apply: func(c *engine.Config) { c.AffinityTrust = 40 },
	},
	{
		name:  "allyhalf",
		about: "sweep: an ally is worth half what trust says (nobody is ever fully counted on)",
		apply: func(c *engine.Config) { c.AllyTrustWeight = 0.5 },
	},
	// Stage 38a: the first skill. It only means anything where there is
	// broken country, so every arm here is laid on one - rough for the clean
	// reading, country for the map a world would be played on. The controls
	// take the three parts apart: no skill at all, a skill that is learned
	// and takes room but does nothing, and one that cannot be copied.
	{
		name:  "roughskill",
		about: "broken country, with knowing how to cross it and how to live off it (38a and 38b together: both skills compete for the same room)",
		apply: func(c *engine.Config) { c.TerrainMap, c.SkillBirthplace = mapRough, 0.5 },
	},
	{
		name:  "roughnoskill",
		about: "control: the same country, and nobody ever learns anything about crossing it (the default)",
		apply: func(c *engine.Config) { c.TerrainMap = mapRough },
	},
	{
		name:  "roughdeadskill",
		about: "control: the skill is learned and takes the room, and does nothing (the cost alone)",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.SkillBirthplace, c.SkillRoughRelief = mapRough, 0.5, 0
		},
	},
	{
		name:  "roughnoteach",
		about: "control: a skill can be born with and inherited, but not caught from anybody",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.SkillBirthplace, c.SkillsSpread = mapRough, 0.5, false
		},
	},
	{
		name:  "roughnoleap",
		about: "control: no genius goes further at it than its line does",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.SkillBirthplace, c.SkillGeniusJump = mapRough, 0.5, 0
		},
	},
	{
		name:  "skillstrong",
		about: "sweep: knowing the ground takes all of the extra cost out of it",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.SkillBirthplace, c.SkillRoughRelief = mapRough, 0.5, 1
		},
	},
	{
		name:  "skilltough",
		about: "sweep: what a body gets out of knowing the ground is capped by how tough it is, not how fast",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.SkillBirthplace = mapRough, 0.5
			c.SkillAptitude[engine.SkillRough] = engine.GeneVitality
		},
	},
	{
		name:  "countryskill",
		about: "the whole country with skills (the map a world would be played on)",
		apply: func(c *engine.Config) { c.TerrainMap, c.SkillBirthplace = mapCountry, 0.5 },
	},
	{
		name:  "countrynoskill",
		about: "control for countryskill: nobody learns the ground (the default)",
		apply: func(c *engine.Config) { c.TerrainMap = mapCountry },
	},
	// Stage 39: what a carcass is worth. Two levers, measured apart (#68) -
	// how much meat there is, and what one item of it does - because a world
	// where hunting suddenly pays cannot say which of them paid. Healing is
	// the default since 2026-09-09, so the arm that says what it bought is
	// the one with it off.
	{
		name:  "meatoff",
		about: "39b off: a carcass fills a stomach and mends nothing, which is every world before stage 39",
		apply: func(c *engine.Config) { c.MeatVitality = 0 },
	},
	{
		name:  "meatquarter",
		about: "39b at a quarter of the eater's ceiling, for the shape of the dose",
		apply: func(c *engine.Config) { c.MeatVitality = 0.25 },
	},
	{
		name:  "meatwhole",
		about: "39b at a whole ceiling: what the consultation asked for",
		apply: func(c *engine.Config) { c.MeatVitality = 1 },
	},
	{
		name:  "meathealblind",
		about: "control: the meat mends as much and nobody is told - selection without choice (the control stage 34 paid for)",
		apply: func(c *engine.Config) { c.MeatHealKnown = false },
	},
	{
		name:  "meatfills",
		about: "control: a carcass worth two meals in the stomach and nothing in the body - nourishing, not medicine",
		apply: func(c *engine.Config) { c.MeatNutrition, c.MeatVitality = 2, 0 },
	},
	{
		name:  "meatmore",
		about: "39a on top of the default: a carcass leaves twice as much (MeatPerBudget 60). What stage 41 needs a surplus from",
		apply: func(c *engine.Config) { c.MeatPerBudget = 60 },
	},
	{
		name:  "meatlots",
		about: "39a four times over, which is where the amount starts scattering the population across the kills",
		apply: func(c *engine.Config) { c.MeatPerBudget = 30 },
	},
	// Stage 40: carrying. The arm that says what it bought is the one with it
	// off, and the flat world is where it is measured first - the load and the
	// ground both multiply what moving costs, so a map with rough country in
	// it measures the two of them at once (#66).
	{
		name:  "carryoff",
		about: "no hands: food is in the ground or in a stomach, which is every world before stage 40",
		apply: func(c *engine.Config) { c.CarryCapacity = 0 },
	},
	{
		name:  "carryfree",
		about: "control: carrying costs nothing to lug, so what is left is the value of having food later",
		apply: func(c *engine.Config) { c.CarryCost = 0 },
	},
	{
		name:  "carryunwanted",
		about: "control: the word exists and nothing is ever worth picking up (CarryValue 0)",
		apply: func(c *engine.Config) { c.CarryValue = 0 },
	},
	{
		name:  "carrytwo",
		about: "two items at a time, between the one that helps and the three that do not",
		apply: func(c *engine.Config) { c.CarryCapacity = 2 },
	},
	{
		name:  "carrycool",
		about: "three items but half as keen to fill them (CarryValue 0.25)",
		apply: func(c *engine.Config) { c.CarryValue = 0.25 },
	},
	{
		name:  "carrybig",
		about: "twice the hands (capacity 6)",
		apply: func(c *engine.Config) { c.CarryCapacity = 6 },
	},
	{
		name:  "carrythree",
		about: "three items at a time, which is where the sweep first went negative",
		apply: func(c *engine.Config) { c.CarryCapacity = 3 },
	},
	{
		name:  "carrydear",
		about: "a full load doubles what moving costs",
		apply: func(c *engine.Config) { c.CarryCost = 1 },
	},
	{
		name:  "carrykeen",
		about: "food in hand is worth as much as food in the stomach (CarryValue 1)",
		apply: func(c *engine.Config) { c.CarryValue = 1 },
	},
	{
		name:  "carrybooks",
		about: "what is held stops counting against the world's allowance: does carrying cost the population by withdrawing food?",
		apply: func(c *engine.Config) { c.CarryOffTheBooks = true },
	},
	{
		name:  "carrybooksbig",
		about: "the same with twice the hands, where the withdrawal would be largest",
		apply: func(c *engine.Config) { c.CarryOffTheBooks, c.CarryCapacity = true, 6 },
	},
	{
		name:  "carrykeeps",
		about: "#77 with hands: what is carried does not rot",
		apply: func(c *engine.Config) { c.CarriedMeatKeeps = true },
	},
	{
		name:  "countrycarry",
		about: "carrying on the map that is played, where the ground already multiplies what moving costs",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.WatersideFood = mapCountry, 1, 1
		},
	},
	{
		name:  "countrycarryoff",
		about: "the same map with no hands: the pair that says what carrying is worth where the ground is dear",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.WatersideFood = mapCountry, 1, 1
			c.CarryCapacity = 0
		},
	},
	// Stage 42: fish. The water finally holds something worth being in it
	// for, and the pair that says what that is worth is the same map with the
	// fish taken out - every water arm has them by default now, exactly as
	// every water arm started drowning people when stage 34 landed.
	{
		name:  "riverfish",
		about: "42: a quarter of what the world grows comes up in the water. The control is river",
		apply: func(c *engine.Config) { c.TerrainMap, c.FishShare = mapRiver, 0.25 },
	},
	{
		name:  "riverfishlots",
		about: "half of it in the water, for the shape of the dose",
		apply: func(c *engine.Config) { c.TerrainMap, c.FishShare = mapRiver, 0.5 },
	},
	{
		name:  "riverfishall",
		about: "control: enemies fish too, which is the species rule this stage does not change by default",
		apply: func(c *engine.Config) { c.TerrainMap, c.FishShare, c.FishForAll = mapRiver, 0.25, true },
	},
	{
		name:  "riverfishswim",
		about: "42 with swimming (38b): does a body that knows the water get more out of what is in it?",
		apply: func(c *engine.Config) { c.TerrainMap, c.FishShare, c.SkillBirthplace = mapRiver, 0.25, 0.5 },
	},
	{
		name:  "countryfish",
		about: "the map that is played, with fish in its river",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.WatersideFood = mapCountry, 1, 1
			c.FishShare = 0.25
		},
	},
	{
		name:  "countryfishkeeps",
		about: "the pair for it: a landed fish keeps for ever, as it did before stage 49",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.WatersideFood = mapCountry, 1, 1
			c.FishShare, c.LandedFishKeeps = 0.25, true
		},
	},
	{
		name:  "countrynofish",
		about: "the same map with the water empty: the pair for the world a game is played in",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.WatersideFood = mapCountry, 1, 1
		},
	},
	// Stage 46: throwing. The one rule in this run that can break the world,
	// so it is measured with TrueRetaliation printed beside the population
	// (#71) and on both maps, because the supply of stones differs fourfold
	// between them (stage 45).
	{
		name:  "throwing",
		about: "46: a body with a stone can throw it, on the played map",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.WatersideFood = mapCountry, 1, 1
			c.Stones, c.Throwing = 60, true
		},
	},
	{
		name:  "throwingoff",
		about: "the pair for it: the same stones lying about and nobody able to throw one",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.WatersideFood = mapCountry, 1, 1
			c.Stones = 60
		},
	},
	{
		name:  "throwrough",
		about: "46 where the ammunition is: broken ground everywhere, one body in two with a stone in sight",
		apply: func(c *engine.Config) { c.TerrainMap, c.Stones, c.Throwing = mapRough, 60, true },
	},
	{
		name:  "throwroughoff",
		about: "the pair for that one",
		apply: func(c *engine.Config) { c.TerrainMap, c.Stones = mapRough, 60 },
	},
	{
		name:  "throwhard",
		about: "a stone that hurts as much as three ticks of a fist, to see the shape of the danger",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.Stones, c.Throwing = mapRough, 60, true
			c.ThrowDamage = 18
		},
	},
	// What the played world is made of, one rule at a time. The viewer turns
	// several of these on together (cmd/devview -terrain), and a population
	// that falls when they are combined is worth finding out about before
	// anybody plays in it.
	{
		name:  "playedbase",
		about: "the played map with terrain, the food tied to it and a rich bank",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.WatersideFood = mapCountry, 1, 1
		},
	},
	{
		name:  "playedfish",
		about: "... and fish in the river",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.WatersideFood = mapCountry, 1, 1
			c.FishShare = 0.25
		},
	},
	{
		name:  "playedcrop",
		about: "... and a fifth of what grows needing knowing",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.WatersideFood = mapCountry, 1, 1
			c.FishShare, c.SpecialtyShare = 0.25, 0.2
		},
	},
	{
		name:  "playedcropmild",
		about: "... with the awkward crop at a tenth and easier to get out of the ground",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.WatersideFood = mapCountry, 1, 1
			c.FishShare, c.SpecialtyShare, c.SpecialtyCatch = 0.25, 0.08, 0.5
		},
	},
	{
		name:  "playedskills",
		about: "... and bodies born somewhere learning what that place teaches",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.WatersideFood = mapCountry, 1, 1
			c.FishShare, c.SpecialtyShare, c.SkillBirthplace = 0.25, 0.2, 0.5
		},
	},
	{
		name:  "playedall",
		about: "... and stones to throw: the whole of what the viewer lays out",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.WatersideFood = mapCountry, 1, 1
			c.FishShare, c.SpecialtyShare, c.SkillBirthplace = 0.25, 0.2, 0.5
			c.Stones, c.Throwing, c.HighGroundCover = 60, true, 0.3
		},
	},
	// Stage 51: money. The value is what it will buy and nothing else (#73),
	// so the discount is the whole stage: at one a coin is worth exactly the
	// meal it claims and nobody gains by trading, above one money is worth
	// more than food, and below one every purchase is worth making to the
	// buyer and none to the seller except for the weight it saves.
	{
		name:  "coins",
		about: "51: money on the played map, worth half the meal it will buy",
		apply: func(c *engine.Config) {
			playedMap(c)
			c.OfferTicks, c.Coins = 30, 60
		},
		stores: playedStores,
	},
	{
		name:  "coinsnone",
		about: "the pair for it: the same world with no money in it",
		apply: func(c *engine.Config) {
			playedMap(c)
			c.OfferTicks = 30
		},
		stores: playedStores,
	},
	{
		name:  "coinsidle",
		about: "the placebo: the coins are lying there and nobody values one",
		apply: func(c *engine.Config) {
			playedMap(c)
			c.OfferTicks, c.Coins, c.CoinValue = 30, 60, 0
		},
		stores: playedStores,
	},
	{
		name:  "coinsdeaf",
		about: "the information taken out: money, and nobody crying what they have to sell",
		apply: func(c *engine.Config) {
			playedMap(c)
			c.Coins = 60
		},
		stores: playedStores,
	},
	{
		name:  "coinspar",
		about: "a coin worth exactly the meal it claims: the point where nobody gains",
		apply: func(c *engine.Config) {
			playedMap(c)
			c.OfferTicks, c.Coins, c.CoinValue = 30, 60, 1
		},
		stores: playedStores,
	},
	{
		name:  "coinscheap",
		about: "a coin worth a fifth of the meal: good for buyers, bad for sellers",
		apply: func(c *engine.Config) {
			playedMap(c)
			c.OfferTicks, c.Coins, c.CoinValue = 30, 60, 0.2
		},
		stores: playedStores,
	},
	// Stage 52: cooking. The default world has it, so the arms turn it off
	// rather than on - and the one that turns it off is a placebo rather than
	// a shorter vocabulary, because the word costs the same whether or not
	// anybody uses it.
	// Stage 55a: the way somebody went. The pair that matters is not on
	// against off but the direction, in the shape stage 54 found works: the
	// same candidate, the same weight, the same price, pointing the wrong
	// way. If that does as well, what the option buys is having somewhere to
	// go rather than having remembered where they went.
	{
		name:  "missing",
		about: "55a: after the one it thinks best of, the way it went",
		apply: func(c *engine.Config) { c.LonelyValue = 20 },
	},
	{
		name:  "missingwrong",
		about: "the control: the same candidate at the same price, pointing the wrong way",
		apply: func(c *engine.Config) { c.LonelyValue, c.LonelyWrongWay = 20, true },
	},
	{
		name:  "missingweak",
		about: "the same, worth a quarter as much: the low end of the dose",
		apply: func(c *engine.Config) { c.LonelyValue = 5 },
	},
	{
		name:  "missingstrong",
		about: "the same, worth three times as much",
		apply: func(c *engine.Config) { c.LonelyValue = 60 },
	},
	{
		name:  "missinglong",
		about: "a direction that takes four times as long to go stale",
		apply: func(c *engine.Config) { c.LonelyValue, c.LonelyHalfLife = 20, 1600 },
	},
	// Stage 53: who rears, and who may feed. The feeding rule is off by
	// default, so the arms that mean anything turn it on - and the three of
	// them have to be read together, because 53b exists to undo a side effect
	// of 53a rather than on its own account.
	{
		name:  "feedboth",
		about: "53: the mother rears and either parent may feed, with feeding on",
		apply: func(c *engine.Config) { c.ParentFeedShare = 0.5 },
	},
	{
		name:  "feedmother",
		about: "53a without 53b: the mother rears and only the guardian may feed, which shuts fathers out",
		apply: func(c *engine.Config) { c.ParentFeedShare, c.ParentFeedByKin = 0.5, false },
	},
	{
		name:  "feedold",
		about: "the world before 53a: whichever parent came first rears, and only it may feed",
		apply: func(c *engine.Config) {
			c.ParentFeedShare, c.ParentFeedByKin, c.GuardianIsMother = 0.5, false, false
		},
	},
	{
		name:  "rearold",
		about: "53a on its own, with feeding off as it is by default: only who rears changes",
		apply: func(c *engine.Config) { c.GuardianIsMother = false },
	},
	// Stage 53: the split itself. The directions come from the roles of 53a
	// (#85): the mother is tied to a radius and the father ranges. The arm to
	// read it against is the one with the roles and no split, because the
	// difference between the sexes is put in by hand and proves nothing - what
	// is worth reading is where the population's whole budget goes.
	{
		name:  "sexes",
		about: "53: the mother reads and remembers, the father ranges and picks",
		apply: func(c *engine.Config) { sexSplit(c, 0.15) },
	},
	{
		name:  "sexesstrong",
		about: "the same split, twice as far",
		apply: func(c *engine.Config) { sexSplit(c, 0.3) },
	},
	{
		name:  "sexesswapped",
		about: "the control: the same split with the sexes exchanged, so the roles and the genes disagree",
		apply: func(c *engine.Config) { sexSplit(c, -0.15) },
	},
	{
		name:  "sexesfed",
		about: "53 in a world where parents feed their children, which is where the roles bite",
		apply: func(c *engine.Config) { sexSplit(c, 0.15); c.ParentFeedShare = 0.5 },
	},
	{
		name:  "sexesnonefed",
		about: "the pair for it: the same feeding world with no split",
		apply: func(c *engine.Config) { c.ParentFeedShare = 0.5 },
	},
	// Stage 54: a mood. The default has none, so the arms turn it on; the
	// pair that matters is not on-against-off but the sign, because the same
	// size of lean with the sign reversed says whether the structure is doing
	// the work or only the magnitude.
	{
		name:  "prowl",
		about: "58: the enemies arrive in some regions more than others - the map's own dangerous country",
		apply: func(c *engine.Config) { c.EnemySpread = 0.6 },
	},
	{
		name:  "prowlweak",
		about: "the same, half as far apart",
		apply: func(c *engine.Config) { c.EnemySpread = 0.3 },
	},
	{
		name:  "prowlhard",
		about: "as far apart as it goes: some regions take almost none of them",
		apply: func(c *engine.Config) { c.EnemySpread = 0.9 },
	},
	{
		name:  "favour",
		about: "57c: the ground favours some genes over others, averaging to one, so that it suits some builds and not others",
		apply: func(c *engine.Config) { c.RegionFavourSpread = 0.3 },
	},
	{
		name:  "favourweak",
		about: "the same taste, half as strong",
		apply: func(c *engine.Config) { c.RegionFavourSpread = 0.15 },
	},
	{
		name:  "favourhard",
		about: "the same taste, twice as strong",
		apply: func(c *engine.Config) { c.RegionFavourSpread = 0.6 },
	},
	{
		name: "favourcarried",
		about: "the control: the same taste, drawn from the ground a body was born on and carried for life " +
			"- the variation without anywhere suiting anybody",
		apply: func(c *engine.Config) {
			c.RegionFavourSpread, c.RegionAbilityCarried = 0.3, true
		},
	},
	{
		name:  "favourcarriedhard",
		about: "the pair for favourhard",
		apply: func(c *engine.Config) {
			c.RegionFavourSpread, c.RegionAbilityCarried = 0.6, true
		},
	},
	{
		name:  "ground",
		about: "57: some ground makes a body better at everything while it stands on it",
		apply: func(c *engine.Config) { c.RegionAbilitySpread = 0.3 },
	},
	{
		name:  "groundweak",
		about: "the same, half as far apart",
		apply: func(c *engine.Config) { c.RegionAbilitySpread = 0.15 },
	},
	{
		name:  "groundhard",
		about: "the same, twice as far apart",
		apply: func(c *engine.Config) { c.RegionAbilitySpread = 0.6 },
	},
	{
		name: "groundcarried",
		about: "the control that matters: the same multiplier, drawn from the ground a body " +
			"was born on and then carried for life - the variation without the reason to stay",
		apply: func(c *engine.Config) {
			c.RegionAbilitySpread, c.RegionAbilityCarried = 0.3, true
		},
	},
	{
		name:  "groundcarriedhard",
		about: "the pair for groundhard",
		apply: func(c *engine.Config) {
			c.RegionAbilitySpread, c.RegionAbilityCarried = 0.6, true
		},
	},
	{
		name:  "mood",
		about: "54: a frightened body weighs being worn down higher, a fed one lower",
		apply: func(c *engine.Config) { c.MoodWeight = 0.5 },
	},
	{
		name:  "moodinverted",
		about: "the control: the same lean with the sign reversed, so fright makes bodies bold",
		apply: func(c *engine.Config) { c.MoodWeight = -0.5 },
	},
	{
		name:  "moodstrong",
		about: "the same lean, twice as far",
		apply: func(c *engine.Config) { c.MoodWeight = 1 },
	},
	{
		name:  "moodfear",
		about: "dread only: the arm that asks stage 12a's question, since cheer is six times the size of fear and swamps it",
		apply: func(c *engine.Config) { c.MoodWeight, c.MoodCheerGain = 0.5, 0 },
	},
	{
		name:  "moodfearlong",
		about: "dread only, fading in six hundred ticks rather than a hundred and fifty",
		apply: func(c *engine.Config) { c.MoodWeight, c.MoodCheerGain, c.MoodHalfLife = 0.5, 0, 600 },
	},
	{
		name:  "moodfearhard",
		about: "dread only, at a gain that makes it a mood rather than a rounding error",
		apply: func(c *engine.Config) {
			c.MoodWeight, c.MoodCheerGain, c.MoodDreadGain = 0.5, 0, 30
		},
	},
	{
		name:  "moodhard",
		about: "both, at gains that make the two sides the same size",
		apply: func(c *engine.Config) {
			c.MoodWeight, c.MoodDreadGain, c.MoodCheerGain = 0.5, 30, 3
		},
	},
	{
		name:  "moodhardinverted",
		about: "the control for it: the same sizes, the sign reversed",
		apply: func(c *engine.Config) {
			c.MoodWeight, c.MoodDreadGain, c.MoodCheerGain = -0.5, 30, 3
		},
	},
	{
		name: "moodflat",
		about: "the control that matters: the same average lean, held constant, with no " +
			"history in it at all - if this does as well, what works is the boldness and not the feeling",
		apply: func(c *engine.Config) { c.ShockRisk *= 1 - 0.5*0.07 },
	},
	{
		name:  "moodcheer",
		about: "cheer only: the pair for it",
		apply: func(c *engine.Config) { c.MoodWeight, c.MoodDreadGain = 0.5, 0 },
	},
	{
		name:  "moodshort",
		about: "a mood that fades in fifty ticks: standing a fifth of the time",
		apply: func(c *engine.Config) { c.MoodWeight, c.MoodHalfLife = 0.5, 50 },
	},
	{
		name:  "moodlong",
		about: "a mood that fades in twelve hundred: standing two thirds of the time, which is nearly a constant",
		apply: func(c *engine.Config) { c.MoodWeight, c.MoodHalfLife = 0.5, 1200 },
	},
	{
		name:  "moodplayed",
		about: "54 on the map that is played",
		apply: func(c *engine.Config) {
			playedMap(c)
			c.MoodWeight, c.MoodDreadGain, c.MoodCheerGain = 0.5, 30, 3
		},
		stores: playedStores,
	},
	{
		name:   "moodplayednone",
		about:  "the pair for it: the played map with no mood",
		apply:  playedMap,
		stores: playedStores,
	},
	{
		name:  "nocook",
		about: "52: the placebo - cooking is worth nothing, so nobody ever does it",
		apply: func(c *engine.Config) { c.CookVitality = 0 },
	},
	{
		name:  "cookprivate",
		about: "cooking that does not survive changing hands: the trade half taken out",
		apply: func(c *engine.Config) { c.CookSurvivesHands = false },
	},
	// The pair that isolates the trade half without letting anybody cook the
	// same thing twice. cookprivate takes the cooking off an item when it
	// changes hands, which the receiver can undo for twenty ticks; these two
	// take away the reason to hand anything over at all, and read against
	// nocook and nocookmean they say how much of cooking's worth needs
	// somebody else.
	{
		name:  "cookmean",
		about: "cooking, and no reason to hand anything to anybody",
		apply: func(c *engine.Config) { c.AffinityGift = 0 },
	},
	{
		name:  "nocookmean",
		about: "the pair for it: no cooking either, so the two differences can be read against each other",
		apply: func(c *engine.Config) { c.AffinityGift, c.CookVitality = 0, 0 },
	},
	{
		name:  "cookslow",
		about: "cooking at three times the price in time",
		apply: func(c *engine.Config) { c.CookTicks = 60 },
	},
	{
		name:  "cookweak",
		about: "cooking worth a fifth of a body rather than half of one",
		apply: func(c *engine.Config) { c.CookVitality = 0.1 },
	},
	{
		name:   "cookplayed",
		about:  "52 on the map that is played, where food is scarce",
		apply:  playedMap,
		stores: playedStores,
	},
	{
		name:   "cookplayednone",
		about:  "the pair for it: the played map with cooking worth nothing",
		apply:  func(c *engine.Config) { playedMap(c); c.CookVitality = 0 },
		stores: playedStores,
	},
	{
		name:  "cookskill",
		about: "52b: the played map where an ignorant cook wastes half of it, and the skill buys it back",
		apply: func(c *engine.Config) {
			playedMap(c)
			c.SkillBirthplace, c.CookQuality = 0.5, 0.5
		},
		stores: playedStores,
	},
	{
		name:  "cookskillnone",
		about: "the pair for it: the same world where knowing how to cook buys nothing",
		apply: func(c *engine.Config) {
			playedMap(c)
			c.SkillBirthplace, c.CookQuality, c.SkillCookRelief = 0.5, 0.5, 0
		},
		stores: playedStores,
	},
	{
		name:  "coinsflat",
		about: "51 on the flat world, which is richer and has more to spare",
		apply: func(c *engine.Config) { c.OfferTicks, c.Coins = 30, 60 },
	},
	{
		name:  "coinsflatnone",
		about: "the pair for it: the flat world with no money",
		apply: func(c *engine.Config) { c.OfferTicks = 30 },
	},
	// The carry valuation, found the wrong way round while stage 50 was being
	// written: what a thing kept for later was worth came out as a loss, so
	// nothing was ever picked up on purpose. The arm to read the fix against
	// is the world every figure for stages 40 to 49 was measured in.
	{
		name:  "carryfix",
		about: "what a held item is worth, reckoned as the meal it would be when needed",
		apply: func(c *engine.Config) {},
	},
	{
		name:  "carryback",
		about: "the world stages 40-49 were measured in: that value with its sign the wrong way",
		apply: func(c *engine.Config) { c.CarryPricedBackwards = true },
	},
	{
		name:  "carryfixplayed",
		about: "the fix on the played map",
		apply: playedMap,
	},
	{
		name:  "carrybackplayed",
		about: "the pair for it on the played map",
		apply: func(c *engine.Config) {
			playedMap(c)
			c.CarryPricedBackwards = true
		},
	},
	// Stage 50: a place to put things, and knowing where it is. The stores are
	// laid out the way the terrain is, so an arm has to put them there.
	{
		name:  "stores",
		about: "50: caches on the played map, and knowing where one is has to be learned",
		apply: func(c *engine.Config) {
			playedMap(c)
			c.OfferTicks = 30
		},
		stores: playedStores,
	},
	{
		name:  "storesnone",
		about: "the pair for it: the same world with no caches in it",
		apply: func(c *engine.Config) {
			playedMap(c)
			c.OfferTicks = 30
		},
	},
	{
		name:  "storesknown",
		about: "the control: the caches are there and everybody knows every one of them",
		apply: func(c *engine.Config) {
			playedMap(c)
			c.OfferTicks, c.StoresKnownToAll = 30, true
		},
		stores: playedStores,
	},
	{
		name:  "storesidle",
		about: "the placebo: the caches are there, everybody knows them, nobody ever uses one",
		apply: func(c *engine.Config) {
			playedMap(c)
			c.OfferTicks, c.StoresKnownToAll, c.StoreValue = 30, true, 0
		},
		stores: playedStores,
	},
	{
		name:   "storesflat",
		about:  "50 on the flat world, which is at its food cap a tenth of the time",
		apply:  func(c *engine.Config) { c.OfferTicks = 30 },
		stores: flatStores,
	},
	{
		name:  "storesflatnone",
		about: "the pair for it: the flat world with no caches",
		apply: func(c *engine.Config) { c.OfferTicks = 30 },
	},
	{
		name:  "storesbig",
		about: "caches that hold four times as much: is six items the limit that bites?",
		apply: func(c *engine.Config) {
			playedMap(c)
			c.OfferTicks, c.StoreCapacity = 30, 24
		},
		stores: playedStores,
	},
	// Stage 49: crying what is in the hand. The target was counted before the
	// rule was written: 27% of bodies hold something and 85% of those are not
	// hungry, so there is a surplus - but 52% of holders can already see a
	// hungry body with a free hand at a mean distance of 64, and the whole of
	// the buyer's side is 2.9% (the share of hungry moments spent with no food
	// in sight and somebody holding some within sight). The arm to read it
	// against is the same world with the word taken out.
	{
		name:  "criers",
		about: "49: a body can stand still and hold out what it has, on the played map",
		apply: func(c *engine.Config) {
			playedMap(c)
			c.OfferTicks = 30
		},
	},
	{
		name:  "criersdead",
		about: "the pair for it: the same world, and nobody can say what they have",
		apply: playedMap,
	},
	{
		name:  "criersdeaf",
		about: "control: the cry is made and costs its time, and nobody can read the hand",
		apply: func(c *engine.Config) {
			playedMap(c)
			c.OfferTicks, c.WaresSeen = 30, false
		},
	},
	{
		name:  "criersflatdeaf",
		about: "the same control on the flat world: crying that nobody can read",
		apply: func(c *engine.Config) { c.OfferTicks, c.WaresSeen = 30, false },
	},
	{
		name:  "criersflat",
		about: "49 on the flat world, where the only things worth holding are food",
		apply: func(c *engine.Config) { c.OfferTicks = 30 },
	},
	{
		name:  "criersflatdead",
		about: "the pair for it: the flat world with the word taken out",
		apply: func(c *engine.Config) {},
	},
	{
		name:  "crierslong",
		about: "a cry that runs three times as long: worth more, and costs more",
		apply: func(c *engine.Config) {
			playedMap(c)
			c.OfferTicks = 90
		},
	},
	{
		name:  "criershands",
		about: "49 with room for three things: hands that can hold dinner and stock",
		apply: func(c *engine.Config) {
			playedMap(c)
			c.OfferTicks, c.CarryCapacity = 30, 3
		},
	},
	// Stage 48: handing something over. The gate the rest of the economy
	// waits on (#73): coins and warehouses carry nothing if nobody would give
	// a thing away in the first place. The arm to read it against is the same
	// world where a gift earns nothing.
	{
		name:  "gifts",
		about: "48: handing something over earns being on good terms, both ways, on the played map",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.WatersideFood = mapCountry, 1, 1
			c.FishShare, c.SpecialtyShare, c.SpecialtyCatch = 0.25, 0.08, 0.5
			c.Stones = 60
		},
	},
	{
		name:  "giftsdead",
		about: "control: the word is there and a gift earns nothing",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.WatersideFood = mapCountry, 1, 1
			c.FishShare, c.SpecialtyShare, c.SpecialtyCatch = 0.25, 0.08, 0.5
			c.Stones = 60
			c.AffinityGift = 0
		},
	},
	{
		name:  "giftsflat",
		about: "48 on the flat world, where the only things worth holding are food",
		apply: func(c *engine.Config) {},
	},
	{
		name:  "giftsflatdead",
		about: "the pair for it: the same world, and a gift earns nothing",
		apply: func(c *engine.Config) { c.AffinityGift = 0 },
	},
	{
		name:  "giftsdear",
		about: "a gift worth three times as much goodwill: does the price change who gives?",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.WatersideFood = mapCountry, 1, 1
			c.FishShare, c.SpecialtyShare, c.SpecialtyCatch = 0.25, 0.08, 0.5
			c.Stones = 60
			c.AffinityGift = 18
		},
	},
	{
		name:  "giftshands",
		about: "48 with room for three things: a body that can hold its dinner and something to give",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.WatersideFood = mapCountry, 1, 1
			c.FishShare, c.SpecialtyShare, c.SpecialtyCatch = 0.25, 0.08, 0.5
			c.Stones = 60
			c.CarryCapacity = 3
		},
	},
	// Stage 47: knowing how to throw. The target is written down before it is
	// measured (stage 44's habit): a throw from arm's length lands nine times
	// in ten and the measured rate is 0.64, so distance costs twenty-six
	// points of accuracy and that is the whole of what this can win back.
	{
		name:  "throwskill",
		about: "47: bodies born where the stones are learn to put one where it was meant to go",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.Stones, c.Throwing = mapRough, 60, true
			c.SkillBirthplace = 0.5
		},
	},
	{
		name:  "throwskilldead",
		about: "control: the same skill learned, taking the same room, worth nothing",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.Stones, c.Throwing = mapRough, 60, true
			c.SkillBirthplace, c.SkillThrowRelief = 0.5, 0
		},
	},
	{
		name:  "throwskillplenty",
		about: "47 at a dose that cannot be missed: stones everywhere and everybody born among them learning it well",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.Stones, c.Throwing = mapRough, 240, true
			c.SkillBirthplace = 1
		},
	},
	{
		name:  "throwskillplentydead",
		about: "control for it",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.Stones, c.Throwing = mapRough, 240, true
			c.SkillBirthplace, c.SkillThrowRelief = 1, 0
		},
	},
	{
		name:  "throwskillwits",
		about: "sweep: aim capped by wits rather than by how well the world is read",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.Stones, c.Throwing = mapRough, 60, true
			c.SkillBirthplace = 0.5
			c.SkillAptitude[engine.SkillThrow] = engine.GeneIntelligence
		},
	},
	// Stage 45: stones. Nothing values one yet - what they are for is stage
	// 46 - so what these arms measure is the supply: how many there are, how
	// often a body has one in sight, and how far away the nearest is. A
	// ranged attack nobody has a stone for is a rule that never fires, and
	// that is worth knowing before it is written.
	{
		name:  "stones",
		about: "45: sixty stones scattered over the broken ground of the played map",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.WatersideFood = mapCountry, 1, 1
			c.Stones = 60
		},
	},
	{
		name:  "stonesfew",
		about: "a fifth as many: what the supply looks like when the ground is stingy",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.WatersideFood = mapCountry, 1, 1
			c.Stones = 12
		},
	},
	{
		name:  "stonesmany",
		about: "three times as many, which is about one for every body alive",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.WatersideFood = mapCountry, 1, 1
			c.Stones = 180
		},
	},
	{
		name:  "stonesrough",
		about: "the rough map rather than the played one: nothing but broken ground to lie on",
		apply: func(c *engine.Config) { c.TerrainMap, c.Stones = mapRough, 60 },
	},
	// Stage 44: the awkward crop. The plant side of what an economy would
	// need somebody to be able to do that somebody else cannot, and the
	// second instance of the form stage 42 built. The control throughout is
	// the same crop with the trick worth nothing: it is grown, it is awkward,
	// and knowing about it buys no more of it.
	{
		name:  "special",
		about: "44: a fifth of what grows needs knowing, unevenly spread, and bodies born where it grows learn the trick",
		apply: func(c *engine.Config) { c.SpecialtyShare, c.SkillBirthplace = 0.2, 0.5 },
	},
	{
		name:  "specialdead",
		about: "control: the same crop, the same trick learned and taking the same room, worth nothing",
		apply: func(c *engine.Config) {
			c.SpecialtyShare, c.SkillBirthplace, c.SkillHarvestRelief = 0.2, 0.5, 0
		},
	},
	{
		name:  "specialnobody",
		about: "control: the crop and nobody knowing anything - what an awkward crop costs a world on its own",
		apply: func(c *engine.Config) { c.SpecialtyShare = 0.2 },
	},
	{
		name:  "speciallots",
		about: "half of what grows needs knowing: the target as large as it can be made",
		apply: func(c *engine.Config) { c.SpecialtyShare, c.SkillBirthplace = 0.5, 0.5 },
	},
	{
		name:  "speciallotsdead",
		about: "control for it",
		apply: func(c *engine.Config) {
			c.SpecialtyShare, c.SkillBirthplace, c.SkillHarvestRelief = 0.5, 0.5, 0
		},
	},
	{
		name:  "specialflat",
		about: "the same crop spread evenly over the world: nowhere is a place to be a specialist",
		apply: func(c *engine.Config) {
			c.SpecialtyShare, c.SkillBirthplace, c.SpecialtySpread = 0.2, 0.5, 0
		},
	},
	// Stage 43: the two ways of fishing. One resource and two trades - in the
	// river most attempts land and the drowning rule charges by the tick, on
	// the bank nothing charges anything and most attempts fail - so what a
	// body has learned decides which of them is worth its time. The control
	// throughout is the same world with the skills learned, taking the same
	// room, and doing nothing.
	{
		name:  "riverchancy",
		about: "43's world without anybody knowing anything: a fish is landed three times in four in the river and three in ten from the bank",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.FishShare = mapRiver, 0.25
			c.FishCatchWater, c.FishCatchBank = 0.75, 0.3
		},
	},
	{
		name:  "riverangle",
		about: "43: and bodies born by the water learn one of the two ways of doing it",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.FishShare, c.SkillBirthplace = mapRiver, 0.25, 0.5
			c.FishCatchWater, c.FishCatchBank = 0.75, 0.3
		},
	},
	{
		name:  "riverangledead",
		about: "control: the same skills learned, taking the same room, worth nothing",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.FishShare, c.SkillBirthplace = mapRiver, 0.25, 0.5
			c.FishCatchWater, c.FishCatchBank = 0.75, 0.3
			c.SkillFishRelief = 0
		},
	},
	{
		name:  "riverangleplenty",
		about: "43 at a dose that cannot be missed: everybody born by the water learns it well, and the two grounds are far apart",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.FishShare, c.SkillBirthplace = mapRiver, 0.25, 1
			c.FishCatchWater, c.FishCatchBank = 0.9, 0.1
			c.SkillFishReach = 4
		},
	},
	{
		name:  "riverangleplentydead",
		about: "control for it: the same learning, the same room, worth nothing",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.FishShare, c.SkillBirthplace = mapRiver, 0.25, 1
			c.FishCatchWater, c.FishCatchBank = 0.9, 0.1
			c.SkillFishReach, c.SkillFishRelief = 4, 0
		},
	},
	{
		name:  "riverangleswim",
		about: "43 with swimming as well, which is the whole water family at once",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.FishShare, c.SkillBirthplace = mapRiver, 0.25, 0.5
			c.FishCatchWater, c.FishCatchBank = 0.75, 0.3
			c.SkillSwimRelief = 1
		},
	},
	// Stage 41: the surplus. What a party cannot carry away stops being
	// theirs to wait for. The pair that says what it bought is the default -
	// where the claim covers the whole carcass - and the ceiling is an arm
	// with no claim at all.
	{
		name:  "meatkept",
		about: "41 off: a claim covers the whole carcass, which is the world stages 11 to 40 were measured in",
		apply: func(c *engine.Config) { c.MeatSurplusFree = false },
	},
	{
		name:  "meatnoclaim",
		about: "the ceiling: no claim at all, so a kill is anybody's the moment it falls",
		apply: func(c *engine.Config) { c.MeatClaimTicks = 0 },
	},
	{
		name:  "meatmorekept",
		about: "more meat (MeatPerBudget 60) with the claim covering all of it: the pair that says what opening the surplus did",
		apply: func(c *engine.Config) { c.MeatSurplusFree, c.MeatPerBudget = false, 60 },
	},
	// What the spoil clock is worth at all. Asked before designing "meat does
	// not rot while it is being carried or offered" (stages 40 and 49): if
	// meat that never rots on the ground buys nothing, meat that does not rot
	// in somebody's hands cannot buy much either, and the rule would be an
	// unpriced benefit of the kind stage 17b showed runs to the ceiling.
	{
		name:  "meatkeeps",
		about: "a carcass lasts four times as long (MeatSpoilTicks 3600)",
		apply: func(c *engine.Config) { c.MeatSpoilTicks = 3600 },
	},
	{
		name:  "meatforever",
		about: "a carcass never rots: the ceiling on anything preservation could buy",
		apply: func(c *engine.Config) { c.MeatSpoilTicks = 1 << 30 },
	},
	{
		name:  "meatrots",
		about: "a carcass lasts a third as long (300): the same question from the other side",
		apply: func(c *engine.Config) { c.MeatSpoilTicks = 300 },
	},
	{
		name:  "meatmoreoff",
		about: "39a with no healing: the amount on its own, as it was measured before healing became the default",
		apply: func(c *engine.Config) { c.MeatPerBudget, c.MeatVitality = 60, 0 },
	},
	// Stage 38b: the second skill. Foraging is seeded by what the ground
	// provides rather than by what it is made of, so unlike the first it
	// needs no terrain - which makes the flat world the clean arm for it, and
	// the rough map the one where the two skills compete for the same room.
	{
		name:  "forageskill",
		about: "a flat world where a body born on thin ground learns to make a mouthful go further (38b)",
		apply: func(c *engine.Config) { c.SkillBirthplace = 0.5 },
	},
	{
		name:  "foragedead",
		about: "control: the skill is learned and takes the room, and the mouthful is worth what it always was",
		apply: func(c *engine.Config) { c.SkillBirthplace, c.SkillForageRelief = 0.5, 0 },
	},
	{
		name:  "foragenoteach",
		about: "control: foraging can be born with and inherited, not caught",
		apply: func(c *engine.Config) { c.SkillBirthplace, c.SkillsSpread = 0.5, false },
	},
	{
		name:  "foragelegs",
		about: "sweep: what a body gets out of foraging is capped by its legs, not its memory",
		apply: func(c *engine.Config) {
			c.SkillBirthplace = 0.5
			c.SkillAptitude[engine.SkillForage] = engine.GeneSpeed
		},
	},
	// Stage 38b, the third skill: swimming. The river is the map it means
	// anything on, and the base to read it against is river - stages 34 to 36
	// as they stand, with nobody knowing anything.
	{
		name:  "riverswim",
		about: "the river, and a body born by it knows how to be in it (38b)",
		apply: func(c *engine.Config) { c.TerrainMap, c.SkillBirthplace = mapRiver, 0.5 },
	},
	{
		name:  "swimdead",
		about: "control: the skill is learned and takes the room, and the water is as deadly as ever",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.SkillBirthplace, c.SkillSwimRelief = mapRiver, 0.5, 0
		},
	},
	{
		name:  "swimnoteach",
		about: "control: swimming can be born with and inherited, not caught",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.SkillBirthplace, c.SkillsSpread = mapRiver, 0.5, false
		},
	},
	{
		name:  "swimfed",
		about: "the river bank rich as well (36 on): does knowing the water change where bodies stand when the food is there?",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.SkillBirthplace, c.WatersideFood = mapRiver, 0.5, 1
		},
	},
	// Stage 31: what a killing leaves with the people who saw it. The base
	// is baseline - the rule is on by default - so the arms here are the
	// controls: each half off, both off, and the reading half at weights
	// either side of the default.
	{
		name:  "nowitness",
		about: "control: a killing teaches the onlookers nothing (31 off, both halves)",
		apply: func(c *engine.Config) { c.KillWitnessFactor, c.AffinityWitnessKill = 0, 0 },
	},
	{
		name:  "noreading",
		about: "control: seeing a killing is worth nothing about the killer's strength (31a off)",
		apply: func(c *engine.Config) { c.KillWitnessFactor = 0 },
	},
	{
		name:  "noavenge",
		about: "control: killing one of theirs earns nothing from the onlookers (31b off)",
		apply: func(c *engine.Config) { c.AffinityWitnessKill = 0 },
	},
	{
		name:  "witnessnolooks",
		about: "31a, with what is seen kept out of the picture of what a build is worth: is it the readings or the line?",
		apply: func(c *engine.Config) { c.KillWitnessLooks = false },
	},
	{
		name:  "readingnolooks",
		about: "the reading half alone, kept out of the line (31b off, 31a on, looks untouched)",
		apply: func(c *engine.Config) { c.KillWitnessLooks, c.AffinityWitnessKill = false, 0 },
	},
	{
		name:  "witnesscoarse",
		about: "sweep: a witnessed killing is worth a quarter of a paid look",
		apply: func(c *engine.Config) { c.KillWitnessFactor = 4 },
	},
	{
		name:  "witnessfine",
		about: "sweep: a witnessed killing is worth as much as a paid look (the thing #55 forbids)",
		apply: func(c *engine.Config) { c.KillWitnessFactor = 1 },
	},
	{
		name:  "avengehigh",
		about: "sweep: seeing one of your own kill an enemy is worth as much as taking part",
		apply: func(c *engine.Config) { c.AffinityWitnessKill = 6 },
	},
	// Stage 37: whether the river divides the world. Nothing is added to the
	// world here - these are controls, and the reading is done by the metrics
	// (bankSplit, crossShare, crossIndex, crossDry, bankGeneGap,
	// bankCountryGap).
	//
	// The pairs to read are river against ponds, and waterrichlink - the world
	// a map would actually be played on - against pondslink. What separates
	// them is only the shape of the water: the area, the cost of crossing it
	// and the ground that can drown a body are the same on both sides of each
	// pair, so anything that moves is the line and not the terrain.
	{
		name:  "ponds",
		about: "control for river: the same water, in six pools instead of one line (37)",
		apply: func(c *engine.Config) { c.TerrainMap = mapPonds },
	},
	{
		name:  "pondslink",
		about: "control for waterrichlink: the same water in pools, both ties on (37)",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.WatersideFood, c.TerrainFoodCorrelation = mapPonds, 1, 1
		},
	},
	{
		name:  "gorge",
		about: "a river four cells wide - wider than sight - down the middle (37)",
		apply: func(c *engine.Config) { c.TerrainMap = mapGorge },
	},
	{
		name:  "pools",
		about: "control for gorge: the same forty-eight cells of water in twelve pools (37)",
		apply: func(c *engine.Config) { c.TerrainMap = mapPools },
	},
	{
		name:  "dangerlink",
		about: "the river believed, with the food following the ground (33 on): is the water only stood in because the food is there?",
		apply: func(c *engine.Config) { c.TerrainMap, c.TerrainFoodCorrelation = mapRiver, 1 },
	},
	{
		name:  "dangerlinknone",
		about: "control for dangerlink: the same world with the danger never weighed",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.RegionDangerTicks = mapRiver, 1, 0
		},
	},
	{
		name:  "edgedanger",
		about: "the river against the western edge, believed: is the water avoided when it is not on the way?",
		apply: func(c *engine.Config) { c.TerrainMap = mapRiverEdge },
	},
	{
		name:  "edgedangernone",
		about: "control for edgedanger: the same edge river, the danger never weighed",
		apply: func(c *engine.Config) { c.TerrainMap, c.RegionDangerTicks = mapRiverEdge, 0 },
	},
	{
		name:  "dangerfine",
		about: "sweep: the same river believed at 12x9 regions - is the belief too coarse to steer by?",
		apply: func(c *engine.Config) { c.TerrainMap, c.RegionCols, c.RegionRows = mapRiver, 12, 9 },
	},
	{
		name:  "dangerfinenone",
		about: "control for dangerfine: 12x9 regions and the danger never weighed",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.RegionCols, c.RegionRows, c.RegionDangerTicks = mapRiver, 12, 9, 0
		},
	},
	{
		name:  "countrydanger",
		about: "the whole country with the danger believed (35 on the map a world would be played on)",
		apply: func(c *engine.Config) { c.TerrainMap = mapCountry },
	},
	{
		name:  "drownblind",
		about: "control: the river drowns exactly as often, and nobody can feel it (34 priced out)",
		apply: func(c *engine.Config) { c.TerrainMap, c.DrownKnown = mapRiver, false },
	},
	{
		name:  "countrydry",
		about: "control: the whole country with the drowning turned off (34 off)",
		apply: func(c *engine.Config) { c.TerrainMap, c.DrownChancePerTick = mapCountry, 0 },
	},
	{
		name:  "softrough",
		about: "sweep: the same broken country at half the penalty",
		apply: func(c *engine.Config) { c.TerrainMap, c.RoughMoveCost = mapRough, 1.5 },
	},
	{
		name:  "hardrough",
		about: "sweep: the same broken country at four times the cost",
		apply: func(c *engine.Config) { c.TerrainMap, c.RoughMoveCost = mapRough, 4 },
	},
	// Stage 29: the going, learned and handed on. The arms are all on the
	// rough map, because a belief about the going says nothing in a world
	// where all the going is the same - the question is whether knowing it
	// changes where the population stands (onDear) and what that is worth.
	{
		name:  "roughlore",
		about: "rough country, and agents learn and tell each other how hard it is",
		apply: func(c *engine.Config) { c.TerrainMap = mapRough },
	},
	{
		name:  "roughnolore",
		about: "control: the same country, and the going is learned but never weighed (29 off)",
		apply: func(c *engine.Config) { c.TerrainMap, c.RegionCostWeight = mapRough, 0 },
	},
	{
		name:  "roughnotold",
		about: "control: the going is weighed but never handed on (29a without 29b)",
		apply: func(c *engine.Config) { c.TerrainMap, c.RegionCostTold = mapRough, false },
	},
	{
		name:  "roughmildlore",
		about: "sweep: the going weighed at half (one multiple of cost = one food in sight)",
		apply: func(c *engine.Config) { c.TerrainMap, c.RegionCostWeight = mapRough, 1 },
	},
	{
		name:  "roughhardlore",
		about: "sweep: the going weighed at double",
		apply: func(c *engine.Config) { c.TerrainMap, c.RegionCostWeight = mapRough, 4 },
	},
	{
		name:  "countrylore",
		about: "the whole country (rough, river, plateau) with the going learned and told",
		apply: func(c *engine.Config) { c.TerrainMap = mapCountry },
	},
	// Stage 33: the food and the ground on the same map. Measured against
	// "roughlore" (stage 29 as it stands, where the two are laid out
	// independently), which is what says whether stage 29's ceiling was the
	// confound or a real limit.
	{
		name:  "roughlink",
		about: "rough country that is also poor country: food follows the going",
		apply: func(c *engine.Config) { c.TerrainMap, c.TerrainFoodCorrelation = mapRough, 1 },
	},
	{
		name:  "roughlinkhalf",
		about: "sweep: the same tie at half strength",
		apply: func(c *engine.Config) { c.TerrainMap, c.TerrainFoodCorrelation = mapRough, 0.5 },
	},
	{
		name:  "roughlinknolore",
		about: "control: food follows the going, and nobody weighs the going (is the tie enough on its own?)",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.RegionCostWeight = mapRough, 1, 0
		},
	},
	{
		name:  "countrylink",
		about: "the whole country with the food following the going",
		apply: func(c *engine.Config) { c.TerrainMap, c.TerrainFoodCorrelation = mapCountry, 1 },
	},
	// Stage 30a: high ground as cover. Measured on the two maps that have any
	// height in them, against the same maps without it.
	//
	// Two firing rates were counted first, and the second is the one that
	// mattered. 28% of attacker-ticks on the plateau map are thrown across a
	// difference in height - plenty of situation - but the first version of
	// the rule multiplied the target's evasion, and a body has an evasion only
	// while it is fighting or running (15.7% of ticks, evasive in 7.7%). It
	// measured as nothing at all, because it was multiplying zero. Cover is a
	// floor now: the bank helps whoever is behind it, guarding or not.
	{
		name:  "plateaucover",
		about: "high ground is harder to hit from below (0.3 of blows miss), on the plateau map",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.HighGroundCover = mapPlateau, 1, 0.3
		},
	},
	{
		name:  "plateaulink",
		about: "control: the same plateau world with no cover",
		apply: func(c *engine.Config) { c.TerrainMap, c.TerrainFoodCorrelation = mapPlateau, 1 },
	},
	{
		name:  "countrycover",
		about: "the whole country, with high ground as cover",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.HighGroundCover = mapCountry, 1, 0.3
		},
	},
	{
		name:  "plateauhardcover",
		about: "sweep: cover at 0.6",
		apply: func(c *engine.Config) {
			c.TerrainMap, c.TerrainFoodCorrelation, c.HighGroundCover = mapPlateau, 1, 0.6
		},
	},
	// The birth bug found on 2026-09-06: a bond that had run its course only
	// produced a child when the loop reached the lower-numbered partner
	// first. This arm is the world every measurement before that date was
	// taken in.
	{
		name:  "oldbirth",
		about: "half of all completed bonds produce nothing: the world before the birth fix",
		apply: func(c *engine.Config) { c.BirthNeedsLowerIDFirst = true },
	},
	// Feeding a child (2026-09-06). The rule does two things at once - it
	// costs the parent and it feeds the child - so the two controls take one
	// of them away each: "feedwaste" charges the parent and feeds nobody,
	// "feedblind" feeds the child but does not let the parent see that its
	// meals are half meals.
	{
		name:  "feedhalf",
		about: "half of every mouthful goes to the children the eater is rearing",
		apply: func(c *engine.Config) { c.ParentFeedShare = 0.5 },
	},
	{
		name:  "feedquarter",
		about: "sweep: a quarter of every mouthful goes to the children",
		apply: func(c *engine.Config) { c.ParentFeedShare = 0.25 },
	},
	{
		name:  "feedall",
		about: "sweep: the whole mouthful goes to the children while there are any to feed",
		apply: func(c *engine.Config) { c.ParentFeedShare = 1 },
	},
	{
		name:  "feedwaste",
		about: "control: the parent loses half of every mouthful and no child is fed",
		apply: func(c *engine.Config) { c.ParentFeedShare, c.ParentFeedWasted = 0.5, true },
	},
	{
		name:  "feedblind",
		about: "control: the children are fed, but the parent plans as though it ate the lot",
		apply: func(c *engine.Config) { c.ParentFeedShare, c.ParentFeedKnown = 0.5, false },
	},
	// Stage 7d's leash, asked the other way round: childcare that ends when
	// the child has grown up rather than when a count runs out. Growing is
	// bought with food, so the two are different lengths as well as different
	// rules - "longrearing" is the control that separates the two, a fixed
	// count set to about how long growing up actually takes.
	{
		name:  "rearuntilgrown",
		about: "a child keeps to its parent until it has finished growing, however long that takes",
		apply: func(c *engine.Config) { c.RearingUntilGrown = true },
	},
	{
		name:  "longrearing",
		about: "childcare still on a clock, but a clock as long as growing up takes (2500)",
		apply: func(c *engine.Config) { c.ChildRearingTicks = 2500 },
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
	"drowned", "drownShare", "drownRate", "onWater", "drownSeen", "dangerRank", "dangerKnown", "drawShare",
	"killSeen", "killLearned", "avengeSeen", "watchShare", "callShare", "joinShare",
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
	"meatDropped", "meatPerHunt", "meatShare", "meatSpoilShare", "meatEatenShare", "meatHeal",
	"meatSurplus", "meatFreeShare",
	"fishItems", "fishShare", "foodInWater",
	"bankHeld", "bankReal", "wadeHeld", "wadeReal", "anglerGap",
	"fishMissRate", "fishSpoilShare",
	"specialShare", "specialHeld", "specialReal", "specialGain", "harvestMissRate",
	"stonesLying", "stoneSeen", "stoneNear", "stoneHeld",
	"throws", "throwHitRate", "throwRate",
	"aimHeld", "aimReal",
	"gifts", "giftRate", "giftsToKin", "giftsToMates", "giftsToStrangers", "giftStones",
	"cryShare", "offerHeard", "offerDraw", "giftsCried",
	"storeHeld", "storeKnown", "storeKnowers", "storeIn", "storeOut",
	"storeFound", "storeSeen", "storeTold", "storeBorn",
	"coinsLying", "coinsHeld", "coinHolders", "sales", "salesRefused", "saleRate",
	"lostSight", "missingDir", "lonelyDraw",
	"motherRears", "mumNear", "dadNear", "ageFemale", "ageMale",
	"dread", "cheer", "afraid",
	"cooked", "cookRate", "cookedMeat", "cookedEaten", "cookedHanded",
	"cookStanding", "cookSplit", "cookHeld", "cookReal",
	"wadersWet", "bankersWet", "anglerSplit", "waders", "bankers",
	"starvedSeen", "starvedNear", "spareShare", "held", "holders", "load", "takeRate",
	"flees", "escapeShare",
	"restShelter", "shelterAll", "shelterGain",
	"humanRich", "enemyRich", "richGain", "enemyRichGain",
	"standGain", "suitGain", "suitCeiling", "regionsSeen", "oneRegion", "regionShare",
	"prowlArrive", "prowlGain", "prowlKept", "enemyBorn", "humanProwl", "enemyCrowd", "prowlBite",
	"regionKnown", "regionTold", "regionRank", "regionSpread", "regionCostRank",
	"dietVariety", "dietDiscount",
	"speedOpen", "speedDear", "speedGap", "onDear", "onHigh",
	"bankSplit", "crossShare", "crossIndex", "crossDry", "bankGeneGap", "bankCountryGap",
	"bankMoves", "bankBoth",
	"speedHigh", "speedLow", "highGap",
	"plantSpread", "plantRegrow", "plantClump", "plantEmpty", "seedsCarried",
	"plantPoison", "plantSignal", "plantHonesty", "plantSpitRate", "poisonDrain",
	"allAsleep", "clockSpread",
	"looksCorr", "looksCorrHuman", "looksCeiling",
	"retal", "trueRetal", "retalErr", "accept", "trueAccept", "acceptErr",
	"loreRate", "taught", "teachTop",
	"hintSlots", "hintsHeld", "hintKinds", "hintEntropy", "hintCopyRate",
	"skillHeld", "skillNominal", "skillReal", "skillSlots", "skillGap",
	"skillBornRate", "skillCopyRate", "skillLeaps",
	"forageHeld", "forageNominal", "forageReal",
	"swimHeld", "swimNominal", "swimReal",
	"tolHeld", "tolNominal", "tolReal",
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
	regionCostRank                                    float64

	// What the population makes of where the ground kills (stage 35): the
	// correlation with the truth, and how many regions the average agent has
	// any view of it for - which can be more than it has stood in, because a
	// drowning can be watched from the bank.
	dangerRank, dangerKnown float64

	// Where the population stands on the map, and who stands where (stage
	// 20). speedOpen and speedDear are the mean share of the budget spent on
	// speed by the agents standing on cheap and on dear ground; the gap
	// between them is the completion condition, because a fall in the
	// population-wide correlation is equally well explained by more variance.
	// onDear is the share of the population out on the dear ground at all -
	// a gap measured over nobody says nothing - and onHigh the share up on a
	// level above the bottom.
	speedOpen, speedDear, speedGap, onDear, onHigh float64
	speedHigh, speedLow, highGap                   float64

	// What the two sides of the world's midline are doing (stage 37): how
	// the population is split between them, how much of what can be seen is
	// seen across the line, that share against what pairing at random would
	// have given, and how far the two sides have drifted in what they are
	// made of and what they believe about the same ground.
	//
	// Taken in every arm, at the same line, whether or not there is a river
	// on it: the control has to be measured with the ruler its arm was.
	bankSplit, crossShare, crossIndex, crossDry float64
	bankGeneGap, bankCountryGap                 float64

	// onWater is the share of the population standing in the river (stage
	// 34). It is kept apart from onDear - which the water is also part of -
	// because what the drowning changes is where the water is, not where the
	// dear ground is.
	onWater float64

	// The awkward crop (stage 44): its share of what grows, who knows the
	// trick, what they make of it, and whether they have settled where it is.
	specialShare, specialHeld, specialReal, specialGain float64

	// standGain is how much better the ground under the humans is at making
	// them capable than ground taken at random (stage 57). Zero in a world
	// where every region is the same, and zero in a world where they differ
	// and nobody stays anywhere.
	standGain float64

	// suitGain is the same question per body (stage 57c): what the ground
	// where it stands does to its own build, less what the whole map would do
	// to that same build. It is the figure the scalar could not have.
	suitGain, suitCeiling float64

	// Where the dangerous country is (stage 58). prowlGain is whether the
	// enemies ended up where the map sends them, humanProwl whether the
	// humans ended up anywhere else, enemyCrowd how piled up they are, and
	// prowlBite whether the dying is more violent where they arrive.
	prowlGain, humanProwl, enemyCrowd, prowlBite float64
	prowlArrive, enemyBorn float64

	// Where the gifts went (stage 48).
	giftsToKin, giftsToMates, giftsToStrangers, giftStones float64

	// Knowing how to throw (stage 47): who has it and what their bodies make
	// of it.
	aimHeld, aimReal float64

	// Cooking (stage 52): who knows how, what their bodies make of it, how
	// far apart cooking and foraging are across the population (the division
	// of labour), and how much cooked food is standing about in hands.
	cookHeld, cookReal, cookSplit, cookStanding float64

	// The supply of stones (stage 45): how many lie about, how often a body
	// has one in sight, how far the nearest is, and how many are in hands.
	stonesLying, stoneSeen, stoneNear, stoneHeld float64

	// The two ways of fishing (stage 43): who holds each, what their bodies
	// can make of it, and the difference between the two - the figure that
	// says whether a population has split into bank and water.
	bankHeld, bankReal, wadeHeld, wadeReal, anglerGap float64

	// Where the two of them stand: the share of each kind found in water, the
	// difference (a population that has split into the two trades stands
	// apart), and how many there are of each.
	wadersWet, bankersWet, anglerSplit, waders, bankers float64

	// What the water holds (stage 42): fish about, and the share of the
	// world's food that is standing in it.
	fishItems, foodInWater float64

	// What the population is holding (stage 40): items per body, the share of
	// bodies holding anything, and how full the hands that exist are.
	held, holders, load float64

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

	// How much a body actually says about a blow: the ceiling on everything
	// stage 10 does. A line fitted to a weak signal cannot read more out of
	// it than is in it.
	looksCorr, looksCorrHuman, looksCeiling float64

	// What the population is making of its rules of thumb: room bought and
	// ideas carried per agent, how many distinct ones are alive at all, and
	// how evenly they are spread over those. The last two are the ones the
	// stage is on the hook for - a population can carry plenty of hints and
	// have them all be the same one.
	hintSlots, hintsHeld, hintKinds, hintEntropy float64

	// What the population knows how to do (stage 38a): the share carrying the
	// skill, the figure they hold, what their bodies actually get out of it,
	// how much of the room they bought is spent on it, and the gap between
	// the ones standing on dear ground and the rest.
	skillHeld, skillNominal, skillReal, skillSlots, skillGap float64
	forageHeld, forageNominal, forageReal                    float64
	swimHeld, swimNominal, swimReal                          float64
	tolHeld, tolNominal, tolReal                             float64
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
	// The caches, if this arm has any (stage 50). Laid out after the world is
	// built and before it runs, which is where an editor would put them, and
	// they draw nothing from the random source - so an arm with no caches runs
	// exactly as it did before this stage.
	if v.stores != nil {
		v.stores(w)
	}
	start := w.Stats()

	// The membership tracker watches only the final fifth, for the same reason
	// the abilities are averaged over it: the early ticks are the population
	// finding its size, and how long agents stay together during that is not
	// what the run is being asked about.
	member := engine.NewMembershipTracker(
		engine.DefaultClusterLinkDist, engine.DefaultMembershipStep, engine.DefaultMembershipLags)
	fights := engine.NewFightTracker(
		engine.DefaultClusterLinkDist, engine.DefaultMembershipStep, engine.DefaultCompanionLag)
	// Whether the same bodies stand on both banks (stage 37), watched over the
	// same tail and at the same cadence as the membership above.
	banks := engine.NewBankTracker(cfg.Width / 2)
	// The census runs over the whole run rather than the tail: its window
	// already trims it to the last stretch, and losing a species is a thing
	// that has to be caught when it happens, because a window that has moved
	// past a death cannot see it.
	census := engine.NewCensusTracker(engine.DefaultCensusWindow)
	// How much of the world one body covers in a lifetime (stage 57). It runs
	// over the whole run rather than the tail, because a lifetime is longer
	// than the tail and a body has to be watched for a while before its answer
	// means anything.
	visits := engine.NewRegionVisitTracker(engine.DefaultVisitSamples)
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
		skills := w.Skills(engine.SkillRough)
		forage := w.Skills(engine.SkillForage)
		swim := w.Skills(engine.SkillSwim)
		bank := w.Skills(engine.SkillFishLand)
		wade := w.Skills(engine.SkillFishWater)
		anglers := w.Anglers()
		crop := w.Specialty()
		rocks := w.Stones()
		aim := w.Skills(engine.SkillThrow)
		chef := w.Skills(engine.SkillCook)
		kitchen := w.Cooking()
		given := w.Gifts()
		tol := w.Skills(engine.SkillPoison)
		shelter := w.Shelter()
		rich := w.Richness()
		stand := w.Standing()
		suited := w.Suits()
		prowl := w.Prowl()
		known := w.RegionKnowledge()
		diet := w.Diet()
		carry := w.Carrying()
		fish := w.Fish()
		plants := w.Plants()
		vig := w.Vigilance(engine.DefaultClusterLinkDist)
		looks := w.LooksSignal()
		ground := whoStandsWhere(w)
		banks := w.Banks(w.Config().Width / 2)
		series = append(series, sample{
			taught: teach.Rate, teachTop: teach.TopShare,
			restShelter: shelter.Resting, shelterAll: shelter.All,
			humanRich: rich.Humans, enemyRich: rich.Enemies, allRich: rich.All,
			standGain: stand.Gain, suitGain: suited.Gain, suitCeiling: suited.Ceiling,
			prowlGain: prowl.EnemyGain, humanProwl: prowl.HumanGain,
			prowlArrive: prowl.ArriveGain, enemyBorn: prowl.BornShare,
			enemyCrowd: prowl.Crowding, prowlBite: prowl.Bite,
			regionKnown: known.Known, regionTold: known.Told,
			regionCostRank: known.CostRank,
			dangerRank:     known.DangerRank, dangerKnown: known.DangerKnown,
			regionRank: known.Rank, regionSpread: known.Spread,
			dietVariety: diet.Variety, dietDiscount: diet.Discount,
			held: carry.Held, holders: carry.Holders, load: carry.Load,
			fishItems: float64(fish.Items), foodInWater: fish.InWater,
			speedOpen: ground.open, speedDear: ground.dear, speedGap: ground.gap,
			onDear: ground.dearShare, onHigh: ground.highShare,
			onWater:   ground.waterShare,
			bankSplit: banks.Split, crossShare: banks.Cross, crossIndex: banks.CrossIndex,
			crossDry:    banks.CrossDry,
			bankGeneGap: banks.GeneGap, bankCountryGap: banks.CountryGap,
			speedHigh: ground.high, speedLow: ground.low, highGap: ground.highSpeedGap,
			plantSpread: plants.Spread, plantRegrow: plants.Regrow,
			plantClump: plants.Clumping, plantEmpty: plants.Empty,
			seedsCarried: plants.Carried,
			plantPoison:  plants.Poison, plantSignal: plants.Signal,
			plantHonesty: plants.Honesty,
			allAsleep:    vig.AllResting, clockSpread: vig.Spread,
			looksCorr: looks.All, looksCorrHuman: looks.Within,
			looksCeiling: looks.Ceiling,
			tolHeld:      tol.Held, tolNominal: tol.Nominal,
			tolReal:  tol.Realised,
			swimHeld: swim.Held, swimNominal: swim.Nominal,
			swimReal: swim.Realised,
			bankHeld: bank.Held, bankReal: bank.Realised,
			wadeHeld: wade.Held, wadeReal: wade.Realised,
			anglerGap:  wade.Realised - bank.Realised,
			giftsToKin: given.ToKin, giftsToMates: given.ToMates,
			giftsToStrangers: given.ToStrange, giftStones: given.Stones,
			aimHeld: aim.Held, aimReal: aim.Realised,
			cookHeld: chef.Held, cookReal: chef.Realised,
			cookSplit: kitchen.Split, cookStanding: kitchen.Standing,
			stonesLying: float64(rocks.Lying), stoneSeen: rocks.InSight,
			stoneNear: rocks.Nearest, stoneHeld: rocks.Carrying,
			specialShare: crop.Share, specialHeld: crop.Held,
			specialReal: crop.Realised, specialGain: crop.Gain,
			wadersWet: anglers.InWater, bankersWet: anglers.OnBank,
			anglerSplit: anglers.Split, waders: anglers.Waders, bankers: anglers.Bankers,
			forageHeld: forage.Held, forageNominal: forage.Nominal,
			forageReal: forage.Realised,
			skillHeld:  skills.Held, skillNominal: skills.Nominal,
			skillReal: skills.Realised, skillSlots: skills.Slots,
			skillGap:  skills.Dear - skills.Open,
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
			banks.Observe(w)
		}
		if w.Tick()%engine.DefaultCensusStep == 0 {
			census.Observe(w)
		}
		if w.Tick()%engine.DefaultVisitStep == 0 {
			visits.Observe(w)
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
	fords := banks.Result()
	roaming := visits.Result(w)
	stored := w.Stored()
	money := w.Coins()
	kitchen := w.Cooking()
	feeling := w.Mood()
	rearing := w.Rearing()
	apart := w.Loneliness()

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
		"pop":    float64(end.Population),
		"gen":    float64(end.MaxGeneration),
		"births": float64(end.Births),
		"deaths": float64(end.Deaths),
		// Starving is what the named causes do not claim, which is why every
		// new way to die has to have a bucket of its own: drowning folded in
		// here would have turned up as starvation in every arm with a river
		// in it (stage 34).
		"starved":    float64(end.Deaths - end.Kills - end.AgingDeaths - end.DrownDeaths),
		"drowned":    float64(end.DrownDeaths),
		"drownShare": share(end.DrownDeaths, end.Deaths),
		"killed":     float64(end.Kills),
		"killShare":  share(end.Kills, end.Deaths),
		"aged":       float64(end.AgingDeaths),
		"agedShare":  share(end.AgingDeaths, end.Deaths),
		"fights":     float64(end.Fights),
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
		"birthRate":  perAgentLifetime(end.Births-tailStart.Births, personTicks),
		"deathRate":  perAgentLifetime(end.Deaths-tailStart.Deaths, personTicks),
		"killRate":   perAgentLifetime(end.Kills-tailStart.Kills, personTicks),
		"starveRate": perAgentLifetime((end.Deaths-end.Kills-end.AgingDeaths-end.DrownDeaths)-(tailStart.Deaths-tailStart.Kills-tailStart.AgingDeaths-tailStart.DrownDeaths), personTicks),
		"drownRate":  perAgentLifetime(end.DrownDeaths-tailStart.DrownDeaths, personTicks),
		// How many pairs of eyes the average drowning had on it (stage 35).
		// A rule that hardly ever fires explains nothing whatever its weight.
		"drownSeen": ratio(end.DrownWitnesses, end.DrownDeaths),
		// What a killing leaves with the people who saw it (stage 31), and
		// what watching costs when it is free: the share of decisions that
		// were "watch somebody" is the check on the weight (#55).
		"killSeen": ratio(end.KillWitnesses, end.Kills),
		// Of the readings a killing offers, how many landed: a memory that is
		// full has no room for what it just watched.
		"killLearned": share(end.KillLessons, end.KillWitnesses),
		"avengeSeen":  ratio(end.AvengeWitnesses, end.Kills),
		"watchShare":  ratio(end.Observes, end.Decisions),
		// Calling others in, and going in on something somebody else has
		// taken on (stage 32). The second is the one that says whether a call
		// is answered: a word nobody acts on is not a hunt.
		"callShare": ratio(end.Calls, end.Decisions),
		// Crying what is in the hand (stage 49), in the same three shapes the
		// call is read in: how often the word is spoken, how often anybody is
		// deciding with somebody's wares in sight, and how often that decision
		// was a walk towards them. The last is the one the stage turns on - a
		// cry nobody walks to is a noise - and giftsCried is what it came to:
		// the share of hand-overs made by a body that had been advertising.
		"cryShare":   ratio(end.Cries, end.Decisions),
		"offerHeard": ratio(end.OffersHeard, end.Decisions),
		"offerDraw":  ratio(end.OfferDraws, end.Decisions),
		"giftsCried": share(end.GiftsCried, end.Gifts),
		// The caches (stage 50). storeIn and storeOut say whether the rule
		// fires at all; storeKnown and storeKnowers say how far the knowledge
		// got; and the three paths say how it travelled. Read them against
		// the arm where everybody knows every cache, which leaves the storing
		// and takes away the knowing (stage 49's lesson).
		"storeHeld":    float64(stored.Held),
		"storeKnown":   stored.Known,
		"storeKnowers": stored.Knowers,
		"storeIn":      float64(stored.Deposits),
		"storeOut":     float64(stored.Withdrawals),
		"storeFound":   float64(stored.Found),
		"storeSeen":    float64(stored.Seen),
		"storeTold":    float64(stored.Told),
		// What is left when the three paths that can be counted are taken
		// out is inheritance, which is the widest one and the reason a cache
		// can end up belonging to a line.
		"storeBorn": float64(stored.Learned - stored.Found - stored.Seen - stored.Told),
		// The money (stage 51). sales says whether a coin ever bought
		// anything; salesRefused says whether the market failed for want of
		// buyers or for want of sellers, which is the question the whole
		// stage turns on.
		"coinsLying":   float64(money.Lying),
		"coinsHeld":    float64(money.Held),
		"coinHolders":  money.Holders,
		"sales":        float64(money.Sales),
		"salesRefused": float64(money.Refused),
		"saleRate":     perAgentLifetime(money.Sales, personTicks),
		// The cooking (stage 52). cooked says whether the word is ever used;
		// cookedHanded is the monopoly question, as hand-overs of cooked food
		// per cooking - cooking that never leaves the cook is cooking no
		// exchange can be built on; cookSplit is the division of labour, as
		// the correlation between being good at this and being good at
		// foraging.
		"cooked":       float64(kitchen.Cooked),
		"cookRate":     perAgentLifetime(kitchen.Cooked, personTicks),
		"cookedMeat":   kitchen.Meat,
		"cookedEaten":  kitchen.Eaten,
		"cookedHanded": kitchen.Handed,
		"cookStanding": tail.cookStanding,
		"cookSplit":    tail.cookSplit,
		// How the population is feeling (stage 54). afraid is the one that
		// says whether the lean is a signal or a constant: near one is a mood
		// that never changes, and a constant changes no ranking.
		// Who rears and who feeds (stage 53). motherRears is one under 53a and
		// about a half before it; dadNear is the target 53b opens, as the
		// share of reared children with their other parent inside the leash.
		// Losing sight of the one a body thinks best of (stage 55a). lonelyDraw
		// is the ceiling on what the whole rule can do: a direction can only
		// reach a body through that one option.
		"lostSight":   float64(apart.Lost),
		"missingDir":  apart.Missing,
		"lonelyDraw":  apart.Draws,
		"motherRears": rearing.Guardians,
		"mumNear":     rearing.Near,
		"dadNear":     rearing.NearOther,
		"ageFemale":   rearing.AgeFemale,
		"ageMale":     rearing.AgeMale,
		"dread":       feeling.Dread,
		"cheer":       feeling.Cheer,
		"afraid":      feeling.Afraid,
		"cookHeld":    tail.cookHeld,
		"cookReal":    tail.cookReal,
		"joinShare":   ratio(end.Joins, end.Decisions),
		// The share of all decisions that were "go to country I think better
		// of" - the one door a belief about a place has into a body, and so
		// the ceiling on what stages 15b, 29 and 35 can do (stage 35).
		"drawShare":    share(end.RegionDraws, end.Decisions),
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
		"hintSlots":    tail.hintSlots,
		"hintsHeld":    tail.hintsHeld,
		"hintKinds":    tail.hintKinds,
		"hintEntropy":  tail.hintEntropy,
		"hintCopyRate": perAgentLifetime(end.HintsCopied-tailStart.HintsCopied, personTicks),
		// What the population knows how to do, and where it came from (stage
		// 38a). A skill that nobody holds explains nothing, and one that
		// spreads only down a line is not the diffusion the stage claims.
		"skillHeld": tail.skillHeld,
		// And the second skill (stage 38b), which is capped by a different
		// gene and seeded by what the ground provides rather than by what it
		// is made of.
		"tolHeld":        tail.tolHeld,
		"tolNominal":     tail.tolNominal,
		"tolReal":        tail.tolReal,
		"swimHeld":       tail.swimHeld,
		"swimNominal":    tail.swimNominal,
		"swimReal":       tail.swimReal,
		"forageHeld":     tail.forageHeld,
		"forageNominal":  tail.forageNominal,
		"forageReal":     tail.forageReal,
		"skillNominal":   tail.skillNominal,
		"skillReal":      tail.skillReal,
		"skillSlots":     tail.skillSlots,
		"skillGap":       tail.skillGap,
		"skillBornRate":  perAgentLifetime(end.SkillsBorn-tailStart.SkillsBorn, personTicks),
		"skillCopyRate":  perAgentLifetime(end.SkillsCopied-tailStart.SkillsCopied, personTicks),
		"skillLeaps":     float64(end.SkillsLeapt),
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
		"hunts":      float64(end.Hunts),
		"jointHunts": float64(end.JointHunts),
		// What becomes of the meat (stage 39). Before making a carcass worth
		// more it has to be said how much of it there is and how much of it
		// nobody takes: meatShare is the share of all the mouthfuls in the
		// world that were meat, and meatSpoilShare the share of the items
		// that rotted where they fell. A rule about meat can do nothing about
		// the meals that were never going to be meat.
		"meatDropped":    float64(end.MeatDropped),
		"meatPerHunt":    ratio(end.MeatDropped, end.Hunts),
		"meatShare":      share(end.MeatEaten, end.MeatEaten+end.PlantsEaten),
		"meatSpoilShare": share(end.MeatSpoiled, end.MeatDropped),
		"meatEatenShare": share(end.MeatEaten, end.MeatDropped),
		// And what the carcasses mended, per item of meat eaten. Counted in
		// every arm, so the ones where the rule is off say how much of a
		// difference it could have made.
		"meatHeal": ratioF(end.MeatHealing, end.MeatEaten),
		// What a kill leaves beyond what those who made it could carry away,
		// and how much of the meat eaten was eaten by somebody who had no
		// claim on it (stage 41). The first is the premise: a rule about a
		// surplus needs there to be one.
		"meatSurplus": share(end.MeatItems-end.MeatKeepable, end.MeatItems),
		// What the water holds (stage 42): how many fish are about, what
		// share of the mouthfuls were fish, and how much of the world's food
		// is standing in water - the figure that says whether the reward and
		// the danger are in the same cells.
		"bankHeld":    tail.bankHeld,
		"bankReal":    tail.bankReal,
		"wadeHeld":    tail.wadeHeld,
		"wadeReal":    tail.wadeReal,
		"anglerGap":   tail.anglerGap,
		"wadersWet":   tail.wadersWet,
		"bankersWet":  tail.bankersWet,
		"anglerSplit": tail.anglerSplit,
		"waders":      tail.waders,
		"bankers":     tail.bankers,
		// How often a fish gets away, against how often one is landed: the
		// figure the two fishing skills are about (stage 43).
		"fishMissRate": share(end.FishMissed, end.FishMissed+end.FishEaten),
		// And the share of the fish that were landed and then went off before
		// anybody ate them - in a hand, or lying where the body carrying them
		// died. A fish still in the river has no clock on it.
		"fishSpoilShare": share(end.FishSpoiled, end.FishSpoiled+end.FishEaten),
		// The awkward crop (stage 44): how much of what grows needs knowing,
		// who knows it, what their bodies make of it, and - the figure the
		// stage turns on - how much more of it grows where they are standing
		// than the world's average.
		// Throwing (stage 46): how many stones were thrown, how many landed,
		// and how often it happens per lifetime. Read beside trueRetal, which
		// is the figure this rule is dangerous to.
		// The gate the economy waits on (stage 48): did anything ever change
		// hands, and to whom.
		"gifts":            float64(end.Gifts),
		"giftRate":         perAgentLifetime(end.Gifts-tailStart.Gifts, personTicks),
		"giftsToKin":       tail.giftsToKin,
		"giftsToMates":     tail.giftsToMates,
		"giftsToStrangers": tail.giftsToStrangers,
		"giftStones":       tail.giftStones,
		"aimHeld":          tail.aimHeld,
		"aimReal":          tail.aimReal,
		"throws":           float64(end.Throws),
		"throwHitRate":     share(end.ThrowHits, end.Throws),
		"throwRate":        perAgentLifetime(end.Throws-tailStart.Throws, personTicks),
		// The supply of things to throw (stage 45).
		"stonesLying":     tail.stonesLying,
		"stoneSeen":       tail.stoneSeen,
		"stoneNear":       tail.stoneNear,
		"stoneHeld":       tail.stoneHeld,
		"specialShare":    tail.specialShare,
		"specialHeld":     tail.specialHeld,
		"specialReal":     tail.specialReal,
		"specialGain":     tail.specialGain,
		"harvestMissRate": share(end.HarvestMissed, end.HarvestMissed+end.PlantsEaten),
		"fishItems":       tail.fishItems,
		"fishShare":       share(end.FishEaten, end.FishEaten+end.PlantsEaten+end.MeatEaten),
		"foodInWater":     tail.foodInWater,
		"meatFreeShare":   share(end.MeatEatenFree, end.MeatEaten),
		// Counting the target for carrying (stage 40, #67). starvedSeen is
		// the share of the bodies that starved which had had food in sight
		// within a planning horizon of dying - the deaths an item in hand
		// could have answered. spareShare is how much of the time spent with
		// food in sight was spent not hungry: the room there is to pick
		// something up for later.
		"starvedSeen": share(end.StarvedFoodSeen, end.StarvedDeaths),
		"starvedNear": share(end.StarvedFoodNear, end.StarvedDeaths),
		// And what carrying itself does: how much is in hand, how many hands
		// have anything in them, how full they are, and how often something
		// was picked up.
		"held":        tail.held,
		"holders":     tail.holders,
		"load":        tail.load,
		"takeRate":    perAgentLifetime(end.Taken-tailStart.Taken, personTicks),
		"spareShare":  share(end.SpareTicks, end.SightTicks),
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
		// Where the population stands, by what the ground does to what a body
		// can do (stage 57), and how much of the world one body covers in a
		// lifetime. oneRegion is the share of bodies that never left the block
		// they were born in - the figure a rule about staying put has to move.
		"standGain":   tail.standGain,
		"suitGain":    tail.suitGain,
		"suitCeiling": tail.suitCeiling,
		// Where the enemies are (stage 58). prowlGain near zero with a spread
		// switched on means the arrivals are not sticking; humanProwl is the
		// one nothing tells them, so it moving at all is them feeling it.
		"prowlArrive": tail.prowlArrive,
		"prowlGain":   tail.prowlGain,
		// How much of what the rule did is still there. Read the two above as
		// a pair: a rule that fires perfectly can leave nothing behind.
		"prowlKept":  kept(tail.prowlGain, tail.prowlArrive),
		"enemyBorn":  tail.enemyBorn,
		"humanProwl": tail.humanProwl,
		"enemyCrowd": tail.enemyCrowd,
		"prowlBite":  tail.prowlBite,
		"regionsSeen": roaming.Mean,
		"oneRegion":   roaming.Alone,
		"regionShare": roaming.Share,
		"enemyRichGain": tail.enemyRich - tail.allRich,
		// What the population has made of the ground. regionRank is the one
		// that says whether any of it is true: the correlation between what
		// agents believe about a region and how well it actually grows.
		"regionKnown":    tail.regionKnown,
		"regionCostRank": tail.regionCostRank,
		"dangerRank":     tail.dangerRank,
		"dangerKnown":    tail.dangerKnown,
		"regionTold":     tail.regionTold,
		"regionRank":     tail.regionRank,
		"regionSpread":   tail.regionSpread,
		// What the population lives on. dietDiscount at one is a rule that
		// never fires; dietVariety at zero is a world with nothing to vary.
		"speedOpen": tail.speedOpen,
		"speedDear": tail.speedDear,
		"speedGap":  tail.speedGap,
		"onDear":    tail.onDear,
		"onHigh":    tail.onHigh,
		"onWater":   tail.onWater,
		// The two banks (stage 37). crossIndex is not to be read on its own -
		// agents cluster locally whatever the ground is, so it is low
		// everywhere; what it is for is the arm against its control.
		"bankSplit":      tail.bankSplit,
		"crossShare":     tail.crossShare,
		"crossIndex":     tail.crossIndex,
		"crossDry":       tail.crossDry,
		"bankGeneGap":    tail.bankGeneGap,
		"bankCountryGap": tail.bankCountryGap,
		"bankMoves":      fords.Rate,
		"bankBoth":       fords.Ever,
		"speedHigh":      tail.speedHigh,
		"speedLow":       tail.speedLow,
		"highGap":        tail.highGap,

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
		"plantPoison":   tail.plantPoison,
		"plantSignal":   tail.plantSignal,
		"plantSpitRate": perAgentLifetime(end.PlantsSpat-tailStart.PlantsSpat, personTicks),
		// What the crop takes off a body per tick of being alive, so that it
		// can be held against what a body recovers in the same tick
		// (RegenRate, 0.09). It is the ceiling on what any defence against it
		// could be worth.
		"poisonDrain":  drain(end.PoisonLoss-tailStart.PoisonLoss, personTicks),
		"plantHonesty": tail.plantHonesty,
		// Stage 18. allAsleep is the share of groups caught with everybody
		// asleep at once; clockSpread is how varied the population's hours
		// are, without which the first says nothing.
		"allAsleep":   tail.allAsleep,
		"clockSpread": tail.clockSpread,
		// The ceiling on stage 10: how much a build says about a blow, over
		// everybody and within one kind, and the error a perfect reader of
		// the build would still be left with.
		"looksCorr":      tail.looksCorr,
		"looksCorrHuman": tail.looksCorrHuman,
		"looksCeiling":   tail.looksCeiling,
		"packSize":       share(end.HuntParty, end.Hunts) * 1, // mean per hunt
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
		out.regionCostRank += s.regionCostRank
		out.dangerRank += s.dangerRank
		out.dangerKnown += s.dangerKnown
		out.regionTold += s.regionTold
		out.regionRank += s.regionRank
		out.regionSpread += s.regionSpread
		out.speedOpen += s.speedOpen
		out.speedDear += s.speedDear
		out.speedGap += s.speedGap
		out.onDear += s.onDear
		out.onWater += s.onWater
		out.bankSplit += s.bankSplit
		out.crossShare += s.crossShare
		out.crossIndex += s.crossIndex
		out.crossDry += s.crossDry
		out.bankGeneGap += s.bankGeneGap
		out.bankCountryGap += s.bankCountryGap
		out.onHigh += s.onHigh
		out.speedHigh += s.speedHigh
		out.speedLow += s.speedLow
		out.highGap += s.highGap
		out.giftsToKin += s.giftsToKin
		out.giftsToMates += s.giftsToMates
		out.giftsToStrangers += s.giftsToStrangers
		out.giftStones += s.giftStones
		out.aimHeld += s.aimHeld
		out.aimReal += s.aimReal
		out.cookHeld += s.cookHeld
		out.cookReal += s.cookReal
		out.cookSplit += s.cookSplit
		out.cookStanding += s.cookStanding
		out.stonesLying += s.stonesLying
		out.stoneSeen += s.stoneSeen
		out.stoneNear += s.stoneNear
		out.stoneHeld += s.stoneHeld
		out.specialShare += s.specialShare
		out.specialHeld += s.specialHeld
		out.specialReal += s.specialReal
		out.specialGain += s.specialGain
		out.standGain += s.standGain
		out.suitGain += s.suitGain
		out.suitCeiling += s.suitCeiling
		out.prowlGain += s.prowlGain
		out.prowlArrive += s.prowlArrive
		out.enemyBorn += s.enemyBorn
		out.humanProwl += s.humanProwl
		out.enemyCrowd += s.enemyCrowd
		out.prowlBite += s.prowlBite
		out.wadersWet += s.wadersWet
		out.bankersWet += s.bankersWet
		out.anglerSplit += s.anglerSplit
		out.waders += s.waders
		out.bankers += s.bankers
		out.bankHeld += s.bankHeld
		out.bankReal += s.bankReal
		out.wadeHeld += s.wadeHeld
		out.wadeReal += s.wadeReal
		out.anglerGap += s.anglerGap
		out.fishItems += s.fishItems
		out.foodInWater += s.foodInWater
		out.held += s.held
		out.holders += s.holders
		out.load += s.load
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
		out.looksCorr += s.looksCorr
		out.looksCorrHuman += s.looksCorrHuman
		out.looksCeiling += s.looksCeiling
		out.tolHeld += s.tolHeld
		out.tolNominal += s.tolNominal
		out.tolReal += s.tolReal
		out.swimHeld += s.swimHeld
		out.swimNominal += s.swimNominal
		out.swimReal += s.swimReal
		out.forageHeld += s.forageHeld
		out.forageNominal += s.forageNominal
		out.forageReal += s.forageReal
		out.skillHeld += s.skillHeld
		out.skillNominal += s.skillNominal
		out.skillReal += s.skillReal
		out.skillSlots += s.skillSlots
		out.skillGap += s.skillGap
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
	out.regionCostRank /= d
	out.dangerRank /= d
	out.dangerKnown /= d
	out.regionTold /= d
	out.regionRank /= d
	out.regionSpread /= d
	out.speedOpen /= d
	out.speedDear /= d
	out.speedGap /= d
	out.onDear /= d
	out.onWater /= d
	out.bankSplit /= d
	out.crossShare /= d
	out.crossIndex /= d
	out.crossDry /= d
	out.bankGeneGap /= d
	out.bankCountryGap /= d
	out.onHigh /= d
	out.speedHigh /= d
	out.speedLow /= d
	out.highGap /= d
	out.giftsToKin /= d
	out.giftsToMates /= d
	out.giftsToStrangers /= d
	out.giftStones /= d
	out.aimHeld /= d
	out.cookHeld /= d
	out.cookReal /= d
	out.cookSplit /= d
	out.cookStanding /= d
	out.aimReal /= d
	out.stonesLying /= d
	out.stoneSeen /= d
	out.stoneNear /= d
	out.stoneHeld /= d
	out.specialShare /= d
	out.specialHeld /= d
	out.specialReal /= d
	out.specialGain /= d
	out.standGain /= d
	out.suitGain /= d
	out.suitCeiling /= d
	out.prowlGain /= d
	out.prowlArrive /= d
	out.enemyBorn /= d
	out.humanProwl /= d
	out.enemyCrowd /= d
	out.prowlBite /= d
	out.wadersWet /= d
	out.bankersWet /= d
	out.anglerSplit /= d
	out.waders /= d
	out.bankers /= d
	out.bankHeld /= d
	out.bankReal /= d
	out.wadeHeld /= d
	out.wadeReal /= d
	out.anglerGap /= d
	out.fishItems /= d
	out.foodInWater /= d
	out.held /= d
	out.holders /= d
	out.load /= d
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
	out.looksCorr /= d
	out.looksCorrHuman /= d
	out.looksCeiling /= d
	out.tolHeld /= d
	out.tolNominal /= d
	out.tolReal /= d
	out.swimHeld /= d
	out.swimNominal /= d
	out.swimReal /= d
	out.forageHeld /= d
	out.forageNominal /= d
	out.forageReal /= d
	out.skillHeld /= d
	out.skillNominal /= d
	out.skillReal /= d
	out.skillSlots /= d
	out.skillGap /= d
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

// groundSplit is who is standing on what (stage 20): the mean share of the
// budget spent on speed by the agents on cheap ground and by those on dear
// ground, the difference, and how much of the population is out on the dear
// ground and up on a level at all.
type groundSplit struct {
	open, dear, gap      float64
	dearShare, highShare float64

	// waterShare is the share standing in the river, counted apart from the
	// dear ground it is also part of (stage 34).
	waterShare float64

	// The same split by height rather than by cost. High ground is the one
	// piece of country the population demonstrably does sort itself over -
	// the ramps are a choice in a way that dear ground is not - so whether
	// the ones up there are a different sort of body is the question.
	high, low, highSpeedGap float64
}

// whoStandsWhere is the measurement the terrain stage turns on.
//
// A fall in the population-wide correlation between speed and vitality is not
// enough to call a niche: more variance looks the same. What says "the fast
// live in the open and the tough live in the rough" is the two means measured
// apart, with the share standing on each next to them - a gap measured over
// two agents is noise wearing a number.
func whoStandsWhere(w *engine.World) groundSplit {
	var out groundSplit
	agents := w.Agents()
	var nOpen, nDear, nHigh, nWater float64
	for i := range agents {
		a := &agents[i]
		b := a.Budget()
		if b <= 0 {
			continue
		}
		share := a.Gene(engine.GeneSpeed) / b
		g := w.TerrainAt(a.X, a.Y)
		if g.Kind == engine.GroundWater {
			nWater++
		}
		if g.Height > 0 {
			nHigh++
			out.high += share
		} else {
			out.low += share
		}
		if g.Cost > 1 {
			out.dear += share
			nDear++
			continue
		}
		out.open += share
		nOpen++
	}
	if n := nOpen + nDear; n > 0 {
		out.dearShare, out.highShare = nDear/n, nHigh/n
		out.waterShare = nWater / n
	}
	if nOpen > 0 {
		out.open /= nOpen
	}
	if nDear > 0 {
		out.dear /= nDear
	}
	if nOpen > 0 && nDear > 0 {
		out.gap = out.open - out.dear
	}
	if n := nOpen + nDear; n > nHigh && nHigh > 0 {
		out.high /= nHigh
		out.low /= n - nHigh
		out.highSpeedGap = out.high - out.low
	} else {
		out.high, out.low = 0, 0
	}
	return out
}

// drain is a quantity of vitality spread over the person-ticks it was taken
// across: what it costs a body per tick of being alive.
// cfgOf is the config an arm actually ran with, for the metrics that have to
// divide by one of its figures.
func cfgOf(v variant) engine.Config {
	cfg := engine.DefaultConfig()
	v.apply(&cfg)
	return cfg
}

func drain(total, personTicks float64) float64 {
	if personTicks <= 0 {
		return 0
	}
	return total / personTicks
}

// share is what fraction of the deaths were killings. It is the headline
// figure for the cooperation work: the point of that work is to get it down
// without simply feeding everybody, which is why it sits next to starved.
// kept is how much of what a rule did is still there, for two figures that
// are already real numbers.
func kept(part, whole float64) float64 {
	if whole == 0 {
		return 0
	}
	return part / whole
}

func share(part, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return float64(part) / float64(whole)
}

// ratio is one count against another when the second is not a total the first
// is part of - how many onlookers per drowning, say.
// ratioF is ratio for a total that is not a count of events.
func ratioF(part float64, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return part / float64(whole)
}

func ratio(part, whole int) float64 {
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
