package engine

// A crop that has to be known to be got (stage 44).
//
// This is the plant side of what an economy would need: something one body can
// produce and another cannot, so that there is ever a reason to get a thing
// from somebody rather than fetch it. Without that, coins and warehouses and
// criers are plumbing with nothing to carry (#70).
//
// It is the second instance of the form stage 42 built for fish, and the two
// are generalised here rather than earlier, for the reason #60 gives: one
// example is not enough to know what the shape is. What they share is a food
// whose amount is decided by what a region is, and which can be reached and
// still not had - so what a skill buys is landing more of them, never getting
// more out of each (the mistake stage 43 was rewritten to undo).
//
// What it deliberately is not.
//
//   - Not a place an agent remembers. A crop known by coordinates would need
//     a new kind of memory and a way to walk to somewhere unseen, which is a
//     stage of its own and not this one (#70). What varies is by region, the
//     grain everything else about a place uses.
//   - Not a gate. A body that has never heard of the thing still gets it a
//     quarter of the time; skill lifts that towards certainty. A skill that
//     switched a food on and off would be the discrete-and-nonlinear trap
//     this project has walked into twice already.
//   - Not extra food, and not a fourth kind. A specialty plant is a plant -
//     it feeds a body what a plant feeds a body, it comes up in place of an
//     ordinary one, and the diet ledger does not learn a new word for it.

// specialShareAt is how much of what comes up in this region is the awkward
// crop: the world's figure, scaled by how much of it this region grows.
func (w *World) specialShareAt(region int) float64 {
	if w.cfg.SpecialtyShare <= 0 || region < 0 || region >= len(w.regions) {
		return 0
	}
	return clamp(w.cfg.SpecialtyShare*w.regions[region].Special, 0, 1)
}

// harvestCatch is the chance this body gets an awkward plant out of the
// ground. Ordinary plants are simply picked, and so are these by a body that
// has never learned the trick - only less often.
func (w *World) harvestCatch(a *Agent, f *Food) float64 {
	if !f.Special || w.cfg.SpecialtyCatch >= 1 {
		return 1
	}
	return catchWith(w.cfg.SpecialtyCatch, w.cfg.SkillHarvestRelief*a.skillAt(&w.cfg, SkillHarvest))
}

// itCameApart is a failed attempt at one. The plant is rooted, so unlike a
// fish it does not go anywhere: what the failure costs is the tick and having
// to decide again.
func (w *World) itCameApart(a *Agent) {
	w.harvestMissed++
	a.requestDecision(TriggerTargetLost)
}

// SpecialtyUse is what the awkward crop is doing. Read only.
type SpecialtyUse struct {
	// Share is how much of what is growing is the awkward kind, Held the share
	// of the population that knows the trick, and Realised what their bodies
	// make of it.
	Share    float64
	Held     float64
	Realised float64

	// Gain is how much more of the crop grows where the knowing bodies are
	// standing than the world's average - the figure this stage turns on. A
	// population that has settled where its trade is worth something reads
	// above zero; one that has not reads zero however well it knows the
	// trick.
	Gain float64
}

// Specialty reports what the crop and the trade are doing.
func (w *World) Specialty() SpecialtyUse {
	var out SpecialtyUse
	var plants, special float64
	for i := range w.foods {
		if w.foods[i].Kind != FoodPlant {
			continue
		}
		plants++
		if w.foods[i].Special {
			special++
		}
	}
	if plants > 0 {
		out.Share = special / plants
	}

	var people, knowing, ground float64
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive || a.Species != SpeciesHuman {
			continue
		}
		people++
		skill := a.skillAt(&w.cfg, SkillHarvest)
		if skill <= 0 {
			continue
		}
		knowing++
		out.Realised += skill
		ground += w.specialShareAt(w.regionIndexAt(a.X, a.Y))
	}
	if people > 0 {
		out.Held = knowing / people
	}
	if knowing > 0 {
		out.Realised /= knowing
		out.Gain = ground/knowing - w.meanSpecialShare()
	}
	return out
}

// meanSpecialShare is what an agent standing anywhere at random would find.
func (w *World) meanSpecialShare() float64 {
	if len(w.regions) == 0 {
		return 0
	}
	sum := 0.0
	for i := range w.regions {
		sum += w.specialShareAt(i)
	}
	return sum / float64(len(w.regions))
}
