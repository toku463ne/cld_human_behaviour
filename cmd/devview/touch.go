package main

import (
	"fmt"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/toku463ne/cld_human_behaviour/engine"
)

// Tap and hold (TODO 9).
//
// This viewer grew up on a keyboard: twenty-odd keys, each one a word of the
// action vocabulary, and a mouse click to aim them. That is the right
// interface for reading a decision and the wrong one for a telephone, which
// is where this is going (#131) - so the whole of what a player does has to
// be reachable with one finger.
//
// Three things are built here, and the third is the one that matters.
//
//   - One press, two sources. A finger and a mouse button both come in as
//     "something is held at this point", so there is one gesture layer and
//     not two input paths that drift apart. The keys stay exactly as they
//     were: development happens on this machine, and a key is still the
//     fastest way to try a rule.
//   - A tap aims and a second tap acts. Aiming and ordering were two steps
//     with two hands (click, then press a letter); on a telephone the second
//     tap is the letter. What the default is depends on what was tapped -
//     a meal is eaten, a beast is attacked, bare ground is walked to - and
//     nothing else about the order changes: it goes through OrderHuman like
//     every other order and is refused on the same grounds.
//   - A hold opens a menu of the rest. This is the only new piece of
//     interface, and it is deliberately a list of the orders that already
//     exist rather than a new way to give one.
//
// The menu is also what makes the questions answerable by finger. The asked
// mode (stage 23), the milestone offer, the succession and a proposal are all
// "pick one of a short list", which is what the menu is - so a tap while one
// of them is standing opens it with those choices in it, and the number keys
// go on working. Four things that were keyboard-only become touchable without
// a fourth way of asking.
//
// What it does not do is show the player anything the node does not know
// (stage 19): every entry is built from the mark, and the mark can only land
// on something World.CanTargetAgent / CanTargetFood allows. A hold over a
// body outside the node's sight aims at the ground under it, which is what a
// click already did.

// How long a press has to last to be a hold, and how far it may wander and
// still count as one place. Half a second is what a telephone means by a long
// press; the slop is what a finger does while it is still.
//
// Time rather than frames, which is worth a word because frames were the
// first version and they were wrong twice over. A gesture is a thing a person
// does with their hand, so it is measured on their clock: a telephone that
// drops to twenty frames a second would otherwise want a press held for a
// second and a half, and the browser check in this same session ran at a
// handful of frames a second and never saw a hold at all.
const (
	holdDelay = 500 * time.Millisecond
	dragSlop  = 8
)

// gesture is what one press came to.
type gesture uint8

const (
	gestureNone gesture = iota
	gestureTap
	gestureHold
)

// pointer turns "something is pressed, and where" into taps and holds.
//
// It knows nothing about ebiten, which is the point: a gesture is a little
// state machine with timing in it, and the only way to be sure of one is to
// drive it frame by frame from a test.
type pointer struct {
	down  bool
	x, y  int       // where it went down
	at    time.Time // when it went down
	moved bool
	fired bool // the hold has already been reported
}

// update advances it one frame and reports what, if anything, just happened.
// The point reported is where the press began rather than where it ended: a
// finger rolls, and what was under it when it landed is what was meant.
func (p *pointer) update(down bool, x, y int, now time.Time) (gesture, int, int) {
	switch {
	case down && !p.down:
		*p = pointer{down: true, x: x, y: y, at: now}
		return gestureNone, 0, 0

	case down:
		if abs(x-p.x) > dragSlop || abs(y-p.y) > dragSlop {
			p.moved = true
		}
		if !p.fired && !p.moved && now.Sub(p.at) >= holdDelay {
			p.fired = true
			return gestureHold, p.x, p.y
		}
		return gestureNone, 0, 0

	case p.down:
		was := *p
		*p = pointer{}
		switch {
		case was.fired || was.moved:
			return gestureNone, 0, 0
		case now.Sub(was.at) >= holdDelay:
			// Long enough, and nobody looked in between: on a slow frame the
			// press can begin and end without a single frame landing inside
			// it, and that is still a long press.
			return gestureHold, was.x, was.y
		}
		return gestureTap, was.x, was.y
	}
	return gestureNone, 0, 0
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// pressedAt is where the one press is, from whichever of the two sources has
// one. The touch wins, because a machine with both is a machine being tested
// with a finger.
func (g *game) pressedAt() (bool, int, int) {
	g.touches = ebiten.AppendTouchIDs(g.touches[:0])
	if len(g.touches) > 0 {
		x, y := ebiten.TouchPosition(g.touches[0])
		return true, x, y
	}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		return true, x, y
	}
	return false, 0, 0
}

// --- the menu ---------------------------------------------------------------

// menuItem is one line of it: what it says and what it does.
type menuItem struct {
	label string
	do    func()
}

// menu is a short list of things to pick from, at a place on the screen.
//
// It is the interface's own and the engine has never heard of it, like the
// milestone offer and the succession: what it holds are closures over orders
// that already existed.
type menu struct {
	title string
	items []menuItem
	x, y  int
}

const (
	menuLine  = lineHeight + 4
	menuWidth = 190
	menuPad   = 6
)

// height is how tall the box is, title included.
func (m *menu) height() int { return menuLine*(len(m.items)+1) + menuPad }

// at returns the item under a point, or -1. The title is not one.
func (m *menu) at(x, y int) int {
	if x < m.x || x > m.x+menuWidth {
		return -1
	}
	i := (y - m.y - menuPad - menuLine) / menuLine
	if i < 0 || i >= len(m.items) {
		return -1
	}
	return i
}

// openMenu puts one on the screen, shifted so that all of it is in the world
// area rather than half of it off the edge or under the panel.
func (g *game) openMenu(title string, x, y int, items ...menuItem) {
	if len(items) == 0 {
		return
	}
	m := &menu{title: title, items: items, x: x, y: y}
	if m.x+menuWidth > worldWidth {
		m.x = worldWidth - menuWidth
	}
	if m.x < 0 {
		m.x = 0
	}
	if m.y+m.height() > screenHeight {
		m.y = screenHeight - m.height()
	}
	if m.y < 0 {
		m.y = 0
	}
	g.menu = m
}

// pickMenu runs the item under a point and closes the menu. A tap anywhere
// else closes it without doing anything, which is what a person expects and
// also the only way out with no keyboard.
func (g *game) pickMenu(x, y int) {
	m := g.menu
	g.menu = nil
	if i := m.at(x, y); i >= 0 {
		m.items[i].do()
	}
}

func (g *game) drawMenu(screen *ebiten.Image) {
	m := g.menu
	if m == nil {
		return
	}
	x, y, w, h := float32(m.x), float32(m.y), float32(menuWidth), float32(m.height())
	vector.DrawFilledRect(screen, x, y, w, h, colorPanel, false)
	vector.StrokeRect(screen, x, y, w, h, 1, colorPanelEdge, false)
	ebitenutil.DebugPrintAt(screen, m.title, m.x+menuPad, m.y+menuPad)
	for i, it := range m.items {
		ty := m.y + menuPad + menuLine*(i+1)
		vector.StrokeLine(screen, x+2, float32(ty)-2, x+w-2, float32(ty)-2, 1, colorPanelEdge, false)
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%d) %s", i+1, it.label), m.x+menuPad, ty)
	}
}

// --- what a tap and a hold mean ---------------------------------------------

// tap is one press that came and went.
func (g *game) tap(x, y int) {
	if g.menu != nil {
		g.pickMenu(x, y)
		return
	}
	// Anything the world is waiting on comes first, wherever the tap landed:
	// the clock is stopped behind it, so nothing else on the screen means
	// anything until it is answered.
	if g.openPending(x, y) {
		return
	}
	if x >= worldWidth {
		return
	}
	if g.play == playDriven {
		g.tapWhileDriving(x, y)
		return
	}
	g.selectAgent(g.nodeAt(x, y))
}

// hold is a press that stayed put.
func (g *game) hold(x, y int) {
	if g.menu != nil {
		g.menu = nil
		return
	}
	if g.openPending(x, y) {
		return
	}
	if x >= worldWidth {
		return
	}
	if g.play == playDriven {
		// Aim first, so that the menu is about whatever is under the finger.
		g.aimAt(x, y)
		g.openMenu(g.describeMark()+":", x, y, g.orderMenu()...)
		return
	}
	g.openMenu("node:", x, y, g.watchMenu(g.nodeAt(x, y))...)
}

// tapWhileDriving aims, and orders if the aim did not move.
//
// The two taps are deliberately the same gesture rather than a tap and a
// double tap: a body walks while you think, so the second tap has to be
// allowed to come late.
func (g *game) tapWhileDriving(x, y int) {
	was := g.mark
	g.aimAt(x, y)
	if g.sameMark(was, g.mark) {
		g.defaultOrder()
		return
	}
	g.say("%s - tap again to %s", g.describeMark(), g.defaultName())
}

// sameMark says whether two aims are at the same thing. A spot is the same
// spot if a second tap would have picked it: what a finger means by "there"
// is as wide as what a finger means by "that".
func (g *game) sameMark(a, b mark) bool {
	if a.kind == markNone || a.kind != b.kind {
		return false
	}
	if a.kind == markSpot {
		reach := g.pickReach()
		return abs64(a.x-b.x) <= reach && abs64(a.y-b.y) <= reach
	}
	return a.id == b.id
}

func abs64(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// defaultName and defaultOrder are the one word a second tap means.
//
// A meal is eaten and bare ground is walked to, which leaves the bodies. A
// beast is attacked - it is the only thing in this world that is hunted - and
// a person is looked at, because attacking somebody is not a thing to do by
// tapping twice. Everything else about either of them is on the hold.
func (g *game) defaultName() string {
	switch g.mark.kind {
	case markFood:
		return "eat it"
	case markAgent:
		if g.markedIsPrey() {
			return "attack it"
		}
		return "watch them"
	case markSpot:
		return "walk there"
	}
	return "act"
}

func (g *game) defaultOrder() {
	switch g.mark.kind {
	case markFood:
		g.orderAt(engine.ActEat, markFood)
	case markAgent:
		if g.markedIsPrey() {
			g.orderAt(engine.ActAttack, markAgent)
			return
		}
		g.orderAt(engine.ActObserve, markAgent)
	case markSpot:
		g.orderMove()
	}
}

// markedIsPrey says whether what is aimed at is one of the beasts. It is
// asked of the node's own view rather than of the world: what species
// something is is on the face of it (AgentView.Species), so this is a thing
// the node knows.
func (g *game) markedIsPrey() bool {
	if g.mark.kind != markAgent {
		return false
	}
	v, ok := g.view()
	if !ok {
		return false
	}
	o, seen := v.AgentByID(g.mark.id)
	return seen && o.Prey
}

// orderMenu is the rest of the vocabulary, for whatever is aimed at.
//
// Only the words that can mean anything for that kind of mark are offered,
// and the engine still refuses the ones it would have refused from a key -
// this list is what is worth trying, not what is allowed.
func (g *game) orderMenu() []menuItem {
	at := func(label string, kind engine.ActionKind, want markKind) menuItem {
		return menuItem{label, func() { g.orderAt(kind, want) }}
	}
	items := []menuItem{}
	switch g.mark.kind {
	case markFood:
		items = append(items, at("eat", engine.ActEat, markFood),
			menuItem{"walk to it", g.orderMove})
	case markAgent:
		items = append(items,
			at("attack", engine.ActAttack, markAgent),
			at("watch", engine.ActObserve, markAgent),
			at("court", engine.ActCourt, markAgent),
			at("call in", engine.ActInvite, markAgent),
			at("throw a stone", engine.ActThrow, markAgent),
			at("flee", engine.ActFlee, markAgent),
			menuItem{"walk to them", g.orderMove})
	case markSpot:
		items = append(items, menuItem{"walk here", g.orderMove})
	}
	items = append(items,
		menuItem{"rest", func() { g.order(engine.Action{Kind: engine.ActRest}) }},
		menuItem{fmt.Sprintf("effort %.1f -> %.1f", g.effort, nextEffort(g.effort)), func() {
			g.effort = nextEffort(g.effort)
			g.say("effort %.1f (from the next order on)", g.effort)
		}},
		menuItem{"think again", g.askToThinkAgain})
	return items
}

// nextEffort steps round the same five levels the number keys set.
func nextEffort(e float64) float64 {
	step := 1 / float64(len(effortKeys))
	next := e + step
	if next > 1+1e-9 {
		next = step
	}
	return next
}

// watchMenu is what a hold offers when nobody is being driven: the three ways
// to run a node, reachable without the keyboard.
func (g *game) watchMenu(id int) []menuItem {
	items := []menuItem{}
	if id != 0 {
		items = append(items, menuItem{fmt.Sprintf("follow #%d", id), func() { g.selectAgent(id) }})
	}
	if g.played == 0 && (id != 0 || g.selected != 0) {
		items = append(items, menuItem{"take the reins", func() {
			if id != 0 {
				g.selectAgent(id)
			}
			g.toggleControl() // off -> asked
			g.toggleControl() // asked -> driven
		}}, menuItem{"let it ask me", func() {
			if id != 0 {
				g.selectAgent(id)
			}
			g.toggleControl()
		}})
	}
	if g.played != 0 {
		items = append(items, menuItem{"give it back", func() {
			// Round the cycle until the node is the world's again, and no
			// further: toggleControl leaves the mode alone when the body has
			// died under it, and a loop on a condition it cannot reach is a
			// frozen screen.
			for g.play != playOff {
				was := g.play
				g.toggleControl()
				if g.play == was {
					return
				}
			}
		}})
	}
	items = append(items, menuItem{pauseLabel(g.paused), func() { g.paused = !g.paused }})
	return items
}

func pauseLabel(paused bool) string {
	if paused {
		return "run the clock"
	}
	return "stop the clock"
}

// --- the questions, as a menu -----------------------------------------------

// openPending puts up whatever the game is waiting for an answer to, and says
// whether there was one. The order is the order the keys answer them in: a
// succession first (there is no body until it is answered), then a proposal
// (it is on a clock), then a question, then an offer.
func (g *game) openPending(x, y int) bool {
	switch {
	case g.succession != nil:
		items := make([]menuItem, 0, len(g.succession.picks))
		for _, p := range g.succession.picks {
			id := p.id
			items = append(items, menuItem{fmt.Sprintf("#%d %s", p.id, p.about), func() { g.goOnAs(id) }})
		}
		g.openMenu("go on as:", x, y, items...)
		return true

	case g.beingCourted():
		who, _ := g.proposer()
		g.openMenu(fmt.Sprintf("#%d is courting:", who), x, y,
			menuItem{"yes", func() { g.sayToSuitor(true) }},
			menuItem{"no", func() { g.sayToSuitor(false) }})
		return true
	}

	if g.play != playAsked || g.played == 0 {
		return false
	}
	if q, ok := g.world.Question(g.played); ok {
		offered := bets(q)
		items := make([]menuItem, 0, len(offered)+1)
		for _, b := range offered {
			i := b.idx
			items = append(items, menuItem{b.label, func() { g.answer(i) }})
		}
		items = append(items, menuItem{"leave it to itself", func() {
			g.world.LetItBe(g.played)
			g.say("left to itself")
			g.paused = false
		}})
		g.openMenu("it is asking:", x, y, items...)
		return true
	}
	if g.offer != nil {
		items := make([]menuItem, 0, len(g.offer.picks)+1)
		for i, p := range g.offer.picks {
			at := i
			items = append(items, menuItem{p.label, func() { g.takeTheOffer(at) }})
		}
		items = append(items, menuItem{"leave it as it was", func() {
			g.offer = nil
			g.say("left as it was raised")
			g.paused = false
		}})
		g.openMenu(g.offer.why, x, y, items...)
		return true
	}
	return false
}

// sayToSuitor is the yes and the no of answerProposal, without the keys. One
// function, because which controller is on the body is a thing this file
// should not have to know twice.
func (g *game) sayToSuitor(yes bool) {
	who, _ := g.proposer()
	if who == 0 {
		return
	}
	switch {
	case g.human != nil:
		g.human.AnswerProposal(who, yes)
	case g.guided != nil:
		g.guided.AnswerProposal(who, yes)
	default:
		return
	}
	if yes {
		g.say("you said yes to #%d", who)
	} else {
		g.say("you turned #%d down", who)
	}
	g.paused = false
}
