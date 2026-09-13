package engine

import "math"

// How well a body has been keeping itself up, and what the second window makes
// of it (stage 72).
//
// Stage 67 gave the utility formula one more planning horizon, and measured
// over 96 seeds it is the largest single rule this world has. It cannot be the
// default, and the reason is the assumption the second window is built on: the
// body is carried forward on its own metabolism with nothing to eat. That is
// what makes a satiated whole body value a meal kept for later - it can see
// itself running short - and it is also what makes a body in trouble read
// every option as equally hopeless, because the life term is a difference of
// two chances of dying and both saturate at one. Made the default on
// 2026-09-13, it cost two rules this project says must not break: a starving
// body courted instead of eating, and a cornered one lay down instead of
// running.
//
// So the assumption is softened rather than removed. A body assumes it will go
// on keeping itself up the way it has been - discounted, like every other
// forward-looking figure here (CarryValue, StoreValue, CoinValue) - and never
// better than breaking even: looking ahead is what happens if things go on as
// they are, not what happens if they go well.
//
// Upkeep is two halves and one dial. Eating is the obvious one; mending is the
// one the measurement pointed at. A body at a seventh of its vitality was
// lying down under a beating rather than running, and it was not hunger that
// made it hopeless: the chance of dying of being worn down is a standing
// hazard, and chained over a second window in which nothing ever gets better
// it goes to one whatever the body does. Escaping was worth 46.23 of life with
// one window and 0.63 with two.
//
// Nothing new is detected and no new state is added. What a body has been
// getting is a decaying ledger in the same shape as the sameness of its diet
// (stage 16) and its mood (stage 54): a number that fades when it is read. Its
// half-life is PlanHorizon, which is not a new figure to calibrate - it is the
// window the guess is about.

// fedDecay brings the feeding ledger up to the present.
func (a *Agent) fedDecay(cfg *Config, tick int) {
	if cfg.PlanHorizon <= 0 {
		a.fedSum, a.fedAt = 0, tick
		return
	}
	if elapsed := tick - a.fedAt; elapsed > 0 {
		a.fedSum = decay(a.fedSum, math.Ln2/cfg.PlanHorizon, elapsed)
	}
	a.fedAt = tick
}

// noteFed records what a mouthful actually took off this body's hunger. It
// rides on the lines where that figure is already worked out - the same three
// places stage 54 takes its cheer from - so no eating is detected twice and
// none is detected anew.
func (w *World) noteFed(a *Agent, fed float64) {
	if w.cfg.LookaheadUpkeep <= 0 || fed <= 0 {
		return // nothing reads it, so nothing pays for it
	}
	a.fedDecay(&w.cfg, w.tick)
	a.fedSum += fed
}

// fedRate is what that ledger says about hunger per tick. A decaying sum of
// the mouthfuls of the last half-life is rate x halfLife / ln2 in the steady
// state, so the rate is the sum read back through the same constant.
func (a *Agent) fedRate(cfg *Config, tick int) float64 {
	if cfg.LookaheadUpkeep <= 0 || cfg.PlanHorizon <= 0 {
		return 0
	}
	a.fedDecay(cfg, tick)
	return a.fedSum * math.Ln2 / cfg.PlanHorizon
}

// hungerClimb is how fast the second window has hunger rising: the body's own
// metabolism, less as much of it as this body has been managing to cover.
//
// The cap at the metabolism is the line between looking ahead and wishing. A
// body that has been eating twice as fast as it burns does not get less hungry
// in the window it is trying to see into - it gets no hungrier, which is as
// far as this goes.
func hungerClimb(cfg *Config, s *SelfView) float64 {
	if cfg.LookaheadUpkeep <= 0 {
		return s.HungerRate
	}
	covered := cfg.LookaheadUpkeep * math.Min(s.FedRate, s.HungerRate)
	return math.Max(0, s.HungerRate-covered)
}
