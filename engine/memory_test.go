package engine

import (
	"math"
	"testing"
)

// Tests for what an agent can hold on to about other agents: how much of it
// there is room for, how fast it arrives, how it fades, and the one positive
// thing in there.

// crowd puts n agents in a ring around a point, close enough to be seen.
func crowd(w *World, n int, x, y float64) []int {
	ids := make([]int, 0, n)
	for i := 0; i < n; i++ {
		angle := 2 * math.Pi * float64(i) / float64(n)
		ids = append(ids, w.addAgent(Agent{Maturity: 1,
			X: x + 30*math.Cos(angle), Y: y + 30*math.Sin(angle),
			Sex: Male, Vitality: 80, Hunger: 10, Genome: genomeOf(50, 50, 50),
		}))
	}
	return ids
}

// --- capacity ---------------------------------------------------------------

// An agent cannot know everybody. However many faces go past it, the number of
// records it holds stops at what it spent on memory.
func TestMemoryCapacityBoundsWhatAnAgentKnows(t *testing.T) {
	cfg := quietConfig()
	cfg.MemoryCapacity = 5
	cfg.MemoryBandwidthShare = 0 // bandwidth off: capacity alone is on trial
	w := NewWorld(cfg)

	subject := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 10,
		Genome: genomeOf(50, 100, 100)})
	crowd(w, 20, 200, 200)

	for i := 0; i < 50; i++ {
		w.Step()
	}
	a := mustAgent(t, w, subject)
	want := a.MemoryCapacity(&cfg)
	if want != 5 {
		t.Fatalf("an average memory holds %d, want the configured 5", want)
	}
	if got := len(a.opinions); got > want {
		t.Fatalf("holds %d records with room for %d", got, want)
	}
	if len(a.opinions) == 0 {
		t.Fatal("holds nothing at all, so the test is not measuring the limit")
	}
}

// The capacity is the memory gene's, not the world's: a better memory holds
// more of the same crowd.
func TestABetterMemoryHoldsMoreOfTheSameCrowd(t *testing.T) {
	cfg := quietConfig()
	cfg.MemoryCapacity = 8

	poor := newGenome()
	rich := newGenome()
	for i := range poor {
		poor[i], rich[i] = midAbility, midAbility
	}
	poor[GeneMemory], rich[GeneMemory] = 10, 100

	a := &Agent{Maturity: 1, Genome: poor}
	b := &Agent{Maturity: 1, Genome: rich}
	if a.MemoryCapacity(&cfg) >= b.MemoryCapacity(&cfg) {
		t.Fatalf("capacity %d with a poor memory, %d with a good one",
			a.MemoryCapacity(&cfg), b.MemoryCapacity(&cfg))
	}
	// And it forgets what it does hold more slowly.
	if a.ForgetScale(&cfg) <= b.ForgetScale(&cfg) {
		t.Fatalf("forget scale %.2f with a poor memory, %.2f with a good one",
			a.ForgetScale(&cfg), b.ForgetScale(&cfg))
	}
}

// A memory full of people who matter has no room for a face in the crowd, but
// the world's own events get in anyway: an agent does not get to decline to
// notice that it was hit.
func TestAFullMemoryKeepsWhatMattersAndStillNoticesABlow(t *testing.T) {
	cfg := quietConfig()
	cfg.MemoryCapacity = 3
	w := NewWorld(cfg)

	subject := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Sex: Male, Vitality: 80, Genome: genomeOf(50, 50, 50)})
	a := mustAgent(t, w, subject)

	// Three people it has reason to remember.
	for id := 101; id <= 103; id++ {
		w.rememberDamage(a, id, 10)
	}
	if got := len(a.opinions); got != 3 {
		t.Fatalf("holds %d records, want the 3 it was given", got)
	}

	// A stranger cannot displace any of them.
	if op := w.recordOpinion(a, 200); op != nil {
		t.Fatal("took a stranger on over three memories that still carry weight")
	}
	if _, ok := a.opinions[200]; ok {
		t.Fatal("the stranger got in anyway")
	}

	// Being hit by that stranger does.
	w.rememberDamage(a, 200, 25)
	if _, ok := a.opinions[200]; !ok {
		t.Fatal("forgot the one that just hit it")
	}
	if got := len(a.opinions); got != 3 {
		t.Fatalf("holds %d records with room for 3", got)
	}
	// And what it gave up is the cheapest of what it had, not the dearest.
	if _, ok := a.opinions[200]; ok && len(a.opinions) == 3 {
		for id := 101; id <= 103; id++ {
			if op, ok := a.opinions[id]; ok && op.Risk < 10 {
				t.Fatalf("kept #%d but its record was emptied", id)
			}
		}
	}
}

// A worthless record - somebody it once looked at and nothing came of it - is
// what gets given up when somebody new turns up.
func TestAStrangerDisplacesAnEmptyAcquaintance(t *testing.T) {
	cfg := quietConfig()
	cfg.MemoryCapacity = 2
	cfg.MemoryBandwidthShare = 0
	w := NewWorld(cfg)

	subject := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Sex: Male, Vitality: 80, Genome: genomeOf(50, 50, 50)})
	a := mustAgent(t, w, subject)

	w.rememberDamage(a, 101, 12) // matters
	if op := w.recordOpinion(a, 102); op == nil {
		t.Fatal("no room for a second acquaintance")
	}
	if op := w.recordOpinion(a, 103); op == nil {
		t.Fatal("would not give up an empty record for a new face")
	}
	if _, ok := a.opinions[101]; !ok {
		t.Fatal("gave up the one that had cost it vitality")
	}
	if _, ok := a.opinions[102]; ok {
		t.Fatal("kept the empty record instead of the new face")
	}
}

// --- bandwidth --------------------------------------------------------------

// Records are taken in a few at a time. Walking into a crowd does not hand an
// agent the whole crowd at once.
func TestBandwidthLimitsWhatIsTakenInPerTick(t *testing.T) {
	cfg := quietConfig()
	cfg.MemoryCapacity = 40
	cfg.MemoryBandwidthShare = 0.1 // four a tick for an average memory
	w := NewWorld(cfg)

	subject := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 10,
		Genome: genomeOf(50, 100, 100)})
	crowd(w, 20, 200, 200)
	a := mustAgent(t, w, subject)
	if got := a.MemoryBandwidth(&cfg); got != 4 {
		t.Fatalf("bandwidth %d, want 4", got)
	}

	w.Step()
	if got := len(a.opinions); got > 4 {
		t.Fatalf("took in %d records in one tick with room for 4 of them", got)
	}
}

// Bandwidth off is the world before stage 9: everything in sight registers.
func TestBandwidthOffTakesTheWholeCrowdIn(t *testing.T) {
	cfg := quietConfig()
	cfg.MemoryCapacity = 0
	cfg.MemoryBandwidthShare = 0
	w := NewWorld(cfg)

	subject := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 10,
		Genome: genomeOf(50, 100, 100)})
	ids := crowd(w, 12, 200, 200)
	w.Step()

	a := mustAgent(t, w, subject)
	if got := len(a.opinions); got != len(ids) {
		t.Fatalf("holds %d records of %d agents in sight, want all of them", got, len(ids))
	}
}

// --- the forgetting curve ---------------------------------------------------

// How fast a memory fades is the holder's own. The same blow is remembered
// longer by the agent that paid for a memory.
func TestForgettingRunsAtTheHoldersOwnRate(t *testing.T) {
	cfg := quietConfig()
	cfg.ContactRefresh = false
	w := NewWorld(cfg)

	forgetful := newGenome()
	sharp := newGenome()
	for i := range forgetful {
		forgetful[i], sharp[i] = midAbility, midAbility
	}
	forgetful[GeneMemory], sharp[GeneMemory] = 5, 95

	dull := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Sex: Male, Vitality: 80, Genome: forgetful})
	keen := w.addAgent(Agent{Maturity: 1, X: 300, Y: 300, Sex: Male, Vitality: 80, Genome: sharp})
	w.rememberDamage(mustAgent(t, w, dull), 900, 30)
	w.rememberDamage(mustAgent(t, w, keen), 900, 30)

	for i := 0; i < 400; i++ {
		w.Step()
	}
	a, b := mustAgent(t, w, dull), mustAgent(t, w, keen)
	fadedDull := w.decayedRisk(a, a.opinions[900])
	fadedKeen := w.decayedRisk(b, b.opinions[900])
	if !(fadedDull < fadedKeen) {
		t.Fatalf("after 400 ticks the forgetful one remembers %.2f and the sharp one %.2f",
			fadedDull, fadedKeen)
	}
	if fadedKeen >= 30 {
		t.Fatalf("the sharp one forgot nothing at all (%.2f)", fadedKeen)
	}
}

// Seeing somebody again keeps what is known about them from going stale. The
// curve is untouched; where it is measured from is not.
func TestSeeingSomebodyKeepsTheMemoryOfThemFresh(t *testing.T) {
	run := func(refresh bool) float64 {
		cfg := quietConfig()
		cfg.ContactRefresh = refresh
		w := NewWorld(cfg)
		subject := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 10,
			Genome: genomeOf(50, 100, 100)})
		other := w.addAgent(Agent{Maturity: 1, X: 215, Y: 200, Sex: Male, Vitality: 80, Hunger: 10,
			Genome: genomeOf(50, 50, 50)})
		w.SetController(other, fixedController{Action{Kind: ActRest}})
		w.rememberDamage(mustAgent(t, w, subject), other, 30)
		for i := 0; i < 600; i++ {
			w.Step()
		}
		a := mustAgent(t, w, subject)
		return w.decayedRisk(a, a.opinions[other])
	}

	kept, faded := run(true), run(false)
	if !(kept > faded) {
		t.Fatalf("risk of a neighbour it keeps seeing = %.2f, of one it does not = %.2f", kept, faded)
	}
}

// --- affinity ---------------------------------------------------------------

// A pair remembers each other for it. This is the first positive thing in the
// memory, and it comes from the event, not from the time spent side by side.
func TestPairingLeavesAffinityOnBothSides(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)
	a := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Sex: Male, Vitality: 90, Genome: genomeOf(50, 50, 50)})
	b := w.addAgent(Agent{Maturity: 1, X: 204, Y: 200, Sex: Female, Vitality: 90, Genome: genomeOf(50, 50, 50)})
	w.bond(mustAgent(t, w, a), mustAgent(t, w, b))

	for _, pair := range [][2]int{{a, b}, {b, a}} {
		holder := mustAgent(t, w, pair[0])
		op := holder.opinion(pair[1])
		if op == nil || op.Affinity < cfg.AffinityPairBond {
			t.Fatalf("#%d remembers nothing good about its partner", pair[0])
		}
	}
}

// A parent starts part of the way in with its own child, without either of
// them having done anything yet.
func TestParentAndChildStartWithAffinity(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)
	parent := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Sex: Female, Vitality: 90, Genome: genomeOf(50, 50, 50)})
	child := w.addAgent(Agent{Maturity: 1, X: 206, Y: 200, Sex: Male, Vitality: 90, Genome: genomeOf(50, 50, 50)})

	p, c := mustAgent(t, w, parent), mustAgent(t, w, child)
	p.ChildIDs = append(p.ChildIDs, child)
	c.ParentIDs = [2]int{parent, 0}

	if op := w.recordOpinion(p, child); op == nil || op.Affinity != cfg.AffinityKin {
		t.Fatalf("a parent's first record of its child carries affinity %v, want %v",
			op, cfg.AffinityKin)
	}
	if op := w.recordOpinion(c, parent); op == nil || op.Affinity != cfg.AffinityKin {
		t.Fatalf("a child's first record of its parent carries no affinity")
	}
	// One hop only: a stranger is a stranger.
	if op := w.recordOpinion(p, 777); op == nil || op.Affinity != 0 {
		t.Fatal("a stranger was treated as family")
	}
}

// Affinity fades like everything else in the record, and switching it off
// leaves nobody fond of anybody.
func TestAffinityFadesAndCanBeTurnedOff(t *testing.T) {
	cfg := quietConfig()
	cfg.ContactRefresh = false
	w := NewWorld(cfg)
	a := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Sex: Male, Vitality: 90, Genome: genomeOf(50, 50, 50)})
	holder := mustAgent(t, w, a)
	w.rememberAffinity(holder, 500, 20)
	start := w.decayedAffinity(holder, holder.opinions[500])
	for i := 0; i < 1000; i++ {
		w.Step()
	}
	if faded := w.decayedAffinity(holder, holder.opinions[500]); faded >= start {
		t.Fatalf("affinity %.2f after 1000 ticks, started at %.2f", faded, start)
	}

	off := quietConfig()
	off.AffinityPairBond, off.AffinityBirth, off.AffinityKin = 0, 0, 0
	w2 := NewWorld(off)
	x := w2.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Sex: Male, Vitality: 90, Genome: genomeOf(50, 50, 50)})
	y := w2.addAgent(Agent{Maturity: 1, X: 204, Y: 200, Sex: Female, Vitality: 90, Genome: genomeOf(50, 50, 50)})
	w2.bond(mustAgent(t, w2, x), mustAgent(t, w2, y))
	if op := mustAgent(t, w2, x).opinion(y); op != nil && op.Affinity != 0 {
		t.Fatalf("affinity %v with every source set to zero", op.Affinity)
	}
}

// --- resting in the open ----------------------------------------------------

// restUtility is what the agent thinks resting is worth in this situation.
func restUtility(t *testing.T, w *World, id int) Utility {
	t.Helper()
	tr := decideWithTrace(t, w, id)
	for _, o := range tr.Options {
		if o.Action.Kind == ActRest {
			return o.Utility
		}
	}
	t.Fatal("resting was not among the options")
	return Utility{}
}

// restScene builds a battered but well fed agent with strangers standing over
// it: the situation where recovering is the obvious move and doing it here is
// the question.
func restScene(cfg Config) (*World, int, []int) {
	w := NewWorld(cfg)
	subject := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Sex: Male, Vitality: 30, Hunger: 5,
		Genome: genomeOf(50, 100, 100)})
	others := crowd(w, 4, 200, 200)
	// Awake. An agent that has not decided anything yet has the zero Action,
	// which is ActRest, and since stage 18 a sleeping neighbour is not one
	// that can be trusted to keep watch - so a crowd left at its zero value
	// would be asleep and the trust would never apply.
	for _, id := range others {
		if o := w.agentByID(id); o != nil {
			o.Action = Action{Kind: ActMove}
		}
	}
	return w, subject, others
}

// Lying down among strangers is worth less than lying down among your own.
// Nothing here says "do not rest near strangers": what the agent sees is that
// the vitality it is trying to win back is the vitality they could take.
func TestRestingIsWorthLessAmongStrangersThanAmongFriends(t *testing.T) {
	cfg := quietConfig()

	wStranger, subject, _ := restScene(cfg)
	amongStrangers := restUtility(t, wStranger, subject).Life.Score()

	wFriends, subject2, friends := restScene(cfg)
	holder := mustAgent(t, wFriends, subject2)
	for _, id := range friends {
		wFriends.rememberAffinity(holder, id, cfg.AffinityTrust)
	}
	amongFriends := restUtility(t, wFriends, subject2).Life.Score()

	if !(amongFriends > amongStrangers) {
		t.Fatalf("resting scored %.3f among friends and %.3f among strangers",
			amongFriends, amongStrangers)
	}
}

// With the weight at zero the world rests exactly as it did before stage 9:
// wherever it is, resting is worth the same.
func TestRestExposureWeightZeroRestoresTheOldRest(t *testing.T) {
	cfg := quietConfig()
	cfg.RestExposureWeight = 0

	wCrowded, subject, _ := restScene(cfg)
	crowded := restUtility(t, wCrowded, subject).Life.Score()

	alone := NewWorld(cfg)
	lone := alone.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Sex: Male, Vitality: 30, Hunger: 5,
		Genome: genomeOf(50, 100, 100)})
	quiet := restUtility(t, alone, lone).Life.Score()

	approx(t, crowded, quiet, 1e-9, "resting in a crowd against resting alone, with the weight off")
}

// The exposure is an estimate made out of what the agent already believes, so
// somebody it reckons is dangerous puts it off more than somebody it does not.
func TestARougherNeighbourhoodMakesRestingWorthLess(t *testing.T) {
	cfg := quietConfig()
	score := func(strength float64) float64 {
		w := NewWorld(cfg)
		subject := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Sex: Male, Vitality: 30, Hunger: 5,
			Genome: genomeOf(50, 100, 100)})
		other := w.addAgent(Agent{Maturity: 1, X: 215, Y: 200, Sex: Male, Vitality: 90,
			Genome: genomeOf(strength, 50, 50)})
		convinceOf(t, w, subject, other)
		return restUtility(t, w, subject).Life.Score()
	}
	if weak, strong := score(10), score(95); !(weak > strong) {
		t.Fatalf("resting scored %.3f next to a weakling and %.3f next to a brute", weak, strong)
	}
}

// --- reading it back --------------------------------------------------------

func TestMemoryUseReportsWhatIsHeld(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)
	a := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Sex: Male, Vitality: 80, Genome: genomeOf(50, 50, 50)})
	holder := mustAgent(t, w, a)
	w.rememberDamage(holder, 900, 10)
	w.rememberAffinity(holder, 901, 10)

	use := w.MemoryUse()
	if use.Remembered != 2 {
		t.Fatalf("remembered = %v, want 2", use.Remembered)
	}
	if use.Friends != 1 {
		t.Fatalf("friends = %v, want 1", use.Friends)
	}
}

// Once an agent has been asked to give up a record and had nothing to give up,
// it is not asked again until the set of records changes. The answer cannot
// change on its own - a record's weight only grows or fades towards zero
// without reaching it - so the cached refusal has to give the same answers the
// search would have.
//
// The reason it is worth caching is stage 12b: trading gives nearly every
// remembered face some affinity, so the share of full memories with anything
// expendable in them fell from 86% to 29%, and the rest were searching their
// whole memory and refusing on every stranger for the rest of their lives.
func TestAMemoryWithNothingToSpareIsNotSearchedAgain(t *testing.T) {
	cfg := quietConfig()
	cfg.MemoryCapacity = 3
	w := NewWorld(cfg)

	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Vitality: 80, Genome: genomeOf(50, 50, 50)}))
	for id := 101; id <= 103; id++ {
		w.rememberDamage(a, id, 10)
	}

	if op := w.recordOpinion(a, 200); op != nil {
		t.Fatal("took a stranger on over three memories that still carry weight")
	}
	if !a.noSpareMemory {
		t.Fatal("the search found nothing to spare but the answer was not kept")
	}
	// Asked again, it gives the same answer - which is the point - and the
	// records are untouched.
	if op := w.recordOpinion(a, 201); op != nil {
		t.Fatal("a second stranger got in")
	}
	if got := len(a.opinions); got != 3 {
		t.Fatalf("holds %d records, want the 3 it started with", got)
	}

	// A blow evicts somebody, so the question is worth asking again: what
	// replaced them might well be expendable.
	w.rememberDamage(a, 200, 25)
	if a.noSpareMemory {
		t.Fatal("a record was replaced and the stale answer was kept")
	}
}

// And the cached answer must never cost an agent a memory it would otherwise
// have taken on. A memory with room in it is never refused, however many times
// it has been full before.
func TestRoomThatOpensUpIsUsed(t *testing.T) {
	cfg := quietConfig()
	cfg.MemoryCapacity = 3
	w := NewWorld(cfg)

	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Vitality: 80, Genome: genomeOf(50, 50, 50)}))
	for id := 101; id <= 103; id++ {
		w.rememberDamage(a, id, 10)
	}
	if op := w.recordOpinion(a, 200); op != nil {
		t.Fatal("took a stranger on over a full memory")
	}

	// Growing into a bigger memory is room, and it has to be usable.
	cfg2 := w.cfg
	cfg2.MemoryCapacity = 6
	w.cfg = cfg2
	if op := w.recordOpinion(a, 201); op == nil {
		t.Fatal("refused a stranger although the memory had grown room for it")
	}
	if a.noSpareMemory {
		t.Fatal("a record joined and the stale answer was kept")
	}
}
