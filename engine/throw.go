package engine

import "math"

// Throwing a stone (stage 46).
//
// The first new way of doing harm since the world began, and the one rule in
// this run of stages that can break it. Everything else here has been about
// food; this is about the thing the whole world is built on - that hitting
// somebody costs the one who does it, because the other one hits back.
//
// Stage 12a measured what that is worth. Agents believe they will be hit back
// 0.7 of the time; the world actually does it 0.15 of the time; and teaching
// them the truth halves the population. The fear is what holds the place
// together. A body that can throw from out of reach lowers the truth further,
// which is why this starts switched off and why TrueRetaliation is read beside
// the population every time it is measured.
//
// What it is, and what it deliberately is not.
//
//   - It is not the melee turned into a skill (#64 forbids that, #71 draws the
//     line). The damage, the guard that turns it aside and the dodge that
//     avoids it are the three genes doing exactly what they do at arm's
//     length, through the same resolution: a thrown stone is an entry in the
//     same list of blows as any other.
//   - It is not free. It costs a stone, which is a thing that had to be found
//     on broken ground and carried in the one hand a body has (stage 45), and
//     the stone lands by the target rather than vanishing - so the world's
//     count of them is unchanged and the ammunition is now over there.
//   - It is not a way of hitting what cannot be seen. The range is longer than
//     an arm and shorter than sight, which is the promise stage 19 made about
//     what a player may aim at, kept for the AI as well.
//   - It leaves no opening (stage 24). A body that misses at arm's length is
//     off balance for a moment; one that misses from twenty paces is not, and
//     the reason 24 gave for the rule does not reach this far.

// throwRange is how far a stone goes. Beyond sight it would be a rule about
// hitting what cannot be seen, so it is clamped there.
func (w *World) throwRange() float64 {
	if !w.cfg.Throwing || w.cfg.ThrowRange <= 0 {
		return 0
	}
	return math.Min(w.cfg.ThrowRange, w.cfg.PerceptionRadius)
}

// canThrow reports whether this body has a stone to throw.
func (a *Agent) canThrow(cfg *Config) bool {
	if !cfg.Throwing {
		return false
	}
	for i := range a.carried {
		if a.carried[i].Kind == FoodStone {
			return true
		}
	}
	return false
}

// throwHit is the chance a stone thrown this far finds its mark, before the
// target's own guard and footwork are asked about.
//
// It falls with distance, and that fall is the only thing the skill of stage
// 47 is allowed to touch: the damage is the attack gene's business and the
// dodging is the target's, so a skill that lifted either would be the melee
// with a new name (#71).
func (w *World) throwHit(a *Agent, dist float64) float64 {
	r := w.throwRange()
	if r <= 0 {
		return 0
	}
	near := w.cfg.CombatRadius
	far := clamp((dist-near)/math.Max(r-near, 1e-9), 0, 1)
	return clamp(w.cfg.ThrowHit*(1-w.throwFalloff(a)*far), 0, 1)
}

// throwFalloff is how much of the hit chance the distance takes away. Stage 47
// is where a body can learn to lose less of it; until then it is the world's
// figure for everybody.
func (w *World) throwFalloff(a *Agent) float64 {
	f := clamp(w.cfg.ThrowFalloff, 0, 1)
	if w.cfg.SkillThrowRelief <= 0 || a == nil {
		return f
	}
	// What a body has learned takes some of the distance back (stage 47).
	// Only this: what the stone does on arrival is the attack gene's work and
	// what the target does about it is the target's, so a skill that touched
	// either would be the melee with a new name (#71).
	return f * (1 - clamp(w.cfg.SkillThrowRelief*a.skillAt(&w.cfg, SkillThrow), 0, 1))
}

// throwStone is the act. It takes the stone out of the thrower's hand, puts it
// on the ground by the target, and adds a blow to the same list every other
// blow goes into - so the guard, the dodge and the memory of what it cost are
// the ordinary rules.
func (w *World) throwStone(a, o *Agent) bool {
	i := -1
	for j := range a.carried {
		if a.carried[j].Kind == FoodStone {
			i = j
			break
		}
	}
	if i < 0 {
		return false
	}
	stone := a.carried[i]
	w.removeCarried(a, i)
	stone.ID = 0
	stone.X, stone.Y = o.X+w.randRange(-6, 6), o.Y+w.randRange(-6, 6)
	w.putFood(stone)

	w.throws++
	w.attacks = append(w.attacks, attack{
		fromID: a.ID, toID: o.ID, effort: a.Action.Effort,
		thrown: true, hit: w.throwHit(a, math.Sqrt(dist2(a.X, a.Y, o.X, o.Y))),
	})
	return true
}

// throwDamage is what a stone does when it lands: the attack gene's work, as
// at arm's length, in one go rather than by the tick.
func (w *World) throwDamage(from *Agent, effort float64) float64 {
	return damagePerTick(&w.cfg, from.Attack(&w.cfg), effort) * w.cfg.ThrowDamage
}

// throwHitFor is what the one doing the reckoning is told about throwing at
// this target: zero unless there is a stone in hand and the target is inside
// the range, so an option a body cannot take is never offered to it.
func (w *World) throwHitFor(a, o *Agent, dist float64) float64 {
	r := w.throwRange()
	if r <= 0 || dist > r || !a.canThrow(&w.cfg) {
		return 0
	}
	return w.throwHit(a, dist)
}
