package engine

import (
	"math"
	"testing"
)

// skillWorld is a still world laid over broken country in the west, so that a
// body can be put on hard ground or on a field.
func skillWorld(t *testing.T) *World {
	t.Helper()
	cfg := quietConfig()
	cfg.Width, cfg.Height = 800, 600
	cfg.TerrainMap = []string{
		"::::....",
		"::::....",
		"::::....",
	}
	// A map's author turns skills on; the physics does not (see
	// Config.SkillBirthplace). Every test here is about a world that has.
	cfg.SkillBirthplace = 0.5
	return NewWorld(cfg)
}

func skilled(t *testing.T, w *World, x, y, speed, mastery float64) *Agent {
	t.Helper()
	g := genomeOf(50, 50, 50)
	g[GeneSpeed] = speed
	id := w.addAgent(Agent{Maturity: 1, X: x, Y: y, Vitality: 90, Genome: g})
	a := mustAgent(t, w, id)
	a.hintSlots = 2
	if mastery > 0 {
		w.learnSkill(a, SkillRough, mastery)
	}
	return a
}

// Knowing the ground takes some of the extra out of crossing it, and never
// makes broken country cheaper than a field.
func TestKnowingTheGroundTakesTheStingOutOfIt(t *testing.T) {
	w := skillWorld(t)
	green := skilled(t, w, 100, 100, 100, 0)
	adept := skilled(t, w, 100, 100, 100, 1)

	rough, open := 100.0, 700.0
	greenRough := w.moveCostOn(green, rough, 100, 1)
	adeptRough := w.moveCostOn(adept, rough, 100, 1)
	adeptOpen := w.moveCostOn(adept, open, 100, 1)

	if !(adeptRough < greenRough) {
		t.Fatalf("crossing the rough cost the adept %v and the green one %v", adeptRough, greenRough)
	}
	if adeptRough < adeptOpen-1e-9 {
		t.Fatalf("the rough cost %v and a field %v: no skill makes hard ground easier than easy ground",
			adeptRough, adeptOpen)
	}
	// And the world with the rule off is the world as it was.
	w.cfg.SkillRoughRelief = 0
	if got := w.moveCostOn(adept, rough, 100, 1); math.Abs(got-greenRough) > 1e-9 {
		t.Fatalf("with the rule off the adept paid %v, want the plain %v", got, greenRough)
	}
}

// What a body gets out of a figure is capped by what its legs can support: a
// slow one realises less than it holds, a fast one realises the lot.
func TestABodyOnlyGetsWhatItCanSupport(t *testing.T) {
	w := skillWorld(t)
	slow := skilled(t, w, 100, 100, 30, 1)
	quick := skilled(t, w, 100, 100, 90, 1)

	if got := slow.skillAt(&w.cfg, SkillRough); math.Abs(got-0.3) > 1e-9 {
		t.Fatalf("the slow one realises %v of a full mastery, want its own 0.30", got)
	}
	if got := quick.skillAt(&w.cfg, SkillRough); math.Abs(got-0.9) > 1e-9 {
		t.Fatalf("the quick one realises %v, want its own 0.90", got)
	}
	// Holding more than the body supports is not worth more.
	w.learnSkill(slow, SkillRough, 1)
	if got := slow.skillAt(&w.cfg, SkillRough); math.Abs(got-0.3) > 1e-9 {
		t.Fatalf("a bigger figure bought the same body %v", got)
	}
}

// Every way of coming by a skill ends at the same comparison: better replaces
// worse, worse is refused, and an agent with no room learns nothing.
func TestASkillIsTakenOnlyWhenItIsBetter(t *testing.T) {
	w := skillWorld(t)
	a := skilled(t, w, 100, 100, 90, 0.4)

	if w.learnSkill(a, SkillRough, 0.2) {
		t.Fatal("took on a worse figure than the one it held")
	}
	if got := a.nominalSkill(SkillRough); got != 0.4 {
		t.Fatalf("holds %v, want the 0.40 it had", got)
	}
	if !w.learnSkill(a, SkillRough, 0.8) {
		t.Fatal("refused a better figure")
	}
	if got := a.nominalSkill(SkillRough); got != 0.8 {
		t.Fatalf("holds %v, want 0.80", got)
	}

	// No room, nothing new. Room is what learning costs, here as everywhere.
	full := skilled(t, w, 100, 100, 90, 0)
	full.hintSlots = 0
	if w.learnSkill(full, SkillRough, 0.9) {
		t.Fatal("learned something with nowhere to put it")
	}
}

// Being born in the rough is itself a way of knowing the rough, and a world
// with no map teaches nobody anything - which is why a flat world runs exactly
// as it always did.
func TestWhereABodyIsBornIsWhatItKnows(t *testing.T) {
	w := skillWorld(t)
	if got := w.skillFromBirthplace(SkillRough, 100, 100); got <= 0 {
		t.Fatalf("born in broken country and knowing %v of it", got)
	}
	if rough, open := w.skillFromBirthplace(SkillRough, 100, 100), w.skillFromBirthplace(SkillRough, 700, 100); rough <= open {
		t.Fatalf("the rough taught %v and the open %v", rough, open)
	}

	flat := quietConfig()
	flat.SkillBirthplace = 0.5
	if got := NewWorld(flat).skillFromBirthplace(SkillRough, 100, 100); got != 0 {
		t.Fatalf("a world with no map taught %v", got)
	}
	// And with the rule off, neither does broken country: no skill enters a
	// world whose author did not ask for one.
	off := w.cfg
	off.SkillBirthplace = 0
	if got := NewWorld(off).skillFromBirthplace(SkillRough, 100, 100); got != 0 {
		t.Fatalf("with the rule off the rough taught %v", got)
	}
}

// Watching somebody who knows the ground better is one of the three ways, and
// the only one that does not need a parent. It can be turned off, which is the
// arm that says how much of what a population knows came from watching.
func TestASkillIsCaughtFromWhoeverKnowsBetter(t *testing.T) {
	w := skillWorld(t)
	learner := skilled(t, w, 100, 100, 90, 0.2)
	master := skilled(t, w, 110, 100, 90, 0.9)

	w.teachSkill(learner, master)
	if got := learner.nominalSkill(SkillRough); got != 0.9 {
		t.Fatalf("after watching, holds %v, want 0.90", got)
	}
	// And nothing flows the other way: the better figure is not dragged down.
	w.teachSkill(master, learner)
	if got := master.nominalSkill(SkillRough); got != 0.9 {
		t.Fatalf("the one who knew better came away with %v", got)
	}

	w.cfg.SkillsSpread = false
	green := skilled(t, w, 120, 100, 90, 0)
	w.teachSkill(green, master)
	if got := green.nominalSkill(SkillRough); got != 0 {
		t.Fatalf("caught %v with the copying turned off", got)
	}
}

// A slot holding a skill is not an opinion about a move: it adds nothing to
// any option's score.
func TestASkillSaysNothingAboutWhichMoveToMake(t *testing.T) {
	var f hintFeatures
	f[HintHunger] = 1
	held := []Hint{
		{Feature: HintHunger, Act: ActEat, Weight: 5},
		{Skill: SkillRough, Mastery: 1},
	}
	for kind := ActionKind(0); kind < numActionKinds; kind++ {
		want := 0.0
		if kind == ActEat {
			want = 5
		}
		if got := f.score(held, kind); math.Abs(got-want) > 1e-9 {
			t.Fatalf("a skill and an idea together scored %v for %s, want %v", got, kind, want)
		}
	}
}

// --- the second skill (stage 38b) -------------------------------------------

// Knowing how to forage is a yield and not a race: the same mouthful goes
// further for whoever has it, and nothing about who reaches the plant first
// changes.
func TestForagingMakesTheSameMouthfulGoFurther(t *testing.T) {
	cfg := quietConfig()
	cfg.SkillBirthplace = 0.5
	w := NewWorld(cfg)

	green := skilled(t, w, 100, 100, 90, 0)
	adept := skilled(t, w, 100, 100, 90, 0)
	for _, a := range []*Agent{green, adept} {
		a.Genome[GeneMemory] = 100 // so the ceiling is not what is being tested
	}
	w.learnSkill(adept, SkillForage, 1)

	// Both have been living on the same thing.
	for i := 0; i < 10; i++ {
		w.noteEaten(green, FoodPlant)
		w.noteEaten(adept, FoodPlant)
	}
	dull, sharp := w.dietValue(green, FoodPlant), w.dietValue(adept, FoodPlant)
	if !(sharp > dull) {
		t.Fatalf("a monotonous meal is worth %v to the forager and %v to anybody else", sharp, dull)
	}
	if sharp > 1+1e-9 {
		t.Fatalf("the meal is worth %v: no skill makes food worth more than food", sharp)
	}
	// And with the rule off it is worth exactly what everybody else's is.
	w.cfg.SkillForageRelief = 0
	if got := w.dietValue(adept, FoodPlant); math.Abs(got-dull) > 1e-9 {
		t.Fatalf("with the rule off the forager's meal is worth %v, want the plain %v", got, dull)
	}
}

// The second skill is seeded by what the country provides rather than by what
// it is made of, so a world with no map can have it - and it is thin ground
// that teaches it.
func TestForagingIsTaughtByThinGround(t *testing.T) {
	cfg := quietConfig()
	cfg.SkillBirthplace = 0.5
	w := NewWorld(cfg)
	if len(w.regions) < 2 {
		t.Fatal("the world has no regions to differ")
	}
	// Two regions, one rich and one thin, and the same body born in each.
	rich, thin := 0, 1
	w.regions[rich].Food, w.regions[thin].Food = 1.6, 0.2
	rx, ry := regionCentre(w, rich)
	tx, ty := regionCentre(w, thin)

	if got := w.skillFromBirthplace(SkillForage, rx, ry); got > 0.001 {
		t.Fatalf("born in plenty and knowing %v about making it last", got)
	}
	if got := w.skillFromBirthplace(SkillForage, tx, ty); got <= 0 {
		t.Fatalf("born on thin ground and knowing %v", got)
	}
	// A world with no map has none of the first skill and can still have this
	// one: what it reads is what the ground provides, not what it is made of.
	if got := w.skillFromBirthplace(SkillRough, tx, ty); got != 0 {
		t.Fatalf("a flat world taught %v about crossing broken country", got)
	}
}

func regionCentre(w *World, i int) (float64, float64) {
	minX, minY, maxX, maxY := w.regionBounds(i)
	return (minX + maxX) / 2, (minY + maxY) / 2
}

// Each skill is capped by its own gene: legs for the ground, memory for making
// what is found go further.
func TestEachSkillIsCappedByItsOwnGene(t *testing.T) {
	cfg := quietConfig()
	cfg.SkillBirthplace = 0.5
	w := NewWorld(cfg)
	a := skilled(t, w, 100, 100, 80, 0)
	a.hintSlots = 2
	a.Genome[GeneMemory] = 20
	w.learnSkill(a, SkillRough, 1)
	w.learnSkill(a, SkillForage, 1)

	if got := a.skillAt(&w.cfg, SkillRough); math.Abs(got-0.8) > 1e-9 {
		t.Fatalf("rough going realised %v, want the legs' 0.80", got)
	}
	if got := a.skillAt(&w.cfg, SkillForage); math.Abs(got-0.2) > 1e-9 {
		t.Fatalf("foraging realised %v, want the memory's 0.20", got)
	}
}
