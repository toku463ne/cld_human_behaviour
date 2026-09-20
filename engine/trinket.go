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
	if w.cfg.WardNeedsHide && a.holdsHide() {
		// The material leaves the hand as the piece goes into it (TODO 8),
		// so there is room by construction. Asked the ordinary way, a body
		// with one hand and a skin in it could never work the skin - a
		// deadlock, and one that only shows up in the worlds where a hand
		// costs something.
		return true
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
	item.Ward, item.Wards = w.wardMadeBy(a)
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

// heldTrinkets is how many ornaments are in this body's hands.
func heldTrinkets(a *Agent) int {
	n := 0
	for i := range a.carried {
		if a.carried[i].Kind == FoodTrinket {
			n++
		}
	}
	return n
}

// noteAdorned moves the ledger of how adorned this body has been lately one
// tick on (TODO 12, stage 89).
//
// Written once a tick per body rather than lazily on reading, which is the
// one place this differs from the diet's ledger it is otherwise copied from
// (stage 16). The diet's is written when something is eaten and faded when it
// is read, because eating is an event; being adorned is not an event, it is a
// state of the hands, and a ledger of a state has to be walked along with it.
// It rides on the pass that already looks at every body's hands once a tick
// (priceHands), so it costs a multiply and an add.
//
// It moves towards what is in the hand rather than jumping to it, and that
// lag is the whole of the rule: a body that has just lost its ornament is
// still recently adorned and does not want another this minute, and one that
// has carried three for a year is thoroughly sated. Without the lag this
// would be a second way of counting the hand.
func (w *World) noteAdorned(a *Agent, n int) {
	rate := clamp(w.cfg.AdornForgetPerTick, 0, 1)
	if rate <= 0 {
		return
	}
	held := float64(n)
	if w.cfg.AdornKeepsSated {
		// The far end of the rule, for the control arm: satisfied once and
		// never again, so the want never comes back and the demand never
		// repeats.
		if held > a.recentAdorn {
			a.recentAdorn = held
		}
		return
	}
	a.recentAdorn += rate * (held - a.recentAdorn)
}

// adornSpare is how much of the want for an ornament this body has left.
//
// One in a world without the rule, and in a body with empty hands. Saturating
// rather than linear, for the diet's reason: the first few of a thing are
// much the same as each other, and the twentieth is no worse than the tenth.
// It never quite reaches nought, which matters less here than it does for
// food but keeps the same shape - a sated body still prefers a fine piece to
// a poor one, it simply does not prefer it to dinner.
func (w *World) adornSpare(a *Agent) float64 {
	if w.cfg.AdornSatiety <= 0 || a == nil {
		return 1
	}
	return w.cfg.AdornSatiety / (w.cfg.AdornSatiety + math.Max(a.recentAdorn, 0))
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
	// ... and how much want this body has left (TODO 12). Here rather than
	// at any of the half dozen sites that ask what an ornament is worth, for
	// the reason the taste and the survival gate are here: when the same
	// question is answered in two places, the two answers drift.
	return worth * w.trinketDelight(a, f) * w.adornWantOf(a) * w.adornSpare(a)
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
	d := math.Abs(f.Style - w.tasteOf(a))
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
		return w.tasteOf(a)
	}
	return w.rng.Float64()
}

// tasteOf is what this body likes, which is what it was born with in every
// world before stage 90 and what it happens to like just now in a world that
// has the fancy switched on.
//
// One accessor rather than a field read, because three things ask (what a
// piece is worth to it, what it expects of the piece it is about to make,
// and what a maker who can aim turns out) and all three have to mean the
// same "likes".
func (w *World) tasteOf(a *Agent) float64 {
	if w.cfg.TrinketFancyTicks > 0 {
		return a.fancy
	}
	return a.taste
}

// drawFancy is a new "what I like just now": the inherited taste, strayed
// from by TrinketFancySpread and wrapped round the circle.
//
// Drawn from the taste rather than freely, so that heredity still means
// something - a body's fancies wander around what it is like, and a spread
// wide enough makes the two independent, which is the dose at one end.
func (w *World) drawFancy(a *Agent) float64 {
	if w.cfg.TrinketFancyTicks <= 0 {
		return a.taste
	}
	f := a.taste
	if w.cfg.TrinketFancySpread > 0 {
		f += w.rng.NormFloat64() * w.cfg.TrinketFancySpread
	}
	return f - math.Floor(f)
}

// refreshFancy draws a new one when something has happened or enough time
// has passed (stage 90).
//
// "Something has happened" is the number of ornaments in the hand changing,
// which covers making one, being given one, buying one, picking one up,
// selling one, handing one over and having one go off - seven sites, none of
// which has to know about this. The alternative was a hook in each, and the
// last time this project put the same fact in several places the two answers
// drifted (stage 77).
func (w *World) refreshFancy(a *Agent, held int) {
	if w.cfg.TrinketFancyTicks <= 0 {
		return
	}
	if held != a.adornHeld || w.tick-a.fancyTick >= w.cfg.TrinketFancyTicks {
		a.fancy = w.drawFancy(a)
		a.fancyTick = w.tick
		w.fancyDraws++
	}
	a.adornHeld = held
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
	sated := w.cfg.Trinkets && w.cfg.AdornSatiety > 0
	fancy := w.cfg.Trinkets && w.cfg.TrinketFancyTicks > 0
	if !adorn && !spare && !sated && !fancy {
		return // never written, never read
	}
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		// Before the views below are built, because they read both (TODO
		// 12). The hand is counted once and the two rules share the count.
		if sated || fancy {
			held := heldTrinkets(a)
			if sated {
				w.noteAdorned(a, held)
			}
			if fancy {
				w.refreshFancy(a, held)
			}
		}
		if !adorn && !spare {
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

	// Fancies is how many times a body has drawn a new "what I like just
	// now" (stage 90), over the whole run. Read against the interval
	// between decisions (14.5 ticks): a fancy redrawn faster than a body
	// can carry anything out is the oscillation of 2026-09-04, and this is
	// the column that says whether an arm is in it.
	Fancies int

	// Spare is the mean share of the want that is left once what a body is
	// already carrying is taken off (TODO 12, stage 89): one where nobody is
	// sated, and the rule's own firing rate everywhere else. It is the first
	// figure to read of that stage - a rule that leaves it at one has not
	// fired, whatever else moved.
	Spare float64

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
		// And where the receiver was standing, in the weather this piece
		// answers (2026-09-20). It is the question the whole of stage 87 was
		// built to ask - a warm thing is worth everything in the cold and
		// nothing in the warm, so whether it reaches the cold, and whether it
		// reaches it by being bought or by being given, is what says whether
		// there is a market here or only a habit of sharing.
		chill := w.weatherAt(to.X, to.Y, f.Wards)
		if sold {
			w.coatsSold++
			w.coatSoldChill += chill
		} else {
			w.coatGivenChill += chill
		}
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
		Fancies: w.fancyDraws,
		Made:    w.trinketsMade,
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
		out.Spare += w.adornSpare(a)
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
		out.Spare /= n
	}
	if w.trinketMoves > 0 {
		out.Gained = w.trinketMoveGain / float64(w.trinketMoves)
	}
	if w.trinketSales > 0 {
		out.GainedSold = w.trinketSaleGain / float64(w.trinketSales)
	}
	return out
}
