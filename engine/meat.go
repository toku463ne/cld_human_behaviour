package engine

import "math"

// What a carcass is worth (stage 39).
//
// Until here, eating did one thing: it took hunger away. Vitality came back
// only from resting while fed, so the whole of "get better" was a matter of
// lying down somewhere safe. This is the first food that touches vitality
// directly - a body that has just brought something down mends on the spot,
// out of what it brought down.
//
// Two things are deliberately kept apart, because they move by different
// mechanisms and mixing them would make the measurement unreadable (#68):
//
//   - How much meat there is (MeatPerBudget, which is not here: dropMeat has
//     scaled the drop by the dead body's budget since stage 11). That is the
//     trading side - whether a kill leaves more than those who made it can
//     use.
//   - What one item of it does (MeatVitality and MeatNutrition, which are).
//     That is the nutritional side, and it reaches the utility formula
//     through the estimate of dying that every other option is scored by.
//
// No new term goes into the utility formula. Healing enters where resting
// already enters - as vitality the body expects to have afterwards - which
// means the existing ceiling does the interesting part on its own: pressure
// hardly falls at all for a body that is nearly whole, so a full-vitality
// agent scores a carcass low and eats a plant instead. Nobody wrote "save it
// for later", and there is no state that says a body is saving anything.

// meatWorth is the multiplier on what one item of this kind takes off hunger.
// One for everything except meat, and one for meat too unless the world says
// otherwise: the control arm for the healing is a carcass that fills a
// stomach by the same amount and mends nothing.
func (w *World) meatWorth(kind FoodKind) float64 {
	if kind != FoodMeat || w.cfg.MeatNutrition <= 0 {
		return 1
	}
	return w.cfg.MeatNutrition
}

// itemHeal is the vitality this one item would put back into this body,
// before the ceiling of what it is missing.
//
// It asks about the item and not about its kind, because since stage 52 two
// things of the same kind can be worth different amounts: one of them has been
// cooked and the other has not. What share of a body it mends is healShare
// (cook.go), which takes the better of what it is and what was done to it.
//
// It carries the same two discounts the hunger side carries, and for the same
// reasons: a body sick of meat gets less out of the next piece of it (stage
// 16), and a parent feeding children keeps only its share of the mouthful
// (provision.go). Writing it any other way would say that the sameness rule
// is about stomachs rather than about mouthfuls.
func (w *World) itemHeal(a *Agent, f *Food) float64 {
	share := w.healShare(f)
	if share <= 0 {
		return 0
	}
	return share * a.MaxVitality(&w.cfg) * w.dietValue(a, f.Kind)
}

// itemHealKnown is the same figure as the agent is told it. The carcass half
// can be hidden, which is stage 39's control arm for telling selection from
// choice; the cooking half never is, because a body can see what has been done
// to what it is holding.
func (w *World) itemHealKnown(a *Agent, f *Food) float64 {
	if !w.cfg.MeatHealKnown && f.Cooked <= 0 {
		return 0
	}
	heal := w.itemHeal(a, f)
	if !w.cfg.MeatHealKnown && f.Kind == FoodMeat && w.cfg.CookVitality > 0 {
		// Only what the cooking is worth, since the flesh is the part being
		// kept from it.
		raw := Food{Kind: FoodMeat}
		heal -= w.itemHeal(a, &raw)
		if heal < 0 {
			heal = 0
		}
	}
	if w.cfg.ParentFeedKnown {
		if kept := w.keptShare(a); kept != 1 {
			heal *= kept
		}
	}
	return heal
}

// mealHeals is what an uncooked item of each kind would mend, for Perception.
// The agent is told exactly what the world will do to it, the way stage 16
// tells it what a mouthful is worth: this is the ordinary case, and lifespan
// remains the one deliberate exception.
//
// What an item in front of this body is actually worth goes through
// itemHealKnown, because since stage 52 that depends on the item. This array
// is the figure for the kind, which is what a body knows about food in
// general rather than about one piece of it.
func (w *World) mealHeals(a *Agent) [NumFoodKinds]float64 {
	var out [NumFoodKinds]float64
	for k := range out {
		f := Food{Kind: FoodKind(k)}
		out[k] = w.itemHealKnown(a, &f)
	}
	return out
}

// mend puts a share of an item's healing into a body, up to what it is
// missing. Called for the eater with what it kept, and for each child with
// what it was given.
func (w *World) mend(a *Agent, f *Food, share float64) {
	heal := share * w.itemHeal(a, f)
	if heal <= 0 {
		return
	}
	room := a.MaxVitality(&w.cfg) - a.Vitality
	if room <= 0 {
		return
	}
	got := math.Min(heal, room)
	a.Vitality += got
	w.meatHealing += got
	// And it is a good thing to have happened (stage 54), in the body's own
	// units and at the line where the figure already exists.
	w.please(a, 0, got)
}
