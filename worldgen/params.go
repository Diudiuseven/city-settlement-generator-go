package worldgen

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
)

// Params are the generation parameters. The world is finite:
// Width*Height tiles, partitioned into ChunkSize*ChunkSize chunks.
//
// Fields are split into two groups by reproducibility semantics:
//
//   - Terrain-affecting fields feed the raw terrain point function and
//     therefore change cell values. They are covered by TerrainDigest:
//     AlgorithmVersion, Width, Height, ChunkSize, WorldSeed, NoiseScale,
//     WaterThreshold, MountainThreshold, BackboneSpacing,
//     SettlementSpacing and SettlementJitter. The last three shape the
//     deterministic passable corridor network that guarantees road
//     reachability, so they are part of the terrain.
//   - Derived-only fields never modify terrain cells. They only steer
//     the settlement and road layers: NameSalt, MinSuitability,
//     MinSettlementDist2 and RoadCostScale. Changing them leaves all
//     already generated regions byte-identical.
type Params struct {
	// AlgorithmVersion selects the frozen terrain algorithm. Zero is
	// normalized to DefaultAlgorithmVersion.
	AlgorithmVersion uint16

	// World extent in cells. Width and Height must be positive and
	// evenly divisible by ChunkSize.
	Width     int
	Height    int
	ChunkSize int

	// WorldSeed is the root of every random decision in the world.
	WorldSeed int64
	// NameSalt perturbs settlement names without touching terrain.
	NameSalt int64

	// NoiseScale is the base noise-lattice spacing in cells.
	NoiseScale uint64

	// Terrain shaping (see classify for biome thresholds).
	MountainThreshold Height
	WaterThreshold    Height
	// BackboneSpacing is the spacing, in cells, of the guaranteed
	// passable vertical backbone corridors.
	BackboneSpacing int

	// Settlement placement. Sites live on a jittered square lattice:
	// base point (ix*S+S/2, iy*S+S/2), offset by a deterministic
	// jitter in [-SettlementJitter,SettlementJitter].
	SettlementSpacing  int
	SettlementJitter   int
	MinSuitability     uint16
	MinSettlementDist2 int

	// RoadCostScale linearly scales road traversal costs. It never
	// changes which paths exist, only edge weights.
	RoadCostScale uint64
}

// DefaultParams returns the version-1 default parameter set: a 256x256
// cell world made of 64x64 chunks.
func DefaultParams() Params {
	return Params{
		AlgorithmVersion:   DefaultAlgorithmVersion,
		Width:              256,
		Height:             256,
		ChunkSize:          64,
		WorldSeed:          1,
		NameSalt:           0,
		NoiseScale:         40,
		MountainThreshold:  46000,
		WaterThreshold:     19000,
		BackboneSpacing:    48,
		SettlementSpacing:  24,
		SettlementJitter:   4,
		MinSuitability:     16000,
		MinSettlementDist2: 16 * 16,
		RoadCostScale:      1,
	}
}

// Normalize fills defaults for zero-valued fields and canonicalizes
// the parameter set. It mutates the receiver in place.
func (p *Params) Normalize() {
	d := DefaultParams()
	if p.AlgorithmVersion == 0 {
		p.AlgorithmVersion = d.AlgorithmVersion
	}
	if p.Width == 0 {
		p.Width = d.Width
	}
	if p.Height == 0 {
		p.Height = d.Height
	}
	if p.ChunkSize == 0 {
		p.ChunkSize = d.ChunkSize
	}
	if p.NoiseScale == 0 {
		p.NoiseScale = d.NoiseScale
	}
	if p.MountainThreshold == 0 {
		p.MountainThreshold = d.MountainThreshold
	}
	if p.WaterThreshold == 0 {
		p.WaterThreshold = d.WaterThreshold
	}
	if p.BackboneSpacing == 0 {
		p.BackboneSpacing = d.BackboneSpacing
	}
	if p.SettlementSpacing == 0 {
		p.SettlementSpacing = d.SettlementSpacing
	}
	if p.MinSuitability == 0 {
		p.MinSuitability = d.MinSuitability
	}
	if p.RoadCostScale == 0 {
		p.RoadCostScale = d.RoadCostScale
	}
	// Minimum center distance is structurally guaranteed by the
	// jittered lattice; keep the explicit field consistent with it.
	if p.MinSettlementDist2 == 0 {
		gap := p.SettlementSpacing - 2*p.SettlementJitter
		p.MinSettlementDist2 = gap * gap
	}
}

// Validate reports an error for parameter combinations that cannot
// produce a world.
func (p Params) Validate() error {
	if p.Width <= 0 || p.Height <= 0 || p.ChunkSize <= 0 {
		return errors.New("worldgen: width, height and chunk size must be positive")
	}
	if p.Width%p.ChunkSize != 0 || p.Height%p.ChunkSize != 0 {
		return errors.New("worldgen: world extent must be divisible by chunk size")
	}
	if p.NoiseScale < 2 {
		return errors.New("worldgen: noise scale too small")
	}
	if p.WaterThreshold >= p.MountainThreshold {
		return errors.New("worldgen: water threshold must be below mountain threshold")
	}
	if p.BackboneSpacing <= 1 {
		return errors.New("worldgen: backbone spacing must be > 1")
	}
	if p.SettlementSpacing < 3 {
		return errors.New("worldgen: settlement spacing must be >= 3")
	}
	if p.SettlementJitter < 0 || 2*p.SettlementJitter+2 > p.SettlementSpacing {
		return errors.New("worldgen: settlement jitter too large for spacing")
	}
	if _, ok := terrainFor(p.AlgorithmVersion); !ok {
		return errors.New("worldgen: unknown algorithm version")
	}
	return nil
}

// ErrInvalidParams is returned by helpers rejecting bad parameter sets.
var ErrInvalidParams = errors.New("worldgen: invalid parameters")

// digestFields serializes every parameter field in canonical
// big-endian order.
func (p Params) digestFields() []byte {
	b := make([]byte, 0, 80)
	be16 := func(v uint16) { b = binary.BigEndian.AppendUint16(b, v) }
	be64 := func(v uint64) { b = binary.BigEndian.AppendUint64(b, v) }
	be16(p.AlgorithmVersion)
	be64(uint64(p.Width))
	be64(uint64(p.Height))
	be64(uint64(p.ChunkSize))
	be64(uint64(p.WorldSeed))
	be64(p.NoiseScale)
	be16(uint16(p.MountainThreshold))
	be16(uint16(p.WaterThreshold))
	be64(uint64(p.BackboneSpacing))
	be64(uint64(p.SettlementSpacing))
	be64(uint64(p.SettlementJitter))
	// derived-only fields below
	be64(uint64(p.NameSalt))
	be16(p.MinSuitability)
	be64(uint64(p.MinSettlementDist2))
	be64(p.RoadCostScale)
	return b
}

// TerrainDigest hashes exactly the fields that can influence terrain
// cell values (including the algorithm version). It is stable forever
// for a given combination, which is what old worlds are pinned to.
func (p Params) TerrainDigest() [32]byte {
	// Same layout as digestFields but truncated before the
	// derived-only fields (NameSalt is the first of those).
	full := p.digestFields()
	terrainLen := 0
	terrainLen += 2 // AlgorithmVersion
	terrainLen += 8 * 6
	terrainLen += 2 * 2
	terrainLen += 8 * 2
	return sha256.Sum256(full[:terrainLen])
}

// Digest hashes every parameter field.
func (p Params) Digest() [32]byte {
	return sha256.Sum256(p.digestFields())
}
