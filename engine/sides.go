package engine

// Whose side a body is on (TODO 14, #138).
//
// What the world does not have yet. Affinity decides a great deal about what
// an agent does around somebody - whether it rests near them, what it hands
// over, what it takes them to be worth watching - but it decides nothing
// about fighting. scoreFight reads how strong the other one is, how close,
// how hungry this body is and what that one has cost it before; it does not
// read, anywhere, whether the two of them are friends. Two bodies racing for
// the same plant are priced the same whether they grew up together or met a
// moment ago.
//
// Nor is there a way to fall out. addAffinity throws away anything that is
// not positive, so the only thing that can happen between two agents is that
// they come to matter more to each other. Being robbed of the meal you were
// walking towards is not an event this world records at all.
//
// This file starts with counting, because the two rules that would change
// that are both aimed at things nothing has measured. A rule that hardly
// ever fires explains nothing however large its weight (stage 24), and this
// project has five measurements of information that changed nobody's mind
// (stages 31, 35, 49). So before either rule is written: how often does a
// body actually see a friend fighting somebody it likes less, and how often
// does somebody eat the very item it was walking towards?
//
// Everything here is read only. Nothing draws a random number, nothing
// changes a decision, and with all of it in place every world runs exactly
// as it did.

// noteSnatched counts the bodies that were walking towards the item this one
// just ate. It is called from eat, before the item is removed, and it draws
// no random numbers and changes nothing.
//
// The scan is over every agent rather than over the ones nearby: eat runs
// inside the loop that moves bodies, and asking the spatial index there would
// rebuild it once per meal (the coding rule in CLAUDE.md). A straight pass
// over the slice costs a couple of field reads per body and no allocation.
func (w *World) noteSnatched(eater *Agent, foodID int) {
	for i := range w.agents {
		o := &w.agents[i]
		if !o.Alive || o.ID == eater.ID {
			continue
		}
		if o.Action.Kind != ActEat || o.Action.TargetID != foodID {
			continue
		}
		w.snatched++
		// What it already thought of the one that beat it to it. The decayed
		// figure, because that is what every other reader of affinity uses.
		if op := o.opinion(eater.ID); op != nil && w.decayedAffinity(o, op) > 0 {
			w.snatchedFriend++
		}
		// And what it thinks of them now (TODO 14, (vi)). The forced path,
		// like being hit and unlike a trade: this is something that was done
		// to this body, and a memory that is full must not be the reason it
		// fails to notice. Nine tenths of the time the one it is recording is
		// a stranger, so what this mostly does is push a nought below nought
		// for the first time - which is why it is worth nothing without (a)
		// and nothing without something that reads the result.
		if w.cfg.AffinitySnatched > 0 && w.cfg.AffinityNegative {
			w.addAffinity(o, eater.ID, -w.cfg.AffinitySnatched, true)
			w.snatchBites++
		}
	}
}

// noteFightChoice counts who a body chooses to swing at (TODO 14). Two
// numbers: how many decisions were an attack, and how many of those were aimed
// at somebody the body thinks well of.
//
// It exists because the metric that was there could not answer the question
// the rules are about. fightCompanion and fightStranger split fights by
// cluster - who happens to be standing in the same crowd - and (i) is about
// affinity, which is not the same thing at all: a crowd is where a body's
// friends are, but it is also where the strangers it races are. Measuring a
// rule about goodwill with a ruler made of geography is how a rule looks
// inert when it is working, or works when it is inert.
//
// Read only, and taken whether or not any of the rules are on.
func (w *World) noteFightChoice(a *Agent) {
	if a.Action.Kind != ActAttack || a.Action.TargetID == 0 {
		return
	}
	w.fightChoices++
	if op := a.opinion(a.Action.TargetID); op != nil && w.decayedAffinity(a, op) > 0 {
		w.fightLiked++
	}
}

// viewOf is the one in sight with this ID, or nil. A straight scan: what is
// in sight is a handful, and the alternative - an index built every decision -
// would cost more than it saved.
func viewOf(p *Perception, id int) *AgentView {
	for i := range p.Others {
		if p.Others[i].ID == id {
			return &p.Others[i]
		}
	}
	return nil
}

// paySides is what going in on somebody's side is worth to the two of them
// (TODO 14, (iii)). It runs when a decision was an attack on a target another
// body had already declared for - the same moment the joins counter is taken -
// and pays both of them, the way a gift and a shared carcass both do.
//
// Forced, like a birth and unlike a trade. A memory that is full would
// otherwise mean that the bodies who fight beside each other most are exactly
// the ones who cannot learn it, and memFull is 0.99: the optional path would
// leave this rule firing almost never.
//
// It draws no random numbers. It is paid per decision that joins rather than
// per tick of the fight, so a long scrap between the same three bodies pays
// once for each time one of them thought about it again, which is what Calls
// already does for an invitation.
func (w *World) paySides(joiner *Agent) {
	if w.cfg.AffinityAlly <= 0 {
		return
	}
	target := joiner.Action.TargetID
	if target == 0 {
		return
	}
	for i := range w.agents {
		o := &w.agents[i]
		if !o.Alive || o.ID == joiner.ID || o.ID == target {
			continue
		}
		if o.declaredFor() != target {
			continue
		}
		w.rememberAffinity(joiner, o.ID, w.cfg.AffinityAlly)
		w.rememberAffinity(o, joiner.ID, w.cfg.AffinityAlly)
		w.sidesTaken++
	}
}
