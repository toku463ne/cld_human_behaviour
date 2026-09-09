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

// mealHeal is the vitality one item of this kind would put back into this
// body, before the ceiling of what it is missing.
//
// It carries the same two discounts the hunger side carries, and for the same
// reasons: a body sick of meat gets less out of the next piece of it (stage
// 16), and a parent feeding children keeps only its share of the mouthful
// (provision.go). Writing it any other way would say that the sameness rule
// is about stomachs rather than about mouthfuls.
func (w *World) mealHeal(a *Agent, kind FoodKind) float64 {
	if kind != FoodMeat || w.cfg.MeatVitality <= 0 {
		return 0
	}
	return w.cfg.MeatVitality * a.MaxVitality(&w.cfg) * w.dietValue(a, kind)
}

// mealHeals is the same for every kind, for Perception. The agent is told
// exactly what the world will do to it, the way stage 16 tells it what a
// mouthful is worth: this is the ordinary case, and lifespan remains the one
// deliberate exception.
func (w *World) mealHeals(a *Agent) [NumFoodKinds]float64 {
	var out [NumFoodKinds]float64
	if w.cfg.MeatVitality <= 0 || !w.cfg.MeatHealKnown {
		return out
	}
	for k := range out {
		out[k] = w.mealHeal(a, FoodKind(k))
	}
	if w.cfg.ParentFeedKnown {
		if kept := w.keptShare(a); kept != 1 {
			for k := range out {
				out[k] *= kept
			}
		}
	}
	return out
}

// mend puts a share of an item's healing into a body, up to what it is
// missing. Called for the eater with what it kept, and for each child with
// what it was given.
func (w *World) mend(a *Agent, kind FoodKind, share float64) {
	heal := share * w.mealHeal(a, kind)
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
}
