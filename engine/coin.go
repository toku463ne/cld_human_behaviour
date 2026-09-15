package engine

import "math"

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
// It is keepValue and nothing else, which is the point (#73) - of the better
// of the two meals money buys, once stage 81 is on. The discount is
// its own figure rather than CarryValue's or StoreValue's because the bet is
// different again: what is in a hand cannot be taken, what is in a cache can
// be taken by whoever knows the place, and a coin has to find somebody willing
// to sell.
// held is what else this body is already holding, in meals (stage 71): a coin
// is a claim on a meal at the moment of running short, so a body that is
// already holding that meal has little use for the claim - and that is what
// keeps money from pushing dinner out of a hand.
func coinWorth(cfg *Config, s *SelfView, held float64) float64 {
	if cfg.CoinValue <= 0 {
		return 0
	}
	claim := keepValue(cfg, s, 0, 1, 0, held, 0) // money does not go off
	if cfg.CoinBuysMending && cfg.MeatVitality > 0 {
		// And the other thing money buys here (stage 81). A coin is a claim on
		// a meal, and not every meal is the same meal: a carcass mends as well
		// as feeds, and carcasses are sold - of the wares anybody could
		// actually be seen offering, counted over six runs, 72% were something
		// that mends.
		//
		// No weights are needed, which is the surprise of the stage. The plan
		// asked for the claim to be shared out by what the body needs now, and
		// mealValueAt already caps mending at what the body is actually
		// missing - so a whole body gets nothing from this branch and a
		// half-dead one gets all of it. The ceiling is the weight.
		if mends := keepValue(cfg, s, 0, 1, cfg.MeatVitality*s.MaxVitality,
			held, 0); mends > claim {
			claim = mends
		}
	}
	if cfg.CoinBuysWarding && cfg.WardStrength > 0 {
		// And the third thing money buys here (#116): something that keeps
		// the weather off. The ceiling is the weight again - a body in no
		// weather, or already wearing the best it could buy, gets nothing
		// from this branch - so nothing has to say how much of the claim is
		// which.
		ward := math.Min(clamp(cfg.WardStrength, 0, 1)*s.ChillRaw, s.Chill)
		if keeps := wardClaim(cfg, s, ward); keeps > claim {
			claim = keeps
		}
	}
	return cfg.CoinValue * claim
}

// wardClaim is what something that keeps the weather off would be worth to
// this body at the moment it is in trouble (#116).
//
// Priced there rather than now for keepValue's reason, and it is the same
// trick: a whole body's chance of dying is flat, so anything read off the
// gradient as it stands is worth nothing to exactly the bodies that have
// something to sell. What money claims is never the thing now, it is the thing
// at the moment of needing it.
func wardClaim(cfg *Config, s *SelfView, ward float64) float64 {
	if ward <= 0 {
		return 0
	}
	short := *s
	short.Hunger = math.Max(s.Hunger, cfg.StarveHunger)
	return warmthValue(cfg, &short, 0, ward)
}

// saleGoodwill is what being on better terms with somebody is worth to this
// body, priced the way stage 48 prices a gift: the trust the hand-over would
// buy, times what standing with somebody you are fond of is worth. One place,
// because the seller and the buyer are looking at the same thing from the two
// ends (stage 68).
//
// Trust saturates, so this is worth most between strangers and nothing at all
// between two who are already close - and its ceiling is LoreValue however
// large AffinitySale is set. That ceiling is a fact about the mechanism and
// was counted before it was written: a goodwill worth LoreValue closes about
// a third of the refusals, and the mean refusal is wider than that.
func saleGoodwill(cfg *Config, affinity float64) float64 {
	if cfg.AffinitySale <= 0 || cfg.AffinityTrust <= 0 || cfg.LoreValue <= 0 {
		return 0
	}
	return cfg.LoreValue * trustBought(cfg, affinity, cfg.AffinitySale)
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
func (w *World) willSell(seller *Agent, buyer *Agent, item *Food) bool {
	coin, food := w.saleTerms(seller, buyer, item)
	return coin > food
}

// saleTerms is the two sides of that comparison kept apart: what the seller
// would be getting and what it would be giving up.
//
// They are returned rather than compared on the spot so that stage 75 can say
// which of the two had moved when a refusal later turned into a sale. Both are
// the seller's - the buyer has no threshold of its own - so that is the whole
// of what changing its mind can mean.
func (w *World) saleTerms(seller *Agent, buyer *Agent, item *Food) (float64, float64) {
	cfg := &w.cfg
	s := w.selfView(seller)
	if cfg.CoinValue <= 0 {
		return 0, 1 // money nobody values buys nothing
	}
	// What the coin would be worth to the seller, once this item has left its
	// hand: the thing being sold is not in the way of its own price.
	coin := coinWorth(cfg, &s, otherMeals(&s, w.mealsOf(seller, item)))
	// And what selling to this one earns, if a sale earns anything (stage
	// 68). It goes on the seller's side because that is the side that was
	// short: the discount on the coin is a loss the seller takes every time,
	// and being on better terms is the only thing this world has to make it
	// up with.
	if cfg.AffinitySale > 0 && buyer != nil {
		affinity := 0.0
		if op := w.opinionOf(seller, buyer.ID); op != nil {
			affinity = w.decayedAffinity(seller, op)
		}
		coin += saleGoodwill(cfg, affinity)
	}
	// And what parting with the thing would cost: the same figure putting it
	// down weighs and the same one a gift gives up (handWorth, stage 77).
	// A meal is worth the better of eating it now and keeping it; a book is
	// worth what it would still tell its owner, which once read is nothing -
	// the asymmetry this world has never had anywhere else.
	//
	// This used to be worked out again here, which was the same arithmetic in
	// two places and the reason a stone had no price on this side at all.
	view := w.handView(seller, item)
	food := handWorth(cfg, &s, &view)
	// ... less what carrying it costs between now and then, which is what a
	// coin does not cost. It is the same lug the option to pick something up
	// is charged (controller.go), over the same wait.
	wait := 0.0
	if s.HungerRate > 0 {
		wait = clamp((cfg.StarveHunger-s.Hunger)/s.HungerRate, 0, cfg.PlanHorizon)
	}
	lug := (burdenWith(cfg, &s, 0) - burdenWith(cfg, &s, -1)) * moveCostAt(cfg, 0.4) * groundOf(&s) * wait
	return coin, food - lug
}

// saleRange is the two limits a price has to sit between (stage 80): the
// fewest coins that would make the seller better off, and the most the buyer
// would be better off paying.
//
// Both come out of figures that already exist. The seller's side is willSell
// itself, rearranged: it says yes when coin x price + goodwill beats what it
// is giving up, so the floor is that comparison divided by what one coin is
// worth to it. The buyer's side is addBuy, rearranged the same way: it is
// better off while what it gets beats what the coins would have bought.
//
// Neither is a new estimate and neither side learns anything about the other.
// The world works both out at the counter, which is where willSell has always
// been asked - a price is settled when the buyer arrives, not advertised.
func (w *World) saleRange(seller, buyer *Agent, item *Food) (float64, float64, bool) {
	cfg := &w.cfg
	ss := w.selfView(seller)
	coinS := coinWorth(cfg, &ss, otherMeals(&ss, w.mealsOf(seller, item)))
	coinTotal, foodNet := w.saleTerms(seller, buyer, item)
	net := foodNet - (coinTotal - coinS) // the goodwill is not per coin
	floor := 0.0
	switch {
	case coinS > 0:
		floor = net / coinS
	case net < 0:
		// A seller that puts nothing on money at all, which is most of them:
		// what it gets out of the sale is the weight it stops carrying, and
		// that is the same whether it is handed one coin or five. These are
		// the sales this world has always had (stage 76), so the price has to
		// leave them alone: anything clears.
		floor = 0
	default:
		return 0, 0, false
	}

	// And the buyer's, priced exactly as addBuy prices it - what is on the
	// counter is worth what it is to whoever is looking (stage 49), and what
	// it gives up is what the coins would have bought.
	sb := w.selfView(buyer)
	coinB := coinWorth(cfg, &sb, otherMeals(&sb, cfg.CoinValue))
	meal := w.wareValue(buyer, &sb, item)
	goodwillB := 0.0
	if op := w.opinionOf(buyer, seller.ID); op != nil {
		goodwillB = saleGoodwill(cfg, w.decayedAffinity(buyer, op))
	}
	if coinB <= 0 {
		// And a buyer that puts nothing on money is not made worse off by any
		// price. What caps it then is the purse, not the reckoning.
		if meal+goodwillB <= 0 {
			return 0, 0, false
		}
		return floor, math.Inf(1), true
	}
	return floor, (meal + goodwillB) / coinB, true
}

// wareValue is what something held out is worth to the one looking at it: the
// same two figures the buyer's own option is scored with, in one place so that
// the price and the decision to walk over cannot drift apart.
func (w *World) wareValue(a *Agent, s *SelfView, item *Food) float64 {
	cfg := &w.cfg
	if item.Kind == FoodBook {
		return cfg.BookValue * w.bookValue(a, item)
	}
	if item.Kind == FoodTrinket {
		return w.trinketWorth(a, item) // what it is worth to this one (stage 84)
	}
	nutrition, heal := s.Nutrition[item.Kind], w.itemHealKnown(a, item)
	meal := mealValue(cfg, s, 0, nutrition, heal)
	if kept := keepValue(cfg, s, 0, nutrition, heal,
		otherMeals(s, cfg.CoinValue), w.spoilsIn(item)); kept > meal {
		meal = kept
	}
	return meal
}

// anchoredFloor is the seller's floor, raised towards what it paid for the
// thing (stage 83).
//
// A price of what it paid clears a floor of one less, so that is what it holds
// out for, and it holds out that far only in proportion to its own expectation
// of being here at the end of the window - the same figure an ornament is
// discounted by (stage 84). Nothing new is estimated: the price paid is on the
// item and the chance is the one every option in this world is weighed with.
//
// It is counted whenever it actually moves the floor, because how often that
// happens is the whole question (#111): an anchor that never meets a resale
// never fires.
func (w *World) anchoredFloor(seller *Agent, item *Food, floor float64) float64 {
	if w.cfg.SaleAnchor <= 0 || item.PricePaid <= 0 {
		return floor
	}
	reserve := float64(item.PricePaid) - 1
	if reserve <= floor {
		return floor // it is being offered more than it paid anyway
	}
	s := w.selfView(seller)
	risk := pressures(&w.cfg, &s, seller.Vitality, seller.Hunger, 0).far
	hold := clamp(w.cfg.SaleAnchor, 0, 1) * clamp(1-risk, 0, 1)
	if hold <= 0 {
		return floor
	}
	w.trade.anchored++
	return floor + (reserve-floor)*hold
}

// salePrice is what a sale costs, in coins (stage 80).
//
// With prices off it is the world every figure before this was measured in:
// one coin, and willSell's yes or no. With them on it is the cheapest number
// of coins that clears the seller's floor - or the middle of the range, which
// is the same rule with the surplus shared the other way (SalePriceSplit).
//
// The rounding is the sharing: coins do not divide, so whichever end the price
// is taken from is the end that keeps what is left over. Counted before this
// was written, it is a choice about very little - the median number of whole
// prices that fit between the two limits is one.
func (w *World) salePrice(seller, buyer *Agent, item *Food) (int, bool) {
	cfg := &w.cfg
	if !cfg.CoinPrices {
		return 1, w.willSell(seller, buyer, item)
	}
	floor, ceiling, ok := w.saleRange(seller, buyer, item)
	if !ok {
		return 0, false
	}
	floor = w.anchoredFloor(seller, item, floor)
	price := math.Floor(floor) + 1 // strictly above: the seller has to gain
	if price < 1 {
		price = 1
	}
	if cfg.SalePriceSplit {
		// The middle of what is actually payable: a purse is a limit on the
		// range as real as the buyer's own reckoning.
		top := math.Min(ceiling, float64(buyer.coinsHeld()))
		if mid := math.Round((math.Max(floor, price-1) + top) / 2); mid > price {
			price = mid
		}
	}
	if price >= ceiling {
		return 0, false // nothing a buyer would pay leaves the seller better off
	}
	return int(price), true
}

// sell moves one item each way. Nothing else is recorded: no price, no ledger,
// no memory of the trade as a trade.
//
// Affinity was deliberately not written here until stage 68. A gift earns
// being on good terms (stage 48) because it is one-sided; a sale is not a
// favour, and paying for something that also bought goodwill would have been
// two rules in one and would have made stage 51's measurement unreadable. That
// measurement is in now, and it is what opened this: the seller is short by
// (1 - CoinValue) x keep on every trade, and being on good terms is the only
// currency there is to make it up with. It is written both ways and through
// the same call a hand-over makes, so nothing new prices it.
func (w *World) sell(buyer, seller *Agent) bool {
	coin := buyer.carriedIndex2(FoodCoin)
	item := w.forSaleIndex(seller)
	if coin < 0 || item < 0 || !w.canCarry(buyer, &seller.carried[item]) {
		return false // nobody buys what it could do nothing with
	}
	price, agreed := w.salePrice(seller, buyer, &seller.carried[item])
	if agreed && !buyer.canReceiveFor(&w.cfg, seller.carried[item].Kind, price) {
		agreed = false // nowhere to put it once the money has gone
	}
	// And what neither side's arithmetic can conjure: the coins to pay with,
	// and somewhere to put them. A price is only a price a body can meet
	// (stage 80) - which is why this stage waited for a hand that a coin does
	// not fill (stage 80a).
	if agreed && (buyer.coinsHeld() < price || !seller.canTakeCoins(&w.cfg, price)) {
		agreed = false
	}
	coinWorth, itemWorth := w.saleTerms(seller, buyer, &seller.carried[item])
	w.noteSale(buyer, seller, coinWorth, itemWorth, agreed)
	w.noteResale(&seller.carried[item], price, agreed)
	if !agreed {
		w.salesRefused++
		return false
	}
	f := seller.carried[item]
	w.removeCarried(seller, item)
	for i := 0; i < price; i++ {
		c := buyer.carried[buyer.carriedIndex2(FoodCoin)]
		w.removeCarried(buyer, buyer.carriedIndex2(FoodCoin))
		seller.carried = append(seller.carried, c)
		w.heldKind[c.Kind]++
	}
	f.PricePaid = price // what this holder gave for it (stage 83)
	buyer.carried = append(buyer.carried, f)
	w.heldKind[f.Kind]++
	w.sales++
	w.salePaid += price
	if f.Kind == FoodTrinket {
		w.trinketsSold++
		w.noteTrinketMove(seller, buyer, &f, true)
	}
	if price > 1 {
		w.salesOverOne++
	}
	// What it earns, both ways, through the same call a hand-over makes
	// (stage 68). Stage 51 left this out on purpose - a sale is not a favour
	// - and what its measurement then showed is that without it the seller is
	// short on every trade by construction. A sale is a hand-over too.
	if w.cfg.AffinitySale > 0 && w.cfg.SaleGoodwillKept {
		w.rememberAffinity(buyer, seller.ID, w.cfg.AffinitySale)
		w.rememberAffinity(seller, buyer.ID, w.cfg.AffinitySale)
	}
	if f.Cooked > 0 {
		w.cookedHanded++
		w.noteCookedHandOver(buyer, &f, true)
		if !w.cfg.CookSurvivesHands {
			buyer.carried[len(buyer.carried)-1].Cooked = 0
		}
	}
	return true
}

// standardPrice is what a meal goes for, in coins: the price both sides'
// arithmetic points at, because a coin is a claim on CoinValue of a meal and
// nothing else (#73). A buyer has no way of knowing what a particular seller
// would take - that is the seller's own state, which is hidden - so it reckons
// on the standard, exactly as a body reckons a rival's speed at the world's
// standard speed (stage 49).
func standardPrice(cfg *Config) int {
	if !cfg.CoinPrices || cfg.CoinPriceBlind || cfg.CoinValue <= 0 {
		return 1
	}
	return max(1, int(math.Ceil(1/cfg.CoinValue)))
}

// canReceiveFor says whether a buyer will have a hand for what it is buying
// once the coins have left.
//
// Written for stage 80 and true of one coin as well: where a coin takes no
// hand (stage 80a), paying frees nothing, so a body holding its dinner and a
// coin has nowhere to put a second dinner. Before 80a this could not arise -
// the coin was in the only hand there was - which is why nothing asked.
func (a *Agent) canReceiveFor(cfg *Config, kind FoodKind, coins int) bool {
	if cfg.CarryCapacity <= 0 {
		return false
	}
	if !cfg.CarrySlotted || (cfg.CarrySlotsWeigh && weightless(kind)) {
		return true
	}
	taken := a.slotsTaken(cfg)
	if !cfg.CarrySlotsWeigh {
		taken -= coins // where a coin takes a hand, paying frees one
	}
	return taken < a.carrySlots(cfg)
}

// coinsHeld is how much money is in these hands (stage 80).
func (a *Agent) coinsHeld() int {
	n := 0
	for i := range a.carried {
		if a.carried[i].Kind == FoodCoin {
			n++
		}
	}
	return n
}

// canTakeCoins says whether a seller could accept that many coins. One goes
// into the hand the item just left; the rest need room of their own, which
// where a coin takes no hand (stage 80a) they always have - and where it does,
// never. That is what makes a price of more than one inert in every world
// before 80a, without a rule saying so.
func (a *Agent) canTakeCoins(cfg *Config, n int) bool {
	for i := 1; i < n; i++ {
		if !a.canCarryKind(cfg, FoodCoin) {
			return false
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

// firstForSale is what this body would put on the counter: a meal, or a book
// (stage 69). A stone is not for sale and neither is a coin.
//
// The book comes second on purpose. A body that is holding both should sell
// the meal first, because the whole of stage 69's claim is that a read book is
// the thing it can most afford to part with - and a rule that sold the book
// while the dinner sat in the other hand would be making that claim true by
// construction rather than letting the comparison find it.
func (a *Agent) firstForSale(cfg *Config) int {
	if i := a.firstEdible(); i >= 0 {
		return i
	}
	if cfg.Books {
		if i := a.heldBook(); i >= 0 {
			return i
		}
	}
	if !cfg.Trinkets {
		return -1
	}
	// And last of all the ornament (stage 82), for the same reason the book
	// comes after the dinner: a body should offer what it can most afford to
	// lose, and a read book is worth nothing to its owner while a trinket is
	// worth what it is worth to anybody.
	return a.carriedIndex2(FoodTrinket)
}

// CoinUse is what the money came to. Read only.
type CoinUse struct {
	// Lying is how many are on the ground and Held how many are in hands.
	// The two add up to the world's money, which is a check worth having:
	// no rule makes a coin or destroys one.
	Lying int
	Held  int

	// Holders is the share of living bodies holding one, and PerHolder how
	// many each of those has. PerHolder was one to the digit in every world
	// before stage 80a, because a hand is one hand - which is why counting
	// hands and counting coins gave the same figure and why they are now
	// counted apart.
	Holders   float64
	PerHolder float64

	// Paid is how many coins changed hands over those sales and OverOne how
	// many of them went for more than one coin (stage 80): together they say
	// whether the price is a variable at all or only a name for one.
	Paid, OverOne int

	// Sales is how many times a coin bought a meal, and Refused how many
	// times a buyer walked up to somebody who would not sell. The second is
	// the one this stage turns on: it says whether the market failed for want
	// of buyers or for want of sellers.
	Sales   int
	Refused int
}

// Coins reports what the money came to.
func (w *World) Coins() CoinUse {
	out := CoinUse{Sales: w.sales, Refused: w.salesRefused,
		Paid: w.salePaid, OverOne: w.salesOverOne}
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
		held := 0
		for k := range a.carried {
			if a.carried[k].Kind == FoodCoin {
				held++
			}
		}
		out.Held += held
		if held > 0 {
			out.Holders++
			out.PerHolder += float64(held)
		}
	}
	if out.Holders > 0 {
		out.PerHolder /= out.Holders
	}
	if n > 0 {
		out.Holders /= n
	}
	return out
}
