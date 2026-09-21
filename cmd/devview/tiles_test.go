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

// worlds is the shapes of world this viewer can be pointed at, as far as the
// pictures are concerned. A beast's build is read off its row in EnemyKinds
// (enemyBuild), so a sheet that is complete for one table can have holes in
// another, and the table below is the one cmd/devview actually offers.
func worlds() map[string]engine.Config {
	plain := engine.DefaultConfig()
	kinds := engine.DefaultConfig()
	kinds.EnemyKinds = []engine.EnemyKind{
		{Name: "stray", Share: 3, BudgetMean: 380},
		{Name: "brute", Share: 1, BudgetMean: 700},
	}
	beasts := engine.DefaultConfig()
	beasts.EnemyKinds = []engine.EnemyKind{
		{Name: "brute", Share: 2, BudgetMean: 700},
		{Name: "stray", Share: 3, BudgetMean: 380},
		{Name: "flyer", Share: 2, BudgetMean: 260, Flies: true, FlyHeight: 2},
		{Name: "lurker", Share: 1, BudgetMean: 450, Water: true},
	}
	return map[string]engine.Config{"plain": plain, "-kinds": kinds, "-beasts": beasts}
}

// Every picture the viewer can ask for is in the sheet. This is the test that
// earns its keep: what a body is doing decides which clip is asked for, so a
// state nobody drew is a hole that only shows up when a body happens to do
// that thing on screen.
//
// Both sexes and every row of every table, because both pick the clip now.
// Half the keys this can ask for came into being the day the sexes started
// picking, and the rest the day the builds did; a sheet drawn for one sex, or
// for one sort of beast, would have looked complete until the first woman or
// the first winged thing on the screen did something.
func TestEveryActionHasAPictureToDrawItWith(t *testing.T) {
	m := theManifest(t)
	have := map[string]int{}
	for _, c := range m.Clips {
		have[c.Name] = c.Frames
	}
	for world, cfg := range worlds() {
		for kind := engine.ActionKind(0); kind < 32; kind++ {
			for _, species := range []engine.Species{engine.SpeciesHuman, engine.SpeciesEnemy} {
				for _, sex := range []engine.Sex{engine.Male, engine.Female} {
					for row := 0; row <= len(cfg.EnemyKinds); row++ {
						for _, aloft := range []bool{false, true} {
							if aloft && species != engine.SpeciesEnemy {
								continue // only a beast is ever off the ground
							}
							a := &engine.Agent{
								Species: species, Sex: sex, Kind: uint8(row),
								Action: engine.Action{Kind: kind},
							}
							name := clipFor(a, &cfg, aloft)
							if have[name] == 0 {
								t.Fatalf("%s: %v (%v, %v, row %d, aloft %v) wants %q",
									world, kind, species, sex, row, aloft, name)
							}
						}
					}
				}
			}
		}
	}
}

// The four builds are four pictures, and which one a beast gets is read off
// what the world says the sort is rather than off what the map called it.
//
// The names are the map's and the properties are the row's (decision #133).
// A map that calls its heavy beast something else still has to get the heavy
// picture, and one that invents a sort nobody anticipated has to get a
// picture rather than a hole - which is the last case here.
func TestABeastIsDrawnAsWhatTheWorldSaysItIs(t *testing.T) {
	cfg := engine.DefaultConfig()
	cfg.EnemyKinds = []engine.EnemyKind{
		{Name: "anything at all", BudgetMean: cfg.EnemyBudgetMean + 100},
		{Name: "anything at all", BudgetMean: cfg.EnemyBudgetMean - 100},
		{Name: "anything at all", Water: true},
		{Name: "anything at all", Flies: true},
		{Name: "anything at all"}, // says nothing, so the world's own size
	}
	want := []string{"big", "small", "water", "fly", "small"}
	for row, w := range want {
		a := &engine.Agent{Species: engine.SpeciesEnemy, Kind: uint8(row)}
		if got := enemyBuild(a, &cfg); got != w {
			t.Errorf("row %d came out %q and should be %q", row, got, w)
		}
	}
	// And a row nobody wrote, which is what a saved world from before a map
	// shortened its table looks like.
	beyond := &engine.Agent{Species: engine.SpeciesEnemy, Kind: 200}
	if got := enemyBuild(beyond, &cfg); got != "small" {
		t.Errorf("a body from off the end of the table came out %q", got)
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
