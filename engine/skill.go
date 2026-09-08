package engine

import "math"

// Skills: something an agent knows how to do, held in the room it bought for
// what it has learned (stage 38a).
//
// Three things about the shape, and all three are deliberate (#63).
//
// It lives in a hint slot. Stage 12c already built room that costs budget, is
// inherited a slot at a time, mutates in two steps, and can be copied into by
// somebody standing next to you. A skill needs every one of those, so it goes
// in there rather than beside it. A slot holds either a rule of thumb or a
// skill, never both, and what it costs is the same either way.
//
// It is learned once, not practised. There is no counter that goes up with
// use: what an agent has is a figure it was given, inherited or copied, and
// what it gets out of that figure depends on the body holding it. Making it
// improve with use would put a second multiplication on the way from a gene to
// what a body can do, and there is exactly one of those (Agent.Ability).
//
// It is one continuous number per category, not a table of grades. Discrete
// steps on a continuous ability have been tried twice here - the strategy
// depth gate, and the proposal to make eyesight a gene - and both landed in
// the wrong place and stayed there. Novice, journeyman and master are a thing
// an interface may say about a number; they are not a thing the world holds.

// SkillKind says what a slot is a skill at. SkillNone is a slot holding an
// ordinary rule of thumb.
type SkillKind uint8

const (
	SkillNone SkillKind = iota

	// SkillRough is knowing how to cross broken country: the first and, for
	// now, the only one. What it does is take the sting out of ground that
	// costs more to cross (terrain.go).
	SkillRough

	NumSkillKinds
)

func (s SkillKind) String() string {
	if s == SkillRough {
		return "rough going"
	}
	return "none"
}

// skillAptitude is how much of a nominal mastery this body can actually
// realise, from 0 to 1.
//
// Which gene is Config.SkillAptitude, and it is speed by default because
// crossing ground is what speed is for. Pairing it instead with a gene that
// competes with speed would manufacture the niche stage 20 failed to find
// (fast and frail against slow and tough) by choosing the answer before the
// measurement - so the alternative is an arm rather than the default.
//
// This is the cap that makes the whole thing behave. A nominal figure can be
// inherited or copied from anybody, but what it is worth is the holder's own
// body: a child of a great crosser of rough country is not one, unless its
// legs are. It is also why no rule is needed for a skill going down through
// the generations - the nominal number need never fall for the realised one to
// return to what the line's genes support - and why the agent that was taught
// can end up better at it than the one that taught it.
func (a *Agent) skillAptitude(cfg *Config) float64 {
	return clamp(a.Gene(cfg.SkillAptitude)/MaxAbility, 0, 1)
}

// skillAt is what this agent can actually do at something: the figure it
// holds, or what its body can support, whichever is less.
//
// A ceiling and not a multiplier. What #63 asks for is that a figure beyond
// what the genes support buys nothing extra - not that every body gets a
// fraction of what it holds - and the difference matters: with a ceiling, a
// body whose legs are ordinary gets the whole of an ordinary skill, and only
// the part it could not have used is lost.
func (a *Agent) skillAt(cfg *Config, kind SkillKind) float64 {
	if kind == SkillNone {
		return 0
	}
	best := 0.0
	for i := range a.hints {
		if h := &a.hints[i]; h.Skill == kind && h.Mastery > best {
			best = h.Mastery
		}
	}
	if best <= 0 {
		return 0
	}
	return math.Min(best, a.skillAptitude(cfg))
}

// nominalSkill is the figure the agent holds, before its body has anything to
// say about it. It is what is inherited, copied and compared.
func (a *Agent) nominalSkill(kind SkillKind) float64 {
	best := 0.0
	for i := range a.hints {
		if h := &a.hints[i]; h.Skill == kind && h.Mastery > best {
			best = h.Mastery
		}
	}
	return best
}

// learnSkill is every way of coming by a skill, and there is only one rule:
// take it if it is better than what you have.
//
// Inheriting, watching somebody and being born somewhere all end here, and
// none of them has a rule of its own. Nothing decays and nothing is demoted:
// the realised value regresses on its own, through the aptitude above, so a
// line whose legs cannot support the figure loses the benefit without anybody
// having to take the figure away.
//
// Where it goes: over the held mastery of the same skill, or into an empty
// slot. An agent with no room and nothing worse to overwrite learns nothing,
// which is the same answer stage 12c gives about ideas and stage 9 about
// people: room is what learning costs.
func (w *World) learnSkill(a *Agent, kind SkillKind, mastery float64) bool {
	if kind == SkillNone || mastery <= 0 {
		return false
	}
	mastery = clamp(mastery, 0, 1)
	empty := -1
	for i := range a.hints {
		h := &a.hints[i]
		if h.Skill == kind {
			if mastery > h.Mastery {
				h.Mastery = mastery
				w.skillsLearned++
				return true
			}
			return false
		}
	}
	if len(a.hints) < a.hintSlots {
		empty = len(a.hints)
	}
	if empty < 0 {
		return false
	}
	a.hints = append(a.hints, Hint{Skill: kind, Mastery: mastery})
	w.skillsLearned++
	return true
}

// skillFromBirthplace is what a newborn knows because of where it was born:
// how much of the country its parents were standing in is hard going.
//
// It is not a fourth mechanism. The figure goes through learnSkill like every
// other, and it can be beaten by what the child inherited. What it says is
// that being born in the rough is itself a way of coming to know the rough,
// which is the only one of the three paths that does not need somebody else to
// have known it first - and so the only one that can start a skill from
// nothing in a line that never had it.
func (w *World) skillFromBirthplace(x, y float64) float64 {
	if w.cfg.SkillBirthplace <= 0 || w.ground == nil {
		return 0
	}
	i := w.regionIndexAt(x, y)
	if i < 0 {
		return 0
	}
	// The share of the region that is dear to cross, read off the same map
	// the region's mean cost is read off (region.go). One is a region that is
	// nothing but rough.
	rough := w.regionMean(i, func(t terrain) float64 {
		if t.Cost > 1 {
			return 1
		}
		return 0
	})
	return clamp(rough*w.cfg.SkillBirthplace, 0, 1)
}

// teachSkill is the copying half, and it is where the diffusion this stage is
// about comes from.
//
// Rules of thumb are copied into empty slots and never compared, because half
// of "go for the big ones when you are starving" is not a weaker version of
// it. A mastery is a number on a line, so it can be compared, and comparing is
// what makes this spread differently: an agent standing among people who know
// the ground better keeps being handed a better figure than the one it holds.
// Nobody practises, and yet what a neighbourhood can do rises - because the
// ceiling on what can be reached by watching is set by the best in sight.
func (w *World) teachSkill(a, o *Agent) {
	if !w.cfg.SkillsSpread {
		return
	}
	for i := range o.hints {
		h := &o.hints[i]
		if h.Skill == SkillNone {
			continue
		}
		if w.learnSkill(a, h.Skill, h.Mastery) {
			w.skillsCopied++
		}
	}
}

// leapSkill is the genius event applied to a skill: the child is far better at
// something than the line it came from.
//
// It can only raise a skill the child already holds. A leap invents a way of
// doing something better, not a thing nobody in the world has ever needed:
// where there is no rough country there is nothing to be a genius at crossing,
// and a world with no map draws no skills at all.
func (w *World) leapSkill(a *Agent) {
	if w.cfg.SkillGeniusJump <= 0 {
		return
	}
	for i := range a.hints {
		h := &a.hints[i]
		if h.Skill != SkillNone {
			if w.learnSkill(a, h.Skill, h.Mastery+w.cfg.SkillGeniusJump) {
				w.skillsLeapt++
			}
			return
		}
	}
}

// --- reading it out ---------------------------------------------------------

// SkillOf is what one agent can actually do at something, for a viewer. Read
// only, and zero for an agent that is gone.
func (w *World) SkillOf(id int, kind SkillKind) float64 {
	a := w.agentByID(id)
	if a == nil {
		return 0
	}
	return a.skillAt(&w.cfg, kind)
}

// SkillUse is what the population has made of what it knows. Read only.
type SkillUse struct {
	// Held is the share of agents carrying the skill at all, Nominal the mean
	// figure among those, and Realised what their bodies actually get out of
	// it. The gap between the last two is the aptitude cap doing its work.
	Held     float64
	Nominal  float64
	Realised float64

	// Slots is how much of the room bought for what an agent has learned is
	// spent on skills rather than on rules of thumb. A skill and an idea cost
	// the same, so this is the trade the population has chosen.
	Slots float64

	// Dear is the mean realised skill of the agents standing on ground that
	// costs more to cross, and Open the same for the rest. The gap between
	// them is what the stage is for: if knowing how to cross broken country
	// is worth anything, the ones out in it are the ones who know.
	Dear, Open float64
}

// skillByGround is the mean realised skill of those standing on dear ground
// and of those standing on the rest, counted over everybody rather than over
// holders: an agent that knows nothing is part of what the ground it stands on
// knows.
func (w *World) skillByGround(kind SkillKind) (dear, open float64) {
	var nDear, nOpen float64
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		v := a.skillAt(&w.cfg, kind)
		if w.terrainAt(a.X, a.Y).Cost > 1 {
			dear, nDear = dear+v, nDear+1
		} else {
			open, nOpen = open+v, nOpen+1
		}
	}
	if nDear > 0 {
		dear /= nDear
	}
	if nOpen > 0 {
		open /= nOpen
	}
	return dear, open
}

// Skills reports what the living population knows. O(population x slots).
func (w *World) Skills(kind SkillKind) SkillUse {
	var out SkillUse
	var agents, holders, slots, held float64
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		agents++
		slots += float64(a.hintSlots)
		has := false
		for j := range a.hints {
			if a.hints[j].Skill == SkillNone {
				continue
			}
			held++
			if a.hints[j].Skill == kind {
				has = true
			}
		}
		if !has {
			continue
		}
		holders++
		out.Nominal += a.nominalSkill(kind)
		out.Realised += a.skillAt(&w.cfg, kind)
	}
	out.Dear, out.Open = w.skillByGround(kind)
	if agents > 0 {
		out.Held = holders / agents
	}
	if holders > 0 {
		out.Nominal /= holders
		out.Realised /= holders
	}
	if slots > 0 {
		out.Slots = held / slots
	}
	return out
}
