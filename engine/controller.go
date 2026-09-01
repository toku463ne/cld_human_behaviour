package engine

import "math"

// How many candidates of each kind are worth scoring. Beyond the nearest few
// the answer never changes, and the cost of deciding does.
const (
	maxFoodOptions  = 4
	maxAgentOptions = 6
)

// Effort levels an agent chooses between. Keeping the set small keeps the
// comparison cheap while still letting an agent decide between strolling and
// sprinting, or between a jab and everything it has.
var effortLevels = [...]float64{0.4, 1.0}

// Lookahead each kind of move needs. An agent can only consider options up to
// the depth its intelligence unlocks, which is what keeps the cleverer moves
// (watching a stranger to size them up, removing a rival before the food runs
// short) out of reach of dull agents.
const (
	depthBasic     = 0 // eat, move, rest
	depthReactive  = 1 // fight over the food in front of you, run away, court
	depthObserve   = 2 // spend time now to judge better later
	depthPreemtive = 3 // remove a competitor before the competition happens

	depthMax = depthPreemtive
)

// strategyDepth is how far ahead an agent can think.
//
// Setting Config.StrategyDepthUnlock to zero turns the gate off entirely, which
// leaves intelligence acting through ChoiceNoise alone. That is the control arm
// of the experiment: the gate is a hard threshold on a continuous ability, and
// the thresholds it produces do not line up with the range abilities actually
// occupy, so it has to be possible to run the world without it.
func strategyDepth(cfg *Config, intelligence float64) int {
	if cfg.StrategyDepthUnlock <= 0 {
		return depthMax
	}
	return int(intelligence / cfg.StrategyDepthUnlock)
}

// fleeExposureTicks is how long an agent reckons it stays within reach of the
// one it is running from.
const fleeExposureTicks = 10

type option struct {
	action Action
	util   float64
}

// AIController scores every action it can think of with one formula and takes
// the best it manages to pick:
//
//	utility = LifeValue * gain in survival probability
//	        + OffspringValue * chance of offspring
//	        - vitality cost
//	        - time cost
//
// There is no threshold anywhere that says "run away" or "attack". Fleeing
// comes out of the fact that being hit shortens the agent's life expectancy,
// and pre-emptive attacks come out of the fact that a rival eats food the agent
// will need. Change how plentiful food is and the violence changes with it.
//
// The zero value is ready to use. One instance is shared by every AI agent: it
// keeps a scratch buffer and the simulation is single threaded.
type AIController struct {
	opts []option

	// terms holds the breakdown of each option in opts, and is only filled in
	// while tracing: an agent nobody is watching pays for the arithmetic but
	// not for carrying the result around, which keeps the common case as cheap
	// as it was before decisions could be explained.
	terms   []Utility
	tracing bool

	// The best meal in sight and who is in the way of it, worked out while
	// scoring the food and then reused when scoring a fight: driving that
	// rival off is worth exactly the part of the meal they are costing.
	bestFood      float64
	bestFoodGap   float64 // value that winning the race would add
	bestFoodRival int
}

func (c *AIController) Decide(p *Perception) Action {
	c.opts = c.opts[:0]
	c.terms = c.terms[:0]
	c.tracing = p.Trace != nil
	c.bestFood, c.bestFoodGap, c.bestFoodRival = 0, 0, 0
	maxDepth := strategyDepth(p.Cfg, p.Self.Intelligence)

	c.addRest(p)
	c.addExplore(p)
	c.addFood(p)
	c.addAgents(p, maxDepth)

	return c.pick(p)
}

// --- the shared utility formula --------------------------------------------

// pressure is the estimated chance of dying inside the planning horizon: the
// risk term the life goal is weighed with.
//
// Two ways to die are folded together. Starving is a countdown: at this drain,
// vitality runs out in so many ticks. Being worn down is a standing hazard:
// whatever the drain, an agent with nothing left in the tank does not survive
// the next thing that happens to it. Leaving the second one out would make
// spending vitality look free to anybody who is not currently hungry.
func pressure(cfg *Config, maxVitality, vitality, drain float64) float64 {
	if vitality <= 0 {
		return 1
	}
	pDrain := 0.0
	if drain > 0 {
		if ticksLeft := vitality / drain; ticksLeft < cfg.PlanHorizon {
			pDrain = 1 - ticksLeft/cfg.PlanHorizon
		}
	}
	pWorn := cfg.ShockRisk * (1 - clamp(vitality/maxVitality, 0, 1))
	return clamp(1-(1-pDrain)*(1-pWorn), 0, 1)
}

// recoverable is the vitality an agent can expect to win back, given how long
// it has before hunger climbs back past the satiated line. It is what makes
// eating worth something to an agent that is battered rather than hungry.
//
// Damage currently coming in is netted off first: an agent being hit for more
// than it heals recovers nothing, however well fed it is. Leaving that out
// would let "sit still and get better" look like an answer to being attacked,
// and nothing would ever run away.
func recoverable(cfg *Config, maxVitality, hungerRate, vitality, hunger, incoming float64) float64 {
	if hunger >= cfg.SatiatedHunger {
		return 0
	}
	net := cfg.RegenRate - incoming
	if net <= 0 {
		return 0
	}
	ticks := (cfg.SatiatedHunger - hunger) / hungerRate
	return math.Max(0, math.Min(maxVitality-vitality, net*ticks))
}

// hungerDrain is the vitality lost per tick at a given hunger level. It is
// negative when the agent is satiated enough to be recovering instead.
func hungerDrain(cfg *Config, hunger float64) float64 {
	if hunger >= cfg.StarveHunger {
		f := (hunger - cfg.StarveHunger) / (cfg.MaxHunger - cfg.StarveHunger)
		return cfg.StarveRate * clamp(f, 0, 1)
	}
	if hunger <= cfg.SatiatedHunger {
		return -cfg.RegenRate
	}
	return 0
}

// projectedDrain is the drain an agent expects to be living with over the
// horizon, since hunger keeps climbing while it makes up its mind.
func projectedDrain(cfg *Config, hungerRate, hunger float64) float64 {
	return hungerDrain(cfg, hunger+hungerRate*cfg.PlanHorizon*0.5)
}

// speedAt is how fast an agent of the given top speed moves at this effort.
// Speed grows with the square root of the effort while the cost grows with the
// effort itself, so hurrying is expensive per unit of distance.
func speedAt(maxSpeed, effort float64) float64 {
	return maxSpeed * math.Sqrt(effort)
}

func moveCostAt(cfg *Config, effort float64) float64 {
	return cfg.MoveCost * effort
}

// damagePerTick is what an attacker of the given power does at the given
// effort. Power is the efficiency of the vitality poured in, nothing else.
func damagePerTick(cfg *Config, power, effort float64) float64 {
	return cfg.AttackDamage * effort * power / midAbility
}

func (c *AIController) add(a Action, u Utility) {
	c.opts = append(c.opts, option{action: a, util: u.Total()})
	if c.tracing {
		c.terms = append(c.terms, u)
	}
}

// --- options ---------------------------------------------------------------

// addRest scores doing nothing. It is not a fallback: for a satiated agent it
// is the only way back to full vitality, and it costs nothing.
func (c *AIController) addRest(p *Perception) {
	cfg := p.Cfg
	s := &p.Self
	drain := projectedDrain(cfg, s.HungerRate, s.Hunger)
	incoming := c.incoming(p)
	now := pressure(cfg, s.MaxVitality, s.Vitality, drain+incoming)
	// Whatever is hitting the agent goes on hitting it while it sits there.
	after := pressure(cfg, s.MaxVitality,
		s.Vitality+recoverable(cfg, s.MaxVitality, s.HungerRate, s.Vitality, s.Hunger, incoming), drain+incoming)
	c.add(Action{Kind: ActRest}, Utility{
		Life: Goal{Value: (now - after) * cfg.LifeValue, Chance: 1},
	})
}

// mealValue is what one item of food is worth to this agent right now, in the
// same units the food options are scored in: how much less likely it makes
// dying inside the planning horizon.
func mealValue(cfg *Config, s *SelfView, incoming float64) float64 {
	now := pressure(cfg, s.MaxVitality, s.Vitality, projectedDrain(cfg, s.HungerRate, s.Hunger)+incoming)
	fed := math.Max(0, s.Hunger-cfg.FoodNutrition)
	after := pressure(cfg, s.MaxVitality, s.Vitality, projectedDrain(cfg, s.HungerRate, fed)+incoming)
	return (now - after) * cfg.LifeValue
}

// addExplore scores wandering off to look for something to eat. It is worth
// something in proportion to how hungry the agent is and nothing else:
// wandering does not mend a wound and it does not shake off an attacker, so
// scoring it against the overall risk of dying would have a cornered agent
// strolling away instead of running.
func (c *AIController) addExplore(p *Perception) {
	cfg := p.Cfg
	s := &p.Self
	hungry := clamp((s.Hunger-cfg.SatiatedHunger)/(cfg.MaxHunger-cfg.SatiatedHunger), 0, 1)
	dx, dy := 0.0, 0.0
	if p.Rand != nil {
		angle := p.Rand.Float64() * 2 * math.Pi
		dx, dy = math.Cos(angle), math.Sin(angle)
	}
	effort := 0.4
	cost := moveCostAt(cfg, effort)
	c.add(Action{Kind: ActMove, DX: dx, DY: dy, Effort: effort}, Utility{
		Explore:      Goal{Value: cfg.ExploreValue, Chance: hungry},
		Vitality:     cost,
		VitalityCost: cost * cfg.VitalityWeight,
	})
}

// addFood scores going for each item in sight. Reaching it is a race against
// whoever else is nearby, and losing that race is what sends an agent looking
// somewhere else instead of standing around.
func (c *AIController) addFood(p *Perception) {
	cfg := p.Cfg
	s := &p.Self
	drain := projectedDrain(cfg, s.HungerRate, s.Hunger)
	incoming := c.incoming(p)
	now := pressure(cfg, s.MaxVitality, s.Vitality, drain+incoming)

	for i := range p.Foods {
		if i >= maxFoodOptions {
			break
		}
		f := &p.Foods[i]

		// Odds of getting there first, modelled as a plain race.
		pGet := 1.0
		if !math.IsInf(f.RivalDist, 1) {
			pGet = clamp(f.RivalDist/(f.RivalDist+f.Dist+1e-9), 0.05, 1)
		}

		for _, effort := range effortLevels {
			ticks := f.Dist/speedAt(s.MaxSpeed, effort) + 1
			cost := moveCostAt(cfg, effort) * ticks
			hungerAfter := math.Max(0, s.Hunger+s.HungerRate*ticks-cfg.FoodNutrition)
			vitAfter := s.Vitality - cost
			vitAfter += recoverable(cfg, s.MaxVitality, s.HungerRate, vitAfter, hungerAfter, incoming)
			after := pressure(cfg, s.MaxVitality, vitAfter, projectedDrain(cfg, s.HungerRate, hungerAfter)+incoming)

			meal := (now - after) * cfg.LifeValue
			c.add(Action{Kind: ActEat, TargetID: f.ID, Effort: effort}, Utility{
				Life:         Goal{Value: meal, Chance: pGet},
				Vitality:     cost,
				Ticks:        ticks,
				VitalityCost: cost * cfg.VitalityWeight,
				TimeCost:     ticks * cfg.TimeCost,
			})

			// Remember what the race is costing: that difference is what
			// clearing the rival out of the way would buy.
			if gap := meal * (1 - pGet); gap > c.bestFoodGap && f.RivalID != 0 {
				c.bestFood, c.bestFoodGap, c.bestFoodRival = meal, gap, f.RivalID
			}
		}
	}
}

// addAgents scores everything an agent might do about the people around it.
func (c *AIController) addAgents(p *Perception, maxDepth int) {
	s := &p.Self

	for i := range p.Others {
		if i >= maxAgentOptions {
			break
		}
		o := &p.Others[i]

		if maxDepth >= depthReactive {
			c.addAttack(p, o)
			if o.AttackingMe {
				c.addFlee(p, o)
			}
			if s.CanReproduce && o.Species == s.Species && o.Sex != s.Sex && !o.Paired && !o.Rejected {
				c.addCourt(p, o)
			}
		}
		if maxDepth >= depthObserve {
			c.addObserve(p, o)
		}
	}
}

// addAttack scores picking a fight. The same option covers both reasons to
// throw a punch: taking the meal in front of you, and thinning out the
// competition before the food runs short. Whether either is worth the vitality
// is what the formula decides.
func (c *AIController) addAttack(p *Perception, o *AgentView) {
	cfg := p.Cfg
	s := &p.Self

	myScore := s.Attack * s.Vitality
	theirScore := o.EstStrength * o.Vitality
	pWin := myScore / (myScore + theirScore + 1e-9)

	maxDepth := strategyDepth(cfg, s.Intelligence)

	// Three ready mixes rather than two levels of one number: how hard to
	// swing is now inseparable from how much guard to keep up.
	for stance := Stance(0); int(stance) < NumStances; stance++ {
		m := stanceMix[stance]
		const effort = 1.0

		// What the exchange is expected to cost, assuming the other side hits
		// back with a fair share of its own effort.
		//
		// What the other side's guard would turn aside is not in here: how
		// well somebody defends is a hidden parameter like everything else
		// about them, so an agent finds out by being surprised. What it does
		// know is its own guard, which is what the incoming blow is reduced
		// by below.
		const retaliation = 0.7
		mine := damagePerTick(cfg, s.Attack, effort*m.Attack)
		theirs := damagePerTick(cfg, o.EstStrength, retaliation) *
			(1 - s.Defence*m.Defence) * (1 - s.Evasion*m.Evasion)

		// Either they go down, or one side breaks off first. A weakened
		// target is cheap to finish, which is what makes hitting somebody who
		// is already hurt the best value there is.
		exchange := math.Min(o.Vitality/math.Max(mine, 1e-9), cfg.SkirmishTicks)
		travel := o.Dist / speedAt(s.MaxSpeed, effort)
		ticks := exchange + travel

		cost := exchange*(theirs+stanceCost(cfg, stance)*effort) + travel*moveCostAt(cfg, effort)

		drain := projectedDrain(cfg, s.HungerRate, s.Hunger+s.HungerRate*ticks)
		now := pressure(cfg, s.MaxVitality, s.Vitality, projectedDrain(cfg, s.HungerRate, s.Hunger)+c.incoming(p))
		after := pressure(cfg, s.MaxVitality, s.Vitality-cost, drain)
		lifeTerm := (now - after) * cfg.LifeValue

		// The meal in front of them. Driving this one off wins the race for
		// the item they are contesting, so it is worth the part of that meal
		// the race was costing.
		stake := Goal{}
		if c.bestFoodRival == o.ID {
			stake = Goal{Value: c.bestFoodGap, Chance: pWin}
		}

		// And the meal it would itself become. A creature of a kind this one
		// eats is worth killing for the carcass, which is what makes hunting
		// something other than a fight - and what makes a large animal worth
		// more than a small one to whoever brings it down.
		//
		// It goes in as one meal rather than the whole carcass on purpose: an
		// agent can only eat so much before it is full, and the rest feeds
		// whoever else took part. That is the arithmetic that makes a big
		// animal worth taking on together and not alone.
		if o.Prey && o.Meat >= 1 && s.Hunger > 0 && cfg.PreyValue > 0 {
			bite := math.Min(o.Meat, 1) * cfg.PreyValue * mealValue(cfg, s, c.incoming(p))
			pKill := clamp(exchange*mine/math.Max(o.Vitality, 1e-9), 0, 1) * pWin
			stake = Goal{Value: stake.Value + bite, Chance: math.Max(stake.Chance, pKill)}
		}

		// Removing somebody who will be eating the same food later on. Only an
		// agent that can think that far ahead sees this at all, and a world
		// with food to spare makes the term vanish on its own.
		competition := Goal{}
		if maxDepth >= depthPreemtive {
			competition = Goal{
				Value:  cfg.CompetitionWeight * cfg.LifeValue * clamp(s.FoodScarcity, 0, 3) / 3,
				Chance: pWin,
			}
		}

		c.add(Action{Kind: ActAttack, TargetID: o.ID, Effort: effort, Stance: stance}, Utility{
			Life:         Goal{Value: lifeTerm, Chance: 1},
			Stake:        stake,
			Rival:        competition,
			Risk:         cfg.RiskWeight * o.Risk,
			Vitality:     cost,
			Ticks:        ticks,
			VitalityCost: cost * cfg.VitalityWeight,
			TimeCost:     ticks * cfg.TimeCost,
		})
	}
}

// addFlee scores running away. Nothing here says "flee when hurt": what the
// formula sees is that the damage coming in shortens the agent's life, and
// that walking away from it is worth more the closer to death it is.
func (c *AIController) addFlee(p *Perception, o *AgentView) {
	cfg := p.Cfg
	s := &p.Self

	drain := projectedDrain(cfg, s.HungerRate, s.Hunger)
	incoming := damagePerTick(cfg, o.EstStrength, 1)
	staying := pressure(cfg, s.MaxVitality, s.Vitality, drain+incoming)

	// Breaking away is not free: for a while the agent is still in reach and
	// is the one not hitting back, which is the cheapest thing there is to
	// hit. This is the only option that gets the incoming damage out of the
	// picture, which is why running away wins exactly when the damage is what
	// is about to kill the agent, and loses whenever it is not.
	cost := moveCostAt(cfg, cfg.FleeEffort)*fleeExposureTicks + incoming*fleeExposureTicks*0.4
	pEscape := clamp(s.Vitality/(s.Vitality+o.Vitality+1e-9), 0.15, 0.9)
	fled := pressure(cfg, s.MaxVitality, s.Vitality-cost, drain)

	c.add(Action{Kind: ActFlee, TargetID: o.ID, Effort: cfg.FleeEffort, Stance: StanceEvasive}, Utility{
		Life:         Goal{Value: (staying - fled) * cfg.LifeValue, Chance: pEscape},
		Vitality:     cost,
		Ticks:        fleeExposureTicks,
		VitalityCost: cost * cfg.VitalityWeight,
		TimeCost:     fleeExposureTicks * cfg.TimeCost,
	})
}

// addCourt scores going after a mate: priority 2, and only ever reachable once
// priority 1 is comfortable.
func (c *AIController) addCourt(p *Perception, o *AgentView) {
	cfg := p.Cfg
	const pAccept = 0.6

	effort := 0.6
	ticks := o.Dist/speedAt(p.Self.MaxSpeed, effort) + 1
	cost := moveCostAt(cfg, effort) * ticks

	c.add(Action{Kind: ActCourt, TargetID: o.ID, Effort: effort}, Utility{
		Offspring:    Goal{Value: cfg.OffspringValue * clamp(o.Fitness/MaxAbility, 0, 1), Chance: pAccept},
		Vitality:     cost,
		Ticks:        ticks,
		VitalityCost: cost * cfg.VitalityWeight,
		TimeCost:     ticks * cfg.TimeCost,
	})
}

// addObserve scores spending a moment sizing somebody up instead of acting.
// Knowing who you are standing next to only pays when you might have to
// contest something with them, so a world with food to spare makes this
// worthless and a tight one makes it worth the pause.
func (c *AIController) addObserve(p *Perception, o *AgentView) {
	cfg := p.Cfg
	unsure := clamp(o.Uncertainty/cfg.PriorVariance, 0, 1)
	relevance := clamp(p.Self.FoodScarcity, 0, 3) / 3
	c.add(Action{Kind: ActObserve, TargetID: o.ID, Effort: 0.3}, Utility{
		Info:     Goal{Value: cfg.InfoValue * unsure, Chance: relevance},
		Ticks:    observeTicks,
		TimeCost: observeTicks * cfg.TimeCost,
	})
}

// incoming is the damage the agent believes is currently landing on it.
func (c *AIController) incoming(p *Perception) float64 {
	if p.Self.AttackerID == 0 {
		return 0
	}
	for i := range p.Others {
		if p.Others[i].ID == p.Self.AttackerID {
			return damagePerTick(p.Cfg, p.Others[i].EstStrength, 1)
		}
	}
	return damagePerTick(p.Cfg, p.Cfg.PriorStrength, 1)
}

// --- choosing --------------------------------------------------------------

// pick takes the option that scored best, as far as the agent can tell them
// apart. A dull agent's scores are noisy, so it regularly settles for a worse
// move; the noise is proportional, so it mixes up two similar options often and
// only rarely talks itself into something disastrous.
//
// This is intelligence, not rationality: rationality already blurred what the
// agent believes about the world before any of this was scored.
func (c *AIController) pick(p *Perception) Action {
	if len(c.opts) == 0 {
		return Action{Kind: ActRest}
	}

	noise := (MaxAbility - p.Self.Intelligence) / MaxAbility * p.Cfg.ChoiceNoise
	best, bestScore := 0, math.Inf(-1)
	for i := range c.opts {
		misjudged := 0.0
		if noise > 0 && p.Rand != nil {
			misjudged = p.Rand.NormFloat64() * noise
		}
		score := c.opts[i].util + misjudged
		if score > bestScore {
			bestScore, best = score, i
		}
		// Recording the whole comparison, and not merely its winner, is what
		// makes a decision reviewable: it shows the runners up and by how much
		// they lost. Only agents somebody asked to follow have a trace.
		if c.tracing && i < len(c.terms) {
			p.Trace.Options = append(p.Trace.Options, TracedOption{
				Action:  c.opts[i].action,
				Utility: c.terms[i],
				Noise:   misjudged,
				Score:   score,
			})
		}
	}
	if p.Trace != nil {
		p.Trace.Chosen = best
	}
	return c.opts[best].action
}
