package engine

import (
	"math"
	"testing"
)

// wadingWorld is a still world with a river down the middle of it, slowed.
//
//	. . . ~ ~ . . .
//	. . . ~ ~ . . .
//	. . . ~ ~ . . .
func wadingWorld(t *testing.T, share float64) *World {
	t.Helper()
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	cfg.TerrainMap = []string{
		"...~~...",
		"...~~...",
		"...~~...",
	}
	cfg.WaterSpeedShare = share
	return NewWorld(cfg)
}

// wader puts a body somewhere and hands back its ID rather than a pointer:
// adding the next one may move the slice underneath, so every test here takes
// its pointers after the last body is in.
func wader(t *testing.T, w *World, x, y float64) int {
	t.Helper()
	g := genomeOf(50, 50, 50)
	g[GeneSpeed] = 60
	id := w.addAgent(Agent{Maturity: 1, X: x, Y: y, Vitality: 90, Genome: g})
	mustAgent(t, w, id).hintSlots = 2
	return id
}

// The default is a world where water is dear and no slower, which is what
// every map measured before this stage ran on. Nothing is written onto any
// body, so speedNow is what it was.
func TestWaterSlowsNobodyByDefault(t *testing.T) {
	if got := DefaultConfig().WaterSpeedShare; got != 1 {
		t.Fatalf("water keeps %v of a body's speed by default, want 1", got)
	}
	w := wadingWorld(t, 1)
	a := mustAgent(t, w, wader(t, w, 400, 300)) // in the river
	w.wade()
	if a.wading != 0 {
		t.Fatalf("the rule is off and it wrote %v onto a body", a.wading)
	}
	if got, want := a.speedNow(&w.cfg), a.MaxSpeed(&w.cfg); got != want {
		t.Fatalf("a body in unslowed water goes at %v, want its built %v", got, want)
	}
}

// With the rule on, being in the water is slower and being out of it is not.
func TestTheWaterDragsAndTheBankDoesNot(t *testing.T) {
	w := wadingWorld(t, 0.5)
	wetID, dryID := wader(t, w, 400, 300), wader(t, w, 100, 300)
	w.wade()
	wet, dry := mustAgent(t, w, wetID), mustAgent(t, w, dryID)

	built := wet.MaxSpeed(&w.cfg)
	if got, want := wet.speedNow(&w.cfg), built*0.5; math.Abs(got-want) > 1e-9 {
		t.Fatalf("a body in the river goes at %v, want %v", got, want)
	}
	if got := dry.speedNow(&w.cfg); math.Abs(got-built) > 1e-9 {
		t.Fatalf("a body on the bank goes at %v, want its built %v", got, built)
	}

	// And it is the distance actually covered, not only a number: two bodies
	// walking the same way for a tick end up in different places.
	beforeWet, beforeDry := wet.X, dry.X
	w.moveDir(wet, -1, 0, 1)
	w.moveDir(dry, -1, 0, 1)
	wetGone, dryGone := beforeWet-wet.X, beforeDry-dry.X
	if !(wetGone < dryGone*0.9) {
		t.Fatalf("the wader covered %v and the walker %v", wetGone, dryGone)
	}
}

// The ground is charged by the tick, so a body that wades at half speed pays
// the water's cost for twice as many ticks to cross it. That is deliberate and
// it is the hidden dose stage 97 has an arm for; this pins it so that nobody
// takes it for a bug.
func TestWadingPaysTheWatersCostForLonger(t *testing.T) {
	w := wadingWorld(t, 0.5)
	wet := mustAgent(t, w, wader(t, w, 400, 300))
	w.wade()

	// Per tick, the cost is the ground's and has nothing to do with the pace.
	fast := wadingWorld(t, 1)
	quick := mustAgent(t, fast, wader(t, fast, 400, 300))
	fast.wade()
	if a, b := w.moveCostOn(wet, 400, 300, 1), fast.moveCostOn(quick, 400, 300, 1); a != b {
		t.Fatalf("a tick of wading costs %v and a tick of walking the river %v: the cost is not the pace", a, b)
	}
	// Per unit of distance, it is twice as much.
	perStep := w.moveCostOn(wet, 400, 300, 1) / wet.speedNow(&w.cfg)
	perStepQuick := fast.moveCostOn(quick, 400, 300, 1) / quick.speedNow(&w.cfg)
	if math.Abs(perStep-2*perStepQuick) > 1e-9 {
		t.Fatalf("crossing a unit of river costs %v slowed and %v not: want twice", perStep, perStepQuick)
	}
}

// Knowing the water gives the drag back, and no amount of it makes the river
// quicker than a field - the mirror of what rough going does with its cost.
func TestKnowingTheWaterKeepsABodyQuickInIt(t *testing.T) {
	w := wadingWorld(t, 0.5)
	greenID, adeptID := wader(t, w, 400, 300), wader(t, w, 400, 300)
	green, adept := mustAgent(t, w, greenID), mustAgent(t, w, adeptID)
	for _, a := range []*Agent{green, adept} {
		a.Genome[GeneVitality] = 100 // the aptitude swimming is capped by
	}
	w.learnSkill(adept, SkillSwim, 1)
	w.wade()

	if !(green.wading < adept.wading) {
		t.Fatalf("the swimmer is dragged to %v and anybody else to %v", adept.wading, green.wading)
	}
	if adept.wading > 1 {
		t.Fatalf("the swimmer goes at %v of its speed in the river: the water may not help", adept.wading)
	}
	if got, want := adept.speedNow(&w.cfg), adept.MaxSpeed(&w.cfg); math.Abs(got-want) > 1e-9 {
		t.Fatalf("a master swimmer goes at %v in the river, want its built %v", got, want)
	}

	// With the relief off it is dragged exactly as much as anybody.
	w.cfg.SkillSwimSpeedRelief = 0
	w.wade()
	if adept.wading != green.wading {
		t.Fatalf("with the relief off the swimmer keeps %v and anybody else %v", adept.wading, green.wading)
	}
}

// The two halves of swimming are two figures. Switching off the one that keeps
// a body quick leaves the one that keeps it alive standing, and the other way
// round - which is what lets the measurement say which half did anything.
func TestTheTwoHalvesOfSwimmingComeApart(t *testing.T) {
	w := wadingWorld(t, 0.5)
	adept := mustAgent(t, w, wader(t, w, 400, 300))
	adept.Genome[GeneVitality] = 100
	w.learnSkill(adept, SkillSwim, 1)
	water := w.terrainAt(400, 300)

	w.cfg.SkillSwimSpeedRelief = 0
	w.wade()
	if adept.wading >= 1 {
		t.Fatal("with the speed relief off the swimmer is still not dragged")
	}
	if got := w.drownChanceFor(adept, water); got >= water.Drown {
		t.Fatalf("switching off the speed half took the drowning half with it: %v", got)
	}

	w.cfg.SkillSwimSpeedRelief, w.cfg.SkillSwimRelief = 1, 0
	w.wade()
	if adept.wading < 1 {
		t.Fatalf("switching off the drowning half took the speed half with it: %v", adept.wading)
	}
	if got := w.drownChanceFor(adept, water); got != water.Drown {
		t.Fatalf("with the drowning relief off the swimmer's chance is %v, want %v", got, water.Drown)
	}
}

// What lives in the river is not something the river wades. It is the same
// flag that keeps it from drowning (stage 63), and the same one fact.
func TestACreatureOfTheWaterIsNotDragged(t *testing.T) {
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	cfg.TerrainMap = []string{"...~~...", "...~~...", "...~~..."}
	cfg.WaterSpeedShare = 0.5
	cfg.EnemyKinds = []EnemyKind{{Name: "lurker", Share: 1, Water: true}}
	w := NewWorld(cfg)

	g := genomeOf(50, 50, 50)
	beastID := w.addAgent(Agent{Maturity: 1, X: 400, Y: 300, Vitality: 90,
		Species: SpeciesEnemy, Genome: g})
	personID := wader(t, w, 400, 300)
	w.wade()
	beast, person := mustAgent(t, w, beastID), mustAgent(t, w, personID)

	if beast.wading < 1 {
		t.Fatalf("what lives in the river is dragged by it to %v", beast.wading)
	}
	if person.wading >= 1 {
		t.Fatalf("a person in the river is not dragged: %v", person.wading)
	}
}

// A body feels its own legs, and the arm that takes the feeling away leaves
// the water exactly as slow: the same shape as DrownKnown.
func TestABodyFeelsHowSlowTheWaterHasMadeIt(t *testing.T) {
	w := wadingWorld(t, 0.5)
	a := mustAgent(t, w, wader(t, w, 400, 300))
	w.wade()

	if got, want := w.perceive(a).Self.MaxSpeed, a.speedNow(&w.cfg); math.Abs(got-want) > 1e-9 {
		t.Fatalf("it thinks it can do %v and it can do %v", got, want)
	}

	w.cfg.WaterSlowKnown = false
	felt := w.perceive(a).Self.MaxSpeed
	if got, want := felt, a.MaxSpeed(&w.cfg); math.Abs(got-want) > 1e-9 {
		t.Fatalf("blinded, it thinks it can do %v, want its dry-land %v", got, want)
	}
	// And the world has not become any kinder: it still moves at the slow one.
	before := a.X
	w.moveDir(a, -1, 0, 1)
	if gone, want := before-a.X, a.MaxSpeed(&w.cfg)*0.5; math.Abs(gone-want) > 1e-9 {
		t.Fatalf("blinded, it covered %v, want the slowed %v", gone, want)
	}
}

// A world with no water in it never enters the loop, whatever the setting says.
func TestAWorldWithNoWaterIsUntouched(t *testing.T) {
	cfg := quietConfig()
	cfg.WaterSpeedShare = 0.25
	w := NewWorld(cfg)
	a := mustAgent(t, w, wader(t, w, 200, 200))
	w.wade()
	if a.wading != 0 {
		t.Fatalf("a flat world wrote %v onto a body", a.wading)
	}
	if got, want := a.speedNow(&w.cfg), a.MaxSpeed(&w.cfg); got != want {
		t.Fatalf("a body on a flat world goes at %v, want %v", got, want)
	}
}

// Everything that reads a speed reads the same one: what it covers, what it
// believes it covers, and what its footwork is worth.
func TestTheReadersOfASpeedAgree(t *testing.T) {
	w := wadingWorld(t, 0.5)
	wetID, dryID := wader(t, w, 400, 300), wader(t, w, 100, 300)
	wet, dry := mustAgent(t, w, wetID), mustAgent(t, w, dryID)
	for _, a := range []*Agent{wet, dry} {
		a.Genome[GeneEvasion] = 80
	}
	w.wade()

	// One at a time: the world keeps a single Perception and hands the same
	// one back to every caller.
	wetSelf := w.perceive(wet).Self
	drySelf := w.perceive(dry).Self
	if !(wetSelf.MaxSpeed < drySelf.MaxSpeed) {
		t.Fatalf("the wader believes it does %v and the walker %v", wetSelf.MaxSpeed, drySelf.MaxSpeed)
	}
	if !(wetSelf.Evasion < drySelf.Evasion) {
		t.Fatalf("the wader dodges %v and the walker %v: footwork reads the same legs", wetSelf.Evasion, drySelf.Evasion)
	}
}

// The measurement reads the drag on the bodies that are in the water, and says
// "no drag" rather than nought when there is nobody in it - so that averaging
// a run's samples cannot put the figure below the floor the ground sets.
func TestWadingReadsTheBodiesInTheWater(t *testing.T) {
	w := wadingWorld(t, 0.5)

	if got := w.Wading(); got.In != 0 || got.Pace != 1 || got.Floor != 1 {
		t.Fatalf("an empty world is wading %+v, want nobody in it and no drag", got)
	}

	dryID := wader(t, w, 100, 300)
	w.wade()
	if got := w.Wading(); got.In != 0 || got.Pace != 1 {
		t.Fatalf("with everybody on the bank it is wading %+v", got)
	}

	greenID, adeptID := wader(t, w, 400, 300), wader(t, w, 400, 300)
	mustAgent(t, w, adeptID).Genome[GeneVitality] = 100
	w.learnSkill(mustAgent(t, w, adeptID), SkillSwim, 1)
	w.wade()
	_ = dryID
	_ = greenID

	got := w.Wading()
	if want := 2.0 / 3.0; math.Abs(got.In-want) > 1e-9 {
		t.Fatalf("%v of the world is in the water, want %v", got.In, want)
	}
	if math.Abs(got.Floor-0.5) > 1e-9 {
		t.Fatalf("the ground alone would make it %v, want 0.5", got.Floor)
	}
	if !(got.Pace > got.Floor) {
		t.Fatalf("the ones in it move at %v against a floor of %v: the skill bought nothing", got.Pace, got.Floor)
	}
	if got.Pace > 1 {
		t.Fatalf("the ones in it move at %v: the water may not help", got.Pace)
	}
}
