package engine

import (
	"errors"
	"fmt"
	"math"
)

// This file holds the other half of stage 23: what a person chooses at the
// milestones of a life, as against the turning points of a day (guided.go).
//
// What is chosen is the three preferences of stage 12a - what somebody's past
// cost puts you off by, what removing a rival is worth, how dangerous running
// low feels. They are the right thing for a person to be given because of what
// stage 12a decided about them: unlike the two beliefs, there is no true value
// for them anywhere in the world, so nothing can be learned about them and
// nothing is being overruled by handing them to a player.
//
// Two things make this a choice rather than a slider:
//
//   - The three are read as multiples of the world's own figures. They are
//     measured in different things and cannot otherwise be added up, and the
//     multiple is already how the rest of the code treats them (the spread
//     drawn at birth is proportional, and the ceiling is a multiple).
//   - The total is what was inherited and does not change. Being more careful
//     therefore costs something: the only place the care can come from is the
//     other two. Without that the answer is the same every time, and a choice
//     with one right answer stops being made after the first life.

// Disposition is a node's three preferences as multiples of the world's own
// figures. A disposition of all ones is a node that wants exactly what the
// world was tuned for.
type Disposition struct {
	Risk        float64 // how much what somebody once cost you puts you off
	Competition float64 // what removing a future rival for food is worth
	Shock       float64 // how dangerous being low on vitality feels
}

// Total is the share a life has to divide between the three. It is inherited
// and a person cannot change it - only how it is split.
func (d Disposition) Total() float64 { return d.Risk + d.Competition + d.Shock }

// ErrDisposition is a split that cannot be made: a negative share, nothing to
// divide, or one that would put a preference past the ceiling the world was
// tuned in.
var ErrDisposition = errors.New("that is not a disposition this node could have")

// Disposition reads how one node is disposed.
func (w *World) Disposition(id int) (Disposition, bool) {
	a := w.agentByID(id)
	if a == nil {
		return Disposition{}, false
	}
	return w.dispositionOf(a), true
}

func (w *World) dispositionOf(a *Agent) Disposition {
	cfg := &w.cfg
	share := func(v, centre float64) float64 {
		if centre == 0 {
			return 0
		}
		return v / centre
	}
	return Disposition{
		Risk:        share(a.lore.riskWeight, cfg.RiskWeight),
		Competition: share(a.lore.competitionWeight, cfg.CompetitionWeight),
		Shock:       share(a.lore.shockRisk, cfg.ShockRisk),
	}
}

// SetDisposition splits what this node has to divide the way a person asked
// for. The proportions are what is read: the ask is scaled to the total the
// node already had, so an interface can hand over three numbers in any units
// it likes and still be correct - the same reason an order's effort is clamped
// rather than refused.
//
// It is refused when the ask cannot be made at all: nothing to divide by, a
// negative share, or a split that puts one preference past the ceiling.
func (w *World) SetDisposition(id int, want Disposition) error {
	a := w.agentByID(id)
	if a == nil || !a.Alive {
		return fmt.Errorf("node #%d is gone", id)
	}
	asked := want.Total()
	if asked <= 0 || math.IsNaN(asked) || math.IsInf(asked, 0) ||
		want.Risk < 0 || want.Competition < 0 || want.Shock < 0 {
		return fmt.Errorf("%w: %v", ErrDisposition, want)
	}
	total := w.dispositionOf(a).Total()
	scale := total / asked
	out := Disposition{
		Risk:        want.Risk * scale,
		Competition: want.Competition * scale,
		Shock:       want.Shock * scale,
	}
	if out.Risk > maxPreferenceFactor || out.Competition > maxPreferenceFactor || out.Shock > maxPreferenceFactor {
		return fmt.Errorf("%w: no share may pass %d times the world's own figure",
			ErrDisposition, maxPreferenceFactor)
	}
	cfg := &w.cfg
	a.lore.riskWeight = out.Risk * cfg.RiskWeight
	a.lore.competitionWeight = out.Competition * cfg.CompetitionWeight
	a.lore.shockRisk = out.Shock * cfg.ShockRisk
	return nil
}
