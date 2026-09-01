package engine

import "math"

// Opinion is everything one agent believes about another. Two separate things
// are kept, because they answer different questions:
//
//   - Risk answers "what has this one already cost me?" and drives avoidance.
//     It is the vitality lost in fights with them, and it fades slowly: without
//     forgetting, long lived agents end up remembering everybody as a maximum
//     threat and the world seizes up.
//   - Strength answers "how would a fight with them go?" for someone never
//     fought. True combat power is hidden, so each agent carries an estimate
//     with an uncertainty attached.
type Opinion struct {
	Risk     float64 // accumulated vitality lost to this agent, decayed
	riskTick int     // tick Risk was last brought up to date

	Strength float64 // estimate of the other's power
	Variance float64 // how unsure that estimate is
	Samples  int     // observations folded in so far, direct or watched
}

// opinionOf returns this agent's opinion of another, creating it from the
// population prior the first time they are considered.
func (w *World) opinionOf(a *Agent, otherID int) *Opinion {
	if a.opinions == nil {
		a.opinions = make(map[int]*Opinion, 4)
	}
	op, ok := a.opinions[otherID]
	if !ok {
		op = &Opinion{
			Strength: w.cfg.PriorStrength,
			Variance: w.cfg.PriorVariance,
			riskTick: w.tick,
		}
		a.opinions[otherID] = op
	}
	return op
}

// Opinions returns what an agent believes about the others it has met, keyed by
// their ID. The map is the live one; callers must only read it. It is what the
// viewer shows when a node is clicked.
func (w *World) Opinions(id int) map[int]Opinion {
	a := w.agentByID(id)
	if a == nil || len(a.opinions) == 0 {
		return nil
	}
	out := make(map[int]Opinion, len(a.opinions))
	for otherID, op := range a.opinions {
		c := *op
		c.Risk = w.decayedRisk(op)
		out[otherID] = c
	}
	return out
}

// decayedRisk applies the forgetting curve lazily, so that nothing has to be
// walked over every tick.
func (w *World) decayedRisk(op *Opinion) float64 {
	elapsed := w.tick - op.riskTick
	if elapsed <= 0 || op.Risk == 0 {
		return op.Risk
	}
	return op.Risk * math.Exp(-w.cfg.RiskDecayPerTick*float64(elapsed))
}

// rememberDamage records that another agent cost this one some vitality.
func (w *World) rememberDamage(a *Agent, fromID int, damage float64) {
	op := w.opinionOf(a, fromID)
	op.Risk = w.decayedRisk(op) + damage
	op.riskTick = w.tick
}

// observeStrength folds one noisy reading of another agent's power into the
// observer's estimate. The reading is off by an amount that shrinks with the
// observer's rationality, and the estimate's variance shrinks with every
// observation, so watching a lot of fights makes an agent hard to surprise.
func (w *World) observeStrength(observer *Agent, target *Agent, obsVariance float64) {
	if observer.ID == target.ID {
		return
	}
	op := w.opinionOf(observer, target.ID)

	noiseStd := (MaxAbility - observer.Rationality()) / MaxAbility * w.cfg.JudgementNoise
	reading := target.Attack() + w.rng.NormFloat64()*noiseStd
	variance := obsVariance + noiseStd*noiseStd

	k := op.Variance / (op.Variance + variance)
	op.Strength += k * (reading - op.Strength)
	op.Variance *= 1 - k
	op.Samples++
}
