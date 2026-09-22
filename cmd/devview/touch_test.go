package main

import (
	"testing"
	"time"

	"github.com/toku463ne/cld_human_behaviour/engine"
)

// clock is the frames a press is watched over: a start, and a reading every
// so often afterwards. A gesture is timing, so the only way to test one is to
// drive it with a clock of one's own.
var t0 = time.Unix(1700000000, 0)

// press holds the pointer down at one place until the given moment and
// returns whatever it reported along the way, a frame every 16 ms.
func press(p *pointer, until time.Duration, x, y int) (gesture, int, int) {
	got, gx, gy := gestureNone, 0, 0
	for d := time.Duration(0); d <= until; d += 16 * time.Millisecond {
		if g, ex, ey := p.update(true, x, y, t0.Add(d)); g != gestureNone {
			got, gx, gy = g, ex, ey
		}
	}
	return got, gx, gy
}

// A short press is a tap, and it is reported when the finger comes up rather
// than when it goes down: until it lifts, it might still become a hold.
func TestAShortPressIsATapOnRelease(t *testing.T) {
	var p pointer
	if g, _, _ := press(&p, holdDelay/2, 40, 50); g != gestureNone {
		t.Fatalf("a press that has not been released reported %v", g)
	}
	g, x, y := p.update(false, 0, 0, t0.Add(holdDelay/2))
	if g != gestureTap || x != 40 || y != 50 {
		t.Fatalf("release gave %v at %d,%d, want a tap at 40,50", g, x, y)
	}
	if g, _, _ := p.update(false, 0, 0, t0.Add(holdDelay)); g != gestureNone {
		t.Fatalf("the same release reported %v a second time", g)
	}
}

// A long one is a hold, it fires while the finger is still down - a person
// holding a finger down is waiting for the menu, not for permission to lift
// it - and lifting afterwards is not also a tap.
func TestALongPressIsAHoldAndNotAlsoATap(t *testing.T) {
	var p pointer
	g, x, y := press(&p, holdDelay+50*time.Millisecond, 11, 12)
	if g != gestureHold || x != 11 || y != 12 {
		t.Fatalf("holding gave %v at %d,%d, want a hold at 11,12", g, x, y)
	}
	if g, _, _ := press(&p, 3*holdDelay, 11, 12); g != gestureNone {
		t.Fatal("holding on reported a second hold")
	}
	if g, _, _ := p.update(false, 0, 0, t0.Add(3*holdDelay)); g != gestureNone {
		t.Fatalf("lifting after a hold reported %v", g)
	}
}

// And a press that is long enough is a hold even if no frame landed inside
// it. A telephone drawing five frames a second is a telephone that sees the
// press begin and end and nothing in between; a menu that never opens there
// is a menu that does not work on the machines this is being built for.
func TestALongPressWithNoFrameInsideItIsStillAHold(t *testing.T) {
	var p pointer
	p.update(true, 70, 80, t0)
	g, x, y := p.update(false, 0, 0, t0.Add(holdDelay+time.Millisecond))
	if g != gestureHold || x != 70 || y != 80 {
		t.Fatalf("got %v at %d,%d, want a hold at 70,80", g, x, y)
	}
}

// A press that wanders is neither. Dragging the mouse across the world is not
// an order, and a finger that slides is somebody changing their mind.
func TestADragIsNeitherATapNorAHold(t *testing.T) {
	var p pointer
	p.update(true, 100, 100, t0)
	if g, _, _ := p.update(true, 100+dragSlop+1, 100, t0); g != gestureNone {
		t.Fatalf("moving reported %v", g)
	}
	if g, _, _ := press(&p, 2*holdDelay, 200, 100); g != gestureNone {
		t.Fatal("a press that wandered still became a hold")
	}
	if g, _, _ := p.update(false, 0, 0, t0.Add(3*holdDelay)); g != gestureNone {
		t.Fatal("a press that wandered still became a tap")
	}
}

// What is under the finger is where it landed, not where it left: a finger
// rolls a few pixels on the way up.
func TestTheGestureIsReportedWhereThePressBegan(t *testing.T) {
	var p pointer
	p.update(true, 300, 200, t0)
	p.update(true, 303, 202, t0.Add(50*time.Millisecond))
	g, x, y := p.update(false, 0, 0, t0.Add(100*time.Millisecond))
	if g != gestureTap || x != 300 || y != 200 {
		t.Fatalf("got %v at %d,%d, want a tap at 300,200", g, x, y)
	}
}

// The menu picks by where it is drawn: the title is not an item, and a tap
// outside it is a miss. Closing on a miss is the only way out without a
// keyboard, so it has to be a miss rather than the nearest item.
func TestTheMenuPicksWhatIsUnderTheTap(t *testing.T) {
	m := &menu{title: "do:", items: []menuItem{{label: "a"}, {label: "b"}}, x: 100, y: 100}
	if got := m.at(110, 104); got != -1 {
		t.Fatalf("the title picked item %d", got)
	}
	first := m.y + menuPad + menuLine + 2
	if got := m.at(110, first); got != 0 {
		t.Fatalf("the first line picked %d", got)
	}
	if got := m.at(110, first+menuLine); got != 1 {
		t.Fatalf("the second line picked %d", got)
	}
	if got := m.at(110, first+menuLine*3); got != -1 {
		t.Fatalf("below the last line picked %d", got)
	}
	if got := m.at(m.x+menuWidth+5, first); got != -1 {
		t.Fatalf("outside to the right picked %d", got)
	}
}

// And it is kept on the screen, whatever corner it was opened in. A menu half
// off the edge is one a finger cannot reach.
func TestTheMenuStaysOnTheScreen(t *testing.T) {
	g := &game{padKey: noKey}
	g.openMenu("do:", worldWidth-4, screenHeight-4,
		menuItem{label: "a"}, menuItem{label: "b"}, menuItem{label: "c"})
	if g.menu == nil {
		t.Fatal("no menu")
	}
	if g.menu.x+menuWidth > worldWidth || g.menu.y+g.menu.height() > screenHeight {
		t.Fatalf("the menu runs off the screen at %d,%d", g.menu.x, g.menu.y)
	}
	// And an empty one is not a menu at all: a hold over nothing should leave
	// the screen alone rather than put up a box with nothing in it.
	g.menu = nil
	g.openMenu("do:", 10, 10)
	if g.menu != nil {
		t.Fatal("an empty menu was opened")
	}
}

// aDrivenWorld is one body driven by a person, with something to aim at.
func aDrivenWorld(t *testing.T) (*game, int) {
	t.Helper()
	cfg := engine.DefaultConfig()
	cfg.Seed = 7
	w := engine.NewWorld(cfg)
	for i := 0; i < 200; i++ {
		w.Step()
	}
	agents := w.Agents()
	if len(agents) == 0 {
		t.Fatal("nobody alive")
	}
	me := agents[0].ID
	h := engine.NewHumanController()
	if !w.SetController(me, h) {
		t.Fatal("could not take the reins")
	}
	w.RequestDecision(me)
	w.Step()
	g := &game{world: w, play: playDriven, played: me, human: h, effort: 0.6, padKey: noKey}
	return g, me
}

// The second tap acts, and only on the same thing: tapping somewhere else is
// a new aim, because a player who changes their mind has not ordered anything.
func TestASecondTapOnTheSameThingOrders(t *testing.T) {
	g, me := aDrivenWorld(t)
	self, _ := g.world.AgentByID(me)

	// Two taps on the same patch of ground, a step to the east of the body.
	x, y := g.onScreen(self.X+20, self.Y)
	g.tapWhileDriving(int(x), int(y))
	if g.mark.kind != markSpot {
		t.Fatalf("the first tap aimed at %v, want bare ground", g.mark.kind)
	}
	if got := g.human.Standing().Kind; got == engine.ActMove {
		t.Fatal("the first tap already gave the order")
	}
	g.tapWhileDriving(int(x), int(y))
	if got := g.human.Standing().Kind; got != engine.ActMove {
		t.Fatalf("the second tap left the order at %v, want a move", got)
	}

	// And a tap somewhere else is an aim again rather than an order.
	_ = g.setOrder(engine.Action{Kind: engine.ActRest})
	fx, fy := g.onScreen(self.X-40, self.Y+30)
	g.tapWhileDriving(int(fx), int(fy))
	if got := g.human.Standing().Kind; got != engine.ActRest {
		t.Fatalf("a tap on new ground ordered %v", got)
	}
}

// The order a second tap gives is the one the mark deserves, and what a body
// is is read from what the node can see of it rather than from the world.
func TestTheDefaultOrderFollowsWhatWasTapped(t *testing.T) {
	g, me := aDrivenWorld(t)
	v, ok := g.view()
	if !ok {
		t.Fatal("the node has not been asked anything yet")
	}

	g.mark = mark{kind: markSpot, x: 10, y: 10}
	if got := g.defaultName(); got != "walk there" {
		t.Fatalf("bare ground defaults to %q", got)
	}

	// Somebody in sight: a person is watched, a beast is attacked. Prey is
	// the node's own view of them (AgentView.Prey), so this is a thing it
	// knows without being told.
	for _, o := range v.Others {
		if o.ID == me {
			continue
		}
		g.mark = mark{kind: markAgent, id: o.ID}
		want := "watch them"
		if o.Prey {
			want = "attack it"
		}
		if got := g.defaultName(); got != want {
			t.Fatalf("#%d (prey=%v) defaults to %q, want %q", o.ID, o.Prey, got, want)
		}
		break
	}

	if len(v.Foods) > 0 {
		g.mark = mark{kind: markFood, id: v.Foods[0].ID}
		if got := g.defaultName(); got != "eat it" {
			t.Fatalf("a meal defaults to %q", got)
		}
	}
}

// A hold offers the rest of the vocabulary, and what it offers depends on
// what is aimed at: there is no courting a plant.
func TestTheHoldMenuFitsTheMark(t *testing.T) {
	g, _ := aDrivenWorld(t)
	has := func(items []menuItem, label string) bool {
		for _, it := range items {
			if it.label == label {
				return true
			}
		}
		return false
	}

	g.mark = mark{kind: markFood, id: 1}
	food := g.orderMenu()
	if !has(food, "eat") || has(food, "court") {
		t.Fatalf("the menu for a meal is %v", labels(food))
	}
	g.mark = mark{kind: markAgent, id: 2}
	body := g.orderMenu()
	if !has(body, "court") || !has(body, "attack") || has(body, "eat") {
		t.Fatalf("the menu for a body is %v", labels(body))
	}
	g.mark = mark{kind: markSpot}
	ground := g.orderMenu()
	if !has(ground, "walk here") || has(ground, "attack") {
		t.Fatalf("the menu for bare ground is %v", labels(ground))
	}
	// Three things are on every one of them, because they are about the body
	// doing the ordering rather than about what it is aimed at.
	for _, items := range [][]menuItem{food, body, ground} {
		if !has(items, "rest") || !has(items, "think again") {
			t.Fatalf("%v is missing what every mark has", labels(items))
		}
	}
}

func labels(items []menuItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.label)
	}
	return out
}

// Effort steps round the same five levels the number keys set, and wraps
// rather than sticking at the top: a menu entry that does nothing once you
// reach the end is a dead entry.
func TestEffortStepsRoundOnTheMenu(t *testing.T) {
	e := 0.2
	seen := map[float64]bool{}
	for i := 0; i < len(effortKeys); i++ {
		e = nextEffort(e)
		if e <= 0 || e > 1 {
			t.Fatalf("effort stepped to %v", e)
		}
		seen[e] = true
	}
	if len(seen) != len(effortKeys) {
		t.Fatalf("stepping visited %d levels, want %d", len(seen), len(effortKeys))
	}
}

// A tap while the world is waiting for an answer puts the answer up as a
// menu, wherever it landed. This is what makes the asked mode playable with
// one finger: until now the only way to answer was a number key.
func TestATapPutsTheSuccessionUpAsAMenu(t *testing.T) {
	g := &game{padKey: noKey, succession: &succession{from: 1, dead: true,
		picks: []successionPick{{id: 2, about: "a child"}, {id: 3, about: "another"}}}}
	if !g.openPending(400, 300) {
		t.Fatal("the succession did not come up")
	}
	if g.menu == nil || len(g.menu.items) != 2 {
		t.Fatalf("the menu is %v", g.menu)
	}
	// And the tap that opened it does not also pick from it.
	g.menu = nil
	g.tap(400, 300)
	if g.menu == nil {
		t.Fatal("a tap did not open the question")
	}
	if g.succession == nil {
		t.Fatal("opening the menu answered the question by itself")
	}
}

// Nothing pending, nothing driven: a tap is what a click always was.
func TestATapSelectsWhenNobodyIsBeingDriven(t *testing.T) {
	cfg := engine.DefaultConfig()
	cfg.Seed = 3
	w := engine.NewWorld(cfg)
	w.Step()
	g := &game{world: w, padKey: noKey}
	a := w.Agents()[0]
	x, y := g.onScreen(a.X, a.Y)
	g.tap(int(x), int(y))
	if g.selected != a.ID {
		t.Fatalf("the tap selected #%d, want #%d", g.selected, a.ID)
	}
	// And a tap on the panel is not a tap on the world.
	g.tap(worldWidth+10, 10)
	if g.selected != a.ID {
		t.Fatal("a tap on the panel changed the selection")
	}
}

// Tapping a den asks whether to call its master out, and only a yes does it
// (TODO 19). The question is the game's: the engine has no player and no
// prompt, and all it offers is Rouse.
func bossedGame(t *testing.T) *game {
	t.Helper()
	cfg := engine.DefaultConfig()
	cfg.Seed = 5
	cfg.InitialEnemies = 0
	cfg.BossBudget, cfg.EnemyHomeCost = 1.5, 2
	cfg.EnemyKinds = []engine.EnemyKind{{Name: "brute", Share: 1, Key: 'b', Homing: 1, Homely: 1}}
	cfg.EnemyKindMap = []string{"b...."}
	w := engine.NewWorld(cfg)
	return &game{world: w, padKey: noKey}
}

func TestATapOnADenAsksBeforeCallingAnythingOut(t *testing.T) {
	g := bossedGame(t)
	den := g.world.EnemyNests()[0]
	x, y := g.onScreen(den.X, den.Y)
	g.tap(int(x), int(y))
	if g.menu == nil || len(g.menu.items) != 2 {
		t.Fatalf("the menu is %+v", g.menu)
	}
	if g.world.EnemyNests()[0].Boss != 0 {
		t.Fatal("the question itself called the master out")
	}
	// No. (pickMenu closes the menu before running what was picked.)
	no := g.menu.items[1].do
	g.menu = nil
	no()
	if g.world.EnemyNests()[0].Boss != 0 {
		t.Fatal("saying no called it out anyway")
	}
	// Yes.
	g.tap(int(x), int(y))
	yes := g.menu.items[0].do
	g.menu = nil
	yes()
	if g.world.EnemyNests()[0].Boss == 0 {
		t.Fatal("saying yes called nothing out")
	}
}

func TestAWorldWithNoMastersIsNotAskedAboutThem(t *testing.T) {
	g := bossedGame(t)
	cfg := g.world.Config()
	cfg.BossBudget = 0
	g.world = engine.NewWorld(cfg)
	den := g.world.EnemyNests()[0]
	x, y := g.onScreen(den.X, den.Y)
	g.tap(int(x), int(y))
	if g.menu != nil {
		t.Fatalf("a world with no masters offered one: %+v", g.menu)
	}
}
