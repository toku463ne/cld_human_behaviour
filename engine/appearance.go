package engine

// This file is what can be seen of somebody without fighting them, and what an
// agent makes of it.
//
// Before this, a stranger was worth PriorStrength (50) to everybody, for ever:
// the same guess about a small young one and about the largest thing in the
// world, and a guess nobody ever revised. That is the one place where the
// world handed every agent the same answer regardless of what it had lived
// through.
//
// Two rules replace it, and neither of them lives in the engine:
//
//   - What is visible is a correlate, not the truth. Appearance is built out
//     of the genes that show - how big a body is and how fast it moves - and
//     is read with an error that shrinks with the observer's rationality. The
//     true attack is never on the wire (AgentView carries Appearance, not it).
//   - What appearance means is learned, one agent at a time. Every reading an
//     agent takes of somebody's strength (in a fight, or watching one) is also
//     a data point of "a body that looked like this hit that hard", and the
//     line through those points is what it assumes about the next stranger.
//     The engine holds no mapping of its own; two agents that have seen
//     different worlds guess differently.
//
// Whether appearance carries any information at all is left to the world. The
// budget makes it a mixed signal: a big body means a big budget, which on
// average means more of everything (positive), but a large share spent on
// being big is a share not spent on hitting (negative). Which of the two wins
// is measured, not designed.

import "math"

// Appearance is what shows of an agent: how much body there is and how fast it
// moves. Both are expressed values, so a child looks like a child and an old
// one looks worn - what is seen is what is there today, not what was
// inherited.
//
// It deliberately leaves out attack itself. A visible correlate that contains
// the answer would make the whole thing a slower way of reading the truth.
func (a *Agent) Appearance(cfg *Config) float64 {
	// How much creature there is, rather than two particular genes.
	//
	// The original reading - the vitality and speed genes averaged - was
	// chosen to keep the answer out of the question: attack must not be
	// visible, or reading a build is not an estimate but a slow measurement.
	// But under a budget those two genes are attack's *competitors*: every
	// point spent on being big or quick is a point not spent on hitting. The
	// harder the budget binds, the more any such reading anti-predicts the
	// thing it is being read for, and the correlation measured at 0.22 when
	// this went in had fallen to 0.08 by the time the world had grown.
	//
	// Bulk is the total, and the total is a different kind of fact from the
	// split. A big creature has more of everything, which is honestly worth
	// knowing; how it spent that total - whether the bulk went into hitting
	// or into carrying - is still hidden, which is what keeps this an
	// estimate. Size is visible, allocation is not.
	if cfg.LooksShowBulk {
		// Scaled to the range an ability lives in, so that a line fitted to
		// it is fitted to numbers of the size everything else here uses.
		if cfg.GeneBudgetMean > 0 {
			return a.Bulk(cfg) / cfg.GeneBudgetMean * midAbility
		}
	}
	// One age factor for both genes rather than one each: this is read for
	// every agent in sight of every agent that thinks, and Ability would work
	// the same curve out twice.
	return (a.Gene(GeneVitality) + a.Gene(GeneSpeed)) / 2 * a.AgeFactor(cfg)
}

// LooksSignal is how much a body actually says about what it hits for: the
// correlation between what an observer can see of an agent (Appearance) and
// what it can do (Attack), across the living population.
//
// It is the ceiling on everything stage 10 does. An agent fitting a line to a
// signal this weak can never read more out of it than is in it, so this is the
// number to look at before asking why learning to read builds is not worth
// more. Read only, and it is not something any agent can see.
type LooksSignal struct {
	All     float64 // over everybody alive
	Within  float64 // within one kind, which is who an agent mostly meets
	Ceiling float64 // the error a perfect reader of the build would still make
}

// LooksSignal reports how much a build says about a blow. Read only.
func (w *World) LooksSignal() LooksSignal {
	var out LooksSignal
	corr := func(pick func(*Agent) bool) float64 {
		var n, sx, sy, sxx, syy, sxy float64
		for i := range w.agents {
			a := &w.agents[i]
			if !a.Alive || !pick(a) {
				continue
			}
			x, y := a.Appearance(&w.cfg), a.Attack(&w.cfg)
			n, sx, sy = n+1, sx+x, sy+y
			sxx, syy, sxy = sxx+x*x, syy+y*y, sxy+x*y
		}
		if n < 2 {
			return 0
		}
		den := math.Sqrt((n*sxx - sx*sx) * (n*syy - sy*sy))
		if den == 0 {
			return 0
		}
		return (n*sxy - sx*sy) / den
	}
	out.All = corr(func(*Agent) bool { return true })
	out.Within = corr(func(a *Agent) bool { return a.Species == SpeciesHuman })

	// What is left over even for a reader that knows the true line: the
	// spread of attack that the build does not account for.
	var n, sum, sumSq float64
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		v := a.Attack(&w.cfg)
		n, sum, sumSq = n+1, sum+v, sumSq+v*v
	}
	if n > 1 {
		if variance := sumSq/n - (sum/n)*(sum/n); variance > 0 {
			out.Ceiling = math.Sqrt(variance * (1 - out.All*out.All))
		}
	}
	return out
}

// looksModel is one agent's answer to "what does a body like that usually
// hit for". It is an ordinary least squares line, kept as running sums so that
// nothing has to be stored per observation.
//
// It is not part of the memory of individuals: forgetting somebody does not
// unlearn what their kind looked like, and a full memory does not stop an
// agent from forming an impression. The two answer different questions - who
// is this, and what is one of these worth - and only the first needs a record
// per person.
type looksModel struct {
	n, sx, sy, sxx, sxy float64
}

// observe folds in one pair: a body that looked like x turned out to hit for y.
func (m *looksModel) observe(x, y float64) {
	m.n++
	m.sx += x
	m.sy += y
	m.sxx += x * x
	m.sxy += x * y
}

// fit is the line, or (mean, 0) when there is nothing to fit a slope to.
//
// The slope is pulled back towards zero by shrink readings' worth of "size
// says nothing", the same shape of assumption the estimate of an individual
// already carries (it starts at PriorStrength with PriorVariance and is only
// moved by evidence). An agent that has watched a dozen fights has a slope
// made mostly of noise, and a line fitted to noise is worse than no line:
// measured, the unshrunk version guesses strangers worse than an agent that
// only learned the average (HISTORY.md 2026-09-03).
func (m *looksModel) fit(slope bool, shrink float64) (intercept, gradient float64) {
	if m.n <= 0 {
		return 0, 0
	}
	mean := m.sy / m.n
	if !slope || m.n < 2 {
		return mean, 0
	}
	den := m.n*m.sxx - m.sx*m.sx
	if math.Abs(den) < 1e-9 {
		return mean, 0
	}
	gradient = (m.n*m.sxy - m.sx*m.sy) / den
	if shrink > 0 {
		gradient *= m.n / (m.n + shrink)
	}
	// Written about the mean rather than as the raw intercept: shrinking the
	// slope must not move where the line sits, only how steep it is.
	return mean - gradient*(m.sx/m.n), gradient
}

// predict is what this agent expects of a body that looks like x.
func (m *looksModel) predict(x float64, slope bool, shrink float64) float64 {
	intercept, gradient := m.fit(slope, shrink)
	return clamp(intercept+gradient*x, MinAbility, MaxAbility)
}

// LooksSense is one agent's fitted line, for the viewer to show. Reading it
// changes nothing.
type LooksSense struct {
	Readings int     // how many strengths it has ever taken a reading of
	Guess    float64 // what it would assume about a body of average size
	Slope    float64 // how much more it expects per unit of size
	Trusted  bool    // whether it has enough readings to go by the line at all
}

// LooksSense returns what an agent has made of appearance so far.
func (w *World) LooksSense(id int) LooksSense {
	a := w.agentByID(id)
	if a == nil {
		return LooksSense{}
	}
	out := LooksSense{
		Readings: int(a.looks.n),
		Trusted:  w.looksTrusted(a),
	}
	if !out.Trusted {
		out.Guess = w.cfg.PriorStrength
		return out
	}
	intercept, gradient := a.looks.fit(w.cfg.LooksSlope, w.cfg.AppearanceSlopePrior)
	out.Guess = clamp(intercept+gradient*midAbility, MinAbility, MaxAbility)
	out.Slope = gradient
	return out
}

func (w *World) looksTrusted(a *Agent) bool {
	return w.cfg.LearnFromLooks && int(a.looks.n) >= w.cfg.AppearanceMinReads
}

// seenAppearance is one glance at somebody's build. Every glance is its own
// reading, in the same way that the estimate of a known agent is re-blurred
// every time it is looked at: an agent does not get one canonical measurement
// of a body it can then reuse.
func (w *World) seenAppearance(observer, target *Agent) float64 {
	return w.glimpse(target, w.judgementScale(observer), w.cfg.AppearanceNoise)
}

// glimpse is the same reading for a caller that already has the observer's
// error unit in hand, which perceive does: it sizes up everybody in sight at
// once and would otherwise work out the same rationality over and over.
func (w *World) glimpse(target *Agent, unit, scale float64) float64 {
	return clamp(target.Appearance(&w.cfg)+w.noise(unit, scale), MinAbility, MaxAbility)
}

// learnFromLooks records that a body of this size hit for that much. It is
// called for every reading, including the ones the observer has no room to
// keep a record of: an agent that cannot remember who it just watched can
// still come away with an impression of what that sort of creature is like.
func (w *World) learnFromLooks(observer, target *Agent, reading float64) {
	if !w.cfg.LearnFromLooks {
		return
	}
	observer.looks.observe(w.seenAppearance(observer, target), reading)
}

// strangerStrength is what an observer assumes about somebody it has never
// taken a reading of. Without the learning it is the flat prior the world used
// to hand out; with it, it is that agent's own line, read off the stranger's
// build.
func (w *World) strangerStrength(observer *Agent, otherID int) float64 {
	if !w.looksTrusted(observer) {
		return w.cfg.PriorStrength
	}
	target := w.agentByID(otherID)
	if target == nil {
		return w.cfg.PriorStrength
	}
	return w.strangerFromLooks(observer, w.seenAppearance(observer, target))
}

// strangerFromLooks is the same answer for a caller that has already looked at
// the stranger this tick. perceive has: it draws one glance per agent in sight
// for the view it hands the controller, and taking a second one here would be
// both a wasted draw and a second, different reading of the same body in the
// same moment.
func (w *World) strangerFromLooks(observer *Agent, seen float64) float64 {
	if !w.looksTrusted(observer) {
		return w.cfg.PriorStrength
	}
	return observer.looks.predict(seen, w.cfg.LooksSlope, w.cfg.AppearanceSlopePrior)
}

// flatStrangerStrength is what the same agent would have said with the slope
// taken away: the level it has learned, and nothing about this particular
// build. It draws no glance, so calling it changes nothing about the run - it
// exists only so that the two estimators can be scored on the same encounters
// instead of in two different worlds, which is the only way to tell whether
// reading a build is worth anything (arms that differ at all meet different
// creatures, and that difference is larger than the one being measured).
func (w *World) flatStrangerStrength(observer *Agent) float64 {
	if !w.looksTrusted(observer) {
		return w.cfg.PriorStrength
	}
	return observer.looks.predict(0, false, 0)
}

// noteFirstSight measures how far out the assumption was. It is a read only
// measurement kept on the world - nothing in the simulation reads it back -
// and it is the completion condition of this stage: the guess an agent makes
// about somebody it has never met should be closer to the truth than the flat
// prior was.
func (w *World) noteFirstSight(observer *Agent, guess float64, otherID int) {
	target := w.agentByID(otherID)
	if target == nil {
		return
	}
	truth := target.Attack(&w.cfg)
	err := math.Abs(guess - truth)
	w.firstSights++
	w.firstSightError += err

	// The same encounter scored by the two estimators this one is meant to
	// beat: the same agent with the slope taken away, and the flat prior the
	// world used before any of this. Both are counterfactual and change
	// nothing.
	w.firstSightErrorFlat += math.Abs(w.flatStrangerStrength(observer) - truth)
	w.firstSightErrorFixed += math.Abs(w.cfg.PriorStrength - truth)

	// Split by whether the observer had enough of its own experience to go by
	// its line. Both halves are in the same world meeting the same sorts of
	// creature, which is the only comparison here that is not confounded by
	// the two arms having lived different runs.
	if w.looksTrusted(observer) {
		w.firstSightsLearned++
		w.firstSightErrorLearned += err
	}
}
