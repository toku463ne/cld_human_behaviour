package engine

import "math"

// Cooking (stage 52).
//
// Stages 50 and 51 ran into the same wall from two directions. What a cache is
// worth and what a coin is worth are both read off the gradient of present
// death risk, that gradient is flat in a body that is not hungry, and the
// bodies holding a surplus are exactly the ones that are not hungry. So the
// store was scored 312 times in a run and won three, and the market refused
// twice for every sale. Nodes in this world only ever look at now (#79).
//
// This is the one way round that wall that needs no change to how a body
// plans, and the reason is the ceiling stage 39 already put on mending.
// Healing is not read off hunger: it is read off how much vitality the body is
// missing, capped at exactly that. So a body that is full and hurt wants a
// cooked meal, and a body that is full and hurt is a body that can be holding
// something. Counted before this was written, over eight runs of the default
// world and eight of the played one:
//
//	                                     flat    played
//	holding something to eat             0.255   0.389
//	...and keeping it is worth anything  0.037   0.063   <- stage 50's question
//	...and a cooked meal beats a raw one 0.235   0.355   <- this stage's
//
// Six times the target, and 92% of everybody holding anything, against 15%.
//
// What it is, and what it deliberately is not.
//
//   - It is not a fourth kind of food. Cooked is a figure on the item, the way
//     stage 44's awkward crop is a flag on it: a cooked plant feeds a body
//     exactly what a plant feeds a body, and what differs is what it mends.
//     A fourth FoodKind would move the scale dietVariety is measured on and
//     make every figure recorded since stage 42 a different measurement (#80).
//   - It is not a gate. Nothing becomes inedible raw. This world has turned
//     down the discrete gate three times now - the strategy depth unlock,
//     sight as a gene, and the awkward crop that anybody can pull up one time
//     in four - and a food that cannot be eaten raw is the shape stage 17b
//     walked into, where a rule with no way round it took the population to a
//     ninth of what it was. If the continuous version turns out too weak,
//     stage 52d adds a food that is merely poor raw, and measures it on its
//     own.
//   - It is not a new term in the utility formula and not a new state. The
//     mending goes exactly where stage 39's goes, and the ceiling of what the
//     body is missing does the interesting part by itself.
//   - Nothing keeps for being cooked (#77). The clock on a held item is the
//     same clock whatever has been done to it.
//
// And one thing that was in the plan and is not here: a claim on what has just
// been cooked (#81). The worry was free-riding - why pay time to cook if
// anybody may take it - and counting where cooking happens answered it before
// it was built. Cooking happens in the hand, and what is in a hand is nobody
// else's in this world: there is no word for taking something off somebody
// (that is still on the shelf, waiting on this very stage). So MeatClaimTicks
// has no analogue to be borrowed here, and the monopoly this stage has to
// watch for is not theft but hoarding - who ends up eating the cooked thing,
// which is measured below.

// cookQuality is how well this body would cook, from 0 to 1.
//
// Anybody can cook: the base is what a body that knows nothing manages, and
// it is one by default, so a world that has not been given skills has cooking
// that is simply cooking. Lowering it is the map-maker's to do, exactly as
// stage 43 leaves the two fishing chances at one until somebody sets them -
// and in a world where it is lowered, what the skill buys is the part an
// ignorant body wastes.
func (w *World) cookQuality(a *Agent) float64 {
	q := w.cfg.CookQuality + w.cfg.SkillCookRelief*a.skillAt(&w.cfg, SkillCook)
	return clamp(q, 0, 1)
}

// canCook says whether there is anything in this body's hands worth doing this
// to. The first item is the one every other hand-rule works on (giving,
// crying, putting away), and cooking is no different.
func (w *World) canCook(a *Agent) bool {
	if w.cfg.CookTicks <= 0 || w.cfg.CookVitality <= 0 || len(a.carried) == 0 {
		return false
	}
	f := &a.carried[0]
	if f.Kind == FoodStone || f.Kind == FoodCoin {
		return false // nothing to be done with either over a fire
	}
	// Already done, or done better than this body could manage: there is
	// nothing to be had from starting again. This is the same comparison
	// learning a skill makes - take it if it is better than what is there -
	// and it is what stops a body cooking the same item for ever.
	return w.cookQuality(a) > f.Cooked
}

// cook runs a cooking for its ticks and then makes the thing. Standing there
// is the whole of the price, in the shape stage 49's cry already uses: no new
// parallel machinery for a multi-tick action (#76).
func (w *World) cook(a *Agent) {
	if !w.canCook(a) {
		a.requestDecision(TriggerTargetLost) // eaten, given away or gone off
		return
	}
	if a.actionTicks < w.cfg.CookTicks {
		return
	}
	f := &a.carried[0]
	f.Cooked = w.cookQuality(a)
	w.cooked++
	if f.Kind == FoodMeat {
		w.cookedMeat++
	}
	a.requestDecision(TriggerGoalReached)
}

// healShare is what share of a body one item mends, before the ceiling of what
// that body is missing.
//
// Two rules meet here and they do not stack. A carcass mends because it is
// flesh (stage 39); anything mends because somebody prepared it (stage 52);
// and what an item is worth is the better of the two, not the sum. Adding them
// would put the total mending in this world two rules deep, so that a change
// to either could not be read on its own - and it would make cooked meat the
// one food worth twice what everything else is, which is a thumb on the scale
// of exactly the question this stage asks.
//
// It also means cooking a carcass buys nothing while the two figures are
// equal, so what gets cooked is what everybody has rather than what only a
// hunting party has. That is the food an exchange can be built on, and it
// falls out of refusing to stack rather than out of a rule about meat.
func (w *World) healShare(f *Food) float64 {
	s := 0.0
	if f.Kind == FoodMeat && w.cfg.MeatVitality > 0 {
		s = w.cfg.MeatVitality
	}
	if f.Cooked > 0 && w.cfg.CookVitality > 0 {
		if c := w.cfg.CookVitality * f.Cooked; c > s {
			s = c
		}
	}
	return s
}

// CookUse is what the cooking came to. Read only.
type CookUse struct {
	// Cooked is how many times anything was prepared, and Meat how much of
	// that was a carcass - which the design expects to be almost none.
	Cooked int
	Meat   float64

	// Standing is the share of living bodies holding something cooked, Eaten
	// how many cooked meals were eaten for every cooking, and Handed how many
	// hand-overs of cooked food there were for every cooking. The last is the
	// monopoly question this stage has to answer - cooking that never leaves
	// the cook is cooking no exchange can be built on - and it is a rate
	// rather than a share on purpose: one cooked plant can pass through
	// several pairs of hands, and on a hungry map it does.
	Standing float64
	Eaten    float64
	Handed   float64

	// Split is how far apart being good at cooking and being good at
	// foraging are - the division of labour, measured as the correlation
	// between the two masteries across the population. Negative is bodies
	// specialising; zero is everybody equally good at both or at neither.
	Split float64
}

// Cooking reports what the cooking came to.
func (w *World) Cooking() CookUse {
	out := CookUse{Cooked: w.cooked}
	if w.cooked > 0 {
		out.Meat = float64(w.cookedMeat) / float64(w.cooked)
		out.Eaten = float64(w.cookedEaten) / float64(w.cooked)
		out.Handed = float64(w.cookedHanded) / float64(w.cooked)
	}
	var n, standing float64
	var sx, sy, sxx, syy, sxy float64
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		n++
		for k := range a.carried {
			if a.carried[k].Cooked > 0 {
				standing++
				break
			}
		}
		x, y := a.nominalSkill(SkillCook), a.nominalSkill(SkillForage)
		sx, sy = sx+x, sy+y
		sxx, syy, sxy = sxx+x*x, syy+y*y, sxy+x*y
	}
	if n == 0 {
		return out
	}
	out.Standing = standing / n
	vx, vy := sxx-sx*sx/n, syy-sy*sy/n
	if vx > 1e-9 && vy > 1e-9 {
		out.Split = (sxy - sx*sy/n) / math.Sqrt(vx*vy)
	}
	return out
}
