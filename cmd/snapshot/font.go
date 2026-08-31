package main

import (
	"image"
	"image/color"
	"image/draw"
)

// A 5x7 bitmap font, written out here rather than pulled from
// golang.org/x/image/font so that the tool stays on the standard library. It
// only has to render the caption strip: capitals, digits and a little
// punctuation, which is why lower case is folded to upper case on the way in.
//
// Each glyph is seven rows of five pixels, most significant bit leftmost.
const (
	glyphW = 5
	glyphH = 7
)

var glyphs = map[rune][glyphH]uint8{
	'A': {0b01110, 0b10001, 0b10001, 0b11111, 0b10001, 0b10001, 0b10001},
	'B': {0b11110, 0b10001, 0b10001, 0b11110, 0b10001, 0b10001, 0b11110},
	'C': {0b01110, 0b10001, 0b10000, 0b10000, 0b10000, 0b10001, 0b01110},
	'D': {0b11110, 0b10001, 0b10001, 0b10001, 0b10001, 0b10001, 0b11110},
	'E': {0b11111, 0b10000, 0b10000, 0b11110, 0b10000, 0b10000, 0b11111},
	'F': {0b11111, 0b10000, 0b10000, 0b11110, 0b10000, 0b10000, 0b10000},
	'G': {0b01110, 0b10001, 0b10000, 0b10111, 0b10001, 0b10001, 0b01111},
	'H': {0b10001, 0b10001, 0b10001, 0b11111, 0b10001, 0b10001, 0b10001},
	'I': {0b11111, 0b00100, 0b00100, 0b00100, 0b00100, 0b00100, 0b11111},
	'J': {0b00111, 0b00010, 0b00010, 0b00010, 0b00010, 0b10010, 0b01100},
	'K': {0b10001, 0b10010, 0b10100, 0b11000, 0b10100, 0b10010, 0b10001},
	'L': {0b10000, 0b10000, 0b10000, 0b10000, 0b10000, 0b10000, 0b11111},
	'M': {0b10001, 0b11011, 0b10101, 0b10101, 0b10001, 0b10001, 0b10001},
	'N': {0b10001, 0b11001, 0b10101, 0b10011, 0b10001, 0b10001, 0b10001},
	'O': {0b01110, 0b10001, 0b10001, 0b10001, 0b10001, 0b10001, 0b01110},
	'P': {0b11110, 0b10001, 0b10001, 0b11110, 0b10000, 0b10000, 0b10000},
	'Q': {0b01110, 0b10001, 0b10001, 0b10001, 0b10101, 0b10010, 0b01101},
	'R': {0b11110, 0b10001, 0b10001, 0b11110, 0b10100, 0b10010, 0b10001},
	'S': {0b01111, 0b10000, 0b10000, 0b01110, 0b00001, 0b00001, 0b11110},
	'T': {0b11111, 0b00100, 0b00100, 0b00100, 0b00100, 0b00100, 0b00100},
	'U': {0b10001, 0b10001, 0b10001, 0b10001, 0b10001, 0b10001, 0b01110},
	'V': {0b10001, 0b10001, 0b10001, 0b10001, 0b10001, 0b01010, 0b00100},
	'W': {0b10001, 0b10001, 0b10001, 0b10101, 0b10101, 0b11011, 0b10001},
	'X': {0b10001, 0b10001, 0b01010, 0b00100, 0b01010, 0b10001, 0b10001},
	'Y': {0b10001, 0b10001, 0b01010, 0b00100, 0b00100, 0b00100, 0b00100},
	'Z': {0b11111, 0b00001, 0b00010, 0b00100, 0b01000, 0b10000, 0b11111},
	'0': {0b01110, 0b10001, 0b10011, 0b10101, 0b11001, 0b10001, 0b01110},
	'1': {0b00100, 0b01100, 0b00100, 0b00100, 0b00100, 0b00100, 0b01110},
	'2': {0b01110, 0b10001, 0b00001, 0b00010, 0b00100, 0b01000, 0b11111},
	'3': {0b11111, 0b00010, 0b00100, 0b00010, 0b00001, 0b10001, 0b01110},
	'4': {0b00010, 0b00110, 0b01010, 0b10010, 0b11111, 0b00010, 0b00010},
	'5': {0b11111, 0b10000, 0b11110, 0b00001, 0b00001, 0b10001, 0b01110},
	'6': {0b00110, 0b01000, 0b10000, 0b11110, 0b10001, 0b10001, 0b01110},
	'7': {0b11111, 0b00001, 0b00010, 0b00100, 0b01000, 0b01000, 0b01000},
	'8': {0b01110, 0b10001, 0b10001, 0b01110, 0b10001, 0b10001, 0b01110},
	'9': {0b01110, 0b10001, 0b10001, 0b01111, 0b00001, 0b00010, 0b01100},
	'.': {0, 0, 0, 0, 0, 0, 0b00100},
	',': {0, 0, 0, 0, 0, 0b00100, 0b01000},
	':': {0, 0b00100, 0, 0, 0, 0b00100, 0},
	'-': {0, 0, 0, 0b11111, 0, 0, 0},
	'_': {0, 0, 0, 0, 0, 0, 0b11111},
	'/': {0b00001, 0b00001, 0b00010, 0b00100, 0b01000, 0b10000, 0b10000},
	'=': {0, 0, 0b11111, 0, 0b11111, 0, 0},
	'+': {0, 0b00100, 0b00100, 0b11111, 0b00100, 0b00100, 0},
	'(': {0b00010, 0b00100, 0b01000, 0b01000, 0b01000, 0b00100, 0b00010},
	')': {0b01000, 0b00100, 0b00010, 0b00010, 0b00010, 0b00100, 0b01000},
	'#': {0b01010, 0b11111, 0b01010, 0b01010, 0b01010, 0b11111, 0b01010},
	'%': {0b11001, 0b11010, 0b00010, 0b00100, 0b01000, 0b01011, 0b10011},
	'?': {0b01110, 0b10001, 0b00001, 0b00010, 0b00100, 0, 0b00100},
	' ': {0, 0, 0, 0, 0, 0, 0},
}

// textWidth is how wide drawText will draw s, at one pixel between glyphs.
func textWidth(s string, scale int) int {
	if len(s) == 0 {
		return 0
	}
	return (len([]rune(s))*(glyphW+1) - 1) * scale
}

// drawText writes s with its top left corner at (x, y). Anything the font does
// not have is drawn as a blank, so a stray character costs a gap and not a
// panic.
func drawText(dst *image.RGBA, x, y int, s string, c color.Color, scale int) {
	src := image.NewUniform(c)
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			r -= 'a' - 'A'
		}
		g := glyphs[r]
		for row := 0; row < glyphH; row++ {
			for col := 0; col < glyphW; col++ {
				if g[row]&(1<<(glyphW-1-col)) == 0 {
					continue
				}
				px := x + col*scale
				py := y + row*scale
				draw.Draw(dst, image.Rect(px, py, px+scale, py+scale), src, image.Point{}, draw.Over)
			}
		}
		x += (glyphW + 1) * scale
	}
}
