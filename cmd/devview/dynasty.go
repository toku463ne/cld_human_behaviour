package main

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/toku463ne/cld_human_behaviour/engine"
)

// The dynasty game (TODO 6, decision #130).
//
// What it adds to playing a node. Until now a line ended or went on: the body
// died, its children were offered, and one of them was taken up on the spot.
// There was nothing to win. This puts a goal at the end of it - your line
// living in every one of the map's goal blocks at once - and it puts a price
// on dying: ten years pass with nobody at the wheel, and you come back
// wherever your line still has somebody, if it still has anybody.
//
// All of it is here rather than in the engine. The engine has no player, no
// goal, no game over and no clock that stops for anybody (stage 19), and the
// two things it does supply - which line a body belongs to and where a line
// still has somebody - are facts about bodies that a run nobody is watching
// has just as much.
//
// Three decisions, and a count decided all three.
//
// The line is the tree, and "one heir per country" is a question rather than
// a tag. PLAN.md had settled on a third inheritance rule, LineageRegionChain,
// where a newborn took its mother's line only if no carrier was standing in
// that block. It is gone, and it went for two reasons an hour apart.
//
// Counted on the map this is played on it falls apart: a line holds 2.09
// blocks of twelve against the tree's 9.25, one 10-year wait takes it to
// 0.91, and 0.55 of the time the line is simply gone. A menu that is empty
// more than half the time is not a choice.
//
// And then it turned out not to be the thing it was named after. What "one
// heir per country" meant was: three sons in a block, the eldest dies and the
// second is the heir, he dies and the third is, the third leaves a child and
// dies and the child is. That rule handed the tag out at birth and nowhere
// else, so a second son born while his brother stood there was never a
// carrier and never could become one, and a block whose carrier died stayed
// empty until some later birth happened to land in it.
//
// The reason it could not have worked is worth keeping: who the heir is
// changes the moment somebody dies, and a birth is nowhere near that moment.
// A tag is written once, at birth. So the thing has to be asked, not stored -
// which is LineHeirs, and which walks exactly the succession above. PLAN.md
// wondered whether the game would need a second tag; it needed none, because
// what it wanted was never a tag.
//
// Winning is settling, not standing. Standing in the goal blocks is something
// a line does without being asked (0.42 of runs hold all three by accident).
// Settling is the instrument stage TODO-5 built for exactly this: most of a
// body's recent past spent in one block.
//
// The wait is not decoration. A year of the world running without you is what
// makes leaving descendants somewhere a decision rather than a detail: where
// the line grows is not up to you once you are dead. It was ten years until
// it was measured, and ten years was not a price, it was the whole game -
// 0.92 of runs were over before the player had done anything.

const (
	// dynastyWaitTicks is how long the world runs after the played body dies
	// before the line is picked up again. Five hundred ticks is one year of
	// world time (TicksPerYear 500).
	//
	// It was ten years to begin with, to sit beside GeniusRate's "one genius
	// in ten years" - a transfer is marked by a genius birth, so the two were
	// kept from treading on each other. That was dropped on 2026-09-21 (the
	// user's call: a genius is the player's, or placed on purpose), which
	// left the length free, and the measurement said to spend it. Ten years
	// of nobody at the wheel ended 0.92 of runs in an extinct line before the
	// player had done anything; at one year it is 0.56 to 0.75, and the
	// number of lifetimes a run gets rises from about 2 to about 3.5.
	dynastyWaitTicks = 500

	// dynastyWaitSpeed is how many ticks of that wait are run per frame. The
	// wait is meant to be felt and not sat through: at this rate ten years
	// takes about two seconds.
	dynastyWaitSpeed = 40
)

// dynasty is the game's own state. The world knows none of it.
type dynasty struct {
	// line is the tag the player is playing for, taken from the first body
	// they were given. It never changes: what is being played is a line of
	// descent, and a line that could be swapped would make the goal
	// meaningless.
	line uint16

	// goals are the blocks the map marked, and settle is how "lives there"
	// is measured.
	goals  []int
	settle *engine.SettlementTracker

	// villages is what the player has founded, by block, and lastTick the
	// tick the year was last counted on (the world runs several ticks a
	// frame, and the year is counted in ticks and not in frames).
	villages map[int]*village
	lastTick int

	// waiting is how many ticks of the ten years are left, and waitFrom the
	// body whose death started them.
	waiting  int
	waitFrom int

	// picks is the transfer menu once the wait is over: one carrier per
	// block, best-looking first.
	picks []dynastyPick

	// over says the run has finished, and why.
	over bool
	why  string

	// held is how many goal blocks the line was settled in last time it was
	// checked, for the line on the screen.
	held int

	// lastRead is the tick the last settlement reading was taken on, and
	// reads how many have been taken. The win waits for a full window of
	// them.
	//
	// It is here and not in the tracker because the tracker is right as it
	// is: it judges a body on whatever readings it has so that a world is
	// not silent for its first thousand ticks, which is what an instrument
	// should do. A win condition cannot afford that - on the very first
	// reading every body is "settled" where it happens to be standing, and
	// a run would be won at the moment it started.
	reads    int
	lastRead int
}

type dynastyPick struct {
	id     int
	region int
	goal   bool
	about  string
}

// startDynasty is called when a body is first taken over in this mode.
func (g *game) startDynasty() {
	if g.dyn == nil || g.dyn.line != 0 || g.played == 0 {
		return
	}
	a, alive := g.world.AgentByID(g.played)
	if !alive {
		return
	}
	g.dyn.line = a.Lineage
	g.dyn.villages = map[int]*village{}
	g.dyn.lastTick = g.world.Tick()
	g.dyn.goals = g.world.GoalRegions()
	sort.Ints(g.dyn.goals)
	if len(g.dyn.goals) == 0 {
		g.dyn.over, g.dyn.why = true, "this map marks no goal blocks, so there is nothing to win"
		g.say("no goal blocks on this map: run it with a map that marks some")
		return
	}
	prices := make([]string, 0, len(g.dyn.goals))
	for _, r := range g.dyn.goals {
		prices = append(prices, fmt.Sprintf("block %d: %d coins", r, g.villagePrice(r)))
	}
	g.say("playing for house %d. found a village in each of the %d goal blocks (%s) and leave your house living there a year",
		a.Lineage, len(g.dyn.goals), strings.Join(prices, ", "))
}

// watchDynasty is the whole of the mode's per-frame work: take the settlement
// reading when it is due, run the wait down if one is running, and see
// whether the run is over either way.
func (g *game) watchDynasty() {
	d := g.dyn
	if d == nil || d.over || d.line == 0 {
		return
	}
	g.readSettlement()
	// The children of the played body belong to the player's house, and the
	// year each village is waiting out is counted in ticks.
	g.adoptChildren()
	tick := g.world.Tick()
	g.watchVillages(tick - d.lastTick)
	d.lastTick = tick
	if d.waiting > 0 {
		g.runTheWait()
		return
	}
	if d.picks != nil {
		return // the menu is up and the clock is stopped behind it
	}
	d.held = 0
	for _, v := range d.villages {
		if v.done {
			d.held++
		}
	}
	if g.villagesWon() {
		d.over = true
		d.why = fmt.Sprintf("your house has lived a year in every one of the %d goal blocks", len(d.goals))
		g.paused = true
		g.say("WON: %s", d.why)
	}
}

// readSettlement takes a reading if one is due, and counts it.
//
// Due is measured against the tick the last one was taken on, not against
// "the tick divides by the step". The frame rate and the clock are not the
// same thing here: at speed the world moves several ticks between frames and
// a test for divisibility would skip readings, and at a tenth speed the same
// tick comes round for ten frames and the window would fill with ten copies
// of one position - which is exactly the reading that makes a body look
// settled when it is walking.
func (g *game) readSettlement() {
	d := g.dyn
	if tick := g.world.Tick(); d.reads == 0 || tick-d.lastRead >= engine.DefaultSettleStep {
		d.settle.Observe(g.world)
		d.reads++
		d.lastRead = tick
	}
}

// settledGoals is which goal blocks the line lives in right now: a block
// counts when somebody of the line is settled in it, by the instrument's own
// definition of settled.
func (g *game) settledGoals() map[int]int {
	d := g.dyn
	out := map[int]int{}
	for _, a := range g.world.Agents() {
		if !a.Alive || a.Lineage != d.line {
			continue
		}
		r, ok := d.settle.SettledIn(a.ID)
		if !ok {
			continue
		}
		for _, goal := range d.goals {
			if r == goal {
				out[r]++
			}
		}
	}
	return out
}

// beginTheWait is what a death costs: ten years with nobody at the wheel.
// Called instead of the ordinary succession question.
func (g *game) beginTheWait(dead int) {
	d := g.dyn
	d.waiting, d.waitFrom = dynastyWaitTicks, dead
	d.picks = nil
	g.play, g.human, g.guided = playOff, nil, nil
	g.played = 0
	g.say("#%d died. ten years pass with nobody at the wheel", dead)
}

// runTheWait advances the world through the wait, a chunk per frame, and puts
// up the transfer menu at the end of it.
func (g *game) runTheWait() {
	d := g.dyn
	for i := 0; i < dynastyWaitSpeed && d.waiting > 0; i++ {
		g.world.Step()
		d.waiting--
		g.readSettlement()
	}
	if d.waiting > 0 {
		return
	}
	d.picks = g.transferPicks()
	if len(d.picks) == 0 {
		d.over = true
		d.why = fmt.Sprintf("ten years after #%d died, your line has nobody left anywhere", d.waitFrom)
		g.paused = true
		g.say("LOST: %s", d.why)
		return
	}
	g.paused = true
	g.say("ten years on. 1-%d: which country does the line carry on in?", len(d.picks))
}

// transferPicks is the menu: the eldest carrier of the line in each block,
// the goal blocks first, and at most as many as there are keys to press.
func (g *game) transferPicks() []dynastyPick {
	d := g.dyn
	cfg := g.world.Config()
	heirs := g.world.LineHeirs(d.line)
	settled := g.settledGoals()
	goal := map[int]bool{}
	for _, r := range d.goals {
		goal[r] = true
	}
	picks := make([]dynastyPick, 0, len(heirs))
	for r, id := range heirs {
		a, alive := g.world.AgentByID(id)
		if !alive {
			continue
		}
		mark := ""
		if goal[r] {
			mark = " GOAL"
			if settled[r] > 0 {
				mark = " GOAL, already settled"
			}
		}
		picks = append(picks, dynastyPick{
			id: id, region: r, goal: goal[r],
			about: fmt.Sprintf("block %d%s: #%d, %.1f years, %.0f%% grown, vit %.0f/%.0f",
				r, mark, id, float64(a.Age)/float64(cfg.TicksPerYear),
				a.Maturity*100, a.Vitality, a.MaxVitality(&cfg)),
		})
	}
	// Goal blocks first, then the oldest heir: what the player is choosing
	// between is countries, and the ones that count are the ones on the list.
	sort.SliceStable(picks, func(i, j int) bool {
		if picks[i].goal != picks[j].goal {
			return picks[i].goal
		}
		return picks[i].region < picks[j].region
	})
	if len(picks) > len(choiceKeys) {
		picks = picks[:len(choiceKeys)]
	}
	return picks
}

// transferTo carries the line on in the chosen body, which is born again as a
// genius: the same Inspire stage 22 built, used as the mark of a transfer.
// It draws no random numbers, so a run is the same run whether or not
// anybody was playing it.
func (g *game) transferTo(p dynastyPick) {
	d := g.dyn
	if !g.world.SetController(p.id, g.controller()) {
		g.say("#%d is gone too", p.id)
		if d.picks = g.transferPicks(); len(d.picks) == 0 {
			d.over = true
			d.why = fmt.Sprintf("ten years after #%d died, your line has nobody left anywhere", d.waitFrom)
			g.say("LOST: %s", d.why)
		}
		return
	}
	from := d.waitFrom
	d.picks, d.waitFrom = nil, 0
	g.play = playDriven
	g.human = engine.NewHumanController()
	g.world.SetController(p.id, g.human)
	if added, err := g.world.Inspire(p.id); err == nil && added > 0 {
		g.say("the line goes on in block %d, as #%d, born knowing more than it should", p.region, p.id)
	}
	g.takeOver(from, p.id, true)
	g.paused = false
}

// handleDynastyInput answers the transfer question. Nothing else means
// anything while it stands, the same as the ordinary succession question.
func (g *game) handleDynastyInput() bool {
	d := g.dyn
	if d == nil || len(d.picks) == 0 {
		return false
	}
	for i, key := range choiceKeys {
		if i < len(d.picks) && inpututil.IsKeyJustPressed(key) {
			g.transferTo(d.picks[i])
			return true
		}
	}
	return true // the clock stays stopped until one of them is pressed
}

// dynastyLines is what the mode puts on the screen, under the ordinary status.
func (g *game) dynastyStatus() string {
	d := g.dyn
	if d == nil || d.line == 0 {
		return ""
	}
	if d.over {
		return "line " + fmt.Sprint(d.line) + ": " + d.why + " (esc to keep watching)"
	}
	if d.waiting > 0 {
		return fmt.Sprintf("line %d: ten years pass... %d ticks left, nobody at the wheel",
			d.line, d.waiting)
	}
	if len(d.picks) > 0 {
		return fmt.Sprintf("line %d: which country does it carry on in? 1-%d (see the panel)",
			d.line, len(d.picks))
	}
	parts := make([]string, 0, len(d.goals))
	for _, r := range d.goals {
		switch v := d.villages[r]; {
		case v == nil:
			parts = append(parts, fmt.Sprintf("%d: %d coins", r, g.villagePrice(r)))
		case v.done:
			parts = append(parts, fmt.Sprintf("%d: yours", r))
		default:
			parts = append(parts, fmt.Sprintf("%d: %.2f of a year", r,
				float64(v.held)/villageHoldTicks))
		}
	}
	return fmt.Sprintf("house %d: %d of %d blocks held | %d coins in hand | blocks %s | [v] found a village",
		d.line, d.held, len(d.goals), g.coinsHeld(g.played), strings.Join(parts, ", "))
}

// dynPicks is the transfer menu for whoever is drawing it, empty when no
// question is up.
func (g *game) dynPicks() []dynastyPick {
	if g.dyn == nil {
		return nil
	}
	return g.dyn.picks
}

// Villages (2026-09-22, the user's own design).
//
// What the dynasty asked for before this was "settle your line in every goal
// block at once", and what a player found there was a wall: a pair bond ends
// when the child is born, so nobody follows you anywhere, and a far country
// is full of strangers who are not yours and never will be. You could walk to
// a goal block; you could not leave a line in one.
//
// So the player founds the village themselves. It costs coins, which are
// lying about the world and have never had anything to buy until now, and the
// coins are laid on the ground where the village stands rather than spent
// into nothing - money in this world is conserved, and a village with its
// founder's money at its feet is a better picture anyway.
//
// The village hands out a line of its own and never the player's. That is the
// point of it: what the player has to do afterwards is marry into it and
// leave children there, and the goal is not "a village stands here" but "my
// house has been living here a year".
//
// The map says what a country asks. Price and years are properties of the
// goal region (tiled: price, years), because a rich valley and a bare shelf
// are not worth the same and the author is the one who drew the difference.
// The engine reads neither of them.

// The colours of a signboard: the post and the board it is nailed to, and
// the three things a board can say.
var (
	colorSignPost    = color.RGBA{0x6b, 0x4b, 0x2a, 0xff}
	colorSignBoard   = color.RGBA{0x2b, 0x2b, 0x2b, 0xdd}
	colorSignPrice   = color.RGBA{0xf0, 0xc0, 0x40, 0xff}
	colorSignWaiting = color.RGBA{0x60, 0xc0, 0xf0, 0xff}
	colorSignHeld    = color.RGBA{0x70, 0xe0, 0x70, 0xff}
)

const (
	// villageHoldTicks is how long the player's house has to be living in a
	// country before it counts: one year of world time.
	villageHoldTicks = 500

	// villagePrice is what a country asks when the map says nothing, and
	// villageYears how long a village keeps its own line up when the map says
	// nothing about that either.
	villagePrice = 10
	villageYears = 10

	// villageRate is how often a village sends somebody, and villageCap how
	// many of its own line it keeps alive. The same figures -villages hands
	// out, because a village founded by a player is a village.
	villageRate = 100
	villageCap  = 20
)

// village is one country the player has founded in, and how long their house
// has been living there since.
type village struct {
	region int
	at     int    // the tick it was founded
	line   uint16 // the village's own line, which is not the player's
	held   int    // ticks the player's house has been living here, unbroken
	done   bool
	x, y   float64
}

// foundVillage is the V key: pay the country's price and put a village where
// the played body is standing.
func (g *game) foundVillage() {
	d := g.dyn
	if d == nil || d.over || g.played == 0 {
		return
	}
	a, alive := g.world.AgentByID(g.played)
	if !alive {
		return
	}
	region := g.world.RegionAt(a.X, a.Y)
	goal := false
	for _, r := range d.goals {
		if r == region {
			goal = true
		}
	}
	if !goal {
		g.say("a village only counts in a goal block, and this is block %d", region)
		return
	}
	if d.villages[region] != nil {
		g.say("block %d already has your village", region)
		return
	}
	price := g.villagePrice(region)
	if held := g.coinsHeld(a.ID); held < price {
		g.say("block %d asks %d coins and you have %d", region, price, held)
		return
	}
	years := g.world.GoalYears(region)
	if years <= 0 {
		years = villageYears
	}
	cfg := g.world.Config()
	line, err := g.world.FoundNest(a.ID, price, engine.HumanNest{
		Name: fmt.Sprintf("village %d", region),
		Rate: villageRate, Cap: villageCap,
		Life: int(years * float64(cfg.TicksPerYear)),
	})
	if err != nil {
		g.say("%v", err)
		return
	}
	d.villages[region] = &village{region: region, at: g.world.Tick(), line: line, x: a.X, y: a.Y}
	g.say("village founded in block %d for %d coins. now leave your house living here for a year",
		region, price)
}

// villagePrice is what this country asks, in coins.
func (g *game) villagePrice(region int) int {
	if p := g.world.GoalPrice(region); p > 0 {
		return int(p)
	}
	return villagePrice
}

// coinsHeld is how much money one body has in its hands.
func (g *game) coinsHeld(id int) int {
	n := 0
	for _, f := range g.world.CarriedBy(id) {
		if f.Kind == engine.FoodCoin {
			n++
		}
	}
	return n
}

// watchVillages runs the year each founded village is waiting out.
//
// The clock runs while somebody of the player's house who is not the played
// body is standing in the block, and goes back to nought when there is
// nobody: what is being asked is whether the house lives there, and a player
// standing in it themselves is not a house living there. It is the reason the
// player has to marry into the village at all.
func (g *game) watchVillages(ticks int) {
	d := g.dyn
	if d == nil || len(d.villages) == 0 || ticks <= 0 {
		return
	}
	living := map[int]int{}
	for _, a := range g.world.Agents() {
		if !a.Alive || a.Lineage != d.line || a.ID == g.played {
			continue
		}
		living[g.world.RegionAt(a.X, a.Y)]++
	}
	for region, v := range d.villages {
		if v.done {
			continue
		}
		if living[region] == 0 {
			v.held = 0
			continue
		}
		v.held += ticks
		if v.held >= villageHoldTicks {
			v.done = true
			g.say("block %d is yours: your house has lived there a year", region)
		}
	}
}

// adoptChildren puts the played body's children into the player's house.
//
// A child takes its mother's line, which is what a family is in this engine.
// A player who is male would otherwise father children who belong to their
// mother's house and count for nothing, and the whole of this mode is about
// leaving your own house somewhere. It is the game saying whose house a child
// is of, through the one call the engine offers for it.
func (g *game) adoptChildren() {
	d := g.dyn
	if d == nil || d.line == 0 || g.played == 0 {
		return
	}
	for _, a := range g.world.Agents() {
		if !a.Alive || a.Lineage == d.line {
			continue
		}
		for _, parent := range a.ParentIDs {
			if parent == g.played {
				g.world.SetLineage(a.ID, d.line)
				break
			}
		}
	}
}

// villagesWon says whether every goal block has had the house living in it
// for its year.
func (g *game) villagesWon() bool {
	d := g.dyn
	if len(d.goals) == 0 {
		return false
	}
	for _, r := range d.goals {
		v := d.villages[r]
		if v == nil || !v.done {
			return false
		}
	}
	return true
}

// drawGoalSigns puts a board up in every goal block.
//
// The price is a thing a player has to know before walking anywhere, and the
// panel is the wrong place for it: what is being decided is "which of these
// countries can I afford", and that is a question about the map. So the map
// answers it, at the middle of each block, in the block's own terms - the
// price while there is no village, how far through its year one is, and
// nothing at all once the block is held.
func (g *game) drawGoalSigns(screen *ebiten.Image) {
	d := g.dyn
	if d == nil || d.line == 0 || len(d.goals) == 0 {
		return
	}
	for _, r := range d.goals {
		// A spot inside the block and not the middle of what it spans: a
		// painted country may be two patches with somebody else's land
		// between them, and a sign has to stand on its own ground.
		x, y := g.onScreen(g.world.RegionCentre(r))
		text, c := fmt.Sprintf("%d coins", g.villagePrice(r)), colorSignPrice
		if v := d.villages[r]; v != nil {
			if v.done {
				text, c = "yours", colorSignHeld
			} else {
				text, c = fmt.Sprintf("%.2f yr", float64(v.held)/villageHoldTicks), colorSignWaiting
			}
		}
		w := float32(len(text)*6 + 8)
		// A post and a board on it, so that it reads as something standing in
		// the country rather than as a label floating over it.
		vector.DrawFilledRect(screen, x-1, y-6, 2, 12, colorSignPost, true)
		vector.DrawFilledRect(screen, x-w/2, y-20, w, 15, colorSignBoard, true)
		vector.StrokeRect(screen, x-w/2, y-20, w, 15, 1, c, true)
		ebitenutil.DebugPrintAt(screen, text, int(x-w/2)+4, int(y)-21)
	}
}
