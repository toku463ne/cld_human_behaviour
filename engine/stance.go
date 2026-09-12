package engine

// This file splits the effort an agent pours into a fight across three
// channels: hitting, guarding, and getting out of the way.
//
// Before it, effort was one number, and a fight was decided by who could pour
// more of it into hitting. That made attack the only gene worth buying (see
// HISTORY.md for stage 7c, where killShare reached 0.92 and the population
// fell by two thirds), because there was nothing else to spend a body on that
// paid off in a fight.
//
// The three channels are offered as three ready mixes rather than as free
// numbers. An agent choosing its own split across three channels would be
// scoring a cube of options every time it looked at somebody; three stances
// cover the shapes that matter - go at them, guard, keep away - and the cost
// of thinking stays where it was.
//
// The vitality a stance costs is the sum over the channels of what each one is
// used at, priced separately, which is PLAN.md's "max x usage x unit price".

// Stance is how an agent is carrying itself in a fight.
type Stance uint8

const (
	// StanceAggressive puts everything into the blow and almost nothing into
	// not being hit.
	StanceAggressive Stance = iota

	// StanceGuarded trades some of the blow for turning others aside.
	StanceGuarded

	// StanceEvasive gives up most of the blow for a chance of not being there
	// when it lands.
	StanceEvasive

	NumStances = int(iota)
)

func (s Stance) String() string {
	switch s {
	case StanceGuarded:
		return "guarded"
	case StanceEvasive:
		return "evasive"
	}
	return "aggressive"
}

// channels is how much of each is being used, from 0 to 1.
type channels struct{ Attack, Defence, Evasion float64 }

// The three mixes. They deliberately do not add up to the same total: guarding
// costs less than swinging, and an agent that gives up on hitting is spending
// less overall, which is what makes standing off a real option for something
// that cannot win.
var stanceMix = [NumStances]channels{
	StanceAggressive: {Attack: 1.0, Defence: 0.1, Evasion: 0.0},
	StanceGuarded:    {Attack: 0.5, Defence: 0.9, Evasion: 0.1},
	StanceEvasive:    {Attack: 0.15, Defence: 0.3, Evasion: 0.9},
}

// mix is the channels an agent is using right now. Only the fighting actions
// carry a stance: an agent that is eating or resting is not guarding, which is
// what makes hitting somebody who is not looking the cheap thing it should be.
func (a *Agent) mix() channels {
	switch a.Action.Kind {
	case ActAttack, ActFlee:
		return stanceMix[a.Action.Stance%Stance(NumStances)]
	}
	return channels{}
}

// defence is the fraction of an incoming blow this agent turns aside: what it
// spent on the gene, at how hard it is currently guarding.
func (a *Agent) defence(cfg *Config) float64 {
	return cfg.DefenceCap * (a.Gene(GeneDefence) / MaxAbility) * a.mix().Defence
}

// composure is what this agent's guard and dodge are currently worth. It is 1
// except in the ticks after a blow of its own found nothing, which is the one
// thing in the world that costs an attacker for missing.
//
// It scales both channels rather than adding damage, because being off balance
// is about not being able to answer the next blow, not about being made of
// something softer.
func (a *Agent) composure(cfg *Config, tick int) float64 {
	if cfg.OpeningTicks <= 0 || a.openUntil <= tick {
		return 1
	}
	return clamp(cfg.OpeningGuard, 0, 1)
}

// evasion is the chance of a blow missing entirely. Unlike defence it is all
// or nothing, and it leans on being quick as well as on the gene: getting out
// of the way is a matter of moving.
func (a *Agent) evasion(cfg *Config) float64 {
	if cfg.MaxSpeed <= 0 {
		return 0
	}
	// The speed it can actually make today, not the one it was born for
	// (stage 66): a body that is moving slower is a body that is easier to
	// hit, and the three readings of a speed have to agree.
	quick := clamp(a.speedNow(cfg)/cfg.MaxSpeed, 0, 2)
	return clamp(cfg.EvasionCap*(a.Gene(GeneEvasion)/MaxAbility)*a.mix().Evasion*quick, 0, cfg.EvasionCap)
}

// cover is what the ground is worth to whoever is being swung at (stage 30):
// a body a level or more above the one attacking it is harder to reach, and
// the bank gives it an evade chance of its own.
//
// It is a floor and not a multiplier, and the first version of it was the
// multiplier. That version could not fire: evasion is a stance channel rather
// than a property of a body, and a body only has a stance while it is fighting
// or running (15.7% of agent-ticks; evasive in 7.7%), so what the rule
// multiplied was almost always zero. Counting how often the *situation* arose
// - 28% of blows are thrown across a height difference - said nothing about
// how often the *quantity being multiplied* was anything at all. A floor works
// on the eating, the resting and the courting alike, which is what "behind a
// bank" ought to mean.
//
// It acts on evasion rather than on damage on purpose. Height does not armour
// anybody - it makes them hard to get at, which is what evasion already means
// (stance.go's third channel); armouring them would be defence, and defence is
// what a narrow place would give (PLAN.md's table).
//
// This is the first rule in the world that reads the ground under two bodies
// rather than under one. Everything terrain has done until now - what a step
// costs, whether a step is allowed - was a property of a cell; being above
// somebody is not a property of anywhere, it is a relation.
//
// One step of advantage is all of it. Being two levels up is not twice as hard
// to reach as being one level up: what a level buys is the edge itself, and a
// rule that kept paying would make a stack of plateaus a fortress nobody could
// ever be dislodged from.
func (w *World) cover(defender, attacker *Agent) float64 {
	if w.cfg.HighGroundCover <= 0 || w.ground == nil {
		return 0
	}
	if w.terrainAt(defender.X, defender.Y).Height <= w.terrainAt(attacker.X, attacker.Y).Height {
		return 0
	}
	return w.cfg.HighGroundCover
}

// stanceCost is the vitality a tick of this stance costs, before anything is
// spent on moving: each channel at what it is used at, priced separately.
func stanceCost(cfg *Config, s Stance) float64 {
	m := stanceMix[s%Stance(NumStances)]
	return cfg.AttackCost*m.Attack + cfg.DefenceCost*m.Defence + cfg.EvasionCost*m.Evasion
}

// coveredFromAttacker says whether the one currently hitting this agent is
// hitting it from below (stage 30). False when nobody is.
func (w *World) coveredFromAttacker(a *Agent) bool {
	if a.attackerID == 0 {
		return false
	}
	from := w.agentByID(a.attackerID)
	if from == nil || !from.Alive {
		return false
	}
	return w.cover(a, from) > 0
}
