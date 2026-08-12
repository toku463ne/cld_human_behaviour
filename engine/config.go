package engine

// Config holds every tunable parameter of the simulation.
//
// Behaviour constants live here instead of package level constants so that a
// test can neutralize one rule at a time (for example MutationStd = 0 makes a
// child's ability the exact average of its parents, and FoodSpawnRate = 0 keeps
// the food layout under the test's control).
type Config struct {
	// World
	Width  float64
	Height float64
	Seed   int64

	InitialPopulation int
	InitialFoodItems  int
	MaxPopulation     int
	MaxFoodItems      int
	FoodSpawnRate     float64 // expected number of food items spawned per tick

	// Metabolism and resources
	FoodNutrition      float64
	Metabolism         float64 // food burned every tick
	MaxFoodStore       float64
	FoodLowThreshold   float64 // below this an agent must forage: survival comes first
	ReproFoodThreshold float64 // an agent looks for a mate only above this
	BirthCost          float64 // food the parents share to produce a child
	ChildInitialFood   float64

	// Reproduction
	PairBondDuration int     // ticks a pair spends together before a child is born
	MatingCooldown   int     // ticks an agent rests after a bond ends
	MutationStd      float64 // standard deviation of the mutation added to a child's ability
	MinLifespan      int
	MaxLifespan      int

	// Perception, judgement and movement
	PerceptionRadius float64
	GrabRadius       float64
	AgentSpeed       float64
	BoundaryMargin   float64

	// JudgementNoise scales how badly an agent misreads a rival or a candidate.
	// The error is proportional to (100 - Rationality) / 100, so a fully
	// rational agent estimates the situation exactly.
	JudgementNoise float64

	// Contests over food
	ContestNoise        float64 // luck involved in the outcome of a fight
	ContestAvoidMargin  float64 // a rival looking this much stronger is not worth fighting
	ContestLossPenalty  float64 // food lost when a fight is lost
	FoodRejectDuration  int     // ticks a contested food item is avoided
	MateRejectDuration  int     // ticks a passed over candidate is avoided
	PatienceBase        float64 // ticks of comparison before committing to a mate
	PatienceRationality float64 // extra patience per point of rationality
	CommitFitness       float64 // a candidate this attractive is worth an immediate commitment
}

// DefaultConfig returns the parameters of the v0 prototype
// (human_evolution_sim.html), which is the reference implementation of these
// rules.
func DefaultConfig() Config {
	return Config{
		Width:  800,
		Height: 600,
		Seed:   1,

		InitialPopulation: 60,
		InitialFoodItems:  80,
		MaxPopulation:     240,
		MaxFoodItems:      160,
		FoodSpawnRate:     1.5,

		FoodNutrition:      30,
		Metabolism:         0.045,
		MaxFoodStore:       120,
		FoodLowThreshold:   32,
		ReproFoodThreshold: 62,
		BirthCost:          24,
		ChildInitialFood:   35,

		PairBondDuration: 150,
		MatingCooldown:   70,
		MutationStd:      4,
		MinLifespan:      1500,
		MaxLifespan:      2200,

		PerceptionRadius: 130,
		GrabRadius:       11,
		AgentSpeed:       1.15,
		BoundaryMargin:   8,

		JudgementNoise: 34,

		ContestNoise:        6,
		ContestAvoidMargin:  1.05,
		ContestLossPenalty:  5,
		FoodRejectDuration:  30,
		MateRejectDuration:  40,
		PatienceBase:        25,
		PatienceRationality: 0.9,
		CommitFitness:       82,
	}
}
