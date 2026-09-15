package engine

import "testing"

// priceConfig is the money world where a coin takes no hand and a sale has a
// price in coins (stages 80a and 80).
//
// The goodwill is on, and it has to be for a price of more than one to be
// possible at all: for an ordinary meal the buyer's ceiling is exactly
// 1/CoinValue, because what it is getting and what its coins claim are the
// same figure. Only a ware worth more to the buyer than a standard meal opens
// a range - being on better terms with the seller (stage 68), something that
// mends (stage 81), or a kind it has not eaten lately (stage 16).
func priceConfig() Config {
	cfg := coinConfig()
	cfg.CarrySlotsWeigh = true
	cfg.CoinPrices = true
	cfg.AffinitySale = 6
	// The price at the seller's floor, which is the world stage 80's own
	// arithmetic was worked out in. Splitting the surplus became the default
	// on 2026-09-15 (stage 84) and has its own tests below.
	cfg.SalePriceSplit = false
	return cfg
}

// giveCoins puts money in a body's hands.
func giveCoins(t *testing.T, w *World, id, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		a := mustAgent(t, w, id)
		w.take(a, w.putFood(Food{X: a.X + 1, Y: a.Y, Kind: FoodCoin}))
	}
	if got := mustAgent(t, w, id).coinsHeld(); got != n {
		t.Fatalf("the body was given %d coins and holds %d", n, got)
	}
}

// The standard price is what the arithmetic says a meal goes for: a coin is a
// claim on CoinValue of a meal, so a meal is 1/CoinValue coins.
func TestTheStandardPriceIsWhatACoinClaims(t *testing.T) {
	cfg := priceConfig()
	if got := standardPrice(&cfg); got != 2 {
		t.Fatalf("with a coin worth half a meal the standard price is %d", got)
	}
	cfg.CoinPrices = false
	if got := standardPrice(&cfg); got != 1 {
		t.Fatalf("with prices off everything costs %d", got)
	}
}

// What the stage is for: a seller that turns down one coin takes two.
func TestTwoCoinsBuyWhatOneWouldNot(t *testing.T) {
	// One coin, and the seller says no.
	w := NewWorld(priceConfig())
	sellerID, buyerID := holding(t, w, 100, 100), holdingCoin(t, w, 104, 100)
	seller, buyer := mustAgent(t, w, sellerID), mustAgent(t, w, buyerID)
	seller.Hunger, seller.Vitality = 40, 50
	buyer.Hunger, buyer.Vitality = 95, 20
	if w.sell(buyer, seller) {
		t.Fatal("this seller should want more than one coin for it")
	}

	// The same pair, and the buyer has two.
	w2 := NewWorld(priceConfig())
	sellerID2, buyerID2 := holding(t, w2, 100, 100), w2.addAgent(Agent{Maturity: 1,
		X: 104, Y: 100, Vitality: 20, Hunger: 95, Genome: genomeOf(50, 50, 50)})
	giveCoins(t, w2, buyerID2, 2)
	seller2, buyer2 := mustAgent(t, w2, sellerID2), mustAgent(t, w2, buyerID2)
	seller2.Hunger, seller2.Vitality = 40, 50
	if !w2.sell(buyer2, seller2) {
		t.Fatal("two coins bought no more than one did")
	}
	if got := seller2.coinsHeld(); got != 2 {
		t.Fatalf("the seller was paid %d coins", got)
	}
	if got := buyer2.coinsHeld(); got != 0 {
		t.Fatalf("the buyer still has %d coins after paying two", got)
	}
	if got := w2.Coins().Lying + w2.Coins().Held; got != 2 {
		t.Fatalf("a priced sale left %d coins in the world, and there were 2", got)
	}
}

// And with the hand still a slot, a price of more than one cannot happen -
// there is nowhere to put the second coin, on either side. No rule says so.
func TestAPriceOfMoreThanOneNeedsAHandThatMoneyDoesNotFill(t *testing.T) {
	cfg := priceConfig()
	cfg.CarrySlotsWeigh = false
	w := NewWorld(cfg)
	buyerID := holdingCoin(t, w, 104, 100)
	giveCoins2 := w.putFood(Food{X: 105, Y: 100, Kind: FoodCoin})
	w.take(mustAgent(t, w, buyerID), giveCoins2)
	if got := mustAgent(t, w, buyerID).coinsHeld(); got != 1 {
		t.Fatalf("a body with one hand is holding %d coins", got)
	}
	seller := mustAgent(t, w, holding(t, w, 100, 100))
	seller.Hunger, seller.Vitality = 55, 90
	if _, ok := w.salePrice(seller, mustAgent(t, w, buyerID), &seller.carried[0]); ok {
		if !w.sell(mustAgent(t, w, buyerID), seller) {
			return // agreed on paper, refused for want of coins: that is the point
		}
		if got := seller.coinsHeld(); got > 1 {
			t.Fatalf("a one-handed seller took %d coins", got)
		}
	}
}

// Neither side is ever made worse off by the price.
func TestAPriceSitsBetweenTheTwoLimits(t *testing.T) {
	for _, split := range []bool{false, true} {
		cfg := priceConfig()
		cfg.SalePriceSplit = split
		w := NewWorld(cfg)
		sellerID := holding(t, w, 100, 100)
		buyerID := w.addAgent(Agent{Maturity: 1, X: 104, Y: 100, Vitality: 20,
			Hunger: 95, Genome: genomeOf(50, 50, 50)})
		giveCoins(t, w, buyerID, 4)
		seller, buyer := mustAgent(t, w, sellerID), mustAgent(t, w, buyerID)
		seller.Hunger, seller.Vitality = 60, 30

		floor, ceiling, ok := w.saleRange(seller, buyer, &seller.carried[0])
		if !ok {
			t.Fatal("no range at all")
		}
		price, agreed := w.salePrice(seller, buyer, &seller.carried[0])
		if !agreed {
			continue
		}
		if float64(price) <= floor {
			t.Fatalf("split=%v: price %d does not clear the seller's floor %v", split, price, floor)
		}
		if float64(price) >= ceiling {
			t.Fatalf("split=%v: price %d is above the buyer's ceiling %v", split, price, ceiling)
		}
	}
}

// The middle of the range is never cheaper than the least the seller would
// take: the rounding is the sharing, and this is which way it shares.
func TestSplittingNeverFavoursTheBuyer(t *testing.T) {
	for _, hunger := range []float64{30, 50, 70, 85} {
		cheap := NewWorld(priceConfig())
		cfgSplit := priceConfig()
		cfgSplit.SalePriceSplit = true
		split := NewWorld(cfgSplit)
		var prices [2]int
		for i, w := range []*World{cheap, split} {
			sellerID := holding(t, w, 100, 100)
			buyerID := w.addAgent(Agent{Maturity: 1, X: 104, Y: 100, Vitality: 20,
				Hunger: 95, Genome: genomeOf(50, 50, 50)})
			giveCoins(t, w, buyerID, 6)
			seller, buyer := mustAgent(t, w, sellerID), mustAgent(t, w, buyerID)
			seller.Hunger, seller.Vitality = hunger, 90
			p, ok := w.salePrice(seller, buyer, &seller.carried[0])
			if !ok {
				p = 0
			}
			prices[i] = p
		}
		if prices[0] > 0 && prices[1] > 0 && prices[1] < prices[0] {
			t.Fatalf("hunger %v: the split price %d is under the cheapest clearing one %d",
				hunger, prices[1], prices[0])
		}
	}
}

// A hand that money does not fill is still a hand: a body holding its dinner
// cannot buy a second one, however much money it has.
func TestMoneyDoesNotBuyAHandYouHaveNot(t *testing.T) {
	w := NewWorld(priceConfig())
	sellerID := holding(t, w, 100, 100)
	buyerID := holding(t, w, 104, 100) // already holding a meal
	giveCoins(t, w, buyerID, 4)
	seller, buyer := mustAgent(t, w, sellerID), mustAgent(t, w, buyerID)
	seller.Hunger, seller.Vitality = 10, 90
	buyer.Hunger, buyer.Vitality = 95, 20

	if w.sell(buyer, seller) {
		t.Fatal("a body with one hand and a meal in it bought a second meal")
	}
	if got := buyer.heavyCarried(); got != 1 {
		t.Fatalf("the buyer is holding %d things that weigh something", got)
	}
}

// With prices off, the world is the one every figure before this was measured
// in: one coin for one thing.
func TestWithPricesOffASaleIsStillOneCoin(t *testing.T) {
	cfg := priceConfig()
	cfg.CoinPrices = false
	w := NewWorld(cfg)
	sellerID := holding(t, w, 100, 100)
	buyerID := w.addAgent(Agent{Maturity: 1, X: 104, Y: 100, Vitality: 20,
		Hunger: 90, Genome: genomeOf(50, 50, 50)})
	giveCoins(t, w, buyerID, 3)
	seller, buyer := mustAgent(t, w, sellerID), mustAgent(t, w, buyerID)
	seller.Hunger, seller.Vitality = 10, 90
	if !w.sell(buyer, seller) {
		t.Fatal("a full seller and a starving buyer did not trade")
	}
	if got := seller.coinsHeld(); got != 1 {
		t.Fatalf("with prices off the seller was paid %d coins", got)
	}
}

// A body that bought a thing holds out for what it paid, and a body in
// trouble does not (stage 83). The rule fires zero times in a running world -
// nothing bought is ever offered for sale again - so this is where it is
// shown to work at all.
func TestASellerHoldsOutForWhatItPaid(t *testing.T) {
	priceFor := func(anchor float64, hunger, vitality float64) (int, bool) {
		cfg := priceConfig()
		cfg.SaleAnchor = anchor
		w := NewWorld(cfg)
		sellerID := holding(t, w, 100, 100)
		buyerID := w.addAgent(Agent{Maturity: 1, X: 104, Y: 100, Vitality: 20,
			Hunger: 95, Genome: genomeOf(50, 50, 50)})
		giveCoins(t, w, buyerID, 6)
		seller, buyer := mustAgent(t, w, sellerID), mustAgent(t, w, buyerID)
		seller.Hunger, seller.Vitality = hunger, vitality
		seller.carried[0].PricePaid = 4 // it paid four for what it is holding
		return w.salePrice(seller, buyer, &seller.carried[0])
	}
	// Whole and well fed: it wants back what it gave.
	held, ok := priceFor(1, 30, 90)
	loose, ok2 := priceFor(0, 30, 90)
	if !ok2 {
		t.Fatal("no price at all without the anchor")
	}
	if ok && held <= loose {
		t.Fatalf("with the anchor it asks %d and without it %d", held, loose)
	}
	if ok && held < 4 {
		t.Fatalf("it paid four and would take %d", held)
	}
	// And a body that does not expect to be here for the window takes what it
	// can get: the anchor is scaled by that chance, so it washes out.
	sinking, ok3 := priceFor(1, 95, 6)
	sinkingLoose, ok4 := priceFor(0, 95, 6)
	if ok3 != ok4 || (ok3 && sinking != sinkingLoose) {
		t.Fatalf("a sinking body asks %d (%v) with the anchor and %d (%v) without",
			sinking, ok3, sinkingLoose, ok4)
	}
}

// And with the anchor off, nothing about a sale changes however much was paid.
func TestThePriceIsUntouchedWhereNobodyIsAnchored(t *testing.T) {
	prices := [2]int{}
	for i, paid := range []int{0, 9} {
		cfg := priceConfig()
		w := NewWorld(cfg)
		sellerID := holding(t, w, 100, 100)
		buyerID := w.addAgent(Agent{Maturity: 1, X: 104, Y: 100, Vitality: 20,
			Hunger: 95, Genome: genomeOf(50, 50, 50)})
		giveCoins(t, w, buyerID, 6)
		seller, buyer := mustAgent(t, w, sellerID), mustAgent(t, w, buyerID)
		seller.Hunger, seller.Vitality = 60, 30
		seller.carried[0].PricePaid = paid
		prices[i], _ = w.salePrice(seller, buyer, &seller.carried[0])
	}
	if prices[0] != prices[1] {
		t.Fatalf("with the rule off, what was paid moved the price: %d against %d", prices[0], prices[1])
	}
}
