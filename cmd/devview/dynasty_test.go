package main

import (
	"testing"

	"github.com/toku463ne/cld_human_behaviour/engine"
)

// aDynastyGame is a game with the mode on, a world that has been running long
// enough to have lines in it, and somebody taken over.
func aDynastyGame(t *testing.T, goals []int) (*game, int) {
	t.Helper()
	cfg := engine.DefaultConfig()
	w := engine.NewWorld(cfg)
	for i := 0; i < 3000; i++ {
		w.Step()
	}
	g := &game{world: w, speed: normalSpeed, effort: 1.0, padKey: noKey}
	g.dyn = &dynasty{
		settle: engine.NewSettlementTracker(engine.DefaultSettleWindow, engine.DefaultSettleShare),
	}
	var who int
	for _, a := range w.Agents() {
		if a.Alive && a.Lineage != 0 {
			who = a.ID
			break
		}
	}
	if who == 0 {
		t.Fatal("nobody alive with a line")
	}
	g.selectAgent(who)
	g.toggleControl()
	// The map has no goal blocks of its own, so the test names some: the
	// engine never reads them and neither does anything but the game.
	g.dyn.goals, g.dyn.over, g.dyn.why = goals, false, ""
	return g, who
}

func TestTheDynastyPlaysForTheLineItWasGiven(t *testing.T) {
	g, who := aDynastyGame(t, []int{0, 1, 2})
	a, _ := g.world.AgentByID(who)
	if g.dyn.line != a.Lineage {
		t.Fatalf("playing for line %d, want the body's own %d", g.dyn.line, a.Lineage)
	}
	// And it never changes hands: the thing being played is a line.
	before := g.dyn.line
	g.startDynasty()
	if g.dyn.line != before {
		t.Fatalf("the line changed to %d", g.dyn.line)
	}
}

func TestAMapWithNoGoalBlocksIsNotAGame(t *testing.T) {
	cfg := engine.DefaultConfig()
	w := engine.NewWorld(cfg)
	for i := 0; i < 2000; i++ {
		w.Step()
	}
	g := &game{world: w, speed: normalSpeed, effort: 1.0, padKey: noKey}
	g.dyn = &dynasty{settle: engine.NewSettlementTracker(engine.DefaultSettleWindow, engine.DefaultSettleShare)}
	for _, a := range w.Agents() {
		if a.Alive && a.Lineage != 0 {
			g.selectAgent(a.ID)
			break
		}
	}
	g.toggleControl()
	if !g.dyn.over {
		t.Fatalf("a map with nothing marked was accepted as a game")
	}
}

func TestADeathCostsTenYears(t *testing.T) {
	g, who := aDynastyGame(t, []int{0, 1, 2})
	at := g.world.Tick()
	g.beginTheWait(who)

	if g.dyn.waiting != dynastyWaitTicks {
		t.Fatalf("waiting %d ticks, want %d", g.dyn.waiting, dynastyWaitTicks)
	}
	if g.played != 0 || g.play != playOff {
		t.Fatalf("somebody is still at the wheel during the wait (played %d, mode %v)", g.played, g.play)
	}
	// The world runs through it, and the menu only comes up at the end.
	for i := 0; i < 200 && g.dyn.waiting > 0; i++ {
		g.runTheWait()
		if g.dyn.waiting > 0 && len(g.dyn.picks) > 0 {
			t.Fatalf("the menu came up with %d ticks still to run", g.dyn.waiting)
		}
	}
	if g.dyn.waiting != 0 {
		t.Fatalf("the wait did not finish: %d left", g.dyn.waiting)
	}
	if ran := g.world.Tick() - at; ran < dynastyWaitTicks {
		t.Fatalf("the world advanced %d ticks over a %d tick wait", ran, dynastyWaitTicks)
	}
	if !g.dyn.over && len(g.dyn.picks) == 0 {
		t.Fatalf("neither a menu nor an ending came out of the wait")
	}
}

func TestTheMenuIsOnePerBlockAndGoalBlocksComeFirst(t *testing.T) {
	g, _ := aDynastyGame(t, []int{0, 1, 2})
	picks := g.transferPicks()
	if len(picks) == 0 {
		t.Skip("this seed's line holds no block by now")
	}
	seen := map[int]bool{}
	lastGoal := true
	for _, p := range picks {
		if seen[p.region] {
			t.Fatalf("block %d offered twice", p.region)
		}
		seen[p.region] = true
		if p.goal && !lastGoal {
			t.Fatalf("a goal block came after a plain one")
		}
		lastGoal = p.goal
		a, alive := g.world.AgentByID(p.id)
		if !alive || a.Lineage != g.dyn.line {
			t.Fatalf("block %d offered #%d, who is not a living carrier", p.region, p.id)
		}
		if g.world.RegionAt(a.X, a.Y) != p.region {
			t.Fatalf("#%d is offered for block %d and stands in %d", p.id, p.region, g.world.RegionAt(a.X, a.Y))
		}
	}
	if len(picks) > len(choiceKeys) {
		t.Fatalf("%d picks offered and only %d keys to press them with", len(picks), len(choiceKeys))
	}
}

func TestALineWithNobodyAnywhereLosesTheRun(t *testing.T) {
	g, who := aDynastyGame(t, []int{0, 1, 2})
	// Nothing carries the line any more.
	g.dyn.line = 60000
	g.beginTheWait(who)
	for g.dyn.waiting > 0 {
		g.runTheWait()
	}
	if !g.dyn.over {
		t.Fatalf("the run went on with nobody left to carry the line")
	}
	if len(g.dyn.picks) != 0 {
		t.Fatalf("a menu was offered: %+v", g.dyn.picks)
	}
}

func TestAReadingIsTakenOnTheClockAndNotOnTheFrame(t *testing.T) {
	// Called every frame with the world standing still, it must take one
	// reading and not one per frame: a window filled with copies of one
	// tick is a body that looks settled wherever it is standing.
	g, _ := aDynastyGame(t, []int{0})
	for i := 0; i < 50; i++ {
		g.readSettlement()
	}
	if g.dyn.reads != 1 {
		t.Fatalf("%d readings taken off one tick, want 1", g.dyn.reads)
	}
	// And once the clock has moved a step on, the next one is due.
	for i := 0; i < engine.DefaultSettleStep; i++ {
		g.world.Step()
	}
	g.readSettlement()
	if g.dyn.reads != 2 {
		t.Fatalf("%d readings after a full step, want 2", g.dyn.reads)
	}
	// Several ticks between frames must not skip it either.
	at := g.dyn.reads
	for i := 0; i < engine.DefaultSettleStep*3; i++ {
		g.world.Step()
	}
	g.readSettlement()
	if g.dyn.reads != at+1 {
		t.Fatalf("%d readings after three steps' worth in one frame, want %d", g.dyn.reads, at+1)
	}
}

func TestStandingInAGoalBlockIsNotSettlingInIt(t *testing.T) {
	// The whole reason the win is written on settling: standing in several
	// blocks at once is something a line does without being asked.
	g, _ := aDynastyGame(t, nil)
	standing := map[int]bool{}
	for _, a := range g.world.Agents() {
		if a.Alive && a.Lineage == g.dyn.line {
			standing[g.world.RegionAt(a.X, a.Y)] = true
		}
	}
	if len(standing) == 0 {
		t.Skip("the line stands nowhere")
	}
	for r := range standing {
		g.dyn.goals = append(g.dyn.goals, r)
	}
	// No readings taken, so nobody is settled anywhere yet.
	g.watchDynasty()
	if g.dyn.over {
		t.Fatalf("standing in the goal blocks won the run without anybody living there")
	}
}

// Villages (2026-09-22).

// aVillageGame is a dynasty game whose played body has money in its hands and
// one goal block: the block it is standing in.
func aVillageGame(t *testing.T, coins int) (*game, int, int) {
	t.Helper()
	g, who := aDynastyGame(t, nil)
	a, _ := g.world.AgentByID(who)
	region := g.world.RegionAt(a.X, a.Y)
	g.dyn.goals = []int{region}
	g.dyn.villages = map[int]*village{}
	for i := 0; i < coins; i++ {
		g.world.GiveItem(who, engine.FoodCoin)
	}
	return g, who, region
}

func TestAVillageCostsTheCountrysPrice(t *testing.T) {
	g, who, region := aVillageGame(t, villagePrice-1)
	g.foundVillage()
	if g.dyn.villages[region] != nil {
		t.Fatal("a village went up without the money for it")
	}
	g.world.GiveItem(who, engine.FoodCoin)
	g.foundVillage()
	v := g.dyn.villages[region]
	if v == nil {
		t.Fatal("the money was there and no village went up")
	}
	if n := g.coinsHeld(who); n != 0 {
		t.Fatalf("%d coins still in hand after paying %d", n, villagePrice)
	}
	// The money is not burned: it is lying where the village stands.
	coins := 0
	for _, f := range g.world.Foods() {
		if f.Kind == engine.FoodCoin {
			coins++
		}
	}
	if coins < villagePrice {
		t.Fatalf("%d coins on the ground, want at least the %d that were paid", coins, villagePrice)
	}
	// And the village is not the player's house: what it hands out is a line
	// of its own, which is the whole reason to marry into it.
	if v.line == g.dyn.line {
		t.Fatalf("the village hands out the player's own line %d", v.line)
	}
}

func TestOnlyAGoalBlockTakesAVillage(t *testing.T) {
	g, _, region := aVillageGame(t, villagePrice)
	g.dyn.goals = []int{region + 1} // anywhere but here
	g.foundVillage()
	if len(g.dyn.villages) != 0 {
		t.Fatal("a village went up in a block the map asks nothing of")
	}
}

func TestABlockIsWonWhenTheHouseHasLivedThereAYear(t *testing.T) {
	g, who, region := aVillageGame(t, villagePrice)
	g.foundVillage()
	v := g.dyn.villages[region]
	if v == nil {
		t.Fatal("no village")
	}
	// The player standing in it themselves is not a house living there.
	g.watchVillages(villageHoldTicks)
	if v.held != 0 || v.done {
		t.Fatalf("the played body alone held the block (held %d)", v.held)
	}
	// Somebody else of the house, standing in the block, is.
	a, _ := g.world.AgentByID(who)
	heir := 0
	for _, o := range g.world.Agents() {
		if o.Alive && o.ID != who && g.world.RegionAt(o.X, o.Y) == region {
			heir = o.ID
			break
		}
	}
	if heir == 0 {
		t.Skip("nobody else is standing in this block")
	}
	g.world.SetLineage(heir, g.dyn.line)
	g.watchVillages(villageHoldTicks - 1)
	if v.done {
		t.Fatal("the block was won a tick early")
	}
	g.watchVillages(1)
	if !v.done {
		t.Fatalf("a year of the house living there did not win the block (held %d)", v.held)
	}
	_ = a
}

func TestTheYearStartsAgainIfTheHouseLeaves(t *testing.T) {
	g, _, region := aVillageGame(t, villagePrice)
	g.foundVillage()
	v := g.dyn.villages[region]
	heir := 0
	for _, o := range g.world.Agents() {
		if o.Alive && o.ID != g.played && g.world.RegionAt(o.X, o.Y) == region {
			heir = o.ID
			break
		}
	}
	if heir == 0 {
		t.Skip("nobody else is standing in this block")
	}
	g.world.SetLineage(heir, g.dyn.line)
	g.watchVillages(200)
	if v.held != 200 {
		t.Fatalf("held %d after 200 ticks", v.held)
	}
	g.world.SetLineage(heir, g.dyn.line+1000) // no longer of the house
	g.watchVillages(10)
	if v.held != 0 {
		t.Fatalf("held %d with nobody of the house there", v.held)
	}
}

// A child of the played body belongs to the player's house, whichever parent
// the played body is. The engine hands a line down from the mother; a male
// player would otherwise be childless by the only tag the game can count.
func TestTheChildrenOfThePlayedBodyJoinItsHouse(t *testing.T) {
	g, _ := aDynastyGame(t, []int{0})
	g.dyn.villages = map[int]*village{}
	// Somebody in this world who has a parent still alive: that parent is
	// the body we play, so that the child is one of the player's.
	child, parent := 0, 0
	for _, a := range g.world.Agents() {
		if !a.Alive {
			continue
		}
		for _, p := range a.ParentIDs {
			if p == 0 {
				continue
			}
			if _, alive := g.world.AgentByID(p); alive {
				child, parent = a.ID, p
			}
		}
		if child != 0 {
			break
		}
	}
	if child == 0 {
		t.Skip("nobody in this world has a living parent")
	}
	g.played = parent
	g.dyn.line = 40000 // a house nobody is born into, so the change is visible
	g.adoptChildren()
	if a, _ := g.world.AgentByID(child); a.Lineage != 40000 {
		t.Fatalf("a child of the played body is of house %d, want the player's", a.Lineage)
	}
	// And nobody but that body's children is taken in - several of them is
	// right, anybody else is not.
	for _, a := range g.world.Agents() {
		if !a.Alive || a.Lineage != 40000 {
			continue
		}
		if a.ParentIDs[0] != parent && a.ParentIDs[1] != parent {
			t.Fatalf("#%d is of the house and is no child of the played body", a.ID)
		}
	}
}

// A world whose people have not arrived yet hands the player nobody rather
// than a beast (2026-09-23). A map that paints a nest starts empty, and a
// body with no line is a dynasty that can never begin.
func TestAWorldWithNoPeopleYetHandsOverNobody(t *testing.T) {
	cfg := engine.DefaultConfig()
	cfg.InitialPopulation = 0
	cfg.InitialEnemies = 6
	cfg.EnemyKinds = []engine.EnemyKind{{Name: "brute", Share: 1}}
	w := engine.NewWorld(cfg)
	if got := quickestBody(w); got != 0 {
		a, _ := w.AgentByID(got)
		t.Fatalf("a world of %d beasts and no people offered #%d (species %v)",
			cfg.InitialEnemies, got, a.Species)
	}
	// And once somebody is there, that is who it offers.
	cfg.HumanNests = []engine.HumanNest{{Name: "hearth", Key: 'h', Rate: 50, Life: 5000, Cap: 5}}
	cfg.HumanNestMap = []string{"h.."}
	w = engine.NewWorld(cfg)
	for i := 0; i < 200; i++ {
		w.Step()
	}
	who := quickestBody(w)
	if who == 0 {
		t.Fatal("the nest sent somebody and nobody was offered")
	}
	a, _ := w.AgentByID(who)
	if a.Species != engine.SpeciesHuman {
		t.Fatalf("#%d is not one of the people", who)
	}
}
