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
	colorTarget     = color.RGBA{0x11, 0x11, 0x11, 0x60}
	colorHungerBar  = color.RGBA{0xc9, 0x8a, 0x20, 0xff}
	colorTail       = color.RGBA{0x44, 0x44, 0x77, 0xb0}
)

// panelMode is what the right hand panel shows about the selected node.
type panelMode uint8

const (
	modeDecision panelMode = iota // the utility comparison behind its last moves
	modeBeliefs                   // what it reckons about everybody it has met
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
}

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
		g.mode = 1 - g.mode
	case inpututil.IsKeyJustPressed(ebiten.KeyBracketLeft):
		g.traceBack++ // further back in time
	case inpututil.IsKeyJustPressed(ebiten.KeyBracketRight):
		g.traceBack = max(g.traceBack-1, 0)
	case inpututil.IsKeyJustPressed(ebiten.KeyEscape):
		g.selectAgent(0)
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		if mx < worldWidth {
			g.selectAgent(g.nodeAt(mx, my))
		}
	}
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
	g.drawSight(screen)

	for _, f := range g.world.Foods() {
		vector.DrawFilledCircle(screen, float32(f.X), float32(f.Y), 3, colorFood, true)
	}

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
	fmt.Fprintf(&b, "tick %d  pop %d (m %d / f %d)  food %d  births %d  deaths %d (kills %d)  gen %d  [%s]\n",
		s.Tick, s.Population, s.Males, s.Females, s.FoodItems, s.Births, s.Deaths, s.Kills, s.MaxGeneration, state)
	fmt.Fprintf(&b, "avg power %.1f  rationality %.1f  intelligence %.1f  vitality %.1f  hunger %.1f\n",
		s.AvgPower, s.AvgRationality, s.AvgIntelligence, s.AvgVitality, s.AvgHunger)
	b.WriteString("circle = body (outline its size, fill what is left in it), tail = speed, ring width = attack, bar = hunger\n")
	b.WriteString("ring: grey forage, orange mate, green paired, red fighting, purple fleeing, blue resting\n")
	b.WriteString("children are small circles: a newborn expresses 60% of its genes and grows into the rest by eating\n")
	b.WriteString("space pause   right/n one tick   -/= slower/faster   click a node   esc clear\n")
	b.WriteString("tab decisions/beliefs   [ ] older/newer decision\n")
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
		t.line("state %s   doing %s", a.State, describeAction(a.Action))
		t.line("parents %v  children %v", a.ParentIDs, a.ChildIDs)
	} else {
		t.line("#%d is gone. its last decisions are below.", g.selected)
	}
	t.line("")

	if g.mode == modeBeliefs {
		g.drawBeliefs(t)
		return
	}
	g.drawDecision(t)
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
	flag.Parse()

	cfg := engine.DefaultConfig()
	cfg.Width, cfg.Height = worldWidth, worldHeight
	cfg.Seed = *seed

	g := &game{world: engine.NewWorld(cfg), speed: normalSpeed}
	if *slow {
		g.speed = 1
	}
	g.selectAgent(*follow)
	if *beliefs {
		g.mode = modeBeliefs
	}

	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("devview - human behaviour simulation")
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
