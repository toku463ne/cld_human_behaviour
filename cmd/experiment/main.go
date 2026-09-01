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
	"clumping", "neighbours", "nearest",
	"clusters", "clusterSize", "grouped", "largestShare",
	"gap", "gapP10", "gapRel",
	"halfLife", "together", "censored",
	"fightCompanion", "fightStranger", "fightRatio",
	"species", "rareShare", "rareTrough", "rareSwing",
	"power", "rationality", "intelligence",
	"sdPower", "sdRationality", "sdIntelligence",
	"dPower", "dRationality", "dIntelligence",
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
		series = append(series, sample{
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
	record()
	for i := 0; i < ticks; i++ {
		w.Step()
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
		"pop":          float64(end.Population),
		"gen":          float64(end.MaxGeneration),
		"births":       float64(end.Births),
		"deaths":       float64(end.Deaths),
		"starved":      float64(end.Deaths - end.Kills - end.AgingDeaths),
		"killed":       float64(end.Kills),
		"killShare":    share(end.Kills, end.Deaths),
		"aged":         float64(end.AgingDeaths),
		"agedShare":    share(end.AgingDeaths, end.Deaths),
		"fights":       float64(end.Fights),
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
		"power":          tail.power,
		"rationality":    tail.rat,
		"intelligence":   tail.intel,
		"sdPower":        tail.sdPower,
		"sdRationality":  tail.sdRat,
		"sdIntelligence": tail.sdIntel,
		"dPower":         tail.power - start.AvgPower,
		"dRationality":   tail.rat - start.AvgRationality,
		"dIntelligence":  tail.intel - start.AvgIntelligence,
		"extinct":        boolToFloat(end.Population == 0),
	}}
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
func abilitySpread(w *engine.World) (power, rationality, intelligence float64) {
	agents := w.Agents()
	n := float64(len(agents))
	if n < 2 {
		return 0, 0, 0
	}
	var mp, mr, mi float64
	for i := range agents {
		mp += agents[i].Power
		mr += agents[i].Rationality
		mi += agents[i].Intelligence
	}
	mp, mr, mi = mp/n, mr/n, mi/n
	var sp, sr, si float64
	for i := range agents {
		sp += (agents[i].Power - mp) * (agents[i].Power - mp)
		sr += (agents[i].Rationality - mr) * (agents[i].Rationality - mr)
		si += (agents[i].Intelligence - mi) * (agents[i].Intelligence - mi)
	}
	return math.Sqrt(sp / n), math.Sqrt(sr / n), math.Sqrt(si / n)
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
