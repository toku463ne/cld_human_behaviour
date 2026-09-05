package engine

import (
	"errors"
	"testing"
)

// Stage 19: the controller a person drives. What these tests hold in place is
// not that a player can win, but that playing goes through the same seam the AI
// does - the same question, the same vocabulary, the same envelope of what a
// node may aim at.

// takeOver installs a fresh human controller on an agent and returns it.
func takeOver(t *testing.T, w *World, id int) *HumanController {
	t.Helper()
	h := NewHumanController()
	if !w.SetController(id, h) {
		t.Fatalf("cannot take agent %d over", id)
	}
	return h
}

// The engine drives a human's answer exactly as it drives the AI's. Nothing in
// the world knows which one it asked.
func TestHumanOrderIsCarriedOutLikeAnyDecision(t *testing.T) {
	w := NewWorld(quietConfig())
	subject := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 40,
		Genome: genomeOf(50, 100, 100)})
	takeOver(t, w, subject)

	if err := w.OrderHuman(subject, Action{Kind: ActMove, DX: 3, DY: 4, Effort: 0.5}); err != nil {
		t.Fatalf("refused a move with a direction: %v", err)
	}
	before := mustAgent(t, w, subject).X
	w.Step()

	got := mustAgent(t, w, subject)
	if got.Action.Kind != ActMove {
		t.Fatalf("the world had it %s, want the order it was given", got.Action.Kind)
	}
	// Normalised on the way in, so an interface can hand over a raw vector.
	if got.Action.DX < 0.59 || got.Action.DX > 0.61 {
		t.Fatalf("direction came through as %.3f, want the unit vector (0.6)", got.Action.DX)
	}
	if got.X <= before {
		t.Fatal("ordered east and did not move")
	}
}

// The complaint that started this: "it will not eat the food in front of it".
// The engine side was never the problem - an eat order walks the node over and
// eats - and pinning that is what says the fault was in the interface.
func TestAnEatOrderIsCarriedAllTheWayThrough(t *testing.T) {
	cfg := testConfig()
	cfg.TriggerIdleTicks = 1 << 30
	cfg.TriggerVitalityDrop = 1e9
	w := NewWorld(cfg)
	subject := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 60,
		Genome: genomeOf(50, 100, 100)})
	takeOver(t, w, subject)
	meal := w.addFood(240, 200) // forty away: a walk and then a meal

	if err := w.OrderHuman(subject, Action{Kind: ActEat, TargetID: meal, Effort: 1}); err != nil {
		t.Fatal(err)
	}
	hungerBefore := mustAgent(t, w, subject).Hunger
	for i := 0; i < 200; i++ {
		w.Step()
		if _, ok := w.FoodByID(meal); !ok {
			break
		}
	}
	if _, ok := w.FoodByID(meal); ok {
		a := mustAgent(t, w, subject)
		t.Fatalf("still not eaten after 200 ticks: it is at %.0f,%.0f doing %s",
			a.X, a.Y, a.Action.Kind)
	}
	if got := mustAgent(t, w, subject).Hunger; got >= hungerBefore {
		t.Fatalf("hunger %v after the meal, was %v", got, hungerBefore)
	}
}

// An order that has been carried out is spent, and that is told apart from one
// that was taken away. Before this, eating the meal you asked for was recorded
// as the meal being taken from you: the order stood, the node answered the next
// question with the same now impossible thing, and a tick later it lapsed.
func TestACarriedOutOrderIsSpentAndNotLapsed(t *testing.T) {
	cfg := testConfig()
	cfg.TriggerIdleTicks = 1 << 30
	cfg.TriggerVitalityDrop = 1e9
	w := NewWorld(cfg)
	subject := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 60,
		Genome: genomeOf(50, 100, 100)})
	h := takeOver(t, w, subject)
	meal := w.addFood(205, 200) // within reach: one tick and it is eaten

	if err := w.OrderHuman(subject, Action{Kind: ActEat, TargetID: meal, Effort: 1}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		w.Step()
	}
	if _, ok := w.FoodByID(meal); ok {
		t.Fatal("the meal was not eaten")
	}
	if got := h.Voided(); got != 0 {
		t.Fatalf("%d order(s) recorded as lost, want none: it ate what it was told to", got)
	}
	if got := h.Finished(); got != 1 {
		t.Fatalf("%d order(s) recorded as carried out, want 1", got)
	}
	if got := h.Standing(); got.Kind != ActRest || got.TargetID != 0 {
		t.Fatalf("the standing order is still %s #%d after it was carried out",
			got.Kind, got.TargetID)
	}
}

// And a meal somebody else got to first is still a loss.
func TestAMealTakenFirstIsStillLapsed(t *testing.T) {
	cfg := testConfig()
	cfg.TriggerIdleTicks = 1 << 30
	cfg.TriggerVitalityDrop = 1e9
	w := NewWorld(cfg)
	subject := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 60,
		Genome: genomeOf(50, 100, 100)})
	h := takeOver(t, w, subject)
	meal := w.addFood(260, 200) // in sight, but far enough that it is still walking

	if err := w.OrderHuman(subject, Action{Kind: ActEat, TargetID: meal, Effort: 1}); err != nil {
		t.Fatal(err)
	}
	w.Step()
	w.removeFoodByID(meal) // somebody else got there
	for i := 0; i < 5; i++ {
		w.Step()
	}
	if got := h.Voided(); got != 1 {
		t.Fatalf("%d order(s) recorded as lost, want 1", got)
	}
	if got := h.Finished(); got != 0 {
		t.Fatalf("%d order(s) recorded as carried out, want none", got)
	}
}

// An order is a standing one. The engine asks on a trigger, so what a player
// types is the answer to the next question rather than an act of its own.
func TestOrderIsTakenUpWhenTheEngineNextAsks(t *testing.T) {
	cfg := quietConfig()
	// Nothing may prompt a decision on its own, or this would be testing the
	// idle trigger.
	cfg.TriggerIdleTicks = 1 << 30
	cfg.TriggerVitalityDrop = 1e9
	w := NewWorld(cfg)
	subject := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 40,
		Genome: genomeOf(50, 100, 100)})
	h := takeOver(t, w, subject)

	// Taking the node over is itself a trigger, so the first question comes on
	// the next tick and is answered with the resting order it starts with.
	w.Step()
	if n, _ := h.Asked(); n != 1 {
		t.Fatalf("asked %d times after being taken over, want once", n)
	}

	if err := w.OrderHuman(subject, Action{Kind: ActMove, DX: 1, Effort: 1}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		w.Step()
	}
	if n, _ := h.Asked(); n != 1 {
		t.Fatalf("asked %d times, want the order to wait for a reason to think again", n)
	}
	if got := mustAgent(t, w, subject).Action.Kind; got != ActRest {
		t.Fatalf("the node is %s, want it still resting on the answer it gave", got)
	}

	// Which is what asking for the question is for.
	if !w.RequestDecision(subject) {
		t.Fatal("cannot ask for a decision")
	}
	w.Step()
	if n, _ := h.Asked(); n != 2 {
		t.Fatalf("asked %d times, want the requested question to have been put", n)
	}
	if got := mustAgent(t, w, subject).Action.Kind; got != ActMove {
		t.Fatalf("the node is %s, want the order it was waiting to give", got)
	}
}

// A player may aim at no more than their node can see. The screen shows the
// whole world; the node does not, and the node is the one acting.
func TestOrderRefusesATargetTheNodeCannotSee(t *testing.T) {
	w := NewWorld(quietConfig())
	subject := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 40,
		Genome: genomeOf(50, 100, 100)})
	near := w.addAgent(Agent{Maturity: 1,
		X: 220, Y: 200, Sex: Female, Vitality: 80, Hunger: 10,
		Genome: genomeOf(50, 0, 0)})
	far := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 395, Sex: Male, Vitality: 80, Hunger: 10,
		Genome: genomeOf(50, 0, 0)})
	h := takeOver(t, w, subject)

	if err := w.OrderHuman(subject, Action{Kind: ActObserve, TargetID: near, Effort: 0.3}); err != nil {
		t.Fatalf("refused somebody standing next to it: %v", err)
	}
	if err := w.OrderHuman(subject, Action{Kind: ActAttack, TargetID: far, Effort: 1}); !errors.Is(err, ErrOrderUnseen) {
		t.Fatalf("aiming across the world was refused with %v, want %v", err, ErrOrderUnseen)
	}
	if err := w.OrderHuman(subject, Action{Kind: ActAttack, TargetID: subject}); err == nil {
		t.Fatal("let the node attack itself")
	}
	if err := w.OrderHuman(subject, Action{Kind: ActEat}); !errors.Is(err, ErrNoOrderTarget) {
		t.Fatalf("eating nothing in particular was refused with %v, want %v", err, ErrNoOrderTarget)
	}
	if err := w.OrderHuman(subject, Action{Kind: ActMove}); !errors.Is(err, ErrNoDirection) {
		t.Fatalf("a move with no direction was refused with %v, want %v", err, ErrNoDirection)
	}

	// A refused order changes nothing: the last good one still stands.
	if got := h.Standing(); got.Kind != ActObserve || got.TargetID != near {
		t.Fatalf("standing order is %s #%d, want the last one that was accepted",
			got.Kind, got.TargetID)
	}
}

// What is in sight is asked of the world, not of the copy of the perception
// the controller is holding. The copy is as old as the last question, and a
// player looking at the screen is looking at now: an order aimed at something
// that turned up since then must not be refused as unseen.
func TestOrderIsCheckedAgainstWhatIsInSightNow(t *testing.T) {
	cfg := quietConfig()
	cfg.TriggerIdleTicks = 1 << 30 // so the node is not asked again on its own
	cfg.TriggerVitalityDrop = 1e9
	w := NewWorld(cfg)
	subject := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 60,
		Genome: genomeOf(50, 100, 100)})
	h := takeOver(t, w, subject)
	w.Step() // its one and only look at an empty neighbourhood

	if v, _ := h.View(); len(v.Foods) != 0 {
		t.Fatalf("the node saw %d meals in an empty world", len(v.Foods))
	}

	// A meal turns up next to it. Nobody has asked the node anything since, so
	// it is not in the copy the controller holds - but it is in front of it.
	meal := w.addFood(210, 205)
	if err := w.OrderHuman(subject, Action{Kind: ActEat, TargetID: meal, Effort: 0.5}); err != nil {
		t.Fatalf("refused the meal at its feet: %v", err)
	}
	if got := h.Standing(); got.Kind != ActEat || got.TargetID != meal {
		t.Fatalf("standing order is %s #%d, want the meal", got.Kind, got.TargetID)
	}

	// The rule itself has not moved: what is out of sight is still out of
	// bounds, however fresh.
	beyond := w.addFood(200, 395)
	if err := w.OrderHuman(subject, Action{Kind: ActEat, TargetID: beyond}); !errors.Is(err, ErrOrderUnseen) {
		t.Fatalf("a meal across the world was refused with %v, want %v", err, ErrOrderUnseen)
	}
}

// Nobody eats its own kind, so nobody can be ordered to.
func TestOrderRefusesWhatTheNodeWillNotEat(t *testing.T) {
	w := NewWorld(quietConfig())
	subject := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 90,
		Genome: genomeOf(50, 100, 100)})
	takeOver(t, w, subject)

	corpse := w.addFood(210, 200)
	f := w.foodByID(corpse)
	f.Kind, f.From = FoodMeat, SpeciesHuman

	if err := w.OrderHuman(subject, Action{Kind: ActEat, TargetID: corpse}); !errors.Is(err, ErrOrderInedible) {
		t.Fatalf("ordering it to eat one of its own was refused with %v, want %v", err, ErrOrderInedible)
	}
}

// What the player is shown is what the node knows, and no more. The view is a
// copy of the perception, which by construction carries no true ability.
func TestHumanViewIsTheNodesOwnPerception(t *testing.T) {
	w := NewWorld(quietConfig())
	subject := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 40,
		Genome: genomeOf(50, 100, 100)})
	near := w.addAgent(Agent{Maturity: 1,
		X: 215, Y: 200, Sex: Female, Vitality: 80, Hunger: 10,
		Genome: genomeOf(90, 0, 0)})
	w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 395, Sex: Male, Vitality: 80, Hunger: 10,
		Genome: genomeOf(50, 0, 0)})
	food := w.addFood(210, 205)
	h := takeOver(t, w, subject)

	if _, ok := h.View(); ok {
		t.Fatal("has a view before it has ever been asked")
	}
	w.Step()

	v, ok := h.View()
	if !ok {
		t.Fatal("no view after the node was asked")
	}
	if v.Self.ID != subject {
		t.Fatalf("the view is of #%d, want the node itself", v.Self.ID)
	}
	if len(v.Others) != 1 || v.Others[0].ID != near {
		t.Fatalf("sees %d others, want only the one within sight", len(v.Others))
	}
	if _, seen := v.FoodByID(food); !seen {
		t.Fatal("the meal at its feet is not in the view")
	}
	// The estimate, not the truth: what the neighbour can really hit for is a
	// hidden parameter, and a player reads the same guess the AI does.
	other, _ := v.AgentByID(near)
	truth := mustAgent(t, w, near).Attack(&w.cfg)
	if other.EstStrength == truth {
		t.Fatal("the view carries the true power of somebody it has never fought")
	}

	// The world reuses its perception buffer, so the copy has to survive the
	// next agent being asked. This is the whole reason HumanView exists.
	w.Step()
	if v2, _ := h.View(); v2.Self.ID != subject {
		t.Fatalf("the view is of #%d after another tick, want the node itself", v2.Self.ID)
	}
}

// A human decision is recorded like any other. The controller reports no
// options, and the world still notes what prompted the question and what came
// back - which is what makes a played run readable afterwards.
func TestHumanDecisionIsTraced(t *testing.T) {
	w := NewWorld(quietConfig())
	subject := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 40,
		Genome: genomeOf(50, 100, 100)})
	if !w.TrackDecisions(subject, true) {
		t.Fatal("cannot follow the node")
	}
	takeOver(t, w, subject)
	if err := w.OrderHuman(subject, Action{Kind: ActMove, DX: 0, DY: -1, Effort: 0.4}); err != nil {
		t.Fatal(err)
	}
	w.Step()

	tr, ok := w.LastDecisionTrace(subject)
	if !ok {
		t.Fatal("nothing was recorded for a node a person is driving")
	}
	if tr.Trigger != TriggerControllerSet {
		t.Fatalf("recorded trigger %s, want the takeover", tr.Trigger)
	}
	if tr.Action.Kind != ActMove {
		t.Fatalf("recorded action %s, want the order that was given", tr.Action.Kind)
	}
	if len(tr.Options) != 0 {
		t.Fatalf("a human controller reported %d options, want none", len(tr.Options))
	}
}

// The other half of the seam: a line, not a body. Only a grown child can be
// handed the controller, by the same maturity that decides whether an agent may
// court at all.
func TestHeirsAreTheGrownChildren(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)
	parent := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Female, Vitality: 80, Hunger: 20,
		Genome: genomeOf(50, 100, 100)})
	grown := w.addAgent(Agent{Maturity: 1,
		X: 205, Y: 200, Sex: Male, Vitality: 80, Hunger: 20,
		Genome: genomeOf(50, 0, 0)})
	child := w.addAgent(Agent{Maturity: cfg.ReproMaturity / 2,
		X: 210, Y: 200, Sex: Male, Vitality: 80, Hunger: 20,
		Genome: genomeOf(50, 0, 0)})
	dead := w.addAgent(Agent{Maturity: 1,
		X: 215, Y: 200, Sex: Male, Vitality: 80, Hunger: 20,
		Genome: genomeOf(50, 0, 0)})
	p := mustAgent(t, w, parent)
	p.ChildIDs = []int{grown, child, dead}
	mustAgent(t, w, dead).Alive = false

	got := w.Heirs(parent)
	if len(got) != 1 || got[0] != grown {
		t.Fatalf("heirs are %v, want only the living grown child #%d", got, grown)
	}

	// And the handover itself is nothing but installing the same controller on
	// the heir: the world never learns that anything changed hands.
	h := takeOver(t, w, parent)
	if !w.SetController(grown, h) {
		t.Fatal("cannot hand the controller to the heir")
	}
	if err := w.OrderHuman(parent, Action{Kind: ActRest}); err != nil {
		t.Fatal(err)
	}
	w.Step()
	if mustAgent(t, w, grown).Controller() != Controller(h) {
		t.Fatal("the heir is not being driven by the player")
	}
}

// Taking a node over must not change what anybody else does. The fingerprint
// test covers a default run; this covers the run in which one node is driven by
// a person, which is the case the game will actually be played in.
func TestTakingANodeOverLeavesTheRestOfTheWorldAlone(t *testing.T) {
	run := func(play bool) []float64 {
		cfg := DefaultConfig()
		cfg.Seed = 99
		w := NewWorld(cfg)
		if play {
			// The lowest numbered agent, so the same one either way.
			id := w.Agents()[0].ID
			h := NewHumanController()
			w.SetController(id, h)
			for i := 0; i < 600; i++ {
				w.Step()
				if i%50 == 0 {
					_ = w.OrderHuman(id, Action{Kind: ActRest})
					w.RequestDecision(id)
				}
			}
		} else {
			for i := 0; i < 600; i++ {
				w.Step()
			}
		}
		s := w.Stats()
		return []float64{float64(s.Population), float64(s.Births), float64(s.Deaths), float64(s.Kills)}
	}
	// Not equality: the played node itself behaves differently, and that
	// difference spreads. What must hold is that a world with a player in it is
	// still the same kind of world.
	played, ai := run(true), run(false)
	for i := range played {
		if played[i] <= 0 && ai[i] > 0 {
			t.Fatalf("figure %d went to zero once a node was played (%v against %v)", i, played, ai)
		}
	}
	if played[0] < ai[0]/2 || played[0] > ai[0]*2 {
		t.Fatalf("population %v against %v: one resting node changed the world", played[0], ai[0])
	}
}

// An order whose target is gone lapses. Without this a player who ordered a
// meal that somebody else ate would have their node answer "eat that" to every
// tick of the rest of its life, because a standing order does not re-score
// itself the way the AI's comparison does.
func TestOrderLapsesWhenItsTargetIsGone(t *testing.T) {
	w := NewWorld(quietConfig())
	subject := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 60,
		Genome: genomeOf(50, 100, 100)})
	meal := w.addFood(260, 200)
	h := takeOver(t, w, subject)
	w.Step()

	if err := w.OrderHuman(subject, Action{Kind: ActEat, TargetID: meal, Effort: 0.5}); err != nil {
		t.Fatal(err)
	}
	w.RequestDecision(subject)
	w.Step()
	if got := mustAgent(t, w, subject).Action.Kind; got != ActEat {
		t.Fatalf("the node is %s, want it heading for the meal", got)
	}

	// Somebody else gets there first.
	w.removeFoodByID(meal)
	for i := 0; i < 5; i++ {
		w.Step()
	}
	if h.Voided() != 1 {
		t.Fatalf("%d orders lapsed, want the one whose meal was taken", h.Voided())
	}
	if got := h.Standing(); got.Kind != ActRest {
		t.Fatalf("standing order is %s, want it to have lapsed into resting", got.Kind)
	}
	// And the node is not being asked over and over about a meal that is gone.
	asked, _ := h.Asked()
	for i := 0; i < 30; i++ {
		w.Step()
	}
	if again, _ := h.Asked(); again-asked > 2 {
		t.Fatalf("asked %d more times in 30 ticks, want it settled on the lapsed order", again-asked)
	}
}

// An order does not outlive the body it was given to. The same controller
// drives a line rather than an agent, and the heir that takes over may not even
// be able to see what its parent was after.
func TestOrderDoesNotOutliveTheBody(t *testing.T) {
	w := NewWorld(quietConfig())
	parent := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 40,
		Genome: genomeOf(50, 100, 100)})
	rival := w.addAgent(Agent{Maturity: 1,
		X: 215, Y: 200, Sex: Male, Vitality: 80, Hunger: 40,
		Genome: genomeOf(50, 0, 0)})
	heir := w.addAgent(Agent{Maturity: 1,
		X: 380, Y: 380, Sex: Male, Vitality: 80, Hunger: 40,
		Genome: genomeOf(50, 0, 0)})

	h := takeOver(t, w, parent)
	w.Step()
	if err := w.OrderHuman(parent, Action{Kind: ActAttack, TargetID: rival, Effort: 1}); err != nil {
		t.Fatal(err)
	}
	w.RequestDecision(parent)
	w.Step()
	if h.Body() != parent {
		t.Fatalf("the controller reckons it is #%d, want #%d", h.Body(), parent)
	}

	// The line carries on in somebody standing on the other side of the world.
	if !w.SetController(heir, h) {
		t.Fatal("cannot hand over")
	}
	w.Step()
	if h.Body() != heir {
		t.Fatalf("after the handover the controller is #%d, want #%d", h.Body(), heir)
	}
	if got := h.Standing(); got.Kind != ActRest {
		t.Fatalf("the heir took over the order to %s, want it to start with none", got.Kind)
	}
	if got := mustAgent(t, w, heir).Action.Kind; got != ActRest {
		t.Fatalf("the heir is %s, want it resting until it is told something", got)
	}
	// And the view is the heir's own, not the fight it inherited.
	if v, _ := h.View(); v.Self.ID != heir {
		t.Fatalf("the view is of #%d, want the body being driven", v.Self.ID)
	}
}
