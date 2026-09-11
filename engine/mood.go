package engine

import "math"

// How a body is feeling, and what that does to what it decides (stage 54).
//
// This is the generalisation of what stage 12a found by accident. The utility
// formula assumed bodies get hit back seven times in ten; the world's real
// figure is 0.15; and telling them the truth halved the population, because
// what was holding the world together was a fear of something that was not
// there. The question here is whether a bias of that kind, put in on purpose
// and tied to what has just happened to a body, holds a world together too.
//
// Three lines, and all three are old (#83).
//
//   - It never overrides a choice. Picking a random action some of the time
//     was tried on 2026-08-14 and rejected: it produces suicidal attacks. What
//     the noise does instead is blur the scores, and a mood is a lean in that
//     same blur - it cannot make an option exist, and it cannot make a lethal
//     one win outright.
//   - It is not held against anybody. Two decaying scalars for the whole body,
//     not one per face. A record of who frightened it is a grudge, and this
//     world has refused to keep one since stage 5: violence here is priced as
//     competition, and what somebody has cost this body is already in the risk
//     memory.
//   - It detects nothing new. Dread comes off the damage a blow actually did,
//     cheer off what a meal actually gave - both at the lines where those
//     numbers are already worked out. No new event, no new scan.
//
// What it changes is one number, and deliberately a preference rather than a
// fact. ShockRisk is how heavily a body weighs being worn down - one of the
// three quantities stage 12a set aside as having no right answer in the world,
// and the three a player is handed at milestones (stage 23). A frightened body
// reads it high and a pleased one low, so the world looks more or less
// dangerous than it is depending on what has just happened. Nothing about the
// body changes: what is stored stays what it inherited, and only what it sees
// this tick leans.
//
// The half-life is the whole design, which the count before building it made
// plain. Share of body-samples taken within so many ticks of a beating:
//
//	window   50    150   300   600   1200   ever
//	flat     0.199 0.346 0.466 0.585 0.673  0.713
//	played   0.115 0.229 0.342 0.486 0.637  0.787
//
// A mood that stands two thirds of the time is a constant, and a constant
// changes no ranking at all. So a long memory for fear should do less than a
// short one, not more.

// moodDecay brings the two scalars up to the present. Lazy, on reading, in the
// same shape as the risk and affinity a body holds about other bodies: a mood
// is bookkeeping that fades, not a fifth thing a body is made of.
func (a *Agent) moodDecay(cfg *Config, tick int) {
	if cfg.MoodHalfLife <= 0 {
		a.dread, a.cheer = 0, 0
		return
	}
	if elapsed := tick - a.moodAt; elapsed > 0 {
		rate := math.Ln2 / float64(cfg.MoodHalfLife)
		a.dread = decay(a.dread, rate, elapsed)
		a.cheer = decay(a.cheer, rate, elapsed)
	}
	a.moodAt = tick
}

// frighten is what a blow does to the one that took it, as a share of the body
// it took it out of. Nothing here knows who swung.
func (w *World) frighten(a *Agent, damage float64) {
	if w.cfg.MoodWeight == 0 || w.cfg.MoodDreadGain <= 0 || damage <= 0 {
		return
	}
	max := a.MaxVitality(&w.cfg)
	if max <= 0 {
		return
	}
	a.moodDecay(&w.cfg, w.tick)
	a.dread = clamp(a.dread+w.cfg.MoodDreadGain*damage/max, 0, 1)
}

// please is the same on the other side: what a mouthful actually did, in the
// body's own units. Both halves of a meal count, because both are what a body
// eats for.
func (w *World) please(a *Agent, fed, mended float64) {
	if w.cfg.MoodWeight == 0 || w.cfg.MoodCheerGain <= 0 {
		return
	}
	good := 0.0
	if w.cfg.MaxHunger > 0 {
		good += fed / w.cfg.MaxHunger
	}
	if max := a.MaxVitality(&w.cfg); max > 0 {
		good += mended / max
	}
	if good <= 0 {
		return
	}
	a.moodDecay(&w.cfg, w.tick)
	a.cheer = clamp(a.cheer+w.cfg.MoodCheerGain*good, 0, 1)
}

// mood is how this body is doing, from -1 (badly) to 1 (well).
func (w *World) mood(a *Agent) float64 {
	if w.cfg.MoodWeight == 0 {
		return 0
	}
	a.moodDecay(&w.cfg, w.tick)
	return clamp(a.cheer-a.dread, -1, 1)
}

// shockRiskFelt is what this body makes of being worn down, right now: what it
// inherited, leaning with how the last few hundred ticks have gone.
//
// The lean is on the preference and not on any fact about the world, which is
// the whole of what makes this a feeling rather than a piece of knowledge. A
// frightened body is not wrong about anything it can check; it is weighing the
// same world differently.
func (w *World) shockRiskFelt(a *Agent) float64 {
	shock := a.lore.shockRisk
	if w.cfg.MoodWeight == 0 {
		return shock
	}
	// A frightened body reads it high, a pleased one low. Bounded either way
	// by the same figure, so no mood can make being worn down weigh nothing
	// or weigh everything: what stage 12a clipped at four times the world's
	// value is not undone here.
	return clamp(shock*(1-w.mood(a)*w.cfg.MoodWeight), 0, 2*shock)
}

// MoodOf is how one body is feeling, from -1 to 1, for a viewer. Zero for a
// body that is gone and for a world with no moods in it.
func (w *World) MoodOf(id int) float64 {
	a := w.agentByID(id)
	if a == nil {
		return 0
	}
	return w.mood(a)
}

// Moods is what the population is feeling. Read only.
type Moods struct {
	// Dread and Cheer are the means over the living, and Afraid the share of
	// bodies whose dread outweighs their cheer. The last is the one that says
	// whether the bias is a signal or a constant: a world where it is near one
	// has a mood that never changes, and a constant lean changes no ranking.
	Dread, Cheer, Afraid float64
}

// Mood reports what the population is feeling.
func (w *World) Mood() Moods {
	var out Moods
	var n float64
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		n++
		a.moodDecay(&w.cfg, w.tick)
		out.Dread += a.dread
		out.Cheer += a.cheer
		if a.dread > a.cheer {
			out.Afraid++
		}
	}
	if n == 0 {
		return out
	}
	out.Dread /= n
	out.Cheer /= n
	out.Afraid /= n
	return out
}
