package engine

import (
	"bytes"
	"testing"
)

func TestEveryFounderStartsALineOfItsOwn(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 7
	cfg.InitialPopulation = 20
	cfg.InitialEnemies = 3
	w := NewWorld(cfg)
	seen := map[uint16]bool{}
	humans, enemies := 0, 0
	for _, a := range w.Agents() {
		switch a.Species {
		case SpeciesHuman:
			humans++
			if a.Lineage == 0 {
				t.Fatal("a founder has no line")
			}
			if seen[a.Lineage] {
				t.Fatalf("two founders share line %d", a.Lineage)
			}
			seen[a.Lineage] = true
		case SpeciesEnemy:
			enemies++
			if a.Lineage != 0 {
				t.Fatalf("an enemy carries line %d", a.Lineage)
			}
		}
	}
	if humans != 20 || enemies != 3 {
		t.Fatalf("%d humans and %d enemies", humans, enemies)
	}
}

func TestAChildTakesItsMothersLine(t *testing.T) {
	cfg := quietConfig()
	w := NewWorld(cfg)
	for _, order := range []struct{ first, second Sex }{{Male, Female}, {Female, Male}} {
		pa := &Agent{Maturity: 1, Vitality: 90, Genome: filledGenome(30),
			Sex: order.first, Lineage: 11}
		pb := &Agent{Maturity: 1, Vitality: 90, Genome: filledGenome(30),
			Sex: order.second, Lineage: 22}
		want := uint16(22)
		if order.first == Female {
			want = 11
		}
		before := len(w.newborns)
		w.tryBirth(pa, pb)
		if len(w.newborns) != before+1 {
			t.Fatal("no child")
		}
		if got := w.newborns[len(w.newborns)-1].Lineage; got != want {
			t.Fatalf("the child is line %d, want its mother's %d", got, want)
		}
	}
}

func TestTheLineTagTakesNothingFromTheRandomSource(t *testing.T) {
	// It is a label and it has to be free: a draw here would move every world
	// that has ever had a birth in it. Two worlds built the same way, so
	// their sources stand at the same place, and one birth each - the parents
	// differing only in whether they carry a line.
	cfg := quietConfig()
	withTags, without := NewWorld(cfg), NewWorld(cfg)
	pa := &Agent{Maturity: 1, Vitality: 90, Genome: filledGenome(30), Sex: Female, Lineage: 3}
	pb := &Agent{Maturity: 1, Vitality: 90, Genome: filledGenome(30), Sex: Male, Lineage: 4}
	pc := &Agent{Maturity: 1, Vitality: 90, Genome: filledGenome(30), Sex: Female}
	pd := &Agent{Maturity: 1, Vitality: 90, Genome: filledGenome(30), Sex: Male}

	at := withTags.draws.draws
	withTags.tryBirth(pa, pb)
	tagged := withTags.draws.draws - at

	at = without.draws.draws
	without.tryBirth(pc, pd)
	plain := without.draws.draws - at

	if tagged != plain {
		t.Fatalf("a birth with lines took %d draws and one without took %d", tagged, plain)
	}
	if withTags.newborns[0].Lineage != 3 {
		t.Fatalf("the child is line %d, want 3", withTags.newborns[0].Lineage)
	}
}

func TestALineIsCountedWhereItStands(t *testing.T) {
	cfg := quietConfig()
	cfg.RegionCols, cfg.RegionRows = 2, 1
	w := NewWorld(cfg)
	// One line in both halves, one in a single half.
	for _, p := range []struct {
		x   float64
		tag uint16
	}{{50, 1}, {350, 1}, {60, 2}, {70, 2}} {
		a := w.randomAgent(SpeciesHuman)
		a.X, a.Y = p.x, cfg.Height/2
		a.Lineage = p.tag
		w.addAgent(a)
	}
	got := w.Lineages()
	if got.Living != 2 {
		t.Fatalf("%d lines alive, want 2", got.Living)
	}
	if got.InTwo != 0.5 {
		t.Fatalf("%.2f of the lines are in two blocks, want 0.5", got.InTwo)
	}
	if got.Regions != 1.5 {
		t.Fatalf("the mean line stands in %.2f blocks, want 1.5", got.Regions)
	}
	if got.Biggest != 0.5 {
		t.Fatalf("the biggest line is %.2f of the living, want 0.5", got.Biggest)
	}
}

func TestALineSurvivesSavingAndLoading(t *testing.T) {
	cfg := quietConfig()
	cfg.InitialPopulation = 5
	w := NewWorld(cfg)
	var buf bytes.Buffer
	if err := w.Save(&buf); err != nil {
		t.Fatal(err)
	}
	back, err := Load(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range w.Agents() {
		if b := back.Agents()[i]; b.Lineage != a.Lineage {
			t.Fatalf("body %d came back as line %d, want %d", i, b.Lineage, a.Lineage)
		}
		if a.Lineage == 0 {
			t.Fatal("a founder has no line to lose")
		}
	}
}

func TestALineMayBeHandedDownOneAtATime(t *testing.T) {
	cfg := quietConfig()
	cfg.LineageRule = LineageChain
	w := NewWorld(cfg)
	motherID := w.addAgent(Agent{
		Maturity: 1, Vitality: 90, Genome: filledGenome(30), Sex: Female, Lineage: 5})
	fatherID := w.addAgent(Agent{
		Maturity: 1, Vitality: 90, Genome: filledGenome(30), Sex: Male})

	// Fetched by ID every time: committing a birth appends to the population
	// and may move it, so a pointer held across one is somebody else's.
	born := func() int {
		t.Helper()
		mother, father := mustAgent(t, w, motherID), mustAgent(t, w, fatherID)
		mother.Vitality, father.Vitality = 90, 90
		before := len(w.newborns)
		w.tryBirth(mother, father)
		if len(w.newborns) != before+1 {
			t.Fatal("no child")
		}
		w.commitNewborns()
		return w.agents[len(w.agents)-1].ID
	}

	firstID := born()
	if got := mustAgent(t, w, firstID).Lineage; got != 5 {
		t.Fatalf("the eldest is line %d, want its mother's 5", got)
	}
	secondID := born()
	if got := mustAgent(t, w, secondID).Lineage; got != 0 {
		t.Fatalf("a second child took line %d while the eldest was alive", got)
	}
	// The eldest dies and the line is free again.
	w.kill(mustAgent(t, w, firstID))
	thirdID := born()
	if got := mustAgent(t, w, thirdID).Lineage; got != 5 {
		t.Fatalf("with the eldest gone the next is line %d, want 5", got)
	}
}

