package engine

import (
	"errors"
	"fmt"
	"sort"
)

// This file holds the controller a person shares with the AI (stage 23). It is
// the third implementation of Controller, and it exists because of what stage
// 19 found by being played: the perception carried enough to decide with and
// the seven actions were enough to say it, but a node is asked to think again
// every fourteen ticks, and a person cannot be the one answering that.
//
// So this controller answers all of them, and puts a few of them to a person.
// Two things follow from that, and both are the point:
//
//   - The AI is not a fallback that runs when the player is idle. It is what
//     the node does, always. A player who answers nothing plays a node that
//     lives its own life, and a player who answers everything only ever
//     answers at a turning point.
//   - A question is raised out of the comparison the node actually made, not
//     out of a menu written for it. The options are the ones it scored, with
//     its own misjudgement (Noise) already in them, so a player driving a dull
//     node is offered a dull node's ranking. Showing the noise free ranking
//     would hand the player a sense the node's genes did not buy.
//
// What a person answers is one move, not a standing order. Stage 19's standing
// order had to be voided by hand when its target went away; an answer is spent
// the moment it is taken up, and the node is back to scoring for itself.

// Errors an answer can be refused with. Aiming is refused with the same errors
// an order is (ErrOrderUnseen, ErrOrderInedible): the promise that a node may
// only be aimed at what it can see does not care which of the two ways of
// playing produced the aim.
var (
	// ErrNoQuestion is an answer to nothing.
	ErrNoQuestion = errors.New("nothing was asked")
	// ErrQuestionStale is an answer to a question the world has moved past.
	ErrQuestionStale = errors.New("that question is out of date")
	// ErrNoSuchOption is a choice that was not on offer.
	ErrNoSuchOption = errors.New("no such option")
)

func errNotGuided(id int) error {
	return fmt.Errorf("node #%d is not being asked", id)
}

// maxQuestionOptions caps how many choices a question carries. The AI scores
// every food item and every neighbour at two efforts, which is tens of
// options, and one entry per thing that can be done to one target already cuts
// that down a long way.
//
// It is larger than a menu would be on purpose. What an interface should put
// in front of a person is a handful, but which handful is a question about
// presentation - the safest option and the boldest one are not near each other
// in this ranking - so the controller hands over the field and lets whoever is
// drawing it decide what a choice looks like.
const maxQuestionOptions = 12

// Question is a decision the world put to a person: why the node was asked,
// what it knew at the time, and what it was choosing between.
//
// Options are best first as this node ranks them, which means Options[0] is
// what it did while the question stood. Answering with 0 is therefore the same
// as not answering, and that is deliberate: "let it be" needs no special case.
type Question struct {
	Tick    int
	AgentID int
	Trigger Trigger
	Self    SelfView
	Options []TracedOption
}

// GuidedController drives a node with the utility comparison and stops to ask
// a person at the triggers it was told are worth stopping for.
type GuidedController struct {
	ai AIController

	// turning says which triggers raise a question. Being hit is the loudest
	// of them by far - half of all decisions - so it is collapsed to one
	// question per fight by fightGap rather than left out.
	turning  [numTriggers]bool
	fightGap int
	// grace is how long an unanswered question stands. A stale answer is worse
	// than none: the player is answering about a world that has moved on, and
	// the node has been living in the new one the whole time.
	grace int

	q     Question
	haveQ bool
	// lastFight is the tick of the last question raised by being hit.
	lastFight int

	answer     Action
	haveAnswer bool

	view     HumanView
	haveView bool

	// scratch is where the AI writes the comparison when nobody is tracing the
	// agent. Its slice is reused, so a question takes a copy.
	scratch DecisionTrace

	body           int
	asked, askedAt int
	answered       int
}

// NewGuidedController returns a controller that asks about the turning points:
// a fight starting, what it was after being gone, somebody worth crossing the
// world for coming into view, a pair breaking up, and a noticeable dent in the
// vitality. Measured on the default world, that is one question every 112
// ticks against one decision every 14.5 - about thirty in a lifetime.
//
// Left to the AI are the two that are neither rare nor a turning point:
// finishing what it was doing (goal reached) and having nothing to do (idle).
//
// TriggerRequested is in the set as well, and it is the one a person raises
// themselves: nothing in the world ever produces it. Without it, playing is
// waiting - the only thing a player can do between turning points is watch,
// and a hundred ticks of watching feels like a game that has stopped
// responding. With it, "what are you thinking?" is always available and goes
// through the same machinery as every other question.
func NewGuidedController() *GuidedController {
	c := &GuidedController{fightGap: 20, grace: 60, lastFight: -1 << 30}
	for _, t := range []Trigger{
		TriggerAttacked, TriggerTargetLost, TriggerMateInSight,
		TriggerBondEnded, TriggerVitalityDrop, TriggerRequested,
	} {
		c.turning[t] = true
	}
	return c
}

// AskAbout sets which triggers raise a question, replacing whatever was there.
// It is how a game with a different appetite for interruption is built without
// a second controller.
func (c *GuidedController) AskAbout(ts ...Trigger) {
	c.turning = [numTriggers]bool{}
	for _, t := range ts {
		if t < numTriggers {
			c.turning[t] = true
		}
	}
}

// Pace sets how long a fight goes before it may ask again, and how long a
// question stands unanswered.
func (c *GuidedController) Pace(fightGap, grace int) {
	c.fightGap, c.grace = fightGap, grace
}

// Decide answers with the person's choice if one is waiting, and otherwise
// with the node's own comparison - raising a question on the way out if this
// was a moment worth stopping for.
func (c *GuidedController) Decide(p *Perception) Action {
	// A new body. The line is what a player drives, not an agent, so a
	// handover drops everything that belonged to the one who died: an answer
	// given for that body, and the question it was an answer to.
	if c.body != 0 && p.Self.ID != c.body {
		c.haveQ, c.haveAnswer = false, false
		c.lastFight = -1 << 30
	}
	c.body = p.Self.ID
	c.asked++
	c.askedAt = p.Tick
	// The same copy the driven controller keeps, for the same reason: a panel
	// showing a player what their node knows must read the node's perception
	// and nothing else.
	c.view.copyOf(p)
	c.haveView = true

	if c.haveAnswer {
		// Spent, not standing. The node scores for itself again from here, so
		// nothing has to notice that the answer went stale.
		c.haveAnswer, c.haveQ = false, false
		c.answered++
		return c.answer
	}

	ask := c.worthAsking(p)
	if ask && p.Trace == nil {
		// Nobody is following this agent, so there is no comparison written
		// down anywhere. Ask for one: this is the only thing a question costs.
		c.scratch = DecisionTrace{
			Tick:    p.Tick,
			AgentID: p.Self.ID,
			Trigger: p.Trigger,
			Self:    p.Self,
			Options: c.scratch.Options[:0],
			Chosen:  -1,
		}
		p.Trace = &c.scratch
	}
	act := c.ai.Decide(p)
	if ask && p.Trace != nil {
		c.raise(p, p.Trace)
	}
	return act
}

// worthAsking reports whether this is a moment to stop a person for, and keeps
// the book on fights while it is at it.
func (c *GuidedController) worthAsking(p *Perception) bool {
	if p.Trigger >= numTriggers || !c.turning[p.Trigger] {
		return false
	}
	if p.Trigger == TriggerAttacked {
		// One question per fight. Being hit re-triggers on every tick of an
		// engagement, and a person asked "you are being hit, what now?" thirty
		// times in a row is being asked nothing at all.
		started := p.Tick-c.lastFight >= c.fightGap
		c.lastFight = p.Tick
		return started
	}
	return true
}

// raise turns the comparison the node just made into a question.
func (c *GuidedController) raise(p *Perception, tr *DecisionTrace) {
	if len(tr.Options) == 0 {
		return
	}
	c.q.Tick, c.q.AgentID, c.q.Trigger, c.q.Self = p.Tick, p.Self.ID, p.Trigger, p.Self
	c.q.Options = c.q.Options[:0]

	// One entry per thing that can be done to one target. The comparison holds
	// every option at each effort, and "eat that at 0.4" next to "eat that at
	// 1.0" is not a choice a person wants to be given: it is the same choice
	// with the arithmetic left in.
	best := make(map[[2]int]int, len(tr.Options))
	for i := range tr.Options {
		o := &tr.Options[i]
		key := [2]int{int(o.Action.Kind), o.Action.TargetID}
		if at, ok := best[key]; ok {
			if c.q.Options[at].Score >= o.Score {
				continue
			}
			c.q.Options[at] = *o
			continue
		}
		best[key] = len(c.q.Options)
		c.q.Options = append(c.q.Options, *o)
	}

	// Best first as this node scores it - which is with its own misjudgement
	// included, so the ranking a player is shown is the ranking the node
	// believes in rather than the one that is true.
	sort.SliceStable(c.q.Options, func(i, j int) bool {
		return c.q.Options[i].Score > c.q.Options[j].Score
	})
	c.q.Options = keepAKindOfEach(c.q.Options)
	c.haveQ = true
}

// keepAKindOfEach trims the field to maxQuestionOptions without letting a whole
// kind of action fall off the end.
//
// A plain "top twelve by score" can delete every option of a kind, and the two
// that go first are the two that matter most to a person: courting is worth
// little to a node with a full belly and a long life left, and eating is worth
// little to one that has just eaten. Those are exactly the moments when a
// player wants to overrule it, and a menu that has quietly dropped the option
// is not a menu they can overrule it with.
//
// The input must already be best first.
func keepAKindOfEach(sorted []TracedOption) []TracedOption {
	if len(sorted) <= maxQuestionOptions {
		return sorted
	}
	keep := make([]bool, len(sorted))
	seen := [numActionKinds]bool{}
	room := maxQuestionOptions
	for i := range sorted {
		k := sorted[i].Action.Kind
		if k < numActionKinds && !seen[k] {
			seen[k], keep[i] = true, true
			room--
		}
	}
	for i := range sorted {
		if room <= 0 {
			break
		}
		if !keep[i] {
			keep[i], room = true, room-1
		}
	}
	out := sorted[:0]
	for i := range sorted {
		if keep[i] {
			out = append(out, sorted[i])
		}
	}
	return out
}

// Question returns the question waiting for an answer, if there is one. The
// options are the controller's own slice: read them, do not keep them.
func (c *GuidedController) Question() (Question, bool) {
	if !c.haveQ {
		return Question{}, false
	}
	return c.q, true
}

// View is the perception this node was last handed, which is the whole of what
// may be shown to a person. It is the controller's own copy: read it, do not
// keep it.
func (c *GuidedController) View() (HumanView, bool) { return c.view, c.haveView }

// Asked reports how many times the engine has put the question, the tick of
// the last one, and how many of them a person answered. The three together are
// what says whether a game is being played or watched.
func (c *GuidedController) Asked() (count, tick, answered int) {
	return c.asked, c.askedAt, c.answered
}

// Body is the agent this controller last answered for, 0 before its first
// question. An interface reads it to tell a handover from a fresh start.
func (c *GuidedController) Body() int { return c.body }

// --- the world side --------------------------------------------------------

// Question returns the choice put to whoever is driving this node.
func (w *World) Question(id int) (Question, bool) {
	c, _ := w.guided(id)
	if c == nil {
		return Question{}, false
	}
	q, ok := c.Question()
	if !ok || w.tick-q.Tick > c.grace {
		return Question{}, false
	}
	return q, true
}

// Answer takes one option of the standing question. The chosen action is what
// the node answers with the next time it is asked, and it is asked at once.
//
// The world does the checking rather than the controller, for the same reason
// OrderHuman does: only the world knows what is in sight now, and a question
// raised sixty ticks ago is a list of things that were.
func (w *World) Answer(id, option int) error {
	c, a := w.guided(id)
	if c == nil {
		return errNotGuided(id)
	}
	q, ok := c.Question()
	if !ok {
		return ErrNoQuestion
	}
	if w.tick-q.Tick > c.grace {
		c.haveQ = false
		return ErrQuestionStale
	}
	if option < 0 || option >= len(q.Options) {
		return ErrNoSuchOption
	}
	act := q.Options[option].Action
	switch act.Kind {
	case ActRest, ActMove:
	case ActEat:
		if !w.CanTargetFood(id, act.TargetID) {
			if f := w.foodByID(act.TargetID); f != nil && w.canSee(a.X, a.Y, f.X, f.Y) {
				return ErrOrderInedible
			}
			return ErrOrderUnseen
		}
	default:
		if !w.CanTargetAgent(id, act.TargetID) {
			return ErrOrderUnseen
		}
	}
	c.answer, c.haveAnswer = act, true
	a.requestDecision(TriggerRequested)
	return nil
}

// LetItBe drops the standing question without answering it, which leaves the
// node doing what it chose for itself. It is not the same as answering with
// the first option: that one is asked again, and this one is not.
func (w *World) LetItBe(id int) bool {
	c, _ := w.guided(id)
	if c == nil || !c.haveQ {
		return false
	}
	c.haveQ = false
	return true
}

func (w *World) guided(id int) (*GuidedController, *Agent) {
	a := w.agentByID(id)
	if a == nil || !a.Alive {
		return nil, nil
	}
	c, ok := a.controller.(*GuidedController)
	if !ok {
		return nil, nil
	}
	return c, a
}
