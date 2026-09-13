package engine

import "math"

// Something somebody wrote down (stage 69).
//
// This is the third thing in this world that can be picked up and not eaten,
// and the first whose worth is not a claim on food. A stone is for throwing
// and a coin is for buying; a book is for knowing, and what it does is move
// what one body has learned into another without the two of them ever having
// to be in the same place at the same time.
//
// What it is for, and why it is shaped this way.
//
//   - The asymmetry is the whole point. Stage 68 measured the seller's side of
//     a market and found it short by construction: what is in a hand is worth
//     the same to whoever is holding it, so every sale is a loss to the seller
//     and only the weight saved makes it up. A book that has been read is the
//     first thing in this world that is genuinely worth nothing to its owner
//     and something to everybody else - it has already told them what it says,
//     and it can still tell somebody else.
//   - So it survives being read (BookSurvivesReading). A book that is used up
//     is a rival good and behaves like a meal, which is the thing this is
//     supposed to be unlike. The consumed version is kept as the control.
//   - What it says is worse than what its writer knows (BookFidelity). Being
//     told at second hand is weaker than seeing for yourself everywhere else
//     in this world (exchangeRegions, StoreCryStrength), and writing is the
//     second hand at its longest.
//   - Nothing new prices it. Reading is worth what being taught is worth,
//     which is LoreValue - the figure the world already uses for standing
//     with somebody worth listening to. Writing is worth what the hand-over
//     it makes possible is worth, which is the same figure stage 48 puts on a
//     gift and stage 49 on a cry, because handing it on is the only use a
//     written book has to the one who wrote it.
//
// What it deliberately is not: a new kind of memory. It writes into the same
// places every other way of learning writes into (learnSkill, learnStore), and
// no rule reads a "has read" tally.

// BookSubject is what books in this world are about.
//
// There are two because the target was counted before this was written and the
// obvious subject turned out to have nowhere to land. Counted on the played
// map: a body has a free hint slot 0.000 of the time - the slots are full,
// always, 1.98 of 1.98 - so a book about a skill can only ever beat a figure
// already held, and even a perfect world library would help 3.2% of bodies to
// the tune of 0.0054 of one skill. That is the same wall stage 31 hit, for the
// third time: the cap is the room, not the signal.
//
// What does have room is what a body knows about places, which is deliberately
// kept out of the memory for people (#41) - and stage 50 has already measured
// what that room is worth: a world where everybody knows the caches beats one
// where they have to find them by +8.40 population.
type BookSubject uint8

const (
	// BookSkills is a book about how to do something: one skill and how well.
	BookSkills BookSubject = iota

	// BookPlaces is a book about where things are: the caches its writer
	// knows. It is the subject with somewhere to land.
	BookPlaces
)

func (s BookSubject) String() string {
	if s == BookPlaces {
		return "places"
	}
	return "skills"
}

// --- writing ----------------------------------------------------------------

// worthWriting is what this body could set down, and how good the record would
// be. It returns the subject matter and the strength of it, or a zero strength
// when there is nothing to write.
//
// The strength is the writer's own figure, through two discounts: what its
// hand can manage (the scribe's skill) and what writing costs in itself
// (BookFidelity). Both are multiplications on one number - there is no second
// formula for how good a book is.
func (w *World) worthWriting(a *Agent) (SkillKind, float64) {
	cfg := &w.cfg
	if !cfg.Books || cfg.WriteTicks <= 0 {
		return SkillNone, 0
	}
	hand := a.skillAt(cfg, SkillScribe)
	if hand <= 0 {
		return SkillNone, 0
	}
	if cfg.BookSubject == BookPlaces {
		// What it knows of the caches, as a share of the caches there are.
		// A body that knows none has nothing to write down.
		if len(w.stores) == 0 {
			return SkillNone, 0
		}
		known := 0
		for i := range w.stores {
			if w.knowsStore(a, i) {
				known++
			}
		}
		if known == 0 {
			return SkillNone, 0
		}
		return SkillNone, hand * cfg.BookFidelity * float64(known) / float64(len(w.stores))
	}
	// Otherwise the best thing it knows how to do, other than writing: a
	// book about writing books is allowed, but it is not what this is for,
	// and leaving it in would let a line of scribes write nothing else.
	best, bestKind := 0.0, SkillNone
	for k := SkillKind(1); k < NumSkillKinds; k++ {
		if k == SkillScribe {
			continue
		}
		if v := a.nominalSkill(k); v > best {
			best, bestKind = v, k
		}
	}
	if best <= 0 {
		return SkillNone, 0
	}
	return bestKind, best * hand * cfg.BookFidelity
}

// write puts a book in this body's hands. It reports whether anything was
// written: an empty hand is needed, and something to say.
func (w *World) write(a *Agent) bool {
	if !a.canCarryMore(&w.cfg) {
		return false
	}
	kind, strength := w.worthWriting(a)
	if strength <= 0 {
		return false
	}
	book := Food{
		ID:      w.nextFoodID,
		X:       a.X,
		Y:       a.Y,
		Kind:    FoodBook,
		Says:    kind,
		Written: clamp(strength, 0, 1),
	}
	// The places it names are the ones its writer knew when it was written.
	// They do not change afterwards: what is written is written, and a book
	// about a world that has moved on is a book that is wrong.
	if w.cfg.BookSubject == BookPlaces {
		for i := range w.stores {
			if w.knowsStore(a, i) {
				book.Places = append(book.Places, i)
			}
		}
	}
	w.nextFoodID++
	a.carried = append(a.carried, book)
	w.heldKind[FoodBook]++
	w.booksWritten++
	return true
}

// --- reading ----------------------------------------------------------------

// bookValue is what reading this one would be worth to this body, in the units
// everything else is scored in. It is the share of what the book says that is
// new to the reader, times what being taught is worth.
//
// A body can work this out without reading first because what it is weighing
// is its own ignorance: how much it already knows, and what its own body could
// support. Nothing about the book is hidden - it is a thing in its hands.
func (w *World) bookValue(a *Agent, f *Food) float64 {
	cfg := &w.cfg
	if !cfg.Books || f.Kind != FoodBook || cfg.BookValue <= 0 {
		return 0
	}
	gain := 0.0
	if len(f.Places) > 0 {
		// Places: how many of the ones it names this body does not know.
		fresh := 0
		for _, i := range f.Places {
			if !w.knowsStore(a, i) {
				fresh++
			}
		}
		if len(w.stores) > 0 {
			gain = f.Written * float64(fresh) / float64(len(w.stores))
		}
	} else if f.Says != SkillNone {
		// A skill: what it would actually add, which is capped by what this
		// body can support - the same ceiling learning it any other way is
		// capped by, so a book cannot hand anybody a skill their body could
		// not have had.
		ceiling := a.skillAptitude(cfg, f.Says)
		held := math.Min(a.nominalSkill(f.Says), ceiling)
		if a.nominalSkill(f.Says) <= 0 && len(a.hints) >= a.hintSlots {
			return 0 // nowhere to put it, which is the usual case
		}
		gain = math.Max(0, math.Min(f.Written, ceiling)-held)
	}
	if gain <= 0 {
		return 0
	}
	return cfg.BookValue * cfg.LoreValue * gain
}

// read applies what a book says to whoever is holding it. It reports whether
// anything was learned.
//
// The book is not used up: it still says what it says, and the reader can hand
// it on. That is the asymmetry this stage is for, and BookSurvivesReading is
// the control that takes it away.
func (w *World) read(a *Agent, idx int) bool {
	if idx < 0 || idx >= len(a.carried) {
		return false
	}
	f := a.carried[idx]
	if f.Kind != FoodBook {
		return false
	}
	learned := false
	for _, i := range f.Places {
		if !w.knowsStore(a, i) {
			learned = true
		}
		w.learnStore(a, i, w.cfg.StoreCryStrength*f.Written)
	}
	if f.Says != SkillNone && w.learnSkill(a, f.Says, f.Written) {
		learned = true
	}
	if learned {
		w.booksRead++
	}
	if !w.cfg.BookSurvivesReading {
		w.removeCarried(a, idx)
	}
	return learned
}

// runWrite and runRead are the world's side of the two words. Both run for
// their ticks and then do the thing, in the shape the cry and the cooking
// already use: no new parallel machinery for a multi-tick action (#76).
func (w *World) runWrite(a *Agent) {
	if _, strength := w.worthWriting(a); strength <= 0 || !a.canCarryMore(&w.cfg) {
		a.requestDecision(TriggerTargetLost) // hands filled, or nothing left to say
		return
	}
	if a.actionTicks < w.cfg.WriteTicks {
		return
	}
	w.write(a)
	a.requestDecision(TriggerGoalReached)
}

func (w *World) runRead(a *Agent) {
	i := a.heldBook()
	if i < 0 {
		a.requestDecision(TriggerTargetLost) // given away or sold while reading
		return
	}
	if a.actionTicks < w.cfg.WriteTicks {
		return
	}
	w.read(a, i)
	a.requestDecision(TriggerGoalReached)
}

// canWrite says whether this body has something to set down and a hand free to
// set it down in.
func (w *World) canWrite(a *Agent) bool {
	if !a.canCarryMore(&w.cfg) {
		return false
	}
	_, strength := w.worthWriting(a)
	return strength > 0
}

// heldBookValue is what reading what this body is already carrying would be
// worth, and zero when it is carrying no book or one that would tell it
// nothing.
func (w *World) heldBookValue(a *Agent) float64 {
	i := a.heldBook()
	if i < 0 {
		return 0
	}
	return w.bookValue(a, &a.carried[i])
}

// heldBook is the first book in this body's hands, or -1.
func (a *Agent) heldBook() int {
	for i := range a.carried {
		if a.carried[i].Kind == FoodBook {
			return i
		}
	}
	return -1
}

// --- what came of it ---------------------------------------------------------

// BookUse is what the writing came to. Read only.
type BookUse struct {
	// Written and Read are how often each happened, Lying how many are on the
	// ground and Held how many are in hands.
	Written int
	Read    int
	Lying   int
	Held    int

	// Holders is the share of living bodies carrying one, which is the figure
	// stage 67 says to watch: a hand holding a book is a hand not holding
	// dinner.
	Holders float64

	// Fidelity is the mean strength of the books that exist, so that a null
	// result can be told apart from a world where nothing worth reading was
	// ever written.
	Fidelity float64
}

// Books reports what the writing came to.
func (w *World) Books() BookUse {
	out := BookUse{Written: w.booksWritten, Read: w.booksRead}
	sum, n := 0.0, 0
	for i := range w.foods {
		if w.foods[i].Kind == FoodBook {
			out.Lying++
			sum += w.foods[i].Written
			n++
		}
	}
	bodies := 0.0
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		bodies++
		for j := range a.carried {
			if a.carried[j].Kind == FoodBook {
				out.Held++
				sum += a.carried[j].Written
				n++
				out.Holders++
				break
			}
		}
	}
	if bodies > 0 {
		out.Holders /= bodies
	}
	if n > 0 {
		out.Fidelity = sum / float64(n)
	}
	return out
}
