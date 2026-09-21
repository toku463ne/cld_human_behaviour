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
	// Tint says this clip is grey and has to be given a colour to be seen at
	// all. The drawn art must not be given one: multiplying a body that is
	// already skin and hair and cloth by a colour does not tint it, it stains
	// it - the sex blue turns a body into a drowned one and the sex pink into
	// something skinned. Which clips are which is the sheet's business rather
	// than this file's, so the sheet says.
	Tint bool `json:"tint"`
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
	tint  map[string]bool
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
	t := &tileset{sheet: ebiten.NewImageFromImage(src), tile: m.Tile,
		clips: map[string][]*ebiten.Image{}, tint: map[string]bool{}}
	for _, c := range m.Clips {
		frames := make([]*ebiten.Image, 0, c.Frames)
		for i := 0; i < c.Frames; i++ {
			r := image.Rect(c.X+i*c.W, c.Y, c.X+(i+1)*c.W, c.Y+c.H)
			frames = append(frames, t.sheet.SubImage(r).(*ebiten.Image))
		}
		t.clips[c.Name] = frames
		t.tint[c.Name] = c.Tint
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

// look is what the viewer knows about a body that the body does not carry
// itself: where it is standing, and what is being done to it. All three are
// read off the world the same frame the body is drawn, none of them is a new
// fact, and none of them is remembered anywhere.
type look struct {
	aloft  bool // off the ground (a winged sort, between meals)
	wading bool // standing in water
	struck bool // somebody is hitting it this tick
}

// clipFor is which run of pictures a body's current action belongs to.
//
// Four states were as much as the eye could tell apart while the art had four
// runs: standing, going somewhere, eating, and having it out with somebody.
// The sheet has more than that now, and what it has more of is not actions
// but circumstances - being hit, being up to your waist in a river, being
// small, being old - so those are asked about first and the action decides
// only what is left.
//
// The order is what matters here, and it is by what the eye needs most. A
// body being struck is the thing a player is watching for, so it wins over
// where it is standing; where it is standing wins over how old it is,
// because a body in water is in immediate danger and a child is only small.
//
// Sex picks the run too. It used to be the colour the body was filled with,
// and drawn art cannot be filled with a colour, so without this every human
// would be the male picture and a fact that has been on this screen since the
// first week would be gone. The art says it the way art can: what the body is
// wearing.
//
// For a beast it is the build instead of the sex, and aloft is a run of its
// own: a winged one in the air is the one body on this screen drawn somewhere
// it is not standing, and until the beasts were drawn it was the same picture
// lifted.
//
// What is still asleep in the sheet is the hair. Seven colours, and each of
// them a whole standing body rather than a head to lay over one, so using it
// would give a body coloured hair while it stood and brown hair the moment it
// walked. Lineage by hair colour needs the art in two layers, which is a
// thing to ask for and not a thing to write.
func clipFor(a *engine.Agent, cfg *engine.Config, l look) string {
	kind := "human"
	switch {
	case a.Species == engine.SpeciesEnemy:
		if l.aloft {
			// Flapping while it crosses the sky, gliding while it holds
			// station. Two pictures for the price of the one the art
			// already had.
			if a.Action.Kind == engine.ActMove || a.Action.Kind == engine.ActFlee {
				return "enemy.fly.flap"
			}
			return "enemy.fly.air"
		}
		kind = "enemy." + enemyBuild(a, cfg)
	case a.Sex == engine.Female:
		kind = "human.f"
	}
	if hitting(a) {
		// Throwing the blow beats taking one: a body doing both at once is
		// more legible as the one going forward.
		return kind + ".fight"
	}
	// Taking a blow and standing in a river are drawn for people only. The
	// beasts' sheet has neither - the row of one being struck was asked for
	// and never arrived, and nobody asked for one in the water because the
	// beast that lives in water is a build rather than a circumstance, and a
	// brute wading across a river is still a brute.
	//
	// This is a hole in the art and it is left as one on purpose. Reaching
	// for enemy.water.* here would draw a heavy beast as a lurker, which is a
	// lie about which sort it is - and which sort it is, is the one thing
	// about a beast this screen has to get right.
	if a.Species == engine.SpeciesHuman {
		if l.struck {
			return kind + ".hurt"
		}
		if l.wading {
			return kind + ".swim"
		}
	}
	// Small and old, and only while standing still.
	//
	// The sheet has one run each for a child and for someone old, which is a
	// body standing there, and nothing for either of them walking or eating
	// or fighting. Using it for those too would leave a child sliding across
	// the ground in a standing pose while every adult beside it walked, which
	// reads as a broken picture rather than as a child. So a child that is
	// doing something is the grown picture drawn small, exactly as it was
	// before these runs existed - nothing is lost, and a child standing
	// still now looks like a child. The fix is two more rows of art.
	if idling(a) {
		if !a.IsAdult(cfg) {
			return childOf(kind) + ".idle"
		}
		// Past its prime is the engine's own line, read off the same figure
		// that already makes an old body draw smaller, so a world with the
		// rule turned off has nobody old in it and asks for nothing.
		if a.Maturity >= 1 && a.AgeFactor(cfg) < 1 {
			return oldOf(kind) + ".idle"
		}
	}
	switch a.Action.Kind {
	case engine.ActEat:
		return kind + ".eat"
	case engine.ActMove, engine.ActFlee, engine.ActCourt, engine.ActInvite,
		engine.ActTake, engine.ActBuy, engine.ActGive, engine.ActOffer:
		return kind + ".walk"
	}
	return kind + ".idle"
}

// hitting is whether this body is throwing a blow, and idling whether it is
// doing something that looks like standing there. Both are the same reading
// the action switch below makes, pulled out so that the circumstances above
// can ask the question before the action answers it.
func hitting(a *engine.Agent) bool {
	return a.Action.Kind == engine.ActAttack || a.Action.Kind == engine.ActThrow
}

func idling(a *engine.Agent) bool {
	switch a.Action.Kind {
	case engine.ActEat, engine.ActAttack, engine.ActThrow, engine.ActMove,
		engine.ActFlee, engine.ActCourt, engine.ActInvite, engine.ActTake,
		engine.ActBuy, engine.ActGive, engine.ActOffer:
		return false
	}
	return true
}

// childOf and oldOf are the runs drawn for the young and the old of a kind.
// Only the people have them; a beast is a beast at every age, which is what
// the sheet was asked for and what the rules say about one.
func childOf(kind string) string {
	switch kind {
	case "human":
		return "child"
	case "human.f":
		return "child.f"
	}
	return kind
}

func oldOf(kind string) string {
	switch kind {
	case "human":
		return "old"
	case "human.f":
		return "old.f"
	}
	return kind
}

// deadClipFor is the picture for a body that has just stopped being one.
//
// Its own function because the world no longer holds the body by the time
// this is wanted - Agents() is the living - so the caller has kept the little
// it needs rather than the Agent itself, and there is no action to ask about.
func deadClipFor(a *engine.Agent, cfg *engine.Config) string {
	if a.Species == engine.SpeciesEnemy {
		return "enemy." + enemyBuild(a, cfg) + ".dead"
	}
	// One picture for both sexes. The render had three dead men and one dead
	// woman rather than a clean pair, and a corpse's sex is not something
	// anybody reads at sixteen pixels.
	return "human.dead"
}

// enemyBuild is which of the four beasts a body is drawn as.
//
// Read off what the world says the sort IS, never off what the map called it.
// A map brings the names and where each sort comes into the world; the row
// brings what the sort is like (decision #133), and a picture is a thing the
// sort is like. A map that calls its heavy beast something else still gets
// the heavy picture, and one that invents a fifth sort gets a picture rather
// than a hole.
//
// Size is the one that needs saying twice: it is NOT in here. The viewer
// draws a beast as big as its body already (bodySize), so the four builds are
// drawn filling their cells the same way and the budget does the rest. Art
// half again the size of a person, on a body whose budget is already half
// again a person's, would count the same fact twice.
func enemyBuild(a *engine.Agent, cfg *engine.Config) string {
	if int(a.Kind) >= len(cfg.EnemyKinds) {
		return "small"
	}
	k := cfg.EnemyKinds[a.Kind]
	switch {
	case k.Flies:
		return "fly"
	case k.Water:
		return "water"
	}
	// A row that says nothing about its size is the world's own size, which
	// is the ordinary beast rather than the heavy one.
	if k.BudgetMean > cfg.EnemyBudgetMean {
		return "big"
	}
	return "small"
}

// animTicks is how many ticks of the world one frame of an animation lasts.
// The world's clock rather than the viewer's, so that a stopped world holds
// still: a body walking on the spot while the clock is paused looks like the
// game has come loose from the simulation.
const animTicks = 5

// drawBody stamps one body. It reports whether it drew anything, so that the
// caller can fall back to the circle where there is no picture for it.
func (g *game) drawBody(screen *ebiten.Image, a *engine.Agent, cfg *engine.Config, x, y, radius float32, tint color.RGBA) bool {
	if g.tiles == nil {
		return false
	}
	// Each body starts its animation at its own point in the cycle, from its
	// ID: sixty bodies marching in step is the one thing that would make this
	// look worse than the circles did.
	clip := clipFor(a, cfg, g.lookAt(a))
	img := g.tiles.frame(clip, a.ID+g.world.Tick()/animTicks)
	if img == nil {
		return false
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	// Drawn to the size the circle would have been, so that the one thing the
	// shape has always said - how much body there is - still holds. A clip
	// carries its own w and h, so the grey placeholders the enemies are still
	// drawn with are half the size in the sheet and the same size on screen.
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
	if g.tiles.tint[clip] {
		op.ColorScale.ScaleWithColor(tint)
	}
	// Nearest while the picture is being made bigger, which is what the zoom
	// the game is played at does to it, and where anything else would turn
	// drawn pixels to mush. Drawn smaller - the whole world on one screen,
	// where a body is seven pixels across - nearest throws away five pixels
	// in six and what is left crawls as the body moves. There the average is
	// the honest one.
	op.Filter = ebiten.FilterNearest
	if scale < 1 {
		op.Filter = ebiten.FilterLinear
	}
	screen.DrawImage(img, op)
	return true
}

// drawItem stamps one thing lying about, in the colour that says what it is.
func (g *game) drawItem(screen *ebiten.Image, f engine.Food, x, y float32, tint color.RGBA) bool {
	if g.tiles == nil {
		return false
	}
	clip, at := itemClip(f)
	img := g.tiles.frame(clip, at)
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
	if g.tiles.tint[clip] {
		op.ColorScale.ScaleWithColor(tint)
	}
	op.Filter = ebiten.FilterNearest
	if scale < 1 {
		op.Filter = ebiten.FilterLinear
	}
	screen.DrawImage(img, op)
	return true
}

// itemClip is which picture stands for one thing lying in the world.
//
// Almost always the strip of kinds below, and the one exception is what is
// left of a person. The engine has drawn that line since the day meat
// existed - a carcass remembers whose kind it came from, because nobody eats
// its own dead - and the viewer drew both sides of it with the same drumstick
// until 2026-09-21, which made a field after a hard winter read as somebody's
// larder. Nothing about the rules changed; the picture caught up with them.
func itemClip(f engine.Food) (string, int) {
	if f.Kind == engine.FoodMeat && f.From == engine.SpeciesHuman {
		return "remains", 0
	}
	return "item", itemFrame(f.Kind)
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
