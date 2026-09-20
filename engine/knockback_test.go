package engine

import (
	"math"
	"testing"
)

// A world with two bodies at arm's length and nothing else happening, so that
// what moves is the blow and not the day.
func knockWorld(t *testing.T, tune func(*Config)) (*World, *Agent, *Agent) {
	t.Helper()
	cfg := DefaultConfig()
	cfg.InitialPopulation, cfg.InitialFoodItems = 0, 0
	cfg.FoodSpawnRate = 0
	cfg.EnemySpawnTicks = 0
	cfg.MutationStd, cfg.MutationRate = 0, 0
	cfg.EvasionCap, cfg.DefenceCap = 0, 0 // the blow always lands, whole
	cfg.OpeningTicks = 0
	cfg.KnockbackDist = 6
	if tune != nil {
		tune(&cfg)
	}
	w := NewWorld(cfg)

	mid := make([]float64, NumGenes)
	for i := range mid {
		mid[i] = midAbility
	}
	striker := w.newAgent(200, 300, Male, append([]float64(nil), mid...), 0, 1)
	struck := w.newAgent(210, 300, Female, append([]float64(nil), mid...), 0, 1)
	a := mustAgent(t, w, w.addAgent(striker))
	b := mustAgent(t, w, w.addAgent(struck))
	return w, a, b
}

// strike resolves one blow from a at b, the way a tick does. The striker is
// put in the stance first: what a blow carries comes off the action the body
// is taking, not off the attack record.
func strike(w *World, a, b *Agent) {
	strikeAt(w, a, b, 1, false)
}

func strikeAt(w *World, a, b *Agent, effort float64, thrown bool) {
	a.Action = Action{Kind: ActAttack, TargetID: b.ID, Effort: effort, Stance: StanceAggressive}
	w.attacks = append(w.attacks[:0], attack{fromID: a.ID, toID: b.ID, effort: effort, thrown: thrown, hit: 1})
	w.resolveAttacks()
}

func TestKnockbackOffByDefault(t *testing.T) {
	if got := DefaultConfig().KnockbackDist; got != 0 {
		t.Fatalf("KnockbackDist default = %v, want 0: the rule is the map author's", got)
	}
	w, a, b := knockWorld(t, func(c *Config) { c.KnockbackDist = 0 })
	x, y := b.X, b.Y
	strike(w, a, b)
	if b.X != x || b.Y != y {
		t.Fatalf("body moved with the rule off: (%v,%v) -> (%v,%v)", x, y, b.X, b.Y)
	}
	if w.Knocks().Pushed != 0 {
		t.Fatalf("Pushed = %d with the rule off", w.Knocks().Pushed)
	}
}

func TestKnockbackPushesAlongTheBlow(t *testing.T) {
	w, a, b := knockWorld(t, nil)
	x, y := b.X, b.Y
	strike(w, a, b)

	if b.Y != y {
		t.Fatalf("pushed sideways: y %v -> %v", y, b.Y)
	}
	moved := b.X - x
	if moved <= 0 {
		t.Fatalf("pushed towards the striker: moved %v", moved)
	}
	// A mid body taking a full blow moves about the configured distance: the
	// damage is AttackDamage and the mass is the reference one.
	if math.Abs(moved-w.cfg.KnockbackDist) > 0.5 {
		t.Fatalf("moved %v, want about %v", moved, w.cfg.KnockbackDist)
	}
	if got := w.Knocks(); got.Pushed != 1 || got.Wet != 0 || got.Fell != 0 {
		t.Fatalf("Knocks = %+v, want one plain push", got)
	}
}

func TestKnockbackHeavyBodyGivesLessGround(t *testing.T) {
	light, heavy := 0.0, 0.0
	for _, big := range []bool{false, true} {
		w, a, b := knockWorld(t, nil)
		if big {
			b.Genome[GeneVitality] = MaxAbility
			b.Vitality = b.MaxVitality(&w.cfg)
		}
		x := b.X
		strike(w, a, b)
		if big {
			heavy = b.X - x
		} else {
			light = b.X - x
		}
	}
	if !(heavy > 0 && heavy < light) {
		t.Fatalf("heavy moved %v, light moved %v: a bigger body must give less ground", heavy, light)
	}
}

func TestKnockbackFollowsWhatLanded(t *testing.T) {
	// Half the effort into the blow, half the distance: the push reads the
	// damage, so everything already priced into a blow is priced into it.
	w, a, b := knockWorld(t, nil)
	x := b.X
	strikeAt(w, a, b, 0.5, false)
	half := b.X - x

	w2, a2, b2 := knockWorld(t, nil)
	x2 := b2.X
	strike(w2, a2, b2)
	full := b2.X - x2

	if math.Abs(full/2-half) > 0.05 {
		t.Fatalf("half a blow moved %v, a whole one %v: the push must follow the damage", half, full)
	}
}

func TestKnockbackThrownStoneDoesNotPushByDefault(t *testing.T) {
	w, a, b := knockWorld(t, nil)
	x := b.X
	strikeAt(w, a, b, 1, true)
	if b.X != x {
		t.Fatalf("a thrown stone pushed: %v -> %v", x, b.X)
	}

	w2, a2, b2 := knockWorld(t, func(c *Config) { c.KnockbackThrown = true })
	x2 := b2.X
	strikeAt(w2, a2, b2, 1, true)
	if b2.X <= x2 {
		t.Fatalf("KnockbackThrown did nothing: %v -> %v", x2, b2.X)
	}
}

// --- the ground behind the one being hit ------------------------------------

// waterTo the right of the strikers, so that a push carries a body into it.
func TestKnockbackIntoWater(t *testing.T) {
	w, a, b := knockWorld(t, func(c *Config) {
		c.TerrainMap = []string{
			"....~~~~",
			"....~~~~",
			"....~~~~",
			"....~~~~",
		}
		c.KnockbackDist = 60
	})
	a.X, a.Y = 320, 300
	b.X, b.Y = 370, 300 // dry, with the bank thirty units behind it
	if w.terrainAt(b.X, b.Y).Drown != 0 {
		t.Fatalf("the struck body starts in the water: the test says nothing")
	}
	strike(w, a, b)
	if w.terrainAt(b.X, b.Y).Drown == 0 {
		t.Fatalf("not pushed into the water: x = %v", b.X)
	}
	if got := w.Knocks(); got.Wet != 1 || got.Fell != 0 {
		t.Fatalf("Knocks = %+v, want one wet push", got)
	}
}

// A body already in the water is not counted again for being pushed about in
// it: what the count is for is the rule putting somebody somewhere new.
func TestKnockbackWithinWaterIsNotCountedWet(t *testing.T) {
	w, a, b := knockWorld(t, func(c *Config) {
		c.TerrainMap = []string{"~~~~~~~~", "~~~~~~~~", "~~~~~~~~", "~~~~~~~~"}
	})
	strike(w, a, b)
	if got := w.Knocks(); got.Pushed != 1 || got.Wet != 0 {
		t.Fatalf("Knocks = %+v, want a push that is not counted wet", got)
	}
}

func TestKnockbackOffALedge(t *testing.T) {
	// The right half is a level up, with no ramp: walking cannot get on or
	// off it, and a shove can only take a body down.
	terrain := []string{
		"....1111",
		"....1111",
		"....1111",
		"....1111",
	}
	w, a, b := knockWorld(t, func(c *Config) {
		c.TerrainMap = terrain
		c.KnockbackDist = 60
	})
	// Put the pair up on the plateau, the striker deeper in it, so the push
	// carries the struck one over the edge.
	a.X, a.Y = 480, 300
	b.X, b.Y = 420, 300
	if w.terrainAt(b.X, b.Y).Height == 0 {
		t.Fatalf("the struck body is not on the plateau")
	}
	before := b.Vitality
	strike(w, a, b)

	if h := w.terrainAt(b.X, b.Y).Height; h != 0 {
		t.Fatalf("still on the plateau: height %d, x %v", h, b.X)
	}
	if got := w.Knocks(); got.Fell != 1 {
		t.Fatalf("Knocks = %+v, want one fall", got)
	}
	// It paid for the blow and for the drop.
	blow := damagePerTick(&w.cfg, a.Attack(&w.cfg), 1)
	if want := before - blow - w.cfg.KnockbackFall; math.Abs(b.Vitality-want) > 1e-9 {
		t.Fatalf("vitality %v, want %v (blow %v + fall %v)", b.Vitality, want, blow, w.cfg.KnockbackFall)
	}
}

func TestKnockbackCannotPushUphill(t *testing.T) {
	w, a, b := knockWorld(t, func(c *Config) {
		c.TerrainMap = []string{
			"....1111",
			"....1111",
			"....1111",
			"....1111",
		}
		c.KnockbackDist = 60
	})
	a.X, a.Y = 300, 300
	b.X, b.Y = 360, 300 // flat, with the plateau to its right
	strike(w, a, b)
	if h := w.terrainAt(b.X, b.Y).Height; h != 0 {
		t.Fatalf("shoved up onto the plateau (height %d): being hit must not be a way up", h)
	}
	if got := w.Knocks(); got.Fell != 0 {
		t.Fatalf("Knocks = %+v, want no fall", got)
	}
}

// A fall that empties a body is a killing, not a new way to die: the blow set
// the clock, so the ordinary bucket claims it and whoever pushed is on the
// list the carcass and the onlookers read.
func TestKnockbackFallDeathCountsAsAKilling(t *testing.T) {
	w, a, b := knockWorld(t, func(c *Config) {
		c.TerrainMap = []string{
			"....1111",
			"....1111",
			"....1111",
			"....1111",
		}
		c.KnockbackDist = 60
		c.KnockbackFall = 500
	})
	a.X, a.Y = 480, 300
	b.X, b.Y = 420, 300
	strike(w, a, b)
	w.metabolise()

	if b.Alive {
		t.Fatalf("survived a fall of 500")
	}
	if got := w.Stats(); got.Kills != 1 || got.DrownDeaths != 0 {
		t.Fatalf("kills %d, drownings %d: a fall is the end of a blow", got.Kills, got.DrownDeaths)
	}
}

// The rule draws no random numbers: the same world with it on and off has
// pulled the same number of draws out of the source by the time the blow is
// resolved.
func TestKnockbackDrawsNoRandomNumbers(t *testing.T) {
	var draws [2]uint64
	for i, dist := range []float64{0, 6} {
		w, a, b := knockWorld(t, func(c *Config) { c.KnockbackDist = dist })
		before := w.draws.draws
		strike(w, a, b)
		draws[i] = w.draws.draws - before
	}
	if draws[0] != draws[1] {
		t.Fatalf("the push drew %d random numbers against %d without it", draws[1], draws[0])
	}
}
