package engine

// Answering a proposal (2026-09-06).
//
// Courting takes two, and until now only one of them was ever asked. The
// suitor decides to walk over - that is an action, and a player driving a node
// has always been able to choose it - but the answer was the world's: when the
// suitor arrived, willCommit was evaluated on both sides and the pair either
// formed or did not. Whoever was being courted found out afterwards.
//
// That is fine for a rule and wrong for a game, so the answer can now be given
// by the controller of the one being courted, and the shape of it is the one
// this codebase keeps coming back to: the engine does not know who is behind a
// controller. It asks whoever is there; a controller that does not answer
// leaves the world exactly as it was.
//
// Two properties are worth stating because they are what make this safe:
//
//   - An AI node is untouched. AIController does not implement CourtAnswerer,
//     so its answer is willCommit on the spot, as it has always been - the
//     fingerprint test passes with its numbers unchanged.
//   - Nobody is held for ever. A proposal waits CourtAnswerTicks and then the
//     agent's own rule answers it, so a player who has walked away from the
//     keyboard cannot freeze somebody else's life. The wait is the one place
//     a played node takes up more of the world's time than an AI one would,
//     and it is bounded and in Config.

// CourtAnswer is what a controller says when somebody proposes to its agent.
type CourtAnswer uint8

const (
	// CourtWaiting is "I am going to answer, but not yet". The suitor stands
	// and waits, up to Config.CourtAnswerTicks.
	CourtWaiting CourtAnswer = iota
	// CourtAccept and CourtRefuse are the answer.
	CourtAccept
	CourtRefuse
	// CourtLeaveIt hands the question back to the agent's own rule, which is
	// what happens for every controller that does not implement the interface
	// at all. A controller returns it when nobody is there to answer.
	CourtLeaveIt
)

// CourtAnswerer is a controller that answers proposals itself rather than
// leaving them to the agent's rule. It is an optional interface: the world
// checks for it and carries on unchanged when it is not there.
type CourtAnswerer interface {
	// AnswerCourt is asked once per tick while somebody stands there
	// proposing, with the ID of the suitor. It must be cheap: it is called
	// from the world's own loop, not from a decision.
	AnswerCourt(suitorID int) CourtAnswer
}

// courtAnswerLeft is how long the one standing here will wait before the
// agent's own rule answers for it. Zero when nobody is waiting.
func (w *World) courtAnswerLeft(a *Agent) int {
	if a.courtedBy == 0 {
		return 0
	}
	return max(w.cfg.CourtAnswerTicks-(w.tick-a.courtedTick), 0)
}

// answersOwnCourtships reports whether this agent's controller wants the
// question rather than the rule.
func (w *World) answersOwnCourtships(a *Agent) (CourtAnswerer, bool) {
	c, ok := a.controller.(CourtAnswerer)
	return c, ok
}

// askAboutCourtship gets the answer of the one being courted: theirs to give
// if their controller takes it, and their own rule otherwise. The second
// return says whether there is an answer at all - false means the suitor is
// standing there waiting for one.
//
// The proposal is opened here rather than in court() so that the whole of
// "somebody is waiting on an answer" lives in one file.
func (w *World) askAboutCourtship(o *Agent, suitor *Agent, seen float64) (bool, bool) {
	c, ok := w.answersOwnCourtships(o)
	if !ok {
		return w.willCommit(o, seen), true
	}
	// Ask before disturbing anybody (2026-09-09). A controller that hands the
	// question straight back has to leave its node exactly as an AI node
	// would be, and that includes not being asked to think again: opening the
	// proposal first fired TriggerCourted at a node whose controller did not
	// want the question, and a decision nobody else would have taken is a
	// draw from the random source nobody else would have made. The promise
	// this file makes at the top - an unattended guided node is
	// indistinguishable from an AI one - was true only for as long as no
	// proposal happened to reach one.
	answer := c.AnswerCourt(suitor.ID)
	if answer == CourtLeaveIt {
		w.closeProposal(o)
		return w.willCommit(o, seen), true
	}
	if o.courtedBy != suitor.ID {
		// It has just arrived. Tell the agent it has been asked something,
		// through the same trigger machinery as everything else - an
		// interface finds out there is a proposal by being asked.
		o.courtedBy, o.courtedTick = suitor.ID, w.tick
		o.requestDecision(TriggerCourted)
	}
	switch answer {
	case CourtAccept:
		w.closeProposal(o)
		return true, true
	case CourtRefuse:
		w.closeProposal(o)
		return false, true
	}
	if w.courtAnswerLeft(o) <= 0 {
		// Waited long enough. The rule answers, which is what it would have
		// done immediately if nobody had wanted the question.
		w.closeProposal(o)
		return w.willCommit(o, seen), true
	}
	return false, false
}

// closeProposal forgets the open proposal, however it ended.
func (w *World) closeProposal(a *Agent) {
	a.courtedBy, a.courtedTick = 0, 0
}

// forgetProposalIfGone clears a proposal whose suitor has stopped proposing:
// died, walked off, or changed its mind. Called from the metabolism, because
// the one waiting is not the one being stepped when the suitor gives up.
func (w *World) forgetProposalIfGone(a *Agent) {
	if a.courtedBy == 0 {
		return
	}
	s := w.agentByID(a.courtedBy)
	if s == nil || !s.Alive || s.Action.Kind != ActCourt || s.Action.TargetID != a.ID {
		w.closeProposal(a)
	}
}

// courtDesk is the little bit of state a controller a person is behind keeps
// about the proposal in front of it. Both of them have one, so it lives here
// rather than on either.
//
// answering says somebody is actually there to answer. A controller with it
// off hands every proposal straight back to the agent's rule, which is what
// makes an unattended guided node indistinguishable from an AI one - the
// promise stage 23 made and this file could easily have broken, since a
// proposal nobody answers holds the suitor still for a while.
type courtDesk struct {
	answering bool
	suitor    int
	answer    CourtAnswer
}

func (d *courtDesk) answerCourt(suitor int) CourtAnswer {
	if !d.answering {
		return CourtLeaveIt
	}
	if d.suitor == suitor && d.answer != CourtWaiting {
		a := d.answer
		d.suitor, d.answer = 0, CourtWaiting // spent: an answer is for one proposal
		return a
	}
	if d.suitor != suitor {
		d.suitor, d.answer = suitor, CourtWaiting // somebody else is asking now
	}
	return CourtWaiting
}

// answerProposal is what an interface calls when the person has said yes or no.
// It is kept until the world asks, which is the next tick at the latest.
func (d *courtDesk) answerProposal(suitor int, accept bool) {
	d.suitor = suitor
	if accept {
		d.answer = CourtAccept
		return
	}
	d.answer = CourtRefuse
}
