package tests

import (
	"context"
	"math"
	"testing"

	"github.com/Diudiuseven/city-settlement-generator-go/pipeline"
	"github.com/Diudiuseven/city-settlement-generator-go/world"
)

// generateWorldWithChunkSize builds a world with a custom chunk size.
func generateWorldWithChunkSize(t *testing.T, seed int64, size int, coords []world.ChunkCoord, workers int) *world.World {
	t.Helper()
	p := world.DefaultParams()
	p.ChunkSize = size
	w, err := world.New(seed, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.Generate(context.Background(), w, coords, pipeline.Options{Workers: workers}); err != nil {
		t.Fatal(err)
	}
	return w
}

// globalTile returns the tile at global coordinates from a world, looking up
// the chunk that contains it.
func globalTile(w *world.World, tx, ty int) *world.Tile {
	size := w.ChunkSize()
	cx := tx / size
	cy := ty / size
	if tx < 0 {
		cx--
	}
	if ty < 0 {
		cy--
	}
	ch := w.GetChunk(world.ChunkCoord{X: cx, Y: cy})
	if ch == nil {
		return nil
	}
	return ch.GlobalTile(tx, ty)
}

// TestSeamlessness: the same global tiles must be produced regardless of
// chunk boundaries. Two worlds with different chunk sizes (32 vs 16) covering
// the same global region must agree on every tile.
func TestSeamlessness(t *testing.T) {
	coords32 := []world.ChunkCoord{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}, {X: 1, Y: 1}}
	w32 := generateWorldWithChunkSize(t, 77, 32, coords32, 4)
	coords16 := []world.ChunkCoord{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 2, Y: 0}, {X: 3, Y: 0},
		{X: 0, Y: 1}, {X: 1, Y: 1}, {X: 2, Y: 1}, {X: 3, Y: 1},
		{X: 0, Y: 2}, {X: 1, Y: 2}, {X: 2, Y: 2}, {X: 3, Y: 2},
		{X: 0, Y: 3}, {X: 1, Y: 3}, {X: 2, Y: 3}, {X: 3, Y: 3},
	}
	w16 := generateWorldWithChunkSize(t, 77, 16, coords16, 4)

	for ty := 0; ty < 64; ty++ {
		for tx := 0; tx < 64; tx++ {
			a := globalTile(w32, tx, ty)
			b := globalTile(w16, tx, ty)
			if a == nil || b == nil {
				t.Fatalf("missing tile at (%d,%d)", tx, ty)
			}
			if a.Height != b.Height || a.Moisture != b.Moisture || a.Temperature != b.Temperature || a.Biome != b.Biome {
				t.Fatalf("tile (%d,%d) differs across chunk sizes: h %.6f/%.6f m %.6f/%.6f",
					tx, ty, a.Height, b.Height, a.Moisture, b.Moisture)
			}
		}
	}
}

// TestBorderContinuity: adjacent tiles across a chunk border are as
// continuous as adjacent tiles inside a chunk (no cliff-like seams).
func TestBorderContinuity(t *testing.T) {
	coords := []world.ChunkCoord{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}, {X: 1, Y: 1}}
	w := generateWorld(t, 77, coords, 4)
	size := w.ChunkSize()

	// Across the vertical border between (0,0) and (1,0).
	for y := 0; y < size; y++ {
		inside := math.Abs(globalTile(w, size-2, y).Height - globalTile(w, size-1, y).Height)
		across := math.Abs(globalTile(w, size-1, y).Height - globalTile(w, size, y).Height)
		if across > inside+0.05 {
			t.Fatalf("vertical seam discontinuity at y=%d: across %.4f inside %.4f", y, across, inside)
		}
	}
	// Across the horizontal border between (0,0) and (0,1).
	for x := 0; x < size; x++ {
		inside := math.Abs(globalTile(w, x, size-2).Height - globalTile(w, x, size-1).Height)
		across := math.Abs(globalTile(w, x, size-1).Height - globalTile(w, x, size).Height)
		if across > inside+0.05 {
			t.Fatalf("horizontal seam discontinuity at x=%d: across %.4f inside %.4f", x, across, inside)
		}
	}
}

// TestSingleChunkEqualsRegion: generating a chunk alone yields the same bytes
// as generating it together with all its neighbours.
func TestSingleChunkEqualsRegion(t *testing.T) {
	region := generateWorld(t, 31337, testCoords(), 4)
	lone := generateWorld(t, 31337, []world.ChunkCoord{{X: 1, Y: 1}}, 1)

	rb := chunkBytes(t, region.GetChunk(world.ChunkCoord{X: 1, Y: 1}))
	lb := chunkBytes(t, lone.GetChunk(world.ChunkCoord{X: 1, Y: 1}))
	if string(rb) != string(lb) {
		t.Fatal("chunk generated alone differs from chunk generated with neighbours")
	}
}

// TestSeamlessAcrossSeeds: the chunk-size independence holds for many seeds.
func TestSeamlessAcrossSeeds(t *testing.T) {
	for _, seed := range []int64{0, 1, 42, 999999} {
		w32 := generateWorldWithChunkSize(t, seed, 32, []world.ChunkCoord{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}, {X: 1, Y: 1}}, 4)
		w16 := generateWorldWithChunkSize(t, seed, 16, []world.ChunkCoord{
			{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 2, Y: 0}, {X: 3, Y: 0},
			{X: 0, Y: 1}, {X: 1, Y: 1}, {X: 2, Y: 1}, {X: 3, Y: 1},
			{X: 0, Y: 2}, {X: 1, Y: 2}, {X: 2, Y: 2}, {X: 3, Y: 2},
			{X: 0, Y: 3}, {X: 1, Y: 3}, {X: 2, Y: 3}, {X: 3, Y: 3},
		}, 4)
		for ty := 0; ty < 64; ty++ {
			for tx := 0; tx < 64; tx++ {
				a := globalTile(w32, tx, ty)
				b := globalTile(w16, tx, ty)
				if a == nil || b == nil {
					t.Fatalf("seed %d: missing tile (%d,%d)", seed, tx, ty)
				}
				if a.Height != b.Height || a.Moisture != b.Moisture {
					t.Fatalf("seed %d: tile (%d,%d) differs across chunk sizes", seed, tx, ty)
				}
			}
		}
	}
}
