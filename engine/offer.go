package engine

// Crying your wares (stage 49).
//
// Stage 48 opened the gate: things are handed over, and almost all of it goes
// to strangers. What it could not do was tell anybody that there was anything
// to be had. Food in a hand is invisible to everybody but its owner (stage
// 40), so a body with a spare plant and a body that would take it find each
// other only by walking into each other.
//
// This is the word for saying so. A body stands still for a while and holds
// out what it has, and for as long as it does, what is in its hand is a fact
// anybody who can see it can read.
//
// The target was counted before this was written, the way stage 40's and 45's
// were. In the default world 27% of living bodies are holding something and
// 85% of those are not hungry, so the surplus is real. But 52% of holders can
// already see a hungry body with a free hand, at a mean distance of 64 - so
// what a cry can win on the seller's side is a walk, not a customer. On the
// buyer's side the whole target is 2.9%: that is the share of hungry moments
// spent with no food in sight and somebody holding some within sight. Nothing
// here can be worth more than those two numbers.
//
// The shape of it, and what it deliberately is not.
//
//   - It is a visible fact, not a memory (#74). What is on offer is read off
//     the current action and stored nowhere, exactly as stage 32's DeclaredFor
//     is. A cry that outlived the crying would be state that goes stale - a
//     body that called and wandered off would still be advertising - and it
//     would have to be saved, cleared and reasoned about.
//   - It is not a third case for the witness scan (#74). forEachWitness is for
//     things that happen in an instant, a kill or a drowning; a body crying is
//     in sight for as long as it cries, and perceive already hands that to
//     everybody who can see it.
//   - The price is time, in the shape stage 32 used: the cry takes OfferTicks,
//     the body does nothing else meanwhile, and then it is asked again.
//   - The range is sight and nothing more. A rumour that crossed a region is
//     worth deciding about after this has been measured in the small.
//   - Nothing keeps while it is being sold (#77). The clock on a held item is
//     the same clock whatever its owner is doing with it, and a benefit with
//     no price is what stage 17b watched run to the ceiling - a body with no
//     intention of selling anything would stand there to stop the rot.

// offering is what this agent is holding out, and it is read off the action
// the same way declaredFor reads what it has taken on. Nil when there is no
// crying in this world, when it is not crying, or when it is crying with an
// empty hand.
func (w *World) offering(a *Agent) *Food {
	if !w.cfg.WaresSeen || w.cfg.OfferTicks <= 0 || a.Action.Kind != ActOffer || len(a.carried) == 0 {
		return nil
	}
	return &a.carried[0]
}

// offerLeft is how much longer the cry has to run. A body that can hear a cry
// knows how long one lasts, in the same way it knows how fast an ordinary body
// walks: it is a rule of the world, not something read off the crier.
func (w *World) offerLeft(o *Agent) int {
	if left := w.cfg.OfferTicks - o.actionTicks; left > 0 {
		return left
	}
	return 0
}

// cry runs a cry for its ticks. There is nothing to do but stand there: the
// whole of the rule is that while this action is the current one, what is in
// the hand is visible to everybody who can see the body.
func (w *World) cry(a *Agent) {
	if len(a.carried) == 0 {
		a.requestDecision(TriggerTargetLost) // sold, eaten, or gone off
		return
	}
	if a.actionTicks >= w.cfg.OfferTicks {
		a.requestDecision(TriggerGoalReached)
	}
}

// OfferUse is what the crying came to. Read only.
type OfferUse struct {
	// Cries is how many times the word was spoken, Heard the share of
	// decisions taken with somebody's wares in sight, and Draws the share
	// that were a walk towards one.
	Cries int
	Heard float64
	Draws float64
	// Sold is the share of gifts made by a body that had been crying within
	// the last two cries' worth of ticks - the closest this world can come to
	// asking whether the advertisement led to the hand-over.
	Sold float64
}

// Offers reports what the crying came to.
func (w *World) Offers() OfferUse {
	out := OfferUse{Cries: w.cries}
	if w.decisions > 0 {
		out.Heard = float64(w.offersHeard) / float64(w.decisions)
		out.Draws = float64(w.offerDraws) / float64(w.decisions)
	}
	if w.gifts > 0 {
		out.Sold = float64(w.giftsCried) / float64(w.gifts)
	}
	return out
}
