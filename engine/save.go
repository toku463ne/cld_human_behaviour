package engine

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
)

// Saving a world and reading it back (stage 21).
//
// Three things the project wants turn out to be one thing (#48): loading a
// population raised somewhere else, handing a world that has been run for a
// while to somebody else, and putting a genius where you want one. All three
// are "write the state out and read it back", so there is one mechanism here
// and not three.
//
// The format is JSON. It is not the network protocol - that gets designed on
// top of what this turns out to need - and it is readable on purpose: a world
// somebody is going to edit by hand before handing on is worth being able to
// look at. Go writes float64 with enough digits to read back exactly, so a
// readable format costs nothing in fidelity.
//
// The acceptance condition is the fingerprint test's: save a world, load it,
// run both for the same number of ticks, and the two must not differ by one
// bit. That is also what proves nothing was left out - a field forgotten here
// shows up as a divergence there, which is why the test is written that way
// rather than by comparing the files.

// The randomness is the one piece that cannot simply be copied.
//
// Everything else in this world is a number in a struct, but the state of the
// generator lives inside math/rand and this version of Go does not hand it
// out - rngSource implements neither BinaryMarshaler nor GobEncoder. Two ways
// round that were rejected before this one: swapping in a generator that can
// be serialised would change every number every recorded measurement was made
// with, and re-deriving the state by arithmetic means copying 607 constants
// out of the standard library and hoping they stay put.
//
// What is saved instead is the seed and how many numbers have been taken out
// of it, and loading replays that many. It is exact - Int63 and Uint64 advance
// the source identically, so the count is all that matters - and it is cheap:
// fifty million draws replay in about two tenths of a second, which is more
// than a hundred thousand ticks' worth.

// countingSource wraps the world's generator to count what has been taken from
// it. It forwards Source64 so that the stream is the one it always was: this
// must not change a single result, and the fingerprint test is what says so.
type countingSource struct {
	src   rand.Source64
	draws uint64
}

func (c *countingSource) Int63() int64 {
	c.draws++
	return c.src.Int63()
}

func (c *countingSource) Uint64() uint64 {
	c.draws++
	return c.src.Uint64()
}

func (c *countingSource) Seed(seed int64) {
	c.src.Seed(seed)
	c.draws = 0
}

// newCountingRand is how a world gets its generator.
func newCountingRand(seed int64) (*rand.Rand, *countingSource) {
	c := &countingSource{src: rand.NewSource(seed).(rand.Source64)}
	return rand.New(c), c
}

// replayTo winds a fresh source forward to where a saved one had got to.
func replayTo(seed int64, draws uint64) (*rand.Rand, *countingSource) {
	r, c := newCountingRand(seed)
	for i := uint64(0); i < draws; i++ {
		c.src.Uint64()
	}
	c.draws = draws
	return r, c
}

// --- what a saved world is made of ------------------------------------------

// SaveFormat is the version of the layout below. A file that does not match is
// refused rather than read as though it did: a snapshot is only worth anything
// if what comes back is what went in.
const SaveFormat = 1

type snapshot struct {
	Format int    `json:"format"`
	Config Config `json:"config"`

	Seed  int64  `json:"seed"`
	Draws uint64 `json:"draws"`

	Tick      int     `json:"tick"`
	FoodAccum float64 `json:"foodAccum"`

	NextAgentID int `json:"nextAgentID"`
	NextFoodID  int `json:"nextFoodID"`

	Agents  []agentSnap `json:"agents"`
	Foods   []Food      `json:"foods"`
	Regions []region    `json:"regions"`

	FoodWeight   float64    `json:"foodWeight"`
	PendingSeeds []seedSnap `json:"pendingSeeds,omitempty"`

	Counters counterSnap `json:"counters"`
}

// counterSnap is the world's running totals. They are what Stats reads, so a
// world that came back without them would be the same world telling a
// different story about itself.
type counterSnap struct {
	Births, Evaded, Hunts, HuntParty, JointHunts int
	FirstSights                                  int
	FirstSightError                              float64
	FirstSightsLearned                           int
	FirstSightErrorLearned                       float64
	FirstSightErrorFlat                          float64
	FirstSightErrorFixed                         float64
	Geniuses, GreatGeniuses                      int
	Deaths, Kills, AgingDeaths, DrownDeaths      int
	DrownWitnesses                               int
	KillWitnesses, AvengeWitnesses, Observes     int
	KillLessons                                  int
	Calls, Joins                                 int
	Matured, ChildDeaths, Fights                 int
	MaxGeneration                                int
	BlowsSeen, BlowsAnswered                     int
	Courtships, CourtshipsAccepted               int
	Flees, Escapes                               int
	Exchanges, HintsCopied                       int
}

type seedSnap struct {
	X, Y  float64
	Genes plantGenes
}

// agentSnap is one body with everything about it, including the parts no
// interface ever sees: what it believes, who it remembers, and where it had
// got to in whatever it was doing.
//
// The controller is not here. A world is a world whoever is playing it, and
// which node a person had hold of belongs to the game rather than to the
// state (stage 19's line). A loaded world runs entirely on the utility
// formula until somebody takes a body over again.
type agentSnap struct {
	Agent

	FrailTicks         int
	CourtStartTick     int
	ReproReady         bool
	LastDecisionTick   int
	VitalityAtDecision float64
	NeedsDecision      bool
	PendingTrigger     Trigger
	AttackerID         int
	LastAttackTick     int

	Lore      loreSnap
	Hints     []Hint
	HintSlots int

	SawFood, SawMate bool
	Chronotype       float64

	Seed      plantGenes
	SeedDueAt int

	RecentFood [NumFoodKinds]float64
	DietTick   int

	Regions     []regionSnap
	TimesTaught int
	Looks       looksSnap

	EngageID, EngageStart, EngageLast int
	OpenUntil                         int
	HitBy                             map[int]int
	EffortSpent                       float64
	ActionTicks                       int

	Opinions      map[int]opinionSnap
	NoSpareMemory bool
	MemoryTick    int
	MemoryUsed    int

	CourtedBy   int
	CourtedTick int
	LastCourt   CourtView
	Rejected    map[int]int
}

type loreSnap struct {
	RetaliationMean, RetaliationN float64
	AcceptMean, AcceptN           float64
	RiskWeight                    float64
	CompetitionWeight             float64
	ShockRisk                     float64
}

type regionSnap struct {
	Seen, N, LogN float64
	LastTick      int
	Cost          float64
	Danger        float64
}

type looksSnap struct {
	N, Sx, Sy, Sxx, Sxy float64
}

type opinionSnap struct {
	Risk, Affinity float64
	LastTick       int
	Strength       float64
	Variance       float64
	Samples        int
}

// --- writing it out ----------------------------------------------------------

// Save writes the whole world. It is meant to be called between ticks: what is
// written is the state Step leaves behind, and the buffers a tick uses while it
// is running (the blows queued, the newborns waiting to be added) are not part
// of it.
func (w *World) Save(out io.Writer) error {
	s := snapshot{
		Format:      SaveFormat,
		Config:      w.cfg,
		Seed:        w.cfg.Seed,
		Draws:       w.draws.draws,
		Tick:        w.tick,
		FoodAccum:   w.foodAccum,
		NextAgentID: w.nextAgentID,
		NextFoodID:  w.nextFoodID,
		Foods:       w.foods,
		Regions:     w.regions,
		FoodWeight:  w.foodWeight,
		Counters: counterSnap{
			Births: w.births, Evaded: w.evaded, Hunts: w.hunts,
			HuntParty: w.huntParty, JointHunts: w.jointHunts,
			FirstSights: w.firstSights, FirstSightError: w.firstSightError,
			FirstSightsLearned: w.firstSightsLearned, FirstSightErrorLearned: w.firstSightErrorLearned,
			FirstSightErrorFlat: w.firstSightErrorFlat, FirstSightErrorFixed: w.firstSightErrorFixed,
			Geniuses: w.geniuses, GreatGeniuses: w.greatGeniuses,
			Deaths: w.deaths, Kills: w.kills, AgingDeaths: w.agingDeaths,
			DrownDeaths: w.drownDeaths, DrownWitnesses: w.drownWitnesses,
			KillWitnesses: w.killWitnesses, AvengeWitnesses: w.avengeWitnesses,
			KillLessons: w.killLessons,
			Calls:       w.calls, Joins: w.joins,
			Observes: w.observes,
			Matured:  w.matured, ChildDeaths: w.childDeaths, Fights: w.fights,
			MaxGeneration: w.maxGeneration,
			BlowsSeen:     w.blowsSeen, BlowsAnswered: w.blowsAnswered,
			Courtships: w.courtships, CourtshipsAccepted: w.courtshipsAccepted,
			Flees: w.flees, Escapes: w.escapes,
			Exchanges: w.exchanges, HintsCopied: w.hintsCopied,
		},
	}
	for i := range w.pendingSeeds {
		p := &w.pendingSeeds[i]
		s.PendingSeeds = append(s.PendingSeeds, seedSnap{X: p.x, Y: p.y, Genes: p.genes})
	}
	for i := range w.agents {
		s.Agents = append(s.Agents, snapAgent(&w.agents[i]))
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", " ")
	return enc.Encode(&s)
}

func snapAgent(a *Agent) agentSnap {
	out := agentSnap{
		Agent:              *a,
		FrailTicks:         a.frailTicks,
		CourtStartTick:     a.courtStartTick,
		ReproReady:         a.reproReady,
		LastDecisionTick:   a.lastDecisionTick,
		VitalityAtDecision: a.vitalityAtDecision,
		NeedsDecision:      a.needsDecision,
		PendingTrigger:     a.pendingTrigger,
		AttackerID:         a.attackerID,
		LastAttackTick:     a.lastAttackTick,
		Lore: loreSnap{
			RetaliationMean: a.lore.retaliation.mean, RetaliationN: a.lore.retaliation.n,
			AcceptMean: a.lore.accept.mean, AcceptN: a.lore.accept.n,
			RiskWeight:        a.lore.riskWeight,
			CompetitionWeight: a.lore.competitionWeight,
			ShockRisk:         a.lore.shockRisk,
		},
		Hints:       a.hints,
		HintSlots:   a.hintSlots,
		SawFood:     a.sawFood,
		SawMate:     a.sawMate,
		Chronotype:  a.chronotype,
		Seed:        a.seed,
		SeedDueAt:   a.seedDueAt,
		RecentFood:  a.recentFood,
		DietTick:    a.dietTick,
		TimesTaught: a.timesTaught,
		Looks: looksSnap{N: a.looks.n, Sx: a.looks.sx, Sy: a.looks.sy,
			Sxx: a.looks.sxx, Sxy: a.looks.sxy},
		EngageID: a.engageID, EngageStart: a.engageStart, EngageLast: a.engageLast,
		OpenUntil:     a.openUntil,
		HitBy:         a.hitBy,
		EffortSpent:   a.effortSpent,
		ActionTicks:   a.actionTicks,
		NoSpareMemory: a.noSpareMemory,
		MemoryTick:    a.memoryTick,
		MemoryUsed:    a.memoryUsed,
		CourtedBy:     a.courtedBy,
		CourtedTick:   a.courtedTick,
		LastCourt:     a.lastCourt,
		Rejected:      a.rejected,
	}
	for i := range a.regions {
		r := &a.regions[i]
		out.Regions = append(out.Regions, regionSnap{Seen: r.seen, N: r.n, LogN: r.logN,
			LastTick: r.lastTick, Cost: r.cost, Danger: r.danger})
	}
	if len(a.opinions) > 0 {
		out.Opinions = make(map[int]opinionSnap, len(a.opinions))
		for id, o := range a.opinions {
			out.Opinions[id] = opinionSnap{Risk: o.Risk, Affinity: o.Affinity,
				LastTick: o.lastTick, Strength: o.Strength, Variance: o.Variance,
				Samples: o.Samples}
		}
	}
	return out
}

// --- reading it back ---------------------------------------------------------

// Load builds the world a snapshot describes.
//
// Everything that can be worked out again is worked out again rather than
// stored: the spatial index, the terrain grid, the perception buffers. Storing
// them would be storing the same thing twice and inviting the two copies to
// disagree.
func Load(in io.Reader) (*World, error) {
	var s snapshot
	if err := json.NewDecoder(in).Decode(&s); err != nil {
		return nil, fmt.Errorf("reading the world: %w", err)
	}
	if s.Format != SaveFormat {
		return nil, fmt.Errorf("this is a format %d world and this build reads %d",
			s.Format, SaveFormat)
	}

	w := &World{
		cfg:         s.Config,
		agents:      make([]Agent, 0, len(s.Agents)),
		foods:       s.Foods,
		index:       make(map[int]int, len(s.Agents)),
		foodIndex:   make(map[int]int, len(s.Foods)),
		ai:          &AIController{},
		nextAgentID: s.NextAgentID,
		nextFoodID:  s.NextFoodID,
		tick:        s.Tick,
		foodAccum:   s.FoodAccum,
		regions:     s.Regions,
		foodWeight:  s.FoodWeight,
	}
	w.rng, w.draws = replayTo(s.Seed, s.Draws)
	w.ground = buildTerrain(&w.cfg)
	for i := range s.PendingSeeds {
		p := &s.PendingSeeds[i]
		w.pendingSeeds = append(w.pendingSeeds, pendingSeed{x: p.X, y: p.Y, genes: p.Genes})
	}
	for i := range s.Agents {
		w.agents = append(w.agents, loadAgent(&s.Agents[i]))
	}
	for i := range w.agents {
		w.index[w.agents[i].ID] = i
	}
	for i := range w.foods {
		w.foodIndex[w.foods[i].ID] = i
	}
	w.invalidateIndex()

	c := s.Counters
	w.births, w.evaded, w.hunts = c.Births, c.Evaded, c.Hunts
	w.huntParty, w.jointHunts = c.HuntParty, c.JointHunts
	w.firstSights, w.firstSightError = c.FirstSights, c.FirstSightError
	w.firstSightsLearned, w.firstSightErrorLearned = c.FirstSightsLearned, c.FirstSightErrorLearned
	w.firstSightErrorFlat, w.firstSightErrorFixed = c.FirstSightErrorFlat, c.FirstSightErrorFixed
	w.geniuses, w.greatGeniuses = c.Geniuses, c.GreatGeniuses
	w.deaths, w.kills, w.agingDeaths = c.Deaths, c.Kills, c.AgingDeaths
	w.drownDeaths, w.drownWitnesses = c.DrownDeaths, c.DrownWitnesses
	w.killWitnesses, w.avengeWitnesses = c.KillWitnesses, c.AvengeWitnesses
	w.killLessons = c.KillLessons
	w.calls, w.joins = c.Calls, c.Joins
	w.observes = c.Observes
	w.matured, w.childDeaths, w.fights = c.Matured, c.ChildDeaths, c.Fights
	w.maxGeneration = c.MaxGeneration
	w.blowsSeen, w.blowsAnswered = c.BlowsSeen, c.BlowsAnswered
	w.courtships, w.courtshipsAccepted = c.Courtships, c.CourtshipsAccepted
	w.flees, w.escapes = c.Flees, c.Escapes
	w.exchanges, w.hintsCopied = c.Exchanges, c.HintsCopied
	return w, nil
}

func loadAgent(s *agentSnap) Agent {
	a := s.Agent
	a.frailTicks = s.FrailTicks
	a.courtStartTick = s.CourtStartTick
	a.reproReady = s.ReproReady
	a.lastDecisionTick = s.LastDecisionTick
	a.vitalityAtDecision = s.VitalityAtDecision
	a.needsDecision = s.NeedsDecision
	a.pendingTrigger = s.PendingTrigger
	a.attackerID = s.AttackerID
	a.lastAttackTick = s.LastAttackTick
	a.lore = lore{
		retaliation:       belief{mean: s.Lore.RetaliationMean, n: s.Lore.RetaliationN},
		accept:            belief{mean: s.Lore.AcceptMean, n: s.Lore.AcceptN},
		riskWeight:        s.Lore.RiskWeight,
		competitionWeight: s.Lore.CompetitionWeight,
		shockRisk:         s.Lore.ShockRisk,
	}
	a.hints = s.Hints
	a.hintSlots = s.HintSlots
	a.sawFood, a.sawMate = s.SawFood, s.SawMate
	a.chronotype = s.Chronotype
	a.seed, a.seedDueAt = s.Seed, s.SeedDueAt
	a.recentFood, a.dietTick = s.RecentFood, s.DietTick
	a.timesTaught = s.TimesTaught
	a.looks = looksModel{n: s.Looks.N, sx: s.Looks.Sx, sy: s.Looks.Sy,
		sxx: s.Looks.Sxx, sxy: s.Looks.Sxy}
	a.engageID, a.engageStart, a.engageLast = s.EngageID, s.EngageStart, s.EngageLast
	a.openUntil = s.OpenUntil
	a.hitBy = s.HitBy
	a.effortSpent = s.EffortSpent
	a.actionTicks = s.ActionTicks
	a.noSpareMemory = s.NoSpareMemory
	a.memoryTick, a.memoryUsed = s.MemoryTick, s.MemoryUsed
	a.courtedBy, a.courtedTick = s.CourtedBy, s.CourtedTick
	a.lastCourt = s.LastCourt
	a.rejected = s.Rejected
	for i := range s.Regions {
		r := &s.Regions[i]
		a.regions = append(a.regions, regionView{seen: r.Seen, n: r.N, logN: r.LogN,
			lastTick: r.LastTick, cost: r.Cost, danger: r.Danger})
	}
	if len(s.Opinions) > 0 {
		a.opinions = make(map[int]*Opinion, len(s.Opinions))
		for id, o := range s.Opinions {
			a.opinions[id] = &Opinion{Risk: o.Risk, Affinity: o.Affinity,
				lastTick: o.LastTick, Strength: o.Strength, Variance: o.Variance,
				Samples: o.Samples}
		}
	}
	return a
}

// --- a population without its world ------------------------------------------
//
// The other half of what stage 21 is for: taking bodies raised in one world
// into another. What can travel is what a body is; what cannot is anything
// that names somebody else, because a name from another world points at
// nobody here.
//
//   travels: the genome (and so the budget), sex, species, what it wants out
//            of life and what it believes about the world, its rules of thumb
//            and the room it has for them, and the hour it sleeps best at.
//
//   stays:   memory of other agents (their IDs mean nothing here), the line it
//            came from (same), the pair it was in, and what it made of that
//            world's country (a region index is a place in that world).
//            Also its body's condition and age: an arrival is a new body
//            carrying an inheritance, not a copy of a life half lived.

// Node is one body's inheritance, free of the world it was raised in.
type Node struct {
	Genome     []float64 `json:"genome"`
	Sex        Sex       `json:"sex"`
	Species    Species   `json:"species"`
	Lore       loreSnap  `json:"lore"`
	Hints      []Hint    `json:"hints,omitempty"`
	HintSlots  int       `json:"hintSlots"`
	Chronotype float64   `json:"chronotype"`
}

type nodeFile struct {
	Format int    `json:"format"`
	Nodes  []Node `json:"nodes"`
}

// Nodes is the living population as inheritances. Read only.
func (w *World) Nodes() []Node {
	out := make([]Node, 0, len(w.agents))
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		out = append(out, Node{
			Genome:  append([]float64(nil), a.Genome...),
			Sex:     a.Sex,
			Species: a.Species,
			Lore: loreSnap{
				RetaliationMean: a.lore.retaliation.mean, RetaliationN: a.lore.retaliation.n,
				AcceptMean: a.lore.accept.mean, AcceptN: a.lore.accept.n,
				RiskWeight:        a.lore.riskWeight,
				CompetitionWeight: a.lore.competitionWeight,
				ShockRisk:         a.lore.shockRisk,
			},
			Hints:      append([]Hint(nil), a.hints...),
			HintSlots:  a.hintSlots,
			Chronotype: a.chronotype,
		})
	}
	return out
}

// SaveNodes writes a population on its own.
func SaveNodes(out io.Writer, nodes []Node) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", " ")
	return enc.Encode(&nodeFile{Format: SaveFormat, Nodes: nodes})
}

// LoadNodes reads one back.
func LoadNodes(in io.Reader) ([]Node, error) {
	var f nodeFile
	if err := json.NewDecoder(in).Decode(&f); err != nil {
		return nil, fmt.Errorf("reading the population: %w", err)
	}
	if f.Format != SaveFormat {
		return nil, fmt.Errorf("this is a format %d population and this build reads %d",
			f.Format, SaveFormat)
	}
	return f.Nodes, nil
}

// Repopulate empties the world of bodies and puts these in instead, and
// returns how many arrived.
//
// They arrive the way the world's own founders do - somewhere at random, grown,
// with a random amount of vitality, hunger and life left - because that is
// what they are: a first generation, carrying an inheritance from elsewhere.
// Everything that happens to them from here happens in this world.
//
// The food, the country and the clock are left alone: this changes who is in
// the world, not what the world is.
func (w *World) Repopulate(nodes []Node) int {
	w.agents = w.agents[:0]
	clear(w.index)
	w.newborns = w.newborns[:0]
	w.invalidateIndex()

	for i := range nodes {
		n := &nodes[i]
		a := w.newAgent(
			w.randRange(20, w.cfg.Width-20),
			w.randRange(20, w.cfg.Height-20),
			n.Sex,
			append([]float64(nil), n.Genome...),
			0,
			1, // grown, like any founder
		)
		a.Species = n.Species
		a.lore = lore{
			retaliation:       belief{mean: n.Lore.RetaliationMean, n: n.Lore.RetaliationN},
			accept:            belief{mean: n.Lore.AcceptMean, n: n.Lore.AcceptN},
			riskWeight:        n.Lore.RiskWeight,
			competitionWeight: n.Lore.CompetitionWeight,
			shockRisk:         n.Lore.ShockRisk,
		}
		a.hints = append([]Hint(nil), n.Hints...)
		a.hintSlots = n.HintSlots
		a.chronotype = n.Chronotype
		a.Vitality = w.randRange(a.MaxVitality(&w.cfg)*0.6, a.MaxVitality(&w.cfg))
		a.Hunger = w.randRange(0, w.cfg.SatiatedHunger)
		a.Lifespan = w.randRange(w.cfg.MaxLifespan*0.5, w.cfg.MaxLifespan)
		w.addAgent(a)
	}
	return len(nodes)
}
