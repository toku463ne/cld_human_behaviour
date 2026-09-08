// Command snapshot draws a record image of a run to a PNG.
//
// It exists to leave a visual record that can be filed next to a commit: the
// numbers cmd/experiment prints say how many clusters there are, but not what
// a world with that many clusters looks like, and they give a half-life
// without the shape of the curve it was read off. Nothing here is part of the
// simulation: every measurement it draws is read only analysis.
//
// Two things can be drawn.
//
//	-mode world  one moment of one run, the population coloured by cluster
//	-mode curve  the membership survival curve, averaged over several seeds
//	-mode genes  the ability distribution at the end of a run against the one
//	             the founders started with, pooled over several seeds
//
// A run is deterministic, so the same flags always produce the same image. The
// caption strip records what they were, together with the commit the image was
// taken at, so a picture that has been moved out of the repository still says
// where it came from.
//
//	go run ./cmd/snapshot -seed 1 -ticks 50000 -out docs/images/clusters.png
//	go run ./cmd/snapshot -mode curve -seeds 8 -out docs/images/membership.png
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/toku463ne/cld_human_behaviour/engine"
)

// The clusters are drawn in these in turn, largest cluster first. There are
// usually more clusters than colours, so the palette cycles and two distant
// clusters can share one: it is the drawn links, not the colour, that say what
// belongs together. Lone agents are drawn in colorLoner instead, because a
// singleton is not a group and giving each one a colour of its own would make
// an empty stretch of world look busy.
var clusterColors = []color.RGBA{
	{0x2a, 0x78, 0xd6, 0xff}, // blue
	{0xd0, 0x5c, 0x1c, 0xff}, // orange
	{0x0c, 0xa3, 0x0c, 0xff}, // green
	{0xa8, 0x3c, 0xb0, 0xff}, // purple
	{0xc9, 0x8a, 0x20, 0xff}, // amber
	{0x1b, 0xa0, 0xa8, 0xff}, // teal
	{0xd0, 0x1c, 0x5c, 0xff}, // crimson
	{0x5a, 0x6e, 0x2a, 0xff}, // olive
	{0x6b, 0x4c, 0xd6, 0xff}, // indigo
	{0x8a, 0x5a, 0x2a, 0xff}, // brown
	{0x1c, 0x84, 0x5c, 0xff}, // sea green
	{0x9c, 0x2a, 0x2a, 0xff}, // brick
}

var (
	colorBackground = color.RGBA{0xfc, 0xfc, 0xfb, 0xff}
	colorLoner      = color.RGBA{0xb4, 0xb4, 0xac, 0xff}
	colorCaption    = color.RGBA{0x33, 0x33, 0x30, 0xff}
	colorCaptionBg  = color.RGBA{0xef, 0xef, 0xec, 0xff}
	colorFood       = color.RGBA{0x1b, 0xaf, 0x7a, 0x50}
	colorWater      = color.RGBA{0x7f, 0xa8, 0xd0, 0x66}
	colorDivide     = color.RGBA{0x33, 0x33, 0x30, 0x44}
)

// terrainMaps are the pieces of country cmd/experiment measures stage 37 on,
// so that a number and a picture are of the same world. They live in both for
// the reason cmd/devview's copies do: they are test fixtures rather than part
// of the engine, and the world gets its country from whoever starts it.
var terrainMaps = map[string][]string{
	"river": {
		".......~~.......", ".......~~.......", ".......~~.......",
		".......~~.......", ".......~~.......", ".......~~.......",
		".......~~.......", ".......~~.......", ".......~~.......",
		".......~~.......", ".......~~.......", ".......~~.......",
	},
	"ponds": {
		"................", ".~~.............", ".~~.........~~..",
		"............~~..", ".....~~.........", ".....~~.........",
		"................", ".........~~.....", "..~~.....~~.....",
		"..~~........~~..", "............~~..", "................",
	},
	"gorge": {
		"......~~~~......", "......~~~~......", "......~~~~......",
		"......~~~~......", "......~~~~......", "......~~~~......",
		"......~~~~......", "......~~~~......", "......~~~~......",
		"......~~~~......", "......~~~~......", "......~~~~......",
	},
	"pools": {
		"~~......~~......", "~~......~~......", "....~~......~~..",
		"....~~......~~..", "..~~......~~....", "..~~......~~....",
		"......~~......~~", "......~~......~~", "~~......~~......",
		"~~......~~......", "....~~......~~..", "....~~......~~..",
	},
}

func main() {
	mode := flag.String("mode", "world", "what to draw: world, curve or genes")
	seed := flag.Int64("seed", 1, "seed of the run (world mode)")
	seeds := flag.Int("seeds", 8, "how many seeds to average over (curve and genes modes)")
	ticks := flag.Int("ticks", 50000, "ticks to run before drawing")
	link := flag.Float64("link", engine.DefaultClusterLinkDist, "cluster linking distance")
	scale := flag.Int("scale", 2, "output pixels per world unit (world mode)")
	land := flag.String("terrain", "", "lay the world out on a piece of country (world mode): river, ponds, gorge or pools (stage 37)")
	commit := flag.String("commit", "", "commit to stamp on the image (default: the current HEAD)")
	out := flag.String("out", "snapshot.png", "file to write")
	flag.Parse()

	stamp := *commit
	if stamp == "" {
		stamp = headCommit()
	}

	var img *image.RGBA
	var note string
	switch *mode {
	case "world":
		cfg := engine.DefaultConfig()
		cfg.Seed = *seed
		if *land != "" {
			m, ok := terrainMaps[*land]
			if !ok {
				fail(fmt.Errorf("no such terrain %q: want river, ponds, gorge or pools", *land))
			}
			// The country a world would be played on: the bank grows more
			// (stage 36) and hard ground is poor ground (stage 33), which is
			// what cmd/devview lays out and what the waterrichlink arm
			// measures.
			cfg.TerrainMap = m
			cfg.WatersideFood, cfg.TerrainFoodCorrelation = 1, 1
		}
		w := engine.NewWorld(cfg)
		for i := 0; i < *ticks; i++ {
			w.Step()
		}
		img = render(w, cfg, *link, *scale, stamp)
		c := w.Clusters(*link)
		b := w.Banks(cfg.Width / 2)
		note = fmt.Sprintf("seed %d, tick %d, link %.0f, pop %d, clusters %d, largest %.0f%%, split %.2f, cross %.2f (dry %.2f)",
			*seed, w.Tick(), *link, w.Stats().Population, c.Groups, c.LargestShare*100,
			b.Split, b.CrossIndex, b.CrossDry)
	case "genes":
		g := measureGenes(*seeds, *ticks)
		img = renderGenes(g, *seeds, *ticks, stamp)
		note = fmt.Sprintf("seeds 1-%d, %d ticks, %d agents, sd %.1f/%.1f/%.1f",
			*seeds, *ticks, g.pop, g.sdEnd[0], g.sdEnd[1], g.sdEnd[2])
	case "curve":
		curves := measureCurves(*seeds, *ticks, *link)
		img = renderCurve(curves, *seeds, *ticks, *link, stamp)
		note = fmt.Sprintf("seeds 1-%d, %d ticks, link %.0f, half-life %.1f",
			*seeds, *ticks, *link, curves.halfLife)
	default:
		fail(fmt.Errorf("unknown mode %q: want world, curve or genes", *mode))
	}

	if dir := filepath.Dir(*out); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fail(err)
		}
	}
	f, err := os.Create(*out)
	if err != nil {
		fail(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		fail(err)
	}
	fmt.Printf("%s: %s\n", *out, note)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "snapshot:", err)
	os.Exit(1)
}

// headCommit is the short hash of HEAD, marked dirty when the tree has
// uncommitted changes. Getting this wrong would defeat the point of the image,
// so it is read from git rather than typed in by hand.
func headCommit() string {
	hash, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	stamp := strings.TrimSpace(string(hash))
	if status, err := exec.Command("git", "status", "--porcelain").Output(); err == nil {
		if len(strings.TrimSpace(string(status))) > 0 {
			stamp += "-dirty"
		}
	}
	return stamp
}

func render(w *engine.World, cfg engine.Config, link float64, scale int, commit string) *image.RGBA {
	const captionH = 22

	worldW, worldH := int(cfg.Width)*scale, int(cfg.Height)*scale
	img := image.NewRGBA(image.Rect(0, 0, worldW, worldH+captionH))
	draw.Draw(img, image.Rect(0, 0, worldW, worldH), image.NewUniform(colorBackground), image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(0, worldH, worldW, worldH+captionH), image.NewUniform(colorCaptionBg), image.Point{}, draw.Src)

	agents := w.Agents()
	c := w.Clusters(link)

	// The water first of all, so that a clump standing in a river is visibly
	// standing in a river (stage 37). The line the banks are counted across
	// goes over it: a picture of two settlements and a picture of one crowd
	// lying along the water look nothing alike, and the cluster figures in the
	// caption cannot tell them apart.
	if cols, rows, cw, ch := w.TerrainSize(); cols > 0 {
		for cy := 0; cy < rows; cy++ {
			for cx := 0; cx < cols; cx++ {
				if w.TerrainAt((float64(cx)+0.5)*cw, (float64(cy)+0.5)*ch).Kind != engine.GroundWater {
					continue
				}
				rect(img, float64(cx)*cw*float64(scale), float64(cy)*ch*float64(scale),
					cw*float64(scale), ch*float64(scale), colorWater)
			}
		}
		x := cfg.Width / 2 * float64(scale)
		line(img, x, 0, x, cfg.Height*float64(scale), colorDivide)
	}

	// Food first, faintly: it is what the agents are gathered around, so
	// leaving it out would make the clumps look unexplained.
	for _, f := range w.Foods() {
		disc(img, f.X*float64(scale), f.Y*float64(scale), 1.6*float64(scale), colorFood)
	}

	// Then the links that put two agents in the same cluster. Drawing them is
	// what makes a chained cluster legible: a long thin cluster is obviously a
	// chain rather than a huddle once its edges are visible.
	for i := range agents {
		for j := i + 1; j < len(agents); j++ {
			dx, dy := agents[i].X-agents[j].X, agents[i].Y-agents[j].Y
			if dx*dx+dy*dy > link*link {
				continue
			}
			col := clusterColor(c, i)
			col.A = 0x55
			line(img, agents[i].X*float64(scale), agents[i].Y*float64(scale),
				agents[j].X*float64(scale), agents[j].Y*float64(scale), col)
		}
	}

	for i := range agents {
		disc(img, agents[i].X*float64(scale), agents[i].Y*float64(scale), 2.6*float64(scale), clusterColor(c, i))
	}

	caption := fmt.Sprintf("SEED %d  TICK %d  LINK %.0f  POP %d  CLUSTERS %d  AVGSIZE %.1f  GROUPED %.0f%%  LARGEST %.0f%%  COMMIT %s",
		cfg.Seed, w.Tick(), link, len(agents), c.Groups, c.AvgGroupSize,
		c.GroupedShare*100, c.LargestShare*100, commit)
	textScale := 1
	if textWidth(caption, 2) < worldW-16 {
		textScale = 2
	}
	drawText(img, 8, worldH+(captionH-glyphH*textScale)/2, caption, colorCaption, textScale)
	return img
}

func clusterColor(c engine.Clustering, i int) color.RGBA {
	label := c.Labels[i]
	if c.Sizes[label] == 1 {
		return colorLoner
	}
	return clusterColors[label%len(clusterColors)]
}

// disc fills a circle, fading the outermost pixels by how much of them the
// circle covers so that the dots do not come out jagged.
func disc(img *image.RGBA, cx, cy, r float64, col color.RGBA) {
	x0, x1 := int(math.Floor(cx-r-1)), int(math.Ceil(cx+r+1))
	y0, y1 := int(math.Floor(cy-r-1)), int(math.Ceil(cy+r+1))
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			cover := math.Min(math.Max(r+0.5-d, 0), 1)
			if cover > 0 {
				blend(img, x, y, col, cover*float64(col.A)/255)
			}
		}
	}
}

// rect fills a rectangle, the same way: a flat wash the agents are drawn over.
func rect(img *image.RGBA, x, y, w, h float64, col color.RGBA) {
	for py := int(math.Floor(y)); py < int(math.Ceil(y+h)); py++ {
		for px := int(math.Floor(x)); px < int(math.Ceil(x+w)); px++ {
			blend(img, px, py, col, float64(col.A)/255)
		}
	}
}

// line draws a one pixel line between two points, the same way.
func line(img *image.RGBA, x0, y0, x1, y1 float64, col color.RGBA) {
	steps := int(math.Ceil(math.Hypot(x1-x0, y1-y0)))
	if steps == 0 {
		return
	}
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x, y := x0+(x1-x0)*t, y0+(y1-y0)*t
		blend(img, int(x), int(y), col, float64(col.A)/255)
	}
}

func blend(img *image.RGBA, x, y int, col color.RGBA, alpha float64) {
	if !(image.Point{x, y}).In(img.Bounds()) {
		return
	}
	i := img.PixOffset(x, y)
	mix := func(dst, src uint8) uint8 {
		return uint8(float64(dst)*(1-alpha) + float64(src)*alpha + 0.5)
	}
	img.Pix[i] = mix(img.Pix[i], col.R)
	img.Pix[i+1] = mix(img.Pix[i+1], col.G)
	img.Pix[i+2] = mix(img.Pix[i+2], col.B)
	img.Pix[i+3] = 0xff
}
