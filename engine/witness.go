package engine

import "slices"

// What everybody standing near an event comes away with.
//
// Two rules need the same scan. Stage 35 wrote the first one out plainly for
// drowning, and said the second would be what tells us which parts are common
// (#60). Here it is, and the answer is: the scan is common, and nothing else
// is. What a witness takes in, where it puts it, and how heavily it counts are
// different in the two cases - one is about a place, the other about a person
// - so only the walk over the onlookers is shared.
//
// The common part is small on purpose. An abstraction that covered the update
// as well would have to be told the field to write to and the weight to write
// with, which is the same code with extra steps.

// forEachWitness runs fn for every living agent that can see a spot, skipping
// the one the event happened to.
//
// Sight is the same rule the agents themselves are held to (canSee), asked
// from the onlooker's side: an agent that could not have seen it learns
// nothing, wherever it happens to be standing on the index.
//
// fn must not walk the index itself: this borrows w.nearScratch, and a nested
// query would take it away underneath the loop.
func (w *World) forEachWitness(x, y float64, subject int, fn func(*Agent)) {
	w.nearScratch = w.appendAgentsInSight(w.nearScratch[:0], x, y)
	for _, idx := range w.nearScratch {
		o := &w.agents[idx]
		if !o.Alive || o.ID == subject {
			continue
		}
		if !w.canSee(o.X, o.Y, x, y) {
			continue
		}
		fn(o)
	}
}

// --- a killing, seen (stage 31) ---------------------------------------------

// witnessKill is what a killing leaves with the people who saw it.
//
// Until now the most informative thing that happens in this world went past
// the onlookers with only whatever the fight itself had already told them. The
// blows are watched (exchangeReadings, every spectateInterval ticks), but the
// end of it - that one of them is now dead and who did it - was not an event
// anybody could learn from.
//
// One path, and the sign of it depends on one comparison: whether the killer
// killed one of the witness's own kind.
//
//   - One of us killed one of them: the witness thinks better of the killer
//     (AffinityWitnessKill). This is the only affinity in the world that can
//     be earned without taking part - AffinityHunt is for those on the
//     carcass's claim - and it is the third party's version of it.
//   - Anything else: the witness takes a reading of the killer's strength, the
//     way it would from watching a fight. A body that has just killed is worth
//     knowing about whoever it belonged to.
//
// The reading is deliberately coarser than a look somebody paid for (#55).
// ActObserve costs vitality and time and can only be spent on one person at a
// time; if standing there and seeing it happen taught as much, nobody would
// ever choose to watch anybody. KillWitnessFactor is a multiplier on the
// variance of an onlooker's reading, so above one is worth less than a look.
//
// KillWitnessLooks is the other half of the same worry. A reading also goes
// into what the observer thinks a build is worth (appearance.go), and these
// readings are not a sample of the world: they are all of somebody who has
// just killed. A line fitted to them is fitted to the winners.
//
// Both weights can be zero, and the count of who saw what is taken either way:
// how often a rule has the chance to fire is the ceiling on what it can
// explain, whatever its weight (the lesson of stage 24).
func (w *World) witnessKill(victim *Agent, killers []int) {
	if len(killers) == 0 {
		return
	}
	cfg := &w.cfg
	spectated := cfg.CombatObsVariance * cfg.SpectateObsFactor * cfg.KillWitnessFactor
	w.forEachWitness(victim.X, victim.Y, victim.ID, func(o *Agent) {
		if slices.Contains(killers, o.ID) {
			// It did it. Whoever took part is paid by the rule for taking
			// part (AffinityHunt, food.go) and does not also collect for
			// having watched: this is the third party's version of that rule,
			// not a second helping of it.
			return
		}
		for _, id := range killers {
			killer := w.agentByID(id)
			if killer == nil || !killer.Alive {
				continue
			}
			if killer.Species == o.Species && victim.Species != killer.Species {
				w.avengeWitnesses++
				w.rememberAffinityIfRoom(o, id, cfg.AffinityWitnessKill)
				continue
			}
			w.killWitnesses++
			if cfg.KillWitnessFactor > 0 && w.takeReading(o, killer, spectated, cfg.KillWitnessLooks) {
				w.killLessons++
			}
		}
	})
}
