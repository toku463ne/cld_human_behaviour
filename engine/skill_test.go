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
	if got := w.skillFromBirthplace(100, 100); got <= 0 {
		t.Fatalf("born in broken country and knowing %v of it", got)
	}
	if rough, open := w.skillFromBirthplace(100, 100), w.skillFromBirthplace(700, 100); rough <= open {
		t.Fatalf("the rough taught %v and the open %v", rough, open)
	}

	flat := quietConfig()
	flat.SkillBirthplace = 0.5
	if got := NewWorld(flat).skillFromBirthplace(100, 100); got != 0 {
		t.Fatalf("a world with no map taught %v", got)
	}
	// And with the rule off, neither does broken country: no skill enters a
	// world whose author did not ask for one.
	off := w.cfg
	off.SkillBirthplace = 0
	if got := NewWorld(off).skillFromBirthplace(100, 100); got != 0 {
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
