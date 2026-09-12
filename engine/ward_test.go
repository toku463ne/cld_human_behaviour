package engine

import (
	"math"
	"testing"
)

// warded is a one-row table whose enemy is the sort there is lore about.
func warded() []EnemyKind {
	return []EnemyKind{{Name: "brute", Share: 1, Homing: 1, Homely: 1, Ward: SkillWard}}
}

// Which beasts are worth knowing about is the map's to say, and a body that
// knows the lore keeps some of their blows off itself.
func TestKnowingTheBeastTakesSomethingOffItsBlows(t *testing.T) {
	cfg := quietConfig()
	cfg.EnemyKinds = warded()
	w := NewWorld(cfg)

	beast := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100,
		Species: SpeciesEnemy, Genome: filledGenome(50)}))
	knows := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 105, Y: 100,
		Genome: filledGenome(50)}))
	greenhorn := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 110, Y: 100,
		Genome: filledGenome(50)}))
	knows.hintSlots = 2
	w.learnSkill(knows, SkillWard, 1)

	if got := w.wardAgainst(greenhorn, beast); got != 0 {
		t.Fatalf("a body with no lore turns aside %v", got)
	}
	got := w.wardAgainst(knows, beast)
	if got <= 0 {
		t.Fatalf("a body that knows the lore turns aside %v", got)
	}
	// Capped by the gene the skill is hung on: defence, at the middle of its
	// range, buys about half of what the relief is worth.
	if want := clamp(50.0/MaxAbility, 0, 1) * cfg.SkillWardRelief; math.Abs(got-want) > 1e-9 {
		t.Fatalf("the lore is worth %v, want %v (capped by defence)", got, want)
	}

	// And nothing at all against another human, however well it is known.
	other := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 120, Y: 100,
		Genome: filledGenome(50)}))
	if got := w.wardAgainst(knows, other); got != 0 {
		t.Fatalf("the beast lore works on people: %v", got)
	}
}

// An unwarded sort teaches nobody anything, which is what keeps a map without
// beasts worth knowing about exactly as it was.
func TestAnUnwardedSortIsTheWorldAsItWas(t *testing.T) {
	run := func(kinds []EnemyKind) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 79
		cfg.SkillBirthplace = 0.5
		cfg.EnemyKinds = kinds
		w := NewWorld(cfg)
		for i := 0; i < 1500; i++ {
			w.Step()
		}
		return w.Stats()
	}
	plain := run([]EnemyKind{{Name: "brute", Share: 1, Homing: 1, Homely: 1}})
	if plain != run(nil) {
		t.Fatal("spelling out the only sort changed the run")
	}
	if lore := run(warded()); lore == plain {
		t.Fatal("naming a sort worth knowing about changed nothing at all")
	}
}

// The lore is seeded by where a body was born, like every other skill that is:
// the country the beasts come from teaches it, and country they never reach
// does not.
func TestTheLoreIsSeededWhereTheBeastsAre(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 83
	cfg.EnemySpread = 0.9
	cfg.EnemyKinds = warded()
	w := NewWorld(cfg)

	low, high := 0, 0
	for i := range w.regions {
		if w.regions[i].Enemies < w.regions[low].Enemies {
			low = i
		}
		if w.regions[i].Enemies > w.regions[high].Enemies {
			high = i
		}
	}
	lowShare, highShare := w.regionWardShare(low), w.regionWardShare(high)
	if highShare <= lowShare {
		t.Fatalf("the beast country teaches %v and the quiet country %v", highShare, lowShare)
	}

	// And a world where no sort is worth knowing about teaches none of it.
	cfg.EnemyKinds = []EnemyKind{{Name: "brute", Share: 1, Homing: 1, Homely: 1}}
	quiet := NewWorld(cfg)
	for i := range quiet.regions {
		if got := quiet.regionWardShare(i); got != 0 {
			t.Fatalf("a world with no lore in it teaches %v", got)
		}
	}
}

// Both halves come off the same figure: what the body keeps off itself, and
// what it therefore reckons standing near one costs.
func TestTheLoreIsWorthTheSameToTheBodyAndToItsReckoning(t *testing.T) {
	cfg := quietConfig()
	cfg.EnemyKinds = warded()
	w := NewWorld(cfg)

	beast := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100,
		Species: SpeciesEnemy, Genome: filledGenome(50)}))
	knows := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 110, Y: 100,
		Genome: filledGenome(50)}))
	knows.hintSlots = 2
	w.learnSkill(knows, SkillWard, 1)
	w.Step()

	p := w.perceive(mustAgent(t, w, knows.ID))
	for i := range p.Others {
		if p.Others[i].ID != beast.ID {
			continue
		}
		if got, want := p.Others[i].Ward, w.wardAgainst(knows, beast); math.Abs(got-want) > 1e-9 {
			t.Fatalf("the reckoning says %v and the body %v", got, want)
		}
		return
	}
	t.Fatal("the beast was not in sight")
}
