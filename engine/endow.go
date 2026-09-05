package engine

import "fmt"

// This file holds the one thing in the engine that hands an agent something it
// did not inherit or earn.
//
// It exists because of a problem that is entirely about playing: the budget is
// fixed and its split is inherited, so the body a player is handed may have
// spent almost nothing on being quick, and a person driving it experiences the
// world as treacle. The fix is not to make the world faster - that would make
// the played world a different one from the measured world - but to hand the
// protagonist a body worth driving.
//
// Three things keep this honest:
//
//   - Nothing in the simulation calls it. No rule, no controller, no birth.
//     The only caller is whatever is running a game.
//   - It pays with new budget rather than out of the other genes. Taking the
//     speed out of the vitality would be a trade the player did not agree to,
//     and would quietly make the protagonist frail.
//   - It is one gene at a time and it says how much it gave, so an interface
//     can show it. A gift nobody is told about is indistinguishable from the
//     world being generous, and this world is not.
//
// What it does not do is stay out of the population. The budget is heritable,
// so a protagonist's children inherit the enlarged one: over a long line, the
// gift spreads. That is a real consequence and there is no way round it short
// of a second rule about whose budget is real, which would be worse.

// Endow raises one of an agent's genes to at least value, paying for it with
// new budget rather than out of the other eight. It returns how much was added,
// which is zero when the gene was already there or better.
//
// The value is clamped to the range a gene can take, so asking for more than
// MaxAbility gives MaxAbility and asking for less than MinAbility gives
// nothing.
func (w *World) Endow(id int, g Gene, value float64) (float64, error) {
	a := w.agentByID(id)
	if a == nil || !a.Alive {
		return 0, fmt.Errorf("node #%d is gone", id)
	}
	if int(g) >= len(a.Genome) {
		return 0, fmt.Errorf("no such gene (%d)", g)
	}
	want := clamp(value, MinAbility, MaxAbility)
	have := a.Genome[g]
	if have >= want {
		return 0, nil
	}
	a.Genome[g] = want
	return want - have, nil
}
