// Package pipeline runs deterministic chunk generation concurrently.
//
// The pipeline treats chunk generation as a set of independent pure functions
// of (seed, params, edits, coordinate). Workers never share mutable state, so
// results are independent of worker count, scheduling order and chunk arrival
// order. Cancellation or timeout aborts the whole batch: callers never observe
// a partial result set.
package pipeline

import (
	"context"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/Diudiuseven/city-settlement-generator-go/world"
	"github.com/Diudiuseven/city-settlement-generator-go/worldgen"
)

// Options controls parallel generation.
type Options struct {
	// Workers is the number of concurrent chunk generators. Zero means
	// runtime.NumCPU().
	Workers int
	// Timeout bounds the whole batch. Zero means no timeout.
	Timeout time.Duration
}

// Result is the outcome of generating one chunk.
type Result struct {
	Chunk *world.Chunk
	Err   error
}

// Generate generates the given chunks concurrently and stores them in the
// world. It returns the generated chunks sorted by coordinate.
//
// Determinism guarantee: the returned chunks depend only on (seed, params,
// edits, coords); worker count, scheduling and arrival order cannot change
// them.
//
// Atomicity guarantee: if the context is cancelled or the timeout elapses,
// Generate returns an error and NO partial result is stored or returned.
func Generate(ctx context.Context, w *world.World, coords []world.ChunkCoord, opts Options) ([]*world.Chunk, error) {
	if opts.Workers <= 0 {
		opts.Workers = runtime.NumCPU()
	}
	if opts.Workers > len(coords) {
		opts.Workers = len(coords)
	}
	if opts.Workers < 1 {
		opts.Workers = 1
	}
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	// Deduplicate coordinates while preserving nothing else (order does not
	// matter: results are sorted afterwards).
	seen := make(map[world.ChunkCoord]bool, len(coords))
	unique := make([]world.ChunkCoord, 0, len(coords))
	for _, c := range coords {
		if !seen[c] {
			seen[c] = true
			unique = append(unique, c)
		}
	}

	jobs := make(chan world.ChunkCoord)
	results := make(chan Result, len(unique))
	var wg sync.WaitGroup
	for i := 0; i < opts.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range jobs {
				ch, err := worldgen.GenerateChunk(w, c)
				results <- Result{Chunk: ch, Err: err}
			}
		}()
	}

	// Feed jobs; stop early if the caller cancels.
	go func() {
		for _, c := range unique {
			select {
			case jobs <- c:
			case <-ctx.Done():
				close(jobs)
				return
			}
		}
		close(jobs)
	}()

	// Collect every result. No partial result is stored until the whole batch
	// succeeds, so cancellation/timeout is atomic from the caller's view.
	collected := make(map[world.ChunkCoord]*world.Chunk, len(unique))
	var firstErr error
	for i := 0; i < len(unique); i++ {
		select {
		case <-ctx.Done():
			// Drain workers so no goroutine leaks, then report the error.
			go func() {
				for range results {
				}
			}()
			wg.Wait()
			return nil, ctx.Err()
		case r := <-results:
			if r.Err != nil {
				firstErr = r.Err
				continue
			}
			collected[r.Chunk.Coord] = r.Chunk
		}
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	out := make([]*world.Chunk, 0, len(unique))
	for _, c := range unique {
		ch := collected[c]
		if ch == nil {
			return nil, context.Canceled
		}
		w.PutChunk(ch)
		out = append(out, ch)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Coord.X != out[j].Coord.X {
			return out[i].Coord.X < out[j].Coord.X
		}
		return out[i].Coord.Y < out[j].Coord.Y
	})
	return out, nil
}
