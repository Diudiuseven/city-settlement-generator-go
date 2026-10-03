// Package world contains the deterministic world model: parameters, chunks,
// tiles, settlements, edits and their stable binary encoding.
package world

import (
	"fmt"
	"hash/fnv"
	"sort"
	"sync"
)

// World is a deterministic, chunked map. The world is a pure function of its
// seed, its parameters and the set of applied edits: generating the same
// region twice, in any order, with any concurrency, yields byte-identical
// chunks.
type World struct {
	seed   int64
	params Params

	mu     sync.RWMutex
	chunks map[ChunkCoord]*Chunk
	// edits is keyed by edit ID; the map makes duplicate submissions no-ops.
	edits map[string]Edit
}

// New creates an empty world bound to a seed and a parameter set.
// The seed is the only source of randomness; every tile is derived from it.
func New(seed int64, params Params) (*World, error) {
	if err := params.Normalize(); err != nil {
		return nil, err
	}
	return &World{
		seed:   seed,
		params: params,
		chunks: make(map[ChunkCoord]*Chunk),
		edits:  make(map[string]Edit),
	}, nil
}

// Seed returns the world seed.
func (w *World) Seed() int64 { return w.seed }

// Params returns a copy of the world parameters.
func (w *World) Params() Params { return w.params }

// ChunkSize returns the configured chunk edge length.
func (w *World) ChunkSize() int { return w.params.ChunkSize }

// AlgorithmVersion returns the algorithm version the world was configured with.
func (w *World) AlgorithmVersion() int { return w.params.AlgorithmVersion }

// HasChunk reports whether a chunk is currently resident.
func (w *World) HasChunk(c ChunkCoord) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	_, ok := w.chunks[c]
	return ok
}

// GetChunk returns a resident chunk, or nil if it has not been generated.
func (w *World) GetChunk(c ChunkCoord) *Chunk {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.chunks[c]
}

// PutChunk stores a chunk, replacing any previous version.
func (w *World) PutChunk(ch *Chunk) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.chunks[ch.Coord] = ch
}

// ChunkCoords returns the coordinates of all resident chunks, sorted.
func (w *World) ChunkCoords() []ChunkCoord {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make([]ChunkCoord, 0, len(w.chunks))
	for c := range w.chunks {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].X != out[j].X {
			return out[i].X < out[j].X
		}
		return out[i].Y < out[j].Y
	})
	return out
}

// ChunkCount returns the number of resident chunks.
func (w *World) ChunkCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.chunks)
}

// ClearChunks discards all resident chunks. Edits are retained, so a later
// rebuild regenerates the edited world from scratch.
func (w *World) ClearChunks() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.chunks = make(map[ChunkCoord]*Chunk)
}

// ApplyEdit submits an edit. Submitting an edit with an ID that was already
// submitted is a no-op and reports Applied=false. Edits are order-independent:
// if two edits override the same tile, the edit with the lexicographically
// greatest ID wins, no matter in which order they were submitted.
func (w *World) ApplyEdit(e Edit) EditResult {
	if e.ID == "" {
		e.ID = deriveEditID(e)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, exists := w.edits[e.ID]; exists {
		return EditResult{Chunk: e.Chunk, Applied: false}
	}
	w.edits[e.ID] = e
	return EditResult{Chunk: e.Chunk, Applied: true, ChangedTiles: len(e.Tiles)}
}

// Edits returns a copy of the edit log keyed by edit ID.
func (w *World) Edits() map[string]Edit {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make(map[string]Edit, len(w.edits))
	for k, v := range w.edits {
		out[k] = v
	}
	return out
}

// EditsForChunk returns the edits touching a chunk, in deterministic order
// (sorted by edit ID).
func (w *World) EditsForChunk(c ChunkCoord) []Edit {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var out []Edit
	for _, e := range w.edits {
		if e.Chunk == c {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// deriveEditID produces a deterministic content hash for edits without an ID.
func deriveEditID(e Edit) string {
	h := fnv.New64a()
	fmt.Fprintf(h, "chunk:%d,%d", e.Chunk.X, e.Chunk.Y)
	keys := make([]int, 0, len(e.Tiles))
	for k := range e.Tiles {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for _, k := range keys {
		o := e.Tiles[k]
		fmt.Fprintf(h, "tile:%d", k)
		if o.Height != nil {
			fmt.Fprintf(h, "h:%.12f", *o.Height)
		}
		if o.Moisture != nil {
			fmt.Fprintf(h, "m:%.12f", *o.Moisture)
		}
	}
	return fmt.Sprintf("edit-%016x", h.Sum64())
}
