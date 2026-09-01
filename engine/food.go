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

// A species that lives on meat alone has to hunt. That is what makes the
// enemy a reason for anything rather than scenery.

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

	// A kill that leaves a carcass somebody can eat is a hunt, and how many
	// took part in it is the figure stage 11 turns on: pack hunting, if it
	// appears at all, appears here as a party size above one.
	if len(claim) > 0 {
		w.hunts++
		w.huntParty += len(claim)
	}
	for i := 0; i < items; i++ {
		if w.countKind(FoodMeat) >= w.cfg.MaxMeatItems {
			return // as much meat as the world will hold is already lying about
		}
		w.putFood(Food{
			X: a.X + w.randRange(-6, 6), Y: a.Y + w.randRange(-6, 6),
			Kind: FoodMeat, From: a.Species,
			Claim: claim, ClaimUntil: w.tick + w.cfg.MeatClaimTicks,
			SpoilAt: w.tick + w.cfg.MeatSpoilTicks,
		})
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

// meatFrom is how many items of meat this agent's carcass would leave.
func (w *World) meatFrom(a *Agent) float64 {
	if w.cfg.MeatPerBudget <= 0 {
		return 0
	}
	return a.Budget() / w.cfg.MeatPerBudget
}
