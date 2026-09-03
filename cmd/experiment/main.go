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
	"hunts", "jointHunts", "packSize", "evadedShare",
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
		series = append(series, sample{
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
		"hunts":       float64(end.Hunts),
		"jointHunts":  float64(end.JointHunts),
		"evadedShare": share(end.Evaded, end.Fights),
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
	return r
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
