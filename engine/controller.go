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

	// riskNow is the chance of dying inside the planning window as things
	// stand, which several options are read against (stage 73).
	riskNow riskPair

	// roomWorth is what a free hand would have been worth to this body: the
	// best pickup it scored and could not offer itself, because its hands
	// were full (stage 70). Nothing reads it but the word for putting
	// something down, and nothing fills it in a world without that word.
	roomWorth float64

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
	homePull float64

	// Which way it gets colder from here and how fast this body walks, kept
	// for the one charge that tells two headings apart (stage 86b).
	chillDX, chillDY, chillSpeed float64
	homeAway                     float64
	homeUX                       float64
	homeUY                       float64
	homeSpan                     float64
	homeSpeed                    float64
	lifeValue                    float64

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
	c.roomWorth = 0
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
	c.chillDX, c.chillDY = p.Self.ChillDX, p.Self.ChillDY
	c.chillSpeed = p.Self.MaxSpeed
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
	// The chance of dying as things stand, worked out once: the food options,
	// courting and wandering all ask for the same figure (stage 73, which is
	// also where wandering started needing it).
	c.riskNow = pressures(p.Cfg, &p.Self, p.Self.Vitality, p.Self.Hunger, c.incomingDmg)
	c.addWarmth(p)
	c.addRest(p)
	c.addExplore(p)
	c.addFood(p)
	c.addStones(p)
	c.addCoins(p)
	c.addPutInStore(p)
	c.addCook(p)
	c.addAgents(p, maxDepth)
	c.addOffer(p)
	c.addBooks(p)
	c.addCraft(p)
	c.addTrinkets(p)
	c.addDrop(p) // last: what a hand is worth depends on what was scored for it

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
// It is worked out from the hunger the option would leave the body at rather
// than from a drain handed in ready made, because the second step below has to
// know what the body's own metabolism will be doing next - and the drain alone
// cannot say. extra is everything that is taking vitality for reasons that are
// not hunger: a blow coming in, the exposure of lying down among strangers.
func pressure(cfg *Config, s *SelfView, vitality, hunger, extra float64) float64 {
	return pressures(cfg, s, vitality, hunger, extra).far
}

// riskPair is the same body read both ways (stage 74): near is the single
// window, far the chain of two. They are kept together because a difference
// and a level want different ones - see gap.
type riskPair struct{ near, far float64 }

// pressures is pressure with both views kept.
func pressures(cfg *Config, s *SelfView, vitality, hunger, extra float64) riskPair {
	// The body's own metabolism, and what the weather where it stands takes
	// on top of it (stage 85). The cold goes here rather than in extra
	// because extra is deliberately dropped at the second window - what is
	// hitting a body now is a fact about now - and a place does not stop
	// being cold.
	own := projectedDrain(cfg, s.HungerRate, hunger) + s.Chill
	p := oneHorizon(cfg, s, vitality, own+extra, true)
	if cfg.LookaheadHorizons <= 0 || p >= 1 {
		return riskPair{p, p}
	}

	// One step further out (stage 67). The body is carried forward by its own
	// metabolism - hunger climbs at its own rate, vitality goes down by what
	// that hunger costs - and then asked the same question again. Chaining the
	// two is the chance of dying in the first window, or surviving it and
	// dying in the second.
	//
	// extra is deliberately not carried across. What is hitting this body now
	// is a fact about now, and stretching it over a second horizon would drain
	// the tank to nothing in every candidate alike, leaving a fight with
	// nothing to choose between. Everything about other agents stays in the
	// window it was measured in.
	//
	// How fast hunger climbs out there is the one thing the body is allowed
	// to know about itself besides its metabolism (stage 72): what it has
	// been managing to eat, discounted, and never better than breaking even.
	// With that off this is the metabolism alone, which is stage 67 exactly.
	h := cfg.PlanHorizon * cfg.LookaheadHorizons
	climb := hungerClimb(cfg, s)
	hu := clamp(hunger+climb*h, 0, cfg.MaxHunger)
	if cfg.LookaheadHolds && s.HeldMeals > 0 {
		// And what is in its hands, which is the one thing out there it is
		// sure of (stage 78). A level and not a rate: a meal is a drop in
		// hunger, not a slower climb. Discounted like everything else this
		// formula assumes about the future, and by the same figure, because
		// eating what you are holding is part of keeping yourself up.
		hu = clamp(hu-cfg.FoodNutrition*s.HeldMeals*cfg.LookaheadUpkeep,
			0, cfg.MaxHunger)
	}
	drain := own
	mend := 0.0
	if cfg.LookaheadUpkeep > 0 {
		// What it would be draining on the way, rather than what it is
		// draining now: a body that keeps its hunger down stops paying for it.
		drain = projectedDrain(cfg, s.HungerRate, (hunger+hu)/2) + s.Chill
		// And what it would win back out there. Upkeep is both halves: a body
		// that has been feeding itself has also been mending, and leaving the
		// mending out is what made a worn body read its own death as settled
		// whatever it did - the chance of dying of being worn down compounds
		// over a second window and nothing in it ever got better. What is
		// hitting it now stays in the first window, here as everywhere else.
		mend = cfg.LookaheadUpkeep * recoverable(cfg, s.MaxVitality, s.HungerRate,
			vitality, (hunger+hu)/2, s.Chill, s.RestRate)
	}
	v := clamp(vitality-drain*h+mend, 0, s.MaxVitality)
	next := oneHorizon(cfg, s, v, projectedDrain(cfg, s.HungerRate, hu)+s.Chill, cfg.LookaheadWornAgain)
	return riskPair{p, clamp(p+(1-p)*next, 0, 1)}
}

// gap is what a change in a body's standing is worth: the difference in the
// chance of dying, read through whichever window can tell the two apart
// (stage 74).
//
// The life term asks a question with a yes or no answer - will this body be
// dead by the end of the window - and one meal moves the death of a starving
// body from tick 594 to tick 1038. Against a window of 700 that moves it from
// inside to outside and the answer changes (0.175 to 0.028); against 1400 both
// are still inside, the answer does not change, and the meal is worth nothing.
// The meal did not change. The question did.
//
// So the rule is: look further only when it tells you more. Where the second
// window separates two states the first cannot - a satiated whole body, which
// cannot die inside one window at all - it is the one that counts, and stage
// 67 is intact. Where it saturates and says both states are death, the near
// window is the one that counts, and the world is the one every scenario in
// this project was written against. Nothing is lost by the swap: the chain is
// monotone in the near window, so where the far one discriminates at all it
// ranks the same way.
func gap(cfg *Config, before, after riskPair) float64 {
	far := before.far - after.far
	if !cfg.LookaheadNeverBlinds || cfg.LookaheadHorizons <= 0 {
		return far
	}
	if near := before.near - after.near; math.Abs(near) > math.Abs(far) {
		return near
	}
	return far
}

// oneHorizon is the chance of dying inside a single planning horizon, which is
// the whole of what pressure was before stage 67.
func oneHorizon(cfg *Config, s *SelfView, vitality, drain float64, worn bool) float64 {
	if vitality <= 0 {
		return 1
	}
	pDrain := 0.0
	if drain > 0 {
		ticksLeft := vitality / drain
		// The deadline reading: nothing at all once the tank outlasts the
		// window, which is where every flat gradient in this world comes from
		// (stage 93).
		if ticksLeft < cfg.PlanHorizon {
			pDrain = 1 - ticksLeft/cfg.PlanHorizon
		}
		// And the rate reading, blended in at whatever weight the world
		// carries: the same countdown as a hazard - how many times over a
		// window this body would run out at this drain - which keeps a shape
		// out past the horizon instead of stopping at it.
		//
		// Blended and not swapped, because a tail that never vanishes lifts
		// what every body reads, and lifting every body's risk alike is
		// lifting LifeValue, which is a rule of its own and was measured as
		// one (stage 67).
		if w := cfg.LookaheadReadsRate; w > 0 {
			pDrain = (1-w)*pDrain + w*(1-math.Exp(-cfg.PlanHorizon/ticksLeft))
		}
	}
	pWorn := 0.0
	if worn {
		pWorn = s.ShockRisk * (1 - clamp(vitality/s.MaxVitality, 0, 1))
	}
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
	// What is in the hand that the legs are actually charged for (stage 71).
	// It used to count everything, while the weight charged leaves out coins
	// and books - so a body holding a coin was told that picking up a berry
	// would cost it twice what it does.
	n := s.Carried
	if cfg.BurdenIgnoresWeightless {
		n = s.CarriedHeavy
	}
	load := float64(n+more) / s.CarryCapacity
	if cfg.CarrySlotted {
		load = clamp(load, 0, 1)
	} else {
		load = math.Max(0, load)
	}
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
	// And what the weather where this option would leave the body costs it
	// (stage 86b). Only the difference from here: the cold underfoot is
	// already in the risk, where it lifts every candidate alike - stage 86
	// measured that it therefore moves nobody however large it is - so what
	// is charged here is the part that tells one heading from another.
	//
	// It is charged over the ticks the option takes, which is how the ground's
	// own danger and the pull of home are charged, and it is nought for every
	// option that does not carry the body anywhere.
	if a.Kind == ActMove && (c.chillDX != 0 || c.chillDY != 0) {
		ticks := u.Ticks
		if ticks < 1 {
			ticks = 1
		}
		gone := speedAt(c.chillSpeed, a.Effort) * ticks
		if worse := (a.DX*c.chillDX + a.DY*c.chillDY) * gone; worse != 0 {
			u.Weather = worse * ticks
		}
	}
	c.opts = append(c.opts, option{action: a, util: u.Total()})
	if c.tracing {
		c.terms = append(c.terms, u)
	}
}

// --- options ---------------------------------------------------------------

// addWarmth scores heading for warmer country (stage 86b).
//
// The slope underfoot is a direction, and this is the option of following it
// far enough for the difference to be worth anything: a region's width, which
// is the grain the weather has. Nothing is remembered and nobody is told - the
// body is not going somewhere it knows about, it is going the way the wind is
// less cold.
//
// It is valued the way stage 15b values a walk to better country: by what the
// place would be worth rather than by what the step is worth. Counted before
// this was written, the step is worth nothing at all - the per-move charge in
// add() averages 0.000013 against a spread between the best and worst option
// of 118 and an evaluation noise of 139, because the weather changes over four
// hundred units of world and a decision carries a body ten or twenty. Two
// factors of forty, and the sense alone can never clear them.
func (c *AIController) addWarmth(p *Perception) {
	cfg, s := p.Cfg, &p.Self
	if s.ChillDX == 0 && s.ChillDY == 0 {
		return
	}
	slope := math.Hypot(s.ChillDX, s.ChillDY)
	if slope <= 0 {
		return
	}
	// The way it gets warmer, and how much warmer a region's width is.
	ux, uy := -s.ChillDX/slope, -s.ChillDY/slope
	span := max(cfg.Width/float64(max(cfg.RegionCols, 1)), 1)
	warmer := math.Min(slope*span, s.Chill) // it cannot get warmer than warm
	if warmer <= 0 {
		return
	}
	incoming := c.incomingDmg
	now := c.riskNow
	for _, effort := range effortLevels {
		ticks := span/speedAt(s.MaxSpeed, effort) + 1
		cost := moveCost(cfg, s, effort) * ticks
		// The same body, in country that much less cold. Chill is the one
		// thing about it that changes: this option is not about what grows
		// there or who is there, neither of which it knows.
		there := *s
		there.Chill = s.Chill - warmer
		after := pressures(cfg, &there, s.Vitality-cost,
			s.Hunger+s.HungerRate*ticks, incoming)
		c.add(Action{Kind: ActMove, DX: ux, DY: uy, Effort: effort}, Utility{
			Life:         Goal{Value: gap(cfg, now, after) * cfg.LifeValue, Chance: 1},
			Vitality:     cost,
			Ticks:        ticks,
			VitalityCost: cost * cfg.VitalityWeight,
			TimeCost:     ticks * cfg.TimeCost,
		})
	}
}

// warmthValue is what a piece that keeps this much of the weather off is worth
// to this body: the difference it makes to the chance of dying, priced exactly
// as a meal is (stage 87a).
//
// Nothing about it is new. The cold is a drain, a drain is a chance of dying,
// and this is that chance with less drain in it - which is why a warm thing is
// worth a great deal in the cold, nothing in the warm, and nothing to a body
// that already wards as much. None of those three is a rule.
func warmthValue(cfg *Config, s *SelfView, incoming, ward float64) float64 {
	if ward <= 0 || s.Chill <= 0 {
		return 0
	}
	warm := *s
	warm.Chill = math.Max(0, s.Chill-ward)
	now := pressures(cfg, s, s.Vitality, s.Hunger, incoming)
	after := pressures(cfg, &warm, s.Vitality, s.Hunger, incoming)
	return gap(cfg, now, after) * cfg.LifeValue
}

// addRest scores doing nothing. It is not a fallback: for a satiated agent it
// is the only way back to full vitality, and it costs nothing.
func (c *AIController) addRest(p *Perception) {
	cfg := p.Cfg
	s := &p.Self
	incoming := c.incomingDmg
	now := c.riskNow

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
	after := pressures(cfg, s,
		s.Vitality+recoverable(cfg, s.MaxVitality, s.HungerRate, s.Vitality, s.Hunger, incoming+exposed+s.Chill, s.RestRate),
		s.Hunger, incoming+exposed)
	c.add(Action{Kind: ActRest}, Utility{
		Life: Goal{Value: gap(cfg, now, after) * cfg.LifeValue, Chance: 1},
	})
}

// raceChance is the odds of getting to something before the nearest other body
// does. One place, because whatever is lying about is raced for on the same
// terms: a meal (stage 40) and money (stage 51) alike.
//
// Whoever gets there first is whoever takes less time to arrive, which is not
// the same question as who is nearer. The agent knows how fast it is; it knows
// nothing about the other one's legs - speed is a hidden ability like every
// other - so it assumes an ordinary body, the same shape of assumption the
// population prior makes about a stranger's strength. Being faster than
// average is what wins races, and it is the first thing the gene has ever
// bought.
//
// Both sides are assumed to be travelling the same way, so the effort cancels
// and only the difference in legs is left. Scoring the agent's own effort
// against a rival assumed to be sprinting was tried and is much worse: the one
// nearby is only the nearest agent, who may well be asleep, and treating every
// one of them as racing makes strolling over to a contested item look
// hopeless. It cost two thirds of the population (see HISTORY.md).
func raceChance(cfg *Config, s *SelfView, dist, rivalDist float64) float64 {
	if math.IsInf(rivalDist, 1) {
		return 1 // nobody else can see it
	}
	mine, theirs := dist/s.MaxSpeed, rivalDist/cfg.MaxSpeed
	if cfg.RaceOnDistance {
		mine, theirs = dist, rivalDist
	}
	// clamp, written out: this is scored for every item in sight, and the
	// same figure through math.Min and math.Max is too dear for the compiler
	// to put back where it came from.
	odds := theirs / (theirs + mine + 1e-9)
	if odds < 0.05 {
		return 0.05
	}
	if odds > 1 {
		return 1
	}
	return odds
}

// carryNeed is how likely it is that having a thing later will matter: the
// discount stage 40 charges on anything kept rather than eaten, scaled by how
// contested the food around this body is. A body in a patch with more food
// than neighbours has little use for a berry in its hand; one where the food
// is contested may well find nothing when it next looks, and FoodScarcity is
// the only reading it has of that.
//
// One place, because a coin is the same bet with one more thing that has to go
// right (stage 51): money is a claim on a meal at the moment of running short,
// so it is worth nothing at all to a body that never runs short, exactly as a
// berry in the hand is.
func carryNeed(cfg *Config, s *SelfView) float64 {
	return cfg.CarryValue * clamp(s.FoodScarcity, 0, 3) / 3
}

// survives is the share of a goal that is still worth having: what is left
// after the chance of not being there for it (stage 73).
//
// It is for goals that happen after this body has gone on living, and there is
// only one of those: a child. Wandering was discounted this way for a day and
// it is wrong - looking for something to eat is not a reward for surviving,
// it is how a body that is running out survives - and a test caught it at
// once: a hungry body with nothing in sight lay down instead of going to
// look.
//
// The goals of this formula are of two kinds, and until this was written only
// one of them knew how far ahead it was looking. Staying alive is priced as a
// difference of two chances of dying, so it shrinks as the window lengthens -
// one meal is the whole of a seven hundred tick problem and half of a
// fourteen hundred tick one. A child and a walk were priced as constants. Put
// side by side, the constants win by simply not shrinking, which is how a
// starving body came to court instead of eat and a cornered one to stroll
// away. This is not a second charge for dying: the life term is what this
// body's own survival is worth, and this is the condition on an event that
// happens later.
// ticks is how long this body has to last for the goal to arrive. A walk is
// paid off inside the window the risk is read over, so it takes the whole of
// it; a child is a hundred and fifty ticks of pairing away, and charging it a
// whole window of dying would be charging it for time it does not need.
func survives(cfg *Config, risk, ticks float64) float64 {
	if !cfg.GoalsNeedSurvival {
		return 1
	}
	left := clamp(1-risk, 0, 1)
	if cfg.PlanHorizon <= 0 || ticks >= cfg.PlanHorizon {
		return left
	}
	return math.Pow(left, ticks/cfg.PlanHorizon)
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
	before := pressures(cfg, s, s.Vitality, hunger, incoming)
	fed := math.Max(0, hunger-cfg.FoodNutrition*nutrition)
	mended := math.Min(s.Vitality+heal, s.MaxVitality)
	after := pressures(cfg, s, mended, fed, incoming)
	return gap(cfg, before, after) * cfg.LifeValue
}

// keepValue is what having this item when it is needed is worth. One place,
// because two options are the same bet: carrying it (stage 40) and putting it
// in a cache (stage 50).
// keeps is how long the thing has before it goes off, and zero for anything
// with no clock on it (stage 78). A thing that will be rotten by the moment it
// is wanted is worth nothing to keep: what is being valued is not the item but
// eating it then.
func keepValue(cfg *Config, s *SelfView, incoming, nutrition, heal, held, keeps float64) float64 {
	if cfg.CarryPricedBackwards {
		// The world as stages 40 to 49 measured it, kept so that those
		// figures can be reproduced: the loss from getting hungrier, with the
		// sign that made every such option a penalty.
		now := pressure(cfg, s, s.Vitality, s.Hunger, incoming)
		later := pressure(cfg, s, s.Vitality,
			math.Max(s.Hunger, cfg.StarveHunger), incoming)
		return (now - later) * cfg.LifeValue
	}
	// And whether it will still be there then (stage 78). The wait is the
	// same one the lug is charged over and the same one a sale reckons with,
	// so nothing new is worked out here.
	if cfg.LookaheadSpoils && keeps > 0 && keeps < shortfallIn(cfg, s) {
		return 0
	}
	// The moment this body runs short - and what it will already have eaten
	// by then (stage 71). A body runs short once inside a horizon, so the
	// second thing in a hand is worth what is left of that shortfall after
	// the first one has filled it, and the tenth is worth nothing at all.
	// held is what else is in the hand, in meals, and never the thing being
	// valued: an item cannot discount itself.
	hunger := math.Max(s.Hunger, cfg.StarveHunger)
	if cfg.CarryDiminishes && held > 0 {
		hunger = math.Max(0, hunger-cfg.FoodNutrition*held)
	}
	return mealValueAt(cfg, s, incoming, hunger, nutrition, heal)
}

// shortfallIn is how long this body has before it runs short: the moment
// keepValue values a thing at. One place, because three rules ask for it - what
// a seller would be giving up, what putting something down saves, and now
// whether what is being kept will still be there (stage 78).
func shortfallIn(cfg *Config, s *SelfView) float64 {
	if s.HungerRate <= 0 {
		return cfg.PlanHorizon
	}
	return clamp((cfg.StarveHunger-s.Hunger)/s.HungerRate, 0, cfg.PlanHorizon)
}

// otherMeals is what is in this body's hand besides the thing being valued,
// counted in meals (stage 71). Money counts at CoinValue apiece, because that
// is exactly what a coin is priced as: a claim on a meal.
func otherMeals(s *SelfView, own float64) float64 {
	return math.Max(0, s.HeldMeals-own)
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
	incoming := c.incomingDmg
	now := pressures(cfg, s, s.Vitality, s.Hunger, incoming)

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

		// Whether it can be landed at all (stage 43). A fish reached is not a
		// fish taken, and the chance of it depends on where this body would
		// be standing and what it has learned about fishing there.
		pGet := 1.0
		if f.Catch > 0 && f.Catch < 1 {
			pGet = f.Catch
		}
		pGet *= raceChance(cfg, s, f.Dist, f.RivalDist)

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
			vitAfter += recoverable(cfg, s.MaxVitality, s.HungerRate, vitAfter, hungerAfter, incoming+s.Chill, s.RestRate)
			after := pressures(cfg, s, vitAfter, hungerAfter, incoming)

			// What the warning on it says it will cost this body. The agent
			// is reading a signal and believing it - nothing here can tell
			// whether the plant meant it, which is the opening a liar would
			// need - but what the warning is worth depends on the stomach
			// hearing it (stage 38b).
			poison := f.Danger * cfg.PoisonDamage * (1 - s.PoisonResist)
			meal := gap(cfg, now, after) * cfg.LifeValue
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
			if !f.Held && cfg.CarryValue > 0 && (s.CarryRoom || cfg.Dropping) {
				// How likely it is to be needed, which money is charged
				// for in the same place and on the same figure (stage 51).
				need := carryNeed(cfg, s)
				keep := keepValue(cfg, s, incoming, f.Nutrition, f.Heal,
					otherMeals(s, 0), f.Spoils) // on the ground: nothing of it is in hand yet
				lug := (burdenWith(cfg, s, 1) - burdenOf(s)) *
					moveCostAt(cfg, effort) * groundOf(s) * wait
				u := Utility{
					Life:         Goal{Value: keep, Chance: pGet * need},
					Vitality:     cost + lug + poison*pGet,
					Ticks:        ticks,
					VitalityCost: (cost + lug + poison*pGet) * cfg.VitalityWeight,
					TimeCost:     ticks * cfg.TimeCost,
				}
				if s.CarryRoom {
					c.add(Action{Kind: ActTake, TargetID: f.ID, Effort: effort}, u)
				} else if t := u.Total(); t > c.roomWorth {
					// No room for it - so this is not an option, it is what
					// room would be worth (stage 70). A hand is worth what
					// would go in it, and a meal is the only thing this world
					// has ever shown a hand is for. It is only worked out
					// where there is a word for emptying one, so a world
					// without that word pays nothing for the arithmetic.
					c.roomWorth = t
				}
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

// trustBought is how much of the way to being trusted a hand-over of this size
// would carry somebody who is this far along already. Trust saturates, so it is
// worth most between strangers and nothing at all between two who are already
// close - which is the direction an economy needs, and it falls out of the
// existing figure rather than being asked for.
func trustBought(cfg *Config, affinity, amount float64) float64 {
	if amount <= 0 || cfg.AffinityTrust <= 0 {
		return 0
	}
	return clamp((affinity+amount)/cfg.AffinityTrust, 0, 1) -
		clamp(affinity/cfg.AffinityTrust, 0, 1)
}

// takerWorth is what the thing that would change hands looks like it would do
// for the one receiving it (stage 92b), in the units everything else here is
// in: the same handWorth, read for a body in the receiver's condition.
//
// Two things are assumed rather than looked up, and both are assumptions this
// world already makes about strangers. Hunger is hidden, so the standard is
// used - the point at which a body runs short, which is exactly what keepValue
// assumes about this body's own future. And the piece is the giver's own view
// of it, because a stranger's diet ledger and what it already holds are hidden
// too. What is not assumed is the half that shows: how far down the other one
// is, which is what makes the same mouthful worth more to a battered child
// than to a whole one.
//
// No lookahead is nested in a lookahead here: this is one more reading of the
// same function, on a different body's numbers.
func takerWorth(cfg *Config, s *SelfView, o *AgentView, item *FoodView) float64 {
	as := *s
	as.Vitality = clamp(o.Vitality, 0, as.MaxVitality)
	as.Hunger = cfg.StarveHunger
	return handWorth(cfg, &as, item)
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
	if s.Carried == 0 || cfg.AffinityGift <= 0 || cfg.AffinityTrust <= 0 {
		return
	}
	// Room for the thing that would actually change hands (stage 80a): where
	// the hand is priced by weight alone, something that weighs nothing needs
	// no hand. A body holding nothing heavy is holding nothing but weightless
	// things, so what it would hand over is one of those; anything else is
	// asked the old question, which is the same bool in every world before
	// this rule.
	room := o.CarryRoom
	if s.CarriedHeavy == 0 {
		room = o.LightRoom
	}
	if !room {
		return
	}
	// What being on better terms with this one would buy, which for a parent
	// or a child of one's own is usually nothing at all: kin start at
	// AffinityKin, that is over AffinityTrust, and a trust already full buys
	// nothing more. Until stage 92b that ended the matter here, and it is why
	// only 6% of gifts go to kin - not a preference for strangers but the
	// saturation, which is the thing #121 says not to fight.
	gained := trustBought(cfg, o.Affinity, cfg.AffinityGift)
	kin := cfg.GiftKinWeight > 0 && o.Kin
	if gained <= 0 && !kin {
		return
	}
	// And what it would be giving up, where a gift costs what it was worth
	// (stage 84), at whatever share of it this body weighs (stage 92a).
	// Selling has always priced this (willSell) and so has putting something
	// down, and leaving it out here is why a body with something to spare
	// hands it over rather than holding out for a coin. It is the same figure
	// both of those weigh: the cheapest thing in the hand, because that is
	// the one that would go where HandOverCheapest is on.
	//
	// GiftPriced is the whole of it as a switch, which is how stage 84 ran
	// it; the dial is here because pricing the giving on its own has been
	// measured twice and stopped the exchange both times, so what it is for
	// is being read next to the reason below rather than alone.
	var item *FoodView
	spare := 0.0
	if cfg.GiftPriced || cfg.GiftSelfLookahead > 0 || kin {
		item, spare = c.spareItem(p)
	}
	gift := cfg.LoreValue * gained
	switch {
	case cfg.GiftPriced:
		gift -= spare
	case cfg.GiftSelfLookahead > 0:
		gift -= cfg.GiftSelfLookahead * spare
	}
	// And what it does for whoever receives it, where that one carries this
	// body's own genes (stage 92b). Hamilton's rB: a second channel, added
	// rather than folded into the trust, because the trust saturates and that
	// saturation is the force that sends 95% of gifts to strangers.
	//
	// The receiver's own reckoning is not available and is not asked for: its
	// hunger is hidden, three bodies' worth of lookahead nested inside one
	// decision is not free, and this world has priced a stranger on the
	// standard since stage 49. So the thing is worth what it is worth here,
	// scaled by how far down the other one looks - the one half of its need
	// that shows.
	if kin && item != nil {
		gift += cfg.GiftKinWeight * takerWorth(cfg, s, o, item)
	}
	if gift <= 0 {
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
			Lore:         Goal{Value: gift, Chance: 1},
			Vitality:     cost,
			Ticks:        ticks,
			VitalityCost: cost * cfg.VitalityWeight,
			TimeCost:     ticks * cfg.TimeCost,
		})
	}
}

// spareWorth is what this body would be giving up by handing something over:
// the least it minds losing, which is the thing that would actually go
// (stage 84). Nothing in hand is nothing to give.
func (c *AIController) spareWorth(p *Perception) float64 {
	_, worth := c.spareItem(p)
	return worth
}

// spareItem is the same question with the thing itself, which stage 92b needs:
// what it is worth to whoever receives it can only be asked of a particular
// piece.
func (c *AIController) spareItem(p *Perception) (*FoodView, float64) {
	cfg, s := p.Cfg, &p.Self
	least, at := math.Inf(1), -1
	for i := range p.Held {
		if worth := handWorth(cfg, s, &p.Held[i]); worth < least {
			least, at = worth, i
		}
	}
	if at < 0 {
		return nil, 0
	}
	return &p.Held[at], least
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
	room := s.CarryRoom
	if weightless(o.OfferKind) {
		room = s.LightRoom
	}
	if !o.Offering || o.OfferLeft <= 0 || !room || o.Dist <= 1e-9 {
		return
	}
	incoming := c.incomingDmg
	now := pressures(cfg, s, s.Vitality, s.Hunger, incoming)
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
		vitAfter += recoverable(cfg, s.MaxVitality, s.HungerRate, vitAfter, hungerAfter, incoming+s.Chill, s.RestRate)
		after := pressures(cfg, s, vitAfter, hungerAfter, incoming)
		c.offerOpts = append(c.offerOpts, len(c.opts))
		chance := pGet * clamp(float64(o.OfferLeft)/ticks, 0, 1)
		// What is being held up. A book is worth what it would say to this
		// body (stage 69), and that is a different goal from a meal: it buys
		// knowing, not another day.
		u := Utility{
			Life:         Goal{Value: gap(cfg, now, after) * cfg.LifeValue, Chance: chance},
			Vitality:     cost,
			Ticks:        ticks,
			VitalityCost: cost * cfg.VitalityWeight,
			TimeCost:     ticks * cfg.TimeCost,
		}
		if o.OfferWorth > 0 {
			u.Life = Goal{}
			if o.OfferKind == FoodTrinket {
				// An ornament is worth what it is worth, and it is its own
				// goal (stage 82): not another day and not knowing anything.
				u.Adorn = Goal{Value: o.OfferWorth, Chance: chance}
			} else {
				u.Lore = Goal{Value: cfg.BookValue * o.OfferWorth, Chance: chance}
			}
		}
		c.add(Action{Kind: ActMove, DX: dx, DY: dy, Effort: effort}, u)
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
	keep := keepValue(cfg, s, incoming, p.Foods[held].Nutrition, p.Foods[held].Heal,
		otherMeals(s, p.Foods[held].Nutrition), p.Foods[held].Spoils) // it is in hand, so not against itself
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

// addBooks scores everything to do with the written word (stage 69): picking
// one up, reading it, and writing one.
//
// Nothing new prices any of it. Reading is worth what being taught is worth,
// which is the Lore goal every exchange of what a body assumes already goes
// through. Writing is worth what the hand-over it makes possible is worth,
// which is the figure stage 48 puts on a gift and stage 49 on a cry - because
// handing it on is the only use a finished book has to whoever wrote it.
//
// That last line is the whole of the stage in one sentence. A book that has
// been read is worth nothing to its reader and something to everybody else,
// which is the first time that has been true of anything here, and it is
// exactly what stage 68 found a seller needs and never has.
func (c *AIController) addBooks(p *Perception) {
	cfg, s := p.Cfg, &p.Self
	if !cfg.Books || cfg.WriteTicks <= 0 {
		return
	}
	ticks := float64(cfg.WriteTicks)

	// Reading what is already in hand. No distance, no race: the only price
	// is standing there long enough.
	if s.BookInHand > 0 {
		c.add(Action{Kind: ActRead}, Utility{
			Lore:     Goal{Value: s.BookInHand, Chance: 1},
			Ticks:    ticks,
			TimeCost: ticks * cfg.TimeCost,
		})
	}

	// Writing one. What it will be worth is what handing it over buys, which
	// is the best such hand-over in sight - the same figure stage 48 puts on
	// a gift and stage 49 on a cry.
	//
	// It is worked out here rather than taken from addGive's bestGiftGain,
	// and that is not tidiness. A body has one hand: addGive only runs for a
	// body that is holding something, and writing only runs for a body with a
	// hand free, so the two are mutually exclusive and the borrowed figure was
	// always zero. Measured before it was noticed: seven bodies in a run could
	// write and not one ever did.
	if s.CanWrite {
		best := 0.0
		for i := range p.Others {
			o := &p.Others[i]
			if !o.LightRoom || o.Species != s.Species {
				continue
			}
			if g := trustBought(cfg, o.Affinity, cfg.AffinityGift); g > best {
				best = g
			}
		}
		if best > 0 {
			gain := cfg.BookValue * cfg.LoreValue * best
			c.add(Action{Kind: ActWrite}, Utility{
				Lore:     Goal{Value: gain, Chance: 1},
				Ticks:    ticks,
				TimeCost: ticks * cfg.TimeCost,
			})
		}
	}

	// And walking over to one lying about. It is scored the way anything
	// lying about is scored - what it is worth, less the walk - except that
	// nobody is racing for it: a book is worth something only to whoever
	// does not already know what it says, so two bodies wanting the same one
	// is not the usual case and is not assumed.
	if !s.LightRoom {
		return
	}
	for i := range p.Books {
		if i >= maxFoodOptions {
			break
		}
		f := &p.Books[i]
		if f.Worth <= 0 {
			continue
		}
		for _, effort := range effortLevels {
			walk := f.Dist/speedAt(s.MaxSpeed, effort) + 1
			cost := moveCost(cfg, s, effort) * walk
			c.add(Action{Kind: ActTake, TargetID: f.ID, Effort: effort}, Utility{
				Lore:         Goal{Value: f.Worth, Chance: 1},
				Vitality:     cost,
				Ticks:        walk,
				VitalityCost: cost * cfg.VitalityWeight,
				TimeCost:     walk * cfg.TimeCost,
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
// It is scored as exactly what it is: the same bet stage 40's pickup is, with
// one more thing that has to go right. Two conditions have to hold before a
// coin ever feeds anybody - this body has to run short, and somebody has to be
// willing to sell when it does - so both discounts are charged, and it is
// raced for like anything else lying on the ground.
//
// That is a correction, and what it corrects is what made money eat this
// world's economy (found in stage 67, fixed here). The chance was written as
// one and the race was left out, so a coin paid a single discount for two
// conditions while a berry paid one for one, and it was charged no weight
// because it has none: money was a meal with no race and no weight, and it
// beat an identical meal 70% of the time a body could see both. Hands filled
// with coins and stopped holding dinner - holders up, load down - and the
// giving and the cooking went with it. Priced this way a coin can never be
// worth more than the meal it claims, which is what coin.go has said in its
// own header since the day it was written. CoinPricedCertain puts the old
// world back.
//
// Nothing here is a second reason to want money. A coin is not saved, not
// counted, and not worth anything to a body that will never need a meal.
func (c *AIController) addCoins(p *Perception) {
	cfg, s := p.Cfg, &p.Self
	if len(p.Coins) == 0 || !s.LightRoom {
		return
	}
	want, need := coinWorth(cfg, s, otherMeals(s, 0)), carryNeed(cfg, s)
	if cfg.CoinPricedCertain {
		// The world stage 51 measured: one discount for both conditions, no
		// race, and a sure thing.
		want, need = want*clamp(s.FoodScarcity, 0, 3)/3, 1
	}
	if want <= 0 || need <= 0 {
		return
	}
	for i := range p.Coins {
		if i >= maxFoodOptions {
			break
		}
		f := &p.Coins[i]
		chance := need
		if !cfg.CoinPricedCertain {
			chance *= raceChance(cfg, s, f.Dist, f.RivalDist)
		}
		for _, effort := range effortLevels {
			ticks := f.Dist/speedAt(s.MaxSpeed, effort) + 1
			cost := moveCost(cfg, s, effort) * ticks
			c.add(Action{Kind: ActTake, TargetID: f.ID, Effort: effort}, Utility{
				Life:         Goal{Value: want, Chance: chance},
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
	// Anything held out that this body would have. Until stage 84 this asked
	// for nutrition, which is a question only a meal can answer: a book and
	// an ornament have none, so neither could be bought at any price, in any
	// world, by anybody. Nothing said so - the rest of this function has
	// priced both of them since the stages that added them - and it is the
	// same mistake as the canEat list stage 82 found, a guard written for the
	// kinds that existed when it was written.
	if !s.HasCoin || !o.Selling {
		return
	}
	if o.OfferValue <= 0 && (cfg.CoinBuysOnlyMeals || o.OfferWorth <= 0) {
		return
	}
	// What is on the counter. A book is worth what it would say to this body
	// and nothing else (stage 69); everything else is worth the meal it is.
	meal := o.OfferWorth * cfg.BookValue
	if o.OfferKind == FoodTrinket {
		meal = o.OfferWorth // already what it is worth, to anybody (stage 82)
		meal += warmthValue(cfg, s, c.incomingDmg, o.OfferWard)
	}
	if o.OfferValue > 0 || o.OfferHeal > 0 {
		meal = mealValue(cfg, s, c.incomingDmg, o.OfferValue, o.OfferHeal)
		if kept := keepValue(cfg, s, c.incomingDmg, o.OfferValue, o.OfferHeal,
			otherMeals(s, cfg.CoinValue), o.OfferSpoils); kept > meal { // the coin leaves the hand
			meal = kept
		}
	}
	// And what buying from this one earns, if a sale earns anything (stage
	// 68). The goodwill is written both ways, so the buyer is told about the
	// half that is its own: a body should not be made to pay for something it
	// cannot see it is getting. It is the same figure the seller weighs, from
	// the other end.
	// What it would cost, in coins (stage 80). What a particular seller would
	// take is that seller's own state and hidden, so a buyer reckons on the
	// standard - and a body that cannot raise it does not set out, which is
	// the same answer it would get at the counter.
	price := float64(standardPrice(cfg))
	if s.Coins < int(price) {
		return
	}
	// And whether there will be a hand for it once the money has gone. Before
	// stage 80a the coin was in the only hand there was, so paying always
	// freed one and nothing had to ask; where a coin takes no hand, paying
	// frees nothing.
	if cfg.CarrySlotsWeigh && cfg.CarrySlotted {
		room := s.CarryRoom
		if weightless(o.OfferKind) {
			room = s.LightRoom
		}
		if !room {
			return
		}
	}
	gain := meal + saleGoodwill(cfg, o.Affinity) -
		price*coinWorth(cfg, s, otherMeals(s, cfg.CoinValue*price))
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

// addCraft scores making something worth looking at (stage 82).
//
// What it is worth is the want itself, which is the one figure in this world
// that stands for nothing else: there is no chance attached because there is
// nothing to go wrong - a body that spends the time gets the thing - and the
// luck of the piece is not reckoned with, because a body cannot know how this
// one will come out until it has made it.
//
// What it costs is vitality and time, in the shape picking a stone up already
// uses. That price is the point rather than an inconvenience: a want that
// arrives free runs to the ceiling (stage 17b), and a maker that can make them
// freely is a supply with no shortage in it.
func (c *AIController) addCraft(p *Perception) {
	cfg, s := p.Cfg, &p.Self
	if !s.CanCraft {
		return
	}
	// What it expects to end up with: its own hands, the luck of the piece it
	// cannot know in advance (CraftDelight), and whether it will be there to
	// enjoy the thing at all (stage 84). The last is the whole difference
	// between this and going to eat: a meal is worth more to a body that is
	// running out, and an ornament is worth less.
	want := cfg.TrinketValue * cfg.LifeValue * s.CraftQuality * s.CraftDelight * s.AdornWant
	// And what it might keep off the weather (stages 87a, 88). A maker cannot
	// aim - not at how good it comes out, not at what it answers - so it
	// reckons on how often one comes out answering at all, and on the weather
	// where it is standing, which is the only one it can price.
	want += clamp(cfg.WardShare, 0, 1) *
		warmthValue(cfg, s, c.incomingDmg, clamp(cfg.WardStrength, 0, 1)*s.ChillRaw)
	if want <= 0 {
		return
	}
	ticks := float64(cfg.CraftTicks)
	c.add(Action{Kind: ActCraft}, Utility{
		Adorn:        Goal{Value: want, Chance: 1},
		Vitality:     cfg.CraftVitality,
		Ticks:        ticks,
		VitalityCost: cfg.CraftVitality * cfg.VitalityWeight,
		TimeCost:     ticks * cfg.TimeCost,
	})
}

// addTrinkets scores walking over to one lying about (stage 82).
//
// They get there by being dropped or by their owner dying, so this is the
// same option picking up money is, with the want in place of the claim - and
// it is raced for the way anything lying about is, because two bodies that
// both want the same piece is exactly the situation this stage is about.
func (c *AIController) addTrinkets(p *Perception) {
	cfg, s := p.Cfg, &p.Self
	if len(p.Trinkets) == 0 || !s.LightRoom || !cfg.Trinkets {
		return
	}
	for i := range p.Trinkets {
		if i >= maxFoodOptions {
			break
		}
		f := &p.Trinkets[i]
		if f.Worth <= 0 {
			continue
		}
		chance := raceChance(cfg, s, f.Dist, f.RivalDist)
		warmth := warmthValue(cfg, s, c.incomingDmg, f.Ward)
		for _, effort := range effortLevels {
			ticks := f.Dist/speedAt(s.MaxSpeed, effort) + 1
			cost := moveCost(cfg, s, effort) * ticks
			c.add(Action{Kind: ActTake, TargetID: f.ID, Effort: effort}, Utility{
				Life:         Goal{Value: warmth, Chance: chance},
				Adorn:        Goal{Value: f.Worth, Chance: chance},
				Vitality:     cost,
				Ticks:        ticks,
				VitalityCost: cost * cfg.VitalityWeight,
				TimeCost:     ticks * cfg.TimeCost,
			})
		}
	}
}

// addDrop scores putting something down (stage 70).
//
// Stage 40 decided there should be no such word, and gave a reason that was
// true at the time: a body can eat what it is holding, so no hand is ever
// stuck. Three things that cannot be eaten have gone into hands since, and a
// hand holding one of them is stuck until the thing is sold or its owner dies
// - which stage 51a measured, and which is why money filled two thirds of the
// hands in this world and the giving and the cooking stopped.
//
// What putting something down is worth is three figures, none of them new:
//
//   - what it stops paying to carry between now and the moment it would be
//     eaten, which is the same lug picking it up was charged;
//   - what a free hand is worth, which is the best pickup this body scored
//     and could not offer itself (roomWorth), and nothing at all when it has
//     a hand free already;
//   - less what is given up, which is the seller's side of a sale - because
//     putting something down is selling it to nobody.
//
// No threshold anywhere: a body with a hand free is offered this too, and the
// comparison is what says no.
//
// Counted before it was written (played map, 25-tick samples): a hand is full
// in 44.5% of moments in the money world and 74.2% once the world can see
// ahead; room is worth 0.00 in both, because nothing on the ground is worth
// carrying at the moment a hand is full; and the lug is worth 2.33 and 0.82.
// So the arithmetic says what this word will do before it does it: it will
// put down food a fed body is paying to carry, and it will never put down a
// coin - a coin weighs nothing, so dropping one saves nothing, and it is
// worth CoinValue of a meal against a hand worth need x race of one.
func (c *AIController) addDrop(p *Perception) {
	cfg, s := p.Cfg, &p.Self
	if !cfg.Dropping || len(p.Held) == 0 {
		return
	}
	// A hand that is already free is not something this can buy.
	room := c.roomWorth
	if s.CarryRoom {
		room = 0
	}
	// When it would be eaten, which is how long it would be carried for.
	wait := 0.0
	if s.HungerRate > 0 {
		wait = clamp((cfg.StarveHunger-s.Hunger)/s.HungerRate, 0, cfg.PlanHorizon)
	}
	for i := range p.Held {
		h := &p.Held[i]
		lug := 0.0
		if !weightless(h.Kind) {
			// What the legs are charged for it. Money and books weigh
			// nothing (#66, stage 69), so putting one down saves nothing -
			// which is most of why this word will not empty a hand of money.
			lug = (burdenOf(s) - burdenWith(cfg, s, -1)) *
				moveCostAt(cfg, 0.4) * groundOf(s) * wait
		}
		gain := lug + room - handWorth(cfg, s, h)
		if gain <= 0 {
			// Nothing to be had by it, so it is not an option - the same
			// guard picking a coin up, a stone or a cache already has.
			// Leaving it in was measured and it is not free: an option worth
			// nothing or less is still an option the misjudgement can pick,
			// and with this one left open a fifth of the drops in the money
			// world and four fifths of them in the world that can see ahead
			// were bodies throwing away what they had just valued above
			// everything in sight.
			continue
		}
		c.add(Action{Kind: ActDrop, TargetID: h.ID}, Utility{
			Life:     Goal{Value: gain, Chance: 1},
			Ticks:    1,
			TimeCost: cfg.TimeCost,
		})
	}
}

// handWorth is what this body would be giving up by parting with something it
// is holding: the same figure the seller of it weighs (willSell), because a
// sale and a gift and putting something down all give up the same thing.
func handWorth(cfg *Config, s *SelfView, h *FoodView) float64 {
	switch h.Kind {
	case FoodCoin:
		return coinWorth(cfg, s, otherMeals(s, cfg.CoinValue))
	case FoodBook:
		return h.Worth // what it would still tell its owner, which once read is nothing
	case FoodTrinket:
		// What this particular piece is worth, less what is left of it - and
		// what it keeps off the weather here (stage 87a), which is the one
		// figure in this world that is large in one place and nought in
		// another without the object changing at all.
		return h.Worth + warmthValue(cfg, s, 0, h.Ward)
	case FoodStone:
		return stoneWorth(cfg, s)
	}
	v := mealValue(cfg, s, 0, h.Nutrition, h.Heal)
	if kept := keepValue(cfg, s, 0, h.Nutrition, h.Heal, otherMeals(s, h.Nutrition),
		h.Spoils); kept > v {
		v = kept
	}
	return v
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
// stoneWorth is what having a stone is worth to this body: a throw that has
// not happened yet, at a rival that may not be there yet, discounted the way a
// meal carried for later is and scaled by how contested this patch feels. One
// place, because picking one up (stage 46) and deciding not to put it down
// again (stage 70) are the same figure.
func stoneWorth(cfg *Config, s *SelfView) float64 {
	// What one stone would do to an ordinary body, as a share of finishing it.
	damage := damagePerTick(cfg, s.Attack, 1) * cfg.ThrowDamage * cfg.ThrowHit
	share := clamp(damage/math.Max(cfg.MaxVitality, 1e-9), 0, 1)
	return cfg.CarryValue * s.CompetitionWeight * cfg.LifeValue *
		clamp(s.FoodScarcity, 0, 3) / 3 * share
}

func (c *AIController) addStones(p *Perception) {
	cfg, s := p.Cfg, &p.Self
	if !cfg.Throwing || !s.CarryRoom || cfg.CarryValue <= 0 || len(p.Stones) == 0 {
		return
	}
	want := stoneWorth(cfg, s)
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
	now := pressures(cfg, s, s.Vitality, s.Hunger, c.incomingDmg)
	eased := c.incomingDmg
	if o.AttackingMe {
		eased = math.Max(0, c.incomingDmg*(1-share))
	}
	// A throw is a throw: the body is not guarding or dodging while it does
	// it, so what it pays is the aggressive stance's cost and no more.
	cost := stanceCost(cfg, StanceAggressive) * effort
	after := pressures(cfg, s, s.Vitality-cost, s.Hunger, eased)
	lifeTerm := gap(cfg, now, after) * cfg.LifeValue

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

	now := pressures(cfg, s, s.Vitality, s.Hunger, c.incomingDmg)
	after := pressures(cfg, s, s.Vitality-cost, s.Hunger+s.HungerRate*ticks, 0)
	lifeTerm := gap(cfg, now, after) * cfg.LifeValue

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

	incoming := damagePerTick(cfg, o.EstStrength, 1)
	staying := pressures(cfg, s, s.Vitality, s.Hunger, incoming)

	// Breaking away is not free: for a while the agent is still in reach and
	// is the one not hitting back, which is the cheapest thing there is to
	// hit. This is the only option that gets the incoming damage out of the
	// picture, which is why running away wins exactly when the damage is what
	// is about to kill the agent, and loses whenever it is not.
	cost := moveCost(cfg, s, cfg.FleeEffort)*fleeExposureTicks + incoming*fleeExposureTicks*0.4
	pEscape := clamp(s.Vitality/(s.Vitality+o.Vitality+1e-9), 0.15, 0.9)
	fled := pressures(cfg, s, s.Vitality-cost, s.Hunger, 0)

	c.add(Action{Kind: ActFlee, TargetID: o.ID, Effort: cfg.FleeEffort, Stance: StanceEvasive}, Utility{
		Life:         Goal{Value: gap(cfg, staying, fled) * cfg.LifeValue, Chance: pEscape},
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
	now := c.riskNow
	after := pressures(cfg, s, s.Vitality-cost-birth, s.Hunger, c.incomingDmg)

	// A child is worth something to a body that is there to have it (stage
	// 73). The life term prices this body's own survival; this prices the
	// event that survival is a condition of, and leaving it out is what let a
	// starving body court instead of eat - the life term shrinks as the
	// window it is read over lengthens, and a constant does not.
	c.add(Action{Kind: ActCourt, TargetID: o.ID, Effort: effort}, Utility{
		Offspring:    Goal{Value: s.MateWeight * clamp(o.Fitness/MaxAbility, 0, 1), Chance: s.AcceptChance * survives(cfg, after.far, ticks+float64(cfg.PairBondDuration))},
		Life:         Goal{Value: gap(cfg, now, after) * cfg.LifeValue, Chance: 1},
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
		// And what knowing this sort of beast takes off its blows (stage 62).
		// It is applied to the threat rather than to the estimate of how hard
		// it hits: the other one is as strong as it is, and what this body
		// knows is what it can do about it.
		threat *= 1 - o.Ward

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
