package engine

// Learning how bodies die (TODO 15, #137).
//
// What it is for. Everything an agent assumes about the world it either
// inherited (a preference, lore.go), found out by doing it (a belief, the same
// file) or was born believing at random (a rule of thumb, hint.go). There was
// no way to learn anything from what happened to somebody else. The most
// informative event in this world - a body stops - taught the onlookers who
// the killer was (stage 31) and, if it was the river, that the place is
// dangerous (stage 35), and nothing about what the dead one had been doing.
//
// A lesson is that: the situation the dead one was in, the move it was making,
// and a mark against making that move in that situation. It is the hint shape
// (hint.go) with everything else about it different - where it comes from,
// where it is kept, and what it costs - and it obeys the same one rule that
// matters: it is added to a score and it decides nothing.
//
// Four decisions, all of them made before a line of it was written.
//
// The vocabulary is its own. A rule of thumb reads one of eight things off the
// situation, and none of the eight is "standing in water" or "being hit right
// now" - which are two of the four ways to die here. Adding them to that list
// would change what drawHint draws in every world, including the ones with no
// lessons in them, and the fingerprint of the whole project with it. So the
// lessons read their own six, and hint.go is untouched.
//
// The room is bought, not given. A lesson lives in a slot of its own, the
// slots are drawn and inherited exactly as the rules of thumb's are, and they
// come out of the same budget (#137, the user's "a new slot in the genome").
// Learning therefore cannot make a body better for free - which is the
// conservation law CLAUDE.md names, and the reason lessons are not simply
// appended to the hints.
//
// It takes two. A body that has seen one death of a kind has seen an accident;
// the promotion into a slot happens the second time. What that costs is one
// bit per (situation, move) pair, which is where seenDeaths comes from - no
// buffer of episodes, no list of what happened when. The dying body already
// carries the situation it died in, so nothing has to be recorded in advance.
//
// And it spreads the way everything else does, on the back of watching
// somebody (exchangeLore). Nothing new was built for that either.

// DeathFeature is what a lesson reads off a situation. Like a rule of thumb's
// feature it is scaled to about 0..1 and comes off things a body can know
// about itself; unlike one, it is chosen for being a way to die.
type DeathFeature uint8

// There are four of them, and they are the four ways to die here: the river,
// a fight, an empty stomach, and running out. That is not the list this
// started with. It started with six - a crowd and dear ground were on it, as
// the circumstances a death might be about rather than its cause - and the
// count before any of it was switched on threw them out: over 2342 deaths on
// two maps, "in a crowd" was what the death was about 5 times and "hard
// ground" none at all.
//
// The reason is arithmetic and it would have been worth seeing sooner. A body
// that has died is, by definition, out of vitality or out of air; the cause
// features are all the way up at the moment they are read, and a continuous
// circumstance below one can never beat them. A key made of "the most extreme
// thing about the situation" is therefore a key made of the cause of death,
// and pretending otherwise would have left two features that no lesson could
// ever be about.
//
// Their order is the precedence the world's own death buckets use (world.go:
// the river takes a body even if somebody was hitting it a moment before),
// and that is not a coincidence - ties are broken by it for the same reason
// the buckets are exclusive.
const (
	DeathWet    DeathFeature = iota // the ground underfoot can end it
	DeathStruck                     // somebody is hitting it
	DeathEmpty                      // how hungry it is
	DeathWorn                       // how far below full its vitality has fallen

	NumDeathFeatures
)

func (f DeathFeature) String() string {
	switch f {
	case DeathWet:
		return "in water"
	case DeathStruck:
		return "under attack"
	case DeathEmpty:
		return "starving"
	case DeathWorn:
		return "worn down"
	}
	return "?"
}

// Lesson is one thing learnt from a death: in so far as this is the case, that
// move is worth this much less.
//
// The weight is always negative and always the same size. What a body learns
// is which pair to be wary of, not how wary to be: a figure learnt from two
// events would be a figure with two observations behind it, and this world
// already has a shape for that (belief, lore.go) which needs no slot.
type Lesson struct {
	Feature DeathFeature
	Act     ActionKind
	Weight  float64
}

// deathFeatures is the situation as the lessons read it, filled in once per
// decision. Unlike the rules of thumb's half of it there is no per-candidate
// part: a lesson is about what this body is doing and where it is standing,
// not about who it is looking at.
type deathFeatures [NumDeathFeatures]float64

// readSelf fills it in from what the body can see of its own position. Every
// figure here is one the perception already carries, which is the check that
// a lesson cannot read anything its holder could not.
func (f *deathFeatures) readSelf(p *Perception) {
	cfg := p.Cfg
	s := &p.Self
	*f = deathFeatures{}
	f[DeathWet] = boolValue(s.Drown > 0)
	f[DeathStruck] = boolValue(s.AttackerID != 0)
	f[DeathEmpty] = clamp(s.Hunger/cfg.MaxHunger, 0, 1)
	f[DeathWorn] = clamp(1-s.Vitality/s.MaxVitality, 0, 1)
}

// score is what this agent's lessons take off an option of the given kind. It
// is the only place a lesson touches a decision, and all it does is add a
// (negative) number - the same one rule the rules of thumb obey.
func (f *deathFeatures) score(lessons []Lesson, kind ActionKind) float64 {
	total := 0.0
	for i := range lessons {
		if l := &lessons[i]; l.Act == kind {
			total += l.Weight * f[l.Feature]
		}
	}
	return total
}

// --- what a death looks like from the outside ------------------------------

// numLessonKeys is how many (situation, move) pairs there are. One bit each is
// the whole of what a body remembers about deaths it has seen, which is what
// makes "the second time" cost nothing to keep.
const numLessonKeys = int(NumDeathFeatures) * int(numActionKinds)

// deathMarks is that bitset.
type deathMarks [(numLessonKeys + 63) / 64]uint64

func (m *deathMarks) has(key int) bool { return m[key/64]&(1<<uint(key%64)) != 0 }
func (m *deathMarks) set(key int)      { m[key/64] |= 1 << uint(key%64) }

func lessonKey(f DeathFeature, act ActionKind) int {
	return int(f)*int(numActionKinds) + int(act)
}

// deathLesson is what one death says: the thing about the dead one's situation
// that stood out most, and what it was doing at the time.
//
// The argmax rather than all six, because a body that came away from one death
// with six rules would fill every slot it owns from a single event and the
// "twice" that makes a pattern out of an accident would never bind. Ties go to
// the lower feature, so nothing here draws a random number.
//
// Nothing about the death is recorded in advance and nothing is kept: the
// dying body already carries everything this reads.
func (w *World) deathLesson(victim *Agent) (DeathFeature, ActionKind) {
	var f deathFeatures
	f[DeathWet] = boolValue(victim.drowned)
	// Somebody was hitting it. The "ever" half of the test is not idle: a
	// body that has never been hit carries nought here, and nought is a
	// tick, so without it every death in the first two ticks of a world
	// would be read as a killing.
	f[DeathStruck] = boolValue(victim.lastAttackTick > 0 && victim.lastAttackTick >= w.tick-1)
	f[DeathEmpty] = clamp(victim.Hunger/w.cfg.MaxHunger, 0, 1)
	f[DeathWorn] = clamp(1-victim.Vitality/victim.MaxVitality(&w.cfg), 0, 1)

	best := DeathWet
	for i := DeathFeature(1); i < NumDeathFeatures; i++ {
		if f[i] > f[best] {
			best = i
		}
	}
	return best, victim.Action.Kind
}

// witnessDeath is what everybody who saw a body stop comes away with.
//
// It runs for every death, not only the violent ones: starving where you sat
// and being taken by the river are exactly the patterns this is for, and
// stage 31's walk only fires on a killing.
//
// One pass over the onlookers, and it draws no random numbers.
//
// The counting happens whether or not anybody has room for a lesson: how often
// a rule could fire is the ceiling on what it can explain (stage 24). That
// count is what made this rule worth writing - see LessonUse.
func (w *World) witnessDeath(victim *Agent) {
	feature, act := w.deathLesson(victim)
	// Which of the six a death turns out to be about, and which move it
	// catches somebody making. Read only, and taken before anything can hold
	// a lesson: a key space that collapses onto one pair is a rule that can
	// only ever learn one thing.
	w.deathFeature[feature]++
	w.deathAct[act]++
	key := lessonKey(feature, act)
	twice := w.cfg.LessonRipeTwice
	crowd := 0
	w.forEachWitness(victim.X, victim.Y, victim.ID, func(o *Agent) {
		crowd++
		seen := o.seenDeaths.has(key)
		o.seenDeaths.set(key)
		if twice && !seen {
			// One of a kind is an accident. The bit is set; the next one
			// makes it a pattern.
			return
		}
		w.lessonsRipe++
		w.learnLesson(o, feature, act)
	})
	if crowd == 0 {
		w.deathsUnseen++
		return
	}
	w.deathsWatched++
	w.deathsSeen += crowd
}

// learnLesson puts a ripened pattern into a slot, if this body bought one and
// has not already filled it with the same pattern.
//
// There is no eviction. A body that paid for two slots and filled them is
// finished learning, exactly as a body that paid for two rules of thumb is
// finished being taught - and for the same reason, which is that this world
// has never decided what makes one idea worth less than another. The slot is
// the scarce thing, and it was bought.
func (w *World) learnLesson(a *Agent, f DeathFeature, act ActionKind) {
	if a.lessonSlots <= 0 || len(a.lessons) >= a.lessonSlots {
		return
	}
	if a.holdsLessonLike(f, act) {
		return
	}
	a.lessons = append(a.lessons, Lesson{Feature: f, Act: act, Weight: -w.cfg.LessonWeight})
	w.lessonsTaken++
}

func (a *Agent) holdsLessonLike(f DeathFeature, act ActionKind) bool {
	for i := range a.lessons {
		if a.lessons[i].Feature == f && a.lessons[i].Act == act {
			return true
		}
	}
	return false
}

// --- where the room comes from ---------------------------------------------

// drawLessonSlots is how much room for lessons a founder is born with, on
// exactly the terms drawHintSlots offers: uniform over the range, so that the
// first generation contains bodies that bought none and bodies that bought the
// lot, and nothing at all is drawn in a world without the rule.
func (w *World) drawLessonSlots() int {
	if w.cfg.LessonSlots <= 0 {
		return 0
	}
	return w.rng.Intn(w.cfg.LessonSlots + 1)
}

// inheritLessonSlots is how much room a child is born with: one parent's, and
// now and then one more. The room is what costs, so this is what the budget is
// charged for.
func (w *World) inheritLessonSlots(pa, pb *Agent, genius bool) int {
	if w.cfg.LessonSlots <= 0 {
		return 0
	}
	slots := pa.lessonSlots
	if w.rng.Intn(2) == 1 {
		slots = pb.lessonSlots
	}
	if genius && slots < w.cfg.LessonSlots {
		slots++
	}
	return min(slots, w.cfg.LessonSlots)
}

// lessonCost is what that room takes out of the budget the genes are fitted
// to, beside what the rules of thumb already take.
func (w *World) lessonCost(slots int) float64 {
	return float64(slots) * w.cfg.LessonSlotCost
}

// Nothing is inherited but the room. A child is born having seen no deaths and
// holding no lessons, which is the whole difference between this and a rule of
// thumb: one is what the lineage was born believing, the other is what this
// body watched happen. Whether that is right is what LamarckRate asks about
// the beliefs in lore.go, and the answer there was "off by default".

// --- passing it on ----------------------------------------------------------

// exchangeLessons is a lesson copied from one body to another, on the back of
// watching somebody (exchangeLore) and on exactly the terms a rule of thumb is
// copied on: into an empty slot, never over the top of one, and never a
// pattern the receiver already has an opinion about.
//
// A lesson is not a number two bodies can meet in the middle - half of "do not
// rest in the water" is not a weaker version of it - so this is a copy, like
// hint.go's, and not the averaging the beliefs get.
func (w *World) exchangeLessons(a, o *Agent) int {
	if !w.cfg.LessonsSpread {
		return 0
	}
	copied := 0
	for _, l := range o.lessons {
		if len(a.lessons) >= a.lessonSlots {
			break
		}
		if a.holdsLessonLike(l.Feature, l.Act) {
			continue
		}
		a.lessons = append(a.lessons, l)
		w.lessonsCopied++
		copied++
	}
	return copied
}

// --- reading it out ---------------------------------------------------------

// Lessons returns what this agent has learnt from watching bodies stop, and
// how much room it paid for. Read only, for the viewer.
func (a *Agent) Lessons() ([]Lesson, int) {
	out := make([]Lesson, len(a.lessons))
	copy(out, a.lessons)
	return out, a.lessonSlots
}

// LessonUse is what a population is making of them, and - the figures that
// matter before any of it is switched on - how often the thing could happen at
// all.
type LessonUse struct {
	Slots float64 // room bought, per agent
	Held  float64 // lessons actually carried, per agent

	// Kinds is how many distinct (situation, move) pairs are alive in the
	// population. A population can carry plenty of lessons and have them all
	// be the same one.
	Kinds float64

	// Feature is how often each of the four turned out to be what a death
	// was about, and Act the move it caught the dead one making. Both are
	// counts over the run, and both are taken whether or not the rule is on:
	// if one feature takes nearly all of them, the rest are decoration -
	// which is exactly what the first count said about the two that are no
	// longer here.
	Feature [NumDeathFeatures]int
	Act     [numActionKinds]int

	// Watched is how many deaths had anybody there to see them, Unseen how
	// many had nobody, and Seen the (death x onlooker) pairs. Ripe is how
	// many of those pairs ripened into a pattern - the second of their kind
	// for that onlooker, unless LessonRipeTwice is off - and Taken how many
	// of those found a slot to go in.
	//
	// The first three are what said this rule was worth writing, and they
	// said the opposite of what the design expected. The plan read the two
	// narrow witness rules already in the world (a drowning seen, a killer of
	// another species seen: 4 to 13 firings a run against 167 to 279
	// killings) as meaning deaths mostly go unwatched. They do not. Watched
	// over Watched+Unseen is 0.99 to 1.00 and Seen per death is 11 to 18:
	// every body that stops has a crowd around it. What is rare is the
	// conditions those two rules ask for, not the watching.
	Watched, Unseen, Seen, Ripe, Taken, Copied int
}

// LessonUse reports it. Read only, and taken whether or not the rule is on.
func (w *World) LessonUse() LessonUse {
	out := LessonUse{
		Watched: w.deathsWatched, Unseen: w.deathsUnseen, Seen: w.deathsSeen,
		Ripe: w.lessonsRipe, Taken: w.lessonsTaken, Copied: w.lessonsCopied,
		Feature: w.deathFeature, Act: w.deathAct,
	}
	n := 0.0
	kinds := make(map[int]struct{})
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		n++
		out.Slots += float64(a.lessonSlots)
		out.Held += float64(len(a.lessons))
		for _, l := range a.lessons {
			kinds[lessonKey(l.Feature, l.Act)] = struct{}{}
		}
	}
	if n > 0 {
		out.Slots /= n
		out.Held /= n
	}
	out.Kinds = float64(len(kinds))
	return out
}
