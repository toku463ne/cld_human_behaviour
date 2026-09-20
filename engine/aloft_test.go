package engine

import "testing"

// flierConfig is a flat world with one sort of enemy in it, which flies.
func flierConfig() Config {
	cfg := quietConfig()
	cfg.EnemyKinds = []EnemyKind{{Name: "flier", Share: 1, Flies: true}}
	return cfg
}

func TestAFlierIsInTheAirUntilItReachesForTheGround(t *testing.T) {
	w := NewWorld(flierConfig())
	a := mustAgent(t, w, w.addAgent(w.randomAgent(SpeciesEnemy)))
	if !w.aloft(a) {
		t.Fatal("a flier doing nothing in particular is not in the air")
	}
	for _, k := range []ActionKind{ActEat, ActAttack, ActTake} {
		a.Action.Kind = k
		if w.aloft(a) {
			t.Fatalf("a flier reaching for the ground with %v is still in the air", k)
		}
	}
	a.Action.Kind = ActMove
	if !w.aloft(a) {
		t.Fatal("a flier that went back to moving is not in the air")
	}
}

func TestOnlyAFlierIsEverInTheAir(t *testing.T) {
	cfg := flierConfig()
	w := NewWorld(cfg)
	h := mustAgent(t, w, w.addAgent(w.randomAgent(SpeciesHuman)))
	h.Action.Kind = ActMove
	if w.aloft(h) {
		t.Fatal("a human is in the air, so the first row of the table reached it")
	}

	cfg2 := quietConfig() // no kinds at all: the world before this rule
	w2 := NewWorld(cfg2)
	e := mustAgent(t, w2, w2.addAgent(w2.randomAgent(SpeciesEnemy)))
	e.Action.Kind = ActMove
	if w2.aloft(e) {
		t.Fatal("an ordinary enemy is in the air")
	}
}

func TestABlowDoesNotLandOnSomethingInTheAir(t *testing.T) {
	w := NewWorld(flierConfig())
	h := w.randomAgent(SpeciesHuman)
	h.X, h.Y = 100, 100
	human := mustAgent(t, w, w.addAgent(h))
	f := w.randomAgent(SpeciesEnemy)
	f.X, f.Y = 105, 100 // well inside CombatRadius
	flier := mustAgent(t, w, w.addAgent(f))
	flier.Action.Kind = ActMove // in the air

	human.Action = Action{Kind: ActAttack, TargetID: flier.ID, Effort: 1}
	w.attacks = w.attacks[:0]
	w.perform(human)
	if len(w.attacks) != 0 {
		t.Fatalf("%d blows landed on something in the air", len(w.attacks))
	}
	if !human.needsDecision {
		t.Fatal("the attacker was not asked to think again")
	}

	// And the moment it comes down, the same blow lands.
	flier.Action.Kind = ActEat
	human.Action = Action{Kind: ActAttack, TargetID: flier.ID, Effort: 1}
	w.attacks = w.attacks[:0]
	w.perform(human)
	if len(w.attacks) != 1 {
		t.Fatalf("%d blows landed on something that had come down", len(w.attacks))
	}
}

func TestAStoneReachesSomethingInTheAir(t *testing.T) {
	cfg := flierConfig()
	cfg.Throwing = true
	cfg.Stones = 0
	w := NewWorld(cfg)
	h := w.randomAgent(SpeciesHuman)
	h.X, h.Y = 100, 100
	human := mustAgent(t, w, w.addAgent(h))
	f := w.randomAgent(SpeciesEnemy)
	f.X, f.Y = 120, 100
	flier := mustAgent(t, w, w.addAgent(f))
	flier.Action.Kind = ActMove // in the air

	// A stone in hand, and a throw at it.
	id := w.putFood(Food{X: human.X, Y: human.Y, Kind: FoodStone})
	w.take(human, id)
	before := flier.Vitality
	human.Action = Action{Kind: ActThrow, TargetID: flier.ID, Effort: 1}
	w.perform(human)
	if flier.Vitality >= before && w.Stats().Throws == 0 {
		t.Fatal("the stone did not reach something in the air")
	}
}

func TestWhatIsInTheAirIsNotOfferedAsSomethingToHit(t *testing.T) {
	// The gate is on the candidates and not on the score: what a body cannot
	// do is not scored at all (stage 25's division).
	w := NewWorld(flierConfig())
	h := w.randomAgent(SpeciesHuman)
	h.X, h.Y = 100, 100
	human := mustAgent(t, w, w.addAgent(h))
	f := w.randomAgent(SpeciesEnemy)
	f.X, f.Y = 110, 100
	flier := mustAgent(t, w, w.addAgent(f))

	for _, up := range []bool{true, false} {
		if up {
			flier.Action.Kind = ActMove
		} else {
			flier.Action.Kind = ActEat
		}
		p := w.perceive(human)
		p.Trace = &DecisionTrace{}
		var other *AgentView
		for i := range p.Others {
			if p.Others[i].ID == flier.ID {
				other = &p.Others[i]
			}
		}
		if other == nil {
			t.Fatal("the flier is not in sight")
		}
		if other.Aloft != up {
			t.Fatalf("in the air = %v, want %v", other.Aloft, up)
		}
		c := &AIController{}
		c.Decide(p)
		hit := false
		for _, o := range p.Trace.Options {
			if o.Action.Kind == ActAttack && o.Action.TargetID == flier.ID {
				hit = true
			}
		}
		if hit == up {
			t.Fatalf("in the air = %v, attack offered = %v", up, hit)
		}
	}
}
