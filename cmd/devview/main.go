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
	played  int
	human   *engine.HumanController
	effort  float64
	stance  engine.Stance
	mark    mark
	heir    int // the child the line is to continue through, 0 for none
	notice  string
	noticed int // tick the notice was put up
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

func (g *game) Update() error {
	g.handleInput()
	if g.paused {
		return nil
	}
	g.tickAccum += speeds[g.speed].ticks
	for g.tickAccum >= 1 {
		g.world.Step()
		g.tickAccum--
	}
	g.carryTheLineOn()
	return nil
}

func (g *game) handleInput() {
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeySpace):
		g.paused = !g.paused
	case inpututil.IsKeyJustPressed(ebiten.KeyRight), inpututil.IsKeyJustPressed(ebiten.KeyN):
		// One tick, and stay stopped: this is how a single decision gets read.
		g.paused = true
		g.tickAccum = 0
		g.world.Step()
	case inpututil.IsKeyJustPressed(ebiten.KeyMinus):
		g.speed = max(g.speed-1, 0)
	case inpututil.IsKeyJustPressed(ebiten.KeyEqual):
		g.speed = min(g.speed+1, len(speeds)-1)
	case inpututil.IsKeyJustPressed(ebiten.KeyTab):
		g.mode = panelMode((int(g.mode) + 1) % numPanelModes)
	case inpututil.IsKeyJustPressed(ebiten.KeyBracketLeft):
		g.traceBack++ // further back in time
	case inpututil.IsKeyJustPressed(ebiten.KeyBracketRight):
		g.traceBack = max(g.traceBack-1, 0)
	case inpututil.IsKeyJustPressed(ebiten.KeyEscape):
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
			if g.played != 0 {
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
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyH):
		g.toggleControl()
	case inpututil.IsKeyJustPressed(ebiten.KeyK):
		g.pickHeir()
	}
	if g.played == 0 {
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
}

var effortKeys = []ebiten.Key{ebiten.Key1, ebiten.Key2, ebiten.Key3, ebiten.Key4, ebiten.Key5}

// toggleControl takes the selected node over, or hands it back to the AI.
func (g *game) toggleControl() {
	if g.played != 0 {
		id := g.played
		g.world.SetController(id, nil) // nil is the world's own AI again
		g.played, g.human, g.heir = 0, nil, 0
		g.say("#%d is back on the utility formula", id)
		return
	}
	if g.selected == 0 {
		g.say("click a node first, then press h to take it over")
		return
	}
	g.human = engine.NewHumanController()
	if !g.world.SetController(g.selected, g.human) {
		g.human = nil
		g.say("#%d is gone", g.selected)
		return
	}
	g.played = g.selected
	g.mode = modePlay
	g.say("you are #%d. it rests until told otherwise", g.played)
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

// carryTheLineOn moves the player to the named heir when the played node dies.
// This is the whole of "switching to a child": the same controller, installed
// on somebody else. The world never learns that anything changed hands.
func (g *game) carryTheLineOn() {
	if g.played == 0 {
		return
	}
	if _, alive := g.world.AgentByID(g.played); alive {
		return
	}
	dead := g.played
	if g.heir != 0 && g.world.SetController(g.heir, g.human) {
		g.played = g.heir
		g.heir = 0
		g.mark = mark{}
		g.selectAgent(g.played)
		g.say("#%d died. you are #%d now", dead, g.played)
		return
	}
	g.played, g.human, g.heir = 0, nil, 0
	g.say("#%d died with nobody named to follow it. the line ends", dead)
}

// aimAt points the order at whatever was clicked: somebody, something to eat,
// or a patch of ground to walk to.
func (g *game) aimAt(mx, my int) {
	if id := g.nodeAt(mx, my); id != 0 && id != g.played {
		g.mark = mark{kind: markAgent, id: id}
		return
	}
	if f, ok := g.foodAt(mx, my); ok {
		g.mark = mark{kind: markFood, id: f.ID, x: f.X, y: f.Y}
		return
	}
	g.mark = mark{kind: markSpot, x: float64(mx), y: float64(my)}
}

// foodAt returns the food item under the cursor.
func (g *game) foodAt(mx, my int) (engine.Food, bool) {
	best, bestDist := engine.Food{}, pickRadius*pickRadius
	found := false
	for _, f := range g.world.Foods() {
		dx, dy := f.X-float64(mx), f.Y-float64(my)
		if d := dx*dx + dy*dy; d < bestDist {
			bestDist, best, found = d, f, true
		}
	}
	return best, found
}

// order hands the standing order to the controller and reports what came of it.
// A refusal is worth showing rather than swallowing: it is the engine saying
// the node could not have come up with that itself.
func (g *game) order(a engine.Action) {
	if g.human == nil {
		return
	}
	a.Effort = g.effort
	a.Stance = g.stance
	if err := g.human.Order(a); err != nil {
		g.say("no: %v", err)
		return
	}
	g.say("order: %s. it will do that when next asked (enter to ask now)", describeAction(g.human.Standing()))
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
	g.order(engine.Action{Kind: engine.ActMove, DX: x - a.X, DY: y - a.Y})
}

// orderAt aims an action at the mark, when the mark is the right kind of thing
// for it.
func (g *game) orderAt(kind engine.ActionKind, want markKind) {
	if g.mark.kind != want {
		what := "somebody"
		if want == markFood {
			what = "something to eat"
		}
		g.say("%s needs %s: click one first", kind, what)
		return
	}
	g.order(engine.Action{Kind: kind, TargetID: g.mark.id})
}

// markPos is where the mark is now. An agent moves, so its mark is followed
// rather than remembered.
func (g *game) markPos() (x, y float64, ok bool) {
	switch g.mark.kind {
	case markAgent:
		if a, live := g.world.AgentByID(g.mark.id); live {
			return a.X, a.Y, true
		}
	case markFood:
		for _, f := range g.world.Foods() {
			if f.ID == g.mark.id {
				return f.X, f.Y, true
			}
		}
	case markSpot:
		return g.mark.x, g.mark.y, true
	}
	return 0, 0, false
}

func (g *game) say(format string, args ...any) {
	g.notice = fmt.Sprintf(format, args...)
	g.noticed = g.world.Tick()
}

// nodeAt returns the node under the cursor, or 0.
func (g *game) nodeAt(mx, my int) int {
	best, bestDist := 0, pickRadius*pickRadius
	for _, a := range g.world.Agents() {
		dx, dy := a.X-float64(mx), a.Y-float64(my)
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

	vector.DrawFilledRect(screen, panelX, 0, panelWidth, screenHeight, colorPanel, false)
	vector.StrokeLine(screen, panelX, 0, panelX, screenHeight, 1, colorPanelEdge, false)

	ebitenutil.DebugPrint(screen, g.overlay())
	g.drawPanel(screen)
}

func (g *game) drawWorld(screen *ebiten.Image) {
	g.drawRegions(screen)
	g.drawSight(screen)

	for _, f := range g.world.Foods() {
		vector.DrawFilledCircle(screen, float32(f.X), float32(f.Y), 3, colorFood, true)
	}

	g.drawAim(screen)

	agents := g.world.Agents()

	// Bonds, drawn once per pair, and every blow being thrown.
	for i := range agents {
		a := &agents[i]
		if a.PartnerID > a.ID {
			if p, ok := g.world.AgentByID(a.PartnerID); ok {
				vector.StrokeLine(screen, float32(a.X), float32(a.Y), float32(p.X), float32(p.Y), 1, colorPairLink, true)
			}
		}
		if a.Action.Kind == engine.ActAttack {
			if t, ok := g.world.AgentByID(a.Action.TargetID); ok {
				vector.StrokeLine(screen, float32(a.X), float32(a.Y), float32(t.X), float32(t.Y), 1.5, colorFightLink, true)
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
		radius := float32(minRadius + capacity/150*(maxRadius-minRadius))
		filled := radius
		if capacity > 0 {
			filled = radius * float32(clamp01(a.Vitality/capacity))
		}
		ringWidth := float32(minRingSize + a.Attack(&cfg)/100*(maxRingSize-minRingSize))

		fill := colorMale
		if a.Sex == engine.Female {
			fill = colorFemale
		}

		x, y := float32(a.X), float32(a.Y)

		// A tail behind it, as long as the agent is quick. Speed is otherwise
		// invisible: two agents standing still look the same however much one
		// of them spent on being fast.
		if speed := a.MaxSpeed(&cfg); speed > 0 {
			tail := float32(speed / cfg.MaxSpeed * 13)
			vx, vy := float32(a.VX), float32(a.VY)
			if l := float32(math.Hypot(float64(vx), float64(vy))); l > 1e-6 {
				vx, vy = vx/l, vy/l
				vector.StrokeLine(screen, x-vx*tail, y-vy*tail, x, y, 1.5, colorTail, true)
			}
		}

		vector.DrawFilledCircle(screen, x, y, filled, fill, true)
		vector.StrokeCircle(screen, x, y, radius, ringWidth, stateColor(a.State), true)

		if hunger := float32(a.Hunger / 100); hunger > 0.01 {
			vector.StrokeLine(screen, x-6, y+radius+3, x-6+12*hunger, y+radius+3, 2, colorHungerBar, true)
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
		w, h := float32(r.MaxX-r.MinX), float32(r.MaxY-r.MinY)

		// Fill: how well the ground grows plants. Green for better than an
		// equal share, brown for worse, nothing at all for ordinary, so a
		// world without regions stays blank.
		if shade := uint8(clamp01(math.Abs(r.Food-1)/0.6) * 55); shade > 0 {
			fill := color.RGBA{0x30, 0x70, 0x30, shade} // rich
			if r.Food < 1 {
				fill = color.RGBA{0x80, 0x60, 0x20, shade} // thin
			}
			vector.DrawFilledRect(screen, float32(r.MinX), float32(r.MinY), w, h, fill, false)
		}

		// Border: how sheltered the resting is. A thick edge is ground with
		// its back covered.
		if r.Shelter < 1 {
			thick := float32(1 + clamp01((1-r.Shelter)/0.6)*3)
			vector.StrokeRect(screen, float32(r.MinX), float32(r.MinY), w, h, thick, colorRegionEdge, false)
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
	vector.StrokeRect(screen, float32(minX), float32(minY),
		float32(maxX-minX), float32(maxY-minY), 2, colorSight, false)
}

// markTarget rings whatever the selected node is currently acting on, so that
// the target named in the trace can be found on the map.
func (g *game) markTarget(screen *ebiten.Image, a *engine.Agent) {
	switch a.Action.Kind {
	case engine.ActEat:
		for _, f := range g.world.Foods() {
			if f.ID == a.Action.TargetID {
				vector.StrokeCircle(screen, float32(f.X), float32(f.Y), 7, 1.5, colorTarget, true)
			}
		}
	case engine.ActAttack, engine.ActFlee, engine.ActObserve, engine.ActCourt:
		if t, ok := g.world.AgentByID(a.Action.TargetID); ok {
			vector.StrokeCircle(screen, float32(t.X), float32(t.Y), 14, 1.5, colorTarget, true)
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
	fx, fy := float32(x), float32(y)
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
	b.WriteString("space pause   right/n one tick   -/= slower/faster   click a node   esc clear\n")
	b.WriteString("tab decisions/beliefs/play   [ ] older/newer decision   h play the selected node\n")
	if g.played != 0 {
		fmt.Fprintf(&b, "playing #%d: click to aim   r rest  m move  e eat  a attack  f flee  o observe  c court   1-5 effort  s stance   enter ask now  k heir\n",
			g.played)
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
	if g.played == 0 {
		t.line("nobody is being played.")
		t.line("")
		t.line("click a node and press h to take it over. it")
		t.line("then answers with whatever you last ordered,")
		t.line("the next time the world asks it anything.")
		t.line("")
		t.line("keys once you have one:")
		t.line("  click   aim at somebody, something to eat, a spot")
		t.line("  r rest   m move to the mark   e eat   a attack")
		t.line("  f flee   o observe            c court")
		t.line("  1-5 effort   s stance   enter think again now")
		t.line("  k name the child to carry on   h hand back to AI")
		return
	}

	asked, at := g.human.Asked()
	// Everything the controller holds - the order, the view, the count - still
	// belongs to the previous body until this one has been asked something.
	fresh := g.human.Body() == g.played
	t.line("YOU ARE #%d   effort %.1f   stance %s", g.played, g.effort, g.stance)
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
	if v := g.human.Voided(); v > 0 {
		t.line("%d order(s) lapsed: what they were aimed at was gone", v)
	}
	t.line("aim: %s", g.describeMark())
	if g.heir != 0 {
		t.line("line continues through #%d", g.heir)
	} else {
		t.line("no heir named (k). the line ends when this body does")
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
		t.line("  not in a state to court")
	}
	t.line("")

	if len(view.Others) == 0 {
		t.line("SEES NOBODY")
	} else {
		t.line("SEES %d (strength is its guess, never the truth):", len(view.Others))
		for i, o := range view.Others {
			if i >= 6 || t.roomLeft() < 8 {
				t.line("  ... and %d more", len(view.Others)-i)
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

	if len(view.Foods) == 0 {
		t.line("SEES NOTHING TO EAT")
		return
	}
	t.line("SEES %d meals:", len(view.Foods))
	for i, f := range view.Foods {
		if i >= 5 || t.roomLeft() < 2 {
			t.line("  ... and %d more", len(view.Foods)-i)
			break
		}
		rival := ""
		if f.RivalID != 0 {
			rival = fmt.Sprintf("  #%d is %.0f away from it", f.RivalID, f.RivalDist)
		}
		t.line("  #%-4d %3.0f away  worth x%.2f%s", f.ID, f.Dist, f.Nutrition, rival)
	}
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

func main() {
	follow := flag.Int("follow", 0, "node ID to follow from the start (0 for none; nodes can also be clicked)")
	seed := flag.Int64("seed", engine.DefaultConfig().Seed, "simulation seed")
	slow := flag.Bool("slow", false, "start at 1/5 speed, for following a single node")
	beliefs := flag.Bool("beliefs", false, "start on the beliefs panel rather than the decision one (tab switches)")
	play := flag.Bool("play", false, "drive the followed node yourself from the start (same as pressing h)")
	flag.Parse()

	cfg := engine.DefaultConfig()
	cfg.Width, cfg.Height = worldWidth, worldHeight
	cfg.Seed = *seed

	g := &game{world: engine.NewWorld(cfg), speed: normalSpeed, effort: 0.6}
	if *slow {
		g.speed = 1
	}
	g.selectAgent(*follow)
	if *beliefs {
		g.mode = modeBeliefs
	}
	if *play {
		if g.selected == 0 {
			// Somebody has to be picked, and the first node is as good as any.
			if agents := g.world.Agents(); len(agents) > 0 {
				g.selectAgent(agents[0].ID)
			}
		}
		g.toggleControl()
	}

	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("devview - human behaviour simulation")
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
