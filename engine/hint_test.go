package engine

import (
	"math"
	"testing"
)

// The one thing a hint may never do is decide anything. It is added to a
// score, and the option still has to win the same comparison: a hint pushing
// hard for a move that would kill the agent loses to the life term the way
// anything else would.
func TestAHintCannotOverrideTheComparison(t *testing.T) {
	cfg := testConfig()
	cfg.HintWeightMax = 20
	w := NewWorld(cfg)

	c := &AIController{}
	c.hints = []Hint{{Feature: HintHunger, Act: ActAttack, Weight: cfg.HintWeightMax}}
	c.feats[HintHunger] = 1

	p := &Perception{Cfg: &cfg, Rand: w.rng, Self: SelfView{Intelligence: MaxAbility}}
	c.add(Action{Kind: ActRest}, Utility{Life: Goal{Value: 500, Chance: 1}})
	c.add(Action{Kind: ActAttack}, Utility{Life: Goal{Value: -500, Chance: 1}})

	if got := c.pick(p).Kind; got != ActRest {
		t.Fatalf("a hint of the largest weight there is turned a fatal option into the chosen one (%v)", got)
	}
	// It did push, though - it is not being ignored.
	if c.opts[1].util <= -500 {
		t.Fatalf("attacking scored %v, want the hint to have added to it", c.opts[1].util)
	}
}

// A hint only speaks about the move it is about, and only reads the situation
// it is about.
func TestAHintOnlyTouchesItsOwnMoveAndFeature(t *testing.T) {
	var f hintFeatures
	f[HintHunger] = 1
	f[HintCrowd] = 0.5
	hints := []Hint{
		{Feature: HintHunger, Act: ActEat, Weight: 10},
		{Feature: HintCrowd, Act: ActEat, Weight: 4},
		{Feature: HintHunger, Act: ActFlee, Weight: -8},
	}
	if got := f.score(hints, ActEat); got != 12 {
		t.Fatalf("eating scored %v of hint, want 10*1 + 4*0.5", got)
	}
	if got := f.score(hints, ActFlee); got != -8 {
		t.Fatalf("fleeing scored %v of hint, want -8", got)
	}
	if got := f.score(hints, ActRest); got != 0 {
		t.Fatalf("resting scored %v of hint, want nothing", got)
	}
}

// Everything a hint reads has to come out of Perception, and everything it
// reads is scaled to about 0..1 so that one range of weights does for all of
// them. A hint about a target says nothing when there is no target.
func TestHintFeaturesStayInRangeAndForgetTheTarget(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	p := &Perception{
		Cfg: &cfg, Rand: w.rng,
		Self: SelfView{
			Hunger: cfg.MaxHunger * 2, Vitality: -10, MaxVitality: cfg.MaxVitality,
			FoodScarcity: 99, Species: SpeciesHuman},
		Others: make([]AgentView, 40),
	}
	var f hintFeatures
	f.readSelf(p)
	f.readTarget(p, &AgentView{EstStrength: 1e6, Dist: -50, Affinity: 1e6, Species: SpeciesEnemy})
	for i, v := range f {
		if v < 0 || v > 1 {
			t.Fatalf("feature %v read %v, want it inside 0..1", HintFeature(i), v)
		}
	}
	f.readTarget(p, nil)
	for _, i := range []HintFeature{HintStrength, HintCloseness, HintTrust, HintOtherKind} {
		if f[i] != 0 {
			t.Fatalf("feature %v read %v with no target, want nothing", i, f[i])
		}
	}
}

// Room for ideas is bought out of the budget the body is built from, and it is
// bought whether or not the room is used. That is the whole economy of it: an
// agent carrying four ideas is smaller, slower or weaker than one carrying
// none.
func TestRoomForIdeasIsBoughtOutOfTheSameBudget(t *testing.T) {
	cfg := testConfig()
	cfg.InitialPopulation = 4000
	cfg.GeneBudgetStd = 0
	w := NewWorld(cfg)

	bySlots := map[int][]float64{}
	for i := range w.Agents() {
		a := &w.Agents()[i]
		bySlots[a.hintSlots] = append(bySlots[a.hintSlots], a.Budget())
	}
	if len(bySlots) < 2 {
		t.Fatalf("founders all bought the same amount of room (%v), want a range to compare", bySlots)
	}
	mean := func(xs []float64) float64 {
		s := 0.0
		for _, x := range xs {
			s += x
		}
		return s / float64(len(xs))
	}
	none, full := mean(bySlots[0]), mean(bySlots[cfg.HintSlots])
	want := cfg.HintSlotCost * float64(cfg.HintSlots)
	if got := none - full; math.Abs(got-want) > 1 {
		t.Fatalf("an agent with room for %d ideas is %v of budget smaller, want %v",
			cfg.HintSlots, got, want)
	}
}

// The two kinds of change to a hint are deliberately different sizes. A weight
// moves the way a gene does; what a hint is *about* can only change on the rare
// large event the world already has for that, a genius birth.
func TestOnlyAGeniusBirthChangesWhatAHintIsAbout(t *testing.T) {
	cfg := quietConfig()
	cfg.MutationRate = 0
	w := NewWorld(cfg)

	pa := &w.agents[w.addAgent(Agent{X: 10, Y: 10})-1]
	pb := &w.agents[w.addAgent(Agent{X: 12, Y: 10})-1]
	held := Hint{Feature: HintHunger, Act: ActEat, Weight: 5}
	pa.hints, pa.hintSlots = []Hint{held}, 1
	pb.hints, pb.hintSlots = []Hint{held}, 1

	for i := 0; i < 200; i++ {
		got := w.inheritHints(pa, pb, 1, false)
		if len(got) != 1 || got[0] != held {
			t.Fatalf("an ordinary birth changed a hint to %v, want %v", got, held)
		}
	}
	changed := 0
	for i := 0; i < 200; i++ {
		if got := w.inheritHints(pa, pb, 1, true); got[0].Feature != held.Feature || got[0].Act != held.Act {
			changed++
		}
	}
	if changed == 0 {
		t.Fatal("no genius birth produced a hint about anything new")
	}
}

// A weight does move on an ordinary birth, and it is kept inside the range the
// world was tuned in.
func TestHintWeightsMutateAndStayInRange(t *testing.T) {
	cfg := quietConfig()
	cfg.MutationRate = 1
	w := NewWorld(cfg)

	pa := &w.agents[w.addAgent(Agent{X: 10, Y: 10})-1]
	pb := &w.agents[w.addAgent(Agent{X: 12, Y: 10})-1]
	h := Hint{Feature: HintHunger, Act: ActEat, Weight: 0}
	pa.hints, pa.hintSlots = []Hint{h}, 1
	pb.hints, pb.hintSlots = []Hint{h}, 1

	moved := 0
	for i := 0; i < 500; i++ {
		got := w.inheritHints(pa, pb, 1, false)[0]
		if got.Weight != 0 {
			moved++
		}
		if math.Abs(got.Weight) > cfg.HintWeightMax {
			t.Fatalf("a weight reached %v, want it inside +/-%v", got.Weight, cfg.HintWeightMax)
		}
		pa.hints[0], pb.hints[0] = got, got
	}
	if moved == 0 {
		t.Fatal("no weight moved with the mutation rate at 1")
	}
}

// An idea is not a number two agents can average, so it is not met in the
// middle: it is copied, and only into an agent that paid for somewhere to put
// it.
func TestAnIdeaIsCopiedOnlyIntoRoomThatWasPaidFor(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)

	teacher := &w.agents[w.addAgent(Agent{X: 10, Y: 10})-1]
	teacher.hintSlots = 2
	teacher.hints = []Hint{
		{Feature: HintHunger, Act: ActEat, Weight: 7},
		{Feature: HintCrowd, Act: ActFlee, Weight: -3},
	}

	roomy := &w.agents[w.addAgent(Agent{X: 12, Y: 10})-1]
	roomy.hintSlots = 3
	if got := w.exchangeHints(roomy, teacher); got != 2 || len(roomy.hints) != 2 {
		t.Fatalf("an agent with room took on %d ideas and holds %d, want both", got, len(roomy.hints))
	}
	// Being taught the same thing twice uses no more room.
	if got := w.exchangeHints(roomy, teacher); got != 0 {
		t.Fatalf("the same ideas were taken on again (%d)", got)
	}

	full := &w.agents[w.addAgent(Agent{X: 14, Y: 10})-1]
	full.hintSlots = 1
	full.hints = []Hint{{Feature: HintWorn, Act: ActRest, Weight: 2}}
	if got := w.exchangeHints(full, teacher); got != 0 || len(full.hints) != 1 {
		t.Fatalf("an agent with no room took on %d ideas, want none", got)
	}

	none := &w.agents[w.addAgent(Agent{X: 16, Y: 10})-1]
	if got := w.exchangeHints(none, teacher); got != 0 {
		t.Fatalf("an agent that bought no room took on %d ideas, want none", got)
	}
}

// HintSlots 0 is the arm the stage is measured against: no room, no ideas, and
// nothing charged for them.
func TestHintSlotsZeroRemovesThemEntirely(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HintSlots = 0
	w := NewWorld(cfg)
	for i := 0; i < 2000; i++ {
		w.Step()
	}
	use := w.HintUse()
	if use.Slots != 0 || use.Held != 0 || use.Kinds != 0 {
		t.Fatalf("with no room the population carries %+v, want nothing", use)
	}
}
