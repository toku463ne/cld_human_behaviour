package engine

import (
	"bytes"
	"testing"
)

func moodConfig() Config {
	cfg := quietConfig()
	cfg.MoodWeight = 0.5
	cfg.MoodDreadGain, cfg.MoodCheerGain = 1, 1
	cfg.MoodHalfLife = 150
	return cfg
}

// A blow frightens the one that took it, in proportion to what it cost, and
// nothing is held against the one that swung.
func TestBeingHitFrightensAndNamesNobody(t *testing.T) {
	w := NewWorld(moodConfig())
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100,
		Vitality: 90, Genome: genomeOf(50, 50, 50)}))
	max := a.MaxVitality(&w.cfg)

	w.frighten(a, 0.1*max)
	first := w.mood(a)
	if first >= 0 {
		t.Fatalf("a body that has just been hit feels %v, want less than nothing", first)
	}
	w.frighten(a, 0.4*max)
	if w.mood(a) >= first {
		t.Fatalf("a harder blow left it feeling %v against %v", w.mood(a), first)
	}
	// It is about the body, not about anybody: nothing was written down about
	// whoever did it.
	if len(w.Opinions(a.ID)) != 0 {
		t.Fatal("being frightened put somebody in this body's memory")
	}
}

// And it fades, at the half-life the world sets.
func TestAMoodFades(t *testing.T) {
	w := NewWorld(moodConfig())
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100,
		Vitality: 90, Genome: genomeOf(50, 50, 50)}))
	w.frighten(a, a.MaxVitality(&w.cfg))
	full := -w.mood(a)
	w.tick += w.cfg.MoodHalfLife
	if got := -w.mood(a); got > 0.51*full || got < 0.49*full {
		t.Fatalf("after one half-life the dread is %v of %v", got, full)
	}
	w.tick += 10 * w.cfg.MoodHalfLife
	if got := -w.mood(a); got > 0.01*full {
		t.Fatalf("after eleven half-lives the dread is still %v of %v", got, full)
	}
}

// A meal is the other side of it, and the two cancel: a body that has been
// beaten and then fed is somewhere in between.
func TestAMealCheersAndTheTwoOffset(t *testing.T) {
	w := NewWorld(moodConfig())
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100,
		Vitality: 90, Genome: genomeOf(50, 50, 50)}))
	w.frighten(a, 0.5*a.MaxVitality(&w.cfg))
	beaten := w.mood(a)
	w.please(a, 0.5*w.cfg.MaxHunger, 0)
	if fed := w.mood(a); fed <= beaten {
		t.Fatalf("after a meal it feels %v, having felt %v", fed, beaten)
	}
}

// What it changes is one preference, and only what the body sees: what it
// holds - and passes on - is untouched.
func TestAMoodLeansThePreferenceAndNotTheBody(t *testing.T) {
	w := NewWorld(moodConfig())
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100,
		Vitality: 90, Genome: genomeOf(50, 50, 50)}))
	held := a.lore.shockRisk

	w.frighten(a, 0.6*a.MaxVitality(&w.cfg))
	if felt := w.shockRiskFelt(a); felt <= held {
		t.Fatalf("a frightened body weighs being worn down at %v, having inherited %v", felt, held)
	}
	if a.lore.shockRisk != held {
		t.Fatalf("the fright changed what the body holds: %v, was %v", a.lore.shockRisk, held)
	}
	// And the other way for a body that things are going well for.
	b := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 300, Y: 300,
		Vitality: 90, Genome: genomeOf(50, 50, 50)}))
	w.please(b, w.cfg.MaxHunger, 0)
	if felt := w.shockRiskFelt(b); felt >= b.lore.shockRisk {
		t.Fatalf("a body that has just eaten weighs it at %v, having inherited %v",
			felt, b.lore.shockRisk)
	}
}

// No mood can take the weight to nothing or to everything: the lean is
// bounded, so a feeling can never stand in for the whole of a judgement.
func TestTheLeanIsBounded(t *testing.T) {
	cfg := moodConfig()
	cfg.MoodWeight = 4 // far past anything that would be used
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100,
		Vitality: 90, Genome: genomeOf(50, 50, 50)}))
	held := a.lore.shockRisk
	w.frighten(a, 10*a.MaxVitality(&w.cfg))
	if felt := w.shockRiskFelt(a); felt > 2*held {
		t.Fatalf("a wholly frightened body weighs it at %v, more than twice %v", felt, held)
	}
	w.please(a, 100*w.cfg.MaxHunger, 0)
	if felt := w.shockRiskFelt(a); felt < 0 {
		t.Fatalf("a wholly pleased body weighs it at %v", felt)
	}
}

// The placebo: no weight is the world before this stage, bit for bit. Nothing
// here draws a random number either way, which is what makes the arm worth
// running at all (the lesson of stage 50).
func TestNoMoodIsTheWorldWithoutIt(t *testing.T) {
	run := func(f func(*Config)) Stats {
		cfg := DefaultConfig()
		cfg.Seed = 5
		f(&cfg)
		w := NewWorld(cfg)
		for i := 0; i < 3000; i++ {
			w.Step()
		}
		return w.Stats()
	}
	off := run(func(c *Config) { c.MoodWeight, c.MoodHalfLife = 0, 0 })
	placebo := run(func(c *Config) { c.MoodWeight = 0 })
	if off != placebo {
		t.Fatalf("a world with no mood in it is not the world with no lean:\n off %+v\n placebo %+v",
			off, placebo)
	}
}

// A mood survives being saved, because the next tick depends on it.
func TestAMoodIsSaved(t *testing.T) {
	w := NewWorld(moodConfig())
	id := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Genome: genomeOf(50, 50, 50)})
	w.frighten(mustAgent(t, w, id), 30)
	want := w.mood(mustAgent(t, w, id))

	var buf bytes.Buffer
	if err := w.Save(&buf); err != nil {
		t.Fatal(err)
	}
	back, err := Load(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := back.mood(mustAgent(t, back, id)); got != want {
		t.Fatalf("the body came back feeling %v, having felt %v", got, want)
	}
}
