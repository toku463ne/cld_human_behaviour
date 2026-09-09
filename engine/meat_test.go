package engine

import (
	"math"
	"testing"
)

// putMeat leaves one item of meat nobody has a claim on, at a place.
func putMeat(w *World, x, y float64) int {
	id := w.putFood(Food{X: x, Y: y, Kind: FoodMeat, From: Species(9)})
	return id
}

// The arm everything before this stage was measured in: carcasses mend
// nothing, and eating one is the hunger rule and nothing else.
func TestWithoutTheRuleAMouthfulOfMeatMendsNothing(t *testing.T) {
	cfg := quietConfig()
	cfg.MeatVitality = 0
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 40,
		Hunger: 90, Genome: genomeOf(50, 50, 50)}))

	before := a.Vitality
	w.eat(a, putMeat(w, 101, 100))
	if a.Vitality != before {
		t.Fatalf("with MeatVitality off a carcass moved vitality from %v to %v", before, a.Vitality)
	}
	if w.Stats().MeatHealing != 0 {
		t.Fatalf("nothing mended, but the world counted %v", w.Stats().MeatHealing)
	}
}

// With the rule on, one item puts back a share of the eater's own ceiling -
// its own, not the world's, so a small body is not mended by the amount a
// large one would be.
func TestACarcassMendsAShareOfTheEatersOwnCeiling(t *testing.T) {
	cfg := quietConfig()
	cfg.MeatVitality = 0.5
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 10,
		Hunger: 90, Genome: genomeOf(50, 50, 50)}))

	want := 0.5 * a.MaxVitality(&cfg)
	before := a.Vitality
	w.eat(a, putMeat(w, 101, 100))
	approx(t, a.Vitality-before, want, 1e-9, "vitality a carcass put back")
	approx(t, w.Stats().MeatHealing, want, 1e-9, "vitality the world counted as mended")
}

// It stops at the ceiling. This is the whole of "save it for later": a body
// that is nearly whole gets almost nothing out of a carcass, and nothing
// anywhere says so.
func TestMendingStopsAtWhatTheBodyIsMissing(t *testing.T) {
	cfg := quietConfig()
	cfg.MeatVitality = 1
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Hunger: 90,
		Genome: genomeOf(50, 50, 50)}))
	max := a.MaxVitality(&cfg)
	a.Vitality = max - 3

	w.eat(a, putMeat(w, 101, 100))
	approx(t, a.Vitality, max, 1e-9, "vitality after a whole carcass")
	approx(t, w.Stats().MeatHealing, 3, 1e-9, "vitality counted as mended")
}

// Mending carries the same discount the hunger side carries. Being sick of
// something is about the mouthful, not about the stomach.
func TestAMouthfulYouAreSickOfMendsLess(t *testing.T) {
	cfg := quietConfig()
	cfg.MeatVitality = 0.4
	w := NewWorld(cfg)

	mended := func(prime int) float64 {
		a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 5,
			Hunger: 90, Genome: genomeOf(50, 50, 50)}))
		for i := 0; i < prime; i++ {
			w.noteEaten(a, FoodMeat)
		}
		before := a.Vitality
		w.eat(a, putMeat(w, 101, 100))
		return a.Vitality - before
	}

	fresh, sickOfIt := mended(0), mended(12)
	if sickOfIt >= fresh {
		t.Fatalf("a carcass mended %v for a body living on meat and %v for a fresh one", sickOfIt, fresh)
	}
	if sickOfIt <= 0 {
		t.Fatalf("a carcass mended %v; being sick of meat is not a reason to be mended by nothing", sickOfIt)
	}
}

// The control the stage needs: a carcass that fills a stomach by more and
// mends nothing. Without it, "meat became nourishing" and "meat became
// medicine" cannot be told apart.
func TestFillingAndMendingAreSeparateLevers(t *testing.T) {
	cfg := quietConfig()
	cfg.MeatNutrition, cfg.MeatVitality = 2, 0
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 40,
		Hunger: 90, Genome: genomeOf(50, 50, 50)}))

	before := a.Vitality
	hunger := a.Hunger
	w.eat(a, putMeat(w, 101, 100))
	approx(t, hunger-a.Hunger, 2*cfg.FoodNutrition, 1e-9, "hunger a doubled carcass took away")
	if a.Vitality != before {
		t.Fatalf("a carcass that only fills a stomach moved vitality from %v to %v", before, a.Vitality)
	}
	// And a plant is untouched by the meat figure.
	b := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 300, Y: 300, Vitality: 40,
		Hunger: 90, Genome: genomeOf(50, 50, 50)}))
	hunger = b.Hunger
	w.eat(b, w.addFood(301, 300))
	approx(t, hunger-b.Hunger, cfg.FoodNutrition, 1e-9, "hunger a plant took away")
}

// What the agent is told matches what the world will do to it. Nothing here
// is hidden: lifespan is the one deliberate exception and this is not it.
func TestPerceptionSaysExactlyWhatACarcassWouldMend(t *testing.T) {
	cfg := quietConfig()
	cfg.MeatVitality = 0.35
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 10,
		Hunger: 90, Genome: genomeOf(50, 50, 50)}))
	id := putMeat(w, 105, 100)

	p := w.perceive(a)
	if len(p.Foods) != 1 {
		t.Fatalf("the agent sees %d items, want the one carcass", len(p.Foods))
	}
	told := p.Foods[0].Heal
	if told != p.Self.Heal[FoodMeat] {
		t.Fatalf("the item says %v and the body says %v", told, p.Self.Heal[FoodMeat])
	}
	if p.Self.Heal[FoodPlant] != 0 {
		t.Fatalf("a plant claims to mend %v", p.Self.Heal[FoodPlant])
	}
	before := a.Vitality
	w.eat(a, id)
	approx(t, a.Vitality-before, told, 1e-9, "what eating it actually mended")
}

// And it reaches the comparison: a battered body scores a carcass above a
// plant, a whole one does not. Nobody wrote a rule about hoarding.
func TestAWholeBodyScoresACarcassBelowAPlant(t *testing.T) {
	cfg := testConfig()
	cfg.MeatVitality = 1

	score := func(vitality float64) (meat, plant float64) {
		s := SelfView{
			ID: 1, X: 100, Y: 100, Vitality: vitality, Hunger: 60,
			MaxVitality: cfg.MaxVitality, MaxSpeed: cfg.MaxSpeed,
			HungerRate: cfg.HungerRate, RestRate: cfg.RegenRate,
			Retaliation: cfg.Retaliation, AcceptChance: cfg.AcceptChance,
			RiskWeight: cfg.RiskWeight, CompetitionWeight: cfg.CompetitionWeight,
			ShockRisk: cfg.ShockRisk,
			Nutrition: [NumFoodKinds]float64{1, 1},
			Heal:      [NumFoodKinds]float64{0, cfg.MeatVitality * cfg.MaxVitality},
		}
		p := &Perception{Tick: 1, Cfg: &cfg, Self: s, Foods: []FoodView{
			{ID: 1, Dist: 20, Kind: FoodMeat, Nutrition: 1, Heal: s.Heal[FoodMeat], RivalDist: math.Inf(1)},
			{ID: 2, Dist: 20, Kind: FoodPlant, Nutrition: 1, RivalDist: math.Inf(1)},
		}}
		c := &AIController{}
		c.addFood(p)
		best := map[int]float64{1: math.Inf(-1), 2: math.Inf(-1)}
		for _, o := range c.opts {
			if o.action.Kind == ActEat && o.util > best[o.action.TargetID] {
				best[o.action.TargetID] = o.util
			}
		}
		return best[1], best[2]
	}

	meatHurt, plantHurt := score(20)
	if meatHurt <= plantHurt {
		t.Fatalf("a battered body scores the carcass %v and the plant %v", meatHurt, plantHurt)
	}
	meatWhole, plantWhole := score(cfg.MaxVitality)
	if meatWhole > plantWhole {
		t.Fatalf("a whole body still scores the carcass %v over the plant %v: the ceiling is not biting",
			meatWhole, plantWhole)
	}
}

// A share of a mouthful is a share of everything the mouthful does. The
// children a parent is feeding are mended by their part of it, exactly as
// they are fed and poisoned by their part of it.
func TestAChildIsMendedByItsShareOfTheMouthful(t *testing.T) {
	cfg := quietConfig()
	cfg.MeatVitality = 0.5
	cfg.ParentFeedShare = 0.5
	w := NewWorld(cfg)
	parent := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 10,
		Hunger: 90, Genome: genomeOf(50, 50, 50)}))
	child := mustAgent(t, w, w.addAgent(Agent{X: 105, Y: 100, Vitality: 5, Hunger: 90,
		Genome: genomeOf(50, 50, 50), GuardianID: parent.ID,
		RearingTimer: cfg.ChildRearingTicks}))
	parent.ChildIDs = append(parent.ChildIDs, child.ID)

	pBefore, cBefore := parent.Vitality, child.Vitality
	w.eat(parent, putMeat(w, 101, 100))

	wantParent := 0.5 * 0.5 * parent.MaxVitality(&cfg)
	wantChild := 0.5 * 0.5 * child.MaxVitality(&cfg)
	approx(t, parent.Vitality-pBefore, wantParent, 1e-9, "what the parent kept")
	approx(t, child.Vitality-cBefore, wantChild, 1e-9, "what the child was given")
}

// Counting before changing anything (the habit stage 24 paid for). These are
// read-only tallies, and they say how much of the meat the world makes is
// eaten and how much of it rots.
func TestTheWorldCountsWhatBecomesOfTheMeat(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 40,
		Hunger: 90, Genome: genomeOf(50, 50, 50)}))
	dead := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 400, Y: 400, Vitality: 1,
		Genome: filledGenome(50), Species: Species(9)}))
	w.kill(dead)

	st := w.Stats()
	if st.MeatDropped == 0 {
		t.Fatal("a carcass left nothing, so there is nothing to count")
	}
	w.eat(a, putMeat(w, 101, 100))
	w.eat(a, w.addFood(102, 100))
	if got := w.Stats().MeatEaten; got != 1 {
		t.Fatalf("meat eaten = %d, want 1", got)
	}
	if got := w.Stats().PlantsEaten; got != 1 {
		t.Fatalf("plants eaten = %d, want 1", got)
	}
	// And what nobody gets to.
	before := w.Stats().MeatSpoiled
	w.tick += cfg.MeatSpoilTicks + 1
	w.clearSpoiled()
	if w.Stats().MeatSpoiled <= before {
		t.Fatalf("meat spoiled stayed at %d after the carcass timed out", before)
	}
}
