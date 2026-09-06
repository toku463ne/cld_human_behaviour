package engine

import "math"

// Feeding a child (2026-09-06).
//
// Until now childcare was entirely one-sided: the child did the staying and
// nothing at all was asked of the parent - no feeding, no cost, and no term in
// anybody's utility formula for the survival of a child (world.go). This is
// the other half of it, and it is the first rule in the world where one agent
// pays for another.
//
// It is deliberately not a new action, not a new state, and not a decision.
// Nobody chooses to feed anybody: a parent that eats while it has a child
// beside it simply gets less of the meal, and the child gets the rest. What is
// shared is the mouthful, not the item, so nothing about who owns food, who
// races for it, or who fights over it changes.
//
// Three things it does change, and they are the reason it is measured rather
// than assumed:
//
//   - The parent is honest about it. Perception carries what a mouthful is
//     actually worth to this body now, so a nursing parent knows its meals are
//     half meals and goes looking for more of them. Hiding it would make the
//     utility formula wrong about the one quantity it is most sensitive to.
//   - The child eats without foraging. It has been able to feed itself since
//     stage 7d; this makes the leash worth something to it directly, rather
//     than only through the company of somebody big.
//   - The parent bears a real cost for the first time, and the formula still
//     has no term for the child. The parent cannot want this, cannot refuse
//     it, and cannot be selected for doing it well. What selection sees is
//     only the line: bodies that fed children left children.

// underCare reports whether this one is still a child in somebody's keeping.
// It is the same question stillReared asks, without spending the tick of
// childhood that stillReared spends: anything that only wants to know must ask
// here, and only the one place that runs the leash may ask there.
func (w *World) underCare(a *Agent) bool {
	if a.GuardianID == 0 {
		return false
	}
	if w.cfg.RearingUntilGrown {
		return a.Maturity < 1
	}
	return a.RearingTimer > 0
}

// mouthsToFeed are the children this agent would hand part of a meal to: still
// in its keeping, and close enough to be handed anything.
//
// The distance is the leash's own (RearingRadius) rather than a number of its
// own. A child that is being kept within a radius is by definition inside it
// nearly all the time, so this is not a second rule about staying close - it
// is what stops a mouthful reaching a child on the other side of the world in
// the tick after its parent died to something over there.
func (w *World) mouthsToFeed(a *Agent) []*Agent {
	if w.cfg.ParentFeedShare <= 0 || len(a.ChildIDs) == 0 {
		return nil
	}
	r2 := w.cfg.RearingRadius * w.cfg.RearingRadius
	var out []*Agent
	for _, id := range a.ChildIDs {
		c := w.agentByID(id)
		if c == nil || !c.Alive || c.GuardianID != a.ID || !w.underCare(c) {
			continue
		}
		if dist2(a.X, a.Y, c.X, c.Y) > r2 {
			continue
		}
		out = append(out, c)
	}
	return out
}

// keptShare is how much of a mouthful this body gets to keep. One when it is
// feeding nobody, which is every agent in the world with the rule off.
func (w *World) keptShare(a *Agent) float64 {
	if len(w.mouthsToFeed(a)) == 0 {
		return 1
	}
	return 1 - clamp(w.cfg.ParentFeedShare, 0, 1)
}

// share hands out one item between whoever is eating it: this body and the
// children it is feeding. It returns what the eater keeps, having already put
// the rest into the children.
//
// The children pay their part of what the food was defended with as well
// (stage 17b). A rule where the parent swallows the poison and the child gets
// the calories would be a claim about the world that nothing here is making.
func (w *World) share(a *Agent, f *Food) float64 {
	mouths := w.mouthsToFeed(a)
	if len(mouths) == 0 {
		return 1
	}
	s := clamp(w.cfg.ParentFeedShare, 0, 1)
	if w.cfg.ParentFeedWasted {
		// The control: the parent pays and nobody eats it. The children are
		// still looked up, so that what is being compared is the same set of
		// meals in the same situations.
		return 1 - s
	}
	each := s / float64(len(mouths))
	for _, c := range mouths {
		c.Hunger = math.Max(0, c.Hunger-each*w.cfg.FoodNutrition*w.dietValue(c, f.Kind))
		if w.cfg.PlantDefence && f.Kind == FoodPlant {
			c.Vitality -= each * f.Genes.Poison * w.cfg.PoisonDamage
		}
		// It has been living on this as much as its parent has, which is what
		// the diet ledger is for. Nothing else about the child changes: it is
		// not told, it does not decide, and it owes nobody anything.
		w.noteEaten(c, f.Kind)
	}
	return 1 - s
}
