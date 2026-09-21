package engine

import "testing"

// lessonWorld is a quiet world with room for lessons and nothing else going
// on: bodies are placed by hand, and the only deaths are the ones the test
// causes.
func lessonWorld(t *testing.T, tune func(*Config)) (*World, func(x, y float64) *Agent) {
	t.Helper()
	cfg := DefaultConfig()
	cfg.InitialPopulation, cfg.InitialFoodItems, cfg.FoodSpawnRate = 0, 0, 0
	cfg.EnemySpawnTicks = 0
	cfg.MutationStd, cfg.MutationRate = 0, 0
	cfg.LessonSlots = 2
	if tune != nil {
		tune(&cfg)
	}
	w := NewWorld(cfg)
	mid := make([]float64, NumGenes)
	for i := range mid {
		mid[i] = midAbility
	}
	place := func(x, y float64) *Agent {
		a := mustAgent(t, w, w.addAgent(w.newAgent(x, y, Male, append([]float64(nil), mid...), 0, 1)))
		a.lessonSlots = 2
		return a
	}
	return w, place
}

func TestLessonsOffByDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.LessonSlots != 0 {
		t.Fatalf("LessonSlots default = %d, want 0: room costs budget, so a world that has not asked for it must buy none", cfg.LessonSlots)
	}
	// And nothing is drawn for it, which is what keeps every world before
	// this one consuming the random source as it did.
	w := NewWorld(cfg)
	if got := w.drawLessonSlots(); got != 0 {
		t.Fatalf("drawLessonSlots = %d with the rule off", got)
	}
}

func TestADeathIsAboutHowItDied(t *testing.T) {
	w, place := lessonWorld(t, nil)
	w.tick = 500 // a world that has been running, so "never hit" is not "hit at tick 0"
	cases := []struct {
		name string
		set  func(*Agent)
		want DeathFeature
	}{
		{"drowned", func(a *Agent) { a.drowned = true; a.Vitality = 0 }, DeathWet},
		{"struck", func(a *Agent) { a.lastAttackTick = w.tick; a.Vitality = 0 }, DeathStruck},
		{"starved", func(a *Agent) { a.Hunger = w.cfg.MaxHunger; a.Vitality = 0 }, DeathEmpty},
		{"worn", func(a *Agent) { a.Vitality = 0 }, DeathWorn},
	}
	for _, c := range cases {
		a := place(100, 100)
		a.Action = Action{Kind: ActRest}
		c.set(a)
		got, act := w.deathLesson(a)
		if got != c.want {
			t.Fatalf("%s: death was about %v, want %v", c.name, got, c.want)
		}
		if act != ActRest {
			t.Fatalf("%s: caught it doing %v, want rest", c.name, act)
		}
		a.Alive = false
	}
}

func TestADrowningBeatsABlow(t *testing.T) {
	// The precedence is the world's own: the river takes a body even if
	// somebody was hitting it a moment before (world.go's exclusive buckets).
	w, place := lessonWorld(t, nil)
	w.tick = 500
	a := place(100, 100)
	a.Action = Action{Kind: ActMove}
	a.drowned, a.lastAttackTick, a.Vitality = true, w.tick, 0
	if got, _ := w.deathLesson(a); got != DeathWet {
		t.Fatalf("death was about %v, want the water", got)
	}
}

func TestItTakesTwoDeathsToLearn(t *testing.T) {
	w, place := lessonWorld(t, nil)
	w.tick = 500
	watcher := place(100, 100)
	die := func() {
		v := place(110, 100)
		v.Action = Action{Kind: ActRest}
		v.Hunger, v.Vitality = w.cfg.MaxHunger, 0
		w.kill(v)
	}

	die()
	if len(watcher.lessons) != 0 {
		t.Fatalf("learnt something from one death: %+v", watcher.lessons)
	}
	if w.LessonUse().Watched != 1 {
		t.Fatalf("the death was not counted as watched")
	}

	die()
	if len(watcher.lessons) != 1 {
		t.Fatalf("learnt nothing from the second death of the same kind: %+v", watcher.lessons)
	}
	l := watcher.lessons[0]
	if l.Feature != DeathEmpty || l.Act != ActRest {
		t.Fatalf("learnt %v x %v, want starving x rest", l.Feature, l.Act)
	}
	if l.Weight >= 0 {
		t.Fatalf("weight = %v, want a mark against", l.Weight)
	}

	// And a third of the same kind teaches nothing new: it is already held.
	die()
	if len(watcher.lessons) != 1 {
		t.Fatalf("the same pattern took a second slot: %+v", watcher.lessons)
	}
}

func TestOneDeathIsEnoughWhenTheWorldSaysSo(t *testing.T) {
	w, place := lessonWorld(t, func(c *Config) { c.LessonRipeTwice = false })
	watcher := place(100, 100)
	v := place(110, 100)
	v.Action = Action{Kind: ActRest}
	v.Hunger, v.Vitality = w.cfg.MaxHunger, 0
	w.kill(v)
	if len(watcher.lessons) != 1 {
		t.Fatalf("learnt nothing from one death with the gate off: %+v", watcher.lessons)
	}
}

func TestADeathNobodySawTeachesNobody(t *testing.T) {
	w, place := lessonWorld(t, nil)
	far := place(100, 100)
	v := place(100, 100)
	v.X, v.Y = w.cfg.Width-1, w.cfg.Height-1
	v.Action = Action{Kind: ActRest}
	v.Vitality = 0
	w.kill(v)
	if got := w.LessonUse().Unseen; got != 1 {
		t.Fatalf("Unseen = %d, want 1", got)
	}
	if len(far.lessons) != 0 || far.seenDeaths != (deathMarks{}) {
		t.Fatalf("somebody out of sight learnt from it")
	}
}

func TestABodyWithNoRoomLearnsNothing(t *testing.T) {
	w, place := lessonWorld(t, nil)
	watcher := place(100, 100)
	watcher.lessonSlots = 0
	for i := 0; i < 3; i++ {
		v := place(110, 100)
		v.Action = Action{Kind: ActRest}
		v.Hunger, v.Vitality = w.cfg.MaxHunger, 0
		w.kill(v)
	}
	if len(watcher.lessons) != 0 {
		t.Fatalf("a body that bought no room learnt %+v", watcher.lessons)
	}
	// But the world still counts what it could have learnt: how often a rule
	// could fire is the ceiling on what it can explain.
	if w.LessonUse().Ripe == 0 {
		t.Fatalf("the ripening was not counted for a body with no room")
	}
}

func TestRoomForLessonsIsPaidForOutOfTheBody(t *testing.T) {
	// The conservation law: learning cannot make a body better for free.
	build := func(slots int) float64 {
		cfg := DefaultConfig()
		cfg.InitialPopulation, cfg.InitialFoodItems, cfg.FoodSpawnRate = 0, 0, 0
		cfg.EnemySpawnTicks = 0
		cfg.LessonSlots, cfg.HintSlots = slots, 0
		w := NewWorld(cfg)
		mid := make([]float64, NumGenes)
		for i := range mid {
			mid[i] = midAbility
		}
		a := w.newAgent(100, 100, Male, append([]float64(nil), mid...), 0, 1)
		a.lessonSlots = slots
		fitBudget(a.Genome, a.Budget()-w.lessonCost(slots))
		total := 0.0
		for _, g := range a.Genome {
			total += g
		}
		return total
	}
	none, two := build(0), build(2)
	if !(two < none) {
		t.Fatalf("two slots cost nothing: genome sum %v against %v", two, none)
	}
}

func TestALessonOnlyEverAddsToAScore(t *testing.T) {
	// The one rule a lesson may never break (stage 12c, and hint.go's first
	// paragraph): it is a number added to an option, and nothing anywhere
	// branches on it.
	w, place := lessonWorld(t, nil)
	a := place(100, 100)
	p := w.perceive(a)
	var f deathFeatures
	f.readSelf(p)

	lessons := []Lesson{{Feature: DeathStruck, Act: ActRest, Weight: -3}}
	if got := f.score(lessons, ActMove); got != 0 {
		t.Fatalf("a lesson about resting touched a move: %v", got)
	}
	p.Self.AttackerID = 7
	f.readSelf(p)
	if got := f.score(lessons, ActRest); got != -3 {
		t.Fatalf("score = %v, want the whole weight while being hit", got)
	}
	p.Self.AttackerID = 0
	f.readSelf(p)
	if got := f.score(lessons, ActRest); got != 0 {
		t.Fatalf("score = %v, want nothing when the situation is not the one", got)
	}
}

func TestALessonIsCopiedIntoAnEmptySlotOnly(t *testing.T) {
	w, place := lessonWorld(t, nil)
	teacher, learner := place(100, 100), place(104, 100)
	teacher.lessons = []Lesson{
		{Feature: DeathWet, Act: ActMove, Weight: -3},
		{Feature: DeathStruck, Act: ActRest, Weight: -3},
	}
	learner.lessons = []Lesson{{Feature: DeathWet, Act: ActMove, Weight: -3}}

	if n := w.exchangeLessons(learner, teacher); n != 1 {
		t.Fatalf("copied %d, want 1: the one it already holds must not be copied again", n)
	}
	if len(learner.lessons) != 2 {
		t.Fatalf("learner holds %d lessons, want 2", len(learner.lessons))
	}
	// And a full body takes nothing more.
	teacher.lessons = append(teacher.lessons, Lesson{Feature: DeathEmpty, Act: ActEat, Weight: -3})
	if n := w.exchangeLessons(learner, teacher); n != 0 {
		t.Fatalf("copied %d into a full body", n)
	}
}

func TestNobodyIsTaughtWhenTheWorldSaysSo(t *testing.T) {
	w, place := lessonWorld(t, func(c *Config) { c.LessonsSpread = false })
	teacher, learner := place(100, 100), place(104, 100)
	teacher.lessons = []Lesson{{Feature: DeathWet, Act: ActMove, Weight: -3}}
	if n := w.exchangeLessons(learner, teacher); n != 0 {
		t.Fatalf("copied %d with spreading off", n)
	}
}

func TestAChildIsBornHavingSeenNothing(t *testing.T) {
	// Only the room is inherited. What a parent watched happen belongs to the
	// parent: that is the whole difference between a lesson and a rule of
	// thumb, and it is what LamarckRate asks about the beliefs.
	w, place := lessonWorld(t, nil)
	pa, pb := place(100, 100), place(104, 100)
	pa.lessons = []Lesson{{Feature: DeathWet, Act: ActMove, Weight: -3}}
	pa.seenDeaths.set(lessonKey(DeathWet, ActMove))
	pb.Sex = Female
	pa.Vitality, pb.Vitality = pa.MaxVitality(&w.cfg), pb.MaxVitality(&w.cfg)

	slots := w.inheritLessonSlots(pa, pb, false)
	if slots != 2 {
		t.Fatalf("child inherited %d slots, want 2", slots)
	}
	child := w.newAgent(102, 100, Male, append([]float64(nil), pa.Genome...), 1, 0)
	child.lessonSlots = slots
	if len(child.lessons) != 0 || child.seenDeaths != (deathMarks{}) {
		t.Fatalf("a newborn came with something it did not watch")
	}
}

func TestTheLessonKeySpaceFitsItsBitset(t *testing.T) {
	// The bitset is sized from the two vocabularies at compile time, but the
	// action list grows now and then and the arithmetic should be checked
	// rather than trusted.
	if got := lessonKey(NumDeathFeatures-1, numActionKinds-1); got >= numLessonKeys {
		t.Fatalf("the last key is %d and the space is %d", got, numLessonKeys)
	}
	var m deathMarks
	for k := 0; k < numLessonKeys; k++ {
		if m.has(k) {
			t.Fatalf("key %d set before anything was", k)
		}
		m.set(k)
		if !m.has(k) {
			t.Fatalf("key %d did not stay set", k)
		}
	}
}
