package engine

import (
	"math"
	"math/rand"
)

// SelfView is what an agent knows about itself, which is everything.
type SelfView struct {
	ID       int
	X, Y     float64
	Sex      Sex
	Species  Species
	Vitality float64
	Hunger   float64

	Attack       float64
	Rationality  float64
	Intelligence float64

	// What this agent's body can hold and how fast it can move it: the
	// world's reference figures scaled by its own genes. The controller needs
	// them because "half full" and "how long to get there" are questions
	// about this body, not about an average one.
	MaxVitality float64
	MaxSpeed    float64

	// HungerRate is what this body costs to run: bigger agents get hungry
	// faster. The controller plans with its own rate rather than the world's,
	// or the agents the budget is pricing would be the ones misjudging how
	// long they have.
	HungerRate float64

	// Defence and Evasion are what this agent's own guard and footwork are
	// worth at full use: the fraction of a blow it can turn aside and the
	// chance of one missing entirely. It knows its own; what somebody else
	// can do stays hidden, as every other ability does.
	Defence float64
	Evasion float64

	// CanReproduce is false while the agent still has to look after itself.
	CanReproduce bool

	// AttackerID is who is currently hitting this agent, 0 if nobody.
	AttackerID int

	// FoodScarcity is how contested the neighbourhood feels: roughly how many
	// other agents there are per food item in sight. It is the crude proxy the
	// pre-emptive attack rule leans on, so that a well stocked world removes
	// the motive for violence all by itself.
	FoodScarcity float64

	// What this agent assumes when it works out what an option is worth: two
	// claims about the world it has been finding out (how often the one you
	// hit hits back, how often a proposal is accepted) and three preferences
	// it was born with (lore.go). They were the same numbers for everybody
	// until stage 12; the controller reads them from here now, so that being
	// wrong about the world, or wanting different things out of it, is
	// something an agent can be.
	Retaliation  float64
	AcceptChance float64

	RiskWeight        float64
	CompetitionWeight float64
	ShockRisk         float64

	// Hints are this agent's own rules of thumb (stage 12c). They are its
	// own, so they belong here; they read nothing that is not already in this
	// Perception, and all they can do is add to an option's score.
	Hints []Hint

	// Nutrition is what one of each kind of food is worth to this agent right
	// now, as a share of what it would be worth to one that had not been
	// living on it (stage 16). Deliberately not hidden: an animal knows when
	// it is sick of something.
	Nutrition [NumFoodKinds]float64

	// Missing is how much the direction the body it thinks best of went in is
	// still worth, from 0 to 1, and MissingDX/DY is that direction (stage
	// 55a). Zero when there is nobody to miss, when the one it thinks best of
	// is in sight, or when the direction has gone stale.
	//
	// A direction and never a place: there is nothing here to walk to, check
	// or tell anybody, which is what keeps this out of the coordinate-level
	// knowledge that is still shelved (#70).
	Missing              float64
	MissingDX, MissingDY float64

	// CookQuality is how well this body cooks, from 0 to 1 (stage 52). A body
	// knows its own hands: this is the same reading Self.Ground is of the
	// ground underfoot, and it is what makes one body's work worth more than
	// another's.
	CookQuality float64

	// CanCook says there is something in this body's hands that it could
	// make better than it is (stage 52). It is a fact about the hands and the
	// body's own skill, which is why it is worked out here rather than in the
	// controller: what a body can do with what it is holding is not a
	// judgement.
	CanCook bool

	// HasStone says there is something to throw in this body's hand (stage
	// 46). What it would be worth throwing at is on the other side, in
	// AgentView.
	HasStone bool

	// Burden is what this body's load multiplies the cost of moving by
	// (stage 40). One for empty hands, which is every body in a world with
	// HasCoin says this body has money on it (stage 51).
	HasCoin bool

	// carrying off. Carried is how many items it is holding and CarryRoom
	// whether there is space for one more: what a body knows about its own
	// hands, and nothing about anybody else's.
	Burden        float64
	Carried       int
	CarryRoom     bool
	CarryCapacity float64

	// Heal is what one of each kind would put back into this body's vitality
	// (stage 39). Zero for everything a world's carcasses do not mend, which
	// is every kind in a world with the rule off. Not hidden, for the same
	// reason Nutrition is not: a body knows what a meal does for it.
	Heal [NumFoodKinds]float64

	// BetterGround is how much more food this agent believes is to be found
	// somewhere it has been than where it is standing, and BetterGroundX/Y
	// where that is (stage 15b). Zero when it knows nowhere better - which
	// includes knowing nowhere at all.
	//
	// It is a belief and not a fact: it is what the agent has made of the
	// ground it has walked, blurred by how well it reads the world, faded by
	// how long ago it was there, and possibly something it was simply told
	// (stage 15c). The world's own figure never appears here.
	BetterGround  float64
	BetterGroundX float64
	BetterGroundY float64

	// RestRate is what lying down would actually mend per tick at this hour
	// (stage 18). An agent knows whether it is sleepy: this is its own body
	// and not a reading of anything.
	RestRate float64

	// Shelter is how exposed lying down right here would be, as a multiplier
	// on RestExposureWeight (stage 14). It is about the spot the agent is
	// standing on rather than about anybody else, which is why it is in this
	// half of the perception: an agent can feel whether its back is covered
	// where it is, and cannot see whether it would be covered over there.
	Shelter float64

	// What this agent is worth as a mate and what it is holding out for.
	//
	// Both are its own. MateValue is built out of two things it knows about
	// itself - what others can see of its build, and the shape it is in -
	// though what a candidate makes of it will be that figure plus their own
	// misjudgement. MateBar is its own rule for accepting somebody, and it
	// comes down to CommitFloor once it has been looking long enough.
	MateValue float64
	MateBar   float64

	// Ground is what a tick of movement costs where this agent is standing,
	// as a multiplier on the flat figure (stage 20). One is level open
	// ground, which is the whole of a world with no map.
	//
	// It is the ground underfoot and nothing else: an agent feels what it is
	// standing on and assumes the country ahead is like it. Nothing here says
	// what is over there, which is why terrain is something to be found out by
	// paying for it rather than a straight line with a different price on it.
	// A player is shown the same figure and no more (stage 19).
	Ground float64

	// PoisonResist is how much of a plant's dose this body turns aside (stage
	// 38b). It is here rather than folded into each plant's Danger because
	// the two are different things: Danger is what the warning says, which is
	// a reading of the world, and this is what the body would make of it,
	// which is a fact about itself - the same split as Defence against a blow.
	PoisonResist float64

	// Drown is the chance this ground ends the agent within the tick (stage
	// 34). Its own footing again, and nothing about the country ahead: an
	// agent feels the current it is standing in, and finds out about the next
	// river by walking into it. Knowing which places are dangerous before
	// going there is what a later stage buys with hearing about it.
	Drown float64

	// Covered is set when whoever is currently hitting this agent is hitting
	// it from below (stage 30), so that the estimate of what the next few
	// ticks will cost knows the ground is helping. Nothing when nobody is
	// hitting it: this is about the fight it is in, not about the view.
	Covered bool

	// CourtedBy is whoever is standing here proposing and waiting for an
	// answer, 0 for nobody, and CourtedTicksLeft how long they will wait
	// before the agent's own rule answers for it. Only an agent whose
	// controller answers proposals ever sees these filled in.
	CourtedBy        int
	CourtedTicksLeft int

	// LastCourt is what came of the last courtship this agent walked all the
	// way up to. Nothing here is new knowledge: the world already teaches the
	// suitor whether the other side agreed (that is what AcceptChance is
	// learned from), and the rest is this agent's own answer and its own
	// judgement of the candidate.
	LastCourt CourtView
}

// CourtView is one courtship that came to a head: who it was with, when, which
// side said no, and the two numbers this agent's own answer was made of.
//
// Both sides get one, so an agent that was courted knows it turned somebody
// down as surely as one that was turned down knows it.
type CourtView struct {
	TargetID int // who it was with, 0 if this agent has never got that far
	Tick     int // when it came to a head

	Accepted     bool // this agent's own answer
	TheyAccepted bool // and the other side's

	// Fitness is what this agent made of the candidate (its own estimate,
	// including its own misjudgement), and Bar what it was holding out for at
	// that moment.
	Fitness float64
	Bar     float64
}

// FoodView is one food item as an agent sees it.
// StoreSight is a cache in sight that this agent knows how to find (stage 50).
// A body that does not know the place has none of them in its perception, and
// standing on one tells it nothing.
type StoreSight struct {
	Index int
	X, Y  float64
	Dist  float64

	// Room is how many more items it would hold. Not hidden: a body that
	// knows the place and can see it can see what is in it.
	Room int
}

type FoodView struct {
	ID   int
	X, Y float64
	Dist float64

	// Kind is what it is. Not hidden: what something is is written on the
	// outside of it, the same way a creature's species is.
	Kind FoodKind

	// Nutrition is what this item is worth to this agent right now, as a
	// share of what it would be worth to one that had not been living on the
	// same thing (stage 16). One for an agent with a varied diet.
	Nutrition float64

	// Heal is what this item would mend of this agent's vitality (stage 39),
	// before the ceiling of what it is actually missing.
	Heal float64

	// Catch is the chance this body lands it if it gets to it (stage 43).
	// One for everything but a fish, and one for a fish too in a world whose
	// water gives them up freely. It is read from where the body is standing
	// now, which is the same assumption about the ground ahead that every
	// other estimate here makes.
	Catch float64

	// Cooked is how well this one has been prepared (stage 52), zero for
	// anything nobody has done anything to. It is not hidden: what has been
	// done to a thing is written on the outside of it, the same way what it
	// is is. What it is worth is already in Heal - this is here so that an
	// agent can tell whether there is anything left to do to it, and so that
	// a human player can see the same.
	Cooked float64

	// Held says this one is already in the agent's own hands (stage 40).
	// Nothing about how it is scored changes - it is a meal at no distance
	// with nobody racing for it - but the option to pick something up is
	// only offered for what is not held, and a human player is shown which
	// is which.
	Held bool

	// Store is which cache this item is in, plus one, and zero for anything
	// lying in the open (stage 50). An item in a store is only in this list
	// at all if the agent knows the place, so what this says is not "there is
	// one" but "this one is in the cache" - which is what tells an agent that
	// walking there and putting something else in is a thing it could do.
	Store int

	// Danger is how poisonous this observer reckons the item is, from 0 to 1
	// (stage 17b). It is a reading of the plant's warning, blurred by the
	// observer's own rationality - the plant's actual poison is a hidden
	// parameter and is not here, exactly as an agent's combat power is not in
	// AgentView. An agent finds out what it really ate by eating it.
	Danger float64

	// RivalDist is how far the nearest other agent is from this item. Getting
	// there first is a race, and this is what tells the agent its odds.
	RivalDist float64
	// RivalID is that agent, 0 when nobody else is near enough to matter.
	RivalID int
}

// AgentView is somebody else as an agent sees them.
//
// It deliberately carries no true ability: combat power is a hidden parameter,
// and all a controller ever gets is its own estimate of it, plus how unsure
// that estimate is. Everything here has already been blurred according to the
// observer's rationality.
type AgentView struct {
	ID       int
	X, Y     float64
	Dist     float64
	Sex      Sex
	Vitality float64 // visible from the body's condition

	// Species is what kind of creature this is. Unlike an ability it is not
	// hidden: what something is is written on the outside of it.
	Species Species

	// Prey says this one is worth killing for what is left of it: a creature
	// of a kind this agent eats, and Meat is roughly how many mouthfuls its
	// carcass would leave. Both are what the observer can judge by looking -
	// a big animal is visibly a big animal - not what the world knows.
	Prey bool
	Meat float64

	Paired  bool
	Seeking bool // looks like it is after a mate, though not of whom
	Resting bool // lying down, which is visible and matters (stage 18)

	// What this one is doing about the observer in particular. Seeking says
	// somebody is after a mate; these two say they are coming here, and the
	// difference between them is the difference between a courtship and a
	// fight - which is not something to be worked out after the first blow.
	//
	// Both are the same kind of fact: an animal walking straight at you is
	// doing something visible, and nothing here reveals an ability or a
	// decision that has not been taken yet. An agent crossing the ground with
	// no target picked out shows neither flag, because there is nothing to
	// show: it has not decided anything about anybody.
	// ThrowHit is the chance a stone thrown at this one from here finds its
	// mark, before its own guard and footwork are asked about (stage 46).
	// Zero when there is no throwing in this world, when it is out of range,
	// or when the body doing the reckoning has nothing to throw - so an
	// option that cannot be taken is never scored.
	ThrowHit float64

	// Selling is set while this one is crying its wares and what it is
	// holding out is food (stage 51). It is the same visible fact stage 49
	// put in Offering, read from the other side: a body advertising a meal is
	// a body somebody with a coin can walk up to. Nothing here says whether
	// it will part with it - that is asked when the buyer arrives.
	Selling bool

	// CarryRoom says this one has a hand free (stage 48). Whether somebody
	// can be handed a thing is as visible as whether they are carrying one.
	CarryRoom bool

	AttackingMe bool
	CourtingMe  bool

	// DeclaredFor is what this one has declared itself against: the target it
	// is calling others in to bring down, or the one it is already hitting
	// (stage 32). Zero when it has taken nothing on.
	//
	// It is the same kind of fact as the two above, seen from the side rather
	// than from in front: an animal shouting at a carcass, or one already in a
	// fight, is doing something anybody watching can see. Nothing hidden is in
	// here - not how hard it will fight, not whether it will still be there in
	// ten ticks. Whether to count on it is what trust is for.
	DeclaredFor int

	// Offering is set while this one is crying its wares (stage 49): the item
	// it is holding out, what it is, and how much of the cry is left to run.
	// False for everybody in a world with the rule off, and false for
	// somebody holding out what this one cannot eat.
	//
	// It is the same kind of fact as DeclaredFor above: read off the current
	// action, stored nowhere, and visible in the plainest sense - a body
	// standing there holding something up is doing something anybody watching
	// can see. What is not in here is what it will do when somebody arrives.
	//
	// OfferValue and OfferHeal are what that item would be worth to whoever
	// is looking, not to the one holding it: the sameness of a diet is the
	// looker's own (stage 16), and so is how much of a wound is left to mend.
	//
	// OfferRivalDist is how far the nearest other body in sight is from those
	// wares: an advertised item goes into one pair of hands, and everybody who
	// heard the cry is walking for it. Infinite when nobody else is in sight,
	// and meaningless unless Offering is set.
	Offering       bool
	OfferKind      FoodKind
	OfferLeft      int
	OfferValue     float64
	OfferHeal      float64
	OfferRivalDist float64

	// Uphill is set when this agent is standing a level or more above the one
	// looking at it (stage 30), which is what makes it harder to hit. It is a
	// relation and not a property - the first thing in this perception that
	// depends on where both bodies are - and it is visible in the plainest
	// sense: an animal can see that the one in front of it is up a bank.
	Uphill bool
	// Rejected is set for a candidate this agent recently walked away from and
	// is not interested in comparing again just yet.
	Rejected bool

	EstStrength float64 // believed power
	Uncertainty float64 // variance of that belief
	Risk        float64 // vitality this one has already cost the observer

	// Affinity is how much good the observer remembers of this one: a partner,
	// a parent, a child. It is the observer's own record, not a property of
	// the other, and the other may well not return it.
	Affinity float64

	// Fitness is how good a mate they look, ability and condition together,
	// already blurred by the observer's rationality.
	Fitness float64

	// Appearance is what can be seen of the body itself - how much of it there
	// is and how fast it moves - as this observer read it this tick. The true
	// abilities are still not here: this is the correlate an agent learns to
	// interpret for itself (appearance.go), and EstStrength is already what it
	// made of it when there is nothing else to go on.
	Appearance float64
}

// Perception is the slice of the world a controller gets to reason about. The
// slices are owned by the world and are rebuilt for the next decision, so a
// controller must not hold on to them.
type Perception struct {
	Tick   int
	Cfg    *Config
	Self   SelfView
	Foods  []FoodView
	Others []AgentView

	// Stones is what is lying about that cannot be eaten (stage 45). Kept
	// apart from Foods so that nothing about eating has to learn the word:
	// how contested a patch feels, how rich the ground looks and when a meal
	// was last in sight all count what is edible, and a stone is not.
	Stones []FoodView

	// Stores is the caches in sight that this agent knows about (stage 50).
	// Kept apart from Foods for the reason the stones are: a cache is not a
	// meal, and none of the figures that count what is edible should count
	// one. An empty store is in here - which is the point, because an empty
	// store is exactly the one worth walking to with something in your hand.
	Stores []StoreSight

	// Coins is the money in sight (stage 51). Kept apart from Foods for the
	// reason the stones are: none of the figures that count what is edible
	// should count one, and money is the least edible thing in the world.
	Coins []FoodView

	// Trigger is why the engine is asking. It is not a instruction - what to
	// do about being hit is still for the controller to work out - but it is
	// something the agent knows about its own situation, and one thing cannot
	// be worked out without it: an agent asked because what it was after is
	// gone (TriggerTargetLost) cannot tell from this perception whether the
	// target was taken or has merely walked out of sight. The AI ignores it
	// and re-scores everything; a human controller uses it to drop an order
	// that has been overtaken by events.
	Trigger Trigger

	// Rand is the simulation's single random source. A controller that needs
	// to break a tie draws from it, so that a run stays reproducible from its
	// seed; a human controller ignores it.
	Rand *rand.Rand

	// Trace is where a controller records the options it compared, and is nil
	// unless somebody asked to follow this agent (World.TrackDecisions).
	// Filling it in is optional: the world records the trigger and the chosen
	// action either way.
	Trace *DecisionTrace
}

// selfView is what an agent knows about itself. It is pulled out of perceive
// so that a rule which has to reckon on somebody else's terms can ask for it -
// whether a seller would part with what it is holding (stage 51) is the first
// such rule, and it is asked of the seller, not of the body doing the asking.
func (w *World) selfView(a *Agent) SelfView {
	ground := w.terrainAt(a.X, a.Y)
	return SelfView{
		ID:           a.ID,
		X:            a.X,
		Y:            a.Y,
		Sex:          a.Sex,
		Species:      a.Species,
		Vitality:     a.Vitality,
		Hunger:       a.Hunger,
		Attack:       a.Attack(&w.cfg),
		Rationality:  a.Rationality(&w.cfg),
		Intelligence: a.Intelligence(&w.cfg),
		MaxVitality:  a.MaxVitality(&w.cfg),
		MaxSpeed:     a.MaxSpeed(&w.cfg),
		HungerRate:   a.HungerRate(&w.cfg),
		Defence:      w.cfg.DefenceCap * a.Gene(GeneDefence) / MaxAbility,
		Evasion:      w.cfg.EvasionCap * a.Gene(GeneEvasion) / MaxAbility * clamp(a.MaxSpeed(&w.cfg)/w.cfg.MaxSpeed, 0, 2),
		CanReproduce: a.CanReproduce(&w.cfg),
		AttackerID:   a.attackerID,
		Covered:      w.coveredFromAttacker(a),

		Retaliation:       a.lore.retaliation.mean,
		AcceptChance:      a.lore.accept.mean,
		RiskWeight:        a.lore.riskWeight,
		CompetitionWeight: a.lore.competitionWeight,
		ShockRisk:         w.shockRiskFelt(a),
		Hints:             a.hints,
		Shelter:           w.shelterAt(a.X, a.Y),
		Ground:            w.groundCostFor(a, ground),
		PoisonResist:      w.poisonResist(a),
		Drown:             w.drownFelt(a, ground),
		CourtedBy:         a.courtedBy,
		CourtedTicksLeft:  w.courtAnswerLeft(a),
		MateValue:         fitness(a, &w.cfg),
		MateBar:           w.commitBar(a),
		LastCourt:         a.lastCourt,
		RestRate:          w.restRate(a),
		Nutrition:         w.mealValues(a),
		Heal:              w.mealHeals(a),
		Burden:            a.burden(&w.cfg),
		Carried:           len(a.carried),
		CarryCapacity:     a.carryCapacity(&w.cfg),
		CarryRoom:         a.canCarryMore(&w.cfg),
		HasStone:          a.canThrow(&w.cfg),
		CanCook:           w.canCook(a),
		CookQuality:       w.cookQuality(a),
		HasCoin:           a.carriedIndex2(FoodCoin) >= 0,
	}
}

// perceive fills the world's reusable perception buffer for one agent.
func (w *World) perceive(a *Agent) *Perception {
	p := &w.perception
	p.Tick = w.tick
	p.Trigger = TriggerNone
	p.Cfg = &w.cfg
	p.Rand = w.rng
	p.Foods = p.Foods[:0]
	p.Stones = p.Stones[:0]
	p.Stores = p.Stores[:0]
	p.Coins = p.Coins[:0]
	p.Others = p.Others[:0]

	p.Self = w.selfView(a)

	// The index narrows the world down to the cells sight could possibly reach;
	// what is actually visible is still tested one by one below, exactly as it
	// was when this walked every agent and every item. The candidates arrive in
	// ascending index order, which is the order those loops used, so the
	// perception buffers are filled identically and the random draws below
	// happen in the same sequence.
	// This look's own scratch flag (stage 49), cleared here so that it says
	// something about this look and not the last one.
	w.sawOffer = false

	w.nearFoods = w.appendFoodsInSight(w.nearFoods[:0], a.X, a.Y)
	w.nearAgents = w.appendAgentsInSight(w.nearAgents[:0], a.X, a.Y)

	for _, i := range w.nearFoods {
		f := &w.foods[i]
		if !w.canSee(a.X, a.Y, f.X, f.Y) {
			continue
		}
		d2 := dist2(a.X, a.Y, f.X, f.Y)
		// A stone is in sight like everything else, and it is not food
		// (stage 45). Nothing values one yet - what they are for is stage 46
		// - so no option is made from this list; it is here because the body
		// can see them, which is what the perception is.
		if f.Kind == FoodStone {
			p.Stones = append(p.Stones, FoodView{
				ID: f.ID, X: f.X, Y: f.Y, Dist: math.Sqrt(d2), Kind: f.Kind,
				RivalDist: math.Inf(1),
			})
			continue
		}
		// And money, in its own list for the same reason (stage 51).
		if f.Kind == FoodCoin {
			p.Coins = append(p.Coins, FoodView{
				ID: f.ID, X: f.X, Y: f.Y, Dist: math.Sqrt(d2), Kind: f.Kind,
				RivalDist: math.Inf(1),
			})
			continue
		}
		// And what is in a cache is nothing to a body that does not know the
		// cache (stage 50). It is in plain sight in every other sense - the
		// world holds it in the same list as everything else lying about -
		// but a place to keep things is not a landmark, and knowing where one
		// is is the whole of what stage 50 is about.
		if f.Store > 0 && !w.knowsStore(a, f.Store-1) {
			continue
		}
		// What this agent cannot eat is not food to it: a carcass of its own
		// kind, or somebody else's kill while the claim on it still stands.
		if !w.canEat(a, f) {
			continue
		}
		p.Foods = append(p.Foods, FoodView{
			ID:        f.ID,
			X:         f.X,
			Y:         f.Y,
			Dist:      math.Sqrt(d2),
			Kind:      f.Kind,
			Store:     f.Store,
			Nutrition: p.Self.Nutrition[f.Kind],
			Heal:      w.itemHealKnown(a, f),
			Cooked:    f.Cooked,
			Catch:     w.catchExpected(a, f),
			Danger:    w.dangerOf(a, f),
			RivalDist: math.Inf(1),
		})
	}

	// How badly this one reads anything, worked out once for the whole crowd.
	unit := w.judgementScale(a)

	// And who, of the ones it can see, it thinks best of (stage 55a). It is
	// filled in as the crowd is walked and settled at the end: seeing them is
	// what clears a direction, and not seeing them is what writes one.
	dearest, dearX, dearY, dearest0 := 0, 0.0, 0.0, 0.0

	for _, i := range w.nearAgents {
		o := &w.agents[i]
		if !o.Alive || o.ID == a.ID {
			continue
		}
		if !w.canSee(a.X, a.Y, o.X, o.Y) {
			continue
		}
		d2 := dist2(a.X, a.Y, o.X, o.Y)

		// Whoever else is around is also a rival for every item in sight. This
		// rides on the scan above rather than asking the index per item: a
		// rival is only one if the observer can see it, so the set to search is
		// the one already in hand, and querying around each item instead would
		// turn up agents outside the observer's sight and change the answer.
		for j := range p.Foods {
			f := &p.Foods[j]
			if d := dist2(o.X, o.Y, f.X, f.Y); d < f.RivalDist*f.RivalDist {
				f.RivalDist = math.Sqrt(d)
				f.RivalID = o.ID
			}
		}

		// What this agent already knows about the other, if it has room to
		// know anything. Seeing somebody is how an acquaintance starts, but a
		// memory that is full of people who matter cannot take a stranger on:
		// it goes on judging them by the population prior, as it did the first
		// time. Meeting somebody it does know keeps that record fresh (#22).
		// One glance at the build, used for both the view and - if this one is
		// a stranger - for what is assumed about it.
		seen := w.glimpse(o, unit, w.cfg.AppearanceNoise)

		est, variance, risk, affinity := 0.0, w.cfg.PriorVariance, 0.0, 0.0
		op := a.opinion(o.ID)
		if op == nil {
			op = w.recordOpinionSeen(a, o.ID, seen)
		} else {
			w.touch(a, op)
		}
		if op != nil {
			est, variance = op.Strength, op.Variance
			risk, affinity = w.decayedRisk(a, op), w.decayedAffinity(a, op)
		} else {
			// No room to take this one on. It is judged by its build every
			// time it is seen, and never becomes anybody in particular.
			est = w.strangerFromLooks(a, seen)
		}

		// What it is holding out, if it is holding anything out (stage 49).
		// Priced in the looker's own terms, the way everything else in this
		// view is: the same figures that price a meal on the ground.
		offering, offerLeft := false, 0
		offerKind, offerValue, offerHeal := FoodKind(0), 0.0, 0.0
		if item := w.offering(o); item != nil && w.canEat(a, item) {
			offering, offerKind, offerLeft = true, item.Kind, w.offerLeft(o)
			offerValue = p.Self.Nutrition[item.Kind]
			offerHeal = w.itemHealKnown(a, item)
			w.sawOffer = true
		}

		if affinity > dearest0 {
			dearest, dearX, dearY, dearest0 = o.ID, o.X, o.Y, affinity
		}

		blur := w.noise(unit, w.cfg.JudgementNoise)
		p.Others = append(p.Others, AgentView{
			ID:          o.ID,
			X:           o.X,
			Y:           o.Y,
			Dist:        math.Sqrt(d2),
			Sex:         o.Sex,
			Species:     o.Species,
			Prey:        o.Species != a.Species && eatsMeat(a.Species),
			Meat:        w.meatFrom(o),
			Vitality:    o.Vitality,
			Appearance:  seen,
			Paired:      o.PartnerID != 0,
			Seeking:     o.State == StateSeekMate,
			Resting:     o.Action.Kind == ActRest,
			Rejected:    a.isRejected(o.ID),
			ThrowHit:    w.throwHitFor(a, o, math.Sqrt(d2)),
			CarryRoom:   o.canCarryMore(&w.cfg),
			AttackingMe: o.Action.Kind == ActAttack && o.Action.TargetID == a.ID,
			CourtingMe:  o.Action.Kind == ActCourt && o.Action.TargetID == a.ID,
			DeclaredFor: o.declaredFor(),
			Offering:    offering,
			Selling:     offering && offerKind < NumEdibleKinds,
			OfferKind:   offerKind,
			OfferLeft:   offerLeft,
			OfferValue:  offerValue,
			OfferHeal:   offerHeal,
			Uphill:      w.terrainAt(o.X, o.Y).Height > w.terrainAt(a.X, a.Y).Height,
			EstStrength: clamp(est+blur, MinAbility, MaxAbility),
			Uncertainty: variance,
			Risk:        risk,
			Affinity:    affinity,
			Fitness:     fitness(o, &w.cfg) + w.noise(unit, w.cfg.JudgementNoise*0.5),
		})
	}
	// Whether the one it thinks best of is still there (stage 55a). Passing
	// the last place it was seen is what turns into a direction; nothing
	// keeps the place itself.
	w.noteDearest(a, dearest, dearX, dearY)
	p.Self.Missing, p.Self.MissingDX, p.Self.MissingDY = w.missing(a)

	// Whoever else can get to those wares first (stage 49). An advertised
	// item is a contested item - it goes to one pair of hands and everybody
	// who heard the cry is walking - so it is priced with the race the world
	// already runs for a meal on the ground. The pass costs nothing in the
	// ordinary world: sawOffer is false unless somebody in this look was
	// holding something up.
	if w.sawOffer {
		for i := range p.Others {
			o := &p.Others[i]
			if !o.Offering {
				continue
			}
			o.OfferRivalDist = math.Inf(1)
			for j := range p.Others {
				if i == j {
					continue
				}
				r := &p.Others[j]
				if d := dist2(o.X, o.Y, r.X, r.Y); d < o.OfferRivalDist*o.OfferRivalDist {
					o.OfferRivalDist = math.Sqrt(d)
				}
			}
		}
	}

	// Counting the target for carrying (stage 40, #67): when this body last
	// had a meal in sight, and how much of that time it was in no hurry to
	// eat it. Neither figure is read by any rule.
	// What it is already holding is food too (stage 40) - a meal at no
	// distance with nobody racing for it. It goes in after everything that
	// counts what is on the ground: an item in the hand says nothing about
	// how rich this patch of country is, and nothing about how contested it
	// is either.
	seen := len(p.Foods)

	if seen > 0 {
		a.sawFoodTick = w.tick
		w.sightTicks++
		if a.Hunger <= w.cfg.SatiatedHunger {
			w.spareTicks++
		}
	}

	if seen > 0 {
		p.Self.FoodScarcity = float64(len(p.Others)) / float64(seen)
	} else if len(p.Others) > 0 {
		p.Self.FoodScarcity = float64(len(p.Others))
	}

	// What this look says about the ground the agent is standing on (stage
	// 15b). It is not a new sense: the food it has just counted is the whole
	// of the reading.
	w.noteRegion(a, seen)

	// And the caches this one knows and can see (stage 50). The scan is over
	// the world's stores rather than over the index because there are at most
	// MaxStores of them - the same order as the regions - and a world with
	// none does not look at all.
	if len(w.stores) > 0 && w.cfg.StoreCapacity > 0 {
		for i := range w.stores {
			st := &w.stores[i]
			if !w.canSee(a.X, a.Y, st.X, st.Y) {
				continue
			}
			// Noticing one it did not know (StoreFindChance). It is the only
			// path to a cache that does not need somebody who already knows,
			// and without it the other three cannot start. What it finds this
			// look it can use from the next one: the food in there was passed
			// over above, which is the ordinary way round for anything a body
			// learns while it is looking.
			if !w.knowsStore(a, i) {
				w.noticeStore(a, i)
				continue
			}
			p.Stores = append(p.Stores, StoreSight{
				Index: i, X: st.X, Y: st.Y,
				Dist: math.Sqrt(dist2(a.X, a.Y, st.X, st.Y)),
				Room: w.storeRoom(i),
			})
		}
	}

	p.Foods = w.carriedViews(a, p.Foods)
	if i, gain, ok := w.bestKnownRegion(a); ok {
		minX, minY, maxX, maxY := w.regionBounds(i)
		p.Self.BetterGroundX = (minX + maxX) / 2
		p.Self.BetterGroundY = (minY + maxY) / 2
		p.Self.BetterGround = gain
	} else {
		p.Self.BetterGround = 0
	}

	return p
}

// judgementError draws the error an observer makes when sizing up somebody
// else. Reading the world correctly is an ability of its own: the higher the
// rationality, the smaller the error.
func (w *World) judgementError(observer *Agent, scale float64) float64 {
	return w.noise(w.judgementScale(observer), scale)
}

// judgementScale is how badly this observer reads anything at all: one, less
// its rationality. It is split out from judgementError because an agent
// sizing up a crowd makes several readings of each of them, and the answer is
// the same for all of them.
func (w *World) judgementScale(observer *Agent) float64 {
	return (MaxAbility - observer.Rationality(&w.cfg)) / MaxAbility
}

// noise draws one misreading of the given size. It draws nothing at all when
// there is no error to make, which is what keeps a run with the noise turned
// off consuming the random source the same way it always did.
func (w *World) noise(unit, scale float64) float64 {
	if std := unit * scale; std > 0 {
		return w.rng.NormFloat64() * std
	}
	return 0
}

// drownFelt is what an agent makes of how dangerous its footing is: the truth,
// or nothing at all in the arm that takes the feeling away and leaves the
// water exactly as deadly (stage 34).
// It is this body's own figure: a body that knows the water is in less danger
// in it, and knows that about itself the way it knows its own legs (stage 38b,
// and the same call Self.Ground makes).
func (w *World) drownFelt(a *Agent, ground terrain) float64 {
	if !w.cfg.DrownKnown {
		return 0
	}
	return w.drownChanceFor(a, ground)
}
