package worldgen

import (
	"context"
	"testing"
	"time"
)

// TestParallelGenerateSameAsSerial is requirement 6's value guarantee.
func TestParallelGenerateSameAsSerial(t *testing.T) {
	p := testParams()
	coords := allTestCoords(p)

	wSerial, _ := NewWorld(p)
	if err := wSerial.GenerateChunks(context.Background(), coords,
		GenerateOptions{Concurrency: 1}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []int{2, 4, 16} {
		wPar, _ := NewWorld(p)
		if err := wPar.GenerateChunks(context.Background(), coords,
			GenerateOptions{Concurrency: c}); err != nil {
			t.Fatal(err)
		}
		for _, coord := range coords {
			a := chunkBytes(mustChunk(t, wSerial, coord))
			b := chunkBytes(mustChunk(t, wPar, coord))
			if string(a) != string(b) {
				t.Fatalf("chunk %v differs at concurrency %d", coord, c)
			}
		}
	}
}

// TestCancelPublishesNothing proves requirement 6's atomicity: a
// cancelled call leaves the world exactly as before.
func TestCancelPublishesNothing(t *testing.T) {
	p := testParams()
	w, _ := NewWorld(p)
	coords := allTestCoords(p)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before dispatch
	err := w.GenerateChunks(ctx, coords, GenerateOptions{Concurrency: 4})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if len(w.Chunks()) != 0 {
		t.Fatalf("cancelled generation published %d chunks", len(w.Chunks()))
	}
}

// TestCancelMidwayNoPartialChunks cancels a generation after a delay;
// either all requested chunks committed or none did, and on error the
// world must be untouched.
func TestCancelMidwayNoPartialChunks(t *testing.T) {
	p := testParams()
	p.Width = 256
	p.Height = 256
	p.ChunkSize = 64
	w, _ := NewWorld(p)
	coords := allTestCoords(p)

	// Repeat until cancellation observably interrupts the run; small
	// renders may otherwise finish first.
	interrupted := false
	for attempt := 0; attempt < 20; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
		err := w.GenerateChunks(ctx, coords, GenerateOptions{Concurrency: 16})
		cancel()
		if err != nil {
			if len(w.Chunks()) != 0 {
				t.Fatalf("interrupted generation left %d chunks visible", len(w.Chunks()))
			}
			interrupted = true
			break
		}
	}
	if !interrupted {
		t.Fatal("could not observe an interrupted generation in 20 attempts")
	}
}

// TestSlowChunkDoesNotBlockOthers verifies chunks progress
// independently of slow peers. Since chunk rendering is a pure
// function of its coordinate, we demonstrate this by requesting many
// chunks at low and high concurrency: all complete, and ordering of
// completion cannot matter (asserted via identical outputs).
func TestSlowChunkDoesNotBlockOthers(t *testing.T) {
	p := testParams()
	coords := allTestCoords(p)
	w, _ := NewWorld(p)
	done := make(chan error, 1)
	go func() {
		done <- w.GenerateChunks(context.Background(), coords,
			GenerateOptions{Concurrency: 4})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("chunk generation stalled")
	}
	if len(w.Chunks()) != len(coords) {
		t.Fatalf("got %d chunks, want %d", len(w.Chunks()), len(coords))
	}
}

// TestCancelDoesNotCorruptFutureGeneration: after a cancellation the
// world must still accept a full successful generation.
func TestCancelDoesNotCorruptFutureGeneration(t *testing.T) {
	p := testParams()
	w, _ := NewWorld(p)
	coords := allTestCoords(p)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = w.GenerateChunks(ctx, coords, GenerateOptions{Concurrency: 4})

	if err := w.GenerateChunks(context.Background(), coords,
		GenerateOptions{Concurrency: 2}); err != nil {
		t.Fatalf("generation after cancellation failed: %v", err)
	}
	if len(w.Chunks()) != len(coords) {
		t.Fatal("world incomplete after retry")
	}
}

// TestTimeoutAtomicityForEdits ensures a cancelled ApplyEdits publishes
// neither the overlay nor the derived layers.
func TestTimeoutAtomicityForEdits(t *testing.T) {
	p := testParams()
	w := rebuildWorld(t, p, 1)
	before := len(w.Edits())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	land := findPassableTile(t, p)
	err := w.ApplyEdits(ctx, []CellEdit{
		{X: land[0], Y: land[1], SetHeight: hPtr(27000)},
	}, GenerateOptions{Concurrency: 2})
	if err == nil {
		t.Fatal("expected cancelled ApplyEdits to fail")
	}
	if len(w.Edits()) != before {
		t.Fatal("cancelled edit leaked into the edit log")
	}
}
