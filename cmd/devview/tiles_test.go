package main

import (
	"encoding/json"
	"image/color"
	"testing"

	"github.com/toku463ne/cld_human_behaviour/engine"
)

// theManifest is what the viewer will be reading at startup. The sheet itself
// cannot be tested here - cutting it up needs a graphics device, and there is
// none in a test - but everything that decides which picture is asked for can
// be, and that is where a missing picture actually comes from.
func theManifest(t *testing.T) manifestFile {
	t.Helper()
	raw, err := loadAsset("manifest.json")
	if err != nil {
		t.Fatalf("the art is not where the program will look for it: %v", err)
	}
	var m manifestFile
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if m.Sheet == "" || m.Tile <= 0 || len(m.Clips) == 0 {
		t.Fatalf("manifest says %+v", m)
	}
	if _, err := loadAsset(m.Sheet); err != nil {
		t.Fatalf("the manifest names a sheet that is not there: %v", err)
	}
	return m
}

// Every picture the viewer can ask for is in the sheet. This is the test that
// earns its keep: what a body is doing decides which clip is asked for, so a
// state nobody drew is a hole that only shows up when a body happens to do
// that thing on screen.
func TestEveryActionHasAPictureToDrawItWith(t *testing.T) {
	m := theManifest(t)
	have := map[string]int{}
	for _, c := range m.Clips {
		have[c.Name] = c.Frames
	}
	for kind := engine.ActionKind(0); kind < 32; kind++ {
		for _, species := range []engine.Species{engine.SpeciesHuman, engine.SpeciesEnemy} {
			a := &engine.Agent{Species: species, Action: engine.Action{Kind: kind}}
			name := clipFor(a)
			if have[name] == 0 {
				t.Fatalf("%v (%v) wants %q, and the sheet has %v", kind, species, name, have)
			}
		}
	}
}

// And every kind of thing that can be lying about has one, at its own place
// in the strip: two kinds sharing a picture would be a coin that looks like a
// stone, which is exactly the sort of thing nobody notices in a screenshot.
func TestEveryKindOfThingHasItsOwnPicture(t *testing.T) {
	m := theManifest(t)
	var items int
	for _, c := range m.Clips {
		if c.Name == "item" {
			items = c.Frames
		}
	}
	if items == 0 {
		t.Fatal("the sheet has no things lying about in it")
	}
	seen := map[int]engine.FoodKind{}
	for kind := engine.FoodKind(0); kind < engine.NumFoodKinds; kind++ {
		if kind == engine.NumEdibleKinds {
			continue // the line between edible and not, and not a kind
		}
		at := itemFrame(kind)
		if at < 0 || at >= items {
			t.Fatalf("%v is drawn with frame %d of %d", kind, at, items)
		}
		if other, ok := seen[at]; ok {
			t.Fatalf("%v and %v are drawn with the same picture", kind, other)
		}
		seen[at] = kind
	}
}

// A body's colour is its sex, moved a little by which body it is. Both halves
// matter: a crowd of one colour looks like one thing repeated, and a shift
// big enough to read as a different sex would be saying something false.
func TestTheTintVariesByBodyWithoutLosingTheSex(t *testing.T) {
	base := color.RGBA{0x2a, 0x78, 0xd6, 0xff}
	seen := map[color.RGBA]bool{}
	for id := 1; id <= 60; id++ {
		a := &engine.Agent{ID: id}
		got := bodyTint(a, base)
		if got != bodyTint(a, base) {
			t.Fatal("the same body was given two colours")
		}
		if got.A != base.A {
			t.Fatalf("#%d came out at alpha %d", id, got.A)
		}
		for _, d := range []int{int(got.R) - int(base.R), int(got.G) - int(base.G), int(got.B) - int(base.B)} {
			if d < -16 || d > 15 {
				t.Fatalf("#%d moved a channel by %d", id, d)
			}
		}
		seen[got] = true
	}
	if len(seen) < 30 {
		t.Fatalf("sixty bodies came out in %d colours", len(seen))
	}
}
