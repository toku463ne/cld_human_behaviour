package engine

// This file splits the fighting by who it is with: the agents somebody has
// been travelling with, or the ones they have just run into. It is the third of
// the "should be" measurements (PLAN.md stage 6.5), where it is written as
// "the fight rate inside a group divided by the fight rate between groups".
//
// That ratio cannot be measured as literally written. A blow only lands inside
// CombatRadius, and CombatRadius (15) is smaller than the cluster linking
// distance (30), so two agents trading blows are always in the same cluster at
// that instant: measured on one frame, the fights between clusters are not rare
// but impossible, and the ratio is a division by zero. (Confirmed by counting:
// 5994 blows inside a cluster, 0 across two, over 20000 ticks.)
//
// What the criterion is actually asking is whether being somebody's companion
// makes fighting them less likely, so that is what is measured here. A pair
// that shares a cluster now is a companion pair if it already shared one
// CompanionLag ticks ago, and a stranger pair if it did not: the same
// encounter, split by whether the two arrived together or just met. Both rates
// are per encounter, so the fact that companions are near each other far more
// often does not decide the answer on its own.

// DefaultCompanionLag is how long two agents must already have been together
// for a meeting between them to count as companions rather than strangers.
//
// It is about three times the measured membership half-life of roughly 60
// ticks (HISTORY.md), so a companion pair is one that has outlasted the
// ordinary chance encounter rather than merely one that was nearby a moment
// ago. Like the other measurement constants it is fixed rather than tunable:
// rates split at different lags are not comparable.
const DefaultCompanionLag = 200

// FightRates is how much fighting goes on within each kind of meeting.
type FightRates struct {
	// CompanionLag is the lag the split was made at.
	CompanionLag int

	// Companion and Stranger are the fraction of meetings of each kind in
	// which a blow is being landed. A meeting is a pair inside CombatRadius at
	// the moment of a reading.
	Companion float64
	Stranger  float64

	// Ratio is Companion divided by Stranger: below one means being somebody's
	// companion makes fighting them less likely, which is what stage 6.5 calls
	// for. It is zero when there were no stranger meetings to divide by.
	Ratio float64

	// The counts behind the rates, so that a ratio resting on a handful of
	// meetings can be recognised as one.
	CompanionMeetings, CompanionFights int
	StrangerMeetings, StrangerFights   int
}

// FightTracker follows who fights whom through a run.
//
// Like MembershipTracker it only reads: it takes a cluster snapshot every step
// ticks, keeps enough of them to look CompanionLag ticks back, and classifies
// the meetings it sees at each reading against the snapshot from back then.
type FightTracker struct {
	linkDist float64
	step     int
	lag      int

	// snapshots of agent ID to cluster label, oldest first.
	snapshots []map[int]int
	ticks     []int

	companionMeetings, companionFights int
	strangerMeetings, strangerFights   int
}

// NewFightTracker splits meetings at lag ticks, clustering at linkDist and
// reading every step ticks.
func NewFightTracker(linkDist float64, step, lag int) *FightTracker {
	if step < 1 {
		step = 1
	}
	if lag < step {
		lag = step
	}
	return &FightTracker{linkDist: linkDist, step: step, lag: lag}
}

// Observe takes one reading of w. Call it every step ticks.
func (f *FightTracker) Observe(w *World) {
	tick := w.Tick()
	agents := w.Agents()
	c := w.Clusters(f.linkDist)

	if past := f.snapshotAt(tick - f.lag); past != nil {
		f.count(w, agents, past)
	}

	labels := make(map[int]int, len(agents))
	for i := range agents {
		labels[agents[i].ID] = c.Labels[i]
	}
	f.snapshots = append(f.snapshots, labels)
	f.ticks = append(f.ticks, tick)

	// Drop the snapshots that are older than the lag needs.
	keep := 0
	for i, t := range f.ticks {
		if tick-t <= f.lag+f.step {
			f.ticks[keep], f.snapshots[keep] = t, f.snapshots[i]
			keep++
		}
	}
	f.ticks, f.snapshots = f.ticks[:keep], f.snapshots[:keep]
}

// count classifies every meeting happening right now against how things stood
// a lag ago. Pairs that were not both there back then are skipped rather than
// guessed at: a newborn has no history to be a companion by.
func (f *FightTracker) count(w *World, agents []Agent, past map[int]int) {
	r2 := w.cfg.CombatRadius * w.cfg.CombatRadius
	for i := range agents {
		for j := i + 1; j < len(agents); j++ {
			if dist2(agents[i].X, agents[i].Y, agents[j].X, agents[j].Y) > r2 {
				continue
			}
			was, ok := past[agents[i].ID]
			if !ok {
				continue
			}
			other, ok := past[agents[j].ID]
			if !ok {
				continue
			}

			fighting := striking(&agents[i], agents[j].ID) || striking(&agents[j], agents[i].ID)
			if was == other {
				f.companionMeetings++
				if fighting {
					f.companionFights++
				}
			} else {
				f.strangerMeetings++
				if fighting {
					f.strangerFights++
				}
			}
		}
	}
}

func striking(a *Agent, targetID int) bool {
	return a.Alive && a.Action.Kind == ActAttack && a.Action.TargetID == targetID
}

// snapshotAt is the reading nearest to the given tick, or nil if none is old
// enough yet.
func (f *FightTracker) snapshotAt(tick int) map[int]int {
	best, bestGap := -1, 0
	for i, t := range f.ticks {
		gap := t - tick
		if gap < 0 {
			gap = -gap
		}
		if best < 0 || gap < bestGap {
			best, bestGap = i, gap
		}
	}
	if best < 0 || bestGap > f.step/2+1 {
		return nil
	}
	return f.snapshots[best]
}

// Result reads off the rates gathered so far.
func (f *FightTracker) Result() FightRates {
	out := FightRates{
		CompanionLag:      f.lag,
		CompanionMeetings: f.companionMeetings,
		CompanionFights:   f.companionFights,
		StrangerMeetings:  f.strangerMeetings,
		StrangerFights:    f.strangerFights,
	}
	if f.companionMeetings > 0 {
		out.Companion = float64(f.companionFights) / float64(f.companionMeetings)
	}
	if f.strangerMeetings > 0 {
		out.Stranger = float64(f.strangerFights) / float64(f.strangerMeetings)
	}
	if out.Stranger > 0 {
		out.Ratio = out.Companion / out.Stranger
	}
	return out
}
