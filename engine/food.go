package engine

// This file is what there is to eat, and who may eat it.
//
// Until stage 11 there was one kind of food that grew in the world and anybody
// could take. Enemies need a second: a body is worth eating, which is what
// makes hunting something other than a fight. That single fact - killing this
// creature leaves food behind - is what puts a reason to hunt together into
// the world without any rule mentioning cooperation.
//
// Two rules keep it from being a free lunch. A species does not eat its own
// dead, so a human killing a human gains nothing edible by it. And a carcass
// belongs for a while to whoever brought it down, so waiting at a safe
// distance for somebody else to make the kill is not the best move available
// (PLAN.md's own warning about the distribution rule deciding everything).

// FoodKind is what a food item is.
type FoodKind uint8

const (
	// FoodPlant grows in the world on its own. Anything that eats plants can
	// take it and nobody has a claim on it.
	FoodPlant FoodKind = iota

	// FoodMeat is what is left of somebody. It remembers whose species it
	// came from, because that decides who is allowed to eat it.
	FoodMeat

	// FoodFish is what the water holds (stage 42). It is a third kind rather
	// than a kind of plant because the whole point of it is that it is
	// somewhere else: a body eats it where it swims, which is the cell that
	// is dear to cross and drowns people.
	FoodFish

	// NumEdibleKinds is how many of them can be eaten. Everything below this
	// line is food; what follows is not, and the rules that are about eating
	// - the diet ledger, what a mouthful is worth - stop here.
	NumEdibleKinds

	// FoodStone is a stone lying about (stage 45): the first thing in this
	// world that can be picked up and not eaten. It is in the same list as
	// the food because that is the list of things lying about - the spatial
	// index, carrying and the viewer all work on it already - and calling it
	// a kind of food is the price of not writing a second one.
	FoodStone = NumEdibleKinds

	// FoodCoin is money (stage 51), and it is here for the same reason the
	// stone is: it is a thing lying about that can be picked up, and the
	// index, the carrying and the viewer already work on that list.
	//
	// It is not a kind of food in any other sense. Nothing eats it, it takes
	// no room in the world's allowance for plants, and it weighs nothing to
	// carry (#66) - which is the only thing it has going for it, and, as it
	// turned out, the only reason anybody would ever take one.
	FoodCoin = NumEdibleKinds + 1

	// NumFoodKinds is how many there are, for the code that keeps one figure
	// per kind (the diet rule of stage 16). It is not a kind, and it is
	// spelled out rather than left to iota: the line above ends the run, and
	// a bare name here would repeat it rather than carry on.
	NumFoodKinds = FoodCoin + 1
)

func (k FoodKind) String() string {
	switch k {
	case FoodMeat:
		return "meat"
	case FoodFish:
		return "fish"
	case FoodStone:
		return "stone"
	case FoodCoin:
		return "coin"
	}
	return "plant"
}

// eatsPlants and eatsMeat are the diet of a species.
//
// They are functions of the species rather than genes: what a creature can
// digest is what it is, not something it spends budget on. Humans live on
// plants and will eat the dead of other kinds; enemies live on meat alone,
// which is what makes them hunt rather than graze.
func eatsPlants(s Species) bool { return s == SpeciesHuman }
func eatsMeat(s Species) bool   { return true }

// A species that lives on meat alone has to hunt. That is what makes the
// enemy a reason for anything rather than scenery.

// canEat says whether this agent may take this item, at this moment.
func (w *World) canEat(a *Agent, f *Food) bool {
	switch f.Kind {
	case FoodStone, FoodCoin:
		return false // nothing eats a stone, and nobody eats money
	case FoodFish:
		return w.eatsFish(a.Species)
	case FoodMeat:
		if !eatsMeat(a.Species) {
			return false
		}
		// Nobody eats its own kind.
		if f.From == a.Species {
			return false
		}
		return f.claimedBy(a.ID, w.tick)
	default:
		// What this body can digest (stage 60). For a human that is what it
		// always was; for an enemy it is what its row says, which is the same
		// line stage 11 drew between the species, drawn one level finer.
		return w.eatsPlantsFor(a)
	}
}

// claimedBy reports whether this agent may take the item: either the claim has
// run out and it is anybody's, or the agent is one of those who brought the
// carcass down.
func (f *Food) claimedBy(id, tick int) bool {
	if tick >= f.ClaimUntil || len(f.Claim) == 0 {
		return true
	}
	for _, c := range f.Claim {
		if c == id {
			return true
		}
	}
	return false
}

// heldBy reports whether this agent is one of those the item belongs to,
// whether or not the claim has run out. It answers "was this its kill", which
// is a different question from claimedBy's "may it eat this now".
func (f *Food) heldBy(id int) bool {
	for _, c := range f.Claim {
		if c == id {
			return true
		}
	}
	return false
}

// dropMeat leaves a carcass where an agent died.
//
// How much there is scales with how much the dead creature was made of, so a
// large enemy feeds a group and a small one barely feeds the agent that killed
// it. That is the whole of the reason to hunt together: the drop grows with the
// size of the animal while what one agent can bring down alone does not.
func (w *World) dropMeat(a *Agent) {
	if w.cfg.MeatPerBudget <= 0 {
		return
	}
	items := int(a.Bulk(&w.cfg) / w.cfg.MeatPerBudget)
	if items <= 0 {
		return
	}
	// Only those who could actually eat it hold a claim. A human that killed
	// another human has no use for the carcass, and letting it hold one would
	// mean the meat sat there spoiling while something that could eat it
	// waited - a rule about sharing a kill turning into a rule about spite.
	claim := a.recentAttackers(w.tick, w.cfg.HuntCreditTicks)
	kept := claim[:0]
	for _, id := range claim {
		if killer := w.agentByID(id); killer != nil && eatsMeat(killer.Species) && killer.Species != a.Species {
			kept = append(kept, id)
		}
	}
	claim = kept

	// How much of this carcass those who brought it down could actually take
	// away (stage 41). Counted whatever the rule is set to, so that an arm
	// with the surplus closed still says how much of a surplus there was:
	// the whole question is whether a kill leaves more than its party can
	// use, and the answer has to be measured before it is acted on.
	//
	// What each of them may actually pick up, which is the same question
	// carrying answers and so the same function: anything with hands keeps at
	// least one item, and a bigger body keeps more where the world's hands
	// are big enough for builds to differ at all. A party of weak bodies
	// leaves more behind than a party of strong ones, and a party of two
	// keeps more than one of them alone - which is what ties the surplus to
	// hunting together rather than to being large (#68).
	theirs := 0
	for _, id := range claim {
		if c := w.agentByID(id); c != nil {
			// One each at the very least, hands or no hands: the claim was
			// always "whoever brought it down eats first", and eating does
			// not need a hand. A world with carrying turned off keeps the
			// ownership it always had.
			theirs += max(1, c.carrySlots(&w.cfg))
		}
	}
	if len(claim) == 0 {
		theirs = items // nobody's kill: nothing is anybody's to keep
	}
	if theirs > items {
		theirs = items
	}
	w.meatItems += items
	w.meatKeepable += theirs

	// A kill that leaves a carcass somebody can eat is a hunt, and how many
	// took part in it is the figure stage 11 turns on: pack hunting, if it
	// appears at all, appears here as a party size above one.
	if len(claim) > 0 {
		w.hunts++
		w.huntParty += len(claim)
	}
	// Counted apart from the affinity so that the arm with the credit turned
	// off still reports how often the rule had the chance to fire.
	if len(claim) > 1 {
		w.jointHunts++
	}
	w.rememberHunt(claim)
	for i := 0; i < items; i++ {
		if w.countKind(FoodMeat) >= w.cfg.MaxMeatItems {
			return // as much meat as the world will hold is already lying about
		}
		// What the party can carry away is theirs; the rest is nobody's
		// (stage 41). It is not a new kind of ownership and there is nothing
		// to steal - the claim was only ever "wait your turn", and beyond
		// what they can take away there is no turn to wait for. Whoever comes
		// for the rest is picking up something that was left, which is why no
		// spite, no witnessing and no new competition rule is needed for it.
		held := claim
		until := w.tick + w.cfg.MeatClaimTicks
		if w.cfg.MeatSurplusFree && i >= theirs {
			held, until = nil, 0
		}
		w.putFood(Food{
			X: a.X + w.randRange(-6, 6), Y: a.Y + w.randRange(-6, 6),
			Kind: FoodMeat, From: a.Species,
			Claim: held, ClaimUntil: until,
			SpoilAt: w.tick + w.cfg.MeatSpoilTicks,
		})
		w.meatDropped++
	}
}

// rememberHunt is what everybody who brought the carcass down comes away with
// besides the meat: each of them remembers each of the others a little better.
//
// This is the only affinity that can be earned from a stranger, and it is
// deliberately hung on the distribution rule rather than on standing nearby.
// An agent that was there but never landed a blow is not on the list, so what
// is remembered is having taken the risk together, not having been present.
//
// Nothing is recorded for a party of one: there is nobody to remember.
func (w *World) rememberHunt(party []int) {
	if w.cfg.AffinityHunt <= 0 || len(party) < 2 {
		return
	}
	for _, id := range party {
		a := w.agentByID(id)
		if a == nil || !a.Alive {
			continue
		}
		for _, other := range party {
			if other != id {
				w.rememberAffinity(a, other, w.cfg.AffinityHunt)
			}
		}
	}
}

// clearSpoiled takes away the dead flesh nobody got to in time. Meat that
// stayed would fill the world's allowance for food and leave no room for
// anything to grow - and so would a fish landed and left on the bank, which is
// the same thing wearing a different name.
func (w *World) clearSpoiled() {
	// Including what is in somebody's hands (stage 40): the clock does not
	// stop for being carried, because a benefit with no price runs to the
	// ceiling (#77).
	for i := range w.agents {
		if a := &w.agents[i]; a.Alive && len(a.carried) > 0 {
			w.spoilCarried(a)
		}
	}
	for i := 0; i < len(w.foods); {
		if f := &w.foods[i]; f.SpoilAt > 0 && w.tick >= f.SpoilAt {
			switch f.Kind {
			case FoodMeat:
				w.meatSpoiled++
			case FoodFish:
				// A fish somebody landed and then left, or dropped on the
				// bank when it died (stage 42 and stage 40 together). The
				// world never plants a fish with a clock on it, so nothing
				// here takes a living fish out of the river.
				w.fishSpoiled++
			}
			w.removeFoodByID(f.ID)
			continue // the last item was swapped into this slot
		}
		i++
	}
}

// meatFrom is how many items of meat this agent's carcass would leave.
func (w *World) meatFrom(a *Agent) float64 {
	if w.cfg.MeatPerBudget <= 0 {
		return 0
	}
	return a.Bulk(&w.cfg) / w.cfg.MeatPerBudget
}
