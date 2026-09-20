package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/png"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/toku463ne/cld_human_behaviour/engine"
)

// Drawing with pictures instead of circles (TODO 10).
//
// The circles were right for what this viewer was for: a ring whose width is
// the attack gene and whose fill is what vitality is left says more per pixel
// than any drawing of a person, and every one of those readings is still
// here. What they were never going to be is a game, and the client this is
// becoming runs on a telephone (#131).
//
// So the body is a picture and everything around it is unchanged. What is
// drawn from the sheet is the shape; what the shape means - who it is, what
// it is doing, how big it is, how hurt - is still read off the world and put
// on top, because a sprite cannot be a gauge.
//
// Four things decided before any of it was drawn:
//
//   - One sheet. Everything stamped from a single image is one batch to the
//     hardware however many times it is stamped, and the count that matters
//     here is 260 things on the screen at once (measured before this, on the
//     map the game is played on).
//   - Grey art, tinted at the moment of drawing (ColorScale). The colour is
//     what says whose body it is and what kind of thing is lying there, and
//     there are sixty bodies and eight kinds of thing: baking the colour in
//     would mean drawing each picture as many times as there are cases.
//   - The manifest is read before the game starts, not while it runs. A
//     picture that arrives on the third frame is a body that was a hole for
//     two of them, and on a telephone over a telephone's network it would be
//     a great deal more than three.
//   - The sheet's file name carries a hash of its own bytes, so a browser
//     may keep it for ever and still never show a stale one. That is the
//     whole of the caching story and it needs no server to be clever.
//
// What this file does not do is know anything about the engine's rules. It
// asks what a body is doing and how big it is, which the viewer has always
// asked, and picks a picture. The engine has never heard of a tile.

// clipInfo is one run of frames in the sheet, as the manifest describes it.
type clipInfo struct {
	Name   string `json:"name"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	W      int    `json:"w"`
	H      int    `json:"h"`
	Frames int    `json:"frames"`
}

type manifestFile struct {
	Sheet string     `json:"sheet"`
	Tile  int        `json:"tile"`
	Clips []clipInfo `json:"clips"`
}

// tileset is the sheet, cut up. The sub-images share the one texture, so
// drawing from any of them batches with drawing from any other.
type tileset struct {
	sheet *ebiten.Image
	clips map[string][]*ebiten.Image
	tile  int
}

// loadTiles reads the manifest, then the sheet, and cuts it up. Everything is
// in hand when it returns or it returns an error: that is what "preloaded"
// means, and it is why this is called before the game is handed to ebiten
// rather than lazily on the first draw.
func loadTiles() (*tileset, error) {
	raw, err := loadAsset("manifest.json")
	if err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	var m manifestFile
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	if m.Sheet == "" || m.Tile <= 0 {
		return nil, fmt.Errorf("manifest names no sheet")
	}
	png, err := loadAsset(m.Sheet)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.Sheet, err)
	}
	src, _, err := image.Decode(bytes.NewReader(png))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.Sheet, err)
	}
	t := &tileset{sheet: ebiten.NewImageFromImage(src), tile: m.Tile, clips: map[string][]*ebiten.Image{}}
	for _, c := range m.Clips {
		frames := make([]*ebiten.Image, 0, c.Frames)
		for i := 0; i < c.Frames; i++ {
			r := image.Rect(c.X+i*c.W, c.Y, c.X+(i+1)*c.W, c.Y+c.H)
			frames = append(frames, t.sheet.SubImage(r).(*ebiten.Image))
		}
		t.clips[c.Name] = frames
	}
	return t, nil
}

// frame is one picture out of a clip, or nil where the world asks for
// something nobody drew. A nil picture is the caller's cue to fall back to
// the circle, so a half-finished sheet is a partly drawn world rather than a
// crash.
func (t *tileset) frame(name string, i int) *ebiten.Image {
	frames := t.clips[name]
	if len(frames) == 0 {
		return nil
	}
	return frames[((i%len(frames))+len(frames))%len(frames)]
}

// clipFor is which run of pictures a body's current action belongs to.
//
// Four states, which is what the art has and as much as the eye can tell
// apart at sixteen pixels: standing, going somewhere, eating, and having it
// out with somebody. Everything else in the vocabulary is one of those from
// the outside - crying your wares and cooking are both standing still, and
// the rings around the body are what tell them apart, as they always were.
func clipFor(a *engine.Agent) string {
	kind := "human"
	if a.Species == engine.SpeciesEnemy {
		kind = "enemy"
	}
	switch a.Action.Kind {
	case engine.ActEat:
		return kind + ".eat"
	case engine.ActAttack, engine.ActThrow:
		return kind + ".fight"
	case engine.ActMove, engine.ActFlee, engine.ActCourt, engine.ActInvite,
		engine.ActTake, engine.ActBuy, engine.ActGive, engine.ActOffer:
		return kind + ".walk"
	}
	return kind + ".idle"
}

// animTicks is how many ticks of the world one frame of an animation lasts.
// The world's clock rather than the viewer's, so that a stopped world holds
// still: a body walking on the spot while the clock is paused looks like the
// game has come loose from the simulation.
const animTicks = 5

// drawBody stamps one body. It reports whether it drew anything, so that the
// caller can fall back to the circle where there is no picture for it.
func (g *game) drawBody(screen *ebiten.Image, a *engine.Agent, x, y, radius float32, tint color.RGBA) bool {
	if g.tiles == nil {
		return false
	}
	// Each body starts its animation at its own point in the cycle, from its
	// ID: sixty bodies marching in step is the one thing that would make this
	// look worse than the circles did.
	img := g.tiles.frame(clipFor(a), a.ID+g.world.Tick()/animTicks)
	if img == nil {
		return false
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	// Drawn to the size the circle would have been, so that the one thing the
	// shape has always said - how much body there is - still holds.
	scale := float64(radius*2) / float64(w)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(-float64(w)/2, -float64(h)/2)
	if a.VX < 0 {
		// Facing the way it is going. A flip rather than a second picture:
		// the art is drawn facing one way and nothing about it is lettered.
		op.GeoM.Scale(-1, 1)
	}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(float64(x), float64(y))
	op.ColorScale.ScaleWithColor(tint)
	op.Filter = ebiten.FilterNearest
	screen.DrawImage(img, op)
	return true
}

// drawItem stamps one thing lying about, in the colour that says what it is.
func (g *game) drawItem(screen *ebiten.Image, kind engine.FoodKind, x, y float32, tint color.RGBA) bool {
	if g.tiles == nil {
		return false
	}
	img := g.tiles.frame("item", itemFrame(kind))
	if img == nil {
		return false
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	size := g.long(11)
	scale := float64(size) / float64(w)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(-float64(w)/2, -float64(h)/2)
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(float64(x), float64(y))
	op.ColorScale.ScaleWithColor(tint)
	op.Filter = ebiten.FilterNearest
	screen.DrawImage(img, op)
	return true
}

// itemFrame is which picture stands for a kind of thing. The order is the
// engine's own numbering with the two gaps closed (NumEdibleKinds is not a
// kind), so adding a kind means adding a picture at the end rather than
// renumbering anything.
func itemFrame(kind engine.FoodKind) int {
	switch kind {
	case engine.FoodPlant:
		return 0
	case engine.FoodFish:
		return 1
	case engine.FoodMeat:
		return 2
	case engine.FoodStone:
		return 3
	case engine.FoodCoin:
		return 4
	case engine.FoodBook:
		return 5
	case engine.FoodTrinket:
		return 6
	case engine.FoodHide:
		return 7
	}
	return 0
}

// bodyTint is the colour a body's picture is stamped in: its sex, shifted a
// little by which body it is.
//
// The shift is what keeps a crowd from looking like one thing repeated sixty
// times. It is small on purpose - the colour still has to read as "male" or
// "female" at a glance, because that is what it is for - and it is drawn from
// the ID rather than from anything the body is, so it says nothing and can be
// mistaken for nothing. There is no gene for complexion.
func bodyTint(a *engine.Agent, base color.RGBA) color.RGBA {
	// A repeatable wobble of about a tenth, from the ID alone.
	h := a.ID*2654435761 + 0x9e3779b9
	shade := func(v uint8, bits uint) uint8 {
		d := int(h>>bits&31) - 16 // -16 .. +15
		n := int(v) + d
		if n < 0 {
			n = 0
		}
		if n > 255 {
			n = 255
		}
		return uint8(n)
	}
	return color.RGBA{shade(base.R, 3), shade(base.G, 9), shade(base.B, 15), base.A}
}
