package engine

import (
	"math"
	"testing"
)

// A belief is a running mean with a cap on how much evidence stands behind it,
// so an agent shown the same answer over and over converges on it and an agent
// shown a different one afterwards can still change its mind.
func TestBeliefConvergesAndStaysAbleToChangeItsMind(t *testing.T) {
	b := belief{mean: 0.7, n: 8}
	for i := 0; i < 500; i++ {
		b.observe(0, 1, 60)
	}
	if b.mean > 0.02 {
		t.Fatalf("after 500 observations of never, the belief is %.3f, want it near zero", b.mean)
	}
	if b.n != 60 {
		t.Fatalf("evidence count is %v, want it capped at 60", b.n)
	}
	for i := 0; i < 500; i++ {
		b.observe(1, 1, 60)
	}
	if b.mean < 0.98 {
		t.Fatalf("the world changed and the belief only reached %.3f, want it to follow", b.mean)
	}
}

// The cap is what makes that possible: without it the count keeps climbing and
// each new observation counts for proportionally less, until an old agent
// cannot notice anything at all.
func TestUncappedEvidenceWouldFreezeABelief(t *testing.T) {
	capped, uncapped := belief{mean: 0.7, n: 8}, belief{mean: 0.7, n: 8}
	for i := 0; i < 2000; i++ {
		capped.observe(0, 1, 60)
		uncapped.observe(0, 1, math.Inf(1))
	}
	for i := 0; i < 100; i++ {
		capped.observe(1, 1, 60)
		uncapped.observe(1, 1, math.Inf(1))
	}
	if capped.mean < 0.5 {
		t.Fatalf("the capped belief only reached %.3f after a hundred observations, want it to have moved", capped.mean)
	}
	if uncapped.mean > 0.1 {
		t.Fatalf("the uncapped belief reached %.3f, want the test to show it stuck", uncapped.mean)
	}
}

// LearningRate 0 is the switch the stage is measured against: nothing an agent
// sees moves anything it assumes.
func TestLearningRateZeroStopsEverything(t *testing.T) {
	cfg := quietConfig()
	cfg.LearningRate = 0
	w := NewWorld(cfg)
	a := &w.agents[w.addAgent(Agent{X: 10, Y: 10})-1]

	before := a.lore
	for i := 0; i < 200; i++ {
		w.noteRetaliation(a, true)
		w.noteCourtship(a, false)
	}
	if a.lore.retaliation != before.retaliation || a.lore.accept != before.accept {
		t.Fatalf("beliefs moved with learning off: %+v then %+v", before, a.lore)
	}
	// The world's own tally still runs: it is the measurement, not a rule, and
	// nothing consults it.
	if w.courtships != 200 {
		t.Fatalf("the world counted %d courtships, want it to keep counting with learning off", w.courtships)
	}
}

// What an agent has come to assume is what its own decisions are made with. A
// world where nobody hits back and one where everybody does have to price a
// fight differently, and nothing but the belief is different between them.
func TestWhatAnAgentAssumesChangesWhatAFightIsWorth(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)

	attackValue := func(retaliation float64) float64 {
		p := &Perception{
			Tick: 1,
			Cfg:  &cfg,
			Self: SelfView{
				ID: 1, X: 200, Y: 200, Vitality: 100, Hunger: 20,
				Attack: 60, Rationality: 100, Intelligence: 100,
				MaxVitality: cfg.MaxVitality, MaxSpeed: cfg.MaxSpeed,
				Retaliation:       retaliation,
				AcceptChance:      cfg.AcceptChance,
				RiskWeight:        cfg.RiskWeight,
				CompetitionWeight: cfg.CompetitionWeight,
				ShockRisk:         cfg.ShockRisk},
			Rand: w.rng}
		victim := AgentView{ID: 2, Dist: 10, Vitality: 60, EstStrength: 60}

		c := &AIController{}
		c.addAttack(p, &victim)
		best := math.Inf(-1)
		for _, o := range c.opts {
			best = math.Max(best, o.util)
		}
		return best
	}

	expectsNothing, expectsAFight := attackValue(0.1), attackValue(0.9)
	if expectsNothing <= expectsAFight {
		t.Fatalf("picking a fight scored %v expecting no answer and %v expecting one, want the first to be worth more",
			expectsNothing, expectsAFight)
	}
}

// Preferences are inherited whole from one parent or the other, the same way
// the genes are, rather than averaged: averaging halves the spread every
// generation and there is soon nothing left to select on.
func TestPreferencesComeWholeFromOneParent(t *testing.T) {
	cfg := quietConfig()
	cfg.LoreMutationStd = 0
	w := NewWorld(cfg)

	pa := &w.agents[w.addAgent(Agent{X: 10, Y: 10})-1]
	pb := &w.agents[w.addAgent(Agent{X: 12, Y: 10})-1]
	pa.lore.riskWeight, pb.lore.riskWeight = 0.1, 0.5
	pa.lore.shockRisk, pb.lore.shockRisk = 0.2, 0.8

	seenLow, seenHigh := 0, 0
	for i := 0; i < 400; i++ {
		child := w.inheritLore(pa, pb)
		switch child.riskWeight {
		case 0.1:
			seenLow++
		case 0.5:
			seenHigh++
		default:
			t.Fatalf("a child got a risk weight of %v, want one of its parents' values", child.riskWeight)
		}
		if child.shockRisk != 0.2 && child.shockRisk != 0.8 {
			t.Fatalf("a child got a shock risk of %v, want one of its parents' values", child.shockRisk)
		}
	}
	if seenLow == 0 || seenHigh == 0 {
		t.Fatalf("in 400 children the low value came up %d times and the high one %d, want both", seenLow, seenHigh)
	}
}

// The two preferences that price violence are kept inside the range the world
// was tuned in. Without a ceiling the mutation is a random walk with nothing to
// stop it.
func TestPreferencesCannotRunAway(t *testing.T) {
	cfg := quietConfig()
	cfg.LoreMutationStd = 2 // absurd on purpose
	w := NewWorld(cfg)

	pa := &w.agents[w.addAgent(Agent{X: 10, Y: 10})-1]
	pb := &w.agents[w.addAgent(Agent{X: 12, Y: 10})-1]
	high := cfg.CompetitionWeight * maxPreferenceFactor
	for i := 0; i < 2000; i++ {
		child := w.inheritLore(pa, pb)
		if child.competitionWeight < 0 || child.competitionWeight > high {
			t.Fatalf("a preference reached %v, want it kept inside [0, %v]", child.competitionWeight, high)
		}
		pa.lore, pb.lore = child, child
	}
}

// Lamarck is the switch that separates a belief spreading because the ones who
// held it lived from a belief spreading because it was handed down. At 0 a
// child starts where the world's own figure is, whatever its parents came to
// think; at 1 it starts where they got to.
func TestLamarckRateDecidesWhetherBeliefsAreHandedDown(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)

	pa := &w.agents[w.addAgent(Agent{X: 10, Y: 10})-1]
	pb := &w.agents[w.addAgent(Agent{X: 12, Y: 10})-1]
	pa.lore.retaliation.mean, pb.lore.retaliation.mean = 0.1, 0.1

	w.cfg.LamarckRate = 0
	if got := w.inheritLore(pa, pb).retaliation.mean; got != cfg.Retaliation {
		t.Fatalf("with LamarckRate 0 a child believes %.3f, want the world's own figure %.3f", got, cfg.Retaliation)
	}
	w.cfg.LamarckRate = 1
	if got := w.inheritLore(pa, pb).retaliation.mean; math.Abs(got-0.1) > 1e-9 {
		t.Fatalf("with LamarckRate 1 a child believes %.3f, want its parents' %.3f", got, 0.1)
	}
	// However it got there, it is a starting point and not a lifetime of
	// evidence: a handed down belief gives way to experience as readily as the
	// world's figure would.
	if got := w.inheritLore(pa, pb).retaliation.n; got != cfg.LorePriorCount {
		t.Fatalf("a handed down belief carries %v of evidence, want the prior's %v", got, cfg.LorePriorCount)
	}
}

// An agent handed to the world rather than drawn by it - which in practice
// means a test - assumes exactly what the world says, and getting it does not
// move the random source along.
func TestAnAgentBuiltByHandAssumesTheWorldsOwnFigures(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)

	before := w.rng.Float64()
	w2 := NewWorld(cfg)
	w2.addAgent(Agent{X: 10, Y: 10})
	if after := w2.rng.Float64(); after != before {
		t.Fatalf("adding an agent consumed randomness: %v then %v", before, after)
	}
	a := &w2.agents[0]
	if a.lore.riskWeight != cfg.RiskWeight || a.lore.retaliation.mean != cfg.Retaliation {
		t.Fatalf("an agent built by hand assumes %+v, want the world's own figures", a.lore)
	}
	if a.lore.unset() {
		t.Fatal("the lore still reads as unset after being filled in")
	}
}

// The question the utility formula asks is whether picking a fight gets you hit
// back, which is a question about an engagement. It is answered once per
// engagement rather than once per blow, or a single long beating would drown
// out every other fight in the world.
func TestRetaliationIsAnsweredOncePerEngagement(t *testing.T) {
	cfg := quietConfig()
	cfg.LearningRate = 1
	w := NewWorld(cfg)

	aID := w.addAgent(Agent{X: 100, Y: 100, Vitality: 500})
	bID := w.addAgent(Agent{X: 102, Y: 100, Vitality: 500})
	a, b := w.agentByID(aID), w.agentByID(bID)
	a.Action = Action{Kind: ActAttack, TargetID: bID, Effort: 1}
	b.Action = Action{Kind: ActRest}
	a.controller = frozenController{Action{Kind: ActAttack, TargetID: bID, Effort: 1}}
	b.controller = frozenController{Action{Kind: ActRest}}

	for i := 0; i < 200; i++ {
		w.Step()
	}
	if w.blowsSeen == 0 {
		t.Fatal("no engagement was ever sampled")
	}
	// Two hundred ticks of continuous blows is one engagement, not two hundred.
	if w.blowsSeen > 3 {
		t.Fatalf("one long fight was sampled %d times, want it counted about once", w.blowsSeen)
	}
	if w.blowsAnswered != 0 {
		t.Fatalf("the target never hit back but %d of %d samples say it did", w.blowsAnswered, w.blowsSeen)
	}
	if got := a.lore.retaliation.mean; got >= cfg.Retaliation {
		t.Fatalf("after being ignored the attacker still believes %.3f, want it below %.3f", got, cfg.Retaliation)
	}
}

// frozenController keeps an agent doing one thing however the world changes
// around it, so that a fight can be held still and watched.
type frozenController struct{ act Action }

func (c frozenController) Decide(*Perception) Action { return c.act }
