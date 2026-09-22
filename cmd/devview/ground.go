package main

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/toku463ne/cld_human_behaviour/engine"
)

// Drawing the country with pictures (2026-09-22).
//
// The bodies stopped being circles in 2026-09-20 and the ground did not: it
// was four flat washes of colour, which said what a cell cost and nothing
// about what sort of place it was. This draws it instead, and the whole of
// what is new is here - the engine has never heard of a tile and still has
// not.
//
// Four things were decided before any of it was drawn, and each of them is a
// thing the art cannot do and the rules can.
//
//   - The picture never says a number. Which level a cell is on, how rich it
//     is, how cold - those are still washes over the top, because a wash can
//     say "two levels up" and a drawing of a rock cannot. What the picture
//     says is what KIND of place it is, which is the one thing the washes
//     were bad at.
//   - A block of high ground is drawn as a face and a rim: the middle of it
//     carries on past its own edges and the cells that look out over lower
//     ground carry the drop. That is how a great tree several cells across
//     reads as one tree rather than as four logs, and it is why the rim
//     pieces are three (a side, an outer corner, an inner corner) and are
//     turned rather than drawn four times over.
//   - From level three up the ground is a thing rather than a place. It is
//     not a new rule - canStep has refused a climb of two levels since stage
//     20, so reaching a level-three cell needs three ramps in a row and no
//     map will ever lay them - it is the rules read out loud: what nobody can
//     stand on is drawn as a trunk or a boulder, and what they can is drawn
//     as ground.
//   - Every plain cell is turned and mirrored by a hash of where it is. This
//     is the cheapest thing in the file and it was worth more than any of the
//     art: three pictures of forest floor laid down in a grid read as
//     wallpaper, and the same three in eight orientations read as forest. The
//     hash is the coordinates, never the world's own random numbers, because
//     a viewer that drew a number would change every measurement ever made in
//     this world.
//
// A set of art that has none of these clips falls back to the washes, which
// is what set1 and set2 get.

// Ground clips. Named here rather than spelt out at each use, so that the
// list the gap report walks (groundClips) and the list the drawing asks for
// cannot drift apart.
const (
	groundPrefix = "ground."

	clipFlat  = "ground.flat."  // a, b, c
	clipRough = "ground.rough." // a, b, c
	clipWater = "ground.water." // a, b - two frames each
	clipHigh  = "ground.high."  // a, b, c: the top of ground a body can stand on
	clipBlock = "ground.block." // a, b, rock: what it cannot

	clipBank  = "ground.bank"  // .side .outer .inner - where land meets water
	clipCliff = "ground.cliff" // .side .outer .inner - where ground drops away
	clipScree = "ground.scree" // the same rocks, at the foot of a boulder
	clipRoot  = "ground.root"  // .side .outer .inner - the foot of a block
	clipSlope = "ground.slope" // .a .b .c and .free
	clipNest  = "ground.nest"
)

// blockLevel is the level from which ground is drawn as a thing standing
// there rather than as ground to walk on, and rockLevel the one from which
// that thing is stone rather than wood.
//
// Both are conventions between whoever draws a map and this file, not rules:
// the engine reads a height and knows nothing about trunks. They are written
// down in tiled/properties.md, which is where a map's author looks.
const (
	blockLevel = 3
	rockLevel  = 5
)

// spot is the little about one cell that picking a picture depends on.
type spot struct {
	kind   engine.Ground
	height int
	slope  bool
}

// stamp is one picture and how it is put down.
type stamp struct {
	clip string
	turn int  // quarter turns clockwise
	flip bool // mirrored left to right, after the turn
}

// The four sides and the four diagonals, in the order everything here counts
// them. A quarter turn clockwise adds one to a side and to a diagonal alike,
// which is the whole arithmetic of turning a rim piece.
const (
	north = iota
	east
	south
	west
)

// groundStamp is which picture a cell is drawn with, and how it is turned.
//
// Pure: it is given the cell, its four neighbours, its four diagonals and
// where it is, and nothing else - and where it is only through a hash of the
// coordinates. That is what lets a test lay out a map on paper
// and check that the rim of a plateau goes round the right way, which is the
// part of this that is easy to get wrong and impossible to see wrong at
// thirty-two pixels.
func groundStamp(here spot, side, diag [4]spot, col, row int) stamp {
	h := cellHash(col, row)
	switch {
	case here.kind == engine.GroundWater:
		// Water is turned about like the plain ground: it has no up and the
		// grid shows through it worse than through anything else.
		return stamp{clip: pick(h, clipWater, "a", "b"), turn: turnOf(h), flip: flipOf(h)}

	case here.slope:
		// A ramp has a direction and the engine does not know it: a slope
		// cell is a height and a flag. The map knows, though - it is
		// whichever neighbour is higher - so the steps are turned to face it.
		// Where nothing says which way is up, the one drawn without a
		// direction is used instead of guessing.
		if up, ok := upFrom(here, side); ok {
			return stamp{clip: pick(h, clipSlope, ".a", ".b", ".c"), turn: up}
		}
		return stamp{clip: clipSlope + ".free"}

	case here.height >= blockLevel:
		// Roots at the foot of a tree and fallen rock at the foot of a
		// boulder: the same three pieces either way, and which family they
		// come from follows what the block itself is drawn as.
		family := clipRoot
		if here.height >= rockLevel {
			// Fallen rock rather than roots, and in the floor's light
			// rather than the ledge's: a boulder stands on the floor, and
			// the ledge's pale green round one reads as a halo.
			family = clipScree
		}
		if s, ok := rim(here, side, diag, family); ok {
			return s
		}
		if here.height >= rockLevel {
			return stamp{clip: clipBlock + "rock"}
		}
		// Not turned. A trunk is drawn with the grain running one way and
		// turning it would cross the grain against the cell beside it, which
		// is the one place the eye reads a block as several tiles again.
		return stamp{clip: pick(h, clipBlock, "a", "b")}

	case here.height > 0:
		if s, ok := rim(here, side, diag, clipCliff); ok {
			return s
		}
		return stamp{clip: pick(h, clipHigh, "a", "b", "c"), turn: turnOf(h), flip: flipOf(h)}
	}

	// Level ground. The shore is drawn on the land side, so that the water
	// keeps its own cells whole and the line between them lands on the edge
	// they share.
	if s, ok := edge(here, side, diag, clipBank, isWater); ok {
		return s
	}
	if here.kind == engine.GroundRough {
		return stamp{clip: pick(h, clipRough, "a", "b", "c"), turn: turnOf(h), flip: flipOf(h)}
	}
	return stamp{clip: pick(h, clipFlat, "a", "b", "c"), turn: turnOf(h), flip: flipOf(h)}
}

// rim is the piece for a cell that looks out over lower ground, if it does.
func rim(here spot, side, diag [4]spot, family string) (stamp, bool) {
	drops := func(there spot) bool {
		// A ramp is the way down, not a drop: the one side of a plateau a
		// body can leave by is the one side that must not be drawn as a wall.
		return there.height < here.height && !there.slope
	}
	return edge(here, side, diag, family, drops)
}

// edge picks the rim piece for whichever sides answer to the test.
//
// Three pieces cover what a rectangle of anything can present to its
// neighbours: a straight side, an outer corner where two sides meet, and an
// inner corner where a single diagonal is missing. What they do not cover -
// two opposite sides, three sides, a cell alone - is drawn as a single side,
// because a one-cell-wide isthmus is a map nobody has drawn and a missing
// piece would be a hole.
func edge(here spot, side, diag [4]spot, family string, out func(spot) bool) (stamp, bool) {
	var on []int
	for i, s := range side {
		if out(s) {
			on = append(on, i)
		}
	}
	switch len(on) {
	case 0:
		// Nothing beside it, but perhaps something at a corner: the inside
		// of an L, where the cell itself is whole and the diagonal is not.
		for d, s := range diag {
			if out(s) && !out(side[d]) && !out(side[(d+1)%4]) {
				return stamp{clip: family + ".inner", turn: (d + 1) % 4}, true
			}
		}
		return stamp{}, false
	case 1:
		return stamp{clip: family + ".side", turn: on[0]}, true
	case 2:
		if a, b := on[0], on[1]; (b-a)%4 == 1 || (a-b+4)%4 == 1 {
			// The piece is drawn with its two sides at north and west, so
			// the turn is whatever takes {north, west} onto this pair.
			if a == north && b == west {
				return stamp{clip: family + ".outer"}, true
			}
			return stamp{clip: family + ".outer", turn: a + 1}, true
		}
	}
	return stamp{clip: family + ".side", turn: on[0]}, true
}

// upFrom is which side of a ramp is the top of it, if any side is higher than
// another. The diagonals are not asked: a ramp is entered from a side, and a
// corner that happens to be higher says nothing about which way the steps run.
func upFrom(here spot, side [4]spot) (int, bool) {
	high, low := 0, 0
	for i, s := range side {
		if s.height > side[high].height {
			high = i
		}
		if s.height < side[low].height {
			low = i
		}
	}
	if side[high].height <= side[low].height {
		return 0, false // flat all round: nothing says which way is up
	}
	return high, true
}

func isWater(s spot) bool { return s.kind == engine.GroundWater }

// pick is one of a clip's variants, from the hash. The low bits, and the turn
// takes higher ones: drawing the variant and the turn from the same bits
// would tie them together and lay the three pictures down in a pattern.
func pick(h uint32, family string, names ...string) string {
	return family + names[int(h%uint32(len(names)))]
}

func turnOf(h uint32) int  { return int(h >> 8 & 3) }
func flipOf(h uint32) bool { return h>>10&1 == 1 }
func cellHash(col, row int) uint32 {
	// Any cheap mixing will do; what matters is that it is the coordinates
	// and not the world's own random numbers, so that what is drawn cannot
	// change what is simulated.
	h := uint32(col)*2654435761 + uint32(row)*2246822519
	h ^= h >> 13
	h *= 3266489917
	return h ^ h>>16
}

// groundClips is every picture this file can ask for. The gap report walks it
// (artgaps.go), which is how a set that is short of one says so instead of
// quietly falling back to the washes.
func groundClips() []string {
	out := []string{
		clipSlope + ".free", clipBlock + "rock", clipNest,
	}
	for _, v := range []string{"a", "b", "c"} {
		out = append(out, clipFlat+v, clipRough+v, clipHigh+v)
	}
	out = append(out, clipWater+"a", clipWater+"b", clipBlock+"a", clipBlock+"b")
	for _, v := range []string{".a", ".b", ".c"} {
		out = append(out, clipSlope+v)
	}
	for _, family := range []string{clipBank, clipCliff, clipRoot, clipScree} {
		for _, v := range []string{".side", ".outer", ".inner"} {
			out = append(out, family+v)
		}
	}
	return out
}

// hasGround says whether this set of art can draw the country at all. One
// clip would be enough to ask the question and is not enough to draw with, so
// it asks for all of them: a half-drawn ground is worse than the washes,
// which at least say the same thing everywhere.
func (t *tileset) hasGround() bool {
	if t == nil {
		return false
	}
	for _, name := range groundClips() {
		if len(t.clips[name]) == 0 {
			return false
		}
	}
	return true
}

// --- putting it on the screen ----------------------------------------------

// groundCell is how big a cell of the country is in a world that has no map
// at all. A map's own cells are 27 by 30 on the one the game is played on, so
// this is the same order, and a flat world gets forest floor instead of the
// white it had.
const groundCell = 32.0

// drawGround stamps the country. It reports whether it drew anything, so that
// a set of art without the pictures falls back to the washes.
func (g *game) drawGround(screen *ebiten.Image) bool {
	if !g.tiles.hasGround() {
		return false
	}
	cols, rows, cw, ch := g.world.TerrainSize()
	if cols == 0 {
		// No map: one flat kind everywhere, on a grid of this file's own.
		cfg := g.world.Config()
		cols, rows = int(math.Ceil(cfg.Width/groundCell)), int(math.Ceil(cfg.Height/groundCell))
		cw, ch = groundCell, groundCell
	}
	at := make([]spot, cols*rows)
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			t := g.world.TerrainAt((float64(col)+0.5)*cw, (float64(row)+0.5)*ch)
			at[row*cols+col] = spot{kind: t.Kind, height: t.Height, slope: t.Slope}
		}
	}
	// Off the edge of the map is level open ground, which is what the engine
	// says too (flatGround), so the rim of a plateau at the edge is drawn.
	get := func(col, row int) spot {
		if col < 0 || row < 0 || col >= cols || row >= rows {
			return spot{}
		}
		return at[row*cols+col]
	}
	frame := g.world.Tick() / animTicks
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			x0, y0 := g.onScreen(float64(col)*cw, float64(row)*ch)
			x1, y1 := g.onScreen(float64(col+1)*cw, float64(row+1)*ch)
			if x1 < 0 || y1 < 0 || x0 > worldWidth || y0 > worldHeight {
				continue // off the screen: the played zoom shows a fifth of it
			}
			here := get(col, row)
			side := [4]spot{get(col, row-1), get(col+1, row), get(col, row+1), get(col-1, row)}
			diag := [4]spot{get(col+1, row-1), get(col+1, row+1), get(col-1, row+1), get(col-1, row-1)}
			s := groundStamp(here, side, diag, col, row)
			g.stampGround(screen, s, frame, x0, y0, x1-x0, y1-y0)

			// Every edge a body cannot step over, drawn as a line.
			//
			// Brightness says WHICH of the two levels a cell is on and
			// nothing else - a ledge is one step lighter than the floor of
			// the world and no ledge is lighter than another (PALETTE, in
			// the packer) - so this is what says where the boundary between
			// them runs, and how many of them there are. It replaced a wash
			// that grew paler with every level: four levels of pale made
			// four middling greens, and none of them read as being up
			// somewhere.
			//
			// Nothing is drawn round a ramp. A ramp stands on the high side
			// and so it has a drop under it like any other high cell, but a
			// line at the foot of a flight of steps reads as a kerb across
			// the one place a body can climb.
			if here.slope {
				continue
			}
			for d, there := range side {
				if there.height >= here.height || there.slope {
					continue
				}
				drawDrop(screen, d, x0, y0, x1, y1)
			}
		}
	}
	return true
}

// stampAt puts one ground picture centred on a point, the size of a cell. It
// reports whether it drew, so a set without the picture keeps what it had.
func (g *game) stampAt(screen *ebiten.Image, clip string, x, y, size float32) bool {
	if g.tiles == nil || len(g.tiles.clips[clip]) == 0 {
		return false
	}
	g.stampGround(screen, stamp{clip: clip}, 0, x-size/2, y-size/2, size, size)
	return true
}

// stampGround puts one picture in one cell, turned as the stamp says.
func (g *game) stampGround(screen *ebiten.Image, s stamp, frame int, x, y, w, h float32) {
	img := g.tiles.frame(s.clip, frame)
	if img == nil {
		return
	}
	iw, ih := img.Bounds().Dx(), img.Bounds().Dy()
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(-float64(iw)/2, -float64(ih)/2)
	if s.flip {
		op.GeoM.Scale(-1, 1)
	}
	if s.turn != 0 {
		op.GeoM.Rotate(float64(s.turn) * math.Pi / 2)
	}
	op.GeoM.Scale(float64(w)/float64(iw), float64(h)/float64(ih))
	op.GeoM.Translate(float64(x)+float64(w)/2, float64(y)+float64(h)/2)
	// Nearest, at every size, which is not what a body is drawn with. A body
	// shrunk below its own size crawls under nearest and is averaged instead;
	// a tile cannot be, because averaging samples across the edge of its
	// picture and what is over that edge is the rest of the sheet. On the map
	// the game is played on a cell comes out a little under 32 and the world
	// was drawn with a pale grid over it, one line per cell.
	op.Filter = ebiten.FilterNearest
	screen.DrawImage(img, op)
}

// drawDrop is the edge of a ledge: a hard line on the boundary, and two
// fading bands inside it.
//
// The hard line alone looked drawn on rather than part of the country - a
// pen stroke over a photograph of leaves. The bands are what a bank of earth
// actually does to the light near its edge, and two of them at a third of
// each other's strength is enough for the eye to read the line as the top of
// something rather than as ink. They go INSIDE the high cell, never over the
// low one: what is below the drop is a different place and nothing about the
// ledge belongs on it.
func drawDrop(screen *ebiten.Image, d int, x0, y0, x1, y1 float32) {
	w, h := x1-x0, y1-y0
	thin := w
	if h < thin {
		thin = h
	}
	// Everything is a share of the cell, because a cell is 27 pixels at the
	// zoom the game is played at and 8 with the whole world on screen.
	band := func(from, thick float32, c color.RGBA) {
		if thick < 1 {
			thick = 1
		}
		switch d {
		case north:
			vector.DrawFilledRect(screen, x0, y0+from, w, thick, c, false)
		case south:
			vector.DrawFilledRect(screen, x0, y1-from-thick, w, thick, c, false)
		case west:
			vector.DrawFilledRect(screen, x0+from, y0, thick, h, c, false)
		case east:
			vector.DrawFilledRect(screen, x1-from-thick, y0, thick, h, c, false)
		}
	}
	edge := thin / 14
	if edge < 1.5 {
		edge = 1.5
	}
	band(0, edge, colorDrop)
	band(edge, thin/9, colorDropInner)
	band(edge+thin/9, thin/7, colorDropFade)
}
