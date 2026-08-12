// Command devview is a development tool: it imports the engine directly (no
// network involved) and draws it, so that a new rule can be watched right after
// it is written. It is not the real client, which will render snapshots
// received from the server.
package main

import (
	"fmt"
	"image/color"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
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
	maxRadius   = 12.0
	minRingSize = 1.0
	maxRingSize = 4.0
)

var (
	colorBackground = color.RGBA{0xfc, 0xfc, 0xfb, 0xff}
	colorFood       = color.RGBA{0x1b, 0xaf, 0x7a, 0xff}
	colorMale       = color.RGBA{0x2a, 0x78, 0xd6, 0xff}
	colorFemale     = color.RGBA{0xe8, 0x7b, 0xa4, 0xff}
	colorForage     = color.RGBA{0xc3, 0xc2, 0xb7, 0xff}
	colorSeekMate   = color.RGBA{0xeb, 0x68, 0x34, 0xff}
	colorPaired     = color.RGBA{0x0c, 0xa3, 0x0c, 0xff}
	colorPairLink   = color.RGBA{0x0b, 0x0b, 0x0b, 0x30}
)

type game struct {
	world *engine.World
}

func (g *game) Update() error {
	for i := 0; i < ticksPerFrame; i++ {
		g.world.Step()
	}
	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	screen.Fill(colorBackground)

	for _, f := range g.world.Foods() {
		vector.DrawFilledCircle(screen, float32(f.X), float32(f.Y), 3, colorFood, true)
	}

	agents := g.world.Agents()

	// Pair links, drawn once per pair (from the lower ID).
	for i := range agents {
		a := &agents[i]
		if a.State != engine.StatePaired || a.PartnerID < a.ID {
			continue
		}
		if p, ok := g.world.AgentByID(a.PartnerID); ok {
			vector.StrokeLine(screen, float32(a.X), float32(a.Y), float32(p.X), float32(p.Y), 1, colorPairLink, true)
		}
	}

	for i := range agents {
		a := &agents[i]
		// Radius shows the food stored, ring width shows power, ring colour
		// shows what the agent is currently doing.
		food := min(a.Food, 100) / 100
		radius := float32(minRadius + food*(maxRadius-minRadius))
		ringWidth := float32(minRingSize + a.Power/100*(maxRingSize-minRingSize))

		fill := colorMale
		if a.Sex == engine.Female {
			fill = colorFemale
		}
		ring := colorForage
		switch a.State {
		case engine.StateSeekMate:
			ring = colorSeekMate
		case engine.StatePaired:
			ring = colorPaired
		}

		x, y := float32(a.X), float32(a.Y)
		vector.DrawFilledCircle(screen, x, y, radius, fill, true)
		vector.StrokeCircle(screen, x, y, radius, ringWidth, ring, true)
	}

	s := g.world.Stats()
	ebitenutil.DebugPrint(screen, fmt.Sprintf(
		"tick %d  pop %d (m %d / f %d)  food %d  births %d  deaths %d  gen %d\n"+
			"avg power %.1f  avg rationality %.1f\n"+
			"blue male / pink female, radius = food, ring width = power\n"+
			"ring grey = foraging, orange = seeking a mate, green = paired",
		s.Tick, s.Population, s.Males, s.Females, s.FoodItems, s.Births, s.Deaths, s.MaxGeneration,
		s.AvgPower, s.AvgRationality))
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
