package engine

// The weather a place keeps, and what it costs to stand in it (stage 85).
//
// Nothing in this file does anything yet. It is the same kind of stage as the
// groundwork laid for the terrain on 2026-09-04: every reading goes through
// one function, that function returns the neutral answer everywhere, and the
// world runs bit for bit as it did. What it buys is that the stage which puts
// a climate on a map has one number to fill in rather than a design to settle.
//
// Three decisions are made here, and they are the whole of the stage.
//
// Where it lives: on the region, not on the cell. A region already carries
// everything that varies by place (how sheltered the resting is, how well the
// plants grow, how much of the awkward crop comes up, what the ground does to
// a body, where the enemies arrive), and, more to the point, this world
// already knows how to learn and pass on what a region is like - stage 15b
// learns it by standing there, 15c tells somebody, stage 29 folds the going
// underfoot into the same record and stage 35 does the same for what kills.
// A climate on the cells would be a second map about the same thing with none
// of that machinery, and stage 29 measured the ceiling anyway: twelve blocks,
// because the controller has no route finding and cannot use anything finer.
//
// What it costs: vitality, per tick, through the drain. That is the one
// decision that makes everything else free. The utility formula prices staying
// alive as the change in the chance of dying inside the planning window, and
// that chance is read off a drain - so a body standing in the cold reads its
// own danger exactly the way a hungry one does, the lookahead carries it into
// the second window with no new code, and anything that cancels part of it is
// priced the way a meal is priced. Every rule in this project that mattered
// worked that way (the carcass that mends, the cooking); every rule that
// invented its own goal instead had to be kept small or turned off.
//
// And which half of the risk it belongs to: the body's own drain, not the
// extra. Stage 67 draws that line - what is hitting a body now is a fact about
// now and is deliberately not carried into the second window, because a fight
// carried forward makes every candidate equally hopeless. A place does not
// stop being cold while a body thinks about it, so the cold goes with the
// metabolism, which is the half that is carried.

// Weather is one thing a place can be. There is one today; the list is where
// a map's author gets more of them (heat, dark, damp, snow), and adding one is
// a line here, a character in the map, and whatever answers it.
type Weather uint8

const (
	// WeatherChill is how cold it is here, from 0 (the ordinary world) up.
	WeatherChill Weather = iota

	// NumWeathers is how many there are. No rule counts them; it is the size
	// of the array on a region.
	NumWeathers
)

// buildClimate reads the map's picture of its weather into the regions.
//
// The picture is written the way the terrain's is - one line per row of the
// world, stretched to fit - except that it is read at the middle of each
// region, because that is the grain a region is. A world that says nothing
// about its weather gets nothing, which is the default.
//
//	.       the ordinary world
//	1 - 9   that much cold
//
// Letters are left alone on purpose: they are where a second kind of weather
// will go, and a map that uses one today should read as ordinary rather than
// as something else.
func (w *World) buildClimate() {
	if len(w.regions) == 0 {
		return
	}
	// Cleared first, so that an editor redrawing the picture takes the old
	// weather away rather than leaving it under the new one.
	for i := range w.regions {
		w.regions[i].Weather = [NumWeathers]float64{}
	}
	rows := w.cfg.ClimateMap
	if len(rows) == 0 {
		return
	}
	cols := 0
	for _, r := range rows {
		if n := len([]rune(r)); n > cols {
			cols = n
		}
	}
	if cols == 0 {
		return
	}
	rcols, rrows := max(w.cfg.RegionCols, 1), max(w.cfg.RegionRows, 1)
	for i := range w.regions {
		// The middle of this block, as a share of the world, read off the
		// picture.
		cx := (float64(i%rcols) + 0.5) / float64(rcols)
		cy := (float64(i/rcols) + 0.5) / float64(rrows)
		row := []rune(rows[clampInt(int(cy*float64(len(rows))), 0, len(rows)-1)])
		c := '.'
		if x := clampInt(int(cx*float64(cols)), 0, cols-1); x < len(row) {
			c = row[x]
		}
		if c >= '1' && c <= '9' {
			w.regions[i].Weather[WeatherChill] = float64(c-'0') / 9
		}
	}
}

// weatherAt is how much of one weather this spot has, and nought everywhere in
// a world that was given none.
func (w *World) weatherAt(x, y float64, kind Weather) float64 {
	if r := w.regionAt(x, y); r != nil {
		return r.Weather[kind]
	}
	return 0
}

// chillOf is what standing here costs this body in vitality per tick.
//
// It is the one place the cold is turned into a number, so it is also the one
// place anything that answers the cold will be subtracted - a coat, a fire,
// whatever a later stage gives the world. wardsOff is that seam, and it is
// nought today: nothing a body can hold answers the weather yet.
func (w *World) chillOf(a *Agent) float64 {
	if w.cfg.ChillDrain <= 0 {
		return 0
	}
	cold := w.weatherAt(a.X, a.Y, WeatherChill)
	if cold <= 0 {
		return 0
	}
	return w.cfg.ChillDrain * cold * (1 - clamp(w.wardsOff(a, WeatherChill), 0, 1))
}

// chillFelt is what this body reads of the cold it is standing in: the whole
// of it in the ordinary world, and nothing where the weather is not something
// a body can feel (stage 86's control). What it pays is not affected.
func (w *World) chillFelt(a *Agent) float64 {
	if !w.cfg.ChillKnown {
		return 0
	}
	return w.chillOf(a)
}

// wardsOff is how much of one weather this body is protected from by what it
// is carrying: nothing, today.
//
// It is written as a function rather than left out so that the stage which
// adds a warm thing has one place to put it, and so that the shape is decided
// now: what answers the weather is a property of what is in a hand, it is a
// share of the cost rather than a flat subtraction (nothing makes a place
// warm, it only makes it bearable), and it is read here rather than in the
// utility formula, so that wanting one and being sheltered by one cannot
// disagree.
func (w *World) wardsOff(a *Agent, kind Weather) float64 {
	return 0
}

// --- reading it out ---------------------------------------------------------

// WeatherUse is what the weather came to (stage 86). Read only.
type WeatherUse struct {
	// OnCold is how cold it is where the humans are, All how cold the world
	// is on average, and Gain the difference. Below nought is a population
	// that has left the cold; nought is one that stands wherever it happens
	// to be, however cold the map is. It is stage 57's Standing in the same
	// shape and for the same reason: a spread that nobody responds to reads
	// nought here however large it is.
	OnCold, All, Gain float64

	// ColdFood is how much of the world's plant growth happens on the cold
	// ground, relative to an equal share. It is here because three stages
	// (33, 35, 36) measured the same thing: a reason to avoid a place and a
	// reason to eat there sit on two different maps, and while they do,
	// avoiding is always throwing food away. Without this column a population
	// that stays in the cold cannot be told from one that is staying with its
	// dinner.
	ColdFood float64

	// Taken is the vitality the cold has taken over the run and Starved what
	// hunger took, to read it against: a rule that takes a hundredth of what
	// hunger does is a rule that will not move any of the world's numbers,
	// whatever else is true of it.
	Taken, Starved float64
}

// Weather reports what the weather came to. It writes nothing.
func (w *World) Weather() WeatherUse {
	out := WeatherUse{Taken: w.chillTaken, Starved: w.hungerTaken}
	if len(w.regions) == 0 {
		return out
	}
	cold, food, coldFood := 0.0, 0.0, 0.0
	for i := range w.regions {
		cold += w.regions[i].Weather[WeatherChill]
		food += w.regions[i].Food
		coldFood += w.regions[i].Weather[WeatherChill] * w.regions[i].Food
	}
	out.All = cold / float64(len(w.regions))
	if cold > 0 && food > 0 {
		// The food on the cold ground, as a share of an equal share: the mean
		// food weight of the regions, weighted by how cold each one is.
		out.ColdFood = (coldFood / cold) / (food / float64(len(w.regions)))
	}
	var humans, standing float64
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive || a.Species != SpeciesHuman {
			continue
		}
		humans++
		standing += w.weatherAt(a.X, a.Y, WeatherChill)
	}
	// Nobody alive is no evidence about where the living stand (stage 57's
	// note, which is stage 14's mistake not made a third time).
	if humans > 0 {
		out.OnCold = standing / humans
	} else {
		out.OnCold = out.All
	}
	out.Gain = out.OnCold - out.All
	return out
}
