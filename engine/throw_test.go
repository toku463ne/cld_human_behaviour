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
