package engine

import "testing"

// The watch is a measurement and not a rule: a world that is being read comes
// out exactly where a world that is not comes out, and no draw goes on it.
func TestTheTradeWatchIsNotARule(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 7
	watched, ignored := NewWorld(cfg), NewWorld(cfg)
	for i := 0; i < 400; i++ {
		watched.Step()
		watched.Trade()
		ignored.Step()
	}
	if got, want := digest(watched), digest(ignored); got != want {
		t.Fatal("reading the trade watch changed the world")
	}
	if got, want := watched.draws.draws, ignored.draws.draws; got != want {
		t.Fatalf("reading the trade watch drew %d times against %d", got, want)
	}
}

// cookedIn puts something already prepared in a body's hand.
func cookedIn(t *testing.T, w *World, id int, quality float64) {
	t.Helper()
	a := mustAgent(t, w, id)
	if len(a.carried) == 0 {
		t.Fatal("nothing in hand to cook")
	}
	a.carried[0].Cooked = quality
}

// A cooked thing changing hands is counted by the path it went down, and by
// whether the receiver could have managed it itself.
func TestACookedHandOverIsCountedByThePathItWentDown(t *testing.T) {
	cfg := coinConfig()
	cfg.CookQuality = 0.2 // a world where cooking is worth knowing
	w := NewWorld(cfg)

	giver := mustAgent(t, w, holding(t, w, 100, 100))
	taker := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 101, Y: 100,
		Vitality: 90, Hunger: 20, Genome: genomeOf(50, 50, 50)}))
	cookedIn(t, w, giver.ID, 0.9)
	if !w.giveItem(giver, taker) {
		t.Fatal("the cooked thing did not change hands")
	}

	sellerID := holding(t, w, 300, 100)
	buyerID := holdingCoin(t, w, 304, 100)
	seller, buyer := mustAgent(t, w, sellerID), mustAgent(t, w, buyerID)
	// Whole as well as fed: a cooked thing mends, and a seller with wounds to
	// mend would be parting with that too.
	seller.Hunger, seller.Vitality = 10, seller.MaxVitality(&cfg)
	buyer.Hunger, buyer.Vitality = 90, 20
	cookedIn(t, w, sellerID, 0.9)
	if !w.sell(buyer, seller) {
		t.Fatal("the cooked thing was not sold")
	}

	got := w.Trade()
	if got.CookedGiven != 1 || got.CookedSold != 1 {
		t.Fatalf("%d given and %d sold, wanted one of each", got.CookedGiven, got.CookedSold)
	}
	// Both receivers cook at 0.2 and were handed 0.9, so both got something
	// they could not have made: this is the figure that says whether cooking
	// is a service at all.
	if got.Better != 1 {
		t.Fatalf("%v of the hand-overs beat what the receiver could make", got.Better)
	}
	if want := 0.7; got.Gap < want-1e-9 || got.Gap > want+1e-9 {
		t.Fatalf("the cooked things beat the receivers by %v, wanted %v", got.Gap, want)
	}
}

// And a body handed something no better than it makes itself did not need
// anybody for it.
func TestACookedHandOverToAnEqualCookIsNotCountedAsBetter(t *testing.T) {
	cfg := coinConfig()
	cfg.CookQuality = 0.9
	w := NewWorld(cfg)
	giver := mustAgent(t, w, holding(t, w, 100, 100))
	taker := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 101, Y: 100,
		Vitality: 90, Hunger: 20, Genome: genomeOf(50, 50, 50)}))
	cookedIn(t, w, giver.ID, 0.9)
	if !w.giveItem(giver, taker) {
		t.Fatal("the cooked thing did not change hands")
	}
	if got := w.Trade(); got.CookedGiven != 1 || got.Better != 0 {
		t.Fatalf("%d given, %v of them better than the receiver could manage",
			got.CookedGiven, got.Better)
	}
}

// Two that failed to agree and agreed later are counted as that, and the
// watch says which of the seller's two figures had moved. Both are the
// seller's: the buyer has no threshold of its own.
func TestARefusalThatTurnsIntoASaleIsCounted(t *testing.T) {
	w := NewWorld(coinConfig())
	sellerID := holding(t, w, 100, 100)
	buyerID := holdingCoin(t, w, 104, 100)
	seller, buyer := mustAgent(t, w, sellerID), mustAgent(t, w, buyerID)

	// A seller that is about to need what it is holding says no.
	seller.Hunger, seller.Vitality = 90, 15
	if w.sell(buyer, seller) {
		t.Fatal("a body that is about to starve sold its dinner")
	}
	if got := w.Trade(); got.Refused != 1 || got.Met != 0 {
		t.Fatalf("%d refused and %d met again after one refusal", got.Refused, got.Met)
	}

	// Later, fed, it says yes to the same buyer.
	w.tick += 500
	seller, buyer = mustAgent(t, w, sellerID), mustAgent(t, w, buyerID)
	seller.Hunger, seller.Vitality = 10, 90
	if !w.sell(buyer, seller) {
		t.Fatal("a full seller still would not part with it")
	}
	got := w.Trade()
	if got.Met != 1 || got.Won != 1 {
		t.Fatalf("%d meetings after a refusal, %d of them sales", got.Met, got.Won)
	}
	if got.Gap2 != 500 {
		t.Fatalf("the two met again after %v ticks, wanted 500", got.Gap2)
	}
	// What moved was the food: a fed body wants its dinner less.
	if got.FoodFell <= 0 {
		t.Fatalf("the seller said yes with the food worth %v more", -got.FoodFell)
	}
	// And the pair is off the books, so a third meeting is not a second
	// conversion of the same refusal.
	if _, open := w.trade.open[[2]int{buyerID, sellerID}]; open {
		t.Fatal("a pair that came to terms is still down as having refused")
	}
}

// Splitting the comparison in two did not change it.
func TestSaleTermsAreTheComparisonWillSellMakes(t *testing.T) {
	w := NewWorld(coinConfig())
	sellerID := holding(t, w, 100, 100)
	buyerID := holdingCoin(t, w, 104, 100)
	seller, buyer := mustAgent(t, w, sellerID), mustAgent(t, w, buyerID)
	for _, hunger := range []float64{5, 30, 60, 90} {
		seller.Hunger = hunger
		coin, food := w.saleTerms(seller, buyer, &seller.carried[0])
		if got, want := w.willSell(seller, buyer, &seller.carried[0]), coin > food; got != want {
			t.Fatalf("at hunger %v willSell says %v and the terms say %v", hunger, got, want)
		}
	}
}
