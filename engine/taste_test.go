package engine

import (
	"math"
	"testing"
)

// tasteConfig is the ornament world where two bodies can want different
// things (stage 84).
func tasteConfig() Config {
	cfg := trinketConfig()
	cfg.TrinketTaste = 1
	cfg.AdornNeedsSurvival = true
	return cfg
}

// The same piece is worth different things to different bodies. Nothing else
// in this world does that, and it is the whole of why there is anything to
// trade: two bodies that price a thing identically have no reason to swap it.
func TestTheSamePieceIsWantedDifferently(t *testing.T) {
	w := NewWorld(tasteConfig())
	near := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100,
		Vitality: 90, Hunger: 20, Genome: genomeOf(50, 50, 50)}))
	far := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 140, Y: 100,
		Vitality: 90, Hunger: 20, Genome: genomeOf(50, 50, 50)}))
	near.taste, far.taste = 0.25, 0.75
	near.adornWant, far.adornWant = 1, 1
	piece := Food{Kind: FoodTrinket, Made: 1, Style: 0.25}

	mine, theirs := w.trinketWorth(near, &piece), w.trinketWorth(far, &piece)
	if mine <= theirs {
		t.Fatalf("the piece is worth %v to the body that likes it and %v to the one that does not", mine, theirs)
	}
	if theirs < 0 {
		t.Fatalf("a piece nobody likes is worth %v, which is less than nothing", theirs)
	}
	// And with no taste in the world they are the same figure, which is the
	// world stage 82 measured.
	flat := NewWorld(trinketConfig())
	a := mustAgent(t, flat, flat.addAgent(Agent{Maturity: 1, X: 100, Y: 100,
		Vitality: 90, Hunger: 20, Genome: genomeOf(50, 50, 50)}))
	b := mustAgent(t, flat, flat.addAgent(Agent{Maturity: 1, X: 140, Y: 100,
		Vitality: 90, Hunger: 20, Genome: genomeOf(50, 50, 50)}))
	a.taste, b.taste = 0.25, 0.75
	if flat.trinketWorth(a, &piece) != flat.trinketWorth(b, &piece) {
		t.Fatal("two bodies disagree about a piece in a world with no taste in it")
	}
}

// A taste is a point on a circle: 0.98 is next to 0.02 and not most of a world
// away from it. The chronotype had to know this and so does this.
func TestATasteIsACircle(t *testing.T) {
	w := NewWorld(tasteConfig())
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100,
		Vitality: 90, Hunger: 20, Genome: genomeOf(50, 50, 50)}))
	a.taste = 0.98
	near := Food{Kind: FoodTrinket, Made: 1, Style: 0.02}
	far := Food{Kind: FoodTrinket, Made: 1, Style: 0.48}
	if w.trinketDelight(a, &near) <= w.trinketDelight(a, &far) {
		t.Fatalf("across midnight: %v against %v", w.trinketDelight(a, &near), w.trinketDelight(a, &far))
	}
}

// It moves want about rather than adding any: over the pieces this world
// turns out, the multiplier averages one. That is what makes the dose
// readable - a world with a strong taste in it wants no more ornaments in
// total than a world with none.
func TestATasteMovesWantAboutRatherThanAddingIt(t *testing.T) {
	w := NewWorld(tasteConfig())
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100,
		Vitality: 90, Hunger: 20, Genome: genomeOf(50, 50, 50)}))
	a.taste = 0.3
	sum, n := 0.0, 2000
	for i := 0; i < n; i++ {
		f := Food{Kind: FoodTrinket, Made: 1, Style: float64(i) / float64(n)}
		sum += w.trinketDelight(a, &f)
	}
	if mean := sum / float64(n); math.Abs(mean-1) > 0.01 {
		t.Fatalf("the average piece is wanted %v times as much as it would be with no taste", mean)
	}
}

// A taste is inherited whole from one parent with a drift, and it wraps.
func TestATasteIsInheritedAndWraps(t *testing.T) {
	cfg := tasteConfig()
	cfg.TrinketTasteMutation = 0.2
	w := NewWorld(cfg)
	pa := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100,
		Vitality: 90, Genome: genomeOf(50, 50, 50)}))
	pb := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 104, Y: 100,
		Vitality: 90, Genome: genomeOf(50, 50, 50)}))
	pa.taste, pb.taste = 0.02, 0.98
	for i := 0; i < 200; i++ {
		got := w.inheritTaste(pa, pb)
		if got < 0 || got >= 1 {
			t.Fatalf("a child's taste came out at %v, which is off the circle", got)
		}
	}
	// And the budget is untouched: a taste is a direction, not an amount.
	before := pa.Budget()
	pa.taste = 0.5
	if pa.Budget() != before {
		t.Fatal("a taste cost the body some of its budget")
	}
}

// A body that does not expect to be there for it does not want it. This is
// the whole difference between an ornament and a meal: the meal is worth more
// to a body that is running out, and the ornament is worth less.
func TestNobodyWantsAnOrnamentOnTheWayOut(t *testing.T) {
	cfg := tasteConfig()
	cfg.TrinketTaste = 0 // one thing at a time
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	dyingID := w.addAgent(Agent{Maturity: 1, X: 400, Y: 400, Vitality: 4,
		Hunger: 95, Genome: genomeOf(50, 50, 50)})
	w.priceHands()
	piece := Food{Kind: FoodTrinket, Made: 1}
	well := w.trinketWorth(mustAgent(t, w, id), &piece)
	dying := w.trinketWorth(mustAgent(t, w, dyingID), &piece)
	if well <= dying {
		t.Fatalf("a whole body wants one %v and a dying one %v", well, dying)
	}
	if dying < 0 || dying >= well {
		t.Fatalf("the dying body's figure is %v", dying)
	}
	// It is a slope and not a gate: there is no vitality at which the want
	// switches off, which is the rule this project does not break.
	seen := map[float64]bool{}
	for v := 5.0; v <= 95; v += 10 {
		b := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 400, Y: 400,
			Vitality: v, Hunger: 80, Genome: genomeOf(50, 50, 50)}))
		w.priceHands()
		seen[w.trinketWorth(b, &piece)] = true
	}
	if len(seen) < 5 {
		t.Fatalf("the want takes %d values over the range of a body's vitality", len(seen))
	}
}

// With the want off, the world draws nothing for any of this: no style, no
// taste, and every figure is the one stage 82 measured.
func TestAWorldWithNoTasteDrawsNone(t *testing.T) {
	cfg := trinketConfig()
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	if a := mustAgent(t, w, id); a.taste != 0 {
		t.Fatalf("a body in a world with no taste has one: %v", a.taste)
	}
	a := mustAgent(t, w, id)
	a.Action = Action{Kind: ActCraft}
	a.actionTicks = cfg.CraftTicks
	w.craft(a)
	if len(a.carried) != 1 || a.carried[0].Style != 0 {
		t.Fatalf("a piece made where nobody has a taste came out as style %v", a.carried[0].Style)
	}
}

// What a body parts with is the thing it minds least, rather than whatever it
// picked up first (stage 84c). The principle is firstForSale's own; what is
// new is that there is now something to compare.
func TestABodyPartsWithWhatItMindsLeast(t *testing.T) {
	cfg := tasteConfig()
	cfg.HandOverCheapest = true
	w := NewWorld(cfg)
	giverID := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	takerID := w.addAgent(Agent{Maturity: 1, X: 104, Y: 100, Vitality: 90,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	giver, taker := mustAgent(t, w, giverID), mustAgent(t, w, takerID)
	giver.taste = 0.5
	// The one it loves first in the hand, the one it does not care for second.
	giver.carried = append(giver.carried,
		Food{ID: 1, Kind: FoodTrinket, Made: 1, Style: 0.5},
		Food{ID: 2, Kind: FoodTrinket, Made: 1, Style: 0.0})
	w.priceHands()
	// What it holds out to be seen is the one it does not care for: a body
	// does not advertise one thing and sell another.
	giver.Action = Action{Kind: ActOffer}
	if got := w.offering(giver); got == nil || got.ID != 2 {
		t.Fatalf("it held up %v", got)
	}
	giver.Action = Action{}
	if !w.giveItem(giver, taker) {
		t.Fatal("nothing was handed over")
	}
	if got := taker.carried[len(taker.carried)-1].ID; got != 2 {
		t.Fatalf("it handed over piece %d, which is the one it likes", got)
	}
	if giver.carried[0].ID != 1 {
		t.Fatal("it kept the wrong one")
	}
}

// An ornament can be bought. It could not, in any world, until stage 84: the
// option asked what was on the counter for its nutrition, and an ornament has
// none - so every sale figure recorded for stages 69 and 82 was taken in a
// world where money bought food and nothing else.
func TestAnOrnamentCanBeBought(t *testing.T) {
	cfg := tasteConfig()
	cfg.TrinketTaste = 0
	w := NewWorld(cfg)
	sellerID := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	seller := mustAgent(t, w, sellerID)
	seller.carried = append(seller.carried, Food{ID: 5, Kind: FoodTrinket, Made: 1})
	seller.Action = Action{Kind: ActOffer}
	buyerID := holdingCoin(t, w, 130, 100)
	w.priceHands()
	if !buys(t, w, buyerID) {
		t.Fatal("nobody can buy an ornament")
	}
	// And the world where they could not is still reachable, because every
	// figure this project has recorded about selling came from it.
	old := cfg
	old.CoinBuysOnlyMeals = true
	w2 := NewWorld(old)
	s2 := mustAgent(t, w2, w2.addAgent(Agent{Maturity: 1, X: 100, Y: 100,
		Vitality: 90, Hunger: 20, Genome: genomeOf(50, 50, 50)}))
	s2.carried = append(s2.carried, Food{ID: 5, Kind: FoodTrinket, Made: 1})
	s2.Action = Action{Kind: ActOffer}
	b2 := holdingCoin(t, w2, 130, 100)
	w2.priceHands()
	if buys(t, w2, b2) {
		t.Fatal("the old world bought an ornament")
	}
}

// And what it goes for follows the buyer's own liking for it, which is the
// first price in this world that two buyers would disagree about.
func TestThePriceFollowsWhoIsBuying(t *testing.T) {
	cfg := tasteConfig()
	w := NewWorld(cfg)
	// A seller that puts something on money: one that never runs short takes
	// no price at all, which is stage 79's wall and not this rule.
	sellerID := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100,
		Vitality: 60, Hunger: 70, Genome: genomeOf(50, 50, 50)})
	// Buyers that put something on money as well: a body that never runs
	// short would pay any price, which is no comparison at all.
	keenID := w.addAgent(Agent{Maturity: 1, X: 130, Y: 100, Vitality: 70,
		Hunger: 65, Genome: genomeOf(50, 50, 50)})
	coolID := w.addAgent(Agent{Maturity: 1, X: 130, Y: 140, Vitality: 70,
		Hunger: 65, Genome: genomeOf(50, 50, 50)})
	// Pointers only once nobody else is being born: the slice moves.
	seller := mustAgent(t, w, sellerID)
	keen, cool := mustAgent(t, w, keenID), mustAgent(t, w, coolID)
	keen.carried = append(keen.carried, Food{Kind: FoodCoin})
	cool.carried = append(cool.carried, Food{Kind: FoodCoin})
	seller.taste = 0.5
	seller.carried = append(seller.carried, Food{ID: 5, Kind: FoodTrinket, Made: 1, Style: 0})
	keen.taste, cool.taste = 0, 0.15
	w.priceHands()
	_, keenTop, ok := w.saleRange(seller, keen, &seller.carried[0])
	if !ok {
		t.Fatal("no range at all for the keen buyer")
	}
	_, coolTop, ok := w.saleRange(seller, cool, &seller.carried[0])
	if !ok {
		t.Fatal("no range at all for the cool buyer")
	}
	if keenTop <= coolTop {
		t.Fatalf("the keen buyer would go to %v and the cool one to %v", keenTop, coolTop)
	}
}

// Giving something away costs what it was worth, where that rule is on: a
// body does not hand over what it minds losing more than the goodwill buys.
func TestAPricedGiftWeighsWhatItGivesUp(t *testing.T) {
	cfg := tasteConfig()
	cfg.GiftPriced, cfg.HandOverCheapest = true, true
	w := NewWorld(cfg)
	giverID := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	w.addAgent(Agent{Maturity: 1, X: 108, Y: 100, Vitality: 90,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	giver := mustAgent(t, w, giverID)
	giver.taste = 0.5
	giver.carried = append(giver.carried, Food{ID: 1, Kind: FoodTrinket, Made: 1, Style: 0.5})
	w.priceHands()
	dear, _ := scored(t, w, giverID)
	gaveDear := false
	for _, o := range dear.opts {
		if o.action.Kind == ActGive {
			gaveDear = true
		}
	}
	// The same body holding a piece it does not care for at all.
	giver = mustAgent(t, w, giverID)
	giver.carried[0].Style = 0
	w.priceHands()
	cheap, _ := scored(t, w, giverID)
	gaveCheap := false
	for _, o := range cheap.opts {
		if o.action.Kind == ActGive {
			gaveCheap = true
		}
	}
	if gaveDear && !gaveCheap {
		t.Fatal("it would give away the piece it loves but not the one it does not")
	}
	if !gaveCheap {
		t.Fatal("it would not give away a piece it does not want at any price")
	}
}
