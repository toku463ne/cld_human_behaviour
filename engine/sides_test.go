package engine

import "testing"

// Whose side a body is on (TODO 14, #138). Every rule here is off by default,
// so each test switches on the one it is about and nothing else: what is being
// checked is that the rule does what it says, and that the world without it is
// the world that was there before.

// bestAttackScore is what the controller made of swinging at this one: the
// best of the three stances, out of the options it has just scored. It reads
// the scores rather than the choice, because what these tests are about is the
// comparison and not which side of it happened to win.
func bestAttackScore(t *testing.T, c *AIController, target int) float64 {
	t.Helper()
	best, found := 0.0, false
	for i := range c.opts {
		o := &c.opts[i]
		if o.action.Kind != ActAttack || o.action.TargetID != target {
			continue
		}
		if !found || o.util > best {
			best, found = o.util, true
		}
	}
	if !found {
		t.Fatalf("no attack on %d was scored at all", target)
	}
	return best
}

// sidesConfig is a still world with the metabolism stopped, so that affinity
// is the only thing moving.
func sidesConfig() Config {
	cfg := quietConfig()
	cfg.AffinityDecayPerTick = 0
	return cfg
}

// --- (a) affinity may go below nought ---------------------------------------

func TestAffinityRefusesToGoNegativeUnlessAsked(t *testing.T) {
	w := NewWorld(sidesConfig())
	a := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Genome: genomeOf(50, 50, 50)})
	b := w.addAgent(Agent{Maturity: 1, X: 110, Y: 100, Genome: genomeOf(50, 50, 50)})

	w.rememberAffinity(mustAgent(t, w, a), b, -5)
	if op := mustAgent(t, w, a).opinion(b); op != nil {
		t.Fatalf("a loss was recorded with AffinityNegative off: %+v", op)
	}

	w.cfg.AffinityNegative = true
	w.rememberAffinity(mustAgent(t, w, a), b, -5)
	op := mustAgent(t, w, a).opinion(b)
	if op == nil || op.Affinity != -5 {
		t.Fatalf("affinity = %+v, want -5", op)
	}
}

// A record that has gone sour is a record that matters, so it is not the one a
// full memory gives up first. Without the absolute value, (vi) would undo
// itself: the body would forget the one that robbed it as fast as it noticed.
func TestFullMemoryKeepsTheOneItHasFallenOutWith(t *testing.T) {
	cfg := sidesConfig()
	cfg.AffinityNegative = true
	w := NewWorld(cfg)
	a := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Genome: genomeOf(50, 50, 50)})
	subject := mustAgent(t, w, a)

	enemy := w.addAgent(Agent{Maturity: 1, X: 110, Y: 100, Genome: genomeOf(50, 50, 50)})
	nobody := w.addAgent(Agent{Maturity: 1, X: 120, Y: 100, Genome: genomeOf(50, 50, 50)})
	w.rememberAffinity(subject, enemy, -9)
	w.recordOpinion(subject, nobody) // met once, came to nothing

	worst, found := w.weakestOpinion(subject, true)
	if !found || worst != nobody {
		t.Fatalf("a full memory would drop %d, want the one that came to nothing (%d)", worst, nobody)
	}
}

// The way back is not closed. Clamping the bottom of the scale at nought would
// say that a gift to somebody you have fallen out with buys no trust at all.
func TestGiftToSomebodyDislikedStillBuysSomething(t *testing.T) {
	cfg := sidesConfig()
	if got := trustBought(&cfg, -10, 6); got != 0 {
		t.Fatalf("with negatives off, trustBought(-10, 6) = %v, want 0", got)
	}
	cfg.AffinityNegative = true
	if got := trustBought(&cfg, -10, 6); got <= 0 {
		t.Fatalf("with negatives on, trustBought(-10, 6) = %v, want > 0", got)
	}
}

// --- (i) swinging at somebody costs their goodwill --------------------------

// The floor stays at nought whichever way (a) is set: a body that is already
// disliked has no goodwill left to lose, so swinging at it costs what swinging
// at a stranger costs. That asymmetry is the whole rule - anything else is a
// flat tax on fighting, which AttackCost already is.
func TestFightingCostsGoodwillOnlyWhereThereIsSome(t *testing.T) {
	cfg := sidesConfig()
	cfg.AffinityNegative = true
	if got := trustLost(&cfg, -10, 6); got != 0 {
		t.Fatalf("trustLost(-10, 6) = %v, want 0: there is nothing left to lose", got)
	}
	if got := trustLost(&cfg, 0, 6); got != 0 {
		t.Fatalf("trustLost(0, 6) = %v, want 0: a stranger's goodwill is not held", got)
	}
	friend := trustLost(&cfg, 12, 6)
	if friend <= 0 {
		t.Fatalf("trustLost(12, 6) = %v, want > 0", friend)
	}
	// And it saturates the same way buying does, so the two are mirrors.
	if back := trustBought(&cfg, 6, 6); back != friend {
		t.Fatalf("trustLost(12, 6) = %v but trustBought(6, 6) = %v: they should mirror", friend, back)
	}
}

// The same fight, scored twice: once against a stranger and once against
// somebody the body is fond of. Only the goodwill differs, so only the goodwill
// can explain the gap.
func TestAFightWithAFriendScoresLower(t *testing.T) {
	score := func(affinity, cost float64) float64 {
		cfg := sidesConfig()
		cfg.FightTrustCost = cost
		w := NewWorld(cfg)
		subject := w.addAgent(Agent{Maturity: 1,
			X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 60,
			Genome: genomeOf(60, 100, 100)})
		other := w.addAgent(Agent{Maturity: 1,
			X: 210, Y: 200, Sex: Male, Vitality: 80, Hunger: 0,
			Genome: genomeOf(40, 0, 0)})
		if affinity > 0 {
			w.rememberAffinity(mustAgent(t, w, subject), other, affinity)
		}
		c := &AIController{}
		c.Decide(w.perceive(mustAgent(t, w, subject)))
		return bestAttackScore(t, c, other)
	}

	stranger := score(0, 6)
	friend := score(12, 6)
	if friend >= stranger {
		t.Fatalf("fighting a friend scored %.4f and a stranger %.4f: the goodwill cost did nothing", friend, stranger)
	}
	// And with the rule off the two are the same fight, which is the world
	// as it was before this.
	if a, b := score(0, 0), score(12, 0); a != b {
		t.Fatalf("with FightTrustCost 0 the friend scored %.4f and the stranger %.4f", b, a)
	}
}

// --- (vi) being beaten to your meal -----------------------------------------

func TestBeingBeatenToAMealCostsTheTakerGoodwill(t *testing.T) {
	cfg := sidesConfig()
	cfg.AffinityNegative = true
	cfg.AffinitySnatched = 6
	w := NewWorld(cfg)

	loser := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Hunger: 50, Genome: genomeOf(50, 50, 50)})
	taker := w.addAgent(Agent{Maturity: 1, X: 120, Y: 100, Hunger: 50, Genome: genomeOf(50, 50, 50)})
	bystander := w.addAgent(Agent{Maturity: 1, X: 140, Y: 100, Hunger: 50, Genome: genomeOf(50, 50, 50)})
	food := w.addFood(130, 100)

	// The loser had chosen that item and was on its way; the bystander is
	// doing something else entirely.
	mustAgent(t, w, loser).Action = Action{Kind: ActEat, TargetID: food}
	mustAgent(t, w, bystander).Action = Action{Kind: ActMove}

	w.eat(mustAgent(t, w, taker), food)

	op := mustAgent(t, w, loser).opinion(taker)
	if op == nil || op.Affinity != -6 {
		t.Fatalf("the one that lost the meal thinks %+v of the taker, want -6", op)
	}
	if by := mustAgent(t, w, bystander).opinion(taker); by != nil {
		t.Fatalf("somebody who was not going for it took offence: %+v", by)
	}
	if got := w.Stats().Snatched; got != 1 {
		t.Fatalf("Snatched = %d, want 1", got)
	}
}

// Counting happens whether or not the rule is on: how often a thing could fire
// is the ceiling on what it can explain (stage 24).
func TestSnatchesAreCountedWithTheRuleOff(t *testing.T) {
	w := NewWorld(sidesConfig())
	loser := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Hunger: 50, Genome: genomeOf(50, 50, 50)})
	taker := w.addAgent(Agent{Maturity: 1, X: 120, Y: 100, Hunger: 50, Genome: genomeOf(50, 50, 50)})
	food := w.addFood(130, 100)
	mustAgent(t, w, loser).Action = Action{Kind: ActEat, TargetID: food}

	w.eat(mustAgent(t, w, taker), food)

	if got := w.Stats().Snatched; got != 1 {
		t.Fatalf("Snatched = %d, want 1", got)
	}
	if op := mustAgent(t, w, loser).opinion(taker); op != nil {
		t.Fatalf("the rule is off but an opinion moved: %+v", op)
	}
}

// --- (iii) taking somebody's side -------------------------------------------

func TestTakingSomebodysSidePaysBoth(t *testing.T) {
	cfg := sidesConfig()
	cfg.AffinityAlly = 6
	w := NewWorld(cfg)

	quarry := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Genome: genomeOf(50, 50, 50)})
	first := w.addAgent(Agent{Maturity: 1, X: 205, Y: 200, Genome: genomeOf(50, 50, 50)})
	joiner := w.addAgent(Agent{Maturity: 1, X: 195, Y: 200, Genome: genomeOf(50, 50, 50)})
	elsewhere := w.addAgent(Agent{Maturity: 1, X: 300, Y: 300, Genome: genomeOf(50, 50, 50)})

	mustAgent(t, w, first).Action = Action{Kind: ActAttack, TargetID: quarry}
	mustAgent(t, w, elsewhere).Action = Action{Kind: ActAttack, TargetID: elsewhere}
	mustAgent(t, w, joiner).Action = Action{Kind: ActAttack, TargetID: quarry}

	w.paySides(mustAgent(t, w, joiner))

	for _, pair := range [][2]int{{joiner, first}, {first, joiner}} {
		op := mustAgent(t, w, pair[0]).opinion(pair[1])
		if op == nil || op.Affinity != 6 {
			t.Fatalf("%d thinks %+v of %d, want 6 both ways", pair[0], op, pair[1])
		}
	}
	if op := mustAgent(t, w, joiner).opinion(quarry); op != nil && op.Affinity > 0 {
		t.Fatalf("the one being fought was thanked for it: %+v", op)
	}
	if got := w.Stats().SidesTaken; got != 1 {
		t.Fatalf("SidesTaken = %d, want 1", got)
	}
}

// --- (iv) and the reason for it ---------------------------------------------

// Going in beside a friend is worth the goodwill it buys. The same fight is
// scored with and without the friend already swinging, and nothing else
// differs - the friend is in sight either way.
func TestJoiningAFriendIsWorthTheGoodwillItBuys(t *testing.T) {
	score := func(declared bool, ally float64) float64 {
		cfg := sidesConfig()
		cfg.AffinityAlly = ally
		w := NewWorld(cfg)
		subject := w.addAgent(Agent{Maturity: 1,
			X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 40,
			Genome: genomeOf(60, 100, 100)})
		quarry := w.addAgent(Agent{Maturity: 1,
			X: 215, Y: 200, Sex: Male, Vitality: 80, Hunger: 0,
			Genome: genomeOf(40, 0, 0)})
		friend := w.addAgent(Agent{Maturity: 1,
			X: 205, Y: 200, Sex: Male, Vitality: 80, Hunger: 0,
			Genome: genomeOf(40, 0, 0)})
		w.rememberAffinity(mustAgent(t, w, subject), friend, 10)
		if declared {
			mustAgent(t, w, friend).Action = Action{Kind: ActAttack, TargetID: quarry}
		} else {
			mustAgent(t, w, friend).Action = Action{Kind: ActRest}
		}
		c := &AIController{}
		c.Decide(w.perceive(mustAgent(t, w, subject)))
		return bestAttackScore(t, c, quarry)
	}

	if with, without := score(true, 6), score(false, 6); with <= without {
		t.Fatalf("joining scored %.4f and going in alone %.4f: the goodwill was not counted", with, without)
	}
	// With the figure at nought, what is left is the help the friend gives to
	// the odds, which stage 32 already had.
	base := score(false, 0)
	if joined := score(true, 0); joined <= base {
		t.Logf("with AffinityAlly 0, joining still scores higher (%.4f vs %.4f) - that is stage 32's help", joined, base)
	}
}

// And the situation the rules are about is counted whether or not any of them
// are on.
func TestSeeingAFriendFightIsCounted(t *testing.T) {
	w := NewWorld(sidesConfig())
	subject := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Vitality: 80, Genome: genomeOf(50, 100, 100)})
	friend := w.addAgent(Agent{Maturity: 1, X: 205, Y: 200, Vitality: 80, Genome: genomeOf(50, 50, 50)})
	quarry := w.addAgent(Agent{Maturity: 1, X: 215, Y: 200, Vitality: 80, Genome: genomeOf(50, 50, 50)})
	w.rememberAffinity(mustAgent(t, w, subject), friend, 10)
	mustAgent(t, w, friend).Action = Action{Kind: ActAttack, TargetID: quarry}

	c := &AIController{}
	c.Decide(w.perceive(mustAgent(t, w, subject)))
	if !c.SawFriendFight {
		t.Fatal("a friend swinging at a stranger in plain sight was not counted")
	}

	// The other way round: the one being fought is the one it is fond of.
	w2 := NewWorld(sidesConfig())
	s2 := w2.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Vitality: 80, Genome: genomeOf(50, 100, 100)})
	f2 := w2.addAgent(Agent{Maturity: 1, X: 205, Y: 200, Vitality: 80, Genome: genomeOf(50, 50, 50)})
	q2 := w2.addAgent(Agent{Maturity: 1, X: 215, Y: 200, Vitality: 80, Genome: genomeOf(50, 50, 50)})
	w2.rememberAffinity(mustAgent(t, w2, s2), q2, 10)
	mustAgent(t, w2, f2).Action = Action{Kind: ActAttack, TargetID: q2}

	c2 := &AIController{}
	c2.Decide(w2.perceive(mustAgent(t, w2, s2)))
	if c2.SawFriendFight {
		t.Fatal("a stranger swinging at a friend was counted as a friend's fight")
	}
}

// --- killing one that was somebody's -----------------------------------------

// Seeing one it was fond of killed costs the killer that body's goodwill, in
// proportion to what the dead one was worth to it - and a stranger's death
// costs the killer nothing, which is what keeps this an accounting of what was
// lost rather than a rule about killing.
func TestKillingSomebodysFriendCostsTheKillerGoodwill(t *testing.T) {
	build := func(fond float64) (*World, int, int) {
		cfg := sidesConfig()
		cfg.AffinityNegative = true
		cfg.AffinityKilledMine = 1
		cfg.AffinityWitnessKill = 0
		w := NewWorld(cfg)
		watcher := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Genome: genomeOf(50, 50, 50)})
		killer := w.addAgent(Agent{Maturity: 1, X: 205, Y: 200, Genome: genomeOf(50, 50, 50)})
		victim := w.addAgent(Agent{Maturity: 1, X: 210, Y: 200, Genome: genomeOf(50, 50, 50)})
		if fond > 0 {
			w.rememberAffinity(mustAgent(t, w, watcher), victim, fond)
		}
		w.witnessKill(mustAgent(t, w, victim), []int{killer})
		return w, watcher, killer
	}

	w, watcher, killer := build(10)
	op := mustAgent(t, w, watcher).opinion(killer)
	if op == nil || op.Affinity != -10 {
		t.Fatalf("the onlooker thinks %+v of the killer, want -10", op)
	}
	if got := w.Stats().MournWitnesses; got != 1 {
		t.Fatalf("MournWitnesses = %d, want 1", got)
	}

	w2, watcher2, killer2 := build(0)
	if op := mustAgent(t, w2, watcher2).opinion(killer2); op != nil && op.Affinity < 0 {
		t.Fatalf("a stranger's death cost the killer goodwill: %+v", op)
	}
}
