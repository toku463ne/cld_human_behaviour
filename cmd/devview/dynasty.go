package main

import (
	"fmt"
	"sort"

	"github.com/hajimehoshi/ebiten/v2/inpututil"

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
	g.dyn.goals = g.world.GoalRegions()
	sort.Ints(g.dyn.goals)
	if len(g.dyn.goals) == 0 {
		g.dyn.over, g.dyn.why = true, "this map marks no goal blocks, so there is nothing to win"
		g.say("no goal blocks on this map: run it with a map that marks some")
		return
	}
	g.say("playing for line %d. settle it in all %d goal blocks at once", a.Lineage, len(g.dyn.goals))
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
	if d.waiting > 0 {
		g.runTheWait()
		return
	}
	if d.picks != nil {
		return // the menu is up and the clock is stopped behind it
	}
	d.held = len(g.settledGoals())
	if d.held == len(d.goals) && d.reads >= engine.DefaultSettleWindow {
		d.over = true
		d.why = fmt.Sprintf("your line is settled in all %d goal blocks at once", len(d.goals))
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
	if d.reads < engine.DefaultSettleWindow {
		return fmt.Sprintf("line %d: settling in (%d of %d readings taken before the goal counts)",
			d.line, d.reads, engine.DefaultSettleWindow)
	}
	return fmt.Sprintf("line %d: settled in %d of %d goal blocks", d.line, d.held, len(d.goals))
}

// dynPicks is the transfer menu for whoever is drawing it, empty when no
// question is up.
func (g *game) dynPicks() []dynastyPick {
	if g.dyn == nil {
		return nil
	}
	return g.dyn.picks
}
