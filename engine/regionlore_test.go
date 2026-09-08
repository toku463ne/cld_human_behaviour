package engine

import (
	"math"
	"testing"
)

// The arm this stage is measured against: with no learning the world is the
// one stage 15a left, down to the values taken from the random source.
func TestNoRegionLearningLeavesTheWorldExactlyAsItWas(t *testing.T) {
	run := func(rate float64) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 6
		cfg.RegionLearnRate = rate
		w := NewWorld(cfg)
		for i := 0; i < 1500; i++ {
			w.Step()
		}
		return w.Stats()
	}
	if off, again := run(0), run(0); off != again {
		t.Fatal("the same world twice gave different runs")
	} else if on := run(1); on == off {
		t.Fatal("letting agents learn the ground changed nothing at all")
	}
}

// An agent forms a view of the ground it is standing on, and of no other. That
// is what makes being told about somewhere worth anything.
func TestAnAgentOnlyLearnsTheGroundItStandsOn(t *testing.T) {
	cfg := quietConfig()
	cfg.RegionNoise = 0
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 50, Y: 50, Vitality: 80, Genome: genomeOf(50, 50, 50)}))

	here := w.regionIndexAt(a.X, a.Y)
	for i := 0; i < 20; i++ {
		w.noteRegion(a, 5)
	}
	seen, known := w.regionEstimate(a, here)
	if !known {
		t.Fatal("stood somewhere twenty times and knows nothing about it")
	}
	if math.Abs(seen-5) > 0.5 {
		t.Fatalf("reckons it saw %v food there, want about 5", seen)
	}
	// Everywhere else is still unknown - not "believed empty".
	for r := range w.regions {
		if r == here {
			continue
		}
		if _, known := w.regionEstimate(a, r); known {
			t.Fatalf("has a view of region %d without ever going there", r)
		}
	}
	if _, _, ok := w.bestKnownRegion(a); ok {
		t.Fatal("knows somewhere better than here, having been nowhere else")
	}
}

// Somewhere you have never been is somewhere you can only hear about, and
// hearing it is a starting point rather than a lifetime of looking.
func TestBeingToldAboutSomewhereYouHaveNeverBeen(t *testing.T) {
	cfg := quietConfig()
	cfg.RegionNoise = 0
	w := NewWorld(cfg)

	traveller := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 50, Y: 50, Vitality: 80, Genome: genomeOf(50, 50, 50)}))
	stayer := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 55, Y: 50, Vitality: 80, Genome: genomeOf(50, 50, 50)}))

	// The traveller has been somewhere good, far from here.
	far := len(w.regions) - 1
	traveller.regions = make([]regionView, len(w.regions))
	traveller.regions[far].setSeen(9, 30, w.tick)

	if _, known := w.regionEstimate(stayer, far); known {
		t.Fatal("knew about it before being told")
	}
	w.exchangeRegions(stayer, traveller)

	seen, known := w.regionEstimate(stayer, far)
	if !known {
		t.Fatal("was told about somewhere and took nothing from it")
	}
	if math.Abs(seen-9) > 1e-9 {
		t.Fatalf("was told 9 and came away with %v", seen)
	}
	// But holds it lightly: one look of its own should nearly overturn it.
	if n := stayer.regions[far].n; n > cfg.RegionToldCount {
		t.Fatalf("a handed-down view is worth %v looks, want no more than %v", n, cfg.RegionToldCount)
	}
	// Where both have been, they meet in the middle instead.
	both := 0
	traveller.regions[both].setSeen(8, 30, w.tick)
	stayer.regions[both].setSeen(2, 30, w.tick)
	w.exchangeRegions(stayer, traveller)
	mine, _ := w.regionEstimate(stayer, both)
	theirs, _ := w.regionEstimate(traveller, both)
	if !(mine > 2 && theirs < 8 && mine < theirs) {
		t.Fatalf("after trading they believe %v and %v, want them to have moved towards each other", mine, theirs)
	}
}

// Knowing where the good ground is only matters if it can be acted on. The
// draw is one more option in the same comparison - a direction for one step,
// not a plan - and it is worth more the better the remembered ground is.
func TestAnAgentHeadsForGroundItRemembersAsBetter(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)

	exploreOptions := func(gain, drawValue float64) []option {
		cfg := cfg
		cfg.RegionDrawValue = drawValue
		p := &Perception{
			Tick: 1, Cfg: &cfg, Rand: w.rng,
			Self: SelfView{
				ID: 1, X: 100, Y: 100, Vitality: 60, Hunger: cfg.MaxHunger * 0.8,
				MaxVitality: cfg.MaxVitality, MaxSpeed: cfg.MaxSpeed,
				HungerRate:    cfg.HungerRate,
				BetterGround:  gain,
				BetterGroundX: 300, BetterGroundY: 100,
				Retaliation: cfg.Retaliation, AcceptChance: cfg.AcceptChance,
				RiskWeight: cfg.RiskWeight, CompetitionWeight: cfg.CompetitionWeight,
				ShockRisk: cfg.ShockRisk},
		}
		c := &AIController{}
		c.addExplore(p)
		return c.opts
	}

	if got := exploreOptions(0, cfg.RegionDrawValue); len(got) != 1 {
		t.Fatalf("with nowhere better known there were %d wandering options, want just the aimless one", len(got))
	}
	if got := exploreOptions(3, 0); len(got) != 1 {
		t.Fatalf("with the draw switched off there were %d options, want just the aimless one", len(got))
	}

	opts := exploreOptions(3, cfg.RegionDrawValue)
	if len(opts) != 2 {
		t.Fatalf("knowing somewhere better gave %d options, want the aimless one and the aimed one", len(opts))
	}
	aimed := opts[1]
	if aimed.action.DX <= 0.99 {
		t.Fatalf("headed %v,%v, want due east towards the remembered ground", aimed.action.DX, aimed.action.DY)
	}
	if aimed.util <= opts[0].util {
		t.Fatalf("heading for better ground scored %v against wandering at %v", aimed.util, opts[0].util)
	}
	// And the better the ground is remembered to be, the more it is worth.
	if small, large := exploreOptions(1, cfg.RegionDrawValue)[1], exploreOptions(6, cfg.RegionDrawValue)[1]; large.util <= small.util {
		t.Fatalf("a slightly better place scored %v and a much better one %v", small.util, large.util)
	}
}

// What the memory gene buys here is how well the country is held - not how
// many people are, which is a separate organ's worth of room (#41).
func TestAGoodMemoryHoldsTheCountryLongerWithoutCrowdingOutPeople(t *testing.T) {
	cfg := quietConfig()
	cfg.RegionNoise = 0
	w := NewWorld(cfg)

	learn := func(memory float64) *Agent {
		g := genomeOf(50, 50, 50)
		g[GeneMemory] = memory
		a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 50, Y: 50, Vitality: 80, Genome: g}))
		for i := 0; i < 60; i++ {
			w.noteRegion(a, 5)
		}
		return a
	}
	poor, good := learn(10), learn(95)
	here := w.regionIndexAt(50, 50)

	// The one that spent on memory holds more looks behind the same estimate.
	if poor.regions[here].n >= good.regions[here].n {
		t.Fatalf("a poor memory holds %v looks and a good one %v", poor.regions[here].n, good.regions[here].n)
	}

	// And keeps it longer once it has walked away. Five thousand ticks is ten
	// years: long past what a poor memory holds country for, well inside what
	// a good one does.
	w.tick += 5000
	_, poorKnows := w.regionEstimate(poor, here)
	_, goodKnows := w.regionEstimate(good, here)
	if poorKnows && !goodKnows {
		t.Fatal("the poor memory outlasted the good one")
	}
	if !goodKnows {
		t.Fatal("even a good memory lost the country entirely")
	}

	// Knowing the country costs nothing in room for people.
	if len(good.opinions) != 0 {
		t.Fatalf("learning the ground took up %d of its memory of people", len(good.opinions))
	}
}

// --- how hard the going is (stage 29) ---------------------------------------

// costCountryConfig is a still world split east and west: open ground on the
// left, country that costs three times as much on the right. The region grid
// is 4x3, so the western two columns of regions are entirely open and the
// eastern two entirely rough.
func costCountryConfig() Config {
	cfg := quietConfig()
	cfg.RegionNoise = 0
	cfg.Width, cfg.Height = 800, 600
	cfg.RoughMoveCost = 3
	cfg.TerrainMap = []string{
		"....::::",
		"....::::",
		"....::::",
	}
	return cfg
}

// An agent learns the going of the ground it stands on, and nothing about the
// ground it has not.
func TestAnAgentLearnsHowHardTheGroundItStandsOnIs(t *testing.T) {
	cfg := costCountryConfig()
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 500, Y: 100, Vitality: 80,
		Genome: genomeOf(50, 50, 50)}))

	here := w.regionIndexAt(a.X, a.Y)
	for i := 0; i < 30; i++ {
		w.noteRegion(a, 3)
	}
	cost, known := w.regionCostEstimate(a, here)
	if !known {
		t.Fatal("stood on hard going thirty times and learned nothing about it")
	}
	if math.Abs(cost-cfg.RoughMoveCost) > 0.3 {
		t.Fatalf("reckons the going here is x%.2f, want about x%.1f", cost, cfg.RoughMoveCost)
	}
	for r := range w.regions {
		if r == here {
			continue
		}
		if _, known := w.regionCostEstimate(a, r); known {
			t.Fatalf("has a view of the going in region %d without ever being there", r)
		}
	}
}

// Hard going makes a region worth less to go to. Two regions with the same
// food, one of them rough: the open one is what the agent heads for.
func TestHardGoingMakesAPlaceWorthLess(t *testing.T) {
	cfg := costCountryConfig()
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 300, Vitality: 80,
		Genome: genomeOf(50, 50, 50)}))

	// Been in three places: here (middling), somewhere open with more food,
	// and somewhere rough with the same amount more.
	openIdx := w.regionIndexAt(100, 100)
	roughIdx := w.regionIndexAt(700, 100)
	hereIdx := w.regionIndexAt(a.X, a.Y)
	a.regions = make([]regionView, len(w.regions))
	for _, c := range []struct {
		i    int
		seen float64
		cost float64
	}{
		{hereIdx, 2, 1},
		{openIdx, 6, 1},
		{roughIdx, 6, cfg.RoughMoveCost},
	} {
		a.regions[c.i].setSeen(c.seen, 10, w.tick)
		a.regions[c.i].cost = c.cost
	}

	best, gain, ok := w.bestKnownRegion(a)
	if !ok {
		t.Fatal("knows nowhere better than here")
	}
	if best != openIdx {
		t.Fatalf("heads for region %d, want the open one (%d) over the rough one (%d)",
			best, openIdx, roughIdx)
	}
	// And the pull is smaller than it would be with the going ignored.
	cfg.RegionCostWeight = 0
	flat := NewWorld(cfg)
	flat.agents = w.agents
	flat.index = w.index
	if _, flatGain, _ := flat.bestKnownRegion(mustAgent(t, flat, a.ID)); !(gain <= flatGain) {
		t.Fatalf("the going cost nothing: gain %v with it, %v without", gain, flatGain)
	}
}

// With the weight at zero the stage is not there: the ranking is stage 15b's.
func TestWithNoWeightTheGoingIsIgnored(t *testing.T) {
	cfg := costCountryConfig()
	cfg.RegionCostWeight = 0
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 300, Vitality: 80,
		Genome: genomeOf(50, 50, 50)}))

	openIdx, roughIdx := w.regionIndexAt(100, 100), w.regionIndexAt(700, 100)
	a.regions = make([]regionView, len(w.regions))
	a.regions[w.regionIndexAt(a.X, a.Y)].setSeen(2, 10, w.tick)
	a.regions[openIdx].setSeen(6, 10, w.tick)
	a.regions[openIdx].cost = 1
	a.regions[roughIdx].setSeen(7, 10, w.tick)
	a.regions[roughIdx].cost = cfg.RoughMoveCost

	if best, _, _ := w.bestKnownRegion(a); best != roughIdx {
		t.Fatalf("heads for region %d, want the one with the most food (%d) when the going is not weighed",
			best, roughIdx)
	}
}

// The going is handed on like everything else two agents trade: somewhere you
// have never been is somewhere you can only hear about.
func TestTheGoingIsHandedOn(t *testing.T) {
	cfg := costCountryConfig()
	w := NewWorld(cfg)
	walker := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 700, Y: 100, Vitality: 80,
		Genome: genomeOf(50, 50, 50)}))
	stayer := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80,
		Genome: genomeOf(50, 50, 50)}))

	rough := w.regionIndexAt(700, 100)
	for i := 0; i < 30; i++ {
		w.noteRegion(walker, 3)
	}
	w.noteRegion(stayer, 3)
	if _, known := w.regionCostEstimate(stayer, rough); known {
		t.Fatal("knows the going somewhere it has never been, before being told")
	}

	w.exchangeLore(stayer, walker)
	cost, known := w.regionCostEstimate(stayer, rough)
	if !known {
		t.Fatal("was told nothing about the going over there")
	}
	if math.Abs(cost-cfg.RoughMoveCost) > 0.5 {
		t.Fatalf("was told the going is x%.2f, want about x%.1f", cost, cfg.RoughMoveCost)
	}

	// And with the telling off, it stays ignorant (29a without 29b).
	cfg.RegionCostTold = false
	w2 := NewWorld(cfg)
	w1 := mustAgent(t, w2, w2.addAgent(Agent{Maturity: 1, X: 700, Y: 100, Vitality: 80,
		Genome: genomeOf(50, 50, 50)}))
	s2 := mustAgent(t, w2, w2.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80,
		Genome: genomeOf(50, 50, 50)}))
	for i := 0; i < 30; i++ {
		w2.noteRegion(w1, 3)
	}
	w2.noteRegion(s2, 3)
	w2.exchangeLore(s2, w1)
	if _, known := w2.regionCostEstimate(s2, w2.regionIndexAt(700, 100)); known {
		t.Fatal("heard about the going with the telling turned off")
	}
}

// A world with no map has nothing to learn about the going, so the stage is
// invisible in it - which is what keeps every earlier measurement comparable.
func TestAFlatWorldLearnsNothingAboutTheGoing(t *testing.T) {
	cfg := quietConfig()
	cfg.RegionNoise = 0
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80,
		Genome: genomeOf(50, 50, 50)}))
	for i := 0; i < 20; i++ {
		w.noteRegion(a, 4)
	}
	cost, _ := w.regionCostEstimate(a, w.regionIndexAt(a.X, a.Y))
	if math.Abs(cost-1) > 1e-9 {
		t.Fatalf("reckons flat ground costs x%v", cost)
	}
	if got := w.worthOfRegion(a, w.regionIndexAt(a.X, a.Y), 4); math.Abs(got-4) > 1e-9 {
		t.Fatalf("a region is worth %v rather than the %v it sees, on flat ground", got, 4.0)
	}
}

// --- stage 35: where the ground kills ---------------------------------------

// A world of two halves: dry in the west, a river down the east. Regions are
// the default four by three over 800 by 600, so the water lies wholly in the
// eastern column and nothing in the west is dangerous.
func drownCountryConfig() Config {
	cfg := quietConfig()
	cfg.RegionNoise = 0
	cfg.Width, cfg.Height = 800, 600
	cfg.DrownChancePerTick = 0.01
	cfg.TerrainMap = []string{
		"......~~",
		"......~~",
		"......~~",
	}
	return cfg
}

// An agent learns how dangerous the ground it stands in is, in the same units
// the world holds it, and nothing about the ground it has not stood in.
func TestAnAgentLearnsHowDangerousItsOwnGroundIs(t *testing.T) {
	cfg := drownCountryConfig()
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 750, Y: 100, Vitality: 80,
		Genome: genomeOf(50, 50, 50)}))

	here := w.regionIndexAt(a.X, a.Y)
	for i := 0; i < 30; i++ {
		w.noteRegion(a, 3)
	}
	danger, known := w.regionDangerEstimate(a, here)
	if !known {
		t.Fatal("stood in the river thirty times and learned nothing about it")
	}
	if math.Abs(danger-cfg.DrownChancePerTick) > cfg.DrownChancePerTick/3 {
		t.Fatalf("reckons a tick here risks %v, want about %v", danger, cfg.DrownChancePerTick)
	}
	for r := range w.regions {
		if r == here {
			continue
		}
		if _, known := w.regionDangerEstimate(a, r); known {
			t.Fatalf("fears region %d without ever having been there", r)
		}
	}
}

// Watching somebody drown teaches the onlookers about the place that killed -
// which is not where they are standing, and need not be anywhere they have
// ever been. This is the whole of the stage: the ones the water takes do not
// come back to say so.
func TestWatchingADrowningTeachesThePlaceThatKilled(t *testing.T) {
	cfg := drownCountryConfig()
	cfg.DrownChancePerTick = 1 // it goes under on the first tick
	w := NewWorld(cfg)
	w.agents = w.agents[:0]
	// The victim just inside the water, the witness just outside it: near
	// enough to watch, dry, and in the region next door.
	victim := w.addAgent(Agent{Maturity: 1, X: 610, Y: 100, Vitality: 80, Genome: genomeOf(50, 50, 50)})
	witness := w.addAgent(Agent{Maturity: 1, X: 590, Y: 100, Vitality: 80, Genome: genomeOf(50, 50, 50)})
	far := w.addAgent(Agent{Maturity: 1, X: 100, Y: 500, Vitality: 80, Genome: genomeOf(50, 50, 50)})

	v := mustAgent(t, w, victim)
	river := w.regionIndexAt(v.X, v.Y)
	if river == w.regionIndexAt(590, 100) {
		t.Fatal("the witness is standing in the same region as the drowning; the test cannot tell them apart")
	}

	w.Step()
	if w.Stats().DrownDeaths != 1 {
		t.Fatalf("nobody drowned: %+v", w.Stats())
	}
	danger, known := w.regionDangerEstimate(mustAgent(t, w, witness), river)
	if !known || danger <= 0 {
		t.Fatalf("watched somebody go under and learned nothing (danger %v, known %v)", danger, known)
	}
	if _, known := w.regionDangerEstimate(mustAgent(t, w, far), river); known {
		t.Fatal("somebody across the world learned about a drowning it could not see")
	}
	if got := w.Stats().DrownWitnesses; got != 1 {
		t.Fatalf("counted %d witnesses, want 1", got)
	}
}

// With the weight at zero the drowning is not news: the water still kills and
// nobody learns anything by watching. This is the arm the stage is measured
// against.
func TestWithNoWitnessWeightADrowningTeachesNobody(t *testing.T) {
	cfg := drownCountryConfig()
	cfg.DrownChancePerTick = 1
	cfg.DrownWitnessLooks = 0
	w := NewWorld(cfg)
	w.agents = w.agents[:0]
	w.addAgent(Agent{Maturity: 1, X: 610, Y: 100, Vitality: 80, Genome: genomeOf(50, 50, 50)})
	witness := w.addAgent(Agent{Maturity: 1, X: 590, Y: 100, Vitality: 80, Genome: genomeOf(50, 50, 50)})
	river := w.regionIndexAt(610, 100)

	w.Step()
	if w.Stats().DrownDeaths != 1 {
		t.Fatal("nobody drowned")
	}
	if _, known := w.regionDangerEstimate(mustAgent(t, w, witness), river); known {
		t.Fatal("learned from a drowning with the weight at zero")
	}
}

// And it travels: an agent that has neither been to the river nor seen it take
// anybody can still be told, on the same trade everything else rides on.
func TestTheDangerIsHandedOn(t *testing.T) {
	cfg := drownCountryConfig()
	w := NewWorld(cfg)
	knows := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 750, Y: 100, Vitality: 80,
		Genome: genomeOf(50, 50, 50)}))
	hears := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80,
		Genome: genomeOf(50, 50, 50)}))

	river := w.regionIndexAt(knows.X, knows.Y)
	for i := 0; i < 30; i++ {
		w.noteRegion(knows, 3)
	}
	w.exchangeRegions(hears, knows)

	danger, known := w.regionDangerEstimate(hears, river)
	if !known || danger <= 0 {
		t.Fatalf("was told nothing about the river (danger %v, known %v)", danger, known)
	}

	// And with the telling off it stays where it was learned.
	cfg.RegionDangerTold = false
	quiet := NewWorld(cfg)
	a := mustAgent(t, quiet, quiet.addAgent(Agent{Maturity: 1, X: 750, Y: 100, Vitality: 80,
		Genome: genomeOf(50, 50, 50)}))
	b := mustAgent(t, quiet, quiet.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80,
		Genome: genomeOf(50, 50, 50)}))
	for i := 0; i < 30; i++ {
		quiet.noteRegion(a, 3)
	}
	quiet.exchangeRegions(b, a)
	if _, known := quiet.regionDangerEstimate(b, river); known {
		t.Fatal("the danger was handed on with the telling off")
	}
}

// A place believed to kill is worth less to go to. Two regions with the same
// food, one of them a river: the dry one is what the agent heads for.
func TestADangerousPlaceIsWorthLess(t *testing.T) {
	cfg := drownCountryConfig()
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 300, Vitality: 80,
		Genome: genomeOf(50, 50, 50)}))

	dryIdx := w.regionIndexAt(100, 100)
	riverIdx := w.regionIndexAt(750, 100)
	hereIdx := w.regionIndexAt(a.X, a.Y)
	if dryIdx == riverIdx || hereIdx == dryIdx || hereIdx == riverIdx {
		t.Fatal("the three places are not three regions")
	}
	a.regions = make([]regionView, len(w.regions))
	for _, c := range []struct {
		i      int
		seen   float64
		danger float64
	}{
		{hereIdx, 2, 0},
		{dryIdx, 6, 0},
		{riverIdx, 6, cfg.DrownChancePerTick},
	} {
		a.regions[c.i].setSeen(c.seen, 10, w.tick)
		a.regions[c.i].danger = c.danger
	}

	best, _, ok := w.bestKnownRegion(a)
	if !ok {
		t.Fatal("knows nowhere better than here")
	}
	if best != dryIdx {
		t.Fatalf("heads for region %d, want the dry one (%d) over the river (%d)", best, dryIdx, riverIdx)
	}

	// With the belief priced out the two are the same place again, which is
	// the control arm the stage is measured against.
	cfg.RegionDangerTicks = 0
	blind := NewWorld(cfg)
	blind.agents = w.agents
	blind.index = w.index
	if got := blind.worthOfRegion(mustAgent(t, blind, a.ID), riverIdx, 6); got != 6 {
		t.Fatalf("the river is worth %v with the danger unpriced, want 6", got)
	}
}

// The danger is never priced above a life: a belief so bad that the stay is
// certain death costs exactly that and no more.
func TestTheFearedGroundIsNeverWorseThanCertainDeath(t *testing.T) {
	cfg := drownCountryConfig()
	w := NewWorld(cfg)
	if got, want := w.dangerPrice(1), cfg.LifeValue/cfg.RegionDrawValue; got != want {
		t.Fatalf("certain death priced at %v, want %v", got, want)
	}
	if got, want := w.dangerPrice(50), cfg.LifeValue/cfg.RegionDrawValue; got != want {
		t.Fatalf("a hopeless belief priced at %v, want %v", got, want)
	}
}

// A flat world has no water in it, so nobody ever fears anywhere - whatever
// the weight is set to. This is what keeps every measurement taken on level
// ground comparable.
func TestAFlatWorldFearsNowhere(t *testing.T) {
	run := func(ticks float64) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 9
		cfg.RegionDangerTicks = ticks
		w := NewWorld(cfg)
		for i := 0; i < 400; i++ {
			w.Step()
		}
		for i := range w.agents {
			for r := range w.agents[i].regions {
				if w.agents[i].regions[r].danger > 0 {
					t.Fatalf("an agent in a flat world fears region %d", r)
				}
			}
		}
		return w.Stats()
	}
	if off, on := run(0), run(700); off != on {
		t.Fatalf("a flat world ran differently with the danger weighed:\n off %+v\n on  %+v", off, on)
	}
}
