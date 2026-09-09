package engine

import (
	"math"
	"testing"
)

// throwConfig is a still world with broken ground, stones and throwing.
func throwConfig() Config {
	cfg := stoneConfig()
	cfg.Stones = 0 // the tests put them where they want them
	cfg.Throwing = true
	cfg.CarryCapacity = 1
	return cfg
}

// armed puts a body somewhere with a stone in its hand.
func armed(t *testing.T, w *World, x, y float64) *Agent {
	t.Helper()
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: x, Y: y, Vitality: 90,
		Genome: filledGenome(60)}))
	id := w.putFood(Food{X: x, Y: y, Kind: FoodStone})
	w.take(a, id)
	if !a.canThrow(&w.cfg) {
		t.Fatal("the body was given a stone and cannot throw one")
	}
	return a
}

// A world with throwing off never sees one, whatever is lying about.
func TestWithoutTheRuleNobodyThrowsAnything(t *testing.T) {
	cfg := throwConfig()
	cfg.Throwing = false
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Genome: filledGenome(60)}))
	id := w.putFood(Food{X: 100, Y: 100, Kind: FoodStone})
	w.take(a, id)
	if a.canThrow(&cfg) {
		t.Fatal("a body in a world with no throwing in it reckons it can throw")
	}
	if got := w.throwRange(); got != 0 {
		t.Fatalf("a stone carries %v in a world with no throwing", got)
	}
}

// An empty hand is not an option: a body with nothing to throw is never
// offered the chance, which is the line stage 25 drew between what a body
// cannot do and what it would rather not.
func TestAnEmptyHandIsNeverOfferedTheThrow(t *testing.T) {
	cfg := throwConfig()
	w := NewWorld(cfg)
	thrower := armed(t, w, 100, 100)
	empty := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 300, Vitality: 90,
		Genome: filledGenome(60)}))
	target := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 140, Y: 100, Vitality: 90,
		Genome: filledGenome(60)}))
	_ = target

	if got := w.perceive(thrower); !hasThrowOption(got) {
		t.Fatal("an armed body sees nothing to throw at")
	}
	if got := w.perceive(empty); hasThrowOption(got) {
		t.Fatal("a body with an empty hand was offered a throw")
	}
}

func hasThrowOption(p *Perception) bool {
	for i := range p.Others {
		if p.Others[i].ThrowHit > 0 {
			return true
		}
	}
	return false
}

// The stone goes: out of the hand, through the air, and onto the ground by
// whoever it was aimed at. The world's count of them does not change.
func TestAThrownStoneLeavesTheHandAndLandsByTheTarget(t *testing.T) {
	cfg := throwConfig()
	cfg.ThrowHit = 1
	w := NewWorld(cfg)
	thrower := armed(t, w, 100, 100)
	target := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 140, Y: 100, Vitality: 90,
		Genome: filledGenome(60)}))

	before := w.countKind(FoodStone)
	thrower.Action = Action{Kind: ActThrow, TargetID: target.ID, Effort: 1}
	w.perform(thrower)
	w.resolveAttacks()

	if thrower.CarriedCount() != 0 {
		t.Fatal("the stone is still in the hand that threw it")
	}
	if got := w.countKind(FoodStone); got != before {
		t.Fatalf("the world holds %d stones, it held %d", got, before)
	}
	var landed *Food
	for i := range w.foods {
		if w.foods[i].Kind == FoodStone {
			landed = &w.foods[i]
		}
	}
	if landed == nil {
		t.Fatal("the stone is nowhere")
	}
	if math.Hypot(landed.X-target.X, landed.Y-target.Y) > 12 {
		t.Fatalf("the stone landed at %.0f,%.0f and the target is at %.0f,%.0f",
			landed.X, landed.Y, target.X, target.Y)
	}
	if target.Vitality >= 90 {
		t.Fatal("a stone that landed did nothing")
	}
	// And the one hit knows who threw it, which is what makes an answer
	// possible at all.
	if target.attackerID != thrower.ID {
		t.Fatalf("the target reckons #%d hit it", target.attackerID)
	}
}

// The range is longer than an arm and shorter than sight. Too far means
// walking, and out of sight means it is not a target at all.
func TestTheRangeIsLongerThanAnArmAndShorterThanSight(t *testing.T) {
	cfg := throwConfig()
	w := NewWorld(cfg)
	if r := w.throwRange(); r <= cfg.CombatRadius || r > cfg.PerceptionRadius {
		t.Fatalf("a stone carries %v, an arm reaches %v and sight is %v",
			r, cfg.CombatRadius, cfg.PerceptionRadius)
	}

	thrower := armed(t, w, 100, 100)
	far := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100 + w.throwRange() + 30, Y: 100,
		Vitality: 90, Genome: filledGenome(60)}))
	thrower.Action = Action{Kind: ActThrow, TargetID: far.ID, Effort: 1}
	x := thrower.X
	w.perform(thrower)
	if thrower.CarriedCount() == 0 {
		t.Fatal("the stone was thrown at something out of range")
	}
	if thrower.X == x {
		t.Fatal("the body neither threw nor walked")
	}
}

// A stone that goes wide leaves nobody off balance. The reason a missed swing
// does (stage 24) is about being close enough to be caught out, and a throw is
// not.
func TestAThrownStoneThatMissesLeavesNoOpening(t *testing.T) {
	cfg := throwConfig()
	cfg.ThrowHit = 0 // nothing ever lands
	cfg.OpeningTicks = 5
	w := NewWorld(cfg)
	thrower := armed(t, w, 100, 100)
	target := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 140, Y: 100, Vitality: 90,
		Genome: filledGenome(60)}))

	thrower.Action = Action{Kind: ActThrow, TargetID: target.ID, Effort: 1}
	w.perform(thrower)
	w.resolveAttacks()

	if thrower.openUntil > w.tick {
		t.Fatal("a body that threw wide is off balance for it")
	}
	if target.Vitality != 90 {
		t.Fatal("a stone that missed still hurt somebody")
	}
}

// What turns a stone aside is what turns a fist aside: the same guard, the
// same footwork, the same three genes.
func TestAStoneIsTurnedAsideByTheOrdinaryGuard(t *testing.T) {
	cfg := throwConfig()
	cfg.ThrowHit = 1
	cfg.EvasionCap = 0 // this test is about the guard, not the dodge
	w := NewWorld(cfg)

	hurt := func(defence float64) float64 {
		thrower := armed(t, w, 100, 100)
		g := filledGenome(60)
		g[GeneDefence] = defence
		target := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 140, Y: 100, Vitality: 90,
			Genome: g, Action: Action{Kind: ActAttack, Stance: StanceGuarded, Effort: 1}}))
		thrower.Action = Action{Kind: ActThrow, TargetID: target.ID, Effort: 1}
		w.perform(thrower)
		w.resolveAttacks()
		return 90 - target.Vitality
	}
	soft, hard := hurt(10), hurt(100)
	if hard >= soft {
		t.Fatalf("a well guarded body took %v from a stone and a soft one %v", hard, soft)
	}
}

// What knowing how to throw buys, and what it deliberately does not (stage
// 47). It takes back some of the accuracy distance costs, and nothing else:
// the damage is the attack gene's and the dodging is the target's.
func TestKnowingHowToThrowBuysAccuracyAtDistanceAndNothingElse(t *testing.T) {
	cfg := throwConfig()
	w := NewWorld(cfg)
	green := armed(t, w, 100, 100)
	adept := armed(t, w, 300, 300)
	adept.hintSlots++
	w.learnSkill(adept, SkillThrow, 1)

	near := cfg.CombatRadius
	far := w.throwRange()
	if got, want := w.throwHit(adept, near), w.throwHit(green, near); math.Abs(got-want) > 1e-9 {
		t.Fatalf("at arm's length a good thrower lands %v and a green one %v", got, want)
	}
	if w.throwHit(adept, far) <= w.throwHit(green, far) {
		t.Fatalf("at the far end a good thrower lands %v and a green one %v",
			w.throwHit(adept, far), w.throwHit(green, far))
	}
	// The stone itself is unchanged: the same body throwing it does the same
	// damage whether or not it has learned anything.
	if got, want := w.throwDamage(adept, 1), w.throwDamage(green, 1); got != want {
		t.Fatalf("a good thrower's stone does %v and a green one's %v", got, want)
	}
}

// With the relief off, the skill is learned, takes the room and buys nothing -
// which is the arm the stage is read against.
func TestAThrowingSkillWorthNothingChangesNoChances(t *testing.T) {
	cfg := throwConfig()
	cfg.SkillThrowRelief = 0
	w := NewWorld(cfg)
	green := armed(t, w, 100, 100)
	adept := armed(t, w, 300, 300)
	adept.hintSlots++
	w.learnSkill(adept, SkillThrow, 1)

	far := w.throwRange()
	if got, want := w.throwHit(adept, far), w.throwHit(green, far); got != want {
		t.Fatalf("with the relief off a good thrower lands %v and a green one %v", got, want)
	}
}

// Aim is capped by how well a body reads the world, never by how hard it hits:
// the gene that decides what the stone does on arrival must not also decide
// whether it arrives (#71).
func TestAimIsCappedByRationalityAndNotByPower(t *testing.T) {
	cfg := throwConfig()
	w := NewWorld(cfg)
	clear := genomeOf(10, 100, 50)
	strong := genomeOf(100, 10, 50)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Genome: clear}))
	b := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 300, Y: 300, Genome: strong}))
	for _, x := range []*Agent{a, b} {
		x.hintSlots++
		w.learnSkill(x, SkillThrow, 1)
	}
	if a.skillAt(&cfg, SkillThrow) <= b.skillAt(&cfg, SkillThrow) {
		t.Fatal("the strong body aims as well as the clear-headed one")
	}
}

// Where a body learns it is where the stones are, and that is read off the
// stones rather than off the ground they lie on: the broken country already
// seeds knowing how to cross it (stage 38a).
func TestThrowingIsLearnedWhereTheStonesAreAndNotWhereTheRoughIs(t *testing.T) {
	cfg := throwConfig()
	cfg.Stones, cfg.SkillBirthplace = 24, 1
	w := NewWorld(cfg)
	// Take every stone out of the world and put them all in one region.
	for i := 0; i < len(w.foods); {
		if w.foods[i].Kind == FoodStone {
			w.removeFoodByID(w.foods[i].ID)
			continue
		}
		i++
	}
	minX, minY, maxX, maxY := w.regionBounds(0)
	for i := 0; i < 24; i++ {
		w.putFood(Food{X: (minX + maxX) / 2, Y: (minY + maxY) / 2, Kind: FoodStone})
	}

	if born := w.skillFromBirthplace(SkillThrow, (minX+maxX)/2, (minY+maxY)/2); born <= 0 {
		t.Fatalf("a body born where every stone is knows %v about throwing", born)
	}
	// Somewhere else on the same broken ground: no stones there, so nothing
	// to learn about throwing - and the ground itself still teaches crossing.
	x, y := cfg.Width*0.45, cfg.Height*0.9
	if w.terrainAt(x, y).Kind != GroundRough {
		t.Fatal("this test needs a second patch of broken ground")
	}
	if born := w.skillFromBirthplace(SkillThrow, x, y); born != 0 {
		t.Fatalf("a body born where there are no stones knows %v about throwing", born)
	}
	if born := w.skillFromBirthplace(SkillRough, x, y); born <= 0 {
		t.Fatal("the broken country stopped teaching anybody to cross it")
	}
}
