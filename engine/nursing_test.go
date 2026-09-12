package engine

import (
	"math"
	"testing"
)

// A world that has not asked for it is untouched, down to the values taken
// from the random source.
func TestNoNursingSlowdownLeavesTheWorldExactlyAsItWas(t *testing.T) {
	run := func(share float64) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 109
		cfg.NursingSpeedShare = share
		w := NewWorld(cfg)
		for i := 0; i < 2000; i++ {
			w.Step()
		}
		return w.Stats()
	}
	plain := run(1)
	if plain != run(1) {
		t.Fatal("the same world twice gave different runs")
	}
	if slowed := run(0.5); slowed == plain {
		t.Fatal("slowing the mothers changed nothing at all")
	}
}

// The flag is on the mother, it is about this tick, and it goes off again the
// moment the child is out of the radius. The rule and its undoing are one
// distance check.
func TestAMotherIsSlowedOnlyWhileTheChildIsNear(t *testing.T) {
	cfg := quietConfig()
	cfg.NursingSpeedShare = 0.5
	w := NewWorld(cfg)

	idMother := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Sex: Female,
		Genome: filledGenome(50)})
	idChild := w.addAgent(Agent{Maturity: 0.5, X: 210, Y: 200, Sex: Male,
		Genome: filledGenome(50)})
	mother, child := mustAgent(t, w, idMother), mustAgent(t, w, idChild)
	child.GuardianID = mother.ID
	child.RearingTimer = 1000

	w.nurse()
	mother = mustAgent(t, w, idMother)
	if !mother.Nursing {
		t.Fatal("a mother with a child at her heel is not nursing")
	}
	if got, want := mother.speedNow(&w.cfg), mother.MaxSpeed(&w.cfg)*0.5; math.Abs(got-want) > 1e-9 {
		t.Fatalf("she moves at %v, want %v", got, want)
	}

	// Out of the radius, and she is herself again - the same check, read the
	// other way round.
	child = mustAgent(t, w, idChild)
	child.X = mother.X + w.cfg.RearingRadius + 10
	w.nurse()
	mother = mustAgent(t, w, idMother)
	if mother.Nursing {
		t.Fatal("a mother whose child has gone is still nursing")
	}
	if got := mother.speedNow(&w.cfg); math.Abs(got-mother.MaxSpeed(&w.cfg)) > 1e-9 {
		t.Fatalf("she moves at %v, want her own %v", got, mother.MaxSpeed(&w.cfg))
	}
	// And nobody else was touched.
	if mustAgent(t, w, idChild).Nursing {
		t.Fatal("the child is nursing")
	}
}

// The three readings of a speed have to agree: a mother who is actually
// slower but dodges and is perceived as if she were not would be three
// different bodies.
func TestTheSlowedMotherIsSlowEverywhereItIsRead(t *testing.T) {
	cfg := quietConfig()
	cfg.NursingSpeedShare = 0.5
	w := NewWorld(cfg)

	idMother := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Sex: Female,
		Genome: filledGenome(50)})
	idChild := w.addAgent(Agent{Maturity: 0.5, X: 205, Y: 200, Sex: Male,
		Genome: filledGenome(50)})
	mother := mustAgent(t, w, idMother)
	child := mustAgent(t, w, idChild)
	child.GuardianID = mother.ID
	child.RearingTimer = 1000

	// Dodging only exists in a fighting stance, so she is given one: evasion
	// is a channel of what a body is doing, not something it has lying about.
	mother.Action = Action{Kind: ActFlee, Effort: 1, Stance: StanceEvasive}
	quick := mustAgent(t, w, idMother).evasion(&w.cfg)
	quickSeen := w.selfView(mustAgent(t, w, idMother)).MaxSpeed

	w.nurse()
	mother = mustAgent(t, w, idMother)
	if slow := mother.evasion(&w.cfg); slow >= quick {
		t.Fatalf("her evasion is %v while nursing and %v otherwise", slow, quick)
	}
	if seen := w.selfView(mother).MaxSpeed; seen >= quickSeen {
		t.Fatalf("she knows herself as %v while nursing and %v otherwise", seen, quickSeen)
	}

	// And the step she actually takes is shorter.
	before := mother.X
	w.moveDir(mother, 1, 0, 1)
	slowStep := mother.X - before
	mother.Nursing = false
	before = mother.X
	w.moveDir(mother, 1, 0, 1)
	if quickStep := mother.X - before; slowStep >= quickStep {
		t.Fatalf("she stepped %v nursing and %v not", slowStep, quickStep)
	}
}

// Asking whether a child is still being reared must not age it: stillReared
// spends a tick of childhood as it answers, and this rule asks once a tick of
// its own.
func TestAskingAboutRearingDoesNotSpendIt(t *testing.T) {
	cfg := quietConfig()
	cfg.NursingSpeedShare = 0.5
	w := NewWorld(cfg)

	idMother := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Sex: Female,
		Genome: filledGenome(50)})
	idChild := w.addAgent(Agent{Maturity: 0.5, X: 205, Y: 200, Sex: Male,
		Genome: filledGenome(50)})
	child := mustAgent(t, w, idChild)
	child.GuardianID = idMother
	child.RearingTimer = 1000

	w.nurse()
	if got := mustAgent(t, w, idChild).RearingTimer; got != 1000 {
		t.Fatalf("asking cost the child %d ticks of childhood", 1000-got)
	}
}
