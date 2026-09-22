package engine

import (
	"fmt"
	"math"
)

// Founding a village (2026-09-22, the dynasty's own rule).
//
// This is on the shelf Endow and SetTerrain are on, and for the same reason:
// nothing in the simulation calls it. No rule, no controller, no tick. The
// only caller is whatever is running a game, and a world nobody is playing
// runs exactly as it did - down to the random source, because this draws
// nothing from it.
//
// What it is for. The dynasty asks a player to leave their line living in
// every country the map marks, and the map's countries are full of strangers
// who will not follow anybody home: a pair bond in this world lasts until the
// child is born and no longer. So a player arriving in a far country has
// nobody to have children with unless somebody lives there. Founding a
// village is how the player puts somebody there - and the village hands out a
// line of its own, not the player's, because what the player has to do with
// it is marry into it.
//
// The price is paid in coins out of the founder's own hands, and the coins are
// laid on the ground where the village stands rather than deleted. Money in
// this world is conserved - nothing makes it and nothing eats it (stage 51) -
// and a game that burned it would be quietly removing the one thing a player
// is collecting. Left on the ground it is the village's own money, and it can
// be picked up again by whoever gets there first.
const foundCoinRing = 6.0 // how far from the founder the paid coins land

// FoundNest puts a place people come out of where one body is standing, paid
// for out of that body's hands.
//
// It returns the line the new village will hand its people. The row carries
// the village's own figures (how often, for how long, how many of its line it
// keeps alive); Key and Lineage are filled in here, and Since is set to now so
// that Life is counted from the founding rather than from the world's first
// tick.
func (w *World) FoundNest(id, price int, row HumanNest) (uint16, error) {
	a := w.agentByID(id)
	if a == nil || !a.Alive {
		return 0, fmt.Errorf("found: no body %d", id)
	}
	if price < 0 {
		price = 0
	}
	coins := 0
	for i := range a.carried {
		if a.carried[i].Kind == FoodCoin {
			coins++
		}
	}
	if coins < price {
		return 0, fmt.Errorf("found: %d coins in hand and %d asked for", coins, price)
	}
	key, err := w.freeNestKey()
	if err != nil {
		return 0, err
	}
	rows := w.nestRowsFor()
	r := clampInt(int(a.Y/w.cfg.Height*float64(len(rows))), 0, len(rows)-1)
	line := []byte(rows[r])
	c := clampInt(int(a.X/w.cfg.Width*float64(len(line))), 0, len(line)-1)
	if line[c] != nestUnpainted {
		return 0, fmt.Errorf("found: there is already a village on this spot")
	}
	line[c] = key
	rows[r] = string(line)

	// Paid before anything else is changed, so that a refusal leaves the
	// world exactly as it was.
	w.payCoins(a, price)

	row.Key = key
	row.Since = w.tick
	w.cfg.HumanNests = append(w.cfg.HumanNests, row)
	w.cfg.HumanNestMap = rows
	sent := w.humanNestSent
	w.buildHumanNests()
	// buildHumanNests starts the counts again; the villages that were already
	// sending keep theirs, because that count is what an interface shows and
	// a village does not forget what it has done because another was founded.
	for i := range sent {
		if i < len(w.humanNestSent) {
			w.humanNestSent[i] = sent[i]
		}
	}
	return w.humanNestLine(len(w.cfg.HumanNests) - 1), nil
}

// nestUnpainted is the character for a cell no village stands on.
const nestUnpainted = '.'

// nestRowsFor is the painting villages are placed on, made to match the
// ground when there is not one yet. A copy, so that a refusal half way
// through leaves the world's own painting alone.
func (w *World) nestRowsFor() []string {
	if len(w.cfg.HumanNestMap) > 0 {
		return append([]string(nil), w.cfg.HumanNestMap...)
	}
	rows, cols := 20, 30
	if n := len(w.cfg.TerrainMap); n > 0 {
		rows = n
		if c := len(w.cfg.TerrainMap[0]); c > 0 {
			cols = c
		}
	}
	out := make([]string, rows)
	blank := make([]byte, cols)
	for i := range blank {
		blank[i] = nestUnpainted
	}
	for i := range out {
		out[i] = string(blank)
	}
	return out
}

// freeNestKey is a character no village is painted with yet.
func (w *World) freeNestKey() (byte, error) {
	taken := map[byte]bool{nestUnpainted: true}
	for i := range w.cfg.HumanNests {
		taken[w.cfg.HumanNests[i].Key] = true
	}
	for k := byte('A'); k <= 'z'; k++ {
		if !taken[k] {
			return k, nil
		}
	}
	return 0, fmt.Errorf("found: no character left to paint a village with")
}

// payCoins takes coins out of a body's hands and lays them on the ground
// around it, in a ring rather than a heap so that they can be told apart and
// picked up one at a time.
//
// No random numbers: this is called from outside the simulation, and a world
// that drew from the source when a player pressed a key would run differently
// for having been watched.
func (w *World) payCoins(a *Agent, price int) {
	paid := 0
	for i := 0; i < len(a.carried) && paid < price; {
		if a.carried[i].Kind != FoodCoin {
			i++
			continue
		}
		angle := 2 * math.Pi * float64(paid) / math.Max(float64(price), 1)
		w.putFood(Food{
			X:    clamp(a.X+math.Cos(angle)*foundCoinRing, 10, w.cfg.Width-10),
			Y:    clamp(a.Y+math.Sin(angle)*foundCoinRing, 10, w.cfg.Height-10),
			Kind: FoodCoin,
		})
		w.heldKind[FoodCoin]--
		a.carried = append(a.carried[:i], a.carried[i+1:]...)
		paid++
	}
}
