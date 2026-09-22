package engine

// Nests people come out of (2026-09-22, TODO 20, decision #142).
//
// What it is for. Until now the only say a map's author had over where people
// are was the first tick: InitialPopulation bodies, scattered evenly, and
// after that the world grew its own. A nest lets the author say "a people
// starts here", which is what the dynasty wants - several countries, each
// founded by somebody, rather than sixty strangers spread over the map.
//
// Three things it is, and one it is not.
//
//   - It is on the shelf EnemySpawnTicks is on: the world lets a body in from
//     outside, in the tick, and no parent pays for it. It is NOT on Endow's
//     shelf, which is the one thing about the user's description this file
//     corrects - Endow and Inspire are only ever called from outside the
//     engine, and this is the world calling it on itself.
//
//   - It hands its people a line of their own (Agent.Lineage). That tag is
//     the whole of what "the founder of this country" means here, and the
//     engine already carries it from mother to child and through a save.
//
//   - It keeps its line up rather than pouring people out. Cap is how many of
//     its own line it wants alive; while there are fewer it sends another,
//     and when Life runs out it stops for good. Counted before it was built:
//     sixty founders come down to 5.08 lines in 20000 ticks and the biggest
//     of them holds half the world, so a nest that hands out founders is
//     handing out lines that mostly die. Keeping one up is the only version
//     of this worth having.
//
// What it is not is a new way of being born. Nothing here touches birth: no
// parents, no vitality paid, no courtship, no cooldown. It is the world
// putting a grown body in, exactly as it does with a beast.
//
// And it is the first time people in this world come from anywhere but a
// birth. A run with a nest in it cannot be compared body for body with one
// without, and an arm that uses one has to say so in its name.

// HumanNest is one place people come into the world, as the map's author laid
// it out. The numbers are here and never on the map (decision #133); the map
// carries the name and where it is.
type HumanNest struct {
	// Name is what the map calls this place, and Key the character it is
	// painted with in Config.HumanNestMap.
	Name string
	Key  byte

	// Rate is how many ticks apart the people it sends are, and Life how long
	// it goes on sending them at all. Either left at nought takes the world's
	// own figure (HumanNestTicks, HumanNestLife), which is the arrangement
	// the table of beasts already has.
	Rate int
	Life int

	// Cap is how many of its own line it keeps alive: while fewer than this
	// are living it sends another, and at nought it sends one every Rate
	// whatever is alive.
	//
	// Its own line rather than the bodies standing near it, which is the
	// asymmetry with a beast's nest (NestCap counts a crowd by where it
	// stands). A person carries the tag of the nest they came from and hands
	// it to their children, so counting the line is both possible and the
	// thing an author means: this country supports this people, wherever they
	// have got to.
	Cap int
}

// HumanNestView is one of them for whoever is drawing or playing the world:
// where it is, whose line comes out of it, how many it has sent and how long
// it has left. Read only; the engine never reads this back.
type HumanNestView struct {
	X, Y, W, H float64
	Name       string
	Lineage    uint16
	Sent       int
	Left       int
}

// buildHumanNests reads Config.HumanNestMap into one list of cells per row,
// once, and gives each row the line its people will carry.
//
// The tags carry on from the founders' own, so a world with sixty founders and
// three nests has lines 1 to 60 and then 61, 62, 63. Nothing collides and
// nothing has to be drawn to decide it.
func (w *World) buildHumanNests() {
	rows := w.cfg.HumanNestMap
	nests := w.cfg.HumanNests
	if len(rows) == 0 || len(nests) == 0 {
		return
	}
	at := map[byte]int{}
	for i := range nests {
		if k := nests[i].Key; k != 0 && k != '.' {
			at[k] = i
		}
	}
	if len(at) == 0 {
		return
	}
	cells := make([][]cell, len(nests))
	for r, row := range rows {
		h := w.cfg.Height / float64(len(rows))
		y := (float64(r) + 0.5) * h
		for c := 0; c < len(row); c++ {
			i, ok := at[row[c]]
			if !ok {
				continue
			}
			cw := w.cfg.Width / float64(len(row))
			cells[i] = append(cells[i], cell{x: (float64(c) + 0.5) * cw, y: y, w: cw, h: h})
		}
	}
	any := false
	for i := range cells {
		if len(cells[i]) > 0 {
			any = true
		}
	}
	if !any {
		return
	}
	w.humanNestCells = cells
	w.humanNestSent = make([]int, len(nests))
}

// humanNestLine is the line the people of this nest carry.
func (w *World) humanNestLine(nest int) uint16 {
	return uint16(w.cfg.InitialPopulation + nest + 1)
}

// spawnHumansOfTick lets each nest send one, when it is due one and short of
// its line.
//
// A world with no nests painted leaves without touching anything, which is
// every world before this one - down to the random source, so its runs are
// the runs it always had.
func (w *World) spawnHumansOfTick() {
	if len(w.humanNestCells) == 0 {
		return
	}
	for nest, cells := range w.humanNestCells {
		if len(cells) == 0 {
			continue
		}
		row := w.cfg.HumanNests[nest]
		rate, life := row.Rate, row.Life
		if rate <= 0 {
			rate = w.cfg.HumanNestTicks
		}
		if life <= 0 {
			life = w.cfg.HumanNestLife
		}
		if rate <= 0 || w.tick > life || w.tick%rate != 0 {
			continue
		}
		// Everything that could refuse is asked before anything is drawn, so
		// that a nest with nothing to do does not consume randomness and
		// shift the rest of the run. It is the rule spawnFood has kept since
		// the stones went in.
		if len(w.agents) >= w.cfg.MaxPopulation {
			continue
		}
		line := w.humanNestLine(nest)
		if row.Cap > 0 && w.livingOfLine(line) >= row.Cap {
			continue
		}
		c := cells[w.rng.Intn(len(cells))]
		x := clamp(c.x+w.randRange(-c.w/2, c.w/2), 20, w.cfg.Width-20)
		y := clamp(c.y+w.randRange(-c.h/2, c.h/2), 20, w.cfg.Height-20)
		a := w.randomAgentAt(SpeciesHuman, 0, x, y)
		a.Lineage = line
		w.addAgent(a)
		w.humanNestSent[nest]++
	}
}

// livingOfLine is how many bodies of this line are alive. Their children count
// too, because a child carries its mother's line: what the cap is about is
// whether the people this nest founded are still here, not how many of them it
// personally put there.
func (w *World) livingOfLine(line uint16) int {
	n := 0
	for i := range w.agents {
		if w.agents[i].Alive && w.agents[i].Lineage == line {
			n++
		}
	}
	return n
}

// HumanNests is every place the map painted for people to come out of. Read
// only, and empty in a world whose map painted none - which is every world
// before 2026-09-22.
func (w *World) HumanNests() []HumanNestView {
	var out []HumanNestView
	for nest, cells := range w.humanNestCells {
		row := w.cfg.HumanNests[nest]
		life := row.Life
		if life <= 0 {
			life = w.cfg.HumanNestLife
		}
		for _, c := range cells {
			out = append(out, HumanNestView{
				X: c.x, Y: c.y, W: c.w, H: c.h,
				Name:    row.Name,
				Lineage: w.humanNestLine(nest),
				Sent:    w.humanNestSent[nest],
				Left:    max(life-w.tick, 0),
			})
		}
	}
	return out
}
