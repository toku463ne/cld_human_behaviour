package engine

import (
	"math"
	"testing"
)

// Tests for growing up and wearing out: the one curve that scales everything
// an agent inherited, the food that moves its young end, the years that move
// its old end, and the childhood that keeps a newborn near a parent.

// growthConfig is a still world with the metabolism running, because growth is
// paid for out of the metabolism.
func growthConfig() Config {
	cfg := testConfig()
	cfg.FoodSpawnRate = 0
	cfg.EnemySpawnTicks = 0 // nothing arrives from off the map to eat the subject
	return cfg
}

// --- the curve --------------------------------------------------------------

// A newborn cannot do what its genome says it can. It grows into it, and what
// it is worth in between is the same number every rule reads.
func TestANewbornExpressesLessThanItInherited(t *testing.T) {
	cfg := growthConfig()
	w := NewWorld(cfg)

	child := &Agent{Genome: genomeOf(80, 80, 80), Maturity: 0}
	adult := &Agent{Genome: genomeOf(80, 80, 80), Maturity: 1}

	if got := child.AgeFactor(&cfg); math.Abs(got-cfg.ChildAbilityShare) > 1e-9 {
		t.Fatalf("a newborn's age factor = %v, want %v", got, cfg.ChildAbilityShare)
	}
	if child.Attack(&cfg) >= adult.Attack(&cfg) {
		t.Fatalf("a newborn hits as hard as an adult: %v against %v",
			child.Attack(&cfg), adult.Attack(&cfg))
	}
	// Everything goes through the one factor, not just the fighting.
	for _, pair := range [][2]float64{
		{child.MaxVitality(&cfg), adult.MaxVitality(&cfg)},
		{child.MaxSpeed(&cfg), adult.MaxSpeed(&cfg)},
		{child.Rationality(&cfg), adult.Rationality(&cfg)},
		{child.Bulk(&cfg), adult.Bulk(&cfg)},
	} {
		if pair[0] >= pair[1] {
			t.Fatalf("a newborn is not smaller than an adult: %v against %v", pair[0], pair[1])
		}
	}
	// But what it will pass on is untouched: age is not inherited.
	if child.Gene(GeneAttack) != adult.Gene(GeneAttack) {
		t.Fatal("the inherited value moved with age")
	}
	_ = w
}

// Growth is bought with food. A well fed child finishes in about the years the
// config says; a starving one does not finish at all.
func TestGrowingUpTakesFoodAndTime(t *testing.T) {
	cfg := growthConfig()
	cfg.HungerRate = 0 // hold the hunger still, so the test is about growth
	w := NewWorld(cfg)

	fed := w.addAgent(Agent{X: 100, Y: 100, Vitality: 60, Hunger: 0, Genome: genomeOf(50, 50, 50)})
	hungry := w.addAgent(Agent{X: 300, Y: 300, Vitality: 60, Hunger: cfg.SatiatedHunger,
		Genome: genomeOf(50, 50, 50)})

	want := int(cfg.ChildhoodYears * float64(cfg.TicksPerYear))
	for i := 0; i < want; i++ {
		w.Step()
	}
	if got := mustAgent(t, w, fed).Maturity; got < 0.99 {
		t.Fatalf("a well fed child is %.2f grown after a full childhood, want 1", got)
	}
	if got := mustAgent(t, w, hungry).Maturity; got != 0 {
		t.Fatalf("a child that never ate grew to %.2f", got)
	}
	if got := w.Stats().Matured; got != 1 {
		t.Fatalf("%d agents finished growing, want 1", got)
	}
}

// Offspring is not on the table until an agent has grown up, however well fed
// and unhurt it is. This is what childhood costs the world: generations.
func TestAChildDoesNotCourt(t *testing.T) {
	cfg := growthConfig()
	w := NewWorld(cfg)

	child := w.addAgent(Agent{X: 200, Y: 200, Sex: Male, Vitality: 100, Hunger: 0,
		Maturity: 0.5, Genome: genomeOf(50, 100, 100)})
	w.addAgent(Agent{X: 214, Y: 200, Sex: Female, Vitality: 100, Hunger: 0,
		Maturity: 1, Genome: genomeOf(50, 50, 50)})

	if mustAgent(t, w, child).CanReproduce(&cfg) {
		t.Fatal("a half grown agent considers offspring")
	}
	wantNotAction(t, w, child, ActCourt, "half grown, with a candidate in front of it")

	mustAgent(t, w, child).Maturity = 1
	if !mustAgent(t, w, child).CanReproduce(&cfg) {
		t.Fatal("a grown, well fed, unhurt agent still will not consider offspring")
	}
}

// The old end of the same curve. It comes down with the years, and it stops at
// the floor rather than running away to nothing.
func TestTheOldDeclineAndThenStopDeclining(t *testing.T) {
	cfg := growthConfig()
	years := func(y float64) *Agent {
		return &Agent{Genome: genomeOf(50, 50, 50), Maturity: 1,
			Age: int(y * float64(cfg.TicksPerYear))}
	}
	prime := years(cfg.SenescenceYears - 1).AgeFactor(&cfg)
	old := years(cfg.SenescenceYears + 5).AgeFactor(&cfg)
	ancient := years(cfg.SenescenceYears + 500).AgeFactor(&cfg)

	if prime != 1 {
		t.Fatalf("an agent in its prime expresses %v of itself, want all of it", prime)
	}
	if !(old < prime) {
		t.Fatalf("five years past its prime it is still at %v", old)
	}
	if math.Abs(ancient-cfg.SenescenceFloor) > 1e-9 {
		t.Fatalf("the decline ran to %v, want it to stop at the floor %v", ancient, cfg.SenescenceFloor)
	}
}

// Both ends off is the world as it was before stage 7d, which is the arm every
// measurement of this stage is compared against.
func TestTheCurveCanBeTurnedOff(t *testing.T) {
	cfg := growthConfig()
	cfg.ChildAbilityShare, cfg.SenescenceRate = 1, 0
	for _, a := range []*Agent{
		{Genome: genomeOf(50, 50, 50), Maturity: 0},
		{Genome: genomeOf(50, 50, 50), Maturity: 1, Age: 100 * cfg.TicksPerYear},
	} {
		if got := a.AgeFactor(&cfg); got != 1 {
			t.Fatalf("age factor = %v with the curve off, want 1", got)
		}
	}
}

// --- being worn down (#33) --------------------------------------------------

// A long spell below the vitality it takes to be alright costs lifespan, but
// only after it has gone on for a while, and the tally resets the moment the
// agent climbs back out.
func TestALongBadSpellCostsLifespan(t *testing.T) {
	cfg := growthConfig()
	cfg.HungerRate, cfg.RegenRate, cfg.StarveRate = 0, 0, 0
	cfg.FrailGraceTicks = 50
	w := NewWorld(cfg)

	id := w.addAgent(Agent{X: 200, Y: 200, Maturity: 1, Hunger: 30,
		Genome: genomeOf(50, 50, 50)})
	// Held still, so that the only thing moving is the lifespan: an agent
	// left to itself would walk about and spend the little vitality it has.
	w.SetController(id, fixedController{Action{Kind: ActRest}})
	a := mustAgent(t, w, id)
	a.Vitality = 0.1 * a.MaxVitality(&cfg)
	start := a.Lifespan

	for i := 0; i < cfg.FrailGraceTicks; i++ {
		w.Step()
	}
	if a = mustAgent(t, w, id); a.Lifespan != start {
		t.Fatalf("lifespan fell to %v inside the grace period", a.Lifespan)
	}
	for i := 0; i < 100; i++ {
		w.Step()
	}
	worn := mustAgent(t, w, id).Lifespan
	if !(worn < start) {
		t.Fatalf("lifespan is still %v after a long bad spell", worn)
	}

	// Back above the line, and the tally starts again from nothing.
	a = mustAgent(t, w, id)
	a.Vitality = a.MaxVitality(&cfg)
	for i := 0; i < cfg.FrailGraceTicks; i++ {
		w.Step()
	}
	if got := mustAgent(t, w, id).Lifespan; got != worn {
		t.Fatalf("lifespan moved to %v while the agent was in good health", got)
	}
}

func TestBeingWornDownCanBeTurnedOff(t *testing.T) {
	cfg := growthConfig()
	cfg.HungerRate, cfg.RegenRate, cfg.StarveRate = 0, 0, 0
	cfg.FrailLifespanRate = 0
	w := NewWorld(cfg)

	id := w.addAgent(Agent{X: 200, Y: 200, Maturity: 1, Hunger: 30, Genome: genomeOf(50, 50, 50)})
	w.SetController(id, fixedController{Action{Kind: ActRest}})
	a := mustAgent(t, w, id)
	a.Vitality = 0.05 * a.MaxVitality(&cfg)
	start := a.Lifespan
	for i := 0; i < 2000; i++ {
		w.Step()
	}
	if got := mustAgent(t, w, id).Lifespan; got != start {
		t.Fatalf("lifespan fell to %v with the rule off", got)
	}
}

// --- childhood --------------------------------------------------------------

// A child that has wandered off turns back. Nothing is fed to it and nothing is
// asked of the parent: it simply does not leave.
func TestAChildKeepsToItsParent(t *testing.T) {
	cfg := growthConfig()
	w := NewWorld(cfg)

	parent := w.addAgent(Agent{X: 200, Y: 200, Maturity: 1, Vitality: 80, Hunger: 10,
		Genome: genomeOf(50, 50, 50)})
	w.SetController(parent, fixedController{Action{Kind: ActRest}})
	child := w.addAgent(Agent{X: 200 + cfg.RearingRadius + 60, Y: 200, Vitality: 40, Hunger: 10,
		Genome: genomeOf(50, 50, 50), GuardianID: parent, RearingTimer: 500})

	before := dist(mustAgent(t, w, child), mustAgent(t, w, parent))
	for i := 0; i < 60; i++ {
		w.Step()
	}
	after := dist(mustAgent(t, w, child), mustAgent(t, w, parent))
	if !(after < before) {
		t.Fatalf("the child is %.1f from its parent, was %.1f", after, before)
	}

	// And once the childhood is over it is on its own.
	c := mustAgent(t, w, child)
	c.RearingTimer = 0
	c.X, c.Y = 200+cfg.RearingRadius+60, 200
	away := dist(c, mustAgent(t, w, parent))
	for i := 0; i < 60; i++ {
		w.Step()
	}
	if got := dist(mustAgent(t, w, child), mustAgent(t, w, parent)); got < away*0.5 {
		t.Fatalf("a grown child is still being pulled back: %.1f from %.1f", got, away)
	}
}

// A newborn is handed a parent to keep to, and the whole arrangement comes off
// with one config field.
func TestBirthSetsUpTheChildhoodAndZeroTurnsItOff(t *testing.T) {
	for _, ticks := range []int{1000, 0} {
		cfg := growthConfig()
		cfg.ChildRearingTicks = ticks
		w, male, female := pairAboutToGiveBirth(t, cfg)
		w.Step()
		child := findChild(t, w, male, female)
		if child.RearingTimer != ticks {
			t.Fatalf("newborn's rearing timer = %d, want %d", child.RearingTimer, ticks)
		}
		if ticks > 0 && child.GuardianID == 0 {
			t.Fatal("newborn has nobody to keep to")
		}
		if child.Maturity != 0 {
			t.Fatalf("newborn starts %v grown", child.Maturity)
		}
	}
}

// A child whose parent has died is on its own from that tick, rather than
// walking back to a corpse.
func TestAnOrphanIsOnItsOwn(t *testing.T) {
	cfg := growthConfig()
	w := NewWorld(cfg)
	parent := w.addAgent(Agent{X: 200, Y: 200, Maturity: 1, Vitality: 80, Genome: genomeOf(50, 50, 50)})
	child := w.addAgent(Agent{X: 400, Y: 200, Vitality: 40, Hunger: 10,
		Genome: genomeOf(50, 50, 50), GuardianID: parent, RearingTimer: 500})

	w.kill(mustAgent(t, w, parent))
	w.Step()
	if c := mustAgent(t, w, child); c.RearingTimer != 0 || c.GuardianID != 0 {
		t.Fatalf("orphan still has guardian %d for %d ticks", c.GuardianID, c.RearingTimer)
	}
}

// A carcass is worth what the body was, not what it was born to become.
func TestAChildLeavesLessMeatThanAnAdult(t *testing.T) {
	cfg := growthConfig()
	w := NewWorld(cfg)
	child := &Agent{Genome: genomeOf(50, 50, 50), Maturity: 0}
	adult := &Agent{Genome: genomeOf(50, 50, 50), Maturity: 1}
	if !(w.meatFrom(child) < w.meatFrom(adult)) {
		t.Fatalf("a newborn leaves %v of meat and an adult %v", w.meatFrom(child), w.meatFrom(adult))
	}
}

func dist(a, b *Agent) float64 { return math.Sqrt(dist2(a.X, a.Y, b.X, b.Y)) }
