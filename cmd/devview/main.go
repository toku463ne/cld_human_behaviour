// Command devview is a development tool: it imports the engine directly (no
// network involved) and draws it, so that a new rule can be watched right after
// it is written. It is not the real client, which will render snapshots
// received from the server.
package main

import (
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
	screenWidth  = 900
	screenHeight = 650

	// The simulation tick rate is independent from the frame rate: several
	// ticks are simulated per drawn frame.
	ticksPerFrame = 2

	minRadius   = 3.5
	maxRadius   = 11.0
	minRingSize = 1.0
	maxRingSize = 4.0

	// How close a click has to land to select a node.
	pickRadius = 14.0

	// How many of a selected agent's opinions to list.
	maxOpinionRows = 8
)

var (
	colorBackground = color.RGBA{0xfc, 0xfc, 0xfb, 0xff}
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
	colorHungerBar  = color.RGBA{0xc9, 0x8a, 0x20, 0xff}
)

type game struct {
	world *engine.World

	paused   bool
	selected int // agent ID, 0 for none
}

func (g *game) Update() error {
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		g.paused = !g.paused
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		g.pick(ebiten.CursorPosition())
	}
	if g.paused {
		return nil
	}
	for i := 0; i < ticksPerFrame; i++ {
		g.world.Step()
	}
	return nil
}

// pick selects the node under the cursor, so that its view of everybody else
// can be inspected.
func (g *game) pick(mx, my int) {
	best, bestDist := 0, pickRadius*pickRadius
	for _, a := range g.world.Agents() {
		dx, dy := a.X-float64(mx), a.Y-float64(my)
		if d := dx*dx + dy*dy; d < bestDist {
			bestDist, best = d, a.ID
		}
	}
	g.selected = best
}

func (g *game) Draw(screen *ebiten.Image) {
	screen.Fill(colorBackground)

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

	for i := range agents {
		a := &agents[i]
		// Radius shows vitality, ring width shows power, ring colour shows
		// what the agent is currently doing, and the notch below it is hunger.
		radius := float32(minRadius + a.Vitality/100*(maxRadius-minRadius))
		ringWidth := float32(minRingSize + a.Power/100*(maxRingSize-minRingSize))

		fill := colorMale
		if a.Sex == engine.Female {
			fill = colorFemale
		}

		x, y := float32(a.X), float32(a.Y)
		vector.DrawFilledCircle(screen, x, y, radius, fill, true)
		vector.StrokeCircle(screen, x, y, radius, ringWidth, stateColor(a.State), true)

		if hunger := float32(a.Hunger / 100); hunger > 0.01 {
			vector.StrokeLine(screen, x-6, y+radius+3, x-6+12*hunger, y+radius+3, 2, colorHungerBar, true)
		}
		if a.ID == g.selected {
			vector.StrokeCircle(screen, x, y, radius+5, 1.5, colorSelected, true)
		}
	}

	ebitenutil.DebugPrint(screen, g.overlay())
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

	pausedNote := ""
	if g.paused {
		pausedNote = "  [PAUSED]"
	}
	fmt.Fprintf(&b, "tick %d  pop %d (m %d / f %d)  food %d  births %d  deaths %d (kills %d)  gen %d%s\n",
		s.Tick, s.Population, s.Males, s.Females, s.FoodItems, s.Births, s.Deaths, s.Kills, s.MaxGeneration, pausedNote)
	fmt.Fprintf(&b, "avg power %.1f  rationality %.1f  intelligence %.1f  vitality %.1f  hunger %.1f\n",
		s.AvgPower, s.AvgRationality, s.AvgIntelligence, s.AvgVitality, s.AvgHunger)
	b.WriteString("radius = vitality, ring width = power, bar = hunger; blue male / pink female\n")
	b.WriteString("ring: grey forage, orange mate, green paired, red fighting, purple fleeing, blue resting\n")
	b.WriteString("space pauses, click a node to see what it believes about the others\n")

	g.describeSelected(&b)
	return b.String()
}

// describeSelected prints the selected agent and, below it, what it reckons
// about everybody it has met: how strong they are, how sure it is, and what
// they have already cost it.
func (g *game) describeSelected(b *strings.Builder) {
	if g.selected == 0 {
		return
	}
	a, ok := g.world.AgentByID(g.selected)
	if !ok {
		g.selected = 0
		return
	}

	fmt.Fprintf(b, "\n#%d %s gen %d  vit %.0f hun %.0f  pow %.0f rat %.0f int %.0f  %s(%s)\n",
		a.ID, a.Sex, a.Generation, a.Vitality, a.Hunger,
		a.Power, a.Rationality, a.Intelligence, a.State, a.Action.Kind)
	fmt.Fprintf(b, "parents %v  children %v\n", a.ParentIDs, a.ChildIDs)

	opinions := g.world.Opinions(a.ID)
	if len(opinions) == 0 {
		b.WriteString("has met nobody yet\n")
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

	b.WriteString("believes about others (true power in brackets):\n")
	for i, id := range ids {
		if i >= maxOpinionRows {
			fmt.Fprintf(b, "  ... and %d more\n", len(ids)-maxOpinionRows)
			break
		}
		op := opinions[id]
		truth := "gone"
		if other, ok := g.world.AgentByID(id); ok {
			truth = fmt.Sprintf("%.0f", other.Power)
		}
		fmt.Fprintf(b, "  #%-4d strength %5.1f +/- %5.1f  risk %5.1f  seen %2d  [%s]\n",
			id, op.Strength, math.Sqrt(op.Variance), op.Risk, op.Samples, truth)
	}
}

func (g *game) Layout(int, int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	cfg := engine.DefaultConfig()
	cfg.Width, cfg.Height = screenWidth, screenHeight

	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("devview - human behaviour simulation")
	if err := ebiten.RunGame(&game{world: engine.NewWorld(cfg)}); err != nil {
		log.Fatal(err)
	}
}
