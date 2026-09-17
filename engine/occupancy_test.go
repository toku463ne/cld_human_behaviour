package engine

import "testing"

// Occupancy (#76): a meal takes time at the food, in the shape the call, the
// cry, the cooking and the writing already use. Nothing parallel is built for
// it - the same actionTicks counts it.
func TestAMealTakesTimeAtTheFood(t *testing.T) {
	cfg := quietConfig()
	cfg.EatTicks = 6
	w := NewWorld(cfg)

	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80,
		Hunger: 60, Genome: genomeOf(50, 50, 50)}))
	id := w.addFood(100, 100)
	a.Action = Action{Kind: ActEat, TargetID: id, Effort: 0.5}

	// Standing on it, but not fed until the time has been spent.
	before := a.Hunger
	for i := 0; i < cfg.EatTicks; i++ {
		a.actionTicks = i
		w.perform(a)
		if a.Hunger != before {
			t.Fatalf("fed after %d of %d ticks at the food", i, cfg.EatTicks)
		}
		if w.foodByID(id) == nil {
			t.Fatalf("the food went after %d of %d ticks", i, cfg.EatTicks)
		}
	}
	a.actionTicks = cfg.EatTicks
	w.perform(a)
	if a.Hunger >= before {
		t.Fatalf("still not fed after the whole %d ticks (hunger %v)", cfg.EatTicks, a.Hunger)
	}
}

// The time is spent at the food and not on the way to it, so a body that is
// still walking has not started.
func TestWalkingToAMealDoesNotCountAsEating(t *testing.T) {
	cfg := quietConfig()
	cfg.EatTicks = 6
	w := NewWorld(cfg)

	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80,
		Hunger: 60, Genome: genomeOf(50, 50, 50)}))
	id := w.addFood(400, 100)
	a.Action = Action{Kind: ActEat, TargetID: id, Effort: 0.5}

	for i := 0; i < 20; i++ {
		a.actionTicks = 99 // long past the gathering time, were it counting
		w.perform(a)
		if got := a.actionTicks; got != 0 {
			t.Fatalf("a body still walking has %d ticks of gathering behind it", got)
		}
	}
	if w.foodByID(id) == nil {
		t.Fatal("it ate from across the map")
	}
}

// With nothing asked for, a meal reached is a meal had, in the tick it was
// reached - and the walking body's counter is not touched either, so a world
// without the rule saves and reloads as it did.
func TestWithNoEatTicksAMealIsHadAtOnce(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)

	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80,
		Hunger: 60, Genome: genomeOf(50, 50, 50)}))
	id := w.addFood(100, 100)
	a.Action = Action{Kind: ActEat, TargetID: id, Effort: 0.5}
	a.actionTicks = 0
	before := a.Hunger
	w.perform(a)
	if a.Hunger >= before {
		t.Fatalf("a meal under the old rule was not had at once (hunger %v)", a.Hunger)
	}

	// And a body walking to one keeps whatever count it had.
	b := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 80,
		Hunger: 60, Genome: genomeOf(50, 50, 50)}))
	gid := w.addFood(400, 100)
	b.Action = Action{Kind: ActEat, TargetID: gid, Effort: 0.5}
	b.actionTicks = 7
	w.perform(b)
	if b.actionTicks != 7 {
		t.Fatalf("the walking body's counter moved to %d", b.actionTicks)
	}
}

// The body is told, so the extra time is in the utility as time and not as
// vitality: standing at a meal is not walking.
func TestTheBodyPricesTheTimeButNotTheWalking(t *testing.T) {
	cfg := quietConfig()
	cfg.JudgementNoise, cfg.ChoiceNoise = 0, 0
	w := NewWorld(cfg)

	meal := func(eatTicks int, known bool) (ticks, vit, time float64) {
		c := &AIController{}
		cf := cfg
		cf.EatTicks, cf.EatTicksKnown = eatTicks, known
		p := &Perception{
			Cfg: &cf,
			Self: SelfView{
				Intelligence: MaxAbility, NoiseWeight: 1,
				Vitality: 80, MaxVitality: 100, Hunger: 50,
				HungerRate: cf.HungerRate, MaxSpeed: cf.MaxSpeed,
				RestRate: cf.RegenRate,
			},
			Foods: []FoodView{{ID: 1, Dist: 100, Nutrition: 1}},
			Rand:  w.rng,
		}
		c.tracing = true
		c.addFood(p)
		for i := range c.opts {
			if c.opts[i].action.Kind == ActEat {
				u := c.terms[i]
				return u.Ticks, u.Vitality, u.TimeCost
			}
		}
		t.Fatal("no meal was scored")
		return
	}

	t0, v0, c0 := meal(0, true)
	t8, v8, c8 := meal(8, true)
	if t8-t0 != 7 {
		t.Fatalf("eight ticks of gathering added %v ticks, want 7 more than the one it always took", t8-t0)
	}
	if v8 != v0 {
		t.Fatalf("standing at a meal cost vitality: %v against %v", v8, v0)
	}
	if c8 <= c0 {
		t.Fatalf("the extra time cost nothing: %v against %v", c8, c0)
	}

	// And with the body not told, it prices the meal exactly as it always did
	// while the world holds it anyway.
	tb, vb, cb := meal(8, false)
	if tb != t0 || vb != v0 || cb != c0 {
		t.Fatalf("the blind arm priced the meal differently: %v/%v/%v against %v/%v/%v",
			tb, vb, cb, t0, v0, c0)
	}
}

// Picking a thing up is still one tick (#76): the gathering time must not ride
// along on the carrying, or there is no telling which of the two did whatever
// a measurement shows.
func TestPickingUpIsStillOneTick(t *testing.T) {
	cfg := carryConfig()
	cfg.JudgementNoise, cfg.ChoiceNoise = 0, 0
	w := NewWorld(cfg)

	take := func(eatTicks int) (ticks, vit float64) {
		c := &AIController{}
		cf := cfg
		cf.EatTicks = eatTicks
		p := &Perception{
			Cfg: &cf,
			Self: SelfView{
				Intelligence: MaxAbility, NoiseWeight: 1,
				Vitality: 80, MaxVitality: 100, Hunger: 50,
				HungerRate: cf.HungerRate, MaxSpeed: cf.MaxSpeed,
				RestRate: cf.RegenRate, CarryRoom: true,
			},
			Foods: []FoodView{{ID: 1, Dist: 100, Nutrition: 1}},
			Rand:  w.rng,
		}
		c.tracing = true
		c.addFood(p)
		for i := range c.opts {
			if c.opts[i].action.Kind == ActTake {
				return c.terms[i].Ticks, c.terms[i].Vitality
			}
		}
		t.Fatal("nothing was scored for picking it up")
		return
	}

	t0, v0 := take(0)
	t8, v8 := take(8)
	if t8 != t0 || v8 != v0 {
		t.Fatalf("the gathering time reached the carrying: %v/%v against %v/%v", t8, v8, t0, v0)
	}
}
