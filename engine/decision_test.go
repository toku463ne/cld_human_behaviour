package engine

import (
	"fmt"
	"math"
	"testing"
)

// Scenario tests: a situation is built by hand, the agent is asked to decide
// once, and the answer is checked. They are the counterpart of watching a run
// in cmd/devview, which shows that something sensible happens but not that the
// right thing happens in the case that was aimed at.
//
// Every subject here is given full rationality and intelligence, so that its
// reading of the world and its scoring of the options carry no noise and the
// test is deterministic. What it believes about somebody else's strength is
// still only what it has observed: combat power is hidden, so a scenario that
// turns on the subject knowing how strong the other one is has to let it find
// out first (see convinceOf).

// convinceOf makes an observer certain about somebody's true strength. It
// stands in for the fights and the watching that would otherwise be needed, so
// that a scenario can be about the decision rather than about how the estimate
// got there.
func convinceOf(t *testing.T, w *World, observerID, targetID int) {
	t.Helper()
	for i := 0; i < 200; i++ {
		w.observeStrength(mustAgent(t, w, observerID), mustAgent(t, w, targetID), 1)
	}
	op := w.opinionOf(mustAgent(t, w, observerID), targetID)
	truth := mustAgent(t, w, targetID).Power
	if math.Abs(op.Strength-truth) > 5 {
		t.Fatalf("observer still believes %d has strength %.1f, true value is %.1f", targetID, op.Strength, truth)
	}
}

// wantAction fails unless the agent decided to do exactly this.
func wantAction(t *testing.T, w *World, id int, kind ActionKind, target int, what string) Action {
	t.Helper()
	got := aiChoice(w, id)
	if got.Kind != kind || (target != 0 && got.TargetID != target) {
		t.Fatalf("%s: agent %d chose %s, want %s #%d", what, id, describe(got), kind, target)
	}
	return got
}

// decideWithTrace follows an agent, lets it make its first decision and returns
// the record of it. A scenario uses this when it is not only the answer that
// matters but the comparison behind it.
func decideWithTrace(t *testing.T, w *World, id int) DecisionTrace {
	t.Helper()
	if !w.TrackDecisions(id, true) {
		t.Fatalf("cannot follow agent %d", id)
	}
	w.Step()
	tr, ok := w.LastDecisionTrace(id)
	if !ok {
		t.Fatalf("agent %d decided nothing", id)
	}
	return tr
}

// chosenOption is the option the trace says was taken.
func chosenOption(t *testing.T, tr DecisionTrace) TracedOption {
	t.Helper()
	if tr.Chosen < 0 || tr.Chosen >= len(tr.Options) {
		t.Fatalf("trace has no chosen option (%d of %d)", tr.Chosen, len(tr.Options))
	}
	return tr.Options[tr.Chosen]
}

// wantNotAction fails if the agent decided to do this.
func wantNotAction(t *testing.T, w *World, id int, kind ActionKind, what string) Action {
	t.Helper()
	got := aiChoice(w, id)
	if got.Kind == kind {
		t.Fatalf("%s: agent %d chose %s, want anything but %s", what, id, describe(got), kind)
	}
	return got
}

func describe(a Action) string {
	if a.TargetID == 0 {
		return a.Kind.String()
	}
	return fmt.Sprintf("%s #%d", a.Kind, a.TargetID)
}

// --- food, and who is standing next to it ----------------------------------

// A hungry agent takes the meal it can actually get to first, not the nearest
// one. Nothing weighs up rivals here: losing the race is simply unlikely to
// feed it.
func TestHungryAgentTakesTheMealItCanWinTheRaceFor(t *testing.T) {
	w := NewWorld(testConfig())
	subject := w.addAgent(Agent{
		X: 200, Y: 200, Sex: Male, Vitality: 70, Hunger: 75,
		Power: 50, Rationality: 100, Intelligence: 100,
	})
	// The near meal has somebody standing on top of it; the far one is free.
	near := w.addFood(230, 200)
	far := w.addFood(200, 280)
	w.addAgent(Agent{X: 236, Y: 200, Sex: Male, Vitality: 100, Hunger: 0, Power: 50})

	got := wantAction(t, w, subject, ActEat, far, "meal in somebody else's lap")
	if got.TargetID == near {
		t.Fatal("went for the contested meal it was going to lose the race for")
	}
}

// The example from the design notes: hungry, food in front of it, and somebody
// far stronger standing on that food. The agent does not fight for it — the
// fight is hopeless — but it does not give the meal up either: a slim chance at
// something that would save its life still beats wandering off, so it races and
// probably loses. Losing that race is what the option is priced at.
func TestOutmatchedAgentRacesForFoodRatherThanFightingForIt(t *testing.T) {
	w := NewWorld(testConfig())
	subject := w.addAgent(Agent{
		X: 200, Y: 200, Sex: Male, Vitality: 45, Hunger: 80,
		Power: 12, Rationality: 100, Intelligence: 100,
	})
	guarded := w.addFood(226, 200)
	giant := w.addAgent(Agent{X: 232, Y: 200, Sex: Male, Vitality: 100, Hunger: 0, Power: 100})
	convinceOf(t, w, subject, giant)

	tr := decideWithTrace(t, w, subject)
	if tr.Action.Kind != ActEat || tr.Action.TargetID != guarded {
		t.Fatalf("chose %s, want it to take its chances at the meal", describe(tr.Action))
	}
	for _, o := range tr.Options {
		if o.Action.Kind == ActAttack && o.Score > tr.Options[tr.Chosen].Score {
			t.Fatalf("picking a fight with a giant scored %v, better than the meal at %v",
				o.Score, tr.Options[tr.Chosen].Score)
		}
	}
	// And it knows it is the outsider: the meal is scored at the odds of
	// getting there first, not at what it would be worth uncontested.
	if got := chosenOption(t, tr).Utility.Life.Chance; got > 0.35 {
		t.Fatalf("reckoned its odds of winning the race at %.2f, want it to know it is losing", got)
	}
}

// The same situation with the strengths the other way round. Driving a weaker
// rival off a meal the agent badly needs is worth the vitality, and that is the
// only thing that changed.
func TestContestedMealIsWorthAFightAgainstAWeakerRival(t *testing.T) {
	w := NewWorld(testConfig())
	subject := w.addAgent(Agent{
		X: 200, Y: 200, Sex: Male, Vitality: 40, Hunger: 92,
		Power: 95, Rationality: 100, Intelligence: 100,
	})
	w.addFood(222, 200)
	rival := w.addAgent(Agent{X: 226, Y: 200, Sex: Male, Vitality: 25, Hunger: 0, Power: 8})
	convinceOf(t, w, subject, rival)

	tr := decideWithTrace(t, w, subject)
	if tr.Action.Kind != ActAttack || tr.Action.TargetID != rival {
		t.Fatalf("chose %s, want it to drive the weaker rival off the meal", describe(tr.Action))
	}
	// The fight is worth having for the meal it secures, not for its own sake.
	if u := chosenOption(t, tr).Utility; u.Stake.Score() <= -u.Life.Score() {
		t.Fatalf("the meal at stake was worth %.2f and the fight cost %.2f of life: "+
			"the fight is not being paid for by the meal", u.Stake.Score(), -u.Life.Score())
	}
}

// Hunger is what sends an agent wandering. With nothing in sight there is no
// option that feeds it, so the only thing worth doing is going to look.
func TestHungryAgentWithNothingInSightGoesLooking(t *testing.T) {
	w := NewWorld(testConfig())
	subject := w.addAgent(Agent{
		X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 85,
		Power: 50, Rationality: 100, Intelligence: 100,
	})

	got := wantAction(t, w, subject, ActMove, 0, "hungry with an empty world")
	if got.DX == 0 && got.DY == 0 {
		t.Fatal("wandering with no direction")
	}
}

// A fed agent that has been knocked about does the one thing that mends it.
// Resting is not a fallback for having no ideas: it is the only route back to
// full vitality, and the comparison has to pick it on its merits.
func TestBatteredButFedAgentRests(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	subject := w.addAgent(Agent{
		X: 200, Y: 200, Sex: Male, Vitality: 30, Hunger: 5,
		Power: 50, Rationality: 100, Intelligence: 100,
	})
	// Not fit to reproduce yet, so that priority 2 stays out of the comparison.
	mustAgent(t, w, subject).CooldownTimer = 100

	wantAction(t, w, subject, ActRest, 0, "hurt but well fed")
}

// --- being attacked --------------------------------------------------------

// attackedBy sets one agent hitting another and runs a tick, so that the victim
// has actually felt it and decides knowing it is under attack.
func attackedBy(t *testing.T, w *World, victim, attacker int) {
	t.Helper()
	w.SetController(attacker, fixedController{Action{Kind: ActAttack, TargetID: victim, Effort: 1}})
	w.SetController(victim, fixedController{Action{Kind: ActRest}})
	w.Step()
	if mustAgent(t, w, victim).attackerID != attacker {
		t.Fatalf("agent %d is not registering %d as its attacker", victim, attacker)
	}
}

// Worn down and being hit by somebody stronger, with nothing to eat in reach:
// the damage is what is about to kill it, and running is the only option that
// takes the damage out of the picture.
func TestCorneredAgentRunsFromTheOneKillingIt(t *testing.T) {
	w := NewWorld(testConfig())
	victim := w.addAgent(Agent{
		X: 200, Y: 200, Sex: Male, Vitality: 14, Hunger: 20,
		Power: 15, Rationality: 100, Intelligence: 100,
	})
	bully := w.addAgent(Agent{
		X: 208, Y: 200, Sex: Male, Vitality: 100, Hunger: 0,
		Power: 95, Rationality: 100, Intelligence: 100,
	})
	convinceOf(t, w, victim, bully)
	attackedBy(t, w, victim, bully)

	got := wantAction(t, w, victim, ActFlee, bully, "nearly dead under a stronger attacker")
	if got.Effort != w.cfg.FleeEffort {
		t.Fatalf("ran at effort %.2f, want %.2f", got.Effort, w.cfg.FleeEffort)
	}
}

// The mirror image, and the reason there is no "flee when hurt" threshold
// anywhere: the same battered agent stands its ground against somebody who can
// barely hurt it, because the damage coming in is not what is going to kill it.
func TestBatteredAgentDoesNotRunFromAFeebleAttacker(t *testing.T) {
	w := NewWorld(testConfig())
	victim := w.addAgent(Agent{
		X: 200, Y: 200, Sex: Male, Vitality: 14, Hunger: 20,
		Power: 60, Rationality: 100, Intelligence: 100,
	})
	weakling := w.addAgent(Agent{
		X: 208, Y: 200, Sex: Male, Vitality: 20, Hunger: 0,
		Power: 3, Rationality: 100, Intelligence: 100,
	})
	convinceOf(t, w, victim, weakling)
	attackedBy(t, w, victim, weakling)

	wantNotAction(t, w, victim, ActFlee, "battered but barely being scratched")
}

// Being hit is a reason to think again, and the trace says so. It also changes
// what there is to think about: running away is only an option against somebody
// who is actually swinging at you.
func TestBeingHitPromptsTheRethinkAndPutsFleeingOnTheTable(t *testing.T) {
	w := NewWorld(testConfig())
	victim := w.addAgent(Agent{
		X: 200, Y: 200, Sex: Male, Vitality: 60, Hunger: 20,
		Power: 40, Rationality: 100, Intelligence: 100,
	})
	bully := w.addAgent(Agent{
		X: 208, Y: 200, Sex: Male, Vitality: 100, Hunger: 0,
		Power: 90, Rationality: 100, Intelligence: 100,
	})
	w.SetController(bully, fixedController{Action{Kind: ActAttack, TargetID: victim, Effort: 1}})
	w.TrackDecisions(victim, true)

	w.Step() // the victim decides on arrival, and takes the first blow
	first, _ := w.LastDecisionTrace(victim)
	if first.Trigger != TriggerSpawned {
		t.Fatalf("first decision was prompted by %q, want its arrival", first.Trigger)
	}
	for _, o := range first.Options {
		if o.Action.Kind == ActFlee {
			t.Fatal("weighed up running away before anybody had swung at it")
		}
	}

	w.Step() // now it has been hit
	tr, _ := w.LastDecisionTrace(victim)
	if tr.Trigger != TriggerAttacked {
		t.Fatalf("second decision was prompted by %q, want the blow that landed", tr.Trigger)
	}
	if tr.Self.AttackerID != bully {
		t.Fatalf("it thinks %d is hitting it, want %d", tr.Self.AttackerID, bully)
	}
	found := false
	for _, o := range tr.Options {
		found = found || (o.Action.Kind == ActFlee && o.Action.TargetID == bully)
	}
	if !found {
		t.Fatal("being under attack did not put running away among the options")
	}
}

// --- priority 2 ------------------------------------------------------------

// With survival comfortable and a candidate in front of it, an agent gets on
// with priority 2. Nothing here outranks it, which is exactly why the same
// agent drops it the moment food or damage enters the picture.
func TestSettledAgentCourtsTheBestCandidateInSight(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	subject := w.addAgent(Agent{
		X: 200, Y: 200, Sex: Male, Vitality: 100, Hunger: 5,
		Power: 50, Rationality: 100, Intelligence: 100,
	})
	dull := w.addAgent(Agent{X: 215, Y: 200, Sex: Female, Vitality: 40, Hunger: 5, Power: 10, Rationality: 10, Intelligence: 10})
	catch := w.addAgent(Agent{X: 225, Y: 200, Sex: Female, Vitality: 100, Hunger: 5, Power: 95, Rationality: 95, Intelligence: 95})
	mustAgent(t, w, subject).reproReady = true

	got := wantAction(t, w, subject, ActCourt, catch, "settled with two candidates in sight")
	if got.TargetID == dull {
		t.Fatal("walked up to the nearer, worse candidate")
	}
}

// The same agent, one thing changed: it is starving. Priority 1 does not merely
// outscore priority 2 here, it removes it from the comparison altogether, which
// is what the trace shows: courting is never even one of the options.
func TestStarvingAgentNeverWeighsUpCourting(t *testing.T) {
	w := NewWorld(testConfig())
	subject := w.addAgent(Agent{
		X: 200, Y: 200, Sex: Male, Vitality: 60, Hunger: 90,
		Power: 50, Rationality: 100, Intelligence: 100,
	})
	w.addAgent(Agent{X: 170, Y: 200, Sex: Female, Vitality: 100, Hunger: 5, Power: 95, Rationality: 95, Intelligence: 95})
	meal := w.addFood(222, 200)
	mustAgent(t, w, subject).reproReady = true

	tr := decideWithTrace(t, w, subject)
	if tr.Action.Kind != ActEat || tr.Action.TargetID != meal {
		t.Fatalf("chose %s, want it to go and eat", describe(tr.Action))
	}
	for _, o := range tr.Options {
		if o.Action.Kind == ActCourt {
			t.Fatal("a starving agent weighed up courting")
		}
	}
}

// --- the trace itself ------------------------------------------------------

// The trace is only kept for the agents somebody asked to follow. Recording
// everybody would cost more than deciding does.
func TestOnlyTrackedAgentsAreTraced(t *testing.T) {
	w := NewWorld(testConfig())
	followed := w.addAgent(Agent{X: 200, Y: 200, Vitality: 80, Hunger: 60, Power: 50, Rationality: 100, Intelligence: 100})
	ignored := w.addAgent(Agent{X: 300, Y: 300, Vitality: 80, Hunger: 60, Power: 50, Rationality: 100, Intelligence: 100})

	if !w.TrackDecisions(followed, true) {
		t.Fatal("could not follow an agent that exists")
	}
	if w.TrackDecisions(999, true) {
		t.Fatal("followed an agent that does not exist")
	}
	for i := 0; i < 50; i++ {
		w.Step()
	}

	if len(w.DecisionTraces(followed)) == 0 {
		t.Fatal("the followed agent recorded nothing in 50 ticks")
	}
	if got := w.DecisionTraces(ignored); got != nil {
		t.Fatalf("an agent nobody is following recorded %d decisions", len(got))
	}

	w.TrackDecisions(followed, false)
	before := len(w.DecisionTraces(followed))
	for i := 0; i < 50; i++ {
		w.Step()
	}
	if before != 0 || len(w.DecisionTraces(followed)) != 0 {
		t.Fatal("dropping an agent did not stop the recording")
	}
}

// A trace has to explain the decision it recorded: the option marked as chosen
// is the action that came out, and the terms of each option add up to the score
// it was compared on.
func TestTraceExplainsTheChosenAction(t *testing.T) {
	w := NewWorld(testConfig())
	subject := w.addAgent(Agent{
		X: 200, Y: 200, Sex: Male, Vitality: 60, Hunger: 70,
		Power: 50, Rationality: 100, Intelligence: 100,
	})
	w.addFood(230, 200)
	w.addAgent(Agent{X: 240, Y: 210, Sex: Female, Vitality: 90, Hunger: 10, Power: 60})
	w.TrackDecisions(subject, true)
	w.Step()

	tr, ok := w.LastDecisionTrace(subject)
	if !ok {
		t.Fatal("nothing recorded")
	}
	if tr.AgentID != subject || tr.Tick != w.Tick() {
		t.Fatalf("trace is of agent %d at tick %d, want %d at %d", tr.AgentID, tr.Tick, subject, w.Tick())
	}
	if tr.Trigger != TriggerSpawned {
		t.Fatalf("first decision was prompted by %q, want the agent's arrival", tr.Trigger)
	}
	if len(tr.Options) < 2 {
		t.Fatalf("only %d options were compared, want the alternatives to be recorded too", len(tr.Options))
	}
	if tr.Chosen < 0 || tr.Chosen >= len(tr.Options) {
		t.Fatalf("chosen option is %d of %d", tr.Chosen, len(tr.Options))
	}

	chosen := tr.Options[tr.Chosen]
	if chosen.Action != tr.Action {
		t.Fatalf("the trace says it picked %s but the action taken was %s", describe(chosen.Action), describe(tr.Action))
	}
	for i, o := range tr.Options {
		if math.Abs(o.Utility.Total()+o.Noise-o.Score) > 1e-9 {
			t.Fatalf("option %d: terms add up to %v but it was compared on %v", i, o.Utility.Total()+o.Noise, o.Score)
		}
		if o.Score > chosen.Score {
			t.Fatalf("option %d scored %v, better than the one that was taken (%v)", i, o.Score, chosen.Score)
		}
	}

	// The breakdown has to name what the option was for, not merely total up.
	if len(chosen.Utility.Goals()) == 0 {
		t.Fatalf("the chosen option (%s) serves no goal at all", describe(chosen.Action))
	}
}

// A followed agent keeps the last few decisions, not just the last one, and
// drops the oldest rather than growing without bound.
func TestTraceKeepsARollingHistory(t *testing.T) {
	cfg := testConfig()
	cfg.FoodSpawnRate = 0.5
	w := NewWorld(cfg)
	subject := w.addAgent(Agent{
		X: 200, Y: 200, Vitality: 80, Hunger: 50,
		Power: 50, Rationality: 100, Intelligence: 100,
	})
	w.TrackDecisions(subject, true)
	for i := 0; i < 600 && mustAgent(t, w, subject).Alive; i++ {
		w.Step()
	}

	traces := w.DecisionTraces(subject)
	if len(traces) != traceHistory {
		t.Fatalf("kept %d decisions, want the last %d", len(traces), traceHistory)
	}
	for i := 1; i < len(traces); i++ {
		if traces[i].Tick < traces[i-1].Tick {
			t.Fatalf("history is not in order: tick %d comes after %d", traces[i].Tick, traces[i-1].Tick)
		}
	}
	last, _ := w.LastDecisionTrace(subject)
	if last.Tick != traces[len(traces)-1].Tick {
		t.Fatal("the last trace is not the last one in the history")
	}
}

// The trace survives the agent: what it was thinking on the way out is exactly
// what is worth reading afterwards.
func TestTraceOutlivesTheAgent(t *testing.T) {
	cfg := testConfig()
	w := NewWorld(cfg)
	doomed := w.addAgent(Agent{
		X: 200, Y: 200, Vitality: 2, Hunger: cfg.MaxHunger,
		Power: 50, Rationality: 100, Intelligence: 100,
	})
	w.TrackDecisions(doomed, true)
	for i := 0; i < 200 && mustAgent(t, w, doomed) != nil; i++ {
		w.Step()
		if w.agentByID(doomed) == nil {
			break
		}
	}
	if w.agentByID(doomed) != nil {
		t.Fatal("the agent was supposed to starve")
	}
	if len(w.DecisionTraces(doomed)) == 0 {
		t.Fatal("the decisions of a dead agent were thrown away with it")
	}
}

// A controller that ignores the trace still leaves a usable record: the world
// fills in what prompted the decision and what came out of it.
func TestTraceWorksForAControllerThatDoesNotFillItIn(t *testing.T) {
	w := NewWorld(testConfig())
	id := w.addAgent(Agent{X: 200, Y: 200, Vitality: 80, Hunger: 10, Power: 50})
	w.TrackDecisions(id, true)
	w.SetController(id, fixedController{Action{Kind: ActMove, DX: 1, Effort: 0.5}})
	w.Step()

	tr, ok := w.LastDecisionTrace(id)
	if !ok {
		t.Fatal("nothing recorded")
	}
	if tr.Action.Kind != ActMove || len(tr.Options) != 0 || tr.Chosen != -1 {
		t.Fatalf("got %+v, want the action recorded and no options", tr)
	}
}

// --- the strategy depth gate -----------------------------------------------

// consideredKinds is the set of action kinds an agent put on the table, which is
// what the depth gate decides. What it went on to choose is a separate question.
func consideredKinds(tr DecisionTrace) map[ActionKind]bool {
	kinds := make(map[ActionKind]bool, len(tr.Options))
	for _, o := range tr.Options {
		kinds[o.Action.Kind] = true
	}
	return kinds
}

// preemptiveValue is the most any option was worth for thinning out the
// competition. It is zero unless the agent can think that far ahead.
func preemptiveValue(tr DecisionTrace) float64 {
	best := 0.0
	for _, o := range tr.Options {
		best = math.Max(best, o.Utility.Rival.Value)
	}
	return best
}

// gateScene puts a dull agent next to a stranger it could fight, court or study,
// and reports what it thought of doing.
func gateScene(t *testing.T, unlock, intelligence float64) DecisionTrace {
	t.Helper()
	cfg := testConfig()
	cfg.StrategyDepthUnlock = unlock
	w := NewWorld(cfg)
	subject := w.addAgent(Agent{
		X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 20,
		Power: 50, Rationality: 100, Intelligence: intelligence,
	})
	w.addAgent(Agent{X: 240, Y: 200, Sex: Female, Vitality: 80, Hunger: 20, Power: 50})
	return decideWithTrace(t, w, subject)
}

// At the default spacing an intelligence of 20 buys the reactive moves and
// nothing else: the agent can fight and court, but studying somebody costs a
// level it has not paid for, and removing a future rival costs two.
func TestStrategyDepthGateWithholdsTheDeeperMoves(t *testing.T) {
	tr := gateScene(t, 16, 20)
	kinds := consideredKinds(tr)

	if !kinds[ActAttack] || !kinds[ActCourt] {
		t.Fatalf("the reactive moves should be unlocked at this level, got %v", kinds)
	}
	if kinds[ActObserve] {
		t.Fatal("observing needs a level this agent has not unlocked")
	}
	if v := preemptiveValue(tr); v != 0 {
		t.Fatalf("pre-emptive value %v, want it out of reach at this level", v)
	}
}

// Zero unlock cost turns the gate off, which is the control arm of the
// intelligence experiment: the same dull agent now weighs up everything, and
// intelligence is left acting through the noise on its scoring alone.
func TestStrategyDepthGateOffUnlocksEverything(t *testing.T) {
	tr := gateScene(t, 0, 20)
	kinds := consideredKinds(tr)

	if !kinds[ActObserve] {
		t.Fatalf("with the gate off every move should be on the table, got %v", kinds)
	}
	if v := preemptiveValue(tr); v <= 0 {
		t.Fatalf("pre-emptive value %v, want it available with the gate off", v)
	}
}
