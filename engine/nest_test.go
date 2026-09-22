package engine

import (
	"math"
	"testing"
)

// The nests carry their own figures (2026-09-22, TODO 18): how many of a
// sort's arrivals come out of each one, and how many it keeps nearby before it
// sends no more.

// twoNests is a world with one sort and two nests, one at each end.
func twoNests(cap float64, rate, caps []string) Config {
	cfg := quietConfig()
	cfg.InitialEnemies = 0
	cfg.EnemyKinds = []EnemyKind{{Name: "brute", Share: 1, Key: 'b'}}
	cfg.EnemyKindMap = []string{"b.b"}
	cfg.NestRateMap = rate
	cfg.NestCap, cfg.NestCapMap = cap, caps
	return cfg
}

func TestTheMapSaysWhichNestSendsThemOut(t *testing.T) {
	// One nest painted '1' against one painted '9': a tenth against nine
	// tenths, which is the whole of what a rate means.
	w := NewWorld(twoNests(0, []string{"1.9"}, nil))
	west, east := 0.0, 0.0
	for i := 0; i < 2000; i++ {
		a := w.randomAgent(SpeciesEnemy)
		if a.X < w.cfg.Width/2 {
			west++
		} else {
			east++
		}
	}
	share := east / (east + west)
	if math.Abs(share-0.9) > 0.03 {
		t.Fatalf("the nest painted 9 against 1 took %.2f of the arrivals", share)
	}
}

func TestAnUnweightedMapDrawsItsNestsAsItAlwaysDid(t *testing.T) {
	// The draw itself must not change where nothing is painted: the same call
	// taking the same value out of the same source, or every world with a
	// nest on it is a different world from 2026-09-22.
	a := NewWorld(twoNests(0, nil, nil))
	b := NewWorld(twoNests(0, nil, nil))
	for i := 0; i < 200; i++ {
		x, y := a.randomAgent(SpeciesEnemy), b.randomAgent(SpeciesEnemy)
		if x.X != y.X || x.Y != y.Y {
			t.Fatalf("the same world drew (%.2f,%.2f) and (%.2f,%.2f)", x.X, x.Y, y.X, y.Y)
		}
	}
	if a.draws.draws != b.draws.draws {
		t.Fatalf("%d draws against %d", a.draws.draws, b.draws.draws)
	}
}

func TestANestPaintedNoughtSendsNobody(t *testing.T) {
	w := NewWorld(twoNests(0, []string{"0.5"}, nil))
	for i := 0; i < 500; i++ {
		if a := w.randomAgent(SpeciesEnemy); a.X < w.cfg.Width/2 {
			t.Fatalf("a nest painted 0 sent one out, at x=%.0f", a.X)
		}
	}
}

func TestAFullNestSendsNoMore(t *testing.T) {
	// Two bodies within roam of the nest and the map says two: the world
	// stops letting them in, and says so rather than putting them elsewhere.
	cfg := twoNests(2, nil, []string{"5.0"})
	cfg.EnemyKinds[0].Roam = 60
	cfg.EnemySpawnTicks = 1
	cfg.MaxEnemies = 50
	w := NewWorld(cfg)
	// The eastern nest is painted 0, which is a nest that holds nobody, so
	// every arrival that happens at all comes out of the western one.
	for i := 0; i < 400; i++ {
		w.spawnEnemyOfTick()
		w.tick++
	}
	n := 0
	for i := range w.agents {
		if w.agents[i].Alive && w.agents[i].Species == SpeciesEnemy {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("a nest capped at two holds %d", n)
	}
	if use := w.Nesting(); use.Stopped <= 0 {
		t.Fatalf("nothing was refused, so the cap never fired (stopped %.2f)", use.Stopped)
	}
}

func TestACapDoesNotStopAnybodyBreeding(t *testing.T) {
	// The asymmetry is the rule, not a limitation of it: a full nest lets in
	// nobody from outside and the beasts already there go on as they were.
	cfg := twoNests(1, nil, []string{"5.5"})
	cfg.EnemyKinds[0].Roam = 60
	w := NewWorld(cfg)
	nest := w.enemyKindCells[0][0]
	for i := 0; i < 5; i++ {
		a := w.randomAgent(SpeciesEnemy)
		a.X, a.Y = nest.x, nest.y
		w.addAgent(a)
	}
	if got := w.nestCrowd(0, nest); got != 5 {
		t.Fatalf("%v stand by a nest capped at one, and the world says %v", 5, got)
	}
	if w.nestHasRoom(0, nest) {
		t.Fatal("a nest with five bodies on it is not full")
	}
}

func TestTheAmbientShareIgnoresTheNests(t *testing.T) {
	// The way out of a country whose nests are all quiet or full: some of the
	// arrivals come in from outside the map however the nests stand.
	cfg := twoNests(0, nil, nil)
	cfg.EnemyAmbientShare = 1
	w := NewWorld(cfg)
	middle := 0
	for i := 0; i < 500; i++ {
		a := w.randomAgent(SpeciesEnemy)
		if a.X > w.cfg.Width*0.4 && a.X < w.cfg.Width*0.6 {
			middle++
		}
	}
	if middle == 0 {
		t.Fatal("every ambient arrival still landed on a nest")
	}
}

func TestNoAmbientShareDrawsNothingForIt(t *testing.T) {
	a := NewWorld(twoNests(0, nil, nil))
	for i := 0; i < 100; i++ {
		a.randomAgent(SpeciesEnemy)
	}
	b := NewWorld(twoNests(0, nil, nil))
	b.cfg.EnemyAmbientShare = 0
	for i := 0; i < 100; i++ {
		b.randomAgent(SpeciesEnemy)
	}
	if a.draws.draws != b.draws.draws {
		t.Fatalf("%d draws against %d", a.draws.draws, b.draws.draws)
	}
}

func TestWhatTheNestsHoldIsReadOnly(t *testing.T) {
	cfg := twoNests(4, nil, []string{"5.5"})
	w := NewWorld(cfg)
	for i := 0; i < 200; i++ {
		w.Step()
	}
	before := w.draws.draws
	first := w.Nesting()
	second := w.Nesting()
	if w.draws.draws != before {
		t.Fatalf("Nesting drew %d values", w.draws.draws-before)
	}
	if first != second {
		t.Fatalf("%+v then %+v", first, second)
	}
	if first.Nests != 2 {
		t.Fatalf("the map painted two nests and the world sees %d", first.Nests)
	}
	if first.Room <= 0 || first.Room >= 1 {
		t.Fatalf("two nests cover %.2f of the map", first.Room)
	}
}

func TestAMapMaySayHowOftenAndHowManyPerNest(t *testing.T) {
	// The tile carries the proportion and Config the figure it is a
	// proportion of (#133), read off the layer the nests themselves are on.
	data := []byte(`{"width":2,"height":1,"tilewidth":32,"tileheight":32,
	 "layers":[
	  {"type":"tilelayer","name":"ground","width":2,"height":1,"data":[3,3]},
	  {"type":"tilelayer","name":"nests","width":2,"height":1,"data":[1,2]}],
	 "tilesets":[{"firstgid":1,"tiles":[
	   {"id":0,"properties":[{"name":"enemy","type":"string","value":"brute"},
	                         {"name":"rate","type":"float","value":1.8},
	                         {"name":"cap","type":"float","value":0.4}]},
	   {"id":1,"properties":[{"name":"enemy","type":"string","value":"brute"}]},
	   {"id":2,"properties":[{"name":"kind","type":"string","value":"flat"}]}]}]}`)
	m, err := ParseTiled(data)
	if err != nil {
		t.Fatalf("ParseTiled: %v", err)
	}
	if len(m.NestRate) != 1 || m.NestRate[0] != "9." {
		t.Fatalf("the rates are %q", m.NestRate)
	}
	if len(m.NestCap) != 1 || m.NestCap[0] != "2." {
		t.Fatalf("the caps are %q", m.NestCap)
	}
	cfg := quietConfig()
	m.Apply(&cfg)
	if len(cfg.NestRateMap) != 1 || len(cfg.NestCapMap) != 1 {
		t.Fatalf("the map handed over %q and %q", cfg.NestRateMap, cfg.NestCapMap)
	}
}

func TestAMapThatSaysNothingAboutItsNestsPaintsNothing(t *testing.T) {
	data := []byte(`{"width":2,"height":1,"tilewidth":32,"tileheight":32,
	 "layers":[
	  {"type":"tilelayer","name":"ground","width":2,"height":1,"data":[2,2]},
	  {"type":"tilelayer","name":"nests","width":2,"height":1,"data":[1,1]}],
	 "tilesets":[{"firstgid":1,"tiles":[
	   {"id":0,"properties":[{"name":"enemy","type":"string","value":"brute"}]},
	   {"id":1,"properties":[{"name":"kind","type":"string","value":"flat"}]}]}]}`)
	m, err := ParseTiled(data)
	if err != nil {
		t.Fatalf("ParseTiled: %v", err)
	}
	if m.NestRate != nil || m.NestCap != nil {
		t.Fatalf("a map saying nothing painted %q and %q", m.NestRate, m.NestCap)
	}
}
