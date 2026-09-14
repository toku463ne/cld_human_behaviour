package engine

// Where trade already happens, and whether a refusal is final (stage 75).
//
// Nothing here is a rule. It adds no word, no state a body can act on and no
// draw on the generator: it counts what the existing rules are already doing,
// in the same standing as stage 37's measurement of the two banks.
//
// It exists because of what the reset on 2026-09-13 did to this world. Caches
// and money are used far more than they were and neither buys any population
// any more, and the reading is that lookahead put the value of needing
// something later inside the utility formula, so the things that were
// supplying it from outside have nothing left to do (#100). Before building
// anything on top of that, two questions are worth asking of the world as it
// stands (#101).
//
// The first is where the cooking goes. Stage 52 measured that a cooked item
// changes hands 0.55 times per cooking in the default world and 0.97 on the
// played one, which makes it the most handed-around thing there is - and
// cooking is the one shortage in this world that walking cannot fix, because
// it is a property of the body and not of the ground. What that figure does
// not say is whether the receiver could have done it itself. So what is
// counted here is the hand-overs where the item was better than the receiver
// could have made, split by the path it went down: a gift or a sale. If most
// cooked food goes to bodies that cook as well as the giver, there is no
// service here and the market has to be built somewhere else.
//
// The second is whether a refusal is final. A seller's figure moves with its
// hunger and its wounds every tick, so two who failed to agree may agree later
// without anybody bargaining - and if that is common enough there is no reason
// to build a bargaining mechanism at all (a new concept this world would then
// have to carry). What is counted is the pairs that met again after a refusal,
// how many of those agreed, how long it took, and which side of the seller's
// comparison had moved. That last is worth saying plainly: both sides of
// willSell are about the seller. The buyer has no threshold - it walked up
// because addBuy won its comparison - so a refusal turning into a sale is
// always the seller changing its mind, and the only question is whether what
// moved was the coin's worth or the food's.

// refusal is a pair that did not come to terms, and what the seller's two
// figures were at the time.
type refusal struct {
	tick       int
	coin, food float64
}

// tradeWatch is the whole of this stage's bookkeeping. It hangs off the world
// rather than living in it: no rule reads any of it.
type tradeWatch struct {
	// Cooked hand-overs, by the path they went down, and how many of them
	// gave the receiver something better than it could have managed itself.
	cookedGiven, cookedSold int
	cookedBetter            int
	cookedGap               float64

	// Pairs that walked away from each other, and what came of them. seen is
	// meetings after a refusal, won is the ones that ended in a sale, gap is
	// the ticks in between; coinRose and foodFell are how much of the seller's
	// two figures had moved by then.
	open                map[[2]int]refusal
	seen, won, gapTicks int
	coinRose, foodFell  float64
}

// noteCookedHandOver records one cooked item changing hands. It rides on the
// two lines that already count such a hand-over (gift.go, coin.go) so nothing
// is detected twice or anew.
func (w *World) noteCookedHandOver(to *Agent, f *Food, sold bool) {
	if sold {
		w.trade.cookedSold++
	} else {
		w.trade.cookedGiven++
	}
	// What the receiver would have managed on its own. The comparison is the
	// one canCook makes when it asks whether there is anything left to do to
	// an item, which is the right question here too: an item already as good
	// as this body could make it is an item this body did not need anybody
	// for.
	own := w.cookQuality(to)
	w.trade.cookedGap += f.Cooked - own
	if f.Cooked > own {
		w.trade.cookedBetter++
	}
}

// noteSale records what came of one buyer walking up to one seller.
func (w *World) noteSale(buyer, seller *Agent, coin, food float64, agreed bool) {
	t := &w.trade
	key := [2]int{buyer.ID, seller.ID}
	if prev, ok := t.open[key]; ok {
		t.seen++
		t.gapTicks += w.tick - prev.tick
		if agreed {
			t.won++
			t.coinRose += coin - prev.coin
			t.foodFell += prev.food - food
		}
	}
	if agreed {
		delete(t.open, key)
		return
	}
	if t.open == nil {
		t.open = make(map[[2]int]refusal)
	}
	t.open[key] = refusal{tick: w.tick, coin: coin, food: food}
}

// TradeUse is what the trading came to. Read only.
type TradeUse struct {
	// CookedGiven and CookedSold are the cooked hand-overs by path. Better is
	// the share of them where the item beat what the receiver could have made
	// itself, and Gap is how far it beat it by, on average - so Better near
	// zero says cooking is not a service anybody needs anybody else for.
	CookedGiven, CookedSold int
	Better, Gap             float64

	// Refused is how many times a buyer was turned away, Met how many of
	// those pairs met again, Won how many of those meetings ended in a sale,
	// and Gap2 the mean ticks between the refusal and the meeting. Won over
	// Met is the figure this stage turns on: if it is high, a refusal is not
	// final and no bargaining has to be built.
	Refused, Met, Won int
	Gap2              float64

	// CoinRose and FoodFell are how much of the seller's two figures had
	// moved by the time it said yes, on average over the sales that followed
	// a refusal. Both sides of the comparison are the seller's, so this says
	// which of the two changed its mind.
	CoinRose, FoodFell float64
}

// Trade reports what the trading came to. It writes nothing and draws nothing.
func (w *World) Trade() TradeUse {
	t := &w.trade
	out := TradeUse{
		CookedGiven: t.cookedGiven,
		CookedSold:  t.cookedSold,
		Refused:     w.salesRefused,
		Met:         t.seen,
		Won:         t.won,
	}
	if n := t.cookedGiven + t.cookedSold; n > 0 {
		out.Better = float64(t.cookedBetter) / float64(n)
		out.Gap = t.cookedGap / float64(n)
	}
	if t.seen > 0 {
		out.Gap2 = float64(t.gapTicks) / float64(t.seen)
	}
	if t.won > 0 {
		out.CoinRose = t.coinRose / float64(t.won)
		out.FoodFell = t.foodFell / float64(t.won)
	}
	return out
}
