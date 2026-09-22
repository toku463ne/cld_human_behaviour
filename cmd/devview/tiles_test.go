package main

import (
	"image/color"
	"math/rand"
	"os"
	"strings"
	"testing"

	"github.com/toku463ne/cld_human_behaviour/engine"
)

// theArt is the sets of pictures the viewer will be choosing between, and
// theManifest is what it will read out of one. The sheet itself cannot be
// tested here - cutting it up needs a graphics device, and there is none in a
// test - but everything that decides which picture is asked for can be, and
// that is where a missing picture actually comes from.
func theArt(t *testing.T) artIndex {
	t.Helper()
	ix, err := artSets()
	if err != nil {
		t.Fatalf("the art is not where the program will look for it: %v", err)
	}
	return ix
}

func theManifest(t *testing.T) manifestFile { return manifestOf(t, theArt(t).Default) }

func manifestOf(t *testing.T, set string) manifestFile {
	t.Helper()
	m, err := readManifest(set)
	if err != nil {
		t.Fatalf("%s: %v", set, err)
	}
	if len(m.Clips) == 0 {
		t.Fatalf("%s: manifest says %+v", set, m)
	}
	if _, err := loadAsset(set + "/" + m.Sheet); err != nil {
		t.Fatalf("%s: the manifest names a sheet that is not there: %v", set, err)
	}
	return m
}

// What sets.json says is on offer is what is on disk.
//
// The file exists because a browser cannot list a directory - it fetches by
// name and has no way to ask what is there - so the list is written by hand,
// and a list written by hand is a list that goes stale. This is the only
// place the two can be compared, because only a test runs where both the
// directory and the program are.
func TestTheArtOnOfferIsTheArtThatIsThere(t *testing.T) {
	ix := theArt(t)
	listed := map[string]bool{}
	for _, set := range ix.Sets {
		if set.Note == "" {
			t.Errorf("%s says nothing about itself, and nothing about a picture will", set.Name)
		}
		if listed[set.Name] {
			t.Fatalf("%s is listed twice", set.Name)
		}
		listed[set.Name] = true
		manifestOf(t, set.Name) // it loads, and its sheet is beside it
	}
	if !listed[ix.Default] {
		t.Fatalf("the default is %q, which is not one of the sets", ix.Default)
	}
	found, err := os.ReadDir("assets")
	if err != nil {
		t.Fatalf("assets: %v", err)
	}
	for _, e := range found {
		if !e.IsDir() {
			continue
		}
		if !listed[e.Name()] {
			t.Errorf("assets/%s is art that sets.json does not offer", e.Name())
		}
		delete(listed, e.Name())
	}
	for name := range listed {
		t.Errorf("sets.json offers %s and there is no assets/%s", name, name)
	}
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
	for _, set := range theArt(t).Sets {
		t.Run(set.Name, func(t *testing.T) { everyActionIsDrawn(t, set.Name) })
	}
}

func everyActionIsDrawn(t *testing.T, set string) {
	m := manifestOf(t, set)
	have := map[string]int{}
	for _, c := range m.Clips {
		have[c.Name] = c.Frames
	}
	// An older set is allowed to be short of a picture, and says so by what
	// it stands in with; the set this is built to draw with is not.
	strict := set == theArt(t).Default
	has := func(n string) bool { return have[n] > 0 }
	asked := askedFor()
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
								// The gap report walks the same ground on its
								// own (askedFor), and a report that missed a
								// case would say a set was complete when it
								// was not, which is worse than no report.
								if asked[name] == 0 {
									t.Fatalf("%s: %v (%v, %v, row %d, %+v, %s) asks for %q and -artgaps does not know to look for it",
										world, kind, species, sex, row, l, age.name, name)
								}
								if have[name] > 0 {
									continue
								}
								if strict {
									t.Fatalf("%s: %v (%v, %v, row %d, %+v, %s) wants %q",
										world, kind, species, sex, row, l, age.name, name)
								}
								if stood := standIn(name, has); have[stood] == 0 {
									t.Fatalf("%s: %v (%v, %v, row %d, %+v, %s) wants %q, and %s stands in with %q, which it has not got",
										world, kind, species, sex, row, l, age.name, name, set, stood)
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
		// How old a body is outranks all of it, because it is not a
		// circumstance: a child that starts walking is still a child. It
		// was the other way round until 2026-09-22, when the art for the
		// other five poses arrived - until then a child that took a step
		// turned into an adult drawn small.
		{"a child standing", &engine.Agent{Maturity: 0}, look{}, "child.idle"},
		{"a child walking", &engine.Agent{Maturity: 0, Action: walking}, look{}, "child.walk"},
		{"a child in water", &engine.Agent{Maturity: 0}, look{wading: true}, "child.swim"},
		{"a child being hit", &engine.Agent{Maturity: 0}, look{struck: true}, "child.hurt"},
		{"a child swinging", &engine.Agent{Maturity: 0, Action: swinging}, look{}, "child.fight"},
		{"a girl walking", &engine.Agent{Maturity: 0, Sex: engine.Female, Action: walking}, look{}, "child.f.walk"},
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
	// And it stays old through everything it does, which is what the six
	// poses bought. A body that was old standing and an adult the moment it
	// walked was the one thing wrong with drawing age at all.
	walking := old()
	walking.Action = engine.Action{Kind: engine.ActMove}
	if got := clipFor(walking, &on, look{}); got != "old.walk" {
		t.Errorf("an old body that walked came out %q", got)
	}
	if got := clipFor(old(), &on, look{wading: true}); got != "old.swim" {
		t.Errorf("an old body in water came out %q", got)
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

// An older set draws a walking child with the grown picture, which is what
// every version of this viewer did until the day the young were drawn moving.
//
// This is what makes the sets swappable at all. Without it, switching to art
// drawn before some distinction was made puts circles back on the screen
// wherever a body falls into that distinction, which reads as a broken viewer
// rather than as older art. The order things are given up in is the design:
// age first, because it is only wrong about size and the viewer says the size
// itself; the pose last, because a body standing still while it walks is the
// one the eye catches.
func TestOlderArtStandsInWithWhatItHas(t *testing.T) {
	// A set from before the young and the old were drawn doing anything.
	older := map[string]bool{}
	for _, n := range []string{"human.idle", "human.walk", "human.f.idle", "human.f.walk",
		"child.idle", "child.f.idle", "old.idle", "old.f.idle",
		"enemy.big.idle", "enemy.big.walk"} {
		older[n] = true
	}
	has := func(n string) bool { return older[n] }
	for _, c := range []struct{ want, from string }{
		{"human.walk", "child.walk"},        // a child that started walking
		{"human.f.walk", "child.f.walk"},    // and a girl, who keeps her clothes
		{"human.walk", "old.walk"},          // the same for the old
		{"child.idle", "child.idle"},        // what it does have, it uses
		{"human.f.idle", "human.f.hurt"},    // no picture of a blow landing
		{"enemy.big.idle", "enemy.big.eat"}, // a beast gives up the pose, never the build
	} {
		if got := standIn(c.from, has); got != c.want {
			t.Errorf("%s came out as %q and should be %q", c.from, got, c.want)
		}
	}
	// And the set this is built for gives nothing up: everything resolves to
	// itself, which is the other half of the guarantee.
	have := map[string]bool{}
	for _, c := range theManifest(t).Clips {
		have[c.Name] = true
	}
	for name := range have {
		if got := standIn(name, func(n string) bool { return have[n] }); got != name {
			t.Errorf("the default art turned %q into %q", name, got)
		}
	}
}

// A wish that has come true is taken out of the list by hand, and this is
// what says so.
//
// The list of art nobody has drawn is written by hand because there is
// nothing to read it off - the viewer does not ask for what does not exist -
// and a hand-written list of things to do is a list that outlives the doing.
// So the one thing that can be checked is checked: if a set actually has what
// a wish asks for, the wish is stale.
func TestNothingIsStillWishedForOnceItHasBeenDrawn(t *testing.T) {
	sets := theArt(t).Sets
	for _, w := range wishes {
		if w.Ask == "" || w.What == "" {
			t.Errorf("a wish with nothing to take to whoever draws: %+v", w)
		}
		if where := drawnIn(w, sets); where != "" {
			t.Errorf("%q is in %s now, so take it out of wishes", w.What, where)
		}
	}
}

// And the report itself runs, on every set there is.
//
// It reads manifests and nothing else - no world, no window, no graphics
// device - which is what lets it be the thing somebody runs over a terminal
// before writing to whoever draws. A report that needed a screen would be
// useless for that, so this is where that stays true.
func TestTheGapReportSaysWhatEachSetIsShortOf(t *testing.T) {
	report, err := artReport()
	if err != nil {
		t.Fatalf("artgaps: %v", err)
	}
	for _, set := range theArt(t).Sets {
		if !strings.Contains(report, set.Name) {
			t.Errorf("the report says nothing about %s", set.Name)
		}
	}
	// set1 is the set from before the young and the old were drawn moving,
	// and it is in here as the case that has holes: a report that cannot see
	// those cannot see any.
	holes, err := holesIn("set1")
	if err != nil {
		t.Fatal(err)
	}
	if len(holes) == 0 {
		t.Fatal("set1 has no holes, which cannot be: it has no picture of a child walking")
	}
	for _, h := range holes {
		if h.StandIn == h.Clip {
			t.Errorf("%s stands in for itself", h.Clip)
		}
	}
}

// The report finds art that is there and wrong, not only art that is absent.
//
// Both halves are pinned here, because both have a way of being quietly
// useless: a measure that finds nothing looks the same as a set with nothing
// wrong, and a measure that finds everything is noise nobody reads.
func TestTheGapReportMeasuresArtThatIsWrong(t *testing.T) {
	for _, set := range theArt(t).Sets {
		flaws, err := flawsIn(set.Name)
		if err != nil {
			t.Fatalf("%s: %v", set.Name, err)
		}
		found := map[string]bool{}
		for _, f := range flaws {
			found[f.Clip] = true
			if f.Says == "" {
				t.Errorf("%s: %s is called wrong and nothing says why", set.Name, f.Clip)
			}
			// A beast fills its cell by width, so its poses are different
			// heights on purpose (docs/sprites.md section 4). Reporting that
			// would be reporting the rule.
			if strings.HasPrefix(f.Clip, "enemy.") && strings.Contains(f.Says, "standing") &&
				strings.Contains(f.Says, "pixels tall") {
				t.Errorf("%s: %s is measured against a height it was never meant to keep", set.Name, f.Clip)
			}
		}
		// The adult woman's walk is in every set drawn so far: her two frames
		// differ less than her breathing does, because the skirt hides the
		// legs. If this stops being found, the measure has gone blind.
		if !found["human.f.walk"] {
			t.Errorf("%s: the measure no longer finds the walk it was built on", set.Name)
		}
		// And it is not flagging everything: the man's walk is the one that
		// was drawn right.
		if found["human.walk"] {
			t.Errorf("%s: human.walk is the walk that works and it was reported", set.Name)
		}
	}
}

// Whose line a body belongs to is read off how bright it is drawn.
//
// The rule is the whole of what the spirits were drawn white for, and it is
// worth pinning because it is two rules that must not drift apart: the line
// is drawn exactly as the art was drawn, and every stranger is drawn darker
// than that. "A little darker for the children" would be the version that
// looks reasonable in the code and fails on the screen, where the question
// is which of sixty bodies is mine.
func TestTheLineIsBrightAndEveryoneElseIsNot(t *testing.T) {
	g := &game{played: 7, lineKids: []int{9, 11}}
	light := func(c color.RGBA) float64 {
		return 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
	}
	played := &engine.Agent{ID: 7}
	child := &engine.Agent{ID: 9}
	stranger := &engine.Agent{ID: 42}

	if got := g.bodyPaint(played, false); got != colorOwnLine {
		t.Errorf("the played body is painted %v, want the art as drawn %v", got, colorOwnLine)
	}
	if got := g.bodyPaint(child, false); got != colorOwnLine {
		t.Errorf("a child of the line is painted %v, want the same as the line %v", got, colorOwnLine)
	}
	dim := g.bodyPaint(stranger, false)
	if light(dim) >= light(colorOwnLine) {
		t.Errorf("a stranger is painted %v (%.0f), which is not darker than the line (%.0f)",
			dim, light(dim), light(colorOwnLine))
	}
	// Every stranger there can be, however the wobble falls.
	for id := 1; id < 400; id++ {
		if id == 7 || id == 9 || id == 11 {
			continue
		}
		if c := g.bodyPaint(&engine.Agent{ID: id}, false); light(c) >= light(colorOwnLine) {
			t.Fatalf("stranger #%d is painted %v (%.0f), as bright as the line", id, c, light(c))
		}
	}
	// And with real bodies, whatever they spent their budget on: the colour
	// varies and the brightness does not. Brightness answers "whose line is
	// this" and a body that happened to be born pale would answer it wrong.
	seen := map[color.RGBA]bool{}
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 2000; i++ {
		body := &engine.Agent{ID: 1000 + i, Genome: make([]float64, engine.NumGenes)}
		for j := range body.Genome {
			body.Genome[j] = rng.Float64() * 120
		}
		c := g.bodyPaint(body, false)
		seen[c] = true
		if light(c) >= light(colorOwnLine)*0.8 {
			t.Fatalf("a body with genome %v is painted %v (%.0f), too close to the line (%.0f)",
				body.Genome, c, light(c), light(colorOwnLine))
		}
	}
	if len(seen) < 200 {
		t.Errorf("2000 genomes are drawn in %d colours; the point of colouring by "+
			"the budget is that a crowd is not one body repeated", len(seen))
	}
	// A body that spent on fighting is redder than one that spent on knowing.
	fighter := &engine.Agent{ID: 1, Genome: make([]float64, engine.NumGenes)}
	thinker := &engine.Agent{ID: 2, Genome: make([]float64, engine.NumGenes)}
	for j := range fighter.Genome {
		fighter.Genome[j], thinker.Genome[j] = 40, 40
	}
	fighter.Genome[engine.GeneAttack] = 200
	thinker.Genome[engine.GeneIntelligence] = 200
	if f, w := strangerColour(fighter), strangerColour(thinker); f.R <= w.R || w.B <= f.B {
		t.Errorf("a fighter is painted %v and a thinker %v; red is meant to be the "+
			"budget spent on fighting and blue on knowing", f, w)
	}

	// Nobody playing: nothing to pick out, so nothing is dimmed.
	none := &game{}
	if got := none.bodyPaint(stranger, false); got != colorOwnLine {
		t.Errorf("with nobody played, a body is painted %v, want %v", got, colorOwnLine)
	}
	// And the old grey art still says the sex, which is all it ever said.
	if got := g.bodyPaint(&engine.Agent{ID: 42, Sex: engine.Female}, true); got.R <= got.B {
		t.Errorf("grey art for a female body is painted %v, which is not the pink it used to be", got)
	}
}

// The set it draws with colours the people and leaves the beasts alone.
//
// Two decisions in one line of the manifest. The spirits are drawn white so
// that a line can be given a colour, so they are tinted; the beasts are
// turned inside out at packing time - dark, with a bright rim - so that they
// read as something not made of the same light as the people, and giving
// them a colour on top would undo it.
func TestThePeopleAreColouredAndTheBeastsAreNot(t *testing.T) {
	m := theManifest(t)
	tint := map[string]bool{}
	for _, c := range m.Clips {
		tint[c.Name] = c.Tint
	}
	for _, name := range []string{"human.idle", "human.f.idle", "child.idle", "old.idle"} {
		if !tint[name] {
			t.Errorf("%s is not given a colour, so no line can be told from another", name)
		}
	}
	for _, name := range []string{"enemy.big.idle", "enemy.small.idle", "enemy.water.idle", "enemy.fly.idle"} {
		if tint[name] {
			t.Errorf("%s is given a colour, which undoes the dark it was packed with", name)
		}
	}
}

// A body that has fallen keeps the colour it had.
//
// It is drawn from what the viewer remembered, because the engine compacts
// the dead out the same tick - so if the colour is not remembered with the
// place, there is nothing left to work it out from. The first version did
// not remember it and every corpse in the world came up white, which is the
// one colour that means "this one is yours".
func TestACorpseKeepsTheColourItHad(t *testing.T) {
	cfg := engine.DefaultConfig()
	cfg.InitialPopulation = 0
	g := &game{world: engine.NewWorld(cfg), played: 1, lineKids: []int{2}}
	genome := func(spend engine.Gene) []float64 {
		out := make([]float64, engine.NumGenes)
		for i := range out {
			out[i] = 40
		}
		out[spend] = 200
		return out
	}
	agents := []engine.Agent{
		{ID: 1, Genome: genome(engine.GeneAttack)},
		{ID: 2, Genome: genome(engine.GeneSpeed)},
		{ID: 3, Genome: genome(engine.GeneAttack)},
		{ID: 4, Genome: genome(engine.GeneIntelligence)},
	}
	g.markFallen(agents, &cfg)
	want := map[int]color.RGBA{
		1: colorOwnLine, // the played body
		2: colorOwnLine, // a child of the line
		3: strangerColour(&agents[2]),
		4: strangerColour(&agents[3]),
	}
	for id, c := range want {
		if got := g.standing[id].paint; got != c {
			t.Errorf("#%d would fall in %v, want %v", id, got, c)
		}
	}
	if g.standing[3].paint == g.standing[4].paint {
		t.Errorf("two strangers who spent their budgets differently fall in the same colour %v",
			g.standing[3].paint)
	}
}
