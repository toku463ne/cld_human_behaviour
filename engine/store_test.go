package engine

import "testing"

// storeConfig is a still world with one cache in it and hands to fill it from.
func storeConfig() Config {
	cfg := quietConfig()
	cfg.CarryCapacity = 1
	cfg.StoreFindLooks = 0 // nobody stumbles on anything in a test
	return cfg
}

// The whole of the rule: what goes into a cache is in the world at that spot,
// and only those who know the place can see it.
func TestWhatIsInACacheIsInTheWorldAndHiddenFromStrangers(t *testing.T) {
	w := NewWorld(storeConfig())
	at, err := w.SetStore(100, 100)
	if err != nil {
		t.Fatal(err)
	}
	keeperID := holding(t, w, 104, 100)
	keeper := mustAgent(t, w, keeperID)
	w.learnStore(keeper, at, 10)

	before := w.countKind(FoodPlant)
	if !w.putInStore(keeper, at) {
		t.Fatal("the keeper could not put anything in a cache it knows")
	}
	if keeper.CarriedCount() != 0 {
		t.Fatal("the item is still in the hand")
	}
	if got := w.countKind(FoodPlant); got != before {
		t.Fatalf("the world holds %d plants and held %d: a cache made or lost one", got, before)
	}
	if got := w.Stored().Held; got != 1 {
		t.Fatalf("%d items in the caches", got)
	}

	// The keeper sees it; a body that arrives afterwards, and so did not see
	// it happen, does not - even standing in the same spot.
	strangerID := w.addAgent(Agent{Maturity: 1, X: 104, Y: 100, Vitality: 90,
		Hunger: 60, Genome: genomeOf(50, 50, 50)})
	if got := len(w.perceive(keeper).Foods); got != 1 {
		t.Fatalf("the keeper sees %d items of food", got)
	}
	if got := len(w.perceive(mustAgent(t, w, strangerID)).Foods); got != 0 {
		t.Fatalf("somebody who does not know the cache sees %d items in it", got)
	}
	// And it is in the same list as everything else lying about, at the spot.
	found := false
	for _, f := range w.Foods() {
		if f.Store == at+1 && f.X == 100 && f.Y == 100 {
			found = true
		}
	}
	if !found {
		t.Fatal("what was put in the cache is not in the world at the cache")
	}
}

// A cache holds what it holds and no more.
func TestACacheFillsUp(t *testing.T) {
	cfg := storeConfig()
	cfg.StoreCapacity = 2
	w := NewWorld(cfg)
	at, _ := w.SetStore(100, 100)
	for i := 0; i < 3; i++ {
		id := holding(t, w, 104, 100)
		a := mustAgent(t, w, id)
		w.learnStore(a, at, 10)
		put := w.putInStore(a, at)
		if i < 2 && !put {
			t.Fatalf("item %d would not go in", i)
		}
		if i == 2 && put {
			t.Fatal("a third item went into a cache that holds two")
		}
	}
	if got := w.Stored().Held; got != 2 {
		t.Fatalf("%d items in a cache that holds two", got)
	}
}

// Nothing can be put in a place the body does not know, and nothing about
// standing on one teaches it.
func TestACacheIsNotALandmark(t *testing.T) {
	w := NewWorld(storeConfig())
	at, _ := w.SetStore(100, 100)
	id := holding(t, w, 100, 100)
	a := mustAgent(t, w, id)

	// Standing on it, looking straight at it, and none the wiser.
	for i := 0; i < 20; i++ {
		w.perceive(a)
	}
	if w.knowsStore(a, at) {
		t.Fatal("a body learned a cache by standing on it")
	}
	if w.putInStore(a, at) {
		t.Fatal("a body put something in a cache it cannot find")
	}
	if got := len(w.perceive(a).Stores); got != 0 {
		t.Fatalf("a body that knows no cache has %d in its perception", got)
	}
}

// The control: in the arm where everybody knows every cache, the same body
// knows it without ever having heard of it. It leaves the storing and takes
// away the knowing.
func TestEverybodyKnowsEveryCacheInTheControlArm(t *testing.T) {
	cfg := storeConfig()
	cfg.StoresKnownToAll = true
	w := NewWorld(cfg)
	at, _ := w.SetStore(100, 100)
	id := holding(t, w, 104, 100)
	a := mustAgent(t, w, id)
	if !w.knowsStore(a, at) {
		t.Fatal("somebody does not know a cache in the arm where everybody does")
	}
	if !w.putInStore(a, at) {
		t.Fatal("nothing could be put in a cache everybody knows")
	}
}

// The four ways of coming to know a place, and that the three the plan named
// cannot start on their own.
func TestKnowingAPlaceComesDownFourPaths(t *testing.T) {
	t.Run("finding it", func(t *testing.T) {
		cfg := storeConfig()
		cfg.StoreFindLooks = 1
		w := NewWorld(cfg)
		at, _ := w.SetStore(100, 100)
		id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
			Genome: genomeOf(50, 50, 50)})
		w.perceive(mustAgent(t, w, id))
		if !w.knowsStore(mustAgent(t, w, id), at) {
			t.Fatal("a body looking at a cache with the chance at one did not notice it")
		}
	})

	t.Run("inheriting it", func(t *testing.T) {
		w := NewWorld(storeConfig())
		at, _ := w.SetStore(100, 100)
		parent := &Agent{}
		child := &Agent{}
		w.learnStore(parent, at, 10)
		w.inheritStores(child, parent)
		if !w.knowsStore(child, at) {
			t.Fatal("a child did not take its parent's cache with it")
		}
	})

	t.Run("seeing it used", func(t *testing.T) {
		w := NewWorld(storeConfig())
		at, _ := w.SetStore(100, 100)
		keeperID := holding(t, w, 104, 100)
		watcherID := w.addAgent(Agent{Maturity: 1, X: 120, Y: 100, Vitality: 90,
			Genome: genomeOf(50, 50, 50)})
		keeper := mustAgent(t, w, keeperID)
		w.learnStore(keeper, at, 10)
		w.putInStore(keeper, at)
		if !w.knowsStore(mustAgent(t, w, watcherID), at) {
			t.Fatal("somebody watching a cache being used learned nothing")
		}
	})

	t.Run("hearing it cried", func(t *testing.T) {
		cfg := storeConfig()
		cfg.OfferTicks = 30
		w := NewWorld(cfg)
		at, _ := w.SetStore(100, 100)
		crierID := holding(t, w, 104, 100)
		listenerID := w.addAgent(Agent{Maturity: 1, X: 120, Y: 100, Vitality: 90,
			Genome: genomeOf(50, 50, 50)})
		crier := mustAgent(t, w, crierID)
		w.learnStore(crier, at, 10)
		w.tellStores(crier)
		if !w.knowsStore(mustAgent(t, w, listenerID), at) {
			t.Fatal("somebody hearing a cry learned nothing about where the goods are")
		}
	})
}

// A place fades if the body stops having anything to do with it: knowledge of
// a cache is not a right, it is a habit.
func TestAPlaceIsForgottenIfNobodyGoesThere(t *testing.T) {
	cfg := storeConfig()
	cfg.RegionForgetPerTick = 0.01
	w := NewWorld(cfg)
	at, _ := w.SetStore(100, 100)
	id := w.addAgent(Agent{Maturity: 1, X: 400, Y: 400, Vitality: 90,
		Genome: genomeOf(50, 50, 50)})
	a := mustAgent(t, w, id)
	w.learnStore(a, at, 3)
	if !w.knowsStore(a, at) {
		t.Fatal("it did not learn the place at all")
	}
	w.tick += 100000
	if w.knowsStore(a, at) {
		t.Fatal("a place nobody has been near for a lifetime is still known")
	}
}

// Taking a cache away leaves what was in it lying in the open, and does not
// take it out of the world.
func TestTakingACacheAwayLeavesWhatWasInIt(t *testing.T) {
	w := NewWorld(storeConfig())
	at, _ := w.SetStore(100, 100)
	id := holding(t, w, 104, 100)
	a := mustAgent(t, w, id)
	w.learnStore(a, at, 10)
	w.putInStore(a, at)

	before := w.countKind(FoodPlant)
	if err := w.RemoveStore(at); err != nil {
		t.Fatal(err)
	}
	if got := w.countKind(FoodPlant); got != before {
		t.Fatalf("removing a cache left %d plants, was %d", got, before)
	}
	for _, f := range w.Foods() {
		if f.Store != 0 {
			t.Fatal("an item is still in a cache that is gone")
		}
	}
	if got := len(w.Stores()); got != 0 {
		t.Fatalf("%d caches left", got)
	}
}

// A world holds only so many, which is what makes the exemption from what a
// body can remember about people (#41) a fair trade.
func TestAWorldHoldsOnlySoManyCaches(t *testing.T) {
	cfg := storeConfig()
	cfg.MaxStores = 2
	w := NewWorld(cfg)
	if _, err := w.SetStore(10, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := w.SetStore(20, 20); err != nil {
		t.Fatal(err)
	}
	if _, err := w.SetStore(30, 30); err == nil {
		t.Fatal("a third cache went into a world that holds two")
	}
}

// A body with something in its hand and a cache it knows is offered the option;
// one with empty hands, or one that knows nowhere, is not.
func TestPuttingSomethingAwayIsOneOptionAmongTheRest(t *testing.T) {
	// A world where hunger is a thing that happens: keeping something for
	// later is worth nothing at all in a world where nobody ever runs short,
	// which is the still world the other tests here use.
	cfg := storeConfig()
	cfg.HungerRate, cfg.StarveRate = DefaultConfig().HungerRate, DefaultConfig().StarveRate
	w := NewWorld(cfg)
	at, _ := w.SetStore(140, 100)
	keeperID := holding(t, w, 100, 100)
	// And somebody else after the same plants. What a body kept for later is
	// worth is read off how contested the patch feels (FoodScarcity, the same
	// reading stage 40 uses for picking something up), so a body alone in an
	// empty world reckons a cache worth nothing - which is right, and is why
	// there is a rival here.
	w.addAgent(Agent{Maturity: 1, X: 112, Y: 100, Vitality: 90, Genome: genomeOf(50, 50, 50)})
	w.addFood(108, 100)
	keeper := mustAgent(t, w, keeperID)
	// And a body with something to lose. What a thing kept for later is worth
	// is the difference it would make at the moment this body runs short, so
	// one in no danger at all reckons it worth nothing - which is stage 40's
	// rule, not this stage's.
	keeper.Vitality, keeper.Hunger = 18, 50
	if stores(t, w, keeperID) {
		t.Fatal("a body that knows no cache was offered a way of using one")
	}
	w.learnStore(keeper, at, 10)
	if !stores(t, w, keeperID) {
		t.Fatal("a body with a full hand and a cache it knows was offered no way of using it")
	}

	empty := w.addAgent(Agent{Maturity: 1, X: 102, Y: 100, Vitality: 18,
		Hunger: 50, Genome: genomeOf(50, 50, 50)})
	w.learnStore(mustAgent(t, w, empty), at, 10)
	if stores(t, w, empty) {
		t.Fatal("a body with empty hands was offered a way of putting something away")
	}
}

// stores reports whether the comparison this body would run contains putting
// something away.
func stores(t *testing.T, w *World, id int) bool {
	t.Helper()
	c, _ := scored(t, w, id)
	for _, o := range c.opts {
		if o.action.Kind == ActStore {
			return true
		}
	}
	return false
}
