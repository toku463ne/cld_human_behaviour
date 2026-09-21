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
		if w.cfg.AffinitySnatched > 0 && w.cfg.AffinityNegative && w.mindsSnatch(o, eater.ID) {
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

// noteClaimsSeen counts what the unwritten rule of #140 would have to work
// with: of the food a body can see when it decides, how much of it somebody
// else has already chosen, and how much of that belongs to somebody it thinks
// well of.
//
// The snatch count answers a different question. That one is taken after the
// fact - an item was eaten from under somebody - and this one before it, which
// is where a rule that made a body give way would have to fire. Standing back
// is offered more often than being robbed is suffered, and by how much is what
// says whether the rule can matter.
//
// Read only, and taken whether or not anything is done about it.
func (w *World) noteClaimsSeen(a *Agent, p *Perception) {
	for i := range p.Foods {
		f := &p.Foods[i]
		w.foodsSeen++
		if f.ClaimedBy == 0 {
			continue
		}
		w.foodsClaimed++
		if op := a.opinion(f.ClaimedBy); op != nil && w.decayedAffinity(a, op) > 0 {
			w.foodsClaimedLiked++
		}
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
// (TODO 14, (iii)), in the form the rule was first written: paid when a
// decision was an attack on a target somebody else had already declared for.
//
// A declaration is an intention. Two things follow from pricing one. The pair
// is paid again every time either of them thinks about the fight again, which
// is why this fired 13954 times a run while joins were 3% of decisions; and a
// body that called a fight and never swung is paid for the call. Both are why
// the blow-priced form below exists (#140), and why it is the default.
func (w *World) paySides(joiner *Agent) {
	if w.cfg.AffinityAlly <= 0 || w.cfg.AllyPaidOnBlows {
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
		w.paySide(joiner, o)
	}
}

// noteSideTaken is the same debt priced off blows instead of intentions
// (#140), called as a swing is resolved and before the ledger of who has hit
// whom is brought up to date.
//
// Two ways to have taken somebody's side, and they are not the same relation:
//
//   - Both of you have swung at the same body. This is the carcass claim's
//     own ledger (recentAttackers, HuntCreditTicks), read for the living.
//   - You swung at the one that is swinging at them. Striking somebody's
//     attacker is taking their side, and it is the half a rule about
//     "joining in" would otherwise miss entirely - the one that looks like
//     defending a friend rather than piling onto a quarry.
//
// Paid once per pair per engagement, not once per blow. What says a swing is
// the start of one is that this body has not swung at that one inside the
// window: the same window the carcass uses, and the same ledger, so nothing
// new is remembered anywhere.
func (w *World) noteSideTaken(from, to *Agent) {
	cfg := &w.cfg
	if cfg.AffinityAlly <= 0 || !cfg.AllyPaidOnBlows {
		return
	}
	// Already in this fight: the side was taken when it started.
	if at, ok := to.hitBy[from.ID]; ok && w.tick-at <= cfg.HuntCreditTicks {
		return
	}
	// Whoever's side is being taken. Every body already swinging, or - where
	// the debt is settled once per fight (#141) - only the first of them,
	// since what is being paid for is having been in it rather than how many
	// were.
	for _, id := range to.recentAttackers(w.tick, cfg.HuntCreditTicks) {
		if id == from.ID {
			continue
		}
		if o := w.agentByID(id); o != nil && o.Alive {
			w.paySide(from, o)
			if cfg.AllyPaidOncePerFight {
				return
			}
		}
	}
	// And whoever this one was already swinging at: hitting their attacker is
	// taking their side. The target has to have actually been struck by it -
	// an intention to attack somebody is not an attack on them.
	if to.Action.Kind != ActAttack || to.Action.TargetID == 0 {
		return
	}
	v := w.agentByID(to.Action.TargetID)
	if v == nil || !v.Alive || v.ID == from.ID {
		return
	}
	if at, ok := v.hitBy[to.ID]; ok && w.tick-at <= cfg.HuntCreditTicks {
		w.paySide(from, v)
	}
}

// paySide is the payment itself, both ways and forced.
//
// Forced, like a birth and unlike a trade: a memory that is full would
// otherwise mean the bodies that fight beside each other most are the ones
// that cannot learn it, and memFull is 0.99.
func (w *World) paySide(a, o *Agent) {
	w.rememberAffinity(a, o.ID, w.cfg.AffinityAlly)
	w.rememberAffinity(o, a.ID, w.cfg.AffinityAlly)
	w.sidesTaken++
}

// mindsSnatch is whether this body takes offence at all (#141). One draw,
// weighted by what it already thinks of the one that got there first: at
// SnatchForgiveness 1 a body it trusts completely is let off every time and a
// stranger never, and at 0 nobody is let off, which is the rule as it was
// first written.
//
// The draw is skipped where there is nothing to forgive, so a world without
// the figure - and a population of strangers with it - consumes the random
// source exactly as it did.
//
// It is a coin tied to a quantity rather than a coin on its own (#124), and
// the quantity is the one every other reading of trust uses.
func (w *World) mindsSnatch(loser *Agent, takerID int) bool {
	cfg := &w.cfg
	if cfg.SnatchForgiveness <= 0 || cfg.AffinityTrust <= 0 {
		return true
	}
	held := 0.0
	if op := loser.opinion(takerID); op != nil {
		held = w.decayedAffinity(loser, op)
	}
	chance := 1 - cfg.SnatchForgiveness*clamp(held/cfg.AffinityTrust, 0, 1)
	if chance >= 1 {
		return true
	}
	w.snatchLooks++
	if w.rng.Float64() < chance {
		return true
	}
	w.snatchForgiven++
	return false
}

// mindsMore is which of two claimants this body would rather not take the item
// from (#142): the one it holds more goodwill for, since that is what the
// taking would cost it. A tie goes to the one already held, so the answer does
// not depend on the order the bodies happen to be scanned in.
//
// It draws nothing, and it is only reached where two bodies in sight have
// chosen the same item, which the count says is rare.
func (w *World) mindsMore(observer *Agent, candidate, held int) bool {
	return w.affinityTo(observer, candidate) > w.affinityTo(observer, held)
}

// affinityTo is what this body currently thinks of another, decayed, or nought
// where it has never registered.
func (w *World) affinityTo(a *Agent, otherID int) float64 {
	if op := a.opinion(otherID); op != nil {
		return w.decayedAffinity(a, op)
	}
	return 0
}

// noteClaimMade wakes the bodies already walking towards the item this one has
// just chosen (#142), so that the weighing happens while the race is on rather
// than only at whatever the body was going to think about next.
//
// Until this, a body that had set out for a meal found out that somebody it is
// fond of wanted it too in one of two ways: at its next idle rethink, forty
// ticks later, or when the meal was gone and the rule that fires is the one
// that lowers an opinion. Both are after the fact. The whole of #140 is a
// weighing that has to happen before the race is decided.
//
// No message is sent and no range is invented: the wake only reaches bodies
// that can see the one that chose, which is the same reach the claim itself
// has. A claim from out of sight stays out of sight.
//
// The scan is over every agent rather than over the index, for the reason
// noteSnatched's is: this runs inside the pass that is about to move bodies.
func (w *World) noteClaimMade(a *Agent) {
	if !w.cfg.ClaimRetriggers || w.cfg.AffinitySnatched <= 0 {
		return
	}
	target := a.Action.TargetID
	if a.Action.Kind != ActEat || target == 0 {
		return
	}
	for i := range w.agents {
		o := &w.agents[i]
		if !o.Alive || o.ID == a.ID {
			continue
		}
		if o.Action.Kind != ActEat || o.Action.TargetID != target {
			continue
		}
		// Only somebody with something to lose, and only where it can see who
		// it would be losing it to.
		if w.affinityTo(o, a.ID) <= 0 || !w.canSee(o.X, o.Y, a.X, a.Y) {
			continue
		}
		o.requestDecision(TriggerClaimHeard)
		w.claimWakes++
	}
}
