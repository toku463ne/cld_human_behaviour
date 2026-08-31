package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"

	"github.com/toku463ne/cld_human_behaviour/engine"
)

// The membership survival curve, drawn because the half-life on its own hides
// the thing worth knowing about it: the curve is not one exponential. Half the
// pairs have parted within about a minute of ticks, and yet a small share of
// them are still together an order of magnitude later. It is that slow
// component, not the half-life, that grouping should move once stage 11 puts a
// reason to keep a neighbour alive into the world, and a table of numbers in
// HISTORY.md does not make the two components visible the way a plot does.

var (
	colorAxis  = color.RGBA{0x88, 0x88, 0x82, 0xff}
	colorGrid  = color.RGBA{0xdd, 0xdd, 0xd6, 0xff}
	colorSeed  = color.RGBA{0x2a, 0x78, 0xd6, 0x40}
	colorMean  = color.RGBA{0x2a, 0x78, 0xd6, 0xff}
	colorMark  = color.RGBA{0xd0, 0x1c, 0x1c, 0xff}
	colorLabel = color.RGBA{0x33, 0x33, 0x30, 0xff}
)

// curves holds one survival curve per seed plus their average.
type curves struct {
	step     int
	perSeed  [][]float64
	mean     []float64
	halfLife float64
}

// measureCurves runs the seeds and collects their survival curves. The tracker
// watches only the final fifth of each run, the same window cmd/experiment
// reads its figures from, so that the picture and the table are of the same
// thing.
func measureCurves(seeds, ticks int, link float64) curves {
	out := curves{step: engine.DefaultMembershipStep}
	var halfSum float64
	for seed := 1; seed <= seeds; seed++ {
		cfg := engine.DefaultConfig()
		cfg.Seed = int64(seed)
		w := engine.NewWorld(cfg)
		m := engine.NewMembershipTracker(link, engine.DefaultMembershipStep, engine.DefaultMembershipLags)
		watchFrom := ticks - max(ticks/5, 1)
		for i := 0; i < ticks; i++ {
			w.Step()
			if i >= watchFrom && w.Tick()%engine.DefaultMembershipStep == 0 {
				m.Observe(w)
			}
		}
		r := m.Result()
		out.perSeed = append(out.perSeed, r.Survival)
		halfSum += r.HalfLife
	}
	out.halfLife = halfSum / float64(seeds)

	// The mean is taken over whichever seeds reach each lag: a run whose curve
	// ends early should not drag the tail towards zero.
	longest := 0
	for _, s := range out.perSeed {
		longest = max(longest, len(s))
	}
	out.mean = make([]float64, longest)
	for k := 0; k < longest; k++ {
		sum, n := 0.0, 0
		for _, s := range out.perSeed {
			if k < len(s) {
				sum += s[k]
				n++
			}
		}
		if n > 0 {
			out.mean[k] = sum / float64(n)
		}
	}
	return out
}

func renderCurve(c curves, seeds, ticks int, link float64, commit string) *image.RGBA {
	const (
		width     = 900
		height    = 520
		captionH  = 22
		plotLeft  = 62
		plotRight = width - 24
		plotTop   = 54
		plotBot   = height - 52
	)
	plotW, plotH := float64(plotRight-plotLeft), float64(plotBot-plotTop)

	img := image.NewRGBA(image.Rect(0, 0, width, height+captionH))
	draw.Draw(img, image.Rect(0, 0, width, height), image.NewUniform(colorBackground), image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(0, height, width, height+captionH), image.NewUniform(colorCaptionBg), image.Point{}, draw.Src)

	// The lag axis is logarithmic: the half-life sits near 60 ticks and the
	// tail runs to 2000, and a linear axis would squash the first into the
	// left edge. Lag zero has no place on it, so the curve starts at one step.
	minLag := float64(c.step)
	maxLag := float64(c.step * (len(c.mean) - 1))
	x := func(lag float64) float64 {
		t := (math.Log10(lag) - math.Log10(minLag)) / (math.Log10(maxLag) - math.Log10(minLag))
		return float64(plotLeft) + t*plotW
	}
	y := func(v float64) float64 { return float64(plotTop) + (1-v)*plotH }

	// Horizontal grid and the survival scale.
	for _, v := range []float64{0, 0.25, 0.5, 0.75, 1} {
		gy := y(v)
		col := colorGrid
		if v == 0.5 {
			col = colorAxis // the line the half-life is read off
		}
		line(img, float64(plotLeft), gy, float64(plotRight), gy, col)
		drawText(img, plotLeft-46, int(gy)-glyphH/2, fmt.Sprintf("%.2f", v), colorLabel, 1)
	}

	// Vertical grid at the lags worth reading off.
	for _, lag := range []int{25, 50, 100, 200, 500, 1000, 2000} {
		if float64(lag) < minLag || float64(lag) > maxLag {
			continue
		}
		gx := x(float64(lag))
		line(img, gx, float64(plotTop), gx, float64(plotBot), colorGrid)
		label := fmt.Sprintf("%d", lag)
		drawText(img, int(gx)-textWidth(label, 1)/2, plotBot+8, label, colorLabel, 1)
	}

	// Each seed faintly, so that the spread behind the average is visible and
	// the average is not mistaken for something every run does.
	for _, s := range c.perSeed {
		plot(img, s, c.step, x, y, colorSeed)
	}
	plot(img, c.mean, c.step, x, y, colorMean)
	for k := 1; k < len(c.mean); k++ {
		disc(img, x(float64(k*c.step)), y(c.mean[k]), 2.2, colorMean)
	}

	// Where the average crosses a half.
	if c.halfLife >= minLag && c.halfLife <= maxLag {
		hx := x(c.halfLife)
		dashed(img, hx, y(0.5), hx, float64(plotBot), colorMark)
		drawText(img, int(hx)+6, int(y(0.5))-glyphH-4,
			fmt.Sprintf("HALF-LIFE %.0f", c.halfLife), colorMark, 1)
	}

	drawText(img, plotLeft, 18, "MEMBERSHIP SURVIVAL: PAIRS STILL IN ONE CLUSTER, LAG IN TICKS (LOG)", colorLabel, 2)
	drawText(img, plotLeft, 36, "FAINT LINES ARE THE SEEDS, SOLID IS THEIR MEAN. DEATHS ARE EXCLUDED, NOT COUNTED AS PARTINGS.", colorLabel, 1)

	caption := fmt.Sprintf("SEEDS 1-%d  TICKS %d  LINK %.0f  STEP %d  WINDOW %d  HALF-LIFE %.1f  COMMIT %s",
		seeds, ticks, link, c.step, c.step*(len(c.mean)-1), c.halfLife, commit)
	textScale := 1
	if textWidth(caption, 2) < width-16 {
		textScale = 2
	}
	drawText(img, 8, height+(captionH-glyphH*textScale)/2, caption, colorCaption, textScale)
	return img
}

// plot joins the readings of one curve, skipping lag zero because the axis is
// logarithmic.
func plot(img *image.RGBA, surv []float64, step int, x, y func(float64) float64, col color.RGBA) {
	for k := 2; k < len(surv); k++ {
		line(img, x(float64((k-1)*step)), y(surv[k-1]), x(float64(k*step)), y(surv[k]), col)
	}
}

func dashed(img *image.RGBA, x0, y0, x1, y1 float64, col color.RGBA) {
	const dash = 4
	length := math.Hypot(x1-x0, y1-y0)
	for at := 0.0; at < length; at += dash * 2 {
		t0, t1 := at/length, math.Min((at+dash)/length, 1)
		line(img, x0+(x1-x0)*t0, y0+(y1-y0)*t0, x0+(x1-x0)*t1, y0+(y1-y0)*t1, col)
	}
}
