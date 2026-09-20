package engine

// Hides: the first material (TODO 8).
//
// Everything a body has ever wanted in this world, it could get by itself. A
// plant is picked, a carcass is killed, a stone is picked up off the rough
// ground, an ornament is made out of nothing but time and vitality. That is
// why there is no market: eight stages of plumbing - money, caches, prices,
// haggling, crying your wares, a warm thing that saves lives - and the trade
// is still about two sales in a run, because #115(b) was true all along.
// Nobody buys what they can make.
//
// A hide is the first thing a body can want and not be able to produce. It
// comes off a dead beast and off nothing else, so how many there are is
// decided by how much hunting the world does; it takes a hand, so a body
// standing over its third carcass cannot keep them all; and with
// WardNeedsHide on it is the only way to a coat, so the bodies that cannot
// get one are exactly the bodies that need one.
//
// Three things it deliberately is not.
//
//   - Not a new kind of ownership (#72, and the stone's rule). It lies where
//     the carcass fell. The body that made the kill is standing on it, which
//     is all the advantage a hunter needs, and the claim that shares out meat
//     is about eating - nothing here eats.
//   - Not a new want. What a hide is worth is what it can be made into,
//     discounted by the same CarryValue a meal kept for later is: one figure,
//     worked out in one place (craftWant), so that picking one up, holding
//     one, selling one, buying one and making something of it cannot
//     disagree. That is stage 77's rule, and stage 88 found the coat break it.
//   - Not free of the weather's arithmetic. A hide in the warm is worth the
//     ornament it would make and no more; in the cold it is worth the coat.
//     Nothing was written to say so.

// hidesDrop says whether this world leaves them at all.
func (w *World) hidesDrop() bool {
	return w.cfg.HidePerBudget > 0
}

// hideOf is how much of this body is worth working, before the divisor: its
// bulk, times what its sort leaves (EnemyKind.Hide), and nothing at all for
// anything that is not a beast.
//
// Only the beasts, because the whole of the stage is that the material is
// somewhere rather than everywhere: bodies die all over this world, and a
// world where every death leaves a hide is a world where everybody has one -
// which is the world being measured against.
func (w *World) hideOf(a *Agent) float64 {
	if a.Species != SpeciesEnemy {
		return 0
	}
	bulk := a.Bulk(&w.cfg)
	if k := w.kindOf(a); k.Hide > 0 {
		return bulk * k.Hide
	}
	return bulk
}

// dropHide leaves what a dead beast's skin comes to where it fell.
//
// It rides beside dropMeat rather than inside it: the two answer different
// questions about the same carcass, they have their own doses, and a world
// with one and not the other is a world either of them might want to be
// measured in.
func (w *World) dropHide(a *Agent) {
	if !w.hidesDrop() {
		return
	}
	items := int(w.hideOf(a) / w.cfg.HidePerBudget)
	if items <= 0 {
		return
	}
	for i := 0; i < items; i++ {
		if w.cfg.MaxHideItems > 0 && w.countKind(FoodHide) >= w.cfg.MaxHideItems {
			return
		}
		w.hidesDropped++
		w.putFood(Food{
			X: a.X + w.randRange(-6, 6), Y: a.Y + w.randRange(-6, 6),
			Kind: FoodHide,
		})
	}
}

// wardGap is what one more warm thing of WardStrength would take off this
// body's drain where it stands, in vitality a tick.
//
// The margin and not the whole of it: a body already wearing a coat gains
// nothing from a second, because what it wards is the best single thing it
// holds (wardsOff). That is the same subtraction wardValue makes for a piece
// lying on the ground, asked about a piece that does not exist yet - which is
// what wanting the material for one is.
//
// Over every weather the world has, best first, because a maker cannot aim at
// which weather its piece answers and the best it could come out with is what
// it would reckon on.
func (w *World) wardGap(a *Agent) float64 {
	strength := clamp(w.cfg.WardStrength, 0, 1)
	if strength <= 0 {
		return 0
	}
	best := 0.0
	for kind := Weather(0); kind < NumWeathers; kind++ {
		drain := w.drainFor(kind)
		if drain <= 0 {
			continue
		}
		here := w.weatherAt(a.X, a.Y, kind)
		if here <= 0 {
			continue
		}
		gain := strength - clamp(w.wardsOff(a, kind), 0, 1)
		if gain <= 0 {
			continue
		}
		if v := drain * here * gain; v > best {
			best = v
		}
	}
	return best
}

// holdsHide says whether this body has the material for a warm thing.
func (a *Agent) holdsHide() bool {
	return a.carriedIndex2(FoodHide) >= 0
}

// asksForHides says whether anything in this world reads the two figures a
// body's own view carries about the material. Asked before either is worked
// out: a view is built for every decision and for every seller a buyer looks
// at, and neither figure is free (one walks the hand, the other the
// weathers). A world with no material in it pays nothing for this file.
func (w *World) asksForHides() bool {
	return w.cfg.WardNeedsHide
}

// takeHide uses one up, and says whether there was one. Called at the end of
// a making, where the piece is put into the hand.
func (w *World) takeHide(a *Agent) bool {
	i := a.carriedIndex2(FoodHide)
	if i < 0 {
		return false
	}
	w.removeCarried(a, i)
	w.hidesWorked++
	return true
}

// HideUse is what the material came to. Read only.
type HideUse struct {
	// Dropped is how many came off the beasts over the run and Worked how
	// many were made into something. The gap between them is the whole
	// supply question: a material nobody picks up is a rule that never fires
	// (stage 45's habit), and one that is worked the moment it is found is a
	// material with no slack in it to trade.
	Dropped int
	Worked  int

	// Lying is how many are on the ground now and Held how many are in hands.
	Lying int
	Held  int

	// Holders is the share of living bodies holding one, and Spare the share
	// of the held ones whose holder would gain nothing by wearing what it
	// would make - the surplus, which is the half of a market that no amount
	// of wanting can conjure (stage 68 measured that the missing side here is
	// the seller).
	Holders float64
	Spare   float64

	// Sold and Given are how many changed hands each way.
	Sold  int
	Given int
}

// Hides reports it.
func (w *World) Hides() HideUse {
	out := HideUse{
		Dropped: w.hidesDropped, Worked: w.hidesWorked,
		Sold: w.hidesSold, Given: w.hidesGiven,
	}
	for i := range w.foods {
		if w.foods[i].Kind == FoodHide {
			out.Lying++
		}
	}
	people, holders, spare := 0.0, 0.0, 0.0
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		people++
		held := 0
		for k := range a.carried {
			if a.carried[k].Kind == FoodHide {
				held++
			}
		}
		if held == 0 {
			continue
		}
		out.Held += held
		holders++
		if w.wardGap(a) <= 0 {
			spare += float64(held)
		}
	}
	if people > 0 {
		out.Holders = holders / people
	}
	if out.Held > 0 {
		out.Spare = spare / float64(out.Held)
	}
	return out
}

// noteHideMove records one changing hands. It rides on the two lines that
// already count a hand-over, as the ornament's own tally does.
func (w *World) noteHideMove(f *Food, sold bool) {
	if f.Kind != FoodHide {
		return
	}
	if sold {
		w.hidesSold++
		return
	}
	w.hidesGiven++
}
