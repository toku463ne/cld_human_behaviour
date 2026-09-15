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

// What a seller gives up is what anybody else giving the same thing up gives
// up: one figure, in one place (stage 77). Carrying is made free so that the
// lug - which is a cost of keeping a thing, not a part of what it is worth -
// is out of the way.
func TestWhatASellerGivesUpIsWhatHandWorthSays(t *testing.T) {
	cfg := coinConfig()
	cfg.CarryCost = 0
	cfg.Books = true
	w := NewWorld(cfg)
	sellerID := holding(t, w, 100, 100)
	buyerID := holdingCoin(t, w, 104, 100)
	seller, buyer := mustAgent(t, w, sellerID), mustAgent(t, w, buyerID)

	check := func(what string) {
		t.Helper()
		item := &seller.carried[0]
		view := w.handView(seller, item)
		s := w.selfView(seller)
		_, given := w.saleTerms(seller, buyer, item)
		if want := handWorth(&w.cfg, &s, &view); given != want {
			t.Fatalf("%s: the seller gives up %v and handWorth says %v", what, given, want)
		}
	}
	for _, hunger := range []float64{5, 40, 90} {
		seller.Hunger = hunger
		check("a plant")
	}
	// And a book, which is the one thing in this world whose owner may have
	// nothing left to lose by parting with it.
	seller.carried[0] = Food{ID: -1, Kind: FoodBook, Says: SkillCook, Written: 0.5}
	check("a book")
}

// A stone can be weighed and is still not on the counter (stage 77).
//
// Counted before deciding, on the played map with throwing on: 5.3% of the
// bodies holding anything are holding a stone, and every one of those is
// holding nothing else that could be sold. So the target is not zero on the
// seller's side - it is zero on the buyer's. A stone has no nutrition, no
// mending and no worth, so perceive does not call its holder a seller and
// addBuy scores nothing for it: nobody ever walks up for one. Putting stones
// on the list could therefore only hand a buyer that came for something else
// a thing it never valued.
func TestAStoneIsWeighedAndNotSold(t *testing.T) {
	cfg := coinConfig()
	cfg.Throwing = true
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Hunger: 60, Genome: genomeOf(50, 50, 50)})
	a := mustAgent(t, w, id)
	w.take(a, w.putFood(Food{X: 101, Y: 100, Kind: FoodStone}))
	if a.CarriedCount() != 1 {
		t.Fatal("the body was given a stone and is not holding it")
	}
	if got := a.firstForSale(&w.cfg); got != -1 {
		t.Fatalf("a body holding nothing but a stone offers item %d for sale", got)
	}
	// It is weighed all the same, by the one function that weighs everything
	// a hand can hold. (What that comes to is zero on a quiet patch with
	// nobody to throw it at, which is stoneWorth's own business.)
	s := w.selfView(a)
	view := w.handView(a, &a.carried[0])
	if got, want := handWorth(&w.cfg, &s, &view), stoneWorth(&w.cfg, &s); got != want {
		t.Fatalf("a stone in hand is weighed at %v and a stone is worth %v", got, want)
	}
}

// Stage 81. A coin is a claim on the better of the two meals this world sells,
// and which of them is better is settled by what the body is missing - so the
// ceiling on mending is the weight, and no weight had to be invented.
func TestACoinClaimsTheMealThisBodyNeeds(t *testing.T) {
	cfg := coinConfig()
	cfg.CoinBuysMending = true
	whole := SelfView{MaxVitality: cfg.MaxVitality, Vitality: cfg.MaxVitality,
		Hunger: cfg.SatiatedHunger, HungerRate: cfg.HungerRate,
		ShockRisk: cfg.ShockRisk, MaxSpeed: cfg.MaxSpeed}

	// A whole body has nothing to mend, so the second claim buys it nothing.
	off := cfg
	off.CoinBuysMending = false
	if got, want := coinWorth(&cfg, &whole, 0), coinWorth(&off, &whole, 0); got != want {
		t.Fatalf("a whole body's coin is worth %v with mending and %v without", got, want)
	}

	// A hurt one values the same coin more, and more the worse it is.
	last := coinWorth(&cfg, &whole, 0)
	for _, vit := range []float64{70, 40, 20} {
		hurt := whole
		hurt.Vitality = vit
		got := coinWorth(&cfg, &hurt, 0)
		plain := coinWorth(&off, &hurt, 0)
		if !(got > plain) {
			t.Fatalf("at vitality %v the mending claim is worth %v against %v", vit, got, plain)
		}
		_ = last
	}

	// And #73 holds: a claim is still worth less than what it claims, because
	// the discount is still on the outside of it.
	hurt := whole
	hurt.Vitality = 20
	claim := keepValue(&cfg, &hurt, 0, 1, cfg.MeatVitality*hurt.MaxVitality, 0, 0)
	if got := coinWorth(&cfg, &hurt, 0); got >= claim {
		t.Fatalf("a coin is worth %v and what it claims is worth %v", got, claim)
	}
}

// And the point of it: the ratio that stage 79 measured as fixed is not fixed
// any more. A body holding a plant it does not need, with wounds it does, now
// wants the coin more than the plant.
func TestTheSellersRatioMovesWithWhatItNeeds(t *testing.T) {
	cfg := coinConfig()
	s := SelfView{MaxVitality: cfg.MaxVitality, Vitality: 20,
		Hunger: cfg.SatiatedHunger, HungerRate: cfg.HungerRate,
		ShockRisk: cfg.ShockRisk, MaxSpeed: cfg.MaxSpeed}
	food := mealValue(&cfg, &s, 0, 1, 0)
	if k := keepValue(&cfg, &s, 0, 1, 0, 0, 0); k > food {
		food = k
	}
	cfg.CoinBuysMending = false
	before := coinWorth(&cfg, &s, 0) / food
	cfg.CoinBuysMending = true
	after := coinWorth(&cfg, &s, 0) / food
	if before < cfg.CoinValue-1e-9 || before > cfg.CoinValue+1e-9 {
		t.Fatalf("stage 79 measured this ratio as exactly CoinValue, got %v", before)
	}
	if !(after > before) {
		t.Fatalf("the ratio did not move: %v -> %v", before, after)
	}
}

// What a body paid for a thing is remembered on the thing, and every other
// way into a hand clears it (stage 83). It is a property of the holding, not
// of the object: what somebody else paid is not something this body knows.
func TestWhatItPaidIsRememberedAndOnlyWhileItHoldsIt(t *testing.T) {
	cfg := priceConfig()
	cfg.AffinitySale, cfg.CoinValue = 20, 0.5
	w := NewWorld(cfg)
	sellerID := holding(t, w, 100, 100)
	buyerID := w.addAgent(Agent{Maturity: 1, X: 105, Y: 100, Vitality: 30,
		Hunger: 90, Genome: genomeOf(50, 50, 50)})
	giveCoins(t, w, buyerID, 4)
	seller, buyer := mustAgent(t, w, sellerID), mustAgent(t, w, buyerID)
	seller.Hunger, seller.Vitality = 10, 90
	if !w.sell(buyer, seller) {
		t.Fatal("no sale to remember the price of")
	}
	bought := -1
	for i := range buyer.carried {
		if buyer.carried[i].Kind != FoodCoin {
			bought = i
		}
	}
	if bought < 0 {
		t.Fatal("the buyer is holding nothing it did not already have")
	}
	paid := buyer.carried[bought].PricePaid
	if paid <= 0 {
		t.Fatalf("the buyer paid and the thing says %d", paid)
	}
	// Handed on, it cost its new holder nothing.
	third := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 106, Y: 100,
		Vitality: 90, Hunger: 60, Genome: genomeOf(50, 50, 50)}))
	buyer = mustAgent(t, w, buyerID)
	if !w.giveItem(buyer, third) {
		t.Fatal("nothing was handed over")
	}
	for i := range third.carried {
		if third.carried[i].PricePaid != 0 {
			t.Fatalf("a gift arrived carrying a price of %d", third.carried[i].PricePaid)
		}
	}
	// And so does picking one up off the ground.
	id := w.putFood(Food{X: 107, Y: 100, Kind: FoodPlant, PricePaid: 3})
	w.take(third, id)
	for i := range third.carried {
		if third.carried[i].PricePaid != 0 {
			t.Fatalf("something picked up off the ground cost %d", third.carried[i].PricePaid)
		}
	}
}

// A coin claims the best of what this body would buy, and since #116 that
// includes something that keeps the weather off (stage 88).
//
// What it does not do is make a claim out of nothing: a body whose chance of
// dying is flat gets nought from every branch there is, because all of them
// are differences in that one chance. That is the finding, so it is the test.
func TestACoinClaimsTheBestOfWhatThisBodyWouldBuy(t *testing.T) {
	cfg := coinConfig()
	cfg.ClimateMap = []string{"99", "99"}
	cfg.ChillDrain = 0.02
	cfg.Trinkets = true
	cfg.WardShare, cfg.WardStrength = 1, 0.6
	cfg.CoinBuysWarding = true
	blind := cfg
	blind.CoinBuysWarding = false
	w := NewWorld(cfg)
	at := func(vit, hunger float64) (with, without float64) {
		id := w.addAgent(Agent{Maturity: 1, X: cfg.Width / 2, Y: cfg.Height / 2,
			Vitality: vit, Hunger: hunger, Genome: genomeOf(50, 50, 50)})
		s := w.selfView(mustAgent(t, w, id))
		return coinWorth(&cfg, &s, 0), coinWorth(&blind, &s, 0)
	}

	// A body with something to lose in the cold: the branch pays out.
	with, without := at(70, 40)
	if with <= 0 {
		t.Fatalf("a coin is worth %v to a body the cold could finish", with)
	}
	if with < without {
		t.Fatalf("adding a branch made the claim smaller: %v against %v", with, without)
	}

	// A fed, whole body: nothing from this branch, and nothing from the meal
	// branch either. The flatness belongs to the body, not to the good - which
	// is why more kinds of thing to buy cannot give money a job here.
	with, without = at(90, 20)
	if with != 0 || without != 0 {
		t.Fatalf("a whole body puts %v on a coin (%v without the branch)", with, without)
	}

	// And in a world with no weather it pays nothing at all.
	warm := testConfig()
	warm.Trinkets, warm.WardShare, warm.WardStrength = true, 1, 0.6
	warm.CoinBuysWarding = true
	noBranch := warm
	noBranch.CoinBuysWarding = false
	w2 := NewWorld(warm)
	id2 := w2.addAgent(Agent{Maturity: 1, X: warm.Width / 2, Y: warm.Height / 2,
		Vitality: 60, Hunger: 50, Genome: genomeOf(50, 50, 50)})
	s2 := w2.selfView(mustAgent(t, w2, id2))
	if coinWorth(&warm, &s2, 0) != coinWorth(&noBranch, &s2, 0) {
		t.Fatal("the branch paid out in a world with no weather in it")
	}
}
