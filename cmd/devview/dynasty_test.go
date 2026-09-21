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

func TestSettlingEveryGoalBlockWinsIt(t *testing.T) {
	g, _ := aDynastyGame(t, nil)
	// Whatever the line is actually settled in, made the goal: the win has
	// to fire on the same reading the status line shows.
	g.dyn.goals = nil
	for i := 0; i < engine.DefaultSettleWindow; i++ {
		for k := 0; k < engine.DefaultSettleStep; k++ {
			g.world.Step()
		}
		g.readSettlement()
	}
	where := map[int]bool{}
	for _, a := range g.world.Agents() {
		if !a.Alive || a.Lineage != g.dyn.line {
			continue
		}
		if r, ok := g.dyn.settle.SettledIn(a.ID); ok {
			where[r] = true
		}
	}
	if len(where) == 0 {
		t.Skip("this seed's line has settled nowhere yet")
	}
	for r := range where {
		g.dyn.goals = append(g.dyn.goals, r)
	}
	g.watchDynasty()
	if !g.dyn.over {
		t.Fatalf("settled in all %d goal blocks and the run did not end", len(g.dyn.goals))
	}
	if g.dyn.held != len(g.dyn.goals) {
		t.Fatalf("held %d of %d", g.dyn.held, len(g.dyn.goals))
	}
}

func TestTheWinWaitsForAFullWindowOfReadings(t *testing.T) {
	// One reading makes every body "settled" where it stands, which is right
	// for the instrument and would hand a player the run at tick nought.
	g, _ := aDynastyGame(t, nil)
	g.readSettlement()
	where := map[int]bool{}
	for _, a := range g.world.Agents() {
		if !a.Alive || a.Lineage != g.dyn.line {
			continue
		}
		if r, ok := g.dyn.settle.SettledIn(a.ID); ok {
			where[r] = true
		}
	}
	if len(where) == 0 {
		t.Skip("the line is nowhere")
	}
	for r := range where {
		g.dyn.goals = append(g.dyn.goals, r)
	}
	g.watchDynasty()
	if g.dyn.over {
		t.Fatalf("won on %d readings, with %d wanted", g.dyn.reads, engine.DefaultSettleWindow)
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
