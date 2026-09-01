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
)

func (k FoodKind) String() string {
	if k == FoodMeat {
		return "meat"
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

// canEat says whether this agent may take this item, at this moment.
func (w *World) canEat(a *Agent, f *Food) bool {
	switch f.Kind {
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
		return eatsPlants(a.Species)
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
	items := int(a.Budget() / w.cfg.MeatPerBudget)
	if items <= 0 {
		return
	}
	claim := a.recentAttackers(w.tick, w.cfg.HuntCreditTicks)
	for i := 0; i < items; i++ {
		id := w.addFood(a.X+w.randRange(-6, 6), a.Y+w.randRange(-6, 6))
		if id == 0 {
			return // the world is as full of food as it may be
		}
		f := w.foodByID(id)
		f.Kind = FoodMeat
		f.From = a.Species
		f.Claim = claim
		f.ClaimUntil = w.tick + w.cfg.MeatClaimTicks
		f.SpoilAt = w.tick + w.cfg.MeatSpoilTicks
	}
}

// clearSpoiled takes away the carcasses nobody got to in time. Meat that stayed
// would fill the world's allowance for food and leave no room for anything to
// grow.
func (w *World) clearSpoiled() {
	for i := 0; i < len(w.foods); {
		if f := &w.foods[i]; f.SpoilAt > 0 && w.tick >= f.SpoilAt {
			w.removeFoodByID(f.ID)
			continue // the last item was swapped into this slot
		}
		i++
	}
}
