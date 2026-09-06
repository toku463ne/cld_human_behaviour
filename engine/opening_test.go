package engine

import "testing"

// Stage 24: a blow that finds nothing leaves the one who threw it off balance.
// It is the first rule in the world that prices missing.

// evasiveGenome is a body that always gets out of the way: everything on the
// evasion gene and on being quick, so that the chance clamps to the cap and the
// test does not depend on the roll.
func evasiveGenome() []float64 {
	g := genomeOf(midAbility, midAbility, midAbility)
	g[GeneEvasion] = MaxAbility
	g[GeneSpeed] = MaxAbility
	return g
}

func TestAMissLeavesTheSwingerOpen(t *testing.T) {
	cfg := quietConfig()
	cfg.EvasionCap = 1 // every blow at this one misses
	w := NewWorld(cfg)

	swinger := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 200, Hunger: 40,
		Genome: genomeOf(90, 100, 100)})
	dodger := w.addAgent(Agent{Maturity: 1,
		X: 205, Y: 200, Sex: Male, Vitality: 200, Hunger: 40,
		Genome: evasiveGenome()})
	w.SetController(swinger, fixedController{Action{Kind: ActAttack, TargetID: dodger, Effort: 1}})
	w.SetController(dodger, fixedController{
		Action{Kind: ActAttack, TargetID: swinger, Effort: 1, Stance: StanceEvasive}})

	before := w.Stats().Deaths
	w.Step()
	w.Step()

	a := mustAgent(t, w, swinger)
	if a.openUntil <= w.Tick() {
		t.Fatalf("swung and missed, and is open until tick %d at tick %d", a.openUntil, w.Tick())
	}
	if got := a.composure(&cfg, w.Tick()); got != cfg.OpeningGuard {
		t.Fatalf("composure while open = %v, want %v", got, cfg.OpeningGuard)
	}
	// And it wears off. Asked about a later tick rather than stepping the
	// world there, because the fight is still going and every tick of it
	// misses again.
	if got := a.composure(&cfg, a.openUntil); got != 1 {
		t.Fatalf("composure once the opening has run out = %v, want 1", got)
	}
	_ = before
}

// Switched off, nothing about a miss is remembered. This is what makes the two
// worlds a pair: off, the rule draws nothing and touches nothing.
func TestNoOpeningWhenSwitchedOff(t *testing.T) {
	cfg := quietConfig()
	cfg.EvasionCap = 1
	cfg.OpeningTicks = 0
	w := NewWorld(cfg)

	swinger := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 200, Hunger: 40,
		Genome: genomeOf(90, 100, 100)})
	dodger := w.addAgent(Agent{Maturity: 1,
		X: 205, Y: 200, Sex: Male, Vitality: 200, Hunger: 40,
		Genome: evasiveGenome()})
	w.SetController(swinger, fixedController{Action{Kind: ActAttack, TargetID: dodger, Effort: 1}})
	w.SetController(dodger, fixedController{
		Action{Kind: ActAttack, TargetID: swinger, Effort: 1, Stance: StanceEvasive}})

	for i := 0; i < 5; i++ {
		w.Step()
	}
	if a := mustAgent(t, w, swinger); a.openUntil != 0 {
		t.Fatalf("open until tick %d with the rule switched off", a.openUntil)
	}
	if got := mustAgent(t, w, swinger).composure(&cfg, w.Tick()); got != 1 {
		t.Fatalf("composure = %v with the rule switched off, want 1", got)
	}
}

// Being off balance costs the guard, not the skin: the same blow lands for more
// because less of it is turned aside.
func TestAnOpenAgentGuardsForLess(t *testing.T) {
	lost := func(open bool) float64 {
		cfg := quietConfig()
		cfg.EvasionCap = 0 // no roll, so the two runs differ in one thing only
		w := NewWorld(cfg)
		hitter := w.addAgent(Agent{Maturity: 1,
			X: 200, Y: 200, Sex: Male, Vitality: 200, Hunger: 40,
			Genome: genomeOf(90, 100, 100)})
		guard := w.addAgent(Agent{Maturity: 1,
			X: 205, Y: 200, Sex: Male, Vitality: 200, Hunger: 40,
			Genome: genomeOf(midAbility, midAbility, midAbility)})
		w.SetController(hitter, fixedController{Action{Kind: ActAttack, TargetID: guard, Effort: 1}})
		w.SetController(guard, fixedController{
			Action{Kind: ActAttack, TargetID: hitter, Effort: 1, Stance: StanceGuarded}})
		w.Step()
		if open {
			mustAgent(t, w, guard).openUntil = w.Tick() + cfg.OpeningTicks
		}
		was := mustAgent(t, w, guard).Vitality
		w.Step()
		return was - mustAgent(t, w, guard).Vitality
	}

	composed, open := lost(false), lost(true)
	if open <= composed {
		t.Fatalf("an open guard lost %v to the same blow, a composed one %v", open, composed)
	}
}

// --- high ground (stage 30) --------------------------------------------------

// coverConfig puts a step in the middle of the world: level ground on the
// west, one level up on the east, with a ramp between so a body can get there.
func coverConfig() Config {
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	cfg.HighGroundCover = 0.3
	cfg.EvasionCap = 0.6
	cfg.TerrainMap = []string{
		"....A111",
		"....A111",
		"....A111",
	}
	return cfg
}

// A body a level above the one swinging at it is harder to hit; the same two
// bodies on the same level are not.
func TestHighGroundIsHarderToHit(t *testing.T) {
	cfg := coverConfig()
	w := NewWorld(cfg)
	low := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 300, Y: 300, Vitality: 90,
		Genome: genomeOf(50, 50, 50)}))
	high := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 700, Y: 300, Vitality: 90,
		Genome: genomeOf(50, 50, 50)}))
	level := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 200, Y: 300, Vitality: 90,
		Genome: genomeOf(50, 50, 50)}))

	if got := w.cover(high, low); got != cfg.HighGroundCover {
		t.Fatalf("the one up the bank gets %v, want %v", got, cfg.HighGroundCover)
	}
	if got := w.cover(low, high); got != 0 {
		t.Fatalf("the one at the bottom gets %v of cover, want none", got)
	}
	if got := w.cover(level, low); got != 0 {
		t.Fatalf("two bodies on the same ground: %v, want none", got)
	}
	// Two levels up is the same edge as one: an edge, not a slope.
	high.X = 700
	if got := w.cover(high, low); got != cfg.HighGroundCover {
		t.Fatalf("cover from two levels up is %v, want the same %v", got, cfg.HighGroundCover)
	}
}

// And it shows in the fighting: the same blows land less often uphill.
func TestBlowsUphillLandLessOften(t *testing.T) {
	hits := func(coverOn bool) int {
		cfg := coverConfig()
		if !coverOn {
			cfg.HighGroundCover = 0
		}
		cfg.Seed = 7
		w := NewWorld(cfg)
		// The one swinging is on the flat (west of the ramp), the one being
		// swung at is up the bank. resolveAttacks does not care how far apart
		// they are - the reach is checked where the action is taken up.
		low := w.addAgent(Agent{Maturity: 1, X: 300, Y: 300, Vitality: 1e6, Sex: Male,
			Genome: genomeOf(50, 50, 50)})
		high := w.addAgent(Agent{Maturity: 1, X: 700, Y: 300, Vitality: 1e6, Sex: Male,
			Genome: genomeOf(50, 50, 50)})
		// Evading is what the stance decides, and the stance rides on the
		// action: a body has a guard up only while it is fighting.
		// A stance rides on an action: a body that is not fighting is neither
		// swinging nor guarding, so both of them need one.
		mustAgent(t, w, high).Action = Action{Kind: ActAttack, TargetID: low, Effort: 1, Stance: StanceEvasive}
		mustAgent(t, w, low).Action = Action{Kind: ActAttack, TargetID: high, Effort: 1, Stance: StanceAggressive}
		landed := 0
		for i := 0; i < 400; i++ {
			before := mustAgent(t, w, high).Vitality
			w.attacks = w.attacks[:0]
			w.attacks = append(w.attacks, attack{fromID: low, toID: high, effort: 1})
			w.resolveAttacks()
			if mustAgent(t, w, high).Vitality < before {
				landed++
			}
			w.tick++
		}
		return landed
	}
	with, without := hits(true), hits(false)
	if !(with < without) {
		t.Fatalf("%d of 400 blows landed uphill against %d on the level: want fewer uphill",
			with, without)
	}
}

// The agent knows the ground is helping: what it expects the next few ticks to
// cost is lower when it is being attacked from below.
func TestABodyOnHighGroundExpectsLessDamage(t *testing.T) {
	cfg := coverConfig()
	w := NewWorld(cfg)
	low := w.addAgent(Agent{Maturity: 1, X: 300, Y: 300, Vitality: 90, Genome: genomeOf(50, 50, 50)})
	high := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 700, Y: 300, Vitality: 90,
		Genome: genomeOf(80, 50, 50)}))
	high.Action = Action{Kind: ActAttack, TargetID: low, Effort: 1, Stance: StanceEvasive}
	high.attackerID = low

	p := w.perceive(high)
	if !p.Self.Covered {
		t.Fatal("does not know it is being come at from below")
	}
	var c AIController
	c.survey(p)
	covered := c.incomingDmg

	// The same fight on the level.
	high.X = 200
	p = w.perceive(high)
	if p.Self.Covered {
		t.Fatal("reckons the ground is helping when both are on it")
	}
	c.survey(p)
	if !(covered < c.incomingDmg) {
		t.Fatalf("expects %v uphill and %v on the level: want less uphill", covered, c.incomingDmg)
	}
}

// A world with no map never has anybody above anybody, so the rule is
// invisible in it whatever the figure is set to.
func TestCoverDoesNothingInAFlatWorld(t *testing.T) {
	cfg := quietConfig()
	cfg.HighGroundCover = 0.3
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90, Genome: genomeOf(50, 50, 50)}))
	b := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 700, Y: 500, Vitality: 90, Genome: genomeOf(50, 50, 50)}))
	if got := w.cover(a, b); got != 0 {
		t.Fatalf("cover in a flat world is %v", got)
	}
}
