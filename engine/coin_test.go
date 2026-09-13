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
	// Stage 51's own world, which is the one its arithmetic was worked out in:
	// one planning window. A seller that can see two of them values the dinner
	// in its hand and will not part with it - which is not a broken rule but
	// the market stage 67 measured thinning, and stage 71 is what gives it
	// back. Turning it off here keeps these tests about money.
	cfg.LookaheadHorizons = 0
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

// Stage 68: a sale is a hand-over too, and earns what one earns.

func TestSaleGoodwillIsOffByDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.AffinitySale != 0 {
		t.Fatalf("AffinitySale should default to 0, got %v", cfg.AffinitySale)
	}
	if got := saleGoodwill(&cfg, 0); got != 0 {
		t.Fatalf("with it off a sale should earn nothing, got %v", got)
	}
}

// Trust saturates, so the goodwill is worth most between strangers and nothing
// between two who are already close - and its ceiling is LoreValue however
// large the figure is set. That ceiling is the falsifiable half of the stage.
func TestSaleGoodwillSaturates(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AffinitySale = 6

	stranger := saleGoodwill(&cfg, 0)
	known := saleGoodwill(&cfg, cfg.AffinityTrust*0.9)
	close := saleGoodwill(&cfg, cfg.AffinityTrust*2)
	if !(stranger > known && known > close) {
		t.Fatalf("goodwill should be worth most to a stranger: %v %v %v", stranger, known, close)
	}
	if close != 0 {
		t.Fatalf("somebody already trusted has nothing left to buy, got %v", close)
	}

	// And no setting can get past LoreValue.
	for _, amount := range []float64{20, 40, 400} {
		cfg.AffinitySale = amount
		if got := saleGoodwill(&cfg, 0); got > cfg.LoreValue+1e-9 {
			t.Fatalf("AffinitySale %v bought %v, above the LoreValue ceiling %v", amount, got, cfg.LoreValue)
		}
	}
	cfg.AffinitySale = 20
	atCeiling := saleGoodwill(&cfg, 0)
	cfg.AffinitySale = 40
	if twice := saleGoodwill(&cfg, 0); twice != atCeiling {
		t.Fatalf("twice the ceiling should buy the same: %v vs %v", twice, atCeiling)
	}
}

// What the stage is for: the seller's side of the comparison is short by the
// discount on the coin, and the goodwill is what makes it up. The pre-count
// says the ceiling is narrower than the mean shortfall, so what must hold is
// the direction - adding it can never make a seller keener to refuse.
func TestSaleGoodwillNeverMakesASellerRefuse(t *testing.T) {
	build := func(sale float64) (*World, *Agent, *Agent) {
		cfg := coinConfig()
		cfg.AffinitySale = sale
		w := NewWorld(cfg)
		seller := mustAgent(t, w, holding(t, w, 100, 100))
		buyer := mustAgent(t, w, holdingCoin(t, w, 105, 100))
		return w, seller, buyer
	}
	for _, hunger := range []float64{10, 30, 50, 70, 90} {
		wOff, sOff, bOff := build(0)
		sOff.Hunger = hunger
		off := wOff.willSell(sOff, bOff, &sOff.carried[0])

		wOn, sOn, bOn := build(20)
		sOn.Hunger = hunger
		on := wOn.willSell(sOn, bOn, &sOn.carried[0])

		if off && !on {
			t.Fatalf("at hunger %v the goodwill turned a sale into a refusal", hunger)
		}
	}
}

// And it is written both ways, through the same call a hand-over makes.
func TestASaleLeavesBothOnBetterTerms(t *testing.T) {
	cfg := coinConfig()
	cfg.AffinitySale, cfg.CoinValue = 20, 0.5
	w := NewWorld(cfg)
	seller := mustAgent(t, w, holding(t, w, 100, 100))
	buyer := mustAgent(t, w, holdingCoin(t, w, 105, 100))
	seller.Hunger, seller.Vitality = 10, 90
	buyer.Hunger, buyer.Vitality = 90, 30

	if !w.sell(buyer, seller) {
		t.Skip("this pair would not trade; how far the goodwill reaches is the stage's own finding")
	}
	for _, pair := range [][2]*Agent{{seller, buyer}, {buyer, seller}} {
		op := w.opinionOf(pair[0], pair[1].ID)
		if op == nil || w.decayedAffinity(pair[0], op) <= 0 {
			t.Fatalf("agent %d should think better of %d after the trade", pair[0].ID, pair[1].ID)
		}
	}
}

// scoredTraced is scored() with the breakdown kept, so that a test can read
// what an option was reckoned to be worth rather than only what it came to
// after the costs.
func scoredTraced(t *testing.T, w *World, id int) (*AIController, *Perception) {
	t.Helper()
	p := w.perceive(mustAgent(t, w, id))
	p.Trace = &DecisionTrace{}
	c := &AIController{}
	c.Decide(p)
	p.Trace = nil
	return c, p
}

// coinAndMeal is what this body reckoned picking up the money and picking up
// the food were worth, before either was charged for the walk.
func coinAndMeal(t *testing.T, w *World, id, coinID, foodID int) (coin, meal Goal) {
	t.Helper()
	c, _ := scoredTraced(t, w, id)
	for i := range c.opts {
		o := &c.opts[i]
		if o.action.Kind != ActTake {
			continue
		}
		switch o.action.TargetID {
		case coinID:
			coin = c.terms[i].Life
		case foodID:
			meal = c.terms[i].Life
		}
	}
	return coin, meal
}

// A coin is a claim on a meal, so it cannot be worth more than the meal it
// claims. coin.go has said so since the day it was written, and the scoring
// did not obey it: picking money up was given a chance of one and no race,
// where picking the same meal up pays CarryValue, the race and the weight. Two
// conditions have to hold before a coin feeds anybody - this body running
// short, and somebody willing to sell when it does - and the old pricing paid
// one discount for both. Found in stage 67, put right afterwards.
func TestACoinIsNeverWorthMoreThanTheMealItClaims(t *testing.T) {
	cfg := coinConfig()
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Hunger: 85, Genome: genomeOf(50, 50, 50)})
	coinID := w.putFood(Food{X: 110, Y: 100, Kind: FoodCoin})
	foodID := w.putFood(Food{X: 90, Y: 100, Kind: FoodPlant})
	// Somebody else in sight, the same distance from both: what is kept for
	// later is worth nothing at all in a patch with nobody to lose it to, and
	// that is true of a coin and a berry alike.
	w.addAgent(Agent{Maturity: 1, X: 100, Y: 160, Vitality: 90,
		Hunger: 85, Genome: genomeOf(50, 50, 50)})

	coin, meal := coinAndMeal(t, w, id, coinID, foodID)
	if coin.Value <= 0 || meal.Value <= 0 {
		t.Fatalf("nothing was worth having: coin %+v meal %+v", coin, meal)
	}
	if coin.Score() > meal.Score() {
		t.Fatalf("a coin outscores the meal it is a claim on: coin %+v meal %+v", coin, meal)
	}
	if coin.Chance >= 1 {
		t.Fatalf("money is a sure thing: %+v", coin)
	}

	// And the world it was measured in is still there, where it is the other
	// way round.
	cfg.CoinPricedCertain = true
	old := NewWorld(cfg)
	id = old.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Hunger: 85, Genome: genomeOf(50, 50, 50)})
	coinID = old.putFood(Food{X: 110, Y: 100, Kind: FoodCoin})
	foodID = old.putFood(Food{X: 90, Y: 100, Kind: FoodPlant})
	old.addAgent(Agent{Maturity: 1, X: 100, Y: 160, Vitality: 90,
		Hunger: 85, Genome: genomeOf(50, 50, 50)})
	coin, meal = coinAndMeal(t, old, id, coinID, foodID)
	if coin.Chance != 1 {
		t.Fatalf("the old pricing is not a sure thing any more: %+v", coin)
	}
	if coin.Score() <= meal.Score() {
		t.Fatalf("the old pricing no longer beats the meal: coin %+v meal %+v", coin, meal)
	}
}

// Money lying about is raced for like anything else lying about. Both worlds
// have exactly one other body in sight, so what is compared is where it is
// standing and nothing else - how crowded the patch feels is the same figure
// in both.
func TestMoneyOnTheGroundIsRacedFor(t *testing.T) {
	chanceOf := func(rivalX, rivalY float64) float64 {
		w := NewWorld(coinConfig())
		id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
			Hunger: 85, Genome: genomeOf(50, 50, 50)})
		coinID := w.putFood(Food{X: 150, Y: 100, Kind: FoodCoin})
		w.addAgent(Agent{Maturity: 1, X: rivalX, Y: rivalY, Vitality: 90,
			Hunger: 85, Genome: genomeOf(50, 50, 50)})
		c, _ := scoredTraced(t, w, id)
		for i := range c.opts {
			if c.opts[i].action.Kind == ActTake && c.opts[i].action.TargetID == coinID {
				return c.terms[i].Life.Chance
			}
		}
		t.Fatal("nobody scored the coin")
		return 0
	}
	far, near := chanceOf(100, 150), chanceOf(152, 100)
	if near >= far {
		t.Fatalf("a body standing on the coin is no rival for it: %v with one there, %v with one away", near, far)
	}
}
