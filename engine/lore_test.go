package engine

import (
	"math"
	"math/rand"
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

// --- stage 12b: trading what you assume --------------------------------------

// The trade is not one of them learning from the other. Both move towards each
// other by the same amount at the same moment, so neither can take without
// giving.
func TestTradingWhatYouAssumeMovesBothSidesAlike(t *testing.T) {
	cfg := quietConfig()
	cfg.LoreExchangeRate = 0.5
	w := NewWorld(cfg)

	a := &w.agents[w.addAgent(Agent{X: 10, Y: 10})-1]
	b := &w.agents[w.addAgent(Agent{X: 12, Y: 10})-1]
	a.lore.riskWeight, b.lore.riskWeight = 0.10, 0.30
	a.lore.retaliation.mean, b.lore.retaliation.mean = 0.2, 0.8
	surer := b.lore.retaliation.n

	w.exchangeLore(a, b)

	if a.lore.riskWeight != 0.20 || b.lore.riskWeight != 0.20 {
		t.Fatalf("after meeting in the middle the two want %v and %v, want both at 0.20",
			a.lore.riskWeight, b.lore.riskWeight)
	}
	if math.Abs(a.lore.retaliation.mean-0.5) > 1e-9 || math.Abs(b.lore.retaliation.mean-0.5) > 1e-9 {
		t.Fatalf("beliefs came out at %v and %v, want both at 0.5",
			a.lore.retaliation.mean, b.lore.retaliation.mean)
	}
	// Being told something is a claim, not the years of watching behind it.
	if b.lore.retaliation.n != surer {
		t.Fatalf("being told something changed how sure the teller was: %v then %v", surer, b.lore.retaliation.n)
	}
}

// What the trade was worth is what actually changed hands, and both sides get
// the same. Two agents that already agree trade nothing and think no more of
// each other for it - which falls out of the arithmetic, not out of a test for
// it.
func TestATradeIsWorthWhatActuallyChangedHands(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)

	affinityAfter := func(gap float64) float64 {
		a := &w.agents[w.addAgent(Agent{X: 10, Y: 10})-1]
		b := &w.agents[w.addAgent(Agent{X: 12, Y: 10})-1]
		a.lore.riskWeight = cfg.RiskWeight
		b.lore.riskWeight = cfg.RiskWeight + gap
		w.exchangeLore(a, b)
		// A trade worth nothing between two strangers leaves no record at
		// all: standing next to somebody and agreeing with them about
		// everything is not how you come to know them.
		held := func(x, y *Agent) float64 {
			op := x.opinion(y.ID)
			if op == nil {
				return 0
			}
			return w.decayedAffinity(x, op)
		}
		mine, theirs := held(a, b), held(b, a)
		if math.Abs(mine-theirs) > 1e-9 {
			t.Fatalf("the two sides of one trade were worth %v and %v, want the same", mine, theirs)
		}
		return mine
	}

	none, some, more := affinityAfter(0), affinityAfter(0.05), affinityAfter(0.15)
	if none != 0 {
		t.Fatalf("two agents that already agree gained %v of affinity, want none", none)
	}
	if !(more > some && some > 0) {
		t.Fatalf("affinity from trades of nothing/some/more came out %v, %v, %v, want it to grow with the gap",
			none, some, more)
	}
}

// Whether or not there was anything to trade, they met: what each remembers of
// the other stops going stale. Meeting is not a favour, so it happens either
// way.
func TestMeetingRefreshesTheRecordEvenWhenNothingIsTraded(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)

	// Two identical pairs. Halfway through, one pair meets and the other does
	// not; nothing is traded either way, because the two sides already agree.
	met := &w.agents[w.addAgent(Agent{X: 10, Y: 10})-1]
	metBy := &w.agents[w.addAgent(Agent{X: 12, Y: 10})-1]
	apart := &w.agents[w.addAgent(Agent{X: 90, Y: 90})-1]
	apartFrom := &w.agents[w.addAgent(Agent{X: 92, Y: 90})-1]
	w.rememberDamage(met, metBy.ID, 20)
	w.rememberDamage(apart, apartFrom.ID, 20)

	w.tick += 500
	metBy.lore = met.lore // identical: the trade moves nothing at all
	w.exchangeLore(met, metBy)
	w.tick += 500

	kept := w.decayedRisk(met, met.opinion(metBy.ID))
	faded := w.decayedRisk(apart, apart.opinion(apartFrom.ID))
	if kept <= faded {
		t.Fatalf("the pair that met remembers %v and the pair that did not %v, want meeting to have kept it fresher",
			kept, faded)
	}
}

// LoreExchangeRate 0 is the arm that says whether what spreads through a
// population spread by being taught or by being survived.
func TestExchangeRateZeroStopsAnythingBeingHandedOn(t *testing.T) {
	cfg := quietConfig()
	cfg.LoreExchangeRate = 0
	w := NewWorld(cfg)

	a := &w.agents[w.addAgent(Agent{X: 10, Y: 10})-1]
	b := &w.agents[w.addAgent(Agent{X: 12, Y: 10})-1]
	a.lore.riskWeight, b.lore.riskWeight = 0.10, 0.30
	w.exchangeLore(a, b)

	if a.lore.riskWeight != 0.10 || b.lore.riskWeight != 0.30 {
		t.Fatalf("something was handed on with the rate at zero: %v and %v", a.lore.riskWeight, b.lore.riskWeight)
	}
	if w.exchanges != 0 {
		t.Fatalf("%d trades were counted with the rate at zero", w.exchanges)
	}
}

// The second path by which somebody else being alive is worth something: an
// agent will spend time on one it has got something out of before. Nothing
// here reads what the other actually believes - it cannot - only who has been
// worth standing with.
func TestWatchingSomebodyYouTrustIsWorthMoreThanWatchingAStranger(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)

	watchValue := func(affinity, loreValue float64) float64 {
		cfg := cfg
		cfg.LoreValue = loreValue
		p := &Perception{
			Tick: 1, Cfg: &cfg, Rand: w.rng,
			Self: SelfView{
				ID: 1, X: 200, Y: 200, Vitality: 100, Hunger: 20,
				MaxVitality: cfg.MaxVitality, MaxSpeed: cfg.MaxSpeed,
				Retaliation: cfg.Retaliation, AcceptChance: cfg.AcceptChance,
				RiskWeight: cfg.RiskWeight, CompetitionWeight: cfg.CompetitionWeight,
				ShockRisk: cfg.ShockRisk},
		}
		o := AgentView{ID: 2, Dist: 10, Vitality: 60, EstStrength: 40,
			Uncertainty: cfg.PriorVariance, Affinity: affinity}

		c := &AIController{}
		c.addObserve(p, &o)
		return c.opts[0].util
	}

	stranger := watchValue(0, cfg.LoreValue)
	friend := watchValue(cfg.AffinityTrust, cfg.LoreValue)
	if friend <= stranger {
		t.Fatalf("watching a friend scored %v and a stranger %v, want the friend to be worth more", friend, stranger)
	}
	if off := watchValue(cfg.AffinityTrust, 0); off != stranger {
		t.Fatalf("with LoreValue 0 a friend scored %v and a stranger %v, want them equal", off, stranger)
	}
}

// TopShare needs a scale before it means anything. Trading spread perfectly
// evenly puts a fifth of it on a fifth of the population; trading handed out at
// random still lands above that, because counts vary. That noise floor is what
// a measured value has to beat before it is evidence of anybody becoming a hub.
func TestTeachingTopShareHasACalibratedScale(t *testing.T) {
	cfg := quietConfig()

	topShare := func(counts []int) float64 {
		w := NewWorld(cfg)
		for _, n := range counts {
			a := &w.agents[w.addAgent(Agent{X: 10, Y: 10})-1]
			a.Age = 1000
			a.timesTaught = n
		}
		return w.Teaching().TopShare
	}

	even := make([]int, 20)
	for i := range even {
		even[i] = 50
	}
	if got := topShare(even); math.Abs(got-0.2) > 1e-9 {
		t.Fatalf("trading spread perfectly evenly gives a top share of %v, want 0.2", got)
	}

	// The noise floor: the same total handed out at random.
	rng := rand.New(rand.NewSource(7))
	sum := 0.0
	const trials = 200
	for i := 0; i < trials; i++ {
		random := make([]int, 20)
		for j := 0; j < 20*50; j++ {
			random[rng.Intn(20)]++
		}
		sum += topShare(random)
	}
	floor := sum / trials
	if floor < 0.2 || floor > 0.26 {
		t.Fatalf("the noise floor for a top share came out at %v; the scale in the docs says about 0.22", floor)
	}
}
