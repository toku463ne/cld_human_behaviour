package engine

// Handing something over (stage 48).
//
// This is the gate the whole of P8 waits on. Coins, warehouses and criers are
// plumbing, and plumbing carries nothing unless somebody would give a thing to
// somebody else in the first place (#73). So the cheapest possible version of
// that is built here and measured: one word, no deal, no price, no ledger.
//
// What a gift buys is being on good terms, because that is the only currency
// this world already has. Affinity is what a shared kill earns (stage 11's
// AffinityHunt, the one kind that can be had from a stranger), and it is what
// makes resting near somebody cheap (stage 9) and counting on them in a fight
// possible (stage 32). A gift earns it the same way and in both directions -
// the giver thinks better of somebody it has helped, exactly as the sharers of
// a carcass think better of each other.
//
// Three things it deliberately is not.
//
//   - Not a bargain. Nothing is exchanged for anything: one body hands over an
//     item and that is the whole rule. A deal - two things moving at once,
//     with terms - is a mechanism with a price to agree on, and there is no
//     point building it until somebody would give anything away at all.
//   - Not a rule about accepting. Receiving costs nothing, so there is nothing
//     to decide: the item goes into a free hand, and what the receiver does
//     with it afterwards is scored the way everything else it does is scored.
//     No threshold anywhere (the principle stage 25 set).
//   - Not new trust and not reputation. It writes into the same affinity every
//     other rule writes into, and no rule reads a "has given" tally.

// giveItem hands the first thing in one body's hands to another. It reports
// whether anything moved: an empty hand, or a receiver with nothing free, is
// simply nothing happening.
func (w *World) giveItem(from, to *Agent) bool {
	if len(from.carried) == 0 || !to.canCarryMore(&w.cfg) {
		return false
	}
	item := from.carried[0]
	// And it has to be something they can do anything with (found in stage
	// 51). Receiving costs nothing was the rule here, and it was wrong for
	// the one case nobody had thought of: a hand is the world's scarcest
	// thing, and a plant given to something that lives on meat blocks it for
	// good. It also got round two rules the world keeps at the mouth - what
	// a species can digest, and that nobody eats its own kind.
	if !w.canCarry(to, &item) {
		return false
	}
	w.removeCarried(from, 0)
	to.carried = append(to.carried, item)
	w.heldKind[item.Kind]++
	w.gifts++
	// And whether it followed a cry (stage 49), which is as close as this
	// world gets to asking whether the advertisement is what brought them
	// together. Two cries' worth of ticks is the window: long enough for
	// somebody to have crossed the sight that heard it, short enough that an
	// ordinary gift does not fall inside it by chance.
	if w.cfg.OfferTicks > 0 && from.criedAt > 0 && w.tick-from.criedAt <= 2*w.cfg.OfferTicks {
		w.giftsCried++
	}
	if item.Kind == FoodStone {
		w.giftStones++
	}
	if item.Cooked > 0 {
		w.cookedHanded++
		if !w.cfg.CookSurvivesHands {
			to.carried[len(to.carried)-1].Cooked = 0
		}
	}

	// What it earns, both ways, and through the ordinary machinery: the same
	// call the shared kill makes.
	if w.cfg.AffinityGift > 0 {
		w.rememberAffinity(to, from.ID, w.cfg.AffinityGift)
		w.rememberAffinity(from, to.ID, w.cfg.AffinityGift)
	}
	// And who it went to, for the measurement this stage exists for: the
	// question is whether anything is ever handed to somebody who is not
	// family or a mate.
	switch {
	case from.PartnerID == to.ID || to.PartnerID == from.ID:
		w.giftsToMates++
	case from.isKin(to.ID):
		w.giftsToKin++
	default:
		w.giftsToStrangers++
	}
	return true
}

// GiftUse is what has been handed over. Read only.
type GiftUse struct {
	// Gifts is how many times anything changed hands, and the three shares
	// say to whom. Strangers is the one the stage turns on: gifts within a
	// family are a family, and an economy needs the other kind.
	Gifts     int
	ToKin     float64
	ToMates   float64
	ToStrange float64

	// Stones is the share of gifts that were something to throw rather than
	// something to eat.
	Stones float64
}

// Gifts reports what has been handed over.
func (w *World) Gifts() GiftUse {
	out := GiftUse{Gifts: w.gifts}
	if w.gifts == 0 {
		return out
	}
	n := float64(w.gifts)
	out.ToKin = float64(w.giftsToKin) / n
	out.ToMates = float64(w.giftsToMates) / n
	out.ToStrange = float64(w.giftsToStrangers) / n
	out.Stones = float64(w.giftStones) / n
	return out
}
