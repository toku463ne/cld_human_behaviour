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
// earns its keep: what a body is doing and what is being done to it decide
// which clip is asked for, so a state nobody drew is a hole that only shows
// up when a body happens to be in it on screen.
//
// Every action, both sexes, every row of every table, every circumstance, and
// every age. Most of the keys this can ask for came into being long after the
// first sheet was drawn - the day the sexes started picking, the day the
// builds did, the day being hit and being in a river did - and each time, a
// sheet that looked complete had holes in it that nothing but this found.
func TestEveryActionHasAPictureToDrawItWith(t *testing.T) {
	m := theManifest(t)
	have := map[string]int{}
	for _, c := range m.Clips {
		have[c.Name] = c.Frames
	}
	looks := []look{
		{},
		{struck: true},
		{wading: true},
		{aloft: true},
		{struck: true, wading: true},
	}
	ages := []struct {
		name string
		with func(*engine.Agent, *engine.Config)
	}{
		{"a child", func(a *engine.Agent, cfg *engine.Config) { a.Maturity = 0 }},
		{"an adult", func(a *engine.Agent, cfg *engine.Config) { a.Maturity = 1 }},
		{"someone old", func(a *engine.Agent, cfg *engine.Config) {
			a.Maturity = 1
			a.Age = int((cfg.SenescenceYears + 20) * float64(cfg.TicksPerYear))
		}},
	}
	for world, cfg := range worlds() {
		for kind := engine.ActionKind(0); kind < 32; kind++ {
			for _, species := range []engine.Species{engine.SpeciesHuman, engine.SpeciesEnemy} {
				for _, sex := range []engine.Sex{engine.Male, engine.Female} {
					for row := 0; row <= len(cfg.EnemyKinds); row++ {
						for _, l := range looks {
							for _, age := range ages {
								if l.aloft && species != engine.SpeciesEnemy {
									continue // only a beast is ever off the ground
								}
								a := &engine.Agent{
									Species: species, Sex: sex, Kind: uint8(row),
									Action: engine.Action{Kind: kind},
								}
								age.with(a, &cfg)
								name := clipFor(a, &cfg, l)
								if have[name] == 0 {
									t.Fatalf("%s: %v (%v, %v, row %d, %+v, %s) wants %q",
										world, kind, species, sex, row, l, age.name, name)
								}
							}
						}
					}
				}
			}
		}
	}
}

// And a body that has stopped being one.
//
// Its own test because the world has already forgotten it by then: the engine
// compacts the dead out every tick, so this picture is chosen from what the
// viewer kept rather than from anything that can be asked for.
func TestABodyThatHasFallenHasAPicture(t *testing.T) {
	have := map[string]int{}
	for _, c := range theManifest(t).Clips {
		have[c.Name] = c.Frames
	}
	for world, cfg := range worlds() {
		for _, species := range []engine.Species{engine.SpeciesHuman, engine.SpeciesEnemy} {
			for _, sex := range []engine.Sex{engine.Male, engine.Female} {
				for row := 0; row <= len(cfg.EnemyKinds); row++ {
					a := &engine.Agent{Species: species, Sex: sex, Kind: uint8(row)}
					if name := deadClipFor(a, &cfg); have[name] == 0 {
						t.Fatalf("%s: a dead %v (%v, row %d) wants %q", world, species, sex, row, name)
					}
				}
			}
		}
	}
}

// Circumstance beats action, and the order between the circumstances is the
// one the eye needs.
//
// The order is the whole of this function's design, so it is the thing worth
// pinning: a body being hit is what a player is watching for, and a body in
// water is in danger, and a child is only small.
func TestWhatIsHappeningToABodyBeatsWhatItIsDoing(t *testing.T) {
	cfg := engine.DefaultConfig()
	walking := engine.Action{Kind: engine.ActMove}
	swinging := engine.Action{Kind: engine.ActAttack}
	adult := func(a *engine.Agent) *engine.Agent { a.Maturity = 1; return a }

	cases := []struct {
		what string
		a    *engine.Agent
		l    look
		want string
	}{
		{"walking", adult(&engine.Agent{Action: walking}), look{}, "human.walk"},
		{"walking and hit", adult(&engine.Agent{Action: walking}), look{struck: true}, "human.hurt"},
		{"walking in water", adult(&engine.Agent{Action: walking}), look{wading: true}, "human.swim"},
		{"hit in water", adult(&engine.Agent{Action: walking}), look{struck: true, wading: true}, "human.hurt"},
		// Throwing the blow beats taking one: a body doing both is more
		// legible as the one going forward.
		{"swinging and hit", adult(&engine.Agent{Action: swinging}), look{struck: true}, "human.fight"},
		// And a child is only small, so anything at all outranks it.
		{"a child standing", &engine.Agent{Maturity: 0}, look{}, "child.idle"},
		{"a child walking", &engine.Agent{Maturity: 0, Action: walking}, look{}, "human.walk"},
		{"a child in water", &engine.Agent{Maturity: 0}, look{wading: true}, "human.swim"},
	}
	for _, c := range cases {
		if got := clipFor(c.a, &cfg, c.l); got != c.want {
			t.Errorf("%s came out %q and should be %q", c.what, got, c.want)
		}
	}
}

// Someone old is drawn old only where the world has ageing in it.
//
// The line is the engine's own - the same figure that already makes an old
// body draw smaller - so a world with the rule switched off has nobody old in
// it and asks the sheet for nothing.
func TestNobodyIsOldInAWorldWithoutAgeing(t *testing.T) {
	on := engine.DefaultConfig()
	off := engine.DefaultConfig()
	off.SenescenceRate = 0
	old := func() *engine.Agent {
		return &engine.Agent{Maturity: 1, Age: int((on.SenescenceYears + 20) * float64(on.TicksPerYear))}
	}
	if got := clipFor(old(), &on, look{}); got != "old.idle" {
		t.Errorf("with ageing on, an old body came out %q", got)
	}
	if got := clipFor(old(), &off, look{}); got != "human.idle" {
		t.Errorf("with ageing off, the same body came out %q", got)
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
