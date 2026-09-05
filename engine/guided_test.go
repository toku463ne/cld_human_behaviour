package engine

import (
	"errors"
	"testing"
)

// Stage 23: the controller that plays the node and stops to ask. What these
// tests hold in place is that asking is free of consequence - a node nobody
// answers is a node the AI is driving, bit for bit - and that an answer is one
// move rather than a standing order.

// guide installs a fresh guided controller on an agent and returns it.
func guide(t *testing.T, w *World, id int) *GuidedController {
	t.Helper()
	c := NewGuidedController()
	if !w.SetController(id, c) {
		t.Fatalf("cannot take agent %d over", id)
	}
	return c
}

// crowdedWorld is a world with enough going on that turning points happen: a
// population, food growing, and everybody deciding for themselves.
func crowdedWorld(t *testing.T, seed int64) *World {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Seed = seed
	return NewWorld(cfg)
}

// A question costs the world nothing. A node whose questions nobody answers is
// driven by the same comparison as one nobody is asking about, down to the last
// bit - which is what makes it safe to leave the game running in a world that
// is also being measured.
func TestUnansweredGuidingChangesNothing(t *testing.T) {
	run := func(guided bool) []Agent {
		w := crowdedWorld(t, 7)
		for i := 0; i < 200; i++ {
			w.Step()
		}
		id := w.Agents()[3].ID
		// Both worlds install a controller, because installing one is itself a
		// trigger: comparing against an untouched agent would be comparing the
		// question with the timing of the question.
		if guided {
			w.SetController(id, NewGuidedController())
		} else {
			w.SetController(id, &AIController{})
		}
		for i := 0; i < 2000; i++ {
			w.Step()
		}
		return w.Agents()
	}

	asked, plain := run(true), run(false)
	if len(asked) != len(plain) {
		t.Fatalf("population %d with the questions, %d without", len(asked), len(plain))
	}
	for i := range asked {
		if asked[i].ID != plain[i].ID || asked[i].X != plain[i].X ||
			asked[i].Y != plain[i].Y || asked[i].Vitality != plain[i].Vitality {
			t.Fatalf("agent %d diverged: %+v vs %+v", i, asked[i], plain[i])
		}
	}
}

// Being hit is a turning point and raises a question; finishing what you were
// doing is not, and does not.
func TestQuestionsComeAtTurningPoints(t *testing.T) {
	cfg := quietConfig()
	cfg.TriggerIdleTicks = 1 << 30
	w := NewWorld(cfg)
	subject := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 40,
		Genome: genomeOf(50, 100, 100)})
	c := guide(t, w, subject)

	// Taken over, then left alone: the takeover is not a turning point.
	w.Step()
	if _, ok := c.Question(); ok {
		t.Fatal("asked about being taken over, want only the turning points")
	}

	bully := w.addAgent(Agent{Maturity: 1,
		X: 205, Y: 200, Sex: Male, Vitality: 100, Hunger: 40,
		Genome: genomeOf(90, 100, 100)})
	w.SetController(bully, fixedController{Action{Kind: ActAttack, TargetID: subject, Effort: 1}})
	w.Step()
	w.Step()

	q, ok := c.Question()
	if !ok {
		t.Fatal("hit and not asked about it")
	}
	if q.Trigger != TriggerAttacked {
		t.Fatalf("asked because of %s, want being hit", q.Trigger)
	}
	if q.AgentID != subject {
		t.Fatalf("asked on behalf of #%d, want #%d", q.AgentID, subject)
	}
	if len(q.Options) == 0 {
		t.Fatal("a question with nothing to choose between")
	}
}

// The one question a person raises for themselves. Turning points are rare on
// purpose, so without this the only thing to do between them is watch.
func TestAskingOnDemandRaisesAQuestion(t *testing.T) {
	cfg := quietConfig()
	cfg.TriggerIdleTicks = 1 << 30
	cfg.TriggerVitalityDrop = 1e9
	w := NewWorld(cfg)
	subject := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 40,
		Genome: genomeOf(50, 100, 100)})
	c := guide(t, w, subject)

	w.Step()
	if _, ok := c.Question(); ok {
		t.Fatal("asked without being asked to ask")
	}
	if !w.RequestDecision(subject) {
		t.Fatal("the world would not put the question")
	}
	w.Step()
	q, ok := c.Question()
	if !ok {
		t.Fatal("asked for a question and did not get one")
	}
	if q.Trigger != TriggerRequested {
		t.Fatalf("asked because of %s, want because it was asked for", q.Trigger)
	}
}

// A fight re-triggers on every tick of itself. One question covers the fight,
// or a person is being asked nothing at all.
func TestAFightAsksOnce(t *testing.T) {
	cfg := quietConfig()
	cfg.TriggerIdleTicks = 1 << 30
	w := NewWorld(cfg)
	subject := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 500, Hunger: 40,
		Genome: genomeOf(50, 100, 100)})
	c := guide(t, w, subject)
	bully := w.addAgent(Agent{Maturity: 1,
		X: 205, Y: 200, Sex: Male, Vitality: 500, Hunger: 40,
		Genome: genomeOf(90, 100, 100)})
	w.SetController(bully, fixedController{Action{Kind: ActAttack, TargetID: subject, Effort: 1}})

	raised, lastTick := 0, -1
	for i := 0; i < 15; i++ {
		w.Step()
		if q, ok := c.Question(); ok && q.Tick != lastTick {
			raised, lastTick = raised+1, q.Tick
		}
	}
	if raised != 1 {
		t.Fatalf("asked %d times in one fight, want once", raised)
	}
}

// The options are a choice, not a comparison sheet: one entry per thing that
// can be done to one target, best first as this node scores it.
func TestQuestionOptionsAreAChoice(t *testing.T) {
	w := crowdedWorld(t, 3)
	for i := 0; i < 400; i++ {
		w.Step()
	}
	var c *GuidedController
	var q Question
	for _, a := range w.Agents() {
		if a.Species != SpeciesHuman {
			continue
		}
		cc := guide(t, w, a.ID)
		for i := 0; i < 400; i++ {
			w.Step()
			if got, ok := cc.Question(); ok {
				c, q = cc, got
				break
			}
		}
		if c != nil {
			break
		}
	}
	if c == nil {
		t.Skip("no turning point came up in this run")
	}

	if len(q.Options) > maxQuestionOptions {
		t.Fatalf("%d options, want at most %d", len(q.Options), maxQuestionOptions)
	}
	seen := map[[2]int]bool{}
	for i, o := range q.Options {
		key := [2]int{int(o.Action.Kind), o.Action.TargetID}
		if seen[key] {
			t.Fatalf("option %d repeats %s on #%d at another effort",
				i, o.Action.Kind, o.Action.TargetID)
		}
		seen[key] = true
		if i > 0 && o.Score > q.Options[i-1].Score {
			t.Fatalf("option %d scores above the one before it: %v > %v",
				i, o.Score, q.Options[i-1].Score)
		}
	}
}

// The field is trimmed to a handful, but never by deleting a whole kind of
// action. Courting and eating are the two the trim would take first - a fed
// node scores neither highly - and they are the two a person is most likely to
// want to overrule it about.
func TestTheFieldKeepsOneOfEachKind(t *testing.T) {
	all := make([]TracedOption, 0, maxQuestionOptions+6)
	// A long tail of high scoring attacks, so a plain top-N would be nothing
	// else, and one poor option of each other kind underneath it.
	for i := 0; i < maxQuestionOptions+2; i++ {
		all = append(all, TracedOption{
			Action: Action{Kind: ActAttack, TargetID: i + 1},
			Score:  100 - float64(i),
		})
	}
	rare := []ActionKind{ActCourt, ActEat, ActRest, ActFlee}
	for i, k := range rare {
		all = append(all, TracedOption{Action: Action{Kind: k, TargetID: 900 + i}, Score: -50})
	}

	kept := keepAKindOfEach(all)
	if len(kept) != maxQuestionOptions {
		t.Fatalf("kept %d options, want %d", len(kept), maxQuestionOptions)
	}
	seen := map[ActionKind]bool{}
	for _, o := range kept {
		seen[o.Action.Kind] = true
	}
	for _, k := range rare {
		if !seen[k] {
			t.Fatalf("%s was scored and did not survive the trim", k)
		}
	}
	if kept[0].Score != 100 {
		t.Fatalf("the best option is no longer first: %v", kept[0].Score)
	}
}

// An answer is one move. It is carried out at once, and then the node is back
// to scoring for itself: nothing has to void it when the world moves on.
func TestAnAnswerIsOneMoveAndIsSpent(t *testing.T) {
	cfg := quietConfig()
	cfg.TriggerIdleTicks = 1 << 30
	w := NewWorld(cfg)
	subject := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 400, Hunger: 40,
		Genome: genomeOf(50, 100, 100)})
	c := guide(t, w, subject)
	bully := w.addAgent(Agent{Maturity: 1,
		X: 205, Y: 200, Sex: Male, Vitality: 400, Hunger: 40,
		Genome: genomeOf(90, 100, 100)})
	w.SetController(bully, fixedController{Action{Kind: ActAttack, TargetID: subject, Effort: 1}})

	w.Step()
	w.Step()
	q, ok := c.Question()
	if !ok {
		t.Fatal("hit and not asked about it")
	}

	// Whichever option is not what it was already doing, so that taking it up
	// is visible.
	pick := -1
	for i, o := range q.Options {
		if o.Action.Kind != mustAgent(t, w, subject).Action.Kind {
			pick = i
			break
		}
	}
	if pick < 0 {
		t.Skip("every option was what it was already doing")
	}
	want := q.Options[pick].Action
	if err := w.Answer(subject, pick); err != nil {
		t.Fatalf("refused a live answer: %v", err)
	}
	w.Step()
	if got := mustAgent(t, w, subject).Action; got.Kind != want.Kind || got.TargetID != want.TargetID {
		t.Fatalf("the world had it %s on #%d, want the answer (%s on #%d)",
			got.Kind, got.TargetID, want.Kind, want.TargetID)
	}
	if _, ok := c.Question(); ok {
		t.Fatal("the question still stands after being answered")
	}
	if _, _, answered := c.Asked(); answered != 1 {
		t.Fatalf("%d answers taken up, want 1", answered)
	}
	// Spent: the next question is the node's own again, not the same answer.
	if err := w.Answer(subject, 0); !errors.Is(err, ErrNoQuestion) {
		t.Fatalf("answering twice gave %v, want %v", err, ErrNoQuestion)
	}
}

// A question the world has moved past is not answerable. A late answer is not
// a slow player being helped; it is the node acting on a world that is gone.
func TestAStaleQuestionIsRefused(t *testing.T) {
	cfg := quietConfig()
	cfg.TriggerIdleTicks = 1 << 30
	w := NewWorld(cfg)
	subject := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 400, Hunger: 40,
		Genome: genomeOf(50, 100, 100)})
	c := guide(t, w, subject)
	c.Pace(20, 5)
	bully := w.addAgent(Agent{Maturity: 1,
		X: 205, Y: 200, Sex: Male, Vitality: 400, Hunger: 40,
		Genome: genomeOf(90, 100, 100)})
	w.SetController(bully, fixedController{Action{Kind: ActAttack, TargetID: subject, Effort: 1}})

	w.Step()
	w.Step()
	if _, ok := c.Question(); !ok {
		t.Fatal("hit and not asked about it")
	}
	for i := 0; i < 10; i++ {
		w.Step()
	}
	if _, ok := w.Question(subject); ok {
		t.Fatal("a question older than its grace is still on offer")
	}
	if err := w.Answer(subject, 0); !errors.Is(err, ErrQuestionStale) {
		t.Fatalf("late answer gave %v, want %v", err, ErrQuestionStale)
	}
}

// The promise of stage 19 covers this way of playing too: a node may only be
// aimed at what it can see, whether the aim came from a key or from a menu.
func TestAnAnswerCannotAimAtWhatIsGone(t *testing.T) {
	cfg := quietConfig()
	cfg.TriggerIdleTicks = 1 << 30
	w := NewWorld(cfg)
	subject := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 400, Hunger: 40,
		Genome: genomeOf(50, 100, 100)})
	c := guide(t, w, subject)
	bully := w.addAgent(Agent{Maturity: 1,
		X: 205, Y: 200, Sex: Male, Vitality: 400, Hunger: 40,
		Genome: genomeOf(90, 100, 100)})
	w.SetController(bully, fixedController{Action{Kind: ActAttack, TargetID: subject, Effort: 1}})

	w.Step()
	w.Step()
	q, ok := c.Question()
	if !ok {
		t.Fatal("hit and not asked about it")
	}
	pick := -1
	for i, o := range q.Options {
		if o.Action.TargetID == bully {
			pick = i
			break
		}
	}
	if pick < 0 {
		t.Skip("no option was aimed at the one doing the hitting")
	}

	// Walk the target out of sight without ending the question.
	b := mustAgent(t, w, bully)
	b.X, b.Y = 20, 20
	if err := w.Answer(subject, pick); !errors.Is(err, ErrOrderUnseen) {
		t.Fatalf("aiming at somebody out of sight gave %v, want %v", err, ErrOrderUnseen)
	}
}

// A handover is a new body. Whatever was asked of the one that died, and
// whatever was answered for it, belongs to it.
func TestAHandoverDropsTheQuestion(t *testing.T) {
	cfg := quietConfig()
	cfg.TriggerIdleTicks = 1 << 30
	w := NewWorld(cfg)
	subject := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 400, Hunger: 40,
		Genome: genomeOf(50, 100, 100)})
	c := guide(t, w, subject)
	bully := w.addAgent(Agent{Maturity: 1,
		X: 205, Y: 200, Sex: Male, Vitality: 400, Hunger: 40,
		Genome: genomeOf(90, 100, 100)})
	w.SetController(bully, fixedController{Action{Kind: ActAttack, TargetID: subject, Effort: 1}})
	w.Step()
	w.Step()
	if _, ok := c.Question(); !ok {
		t.Fatal("hit and not asked about it")
	}

	heir := w.addAgent(Agent{Maturity: 1,
		X: 300, Y: 300, Sex: Male, Vitality: 80, Hunger: 40,
		Genome: genomeOf(50, 100, 100)})
	w.SetController(heir, c)
	w.Step()
	if _, ok := c.Question(); ok {
		t.Fatal("the heir is being asked what its parent was asked")
	}
	if c.Body() != heir {
		t.Fatalf("the controller answers for #%d, want the heir #%d", c.Body(), heir)
	}
}

// --- the milestone half: disposition ---------------------------------------

// What a person splits is what the node inherited. Being more careful is paid
// for out of the other two, which is what makes it a choice.
func TestDispositionKeepsWhatThereIsToDivide(t *testing.T) {
	w := NewWorld(quietConfig())
	subject := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 40,
		Genome: genomeOf(50, 100, 100)})

	was, ok := w.Disposition(subject)
	if !ok {
		t.Fatal("no disposition on a living node")
	}
	// A node built by hand assumes the world's own figures, so it starts at one
	// share each.
	approx(t, was.Total(), 3, 1e-9, "what there is to divide")

	// Asked for in whatever units the interface felt like: the proportions are
	// what is read.
	if err := w.SetDisposition(subject, Disposition{Risk: 60, Competition: 20, Shock: 20}); err != nil {
		t.Fatal(err)
	}
	now, _ := w.Disposition(subject)
	approx(t, now.Total(), was.Total(), 1e-9, "what there is to divide, after")
	approx(t, now.Risk, 1.8, 1e-9, "the careful share")
	approx(t, now.Competition, 0.6, 1e-9, "the rival share")

	// And the utility formula reads the new figures, not the old ones.
	cfg := DefaultConfig()
	approx(t, mustAgent(t, w, subject).Assumes().RiskWeight, 1.8*cfg.RiskWeight, 1e-9, "what it now brings")
}

// The ceiling of stage 12a still holds: a split that puts one preference past
// it is not a split this node could have.
func TestDispositionRefusesTheImpossible(t *testing.T) {
	w := NewWorld(quietConfig())
	subject := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 80, Hunger: 40,
		Genome: genomeOf(50, 100, 100)})

	// Three shares to divide, all of them into one preference: past the four
	// times ceiling? No - but ten times over is.
	if err := w.SetDisposition(subject, Disposition{Risk: 1}); err != nil {
		t.Fatalf("refused everything in one preference: %v", err)
	}
	now, _ := w.Disposition(subject)
	approx(t, now.Risk, 3, 1e-9, "everything in one preference")

	if err := w.SetDisposition(subject, Disposition{}); !errors.Is(err, ErrDisposition) {
		t.Fatalf("dividing by nothing gave %v, want %v", err, ErrDisposition)
	}
	if err := w.SetDisposition(subject, Disposition{Risk: 1, Competition: -1, Shock: 1}); !errors.Is(err, ErrDisposition) {
		t.Fatalf("a negative share gave %v, want %v", err, ErrDisposition)
	}
}
