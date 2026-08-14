package engine

// This file holds the decision trace: the record of why an agent was asked to
// think, what it compared, and what it settled on.
//
// The trace is a development and debugging facility, not a rule: nothing in the
// simulation reads it back. Recording it for everybody would cost more than the
// deciding does, so it is kept per agent and only for the few agents a caller
// has explicitly asked to follow (see World.TrackDecisions).

// traceHistory is how many decisions are kept for a tracked agent. Decisions
// are trigger driven and therefore sparse, so a handful covers a good while.
const traceHistory = 16

// Trigger says what made an agent think again. Deciding is event driven, so
// every trace starts with one of these.
type Trigger uint8

const (
	TriggerNone          Trigger = iota // nothing to reconsider
	TriggerSpawned                      // just entered the world
	TriggerTargetLost                   // what it was after is gone, taken or unsuitable
	TriggerGoalReached                  // the action ran to completion
	TriggerAttacked                     // somebody hit it
	TriggerVitalityDrop                 // it has lost a noticeable amount of vitality
	TriggerIdle                         // nothing has happened for a while
	TriggerFoodInSight                  // food turned up while it had nothing better to do
	TriggerBondEnded                    // its pair broke up
	TriggerControllerSet                // a different controller took it over
	TriggerRequested                    // asked for without a stated reason
)

func (t Trigger) String() string {
	switch t {
	case TriggerSpawned:
		return "spawned"
	case TriggerTargetLost:
		return "target lost"
	case TriggerGoalReached:
		return "goal reached"
	case TriggerAttacked:
		return "attacked"
	case TriggerVitalityDrop:
		return "vitality drop"
	case TriggerIdle:
		return "idle"
	case TriggerFoodInSight:
		return "food in sight"
	case TriggerBondEnded:
		return "bond ended"
	case TriggerControllerSet:
		return "controller set"
	case TriggerRequested:
		return "requested"
	default:
		return "none"
	}
}

// Goal is one thing an option is trying to achieve: what reaching it is worth,
// and how likely this option is to reach it. The utility formula is nothing but
// the sum of these products minus the costs, so keeping the two factors apart
// is what makes a decision explainable rather than merely ranked.
type Goal struct {
	Value  float64
	Chance float64
}

// Score is the contribution this goal makes to an option's utility.
func (g Goal) Score() float64 { return g.Value * g.Chance }

// Utility is the breakdown of one candidate action's score:
//
//	utility = Σ(goal value × chance of reaching it) − risk − vitality cost − time cost
//
// Which goals an option touches depends on what it is: eating buys life,
// courting buys offspring, attacking can buy all of life, a contested meal and
// one fewer mouth to feed later on.
type Utility struct {
	Life      Goal // staying alive: what the option does to the odds of dying
	Stake     Goal // the contested meal winning this would secure
	Rival     Goal // one less competitor for the food that is running short
	Offspring Goal // a child
	Info      Goal // learning how strong somebody really is
	Explore   Goal // finding something to eat that is not in sight yet

	// Risk is the penalty for what this particular agent has already cost the
	// deciding one, which is what keeps it away from somebody it lost to.
	Risk float64

	// The two costs of the formula, already weighted, plus what they were
	// worked out from so that a trace can show both.
	VitalityCost float64
	TimeCost     float64
	Vitality     float64 // vitality the option is expected to spend
	Ticks        float64 // ticks it is expected to take
}

// Total is the option's score. The order of the terms matches the formula in
// the design notes: goals first, then what they cost.
func (u Utility) Total() float64 {
	return u.Life.Score() + u.Stake.Score() + u.Rival.Score() +
		u.Offspring.Score() + u.Info.Score() + u.Explore.Score() -
		u.Risk - u.VitalityCost - u.TimeCost
}

// NamedGoal is a goal together with the name it goes by, for display.
type NamedGoal struct {
	Name string
	Goal
}

// Goals returns the goal terms that actually contributed, so that a viewer can
// print a decision without knowing which option kinds use which terms. It
// allocates and is meant for inspection, not for the simulation loop.
func (u Utility) Goals() []NamedGoal {
	all := [...]NamedGoal{
		{"life", u.Life},
		{"stake", u.Stake},
		{"rival", u.Rival},
		{"offspring", u.Offspring},
		{"info", u.Info},
		{"explore", u.Explore},
	}
	out := make([]NamedGoal, 0, len(all))
	for _, g := range all {
		if g.Value != 0 {
			out = append(out, g)
		}
	}
	return out
}

// TracedOption is one candidate as it was scored.
type TracedOption struct {
	Action  Action
	Utility Utility

	// Noise is the misjudgement the agent's intelligence put on this option's
	// score, and Score is what it actually compared: Utility.Total() + Noise.
	// A dull agent therefore has a visible reason in its trace for picking
	// something that was not the best option on paper.
	Noise float64
	Score float64
}

// DecisionTrace is one call to a controller: why it was asked, what it
// compared, and what it settled on.
//
// Options is empty for a controller that does not fill it in (a human player,
// or a test's fixed controller); Tick, Trigger and Action are always recorded.
type DecisionTrace struct {
	Tick    int
	AgentID int
	Trigger Trigger

	// Self is the deciding agent as it saw itself at that moment, which is
	// what every number below was worked out from.
	Self SelfView

	Options []TracedOption
	Chosen  int // index into Options, -1 when the controller did not say
	Action  Action
}

// traceLog is a per agent ring of the last traceHistory decisions. Entries are
// overwritten in place so that following an agent for a long run does not
// allocate on every decision.
type traceLog struct {
	entries [traceHistory]DecisionTrace
	written int // total decisions recorded, of which the last traceHistory are kept
}

// begin hands out the slot for the next decision, wiped and ready to fill.
func (l *traceLog) begin(tick int, a *Agent, trigger Trigger, self SelfView) *DecisionTrace {
	t := &l.entries[l.written%traceHistory]
	l.written++
	*t = DecisionTrace{
		Tick:    tick,
		AgentID: a.ID,
		Trigger: trigger,
		Self:    self,
		Options: t.Options[:0], // keep the capacity of whatever was here before
		Chosen:  -1,
	}
	return t
}

// list copies the log out, oldest first.
func (l *traceLog) list() []DecisionTrace {
	n := min(l.written, traceHistory)
	out := make([]DecisionTrace, 0, n)
	for i := l.written - n; i < l.written; i++ {
		t := l.entries[i%traceHistory]
		t.Options = append([]TracedOption(nil), t.Options...)
		out = append(out, t)
	}
	return out
}

// --- the world side --------------------------------------------------------

// TrackDecisions starts or stops recording the decisions of one agent. Only
// tracked agents pay anything for the trace, which is why it is off by default:
// it is meant for watching a single node closely, not for the whole population.
//
// The log survives the agent, so a caller can still read why it did what it did
// after it has died. It returns false if there is no such agent.
func (w *World) TrackDecisions(id int, on bool) bool {
	if !on {
		delete(w.traces, id)
		if a := w.agentByID(id); a != nil {
			a.trace = nil
		}
		return true
	}
	a := w.agentByID(id)
	if a == nil {
		return false
	}
	if w.traces == nil {
		w.traces = make(map[int]*traceLog, 1)
	}
	log, ok := w.traces[id]
	if !ok {
		log = &traceLog{}
		w.traces[id] = log
	}
	a.trace = log
	return true
}

// IsTracked reports whether an agent's decisions are being recorded.
func (w *World) IsTracked(id int) bool {
	_, ok := w.traces[id]
	return ok
}

// DecisionTraces returns the recorded decisions of a tracked agent, oldest
// first. The result is a copy, so it stays valid while the world runs on.
func (w *World) DecisionTraces(id int) []DecisionTrace {
	log, ok := w.traces[id]
	if !ok {
		return nil
	}
	return log.list()
}

// LastDecisionTrace returns the most recent decision of a tracked agent.
func (w *World) LastDecisionTrace(id int) (DecisionTrace, bool) {
	log, ok := w.traces[id]
	if !ok || log.written == 0 {
		return DecisionTrace{}, false
	}
	t := log.entries[(log.written-1)%traceHistory]
	t.Options = append([]TracedOption(nil), t.Options...)
	return t, true
}
