package engine

import "testing"

// witnessWorld is a still world big enough that two agents can be out of sight
// of each other, with the population and the food it grows left out.
func witnessWorld(t *testing.T) *World {
	t.Helper()
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	return NewWorld(cfg)
}

func witnessAgent(w *World, x, y float64, species Species) int {
	id := w.addAgent(Agent{
		Maturity: 1, X: x, Y: y, Vitality: 80, Species: species,
		Genome: genomeOf(50, 50, 50),
	})
	w.agentByID(id).Species = species
	return id
}

// killed makes one agent the killer of another and runs the death, the way a
// fight ending would.
func killed(w *World, victimID, killerID int) {
	v := w.agentByID(victimID)
	v.noteHit(killerID, w.tick)
	v.lastAttackTick = w.tick
	w.kill(v)
}

// Somebody who watched a killing knows something about whoever did it, and
// somebody standing too far off knows nothing.
func TestAKillingTeachesThoseWhoSawIt(t *testing.T) {
	w := witnessWorld(t)
	killer := witnessAgent(w, 100, 100, SpeciesHuman)
	victim := witnessAgent(w, 110, 100, SpeciesHuman)
	near := witnessAgent(w, 130, 110, SpeciesHuman)
	far := witnessAgent(w, 700, 500, SpeciesHuman)

	killed(w, victim, killer)

	if op := w.agentByID(near).opinion(killer); op == nil || op.Samples == 0 {
		t.Fatalf("the onlooker came away with %+v, want a reading of the killer", op)
	}
	if op := w.agentByID(far).opinion(killer); op != nil {
		t.Fatalf("an agent on the far side of the world learned %+v", op)
	}
	if op := w.agentByID(killer).opinion(killer); op != nil {
		t.Fatal("the killer was told about itself")
	}
	if w.Stats().KillWitnesses != 1 {
		t.Fatalf("%d readings counted, want 1", w.Stats().KillWitnesses)
	}
}

// Killing one of another kind is the other half of the same path: the
// onlookers of the killer's own kind think better of it, and nobody takes a
// reading.
func TestKillingAnotherKindEarnsAffinityFromItsOwn(t *testing.T) {
	w := witnessWorld(t)
	killer := witnessAgent(w, 100, 100, SpeciesHuman)
	victim := witnessAgent(w, 110, 100, SpeciesEnemy)
	fellow := witnessAgent(w, 130, 110, SpeciesHuman)
	stranger := witnessAgent(w, 130, 90, SpeciesEnemy)

	killed(w, victim, killer)

	op := w.agentByID(fellow).opinion(killer)
	if op == nil || op.Affinity <= 0 {
		t.Fatalf("one of the killer's own came away with %+v, want affinity", op)
	}
	if op.Samples != 0 {
		t.Fatalf("it also took a reading (%d): the two halves are one path, not both", op.Samples)
	}
	// The other kind watched somebody kill one of theirs, which is a fact
	// about how dangerous that somebody is.
	if op := w.agentByID(stranger).opinion(killer); op == nil || op.Samples == 0 {
		t.Fatalf("the other kind came away with %+v, want a reading", op)
	}
	if s := w.Stats(); s.AvengeWitnesses != 1 || s.KillWitnesses != 1 {
		t.Fatalf("counted %d avenged and %d read, want 1 and 1", s.AvengeWitnesses, s.KillWitnesses)
	}
}

// A death that nobody caused is nobody's doing: starving to death in front of
// a crowd teaches them nothing about anybody.
func TestOnlyKillingsAreWitnessed(t *testing.T) {
	w := witnessWorld(t)
	victim := witnessAgent(w, 110, 100, SpeciesHuman)
	near := witnessAgent(w, 130, 110, SpeciesHuman)

	w.kill(w.agentByID(victim))

	if op := w.agentByID(near).opinion(victim); op != nil {
		t.Fatalf("watching somebody die of hunger taught %+v", op)
	}
	if w.Stats().KillWitnesses != 0 {
		t.Fatal("a death with no killer was counted as a killing seen")
	}
}

// The reading is coarser than a look somebody paid for (#55): the higher the
// factor, the less an onlooker's estimate firms up.
func TestAWitnessedKillingIsWorthLessThanAPaidLook(t *testing.T) {
	variance := func(factor float64) float64 {
		cfg := quietConfig()
		cfg.Width, cfg.Height = 800, 600
		cfg.KillWitnessFactor = factor
		w := NewWorld(cfg)
		killer := witnessAgent(w, 100, 100, SpeciesHuman)
		victim := witnessAgent(w, 110, 100, SpeciesHuman)
		near := witnessAgent(w, 130, 110, SpeciesHuman)
		killed(w, victim, killer)
		op := w.agentByID(near).opinion(killer)
		if op == nil {
			t.Fatalf("no reading at factor %v", factor)
		}
		return op.Variance
	}
	paid, coarse := variance(1), variance(2)
	if !(coarse > paid) {
		t.Fatalf("a witnessed killing left the estimate at %v against a paid look's %v: it must teach less", coarse, paid)
	}
}

// With both weights at zero nothing is learned, and the chances the rule had
// are counted all the same - a rule that hardly ever fires explains nothing
// whatever its weight, so the count has to survive the arm that turns it off.
func TestWitnessedKillingsAreCountedEvenWithTheRuleOff(t *testing.T) {
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	cfg.KillWitnessFactor, cfg.AffinityWitnessKill = 0, 0
	w := NewWorld(cfg)
	killer := witnessAgent(w, 100, 100, SpeciesHuman)
	victim := witnessAgent(w, 110, 100, SpeciesHuman)
	near := witnessAgent(w, 130, 110, SpeciesHuman)

	killed(w, victim, killer)

	if op := w.agentByID(near).opinion(killer); op != nil {
		t.Fatalf("the rule is off and the onlooker still learned %+v", op)
	}
	if w.Stats().KillWitnesses != 1 {
		t.Fatalf("%d chances counted, want 1", w.Stats().KillWitnesses)
	}
}
