package engine

import "testing"

// Who a newborn keeps close to (stage 53a), and who may feed it (53b).

// pairAndBirth puts a couple on a spot, runs them to the end of their bond and
// returns the child. The two are made deliberately in the order that used to
// decide this, so that a rule reading the sexes is telling them apart.
func pairAndBirth(t *testing.T, cfg Config, firstIs Sex) (*World, *Agent) {
	t.Helper()
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, Sex: firstIs, X: 200, Y: 200,
		Vitality: 90, Genome: genomeOf(50, 50, 50)}))
	other := Male
	if firstIs == Male {
		other = Female
	}
	b := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, Sex: other, X: 202, Y: 200,
		Vitality: 90, Genome: genomeOf(50, 50, 50)}))
	a.PartnerID, b.PartnerID = b.ID, a.ID
	a.PairTimer, b.PairTimer = 1, 1
	w.tryBirth(a, b)
	if len(w.newborns) != 1 {
		t.Fatalf("%d newborns", len(w.newborns))
	}
	return w, &w.newborns[0]
}

// The mother rears, whichever of the two the world happened to reach first.
func TestTheMotherIsTheGuardian(t *testing.T) {
	for _, first := range []Sex{Male, Female} {
		cfg := growthConfig()
		cfg.GuardianIsMother = true
		w, child := pairAndBirth(t, cfg, first)
		g := mustAgent(t, w, child.GuardianID)
		if g.Sex != Female {
			t.Fatalf("with %v reached first, the child keeps to a %v", first, g.Sex)
		}
	}
}

// And the world before stage 53a is still there: whichever came first.
func TestTheOldRuleTookWhicheverCameFirst(t *testing.T) {
	for _, first := range []Sex{Male, Female} {
		cfg := growthConfig()
		cfg.GuardianIsMother = false
		w, child := pairAndBirth(t, cfg, first)
		if g := mustAgent(t, w, child.GuardianID); g.Sex != first {
			t.Fatalf("with %v reached first, the old rule gave the child a %v", first, g.Sex)
		}
	}
}

// Rearing says nothing about what being born cost: both parents still pay
// half of it (stage 53a changes who rears and nothing else).
func TestRearingDoesNotChangeWhatBirthCost(t *testing.T) {
	cfg := growthConfig()
	cfg.GuardianIsMother = true
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, Sex: Male, X: 200, Y: 200,
		Vitality: 90, Genome: genomeOf(50, 50, 50)}))
	b := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, Sex: Female, X: 202, Y: 200,
		Vitality: 90, Genome: genomeOf(50, 50, 50)}))
	a.PartnerID, b.PartnerID = b.ID, a.ID
	a.PairTimer, b.PairTimer = 1, 1
	before := [2]float64{a.Vitality, b.Vitality}
	w.tryBirth(a, b)
	if got := [2]float64{before[0] - a.Vitality, before[1] - b.Vitality}; got[0] != got[1] {
		t.Fatalf("the father paid %v and the mother %v", got[0], got[1])
	}
}

// The parent that is not the guardian feeds its child too, as long as it is
// there (stage 53b) - and does not, in the world where that rule is off.
func TestTheOtherParentFeedsItsChildToo(t *testing.T) {
	for _, tc := range []struct {
		name string
		kin  bool
		want bool
	}{{"by kin", true, true}, {"guardian only", false, false}} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := feedingConfig(0.5)
			cfg.ParentFeedByKin = tc.kin
			w := NewWorld(cfg)
			mother := w.addAgent(Agent{Maturity: 1, Sex: Female, X: 200, Y: 200,
				Vitality: 80, Hunger: 60, Genome: genomeOf(50, 50, 50)})
			father := w.addAgent(Agent{Maturity: 1, Sex: Male, X: 210, Y: 200,
				Vitality: 80, Hunger: 60, Genome: genomeOf(50, 50, 50)})
			child := w.addAgent(Agent{X: 205, Y: 200, Vitality: 40, Hunger: 60,
				Genome: genomeOf(50, 50, 50), GuardianID: mother, RearingTimer: 500})
			c := mustAgent(t, w, child)
			c.ParentIDs = [2]int{mother, father}
			mustAgent(t, w, mother).ChildIDs = []int{child}
			mustAgent(t, w, father).ChildIDs = []int{child}

			food := w.addFood(210, 200)
			before := c.Hunger
			w.eat(mustAgent(t, w, father), food)
			if fed := c.Hunger < before; fed != tc.want {
				t.Fatalf("the father ate and the child was fed: %v, want %v", fed, tc.want)
			}
		})
	}
}

// It is still only its own children, and only while they are small.
func TestNobodyFeedsSomebodyElsesChild(t *testing.T) {
	cfg := feedingConfig(0.5)
	w := NewWorld(cfg)
	stranger := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Vitality: 80,
		Hunger: 60, Genome: genomeOf(50, 50, 50)})
	mother := w.addAgent(Agent{Maturity: 1, Sex: Female, X: 260, Y: 200,
		Vitality: 80, Genome: genomeOf(50, 50, 50)})
	child := w.addAgent(Agent{X: 204, Y: 200, Vitality: 40, Hunger: 60,
		Genome: genomeOf(50, 50, 50), GuardianID: mother, RearingTimer: 500})
	mustAgent(t, w, child).ParentIDs = [2]int{mother, 0}
	mustAgent(t, w, mother).ChildIDs = []int{child}

	food := w.addFood(200, 200)
	before := mustAgent(t, w, child).Hunger
	w.eat(mustAgent(t, w, stranger), food)
	if mustAgent(t, w, child).Hunger < before {
		t.Fatal("a stranger's meal fed somebody else's child")
	}
}
