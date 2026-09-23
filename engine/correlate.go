package engine

import "math"

// Counting what a body could learn from what happens to it (TODO 24, #148).
//
// Nothing here is a rule. It adds no word, no state a body can act on, and no
// draw on the generator: it counts what the existing rules are already doing,
// in the same standing as the two banks of stage 37 and the trade watch of
// stage 75. A world with Correlate off allocates nothing and behaves, bit for
// bit, as it always did.
//
// What it is for. The proposal (#148) is that a body should collect "doing
// this in this situation tends to be followed by that", price the outcome with
// the utility formula rather than a new one, compose two such links into one
// where they share a middle term, and pass them on. Before a line of that is
// written, three things have to be true of this world, and none of them is
// obvious:
//
//   - that such a correlation exists at all, and is not already written into
//     the formula - a relation the formula prices is one the learning would
//     count twice (see Unwritten below);
//   - that two links ever share a middle term, which is the whole of whether
//     composing them is worth building (Pairs below);
//   - that two bodies ever each prefer what the other is holding, which is the
//     whole of whether the barter this world skipped can be put back (Swap).
//
// Any of the three coming back empty stops the item, which is why this is a
// separate piece of work from the item itself.
//
// Where the cuts are, and why there is no threshold in them. An event here
// fires on any change of its kind and carries its size as a value, rather than
// firing when a change is "large". A constant saying what large means would
// decide, before any counting, which relations could be found - and the count
// is what the constant should have been read off in the first place. So the
// events are cut where the world itself cuts them: hunger only ever climbs on
// its own, so any fall in it is a meal; vitality falling faster than this
// body's own drain is a blow. What "large" turns out to mean is then a reading
// (Loud below), not an input.

// CorrEvent is one thing that can happen to a body. The list is short and it
// is cut along the two currencies the utility formula already has: the first
// six are read through the chance of dying, the last of them through what a
// child is worth, and the three in the middle are worth nothing at all on
// their own.
//
// That the means are worth nothing is the point rather than an oversight. A
// coin is not food and a body that picks one up is no better off for it; what
// makes a coin worth having is that it is followed by something that is worth
// having, which is a fact about two events and not about either. If this world
// has money in it anywhere, it is in the co-occurrence table and nowhere else.
type CorrEvent uint8

const (
	CorrFed    CorrEvent = iota // hunger fell: it ate
	CorrMended                  // vitality rose
	CorrHurt                    // vitality fell faster than its own drain
	CorrSafe                    // whoever was hitting it stopped
	CorrWet                     // the ground underfoot started to be able to end it
	CorrDied                    // it stopped
	CorrCoin                    // money came into its hands
	CorrThing                   // something that is not money came into its hands
	CorrGave                    // something left its hands
	CorrChild                   // a child of its own was born

	NumCorrEvents
)

func (e CorrEvent) String() string {
	switch e {
	case CorrFed:
		return "fed"
	case CorrMended:
		return "mended"
	case CorrHurt:
		return "hurt"
	case CorrSafe:
		return "safe"
	case CorrWet:
		return "wet"
	case CorrDied:
		return "died"
	case CorrCoin:
		return "got coin"
	case CorrThing:
		return "got thing"
	case CorrGave:
		return "gave"
	case CorrChild:
		return "child"
	}
	return "?"
}

// The key a decision is filed under: the same shape the rules of thumb and the
// lessons use, which is a situation and a move. The situation is the loudest
// of the eight things a rule of thumb can read (hint.go), taken as an argmax
// the way a lesson takes its own - and taken that way deliberately, because
// #137 found that an argmax over continuous features collapses onto whatever
// is pinned at one. Whether the same thing happens here is Features below.
const numCorrKeys = int(NumHintFeatures) * int(numActionKinds)

func corrKey(f HintFeature, act ActionKind) int {
	return int(f)*int(numActionKinds) + int(act)
}

// The three windows the time-keyed probe reads, in ticks. They are the world's
// own scales and not new numbers: a skirmish, something between, and the
// planning horizon. Counting at all three is how "the horizon is an event"
// stops being an argument and becomes a reading - if a fixed window says as
// much as the events do, the simpler rule wins.
var corrHorizons = [...]int{30, 200, 700}

const numCorrHorizons = len(corrHorizons)

// loudEnough is the yardstick for whether a relation is worth anything, and it
// is not a constant: it is the spread of the error this world's own bodies
// make when they score an option. A term smaller than the noise it competes
// with moves nobody, which stage 86 measured the hard way, so the count that
// matters is of the keys that clear it.
//
// It lives here rather than in Config because it is a scale on the measuring
// side, and a measurement taken against a different scale cannot be compared
// with this one (CLAUDE.md, the coding rules).
const corrQuietShare = 0.25 // of the mean evaluation noise, the floor for "says anything"

// corrWrittenShare is how much of an outcome the formula has to have predicted
// before the relation counts as one the designer already wrote down.
//
// What is compared is two expectations and not a value against an expectation.
// The realised figure has to be multiplied by how often the event follows the
// key, because the formula's own number is already a value times a chance: a
// courtship that works is worth a whole child, and reading that against what
// the formula expects of a courtship that might not work counts every goal in
// the world as unwritten. A key
// whose realised value is large while the formula expected nothing of the move
// is a relation the formula does not have; one where the two move together is
// a relation it does.
//
// What is compared is all of the option's goals rather than its life term
// alone (Utility.goalScore). Reading only the life term got this wrong for
// every goal that lives somewhere else: a courtship costs vitality and buys a
// child, so by the life term the formula "expected nothing" from the one move
// whose whole point it prices exactly.
const corrWrittenShare = 0.5

// corrCell is one (key, event) pair as the counting sees it.
type corrCell struct {
	n     int
	value float64 // what the event was worth, summed
	pred  float64 // what the formula expected the move to be worth, summed
	lag   float64 // ticks between the decision and the event, summed
}

// corrSpread is a running count, sum and sum of squares, for the windows.
type corrSpread struct {
	n          int
	sum, sqsum float64
}

func (s *corrSpread) add(x float64) {
	s.n++
	s.sum += x
	s.sqsum += x * x
}

func (s *corrSpread) mean() float64 {
	if s.n == 0 {
		return 0
	}
	return s.sum / float64(s.n)
}

func (s *corrSpread) variance() float64 {
	if s.n < 2 {
		return 0
	}
	m := s.mean()
	return math.Max(0, s.sqsum/float64(s.n)-m*m)
}

// corrTrace is one decision still eligible to be credited with what happens
// next: the key it was filed under, when it was taken, and what the formula
// said the move was worth to this body's chances.
//
// It is a trace and not a single entry because crediting only the last
// decision is what lesson.go does, and it can only ever learn "what it was
// doing at the time". The move that got a starving body to the meat was not
// the eating; it was whatever took it there.
type corrTrace struct {
	key  int
	at   int
	pred float64
}

// corrProbe is the time-keyed reading: one decision held open for exactly so
// many ticks, then asked what became of the body. One per window per body, so
// the memory is bounded however long the window is - a sample of decisions
// rather than all of them, which is all a count needs.
type corrProbe struct {
	key              int
	at               int
	open             bool
	hunger, vitality float64
}

// corrSnap is what a body looked like at the end of last tick, which is the
// whole of how an event is detected. Nothing is recorded in advance.
type corrSnap struct {
	hunger, vitality float64
	drown            float64
	kinds            [NumFoodKinds]uint8
	attacked         bool
	alive            bool
}

// corrHold is one thing that came into a hand and is still eligible to be
// credited with what happens next. It is the decision trace's shape, applied
// to things instead of moves.
//
// This is the back propagation of #148 (the user's, 2026-09-23). Composing two
// links drops the middle term - "offer, then get a coin, then eat" composes to
// "offer, then eat" and the coin is gone from it - so a body that learnt only
// composed links would have no reason to pick a coin up, keep it, or not hand
// it away. Crediting the thing rather than dropping it puts the reason back,
// and it is far cheaper than composing: eight numbers rather than a key space.
//
// Both signs, which is the user's own rule applied to their own idea. An
// outcome is worth what it is worth; crediting only the good ones would walk
// every kind of thing upwards, because everything is followed by something
// good eventually.
type corrHold struct {
	kind FoodKind
	at   int
}

// agentCorr is the per body half, allocated the first time a body is seen and
// only in a world with the instrument on.
type agentCorr struct {
	trace  []corrTrace
	probes [numCorrHorizons]corrProbe
	snap   corrSnap
	ready  bool

	// seen is one bit per (key, event) pair this body has already met, which
	// is how "the same key twice in one life" is counted without keeping any
	// history. It is lesson.go's trick, widened by the event.
	seen []uint64

	// holds is what has lately come into this body's hands, still eligible to
	// be credited with what happens next.
	holds []corrHold

	// gotAt is when the run of each kind now in this body's hands began, for
	// the figure that says how long a thing stays in a hand before it goes.
	gotAt [NumFoodKinds]int

	// last is the tick each kind of event last happened to this body. Ten
	// ints, so the co-occurrence table costs no history either.
	last [NumCorrEvents]int
}

func (c *agentCorr) hasSeen(key int) bool {
	i := key / 64
	return i < len(c.seen) && c.seen[i]&(1<<uint(key%64)) != 0
}

func (c *agentCorr) markSeen(key int) {
	i := key / 64
	for len(c.seen) <= i {
		c.seen = append(c.seen, 0)
	}
	c.seen[i] |= 1 << uint(key%64)
}

// correlateWatch is the world's half. It hangs off the world; no rule reads
// any of it.
type correlateWatch struct {
	on bool

	// What was decided, and what followed it.
	decisions int
	keyN      [numCorrKeys]int
	cells     []corrCell // numCorrKeys x NumCorrEvents, flattened
	features  [NumHintFeatures]int
	acts      [numActionKinds]int

	// How loud this world's bodies are when they misjudge an option, which is
	// what a relation has to beat to be worth anything.
	noise corrSpread

	// Events in their own right: how many, what each was worth, how many had
	// anybody there to see them, and over how many onlookers.
	events    [NumCorrEvents]int
	value     [NumCorrEvents]float64
	watched   [NumCorrEvents]int
	onlookers [NumCorrEvents]int
	repeats   int // (key, event) pairs a body had already met once

	// One event after another, which is the whole of whether two links can be
	// composed into one. pairs[b][c] is how often c followed b inside the
	// window, over the same body.
	pairs   [NumCorrEvents][NumCorrEvents]int
	pairLag [NumCorrEvents][NumCorrEvents]int

	// The time-keyed reading, for each window: what became of a body so many
	// ticks after a decision, both overall and per key. A key that says no
	// more than the overall figure says nothing.
	spanAll [numCorrHorizons]corrSpread
	spanKey [][]corrSpread // numCorrHorizons x numCorrKeys

	// What a thing in the hand turns out to be followed by: how many events
	// were credited to holding one of each kind and what they came to, how
	// many of each came into a hand at all, how many left one, and how long
	// they stayed. Money is the coin row of these.
	itemN    [NumFoodKinds]int
	itemSum  [NumFoodKinds]float64
	itemGot  [NumFoodKinds]int
	itemGone [NumFoodKinds]int
	itemHeld [NumFoodKinds]float64

	// Whether two bodies in sight of each other each hold something the other
	// would rather have (the double coincidence of wants), sampled rather
	// than swept: samples is how many times the question was asked of a body,
	// swaps how many times the answer was yes, and gain what both sides
	// together stood to make.
	samples, swaps int
	gain           float64
	trinketHands   int
	// The same question asked of food through the discount on sameness
	// (stage 16), which is the other place a swap could pay.
	foodSwaps int
	foodGain  float64

	// A seller with something spare, a buyer in sight, and a coin in that
	// buyer's hand: the state a sale needs, counted whether or not one
	// happened.
	sellerReady int

	// Gifts, and whether one is ever returned: open is who gave to whom and
	// when, back how many of those were answered, and backTicks how long it
	// took.
	open      map[[2]int]int
	gifts     int
	back      int
	backTicks int
}

// --- switching it on --------------------------------------------------------

// startCorrelate readies the watch. Called once, from NewWorld.
func (w *World) startCorrelate() {
	if !w.cfg.Correlate {
		return
	}
	w.corr.on = true
	w.corr.cells = make([]corrCell, numCorrKeys*int(NumCorrEvents))
	w.corr.spanKey = make([][]corrSpread, numCorrHorizons)
	for i := range w.corr.spanKey {
		w.corr.spanKey[i] = make([]corrSpread, numCorrKeys)
	}
	w.corr.open = make(map[[2]int]int)
}

func (w *World) corrOf(a *Agent) *agentCorr {
	if a.corr == nil {
		a.corr = &agentCorr{}
	}
	return a.corr
}

// --- what was decided -------------------------------------------------------

// noteDecisionKey files one decision. It runs inside decide, where the
// perception it reads has already been built, so nothing is computed twice and
// nothing new is looked at: the situation is read by the same hintFeatures the
// rules of thumb read, and the prediction is the life term of the option the
// body actually took, before what it costs.
func (w *World) noteDecisionKey(a *Agent, p *Perception, pred, noise float64) {
	if !w.corr.on {
		return
	}
	var f hintFeatures
	f.readSelf(p)
	// And the half of the situation that is about whoever the move is aimed
	// at, so that a key can be about a stranger at all. Nil for a move aimed
	// at nobody, which reads nought for those four exactly as a candidate
	// with no target does.
	f.readTarget(p, corrTargetOf(p, a.Action.TargetID))

	loud := HintFeature(0)
	for i := HintFeature(1); i < NumHintFeatures; i++ {
		if f[i] > f[loud] {
			loud = i
		}
	}
	key := corrKey(loud, a.Action.Kind)

	c := &w.corr
	c.decisions++
	c.keyN[key]++
	c.features[loud]++
	c.acts[a.Action.Kind]++
	c.noise.add(noise)

	ac := w.corrOf(a)
	ac.trace = append(ac.trace, corrTrace{key: key, at: w.tick, pred: pred})
	if n := w.cfg.CorrelateTrace; n > 0 && len(ac.trace) > n {
		// Copied to the front rather than resliced: resliding the head
		// forward every time would walk the backing array off the end of
		// itself, so a body that lived long enough would keep growing one.
		ac.trace = append(ac.trace[:0], ac.trace[len(ac.trace)-n:]...)
	}
	// And one of the time-keyed probes, if that window is free. Only when it
	// is free: a body that filled every window on every decision would
	// need a record per decision per window, and a sample answers the
	// question as well.
	for i := range ac.probes {
		if !ac.probes[i].open {
			ac.probes[i] = corrProbe{key: key, at: w.tick, open: true,
				hunger: a.Hunger, vitality: a.Vitality}
		}
	}
}

// corrTargetOf is the one in sight this move is aimed at, or nil for a move
// aimed at nobody - which reads nought for the four features that are about
// somebody else, exactly as a candidate with no target does.
func corrTargetOf(p *Perception, id int) *AgentView {
	if id == 0 {
		return nil
	}
	return viewOf(p, id)
}

// --- what followed it -------------------------------------------------------

// stepCorrelate is the one pass per tick that turns changed state into events
// and credits them back to the decisions that could have caused them.
//
// It runs after everything else in the tick, so that what it reads is settled:
// the blows have landed, the river has taken whoever it was going to take, and
// the newborn is in the world. The dead are still here, which is why it runs
// before they are compacted away - a body that stopped is the one event this
// world was already sure mattered.
func (w *World) stepCorrelate() {
	if !w.corr.on {
		return
	}
	for i := range w.agents {
		a := &w.agents[i]
		ac := w.corrOf(a)
		s := w.selfView(a)
		now := corrSnap{
			hunger: a.Hunger, vitality: a.Vitality, drown: s.Drown,
			attacked: a.attackerID != 0, alive: a.Alive,
		}
		for j := range a.carried {
			if k := a.carried[j].Kind; k < NumFoodKinds && now.kinds[k] < 255 {
				now.kinds[k]++
			}
		}
		if !ac.ready {
			ac.snap, ac.ready = now, true
			// A newborn's parents have gained something, and this is the one
			// tick it can be told from any other: nothing about a child is
			// kept on the body that had it.
			if a.Age == 0 {
				for _, id := range a.ParentIDs {
					if parent := w.agentByID(id); parent != nil {
						w.fireCorr(parent, CorrChild, w.cfg.OffspringValue)
					}
				}
			}
			continue
		}
		w.readEvents(a, ac, &s, &now)
		ac.snap = now
		if !a.Alive {
			// A body that has stopped is not a reading about what became of
			// it: the window would close on a corpse and file the whole of
			// LifeValue under whichever key happened to be in the probe.
			continue
		}
		w.closeProbes(ac, &s, &now)
	}
	w.sampleWants()
}

// readEvents is the diff: what changed about this body since last tick, and
// what each change was worth in the currency the formula already uses.
func (w *World) readEvents(a *Agent, ac *agentCorr, s *SelfView, now *corrSnap) {
	was := &ac.snap
	if was.alive && !now.alive {
		// The one event with no next state to price. What a life is worth is
		// what the formula pays for the whole of the odds, so that is what
		// stopping costs.
		w.fireCorr(a, CorrDied, -w.cfg.LifeValue)
		return
	}
	if !now.alive {
		return
	}
	// Hunger only ever climbs on its own, so any fall in it is a meal, and
	// there is no figure here saying how big a meal has to be.
	if now.hunger < was.hunger {
		w.fireCorr(a, CorrFed, w.corrLifeGap(s, was.vitality, was.hunger, now.vitality, now.hunger))
	}
	// Mending faster than this body mends anyway. Lying down and getting
	// better is the recovery path this world is built on (CLAUDE.md: leave a
	// way back, or everybody starves), so it is the weather rather than an
	// event - the first count had it firing half a million times against two
	// and a half thousand meals. What clears the body's own rate is the thing
	// the proposal is about: a carcass putting a starving body back on its
	// feet in one go.
	if now.vitality-was.vitality > w.restRate(a) {
		w.fireCorr(a, CorrMended, w.corrLifeGap(s, was.vitality, was.hunger, now.vitality, was.hunger))
	}
	// A blow, and not "vitality went down". Vitality goes down in every body
	// every tick, from a metabolism the formula prices exactly and from the
	// effort the body chose to spend, and counting either as something that
	// happened to it would fire on every tick of every life - which is what
	// the first count did, 92574 times against 785 meals. What is left when
	// those two are taken out is the one thing the formula refuses to carry
	// past the tick it is in: somebody hitting it.
	if a.lastAttackTick == w.tick {
		w.fireCorr(a, CorrHurt, w.corrLifeGap(s, was.vitality, was.hunger, now.vitality, was.hunger))
	}
	if was.attacked && !now.attacked {
		// Whatever was hitting it has stopped, which is the removal of a
		// threat - the thing the formula deliberately refuses to carry into
		// its second window, and therefore the thing it cannot predict.
		w.fireCorr(a, CorrSafe, w.corrIncoming(s))
	}
	if was.drown <= 0 && now.drown > 0 {
		w.fireCorr(a, CorrWet, -now.drown*w.cfg.LifeValue)
	}
	// What is in the hands, kind by kind: one event for something arriving and
	// one for something leaving, because the two are the two sides of every
	// exchange this world could have. Money is one of the kinds rather than a
	// case of its own - what makes a coin a coin here is what follows it, and
	// that is what the item table is for.
	for k := FoodKind(0); k < NumFoodKinds; k++ {
		switch {
		case now.kinds[k] > was.kinds[k]:
			if ac.gotAt[k] == 0 {
				ac.gotAt[k] = w.tick
			}
			w.corr.itemGot[k]++
			ac.holds = append(ac.holds, corrHold{kind: k, at: w.tick})
			if k == FoodCoin {
				w.fireCorr(a, CorrCoin, 0)
			} else {
				w.fireCorr(a, CorrThing, 0)
			}
		case now.kinds[k] < was.kinds[k]:
			if now.kinds[k] == 0 && ac.gotAt[k] != 0 {
				w.corr.itemHeld[k] += float64(w.tick - ac.gotAt[k])
				w.corr.itemGone[k]++
				ac.gotAt[k] = 0
			}
			w.fireCorr(a, CorrGave, 0)
		}
	}
	if n := w.cfg.CorrelateTrace * 2; n > 0 && len(ac.holds) > n {
		ac.holds = append(ac.holds[:0], ac.holds[len(ac.holds)-n:]...)
	}
}

// corrLifeGap prices a change of state the way the formula prices everything
// else: the difference it makes to the chance of dying, times what a life is
// worth. It is the same three calls addRest and warmthValue make, which is the
// point - a count taken in different units from the formula could not be held
// against it.
func (w *World) corrLifeGap(s *SelfView, v0, h0, v1, h1 float64) float64 {
	before := pressures(&w.cfg, s, v0, h0, 0)
	after := pressures(&w.cfg, s, v1, h1, 0)
	return gap(&w.cfg, before, after) * w.cfg.LifeValue
}

// corrIncoming prices no longer being hit: the difference between this body's
// odds with a blow landing on it and without one.
func (w *World) corrIncoming(s *SelfView) float64 {
	hit := pressures(&w.cfg, s, s.Vitality, s.Hunger, w.cfg.AttackDamage)
	free := pressures(&w.cfg, s, s.Vitality, s.Hunger, 0)
	return gap(&w.cfg, hit, free) * w.cfg.LifeValue
}

// fireCorr records one event: in its own right, against the decisions still
// eligible for it, against whatever event this body last had, and against the
// bodies that were there to see it.
func (w *World) fireCorr(a *Agent, e CorrEvent, value float64) {
	c := &w.corr
	c.events[e]++
	c.value[e] += value

	ac := w.corrOf(a)
	window := w.cfg.CorrelateWindow
	kept := ac.trace[:0]
	for _, t := range ac.trace {
		age := w.tick - t.at
		if age > window {
			continue
		}
		kept = append(kept, t)
		cell := &c.cells[t.key*int(NumCorrEvents)+int(e)]
		cell.n++
		cell.value += value
		cell.pred += t.pred
		cell.lag += float64(age)
		pair := t.key*int(NumCorrEvents) + int(e)
		if ac.hasSeen(pair) {
			c.repeats++
		} else {
			ac.markSeen(pair)
		}
	}
	ac.trace = kept

	// One event after another, over the same body: the table that says
	// whether there is a middle term to compose on.
	for prev := CorrEvent(0); prev < NumCorrEvents; prev++ {
		if t := ac.lastOf(prev); t != 0 && w.tick-t <= window {
			c.pairs[prev][e]++
			c.pairLag[prev][e] += w.tick - t
		}
	}
	ac.note(e, w.tick)

	// And back onto the things that came into this body's hands lately, which
	// is the other half of #148: what a thing turns out to be worth is what
	// follows holding it. Signed, so a kind that is followed by trouble loses
	// what a kind followed by a meal gains.
	live := ac.holds[:0]
	for _, h := range ac.holds {
		if w.tick-h.at > window {
			continue
		}
		live = append(live, h)
		c.itemN[h.kind]++
		c.itemSum[h.kind] += value
	}
	ac.holds = live

	// And who was there. This is the witness channel of #148 measured before
	// it is built: an event nobody sees can teach nobody but the one it
	// happened to.
	crowd := 0
	w.forEachWitness(a.X, a.Y, a.ID, func(*Agent) { crowd++ })
	if crowd > 0 {
		c.watched[e]++
		c.onlookers[e] += crowd
	}
}

func (c *agentCorr) lastOf(e CorrEvent) int     { return c.last[e] }
func (c *agentCorr) note(e CorrEvent, tick int) { c.last[e] = tick }

// closeProbes answers the time-keyed question for any window that has run its
// length: what became of this body exactly so many ticks after the decision.
func (w *World) closeProbes(ac *agentCorr, s *SelfView, now *corrSnap) {
	for i := range ac.probes {
		p := &ac.probes[i]
		if !p.open || w.tick-p.at < corrHorizons[i] {
			continue
		}
		d := w.corrLifeGap(s, p.vitality, p.hunger, now.vitality, now.hunger)
		w.corr.spanAll[i].add(d)
		w.corr.spanKey[i][p.key].add(d)
		p.open = false
	}
}

// --- what two bodies could trade -------------------------------------------

// sampleWants asks, now and then, whether two bodies in sight of each other
// each hold something the other would rather have.
//
// It is the count the barter this world never built turns on. Stage 48 was the
// gate #73 asked for, but what it implemented was the one-way handover, so
// "two things moving at once" has never had a word - and whether it would ever
// fire is a question about the world rather than about the word.
//
// Sampled rather than swept because it is quadratic in the neighbourhood and
// answers a question about a rate, and because the hands it reads only change
// every few hundred ticks.
func (w *World) sampleWants() {
	every := w.cfg.CorrelateSample
	if every <= 0 || w.tick%every != 0 {
		return
	}
	c := &w.corr
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive || len(a.carried) == 0 {
			continue
		}
		for k := range a.carried {
			if a.carried[k].Kind == FoodTrinket {
				c.trinketHands++
				break
			}
		}
		c.samples++
		spare := w.corrSpare(a)
		seen := false
		w.forEachWitness(a.X, a.Y, a.ID, func(o *Agent) {
			if seen || !o.Alive {
				return
			}
			// A sale needs three things at once, and this is the count of how
			// often all three are true: something the seller can part with, a
			// body that can see it, and money in that body's hand.
			if spare && o.coinsHeld() > 0 {
				c.sellerReady++
				spare = false
			}
			if g := w.corrSwapGain(a, o); g > 0 {
				c.swaps++
				c.gain += g
				seen = true
			}
			if g := w.corrFoodGain(a, o); g > 0 {
				c.foodSwaps++
				c.foodGain += g
			}
		})
	}
}

// corrSpare says whether this body has anything to spare, which here means
// nothing more than holding more than one thing.
//
// It is a rough proxy and deliberately so: the question being counted is
// whether the three conditions a sale needs are ever true at once, and a
// generous reading of the seller's side makes the answer an upper bound. If
// even the upper bound were small, the market would have an excuse; it is
// twenty thousand, and it does not.
func (w *World) corrSpare(a *Agent) bool {
	return len(a.carried) > 1
}

// corrSwapGain is what two bodies would both make by exchanging one ornament
// for another: the only place in this world where the same thing is worth
// different amounts to two bodies for a reason that is not their condition
// (trinketDelight, stage 84). Nought unless both sides gain.
func (w *World) corrSwapGain(a, o *Agent) float64 {
	best := 0.0
	for i := range a.carried {
		fa := &a.carried[i]
		if fa.Kind != FoodTrinket {
			continue
		}
		for j := range o.carried {
			fo := &o.carried[j]
			if fo.Kind != FoodTrinket {
				continue
			}
			mine := w.trinketWorth(a, fo) - w.trinketWorth(a, fa)
			theirs := w.trinketWorth(o, fa) - w.trinketWorth(o, fo)
			if mine > 0 && theirs > 0 && mine+theirs > best {
				best = mine + theirs
			}
		}
	}
	return best
}

// corrFoodGain is the same question asked of food, where what makes two bodies
// differ is the discount on eating the same thing over and over (stage 16).
// The gain is smaller by an order of magnitude, which is the reason for asking
// both: if the ornaments do not carry a swap, nothing will.
func (w *World) corrFoodGain(a, o *Agent) float64 {
	best := 0.0
	for i := range a.carried {
		fa := &a.carried[i]
		if !w.canEat(a, fa) || !w.canEat(o, fa) {
			continue
		}
		for j := range o.carried {
			fo := &o.carried[j]
			if !w.canEat(a, fo) || !w.canEat(o, fo) {
				continue
			}
			mine := w.dietValue(a, fo.Kind) - w.dietValue(a, fa.Kind)
			theirs := w.dietValue(o, fa.Kind) - w.dietValue(o, fo.Kind)
			if mine > 0 && theirs > 0 && mine+theirs > best {
				best = mine + theirs
			}
		}
	}
	return best
}

// --- gifts, and whether one is ever returned --------------------------------

// noteGift records one thing handed over. It rides on giveItem, which is the
// one place a gift happens, so nothing is detected twice or anew.
func (w *World) noteGift(from, to *Agent) {
	if !w.corr.on {
		return
	}
	c := &w.corr
	c.gifts++
	if at, ok := c.open[[2]int{to.ID, from.ID}]; ok {
		// This body is giving back to somebody who gave to it.
		c.back++
		c.backTicks += w.tick - at
		delete(c.open, [2]int{to.ID, from.ID})
		return
	}
	c.open[[2]int{from.ID, to.ID}] = w.tick
}

// --- reading it out ---------------------------------------------------------

// CorrKey is one (situation, move) pair as the counting found it, for the
// handful worth printing.
type CorrKey struct {
	Feature HintFeature
	Act     ActionKind
	Event   CorrEvent

	N     int     // times the event followed this key inside the window
	Lift  float64 // how much more often than the event happens at all
	Value float64 // what it was worth when it happened, on average
	Want  float64 // Value times how often it follows this key: the expectation
	Pred  float64 // what the formula expected the move to be worth, on average
	Lag   float64 // ticks between the decision and the event
}

// CorrPair is one event following another over the same body, with the figure
// that says whether the two are associated at all.
//
// Rate is the share of the second event's occurrences that had the first
// inside the window before them, and Lift is that against how often the first
// precedes any event. Lift near one means the pair fires often only because
// both events are common, which is the trap the raw counts fall into: eating
// follows everything, because eating happens.
type CorrPair struct {
	Before, After CorrEvent
	N             int
	Rate, Lift    float64
}

// CorrItem is one kind of thing, priced by what followed holding it: the back
// propagation of #148.
//
// Gain is the figure that matters. Value on its own says only how the world
// was going while this was in a hand, and the world is mostly going the same
// way for everybody; what the thing is worth is how far it moves that.
type CorrItem struct {
	Kind FoodKind

	Got   int     // times one came into a hand
	N     int     // events credited to holding one
	Value float64 // what those events came to, on average
	Gain  float64 // that, less what any event comes to on average
	Held  float64 // ticks it stays in a hand before it goes
	Gone  int     // times one left a hand
}

// CorrComposed is two links joined at their middle term: this situation and
// this move tend to be followed by that, and that tends to be followed by the
// other. The other is what the composed link is about, and the middle term is
// gone from it - which is why the item table above exists.
type CorrComposed struct {
	Feature HintFeature
	Act     ActionKind
	Middle  CorrEvent
	Event   CorrEvent

	N      int     // the smaller of the two links' counts, as a weight
	Want   float64 // what the composition says this move is worth
	Direct float64 // what the same move is worth by direct observation, if seen
	Pred   float64 // what the formula expected of the move
	Lag    float64 // the two lags added
}

// CorrelateUse is what the counting came to. Every figure is taken whether or
// not anything would use it, and none of it is read by any rule.
type CorrelateUse struct {
	// Decisions is how many were filed, Keys how many distinct (situation,
	// move) pairs were ever decided, and Noise the spread of the error the
	// bodies made scoring their options - the yardstick everything below is
	// held against.
	Decisions, Keys int
	Noise           float64

	// Features and Acts are what the keys turned out to be about. The first
	// is the one #137 was caught by: an argmax over continuous features can
	// collapse onto whichever is pinned at one, and if it does here too then
	// most of the key space is decoration.
	Features [NumHintFeatures]int
	Acts     [numActionKinds]int

	// Events, Value, Watched and Onlookers are the events in their own
	// right: how many, what each was worth on average, the share that had
	// anybody there to see them, and how many onlookers each had. The last
	// two are the witness channel measured before it is built.
	Events    [NumCorrEvents]int
	Value     [NumCorrEvents]float64
	Watched   [NumCorrEvents]float64
	Onlookers [NumCorrEvents]float64

	// Live is how many (key, event) pairs the counting saw at all, Loud how
	// many of those were worth more than a quarter of the evaluation noise,
	// Varying how many are keyed on something that differs between the
	// candidates of one decision (a term that lifts every option alike
	// cancels out of the comparison - stage 86), and Unwritten how many of
	// the loud ones the formula expected nothing of.
	//
	// Acted is how many of the loud ones are worth anything on average
	// rather than only when they come off: an outcome worth a whole child
	// that follows one courtship in fifty is not something to lean on.
	//
	// Unwritten is the figure this whole item turns on. A relation the
	// formula prices is one the learning would count twice.
	Live, Loud, Varying, Acted, Unwritten int

	// Repeats is how often a body met a (key, event) pair it had already met
	// once - the event "the second time" turns on, which is what a promotion
	// rule would wait for.
	Repeats int

	// Top is the loudest, for reading. Held deeper than anything would print
	// because the terminal event swamps the head of it: a death is worth the
	// whole of LifeValue, so every key it touches outranks every key it does
	// not, and what is worth reading is further down.
	Top []CorrKey

	// Links is the same for the pairs: which event actually raises the odds
	// of which, rather than which pair happens to fire most.
	Links []CorrPair

	// Pairs is one event following another over the same body, and PairKinds
	// how many of those hundred cells ever fired. CoinToFed is the one cell
	// the money question turns on: if getting a coin is never followed by
	// eating, no amount of composing will make a coin worth anything.
	Pairs     [NumCorrEvents][NumCorrEvents]int
	PairKinds int
	CoinToFed int

	// Span is what a fixed window says instead, for each of the three: Told
	// is how much of the variance in what became of a body the key accounts
	// for, which is the time-keyed rule's whole claim. Near nought at every
	// window is the reading that says the horizon has to be an event.
	SpanN    [numCorrHorizons]int
	SpanMean [numCorrHorizons]float64
	SpanTold [numCorrHorizons]float64

	// CoinHold is how long money sits in a hand before it is spent, Spent how
	// many coins were, and Stuck the share that arrived and never left.
	CoinHold float64
	Spent    int
	Stuck    float64

	// Swap is the share of the sampled moments where two bodies in sight of
	// each other each held an ornament the other would rather have, Gain what
	// the two of them together stood to make, and FoodSwap the same asked of
	// food. TrinketHands is the share of sampled bodies holding an ornament
	// at all - the stock any of this would have to work with.
	Swap, Gain         float64
	FoodSwap, FoodGain float64
	TrinketHands       float64

	// Items is what each kind of thing turned out to be followed by, and
	// Composed is what two links make when they share a middle term. Both
	// are the two halves of #148 measured before either is built.
	Items    []CorrItem
	Composed []CorrComposed

	// ComposeErr is what a composition claims over what the same move is
	// worth by direct observation, averaged over the compositions where the
	// triple was also seen directly. One is a composition that tells the
	// truth; far above one is a rule of thumb that would have a body chasing
	// something that does not pay.
	//
	// It is the test the whole of composing turns on, and it is why Direct
	// is carried at all.
	ComposeErr float64
	ComposeN   int

	// SellerReady is how often all three things a sale needs were true at
	// once: something spare, a body in sight, and a coin in its hand.
	SellerReady int

	// Gifts is how many things were handed over, Back how many of those were
	// answered by a gift the other way, and BackTicks how long that took.
	Gifts, Back int
	BackTicks   float64
}

// ItemGain is what one kind of thing turned out to be worth, or nought for a
// kind that never came into a hand. For the measuring, which wants one figure
// rather than a table.
func (u CorrelateUse) ItemGain(k FoodKind) float64 {
	for _, it := range u.Items {
		if it.Kind == k {
			return it.Gain
		}
	}
	return 0
}

// Correlate reports it. It writes nothing and draws nothing.
func (w *World) Correlate() CorrelateUse {
	c := &w.corr
	out := CorrelateUse{
		Decisions: c.decisions,
		Features:  c.features,
		Acts:      c.acts,
		Events:    c.events,
		Pairs:     c.pairs,
		Repeats:   c.repeats,
		Spent:     c.itemGone[FoodCoin],
		Gifts:     c.gifts,
		Back:      c.back,
	}
	if !c.on {
		return out
	}
	out.Noise = c.noise.mean()
	quiet := out.Noise * corrQuietShare
	for k := 0; k < numCorrKeys; k++ {
		if c.keyN[k] > 0 {
			out.Keys++
		}
	}
	for e := CorrEvent(0); e < NumCorrEvents; e++ {
		if c.events[e] > 0 {
			out.Value[e] = c.value[e] / float64(c.events[e])
			out.Watched[e] = float64(c.watched[e]) / float64(c.events[e])
			out.Onlookers[e] = float64(c.onlookers[e]) / float64(c.events[e])
		}
	}
	// The base rate: how often each event follows a decision at all. A key is
	// only worth anything in so far as it beats this.
	var base [NumCorrEvents]float64
	if c.decisions > 0 {
		for e := CorrEvent(0); e < NumCorrEvents; e++ {
			base[e] = float64(c.events[e]) / float64(c.decisions)
		}
	}
	for k := 0; k < numCorrKeys; k++ {
		if c.keyN[k] == 0 {
			continue
		}
		for e := CorrEvent(0); e < NumCorrEvents; e++ {
			cell := &c.cells[k*int(NumCorrEvents)+int(e)]
			if cell.n == 0 {
				continue
			}
			out.Live++
			rate := float64(cell.n) / float64(c.keyN[k])
			lift := 0.0
			if base[e] > 0 {
				lift = rate / base[e]
			}
			value := cell.value / float64(cell.n)
			pred := cell.pred / float64(cell.n)
			// What choosing this move in this situation is worth on
			// average, which is the figure the formula's own number is
			// comparable with.
			want := value * rate
			if math.Abs(value) <= quiet {
				continue
			}
			out.Loud++
			feature := HintFeature(k / int(numActionKinds))
			if feature >= HintStrength {
				// The four that are read off whoever the move is aimed at,
				// and therefore the four that can tell one candidate from
				// another within a decision.
				out.Varying++
			}
			if math.Abs(pred) < math.Abs(want)*corrWrittenShare {
				out.Unwritten++
			}
			if math.Abs(want) > quiet {
				// And the ones a body could act on: an outcome that is
				// worth a great deal but hardly ever follows is not worth
				// leaning on, however loud it is when it comes.
				out.Acted++
			}
			out.Top = append(out.Top, CorrKey{
				Feature: feature,
				Act:     ActionKind(k % int(numActionKinds)),
				Event:   e,
				N:       cell.n,
				Lift:    lift,
				Value:   value,
				Want:    want,
				Pred:    pred,
				Lag:     cell.lag / float64(cell.n),
			})
		}
	}
	sortCorrKeys(out.Top)
	if len(out.Top) > 60 {
		out.Top = out.Top[:60]
	}
	out.Links = c.links()
	for b := CorrEvent(0); b < NumCorrEvents; b++ {
		for e := CorrEvent(0); e < NumCorrEvents; e++ {
			if c.pairs[b][e] > 0 {
				out.PairKinds++
			}
		}
	}
	out.CoinToFed = c.pairs[CorrCoin][CorrFed] + c.pairs[CorrCoin][CorrMended]
	for i := range c.spanAll {
		out.SpanN[i] = c.spanAll[i].n
		out.SpanMean[i] = c.spanAll[i].mean()
		if v := c.spanAll[i].variance(); v > 0 {
			// How much of the spread in what became of a body is accounted
			// for by which key it was: the weighted variance within the keys,
			// against the variance over the lot.
			within, n := 0.0, 0
			for k := range c.spanKey[i] {
				s := &c.spanKey[i][k]
				if s.n < 2 {
					continue
				}
				within += s.variance() * float64(s.n)
				n += s.n
			}
			if n > 0 {
				out.SpanTold[i] = clamp(1-(within/float64(n))/v, 0, 1)
			}
		}
	}
	out.Items = c.items()
	out.Composed = c.compose(quiet)
	for _, m := range out.Composed {
		if m.Direct != 0 {
			out.ComposeErr += m.Want / m.Direct
			out.ComposeN++
		}
	}
	if out.ComposeN > 0 {
		out.ComposeErr /= float64(out.ComposeN)
	}
	if c.itemGone[FoodCoin] > 0 {
		out.CoinHold = c.itemHeld[FoodCoin] / float64(c.itemGone[FoodCoin])
	}
	if c.itemGot[FoodCoin] > 0 {
		out.Stuck = 1 - float64(c.itemGone[FoodCoin])/float64(c.itemGot[FoodCoin])
	}
	if c.samples > 0 {
		out.Swap = float64(c.swaps) / float64(c.samples)
		out.FoodSwap = float64(c.foodSwaps) / float64(c.samples)
		out.TrinketHands = float64(c.trinketHands) / float64(c.samples)
	}
	if c.swaps > 0 {
		out.Gain = c.gain / float64(c.swaps)
	}
	if c.foodSwaps > 0 {
		out.FoodGain = c.foodGain / float64(c.foodSwaps)
	}
	out.SellerReady = c.sellerReady
	if c.back > 0 {
		out.BackTicks = float64(c.backTicks) / float64(c.back)
	}
	return out
}

// items is what each kind of thing turned out to be worth, by what followed
// holding one.
//
// The baseline is every event credited to anything at all. A thing in a hand
// while the world goes the way it usually goes has told its holder nothing;
// what it is worth is the difference.
func (c *correlateWatch) items() []CorrItem {
	var n int
	var sum float64
	for k := FoodKind(0); k < NumFoodKinds; k++ {
		n += c.itemN[k]
		sum += c.itemSum[k]
	}
	if n == 0 {
		return nil
	}
	base := sum / float64(n)
	out := make([]CorrItem, 0, NumFoodKinds)
	for k := FoodKind(0); k < NumFoodKinds; k++ {
		if c.itemGot[k] == 0 {
			continue
		}
		it := CorrItem{Kind: k, Got: c.itemGot[k], N: c.itemN[k], Gone: c.itemGone[k]}
		if it.N > 0 {
			it.Value = c.itemSum[k] / float64(it.N)
			it.Gain = it.Value - base
		}
		if it.Gone > 0 {
			it.Held = c.itemHeld[k] / float64(it.Gone)
		}
		out = append(out, it)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Gain > out[j-1].Gain; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// compose joins two links at their middle term: (situation, move) -> B, and
// B -> C, giving (situation, move) -> C.
//
// What the composition claims the move is worth is the far outcome's value
// times both rates - how often B follows the move, and how often C follows B.
// Where the same triple was also seen directly, Direct says what it came to,
// and the two together are the test: a composition that does not agree with
// direct observation is a rule of thumb this world would be wrong to carry.
//
// Nothing here is a rule. No body composes anything; this asks whether it
// would be worth teaching one to.
func (c *correlateWatch) compose(quiet float64) []CorrComposed {
	if c.decisions == 0 {
		return nil
	}
	// How often each event has each other one behind it, over all events, so
	// that a middle term can be charged for how often it precedes anything.
	// Without this the composition multiplies in a base rate: a mend follows
	// a coin twenty-three times over, because mending follows everything.
	var afterAny float64
	for e := CorrEvent(0); e < NumCorrEvents; e++ {
		afterAny += float64(c.events[e])
	}
	var before [NumCorrEvents]float64
	for b := CorrEvent(0); b < NumCorrEvents; b++ {
		n := 0
		for e := CorrEvent(0); e < NumCorrEvents; e++ {
			n += c.pairs[b][e]
		}
		if afterAny > 0 {
			before[b] = float64(n) / afterAny
		}
	}
	out := make([]CorrComposed, 0, 32)
	for k := 0; k < numCorrKeys; k++ {
		if c.keyN[k] == 0 {
			continue
		}
		for b := CorrEvent(0); b < NumCorrEvents; b++ {
			first := &c.cells[k*int(NumCorrEvents)+int(b)]
			if first.n == 0 || c.events[b] == 0 {
				continue
			}
			toB := float64(first.n) / float64(c.keyN[k])
			for e := CorrEvent(0); e < NumCorrEvents; e++ {
				if e == b || c.pairs[b][e] == 0 || c.events[e] == 0 {
					continue
				}
				second := &c.cells[k*int(NumCorrEvents)+int(e)]
				// Only the part of "e follows b" that is more than e
				// follows anything: the rest is the base rate, and
				// multiplying base rates together is how a composition
				// ends up claiming a move is worth four hundred.
				rate := float64(c.pairs[b][e]) / float64(c.events[e])
				if before[b] <= 0 || rate <= before[b] {
					continue
				}
				excess := 1 - before[b]/rate
				toC := float64(c.pairs[b][e]) / float64(c.events[b]) * excess
				worth := c.value[e] / float64(c.events[e])
				want := worth * toB * toC
				if math.Abs(want) <= quiet {
					continue
				}
				n := first.n
				if c.pairs[b][e] < n {
					n = c.pairs[b][e]
				}
				cm := CorrComposed{
					Feature: HintFeature(k / int(numActionKinds)),
					Act:     ActionKind(k % int(numActionKinds)),
					Middle:  b, Event: e, N: n, Want: want,
					Lag:  first.lag/float64(first.n) + float64(c.pairLag[b][e])/float64(c.pairs[b][e]),
					Pred: first.pred / float64(first.n),
				}
				if second.n > 0 {
					cm.Direct = second.value / float64(second.n) *
						float64(second.n) / float64(c.keyN[k])
				}
				out = append(out, cm)
			}
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && math.Abs(out[j].Want) > math.Abs(out[j-1].Want); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}

// links is which event raises the odds of which.
//
// pairs[b][c] is how many occurrences of c had a b inside the window before
// them, so the share is over c's own count. What that share has to be held
// against is how often b precedes anything at all - otherwise every column
// ranks by how common its event is, and the table says only that eating is
// common.
func (c *correlateWatch) links() []CorrPair {
	var afterAny float64
	for e := CorrEvent(0); e < NumCorrEvents; e++ {
		afterAny += float64(c.events[e])
	}
	if afterAny == 0 {
		return nil
	}
	var before [NumCorrEvents]float64
	for b := CorrEvent(0); b < NumCorrEvents; b++ {
		n := 0
		for e := CorrEvent(0); e < NumCorrEvents; e++ {
			n += c.pairs[b][e]
		}
		before[b] = float64(n) / afterAny
	}
	out := make([]CorrPair, 0, int(NumCorrEvents)*int(NumCorrEvents))
	for b := CorrEvent(0); b < NumCorrEvents; b++ {
		for e := CorrEvent(0); e < NumCorrEvents; e++ {
			n := c.pairs[b][e]
			if n == 0 || c.events[e] == 0 || before[b] <= 0 {
				continue
			}
			rate := float64(n) / float64(c.events[e])
			out = append(out, CorrPair{Before: b, After: e, N: n,
				Rate: rate, Lift: rate / before[b]})
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Lift > out[j-1].Lift; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// sortCorrKeys puts the loudest first. An insertion sort: the slice is a few
// hundred at most and this keeps the package free of another import.
func sortCorrKeys(k []CorrKey) {
	for i := 1; i < len(k); i++ {
		for j := i; j > 0 && math.Abs(k[j].Value)*k[j].Lift > math.Abs(k[j-1].Value)*k[j-1].Lift; j-- {
			k[j], k[j-1] = k[j-1], k[j]
		}
	}
}
