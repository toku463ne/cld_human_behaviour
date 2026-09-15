package engine

import "math"

// Trinkets (stage 82).
//
// Everything this world wants, it wants for a reason. A meal is survival, a
// stone is a throw that has not happened, a coin is a claim on a meal and
// nothing else (#73), a book is what it says. That is a good rule and it is
// why money never became a third goal - and it is also why, after seven stages
// of plumbing, there is still almost no trade: what a body values, it values
// because it needs it, and a body that needs something does not sell it.
//
// So this is the one want with nothing behind it (#109, #110). A trinket feeds
// nobody, mends nobody, kills nobody and tells nobody anything. It is worth a
// figure the world is given rather than one it works out, the way a real
// ornament is: modelling every reason somebody wants a pretty thing is not a
// simplification, it is a different project.
//
// The position of this stage is written down before any of it runs, because
// the number that will move is not the number that matters (#110). This does
// not solve the food economy's problem - self-sufficiency - and a rise in
// sales here must not be read as having solved it. What it does is try the
// price machinery out of that headwind: if prices and haggling work on a want
// with no survival value, the same machinery is there when the food side is
// solved, and if they do not work even here, they were never going to.
//
// Four things are built rather than assumed, and each one closes a door this
// project has already walked into:
//
//   - It comes out of a body's hands, not out of the ground. Scattering them
//     would inherit the distribution problem food has, and walking somewhere
//     solves that: six measurements say food is the only thing that moves
//     anybody, and a shortage that moving fixes is not a shortage.
//   - Making one costs vitality and time, and is chosen like anything else.
//     A thing that arrives by simply having the skill is stage 17b's shape,
//     where a benefit with no price runs to the ceiling. The price also means
//     the maker cannot be generous with them, so the supply side is short too.
//   - What one is worth varies from piece to piece. Nothing else in this
//     world does: every value is per kind, so two bodies never disagree about
//     a particular object, and a price with nothing to disagree about is the
//     one price stage 80 measured. This is what could make a price scatter.
//   - It goes off, on the same clock meat does (#77 - a hand does not stop
//     it). Nothing new times it, and it means holding one for ever is not
//     free: there is always a reason to use it or pass it on.
//
// Stage 84 opened the one of those four this stage had deliberately shut. The
// third says what a piece is worth varies, and it went on to say that what it
// is worth does not depend on who is holding it - so that a scattered price
// could only have come from the pieces. That measurement is done (it does not
// scatter either way), and the taste is the thing this world has never had:
// two bodies that price the same object differently, which is the only way
// past the arithmetic stage 79 wrote down. The second thing 84 added is the
// difference between this and a meal, which is that you have to be there for
// it (adornWant). Both live in trinketWorth, below.

// trinketQuality is how good a thing this body would make, from 0 to 1.
//
// Anybody may try: the base is what a body that knows nothing manages, and the
// skill buys the part an ignorant body wastes - the same shape cooking uses,
// and for the same reason (no discrete gate on a gene).
func (w *World) trinketQuality(a *Agent) float64 {
	q := 1 - w.cfg.SkillTrinketRelief +
		w.cfg.SkillTrinketRelief*a.skillAt(&w.cfg, SkillTrinket)
	return clamp(q, 0, 1)
}

// canCraft says whether this body could make one: a world that has them, and
// somewhere to put it.
func (w *World) canCraft(a *Agent) bool {
	if !w.cfg.Trinkets || w.cfg.CraftTicks <= 0 || w.cfg.TrinketValue <= 0 {
		return false
	}
	return a.canCarryKind(&w.cfg, FoodTrinket)
}

// craft runs a making for its ticks and then puts the thing in the hand, in
// the shape the cry and the cooking already use: no new parallel machinery for
// a multi-tick action (#76).
func (w *World) craft(a *Agent) {
	if !w.canCraft(a) {
		a.requestDecision(TriggerTargetLost) // hands filled while it worked
		return
	}
	if a.actionTicks < w.cfg.CraftTicks {
		return
	}
	// What it cost, charged once at the end rather than spread over the ticks:
	// the option was scored with this same figure, and a body that is
	// interrupted has spent the time and not the vitality.
	a.Vitality -= w.cfg.CraftVitality
	made := w.trinketQuality(a)
	if w.cfg.TrinketSpread > 0 {
		// And what this particular one came out like. The maker's hand sets
		// the middle of it; the piece is still a piece.
		made *= 1 + w.cfg.TrinketSpread*(2*w.rng.Float64()-1)
	}
	w.trinketsMade++
	w.trinketWorthMade += made
	if w.cfg.TrinketsVanish {
		// The control: the time and the vitality are spent and there is
		// nothing in the hand at the end of it.
		a.requestDecision(TriggerGoalReached)
		return
	}
	item := Food{
		X: a.X, Y: a.Y, Kind: FoodTrinket, Made: clamp(made, 0, 2),
		Style: w.drawStyle(a),
	}
	item.Ward, item.Wards = w.wardMade()
	w.trinketFitMade += w.trinketDelight(a, &item)
	w.trinketFitN++
	if w.cfg.TrinketSpoilTicks > 0 {
		item.SpoilAt = w.tick + w.cfg.TrinketSpoilTicks
	}
	if item.Ward > 0 {
		w.coatsMade++
	}
	a.carried = append(a.carried, item)
	w.heldKind[FoodTrinket]++
	a.requestDecision(TriggerGoalReached)
}

// trinketWorth is what having this particular one is worth to this body.
//
// It is the given figure times what this piece came out like, less what is
// left of it: a thing an hour from being gone is worth an hour of it. The
// discount is continuous rather than a cliff so that letting one go while it
// is still worth something is an ordinary comparison and not a deadline.
//
// Two things were added in stage 84, and both of them take the same place
// here rather than at any of the half dozen sites that ask what an ornament
// is worth - buying one, picking one up, selling one, giving one away,
// putting one down. Stage 77 is why: when the same question is answered in
// two places, the two answers drift.
//
//   - Who is looking (trinketDelight). Until this, no value in this world
//     depended on that.
//   - Whether this body expects to be there for it (adornWant), which is the
//     whole difference between an ornament and a meal.
//
// A nil body is one nobody is holding: a piece lying on the ground is worth
// what it is worth before anybody in particular has looked at it.
func (w *World) trinketWorth(a *Agent, f *Food) float64 {
	if !w.cfg.Trinkets || f.Kind != FoodTrinket {
		return 0
	}
	worth := w.cfg.TrinketValue * w.cfg.LifeValue * f.Made
	if f.SpoilAt > 0 && w.cfg.TrinketSpoilTicks > 0 {
		left := float64(f.SpoilAt-w.tick) / float64(w.cfg.TrinketSpoilTicks)
		worth *= clamp(left, 0, 1)
	}
	if a == nil {
		return worth
	}
	return worth * w.trinketDelight(a, f) * w.adornWantOf(a)
}

// trinketDelight is how much this body wants this particular piece rather
// than some other one (stage 84).
//
// Circular, because a style is a point on a circle and not a rank: the
// distance from 0.95 to 0.05 is a tenth and not nine tenths. That is the
// chronotype's shape (clock.go) and it is here for the chronotype's reason -
// a taste is a direction, so nothing about it can be bought.
//
// It averages one over a piece drawn at random, so turning the taste up moves
// want about between bodies without putting any more of it into the world.
func (w *World) trinketDelight(a *Agent, f *Food) float64 {
	t := clamp(w.cfg.TrinketTaste, 0, 1)
	if t <= 0 {
		return 1
	}
	d := math.Abs(f.Style - a.taste)
	if d > 0.5 {
		d = 1 - d
	}
	return 1 + t*(1-4*d)
}

// craftDelight is how much a body expects to want the piece it is about to
// make. It cannot know how this one will come out - which is why a maker
// cannot simply make itself what it wants - so it reckons on the average,
// which is one. Where a maker does turn out its own style, it knows that too.
func (w *World) craftDelight(a *Agent) float64 {
	t := clamp(w.cfg.TrinketTaste, 0, 1)
	if t <= 0 || !w.cfg.TrinketStyleAimed {
		return 1
	}
	return 1 + t
}

// drawStyle is which ornament this one turned out to be. It draws nothing
// where no body has a taste, so a world without one is the world stage 82
// measured, to the bit.
func (w *World) drawStyle(a *Agent) float64 {
	if w.cfg.TrinketTaste <= 0 {
		return 0
	}
	if w.cfg.TrinketStyleAimed {
		return a.taste
	}
	return w.rng.Float64()
}

// drawTaste is the ornament a founder likes, and inheritTaste the one a child
// does: one parent's, whole, with a drift, wrapped round the circle. Both are
// the chronotype's, for the chronotype's reasons.
func (w *World) drawTaste() float64 {
	if !w.cfg.Trinkets || w.cfg.TrinketTaste <= 0 {
		return 0
	}
	return w.rng.Float64()
}

func (w *World) inheritTaste(pa, pb *Agent) float64 {
	if !w.cfg.Trinkets || w.cfg.TrinketTaste <= 0 {
		return 0
	}
	t := pa.taste
	if w.rng.Float64() < 0.5 {
		t = pb.taste
	}
	if w.cfg.TrinketTasteMutation > 0 {
		t += w.rng.NormFloat64() * w.cfg.TrinketTasteMutation
	}
	return t - math.Floor(t)
}

// wantAdornment writes, once a tick and before anybody decides anything with
// it, how much of an ornament's worth is left to each body (stage 84).
//
// This is the one thing that tells a meal and an ornament apart in a decision.
// Staying alive is priced as a difference between two chances of dying, so it
// grows as a body runs out; a want with nothing behind it was priced as a
// constant, and a constant beats a shrinking figure exactly when it should
// lose. Stage 73 found the same fault in the price of a child and fixed it
// with survives(), and this is that function, on the goal that needs it most:
// an ornament is the purest case of something you have to be alive to enjoy.
//
// It is worked out here rather than where it is used because the body's own
// view of itself is expensive to build and the answer is the same all tick,
// and it is kept on the agent for the reason Nursing is: three sites read it
// and they must read the same number.
// It shares its pass with the other thing a body has to work out about its
// own hands before it can decide anything (stage 84's spare item): both want
// the body's own view of itself, which is the expensive part.
func (w *World) priceHands() {
	adorn := w.cfg.Trinkets && w.cfg.AdornNeedsSurvival
	spare := w.cfg.HandOverCheapest
	if !adorn && !spare {
		return // never written, never read
	}
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		s := w.selfView(a)
		if adorn {
			// Before the spare item is worked out: what an ornament is worth
			// to this body is part of what it would be giving up.
			risk := pressures(&w.cfg, &s, a.Vitality, a.Hunger, 0).far
			a.adornWant = survives(&w.cfg, risk, w.cfg.PlanHorizon)
			s.AdornWant = a.adornWant
		}
		if spare {
			a.spare = w.spareIndex(a, &s, false)
			a.spareSale = w.spareIndex(a, &s, true)
		}
	}
}

// adornWantOf is that figure, and one where the rule is off.
func (w *World) adornWantOf(a *Agent) float64 {
	if !w.cfg.AdornNeedsSurvival {
		return 1
	}
	return a.adornWant
}

// TrinketUse is what the trinkets came to. Read only.
type TrinketUse struct {
	// Made is how many were made and Quality what the average one came out
	// like: the supply side, which is the half a want cannot conjure.
	Made    int
	Quality float64

	// Held is how many are in hands, Holders the share of living bodies
	// holding one, and Best the average worth of the ones held - if the last
	// is above the average made, the good ones are being kept and the poor
	// ones passed on, which is the sorting stage 82f expects to fall out of
	// the pricing without a rule for it.
	Held    int
	Holders float64
	Best    float64

	// Fit is how well the pieces in hands suit the bodies holding them, and
	// FitMade how well the ones coming off the bench suited the hand that
	// made it (stage 84). Both average one where nobody has a taste.
	//
	// The two together are the whole of the stage's claim, and neither can be
	// read without the other: a maker cannot aim, so what comes out fits
	// nobody in particular, and anything above that in the hands is a piece
	// that reached somebody who wanted it. How it got there - bought, given,
	// picked up off the ground, or simply kept when the poor ones were let go
	// - the figures below say.
	Fit     float64
	FitMade float64

	// Sold and Given are how many changed hands each way, and Want the mean
	// share of an ornament's worth the living still expect to be there for.
	Sold  int
	Given int
	Want  float64

	// Gained is what a hand-over did to how well the piece suited whoever was
	// holding it: the new holder's liking for it less the old one's, over
	// every ornament that changed hands (stage 84).
	//
	// It is the stage's claim in one figure, and it is counted as it happens
	// rather than read off the hands at the end: a hand holds what the last
	// twenty thousand ticks left in it, and there are a few dozen hands.
	// Above nought says pieces are moving towards the bodies that want them,
	// which is what a market is for and what no rule anywhere asks for.
	// Gained is over every hand-over, and GainedSold over the sales alone.
	// The two apart are what tells a rule from a market: where a body hands
	// over the piece it minds least, the giving side is bound to come out
	// above nought because the giver was chosen for disliking it, and only
	// the receiving end can say whether anybody wanted it. Nothing chooses
	// the receiver of a gift; a buyer chooses itself.
	Gained     float64
	GainedSold float64
}

// noteTrinketMove records one ornament changing hands (stage 84). It rides on
// the two lines that already count a hand-over, so nothing is detected anew.
func (w *World) noteTrinketMove(from, to *Agent, f *Food, sold bool) {
	if !w.cfg.Trinkets || f.Kind != FoodTrinket {
		return
	}
	if f.Ward > 0 {
		w.coatsHanded++
	}
	gain := w.trinketDelight(to, f) - w.trinketDelight(from, f)
	w.trinketMoves++
	w.trinketMoveGain += gain
	if sold {
		w.trinketSales++
		w.trinketSaleGain += gain
	}
}

// Trinkets reports what they came to.
func (w *World) Trinkets() TrinketUse {
	out := TrinketUse{
		Made:  w.trinketsMade,
		Sold:  w.trinketsSold,
		Given: w.trinketsGiven,
	}
	if w.trinketsMade > 0 {
		out.Quality = w.trinketWorthMade / float64(w.trinketsMade)
	}
	if w.trinketFitN > 0 {
		out.FitMade = w.trinketFitMade / float64(w.trinketFitN)
	}
	n, worth, fit := 0.0, 0.0, 0.0
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		n++
		out.Want += w.adornWantOf(a)
		held := 0
		for k := range a.carried {
			if a.carried[k].Kind == FoodTrinket {
				held++
				worth += a.carried[k].Made
				fit += w.trinketDelight(a, &a.carried[k])
			}
		}
		out.Held += held
		if held > 0 {
			out.Holders++
		}
	}
	if out.Held > 0 {
		out.Best = worth / float64(out.Held)
		out.Fit = fit / float64(out.Held)
	}
	if n > 0 {
		out.Holders /= n
		out.Want /= n
	}
	if w.trinketMoves > 0 {
		out.Gained = w.trinketMoveGain / float64(w.trinketMoves)
	}
	if w.trinketSales > 0 {
		out.GainedSold = w.trinketSaleGain / float64(w.trinketSales)
	}
	return out
}
