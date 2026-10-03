package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Diudiuseven/city-settlement-generator-go/pipeline"
	"github.com/Diudiuseven/city-settlement-generator-go/world"
)

// TestConcurrencyDeterminism: worker count and submission order cannot change
// the result.
func TestConcurrencyDeterminism(t *testing.T) {
	coords := testCoords()
	base := generateWorld(t, 2024, coords, 1)
	want := make(map[world.ChunkCoord][]byte, len(coords))
	for _, c := range coords {
		want[c] = chunkBytes(t, base.GetChunk(c))
	}

	shuffled := []world.ChunkCoord{
		{X: 2, Y: 2}, {X: 0, Y: 0}, {X: 1, Y: 2}, {X: 0, Y: 1}, {X: 2, Y: 0},
		{X: 1, Y: 1}, {X: 0, Y: 2}, {X: 2, Y: 1}, {X: 1, Y: 0},
	}
	for _, workers := range []int{1, 2, 4, 8, 16} {
		w := generateWorld(t, 2024, shuffled, workers)
		for _, c := range coords {
			got := chunkBytes(t, w.GetChunk(c))
			if string(got) != string(want[c]) {
				t.Fatalf("workers=%d: chunk %v differs", workers, c)
			}
		}
	}
}

// TestCancelAtomic: a cancelled context aborts the batch and stores nothing.
func TestCancelAtomic(t *testing.T) {
	w, err := world.New(7, testParams())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = pipeline.Generate(ctx, w, testCoords(), pipeline.Options{Workers: 4})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if w.ChunkCount() != 0 {
		t.Fatalf("cancelled batch stored %d chunks", w.ChunkCount())
	}
}

// TestTimeoutAtomic: an expired timeout aborts the batch and stores nothing.
func TestTimeoutAtomic(t *testing.T) {
	w, err := world.New(8, testParams())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	_, err = pipeline.Generate(ctx, w, testCoords(), pipeline.Options{Workers: 8})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
	if w.ChunkCount() != 0 {
		t.Fatalf("timed-out batch stored %d chunks", w.ChunkCount())
	}
}

// TestSlowChunksDoNotBlockOthers: a large batch with many workers completes
// and produces identical results to a single-worker run.
func TestSlowChunksDoNotBlockOthers(t *testing.T) {
	coords := make([]world.ChunkCoord, 0, 64)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			coords = append(coords, world.ChunkCoord{X: x, Y: y})
		}
	}
	one := generateWorld(t, 4242, coords, 1)
	many := generateWorld(t, 4242, coords, 8)
	for _, c := range coords {
		if string(chunkBytes(t, one.GetChunk(c))) != string(chunkBytes(t, many.GetChunk(c))) {
			t.Fatalf("chunk %v differs between 1 and 8 workers", c)
		}
	}
}

// TestDuplicateCoords: requesting the same coordinate twice is harmless.
func TestDuplicateCoords(t *testing.T) {
	coords := []world.ChunkCoord{{X: 0, Y: 0}, {X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 0}}
	w := generateWorld(t, 5, coords, 4)
	if w.ChunkCount() != 2 {
		t.Fatalf("expected 2 unique chunks, got %d", w.ChunkCount())
	}
}
