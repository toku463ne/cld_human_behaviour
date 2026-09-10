package engine

import "testing"

// coinConfig is a world with money in it and hands to hold it.
//
// The metabolism is left running, unlike the still world the other tests here
// use: money is worth what it will buy when a body runs short, so in a world
// where nobody ever runs short a coin is worth nothing and neither is a meal.
// That is not a quirk of the test - it is the whole of what stage 50 found,
// and it is why this config has to put hunger back.
func coinConfig() Config {
	cfg := giftConfig()
	def := DefaultConfig()
	cfg.HungerRate, cfg.StarveRate, cfg.RegenRate = def.HungerRate, def.StarveRate, def.RegenRate
	cfg.OfferTicks = 30
	return cfg
}

// holdingCoin puts a body somewhere with money in its hand.
func holdingCoin(t *testing.T, w *World, x, y float64) int {
	t.Helper()
	id := w.addAgent(Agent{Maturity: 1, X: x, Y: y, Vitality: 90,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	w.take(mustAgent(t, w, id), w.putFood(Food{X: x + 1, Y: y, Kind: FoodCoin}))
	if mustAgent(t, w, id).CarriedCount() != 1 {
		t.Fatal("the body was given a coin and is not holding it")
	}
	return id
}

// Money is not food: nobody eats it, and it is not in the list of things to
// eat even for a body standing on it.
func TestNobodyEatsMoney(t *testing.T) {
	w := NewWorld(coinConfig())
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Hunger: 80, Genome: genomeOf(50, 50, 50)})
	w.putFood(Food{X: 101, Y: 100, Kind: FoodCoin})
	a := mustAgent(t, w, id)
	p := w.perceive(a)
	if len(p.Foods) != 0 {
		t.Fatalf("a starving body sees %d things to eat, and the only thing there is a coin", len(p.Foods))
	}
	if len(p.Coins) != 1 {
		t.Fatalf("%d coins in sight", len(p.Coins))
	}
	if w.canEat(a, w.foodByID(p.Coins[0].ID)) {
		t.Fatal("a coin can be eaten")
	}
}

// Money weighs nothing, and that is the whole of what it has over a meal.
func TestMoneyWeighsNothing(t *testing.T) {
	cfg := coinConfig()
	w := NewWorld(cfg)
	withFood := mustAgent(t, w, holding(t, w, 100, 100))
	withCoin := mustAgent(t, w, holdingCoin(t, w, 200, 100))
	if got := withFood.burden(&cfg); got <= 1 {
		t.Fatalf("carrying a meal is free: burden %v", got)
	}
	if got := withCoin.burden(&cfg); got != 1 {
		t.Fatalf("carrying a coin costs something: burden %v", got)
	}
	// And it still takes a hand: a body holding a coin is not holding dinner.
	if withCoin.canCarryMore(&cfg) {
		t.Fatal("a body holding a coin has a hand free in a world with one hand")
	}
}

// Money does not crowd the plants out. This is the bug stones had when they
// went in (stage 45), and the reason it is a test rather than a comment.
func TestMoneyTakesNoRoomFromWhatGrows(t *testing.T) {
	grown := func(coins int) int {
		cfg := testConfig()
		cfg.Seed = 7
		cfg.Coins = coins
		cfg.FoodSpawnRate = DefaultConfig().FoodSpawnRate
		w := NewWorld(cfg)
		for i := 0; i < 4000; i++ {
			w.Step()
		}
		return w.countKind(FoodPlant)
	}
	none, many := grown(0), grown(200)
	if many < none {
		t.Fatalf("a world with 200 coins grew %d plants and one with none grew %d", many, none)
	}
}

// The whole of the rule: a coin and a meal change places, and nothing else
// happens - no goodwill, no memory of it, no price written anywhere.
func TestASaleMovesOneThingEachWay(t *testing.T) {
	w := NewWorld(coinConfig())
	sellerID := holding(t, w, 100, 100)
	buyerID := holdingCoin(t, w, 104, 100)
	seller, buyer := mustAgent(t, w, sellerID), mustAgent(t, w, buyerID)
	// A seller with no use for what it holds and a buyer who is hungry.
	seller.Hunger, seller.Vitality = 10, 90
	buyer.Hunger, buyer.Vitality = 90, 20

	plants, coins := w.countKind(FoodPlant), w.countKind(FoodCoin)
	if !w.sell(buyer, seller) {
		t.Fatal("a sale between a full seller and a starving buyer did not happen")
	}
	if buyer.carriedIndex2(FoodPlant) < 0 || seller.carriedIndex2(FoodCoin) < 0 {
		t.Fatal("the two things did not change places")
	}
	if got, want := w.countKind(FoodPlant), plants; got != want {
		t.Fatalf("a sale left %d plants in the world, was %d", got, want)
	}
	if got, want := w.countKind(FoodCoin), coins; got != want {
		t.Fatalf("a sale left %d coins in the world, was %d", got, want)
	}
	if w.Coins().Sales != 1 {
		t.Fatalf("%d sales counted", w.Coins().Sales)
	}
	// A sale is not a favour.
	if a := w.Opinions(seller.ID)[buyer.ID].Affinity; a != 0 {
		t.Fatalf("the seller thinks %v of the buyer for paying", a)
	}
}

// The seller is asked, and a body that would rather keep what it has says no.
func TestASellerThatWouldRatherKeepItSaysNo(t *testing.T) {
	w := NewWorld(coinConfig())
	sellerID := holding(t, w, 100, 100)
	buyerID := holdingCoin(t, w, 104, 100)
	seller, buyer := mustAgent(t, w, sellerID), mustAgent(t, w, buyerID)
	// A seller that is about to need what it is holding.
	seller.Hunger, seller.Vitality = 90, 15

	if w.sell(buyer, seller) {
		t.Fatal("a starving body sold its last meal for a coin")
	}
	if w.Coins().Refused != 1 {
		t.Fatalf("%d refusals counted", w.Coins().Refused)
	}
	if seller.carriedIndex2(FoodPlant) < 0 || buyer.carriedIndex2(FoodCoin) < 0 {
		t.Fatal("a refused sale moved something anyway")
	}
}

// Money can only buy what is being held out. A meal in a hand is invisible
// (stage 40) until it is cried (stage 49), and that is the shop window.
func TestOnlyWaresOnOfferCanBeBought(t *testing.T) {
	w := NewWorld(coinConfig())
	sellerID := holding(t, w, 100, 100)
	buyerID := holdingCoin(t, w, 130, 100)
	buyer := mustAgent(t, w, buyerID)
	buyer.Hunger, buyer.Vitality = 90, 20

	if buys(t, w, buyerID) {
		t.Fatal("somebody was offered a way of buying from a body that is not selling")
	}
	mustAgent(t, w, sellerID).Action = Action{Kind: ActOffer}
	if !buys(t, w, buyerID) {
		t.Fatal("a hungry body with a coin was offered no way of buying from a crier")
	}
}

// A body with no coin cannot buy, and a coin worth nothing is never picked up.
func TestNoCoinNoPurchaseAndNoValueNoCoin(t *testing.T) {
	w := NewWorld(coinConfig())
	sellerID := holding(t, w, 100, 100)
	mustAgent(t, w, sellerID).Action = Action{Kind: ActOffer}
	brokeID := w.addAgent(Agent{Maturity: 1, X: 130, Y: 100, Vitality: 20,
		Hunger: 90, Genome: genomeOf(50, 50, 50)})
	if buys(t, w, brokeID) {
		t.Fatal("a body with no money was offered a way of buying")
	}

	// And the placebo: money nobody values is money nobody walks to.
	cfg := coinConfig()
	cfg.CoinValue = 0
	dead := NewWorld(cfg)
	id := dead.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 20,
		Hunger: 90, Genome: genomeOf(50, 50, 50)})
	dead.putFood(Food{X: 110, Y: 100, Kind: FoodCoin})
	c, _ := scored(t, dead, id)
	for _, o := range c.opts {
		if o.action.Kind == ActTake {
			t.Fatal("somebody went for a coin worth nothing")
		}
	}
}

// buys reports whether the comparison this body would run contains a purchase.
func buys(t *testing.T, w *World, id int) bool {
	t.Helper()
	c, _ := scored(t, w, id)
	for _, o := range c.opts {
		if o.action.Kind == ActBuy {
			return true
		}
	}
	return false
}

// A gift is refused if the receiver could do nothing with it. Found in stage
// 51: what a species can digest and the rule that nobody eats its own kind
// were both being got round through the hand.
func TestNothingIsGivenToSomebodyWhoCouldNotUseIt(t *testing.T) {
	w := NewWorld(coinConfig())
	humanID := holding(t, w, 100, 100)
	enemy := w.addAgent(Agent{Maturity: 1, X: 104, Y: 100, Vitality: 90,
		Species: SpeciesEnemy, Genome: genomeOf(50, 50, 50)})
	human, beast := mustAgent(t, w, humanID), mustAgent(t, w, enemy)

	if w.giveItem(human, beast) {
		t.Fatal("something that lives on meat was handed a plant")
	}
	if beast.CarriedCount() != 0 {
		t.Fatal("the plant went across anyway")
	}
	// Money is another matter: anybody can hold a coin.
	coinID := holdingCoin(t, w, 108, 100)
	if !w.giveItem(mustAgent(t, w, coinID), beast) {
		t.Fatal("a coin could not be handed over")
	}
}

// And what cannot be eaten is not a meal in the hand: it is not in the list of
// things to eat, and eating it is refused.
func TestWhatCannotBeEatenIsNotAMealInTheHand(t *testing.T) {
	w := NewWorld(coinConfig())
	id := holdingCoin(t, w, 100, 100)
	a := mustAgent(t, w, id)
	a.Hunger = 90

	p := w.perceive(a)
	for _, f := range p.Foods {
		if f.Kind == FoodCoin {
			t.Fatal("a coin in the hand is in the list of things to eat")
		}
	}
	coin := a.carried[0].ID
	before := a.Hunger
	w.eatCarried(a, coin)
	if a.CarriedCount() != 1 || a.Hunger != before {
		t.Fatalf("a coin was eaten: holding %d, hunger %.2f -> %.2f",
			a.CarriedCount(), before, a.Hunger)
	}
}
