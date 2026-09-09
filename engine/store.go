package engine

import (
	"fmt"
	"math"
)

// A place to put things, and knowing where it is (stage 50).
//
// Everything a body has been able to keep so far it has kept in its hands
// (stage 40), which holds one item and charges for the weight of it. A store
// is the other way of keeping something: a spot on the ground where things can
// be left, and picked up again by whoever knows the spot.
//
// The target was counted before this was written, as stage 40's and 49's were,
// and it came out split by map. The world is at its allowance for plants 9% of
// the time on the flat world and 86% of the time on the map that is played on
// - so on the played map an item put in a store is, most of the time, a plant
// that will not grow. A store cannot make food, and it cannot stop food going
// off either (#77: the clock does not stop for being kept). What is left for
// it to be worth is WHEN and WHERE food is to hand, which is the same "knowing
// where something is" that has now failed to move this world four times
// (stages 30b, 31, 35 and 49).
//
// The shape of it, and what it deliberately is not.
//
//   - A store is not a container. What is in it is in the world, at that spot,
//     in the same list as everything else lying about - so it counts against
//     the world's allowance, it goes off at the ordinary rate, and eating out
//     of a store is eating, with no rule of its own. The only thing a store
//     does to an item is hide it from anybody who does not know the place.
//   - It is not built. Like the ground and the regions it is a thing whoever
//     lays the world out puts there (edit.go's footing): the simulation never
//     makes one, and there is no action for building. What a body can do is
//     put something in one and take something out.
//   - The knowledge is not a skill (#75). A skill is good anywhere and never
//     fades; a store is one place, and a body that stops going there forgets
//     it. That is the shape of what an agent makes of a region - fading on
//     reading, its clock reset by being there - so that is what is used.
//   - The knowledge takes no room from what a body can remember about people
//     (#41). Somewhere is not somebody. What bounds it instead is MaxStores,
//     which keeps the count in the same order as the regions.

// storeMemory is one agent's knowledge of one store: how firmly it knows it
// and when it last had anything to do with it.
//
// It fades exactly as a view of a region does, by the same expression, and for
// the same reason it is written as a strength rather than a flag: a place
// somebody was told about once and never went to should slip away, and a place
// they use every day should not.
type storeMemory struct {
	n        float64
	logN     float64
	lastTick int

	// looks is how often this body has looked at the place while not knowing
	// it. It is how a cache is first noticed (StoreFindLooks) and it is reset
	// when the place is learned, so it costs nothing after that.
	looks int
}

func (m *storeMemory) learn(strength float64, cap float64, tick int) {
	m.n = math.Min(m.n+strength, cap)
	m.logN = math.Log(m.n)
	m.lastTick = tick
	m.looks = 0
}

// faded is the region view's test, on the store's own record. The two are the
// same question - has this gone stale - and the same answer, written out here
// rather than shared through a type, because a store's record holds nothing
// else and a shared one would carry three unused figures onto every agent.
func (m *storeMemory) faded(rate float64, tick int) bool {
	if m.n <= 0 {
		return true
	}
	elapsed := tick - m.lastTick
	if elapsed <= 0 || rate <= 0 {
		return false
	}
	return rate*float64(elapsed) > m.logN
}

// store is a place on the ground, and nothing else: what is in it lives in the
// world's own list of things lying about.
type store struct {
	X, Y float64
}

// --- what a body knows ------------------------------------------------------

// knowsStore reports whether this agent could find store i.
//
// Everybody knows every store in the arm that says so, which is the control
// this stage is read against: it leaves the storing and takes away the
// knowing, the way stage 49's WaresSeen leaves the crying and takes away the
// hearing.
func (w *World) knowsStore(a *Agent, i int) bool {
	if i < 0 || i >= len(w.stores) {
		return false
	}
	if w.cfg.StoresKnownToAll {
		return true
	}
	if i >= len(a.stores) {
		return false
	}
	return !a.stores[i].faded(w.storeForgetRate(a), w.tick)
}

// storeForgetRate is how fast this agent loses a place, which is how fast it
// loses country: the memory gene buys the same thing here that it buys there
// (#41), and no new figure is invented for it.
func (w *World) storeForgetRate(a *Agent) float64 {
	return w.cfg.RegionForgetPerTick / math.Max(a.MemoryScale(&w.cfg), 1e-9)
}

// learnStore is the one place a store is written into an agent.
//
// Four things call it. Three of them are the ways the plan named (#75) - being
// born to somebody who knows it, seeing somebody use it, and hearing it cried
// - and writing them showed that none of them can start, because every one
// needs somebody who already knows. The fourth is noticing one (perceive,
// StoreFindChance), and it is the only path that needs nobody else.
//
// Standing on a cache is not one of them. A place to keep things is not a
// landmark, and that is the whole reason knowing where one is is worth
// anything at all.
func (w *World) learnStore(a *Agent, i int, strength float64) {
	if i < 0 || i >= len(w.stores) || strength <= 0 {
		return
	}
	w.storeRoomFor(a)
	a.stores[i].learn(strength, w.cfg.RegionMemory*a.MemoryScale(&w.cfg), w.tick)
	w.storeLearned++
}

// storeRoomFor makes sure this body has a place to keep what it knows about
// the caches. One entry per cache in the world, which is bounded by MaxStores.
func (w *World) storeRoomFor(a *Agent) {
	if len(a.stores) < len(w.stores) {
		grown := make([]storeMemory, len(w.stores))
		copy(grown, a.stores)
		a.stores = grown
	}
}

// noticeStore is a body looking at a cache it does not know. It is the only
// path to a place that needs nobody else, and it counts rather than rolls -
// see StoreFindLooks for why that is a measurement decision.
func (w *World) noticeStore(a *Agent, i int) bool {
	if w.cfg.StoreFindLooks <= 0 {
		return false
	}
	w.storeRoomFor(a)
	a.stores[i].looks++
	if a.stores[i].looks < w.cfg.StoreFindLooks {
		return false
	}
	w.learnStore(a, i, w.cfg.StoreWitnessStrength)
	w.storeFound++
	return true
}

// inheritStores is a child taking its parents' places with it. It is the
// widest of the three paths and the reason a store can end up belonging to a
// line rather than to the world.
func (w *World) inheritStores(child *Agent, parents ...*Agent) {
	if len(w.stores) == 0 || w.cfg.StoreInheritStrength <= 0 {
		return
	}
	for _, p := range parents {
		if p == nil {
			continue
		}
		for i := range p.stores {
			if w.knowsStore(p, i) {
				w.learnStore(child, i, w.cfg.StoreInheritStrength)
			}
		}
	}
}

// witnessStoreUse is what using a store leaves with the people who saw it.
//
// This is the third use of the witness scan and the first one since stage 49
// said what it is for: a killing and a drowning are things that happen in an
// instant, and so is somebody reaching into a cache. (Stage 49's wares were
// the other kind - a body crying is in sight for as long as it cries - which
// is why that one went through the perception instead.)
func (w *World) witnessStoreUse(a *Agent, i int) {
	if w.cfg.StoreWitnessStrength <= 0 {
		return
	}
	w.forEachWitness(a.X, a.Y, a.ID, func(o *Agent) {
		if o.Species != a.Species {
			return // a place to keep food is not a thing a predator learns
		}
		w.learnStore(o, i, w.cfg.StoreWitnessStrength)
		w.storeSeen++
	})
}

// tellStores is what a body crying its wares says beyond what is in its hand
// (stage 49's pair). Somebody advertising is somebody saying where the goods
// are, so everybody who can see it picks up the places it knows.
//
// It is weaker than seeing a store used, for the reason every second-hand
// figure in this world is weaker than a first-hand one (exchangeRegions): a
// place you were told about is a place you have not been.
func (w *World) tellStores(a *Agent) {
	if w.cfg.StoreCryStrength <= 0 || len(w.stores) == 0 {
		return
	}
	w.forEachWitness(a.X, a.Y, a.ID, func(o *Agent) {
		if o.Species != a.Species {
			return
		}
		for i := range w.stores {
			if w.knowsStore(a, i) {
				w.learnStore(o, i, w.cfg.StoreCryStrength)
				w.storeTold++
			}
		}
	})
}

// --- putting something in ---------------------------------------------------

// storeRoom is how much room is left in a store.
func (w *World) storeRoom(i int) int {
	if i < 0 || i >= len(w.stores) || w.cfg.StoreCapacity <= 0 {
		return 0
	}
	held := 0
	for k := range w.foods {
		if w.foods[k].Store == i+1 {
			held++
		}
	}
	return w.cfg.StoreCapacity - held
}

// putInStore moves what is in a body's hand onto the store's spot.
//
// Nothing is created and nothing is destroyed: the item goes from the hand
// into the world's own list, which is where it would have gone if the body had
// died holding it. What changes is that it is now somewhere on purpose, and
// that only those who know the place can see it.
func (w *World) putInStore(a *Agent, i int) bool {
	if len(a.carried) == 0 || w.storeRoom(i) <= 0 || !w.knowsStore(a, i) {
		return false
	}
	f := a.carried[0]
	f.ID = 0
	f.X, f.Y = w.stores[i].X, w.stores[i].Y
	f.Store = i + 1
	w.removeCarried(a, 0)
	w.putFood(f)
	w.stored++
	// Using it teaches whoever was watching, and using it is also how the
	// body keeps its own knowledge of the place fresh.
	w.learnStore(a, i, w.cfg.StoreWitnessStrength)
	w.witnessStoreUse(a, i)
	return true
}

// tookFromStore is the other half, and it is not a rule of its own: taking
// something out of a store is eating it or picking it up, through exactly the
// paths that do those things to anything else lying about. All this does is
// notice, so that the place stays fresh in the taker's mind and the people who
// saw it learn it too.
func (w *World) tookFromStore(a *Agent, f *Food) {
	if f.Store <= 0 {
		return
	}
	i := f.Store - 1
	w.withdrawn++
	w.learnStore(a, i, w.cfg.StoreWitnessStrength)
	w.witnessStoreUse(a, i)
}

// --- laying them out (the editor's side) ------------------------------------

// SetStore puts a store on the ground and returns which one it is. Like the
// terrain and the regions it is the hand of whoever is laying the world out:
// the simulation never calls it, and it draws nothing from the random source.
func (w *World) SetStore(x, y float64) (int, error) {
	if w.cfg.StoreCapacity <= 0 {
		return 0, fmt.Errorf("this world has no stores in it (StoreCapacity is 0)")
	}
	if len(w.stores) >= w.cfg.MaxStores {
		return 0, fmt.Errorf("a world holds at most %d stores", w.cfg.MaxStores)
	}
	w.stores = append(w.stores, store{X: clamp(x, 0, w.cfg.Width), Y: clamp(y, 0, w.cfg.Height)})
	return len(w.stores) - 1, nil
}

// RemoveStore takes one away. What was in it is left lying where it was, in
// the open: the things in a store are in the world, and taking the store away
// does not take them out of it.
func (w *World) RemoveStore(i int) error {
	if i < 0 || i >= len(w.stores) {
		return fmt.Errorf("no such store: %d", i)
	}
	for k := range w.foods {
		switch {
		case w.foods[k].Store == i+1:
			w.foods[k].Store = 0
		case w.foods[k].Store > i+1:
			w.foods[k].Store--
		}
	}
	w.stores = append(w.stores[:i], w.stores[i+1:]...)
	// Everybody's knowledge of it goes with it, including the ones waiting to
	// be born: what an agent holds is one entry per store, in order, so a
	// removal that missed a body would leave it knowing the wrong places.
	forget := func(a *Agent) {
		if i < len(a.stores) {
			a.stores = append(a.stores[:i], a.stores[i+1:]...)
		}
	}
	for j := range w.agents {
		forget(&w.agents[j])
	}
	for j := range w.newborns {
		forget(&w.newborns[j])
	}
	return nil
}

// StoreView is where a store is and what is in it, for an editor and a viewer.
type StoreView struct {
	Index int
	X, Y  float64
	Held  int
	Room  int
}

// Stores is where the stores are. Read only, and the caller's copy.
func (w *World) Stores() []StoreView {
	out := make([]StoreView, 0, len(w.stores))
	for i := range w.stores {
		room := w.storeRoom(i)
		out = append(out, StoreView{
			Index: i, X: w.stores[i].X, Y: w.stores[i].Y,
			Held: w.cfg.StoreCapacity - room, Room: room,
		})
	}
	return out
}

// StoresKnownBy is how many of the world's stores this agent could find, for
// the panel and for the measurement.
func (w *World) StoresKnownBy(id int) (known, total int) {
	a := w.agentByID(id)
	if a == nil {
		return 0, len(w.stores)
	}
	for i := range w.stores {
		if w.knowsStore(a, i) {
			known++
		}
	}
	return known, len(w.stores)
}

// StoreUse is what the stores came to. Read only.
type StoreUse struct {
	// Stores is how many there are and Held how many items are in them.
	Stores int
	Held   int

	// Known is the share of the world's stores an average living body could
	// find, and Knowers the share of bodies that know at least one.
	Known   float64
	Knowers float64

	// Deposits and Withdrawals are how many items have gone in and come out,
	// and Learned how many times anybody came to know a place - split by the
	// path it came down: seen in use, or heard cried. What is left over is
	// inheritance.
	Deposits    int
	Withdrawals int
	Learned     int
	Found       int
	Seen        int
	Told        int
}

// Stored reports what the stores came to.
func (w *World) Stored() StoreUse {
	out := StoreUse{
		Stores: len(w.stores), Deposits: w.stored, Withdrawals: w.withdrawn,
		Learned: w.storeLearned, Found: w.storeFound,
		Seen: w.storeSeen, Told: w.storeTold,
	}
	if len(w.stores) == 0 {
		return out
	}
	for k := range w.foods {
		if w.foods[k].Store > 0 {
			out.Held++
		}
	}
	n := 0.0
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive || a.Species != SpeciesHuman {
			continue
		}
		n++
		known := 0
		for j := range w.stores {
			if w.knowsStore(a, j) {
				known++
			}
		}
		out.Known += float64(known) / float64(len(w.stores))
		if known > 0 {
			out.Knowers++
		}
	}
	if n > 0 {
		out.Known /= n
		out.Knowers /= n
	}
	return out
}
