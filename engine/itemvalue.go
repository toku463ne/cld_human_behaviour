package engine

// What a thing in the hand is worth, learnt rather than written down (#148).
//
// The rest of this world prices a thing by what the designer worked out it
// does: a meal by what it takes off the hunger, a coat by how much cold it
// keeps off, a coin by what it is a claim on. That covers the things whose use
// somebody thought of. It does not cover a thing whose worth is not in itself
// but in what comes after it - and this world has one of those already (a hide
// is not food, not warmth and not money; it is the thing a coat is made of)
// and would have more if money ever worked.
//
// So a body may also hold an opinion about a kind of thing, and that opinion is
// the running mean of what followed having one. Nothing is labelled: the
// outcome is priced by the same pressures -> gap -> LifeValue the formula uses
// everywhere, so a thing that is followed by trouble loses exactly what a thing
// followed by a meal gains. That is the user's own rule (2026-09-23) about not
// teaching one side of a thing, applied to the thing that learns.
//
// Four decisions, all made before a line of it was written.
//
// It replaces composing links rather than joining it. Composing "offer, then a
// coin, then a meal" into "offer, then a meal" drops the coin out of it, so a
// body carrying only composed links has no reason to pick one up, keep it, or
// refuse to hand it over. Crediting the thing puts the reason back, and the
// counting found the composition dishonest anyway (it claimed 13 to 30 times
// what direct observation said, because multiplying two conditional rates
// multiplies in their base rates).
//
// The room is bought. Slots come out of the same budget the genes are fitted
// to, exactly as a rule of thumb's room and a lesson's room do. A body with an
// opinion about three kinds of thing is measurably smaller than one with none,
// which is the conservation law CLAUDE.md names.
//
// A coat is not an ornament. Stage 87a built the clothing on the trinkets, so
// by FoodKind the two are the same thing; the counting found that the one that
// keeps the weather off is worth about forty points more than the one that does
// not, which is the largest clean difference it turned up. Lumping them would
// have buried it.
//
// And it only ever adds. The opinion is a term in the comparison like any
// other, and an option still has to win against the same rivals; there is no
// branch anywhere that reads one.

// numItemKinds is how many opinions there are to be had, which is the food
// kinds plus the coat.
const numItemKinds = numCorrItems

// itemOpinion is what one body thinks one kind of thing is followed by, as a
// running mean of what actually followed.
type itemOpinion struct {
	kind int
	b    belief
}

// itemHold is one thing in a hand, and what has happened to its holder since
// it arrived. The credit is banked when the thing goes or the window runs out,
// so what the opinion is a mean of is "what followed having one of these",
// once per thing - not once per event, which would divide the whole of what a
// coat predicts by the hundreds of blows and mends that happen while it is on
// somebody's back.
type itemHold struct {
	kind   int
	at     int
	credit float64
}

// itemValue is what this body reckons a kind of thing is worth, or nought when
// it has no opinion about that kind. It is the only thing the decision reads.
func (a *Agent) itemValue(kind int) float64 {
	for i := range a.itemLore {
		if a.itemLore[i].kind == kind {
			return a.itemLore[i].b.mean
		}
	}
	return 0
}

// itemValues is the whole of it, for the perception to carry.
func (a *Agent) itemValues(cfg *Config) [numItemKinds]float64 {
	var out [numItemKinds]float64
	if cfg.ItemValueWeight == 0 {
		return out
	}
	for i := range a.itemLore {
		o := &a.itemLore[i]
		if o.kind >= 0 && o.kind < numItemKinds {
			out[o.kind] = o.b.mean * cfg.ItemValueWeight
		}
	}
	return out
}

// ItemValues reports what this body thinks its kinds of thing are worth, and
// how much room it paid for. Read only, for the viewer.
func (a *Agent) ItemValues() ([]ItemOpinion, int) {
	out := make([]ItemOpinion, 0, len(a.itemLore))
	for i := range a.itemLore {
		out = append(out, ItemOpinion{
			Name:  corrItemName(a.itemLore[i].kind),
			Worth: a.itemLore[i].b.mean,
			Seen:  a.itemLore[i].b.n,
		})
	}
	return out, a.itemSlots
}

// ItemOpinion is one of those, for reading.
type ItemOpinion struct {
	Name  string
	Worth float64
	Seen  float64
}

// --- learning ---------------------------------------------------------------

// Where the arrivals and departures are noticed, and where what happens to a
// body is credited, is the one pass correlate.go already makes over every body
// every tick (stepCorrelate, readEvents, fireCorr). The rule reads the same
// diff and the same events as the instrument on purpose: when the same
// question is answered in two places, the two answers drift.

// bankItemHold takes the oldest still open hold of that kind and writes what
// it came to into the body's opinion.
func (w *World) bankItemHold(a *Agent, kind int) {
	for i := range a.itemHolds {
		if a.itemHolds[i].kind != kind {
			continue
		}
		w.learnItem(a, kind, w.itemExcess(a, &a.itemHolds[i]))
		a.itemHolds = append(a.itemHolds[:i], a.itemHolds[i+1:]...)
		return
	}
}

// expireItemHolds banks anything that has been held longer than the window.
func (w *World) expireItemHolds(a *Agent) {
	window := w.cfg.ItemValueWindow
	if window <= 0 {
		return
	}
	for i := 0; i < len(a.itemHolds); {
		if w.tick-a.itemHolds[i].at < window {
			i++
			continue
		}
		w.learnItem(a, a.itemHolds[i].kind, w.itemExcess(a, &a.itemHolds[i]))
		a.itemHolds = append(a.itemHolds[:i], a.itemHolds[i+1:]...)
	}
}

// creditItemHolds is what one thing happening to a body does to every opinion
// it is in the middle of forming. It is called from the same place the
// counting is fed, so the two see the same events.
//
// The same event also goes into this body's own running total, which is what
// the credit is measured against when it is banked. Without that the opinions
// are all large and all positive: this world's events average out well above
// nought - a body mends dozens of times for every blow - so anything held for
// long enough collects a fortune it had nothing to do with.
func (w *World) creditItemHolds(a *Agent, value float64) {
	a.lifeCredit += value
	for i := range a.itemHolds {
		a.itemHolds[i].credit += value
	}
}

// itemExcess is what a hold came to, less what this body's life was coming to
// anyway over the same stretch.
//
// The baseline is the body's own and not the world's, which is the only kind
// it could be: nothing here may read anything a body could not know about
// itself. It is per tick because a coat is worn for a thousand ticks and a
// meal is gone in one, and the two have to be comparable.
func (w *World) itemExcess(a *Agent, h *itemHold) float64 {
	held := float64(w.tick - h.at)
	if held <= 0 || a.lifeTicks <= 0 {
		return h.credit
	}
	return h.credit - a.lifeCredit/float64(a.lifeTicks)*held
}

// learnItem moves this body's opinion about a kind of thing towards what just
// happened, and gives it room for a new opinion if it has any left.
//
// There is no eviction, for the reason the lessons have none: this world has
// never decided what makes one idea worth less than another, and the slot is
// the scarce thing that was bought.
func (w *World) learnItem(a *Agent, kind int, credit float64) {
	rate, cap := w.cfg.ItemValueRate, w.cfg.ItemValueCount
	for i := range a.itemLore {
		if a.itemLore[i].kind == kind {
			a.itemLore[i].b.observe(credit, rate, cap)
			w.itemsLearnt++
			return
		}
	}
	if len(a.itemLore) >= a.itemSlots {
		return
	}
	o := itemOpinion{kind: kind}
	o.b.observe(credit, rate, cap)
	a.itemLore = append(a.itemLore, o)
	w.itemsLearnt++
}

// --- where the room comes from ----------------------------------------------

// drawItemSlots is how much room a founder is born with, on the terms the
// rules of thumb and the lessons use: uniform over the range, so the first
// generation holds bodies that bought none and bodies that bought the lot.
func (w *World) drawItemSlots() int {
	if w.cfg.ItemSlots <= 0 {
		return 0
	}
	return w.rng.Intn(w.cfg.ItemSlots + 1)
}

// inheritItemSlots is how much room a child is born with: one parent's, and
// now and then one more.
func (w *World) inheritItemSlots(pa, pb *Agent, genius bool) int {
	if w.cfg.ItemSlots <= 0 {
		return 0
	}
	slots := pa.itemSlots
	if w.rng.Intn(2) == 1 {
		slots = pb.itemSlots
	}
	if genius && slots < w.cfg.ItemSlots {
		slots++
	}
	return min(slots, w.cfg.ItemSlots)
}

// itemSlotCost is what that room takes out of the budget the genes are fitted
// to, beside what the rules of thumb and the lessons already take.
func (w *World) itemSlotCost(slots int) float64 {
	return float64(slots) * w.cfg.ItemSlotCost
}

// Nothing is inherited but the room. A child is born with no opinion about
// anything, which is the same rule a lesson follows and for the same reason:
// this is what the body watched happen, not what its line was born believing.

// --- passing it on ----------------------------------------------------------

// exchangeItemValues is an opinion copied from one body to another, on the back
// of watching somebody, and on the terms a rule of thumb is copied on: into an
// empty slot, never over the top of one, and never about a kind the receiver
// already has an opinion about.
//
// A copied opinion arrives as a claim and not as the years of watching that
// produced it, which is how lore.go trades a fact: the mean comes across, the
// count does not, so being told does not make anybody surer of anything.
func (w *World) exchangeItemValues(a, o *Agent) int {
	if !w.cfg.ItemsSpread {
		return 0
	}
	copied := 0
	for i := range o.itemLore {
		if len(a.itemLore) >= a.itemSlots {
			break
		}
		if a.holdsItemLike(o.itemLore[i].kind) {
			continue
		}
		a.itemLore = append(a.itemLore, itemOpinion{
			kind: o.itemLore[i].kind,
			b:    belief{mean: o.itemLore[i].b.mean, n: 1},
		})
		w.itemsCopied++
		copied++
	}
	return copied
}

func (a *Agent) holdsItemLike(kind int) bool {
	for i := range a.itemLore {
		if a.itemLore[i].kind == kind {
			return true
		}
	}
	return false
}

// --- reading it out ---------------------------------------------------------

// ItemLoreUse is what a population is making of its opinions about things.
type ItemLoreUse struct {
	Slots float64 // room bought, per body
	Held  float64 // opinions actually carried, per body
	Kinds float64 // distinct kinds anybody has an opinion about

	// Worth is what the living think each kind is worth, averaged over the
	// bodies that have an opinion about it, and Holders how many do.
	Worth   [numItemKinds]float64
	Holders [numItemKinds]int

	Learnt, Copied int
}

// ItemLore reports it. Read only.
func (w *World) ItemLore() ItemLoreUse {
	out := ItemLoreUse{Learnt: w.itemsLearnt, Copied: w.itemsCopied}
	n := 0.0
	kinds := make(map[int]struct{})
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		n++
		out.Slots += float64(a.itemSlots)
		out.Held += float64(len(a.itemLore))
		for j := range a.itemLore {
			o := &a.itemLore[j]
			kinds[o.kind] = struct{}{}
			out.Worth[o.kind] += o.b.mean
			out.Holders[o.kind]++
		}
	}
	if n > 0 {
		out.Slots /= n
		out.Held /= n
	}
	for k := 0; k < numItemKinds; k++ {
		if out.Holders[k] > 0 {
			out.Worth[k] /= float64(out.Holders[k])
		}
	}
	out.Kinds = float64(len(kinds))
	return out
}

// ItemWorth is what the living reckon one kind is worth. Read only.
func (u ItemLoreUse) ItemWorth(name string) float64 {
	for k := 0; k < numItemKinds; k++ {
		if corrItemName(k) == name {
			return u.Worth[k]
		}
	}
	return 0
}
