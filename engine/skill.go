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

	// SkillRough is knowing how to cross broken country: the first one. What
	// it does is take the sting out of ground that costs more to cross
	// (terrain.go).
	SkillRough

	// SkillForage is knowing how to get the most out of what grows: the
	// second. What it does is soften the discount on eating the same thing
	// over and over (diet.go, stage 16).
	//
	// Yield and not a race (#64). A skill that helped an agent reach food
	// first, or win it off somebody, would let whoever had it take the
	// plants; one that means the same mouthful goes further has no such
	// room - a good forager needs *less* of what there is, so the pressure on
	// the food supply goes down rather than up.
	SkillForage

	// SkillSwim is knowing how to be in water: the third. What it does is
	// lower the chance that a tick spent in the river is the last one (stage
	// 34, terrain.go).
	//
	// It acts on the hazard and not on the cost of crossing, because stage 34
	// found what actually decides who comes out: not how fast a body crosses
	// but how long it stays, and agents here do not cross rivers - they live
	// in them (14.5% of all ticks, an average stay of 127). A skill that made
	// the crossing cheaper would be a skill about a thing nobody does.
	SkillSwim

	// SkillPoison is knowing what not to eat, and how much of it a body can
	// take: the fourth. What it does is lower the dose a plant's poison
	// delivers, and - because a body knows its own stomach - lower what it
	// prices the warning at (stage 17b, diet and controller).
	//
	// It is the first skill whose subject is not written on the map. Rough
	// going is what the ground is made of, foraging what it provides,
	// swimming how much of it is water; poison is carried by the plants, and
	// the plants are everywhere. What a newborn reads is therefore the crop
	// standing in the region it was born in - the same shape as the other
	// three, pointed at the thing this skill is about.
	SkillPoison

	// SkillFishLand and SkillFishWater are the two ways of taking what the
	// water holds (stage 43). They are one resource and two trades: the first
	// reaches out from dry ground and never rolls the drowning dice, the
	// second wades in and gets more out of every fish. Nothing else about
	// either differs, which is the point - it is the first place in this
	// world where the same food can be had safely and slowly or riskily and
	// well.
	SkillFishLand
	SkillFishWater

	// SkillHarvest is knowing how to get the awkward crop of stage 44 out of
	// the ground: the plant side of what an economy would need somebody to be
	// able to do that somebody else cannot.
	SkillHarvest

	// SkillThrow is knowing how to put a stone where it was meant to go
	// (stage 47). It is the one part of throwing that nothing else in the
	// world already does: what the stone does when it lands is the attack
	// gene's, and what the target does about it is the target's, so all this
	// may touch is the accuracy distance takes away (#71).
	SkillThrow

	// SkillCook is knowing how to make food out of food (stage 52). What it
	// buys is what an ignorant cook wastes, and what caps it is intelligence.
	//
	// It is the one skill with nothing to read off the ground. The other
	// seven are seeded by where a body was born, and five measurements
	// running have said that a skill seeded that way does not move where
	// bodies live; this one is seeded flat, so every bit of spread in it is
	// the leaps and the copying.
	SkillCook

	// SkillWard is knowing how to handle one sort of beast (stage 62): how it
	// comes at you and what to do about it. What it buys is what its blows
	// take off you, and - through the same figure - what a body reckons
	// standing near one costs it.
	//
	// Which sort it is about is the map's to say: a kind's row names the
	// skill that wards it (EnemyKind.Ward), so a world can have one sort
	// worth knowing about, or two sorts warded by the same lore, and a second
	// ward for a world that wants to tell them apart is one more entry here.
	//
	// The ceiling is defence, which is already "how much of a blow this body
	// keeps off itself". Power was refused for stage 47's reason: the gene
	// that decides what a blow does when it lands must not also decide
	// whether it lands.
	SkillWard

	NumSkillKinds
)

func (s SkillKind) String() string {
	switch s {
	case SkillRough:
		return "rough going"
	case SkillForage:
		return "foraging"
	case SkillSwim:
		return "swimming"
	case SkillPoison:
		return "poison"
	case SkillFishLand:
		return "fishing from the bank"
	case SkillFishWater:
		return "fishing in the water"
	case SkillHarvest:
		return "harvesting"
	case SkillThrow:
		return "throwing"
	case SkillCook:
		return "cooking"
	case SkillWard:
		return "handling beasts"
	}
	return "none"
}

// skillAptitude is how much of a nominal mastery this body can actually
// realise at one skill, from 0 to 1.
//
// Each skill names the gene that limits it (Config.SkillAptitude), and the
// rule for choosing is the same both times: the gene the doing already belongs
// to. Crossing ground is legs, so rough going is capped by speed. Getting more
// out of what has been found is a matter of what the body keeps of what it has
// learned about it, so foraging is capped by memory - which has never bought
// anything measurable in this world, and this is the first rule that asks it
// to.
//
// Pairing a skill instead with a gene chosen for the result it would produce -
// capping rough going with toughness to manufacture stage 20's missing niche -
// is choosing the answer before the measurement, so that lives in an arm.
//
// This is the cap that makes the whole thing behave. A nominal figure can be
// inherited or copied from anybody, but what it is worth is the holder's own
// body: a child of a great crosser of rough country is not one, unless its
// legs are. It is also why no rule is needed for a skill going down through
// the generations - the nominal number need never fall for the realised one to
// return to what the line's genes support - and why the agent that was taught
// can end up better at it than the one that taught it.
func (a *Agent) skillAptitude(cfg *Config, kind SkillKind) float64 {
	if int(kind) >= len(cfg.SkillAptitude) {
		return 0
	}
	return clamp(a.Gene(cfg.SkillAptitude[kind])/MaxAbility, 0, 1)
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
	return math.Min(best, a.skillAptitude(cfg, kind))
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
func (w *World) skillFromBirthplace(kind SkillKind, x, y float64) float64 {
	if w.cfg.SkillBirthplace <= 0 {
		return 0
	}
	i := w.regionIndexAt(x, y)
	if i < 0 || i >= len(w.regions) {
		return 0
	}
	share := 0.0
	switch kind {
	case SkillRough:
		// The share of the region that is dear to cross and is not water,
		// read off the same map the region's mean cost is read off
		// (region.go). Water is left out because being in water is a
		// different skill with a different gene behind it, and counting the
		// river twice would seed both from the same cells.
		//
		// A world with no map is nothing but level, so no such skill ever
		// appears in one.
		if w.ground == nil {
			return 0
		}
		share = w.regionMean(i, func(t terrain) float64 {
			if t.Cost > 1 && t.Kind != GroundWater {
				return 1
			}
			return 0
		})
	case SkillForage:
		// How little grows there. Necessity is what teaches this one: a body
		// born where the plants are thin is a body that learns to make a
		// mouthful go further, and one born in plenty never has to.
		//
		// It needs no terrain at all, which is the difference the second
		// skill makes: the first was a thing about the ground, and this is a
		// thing about what the ground provides, so a world with no map can
		// have it.
		share = clamp(1-w.regions[i].Food, 0, 1)
	case SkillSwim:
		// How much of the region is water. The same reading stage 36 uses to
		// decide where the bank is rich: a body born by the river is a body
		// that grew up in it.
		if w.ground == nil {
			return 0
		}
		share = w.regionWaterShare(i)
	case SkillFishLand:
		// The bank: dry ground with water next to it. A body born where the
		// land meets the water is a body that learned to fish without going
		// in - and a region that is all water, or all dry, teaches nobody
		// this one. It is a different reading from the water share, so the
		// two ways of fishing are not seeded from the same cells.
		if w.ground == nil {
			return 0
		}
		share = w.regionBankShare(i)
	case SkillFishWater:
		// How much of the region is water, the same reading swimming takes.
		// These two are the same fact about a childhood - it was spent by the
		// river - and what they buy from it is different: one is not drowning
		// and the other is getting more out of what is in there.
		if w.ground == nil {
			return 0
		}
		share = w.regionWaterShare(i)
	case SkillThrow:
		// How much there is to throw where this one was born. It is read off
		// the stones themselves rather than off the ground they lie on: the
		// broken country already seeds knowing how to cross it, and seeding
		// two skills from one reading of a place is what stage 38a set out
		// not to do.
		share = w.regionStoneShare(i)
	case SkillHarvest:
		// How much of what grows there needs knowing. A body born where the
		// awkward crop is the ordinary crop grows up knowing the trick, and
		// one born where it does not grow never sees the thing.
		share = w.specialShareAt(i)
	case SkillPoison:
		// What the crop standing in that region is carrying. A world whose
		// plants have no defences teaches nobody anything about them, the
		// same way a world with no map teaches nobody about broken country.
		if !w.cfg.PlantDefence {
			return 0
		}
		share = w.regionPoison(i)
	case SkillWard:
		// How much of the warded sort turns up where this one was born
		// (stage 62). It is the map's own weighting (stage 58) times the
		// share of arrivals that are worth knowing about (stage 59), so a
		// childhood spent in the country the beasts come from teaches this
		// and one spent anywhere else does not.
		share = w.regionWardShare(i)
	case SkillCook:
		// Nothing about the place at all: one for everybody, so the figure a
		// body starts with is the same wherever it was born, and the whole of
		// what separates one cook from another afterwards is the rare leap
		// and who it has stood near. It is the deliberate exception to the
		// other seven, and the reason is that seeding this one off the ground
		// would be a sixth go at a question five measurements have already
		// answered (see SkillCookRelief).
		share = 1
	}
	return clamp(share*w.cfg.SkillBirthplace, 0, 1)
}

// poisonResist is how much of a dose this body escapes, from 0 to 1. It is the
// one figure the two halves of the skill share: what the stomach turns aside,
// and what the body therefore knocks off the warning when it decides.
func (w *World) poisonResist(a *Agent) float64 {
	if w.cfg.SkillPoisonRelief <= 0 {
		return 0
	}
	return clamp(a.skillAt(&w.cfg, SkillPoison)*w.cfg.SkillPoisonRelief, 0, 1)
}

// learnFromBirthplace hands a new body whatever the country it arrived in has
// to teach, one skill at a time and each through the same comparison.
func (w *World) learnFromBirthplace(a *Agent) {
	if w.cfg.SkillBirthplace <= 0 {
		return
	}
	for kind := SkillKind(1); kind < NumSkillKinds; kind++ {
		if w.learnSkill(a, kind, w.skillFromBirthplace(kind, a.X, a.Y)) {
			w.skillsBorn++
		}
	}
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
	// Whichever of the ones it holds: with two skills in the world, always
	// leaping the first would be a rule about the order of the slots.
	held := 0
	for i := range a.hints {
		if a.hints[i].Skill != SkillNone {
			held++
		}
	}
	if held == 0 {
		return
	}
	pick := w.rng.Intn(held)
	for i := range a.hints {
		h := &a.hints[i]
		if h.Skill == SkillNone {
			continue
		}
		if pick == 0 {
			if w.learnSkill(a, h.Skill, h.Mastery+w.cfg.SkillGeniusJump) {
				w.skillsLeapt++
			}
			return
		}
		pick--
	}
}

// regionPoison is how poisonous the plants standing in a region are, on
// average. Zero when nothing is growing there: an empty region has nothing to
// teach.
func (w *World) regionPoison(i int) float64 {
	var sum, n float64
	for k := range w.foods {
		f := &w.foods[k]
		if f.Kind != FoodPlant || w.regionIndexAt(f.X, f.Y) != i {
			continue
		}
		sum += f.Genes.Poison
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / n
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

// regionWardShare is how much of the warded sort of beast turns up in this
// block, relative to an even share of them (stage 62). Zero in a world where
// no kind is warded, which is why a map without beasts worth knowing about
// teaches nobody how to handle them.
func (w *World) regionWardShare(i int) float64 {
	kinds := w.enemyKinds()
	var warded, total float64
	for j := range kinds {
		share := max(kinds[j].Share, 0)
		total += share
		if kinds[j].Ward != SkillNone {
			warded += share
		}
	}
	if total <= 0 || warded <= 0 || i < 0 || i >= len(w.regions) {
		return 0
	}
	weight := 1.0
	if w.regions[i].Enemies > 0 {
		weight = w.regions[i].Enemies
	}
	return clamp(weight*warded/total, 0, 1)
}

// wardAgainst is how much of this attacker's blow the defender turns aside by
// knowing what it is dealing with (stage 62), from 0 to 1.
//
// The one figure both halves of the skill share, the way poisonResist is: what
// the body actually keeps off itself, and what it therefore knocks off what
// standing near one of these looks like it will cost.
func (w *World) wardAgainst(defender, attacker *Agent) float64 {
	if w.cfg.SkillWardRelief <= 0 || attacker == nil || defender == nil {
		return 0
	}
	if attacker.Species != SpeciesEnemy {
		return 0
	}
	ward := w.kindOf(attacker).Ward
	if ward == SkillNone {
		return 0
	}
	return clamp(defender.skillAt(&w.cfg, ward)*w.cfg.SkillWardRelief, 0, 1)
}
