package main

import (
	"testing"

	"github.com/toku463ne/cld_human_behaviour/engine"
)

// aWorldWithFamilies runs far enough for children to have been born, and
// returns the world along with a parent and one of its children.
func aWorldWithFamilies(t *testing.T) (*engine.World, int, int) {
	t.Helper()
	cfg := engine.DefaultConfig()
	w := engine.NewWorld(cfg)
	for i := 0; i < 6000; i++ {
		w.Step()
		for _, a := range w.Agents() {
			if len(a.ChildIDs) == 0 {
				continue
			}
			for _, kid := range a.ChildIDs {
				if _, alive := w.AgentByID(kid); alive {
					return w, a.ID, kid
				}
			}
		}
	}
	t.Fatal("nobody had a living child in 6000 ticks")
	return nil, 0, 0
}

// The line does not end silently when the body dies: the player is asked, and
// a child that has not finished growing is on the list. That is the whole
// point of asking - World.Heirs would have left a newborn out and reported the
// line over with the child alive on screen.
func TestADeathRaisesTheChoiceOfBodyIncludingAChildStillGrowing(t *testing.T) {
	w, parent, kid := aWorldWithFamilies(t)
	cfg := w.Config()
	child, _ := w.AgentByID(kid)
	if child.IsAdult(&cfg) {
		t.Skip("the first child found had already grown up")
	}

	g := &game{world: w, play: playAsked, guided: engine.NewGuidedController(),
		played: parent, lineKids: []int{kid}, padKey: noKey}
	w.SetController(parent, g.guided)

	// The body is gone: this is what carryTheLineOn is called on every tick.
	g.played = -1 // an ID no agent has, which is what a dead one amounts to here
	g.lineKids = []int{kid}
	g.carryTheLineOn()

	if g.succession == nil {
		t.Fatal("the line ended without asking")
	}
	if !g.paused {
		t.Fatal("the world is still running behind the question")
	}
	if len(g.succession.picks) != 1 || g.succession.picks[0].id != kid {
		t.Fatalf("offered %+v, want the one living child #%d", g.succession.picks, kid)
	}

	g.goOnAs(kid)
	if g.succession != nil || g.played != kid {
		t.Fatalf("playing #%d after answering, succession %+v", g.played, g.succession)
	}
	if g.bodies != 1 || g.paused {
		t.Fatalf("bodies %d, paused %v after taking over", g.bodies, g.paused)
	}
}

// Nobody left alive is the one case where the line really is over, and it ends
// without a question standing in the way.
func TestALineWithNobodyLeftEndsWithoutAQuestion(t *testing.T) {
	w := engine.NewWorld(engine.DefaultConfig())
	g := &game{world: w, play: playAsked, guided: engine.NewGuidedController(),
		played: -1, lineKids: []int{-2, -3}, padKey: noKey}
	g.carryTheLineOn()
	if g.succession != nil {
		t.Fatal("asked which of nobody to go on as")
	}
	if g.play != playOff || g.played != 0 || g.lineKids != nil {
		t.Fatalf("the line did not end: play %v played %d kids %v", g.play, g.played, g.lineKids)
	}
}

// The dead body's own children come before the rest of the line, and the grown
// before the still growing.
func TestTheNearestOfKinIsOfferedFirst(t *testing.T) {
	w, parent, kid := aWorldWithFamilies(t)
	other := 0
	for _, a := range w.Agents() {
		if a.ID != parent && a.ID != kid {
			other = a.ID
			break
		}
	}
	g := &game{world: w, lineKids: []int{other, kid}, padKey: noKey}
	picks := g.survivors(parent)
	if len(picks) != 2 {
		t.Fatalf("offered %d bodies, want 2", len(picks))
	}
	if picks[0].id != kid {
		t.Fatalf("offered #%d first, want the dead one's own child #%d", picks[0].id, kid)
	}
}

// h escalates: the AI has it, then you answer at the turning points, then you
// drive it, then the AI has it again - and the node it let go of is still
// selected, because h is how it is taken up again.
func TestPressingHEscalatesControlAndLeavesTheNodeSelected(t *testing.T) {
	w := engine.NewWorld(engine.DefaultConfig())
	for i := 0; i < 200; i++ {
		w.Step()
	}
	id := w.Agents()[0].ID

	g := &game{world: w, padKey: noKey, effort: 1}
	g.selectAgent(id)

	g.toggleControl()
	if g.play != playAsked || g.played != id || g.guided == nil {
		t.Fatalf("first press: play %v played %d guided %v", g.play, g.played, g.guided != nil)
	}
	g.toggleControl()
	if g.play != playDriven || g.played != id || g.human == nil || g.guided != nil {
		t.Fatalf("second press: play %v played %d human %v", g.play, g.played, g.human != nil)
	}
	g.toggleControl()
	if g.play != playOff || g.played != 0 {
		t.Fatalf("third press: play %v played %d", g.play, g.played)
	}
	if g.selected != id {
		t.Fatalf("selection is #%d after letting go of #%d: h could not take it up again",
			g.selected, id)
	}
	g.toggleControl()
	if g.play != playAsked || g.played != id {
		t.Fatalf("fourth press: play %v played %d, want the same node taken up again", g.play, g.played)
	}
}

// Taking a node up puts a person behind its proposals: an unattended guided
// controller answers with the node's own rule, and one a player is behind
// waits for them.
func TestTakingANodeUpMeansAnsweringItsProposals(t *testing.T) {
	w := engine.NewWorld(engine.DefaultConfig())
	for i := 0; i < 200; i++ {
		w.Step()
	}
	id := w.Agents()[0].ID

	if got := engine.NewGuidedController().AnswerCourt(7); got != engine.CourtLeaveIt {
		t.Fatalf("a guided controller nobody is behind answers %v, want it left to the node", got)
	}

	g := &game{world: w, padKey: noKey, effort: 1}
	g.selectAgent(id)
	g.toggleControl() // the asked mode, which is where a person arrives first
	if got := g.guided.AnswerCourt(7); got != engine.CourtWaiting {
		t.Fatalf("a played node answers %v, want it to wait for the player", got)
	}
	g.guided.AnswerProposal(7, false)
	if got := g.guided.AnswerCourt(7); got != engine.CourtRefuse {
		t.Fatalf("after the player said no the controller answers %v", got)
	}
}
