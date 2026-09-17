package engine

import "testing"

// Stage 96: wanting goodwill for its own sake, damped by how much of it the
// body already has standing near.
//
// What the goodwill an act buys is worth had been LoreValue flat since stage
// 48 - the same figure to a body with nobody in the world and to one standing
// among friends.
func TestGoodwillIsWorthMostToABodyWithNobody(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AllyValue = 18
	c := &AIController{}

	c.trustNear = 0
	alone := c.goodwillWorth(&cfg)
	c.trustNear = 1
	one := c.goodwillWorth(&cfg)
	c.trustNear = 4
	crowd := c.goodwillWorth(&cfg)

	if !(alone > one && one > crowd) {
		t.Fatalf("goodwill is worth %v alone, %v with one, %v with four", alone, one, crowd)
	}
	// Alone it is the whole addition; among enough of them it falls back
	// towards what every world before this paid.
	approx(t, alone, cfg.LoreValue+cfg.AllyValue, 1e-9, "the whole addition to a body with nobody")
	if crowd >= cfg.LoreValue+cfg.AllyValue/2 {
		t.Fatalf("a body among four still reads %v", crowd)
	}
	if crowd <= cfg.LoreValue {
		t.Fatalf("the addition went away entirely at %v", crowd)
	}
}

// With nothing asked for, it is LoreValue exactly, which is why the default
// world is the one it was.
func TestWithNoAllyValueGoodwillIsWhatItAlwaysWas(t *testing.T) {
	cfg := DefaultConfig()
	c := &AIController{}
	for _, near := range []float64{0, 1, 5, 100} {
		c.trustNear = near
		if got := c.goodwillWorth(&cfg); got != cfg.LoreValue {
			t.Fatalf("with no ally value and %v trust near, goodwill reads %v", near, got)
		}
	}
}

// The control arm: the same addition with no reason in it. It has to be a
// separate field, because an arm that only raises the amount cannot be told
// from one that gives the wanting a reason (stage 54 and stage 95 both needed
// a control of this shape).
func TestTheFlatArmRaisesGoodwillWithoutTheReason(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AllyFlat = 13.08
	c := &AIController{}
	for _, near := range []float64{0, 1, 5} {
		c.trustNear = near
		if got := c.goodwillWorth(&cfg); got != cfg.LoreValue+13.08 {
			t.Fatalf("the flat arm reads %v with %v trust near", got, near)
		}
	}
}

// The trust standing near is summed rather than counted, so that nothing here
// draws a line at "a friend". A stranger adds nothing, somebody halfway to
// being trusted adds half.
func TestTrustNearIsSummedAndNotCounted(t *testing.T) {
	cfg := DefaultConfig()
	w := NewWorld(cfg)
	c := &AIController{}
	p := &Perception{
		Cfg:  &cfg,
		Self: SelfView{Intelligence: 50, NoiseWeight: 1},
		Rand: w.rng,
		Others: []AgentView{
			{ID: 1, Affinity: 0},                    // a stranger
			{ID: 2, Affinity: cfg.AffinityTrust / 2}, // halfway
			{ID: 3, Affinity: cfg.AffinityTrust * 3}, // well past it
		},
	}
	c.survey(p)
	approx(t, c.trustNear, 1.5, 1e-9, "nought plus a half plus a full one")
}

// And the wanting reaches the hand-over that buys it: a body with nobody near
// prices the same gift higher than one standing among friends.
func TestABodyWithNobodyPricesAGiftHigher(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AllyValue = 36
	cfg.JudgementNoise, cfg.ChoiceNoise = 0, 0
	w := NewWorld(cfg)

	// The same body, the same hand, the same stranger to give to - twice,
	// with the only difference being who else is standing about.
	gift := func(company []AgentView) float64 {
		c := &AIController{}
		target := AgentView{ID: 9, Dist: 5, CarryRoom: true, LightRoom: true}
		p := &Perception{
			Cfg: &cfg,
			Self: SelfView{
				Intelligence: MaxAbility, NoiseWeight: 1,
				Vitality: 80, MaxVitality: 100, Hunger: 20,
				MaxSpeed: cfg.MaxSpeed, Carried: 1,
			},
			Others: append(append([]AgentView(nil), company...), target),
			Rand:   w.rng,
		}
		c.survey(p)
		c.addGive(p, &p.Others[len(p.Others)-1])
		best := 0.0
		for i := range c.opts {
			if c.opts[i].action.Kind == ActGive && c.opts[i].util > best {
				best = c.opts[i].util
			}
		}
		return best
	}

	alone := gift(nil)
	amongFriends := gift([]AgentView{
		{ID: 1, Affinity: cfg.AffinityTrust},
		{ID: 2, Affinity: cfg.AffinityTrust},
		{ID: 3, Affinity: cfg.AffinityTrust},
	})
	if alone <= 0 {
		t.Fatalf("a lonely body did not score a gift at all (%v)", alone)
	}
	if alone <= amongFriends {
		t.Fatalf("a gift is worth %v to a lonely body and %v to one among friends", alone, amongFriends)
	}
}
