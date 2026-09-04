package engine

// A small penalty for living on one thing (stage 16, decision #45).
//
// Eating the same kind over and over is worth progressively less: what an
// agent has had lately discounts what the next mouthful of it is worth, and
// the discount fades once it stops. Nothing else changes - hunger falls by
// less, and everything downstream of hunger follows on its own.
//
// Three things this deliberately is not.
//
//   - Not a new state axis. There are still three (vitality, hunger, maturity).
//     What an agent has been eating is bookkeeping of the same sort as who has
//     hit it lately, and it fades the same lazy way every other memory does.
//   - Not hidden. Lifespan is a silent tally an agent has no way to know about
//     (world.go), and that is a deliberate exception; this is the ordinary
//     case. It goes into Perception and into the utility formula like hunger
//     does, because an animal knows perfectly well when it is sick of
//     something.
//   - Not expected to do much yet, and that is the point of measuring it now.
//     A human's food is plants, near enough always; meat is what happens when
//     an enemy loses. A rule about varying your diet has almost nothing to
//     bite on until there is a choice to make, which is stage 17's job. This
//     is the baseline to compare that against.

// noteEaten records that this agent has just had one of something.
func (w *World) noteEaten(a *Agent, kind FoodKind) {
	if w.cfg.SamenessPenalty <= 0 {
		return
	}
	a.recentFood[kind] = w.recentlyEaten(a, kind) + 1
	a.dietTick = w.tick
}

// recentlyEaten is how much of this kind the agent has had lately, faded to
// now. Applied on reading rather than every tick, which is how every other
// fading quantity in the world works.
//
// The whole record shares one clock. It is only ever written all at once (an
// agent eats one thing at a time and the rest fade from the same moment), so a
// tick per kind would say nothing a single tick does not.
func (w *World) recentlyEaten(a *Agent, kind FoodKind) float64 {
	return decay(a.recentFood[kind], w.cfg.DietForgetPerTick, w.tick-a.dietTick)
}

// dietValue is what one of this kind is worth to this agent now, as a share of
// what it would be worth to somebody that has not been living on it.
//
// It never reaches zero. Being sick of something is a reason to look for
// something else, not a reason to starve next to food, and a discount that
// could reach zero would make an agent that has only one thing available
// refuse to eat at all.
func (w *World) dietValue(a *Agent, kind FoodKind) float64 {
	cfg := &w.cfg
	if cfg.SamenessPenalty <= 0 {
		return 1
	}
	had := w.recentlyEaten(a, kind)
	// Saturating rather than linear: the first few of a thing are the same as
	// each other and the twentieth is no worse than the tenth.
	share := had / (had + cfg.DietSatiety)
	return 1 - clamp(cfg.SamenessPenalty, 0, 1)*share
}

// dietValues is what every kind is worth to this agent now, for Perception.
func (w *World) dietValues(a *Agent) [NumFoodKinds]float64 {
	var out [NumFoodKinds]float64
	for k := range out {
		out[k] = w.dietValue(a, FoodKind(k))
	}
	return out
}

// --- reading it out ---------------------------------------------------------

// DietOf is what each kind of food is currently worth to one agent, for the
// viewer. Read only; the agent gets the same figures through Perception.
func (w *World) DietOf(id int) [NumFoodKinds]float64 {
	a := w.agentByID(id)
	if a == nil {
		var none [NumFoodKinds]float64
		for i := range none {
			none[i] = 1
		}
		return none
	}
	return w.dietValues(a)
}

// DietUse is what the population is living on.
type DietUse struct {
	// Variety is how mixed the average agent's recent diet is, from 0 (one
	// thing only) to 1 (an even share of everything there is). It is the
	// measurement the stage turns on: a rule about varying the diet can only
	// matter in so far as there is anything to vary it with.
	Variety float64

	// Discount is what the average mouthful is actually worth, as a share of
	// what it would be worth to an agent that had not been living on it. One
	// is a rule that never fires.
	Discount float64
}

// Diet reports what the living population has been eating. Read only.
func (w *World) Diet() DietUse {
	var out DietUse
	n := 0.0
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		n++

		var total, best float64
		for k := FoodKind(0); k < NumFoodKinds; k++ {
			had := w.recentlyEaten(a, k)
			total += had
			if had > best {
				best = had
			}
			out.Discount += w.dietValue(a, k)
		}
		// One minus the share of the commonest thing, scaled so that an even
		// split over all the kinds there are comes to one.
		if total > 0 && NumFoodKinds > 1 {
			out.Variety += (1 - best/total) * float64(NumFoodKinds) / float64(NumFoodKinds-1)
		}
	}
	if n == 0 {
		return DietUse{Discount: 1}
	}
	out.Variety /= n
	out.Discount /= n * float64(NumFoodKinds)
	return out
}
