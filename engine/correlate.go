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
// before the relation counts as one the designer already wrote down. A key
// whose realised value is large while the formula scored the move at nothing
// is a relation the formula does not have; one where the two move together is
// a relation it does.
const corrWrittenShare = 0.5

// corrCell is one (key, event) pair as the counting sees it.
type corrCell struct {
	n     int
	value float64 // what the event was worth, summed
	pred  float64 // what the formula had scored the move at, summed
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
	carried, coins   int
	attacked         bool
	alive            bool
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

	// coinAt is when the money now in this body's hands first arrived, for
	// the one figure that says whether a coin is ever spent.
	coinAt int

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
	pairs [NumCorrEvents][NumCorrEvents]int

	// The time-keyed reading, for each window: what became of a body so many
	// ticks after a decision, both overall and per key. A key that says no
	// more than the overall figure says nothing.
	spanAll [numCorrHorizons]corrSpread
	spanKey [][]corrSpread // numCorrHorizons x numCorrKeys

	// How long money sits in a hand: sum and count over the coins that were
	// spent or handed on, and how many are still sitting there at the end.
	coinHeld  float64
	coinSpent int
	coinFirst int // coins that arrived in a hand at all

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
// body actually took.
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
			carried: len(a.carried), coins: a.coinsHeld(),
			attacked: a.attackerID != 0, alive: a.Alive,
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
	if now.coins > was.coins {
		if ac.coinAt == 0 {
			ac.coinAt = w.tick
			w.corr.coinFirst++
		}
		w.fireCorr(a, CorrCoin, 0)
	}
	if now.coins < was.coins {
		if ac.coinAt != 0 {
			w.corr.coinHeld += float64(w.tick - ac.coinAt)
			w.corr.coinSpent++
			ac.coinAt = 0
		}
	}
	if now.coins == 0 {
		ac.coinAt = 0
	}
	// What is in the hands, money aside: one event for something arriving and
	// one for something leaving, because the two are the two sides of every
	// exchange this world could have.
	wasThings, nowThings := was.carried-was.coins, now.carried-now.coins
	if nowThings > wasThings {
		w.fireCorr(a, CorrThing, 0)
	} else if nowThings < wasThings {
		w.fireCorr(a, CorrGave, 0)
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
		}
	}
	ac.note(e, w.tick)

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
	Value float64 // what it was worth, on average
	Pred  float64 // what the formula had scored the move at, on average
	Lag   float64 // ticks between the decision and the event
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
	// the loud ones the formula had not already scored the move for.
	//
	// Unwritten is the figure this whole item turns on. A relation the
	// formula prices is one the learning would count twice.
	Live, Loud, Varying, Unwritten int

	// Repeats is how often a body met a (key, event) pair it had already met
	// once - the event "the second time" turns on, which is what a promotion
	// rule would wait for.
	Repeats int

	// Top is the loudest handful, for reading.
	Top []CorrKey

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

	// SellerReady is how often all three things a sale needs were true at
	// once: something spare, a body in sight, and a coin in its hand.
	SellerReady int

	// Gifts is how many things were handed over, Back how many of those were
	// answered by a gift the other way, and BackTicks how long that took.
	Gifts, Back int
	BackTicks   float64
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
		Spent:     c.coinSpent,
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
			if math.Abs(pred) < math.Abs(value)*corrWrittenShare {
				out.Unwritten++
			}
			out.Top = append(out.Top, CorrKey{
				Feature: feature,
				Act:     ActionKind(k % int(numActionKinds)),
				Event:   e,
				N:       cell.n,
				Lift:    lift,
				Value:   value,
				Pred:    pred,
				Lag:     cell.lag / float64(cell.n),
			})
		}
	}
	sortCorrKeys(out.Top)
	if len(out.Top) > 12 {
		out.Top = out.Top[:12]
	}
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
	if c.coinSpent > 0 {
		out.CoinHold = c.coinHeld / float64(c.coinSpent)
	}
	if c.coinFirst > 0 {
		out.Stuck = 1 - float64(c.coinSpent)/float64(c.coinFirst)
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

// sortCorrKeys puts the loudest first. An insertion sort: the slice is a few
// hundred at most and this keeps the package free of another import.
func sortCorrKeys(k []CorrKey) {
	for i := 1; i < len(k); i++ {
		for j := i; j > 0 && math.Abs(k[j].Value)*k[j].Lift > math.Abs(k[j-1].Value)*k[j-1].Lift; j-- {
			k[j], k[j-1] = k[j-1], k[j]
		}
	}
}
