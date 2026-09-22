package engine

// Kinds of enemy (stage 59).
//
// The world has two species and it is going to keep two: a kind is not a third
// species but a row in a table, in exactly the way stage 11 already said the
// difference between a human and an enemy is - the same node, the same
// decision engine, and a different range to draw its budget from.
//
// Adding a kind is meant to cost one entry in Config.EnemyKinds and nothing
// else. Every rule below reads the table rather than counting rows, an unset
// field falls back to the world's own figure, and the shares do not have to
// add up to anything in particular. Two kinds is what stages 60 to 64 are
// measured with; the machinery does not know that.
//
// What a kind must never become is a branch in the decision engine. If a rule
// ever wants to ask "which kind is this", the question belongs in a number on
// the row - what it eats, how far it strays, how much of a region's arrivals
// it takes - so that a new kind is a new row rather than a new case.

// EnemyKind is one sort of enemy: what it is built like and where it turns up.
type EnemyKind struct {
	// Name is for the viewer and for the experiment's tables. It changes
	// nothing.
	Name string

	// Share is how many of the world's arriving enemies are this kind,
	// relative to the other rows. They are normalised, so 1 and 3 means a
	// quarter and three quarters, and a row left at zero never arrives.
	Share float64

	// BudgetMean and BudgetStd are the range this kind's body is drawn from.
	// Zero means the world's own figure (EnemyBudgetMean, EnemyBudgetStd), so
	// a row that only wants to change where it lives says nothing about size.
	BudgetMean float64
	BudgetStd  float64

	// Homing is how much this kind follows the map's own weighting of where
	// enemies arrive (stage 58, region.Enemies). One is the full weighting,
	// zero is anywhere at all, and in between is a kind that prefers its
	// country without being tied to it.
	//
	// It is a blend rather than a second map: a kind does not get a weighting
	// of its own to draw, because the map's dangerous country is one place
	// and two kinds disagreeing about where it is would make it two.
	Homing float64

	// CanEatPlants is whether this sort can digest what the humans live on
	// (stage 60), and PlantAppetite what a plant is worth to it against a
	// mouthful of meat - one for a body that finds them as good, a small
	// figure for one that will only bother when it is starving. Unset is one.
	//
	// The pair is stage 25's division: what a body cannot do is a gate on the
	// candidates, and what is not worth doing is left to the comparison. There
	// is no coin flip anywhere - "it rarely bothers" has to come out of the
	// utility being low, so that a starving one still does.
	CanEatPlants  bool
	PlantAppetite float64

	// Water makes this sort a creature of the water (stage 63): it arrives in
	// a water cell rather than by the map's region weighting, and the water
	// does not drown it.
	//
	// Two meanings on one flag, and they are the same fact: a thing that
	// lives in the river is not at risk in the river. It says nothing about
	// what it eats - the fish of stages 42 and 43 are food items and this is
	// a body, and they do not meet (#95).
	//
	// A map with no water in it cannot hold one, and a row that asks for it
	// anyway arrives like any other sort rather than not arriving at all:
	// quietly dropping arrivals would change how many enemies the world has
	// without saying so.
	Water bool

	// Ward is the skill that knowing this sort of beast buys protection
	// against (stage 62), or SkillNone for a sort nobody has lore about.
	//
	// It is on the row rather than in the engine for the reason the rest of
	// this table is: which beasts are worth knowing about is a thing a map
	// says, and a second sort warded by the same lore is a second row
	// pointing at the same skill.
	Ward SkillKind

	// Key is the character that stands for this sort in Config.EnemyKindMap,
	// for a map that says which country a sort comes into the world in
	// (2026-09-20). Nought is a sort the map paints nowhere, and then it
	// arrives the way it always did - by Homing and the region weighting.
	//
	// It is the same arrangement RegionShape.Key and PlantKind.Key have, and
	// the same division of labour: the map brings the name and the place, the
	// row brings what the sort is like (decision #133).
	Key byte

	// Climbs and Flies are how this sort gets about, and what each of them
	// ignores about the ground is spelled out rather than bundled
	// (2026-09-20). A piece of ground does four separate things - it costs
	// (stage 20), it drowns (stage 34), it drags (stage 97) and it drains
	// (stage 99) - plus it stops a step between levels, and a flag that meant
	// "ignores water" would quietly mean all five. Water above is the one
	// that already exists and it says what it bundles and why.
	//
	// Climbs: level changes only. A climber steps up or down a cliff without
	// a ramp and pays the ground's price for everything else exactly as a
	// walker does - cost, drowning, drag and drain all unchanged.
	//
	// Flies: over the ground rather than on it. It steps between any levels,
	// runs no risk of drowning, is not dragged and is not drained - and pays
	// FlyCost a tick for the crossing instead of the ground's own figure.
	//
	// FlyCost is what flying costs, as a multiplier on ordinary open ground,
	// and unset is one. It is deliberately not zero: stage 20 says no ground
	// is impassable, and its mirror is that no ground is free. A flier that
	// crossed a gorge for nothing would make the gorge stop being country and
	// start being scenery, which is the line PLAN.md P16-3 asked to keep.
	Climbs  bool
	Flies   bool
	FlyCost float64

	// FlyHeight is how high this sort flies, in the levels the map's tiles
	// are drawn with (2026-09-20): it passes over ground up to that height
	// and is turned back by anything above it. Unset is no ceiling at all,
	// which is what the rule was when it was first written.
	//
	// The two halves live where they belong (decision #133). The map says
	// how high its country is - that is the tiles' own "height", 1 to 9,
	// which terrain.go has read since stage 20 - and the row says how high
	// the sort gets. Neither carries the other's number.
	//
	// A ceiling never makes a flier worse than a walker: where it is too low
	// to fly over something, it may still take the ground's own way round -
	// a ramp, a level step - exactly as anything else does. Flight is an
	// extra way through and never a lesser one, which is also what keeps
	// stage 20's line intact: no ground is impassable to everybody.
	FlyHeight int

	// Roam is how far from its nest this sort goes before EnemyHomeCost
	// starts charging it, in the world's own units (2026-09-20). Unset is
	// half the width of a region, which is what the rule was charged from
	// when the home was a block rather than a point.
	//
	// The radius is the whole of what the region was standing in for. Stage
	// 64 used a block because "the finest thing this world says about a place
	// is a region, and a body that wandered ten paces from where it was born
	// is not away from home" - a point on its own would have charged for the
	// first step. A point with a radius says that better, and says it in the
	// map author's own figures rather than in the region grid's.
	//
	// It is not a leash. Past the radius the cost rises with the distance and
	// nothing turns the body round, which is the line every stage has kept:
	// a good enough reason still takes it out of its country.
	Roam float64

	// Meat is what this sort's carcass is worth, as a multiplier on what a
	// body of its size would ordinarily leave (2026-09-20). Unset - which is
	// every world before this - is one, and then every sort leaves
	// Bulk / MeatPerBudget as it always did.
	//
	// It is on the row rather than worked out from the body because "what is
	// this thing made of" is a fact about the sort and not about its size: a
	// lean fast thing and a fat slow one of the same bulk are two rows, and
	// this is the field that lets them be.
	//
	// Counted before it was built (PLAN.md P16-3): meat is 0.16 of every
	// mouthful over 20,000 ticks and 0.30 over 200,000, and 0.67 to 0.80 of
	// what is dropped gets eaten. So the target is real and the meat is not
	// already going to waste - which is what makes this worth a field, where
	// the movement flags beside it are not yet (terrain does not confine
	// anybody today: bodies stand on high ground and in water in proportion
	// to how much of the map it is).
	Meat float64

	// Hide is the same for what it leaves that nobody eats (TODO 8): a
	// multiplier on Bulk / HidePerBudget, and unset is one.
	//
	// It is a second field rather than a share of Meat because the two are
	// facts about different things - how much of a beast is worth eating and
	// how much of it is worth working - and a sort that is all meat and no
	// skin is a row, not a special case.
	Hide float64

	// Homely is how much of EnemyHomeCost this kind pays (stage 64): one is
	// a sort that keeps to the country it came into the world in, zero one
	// that goes wherever it likes. An unset row is zero - a kind that was
	// never told to stay anywhere does not.
	//
	// This is the field the comment at the top of this file was written for:
	// "how far it strays" is a number on a row, so that a homebody and a
	// wanderer are two rows rather than two cases.
	Homely float64
}

// enemyKinds is the table as the world actually uses it: the configured rows,
// or one row standing for the enemy the world had before this stage.
//
// The single default row matters more than it looks. Everything below is
// written so that a world with one kind draws from the random source exactly
// as it did before kinds existed - no choice is made when there is nothing to
// choose between - which is what keeps every figure recorded before 2026-09-12
// readable.
func (w *World) enemyKinds() []EnemyKind {
	if len(w.cfg.EnemyKinds) > 0 {
		return w.cfg.EnemyKinds
	}
	return []EnemyKind{{
		Name:       "enemy",
		Share:      1,
		BudgetMean: w.cfg.EnemyBudgetMean,
		BudgetStd:  w.cfg.EnemyBudgetStd,
		Homing:     1,
		Homely:     1,
	}}
}

// pickEnemyKind draws which kind is arriving. It takes nothing from the random
// source when there is only one, which is the default world.
func (w *World) pickEnemyKind() int {
	kinds := w.enemyKinds()
	if len(kinds) <= 1 {
		return 0
	}
	// A sort whose every nest is full is not coming, so it is not drawn from
	// either: leaving it in would spend the arrival on a sort that has no
	// room and quietly make the cap a cap on the world instead of on a nest.
	// With no caps painted, share is what it always was and nothing here
	// counts a body.
	capped := w.nestsCapped()
	total := 0.0
	for i := range kinds {
		if capped && !w.kindHasRoom(i) {
			continue
		}
		total += max(kinds[i].Share, 0)
	}
	if total <= 0 {
		return 0
	}
	r := w.rng.Float64() * total
	for i := range kinds {
		if capped && !w.kindHasRoom(i) {
			continue
		}
		r -= max(kinds[i].Share, 0)
		if r <= 0 {
			return i
		}
	}
	return len(kinds) - 1
}

// kindOf is the row an agent belongs to, and the first row for anything else -
// humans included, which never read it.
func (w *World) kindOf(a *Agent) EnemyKind {
	kinds := w.enemyKinds()
	if int(a.Kind) >= len(kinds) {
		return kinds[0]
	}
	return kinds[a.Kind]
}

// budgetRangeFor is the range a founder of this species and kind is drawn
// from. An unset figure on the row falls back to the world's own.
func (w *World) budgetRangeFor(species Species, kind int) (mean, std float64) {
	if species != SpeciesEnemy {
		return w.cfg.GeneBudgetMean, w.cfg.GeneBudgetStd
	}
	kinds := w.enemyKinds()
	if kind < 0 || kind >= len(kinds) {
		kind = 0
	}
	mean, std = kinds[kind].BudgetMean, kinds[kind].BudgetStd
	if mean <= 0 {
		mean = w.cfg.EnemyBudgetMean
	}
	if std <= 0 {
		std = w.cfg.EnemyBudgetStd
	}
	return mean, std
}

// KindUse is what the table actually produced (stage 59).
//
// It is written to say something whatever the table's length is: a run with
// one kind reads one and nothing else, and a run with six needs no new
// columns. What it reports is whether the mix is the one the map asked for and
// whether the rows really differ.
type KindUse struct {
	// Kinds is how many rows the world is running.
	Kinds int

	// MixError is how far the arrivals are from the shares the table asked
	// for: the mean absolute difference over the rows. Zero is a world that
	// came out as written.
	//
	// It is about what the world puts in rather than what is standing about
	// later, because with a handful of enemies alive at any moment the
	// standing mix drifts far enough to say nothing about the table.
	MixError float64

	// Toughest over Weakest is how far apart the rows turned out in the
	// bodies they actually produced. One is a table whose rows are the same
	// creature under different names.
	BudgetGap float64

	// Homed is how much better the kinds ended up sitting in their own
	// country than the map's average - the same reading as Prowl.EnemyGain,
	// weighted by each kind's Homing, so that a table of stay-at-homes and a
	// table of wanderers do not average into nothing.
	Homed float64
}

// Kinds reports what the table produced. Read only.
func (w *World) Kinds() KindUse {
	kinds := w.enemyKinds()
	out := KindUse{Kinds: len(kinds), BudgetGap: 1}

	counts := make([]float64, len(kinds))
	budgets := make([]float64, len(kinds))
	var enemies float64
	var homed, homedWeight float64
	mean := 0.0
	if n := len(w.regions); n > 0 {
		mean = w.enemyWeight / float64(n)
	}
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive || a.Species != SpeciesEnemy {
			continue
		}
		k := int(a.Kind)
		if k >= len(kinds) {
			k = 0
		}
		counts[k]++
		budgets[k] += a.Budget()
		enemies++
		if h := kinds[k].Homing; h > 0 {
			homed += h * (w.prowlAt(a.X, a.Y) - mean)
			homedWeight += h
		}
	}
	if enemies == 0 {
		return out
	}

	wanted := 0.0
	for i := range kinds {
		wanted += max(kinds[i].Share, 0)
	}
	if wanted > 0 && w.enemyArrivals > 0 {
		for i := range kinds {
			arrived := 0.0
			if i < len(w.enemyArrivalsByKind) {
				arrived = w.enemyArrivalsByKind[i]
			}
			out.MixError += abs(arrived/w.enemyArrivals - max(kinds[i].Share, 0)/wanted)
		}
		out.MixError /= float64(len(kinds))
	}

	low, high := 0.0, 0.0
	for i := range kinds {
		if counts[i] == 0 {
			continue
		}
		b := budgets[i] / counts[i]
		if low == 0 || b < low {
			low = b
		}
		if b > high {
			high = b
		}
	}
	if low > 0 {
		out.BudgetGap = high / low
	}
	if homedWeight > 0 {
		out.Homed = homed / homedWeight
	}
	return out
}

// KindNameOf is which sort of enemy this is, for the viewer, and empty for a
// human or for a world with only one sort in it - where a name on every enemy
// would say nothing.
func (w *World) KindNameOf(id int) string {
	kinds := w.enemyKinds()
	if len(kinds) <= 1 {
		return ""
	}
	a := w.agentByID(id)
	if a == nil || a.Species != SpeciesEnemy {
		return ""
	}
	return w.kindOf(a).Name
}

// eatsPlantsFor says whether this body can digest what the humans live on
// (stage 60). For a human, always; for an enemy, what its row says.
//
// This is the same line stage 11 drew - what tells the species apart is the
// range it is drawn from and what it eats - generalised so that the second
// half can differ within a species too.
func (w *World) eatsPlantsFor(a *Agent) bool {
	if a.Species != SpeciesEnemy {
		return eatsPlants(a.Species)
	}
	return w.kindOf(a).CanEatPlants
}

// appetiteFor is what a plant is worth to this body against a mouthful of
// meat (stage 60). One for anything that lives on them.
func (w *World) appetiteFor(a *Agent, kind FoodKind) float64 {
	if kind != FoodPlant || a.Species != SpeciesEnemy {
		return 1
	}
	k := w.kindOf(a)
	if !k.CanEatPlants || k.PlantAppetite <= 0 {
		return 1
	}
	return clamp(k.PlantAppetite, 0, 1)
}

// speciesToll is how one species has been dying (stage 60).
type speciesToll struct {
	deaths  float64
	kills   float64
	starved float64
	byOther float64 // killed with the other species among its attackers
}

// speciesIndex is which slot a species keeps its tally in.
func speciesIndex(s Species) int {
	if s == SpeciesEnemy {
		return 1
	}
	return 0
}

// Feeding is what each species dies of and what the enemies are eating
// (stage 60).
//
// The world-wide killShare cannot answer this stage's question: a rule that
// puts the two species on the same food changes who kills whom, and that is
// invisible in a total that mixes both. Hence a tally per species, and the
// share of each one's violent deaths that the other species had a hand in.
type Feeding struct {
	HumanKillShare float64 // of the humans that died, how many were killed
	EnemyKillShare float64 // ... and of the enemies
	HumansByEnemy  float64 // of the humans killed, how many with an enemy among the attackers
	EnemiesByHuman float64 // ... and the other way round
	PlantsToEnemy  float64 // plants eaten by enemies, over the run

	// EnemyDeaths is how many of them died over the run, EnemyKills how many
	// of those were killed, and EnemyKillsByHuman how many of the kills had a
	// human among the attackers (2026-09-20).
	//
	// The three shares above cannot answer how much of anything a carcass
	// could supply: a share says what fraction of the dying was violent, and
	// what a material dropped by the dead would amount to is a count. It is
	// the supply side of TODO 8, counted before the material is built, in the
	// habit stage 45 set - a thing nobody can find is a rule that never
	// fires.
	EnemyDeaths       float64
	EnemyKills        float64
	EnemyKillsByHuman float64

	// EnemiesOnWater is how many of the enemies are standing in the water
	// (stage 63), and HumansOnWater the same for the humans - the figure
	// three stages running have failed to move, now that there is something
	// in the river that hunts.
	EnemiesOnWater float64
	HumansOnWater  float64

	// SeenByEnemy is the rule's own target: how many of the enemies have a
	// plant in sight at all. A rule that can only fire where a body can see
	// what it is now allowed to eat is capped by this (stage 45's habit of
	// counting the supply before building on it).
	SeenByEnemy float64
}

// Feeding reports it. Read only.
func (w *World) Feeding() Feeding {
	out := Feeding{PlantsToEnemy: w.plantsToEnemies}
	h, e := &w.tollOf[0], &w.tollOf[1]
	out.EnemyDeaths, out.EnemyKills, out.EnemyKillsByHuman = e.deaths, e.kills, e.byOther
	if h.deaths > 0 {
		out.HumanKillShare = h.kills / h.deaths
	}
	if e.deaths > 0 {
		out.EnemyKillShare = e.kills / e.deaths
	}
	if h.kills > 0 {
		out.HumansByEnemy = h.byOther / h.kills
	}
	if e.kills > 0 {
		out.EnemiesByHuman = e.byOther / e.kills
	}

	var enemies, withPlant, wetEnemies float64
	var humans, wetHumans float64
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		if a.Species != SpeciesEnemy {
			humans++
			if w.terrainAt(a.X, a.Y).Kind == GroundWater {
				wetHumans++
			}
			continue
		}
		enemies++
		if w.terrainAt(a.X, a.Y).Kind == GroundWater {
			wetEnemies++
		}
		for j := range w.foods {
			f := &w.foods[j]
			if f.Kind != FoodPlant {
				continue
			}
			if w.canSee(a.X, a.Y, f.X, f.Y) {
				withPlant++
				break
			}
		}
	}
	if enemies > 0 {
		out.SeenByEnemy = withPlant / enemies
		out.EnemiesOnWater = wetEnemies / enemies
	}
	if humans > 0 {
		out.HumansOnWater = wetHumans / humans
	}
	return out
}

// nestCell is one square the map painted for a sort, and what the map said
// about it: how many of the sort's arrivals come out of it, relative to its
// other nests, and how many bodies it keeps nearby before it sends no more.
//
// Both are a proportion rather than a figure (#133): rate is a share of this
// sort's arrivals and cap is a multiple of Config.NestCap. A world whose map
// painted neither has rate 1 and cap 0 everywhere, which is the world before
// 2026-09-22 down to the draw.
type nestCell struct {
	cell
	rate  float64
	cap   float64
	quiet float64
}

// buildEnemyKindCells reads Config.EnemyKindMap into one list of cells per
// sort, once, because Config does not change while a world runs. The rate and
// the cap are read off the same grid, cell for cell.
func (w *World) buildEnemyKindCells() {
	rows := w.cfg.EnemyKindMap
	kinds := w.cfg.EnemyKinds
	if len(rows) == 0 || len(kinds) == 0 {
		return
	}
	at := map[byte]int{}
	for i := range kinds {
		if k := kinds[i].Key; k != 0 && k != '.' {
			at[k] = i
		}
	}
	if len(at) == 0 {
		return
	}
	cells := make([][]nestCell, len(kinds))
	for r, row := range rows {
		h := w.cfg.Height / float64(len(rows))
		y := (float64(r) + 0.5) * h
		for c := 0; c < len(row); c++ {
			i, ok := at[row[c]]
			if !ok {
				continue
			}
			cw := w.cfg.Width / float64(len(row))
			cells[i] = append(cells[i], nestCell{
				cell: cell{x: (float64(c) + 0.5) * cw, y: y, w: cw, h: h},
				rate:  fifthsAt(w.cfg.NestRateMap, r, c, 1),
				cap:   w.cfg.NestCap * fifthsAt(w.cfg.NestCapMap, r, c, 1),
				quiet: fifthsAt(w.cfg.NestQuietMap, r, c, 1),
			})
		}
	}
	for i := range cells {
		if len(cells[i]) > 0 {
			w.enemyKindCells = cells
			return
		}
	}
}

// fifthsAt is what a painted grid says about one cell, as a multiplier: a
// digit is that many fifths, so '5' is the ordinary one. Anything the grid
// does not cover - an empty map, a short row, a character nobody knows - is
// the fallback, which is what keeps an unpainted world identical.
func fifthsAt(rows []string, r, c int, fallback float64) float64 {
	if r < 0 || r >= len(rows) {
		return fallback
	}
	row := rows[r]
	if c < 0 || c >= len(row) {
		return fallback
	}
	ch := row[c]
	if ch < '0' || ch > '9' {
		return fallback
	}
	return float64(ch-'0') / 5
}

// nestCrowd is how many living bodies of this sort are standing within its
// roam of this nest - what its cap is a cap on.
func (w *World) nestCrowd(kind int, c nestCell) float64 {
	roam := w.roamOf(kind)
	n := 0.0
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive || a.Species != SpeciesEnemy || int(a.Kind) != kind {
			continue
		}
		if distToCell(a.X, a.Y, c.cell) <= roam {
			n++
		}
	}
	return n
}

// nestHasRoom is whether this nest would send another one out.
//
// A cap of nought is a nest that holds nobody rather than a nest that holds
// everybody: "not from here" is a thing a map should be able to say, and
// whether caps are in play at all is Config.NestCap's business, not a cell's.
func (w *World) nestHasRoom(kind int, c nestCell) bool {
	if !w.nestsCapped() {
		return true
	}
	return w.nestCrowd(kind, c) < c.cap
}

// kindHasRoom is whether any nest of this sort would. A sort the map painted
// nowhere is not capped by anything, and neither is a world with no caps
// painted at all - both answer yes without counting anybody.
func (w *World) kindHasRoom(kind int) bool {
	if !w.nestsCapped() || kind < 0 || kind >= len(w.enemyKindCells) {
		return true
	}
	cells := w.enemyKindCells[kind]
	if len(cells) == 0 {
		return true
	}
	for _, c := range cells {
		if w.nestHasRoom(kind, c) {
			return true
		}
	}
	return false
}

// nestsCapped is whether this world caps its nests at all. It is the gate that
// keeps every world before this one untouched: with no cap set, nothing below
// counts a body or draws a value.
func (w *World) nestsCapped() bool { return w.cfg.NestCap > 0 }

// nestsHaveRoom is whether anywhere on the map would send another enemy out.
// True in a world with no caps, so the ordinary arrival is unchanged.
func (w *World) nestsHaveRoom() bool {
	if !w.nestsCapped() {
		return true
	}
	for kind := range w.enemyKindCells {
		if len(w.enemyKindCells[kind]) > 0 && w.kindHasRoom(kind) {
			return true
		}
	}
	return false
}

// paintedSpotFor draws where this sort comes into the world, when the map
// painted anywhere for it. The second return is false when it painted none,
// and then the arrival is the one it always was.
//
// Which of its nests it comes out of is the map's own weighting where there is
// one, and otherwise the plain draw over the cells the rule has had since the
// nests were painted - the same call, taking the same value out of the random
// source, so an unweighted map runs exactly as it did.
func (w *World) paintedSpotFor(kind int) (float64, float64, bool) {
	if kind < 0 || kind >= len(w.enemyKindCells) {
		return 0, 0, false
	}
	cells := w.enemyKindCells[kind]
	if len(cells) == 0 {
		return 0, 0, false
	}
	open := cells
	if w.nestsCapped() || w.bossesAt() {
		open = open[:0:0]
		for at, c := range cells {
			// A nest whose master has been killed sends nobody until it has
			// one again (TODO 19), and a full one sends nobody either.
			if w.nestQuiet(kind, at) {
				continue
			}
			if w.nestHasRoom(kind, c) {
				open = append(open, c)
			}
		}
		if len(open) == 0 {
			return 0, 0, false
		}
	}
	c, ok := w.drawNest(open)
	if !ok {
		return 0, 0, false
	}
	x := clamp(c.x+w.randRange(-c.w/2, c.w/2), 20, w.cfg.Width-20)
	y := clamp(c.y+w.randRange(-c.h/2, c.h/2), 20, w.cfg.Height-20)
	return x, y, true
}

// drawNest picks one of them, weighted by what the map painted. With nothing
// painted every nest weighs one and the draw is the plain one it always was.
func (w *World) drawNest(cells []nestCell) (nestCell, bool) {
	if len(w.cfg.NestRateMap) == 0 {
		return cells[w.rng.Intn(len(cells))], true
	}
	total := 0.0
	for _, c := range cells {
		total += max(c.rate, 0)
	}
	if total <= 0 {
		// Every nest here was painted '0'. The map has said this sort does
		// not come out of these, and a fallback to an even draw would be the
		// engine overruling it.
		return nestCell{}, false
	}
	r := w.rng.Float64() * total
	for _, c := range cells {
		r -= max(c.rate, 0)
		if r <= 0 {
			return c, true
		}
	}
	return cells[len(cells)-1], true
}

// NestView is one square a map painted for a sort of enemy: where it is, how
// big the cell is, and which row it belongs to. Read only, for whoever is
// drawing the world - the engine itself never asks.
type NestView struct {
	X, Y, W, H float64
	Kind       int
	Name       string

	// ID is what Rouse takes, and it is this nest's place in this very walk
	// (2026-09-22, TODO 19). Boss is the master that is out of it just now,
	// or nought, and Quiet how many ticks it has left with no master to call
	// - both nought in a world with no masters in it.
	ID    int
	Boss  int
	Quiet int
}

// EnemyNests is every square the map painted for every sort. Empty in a world
// whose map painted none, which is every world before 2026-09-20.
func (w *World) EnemyNests() []NestView {
	var out []NestView
	for kind, cells := range w.enemyKindCells {
		name := ""
		if kind < len(w.cfg.EnemyKinds) {
			name = w.cfg.EnemyKinds[kind].Name
		}
		for at, c := range cells {
			v := NestView{X: c.x, Y: c.y, W: c.w, H: c.h, Kind: kind, Name: name, ID: len(out)}
			if kind < len(w.nestLives) && at < len(w.nestLives[kind]) {
				life := w.nestLives[kind][at]
				if b := w.agentByID(life.boss); b != nil && b.Alive {
					v.Boss = life.boss
				}
				if left := life.quietTill - w.tick; left > 0 {
					v.Quiet = left
				}
			}
			out = append(out, v)
		}
	}
	return out
}
