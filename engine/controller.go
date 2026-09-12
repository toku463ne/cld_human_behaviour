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

	// What the neighbours mean, worked out once in survey and read by
	// everything after it: the damage currently landing, and how much of the
	// neighbourhood would be free to land on the agent if it stopped watching
	// them.
	incomingDmg float64
	exposure    float64

	// What a tick spent on this ground may cost (stage 34): the chance of
	// drowning where the agent is standing, and what a life is worth. Zero
	// everywhere but in the water, and zero in every world with no map, which
	// is why nothing about a flat world changed.
	drownChance float64

	// Where home is and how far out this body is (stage 64), in the width of
	// a region, with the direction and speed needed to charge an option for
	// where it would take the body.
	homePull  float64
	homeAway  float64
	homeUX    float64
	homeUY    float64
	homeSpan  float64
	homeSpeed float64
	lifeValue   float64

	// Which option, if any, was the one that goes to better country (stage
	// 15b), and whether it won. Measurement only: no rule reads it, and it
	// exists because a belief about a place can only reach a body through
	// this one option, so how often that option is taken is the ceiling on
	// what any such belief can do (stage 35).
	betterGroundOpt   int
	ChoseBetterGround bool

	// The same pair for the walk after whoever went out of sight (stage 55a),
	// and for the same reason: a remembered direction can only reach a body
	// through this one option, so how often it wins is the ceiling on what
	// the whole rule can do.
	lonelyOpt    int
	ChoseMissing bool

	// And for the walk back to the country it came into the world in (stage
	// 64). Same reason again: it is the one option the rule can reach a body
	// through, so how often it wins is the ceiling on what the rule can do.
	homeOpt   int
	ChoseHome bool

	// Who has declared for what, worked out once in survey (stage 32): for
	// each target somebody has called about or is already hitting, the
	// trust-weighted strength and damage that side of the fight can count on.
	// Empty in a world with nobody calling and nobody fighting, which is most
	// of the time.
	allies []allyForce

	// Whether the chosen action was a fight somebody else had declared for.
	// Measurement only, like ChoseBetterGround.
	JoinedDeclared bool

	// Which options, if any, were walks towards somebody crying their wares
	// (stage 49), and whether one of them won. Measurement only, and for the
	// same reason ChoseBetterGround is measured: an advertisement can only
	// reach a body through this option, so how often it is taken is the
	// ceiling on what advertising can do.
	offerOpts   []int
	WentToOffer bool

	// The best hand-over in sight and how far off it is, worked out while
	// scoring the gifts and read by the cry that would arrange one (stage 49).
	bestGiftGain float64
	bestGiftDist float64

	// The deciding agent's rules of thumb and the situation they read (stage
	// 12c). The situation is filled in once for the agent's own state and
	// again for whoever each option is aimed at, so a hint about a stranger's
	// strength says nothing about an option with no stranger in it.
	hints []Hint
	feats hintFeatures
}

func (c *AIController) Decide(p *Perception) Action {
	c.opts = c.opts[:0]
	c.terms = c.terms[:0]
	c.tracing = p.Trace != nil
	c.bestFood, c.bestFoodGap, c.bestFoodRival = 0, 0, 0
	c.betterGroundOpt, c.ChoseBetterGround = -1, false
	c.lonelyOpt, c.ChoseMissing = -1, false
	c.homeOpt, c.ChoseHome = -1, false
	c.offerOpts, c.WentToOffer = c.offerOpts[:0], false
	c.bestGiftGain, c.bestGiftDist = 0, 0
	c.allies, c.JoinedDeclared = c.allies[:0], false
	// readSelf clears the whole situation, so the options scored before any
	// target is read (rest, wandering, food) see nothing about a target.
	c.hints = p.Self.Hints
	c.feats.readSelf(p)
	c.drownChance, c.lifeValue = p.Self.Drown, p.Cfg.LifeValue
	// And how far from home this body is (stage 64), in the width of a
	// region, with the direction home kept so that an option can be charged
	// for where it would take the body rather than only for where it is.
	c.homePull, c.homeAway, c.homeUX, c.homeUY = 0, 0, 0, 0
	if p.Self.HomePull > 0 {
		dx, dy := p.Self.X-p.Self.HomeX, p.Self.Y-p.Self.HomeY
		if d := math.Hypot(dx, dy); d > 1e-9 {
			span := max(p.Cfg.Width/float64(max(p.Cfg.RegionCols, 1)), 1)
			c.homePull = p.Self.HomePull
			c.homeAway = d / span
			c.homeUX, c.homeUY = dx/d, dy/d
			c.homeSpan = span
			c.homeSpeed = p.Self.MaxSpeed
		}
	}
	maxDepth := strategyDepth(p.Cfg, p.Self.Intelligence)

	c.survey(p)
	c.addRest(p)
	c.addExplore(p)
	c.addFood(p)
	c.addStones(p)
	c.addCoins(p)
	c.addPutInStore(p)
	c.addCook(p)
	c.addAgents(p, maxDepth)
	c.addOffer(p)

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
func pressure(cfg *Config, s *SelfView, vitality, drain float64) float64 {
	if vitality <= 0 {
		return 1
	}
	pDrain := 0.0
	if drain > 0 {
		if ticksLeft := vitality / drain; ticksLeft < cfg.PlanHorizon {
			pDrain = 1 - ticksLeft/cfg.PlanHorizon
		}
	}
	pWorn := s.ShockRisk * (1 - clamp(vitality/s.MaxVitality, 0, 1))
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
func recoverable(cfg *Config, maxVitality, hungerRate, vitality, hunger, incoming, regen float64) float64 {
	if hunger >= cfg.SatiatedHunger {
		return 0
	}
	net := regen - incoming
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

// groundOf is what the ground under this body multiplies movement by, and
// burdenOf what its load does (stage 40), with the guard the rest of the file
// uses for a view that never had one set.
func groundOf(s *SelfView) float64 {
	if s.Ground <= 0 {
		return 1
	}
	return s.Ground
}

func burdenOf(s *SelfView) float64 {
	if s.Burden <= 0 {
		return 1
	}
	return s.Burden
}

// burdenWith is what the load would multiply movement by with one more item
// in hand. The difference between the two is the price of picking something
// up: not what it costs to reach it, but what it costs to carry it until it
// is eaten.
func burdenWith(cfg *Config, s *SelfView, more int) float64 {
	if s.CarryCapacity <= 0 || cfg.CarryCost <= 0 {
		return burdenOf(s)
	}
	load := clamp(float64(s.Carried+more)/s.CarryCapacity, 0, 1)
	return 1 + cfg.CarryCost*load
}

func moveCostAt(cfg *Config, effort float64) float64 {
	return cfg.MoveCost * effort
}

// moveCost is what the agent doing the reckoning expects a tick of movement to
// cost: the flat figure, times the ground under its own feet (stage 20).
//
// It assumes the country ahead is like the country it is standing on, which is
// the most an animal can do without a map. On level open ground - the whole
// world before terrain, and the default still - Ground is 1 and this is the
// flat figure exactly, so nothing about a flat world changed when this arrived.
func moveCost(cfg *Config, s *SelfView, effort float64) float64 {
	g := s.Ground
	if g <= 0 {
		g = 1
	}
	b := s.Burden
	if b <= 0 {
		b = 1
	}
	return moveCostAt(cfg, effort) * g * b
}

// damagePerTick is what an attacker of the given power does at the given
// effort. Power is the efficiency of the vitality poured in, nothing else.
func damagePerTick(cfg *Config, power, effort float64) float64 {
	return cfg.AttackDamage * effort * power / midAbility
}

func (c *AIController) add(a Action, u Utility) {
	// The one place a rule of thumb touches a decision, and all it does is
	// add to the score. Nothing branches on it.
	u.Hint = c.feats.score(c.hints, a.Kind)

	// And the one place the ground's own danger touches a decision (stage
	// 34). It is charged per tick the option is expected to take, so what
	// tells the options apart is how long each would keep the body in the
	// water - which is the only thing an agent that cannot see the far bank
	// has to go on. An option that names no duration is charged for the one
	// tick it is about to spend.
	if c.drownChance > 0 {
		ticks := u.Ticks
		if ticks < 1 {
			ticks = 1
		}
		u.Hazard = clamp(c.drownChance*ticks, 0, 1) * c.lifeValue
	}

	// And what being away from home costs it (stage 64), charged the same
	// way: per tick, and worse the further out. What tells the options apart
	// is the second half - an option that carries the body further out is
	// charged for where it would be, and one that heads back is charged less,
	// which is the whole of "it stops being worth it" without a threshold
	// anywhere.
	//
	// Free inside its own region: home is a block, not a spot (#53).
	if c.homePull > 0 {
		ticks := u.Ticks
		if ticks < 1 {
			ticks = 1
		}
		away := c.homeAway
		if a.Kind == ActMove && c.homeSpan > 0 {
			// How much further out this heading would take it, in the same
			// units, over the ticks it would take.
			radial := a.DX*c.homeUX + a.DY*c.homeUY
			away += radial * c.homeSpeed * a.Effort * ticks / c.homeSpan
		}
		if away > 0.5 {
			u.Roam = c.homePull * (away - 0.5) * ticks
		}
	}
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
	incoming := c.incomingDmg
	now := pressure(cfg, s, s.Vitality, drain+incoming)

	// Lying down among strangers is not the same as lying down among your
	// own. What it costs is worked out the way every other few ticks ahead is
	// worked out: one trajectory, no branching, today's state carried forward
	// - here, the share of the surrounding strength that would be free to land
	// on an agent that has stopped watching for it.
	// ... and how much of that this particular patch of ground lets through
	// (stage 14). Somewhere with its back covered is somewhere the same
	// neighbours matter less. It is one multiplication and there is no second
	// formula: the whole of "a good place to rest" is this.
	exposed := cfg.RestExposureWeight * s.Shelter * c.exposure

	// Whatever is hitting the agent goes on hitting it while it sits there,
	// and so does whatever starts while it is down.
	after := pressure(cfg, s,
		s.Vitality+recoverable(cfg, s.MaxVitality, s.HungerRate, s.Vitality, s.Hunger, incoming+exposed, s.RestRate),
		drain+incoming+exposed)
	c.add(Action{Kind: ActRest}, Utility{
		Life: Goal{Value: (now - after) * cfg.LifeValue, Chance: 1},
	})
}

// mealValue is what one item of food is worth to this agent right now, in the
// same units the food options are scored in: how much less likely it makes
// dying inside the planning horizon.
// nutrition is what one of it is worth to this agent: the world's figure, less
// whatever it has been living on lately (stage 16).
// heal is what it would mend as well (stage 39), which is nothing for every
// kind of food but a carcass in a world where carcasses mend anything. It is
// capped at what the body is actually missing, and that cap is the whole of
// why nobody had to be told to save meat for later: a body that is nearly
// whole gets almost nothing from it and scores a plant higher.
func mealValue(cfg *Config, s *SelfView, incoming, nutrition, heal float64) float64 {
	return mealValueAt(cfg, s, incoming, s.Hunger, nutrition, heal)
}

// mealValueAt is the same figure reckoned from a hunger this body is not at
// yet, which is what a thing kept for later is worth: not the meal it would be
// today, but the meal it would be at the moment there is nothing about.
//
// It exists because that value was being worked out the wrong way round.
// Stage 40 wrote it as the difference between now and the moment of running
// short - which is how much worse things get, a loss - where what it meant was
// the good that eating then would do. Measured before the fix: of 138,310
// carry options scored in one run, not one had a positive value, so nothing
// was ever picked up on purpose. CarryPricedBackwards puts that world back.
func mealValueAt(cfg *Config, s *SelfView, incoming, hunger, nutrition, heal float64) float64 {
	before := pressure(cfg, s, s.Vitality, projectedDrain(cfg, s.HungerRate, hunger)+incoming)
	fed := math.Max(0, hunger-cfg.FoodNutrition*nutrition)
	mended := math.Min(s.Vitality+heal, s.MaxVitality)
	after := pressure(cfg, s, mended, projectedDrain(cfg, s.HungerRate, fed)+incoming)
	return (before - after) * cfg.LifeValue
}

// keepValue is what having this item when it is needed is worth. One place,
// because two options are the same bet: carrying it (stage 40) and putting it
// in a cache (stage 50).
func keepValue(cfg *Config, s *SelfView, incoming, nutrition, heal float64) float64 {
	if cfg.CarryPricedBackwards {
		// The world as stages 40 to 49 measured it, kept so that those
		// figures can be reproduced: the loss from getting hungrier, with the
		// sign that made every such option a penalty.
		now := pressure(cfg, s, s.Vitality, projectedDrain(cfg, s.HungerRate, s.Hunger)+incoming)
		later := pressure(cfg, s, s.Vitality,
			projectedDrain(cfg, s.HungerRate, math.Max(s.Hunger, cfg.StarveHunger))+incoming)
		return (now - later) * cfg.LifeValue
	}
	return mealValueAt(cfg, s, incoming, math.Max(s.Hunger, cfg.StarveHunger), nutrition, heal)
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
	cost := moveCost(cfg, s, effort)
	c.add(Action{Kind: ActMove, DX: dx, DY: dy, Effort: effort}, Utility{
		Explore:      Goal{Value: cfg.ExploreValue, Chance: hungry},
		Vitality:     cost,
		VitalityCost: cost * cfg.VitalityWeight,
	})

	// And, for an agent that has been somewhere better than this, going back
	// to it (stage 15b). This is the difference between finding good ground by
	// accident and going to it: everything before this stage could only stay
	// where it happened to be doing well.
	//
	// It is scored as one more option in the same comparison, not as a plan.
	// The agent proposes a direction for one step; whether it keeps going that
	// way is decided again next time it thinks, with whatever it has seen
	// since. Nothing here searches a route or commits to a destination.
	// And, for one that has lost sight of whoever it thinks best of, the way
	// they went (stage 55a). It is the same shape as the walk to better
	// country directly below - one more direction to propose, scored and
	// costed like any other - and the chance on it is how much the direction
	// is still worth, which falls as it ages.
	//
	// Nothing here is about children or mates. What it follows is the highest
	// affinity this body holds, which is as often somebody it hunted with as
	// somebody it is related to.
	if s.Missing > 0 && cfg.LonelyValue > 0 {
		c.lonelyOpt = len(c.opts)
		c.add(Action{Kind: ActMove, DX: s.MissingDX, DY: s.MissingDY, Effort: effort}, Utility{
			Lore:         Goal{Value: cfg.LonelyValue, Chance: s.Missing},
			Vitality:     cost,
			VitalityCost: cost * cfg.VitalityWeight,
		})
	}

	// And, for a body that has wandered out of the country it came into the
	// world in, the way back (stage 64). One more direction to propose, in
	// the same shape as the two above.
	//
	// It carries no value of its own: what makes it win is that add() charges
	// every option for where it would take the body, so heading home is the
	// cheap one and everything else is dear. Without it the charge would only
	// make a body far from home dislike all of its options equally, which is
	// the failure stage 55a found from the other side - a cost with nothing
	// to spend it on changes nothing.
	if s.HomePull > 0 {
		hdx, hdy := s.HomeX-s.X, s.HomeY-s.Y
		if d := math.Hypot(hdx, hdy); d > 1e-9 {
			c.homeOpt = len(c.opts)
			c.add(Action{Kind: ActMove, DX: hdx / d, DY: hdy / d, Effort: effort}, Utility{
				Vitality:     cost,
				VitalityCost: cost * cfg.VitalityWeight,
			})
		}
	}

	if s.BetterGround > 0 && cfg.RegionDrawValue > 0 {
		ddx, ddy := s.BetterGroundX-s.X, s.BetterGroundY-s.Y
		if d := math.Hypot(ddx, ddy); d > 1e-9 {
			c.betterGroundOpt = len(c.opts)
			c.add(Action{Kind: ActMove, DX: ddx / d, DY: ddy / d, Effort: effort}, Utility{
				Explore:      Goal{Value: cfg.RegionDrawValue * s.BetterGround, Chance: hungry},
				Vitality:     cost,
				VitalityCost: cost * cfg.VitalityWeight,
			})
		}
	}
}

// addFood scores going for each item in sight. Reaching it is a race against
// whoever else is nearby, and losing that race is what sends an agent looking
// somewhere else instead of standing around.
func (c *AIController) addFood(p *Perception) {
	cfg := p.Cfg
	s := &p.Self
	drain := projectedDrain(cfg, s.HungerRate, s.Hunger)
	incoming := c.incomingDmg
	now := pressure(cfg, s, s.Vitality, drain+incoming)

	// When this body would run short, and how much worse off it would be
	// then (stage 40). Carrying is not about being fed now - it is about
	// being fed when there is nothing about - so what a held item is worth is
	// the meal it would be at that moment, not the meal it would be today.
	wait := 0.0
	if s.HungerRate > 0 {
		wait = clamp((cfg.StarveHunger-s.Hunger)/s.HungerRate, 0, cfg.PlanHorizon)
	}
	for i := range p.Foods {
		if i >= maxFoodOptions {
			break
		}
		f := &p.Foods[i]

		// Whoever gets there first is whoever takes less time to arrive, which
		// is not the same question as who is nearer. The agent knows how fast
		// it is; it knows nothing about the other one's legs - speed is a
		// hidden ability like every other - so it assumes an ordinary body,
		// the same shape of assumption the population prior makes about a
		// stranger's strength. Being faster than average is what wins races,
		// and it is the first thing the gene has ever bought.
		//
		// Both sides are assumed to be travelling the same way, so the effort
		// cancels and only the difference in legs is left. Scoring the agent's
		// own effort against a rival assumed to be sprinting was tried and is
		// much worse: the one nearby is only the nearest agent, who may well
		// be asleep, and treating every one of them as racing makes strolling
		// over to a contested item look hopeless. It cost two thirds of the
		// population (see HISTORY.md).
		// Whether it can be landed at all (stage 43). A fish reached is not a
		// fish taken, and the chance of it depends on where this body would
		// be standing and what it has learned about fishing there.
		pGet := 1.0
		if f.Catch > 0 && f.Catch < 1 {
			pGet = f.Catch
		}
		if !math.IsInf(f.RivalDist, 1) {
			if cfg.RaceOnDistance {
				pGet *= clamp(f.RivalDist/(f.RivalDist+f.Dist+1e-9), 0.05, 1)
			} else {
				mine := f.Dist / s.MaxSpeed
				theirs := f.RivalDist / cfg.MaxSpeed
				pGet *= clamp(theirs/(theirs+mine+1e-9), 0.05, 1)
			}
		}

		for _, effort := range effortLevels {
			ticks := f.Dist/speedAt(s.MaxSpeed, effort) + 1

			cost := moveCost(cfg, s, effort) * ticks
			hungerAfter := math.Max(0, s.Hunger+s.HungerRate*ticks-cfg.FoodNutrition*f.Nutrition)
			vitAfter := s.Vitality - cost
			// What the item itself mends, up to what is missing (stage 39).
			// It goes in before the resting estimate for the same reason the
			// cost of walking does: it is what the body would have when it
			// got there.
			vitAfter = math.Min(vitAfter+f.Heal, s.MaxVitality)
			vitAfter += recoverable(cfg, s.MaxVitality, s.HungerRate, vitAfter, hungerAfter, incoming, s.RestRate)
			after := pressure(cfg, s, vitAfter, projectedDrain(cfg, s.HungerRate, hungerAfter)+incoming)

			// What the warning on it says it will cost this body. The agent
			// is reading a signal and believing it - nothing here can tell
			// whether the plant meant it, which is the opening a liar would
			// need - but what the warning is worth depends on the stomach
			// hearing it (stage 38b).
			poison := f.Danger * cfg.PoisonDamage * (1 - s.PoisonResist)
			meal := (now - after) * cfg.LifeValue
			c.add(Action{Kind: ActEat, TargetID: f.ID, Effort: effort}, Utility{
				Life:         Goal{Value: meal, Chance: pGet},
				Vitality:     cost + poison*pGet,
				Ticks:        ticks,
				VitalityCost: (cost + poison*pGet) * cfg.VitalityWeight,
				TimeCost:     ticks * cfg.TimeCost,
			})

			// Remember what the race is costing: that difference is what
			// clearing the rival out of the way would buy.
			if gap := meal * (1 - pGet); gap > c.bestFoodGap && f.RivalID != 0 {
				c.bestFood, c.bestFoodGap, c.bestFoodRival = meal, gap, f.RivalID
			}

			// Or pick it up and eat it when it is needed (stage 40). Same
			// walk, same race - what changes is when the meal happens, so it
			// is scored as the meal the body would be having at the moment it
			// runs short, discounted for the chance that it finds something
			// by then, or does not live to need it.
			//
			// What it will cost to lug is charged here rather than left to be
			// felt afterwards: a body knows its own hands, and it knows
			// roughly how long it is until it eats. One trajectory, no
			// branching - the same shape as every other estimate here.
			if !f.Held && s.CarryRoom && cfg.CarryValue > 0 {
				// How likely it is to be needed. A body in a patch with more
				// food than neighbours has little use for a berry in its
				// hand; one where the food is contested may well find nothing
				// when it next looks. FoodScarcity is the only reading it has
				// of that, and it is normalised the way the competition term
				// already normalises it.
				need := cfg.CarryValue * clamp(s.FoodScarcity, 0, 3) / 3
				keep := keepValue(cfg, s, incoming, f.Nutrition, f.Heal)
				lug := (burdenWith(cfg, s, 1) - burdenOf(s)) *
					moveCostAt(cfg, effort) * groundOf(s) * wait
				c.add(Action{Kind: ActTake, TargetID: f.ID, Effort: effort}, Utility{
					Life:         Goal{Value: keep, Chance: pGet * need},
					Vitality:     cost + lug + poison*pGet,
					Ticks:        ticks,
					VitalityCost: (cost + lug + poison*pGet) * cfg.VitalityWeight,
					TimeCost:     ticks * cfg.TimeCost,
				})
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
		// Every option below is aimed at this one, so the half of the
		// situation that is about the target is read once here.
		c.feats.readTarget(p, o)

		if maxDepth >= depthReactive {
			c.addAttack(p, o)
			c.addThrow(p, o)
			c.addGive(p, o)
			c.addGoToOffer(p, o)
			c.addBuy(p, o)
			if o.Prey && o.Meat >= 1 {
				c.addInvite(p, o)
			}
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

// addGive scores handing what is in this body's hand to somebody (stage 48).
//
// What it is worth is what being on better terms with them is worth, which is
// the figure the world already has for that: the same LoreValue that makes
// standing with somebody worth the pause, times the trust the gift would buy.
// Nothing new prices a gift.
//
// Two things fall out of using that figure rather than a new one. Trust
// saturates, so a gift to somebody already close is worth nothing and a gift
// to a stranger is worth the most - which is the direction an economy would
// need. And the meal being given away is not subtracted here: eating it is a
// separate option, scored at no distance at all, so the comparison decides
// between them the way it decides everything else.
func (c *AIController) addGive(p *Perception, o *AgentView) {
	cfg, s := p.Cfg, &p.Self
	if s.Carried == 0 || cfg.AffinityGift <= 0 || cfg.AffinityTrust <= 0 || !o.CarryRoom {
		return
	}
	gained := clamp((o.Affinity+cfg.AffinityGift)/cfg.AffinityTrust, 0, 1) -
		clamp(o.Affinity/cfg.AffinityTrust, 0, 1)
	if gained <= 0 {
		return
	}
	// The best hand-over in sight, kept for the cry that would arrange one
	// instead of walking to it (stage 49). It is worked out here rather than
	// again because it is the same question.
	if gained > c.bestGiftGain {
		c.bestGiftGain, c.bestGiftDist = gained, o.Dist
	}
	for _, effort := range effortLevels {
		ticks := o.Dist/speedAt(s.MaxSpeed, effort) + 1
		cost := moveCost(cfg, s, effort) * ticks
		c.add(Action{Kind: ActGive, TargetID: o.ID, Effort: effort}, Utility{
			Lore:         Goal{Value: cfg.LoreValue * gained, Chance: 1},
			Vitality:     cost,
			Ticks:        ticks,
			VitalityCost: cost * cfg.VitalityWeight,
			TimeCost:     ticks * cfg.TimeCost,
		})
	}
}

// addOffer scores standing there and crying what is in the hand (stage 49).
//
// It is worth the same thing a gift is worth - the trust it buys - because it
// is the same hand-over, arranged rather than walked to. What tells the two
// apart is the price and the chance. Walking to somebody costs vitality and
// arrives for certain; crying costs only time, and comes off if whoever would
// come can get here before the cry stops.
//
// That chance is an assumption and not a fact, in the shape this world already
// uses for the legs of somebody else: how fast a stranger walks is hidden, so
// an ordinary body's speed is assumed, exactly as the race for a piece of food
// assumes it. Nothing here knows whether anybody wants what is being held up.
// If they do not come, the cry was time spent for nothing, and the body is
// asked again with the distance unchanged - so a cry that is not worth making
// is not made twice for a different reason.
func (c *AIController) addOffer(p *Perception) {
	cfg, s := p.Cfg, &p.Self
	if cfg.OfferTicks <= 0 || s.Carried == 0 || c.bestGiftGain <= 0 {
		return
	}
	ticks := float64(cfg.OfferTicks)
	comes := clamp(ticks*cfg.MaxSpeed/math.Max(c.bestGiftDist, 1e-9), 0, 1)
	c.add(Action{Kind: ActOffer}, Utility{
		Lore:     Goal{Value: cfg.LoreValue * c.bestGiftGain, Chance: comes},
		Ticks:    ticks,
		TimeCost: ticks * cfg.TimeCost,
	})
}

// addGoToOffer scores walking over to somebody holding something up (stage
// 49). It is the other half of the cry, and without it a cry is a noise made
// at bodies with no reason to answer it.
//
// No new word is needed for it: heading somewhere because there is thought to
// be something there is what stage 15b's walk to better country already is,
// and this is the same option with a body at the end of it instead of a
// region. What happens when it arrives is not arranged here - the one holding
// the item scores handing it over the way it scores everything else - so the
// chance on this is what the walker can actually reckon: whether the cry will
// still be running when it gets there.
func (c *AIController) addGoToOffer(p *Perception, o *AgentView) {
	cfg, s := p.Cfg, &p.Self
	if !o.Offering || o.OfferLeft <= 0 || !s.CarryRoom || o.Dist <= 1e-9 {
		return
	}
	incoming := c.incomingDmg
	now := pressure(cfg, s, s.Vitality, projectedDrain(cfg, s.HungerRate, s.Hunger)+incoming)
	dx, dy := (o.X-s.X)/o.Dist, (o.Y-s.Y)/o.Dist
	for _, effort := range effortLevels {
		// Whoever else heard the cry is walking for it too, and it goes into
		// one pair of hands. That is the same race as a piece of food on the
		// ground, judged the same way: my legs against an ordinary body's,
		// because a stranger's speed is hidden here as it is everywhere.
		//
		// Without it this option was worth about five times what it came to:
		// measured before the term went in, one walk in five ended with
		// anything being handed over.
		pGet := 1.0
		if !math.IsInf(o.OfferRivalDist, 1) && o.OfferRivalDist > 0 {
			mine, theirs := o.Dist/s.MaxSpeed, o.OfferRivalDist/cfg.MaxSpeed
			pGet = clamp(theirs/(theirs+mine+1e-9), 0.05, 1)
		}
		ticks := o.Dist/speedAt(s.MaxSpeed, effort) + 1
		cost := moveCost(cfg, s, effort) * ticks
		hungerAfter := math.Max(0, s.Hunger+s.HungerRate*ticks-cfg.FoodNutrition*o.OfferValue)
		vitAfter := math.Min(s.Vitality-cost+o.OfferHeal, s.MaxVitality)
		vitAfter += recoverable(cfg, s.MaxVitality, s.HungerRate, vitAfter, hungerAfter, incoming, s.RestRate)
		after := pressure(cfg, s, vitAfter, projectedDrain(cfg, s.HungerRate, hungerAfter)+incoming)
		c.offerOpts = append(c.offerOpts, len(c.opts))
		c.add(Action{Kind: ActMove, DX: dx, DY: dy, Effort: effort}, Utility{
			Life:         Goal{Value: (now - after) * cfg.LifeValue, Chance: pGet * clamp(float64(o.OfferLeft)/ticks, 0, 1)},
			Vitality:     cost,
			Ticks:        ticks,
			VitalityCost: cost * cfg.VitalityWeight,
			TimeCost:     ticks * cfg.TimeCost,
		})
	}
}

// addPutInStore scores walking to a cache and leaving what is in the hand
// there (stage 50).
//
// What it is worth is the same bet picking something up is (stage 40): the
// meal this body would be having at the moment it runs short, discounted for
// the chance it finds something else by then. What differs is who can take it
// in the meantime. Food in a hand is nobody else's; food in a cache is there
// for everybody who knows the place, which is why the discount is its own
// figure (StoreValue) rather than CarryValue.
//
// What it buys against carrying is the weight: a body walking with its hands
// empty pays no burden, and a body with its hands empty can pick up the next
// thing it sees. Neither is written here as a bonus - both fall out of the
// comparison, because putting the item down ends the burden the carry option
// charged for and frees the room the carry option needed.
func (c *AIController) addPutInStore(p *Perception) {
	cfg, s := p.Cfg, &p.Self
	if len(p.Stores) == 0 || s.Carried == 0 || cfg.StoreValue <= 0 {
		return
	}
	// What is in the hand, since what it is worth keeping depends on what it
	// is. The held items are in the food list with nothing between the body
	// and them (stage 40), and it is the first of them that ActStore puts
	// away.
	held := -1
	for i := range p.Foods {
		if p.Foods[i].Held {
			held = i
			break
		}
	}
	if held < 0 {
		return
	}
	incoming := c.incomingDmg
	// How likely it is to be wanted at all, read the same way the reason to
	// pick something up is read: a body in a patch with more food than
	// neighbours has little use for a cache, and one where every plant is
	// contested may well find nothing when it next looks.
	need := cfg.StoreValue * clamp(s.FoodScarcity, 0, 3) / 3
	keep := keepValue(cfg, s, incoming, p.Foods[held].Nutrition, p.Foods[held].Heal)
	if keep <= 0 || need <= 0 {
		return
	}
	for i := range p.Stores {
		st := &p.Stores[i]
		if st.Room <= 0 {
			continue
		}
		for _, effort := range effortLevels {
			ticks := st.Dist/speedAt(s.MaxSpeed, effort) + 1
			cost := moveCost(cfg, s, effort) * ticks
			c.add(Action{Kind: ActStore, TargetID: st.Index, Effort: effort}, Utility{
				Life:         Goal{Value: keep, Chance: need},
				Vitality:     cost,
				Ticks:        ticks,
				VitalityCost: cost * cfg.VitalityWeight,
				TimeCost:     ticks * cfg.TimeCost,
			})
		}
	}
}

// addCook scores making something of what is in the hand (stage 52).
//
// It is priced with nothing new. What cooking does is raise what one item
// mends, and what mending is worth is already the difference between two
// readings of the same meal - so this is that meal scored twice, once as it is
// and once as it would be, and the option is the gap between them.
//
// That gap is the whole reason this stage exists. Everything stages 50 and 51
// could offer a body was worth the gradient of present death risk, and that
// gradient is flat in a body that is not hungry - which is every body with
// anything to spare. Mending is not read off hunger: it is read off what the
// body is missing, capped there. So a full and battered body, which is the
// commonest sort to be holding something, has a use for this when it has a use
// for nothing else.
//
// Nothing here knows about trade. A body cooks because a cooked meal is worth
// more to it, and what makes cooking the beginning of an exchange is that the
// same item is then worth more to somebody else as well - most of all to
// somebody hurt, which the body holding it may well not be. Comparative
// advantage, out of the ceiling and not out of a rule about markets.
func (c *AIController) addCook(p *Perception) {
	cfg, s := p.Cfg, &p.Self
	if !s.CanCook || cfg.CookTicks <= 0 || cfg.CookVitality <= 0 {
		return
	}
	// The item ActCook works on is the first one in the hands, which is the
	// one every other rule about hands works on.
	held := -1
	for i := range p.Foods {
		if p.Foods[i].Held {
			held = i
			break
		}
	}
	if held < 0 {
		return
	}
	f := &p.Foods[held]
	// What it would mend once it is done. The quality is the body's own, and
	// it is the figure Self.CanCook was decided on, so nothing here has to
	// ask the world a second time.
	cookedHeal := cfg.CookVitality * s.CookQuality * s.MaxVitality * f.Nutrition
	if cookedHeal <= f.Heal {
		return
	}
	ticks := float64(cfg.CookTicks)
	incoming := c.incomingDmg
	// Scored at the hunger this body will be at when the cooking is done,
	// because that is when the meal is: one trajectory carried forward, the
	// same shape as every other estimate here.
	hunger := s.Hunger + s.HungerRate*ticks
	gain := mealValueAt(cfg, s, incoming, hunger, f.Nutrition, cookedHeal) -
		mealValueAt(cfg, s, incoming, hunger, f.Nutrition, f.Heal)
	if gain <= 0 {
		return
	}
	c.add(Action{Kind: ActCook}, Utility{
		Life:     Goal{Value: gain, Chance: 1},
		Ticks:    ticks,
		TimeCost: ticks * cfg.TimeCost,
	})
}

// addCoins scores picking money up (stage 51).
//
// A coin is worth the meal it will buy when there is nothing about, which is
// keepValue and nothing else (#73), discounted for the chance of finding
// anybody willing to sell. That last figure is the one this whole stage turns
// on, and it is not guessed: it is CoinValue, and the measurement is what it
// takes for a sale to be worth making to the other side.
//
// Nothing here is a second reason to want money. A coin is not saved, not
// counted, and not worth anything to a body that will never need a meal.
func (c *AIController) addCoins(p *Perception) {
	cfg, s := p.Cfg, &p.Self
	if len(p.Coins) == 0 || !s.CarryRoom {
		return
	}
	want := coinWorth(cfg, s) * clamp(s.FoodScarcity, 0, 3) / 3
	if want <= 0 {
		return
	}
	for i := range p.Coins {
		if i >= maxFoodOptions {
			break
		}
		f := &p.Coins[i]
		for _, effort := range effortLevels {
			ticks := f.Dist/speedAt(s.MaxSpeed, effort) + 1
			cost := moveCost(cfg, s, effort) * ticks
			c.add(Action{Kind: ActTake, TargetID: f.ID, Effort: effort}, Utility{
				Life:         Goal{Value: want, Chance: 1},
				Vitality:     cost,
				Ticks:        ticks,
				VitalityCost: cost * cfg.VitalityWeight,
				TimeCost:     ticks * cfg.TimeCost,
			})
		}
	}
}

// addBuy scores walking up to somebody holding a meal out and paying for it
// (stage 51).
//
// What the buyer gets is the meal, scored exactly as any other meal is. What
// it gives up is the coin, which is worth what it would have bought later - so
// a hungry body pays gladly and a fed one has no reason to, which is the right
// way round and falls out of the same two figures the rest of the world uses.
//
// Only somebody crying their wares can be bought from, because a meal in a
// hand is invisible until it is held out (stage 40, and stage 49's cry is what
// makes it visible). That is the chain the plumbing was laid for: without the
// crier there is no shop window.
func (c *AIController) addBuy(p *Perception, o *AgentView) {
	cfg, s := p.Cfg, &p.Self
	if !s.HasCoin || !o.Selling || o.OfferValue <= 0 {
		return
	}
	meal := mealValue(cfg, s, c.incomingDmg, o.OfferValue, o.OfferHeal)
	if kept := keepValue(cfg, s, c.incomingDmg, o.OfferValue, o.OfferHeal); kept > meal {
		meal = kept
	}
	gain := meal - coinWorth(cfg, s)
	if gain <= 0 {
		return
	}
	for _, effort := range effortLevels {
		ticks := o.Dist/speedAt(s.MaxSpeed, effort) + 1
		cost := moveCost(cfg, s, effort) * ticks
		c.add(Action{Kind: ActBuy, TargetID: o.ID, Effort: effort}, Utility{
			Life:         Goal{Value: gain, Chance: 1},
			Vitality:     cost,
			Ticks:        ticks,
			VitalityCost: cost * cfg.VitalityWeight,
			TimeCost:     ticks * cfg.TimeCost,
		})
	}
}

// addStones scores picking one up (stage 46).
//
// A stone is worth what it lets a body do, which is throw it - so what is
// scored here is a throw that has not happened yet, at a rival that may not be
// there yet, discounted by the same figure that discounts a meal carried for
// later (CarryValue) and scaled by how contested this patch feels. In a quiet
// corner with food to spare, a stone is a stone; where bodies are crowding the
// same plants, it is worth having one.
//
// Before there was anything to throw, nothing valued a stone and nobody picked
// one up - which is what stage 45 measured, and why the supply was counted
// there rather than assumed here.
func (c *AIController) addStones(p *Perception) {
	cfg, s := p.Cfg, &p.Self
	if !cfg.Throwing || !s.CarryRoom || cfg.CarryValue <= 0 || len(p.Stones) == 0 {
		return
	}
	// What one stone would do to an ordinary body, as a share of finishing it.
	damage := damagePerTick(cfg, s.Attack, 1) * cfg.ThrowDamage * cfg.ThrowHit
	share := clamp(damage/math.Max(cfg.MaxVitality, 1e-9), 0, 1)
	want := cfg.CarryValue * s.CompetitionWeight * cfg.LifeValue *
		clamp(s.FoodScarcity, 0, 3) / 3 * share
	if want <= 0 {
		return
	}
	for i := range p.Stones {
		if i >= maxAgentOptions {
			break
		}
		st := &p.Stones[i]
		for _, effort := range effortLevels {
			ticks := st.Dist/speedAt(s.MaxSpeed, effort) + 1
			cost := moveCost(cfg, s, effort) * ticks
			c.add(Action{Kind: ActTake, TargetID: st.ID, Effort: effort}, Utility{
				Life:         Goal{Value: want, Chance: 1},
				Vitality:     cost,
				Ticks:        ticks,
				VitalityCost: cost * cfg.VitalityWeight,
				TimeCost:     ticks * cfg.TimeCost,
			})
		}
	}
}

// addThrow scores putting a stone into somebody from out of reach (stage 46).
//
// It is scored as an attack is scored and not as something new: what a body
// wants out of hurting somebody - being rid of a rival for the food, finishing
// something worth eating, easing what is coming at it now - is the same list,
// and the only differences are how much of it one stone buys and that the
// answer has to walk over first.
//
// The exchange it charges itself for is the same one a fight charges. A body
// that throws is not believed to be safe: it expects to be hit back exactly as
// often as if it had walked up, which is conservative, and deliberately so.
// Stage 12a found that the belief in retaliation is what holds this world
// together, so a rule that let agents notice range makes them safe would be
// changing two things at once. What range changes here is what the world
// actually does, and that is what the measurement reads.
func (c *AIController) addThrow(p *Perception, o *AgentView) {
	cfg, s := p.Cfg, &p.Self
	if !s.HasStone || o.ThrowHit <= 0 || o.Species == s.Species && o.Paired {
		return
	}
	const effort = 1.0

	// What one stone does, if it lands.
	damage := damagePerTick(cfg, s.Attack, effort) * cfg.ThrowDamage * o.ThrowHit
	share := clamp(damage/math.Max(o.Vitality, 1e-9), 0, 1)

	// What it eases. A stone in somebody who is hitting this body now takes
	// part of what is coming: the same arithmetic the fight uses, over one
	// throw instead of an exchange.
	drain := projectedDrain(cfg, s.HungerRate, s.Hunger)
	now := pressure(cfg, s, s.Vitality, drain+c.incomingDmg)
	eased := c.incomingDmg
	if o.AttackingMe {
		eased = math.Max(0, c.incomingDmg*(1-share))
	}
	// A throw is a throw: the body is not guarding or dodging while it does
	// it, so what it pays is the aggressive stance's cost and no more.
	cost := stanceCost(cfg, StanceAggressive) * effort
	after := pressure(cfg, s, s.Vitality-cost, drain+eased)
	lifeTerm := (now - after) * cfg.LifeValue

	// What it is worth if it finishes them: the same two reasons a fight has.
	stake := Goal{}
	if c.bestFoodRival == o.ID {
		stake = Goal{Value: c.bestFoodGap, Chance: share}
	}
	if o.Prey && o.Meat >= 1 && s.Hunger > 0 && cfg.PreyValue > 0 {
		bite := math.Min(o.Meat, 1) * cfg.PreyValue *
			mealValue(cfg, s, c.incomingDmg, s.Nutrition[FoodMeat], s.Heal[FoodMeat])
		stake = Goal{Value: stake.Value + bite, Chance: math.Max(stake.Chance, share)}
	}
	competition := Goal{
		Value:  s.CompetitionWeight * cfg.LifeValue * clamp(s.FoodScarcity, 0, 3) / 3,
		Chance: share,
	}

	c.add(Action{Kind: ActThrow, TargetID: o.ID, Effort: effort}, Utility{
		Life:         Goal{Value: lifeTerm, Chance: 1},
		Stake:        stake,
		Rival:        competition,
		Risk:         s.RiskWeight * o.Risk,
		Vitality:     cost,
		Ticks:        1,
		VitalityCost: cost * cfg.VitalityWeight,
		TimeCost:     cfg.TimeCost,
	})
}

// allyForce is what one side of a fight can count on besides itself: the
// trust-weighted strength of everybody who has declared for the same target,
// and the damage they are expected to be putting in.
type allyForce struct {
	target  int
	score   float64
	damage  float64
	backers int
}

func (c *AIController) noteAlly(target int, score, damage float64) {
	for i := range c.allies {
		if c.allies[i].target == target {
			c.allies[i].score += score
			c.allies[i].damage += damage
			c.allies[i].backers++
			return
		}
	}
	c.allies = append(c.allies, allyForce{target: target, score: score, damage: damage, backers: 1})
}

// quarrelOnly reports that a declaration should not be counted because it is
// against one of this agent's own kind and the arm being measured only counts
// hunts (AllyPreyOnly).
func (c *AIController) quarrelOnly(p *Perception, target int) bool {
	if !p.Cfg.AllyPreyOnly {
		return false
	}
	for i := range p.Others {
		if p.Others[i].ID == target {
			return !p.Others[i].Prey
		}
	}
	return true // out of sight: nothing to say it is a hunt
}

// help is what is already declared against a target.
func (c *AIController) help(target int) allyForce {
	for i := range c.allies {
		if c.allies[i].target == target {
			return c.allies[i]
		}
	}
	return allyForce{target: target}
}

// hoped is what an agent about to call out can hope for: the same
// trust-weighted sum over everybody in sight of its own kind who has not
// already taken something else on.
//
// It is a hope and not a fact, which is the difference between calling and
// joining. Whoever is already swinging can be seen swinging; whoever has yet
// to be asked can only be counted on as far as they are trusted, and that is
// exactly the number trust is.
func (c *AIController) hoped(p *Perception, prey *AgentView) allyForce {
	cfg := p.Cfg
	out := allyForce{target: prey.ID}
	if cfg.AllyTrustWeight <= 0 || cfg.AffinityTrust <= 0 {
		return out
	}
	for i := range p.Others {
		o := &p.Others[i]
		if o.ID == prey.ID || o.Species != p.Self.Species || o.DeclaredFor != 0 {
			continue
		}
		trust := clamp(o.Affinity/cfg.AffinityTrust, 0, 1) * cfg.AllyTrustWeight
		if trust <= 0 {
			continue
		}
		out.score += trust * o.EstStrength * o.Vitality
		out.damage += trust * damagePerTick(cfg, o.EstStrength, 1)
		out.backers++
	}
	return out
}

// addAttack scores picking a fight. The same option covers both reasons to
// throw a punch: taking the meal in front of you, and thinning out the
// competition before the food runs short. Whether either is worth the vitality
// is what the formula decides.
func (c *AIController) addAttack(p *Perception, o *AgentView) {
	help := c.help(o.ID)
	// Three ready mixes rather than two levels of one number: how hard to
	// swing is now inseparable from how much guard to keep up.
	for stance := Stance(0); int(stance) < NumStances; stance++ {
		c.scoreFight(p, o, help, ActAttack, stance, 0)
	}
}

// addInvite scores calling others in against something (stage 32).
//
// It is the same fight scored with the help that could come, plus the moment
// spent calling. There is no bonus for cooperating and no threshold above
// which an invitation is worth making: with nobody trusted in sight the hope
// is zero, the option is the attack plus a wasted moment, and it loses to the
// attack on arithmetic alone.
//
// Scored at every stance, like the fight it is the front half of, so that the
// two are compared like for like: the only differences between calling and
// going in alone are the help that could come and the moment spent asking.
func (c *AIController) addInvite(p *Perception, o *AgentView) {
	cfg := p.Cfg
	if cfg.CallTicks <= 0 {
		return
	}
	hope := c.hoped(p, o)
	if hope.backers == 0 {
		return
	}
	for stance := Stance(0); int(stance) < NumStances; stance++ {
		c.scoreFight(p, o, hope, ActInvite, stance, cfg.CallTicks)
	}
}

// scoreFight is the whole of what a fight is worth, with whatever help is
// counted on folded into both sides of it: the odds, and how long the thing
// takes to bring down.
//
// The help is a number of the observer's own making - somebody else's believed
// strength, times how far it trusts them to still be there. Nothing in the
// world enforces it. An ally that thinks better of it and walks away leaves
// this agent in a fight it priced as a shared one, and the vitality it loses
// finding that out is what teaches it (through the ordinary risk memory) not
// to count on that one again.
func (c *AIController) scoreFight(p *Perception, o *AgentView, help allyForce, kind ActionKind, stance Stance, extraTicks int) {
	cfg := p.Cfg
	s := &p.Self

	myScore := s.Attack*s.Vitality + help.score
	theirScore := o.EstStrength * o.Vitality
	pWin := myScore / (myScore + theirScore + 1e-9)

	maxDepth := strategyDepth(cfg, s.Intelligence)

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
	//
	// How readily the other side hits back at all is this agent's own
	// figure now (lore.go): one that has been picking on people who did
	// not fight back expects the next one not to either, and is wrong
	// about the one that does.
	mine := damagePerTick(cfg, s.Attack, effort*m.Attack)
	if o.Uphill && cfg.HighGroundCover > 0 {
		// Swinging up a bank at somebody: what they can do about it is
		// their own hidden business, but that the bank is there is not.
		// The figure is the world's ordinary evasion, since how well this
		// one in particular gets out of the way is not knowable.
		mine *= 1 - clamp(cfg.HighGroundCover, 0, cfg.EvasionCap)
	}
	theirs := damagePerTick(cfg, o.EstStrength, s.Retaliation) *
		(1 - s.Defence*m.Defence) * (1 - s.Evasion*m.Evasion)

	// Either they go down, or one side breaks off first. A weakened
	// target is cheap to finish, which is what makes hitting somebody who
	// is already hurt the best value there is.
	// How fast it goes down is this agent's blows plus whatever the ones
	// who have declared for it are putting in. This is where a big animal
	// becomes worth taking on: alone the exchange runs out at
	// SkirmishTicks with the thing still standing, and with two of you it
	// does not.
	exchange := math.Min(o.Vitality/math.Max(mine+help.damage, 1e-9), cfg.SkirmishTicks)
	travel := o.Dist / speedAt(s.MaxSpeed, effort)
	ticks := exchange + travel + float64(extraTicks)

	cost := exchange*(theirs+stanceCost(cfg, stance)*effort) + travel*moveCost(cfg, s, effort)

	drain := projectedDrain(cfg, s.HungerRate, s.Hunger+s.HungerRate*ticks)
	now := pressure(cfg, s, s.Vitality, projectedDrain(cfg, s.HungerRate, s.Hunger)+c.incomingDmg)
	after := pressure(cfg, s, s.Vitality-cost, drain)
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
		bite := math.Min(o.Meat, 1) * cfg.PreyValue *
			mealValue(cfg, s, c.incomingDmg, s.Nutrition[FoodMeat], s.Heal[FoodMeat])
		pKill := clamp(exchange*(mine+help.damage)/math.Max(o.Vitality, 1e-9), 0, 1) * pWin
		stake = Goal{Value: stake.Value + bite, Chance: math.Max(stake.Chance, pKill)}
	}

	// Removing somebody who will be eating the same food later on. Only an
	// agent that can think that far ahead sees this at all, and a world
	// with food to spare makes the term vanish on its own.
	competition := Goal{}
	if maxDepth >= depthPreemtive {
		competition = Goal{
			Value:  s.CompetitionWeight * cfg.LifeValue * clamp(s.FoodScarcity, 0, 3) / 3,
			Chance: pWin,
		}
	}

	c.add(Action{Kind: kind, TargetID: o.ID, Effort: effort, Stance: stance}, Utility{
		Life:         Goal{Value: lifeTerm, Chance: 1},
		Stake:        stake,
		Rival:        competition,
		Risk:         s.RiskWeight * o.Risk,
		Vitality:     cost,
		Ticks:        ticks,
		VitalityCost: cost * cfg.VitalityWeight,
		TimeCost:     ticks * cfg.TimeCost,
	})
}

// addFlee scores running away. Nothing here says "flee when hurt": what the
// formula sees is that the damage coming in shortens the agent's life, and
// that walking away from it is worth more the closer to death it is.
func (c *AIController) addFlee(p *Perception, o *AgentView) {
	cfg := p.Cfg
	s := &p.Self

	drain := projectedDrain(cfg, s.HungerRate, s.Hunger)
	incoming := damagePerTick(cfg, o.EstStrength, 1)
	staying := pressure(cfg, s, s.Vitality, drain+incoming)

	// Breaking away is not free: for a while the agent is still in reach and
	// is the one not hitting back, which is the cheapest thing there is to
	// hit. This is the only option that gets the incoming damage out of the
	// picture, which is why running away wins exactly when the damage is what
	// is about to kill the agent, and loses whenever it is not.
	cost := moveCost(cfg, s, cfg.FleeEffort)*fleeExposureTicks + incoming*fleeExposureTicks*0.4
	pEscape := clamp(s.Vitality/(s.Vitality+o.Vitality+1e-9), 0.15, 0.9)
	fled := pressure(cfg, s, s.Vitality-cost, drain)

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
	s := &p.Self

	effort := 0.6
	ticks := o.Dist/speedAt(p.Self.MaxSpeed, effort) + 1
	cost := moveCost(cfg, s, effort) * ticks

	// What a child costs the body that has one: the parents share the birth,
	// and it is only paid if the courtship is accepted (stage 25).
	//
	// Until this was here, courting looked as though it cost a walk. What
	// stood in for the missing term was a threshold that stopped a hungry
	// agent courting at all - a hardcoded behavioural gate of exactly the kind
	// the design says not to write. Priced properly, an agent that cannot
	// afford a child turns one down on the numbers.
	birth := cfg.BirthVitalityCost / 2 * s.AcceptChance
	drain := projectedDrain(cfg, s.HungerRate, s.Hunger) + c.incomingDmg
	now := pressure(cfg, s, s.Vitality, drain)
	after := pressure(cfg, s, s.Vitality-cost-birth, drain)

	c.add(Action{Kind: ActCourt, TargetID: o.ID, Effort: effort}, Utility{
		Offspring:    Goal{Value: cfg.OffspringValue * clamp(o.Fitness/MaxAbility, 0, 1), Chance: s.AcceptChance},
		Life:         Goal{Value: (now - after) * cfg.LifeValue, Chance: 1},
		Vitality:     cost + birth,
		Ticks:        ticks,
		VitalityCost: (cost + birth) * cfg.VitalityWeight,
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

	// And standing with somebody is also how what each of them assumes gets
	// traded (stage 12b). An agent cannot see what anybody else believes, so
	// what it goes on is who has been worth standing with before - which makes
	// this the one term in the whole formula that is worth more the more
	// somebody else matters to it. It is what a group has to last on.
	trust := clamp(o.Affinity/cfg.AffinityTrust, 0, 1)

	c.add(Action{Kind: ActObserve, TargetID: o.ID, Effort: 0.3}, Utility{
		Info:     Goal{Value: cfg.InfoValue * unsure, Chance: relevance},
		Lore:     Goal{Value: cfg.LoreValue * trust, Chance: 1},
		Ticks:    observeTicks,
		TimeCost: observeTicks * cfg.TimeCost,
	})
}

// survey walks the neighbours once and works out the two things the options
// need to know about them as a group: the damage already landing, and how
// exposed the agent would be if it stopped paying attention to them.
//
// One pass, before anything is scored. Every option used to ask for the
// incoming damage separately and walk the whole neighbourhood to answer, so
// the same list was crossed several times a decision; the exposure of resting
// needs exactly the same walk, and rides on this one rather than adding
// another.
func (c *AIController) survey(p *Perception) {
	cfg := p.Cfg
	c.incomingDmg, c.exposure = 0, 0
	attacker := p.Self.AttackerID
	known := false

	for i := range p.Others {
		o := &p.Others[i]
		threat := damagePerTick(cfg, o.EstStrength, 1)

		// Somebody who has taken a target on, and how much of its weight this
		// agent can count on (stage 32). Trust, not affinity: what is being
		// estimated is not how much this agent likes the other but how likely
		// it is that the other is still swinging when the blows land. A
		// stranger is worth nothing here however strong it is, which is why a
		// crowd of strangers does not add up to a hunting party.
		if cfg.AllyTrustWeight > 0 && cfg.AffinityTrust > 0 {
			if target := o.DeclaredFor; target != 0 && !c.quarrelOnly(p, target) {
				trust := clamp(o.Affinity/cfg.AffinityTrust, 0, 1) * cfg.AllyTrustWeight
				if trust > 0 {
					c.noteAlly(target, trust*o.EstStrength*o.Vitality, trust*threat)
				}
			}
		}
		if o.ID == attacker {
			// What the ground is worth, if this one is swinging uphill
			// (stage 30). The agent knows it is standing above its attacker -
			// that is what it can see - and it knows its own evasion, so what
			// it works out here is its own body on its own ground.
			if p.Self.Covered && cfg.HighGroundCover > 0 {
				dodge := clamp(math.Max(p.Self.Evasion, cfg.HighGroundCover), 0, cfg.EvasionCap)
				threat *= 1 - dodge
			}
			c.incomingDmg, known = threat, true
		}
		if cfg.RestExposureWeight <= 0 {
			continue
		}
		// How much of that threat is a threat to this agent: what is close
		// enough to reach it, less whatever it trusts. Somebody it has a bond
		// with is not somebody it has to keep an eye on.
		near := 1.0
		if cfg.PerceptionRadius > 0 {
			near = clamp(1-o.Dist/cfg.PerceptionRadius, 0, 1)
		}
		// Somebody it trusts is somebody it does not have to keep an eye on -
		// but only while that somebody is awake. A sleeping friend cannot
		// watch over anybody (stage 18).
		//
		// This is a condition on a term that was already there rather than a
		// new one, and it is what turns "we happen to sleep at different
		// hours" into "somebody is keeping watch" without a watchman existing
		// anywhere in the rules.
		trust := 0.0
		if cfg.AffinityTrust > 0 && (cfg.SleepingWatch || !o.Resting) {
			trust = clamp(o.Affinity/cfg.AffinityTrust, 0, 1)
		}
		c.exposure += threat * near * (1 - trust)
	}

	// Being hit by somebody out of sight: it is still being hit, and the
	// stranger's prior is the only figure there is to put on it.
	if attacker != 0 && !known {
		c.incomingDmg = damagePerTick(cfg, cfg.PriorStrength, 1)
	}
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
	c.ChoseBetterGround = best == c.betterGroundOpt
	c.ChoseMissing = best == c.lonelyOpt
	c.ChoseHome = best == c.homeOpt
	for _, i := range c.offerOpts {
		if i == best {
			c.WentToOffer = true
			break
		}
	}
	// Whether the fight it picked was one somebody else had already taken on
	// (stage 32). Measurement only, like the line above: a rule that produces
	// pack hunting has to show up as agents choosing the same target, and a
	// call nobody answers is a word rather than a hunt.
	if act := c.opts[best].action; act.Kind == ActAttack {
		c.JoinedDeclared = c.help(act.TargetID).backers > 0
	}
	return c.opts[best].action
}
