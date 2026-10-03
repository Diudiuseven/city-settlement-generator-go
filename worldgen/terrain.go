package worldgen

import (
	"math"
	"sort"

	"github.com/Diudiuseven/city-settlement-generator-go/world"
)

// Noise salts keep the different noise fields independent while remaining
// pure functions of the seed.
const (
	saltContinent uint64 = 0x636F6E74 // "cont"
	saltMountain  uint64 = 0x6D6F756E // "moun"
	saltMoisture  uint64 = 0x6D6F6973 // "mois"
	saltTemp      uint64 = 0x74656D70 // "temp"
	saltSettle    uint64 = 0x7365746C // "setl"
)

// overrideKey addresses a tile inside a specific chunk.
type overrideKey struct {
	cx, cy int
	idx    int
}

// buildOverrideMaps compiles the edit log into per-tile absolute overrides.
// Edits are applied in ID order; because later edits have greater IDs and
// override earlier ones on conflict, the edit with the lexicographically
// greatest ID wins for every tile, regardless of submission order.
func buildOverrideMaps(w *world.World) (map[overrideKey]float64, map[overrideKey]float64) {
	height := make(map[overrideKey]float64)
	moisture := make(map[overrideKey]float64)
	edits := w.Edits()
	ids := make([]string, 0, len(edits))
	for id := range edits {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		e := edits[id]
		for idx, o := range e.Tiles {
			k := overrideKey{cx: e.Chunk.X, cy: e.Chunk.Y, idx: idx}
			if o.Height != nil {
				height[k] = *o.Height
			}
			if o.Moisture != nil {
				moisture[k] = *o.Moisture
			}
		}
	}
	return height, moisture
}

func keyFor(size, tx, ty int) overrideKey {
	cx := tx / size
	cy := ty / size
	if tx < 0 {
		cx--
	}
	if ty < 0 {
		cy--
	}
	lx := tx - cx*size
	ly := ty - cy*size
	return overrideKey{cx: cx, cy: cy, idx: ly*size + lx}
}

// terrainHeight is the pure base height field in [0,1].
func terrainHeight(seed uint64, p world.Params, tx, ty int) float64 {
	x := float64(tx)
	y := float64(ty)
	cont := fbm(seed+saltContinent, x*p.Terrain.ContinentScale, y*p.Terrain.ContinentScale, 5, 2.0, 0.5)
	base := 0.5 + 0.5*cont
	m := ridgeNoise(seed+saltMountain, x*p.Terrain.MountainScale, y*p.Terrain.MountainScale, 4)
	h := base + m*m*p.Terrain.Relief
	if h < 0 {
		h = 0
	}
	if h > 1 {
		h = 1
	}
	return h
}

// terrainMoisture is the pure base moisture field in [0,1].
func terrainMoisture(seed uint64, p world.Params, tx, ty int) float64 {
	n := fbm(seed+saltMoisture, float64(tx)*p.Terrain.MoistureScale+500.0, float64(ty)*p.Terrain.MoistureScale, 4, 2.0, 0.5)
	m := 0.5 + 0.5*n
	if m < 0 {
		m = 0
	}
	if m > 1 {
		m = 1
	}
	return m
}

// terrainTemperature is the pure temperature field in [0,1] (hot -> cold).
func terrainTemperature(seed uint64, p world.Params, tx, ty int, height float64) float64 {
	t := 1.0 - math.Abs(float64(ty))*p.Terrain.LatitudeEffect
	t -= (height - 0.5) * 0.7
	t += 0.05 * fbm(seed+saltTemp, float64(tx)*0.01, float64(ty)*0.01, 2, 2.0, 0.5)
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return t
}

// effectiveHeight returns the edited height of a global tile: base terrain
// with the winning edit override applied. It is a pure function of
// (seed, params, edits, coordinates).
func effectiveHeight(seed uint64, p world.Params, ov map[overrideKey]float64, tx, ty int) float64 {
	if v, ok := ov[keyFor(p.ChunkSize, tx, ty)]; ok {
		return v
	}
	return terrainHeight(seed, p, tx, ty)
}

func effectiveMoisture(seed uint64, p world.Params, ov map[overrideKey]float64, tx, ty int) float64 {
	if v, ok := ov[keyFor(p.ChunkSize, tx, ty)]; ok {
		return v
	}
	return terrainMoisture(seed, p, tx, ty)
}

// GenerateChunk produces the terrain and settlement layers of one chunk as a
// pure function of the world seed, parameters and edits. It never reads or
// writes shared state: calling it concurrently, repeatedly or in any order
// yields the same bytes for the same inputs.
//
// The chunk's tiles depend only on the seed, parameters, edits and the
// chunk's coordinates, so generating a chunk alone yields exactly the same
// tiles as generating it together with its neighbours.
func GenerateChunk(w *world.World, coord world.ChunkCoord) (*world.Chunk, error) {
	p := w.Params()
	seed := uint64(w.Seed())
	size := p.ChunkSize
	ovH, ovM := buildOverrideMaps(w)

	ch := &world.Chunk{
		Coord:            coord,
		Size:             size,
		AlgorithmVersion: p.AlgorithmVersion,
		Seed:             w.Seed(),
		Tiles:            make([]world.Tile, size*size),
	}
	baseX := coord.X * size
	baseY := coord.Y * size

	// First pass: height, moisture, temperature.
	for ly := 0; ly < size; ly++ {
		for lx := 0; lx < size; lx++ {
			tx := baseX + lx
			ty := baseY + ly
			h := effectiveHeight(seed, p, ovH, tx, ty)
			m := effectiveMoisture(seed, p, ovM, tx, ty)
			temp := terrainTemperature(seed, p, tx, ty, h)
			ch.Tiles[ch.TileIndex(lx, ly)] = world.Tile{
				Height:      h,
				Moisture:    m,
				Temperature: temp,
			}
		}
	}

	// Second pass: slope (needs the 8-neighbourhood), biome, passability.
	for ly := 0; ly < size; ly++ {
		for lx := 0; lx < size; lx++ {
			t := ch.LocalTile(lx, ly)
			tx := baseX + lx
			ty := baseY + ly
			slope := 0.0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					hn := effectiveHeight(seed, p, ovH, tx+dx, ty+dy)
					if d := math.Abs(hn - t.Height); d > slope {
						slope = d
					}
				}
			}
			t.Slope = slope
			t.Biome = classifyBiome(t.Height, t.Temperature, t.Moisture, p.SeaLevel)
			if slope > p.Terrain.CliffSlope {
				t.SetImpassable()
			}
		}
	}

	// Settlement layer (pure; see settlement.go).
	sets, err := generateSettlements(seed, p, coord, ch, ovH, ovM)
	if err != nil {
		return nil, err
	}
	ch.Settlements = sets
	for i := range sets {
		t := ch.GlobalTile(int(sets[i].TileX), int(sets[i].TileY))
		if t != nil {
			t.SetSettlement()
		}
	}

	return ch, nil
}
