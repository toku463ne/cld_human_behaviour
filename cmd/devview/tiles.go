package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"math"
	"strings"

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

// artSet is one set of pictures: a directory under assets/ holding a sheet
// and the manifest that cuts it up.
//
// There are several because the art is generated, a fresh render is a fresh
// throw, and the only way to tell whether a new one is better is to put the
// two on the same world and look. A set is a few hundred kilobytes, so
// keeping the old one costs less than the argument about whether to.
//
// The note is what the set is, in a few words, said when it is switched to.
// It is written by hand: nothing about a picture tells you what was different
// about the day it was asked for.
type artSet struct {
	Name string `json:"name"`
	Note string `json:"note"`
}

type artIndex struct {
	Default string   `json:"default"`
	Sets    []artSet `json:"sets"`
}

// artSets is what is on offer, and which of them is drawn with unless
// somebody says otherwise.
//
// A file rather than a listing of the directory, because the browser has no
// directory to list: it fetches by name over the network and has no way to
// ask what is there. A test keeps the file honest against what is on disk.
func artSets() (artIndex, error) {
	raw, err := loadAsset("sets.json")
	if err != nil {
		return artIndex{}, err
	}
	var ix artIndex
	if err := json.Unmarshal(raw, &ix); err != nil {
		return artIndex{}, fmt.Errorf("sets.json: %w", err)
	}
	if len(ix.Sets) == 0 {
		return artIndex{}, fmt.Errorf("sets.json names no art")
	}
	if ix.Default == "" {
		ix.Default = ix.Sets[0].Name
	}
	return ix, nil
}

// tileset is the sheet, cut up. The sub-images share the one texture, so
// drawing from any of them batches with drawing from any other.
type tileset struct {
	name  string
	sheet *ebiten.Image
	clips map[string][]*ebiten.Image
	tint  map[string]bool
	tile  int
}

// loadTiles reads the manifest, then the sheet, and cuts it up. Everything is
// in hand when it returns or it returns an error: that is what "preloaded"
// means, and it is why this is called before the game is handed to ebiten
// rather than lazily on the first draw.
func loadTiles(set string) (*tileset, error) {
	m, err := readManifest(set)
	if err != nil {
		return nil, err
	}
	png, err := loadAsset(set + "/" + m.Sheet)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.Sheet, err)
	}
	src, _, err := image.Decode(bytes.NewReader(png))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.Sheet, err)
	}
	t := &tileset{name: set, sheet: ebiten.NewImageFromImage(src), tile: m.Tile,
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

// readManifest is the half of loading a set that needs no graphics device,
// which is the half a test can run.
func readManifest(set string) (manifestFile, error) {
	raw, err := loadAsset(set + "/manifest.json")
	if err != nil {
		return manifestFile{}, fmt.Errorf("manifest: %w", err)
	}
	var m manifestFile
	if err := json.Unmarshal(raw, &m); err != nil {
		return manifestFile{}, fmt.Errorf("manifest: %w", err)
	}
	if m.Sheet == "" || m.Tile <= 0 {
		return manifestFile{}, fmt.Errorf("%s: manifest names no sheet", set)
	}
	return m, nil
}

// frame is one picture out of a clip, or nil where the set has nothing that
// will stand in for it. A nil picture is the caller's cue to fall back to the
// circle, so a half-finished sheet is a partly drawn world rather than a
// crash.
func (t *tileset) frame(name string, i int) *ebiten.Image {
	frames := t.clips[name]
	if len(frames) == 0 {
		has := func(n string) bool { return len(t.clips[n]) > 0 }
		if stood := t.clips[standIn(name, has)]; len(stood) > 0 {
			frames = stood
		} else {
			return nil
		}
	}
	return frames[((i%len(frames))+len(frames))%len(frames)]
}

// standIn is what an older set draws with where it has nothing for what was
// asked.
//
// Sets exist to be swapped, and a set drawn before some distinction was made
// does not have it: every set before 2026-09-22 has one picture of a child
// standing and none of one walking. Without this, switching to one of those
// puts circles back on this screen wherever a child moves, which reads as a
// broken viewer rather than as older art.
//
// What is given up, and in what order, is the design. The pose is given up
// last, because it is the only part of a picture the eye has a neighbour to
// compare against: a child sliding along the ground in a standing pose while
// every adult beside it walks is what a wrong pose looks like, and it reads
// as a broken picture rather than as a child. So age goes first - a child
// drawn as a grown body walking is the world exactly as it stood until the
// young were drawn moving, and it is only wrong about size, which the viewer
// says for itself by drawing the body smaller. Sex goes second, and costs the
// clothes. Only when nothing at all is drawn for the pose does the body stand
// still, and then the most particular body that set has is used.
func standIn(name string, has func(string) bool) string {
	if has(name) {
		return name // a set that has it gives nothing up
	}
	if strings.HasPrefix(name, groundPrefix) {
		// The ground stands in for nothing. A body drawn in the wrong pose
		// is still that body; a cell drawn with the wrong piece of ground is
		// a river running up a cliff. A set that is short of any of it falls
		// back to the washes as a whole (tileset.hasGround), which is one
		// world consistently rather than two mixed.
		return name
	}
	part := strings.Split(name, ".")
	if part[0] == "enemy" {
		// A beast has no age and no sex, and its build is the one thing not
		// to lie about - a heavy one drawn as a lurker is wrong about which
		// sort it is, which is the single thing this screen has to get right.
		// So the pose is all there is to give up.
		if len(part) == 3 {
			return part[0] + "." + part[1] + ".idle"
		}
		return name
	}
	if len(part) < 2 {
		return name
	}
	kind, sex, pose := part[0], "", part[len(part)-1]
	if len(part) == 3 {
		sex = part[1]
	}
	with := func(kind, sex, pose string) string {
		if sex == "" {
			return kind + "." + pose
		}
		return kind + "." + sex + "." + pose
	}
	for _, try := range []string{
		with("human", sex, pose), // the same thing, grown
		with("human", "", pose),  // and a man
		with(kind, sex, "idle"),  // nobody drew the pose: this body, standing
		with("human", sex, "idle"),
		"human.idle",
	} {
		if has(try) {
			return try
		}
	}
	return "human.idle"
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
// where it is standing, and where it is standing wins over what it is doing.
// How old it is is not in that order at all: it is picked before any of them,
// because it is not a circumstance but part of who the body is - the same
// place the sex is picked, and for the same reason.
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
	// How old it is picks the kind, the same way its sex does, and for the
	// same reason: both of them are what the body IS, and neither of them
	// stops being true because it started walking. This used to be asked
	// after the action, and only of a body standing still, because the sheet
	// had one picture each for a child and for someone old - so a child that
	// took a step turned into an adult drawn small. The sheet has all six
	// poses for both now (2026-09-22) and the question moved to where it
	// belonged in the first place.
	if a.Species == engine.SpeciesHuman {
		switch {
		case !a.IsAdult(cfg):
			kind = childOf(kind)
		// Past its prime is the engine's own line, read off the same figure
		// that already makes an old body draw smaller, so a world with the
		// rule turned off has nobody old in it and asks for nothing.
		case a.Maturity >= 1 && a.AgeFactor(cfg) < 1:
			kind = oldOf(kind)
		}
	}
	if hitting(a) {
		// Throwing the blow beats taking one: a body doing both at once is
		// more legible as the one going forward.
		return kind + ".fight"
	}
	// Taking a blow is drawn for everything now (2026-09-22). The row of a
	// beast being struck was asked for twice, arrived with the spirits, and
	// this is the line that wakes it. An older set that has not got it
	// draws that build standing instead (standIn), which is what every set
	// before this did anyway.
	if l.struck {
		return kind + ".hurt"
	}
	// Standing in a river is drawn for people only, and that one is not a
	// hole in the art at all: the beast that lives in water is a BUILD rather than a
	// circumstance, so a heavy one crossing a river is still a heavy one.
	// Reaching for enemy.water.* here would lie about which sort it is,
	// which is the one thing about a beast this screen has to get right.
	if a.Species == engine.SpeciesHuman && l.wading {
		return kind + ".swim"
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

// hitting is whether this body is throwing a blow: the same reading the
// action switch below makes, pulled out so that the circumstances above can
// ask the question before the action answers it.
func hitting(a *engine.Agent) bool {
	return a.Action.Kind == engine.ActAttack || a.Action.Kind == engine.ActThrow
}

// childOf and oldOf are the runs drawn for the young and the old of a kind.
// Only the people have them; a beast is a beast at every age, which is what
// the sheet was asked for and what the rules say about one. Every pose the
// grown picture has, these have, so the caller can pick the kind and then
// forget that it did.
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

// bodyPaint is the colour a body's picture is multiplied by.
//
// Two different questions, decided by which art is in hand.
//
// Grey placeholder art has to be given a colour to be visible at all, and
// the colour it was given was the sex. That is what every set before the
// spirits is made of, and it is left alone.
//
// Art drawn white to be coloured (the spirits) answers a better question
// with it: whose line this body belongs to. The played body and every child
// the line has had are drawn exactly as the art was drawn - the brightest
// thing in the world - and everybody else is stepped down. The children are
// not stepped down a little, they are not stepped down at all: a line is one
// thing, and the player has to be able to find their own at a glance in a
// crowd of sixty. The sex is not lost by this, because the spirits say it
// with the shape of the head instead (docs/sprites.md section 10).
//
// A world with nobody played has no line to pick out, so everybody is drawn
// at full: dimming a whole world against nothing is just a darker world.
func (g *game) bodyPaint(a *engine.Agent, grey bool) color.RGBA {
	if grey {
		fill := colorMale
		if a.Sex == engine.Female {
			fill = colorFemale
		}
		return bodyTint(a, fill)
	}
	// The played body and whoever would take over from it. Not every child:
	// one per generation, the eldest living one, which is what a person means
	// by "mine" when they are playing a line rather than a body.
	if g.played != 0 && g.carriesMyColour(a.ID) {
		return colorOwnLine
	}
	// Everybody else is drawn in the colour of the family they were born
	// into, which a newborn takes from its mother (2026-09-22). It says
	// nothing about what a body can do and nothing reads it - it is here so
	// that a crowd of sixty can be seen to be four families rather than sixty
	// strangers.
	if g.paintBy != paintByBudget {
		if c, ok := lineColour(a.Lineage); ok {
			return c
		}
	}
	return strangerColour(a)
}

// paintBy says what a body's colour is for.
type paintRule int

const (
	// paintByLine is the default: the family a body was born into.
	paintByLine paintRule = iota
	// paintByBudget is what it was until 2026-09-22: where the body's budget
	// went, red for fighting, green for getting about, blue for knowing. It
	// is kept because it says something the family colour cannot, and a crowd
	// is where a per-gene reading is too small to use.
	paintByBudget
)

// How the eye weighs the channels (Rec. 601), which is what "the same
// lightness" has to mean for a colour to say nothing by being bright.
const (
	lumaR = 0.299
	lumaG = 0.587
	lumaB = 0.114
)

// lineColour is the colour of a family, from the tag the engine already hands
// down (Agent.Lineage): each founder starts one, and a newborn takes its
// mother's. The second return is false for a body with no family tag at all -
// a beast, or a world built before the tag - and then the caller falls back to
// what it drew before.
//
// Nothing in the world reads the tag and nothing here changes what a body
// does: this is a colour, and the whole of its job is that a person watching
// can see who is related to whom.
//
// Why the mother's rather than either parent's: the engine hands the tag down
// that way already, so a colour that follows it costs nothing, survives a save
// and cannot disagree with itself. A colour picked from either parent would
// need a memory of its own out here and would come out differently in two
// viewers watching the same world.
func lineColour(tag uint16) (color.RGBA, bool) {
	if tag == 0 {
		return color.RGBA{}, false
	}
	// Hues a third of a turn apart, one channel each, added to the same grey
	// the strangers are built on. The three offsets sum to nought, so every
	// family is drawn at exactly the same lightness - brightness already
	// answers "is this my line" and a second meaning on that channel would
	// spoil the first.
	//
	// The hue itself steps by the golden ratio, so families that were founded
	// one after another are as far apart on the wheel as they can be rather
	// than a shade apart.
	const (
		grey = 0.55
		amp  = 0.18
	)
	h := math.Mod(float64(tag)*0.6180339887498949, 1)
	off := func(turn float64) float64 { return amp * math.Cos(2*math.Pi*(h-turn)) }
	r, gr, b := off(0), off(1.0/3), off(2.0/3)
	// The three offsets already sum to nought, but the eye does not weigh the
	// channels alike: a green-leaning family built that way comes out clearly
	// brighter than a blue-leaning one. Taking off how much brighter the mix
	// reads makes the perceived lightness the same for every family, which is
	// what the rule above is actually about.
	lift := lumaR*r + lumaG*gr + lumaB*b
	r, gr, b = r-lift, gr-lift, b-lift
	at := func(v float64) uint8 { return uint8(clamp01(grey+v) * 255) }
	return color.RGBA{at(r), at(gr), at(b), 0xff}, true
}

// strangerColour is what a body not of the played line is drawn in: dimmer
// than the line, always, and coloured by where its budget went.
//
// This is a reversal. The wobble that used to be here came from the ID and
// said nothing on purpose - "there is no gene for complexion" - and it was
// there only so that a crowd did not look like one body repeated sixty
// times. The spirits can carry a colour properly, so it may as well be a
// reading: red is a body that spent on fighting and surviving, green on
// getting about, blue on knowing things. It is the same fact the ring width
// and the tail already show one gene at a time, said all at once and at a
// glance, and it is worth having because a crowd of sixty is exactly where
// those readings are too small to use.
//
// Brightness is not part of it. Every colour this returns is held at the
// same lightness, well under the line's own, because brightness already
// answers a question - whose line is this - and a second meaning on the
// same channel would spoil the first. A test pins that for any genome.
func strangerColour(a *engine.Agent) color.RGBA {
	if len(a.Genome) < engine.NumGenes {
		return colorStranger
	}
	mean := func(genes ...engine.Gene) float64 {
		sum := 0.0
		for _, g := range genes {
			sum += a.Genome[g]
		}
		return sum / float64(len(genes))
	}
	// Grouped by what the genes are for, and averaged inside each group so
	// that a group of three does not outweigh a group of two by being three.
	power := mean(engine.GeneAttack, engine.GeneDefence, engine.GeneVitality)
	quick := mean(engine.GeneSpeed, engine.GeneEvasion)
	wits := mean(engine.GeneMemory, engine.GeneRationality, engine.GeneIntelligence)
	total := power + quick + wits
	if total <= 0 {
		return colorStranger
	}
	// A third each is the even body, and what is drawn is the departure
	// from it: one step lighter in the channel a body spent on, one step
	// darker in the ones it did not.
	//
	// Added to a fixed grey rather than scaled to a fixed brightness, which
	// is how the first two attempts turned the world into a bag of sweets.
	// Scaling puts every mix at one lightness, and a mix whose strong
	// channel is a dark one - red, or blue - has to be multiplied a long
	// way up to get there, which runs it off the top and clips it into a
	// primary. Adding cannot: the furthest any channel goes is one step,
	// so the colours stay the greys they are made of and nothing is ever
	// saturated. Measured against real bodies (3000 ticks of the default
	// world), the budget shares run 0.10 to 0.60 and these give the likes
	// of rgb(132,132,155), rgb(128,165,126), rgb(165,134,120): lavender,
	// sage, dust.
	const (
		grey   = 0.55 // what a body with an even budget is drawn at
		step   = 0.45 // how far a lopsided one pulls a channel
		lowest = 0.35
		most   = 0.75
	)
	tint := func(share float64) float64 {
		v := grey + (share/total-1.0/3)*step
		if v < lowest {
			v = lowest
		}
		if v > most {
			v = most
		}
		return v
	}
	r, gr, b := tint(power), tint(quick), tint(wits)
	return color.RGBA{uint8(r * 255), uint8(gr * 255), uint8(b * 255), 0xff}
}

// greyArt says whether a clip is one of the grey placeholders, which are
// coloured by a different rule (bodyPaint). A clip carries its own size and
// the placeholders are the small ones.
func (g *game) greyArt(clip string) bool {
	if g.tiles == nil {
		return false
	}
	frames := g.tiles.clips[clip]
	return len(frames) > 0 && frames[0].Bounds().Dx() != g.tiles.tile
}

// drawBody stamps one body. It reports whether it drew anything, so that the
// caller can fall back to the circle where there is no picture for it.
func (g *game) drawBody(screen *ebiten.Image, a *engine.Agent, cfg *engine.Config, x, y, radius float32) bool {
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
		op.ColorScale.ScaleWithColor(g.bodyPaint(a, w != g.tiles.tile))
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

// drawAura lays the light that says whose line a body belongs to, under it.
//
// Brightness already says it (bodyPaint) and this says it again, louder, for
// the one body the player is actually holding. Two signals for one fact is
// worth it here and nowhere else: the played body is what the eye has to
// find first on a screen with sixty bodies on it, and it is the thing that
// scrolls off and has to be found again.
//
// What each light is for:
//
//   - the ripple, on the ground at the feet: this body is where you are.
//     It is under everything and it is the one that survives a crowd,
//     because a body standing over it hides its middle and not its edge.
//   - the ring, around the body: the same thing said around the outline, so
//     that a body drawn small at a wide zoom still has it.
//   - the motes, for the children of the line: of your blood, not you.
//     Scattered rather than a ring, so it can never be mistaken for the
//     played body at a glance.
//
// Gold, because gold has meant "yours" on this screen since the first day
// there was a player, and the rings that say it are still drawn on top.
// Nothing here is drawn when nobody is playing: there is no line to point
// at, and a world lit up for nobody is just a brighter world.
func (g *game) drawAura(screen *ebiten.Image, a *engine.Agent, x, y, radius float32) {
	if g.tiles == nil || g.played == 0 || a.Species != engine.SpeciesHuman {
		return
	}
	switch {
	case a.ID == g.played:
		g.stampLight(screen, "aura.ripple", x, y+radius*0.5, radius*3.2, colorPlayed)
		g.stampLight(screen, "aura.ring", x, y, radius*2.6, colorPlayed)
	case g.isKin(a.ID):
		g.stampLight(screen, "aura.motes", x, y, radius*2.6, colorKin)
	}
}

// stampLight draws one of those, if this set of art has it. Asked for by
// name rather than through frame(), because frame() will stand in the
// nearest body for a picture it has not got and a body drawn as somebody's
// halo is worse than no halo.
func (g *game) stampLight(screen *ebiten.Image, clip string, x, y, size float32, tint color.RGBA) {
	frames := g.tiles.clips[clip]
	if len(frames) == 0 {
		return
	}
	img := frames[(g.world.Tick()/animTicks)%len(frames)]
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	scale := float64(size) / float64(w)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(-float64(w)/2, -float64(h)/2)
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(float64(x), float64(y))
	op.ColorScale.ScaleWithColor(tint)
	op.Filter = ebiten.FilterNearest
	if scale < 1 {
		op.Filter = ebiten.FilterLinear
	}
	screen.DrawImage(img, op)
}

// colorItemGlow is what is thrown behind a thing lying on the ground.
//
// Fluorescent on purpose, and the only place in this world that is. The
// bodies are held to muted colours because colour there is a reading - whose
// line, what budget - and the country is moss and earth for reasons of its
// own. A berry lying in that is a small dark green dot on a large dark green
// field: the thing a player looks for most often and the hardest thing on
// the screen to see. It is not a reading, it is a marker, so it may shout.
//
// One colour for everything rather than each kind's own, which was tried
// first: a green glow under a green plant is no glow at all, and what the
// glow has to say is "something is lying here" - which kind it is, is the
// picture inside it.
var colorItemGlow = color.RGBA{0xdf, 0xff, 0x4f, 0xcc}

// itemGlowSize is how much bigger than the thing its glow is drawn.
const itemGlowSize = 1.7

// cropColours are the colours the numbered food tiles are painted in, in the
// palette a map is drawn with (tiled/samples/tilesets/001_simple/food_spawn.png),
// food1 first. The author picks a tile by its colour, so the world answers in
// the same colours - two pictures of the same thing that disagreed about
// which sort is which would be worse than one picture.
var cropColours = []color.RGBA{
	{0x46, 0xaf, 0x55, 0xff}, // food1
	{0x82, 0xbe, 0x3c, 0xff}, // food2
	{0xd2, 0xc3, 0x41, 0xff}, // food3
	{0xdc, 0x91, 0x37, 0xff}, // food4
	{0xcd, 0x5f, 0x4b, 0xff}, // food5
	{0x96, 0x73, 0xc3, 0xff}, // food6
}

// cropOf is which sort of plant one item grew from, counted from nought, or
// -1 for anything that is not a plant of a named sort.
//
// A plant carries its sort in its genes and hands it to its seedlings, so
// this reads the item rather than the ground it is standing on: a berry that
// seeded into the next country is still a berry.
func (g *game) cropOf(f engine.Food) int {
	if f.Kind != engine.FoodPlant || f.Genes.Strain == 0 {
		return -1
	}
	kinds := g.world.Config().PlantKinds
	if k := int(f.Genes.Strain) - 1; k < len(kinds) {
		return cropSlot(kinds[k].Name, k)
	}
	return -1
}

// cropSlot is which colour a sort is drawn in: the digit its name ends in
// when it has one, which is what makes food3 the third colour wherever it
// sits in the table, and otherwise its place in the table.
//
// The digit rather than the table's order because the order is the order the
// names were first met reading the map, and an author who paints food3 in one
// corner expects the colour of the third tile in the palette - not the colour
// of whatever happened to be painted third.
func cropSlot(name string, at int) int {
	if n := len(name); n > 0 && name[n-1] >= '1' && name[n-1] <= '9' {
		return int(name[n-1]-'1') % len(cropColours)
	}
	return at % len(cropColours)
}

// cropShade is what to multiply an item's picture by so that two sorts of
// plant lying side by side can be told apart.
//
// A shade rather than a stain: the numbers average to one, so what changes is
// the hue and not how bright the thing is, and a berry still looks like a
// berry. The pictures are drawn already coloured (which is why the item clip
// is not a tinted one), and a full tint would turn every sort into a
// silhouette of itself.
func cropShade(slot int) (float32, float32, float32) {
	if slot < 0 || slot >= len(cropColours) {
		return 1, 1, 1
	}
	c := cropColours[slot]
	mean := (float32(c.R) + float32(c.G) + float32(c.B)) / 3
	if mean == 0 {
		return 1, 1, 1
	}
	// Towards the colour's own proportions, with a floor under every channel
	// so that nothing is wiped out: multiplying keeps the thing as bright as
	// it was drawn, and what changes is its hue.
	toward := func(v uint8) float32 {
		f := (1 + float32(v)/mean) / 2
		if f < cropShadeFloor {
			return cropShadeFloor
		}
		return f
	}
	return toward(c.R), toward(c.G), toward(c.B)
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
	// The same shape, larger and lit up, underneath: an outline without
	// having to draw one - four offset copies were tried and the body
	// covered them - and enough to lift a berry off a field of moss. Drawn
	// from the same texture, so it costs a stamp and no batch.
	halo := &ebiten.DrawImageOptions{}
	halo.GeoM.Translate(-float64(w)/2, -float64(h)/2)
	halo.GeoM.Scale(float64(size)*itemGlowSize/float64(w), float64(size)*itemGlowSize/float64(h))
	halo.GeoM.Translate(float64(x), float64(y))
	slot := g.cropOf(f)
	halo.ColorScale.ScaleWithColor(cropGlow(slot))
	halo.Filter = ebiten.FilterLinear
	screen.DrawImage(img, halo)

	scale := float64(size) / float64(w)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(-float64(w)/2, -float64(h)/2)
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(float64(x), float64(y))
	if g.tiles.tint[clip] {
		op.ColorScale.ScaleWithColor(tint)
	} else if slot >= 0 {
		// Which sort of crop this is (2026-09-22). Only the picture of the
		// thing, never its glow: the glow says "something is lying here" and
		// one colour is what makes that legible.
		r, gr, b := cropShade(slot)
		op.ColorScale.Scale(r, gr, b, 1)
	}
	op.Filter = ebiten.FilterNearest
	if scale < 1 {
		op.Filter = ebiten.FilterLinear
	}
	screen.DrawImage(img, op)
	return true
}

// cropGlow is the colour of the light under one item: the ordinary one, or
// the sort's own where the map named sorts (2026-09-22).
//
// The glow says "something is lying here" and one colour is what made that
// legible, so this was the second thing tried. The first was to tint the
// picture and nothing else, and it could not be seen at the size an item is
// drawn: the pictures are nearly pure green, and no multiple of nothing is
// red. The light underneath is the one part of an item that is the viewer's
// own, so it is the part that can carry a name.
func cropGlow(slot int) color.RGBA {
	if slot < 0 || slot >= len(cropColours) {
		return colorItemGlow
	}
	c := cropColours[slot]
	// Lifted towards white, because what it has to stay is bright: a glow
	// the colour of the ground is not a glow.
	lift := func(v uint8) uint8 { return uint8(int(v) + (255-int(v))*2/5) }
	return color.RGBA{lift(c.R), lift(c.G), lift(c.B), colorItemGlow.A}
}

// How far an item's own picture may be pulled towards its sort's colour.
// Small on purpose: what this says is "a different crop", not "a different
// thing", and the light underneath is what says it loudly.
const cropShadeFloor = 0.55

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
