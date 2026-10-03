package worldgen

import (
	"context"
)

// forcedLandHeight is the elevation assigned to guaranteed-passable
// corridor cells: dry land well above WaterThreshold.
const forcedLandHeight Height = 22000

// classify derives the biome from elevation and moisture. It is a pure
// function of the two terrain values and the thresholds, so neighbors
// rendered by different chunks always agree on shared classifications.
func classify(h Height, p Params) Biome {
	switch {
	case h >= p.MountainThreshold:
		return BiomeMountain
	case h >= p.WaterThreshold*8/10 && h < p.WaterThreshold:
		return BiomeDeepWater
	case h < p.WaterThreshold:
		return BiomeWater
	case h < p.WaterThreshold+(p.MountainThreshold-p.WaterThreshold)/3:
		// Dry lowland: desert when arid, otherwise grassland.
		if Moisture(h) < 18000 {
			return BiomeDesert
		}
		return BiomeGrassland
	default:
		// Higher, drier ground.
		if Moisture(h) < 32000 {
			return BiomeGrassland
		}
		return BiomeForest
	}
}

// Passable reports whether roads may traverse a biome. Mountains and
// water are impassable; land biomes are passable.
func Passable(b Biome) bool {
	switch b {
	case BiomeDesert, BiomeGrassland, BiomeForest:
		return true
	default:
		return false
	}
}

// nearSettlementBase reports whether (x,y) is inside the forced-safe
// square (radius 2) around ANY settlement lattice base point. Lattice
// parameters are part of the terrain digest, so this is a pure,
// chunk-independent test.
func nearSettlementBase(p Params, x, y int) bool {
	s := p.SettlementSpacing
	if s <= 0 {
		return false
	}
	ix := x / s
	iy := y / s
	for ddy := -1; ddy <= 1; ddy++ {
		for ddx := -1; ddx <= 1; ddx++ {
			bx := (ix + ddx) * s
			by := (iy + ddy) * s
			bx += s / 2
			by += s / 2
			if bx < 0 || by < 0 || bx >= p.Width || by >= p.Height {
				continue
			}
			if absInt(x-bx) <= 2 && absInt(y-by) <= 2 {
				return true
			}
		}
	}
	return false
}

// onSettlementApproach reports whether (x,y) lies on the one-cell
// horizontal approach strip connecting some lattice base row to the
// nearest vertical backbone column.
func onSettlementApproach(p Params, x, y int) bool {
	s := p.SettlementSpacing
	if s <= 0 {
		return false
	}
	if absInt(y-(y/s*s+s/2)) > 0 {
		return false
	}
	ix := x / s
	iy := y / s
	for ddx := -1; ddx <= 1; ddx++ {
		bx := (ix + ddx) * s
		by := iy * s
		bx += s / 2
		by += s / 2
		if bx < 0 || by < 0 || bx >= p.Width || by >= p.Height {
			continue
		}
		if absInt(y-by) != 0 {
			continue
		}
		bb := nearestBackbone(p, bx)
		lo, hi := bx, bb
		if lo > hi {
			lo, hi = hi, lo
		}
		if x >= lo && x <= hi {
			return true
		}
	}
	return false
}

// nearestBackbone returns the column x of the vertical backbone
// corridor closest to bx.
func nearestBackbone(p Params, bx int) int {
	g := p.BackboneSpacing
	if g <= 0 {
		return 0
	}
	k := bx / g
	cand := []int{k * g, (k + 1) * g}
	if k-1 >= 0 {
		cand = append(cand, (k-1)*g)
	}
	best := cand[0]
	bestD := absInt(bx - best)
	for _, c := range cand[1:] {
		if c < 0 || c >= p.Width {
			continue
		}
		if d := absInt(bx - c); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

// forcePassable shapes the terrain so that road reachability is
// guaranteed structurally: vertical backbones run the whole height,
// every settlement base has a safe 5x5 area, and a horizontal strip
// joins each base row to its nearest backbone. It only ever lifts
// cells onto dry land; it never creates water or mountains.
func forcePassable(p Params, x, y int, h Height) Height {
	g := p.BackboneSpacing
	if g > 0 && x%g == 0 {
		if h < p.WaterThreshold || h >= p.MountainThreshold {
			return forcedLandHeight
		}
	}
	if nearSettlementBase(p, x, y) {
		if !Passable(classify(h, p)) {
			return forcedLandHeight
		}
	}
	if onSettlementApproach(p, x, y) {
		if !Passable(classify(h, p)) {
			return forcedLandHeight
		}
	}
	return h
}

// terrain1 is the frozen version-1 raw terrain point function.
//
// It is a pure function of (p, x, y): rendering a chunk in isolation
// yields exactly the same cells as rendering it among neighbors, so
// chunk borders can never show seams.
func terrain1(p Params, x, y int) (Height, Moisture) {
	n := newNoiseState(p.WorldSeed, p.NoiseScale)
	h := n.fbmHeight(x, y)
	m := n.fbmMoisture(x, y)
	h = forcePassable(p, x, y, h)
	return h, m
}

func init() {
	registerAlgorithm(1, terrain1)
}

// terrainView exposes (possibly edited) cell values to the derived
// layers. Every access is a pure function, so views never depend on
// which chunks happen to be materialized.
type terrainView struct {
	params  Params
	overlay *editOverlay
	raw     terrainFn
}

func newTerrainView(p Params, ov *editOverlay) terrainView {
	fn, ok := terrainFor(p.AlgorithmVersion)
	if !ok {
		fn = terrain1
	}
	return terrainView{params: p, overlay: ov, raw: fn}
}

// cellAt returns the biome-classified cell at a coordinate. Out of
// range coordinates are impassable sentinel mountains.
func (v terrainView) cellAt(x, y int) Cell {
	if x < 0 || y < 0 || x >= v.params.Width || y >= v.params.Height {
		return Cell{Height: v.params.MountainThreshold, Moisture: 0, Biome: BiomeMountain}
	}
	h, m := v.raw(v.params, x, y)
	if e, ok := v.overlay.get(x, y); ok {
		if e.SetHeight != nil {
			h = *e.SetHeight
		}
		if e.SetMoisture != nil {
			m = *e.SetMoisture
		}
	}
	return Cell{Height: h, Moisture: m, Biome: classify(h, v.params)}
}

// renderChunk renders one chunk, edits included, from the point
// function alone.
func renderChunk(p Params, ov *editOverlay, c ChunkCoord) *Chunk {
	view := newTerrainView(p, ov)
	ch := &Chunk{Coord: c, Size: p.ChunkSize, Cells: make([]Cell, p.ChunkSize*p.ChunkSize)}
	ox, oy := c.CX*p.ChunkSize, c.CY*p.ChunkSize
	for ly := 0; ly < p.ChunkSize; ly++ {
		for lx := 0; lx < p.ChunkSize; lx++ {
			ch.Cells[ly*p.ChunkSize+lx] = view.cellAt(ox+lx, oy+ly)
		}
	}
	return ch
}

// generateChunks renders several chunks with bounded parallelism.
//
// Determinism: each chunk is a pure function of its coordinate, so the
// output map is identical regardless of worker count or scheduling.
//
// Atomicity: on cancellation/timeout the returned map is nil and no
// result is observable. Slow chunks do not block cancellation of
// finished work, and one slow chunk never delays the others (all run up
// to the concurrency limit independently).
func generateChunks(ctx context.Context, p Params, ov *editOverlay, coords []ChunkCoord, concurrency int) (map[ChunkCoord]*Chunk, error) {
	if concurrency < 1 {
		concurrency = 1
	}
	out := make([]*Chunk, len(coords))
	errs := make([]error, len(coords))
	sem := make(chan struct{}, concurrency)
	done := make(chan int, len(coords))
	for i, c := range coords {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		go func(i int, c ChunkCoord) {
			defer func() { <-sem }()
			defer func() { done <- i }()
			// Pure render; respect cancellation before starting work.
			select {
			case <-ctx.Done():
				errs[i] = ctx.Err()
				return
			default:
			}
			out[i] = renderChunk(p, ov, c)
		}(i, c)
	}
	for range coords {
		<-done
		// Bail out as soon as cancellation is observable; in-flight
		// renders finish into local slices and are discarded, so no
		// half chunk can ever be published.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	// Deterministic assembly: every chunk is a pure function of its
	// coordinate, so worker scheduling cannot affect the values.
	m := make(map[ChunkCoord]*Chunk, len(coords))
	for i, ch := range out {
		if errs[i] != nil {
			return nil, errs[i]
		}
		if ch == nil {
			return nil, context.Canceled
		}
		m[ch.Coord] = ch
	}
	return m, nil
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
