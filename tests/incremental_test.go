package tests

import (
	"context"
	"testing"

	"github.com/Diudiuseven/city-settlement-generator-go/pipeline"
	"github.com/Diudiuseven/city-settlement-generator-go/world"
	"github.com/Diudiuseven/city-settlement-generator-go/worldgen"
)

// flattenEdit builds an edit that sets the height of the given local tiles.
func flattenEdit(id string, coord world.ChunkCoord, indices []int, height float64) world.Edit {
	tiles := make(map[int]world.TileOverride, len(indices))
	for _, idx := range indices {
		h := height
		tiles[idx] = world.TileOverride{Height: &h}
	}
	return world.Edit{ID: id, Chunk: coord, Tiles: tiles}
}

func blockIndices(size, x0, y0, w, h int) []int {
	var out []int
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			out = append(out, y*size+x)
		}
	}
	return out
}

// rebuildWorld creates a fresh world, applies edits first, then generates
// everything and rebuilds roads — the "from scratch" reference.
func rebuildWorld(t *testing.T, seed int64, coords []world.ChunkCoord, edits []world.Edit) *world.World {
	t.Helper()
	w, err := world.New(seed, testParams())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range edits {
		w.ApplyEdit(e)
	}
	if _, err := pipeline.Generate(context.Background(), w, coords, pipeline.Options{Workers: 4}); err != nil {
		t.Fatal(err)
	}
	if _, err := worldgen.RecomputeRoads(w); err != nil {
		t.Fatal(err)
	}
	return w
}

// incrementalWorld generates first, then applies edits and recomputes — the
// "incremental" path.
func incrementalWorld(t *testing.T, seed int64, coords []world.ChunkCoord, edits []world.Edit) *world.World {
	t.Helper()
	w := generateWorld(t, seed, coords, 4)
	for _, e := range edits {
		w.ApplyEdit(e)
		ch, err := worldgen.GenerateChunk(w, e.Chunk)
		if err != nil {
			t.Fatal(err)
		}
		w.PutChunk(ch)
	}
	if _, err := worldgen.RecomputeRoads(w); err != nil {
		t.Fatal(err)
	}
	return w
}

func assertWorldsEqual(t *testing.T, a, b *world.World, coords []world.ChunkCoord) {
	t.Helper()
	for _, c := range coords {
		ab := chunkBytes(t, a.GetChunk(c))
		bb := chunkBytes(t, b.GetChunk(c))
		if string(ab) != string(bb) {
			t.Fatalf("chunk %v differs between incremental and rebuild", c)
		}
	}
}

// TestIncrementalEqualsRebuild: after a local edit, regenerating the affected
// chunk and recomputing roads yields the same map as rebuilding from scratch.
func TestIncrementalEqualsRebuild(t *testing.T) {
	coords := testCoords()
	size := testParams().ChunkSize
	edit := flattenEdit("flatten-1", world.ChunkCoord{X: 0, Y: 0}, blockIndices(size, 10, 10, 4, 4), 0.5)

	inc := incrementalWorld(t, 4242, coords, []world.Edit{edit})
	reb := rebuildWorld(t, 4242, coords, []world.Edit{edit})
	assertWorldsEqual(t, inc, reb, coords)
}

// TestMultipleEditsRebuildEquivalent: several edits across chunks.
func TestMultipleEditsRebuildEquivalent(t *testing.T) {
	coords := testCoords()
	size := testParams().ChunkSize
	edits := []world.Edit{
		flattenEdit("e1", world.ChunkCoord{X: 0, Y: 0}, blockIndices(size, 5, 5, 3, 3), 0.4),
		flattenEdit("e2", world.ChunkCoord{X: 2, Y: 1}, blockIndices(size, 8, 8, 5, 2), 0.7),
		flattenEdit("e3", world.ChunkCoord{X: 1, Y: 2}, blockIndices(size, 20, 20, 4, 4), 0.35),
	}
	inc := incrementalWorld(t, 777, coords, edits)
	reb := rebuildWorld(t, 777, coords, edits)
	assertWorldsEqual(t, inc, reb, coords)
}

// TestEditOrderIndependence: edits applied in any order produce the same map.
func TestEditOrderIndependence(t *testing.T) {
	coords := testCoords()
	size := testParams().ChunkSize
	// e2 and e3 overlap on one tile; the greater id must win regardless of
	// submission order.
	e1 := flattenEdit("e1", world.ChunkCoord{X: 0, Y: 0}, blockIndices(size, 5, 5, 3, 3), 0.4)
	e2 := flattenEdit("e2", world.ChunkCoord{X: 0, Y: 0}, blockIndices(size, 6, 6, 3, 3), 0.9)
	e3 := flattenEdit("e3", world.ChunkCoord{X: 0, Y: 0}, blockIndices(size, 7, 7, 3, 3), 0.2)

	orders := [][]world.Edit{{e1, e2, e3}, {e3, e1, e2}, {e2, e3, e1}, {e3, e2, e1}}
	var reference []byte
	for i, order := range orders {
		w := generateWorld(t, 5150, coords, 4)
		for _, e := range order {
			w.ApplyEdit(e)
			ch, err := worldgen.GenerateChunk(w, e.Chunk)
			if err != nil {
				t.Fatal(err)
			}
			w.PutChunk(ch)
		}
		if _, err := worldgen.RecomputeRoads(w); err != nil {
			t.Fatal(err)
		}
		b := chunkBytes(t, w.GetChunk(world.ChunkCoord{X: 0, Y: 0}))
		if i == 0 {
			reference = b
		} else if string(b) != string(reference) {
			t.Fatalf("edit order %v produced a different chunk", order)
		}
	}
}

// TestDuplicateEditIdempotent: submitting the same edit id twice is a no-op.
func TestDuplicateEditIdempotent(t *testing.T) {
	coords := testCoords()
	size := testParams().ChunkSize
	edit := flattenEdit("dup-1", world.ChunkCoord{X: 0, Y: 0}, blockIndices(size, 10, 10, 4, 4), 0.5)

	w := generateWorld(t, 6060, coords, 4)
	r1 := w.ApplyEdit(edit)
	if !r1.Applied {
		t.Fatal("first application should be applied")
	}
	ch, err := worldgen.GenerateChunk(w, edit.Chunk)
	if err != nil {
		t.Fatal(err)
	}
	w.PutChunk(ch)
	before := chunkBytes(t, w.GetChunk(edit.Chunk))

	r2 := w.ApplyEdit(edit)
	if r2.Applied {
		t.Fatal("duplicate application should be a no-op")
	}
	after := chunkBytes(t, w.GetChunk(edit.Chunk))
	if string(before) != string(after) {
		t.Fatal("duplicate edit changed the map")
	}

	// And the result matches a rebuild with the edit once.
	if _, err := worldgen.RecomputeRoads(w); err != nil {
		t.Fatal(err)
	}
	reb := rebuildWorld(t, 6060, coords, []world.Edit{edit})
	assertWorldsEqual(t, w, reb, coords)
}

// TestEditLocalEffect: editing chunk (0,0) must not change terrain of a
// non-neighbour chunk, and must not change a far-away chunk at all. The
// affected region is the dependency closure (edited chunk, its settlement
// halo, and road corridors) — never the whole map.
func TestEditLocalEffect(t *testing.T) {
	coords := make([]world.ChunkCoord, 0, 25)
	for y := 0; y < 5; y++ {
		for x := 0; x < 5; x++ {
			coords = append(coords, world.ChunkCoord{X: x, Y: y})
		}
	}
	size := testParams().ChunkSize
	w := generateWorld(t, 9090, coords, 4)
	if _, err := worldgen.RecomputeRoads(w); err != nil {
		t.Fatal(err)
	}
	farBefore := chunkBytes(t, w.GetChunk(world.ChunkCoord{X: 4, Y: 4}))
	midBefore := w.GetChunk(world.ChunkCoord{X: 2, Y: 2})

	edit := flattenEdit("local-1", world.ChunkCoord{X: 0, Y: 0}, blockIndices(size, 2, 2, 6, 6), 0.6)
	w.ApplyEdit(edit)
	ch, err := worldgen.GenerateChunk(w, edit.Chunk)
	if err != nil {
		t.Fatal(err)
	}
	w.PutChunk(ch)
	if _, err := worldgen.RecomputeRoads(w); err != nil {
		t.Fatal(err)
	}

	farAfter := chunkBytes(t, w.GetChunk(world.ChunkCoord{X: 4, Y: 4}))
	if string(farBefore) != string(farAfter) {
		t.Fatal("edit in (0,0) changed far-away chunk (4,4)")
	}
	// Terrain (tiles) of a non-neighbour chunk is unaffected by the edit.
	midAfter := w.GetChunk(world.ChunkCoord{X: 2, Y: 2})
	for i := range midBefore.Tiles {
		a := midBefore.Tiles[i]
		b := midAfter.Tiles[i]
		if a.Height != b.Height || a.Moisture != b.Moisture || a.Biome != b.Biome {
			t.Fatalf("terrain of non-neighbour chunk (2,2) changed at tile %d", i)
		}
	}
}

// TestEditReplayAfterClear: clearing chunks and rebuilding replays edits.
func TestEditReplayAfterClear(t *testing.T) {
	coords := testCoords()
	size := testParams().ChunkSize
	edit := flattenEdit("replay-1", world.ChunkCoord{X: 1, Y: 1}, blockIndices(size, 12, 12, 4, 4), 0.45)

	w := generateWorld(t, 3030, coords, 4)
	w.ApplyEdit(edit)
	ch, err := worldgen.GenerateChunk(w, edit.Chunk)
	if err != nil {
		t.Fatal(err)
	}
	w.PutChunk(ch)
	if _, err := worldgen.RecomputeRoads(w); err != nil {
		t.Fatal(err)
	}
	want := chunkBytes(t, w.GetChunk(edit.Chunk))

	w.ClearChunks()
	if _, err := pipeline.Generate(context.Background(), w, coords, pipeline.Options{Workers: 4}); err != nil {
		t.Fatal(err)
	}
	if _, err := worldgen.RecomputeRoads(w); err != nil {
		t.Fatal(err)
	}
	got := chunkBytes(t, w.GetChunk(edit.Chunk))
	if string(want) != string(got) {
		t.Fatal("rebuild after clear did not replay edits")
	}
}
