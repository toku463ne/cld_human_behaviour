package engine

import "testing"

// hideConfig is the cold half of climateConfig with a material in it: beasts
// leave skins, and a warm thing can only be made out of one.
func hideConfig() Config {
	cfg := climateConfig()
	cfg.Trinkets = true
	cfg.WardStrength = 1
	cfg.WardShare = 0 // the material decides it, not the die
	cfg.HidePerBudget = 130
	cfg.WardNeedsHide = true
	// The hands the coat was measured with (stage 80a): a finished piece
	// weighs nothing and the material weighs, which is the whole of why a
	// hunter cannot simply keep every skin it finds.
	cfg.CarrySlotted, cfg.CarrySlotsWeigh = true, true
	return cfg
}

// A beast leaves its skin where it fell, and a person leaves none: the whole
// of the stage is that the material is somewhere rather than everywhere.
func TestOnlyTheBeastsLeaveAHide(t *testing.T) {
	cfg := hideConfig()
	w := NewWorld(cfg)

	beast := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100,
		Species: SpeciesEnemy, Genome: filledGenome(50)}))
	want := int(w.hideOf(beast) / cfg.HidePerBudget)
	if want <= 0 {
		t.Fatalf("a beast of this size is worth %d hides", want)
	}
	w.kill(beast)
	if got := w.countKind(FoodHide); got != want {
		t.Fatalf("the carcass left %d hides, want %d", got, want)
	}

	person := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 200, Y: 100,
		Genome: filledGenome(50)}))
	w.kill(person)
	if got := w.countKind(FoodHide); got != want {
		t.Fatalf("a dead person left %d hides", got-want)
	}
}

// A world that asks for no material leaves none, whatever dies in it: the
// default is every world before this one.
func TestNoMaterialNoHides(t *testing.T) {
	w := NewWorld(climateConfig())
	beast := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100,
		Species: SpeciesEnemy, Genome: filledGenome(50)}))
	w.kill(beast)
	if got := w.countKind(FoodHide); got != 0 {
		t.Fatalf("a world with no material in it left %d hides", got)
	}
}

// The kind's own row says how much of a beast is worth working, and it is a
// separate figure from how much of it is worth eating.
func TestTheRowSaysHowMuchSkinASortLeaves(t *testing.T) {
	cfg := hideConfig()
	cfg.EnemyKinds = []EnemyKind{{Name: "thick", Share: 1, Homing: 1, Hide: 3}}
	w := NewWorld(cfg)
	beast := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100,
		Species: SpeciesEnemy, Genome: filledGenome(50)}))
	plain := NewWorld(hideConfig())
	same := mustAgent(t, plain, plain.addAgent(Agent{Maturity: 1, X: 100, Y: 100,
		Species: SpeciesEnemy, Genome: filledGenome(50)}))
	if w.hideOf(beast) <= plain.hideOf(same) {
		t.Fatalf("a thick-skinned sort leaves %v, an ordinary one %v",
			w.hideOf(beast), plain.hideOf(same))
	}
}

// Making something with a hide in hand makes a coat and uses the hide up;
// making something without one makes an ornament that wards nothing. No die
// is rolled either way - which is the difference between a material and
// WardShare's luck.
func TestAWarmThingIsMadeOutOfAHide(t *testing.T) {
	cfg := hideConfig()
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 380, Y: 200, Vitality: 90,
		Genome: filledGenome(50)}))
	a.carried = append(a.carried, Food{Kind: FoodHide})
	w.heldKind[FoodHide]++

	a.actionTicks = cfg.CraftTicks
	w.craft(a)
	if a.holdsHide() {
		t.Fatal("the hide is still in hand after the making")
	}
	made := a.carried[len(a.carried)-1]
	if made.Kind != FoodTrinket || made.Ward <= 0 {
		t.Fatalf("what came out of it is %v warding %v", made.Kind, made.Ward)
	}
	if w.Hides().Worked != 1 {
		t.Fatalf("the material was worked %d times", w.Hides().Worked)
	}

	// And again with nothing to work: the same making, and nothing warded.
	a.actionTicks = cfg.CraftTicks
	w.craft(a)
	if second := a.carried[len(a.carried)-1]; second.Ward != 0 {
		t.Fatalf("a piece made out of nothing wards %v", second.Ward)
	}
}

// What a hide is worth is what its holder could make of it, and that differs
// between two bodies standing in different weather - the first thing in this
// world whose price the buyer and the seller disagree about for a reason
// either of them could point at.
func TestAHideIsWorthTheCoatItWouldMake(t *testing.T) {
	cfg := hideConfig()
	w := NewWorld(cfg)
	warmID := w.addAgent(Agent{Maturity: 1, X: 40, Y: 200, Vitality: 90,
		Genome: filledGenome(50)})
	coldID := w.addAgent(Agent{Maturity: 1, X: 380, Y: 200, Vitality: 90,
		Genome: filledGenome(50)})
	// Looked up after the last of them is in: the list they live in moves
	// when it grows, so a pointer taken before then is a pointer into the
	// world as it was.
	warm, cold := mustAgent(t, w, warmID), mustAgent(t, w, coldID)

	// What an ornament is still worth to a body that may not be there for it
	// is written once a tick and read by everything that prices one (stage
	// 84), so it has to have been written before any of this is asked.
	w.priceHands()
	sw, sc := w.selfView(warm), w.selfView(cold)
	inWarm, inCold := hideWorth(&cfg, &sw), hideWorth(&cfg, &sc)
	if inWarm <= 0 {
		t.Fatalf("a hide is worth %v to a body that would make an ornament of it", inWarm)
	}
	if inCold <= inWarm {
		t.Fatalf("a hide is worth %v in the cold and %v in the warm", inCold, inWarm)
	}

	// And nothing at all where the world does not ask for a material: then it
	// is a thing lying about that nobody can do anything with.
	plain := cfg
	plain.WardNeedsHide = false
	if got := hideWorth(&plain, &sc); got != 0 {
		t.Fatalf("a hide is worth %v in a world that does not ask for one", got)
	}
}

// A body that is already wearing one gains nothing by a second, so the skin
// in its hand is worth the ornament and no more. That is what leaves a
// hunter with something it would rather have a coin for.
func TestASecondCoatIsWorthNothingToItsOwner(t *testing.T) {
	cfg := hideConfig()
	w := NewWorld(cfg)
	bareID := w.addAgent(Agent{Maturity: 1, X: 380, Y: 200, Vitality: 90,
		Genome: filledGenome(50)})
	coatedID := w.addAgent(Agent{Maturity: 1, X: 380, Y: 220, Vitality: 90,
		Genome: filledGenome(50)})
	bare, coated := mustAgent(t, w, bareID), mustAgent(t, w, coatedID)
	coated.carried = append(coated.carried, Food{Kind: FoodTrinket, Made: 1,
		Ward: 1, Wards: WeatherChill})
	w.heldKind[FoodTrinket]++

	if got := w.wardGap(coated); got != 0 {
		t.Fatalf("a second coat would take off another %v", got)
	}
	if got := w.wardGap(bare); got <= 0 {
		t.Fatalf("a first coat would take off %v", got)
	}
	w.priceHands()
	sb, sc := w.selfView(bare), w.selfView(coated)
	if hideWorth(&cfg, &sc) >= hideWorth(&cfg, &sb) {
		t.Fatalf("the hide is worth %v to a coated body and %v to a bare one",
			hideWorth(&cfg, &sc), hideWorth(&cfg, &sb))
	}
}

// A skin lying about is something to walk over for, and only where the world
// asks for a material: the option is the stone's, with the want changed.
func TestAHideOnTheGroundIsWorthWalkingTo(t *testing.T) {
	cfg := hideConfig()
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 360, Y: 200, Vitality: 90,
		Genome: filledGenome(50)}))
	w.putFood(Food{X: 380, Y: 200, Kind: FoodHide})

	w.priceHands()
	p := w.perceive(a)
	if len(p.Hides) != 1 {
		t.Fatalf("the body can see %d hides", len(p.Hides))
	}
	id := p.Hides[0].ID
	c := &AIController{}
	c.Decide(p)
	scored := false
	for _, o := range c.opts {
		if o.action.Kind == ActTake && o.action.TargetID == id {
			scored = true
		}
	}
	if !scored {
		t.Fatal("nothing was scored for a skin lying in front of it")
	}

	// And nothing at all where the world does not ask for a material: the
	// skin is in sight, and there is no option to go and get it.
	w.cfg.WardNeedsHide = false
	p = w.perceive(a)
	c.Decide(p)
	for _, o := range c.opts {
		if o.action.Kind == ActTake && o.action.TargetID == id {
			t.Fatal("a body walked over for a skin it could do nothing with")
		}
	}
}

// Hides do not count against what the world is allowed to grow. The total the
// world keeps fixed is a total of food (stage 15a), and a skin is not food -
// the same line the stones, the money, the books and the ornaments are on.
func TestHidesDoNotCrowdOutThePlants(t *testing.T) {
	cfg := hideConfig()
	cfg.MaxFoodItems = 3
	w := NewWorld(cfg)
	for i := 0; i < 10; i++ {
		w.putFood(Food{X: float64(100 + i), Y: 100, Kind: FoodHide})
	}
	before := w.countKind(FoodPlant)
	w.cfg.FoodSpawnRate = 1
	for i := 0; i < 200; i++ {
		w.spawnFood()
	}
	if w.countKind(FoodPlant) <= before {
		t.Fatal("a world full of skins grew nothing at all")
	}
}
