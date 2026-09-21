package engine

import (
	"math"
	"testing"
)

// shoveWorld is knockWorld with the passive push off and the chosen one on:
// two bodies at arm's length, nothing else happening, and the only thing that
// can move either of them is a stance somebody picked.
func shoveWorld(t *testing.T, tune func(*Config)) (*World, *Agent, *Agent) {
	t.Helper()
	return knockWorld(t, func(c *Config) {
		c.KnockbackDist = 0
		c.ShovePush = 6
		if tune != nil {
			tune(c)
		}
	})
}

// shoveAt resolves one blow from a at b in the given stance, the way a tick
// does.
func shoveAt(w *World, a, b *Agent, stance Stance) {
	a.Action = Action{Kind: ActAttack, TargetID: b.ID, Effort: 1, Stance: stance}
	w.attacks = append(w.attacks[:0], attack{fromID: a.ID, toID: b.ID, effort: 1, hit: 1})
	w.resolveAttacks()
}

func TestShoveOffByDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ShovePush != 0 {
		t.Fatalf("ShovePush default = %v, want 0: what a push is worth is the map author's business", cfg.ShovePush)
	}
	// And the fourth stance is not even on the list, because pick draws a
	// random number per candidate and a world without the rule has to draw
	// the numbers it always did.
	if got := numStances(&cfg); got != NumStances-1 {
		t.Fatalf("numStances with the rule off = %d, want %d", got, NumStances-1)
	}
	cfg.ShovePush = 5
	if got := numStances(&cfg); got != NumStances {
		t.Fatalf("numStances with the rule on = %d, want %d", got, NumStances)
	}
}

func TestOnlyTheShovingStanceMovesAnybody(t *testing.T) {
	for _, stance := range []Stance{StanceAggressive, StanceGuarded, StanceEvasive} {
		w, a, b := shoveWorld(t, nil)
		x, y := b.X, b.Y
		shoveAt(w, a, b, stance)
		if b.X != x || b.Y != y {
			t.Fatalf("%v moved a body: (%v,%v) -> (%v,%v)", stance, x, y, b.X, b.Y)
		}
		if w.Shoving().Made != 0 {
			t.Fatalf("%v filed a push", stance)
		}
	}

	w, a, b := shoveWorld(t, nil)
	x, y := b.X, b.Y
	shoveAt(w, a, b, StanceShoving)
	if b.X <= x {
		t.Fatalf("shoving stance did not push: x %v -> %v", x, b.X)
	}
	if b.Y != y {
		t.Fatalf("pushed sideways: y %v -> %v", y, b.Y)
	}
	if got := w.Shoving().Made; got != 1 {
		t.Fatalf("Made = %d, want 1", got)
	}
}

func TestShovingGivesUpTheBlow(t *testing.T) {
	// Two identical exchanges, one swinging and one shoving. The shove has to
	// cost the target less vitality and more ground, or the fourth stance is
	// not a trade at all.
	w, a, b := shoveWorld(t, nil)
	before := b.Vitality
	shoveAt(w, a, b, StanceAggressive)
	swung := before - b.Vitality

	w2, a2, b2 := shoveWorld(t, nil)
	before2 := b2.Vitality
	x2 := b2.X
	shoveAt(w2, a2, b2, StanceShoving)
	shoved := before2 - b2.Vitality

	if !(shoved < swung) {
		t.Fatalf("shoving took %v and swinging %v: the push has to give up the blow", shoved, swung)
	}
	if b2.X <= x2 {
		t.Fatalf("shoving bought no ground at all")
	}
}

func TestAGuardedBodyIsMovedLess(t *testing.T) {
	// Defence is one of the two gates the design put on a chosen push, and
	// it belongs to the one being pushed, not to the one pushing.
	push := func(guarding bool) float64 {
		w, a, b := shoveWorld(t, func(c *Config) { c.DefenceCap = 0.5 })
		if guarding {
			b.Action = Action{Kind: ActAttack, TargetID: a.ID, Effort: 1, Stance: StanceGuarded}
		}
		x := b.X
		shoveAt(w, a, b, StanceShoving)
		return b.X - x
	}
	open, guarded := push(false), push(true)
	if !(guarded < open) {
		t.Fatalf("guarding moved %v and standing there %v: the guard has to matter", guarded, open)
	}
	if guarded <= 0 {
		t.Fatalf("guarding stopped the push entirely (%v): it reduces, it does not block", guarded)
	}
}

func TestADodgedShoveMovesNobody(t *testing.T) {
	// The other gate: getting out of the way means the push never happens,
	// for the same reason a dodged blow does no damage.
	w, a, b := shoveWorld(t, func(c *Config) { c.EvasionCap = 1 })
	b.Action = Action{Kind: ActAttack, TargetID: a.ID, Effort: 1, Stance: StanceEvasive}
	x, y := b.X, b.Y
	moved := 0
	for i := 0; i < 40; i++ {
		b.X, b.Y = x, y
		shoveAt(w, a, b, StanceShoving)
		if b.X != x || b.Y != y {
			moved++
		}
	}
	if moved == 40 {
		t.Fatalf("a body dodging was pushed every single time")
	}
	if moved == 0 {
		t.Fatalf("a body dodging was never pushed at all: evasion is a chance, not a wall")
	}
}

func TestAHeavyBodyGivesLessGround(t *testing.T) {
	ground := func(vitalityGene float64) float64 {
		w, a, b := shoveWorld(t, nil)
		b.Genome[GeneVitality] = vitalityGene
		x := b.X
		shoveAt(w, a, b, StanceShoving)
		return b.X - x
	}
	light, heavy := ground(midAbility*0.5), ground(midAbility*1.5)
	if !(heavy < light) {
		t.Fatalf("heavy moved %v and light %v: mass has to matter", heavy, light)
	}
}

func TestAPushThatOpensTheGapIsWhatBothOfThemLearn(t *testing.T) {
	// The belief is the one thing a chosen push teaches, and the event has
	// two doors into it: the one who pushed and the one who went backwards.
	w, a, b := shoveWorld(t, func(c *Config) { c.ShovePush = 400 }) // far past arm's length
	beforeA, beforeB := a.lore.shoveWorks.mean, b.lore.shoveWorks.mean
	shoveAt(w, a, b, StanceShoving)

	if got := w.Shoving().Broke; got != 1 {
		t.Fatalf("Broke = %d after a push well past arm's length, want 1", got)
	}
	if !(a.lore.shoveWorks.mean > beforeA) {
		t.Fatalf("the one who pushed learnt nothing: %v -> %v", beforeA, a.lore.shoveWorks.mean)
	}
	if !(b.lore.shoveWorks.mean > beforeB) {
		t.Fatalf("the one who was pushed learnt nothing: %v -> %v", beforeB, b.lore.shoveWorks.mean)
	}
}

func TestAPushThatChangesNothingTeachesThatToo(t *testing.T) {
	w, a, b := shoveWorld(t, func(c *Config) { c.ShovePush = 0.01 })
	before := a.lore.shoveWorks.mean
	shoveAt(w, a, b, StanceShoving)
	if got := w.Shoving().Held; got != 1 {
		t.Fatalf("Held = %d after a push that left them in reach, want 1", got)
	}
	if !(a.lore.shoveWorks.mean < before) {
		t.Fatalf("a push that opened nothing did not lower the belief: %v -> %v", before, a.lore.shoveWorks.mean)
	}
}

func TestOnlyThePusherLearnsWhenTheWorldSaysSo(t *testing.T) {
	w, a, b := shoveWorld(t, func(c *Config) {
		c.ShovePush, c.ShoveLearnBoth = 400, false
	})
	beforeB := b.lore.shoveWorks.mean
	shoveAt(w, a, b, StanceShoving)
	if b.lore.shoveWorks.mean != beforeB {
		t.Fatalf("the one pushed learnt with ShoveLearnBoth off: %v -> %v", beforeB, b.lore.shoveWorks.mean)
	}
}

func TestTheFrozenBeliefIsFrozen(t *testing.T) {
	// The control arm for "is it the pushing or the knowing about pushing":
	// the push lands exactly as it would, and nothing is learnt from it.
	w, a, b := shoveWorld(t, func(c *Config) {
		c.ShovePush, c.ShoveLearnRate = 400, 0
	})
	beforeA, beforeB := a.lore.shoveWorks.mean, b.lore.shoveWorks.mean
	x := b.X
	shoveAt(w, a, b, StanceShoving)
	if b.X <= x {
		t.Fatalf("the frozen arm stopped the push itself")
	}
	if a.lore.shoveWorks.mean != beforeA || b.lore.shoveWorks.mean != beforeB {
		t.Fatalf("somebody learnt with the belief frozen")
	}
	// And the world still counts what happened, because a control that
	// cannot be compared is not a control.
	if got := w.Shoving().Broke; got != 1 {
		t.Fatalf("Broke = %d in the frozen arm, want 1", got)
	}
}

func TestAShoveIsNeverThrown(t *testing.T) {
	// A stance is something a body does with its weight, and there is no
	// weight behind a stone.
	w, a, b := shoveWorld(t, func(c *Config) { c.Stones, c.KnockbackThrown = 10, true })
	a.Action = Action{Kind: ActAttack, TargetID: b.ID, Effort: 1, Stance: StanceShoving}
	w.attacks = append(w.attacks[:0], attack{fromID: a.ID, toID: b.ID, effort: 1, thrown: true, hit: 1})
	w.resolveAttacks()
	if got := w.Shoving().Made; got != 0 {
		t.Fatalf("a thrown stone filed %d chosen pushes", got)
	}
}

func TestTheGroundBehindIsUnreadWithoutTheRule(t *testing.T) {
	// Nothing is read and nothing is drawn where nobody can push anybody,
	// which is what keeps every world before this one running as it did.
	w, _, _ := shoveWorld(t, func(c *Config) {
		c.ShovePush = 0
		c.TerrainMap = []string{"1111", "1111", "...."}
	})
	if w.readsBehind() {
		t.Fatalf("the ground behind is read with the rule off")
	}
}

func TestTheDropBehindSomebodyIsSeen(t *testing.T) {
	// Two bodies on the top level with the edge just past the one in front:
	// what the looker reads has to be the fall, and nothing at all when it
	// is looking the other way.
	cfg := DefaultConfig()
	cfg.InitialPopulation, cfg.InitialFoodItems, cfg.FoodSpawnRate = 0, 0, 0
	cfg.EnemySpawnTicks = 0
	cfg.ShovePush = 60
	cfg.GroundAheadNoise = 0 // read the ground exactly, so the figure is the figure
	cfg.TerrainMap = []string{
		"1111....",
		"1111....",
		"1111....",
		"1111....",
	}
	w := NewWorld(cfg)
	mid := make([]float64, NumGenes)
	for i := range mid {
		mid[i] = midAbility
	}
	// The map is 8 by 4 over the default world, so the edge of the high
	// ground is halfway across.
	edge := cfg.Width / 2
	a := mustAgent(t, w, w.addAgent(w.newAgent(edge-60, 100, Male, append([]float64(nil), mid...), 0, 1)))
	b := mustAgent(t, w, w.addAgent(w.newAgent(edge-20, 100, Female, append([]float64(nil), mid...), 0, 1)))

	p := w.perceive(a)
	v := viewOf(p, b.ID)
	if v == nil {
		t.Fatal("the other body is not in sight")
	}
	if v.BehindFall <= 0 {
		t.Fatalf("BehindFall = %v looking at a body with the edge behind it", v.BehindFall)
	}
	if want := cfg.KnockbackFall; math.Abs(v.BehindFall-want) > 1e-9 {
		t.Fatalf("BehindFall = %v, want the one level fall %v", v.BehindFall, want)
	}

	// And from the other side there is nothing behind anybody: the drop is a
	// relation between two bodies, not a property of one.
	p = w.perceive(b)
	v = viewOf(p, a.ID)
	if v == nil {
		t.Fatal("the other body is not in sight from the far side")
	}
	if v.BehindFall != 0 {
		t.Fatalf("BehindFall = %v looking inland, want 0", v.BehindFall)
	}
}

func TestTheGroundBehindIsHiddenInTheBlindArm(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InitialPopulation, cfg.InitialFoodItems, cfg.FoodSpawnRate = 0, 0, 0
	cfg.EnemySpawnTicks = 0
	cfg.ShovePush, cfg.ShoveGroundSeen = 60, false
	cfg.GroundAheadNoise = 0
	cfg.TerrainMap = []string{"1111....", "1111....", "1111....", "1111...."}
	w := NewWorld(cfg)
	mid := make([]float64, NumGenes)
	for i := range mid {
		mid[i] = midAbility
	}
	edge := cfg.Width / 2
	a := mustAgent(t, w, w.addAgent(w.newAgent(edge-60, 100, Male, append([]float64(nil), mid...), 0, 1)))
	b := mustAgent(t, w, w.addAgent(w.newAgent(edge-20, 100, Female, append([]float64(nil), mid...), 0, 1)))
	_ = b
	p := w.perceive(a)
	if v := viewOf(p, b.ID); v == nil || v.BehindFall != 0 {
		t.Fatalf("the blind arm read the ground behind: %+v", v)
	}
}

func TestPushingIsWorthMoreToABodyThatBelievesInIt(t *testing.T) {
	// No threshold anywhere says "shove when you are losing". What the
	// formula sees is that a push takes the incoming damage off for a while
	// and takes this body's own blow off too, so the option wins exactly
	// where the exchange is not going to be won and the damage is what
	// matters.
	//
	// Two scorings of the same fight, differing only in what the body
	// believes about pushing. Believing it works has to raise what the
	// shoving stance is worth and leave the other three where they were:
	// nothing else in the formula reads the belief.
	score := func(works float64) (shoving, swinging float64) {
		w, a, b := shoveWorld(t, func(c *Config) { c.ShovePush = 20 })
		a.lore.shoveWorks = belief{mean: works, n: 50}
		// Somebody worth swinging at: hungry, and the other one in reach.
		a.Hunger = 0.8
		b.Vitality = b.MaxVitality(&w.cfg) * 0.3
		tr := decideWithTrace(t, w, a.ID)
		for _, o := range tr.Options {
			if o.Action.Kind != ActAttack || o.Action.TargetID != b.ID {
				continue
			}
			switch o.Action.Stance {
			case StanceShoving:
				shoving = o.Utility.Total()
			case StanceAggressive:
				swinging = o.Utility.Total()
			}
		}
		return shoving, swinging
	}
	lowShove, lowSwing := score(0)
	highShove, highSwing := score(1)

	if lowShove == 0 || highShove == 0 {
		t.Fatalf("the shoving stance was never scored (%v, %v)", lowShove, highShove)
	}
	if !(highShove > lowShove) {
		t.Fatalf("believing in the push did not raise what it is worth: %v -> %v", lowShove, highShove)
	}
	if highSwing != lowSwing {
		t.Fatalf("the belief moved what swinging is worth (%v -> %v): nothing else should read it", lowSwing, highSwing)
	}
}
