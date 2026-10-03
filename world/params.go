package world

// Params groups all generation parameters. The world is a pure function of
// (Seed, Params, edits). Parameters are split into groups so that changing a
// parameter group that does not affect a given layer never invalidates that
// layer: e.g. changing RoadParams leaves every terrain byte untouched.
type Params struct {
	// AlgorithmVersion selects the generation algorithm. 0 means
	// CurrentAlgorithmVersion. Old versions remain reproducible forever.
	AlgorithmVersion int

	// ChunkSize is the edge length of a chunk in tiles. Must be > 0.
	ChunkSize int

	// SeaLevel is the height threshold (in [0,1]) below which tiles are ocean.
	SeaLevel float64

	// Terrain contains all parameters driving terrain generation.
	Terrain TerrainParams

	// Settlement contains all parameters driving settlement placement.
	Settlement SettlementParams

	// Road contains all parameters driving road network generation.
	// Road parameters never affect terrain, moisture, biomes or settlements.
	RoadParams RoadParams
}

// TerrainParams drives the pure terrain function.
type TerrainParams struct {
	// ContinentScale controls the frequency of the base continent noise.
	// Smaller values produce larger landmasses.
	ContinentScale float64
	// MountainScale controls the frequency of ridge/mountain noise.
	MountainScale float64
	// MoistureScale controls the frequency of the moisture field.
	MoistureScale float64
	// Relief controls the amplitude of height variation.
	Relief float64
	// LatitudeEffect controls how strongly temperature decays towards the poles.
	LatitudeEffect float64
	// CliffSlope is the slope above which a tile is impassable for roads.
	CliffSlope float64
}

// SettlementParams drives deterministic settlement placement.
type SettlementParams struct {
	// MinSpacing is the minimum euclidean distance (in tiles) between two
	// settlements, measured between their tile coordinates.
	MinSpacing float64
	// CandidatesPerChunk controls how many candidate sites are evaluated per
	// chunk (candidates are laid out on a jittered grid).
	CandidatesPerChunk int
	// MinScore is the minimum suitability score a site must reach.
	MinScore float64
	// MaxSlope is the steepest tile a settlement may occupy.
	MaxSlope float64
}

// RoadParams drives road network generation. These parameters never affect
// terrain.
type RoadParams struct {
	// LandCost is the A* cost of walking one land tile.
	LandCost float64
	// WaterCost is the A* cost of crossing a water tile (bridge/ford).
	WaterCost float64
	// MaxSlope is the steepness a road may traverse; steeper tiles are
	// impassable.
	MaxSlope float64
}

// DefaultParams returns the default parameter set for the current algorithm
// version.
func DefaultParams() Params {
	return Params{
		AlgorithmVersion: CurrentAlgorithmVersion,
		ChunkSize:        32,
		SeaLevel:         0.55,
		Terrain: TerrainParams{
			ContinentScale: 0.012,
			MountainScale:  0.021,
			MoistureScale:  0.018,
			Relief:         0.45,
			LatitudeEffect: 0.0045,
			CliffSlope:     0.85,
		},
		Settlement: SettlementParams{
			MinSpacing:         24,
			CandidatesPerChunk: 9,
			MinScore:           0.55,
			MaxSlope:           0.35,
		},
		RoadParams: RoadParams{
			LandCost:  1.0,
			WaterCost: 3.0,
			MaxSlope:  0.85,
		},
	}
}

// Normalize fills zero values with defaults and validates the parameters.
// It returns an error if the parameters are invalid.
func (p *Params) Normalize() error {
	if p.AlgorithmVersion == 0 {
		p.AlgorithmVersion = CurrentAlgorithmVersion
	}
	if p.ChunkSize <= 0 {
		p.ChunkSize = 32
	}
	if p.SeaLevel <= 0 || p.SeaLevel >= 1 {
		p.SeaLevel = 0.55
	}
	d := DefaultParams()
	if p.Terrain.ContinentScale == 0 {
		p.Terrain.ContinentScale = d.Terrain.ContinentScale
	}
	if p.Terrain.MountainScale == 0 {
		p.Terrain.MountainScale = d.Terrain.MountainScale
	}
	if p.Terrain.MoistureScale == 0 {
		p.Terrain.MoistureScale = d.Terrain.MoistureScale
	}
	if p.Terrain.Relief == 0 {
		p.Terrain.Relief = d.Terrain.Relief
	}
	if p.Terrain.LatitudeEffect == 0 {
		p.Terrain.LatitudeEffect = d.Terrain.LatitudeEffect
	}
	if p.Terrain.CliffSlope == 0 {
		p.Terrain.CliffSlope = d.Terrain.CliffSlope
	}
	if p.Settlement.MinSpacing == 0 {
		p.Settlement.MinSpacing = d.Settlement.MinSpacing
	}
	if p.Settlement.CandidatesPerChunk == 0 {
		p.Settlement.CandidatesPerChunk = d.Settlement.CandidatesPerChunk
	}
	if p.Settlement.MinScore == 0 {
		p.Settlement.MinScore = d.Settlement.MinScore
	}
	if p.Settlement.MaxSlope == 0 {
		p.Settlement.MaxSlope = d.Settlement.MaxSlope
	}
	if p.RoadParams.LandCost == 0 {
		p.RoadParams.LandCost = d.RoadParams.LandCost
	}
	if p.RoadParams.WaterCost == 0 {
		p.RoadParams.WaterCost = d.RoadParams.WaterCost
	}
	if p.RoadParams.MaxSlope == 0 {
		p.RoadParams.MaxSlope = d.RoadParams.MaxSlope
	}
	return nil
}
