package engine

import (
	"bytes"
	"testing"
)

// The master of a nest (2026-09-22, TODO 19): called out from outside the
// engine, gone when it falls, and back in when it is left alone.

func nestWithMaster() Config {
	cfg := twoNests(0, nil, nil)
	cfg.BossBudget = 1.5
	cfg.EnemyHomeCost = 2
	cfg.EnemyKinds[0].Roam = 60
	return cfg
}

func TestAWorldWithNoMastersDrawsNothingForThem(t *testing.T) {
	a := NewWorld(twoNests(0, nil, nil))
	for i := 0; i < 300; i++ {
		a.Step()
	}
	b := NewWorld(twoNests(0, nil, nil))
	b.cfg.BossRouseTicks, b.cfg.NestQuietYears = 1, 1
	for i := 0; i < 300; i++ {
		b.Step()
	}
	if a.draws.draws != b.draws.draws {
		t.Fatalf("%d draws against %d", a.draws.draws, b.draws.draws)
	}
	if _, err := a.Rouse(0); err == nil {
		t.Fatal("a world with no masters called one out")
	}
}

func TestTheMasterIsBiggerAndComesOutOfItsOwnNest(t *testing.T) {
	w := NewWorld(nestWithMaster())
	id, err := w.Rouse(0)
	if err != nil {
		t.Fatalf("Rouse: %v", err)
	}
	b := mustAgent(t, w, id)
	if b.Species != SpeciesEnemy {
		t.Fatalf("the master is %v", b.Species)
	}
	nest := w.enemyKindCells[0][0]
	if distToCell(b.X, b.Y, nest.cell) > 0 {
		t.Fatalf("the master came out at (%.0f,%.0f) and its nest is at (%.0f,%.0f)",
			b.X, b.Y, nest.x, nest.y)
	}
	ordinary := w.randomAgent(SpeciesEnemy)
	if b.Budget() <= ordinary.Budget() {
		t.Fatalf("the master is %.0f and an ordinary one is %.0f", b.Budget(), ordinary.Budget())
	}
}

func TestAMasterAlreadyOutIsNotCalledTwice(t *testing.T) {
	w := NewWorld(nestWithMaster())
	if _, err := w.Rouse(0); err != nil {
		t.Fatalf("Rouse: %v", err)
	}
	if _, err := w.Rouse(0); err == nil {
		t.Fatal("the same nest sent out two masters")
	}
	// The other nest is its own business.
	if _, err := w.Rouse(1); err != nil {
		t.Fatalf("the second nest refused: %v", err)
	}
}

func TestANestThatLosesItsMasterGoesQuiet(t *testing.T) {
	cfg := nestWithMaster()
	cfg.NestQuietYears = 2
	cfg.EnemySpawnTicks = 1
	cfg.MaxEnemies = 200
	w := NewWorld(cfg)
	id, err := w.Rouse(0)
	if err != nil {
		t.Fatalf("Rouse: %v", err)
	}
	deaths := w.deaths
	w.kill(mustAgent(t, w, id))
	killed := w.deaths
	w.Step()
	if w.deaths != killed {
		t.Fatalf("the step added %d deaths of its own", w.deaths-killed)
	}
	if w.deaths != deaths+1 {
		t.Fatalf("deaths %d -> %d", deaths, w.deaths)
	}
	quiet := w.nestLives[0][0].quietTill - w.tick
	if want := 2 * cfg.TicksPerYear; quiet < want-2 || quiet > want {
		t.Fatalf("the nest is quiet for %d ticks, and two years is %d", quiet, want)
	}
	if _, err := w.Rouse(0); err == nil {
		t.Fatal("a nest with no master sent one out")
	}
	// And nothing arrives there while it is quiet: every arrival that
	// happens at all comes out of the other nest.
	for i := 0; i < 200; i++ {
		a := w.randomAgent(SpeciesEnemy)
		if distToCell(a.X, a.Y, w.enemyKindCells[0][0].cell) <= 0 {
			t.Fatalf("an arrival came out of a quiet nest at (%.0f,%.0f)", a.X, a.Y)
		}
	}
}

func TestTheQuietEndsAndTheNestAnswersAgain(t *testing.T) {
	cfg := nestWithMaster()
	cfg.NestQuietYears = 1
	w := NewWorld(cfg)
	id, _ := w.Rouse(0)
	w.kill(mustAgent(t, w, id))
	w.Step()
	w.tick = w.nestLives[0][0].quietTill
	if _, err := w.Rouse(0); err != nil {
		t.Fatalf("the nest never came back: %v", err)
	}
}

func TestAMasterLeftAloneWalksBackIn(t *testing.T) {
	cfg := nestWithMaster()
	cfg.BossRouseTicks = 10
	w := NewWorld(cfg)
	id, _ := w.Rouse(0)
	before := w.deaths
	// It is standing on its nest, and long enough has gone by.
	w.tick += cfg.BossRouseTicks
	w.nestsOfTick()
	if b := w.agentByID(id); b != nil && b.Alive {
		t.Fatal("the master stayed out")
	}
	if w.deaths != before {
		t.Fatalf("going back in counted as a death (%d -> %d)", before, w.deaths)
	}
	if w.countKind(FoodMeat) != 0 {
		t.Fatal("a master that walked home left a carcass")
	}
	if w.nestLives[0][0].quietTill > w.tick {
		t.Fatal("a nest whose master walked home went quiet")
	}
	if _, err := w.Rouse(0); err != nil {
		t.Fatalf("the nest would not send it out again: %v", err)
	}
}

func TestAMasterAwayFromItsNestStaysOut(t *testing.T) {
	cfg := nestWithMaster()
	cfg.BossRouseTicks = 10
	w := NewWorld(cfg)
	id, _ := w.Rouse(0)
	b := mustAgent(t, w, id)
	b.X, b.Y = w.cfg.Width/2, w.cfg.Height/2
	w.tick += cfg.BossRouseTicks * 5
	w.nestsOfTick()
	if b := w.agentByID(id); b == nil || !b.Alive {
		t.Fatal("a master out in the world was taken back in")
	}
}

func TestOnlyTheMasterOfThatNestIsTakenBackIn(t *testing.T) {
	// Retiring is Rouse's mirror and nothing else: no ordinary body can be
	// taken out of the world by standing somewhere.
	cfg := nestWithMaster()
	cfg.BossRouseTicks = 1
	w := NewWorld(cfg)
	nest := w.enemyKindCells[0][0]
	a := w.randomAgent(SpeciesEnemy)
	a.X, a.Y = nest.x, nest.y
	id := w.addAgent(a)
	w.tick += 100
	w.nestsOfTick()
	if b := w.agentByID(id); b == nil || !b.Alive {
		t.Fatal("an ordinary beast standing on a nest was taken into it")
	}
}

func TestWhatTheNestsSayIsWhatRouseTakes(t *testing.T) {
	w := NewWorld(nestWithMaster())
	nests := w.EnemyNests()
	if len(nests) != 2 {
		t.Fatalf("%d nests", len(nests))
	}
	for i, n := range nests {
		if n.ID != i {
			t.Fatalf("nest %d calls itself %d", i, n.ID)
		}
	}
	id, err := w.Rouse(nests[1].ID)
	if err != nil {
		t.Fatalf("Rouse: %v", err)
	}
	after := w.EnemyNests()
	if after[1].Boss != id {
		t.Fatalf("the nest says its master is #%d and Rouse made #%d", after[1].Boss, id)
	}
	if after[0].Boss != 0 {
		t.Fatalf("the other nest grew a master (#%d)", after[0].Boss)
	}
	w.kill(mustAgent(t, w, id))
	w.Step()
	if left := w.EnemyNests()[1].Quiet; left <= 0 {
		t.Fatalf("the nest says it is quiet for %d ticks", left)
	}
}

func TestTheMastersSurviveASaveAndLoad(t *testing.T) {
	cfg := nestWithMaster()
	w := NewWorld(cfg)
	id, _ := w.Rouse(0)
	w.kill(mustAgent(t, w, id))
	w.Step()
	if _, err := w.Rouse(1); err != nil {
		t.Fatalf("Rouse: %v", err)
	}
	var buf bytes.Buffer
	if err := w.Save(&buf); err != nil {
		t.Fatalf("Save: %v", err)
	}
	back, err := Load(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if a, b := w.EnemyNests(), back.EnemyNests(); a[0].Quiet != b[0].Quiet || a[1].Boss != b[1].Boss {
		t.Fatalf("%+v came back as %+v", a, b)
	}
	if _, err := back.Rouse(0); err == nil {
		t.Fatal("a quiet nest woke up on being loaded")
	}
}

func TestAMapMaySayHowLongANestStaysQuiet(t *testing.T) {
	data := []byte(`{"width":2,"height":1,"tilewidth":32,"tileheight":32,
	 "layers":[
	  {"type":"tilelayer","name":"ground","width":2,"height":1,"data":[3,3]},
	  {"type":"tilelayer","name":"nests","width":2,"height":1,"data":[1,2]}],
	 "tilesets":[{"firstgid":1,"tiles":[
	   {"id":0,"properties":[{"name":"enemy","type":"string","value":"brute"},
	                         {"name":"quiet","type":"float","value":0.6}]},
	   {"id":1,"properties":[{"name":"enemy","type":"string","value":"brute"}]},
	   {"id":2,"properties":[{"name":"kind","type":"string","value":"flat"}]}]}]}`)
	m, err := ParseTiled(data)
	if err != nil {
		t.Fatalf("ParseTiled: %v", err)
	}
	if len(m.NestQuiet) != 1 || m.NestQuiet[0] != "3." {
		t.Fatalf("the quiets are %q", m.NestQuiet)
	}
	cfg := quietConfig()
	cfg.EnemyKinds = []EnemyKind{{Name: "brute", Share: 1}}
	cfg.BossBudget, cfg.NestQuietYears = 1.5, 5
	m.Apply(&cfg)
	w := NewWorld(cfg)
	if len(w.enemyKindCells) == 0 || len(w.enemyKindCells[0]) != 2 {
		t.Fatalf("the map made %v", w.enemyKindCells)
	}
	// Three fifths of five years for the painted one, five years for the
	// other: the map carries the proportion and Config the figure (#133).
	if got, want := w.quietTicks(w.enemyKindCells[0][0]), 3*cfg.TicksPerYear; got != want {
		t.Fatalf("the painted nest is quiet for %d, want %d", got, want)
	}
	if got, want := w.quietTicks(w.enemyKindCells[0][1]), 5*cfg.TicksPerYear; got != want {
		t.Fatalf("the unpainted nest is quiet for %d, want %d", got, want)
	}
}

func TestWhatAnAuthorPaintedSurvivesASaveAndLoad(t *testing.T) {
	// Until 2026-09-22 it did not: Load rebuilt the ground and the regions
	// and left the painted richness, plant sorts and nests behind, so a saved
	// world came back a different one without saying so.
	cfg := twoNests(0, nil, nil)
	cfg.RichMap = []string{"91"}
	cfg.PlantKinds = []PlantKind{{Name: "berry", Key: 'b'}}
	cfg.PlantKindMap = []string{"b."}
	w := NewWorld(cfg)
	for i := 0; i < 50; i++ {
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
	if got, want := len(back.EnemyNests()), len(w.EnemyNests()); got != want {
		t.Fatalf("%d nests came back as %d", want, got)
	}
	if back.rich == nil {
		t.Fatal("the painted richness did not come back")
	}
	if len(back.plantKindKeys) != len(w.plantKindKeys) {
		t.Fatalf("the painted plant sorts came back as %v", back.plantKindKeys)
	}
}
