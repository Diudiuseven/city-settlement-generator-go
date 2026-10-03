package worldgen

import (
	"context"
	"encoding/binary"
	"sort"
)

// World is a reproducible tile world. Its complete logical state is
// exactly:
//
//	world seed + parameters (incl. algorithm version) + edit log
//
// The materialized chunks, settlement layer and road graph are all
// deterministic derivations of that state and can be discarded and
// rebuilt at any time with byte-identical results.
type World struct {
	params  Params
	overlay *editOverlay
	chunks  map[ChunkCoord]*Chunk
	sites   []Settlement
	roads   *RoadGraph
}

// NewWorld creates an empty world with normalized, validated params.
func NewWorld(p Params) (*World, error) {
	p.Normalize()
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &World{
		params:  p,
		overlay: newEditOverlay(),
		chunks:  map[ChunkCoord]*Chunk{},
		sites:   nil,
		roads:   &RoadGraph{},
	}, nil
}

// Params returns a copy of the bound parameters.
func (w *World) Params() Params { return w.params }

// Seed returns the world seed.
func (w *World) Seed() int64 { return w.params.WorldSeed }

// AlgorithmVersion returns the frozen terrain algorithm version.
func (w *World) AlgorithmVersion() uint16 { return w.params.AlgorithmVersion }

// allCoords returns every chunk coordinate in canonical (cx-major) order.
func (w *World) allCoords() []ChunkCoord {
	nx := w.params.Width / w.params.ChunkSize
	ny := w.params.Height / w.params.ChunkSize
	coords := make([]ChunkCoord, 0, nx*ny)
	for cy := 0; cy < ny; cy++ {
		for cx := 0; cx < nx; cx++ {
			coords = append(coords, ChunkCoord{CX: cx, CY: cy})
		}
	}
	return coords
}

// derive rebuilds the settlement and road layers from the current
// params + overlay. It is deterministic and independent of which
// chunks are materialized, because the terrain view is a pure point
// function.
func (w *World) derive() error {
	view := newTerrainView(w.params, w.overlay)
	sites := planSettlements(w.params, view)
	roads, err := buildRoadGraph(w.params, view, sites)
	if err != nil {
		return err
	}
	w.sites = sites
	w.roads = roads
	return nil
}

// GenerateChunks materializes chunks in parallel and commits them
// atomically. On cancellation/timeout the world is left exactly as it
// was; slow chunks never block the others (all run up to the
// concurrency limit) and no partial result is observable.
func (w *World) GenerateChunks(ctx context.Context, coords []ChunkCoord, opts GenerateOptions) error {
	if err := validateCoords(w.params, coords); err != nil {
		return err
	}
	rendered, err := generateChunks(ctx, w.params, w.overlay, coords, opts.Concurrency)
	if err != nil {
		return err
	}
	// Atomic commit: the map swap is the only mutation.
	for c, ch := range rendered {
		w.chunks[c] = ch
	}
	return nil
}

func validateCoords(p Params, coords []ChunkCoord) error {
	nx := p.Width / p.ChunkSize
	ny := p.Height / p.ChunkSize
	seen := map[ChunkCoord]bool{}
	for _, c := range coords {
		if c.CX < 0 || c.CY < 0 || c.CX >= nx || c.CY >= ny {
			return ErrInvalidParams
		}
		if seen[c] {
			return ErrInvalidParams
		}
		seen[c] = true
	}
	return nil
}

// ApplyEdits commits a batch of cell edits.
//
// The batch is first validated against a scratch overlay and scratch
// derived layers. Only when the new road graph is fully connected is
// the overlay swapped, affected chunks re-rendered and the new layers
// published. A disconnecting (or cancelled) batch leaves the world
// byte-for-byte unchanged.
//
// Re-submitting an edit is idempotent and the submission order of
// disjoint edits cannot change the final map.
func (w *World) ApplyEdits(ctx context.Context, edits []CellEdit, opts GenerateOptions) error {
	for _, e := range edits {
		if e.X < 0 || e.Y < 0 || e.X >= w.params.Width || e.Y >= w.params.Height {
			return ErrInvalidParams
		}
	}
	candidate := w.overlay.apply(edits)

	// Validate on scratch state first.
	scratch := &World{
		params:  w.params,
		overlay: candidate,
		chunks:  map[ChunkCoord]*Chunk{},
		roads:   &RoadGraph{},
	}
	if err := scratch.derive(); err != nil {
		return err
	}

	// Re-render only chunks touched by the batch (parallel, bounded).
	touched := touchedChunks(w.params, edits)
	rendered, err := generateChunks(ctx, w.params, candidate, touched, opts.Concurrency)
	if err != nil {
		return err
	}

	// Commit point: overlay, affected chunks, derived layers.
	w.overlay = candidate
	for c, ch := range rendered {
		w.chunks[c] = ch
	}
	w.sites = scratch.sites
	w.roads = scratch.roads
	return nil
}

// touchedChunks returns the de-duplicated, canonically ordered set of
// chunks containing any edited cell.
func touchedChunks(p Params, edits []CellEdit) []ChunkCoord {
	seen := map[ChunkCoord]bool{}
	var coords []ChunkCoord
	for _, e := range edits {
		c := ChunkCoord{CX: e.X / p.ChunkSize, CY: e.Y / p.ChunkSize}
		if !seen[c] {
			seen[c] = true
			coords = append(coords, c)
		}
	}
	sortCoords(coords)
	return coords
}

func sortCoords(coords []ChunkCoord) {
	sort.Slice(coords, func(i, j int) bool {
		if coords[i].CY != coords[j].CY {
			return coords[i].CY < coords[j].CY
		}
		return coords[i].CX < coords[j].CX
	})
}

// Rebuild discards all derived state and reconstructs the whole world
// from scratch: seed + params + edit log. The rebuilt chunks are
// byte-identical to the incrementally maintained chunks with the same
// edit log, and the settlement/road layers are identical too.
func (w *World) Rebuild(ctx context.Context, opts GenerateOptions) error {
	overlay := w.overlay // edit log is source-of-truth state
	rendered, err := generateChunks(ctx, w.params, overlay, w.allCoords(), opts.Concurrency)
	if err != nil {
		return err
	}
	w.chunks = rendered
	if err := w.derive(); err != nil {
		return err
	}
	return nil
}

// UpdateParams changes generation parameters. Terrain-independent
// parameter changes leave every materialized terrain cell untouched
// (only derived layers are recomputed); terrain-affecting changes are
// rejected here — callers must build a new world for those, keeping
// old worlds pinned to their TerrainDigest.
func (w *World) UpdateParams(p Params) error {
	p.Normalize()
	if err := p.Validate(); err != nil {
		return err
	}
	if p.TerrainDigest() != w.params.TerrainDigest() {
		return ErrTerrainParamsChanged
	}
	w.params = p
	return w.derive()
}

// ErrTerrainParamsChanged is returned when a parameter update would
// alter existing terrain regions.
var ErrTerrainParamsChanged = constErr("worldgen: terrain-affecting parameters changed; create a new versioned world")

type constErr string

func (e constErr) Error() string { return string(e) }

// Settlements returns the canonical (ID-ordered) settlement list.
func (w *World) Settlements() []Settlement {
	out := make([]Settlement, len(w.sites))
	copy(out, w.sites)
	return out
}

// Roads returns the canonical road graph.
func (w *World) Roads() *RoadGraph {
	if w.roads == nil {
		return &RoadGraph{}
	}
	out := &RoadGraph{Edges: make([]RoadEdge, len(w.roads.Edges))}
	for i, e := range w.roads.Edges {
		path := make([]RoadPoint, len(e.Path))
		copy(path, e.Path)
		out.Edges[i] = RoadEdge{A: e.A, B: e.B, Cost: e.Cost, Path: path}
	}
	return out
}

// Chunk returns a materialized chunk.
func (w *World) Chunk(c ChunkCoord) (*Chunk, bool) {
	ch, ok := w.chunks[c]
	return ch, ok
}

// Chunks returns the sorted coordinates of materialized chunks.
func (w *World) Chunks() []ChunkCoord {
	out := make([]ChunkCoord, 0, len(w.chunks))
	for c := range w.chunks {
		out = append(out, c)
	}
	sortCoords(out)
	return out
}

// Edits returns a copy of the recorded cell edits in canonical order.
func (w *World) Edits() []CellEdit {
	keys := make([][2]int, 0, w.overlay.len())
	for k := range w.overlay.m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][1] != keys[j][1] {
			return keys[i][1] < keys[j][1]
		}
		return keys[i][0] < keys[j][0]
	})
	out := make([]CellEdit, 0, len(keys))
	for _, k := range keys {
		out = append(out, w.overlay.m[k])
	}
	return out
}

// EncodeWorld serializes the full deterministic world state in a
// canonical big-endian binary form: magic/version header, parameter
// digests, edit log, materialized chunks (sorted), settlements and the
// road graph. Equal worlds encode to equal bytes.
func EncodeWorld(w *World) []byte {
	buf := make([]byte, 0, 4096)
	putU16 := func(v uint16) { buf = binary.BigEndian.AppendUint16(buf, v) }
	putU32 := func(v uint32) { buf = binary.BigEndian.AppendUint32(buf, v) }
	putU64 := func(v uint64) { buf = binary.BigEndian.AppendUint64(buf, v) }

	buf = append(buf, []byte("WGEN")...)
	putU16(1) // serialization format version

	td := w.params.TerrainDigest()
	d := w.params.Digest()
	buf = append(buf, td[:]...)
	buf = append(buf, d[:]...)

	buf = append(buf, w.overlay.encode()...)

	coords := w.Chunks()
	putU32(uint32(len(coords)))
	for _, c := range coords {
		ch := w.chunks[c]
		putU32(uint32(c.CX))
		putU32(uint32(c.CY))
		putU32(uint32(ch.Size))
		for _, cell := range ch.Cells {
			putU16(uint16(cell.Height))
			putU16(uint16(cell.Moisture))
			putU16(uint16(cell.Biome))
		}
	}

	putU32(uint32(len(w.sites)))
	for _, s := range w.sites {
		putU64(s.ID)
		putU32(uint32(s.X))
		putU32(uint32(s.Y))
		putU16(s.Suitability)
		name := []byte(s.Name)
		putU32(uint32(len(name)))
		buf = append(buf, name...)
	}

	putU32(uint32(len(w.roads.Edges)))
	for _, e := range w.roads.Edges {
		putU64(e.A)
		putU64(e.B)
		putU64(e.Cost)
		putU32(uint32(len(e.Path)))
		for _, pt := range e.Path {
			putU32(uint32(pt.X))
			putU32(uint32(pt.Y))
		}
	}
	return buf
}
