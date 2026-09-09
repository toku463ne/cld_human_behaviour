package engine

import "fmt"

// Changing a world from outside it (stage 22).
//
// Everything here is on the same footing as Endow: the simulation never calls
// any of it. These are the hands of whoever is laying a world out - an
// administrator with an editor, or a test - and the world itself has no way to
// reshape its own ground, redraw its own regions or retune its own rules.
//
// Three things they deliberately do not do.
//
//   - They do not touch the population. Changing the country under somebody's
//     feet is a change to the world; moving them is a different kind of act,
//     and Repopulate (save.go) is where that lives.
//   - They do not draw anything from the random source. An editor must not
//     change what the world would have done next, and a world laid out with
//     the editor is meant to run exactly as one laid out in a config file.
//   - They do not re-apply the world's building rules. The tie between the
//     ground and where the plants grow (stage 33) is applied when a world is
//     built; edit the map afterwards and the going changes while the crop
//     stays where the administrator put it, because after the first edit the
//     regions are the administrator's to say and not the rule's to overwrite.

// SetTerrain lays a new piece of country under the world. The rows are the
// same map a Config carries (terrain.go), and an empty one makes the world
// flat again.
//
// Positions are left alone. A body standing where a cliff has just appeared is
// standing on top of it, not inside it: the step rule only ever refuses a
// move, so the worst that a careless edit does is strand somebody, which is a
// thing an administrator can see and fix.
func (w *World) SetTerrain(rows []string) {
	w.cfg.TerrainMap = append([]string(nil), rows...)
	w.ground = buildTerrain(&w.cfg)
	w.water = waterCells(w.ground)
}

// Terrain is the map as it stands, for an editor to draw and change. The copy
// is the caller's.
func (w *World) Terrain() []string {
	return append([]string(nil), w.cfg.TerrainMap...)
}

// SetRegion says what one block of the world provides: how sheltered resting
// in it is and how well it grows plants, both relative to ordinary ground.
//
// The plant figure is a share of the world's growth rather than an amount, so
// raising one region lowers everybody else's share of the same total. That is
// stage 15a's rule and it holds here: an editor can move the food about and
// cannot conjure any.
// Inspire is the genius event, done by hand: the one thing an administrator
// can give a node that is not the ground under it (stage 22's leftover).
//
// It does exactly what a genius birth does to what a body has learned, and
// nothing else. One more room for an idea, if the body is under the world's
// cap - paid for with new budget, the way Endow pays, so that nothing is
// quietly taken out of the genes to fund it - and a leap at whatever skill the
// body already has. It cannot invent a skill nobody in the world has needed,
// for the same reason a genius birth cannot: a leap goes further at a thing,
// it does not conjure the thing.
//
// It draws no random number, which is the whole reason it is written this way.
// A genius birth picks a new idea out of the air, and an editor that did the
// same would change what the world was going to do next; the empty room this
// leaves is the deterministic half of the same event, and somebody else's idea
// is what fills it (exchangeHints).
//
// It returns the budget it added, so an interface can say that this was not
// the world's doing.
func (w *World) Inspire(id int) (added float64, err error) {
	a := w.agentByID(id)
	if a == nil || !a.Alive {
		return 0, fmt.Errorf("no such node: %d", id)
	}
	if a.hintSlots < w.cfg.HintSlots {
		a.hintSlots++
		added = w.hintCost(1)
		if added > 0 {
			// New budget rather than a redistribution: an administrator's
			// gift must not make the body quietly worse at something it was
			// never asked about (endow.go).
			fitBudget(a.Genome, a.Budget()+added)
		}
	}
	best, kind := 0.0, SkillNone
	for i := range a.hints {
		if h := &a.hints[i]; h.Skill != SkillNone && h.Mastery >= best {
			best, kind = h.Mastery, h.Skill
		}
	}
	if kind != SkillNone && w.cfg.SkillGeniusJump > 0 {
		if w.learnSkill(a, kind, best+w.cfg.SkillGeniusJump) {
			w.skillsLeapt++
		}
	}
	if added == 0 && kind == SkillNone {
		return 0, fmt.Errorf("node %d has all the room it can have and nothing to be a genius at", id)
	}
	return added, nil
}

func (w *World) SetRegion(i int, shelter, food float64) error {
	if i < 0 || i >= len(w.regions) {
		return fmt.Errorf("there is no region %d (the world has %d)", i, len(w.regions))
	}
	w.regions[i].Shelter = clamp(shelter, 0, 2)
	w.regions[i].Food = clamp(food, 0, 2)
	w.foodWeight = 0
	for j := range w.regions {
		w.foodWeight += w.regions[j].Food
	}
	return nil
}

// RegionAt is which block a position falls in, for an editor pointing at one.
func (w *World) RegionAt(x, y float64) int { return w.regionIndexAt(x, y) }

// Tunable names a rule an interface may change while a world is running.
//
// The list is short on purpose (#50). Every figure in Config could be put on a
// screen and most of them should not be: a few of them move the world from
// "alive" to "empty" within a few thousand ticks, and the ones here are the
// ones worth reaching for, each with the range it has actually been measured
// in. Anything outside that range is allowed and reported as outside it -
// an administrator may want a world that does not work, and should be told
// that is what they are making.
type Tunable int

const (
	TuneFoodSpawnRate Tunable = iota
	TuneBudgetMean
	TuneBudgetSpread
	TuneCompetitionWeight
	TuneSkirmishTicks
	TuneMutationRate
	numTunables
)

// tunables is the whole of what an interface may reach: what it is called,
// what it does, and the range anybody has measured it in.
var tunables = [numTunables]struct {
	Name     string
	About    string
	Low, Hig float64
}{
	TuneFoodSpawnRate:     {"food", "plants appearing per tick: the world's whole supply", 0.12, 0.30},
	TuneBudgetMean:        {"budget", "what a body has to spend on itself: the difficulty dial", 300, 430},
	TuneBudgetSpread:      {"budget spread", "how unlike each other the founders are", 15, 60},
	TuneCompetitionWeight: {"rivalry", "what removing a rival for food is worth", 0.05, 0.25},
	TuneSkirmishTicks:     {"skirmish", "how long a fight is reckoned to last", 20, 80},
	TuneMutationRate:      {"mutation", "share of genes that jump at a birth", 0, 0.05},
}

// TuneInfo is one rule an interface may offer, and where it stands.
type TuneInfo struct {
	Name     string
	About    string
	Value    float64
	Low, Hig float64
	Safe     bool // whether the value is inside the measured range
}

// Tunables reports the short list and where each one stands. Read only.
func (w *World) Tunables() []TuneInfo {
	out := make([]TuneInfo, 0, numTunables)
	for t := Tunable(0); t < numTunables; t++ {
		v := w.tuned(t)
		out = append(out, TuneInfo{
			Name: tunables[t].Name, About: tunables[t].About, Value: v,
			Low: tunables[t].Low, Hig: tunables[t].Hig,
			Safe: v >= tunables[t].Low && v <= tunables[t].Hig,
		})
	}
	return out
}

func (w *World) tuned(t Tunable) float64 {
	switch t {
	case TuneFoodSpawnRate:
		return w.cfg.FoodSpawnRate
	case TuneBudgetMean:
		return w.cfg.GeneBudgetMean
	case TuneBudgetSpread:
		return w.cfg.GeneBudgetStd
	case TuneCompetitionWeight:
		return w.cfg.CompetitionWeight
	case TuneSkirmishTicks:
		return w.cfg.SkirmishTicks
	case TuneMutationRate:
		return w.cfg.MutationRate
	}
	return 0
}

// Tune changes one of them, and reports whether the new value is inside the
// range it has been measured in. A value outside is set anyway: the warning is
// the point, not a refusal.
func (w *World) Tune(t Tunable, v float64) (safe bool, err error) {
	if t < 0 || t >= numTunables {
		return false, fmt.Errorf("there is no such rule to change")
	}
	if v < 0 {
		v = 0
	}
	switch t {
	case TuneFoodSpawnRate:
		w.cfg.FoodSpawnRate = v
	case TuneBudgetMean:
		w.cfg.GeneBudgetMean = v
	case TuneBudgetSpread:
		w.cfg.GeneBudgetStd = v
	case TuneCompetitionWeight:
		w.cfg.CompetitionWeight = v
	case TuneSkirmishTicks:
		w.cfg.SkirmishTicks = v
	case TuneMutationRate:
		w.cfg.MutationRate = v
	}
	return v >= tunables[t].Low && v <= tunables[t].Hig, nil
}

// Difficulty is a named set of the figures a game hands a player, kept apart
// from the ones measurements are taken at (#50).
//
// It is only the budget. A harder world is one where a body has less to spend
// on itself, which is the dial the plan chose precisely because it does not
// touch the rules the measurements are about: the world works the same way, the
// bodies in it are simply poorer or richer.
type Difficulty struct {
	Name       string
	Budget     float64
	Spread     float64
	AboutInOne string
}

// Difficulties are the presets an interface may offer. The middle one is the
// measured default, so a game played on it is a game played in the world every
// number in HISTORY.md was taken from.
var Difficulties = []Difficulty{
	{"kind", 430, 45, "bodies with about a fifth more to spend than the measured world"},
	{"measured", 360, 30, "exactly the world every number in HISTORY.md was taken in"},
	{"hard", 300, 20, "less to go round, and the founders more alike"},
}

// SetDifficulty applies one by name, and reports whether it was known.
func (w *World) SetDifficulty(name string) bool {
	for _, d := range Difficulties {
		if d.Name != name {
			continue
		}
		w.cfg.GeneBudgetMean, w.cfg.GeneBudgetStd = d.Budget, d.Spread
		return true
	}
	return false
}
