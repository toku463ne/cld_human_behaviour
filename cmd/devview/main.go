// Command devview is a development tool: it imports the engine directly (no
// network involved) and draws it, so that a new rule can be watched right after
// it is written. It is not the real client, which will render snapshots
// received from the server.
//
// Watching the whole population at speed answers "does the world hold
// together". Following one node answers "why did it do that", which needs the
// opposite: click a node to select it, slow the clock down or step tick by
// tick, and read its decisions in the panel on the right.
package main

import (
	"flag"
	"fmt"
	"image/color"
	"log"
	"math"
	"sort"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/toku463ne/cld_human_behaviour/engine"
)

const (
	worldWidth  = 820
	worldHeight = 660

	// The panel on the right holds the selected node's decisions. It is text
	// only: the point is to read the numbers the choice was made on.
	panelWidth = 460
	panelX     = worldWidth
	lineHeight = 16
	panelChars = panelWidth/6 - 2 // the debug font is 6 pixels wide

	screenWidth  = worldWidth + panelWidth
	screenHeight = worldHeight

	minRadius   = 3.5
	maxRadius   = 11.0
	minRingSize = 1.0
	maxRingSize = 4.0

	// How close a click has to land to select a node.
	pickRadius = 14.0

	// How many of a selected agent's opinions to list.
	maxOpinionRows = 12
)

// speeds are the playback rates, in simulation ticks per drawn frame. The slow
// end is what following a single node needs: at 1/10 there is time to read a
// decision before the next one happens.
var speeds = []struct {
	label string
	ticks float64
}{
	{"1/10", 0.2},
	{"1/5", 0.4},
	{"1/2", 1},
	{"normal", 2},
	{"fast", 8},
}

const normalSpeed = 3 // index of the rate the viewer starts at

// zoomLevels are how far in the camera can go. Perceived speed is relative to
// the view: a body crossing two hundred pixels of screen looks brisk and the
// same body crossing eight hundred looks glacial, which is most of why playing
// felt slow.
var zoomLevels = []float64{1, 1.6, 2.5, 4}

const closeZoom = 2 // the level playing starts at

var (
	colorBackground = color.RGBA{0xfc, 0xfc, 0xfb, 0xff}
	colorPanel      = color.RGBA{0xef, 0xef, 0xec, 0xff}
	colorPanelEdge  = color.RGBA{0xc0, 0xc0, 0xba, 0xff}
	colorFood       = color.RGBA{0x1b, 0xaf, 0x7a, 0xff}
	colorMale       = color.RGBA{0x2a, 0x78, 0xd6, 0xff}
	colorFemale     = color.RGBA{0xe8, 0x7b, 0xa4, 0xff}
	colorForage     = color.RGBA{0xc3, 0xc2, 0xb7, 0xff}
	colorSeekMate   = color.RGBA{0xeb, 0x68, 0x34, 0xff}
	colorPaired     = color.RGBA{0x0c, 0xa3, 0x0c, 0xff}
	colorFighting   = color.RGBA{0xd0, 0x1c, 0x1c, 0xff}
	colorFleeing    = color.RGBA{0x8a, 0x4c, 0xd6, 0xff}
	colorResting    = color.RGBA{0x6c, 0x9c, 0xc4, 0xff}
	colorPairLink   = color.RGBA{0x0b, 0x0b, 0x0b, 0x30}
	colorFightLink  = color.RGBA{0xd0, 0x1c, 0x1c, 0x80}
	colorSelected   = color.RGBA{0x11, 0x11, 0x11, 0xff}
	colorSight      = color.RGBA{0x33, 0x88, 0xcc, 0xa0}
	colorRegionEdge = color.RGBA{0x30, 0x60, 0x30, 0x50}
	colorTarget     = color.RGBA{0x11, 0x11, 0x11, 0x60}
	colorHungerBar  = color.RGBA{0xc9, 0x8a, 0x20, 0xff}
	colorTail       = color.RGBA{0x44, 0x44, 0x77, 0xb0}
	colorPlayed     = color.RGBA{0xd9, 0x9a, 0x00, 0xff}
	colorBubble     = color.RGBA{0x1a, 0x1a, 0x22, 0xe0}
	colorKin        = color.RGBA{0xd9, 0x9a, 0x00, 0x90}
	colorMark       = color.RGBA{0xd9, 0x9a, 0x00, 0xc0}
	colorHeir       = color.RGBA{0x0c, 0xa3, 0x0c, 0xc0}
)

// panelMode is what the right hand panel shows about the selected node.
type panelMode uint8

const (
	modeDecision panelMode = iota // the utility comparison behind its last moves
	modeBeliefs                   // what it reckons about everybody it has met
	modePlay                      // what a person driving it has to go on

	numPanelModes = int(iota)
)

type game struct {
	world *engine.World

	paused bool
	speed  int
	// zoom is an index into zoomLevels. The camera follows the played node
	// when there is one and the selected node otherwise, which is the whole of
	// it: there is no free camera, because there is nothing to look at that is
	// not one of those two.
	zoom int
	// tickAccum carries the fraction of a tick left over by a slow rate, so
	// that 1/5 speed really is one tick every five frames.
	tickAccum float64

	selected int // agent ID, 0 for none
	mode     panelMode
	// traceBack is how far into the decision history the panel is looking:
	// 0 is the most recent decision.
	traceBack int

	// Stage 19: one node can be driven by whoever is at the keyboard. The
	// controller is the engine's; everything else here is the interface's own
	// bookkeeping, because the engine has no idea a game is being played.
	//
	// Stage 23 added the other way of playing: the node decides for itself and
	// stops to ask at the turning points. Both are Controller implementations
	// and the world cannot tell them apart, so all that is kept here is which
	// one is installed.
	play    playMode
	played  int
	human   *engine.HumanController
	guided  *engine.GuidedController
	effort  float64
	stance  engine.Stance
	mark    mark
	heir    int // the child the line is to continue through, 0 for none
	notice  string
	noticed int // tick the notice was put up

	// Stage 23. askedAt is the tick of the question the viewer has already
	// stopped for, so that one question stops it once. was is the state of the
	// played node last time round, which is all a milestone is: a change in
	// one of the four things in it. offer is the split waiting to be made.
	askedAt int
	was     lifeMark
	offer   *offer

	// The question raised by the played body dying: which of the line's
	// living children to go on as. It is the interface's question, like the
	// milestone offer - the engine has never heard of a line - and it is asked
	// rather than assumed, because going on as a newborn is a different game
	// from going on as a grown child.
	succession *succession

	// What the player is playing, as against what the node is optimising for
	// (the utility formula has no term for a line continuing). lineKids is
	// every child ever born to a body this line has occupied, which is the
	// only way to count descendants once the parents are gone.
	lineFrom int
	lineAt   int
	bodies   int
	lineKids []int

	// The answer last taken up, so that the next question can say what it
	// bought. Without it a choice is made into a void: the world moves on and
	// nothing ever reports back.
	last *choiceMade

	// padKey is the key the player is walking with, noKey when none. Walking
	// is held rather than set: letting go stops the node, which is what a key
	// held down means to a person and needs no new action - the engine already
	// has one for "stand still".
	padKey ebiten.Key

	// walkTo is a place the node is walking to under its own steam, and the
	// answer to running past things: an order carries a direction and nothing
	// else, so somebody has to notice the arrival, and that somebody is the
	// interface. lastAim is the direction last given, so that the order is
	// only re-issued when it has drifted.
	walkTo  mark
	lastAim float64

	// boost is whether the protagonist's body is brought up to the population's
	// average speed, and given is how much was added to the body being played.
	// Shown on the panel: a gift nobody is told about is indistinguishable from
	// the world being generous, and this world is not.
	boost bool
	given float64

	// What the protagonist has been through since the last frame, and the
	// short lines it is saying about it. Bubbles are for the played node only:
	// one node's news is legible, sixty nodes' news is the panel again.
	seen    seenState
	bubbles []bubble
	frame   int
}

// mark is what the player last clicked: the thing an order will be aimed at.
// It is not part of any decision - it is a cursor.
type mark struct {
	kind markKind
	id   int
	x, y float64
}

type markKind uint8

const (
	markNone markKind = iota
	markAgent
	markFood
	markSpot
)

// playMode is which of the two ways of playing is installed on the node, if
// either. They are not settings of one thing: each is a different Controller,
// and the difference between them is who answers the world's question.
type playMode uint8

const (
	playOff    playMode = iota
	playAsked           // stage 23: it decides, you answer at the turning points
	playDriven          // stage 19: you decide, every time it is asked
)

// lifeMark is the state of the played node the last time the interface looked.
// A milestone is a change in one of these: growing up, pairing, a child, or a
// body handed on.
type lifeMark struct {
	body     int
	adult    bool
	partner  int
	children int
	known    bool
}

// choiceMade is an answer and the state of the node when it was given, so that
// the difference can be read off later.
type choiceMade struct {
	tick     int
	act      engine.Action
	vitality float64
	hunger   float64
}

// bet is one option picked out of the field for the kind of wager it is, not
// for where it came in the ranking. Three kinds cover the shapes a decision
// takes here: spend little, finish soon, or go for the most.
type bet struct {
	label string
	idx   int
}

// bubble is a short line the protagonist says about something that just
// happened to it. It is measured in frames rather than ticks so that it still
// fades while the clock is stopped, and it is deliberately tiny: the complaint
// it answers is not that the panel lacks the information but that nobody can
// find one line in forty while driving.
type bubble struct {
	text string
	born int // frame
}

// bubbleLife is how long one stays up, in frames. Long enough to read three
// words, short enough that two events do not become a wall.
const bubbleLife = 150

// seenState is the protagonist as it was last frame. Every bubble comes from
// comparing this with now: the engine emits no events and does not need to,
// because a life is fully described by what changed in it.
type seenState struct {
	known    bool
	body     int
	partner  int
	children int
	voided   int
	action   engine.ActionKind
	target   int
	attacker int
	hunger   float64
	vitality float64
	frail    bool
	canCourt bool
}

// offer is a split of the three preferences put to a person at a milestone.
// The engine holds the rule (the total is what was inherited and does not
// change); which splits are worth offering is the interface's business.
type offer struct {
	why   string
	picks []offerPick
}

type offerPick struct {
	label string
	want  engine.Disposition
}

// succession is the choice of body put to the player when the one they were
// playing dies, and successionPick is one of the line's living children with a
// word about what taking it over would be like.
type succession struct {
	dead  int
	picks []successionPick
}

type successionPick struct {
	id    int
	about string
}

func (g *game) Update() error {
	g.handleInput()
	// Before the pause, so that bubbles go on fading while the clock is
	// stopped and a player who paused to read is not left with a wall of them.
	g.watchProtagonist()
	if g.paused {
		return nil
	}
	g.steerToWalkTo()
	g.tickAccum += speeds[g.speed].ticks
	for g.tickAccum >= 1 {
		g.world.Step()
		g.tickAccum--
	}
	g.carryTheLineOn()
	g.followTheLine()
	g.watchTheLife()
	return nil
}

func (g *game) handleInput() {
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeySpace):
		g.paused = !g.paused
	case g.succession == nil && g.play != playDriven &&
		(inpututil.IsKeyJustPressed(ebiten.KeyRight) || inpututil.IsKeyJustPressed(ebiten.KeyN)):
		// One tick, and stay stopped: this is how a single decision gets read.
		// While somebody is driving a node these keys walk it instead (see
		// walkWithPad), because that is the same thing plus a step.
		g.paused = true
		g.tickAccum = 0
		g.world.Step()
	case inpututil.IsKeyJustPressed(ebiten.KeyMinus):
		g.speed = max(g.speed-1, 0)
	case inpututil.IsKeyJustPressed(ebiten.KeyEqual):
		g.speed = min(g.speed+1, len(speeds)-1)
	case inpututil.IsKeyJustPressed(ebiten.KeyZ):
		g.zoom = (g.zoom + 1) % len(zoomLevels)
		g.say("zoom x%.1f", zoomLevels[g.zoom])
	case inpututil.IsKeyJustPressed(ebiten.KeyTab):
		g.mode = panelMode((int(g.mode) + 1) % numPanelModes)
	case inpututil.IsKeyJustPressed(ebiten.KeyBracketLeft):
		g.traceBack++ // further back in time
	case inpututil.IsKeyJustPressed(ebiten.KeyBracketRight):
		g.traceBack = max(g.traceBack-1, 0)
	case g.succession == nil && inpututil.IsKeyJustPressed(ebiten.KeyEscape):
		// While a node is being played the click is an aim rather than a
		// selection, so escape drops the aim first and the node second.
		if g.mark.kind != markNone {
			g.mark = mark{}
		} else {
			g.selectAgent(0)
		}
	}

	g.handlePlayInput()

	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		if mx < worldWidth {
			// Only the driven mode has an aim: answering a question is
			// choosing between things the node already picked out, so a click
			// goes back to being what it is everywhere else.
			if g.play == playDriven {
				g.aimAt(mx, my)
			} else {
				g.selectAgent(g.nodeAt(mx, my))
			}
		}
	}
}

// --- playing a node (stage 19) ---------------------------------------------
//
// The engine is not being asked to do anything new here: taking over is
// SetController, an order is what the controller answers with the next time the
// world asks, and handing the line on is the same controller installed on a
// child. Everything below is interface.

// handlePlayInput reads the keys that only mean something to a player.
func (g *game) handlePlayInput() {
	if g.succession != nil {
		// Nothing else means anything: there is no body to drive until this
		// is answered, and the clock is stopped behind it.
		g.handleSuccessionInput()
		return
	}
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyH):
		g.toggleControl()
	case inpututil.IsKeyJustPressed(ebiten.KeyK):
		g.pickHeir()
	}
	if g.played == 0 {
		return
	}
	if g.play == playAsked {
		g.handleAskedInput()
		return
	}
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyEnter), inpututil.IsKeyJustPressed(ebiten.KeyKPEnter):
		// Ask for the question rather than answering one: this is the only
		// way a player gets a decision out of turn, and it goes through the
		// same trigger machinery as everything else.
		if g.world.RequestDecision(g.played) {
			g.say("asked #%d to think again", g.played)
		}
	case inpututil.IsKeyJustPressed(ebiten.KeyS):
		g.stance = (g.stance + 1) % engine.Stance(engine.NumStances)
		g.say("stance %s (fighting orders only)", g.stance)
	case inpututil.IsKeyJustPressed(ebiten.KeyR):
		g.order(engine.Action{Kind: engine.ActRest})
	case inpututil.IsKeyJustPressed(ebiten.KeyM):
		g.orderMove()
	case inpututil.IsKeyJustPressed(ebiten.KeyE):
		g.orderAt(engine.ActEat, markFood)
	case inpututil.IsKeyJustPressed(ebiten.KeyA):
		g.orderAt(engine.ActAttack, markAgent)
	case inpututil.IsKeyJustPressed(ebiten.KeyF):
		g.orderAt(engine.ActFlee, markAgent)
	case inpututil.IsKeyJustPressed(ebiten.KeyO):
		g.orderAt(engine.ActObserve, markAgent)
	case inpututil.IsKeyJustPressed(ebiten.KeyC):
		g.orderAt(engine.ActCourt, markAgent)
	}
	for i, key := range effortKeys {
		if inpututil.IsKeyJustPressed(key) {
			g.effort = float64(i+1) / float64(len(effortKeys))
			g.say("effort %.1f (from the next order on)", g.effort)
		}
	}

	g.walkWithPad()
}

var effortKeys = []ebiten.Key{ebiten.Key1, ebiten.Key2, ebiten.Key3, ebiten.Key4, ebiten.Key5}

// padWalk is one direction and the keys that mean it, laid out as the number
// pad is: 8 is up, 2 is down, and the corners are the diagonals. 5 stands
// still.
//
// Each direction takes two keys, because with num lock off the pad does not
// send the pad: X11 turns the keys into the navigation cluster, so 6 arrives
// as the right arrow and 9 as page up. Binding both means the layout works
// whichever way the lock happens to be, and the arrows and Home/End/PgUp/PgDn
// walk a node as well - the pad arrangement is only what the keys are named
// after.
type padWalk struct {
	keys   []ebiten.Key
	dx, dy float64
}

// A slice rather than a map, because two keys held at once must resolve the
// same way every frame.
var padWalks = []padWalk{
	{[]ebiten.Key{ebiten.KeyNumpad7, ebiten.KeyHome}, -1, -1},
	{[]ebiten.Key{ebiten.KeyNumpad8, ebiten.KeyUp}, 0, -1},
	{[]ebiten.Key{ebiten.KeyNumpad9, ebiten.KeyPageUp}, 1, -1},
	{[]ebiten.Key{ebiten.KeyNumpad4, ebiten.KeyLeft}, -1, 0},
	{[]ebiten.Key{ebiten.KeyNumpad6, ebiten.KeyRight}, 1, 0},
	{[]ebiten.Key{ebiten.KeyNumpad1, ebiten.KeyEnd}, -1, 1},
	{[]ebiten.Key{ebiten.KeyNumpad2, ebiten.KeyDown}, 0, 1},
	{[]ebiten.Key{ebiten.KeyNumpad3, ebiten.KeyPageDown}, 1, 1},
	// Wait: let a tick pass without changing anything. With num lock off the
	// middle of the pad sends nothing ebiten knows, so n stands in for it -
	// which is what n did before a node could be driven at all.
	{[]ebiten.Key{ebiten.KeyNumpad5, ebiten.KeyN}, 0, 0},
}

// arrowOf names a direction in words. The debug font has no arrows in it.
func arrowOf(dx, dy float64) string {
	v := ""
	switch {
	case dy < 0:
		v = "north"
	case dy > 0:
		v = "south"
	}
	switch {
	case dx < 0:
		v += "west"
	case dx > 0:
		v += "east"
	}
	return v
}

// noKey is "no key is being held". It is not zero: zero is a real key.
const noKey = ebiten.Key(-1)

// padRepeatDelay is how long a key has to be held before it starts repeating,
// in frames. Below it, one press is one tick, which is how a node is walked a
// step at a time; above it the clock runs while the key is down.
const padRepeatDelay = 15

// walkWithPad moves the played node with the number pad, and advances the world
// while a key is down.
//
// This is the one place the interface drives the clock as well as the node, and
// it is worth saying why: deciding is trigger driven, so a player who orders a
// step and then waits for the world to ask is not playing a turn, they are
// waiting. Pressing a direction gives the order, asks for the question, and
// lets exactly one tick happen - which together are a turn.
//
// Letting go stops the node. Walking is the only order held rather than set;
// the standing kind is still there under m, which points at the mark and keeps
// going without anybody holding anything down.
func (g *game) walkWithPad() {
	pressed := padWalk{}
	held, heldKey := 0, noKey
	for _, w := range padWalks {
		for _, k := range w.keys {
			if !ebiten.IsKeyPressed(k) {
				continue
			}
			if d := inpututil.KeyPressDuration(k); d > held {
				pressed, held, heldKey = w, d, k
			}
		}
	}

	if held == 0 {
		if g.padKey != noKey {
			g.padKey = noKey
			// Only the walk's own order is put down. If the player has given
			// another one since - gone to eat something, say - letting go of
			// the key must not wipe it: that made every pursuit impossible to
			// order while the clock was stopped, since the walk keys were the
			// only thing that made time pass.
			if g.human != nil && g.human.Standing().Kind == engine.ActMove {
				_ = g.setOrder(engine.Action{Kind: engine.ActRest})
				g.world.RequestDecision(g.played)
			}
		}
		return
	}
	g.walkTo = mark{} // walking by hand cancels walking to somewhere

	// A new direction: give the order, and ask for it to be taken up now
	// rather than whenever the world next wonders.
	if heldKey != g.padKey {
		g.padKey = heldKey
		if pressed.dx != 0 || pressed.dy != 0 {
			if err := g.setOrder(engine.Action{Kind: engine.ActMove, DX: pressed.dx, DY: pressed.dy}); err != nil {
				g.say("no: %v", err)
			} else {
				g.say("walking %s - hold to keep going, let go to stop", arrowOf(pressed.dx, pressed.dy))
			}
			g.world.RequestDecision(g.played)
		}
	}

	// One tick for the press, then the clock runs while the key stays down.
	if held == 1 || held > padRepeatDelay {
		g.tickAccum = 0
		g.world.Step()
		g.carryTheLineOn()
	}
}

// toggleControl cycles the one node between the three ways it can be run: by
// the utility formula alone, by the formula with a person answering the
// turning points, and by a person alone. They are three controllers, and the
// world is told about the change the same way each time.
//
// The order is the amount of control taken, in one direction: the AI has it,
// then you answer at the turning points, then you drive. It ran the other way
// round until 2026-09-06, and the second step was a trap - a player in the
// asked mode who wanted the reins pressed h and got the AI instead, and then h
// again did nothing at all because dropping a node also dropped the selection
// the next take-over needed. Both halves of that are fixed here: the cycle
// escalates, and letting a node go leaves it selected.
func (g *game) toggleControl() {
	switch g.play {
	case playOff:
		if g.selected == 0 {
			g.say("click a node first, then press h to take it over")
			return
		}
		g.guided = engine.NewGuidedController()
		if !g.world.SetController(g.selected, g.guided) {
			g.guided = nil
			g.say("#%d is gone", g.selected)
			return
		}
		g.play, g.played = playAsked, g.selected
		g.mode = modePlay
		g.zoom = closeZoom
		g.was = lifeMark{}
		g.askedAt = -1
		g.lineFrom, g.lineAt = g.world.Tick(), g.world.Tick()
		g.bodies, g.lineKids, g.last, g.walkTo = 1, nil, nil, mark{}
		g.endowTheProtagonist()
		g.say("#%d decides for itself and asks you at the turning points. h again to drive it", g.played)
		g.raiseOffer("you have taken up its life")

	case playAsked:
		// The same body, a different hand on it. Nothing about the node
		// changes: only who answers when the world asks.
		g.human = engine.NewHumanController()
		if !g.world.SetController(g.played, g.human) {
			g.human = nil
			g.say("#%d is gone", g.played)
			return
		}
		g.play, g.guided = playDriven, nil
		if g.offer != nil {
			// The clock was stopped for a milestone that belongs to the mode
			// being left. Taking the reins is the answer to it, so the world
			// starts again rather than sitting on a menu nobody can reach.
			g.offer, g.paused = nil, false
		}
		g.was = lifeMark{}
		g.padKey = noKey // a key held through the change walks the node now
		g.say("you are #%d. hold an arrow or numpad key to walk it", g.played)

	case playDriven:
		id := g.played
		g.world.SetController(id, nil) // nil is the world's own AI again
		g.play, g.played, g.human, g.guided, g.heir, g.offer = playOff, 0, nil, nil, 0, nil
		g.walkTo, g.lineKids = mark{}, nil
		// Left selected on purpose: h is how it is taken up again, and it
		// needs something selected to take up.
		g.selectAgent(id)
		g.say("#%d is back on the utility formula (h takes it up again)", id)
	}
}

// controller is whichever of the two a person is behind, for the code that
// only needs to install it somewhere else.
func (g *game) controller() engine.Controller {
	switch g.play {
	case playAsked:
		return g.guided
	case playDriven:
		return g.human
	}
	return nil
}

// pickHeir names the child the line is to continue through, cycling if there
// is more than one. Only grown children are offered: the engine's own line
// between a child and an adult (Agent.IsAdult) is the same one that decides
// whether an agent may court at all.
func (g *game) pickHeir() {
	if g.played == 0 {
		return
	}
	heirs := g.world.Heirs(g.played)
	if len(heirs) == 0 {
		g.say("no grown children yet: the line ends with #%d", g.played)
		g.heir = 0
		return
	}
	next := heirs[0]
	for i, id := range heirs {
		if id == g.heir {
			next = heirs[(i+1)%len(heirs)]
			break
		}
	}
	g.heir = next
	g.say("the line goes on through #%d (%d grown children)", g.heir, len(heirs))
}

// carryTheLineOn deals with the played body dying. A named heir is taken over
// straight away - that choice was already made, with k - and otherwise the
// line's living children are put to the player as a question.
//
// This is the whole of "switching to a child": the same controller, installed
// on somebody else. The world never learns that anything changed hands.
func (g *game) carryTheLineOn() {
	if g.played == 0 || g.succession != nil {
		return
	}
	if _, alive := g.world.AgentByID(g.played); alive {
		return
	}
	dead := g.played
	if g.heir != 0 && g.world.SetController(g.heir, g.controller()) {
		g.heir = 0
		g.takeOver(dead, g.played)
		return
	}
	if picks := g.survivors(dead); len(picks) > 0 {
		g.succession = &succession{dead: dead, picks: picks}
		g.paused = true
		g.mode = modePlay
		g.say("#%d died. 1-%d go on as one of its line, enter to stop here", dead, len(picks))
		return
	}
	g.endTheLine("#%d died with nobody left to follow it. the line ends", dead)
}

// survivors lists who the line could go on in, nearest of kin first: the dead
// body's own children before the rest of the line, and the grown before the
// still growing.
//
// It is wider than World.Heirs, which counts only children that have finished
// growing. That is the right line for naming an heir in advance - it is the
// same maturity that decides whether a node may court at all - but it is the
// wrong one for the moment a body dies. A player whose only child was born a
// hundred ticks ago was told the line ended, with the child alive on screen.
// Going on as a newborn is a poor hand, not an impossible one, and which it is
// is the player's to judge, so it is offered and labelled rather than hidden.
func (g *game) survivors(dead int) []successionPick {
	cfg := g.world.Config()
	type cand struct {
		pick  successionPick
		own   bool
		grown bool
	}
	cands := make([]cand, 0, len(g.lineKids))
	for _, id := range g.lineKids {
		a, alive := g.world.AgentByID(id)
		if !alive || id == dead {
			continue
		}
		own := a.ParentIDs[0] == dead || a.ParentIDs[1] == dead
		kin := "of the line"
		if own {
			kin = "its child"
		}
		grown := a.IsAdult(&cfg)
		age := "still growing"
		if grown {
			age = "grown"
		}
		cands = append(cands, cand{own: own, grown: grown, pick: successionPick{
			id: id,
			about: fmt.Sprintf("%s, %s (%.0f%% grown, vit %.0f/%.0f)",
				kin, age, a.Maturity*100, a.Vitality, a.MaxVitality(&cfg)),
		}})
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].own != cands[j].own {
			return cands[i].own
		}
		return cands[i].grown && !cands[j].grown
	})
	picks := make([]successionPick, 0, len(choiceKeys))
	for _, c := range cands {
		if len(picks) == len(choiceKeys) {
			break
		}
		picks = append(picks, c.pick)
	}
	return picks
}

// handleSuccessionInput answers the question the death raised. Nothing else a
// player can press means anything while it stands: there is no node to drive
// and no question for one to answer.
func (g *game) handleSuccessionInput() {
	for i, key := range choiceKeys {
		if i < len(g.succession.picks) && inpututil.IsKeyJustPressed(key) {
			g.goOnAs(g.succession.picks[i].id)
			return
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyKPEnter) ||
		inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		dead := g.succession.dead
		g.succession = nil
		g.endTheLine("#%d died and the line was not carried on. it ends", dead)
	}
}

// goOnAs installs the same controller on the chosen body.
func (g *game) goOnAs(id int) {
	dead := g.succession.dead
	if !g.world.SetController(id, g.controller()) {
		// It died while the question was up. The rest of the offer still
		// stands, so the question is asked again rather than ended for it.
		g.say("#%d is gone too", id)
		if picks := g.survivors(dead); len(picks) > 0 {
			g.succession.picks = picks
			return
		}
		g.succession = nil
		g.endTheLine("#%d died with nobody left to follow it. the line ends", dead)
		return
	}
	g.succession = nil
	g.heir = 0
	g.takeOver(dead, id)
	g.paused = false
}

// takeOver is the bookkeeping either way in: the controller is already on the
// new body, and everything reset here belonged to the old one.
func (g *game) takeOver(dead, id int) {
	g.played = id
	g.bodies++
	g.last = nil // what the last body's answer bought died with it
	g.endowTheProtagonist()
	g.mark = mark{}
	g.walkTo = mark{}
	g.padKey = noKey // so a key still held walks the new body too
	g.selectAgent(id)
	g.say("#%d died. you are #%d now", dead, id)
}

// endTheLine puts everything down. A line that is over is over: the rings on
// its children go with it.
func (g *game) endTheLine(format string, args ...any) {
	g.play, g.played, g.human, g.guided = playOff, 0, nil, nil
	g.heir, g.offer, g.succession = 0, nil, nil
	g.walkTo, g.mark, g.lineKids = mark{}, mark{}, nil
	g.say(format, args...)
}

// --- what kind of bet each option is (C and D) ------------------------------
//
// The field arrives ranked, and a ranked list with the best one at the top is
// not a choice: there is no reason to disagree with it, since the player knows
// nothing the node does not. What makes it a choice is showing the options as
// different kinds of wager and leaving the arithmetic out - the goal terms and
// what they cost are on the panel, the total is not.

// spend is what an option is expected to cost this body: the vitality it pours
// in plus what the one it is aimed at has already cost it.
func spend(u engine.Utility) float64 { return u.Vitality + u.Risk }

// promise is everything the option is trying to win, before any of it is paid
// for. This is the number the boldest option is bold about.
func promise(u engine.Utility) float64 {
	total := 0.0
	for _, g := range u.Goals() {
		total += g.Score()
	}
	return total
}

// bets picks what to put in front of a person: the two things the world
// actually pays out on, and three ways of playing everything else.
//
// Eating and courting get slots of their own rather than competing on score,
// because of what the player is playing. A line needs a body that stays alive
// and a body that has a child, and the utility formula prices both of those
// against the life of one node: a full belly makes eating worth little and a
// long life left makes courting worth little. Those are exactly the moments a
// person wants to overrule it, and an option that only appears when it also
// tops a ranking cannot be overruled with.
//
// The other three are ways of wagering rather than things to want: spend
// least, finish soonest, go for the most.
func bets(q engine.Question) []bet {
	of := func(k engine.ActionKind) func(engine.TracedOption) bool {
		return func(o engine.TracedOption) bool { return o.Action.Kind == k }
	}
	kinds := []struct {
		label string
		want  func(o engine.TracedOption) bool
		less  func(a, b engine.Utility) bool
	}{
		{"a meal ", of(engine.ActEat),
			func(a, b engine.Utility) bool { return a.Life.Score() > b.Life.Score() }},
		{"a child", of(engine.ActCourt),
			func(a, b engine.Utility) bool { return a.Offspring.Score() > b.Offspring.Score() }},
		{"safest ", nil, func(a, b engine.Utility) bool { return spend(a) < spend(b) }},
		{"soonest", nil, func(a, b engine.Utility) bool { return a.Ticks < b.Ticks }},
		{"boldest", nil, func(a, b engine.Utility) bool { return promise(a) > promise(b) }},
	}
	out := make([]bet, 0, len(kinds))
	taken := map[int]bool{}
	for _, k := range kinds {
		best := -1
		for i := range q.Options {
			if taken[i] || (k.want != nil && !k.want(q.Options[i])) {
				continue
			}
			if best < 0 || k.less(q.Options[i].Utility, q.Options[best].Utility) {
				best = i
			}
		}
		if best < 0 {
			continue // nothing in sight to eat, or nobody to court
		}
		taken[best] = true
		out = append(out, bet{label: k.label, idx: best})
	}
	return out
}

// --- what the last answer bought (B) ----------------------------------------

// noteChoice remembers the state an answer was given in. The node is asked
// again later and the difference is the only honest report there is: nothing
// in the world knows what a choice was "for".
func (g *game) noteChoice(act engine.Action) {
	a, alive := g.world.AgentByID(g.played)
	if !alive {
		return
	}
	g.last = &choiceMade{tick: g.world.Tick(), act: act, vitality: a.Vitality, hunger: a.Hunger}
}

func (g *game) drawLastChoice(t *textBox) {
	if g.last == nil {
		return
	}
	a, alive := g.world.AgentByID(g.played)
	if !alive {
		return
	}
	t.line("LAST TIME you chose %s, %d ticks ago:",
		describeAction(g.last.act), g.world.Tick()-g.last.tick)
	t.line("  vitality %.1f -> %.1f (%+.1f)   hunger %.1f -> %.1f (%+.1f)",
		g.last.vitality, a.Vitality, a.Vitality-g.last.vitality,
		g.last.hunger, a.Hunger, a.Hunger-g.last.hunger)
	if id := g.last.act.TargetID; id != 0 {
		switch g.last.act.Kind {
		case engine.ActEat:
			if _, ok := g.world.FoodByID(id); ok {
				t.line("  meal #%d is still there", id)
			} else {
				t.line("  meal #%d is gone", id)
			}
		default:
			if _, ok := g.world.AgentByID(id); ok {
				t.line("  #%d is still alive", id)
			} else {
				t.line("  #%d is dead", id)
			}
		}
	}
}

// --- the line, which is what a player is actually playing (A) ---------------

// followTheLine keeps the count of what this line has come to. The utility
// formula has no term for any of it: a node is scored on its own life, and a
// line is the thing the person at the keyboard is playing instead.
func (g *game) followTheLine() {
	a, alive := g.world.AgentByID(g.played)
	if !alive {
		return
	}
	g.lineAt = g.world.Tick()
	for _, kid := range a.ChildIDs {
		known := false
		for _, seen := range g.lineKids {
			if seen == kid {
				known = true
				break
			}
		}
		if !known {
			g.lineKids = append(g.lineKids, kid)
		}
	}
}

// describeKin lists the line's living children and whether the played body can
// see each of them. Knowing you have a child is knowing about your own life;
// knowing where it is, is not, so an unseen one is said to be unseen.
func (g *game) describeKin() string {
	parts := make([]string, 0, 4)
	for _, id := range g.lineKids {
		if _, alive := g.world.AgentByID(id); !alive {
			continue
		}
		where := " (not in sight)"
		if g.world.CanTargetAgent(g.played, id) {
			where = ""
		}
		parts = append(parts, fmt.Sprintf("#%d%s", id, where))
		if len(parts) == 4 {
			parts = append(parts, "...")
			break
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "children: " + strings.Join(parts, ", ")
}

// isKin reports whether this is one of the children the played line has had.
func (g *game) isKin(id int) bool {
	for _, kid := range g.lineKids {
		if kid == id {
			return true
		}
	}
	return false
}

// livingKin is how many of the children this line has produced are still alive.
func (g *game) livingKin() int {
	n := 0
	for _, id := range g.lineKids {
		if _, ok := g.world.AgentByID(id); ok {
			n++
		}
	}
	return n
}

// --- being asked (stage 23) -------------------------------------------------
//
// Two clocks run here and they are deliberately different. A question comes at
// the turning points of a day - a fight starting, what it was after being gone
// - and is answered with one move. An offer comes at the milestones of a life
// and is answered with the kind of node it is going to be. The engine raises
// the first and knows nothing of the second: which changes count as milestones
// is the game's business, the same way "player" and "game over" were.

// choiceKeys are the number row, which is what answers both. In the driven
// mode they set the effort instead; the two never overlap because a mode only
// ever offers one of them.
var choiceKeys = []ebiten.Key{ebiten.Key1, ebiten.Key2, ebiten.Key3, ebiten.Key4, ebiten.Key5}

// handleAskedInput reads the keys of the asked mode. A question is answered
// before a milestone offer is, because a question goes stale and an offer does
// not: the world is stopped for the one that is on a clock.
func (g *game) handleAskedInput() {
	g.sayWalkingIsNotYours()
	enter := inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyKPEnter)

	if q, ok := g.world.Question(g.played); ok {
		offered := bets(q)
		for i, key := range choiceKeys {
			if i < len(offered) && inpututil.IsKeyJustPressed(key) {
				g.answer(offered[i].idx)
				return
			}
		}
		if enter {
			// Not the same as taking the first option: that one is asked
			// again, and this one is not.
			g.world.LetItBe(g.played)
			g.say("left to itself")
			g.paused = false
		}
		return
	}

	if g.offer != nil {
		for i, key := range choiceKeys {
			if i < len(g.offer.picks) && inpututil.IsKeyJustPressed(key) {
				g.takeTheOffer(i)
				return
			}
		}
		if enter {
			// An offer never goes stale, so without a way to put it down it
			// would sit on the number keys for the rest of the life.
			g.offer = nil
			g.say("left as it was raised")
			g.paused = false
		}
		return
	}

	if enter {
		// The one question a person raises themselves. Without it the only
		// thing to do between turning points is watch.
		if g.world.RequestDecision(g.played) {
			g.say("asked #%d what it is thinking", g.played)
		}
	}

}

// sayWalkingIsNotYours answers the walk keys in the mode that does not have
// them. The node is driving itself here, so the pad does nothing - and doing
// nothing silently reads as the game being broken rather than as the node
// deciding for itself.
//
// The two keys that already mean something in this mode (right and n advance a
// tick) are left alone: they are not the ones a puzzled player is pressing.
func (g *game) sayWalkingIsNotYours() {
	// Not in the first frames: X reports the keyboard state as the window
	// opens, and an arrow or two comes through as "just pressed" without
	// anybody having touched anything (seen under WSLg: ArrowUp and ArrowLeft
	// on frame one). A hint that fires before the player has done anything
	// teaches them to ignore the notice line.
	if g.frame < 30 {
		return
	}
	for _, w := range padWalks {
		for _, k := range w.keys {
			if k == ebiten.KeyRight || k == ebiten.KeyN {
				continue
			}
			if inpututil.IsKeyJustPressed(k) {
				g.say("#%d walks itself in this mode - press h to take the reins", g.played)
				return
			}
		}
	}
}

func (g *game) answer(i int) {
	q, ok := g.world.Question(g.played)
	if !ok {
		return
	}
	if err := g.world.Answer(g.played, i); err != nil {
		g.say("no: %v", err)
		return
	}
	g.noteChoice(q.Options[i].Action)
	g.say("#%d will do that next", g.played)
	g.paused = false
}

// watchTheLife stops the clock when there is something to answer. A question
// stops it once - the tick it was raised on - so that a player who lets it
// stand is not stopped again on the next frame.
func (g *game) watchTheLife() {
	if g.play != playAsked || g.played == 0 {
		return
	}
	raised := g.milestone()
	if q, ok := g.world.Question(g.played); ok && q.Tick != g.askedAt {
		g.askedAt = q.Tick
		raised = true
	}
	if !raised {
		return
	}
	g.paused = true
	g.mode = modePlay
}

// milestone reports whether one of the four things that happen to a life just
// happened, raising the offer if so.
func (g *game) milestone() bool {
	a, alive := g.world.AgentByID(g.played)
	if !alive {
		return false
	}
	cfg := g.world.Config()
	now := lifeMark{
		body:     a.ID,
		adult:    a.IsAdult(&cfg),
		partner:  a.PartnerID,
		children: len(a.ChildIDs),
		known:    true,
	}
	was := g.was
	g.was = now
	if !was.known || g.offer != nil {
		return false
	}
	why := ""
	switch {
	case now.body != was.body:
		why = "a body handed on: what it was raised as is what it has"
	case now.adult && !was.adult:
		why = "it has grown up"
	case now.children > was.children:
		why = "it has a child"
	case now.partner != 0 && was.partner == 0:
		why = "it has paired"
	}
	if why == "" {
		return false
	}
	g.raiseOffer(why)
	return true
}

// raiseOffer puts the split to the player. The three splits on offer are not
// the only ones possible - the engine would take any proportions - but a
// milestone is a choice, and a choice is a handful of things with names.
func (g *game) raiseOffer(why string) {
	d, ok := g.world.Disposition(g.played)
	if !ok {
		return
	}
	g.offer = &offer{why: why, picks: []offerPick{
		{"cautious", engine.Disposition{Risk: 3, Competition: 1, Shock: 1}},
		{"quick-tempered", engine.Disposition{Risk: 1, Competition: 3, Shock: 1}},
		{"clings to life", engine.Disposition{Risk: 1, Competition: 1, Shock: 3}},
		{"as it was raised", d},
	}}
	g.paused = true
	g.mode = modePlay
}

func (g *game) takeTheOffer(i int) {
	pick := g.offer.picks[i]
	if err := g.world.SetDisposition(g.played, pick.want); err != nil {
		g.say("no: %v", err)
		return
	}
	g.offer = nil
	g.say("#%d is %s", g.played, pick.label)
	g.paused = false
}

// scaled is what a split comes to once the engine has fitted it to what this
// node has to divide, which is what the panel shows next to each choice.
func scaled(want engine.Disposition, total float64) engine.Disposition {
	sum := want.Total()
	if sum <= 0 {
		return engine.Disposition{}
	}
	f := total / sum
	return engine.Disposition{Risk: want.Risk * f, Competition: want.Competition * f, Shock: want.Shock * f}
}

// aimAt points the order at whatever was clicked: somebody, something to eat,
// or a patch of ground to walk to.
//
// Clicking something the node cannot see aims at the ground where it is
// instead, and says so. The player can see the whole world and the node cannot,
// and the honest thing to do with the difference is to let them walk over
// there and find out - not to let them act on it, and not to pretend the click
// did nothing.
func (g *game) aimAt(mx, my int) {
	if id := g.nodeAt(mx, my); id != 0 && id != g.played {
		if g.world.CanTargetAgent(g.played, id) {
			g.mark = mark{kind: markAgent, id: id}
			return
		}
		wx, wy := g.inWorld(mx, my)
		g.mark = mark{kind: markSpot, x: wx, y: wy}
		g.say("#%d is outside what it can see - aiming at that ground instead", id)
		return
	}
	if f, ok := g.foodAt(mx, my); ok {
		if g.world.CanTargetFood(g.played, f.ID) {
			g.mark = mark{kind: markFood, id: f.ID, x: f.X, y: f.Y}
			return
		}
		g.mark = mark{kind: markSpot, x: f.X, y: f.Y}
		g.say("meal #%d is not something it can see and eat - aiming at that ground", f.ID)
		return
	}
	wx, wy := g.inWorld(mx, my)
	g.mark = mark{kind: markSpot, x: wx, y: wy}
}

// foodAt returns the food item under the cursor.
// foodAt and nodeAt take screen coordinates and answer in world ones. The
// radius they pick within is a screen radius - a click is a click whatever the
// zoom - so it is divided back out rather than scaled up.
func (g *game) foodAt(mx, my int) (engine.Food, bool) {
	wx, wy := g.inWorld(mx, my)
	reach := g.pickReach()
	best, bestDist := engine.Food{}, reach*reach
	found := false
	for _, f := range g.world.Foods() {
		dx, dy := f.X-wx, f.Y-wy
		if d := dx*dx + dy*dy; d < bestDist {
			bestDist, best, found = d, f, true
		}
	}
	return best, found
}

// pickReach is how far a click reaches, in world units.
func (g *game) pickReach() float64 {
	scale, _, _ := g.camera()
	return pickRadius / scale
}

// order hands the standing order to the controller and reports what came of it.
// A refusal is worth showing rather than swallowing: it is the engine saying
// the node could not have come up with that itself.
func (g *game) order(a engine.Action) {
	if a.Kind != engine.ActMove {
		// Any other order is the player taking the wheel back.
		g.walkTo = mark{}
	}
	if g.human == nil {
		return
	}
	if err := g.setOrder(a); err != nil {
		g.say("no: %v", err)
		return
	}
	// And ask the question, because pressing the key is the player doing
	// something. The order is still only an answer - the engine decides when
	// it is put - but a deliberate keypress is a good enough reason to put it
	// now, and waiting for the world to wonder felt like the game ignoring
	// you. Enter is still there for asking again without changing the order.
	g.world.RequestDecision(g.played)
	g.say("order: %s%s", describeAction(g.human.Standing()), g.letTimeRun())
}

// letTimeRun makes sure an order can actually be carried out, and says what it
// did about it.
//
// An order is not an act: eating something seventeen paces away is a walk and
// then a meal, and it needs the ticks for both. With the clock stopped, the one
// thing that advanced it was the walk keys - which set a move order of their
// own - so an order given while paused was placed, never taken up, and then
// overwritten by the only key that made time pass. From the outside that is a
// game that ignores you, and it is what "it will not eat the food in front of
// it" turned out to be.
//
// So an order starts the clock. Stopping it again is a key away, and the walk
// keys still step a tick at a time for anybody who wants to read the world one
// tick at a time.
func (g *game) letTimeRun() string {
	if !g.paused {
		return ""
	}
	g.paused = false
	return " (and let the clock run - space stops it again)"
}

// setOrder hands the order over without saying anything about it. The walking
// keys use this: they report themselves by moving.
//
// It goes through the world rather than straight to the controller, because
// the world is what knows whether the node can see what the order names, and
// it knows it as of now rather than as of the last question (World.OrderHuman).
func (g *game) setOrder(a engine.Action) error {
	if g.human == nil {
		return nil
	}
	a.Effort = g.effort
	a.Stance = g.stance
	return g.world.OrderHuman(g.played, a)
}

// orderMove walks towards the mark, whatever kind of thing it is.
func (g *game) orderMove() {
	a, ok := g.world.AgentByID(g.played)
	if !ok {
		return
	}
	x, y, ok := g.markPos()
	if !ok {
		g.say("click somewhere first: a move needs a direction")
		return
	}
	// Walk there and stop, rather than walk that way for ever. The engine is
	// given an ordinary move order; the stopping is the interface noticing.
	g.walkTo = g.mark
	g.lastAim = math.Atan2(y-a.Y, x-a.X)
	g.order(engine.Action{Kind: engine.ActMove, DX: x - a.X, DY: y - a.Y})
	g.say("walking to %s - it stops when it gets there", g.describeMark())
}

// orderAt aims an action at the mark, or, when nothing of the right kind is
// aimed at, at the nearest one the node can see.
//
// Falling back rather than refusing is the difference between a game and a
// form: the meal three paces away is in the node's own perception, so choosing
// it for the player tells them nothing their node did not already know. What
// it must never do is reach past what the node can see, which is why the
// candidates come from the view and not from the world.
func (g *game) orderAt(kind engine.ActionKind, want markKind) {
	if g.mark.kind == want {
		g.order(engine.Action{Kind: kind, TargetID: g.mark.id})
		return
	}
	id, dist, ok := g.nearestVisible(kind, want)
	if !ok {
		what := "nobody"
		if want == markFood {
			what = "nothing"
		}
		g.say("%s: it can see %s to do that to", kind, what)
		return
	}
	g.mark = mark{kind: want, id: id}
	g.walkTo = mark{} // an order of any other kind is the player taking the wheel back
	if err := g.setOrder(engine.Action{Kind: kind, TargetID: id}); err != nil {
		g.say("no: %v", err)
		return
	}
	g.world.RequestDecision(g.played)
	g.say("%s #%d, %.0f away - the nearest it can see%s", kind, id, dist, g.letTimeRun())
}

// nearestVisible is the closest thing of the right kind the node can see right
// now. The world answers what is in sight (CanTargetAgent / CanTargetFood), so
// this can never reach past what the node knows; courting looks at more than
// distance, since somebody already paired is not a candidate.
func (g *game) nearestVisible(kind engine.ActionKind, want markKind) (id int, dist float64, ok bool) {
	me, live := g.world.AgentByID(g.played)
	if !live {
		return 0, 0, false
	}
	best := math.Inf(1)
	if want == markFood {
		for _, f := range g.world.Foods() {
			if !g.world.CanTargetFood(g.played, f.ID) {
				continue
			}
			if d := math.Hypot(f.X-me.X, f.Y-me.Y); d < best {
				best, id, ok = d, f.ID, true
			}
		}
		return id, best, ok
	}
	for _, o := range g.world.Agents() {
		if !g.world.CanTargetAgent(g.played, o.ID) {
			continue
		}
		if kind == engine.ActCourt && (o.Sex == me.Sex || o.PartnerID != 0) {
			continue
		}
		if d := math.Hypot(o.X-me.X, o.Y-me.Y); d < best {
			best, id, ok = d, o.ID, true
		}
	}
	return id, best, ok
}

// markPos is where the mark is now. An agent moves, so its mark is followed
// rather than remembered.
func (g *game) markPos() (x, y float64, ok bool) { return g.markPosOf(g.mark) }

// markPosOf is where a mark is now. A mark on somebody moves with them, which
// is what makes walking to a stranger work as well as walking to a spot.
func (g *game) markPosOf(m mark) (x, y float64, ok bool) {
	switch m.kind {
	case markAgent:
		if a, live := g.world.AgentByID(m.id); live {
			return a.X, a.Y, true
		}
	case markFood:
		if f, live := g.world.FoodByID(m.id); live {
			return f.X, f.Y, true
		}
	case markSpot:
		return m.x, m.y, true
	}
	return 0, 0, false
}

func (g *game) say(format string, args ...any) {
	g.notice = fmt.Sprintf(format, args...)
	g.noticed = g.world.Tick()
}

// nodeAt returns the node under the cursor, or 0.
func (g *game) nodeAt(mx, my int) int {
	wx, wy := g.inWorld(mx, my)
	reach := g.pickReach()
	best, bestDist := 0, reach*reach
	for _, a := range g.world.Agents() {
		dx, dy := a.X-wx, a.Y-wy
		if d := dx*dx + dy*dy; d < bestDist {
			bestDist, best = d, a.ID
		}
	}
	return best
}

// selectAgent switches which node is being followed. Only the selected one has
// its decisions recorded, which is why the engine keeps tracing off by default.
func (g *game) selectAgent(id int) {
	if id == g.selected {
		return
	}
	if g.selected != 0 {
		g.world.TrackDecisions(g.selected, false)
	}
	g.selected = id
	g.traceBack = 0
	if id != 0 {
		g.world.TrackDecisions(id, true)
	}
}

func (g *game) Draw(screen *ebiten.Image) {
	screen.Fill(colorBackground)
	g.drawWorld(screen)
	g.drawBubbles(screen)

	vector.DrawFilledRect(screen, panelX, 0, panelWidth, screenHeight, colorPanel, false)
	vector.StrokeLine(screen, panelX, 0, panelX, screenHeight, 1, colorPanelEdge, false)

	ebitenutil.DebugPrint(screen, g.overlay())
	g.drawPanel(screen)
}

// averageSpeedGene is what an ordinary grown human has spent on being quick.
// It is read live rather than kept as a number, so it goes on meaning the same
// thing as the population's allocations drift.
func averageSpeedGene(w *engine.World) float64 {
	cfg := w.Config()
	sum, n := 0.0, 0
	for _, a := range w.Agents() {
		if a.Species != engine.SpeciesHuman || !a.IsAdult(&cfg) {
			continue
		}
		sum, n = sum+a.Gene(engine.GeneSpeed), n+1
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// endowTheProtagonist brings the body being played up to the average speed,
// paying for it with new budget rather than out of its other genes.
//
// It is not a rule of the world and the engine never does it on its own: it is
// the interface admitting that a body which spent five points on being quick
// is unplayable by a person, whatever it is to the utility formula. The rest of
// the genome is left exactly as it was inherited.
func (g *game) endowTheProtagonist() {
	g.given = 0
	if !g.boost || g.played == 0 {
		return
	}
	cfg := g.world.Config()
	// Two things, and they are different in kind. Speed is what makes a body
	// drivable; being able to pay for a birth and survive it is what makes a
	// line possible at all, and a protagonist that cannot found one is not
	// playing this game. Both are paid for with new budget.
	if added, err := g.world.Endow(g.played, engine.GeneSpeed, averageSpeedGene(g.world)); err == nil {
		g.given += added
	}
	if cfg.MaxVitality > 0 {
		// The gene that puts MaxVitality at what a birth costs twice over: it
		// can pay its half and still have as much again left.
		need := midGene * cfg.BirthVitalityCost / cfg.MaxVitality
		if added, err := g.world.Endow(g.played, engine.GeneVitality, need); err == nil {
			g.given += added
		}
	}
	if g.given <= 0 {
		return
	}
	g.say("#%d was given %.0f to be playable (speed to the world's average, vitality enough for a birth)",
		g.played, g.given)
}

// midGene is the gene value an ability of exactly the world's own figure comes
// from: MaxVitality and MaxSpeed are both the config's number scaled by the
// gene over this. It is engine.MaxAbility/2 rounded the way the engine does it.
const midGene = 50.5

// --- walking somewhere and stopping there ----------------------------------
//
// ActMove carries a direction, not a destination, and that is right for the
// engine: an agent heading for something names the something (a meal, a
// stranger), and an agent heading nowhere in particular is wandering. What it
// leaves out is the one thing a person at a keyboard wants - "go there and
// stop" - so the interface does it, by steering an ordinary move order and
// noticing the arrival.

// arriveRadius is how close counts as there, in world units. It is the reach
// an order needs to act on what it walked to.
const arriveRadius = 10

func (g *game) steerToWalkTo() {
	if g.play != playDriven || g.played == 0 || g.walkTo.kind == markNone {
		return
	}
	a, alive := g.world.AgentByID(g.played)
	if !alive {
		g.walkTo = mark{}
		return
	}
	// Somebody has given this node something else to do. Steering it would
	// overwrite that, and the whole point of the steering is to save the
	// player work, not to argue with them.
	if g.human != nil && g.human.Standing().Kind != engine.ActMove {
		g.walkTo = mark{}
		return
	}
	tx, ty, ok := g.markPosOf(g.walkTo)
	if !ok {
		g.walkTo = mark{}
		g.say("what it was walking to is gone")
		_ = g.setOrder(engine.Action{Kind: engine.ActRest})
		g.world.RequestDecision(g.played)
		return
	}
	dx, dy := tx-a.X, ty-a.Y
	if math.Hypot(dx, dy) <= arriveRadius {
		g.walkTo = mark{}
		_ = g.setOrder(engine.Action{Kind: engine.ActRest})
		g.world.RequestDecision(g.played)
		g.say("arrived")
		return
	}
	// Only re-aimed when the bearing has actually drifted. Asking the world to
	// think again on every tick is the one freedom a player has that the AI
	// does not, and spending it on arithmetic nobody needed would be a waste
	// of it.
	aim := math.Atan2(dy, dx)
	if math.Abs(angleGap(aim, g.lastAim)) < 0.08 {
		return
	}
	g.lastAim = aim
	if err := g.setOrder(engine.Action{Kind: engine.ActMove, DX: dx, DY: dy}); err != nil {
		g.walkTo = mark{}
		g.say("no: %v", err)
		return
	}
	g.world.RequestDecision(g.played)
}

// angleGap is the shortest way round from one bearing to the other.
func angleGap(a, b float64) float64 {
	d := math.Mod(a-b+math.Pi, 2*math.Pi)
	if d < 0 {
		d += 2 * math.Pi
	}
	return d - math.Pi
}

// --- what the protagonist has to say ---------------------------------------
//
// The engine emits no events and does not need to. A life is fully described by
// what changed in it, and everything worth saying out loud is a difference
// between this frame's protagonist and last frame's: a partner where there was
// none, a child more than before, an order that lapsed, somebody hitting it.
//
// Deriving them here rather than adding a notification to the engine is not
// only cheaper. A game that can ask the engine to tell it things starts pulling
// the engine towards being a game, and the one thing that has held through
// every stage of this is that the engine does not know a game is being played.

// pop puts a line above the protagonist's head.
func (g *game) pop(format string, args ...any) {
	if len(g.bubbles) >= 4 {
		g.bubbles = g.bubbles[1:]
	}
	g.bubbles = append(g.bubbles, bubble{text: fmt.Sprintf(format, args...), born: g.frame})
}

// watchProtagonist reads the difference between now and last frame, and says
// the parts of it a person would want to know while their hands are busy.
func (g *game) watchProtagonist() {
	g.frame++
	// Expire from the front: they are in the order they were said.
	for len(g.bubbles) > 0 && g.frame-g.bubbles[0].born > bubbleLife {
		g.bubbles = g.bubbles[1:]
	}
	if g.played == 0 {
		g.seen = seenState{}
		return
	}
	a, alive := g.world.AgentByID(g.played)
	if !alive {
		return
	}
	cfg := g.world.Config()

	now := seenState{
		known:    true,
		body:     a.ID,
		partner:  a.PartnerID,
		children: len(a.ChildIDs),
		action:   a.Action.Kind,
		target:   a.Action.TargetID,
		hunger:   a.Hunger,
		vitality: a.Vitality,
		frail:    a.Vitality < a.MaxVitality(&cfg)/3,
		canCourt: a.CanReproduce(&cfg),
	}
	if g.human != nil {
		now.voided = g.human.Voided()
		if v, ok := g.human.View(); ok && g.human.Body() == a.ID {
			now.attacker = v.Self.AttackerID
		}
	} else if g.guided != nil {
		if v, ok := g.guided.View(); ok && g.guided.Body() == a.ID {
			now.attacker = v.Self.AttackerID
		}
	}

	was := g.seen
	g.seen = now
	if !was.known {
		return
	}
	if was.body != now.body {
		g.bubbles = g.bubbles[:0]
		g.pop("you are #%d now", now.body)
		return
	}

	// The world doing things to it comes first: those are the ones a player
	// needs even when they are looking somewhere else.
	if now.attacker != 0 && now.attacker != was.attacker {
		g.pop("#%d is hitting me", now.attacker)
	}
	if now.action == engine.ActAttack && (was.action != engine.ActAttack || was.target != now.target) {
		g.pop("fighting #%d", now.target)
	}
	if now.action == engine.ActFlee && was.action != engine.ActFlee {
		g.pop("running from #%d", now.target)
	}

	// Then the life: pairing, children, and the courtship that did not take.
	switch {
	case was.partner == 0 && now.partner != 0:
		g.pop("paired with #%d!", now.partner)
	case was.partner != 0 && now.partner == 0:
		// A bond ends in one of three ways, and until 2026-09-06 they all
		// read as the same four words. Nearly all of them end in a child now;
		// the other two are worth saying out loud precisely because they are
		// rare, and because "the bond has ended" over and over with nothing
		// to show for it is what a broken world looks like.
		switch _, there := g.world.AgentByID(was.partner); {
		case now.children > was.children:
			g.pop("the bond has ended") // the next line says a child came of it
		case !there:
			g.pop("#%d is gone: no child", was.partner)
		default:
			g.pop("the bond ended with no child")
		}
	}
	if now.children > was.children {
		g.pop("a child! #%d", a.ChildIDs[len(a.ChildIDs)-1])
	}
	if was.action == engine.ActCourt && now.action != engine.ActCourt && now.partner == 0 {
		// Turned down, and not merely told to do something else. The node's
		// own perception is what says which: a candidate it walked away from
		// is marked as one it is not interested in comparing again just yet,
		// and that mark is only ever set by a courtship that did not take.
		if turned, ok := g.turnedDownBy(was.target); ok && turned {
			// Which side said no. "#12 turned me down" was said whoever had
			// refused, and half the time it was this node itself holding out
			// for somebody better - the same kind of lie the lapsed-order
			// count used to tell.
			switch c, ok := g.lastCourtship(was.target); {
			case !ok:
				g.pop("#%d and I did not pair", was.target)
			case c.Accepted && !c.TheyAccepted:
				g.pop("#%d turned me down", was.target)
			case !c.Accepted && c.TheyAccepted:
				g.pop("I turned #%d down", was.target)
			default:
				g.pop("#%d and I both said no", was.target)
			}
		}
	}
	if now.canCourt && !was.canCourt {
		g.pop("ready to court")
	}

	// And the housekeeping a player would otherwise only find out by noticing
	// that nothing is happening.
	if now.voided > was.voided {
		if was.action == engine.ActEat {
			g.pop("somebody took that meal")
		} else {
			g.pop("what I was after is gone")
		}
	}
	if now.hunger < was.hunger-5 {
		g.pop("ate")
	}
	if now.frail && !was.frail {
		g.pop("badly hurt")
	}
}

// turnedDownBy reports whether the node's own view of somebody says it has
// just walked away from courting them.
func (g *game) turnedDownBy(id int) (bool, bool) {
	v, ok := g.view()
	if !ok {
		return false, false
	}
	o, seen := v.AgentByID(id)
	if !seen {
		return false, false
	}
	return o.Rejected, true
}

// lastCourtship is how the node's last courtship went, if it was with this
// candidate and not with somebody since.
func (g *game) lastCourtship(id int) (engine.CourtView, bool) {
	v, ok := g.view()
	if !ok || v.Self.LastCourt.TargetID != id || id == 0 {
		return engine.CourtView{}, false
	}
	return v.Self.LastCourt, true
}

// view is whatever the node was last handed, whichever hand is on it.
func (g *game) view() (engine.HumanView, bool) {
	switch {
	case g.human != nil:
		return g.human.View()
	case g.guided != nil:
		return g.guided.View()
	}
	return engine.HumanView{}, false
}

// drawBubbles stacks what the protagonist is saying above its head, newest
// nearest to it.
func (g *game) drawBubbles(screen *ebiten.Image) {
	if g.played == 0 || len(g.bubbles) == 0 {
		return
	}
	a, ok := g.world.AgentByID(g.played)
	if !ok {
		return
	}
	cfg := g.world.Config()
	x, y := g.onScreen(a.X, a.Y)
	top := y - g.long(minRadius+a.MaxVitality(&cfg)/150*(maxRadius-minRadius)) - 8

	for i := len(g.bubbles) - 1; i >= 0; i-- {
		b := g.bubbles[i]
		w := float32(len(b.text)*6 + 8)
		by := top - float32((len(g.bubbles)-1-i)*(lineHeight+2))
		bx := x - w/2
		vector.DrawFilledRect(screen, bx, by-13, w, 15, colorBubble, true)
		ebitenutil.DebugPrintAt(screen, b.text, int(bx)+4, int(by)-14)
	}
}

// --- the camera ------------------------------------------------------------
//
// Everything the world draws is in world coordinates, and everything on the
// screen goes through these three. Keeping the transform in one place is what
// lets the zoom exist at all: the alternative was scaling a rendered image,
// which turns every circle to mush at the zoom the game actually wants.

// camera is the scale and the top left corner of what is on screen, in world
// coordinates. It follows the played node, or the selected one, and stops at
// the edges of the world so that the view is never half empty.
func (g *game) camera() (scale, ox, oy float64) {
	scale = zoomLevels[g.zoom]
	if scale <= 1 {
		return 1, 0, 0
	}
	viewW, viewH := worldWidth/scale, worldHeight/scale
	cx, cy := float64(worldWidth)/2, float64(worldHeight)/2
	follow := g.played
	if follow == 0 {
		follow = g.selected
	}
	if a, ok := g.world.AgentByID(follow); ok {
		cx, cy = a.X, a.Y
	}
	ox = clamp(cx-viewW/2, 0, math.Max(worldWidth-viewW, 0))
	oy = clamp(cy-viewH/2, 0, math.Max(worldHeight-viewH, 0))
	return scale, ox, oy
}

// onScreen turns a point in the world into a point on the screen.
func (g *game) onScreen(x, y float64) (float32, float32) {
	scale, ox, oy := g.camera()
	return float32((x - ox) * scale), float32((y - oy) * scale)
}

// long turns a length in the world into a length on the screen.
func (g *game) long(v float64) float32 {
	scale, _, _ := g.camera()
	return float32(v * scale)
}

// inWorld turns a point on the screen back into a point in the world, which is
// what every click needs.
func (g *game) inWorld(mx, my int) (float64, float64) {
	scale, ox, oy := g.camera()
	return float64(mx)/scale + ox, float64(my)/scale + oy
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (g *game) drawWorld(screen *ebiten.Image) {
	g.drawRegions(screen)
	g.drawSight(screen)

	for _, f := range g.world.Foods() {
		fx, fy := g.onScreen(f.X, f.Y)
		vector.DrawFilledCircle(screen, fx, fy, g.long(3), colorFood, true)
	}

	g.drawAim(screen)

	agents := g.world.Agents()

	// Bonds, drawn once per pair, and every blow being thrown.
	for i := range agents {
		a := &agents[i]
		ax, ay := g.onScreen(a.X, a.Y)
		if a.PartnerID > a.ID {
			if p, ok := g.world.AgentByID(a.PartnerID); ok {
				px, py := g.onScreen(p.X, p.Y)
				vector.StrokeLine(screen, ax, ay, px, py, 1, colorPairLink, true)
			}
		}
		if a.Action.Kind == engine.ActAttack {
			if t, ok := g.world.AgentByID(a.Action.TargetID); ok {
				tx, ty := g.onScreen(t.X, t.Y)
				vector.StrokeLine(screen, ax, ay, tx, ty, 1.5, colorFightLink, true)
			}
		}
	}

	cfg := g.world.Config()
	for i := range agents {
		a := &agents[i]
		// The circle is the body: how wide it is, is what the agent spent on
		// being big, and how much of it is filled in is how much vitality it
		// has left in there. Drawing the radius from the vitality itself, as
		// this used to, made a large agent that had been hurt look exactly
		// like a small one in good health - which is the whole difference the
		// budget is supposed to create.
		capacity := a.MaxVitality(&cfg)
		radius := g.long(minRadius + capacity/150*(maxRadius-minRadius))
		filled := radius
		if capacity > 0 {
			filled = radius * float32(clamp01(a.Vitality/capacity))
		}
		// The ring keeps its width on screen rather than in the world: it is a
		// reading of the attack gene, not a part of the body's size.
		ringWidth := float32(minRingSize + a.Attack(&cfg)/100*(maxRingSize-minRingSize))

		fill := colorMale
		if a.Sex == engine.Female {
			fill = colorFemale
		}

		x, y := g.onScreen(a.X, a.Y)

		// A tail behind it, as long as the agent is quick. Speed is otherwise
		// invisible: two agents standing still look the same however much one
		// of them spent on being fast.
		if speed := a.MaxSpeed(&cfg); speed > 0 {
			tail := g.long(speed / cfg.MaxSpeed * 13)
			vx, vy := float32(a.VX), float32(a.VY)
			if l := float32(math.Hypot(float64(vx), float64(vy))); l > 1e-6 {
				vx, vy = vx/l, vy/l
				vector.StrokeLine(screen, x-vx*tail, y-vy*tail, x, y, 1.5, colorTail, true)
			}
		}

		vector.DrawFilledCircle(screen, x, y, filled, fill, true)
		vector.StrokeCircle(screen, x, y, radius, ringWidth, stateColor(a.State), true)

		if hunger := float32(a.Hunger / 100); hunger > 0.01 {
			bar := g.long(12)
			vector.StrokeLine(screen, x-bar/2, y+radius+3, x-bar/2+bar*hunger, y+radius+3, 2, colorHungerBar, true)
		}
		if a.ID == g.selected {
			vector.StrokeCircle(screen, x, y, radius+5, 1.5, colorSelected, true)
			g.markTarget(screen, a)
		}
		// The body a person is driving, and the child the line is to carry on
		// through. Neither is anything to the engine.
		if a.ID == g.played {
			vector.StrokeCircle(screen, x, y, radius+8, 2, colorPlayed, true)
		}
		if a.ID == g.heir {
			vector.StrokeCircle(screen, x, y, radius+8, 1.5, colorHeir, true)
		}
		// The line's own children. A birth put a bubble up and then the child
		// was one more circle among sixty: which one it was could not be told
		// from the screen at all.
		//
		// Ringing them is labelling something already drawn rather than
		// telling the player anything new - devview has always drawn the whole
		// world - and the rule that matters is untouched: a child outside what
		// the node can see still cannot be aimed at.
		if a.ID != g.played && a.ID != g.heir && g.isKin(a.ID) {
			vector.StrokeCircle(screen, x, y, radius+12, 2, colorKin, true)
		}
	}
}

// geneShort is what each gene is called in the panel, short enough that all
// nine fit on two lines.
var geneShort = [engine.NumGenes]string{"atk", "def", "vit", "spd", "eva", "mem", "rat", "int", "look"}

// geneLine prints a run of genes as "name value (share of budget)". The share
// is the figure to compare between agents: the raw value moves whenever the
// budget does.
func geneLine(a *engine.Agent, from, to int) string {
	budget := a.Budget()
	var b strings.Builder
	for g := from; g < to && g < engine.NumGenes; g++ {
		v := a.Gene(engine.Gene(g))
		share := 0.0
		if budget > 0 {
			share = v / budget * 100
		}
		fmt.Fprintf(&b, "%s %3.0f(%2.0f%%) ", geneShort[g], v, share)
	}
	return b.String()
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// drawRegions shades the world's own blocks by how exposed resting in them is
// (stage 14). Darker is ground with its back covered, where lying down among
// strangers costs less. Nothing else about a region is visible, because nothing
// else about a region exists: it is not a wall and no node knows it is in one.
func (g *game) drawRegions(screen *ebiten.Image) {
	for _, r := range g.world.Regions() {
		rx, ry := g.onScreen(r.MinX, r.MinY)
		w, h := g.long(r.MaxX-r.MinX), g.long(r.MaxY-r.MinY)

		// Fill: how well the ground grows plants. Green for better than an
		// equal share, brown for worse, nothing at all for ordinary, so a
		// world without regions stays blank.
		if shade := uint8(clamp01(math.Abs(r.Food-1)/0.6) * 55); shade > 0 {
			fill := color.RGBA{0x30, 0x70, 0x30, shade} // rich
			if r.Food < 1 {
				fill = color.RGBA{0x80, 0x60, 0x20, shade} // thin
			}
			vector.DrawFilledRect(screen, rx, ry, w, h, fill, false)
		}

		// Border: how sheltered the resting is. A thick edge is ground with
		// its back covered.
		if r.Shelter < 1 {
			thick := float32(1 + clamp01((1-r.Shelter)/0.6)*3)
			vector.StrokeRect(screen, rx, ry, w, h, thick, colorRegionEdge, false)
		}
	}
}

// drawSight outlines what the followed node can see. Since stage 13 that is a
// block of cells rather than a circle, and the thing worth seeing on screen is
// that it does not move with the node: it jumps a whole cell at a time, so a
// node walking a straight line watches the world behind it stay visible and
// then vanish all at once.
func (g *game) drawSight(screen *ebiten.Image) {
	if g.selected == 0 {
		return
	}
	a, ok := g.world.AgentByID(g.selected)
	if !ok {
		return
	}
	minX, minY, maxX, maxY := g.world.SightBlock(a.X, a.Y)
	sx, sy := g.onScreen(minX, minY)
	vector.StrokeRect(screen, sx, sy, g.long(maxX-minX), g.long(maxY-minY), 2, colorSight, false)
}

// markTarget rings whatever the selected node is currently acting on, so that
// the target named in the trace can be found on the map.
func (g *game) markTarget(screen *ebiten.Image, a *engine.Agent) {
	switch a.Action.Kind {
	case engine.ActEat:
		for _, f := range g.world.Foods() {
			if f.ID == a.Action.TargetID {
				fx, fy := g.onScreen(f.X, f.Y)
				vector.StrokeCircle(screen, fx, fy, g.long(7), 1.5, colorTarget, true)
			}
		}
	case engine.ActAttack, engine.ActFlee, engine.ActObserve, engine.ActCourt:
		if t, ok := g.world.AgentByID(a.Action.TargetID); ok {
			tx, ty := g.onScreen(t.X, t.Y)
			vector.StrokeCircle(screen, tx, ty, g.long(14), 1.5, colorTarget, true)
		}
	}
}

// drawAim shows where an order would be pointed. It is a cursor and nothing
// more: the node knows nothing about it until an order names it.
func (g *game) drawAim(screen *ebiten.Image) {
	if g.played == 0 {
		return
	}
	x, y, ok := g.markPos()
	if !ok {
		return
	}
	fx, fy := g.onScreen(x, y)
	vector.StrokeCircle(screen, fx, fy, 9, 1.5, colorMark, true)
	vector.StrokeLine(screen, fx-13, fy, fx-10, fy, 1.5, colorMark, true)
	vector.StrokeLine(screen, fx+10, fy, fx+13, fy, 1.5, colorMark, true)
	vector.StrokeLine(screen, fx, fy-13, fx, fy-10, 1.5, colorMark, true)
	vector.StrokeLine(screen, fx, fy+10, fx, fy+13, 1.5, colorMark, true)
}

func stateColor(s engine.State) color.RGBA {
	switch s {
	case engine.StateSeekMate:
		return colorSeekMate
	case engine.StatePaired:
		return colorPaired
	case engine.StateFighting:
		return colorFighting
	case engine.StateFleeing:
		return colorFleeing
	case engine.StateResting:
		return colorResting
	default:
		return colorForage
	}
}

func (g *game) overlay() string {
	s := g.world.Stats()
	var b strings.Builder

	state := "playing " + speeds[g.speed].label
	if g.paused {
		state = "PAUSED"
	}
	fmt.Fprintf(&b, "tick %d (hour %.2f)  pop %d (m %d / f %d)  food %d  births %d  deaths %d (kills %d)  gen %d  [%s]\n",
		s.Tick, g.world.Hour(), s.Population, s.Males, s.Females, s.FoodItems, s.Births, s.Deaths, s.Kills, s.MaxGeneration, state)
	fmt.Fprintf(&b, "avg power %.1f  rationality %.1f  intelligence %.1f  vitality %.1f  hunger %.1f\n",
		s.AvgPower, s.AvgRationality, s.AvgIntelligence, s.AvgVitality, s.AvgHunger)
	b.WriteString("circle = body (outline its size, fill what is left in it), tail = speed, ring width = attack, bar = hunger\n")
	b.WriteString("ring: grey forage, orange mate, green paired, red fighting, purple fleeing, blue resting\n")
	b.WriteString("children are small circles: a newborn expresses 60% of its genes and grows into the rest by eating\n")
	if g.played != 0 {
		b.WriteString("gold ring = you, green ring = the heir, faint gold ring = a child of your line\n")
	}
	switch g.play {
	case playDriven:
		b.WriteString("space pause   -/= slower/faster   z zoom   esc drop the aim   h hand it back to the AI\n")
	case playAsked:
		b.WriteString("space pause   right/n one tick   -/= slower/faster   z zoom   h take the reins (the pad walks it)\n")
	default:
		b.WriteString("space pause   right/n one tick   -/= slower/faster   z zoom   click a node   esc clear\n")
	}
	b.WriteString("tab decisions/beliefs/play   [ ] older/newer decision   h play the selected node\n")
	switch g.play {
	case playDriven:
		fmt.Fprintf(&b, "playing #%d: numpad or arrows+home/end/pgup/pgdn walk it (hold to keep going)   click a spot then m walks there and stops\n", g.played)
		b.WriteString("   click to aim   r rest  m walk to the mark  e eat  a attack  f flee  o observe  c court   1-5 effort  s stance  k heir\n")
	case playAsked:
		fmt.Fprintf(&b, "playing #%d: it decides for itself and stops to ask at the turning points. 1-5 answer, enter ask now / leave it, k heir\n", g.played)
		b.WriteString("   the pad does not walk it in this mode - it walks itself. press h to take the reins\n")
	}
	if g.succession != nil {
		fmt.Fprintf(&b, "#%d is dead: 1-%d go on as one of its line, enter to stop here (see the panel)\n",
			g.succession.dead, len(g.succession.picks))
	}
	// Why the clock is stopped, where the player is already looking. "PAUSED"
	// on its own is indistinguishable from a game that has stopped working,
	// which is exactly what a stopped world plus keys that do nothing reads as.
	if g.play == playAsked && g.succession == nil {
		if q, ok := g.world.Question(g.played); ok {
			fmt.Fprintf(&b, "waiting on you: %s. answer 1-%d on the panel, or enter to leave it to itself\n",
				q.Trigger, len(bets(q)))
		} else if g.offer != nil {
			fmt.Fprintf(&b, "waiting on you: %s. answer 1-%d on the panel, or enter to leave it as it is\n",
				g.offer.why, len(g.offer.picks))
		}
	}
	if g.play == playDriven && g.walkTo.kind != markNone {
		fmt.Fprintf(&b, "walking to %s\n", g.describeMark())
	}
	if g.notice != "" {
		fmt.Fprintf(&b, "%s (tick %d)\n", g.notice, g.noticed)
	}
	return b.String()
}

// --- the panel -------------------------------------------------------------

// textBox writes the panel line by line, and stops when it runs out of room so
// that a long list never spills over the bottom edge.
type textBox struct {
	screen *ebiten.Image
	y      int
}

func (t *textBox) line(format string, args ...any) {
	if t.y > screenHeight-lineHeight {
		return
	}
	s := fmt.Sprintf(format, args...)
	if len(s) > panelChars {
		s = s[:panelChars]
	}
	ebitenutil.DebugPrintAt(t.screen, s, panelX+8, t.y)
	t.y += lineHeight
}

// roomLeft is how many more lines fit.
func (t *textBox) roomLeft() int {
	return (screenHeight - lineHeight - t.y) / lineHeight
}

func (g *game) drawPanel(screen *ebiten.Image) {
	t := &textBox{screen: screen, y: 8}

	// Playing beats following. Clicking bare ground or pressing escape drops
	// the selection, and while a node was being played that took the game off
	// the panel with it - including the question the world had stopped for, so
	// the clock was waiting for an answer to something nobody could read.
	if g.play != playOff && g.selected == 0 {
		g.drawPlay(t)
		return
	}

	if g.selected == 0 {
		t.line("no node selected")
		t.line("")
		t.line("click a node to follow it. only the node you are")
		t.line("following has its decisions recorded, so this is")
		t.line("cheap enough to leave on.")
		t.line("")
		t.line("to watch one node decide:")
		t.line("  - click it")
		t.line("  - press - a few times, or space to stop")
		t.line("  - press right (or n) to advance one tick")
		return
	}

	a, alive := g.world.AgentByID(g.selected)
	if alive {
		t.line("#%d %s  gen %d  age %d", a.ID, a.Sex, a.Generation, a.Age)
		cfg := g.world.Config()
		t.line("vit %5.1f/%.0f  hun %5.1f   %s   budget %.0f",
			a.Vitality, a.MaxVitality(&cfg), a.Hunger, a.Species, a.Budget())
		t.line("genes %s", geneLine(&a, 0, 5))
		t.line("      %s", geneLine(&a, 5, engine.NumGenes))
		t.line("age %5.1f years  grown %3.0f%%  expressing %3.0f%% of its genes",
			float64(a.Age)/float64(max(cfg.TicksPerYear, 1)), a.Maturity*100, a.AgeFactor(&cfg)*100)
		t.line("looks %.0f (what others can see of its build)", a.Appearance(&cfg))
		t.line("memory %d/%d faces   forgets at x%.2f",
			len(g.world.Opinions(a.ID)), a.MemoryCapacity(&cfg), a.ForgetScale(&cfg))
		// What it assumes, as against what it knows about anybody in
		// particular. The counts say whether it has seen anything: with
		// learning off they never move off the founding figure.
		as := a.Assumes()
		t.line("assumes: hit back %.2f (%.0f seen)   courted %.2f (%.0f seen)",
			as.Retaliation, as.RetaliationSeen, as.Accept, as.AcceptSeen)
		t.line("wants:   risk x%.2f   rival %.3f   empty %.2f",
			as.RiskWeight, as.Competition, as.ShockRisk)
		// One rule of thumb per line: they are the least self explanatory
		// thing on the panel, and running them together made the line longer
		// than the panel is wide.
		if hints, slots := a.Hints(); slots > 0 {
			t.line("hunches: %d of %d rooms used", len(hints), slots)
			for _, h := range hints {
				t.line("   %-11s -> %-8s %+5.1f", h.Feature, h.Act, h.Weight)
			}
		}
		if chrono, fit := g.world.ClockOf(a.ID); g.world.Hour() > 0 || chrono > 0 {
			sleepy := "wide awake"
			switch {
			case fit > 0.5:
				sleepy = "its own hour"
			case fit > -0.5:
				sleepy = "neither here nor there"
			}
			t.line("clock: sleeps best at %.2f, now %.2f (%s)", chrono, g.world.Hour(), sleepy)
		}
		shelter, food := g.world.GroundAt(a.X, a.Y)
		t.line("ground here: resting x%.2f, plants x%.2f (1 = ordinary)", shelter, food)
		if d := g.world.DietOf(a.ID); d[engine.FoodPlant] < 1 || d[engine.FoodMeat] < 1 {
			t.line("sick of it: a plant is worth x%.2f, meat x%.2f",
				d[engine.FoodPlant], d[engine.FoodMeat])
		}
		if known, total, gain := g.world.CountryKnownBy(a.ID); total > 0 {
			where := "nowhere better known"
			if gain > 0 {
				where = fmt.Sprintf("reckons somewhere is %.1f better", gain)
			}
			t.line("country: knows %d of %d regions, %s", known, total, where)
		}
		t.line("state %s   doing %s", a.State, describeAction(a.Action))
		t.line("parents %v  children %v", a.ParentIDs, a.ChildIDs)
	} else {
		t.line("#%d is gone. its last decisions are below.", g.selected)
	}
	t.line("")

	switch g.mode {
	case modeBeliefs:
		g.drawBeliefs(t)
	case modePlay:
		g.drawPlay(t)
	default:
		g.drawDecision(t)
	}
}

// drawPlay is the panel a person drives from. Everything on it comes out of
// the perception the node was last handed, and nothing comes out of
// World.Agents(): a player who could read the true power of the stranger in
// front of them would be playing a different game from the one the AI plays.
func (g *game) drawPlay(t *textBox) {
	if g.succession != nil {
		g.drawSuccession(t)
		return
	}
	if g.played == 0 {
		t.line("nobody is being played.")
		t.line("")
		t.line("click a node and press h to take it up: it decides")
		t.line("for itself and stops to ask you at the turning")
		t.line("points. press h again to drive it yourself (the")
		t.line("pad walks it), and again to give it back to the AI.")
		t.line("")
		t.line("z zooms the camera in on whoever you are playing.")
		t.line("")
		t.line("driving it yourself:")
		t.line("  click   aim at somebody, something to eat, a spot")
		t.line("  r rest   m move to the mark   e eat   a attack")
		t.line("  f flee   o observe            c court")
		t.line("  (with nothing aimed at, those take the nearest one")
		t.line("   the node can see)")
		t.line("  numpad 1-9 (or the arrows with home/end/pgup/pgdn)")
		t.line("    walk it one tick per press, or hold to keep going")
		t.line("  1-5 effort   s stance   enter think again now")
		t.line("  k name the child to carry on   h hand back to AI")
		return
	}

	if g.play == playAsked {
		g.drawAsked(t)
		return
	}

	asked, at := g.human.Asked()
	// Everything the controller holds - the order, the view, the count - still
	// belongs to the previous body until this one has been asked something.
	fresh := g.human.Body() == g.played
	t.line("YOU ARE #%d   effort %.1f   stance %s", g.played, g.effort, g.stance)
	g.drawGift(t)
	switch {
	case !fresh:
		t.line("standing order: none. an heir starts with none")
		t.line("not asked as #%d yet (the line has been asked %d times)", g.played, asked)
	case asked == 0:
		t.line("standing order: %s", describeAction(g.human.Standing()))
		t.line("not asked yet. it acts when the world next asks it.")
	default:
		t.line("standing order: %s", describeAction(g.human.Standing()))
		t.line("asked %d times, last on tick %d (%d ago); it answered %s",
			asked, at, g.world.Tick()-at, describeAction(g.human.LastAnswer()))
	}
	if v, d := g.human.Voided(), g.human.Finished(); v > 0 || d > 0 {
		t.line("%d order(s) carried out, %d lapsed (what they were aimed at was gone)", d, v)
	}
	t.line("aim: %s", g.describeMark())
	t.line("YOUR LINE: %d bod(ies), %d ticks, %d born, %d alive",
		g.bodies, g.lineAt-g.lineFrom, len(g.lineKids), g.livingKin())
	if kin := g.describeKin(); kin != "" {
		t.line("  %s", kin)
	}
	if g.heir != 0 {
		t.line("line continues through #%d", g.heir)
	} else {
		t.line("no heir named (k). the line ends when this body does")
	}
	if v, ok := g.human.View(); ok && fresh {
		// High up, because "why did that one turn me down" is a question a
		// player asks while it is happening, and the bottom of the panel is
		// where the lines that get cut off live.
		g.drawCourting(t, v)
	}
	t.line("")

	view, ok := g.human.View()
	if !ok || !fresh {
		// Between a handover and the first question put to the new body, the
		// view still belongs to the one that died. Showing it under "you are
		// #84" would be showing a player their predecessor's last moments as
		// if they were their own.
		t.line("it has not been asked anything yet, so it has")
		t.line("nothing to tell you. press enter to ask it.")
		return
	}

	g.drawWhatItKnows(t, view)
}

// drawWhatItKnows prints the perception the node was last handed, and nothing
// else. It is shared by both ways of playing because the promise is the same
// one: a player who could read the true power of the stranger in front of them
// would be playing a different game from the one the AI plays.
func (g *game) drawWhatItKnows(t *textBox, view engine.HumanView) {
	// What the node knows about itself. This is the whole of what a decision
	// has to go on, which is the question this stage exists to answer.
	self := view.Self
	t.line("IT KNOWS (as of tick %d):", view.Tick)
	t.line("  vit %.1f/%.0f  hunger %.1f  crowding %.2f per meal",
		self.Vitality, self.MaxVitality, self.Hunger, self.FoodScarcity)
	t.line("  resting here mends %.3f/tick, exposure x%.2f", self.RestRate, self.Shelter)
	if self.AttackerID != 0 {
		t.line("  #%d is hitting it", self.AttackerID)
	}
	if self.BetterGround > 0 {
		t.line("  reckons the ground at %.0f,%.0f is %.1f better",
			self.BetterGroundX, self.BetterGroundY, self.BetterGround)
	}
	if !self.CanReproduce {
		// Why, not merely that. The three conditions are the node's own rule
		// read off its own state, and a player who is not told which one is
		// failing cannot do anything about it.
		cfg := g.world.Config()
		want := []string{}
		// What a body cannot do comes first, because it is the only part that
		// is a refusal rather than a judgement.
		if floor := cfg.BirthVitalityCost / 2; self.Vitality <= floor {
			want = append(want, fmt.Sprintf("vitality %.1f -> over %.1f, which is what a birth costs it",
				self.Vitality, floor))
		}
		if cfg.CourtNeedsSurplus {
			if self.Hunger >= cfg.ReproHunger {
				want = append(want, fmt.Sprintf("hunger %.1f -> under %.0f", self.Hunger, cfg.ReproHunger))
			}
			if floor := cfg.ReproVitalityShare * self.MaxVitality; self.Vitality < floor {
				want = append(want, fmt.Sprintf("vitality %.1f -> over %.1f", self.Vitality, floor))
			}
		}
		if len(want) == 0 {
			want = append(want, "it is still growing up, or has just had a child")
		}
		t.line("  CANNOT COURT: %s", strings.Join(want, "; "))
	} else {
		t.line("  it can court: feed it and it will be asked when it sees somebody")
	}
	t.line("")

	// Nearest first. The perception is in no particular order, and what a
	// player wants to know first is what is close enough to act on.
	others := append([]engine.AgentView(nil), view.Others...)
	sort.Slice(others, func(i, j int) bool { return others[i].Dist < others[j].Dist })
	meals := append([]engine.FoodView(nil), view.Foods...)
	sort.Slice(meals, func(i, j int) bool { return meals[i].Dist < meals[j].Dist })

	if len(others) == 0 {
		t.line("SEES NOBODY")
	} else {
		t.line("SEES %d (strength is its guess, never the truth):", len(others))
		for i, o := range others {
			if i >= 6 || t.roomLeft() < 8 {
				t.line("  ... and %d more", len(others)-i)
				break
			}
			tag := ""
			switch {
			case o.AttackingMe:
				tag = " HITTING IT"
			case o.Prey:
				tag = " prey"
			case o.Resting:
				tag = " asleep"
			case o.Paired:
				tag = " paired"
			}
			t.line("  #%-4d %3.0f away  %s str %4.1f+/-%4.1f  aff %4.1f%s",
				o.ID, o.Dist, o.Sex, o.EstStrength, math.Sqrt(o.Uncertainty), o.Affinity, tag)
		}
	}
	t.line("")

	if len(meals) == 0 {
		t.line("SEES NOTHING TO EAT")
		return
	}
	t.line("SEES %d meals:", len(meals))
	for i, f := range meals {
		if i >= 5 || t.roomLeft() < 2 {
			t.line("  ... and %d more", len(meals)-i)
			break
		}
		rival := ""
		if f.RivalID != 0 {
			rival = fmt.Sprintf("  #%d is %.0f away from it", f.RivalID, f.RivalDist)
		}
		t.line("  #%-4d %3.0f away  worth x%.2f%s", f.ID, f.Dist, f.Nutrition, rival)
	}
}

// drawAsked is the panel of the way of playing where the node decides and a
// person answers. What is on it is the comparison the node actually made: the
// options are its own, ranked the way it ranks them, with the misjudgement its
// intelligence put on each one already in the score. A player driving a dull
// node is offered a dull node's ranking, which is the point.
func (g *game) drawAsked(t *textBox) {
	asked, at, answered := g.guided.Asked()
	fresh := g.guided.Body() == g.played
	t.line("YOU ARE #%d   it decides, you answer", g.played)
	if a, alive := g.world.AgentByID(g.played); alive {
		t.line("it is %s (%s)   vit %.1f  hunger %.1f",
			a.State, describeAction(a.Action), a.Vitality, a.Hunger)
	}
	if g.selected != g.played && g.selected != 0 {
		// The block at the top of the panel is whatever was last clicked, and
		// while playing that is often somebody else entirely. With nothing
		// selected there is no block above this at all - the panel opens on
		// the game.
		t.line("(the block above is #%d, which you clicked. esc drops it)", g.selected)
	}
	if !fresh {
		t.line("not asked as #%d yet (the line has been asked %d times)", g.played, asked)
	} else {
		t.line("asked %d times, you answered %d   last on tick %d (%d ago)",
			asked, answered, at, g.world.Tick()-at)
	}
	g.drawGift(t)
	if d, ok := g.world.Disposition(g.played); ok {
		t.line("disposed: wary %.2f  rivalrous %.2f  fearful %.2f  (of %.2f)",
			d.Risk, d.Competition, d.Shock, d.Total())
	}
	// What the player is playing. None of it is in the utility formula: a node
	// is scored on its own life, and a line is the thing that outlives it. It
	// is the one thing a person has that the node does not, which is what
	// makes disagreeing with the node's own ranking a real thing to do.
	t.line("YOUR LINE: %d bod(ies), %d ticks, %d born, %d of them alive",
		g.bodies, g.lineAt-g.lineFrom, len(g.lineKids), g.livingKin())
	if kin := g.describeKin(); kin != "" {
		t.line("  %s", kin)
	}
	if g.heir != 0 {
		t.line("  it goes on through #%d", g.heir)
	} else {
		t.line("  no heir named (k). the line ends when this body does")
	}
	if v, ok := g.guided.View(); ok && fresh {
		g.drawCourting(t, v)
	}
	t.line("")
	g.drawLastChoice(t)

	// The question first: it is the one on a clock, and an offer that hid it
	// would stop the world for a reason the panel was not showing.
	if _, ok := g.world.Question(g.played); ok || g.offer == nil {
		g.drawQuestion(t)
		if g.offer != nil {
			t.line("(a milestone is waiting behind this)")
		}
	} else {
		g.drawOffer(t)
	}
	t.line("")

	view, ok := g.guided.View()
	if !ok || !fresh {
		t.line("it has not been asked anything yet, so it has")
		t.line("nothing to tell you.")
		return
	}
	g.drawWhatItKnows(t, view)
}

// drawSuccession prints the choice of body left by a death. What is shown of
// each one is what that body knows about itself - how far grown it is and what
// is left in it - because a player who takes it over is about to be it.
func (g *game) drawSuccession(t *textBox) {
	t.line("#%d IS DEAD.", g.succession.dead)
	t.line("")
	t.line("YOUR LINE: %d bod(ies), %d ticks, %d born, %d alive",
		g.bodies, g.lineAt-g.lineFrom, len(g.lineKids), g.livingKin())
	t.line("")
	t.line("go on as one of them?")
	for i, p := range g.succession.picks {
		if i >= len(choiceKeys) {
			break
		}
		t.line("  [%d] #%-5d %s", i+1, p.id, p.about)
	}
	t.line("  [enter] stop here: the line ends")
	t.line("")
	t.line("a child that is still growing cannot court and")
	t.line("expresses only part of what it inherited, and the")
	t.line("parent it kept close to is the one that just died.")
	t.line("it is a poor hand, not an impossible one.")
}

// drawCourting says what this body is worth to a mate and what it is holding
// out for, and how the last proposal went.
//
// Every figure here is the node's own: what others can see of its build and
// the shape it is in (which is all MateValue is made of), its own rule for
// accepting, and the two answers of a courtship it was standing in. What a
// candidate privately made of it is not here - that is their misjudgement, not
// its knowledge - which is why a refusal can still be a surprise.
func (g *game) drawCourting(t *textBox, view engine.HumanView) {
	cfg := g.world.Config()
	t.line("AS A MATE you are worth %.0f, and want %.0f; everybody wants %.0f",
		view.Self.MateValue, view.Self.MateBar, cfg.CommitFitness)
	t.line("  until their own patience runs out, then %.0f", cfg.CommitFloor)

	c := view.Self.LastCourt
	if c.TargetID == 0 {
		return
	}
	who := ""
	switch {
	case c.Accepted && c.TheyAccepted:
		who = "you both agreed"
	case c.Accepted && !c.TheyAccepted:
		who = "THEY said no"
	case !c.Accepted && c.TheyAccepted:
		who = "YOU said no"
	default:
		who = "you both said no"
	}
	t.line("LAST PROPOSAL #%d, %d ago: %s (you made them %.0f, wanted %.0f)",
		c.TargetID, view.Tick-c.Tick, who, c.Fitness, c.Bar)
}

// drawGift says what this body was given for being the one that is played.
// Everything else on the panel is the world; this line is not, and saying so is
// the whole reason it is a line rather than a silent adjustment.
func (g *game) drawGift(t *textBox) {
	if g.given <= 0 {
		return
	}
	a, ok := g.world.AgentByID(g.played)
	if !ok {
		return
	}
	cfg := g.world.Config()
	t.line("GIVEN +%.0f to be playable: speed %.0f, vitality %.0f/%.0f. not the world's doing",
		g.given, a.Gene(engine.GeneSpeed), a.MaxVitality(&cfg), cfg.BirthVitalityCost)
}

// drawQuestion prints the choice standing, if one is.
func (g *game) drawQuestion(t *textBox) {
	q, ok := g.world.Question(g.played)
	if !ok {
		t.line("NOTHING ASKED. it is getting on with it.")
		t.line("it stops and asks when something turns - or press")
		t.line("enter to ask it what it is thinking right now.")
		return
	}
	t.line("ASKED ON TICK %d: %s", q.Tick, q.Trigger)
	for i, b := range bets(q) {
		if i >= len(choiceKeys) || t.roomLeft() < 4 {
			break
		}
		o := q.Options[b.idx]
		t.line("  [%d] %s  %s", i+1, b.label, describeAction(o.Action))
		t.line("      for   %s", goalTerms(o.Utility))
		cost := fmt.Sprintf("costs %.1f vit over %.0f ticks", o.Utility.Vitality, o.Utility.Ticks)
		if o.Utility.Risk != 0 {
			cost += fmt.Sprintf("; it has cost you %.1f before", o.Utility.Risk)
		}
		t.line("      %s", cost)
	}
	t.line("  [enter] leave it to itself (it has already started)")
	// No total, on purpose. The node's own ranking is still there for anyone
	// who wants it - that is what leaving it to itself is - but a list with
	// "best" written next to one line is not a decision a person makes.
}

// drawOffer prints the split waiting to be made at a milestone.
func (g *game) drawOffer(t *textBox) {
	d, _ := g.world.Disposition(g.played)
	total := d.Total()
	t.line("A MILESTONE: %s", g.offer.why)
	t.line("what it wants out of life, in %.2f shares between three.", total)
	t.line("more care has to come out of the other two.")
	for i, p := range g.offer.picks {
		if i >= len(choiceKeys) {
			break
		}
		got := scaled(p.want, total)
		t.line("  [%d] %-17s wary %.2f  rivalrous %.2f  fearful %.2f",
			i+1, p.label, got.Risk, got.Competition, got.Shock)
	}
	t.line("  [enter] leave it as it is")
}

// describeMark says what an order would be aimed at.
func (g *game) describeMark() string {
	switch g.mark.kind {
	case markAgent:
		if _, ok := g.world.AgentByID(g.mark.id); !ok {
			return fmt.Sprintf("#%d, who is gone", g.mark.id)
		}
		return fmt.Sprintf("node #%d", g.mark.id)
	case markFood:
		if _, _, ok := g.markPos(); !ok {
			return fmt.Sprintf("meal #%d, which is gone", g.mark.id)
		}
		return fmt.Sprintf("meal #%d", g.mark.id)
	case markSpot:
		return fmt.Sprintf("the ground at %.0f,%.0f", g.mark.x, g.mark.y)
	}
	return "nothing yet (click)"
}

// drawDecision prints one recorded decision: what prompted it, every option it
// weighed up with the terms behind the score, and which one it took.
func (g *game) drawDecision(t *textBox) {
	traces := g.world.DecisionTraces(g.selected)
	if len(traces) == 0 {
		t.line("no decision recorded yet.")
		t.line("deciding is trigger driven, so nothing happens")
		t.line("until something prompts it: food coming into")
		t.line("sight, a blow landing, or a goal being reached.")
		return
	}

	g.traceBack = min(g.traceBack, len(traces)-1)
	tr := traces[len(traces)-1-g.traceBack]

	t.line("DECISION %d of %d   tick %d (%d ticks ago)",
		len(traces)-g.traceBack, len(traces), tr.Tick, g.world.Tick()-tr.Tick)
	t.line("asked because: %s", tr.Trigger)
	t.line("at the time: vit %.1f  hun %.1f  scarcity %.2f",
		tr.Self.Vitality, tr.Self.Hunger, tr.Self.FoodScarcity)
	t.line("took: %s", describeAction(tr.Action))
	t.line("")

	if len(tr.Options) == 0 {
		t.line("(this controller does not report its options)")
		return
	}

	// Best first, which is not the order they were scored in.
	order := make([]int, len(tr.Options))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		return tr.Options[order[i]].Score > tr.Options[order[j]].Score
	})

	shown := min(len(order), t.roomLeft()/3)
	t.line("compared %d options, best first:", len(tr.Options))
	for _, i := range order[:shown] {
		o := tr.Options[i]
		mark := "  "
		if i == tr.Chosen {
			mark = "=>"
		}
		noise := ""
		if o.Noise != 0 {
			noise = fmt.Sprintf("  (%+.2f misjudged)", o.Noise)
		}
		t.line("%s %-22s %8.3f%s", mark, describeAction(o.Action), o.Score, noise)
		t.line("      %s", goalTerms(o.Utility))
		t.line("      %s", costTerms(o.Utility))
	}
	if rest := len(order) - shown; rest > 0 {
		t.line("   ... and %d worse", rest)
	}
}

// goalTerms is the "what it is worth x how likely" half of the score.
func goalTerms(u engine.Utility) string {
	goals := u.Goals()
	if len(goals) == 0 {
		return "no goal served"
	}
	parts := make([]string, 0, len(goals))
	for _, g := range goals {
		parts = append(parts, fmt.Sprintf("%s %.2fx%.2f=%.2f", g.Name, g.Value, g.Chance, g.Score()))
	}
	return strings.Join(parts, "  ")
}

// costTerms is what the option costs: vitality, time, and being wary of
// somebody who has hurt this agent before.
func costTerms(u engine.Utility) string {
	s := fmt.Sprintf("cost vit %.2f=-%.2f  time %.0ft=-%.2f", u.Vitality, u.VitalityCost, u.Ticks, u.TimeCost)
	if u.Risk != 0 {
		s += fmt.Sprintf("  risk -%.2f", u.Risk)
	}
	// What this node's own rules of thumb made of the option. It is neither a
	// goal nor a cost: it stands for nothing in particular, which is the
	// point of it.
	if u.Hint != 0 {
		s += fmt.Sprintf("  hunch %+.2f", u.Hint)
	}
	return s
}

func describeAction(a engine.Action) string {
	switch a.Kind {
	case engine.ActRest:
		return "rest"
	case engine.ActMove:
		return fmt.Sprintf("move %+.1f%+.1f e%.2f", a.DX, a.DY, a.Effort)
	default:
		return fmt.Sprintf("%s #%d e%.2f", a.Kind, a.TargetID, a.Effort)
	}
}

// drawLooksSense prints what the node has made of appearance: the line it has
// fitted for itself from every strength it has ever read, and what that line
// says about somebody it has never met. This is the half of its beliefs that
// is not about anybody in particular.
func (g *game) drawLooksSense(t *textBox) {
	ls := g.world.LooksSense(g.selected)
	if !ls.Trusted {
		t.line("sizing up strangers: %d readings, not enough to go by yet;", ls.Readings)
		t.line("  a stranger is worth the flat prior %.0f to it", ls.Guess)
		t.line("")
		return
	}
	t.line("sizing up strangers: %d readings -> an average build is worth", ls.Readings)
	t.line("  %.1f to it, %+.2f per point of build above that", ls.Guess, ls.Slope)
	t.line("  (this is its own line, fitted from what it has seen)")
	t.line("")
}

// drawBeliefs prints what the selected node reckons about everybody it has met:
// how strong they are, how sure it is, and what they have already cost it.
func (g *game) drawBeliefs(t *textBox) {
	g.drawLooksSense(t)

	opinions := g.world.Opinions(g.selected)
	if len(opinions) == 0 {
		t.line("has met nobody yet")
		return
	}

	ids := make([]int, 0, len(opinions))
	for id := range opinions {
		ids = append(ids, id)
	}
	// Most talked about first: the ones it has the most readings on.
	sort.Slice(ids, func(i, j int) bool {
		if opinions[ids[i]].Samples != opinions[ids[j]].Samples {
			return opinions[ids[i]].Samples > opinions[ids[j]].Samples
		}
		return ids[i] < ids[j]
	})

	cfg := g.world.Config()
	t.line("believes about others (true power in brackets):")
	t.line("  aff is what it remembers them doing for it: a bond, a birth,")
	t.line("  being its parent or its child, or having helped bring a")
	t.line("  carcass down. it will rest next to those.")
	for i, id := range ids {
		if i >= maxOpinionRows {
			t.line("  ... and %d more", len(ids)-maxOpinionRows)
			break
		}
		op := opinions[id]
		truth := "gone"
		if other, ok := g.world.AgentByID(id); ok {
			truth = fmt.Sprintf("%.0f", other.Attack(&cfg))
		}
		t.line("  #%-4d str %5.1f+/-%5.1f  risk %5.1f  aff %5.1f  seen %2d [%s]",
			id, op.Strength, math.Sqrt(op.Variance), op.Risk, op.Affinity, op.Samples, truth)
	}
}

func (g *game) Layout(int, int) (int, int) {
	return screenWidth, screenHeight
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// quickestBody is the node a game starts on when nobody named one.
//
// It used to be whichever agent happened to be first in the list, and that is
// how a player ended up driving a body that had spent five per cent of its
// budget on being quick - a quarter of what an ordinary one spends - and
// concluded that the world was slow. Speed is the gene a player feels every
// second of a game, so the default body is a quick one.
//
// Quick, but not merely the quickest. The budget is fixed, so the very fastest
// body in a population is usually the one that bought its speed with
// everything else, and the first one picked that way died in ninety-six ticks.
// So: the quickest of the sounder half. That is two ranks and no thresholds,
// which is what keeps it working as the population's allocations drift - there
// is always a sounder half, and always a quickest in it.
//
// Two requirements sit in front of the ranks, and both are the game's own
// rather than invented numbers.
//
//   - It has to be able to pay for a birth and still be alive. The first pick
//     that ignored this had a vitality of twenty against a birth costing
//     twenty, so two fifths of its life it could not court at all, and the
//     line it was supposed to carry could never start.
//   - Somebody has to be willing to have it. Nearly two thirds of how good a
//     mate an agent looks is a gene, and a body picked for speed and budget
//     can easily have almost none of it: the second pick courted two hundred
//     times and was accepted three. So the looks have to be at least the
//     population's own middle.
//
// Both are ranks or thresholds read off the world, not constants: the median
// moves with the population, and what a birth costs is a rule.
func quickestBody(w *engine.World) int {
	cfg := w.Config()
	var grown []engine.Agent
	for _, a := range w.Agents() {
		if a.Species != engine.SpeciesHuman || !a.IsAdult(&cfg) {
			continue
		}
		grown = append(grown, a)
	}
	if len(grown) == 0 {
		if agents := w.Agents(); len(agents) > 0 {
			return agents[0].ID
		}
		return 0
	}
	middling := medianLooks(grown)

	// Each filter is dropped rather than allowed to leave nothing.
	pool := filterAgents(grown, func(a *engine.Agent) bool {
		return a.MaxVitality(&cfg) >= cfg.BirthVitalityCost
	})
	if len(pool) == 0 {
		pool = grown
	}
	if better := filterAgents(pool, func(a *engine.Agent) bool {
		return a.Gene(engine.GeneAttractiveness) >= middling
	}); len(better) > 0 {
		pool = better
	}

	sort.Slice(pool, func(i, j int) bool { return pool[i].Budget() > pool[j].Budget() })
	sound := pool[:max(len(pool)/2, 1)]
	best, bestSpeed := sound[0].ID, -1.0
	for i := range sound {
		if s := sound[i].MaxSpeed(&cfg); s > bestSpeed {
			best, bestSpeed = sound[i].ID, s
		}
	}
	return best
}

func filterAgents(in []engine.Agent, keep func(*engine.Agent) bool) []engine.Agent {
	out := make([]engine.Agent, 0, len(in))
	for i := range in {
		if keep(&in[i]) {
			out = append(out, in[i])
		}
	}
	return out
}

// medianLooks is the middle of what the population has spent on being worth
// looking at. Read live, so it goes on meaning the same thing as the gene
// drifts.
func medianLooks(in []engine.Agent) float64 {
	v := make([]float64, len(in))
	for i := range in {
		v[i] = in[i].Gene(engine.GeneAttractiveness)
	}
	sort.Float64s(v)
	if len(v) == 0 {
		return 0
	}
	return v[len(v)/2]
}

func main() {
	follow := flag.Int("follow", 0, "node ID to follow from the start (0 for none; nodes can also be clicked)")
	seed := flag.Int64("seed", engine.DefaultConfig().Seed, "simulation seed")
	slow := flag.Bool("slow", false, "start at 1/5 speed, for following a single node")
	beliefs := flag.Bool("beliefs", false, "start on the beliefs panel rather than the decision one (tab switches)")
	play := flag.Bool("play", false, "play a node yourself: -follow picks it, otherwise the quickest body in the world (same as pressing h)")
	ask := flag.Bool("ask", false, "play it the other way: the node decides for itself and asks you at the turning points (h twice)")
	boost := flag.Bool("boost", true, "bring the played body up to the world's average speed, paid for with new budget (a gift, shown on the panel)")
	flag.Parse()

	cfg := engine.DefaultConfig()
	cfg.Width, cfg.Height = worldWidth, worldHeight
	cfg.Seed = *seed

	// Effort 1.0 to start with. Walking flat out costs MoveCost per tick and
	// empties an ordinary body in about half a minute of it, so it is a real
	// choice rather than a free setting - but starting below it only made the
	// game feel slow for a reason no player could see.
	g := &game{world: engine.NewWorld(cfg), speed: normalSpeed, effort: 1.0, padKey: noKey}
	g.boost = *boost
	if *slow {
		g.speed = 1
	}
	g.selectAgent(*follow)
	if *beliefs {
		g.mode = modeBeliefs
	}
	if *play || *ask {
		if g.selected == 0 {
			g.selectAgent(quickestBody(g.world))
		}
		g.toggleControl() // the first press is the asked mode
		if *play {
			g.toggleControl() // and the second takes the reins
		}
	}

	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("devview - human behaviour simulation")
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
