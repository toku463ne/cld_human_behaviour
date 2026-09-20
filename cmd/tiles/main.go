// Command tiles draws the sprite sheet the viewer uses and writes it out
// with a manifest (TODO 10).
//
// Why the pictures are in a program rather than in a paint package: this
// project has no artist, and the one thing it does have is a habit of writing
// maps as characters with a legend - the terrain, the regions, the weather,
// the plants and the beasts are all drawn that way already (decision #133).
// A sprite is the same thing one step smaller, one character to a pixel, so
// the art lives where everything else about this world lives: in text a
// person can edit in the same editor as the rules.
//
// Everything here is drawn in greys. What colour anything ends up is the
// viewer's business (ColorScale), and it has to be, because the colour says
// what the thing is and who it belongs to: a body is tinted by its sex and by
// a shade of its own, and a meal is tinted by what kind of meal it is. Art
// with colour baked in would need one picture per case.
//
// The output is one sheet and one manifest. One sheet because everything
// drawn from a single image is one draw call to the hardware however many
// times it is stamped, and 300 bodies a frame is the figure this stage was
// counted against. The manifest is what says where each frame is in it, and
// the sheet's name carries a hash of its own bytes so that a browser may keep
// it for ever and still never show a stale one.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// tile is the side of one cell, in pixels. Sixteen is what a body is drawn at
// on this screen at the close zoom, so the art is never stretched much in
// either direction.
const tile = 16

// The greys. A pixel is either not there, the outline, the body, or a shade
// of the body - four values, because a sprite that is going to be multiplied
// by a colour cannot afford detail that the tint will flatten anyway.
var shades = map[byte]color.NRGBA{
	'.': {0, 0, 0, 0},
	'o': {48, 48, 56, 255},    // outline: dark whatever it is tinted
	'b': {255, 255, 255, 255}, // body: takes the tint whole
	's': {176, 176, 176, 255}, // shade: the same tint, darker
	'e': {80, 80, 88, 255},    // an eye, a seed, a hole
}

// clipArt is one run of pictures and the name it goes by: what a body is
// doing, drawn as many times as the doing takes.
type clipArt struct {
	name   string
	frames [][]string
}

// clip is a run of frames that belong together: what a body is doing, and how
// many pictures that takes.
type clip struct {
	Name   string `json:"name"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	W      int    `json:"w"`
	H      int    `json:"h"`
	Frames int    `json:"frames"`
}

// manifest is what the viewer reads before it draws anything.
type manifest struct {
	Sheet string `json:"sheet"`
	Tile  int    `json:"tile"`
	Clips []clip `json:"clips"`
}

func main() {
	out := flag.String("out", "cmd/devview/assets", "where to write the sheet and the manifest")
	flag.Parse()

	rows := sheet()
	width, height := 0, len(rows)*tile
	for _, r := range rows {
		if len(r.frames) > width {
			width = len(r.frames)
		}
	}
	img := image.NewNRGBA(image.Rect(0, 0, width*tile, height))
	var clips []clip
	for y, row := range rows {
		for x, art := range row.frames {
			if len(art) > tile {
				log.Fatalf("%s frame %d is %d rows, and a tile is %d", row.name, x, len(art), tile)
			}
			draw(img, x*tile, y*tile, art)
		}
		// The frames of one clip sit side by side, so the viewer needs one
		// rectangle and a count rather than a rectangle each.
		clips = append(clips, clip{Name: row.name, X: 0, Y: y * tile,
			W: tile, H: tile, Frames: len(row.frames)})
	}

	var buf strings.Builder
	if err := png.Encode(&stringWriter{&buf}, img); err != nil {
		log.Fatal(err)
	}
	data := []byte(buf.String())
	sum := sha256.Sum256(data)
	name := fmt.Sprintf("tiles.%x.png", sum[:4])

	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	// Whatever sheet was there before goes: the name carries the hash, so an
	// old one would sit in the directory for ever and be embedded with it.
	old, _ := filepath.Glob(filepath.Join(*out, "tiles.*.png"))
	for _, f := range old {
		if filepath.Base(f) != name {
			os.Remove(f)
		}
	}
	if err := os.WriteFile(filepath.Join(*out, name), data, 0o644); err != nil {
		log.Fatal(err)
	}
	m, err := json.MarshalIndent(manifest{Sheet: name, Tile: tile, Clips: clips}, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*out, "manifest.json"), append(m, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s: %d x %d, %d clips, %d bytes\n", name, width*tile, height, len(clips), len(data))
}

type stringWriter struct{ b *strings.Builder }

func (w *stringWriter) Write(p []byte) (int, error) { return w.b.Write(p) }

// draw stamps one picture into the sheet. A row shorter than the tile is
// padded with nothing, so the art below can leave off the trailing blanks.
func draw(img *image.NRGBA, ox, oy int, art []string) {
	for y, row := range art {
		for x, ch := range []byte(row) {
			c, ok := shades[ch]
			if !ok || c.A == 0 {
				continue
			}
			img.SetNRGBA(ox+x, oy+y, c)
		}
	}
}
