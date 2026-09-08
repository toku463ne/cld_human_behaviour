package engine

import (
	"math"
	"testing"
)

// A world with no map is level ground everywhere, which is the default and
// what every measurement before stage 20 was taken in.
func TestTheWholeWorldIsLevelGround(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	for x := 0.0; x <= cfg.Width; x += cfg.Width / 8 {
		for y := 0.0; y <= cfg.Height; y += cfg.Height / 8 {
			if got := w.terrainAt(x, y); got != flatGround {
				t.Fatalf("the ground at (%.0f, %.0f) is %+v, want %+v", x, y, got, flatGround)
			}
		}
	}
}

// Movement is charged through the ground, so that giving the world hills is a
// change in terrain.go and nowhere else. With level ground it comes to exactly
// what it always did.
func TestMovementIsChargedThroughTheGround(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)

	for _, effort := range []float64{0.4, 1} {
		want := moveCostAt(&cfg, effort)
		if got := w.moveCostOn(nil, 100, 100, effort); got != want {
			t.Fatalf("crossing level ground at effort %.1f cost %v, want the flat %v", effort, got, want)
		}
	}

	// And what an agent actually pays when it moves is that same figure.
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80, Genome: genomeOf(50, 50, 50)}))
	before := a.Vitality
	w.moveDir(a, 1, 0, 1)
	if paid, want := before-a.Vitality, moveCostAt(&cfg, 1); math.Abs(paid-want) > 1e-9 {
		t.Fatalf("moving cost %v of vitality, want %v", paid, want)
	}
}

// --- the map (stage 20) -----------------------------------------------------

// terrainConfig is a still world laid over a map: open ground on the left,
// rough in the middle, a river, and a two-level plateau on the right whose
// only way up is the ramp on its bottom row.
//
//	. . : : ~ . 1 1
//	. . : : ~ A 1 2
//	. . : : ~ . 1 1
func terrainConfig() Config {
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	cfg.TerrainMap = []string{
		"..::~.11",
		"..::~A12",
		"..::~.11",
	}
	return cfg
}

// Each rune is the piece of country it says it is, and a map of eight by three
// over an 800 by 600 world makes cells of 100 by 200.
func TestAMapPutsTheCountryWhereItSaysItIs(t *testing.T) {
	cfg := terrainConfig()
	w := NewWorld(cfg)
	cols, rows, cw, ch := w.TerrainSize()
	if cols != 8 || rows != 3 || cw != 100 || ch != 200 {
		t.Fatalf("the map is %dx%d cells of %.0fx%.0f", cols, rows, cw, ch)
	}

	for _, c := range []struct {
		x, y   float64
		kind   Ground
		cost   float64
		height int
		slope  bool
	}{
		{50, 100, GroundOpen, 1, 0, false},
		{250, 100, GroundRough, cfg.RoughMoveCost, 0, false},
		{450, 100, GroundWater, cfg.WaterMoveCost, 0, false},
		{550, 300, GroundSlope, cfg.SlopeMoveCost, 1, true},
		{650, 100, GroundOpen, 1, 1, false},
		{750, 300, GroundOpen, 1, 2, false},
		{9999, 9999, GroundOpen, 1, 0, false}, // off the map is level ground
	} {
		got := w.TerrainAt(c.x, c.y)
		if got.Kind != c.kind || got.Cost != c.cost || got.Height != c.height || got.Slope != c.slope {
			t.Fatalf("the ground at (%.0f, %.0f) is %+v, want kind %v cost %v height %d slope %v",
				c.x, c.y, got, c.kind, c.cost, c.height, c.slope)
		}
	}
}

// Crossing dear country takes more out of a body than crossing open ground,
// and what it pays is what the map says.
func TestDearGroundCostsMoreToCross(t *testing.T) {
	cfg := terrainConfig()
	w := NewWorld(cfg)

	open := w.moveCostOn(nil, 50, 100, 1)
	rough := w.moveCostOn(nil, 250, 100, 1)
	water := w.moveCostOn(nil, 450, 100, 1)
	if !(open < rough && rough < water) {
		t.Fatalf("open %v, rough %v, water %v: want each dearer than the last", open, rough, water)
	}

	// And it is what an agent actually pays for the tick, charged for the
	// ground it ends the step on.
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 249, Y: 100, Vitality: 80,
		Genome: genomeOf(50, 50, 50)}))
	before := a.Vitality
	w.moveDir(a, 1, 0, 1)
	if paid := before - a.Vitality; math.Abs(paid-rough) > 1e-9 {
		t.Fatalf("crossing the rough cost %v, want %v", paid, rough)
	}
}

// The only way onto high ground is a ramp, and a body that walks into the side
// of it goes nowhere - and pays nothing for not going.
func TestHighGroundIsReachedOnlyByTheRamp(t *testing.T) {
	cfg := terrainConfig()
	w := NewWorld(cfg)

	// Straight at the cliff: the cell to the east is a level up and is not a
	// slope, and the cells north and south of it are the same, so the step is
	// refused whichever half of it is tried.
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 599, Y: 100, Vitality: 80,
		Genome: genomeOf(50, 50, 50)}))
	before := *a
	w.moveDir(a, 1, 0, 1)
	if a.X != before.X || a.Y != before.Y {
		t.Fatalf("walked up a cliff: (%.1f, %.1f) -> (%.1f, %.1f)", before.X, before.Y, a.X, a.Y)
	}
	if a.Vitality != before.Vitality {
		t.Fatalf("paid %v for a step it did not take", before.Vitality-a.Vitality)
	}

	// The same step where the ramp is: allowed, and now it is a level up.
	b := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 499, Y: 300, Vitality: 80,
		Genome: genomeOf(50, 50, 50)}))
	w.moveDir(b, 1, 0, 1)
	if got := w.TerrainAt(b.X, b.Y); got.Height != 1 {
		t.Fatalf("the ramp led to height %d at (%.1f, %.1f)", got.Height, b.X, b.Y)
	}
}

// Two levels at once is refused even on a ramp: getting to the top means
// finding the next ramp when you are up there.
func TestALevelAtATime(t *testing.T) {
	cfg := terrainConfig()
	w := NewWorld(cfg)
	if w.canStep(550, 300, 750, 300) {
		t.Fatal("stepped from the ramp at level 1 straight onto level 2")
	}
	if w.canStep(650, 300, 750, 300) {
		t.Fatal("climbed from level 1 to level 2 with no ramp on either side")
	}
	// Down the way it came up, though: a ramp is a way in and a way out.
	if !w.canStep(550, 300, 450, 300) {
		t.Fatal("could not come back down the ramp")
	}
}

// Sliding: a body denied the step it wanted takes the half of it the ground
// allows, which is how it walks along the foot of a cliff instead of sticking
// to it.
func TestABlockedBodySlidesAlongTheEdge(t *testing.T) {
	cfg := terrainConfig()
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 599, Y: 100, Vitality: 80,
		Genome: genomeOf(50, 50, 50)}))
	before := *a
	w.moveDir(a, 1, 1, 1) // north-east, into the cliff and along it
	if a.X != before.X {
		t.Fatalf("crossed the cliff: x %.1f -> %.1f", before.X, a.X)
	}
	if a.Y <= before.Y {
		t.Fatalf("did not slide: y %.1f -> %.1f", before.Y, a.Y)
	}
}

// What an agent knows about the ground is what is under its own feet, and it
// is the same figure a player is shown.
func TestAnAgentFeelsTheGroundItStandsOn(t *testing.T) {
	cfg := terrainConfig()
	w := NewWorld(cfg)
	open := w.addAgent(Agent{Maturity: 1, X: 50, Y: 100, Vitality: 80, Genome: genomeOf(50, 50, 50)})
	rough := w.addAgent(Agent{Maturity: 1, X: 250, Y: 100, Vitality: 80, Genome: genomeOf(50, 50, 50)})

	if got := w.perceive(mustAgent(t, w, open)).Self.Ground; got != 1 {
		t.Fatalf("an agent on open ground feels %v, want 1", got)
	}
	if got := w.perceive(mustAgent(t, w, rough)).Self.Ground; got != cfg.RoughMoveCost {
		t.Fatalf("an agent on the rough feels %v, want %v", got, cfg.RoughMoveCost)
	}

	// And it reckons with it: the same option costs more to the one standing
	// on the dear ground.
	cheap := moveCost(&cfg, &SelfView{Ground: 1}, 1)
	dear := moveCost(&cfg, &SelfView{Ground: cfg.RoughMoveCost}, 1)
	if !(dear > cheap) {
		t.Fatalf("a body on the rough reckons a step at %v against %v on the open", dear, cheap)
	}
}

// A world with no map runs exactly as it did: every step allowed, every cost
// the flat one. This is what makes every measurement before stage 20 still
// comparable.
func TestNoMapMeansNothingChanged(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)
	if !w.canStep(0, 0, 799, 599) {
		t.Fatal("a flat world refused a step")
	}
	if got := w.perceive(mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100,
		Vitality: 80, Genome: genomeOf(50, 50, 50)}))).Self.Ground; got != 1 {
		t.Fatalf("an agent in a flat world feels ground %v, want 1", got)
	}
}

// --- stage 34: what the water does -----------------------------------------

// A world with no map has no water in it, so the rule cannot fire however high
// the chance is set - and, because it draws no random number, such a world
// runs to exactly the same state it did before the rule existed. This is the
// same guarantee stage 20 gave, and it is what keeps every measurement taken
// on flat ground comparable.
func TestAFlatWorldNeverDrowns(t *testing.T) {
	run := func(chance float64) Stats {
		cfg := testConfig()
		cfg.DrownChancePerTick = chance
		w := NewWorld(cfg)
		for i := 0; i < 400; i++ {
			w.Step()
		}
		return w.Stats()
	}
	off, on := run(0), run(0.5)
	if off != on {
		t.Fatalf("a flat world ran differently with drowning on:\n off %+v\n on  %+v", off, on)
	}
	if on.DrownDeaths != 0 {
		t.Fatalf("a flat world drowned %d agents", on.DrownDeaths)
	}
}

// Every tick spent in the water is a throw of the dice, so a body that stays
// in it goes under. With the chance at one it takes exactly one tick, which is
// the deterministic form of the rule.
func TestTheWaterTakesTheBodiesThatStayInIt(t *testing.T) {
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	cfg.TerrainMap = []string{"~~~~"}
	cfg.DrownChancePerTick = 1
	w := NewWorld(cfg)
	w.agents = w.agents[:0]
	id := w.addAgent(Agent{Maturity: 1, X: 400, Y: 300, Vitality: 80, Genome: genomeOf(50, 50, 50)})

	w.Step()
	if a := w.agentByID(id); a != nil {
		t.Fatalf("an agent standing in the water at chance 1 is still alive after a tick (vitality %.1f)", a.Vitality)
	}
	if got := w.Stats().DrownDeaths; got != 1 {
		t.Fatalf("drowned %d, want 1", got)
	}
}

// The dry half of the same map is dry. A rule that killed everybody would pass
// the test above.
func TestOnlyTheWaterDrowns(t *testing.T) {
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	cfg.TerrainMap = []string{"~~.."}
	cfg.DrownChancePerTick = 1
	w := NewWorld(cfg)
	w.agents = w.agents[:0]
	dry := w.addAgent(Agent{Maturity: 1, X: 700, Y: 300, Vitality: 80, Genome: genomeOf(50, 50, 50)})

	for i := 0; i < 20; i++ {
		w.Step()
	}
	if w.agentByID(dry) == nil {
		t.Fatal("an agent on dry ground drowned")
	}
	if got := w.Stats().DrownDeaths; got != 0 {
		t.Fatalf("drowned %d on dry ground, want 0", got)
	}
}

// Drowning is a cause of death of its own and not a killing, even when
// somebody had been hitting the body a moment before. The buckets have to stay
// exclusive: starvation is read as what the other counters do not claim, so a
// death counted twice would turn up as a change in starvation in every
// measurement taken from here on.
func TestDrowningIsItsOwnCauseOfDeath(t *testing.T) {
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	cfg.TerrainMap = []string{"~~~~"}
	cfg.DrownChancePerTick = 1
	w := NewWorld(cfg)
	w.agents = w.agents[:0]
	id := w.addAgent(Agent{Maturity: 1, X: 400, Y: 300, Vitality: 80, Genome: genomeOf(50, 50, 50)})
	mustAgent(t, w, id).lastAttackTick = w.tick

	w.Step()
	st := w.Stats()
	if st.DrownDeaths != 1 || st.Kills != 0 {
		t.Fatalf("a body that was being hit and then drowned counted as %d drownings and %d killings, want 1 and 0", st.DrownDeaths, st.Kills)
	}
	if st.Deaths-st.Kills-st.AgingDeaths-st.DrownDeaths != 0 {
		t.Fatalf("the causes do not add up: %+v", st)
	}
}

// An agent feels how dangerous its own footing is, exactly as it feels what
// the ground costs - and nothing about the country it is not standing on.
func TestAnAgentFeelsHowDangerousItsFootingIs(t *testing.T) {
	cfg := terrainConfig()
	cfg.DrownChancePerTick = 0.01
	w := NewWorld(cfg)
	dry := w.addAgent(Agent{Maturity: 1, X: 50, Y: 100, Vitality: 80, Genome: genomeOf(50, 50, 50)})
	wet := w.addAgent(Agent{Maturity: 1, X: 450, Y: 100, Vitality: 80, Genome: genomeOf(50, 50, 50)})

	if got := w.perceive(mustAgent(t, w, dry)).Self.Drown; got != 0 {
		t.Fatalf("an agent on dry ground feels a drowning chance of %v, want 0", got)
	}
	if got := w.perceive(mustAgent(t, w, wet)).Self.Drown; got != cfg.DrownChancePerTick {
		t.Fatalf("an agent in the water feels %v, want %v", got, cfg.DrownChancePerTick)
	}
}

// What the agent does with it: an option that would keep the body in the water
// longer is charged more for it, and one on dry ground is charged nothing.
// That is the whole of "the chance is priced into the comparison" - there is
// no threshold anywhere saying to keep out of the river.
func TestTheLongerTheOptionTheMoreTheWaterCosts(t *testing.T) {
	cfg := terrainConfig()
	cfg.DrownChancePerTick = 0.01
	c := &AIController{}
	c.drownChance, c.lifeValue = cfg.DrownChancePerTick, cfg.LifeValue

	c.add(Action{Kind: ActMove}, Utility{Ticks: 1})
	c.add(Action{Kind: ActMove}, Utility{Ticks: 10})
	short, long := c.opts[0].util, c.opts[1].util
	if !(long < short) {
		t.Fatalf("ten ticks in the water scored %v against one tick at %v", long, short)
	}

	dry := &AIController{}
	dry.add(Action{Kind: ActMove}, Utility{Ticks: 10})
	if dry.opts[0].util != 0 {
		t.Fatalf("an option on dry ground was charged %v for the ground", -dry.opts[0].util)
	}
}

// The hazard never prices a life at more than a life: an option long enough to
// be certain death is worth exactly that and no worse, which is what keeps a
// long plan in shallow danger from swamping every other term.
func TestTheWaterIsNeverWorseThanCertainDeath(t *testing.T) {
	cfg := terrainConfig()
	c := &AIController{}
	c.drownChance, c.lifeValue = 0.5, cfg.LifeValue
	c.add(Action{Kind: ActMove}, Utility{Ticks: 1000})
	if got := -c.opts[0].util; got != cfg.LifeValue {
		t.Fatalf("a hopeless crossing was charged %v, want %v", got, cfg.LifeValue)
	}
}
