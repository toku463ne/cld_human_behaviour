package engine

import "testing"

// offerConfig is the gift world with the crying switched on.
func offerConfig() Config {
	cfg := giftConfig()
	cfg.OfferTicks = 30
	return cfg
}

// The whole of the rule on the crier's side: while it is crying, what is in
// its hand is a fact anybody who can see it reads - and when it stops, it is
// not.
func TestCryingShowsWhatIsInTheHandAndOnlyWhileItLasts(t *testing.T) {
	w := NewWorld(offerConfig())
	crierID := holding(t, w, 100, 100)
	watcherID := w.addAgent(Agent{Maturity: 1, X: 130, Y: 100, Vitality: 90,
		Genome: genomeOf(50, 50, 50)})

	crier := mustAgent(t, w, crierID)
	crier.Action = Action{Kind: ActOffer}

	view := sees(t, w, watcherID, crierID)
	if !view.Offering {
		t.Fatal("a body crying its wares is not seen to be holding anything out")
	}
	if view.OfferKind != FoodPlant || view.OfferValue <= 0 {
		t.Fatalf("the wares read as kind %v worth %v", view.OfferKind, view.OfferValue)
	}
	if view.OfferLeft != w.cfg.OfferTicks {
		t.Fatalf("a cry just started has %d ticks left of %d", view.OfferLeft, w.cfg.OfferTicks)
	}

	// Half way through, half of it is left; and once it is doing something
	// else, there is nothing to read.
	crier.actionTicks = w.cfg.OfferTicks / 2
	if got := sees(t, w, watcherID, crierID).OfferLeft; got != w.cfg.OfferTicks/2 {
		t.Fatalf("half way through a cry, %d ticks are left", got)
	}
	crier.Action = Action{Kind: ActRest}
	if sees(t, w, watcherID, crierID).Offering {
		t.Fatal("a body that has stopped crying is still advertising")
	}
}

// An empty hand advertises nothing, whatever the body is doing.
func TestAnEmptyHandCriesNothing(t *testing.T) {
	w := NewWorld(offerConfig())
	crierID := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Genome: genomeOf(50, 50, 50)})
	watcherID := w.addAgent(Agent{Maturity: 1, X: 130, Y: 100, Vitality: 90,
		Genome: genomeOf(50, 50, 50)})
	mustAgent(t, w, crierID).Action = Action{Kind: ActOffer}

	if sees(t, w, watcherID, crierID).Offering {
		t.Fatal("a body with nothing in its hand is advertising something")
	}
	// And it does not stand there for the full run either: the cry ends the
	// moment there is nothing to cry about.
	w.cry(mustAgent(t, w, crierID))
	if !mustAgent(t, w, crierID).needsDecision {
		t.Fatal("a cry with nothing to sell went on regardless")
	}
}

// What cannot be eaten is not on offer. A body reads somebody else's hand the
// way it reads the ground: what is in it that it could eat.
func TestWaresThatCannotBeEatenAreNotSeen(t *testing.T) {
	cfg := offerConfig()
	cfg.Stones = 0
	w := NewWorld(cfg)
	crierID := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Genome: genomeOf(50, 50, 50)})
	watcherID := w.addAgent(Agent{Maturity: 1, X: 130, Y: 100, Vitality: 90,
		Genome: genomeOf(50, 50, 50)})
	crier := mustAgent(t, w, crierID)
	// A stone: something to hold and nothing to eat.
	w.take(crier, w.putFood(Food{X: 101, Y: 100, Kind: FoodStone}))
	crier.Action = Action{Kind: ActOffer}

	if sees(t, w, watcherID, crierID).Offering {
		t.Fatal("a stone held up reads as something to eat")
	}
}

// The other half: a hungry body with a free hand walks towards somebody
// holding food out, and that walk is one option in the ordinary comparison.
func TestSomebodyHungryWalksTowardsTheWaresAndNobodyElseDoes(t *testing.T) {
	w := NewWorld(offerConfig())
	crierID := holding(t, w, 100, 100)
	mustAgent(t, w, crierID).Action = Action{Kind: ActOffer}
	buyerID := w.addAgent(Agent{Maturity: 1, X: 160, Y: 100, Vitality: 60,
		Hunger: 60, Genome: genomeOf(50, 50, 50)})

	act, ok := drawnToWares(t, w, buyerID)
	if !ok {
		t.Fatal("a hungry body was offered no way of going to the food held out for it")
	}
	if act.DX >= 0 {
		t.Fatalf("the walk to the wares heads away from them: %v,%v", act.DX, act.DY)
	}

	// With no room to take anything, there is nothing to walk for: the rule
	// that stops a gift is room, so the walk that arranges one asks the same
	// question.
	full := holding(t, w, 160, 140)
	fullBody := mustAgent(t, w, full)
	fullBody.Hunger = 60
	if _, ok := drawnToWares(t, w, full); ok {
		t.Fatal("a body with both hands full was offered a walk towards a gift it could not take")
	}
}

// A cry is worth the hand-over it would arrange, so a body with nobody worth
// giving anything to never makes one - and neither does one with empty hands.
func TestNobodyCriesWithNothingToSellOrNobodyToSellTo(t *testing.T) {
	w := NewWorld(offerConfig())
	crierID := holding(t, w, 100, 100)
	if cries(t, w, crierID) {
		t.Fatal("a body alone in the world cried its wares")
	}
	w.addAgent(Agent{Maturity: 1, X: 140, Y: 100, Vitality: 90, Genome: genomeOf(50, 50, 50)})
	if !cries(t, w, crierID) {
		t.Fatal("a body holding food with a stranger in sight was offered no cry")
	}

	empty := w.addAgent(Agent{Maturity: 1, X: 100, Y: 160, Vitality: 90,
		Genome: genomeOf(50, 50, 50)})
	if cries(t, w, empty) {
		t.Fatal("a body with nothing in its hand cried its wares")
	}
}

// Zero takes the word out of the world: no cry is offered, and nobody's hands
// are read even when they are held out.
func TestWithNoCryingNothingIsOfferedAndNoHandIsRead(t *testing.T) {
	cfg := offerConfig()
	cfg.OfferTicks = 0
	w := NewWorld(cfg)
	crierID := holding(t, w, 100, 100)
	buyerID := w.addAgent(Agent{Maturity: 1, X: 140, Y: 100, Vitality: 60,
		Hunger: 60, Genome: genomeOf(50, 50, 50)})
	mustAgent(t, w, crierID).Action = Action{Kind: ActOffer}

	if cries(t, w, crierID) {
		t.Fatal("a cry was offered in a world with no crying in it")
	}
	if sees(t, w, buyerID, crierID).Offering {
		t.Fatal("a hand was read in a world with no crying in it")
	}
	if _, ok := drawnToWares(t, w, buyerID); ok {
		t.Fatal("a walk to the wares was offered in a world with no crying in it")
	}
}

// sees is what one body's look turns up about another.
func sees(t *testing.T, w *World, watcher, seen int) AgentView {
	t.Helper()
	p := w.perceive(mustAgent(t, w, watcher))
	for i := range p.Others {
		if p.Others[i].ID == seen {
			return p.Others[i]
		}
	}
	t.Fatalf("#%d cannot see #%d at all", watcher, seen)
	return AgentView{}
}

// cries reports whether the comparison this body would run contains a cry.
func cries(t *testing.T, w *World, id int) bool {
	t.Helper()
	c, _ := scored(t, w, id)
	for _, o := range c.opts {
		if o.action.Kind == ActOffer {
			return true
		}
	}
	return false
}

// drawnToWares finds the walk towards somebody's wares, if there is one.
func drawnToWares(t *testing.T, w *World, id int) (Action, bool) {
	t.Helper()
	c, _ := scored(t, w, id)
	for _, i := range c.offerOpts {
		return c.opts[i].action, true
	}
	return Action{}, false
}

// scored runs the whole comparison for one body.
func scored(t *testing.T, w *World, id int) (*AIController, *Perception) {
	t.Helper()
	p := w.perceive(mustAgent(t, w, id))
	c := &AIController{}
	c.Decide(p)
	return c, p
}

// The control the stage is read against: the cry is made, costs its time and
// is chosen the same way, and nobody can read the hand. What it separates is
// what the advertisement does from what standing still does.
func TestWaresNobodyCanSeeAreStillCried(t *testing.T) {
	cfg := offerConfig()
	cfg.WaresSeen = false
	w := NewWorld(cfg)
	crierID := holding(t, w, 100, 100)
	watcherID := w.addAgent(Agent{Maturity: 1, X: 130, Y: 100, Vitality: 60,
		Hunger: 60, Genome: genomeOf(50, 50, 50)})
	mustAgent(t, w, crierID).Action = Action{Kind: ActOffer}

	if !cries(t, w, crierID) {
		t.Fatal("the cry stopped being worth making when nobody could read it")
	}
	if sees(t, w, watcherID, crierID).Offering {
		t.Fatal("a hand was read in the arm where hands cannot be read")
	}
	if _, ok := drawnToWares(t, w, watcherID); ok {
		t.Fatal("somebody walked towards wares they cannot see")
	}
}
