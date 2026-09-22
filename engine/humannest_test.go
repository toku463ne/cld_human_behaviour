package engine

import (
	"bytes"
	"testing"
)

// Nests people come out of (2026-09-22, TODO 20).

// twoVillages is a world with two places people come from, one at each end,
// and nobody else in it.
func twoVillages(cap int) Config {
	cfg := quietConfig()
	cfg.InitialPopulation, cfg.InitialEnemies = 0, 0
	cfg.HumanNests = []HumanNest{
		{Name: "west", Key: 'w', Rate: 100, Cap: cap},
		{Name: "east", Key: 'e', Rate: 100, Cap: cap},
	}
	cfg.HumanNestMap = []string{"w.e"}
	return cfg
}

func TestAWorldWithNoVillagesIsTheWorldItAlwaysWas(t *testing.T) {
	a := NewWorld(quietConfig())
	for i := 0; i < 400; i++ {
		a.Step()
	}
	cfg := quietConfig()
	cfg.HumanNestTicks, cfg.HumanNestLife = 1, 100000 // said, and painted nowhere
	b := NewWorld(cfg)
	for i := 0; i < 400; i++ {
		b.Step()
	}
	if a.draws.draws != b.draws.draws {
		t.Fatalf("%d draws against %d", a.draws.draws, b.draws.draws)
	}
	if len(a.agents) != len(b.agents) {
		t.Fatalf("%d bodies against %d", len(a.agents), len(b.agents))
	}
}

func TestPeopleComeOutOfTheirOwnVillage(t *testing.T) {
	w := NewWorld(twoVillages(0))
	for i := 0; i < 600; i++ {
		w.Step()
	}
	west, east := 0, 0
	for i := range w.agents {
		a := &w.agents[i]
		if a.Lineage == w.humanNestLine(0) {
			west++
			if a.X > w.cfg.Width/2 {
				// It may have walked; what is pinned is where it arrived,
				// and after six hundred ticks that is no longer where it is.
				continue
			}
		}
		if a.Lineage == w.humanNestLine(1) {
			east++
		}
	}
	if west == 0 || east == 0 {
		t.Fatalf("west sent %d and east %d", west, east)
	}
	if west == 0 || east == 0 || w.humanNestSent[0] == 0 || w.humanNestSent[1] == 0 {
		t.Fatalf("the nests sent %v", w.humanNestSent)
	}
}

func TestEachVillageHandsOutALineOfItsOwn(t *testing.T) {
	cfg := twoVillages(0)
	cfg.InitialPopulation = 4
	w := NewWorld(cfg)
	// The founders take 1..4 and the villages carry on from there, so nothing
	// collides and nothing had to be drawn to decide it.
	if a, b := w.humanNestLine(0), w.humanNestLine(1); a != 5 || b != 6 {
		t.Fatalf("the villages carry lines %d and %d", a, b)
	}
	w.Step()
	for i := range w.agents {
		a := &w.agents[i]
		if a.Lineage == 0 {
			t.Fatalf("#%d belongs to no line", a.ID)
		}
	}
}

func TestAVillageStopsWhenItsLifeIsSpent(t *testing.T) {
	cfg := twoVillages(0)
	cfg.HumanNests[0].Life = 300
	cfg.HumanNests[1].Life = 300
	w := NewWorld(cfg)
	for i := 0; i < 300; i++ {
		w.Step()
	}
	sent := append([]int(nil), w.humanNestSent...)
	if sent[0] == 0 {
		t.Fatal("nothing came out of it at all")
	}
	for i := 0; i < 500; i++ {
		w.Step()
	}
	if got := w.humanNestSent; got[0] != sent[0] || got[1] != sent[1] {
		t.Fatalf("a spent village went on sending: %v then %v", sent, got)
	}
	// What it sent is still living its life: a village that has stopped is
	// one that sends nobody, not one that takes anybody away.
	if len(w.agents) == 0 {
		t.Fatal("the people went with it")
	}
}

func TestAVillageKeepsItsLineUpAndNoMore(t *testing.T) {
	w := NewWorld(twoVillages(3))
	for i := 0; i < 2000; i++ {
		w.Step()
	}
	// It may be over the cap by birth - the cap is on what the world puts in
	// and never on what anybody has - but it must not be over it by sending.
	if sent := w.humanNestSent[0]; sent > 12 {
		t.Fatalf("a village capped at three sent %d", sent)
	}
	line := w.humanNestLine(0)
	w.killLine(line)
	before := w.humanNestSent[0]
	for i := 0; i < 300; i++ {
		w.Step()
	}
	if w.humanNestSent[0] <= before {
		t.Fatal("a village whose people are gone did not send another")
	}
}

// killLine takes every body of a line out, for a test that wants the nest to
// have somebody to replace.
func (w *World) killLine(line uint16) {
	for i := range w.agents {
		if w.agents[i].Alive && w.agents[i].Lineage == line {
			w.kill(&w.agents[i])
		}
	}
	w.removeDead()
}

func TestTheCapCountsChildrenToo(t *testing.T) {
	// A village supports a people, not a queue of arrivals: a line that is
	// keeping itself up needs nothing from the nest.
	cfg := twoVillages(2)
	w := NewWorld(cfg)
	line := w.humanNestLine(0)
	for i := 0; i < 4; i++ {
		a := w.randomAgent(SpeciesHuman)
		a.Lineage = line
		w.addAgent(a)
	}
	before := w.humanNestSent[0]
	for i := 0; i < 400; i++ {
		w.Step()
	}
	if w.humanNestSent[0] != before {
		t.Fatalf("a village whose line stands on its own still sent %d",
			w.humanNestSent[0]-before)
	}
}

func TestNobodyIsBornOfTheVillage(t *testing.T) {
	// Nothing here is a birth: no parents, nothing paid, and the birth
	// counter does not move.
	w := NewWorld(twoVillages(0))
	births := w.births
	for i := 0; i < 300; i++ {
		w.Step()
	}
	if w.humanNestSent[0] == 0 {
		t.Fatal("nothing came out of it")
	}
	if w.births != births {
		t.Fatalf("the village counted %d births", w.births-births)
	}
	for i := range w.agents {
		a := &w.agents[i]
		if a.ParentIDs[0] != 0 || a.ParentIDs[1] != 0 {
			t.Fatalf("#%d came out of a village with parents %v", a.ID, a.ParentIDs)
		}
		if a.Maturity < 1 {
			t.Fatalf("#%d came out of a village still growing", a.ID)
		}
	}
}

func TestTheWorldsCeilingStillHolds(t *testing.T) {
	cfg := twoVillages(0)
	cfg.MaxPopulation = 5
	cfg.HumanNests[0].Rate, cfg.HumanNests[1].Rate = 1, 1
	w := NewWorld(cfg)
	for i := 0; i < 300; i++ {
		w.Step()
	}
	if n := len(w.agents); n > cfg.MaxPopulation {
		t.Fatalf("%d bodies against a ceiling of %d", n, cfg.MaxPopulation)
	}
}

func TestWhatTheVillagesSaySurvivesASaveAndLoad(t *testing.T) {
	w := NewWorld(twoVillages(0))
	for i := 0; i < 400; i++ {
		w.Step()
	}
	var buf bytes.Buffer
	if err := w.Save(&buf); err != nil {
		t.Fatalf("Save: %v", err)
	}
	back, err := Load(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	a, b := w.HumanNests(), back.HumanNests()
	if len(a) != len(b) || len(a) == 0 {
		t.Fatalf("%d villages came back as %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("%+v came back as %+v", a[i], b[i])
		}
	}
}

func TestAMapMaySayWhereAPeopleStarts(t *testing.T) {
	data := []byte(`{"width":2,"height":1,"tilewidth":32,"tileheight":32,
	 "layers":[
	  {"type":"tilelayer","name":"ground","width":2,"height":1,"data":[3,3]},
	  {"type":"tilelayer","name":"folk","width":2,"height":1,"data":[1,2]}],
	 "tilesets":[{"firstgid":1,"tiles":[
	   {"id":0,"properties":[{"name":"human","type":"string","value":"hearth"}]},
	   {"id":1,"properties":[{"name":"human","type":"string","value":"haven"}]},
	   {"id":2,"properties":[{"name":"kind","type":"string","value":"flat"}]}]}]}`)
	m, err := ParseTiled(data)
	if err != nil {
		t.Fatalf("ParseTiled: %v", err)
	}
	if len(m.HumanNestMap) != 1 || m.HumanNestMap[0] != "ab" {
		t.Fatalf("the painting is %q", m.HumanNestMap)
	}
	if len(m.HumanNests) != 2 || m.HumanNests[0].Name != "hearth" {
		t.Fatalf("the villages are %+v", m.HumanNests)
	}
	// Names and places only - never figures.
	if m.HumanNests[0].Rate != 0 || m.HumanNests[0].Cap != 0 {
		t.Fatalf("the map carried figures: %+v", m.HumanNests[0])
	}
	cfg := quietConfig()
	cfg.HumanNests = []HumanNest{{Name: "hearth", Rate: 200, Cap: 4}}
	m.Apply(&cfg)
	if len(cfg.HumanNests) != 2 {
		t.Fatalf("%d rows, want 2", len(cfg.HumanNests))
	}
	if cfg.HumanNests[0].Rate != 200 || cfg.HumanNests[0].Key != 'a' {
		t.Fatalf("hearth is %+v", cfg.HumanNests[0])
	}
	if cfg.HumanNests[1].Name != "haven" {
		t.Fatalf("the second is %+v", cfg.HumanNests[1])
	}
}
