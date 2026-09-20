package engine

// Being in the air (2026-09-20).
//
// A sort with EnemyKind.Flies is over the ground rather than on it, and the
// ground's costs, its drowning, its drag and its drain are nothing to it
// (enemykind.go). This is the other half of that bargain, and without it the
// first half is simply a better body: what is in the air cannot reach the
// ground, and what is on the ground cannot reach the air.
//
//   - It cannot strike and it cannot feed from up there. Both are things that
//     happen at arm's length from the ground.
//   - Nothing on the ground can strike it either. A blow aimed at something
//     in the air does not land.
//   - A thrown stone does (stage 46). That is the one answer the ground has,
//     and it is the reason this is a rule and not an invulnerability.
//
// What decides whether it is up or down, and what was not chosen. It is up
// except while its chosen action is one that needs the ground - so coming
// down is not a decision of its own, it is what choosing to eat or to strike
// already means. That keeps the whole rule out of the utility comparison:
// there is no threshold at which it descends, no "land" in the vocabulary,
// and nothing to tune. The cost of eating, for a flier, is that it is a
// target while it eats.
//
// The alternative was to make it depend on where it actually is - down only
// while inside CombatRadius of what it is striking. That is more exact and
// it was not taken: it would make a diving flier safe for the whole approach
// and vulnerable for one tick, which is an invulnerability with a formality
// attached, and it would need the distance computing in three more places.
// Committing to the dive is the exposure.
//
// A sort that is not a flier is never aloft, so every world before this one
// reads false everywhere and nothing here runs.

// aloft says this body is in the air right now.
//
// The list of actions that bring it down is deliberately short and explicit:
// anything added to the vocabulary that reaches the ground has to be added
// here too, and the compiler will not say so.
func (w *World) aloft(a *Agent) bool {
	if a == nil || a.Species != SpeciesEnemy || !w.kindOf(a).Flies {
		return false
	}
	switch a.Action.Kind {
	case ActEat, ActAttack, ActTake:
		return false
	}
	return true
}
