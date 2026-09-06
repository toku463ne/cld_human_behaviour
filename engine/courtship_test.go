package engine

import "testing"

// A controller that answers proposals with whatever it was told to, and counts
// how often it was asked.
type proposalController struct {
	AIController
	answer CourtAnswer
	asked  int
	suitor int
}

func (c *proposalController) AnswerCourt(suitorID int) CourtAnswer {
	c.asked++
	c.suitor = suitorID
	return c.answer
}

// twoAboutToCourt puts a suitor next to somebody worth having, both able to pay
// for a birth, and returns the world and the two of them.
func twoAboutToCourt(t *testing.T, cfg Config) (*World, int, int) {
	t.Helper()
	w := NewWorld(cfg)
	suitor := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Sex: Male, Vitality: 90, Hunger: 5,
		Genome: genomeOf(50, 100, 100)})
	courted := w.addAgent(Agent{Maturity: 1, X: 202, Y: 200, Sex: Female, Vitality: 90, Hunger: 5,
		Genome: genomeOf(50, 100, 100)})
	sa, ca := mustAgent(t, w, suitor), mustAgent(t, w, courted)
	// Both obvious catches, so the rule would say yes on both sides: what the
	// tests below change is the answer, not whether the rule would give one.
	sa.Genome[GeneAttractiveness], ca.Genome[GeneAttractiveness] = 100, 100
	sa.courtStartTick, ca.courtStartTick = w.tick, w.tick
	sa.Action = Action{Kind: ActCourt, TargetID: courted, Effort: 1}
	return w, suitor, courted
}

// The one being courted answers, and the answer is what happens.
func TestTheCourtedSideCanAnswerForItself(t *testing.T) {
	for _, c := range []struct {
		name   string
		answer CourtAnswer
		paired bool
	}{
		{"accept", CourtAccept, true},
		{"refuse", CourtRefuse, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			cfg := testConfig()
			cfg.JudgementNoise = 0
			w, suitor, courted := twoAboutToCourt(t, cfg)
			ctrl := &proposalController{answer: c.answer}
			w.SetController(courted, ctrl)

			w.court(mustAgent(t, w, suitor))

			if ctrl.asked == 0 || ctrl.suitor != suitor {
				t.Fatalf("the controller was asked %d times about #%d", ctrl.asked, ctrl.suitor)
			}
			if got := mustAgent(t, w, courted).PartnerID != 0; got != c.paired {
				t.Fatalf("paired = %v, want %v", got, c.paired)
			}
		})
	}
}

// Refusing beats the rule: the rule would have said yes to this one.
func TestARefusalOverridesTheRuleThatWouldHaveAccepted(t *testing.T) {
	cfg := testConfig()
	cfg.JudgementNoise = 0
	w, suitor, courted := twoAboutToCourt(t, cfg)

	// What the rule would have said, with nobody answering.
	if !w.willCommit(mustAgent(t, w, courted), w.perceivedFitness(mustAgent(t, w, courted), mustAgent(t, w, suitor))) {
		t.Skip("the rule would have refused anyway, so this proves nothing")
	}
	w.SetController(courted, &proposalController{answer: CourtRefuse})
	w.court(mustAgent(t, w, suitor))
	if mustAgent(t, w, courted).PartnerID != 0 {
		t.Fatal("a refusal did not stop the pair forming")
	}
}

// While nobody has answered, the suitor waits: no pair, no rejection, and the
// one being courted knows somebody is standing there.
func TestASuitorWaitsForAnAnswerAndTheRuleAnswersInTheEnd(t *testing.T) {
	cfg := testConfig()
	cfg.JudgementNoise = 0
	cfg.CourtAnswerTicks = 10
	w, suitor, courted := twoAboutToCourt(t, cfg)
	w.SetController(courted, &proposalController{answer: CourtWaiting})

	for i := 0; i < cfg.CourtAnswerTicks; i++ {
		w.court(mustAgent(t, w, suitor))
		if mustAgent(t, w, courted).PartnerID != 0 {
			t.Fatalf("paired on tick %d without an answer", i)
		}
		if got := mustAgent(t, w, courted).courtedBy; got != suitor {
			t.Fatalf("tick %d: the courted one says #%d is proposing, want #%d", i, got, suitor)
		}
		// And it is in the perception, which is how an interface finds out.
		if p := w.perceive(mustAgent(t, w, courted)); p.Self.CourtedBy != suitor {
			t.Fatalf("tick %d: the perception says #%d is proposing", i, p.Self.CourtedBy)
		}
		w.tick++
	}
	// Waited long enough: the node's own rule answers, which here means yes.
	w.court(mustAgent(t, w, suitor))
	if mustAgent(t, w, courted).PartnerID != suitor {
		t.Fatal("the rule did not answer once the waiting was over")
	}
}

// A controller that does not answer proposals leaves the world exactly as it
// was: the answer is the rule's, on the spot, with nobody held up.
func TestAControllerThatDoesNotAnswerChangesNothing(t *testing.T) {
	cfg := testConfig()
	cfg.JudgementNoise = 0
	w, suitor, courted := twoAboutToCourt(t, cfg)
	w.SetController(courted, NewGuidedController()) // answering is off by default

	w.court(mustAgent(t, w, suitor))
	if mustAgent(t, w, courted).courtedBy != 0 {
		t.Fatal("a proposal was left open for a controller that does not answer")
	}
	if mustAgent(t, w, courted).PartnerID != suitor {
		t.Fatal("the rule did not answer on the spot")
	}
}

// A proposal is forgotten when the one who made it stops making it.
func TestAProposalLapsesWhenTheSuitorGivesUp(t *testing.T) {
	cfg := testConfig()
	cfg.JudgementNoise = 0
	w, suitor, courted := twoAboutToCourt(t, cfg)
	w.SetController(courted, &proposalController{answer: CourtWaiting})
	w.court(mustAgent(t, w, suitor))
	if mustAgent(t, w, courted).courtedBy != suitor {
		t.Fatal("no proposal was opened")
	}

	// Gone for good, which is the case an interface must not leave a prompt
	// standing for. (A suitor that merely rests this tick may well decide to
	// court again the next one, so resting proves nothing.)
	w.kill(mustAgent(t, w, suitor))
	w.Step()
	if got := mustAgent(t, w, courted).courtedBy; got != 0 {
		t.Fatalf("still waiting on #%d after it died", got)
	}
}

// End to end with the controller a person actually drives: the suitor waits,
// the player says yes, and the pair forms on the next tick of the courtship.
func TestAPlayerAnswersAProposalThroughTheHumanController(t *testing.T) {
	cfg := testConfig()
	cfg.JudgementNoise = 0
	w, suitor, courted := twoAboutToCourt(t, cfg)
	h := NewHumanController()
	w.SetController(courted, h)

	// Nothing said yet: the suitor stands there and the perception says so.
	w.court(mustAgent(t, w, suitor))
	if mustAgent(t, w, courted).PartnerID != 0 {
		t.Fatal("paired before the player said anything")
	}
	p := w.perceive(mustAgent(t, w, courted))
	if p.Self.CourtedBy != suitor || p.Self.CourtedTicksLeft != cfg.CourtAnswerTicks {
		t.Fatalf("the perception reads courted by #%d with %d ticks left",
			p.Self.CourtedBy, p.Self.CourtedTicksLeft)
	}

	// The player says no, and no is what happens even though the rule would
	// have said yes.
	h.AnswerProposal(suitor, false)
	w.court(mustAgent(t, w, suitor))
	if mustAgent(t, w, courted).PartnerID != 0 {
		t.Fatal("the player's refusal did not hold")
	}
	if mustAgent(t, w, courted).courtedBy != 0 {
		t.Fatal("the proposal is still standing after it was answered")
	}

	// And an answer is spent: the next proposal from the same one waits again.
	mustAgent(t, w, suitor).Action = Action{Kind: ActCourt, TargetID: courted, Effort: 1}
	mustAgent(t, w, suitor).CooldownTimer = 0
	w.court(mustAgent(t, w, suitor))
	if got := mustAgent(t, w, courted).courtedBy; got != suitor {
		t.Fatalf("the second proposal was answered by the old answer (courtedBy %d)", got)
	}
	h.AnswerProposal(suitor, true)
	w.court(mustAgent(t, w, suitor))
	if mustAgent(t, w, courted).PartnerID != suitor {
		t.Fatal("the player's yes did not form the pair")
	}
}
