package engine

// Money (stage 51).
//
// The last piece of the plumbing, and the one the two before it were laid for:
// something that is worth having only because somebody else will take it.
//
// What it is worth was settled before this was written (#73). It is not a
// third thing a body wants - the priorities are life and offspring and nothing
// else - so a coin is worth exactly what it will buy: a meal, at the moment
// this body runs short, discounted for the chance of finding a seller. That is
// keepValue, the same figure a thing kept for later is worth (carry.go,
// store.go), and using it is what keeps money out of the goals.
//
// And that settles the stage, before any of it runs. A coin priced as a claim
// on a meal cannot be worth more than the meal it claims. Counted over 55,000
// looks in two worlds before this file existed: of every moment a body was
// holding food, the share where a coin would have been worth more than the
// food was 0.0000, and the mean value of the two was identical to three
// decimal places. A seller is never better off. That is arithmetic and not
// chance, and it is what #73 buys with its own constraint: money that cannot
// be worth more than its expected food cannot be bought with food.
//
// Two things break that tie, and between them they are the whole of why any
// sale ever happens here.
//
// The first is that the coin is valued by each side in its own terms. A coin
// is worth the meal it will buy TO WHOEVER IS HOLDING IT, and a hungry body
// and a fed one do not put the same figure on a meal - so a purchase is not
// zero-sum, and what makes it work is the oldest reason for trade there is:
// the same thing is worth different amounts to two people. That is why the
// discount matters and why it must be below one. At exactly one a coin is
// worth precisely the meal it claims and nobody on either side gains
// anything; above one money is worth more than what it buys, which is a third
// goal by the back door (#73). The middle is not thin, but it is one-sided:
// everything below one favours the buyer, and the seller is left with
//
// the second thing - a coin weighs nothing (#66). Carrying it is free, so a
// body that swaps its dinner for a coin keeps a claim it values at about the
// same and walks lighter. In a world whose only cost of holding something is
// its weight, that is the whole of what liquidity is.
//
// Three things it deliberately is not.
//
//   - Not a store of value with a life of its own. No half-life, no supply
//     cap, no interest. Deflation would make hoarding worth something in
//     itself, which is a third goal by the back door (#73).
//   - Not a geography. Coins are scattered uniformly and belong to no region,
//     because a map with money in one corner is one more condition pulling
//     bodies somewhere for a reason that is not food, and four measurements
//     say food is the only thing that moves anybody (34, 35, 36, 38a).
//   - Not a new kind of ownership. What is lying about can be picked up, what
//     is held is held, and a sale is one item each way. There is no debt, no
//     price list and no ledger.

// scatterCoins lays the world's money out, once, when the world is built.
//
// Uniformly, and nowhere in particular: see above. A world whose author asked
// for none draws nothing at all, so its runs are the runs it had.
func (w *World) scatterCoins() {
	for i := 0; i < w.cfg.Coins; i++ {
		w.putFood(Food{
			X:    w.randRange(10, w.cfg.Width-10),
			Y:    w.randRange(10, w.cfg.Height-10),
			Kind: FoodCoin,
		})
	}
}

// coinWorth is what a coin is worth to this body: the meal it will buy at the
// moment there is nothing about, discounted for the chance of finding anybody
// to buy it from.
//
// It is keepValue and nothing else, which is the point (#73). The discount is
// its own figure rather than CarryValue's or StoreValue's because the bet is
// different again: what is in a hand cannot be taken, what is in a cache can
// be taken by whoever knows the place, and a coin has to find somebody willing
// to sell.
func coinWorth(cfg *Config, s *SelfView) float64 {
	if cfg.CoinValue <= 0 {
		return 0
	}
	return cfg.CoinValue * keepValue(cfg, s, 0, 1, 0)
}

// willSell is the seller's side of a sale, and it is a rule of the world
// rather than a question put to a controller - the same shape as whether a
// courtship is accepted (willCommit).
//
// What it compares is what this body would be giving up against what it would
// be getting, both in its own terms: the food in its hand is worth the better
// of eating it now and keeping it, and the coin is worth what it will buy.
// Nothing here is a threshold - it is the same comparison every other option
// in this world goes through, asked at the moment the buyer arrives.
//
// The weight is the whole of the margin. A coin is free to carry and a meal is
// not, so a body that sells keeps the same expectation and walks lighter, and
// that difference is what this returns true on.
func (w *World) willSell(seller *Agent, item *Food) bool {
	cfg := &w.cfg
	s := w.selfView(seller)
	if cfg.CoinValue <= 0 {
		return false // money nobody values buys nothing
	}
	coin := coinWorth(cfg, &s)
	// What the food in hand is worth to it: eaten now, or kept.
	nutrition := w.mealValues(seller)[item.Kind]
	heal := w.itemHealKnown(seller, item)
	food := mealValue(cfg, &s, 0, nutrition, heal)
	if kept := keepValue(cfg, &s, 0, nutrition, heal); kept > food {
		food = kept
	}
	// ... less what carrying it costs between now and then, which is what a
	// coin does not cost. It is the same lug the option to pick something up
	// is charged (controller.go), over the same wait.
	wait := 0.0
	if s.HungerRate > 0 {
		wait = clamp((cfg.StarveHunger-s.Hunger)/s.HungerRate, 0, cfg.PlanHorizon)
	}
	lug := (burdenWith(cfg, &s, 0) - burdenWith(cfg, &s, -1)) * moveCostAt(cfg, 0.4) * groundOf(&s) * wait
	return coin > food-lug
}

// sell moves one item each way. Nothing else happens: no goodwill, no memory
// of the trade, no price recorded anywhere.
//
// Affinity is deliberately not written. A gift earns being on good terms
// (stage 48) because it is one-sided; a sale is not a favour, and paying for
// something that also bought goodwill would be two rules in one and would make
// the measurement unreadable.
func (w *World) sell(buyer, seller *Agent) bool {
	coin := buyer.carriedIndex2(FoodCoin)
	item := seller.firstEdible()
	if coin < 0 || item < 0 || !w.canEat(buyer, &seller.carried[item]) {
		return false // nobody buys what it could not eat
	}
	if !w.willSell(seller, &seller.carried[item]) {
		w.salesRefused++
		return false
	}
	c, f := buyer.carried[coin], seller.carried[item]
	w.removeCarried(buyer, coin)
	w.removeCarried(seller, item)
	buyer.carried = append(buyer.carried, f)
	seller.carried = append(seller.carried, c)
	w.heldKind[f.Kind]++
	w.heldKind[c.Kind]++
	w.sales++
	if f.Cooked > 0 {
		w.cookedHanded++
		if !w.cfg.CookSurvivesHands {
			buyer.carried[len(buyer.carried)-1].Cooked = 0
		}
	}
	return true
}

// carriedIndex2 is the first held item of a kind, or -1.
func (a *Agent) carriedIndex2(kind FoodKind) int {
	for i := range a.carried {
		if a.carried[i].Kind == kind {
			return i
		}
	}
	return -1
}

// firstEdible is the first held item that is food, or -1. A stone is not for
// sale and neither is a coin: what money buys is a meal.
func (a *Agent) firstEdible() int {
	for i := range a.carried {
		if a.carried[i].Kind < NumEdibleKinds {
			return i
		}
	}
	return -1
}

// CoinUse is what the money came to. Read only.
type CoinUse struct {
	// Lying is how many are on the ground and Held how many are in hands.
	Lying int
	Held  int

	// Holders is the share of living bodies holding one.
	Holders float64

	// Sales is how many times a coin bought a meal, and Refused how many
	// times a buyer walked up to somebody who would not sell. The second is
	// the one this stage turns on: it says whether the market failed for want
	// of buyers or for want of sellers.
	Sales   int
	Refused int
}

// Coins reports what the money came to.
func (w *World) Coins() CoinUse {
	out := CoinUse{Sales: w.sales, Refused: w.salesRefused}
	for i := range w.foods {
		if w.foods[i].Kind == FoodCoin {
			out.Lying++
		}
	}
	n := 0.0
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		n++
		if a.carriedIndex2(FoodCoin) >= 0 {
			out.Held++
			out.Holders++
		}
	}
	if n > 0 {
		out.Holders /= n
	}
	return out
}
