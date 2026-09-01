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
}

// FoodView is one food item as an agent sees it.
type FoodView struct {
	ID   int
	X, Y float64
	Dist float64

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

	Paired      bool
	Seeking     bool // looks like it is after a mate
	AttackingMe bool
	// Rejected is set for a candidate this agent recently walked away from and
	// is not interested in comparing again just yet.
	Rejected bool

	EstStrength float64 // believed power
	Uncertainty float64 // variance of that belief
	Risk        float64 // vitality this one has already cost the observer

	// Fitness is how good a mate they look, ability and condition together,
	// already blurred by the observer's rationality.
	Fitness float64
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

// perceive fills the world's reusable perception buffer for one agent.
func (w *World) perceive(a *Agent) *Perception {
	p := &w.perception
	p.Tick = w.tick
	p.Cfg = &w.cfg
	p.Rand = w.rng
	p.Foods = p.Foods[:0]
	p.Others = p.Others[:0]

	p.Self = SelfView{
		ID:           a.ID,
		X:            a.X,
		Y:            a.Y,
		Sex:          a.Sex,
		Species:      a.Species,
		Vitality:     a.Vitality,
		Hunger:       a.Hunger,
		Attack:       a.Attack(),
		Rationality:  a.Rationality(),
		Intelligence: a.Intelligence(),
		MaxVitality:  a.MaxVitality(&w.cfg),
		MaxSpeed:     a.MaxSpeed(&w.cfg),
		HungerRate:   a.HungerRate(&w.cfg),
		Defence:      w.cfg.DefenceCap * a.Gene(GeneDefence) / MaxAbility,
		Evasion:      w.cfg.EvasionCap * a.Gene(GeneEvasion) / MaxAbility * clamp(a.MaxSpeed(&w.cfg)/w.cfg.MaxSpeed, 0, 2),
		CanReproduce: a.CanReproduce(&w.cfg),
		AttackerID:   a.attackerID,
	}

	r := w.cfg.PerceptionRadius
	r2 := r * r

	// The index narrows the world down to the cells the circle of sight
	// touches; the circle itself is still tested below, exactly as it was when
	// this walked every agent and every item. The candidates arrive in
	// ascending index order, which is the order those loops used, so the
	// perception buffers are filled identically and the random draws below
	// happen in the same sequence.
	g := w.spatialIndex()
	w.nearFoods = g.appendFoodsNear(w.nearFoods[:0], a.X, a.Y, r)
	w.nearAgents = g.appendAgentsNear(w.nearAgents[:0], a.X, a.Y, r)

	for _, i := range w.nearFoods {
		f := &w.foods[i]
		d2 := dist2(a.X, a.Y, f.X, f.Y)
		if d2 > r2 {
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
			RivalDist: math.Inf(1),
		})
	}

	for _, i := range w.nearAgents {
		o := &w.agents[i]
		if !o.Alive || o.ID == a.ID {
			continue
		}
		d2 := dist2(a.X, a.Y, o.X, o.Y)
		if d2 > r2 {
			continue
		}

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

		op := w.opinionOf(a, o.ID)
		blur := w.judgementError(a, w.cfg.JudgementNoise)
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
			Paired:      o.PartnerID != 0,
			Seeking:     o.State == StateSeekMate,
			Rejected:    a.isRejected(o.ID),
			AttackingMe: o.Action.Kind == ActAttack && o.Action.TargetID == a.ID,
			EstStrength: clamp(op.Strength+blur, MinAbility, MaxAbility),
			Uncertainty: op.Variance,
			Risk:        w.decayedRisk(op),
			Fitness:     fitness(o, &w.cfg) + w.judgementError(a, w.cfg.JudgementNoise*0.5),
		})
	}

	if len(p.Foods) > 0 {
		p.Self.FoodScarcity = float64(len(p.Others)) / float64(len(p.Foods))
	} else if len(p.Others) > 0 {
		p.Self.FoodScarcity = float64(len(p.Others))
	}

	return p
}

// judgementError draws the error an observer makes when sizing up somebody
// else. Reading the world correctly is an ability of its own: the higher the
// rationality, the smaller the error.
func (w *World) judgementError(observer *Agent, scale float64) float64 {
	std := (MaxAbility - observer.Rationality()) / MaxAbility * scale
	if std <= 0 {
		return 0
	}
	return w.rng.NormFloat64() * std
}
