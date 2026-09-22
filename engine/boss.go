package engine

import "fmt"

// The master of a nest (2026-09-22, TODO 19, decision #141).
//
// What it is. A nest may hold one body bigger than the rest of its sort. It is
// not in the world until somebody calls it out, it is an ordinary beast in
// every other way, and while it is gone - killed, or walked back in - the nest
// sends nobody out and cannot be called on again for a while.
//
// What it is not. There is no branch anywhere asking "is this the master": the
// difference is one number on the row (Config.BossBudget) and the decision
// engine has never been told. Nothing in here gates anything on a gene, a
// score or a body's history, and the reason an unkillable master cannot be
// written even by accident is that the world already caps a body: nine genes
// at MaxAbility is a budget of 900, which is 1.73 of the mean beast and 2.16
// of the mean body. The strongest thing this world can hold is a hard fight,
// never a wall.
//
// Who calls it out. Rouse, and only Rouse, from outside the engine - the shelf
// Endow and Inspire and SetTerrain are on. No rule calls it, no controller
// calls it, and a world nobody calls it in draws nothing for it and runs
// exactly as it would have. The click, the question and the yes are the game's
// (stage 19, #130).
//
// Why leaving it alone puts it back rather than leaving it standing: a master
// that can be called out and then ignored forever is a way of emptying a nest
// without fighting anything. It walks home because being away from home costs
// it (stage 64), and it goes back in when it is home and has been out long
// enough - a time and a place, never a state. "It flees below half vitality"
// would be the first behavioural threshold in this project.

// nestLife is what has become of one nest: whether its master is out, when it
// came out, and until when the nest is quiet.
//
// A nest with no master out and no quiet left is one that will answer Rouse.
type nestLife struct {
	boss      int // the body that is out, or nought
	rousedAt  int
	quietTill int
}

// bossesAt is whether this world has masters in it at all. Everything below
// answers no without touching anything when it does not.
func (w *World) bossesAt() bool { return w.cfg.BossBudget > 0 && len(w.enemyKindCells) > 0 }

// buildNestLives gives every painted nest its own state, once.
func (w *World) buildNestLives() {
	if len(w.enemyKindCells) == 0 {
		return
	}
	w.nestLives = make([][]nestLife, len(w.enemyKindCells))
	for kind, cells := range w.enemyKindCells {
		w.nestLives[kind] = make([]nestLife, len(cells))
	}
}

// nestAt finds one by the number EnemyNests gives it, which is its place in
// the walk over sorts and then cells.
func (w *World) nestAt(id int) (kind, at int, ok bool) {
	n := 0
	for kind, cells := range w.enemyKindCells {
		for at := range cells {
			if n == id {
				return kind, at, true
			}
			n++
		}
	}
	return 0, 0, false
}

// Rouse calls the master of this nest out into the world and returns the body
// it made. It is called from outside the engine and by nothing inside it.
//
// It refuses a nest that has no master to call - one whose master is already
// out, one still quiet after losing it, and any nest at all in a world that
// was not given masters.
func (w *World) Rouse(nest int) (int, error) {
	if w.cfg.BossBudget <= 0 {
		return 0, fmt.Errorf("this world has no masters in it (BossBudget is nought)")
	}
	kind, at, ok := w.nestAt(nest)
	if !ok {
		return 0, fmt.Errorf("no nest #%d", nest)
	}
	life := &w.nestLives[kind][at]
	if life.boss != 0 {
		if b := w.agentByID(life.boss); b != nil && b.Alive {
			return 0, fmt.Errorf("the master of nest #%d is already out", nest)
		}
	}
	if w.tick < life.quietTill {
		return 0, fmt.Errorf("nest #%d is empty for another %d ticks", nest, life.quietTill-w.tick)
	}
	a := w.masterOf(kind, w.enemyKindCells[kind][at])
	id := w.addAgent(a)
	life.boss, life.rousedAt = id, w.tick
	return id, nil
}

// masterOf builds the body. It is an arrival of that sort, standing on its own
// nest, with the budget its row would have given it multiplied by
// Config.BossBudget - and the world's own ceiling applies to it as to anybody,
// which is what keeps it a fight rather than a wall.
func (w *World) masterOf(kind int, c nestCell) Agent {
	a := w.newAgent(
		clamp(c.x, 20, w.cfg.Width-20),
		clamp(c.y, 20, w.cfg.Height-20),
		w.randomSex(),
		w.drawGenomeOf(SpeciesEnemy, kind),
		0,
		1, // grown, like anything the world puts in from outside
	)
	a.Species = SpeciesEnemy
	a.Kind = uint8(kind)
	a.lore = w.newLore()
	a.chronotype = w.drawChronotype()
	a.taste = w.drawTaste()
	a.fancy = w.drawFancy(&a)
	a.adornWant = 1
	a.hintSlots = w.drawHintSlots()
	a.hints = w.drawHints(a.hintSlots)
	a.lessonSlots = w.drawLessonSlots()
	w.learnFromBirthplace(&a)
	fitBudget(a.Genome, (a.Budget()-w.hintCost(a.hintSlots)-w.lessonCost(a.lessonSlots))*w.cfg.BossBudget)
	a.Vitality = a.MaxVitality(&w.cfg)
	a.Hunger = w.randRange(0, w.cfg.SatiatedHunger)
	a.Lifespan = w.cfg.MaxLifespan
	return a
}

// nestsOfTick settles what became of the masters that are out: one that has
// fallen leaves its nest quiet, and one that is home again and has been out
// long enough goes back in.
//
// It runs before the dead are compacted away, so a master killed this tick is
// still here to be found.
func (w *World) nestsOfTick() {
	if !w.bossesAt() {
		return
	}
	for kind := range w.nestLives {
		for at := range w.nestLives[kind] {
			life := &w.nestLives[kind][at]
			if life.boss == 0 {
				continue
			}
			b := w.agentByID(life.boss)
			if b == nil || !b.Alive {
				// Whatever took it - a spear, hunger, the years - the nest
				// has lost its master. Asking who would be a branch on how a
				// body died, and the thing that rewards the one who brought
				// it down is the carcass, which already only appears where
				// something did (MeatFromKills).
				life.boss = 0
				life.quietTill = w.tick + w.quietTicks(w.enemyKindCells[kind][at])
				continue
			}
			if w.tick-life.rousedAt < w.cfg.BossRouseTicks {
				continue
			}
			if distToCell(b.X, b.Y, w.enemyKindCells[kind][at].cell) > 0 {
				continue
			}
			w.retire(b)
			life.boss = 0
		}
	}
}

// quietTicks is how long this nest stays empty after losing its master: the
// world's own figure in years, times what the map painted for this cell.
func (w *World) quietTicks(c nestCell) int {
	years := w.cfg.NestQuietYears * c.quiet
	return max(int(years*float64(w.cfg.TicksPerYear)), 1)
}

// retire takes a body out of the world without killing it.
//
// It is the mirror of Rouse and it is the only way out of the world that is
// not death: no cause is recorded, no carcass is left, no witness learns
// anything, and the counters do not move. What it does do is put down whatever
// the body was holding, because food carried out of the world would be a leak
// in a total that has been closed since stage 15a.
func (w *World) retire(a *Agent) {
	w.dropCarried(a)
	a.Alive = false
	w.retired++
}

// nestQuiet is whether this nest is sending nobody out just now.
func (w *World) nestQuiet(kind, at int) bool {
	if kind < 0 || kind >= len(w.nestLives) || at < 0 || at >= len(w.nestLives[kind]) {
		return false
	}
	return w.tick < w.nestLives[kind][at].quietTill
}

// BossOf is the body that is out of this nest just now, or nought. Read only,
// for whoever is drawing the world.
func (w *World) BossOf(nest int) int {
	kind, at, ok := w.nestAt(nest)
	if !ok || len(w.nestLives) == 0 {
		return 0
	}
	id := w.nestLives[kind][at].boss
	if b := w.agentByID(id); b == nil || !b.Alive {
		return 0
	}
	return id
}
