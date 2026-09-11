package engine

import "testing"

// cookConfig is a still world where hands hold one thing and preparing it is
// worth half a body.
func cookConfig() Config {
	cfg := quietConfig()
	cfg.CarryCapacity = 1
	cfg.CookTicks = 20
	cfg.CookVitality = 0.5
	cfg.CookQuality = 1
	return cfg
}

// cooking takes its ticks and then changes the thing in the hand, and nothing
// about the world's stock of food moves.
func TestCookingTakesItsTicksAndMakesTheThing(t *testing.T) {
	w := NewWorld(cookConfig())
	id := holding(t, w, 100, 100)
	a := mustAgent(t, w, id)
	before := w.countKind(FoodPlant) + w.heldKind[FoodPlant]

	a.Action = Action{Kind: ActCook}
	for i := 0; i < w.cfg.CookTicks; i++ {
		w.perform(a)
		if a.Carrying()[0].Cooked != 0 {
			t.Fatalf("it was done after %d ticks of %d", i, w.cfg.CookTicks)
		}
		a.actionTicks++
	}
	w.perform(a)
	if got := a.Carrying()[0].Cooked; got != 1 {
		t.Fatalf("cooked to %v, want 1", got)
	}
	if w.Cooking().Cooked != 1 {
		t.Fatalf("%d cookings counted", w.Cooking().Cooked)
	}
	if got := w.countKind(FoodPlant) + w.heldKind[FoodPlant]; got != before {
		t.Fatalf("the world holds %d plants and held %d: cooking made or lost one", got, before)
	}
}

// What it buys: a cooked plant mends, and an uncooked one never has.
func TestACookedMealMendsAndARawOneDoesNot(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cooked float64
		want   bool
	}{{"raw", 0, false}, {"cooked", 1, true}} {
		t.Run(tc.name, func(t *testing.T) {
			w := NewWorld(cookConfig())
			id := holding(t, w, 100, 100)
			a := mustAgent(t, w, id)
			a.Vitality = 10
			a.carried[0].Cooked = tc.cooked
			before := a.Vitality
			w.eatCarried(a, a.carried[0].ID)
			if got := a.Vitality > before; got != tc.want {
				t.Fatalf("vitality went %v -> %v, mended=%v want %v",
					before, a.Vitality, got, tc.want)
			}
		})
	}
}

// The two reasons a thing mends do not stack: cooked meat is worth the better
// of the two and not the sum.
func TestCookingAndFleshDoNotStack(t *testing.T) {
	cfg := cookConfig()
	cfg.MeatVitality = 0.5
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100,
		Vitality: 10, Genome: genomeOf(50, 50, 50)}))
	raw := Food{Kind: FoodMeat, From: SpeciesEnemy}
	cooked := Food{Kind: FoodMeat, From: SpeciesEnemy, Cooked: 1}
	rh, ch := w.itemHeal(a, &raw), w.itemHeal(a, &cooked)
	if rh <= 0 || ch != rh {
		t.Fatalf("raw meat mends %v and cooked meat %v: they should be the same "+
			"while the two figures are equal", rh, ch)
	}
	// And with the cooking worth more, the better of the two wins - never the
	// sum of them.
	w.cfg.CookVitality = 0.8
	if got, want := w.itemHeal(a, &cooked), 0.8/0.5*rh; got != want {
		t.Fatalf("cooked meat mends %v, want %v (the better figure, not the sum)", got, want)
	}
}

// Cooking is not a gate: what has not been cooked is still a meal.
func TestNothingIsInedibleRaw(t *testing.T) {
	w := NewWorld(cookConfig())
	id := holding(t, w, 100, 100)
	a := mustAgent(t, w, id)
	a.Hunger = 60
	before := a.Hunger
	w.eatCarried(a, a.carried[0].ID)
	if a.Hunger >= before {
		t.Fatalf("a raw plant took hunger from %v to %v", before, a.Hunger)
	}
}

// Being good at it is worth something, and it is a yield and not a speed: the
// same ticks, a better result.
func TestSkillMakesTheResultBetterAndNotFaster(t *testing.T) {
	cfg := cookConfig()
	cfg.CookQuality = 0.4
	cfg.SkillCookRelief = 1
	w := NewWorld(cfg)
	dull := mustAgent(t, w, holding(t, w, 100, 100))
	keen := mustAgent(t, w, holding(t, w, 300, 300))
	keen.hints = append(keen.hints, Hint{Skill: SkillCook, Mastery: 1})
	if got, want := w.cookQuality(dull), 0.4; got != want {
		t.Fatalf("a body that knows nothing cooks at %v, want %v", got, want)
	}
	if got := w.cookQuality(keen); got <= w.cookQuality(dull) {
		t.Fatalf("knowing how is worth %v against %v", got, w.cookQuality(dull))
	}
	// The ceiling is the body's own gene, not the figure it holds (#63).
	keen.Genome[GeneIntelligence] = 0
	if got, want := w.cookQuality(keen), w.cookQuality(dull); got != want {
		t.Fatalf("a body with nothing behind it cooks at %v, want %v", got, want)
	}
}

// A body that would gain nothing does not do it, and a body that would does.
// No threshold anywhere: the comparison decides (the principle of stage 25).
func TestWhetherToCookIsScoredAndNotGated(t *testing.T) {
	// Whole, and with nothing to mend there is nothing cooking can buy;
	// battered, and there is. No threshold anywhere - the comparison decides.
	for _, tc := range []struct {
		name  string
		share float64
		want  bool
	}{{"whole", 1, false}, {"battered", 0.2, true}} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := cookConfig()
			cfg.Seed = 7
			cfg.ChoiceNoise = 0
			w := NewWorld(cfg)
			id := holding(t, w, 100, 100)
			a := mustAgent(t, w, id)
			a.Vitality = tc.share * a.MaxVitality(&w.cfg)
			tr := decideWithTrace(t, w, id)
			got := false
			for _, o := range tr.Options {
				if o.Action.Kind == ActCook && o.Utility.Total() > 0 {
					got = true
				}
			}
			if got != tc.want {
				t.Fatalf("cooking was worth something: %v, want %v", got, tc.want)
			}
		})
	}
}

// The placebo: cooking worth nothing is the world without this stage, bit for
// bit. It is the arm stage 50 showed the measurement stands on, and it only
// means anything if no random number is drawn either way.
func TestCookingWorthNothingIsTheWorldWithoutIt(t *testing.T) {
	run := func(f func(*Config)) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 3
		f(&cfg)
		w := NewWorld(cfg)
		for i := 0; i < 3000; i++ {
			w.Step()
		}
		return w.Stats()
	}
	off := run(func(c *Config) { c.CookTicks = 0 })
	placebo := run(func(c *Config) { c.CookVitality = 0 })
	if off != placebo {
		t.Fatalf("a world where cooking is worth nothing is not the world with no\n"+
			"cooking in it:\n no cooking %+v\n placebo   %+v", off, placebo)
	}
}
