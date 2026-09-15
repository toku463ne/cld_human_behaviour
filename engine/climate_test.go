package engine

import (
	"bytes"
	"testing"
)

// climateConfig is a world with a cold half (stage 85). The picture is read at
// the middle of each region, so this is four columns of regions: the left two
// ordinary, the right two cold.
func climateConfig() Config {
	cfg := testConfig()
	cfg.ClimateMap = []string{
		"..99",
		"..99",
		"..99",
	}
	cfg.ChillDrain = 0.5
	return cfg
}

// A world that says nothing about its weather has none, and asks nothing of
// anybody.
func TestAWorldWithNoClimateHasNone(t *testing.T) {
	w := NewWorld(testConfig())
	for i := range w.regions {
		if got := w.regions[i].Weather[WeatherChill]; got != 0 {
			t.Fatalf("region %d is at %v cold in a world with no weather", i, got)
		}
	}
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90,
		Genome: genomeOf(50, 50, 50)}))
	if got := w.chillOf(a); got != 0 {
		t.Fatalf("standing anywhere costs %v", got)
	}
}

// The picture puts the cold where it is drawn, at the grain of a region.
func TestTheWeatherIsWhereThePictureSaysItIs(t *testing.T) {
	cfg := climateConfig()
	w := NewWorld(cfg)
	warm := w.weatherAt(cfg.Width*0.1, cfg.Height*0.5, WeatherChill)
	cold := w.weatherAt(cfg.Width*0.9, cfg.Height*0.5, WeatherChill)
	if warm != 0 {
		t.Fatalf("the left of the world is at %v cold", warm)
	}
	if cold <= 0 {
		t.Fatalf("the right of the world is at %v cold", cold)
	}
	// And redrawing it takes the old weather away rather than leaving it
	// under the new (the editor's side, stage 22's standing).
	w.SetClimate(nil)
	if got := w.weatherAt(cfg.Width*0.9, cfg.Height*0.5, WeatherChill); got != 0 {
		t.Fatalf("after clearing the picture the cold is still %v", got)
	}
}

// Standing in it costs vitality every tick, whatever else the body is doing -
// including a satiated one that would otherwise be mending.
func TestTheColdTakesVitalityFromABodyThatIsOtherwiseMending(t *testing.T) {
	cfg := climateConfig()
	cfg.MutationRate = 0
	warmth, cold := 0.0, 0.0
	for i, x := range []float64{cfg.Width * 0.1, cfg.Width * 0.9} {
		w := NewWorld(cfg)
		id := w.addAgent(Agent{Maturity: 1, X: x, Y: cfg.Height * 0.5,
			Vitality: 50, Hunger: 10, Genome: genomeOf(50, 50, 50)})
		before := mustAgent(t, w, id).Vitality
		for k := 0; k < 20; k++ {
			w.metabolise()
		}
		got := mustAgent(t, w, id).Vitality - before
		if i == 0 {
			warmth = got
		} else {
			cold = got
		}
	}
	if warmth <= 0 {
		t.Fatalf("a fed body in the ordinary world gained %v", warmth)
	}
	if cold >= warmth {
		t.Fatalf("the cold cost nothing: %v against %v", cold, warmth)
	}
}

// And the body sees it coming. The cold goes with the metabolism rather than
// with what is hitting the body, so unlike a blow it is carried into the
// second window - which is the whole reason it was put there.
func TestTheColdIsCarriedIntoTheSecondWindow(t *testing.T) {
	cfg := climateConfig()
	w := NewWorld(cfg)
	// A whole body, so that the figures are not both saturated at certain
	// death before the comparison can be made.
	id := w.addAgent(Agent{Maturity: 1, X: cfg.Width * 0.9, Y: cfg.Height * 0.5,
		Vitality: 90, Hunger: 20, Genome: genomeOf(50, 50, 50)})
	s := w.selfView(mustAgent(t, w, id))
	if s.Chill <= 0 {
		t.Fatalf("a body in the cold feels %v of it", s.Chill)
	}
	cold := pressures(&w.cfg, &s, 90, 20, 0)

	// The same body, with the same amount taken from it as a blow instead.
	warm := s
	warm.Chill = 0
	blow := pressures(&w.cfg, &warm, 90, 20, s.Chill)
	if warm := pressures(&w.cfg, &warm, 90, 20, 0); cold.far <= warm.far {
		t.Fatalf("the cold reads as %v against %v in the ordinary world", cold.far, warm.far)
	}
	if cold.near != blow.near {
		t.Fatalf("over one window a blow and the cold differ: %v against %v", cold.near, blow.near)
	}
	if cold.far <= blow.far {
		t.Fatalf("the cold is not carried further than a blow: %v against %v", cold.far, blow.far)
	}
}

// What is in a hand answers nothing yet, and that is the seam the next stage
// fills: with it at nought, standing in the cold costs the whole of it.
func TestNothingAnswersTheWeatherYet(t *testing.T) {
	cfg := climateConfig()
	w := NewWorld(cfg)
	a := mustAgent(t, w, w.addAgent(Agent{Maturity: 1, X: cfg.Width * 0.9,
		Y: cfg.Height * 0.5, Vitality: 90, Genome: genomeOf(50, 50, 50)}))
	a.carried = append(a.carried, Food{Kind: FoodTrinket, Made: 1})
	if got := w.wardsOff(a, WeatherChill); got != 0 {
		t.Fatalf("something in a hand answers the weather: %v", got)
	}
	want := cfg.ChillDrain * w.weatherAt(a.X, a.Y, WeatherChill)
	if got := w.chillOf(a); got != want {
		t.Fatalf("the cold costs %v and the whole of it is %v", got, want)
	}
}

// It survives being saved and read back, which is stage 21's invariant: the
// weather is part of what the world is, not something the population carries.
func TestTheWeatherSurvivesASave(t *testing.T) {
	cfg := climateConfig()
	w := NewWorld(cfg)
	for i := 0; i < 50; i++ {
		w.Step()
	}
	var buf bytes.Buffer
	if err := w.Save(&buf); err != nil {
		t.Fatalf("save: %v", err)
	}
	back, err := Load(&buf)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for i := range w.regions {
		if got, want := back.regions[i].Weather, w.regions[i].Weather; got != want {
			t.Fatalf("region %d came back at %v instead of %v", i, got, want)
		}
	}
	if got := back.weatherAt(cfg.Width*0.9, cfg.Height*0.5, WeatherChill); got <= 0 {
		t.Fatalf("the cold half came back at %v", got)
	}
}

// A body can be made blind to the weather without being spared it (stage 86's
// control). What it pays is the same; what it knows is nothing.
func TestTheColdCanBeFeltOrNot(t *testing.T) {
	cfg := climateConfig()
	cfg.ChillKnown = false
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: cfg.Width * 0.9, Y: cfg.Height * 0.5,
		Vitality: 50, Hunger: 10, Genome: genomeOf(50, 50, 50)})
	if got := w.selfView(mustAgent(t, w, id)).Chill; got != 0 {
		t.Fatalf("a body that cannot feel the cold reads %v of it", got)
	}
	if got := w.chillOf(mustAgent(t, w, id)); got <= 0 {
		t.Fatalf("it is not being charged for it either: %v", got)
	}
	// And it still loses the vitality.
	before := mustAgent(t, w, id).Vitality
	for k := 0; k < 20; k++ {
		w.metabolise()
	}
	if got := mustAgent(t, w, id).Vitality; got >= before {
		t.Fatalf("twenty ticks in the cold and it is at %v against %v", got, before)
	}
}

// And what the measurement reports is where the bodies are against where the
// cold is, which is nought when nobody has responded to it.
func TestWhereTheColdIsAndWhereTheBodiesAre(t *testing.T) {
	cfg := climateConfig()
	w := NewWorld(cfg)
	// Two bodies in the warm, one in the cold: below the world's own average.
	for _, x := range []float64{0.1, 0.2, 0.9} {
		w.addAgent(Agent{Maturity: 1, X: cfg.Width * x, Y: cfg.Height * 0.5,
			Vitality: 90, Genome: genomeOf(50, 50, 50)})
	}
	got := w.Weather()
	if got.All <= 0 || got.All >= 1 {
		t.Fatalf("half a cold world averages %v", got.All)
	}
	if got.OnCold >= got.All || got.Gain >= 0 {
		t.Fatalf("two bodies out of three in the warm reads %v against %v", got.OnCold, got.All)
	}
	if got.Taken != 0 {
		t.Fatalf("nothing has happened yet and the cold has taken %v", got.Taken)
	}
}

// A body feels which way it gets colder, and that tells two headings apart -
// which is the one thing the cold underfoot cannot do (stage 86b).
func TestABodyFeelsWhichWayIsWarmer(t *testing.T) {
	cfg := climateConfig()
	cfg.ClimateMap = []string{"1369", "1369", "1369"} // colder towards the east
	cfg.ChillGradient = true
	w := NewWorld(cfg)
	id := w.addAgent(Agent{Maturity: 1, X: cfg.Width * 0.5, Y: cfg.Height * 0.5,
		Vitality: 90, Hunger: 30, Genome: genomeOf(50, 50, 50)})
	s := w.selfView(mustAgent(t, w, id))
	if s.ChillDX <= 0 {
		t.Fatalf("the world gets colder eastward and the slope reads %v", s.ChillDX)
	}
	if s.ChillDY != 0 {
		t.Fatalf("the world is the same north and south and the slope reads %v", s.ChillDY)
	}
	// East costs more than west, and by more than nothing.
	c := &AIController{}
	p := w.perceive(mustAgent(t, w, id))
	c.chillDX, c.chillDY, c.chillSpeed = p.Self.ChillDX, p.Self.ChillDY, p.Self.MaxSpeed
	c.opts, c.terms, c.tracing = nil, nil, true
	c.add(Action{Kind: ActMove, DX: 1, Effort: 0.5}, Utility{Ticks: 10})
	c.add(Action{Kind: ActMove, DX: -1, Effort: 0.5}, Utility{Ticks: 10})
	east, west := c.terms[0].Weather, c.terms[1].Weather
	if east <= 0 || west >= 0 || east != -west {
		t.Fatalf("east is charged %v and west %v", east, west)
	}
	// And with the sense off, the two headings are charged the same nothing -
	// which is the world stage 86 measured.
	blind := climateConfig()
	blind.ClimateMap = cfg.ClimateMap
	w2 := NewWorld(blind)
	id2 := w2.addAgent(Agent{Maturity: 1, X: blind.Width * 0.5, Y: blind.Height * 0.5,
		Vitality: 90, Hunger: 30, Genome: genomeOf(50, 50, 50)})
	if s2 := w2.selfView(mustAgent(t, w2, id2)); s2.ChillDX != 0 || s2.ChillDY != 0 {
		t.Fatalf("a body that cannot feel the slope reads %v, %v", s2.ChillDX, s2.ChillDY)
	}
	// The cold itself is still there, and still costs the same.
	if w2.chillOf(mustAgent(t, w2, id2)) != w.chillOf(mustAgent(t, w, id)) {
		t.Fatal("taking the sense away changed what the cold takes")
	}
}
