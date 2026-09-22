package main

import (
	"bytes"
	"fmt"
	"image"
	_ "image/png"
	"sort"
	"strings"

	"github.com/toku463ne/cld_human_behaviour/engine"
)

// What each set of art is short of, so that somebody can go and ask for it.
//
// The art is generated, a sheet comes back with whatever it came back with,
// and twice now a row that was asked for simply did not arrive. Nothing on
// the screen says so: a picture that is not there is drawn with the nearest
// one that is (standIn), which is the right thing to do while playing and
// exactly the wrong thing while deciding what to ask for next. So this counts
// it instead.
//
// Two different things are missing from a set, and they are found two
// different ways.
//
//   - A hole is a picture the viewer asks for and the set has not got. The
//     viewer itself is the authority on what is asked for - clipFor is the
//     only thing that knows - so these are found by asking it, for every
//     action, sex, age, circumstance and row of every table this viewer can
//     be pointed at. Nobody writes this list down and so nobody can forget
//     to update it.
//   - A wish is art nobody has drawn in any set, which the viewer therefore
//     does not ask for and which no amount of asking clipFor will turn up.
//     A beast being struck is the standing example: the row was asked for,
//     never arrived, and clipFor deliberately does not reach for it. These
//     have to be written down by hand, and a test keeps them honest by
//     failing once a set actually has one.

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

// askedFor is every picture this viewer can ask a set for, and how many frames
// deep it reads into each.
//
// By running the choosing, not by listing the answers: every key here comes
// out of clipFor, deadClipFor or itemClip, so a rule that starts asking for
// something new starts being counted here the same day, with nobody to
// remember to add it.
func askedFor() map[string]int {
	want := map[string]int{}
	ask := func(name string, frames int) {
		if frames > want[name] {
			want[name] = frames
		}
	}
	looks := []look{{}, {struck: true}, {wading: true}, {aloft: true}, {struck: true, wading: true}}
	for _, cfg := range worlds() {
		cfg := cfg
		ages := []func(*engine.Agent){
			func(a *engine.Agent) { a.Maturity = 0 },
			func(a *engine.Agent) { a.Maturity = 1 },
			func(a *engine.Agent) {
				a.Maturity = 1
				a.Age = int((cfg.SenescenceYears + 20) * float64(cfg.TicksPerYear))
			},
		}
		for kind := engine.ActionKind(0); kind < 32; kind++ {
			for _, species := range []engine.Species{engine.SpeciesHuman, engine.SpeciesEnemy} {
				for _, sex := range []engine.Sex{engine.Male, engine.Female} {
					for row := 0; row <= len(cfg.EnemyKinds); row++ {
						for _, age := range ages {
							for _, l := range looks {
								if l.aloft && species != engine.SpeciesEnemy {
									continue // only a beast is ever off the ground
								}
								a := &engine.Agent{Species: species, Sex: sex, Kind: uint8(row),
									Action: engine.Action{Kind: kind}}
								age(a)
								ask(clipFor(a, &cfg, l), 1)
							}
						}
						ask(deadClipFor(&engine.Agent{Species: species, Sex: sex, Kind: uint8(row)}, &cfg), 1)
					}
				}
			}
		}
	}
	// And the things lying about. This one is read by frame rather than by
	// name - one clip holds the whole strip - so what it needs is a count.
	for kind := engine.FoodKind(0); kind < engine.NumFoodKinds; kind++ {
		if kind == engine.NumEdibleKinds {
			continue // the line between edible and not, and not a kind
		}
		for _, from := range []engine.Species{engine.SpeciesHuman, engine.SpeciesEnemy} {
			name, at := itemClip(engine.Food{Kind: kind, From: from})
			ask(name, at+1)
		}
	}
	return want
}

// hole is one picture a set has not got, and what it draws instead.
type hole struct {
	Clip    string
	StandIn string
	Frames  int // how many frames are wanted, where it is there but too short
	Has     int
}

// holesIn is what one set is short of, in the order a person would read it.
func holesIn(set string) ([]hole, error) {
	m, err := readManifest(set)
	if err != nil {
		return nil, err
	}
	have := map[string]int{}
	for _, c := range m.Clips {
		have[c.Name] = c.Frames
	}
	has := func(n string) bool { return have[n] > 0 }
	var out []hole
	for clip, frames := range askedFor() {
		switch {
		case have[clip] == 0:
			out = append(out, hole{Clip: clip, StandIn: standIn(clip, has), Frames: frames})
		case have[clip] < frames:
			// One clip short of a frame, which only the strip of things can
			// be: a body cycles its frames and does not care how many there
			// are, but a coin drawn with frame six of five is a stone.
			out = append(out, hole{Clip: clip, Frames: frames, Has: have[clip]})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Clip < out[j].Clip })
	return out, nil
}

// flaw is a picture a set HAS and that is wrong, which is the other half of
// what somebody needs before writing to whoever draws.
//
// Only what can be measured off the sheet is in here. Whether a body looks
// like an old man is not something to compute; whether its walking frames
// differ from each other is, and that is the one the generator has actually
// got wrong twice - a sheet came back where "walking" was two copies of
// standing, and another where the women walk with their legs under a skirt
// and barely move at all.
type flaw struct {
	Clip string
	Says string
}

// flawsIn measures one set against itself.
//
// Against itself, and never against a number written down here: what counts
// as "moving" depends on how the sheet was drawn, so the yardstick is the
// same body standing still in the same set. A walk has to differ from frame
// to frame by half again what a breath does, and a pose that is meant to be
// standing has to stand at the same height as that body standing.
func flawsIn(set string) ([]flaw, error) {
	m, err := readManifest(set)
	if err != nil {
		return nil, err
	}
	raw, err := loadAsset(set + "/" + m.Sheet)
	if err != nil {
		return nil, err
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.Sheet, err)
	}
	type shape struct {
		moved  float64 // how much of the body changes between the first two frames
		height int
	}
	read := func(c clipInfo) shape {
		at := func(f, x, y int) bool {
			_, _, _, a := src.At(c.X+f*c.W+x, c.Y+y).RGBA()
			return a > 0
		}
		var moved, body, top, bottom int
		top = -1
		for y := 0; y < c.H; y++ {
			for x := 0; x < c.W; x++ {
				first, second := at(0, x, y), at(min(1, c.Frames-1), x, y)
				if first || second {
					body++
				}
				if first != second {
					moved++
				}
				if first {
					if top < 0 {
						top = y
					}
					bottom = y
				}
			}
		}
		if body == 0 || top < 0 {
			return shape{}
		}
		return shape{moved: float64(moved) / float64(body), height: bottom - top + 1}
	}

	have := map[string]shape{}
	for _, c := range m.Clips {
		have[c.Name] = read(c)
	}
	var out []flaw
	for _, c := range m.Clips {
		at := strings.LastIndex(c.Name, ".")
		if at < 0 {
			continue
		}
		who, pose := c.Name[:at], c.Name[at+1:]
		standing, ok := have[who+".idle"]
		if !ok || standing.height == 0 {
			continue // nothing to measure it against
		}
		this := have[c.Name]
		if pose == "walk" && c.Frames > 1 && this.moved < standing.moved*1.5 {
			out = append(out, flaw{c.Name, fmt.Sprintf(
				"its two frames differ by %.0f%% of the body and standing still differs by %.0f%%, so the legs are not showing that it walks",
				100*this.moved, 100*standing.moved)})
		}
		// A pose that is meant to be a body on its feet, at the height that
		// body stands at. Sitting and wading are meant to be lower and are
		// left out of this on purpose.
		//
		// People only. A beast's clips are all scaled together to fill the
		// cell by width - its size on screen comes from its budget, not from
		// its picture - so a pouncing beast being lower and wider than a
		// standing one is the art doing what it was asked, and reporting it
		// would be reporting the rule.
		person := !strings.HasPrefix(c.Name, "enemy.")
		if person && (pose == "walk" || pose == "fight" || pose == "hurt") {
			if off := 100 * float64(this.height-standing.height) / float64(standing.height); off < -5 || off > 5 {
				out = append(out, flaw{c.Name, fmt.Sprintf(
					"drawn %d pixels tall against %d standing (%+.0f%%), so the body changes size when it does this",
					this.height, standing.height, off)})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Clip < out[j].Clip })
	return out, nil
}

// wish is art nobody has drawn in any set yet.
//
// Written by hand because there is nothing to read it off: the viewer does
// not ask for what does not exist, and a sheet that never arrived leaves no
// trace anywhere. Clips names what it would be called once it is drawn, so
// that a set which has it stops being told it is missing - and so that a test
// can say when one of these is done and the entry should go.
type wish struct {
	What  string
	Clips []string
	Ask   string
}

var wishes = []wish{
	{
		What:  "a beast being struck",
		Clips: []string{"enemy.big.hurt", "enemy.small.hurt", "enemy.water.hurt", "enemy.fly.hurt"},
		Ask:   "docs/sprites.md section 5, the brief as it stands: row 5 came back missing. Ask again with the standing row beside it.",
	},
	{
		What: "hair in two layers",
		// No names: what it would be called depends on how it is drawn, and
		// guessing one here would make this entry look done the day somebody
		// drew something else with that name.
		Ask: "docs/sprites.md section 7-2. Seven whole standing bodies is what there is, so hair colour shows only while a body stands still. A bald body plus hair alone, same size and place, is what would fix it.",
	},
}

// artReport is what to ask for next, per set, as plain text.
//
// Printed rather than drawn: the point of it is to be read beside the brief
// that goes to whoever draws, and a screen full of clip names would be the
// wrong place for that.
func artReport() (string, error) {
	ix, err := artSets()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, set := range ix.Sets {
		mark := ""
		if set.Name == ix.Default {
			mark = "  (the one it draws with)"
		}
		fmt.Fprintf(&b, "%s%s\n    %s\n", set.Name, mark, set.Note)
		holes, err := holesIn(set.Name)
		if err != nil {
			return "", err
		}
		if len(holes) == 0 {
			fmt.Fprintf(&b, "    missing  nothing it is asked for\n")
		}
		for _, line := range byBody(holes) {
			fmt.Fprintf(&b, "    missing  %s\n", line)
		}
		flaws, err := flawsIn(set.Name)
		if err != nil {
			return "", err
		}
		for _, f := range flaws {
			fmt.Fprintf(&b, "    wrong    %-17s %s\n", f.Clip, f.Says)
		}
		b.WriteString("\n")
	}
	b.WriteString("nobody has drawn these, in any set, so the viewer never asks:\n")
	for _, w := range wishes {
		if where := drawnIn(w, ix.Sets); where != "" {
			continue // it arrived; the entry is stale and a test says so
		}
		fmt.Fprintf(&b, "  - %s\n      %s\n", w.What, w.Ask)
	}
	return b.String(), nil
}

// drawnIn names the set that has what a wish asks for, or nothing.
//
// A wish with no clip names can never be answered this way - a redraw of a
// picture that is already there has no new name to look for - so it stands
// until somebody takes it out by hand.
func drawnIn(w wish, sets []artSet) string {
	if len(w.Clips) == 0 {
		return ""
	}
	for _, set := range sets {
		m, err := readManifest(set.Name)
		if err != nil {
			continue
		}
		have := map[string]bool{}
		for _, c := range m.Clips {
			have[c.Name] = true
		}
		all := true
		for _, clip := range w.Clips {
			if !have[clip] {
				all = false
			}
		}
		if all {
			return set.Name
		}
	}
	return ""
}

// byBody gathers holes into one line per body, because that is the shape of
// what gets asked for: a sheet is rows of one subject doing several things,
// so "the boy is missing five poses" is one request and five clip names are
// not.
func byBody(holes []hole) []string {
	var order []string
	poses := map[string][]string{}
	for _, h := range holes {
		who, pose := h.Clip, ""
		if at := strings.LastIndex(h.Clip, "."); at > 0 {
			who, pose = h.Clip[:at], h.Clip[at+1:]
		}
		switch {
		case h.Has > 0:
			pose = fmt.Sprintf("%s (%d frames of %d)", pose, h.Has, h.Frames)
		case h.StandIn != "":
			pose += " -> " + h.StandIn
		}
		if _, seen := poses[who]; !seen {
			order = append(order, who)
		}
		poses[who] = append(poses[who], pose)
	}
	out := make([]string, 0, len(order))
	for _, who := range order {
		out = append(out, fmt.Sprintf("%-10s %s", who, strings.Join(poses[who], ", ")))
	}
	return out
}
