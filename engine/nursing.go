package engine

// A mother with a child at her heel moves slower (stage 66).
//
// What it is for is time together. The handing-on in this world rides on
// watching (stage 12b), and watching needs the two of them in sight of each
// other; a body that keeps moving spends its rearing period trailing a child
// that is forever catching up. Slowing her down buys the pair the standing
// still that a trade needs.
//
// What it deliberately is not:
//
//   - It is not in Agent.Ability or MaxSpeed. Those are inheritance times age
//     times sex, a chain that knows nothing about the world it is in, and
//     stage 53 settled that the multiplication happens in one place and means
//     the same thing for a lifetime. "There is a child of mine nearby" is a
//     fact about this tick.
//   - It is not a rule that mothers and children should stay together. The
//     mother still scores every option she always did, and so does the child.
//     What changes is one number in her body, and whether anybody stays is
//     left where it has always been: the child already gets a positive term
//     for watching somebody it is fond of (LoreValue), and a mother who is
//     not going anywhere is cheaper to stay beside.
//
// The shape is stage 54's and stage 57's: a fact about right now, written on
// to the agent once a tick by the world and read back by a plain method that
// knows nothing but the config.

// nurse says, for each mother, whether a child of hers is within the rearing
// radius (stage 66).
//
// It runs at the top of the tick, before anybody has moved, so that a mother
// is slowed for the tick she is being followed through rather than for the one
// after it. It is a second reading of the distance keepToGuardian takes later
// in the same tick - the same formula, on the same pair - and it is separate
// because that one runs during the movement loop, by which time mothers
// earlier in the slice have already taken their step.
//
// A world that has not asked for the rule never enters the loop.
func (w *World) nurse() {
	if w.cfg.NursingSpeedShare >= 1 || w.cfg.NursingSpeedShare <= 0 {
		return
	}
	for i := range w.agents {
		w.agents[i].Nursing = false
	}
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive || !w.beingReared(a) {
			continue
		}
		guardian := w.agentByID(a.GuardianID)
		if guardian == nil || !guardian.Alive {
			continue
		}
		near := dist2(a.X, a.Y, guardian.X, guardian.Y) <= w.cfg.RearingRadius*w.cfg.RearingRadius
		if near || w.cfg.NursingAlways {
			guardian.Nursing = true
			w.nursingTicks++
		}
	}
}

// beingReared is stillReared's question without stillReared's answer: whether
// this one is still keeping to a parent, and nothing else.
//
// stillReared spends a tick of childhood as it answers, which is right for the
// one place that decides whether to walk the child back and wrong for anywhere
// else: asking twice in a tick would age the child twice.
func (w *World) beingReared(a *Agent) bool {
	if a.GuardianID == 0 {
		return false
	}
	if w.cfg.RearingUntilGrown {
		return a.Maturity < 1
	}
	return a.RearingTimer > 0
}

// speedNow is how fast this body can go today: what it is built for, less
// whatever it is carrying and whoever it is carrying it for (stage 66).
//
// It is a method on the agent rather than on the world so that the three
// places that read a speed - moving, dodging, and what a body knows about
// itself - can all reach it. They have to agree: a mother who is actually
// slower but dodges and is perceived as if she were not would be three
// different bodies.
func (a *Agent) speedNow(cfg *Config) float64 {
	speed := a.MaxSpeed(cfg)
	if a.Nursing && cfg.NursingSpeedShare > 0 && cfg.NursingSpeedShare < 1 {
		speed *= cfg.NursingSpeedShare
	}
	return speed
}

// Nursing is what the rule is doing (stage 66): how much of the time a mother
// is slowed, and what the pair got out of it.
type Nursing struct {
	// Ticks is how many mother-ticks were spent with a child in the radius,
	// over the run. Zero in a world without the rule, since nothing counts
	// what it does not do.
	Ticks float64

	// Near is the share of reared children that are inside the radius right
	// now - the figure the rule is trying to raise, and the one stage 53b
	// measured at 0.774 before any of this.
	Near float64

	// Trades is how many lore trades happened between a child and its
	// guardian, over the run, and Moved how much they moved. This is the
	// target: the whole point of the slowing is that these two go up.
	Trades float64
	Moved  float64
}

// Nursing reports it. Read only.
func (w *World) Nursing() Nursing {
	out := Nursing{
		Ticks:  float64(w.nursingTicks),
		Trades: float64(w.rearTrades),
		Moved:  w.rearMoved,
	}
	var reared, near float64
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive || !w.beingReared(a) {
			continue
		}
		guardian := w.agentByID(a.GuardianID)
		if guardian == nil || !guardian.Alive {
			continue
		}
		reared++
		if dist2(a.X, a.Y, guardian.X, guardian.Y) <= w.cfg.RearingRadius*w.cfg.RearingRadius {
			near++
		}
	}
	if reared > 0 {
		out.Near = near / reared
	}
	return out
}

// NursingNow says whether this body is being slowed by a child at its heel
// right now (stage 66), for the viewer.
func (w *World) NursingNow(id int) bool {
	a := w.agentByID(id)
	return a != nil && a.Nursing
}
