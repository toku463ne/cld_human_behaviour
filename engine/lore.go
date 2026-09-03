package engine

import "math"

// This file is what an agent assumes when it works out what an option is
// worth: the numbers that used to be the same for everybody, written into the
// config or into the controller as constants.
//
// There are two kinds, and they are kept apart on purpose.
//
//   - Facts about the world. "Somebody I hit hits back about this often",
//     "a courtship is accepted about this often". These have a right answer,
//     the world knows it, and an agent can find it out by living: they are
//     updated from what actually happened, exactly as the estimate of
//     somebody's strength is (memory.go).
//   - Preferences. How much a past mauling puts you off, how much a future
//     rival is worth removing, how frightening it is to be running on empty.
//     These have no right answer - they are what an agent is like - so they
//     are inherited, mutated, and left to selection.
//
// The point of the split is that it says where a value can come from. A fact
// converges on the truth and being wrong about it is simply a mistake; a
// preference cannot be wrong, only more or less successful. Stage 12b makes
// both of them tradeable, and the difference shows up there too: trading a
// fact is somebody telling you something true, trading a preference is
// somebody rubbing off on you.

// belief is one fact an agent has been finding out about, held as a running
// mean of what it has seen. The count is capped so that a lifetime of old
// evidence never makes an agent unable to notice that the world has changed -
// the same reason the memory of individuals fades.
type belief struct {
	mean float64
	n    float64
}

func (b *belief) observe(x, rate, cap float64) {
	if rate <= 0 {
		return
	}
	b.n = min(b.n+1, cap)
	b.mean += (x - b.mean) * rate / b.n
}

// lore is everything an agent assumes, as opposed to everything it knows about
// somebody in particular (that is opinions, in memory.go). It is small and
// copied by value.
type lore struct {
	retaliation belief // how often the one you hit hits back
	accept      belief // how often a courtship is accepted

	riskWeight        float64 // how much what somebody once cost you puts you off
	competitionWeight float64 // what removing a future rival for food is worth
	shockRisk         float64 // how dangerous being low on vitality feels
}

// newLore draws what a founder starts life assuming. The facts start at the
// world's own figures with a little confidence behind them, so that the first
// few observations move them but a single surprise does not; the preferences
// are drawn around the world's figures with a spread, because a population
// whose members all want the same things has nothing for selection to work on.
func (w *World) newLore() lore {
	cfg := &w.cfg
	return lore{
		retaliation:       belief{mean: cfg.Retaliation, n: cfg.LorePriorCount},
		accept:            belief{mean: cfg.AcceptChance, n: cfg.LorePriorCount},
		riskWeight:        w.spreadAround(cfg.RiskWeight, cfg.LoreInitSpread),
		competitionWeight: w.spreadAround(cfg.CompetitionWeight, cfg.LoreInitSpread),
		shockRisk:         w.spreadAround(cfg.ShockRisk, cfg.LoreInitSpread),
	}
}

// plainLore is what an agent assumes when nobody drew it one: the world's own
// figures, exactly. It draws nothing, so building an agent outside the
// simulation - which in practice means a test - does not move the random
// source along.
func (w *World) plainLore() lore {
	cfg := &w.cfg
	return lore{
		retaliation:       belief{mean: cfg.Retaliation, n: cfg.LorePriorCount},
		accept:            belief{mean: cfg.AcceptChance, n: cfg.LorePriorCount},
		riskWeight:        cfg.RiskWeight,
		competitionWeight: cfg.CompetitionWeight,
		shockRisk:         cfg.ShockRisk,
	}
}

// unset reports a lore nobody has filled in. A drawn or inherited one always
// has some weight of evidence behind its beliefs, so a zero count is the
// marker; the alternative would be a flag that has to be kept in step.
func (l lore) unset() bool { return l.retaliation.n == 0 && l.accept.n == 0 }

// spreadAround draws a value around a centre, proportionally: a preference of
// twice the size gets twice the spread, so one number does for all of them.
func (w *World) spreadAround(centre, spread float64) float64 {
	if spread <= 0 {
		return centre
	}
	return clampPreference(centre*(1+w.rng.NormFloat64()*spread), centre)
}

// clampPreference keeps a preference inside the range the world was tuned in.
// Without a ceiling the mutation is a random walk with nothing to stop it, and
// the two preferences that price violence (competition and risk) would wander
// until the population collapsed one way or the other.
func clampPreference(v, centre float64) float64 {
	return clamp(v, 0, centre*maxPreferenceFactor)
}

// How far from the world's own figure a preference is allowed to get. Four
// times is wide enough that a lineage can specialise and narrow enough that
// nothing runs away.
const maxPreferenceFactor = 4

// inheritLore is what a child starts life assuming.
//
// Preferences come from one parent or the other, whole, plus a mutation - the
// same particulate inheritance the genes use (stage 7b), for the same reason:
// averaging the two parents halves the spread every generation and there is
// soon nothing left to select on.
//
// Facts are the Lamarckian part, and are off by default. At LamarckRate 0 a
// child is born knowing nothing and has to find the world out for itself; at 1
// it starts where its parent had got to. Both have to be runnable or there is
// no telling whether what spreads through a population is selection or
// teaching.
func (w *World) inheritLore(pa, pb *Agent) lore {
	cfg := &w.cfg
	pick := func(a, b float64) float64 {
		if w.rng.Float64() < 0.5 {
			return a
		}
		return b
	}
	mutate := func(v, centre float64) float64 {
		if cfg.LoreMutationStd <= 0 {
			return clampPreference(v, centre)
		}
		return clampPreference(v*(1+w.rng.NormFloat64()*cfg.LoreMutationStd), centre)
	}

	out := lore{
		riskWeight:        mutate(pick(pa.lore.riskWeight, pb.lore.riskWeight), cfg.RiskWeight),
		competitionWeight: mutate(pick(pa.lore.competitionWeight, pb.lore.competitionWeight), cfg.CompetitionWeight),
		shockRisk:         mutate(pick(pa.lore.shockRisk, pb.lore.shockRisk), cfg.ShockRisk),
	}

	l := clamp(cfg.LamarckRate, 0, 1)
	learned := func(a, b belief, prior float64) belief {
		if l <= 0 {
			return belief{mean: prior, n: cfg.LorePriorCount}
		}
		from := pick(a.mean, b.mean)
		// The child inherits what its parent came to believe, not how sure it
		// was of it: a handed down assumption is a starting point, not a
		// lifetime of evidence, so it gives way to the child's own experience
		// as readily as the world's figure would.
		return belief{mean: (1-l)*prior + l*from, n: cfg.LorePriorCount}
	}
	out.retaliation = learned(pa.lore.retaliation, pb.lore.retaliation, cfg.Retaliation)
	out.accept = learned(pa.lore.accept, pb.lore.accept, cfg.AcceptChance)
	return out
}

// --- what happened, folded back in ------------------------------------------

// noteRetaliation records one answer to "does that sort of thing fight back".
// Who gets to see it, and once per what, is decided in World.noteEngagement:
// the question is about picking a fight, not about a tick of one.
func (w *World) noteRetaliation(observer *Agent, hitBack bool) {
	observer.lore.retaliation.observe(boolValue(hitBack), w.cfg.LearningRate, w.cfg.LoreMemory)
}

// noteCourtship records that a proposal was accepted or turned down. Only the
// one that made it learns from it: the other side's answer is about itself,
// not about how the world tends to answer.
func (w *World) noteCourtship(suitor *Agent, accepted bool) {
	suitor.lore.accept.observe(boolValue(accepted), w.cfg.LearningRate, w.cfg.LoreMemory)
	// The world's own count, for the measurement that says whether what the
	// agents come to believe is true (cmd/experiment).
	w.courtships++
	if accepted {
		w.courtshipsAccepted++
	}
}

func boolValue(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// --- reading it out ---------------------------------------------------------

// Assumptions is what one agent assumes, for the viewer. The two counts are
// how much evidence stands behind each belief, which is what says whether an
// agent has actually seen anything or is still repeating what it was born
// with.
type Assumptions struct {
	Retaliation     float64
	RetaliationSeen float64
	Accept          float64
	AcceptSeen      float64

	RiskWeight  float64
	Competition float64
	ShockRisk   float64
}

// Assumes reports what this agent brings to the utility formula. Read only.
func (a *Agent) Assumes() Assumptions {
	return Assumptions{
		Retaliation:     a.lore.retaliation.mean,
		RetaliationSeen: a.lore.retaliation.n,
		Accept:          a.lore.accept.mean,
		AcceptSeen:      a.lore.accept.n,
		RiskWeight:      a.lore.riskWeight,
		Competition:     a.lore.competitionWeight,
		ShockRisk:       a.lore.shockRisk,
	}
}

// LoreView is what a population assumes, averaged, for the viewer and the
// experiment runner. It is read only.
type LoreView struct {
	Retaliation float64 // mean of what agents believe about hitting back
	Accept      float64 // ... and about courtship being accepted
	RiskWeight  float64
	Competition float64
	ShockRisk   float64

	// The spread of the three preferences. A mean says which way a population
	// leans; only the spread says whether there is anything left to select.
	SdRiskWeight  float64
	SdCompetition float64
	SdShockRisk   float64

	// What the world actually does, over the whole run: how often somebody who
	// was hit hit back, and how often a courtship was accepted. These are what
	// the two beliefs above are trying to find out, and nothing reads them.
	TrueRetaliation float64
	TrueAccept      float64
}

// Lore reports what the living population assumes on average, and what the
// world has actually been doing. Read only measurement.
func (w *World) Lore() LoreView {
	var out LoreView
	if len(w.agents) == 0 {
		return out
	}
	n := 0.0
	for i := range w.agents {
		a := &w.agents[i]
		out.Retaliation += a.lore.retaliation.mean
		out.Accept += a.lore.accept.mean
		out.RiskWeight += a.lore.riskWeight
		out.Competition += a.lore.competitionWeight
		out.ShockRisk += a.lore.shockRisk
		n++
	}
	out.Retaliation /= n
	out.Accept /= n
	out.RiskWeight /= n
	out.Competition /= n
	out.ShockRisk /= n

	for i := range w.agents {
		a := &w.agents[i]
		out.SdRiskWeight += square(a.lore.riskWeight - out.RiskWeight)
		out.SdCompetition += square(a.lore.competitionWeight - out.Competition)
		out.SdShockRisk += square(a.lore.shockRisk - out.ShockRisk)
	}
	out.SdRiskWeight = math.Sqrt(out.SdRiskWeight / n)
	out.SdCompetition = math.Sqrt(out.SdCompetition / n)
	out.SdShockRisk = math.Sqrt(out.SdShockRisk / n)

	if w.blowsSeen > 0 {
		out.TrueRetaliation = float64(w.blowsAnswered) / float64(w.blowsSeen)
	}
	if w.courtships > 0 {
		out.TrueAccept = float64(w.courtshipsAccepted) / float64(w.courtships)
	}
	return out
}

func square(x float64) float64 { return x * x }
