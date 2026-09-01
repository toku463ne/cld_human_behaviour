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

// evasion is the chance of a blow missing entirely. Unlike defence it is all
// or nothing, and it leans on being quick as well as on the gene: getting out
// of the way is a matter of moving.
func (a *Agent) evasion(cfg *Config) float64 {
	if cfg.MaxSpeed <= 0 {
		return 0
	}
	quick := clamp(a.MaxSpeed(cfg)/cfg.MaxSpeed, 0, 2)
	return clamp(cfg.EvasionCap*(a.Gene(GeneEvasion)/MaxAbility)*a.mix().Evasion*quick, 0, cfg.EvasionCap)
}

// stanceCost is the vitality a tick of this stance costs, before anything is
// spent on moving: each channel at what it is used at, priced separately.
func stanceCost(cfg *Config, s Stance) float64 {
	m := stanceMix[s%Stance(NumStances)]
	return cfg.AttackCost*m.Attack + cfg.DefenceCost*m.Defence + cfg.EvasionCost*m.Evasion
}
