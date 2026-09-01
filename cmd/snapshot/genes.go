package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"

	"github.com/toku463ne/cld_human_behaviour/engine"
)

// The distribution of the three abilities across the population, drawn because
// the inheritance rule is a change nobody can see in a picture of the world:
// two runs under two rules differ in where every agent happens to be standing,
// which says nothing about either rule. What the rule decides is the shape of
// this histogram - how much of the variation the founders were born with is
// still there twenty generations later.
//
// The founding distribution is drawn behind the final one for scale. Founders
// are drawn uniformly from a band in the middle of the range, so anything
// narrower than that band is variation that has been lost, and the mean sliding
// away from the middle is selection.

var (
	colorStart = color.RGBA{0x99, 0x99, 0x92, 0xff}
	colorEnd   = color.RGBA{0x2a, 0x78, 0xd6, 0xff}
	colorFill  = color.RGBA{0x2a, 0x78, 0xd6, 0x30}
)

const geneBins = 50

// geneDist is the pooled ability distribution of several runs, at the first
// tick and at the last.
type geneDist struct {
	names [3]string

	// start and end hold, per ability, the share of the population in each
	// bin across the range abilities can take.
	start, end [3][]float64

	meanStart, sdStart [3]float64
	meanEnd, sdEnd     [3]float64

	pop int // agents pooled at the end, over all the seeds
}

func measureGenes(seeds, ticks int) geneDist {
	g := geneDist{names: [3]string{"POWER", "RATIONALITY", "INTELLIGENCE"}}
	var startVals, endVals [3][]float64

	for seed := 1; seed <= seeds; seed++ {
		cfg := engine.DefaultConfig()
		cfg.Seed = int64(seed)
		w := engine.NewWorld(cfg)
		collect(&startVals, w)
		for i := 0; i < ticks; i++ {
			w.Step()
		}
		collect(&endVals, w)
	}

	g.pop = len(endVals[0])
	for k := 0; k < 3; k++ {
		g.start[k] = histogram(startVals[k])
		g.end[k] = histogram(endVals[k])
		g.meanStart[k], g.sdStart[k] = meanSD(startVals[k])
		g.meanEnd[k], g.sdEnd[k] = meanSD(endVals[k])
	}
	return g
}

func collect(into *[3][]float64, w *engine.World) {
	for _, a := range w.Agents() {
		into[0] = append(into[0], a.Attack())
		into[1] = append(into[1], a.Rationality())
		into[2] = append(into[2], a.Intelligence())
	}
}

// histogram is the share of the values falling in each bin, so that runs with
// different populations can be drawn on the same axes.
func histogram(vs []float64) []float64 {
	out := make([]float64, geneBins)
	if len(vs) == 0 {
		return out
	}
	lo, hi := engine.MinAbility, engine.MaxAbility
	for _, v := range vs {
		b := int((v - lo) / (hi - lo) * geneBins)
		if b < 0 {
			b = 0
		}
		if b >= geneBins {
			b = geneBins - 1
		}
		out[b]++
	}
	for i := range out {
		out[i] /= float64(len(vs))
	}
	return out
}

func meanSD(vs []float64) (float64, float64) {
	if len(vs) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range vs {
		sum += v
	}
	mean := sum / float64(len(vs))
	var sq float64
	for _, v := range vs {
		sq += (v - mean) * (v - mean)
	}
	return mean, math.Sqrt(sq / float64(len(vs)))
}

func renderGenes(g geneDist, seeds, ticks int, commit string) *image.RGBA {
	const (
		width    = 900
		panelH   = 150
		panelGap = 46 // room for the axis labels and the next panel's heading
		top      = 80
		left     = 62
		right    = width - 24
		captionH = 22
	)
	height := top + 3*panelH + 2*panelGap + 30
	img := image.NewRGBA(image.Rect(0, 0, width, height+captionH))
	draw.Draw(img, image.Rect(0, 0, width, height), image.NewUniform(colorBackground), image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(0, height, width, height+captionH), image.NewUniform(colorCaptionBg), image.Point{}, draw.Src)

	drawText(img, left, 20, "ABILITY DISTRIBUTION AFTER THE RUN, POOLED OVER THE SEEDS", colorLabel, 2)
	drawText(img, left, 42, "GREY IS THE FOUNDING POPULATION, BLUE IS THE END. A NARROWER SHAPE IS VARIATION THAT INHERITANCE LOST.", colorLabel, 1)
	drawText(img, left, 54, "DASHED: GREY THE FOUNDING MEAN, RED WHERE SELECTION HAS TAKEN IT.", colorLabel, 1)

	// One vertical scale for all three panels, so that the panels can be
	// compared with each other as well as with the founders.
	peak := 0.0
	for k := 0; k < 3; k++ {
		for _, v := range g.end[k] {
			peak = math.Max(peak, v)
		}
		for _, v := range g.start[k] {
			peak = math.Max(peak, v)
		}
	}
	if peak <= 0 {
		peak = 1
	}

	plotW := float64(right - left)
	for k := 0; k < 3; k++ {
		panelTop := top + k*(panelH+panelGap)
		panelBot := panelTop + panelH
		x := func(ability float64) float64 {
			t := (ability - engine.MinAbility) / (engine.MaxAbility - engine.MinAbility)
			return float64(left) + t*plotW
		}
		y := func(share float64) float64 {
			return float64(panelBot) - share/peak*float64(panelH)
		}

		for _, ability := range []float64{1, 25, 50, 75, 100} {
			gx := x(ability)
			line(img, gx, float64(panelTop), gx, float64(panelBot), colorGrid)
			label := fmt.Sprintf("%.0f", ability)
			drawText(img, int(gx)-textWidth(label, 1)/2, panelBot+7, label, colorLabel, 1)
		}
		line(img, float64(left), float64(panelBot), float64(right), float64(panelBot), colorAxis)

		steps(img, g.start[k], x, y, colorStart, color.RGBA{})
		steps(img, g.end[k], x, y, colorEnd, colorFill)

		// The mean of each, as a tick above the axis.
		dashed(img, x(g.meanStart[k]), float64(panelTop), x(g.meanStart[k]), float64(panelBot), colorStart)
		dashed(img, x(g.meanEnd[k]), float64(panelTop), x(g.meanEnd[k]), float64(panelBot), colorMark)

		head := fmt.Sprintf("%s   START %.1f +/-%.1f   END %.1f +/-%.1f   SD %+.0f%%",
			g.names[k], g.meanStart[k], g.sdStart[k], g.meanEnd[k], g.sdEnd[k],
			percentChange(g.sdStart[k], g.sdEnd[k]))
		drawText(img, left, panelTop-13, head, colorLabel, 1)
	}

	caption := fmt.Sprintf("SEEDS 1-%d  TICKS %d  AGENTS %d  SD POWER %.1f  RAT %.1f  INT %.1f  COMMIT %s",
		seeds, ticks, g.pop, g.sdEnd[0], g.sdEnd[1], g.sdEnd[2], commit)
	textScale := 1
	if textWidth(caption, 2) < width-16 {
		textScale = 2
	}
	drawText(img, 8, height+(captionH-glyphH*textScale)/2, caption, colorCaption, textScale)
	return img
}

func percentChange(from, to float64) float64 {
	if from == 0 {
		return 0
	}
	return (to/from - 1) * 100
}

// steps draws a histogram as a step outline, optionally filled.
func steps(img *image.RGBA, bins []float64, x, y func(float64) float64, outline, fill color.RGBA) {
	span := (engine.MaxAbility - engine.MinAbility) / float64(len(bins))
	for i, v := range bins {
		x0 := x(engine.MinAbility + float64(i)*span)
		x1 := x(engine.MinAbility + float64(i+1)*span)
		if fill.A > 0 && v > 0 {
			for px := x0; px < x1; px++ {
				line(img, px, y(v), px, y(0), fill)
			}
		}
		line(img, x0, y(v), x1, y(v), outline)
		prev := 0.0
		if i > 0 {
			prev = bins[i-1]
		}
		line(img, x0, y(prev), x0, y(v), outline)
		if i == len(bins)-1 {
			line(img, x1, y(v), x1, y(0), outline)
		}
	}
}
