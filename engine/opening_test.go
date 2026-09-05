package engine

import "testing"

// Stage 24: a blow that finds nothing leaves the one who threw it off balance.
// It is the first rule in the world that prices missing.

// evasiveGenome is a body that always gets out of the way: everything on the
// evasion gene and on being quick, so that the chance clamps to the cap and the
// test does not depend on the roll.
func evasiveGenome() []float64 {
	g := genomeOf(midAbility, midAbility, midAbility)
	g[GeneEvasion] = MaxAbility
	g[GeneSpeed] = MaxAbility
	return g
}

func TestAMissLeavesTheSwingerOpen(t *testing.T) {
	cfg := quietConfig()
	cfg.EvasionCap = 1 // every blow at this one misses
	w := NewWorld(cfg)

	swinger := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 200, Hunger: 40,
		Genome: genomeOf(90, 100, 100)})
	dodger := w.addAgent(Agent{Maturity: 1,
		X: 205, Y: 200, Sex: Male, Vitality: 200, Hunger: 40,
		Genome: evasiveGenome()})
	w.SetController(swinger, fixedController{Action{Kind: ActAttack, TargetID: dodger, Effort: 1}})
	w.SetController(dodger, fixedController{
		Action{Kind: ActAttack, TargetID: swinger, Effort: 1, Stance: StanceEvasive}})

	before := w.Stats().Deaths
	w.Step()
	w.Step()

	a := mustAgent(t, w, swinger)
	if a.openUntil <= w.Tick() {
		t.Fatalf("swung and missed, and is open until tick %d at tick %d", a.openUntil, w.Tick())
	}
	if got := a.composure(&cfg, w.Tick()); got != cfg.OpeningGuard {
		t.Fatalf("composure while open = %v, want %v", got, cfg.OpeningGuard)
	}
	// And it wears off. Asked about a later tick rather than stepping the
	// world there, because the fight is still going and every tick of it
	// misses again.
	if got := a.composure(&cfg, a.openUntil); got != 1 {
		t.Fatalf("composure once the opening has run out = %v, want 1", got)
	}
	_ = before
}

// Switched off, nothing about a miss is remembered. This is what makes the two
// worlds a pair: off, the rule draws nothing and touches nothing.
func TestNoOpeningWhenSwitchedOff(t *testing.T) {
	cfg := quietConfig()
	cfg.EvasionCap = 1
	cfg.OpeningTicks = 0
	w := NewWorld(cfg)

	swinger := w.addAgent(Agent{Maturity: 1,
		X: 200, Y: 200, Sex: Male, Vitality: 200, Hunger: 40,
		Genome: genomeOf(90, 100, 100)})
	dodger := w.addAgent(Agent{Maturity: 1,
		X: 205, Y: 200, Sex: Male, Vitality: 200, Hunger: 40,
		Genome: evasiveGenome()})
	w.SetController(swinger, fixedController{Action{Kind: ActAttack, TargetID: dodger, Effort: 1}})
	w.SetController(dodger, fixedController{
		Action{Kind: ActAttack, TargetID: swinger, Effort: 1, Stance: StanceEvasive}})

	for i := 0; i < 5; i++ {
		w.Step()
	}
	if a := mustAgent(t, w, swinger); a.openUntil != 0 {
		t.Fatalf("open until tick %d with the rule switched off", a.openUntil)
	}
	if got := mustAgent(t, w, swinger).composure(&cfg, w.Tick()); got != 1 {
		t.Fatalf("composure = %v with the rule switched off, want 1", got)
	}
}

// Being off balance costs the guard, not the skin: the same blow lands for more
// because less of it is turned aside.
func TestAnOpenAgentGuardsForLess(t *testing.T) {
	lost := func(open bool) float64 {
		cfg := quietConfig()
		cfg.EvasionCap = 0 // no roll, so the two runs differ in one thing only
		w := NewWorld(cfg)
		hitter := w.addAgent(Agent{Maturity: 1,
			X: 200, Y: 200, Sex: Male, Vitality: 200, Hunger: 40,
			Genome: genomeOf(90, 100, 100)})
		guard := w.addAgent(Agent{Maturity: 1,
			X: 205, Y: 200, Sex: Male, Vitality: 200, Hunger: 40,
			Genome: genomeOf(midAbility, midAbility, midAbility)})
		w.SetController(hitter, fixedController{Action{Kind: ActAttack, TargetID: guard, Effort: 1}})
		w.SetController(guard, fixedController{
			Action{Kind: ActAttack, TargetID: hitter, Effort: 1, Stance: StanceGuarded}})
		w.Step()
		if open {
			mustAgent(t, w, guard).openUntil = w.Tick() + cfg.OpeningTicks
		}
		was := mustAgent(t, w, guard).Vitality
		w.Step()
		return was - mustAgent(t, w, guard).Vitality
	}

	composed, open := lost(false), lost(true)
	if open <= composed {
		t.Fatalf("an open guard lost %v to the same blow, a composed one %v", open, composed)
	}
}
