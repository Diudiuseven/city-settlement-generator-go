package worldgen

// Height is the terrain elevation of a cell in the range [0,65535].
// Integer fixed-point values are used everywhere so results are
// bit-identical across machines and Go versions.
type Height uint16

// Moisture is the soil moisture of a cell in the range [0,65535].
type Moisture uint16

// Biome is the ecological classification of a cell.
type Biome uint8

const (
	BiomeUnknown   Biome = 0
	BiomeDeepWater Biome = 1
	BiomeWater     Biome = 2
	BiomeDesert    Biome = 3
	BiomeGrassland Biome = 4
	BiomeForest    Biome = 5
	BiomeMountain  Biome = 6
)

// Cell is a single terrain tile. All fields are derived deterministically
// from the world seed, generation parameters and recorded edits.
type Cell struct {
	Height   Height
	Moisture Moisture
	Biome    Biome
}

// ChunkCoord identifies a chunk in chunk-grid coordinates.
type ChunkCoord struct {
	CX, CY int
}

// Chunk is a square, row-major tile block of Size*Size cells.
type Chunk struct {
	Coord ChunkCoord
	Size  int
	Cells []Cell
}

// Settlement is a settlement site placed on the world tile grid.
type Settlement struct {
	ID   uint64
	X, Y int
	// Home is the chunk that owns the candidate lattice point.
	Home ChunkCoord
	// Suitability is the deterministic score in [0,65535].
	Suitability uint16
	Name        string
}

// RoadPoint is one tile of a road path.
type RoadPoint struct{ X, Y int }

// RoadEdge is a road segment connecting two settlements.
type RoadEdge struct {
	A, B uint64
	Cost uint64
	Path []RoadPoint
}

// RoadGraph is the connected road network of a world.
type RoadGraph struct {
	Edges []RoadEdge
}

// CellEdit is a deterministic override of a single cell's elevation
// and/or moisture. Nil pointers mean "leave this field unchanged".
type CellEdit struct {
	X, Y        int
	SetHeight   *Height
	SetMoisture *Moisture
}

// GenerateOptions controls the scheduling of a generation request.
type GenerateOptions struct {
	// Concurrency is the maximum number of chunks produced in parallel.
	// Values < 1 mean 1. The result never depends on this value.
	Concurrency int
}
