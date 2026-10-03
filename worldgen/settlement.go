package worldgen

import (
	"math"
	"sort"

	"github.com/Diudiuseven/city-settlement-generator-go/world"
)

const saltSettleID uint64 = 0x73697465 // "site"

// candidate is a potential settlement site.
type candidate struct {
	id     uint64
	tx, ty int
	score  float64
}

// nameSyllables feed the deterministic name generator.
var namePrefix = []string{
	"Al", "Bran", "Cor", "Dun", "El", "Fen", "Glen", "Hav", "Ith", "Jor",
	"Kel", "Lor", "Mir", "Nor", "Os", "Pel", "Quen", "Riv", "Sol", "Thal",
	"Ul", "Ves", "Wyr", "Zeph",
}
var nameSuffix = []string{
	"ford", "burg", "haven", "dale", "field", "mont", "wick", "mere",
	"stead", "holm", "gar", "fell", "port", "grad", "wijk", "berg",
}

// settlementName builds a stable, deterministic name from a settlement id.
func settlementName(id uint64) string {
	p := namePrefix[id%uint64(len(namePrefix))]
	s := nameSuffix[(id/uint64(len(namePrefix)))%uint64(len(nameSuffix))]
	return p + s
}

// evaluateSite scores a candidate tile. It returns ok=false when the tile is
// unsuitable (water, cliff, too steep, polar ice).
func evaluateSite(seed uint64, p world.Params, tx, ty int, ovH map[overrideKey]float64, ovM map[overrideKey]float64) (candidate, bool) {
	h := effectiveHeight(seed, p, ovH, tx, ty)
	m := effectiveMoisture(seed, p, ovM, tx, ty)
	temp := terrainTemperature(seed, p, tx, ty, h)
	slope := 0.0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			hn := effectiveHeight(seed, p, ovH, tx+dx, ty+dy)
			if d := math.Abs(hn - h); d > slope {
				slope = d
			}
		}
	}
	biome := classifyBiome(h, temp, m, p.SeaLevel)
	if h < p.SeaLevel+0.03 {
		return candidate{}, false
	}
	if slope > p.Settlement.MaxSlope {
		return candidate{}, false
	}
	if biome == world.BiomeSnow || biome == world.BiomeOcean || biome == world.BiomeBeach {
		return candidate{}, false
	}

	score := (1 - slope/p.Settlement.MaxSlope) * 0.5
	score += m * 0.3
	score += 0.2
	if biome == world.BiomeDesert {
		score -= 0.2
	}
	// Coastal bonus: fresh water/sea access nearby.
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			if effectiveHeight(seed, p, ovH, tx+dx, ty+dy) < p.SeaLevel+0.02 {
				score += 0.1
				break
			}
		}
	}
	if score < p.Settlement.MinScore {
		return candidate{}, false
	}
	id := hash2(seed+saltSettleID, int64(tx), int64(ty))
	return candidate{id: id, tx: tx, ty: ty, score: score}, true
}

// candidatesInChunk enumerates and scores all candidate sites of one chunk.
// It is a pure function of (seed, params, edits, chunk coordinates).
func candidatesInChunk(seed uint64, p world.Params, cx, cy int, ovH map[overrideKey]float64, ovM map[overrideKey]float64) []candidate {
	size := p.ChunkSize
	grid := int(math.Ceil(math.Sqrt(float64(p.Settlement.CandidatesPerChunk))))
	if grid < 1 {
		grid = 1
	}
	cell := size / grid
	if cell < 1 {
		cell = 1
	}
	var out []candidate
	for j := 0; j < grid; j++ {
		for i := 0; i < grid; i++ {
			jx, jy := jitter(seed, int64(cx), int64(cy), int64(j*grid+i))
			tx := cx*size + i*cell + cell/2 + int((jx-0.5)*float64(cell)*0.8)
			ty := cy*size + j*cell + cell/2 + int((jy-0.5)*float64(cell)*0.8)
			// Keep the jittered site inside the chunk.
			if tx < cx*size {
				tx = cx * size
			}
			if ty < cy*size {
				ty = cy * size
			}
			if tx >= (cx+1)*size {
				tx = (cx+1)*size - 1
			}
			if ty >= (cy+1)*size {
				ty = (cy+1)*size - 1
			}
			if c, ok := evaluateSite(seed, p, tx, ty, ovH, ovM); ok {
				out = append(out, c)
			}
		}
	}
	return out
}

// generateSettlements computes the deterministic settlement layer for chunk
// coord. Selection considers the chunk and its 8 neighbours (the halo);
// because every candidate is a pure function of the seed, parameters and
// edits, generating a chunk alone yields exactly the same settlements as
// generating it together with its neighbours.
func generateSettlements(seed uint64, p world.Params, coord world.ChunkCoord, ch *world.Chunk, ovH map[overrideKey]float64, ovM map[overrideKey]float64) ([]world.Settlement, error) {
	var all []candidate
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			all = append(all, candidatesInChunk(seed, p, coord.X+dx, coord.Y+dy, ovH, ovM)...)
		}
	}
	// Deterministic priority: higher score wins; ties broken by stable id.
	sort.Slice(all, func(i, j int) bool {
		if all[i].score != all[j].score {
			return all[i].score > all[j].score
		}
		return all[i].id < all[j].id
	})

	minDist2 := p.Settlement.MinSpacing * p.Settlement.MinSpacing
	var accepted []candidate
	for _, c := range all {
		ok := true
		for _, a := range accepted {
			dx := float64(c.tx - a.tx)
			dy := float64(c.ty - a.ty)
			if dx*dx+dy*dy < minDist2 {
				ok = false
				break
			}
		}
		if ok {
			accepted = append(accepted, c)
		}
	}

	var out []world.Settlement
	for _, a := range accepted {
		lx := a.tx - coord.X*p.ChunkSize
		ly := a.ty - coord.Y*p.ChunkSize
		if lx < 0 || ly < 0 || lx >= p.ChunkSize || ly >= p.ChunkSize {
			continue // belongs to a neighbour chunk
		}
		out = append(out, world.Settlement{
			ID:     a.id,
			TileX:  int32(a.tx),
			TileY:  int32(a.ty),
			ChunkX: int32(coord.X),
			ChunkY: int32(coord.Y),
			Score:  a.score,
			Name:   settlementName(a.id),
		})
	}
	return out, nil
}
