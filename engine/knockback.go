package engine

import "math"

// Being hit moves a body (TODO 13, #136).
//
// What it is for. Until now a fight and the ground it happened on had nothing
// to do with each other: the blows came off the two bodies, and where they
// were standing only changed what the walk to the fight cost. This is the
// join. A blow pushes the one it lands on, along the line from the striker to
// the struck, and whatever is behind them is what they get - a river that
// drowns (stage 34), a drag that slows (stage 97), a drain (stage 99), or the
// edge of a plateau.
//
// Three things about the shape of it.
//
// Nothing here is chosen. There is no term in the utility formula for "I
// might be pushed", and none is wanted: this is a consequence a body has
// done to it, like the opening a missed swing leaves (stage 24) or the fright
// a hard blow gives it (stage 54). What a body does about the water it lands
// in is the ordinary Hazard term reading the ground under its feet on the
// next decision, which already exists and needed no line.
//
// That is still true of this rule after #139 put a chosen push beside it
// (shove.go). The choosing is all on the pushing side: the fourth stance is
// scored like the other three, and being pushed is exactly what it was - a
// thing that happens to you. The two share the machinery below and nothing
// else, and either can be run without the other.
//
// The push is collected and applied after all the blows, not as each lands.
// resolveAttacks asks the spatial index questions inside its loop (the
// readings onlookers take, every spectateInterval ticks), and moving a body
// marks the index stale, so pushing where the blow is resolved would rebuild
// it once per blow - the exact shape the coding rules in CLAUDE.md name. The
// collection buys a second thing for nothing: the outcome no longer depends
// on the order the blows happen to be resolved in.
//
// Down is allowed where walking is not. canStep refuses any change of level
// that is not a ramp, in both directions, which is what makes a plateau a
// route and not a door (stage 20). A shove keeps the half of that rule that
// matters: a body cannot be pushed *up* a level, because that would be a way
// of getting somewhere that walking cannot reach, but it can be pushed off an
// edge, and it pays for the drop. The way back is still the ramp.
type shove struct {
	id     int
	fromID int     // who did it, for the chosen push that learns from the result
	dx, dy float64 // unit vector, striker -> struck
	dist   float64
	chosen bool // a push somebody picked (#139) rather than one a blow gave
}

// noteShove works out where a blow would put the one it landed on, and files
// it for after the loop. It draws no random numbers.
//
// The distance is the damage that actually landed - so a glancing blow at a
// well guarded body moves it less than a full one, and everything that is
// already priced into a blow (power, effort, stance, guard, what the body
// knows about this attacker) is priced into the push without being named
// again - divided by how big the body is. A heavy body takes the same blow
// and gives less ground, which is the mass the user asked for, read off the
// gene that already means "how much body there is".
func (w *World) noteShove(from, to *Agent, damage float64) {
	cfg := &w.cfg
	if cfg.KnockbackDist <= 0 || damage <= 0 || cfg.AttackDamage <= 0 {
		return
	}
	dx, dy := to.X-from.X, to.Y-from.Y
	d := math.Hypot(dx, dy)
	if d < 1e-9 {
		// Standing on exactly the same spot: there is no line to push along,
		// so nothing moves. It is rare, and inventing a direction for it
		// would be the one random draw this rule does not have.
		return
	}
	mass := to.MaxVitality(cfg)
	if mass <= 0 {
		return
	}
	w.shoves = append(w.shoves, shove{
		id:     to.ID,
		fromID: from.ID,
		dx:     dx / d,
		dy:     dy / d,
		dist:   cfg.KnockbackDist * (damage / cfg.AttackDamage) * (cfg.MaxVitality / mass),
	})
}

// noteChosenShove is the same filing for a push somebody picked (#139): the
// fourth stance, where the tick's effort went into moving the other one
// instead of into hurting it.
//
// It is the passive push's arithmetic with the shove channel in place of the
// attack channel, which is what lets the two go opposite ways: the stance that
// pushes hardest is the one that hits least. Everything else is the same, on
// purpose - the force comes off the same gene, so there is no gene for
// pushing, and it is divided by the same mass, so a heavy body gives less
// ground.
//
// guard is what the one being pushed turned aside, already worked out for the
// blow: a body with its guard up is moved less. Getting out of the way is not
// in here because it has already had its say - a shove that was dodged never
// reaches this function, for the same reason a blow that was dodged does no
// damage.
func (w *World) noteChosenShove(from, to *Agent, use, guard float64) {
	cfg := &w.cfg
	if cfg.ShovePush <= 0 || use <= 0 || cfg.AttackDamage <= 0 {
		return
	}
	force := damagePerTick(cfg, from.Attack(cfg), use) * guard
	if force <= 0 {
		return
	}
	mass := to.MaxVitality(cfg)
	if mass <= 0 {
		return
	}
	dx, dy := to.X-from.X, to.Y-from.Y
	d := math.Hypot(dx, dy)
	if d < 1e-9 {
		return
	}
	w.shoves = append(w.shoves, shove{
		id:     to.ID,
		fromID: from.ID,
		dx:     dx / d,
		dy:     dy / d,
		dist:   cfg.ShovePush * (force / cfg.AttackDamage) * (cfg.MaxVitality / mass),
		chosen: true,
	})
	w.shovesMade++
}

// applyShoves moves everybody that was pushed this tick, in the order the
// blows were resolved. A body hit by two at once is pushed twice, each time
// away from the one that hit it, which is what being surrounded does.
func (w *World) applyShoves() {
	for i := range w.shoves {
		s := &w.shoves[i]
		if a := w.agentByID(s.id); a != nil && a.Alive {
			w.pushBody(a, s.dx, s.dy, s.dist)
			if s.chosen {
				w.noteShoveOutcome(s.fromID, a)
			}
		}
	}
	w.shoves = w.shoves[:0]
}

// pushBody is the move itself: the same "try the step, then each half of it"
// shape as walking, so a body shoved into a wall of rock slides along it
// rather than sticking to it, and a body with nowhere to go stands where it
// is.
//
// It charges no movement cost. The body did not choose to go anywhere and is
// not spending anything getting there; what it pays is the fall, and then
// whatever the ground it is now standing on takes per tick like any other
// ground.
func (w *World) pushBody(a *Agent, dx, dy, dist float64) {
	fromX, fromY := a.X, a.Y
	tx, ty := a.X+dx*dist, a.Y+dy*dist
	switch {
	case w.canBeShoved(a, a.X, a.Y, tx, ty):
		a.X, a.Y = tx, ty
	case dx != 0 && w.canBeShoved(a, a.X, a.Y, tx, a.Y):
		a.X = tx
	case dy != 0 && w.canBeShoved(a, a.X, a.Y, a.X, ty):
		a.Y = ty
	default:
		return
	}
	w.keepInBounds(a)
	if a.X == fromX && a.Y == fromY {
		// Shoved into the edge of the world and put back where it was. It
		// did not move, so it is not counted as moved.
		return
	}
	w.invalidateIndex()
	w.knocked++
	// Not marked as having stirred, and no effort recorded against it: those
	// two say what a body did with its tick, and this body did not do this.
	// A body shoved into the river and left standing there is standing still,
	// which is what the measurements of the water want to hear (stage 99).
	if w.ground == nil {
		return
	}

	// What it landed in, read after the edge of the world has had its say, so
	// that a body pushed against the boundary is counted where it ended up.
	before, after := w.terrainAt(fromX, fromY), w.terrainAt(a.X, a.Y)
	if after.Drown > 0 && before.Drown == 0 {
		w.knockedWet++
	}
	drop := int(before.Height) - int(after.Height)
	if drop <= 0 || w.canStep(a, fromX, fromY, a.X, a.Y) {
		// Level ground, or a ramp it could have walked down anyway. Being
		// shoved along a slope is being shoved along ground.
		return
	}
	w.knockedFell++
	a.Vitality -= w.cfg.KnockbackFall * float64(drop)
	// Nothing is done about the death here. The blow that caused this set
	// lastAttackTick to this tick, so a body that runs out of vitality is
	// killed by metabolise a moment later and counted as a killing, with the
	// one who pushed it on the list that shares the carcass (stage 41) and
	// that the onlookers learn from (stage 31). A fall is the end of a blow,
	// not a new way to die.
}

// canBeShoved is canStep with one exception: a shove may take a body down a
// level where walking may not. Up is refused exactly as walking refuses it -
// a world where being hit is how you get onto the plateau is a world where
// the plateau is not a plateau.
func (w *World) canBeShoved(a *Agent, fromX, fromY, toX, toY float64) bool {
	if w.canStep(a, fromX, fromY, toX, toY) {
		return true
	}
	if w.ground == nil {
		return true
	}
	return w.terrainAt(toX, toY).Height < w.terrainAt(fromX, fromY).Height
}

// Knocks is what the rule is doing: how many blows moved a body, how many of
// those put one in water it was not already in, and how many pushed one off
// an edge. Read only, and taken whether or not the rule is on - a count of
// how often something could fire is the ceiling on what it can explain
// (stage 24).
type Knocks struct {
	Pushed int
	Wet    int
	Fell   int
}

// Knocks reports them. Read only.
func (w *World) Knocks() Knocks {
	return Knocks{Pushed: w.knocked, Wet: w.knockedWet, Fell: w.knockedFell}
}
