package engine

import (
	"math"
	"testing"
)

// drownWorld is a still world with a river down the middle of it, deadly.
func drownWorld(t *testing.T, factor float64) *World {
	t.Helper()
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	cfg.TerrainMap = []string{
		"...~~...",
		"...~~...",
		"...~~...",
	}
	cfg.DrownUnskilledFactor = factor
	return NewWorld(cfg)
}

// swimmer puts a body in the world and hands back its ID: adding the next one
// may move the slice, so pointers are taken after the last body is in.
func swimmer(t *testing.T, w *World, x, y, mastery float64) int {
	t.Helper()
	g := genomeOf(50, 50, 50)
	g[GeneVitality] = 100 // the aptitude swimming is capped by
	id := w.addAgent(Agent{Maturity: 1, X: x, Y: y, Vitality: 90, Genome: g})
	a := mustAgent(t, w, id)
	a.hintSlots = 2
	if mastery > 0 {
		w.learnSkill(a, SkillSwim, mastery)
	}
	return id
}

// The default is the world stages 34 to 97 all ran in: the water takes the
// swimmer and the sinker alike, so every figure recorded before this stage
// still reads.
func TestTheWaterAsksTheSameOfEverybodyByDefault(t *testing.T) {
	if got := DefaultConfig().DrownUnskilledFactor; got != 1 {
		t.Fatalf("a body that cannot swim faces %v times the water by default, want 1", got)
	}
	w := drownWorld(t, 1)
	greenID, adeptID := swimmer(t, w, 400, 300, 0), swimmer(t, w, 400, 300, 1)
	green, adept := mustAgent(t, w, greenID), mustAgent(t, w, adeptID)
	water := w.terrainAt(400, 300)

	if got := w.drownFactorFor(green); got != 1 {
		t.Fatalf("the rule is off and it charges a non-swimmer %v", got)
	}
	w.cfg.SkillSwimRelief = 0 // leave only what this stage does
	if got := w.drownChanceFor(green, water); got != water.Drown {
		t.Fatalf("a non-swimmer faces %v, want the ground's own %v", got, water.Drown)
	}
	if got := w.drownChanceFor(adept, water); got != water.Drown {
		t.Fatalf("a swimmer faces %v, want the ground's own %v", got, water.Drown)
	}
}

// The point of the stage: the water asks more of a body that cannot swim.
func TestTheWaterAsksMoreOfABodyThatCannotSwim(t *testing.T) {
	w := drownWorld(t, 3)
	w.cfg.SkillSwimRelief = 0
	green := mustAgent(t, w, swimmer(t, w, 400, 300, 0))
	water := w.terrainAt(400, 300)

	if got, want := w.drownChanceFor(green, water), 3*water.Drown; math.Abs(got-want) > 1e-12 {
		t.Fatalf("a non-swimmer faces %v, want %v", got, want)
	}
}

// And a body that has reached the mastery this world can actually attain
// faces exactly what the water has always asked - not less.
func TestMasteryBringsTheRiskBackToWhatItAlwaysWas(t *testing.T) {
	w := drownWorld(t, 3)
	w.cfg.SkillSwimRelief = 0 // so that only this stage's curve is in the answer
	full := w.cfg.DrownSkillFull
	adeptID := swimmer(t, w, 400, 300, full)
	overID := swimmer(t, w, 400, 300, 1)
	halfID := swimmer(t, w, 400, 300, full/2)
	adept, over, half := mustAgent(t, w, adeptID), mustAgent(t, w, overID), mustAgent(t, w, halfID)
	water := w.terrainAt(400, 300)

	if got := w.drownChanceFor(adept, water); math.Abs(got-water.Drown) > 1e-12 {
		t.Fatalf("a body at the reachable mastery faces %v, want the ground's own %v", got, water.Drown)
	}
	if got := w.drownChanceFor(over, water); math.Abs(got-water.Drown) > 1e-12 {
		t.Fatalf("mastery beyond the point buys more than the ground's own: %v", got)
	}
	if got, want := w.drownChanceFor(half, water), 2*water.Drown; math.Abs(got-want) > 1e-12 {
		t.Fatalf("halfway there faces %v, want %v", got, want)
	}
}

// The relief (stage 38b) is the same curve carried on below one: with it on,
// knowing the water buys something beyond simply not being penalised. The two
// come apart, so an arm can switch either off.
func TestTheCurveRunsOnIntoTheOldRelief(t *testing.T) {
	w := drownWorld(t, 3)
	adept := mustAgent(t, w, swimmer(t, w, 400, 300, 1))
	water := w.terrainAt(400, 300)

	if got := w.drownChanceFor(adept, water); got >= water.Drown {
		t.Fatalf("with the relief on, a master faces %v, want under the ground's %v", got, water.Drown)
	}
	w.cfg.DrownUnskilledFactor = 1
	if got := w.drownChanceFor(adept, water); got >= water.Drown {
		t.Fatal("switching off this stage took the old relief with it")
	}
}

// The linear arm is the same field: at a reachable point of one, a mastery
// this world does attain keeps most of the penalty - which is why it is an arm
// and not the default.
func TestTheLinearArmLeavesTheSwimmerPaying(t *testing.T) {
	w := drownWorld(t, 3)
	w.cfg.SkillSwimRelief, w.cfg.DrownSkillFull = 0, 1
	adept := mustAgent(t, w, swimmer(t, w, 400, 300, 0.5))
	water := w.terrainAt(400, 300)

	if got, want := w.drownChanceFor(adept, water), 2*water.Drown; math.Abs(got-want) > 1e-12 {
		t.Fatalf("a body halfway up the linear curve faces %v, want %v", got, want)
	}
}

// What lives in the river is not something the river takes, whatever it knows
// about swimming (stage 63): the same flag, the same one fact.
func TestACreatureOfTheWaterIsNotChargedForNotSwimming(t *testing.T) {
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	cfg.TerrainMap = []string{"...~~...", "...~~...", "...~~..."}
	cfg.DrownUnskilledFactor = 5
	cfg.EnemyKinds = []EnemyKind{{Name: "lurker", Share: 1, Water: true}}
	w := NewWorld(cfg)

	g := genomeOf(50, 50, 50)
	beastID := w.addAgent(Agent{Maturity: 1, X: 400, Y: 300, Vitality: 90,
		Species: SpeciesEnemy, Genome: g})
	beast := mustAgent(t, w, beastID)
	if got := w.drownChanceFor(beast, w.terrainAt(400, 300)); got != 0 {
		t.Fatalf("the river charges what lives in it %v", got)
	}
}

// A body feels the water as IT would face it, and the arm that takes the
// feeling away leaves the water exactly as deadly (stage 34's DrownKnown).
func TestABodyFeelsTheWaterAsItWouldFaceIt(t *testing.T) {
	w := drownWorld(t, 3)
	green := mustAgent(t, w, swimmer(t, w, 400, 300, 0))
	adeptID := swimmer(t, w, 400, 300, 1)
	adept := mustAgent(t, w, adeptID)
	green = mustAgent(t, w, green.ID) // the slice may have moved

	// One at a time: perceive hands back the world's one reusable buffer, so
	// two calls in the same expression both read the second one's answer.
	gp := w.perceive(green).Self.Drown
	ap := w.perceive(adept).Self.Drown
	if !(ap < gp) {
		t.Fatalf("the swimmer reckons %v and the sinker %v: want the sinker's higher", ap, gp)
	}
	if got := w.drownChanceFor(green, w.terrainAt(400, 300)); math.Abs(gp-got) > 1e-12 {
		t.Fatalf("it feels %v and faces %v", gp, got)
	}

	w.cfg.DrownKnown = false
	if got := w.perceive(green).Self.Drown; got != 0 {
		t.Fatalf("with the feeling off it still reckons %v", got)
	}
	if got := w.drownChanceFor(green, w.terrainAt(400, 300)); got <= 0 {
		t.Fatal("switching off the feeling made the water safe")
	}
}

// What a place is believed to do is a property of the place, not of who is
// asking - until the arm says otherwise.
func TestTheBeliefIsAboutThePlaceUnlessTheArmSaysOtherwise(t *testing.T) {
	w := drownWorld(t, 3)
	greenID, adeptID := swimmer(t, w, 100, 300, 0), swimmer(t, w, 100, 300, 1)
	green, adept := mustAgent(t, w, greenID), mustAgent(t, w, adeptID)
	i := w.regionIndexAt(400, 300)
	for _, a := range []*Agent{green, adept} {
		a.regions = make([]regionView, len(w.regions))
		a.regions[i].setSeen(1, 30, w.tick)
		a.regions[i].danger = 0.001
	}

	if gw, aw := w.worthOfRegion(green, i, 1), w.worthOfRegion(adept, i, 1); gw != aw {
		t.Fatalf("by default the two price the same place at %v and %v", gw, aw)
	}
	w.cfg.DrownBeliefPerBody = true
	if gw, aw := w.worthOfRegion(green, i, 1), w.worthOfRegion(adept, i, 1); !(gw < aw) {
		t.Fatalf("with the arm on, the sinker prices the river at %v and the swimmer at %v", gw, aw)
	}
}

// The measurement writes nothing and draws nothing: a run that is asked about
// itself every tick ends up exactly where one that is never asked does.
func TestAskingAboutTheDrowningChangesNothing(t *testing.T) {
	run := func(ask bool) Stats {
		cfg := quietConfig()
		cfg.Seed = 7
		cfg.TerrainMap = []string{"...~~...", "...~~...", "...~~..."}
		cfg.DrownUnskilledFactor = 3
		w := NewWorld(cfg)
		for i := 0; i < 500; i++ {
			w.Step()
			if ask {
				w.Drowning()
			}
		}
		return w.Stats()
	}
	if quiet, asked := run(false), run(true); quiet != asked {
		t.Fatalf("asking changed the world: %+v vs %+v", quiet, asked)
	}
}
