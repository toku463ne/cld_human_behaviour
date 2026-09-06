package engine

import (
	"math"
	"testing"
)

// Tests for a parent's meal going part way to the child it is rearing. The
// rule is off by default, so every one of these turns it on.

func feedingConfig(share float64) Config {
	cfg := growthConfig()
	cfg.ParentFeedShare = share
	cfg.SamenessPenalty = 0 // one discount at a time
	return cfg
}

// aParentAndItsChild puts the two of them on the same spot with a plant to eat.
func aParentAndItsChild(t *testing.T, cfg Config, apart float64) (*World, int, int, int) {
	t.Helper()
	w := NewWorld(cfg)
	parent := w.addAgent(Agent{X: 200, Y: 200, Maturity: 1, Vitality: 80, Hunger: 60,
		Genome: genomeOf(50, 50, 50)})
	child := w.addAgent(Agent{X: 200 + apart, Y: 200, Vitality: 40, Hunger: 60,
		Genome: genomeOf(50, 50, 50), GuardianID: parent, RearingTimer: 500})
	w.agentByID(parent).ChildIDs = []int{child}
	food := w.addFood(200, 200)
	return w, parent, child, food
}

// The mouthful is divided: the parent gets what it kept and the child the rest.
func TestAParentsMealGoesPartWayToItsChild(t *testing.T) {
	cfg := feedingConfig(0.5)
	w, parent, child, food := aParentAndItsChild(t, cfg, 10)

	before := mustAgent(t, w, parent).Hunger
	childBefore := mustAgent(t, w, child).Hunger
	w.eat(mustAgent(t, w, parent), food)

	gotParent := before - mustAgent(t, w, parent).Hunger
	gotChild := childBefore - mustAgent(t, w, child).Hunger
	want := cfg.FoodNutrition / 2
	if math.Abs(gotParent-want) > 1e-9 || math.Abs(gotChild-want) > 1e-9 {
		t.Fatalf("the meal went %v to the parent and %v to the child, want %v each",
			gotParent, gotChild, want)
	}
}

// Two children in its keeping split the child's half between them: what is
// divided is one mouthful, not one per child.
func TestTheChildrensShareIsOneShareBetweenThem(t *testing.T) {
	cfg := feedingConfig(0.5)
	w, parent, first, food := aParentAndItsChild(t, cfg, 10)
	second := w.addAgent(Agent{X: 205, Y: 200, Vitality: 40, Hunger: 60,
		Genome: genomeOf(50, 50, 50), GuardianID: parent, RearingTimer: 500})
	p := mustAgent(t, w, parent)
	p.ChildIDs = []int{first, second}

	w.eat(p, food)
	want := cfg.FoodNutrition / 4
	for _, id := range []int{first, second} {
		if got := 60 - mustAgent(t, w, id).Hunger; math.Abs(got-want) > 1e-9 {
			t.Fatalf("child #%d got %v of the meal, want %v", id, got, want)
		}
	}
}

// Nobody to feed, nothing given away: a child that has wandered out of the
// leash's own radius, or grown up, is fed by nobody.
func TestNothingIsHandedToAChildThatIsNotThere(t *testing.T) {
	cfg := feedingConfig(0.5)
	for _, c := range []struct {
		name  string
		apart float64
		grown bool
	}{
		{"too far away", cfg.RearingRadius + 20, false},
		{"grown up", 10, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			w, parent, child, food := aParentAndItsChild(t, cfg, c.apart)
			if c.grown {
				k := mustAgent(t, w, child)
				k.RearingTimer, k.GuardianID, k.Maturity = 0, 0, 1
			}
			before := mustAgent(t, w, parent).Hunger
			w.eat(mustAgent(t, w, parent), food)
			if got := before - mustAgent(t, w, parent).Hunger; math.Abs(got-cfg.FoodNutrition) > 1e-9 {
				t.Fatalf("the parent kept %v of the meal, want all %v", got, cfg.FoodNutrition)
			}
			if got := mustAgent(t, w, child).Hunger; got != 60 {
				t.Fatalf("the child was fed anyway: hunger %v", got)
			}
		})
	}
}

// The parent knows. A body whose meals are half meals has to be able to see
// that they are, or its utility formula is wrong about the quantity it is most
// sensitive to.
func TestANursingParentSeesHalfMeals(t *testing.T) {
	cfg := feedingConfig(0.5)
	w, parent, _, _ := aParentAndItsChild(t, cfg, 10)

	p := w.perceive(mustAgent(t, w, parent))
	if got := p.Self.Nutrition[FoodPlant]; math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("a nursing parent reckons a plant is worth %v of one, want 0.5", got)
	}

	// And an agent that is feeding nobody sees a whole one.
	alone := w.addAgent(Agent{X: 400, Y: 400, Maturity: 1, Vitality: 80, Hunger: 60,
		Genome: genomeOf(50, 50, 50)})
	p = w.perceive(mustAgent(t, w, alone))
	if got := p.Self.Nutrition[FoodPlant]; math.Abs(got-1) > 1e-9 {
		t.Fatalf("an agent with no children reckons a plant is worth %v of one, want 1", got)
	}
}

// With the rule off nothing about eating changes at all, which is what keeps
// every measurement made before it exists comparable.
func TestWithTheRuleOffTheParentKeepsTheWholeMeal(t *testing.T) {
	cfg := feedingConfig(0)
	w, parent, child, food := aParentAndItsChild(t, cfg, 10)
	before := mustAgent(t, w, parent).Hunger
	w.eat(mustAgent(t, w, parent), food)
	if got := before - mustAgent(t, w, parent).Hunger; math.Abs(got-cfg.FoodNutrition) > 1e-9 {
		t.Fatalf("the parent got %v of the meal, want all %v", got, cfg.FoodNutrition)
	}
	if got := mustAgent(t, w, child).Hunger; got != 60 {
		t.Fatalf("the child was fed with the rule off: hunger %v", got)
	}
}

// The control arms do what they say: the parent pays the same either way, and
// only one of them feeds anybody.
func TestTheControlsPayTheSameAndFeedNobody(t *testing.T) {
	cfg := feedingConfig(0.5)
	cfg.ParentFeedWasted = true
	w, parent, child, food := aParentAndItsChild(t, cfg, 10)

	before := mustAgent(t, w, parent).Hunger
	w.eat(mustAgent(t, w, parent), food)
	if got := before - mustAgent(t, w, parent).Hunger; math.Abs(got-cfg.FoodNutrition/2) > 1e-9 {
		t.Fatalf("the parent kept %v of the meal, want half of %v", got, cfg.FoodNutrition)
	}
	if got := mustAgent(t, w, child).Hunger; got != 60 {
		t.Fatalf("the child was fed by the wasted arm: hunger %v", got)
	}
}

// And the blind arm hides it from the parent without changing what it gets.
func TestABlindParentPlansAsThoughItAteTheLot(t *testing.T) {
	cfg := feedingConfig(0.5)
	cfg.ParentFeedKnown = false
	w, parent, child, food := aParentAndItsChild(t, cfg, 10)

	p := w.perceive(mustAgent(t, w, parent))
	if got := p.Self.Nutrition[FoodPlant]; math.Abs(got-1) > 1e-9 {
		t.Fatalf("a blind parent reckons a plant is worth %v of one, want 1", got)
	}
	childBefore := mustAgent(t, w, child).Hunger
	w.eat(mustAgent(t, w, parent), food)
	if got := childBefore - mustAgent(t, w, child).Hunger; math.Abs(got-cfg.FoodNutrition/2) > 1e-9 {
		t.Fatalf("the child got %v of the meal, want half of %v", got, cfg.FoodNutrition)
	}
}
