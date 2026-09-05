package engine

import (
	"errors"
	"fmt"
	"math"
)

// This file holds the controller a person drives (stage 19). It is the second
// implementation of Controller, and it is the whole of what "playing" means to
// the engine: the world asks whoever is driving an agent what to do, and drives
// the answer the same way whether a utility comparison or a player produced it.
//
// Two rules shape it, and both are about keeping the played game the same game
// the engine is running:
//
//   - A player is shown no more than the node knows. The perception carries no
//     true ability (see AgentView), so a controller that hands the perception
//     to a screen is showing exactly what the AI reasons about. Anything the
//     interface adds from World.Agents() would be a sense the AI does not have.
//   - A player may aim at no more than the node can see. Order refuses a target
//     that is not in the last perception, which is the same envelope the AI
//     picks its candidates from.
//
// What this file deliberately does not do is change when a decision happens.
// The engine asks on a trigger, not every tick, so an order is a standing one:
// it is what this node will answer with the next time it is asked. A player who
// does not want to wait asks for the question with World.RequestDecision.

// Errors an order can be refused with. They are worth telling apart because
// each one says something different about the interface that produced it.
var (
	// ErrNoOrderTarget is an action that needs somebody to act on and does not
	// name one.
	ErrNoOrderTarget = errors.New("this action needs a target")
	// ErrOrderUnseen is an order aimed at something this node cannot see. The
	// player can see it on the screen; the node cannot, and the node is the
	// one acting.
	ErrOrderUnseen = errors.New("this node cannot see that")
	// ErrNoDirection is a move with nowhere to go.
	ErrNoDirection = errors.New("a move needs a direction")
)

// HumanView is a copy of the last perception a human controller was handed.
//
// The world owns its perception buffer and rebuilds it for the next decision,
// so a controller that wants to show a player what its node knows has to take
// a copy. The slices here belong to the controller and are rewritten the next
// time it is asked: read them, do not keep them.
type HumanView struct {
	Tick   int
	Self   SelfView
	Foods  []FoodView
	Others []AgentView
}

// FoodByID returns what the node can see of a food item, and whether it can
// see it at all.
func (v *HumanView) FoodByID(id int) (FoodView, bool) {
	for i := range v.Foods {
		if v.Foods[i].ID == id {
			return v.Foods[i], true
		}
	}
	return FoodView{}, false
}

// AgentByID returns what the node can see of somebody else, and whether it can
// see them at all.
func (v *HumanView) AgentByID(id int) (AgentView, bool) {
	for i := range v.Others {
		if v.Others[i].ID == id {
			return v.Others[i], true
		}
	}
	return AgentView{}, false
}

// HumanController is a node driven by a person.
//
// It holds a standing order and hands it back whenever the engine asks. That
// indirection is not a shortcoming of the interface: the engine decides when
// an agent thinks, and a person types when they feel like it, so the two have
// to meet somewhere. They meet on the order.
type HumanController struct {
	order Action

	view     HumanView
	haveView bool

	asked      int
	askedAt    int
	lastAnswer Action
	voided     int
	body       int
}

// NewHumanController returns a controller whose standing order is to rest.
// Resting is the one action that needs nothing of the world, so a node that has
// just been taken over does something harmless until it is told otherwise.
func NewHumanController() *HumanController {
	return &HumanController{order: Action{Kind: ActRest}}
}

// Decide hands back the standing order. It is the whole of the controller: the
// player did the deciding, possibly some ticks ago.
//
// The order is not re-checked against what is in sight. An order is aimed at
// what the node could see when the player gave it, and by the time it is taken
// up the target may have walked out of view - which is exactly what happens to
// an AI decision too, since the engine keeps driving an action long after the
// sighting that prompted it.
//
// An order whose target is gone for good is another matter. The world says so
// by asking again with TriggerTargetLost, and a standing order kept through
// that would be answered with the same impossible thing every tick, for ever:
// the AI escapes because it re-scores, and a player who has walked away from
// the keyboard does not. So the order is voided and the node rests until it is
// told something else. This is the one place the controller overrules the
// player, and it overrules them with something harmless.
func (h *HumanController) Decide(p *Perception) Action {
	// A new body. The same controller drives a player's line rather than a
	// single agent, so this is what a handover feels like from in here: an
	// order belonged to the one who was given it, and the one who takes over
	// starts with none. It may not even be able to see what its parent was
	// after.
	if h.body != 0 && p.Self.ID != h.body {
		h.order = Action{Kind: ActRest}
	}
	h.body = p.Self.ID

	h.snapshot(p)
	if p.Trigger == TriggerTargetLost && h.order.TargetID != 0 {
		h.order = Action{Kind: ActRest}
		h.voided++
	}
	h.asked++
	h.askedAt = p.Tick
	h.lastAnswer = h.order
	return h.order
}

// snapshot copies the perception, since the world reuses its buffer.
func (h *HumanController) snapshot(p *Perception) {
	h.view.Tick = p.Tick
	h.view.Self = p.Self
	h.view.Foods = append(h.view.Foods[:0], p.Foods...)
	h.view.Others = append(h.view.Others[:0], p.Others...)
	h.haveView = true
}

// Order puts a standing order in, refusing one the node could not have come up
// with itself. What it checks is knowledge, not wisdom: a player is free to
// walk into a fight they will lose, and is not free to aim at somebody their
// node cannot see.
//
// Effort is clamped rather than refused, and a move direction is normalised, so
// that an interface does not have to do arithmetic to be correct.
func (h *HumanController) Order(a Action) error {
	if a.Kind >= numActionKinds {
		return fmt.Errorf("no such action (%d)", a.Kind)
	}
	a.Effort = clamp(a.Effort, 0, 1)

	switch a.Kind {
	case ActRest:
		a.TargetID, a.DX, a.DY = 0, 0, 0

	case ActMove:
		l := math.Hypot(a.DX, a.DY)
		if l < 1e-9 {
			return ErrNoDirection
		}
		a.DX, a.DY = a.DX/l, a.DY/l
		a.TargetID = 0

	case ActEat:
		if a.TargetID == 0 {
			return ErrNoOrderTarget
		}
		if !h.haveView {
			return ErrOrderUnseen
		}
		if _, ok := h.view.FoodByID(a.TargetID); !ok {
			return fmt.Errorf("food #%d: %w", a.TargetID, ErrOrderUnseen)
		}

	default: // attack, flee, observe, court
		if a.TargetID == 0 {
			return ErrNoOrderTarget
		}
		if !h.haveView {
			return ErrOrderUnseen
		}
		if a.TargetID == h.view.Self.ID {
			return errors.New("a node cannot do that to itself")
		}
		if _, ok := h.view.AgentByID(a.TargetID); !ok {
			return fmt.Errorf("node #%d: %w", a.TargetID, ErrOrderUnseen)
		}
	}

	h.order = a
	return nil
}

// Standing is the order the node will answer with the next time it is asked.
func (h *HumanController) Standing() Action { return h.order }

// View is what the node knew when it was last asked, and whether it has been
// asked at all. This is what an interface should draw: everything in it is
// something the node itself has, and nothing else is.
func (h *HumanController) View() (HumanView, bool) { return h.view, h.haveView }

// Asked reports how many times the engine has put the question, and the tick of
// the last time it did. An interface shows it so that a player can tell "my
// order has not been taken up yet" from "it was taken up and did nothing".
func (h *HumanController) Asked() (count, tick int) { return h.asked, h.askedAt }

// LastAnswer is the order that was actually handed over the last time the
// engine asked.
func (h *HumanController) LastAnswer() Action { return h.lastAnswer }

// Body is the agent this controller was last asked as, 0 before it has ever
// been asked. An interface compares it with the agent it thinks it is driving:
// they differ between a handover and the first question put to the new body,
// and everything in the view still belongs to the old one until then.
func (h *HumanController) Body() int { return h.body }

// Voided is how many orders have been dropped because what they were aimed at
// was gone. An interface shows it so that a player is told their order lapsed
// rather than left wondering why the node is lying down.
func (h *HumanController) Voided() int { return h.voided }

// RequestDecision asks the world to put the question to this agent on the next
// tick. It is how a player who has just changed their mind avoids waiting for
// the world to ask on its own, and it goes through the existing trigger
// machinery rather than around it: the decision is recorded as "requested",
// the same as any other.
//
// It is not a way to act every tick. Nothing stops an interface calling it
// every tick, but an agent that reconsiders that often does not follow a plan
// through - which is the reason deciding is trigger driven in the first place.
func (w *World) RequestDecision(id int) bool {
	a := w.agentByID(id)
	if a == nil || !a.Alive {
		return false
	}
	a.requestDecision(TriggerRequested)
	return true
}

// Heirs lists the agent's living children that have finished growing up, in
// the order they were born.
//
// This is the other half of the seam stage 0 laid down: a player's line does
// not end with one body. Nothing here is new machinery - it is ChildIDs, which
// the world has recorded since the beginning, filtered by the same maturity
// that decides whether an agent may court at all.
func (w *World) Heirs(id int) []int {
	a := w.agentByID(id)
	if a == nil {
		return nil
	}
	var heirs []int
	for _, child := range a.ChildIDs {
		c := w.agentByID(child)
		if c == nil || !c.Alive || !c.IsAdult(&w.cfg) {
			continue
		}
		heirs = append(heirs, child)
	}
	return heirs
}
