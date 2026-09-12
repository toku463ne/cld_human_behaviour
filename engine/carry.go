package engine

import "math"

// Carrying: the first thing an agent owns (stage 40).
//
// Up to here, food lay in the world and eating it lowered hunger. There was no
// way to have food without eating it, which meant a body could only ever be as
// fed as the ground under it at that moment. Three quarters of the bodies that
// starve had something in sight within a planning horizon of dying (measured
// before this was built, #67): they did not die of a world with no food in it,
// they died of not having it when it mattered.
//
// What this is, and what it deliberately is not.
//
//   - It is the fifth state axis, and the first added since the world began.
//     Every other quantity that looked like state turned out to be bookkeeping
//     that fades on reading - what an agent has eaten lately, who has hit it,
//     what it thinks of the ground. An inventory cannot be written that way:
//     it holds the world's own items, and picking up, eating and dying are
//     reversible movements of a thing rather than a number decaying (#65).
//   - It is not a tenth gene. The rooms of stage 12c cost budget even when
//     empty, and adding another draw on the same budget would move all nine
//     shares at once and make every figure recorded since stage 7c a different
//     measurement. Capacity comes out of the vitality gene: buying a body that
//     holds more vitality buys a body that carries more, which is paid for out
//     of the budget already (#65).
//   - The price is weight against that same gene (#66). Power is combat
//     efficiency and nothing else - giving it a second job would make the two
//     selection pressures on it impossible to tell apart - and vitality is a
//     quantity rather than a kind of judgement, which is why swimming's
//     ceiling is hung on it too.
//   - The weight goes into the cost of moving, not into the speed. Effort
//     already moves speed; putting a second thing there would make the two
//     impossible to separate in a measurement, which is the reason stage 20
//     put the ground's figure on the cost as well.

// carryCapacity is how many items this body can hold at once, as a continuous
// figure. The integer ceiling is what a body may pick up; the continuous one
// is what the burden is measured against, so a strong body carrying its third
// item is less burdened than a weak one carrying its third.
func (a *Agent) carryCapacity(cfg *Config) float64 {
	if cfg.CarryCapacity <= 0 {
		return 0
	}
	return cfg.CarryCapacity * a.capacity(GeneVitality, cfg) / midAbility
}

// CarriedCount is how many items this agent is holding. For the viewer.
func (a *Agent) CarriedCount() int { return len(a.carried) }

// Carrying is what this agent is holding, for a viewer. The copy is the
// caller's: what is in a hand is the world's, not something to be reached into
// from outside it.
func (a *Agent) Carrying() []Food { return append([]Food(nil), a.carried...) }

// carryLoad is how full this body is, from 0 to 1. It is measured against the
// continuous capacity rather than against what the body may actually pick up,
// which is what makes the same item a lighter burden on a bigger body and a
// heavy one on a small one.
func (a *Agent) carryLoad(cfg *Config) float64 {
	cap := a.carryCapacity(cfg)
	if cap <= 0 {
		return 0
	}
	// Money weighs nothing (#66, stage 51). It still takes a hand - a body
	// holding a coin is a body not holding its dinner - but it is not part of
	// what the legs are charged for, and in a world where the only cost of
	// holding something is its weight, that is the whole of what makes money
	// worth taking at all.
	n := 0
	for i := range a.carried {
		if a.carried[i].Kind != FoodCoin {
			n++
		}
	}
	return clamp(float64(n)/cap, 0, 1)
}

// burden is the multiplier a load puts on the cost of moving. One for a body
// with empty hands, which is every body in a world with the rule off.
func (a *Agent) burden(cfg *Config) float64 {
	// Nil for the callers that ask what a piece of ground costs rather than
	// what it costs somebody: the ground has no hands.
	if a == nil || len(a.carried) == 0 || cfg.CarryCost <= 0 {
		return 1
	}
	return 1 + cfg.CarryCost*a.carryLoad(cfg)
}

// canCarryMore reports whether there is room for one more.
//
// Anything with hands at all can hold one thing. What the gene buys is what it
// costs to carry it and how much more can be carried beyond it, both of which
// are continuous - if holding anything at all were a threshold on a gene, a
// body a point short of it would be unable to pick up a berry, which is the
// discrete-and-nonlinear trap this project has already walked into twice (the
// strategy depth gate, and the proposal to make sight a discrete gene).
func (a *Agent) canCarryMore(cfg *Config) bool {
	return len(a.carried) < a.carrySlots(cfg)
}

// carrySlots is how many items this body may actually hold: at least one for
// anything with hands at all, and more as the gene allows. It is the whole of
// what "how much can it carry" means, and the surplus rule of stage 41 asks
// the same question of a hunting party, so both go through here.
func (a *Agent) carrySlots(cfg *Config) int {
	if cfg.CarryCapacity <= 0 {
		return 0
	}
	return max(1, int(a.carryCapacity(cfg)))
}

// canCarry says whether this agent may pick this item up.
//
// For food it is the same question as whether it may eat it: the claim on a
// carcass and the rule that nobody eats its own kind are the same rules here
// as at the mouth, so carrying opens no way round either. For a stone (stage
// 45) there is no such question - it is not food, nobody's kill and nobody's
// kind - so anything may pick one up.
func (w *World) canCarry(a *Agent, f *Food) bool {
	if f.Kind == FoodStone || f.Kind == FoodCoin {
		return true
	}
	return w.canEat(a, f)
}

// take moves an item out of the world and into a pair of hands.
func (w *World) take(a *Agent, foodID int) {
	f := w.foodByID(foodID)
	if f == nil || !w.canCarry(a, f) || !a.canCarryMore(&w.cfg) {
		a.requestDecision(TriggerTargetLost)
		return
	}
	w.tookFromStore(a, f) // out of a cache, if that is where it was (stage 50)
	held := *f
	held.Claim = nil // in hand it is nobody else's business whose kill it was
	held.ClaimUntil = 0
	held.Store = 0 // and out of it for good: a hand is not the place
	// A fish out of the water is dead, and dead flesh goes off (stage 49's
	// aftermath). Landing one is the moment its clock starts: a fish the
	// world planted is alive and keeps for ever, and there is no other way
	// out of the river than a mouth or a hand. From here it is a carcass in
	// every respect - it goes off in the hand, it goes off if the hand that
	// held it dies and drops it on the bank, and it goes off at a carcass's
	// rate rather than at one of its own.
	if held.Kind == FoodFish && held.SpoilAt == 0 &&
		w.cfg.MeatSpoilTicks > 0 && !w.cfg.LandedFishKeeps {
		held.SpoilAt = w.tick + w.cfg.MeatSpoilTicks
	}
	a.carried = append(a.carried, held)
	w.heldKind[held.Kind]++
	w.removeFoodByID(foodID)
	w.taken++
	a.requestDecision(TriggerGoalReached)
}

// dropCarried puts everything a body is holding back into the world where it
// fell. Death is the only place this happens: what an agent is carrying is
// food the world still has, and a body that vanished with it would be a leak
// in a total that stage 15a has kept fixed ever since regions arrived.
func (w *World) dropCarried(a *Agent) {
	for i := range a.carried {
		f := a.carried[i]
		f.ID = 0
		f.X, f.Y = a.X+w.randRange(-4, 4), a.Y+w.randRange(-4, 4)
		w.heldKind[f.Kind]--
		if w.countKind(f.Kind) >= w.kindAllowance(f.Kind) {
			continue // the world has no room for it; it is gone
		}
		w.putFood(f)
	}
	a.carried = a.carried[:0]
}

// eatCarried is eating something out of one's own hands. It is the same meal
// as any other - the sameness ledger, the poison, the mending and the share
// the children get are all the ordinary rules - and the only difference is
// that there is no item in the world to remove afterwards.
func (w *World) eatCarried(a *Agent, foodID int) {
	i := a.carriedIndex(foodID)
	if i < 0 || !w.canEat(a, &a.carried[i]) {
		a.requestDecision(TriggerTargetLost)
		return
	}
	f := a.carried[i]
	kept := w.share(a, &f)
	hungerBefore := a.Hunger
	a.Hunger = math.Max(0, a.Hunger-kept*w.cfg.FoodNutrition*w.dietValue(a, f.Kind)*w.meatWorth(f.Kind))
	if w.cfg.PlantDefence && f.Kind == FoodPlant {
		dose := kept * f.Genes.Poison * w.cfg.PoisonDamage * (1 - w.poisonResist(a))
		a.Vitality -= dose
		w.poisonLoss += dose
		if w.cfg.PlantPoisonSaves > 0 &&
			w.rng.Float64() < f.Genes.Poison*w.cfg.PlantPoisonSaves {
			// Spat out - and still in hand, exactly as a plant that is spat
			// out is still standing.
			a.Hunger = hungerBefore
			w.plantsSpat++
			return
		}
	}
	w.please(a, hungerBefore-a.Hunger, 0) // stage 54
	w.mend(a, &f, kept)
	if f.Cooked > 0 {
		w.cookedEaten++
	}
	if f.Kind == FoodFish {
		w.fishEaten++
	}
	if f.Kind == FoodMeat {
		w.meatEaten++
		// And whether the eater was one of those who brought it down, or
		// somebody who came upon what was left (stage 41).
		if f.heldBy(a.ID) {
			w.meatEatenHeld++
		} else {
			w.meatEatenFree++
		}
	} else {
		w.plantsEaten++
	}
	w.noteEaten(a, f.Kind)
	w.removeCarried(a, i)
	a.requestDecision(TriggerGoalReached)
}

// carriedIndex finds a held item by the id it had in the world.
func (a *Agent) carriedIndex(id int) int {
	for i := range a.carried {
		if a.carried[i].ID == id {
			return i
		}
	}
	return -1
}

func (w *World) removeCarried(a *Agent, i int) {
	w.heldKind[a.carried[i].Kind]--
	a.carried = append(a.carried[:i], a.carried[i+1:]...)
}

// spoilCarried takes away what has gone off in somebody's hands.
//
// The clock does not stop for being carried. Stopping it would be a benefit
// with no price, of exactly the kind stage 17b showed runs to the ceiling, and
// measured on the ground beforehand it buys almost nothing anyway: meat that
// never rots at all leaves the population where it was (#77).
func (w *World) spoilCarried(a *Agent) {
	for i := 0; i < len(a.carried); {
		if f := &a.carried[i]; f.SpoilAt > 0 && w.tick >= f.SpoilAt && !w.cfg.CarriedMeatKeeps {
			switch f.Kind {
			case FoodMeat:
				w.meatSpoiled++
			case FoodFish:
				w.fishSpoiled++
			}
			w.removeCarried(a, i)
			continue
		}
		i++
	}
}

// carriedViews is what a body is holding, in the same shape as the food it can
// see. No distance, nobody racing it for them: they are already in its hands,
// and the utility formula scores them exactly as it scores anything else.
func (w *World) carriedViews(a *Agent, out []FoodView) []FoodView {
	for i := range a.carried {
		f := &a.carried[i]
		// What it cannot eat is not a meal in its hand either.
		//
		// Nothing exercised this until stage 51, and then it turned out to
		// matter twice over. Money went first: 59 coins out of 60 were eaten
		// in eight thousand ticks. But counting what else was being held that
		// could not be eaten turned up two rules of this world being got
		// round through the hand - an enemy holding a plant (17,066 body-
		// ticks in five thousand), and a human holding human meat - both of
		// them handed over as gifts, which asked whether the receiver had a
		// hand free and not whether it could eat the thing. At the mouth the
		// world has always refused both.
		if !w.canEat(a, f) {
			continue
		}
		out = append(out, FoodView{
			ID:        f.ID,
			X:         a.X,
			Y:         a.Y,
			Dist:      0,
			Kind:      f.Kind,
			Held:      true,
			Nutrition: w.mealValues(a)[f.Kind],
			Catch:     1, // in the hand already: there is nothing left to land
			Heal:      w.itemHealKnown(a, f),
			Cooked:    f.Cooked,
			Danger:    w.dangerOf(a, f),
			RivalDist: math.Inf(1),
		})
	}
	return out
}

// CarryUse is what the population is holding. Read only.
type CarryUse struct {
	// Held is the mean number of items in a living body's hands, Holders the
	// share of bodies holding anything, and Load the mean fullness of the
	// hands there are - which is what the burden is charged on.
	Held    float64
	Holders float64
	Load    float64
}

// Carrying reports what the population is holding.
func (w *World) Carrying() CarryUse {
	var out CarryUse
	n := 0.0
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		n++
		out.Held += float64(len(a.carried))
		out.Load += a.carryLoad(&w.cfg)
		if len(a.carried) > 0 {
			out.Holders++
		}
	}
	if n == 0 {
		return CarryUse{}
	}
	out.Held /= n
	out.Holders /= n
	out.Load /= n
	return out
}
