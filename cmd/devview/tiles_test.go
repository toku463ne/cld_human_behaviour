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
//
// Both sexes, because the sex picks the clip now as well as the action. Half
// the keys this can ask for came into being the day that started, and a sheet
// drawn for one sex would have looked complete until the first woman on the
// screen did something.
func TestEveryActionHasAPictureToDrawItWith(t *testing.T) {
	m := theManifest(t)
	have := map[string]int{}
	for _, c := range m.Clips {
		have[c.Name] = c.Frames
	}
	for kind := engine.ActionKind(0); kind < 32; kind++ {
		for _, species := range []engine.Species{engine.SpeciesHuman, engine.SpeciesEnemy} {
			for _, sex := range []engine.Sex{engine.Male, engine.Female} {
				a := &engine.Agent{Species: species, Sex: sex, Action: engine.Action{Kind: kind}}
				name := clipFor(a)
				if have[name] == 0 {
					t.Fatalf("%v (%v, %v) wants %q, and the sheet has %v", kind, species, sex, name, have)
				}
			}
		}
	}
}

// A man and a woman doing the same thing are drawn with different pictures.
//
// The one test standing where the colour used to. Sex was the fill of the
// circle from the first week of this viewer; a drawn body cannot be filled
// with a colour, and if this ever comes back green the fact has quietly left
// the screen rather than broken anything.
func TestTheSexesAreDrawnApart(t *testing.T) {
	have := map[string]bool{}
	for _, c := range theManifest(t).Clips {
		have[c.Name] = true
	}
	for kind := engine.ActionKind(0); kind < 32; kind++ {
		man := clipFor(&engine.Agent{Sex: engine.Male, Action: engine.Action{Kind: kind}})
		woman := clipFor(&engine.Agent{Sex: engine.Female, Action: engine.Action{Kind: kind}})
		if man == woman {
			t.Fatalf("%v draws both sexes with %q", kind, man)
		}
		if !have[man] || !have[woman] {
			t.Fatalf("%v wants %q and %q", kind, man, woman)
		}
	}
}

// Drawn art is never given a colour, and grey art always is.
//
// Multiplying a body that is already skin and hair and cloth by the sex blue
// turns it into a drowned one, and by the sex pink into something skinned -
// which is what this looked like when it was first tried. The rule is in the
// sheet rather than in the code, so this is where it is checked.
func TestOnlyTheGreyArtIsGivenAColour(t *testing.T) {
	for _, c := range theManifest(t).Clips {
		drawn := c.W == 32
		if drawn && c.Tint {
			t.Errorf("%s is drawn art and would be stained by a tint", c.Name)
		}
		if !drawn && !c.Tint {
			t.Errorf("%s is a grey placeholder and would be invisible without a tint", c.Name)
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

// What is left of a person is not drawn as a meal.
//
// The engine has told these apart since meat existed - a carcass remembers
// whose kind it came from, and nobody eats its own dead - and the viewer drew
// both with the same drumstick until 2026-09-21. This is the test that keeps
// them apart: it is the one place a rule the simulation already has reaches
// the only person who ever sees it.
func TestAPersonsRemainsAreNotDrawnAsAMeal(t *testing.T) {
	have := map[string]int{}
	for _, c := range theManifest(t).Clips {
		have[c.Name] = c.Frames
	}
	mine := engine.Food{Kind: engine.FoodMeat, From: engine.SpeciesHuman}
	theirs := engine.Food{Kind: engine.FoodMeat, From: engine.SpeciesEnemy}
	mineClip, mineAt := itemClip(mine)
	theirsClip, theirsAt := itemClip(theirs)
	if mineClip == theirsClip && mineAt == theirsAt {
		t.Fatalf("a person and a beast both come out as %s[%d]", mineClip, mineAt)
	}
	for _, c := range []string{mineClip, theirsClip} {
		if have[c] == 0 {
			t.Fatalf("nothing in the sheet is called %q", c)
		}
	}
	// And nothing that is not meat is diverted by it: the line is whose
	// carcass it is, not what the thing is.
	for kind := engine.FoodKind(0); kind < engine.NumFoodKinds; kind++ {
		if kind == engine.NumEdibleKinds || kind == engine.FoodMeat {
			continue
		}
		if c, _ := itemClip(engine.Food{Kind: kind, From: engine.SpeciesHuman}); c != "item" {
			t.Fatalf("%v came out as %q", kind, c)
		}
	}
}
