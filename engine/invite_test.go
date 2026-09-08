package engine

import (
	"math"
	"testing"
)

// inviteWorld is a still world with a hungry human, a big animal worth taking
// on, and room to put other people in.
func inviteWorld(t *testing.T) (*World, int, int) {
	t.Helper()
	cfg := testConfig()
	cfg.Width, cfg.Height = 800, 600
	w := NewWorld(cfg)
	hunter := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 90, Hunger: 70,
		Genome: genomeOf(40, 100, 100)})
	// Something too big to bring down alone: plenty of vitality, and enough
	// of a body on it to be worth several meals.
	prey := w.addAgent(Agent{Maturity: 1,
		X: 260, Y: 200, Vitality: 300, Hunger: 0, Species: SpeciesEnemy,
		Genome: genomeOf(60, 50, 50)})
	w.agentByID(prey).Species = SpeciesEnemy
	return w, hunter, prey
}

// A companion: somebody the subject has reason to count on, standing nearby.
func companion(t *testing.T, w *World, subject int, x, y float64, affinity float64) int {
	t.Helper()
	id := w.addAgent(Agent{Maturity: 1,
		X: x, Y: y, Sex: Female, Vitality: 90, Hunger: 20,
		Genome: genomeOf(60, 50, 50)})
	if affinity > 0 {
		w.rememberAffinity(mustAgent(t, w, subject), id, affinity)
	}
	return id
}

// A call is a plain visible fact while it is being made, and going in after
// what was called about says the same thing.
func TestACallIsVisibleAndSoIsGoingIn(t *testing.T) {
	w, hunter, prey := inviteWorld(t)
	watcher := companion(t, w, hunter, 230, 220, 0)
	w.SetController(hunter, fixedController{Action{Kind: ActInvite, TargetID: prey}})
	w.SetController(watcher, fixedController{Action{Kind: ActRest}})

	seen := func() int {
		p := w.perceive(mustAgent(t, w, watcher))
		for i := range p.Others {
			if p.Others[i].ID == hunter {
				return p.Others[i].DeclaredFor
			}
		}
		t.Fatal("the caller is not in sight")
		return 0
	}

	w.Step()
	if got := seen(); got != prey {
		t.Fatalf("the watcher sees the caller declared for %d, want %d", got, prey)
	}

	// Gone in after it: still declared, and by the plainest evidence there is.
	w.SetController(hunter, fixedController{Action{Kind: ActAttack, TargetID: prey}})
	w.Step()
	if got := seen(); got != prey {
		t.Fatalf("somebody in the fight is declared for %d, want %d", got, prey)
	}

	// Walked away: nothing to see.
	w.SetController(hunter, fixedController{Action{Kind: ActRest}})
	w.Step()
	if got := seen(); got != 0 {
		t.Fatalf("an agent doing nothing is declared for %d, want nothing", got)
	}
}

// Somebody the subject trusts, declared against the same animal, makes the
// fight worth more than the same somebody standing there as a stranger.
func TestATrustedAllyMakesTheFightWorthMore(t *testing.T) {
	worth := func(affinity float64) float64 {
		w, hunter, prey := inviteWorld(t)
		mate := companion(t, w, hunter, 250, 210, affinity)
		w.SetController(mate, fixedController{Action{Kind: ActAttack, TargetID: prey}})
		w.Step() // the ally declares itself by going in

		w.ai.Decide(w.perceive(mustAgent(t, w, hunter)))
		best, found := math.Inf(-1), false
		for i := range w.ai.opts {
			if a := w.ai.opts[i].action; a.Kind == ActAttack && a.TargetID == prey {
				found = true
				if w.ai.opts[i].util > best {
					best = w.ai.opts[i].util
				}
			}
		}
		if !found {
			t.Fatal("the fight was never scored")
		}
		return best
	}
	stranger, friend := worth(0), worth(40)
	if friend <= stranger {
		t.Fatalf("a friend already in the fight is worth %v against a stranger's %v: trust must count", friend, stranger)
	}
}

// And trust is what counts, not the crowd: an agent with nobody it trusts in
// sight never bothers to call, because calling is the fight plus a wasted
// moment.
func TestNobodyWorthCallingMeansNoCall(t *testing.T) {
	w, hunter, prey := inviteWorld(t)
	companion(t, w, hunter, 250, 210, 0)
	companion(t, w, hunter, 240, 230, 0)

	p := w.perceive(mustAgent(t, w, hunter))
	w.ai.Decide(p)
	for i := range w.ai.opts {
		if w.ai.opts[i].action.Kind == ActInvite {
			t.Fatal("called out to a crowd of strangers")
		}
	}
	_ = prey
}

// With somebody worth calling, the word is there to be chosen - and what it is
// worth is more than the same fight taken on alone.
func TestCallingIsWorthMoreThanGoingInAlone(t *testing.T) {
	w, hunter, prey := inviteWorld(t)
	companion(t, w, hunter, 250, 210, 40)

	w.ai.Decide(w.perceive(mustAgent(t, w, hunter)))
	call, alone := math.Inf(-1), math.Inf(-1)
	called := false
	for i := range w.ai.opts {
		a := w.ai.opts[i].action
		if a.TargetID != prey {
			continue
		}
		switch a.Kind {
		case ActInvite:
			call, called = w.ai.opts[i].util, true
		case ActAttack:
			if w.ai.opts[i].util > alone {
				alone = w.ai.opts[i].util
			}
		}
	}
	if !called {
		t.Fatal("calling was never an option")
	}
	if call <= alone {
		t.Fatalf("calling scored %v and going in alone %v: the help has to be worth the moment spent asking", call, alone)
	}
}

// Both halves can be turned off, which is what the measurement compares
// against: the word without anybody being counted on, and being counted on
// without the word.
func TestTheWordAndTheTrustCanBeTurnedOffApart(t *testing.T) {
	offered := func(apply func(*Config)) (invite bool, allyCounted bool) {
		cfg := testConfig()
		cfg.Width, cfg.Height = 800, 600
		apply(&cfg)
		w := NewWorld(cfg)
		hunter := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Sex: Male,
			Vitality: 90, Hunger: 70, Genome: genomeOf(40, 100, 100)})
		prey := w.addAgent(Agent{Maturity: 1, X: 260, Y: 200, Vitality: 300,
			Hunger: 0, Species: SpeciesEnemy, Genome: genomeOf(60, 50, 50)})
		w.agentByID(prey).Species = SpeciesEnemy
		mate := companion(t, w, hunter, 250, 210, 40)
		w.SetController(mate, fixedController{Action{Kind: ActAttack, TargetID: prey}})
		w.Step()

		w.ai.Decide(w.perceive(mustAgent(t, w, hunter)))
		for i := range w.ai.opts {
			if w.ai.opts[i].action.Kind == ActInvite {
				invite = true
			}
		}
		return invite, w.ai.help(prey).backers > 0
	}

	if invite, ally := offered(func(c *Config) { c.CallTicks = 0 }); invite || !ally {
		t.Fatalf("with the word gone: invite offered %v, ally counted %v - want false and true", invite, ally)
	}
	if invite, ally := offered(func(c *Config) { c.AllyTrustWeight = 0 }); invite || ally {
		t.Fatalf("with nobody counted on: invite offered %v, ally counted %v - want false and false", invite, ally)
	}
}
