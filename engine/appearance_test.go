package engine

import (
	"math"
	"testing"
)

// What can be seen of a body is its size and its speed, expressed rather than
// inherited: a child looks like a child. What cannot be seen is how hard it
// hits.
func TestAppearanceIsBuildNotStrength(t *testing.T) {
	cfg := quietConfig()
	cfg.AppearanceNoise = 0
	w := NewWorld(cfg)
	genome := filledGenome(40)
	genome[GeneVitality], genome[GeneSpeed], genome[GeneAttack] = 80, 60, 100
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Genome: genome})
	a := w.agentByID(id)

	if got, want := a.Appearance(&cfg), 70.0; math.Abs(got-want) > 1e-9 {
		t.Errorf("appearance %.1f, want the average of build and speed (%.1f)", got, want)
	}

	// What shows is what it can do today, not what it inherited: a newborn
	// looks like a newborn.
	a.Maturity = 0
	if child := a.Appearance(&cfg); child >= 70 {
		t.Errorf("a child looks %.1f, want less than the %.1f a grown one does", child, 70.0)
	}
}

// An agent that has never taken a reading has nothing to go on and falls back
// to the flat prior. Once it has, it goes by its own line - and two agents
// that have seen different worlds guess differently about the same stranger.
func TestStrangersAreJudgedByWhatAnAgentHasSeen(t *testing.T) {
	cfg := quietConfig()
	cfg.AppearanceNoise = 0
	cfg.JudgementNoise = 0 // read the world exactly, so the line is exact
	cfg.AppearanceMinReads = 2
	w := NewWorld(cfg)

	weakWorld := w.agentByID(w.addAgent(Agent{Maturity: 1, X: 10, Y: 10, Genome: filledGenome(50)}))
	strongWorld := w.agentByID(w.addAgent(Agent{Maturity: 1, X: 20, Y: 20, Genome: filledGenome(50)}))

	// Two observers, same stranger, different experience.
	strangerGenome := filledGenome(50)
	strangerID := w.addAgent(Agent{Maturity: 1, X: 300, Y: 300, Genome: strangerGenome})

	teach := func(observerID int, attack float64, n int) {
		for i := 0; i < n; i++ {
			g := filledGenome(50)
			g[GeneAttack] = attack
			id := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Genome: g})
			w.observeStrength(w.agentByID(observerID), w.agentByID(id), 1)
			w.agentByID(id).Alive = false
		}
	}
	teach(weakWorld.ID, 10, 6)
	teach(strongWorld.ID, 90, 6)

	weakGuess := w.strangerStrength(w.agentByID(weakWorld.ID), strangerID)
	strongGuess := w.strangerStrength(w.agentByID(strongWorld.ID), strangerID)
	if weakGuess >= strongGuess {
		t.Errorf("the one that only ever met weaklings guesses %.1f and the one among the strong %.1f: experience is not telling",
			weakGuess, strongGuess)
	}
	if math.Abs(weakGuess-10) > 5 || math.Abs(strongGuess-90) > 5 {
		t.Errorf("guesses %.1f / %.1f, want near the strengths each has actually seen (10 / 90)", weakGuess, strongGuess)
	}

	// With the rule off, everybody assumes the same thing about everybody.
	cfg.LearnFromLooks = false
	w2 := NewWorld(cfg)
	obs := w2.agentByID(w2.addAgent(Agent{Maturity: 1, X: 10, Y: 10, Genome: filledGenome(50)}))
	other := w2.addAgent(Agent{Maturity: 1, X: 20, Y: 20, Genome: filledGenome(90)})
	if got := w2.strangerStrength(obs, other); got != cfg.PriorStrength {
		t.Errorf("with the learning off a stranger is worth %.1f, want the flat prior %.1f", got, cfg.PriorStrength)
	}
}

// A body that is bigger really does tell an agent something, if the world it
// has seen made it so: an observer taught that big means strong expects more
// of a big stranger than of a small one.
func TestABuildIsWorthSomethingOnceItsMeaningIsLearned(t *testing.T) {
	cfg := quietConfig()
	cfg.AppearanceNoise, cfg.JudgementNoise = 0, 0
	cfg.AppearanceMinReads = 2
	w := NewWorld(cfg)
	obsID := w.addAgent(Agent{Maturity: 1, X: 10, Y: 10, Genome: filledGenome(50)})

	// A world in which the big ones hit hard and the small ones do not.
	for _, pair := range [][2]float64{{20, 20}, {80, 80}, {30, 30}, {70, 70}} {
		g := filledGenome(50)
		g[GeneVitality], g[GeneSpeed] = pair[0], pair[0]
		g[GeneAttack] = pair[1]
		id := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Genome: g})
		w.observeStrength(w.agentByID(obsID), w.agentByID(id), 1)
	}

	small := filledGenome(50)
	small[GeneVitality], small[GeneSpeed] = 20, 20
	big := filledGenome(50)
	big[GeneVitality], big[GeneSpeed] = 80, 80
	smallID := w.addAgent(Agent{Maturity: 1, X: 300, Y: 300, Genome: small})
	bigID := w.addAgent(Agent{Maturity: 1, X: 310, Y: 300, Genome: big})

	guessSmall := w.strangerStrength(w.agentByID(obsID), smallID)
	guessBig := w.strangerStrength(w.agentByID(obsID), bigID)
	if guessBig <= guessSmall {
		t.Errorf("a big stranger is guessed at %.1f and a small one at %.1f: the build is being ignored", guessBig, guessSmall)
	}

	// Flatten the line and the same agent stops telling them apart, while
	// still knowing roughly what the world holds.
	cfg2 := cfg
	cfg2.LooksSlope = false
	w.cfg = cfg2
	if a, b := w.strangerStrength(w.agentByID(obsID), smallID), w.strangerStrength(w.agentByID(obsID), bigID); a != b {
		t.Errorf("with the slope off a small stranger is %.1f and a big one %.1f, want the same guess", a, b)
	}
}

// The impression is not a memory of anybody: an agent whose memory is full
// still learns what a body of that size is worth, and still judges the next
// stranger by it.
func TestAnImpressionSurvivesAFullMemory(t *testing.T) {
	cfg := quietConfig()
	cfg.AppearanceNoise, cfg.JudgementNoise = 0, 0
	cfg.MemoryCapacity, cfg.AppearanceMinReads = 1, 1
	w := NewWorld(cfg)
	obsID := w.addAgent(Agent{Maturity: 1, X: 10, Y: 10, Genome: filledGenome(50)})

	// Fill the one slot with somebody who matters, so nobody else can get in.
	keeperID := w.addAgent(Agent{Maturity: 1, X: 20, Y: 20, Genome: filledGenome(50)})
	w.rememberDamage(w.agentByID(obsID), keeperID, 30)

	g := filledGenome(50)
	g[GeneAttack] = 10
	otherID := w.addAgent(Agent{Maturity: 1, X: 30, Y: 30, Genome: g})
	w.observeStrength(w.agentByID(obsID), w.agentByID(otherID), 1)

	if op := w.agentByID(obsID).opinion(otherID); op != nil {
		t.Fatal("the full memory took a record on anyway; this test is not testing what it means to")
	}
	if n := int(w.agentByID(obsID).looks.n); n != 1 {
		t.Fatalf("readings folded into the impression = %d, want 1: a full memory stopped it learning", n)
	}
	strangerID := w.addAgent(Agent{Maturity: 1, X: 40, Y: 40, Genome: filledGenome(50)})
	if got := w.strangerStrength(w.agentByID(obsID), strangerID); math.Abs(got-10) > 1e-6 {
		t.Errorf("guess %.1f, want the 10 it has actually seen", got)
	}
}

// The world counts how far out these assumptions are, and only counts them
// once per stranger.
func TestFirstSightErrorIsMeasured(t *testing.T) {
	cfg := quietConfig()
	cfg.LearnFromLooks = false
	w := NewWorld(cfg)
	obsID := w.addAgent(Agent{Maturity: 1, X: 10, Y: 10, Genome: filledGenome(50)})
	g := filledGenome(50)
	g[GeneAttack] = 20
	otherID := w.addAgent(Agent{Maturity: 1, X: 20, Y: 20, Genome: g})

	w.rememberDamage(w.agentByID(obsID), otherID, 5)
	w.rememberDamage(w.agentByID(obsID), otherID, 5)

	s := w.Stats()
	if s.FirstSights != 1 {
		t.Fatalf("first sights = %d, want 1: the same stranger was counted twice", s.FirstSights)
	}
	if want := math.Abs(cfg.PriorStrength - 20); math.Abs(s.FirstSightError-want) > 1e-9 {
		t.Errorf("first sight error %.2f, want %.2f", s.FirstSightError, want)
	}
	if s.FirstSightsLearned != 0 {
		t.Error("an agent with the learning off was counted as going by its own line")
	}
}
