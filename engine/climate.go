package engine

import "math"

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
// Where it lives: on a map of its own, at whatever grain its author drew it
// (2026-09-20, #135). It was on the region until then, for a reason that did
// not survive being measured - that everything this world learns and passes on
// about a place is on the region - because stage 86 found that nothing learns
// or passes on the weather at all: the three beliefs a body keeps about a
// region are how rich it is, how hard it is to cross and what killed somebody
// there, and the weather is none of them. What broke the old arrangement was
// the region count becoming a difficulty dial: a first stage with three
// regions in it could hold three temperatures.
//
// And a grain of its own is what lets the weather be read the way the ground
// is. Stage 100 reads the cell one step ahead of a body and is the only rule
// in this project that moved where anybody lives; stage 86's cold moved
// nobody, and named its own reason - what a body reads is the cold underfoot,
// which lifts every candidate alike and cancels out of the comparison. A
// weather that changes from one cell to the next does not cancel.
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

	// WeatherHeat is the second one (stage 88), and the proof that adding a
	// kind is a line here: nothing counts them, the map reads a character for
	// it, one figure says what standing in it costs, and whatever answers it
	// says so on itself.
	WeatherHeat

	// NumWeathers is how many there are. No rule counts them; it is the size
	// of the array on a region.
	NumWeathers
)

// climateGrid is the weather laid over the world: cells of equal size, at the
// grain of the picture its author drew (#135).
//
// The size of a cell comes from the picture, the way the terrain's does, so
// the same file describes the same country whatever the world's dimensions
// are - and so that an author can draw the weather finely over a world cut
// into three regions, which is the whole point of taking it off the region.
type climateGrid struct {
	cols, rows   int
	cellW, cellH float64
	cells        [][NumWeathers]float64

	// shift is how far round the year the picture has been slid, in columns
	// (stage 87b). It is a whole number of cells rather than a fraction so
	// that a season is exactly reproducible from the tick.
	shift int
}

// at is how much of one weather this spot has. Nought everywhere in a world
// that was given no picture, and nought off the top and bottom of one.
func (g *climateGrid) at(x, y float64, kind Weather) float64 {
	if g == nil || g.cols == 0 || g.rows == 0 {
		return 0
	}
	cy := int(math.Floor(y / g.cellH))
	if cy < 0 || cy >= g.rows {
		return 0
	}
	// Round the world rather than off its edge, so that the season slides the
	// picture without leaving a seam behind it.
	cx := int(math.Floor(x / g.cellW))
	cx = ((cx+g.shift)%g.cols + g.cols) % g.cols
	return g.cells[cy*g.cols+cx][kind]
}

// makeClimate reads the map's picture of its weather into a grid. Each string
// is a row and each rune a cell, the way the terrain map is written:
//
//	.       the ordinary world
//	1 - 9   that much cold
//	a - i   that much heat (stage 88)
//
// A world that says nothing about its weather gets none, which is the default,
// and nothing here draws a random number - the picture is the author's.
func makeClimate(cfg *Config) *climateGrid {
	rows := cfg.ClimateMap
	if len(rows) == 0 {
		return nil
	}
	cols := 0
	for _, r := range rows {
		if n := len([]rune(r)); n > cols {
			cols = n
		}
	}
	if cols == 0 {
		return nil
	}
	g := &climateGrid{
		cols: cols, rows: len(rows),
		cellW: cfg.Width / float64(cols), cellH: cfg.Height / float64(len(rows)),
		cells: make([][NumWeathers]float64, cols*len(rows)),
	}
	for y, row := range rows {
		runes := []rune(row)
		for x := 0; x < cols; x++ {
			c := '.'
			if x < len(runes) {
				c = runes[x]
			}
			switch {
			case c >= '1' && c <= '9':
				g.cells[y*cols+x][WeatherChill] = float64(c-'0') / 9
			case c >= 'a' && c <= 'i':
				// The letters are the second weather (stage 88), written the
				// way the terrain map writes its slopes.
				g.cells[y*cols+x][WeatherHeat] = float64(c-'a'+1) / 9
			}
		}
	}
	return g
}

// buildClimate lays the picture over the world, at whatever phase of the year
// the clock is at.
func (w *World) buildClimate() {
	w.climate = makeClimate(&w.cfg)
	w.layClimate(w.seasonPhase())
}

// seasonPhase is how far round the year the weather has turned, from 0 to 1
// (stage 87b). A world with no season is always at nought, which is the same
// picture for ever and therefore no season at all.
//
// It rides the calendar this world already keeps (TicksPerYear), because the
// rest of a body - how long it takes to grow, how long it lives - is measured
// in those years, and a season that did not line up with them would be a
// second clock saying nearly the same thing (#38's rule about second maps,
// applied to time).
func (w *World) seasonPhase() float64 {
	if w.cfg.SeasonTicks <= 0 {
		return 0
	}
	return float64(w.tick%w.cfg.SeasonTicks) / float64(w.cfg.SeasonTicks)
}

// turnSeason moves the weather round, once a tick and before anything reads
// it. It draws nothing: the picture is the author's and the phase is the
// clock's, so a season is as reproducible as the map it turns.
func (w *World) turnSeason() {
	if w.cfg.SeasonTicks <= 0 || w.climate == nil {
		return
	}
	w.layClimate(w.seasonPhase())
}

// layClimate slides the picture to one phase of the year. At phase nought it
// is the picture as drawn; further round, the same picture moved sideways - so
// what was the cold end of the world becomes the warm one and back, and a body
// that has not moved finds the weather has.
//
// Sliding it rather than deepening it is the whole point (#117): a winter that
// only got colder everywhere would make everybody want a coat at once, which
// is one demand and no trade. A weather that moves makes the demand somewhere
// the supply is not, over and over, which is the one structure this world has
// never had (#115).
func (w *World) layClimate(phase float64) {
	if w.climate == nil {
		return
	}
	w.climate.shift = int(phase * float64(w.climate.cols))
}

// weatherAt is how much of one weather this spot has, and nought everywhere in
// a world that was given none.
func (w *World) weatherAt(x, y float64, kind Weather) float64 {
	return w.climate.at(x, y, kind)
}

// drainFor is what the worst of one weather costs a body per tick. One line
// per kind (stage 88), which is the whole of what adding a weather costs.
func (w *World) drainFor(kind Weather) float64 {
	switch kind {
	case WeatherChill:
		return w.cfg.ChillDrain
	case WeatherHeat:
		return w.cfg.HeatDrain
	}
	return 0
}

// chillOf is what standing here costs this body in vitality per tick, over
// every weather there is.
//
// It is the one place the weather is turned into a number, so it is also the
// one place what a body carries is subtracted - and one thing answers one
// weather, so a body in a cold place with a sunshade pays the whole of the
// cold (stage 88).
func (w *World) chillOf(a *Agent) float64 {
	return w.weatherTax(a, true)
}

// weatherTax is that sum, with or without what the body is carrying.
func (w *World) weatherTax(a *Agent, warded bool) float64 {
	return w.weatherTaxAt(a, a.X, a.Y, warded)
}

// weatherTaxAt is the same sum at a place the body is not standing in, which
// is what reading the weather one cell ahead needs (#135). What the body
// carries goes with it, so the warding is the same wherever it is asked about.
func (w *World) weatherTaxAt(a *Agent, x, y float64, warded bool) float64 {
	total := 0.0
	for kind := Weather(0); kind < NumWeathers; kind++ {
		drain := w.drainFor(kind)
		if drain <= 0 {
			continue
		}
		here := w.weatherAt(x, y, kind)
		if here <= 0 {
			continue
		}
		off := 0.0
		if warded {
			off = clamp(w.wardsOff(a, kind), 0, 1)
		}
		total += drain * here * (1 - off)
	}
	return total
}

// wardValue is what one piece would take off this body's drain where it is
// standing: nought for a piece that answers a weather this place does not
// have, and nought for one no better than what the body wards already
// (stage 88). It is what an option needs to know and the only thing it needs.
func (w *World) wardValue(a *Agent, f *Food) float64 {
	if f.Ward <= 0 {
		return 0
	}
	drain := w.drainFor(f.Wards)
	here := w.weatherAt(a.X, a.Y, f.Wards)
	if drain <= 0 || here <= 0 {
		return 0
	}
	// What it adds to the rest of this hand. For something out in the world
	// that is everything this body wards; for something already in the hand
	// it is that piece's own worth, which is what parting with it would cost.
	skip := -1
	for i := range a.carried {
		if &a.carried[i] == f {
			skip = i
			break
		}
	}
	gain := clamp(f.Ward, 0, 1) - clamp(w.wardsOffBut(a, f.Wards, skip), 0, 1)
	if gain <= 0 {
		return 0
	}
	return drain * here * gain
}

// wardsAnything says whether this body is carrying something that answers any
// weather at all.
func (w *World) wardsAnything(a *Agent) bool {
	for i := range a.carried {
		if a.carried[i].Ward > 0 {
			return true
		}
	}
	return false
}

// wardOf is what one piece keeps off one weather.
func wardOf(f *Food, kind Weather) float64 {
	if f.Wards != kind {
		return 0
	}
	return f.Ward
}

// chillRawFelt is what the weather here would take before anything in a hand
// keeps it off: the figure an option needs to price a piece that would ward
// more than this body wards already.
func (w *World) chillRawFelt(a *Agent) float64 {
	if !w.cfg.ChillKnown {
		return 0
	}
	return w.weatherTax(a, false)
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

// chillAheadFelt is what the weather one cell away would take from this body,
// as it reads it (#135). It is the weather's half of stage 100: the ground one
// step ahead is read for what crossing it costs and what standing on it takes,
// and this is the same question asked of the map the weather is on.
//
// Nought where the body cannot feel the weather at all (stage 86's control),
// because a body that cannot feel the cold it is standing in cannot read the
// cold it is walking into either.
func (w *World) chillAheadFelt(a *Agent, x, y float64) float64 {
	if !w.cfg.ChillKnown {
		return 0
	}
	return w.weatherTaxAt(a, x, y, true)
}

// chillSlope is which way it gets colder from here, and by how much per unit
// of distance, in the same vitality-per-tick the chill itself is in
// (stage 86b).
//
// It is read one cell of the weather's own map away in each direction, which
// is the grain the weather has (#135; until then it was a region's width,
// because that was). A step finer than the picture reads the same cell twice
// and says the world is flat. Nothing is remembered and nothing is told - this
// is what a body feels where it stands, and it is gone the moment it moves.
func (w *World) chillSlope(a *Agent) (dx, dy float64) {
	if !w.cfg.ChillGradient || !w.cfg.ChillKnown || w.climate == nil {
		return 0, 0
	}
	spanX, spanY := w.climate.cellW, w.climate.cellH
	if spanX <= 0 || spanY <= 0 {
		return 0, 0
	}
	at := func(x, y float64) float64 {
		x, y = clamp(x, 0, w.cfg.Width-1e-9), clamp(y, 0, w.cfg.Height-1e-9)
		total := 0.0
		for kind := Weather(0); kind < NumWeathers; kind++ {
			total += w.drainFor(kind) * w.weatherAt(x, y, kind)
		}
		return total
	}
	dx = (at(a.X+spanX, a.Y) - at(a.X-spanX, a.Y)) / (2 * spanX)
	dy = (at(a.X, a.Y+spanY) - at(a.X, a.Y-spanY)) / (2 * spanY)
	return dx, dy
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
	return w.wardsOffBut(a, kind, -1)
}

// wardsOffBut is the same, ignoring one thing in the hand.
//
// It is what lets a body price the coat it is wearing (stage 88's correction):
// asked what one more would be worth, the answer is what it adds to the best
// it already has - but asked what parting with this one would cost, the answer
// is what it adds to the best of the rest. Without the second reading a body
// prices its own coat at nothing and hands it over for nothing, which is what
// stage 87a measured before this was found.
func (w *World) wardsOffBut(a *Agent, kind Weather, skip int) float64 {
	best := 0.0
	for i := range a.carried {
		if i == skip {
			continue
		}
		f := &a.carried[i]
		if f.Wards == kind && f.Ward > best {
			best = f.Ward
		}
	}
	return best
}

// wardMade is what a piece this body has just made keeps off, and which
// weather it answers (stage 87a).
//
// A share of them come out answering the weather rather than all of them, so
// that making one is not the same thing as making a coat: a maker cannot aim,
// which is the same hand the style is dealt by (stage 84) and the reason a
// body that wants one cannot simply sit down and produce it.
func (w *World) wardMade() (float64, Weather) {
	if w.cfg.WardShare <= 0 || w.cfg.WardStrength <= 0 {
		return 0, WeatherChill
	}
	if w.rng.Float64() >= clamp(w.cfg.WardShare, 0, 1) {
		return 0, WeatherChill
	}
	// And which weather it answers, where the world has more than one
	// (stage 88). Drawn rather than chosen, for the reason the style is: a
	// maker that could aim would have no reason to want anybody else's.
	kind := WeatherChill
	kinds := 0
	for k := Weather(0); k < NumWeathers; k++ {
		if w.drainFor(k) > 0 {
			kinds++
		}
	}
	if kinds > 1 {
		n := int(w.rng.Float64() * float64(NumWeathers))
		kind = Weather(clampInt(n, 0, int(NumWeathers)-1))
	}
	return clamp(w.cfg.WardStrength, 0, 1), kind
}

// regionChill is how cold each region is on average, and how cold the world is
// (#135). Every cell of the weather's map counts once, towards the region its
// middle falls in.
//
// No rule reads either figure. It is here because the food weights are per
// region and something has to be the same shape as them to be read against,
// and because an editor that shows a region wants one number for it. What a
// body pays is read off the cell it is standing in and never off this.
func (w *World) regionChill() (means []float64, all float64) {
	means = make([]float64, len(w.regions))
	if w.climate == nil || len(w.regions) == 0 {
		return means, 0
	}
	count := make([]float64, len(w.regions))
	cells := 0.0
	for cy := 0; cy < w.climate.rows; cy++ {
		for cx := 0; cx < w.climate.cols; cx++ {
			x := (float64(cx) + 0.5) * w.climate.cellW
			y := (float64(cy) + 0.5) * w.climate.cellH
			c := w.climate.at(x, y, WeatherChill)
			all += c
			cells++
			i := w.regionIndexAt(x, y)
			means[i] += c
			count[i]++
		}
	}
	for i := range means {
		if count[i] > 0 {
			means[i] /= count[i]
		}
	}
	if cells > 0 {
		all /= cells
	}
	return means, all
}

// --- reading it out ---------------------------------------------------------

// ClimateSize is the shape of the weather's map, for a viewer to draw it by.
// All zeroes in a world that was given no weather.
func (w *World) ClimateSize() (cols, rows int, cellW, cellH float64) {
	if w.climate == nil {
		return 0, 0, 0, 0
	}
	return w.climate.cols, w.climate.rows, w.climate.cellW, w.climate.cellH
}

// ClimateAt is how much of each weather this spot has, as a share of the worst
// in the world. Read only, and nought everywhere in a world with no weather.
//
// It is the share and not what it costs, for the reason the map is drawn that
// way (decision #133): the dose lives in Config and the picture does not, so
// the same drawing is a mild winter or a killing one depending on the world.
func (w *World) ClimateAt(x, y float64) (chill, heat float64) {
	return w.weatherAt(x, y, WeatherChill), w.weatherAt(x, y, WeatherHeat)
}

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

	// Coats is how many warding pieces have been made, Wearing the share of
	// living bodies that are holding one, and Handed how many changed hands
	// (stage 87a). The last is the stage's own question: a coat is worth
	// everything in the cold and nothing in the warm, so whether that
	// asymmetry moves any of them is the whole of what there is to see.
	Coats, Handed int
	Wearing       float64

	// Spare is the share of the warding pieces in hands that are worth
	// nothing to whoever is carrying them (stages 87b, 88): the wrong weather
	// here, no weather here, or one no better than the one already worn. It
	// is the stock of merchandise - a thing worth everything to somebody else
	// and nothing to its holder.
	Spare float64

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
	means, all := w.regionChill()
	out.All = all
	cold, food, coldFood := 0.0, 0.0, 0.0
	for i := range w.regions {
		c := means[i]
		cold += c
		food += w.regions[i].Food
		coldFood += c * w.regions[i].Food
	}
	if cold > 0 && food > 0 {
		// The food on the cold ground, as a share of an equal share: the mean
		// food weight of the regions, weighted by how cold each one is.
		out.ColdFood = (coldFood / cold) / (food / float64(len(w.regions)))
	}
	var humans, standing, wearing, worn, spare float64
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive || a.Species != SpeciesHuman {
			continue
		}
		humans++
		standing += w.weatherAt(a.X, a.Y, WeatherChill)
		for k := range a.carried {
			f := &a.carried[k]
			if f.Ward <= 0 {
				continue
			}
			worn++
			if w.wardValue(a, f) <= 0 {
				spare++
			}
		}
		if w.wardsAnything(a) {
			wearing++
		}
	}
	// Nobody alive is no evidence about where the living stand (stage 57's
	// note, which is stage 14's mistake not made a third time).
	if humans > 0 {
		out.OnCold = standing / humans
	} else {
		out.OnCold = out.All
	}
	if humans > 0 {
		out.Wearing = wearing / humans
	}
	if worn > 0 {
		out.Spare = spare / worn
	}
	out.Coats, out.Handed = w.coatsMade, w.coatsHanded
	out.Gain = out.OnCold - out.All
	return out
}
