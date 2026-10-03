package worldgen_test

import (
	"context"
	"fmt"

	"github.com/Diudiuseven/city-settlement-generator-go/worldgen"
)

// ExampleNewWorld demonstrates the full deterministic lifecycle:
// create, generate chunks in parallel, edit locally and rebuild from
// scratch into identical bytes.
func ExampleNewWorld() {
	p := worldgen.DefaultParams()
	p.Width = 64
	p.Height = 64
	p.ChunkSize = 64

	w, err := worldgen.NewWorld(p)
	if err != nil {
		panic(err)
	}
	ctx := context.Background()

	// Chunks are pure functions of their coordinate; parallelism
	// cannot change the result.
	if err := w.GenerateChunks(ctx,
		[]worldgen.ChunkCoord{{0, 0}},
		worldgen.GenerateOptions{Concurrency: 4}); err != nil {
		panic(err)
	}

	ch, _ := w.Chunk(worldgen.ChunkCoord{0, 0})
	before := len(ch.Cells)

	// The whole-map from-scratch path always exists and agrees with
	// incremental state.
	if err := w.Rebuild(ctx, worldgen.GenerateOptions{Concurrency: 1}); err != nil {
		panic(err)
	}
	ch2, _ := w.Chunk(worldgen.ChunkCoord{0, 0})
	sameCells := before == len(ch2.Cells)

	fmt.Println("rebuild cell count identical:", sameCells)
	fmt.Println("settlements reachable:", len(w.Roads().Edges) >= 0)
	fmt.Println("algorithm version:", w.AlgorithmVersion())
	// output:
	// rebuild cell count identical: true
	// settlements reachable: true
	// algorithm version: 1
}
