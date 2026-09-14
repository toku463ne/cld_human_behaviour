package engine

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
	}
	if w.cfg.TrinketSpoilTicks > 0 {
		item.SpoilAt = w.tick + w.cfg.TrinketSpoilTicks
	}
	a.carried = append(a.carried, item)
	w.heldKind[FoodTrinket]++
	a.requestDecision(TriggerGoalReached)
}

// trinketWorth is what having this particular one is worth.
//
// It is the given figure times what this piece came out like, less what is
// left of it: a thing an hour from being gone is worth an hour of it. The
// discount is continuous rather than a cliff so that letting one go while it
// is still worth something is an ordinary comparison and not a deadline.
//
// It does not depend on who is holding it. What varies is the object, not the
// taste - a taste would be a second kind of variation and would make it
// impossible to say which of the two a scattered price came from.
func (w *World) trinketWorth(f *Food) float64 {
	if !w.cfg.Trinkets || f.Kind != FoodTrinket {
		return 0
	}
	worth := w.cfg.TrinketValue * w.cfg.LifeValue * f.Made
	if f.SpoilAt > 0 && w.cfg.TrinketSpoilTicks > 0 {
		left := float64(f.SpoilAt-w.tick) / float64(w.cfg.TrinketSpoilTicks)
		worth *= clamp(left, 0, 1)
	}
	return worth
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
}

// Trinkets reports what they came to.
func (w *World) Trinkets() TrinketUse {
	out := TrinketUse{Made: w.trinketsMade}
	if w.trinketsMade > 0 {
		out.Quality = w.trinketWorthMade / float64(w.trinketsMade)
	}
	n, worth := 0.0, 0.0
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		n++
		held := 0
		for k := range a.carried {
			if a.carried[k].Kind == FoodTrinket {
				held++
				worth += a.carried[k].Made
			}
		}
		out.Held += held
		if held > 0 {
			out.Holders++
		}
	}
	if out.Held > 0 {
		out.Best = worth / float64(out.Held)
	}
	if n > 0 {
		out.Holders /= n
	}
	return out
}
